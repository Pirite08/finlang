// Package lexer scans UTF-8 source without interpreting numeric values.
package lexer

import (
	"strings"
	"unicode/utf8"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/token"
)

type Lexer struct {
	file, source         string
	offset, line, column int
	maxToken             int
}

func New(file, source string, limits config.Limits) (*Lexer, error) {
	defaults := config.DefaultLimits()
	if limits.MaxSourceBytes <= 0 {
		limits.MaxSourceBytes = defaults.MaxSourceBytes
	}
	if limits.MaxTokenBytes <= 0 {
		limits.MaxTokenBytes = defaults.MaxTokenBytes
	}
	l := &Lexer{file: file, source: source, line: 1, column: 1, maxToken: limits.MaxTokenBytes}
	if len(source) > limits.MaxSourceBytes {
		return nil, l.fail("E_SOURCE_LIMIT", "source exceeds byte limit", l.span())
	}
	if !utf8.ValidString(source) {
		for l.offset < len(source) {
			r, n := utf8.DecodeRuneInString(source[l.offset:])
			if r == utf8.RuneError && n == 1 {
				return nil, l.fail("E_SOURCE_ENCODING", "source must be valid UTF-8", l.span())
			}
			l.advance()
		}
	}
	return l, nil
}

func (l *Lexer) span() diagnostic.Span {
	return diagnostic.Span{File: l.file, Start: l.offset, End: l.offset, Line: l.line, Column: l.column}
}
func (l *Lexer) fail(code diagnostic.Code, message string, span diagnostic.Span) error {
	return diagnostic.WithSpan(diagnostic.New(code, message), span)
}
func (l *Lexer) advance() {
	r, n := utf8.DecodeRuneInString(l.source[l.offset:])
	l.offset += n
	if r == '\n' {
		l.line++
		l.column = 1
	} else {
		l.column++
	}
}
func isStart(b byte) bool       { return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b == '_' }
func isDigit(b byte) bool       { return b >= '0' && b <= '9' }
func (l *Lexer) at(b byte) bool { return l.offset < len(l.source) && l.source[l.offset] == b }
func (l *Lexer) finish(kind token.Kind, start diagnostic.Span, literal string) (token.Token, error) {
	start.End = l.offset
	return token.Token{Kind: kind, Literal: literal, Span: start}, nil
}
func (l *Lexer) checkSize(start diagnostic.Span) error {
	if l.offset-start.Start > l.maxToken {
		return l.fail("E_TOKEN_LIMIT", "token exceeds byte limit", start)
	}
	return nil
}

func (l *Lexer) Next() (token.Token, error) {
	for l.offset < len(l.source) {
		b := l.source[l.offset]
		if b == ' ' || b == '\t' || b == '\r' || b == '\n' {
			l.advance()
			continue
		}
		if b == '/' && l.offset+1 < len(l.source) && l.source[l.offset+1] == '/' {
			for l.offset < len(l.source) && !l.at('\n') {
				l.advance()
			}
			continue
		}
		break
	}
	start := l.span()
	if l.offset == len(l.source) {
		return l.finish(token.EOF, start, "")
	}
	b := l.source[l.offset]
	if isStart(b) {
		for l.offset < len(l.source) && (isStart(l.source[l.offset]) || isDigit(l.source[l.offset])) {
			l.advance()
			if e := l.checkSize(start); e != nil {
				return token.Token{}, e
			}
		}
		literal := l.source[start.Start:l.offset]
		kind := token.Identifier
		switch literal {
		case "let":
			kind = token.Let
		case "fn":
			kind = token.Fn
		case "return":
			kind = token.Return
		case "if":
			kind = token.If
		case "else":
			kind = token.Else
		case "true":
			kind = token.True
		case "false":
			kind = token.False
		}
		return l.finish(kind, start, literal)
	}
	if isDigit(b) {
		dot := false
		for l.offset < len(l.source) {
			c := l.source[l.offset]
			if isDigit(c) {
				l.advance()
			} else if c == '.' {
				if dot {
					return token.Token{}, l.fail("E_INVALID_NUMBER", "number has more than one decimal point", start)
				}
				dot = true
				l.advance()
				if l.offset == len(l.source) || !isDigit(l.source[l.offset]) {
					return token.Token{}, l.fail("E_INVALID_NUMBER", "decimal point requires following digits", start)
				}
			} else {
				break
			}
			if e := l.checkSize(start); e != nil {
				return token.Token{}, e
			}
		}
		return l.finish(token.Number, start, l.source[start.Start:l.offset])
	}
	if b == '"' {
		l.advance()
		var value strings.Builder
		for l.offset < len(l.source) {
			if l.at('"') {
				l.advance()
				return l.finish(token.String, start, value.String())
			}
			if l.at('\n') || l.at('\r') {
				return token.Token{}, l.fail("E_UNTERMINATED_STRING", "string requires a closing quote before newline", start)
			}
			if l.at('\\') {
				l.advance()
				if l.offset == len(l.source) {
					break
				}
				c := l.source[l.offset]
				l.advance()
				switch c {
				case '"':
					value.WriteByte('"')
				case '\\':
					value.WriteByte('\\')
				case 'n':
					value.WriteByte('\n')
				case 't':
					value.WriteByte('\t')
				default:
					return token.Token{}, l.fail("E_STRING_ESCAPE", "unsupported string escape", start)
				}
			} else {
				begin := l.offset
				l.advance()
				value.WriteString(l.source[begin:l.offset])
			}
			// Quotes are delimiters; the source bytes between them form the token payload.
			if l.offset-start.Start-1 > l.maxToken || value.Len() > l.maxToken {
				return token.Token{}, l.fail("E_TOKEN_LIMIT", "string exceeds byte limit", start)
			}
		}
		return token.Token{}, l.fail("E_UNTERMINATED_STRING", "string requires a closing quote", start)
	}
	l.advance()
	kind := token.Kind(string(b))
	switch b {
	case '=', '!', '<', '>':
		if l.at('=') {
			l.advance()
			kind = token.Kind(string(b) + "=")
		}
	case '+', '-', '*', '/', '(', ')', '{', '}', ',', ';':
	default:
		return token.Token{}, l.fail("E_INVALID_TOKEN", "unexpected character", start)
	}
	return l.finish(kind, start, l.source[start.Start:l.offset])
}
