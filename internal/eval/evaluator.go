package eval

import (
	"fmt"
	"io"

	"github.com/Pirite08/finlang/internal/ast"
	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/decimal"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/fx"
	"github.com/Pirite08/finlang/internal/trace"
)

type Options struct {
	Limits          config.Limits
	Stdout          io.Writer
	JSONMode        bool
	Trace           trace.Sink
	FXProvider      fx.Provider
	RuleOnly        bool
	CollectionRules bool
	FXSnapshot      *fx.Snapshot
}

type Evaluator struct {
	options       Options
	steps         int
	callDepth     int
	traceSequence int
	rule          *ruleExecution
	fxQuotes      []fx.Quote
}

func New(options Options) *Evaluator {
	if options.Limits == (config.Limits{}) {
		options.Limits = config.DefaultLimits()
	}
	if options.RuleOnly && options.CollectionRules {
		options.Limits = config.RuleCollectionLimits(options.Limits)
	}
	if options.Stdout == nil {
		options.Stdout = io.Discard
	}
	return &Evaluator{options: options}
}

func (e *Evaluator) collectionRules() bool { return e.options.RuleOnly && e.options.CollectionRules }

type outcome struct {
	value    Value
	returned bool
}

func (e *Evaluator) Run(program *ast.Program, env *Environment) (Value, error) {
	if e.options.RuleOnly {
		return nil, diagnostic.New("E_RULE_POLICY", "rule-only mode requires RunRule with a rule declaration")
	}
	if program.Rule != nil {
		return nil, diagnostic.WithSpan(diagnostic.New("E_RULE_INPUT_REQUIRED", "use RunRule to supply a rule input"), program.Rule.Span)
	}
	result, err := e.statements(program.Statements, env)
	if err != nil {
		return nil, err
	}
	return result.value, nil
}

func (e *Evaluator) tick(span diagnostic.Span) error {
	if e.steps >= e.options.Limits.MaxEvalSteps {
		return diagnostic.WithSpan(diagnostic.New("E_EXECUTION_LIMIT", "evaluation step limit exceeded"), span)
	}
	e.steps++
	return nil
}

func (e *Evaluator) statements(statements []ast.Stmt, env *Environment) (outcome, error) {
	result := outcome{value: UnitValue{}}
	for _, statement := range statements {
		next, err := e.statement(statement, env)
		if err != nil {
			return outcome{}, err
		}
		result = next
		if next.returned {
			return result, nil
		}
	}
	return result, nil
}

func (e *Evaluator) block(block *ast.Block, env *Environment, newScope bool) (outcome, error) {
	if err := e.tick(block.GetSpan()); err != nil {
		return outcome{}, err
	}
	if newScope {
		env = NewEnvironment(env)
	}
	return e.statements(block.Statements, env)
}

func (e *Evaluator) statement(node ast.Stmt, env *Environment) (result outcome, err error) {
	if err = e.tick(node.GetSpan()); err != nil {
		return outcome{}, err
	}
	defer func() {
		if err != nil {
			err = diagnostic.WithSpan(err, node.GetSpan())
		}
	}()
	switch node := node.(type) {
	case *ast.Let:
		return e.bind(node.Name, node.Value, env, false, node.GetSpan())
	case *ast.Var:
		return e.bind(node.Name, node.Value, env, true, node.GetSpan())
	case *ast.Assign:
		scope, err := env.assignmentScope(node.Name)
		if err != nil {
			return outcome{}, err
		}
		value, err := e.expression(node.Value, env)
		if err != nil {
			return outcome{}, err
		}
		scope.values[node.Name] = value
		if e.collectionRules() {
			e.emit(trace.Event{Kind: "assignment", Span: node.GetSpan(), Binding: node.Name}, nil, value)
		}
		return outcome{value: UnitValue{}}, nil
	case *ast.For:
		return e.runFor(node, env)
	case *ast.Require:
		condition, err := e.expression(node.Condition, env)
		if err != nil {
			return outcome{}, err
		}
		truth, ok := condition.(BoolValue)
		if !ok {
			return outcome{}, typeError("require condition", condition)
		}
		e.emit(trace.Event{Kind: "require", Span: node.GetSpan()}, nil, truth)
		if !truth {
			return outcome{}, diagnostic.New("E_REQUIRE_FAILED", "required condition evaluated to false")
		}
		return outcome{value: truth}, nil
	case *ast.ExpressionStatement:
		value, err := e.expression(node.Expression, env)
		return outcome{value: value}, err
	case *ast.Return:
		if e.callDepth == 0 {
			return outcome{}, diagnostic.New("E_RETURN_OUTSIDE_FUNCTION", "return is only valid inside a function")
		}
		value, err := e.expression(node.Value, env)
		return outcome{value: value, returned: true}, err
	case *ast.If:
		truth, err := e.ifCondition(node.Condition, env)
		if err != nil {
			return outcome{}, err
		}
		var branch *ast.Block
		if truth {
			branch = node.Consequence
		} else {
			branch = node.Alternative
		}
		if branch != nil {
			result, err := e.block(branch, env, true)
			if err != nil || result.returned {
				return result, err
			}
		}
		return outcome{value: UnitValue{}}, nil
	}
	return outcome{}, diagnostic.New("E_INTERNAL", "unsupported statement node")
}

