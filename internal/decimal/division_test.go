package decimal_test

import (
	"example.com/finlang/internal/decimal"
	"example.com/finlang/internal/diagnostic"
	"strings"
	"testing"
)

func TestDivision(t *testing.T) {
	for _, tc := range []struct {
		a, b    string
		digits  uint32
		mode    decimal.Mode
		want    string
		inexact bool
		code    diagnostic.Code
	}{
		{"1", "8", 3, decimal.Exact, "0.125", false, ""},
		{"-2", "3", 2, decimal.HalfEven, "-0.67", true, ""},
		{"2", "-3", 2, decimal.HalfUp, "-0.67", true, ""},
		{"-2", "-3", 2, decimal.TowardZero, "0.66", true, ""},
		{"1", "3", 2, decimal.Exact, "", false, "E_INEXACT"},
		{"1", "0", 2, decimal.HalfEven, "", false, "E_DIVISION_BY_ZERO"},
		{"0", "0", 2, decimal.Exact, "", false, "E_DIVISION_BY_ZERO"},
		{"0", "3", 36, decimal.Exact, "0", false, ""},
		{"0.001", "0.2", 2, decimal.HalfEven, "0", true, ""},
		{"2", "0.001", 0, decimal.Exact, "2000", false, ""},
		{"1", "2", 0, decimal.HalfUp, "1", true, ""},
		{"-1", "2", 0, decimal.HalfEven, "0", true, ""},
	} {
		a, b := mustParse(t, tc.a), mustParse(t, tc.b)
		got, err := decimal.Div(a, b, tc.digits, tc.mode, limits)
		checkCode(t, err, tc.code)
		if err == nil && (got.Value.String() != tc.want || got.Inexact != tc.inexact) {
			t.Errorf("%s/%s -> %+v, want %s,%t", tc.a, tc.b, got, tc.want, tc.inexact)
		}
		if a.String() != mustParse(t, tc.a).String() || b.String() != mustParse(t, tc.b).String() {
			t.Fatal("division mutated operands")
		}
	}
	_, err := decimal.Div(mustParse(t, "1"), mustParse(t, "2"), 37, decimal.HalfEven, limits)
	checkCode(t, err, "E_NUMERIC_LIMIT")
	_, err = decimal.Div(mustParse(t, "1"), mustParse(t, "2"), 2, decimal.Mode("bad"), limits)
	checkCode(t, err, "E_ROUNDING_MODE")
	_, err = decimal.Div(mustParse(t, strings.Repeat("9", 128)), mustParse(t, "0.1"), 0, decimal.Exact, limits)
	checkCode(t, err, "E_NUMERIC_LIMIT")
}
