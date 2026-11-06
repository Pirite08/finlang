package fx_test

import (
	"testing"
	"time"

	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/fx"
)

func TestLivePolicyChecksExactRateBounds(t *testing.T) {
	custom, err := fx.ParseRateBounds("1300", "1400")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, base, target, rate string
		bounds                   fx.RateBounds
		want                     diagnostic.Code
	}{
		{name: "inclusive minimum", base: "USD", target: "KRW", rate: "500"},
		{name: "inclusive maximum", base: "USD", target: "KRW", rate: "3000"},
		{name: "custom minimum", base: "USD", target: "KRW", rate: "1300", bounds: custom},
		{name: "outside custom range", base: "USD", target: "KRW", rate: "1400.000000000000000001", bounds: custom, want: "E_FX_OUTLIER"},
		{name: "reverse inclusive maximum", base: "KRW", target: "USD", rate: "0.002"},
		{name: "reverse over maximum", base: "KRW", target: "USD", rate: "0.002000000000000000000000000000000001", want: "E_FX_OUTLIER"},
		{name: "reverse below reciprocal", base: "KRW", target: "USD", rate: "0.000333333333333333333333333333333333", want: "E_FX_OUTLIER"},
		{name: "reverse above reciprocal", base: "KRW", target: "USD", rate: "0.000333333333333333333333333333333334"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			quote := fx.Quote{Base: tc.base, Target: tc.target, Rate: tc.rate, Source: "fixture", AsOf: now}
			policy := fx.LivePolicy{MaxAge: time.Minute, Rates: tc.bounds}
			if err := policy.ValidateQuote(quote, tc.base, tc.target, now); diagnostic.CodeOf(err) != tc.want {
				t.Fatalf("expected diagnostic %q, got %v", tc.want, err)
			}
		})
	}
}

func TestRateBoundsRejectInvalidConfiguration(t *testing.T) {
	for _, tc := range []struct{ min, max string }{
		{"", "1400"},
		{"0", "1400"},
		{"-1", "1400"},
		{"1400", "1300"},
		{"1400.00", "1400"},
		{"1300", "1.4e3"},
	} {
		if _, err := fx.ParseRateBounds(tc.min, tc.max); diagnostic.CodeOf(err) != "E_FX_CONFIG" {
			t.Errorf("bounds [%s, %s]: expected E_FX_CONFIG, got %v", tc.min, tc.max, err)
		}
	}
}

func TestPolicySnapshotRecordsEffectiveSettings(t *testing.T) {
	custom, err := fx.ParseRateBounds("01300.2500", "1400.0")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		policy fx.LivePolicy
		want   fx.PolicySnapshot
	}{
		{fx.LivePolicy{MaxAge: time.Minute}, fx.PolicySnapshot{Version: fx.PolicyVersion, Mode: "http", MaxAge: "1m0s", MinRate: "500", MaxRate: "3000"}},
		{fx.LivePolicy{MaxAge: 75 * time.Second, Rates: custom}, fx.PolicySnapshot{Version: fx.PolicyVersion, Mode: "alpha-vantage", MaxAge: "1m15s", MinRate: "1300.25", MaxRate: "1400"}},
		{fx.LivePolicy{MaxAge: time.Minute}, fx.PolicySnapshot{Version: fx.PolicyVersion, Mode: "fixed"}},
		{fx.LivePolicy{}, fx.PolicySnapshot{Version: fx.PolicyVersion, Mode: "none"}},
	} {
		if got := tc.policy.Snapshot(tc.want.Mode); got != tc.want {
			t.Fatalf("snapshot = %+v, want %+v", got, tc.want)
		}
	}
}
