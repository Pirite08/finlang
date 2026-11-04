// Package fx loads exact, directional exchange-rate quotes. Providers fetch a
// quote on each call; the interpreter owns the snapshot for one execution.
package fx

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/decimal"
	"github.com/Pirite08/finlang/internal/diagnostic"
)

const maxQuoteBytes = 4096

// The Go time parser also accepts comma fractions and offsets such as +00:60.
// Check the wire shape before parsing so a malformed quote cannot enter a trace.
var quoteTimestamp = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]{1,9})?(Z|[+-]([01][0-9]|2[0-3]):[0-5][0-9])$`)

// Quote is an exact number of target currency units per base currency unit.
type Quote struct {
	Base   string    `json:"base"`
	Target string    `json:"target"`
	Rate   string    `json:"rate"`
	Source string    `json:"source"`
	AsOf   time.Time `json:"as_of"`
}

type Provider interface {
	Latest(base, target string) (Quote, error)
}

func validPair(base, target string) bool {
	return (base == "USD" && target == "KRW") || (base == "KRW" && target == "USD")
}

func pairError() error {
	return diagnostic.New("E_FX_PAIR", "quote must match the requested USD/KRW or KRW/USD direction")
}

func quoteError() error {
	return diagnostic.New("E_FX_QUOTE", "expected one valid exchange quote with exact decimal rate, source and timestamp")
}

// Validate checks the direction and metadata and parses the rate directly as a
// Decimal. The caller sets the numeric limits for its own execution.
func (q Quote) Validate(base, target string, limits config.Limits) (decimal.Decimal, error) {
	if !validPair(base, target) || q.Base != base || q.Target != target {
		return decimal.Decimal{}, pairError()
	}
	if strings.TrimSpace(q.Source) == "" || len(q.Source) > 256 || !utf8.ValidString(q.Source) || strings.ContainsFunc(q.Source, unicode.IsControl) ||
		q.AsOf.IsZero() || q.AsOf.Year() < 1 {
		return decimal.Decimal{}, quoteError()
	}
	if _, err := q.AsOf.MarshalJSON(); err != nil {
		return decimal.Decimal{}, quoteError()
	}
	rate, err := decimal.Parse(q.Rate, limits)
	if err != nil || decimal.Cmp(rate, decimal.Decimal{}) <= 0 {
		return decimal.Decimal{}, quoteError()
	}
	return rate, nil
}

type fixedProvider struct{ quote Quote }

// LoadFixed loads one quote for reproducible runs, including historical replay.
func LoadFixed(r io.Reader) (Provider, error) {
	quote, err := decodeQuote(r)
	if err != nil {
		return nil, err
	}
	if _, err := quote.Validate(quote.Base, quote.Target, config.DefaultLimits()); err != nil {
		return nil, err
	}
	return fixedProvider{quote: quote}, nil
}

func (p fixedProvider) Latest(base, target string) (Quote, error) {
	if base != p.quote.Base || target != p.quote.Target {
		return Quote{}, pairError()
	}
	return p.quote, nil
}

func quoteDecoder(r io.Reader) (*json.Decoder, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxQuoteBytes+1))
	if err != nil || len(data) > maxQuoteBytes || !utf8.Valid(data) {
		return nil, quoteError()
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	return d, nil
}

// stringFields checks decoded field names, including duplicates expressed using
// JSON escapes. Quoted strings preserve numeric input without a float conversion.
func stringFields(d *json.Decoder, keys ...string) (map[string]string, error) {
	if token, err := d.Token(); err != nil || token != json.Delim('{') {
		return nil, quoteError()
	}
	values := make(map[string]string, len(keys))
	for d.More() {
		token, err := d.Token()
		key, ok := token.(string)
		if err != nil || !ok {
			return nil, quoteError()
		}
		if _, seen := values[key]; seen || !slices.Contains(keys, key) {
			return nil, quoteError()
		}
		token, err = d.Token()
		value, ok := token.(string)
		if err != nil || !ok {
			return nil, quoteError()
		}
		values[key] = value
	}
	if token, err := d.Token(); err != nil || token != json.Delim('}') {
		return nil, quoteError()
	}
	return values, nil
}

func decodeQuote(r io.Reader) (Quote, error) {
	d, err := quoteDecoder(r)
	if err != nil {
		return Quote{}, err
	}
	fields, err := stringFields(d, "base", "target", "rate", "source", "as_of")
	if err != nil || len(fields) != 5 {
		return Quote{}, quoteError()
	}
	if _, err := d.Token(); err != io.EOF {
		return Quote{}, quoteError()
	}
	if !quoteTimestamp.MatchString(fields["as_of"]) {
		return Quote{}, quoteError()
	}
	asOf, err := time.Parse(time.RFC3339Nano, fields["as_of"])
	if err != nil {
		return Quote{}, quoteError()
	}
	return Quote{Base: fields["base"], Target: fields["target"], Rate: fields["rate"], Source: fields["source"], AsOf: asOf}, nil
}
