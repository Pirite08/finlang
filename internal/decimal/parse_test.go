package decimal_test

import (
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/decimal"
	"github.com/Pirite08/finlang/internal/diagnostic"
)

var limits = config.DefaultLimits()

func mustParse(t testing.TB, s string) decimal.Decimal {
	t.Helper()
	d, err := decimal.Parse(s, limits)
	if err != nil {
		t.Fatalf("Parse(%q): %v", s, err)
	}
	return d
}

func checkCode(t testing.TB, err error, want diagnostic.Code) {
	t.Helper()
	if got := diagnostic.CodeOf(err); got != want {
		t.Fatalf("error = %v (%s), want %s", err, got, want)
	}
}

func TestParseCanonical(t *testing.T) {
	for input, want := range map[string]string{"12.30": "12.3", "-0": "0", "-0.003": "-0.003", "00012.0300": "12.03", "0.0000": "0", "1000": "1000"} {
		if got := mustParse(t, input).String(); got != want {
			t.Errorf("Parse(%q) = %s, want %s", input, got, want)
		}
	}
	var z decimal.Decimal
	if z.String() != "0" {
		t.Fatalf("zero value = %s", z.String())
	}
}

func TestParseRejectsInvalid(t *testing.T) {
	for _, input := range []string{"", ".5", "1.", "1e3", "1,000", "NaN", " 1", "1 ", "--1", "+1", "++1", "١", "1.2.3", "-"} {
		_, err := decimal.Parse(input, limits)
		checkCode(t, err, "E_INVALID_NUMBER")
	}
	_, err := decimal.Parse(strings.Repeat("0", 257), limits)
	checkCode(t, err, "E_NUMERIC_LIMIT")
}

func TestDecimalBoundaries(t *testing.T) {
	mustParse(t, strings.Repeat("9", 128))
	_, err := decimal.Parse(strings.Repeat("9", 129), limits)
	checkCode(t, err, "E_NUMERIC_LIMIT")
	mustParse(t, "0."+strings.Repeat("0", 35)+"1")
	_, err = decimal.Parse("0."+strings.Repeat("0", 36)+"1", limits)
	checkCode(t, err, "E_NUMERIC_LIMIT")
	// Limits apply after normalization, while token bytes apply to the source.
	if got := mustParse(t, "1."+strings.Repeat("0", 100)).String(); got != "1" {
		t.Fatal(got)
	}
	custom := limits
	custom.MaxCoefficientDigits = 2
	custom.MaxScale = 1
	custom.MaxTokenBytes = 10
	_, err = decimal.Parse("123", custom)
	checkCode(t, err, "E_NUMERIC_LIMIT")
	_, err = decimal.Parse("0.01", custom)
	checkCode(t, err, "E_NUMERIC_LIMIT")
}

func FuzzDecimalParse(f *testing.F) {
	for _, s := range []string{"0", "-0.003", "12.30", ".5", "1e3", strings.Repeat("9", 128), "0." + strings.Repeat("0", 36) + "1"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 512 {
			t.Skip()
		}
		d, err := decimal.Parse(s, limits)
		if err != nil {
			return
		}
		again, err := decimal.Parse(d.String(), limits)
		if err != nil || again.String() != d.String() {
			t.Fatalf("canonical round trip %q -> %q: %v", s, d.String(), err)
		}
	})
}
