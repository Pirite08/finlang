// Package ast defines the source-located syntax tree shared by parser and evaluator.
package ast

import "github.com/Pirite08/finlang/internal/diagnostic"

type Node interface{ GetSpan() diagnostic.Span }
type Expr interface {
	Node
	exprNode()
}
type Stmt interface {
	Node
	stmtNode()
}
type Program struct{ Statements []Stmt }
type Number struct {
	Span    diagnostic.Span
	Literal string
}
type String struct {
	Span  diagnostic.Span
	Value string
}
type Bool struct {
	Span  diagnostic.Span
	Value bool
}
type Identifier struct {
	Span diagnostic.Span
	Name string
}
type Prefix struct {
	Span     diagnostic.Span
	Operator string
	Right    Expr
}
type Infix struct {
	Span     diagnostic.Span
	Left     Expr
	Operator string
	Right    Expr
}
type Call struct {
	Span      diagnostic.Span
	Function  Expr
	Arguments []Expr
}
type Function struct {
	Span       diagnostic.Span
	Parameters []string
	Body       *Block
}
type Block struct {
	Span       diagnostic.Span
	Statements []Stmt
}
type Let struct {
	Span  diagnostic.Span
	Name  string
	Value Expr
}
type If struct {
	Span        diagnostic.Span
	Condition   Expr
	Consequence *Block
	Alternative *Block
}
type Return struct {
	Span  diagnostic.Span
	Value Expr
}
type ExpressionStatement struct {
	Span       diagnostic.Span
	Expression Expr
}

func (n *Number) GetSpan() diagnostic.Span              { return n.Span }
func (n *String) GetSpan() diagnostic.Span              { return n.Span }
func (n *Bool) GetSpan() diagnostic.Span                { return n.Span }
func (n *Identifier) GetSpan() diagnostic.Span          { return n.Span }
func (n *Prefix) GetSpan() diagnostic.Span              { return n.Span }
func (n *Infix) GetSpan() diagnostic.Span               { return n.Span }
func (n *Call) GetSpan() diagnostic.Span                { return n.Span }
func (n *Function) GetSpan() diagnostic.Span            { return n.Span }
func (n *Block) GetSpan() diagnostic.Span               { return n.Span }
func (n *Let) GetSpan() diagnostic.Span                 { return n.Span }
func (n *If) GetSpan() diagnostic.Span                  { return n.Span }
func (n *Return) GetSpan() diagnostic.Span              { return n.Span }
func (n *ExpressionStatement) GetSpan() diagnostic.Span { return n.Span }
func (*Number) exprNode()                               {}
func (*String) exprNode()                               {}
func (*Bool) exprNode()                                 {}
func (*Identifier) exprNode()                           {}
func (*Prefix) exprNode()                               {}
func (*Infix) exprNode()                                {}
func (*Call) exprNode()                                 {}
func (*Function) exprNode()                             {}
func (*Let) stmtNode()                                  {}
func (*If) stmtNode()                                   {}
func (*Return) stmtNode()                               {}
func (*ExpressionStatement) stmtNode()                  {}
