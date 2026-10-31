// Package parser builds a Pratt syntax tree and bounds both recursive syntax and tree height.
package parser

import (
	"fmt"

	"github.com/Pirite08/finlang/internal/ast"
	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/lexer"
	"github.com/Pirite08/finlang/internal/token"
)

type parser struct {
	lexer             *lexer.Lexer
	current, previous token.Token
	err               error
	depth, maxDepth   int
	heights           map[ast.Node]int
}

func Parse(file, source string, limits config.Limits) (*ast.Program, error) {
	l, e := lexer.New(file, source, limits)
	if e != nil {
		return nil, e
	}
	if limits.MaxSyntaxDepth <= 0 {
		limits.MaxSyntaxDepth = config.DefaultLimits().MaxSyntaxDepth
	}
	p := &parser{lexer: l, maxDepth: limits.MaxSyntaxDepth, heights: make(map[ast.Node]int)}
	p.advance()
	program := &ast.Program{}
	for p.err == nil && p.current.Kind != token.EOF {
		stmt := p.statement()
		if p.err == nil {
			program.Statements = append(program.Statements, stmt)
		}
	}
	if p.err != nil {
		return nil, p.err
	}
	return program, nil
}
func (p *parser) fail(code diagnostic.Code, message string, span diagnostic.Span) {
	if p.err == nil {
		p.err = diagnostic.WithSpan(diagnostic.New(code, message), span)
	}
}
func (p *parser) advance() {
	if p.err != nil {
		return
	}
	p.previous = p.current
	p.current, p.err = p.lexer.Next()
	if p.err == nil && p.current.Kind == token.Slash {
		p.fail("E_DIVISION_POLICY_REQUIRED", `use div(value, divisor, scale, "MODE") to specify division scale and rounding policy`, p.current.Span)
	}
}
func (p *parser) expect(kind token.Kind) {
	if p.err != nil {
		return
	}
	if p.current.Kind != kind {
		p.fail("E_SYNTAX", fmt.Sprintf("expected %s, got %s", kind, p.current.Kind), p.current.Span)
		return
	}
	p.advance()
}
func (p *parser) span(start diagnostic.Span) diagnostic.Span {
	start.End = p.previous.Span.End
	return start
}
func (p *parser) enter() bool {
	p.depth++
	if p.depth > p.maxDepth+1 {
		p.fail("E_PARSE_DEPTH", "syntax nesting exceeds depth limit", p.current.Span)
		p.depth--
		return false
	}
	return true
}

