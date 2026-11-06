package fxstream

import "testing"

func TestDecodeMessageRejectsAmbiguousFields(t *testing.T) {
	for name, body := range map[string]string{
		"duplicate price":      `{"event":"price","symbol":"USD/KRW","timestamp":1700000000,"price":1,"price":1350}`,
		"escaped duplicate":    `{"price":1350,"\u0070rice":1}`,
		"price case collision": `{"price":1350,"Price":1}`,
		"case variant alone":   `{"Price":1350}`,
		"duplicate timestamp":  `{"timestamp":1700000000,"timestamp":1700000001}`,
		"folded symbol":        `{"\u017fymbol":"USD/KRW"}`,
		"duplicate event":      `{"event":"error","event":"price"}`,
		"duplicate status":     `{"status":"error","status":"ok"}`,
		"duplicate success":    `{"success":[],"success":[{"symbol":"USD/KRW"}]}`,
		"duplicate instrument": `{"success":[{"symbol":"EUR/USD","symbol":"USD/KRW"}]}`,
		"instrument exchange":  `{"success":[{"symbol":"USD/KRW","Exchange":"FOREX"}]}`,
		"duplicate extension":  `{"type":"Equity","type":"Forex"}`,
		"second document":      `{"event":"price"} {"event":"error"}`,
		"null message":         `null`,
	} {
		t.Run(name, func(t *testing.T) {
			if msg, ok := decodeMessage([]byte(body)); ok {
				t.Fatalf("accepted malformed message: %+v", msg)
			}
		})
	}
}

func TestDecodeMessagePreservesVendorExtensionsAndExactPrice(t *testing.T) {
	status, ok := decodeMessage([]byte(`{
		"event":"subscribe-status","status":"ok",
		"success":[{"symbol":"USD/KRW","exchange":"FOREX","type":"Physical Currency","country":""}],
		"fails":[]
	}`))
	if !ok || !status.subscribed() {
		t.Fatalf("expected subscription confirmation, got %+v", status)
	}
	msg, ok := decodeMessage([]byte(`{
		"event":"price","symbol":"USD/KRW","exchange":"FOREX","timestamp":1700000000,
		"\u0070rice":1.35025000000000000001e3,"type":"Physical Currency","meta":{"price":7},"day_volume":112
	}`))
	if !ok {
		t.Fatal("normal price message rejected")
	}
	quote, ok := msg.quote()
	if !ok || quote.Rate != "1350.25000000000000001" {
		t.Fatalf("expected exact decimal price, got %+v", quote)
	}
}
