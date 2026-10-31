// Package token defines lexical tokens with byte offsets and rune columns.
package token

import "example.com/finlang/internal/diagnostic"

type Kind string

const (
	EOF          Kind = "EOF"
	Identifier   Kind = "IDENTIFIER"
	Number       Kind = "NUMBER"
	String       Kind = "STRING"
	Let          Kind = "let"
	Fn           Kind = "fn"
	Return       Kind = "return"
	If           Kind = "if"
	Else         Kind = "else"
	True         Kind = "true"
	False        Kind = "false"
	Assign       Kind = "="
	Plus         Kind = "+"
	Minus        Kind = "-"
	Star         Kind = "*"
	Slash        Kind = "/"
	Bang         Kind = "!"
	Equal        Kind = "=="
	NotEqual     Kind = "!="
	Less         Kind = "<"
	LessEqual    Kind = "<="
	Greater      Kind = ">"
	GreaterEqual Kind = ">="
	LParen       Kind = "("
	RParen       Kind = ")"
	LBrace       Kind = "{"
	RBrace       Kind = "}"
	Comma        Kind = ","
	Semicolon    Kind = ";"
)

type Token struct {
	Kind    Kind
	Literal string
	Span    diagnostic.Span
}
