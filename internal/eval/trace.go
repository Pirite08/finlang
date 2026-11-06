package eval

import "github.com/Pirite08/finlang/internal/trace"

// emit serializes values and an optional target scale only for a sink that will
// record the event. The sink receives no mutable runtime values.
func (e *Evaluator) emit(event trace.Event, inputs []Value, result Value, targetScale ...uint32) {
	sink := e.options.Trace
	if sink == nil {
		return
	}
	if recorder, ok := sink.(*trace.Recorder); ok && !recorder.AcceptEvent() {
		return
	}
	e.traceSequence++
	event.Sequence = e.traceSequence
	if e.rule != nil {
		event.Step = e.rule.step
	}
	if len(targetScale) > 0 {
		scale := targetScale[0]
		event.TargetScale = &scale
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
	sink.Record(event)
}
