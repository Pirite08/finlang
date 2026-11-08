package parser_test

import (
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/ast"
	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/parser"
)

func TestParseCompositeSyntax(t *testing.T) {
	for _, source := range []string{
		`let row = record { amount: 1.25, label: "KRW", }; let totals = map { "KRW": row.amount, }; totals["KRW"].round(0, "HALF_EVEN");`,
		`record {};`,
		`map {};`,
		`let map = []; let record = []; for x in map {} for x in record { let y = x; }`,
		`for x in map { "KRW": 1 }.keys() {}`,
		`for x in map {}.keys() {}`,
		`for x in map {} (1);`,
	} {
		t.Run(source, func(t *testing.T) {
			parse(t, source)
		})
	}
}

func TestMalformedCompositeSyntax(t *testing.T) {
	for _, source := range []string{
		`record { "x": 1 }`,
		`record { x 1 }`,
		`map { x: 1 }`,
		`map { "KRW": }`,
		`map { "KRW": 1 "USD": 2 }`,
		`row.`,
		`row.0`,
	} {
		t.Run(source, func(t *testing.T) {
			if _, err := parser.Parse("test.fin", source, config.DefaultLimits()); diagnostic.CodeOf(err) != "E_SYNTAX" {
				t.Fatalf("got %v, want E_SYNTAX", err)
			}
		})
	}
}

func TestCompositeASTAndMemberPrecedence(t *testing.T) {
	source := `-map { "KRW": record { amount: 1.25 } }["KRW"].amount.round(0, "HALF_EVEN") * 2`
	infix := expr(t, source).(*ast.Infix)
	call := infix.Left.(*ast.Prefix).Right.(*ast.Call)
	method := call.Function.(*ast.Member)
	field := method.Receiver.(*ast.Member)
	index := field.Receiver.(*ast.Index)
	mapLiteral := index.Collection.(*ast.Map)
	entry := mapLiteral.Entries[0]
	amount := entry.Value.(*ast.Record).Fields[0]
	if method.Name != "round" || field.Name != "amount" || entry.Name != "KRW" || amount.Name != "amount" {
		t.Fatalf("unexpected member and literal names: %q, %q, %q, %q", method.Name, field.Name, entry.Name, amount.Name)
	}
	if got := source[entry.Span.Start:entry.Span.End]; got != `"KRW"` {
		t.Fatalf("map key span points to %q", got)
	}
	if got := source[amount.Span.Start:amount.Span.End]; got != "amount" {
		t.Fatalf("field name span points to %q", got)
	}
	if got := source[method.Span.Start:method.Span.End]; !strings.HasSuffix(got, ".amount.round") {
		t.Fatalf("method span points to %q", got)
	}
}

func TestCompositeSyntaxDepth(t *testing.T) {
	for _, source := range []string{
		strings.Repeat("record { x: ", 129) + "1" + strings.Repeat("}", 129),
		"x" + strings.Repeat(".field", 130),
	} {
		if _, err := parser.Parse("test.fin", source, config.DefaultLimits()); diagnostic.CodeOf(err) != "E_PARSE_DEPTH" {
			t.Fatalf("source %.25q: got %v, want E_PARSE_DEPTH", source, err)
		}
	}
}
