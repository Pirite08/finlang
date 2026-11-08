package eval

import (
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
)

func TestQueueFIFOAndPersistence(t *testing.T) {
	const source = `
let initial = enqueue(enqueue(enqueue(queue(), 1), 2), 3);
let first = dequeue(initial);
let withFourth = first.queue.enqueue(4);
let second = withFourth.dequeue();
let third = second.queue.dequeue();
let fourth = third.queue.dequeue();
[first.value, second.value, third.value, fourth.value, initial.len(), fourth.queue.len()];
`
	result := evaluate(source, config.DefaultLimits())
	if result.err != nil || result.value == nil || result.value.Inspect() != "[1, 2, 3, 4, 3, 0]" {
		t.Fatalf("got %v, error %v", result.value, result.err)
	}
}

func TestQueueErrors(t *testing.T) {
	cases := []struct {
		source string
		code   diagnostic.Code
	}{
		{`dequeue(queue())`, "E_EMPTY_QUEUE"},
		{`dequeue(dequeue(enqueue(queue(), 1)).queue)`, "E_EMPTY_QUEUE"},
		{`enqueue([], 1)`, "E_TYPE_MISMATCH"},
		{`dequeue(1)`, "E_TYPE_MISMATCH"},
	}
	for _, tc := range cases {
		t.Run(tc.source, func(t *testing.T) {
			if result := evaluate(tc.source, config.DefaultLimits()); diagnostic.CodeOf(result.err) != tc.code {
				t.Fatalf("got %v; want %s", result.err, tc.code)
			}
		})
	}
}

func TestQueueBoundedInspection(t *testing.T) {
	small := evaluate(`enqueue(enqueue(enqueue(queue(), 1), 2), 3)`, config.DefaultLimits())
	if small.err != nil || small.value == nil || small.value.Inspect() != "queue([1, 2, 3])" {
		t.Fatalf("got %v, error %v", small.value, small.err)
	}
	nested := evaluate(`var q = queue(); for i in range(0, 30) { q = enqueue(enqueue(queue(), q), q); } q;`, config.DefaultLimits())
	if nested.err != nil {
		t.Fatal(nested.err)
	}
	if text := nested.value.Inspect(); len(text) > 4096 || !strings.Contains(text, "...") {
		t.Fatalf("expected a bounded display, got %q", text)
	}
}

func TestQueueChargesBackStackReversal(t *testing.T) {
	result := evaluate(`let q = enqueue(enqueue(enqueue(enqueue(queue(), 1), 2), 3), 4); dequeue(q).queue;`, config.DefaultLimits())
	if result.err != nil || result.value == nil {
		t.Fatalf("could not prepare queue: %v", result.err)
	}
	limits := config.DefaultLimits()
	limits.MaxEvalSteps = 2
	limited := New(Options{Limits: limits})
	if _, err := limited.queueBuiltin("dequeue", []Value{result.value}, diagnostic.Span{}); diagnostic.CodeOf(err) != "E_EXECUTION_LIMIT" {
		t.Fatalf("got %v; want E_EXECUTION_LIMIT before reversal", err)
	}
	unlimited := New(Options{Limits: config.DefaultLimits()})
	next, err := unlimited.queueBuiltin("dequeue", []Value{result.value}, diagnostic.Span{})
	if err != nil || next.Inspect() != "record {value: 2, queue: queue([3, 4])}" || result.value.Inspect() != "queue([2, 3, 4])" {
		t.Fatalf("got %v, error %v; original queue %v", next, err, result.value)
	}
}
