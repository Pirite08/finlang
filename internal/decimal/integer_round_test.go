package decimal_test

import (
	"testing"

	"github.com/Pirite08/finlang/internal/decimal"
)

func TestDirectionalIntegerRounding(t *testing.T) {
	for _, tc := range []struct {
		input, floor, ceil string
		sign               int
		inexact            bool
	}{
		{"-2.5", "-3", "-2", -1, true},
		{"2.5", "2", "3", 1, true},
		{"-0.01", "-1", "0", -1, true},
		{"0.01", "0", "1", 1, true},
		{"-2", "-2", "-2", -1, false},
		{"0", "0", "0", 0, false},
	} {
		t.Run(tc.input, func(t *testing.T) {
			input := mustParse(t, tc.input)
			floor, err := decimal.Floor(input, limits)
			if err != nil || floor.Value.String() != tc.floor || floor.Inexact != tc.inexact {
				t.Fatalf("floor = %v, %v; want %s (inexact %t)", floor, err, tc.floor, tc.inexact)
			}
			ceil, err := decimal.Ceil(input, limits)
			if err != nil || ceil.Value.String() != tc.ceil || ceil.Inexact != tc.inexact {
				t.Fatalf("ceil = %v, %v; want %s (inexact %t)", ceil, err, tc.ceil, tc.inexact)
			}
			if input.String() != tc.input || input.Sign() != tc.sign {
				t.Fatalf("input = %s (sign %d), want %s (sign %d)", input, input.Sign(), tc.input, tc.sign)
			}
		})
	}
}

func TestDirectionalIntegerRoundingChecksCarry(t *testing.T) {
	narrow := limits
	narrow.MaxCoefficientDigits = 2
	_, err := decimal.Floor(mustParse(t, "-99.1"), narrow)
	checkCode(t, err, "E_NUMERIC_LIMIT")
	_, err = decimal.Ceil(mustParse(t, "99.1"), narrow)
	checkCode(t, err, "E_NUMERIC_LIMIT")
}