func (e *Evaluator) bind(name string, initializer ast.Expr, env *Environment, mutable bool, span diagnostic.Span) (outcome, error) {
	if _, exists := env.values[name]; exists {
		return outcome{}, diagnostic.New("E_DUPLICATE_BINDING", fmt.Sprintf("name %q is already defined in this scope", name))
	}
	value, err := e.expression(initializer, env)
	if err != nil {
		return outcome{}, err
	}
	if err := env.define(name, value, mutable); err != nil {
		return outcome{}, err
	}
	if e.rule != nil && e.rule.step != "" {
		e.emit(trace.Event{Kind: "binding", Span: span, Binding: name}, nil, value)
	}
	return outcome{value: UnitValue{}}, nil
}

func (e *Evaluator) ifCondition(node ast.Expr, env *Environment) (BoolValue, error) {
	condition, err := e.expression(node, env)
	if err != nil {
		return false, err
	}
	truth, ok := condition.(BoolValue)
	if !ok {
		return false, typeError("if condition", condition)
	}
	e.emit(trace.Event{Kind: "branch", Span: node.GetSpan(), Branch: truth.Inspect()}, []Value{condition}, condition)
	return truth, nil
}

func (e *Evaluator) expression(node ast.Expr, env *Environment) (value Value, err error) {
	if err = e.tick(node.GetSpan()); err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			err = diagnostic.WithSpan(err, node.GetSpan())
		}
	}()
	switch node := node.(type) {
	case *ast.Number:
		number, err := decimal.Parse(node.Literal, e.options.Limits)
		if err != nil {
			return nil, err
		}
		return DecimalValue{Value: number}, nil
	case *ast.String:
		return StringValue(node.Value), nil
	case *ast.Bool:
		return BoolValue(node.Value), nil
	case *ast.List:
		return e.list(node, env)
	case *ast.Record:
		return e.record(node, env)
	case *ast.Map:
		return e.mapLiteral(node, env)
	case *ast.Index:
		return e.index(node, env)
	case *ast.Member:
		return e.member(node, env)
	case *ast.IfExpr:
		truth, err := e.ifCondition(node.Condition, env)
		if err != nil {
			return nil, err
		}
		if truth {
			return e.expression(node.Consequence, env)
		}
		return e.expression(node.Alternative, env)
	case *ast.Identifier:
		if value, ok := env.Lookup(node.Name); ok {
			return value, nil
		}
		if !e.options.RuleOnly && e.isBuiltin(node.Name) {
			return builtinValue{name: node.Name}, nil
		}
		return nil, diagnostic.New("E_UNDEFINED_NAME", fmt.Sprintf("name %q is not defined", node.Name))
	case *ast.Prefix:
		right, err := e.expression(node.Right, env)
		if err != nil {
			return nil, err
		}
		if node.Operator == "!" {
			if right, ok := right.(BoolValue); ok {
				result := BoolValue(!bool(right))
				e.emit(trace.Event{Kind: "prefix:!", Span: node.GetSpan()}, []Value{right}, result)
				return result, nil
			}
		}
		if node.Operator == "-" {
			if right, ok := right.(DecimalValue); ok {
				negative, err := decimal.Sub(decimal.Decimal{}, right.Value, e.options.Limits)
				if err != nil {
					return nil, err
				}
				result := DecimalValue{Value: negative}
				e.emit(trace.Event{Kind: "prefix:-", Span: node.GetSpan()}, []Value{right}, result)
				return result, nil
			}
		}
		return nil, typeError("prefix "+node.Operator, right)
	case *ast.Infix:
		left, err := e.expression(node.Left, env)
		if err != nil {
			return nil, err
		}
		if node.Operator == "&&" || node.Operator == "||" {
			return e.logical(node, left, env)
		}
		right, err := e.expression(node.Right, env)
		if err != nil {
			return nil, err
		}
		if !e.collectionRules() && left.Kind() == KindMatrix && right.Kind() == KindMatrix {
			switch node.Operator {
			case "+":
				return e.matrixBuiltin("mat_add", []Value{left, right}, node.GetSpan())
			case "*":
				return e.matrixBuiltin("mat_mul", []Value{left, right}, node.GetSpan())
			}
		}
		result, err := e.infix(node.Operator, left, right)
		if err == nil {
			e.emit(trace.Event{Kind: node.Operator, Span: node.GetSpan()}, []Value{left, right}, result)
		}
		return result, err
	case *ast.Function:
		names := make(map[string]bool, len(node.Parameters))
		for _, name := range node.Parameters {
			if names[name] {
				return nil, diagnostic.New("E_DUPLICATE_BINDING", fmt.Sprintf("parameter %q is repeated", name))
			}
			names[name] = true
		}
		return &FunctionValue{parameters: append([]string(nil), node.Parameters...), body: node.Body, environment: env}, nil
	case *ast.Call:
		if member, ok := node.Function.(*ast.Member); ok && (!e.options.RuleOnly || e.collectionRules()) {
			return e.callMethod(member, node.Arguments, env, node.GetSpan())
		}
		var function Value
		if e.options.RuleOnly {
			callee, ok := node.Function.(*ast.Identifier)
			if !ok || callee == nil {
				return nil, rulePolicyError(node.Span, "only direct calls are allowed")
			}
			if err := e.tick(callee.Span); err != nil {
				return nil, err
			}
			function = builtinValue{name: callee.Name}
		} else {
			function, err = e.expression(node.Function, env)
			if err != nil {
				return nil, err
			}
		}
		arguments := make([]Value, 0, len(node.Arguments))
		for _, argument := range node.Arguments {
			value, err := e.expression(argument, env)
			if err != nil {
				return nil, err
			}
			arguments = append(arguments, value)
		}
		return e.Call(function, arguments, node.GetSpan())
	}
	return nil, diagnostic.New("E_INTERNAL", "unsupported expression node")
}

