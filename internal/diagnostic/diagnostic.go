// Package diagnostic defines stable error codes and source locations.
package diagnostic

import (
	"errors"
	"fmt"
)

type Code string

type Span struct {
	File   string `json:"file"`
	Start  int    `json:"start"`
	End    int    `json:"end"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type Error struct {
	Code    Code
	Message string
	Span    *Span
}

func (e *Error) Error() string {
	if e.Span != nil {
		return fmt.Sprintf("%s:%d:%d: %s: %s", e.Span.File, e.Span.Line, e.Span.Column, e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func New(code Code, message string) *Error {
	return &Error{Code: code, Message: message}
}

// WithSpan preserves an inner source location and never mutates a shared error.
func WithSpan(err error, span Span) error {
	if err == nil {
		return nil
	}
	var original *Error
	if errors.As(err, &original) {
		result := *original
		if result.Span == nil {
			result.Span = &span
		}
		return &result
	}
	return &Error{Code: "E_INTERNAL", Message: err.Error(), Span: &span}
}

func CodeOf(err error) Code {
	if err == nil {
		return ""
	}
	var typed *Error
	if errors.As(err, &typed) {
		return typed.Code
	}
	return "E_INTERNAL"
}
