package cli

import (
	"bytes"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/contract"
	"github.com/Pirite08/finlang/internal/decimal"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/eval"
	"github.com/Pirite08/finlang/internal/trace"
	"github.com/Pirite08/finlang/internal/transfer"
)

const transferCurrency = "KRW"

type transferPlanAudit struct {
	Record                  string `json:"record"`
	RuleVersion             string `json:"rule_version,omitempty"`
	ContractArtifactVersion string `json:"contract_artifact_version,omitempty"`
	trace.Header
}

func validateTransferRoute(route servedRule) error {
	input := map[string]string{"request_id": "string", "from_account": "string", "to_account": "string", "amount": "money"}
	output := map[string]string{"debit": "money", "credit": "money", "fee": "money"}
	if !route.spec.MatchesRecordShape("transfer", input, output, transferCurrency, 0) {
		return diagnostic.New("E_SERVE_CONFIG", "transfer action requires schema 3 with a transfer Record and debit, credit, fee Money output fields in whole KRW")
	}
	return nil
}

func servePlannedTransfer(w http.ResponseWriter, r *http.Request, route servedRule, store *transfer.Store, body []byte, args []eval.Value) {
	req, err := transferRequestFromRecord(args, route.limits)
	if err != nil {
		writeServeError(w, http.StatusBadRequest, route.spec, diagnostic.CodeOf(err), "request does not match the transfer input contract")
		return
	}
	receipt, err := store.TransferPlanned(r.Context(), req, func() (transfer.Plan, error) {
		return buildTransferPlan(route, args, body, req.Currency)
	})
	if err != nil {
		writeTransferError(w, route.spec, err)
		return
	}
	writeTransferReceipt(w, route.spec, receipt)
}

// A tightened input contract must not hide a receipt for a committed request.
// This path can only read an existing receipt and checks the original contract.
func serveHistoricalTransferRetry(w http.ResponseWriter, r *http.Request, route servedRule, store *transfer.Store, body []byte) bool {
	var envelope struct {
		SchemaVersion string `json:"schema_version"`
		Rule          string `json:"rule"`
		Inputs        struct {
			Transfer struct {
				RequestID string          `json:"request_id"`
				From      string          `json:"from_account"`
				To        string          `json:"to_account"`
				Amount    json.RawMessage `json:"amount"`
			} `json:"transfer"`
		} `json:"inputs"`
	}
	if err := jsonv2.Unmarshal(body, &envelope, jsonv2.RejectUnknownMembers(true)); err != nil ||
		envelope.SchemaVersion != route.spec.SchemaVersion || envelope.Rule != route.spec.Rule {
		return false
	}
	ctx := r.Context()
	receipt, rawAudit, err := store.ReadPlannedTransfer(ctx, envelope.Inputs.Transfer.RequestID)
	if err != nil || receipt.Before == nil || receipt.After == nil {
		return false
	}
	audit, err := decodeTransferPlanAudit(rawAudit, route.limits)
	if err != nil || audit.Rule != envelope.Rule {
		return false
	}
	metadata, err := store.LoadPlanArtifact(ctx, "contract", audit.Rule, audit.ContractArtifactVersion, audit.ContractSHA256)
	if err != nil {
		return false
	}
	spec, err := contract.Load(bytes.NewReader(metadata.Data), route.limits)
	if err != nil || spec.Rule != audit.Rule || validateTransferRoute(servedRule{spec: spec}) != nil {
		return false
	}
	args, err := contract.DecodeInput(bytes.NewReader(body), spec, route.limits)
	if err != nil {
		return false
	}
	req, err := transferRequestFromRecord(args, route.limits)
	if err != nil {
		return false
	}
	result := receipt.Result
	if req != (transfer.Request{ID: result.ID, From: result.From, To: result.To, Currency: result.Currency, Amount: result.Amount}) {
		writeTransferError(w, route.spec, transfer.ErrConflict)
		return true
	}
	writeTransferReceipt(w, route.spec, receipt)
	return true
}

func writeTransferReceipt(w http.ResponseWriter, spec contract.Spec, receipt transfer.Receipt) {
	response := struct {
		SchemaVersion string                `json:"schema_version"`
		Rule          string                `json:"rule"`
		Status        string                `json:"status"`
		Result        servedTransferReceipt `json:"result"`
	}{
		SchemaVersion: spec.SchemaVersion,
		Rule:          spec.Rule,
		Status:        "ok",
		Result: servedTransferReceipt{
			RequestID: receipt.Result.ID, Currency: receipt.Result.Currency,
			Before: receipt.Before, After: receipt.After, Postings: receipt.Postings,
		},
	}
	_ = json.NewEncoder(w).Encode(response)
}

type servedTransferReceipt struct {
	RequestID string             `json:"request_id"`
	Currency  string             `json:"currency"`
	Before    *transfer.Balances `json:"before"`
	After     *transfer.Balances `json:"after"`
	Postings  []transfer.Posting `json:"postings"`
}

