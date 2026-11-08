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
type Program struct {
	Statements []Stmt
	Rule       *Rule
}
type Rule struct {
	Span   diagnostic.Span
	Name   string
	Inputs []InputDecl
	Output OutputDecl
	Steps  []*Step
	Return *Return
}
type InputDecl struct {
	Span                 diagnostic.Span
	Name, Type, Currency string
}
type OutputDecl struct {
	Span                 diagnostic.Span
	Name, Type, Currency string
}
type Step struct {
	Span diagnostic.Span
	Name string
	Body *Block
}
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
type IfExpr struct {
	Span        diagnostic.Span
	Condition   Expr
	Consequence Expr
	Alternative Expr
}
type Call struct {
	Span      diagnostic.Span
	Function  Expr
	Arguments []Expr
}
type List struct {
	Span     diagnostic.Span
	Elements []Expr
}
type Index struct {
	Span       diagnostic.Span
	Collection Expr
	Index      Expr
}

// NamedField stores the source span of the field or key name.
type NamedField struct {
	Span  diagnostic.Span
	Name  string
	Value Expr
}
type Record struct {
	Span   diagnostic.Span
	Fields []NamedField
}
type Map struct {
	Span    diagnostic.Span
	Entries []NamedField
}
type Member struct {
	Span     diagnostic.Span
	Receiver Expr
	Name     string
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
type Var struct {
	Span  diagnostic.Span
	Name  string
	Value Expr
}
type Assign struct {
	Span  diagnostic.Span
	Name  string
	Value Expr
}
type For struct {
	Span     diagnostic.Span
	Name     string
	Iterable Expr
	Body     *Block
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
type Require struct {
	Span      diagnostic.Span
	Condition Expr
}
type ExpressionStatement struct {
	Span       diagnostic.Span
	Expression Expr
}

func (n *Rule) GetSpan() diagnostic.Span                { return n.Span }
func (n *InputDecl) GetSpan() diagnostic.Span           { return n.Span }
func (n *OutputDecl) GetSpan() diagnostic.Span          { return n.Span }
func (n *Step) GetSpan() diagnostic.Span                { return n.Span }
func (n *Number) GetSpan() diagnostic.Span              { return n.Span }
func (n *String) GetSpan() diagnostic.Span              { return n.Span }
func (n *Bool) GetSpan() diagnostic.Span                { return n.Span }
func (n *Identifier) GetSpan() diagnostic.Span          { return n.Span }
func (n *Prefix) GetSpan() diagnostic.Span              { return n.Span }
func (n *Infix) GetSpan() diagnostic.Span               { return n.Span }
func (n *IfExpr) GetSpan() diagnostic.Span              { return n.Span }
func (n *Call) GetSpan() diagnostic.Span                { return n.Span }
func (n *List) GetSpan() diagnostic.Span                { return n.Span }
func (n *Index) GetSpan() diagnostic.Span               { return n.Span }
func (n *Record) GetSpan() diagnostic.Span              { return n.Span }
func (n *Map) GetSpan() diagnostic.Span                 { return n.Span }
func (n *Member) GetSpan() diagnostic.Span              { return n.Span }
func (n *Function) GetSpan() diagnostic.Span            { return n.Span }
func (n *Block) GetSpan() diagnostic.Span               { return n.Span }
func (n *Let) GetSpan() diagnostic.Span                 { return n.Span }
func (n *Var) GetSpan() diagnostic.Span                 { return n.Span }
func (n *Assign) GetSpan() diagnostic.Span              { return n.Span }
func (n *For) GetSpan() diagnostic.Span                 { return n.Span }
func (n *If) GetSpan() diagnostic.Span                  { return n.Span }
func (n *Return) GetSpan() diagnostic.Span              { return n.Span }
func (n *Require) GetSpan() diagnostic.Span             { return n.Span }
func (n *ExpressionStatement) GetSpan() diagnostic.Span { return n.Span }
func (*Number) exprNode()                               {}
func (*String) exprNode()                               {}
func (*Bool) exprNode()                                 {}
func (*Identifier) exprNode()                           {}
func (*Prefix) exprNode()                               {}
func (*Infix) exprNode()                                {}
func (*IfExpr) exprNode()                               {}
func (*Call) exprNode()                                 {}
func (*List) exprNode()                                 {}
func (*Index) exprNode()                                {}
func (*Record) exprNode()                               {}
func (*Map) exprNode()                                  {}
func (*Member) exprNode()                               {}
func (*Function) exprNode()                             {}
func (*Let) stmtNode()                                  {}
func (*Var) stmtNode()                                  {}
func (*Assign) stmtNode()                               {}
func (*For) stmtNode()                                  {}
func (*If) stmtNode()                                   {}
func (*Return) stmtNode()                               {}
func (*Require) stmtNode()                              {}
func (*ExpressionStatement) stmtNode()                  {}
