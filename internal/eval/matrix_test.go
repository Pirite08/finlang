package eval

import (
	"testing"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
)

func TestMatrixOperations(t *testing.T) {
	cases := []struct{ name, source, want string }{
		{"exact addition", `mat_add(matrix([[0.1, 2]]), matrix([[0.2, -2]]))`, "matrix([[0.3, 0]])"},
		{"rectangular multiplication", `mat_mul(matrix([[1, 2, 3], [4, 5, 6]]), matrix([[7, 8], [9, 10], [11, 12]]))`, "matrix([[58, 64], [139, 154]])"},
		{"exact multiplication", `mat_mul(matrix([[0.1, 0.2]]), matrix([[0.2], [0.3]]))`, "matrix([[0.08]])"},
		{"transpose", `transpose(matrix([[1, 2, 3], [4, 5, 6]]))`, "matrix([[1, 4], [2, 5], [3, 6]])"},
		{"immutable input", `let a = matrix([[1, 2]]); mat_add(a, matrix([[3, 4]])); transpose(a); a;`, "matrix([[1, 2]])"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := evaluate(tc.source, config.DefaultLimits())
			if result.err != nil || result.value == nil || result.value.Kind() != "Matrix" || result.value.Inspect() != tc.want {
				t.Fatalf("got %v, error %v; want %s", result.value, result.err, tc.want)
			}
		})
	}
}

func TestMatrixErrors(t *testing.T) {
	cases := []struct {
		name, source string
		code         diagnostic.Code
	}{
		{"no rows", `matrix([])`, "E_MATRIX_SHAPE"},
		{"no columns", `matrix([[]])`, "E_MATRIX_SHAPE"},
		{"ragged rows", `matrix([[1, 2], [3]])`, "E_MATRIX_SHAPE"},
		{"scalar row", `matrix([[1], 2])`, "E_TYPE_MISMATCH"},
		{"money cell", `matrix([[money("KRW", 1)]])`, "E_TYPE_MISMATCH"},
		{"addition shape", `mat_add(matrix([[1, 2]]), matrix([[3], [4]]))`, "E_MATRIX_SHAPE"},
		{"multiplication shape", `mat_mul(matrix([[1, 2]]), matrix([[3, 4]]))`, "E_MATRIX_SHAPE"},
		{"transpose type", `transpose([[1, 2]])`, "E_TYPE_MISMATCH"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := evaluate(tc.source, config.DefaultLimits())
			if diagnostic.CodeOf(result.err) != tc.code {
				t.Fatalf("got %v; want %s", result.err, tc.code)
			}
		})
	}
}

func TestMatrixLimits(t *testing.T) {
	limits := config.DefaultLimits()
	limits.MaxScale = 1
	if result := evaluate(`mat_mul(matrix([[0.1]]), matrix([[0.1]]))`, limits); diagnostic.CodeOf(result.err) != "E_NUMERIC_LIMIT" {
		t.Fatalf("got %v; want E_NUMERIC_LIMIT", result.err)
	}

	left := evaluate(`matrix([[1], [2]])`, config.DefaultLimits())
	right := evaluate(`matrix([[3, 4]])`, config.DefaultLimits())
	if left.err != nil || right.err != nil {
		t.Fatalf("matrix fixtures failed: %v, %v", left.err, right.err)
	}
	limits = config.DefaultLimits()
	limits.MaxEvalSteps = 3
	e := New(Options{Limits: limits})
	if _, err := e.builtin("mat_mul", []Value{left.value, right.value}, diagnostic.Span{}); diagnostic.CodeOf(err) != "E_EXECUTION_LIMIT" {
		t.Fatalf("got %v; want E_EXECUTION_LIMIT", err)
	}
}
