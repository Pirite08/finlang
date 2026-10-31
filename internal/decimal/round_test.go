package decimal_test

import (
	"github.com/Pirite08/finlang/internal/decimal"
	"strings"
	"testing"
)

func TestSignedTies(t *testing.T) {
	for _, tc := range []struct {
		mode decimal.Mode
		want string
	}{{decimal.HalfEven, "-2"}, {decimal.HalfUp, "-3"}, {decimal.TowardZero, "-2"}} {
		got, err := decimal.Round(mustParse(t, "-2.5"), 0, tc.mode, limits)
		if err != nil || got.Value.String() != tc.want || !got.Inexact {
			t.Errorf("-2.5 %s = %+v,%v", tc.mode, got, err)
		}
	}
}

func TestHalfEvenBoundaries(t *testing.T) {
	for _, tc := range []struct{ input, want string }{{"2.345", "2.34"}, {"2.355", "2.36"}, {"-2.345", "-2.34"}, {"9.995", "10"}, {"-9.995", "-10"}, {"0.005", "0"}, {"0.015", "0.02"}, {"-0.005", "0"}} {
		got, err := decimal.Round(mustParse(t, tc.input), 2, decimal.HalfEven, limits)
		if err != nil || got.Value.String() != tc.want || !got.Inexact {
			t.Errorf("%s -> %s,%t,%v; want %s,true", tc.input, got.Value.String(), got.Inexact, err, tc.want)
		}
	}
}

func TestRoundingValidationAndExact(t *testing.T) {
	for _, mode := range []decimal.Mode{decimal.HalfEven, decimal.HalfUp, decimal.TowardZero, decimal.Exact} {
		parsed, err := decimal.ParseMode(string(mode))
		if err != nil || parsed != mode {
			t.Fatal(parsed, err)
		}
		got, err := decimal.Round(mustParse(t, "1.20"), 2, mode, limits)
		if err != nil || got.Value.String() != "1.2" || got.Inexact {
			t.Fatal(got, err)
		}
		var zero decimal.Decimal
		got, err = decimal.Round(zero, 36, mode, limits)
		if err != nil || got.Value.String() != "0" || got.Inexact {
			t.Fatal(got, err)
		}
	}
	for _, invalid := range []string{"", "half_even", "HALF_EVEN ", "ROUND_HALF_EVEN"} {
		_, err := decimal.ParseMode(invalid)
		checkCode(t, err, "E_ROUNDING_MODE")
	}
	_, err := decimal.Round(mustParse(t, "1.21"), 1, decimal.Exact, limits)
	checkCode(t, err, "E_INEXACT")
	_, err = decimal.Round(mustParse(t, "0"), 37, decimal.HalfEven, limits)
	checkCode(t, err, "E_NUMERIC_LIMIT")
	_, err = decimal.Round(mustParse(t, "0"), 0, decimal.Mode("bad"), limits)
	checkCode(t, err, "E_ROUNDING_MODE")
	custom := limits
	custom.MaxCoefficientDigits = 2
	_, err = decimal.Round(mustParse(t, "99.5"), 0, decimal.HalfUp, custom)
	checkCode(t, err, "E_NUMERIC_LIMIT")
	x := mustParse(t, "2.355")
	alias := x
	_, err = decimal.Round(x, 2, decimal.HalfEven, limits)
	if err != nil || x.String() != "2.355" || alias.String() != "2.355" {
		t.Fatal("round mutated input", x, alias, err)
	}
	_, err = decimal.Round(mustParse(t, strings.Repeat("9", 128)), 0, decimal.Exact, limits)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDoubleRounding(t *testing.T) {
	x := mustParse(t, "1.2451")
	first, err := decimal.Round(x, 3, decimal.HalfEven, limits)
	if err != nil {
		t.Fatal(err)
	}
	second, err := decimal.Round(first.Value, 2, decimal.HalfEven, limits)
	if err != nil {
		t.Fatal(err)
	}
	direct, err := decimal.Round(x, 2, decimal.HalfEven, limits)
	if err != nil {
		t.Fatal(err)
	}
	if first.Value.String() != "1.245" || second.Value.String() != "1.24" || direct.Value.String() != "1.25" {
		t.Fatal(first, second, direct)
	}
}
