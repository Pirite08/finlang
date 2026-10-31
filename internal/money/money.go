// Package money provides exact amounts with explicit currencies and settlement.
package money

import (
	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/decimal"
	"github.com/Pirite08/finlang/internal/diagnostic"
)

// Money preserves its Decimal amount until an explicit Div or Settle call.
// Its zero value has no currency and cannot participate in operations.
type Money struct {
	currency string
	amount   decimal.Decimal
}

// New accepts an exact amount without currency-dependent rounding.
func New(currency string, amount decimal.Decimal) (Money, error) {
	if err := validateCurrency(currency); err != nil {
		return Money{}, err
	}
	checked, err := decimal.Add(amount, decimal.Decimal{}, config.DefaultLimits())
	if err != nil {
		return Money{}, err
	}
	return Money{currency, checked}, nil
}

func (m Money) Currency() string        { return m.currency }
func (m Money) Amount() decimal.Decimal { return m.amount }

func validateCurrency(currency string) error {
	if currency != "KRW" && currency != "USD" {
		return diagnostic.New("E_UNSUPPORTED_CURRENCY", "supported currencies are KRW and USD")
	}
	return nil
}

func sameCurrency(a, b Money) error {
	if err := validateCurrency(a.currency); err != nil {
		return err
	}
	if err := validateCurrency(b.currency); err != nil {
		return err
	}
	if a.currency != b.currency {
		return diagnostic.New("E_CURRENCY_MISMATCH", "Money operands must have the same currency")
	}
	return nil
}

// Add returns an exact same-currency sum.
func Add(a, b Money, limits config.Limits) (Money, error) {
	if err := sameCurrency(a, b); err != nil {
		return Money{}, err
	}
	value, err := decimal.Add(a.amount, b.amount, limits)
	if err != nil {
		return Money{}, err
	}
	return Money{a.currency, value}, nil
}

// Sub returns an exact same-currency difference.
func Sub(a, b Money, limits config.Limits) (Money, error) {
	if err := sameCurrency(a, b); err != nil {
		return Money{}, err
	}
	value, err := decimal.Sub(a.amount, b.amount, limits)
	if err != nil {
		return Money{}, err
	}
	return Money{a.currency, value}, nil
}

// Cmp returns -1, 0 or +1 for same-currency amounts.
func Cmp(a, b Money) (int, error) {
	if err := sameCurrency(a, b); err != nil {
		return 0, err
	}
	return decimal.Cmp(a.amount, b.amount), nil
}

// Mul multiplies an amount by a dimensionless Decimal without rounding.
func Mul(m Money, factor decimal.Decimal, limits config.Limits) (Money, error) {
	if err := validateCurrency(m.currency); err != nil {
		return Money{}, err
	}
	value, err := decimal.Mul(m.amount, factor, limits)
	if err != nil {
		return Money{}, err
	}
	return Money{m.currency, value}, nil
}

// Div explicitly rounds a quotient; its bool reports a successful inexact result.
func Div(m Money, divisor decimal.Decimal, digits uint32, mode decimal.Mode, limits config.Limits) (Money, bool, error) {
	if err := validateCurrency(m.currency); err != nil {
		return Money{}, false, err
	}
	result, err := decimal.Div(m.amount, divisor, digits, mode, limits)
	if err != nil {
		return Money{}, false, err
	}
	return Money{m.currency, result.Value}, result.Inexact, nil
}

// Settle explicitly determines the payable amount at the requested scale.
// Its bool reports whether nonzero digits were discarded.
func Settle(m Money, digits uint32, mode decimal.Mode, limits config.Limits) (Money, bool, error) {
	if err := validateCurrency(m.currency); err != nil {
		return Money{}, false, err
	}
	result, err := decimal.Round(m.amount, digits, mode, limits)
	if err != nil {
		return Money{}, false, err
	}
	return Money{m.currency, result.Value}, result.Inexact, nil
}

// FormatAt pads zeros for display, never changing or rounding the amount.
func FormatAt(m Money, digits uint32, limits config.Limits) (string, error) {
	if err := validateCurrency(m.currency); err != nil {
		return "", err
	}
	text, err := decimal.FormatAt(m.amount, digits, limits)
	if err != nil {
		return "", err
	}
	return m.currency + " " + text, nil
}
