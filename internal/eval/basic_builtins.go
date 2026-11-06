package eval

import (
	"github.com/Pirite08/finlang/internal/decimal"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/money"
	"github.com/Pirite08/finlang/internal/trace"
)

func (e *Evaluator) basicBuiltin(name string, args []Value, span diagnostic.Span) (Value, error) {
	var result Value
	var err error
	switch name {
	case "abs", "sign":
		result, err = e.magnitudeBuiltin(name, args[0])
	case "min", "max", "sum":
		result, err = e.aggregateBuiltin(name, args)
	case "clamp":
		result, err = clamp(args[0], args[1], args[2])
	case "floor", "ceil", "trunc":
		return e.integerBuiltin(name, args, span)
	case "decimal":
		text, ok := args[0].(StringValue)
		if !ok {
			return nil, typeError(name, args[0])
		}
		var value decimal.Decimal
		value, err = decimal.Parse(string(text), e.options.Limits)
		result = DecimalValue{Value: value}
	case "amount", "currency":
		value, ok := args[0].(MoneyValue)
		if !ok {
			return nil, typeError(name, args[0])
		}
		if name == "amount" {
			result = DecimalValue{Value: value.Value.Amount()}
		} else {
			result = StringValue(value.Value.Currency())
		}
	default:
		return nil, diagnostic.New("E_INTERNAL", "unsupported basic builtin")
	}
	if err != nil {
		return nil, err
	}
	e.emit(trace.Event{Kind: name, Span: span}, args, result)
	return result, nil
}

func (e *Evaluator) magnitudeBuiltin(name string, arg Value) (Value, error) {
	var value decimal.Decimal
	switch arg := arg.(type) {
	case DecimalValue:
		value = arg.Value
	case MoneyValue:
		value = arg.Value.Amount()
	default:
		return nil, typeError(name, arg)
	}
	sign := value.Sign()
	if name == "sign" {
		result, err := decimal.FromInt64(int64(sign), e.options.Limits)
		return DecimalValue{Value: result}, err
	}
	if sign >= 0 {
		return arg, nil
	}
	if amount, ok := arg.(MoneyValue); ok {
		zero, err := money.New(amount.Value.Currency(), decimal.Decimal{})
		if err != nil {
			return nil, err
		}
		result, err := money.Sub(zero, amount.Value, e.options.Limits)
		return MoneyValue{Value: result}, err
	}
	result, err := decimal.Sub(decimal.Decimal{}, value, e.options.Limits)
	return DecimalValue{Value: result}, err
}

func (e *Evaluator) aggregateBuiltin(name string, args []Value) (Value, error) {
	result := args[0]
	switch result.(type) {
	case DecimalValue, MoneyValue:
	default:
		return nil, typeError(name, result)
	}
	for _, arg := range args[1:] {
		if name == "sum" {
			if result.Kind() != arg.Kind() {
				return nil, typeError(name, result, arg)
			}
			var err error
			result, err = e.infix("+", result, arg)
			if err != nil {
				return nil, err
			}
			continue
		}
		order, err := compareNumbers(name, arg, result)
		if err != nil {
			return nil, err
		}
		if (name == "min" && order < 0) || (name == "max" && order > 0) {
			result = arg
		}
	}
	return result, nil
}

func compareNumbers(name string, left, right Value) (int, error) {
	switch left := left.(type) {
	case DecimalValue:
		if right, ok := right.(DecimalValue); ok {
			return decimal.Cmp(left.Value, right.Value), nil
		}
	case MoneyValue:
		if right, ok := right.(MoneyValue); ok {
			return money.Cmp(left.Value, right.Value)
		}
	}
	return 0, typeError(name, left, right)
}

func clamp(value, lower, upper Value) (Value, error) {
	order, err := compareNumbers("clamp", lower, upper)
	if err != nil {
		return nil, err
	}
	if order > 0 {
		return nil, diagnostic.New("E_INVALID_RANGE", "clamp lower bound exceeds upper bound")
	}
	order, err = compareNumbers("clamp", value, lower)
	if err != nil {
		return nil, err
	}
	if order < 0 {
		return lower, nil
	}
	order, err = compareNumbers("clamp", value, upper)
	if err != nil {
		return nil, err
	}
	if order > 0 {
		return upper, nil
	}
	return value, nil
}

func (e *Evaluator) integerBuiltin(name string, args []Value, span diagnostic.Span) (Value, error) {
	value, ok := args[0].(DecimalValue)
	if !ok {
		return nil, typeError(name, args[0])
	}
	var rounded decimal.Rounded
	var err error
	switch name {
	case "floor":
		rounded, err = decimal.Floor(value.Value, e.options.Limits)
	case "ceil":
		rounded, err = decimal.Ceil(value.Value, e.options.Limits)
	case "trunc":
		rounded, err = decimal.Round(value.Value, 0, decimal.TowardZero, e.options.Limits)
	}
	if err != nil {
		return nil, err
	}
	result := DecimalValue{Value: rounded.Value}
	e.emit(trace.Event{Kind: name, Span: span, Inexact: rounded.Inexact}, args, result, 0)
	return result, nil
}
