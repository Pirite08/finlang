package parser_test

import (
	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/parser"
	"strings"
	"testing"
)

func FuzzParserNoPanic(f *testing.F) {
	for _, source := range []string{
		"",
		`let f = fn(x) { if (true) { return x } else { return 0 } }; f(0.100);`,
		`let xs = [1, 2,]; var total = 0; for i in range(0, len(xs)) { total = total + xs[i]; } total;`,
		`let rate = if (amount >= 1000 && active || preferred) { 0.005 } else { 0.01 };`,
		`let totals = map { "KRW": record { amount: 1.25 } }; totals["KRW"].amount.round(0, "HALF_EVEN");`,
		exchangeRule,
		`"unterminated`,
		"\xff",
		strings.Repeat("(", 200) + "1",
		strings.Repeat("1+", 200) + "1",
		"f" + strings.Repeat("()", 200),
	} {
		f.Add(source)
	}
	f.Fuzz(func(t *testing.T, source string) {
		if len(source) > 4096 {
			t.Skip()
		}
		limits := config.DefaultLimits()
		limits.MaxSourceBytes = 4096
		limits.MaxTokenBytes = 128
		limits.MaxSyntaxDepth = 32
		p, e := parser.Parse("fuzz.fin", source, limits)
		if e == nil && p == nil {
			t.Fatal("successful parse returned nil program")
		}
	})
}
