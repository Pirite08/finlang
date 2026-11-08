package eval

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Pirite08/finlang/internal/decimal"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/trace"
)

type builtinScope uint8

const (
	builtinStandard builtinScope = iota
	builtinFX
	builtinOutput
	// New builtin names can overlap with bindings in existing rules.
	builtinBasic
	builtinCollection
)

type builtinSpec struct {
	scope    builtinScope
	arity    int
	variadic bool
	run      func(*Evaluator, string, []Value, diagnostic.Span) (Value, error)
}

var builtins = map[string]builtinSpec{
	"print":        {scope: builtinOutput, arity: 1, run: (*Evaluator).standardBuiltin},
	"round":        {scope: builtinStandard, arity: 3, run: (*Evaluator).standardBuiltin},
	"div":          {scope: builtinStandard, arity: 4, run: (*Evaluator).standardBuiltin},
	"format":       {scope: builtinStandard, arity: 2, run: (*Evaluator).standardBuiltin},
	"money":        {scope: builtinStandard, arity: 2, run: (*Evaluator).standardBuiltin},
	"settle":       {scope: builtinStandard, arity: 3, run: (*Evaluator).standardBuiltin},
	"format_money": {scope: builtinStandard, arity: 2, run: (*Evaluator).standardBuiltin},
	"rate":         {scope: builtinFX, arity: 2, run: (*Evaluator).fxBuiltin},
	"convert":      {scope: builtinFX, arity: 2, run: (*Evaluator).fxBuiltin},
	"abs":          {scope: builtinBasic, arity: 1, run: (*Evaluator).basicBuiltin},
	"sign":         {scope: builtinBasic, arity: 1, run: (*Evaluator).basicBuiltin},
	"min":          {scope: builtinBasic, arity: 1, variadic: true, run: (*Evaluator).basicBuiltin},
	"max":          {scope: builtinBasic, arity: 1, variadic: true, run: (*Evaluator).basicBuiltin},
	"clamp":        {scope: builtinBasic, arity: 3, run: (*Evaluator).basicBuiltin},
	"sum":          {scope: builtinBasic, arity: 1, variadic: true, run: (*Evaluator).basicBuiltin},
	"floor":        {scope: builtinBasic, arity: 1, run: (*Evaluator).basicBuiltin},
	"ceil":         {scope: builtinBasic, arity: 1, run: (*Evaluator).basicBuiltin},
	"trunc":        {scope: builtinBasic, arity: 1, run: (*Evaluator).basicBuiltin},
	"decimal":      {scope: builtinBasic, arity: 1, run: (*Evaluator).basicBuiltin},
	"amount":       {scope: builtinBasic, arity: 1, run: (*Evaluator).basicBuiltin},
	"currency":     {scope: builtinBasic, arity: 1, run: (*Evaluator).basicBuiltin},
	"len":          {scope: builtinCollection, arity: 1, run: (*Evaluator).collectionBuiltin},
	"range":        {scope: builtinCollection, arity: 2, run: (*Evaluator).collectionBuiltin},
	"all":          {scope: builtinCollection, arity: 1, run: (*Evaluator).listBuiltin},
	"any":          {scope: builtinCollection, arity: 1, run: (*Evaluator).listBuiltin},
	"sorted":       {scope: builtinCollection, arity: 1, run: (*Evaluator).listBuiltin},
	"lower_bound":  {scope: builtinCollection, arity: 2, run: (*Evaluator).listBuiltin},
	"enumerate":    {scope: builtinCollection, arity: 1, run: (*Evaluator).pairBuiltin},
	"zip_exact":    {scope: builtinCollection, arity: 2, run: (*Evaluator).pairBuiltin},
	"matrix":       {scope: builtinCollection, arity: 1, run: (*Evaluator).matrixBuiltin},
	"mat_add":      {scope: builtinCollection, arity: 2, run: (*Evaluator).matrixBuiltin},
	"mat_mul":      {scope: builtinCollection, arity: 2, run: (*Evaluator).matrixBuiltin},
	"transpose":    {scope: builtinCollection, arity: 1, run: (*Evaluator).matrixBuiltin},
	"queue":        {scope: builtinCollection, arity: 0, run: (*Evaluator).queueBuiltin},
	"enqueue":      {scope: builtinCollection, arity: 2, run: (*Evaluator).queueBuiltin},
	"dequeue":      {scope: builtinCollection, arity: 1, run: (*Evaluator).queueBuiltin},
}

func (e *Evaluator) isBuiltin(name string) bool {
	spec, known := builtins[name]
	return known && (spec.scope != builtinFX || e.rule != nil)
}

func arityError(name string, want, got int) error {
	return diagnostic.New("E_ARITY", fmt.Sprintf("%s expects %d arguments; received %d", name, want, got))
}

func (spec builtinSpec) checkArity(name string, count int) error {
	if spec.variadic {
		if count < spec.arity {
			return diagnostic.New("E_ARITY", fmt.Sprintf("%s expects at least %d arguments; received %d", name, spec.arity, count))
		}
	} else if count != spec.arity {
		return arityError(name, spec.arity, count)
	}
	return nil
}

func (e *Evaluator) builtin(name string, args []Value, span diagnostic.Span) (Value, error) {
	spec, known := builtins[name]
	if !known {
		return nil, diagnostic.New("E_INTERNAL", "unknown builtin")
	}
	if spec.scope == builtinFX && e.rule == nil {
		return nil, diagnostic.New("E_FX_CONTEXT", "FX operations require a rule execution")
	}
	if err := spec.checkArity(name, len(args)); err != nil {
		return nil, err
	}
	return spec.run(e, name, args, span)
}

func (e *Evaluator) standardBuiltin(name string, args []Value, span diagnostic.Span) (Value, error) {
	if name == "print" {
		if e.options.JSONMode {
			return nil, diagnostic.New("E_OUTPUT_SIDE_EFFECT", "print is unavailable in JSON result mode")
		}
		if _, err := fmt.Fprintln(e.options.Stdout, args[0].Inspect()); err != nil {
			return nil, diagnostic.New("E_OUTPUT_IO", err.Error())
		}
		return UnitValue{}, nil
	}
	if name == "money" || name == "settle" || name == "format_money" || (name == "div" && args[0].Kind() == KindMoney) {
		return e.moneyBuiltin(name, args, span)
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
		result := StringValue(formatted)
		e.emit(trace.Event{Kind: name, Span: span}, args[:1], result, digits)
		return result, nil
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
	result := DecimalValue{Value: rounded.Value}
	e.emit(trace.Event{Kind: name, Span: span, Mode: string(mode), Inexact: rounded.Inexact}, args[:scaleIndex], result, digits)
	return result, nil
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
