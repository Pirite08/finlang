package fx

import (
	"time"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/decimal"
	"github.com/Pirite08/finlang/internal/diagnostic"
)

const (
	DefaultMinRate = "500"
	DefaultMaxRate = "3000"
	PolicyVersion  = "fx-policy-v1"
)

// RateBounds defines the inclusive range of KRW per USD for a live quote.
// Its zero value applies the default range.
type RateBounds struct {
	min, max decimal.Decimal
}

var (
	rateUnit, _       = decimal.Parse("1", config.DefaultLimits())
	defaultRateBounds = func() RateBounds {
		bounds, err := ParseRateBounds(DefaultMinRate, DefaultMaxRate)
		if err != nil {
			panic(err)
		}
		return bounds
	}()
)

// ParseRateBounds accepts two positive exact decimals in increasing order.
func ParseRateBounds(min, max string) (RateBounds, error) {
	limits := config.DefaultLimits()
	lower, lowerErr := decimal.Parse(min, limits)
	upper, upperErr := decimal.Parse(max, limits)
	if lowerErr != nil || upperErr != nil || decimal.Cmp(lower, decimal.Decimal{}) <= 0 || decimal.Cmp(lower, upper) >= 0 {
		return RateBounds{}, diagnostic.New("E_FX_CONFIG", "live FX bounds require 0 < minimum < maximum in KRW per USD")
	}
	return RateBounds{min: lower, max: upper}, nil
}

func (b RateBounds) effective() RateBounds {
	if decimal.Cmp(b.min, decimal.Decimal{}) == 0 {
		return defaultRateBounds
	}
	return b
}

func (b RateBounds) contains(base string, rate decimal.Decimal, limits config.Limits) bool {
	b = b.effective()
	if base == "USD" {
		return decimal.Cmp(rate, b.min) >= 0 && decimal.Cmp(rate, b.max) <= 0
	}
	// In the reverse direction min <= 1/rate <= max is equivalent to
	// min*rate <= 1 <= max*rate. The wider limits hold both exact products.
	limits.MaxCoefficientDigits *= 2
	limits.MaxScale *= 2
	lower, lowerErr := decimal.Mul(b.min, rate, limits)
	upper, upperErr := decimal.Mul(b.max, rate, limits)
	return lowerErr == nil && upperErr == nil && decimal.Cmp(lower, rateUnit) <= 0 && decimal.Cmp(upper, rateUnit) >= 0
}

// LivePolicy governs freshness and plausibility without changing saved quotes.
type LivePolicy struct {
	MaxAge time.Duration
	Rates  RateBounds
}

// PolicySnapshot identifies the FX policy and effective live settings in audit records.
type PolicySnapshot struct {
	Version string `json:"version"`
	Mode    string `json:"mode"`
	MaxAge  string `json:"max_age,omitempty"`
	MinRate string `json:"min_rate,omitempty"`
	MaxRate string `json:"max_rate,omitempty"`
}

// Snapshot records the policy without a provider address or credentials.
// The caller supplies the provider mode, such as "http" or "fixed".
func (p LivePolicy) Snapshot(mode string) PolicySnapshot {
	snapshot := PolicySnapshot{Version: PolicyVersion, Mode: mode}
	if mode == "http" || mode == "alpha-vantage" {
		rates := p.Rates.effective()
		snapshot.MaxAge = p.MaxAge.String()
		snapshot.MinRate = rates.min.String()
		snapshot.MaxRate = rates.max.String()
	}
	return snapshot
}

// ValidateQuote validates a live quote at the supplied time.
func (p LivePolicy) ValidateQuote(q Quote, base, target string, now time.Time) error {
	if p.MaxAge <= 0 {
		return diagnostic.New("E_FX_CONFIG", "maximum quote age must be positive")
	}
	limits := config.DefaultLimits()
	rate, err := q.Validate(base, target, limits)
	if err != nil {
		return err
	}
	if q.AsOf.After(now) {
		return diagnostic.New("E_FX_FUTURE", "quote timestamp is in the future")
	}
	if now.Sub(q.AsOf) > p.MaxAge {
		return diagnostic.New("E_FX_STALE", "quote timestamp exceeds the configured maximum age")
	}
	if !p.Rates.contains(base, rate, limits) {
		return diagnostic.New("E_FX_OUTLIER", "live quote is outside the configured USD/KRW rate bounds")
	}
	return nil
}
