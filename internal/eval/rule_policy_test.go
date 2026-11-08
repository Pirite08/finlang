package eval

import (
	"bytes"
	"errors"
	"testing"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/fx"
	"github.com/Pirite08/finlang/internal/parser"
	"github.com/Pirite08/finlang/internal/trace"
)

func TestRuleOnlyRejectsPolicyViolationsBeforeSteps(t *testing.T) {
	for _, tc := range []struct{ name, statement string }{
		{"print in dead branch", `if (false) { let ignored = print(amount) }`},
		{"print in unselected else", `if (true) { require true } else { print(amount) }`},
		{"print in unselected if expression", `let ignored = if (true) { amount } else { print(amount) }`},
		{"print in short-circuited and", `require false && print(amount)`},
		{"print in short-circuited or", `require true || print(amount)`},
		{"function literal", `let helper = fn() { return amount; }`},
		{"function value", `let helper = round`},
		{"new builtin through an alias", `let helper = sum; let ignored = helper(1, 2)`},
		{"indirect call", `let ignored = money("USD", 1)()`},
		{"unknown call", `let ignored = helper(amount)`},
		{"builtin shadowing", `let rate = 1`},
		{"len in dead branch", `if (false) { let ignored = len(amount) }`},
		{"range in dead branch", `if (false) { let ignored = range(0, 1) }`},
		{"list literal", `let ignored = [amount]`},
		{"indexing", `let ignored = amount[0]`},
		{"mutable declaration", `var mutable = amount`},
		{"assignment", `result = amount`},
		{"loop in dead branch", `if (false) { for item in amount { require true } }`},
		{"record in dead branch", `if (false) { let ignored = record { value: amount } }`},
		{"map in dead branch", `if (false) { let ignored = map { "USD": amount } }`},
		{"field in dead branch", `if (false) { let ignored = amount.value }`},
		{"method in dead branch", `if (false) { let ignored = amount.currency() }`},
		{"enumerate in dead branch", `if (false) { let ignored = enumerate(amount) }`},
		{"zip_exact in dead branch", `if (false) { let ignored = zip_exact(amount, amount) }`},
		{"matrix in dead branch", `if (false) { let ignored = matrix(amount) }`},
		{"mat_mul in dead branch", `if (false) { let ignored = mat_mul(amount, amount) }`},
		{"queue in dead branch", `if (false) { let ignored = queue() }`},
		{"dequeue in dead branch", `if (false) { let ignored = dequeue(amount) }`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule := parseRuleForTest(t, `rule checked {
				input amount in "USD"
				output result in "USD"
				step earlier { let result = amount }
				step checked { `+tc.statement+` }
				return result
			}`)
			recorder := trace.NewRecorder(100)
			var output bytes.Buffer
			evaluator := New(Options{RuleOnly: true, Trace: recorder, Stdout: &output})
			_, err := evaluator.RunRule(rule, ruleMoney(t, "USD", "10"))
			var typed *diagnostic.Error
			if !errors.As(err, &typed) || typed.Code != "E_RULE_POLICY" || typed.Span == nil || typed.Span.Line != 5 {
				t.Fatalf("expected positioned policy error, got %v", err)
			}
			if len(recorder.Events()) != 0 || output.Len() != 0 {
				t.Fatalf("executed before policy rejection: output=%q events=%+v", output.String(), recorder.Events())
			}
		})
	}
}

func TestRuleOnlyKeepsCollectionBuiltinNamesUsableAsBindings(t *testing.T) {
	rule := parseRuleForTest(t, `rule checked {
		input len in "USD"
		output range in "USD"
		step preserve { let range = len }
		return range
	}`)
	result, err := New(Options{RuleOnly: true}).RunRule(rule, ruleMoney(t, "USD", "10"))
	if err != nil || result.Inspect() != "USD 10" {
		t.Fatalf("existing bindings were not preserved: result=%v err=%v", result, err)
	}
}

func TestRuleOnlyRejectsLiveProviderBeforeSteps(t *testing.T) {
	recorder := trace.NewRecorder(100)
	calls := 0
	evaluator := New(Options{RuleOnly: true, Trace: recorder, FXProvider: quoteSource(func(base, target string) (fx.Quote, error) {
		calls++
		return fixedQuote(base, target, "1350.25"), nil
	})})
	_, err := evaluator.RunRule(parseRuleForTest(t, exchangeRule), ruleMoney(t, "USD", "10"))
	if diagnostic.CodeOf(err) != "E_RULE_POLICY" || calls != 0 || len(recorder.Events()) != 0 {
		t.Fatalf("live provider was not rejected before evaluation: err=%v calls=%d events=%+v", err, calls, recorder.Events())
	}
}

func TestRuleOnlyChecksAllDeclaredInputNames(t *testing.T) {
	rule := parseRuleForTest(t, `rule checked {
        input amount in "USD"
        input print as decimal
        output result in "USD"
        step calculate { let result = amount }
        return result
    }`)
	recorder := trace.NewRecorder(100)
	e := New(Options{RuleOnly: true, Trace: recorder})
	_, err := e.RunRule(rule, ruleMoney(t, "USD", "10"), DecimalValue{})
	var sourceError *diagnostic.Error
	if !errors.As(err, &sourceError) || sourceError.Code != "E_RULE_POLICY" || sourceError.Span == nil || sourceError.Span.Line != 3 {
		t.Fatalf("expected policy error at the second input, got %v", err)
	}
	if len(recorder.Events()) != 0 {
		t.Fatalf("started steps before policy validation: %+v", recorder.Events())
	}
}

func TestRuleOnlyUsesValidatedSnapshot(t *testing.T) {
	quote := fixedQuote("USD", "KRW", "1350.25")
	snapshot, err := fx.NewSnapshot([]fx.Quote{quote})
	if err != nil {
		t.Fatal(err)
	}
	evaluator := New(Options{RuleOnly: true, FXSnapshot: snapshot})
	rule := parseRuleForTest(t, exchangeRule)
	input := ruleMoney(t, "USD", "10")
	value, err := evaluator.RunRule(rule, input)
	if err != nil || value.Inspect() != "KRW 13502" {
		t.Fatalf("value=%v err=%v", value, err)
	}
	if quotes := evaluator.FXQuotes(); len(quotes) != 1 || quotes[0] != quote {
		t.Fatalf("wrong recorded quote: %+v", quotes)
	}
	if _, err := New(Options{RuleOnly: true}).RunRule(rule, input); diagnostic.CodeOf(err) != "E_FX_PROVIDER" {
		t.Fatalf("missing snapshot: %v", err)
	}
}

func TestRuleOnlyRejectsOtherEvaluatorEntryPoints(t *testing.T) {
	program, err := parser.Parse("ordinary.fin", `fn() { return 1; }`, config.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	evaluator := New(Options{RuleOnly: true})
	if _, err := evaluator.Run(program, NewEnvironment(nil)); diagnostic.CodeOf(err) != "E_RULE_POLICY" {
		t.Fatalf("standalone program was not rejected: %v", err)
	}
	closure, err := New(Options{}).Run(program, NewEnvironment(nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := evaluator.Call(closure, nil, diagnostic.Span{}); diagnostic.CodeOf(err) != "E_RULE_POLICY" {
		t.Fatalf("function value call was not rejected: %v", err)
	}
}
