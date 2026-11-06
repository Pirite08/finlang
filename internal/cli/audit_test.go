package cli

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestOperationalLogAndAuditReplay(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "private-rule-path.fin")
	contractPath := filepath.Join(dir, "private-contract-path.json")
	input := filepath.Join(dir, "private-input-path.json")
	source, err := os.ReadFile("../../examples/usd_to_krw.fin")
	if err != nil {
		t.Fatal(err)
	}
	contractData, err := os.ReadFile("../../contracts/usd_to_krw.json")
	if err != nil {
		t.Fatal(err)
	}
	source = bytes.Replace(source, []byte(`let quote = rate("USD", "KRW")`), []byte(`let quote = rate("USD", "KRW")
        let back = rate("KRW", "USD")
        require convert(money("KRW", 1400), back) > money("USD", 0)`), 1)
	for path, data := range map[string][]byte{
		file: source, contractPath: contractData,
		input: []byte(`{"schema_version":"1","rule":"usd_to_krw","inputs":{"amount":{"currency":"USD","amount":"987.65"}}}`),
	} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests.Add(1)
		base, target, rate := "USD", "KRW", "1350.25"
		if req.URL.Query().Get("base") == "KRW" {
			base, target, rate = "KRW", "USD", "0.00074"
		}
		fmt.Fprintf(w, `{"base":%q,"target":%q,"rate":%q,"source":"private-provider-label","as_of":%q}`, base, target, rate, time.Now().UTC().Format(time.RFC3339Nano))
	}))
	defer server.Close()
	audit := filepath.Join(dir, "private-audit.jsonl")
	ops := filepath.Join(dir, "operations.jsonl")
	base := []string{"-file", file, "-contract", contractPath, "-json"}
	args := append(append([]string{}, base...), "-input", input, "-fx-url", server.URL, "-fx-min-rate", "1300.00", "-fx-max-age", "90s", "-audit", audit, "-ops-log", ops)
	var out, stderr bytes.Buffer
	if code := Run(args, strings.NewReader(""), &out, &stderr); code != 0 {
		t.Fatalf("live exit=%d: %s", code, &stderr)
	}
	want := out.String()
	if result := decodeRecord(t, want)["result"].(map[string]any); result["amount"] != "1333574" || result["currency"] != "KRW" {
		t.Fatalf("unexpected live result: %v", result)
	}
	server.Close()
	auditData, err := os.ReadFile(audit)
	if err != nil {
		t.Fatal(err)
	}
	line, _, _ := bytes.Cut(auditData, []byte{'\n'})
	header := decodeRecord(t, string(line))
	policy, _ := header["fx_policy"].(map[string]any)
	quotes, _ := header["fx_quotes"].([]any)
	if header["rule"] != "usd_to_krw" || header["contract_version"] != "1" || header["program_sha256"] == "" || header["contract_sha256"] == "" ||
		policy["version"] != "fx-policy-v1" || policy["min_rate"] != "1300" || policy["max_age"] != "1m30s" || len(quotes) != 2 {
		t.Fatalf("missing audit inputs or policy: %v", header)
	}
	replay := append(append([]string{}, base...), "-replay-audit", audit, "-ops-log", ops)
	out.Reset()
	stderr.Reset()
	if code := Run(replay, strings.NewReader(""), &out, &stderr); code != 0 || out.String() != want || requests.Load() != 2 {
		t.Fatalf("replay exit=%d: out=%s want=%s stderr=%s requests=%d", code, &out, want, &stderr, requests.Load())
	}
	for _, path := range []string{file, contractPath} {
		original, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(append([]byte{}, original...), '\n'), 0600); err != nil {
			t.Fatal(err)
		}
		out.Reset()
		stderr.Reset()
		if code := Run(replay, strings.NewReader(""), &out, &stderr); code != 1 || !strings.Contains(out.String(), "E_AUDIT_REPLAY") {
			t.Fatalf("modified artifact: exit=%d, out=%s", code, &out)
		}
		if err := os.WriteFile(path, original, 0600); err != nil {
			t.Fatal(err)
		}
	}
	tampered := filepath.Join(dir, "tampered.jsonl")
	if err := os.WriteFile(tampered, bytes.Replace(auditData, []byte(`"rate":"1350.25"`), []byte(`"rate":"1351.25"`), 1), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	stderr.Reset()
	tamperedReplay := append(append([]string{}, base...), "-replay-audit", tampered)
	if code := Run(tamperedReplay, strings.NewReader(""), &out, &stderr); code != 1 || !strings.Contains(out.String(), "E_AUDIT_REPLAY") {
		t.Fatalf("mismatched result: exit=%d, out=%s", code, &out)
	}
	missing := filepath.Join(dir, "private-missing-input.json")
	out.Reset()
	stderr.Reset()
	if code := Run(append(append([]string{}, base...), "-input", missing, "-ops-log", ops), strings.NewReader(""), &out, &stderr); code != 2 || !strings.Contains(stderr.String(), "private-missing-input.json") {
		t.Fatalf("expected private path in caller diagnostic: exit=%d, stderr=%s", code, &stderr)
	}
	logData, err := os.ReadFile(ops)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"987.65", "1333574", "1350.25", "0.00074", "usd_to_krw", "private-", "http://", "sha256", "fx_policy"} {
		if strings.Contains(string(logData), secret) {
			t.Fatalf("operational log contains a sensitive field: %q", secret)
		}
	}
	lines := bytes.Split(bytes.TrimSpace(logData), []byte{'\n'})
	if len(lines) != 5 {
		t.Fatalf("got %d operational records", len(lines))
	}
	first := decodeRecord(t, string(lines[0]))
	replayFailure := decodeRecord(t, string(lines[3]))
	last := decodeRecord(t, string(lines[len(lines)-1]))
	if first["run_id"] == "" || first["run_id"] != header["run_id"] || first["status"] != "ok" || replayFailure["error_code"] != "E_AUDIT_REPLAY" || last["error_code"] != "E_INPUT_IO" {
		t.Fatalf("wrong operational record: first=%v last=%v", first, last)
	}
}

func TestAuditAndOperationsUseSeparateFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	input := filepath.Join("..", "..", "testdata", "inputs", "interest.json")
	args := jsonArgs("interest", input)
	var out, stderr bytes.Buffer
	if code := Run(append(args, "-audit", path, "-ops-log", path), strings.NewReader(""), &out, &stderr); code != 2 {
		t.Fatalf("shared destination: exit=%d, stderr=%s", code, &stderr)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("shared output was created: %v", err)
	}
	if err := os.WriteFile(path, []byte("previous audit"), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	stderr.Reset()
	if code := Run(append(args, "-audit", path), strings.NewReader(""), &out, &stderr); code != 2 || !strings.Contains(out.String(), "E_AUDIT_IO") {
		t.Fatalf("existing audit: exit=%d, out=%s", code, &out)
	}
	out.Reset()
	stderr.Reset()
	if code := Run(append(args, "-ops-log", path), strings.NewReader(""), &out, &stderr); code != 2 || !strings.Contains(out.String(), "E_OPS_LOG_IO") {
		t.Fatalf("audit as operational log: exit=%d, out=%s", code, &out)
	}
	if contents, err := os.ReadFile(path); err != nil || string(contents) != "previous audit" {
		t.Fatalf("existing audit changed: %v", err)
	}
}
