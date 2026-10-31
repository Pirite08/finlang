// Package experiments contains small reproducible probes, not performance claims.
package experiments

import (
	"math/big"
	"strconv"
	"testing"
)

func TestRepresentationProbe(t *testing.T) {
	a, b := 0.1, 0.2
	floatResult := a + b
	if floatResult == 0.3 {
		t.Fatal("expected binary float runtime rounding to differ from 0.3")
	}
	left, ok := new(big.Rat).SetString("0.1")
	if !ok {
		t.Fatal("invalid probe input")
	}
	right, ok := new(big.Rat).SetString("0.2")
	if !ok {
		t.Fatal("invalid probe input")
	}
	stringResult := new(big.Rat).Add(left, right).FloatString(1)
	if stringResult != "0.3" {
		t.Fatalf("direct string result=%s", stringResult)
	}
	coefficient := new(big.Int).Add(big.NewInt(1), big.NewInt(2))
	if coefficient.String() != "3" {
		t.Fatalf("common-scale coefficient=%s", coefficient)
	}
	t.Logf("runtime float64=%s; string rational=%s; scale=1 integer coefficient=%s", strconv.FormatFloat(floatResult, 'g', -1, 64), stringResult, coefficient)
}

var floatSink float64
var rationalSink *big.Rat
var integerSink *big.Int

func BenchmarkRepresentationProbe(b *testing.B) {
	b.Run("runtime_float64_add", func(b *testing.B) {
		left, right := 0.1, 0.2
		for i := 0; i < b.N; i++ {
			floatSink = left + right
		}
	})
	b.Run("direct_string_rational_parse_and_add", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			left, _ := new(big.Rat).SetString("0.1")
			right, _ := new(big.Rat).SetString("0.2")
			rationalSink = new(big.Rat).Add(left, right)
		}
	})
	b.Run("prepared_common_scale_integer_add", func(b *testing.B) {
		left, right := big.NewInt(1), big.NewInt(2)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			integerSink = new(big.Int).Add(left, right)
		}
	})
}
