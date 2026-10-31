package decimal_test

import (
	"example.com/finlang/internal/decimal"
	"strings"
	"testing"
)

// Each benchmark exercises useful workloads, reports allocations, and checks
// errors rather than silently timing an unsuccessful operation.
func BenchmarkParse(b *testing.B) {
	for _, tc := range []struct{ name, text string }{{"money", "1234567.89"}, {"max_coefficient", strings.Repeat("9", 92) + "." + strings.Repeat("9", 36)}} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := decimal.Parse(tc.text, limits); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkArithmetic(b *testing.B) {
	a, c := mustParse(b, "123456789012345678901234567890.123456"), mustParse(b, "-0.000000000000000000000000000000000001")
	b.Run("add_scale_gap_30", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := decimal.Add(a, c, limits); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("multiply", func(b *testing.B) {
		x, y := mustParse(b, "123456789012345678901234567890.123456"), mustParse(b, "9876543210.0123456789")
		b.ReportAllocs()
		for b.Loop() {
			if _, err := decimal.Mul(x, y, limits); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("compare_scale_gap_30", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if decimal.Cmp(a, c) != 1 {
				b.Fatal("incorrect comparison")
			}
		}
	})
}

func BenchmarkRound(b *testing.B) {
	a := mustParse(b, "123456789012345678901234567890.123456789012345678901234567895")
	b.ReportAllocs()
	for b.Loop() {
		if _, err := decimal.Round(a, 2, decimal.HalfEven, limits); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDivision(b *testing.B) {
	for _, tc := range []struct {
		name, a, divisor string
		digits           uint32
		mode             decimal.Mode
	}{{"terminating_exact", "12345678901234567890.123456", "8", 12, decimal.Exact}, {"recurring_36_places", "12345678901234567890.123456", "7", 36, decimal.HalfEven}} {
		b.Run(tc.name, func(b *testing.B) {
			a, c := mustParse(b, tc.a), mustParse(b, tc.divisor)
			b.ReportAllocs()
			for b.Loop() {
				if _, err := decimal.Div(a, c, tc.digits, tc.mode, limits); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
