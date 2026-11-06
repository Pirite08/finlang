package fxfeed

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Pirite08/finlang/internal/fx"
)

const latestURL = "/latest?base=USD&target=KRW"

func getLatest(receiver *Receiver) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	receiver.ServeHTTP(response, httptest.NewRequest(http.MethodGet, latestURL, nil))
	return response
}

func TestReceiverServesExactQuoteUntilExpiry(t *testing.T) {
	now := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	receiver := New(fx.LivePolicy{MaxAge: time.Minute})
	receiver.now = func() time.Time { return now }
	if got := getLatest(receiver).Code; got != http.StatusServiceUnavailable {
		t.Fatalf("before first quote got status %d", got)
	}
	quote := fx.Quote{Base: "USD", Target: "KRW", Rate: "1350.250000000000000001", Source: "test-feed", AsOf: now}
	if !receiver.Publish(quote) {
		t.Fatal("valid quote rejected")
	}
	now = now.Add(time.Minute)
	response := getLatest(receiver)
	if response.Code != http.StatusOK {
		t.Fatalf("at max age got status %d: %s", response.Code, response.Body)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	var payload fx.Quote
	decoder := json.NewDecoder(response.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Base != quote.Base || payload.Target != quote.Target || payload.Rate != quote.Rate ||
		payload.Source != quote.Source || !payload.AsOf.Equal(quote.AsOf) {
		t.Fatalf("decoded quote = %+v, want %+v", payload, quote)
	}
	now = now.Add(time.Nanosecond)
	if got := getLatest(receiver).Code; got != http.StatusServiceUnavailable {
		t.Fatalf("after expiry got status %d", got)
	}
}

func TestReceiverRejectsInvalidAndOlderQuotes(t *testing.T) {
	now := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	receiver := New(fx.LivePolicy{MaxAge: time.Minute})
	receiver.now = func() time.Time { return now }
	quote := fx.Quote{Base: "USD", Target: "KRW", Rate: "1350", Source: "test-feed", AsOf: now}
	if !receiver.Publish(quote) {
		t.Fatal("valid quote rejected")
	}
	for _, tc := range []struct {
		name   string
		change func(*fx.Quote)
	}{
		{"wrong direction", func(q *fx.Quote) { q.Base, q.Target = q.Target, q.Base }},
		{"nonpositive rate", func(q *fx.Quote) { q.Rate = "0" }},
		{"outlier below range", func(q *fx.Quote) { q.Rate = "1" }},
		{"outlier above range", func(q *fx.Quote) { q.Rate = "3000.000000000000000001" }},
		{"future timestamp", func(q *fx.Quote) { q.AsOf = now.Add(time.Nanosecond) }},
		{"older timestamp", func(q *fx.Quote) { q.AsOf = now.Add(-time.Nanosecond) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rejected := quote
			rejected.Source = "wrong-tick"
			tc.change(&rejected)
			if receiver.Publish(rejected) {
				t.Fatal("invalid quote was accepted")
			}
			var current fx.Quote
			if err := json.Unmarshal(getLatest(receiver).Body.Bytes(), &current); err != nil {
				t.Fatal(err)
			}
			if current.Source != quote.Source || current.Rate != quote.Rate {
				t.Fatalf("rejected tick replaced current quote: %+v", current)
			}
		})
	}
	quote.Rate = "1351.5"
	if !receiver.Publish(quote) {
		t.Fatal("same-second update was rejected")
	}
	var current fx.Quote
	if err := json.Unmarshal(getLatest(receiver).Body.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	if current.Rate != quote.Rate {
		t.Fatalf("same-second update was not retained: %+v", current)
	}
}

func TestReceiverRejectsUnsupportedRequests(t *testing.T) {
	receiver := New(fx.LivePolicy{MaxAge: time.Minute})
	for _, tc := range []struct {
		name, method, path string
		status             int
	}{
		{"wrong path", http.MethodGet, "/other?base=USD&target=KRW", http.StatusNotFound},
		{"wrong method", http.MethodPost, latestURL, http.StatusMethodNotAllowed},
		{"wrong pair", http.MethodGet, "/latest?base=KRW&target=USD", http.StatusBadRequest},
		{"missing target", http.MethodGet, "/latest?base=USD", http.StatusBadRequest},
		{"ambiguous pair", http.MethodGet, latestURL + "&base=KRW", http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			receiver.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
			if response.Code != tc.status {
				t.Fatalf("got status %d, want %d", response.Code, tc.status)
			}
		})
	}
}
