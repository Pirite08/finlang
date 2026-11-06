package decimal

import (
	"math/big"

	"github.com/Pirite08/finlang/internal/config"
)

// FromInt64 constructs an exact integer within the numeric limits.
func FromInt64(value int64, limits config.Limits) (Decimal, error) {
	return makeDecimal(big.NewInt(value), 0, limits)
}

func aligned(a, b Decimal) (*big.Int, *big.Int, uint32) {
	scale := a.scale
	if b.scale > scale {
		scale = b.scale
	}
	ac, bc := a.readCoefficient(), b.readCoefficient()
	if a.scale < scale {
		ac = new(big.Int).Mul(ac, pow10(scale-a.scale))
	}
	if b.scale < scale {
		bc = new(big.Int).Mul(bc, pow10(scale-b.scale))
	}
	return ac, bc, scale
}

// Add returns the exact sum or a numeric-limit error.
func Add(a, b Decimal, limits config.Limits) (Decimal, error) {
	ac, bc, scale := aligned(a, b)
	return makeDecimal(new(big.Int).Add(ac, bc), scale, limits)
}

// Sub returns the exact difference or a numeric-limit error.
func Sub(a, b Decimal, limits config.Limits) (Decimal, error) {
	ac, bc, scale := aligned(a, b)
	return makeDecimal(new(big.Int).Sub(ac, bc), scale, limits)
}

// Mul returns the exact product, checking limits after normalization.
func Mul(a, b Decimal, limits config.Limits) (Decimal, error) {
	return makeDecimal(new(big.Int).Mul(a.readCoefficient(), b.readCoefficient()), a.scale+b.scale, limits)
}

// Cmp returns -1, 0 or +1 according to the numeric ordering of a and b.
func Cmp(a, b Decimal) int { ac, bc, _ := aligned(a, b); return ac.Cmp(bc) }
