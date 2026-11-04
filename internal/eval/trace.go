package eval

import "github.com/Pirite08/finlang/internal/trace"

// emit serializes values only for an enabled sink. The sink receives no mutable
// runtime values, so recording cannot change the calculation.
func (e *Evaluator) emit(event trace.Event, inputs []Value, result Value) {
	if e.options.Trace == nil {
		return
	}
	e.traceSequence++
	event.Sequence = e.traceSequence
	if e.rule != nil {
		event.Step = e.rule.step
	}
	event.Inputs = make([]string, len(inputs))
	for i, value := range inputs {
		event.Inputs[i] = value.Inspect()
		if m, ok := value.(MoneyValue); ok && event.Currency == "" {
			event.Currency = m.Value.Currency()
		}
	}
	event.Result = result.Inspect()
	if m, ok := result.(MoneyValue); ok {
		event.Currency = m.Value.Currency()
	}
	e.options.Trace.Record(event)
}
