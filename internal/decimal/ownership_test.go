package decimal

import (
	"example.com/finlang/internal/config"
	"math/big"
	"testing"
)

func TestConstructorOwnsCoefficient(t *testing.T) {
	c := big.NewInt(12300)
	value, err := makeDecimal(c, 3, config.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if c.String() != "12300" {
		t.Fatalf("normalization mutated supplied coefficient: %s", c)
	}
	c.SetInt64(999)
	if value.String() != "12.3" {
		t.Fatalf("retained caller-owned coefficient: %s", value.String())
	}
	// Even the internal copying helper cannot hand out an alias to stored state.
	copied := value.coeff()
	copied.SetInt64(-1)
	if value.String() != "12.3" {
		t.Fatalf("copy helper leaked coefficient: %s", value.String())
	}
}
