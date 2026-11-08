// Package parser builds a Pratt syntax tree and bounds both recursive syntax and tree height.
package parser

import (
	"fmt"
	"strings"

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
	inStep            bool
	detachLexemes     bool
}

func Parse(file, source string, limits config.Limits) (*ast.Program, error) {
	return parse(file, source, limits, false)
}

// ParseDetached builds a tree that can outlive its source without retaining it.
func ParseDetached(file, source string, limits config.Limits) (*ast.Program, error) {
	return parse(file, source, limits, true)
}

func parse(file, source string, limits config.Limits, detachLexemes bool) (*ast.Program, error) {
	l, e := lexer.New(file, source, limits)
	if e != nil {
		return nil, e
	}
	if limits.MaxSyntaxDepth <= 0 {
		limits.MaxSyntaxDepth = config.DefaultLimits().MaxSyntaxDepth
	}
	p := &parser{lexer: l, maxDepth: limits.MaxSyntaxDepth, heights: make(map[ast.Node]int), detachLexemes: detachLexemes}
	p.advance()
	program := &ast.Program{}
	if p.startsRule() {
		program.Rule = p.rule()
		if p.current.Kind != token.EOF {
			p.fail("E_SYNTAX", "a rule must be the only top-level declaration", p.current.Span)
		}
	} else {
		for p.err == nil && p.current.Kind != token.EOF {
			stmt := p.statement()
			if p.err == nil {
				program.Statements = append(program.Statements, stmt)
			}
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
	if p.detachLexemes && p.current.Kind != token.String {
		// The lexer already allocates separate storage for decoded strings.
		p.current.Literal = strings.Clone(p.current.Literal)
	}
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
	if p.current.Kind == token.EOF || p.current.Kind == token.RBrace {
		return
	}
	if p.inStep {
		if p.current.Span.Line <= p.previous.Span.Line {
			p.fail("E_SYNTAX", "expected newline or semicolon between step statements", p.current.Span)
		}
		return
	}
	p.fail("E_SYNTAX", "expected semicolon between statements", p.current.Span)
}
func (p *parser) statement() ast.Stmt {
	start := p.current.Span
	if p.inStep && p.isWord("require") {
		p.advance()
		condition := p.expression(0)
		p.terminator()
		n := &ast.Require{Span: p.span(start), Condition: condition}
		p.record(n, condition)
		return n
	}
	if p.current.Kind == token.Identifier {
		if statement := p.identifierStatement(); statement != nil {
			return statement
		}
	}
	switch p.current.Kind {
	case token.Let:
		return p.binding(false)
	case token.Return:
		if p.inStep {
			p.fail("E_SYNTAX", "return belongs after the final step", start)
			return nil
		}
		return p.returnStatement()
	case token.If:
		condition := p.ifCondition()
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

func (p *parser) identifierStatement() ast.Stmt {
	lookahead := *p.lexer
	next, err := lookahead.Next()
	if err != nil {
		return nil
	}
	if next.Kind == token.Assign {
		start := p.current.Span
		name := p.current.Literal
		p.advance()
		p.expect(token.Assign)
		value := p.expression(0)
		p.terminator()
		n := &ast.Assign{Span: p.span(start), Name: name, Value: value}
		p.record(n, value)
		return n
	}
	// In a rule step, a newline can separate two existing identifier statements.
	if next.Kind != token.Identifier || (p.inStep && next.Span.Line != p.current.Span.Line) {
		return nil
	}
	switch p.current.Literal {
	case "var":
		afterName, err := lookahead.Next()
		if err == nil && afterName.Kind == token.Assign {
			return p.binding(true)
		}
	case "for":
		afterName, err := lookahead.Next()
		if err == nil && afterName.Kind == token.Identifier && afterName.Literal == "in" {
			return p.forStatement()
		}
	}
	return nil
}

func (p *parser) binding(mutable bool) ast.Stmt {
	start := p.current.Span
	p.advance()
	name := p.current.Literal
	p.expect(token.Identifier)
	p.expect(token.Assign)
	value := p.expression(0)
	p.terminator()
	span := p.span(start)
	var n ast.Stmt
	if mutable {
		n = &ast.Var{Span: span, Name: name, Value: value}
	} else {
		n = &ast.Let{Span: span, Name: name, Value: value}
	}
	p.record(n, value)
	return n
}

func (p *parser) forStatement() ast.Stmt {
	start := p.current.Span
	p.advance()
	name := p.current.Literal
	p.expect(token.Identifier)
	p.expectWord("in")
	iterable := p.expressionContext(0, true)
	body := p.block()
	n := &ast.For{Span: p.span(start), Name: name, Iterable: iterable, Body: body}
	p.record(n, iterable, body)
	p.optionalSemicolon()
	return n
}

func (p *parser) returnStatement() *ast.Return {
	start := p.current.Span
	p.expect(token.Return)
	value := p.expression(0)
	p.terminator()
	n := &ast.Return{Span: p.span(start), Value: value}
	p.record(n, value)
	return n
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

const prefixPrecedence = 7

func precedence(kind token.Kind) int {
	switch kind {
	case token.Or:
		return 1
	case token.And:
		return 2
	case token.Equal, token.NotEqual:
		return 3
	case token.Less, token.LessEqual, token.Greater, token.GreaterEqual:
		return 4
	case token.Plus, token.Minus:
		return 5
	case token.Star:
		return 6
	case token.LParen, token.LBracket, token.Dot:
		return 8
	}
	return 0
}
func (p *parser) expression(min int) ast.Expr {
	return p.expressionContext(min, false)
}

func (p *parser) expressionContext(min int, beforeBlock bool) ast.Expr {
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
		p.advance()
		isNamedLiteral := tok.Literal == "record" || tok.Literal == "map"
		if isNamedLiteral && p.current.Kind == token.LBrace && (!beforeBlock || p.literalBeforeBlock(tok.Literal)) {
			left = p.namedLiteral(tok)
		} else {
			left = &ast.Identifier{Span: tok.Span, Name: tok.Literal}
			p.record(left)
		}
	case token.Minus, token.Bang:
		p.advance()
		right := p.expressionContext(prefixPrecedence, beforeBlock)
		left = &ast.Prefix{Span: p.span(start), Operator: tok.Literal, Right: right}
		p.record(left, right)
	case token.LParen:
		p.advance()
		left = p.expression(0)
		p.expect(token.RParen)
	case token.Fn:
		left = p.function()
	case token.LBracket:
		left = p.list()
	case token.If:
		left = p.ifExpression()
	default:
		p.fail("E_SYNTAX", "expected expression", tok.Span)
		return nil
	}
	for p.err == nil && precedence(p.current.Kind) > min {
		switch p.current.Kind {
		case token.LParen:
			left = p.call(left)
			continue
		case token.LBracket:
			left = p.index(left)
			continue
		case token.Dot:
			left = p.member(left)
			continue
		}
		op := p.current
		power := precedence(op.Kind)
		p.advance()
		right := p.expressionContext(power, beforeBlock)
		n := &ast.Infix{Span: p.span(start), Left: left, Operator: op.Literal, Right: right}
		p.record(n, left, right)
		left = n
	}
	return left
}

func (p *parser) ifCondition() ast.Expr {
	p.expect(token.If)
	p.expect(token.LParen)
	condition := p.expression(0)
	p.expect(token.RParen)
	return condition
}

func (p *parser) ifExpression() ast.Expr {
	start := p.current.Span
	condition := p.ifCondition()
	consequence := p.valueBranch()
	p.expect(token.Else)
	alternative := p.valueBranch()
	node := &ast.IfExpr{Span: p.span(start), Condition: condition, Consequence: consequence, Alternative: alternative}
	p.record(node, condition, consequence, alternative)
	return node
}

func (p *parser) valueBranch() ast.Expr {
	p.expect(token.LBrace)
	value := p.expression(0)
	p.optionalSemicolon()
	p.expect(token.RBrace)
	return value
}

func (p *parser) list() ast.Expr {
	start := p.current.Span
	p.expect(token.LBracket)
	n := &ast.List{}
	children := []ast.Node{}
	for p.err == nil && p.current.Kind != token.RBracket {
		element := p.expression(0)
		n.Elements = append(n.Elements, element)
		children = append(children, element)
		if p.current.Kind != token.Comma {
			break
		}
		p.advance()
	}
	p.expect(token.RBracket)
	n.Span = p.span(start)
	p.record(n, children...)
	return n
}

func (p *parser) index(collection ast.Expr) ast.Expr {
	start := collection.GetSpan()
	p.expect(token.LBracket)
	index := p.expression(0)
	p.expect(token.RBracket)
	n := &ast.Index{Span: p.span(start), Collection: collection, Index: index}
	p.record(n, collection, index)
	return n
}

// After a for iterable, a brace can belong to the loop body. A key
// followed by a colon identifies a literal. An empty literal must be
// followed by the body or a dot, or be wrapped in parentheses.
func (p *parser) literalBeforeBlock(name string) bool {
	lookahead := *p.lexer
	first, err := lookahead.Next()
	if err != nil {
		return false
	}
	second, err := lookahead.Next()
	if err != nil {
		return false
	}
	if first.Kind == token.RBrace {
		return second.Kind == token.LBrace || second.Kind == token.Dot
	}
	return first.Kind == fieldKeyKind(name) && second.Kind == token.Colon
}

func fieldKeyKind(name string) token.Kind {
	if name == "map" {
		return token.String
	}
	return token.Identifier
}

func (p *parser) namedLiteral(start token.Token) ast.Expr {
	p.expect(token.LBrace)
	keyKind := fieldKeyKind(start.Literal)
	var fields []ast.NamedField
	var children []ast.Node
	for p.err == nil && p.current.Kind != token.RBrace {
		key := p.current
		p.expect(keyKind)
		p.expect(token.Colon)
		value := p.expression(0)
		fields = append(fields, ast.NamedField{Span: key.Span, Name: key.Literal, Value: value})
		children = append(children, value)
		if p.current.Kind != token.Comma {
			break
		}
		p.advance()
	}
	p.expect(token.RBrace)
	span := p.span(start.Span)
	var literal ast.Expr
	if keyKind == token.String {
		literal = &ast.Map{Span: span, Entries: fields}
	} else {
		literal = &ast.Record{Span: span, Fields: fields}
	}
	p.record(literal, children...)
	return literal
}

func (p *parser) member(receiver ast.Expr) ast.Expr {
	start := receiver.GetSpan()
	p.expect(token.Dot)
	name := p.current.Literal
	p.expect(token.Identifier)
	member := &ast.Member{Span: p.span(start), Receiver: receiver, Name: name}
	p.record(member, receiver)
	return member
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
	inStep := p.inStep
	p.inStep = false
	n.Body = p.block()
	p.inStep = inStep
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
