package money_test

import (
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/decimal"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/money"
)

var limits = config.DefaultLimits()

func number(t *testing.T, text string) decimal.Decimal {
	t.Helper()
	n, err := decimal.Parse(text, limits)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func amount(t *testing.T, currency, text string) money.Money {
	t.Helper()
	m, err := money.New(currency, number(t, text))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	if string(diagnostic.CodeOf(err)) != code {
		t.Fatalf("error = %v; want %s", err, code)
	}
}

func wantMoney(t *testing.T, got money.Money, err error, currency, value string) {
	t.Helper()
	if err != nil || got.Currency() != currency || got.Amount().String() != value {
		t.Fatalf("got %s %s, %v; want %s %s", got.Currency(), got.Amount(), err, currency, value)
	}
}

func TestCurrencyRules(t *testing.T) {
	usd, krw := amount(t, "USD", "0.335"), amount(t, "KRW", "0.335")
	if usd.Amount().String() != "0.335" || krw.Amount().String() != "0.335" {
		t.Fatal("construction rounded amount")
	}
	sum, err := money.Add(usd, amount(t, "USD", "0.665"), limits)
	wantMoney(t, sum, err, "USD", "1")
	difference, err := money.Sub(usd, amount(t, "USD", "0.5"), limits)
	wantMoney(t, difference, err, "USD", "-0.165")
	for _, tc := range []struct {
		right string
		want  int
	}{{"0.3350", 0}, {"0.334", 1}, {"0.336", -1}} {
		got, err := money.Cmp(usd, amount(t, "USD", tc.right))
		if err != nil || got != tc.want {
			t.Fatalf("Cmp = %d,%v; want %d", got, err, tc.want)
		}
	}
	_, err = money.Add(usd, krw, limits)
	wantCode(t, err, "E_CURRENCY_MISMATCH")
	_, err = money.Sub(usd, krw, limits)
	wantCode(t, err, "E_CURRENCY_MISMATCH")
	_, err = money.Cmp(usd, krw)
	wantCode(t, err, "E_CURRENCY_MISMATCH")
	for _, code := range []string{"", "EUR", "usd", " USD", "USD ", "US"} {
		_, err = money.New(code, number(t, "1"))
		wantCode(t, err, "E_UNSUPPORTED_CURRENCY")
	}
}

func TestInvalidMoneyRejected(t *testing.T) {
	var invalid money.Money
	valid := amount(t, "USD", "1")
	for _, pair := range [][2]money.Money{{invalid, valid}, {valid, invalid}, {invalid, invalid}} {
		_, err := money.Add(pair[0], pair[1], limits)
		wantCode(t, err, "E_UNSUPPORTED_CURRENCY")
		_, err = money.Sub(pair[0], pair[1], limits)
		wantCode(t, err, "E_UNSUPPORTED_CURRENCY")
		_, err = money.Cmp(pair[0], pair[1])
		wantCode(t, err, "E_UNSUPPORTED_CURRENCY")
	}
	_, err := money.Mul(invalid, number(t, "0"), limits)
	wantCode(t, err, "E_UNSUPPORTED_CURRENCY")
	_, _, err = money.Div(invalid, number(t, "1"), 2, decimal.HalfEven, limits)
	wantCode(t, err, "E_UNSUPPORTED_CURRENCY")
	_, _, err = money.Settle(invalid, 2, decimal.HalfEven, limits)
	wantCode(t, err, "E_UNSUPPORTED_CURRENCY")
	_, err = money.FormatAt(invalid, 2, limits)
	wantCode(t, err, "E_UNSUPPORTED_CURRENCY")
	zero := amount(t, "KRW", "0")
	got, err := money.Add(zero, amount(t, "KRW", "0.1"), limits)
	wantMoney(t, got, err, "KRW", "0.1")
}

func TestSettlementTiming(t *testing.T) {
	raw := amount(t, "USD", "0.335")
	single, inexact, err := money.Settle(raw, 2, decimal.HalfEven, limits)
	wantMoney(t, single, err, "USD", "0.34")
	if !inexact {
		t.Fatal("settlement must report inexact")
	}
	individual, err := money.Mul(single, number(t, "3"), limits)
	wantMoney(t, individual, err, "USD", "1.02")
	aggregate, err := money.Mul(raw, number(t, "3"), limits)
	wantMoney(t, aggregate, err, "USD", "1.005")
	settled, inexact, err := money.Settle(aggregate, 2, decimal.HalfEven, limits)
	wantMoney(t, settled, err, "USD", "1")
	if !inexact {
		t.Fatal("aggregate settlement must report inexact")
	}
}

func TestMoneyRounding(t *testing.T) {
	for _, tc := range []struct {
		input   string
		mode    decimal.Mode
		want    string
		inexact bool
	}{
		{"-1.005", decimal.HalfEven, "-1", true},
		{"-1.015", decimal.HalfEven, "-1.02", true},
		{"-1.005", decimal.HalfUp, "-1.01", true},
		{"-1.019", decimal.TowardZero, "-1.01", true},
		{"1.2", decimal.Exact, "1.2", false},
		{"0", decimal.HalfUp, "0", false},
	} {
		got, inexact, err := money.Settle(amount(t, "USD", tc.input), 2, tc.mode, limits)
		wantMoney(t, got, err, "USD", tc.want)
		if inexact != tc.inexact {
			t.Fatalf("%s: inexact = %v", tc.input, inexact)
		}
	}
	_, inexact, err := money.Settle(amount(t, "USD", "1.005"), 2, decimal.Exact, limits)
	wantCode(t, err, "E_INEXACT")
	if inexact {
		t.Fatal("error result must not claim a successful inexact settlement")
	}
	_, _, err = money.Settle(amount(t, "USD", "1"), 2, decimal.Mode("BAD"), limits)
	wantCode(t, err, "E_ROUNDING_MODE")
}

func TestMoneyDivision(t *testing.T) {
	for _, tc := range []struct {
		numerator, divisor string
		mode               decimal.Mode
		want               string
		inexact            bool
	}{
		{"1", "3", decimal.HalfEven, "0.33", true},
		{"-1.005", "1", decimal.HalfUp, "-1.01", true},
		{"1", "4", decimal.Exact, "0.25", false},
		{"0", "3", decimal.Exact, "0", false},
	} {
		got, inexact, err := money.Div(amount(t, "KRW", tc.numerator), number(t, tc.divisor), 2, tc.mode, limits)
		wantMoney(t, got, err, "KRW", tc.want)
		if inexact != tc.inexact {
			t.Fatalf("%s/%s inexact = %v", tc.numerator, tc.divisor, inexact)
		}
	}
	_, _, err := money.Div(amount(t, "USD", "1"), number(t, "0"), 2, decimal.HalfEven, limits)
	wantCode(t, err, "E_DIVISION_BY_ZERO")
	_, _, err = money.Div(amount(t, "USD", "1"), number(t, "3"), 2, decimal.Exact, limits)
	wantCode(t, err, "E_INEXACT")
}

func TestMoneyFormat(t *testing.T) {
	for _, tc := range []struct {
		currency, input string
		digits          uint32
		want            string
	}{
		{"USD", "1", 2, "USD 1.00"}, {"KRW", "-0.1", 3, "KRW -0.100"}, {"USD", "0", 0, "USD 0"},
	} {
		m := amount(t, tc.currency, tc.input)
		got, err := money.FormatAt(m, tc.digits, limits)
		if err != nil || got != tc.want {
			t.Fatalf("FormatAt = %q,%v; want %q", got, err, tc.want)
		}
		if m.Amount().String() != tc.input {
			t.Fatal("format mutated amount")
		}
	}
	_, err := money.FormatAt(amount(t, "USD", "1.001"), 2, limits)
	wantCode(t, err, "E_INEXACT")
}

func TestMoneyOperandsRemainUnchanged(t *testing.T) {
	a, b := amount(t, "USD", "1.005"), amount(t, "USD", "-0.335")
	alias, exposed := a, a.Amount()
	_, _ = money.Add(a, b, limits)
	_, _ = money.Sub(a, b, limits)
	_, _ = money.Cmp(a, b)
	_, _ = money.Mul(a, number(t, "3"), limits)
	_, _, _ = money.Div(a, number(t, "3"), 2, decimal.HalfEven, limits)
	_, _, _ = money.Settle(a, 2, decimal.HalfEven, limits)
	_, _ = money.FormatAt(a, 3, limits)
	_, _ = decimal.Add(exposed, number(t, "1"), limits)
	if a.Amount().String() != "1.005" || alias.Amount().String() != "1.005" || exposed.String() != "1.005" || b.Amount().String() != "-0.335" {
		t.Fatal("Money or its Decimal alias was mutated")
	}
}

func TestMoneyLimits(t *testing.T) {
	max := amount(t, "USD", strings.Repeat("9", 128))
	_, err := money.Add(max, amount(t, "USD", "1"), limits)
	wantCode(t, err, "E_NUMERIC_LIMIT")
	_, err = money.Sub(amount(t, "USD", "-"+strings.Repeat("9", 128)), amount(t, "USD", "1"), limits)
	wantCode(t, err, "E_NUMERIC_LIMIT")
	_, err = money.Mul(max, number(t, "10"), limits)
	wantCode(t, err, "E_NUMERIC_LIMIT")
	tiny := amount(t, "USD", "0."+strings.Repeat("0", 35)+"1")
	_, err = money.Mul(tiny, number(t, "0.1"), limits)
	wantCode(t, err, "E_NUMERIC_LIMIT")
	for _, digits := range []uint32{37, ^uint32(0)} {
		_, _, err = money.Div(tiny, number(t, "1"), digits, decimal.HalfEven, limits)
		wantCode(t, err, "E_NUMERIC_LIMIT")
		_, _, err = money.Settle(tiny, digits, decimal.HalfEven, limits)
		wantCode(t, err, "E_NUMERIC_LIMIT")
		_, err = money.FormatAt(tiny, digits, limits)
		wantCode(t, err, "E_NUMERIC_LIMIT")
	}
	got, inexact, err := money.Settle(tiny, 36, decimal.Exact, limits)
	wantMoney(t, got, err, "USD", "0."+strings.Repeat("0", 35)+"1")
	if inexact {
		t.Fatal("unchanged scale-36 settlement must be exact")
	}
	text, err := money.FormatAt(tiny, 36, limits)
	if err != nil || text != "USD 0."+strings.Repeat("0", 35)+"1" {
		t.Fatal(text, err)
	}
	loose := limits
	loose.MaxCoefficientDigits = 129
	loose.MaxScale = 37
	for _, input := range []string{strings.Repeat("9", 129), "0." + strings.Repeat("0", 36) + "1"} {
		d, parseErr := decimal.Parse(input, loose)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		_, err = money.New("USD", d)
		wantCode(t, err, "E_NUMERIC_LIMIT")
	}
	tight := limits
	tight.MaxScale = 1
	tight.MaxCoefficientDigits = 2
	_, err = money.Add(amount(t, "USD", "9"), amount(t, "USD", "2"), tight)
	if err != nil {
		t.Fatal(err)
	}
	_, err = money.Mul(amount(t, "USD", "0.1"), number(t, "0.1"), tight)
	wantCode(t, err, "E_NUMERIC_LIMIT")
	_, _, err = money.Settle(amount(t, "USD", "99.5"), 0, decimal.HalfUp, tight)
	wantCode(t, err, "E_NUMERIC_LIMIT")
}
