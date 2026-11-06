package fxstream

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Pirite08/finlang/internal/cli"
	"github.com/Pirite08/finlang/internal/fx"
	"github.com/Pirite08/finlang/internal/fxfeed"
	"github.com/coder/websocket"
)

func TestStreamUpdatesNextFinlangRun(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	t.Cleanup(cancel)
	secondTick := make(chan struct{})
	asOf := time.Now().UTC().Truncate(time.Second)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.CloseNow()
		if _, _, err := conn.Read(ctx); err != nil {
			t.Error(err)
			return
		}
		ack := `{"event":"subscribe-status","status":"ok","success":[{"symbol":"USD/KRW","exchange":"FOREX"}],"fails":[]}`
		if err := conn.Write(ctx, websocket.MessageText, []byte(ack)); err != nil {
			t.Error(err)
			return
		}
		for i, rate := range []string{"1350.25", "1350.35"} {
			if i == 1 {
				select {
				case <-secondTick:
				case <-ctx.Done():
					return
				}
				for _, priceFields := range []string{`"price":1350.25,"price":1450`, `"price":1`} {
					message := fmt.Sprintf(`{"event":"price","symbol":"USD/KRW","exchange":"FOREX","timestamp":%d,%s}`, asOf.Unix(), priceFields)
					if err := conn.Write(ctx, websocket.MessageText, []byte(message)); err != nil {
						t.Error(err)
						return
					}
				}
			}
			message := fmt.Sprintf(`{"event":"price","symbol":"USD/KRW","exchange":"FOREX","timestamp":%d,"price":%s}`, asOf.Unix(), rate)
			if err := conn.Write(ctx, websocket.MessageText, []byte(message)); err != nil {
				t.Error(err)
				return
			}
		}
		<-ctx.Done()
	}))
	t.Cleanup(upstream.Close)

	receiver := fxfeed.New(fx.LivePolicy{MaxAge: time.Minute})
	stream, err := New("test-key")
	if err != nil {
		t.Fatal(err)
	}
	stream.endpoint = "ws://" + strings.TrimPrefix(upstream.URL, "http://")
	published := make(chan fx.Quote, 2)
	var rejected atomic.Int32
	streamDone := make(chan struct{})
	go func() {
		defer close(streamDone)
		stream.Run(ctx, func(quote fx.Quote) {
			if !receiver.Publish(quote) {
				if quote.Rate != "1" {
					t.Errorf("receiver rejected a valid tick: %s", quote.Rate)
				}
				rejected.Add(1)
				return
			}
			select {
			case published <- quote:
			case <-ctx.Done():
			}
		}, func(err error) { t.Error(err) })
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-streamDone:
		case <-time.After(time.Second):
			t.Error("stream did not stop after cancellation")
		}
	})
	select {
	case first := <-published:
		if first.Rate != "1350.25" {
			t.Fatalf("first tick = %v", first)
		}
	case <-ctx.Done():
		t.Fatal("first tick was not published")
	}

	var requests atomic.Int32
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) != 1 {
			receiver.ServeHTTP(w, r)
			return
		}
		// Capture the first HTTP response, then deliver the newer upstream tick
		// while this finlang invocation is still in its first rate() call.
		first := httptest.NewRecorder()
		receiver.ServeHTTP(first, r)
		close(secondTick)
		select {
		case second := <-published:
			if second.Rate != "1350.35" {
				t.Errorf("second tick = %v", second)
			}
		case <-ctx.Done():
			t.Error("second tick was not published")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		for key, values := range first.Header() {
			w.Header()[key] = values
		}
		w.WriteHeader(first.Code)
		_, _ = w.Write(first.Body.Bytes())
	}))
	t.Cleanup(func() {
		cancel()
		local.Close()
	})

	example, err := os.ReadFile("../../examples/usd_to_krw.fin")
	if err != nil {
		t.Fatal(err)
	}
	const conversion = "let converted = convert(amount, quote)"
	if !strings.Contains(string(example), conversion) {
		t.Fatal("example no longer has the conversion line used by this test")
	}
	source := strings.Replace(string(example), conversion,
		conversion+"\n        require convert(amount, rate(\"USD\", \"KRW\")) == converted", 1)
	file := filepath.Join(t.TempDir(), "snapshot.fin")
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	for i, tc := range []struct{ rate, amount string }{{"1350.25", "13502"}, {"1350.35", "13504"}} {
		tracePath := filepath.Join(t.TempDir(), "stream.trace.jsonl")
		args := []string{"-file", file, "-contract", "../../contracts/usd_to_krw.json", "-input", "../../testdata/inputs/usd_to_krw.json",
			"-json", "-fx-url", local.URL + "/latest", "-trace", tracePath}
		var out, stderr bytes.Buffer
		if code := cli.Run(args, strings.NewReader(""), &out, &stderr); code != 0 || stderr.Len() != 0 {
			t.Fatalf("execution %d: exit=%d, stdout=%s, stderr=%s", i+1, code, &out, &stderr)
		}
		var result struct {
			Status string `json:"status"`
			Result struct {
				Amount   string `json:"amount"`
				Currency string `json:"currency"`
			} `json:"result"`
		}
		if err := json.Unmarshal(out.Bytes(), &result); err != nil || result.Status != "ok" || result.Result.Amount != tc.amount || result.Result.Currency != "KRW" {
			t.Fatalf("execution %d: got %s, want KRW %s", i+1, &out, tc.amount)
		}
		traceData, err := os.ReadFile(tracePath)
		if err != nil {
			t.Fatal(err)
		}
		var header struct {
			Quotes []fx.Quote `json:"fx_quotes"`
		}
		firstLine, _, _ := bytes.Cut(traceData, []byte{'\n'})
		if err := json.Unmarshal(firstLine, &header); err != nil || len(header.Quotes) != 1 {
			t.Fatalf("execution %d: expected one quote in trace, got %s", i+1, firstLine)
		}
		if q := header.Quotes[0]; q.Rate != tc.rate || q.Source != "twelve-data" || !q.AsOf.Equal(asOf) {
			t.Fatalf("execution %d: wrong trace quote %+v", i+1, q)
		}
		t.Logf("execution %d used %s and returned KRW %s", i+1, tc.rate, tc.amount)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("%d HTTP requests, want one per execution", got)
	}
	if got := rejected.Load(); got != 1 {
		t.Fatalf("%d ticks rejected by receiver, want one outlier after skipping the duplicate frame", got)
	}
}
