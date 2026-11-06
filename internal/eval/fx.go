package eval

import (
	"github.com/Pirite08/finlang/internal/decimal"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/fx"
	"github.com/Pirite08/finlang/internal/money"
	"github.com/Pirite08/finlang/internal/trace"
)

type currencyPair struct{ base, target string }

type fxQuoteValue struct {
	quote fx.Quote
	rate  decimal.Decimal
}

func (fxQuoteValue) Kind() Kind { return "FXQuote" }

func (v fxQuoteValue) Inspect() string {
	return v.quote.Base + "/" + v.quote.Target + " " + v.quote.Rate
}

func (e *Evaluator) fxBuiltin(name string, args []Value, span diagnostic.Span) (Value, error) {
	var result Value
	if name == "rate" {
		base, baseOK := args[0].(StringValue)
		target, targetOK := args[1].(StringValue)
		if !baseOK || !targetOK {
			return nil, typeError("rate", args...)
		}
		quote, err := e.rate(currencyPair{base: string(base), target: string(target)})
		if err != nil {
			return nil, err
		}
		result = quote
	} else {
		amount, amountOK := args[0].(MoneyValue)
		quote, quoteOK := args[1].(fxQuoteValue)
		if !amountOK || !quoteOK {
			return nil, typeError("convert", args...)
		}
		if amount.Value.Currency() != quote.quote.Base {
			return nil, diagnostic.New("E_CURRENCY_MISMATCH", "amount currency must match the base currency of the FX quote")
		}
		converted, err := decimal.Mul(amount.Value.Amount(), quote.rate, e.options.Limits)
		if err != nil {
			return nil, err
		}
		value, err := money.New(quote.quote.Target, converted)
		if err != nil {
			return nil, err
		}
		result = MoneyValue{Value: value}
	}
	e.emit(trace.Event{Kind: name, Span: span}, args, result)
	return result, nil
}

func (e *Evaluator) rate(pair currencyPair) (fxQuoteValue, error) {
	if cached, ok := e.rule.rates[pair]; ok {
		return cached, nil
	}
	for _, currency := range []string{pair.base, pair.target} {
		if _, err := money.New(currency, decimal.Decimal{}); err != nil {
			return fxQuoteValue{}, err
		}
	}
	if pair.base == pair.target {
		return fxQuoteValue{}, diagnostic.New("E_FX_PAIR", "FX rate requires two different currencies")
	}
	var quote fx.Quote
	var err error
	if e.options.RuleOnly {
		if e.options.FXSnapshot == nil {
			return fxQuoteValue{}, diagnostic.New("E_FX_PROVIDER", "no prepared FX snapshot is configured")
		}
		quote, err = e.options.FXSnapshot.Latest(pair.base, pair.target)
	} else {
		if e.options.FXProvider == nil {
			return fxQuoteValue{}, diagnostic.New("E_FX_PROVIDER", "no FX provider is configured")
		}
		quote, err = e.options.FXProvider.Latest(pair.base, pair.target)
	}
	if err != nil {
		return fxQuoteValue{}, err
	}
	rate, err := quote.Validate(pair.base, pair.target, e.options.Limits)
	if err != nil {
		return fxQuoteValue{}, err
	}
	value := fxQuoteValue{quote: quote, rate: rate}
	e.rule.rates[pair] = value
	e.fxQuotes = append(e.fxQuotes, quote)
	return value, nil
}
