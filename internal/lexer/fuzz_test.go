package lexer_test

import (
	"example.com/finlang/internal/config"
	"example.com/finlang/internal/lexer"
	"example.com/finlang/internal/token"
	"strings"
	"testing"
)

func FuzzLexerNoPanic(f *testing.F) {
	for _, source := range []string{"", strings.Repeat("1", 300), `"unterminated`, "\xff", `// 한글
let amount = 0.100;`, `"a\"\\\n\t"`} {
		f.Add(source)
	}
	f.Fuzz(func(t *testing.T, source string) {
		if len(source) > 4096 {
			t.Skip()
		}
		limits := config.DefaultLimits()
		limits.MaxSourceBytes = 4096
		limits.MaxTokenBytes = 128
		l, err := lexer.New("fuzz.fin", source, limits)
		if err != nil {
			return
		}
		previousEnd := 0
		for steps := 0; steps <= len(source)+1; steps++ {
			tok, err := l.Next()
			if err != nil {
				return
			}
			if tok.Span.Start < previousEnd || tok.Span.End < tok.Span.Start || tok.Span.End > len(source) || tok.Span.Line < 1 || tok.Span.Column < 1 {
				t.Fatalf("invalid span: %+v", tok.Span)
			}
			previousEnd = tok.Span.End
			if tok.Kind == token.EOF {
				return
			}
		}
		t.Fatal("lexer failed to terminate within source bound")
	})
}
