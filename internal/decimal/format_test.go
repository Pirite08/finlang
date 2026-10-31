package decimal_test

import (
	"example.com/finlang/internal/decimal"
	"testing"
)

func TestFormatExact(t *testing.T) {
	for _, tc := range []struct {
		input  string
		digits uint32
		want   string
	}{{"1.2", 2, "1.20"}, {"0", 2, "0.00"}, {"-0.003", 4, "-0.0030"}, {"100", 0, "100"}, {"1.2300", 2, "1.23"}} {
		got, err := decimal.FormatAt(mustParse(t, tc.input), tc.digits, limits)
		if err != nil || got != tc.want {
			t.Errorf("FormatAt(%s,%d) = %q,%v; want %q", tc.input, tc.digits, got, err, tc.want)
		}
	}
	_, err := decimal.FormatAt(mustParse(t, "1.234"), 2, limits)
	checkCode(t, err, "E_INEXACT")
	_, err = decimal.FormatAt(mustParse(t, "0"), 37, limits)
	checkCode(t, err, "E_NUMERIC_LIMIT")
}
