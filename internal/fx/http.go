package fx

import (
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
)

const maxRequestTime = 5 * time.Second

type httpProvider struct {
	endpoint url.URL
	client   http.Client
	maxAge   time.Duration
}

// NewHTTP uses an endpoint serving the Quote JSON format. Requests include
// base and target query parameters. Plain HTTP is limited to loopback adapters.
func NewHTTP(endpoint string, client *http.Client, maxAge time.Duration) (Provider, error) {
	provider, err := newHTTPProvider(endpoint, client, maxAge)
	if err != nil {
		return nil, err
	}
	return provider, nil
}

func newHTTPProvider(endpoint string, client *http.Client, maxAge time.Duration) (*httpProvider, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" ||
		(u.Scheme != "https" && (u.Scheme != "http" || !isLoopback(u.Hostname()))) {
		return nil, diagnostic.New("E_FX_CONFIG", "quote endpoint must be HTTPS or loopback HTTP without credentials, query or fragment")
	}
	if maxAge <= 0 {
		return nil, diagnostic.New("E_FX_CONFIG", "maximum quote age must be positive")
	}
	if client == nil {
		client = http.DefaultClient
	}
	bounded := *client
	bounded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if bounded.Timeout <= 0 || bounded.Timeout > maxRequestTime {
		bounded.Timeout = maxRequestTime
	}
	return &httpProvider{endpoint: *u, client: bounded, maxAge: maxAge}, nil
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}

func (p *httpProvider) Latest(base, target string) (Quote, error) {
	return p.fetch(base, target, url.Values{"base": {base}, "target": {target}}, decodeQuote)
}

func (p *httpProvider) fetch(base, target string, query url.Values, decode func(io.Reader) (Quote, error)) (Quote, error) {
	if !validPair(base, target) {
		return Quote{}, pairError()
	}
	endpoint := p.endpoint
	endpoint.RawQuery = query.Encode()
	req, err := http.NewRequest(http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return Quote{}, diagnostic.New("E_FX_CONFIG", "could not create quote request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-cache")
	response, err := p.client.Do(req)
	if err != nil {
		return Quote{}, diagnostic.New("E_FX_FETCH", "could not retrieve a quote from the provider")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Quote{}, diagnostic.New("E_FX_FETCH", fmt.Sprintf("quote provider returned HTTP %d", response.StatusCode))
	}
	quote, err := decode(response.Body)
	if err != nil {
		return Quote{}, err
	}
	if _, err := quote.Validate(base, target, config.DefaultLimits()); err != nil {
		return Quote{}, err
	}
	now := time.Now()
	if quote.AsOf.After(now) {
		return Quote{}, diagnostic.New("E_FX_FUTURE", "quote timestamp is in the future")
	}
	if now.Sub(quote.AsOf) > p.maxAge {
		return Quote{}, diagnostic.New("E_FX_STALE", "quote timestamp exceeds the configured maximum age")
	}
	return quote, nil
}
