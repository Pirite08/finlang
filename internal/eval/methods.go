package eval

import (
	"fmt"
	"maps"
	"slices"

	"github.com/Pirite08/finlang/internal/ast"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/trace"
)

var builtinMethods = map[Kind][]string{
	KindList:    {"len", "sum", "all", "any", "sorted", "lower_bound", "enumerate", "zip_exact"},
	KindRange:   {"len", "enumerate", "zip_exact"},
	KindDecimal: {"abs", "round", "format"},
	KindMoney:   {"amount", "currency", "settle"},
	KindString:  {"decimal"},
	KindMatrix:  {"transpose"},
	KindQueue:   {"len", "enqueue", "dequeue"},
}

var mapMethodArities = map[string]int{
	"len":  0,
	"has":  1,
	"get":  1,
	"with": 2,
	"keys": 0,
}

func (e *Evaluator) callMethod(member *ast.Member, args []ast.Expr, env *Environment, span diagnostic.Span) (Value, error) {
	if e.options.RuleOnly && (!e.collectionRules() || !ruleMapMethod(member.Name)) {
		return nil, rulePolicyError(span, "only approved Map methods may be called in a rule")
	}
	if err := e.tick(member.GetSpan()); err != nil {
		return nil, err
	}
	receiver, err := e.expression(member.Receiver, env)
	if err != nil {
		return nil, err
	}
	mapping, isMap := receiver.(mapValue)
	if e.collectionRules() && !isMap {
		return nil, rulePolicyError(span, "a rule method receiver must be a Map")
	}
	var arity int
	var allowed bool
	if isMap {
		arity, allowed = mapMethodArities[member.Name]
	} else if slices.Contains(builtinMethods[receiver.Kind()], member.Name) {
		arity, allowed = builtins[member.Name].arity-1, true
	}
	method := string(receiver.Kind()) + "." + member.Name
	if !allowed {
		return nil, diagnostic.New("E_UNKNOWN_METHOD", fmt.Sprintf("%s has no method %q", receiver.Kind(), member.Name))
	}
	if len(args) != arity {
		return nil, arityError(method, arity, len(args))
	}
	values := make([]Value, len(args)+1)
	values[0] = receiver
	for index, argument := range args {
		value, err := e.expression(argument, env)
		if err != nil {
			return nil, err
		}
		values[index+1] = value
	}
	if !isMap {
		return e.builtin(member.Name, values, span)
	}
	result, err := e.mapMethod(mapping, member.Name, values[1:], span)
	if err != nil {
		return nil, err
	}
	e.emit(trace.Event{Kind: method, Span: span}, values, result)
	return result, nil
}

func (e *Evaluator) mapMethod(mapping mapValue, name string, args []Value, span diagnostic.Span) (Value, error) {
	switch name {
	case "len":
		return integerValue(int64(len(mapping.keys)), e.options.Limits)
	case "keys":
		count := int64(len(mapping.keys))
		if err := e.checkCollectionSize(count, span); err != nil {
			return nil, err
		}
		if err := e.chargeCollectionWork(count, span); err != nil {
			return nil, err
		}
		elements := make([]Value, len(mapping.keys))
		for index, key := range mapping.keys {
			elements[index] = StringValue(key)
		}
		return listValue{elements: elements}, nil
	case "get":
		return mapping.lookup(args[0])
	case "has", "with":
		key, ok := args[0].(StringValue)
		if !ok {
			return nil, typeError("Map."+name, args[0])
		}
		_, exists := mapping.values[string(key)]
		if name == "has" {
			return BoolValue(exists), nil
		}
		if !exists {
			if err := e.checkCollectionSize(int64(len(mapping.keys))+1, span); err != nil {
				return nil, err
			}
		}
		if err := e.chargeCollectionWork(int64(len(mapping.keys))+1, span); err != nil {
			return nil, err
		}
		keys := mapping.keys
		if !exists {
			keys = make([]string, len(mapping.keys)+1)
			copy(keys, mapping.keys)
			keys[len(mapping.keys)] = string(key)
		}
		values := make(map[string]Value, len(keys))
		maps.Copy(values, mapping.values)
		values[string(key)] = args[1]
		return mapValue{keys: keys, values: values}, nil
	default:
		return nil, diagnostic.New("E_INTERNAL", "unknown map method")
	}
}

func (e *Evaluator) pairBuiltin(name string, args []Value, span diagnostic.Span) (Value, error) {
	items, ok := args[0].(sequence)
	if !ok {
		return nil, typeError(name, args[0])
	}
	count := items.length()
	firstField, secondField := "index", "value"
	var other sequence
	if name == "zip_exact" {
		other, ok = args[1].(sequence)
		if !ok {
			return nil, typeError(name, args[1])
		}
		if count != other.length() {
			return nil, diagnostic.New("E_LENGTH_MISMATCH", fmt.Sprintf("zip_exact requires equal lengths; received %d and %d", count, other.length()))
		}
		firstField, secondField = "left", "right"
	}
	if err := e.checkCollectionSize(count, span); err != nil {
		return nil, err
	}
	if count != 0 {
		if err := e.checkCollectionSize(2, span); err != nil {
			return nil, err
		}
	}
	if err := e.chargeCollectionWork(count, span); err != nil {
		return nil, err
	}
	elements := make([]Value, int(count))
	for index := range elements {
		item, err := items.elementAt(int64(index), e.options.Limits)
		if err != nil {
			return nil, err
		}
		first, second := item, item
		if other == nil {
			first, err = integerValue(int64(index), e.options.Limits)
		} else {
			second, err = other.elementAt(int64(index), e.options.Limits)
		}
		if err != nil {
			return nil, err
		}
		elements[index] = recordValue{fields: []namedValue{{name: firstField, value: first}, {name: secondField, value: second}}}
	}
	result := listValue{elements: elements}
	e.emit(trace.Event{Kind: name, Span: span}, args, result)
	return result, nil
}

// Charge native collection work before allocating, including lengths of lazy ranges.
func (e *Evaluator) chargeCollectionWork(count int64, span diagnostic.Span) error {
	if count > int64(e.options.Limits.MaxEvalSteps-e.steps) {
		return diagnostic.WithSpan(diagnostic.New("E_EXECUTION_LIMIT", "evaluation step limit exceeded"), span)
	}
	e.steps += int(count)
	return nil
}
