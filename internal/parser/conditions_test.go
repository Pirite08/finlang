package parser_test

import (
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/ast"
	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/parser"
)

func TestParseValueIf(t *testing.T) {
	source := `let rate = if (principal >= money("KRW", 1000000)) { 0.005 } else { 0.01 };
if (true) { let previous = 1; }`
	program := parse(t, source)
	initializer := program.Statements[0].(*ast.Let).Value
	conditional, ok := initializer.(*ast.IfExpr)
	if !ok {
		t.Fatalf("value if initializer: got %T", initializer)
	}
	if conditional.Condition.(*ast.Infix).Operator != ">=" ||
		conditional.Consequence.(*ast.Number).Literal != "0.005" ||
		conditional.Alternative.(*ast.Number).Literal != "0.01" {
		t.Fatalf("unexpected value if: %#v", conditional)
	}
	if span := conditional.Span; span.Start != strings.Index(source, "if") || span.End != strings.Index(source, ";") {
		t.Fatalf("unexpected value if span: %+v", span)
	}
	statement, ok := program.Statements[1].(*ast.If)
	if !ok || statement.Alternative != nil {
		t.Fatalf("statement if: got %#v", program.Statements[1])
	}
	parse(t, `print((if (true) { [1]; } else { [2]; })[0]);`)
	parse(t, `let rate = if (true) { if (false) { 0 } else { 0.005 } } else { 0.01 };`)
}

func TestValueIfRejectsInvalidBranches(t *testing.T) {
	for _, source := range []string{
		`let x = if (true) { 1 };`,
		`let x = if (true) {} else { 2 };`,
		`let x = if (true) { 1 } else {};`,
		`let x = if (true) { let y = 1; } else { 2 };`,
		`let x = if (true) { 1; 2 } else { 3 };`,
		`let x = if (true) { 1 } else if (false) { 2 } else { 3 };`,
	} {
		t.Run(source, func(t *testing.T) {
			if _, err := parser.Parse("test.fin", source, config.DefaultLimits()); diagnostic.CodeOf(err) != "E_SYNTAX" {
				t.Fatalf("got %v, want E_SYNTAX", err)
			}
		})
	}
}

func TestLogicalOperatorPrecedence(t *testing.T) {
	or := expr(t, "false || true && 1 + 2 * [3][0] < 8 == !false").(*ast.Infix)
	and := or.Right.(*ast.Infix)
	equal := and.Right.(*ast.Infix)
	less := equal.Left.(*ast.Infix)
	add := less.Left.(*ast.Infix)
	mul := add.Right.(*ast.Infix)
	if or.Operator != "||" || and.Operator != "&&" || equal.Operator != "==" || less.Operator != "<" ||
		add.Operator != "+" || mul.Operator != "*" || equal.Right.(*ast.Prefix).Operator != "!" {
		t.Fatalf("unexpected operator precedence: %#v", or)
	}
	if _, ok := mul.Right.(*ast.Index); !ok {
		t.Fatalf("multiplication right side = %T, want index", mul.Right)
	}
	left := expr(t, "true || false || true").(*ast.Infix).Left
	if _, ok := left.(*ast.Infix); !ok {
		t.Fatalf("|| should associate to the left: got %T", left)
	}
}

func TestValueIfAndLogicDepth(t *testing.T) {
	for _, source := range []string{
		"let x = " + strings.Repeat("if (true) { ", 129) + "1" + strings.Repeat(" } else { 0 }", 129),
		"true" + strings.Repeat(" && true", 129),
	} {
		if _, err := parser.Parse("test.fin", source, config.DefaultLimits()); diagnostic.CodeOf(err) != "E_PARSE_DEPTH" {
			t.Fatalf("source %.25q: got %v, want E_PARSE_DEPTH", source, err)
		}
	}
}
