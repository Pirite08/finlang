package diagnostic

import (
	"fmt"
	"testing"
)

func TestDiagnosticCodeAndSpan(t *testing.T) {
	original := New("E_INEXACT", "rounding would lose value")
	span := Span{File: "example.fin", Start: 4, End: 8, Line: 2, Column: 3}
	err := WithSpan(fmt.Errorf("evaluation: %w", original), span)
	if CodeOf(err) != "E_INEXACT" {
		t.Fatalf("lost error code: %v", err)
	}
	if original.Span != nil {
		t.Fatal("WithSpan mutated its original error")
	}
	if err.Error() != "example.fin:2:3: E_INEXACT: rounding would lose value" {
		t.Fatalf("missing location or message: %v", err)
	}
}
