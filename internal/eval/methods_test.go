package eval

import (
	"testing"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
)

func TestCollectionPairs(t *testing.T) {
	cases := []struct{ source, want string }{
		{`enumerate([])`, "[]"},
		{`enumerate(["a", "b"])[1].index`, "1"},
		{`enumerate(["a", "b"])[1].value`, "b"},
		{`enumerate(range(2, 5))[2].value`, "4"},
		{`zip_exact([], range(0, 0))`, "[]"},
		{`zip_exact(["a", "b"], range(10, 12))[1].left`, "b"},
		{`zip_exact(["a", "b"], range(10, 12))[1].right`, "11"},
		{`range(2, 5).enumerate()[1].value`, "3"},
		{`range(2, 5).zip_exact([4, 5, 6])[2].right`, "6"},
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

func TestTypedMethods(t *testing.T) {
	cases := []struct{ source, want string }{
		{`let len = 0; [1, 2, 3].len()`, "3"},
		{`[1, 2, 3].sum()`, "6"},
		{`[true, true].all()`, "true"},
		{`[false, true].any()`, "true"},
		{`[3, 1, 2].sorted().lower_bound(2)`, "1"},
		{`range(2, 5).len()`, "3"},
		{`(-2).abs()`, "2"},
		{`(1.235).round(2, "HALF_EVEN")`, "1.24"},
		{`(1.2).format(2)`, "1.20"},
		{`money("KRW", 1.235).settle(2, "HALF_EVEN").amount()`, "1.24"},
		{`money("KRW", 1).currency()`, "KRW"},
		{`"1.235".decimal()`, "1.235"},
		{`map {}.has("missing")`, "false"},
		{`map { "KRW": 0 }.has("KRW")`, "true"},
		{`map {}.with("new", 1).get("new")`, "1"},
		{`let old = map { "USD": 1, "KRW": 2 };
let next = old.with("USD", 3).with("new", 4);
[old.get("USD"), next.get("USD"), next.len(), old.keys(), next.keys()];`, "[1, 3, 3, [USD, KRW], [USD, KRW, new]]"},
		{`var calls = 0;
let items = fn() { calls = calls + 1; return [1, 2]; };
[items().len(), calls];`, "[2, 1]"},
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

func TestTypedMethodAndPairErrors(t *testing.T) {
	cases := []struct {
		source string
		code   diagnostic.Code
	}{
		{`enumerate(true)`, "E_TYPE_MISMATCH"},
		{`zip_exact([1], false)`, "E_TYPE_MISMATCH"},
		{`zip_exact([1, 2], [3])`, "E_LENGTH_MISMATCH"},
		{`zip_exact(range(0, 9223372036854775807), [])`, "E_LENGTH_MISMATCH"},
		{`enumerate(range(0, 9223372036854775807))`, "E_EXECUTION_LIMIT"},
		{`zip_exact(range(0, 9223372036854775807), range(0, 9223372036854775807))`, "E_EXECUTION_LIMIT"},
		{`map {}.get("missing")`, "E_MISSING_KEY"},
		{`map {}.has(1)`, "E_TYPE_MISMATCH"},
		{`map {}.with(1, "value")`, "E_TYPE_MISMATCH"},
		{`map {}.get()`, "E_ARITY"},
		{`[].sum(1)`, "E_ARITY"},
		{`true.sum()`, "E_UNKNOWN_METHOD"},
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

func TestPairEvaluationBudget(t *testing.T) {
	numbers := evaluate(`[1, 2, 3, 4]`, config.DefaultLimits())
	if numbers.err != nil {
		t.Fatal(numbers.err)
	}
	for _, tc := range []struct {
		name string
		args []Value
	}{
		{"enumerate", []Value{numbers.value}},
		{"zip_exact", []Value{numbers.value, numbers.value}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := config.DefaultLimits()
			limits.MaxEvalSteps = 3
			e := New(Options{Limits: limits})
			if _, err := e.builtin(tc.name, tc.args, diagnostic.Span{}); diagnostic.CodeOf(err) != "E_EXECUTION_LIMIT" {
				t.Fatalf("got %v; want E_EXECUTION_LIMIT", err)
			}
		})
	}
}

func TestMapMethodEvaluationBudget(t *testing.T) {
	result := evaluate(`map {"USD": 1, "KRW": 2}`, config.DefaultLimits())
	if result.err != nil {
		t.Fatal(result.err)
	}
	mapping := result.value.(mapValue)
	limits := config.DefaultLimits()
	limits.MaxEvalSteps = 1
	for _, tc := range []struct {
		name string
		args []Value
	}{
		{"keys", nil},
		{"with", []Value{StringValue("USD"), DecimalValue{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := New(Options{Limits: limits})
			if _, err := e.mapMethod(mapping, tc.name, tc.args, diagnostic.Span{}); diagnostic.CodeOf(err) != "E_EXECUTION_LIMIT" {
				t.Fatalf("got %v; want E_EXECUTION_LIMIT", err)
			}
		})
	}
}
