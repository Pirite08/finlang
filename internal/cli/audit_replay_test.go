package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/fx"
)

func TestLoadAuditReplayUsesBoundedStrictHeader(t *testing.T) {
	limits := config.DefaultLimits()
	limits.MaxSourceBytes = 512
	hash := strings.Repeat("a", 64)
	header, err := json.Marshal(map[string]any{
		"record": "header", "schema_version": "1", "run_id": "audit-test-run",
		"rule": "sample", "program_sha256": hash, "contract_sha256": hash, "contract_version": "1",
		"runner_version": "fin " + Version, "limits": limits,
		"fx_policy": map[string]any{"version": fx.PolicyVersion, "mode": "none"},
		"input":     json.RawMessage(`{"rule":"sample","inputs":{}}`),
		"result":    json.RawMessage(`{"status":"ok","result":{"currency":"USD","amount":"1.00"}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	line := string(header)
	for _, tc := range []struct {
		name, text string
		want       diagnostic.Code
	}{
		{"first line only", line + "\nnot read", ""},
		{"duplicate", `{"run_id":"other",` + line[1:] + "\n", "E_AUDIT_REPLAY"},
		{"case variant", strings.Replace(line, `"run_id"`, `"Run_ID"`, 1) + "\n", "E_AUDIT_REPLAY"},
		{"invalid hash", strings.Replace(line, hash, strings.Repeat("z", 64), 1) + "\n", "E_AUDIT_REPLAY"},
		{"wrong limits", strings.Replace(line, `"max_source_bytes":512`, `"max_source_bytes":1024`, 1) + "\n", "E_AUDIT_REPLAY"},
		{"failed execution", strings.Replace(line, `"status":"ok"`, `"status":"error"`, 1) + "\n", "E_AUDIT_REPLAY"},
		{"ambiguous result", strings.Replace(line, `"status":"ok"`, `"status":"error","status":"ok"`, 1) + "\n", "E_AUDIT_REPLAY"},
		{"oversize", strings.Repeat(" ", 72<<10) + line + "\n", "E_AUDIT_REPLAY"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fixture.jsonl")
			if err := os.WriteFile(path, []byte(tc.text), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := loadAuditReplay(path, limits)
			if diagnostic.CodeOf(err) != tc.want {
				t.Fatalf("code = %s, want %s", diagnostic.CodeOf(err), tc.want)
			}
			if err == nil && got.RunID != "audit-test-run" {
				t.Fatal("wrong audit header")
			}
		})
	}
}

func TestVerifyAuditReplayResultPreservesAmountScale(t *testing.T) {
	want := json.RawMessage(`{"result":{"amount":"1.00"}}`)
	if err := verifyAuditReplayResult(want, []byte(" {\n\"result\": {\"amount\": \"1.00\"}}\n")); err != nil {
		t.Fatal(err)
	}
	if err := verifyAuditReplayResult(want, []byte(`{"result":{"amount":"1.0"}}`)); diagnostic.CodeOf(err) != "E_AUDIT_REPLAY" {
		t.Fatalf("changed amount scale was accepted: %v", err)
	}
}

func TestVerifyCollectionAuditReplayResult(t *testing.T) {
	want := json.RawMessage(`{"schema_version":"2","rule":"group_transactions","status":"ok","result":{"KRW":{"currency":"KRW","amount":"3500.00"},"USD":{"currency":"USD","amount":"20.00"}}}`)
	if err := verifyAuditReplayResult(want, want); err != nil {
		t.Fatal(err)
	}
	for _, change := range []struct{ from, to string }{
		{`"20.00"`, `"20.0"`},
		{`"20.00"`, `"20.01"`},
		{`"currency":"USD"`, `"currency":"KRW"`},
		{`,"USD":{"currency":"USD","amount":"20.00"}`, ""},
	} {
		actual := []byte(strings.Replace(string(want), change.from, change.to, 1))
		if err := verifyAuditReplayResult(want, actual); diagnostic.CodeOf(err) != "E_AUDIT_REPLAY" {
			t.Fatalf("changed collection result was accepted: %s", actual)
		}
	}
}

func TestLoadCollectionAuditReplay(t *testing.T) {
	limits := config.DefaultLimits()
	limits.MaxSourceBytes = 512
	limitJSON, err := json.Marshal(limits)
	if err != nil {
		t.Fatal(err)
	}
	var collectionLimits map[string]any
	if err := json.Unmarshal(limitJSON, &collectionLimits); err != nil {
		t.Fatal(err)
	}
	collectionLimits["max_collection_items"] = 1024
	hash := strings.Repeat("a", 64)
	header, err := json.Marshal(map[string]any{
		"record": "header", "schema_version": "1", "run_id": "collection-audit-run",
		"rule": "group_transactions", "program_sha256": hash, "contract_sha256": hash, "contract_version": "2",
		"runner_version": "fin " + Version, "limits": collectionLimits, "execution_profile": "rule-only-v2",
		"fx_policy": map[string]any{"version": fx.PolicyVersion, "mode": "none"},
		"input":     json.RawMessage(`{"schema_version":"2","rule":"group_transactions","inputs":{"transactions":[{"amount":{"currency":"KRW","amount":"3500.00"}}]}}`),
		"result":    json.RawMessage(`{"schema_version":"2","rule":"group_transactions","status":"ok","result":{"KRW":{"currency":"KRW","amount":"3500.00"},"USD":{"currency":"USD","amount":"20.00"}}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, from, to string
		want           diagnostic.Code
	}{
		{name: "successful collection audit"},
		{"contract profile mismatch", `"contract_version":"2"`, `"contract_version":"1"`, "E_AUDIT_REPLAY"},
		{"downgraded profile", `"rule-only-v2"`, `"rule-only-v1"`, "E_AUDIT_REPLAY"},
		{"omitted profile", `"execution_profile":"rule-only-v2",`, "", "E_AUDIT_REPLAY"},
		{"wrong cap", `"max_collection_items":1024`, `"max_collection_items":1025`, "E_AUDIT_REPLAY"},
		{"omitted cap", `"max_collection_items":1024,`, "", "E_AUDIT_REPLAY"},
		{"wrong execution budget", `"max_eval_steps":100000`, `"max_eval_steps":100001`, "E_AUDIT_REPLAY"},
		{"duplicate transaction amount", `"amount":{"currency":"KRW","amount":"3500.00"}`, `"amount":{"currency":"KRW","amount":"3500.00","\u0061mount":"3500.00"}`, "E_AUDIT_REPLAY"},
		{"oversized embedded input", `"transactions":[`, `"transactions":[` + strings.Repeat(" ", 512), "E_AUDIT_REPLAY"},
		{"wrong output schema", `"schema_version":"2","rule":"group_transactions","status":"ok"`, `"schema_version":"1","rule":"group_transactions","status":"ok"`, "E_AUDIT_REPLAY"},
		{"wrong result rule", `"rule":"group_transactions","status":"ok"`, `"rule":"other","status":"ok"`, "E_AUDIT_REPLAY"},
		{"duplicate currency key", `"result":{"KRW":`, `"result":{"KRW":{"currency":"KRW","amount":"1.00"},"KRW":`, "E_AUDIT_REPLAY"},
		{"duplicate nested amount", `"amount":"20.00"`, `"amount":"20.00","amount":"20.00"`, "E_AUDIT_REPLAY"},
		{"key currency mismatch", `"USD":{"currency":"USD"`, `"USD":{"currency":"KRW"`, "E_AUDIT_REPLAY"},
		{"unsupported currency", `"USD":{"currency":"USD"`, `"EUR":{"currency":"EUR"`, "E_AUDIT_REPLAY"},
		{"numeric amount", `"amount":"20.00"`, `"amount":20.00`, "E_AUDIT_REPLAY"},
		{"noncanonical amount", `"amount":"20.00"`, `"amount":"020.00"`, "E_AUDIT_REPLAY"},
		{"extra output field", `"amount":"20.00"`, `"amount":"20.00","extra":true`, "E_AUDIT_REPLAY"},
		{"live FX mode", `"mode":"none"`, `"mode":"http"`, "E_AUDIT_REPLAY"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			line := string(header)
			if tc.from != "" {
				if !strings.Contains(line, tc.from) {
					t.Fatalf("missing fixture fragment %q", tc.from)
				}
				line = strings.Replace(line, tc.from, tc.to, 1)
			}
			path := filepath.Join(t.TempDir(), "collection.jsonl")
			if err := os.WriteFile(path, []byte(line+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := loadAuditReplay(path, limits)
			if diagnostic.CodeOf(err) != tc.want {
				t.Fatalf("code = %s, want %s", diagnostic.CodeOf(err), tc.want)
			}
			if err == nil && got.ExecutionProfile != "rule-only-v2" {
				t.Fatalf("unexpected replay profile %q", got.ExecutionProfile)
			}
		})
	}
}
