package compat

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/contract"
	"github.com/Pirite08/finlang/internal/diagnostic"
)

const caseSeed int64 = 20261031
const acceptedPerRule = 1024

var ruleNames = []string{"interest_v1", "fee_v1", "sum_v1"}

type compatCase struct {
	ID    string          `json:"id"`
	Label string          `json:"label"`
	Args  []string        `json:"arguments"`
	Input json.RawMessage `json:"input"`
	Want  *outcome        `json:"manual_anchor,omitempty"`
}

func TestGeneratorsAreDeterministicAndAccepted(t *testing.T) {
	for _, name := range ruleNames {
		t.Run(name, func(t *testing.T) {
			rule := loadRule(t, name)
			cases := generatedCases(rule.spec)
			if !reflect.DeepEqual(cases, generatedCases(rule.spec)) {
				t.Fatal("fixed-seed generation changed within the same run")
			}
			if len(cases) != acceptedPerRule {
				t.Fatalf("got %d cases, want %d", len(cases), acceptedPerRule)
			}
			seen := map[string]bool{}
			for _, c := range cases {
				if seen[string(c.Input)] {
					t.Fatalf("duplicate generated input: %s", c.Input)
				}
				seen[string(c.Input)] = true
				if _, err := contract.DecodeInput(strings.NewReader(string(c.Input)), rule.spec, config.DefaultLimits()); err != nil {
					t.Fatalf("generated case %s must be contract-accepted: %v", c.ID, err)
				}
			}
		})
	}
}

