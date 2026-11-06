package decimal

import (
	"testing"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
)

func TestBorrowedCoefficientsRemainUnchanged(t *testing.T) {
	limits := config.DefaultLimits()
	value, err := Parse("-12.3500", limits)
	if err != nil {
		t.Fatal(err)
	}
	divisor, err := Parse("-2", limits)
	if err != nil {
		t.Fatal(err)
	}
	alias := value
	quotient, err := Div(value, divisor, 1, HalfEven, limits)
	if err != nil || quotient.Value.String() != "6.2" {
		t.Fatalf("division = %v, %v", quotient, err)
	}
	if value.String() != "-12.35" || alias.String() != "-12.35" || divisor.String() != "-2" {
		t.Fatal("division or formatting changed an existing coefficient", value, alias, divisor)
	}
}

func TestUnchangedRoundReusesCoefficient(t *testing.T) {
	limits := config.DefaultLimits()
	value, err := Parse("-123.45", limits)
	if err != nil {
		t.Fatal(err)
	}
	rounded, err := Round(value, 2, Exact, limits)
	if err != nil || rounded.Inexact || rounded.Value.String() != "-123.45" {
		t.Fatalf("unchanged round = %v, %v", rounded, err)
	}
	if rounded.Value.coefficient != value.coefficient {
		t.Fatal("unchanged round copied an immutable coefficient")
	}
	limits.MaxCoefficientDigits = 4
	if _, err := Round(value, 2, Exact, limits); diagnostic.CodeOf(err) != "E_NUMERIC_LIMIT" {
		t.Fatalf("unchanged round with tighter coefficient limit = %v", err)
	}
}
