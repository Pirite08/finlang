// Package transferfee runs the demo transfer fee as a restricted finlang rule.
package transferfee

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"fmt"
	"strconv"
	"strings"

	"github.com/Pirite08/finlang/internal/ast"
	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/contract"
	"github.com/Pirite08/finlang/internal/decimal"
	"github.com/Pirite08/finlang/internal/eval"
	"github.com/Pirite08/finlang/internal/fx"
	"github.com/Pirite08/finlang/internal/parser"
	"github.com/Pirite08/finlang/internal/trace"
)

const (
	runnerVersion    = "fin-transfer-fee-v1"
	executionProfile = "rule-only-v1"
	maxAuditBytes    = 64 << 10
)

//go:embed rules/*.fin rules/*.json
var rules embed.FS

type policy struct {
	rule         *ast.Rule
	spec         contract.Spec
	limits       config.Limits
	programHash  string
	contractHash string
}

type auditRecord struct {
	trace.Header
	Events []trace.Event `json:"events"`
}

type feeMoney struct {
	Currency string `json:"currency"`
	Amount   string `json:"amount"`
}

// Calculate returns a fee in minor units and the reproducible calculation record.
func Calculate(currency string, amount int64) (fee int64, audit []byte, err error) {
	p, err := loadPolicy(currency)
	if err != nil {
		return 0, nil, err
	}
	input, err := p.input(amount)
	if err != nil {
		return 0, nil, err
	}
	return p.calculate(input)
}

// Replay recalculates an archived fee against the expected ledger input.
// The archive is canonical JSON from Calculate and is checked byte for byte.
func Replay(audit []byte, currency string, amount int64) (int64, error) {
	if len(audit) == 0 || len(audit) > maxAuditBytes {
		return 0, fmt.Errorf("invalid fee audit size")
	}
	p, err := loadPolicy(currency)
	if err != nil {
		return 0, err
	}
	expectedInput, err := p.input(amount)
	if err != nil {
		return 0, err
	}
	var stored auditRecord
	if err := jsonv2.Unmarshal(audit, &stored, jsonv2.RejectUnknownMembers(true)); err != nil {
		return 0, fmt.Errorf("decode fee audit: %w", err)
	}
	if stored.ProgramSHA256 != p.programHash || stored.ContractSHA256 != p.contractHash ||
		stored.RunnerVersion != runnerVersion || stored.ExecutionProfile != executionProfile {
		return 0, fmt.Errorf("fee audit rule, contract or runner version mismatch")
	}
	if !bytes.Equal(stored.Input, expectedInput) {
		return 0, fmt.Errorf("fee audit input differs from the recorded transfer")
	}
	fee, recalculated, err := p.calculate(stored.Input)
	if err != nil {
		return 0, err
	}
	if !bytes.Equal(audit, recalculated) {
		return 0, fmt.Errorf("fee audit differs from the reproduced calculation")
	}
	return fee, nil
}

func loadPolicy(currency string) (policy, error) {
	if currency != "KRW" && currency != "USD" {
		return policy{}, fmt.Errorf("unsupported fee currency %q", currency)
	}
	path := "rules/" + strings.ToLower(currency)
	source, err := rules.ReadFile(path + ".fin")
	if err != nil {
		return policy{}, err
	}
	metadata, err := rules.ReadFile(path + ".json")
	if err != nil {
		return policy{}, err
	}
	limits := config.DefaultLimits()
	spec, err := contract.Load(bytes.NewReader(metadata), limits)
	if err != nil {
		return policy{}, err
	}
	program, err := parser.Parse(path+".fin", string(source), limits)
	if err != nil {
		return policy{}, err
	}
	rule := program.Rule
	var scale uint32
	if currency == "USD" {
		scale = 2
	}
	if rule == nil || spec.SchemaVersion != "1" || spec.Rule != rule.Name || spec.Entry != rule.Name ||
		len(rule.Inputs) != 1 || len(spec.Parameters) != 1 || spec.Output.Currency != currency ||
		rule.Output.Currency != currency || spec.Output.Scale != scale {
		return policy{}, fmt.Errorf("embedded fee rule does not match its contract")
	}
	decl, param := rule.Inputs[0], spec.Parameters[0]
	if decl.Name != "amount" || param.Name != decl.Name || decl.Type != "money" || param.Type != decl.Type ||
		decl.Currency != currency || param.Currency != currency || param.ScaleMax != scale {
		return policy{}, fmt.Errorf("embedded fee input does not match its contract")
	}
	return policy{rule: rule, spec: spec, limits: limits, programHash: hash(source), contractHash: hash(metadata)}, nil
}

func (p policy) input(amount int64) ([]byte, error) {
	if amount <= 0 {
		return nil, fmt.Errorf("fee input must be positive")
	}
	amountText := strconv.FormatInt(amount, 10)
	if p.spec.Output.Currency == "USD" {
		amountText = fmt.Sprintf("%d.%02d", amount/100, amount%100)
	}
	record := struct {
		SchemaVersion string              `json:"schema_version"`
		Rule          string              `json:"rule"`
		Inputs        map[string]feeMoney `json:"inputs"`
	}{
		SchemaVersion: p.spec.SchemaVersion,
		Rule:          p.spec.Rule,
		Inputs:        map[string]feeMoney{"amount": {Currency: p.spec.Output.Currency, Amount: amountText}},
	}
	return json.Marshal(record)
}

func (p policy) calculate(input []byte) (int64, []byte, error) {
	args, err := contract.DecodeInput(bytes.NewReader(input), p.spec, p.limits)
	if err != nil {
		return 0, nil, err
	}
	recorder := trace.NewRecorder(p.limits.MaxTraceEvents)
	evaluator := eval.New(eval.Options{RuleOnly: true, JSONMode: true, Limits: p.limits, Trace: recorder})
	result, err := evaluator.RunRule(p.rule, args...)
	if err != nil {
		return 0, nil, err
	}
	var output bytes.Buffer
	if err := contract.EncodeResult(&output, p.spec, result); err != nil {
		return 0, nil, err
	}
	value, ok := result.(eval.MoneyValue)
	if !ok {
		return 0, nil, fmt.Errorf("fee rule did not return Money")
	}
	text, err := decimal.FormatAt(value.Value.Amount(), p.spec.Output.Scale, p.limits)
	if err != nil {
		return 0, nil, err
	}
	minor, err := strconv.ParseInt(strings.ReplaceAll(text, ".", ""), 10, 64)
	if err != nil {
		return 0, nil, fmt.Errorf("fee result exceeds minor-unit range: %w", err)
	}
	audit := auditRecord{
		Header: trace.Header{
			SchemaVersion: "1", RunnerVersion: runnerVersion,
			Rule: p.spec.Rule, ExecutionProfile: executionProfile, ContractVersion: p.spec.SchemaVersion,
			ProgramSHA256: p.programHash, ContractSHA256: p.contractHash, Limits: p.limits,
			FXPolicy: &fx.PolicySnapshot{Version: fx.PolicyVersion, Mode: "none"},
			Input:    input, Result: output.Bytes(),
		},
		Events: recorder.Events(),
	}
	data, err := json.Marshal(audit)
	if err != nil {
		return 0, nil, err
	}
	if recorder.Truncated() || len(data) > maxAuditBytes {
		return 0, nil, fmt.Errorf("fee audit exceeds recording limits")
	}
	return minor, data, nil
}

func hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
