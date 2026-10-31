package compat

import (
	"fmt"
	"testing"
)

type outcome struct {
	Status     string `json:"status"`
	Currency   string `json:"currency"`
	Amount     string `json:"amount,omitempty"`
	ErrorClass string `json:"error_class,omitempty"`
	Branch     string `json:"branch"`
}

// These mutations must not be hidden by numeric coercion or success-only checks.
func TestComparatorDetectsDifferences(t *testing.T) {
	good := outcome{Status: "ok", Currency: "USD", Amount: "1.00", Branch: "low"}
	for _, tc := range []struct {
		name string
		got  outcome
	}{
		{"wrong value", outcome{Status: "ok", Currency: "USD", Amount: "1.01", Branch: "low"}},
		{"wrong scale", outcome{Status: "ok", Currency: "USD", Amount: "1.0", Branch: "low"}},
		{"wrong currency", outcome{Status: "ok", Currency: "KRW", Amount: "1.00", Branch: "low"}},
		{"wrong branch", outcome{Status: "ok", Currency: "USD", Amount: "1.00", Branch: "high"}},
		{"wrong status", outcome{Status: "error", Currency: "USD", Amount: "1.00", Branch: "low"}},
		{"unexpected error", outcome{Status: "ok", Currency: "USD", Amount: "1.00", Branch: "low", ErrorClass: "E_LEGACY_RANGE"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := compareOutcomes(tc.got, good); err == nil {
				t.Fatalf("comparator accepted %s: %+v", tc.name, tc.got)
			}
		})
	}
	if err := compareOutcomes(good, good); err != nil {
		t.Fatal(err)
	}
	wantError := outcome{Status: "error", Currency: "USD", ErrorClass: "E_LEGACY_RANGE", Branch: "none"}
	wrongError := wantError
	wrongError.ErrorClass = "E_CONTRACT_INPUT"
	if err := compareOutcomes(wrongError, wantError); err == nil {
		t.Fatal("comparator accepted a different error class")
	}
}

func compareOutcomes(got, want outcome) error {
	if got != want {
		return fmt.Errorf("outcome differs: got %+v; want %+v", got, want)
	}
	return nil
}
