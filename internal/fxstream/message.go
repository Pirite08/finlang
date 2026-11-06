package fxstream

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/fx"
)

type instrument struct {
	Symbol   string
	Exchange string
}

func (i *instrument) UnmarshalJSON(body []byte) error {
	return decodeObject(body, map[string]any{
		"symbol":   &i.Symbol,
		"exchange": &i.Exchange,
	})
}

type message struct {
	Event     string
	Status    string
	Symbol    string
	Exchange  string
	Timestamp int64
	Price     json.RawMessage
	Success   []instrument
}

func decodeMessage(body []byte) (message, bool) {
	if !utf8.Valid(body) {
		return message{}, false
	}
	var msg message
	if err := decodeObject(body, map[string]any{
		"event":     &msg.Event,
		"status":    &msg.Status,
		"symbol":    &msg.Symbol,
		"exchange":  &msg.Exchange,
		"timestamp": &msg.Timestamp,
		"price":     &msg.Price,
		"success":   &msg.Success,
	}); err != nil {
		return message{}, false
	}
	return msg, true
}

var errMalformedObject = errors.New("malformed WebSocket message object")

// decodeObject allows provider extensions while requiring exact spellings for
// fields we consume and rejecting repeated keys after JSON escape decoding.
func decodeObject(body []byte, fields map[string]any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return errMalformedObject
	}
	seen := make(map[string]struct{}, len(fields))
	var ignored json.RawMessage
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok {
			return errMalformedObject
		}
		if _, duplicate := seen[key]; duplicate {
			return errMalformedObject
		}
		seen[key] = struct{}{}
		target, known := fields[key]
		if !known {
			for field := range fields {
				if strings.EqualFold(key, field) {
					return errMalformedObject
				}
			}
			target = &ignored
		}
		if err := decoder.Decode(target); err != nil {
			return err
		}
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return errMalformedObject
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errMalformedObject
	}
	return nil
}

func (m message) subscribed() bool {
	if m.Status != "ok" {
		return false
	}
	for _, inst := range m.Success {
		if usdKRW(inst.Symbol, inst.Exchange) {
			return true
		}
	}
	return false
}

func usdKRW(pair, exchange string) bool {
	return pair == symbol && (exchange == "" || exchange == "FOREX")
}

func (m message) quote() (fx.Quote, bool) {
	if !usdKRW(m.Symbol, m.Exchange) || m.Timestamp <= 0 {
		return fx.Quote{}, false
	}
	limits := config.DefaultLimits()
	rate := decimalPrice(m.Price, limits.MaxTokenBytes)
	if rate == "" {
		return fx.Quote{}, false
	}
	quote := fx.Quote{Base: "USD", Target: "KRW", Rate: rate, Source: "twelve-data", AsOf: time.Unix(m.Timestamp, 0).UTC()}
	if _, err := quote.Validate("USD", "KRW", limits); err != nil {
		return fx.Quote{}, false
	}
	return quote, true
}

// decimalPrice preserves the JSON number text. When the provider uses an
// exponent, move its decimal point without converting through float64.
func decimalPrice(price json.RawMessage, maxBytes int) string {
	if len(price) == 0 || len(price) > maxBytes || price[0] < '0' || price[0] > '9' {
		return ""
	}
	text := string(price)
	index := strings.IndexAny(text, "eE")
	if index < 0 {
		return text
	}
	exponent, err := strconv.ParseInt(text[index+1:], 10, 32)
	if err != nil || exponent > int64(maxBytes) || exponent < -int64(maxBytes) {
		return ""
	}
	digits := text[:index]
	point := strings.IndexByte(digits, '.')
	if point < 0 {
		point = len(digits)
	} else {
		digits = digits[:point] + digits[point+1:]
	}
	point += int(exponent)
	if point <= 0 {
		return "0." + strings.Repeat("0", -point) + digits
	}
	if point >= len(digits) {
		return digits + strings.Repeat("0", point-len(digits))
	}
	return digits[:point] + "." + digits[point:]
}
