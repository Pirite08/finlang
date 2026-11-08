package eval

import (
	"fmt"
	"math"
	"strings"

	"github.com/Pirite08/finlang/internal/ast"
	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/decimal"
	"github.com/Pirite08/finlang/internal/diagnostic"
)

const (
	KindList  Kind = "List"
	KindRange Kind = "Range"
)

// The evaluator checks the index before calling elementAt.
type sequence interface {
	Value
	length() int64
	elementAt(int64, config.Limits) (Value, error)
}

// List elements are owned by the list after construction and cannot be changed.
type listValue struct{ elements []Value }

// NewListValue copies a list supplied by a trusted Go caller, such as the
// contract decoder. The interpreter owns the copied slice.
func NewListValue(elements []Value) Value {
	return listValue{elements: append([]Value(nil), elements...)}
}

func (listValue) Kind() Kind      { return KindList }
func (v listValue) length() int64 { return int64(len(v.elements)) }
func (v listValue) elementAt(index int64, _ config.Limits) (Value, error) {
	return v.elements[index], nil
}

func (v listValue) Inspect() string {
	return inspectComposite(v, KindList, len(v.elements))
}

func (v listValue) appendInspection(text *strings.Builder, remaining *int) bool {
	if !writeInspection(text, "[") {
		return false
	}
	for i, element := range v.elements {
		if i > 0 && !writeInspection(text, ", ") {
			return false
		}
		if !appendValueInspection(text, remaining, element) {
			return false
		}
	}
	return writeInspection(text, "]")
}

type rangeValue struct {
	start int64
	stop  int64
	count int64
}

func (rangeValue) Kind() Kind        { return KindRange }
func (v rangeValue) length() int64   { return v.count }
func (v rangeValue) Inspect() string { return fmt.Sprintf("range(%d, %d)", v.start, v.stop) }
func (v rangeValue) elementAt(index int64, limits config.Limits) (Value, error) {
	return integerValue(v.start+index, limits)
}

func integerValue(number int64, limits config.Limits) (Value, error) {
	result, err := decimal.FromInt64(number, limits)
	if err != nil {
		return nil, err
	}
	return DecimalValue{Value: result}, nil
}

func exactInt64(value Value) (int64, bool) {
	number, ok := value.(DecimalValue)
	if !ok {
		return 0, false
	}
	return number.Value.Int64Exact()
}

func (e *Evaluator) collectionBuiltin(name string, args []Value, span diagnostic.Span) (Value, error) {
	switch name {
	case "len":
		if queue, ok := args[0].(queueValue); ok && !e.collectionRules() {
			return integerValue(queue.length(), e.options.Limits)
		}
		value, ok := args[0].(sequence)
		if !ok {
			return nil, typeError(name, args[0])
		}
		return integerValue(value.length(), e.options.Limits)
	case "range":
		start, validStart := exactInt64(args[0])
		stop, validStop := exactInt64(args[1])
		if !validStart || !validStop {
			return nil, diagnostic.New("E_RANGE", "range endpoints must be signed 64-bit integers")
		}
		result := rangeValue{start: start, stop: stop}
		if stop > start {
			if start < 0 && stop > math.MaxInt64+start {
				return nil, diagnostic.New("E_RANGE", "range length exceeds the signed 64-bit limit")
			}
			result.count = stop - start
		}
		if err := e.checkCollectionSize(result.count, span); err != nil {
			return nil, err
		}
		return result, nil
	default:
		return nil, diagnostic.New("E_INTERNAL", "unknown collection builtin")
	}
}

func (e *Evaluator) checkCollectionSize(count int64, span diagnostic.Span) error {
	if !e.collectionRules() {
		return nil
	}
	return checkCollectionSize(count, e.options.Limits.MaxCollectionItems, span)
}

func checkCollectionSize(count int64, maxItems int, span diagnostic.Span) error {
	if count > int64(maxItems) {
		message := fmt.Sprintf("collection has %d items, limit is %d", count, maxItems)
		return diagnostic.WithSpan(diagnostic.New("E_COLLECTION_LIMIT", message), span)
	}
	return nil
}

func (e *Evaluator) list(node *ast.List, env *Environment) (Value, error) {
	// Each element costs at least one step. Use the remaining budget to bound
	// the allocation even when a caller sets a very small execution limit.
	elements := make([]Value, 0, min(len(node.Elements), e.options.Limits.MaxEvalSteps-e.steps))
	for _, element := range node.Elements {
		value, err := e.expression(element, env)
		if err != nil {
			return nil, err
		}
		elements = append(elements, value)
	}
	return listValue{elements: elements}, nil
}

func (e *Evaluator) index(node *ast.Index, env *Environment) (Value, error) {
	collection, err := e.expression(node.Collection, env)
	if err != nil {
		return nil, err
	}
	items, sequential := collection.(sequence)
	keyed, isMap := collection.(mapValue)
	if !sequential && !isMap {
		return nil, typeError("indexing", collection)
	}
	indexValue, err := e.expression(node.Index, env)
	if err != nil {
		return nil, err
	}
	if isMap {
		value, err := keyed.lookup(indexValue)
		return value, diagnostic.WithSpan(err, node.Index.GetSpan())
	}
	index, ok := exactInt64(indexValue)
	if !ok {
		return nil, diagnostic.WithSpan(diagnostic.New("E_INDEX", "index must be an integer in the signed 64-bit range"), node.Index.GetSpan())
	}
	if index < 0 || index >= items.length() {
		message := fmt.Sprintf("index %d is outside the bounds of a collection with length %d", index, items.length())
		return nil, diagnostic.WithSpan(diagnostic.New("E_INDEX", message), node.Index.GetSpan())
	}
	return items.elementAt(index, e.options.Limits)
}

func (e *Evaluator) runFor(node *ast.For, env *Environment) (outcome, error) {
	iterable, err := e.expression(node.Iterable, env)
	if err != nil {
		return outcome{}, err
	}
	items, ok := iterable.(sequence)
	if !ok {
		return outcome{}, typeError("for", iterable)
	}
	for index, length := int64(0), items.length(); index < length; index++ {
		if err := e.tick(node.GetSpan()); err != nil {
			return outcome{}, err
		}
		value, err := items.elementAt(index, e.options.Limits)
		if err != nil {
			return outcome{}, err
		}
		scope := NewEnvironment(env)
		if err := scope.Define(node.Name, value); err != nil {
			return outcome{}, err
		}
		result, err := e.block(node.Body, scope, false)
		if err != nil || result.returned {
			return result, err
		}
	}
	return outcome{value: UnitValue{}}, nil
}
