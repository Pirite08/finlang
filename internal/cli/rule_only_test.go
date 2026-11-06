package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuleOnlyAuditsAndReplaysSuppliedQuotes(t *testing.T) {
	dir := t.TempDir()
	source, err := os.ReadFile("../../examples/usd_to_krw.fin")
	if err != nil {
		t.Fatal(err)
	}
	source = bytes.Replace(source, []byte(`let quote = rate("USD", "KRW")`), []byte(`let quote = rate("USD", "KRW")
        let back = rate("KRW", "USD")
        require convert(money("KRW", 1400), back) > money("USD", 0)`), 1)
	file := filepath.Join(dir, "rule.fin")
	snapshot := filepath.Join(dir, "snapshot.json")
	audit := filepath.Join(dir, "audit.jsonl")
	for path, data := range map[string][]byte{
		file: source,
		snapshot: []byte(`[
            {"base":"USD","target":"KRW","rate":"1350.25","source":"approved-fixture","as_of":"2026-01-01T00:00:00Z"},
            {"base":"KRW","target":"USD","rate":"0.00074","source":"approved-fixture","as_of":"2026-01-01T00:00:00Z"}
        ]`),
	} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	args := fxRuleArgs()
	args[1] = file
	var out, stderr bytes.Buffer
	if code := Run(append(args, "-rule-only", "-fx-snapshot", snapshot, "-audit", audit), strings.NewReader(""), &out, &stderr); code != 0 {
		t.Fatalf("rule-only execution: exit=%d, stderr=%s", code, &stderr)
	}
	want := out.String()
	if result := decodeRecord(t, want)["result"].(map[string]any); result["amount"] != "13502" || result["currency"] != "KRW" {
		t.Fatalf("unexpected amount: %v", result)
	}
	auditData, err := os.ReadFile(audit)
	if err != nil {
		t.Fatal(err)
	}
	line, _, _ := bytes.Cut(auditData, []byte{'\n'})
	header := decodeRecord(t, string(line))
	if quotes, ok := header["fx_quotes"].([]any); !ok || len(quotes) != 2 || header["execution_profile"] != "rule-only-v1" {
		t.Fatalf("audit lacks profile or applied quotes: %v", header)
	}
	replayArgs := []string{"-file", file, "-contract", args[3], "-json", "-replay-audit", audit}
	out.Reset()
	stderr.Reset()
	if code := Run(replayArgs, strings.NewReader(""), &out, &stderr); code != 0 || out.String() != want {
		t.Fatalf("audit replay: exit=%d, out=%s, stderr=%s", code, &out, &stderr)
	}
	// Even a matching hash and result do not bypass the recorded execution profile.
	changed := bytes.Replace(source, []byte("step check_amount {"), []byte(`step check_amount {
        if (false) { print("blocked") }`), 1)
	if err := os.WriteFile(file, changed, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(audit, bytes.Replace(auditData, []byte(sha256Hex(source)), []byte(sha256Hex(changed)), 1), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	stderr.Reset()
	if code := Run(replayArgs, strings.NewReader(""), &out, &stderr); code != 1 || !strings.Contains(out.String(), "E_RULE_POLICY") {
		t.Fatalf("audit replay bypassed rule policy: exit=%d, out=%s, stderr=%s", code, &out, &stderr)
	}
}

func TestRuleOnlyRejectsLiveProviderAndLegacyProgram(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("rule-only execution tried a network request")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	var out, stderr bytes.Buffer
	if code := Run(append(fxRuleArgs(), "-rule-only", "-fx-url", server.URL), strings.NewReader(""), &out, &stderr); code != 2 || !strings.Contains(out.String(), "E_CLI_USAGE") {
		t.Fatalf("live source: exit=%d, out=%s, stderr=%s", code, &out, &stderr)
	}
	out.Reset()
	stderr.Reset()
	args := jsonArgs("interest", "../../testdata/inputs/interest.json")
	if code := Run(append(args, "-rule-only"), strings.NewReader(""), &out, &stderr); code != 1 || !strings.Contains(out.String(), "E_RULE_POLICY") {
		t.Fatalf("legacy program: exit=%d, out=%s, stderr=%s", code, &out, &stderr)
	}
}
