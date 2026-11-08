package eval

import (
	"testing"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
)

func TestListBuiltins(t *testing.T) {
	cases := []struct{ source, want string }{
		{`sum([0.1, 0.2, -0.05])`, "0.25"},
		{`sum([money("KRW", 0.5), money("KRW", 1)])`, "KRW 1.5"},
		{`sum(1, 2, 3)`, "6"},
		{`all([])`, "true"},
		{`all([true, false])`, "false"},
		{`all([true, true])`, "true"},
		{`any([])`, "false"},
		{`any([false, true])`, "true"},
		{`any([false, false])`, "false"},
		{`sorted([])`, "[]"},
		{`sorted([0.1, -5, 0.10, 0])`, "[-5, 0, 0.1, 0.1]"},
		{`sorted([money("USD", 2), money("USD", 1)])`, "[USD 1, USD 2]"},
		{`lower_bound([], 4)`, "0"},
		{`lower_bound([], money("KRW", 4))`, "0"},
		{`lower_bound([10, 20, 20, 30], 9)`, "0"},
		{`lower_bound([10, 20, 20, 30], 20)`, "1"},
		{`lower_bound([10, 20, 20, 30], 21)`, "3"},
		{`lower_bound([10, 20, 20, 30], 31)`, "4"},
		{`lower_bound([money("KRW", 10), money("KRW", 20)], money("KRW", 19))`, "1"},
	}
	for _, tc := range cases {
		t.Run(tc.source, func(t *testing.T) {
			result := evaluate(tc.source, config.DefaultLimits())
			if result.err != nil || result.value == nil || result.value.Inspect() != tc.want {
				t.Fatalf("got %v, error %v; want %s", result.value, result.err, tc.want)
			}
		})
	}
}

func TestListBuiltinErrors(t *testing.T) {
	cases := []struct {
		source string
		code   diagnostic.Code
	}{
		{`sum()`, "E_ARITY"},
		{`sum([])`, "E_EMPTY_LIST"},
		{`sum([1], 2)`, "E_TYPE_MISMATCH"},
		{`sum([1, money("KRW", 2)])`, "E_TYPE_MISMATCH"},
		{`sum([money("USD", 1), money("KRW", 2)])`, "E_CURRENCY_MISMATCH"},
		{`all([false, 1])`, "E_TYPE_MISMATCH"},
		{`any([true, 1])`, "E_TYPE_MISMATCH"},
		{`any(range(0, 3))`, "E_TYPE_MISMATCH"},
		{`sorted([true])`, "E_TYPE_MISMATCH"},
		{`sorted([money("USD", 1), money("KRW", 2)])`, "E_CURRENCY_MISMATCH"},
		{`lower_bound([2, 1], 0)`, "E_UNSORTED_LIST"},
		{`lower_bound([1, 2, 3, 0], 1)`, "E_UNSORTED_LIST"},
		{`lower_bound([], false)`, "E_TYPE_MISMATCH"},
		{`lower_bound([1, 2], money("USD", 1))`, "E_TYPE_MISMATCH"},
		{`lower_bound([money("USD", 1), money("KRW", 2)], money("USD", 1))`, "E_CURRENCY_MISMATCH"},
	}
	for _, tc := range cases {
		t.Run(tc.source, func(t *testing.T) {
			result := evaluate(tc.source, config.DefaultLimits())
			if diagnostic.CodeOf(result.err) != tc.code {
				t.Fatalf("got %v; want %s", result.err, tc.code)
			}
		})
	}
}

func TestListSortPreservesTiesAndOriginal(t *testing.T) {
	result := evaluate(`let items = [2.0, 1, 2.00, 0]; sorted(items);`, config.DefaultLimits())
	if result.err != nil {
		t.Fatal(result.err)
	}
	original, _ := result.env.Lookup("items")
	originalValues := original.(listValue).elements
	sortedValues := result.value.(listValue).elements
	if original.Inspect() != "[2, 1, 2, 0]" {
		t.Fatalf("sort changed original: %s", original.Inspect())
	}
	if originalValues[0] == originalValues[2] {
		t.Fatal("tie fixtures must be equal numbers from distinct evaluations")
	}
	if sortedValues[2] != originalValues[0] || sortedValues[3] != originalValues[2] {
		t.Fatal("sort changed the relative order of equal numbers")
	}
}

func TestListBuiltinEvaluationBudget(t *testing.T) {
	numbers := evaluate(`[1, 2, 3, 4]`, config.DefaultLimits())
	bools := evaluate(`[true, false, true, false]`, config.DefaultLimits())
	if numbers.err != nil || bools.err != nil {
		t.Fatalf("list setup failed: %v, %v", numbers.err, bools.err)
	}
	key := numbers.value.(listValue).elements[0]
	cases := []struct {
		name string
		args []Value
		max  int
	}{
		{"sum", []Value{numbers.value}, 3},
		{"all", []Value{bools.value}, 3},
		{"any", []Value{bools.value}, 3},
		{"sorted", []Value{numbers.value}, 3},
		{"sorted", []Value{numbers.value}, 4},
		{"lower_bound", []Value{numbers.value, key}, 3},
		{"lower_bound", []Value{numbers.value, key}, 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			limits := config.DefaultLimits()
			limits.MaxEvalSteps = tc.max
			e := New(Options{Limits: limits})
			if _, err := e.builtin(tc.name, tc.args, diagnostic.Span{}); diagnostic.CodeOf(err) != "E_EXECUTION_LIMIT" {
				t.Fatalf("got %v; want E_EXECUTION_LIMIT", err)
			}
		})
	}
}
