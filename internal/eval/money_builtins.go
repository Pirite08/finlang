package eval

import (
	"github.com/Pirite08/finlang/internal/decimal"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/money"
	"github.com/Pirite08/finlang/internal/trace"
)

func (e *Evaluator) moneyBuiltin(name string, args []Value, span diagnostic.Span) (Value, error) {
	if name == "money" {
		currency, ok := args[0].(StringValue)
		if !ok {
			return nil, typeError("money currency", args[0])
		}
		amount, ok := args[1].(DecimalValue)
		if !ok {
			return nil, typeError("money amount", args[1])
		}
		value, err := money.New(string(currency), amount.Value)
		if err != nil {
			return nil, err
		}
		result := MoneyValue{Value: value}
		e.emit(trace.Event{Kind: name, Span: span}, args, result)
		return result, nil
	}
	value, ok := args[0].(MoneyValue)
	if !ok {
		return nil, typeError(name, args[0])
	}
	scaleIndex := 1
	if name == "div" {
		scaleIndex = 2
	}
	digits, err := e.scale(args[scaleIndex])
	if err != nil {
		return nil, err
	}
	if name == "format_money" {
		result, err := money.FormatAt(value.Value, digits, e.options.Limits)
		if err != nil {
			return nil, err
		}
		formatted := StringValue(result)
		e.emit(trace.Event{Kind: name, Span: span}, args[:1], formatted, digits)
		return formatted, nil
	}
	modeText, ok := args[scaleIndex+1].(StringValue)
	if !ok {
		return nil, typeError(name+" rounding mode", args[scaleIndex+1])
	}
	mode, err := decimal.ParseMode(string(modeText))
	if err != nil {
		return nil, err
	}
	var result money.Money
	var inexact bool
	if name == "settle" {
		result, inexact, err = money.Settle(value.Value, digits, mode, e.options.Limits)
	} else {
		divisor, ok := args[1].(DecimalValue)
		if !ok {
			return nil, typeError("div", args[1])
		}
		result, inexact, err = money.Div(value.Value, divisor.Value, digits, mode, e.options.Limits)
	}
	if err != nil {
		return nil, err
	}
	wrapped := MoneyValue{Value: result}
	e.emit(trace.Event{Kind: name, Span: span, Mode: string(mode), Inexact: inexact}, args[:scaleIndex], wrapped, digits)
	return wrapped, nil
}
