// Package fx loads exact, directional exchange-rate quotes from live and saved sources.
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

const (
	maxQuoteBytes     = 4096
	maxSnapshotQuotes = 2
	maxSnapshotBytes  = 2*maxQuoteBytes + 3
)

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

// Snapshot holds validated quotes and performs lookups entirely in memory.
type Snapshot struct {
	quotes []Quote
}

// LoadFixed loads one quote for reproducible runs, including historical replay.
func LoadFixed(r io.Reader) (*Snapshot, error) {
	quote, err := decodeQuote(r)
	if err != nil {
		return nil, err
	}
	return NewSnapshot([]Quote{quote})
}

// LoadSnapshot loads a JSON array of up to two exact, directional quotes.
func LoadSnapshot(r io.Reader) (*Snapshot, error) {
	d, err := quoteDecoder(r, maxSnapshotBytes)
	if err != nil {
		return nil, err
	}
	if token, err := d.Token(); err != nil || token != json.Delim('[') {
		return nil, diagnostic.New("E_FX_QUOTE", "an FX snapshot must be an array of quotes")
	}
	var quotes []Quote
	for d.More() {
		if len(quotes) == maxSnapshotQuotes {
			return nil, snapshotSizeError()
		}
		quote, err := decodeQuoteObject(d)
		if err != nil {
			return nil, err
		}
		quotes = append(quotes, quote)
	}
	if token, err := d.Token(); err != nil || token != json.Delim(']') {
		return nil, diagnostic.New("E_FX_QUOTE", "invalid FX snapshot array")
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, diagnostic.New("E_FX_QUOTE", "unexpected data after FX snapshot")
	}
	return NewSnapshot(quotes)
}

// NewSnapshot validates and copies recorded quotes without applying live expiry.
func NewSnapshot(quotes []Quote) (*Snapshot, error) {
	if len(quotes) > maxSnapshotQuotes {
		return nil, snapshotSizeError()
	}
	limits := config.DefaultLimits()
	for i, quote := range quotes {
		if _, err := quote.Validate(quote.Base, quote.Target, limits); err != nil {
			return nil, err
		}
		if i > 0 && quote.Base == quotes[0].Base && quote.Target == quotes[0].Target {
			return nil, diagnostic.New("E_FX_QUOTE", "an FX snapshot cannot repeat a direction")
		}
	}
	return &Snapshot{quotes: slices.Clone(quotes)}, nil
}

func snapshotSizeError() error {
	return diagnostic.New("E_FX_QUOTE", "an FX snapshot can contain at most two directions")
}

// Latest returns the stored quote for the requested direction.
func (s *Snapshot) Latest(base, target string) (Quote, error) {
	for _, quote := range s.quotes {
		if base == quote.Base && target == quote.Target {
			return quote, nil
		}
	}
	return Quote{}, pairError()
}

func quoteDecoder(r io.Reader, maxBytes int64) (*json.Decoder, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil || int64(len(data)) > maxBytes || !utf8.Valid(data) {
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
	d, err := quoteDecoder(r, maxQuoteBytes)
	if err != nil {
		return Quote{}, err
	}
	quote, err := decodeQuoteObject(d)
	if err != nil {
		return Quote{}, err
	}
	if _, err := d.Token(); err != io.EOF {
		return Quote{}, quoteError()
	}
	return quote, nil
}

func decodeQuoteObject(d *json.Decoder) (Quote, error) {
	fields, err := stringFields(d, "base", "target", "rate", "source", "as_of")
	if err != nil || len(fields) != 5 {
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
