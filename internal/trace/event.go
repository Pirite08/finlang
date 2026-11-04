// Package trace records bounded, deterministic events from actual evaluation.
package trace

import "github.com/Pirite08/finlang/internal/diagnostic"

type Event struct {
	Sequence    int             `json:"sequence"`
	Kind        string          `json:"kind"`
	Span        diagnostic.Span `json:"source"`
	Inputs      []string        `json:"inputs"`
	Result      string          `json:"result"`
	Currency    string          `json:"currency,omitempty"`
	TargetScale *uint32         `json:"target_scale,omitempty"`
	Mode        string          `json:"mode,omitempty"`
	Inexact     bool            `json:"inexact"`
	Branch      string          `json:"branch,omitempty"`
	Step        string          `json:"step,omitempty"`
	Status      string          `json:"status,omitempty"`
	Binding     string          `json:"binding,omitempty"`
	ErrorCode   diagnostic.Code `json:"error_code,omitempty"`
}

type Sink interface{ Record(Event) }
