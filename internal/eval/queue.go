package eval

import (
	"math"
	"strings"

	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/trace"
)

const KindQueue Kind = "Queue"

type queueNode struct {
	value Value
	next  *queueNode
}

// A queue shares immutable nodes between versions. The front starts at the
// oldest value, while the back starts at the newest.
type queueValue struct {
	front *queueNode
	back  *queueNode
	count int
}

func (queueValue) Kind() Kind      { return KindQueue }
func (q queueValue) length() int64 { return int64(q.count) }

func (q queueValue) Inspect() string {
	return inspectComposite(q, KindQueue, q.count)
}

func (q queueValue) appendInspection(text *strings.Builder, remaining *int) bool {
	if q.count > *remaining || !writeInspection(text, "queue(") {
		return false
	}
	items := make([]Value, q.count)
	index := 0
	for node := q.front; node != nil; node = node.next {
		items[index] = node.value
		index++
	}
	index = len(items) - 1
	for node := q.back; node != nil; node = node.next {
		items[index] = node.value
		index--
	}
	return (listValue{elements: items}).appendInspection(text, remaining) && writeInspection(text, ")")
}

func (e *Evaluator) queueBuiltin(name string, args []Value, span diagnostic.Span) (Value, error) {
	var result Value
	if name == "queue" {
		result = queueValue{}
	} else {
		q, ok := args[0].(queueValue)
		if !ok {
			return nil, typeError(name, args[0])
		}
		var err error
		switch name {
		case "enqueue":
			result, err = e.enqueue(q, args[1], span)
		case "dequeue":
			result, err = e.dequeue(q, span)
		default:
			return nil, diagnostic.New("E_INTERNAL", "unknown queue builtin")
		}
		if err != nil {
			return nil, err
		}
	}
	e.emit(trace.Event{Kind: name, Span: span}, args, result)
	return result, nil
}

func (e *Evaluator) enqueue(q queueValue, value Value, span diagnostic.Span) (Value, error) {
	if q.count == math.MaxInt {
		return nil, diagnostic.New("E_EXECUTION_LIMIT", "queue size limit exceeded")
	}
	if err := e.chargeCollectionWork(1, span); err != nil {
		return nil, err
	}
	if q.count == 0 {
		q.front = &queueNode{value: value}
	} else {
		q.back = &queueNode{value: value, next: q.back}
	}
	q.count++
	return q, nil
}

func (e *Evaluator) dequeue(q queueValue, span diagnostic.Span) (Value, error) {
	if q.count == 0 {
		return nil, diagnostic.New("E_EMPTY_QUEUE", "cannot dequeue an empty queue")
	}
	head := q.front
	if head == nil {
		// Rebuild the front in FIFO order. The final node of the back stack
		// is the oldest value, which can be returned without copying its node.
		if err := e.chargeCollectionWork(q.length(), span); err != nil {
			return nil, err
		}
		for head = q.back; head.next != nil; head = head.next {
			q.front = &queueNode{value: head.value, next: q.front}
		}
		q.back = nil
	} else {
		if err := e.chargeCollectionWork(1, span); err != nil {
			return nil, err
		}
		q.front = head.next
	}
	q.count--
	return recordValue{fields: []namedValue{{name: "value", value: head.value}, {name: "queue", value: q}}}, nil
}
