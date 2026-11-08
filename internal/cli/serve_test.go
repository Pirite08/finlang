package cli

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type serveStartup chan string

func (s serveStartup) Write(data []byte) (int, error) {
	s <- string(data)
	return len(data), nil
}

func TestServeSubcommandHelp(t *testing.T) {
	var stderr bytes.Buffer
	if code := Run([]string{"serve", "-h"}, strings.NewReader(""), io.Discard, &stderr); code != 0 ||
		!strings.Contains(stderr.String(), "-app") {
		t.Fatalf("serve help exit=%d, stderr=%q", code, stderr.String())
	}
}

func TestServeTransferQuotesOverHTTP(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	started := make(serveStartup, 4)
	done := make(chan int, 1)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		done <- runServe(ctx, []string{"-app", "../../examples/transfer_quotes_app.json", "-port", "0"}, started)
	}()
	defer func() {
		cancel()
		<-stopped
	}()

	var url string
	select {
	case line := <-started:
		const prefix = "fin serve listening on "
		if !strings.HasPrefix(line, prefix+"http://127.0.0.1:") {
			t.Fatalf("expected loopback listener, got %q", line)
		}
		url = strings.TrimSpace(strings.TrimPrefix(line, prefix)) + "/transfer-quotes"
	case code := <-done:
		t.Fatalf("serve exited before listening, code=%d", code)
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not start")
	}

	client := &http.Client{Timeout: 5 * time.Second}
	const valid = `{"schema_version":"1","rule":"transfer_fee_krw_v1","inputs":{"amount":{"currency":"KRW","amount":"10000"}}}`
	for _, tc := range []struct {
		name string
		body string
		code int
		want string
	}{
		{"quote", valid, http.StatusOK, `"result":{"currency":"KRW","amount":"500"}`},
		{"wrong currency", strings.Replace(valid, "KRW", "USD", 1), http.StatusBadRequest, `"code":"E_CONTRACT_INPUT"`},
		{"fractional won", strings.Replace(valid, "10000", "10000.01", 1), http.StatusBadRequest, `"code":"E_CONTRACT_INPUT"`},
		{"outside contract range", strings.Replace(valid, "10000", "0", 1), http.StatusBadRequest, `"code":"E_CONTRACT_INPUT"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response, err := client.Post(url, "application/json", strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil || response.StatusCode != tc.code || !bytes.Contains(body, []byte(tc.want)) {
				t.Fatalf("status=%d body=%s err=%v, want %d containing %s", response.StatusCode, body, err, tc.code, tc.want)
			}
		})
	}
	cancel()
	if code := <-done; code != 0 {
		t.Fatalf("serve shutdown exit=%d", code)
	}
}
