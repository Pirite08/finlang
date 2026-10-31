package decimal_test

import (
	"math/big"
	"math/rand"
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/decimal"
	"github.com/Pirite08/finlang/internal/diagnostic"
)

func TestArithmeticExact(t *testing.T) {
	for _, tc := range []struct{ op, a, b, want string }{{"+", "0.1", "0.2", "0.3"}, {"+", "1.2", "0.003", "1.203"}, {"*", "0.01", "0.01", "0.0001"}, {"-", "1.2", "1.203", "-0.003"}, {"*", "-0.01", "-0.01", "0.0001"}, {"+", "-5", "5", "0"}} {
		var got decimal.Decimal
		var err error
		a, b := mustParse(t, tc.a), mustParse(t, tc.b)
		switch tc.op {
		case "+":
			got, err = decimal.Add(a, b, limits)
		case "-":
			got, err = decimal.Sub(a, b, limits)
		case "*":
			got, err = decimal.Mul(a, b, limits)
		}
		if err != nil || got.String() != tc.want {
			t.Errorf("%s%s%s = %s,%v; want %s", tc.a, tc.op, tc.b, got.String(), err, tc.want)
		}
	}
	for _, tc := range []struct {
		a, b string
		want int
	}{{"1.20", "1.2", 0}, {"-1.2", "-1.199", -1}, {"0.0001", "0", 1}, {"100", "99.999", 1}} {
		if got := decimal.Cmp(mustParse(t, tc.a), mustParse(t, tc.b)); got != tc.want {
			t.Errorf("Cmp(%s,%s)=%d", tc.a, tc.b, got)
		}
	}
	var z decimal.Decimal
	v, err := decimal.Add(z, mustParse(t, "1.2"), limits)
	if err != nil || v.String() != "1.2" {
		t.Fatal(v, err)
	}
}

func TestOperandsRemainUnchanged(t *testing.T) {
	a, b := mustParse(t, "123.4500"), mustParse(t, "-0.06")
	alias := a
	for _, op := range []func(decimal.Decimal, decimal.Decimal, config.Limits) (decimal.Decimal, error){decimal.Add, decimal.Sub, decimal.Mul} {
		result, err := op(a, b, limits)
		if err != nil {
			t.Fatal(err)
		}
		_, err = decimal.Add(result, a, limits)
		if err != nil {
			t.Fatal(err)
		}
		if a.String() != "123.45" || alias.String() != "123.45" || b.String() != "-0.06" {
			t.Fatal("operand mutated", a, b, alias)
		}
	}
}

func TestArithmeticLimits(t *testing.T) {
	tiny := mustParse(t, "0."+strings.Repeat("0", 35)+"1")
	if got, err := decimal.Mul(tiny, mustParse(t, "1"), limits); err != nil || got.String() != tiny.String() {
		t.Fatal(got, err)
	}
	_, err := decimal.Mul(tiny, mustParse(t, "0.1"), limits)
	checkCode(t, err, "E_NUMERIC_LIMIT")
	_, err = decimal.Add(mustParse(t, strings.Repeat("9", 128)), mustParse(t, "1"), limits)
	checkCode(t, err, "E_NUMERIC_LIMIT")
	// Raw product scale is 37; trailing zero normalization makes scale 36 legal.
	a := mustParse(t, "0."+strings.Repeat("0", 17)+"2")
	b := mustParse(t, "0."+strings.Repeat("0", 18)+"5")
	if got, err := decimal.Mul(a, b, limits); err != nil || got.String() != tiny.String() {
		t.Fatal(got, err)
	}
}

// ratWithinLimits derives admissibility from a rational denominator, not Decimal helpers.
func ratWithinLimits(r *big.Rat) bool {
	if r.Sign() == 0 {
		return true
	}
	denominator := new(big.Int).Set(r.Denom())
	count2, count5 := 0, 0
	for _, factor := range []int64{2, 5} {
		q, rem := new(big.Int), new(big.Int)
		for {
			q.QuoRem(denominator, big.NewInt(factor), rem)
			if rem.Sign() != 0 {
				break
			}
			denominator.Set(q)
			if factor == 2 {
				count2++
			} else {
				count5++
			}
		}
	}
	if denominator.Cmp(big.NewInt(1)) != 0 {
		return false
	}
	scale := count2
	if count5 > scale {
		scale = count5
	}
	if scale > 36 {
		return false
	}
	coefficient := new(big.Int).Abs(r.Num())
	coefficient.Mul(coefficient, new(big.Int).Exp(big.NewInt(2), big.NewInt(int64(scale-count2)), nil))
	coefficient.Mul(coefficient, new(big.Int).Exp(big.NewInt(5), big.NewInt(int64(scale-count5)), nil))
	return len(coefficient.String()) <= 128
}

func TestRatAgreement(t *testing.T) {
	rng := rand.New(rand.NewSource(20261031))
	randomText := func() string {
		n := rng.Intn(128) + 1
		raw := make([]byte, n)
		raw[0] = byte('1' + rng.Intn(9))
		for i := 1; i < n; i++ {
			raw[i] = byte('0' + rng.Intn(10))
		}
		scale := rng.Intn(37)
		text := string(raw)
		if scale > 0 {
			if len(text) <= scale {
				text = strings.Repeat("0", scale+1-len(text)) + text
			}
			text = text[:len(text)-scale] + "." + text[len(text)-scale:]
		}
		if rng.Intn(2) == 0 {
			text = "-" + text
		}
		return text
	}
	okCount, limitCount := 0, 0
	for i := 0; i < 10000; i++ {
		as, bs := randomText(), randomText()
		a, b := mustParse(t, as), mustParse(t, bs)
		ar, _ := new(big.Rat).SetString(as)
		br, _ := new(big.Rat).SetString(bs)
		for _, op := range []string{"+", "-", "*"} {
			var got decimal.Decimal
			var err error
			want := new(big.Rat)
			switch op {
			case "+":
				got, err = decimal.Add(a, b, limits)
				want.Add(ar, br)
			case "-":
				got, err = decimal.Sub(a, b, limits)
				want.Sub(ar, br)
			case "*":
				got, err = decimal.Mul(a, b, limits)
				want.Mul(ar, br)
			}
			if !ratWithinLimits(want) {
				if diagnostic.CodeOf(err) != "E_NUMERIC_LIMIT" {
					t.Fatalf("pair %d %s: expected limit, got %s %v", i, op, got, err)
				}
				limitCount++
				continue
			}
			if err != nil {
				t.Fatalf("pair %d %s: %v", i, op, err)
			}
			actual, ok := new(big.Rat).SetString(got.String())
			if !ok || actual.Cmp(want) != 0 {
				t.Fatalf("pair %d: %s %s %s = %s; want %s", i, as, op, bs, got.String(), want.RatString())
			}
			okCount++
			if op == "+" && decimal.Cmp(a, b) != ar.Cmp(br) {
				t.Fatalf("pair %d comparison disagreement", i)
			}
		}
	}
	t.Logf("seed=20261031 pairs=10000 operations=30000 exact=%d intended_limit_errors=%d", okCount, limitCount)
	if okCount == 0 || limitCount == 0 {
		t.Fatal("missing successful or range-error coverage")
	}
}
