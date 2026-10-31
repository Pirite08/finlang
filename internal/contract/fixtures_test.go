package contract

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/eval"
	"github.com/Pirite08/finlang/internal/parser"
)

func fixture(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile("../../" + path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func executeFixture(t *testing.T, rule, input string) (string, error) {
	t.Helper()
	limits := config.DefaultLimits()
	spec, err := Load(strings.NewReader(fixture(t, "contracts/"+rule+".json")), limits)
	if err != nil {
		return "", err
	}
	args, err := DecodeInput(strings.NewReader(input), spec, limits)
	if err != nil {
		return "", err
	}
	program, err := parser.Parse(rule+".fin", fixture(t, "examples/"+rule+".fin"), limits)
	if err != nil {
		return "", err
	}
	evaluator := eval.New(eval.Options{Limits: limits, JSONMode: true})
	env := eval.NewEnvironment(nil)
	if _, err := evaluator.Run(program, env); err != nil {
		return "", err
	}
	main, ok := env.Lookup(spec.Entry)
	if !ok {
		return "", fmt.Errorf("entry missing")
	}
	value, err := evaluator.Call(main, args, diagnostic.Span{File: rule + ".fin", Line: 1, Column: 1})
	if err != nil {
		return "", err
	}
	var output bytes.Buffer
	if err := EncodeResult(&output, spec, value); err != nil {
		return output.String(), err
	}
	return output.String(), nil
}

func TestRuleFixtures(t *testing.T) {
	for _, tc := range []struct{ rule, input, currency, amount string }{
		{"interest_v1", "interest", "KRW", "25000"},
		{"fee_v1", "fee", "KRW", "1500"},
		{"sum_v1", "sum", "USD", "1.00"},
	} {
		t.Run(tc.rule, func(t *testing.T) {
			out, err := executeFixture(t, tc.rule, fixture(t, "testdata/inputs/"+tc.input+".json"))
			if err != nil {
				t.Fatal(err)
			}
			want := fmt.Sprintf("{\"schema_version\":\"1\",\"rule\":%q,\"status\":\"ok\",\"result\":{\"currency\":%q,\"amount\":%q}}\n", tc.rule, tc.currency, tc.amount)
			if out != want {
				t.Fatalf("got %s want %s", out, want)
			}
		})
	}
}

func TestRuleFixtureBoundaries(t *testing.T) {
	interest := fixture(t, "testdata/inputs/interest.json")
	sum := fixture(t, "testdata/inputs/sum.json")
	fee := fixture(t, "testdata/inputs/fee.json")
	for _, tc := range []struct {
		name, rule, input, amount string
		code                      diagnostic.Code
	}{
		{"interest zero", "interest_v1", strings.Replace(interest, `"1000000"`, `"0"`, 1), "0", ""},
		{"interest half up", "interest_v1", strings.Replace(strings.Replace(strings.Replace(interest, `"1000000"`, `"1"`, 1), `"0.05"`, `"1"`, 1), `"0.5"`, `"0.5"`, 1), "1", ""},
		{"interest receiver overflow", "interest_v1", strings.Replace(strings.Replace(strings.Replace(interest, `"1000000"`, `"999999999.99"`, 1), `"0.05"`, `"1"`, 1), `"0.5"`, `"10"`, 1), "", "E_LEGACY_RANGE"},
		{"interest rate scale", "interest_v1", strings.Replace(interest, `"0.05"`, `"0.0500000"`, 1), "", "E_CONTRACT_INPUT"},
		{"fee below", "fee_v1", strings.Replace(fee, `"2000000"`, `"999999.99"`, 1), "1000", ""},
		{"fee equal", "fee_v1", strings.Replace(fee, `"2000000"`, `"1000000"`, 1), "1000", ""},
		{"fee above", "fee_v1", strings.Replace(fee, `"2000000"`, `"1000000.01"`, 1), "1000", ""},
		{"fee half up", "fee_v1", strings.Replace(fee, `"2000000"`, `"1001000"`, 1), "1001", ""},
		{"negative adjustment", "sum_v1", strings.Replace(sum, `"amount":"0.335"`, `"amount":"-0.335"`, 1), "0.34", ""},
		{"zero nearby", "sum_v1", strings.ReplaceAll(sum, `"0.335"`, `"-0.001"`), "0.00", ""},
		{"sum receiver overflow", "sum_v1", strings.ReplaceAll(sum, `"0.335"`, `"999999999.999"`), "", "E_LEGACY_RANGE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := executeFixture(t, tc.rule, tc.input)
			if diagnostic.CodeOf(err) != tc.code {
				t.Fatalf("got %v want %s", err, tc.code)
			}
			if tc.code != "" {
				if out != "" {
					t.Fatalf("failure emitted success bytes %q", out)
				}
				return
			}
			if !strings.Contains(out, `"amount":"`+tc.amount+`"`) {
				t.Fatalf("got %s want amount %s", out, tc.amount)
			}
		})
	}
}
