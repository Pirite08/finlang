package fx_test

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/fx"
)

type alphaTransport func(*http.Request) (*http.Response, error)

func (f alphaTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestAlphaVantageNormalizesQuote(t *testing.T) {
	const key = "fixture-key"
	const rate = "1350.125000000000000001"
	asOf := time.Now().UTC().Add(-time.Second).Truncate(time.Second)
	client := &http.Client{Transport: alphaTransport(func(req *http.Request) (*http.Response, error) {
		query := req.URL.Query()
		if req.Method != http.MethodGet || req.URL.Scheme != "https" || req.URL.Host != "www.alphavantage.co" || req.URL.Path != "/query" ||
			query.Get("function") != "CURRENCY_EXCHANGE_RATE" || query.Get("from_currency") != "USD" || query.Get("to_currency") != "KRW" || query.Get("apikey") != key {
			return nil, fmt.Errorf("incorrect quote request")
		}
		return alphaResponse(alphaBody(rate, asOf)), nil
	})}
	provider, err := fx.NewAlphaVantage(key, client, fx.LivePolicy{MaxAge: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	quote, err := provider.Latest("USD", "KRW")
	if err != nil || quote.Rate != rate || !quote.AsOf.Equal(asOf) || quote.Source != "alpha-vantage" {
		t.Fatalf("unexpected normalized quote: %+v %v", quote, err)
	}
}

func TestAlphaVantageSanitizesFailures(t *testing.T) {
	const key = "fixture-key"
	now := time.Now().UTC()
	cases := []struct {
		name string
		body string
		code diagnostic.Code
	}{
		{"transport", "", "E_FX_FETCH"},
		{"provider limit", `{"Note":"quota exceeded for fixture-key"}`, "E_FX_FETCH"},
		{"pair", strings.Replace(alphaBody("1350.25", now), `"KRW"`, `"USD"`, 1), "E_FX_PAIR"},
		{"stale", alphaBody("1350.25", now.Add(-2*time.Minute)), "E_FX_STALE"},
		{"outlier", alphaBody("1", now), "E_FX_OUTLIER"},
		{"time zone", strings.Replace(alphaBody("1350.25", now), `"UTC"`, `"EST"`, 1), "E_FX_QUOTE"},
		{"duplicate", strings.Replace(alphaBody("1350.25", now), `"5. Exchange Rate":"1350.25"`, `"5. Exchange Rate":"1350.25","5. Exchange Rate":"1000"`, 1), "E_FX_QUOTE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: alphaTransport(func(req *http.Request) (*http.Response, error) {
				if tc.body == "" {
					return nil, fmt.Errorf("could not access %s", req.URL)
				}
				return alphaResponse(tc.body), nil
			})}
			provider, err := fx.NewAlphaVantage(key, client, fx.LivePolicy{MaxAge: time.Minute})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := provider.Latest("USD", "KRW"); diagnostic.CodeOf(err) != tc.code || strings.Contains(err.Error(), key) {
				t.Fatalf("expected sanitized %s, got %v", tc.code, err)
			}
		})
	}
}

func alphaResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}
}

func alphaBody(rate string, asOf time.Time) string {
	return fmt.Sprintf(`{"Realtime Currency Exchange Rate":{
		"1. From_Currency Code":"USD","2. From_Currency Name":"United States Dollar",
		"3. To_Currency Code":"KRW","4. To_Currency Name":"South Korean Won",
		"5. Exchange Rate":%q,"6. Last Refreshed":%q,"7. Time Zone":"UTC",
		"8. Bid Price":"1349.00","9. Ask Price":"1351.00"}}`, rate, asOf.UTC().Format("2006-01-02 15:04:05"))
}
