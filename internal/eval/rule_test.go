package eval

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Pirite08/finlang/internal/ast"
	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/decimal"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/fx"
	"github.com/Pirite08/finlang/internal/money"
	"github.com/Pirite08/finlang/internal/parser"
	"github.com/Pirite08/finlang/internal/trace"
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

type quoteSource func(base, target string) (fx.Quote, error)

func (source quoteSource) Latest(base, target string) (fx.Quote, error) {
	return source(base, target)
}

func fixedQuote(base, target, rate string) fx.Quote {
	return fx.Quote{Base: base, Target: target, Rate: rate, Source: "fixed test fixture", AsOf: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)}
}

func parseRuleForTest(t *testing.T, source string) *ast.Rule {
	t.Helper()
	program, err := parser.Parse("exchange.fin", source, config.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if program.Rule == nil {
		t.Fatal("rule declaration was not parsed")
	}
	return program.Rule
}

func ruleMoney(t *testing.T, currency, value string) MoneyValue {
	t.Helper()
	amount, err := decimal.Parse(value, config.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	m, err := money.New(currency, amount)
	if err != nil {
		t.Fatal(err)
	}
	return MoneyValue{Value: m}
}

func TestRuleRunsStepsInSourceOrderWithExactConversion(t *testing.T) {
	recorder := trace.NewRecorder(100)
	quote := fixedQuote("USD", "KRW", "1350.25")
	provider := quoteSource(func(base, target string) (fx.Quote, error) {
		if base != "USD" || target != "KRW" {
			t.Fatalf("unexpected rate direction %s/%s", base, target)
		}
		return quote, nil
	})
	e := New(Options{Trace: recorder, FXProvider: provider})
	value, err := e.RunRule(parseRuleForTest(t, exchangeRule), ruleMoney(t, "USD", "10"))
	if err != nil || value.Inspect() != "KRW 13502" {
		t.Fatalf("value = %v, error = %v", value, err)
	}
	var steps []string
	var converted, settled bool
	for _, event := range recorder.Events() {
		if event.Kind == "step_end" && event.Status == "ok" {
			steps = append(steps, event.Step)
		}
		if event.Kind == "binding" && event.Step == "convert_amount" && event.Binding == "converted" && event.Result == "KRW 13502.5" {
			converted = true
		}
		if event.Kind == "settle" && event.Step == "settle_amount" && event.Result == "KRW 13502" && event.Inexact {
			settled = true
		}
	}
	if !slices.Equal(steps, []string{"check_amount", "get_rate", "convert_amount", "settle_amount"}) || !converted || !settled {
		t.Fatalf("missing ordered step results: %+v", recorder.Events())
	}
	if quotes := e.FXQuotes(); len(quotes) != 1 || quotes[0] != quote {
		t.Fatalf("wrong quote provenance: %+v", quotes)
	}
}

func TestFailedRuleRequirementStopsBeforeRateLookup(t *testing.T) {
	recorder := trace.NewRecorder(100)
	e := New(Options{Trace: recorder, FXProvider: quoteSource(func(_, _ string) (fx.Quote, error) {
		t.Fatal("rate lookup ran after a failed require")
		return fx.Quote{}, nil
	})})
	_, err := e.RunRule(parseRuleForTest(t, exchangeRule), ruleMoney(t, "USD", "0"))
	var sourceError *diagnostic.Error
	if !errors.As(err, &sourceError) || sourceError.Code != "E_REQUIRE_FAILED" || sourceError.Span == nil || sourceError.Span.Line != 5 {
		t.Fatalf("missing require error location: %v", err)
	}
	var failed bool
	for _, event := range recorder.Events() {
		if event.Step != "check_amount" {
			t.Fatalf("recorded an unexecuted step: %+v", event)
		}
		if event.Kind == "step_end" && event.Status == "error" && event.ErrorCode == "E_REQUIRE_FAILED" {
			failed = true
		}
	}
	if !failed || len(e.FXQuotes()) != 0 {
		t.Fatalf("missing failed step or unexpected quotes: %+v", recorder.Events())
	}
}

func TestRuleFreezesRateWithinRunAndRefreshesOnNextRun(t *testing.T) {
	rule := parseRuleForTest(t, `rule refresh {
        input amount in "USD"
        output result in "KRW"
        step first { let first = convert(amount, rate("USD", "KRW")) }
        step second { let result = first + convert(amount, rate("USD", "KRW")) }
        return result
    }`)
	calls := 0
	rates := []string{"1350.25", "1400.25"}
	e := New(Options{FXProvider: quoteSource(func(base, target string) (fx.Quote, error) {
		if calls >= len(rates) {
			t.Fatal("provider was called more than once per run")
		}
		quote := fixedQuote(base, target, rates[calls])
		calls++
		return quote, nil
	})})
	for i, want := range []string{"KRW 27005", "KRW 28005"} {
		value, err := e.RunRule(rule, ruleMoney(t, "USD", "10"))
		if err != nil || value.Inspect() != want || calls != i+1 {
			t.Fatalf("run %d: value = %v, error = %v, provider calls = %d", i+1, value, err, calls)
		}
		quotes := e.FXQuotes()
		if len(quotes) != 1 || quotes[0].Rate != rates[i] {
			t.Fatalf("wrong quote snapshot: %+v", quotes)
		}
		quotes[0].Rate = "999"
		if e.FXQuotes()[0].Rate != rates[i] {
			t.Fatal("caller changed the evaluator's quote snapshot")
		}
	}
}

func TestRuleRejectsConversionWithOppositeQuoteDirection(t *testing.T) {
	rule := parseRuleForTest(t, `rule direction {
        input amount in "USD"
        output result in "KRW"
        step conversion {
            let quote = rate("KRW", "USD")
            let result = convert(amount, quote)
        }
        return result
    }`)
	recorder := trace.NewRecorder(100)
	e := New(Options{Trace: recorder, FXProvider: quoteSource(func(base, target string) (fx.Quote, error) {
		return fixedQuote(base, target, "0.00074"), nil
	})})
	_, err := e.RunRule(rule, ruleMoney(t, "USD", "10"))
	var sourceError *diagnostic.Error
	if !errors.As(err, &sourceError) || sourceError.Code != "E_CURRENCY_MISMATCH" || sourceError.Span == nil || sourceError.Span.Line != 6 {
		t.Fatalf("wrong conversion direction diagnostic: %v", err)
	}
	if quotes := e.FXQuotes(); len(quotes) != 1 || quotes[0].Base != "KRW" || quotes[0].Target != "USD" {
		t.Fatalf("failed execution lost its acquired quote: %+v", quotes)
	}
	events := recorder.Events()
	last := events[len(events)-1]
	if last.Kind != "step_end" || last.Step != "conversion" || last.Status != "error" || last.ErrorCode != "E_CURRENCY_MISMATCH" {
		t.Fatalf("wrong failed step trace: %+v", last)
	}
}

func TestRuleValidatesAllInputsBeforeSteps(t *testing.T) {
	rule := parseRuleForTest(t, `rule charges {
        input principal in "KRW"
        input rate_value as decimal
        input fee in "KRW"
        output result in "KRW"
        step calculate { let result = principal * rate_value + fee }
        return result
    }`)
	principal := ruleMoney(t, "KRW", "1000")
	fee := ruleMoney(t, "KRW", "10")
	for _, tc := range []struct {
		name   string
		inputs []Value
		line   int
	}{
		{"missing input", []Value{principal, DecimalValue{}}, 1},
		{"extra input", []Value{principal, DecimalValue{}, fee, fee}, 1},
		{"wrong decimal type", []Value{principal, fee, fee}, 3},
		{"wrong later currency", []Value{principal, DecimalValue{}, ruleMoney(t, "USD", "10")}, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := trace.NewRecorder(100)
			e := New(Options{Trace: recorder, RuleOnly: true})
			_, err := e.RunRule(rule, tc.inputs...)
			var sourceError *diagnostic.Error
			if !errors.As(err, &sourceError) || sourceError.Code != "E_RULE_INPUT" || sourceError.Span == nil || sourceError.Span.Line != tc.line {
				t.Fatalf("wrong input diagnostic: %v", err)
			}
			if len(recorder.Events()) != 0 {
				t.Fatalf("started steps before input validation: %+v", recorder.Events())
			}
		})
	}
}
