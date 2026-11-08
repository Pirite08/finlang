// Package config contains bounded defaults for the interpreter.
package config

type Limits struct {
	MaxCoefficientDigits int    `json:"max_coefficient_digits"`
	MaxScale             uint32 `json:"max_scale"`
	MaxTokenBytes        int    `json:"max_token_bytes"`
	MaxSourceBytes       int    `json:"max_source_bytes"`
	MaxSyntaxDepth       int    `json:"max_syntax_depth"`
	MaxEvalSteps         int    `json:"max_eval_steps"`
	MaxCallDepth         int    `json:"max_call_depth"`
	MaxTraceEvents       int    `json:"max_trace_events"`
	MaxCollectionItems   int    `json:"max_collection_items,omitempty"`
}

const MaxRuleCollectionItems = 1024

// RuleCollectionLimits enables the per-collection cap for the collection rule profile.
func RuleCollectionLimits(limits Limits) Limits {
	if limits.MaxCollectionItems <= 0 || limits.MaxCollectionItems > MaxRuleCollectionItems {
		limits.MaxCollectionItems = MaxRuleCollectionItems
	}
	return limits
}

func DefaultLimits() Limits {
	return Limits{
		MaxCoefficientDigits: 128,
		MaxScale:             36,
		MaxTokenBytes:        256,
		MaxSourceBytes:       1 << 20,
		MaxSyntaxDepth:       128,
		MaxEvalSteps:         100000,
		MaxCallDepth:         64,
		MaxTraceEvents:       10000,
	}
}
