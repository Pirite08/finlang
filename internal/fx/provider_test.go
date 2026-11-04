package fx_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/fx"
)

const fixedQuote = `{"base":"USD","target":"KRW","rate":"1350.125000000000000001","source":"fixture","as_of":"2024-06-01T00:00:00Z"}`

func TestFixedQuoteKeepsExactRateAndDirection(t *testing.T) {
	provider, err := fx.LoadFixed(strings.NewReader(fixedQuote))
	if err != nil {
		t.Fatal(err)
	}
	quote, err := provider.Latest("USD", "KRW")
	if err != nil {
		t.Fatal(err)
	}
	rate, err := quote.Validate("USD", "KRW", config.DefaultLimits())
	if err != nil || rate.String() != "1350.125000000000000001" || quote.Source != "fixture" || quote.AsOf.Format(time.RFC3339) != "2024-06-01T00:00:00Z" {
		t.Fatalf("unexpected exact quote: %+v, rate=%s, err=%v", quote, rate.String(), err)
	}
	if _, err := provider.Latest("KRW", "USD"); diagnostic.CodeOf(err) != "E_FX_PAIR" {
		t.Fatalf("reverse quote accepted or wrong error: %v", err)
	}
}

func TestFixedQuoteRejectsMalformedInputs(t *testing.T) {
	cases := map[string]string{
		"number":            strings.Replace(fixedQuote, `"1350.125000000000000001"`, `1350.125000000000000001`, 1),
		"zero":              strings.Replace(fixedQuote, `1350.125000000000000001`, `0`, 1),
		"negative":          strings.Replace(fixedQuote, `1350.125000000000000001`, `-1`, 1),
		"exponent":          strings.Replace(fixedQuote, `1350.125000000000000001`, `1e3`, 1),
		"missing":           strings.Replace(fixedQuote, `"source":"fixture",`, ``, 1),
		"duplicate":         strings.Replace(fixedQuote, `"source":"fixture"`, `"source":"fixture","sourc\u0065":"other"`, 1),
		"blank":             strings.Replace(fixedQuote, `"fixture"`, `" "`, 1),
		"unknown":           strings.Replace(fixedQuote, `"source"`, `"origin"`, 1),
		"bad time":          strings.Replace(fixedQuote, `2024-06-01T00:00:00Z`, `2024-06-01`, 1),
		"bad zone":          strings.Replace(fixedQuote, `2024-06-01T00:00:00Z`, `2024-06-01T00:00:00+24:00`, 1),
		"bad minute offset": strings.Replace(fixedQuote, `2024-06-01T00:00:00Z`, `2024-06-01T00:00:00+00:60`, 1),
		"comma fraction":    strings.Replace(fixedQuote, `2024-06-01T00:00:00Z`, `2024-06-01T00:00:00,1Z`, 1),
		"trailing":          fixedQuote + ` {}`,
		"oversized":         fixedQuote + strings.Repeat(" ", 5000),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := fx.LoadFixed(strings.NewReader(input)); diagnostic.CodeOf(err) != "E_FX_QUOTE" {
				t.Fatalf("expected E_FX_QUOTE, got %v", err)
			}
		})
	}
}

func TestHTTPProviderRequestsLatestEachTime(t *testing.T) {
	quotes := make(chan string, 2)
	quotes <- "1350.125000000000000001"
	quotes <- "1400.75"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Query().Get("base") != "USD" || r.URL.Query().Get("target") != "KRW" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		select {
		case rate := <-quotes:
			fmt.Fprint(w, quoteBody(rate, time.Now().Add(-time.Second)))
		default:
			w.WriteHeader(http.StatusTooManyRequests)
		}
	}))
	defer server.Close()
	provider, err := fx.NewHTTP(server.URL, server.Client(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"1350.125000000000000001", "1400.75"} {
		quote, err := provider.Latest("USD", "KRW")
		if err != nil || quote.Rate != want || quote.Source != "test-feed" {
			t.Fatalf("expected latest %s, got %+v, %v", want, quote, err)
		}
	}
}

func TestHTTPProviderRejectsUnusableQuotes(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		body string
		code diagnostic.Code
	}{
		{"stale", quoteBody("1350.25", now.Add(-2*time.Minute)), "E_FX_STALE"},
		{"future", quoteBody("1350.25", now.Add(time.Minute)), "E_FX_FUTURE"},
		{"pair", strings.Replace(quoteBody("1350.25", now), `"USD"`, `"EUR"`, 1), "E_FX_PAIR"},
		{"response shape", `{"error":"private provider diagnostic"}`, "E_FX_QUOTE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			provider, err := fx.NewHTTP(server.URL, server.Client(), time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := provider.Latest("USD", "KRW"); diagnostic.CodeOf(err) != tc.code || strings.Contains(err.Error(), "private provider diagnostic") {
				t.Fatalf("expected sanitized %s, got %v", tc.code, err)
			}
		})
	}
}

func TestHTTPProviderRejectsRedirectAndUnsafeEndpoint(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, quoteBody("1350.25", time.Now()))
	}))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer server.Close()
	provider, err := fx.NewHTTP(server.URL, server.Client(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Latest("USD", "KRW"); diagnostic.CodeOf(err) != "E_FX_FETCH" {
		t.Fatalf("redirect followed: %v", err)
	}
	for _, endpoint := range []string{"http://example.com/quote", "https://user:secret@example.com/quote", "https://example.com/quote?token=secret", "https://example.com/quote#secret"} {
		if provider, err := fx.NewHTTP(endpoint, nil, time.Minute); provider != nil || diagnostic.CodeOf(err) != "E_FX_CONFIG" || strings.Contains(err.Error(), "secret") {
			t.Fatalf("unsafe endpoint accepted or error leaked: %v", err)
		}
	}
}

func quoteBody(rate string, asOf time.Time) string {
	return fmt.Sprintf(`{"base":"USD","target":"KRW","rate":%q,"source":"test-feed","as_of":%q}`, rate, asOf.UTC().Format(time.RFC3339Nano))
}
