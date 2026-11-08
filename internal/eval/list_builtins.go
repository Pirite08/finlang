package eval

import (
	"slices"

	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/trace"
)

func (e *Evaluator) listBuiltin(name string, args []Value, span diagnostic.Span) (Value, error) {
	list, ok := args[0].(listValue)
	if !ok {
		return nil, typeError(name, args[0])
	}
	var result Value
	var err error
	switch name {
	case "all", "any":
		result, err = e.listTruth(name, list.elements, span)
	case "sorted":
		result, err = e.sortedList(list.elements, span)
	case "lower_bound":
		result, err = e.lowerBound(list.elements, args[1], span)
	default:
		return nil, diagnostic.New("E_INTERNAL", "unknown list builtin")
	}
	if err != nil {
		return nil, err
	}
	e.emit(trace.Event{Kind: name, Span: span}, args, result)
	return result, nil
}

func (e *Evaluator) listTruth(name string, values []Value, span diagnostic.Span) (Value, error) {
	all := name == "all"
	result := all
	for _, value := range values {
		if err := e.tick(span); err != nil {
			return nil, err
		}
		truth, ok := value.(BoolValue)
		if !ok {
			return nil, typeError(name, value)
		}
		if all {
			result = result && bool(truth)
		} else {
			result = result || bool(truth)
		}
	}
	return BoolValue(result), nil
}

// validateNumberList checks all items against the numeric kind and currency of
// first. When ascending is set, each pair of neighbors must also be ordered.
func (e *Evaluator) validateNumberList(name string, values []Value, first Value, ascending bool, span diagnostic.Span) error {
	if len(values) == 0 {
		_, err := compareNumbers(name, first, first)
		return err
	}
	previous := first
	for i, value := range values {
		if err := e.tick(span); err != nil {
			return err
		}
		order, err := compareNumbers(name, previous, value)
		if err != nil {
			return err
		}
		if ascending && i > 0 && order > 0 {
			return diagnostic.New("E_UNSORTED_LIST", "lower_bound requires an ascending list")
		}
		previous = value
	}
	return nil
}

func (e *Evaluator) sortedList(values []Value, span diagnostic.Span) (Value, error) {
	if len(values) == 0 {
		return listValue{}, nil
	}
	if err := e.validateNumberList("sorted", values, values[0], false, span); err != nil {
		return nil, err
	}
	result := slices.Clone(values)
	if len(result) > 1 {
		if err := e.mergeSort(result, make([]Value, len(result)), span); err != nil {
			return nil, err
		}
	}
	return listValue{elements: result}, nil
}

func (e *Evaluator) mergeSort(values, scratch []Value, span diagnostic.Span) error {
	if len(values) < 2 {
		return nil
	}
	mid := len(values) / 2
	left, right := values[:mid], values[mid:]
	if err := e.mergeSort(left, scratch[:mid], span); err != nil {
		return err
	}
	if err := e.mergeSort(right, scratch[mid:], span); err != nil {
		return err
	}
	for i := range scratch {
		if err := e.tick(span); err != nil {
			return err
		}
		useLeft := len(right) == 0
		if len(left) != 0 && len(right) != 0 {
			order, err := compareNumbers("sorted", left[0], right[0])
			if err != nil {
				return err
			}
			useLeft = order <= 0
		}
		if useLeft {
			scratch[i], left = left[0], left[1:]
		} else {
			scratch[i], right = right[0], right[1:]
		}
	}
	copy(values, scratch)
	return nil
}

func (e *Evaluator) lowerBound(values []Value, key Value, span diagnostic.Span) (Value, error) {
	if err := e.validateNumberList("lower_bound", values, key, true, span); err != nil {
		return nil, err
	}
	low, high := 0, len(values)
	for low < high {
		if err := e.tick(span); err != nil {
			return nil, err
		}
		mid := low + (high-low)/2
		order, err := compareNumbers("lower_bound", values[mid], key)
		if err != nil {
			return nil, err
		}
		if order < 0 {
			low = mid + 1
		} else {
			high = mid
		}
	}
	return integerValue(int64(low), e.options.Limits)
}
