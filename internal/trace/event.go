// Package trace defines the optional event hook. Recording is a later release.
package trace

import "example.com/finlang/internal/diagnostic"

type Event struct {
	Sequence    int
	Kind        string
	Span        diagnostic.Span
	Inputs      []string
	Result      string
	Currency    string
	TargetScale *uint32
	Mode        string
	Inexact     bool
	Branch      string
}

type Sink interface{ Record(Event) }