func (e *Evaluator) logical(node *ast.Infix, left Value, env *Environment) (Value, error) {
	truth, ok := left.(BoolValue)
	if !ok {
		return nil, diagnostic.WithSpan(typeError(node.Operator, left), node.Left.GetSpan())
	}
	event := trace.Event{Kind: node.Operator, Span: node.GetSpan()}
	if (node.Operator == "&&" && !truth) || (node.Operator == "||" && truth) {
		e.emit(event, []Value{left}, truth)
		return truth, nil
	}
	right, err := e.expression(node.Right, env)
	if err != nil {
		return nil, err
	}
	result, ok := right.(BoolValue)
	if !ok {
		return nil, diagnostic.WithSpan(typeError(node.Operator, left, right), node.Right.GetSpan())
	}
	e.emit(event, []Value{left, right}, result)
	return result, nil
}

func typeError(operation string, values ...Value) error {
	kinds := make([]Kind, len(values))
	for i, value := range values {
		kinds[i] = value.Kind()
	}
	return diagnostic.New("E_TYPE_MISMATCH", fmt.Sprintf("%s does not accept %v", operation, kinds))
}

func (e *Evaluator) infix(operator string, left, right Value) (Value, error) {
	if left.Kind() == KindMoney || right.Kind() == KindMoney {
		return e.moneyInfix(operator, left, right)
	}
	if left.Kind() != right.Kind() {
		return nil, typeError(operator, left, right)
	}
	switch left := left.(type) {
	case DecimalValue:
		other := right.(DecimalValue)
		var result decimal.Decimal
		var err error
		switch operator {
		case "+":
			result, err = decimal.Add(left.Value, other.Value, e.options.Limits)
		case "-":
			result, err = decimal.Sub(left.Value, other.Value, e.options.Limits)
		case "*":
			result, err = decimal.Mul(left.Value, other.Value, e.options.Limits)
		default:
			comparison := decimal.Cmp(left.Value, other.Value)
			switch operator {
			case "==":
				return BoolValue(comparison == 0), nil
			case "!=":
				return BoolValue(comparison != 0), nil
			case "<":
				return BoolValue(comparison < 0), nil
			case "<=":
				return BoolValue(comparison <= 0), nil
			case ">":
				return BoolValue(comparison > 0), nil
			case ">=":
				return BoolValue(comparison >= 0), nil
			}
			return nil, typeError(operator, left, right)
		}
		if err != nil {
			return nil, err
		}
		return DecimalValue{Value: result}, nil
	case BoolValue:
		equal := left == right.(BoolValue)
		if operator == "==" {
			return BoolValue(equal), nil
		}
		if operator == "!=" {
			return BoolValue(!equal), nil
		}
	case StringValue:
		equal := left == right.(StringValue)
		if operator == "==" {
			return BoolValue(equal), nil
		}
		if operator == "!=" {
			return BoolValue(!equal), nil
		}
	}
	return nil, typeError(operator, left, right)
}
