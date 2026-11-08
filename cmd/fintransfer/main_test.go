package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type startupMessage chan string

func (s startupMessage) Write(p []byte) (int, error) {
	s <- string(p)
	return len(p), nil
}

func TestTransferHTTPAndOfflineReplay(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	database := filepath.Join(t.TempDir(), "테스트 ledger.sqlite")
	started := make(startupMessage, 1)
	done := make(chan error, 1)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		done <- run(ctx, []string{"-db", database, "-seed-demo", "-port", "0"}, io.Discard, started)
	}()
	defer func() {
		cancel()
		<-stopped
	}()
	var endpoint string
	select {
	case line := <-started:
		const prefix = "fintransfer listening on "
		if !strings.HasPrefix(line, prefix+"http://127.0.0.1:") {
			t.Fatalf("expected loopback endpoint, got %q", line)
		}
		endpoint = strings.TrimSpace(strings.TrimPrefix(line, prefix))
	case err := <-done:
		t.Fatalf("server exited before listening: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("server did not start")
	}
	const request = `{"request_id":"transfer-001","from_account":"A","to_account":"B","currency":"KRW","amount_minor":10000}`
	const want = "{\"request_id\":\"transfer-001\",\"from_account\":\"A\",\"to_account\":\"B\",\"currency\":\"KRW\",\"amount_minor\":10000,\"fee_minor\":500,\"debit_minor\":10500}\n"
	client := &http.Client{Timeout: 5 * time.Second}
	foreignHost, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(request))
	if err != nil {
		t.Fatal(err)
	}
	foreignHost.Host = "external.invalid"
	foreignHost.Header.Set("Content-Type", "application/json")
	response, err := client.Do(foreignHost)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("nonlocal Host header returned %d, want %d", response.StatusCode, http.StatusForbidden)
	}
	for _, attempt := range []string{"initial transfer", "retry"} {
		response, err := client.Post(endpoint, "application/json", strings.NewReader(request))
		if err != nil {
			t.Fatalf("%s: %v", attempt, err)
		}
		result, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK || string(result) != want {
			t.Fatalf("%s: status=%d result=%s err=%v", attempt, response.StatusCode, result, err)
		}
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	var replay bytes.Buffer
	if err := run(t.Context(), []string{"-db", database, "-replay", "transfer-001"}, &replay, io.Discard); err != nil || replay.String() != want {
		t.Fatalf("offline replay: result=%s err=%v", &replay, err)
	}
}

func TestReplayDoesNotCreateMissingDatabase(t *testing.T) {
	database := filepath.Join(t.TempDir(), "missing.sqlite")
	if err := run(t.Context(), []string{"-db", database, "-replay", "missing"}, io.Discard, io.Discard); err == nil {
		t.Fatal("replay unexpectedly succeeded")
	}
	if _, err := os.Stat(database); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only replay unexpectedly created the database: %v", err)
	}
}