// A Pratt loop does not recurse for its left side. Track resulting tree height as
// well, so long left-associative and repeated-call chains cannot bypass the limit.
func (p *parser) record(n ast.Node, children ...ast.Node) {
	if n == nil || p.err != nil {
		return
	}
	height := 0
	for _, child := range children {
		if child != nil {
			height = max(height, p.heights[child]+1)
		}
	}
	p.heights[n] = height
	if height > p.maxDepth {
		p.fail("E_PARSE_DEPTH", "syntax tree exceeds depth limit", n.GetSpan())
	}
}
func (p *parser) terminator() {
	if p.current.Kind == token.Semicolon {
		p.advance()
		return
	}
	if p.current.Kind != token.EOF && p.current.Kind != token.RBrace {
		p.fail("E_SYNTAX", "expected semicolon between statements", p.current.Span)
	}
}
func (p *parser) statement() ast.Stmt {
	start := p.current.Span
	switch p.current.Kind {
	case token.Let:
		p.advance()
		name := p.current.Literal
		p.expect(token.Identifier)
		p.expect(token.Assign)
		value := p.expression(0)
		p.terminator()
		n := &ast.Let{Span: p.span(start), Name: name, Value: value}
		p.record(n, value)
		return n
	case token.Return:
		p.advance()
		value := p.expression(0)
		p.terminator()
		n := &ast.Return{Span: p.span(start), Value: value}
		p.record(n, value)
		return n
	case token.If:
		p.advance()
		p.expect(token.LParen)
		condition := p.expression(0)
		p.expect(token.RParen)
		consequence := p.block()
		var alternative *ast.Block
		if p.current.Kind == token.Else {
			p.advance()
			alternative = p.block()
		}
		n := &ast.If{Span: p.span(start), Condition: condition, Consequence: consequence, Alternative: alternative}
		children := []ast.Node{condition, consequence}
		if alternative != nil {
			children = append(children, alternative)
		}
		p.record(n, children...)
		if p.current.Kind == token.Semicolon {
			p.advance()
		}
		return n
	default:
		value := p.expression(0)
		p.terminator()
		n := &ast.ExpressionStatement{Span: p.span(start), Expression: value}
		p.record(n, value)
		return n
	}
}
func (p *parser) block() *ast.Block {
	if !p.enter() {
		return nil
	}
	defer func() { p.depth-- }()
	start := p.current.Span
	p.expect(token.LBrace)
	n := &ast.Block{Span: start}
	children := []ast.Node{}
	for p.err == nil && p.current.Kind != token.RBrace && p.current.Kind != token.EOF {
		stmt := p.statement()
		if p.err == nil {
			n.Statements = append(n.Statements, stmt)
			children = append(children, stmt)
		}
	}
	p.expect(token.RBrace)
	n.Span = p.span(start)
	p.record(n, children...)
	return n
}
func precedence(kind token.Kind) int {
	switch kind {
	case token.Equal, token.NotEqual:
		return 1
	case token.Less, token.LessEqual, token.Greater, token.GreaterEqual:
		return 2
	case token.Plus, token.Minus:
		return 3
	case token.Star:
		return 4
	case token.LParen:
		return 6
	}
	return 0
}
func (p *parser) expression(min int) ast.Expr {
	if p.err != nil {
		return nil
	}
	if !p.enter() {
		return nil
	}
	defer func() { p.depth-- }()
	start := p.current.Span
	tok := p.current
	var left ast.Expr
	switch tok.Kind {
	case token.Number:
		left = &ast.Number{Span: tok.Span, Literal: tok.Literal}
		p.advance()
		p.record(left)
	case token.String:
		left = &ast.String{Span: tok.Span, Value: tok.Literal}
		p.advance()
		p.record(left)
	case token.True, token.False:
		left = &ast.Bool{Span: tok.Span, Value: tok.Kind == token.True}
		p.advance()
		p.record(left)
	case token.Identifier:
		left = &ast.Identifier{Span: tok.Span, Name: tok.Literal}
		p.advance()
		p.record(left)
	case token.Minus, token.Bang:
		p.advance()
		right := p.expression(5)
		left = &ast.Prefix{Span: p.span(start), Operator: tok.Literal, Right: right}
		p.record(left, right)
	case token.LParen:
		p.advance()
		left = p.expression(0)
		p.expect(token.RParen)
	case token.Fn:
		left = p.function()
	default:
		p.fail("E_SYNTAX", "expected expression", tok.Span)
		return nil
	}
	for p.err == nil && precedence(p.current.Kind) > min {
		if p.current.Kind == token.LParen {
			left = p.call(left)
			continue
		}
		op := p.current
		power := precedence(op.Kind)
		p.advance()
		right := p.expression(power)
		n := &ast.Infix{Span: p.span(start), Left: left, Operator: op.Literal, Right: right}
		p.record(n, left, right)
		left = n
	}
	return left
}
func (p *parser) function() ast.Expr {
	start := p.current.Span
	p.advance()
	p.expect(token.LParen)
	n := &ast.Function{}
	if p.current.Kind != token.RParen {
		for p.err == nil {
			n.Parameters = append(n.Parameters, p.current.Literal)
			p.expect(token.Identifier)
			if p.current.Kind != token.Comma {
				break
			}
			p.advance()
		}
	}
	p.expect(token.RParen)
	n.Body = p.block()
	n.Span = p.span(start)
	p.record(n, n.Body)
	return n
}
func (p *parser) call(function ast.Expr) ast.Expr {
	start := function.GetSpan()
	p.expect(token.LParen)
	n := &ast.Call{Function: function}
	children := []ast.Node{function}
	if p.current.Kind != token.RParen {
		for p.err == nil {
			argument := p.expression(0)
			if p.err != nil {
				break
			}
			n.Arguments = append(n.Arguments, argument)
			children = append(children, argument)
			if p.current.Kind != token.Comma {
				break
			}
			p.advance()
		}
	}
	p.expect(token.RParen)
	n.Span = p.span(start)
	p.record(n, children...)
	return n
}