func TestManualAnchors(t *testing.T) {
	for _, name := range ruleNames {
		rule := loadRule(t, name)
		for _, c := range manualAnchors(name) {
			c.Input = makeInput(rule.spec, c.Args)
			t.Run(name+"/"+c.Label, func(t *testing.T) {
				got, err := rule.execute(c.Input)
				if err != nil {
					t.Fatal(err)
				}
				if err := compareOutcomes(got, *c.Want); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestContractRejectedInputsStayOutsideExecution(t *testing.T) {
	for _, name := range ruleNames {
		rule := loadRule(t, name)
		for _, c := range rejectedCases(rule.spec) {
			t.Run(name+"/"+c.Label, func(t *testing.T) {
				_, err := contract.DecodeInput(strings.NewReader(string(c.Input)), rule.spec, config.DefaultLimits())
				if diagnostic.CodeOf(err) != "E_CONTRACT_INPUT" {
					t.Fatalf("rejected table entry got %v, want E_CONTRACT_INPUT", err)
				}
			})
		}
	}
}

func manualAnchors(rule string) []compatCase {
	var cases []compatCase
	add := func(label string, want outcome, args ...string) {
		cases = append(cases, compatCase{Label: label, Args: args, Want: &want})
	}
	ok := func(currency, amount, branch string) outcome {
		return outcome{Status: "ok", Currency: currency, Amount: amount, Branch: branch}
	}
	overflow := func(currency, branch string) outcome {
		return outcome{Status: "error", Currency: currency, ErrorClass: "E_LEGACY_RANGE", Branch: branch}
	}
	switch rule {
	case "interest_v1":
		add("reference", ok("KRW", "25000", "none"), "1000000", "0.05", "0.5")
		add("zero-principal", ok("KRW", "0", "none"), "0", "1", "10")
		add("zero-rate", ok("KRW", "0", "none"), "999999999.99", "0", "10")
		add("zero-years", ok("KRW", "0", "none"), "999999999.99", "1", "0")
		add("rate-before-half", ok("KRW", "0", "none"), "1", "0.499999", "1")
		add("rate-at-half", ok("KRW", "1", "none"), "1", "0.500000", "1")
		add("rate-after-half", ok("KRW", "1", "none"), "1", "0.500001", "1")
		add("years-before-half", ok("KRW", "0", "none"), "1", "1", "0.499999")
		add("years-at-half", ok("KRW", "1", "none"), "1", "1", "0.500000")
		add("years-after-half", ok("KRW", "1", "none"), "1", "1", "0.500001")
		add("fourteen-fractional-digits", ok("KRW", "0", "none"), "0.01", "0.000001", "0.000001")
		add("receiver-max", ok("KRW", "999999999", "none"), "999999999", "1", "1")
		add("before-receiver-round-up", ok("KRW", "999999999", "none"), "999999999.49", "1", "1")
		add("receiver-round-up-overflow", overflow("KRW", "none"), "999999999.50", "1", "1")
		add("maximum-principal-overflow", overflow("KRW", "none"), "999999999.99", "1", "1")
		add("maximum-product-overflow", overflow("KRW", "none"), "999999999.99", "1", "10")
	case "fee_v1":
		add("zero", ok("KRW", "0", "low"), "0")
		add("before-low-half", ok("KRW", "0", "low"), "499.99")
		add("at-low-half", ok("KRW", "1", "low"), "500.00")
		add("after-low-half", ok("KRW", "1", "low"), "500.01")
		add("before-threshold", ok("KRW", "1000", "low"), "999999.99")
		add("at-threshold", ok("KRW", "1000", "low"), "1000000")
		add("after-threshold", ok("KRW", "1000", "high"), "1000000.01")
		add("before-high-half", ok("KRW", "1000", "high"), "1000999.99")
		add("at-high-half", ok("KRW", "1001", "high"), "1001000")
		add("after-high-half", ok("KRW", "1001", "high"), "1001000.01")
		add("plan-example", ok("KRW", "1001", "high"), "1002000")
		add("reference", ok("KRW", "1500", "high"), "2000000")
		add("maximum-input", ok("KRW", "500500", "high"), "999999999.99")
	case "sum_v1":
		add("round-after-summing", ok("USD", "1.00", "none"), "0.335", "0.335", "0.335")
		add("negative-adjustment", ok("USD", "0.34", "none"), "-0.335", "0.335", "0.335")
		add("negative-triple", ok("USD", "-1.00", "none"), "-0.335", "-0.335", "-0.335")
		add("before-positive-half", ok("USD", "1.00", "none"), "1.004", "0", "0")
		add("positive-even-half", ok("USD", "1.00", "none"), "1.005", "0", "0")
		add("after-positive-half", ok("USD", "1.01", "none"), "1.006", "0", "0")
		add("positive-odd-half", ok("USD", "1.02", "none"), "1.015", "0", "0")
		add("before-negative-half", ok("USD", "-1.00", "none"), "-1.004", "0", "0")
		add("negative-even-half", ok("USD", "-1.00", "none"), "-1.005", "0", "0")
		add("after-negative-half", ok("USD", "-1.01", "none"), "-1.006", "0", "0")
		add("negative-odd-half", ok("USD", "-1.02", "none"), "-1.015", "0", "0")
		add("negative-zero-input", ok("USD", "0.00", "none"), "-0.000", "0", "-0.000")
		add("negative-tiny", ok("USD", "0.00", "none"), "-0.001", "-0.001", "-0.001")
		add("negative-zero-half", ok("USD", "0.00", "none"), "-0.005", "0", "0")
		add("negative-past-zero-half", ok("USD", "-0.01", "none"), "-0.006", "0", "0")
		add("cancellation", ok("USD", "0.00", "none"), "999999999.999", "-999999999.999", "0")
		add("cancellation-before-settlement", ok("USD", "0.02", "none"), "999999999.999", "-999999999.999", "0.015")
		add("receiver-max", ok("USD", "999999999.99", "none"), "999999999.990", "0", "0")
		add("receiver-min", ok("USD", "-999999999.99", "none"), "-999999999.990", "0", "0")
		add("before-positive-overflow", ok("USD", "999999999.99", "none"), "999999999.990", "0.004", "0")
		add("positive-round-up-overflow", overflow("USD", "none"), "999999999.990", "0.005", "0")
		add("before-negative-overflow", ok("USD", "-999999999.99", "none"), "-999999999.990", "-0.004", "0")
		add("negative-round-up-overflow", overflow("USD", "none"), "-999999999.990", "-0.005", "0")
		add("positive-maximum-overflow", overflow("USD", "none"), "999999999.999", "999999999.999", "999999999.999")
		add("negative-maximum-overflow", overflow("USD", "none"), "-999999999.999", "-999999999.999", "-999999999.999")
	}
	return cases
}

func generatedCases(spec contract.Spec) []compatCase {
	cases := manualAnchors(spec.Rule)
	seed := caseSeed
	for i, name := range ruleNames {
		if name == spec.Rule {
			seed += int64(i)
		}
	}
	rng := rand.New(rand.NewSource(seed))
	seen := map[string]bool{}
	for i := range cases {
		cases[i].Input = makeInput(spec, cases[i].Args)
		seen[string(cases[i].Input)] = true
	}
	for len(cases) < acceptedPerRule {
		var args []string
		switch spec.Rule {
		case "interest_v1":
			args = []string{fixed(rng.Int63n(100000000000), 2), fixed(rng.Int63n(1000001), 6), fixed(rng.Int63n(10000001), 6)}
		case "fee_v1":
			amount := rng.Int63n(100000001)
			if len(cases)%2 == 1 {
				amount = 100000001 + rng.Int63n(99900000000-1)
			}
			args = []string{fixed(amount, 2)}
		case "sum_v1":
			args = []string{fixed(rng.Int63n(1999999999999)-999999999999, 3), fixed(rng.Int63n(1999999999999)-999999999999, 3), fixed(rng.Int63n(1999999999999)-999999999999, 3)}
		}
		input := makeInput(spec, args)
		if !seen[string(input)] {
			seen[string(input)] = true
			cases = append(cases, compatCase{Label: "generated", Args: args, Input: input})
		}
	}
	for i := range cases {
		cases[i].ID = fmt.Sprintf("%s-%04d", spec.Rule, i)
	}
	return cases
}

func fixed(n int64, scale int) string {
	sign := ""
	if n < 0 {
		sign, n = "-", -n
	}
	power := int64(1)
	for i := 0; i < scale; i++ {
		power *= 10
	}
	return fmt.Sprintf("%s%d.%0*d", sign, n/power, scale, n%power)
}

func makeInput(spec contract.Spec, args []string) json.RawMessage {
	inputs := map[string]any{}
	for i, p := range spec.Parameters {
		if p.Type == "money" {
			inputs[p.Name] = map[string]string{"currency": p.Currency, "amount": args[i]}
		} else {
			inputs[p.Name] = args[i]
		}
	}
	data, err := json.Marshal(map[string]any{"schema_version": "1", "rule": spec.Rule, "inputs": inputs})
	if err != nil {
		panic(err)
	}
	return data
}

func rejectedCases(spec contract.Spec) []compatCase {
	var cases []compatCase
	add := func(label string, args ...string) {
		cases = append(cases, compatCase{Label: label, Input: makeInput(spec, args)})
	}
	switch spec.Rule {
	case "interest_v1":
		add("negative-principal", "-0.01", "1", "1")
		add("principal-range", "1000000000", "1", "1")
		add("principal-scale", "0.001", "1", "1")
		add("rate-range", "1", "1.000001", "1")
		add("rate-scale", "1", "0.0000001", "1")
		add("years-range", "1", "1", "10.000001")
		add("years-scale", "1", "1", "0.0000001")
		add("exponent", "1e2", "1", "1")
	case "fee_v1":
		add("negative", "-0.01")
		add("range", "1000000000")
		add("scale", "0.001")
		add("exponent", "1e2")
	case "sum_v1":
		add("positive-range", "1000000000", "0", "0")
		add("negative-range", "-1000000000", "0", "0")
		add("scale", "0.0001", "0", "0")
		add("exponent", "1e2", "0", "0")
	}
	args := make([]string, len(spec.Parameters))
	for i := range args {
		args[i] = "0"
	}
	base := string(makeInput(spec, args))
	otherCurrency := "USD"
	if spec.Output.Currency == "USD" {
		otherCurrency = "KRW"
	}
	for _, tc := range []struct{ label, input string }{
		{"wrong-currency", strings.Replace(base, `"currency":"`+spec.Output.Currency+`"`, `"currency":"`+otherCurrency+`"`, 1)},
		{"json-number", strings.Replace(base, `"amount":"0"`, `"amount":0`, 1)},
		{"wrong-version", strings.Replace(base, `"schema_version":"1"`, `"schema_version":"2"`, 1)},
		{"wrong-rule", strings.Replace(base, `"rule":"`+spec.Rule+`"`, `"rule":"other"`, 1)},
		{"unknown-field", strings.Replace(base, `"inputs":{`, `"inputs":{"extra":"0",`, 1)},
		{"duplicate-key", strings.Replace(base, `"schema_version":"1"`, `"schema_version":"1","schema_version":"1"`, 1)},
		{"missing-inputs", `{"schema_version":"1","rule":"` + spec.Rule + `","inputs":{}}`},
		{"malformed-json", `{"schema_version":`},
	} {
		cases = append(cases, compatCase{Label: tc.label, Input: json.RawMessage(tc.input)})
	}
	for i := range cases {
		cases[i].ID = fmt.Sprintf("%s-rejected-%02d", spec.Rule, i)
	}
	return cases
}
