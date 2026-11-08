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

// ValidateRulePolicy checks the restricted rule profile without executing a step.
func (e *Evaluator) ValidateRulePolicy(rule *ast.Rule) error {
	policy := rulePolicy{collections: e.collectionRules(), maxItems: e.options.Limits.MaxCollectionItems}
	return policy.validate(rule, e.options.Limits.MaxSyntaxDepth)
}

// RunRule starts an independent rule execution with inputs in declaration order.
// All step bodies share one environment so later steps can read earlier bindings.
func (e *Evaluator) RunRule(rule *ast.Rule, inputs ...Value) (Value, error) {
	e.steps = 0
	e.fxQuotes = nil
	if e.options.RuleOnly {
		if err := e.ValidateRulePolicy(rule); err != nil {
			return nil, err
		}
		if e.options.FXProvider != nil {
			return nil, rulePolicyError(rule.Span, "rule-only mode accepts a prepared FX snapshot only")
		}
	}
	if len(inputs) != len(rule.Inputs) {
		message := fmt.Sprintf("rule %q expects %d inputs, got %d", rule.Name, len(rule.Inputs), len(inputs))
		return nil, diagnostic.WithSpan(diagnostic.New("E_RULE_INPUT", message), rule.Span)
	}
	switch rule.Output.Type {
	case "map":
		if !e.collectionRules() {
			return nil, rulePolicyError(rule.Output.Span, "Map output requires the collection rule profile")
		}
	case "record":
		if !e.collectionRules() {
			return nil, rulePolicyError(rule.Output.Span, "Record output requires the collection rule profile")
		}
	default:
		if _, err := money.New(rule.Output.Currency, decimal.Decimal{}); err != nil {
			return nil, diagnostic.WithSpan(err, rule.Output.Span)
		}
	}
	frame := NewEnvironment(nil)
	for i, declaration := range rule.Inputs {
		if err := e.validateRuleInput(declaration, inputs[i]); err != nil {
			return nil, err
		}
		if err := frame.Define(declaration.Name, inputs[i]); err != nil {
			return nil, diagnostic.WithSpan(err, declaration.Span)
		}
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
	if err := validateRuleOutput(rule, value); err != nil {
		return nil, err
	}
	e.emit(trace.Event{Kind: "rule_return", Status: "ok", Span: rule.Return.Span}, nil, value)
	return value, nil
}

func validateRuleOutput(rule *ast.Rule, value Value) error {
	var expected string
	switch rule.Output.Type {
	case "map":
		if _, ok := value.(mapValue); ok {
			return nil
		}
		expected = "Map"
	case "record":
		if _, ok := value.(recordValue); ok {
			return nil
		}
		expected = "Record"
	default:
		if result, ok := value.(MoneyValue); ok && result.Value.Currency() == rule.Output.Currency {
			return nil
		}
		expected = "Money in " + rule.Output.Currency
	}
	message := fmt.Sprintf("output %q requires %s", rule.Output.Name, expected)
	return diagnostic.WithSpan(diagnostic.New("E_RULE_OUTPUT", message), rule.Return.Span)
}

func (e *Evaluator) validateRuleInput(declaration ast.InputDecl, value Value) error {
	var expected string
	switch declaration.Type {
	case "money":
		if _, err := money.New(declaration.Currency, decimal.Decimal{}); err != nil {
			return diagnostic.WithSpan(err, declaration.Span)
		}
		if amount, ok := value.(MoneyValue); ok && amount.Value.Currency() == declaration.Currency {
			return nil
		}
		expected = "Money in " + declaration.Currency
	case "decimal":
		if _, ok := value.(DecimalValue); ok {
			return nil
		}
		expected = "Decimal"
	case "list":
		if !e.collectionRules() {
			return rulePolicyError(declaration.Span, "List input requires the collection rule profile")
		}
		if rows, ok := value.(listValue); ok {
			return e.validateRecordList(declaration, rows)
		}
		expected = "List of Records"
	case "record":
		if !e.collectionRules() {
			return rulePolicyError(declaration.Span, "Record input requires the collection rule profile")
		}
		if record, ok := value.(recordValue); ok {
			return e.validateRecordFields(declaration, record)
		}
		expected = "Record"
	default:
		return diagnostic.WithSpan(diagnostic.New("E_RULE_INPUT", "unsupported rule input type"), declaration.Span)
	}
	message := fmt.Sprintf("input %q requires %s", declaration.Name, expected)
	return diagnostic.WithSpan(diagnostic.New("E_RULE_INPUT", message), declaration.Span)
}

func (e *Evaluator) validateRecordList(declaration ast.InputDecl, rows listValue) error {
	span := declaration.Span
	if err := e.checkCollectionSize(rows.length(), span); err != nil {
		return err
	}
	if err := e.chargeCollectionWork(rows.length(), span); err != nil {
		return err
	}
	for _, item := range rows.elements {
		record, ok := item.(recordValue)
		if !ok {
			message := fmt.Sprintf("input %q must contain only Records", declaration.Name)
			return diagnostic.WithSpan(diagnostic.New("E_RULE_INPUT", message), span)
		}
		if err := e.validateRecordFields(declaration, record); err != nil {
			return err
		}
	}
	return nil
}

func (e *Evaluator) validateRecordFields(declaration ast.InputDecl, record recordValue) error {
	span := declaration.Span
	count := int64(len(record.fields))
	if err := e.checkCollectionSize(count, span); err != nil {
		return err
	}
	if err := e.chargeCollectionWork(count, span); err != nil {
		return err
	}
	for _, field := range record.fields {
		switch field.value.(type) {
		case MoneyValue, DecimalValue, StringValue:
		default:
			message := fmt.Sprintf("input %q Record fields must be Money, Decimal or String", declaration.Name)
			return diagnostic.WithSpan(diagnostic.New("E_RULE_INPUT", message), span)
		}
	}
	return nil
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
		switch binding := statements[len(statements)-1].(type) {
		case *ast.Let:
			result.value, _ = frame.Lookup(binding.Name)
		case *ast.Var:
			if e.collectionRules() {
				result.value, _ = frame.Lookup(binding.Name)
			}
		}
	}
	e.emit(trace.Event{Kind: "step_end", Status: "ok", Span: step.Span}, nil, result.value)
	return nil
}

// FXQuotes returns the rates acquired in first-use order for the latest rule
// run, including rates acquired before a later failure in that run.
func (e *Evaluator) FXQuotes() []fx.Quote { return slices.Clone(e.fxQuotes) }
