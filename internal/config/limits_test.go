package config

import "testing"

func TestDefaultLimits(t *testing.T) {
	limits := DefaultLimits()
	if limits.MaxCoefficientDigits != 128 || limits.MaxScale != 36 ||
		limits.MaxTokenBytes != 256 || limits.MaxSourceBytes != 1<<20 ||
		limits.MaxSyntaxDepth != 128 || limits.MaxEvalSteps != 100000 ||
		limits.MaxCallDepth != 64 || limits.MaxTraceEvents != 10000 {
		t.Fatalf("unexpected default limits: %+v", limits)
	}
}
