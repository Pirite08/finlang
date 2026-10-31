package lexer_test

import (
	"errors"
	"example.com/finlang/internal/config"
	"example.com/finlang/internal/diagnostic"
	"example.com/finlang/internal/lexer"
	"example.com/finlang/internal/token"
	"strings"
	"testing"
)

func scan(t *testing.T, source string) []token.Token {
	t.Helper()
	l, err := lexer.New("test.fin", source, config.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	var result []token.Token
	for {
		tok, err := l.Next()
		if err != nil {
			t.Fatal(err)
		}
		result = append(result, tok)
		if tok.Kind == token.EOF {
			return result
		}
	}
}
func TestTokenizeProgram(t *testing.T) {
	ts := scan(t, `let f = fn(x) { if (x >= 0.100) { return "a\"\\\n\t"; } else { return false; } }; f(2, 3); true == !false; 1 != 2; 1 < 2; 2 <= 3; 3 > 2; 2 + 1 - 1 * 4 / 2;`)
	kinds := []token.Kind{token.Let, token.Identifier, token.Assign, token.Fn, token.LParen, token.Identifier, token.RParen, token.LBrace, token.If, token.LParen, token.Identifier, token.GreaterEqual, token.Number}
	if len(ts) < len(kinds) {
		t.Fatalf("got %d tokens", len(ts))
	}
	for i, want := range kinds {
		if ts[i].Kind != want {
			t.Fatalf("token %d = %v, want %v", i, ts[i].Kind, want)
		}
	}
	found := map[token.Kind]bool{}
	for _, tok := range ts {
		found[tok.Kind] = true
	}
	for _, kind := range []token.Kind{token.Return, token.Else, token.True, token.False, token.String, token.Equal, token.NotEqual, token.Bang, token.Less, token.LessEqual, token.Greater, token.Comma, token.Semicolon, token.Plus, token.Minus, token.Star, token.Slash} {
		if !found[kind] {
			t.Errorf("missing %v", kind)
		}
	}
	if ts[16].Literal != "a\"\\\n\t" {
		t.Fatalf("decoded string = %q", ts[16].Literal)
	}
}
func TestNumericLiteralPreserved(t *testing.T) {
	if got := scan(t, "0.100")[0].Literal; got != "0.100" {
		t.Fatalf("got %q", got)
	}
}
func TestUTF8CommentPosition(t *testing.T) {
	l, err := lexer.New("한글.fin", "// 한글 주석\n\"한\" @", config.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = l.Next(); err != nil {
		t.Fatal(err)
	}
	_, err = l.Next()
	var d *diagnostic.Error
	if !errors.As(err, &d) || d.Span == nil || d.Span.Line != 2 || d.Span.Column != 5 {
		t.Fatalf("position error: %#v", err)
	}
}
func TestLexerLimitsAndEncoding(t *testing.T) {
	for _, tc := range []struct {
		src  string
		code diagnostic.Code
	}{{strings.Repeat("1", 257), "E_TOKEN_LIMIT"}, {strings.Repeat("a", 257), "E_TOKEN_LIMIT"}, {`"` + strings.Repeat("a", 257) + `"`, "E_TOKEN_LIMIT"}, {"\xff", "E_SOURCE_ENCODING"}, {`"unterminated`, "E_UNTERMINATED_STRING"}, {`"bad\q"`, "E_STRING_ESCAPE"}, {"가", "E_INVALID_TOKEN"}, {"1.2.3", "E_INVALID_NUMBER"}} {
		t.Run(string(tc.code)+tc.src[:1], func(t *testing.T) {
			l, err := lexer.New("x", tc.src, config.DefaultLimits())
			if err == nil {
				_, err = l.Next()
			}
			if diagnostic.CodeOf(err) != tc.code {
				t.Fatalf("got %v want %s", err, tc.code)
			}
		})
	}
	limits := config.DefaultLimits()
	limits.MaxSourceBytes = 2
	_, err := lexer.New("x", "123", limits)
	if diagnostic.CodeOf(err) != "E_SOURCE_LIMIT" {
		t.Fatal(err)
	}
}
func TestTokenBoundariesAndEOF(t *testing.T) {
	for _, source := range []string{strings.Repeat("1", 256), strings.Repeat("a", 256), `"` + strings.Repeat("a", 256) + `"`} {
		ts := scan(t, source)
		if len(ts) != 2 {
			t.Fatalf("boundary token rejected: %d tokens", len(ts))
		}
	}
	l, e := lexer.New("x", "// 주석", config.Limits{})
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		tok, e := l.Next()
		if e != nil || tok.Kind != token.EOF || tok.Span.Column != 6 {
			t.Fatalf("EOF = %+v, %v", tok, e)
		}
	}
}
