// Package fxfeed serves the latest USD/KRW quote in finlang's HTTP quote format.
package fxfeed

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/Pirite08/finlang/internal/fx"
)

const (
	baseCurrency   = "USD"
	targetCurrency = "KRW"
)

// Receiver keeps one validated quote. It is safe to publish and read concurrently.
type Receiver struct {
	policy fx.LivePolicy
	now    func() time.Time

	mu    sync.RWMutex
	quote fx.Quote
}

// New creates a receiver that serves fresh quotes within the live policy.
func New(policy fx.LivePolicy) *Receiver {
	return &Receiver{policy: policy, now: time.Now}
}

// Publish accepts newer quotes and lets the last arrival win at equal timestamps.
func (r *Receiver) Publish(quote fx.Quote) bool {
	quote.AsOf = quote.AsOf.UTC()
	if err := r.policy.ValidateQuote(quote, baseCurrency, targetCurrency, r.now()); err != nil {
		return false
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if quote.AsOf.Before(r.quote.AsOf) {
		return false
	}
	r.quote = quote
	return true
}

func (r *Receiver) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if req.URL.Path != "/latest" {
		http.NotFound(w, req)
		return
	}
	if req.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "use GET for the latest quote", http.StatusMethodNotAllowed)
		return
	}
	params, err := url.ParseQuery(req.URL.RawQuery)
	if err != nil || len(params) != 2 || len(params["base"]) != 1 || len(params["target"]) != 1 ||
		params.Get("base") != baseCurrency || params.Get("target") != targetCurrency {
		http.Error(w, "request the USD/KRW quote with base=USD and target=KRW", http.StatusBadRequest)
		return
	}

	r.mu.RLock()
	quote := r.quote
	r.mu.RUnlock()
	if quote.AsOf.IsZero() {
		http.Error(w, "waiting for a USD/KRW quote", http.StatusServiceUnavailable)
		return
	}
	age := r.now().Sub(quote.AsOf)
	if age < 0 || age > r.policy.MaxAge {
		http.Error(w, "USD/KRW quote is outside the accepted age", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(quote)
}
