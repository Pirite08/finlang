package decimal

import (
	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
)

// Div performs a single explicit rounding at the requested fractional scale.
func Div(a, b Decimal, digits uint32, mode Mode, limits config.Limits) (Rounded, error) {
	if err := validateRounding(digits, mode, limits); err != nil {
		return Rounded{}, err
	}
	numerator, denominator := a.coeff(), b.coeff()
	if denominator.Sign() == 0 {
		return Rounded{}, diagnostic.New("E_DIVISION_BY_ZERO", "cannot divide by zero")
	}
	sign := numerator.Sign() * denominator.Sign()
	numerator.Abs(numerator)
	denominator.Abs(denominator)
	exponent := int64(digits) + int64(b.scale) - int64(a.scale)
	if exponent >= 0 {
		numerator.Mul(numerator, pow10(uint32(exponent)))
	} else {
		denominator.Mul(denominator, pow10(uint32(-exponent)))
	}
	return roundedQuotient(numerator, denominator, sign, digits, mode, limits)
}
