package decimal

import (
	"math/big"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
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

// roundedQuotient borrows signed operands and applies the rounding mode to their magnitudes.
func roundedQuotient(numerator, denominator *big.Int, digits uint32, mode Mode, limits config.Limits) (Rounded, error) {
	q, r := new(big.Int), new(big.Int)
	q.QuoRem(numerator, denominator, r)
	q.Abs(q)
	inexact := r.Sign() != 0
	if inexact {
		if mode == Exact {
			return Rounded{}, diagnostic.New("E_INEXACT", "exact operation would discard nonzero digits")
		}
		if mode == HalfEven || mode == HalfUp {
			comparison := new(big.Int).Lsh(r, 1).CmpAbs(denominator)
			if comparison > 0 || (comparison == 0 && (mode == HalfUp || q.Bit(0) == 1)) {
				q.Add(q, big.NewInt(1))
			}
		}
	}
	if numerator.Sign()*denominator.Sign() < 0 {
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
	if digits >= x.scale {
		if err := x.Validate(limits); err != nil {
			return Rounded{}, err
		}
		return Rounded{Value: x}, nil
	}
	return roundedQuotient(x.readCoefficient(), pow10(x.scale-digits), digits, mode, limits)
}

// Floor rounds x to the greatest integer less than or equal to x.
func Floor(x Decimal, limits config.Limits) (Rounded, error) {
	return roundInteger(x, -1, limits)
}

// Ceil rounds x to the least integer greater than or equal to x.
func Ceil(x Decimal, limits config.Limits) (Rounded, error) {
	return roundInteger(x, 1, limits)
}

func roundInteger(x Decimal, direction int, limits config.Limits) (Rounded, error) {
	if x.scale == 0 {
		return Round(x, 0, TowardZero, limits)
	}
	var quotient, remainder big.Int
	quotient.QuoRem(x.readCoefficient(), pow10(x.scale), &remainder)
	remainderSign := remainder.Sign()
	if remainderSign == direction {
		quotient.Add(&quotient, big.NewInt(int64(direction)))
	}
	value, err := makeDecimal(&quotient, 0, limits)
	if err != nil {
		return Rounded{}, err
	}
	return Rounded{Value: value, Inexact: remainderSign != 0}, nil
}
