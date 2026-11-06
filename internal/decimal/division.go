package decimal

import (
	"math/big"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
)

// Div performs a single explicit rounding at the requested fractional scale.
func Div(a, b Decimal, digits uint32, mode Mode, limits config.Limits) (Rounded, error) {
	if err := validateRounding(digits, mode, limits); err != nil {
		return Rounded{}, err
	}
	numerator, denominator := a.readCoefficient(), b.readCoefficient()
	if denominator.Sign() == 0 {
		return Rounded{}, diagnostic.New("E_DIVISION_BY_ZERO", "cannot divide by zero")
	}
	exponent := int64(digits) + int64(b.scale) - int64(a.scale)
	if exponent > 0 {
		numerator = new(big.Int).Mul(numerator, pow10(uint32(exponent)))
	} else if exponent < 0 {
		denominator = new(big.Int).Mul(denominator, pow10(uint32(-exponent)))
	}
	return roundedQuotient(numerator, denominator, digits, mode, limits)
}
