package experiments

import (
	"math/big"
	"math/rand"
	"strconv"
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/decimal"
	"github.com/Pirite08/finlang/internal/diagnostic"
)

func addDigitText(left, right string) (string, error) {
	limits := config.DefaultLimits()
	a, err := parseDigitDecimal(left, limits)
	if err != nil {
		return "", err
	}
	b, err := parseDigitDecimal(right, limits)
	if err != nil {
		return "", err
	}
	sum, err := addDigitDecimal(a, b, limits)
	return sum.String(), err
}

// These anchors catch lost carries, sign mistakes, scale alignment and signed zero.
func TestDigitStringAdditionAnchors(t *testing.T) {
	for _, tc := range []struct{ left, right, want string }{
		{"0.1", "0.2", "0.3"},
		{"1.2", "0.003", "1.203"},
		{"19.95", "0.05", "20"},
		{"-12.30", "2.3", "-10"},
		{"2.3", "-12.30", "-10"},
		{"-0.001", "0.0010", "0"},
		{"-9.99", "-0.01", "-10"},
		{"0.001", "-0.0009", "0.0001"},
		{"00012.0300", "-0", "12.03"},
		{strings.Repeat("9", 100) + ".99", "0.01", "1" + strings.Repeat("0", 100)},
	} {
		got, err := addDigitText(tc.left, tc.right)
		if err != nil || got != tc.want {
			t.Errorf("%s + %s = %q, %v; want %s", tc.left, tc.right, got, err, tc.want)
		}
	}
}

func TestDigitStringValidationAndLimits(t *testing.T) {
	limits := config.DefaultLimits()
	for _, text := range []string{"", "-", ".5", "1.", "+1", "1e3", "NaN", " 1", "1 ", "١", "1.2.3", string([]byte{0xff})} {
		_, err := parseDigitDecimal(text, limits)
		if diagnostic.CodeOf(err) != "E_INVALID_NUMBER" {
			t.Errorf("parse %q: %v; want E_INVALID_NUMBER", text, err)
		}
	}
	for _, text := range []string{strings.Repeat("0", 257), strings.Repeat("9", 129), "0." + strings.Repeat("0", 36) + "1"} {
		_, err := parseDigitDecimal(text, limits)
		if diagnostic.CodeOf(err) != "E_NUMERIC_LIMIT" {
			t.Errorf("parse %q: %v; want E_NUMERIC_LIMIT", text, err)
		}
	}
	for _, text := range []string{strings.Repeat("9", 128), "0." + strings.Repeat("0", 35) + "1", "1." + strings.Repeat("0", 100)} {
		if _, err := parseDigitDecimal(text, limits); err != nil {
			t.Errorf("parse boundary %q: %v", text, err)
		}
	}
	if _, err := addDigitText(strings.Repeat("9", 128), "1"); diagnostic.CodeOf(err) != "E_NUMERIC_LIMIT" {
		t.Errorf("carry to 129 digits: %v; want E_NUMERIC_LIMIT", err)
	}
}

func TestDigitStringAdditionRatAgreement(t *testing.T) {
	rng := rand.New(rand.NewSource(20261031))
	for i := 0; i < 10000; i++ {
		makeInput := func() string {
			text := strconv.FormatInt(rng.Int63n(2000000001)-1000000000, 10)
			scale := rng.Intn(7)
			if scale == 0 {
				return text
			}
			negative := strings.HasPrefix(text, "-")
			text = strings.TrimPrefix(text, "-")
			text = strings.Repeat("0", max(0, scale+1-len(text))) + text
			text = text[:len(text)-scale] + "." + text[len(text)-scale:]
			if negative {
				text = "-" + text
			}
			return text
		}
		left, right := makeInput(), makeInput()
		got, err := addDigitText(left, right)
		if err != nil {
			t.Fatalf("pair %d: %v", i, err)
		}
		a, _ := new(big.Rat).SetString(left)
		b, _ := new(big.Rat).SetString(right)
		want := new(big.Rat).Add(a, b)
		actual, ok := new(big.Rat).SetString(got)
		if !ok || actual.Cmp(want) != 0 {
			t.Fatalf("pair %d: %s + %s = %s; want %s", i, left, right, got, want.RatString())
		}
	}
}

func TestAdditionWorkloads(t *testing.T) {
	limits := config.DefaultLimits()
	for _, workload := range additionWorkloads() {
		t.Run(workload.name, func(t *testing.T) {
			var exact decimal.Decimal
			var direct digitDecimal
			var approximate float64
			oracle := new(big.Rat)
			for _, text := range workload.inputs {
				d, err := decimal.Parse(text, limits)
				if err != nil {
					t.Fatal(err)
				}
				exact, err = decimal.Add(exact, d, limits)
				if err != nil {
					t.Fatal(err)
				}
				s, err := parseDigitDecimal(text, limits)
				if err != nil {
					t.Fatal(err)
				}
				before := s.String()
				direct, err = addDigitDecimal(direct, s, limits)
				if err != nil || s.String() != before {
					t.Fatalf("direct addition: %v; operand changed=%t", err, s.String() != before)
				}
				v, err := strconv.ParseFloat(text, 64)
				if err != nil {
					t.Fatal(err)
				}
				approximate += v
				r, ok := new(big.Rat).SetString(text)
				if !ok {
					t.Fatal("invalid independent reference input")
				}
				oracle.Add(oracle, r)
			}
			for name, got := range map[string]string{"scaled_integer": exact.String(), "digit_string": direct.String()} {
				r, ok := new(big.Rat).SetString(got)
				if !ok || r.Cmp(oracle) != 0 || got != workload.want {
					t.Errorf("%s=%s, want anchor %s and rational %s", name, got, workload.want, oracle.RatString())
				}
			}
			value, err := executeAdditionProgram(additionProgram(workload.inputs), limits)
			if err != nil || value.Inspect() != workload.want {
				t.Fatalf("whole execution=%v, %v; want %s", value, err, workload.want)
			}
			difference := new(big.Rat).Sub(new(big.Rat).SetFloat64(approximate), oracle)
			t.Logf("inputs=%d; exact=%s; float64=%s; float64-minus-exact=%s", len(workload.inputs), workload.want, strconv.FormatFloat(approximate, 'g', -1, 64), difference.RatString())
		})
	}
}
