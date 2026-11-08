package cli

import (
	"bytes"
	"context"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/contract"
	"github.com/Pirite08/finlang/internal/transfer"
)

type transferReplayOutput struct {
	Status                  string          `json:"status"`
	RequestID               string          `json:"request_id"`
	Rule                    string          `json:"rule"`
	RuleVersion             string          `json:"rule_version"`
	ContractArtifactVersion string          `json:"contract_artifact_version"`
	Result                  json.RawMessage `json:"result"`
}

func runTransferReplay(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("fin replay-transfer", flag.ContinueOnError)
	flags.SetOutput(stderr)
	database := flags.String("db", "", "path to the transfer ledger")
	id := flags.String("id", "", "committed transfer request ID")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *database == "" || *id == "" || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "E_CLI_USAGE: replay-transfer requires -db and -id")
		return 2
	}
	store, err := transfer.OpenReadOnly(ctx, *database)
	if err != nil {
		fmt.Fprintln(stderr, "E_AUDIT_REPLAY: could not open the transfer ledger read-only")
		return 2
	}
	defer store.Close()

	output, err := replayTransfer(ctx, store, *id)
	if err != nil {
		fmt.Fprintf(stderr, "E_AUDIT_REPLAY: %v\n", err)
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(output); err != nil {
		fmt.Fprintln(stderr, "E_OUTPUT_IO: could not write the replay result")
		return 2
	}
	return 0
}

func replayTransfer(ctx context.Context, store *transfer.Store, id string) (transferReplayOutput, error) {
	receipt, rawAudit, err := store.ReadPlannedTransfer(ctx, id)
	if err != nil {
		return transferReplayOutput{}, fmt.Errorf("could not read the committed transfer: %w", err)
	}
	if receipt.Before == nil || receipt.After == nil {
		return transferReplayOutput{}, errors.New("transfer has no balance snapshots")
	}
	limits := config.RuleCollectionLimits(config.DefaultLimits())
	audit, err := decodeTransferPlanAudit(rawAudit, limits)
	if err != nil {
		return transferReplayOutput{}, err
	}

	source, err := store.LoadPlanArtifact(ctx, "rule", audit.Rule, audit.RuleVersion, audit.ProgramSHA256)
	if err != nil {
		return transferReplayOutput{}, fmt.Errorf("recorded rule is missing or differs from its archived version: %w", err)
	}
	metadata, err := store.LoadPlanArtifact(ctx, "contract", audit.Rule, audit.ContractArtifactVersion, audit.ContractSHA256)
	if err != nil {
		return transferReplayOutput{}, fmt.Errorf("recorded contract is missing or differs from its archived version: %w", err)
	}
	route, err := prepareServedRule("archived-transfer.fin", source.Data, metadata.Data, limits)
	if err != nil {
		return transferReplayOutput{}, errors.New("could not load the archived transfer rule and contract")
	}
	if route.spec.Rule != audit.Rule || route.spec.SchemaVersion != audit.ContractVersion ||
		route.programHash != audit.ProgramSHA256 || route.contractHash != audit.ContractSHA256 ||
		validateTransferRoute(route) != nil {
		return transferReplayOutput{}, errors.New("archived rule and contract do not match the audit record")
	}
	route.ruleVersion, route.contractArtifactVersion = source.Version, metadata.Version
	args, err := contract.DecodeInput(bytes.NewReader(audit.Input), route.spec, route.limits)
	if err != nil {
		return transferReplayOutput{}, errors.New("recorded input does not satisfy the archived contract")
	}
	args, err = ruleArguments(route.rule, route.spec, args)
	if err != nil {
		return transferReplayOutput{}, errors.New("recorded input does not match the archived rule")
	}
	req, err := transferRequestFromRecord(args, route.limits)
	result := receipt.Result
	if err != nil || req != (transfer.Request{
		ID: result.ID, From: result.From, To: result.To, Currency: result.Currency, Amount: result.Amount,
	}) {
		return transferReplayOutput{}, errors.New("recorded input differs from the committed transfer")
	}
	plan, err := buildTransferPlan(route, args, audit.Input, req.Currency)
	if err != nil {
		return transferReplayOutput{}, errors.New("archived rule could not reproduce the transfer plan")
	}
	if plan.Debit != result.Debit || plan.Credit != result.Amount || plan.Fee != result.Fee {
		return transferReplayOutput{}, errors.New("replayed plan differs from the committed ledger")
	}
	var computed transferPlanAudit
	if err := json.Unmarshal(plan.Audit, &computed); err != nil ||
		verifyAuditReplayResult(audit.Result, computed.Result) != nil {
		return transferReplayOutput{}, errors.New("replayed plan differs from the audit record")
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(computed.Result, &envelope); err != nil || !isAuditObject(envelope.Result) {
		return transferReplayOutput{}, errors.New("invalid replayed result")
	}
	return transferReplayOutput{
		Status: "ok", RequestID: result.ID, Rule: audit.Rule,
		RuleVersion: source.Version, ContractArtifactVersion: metadata.Version, Result: envelope.Result,
	}, nil
}

func decodeTransferPlanAudit(data []byte, limits config.Limits) (transferPlanAudit, error) {
	var audit transferPlanAudit
	// Stage 3 audit JSON could expand its embedded input by escaping HTML.
	if len(data) > 6*limits.MaxSourceBytes+(64<<10) ||
		jsonv2.Unmarshal(data, &audit, jsonv2.RejectUnknownMembers(true)) != nil ||
		audit.Record != "transfer_plan" || audit.Rule == "" || audit.ContractVersion != "3" ||
		audit.RunnerVersion != "fin "+Version || audit.ExecutionProfile != ruleCollectionProfile || audit.Limits != limits ||
		audit.ErrorCode != "" || audit.FXPolicy != nil || len(audit.FXQuotes) != 0 ||
		!validSHA256(audit.ProgramSHA256) || !validSHA256(audit.ContractSHA256) ||
		!isAuditObject(audit.Input) || !isAuditObject(audit.Result) {
		return transferPlanAudit{}, errors.New("invalid or unsupported transfer audit")
	}
	switch audit.SchemaVersion {
	case "1":
		if audit.RuleVersion != "" || audit.ContractArtifactVersion != "" {
			return transferPlanAudit{}, errors.New("unexpected versions in stage 3 transfer audit")
		}
	case "2":
		if audit.RuleVersion == "" || audit.ContractArtifactVersion == "" {
			return transferPlanAudit{}, errors.New("transfer audit is missing recorded versions")
		}
	default:
		return transferPlanAudit{}, errors.New("unsupported transfer audit version")
	}
	return audit, nil
}
