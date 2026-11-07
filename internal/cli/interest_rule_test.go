package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	interestRuleContract = "../../contracts/interest_rule_v1.json"
	interestRuleInput    = "../../testdata/inputs/interest.json"
)

func interestRuleArgs(spec string) []string {
	return []string{"-file", "../../examples/interest_rule_v1.fin", "-contract", spec, "-json"}
}

func TestInterestRuleAuditReplay(t *testing.T) {
	audit := filepath.Join(t.TempDir(), "interest.audit.jsonl")
	var out, stderr bytes.Buffer
	args := append(interestRuleArgs(interestRuleContract), "-input", interestRuleInput, "-rule-only", "-audit", audit)
	if code := Run(args, strings.NewReader(""), &out, &stderr); code != 0 {
		t.Fatalf("rule-only run: exit=%d, stdout=%s, stderr=%s", code, &out, &stderr)
	}
	want := out.String()
	if result := decodeRecord(t, want)["result"].(map[string]any); result["amount"] != "25000" || result["currency"] != "KRW" {
		t.Fatalf("unexpected interest: %v", result)
	}
	data, err := os.ReadFile(audit)
	if err != nil {
		t.Fatal(err)
	}
	line, _, _ := bytes.Cut(data, []byte{'\n'})
	if header := decodeRecord(t, string(line)); header["execution_profile"] != "rule-only-v1" {
		t.Fatalf("audit omitted the rule-only profile: %v", header)
	}
	out.Reset()
	stderr.Reset()
	args = append(interestRuleArgs(interestRuleContract), "-replay-audit", audit)
	if code := Run(args, strings.NewReader(""), &out, &stderr); code != 0 || out.String() != want {
		t.Fatalf("audit replay: exit=%d, stdout=%s, stderr=%s", code, &out, &stderr)
	}
}

func TestInterestRuleBindsInputsByName(t *testing.T) {
	data, err := os.ReadFile(interestRuleContract)
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]json.RawMessage
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	var params []json.RawMessage
	if err := json.Unmarshal(spec["parameters"], &params); err != nil {
		t.Fatal(err)
	}
	reordered, err := json.Marshal([]json.RawMessage{params[2], params[0], params[1]})
	if err != nil {
		t.Fatal(err)
	}
	spec["parameters"] = reordered
	data, err = json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	contractPath := filepath.Join(dir, "reordered.json")
	tracePath := filepath.Join(dir, "reordered.trace.jsonl")
	if err := os.WriteFile(contractPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	args := append(interestRuleArgs(contractPath), "-input", interestRuleInput, "-rule-only", "-trace", tracePath)
	if code := Run(args, strings.NewReader(""), &out, &stderr); code != 0 {
		t.Fatalf("reordered contract: exit=%d, stdout=%s, stderr=%s", code, &out, &stderr)
	}
	data, err = os.ReadFile(tracePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte{'\n'}) {
		event := decodeRecord(t, string(line))
		if event["kind"] == "*" {
			// The final multiplication commutes, so check the first product.
			if event["result"] != "KRW 50000" {
				t.Fatalf("wrong principal × annual_rate: %v", event)
			}
			return
		}
	}
	t.Fatal("missing interest multiplication trace")
}

func TestInterestRuleRejectsInvalidInputs(t *testing.T) {
	input, err := os.ReadFile(interestRuleInput)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, original, replace, field string
	}{
		{"currency", `"currency":"KRW"`, `"currency":"USD"`, "principal"},
		{"rate above range", `"annual_rate":"0.05"`, `"annual_rate":"1.01"`, "annual_rate"},
		{"period above range", `"years":"0.5"`, `"years":"10.01"`, "years"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "invalid.json")
			invalid := bytes.Replace(input, []byte(tc.original), []byte(tc.replace), 1)
			if err := os.WriteFile(path, invalid, 0600); err != nil {
				t.Fatal(err)
			}
			var out, stderr bytes.Buffer
			args := append(interestRuleArgs(interestRuleContract), "-input", path, "-rule-only")
			if code := Run(args, strings.NewReader(""), &out, &stderr); code != 1 {
				t.Fatalf("invalid input: exit=%d, stdout=%s, stderr=%s", code, &out, &stderr)
			}
			record := decodeRecord(t, out.String())
			failure, ok := record["error"].(map[string]any)
			if !ok || failure["code"] != "E_CONTRACT_INPUT" || !strings.Contains(stderr.String(), tc.field) {
				t.Fatalf("wrong failure for %s: %v, stderr=%s", tc.field, record, &stderr)
			}
		})
	}
}

func TestInterestRuleRejectsMismatchedContract(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.json")
	contractPath := filepath.Join(dir, "contract.json")
	for original, target := range map[string]string{interestRuleInput: inputPath, interestRuleContract: contractPath} {
		data, err := os.ReadFile(original)
		if err != nil {
			t.Fatal(err)
		}
		changed := bytes.Replace(data, []byte(`"annual_rate"`), []byte(`"interest_rate"`), 1)
		if err := os.WriteFile(target, changed, 0600); err != nil {
			t.Fatal(err)
		}
	}
	var out, stderr bytes.Buffer
	args := append(interestRuleArgs(contractPath), "-input", inputPath, "-rule-only")
	if code := Run(args, strings.NewReader(""), &out, &stderr); code != 1 {
		t.Fatalf("mismatched contract: exit=%d, stdout=%s, stderr=%s", code, &out, &stderr)
	}
	record := decodeRecord(t, out.String())
	failure, ok := record["error"].(map[string]any)
	if !ok || failure["code"] != "E_CONTRACT" || !strings.Contains(stderr.String(), "annual_rate") {
		t.Fatalf("expected a named input declaration error: %v, stderr=%s", record, &stderr)
	}
}
