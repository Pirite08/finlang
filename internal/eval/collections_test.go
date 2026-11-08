package eval

import (
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
)

func TestCollectionsAndMutableLocals(t *testing.T) {
	cases := []struct{ name, source, want string }{
		{"empty list", `len([])`, "0"},
		{"index of nested list", `let xs = [[1], [2, 3]]; xs[1.0][0];`, "2"},
		{"list display", `[1, true, [3]]`, "[1, true, [3]]"},
		{"range length", `len(range(-2.0, 3))`, "5"},
		{"reverse range is empty", `len(range(4, 2))`, "0"},
		{"range index", `range(-2, 3)[4]`, "2"},
		{"range extreme endpoints", `range(-9223372036854775808, -9223372036854775806)[1]`, "-9223372036854775807"},
		{"loop accumulator", `var total = 0; for x in [1.2, -0.3, 0.4] { if (x > 0) { total = total + x; } } total;`, "1.6"},
		{"assign nearest mutable name", `var x = 1; if (true) { var x = 2; x = 3; } x;`, "1"},
		{"for captures fresh bindings", `var first = fn() { return 0; }; var last = first;
			for item in [10, 20] { if (item == 10) { first = fn() { return item; }; } else { last = fn() { return item; }; } }
			first() * 100 + last();`, "1020"},
		{"return exits for and function", `let f = fn() { for i in range(0, 1000000000) { return i; } return -1; }; f();`, "0"},
		{"empty loop leaves var intact", `var total = 1; for item in [] { total = total + item; } total;`, "1"},
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

func TestCollectionAndAssignmentErrors(t *testing.T) {
	cases := []struct {
		source string
		code   diagnostic.Code
	}{
		{`[1][-1]`, "E_INDEX"},
		{`[1][0.5]`, "E_INDEX"},
		{`[1][9223372036854775808]`, "E_INDEX"},
		{`[1]["0"]`, "E_INDEX"},
		{`[1][1]`, "E_INDEX"},
		{`[][0]`, "E_INDEX"},
		{`range(3, 4)[1]`, "E_INDEX"},
		{`1[0]`, "E_TYPE_MISMATCH"},
		{`len(true)`, "E_TYPE_MISMATCH"},
		{`range(0, 1.5)`, "E_RANGE"},
		{`range(0, 9223372036854775808)`, "E_RANGE"},
		{`range(-9223372036854775808, 0)`, "E_RANGE"},
		{`for value in 1 {}`, "E_TYPE_MISMATCH"},
		{`let x = 1; x = print(2);`, "E_IMMUTABLE_BINDING"},
		{`let f = fn(x) { x = 2; return x; }; f(1);`, "E_IMMUTABLE_BINDING"},
		{`for value in [1] { value = 2; }`, "E_IMMUTABLE_BINDING"},
		{`var x = 1; if (true) { let x = 2; x = 3; }`, "E_IMMUTABLE_BINDING"},
		{`missing = print(2);`, "E_UNDEFINED_NAME"},
		{`for value in [1] { var local = value; } local;`, "E_UNDEFINED_NAME"},
		{`var x = 1; let x = 2;`, "E_DUPLICATE_BINDING"},
	}
	for _, tc := range cases {
		t.Run(tc.source, func(t *testing.T) {
			result := evaluate(tc.source, config.DefaultLimits())
			if diagnostic.CodeOf(result.err) != tc.code || result.output != "" {
				t.Fatalf("got %v with output %q; want %s and no output", result.err, result.output, tc.code)
			}
		})
	}
}

func TestCollectionExecutionAndDisplayAreBounded(t *testing.T) {
	limits := config.DefaultLimits()
	limits.MaxEvalSteps = 12
	result := evaluate(`for item in range(0, 9223372036854775807) {}`, limits)
	if diagnostic.CodeOf(result.err) != "E_EXECUTION_LIMIT" {
		t.Fatalf("empty loop budget was not enforced: %v", result.err)
	}
	result = evaluate(`var xs = [1]; for i in range(0, 60) { xs = [xs, xs]; } xs;`, config.DefaultLimits())
	if result.err != nil {
		t.Fatal(result.err)
	}
	text := result.value.Inspect()
	if len(text) > 4096 || !strings.Contains(text, "...") {
		t.Fatalf("expected bounded, truncated display, got %d bytes", len(text))
	}
}
