package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuleOnlyBasicBuiltinsAuditReplay(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "fee.fin")
	contract := filepath.Join(dir, "contract.json")
	audit := filepath.Join(dir, "audit.jsonl")
	const source = `rule fee_v1 {
        input amount in "KRW"
        output result in "KRW"
        step calculate_fee {
            let raw_fee = money(currency(amount), amount(amount) * decimal("0.003"))
            let bounded_fee = clamp(raw_fee, money("KRW", 100), money("KRW", 3000.5))
            let result = settle(bounded_fee, 0, "HALF_EVEN")
        }
        return result
    }`
	const specification = `{
        "schema_version":"1", "rule":"fee_v1", "entry":"fee_v1",
        "parameters":[{"name":"amount","type":"money","currency":"KRW","min":"0","max":"999999999.99","scale_max":2}],
        "output":{"type":"money","currency":"KRW","scale":0,"min":"0","max":"999999999"}
    }`
	for path, content := range map[string]string{file: source, contract: specification} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	args := []string{"-file", file, "-contract", contract, "-json"}
	var out, stderr bytes.Buffer
	code := Run(append(args, "-input", "../../testdata/inputs/fee.json", "-rule-only", "-audit", audit), strings.NewReader(""), &out, &stderr)
	if code != 0 {
		t.Fatalf("rule-only run: exit=%d, stdout=%s, stderr=%s", code, &out, &stderr)
	}
	resultJSON := out.String()
	if result := decodeRecord(t, resultJSON)["result"].(map[string]any); result["amount"] != "3000" || result["currency"] != "KRW" {
		t.Fatalf("unexpected fee: %v", result)
	}
	auditData, err := os.ReadFile(audit)
	if err != nil {
		t.Fatal(err)
	}
	wantEvents := map[string]string{"amount": "2000000", "currency": "KRW", "decimal": "0.003", "clamp": "KRW 3000.5", "settle": "KRW 3000"}
	for _, line := range bytes.Split(bytes.TrimSpace(auditData), []byte{'\n'}) {
		event := decodeRecord(t, string(line))
		kind, _ := event["kind"].(string)
		if expected, ok := wantEvents[kind]; ok {
			if event["record"] != "event" || event["result"] != expected || event["step"] != "calculate_fee" {
				t.Fatalf("unexpected %s event: %v", kind, event)
			}
			if kind == "settle" && (event["mode"] != "HALF_EVEN" || event["inexact"] != true) {
				t.Fatalf("expected explicit inexact settlement: %v", event)
			}
			delete(wantEvents, kind)
		}
	}
	if len(wantEvents) != 0 {
		t.Fatalf("missing builtin events: %v", wantEvents)
	}
	out.Reset()
	stderr.Reset()
	code = Run(append(args, "-replay-audit", audit), strings.NewReader(""), &out, &stderr)
	if code != 0 || out.String() != resultJSON {
		t.Fatalf("audit replay: exit=%d, stdout=%s, stderr=%s", code, &out, &stderr)
	}
}
