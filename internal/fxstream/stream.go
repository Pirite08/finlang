// Package fxstream receives USD/KRW price updates from Twelve Data.
package fxstream

import (
	"context"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/fx"
	"github.com/coder/websocket"
)

const (
	endpoint          = "wss://ws.twelvedata.com/v1/quotes/price"
	symbol            = "USD/KRW"
	connectTimeout    = 10 * time.Second
	subscribeTimeout  = 10 * time.Second
	writeTimeout      = 5 * time.Second
	maxMessageBytes   = 16 * 1024
	maxReconnectDelay = 30 * time.Second
)

type Stream struct {
	endpoint       string
	apiKey         string
	heartbeatEvery time.Duration
	idleTimeout    time.Duration
	minBackoff     time.Duration
}

func New(apiKey string) (*Stream, error) {
	if apiKey == "" || len(apiKey) > 256 || !utf8.ValidString(apiKey) || strings.ContainsFunc(apiKey, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) {
		return nil, diagnostic.New("E_FX_STREAM_CONFIG", "a valid Twelve Data API key is required")
	}
	return &Stream{
		endpoint: endpoint, apiKey: apiKey,
		heartbeatEvery: 10 * time.Second, idleTimeout: 90 * time.Second,
		minBackoff: time.Second,
	}, nil
}

// Run reads prices until ctx is canceled. It calls publish synchronously for
// each valid update and report for each lost or rejected connection.
func (s *Stream) Run(ctx context.Context, publish func(fx.Quote), report func(error)) {
	delay := s.minBackoff
	for ctx.Err() == nil {
		received, err := s.runConnection(ctx, publish)
		if ctx.Err() != nil {
			return
		}
		if report != nil {
			report(err)
		}
		if received {
			delay = s.minBackoff
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			delay = min(delay*2, maxReconnectDelay)
		}
	}
}

func (s *Stream) runConnection(ctx context.Context, publish func(fx.Quote)) (bool, error) {
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	header := make(http.Header)
	header.Set("X-TD-APIKEY", s.apiKey)
	dialCtx, dialCancel := context.WithTimeout(ctx, connectTimeout)
	conn, response, err := websocket.Dial(dialCtx, s.endpoint, &websocket.DialOptions{HTTPClient: client, HTTPHeader: header})
	dialCancel()
	if err != nil {
		if response != nil && (response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden) {
			return false, diagnostic.New("E_FX_STREAM_AUTH", "Twelve Data rejected stream authentication")
		}
		return false, diagnostic.New("E_FX_STREAM_CONNECT", "could not connect to the Twelve Data stream")
	}
	defer conn.CloseNow()
	conn.SetReadLimit(maxMessageBytes)
	if err := write(ctx, conn, `{"action":"subscribe","params":{"symbols":"USD/KRW"}}`); err != nil {
		return false, diagnostic.New("E_FX_STREAM_SUBSCRIBE", "could not request the USD/KRW subscription")
	}

	session, cancel := context.WithCancel(ctx)
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		s.heartbeat(session, conn, cancel)
	}()
	defer func() {
		cancel()
		<-heartbeatDone
	}()

	acknowledged, received := false, false
	ackDeadline := time.Now().Add(subscribeTimeout)
	for {
		deadline := ackDeadline
		if acknowledged {
			deadline = time.Now().Add(s.idleTimeout)
		}
		readCtx, readCancel := context.WithDeadline(session, deadline)
		messageType, body, err := conn.Read(readCtx)
		readCancel()
		if err != nil {
			if !acknowledged && session.Err() == nil && !time.Now().Before(ackDeadline) {
				return false, diagnostic.New("E_FX_STREAM_SUBSCRIBE", "Twelve Data did not confirm the USD/KRW subscription")
			}
			return received, diagnostic.New("E_FX_STREAM_READ", "Twelve Data stream disconnected or timed out")
		}
		if messageType != websocket.MessageText {
			continue
		}
		msg, ok := decodeMessage(body)
		if !ok {
			continue
		}
		if msg.Event == "subscribe-status" {
			if !msg.subscribed() {
				return received, diagnostic.New("E_FX_STREAM_SUBSCRIBE", "Twelve Data rejected the USD/KRW subscription; check key and symbol access")
			}
			acknowledged = true
			continue
		}
		if msg.Event == "error" || msg.Status == "error" {
			return received, diagnostic.New("E_FX_STREAM_REJECTED", "Twelve Data reported a stream error; check key and symbol access")
		}
		if !acknowledged || msg.Event != "price" {
			continue
		}
		if quote, ok := msg.quote(); ok {
			received = true
			if publish != nil {
				publish(quote)
			}
		}
	}
}

func (s *Stream) heartbeat(ctx context.Context, conn *websocket.Conn, cancel context.CancelFunc) {
	ticker := time.NewTicker(s.heartbeatEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := write(ctx, conn, `{"action":"heartbeat"}`); err != nil {
				cancel()
				return
			}
		}
	}
}

func write(ctx context.Context, conn *websocket.Conn, payload string) error {
	writeCtx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return conn.Write(writeCtx, websocket.MessageText, []byte(payload))
}
