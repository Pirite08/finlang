package fx

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/Pirite08/finlang/internal/diagnostic"
)

const (
	alphaEndpoint   = "https://www.alphavantage.co/query"
	alphaTimeLayout = "2006-01-02 15:04:05"
)

type alphaVantageProvider struct {
	feed   *httpProvider
	apiKey string
}

// NewAlphaVantage retrieves the provider's CURRENCY_EXCHANGE_RATE quote using
// an API key. The normalized quote retains the original decimal rate string.
func NewAlphaVantage(apiKey string, client *http.Client, maxAge time.Duration) (Provider, error) {
	if apiKey == "" || len(apiKey) > 256 || strings.ContainsFunc(apiKey, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) {
		return nil, diagnostic.New("E_FX_CONFIG", "a valid Alpha Vantage API key is required")
	}
	feed, err := newHTTPProvider(alphaEndpoint, client, maxAge)
	if err != nil {
		return nil, err
	}
	return &alphaVantageProvider{feed: feed, apiKey: apiKey}, nil
}

func (p *alphaVantageProvider) Latest(base, target string) (Quote, error) {
	query := url.Values{
		"function":      {"CURRENCY_EXCHANGE_RATE"},
		"from_currency": {base},
		"to_currency":   {target},
		"apikey":        {p.apiKey},
	}
	return p.feed.fetch(base, target, query, decodeAlphaVantage)
}

func decodeAlphaVantage(r io.Reader) (Quote, error) {
	d, err := quoteDecoder(r)
	if err != nil {
		return Quote{}, err
	}
	if token, err := d.Token(); err != nil || token != json.Delim('{') {
		return Quote{}, quoteError()
	}
	key, err := d.Token()
	if err != nil {
		return Quote{}, quoteError()
	}
	switch key {
	case "Note", "Information", "Error Message":
		return Quote{}, diagnostic.New("E_FX_FETCH", "Alpha Vantage did not provide a quote")
	case "Realtime Currency Exchange Rate":
	default:
		return Quote{}, quoteError()
	}
	fields, err := stringFields(d, "1. From_Currency Code", "2. From_Currency Name", "3. To_Currency Code", "4. To_Currency Name",
		"5. Exchange Rate", "6. Last Refreshed", "7. Time Zone", "8. Bid Price", "9. Ask Price")
	if err != nil || fields["7. Time Zone"] != "UTC" || fields["1. From_Currency Code"] == "" || fields["3. To_Currency Code"] == "" {
		return Quote{}, quoteError()
	}
	if token, err := d.Token(); err != nil || token != json.Delim('}') {
		return Quote{}, quoteError()
	}
	if _, err := d.Token(); err != io.EOF {
		return Quote{}, quoteError()
	}
	asOf, err := time.Parse(alphaTimeLayout, fields["6. Last Refreshed"])
	if err != nil || asOf.Format(alphaTimeLayout) != fields["6. Last Refreshed"] {
		return Quote{}, quoteError()
	}
	return Quote{
		Base: fields["1. From_Currency Code"], Target: fields["3. To_Currency Code"],
		Rate: fields["5. Exchange Rate"], Source: "alpha-vantage", AsOf: asOf,
	}, nil
}
