package eval

import (
	"bytes"
	"testing"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/parser"
	"github.com/Pirite08/finlang/internal/trace"
)

func traced(source string, cap int) (execution, *trace.Recorder) {
	limits := config.DefaultLimits()
	program, err := parser.Parse("trace.fin", source, limits)
	if err != nil {
		return execution{err: err}, nil
	}
	var output bytes.Buffer
	recorder := trace.NewRecorder(cap)
	env := NewEnvironment(nil)
	value, err := New(Options{Limits: limits, Stdout: &output, Trace: recorder}).Run(program, env)
	return execution{value: value, env: env, output: output.String(), err: err}, recorder
}

func TestTraceReflectsExecutedCalculation(t *testing.T) {
	source := "let x=money(\"USD\",0.335);\nif(true){\n print(format_money(settle(x,2,\"HALF_EVEN\"),2));\n}else{print(999);}\ndiv(1,8,3,\"EXACT\");"
	result, recorder := traced(source, 100)
	if result.err != nil {
		t.Fatal(result.err)
	}
	if result.output != "USD 0.34\n" || result.value.Inspect() != "0.125" {
		t.Fatalf("wrong execution: %+v", result)
	}
	events := recorder.Events()
	var foundBranch, foundSettlement, foundDivision bool
	for i, event := range events {
		if event.Sequence != i+1 {
			t.Fatal("unstable sequence")
		}
		if event.Kind == "branch" {
			foundBranch = true
			if event.Branch != "true" {
				t.Fatalf("recorded unexecuted branch: %+v", event)
			}
		}
		if event.Kind == "settle" {
			foundSettlement = true
			if len(event.Inputs) != 1 || event.Inputs[0] != "USD 0.335" || event.Result != "USD 0.34" ||
				event.Currency != "USD" || event.Mode != "HALF_EVEN" || !event.Inexact ||
				event.TargetScale == nil || *event.TargetScale != 2 || event.Span.Line != 3 {
				t.Fatalf("missing actual settlement facts: %+v", event)
			}
		}
		if event.Kind == "div" {
			foundDivision = true
			if len(event.Inputs) != 2 || event.Inputs[0] != "1" || event.Inputs[1] != "8" ||
				event.Result != "0.125" || event.Inexact || event.Mode != "EXACT" {
				t.Fatalf("missing actual division facts: %+v", event)
			}
		}
	}
	if !foundBranch || !foundSettlement || !foundDivision {
		t.Fatalf("events absent: %+v", events)
	}
}

func TestTraceDoesNotChangeValues(t *testing.T) {
	source := "let x=money(\"USD\",0.335);let y=x+x+x;format_money(settle(y,2,\"HALF_EVEN\"),2);"
	plain := evaluate(source, config.DefaultLimits())
	full, fullRecorder := traced(source, 10000)
	capped, recorder := traced(source, 1)
	if plain.err != nil || full.err != nil || capped.err != nil {
		t.Fatalf("errors %v %v %v", plain.err, full.err, capped.err)
	}
	if full.value.Inspect() != plain.value.Inspect() || capped.value.Inspect() != plain.value.Inspect() || capped.value.Inspect() != "USD 1.00" {
		t.Fatal("trace changed calculation result")
	}
	if len(fullRecorder.Events()) == 0 || len(recorder.Events()) != 1 || !recorder.Truncated() {
		t.Fatal("trace limit not observed")
	}
}

type traceInspectionCounter struct{ count int }

func (*traceInspectionCounter) Kind() Kind { return KindString }
func (v *traceInspectionCounter) Inspect() string {
	v.count++
	return "value"
}

func TestTraceSkipsInspectionAfterLimit(t *testing.T) {
	recorder := trace.NewRecorder(1)
	evaluator := New(Options{Trace: recorder})
	value := &traceInspectionCounter{}
	event := trace.Event{Kind: "test"}
	evaluator.emit(event, []Value{value}, value, 0)
	if value.count != 2 || recorder.Truncated() {
		t.Fatalf("at limit: inspections = %d, truncated = %t", value.count, recorder.Truncated())
	}
	if scale := recorder.Events()[0].TargetScale; scale == nil || *scale != 0 {
		t.Fatalf("recorded target scale = %v, want an explicit zero", scale)
	}

	evaluator.emit(event, []Value{value}, value, 0)
	if value.count != 2 || !recorder.Truncated() {
		t.Fatalf("past limit: inspections = %d, truncated = %t", value.count, recorder.Truncated())
	}
}

func TestTraceContainsActualMoneyArithmetic(t *testing.T) {
	result, recorder := traced("money(\"KRW\",1000)*0.05;", 100)
	if result.err != nil {
		t.Fatal(result.err)
	}
	for _, event := range recorder.Events() {
		if event.Kind == "*" && event.Result == "KRW 50" && event.Currency == "KRW" && len(event.Inputs) == 2 &&
			event.Inputs[0] == "KRW 1000" && event.Inputs[1] == "0.05" {
			return
		}
	}
	t.Fatalf("actual arithmetic event absent: %+v", recorder.Events())
}

func TestTraceRecordsFunctionResult(t *testing.T) {
	result, recorder := traced("let f=fn(x){return settle(money(\"USD\",x),2,\"HALF_EVEN\");};f(0.335);", 100)
	if result.err != nil {
		t.Fatal(result.err)
	}
	for _, event := range recorder.Events() {
		if event.Kind == "call" && len(event.Inputs) == 1 && event.Inputs[0] == "0.335" && event.Result == "USD 0.34" {
			return
		}
	}
	t.Fatal("actual function call result is missing")
}
