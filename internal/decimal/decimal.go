// Package decimal provides immutable, bounded, exact base-ten numbers.
package decimal

import (
	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"math/big"
)

// Decimal represents coefficient * 10^-scale. Its zero value is zero.
// Coefficients are never exposed or mutated after a Decimal is constructed.
type Decimal struct {
	coefficient *big.Int
	scale       uint32
}

func (d Decimal) coeff() *big.Int {
	if d.coefficient == nil {
		return new(big.Int)
	}
	return new(big.Int).Set(d.coefficient)
}

func pow10(n uint32) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), new(big.Int).SetUint64(uint64(n)), nil)
}

// makeDecimal takes a copy before normalization, retaining exclusive ownership.
func makeDecimal(c *big.Int, scale uint32, limits config.Limits) (Decimal, error) {
	c = new(big.Int).Set(c)
	if c.Sign() == 0 {
		return Decimal{}, nil
	}
	ten := big.NewInt(10)
	q, r := new(big.Int), new(big.Int)
	for scale > 0 {
		q.QuoRem(c, ten, r)
		if r.Sign() != 0 {
			break
		}
		c.Set(q)
		scale--
	}
	if scale > limits.MaxScale || len(new(big.Int).Abs(c).String()) > limits.MaxCoefficientDigits {
		return Decimal{}, diagnostic.New("E_NUMERIC_LIMIT", "normalized decimal exceeds numeric limits")
	}
	return Decimal{c, scale}, nil
}