func writeTransferError(w http.ResponseWriter, spec contract.Spec, err error) {
	switch {
	case errors.Is(err, transfer.ErrInvalid):
		writeServeError(w, http.StatusBadRequest, spec, "E_INVALID_TRANSFER", "transfer request or resulting balance is out of range")
	case errors.Is(err, transfer.ErrCurrency):
		writeServeError(w, http.StatusBadRequest, spec, "E_CURRENCY_MISMATCH", "transfer and accounts must share a currency")
	case errors.Is(err, transfer.ErrNotFound):
		writeServeError(w, http.StatusNotFound, spec, "E_ACCOUNT_NOT_FOUND", "account not found")
	case errors.Is(err, transfer.ErrInsufficientFunds):
		writeServeError(w, http.StatusConflict, spec, "E_INSUFFICIENT_FUNDS", "insufficient funds for amount and fee")
	case errors.Is(err, transfer.ErrConflict):
		writeServeError(w, http.StatusConflict, spec, "E_REQUEST_CONFLICT", "request ID is already used for another transfer")
	case errors.Is(err, transfer.ErrPlan):
		writeServeError(w, http.StatusInternalServerError, spec, "E_TRANSFER_PLAN", "rule did not produce a valid transfer plan")
	default:
		writeServeError(w, http.StatusInternalServerError, spec, "E_INTERNAL", "transfer could not be committed")
	}
}

func buildTransferPlan(route servedRule, args []eval.Value, body []byte, currency string) (transfer.Plan, error) {
	evaluator := eval.New(eval.Options{Limits: route.limits, JSONMode: true, RuleOnly: true, CollectionRules: true})
	result, err := evaluator.RunRule(route.rule, args...)
	if err != nil {
		return transfer.Plan{}, fmt.Errorf("%w: evaluate rule: %v", transfer.ErrPlan, err)
	}
	var output bytes.Buffer
	if err := contract.EncodeResult(&output, route.spec, result); err != nil {
		return transfer.Plan{}, fmt.Errorf("%w: validate output: %v", transfer.ErrPlan, err)
	}
	plan, err := transferPlanFromRecord(result, currency, route.limits)
	if err != nil {
		return transfer.Plan{}, err
	}
	plan.Audit, err = json.Marshal(transferPlanAudit{
		Record: "transfer_plan", RuleVersion: route.ruleVersion, ContractArtifactVersion: route.contractArtifactVersion,
		Header: trace.Header{
			SchemaVersion: "2", ProgramSHA256: route.programHash, ContractSHA256: route.contractHash,
			ContractVersion: route.spec.SchemaVersion, Rule: route.spec.Rule, RunnerVersion: "fin " + Version,
			ExecutionProfile: ruleCollectionProfile, Limits: route.limits,
			Input: json.RawMessage(body), Result: json.RawMessage(output.Bytes()),
		},
	})
	return plan, err
}

func transferRequestFromRecord(args []eval.Value, limits config.Limits) (transfer.Request, error) {
	badShape := diagnostic.New("E_CONTRACT_INPUT", "expected a transfer Record with ID, accounts and a positive whole KRW amount")
	if len(args) != 1 {
		return transfer.Request{}, badShape
	}
	fields, ok := exactRecordFields(args[0], 4)
	if !ok {
		return transfer.Request{}, badShape
	}
	id, _ := fields["request_id"].(eval.StringValue)
	from, _ := fields["from_account"].(eval.StringValue)
	to, _ := fields["to_account"].(eval.StringValue)
	amount, ok := wholeMoney(fields["amount"], transferCurrency, limits)
	if id == "" || from == "" || to == "" || !ok || amount <= 0 {
		return transfer.Request{}, badShape
	}
	return transfer.Request{ID: string(id), From: string(from), To: string(to), Currency: transferCurrency, Amount: amount}, nil
}

func transferPlanFromRecord(value eval.Value, currency string, limits config.Limits) (transfer.Plan, error) {
	fields, ok := exactRecordFields(value, 3)
	if !ok {
		return transfer.Plan{}, transfer.ErrPlan
	}
	debit, debitOK := wholeMoney(fields["debit"], currency, limits)
	credit, creditOK := wholeMoney(fields["credit"], currency, limits)
	fee, feeOK := wholeMoney(fields["fee"], currency, limits)
	if !debitOK || !creditOK || !feeOK {
		return transfer.Plan{}, transfer.ErrPlan
	}
	return transfer.Plan{Debit: debit, Credit: credit, Fee: fee}, nil
}

func exactRecordFields(value eval.Value, length int) (map[string]eval.Value, bool) {
	record, ok := eval.RecordFields(value)
	if !ok || len(record) != length {
		return nil, false
	}
	fields := make(map[string]eval.Value, len(record))
	for _, field := range record {
		fields[field.Name] = field.Value
	}
	return fields, len(fields) == length
}

func wholeMoney(value eval.Value, currency string, limits config.Limits) (int64, bool) {
	amount, ok := value.(eval.MoneyValue)
	if !ok || amount.Value.Currency() != currency {
		return 0, false
	}
	text, err := decimal.FormatAt(amount.Value.Amount(), 0, limits)
	if err != nil {
		return 0, false
	}
	minor, err := strconv.ParseInt(text, 10, 64)
	return minor, err == nil
}
