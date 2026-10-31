package eval

import (
	"fmt"
	"strconv"
	"strings"

	"example.com/finlang/internal/decimal"
	"example.com/finlang/internal/diagnostic"
)

func isBuiltin(name string) bool {
	switch name {
	case "print", "round", "div", "format":
		return true
	}
	return false
}

func arityError(name string, want, got int) error {
	return diagnostic.New("E_ARITY", fmt.Sprintf("%s expects %d arguments; received %d", name, want, got))
}

func (e *Evaluator) builtin(name string, args []Value) (Value, error) {
	expected := map[string]int{"print": 1, "round": 3, "div": 4, "format": 2}
	if len(args) != expected[name] {
		return nil, arityError(name, expected[name], len(args))
	}
	if name == "print" {
		if e.options.JSONMode {
			return nil, diagnostic.New("E_OUTPUT_SIDE_EFFECT", "print is unavailable in JSON result mode")
		}
		if _, err := fmt.Fprintln(e.options.Stdout, args[0].Inspect()); err != nil {
			return nil, diagnostic.New("E_OUTPUT_IO", err.Error())
		}
		return UnitValue{}, nil
	}
	first, ok := args[0].(DecimalValue)
	if !ok {
		return nil, typeError(name, args[0])
	}
	if name == "format" {
		digits, err := e.scale(args[1])
		if err != nil {
			return nil, err
		}
		formatted, err := decimal.FormatAt(first.Value, digits, e.options.Limits)
		if err != nil {
			return nil, err
		}
		return StringValue(formatted), nil
	}
	scaleIndex := 1
	if name == "div" {
		scaleIndex = 2
	}
	digits, err := e.scale(args[scaleIndex])
	if err != nil {
		return nil, err
	}
	modeText, ok := args[scaleIndex+1].(StringValue)
	if !ok {
		return nil, typeError(name+" rounding mode", args[scaleIndex+1])
	}
	mode, err := decimal.ParseMode(string(modeText))
	if err != nil {
		return nil, err
	}
	var rounded decimal.Rounded
	if name == "round" {
		rounded, err = decimal.Round(first.Value, digits, mode, e.options.Limits)
	} else {
		divisor, ok := args[1].(DecimalValue)
		if !ok {
			return nil, typeError("div", args[1])
		}
		rounded, err = decimal.Div(first.Value, divisor.Value, digits, mode, e.options.Limits)
	}
	if err != nil {
		return nil, err
	}
	return DecimalValue{Value: rounded.Value}, nil
}

func (e *Evaluator) scale(value Value) (uint32, error) {
	number, ok := value.(DecimalValue)
	if !ok {
		return 0, typeError("scale", value)
	}
	text := number.Value.String()
	if strings.HasPrefix(text, "-") || strings.Contains(text, ".") {
		return 0, diagnostic.New("E_INVALID_SCALE", "scale must be a non-negative integer")
	}
	result, err := strconv.ParseUint(text, 10, 32)
	if err != nil || result > uint64(e.options.Limits.MaxScale) {
		return 0, diagnostic.New("E_NUMERIC_LIMIT", fmt.Sprintf("scale must be between 0 and %d", e.options.Limits.MaxScale))
	}
	return uint32(result), nil
}
