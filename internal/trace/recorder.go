package trace

// Recorder retains the first maxEvents events without affecting evaluation.
// It is intended for one evaluator execution and is not safe for concurrent use.
type Recorder struct {
	maxEvents int
	events    []Event
	truncated bool
}

// NewRecorder creates an empty recorder. Nonpositive limits retain no events.
func NewRecorder(maxEvents int) *Recorder {
	return &Recorder{maxEvents: maxEvents}
}

// Record assigns a contiguous sequence and takes ownership of a copy of event.
// An attempted event beyond the limit marks truncation and is discarded.
func (r *Recorder) Record(event Event) {
	if len(r.events) >= r.maxEvents {
		r.truncated = true
		return
	}
	event.Sequence = len(r.events) + 1
	r.events = append(r.events, cloneEvent(event))
}

// Events returns an independent snapshot of the retained events.
func (r *Recorder) Events() []Event {
	result := make([]Event, len(r.events))
	for i, event := range r.events {
		result[i] = cloneEvent(event)
	}
	return result
}

// Truncated reports whether at least one event was discarded.
func (r *Recorder) Truncated() bool { return r.truncated }

func cloneEvent(event Event) Event {
	if event.Inputs != nil {
		inputs := make([]string, len(event.Inputs))
		copy(inputs, event.Inputs)
		event.Inputs = inputs
	}
	if event.TargetScale != nil {
		scale := *event.TargetScale
		event.TargetScale = &scale
	}
	return event
}
