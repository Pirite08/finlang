package compat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Pirite08/finlang/internal/ast"
	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/contract"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/eval"
	"github.com/Pirite08/finlang/internal/parser"
	"github.com/Pirite08/finlang/internal/trace"
)

type ruleRuntime struct {
	spec         contract.Spec
	program      *ast.Program
	source       []byte
	contractData []byte
	root         string
}

func loadRule(t *testing.T, name string) ruleRuntime {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	read := func(path string) []byte {
		data, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	limits := config.DefaultLimits()
	r := ruleRuntime{root: root}
	r.contractData = read("contracts/" + name + ".json")
	r.spec, err = contract.Load(bytes.NewReader(r.contractData), limits)
	if err != nil {
		t.Fatal(err)
	}
	r.source = read("examples/" + name + ".fin")
	r.program, err = parser.Parse(name+".fin", string(r.source), limits)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// DecodeInput precedes all execution. Callers treat rejection here as a harness
// error; rejected-table cases never reach this function or the COBOL process.
func (r ruleRuntime) execute(input json.RawMessage) (outcome, error) {
	limits := config.DefaultLimits()
	args, err := contract.DecodeInput(bytes.NewReader(input), r.spec, limits)
	if err != nil {
		return outcome{}, fmt.Errorf("accepted case rejected by contract: %w", err)
	}
	recorder := trace.NewRecorder(limits.MaxTraceEvents)
	evaluator := eval.New(eval.Options{Limits: limits, JSONMode: true, Trace: recorder})
	env := eval.NewEnvironment(nil)
	_, runErr := evaluator.Run(r.program, env)
	var encoded bytes.Buffer
	if runErr == nil {
		entry, ok := env.Lookup(r.spec.Entry)
		if !ok {
			return outcome{}, fmt.Errorf("missing entry %s", r.spec.Entry)
		}
		value, err := evaluator.Call(entry, args, diagnostic.Span{File: r.spec.Rule + ".fin", Line: 1, Column: 1})
		runErr = err
		if runErr == nil {
			runErr = contract.EncodeResult(&encoded, r.spec, value)
		}
	}
	if recorder.Truncated() {
		return outcome{}, fmt.Errorf("branch evidence was truncated")
	}
	branch := "none"
	branchEvents := 0
	for _, event := range recorder.Events() {
		if event.Kind != "branch" {
			continue
		}
		branchEvents++
		switch event.Branch {
		case "true":
			branch = "low"
		case "false":
			branch = "high"
		default:
			return outcome{}, fmt.Errorf("unknown actual trace branch %q", event.Branch)
		}
	}
	if (r.spec.Rule == "fee_v1" && branchEvents != 1) || (r.spec.Rule != "fee_v1" && branchEvents != 0) {
		return outcome{}, fmt.Errorf("unexpected branch event count %d for %s", branchEvents, r.spec.Rule)
	}
	if runErr != nil {
		if encoded.Len() != 0 {
			return outcome{}, fmt.Errorf("error produced success bytes %q", encoded.String())
		}
		return outcome{Status: "error", Currency: r.spec.Output.Currency, ErrorClass: string(diagnostic.CodeOf(runErr)), Branch: branch}, nil
	}
	var result struct {
		SchemaVersion string `json:"schema_version"`
		Rule          string `json:"rule"`
		Status        string `json:"status"`
		Result        struct {
			Currency string `json:"currency"`
			Amount   string `json:"amount"`
		} `json:"result"`
	}
	decoder := json.NewDecoder(&encoded)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return outcome{}, err
	}
	if result.SchemaVersion != "1" || result.Rule != r.spec.Rule || result.Status != "ok" {
		return outcome{}, fmt.Errorf("invalid encoded result metadata: %+v", result)
	}
	return outcome{Status: result.Status, Currency: result.Result.Currency, Amount: result.Result.Amount, Branch: branch}, nil
}

func TestExecutedFeeBranchUsesTrace(t *testing.T) {
	rule := loadRule(t, "fee_v1")
	// The amounts round to the same value, so amount comparison cannot stand in
	// for observing which source branch actually executed.
	for _, tc := range []struct{ amount, branch string }{{"999999.99", "low"}, {"1000000", "low"}, {"1000000.01", "high"}} {
		got, err := rule.execute(makeInput(rule.spec, []string{tc.amount}))
		if err != nil {
			t.Fatal(err)
		}
		if got.Amount != "1000" || got.Branch != tc.branch {
			t.Fatalf("amount=%s got %+v", tc.amount, got)
		}
	}
}
