// Package decimal provides immutable, bounded, exact base-ten numbers.
package decimal

import (
	"math/big"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
)

// Decimal represents coefficient * 10^-scale. Its zero value is zero.
// Coefficients are never exposed or mutated after a Decimal is constructed.
type Decimal struct {
	coefficient *big.Int
	scale       uint32
}

var zeroCoefficient big.Int

// readCoefficient returns a borrowed operand. It must never be used as a receiver for mutation.
func (d Decimal) readCoefficient() *big.Int {
	if d.coefficient == nil {
		return &zeroCoefficient
	}
	return d.coefficient
}

// Sign returns -1, 0, or 1 according to the sign of d.
func (d Decimal) Sign() int { return d.readCoefficient().Sign() }

func pow10(n uint32) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), new(big.Int).SetUint64(uint64(n)), nil)
}

// makeDecimal normalizes and takes ownership of a freshly computed coefficient.
func makeDecimal(c *big.Int, scale uint32, limits config.Limits) (Decimal, error) {
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
	value := Decimal{c, scale}
	if err := value.Validate(limits); err != nil {
		return Decimal{}, err
	}
	return value, nil
}

// Validate checks whether the normalized value fits the given numeric limits.
func (d Decimal) Validate(limits config.Limits) error {
	if d.coefficient == nil {
		return nil
	}
	if d.scale <= limits.MaxScale {
		digits := len(d.coefficient.String())
		if d.coefficient.Sign() < 0 {
			digits--
		}
		if digits <= limits.MaxCoefficientDigits {
			return nil
		}
	}
	return diagnostic.New("E_NUMERIC_LIMIT", "normalized decimal exceeds numeric limits")
}
