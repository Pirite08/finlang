package eval

import (
	"fmt"
	"slices"

	"github.com/Pirite08/finlang/internal/ast"
	"github.com/Pirite08/finlang/internal/decimal"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/fx"
	"github.com/Pirite08/finlang/internal/money"
	"github.com/Pirite08/finlang/internal/trace"
)

type ruleExecution struct {
	step  string
	rates map[currencyPair]fxQuoteValue
}

// RunRule starts an independent rule execution. All step bodies share one
// environment so a step can read bindings declared by an earlier step.
func (e *Evaluator) RunRule(rule *ast.Rule, input Value) (Value, error) {
	e.steps = 0
	e.fxQuotes = nil
	if e.options.RuleOnly {
		if err := validateRulePolicy(rule, e.options.Limits.MaxSyntaxDepth); err != nil {
			return nil, err
		}
		if e.options.FXProvider != nil {
			return nil, rulePolicyError(rule.Span, "rule-only mode accepts a prepared FX snapshot only")
		}
	}
	for _, declaration := range []ast.MoneyDecl{rule.Input, rule.Output} {
		if _, err := money.New(declaration.Currency, decimal.Decimal{}); err != nil {
			return nil, diagnostic.WithSpan(err, declaration.Span)
		}
	}
	amount, ok := input.(MoneyValue)
	if !ok || amount.Value.Currency() != rule.Input.Currency {
		return nil, diagnostic.WithSpan(diagnostic.New("E_RULE_INPUT", fmt.Sprintf("input %q requires Money in %s", rule.Input.Name, rule.Input.Currency)), rule.Input.Span)
	}
	frame := NewEnvironment(nil)
	if err := frame.Define(rule.Input.Name, input); err != nil {
		return nil, diagnostic.WithSpan(err, rule.Input.Span)
	}
	e.rule = &ruleExecution{rates: make(map[currencyPair]fxQuoteValue)}
	defer func() { e.rule = nil }()
	for _, step := range rule.Steps {
		if err := e.runStep(step, frame); err != nil {
			return nil, err
		}
	}
	e.rule.step = ""
	if err := e.tick(rule.Return.Span); err != nil {
		return nil, err
	}
	value, err := e.expression(rule.Return.Value, frame)
	if err != nil {
		return nil, err
	}
	result, ok := value.(MoneyValue)
	if !ok || result.Value.Currency() != rule.Output.Currency {
		return nil, diagnostic.WithSpan(diagnostic.New("E_RULE_OUTPUT", fmt.Sprintf("output %q requires Money in %s", rule.Output.Name, rule.Output.Currency)), rule.Return.Span)
	}
	e.emit(trace.Event{Kind: "rule_return", Status: "ok", Span: rule.Return.Span}, nil, result)
	return result, nil
}

func (e *Evaluator) runStep(step *ast.Step, frame *Environment) error {
	e.rule.step = step.Name
	e.emit(trace.Event{Kind: "step_start", Status: "started", Span: step.Span}, nil, UnitValue{})
	result, err := e.block(step.Body, frame, false)
	if result.returned {
		err = diagnostic.WithSpan(diagnostic.New("E_STEP_RETURN", "return must follow the rule's steps"), step.Span)
	}
	if err != nil {
		e.emit(trace.Event{Kind: "step_end", Status: "error", Span: step.Span, ErrorCode: diagnostic.CodeOf(err)}, nil, UnitValue{})
		return err
	}
	if statements := step.Body.Statements; len(statements) > 0 {
		if binding, ok := statements[len(statements)-1].(*ast.Let); ok {
			result.value, _ = frame.Lookup(binding.Name)
		}
	}
	e.emit(trace.Event{Kind: "step_end", Status: "ok", Span: step.Span}, nil, result.value)
	return nil
}

// FXQuotes returns the rates acquired in first-use order for the latest rule
// run, including rates acquired before a later failure in that run.
func (e *Evaluator) FXQuotes() []fx.Quote { return slices.Clone(e.fxQuotes) }
