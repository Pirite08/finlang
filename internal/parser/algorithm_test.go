package parser_test

import (
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/ast"
	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/parser"
)

func TestParseAlgorithmSyntax(t *testing.T) {
	p := parse(t, "var total = 0; for value in [1, 2,] { total = total + value; } total;")
	variable := p.Statements[0].(*ast.Var)
	loop := p.Statements[1].(*ast.For)
	assign := loop.Body.Statements[0].(*ast.Assign)
	if variable.Name != "total" || loop.Name != "value" || assign.Name != "total" {
		t.Fatalf("unexpected binding names: %q, %q, %q", variable.Name, loop.Name, assign.Name)
	}
	if n := len(loop.Iterable.(*ast.List).Elements); n != 2 {
		t.Fatalf("list has %d elements, want 2", n)
	}
	if n := len(expr(t, "[]").(*ast.List).Elements); n != 0 {
		t.Fatalf("empty list has %d elements", n)
	}
}

func TestIndexPrecedenceAndSpans(t *testing.T) {
	infix := expr(t, "-factory()[1][0](2) * 3").(*ast.Infix)
	call := infix.Left.(*ast.Prefix).Right.(*ast.Call)
	lastIndex := call.Function.(*ast.Index)
	firstIndex := lastIndex.Collection.(*ast.Index)
	if firstIndex.Collection.(*ast.Call).Function.(*ast.Identifier).Name != "factory" {
		t.Fatal("postfix calls and indexing must chain from left to right")
	}
	if lastIndex.Index.(*ast.Number).Literal != "0" || lastIndex.Span.Start != 1 || lastIndex.Span.End != len("-factory()[1][0]") {
		t.Fatalf("unexpected last index node: %+v", lastIndex)
	}
}

func TestAlgorithmWordsRemainIdentifiers(t *testing.T) {
	parse(t, `let var = 1; let for = fn(in) { return in; }; for(var);`)
	parse(t, `rule test {
input amount in "KRW"
output result in "KRW"
step keep_identifiers {
    let for = amount
    let x = amount
    let in = amount
    for
    x
    in
    let result = amount
}
return result
}`)
}

func TestRejectMalformedAlgorithmSyntax(t *testing.T) {
	for _, source := range []string{
		"[",
		"[1 2]",
		"[,]",
		"x[]",
		"x[1",
		"x[1, 2]",
		"x[0] = 1",
		"var x",
		"x =",
		"for x in {}",
		"for x in [1]",
	} {
		t.Run(source, func(t *testing.T) {
			if _, err := parser.Parse("test.fin", source, config.DefaultLimits()); diagnostic.CodeOf(err) != "E_SYNTAX" {
				t.Fatalf("got %v, want E_SYNTAX", err)
			}
		})
	}
}

func TestAlgorithmSyntaxDepth(t *testing.T) {
	for _, source := range []string{
		strings.Repeat("[", 129) + "1" + strings.Repeat("]", 129),
		"x" + strings.Repeat("[0]", 129),
		strings.Repeat("for x in [] { ", 129) + strings.Repeat("}", 129),
	} {
		if _, err := parser.Parse("test.fin", source, config.DefaultLimits()); diagnostic.CodeOf(err) != "E_PARSE_DEPTH" {
			t.Fatalf("source %.25q: got %v, want E_PARSE_DEPTH", source, err)
		}
	}
}
