// Package eval executes source-located AST nodes with bounded resources.
package eval

import (
	"example.com/finlang/internal/ast"
	"example.com/finlang/internal/decimal"
)

type Kind string

const (
	KindDecimal  Kind = "Decimal"
	KindBool     Kind = "Bool"
	KindString   Kind = "String"
	KindUnit     Kind = "Unit"
	KindFunction Kind = "Function"
	KindBuiltin  Kind = "Builtin"
)

type Value interface {
	Kind() Kind
	Inspect() string
}

type DecimalValue struct{ Value decimal.Decimal }

func (DecimalValue) Kind() Kind        { return KindDecimal }
func (v DecimalValue) Inspect() string { return v.Value.String() }

type BoolValue bool

func (BoolValue) Kind() Kind { return KindBool }
func (v BoolValue) Inspect() string {
	if v {
		return "true"
	}
	return "false"
}

type StringValue string

func (StringValue) Kind() Kind        { return KindString }
func (v StringValue) Inspect() string { return string(v) }

type UnitValue struct{}

func (UnitValue) Kind() Kind      { return KindUnit }
func (UnitValue) Inspect() string { return "unit" }

type FunctionValue struct {
	parameters  []string
	body        *ast.Block
	environment *Environment
}

func (*FunctionValue) Kind() Kind      { return KindFunction }
func (*FunctionValue) Inspect() string { return "<function>" }

type builtinValue struct{ name string }

func (builtinValue) Kind() Kind        { return KindBuiltin }
func (v builtinValue) Inspect() string { return "<builtin " + v.name + ">" }
