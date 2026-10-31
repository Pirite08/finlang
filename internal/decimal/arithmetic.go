package decimal

import (
	"github.com/Pirite08/finlang/internal/config"
	"math/big"
)

func aligned(a, b Decimal) (*big.Int, *big.Int, uint32) {
	scale := a.scale
	if b.scale > scale {
		scale = b.scale
	}
	ac, bc := a.coeff(), b.coeff()
	if a.scale < scale {
		ac.Mul(ac, pow10(scale-a.scale))
	}
	if b.scale < scale {
		bc.Mul(bc, pow10(scale-b.scale))
	}
	return ac, bc, scale
}

// Add returns the exact sum or a numeric-limit error.
func Add(a, b Decimal, limits config.Limits) (Decimal, error) {
	ac, bc, scale := aligned(a, b)
	return makeDecimal(ac.Add(ac, bc), scale, limits)
}

// Sub returns the exact difference or a numeric-limit error.
func Sub(a, b Decimal, limits config.Limits) (Decimal, error) {
	ac, bc, scale := aligned(a, b)
	return makeDecimal(ac.Sub(ac, bc), scale, limits)
}

// Mul returns the exact product, checking limits after normalization.
func Mul(a, b Decimal, limits config.Limits) (Decimal, error) {
	ac := a.coeff()
	return makeDecimal(ac.Mul(ac, b.coeff()), a.scale+b.scale, limits)
}

// Cmp returns -1, 0 or +1 according to the numeric ordering of a and b.
func Cmp(a, b Decimal) int { ac, bc, _ := aligned(a, b); return ac.Cmp(bc) }
