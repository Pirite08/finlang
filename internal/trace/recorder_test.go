package trace_test

import (
	"fmt"
	"testing"

	"github.com/Pirite08/finlang/internal/trace"
)

func TestRecorderRetainsFirstEventsAndAssignsSequence(t *testing.T) {
	for _, limit := range []int{3, 10000} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			recorder := trace.NewRecorder(limit)
			for i := 0; i < limit; i++ {
				recorder.Record(trace.Event{Sequence: 999, Kind: "add", Result: fmt.Sprint(i)})
			}
			if recorder.Truncated() {
				t.Fatal("reaching the cap must not mark the trace truncated")
			}
			recorder.Record(trace.Event{Kind: "overflow", Result: "discarded"})
			events := recorder.Events()
			if len(events) != limit || !recorder.Truncated() {
				t.Fatalf("retained %d events, truncated=%v; want %d, true", len(events), recorder.Truncated(), limit)
			}
			for i, event := range events {
				if event.Sequence != i+1 || event.Kind != "add" || event.Result != fmt.Sprint(i) {
					t.Fatalf("event %d: %+v", i, event)
				}
			}
		})
	}
}

func TestRecorderNonpositiveLimitTruncatesOnlyAfterRecord(t *testing.T) {
	for _, limit := range []int{-1, 0} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			recorder := trace.NewRecorder(limit)
			if recorder.Truncated() || len(recorder.Events()) != 0 {
				t.Fatal("new recorder must be empty and untruncated")
			}
			recorder.Record(trace.Event{Kind: "add"})
			if !recorder.Truncated() || len(recorder.Events()) != 0 {
				t.Fatal("record attempt must mark truncation without retaining events")
			}
		})
	}
}

func TestRecorderOwnsMutableEventData(t *testing.T) {
	scale := uint32(2)
	inputs := []string{"1.005"}
	recorder := trace.NewRecorder(2)
	recorder.Record(trace.Event{Kind: "round", Inputs: inputs, TargetScale: &scale, Result: "1"})
	inputs[0], scale = "changed", 9
	first := recorder.Events()
	if first[0].Inputs[0] != "1.005" || *first[0].TargetScale != 2 {
		t.Fatalf("Record borrowed caller data: %+v", first[0])
	}
	first[0].Inputs[0], *first[0].TargetScale, first[0].Kind = "changed again", 8, "changed"
	second := recorder.Events()
	if second[0].Inputs[0] != "1.005" || *second[0].TargetScale != 2 || second[0].Kind != "round" {
		t.Fatalf("Events exposed recorder data: %+v", second[0])
	}
}
