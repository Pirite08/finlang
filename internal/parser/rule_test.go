package parser_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/ast"
	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/parser"
)

const exchangeRule = `rule usd_to_krw {
    input amount in "USD"
    output result in "KRW"

    step check_amount {
        require amount > money("USD", 0)
    }

    step get_rate {
        let quote = rate("USD", "KRW")
    }

    step convert_amount {
        let converted = convert(amount, quote)
    }

    step settle_amount {
        let result = settle(converted, 0, "HALF_EVEN")
    }

    return result
}`

func TestParseExchangeRule(t *testing.T) {
	program := parse(t, exchangeRule)
	rule := program.Rule
	if rule == nil || len(program.Statements) != 0 || rule.Name != "usd_to_krw" {
		t.Fatalf("expected one named rule, got %+v", program)
	}
	if len(rule.Inputs) != 1 || rule.Inputs[0].Name != "amount" || rule.Inputs[0].Type != "money" || rule.Inputs[0].Currency != "USD" || rule.Output.Name != "result" || rule.Output.Currency != "KRW" {
		t.Fatalf("wrong input/output declarations: %+v, %+v", rule.Inputs, rule.Output)
	}
	want := []string{"check_amount", "get_rate", "convert_amount", "settle_amount"}
	if len(rule.Steps) != len(want) {
		t.Fatalf("got %d steps, want %d", len(rule.Steps), len(want))
	}
	for i, step := range rule.Steps {
		if step.Name != want[i] || len(step.Body.Statements) != 1 {
			t.Fatalf("step %d: %+v", i, step)
		}
	}
	check, ok := rule.Steps[0].Body.Statements[0].(*ast.Require)
	if !ok || check.Span.Line != 6 || check.Condition.GetSpan().Column != 17 {
		t.Fatalf("wrong require node or source location: %+v", check)
	}
	if returned, ok := rule.Return.Value.(*ast.Identifier); !ok || returned.Name != "result" {
		t.Fatalf("wrong return value: %+v", rule.Return.Value)
	}
}

func TestParseRuleWithMultipleInputDeclarations(t *testing.T) {
	rule := parse(t, `rule interest {
        input principal in "KRW"
        input annual_rate as decimal; input years as decimal
        output result in "KRW"
        step calculate {
            let result = principal * annual_rate * years
        }
        return result
    }`).Rule
	want := []struct{ name, kind, currency string }{
		{"principal", "money", "KRW"},
		{"annual_rate", "decimal", ""},
		{"years", "decimal", ""},
	}
	if len(rule.Inputs) != len(want) {
		t.Fatalf("got %d inputs, want %d", len(rule.Inputs), len(want))
	}
	for i, input := range rule.Inputs {
		if input.Name != want[i].name || input.Type != want[i].kind || input.Currency != want[i].currency {
			t.Fatalf("input %d: got %+v, want %+v", i, input, want[i])
		}
	}
}

func TestRuleStepSeparatorsAndNestedFunctions(t *testing.T) {
	program := parse(t, `rule sample {
        input amount in "USD"; output result in "USD";
        step work {
            let identity = fn(x) { return x } // Old function semantics.
            if (true) {
                require amount > money("USD", 0)
                require true
            }
            let result = identity(amount)
        };
        return result;
    };`)
	if got := len(program.Rule.Steps[0].Body.Statements); got != 3 {
		t.Fatalf("got %d statements, want 3", got)
	}
}

func TestRuleSyntaxErrors(t *testing.T) {
	for _, test := range []struct {
		name, source, message string
	}{
		{"no input", strings.Replace(exchangeRule, `input amount in "USD"`, "", 1), `expected "input"`},
		{"no output", strings.Replace(exchangeRule, `output result in "KRW"`, "", 1), `expected "output"`},
		{"duplicate input", strings.Replace(exchangeRule, `output result`, `input amount as decimal; output result`, 1), "duplicate input"},
		{"unknown input type", strings.Replace(exchangeRule, `input amount in "USD"`, `input amount as integer`, 1), `expected "decimal"`},
		{"output shares second input name", strings.Replace(exchangeRule, `output result`, `input result as decimal; output result`, 1), "different names"},
		{"same declaration names", strings.Replace(exchangeRule, `output result`, `output amount`, 1), "different names"},
		{"no steps", `rule r { input amount in "USD" output result in "USD" return result }`, "at least one step"},
		{"duplicate step", strings.Replace(exchangeRule, "step get_rate", "step check_amount", 1), "duplicate step"},
		{"no return", strings.Replace(exchangeRule, "return result", "", 1), "expected return"},
		{"wrong output", strings.Replace(exchangeRule, "return result", "return amount", 1), "declared output"},
		{"output expression", strings.Replace(exchangeRule, "return result", "return result * 2", 1), "declared output"},
		{"early return", strings.Replace(exchangeRule, "require amount > money(\"USD\", 0)", "return amount", 1), "return belongs after"},
		{"early conditional return", strings.Replace(exchangeRule, "require amount > money(\"USD\", 0)", "if (true) { return amount }", 1), "return belongs after"},
		{"missing separator", strings.Replace(exchangeRule, "let quote = rate(\"USD\", \"KRW\")", "let quote = 1 let extra = 2", 1), "expected newline or semicolon"},
		{"extra top-level statement", exchangeRule + "\nlet extra = 1", "only top-level declaration"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := parser.Parse("rule.fin", test.source, config.DefaultLimits())
			var detail *diagnostic.Error
			if !errors.As(err, &detail) || detail.Code != "E_SYNTAX" || detail.Span == nil || detail.Span.File != "rule.fin" || !strings.Contains(detail.Message, test.message) {
				t.Fatalf("got %v, want positioned E_SYNTAX containing %q", err, test.message)
			}
		})
	}
}

func TestRuleKeywordsRemainLegacyIdentifiers(t *testing.T) {
	for _, source := range []string{
		`rule(1)`,
		`let rule = fn(input, output, step, require, in, as, decimal) { return input }; rule(1, 2, 3, 4, 5, 6, 7)`,
	} {
		if program := parse(t, source); program.Rule != nil {
			t.Fatalf("legacy source parsed as rule: %s", source)
		}
	}
}
