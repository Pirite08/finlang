package fxstream

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/fx"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const subscriptionOK = `{"event":"subscribe-status","status":"ok","success":[{"symbol":"USD/KRW","exchange":"FOREX"}],"fails":[]}`

func TestStreamSubscribesAndPublishesExactQuote(t *testing.T) {
	heartbeat := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-TD-APIKEY") != "test-key" || r.URL.RawQuery != "" {
			t.Error("expected API key only in the authentication header")
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.CloseNow()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		var subscribe struct {
			Action string `json:"action"`
			Params struct {
				Symbols string `json:"symbols"`
			} `json:"params"`
		}
		if err := wsjson.Read(ctx, conn, &subscribe); err != nil || subscribe.Action != "subscribe" || subscribe.Params.Symbols != "USD/KRW" {
			t.Errorf("unexpected subscription %#v (%v)", subscribe, err)
			return
		}
		for _, message := range []string{
			`{"event":"price","symbol":"USD/KRW","timestamp":1700000000,"price":999}`,
			subscriptionOK,
			`{"event":"price","symbol":"KRW/USD","exchange":"FOREX","timestamp":1700000000,"price":0.0007}`,
			`{"event":"price","symbol":"USD/KRW","exchange":"FOREX","timestamp":1700000000,"price":1350.25000000000000001}`,
		} {
			if err := conn.Write(ctx, websocket.MessageText, []byte(message)); err != nil {
				t.Error(err)
				return
			}
		}
		var message map[string]string
		if err := wsjson.Read(ctx, conn, &message); err != nil || message["action"] != "heartbeat" {
			t.Errorf("expected heartbeat, got %v (%v)", message, err)
			return
		}
		heartbeat <- struct{}{}
	}))
	defer server.Close()

	stream := testStream(t, server.URL)
	stream.heartbeatEvery = 10 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	quotes := make(chan fx.Quote, 4)
	done := make(chan struct{})
	go func() {
		defer close(done)
		stream.Run(ctx, func(q fx.Quote) { quotes <- q }, nil)
	}()
	select {
	case q := <-quotes:
		if q.Base != "USD" || q.Target != "KRW" || q.Rate != "1350.25000000000000001" || q.Source != "twelve-data" || q.AsOf.Unix() != 1700000000 {
			t.Fatalf("wrong normalized quote: %+v", q)
		}
	case <-ctx.Done():
		t.Fatal("stream produced no quote")
	}
	select {
	case <-heartbeat:
	case <-ctx.Done():
		t.Fatal("stream did not send heartbeat")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream did not exit after cancellation")
	}
	if len(quotes) != 0 {
		t.Fatal("stream published an unsolicited quote")
	}
}

func TestStreamRetriesDisconnectAndSanitizesRejection(t *testing.T) {
	var connections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := connections.Add(1)
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.CloseNow()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, _, err := conn.Read(ctx); err != nil {
			t.Error(err)
			return
		}
		switch attempt {
		case 1:
			if err := conn.Write(ctx, websocket.MessageText, []byte(`{"event":"subscribe-status","status":"error","message":"test-key rejected"}`)); err != nil {
				t.Error(err)
			}
		case 2:
			// Close before acknowledgement to exercise retrying a lost connection.
		default:
			for _, message := range []string{subscriptionOK, `{"event":"price","symbol":"USD/KRW","timestamp":1700000001,"price":1351.25}`} {
				if err := conn.Write(ctx, websocket.MessageText, []byte(message)); err != nil {
					t.Error(err)
					return
				}
			}
			_, _, _ = conn.Read(ctx)
		}
	}))
	defer server.Close()

	stream := testStream(t, server.URL)
	stream.minBackoff = time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var reports []error
	var quote fx.Quote
	stream.Run(ctx, func(q fx.Quote) {
		quote = q
		cancel()
	}, func(err error) { reports = append(reports, err) })
	if connections.Load() != 3 || quote.Rate != "1351.25" {
		t.Fatalf("wanted subscription on third connection, got %d connections and %+v", connections.Load(), quote)
	}
	if len(reports) != 2 || diagnostic.CodeOf(reports[0]) != "E_FX_STREAM_SUBSCRIBE" {
		t.Fatalf("expected sanitized rejection and disconnect, got %v", reports)
	}
	for _, err := range reports {
		if strings.Contains(err.Error(), "test-key") {
			t.Fatal("stream report included authentication information")
		}
	}
}

func TestStreamDeniesRedirect(t *testing.T) {
	var redirected atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected.Store(true)
	}))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()

	stream := testStream(t, server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var report error
	stream.Run(ctx, nil, func(err error) { report = err; cancel() })
	if redirected.Load() || diagnostic.CodeOf(report) != "E_FX_STREAM_CONNECT" {
		t.Fatalf("expected blocked redirect and connection diagnostic, got %v", report)
	}
}

func TestPriceDecodingRejectsMalformedValues(t *testing.T) {
	for _, tc := range []struct{ price, want string }{
		{"1350.25000000000000001", "1350.25000000000000001"},
		{"1.35025e3", "1350.25"},
		{"0", ""},
		{"-10", ""},
		{`"1350.25"`, ""},
		{"1e99999", ""},
		{"null", ""},
	} {
		t.Run(tc.price, func(t *testing.T) {
			msg := message{Symbol: "USD/KRW", Price: json.RawMessage(tc.price), Timestamp: 1700000000}
			q, ok := msg.quote()
			if ok != (tc.want != "") || q.Rate != tc.want {
				t.Fatalf("got %q, ok=%v, want %q", q.Rate, ok, tc.want)
			}
		})
	}
}

func testStream(t *testing.T, endpoint string) *Stream {
	t.Helper()
	stream, err := New("test-key")
	if err != nil {
		t.Fatal(err)
	}
	stream.endpoint = "ws://" + strings.TrimPrefix(endpoint, "http://")
	return stream
}
