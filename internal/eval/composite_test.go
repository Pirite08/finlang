package eval

import (
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
)

func TestCompositeValues(t *testing.T) {
	cases := []struct{ name, source, want string }{
		{"record field", `let row = record { amount: money("KRW", 1200), name: "deposit" }; row.amount;`, "KRW 1200"},
		{"string key", `let key = "USD"; let totals = map { "KRW": 3, "USD": 4 }; totals[key];`, "4"},
		{"nested members and indexing", `map { "rows": [record { amount: 2 }] }["rows"][0].amount;`, "2"},
		{"ordered record display", `record { second: 2, first: [1] };`, "record {second: 2, first: [1]}"},
		{"ordered map display", `map { "USD": 2, "KRW": 1 };`, `map {"USD": 2, "KRW": 1}`},
		{"empty record", `record {};`, "record {}"},
		{"empty map", `map {};`, "map {}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := evaluate(tc.source, config.DefaultLimits())
			if result.err != nil || result.value == nil || result.value.Inspect() != tc.want {
				t.Fatalf("got %v, error %v; want %s", result.value, result.err, tc.want)
			}
		})
	}
}

func TestCompositeErrors(t *testing.T) {
	cases := []struct {
		name, source string
		code         diagnostic.Code
	}{
		{"missing field", `record { amount: 1 }.currency;`, "E_MISSING_FIELD"},
		{"missing key", `map { "KRW": 1 }["USD"];`, "E_MISSING_KEY"},
		{"non-string map key", `map { "KRW": 1 }[0];`, "E_TYPE_MISMATCH"},
		{"duplicate field", `record { amount: 1, amount: print(2) };`, "E_DUPLICATE_FIELD"},
		{"duplicate key", `map { "KRW": 1, "KRW": print(2) };`, "E_DUPLICATE_KEY"},
		{"non-record field access", `[1].amount;`, "E_TYPE_MISMATCH"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := evaluate(tc.source, config.DefaultLimits())
			if diagnostic.CodeOf(result.err) != tc.code || result.output != "" {
				t.Fatalf("got %v with output %q; want %s and no output", result.err, result.output, tc.code)
			}
		})
	}
}

func TestCompositeWorkAndDisplayAreBounded(t *testing.T) {
	limits := config.DefaultLimits()
	limits.MaxEvalSteps = 10
	result := evaluate(`let row = record { first: 1, second: 2, third: 3 }; row.third;`, limits)
	if diagnostic.CodeOf(result.err) != "E_EXECUTION_LIMIT" {
		t.Fatalf("field lookup did not consume evaluation budget: %v", result.err)
	}

	result = evaluate(`var value = [];
		for i in range(0, 12) {
			value = [record { left: value, right: value }, map { "a": value, "b": value }];
		}
		value;`, config.DefaultLimits())
	if result.err != nil || result.value == nil {
		t.Fatalf("nested composite construction failed: %v", result.err)
	}
	text := result.value.Inspect()
	if len(text) > 4096 || !strings.Contains(text, "...") {
		t.Fatalf("expected bounded composite display, got %d bytes", len(text))
	}
}
