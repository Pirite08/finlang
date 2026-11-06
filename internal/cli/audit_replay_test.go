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
