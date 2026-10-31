package eval

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/parser"
)

type execution struct {
	value  Value
	env    *Environment
	output string
	err    error
}

func evaluate(source string, limits config.Limits) execution {
	program, err := parser.Parse("test.fin", source, limits)
	if err != nil {
		return execution{err: err}
	}
	var output bytes.Buffer
	env := NewEnvironment(nil)
	evaluator := New(Options{Limits: limits, Stdout: &output})
	value, err := evaluator.Run(program, env)
	return execution{value: value, env: env, output: output.String(), err: err}
}

func TestExpressions(t *testing.T) {
	cases := []struct{ source, want string }{
		{"0.1+0.2", "0.3"},
		{"1.2+0.003", "1.203"},
		{"0.01*0.01", "0.0001"},
		{"-(1.2+0.03)", "-1.23"},
		{"-0", "0"},
		{"1.20==1.2", "true"},
		{"1.2<1.21", "true"},
		{"3>=3.00", "true"},
		{"!false", "true"},
		{"!!true", "true"},
		{"true!=false", "true"},
		{"\"same\"==\"same\"", "true"},
		{"let x=0.1; let y=x+0.2; y;", "0.3"},
	}
	for _, tc := range cases {
		t.Run(tc.source, func(t *testing.T) {
			result := evaluate(tc.source, config.DefaultLimits())
			if result.err != nil {
				t.Fatal(result.err)
			}
			if result.value == nil || result.value.Inspect() != tc.want {
				t.Fatalf("got %v; want %s", result.value, tc.want)
			}
		})
	}
}

func TestNamesAndBranches(t *testing.T) {
	source := "let x=0.1; if(true){let x=0.2; print(x);} else {print(999);} print(x);"
	result := evaluate(source, config.DefaultLimits())
	if result.err != nil {
		t.Fatal(result.err)
	}
	if result.output != "0.2\n0.1\n" {
		t.Fatalf("wrong scope or branch: %q", result.output)
	}
	x, _ := result.env.Lookup("x")
	if x.Inspect() != "0.1" {
		t.Fatal("original variable changed")
	}
}

func TestBuiltins(t *testing.T) {
	cases := []struct{ source, want string }{
		{"round(2.345,2,\"HALF_EVEN\")", "2.34"},
		{"round(2.355,2,\"HALF_EVEN\")", "2.36"},
		{"round(-2.5,0,\"HALF_UP\")", "-3"},
		{"round(-2.5,0,\"TOWARD_ZERO\")", "-2"},
		{"div(1,8,3,\"EXACT\")", "0.125"},
		{"div(-2,3,2,\"HALF_EVEN\")", "-0.67"},
		{"format(round(9.995,2,\"HALF_EVEN\"),2)", "10.00"},
		{"format(1.2,2.0)", "1.20"},
	}
	for _, tc := range cases {
		t.Run(tc.source, func(t *testing.T) {
			result := evaluate(tc.source, config.DefaultLimits())
			if result.err != nil {
				t.Fatal(result.err)
			}
			if result.value.Inspect() != tc.want {
				t.Fatalf("got %q; want %q", result.value.Inspect(), tc.want)
			}
		})
	}
}

func TestLanguageErrors(t *testing.T) {
	cases := []struct {
		source string
		code   diagnostic.Code
	}{
		{"if(1){print(1);}", "E_TYPE_MISMATCH"},
		{"1+true", "E_TYPE_MISMATCH"},
		{"1==\"1\"", "E_TYPE_MISMATCH"},
		{"\"a\"+\"b\"", "E_TYPE_MISMATCH"},
		{"!1", "E_TYPE_MISMATCH"},
		{"missing", "E_UNDEFINED_NAME"},
		{"let x=1;let x=2;", "E_DUPLICATE_BINDING"},
		{"let print=1;print(2);", "E_NOT_CALLABLE"},
		{"round(1,2)", "E_ARITY"},
		{"print()", "E_ARITY"},
		{"round(1,2,\"OTHER\")", "E_ROUNDING_MODE"},
		{"round(1,-1,\"HALF_EVEN\")", "E_INVALID_SCALE"},
		{"round(1,0.5,\"HALF_EVEN\")", "E_INVALID_SCALE"},
		{"round(1,37,\"HALF_EVEN\")", "E_NUMERIC_LIMIT"},
		{"round(\"1\",2,\"HALF_EVEN\")", "E_TYPE_MISMATCH"},
		{"format(1.234,2)", "E_INEXACT"},
		{"div(1,0,2,\"HALF_EVEN\")", "E_DIVISION_BY_ZERO"},
		{"div(1,3,2,\"EXACT\")", "E_INEXACT"},
		{"return 1;", "E_RETURN_OUTSIDE_FUNCTION"},
	}
	for _, tc := range cases {
		t.Run(tc.source, func(t *testing.T) {
			got := evaluate(tc.source, config.DefaultLimits())
			if diagnostic.CodeOf(got.err) != tc.code {
				t.Fatalf("got %v; want %s", got.err, tc.code)
			}
		})
	}
}

