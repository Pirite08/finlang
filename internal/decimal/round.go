package decimal

import (
	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"math/big"
)

type Mode string

const (
	HalfEven   Mode = "HALF_EVEN"
	HalfUp     Mode = "HALF_UP"
	TowardZero Mode = "TOWARD_ZERO"
	Exact      Mode = "EXACT"
)

type Rounded struct {
	Value   Decimal
	Inexact bool
}

// ParseMode recognizes the four exact, case-sensitive language mode names.
func ParseMode(text string) (Mode, error) {
	mode := Mode(text)
	switch mode {
	case HalfEven, HalfUp, TowardZero, Exact:
		return mode, nil
	}
	return "", diagnostic.New("E_ROUNDING_MODE", "unknown rounding mode")
}

func validateRounding(digits uint32, mode Mode, limits config.Limits) error {
	if _, err := ParseMode(string(mode)); err != nil {
		return err
	}
	if digits > limits.MaxScale {
		return diagnostic.New("E_NUMERIC_LIMIT", "target scale exceeds numeric limit")
	}
	return nil
}

// roundedQuotient uses positive magnitudes; only the final coefficient is signed.
func roundedQuotient(numerator, denominator *big.Int, sign int, digits uint32, mode Mode, limits config.Limits) (Rounded, error) {
	q, r := new(big.Int), new(big.Int)
	q.QuoRem(numerator, denominator, r)
	inexact := r.Sign() != 0
	if inexact {
		if mode == Exact {
			return Rounded{}, diagnostic.New("E_INEXACT", "exact operation would discard nonzero digits")
		}
		if mode == HalfEven || mode == HalfUp {
			comparison := new(big.Int).Lsh(r, 1).Cmp(denominator)
			if comparison > 0 || (comparison == 0 && (mode == HalfUp || q.Bit(0) == 1)) {
				q.Add(q, big.NewInt(1))
			}
		}
	}
	if sign < 0 {
		q.Neg(q)
	}
	value, err := makeDecimal(q, digits, limits)
	if err != nil {
		return Rounded{}, err
	}
	return Rounded{Value: value, Inexact: inexact}, nil
}

// Round explicitly rounds to digits fractional places using mode.
func Round(x Decimal, digits uint32, mode Mode, limits config.Limits) (Rounded, error) {
	if err := validateRounding(digits, mode, limits); err != nil {
		return Rounded{}, err
	}
	c := x.coeff()
	if digits >= x.scale {
		value, err := makeDecimal(c, x.scale, limits)
		if err != nil {
			return Rounded{}, err
		}
		return Rounded{Value: value}, nil
	}
	sign := c.Sign()
	c.Abs(c)
	return roundedQuotient(c, pow10(x.scale-digits), sign, digits, mode, limits)
}
