package compat

import (
	"bufio"
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/contract"
)

func TestCOBOLProtocolAcceptsExactRecords(t *testing.T) {
	for _, tc := range []struct {
		rule, text string
		want       outcome
	}{
		{"interest_v1", "case-0|ok|25000|none\n", outcome{Status: "ok", Currency: "KRW", Amount: "25000", Branch: "none"}},
		{"interest_v1", "case-0|error|E_LEGACY_RANGE|none\r\n", outcome{Status: "error", Currency: "KRW", ErrorClass: "E_LEGACY_RANGE", Branch: "none"}},
		{"fee_v1", "case-0|ok|1000|low\n", outcome{Status: "ok", Currency: "KRW", Amount: "1000", Branch: "low"}},
		{"fee_v1", "case-0|ok|1000|high\n", outcome{Status: "ok", Currency: "KRW", Amount: "1000", Branch: "high"}},
		{"sum_v1", "case-0|ok|-1.02|none\n", outcome{Status: "ok", Currency: "USD", Amount: "-1.02", Branch: "none"}},
		{"sum_v1", "case-0|ok|0.00|none\n", outcome{Status: "ok", Currency: "USD", Amount: "0.00", Branch: "none"}},
	} {
		t.Run(tc.rule+"/"+tc.want.Status+tc.want.Amount+tc.want.Branch, func(t *testing.T) {
			spec := loadRule(t, tc.rule).spec
			got, err := parseCOBOL([]byte(tc.text), []compatCase{{ID: "case-0"}}, spec)
			if err != nil || len(got) != 1 {
				t.Fatalf("parse returned %v, %v", got, err)
			}
			if err := compareOutcomes(got[0], tc.want); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCOBOLProtocolRejectsCorruption(t *testing.T) {
	for _, tc := range []struct {
		name, rule, text string
		ids              []string
	}{
		{"missing", "interest_v1", "", []string{"case-0"}},
		{"extra", "interest_v1", "case-0|ok|1|none\ncase-1|ok|2|none\n", []string{"case-0"}},
		{"duplicate", "interest_v1", "case-0|ok|1|none\ncase-0|ok|2|none\n", []string{"case-0", "case-1"}},
		{"out-of-order", "interest_v1", "case-1|ok|1|none\ncase-0|ok|2|none\n", []string{"case-0", "case-1"}},
		{"unknown-id", "interest_v1", "other|ok|1|none\n", []string{"case-0"}},
		{"extra-field", "interest_v1", "case-0|ok|1|none|ignored\n", []string{"case-0"}},
		{"missing-field", "interest_v1", "case-0|ok|1\n", []string{"case-0"}},
		{"unknown-status", "interest_v1", "case-0|success|1|none\n", []string{"case-0"}},
		{"wrong-error-class", "interest_v1", "case-0|error|E_CONTRACT_INPUT|none\n", []string{"case-0"}},
		{"integer-scale", "interest_v1", "case-0|ok|1.0|none\n", []string{"case-0"}},
		{"integer-negative-zero", "interest_v1", "case-0|ok|-0|none\n", []string{"case-0"}},
		{"leading-zero", "interest_v1", "case-0|ok|01|none\n", []string{"case-0"}},
		{"plus-sign", "interest_v1", "case-0|ok|+1|none\n", []string{"case-0"}},
		{"whitespace", "interest_v1", "case-0|ok| 1|none\n", []string{"case-0"}},
		{"blank-line", "interest_v1", "case-0|ok|1|none\n\n", []string{"case-0"}},
		{"wrong-branch", "interest_v1", "case-0|ok|1|low\n", []string{"case-0"}},
		{"fee-branch-missing", "fee_v1", "case-0|ok|1|none\n", []string{"case-0"}},
		{"sum-scale-one", "sum_v1", "case-0|ok|1.0|none\n", []string{"case-0"}},
		{"sum-scale-three", "sum_v1", "case-0|ok|1.000|none\n", []string{"case-0"}},
		{"sum-negative-zero", "sum_v1", "case-0|ok|-0.00|none\n", []string{"case-0"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cases := make([]compatCase, len(tc.ids))
			for i, id := range tc.ids {
				cases[i].ID = id
			}
			if _, err := parseCOBOL([]byte(tc.text), cases, loadRule(t, tc.rule).spec); err == nil {
				t.Fatalf("accepted corrupted stream %q", tc.text)
			}
		})
	}
}

func parseCOBOL(data []byte, cases []compatCase, spec contract.Spec) ([]outcome, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	results := make([]outcome, 0, len(cases))
	seen := map[string]bool{}
	amountPattern := `^-?(0|[1-9][0-9]*)$`
	if spec.Output.Scale > 0 {
		amountPattern = fmt.Sprintf(`^-?(0|[1-9][0-9]*)\.[0-9]{%d}$`, spec.Output.Scale)
	}
	amountRE := regexp.MustCompile(amountPattern)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "|")
		if len(fields) != 4 {
			return nil, fmt.Errorf("line %d: want four fields, got %q", len(results)+1, scanner.Text())
		}
		if seen[fields[0]] {
			return nil, fmt.Errorf("duplicate result ID %q", fields[0])
		}
		seen[fields[0]] = true
		if len(results) >= len(cases) {
			return nil, fmt.Errorf("unexpected extra result %q", fields[0])
		}
		if fields[0] != cases[len(results)].ID {
			return nil, fmt.Errorf("missing, unknown or out-of-order result: got %q want %q", fields[0], cases[len(results)].ID)
		}
		result := outcome{Status: fields[1], Currency: spec.Output.Currency, Branch: fields[3]}
		if (spec.Rule == "fee_v1" && result.Branch != "low" && result.Branch != "high") || (spec.Rule != "fee_v1" && result.Branch != "none") {
			return nil, fmt.Errorf("%s: invalid branch %q", fields[0], result.Branch)
		}
		switch result.Status {
		case "ok":
			if !amountRE.MatchString(fields[2]) || fields[2] == "-0" || fields[2] == "-0.00" {
				return nil, fmt.Errorf("%s: amount %q is not canonical fixed scale %d", fields[0], fields[2], spec.Output.Scale)
			}
			result.Amount = fields[2]
		case "error":
			if fields[2] != "E_LEGACY_RANGE" {
				return nil, fmt.Errorf("%s: unexpected error class %q", fields[0], fields[2])
			}
			result.ErrorClass = fields[2]
		default:
			return nil, fmt.Errorf("%s: unknown status %q", fields[0], result.Status)
		}
		results = append(results, result)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading COBOL output: %w", err)
	}
	if len(results) != len(cases) {
		return nil, fmt.Errorf("missing results: got %d want %d", len(results), len(cases))
	}
	return results, nil
}