func TestFunctionsAndClosure(t *testing.T) {
	cases := []struct{ source, want string }{
		{"let interest=fn(p,r,t){return p*r*t;}; interest(1000000,0.05,0.5);", "25000"},
		{"let make=fn(x){return fn(y){return x+y;};};let add=make(0.1);add(0.2);", "0.3"},
		{"let x=1;let f=fn(){return x;};if(true){let x=2;print(f());}x;", "1"},
		{"let fib=fn(n){if(n<=1){return n;}return fib(n-1)+fib(n-2);};fib(10);", "55"},
	}
	for _, tc := range cases {
		result := evaluate(tc.source, config.DefaultLimits())
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.value.Inspect() != tc.want {
			t.Fatalf("got %s; want %s", result.value.Inspect(), tc.want)
		}
	}
}

func TestFunctionErrors(t *testing.T) {
	cases := []struct {
		source string
		code   diagnostic.Code
	}{
		{"let f=fn(a){return a;};f();", "E_ARITY"},
		{"let f=fn(){let a=1;};f();", "E_MISSING_RETURN"},
		{"let f=fn(a,a){return a;};f(1,2);", "E_DUPLICATE_BINDING"},
		{"let f=fn(){let x=1;return x;};f();x;", "E_UNDEFINED_NAME"},
		{"let f=fn(){return 1;};f==f;", "E_TYPE_MISMATCH"},
	}
	for _, tc := range cases {
		got := evaluate(tc.source, config.DefaultLimits())
		if diagnostic.CodeOf(got.err) != tc.code {
			t.Fatalf("%s: got %v; want %s", tc.source, got.err, tc.code)
		}
	}
}

func TestEvaluationLimits(t *testing.T) {
	limits := config.DefaultLimits()
	limits.MaxCallDepth = 4
	got := evaluate("let forever=fn(){return forever();};forever();", limits)
	if diagnostic.CodeOf(got.err) != "E_EXECUTION_LIMIT" {
		t.Fatalf("unbounded recursion: %v", got.err)
	}
	limits = config.DefaultLimits()
	limits.MaxEvalSteps = 8
	got = evaluate("let x=1;let y=x+2;let z=y+3;z;", limits)
	if diagnostic.CodeOf(got.err) != "E_EXECUTION_LIMIT" {
		t.Fatalf("unbounded evaluation: %v", got.err)
	}
}

func TestNestedErrorSourceLocation(t *testing.T) {
	got := evaluate("let bad=fn(){\n return div(1,0,2,\"HALF_EVEN\");\n};\nbad();", config.DefaultLimits())
	var located *diagnostic.Error
	if !errors.As(got.err, &located) || located.Span == nil || located.Span.Line != 2 {
		t.Fatalf("lost inner error position: %v", got.err)
	}
}

func TestJSONPrintGuardAndOutputError(t *testing.T) {
	program, err := parser.Parse("guard.fin", "print(1)", config.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	e := New(Options{Limits: config.DefaultLimits(), Stdout: &output, JSONMode: true})
	_, err = e.Run(program, NewEnvironment(nil))
	if diagnostic.CodeOf(err) != "E_OUTPUT_SIDE_EFFECT" || output.Len() != 0 {
		t.Fatalf("print guard failed: %v %q", err, output.String())
	}
}

func TestCallEvaluationOrder(t *testing.T) {
	got := evaluate("let mark=fn(x){print(x);return x;};let add=fn(a,b){return a+b;};add(mark(1),mark(2));", config.DefaultLimits())
	if got.err != nil {
		t.Fatal(got.err)
	}
	if got.output != "1\n2\n" || got.value.Inspect() != "3" {
		t.Fatalf("wrong order: %+v", got)
	}
}

func TestLargeScaleArgumentDoesNotAllocate(t *testing.T) {
	source := "format(1," + strings.Repeat("9", 128) + ")"
	got := evaluate(source, config.DefaultLimits())
	if diagnostic.CodeOf(got.err) != "E_NUMERIC_LIMIT" {
		t.Fatalf("unexpected error: %v", got.err)
	}
}
