// Package config contains bounded defaults for the interpreter.
package config

type Limits struct {
	MaxCoefficientDigits int
	MaxScale             uint32
	MaxTokenBytes        int
	MaxSourceBytes       int
	MaxSyntaxDepth       int
	MaxEvalSteps         int
	MaxCallDepth         int
	MaxTraceEvents       int
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
