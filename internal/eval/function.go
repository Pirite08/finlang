package eval

import (
	"fmt"

	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/trace"
)

// Call shares this evaluator's step and call budget with Run.
func (e *Evaluator) Call(function Value, args []Value, span diagnostic.Span) (value Value, err error) {
	defer func() {
		if err != nil {
			err = diagnostic.WithSpan(err, span)
		}
	}()
	builtin, isBuiltin := function.(builtinValue)
	if e.options.RuleOnly && (e.rule == nil || !isBuiltin || !ruleBuiltin(builtin.name, e.collectionRules())) {
		return nil, diagnostic.New("E_RULE_POLICY", "only approved calculation functions can be called in a rule")
	}
	if isBuiltin {
		return e.builtin(builtin.name, args, span)
	}
	closure, ok := function.(*FunctionValue)
	if !ok {
		return nil, diagnostic.New("E_NOT_CALLABLE", fmt.Sprintf("%s is not callable", function.Kind()))
	}
	if len(args) != len(closure.parameters) {
		return nil, arityError("function", len(closure.parameters), len(args))
	}
	if e.callDepth >= e.options.Limits.MaxCallDepth {
		return nil, diagnostic.New("E_EXECUTION_LIMIT", "function call depth limit exceeded")
	}
	frame := NewEnvironment(closure.environment)
	for i, name := range closure.parameters {
		if err := frame.Define(name, args[i]); err != nil {
			return nil, err
		}
	}
	e.callDepth++
	defer func() { e.callDepth-- }()
	result, err := e.block(closure.body, frame, false)
	if err != nil {
		return nil, err
	}
	if !result.returned {
		return nil, diagnostic.New("E_MISSING_RETURN", "function reached its end without return")
	}
	e.emit(trace.Event{Kind: "call", Span: span}, args, result.value)
	return result.value, nil
}
