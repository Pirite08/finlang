package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	collectionRuleSource = "../../examples/group_transactions_rule_v2.fin"
	collectionRuleSpec   = "../../contracts/group_transactions_v2.json"
	collectionRuleInput  = "../../testdata/inputs/group_transactions_v2.json"
)

func TestCollectionRuleAuditRoundTrip(t *testing.T) {
	dir := t.TempDir()
	audit := filepath.Join(dir, "group.audit.jsonl")
	operations := filepath.Join(dir, "operations.jsonl")
	base := []string{"-file", collectionRuleSource, "-contract", collectionRuleSpec, "-json"}
	var out, stderr bytes.Buffer
	args := append(base, "-input", collectionRuleInput, "-rule-only", "-audit", audit, "-ops-log", operations)
	if code := Run(args, strings.NewReader(""), &out, &stderr); code != 0 {
		t.Fatalf("collection rule: exit=%d, stdout=%s, stderr=%s", code, &out, &stderr)
	}
	want := "{\"schema_version\":\"2\",\"rule\":\"group_transactions_v2\",\"status\":\"ok\",\"result\":{\"KRW\":{\"currency\":\"KRW\",\"amount\":\"3500.00\"},\"USD\":{\"currency\":\"USD\",\"amount\":\"20.00\"}}}\n"
	if out.String() != want {
		t.Fatalf("result=%s, want=%s", out.String(), want)
	}
	data, err := os.ReadFile(audit)
	if err != nil {
		t.Fatal(err)
	}
	headerLine, events, _ := bytes.Cut(data, []byte{'\n'})
	header := decodeRecord(t, string(headerLine))
	limits := header["limits"].(map[string]any)
	if header["execution_profile"] != "rule-only-v2" || header["contract_version"] != "2" || limits["max_collection_items"] != float64(1024) || limits["max_eval_steps"] != float64(100000) {
		t.Fatalf("wrong audit metadata: %v", header)
	}
	if !bytes.Contains(headerLine, []byte(`"id":"T001"`)) || !bytes.Contains(headerLine, []byte(`"amount":"3500.00"`)) {
		t.Fatalf("audit omitted input or result: %s", headerLine)
	}
	if !bytes.Contains(events, []byte(`"step":"initialize"`)) || !bytes.Contains(events, []byte(`"step":"aggregate"`)) {
		t.Fatalf("audit omitted step events: %s", events)
	}
	ops, err := os.ReadFile(operations)
	if err != nil {
		t.Fatal(err)
	}
	for _, sensitive := range []string{"T001", "3500.00", "1000.25", "transactions\""} {
		if bytes.Contains(ops, []byte(sensitive)) {
			t.Fatalf("operations log contains %q: %s", sensitive, ops)
		}
	}
	out.Reset()
	stderr.Reset()
	if code := Run(append(base, "-replay-audit", audit), strings.NewReader(""), &out, &stderr); code != 0 || out.String() != want {
		t.Fatalf("replay: exit=%d, stdout=%s, stderr=%s", code, &out, &stderr)
	}
}

func TestCollectionContractRequiresRestrictedRule(t *testing.T) {
	var out, stderr bytes.Buffer
	args := []string{"-file", collectionRuleSource, "-contract", collectionRuleSpec, "-json", "-input", collectionRuleInput}
	if code := Run(args, strings.NewReader(""), &out, &stderr); code != 1 {
		t.Fatalf("unrestricted collection contract: exit=%d, stdout=%s, stderr=%s", code, &out, &stderr)
	}
	record := decodeRecord(t, out.String())
	if record["schema_version"] != "2" || record["error"].(map[string]any)["code"] != "E_RULE_POLICY" {
		t.Fatalf("unexpected contract policy error: %v", record)
	}
}

func TestCollectionRuleTransactionCountBoundary(t *testing.T) {
	const transaction = `{"id":"T001","amount":{"currency":"KRW","amount":"1.00"}}`
	input := filepath.Join(t.TempDir(), "input.json")
	args := []string{"-file", collectionRuleSource, "-contract", collectionRuleSpec, "-json", "-rule-only", "-input", input}
	for _, tc := range []struct {
		name string
		size int
		code int
		want string
	}{
		{"empty", 0, 0, `"result":{}`},
		{"at limit", 128, 0, `"result":{"KRW":{"currency":"KRW","amount":"128.00"}}`},
		{"over limit", 129, 1, `"code":"E_CONTRACT_INPUT"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			items := make([]string, tc.size)
			for i := range items {
				items[i] = transaction
			}
			payload := `{"schema_version":"2","rule":"group_transactions_v2","inputs":{"transactions":[` + strings.Join(items, ",") + `]}}`
			if err := os.WriteFile(input, []byte(payload), 0600); err != nil {
				t.Fatal(err)
			}
			var out, stderr bytes.Buffer
			if code := Run(args, strings.NewReader(""), &out, &stderr); code != tc.code || !strings.Contains(out.String(), tc.want) {
				t.Fatalf("exit=%d, stdout=%s, stderr=%s, want exit=%d and %s", code, &out, &stderr, tc.code, tc.want)
			}
		})
	}
}
