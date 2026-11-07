package parser

import (
	"fmt"

	"github.com/Pirite08/finlang/internal/ast"
	"github.com/Pirite08/finlang/internal/token"
)

// A rule begins with two identifiers. Looking ahead only in this case preserves
// existing programs that use rule as an identifier, including a call to rule().
func (p *parser) startsRule() bool {
	if p.err != nil || !p.isWord("rule") {
		return false
	}
	lookahead := *p.lexer
	next, err := lookahead.Next()
	return err == nil && next.Kind == token.Identifier
}

func (p *parser) isWord(word string) bool {
	return p.current.Kind == token.Identifier && p.current.Literal == word
}

func (p *parser) expectWord(word string) {
	if p.err != nil {
		return
	}
	if !p.isWord(word) {
		p.fail("E_SYNTAX", fmt.Sprintf("expected %q", word), p.current.Span)
		return
	}
	p.advance()
}

func (p *parser) optionalSemicolon() {
	if p.current.Kind == token.Semicolon {
		p.advance()
	}
}

func (p *parser) rule() *ast.Rule {
	if !p.enter() {
		return nil
	}
	defer func() { p.depth-- }()
	start := p.current.Span
	p.expectWord("rule")
	rule := &ast.Rule{Name: p.current.Literal}
	p.expect(token.Identifier)
	p.expect(token.LBrace)
	inputs := make(map[string]bool)
	for p.err == nil && p.isWord("input") {
		input := p.inputDecl()
		if inputs[input.Name] {
			p.fail("E_SYNTAX", fmt.Sprintf("duplicate input %q", input.Name), input.Span)
		}
		inputs[input.Name] = true
		rule.Inputs = append(rule.Inputs, input)
	}
	if len(rule.Inputs) == 0 {
		p.fail("E_SYNTAX", `expected "input"`, p.current.Span)
	}
	rule.Output = p.outputDecl()
	if inputs[rule.Output.Name] {
		p.fail("E_SYNTAX", "input and output must have different names", rule.Output.Span)
	}
	steps := make(map[string]bool)
	children := []ast.Node{&rule.Output}
	for i := range rule.Inputs {
		children = append(children, &rule.Inputs[i])
	}
	for p.err == nil && p.isWord("step") {
		step := p.ruleStep()
		if steps[step.Name] {
			p.fail("E_SYNTAX", fmt.Sprintf("duplicate step %q", step.Name), step.Span)
		}
		steps[step.Name] = true
		rule.Steps = append(rule.Steps, step)
		children = append(children, step)
	}
	if len(rule.Steps) == 0 {
		p.fail("E_SYNTAX", "a rule must have at least one step", p.current.Span)
	}
	rule.Return = p.returnStatement()
	if p.err == nil {
		name, ok := rule.Return.Value.(*ast.Identifier)
		if !ok || name.Name != rule.Output.Name {
			p.fail("E_SYNTAX", fmt.Sprintf("return must name the declared output %q", rule.Output.Name), rule.Return.Value.GetSpan())
		}
	}
	p.expect(token.RBrace)
	rule.Span = p.span(start)
	p.record(rule, append(children, rule.Return)...)
	p.optionalSemicolon()
	return rule
}

func (p *parser) inputDecl() ast.InputDecl {
	start := p.current.Span
	p.expectWord("input")
	decl := ast.InputDecl{Name: p.current.Literal, Type: "money"}
	p.expect(token.Identifier)
	if p.isWord("as") {
		p.advance()
		p.expectWord("decimal")
		decl.Type = "decimal"
	} else {
		decl.Currency = p.declaredCurrency()
	}
	decl.Span = p.span(start)
	p.optionalSemicolon()
	return decl
}

func (p *parser) outputDecl() ast.MoneyDecl {
	start := p.current.Span
	p.expectWord("output")
	name := p.current.Literal
	p.expect(token.Identifier)
	currency := p.declaredCurrency()
	decl := ast.MoneyDecl{Span: p.span(start), Name: name, Currency: currency}
	p.optionalSemicolon()
	return decl
}

func (p *parser) declaredCurrency() string {
	p.expectWord("in")
	currency := p.current.Literal
	p.expect(token.String)
	return currency
}

func (p *parser) ruleStep() *ast.Step {
	start := p.current.Span
	p.expectWord("step")
	name := p.current.Literal
	p.expect(token.Identifier)
	p.inStep = true
	body := p.block()
	p.inStep = false
	step := &ast.Step{Span: p.span(start), Name: name, Body: body}
	p.record(step, body)
	p.optionalSemicolon()
	return step
}
