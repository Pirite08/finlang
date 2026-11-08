package eval

import (
	"slices"
	"testing"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/parser"
	"github.com/Pirite08/finlang/internal/trace"
)

func TestIfExpressionAndShortCircuit(t *testing.T) {
	for _, tc := range []struct{ name, source, want string }{
		{"true if branch", `let result = if (true) { 0.05 } else { missing }; result;`, "0.05"},
		{"false if branch", `let result = if (false) { missing } else { "selected" }; result;`, "selected"},
		{"nested if expression", `let result = if (true) { if (false) { 1 } else { 2 } } else { 3 }; result;`, "2"},
		{"and skips right operand", `false && missing`, "false"},
		{"or skips right operand", `true || missing`, "true"},
		{"and evaluates right operand", `true && false`, "false"},
		{"or evaluates right operand", `false || true`, "true"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := evaluate(tc.source, config.DefaultLimits())
			if result.err != nil || result.value == nil || result.value.Inspect() != tc.want {
				t.Fatalf("got %v, error %v; want %s", result.value, result.err, tc.want)
			}
		})
	}
}

func TestIfExpressionAndLogicalTypeErrors(t *testing.T) {
	for _, source := range []string{
		`let result = if (1) { 2 } else { 3 }; result;`,
		`1 && missing`,
		`1 || missing`,
		`true && 1`,
		`false || 1`,
	} {
		t.Run(source, func(t *testing.T) {
			result := evaluate(source, config.DefaultLimits())
			if diagnostic.CodeOf(result.err) != "E_TYPE_MISMATCH" {
				t.Fatalf("got %v; want E_TYPE_MISMATCH", result.err)
			}
		})
	}
}

func TestLogicalTraceRecordsOnlyEvaluatedOperands(t *testing.T) {
	program, err := parser.Parse("logic.fin", `false && missing; false || true;`, config.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	recorder := trace.NewRecorder(10)
	if _, err := New(Options{Trace: recorder}).Run(program, NewEnvironment(nil)); err != nil {
		t.Fatal(err)
	}
	events := recorder.Events()
	if len(events) != 2 ||
		events[0].Kind != "&&" || !slices.Equal(events[0].Inputs, []string{"false"}) || events[0].Result != "false" ||
		events[1].Kind != "||" || !slices.Equal(events[1].Inputs, []string{"false", "true"}) || events[1].Result != "true" {
		t.Fatalf("unexpected trace: %+v", events)
	}
}

func TestRuleOnlyAllowsIfExpressionsAndLogicalOperators(t *testing.T) {
	rule := parseRuleForTest(t, `rule checked {
		input amount in "USD"
		output result in "USD"
		step calculate {
			let result = if (amount > money("USD", 0) && true) { amount } else { money("USD", 0) }
			require true || missing
		}
		return result
	}`)
	recorder := trace.NewRecorder(100)
	result, err := New(Options{RuleOnly: true, Trace: recorder}).RunRule(rule, ruleMoney(t, "USD", "10"))
	if err != nil || result == nil || result.Inspect() != "USD 10" {
		t.Fatalf("result=%v err=%v", result, err)
	}
	if !slices.ContainsFunc(recorder.Events(), func(event trace.Event) bool {
		return event.Kind == "branch" && event.Branch == "true" && event.Step == "calculate"
	}) {
		t.Fatalf("if expression did not emit the branch: %+v", recorder.Events())
	}
}
