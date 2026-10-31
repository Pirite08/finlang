package parser_test

import (
	"github.com/Pirite08/finlang/internal/ast"
	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/parser"
	"strings"
	"testing"
)

func parse(t *testing.T, s string) *ast.Program {
	t.Helper()
	p, e := parser.Parse("test.fin", s, config.DefaultLimits())
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func expr(t *testing.T, s string) ast.Expr {
	t.Helper()
	p := parse(t, s)
	if len(p.Statements) != 1 {
		t.Fatalf("got %d statements", len(p.Statements))
	}
	return p.Statements[0].(*ast.ExpressionStatement).Expression
}
func TestPrecedence(t *testing.T) {
	a := expr(t, "1+2*3").(*ast.Infix)
	if a.Operator != "+" || a.Right.(*ast.Infix).Operator != "*" {
		t.Fatal(a)
	}
	b := expr(t, "(1+2)*3").(*ast.Infix)
	if b.Operator != "*" || b.Left.(*ast.Infix).Operator != "+" {
		t.Fatal(b)
	}
	c := expr(t, "-f(1) == 2 < 3 + 4 * 5").(*ast.Infix)
	if c.Operator != "==" || c.Left.(*ast.Prefix).Right.(*ast.Call).Function.(*ast.Identifier).Name != "f" || c.Right.(*ast.Infix).Operator != "<" {
		t.Fatal(c)
	}
	d := expr(t, "1-2-3").(*ast.Infix)
	if _, ok := d.Left.(*ast.Infix); !ok {
		t.Fatal("not left associative")
	}
}
func TestParseFunctionsAndIf(t *testing.T) {
	p := parse(t, `let f = fn(x, y) { if (x > y) { return x; } else { return y } }; f(1,2)`)
	fn := p.Statements[0].(*ast.Let).Value.(*ast.Function)
	if len(fn.Parameters) != 2 || fn.Parameters[1] != "y" {
		t.Fatal(fn)
	}
	iff := fn.Body.Statements[0].(*ast.If)
	if iff.Alternative == nil || len(iff.Consequence.Statements) != 1 {
		t.Fatal(iff)
	}
	if _, ok := iff.Alternative.Statements[0].(*ast.Return).Value.(*ast.Identifier); !ok {
		t.Fatal("return value")
	}
}
func TestSlashRequiresPolicy(t *testing.T) {
	_, e := parser.Parse("x", "1/3", config.DefaultLimits())
	if diagnostic.CodeOf(e) != "E_DIVISION_POLICY_REQUIRED" || !strings.Contains(e.Error(), "div") {
		t.Fatal(e)
	}
}
func TestSyntaxDepth(t *testing.T) {
	for _, s := range []string{strings.Repeat("(", 129) + "1" + strings.Repeat(")", 129), strings.Repeat("!", 129) + "true", strings.Repeat("1+", 129) + "1", "f" + strings.Repeat("()", 129)} {
		_, e := parser.Parse("x", s, config.DefaultLimits())
		if diagnostic.CodeOf(e) != "E_PARSE_DEPTH" {
			t.Fatalf("depth source %.30s: %v", s, e)
		}
	}
	parse(t, strings.Repeat("(", 32)+"1"+strings.Repeat(")", 32))
}
func TestStatementTerminators(t *testing.T) {
	for _, s := range []string{"let x = 1 let y = 2;", "1 2;", "return 1 return 2;", "fn(a,) { return a; }", "if (true) {} else if (true) {}", "let x = if (true) {};"} {
		if _, e := parser.Parse("x", s, config.DefaultLimits()); e == nil {
			t.Errorf("accepted %q", s)
		}
	}
	for _, s := range []string{"let x = 1", `if (true) { let x = 1 }`, "if (true) {} let x = 1;"} {
		parse(t, s)
	}
}
func TestSourceSpans(t *testing.T) {
	p := parse(t, "// 한글\nlet x = 0.100;")
	n := p.Statements[0].(*ast.Let)
	if n.Span.Line != 2 || n.Span.Column != 1 || n.Value.(*ast.Number).Literal != "0.100" || n.Value.GetSpan().Column != 9 {
		t.Fatal(n)
	}
}
func TestMalformedSyntaxDoesNotPanic(t *testing.T) {
	for _, s := range []string{"if (true) {", "fn(", "fn()", "f(", "f(1,)", "(", "let", "return;", "if (true) {} else", "}", ";", "1+", "if (true) { / }"} {
		t.Run(s, func(t *testing.T) {
			if _, e := parser.Parse("x", s, config.DefaultLimits()); e == nil {
				t.Errorf("accepted %q", s)
			}
		})
	}
}
