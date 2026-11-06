package fx_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/fx"
)

const reverseQuote = `{"base":"KRW","target":"USD","rate":"0.000740000000000001","source":"fixture","as_of":"2024-06-01T00:00:00.000000001+09:00"}`

func TestLoadSnapshotKeepsBothExactDirections(t *testing.T) {
	snapshot, err := fx.LoadSnapshot(strings.NewReader("[" + fixedQuote + "," + reverseQuote + "]"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct{ base, target, rate, asOf string }{
		{"USD", "KRW", "1350.125000000000000001", "2024-06-01T00:00:00Z"},
		{"KRW", "USD", "0.000740000000000001", "2024-06-01T00:00:00.000000001+09:00"},
	} {
		got, err := snapshot.Latest(want.base, want.target)
		if err != nil || got.Rate != want.rate || got.Source != "fixture" || got.AsOf.Format(time.RFC3339Nano) != want.asOf {
			t.Fatalf("%s/%s: got %+v, %v, want %+v", want.base, want.target, got, err, want)
		}
	}
	empty, err := fx.LoadSnapshot(strings.NewReader("[]"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := empty.Latest("USD", "KRW"); diagnostic.CodeOf(err) != "E_FX_PAIR" {
		t.Fatalf("empty snapshot resolved a pair: %v", err)
	}
}

func TestLoadSnapshotRejectsInvalidInputs(t *testing.T) {
	for name, body := range map[string]string{
		"single quote object": fixedQuote,
		"null":                "null",
		"null entry":          "[null]",
		"duplicate pair":      "[" + fixedQuote + "," + fixedQuote + "]",
		"duplicate key":       "[" + strings.Replace(fixedQuote, `"source":"fixture",`, `"source":"fixture","sourc\u0065":"other",`, 1) + "]",
		"numeric rate":        "[" + strings.Replace(fixedQuote, `"1350.125000000000000001"`, `1350.125000000000000001`, 1) + "]",
		"invalid timestamp":   "[" + strings.Replace(reverseQuote, "+09:00", "+09:60", 1) + "]",
		"trailing value":      "[" + fixedQuote + "] []",
		"too many quotes":     "[" + fixedQuote + "," + reverseQuote + "," + fixedQuote + "]",
		"oversized":           "[]" + strings.Repeat(" ", 9000),
	} {
		t.Run(name, func(t *testing.T) {
			if snapshot, err := fx.LoadSnapshot(strings.NewReader(body)); snapshot != nil || diagnostic.CodeOf(err) != "E_FX_QUOTE" {
				t.Fatalf("accepted invalid snapshot: %v, %v", snapshot, err)
			}
		})
	}
}

func TestSnapshotReplaysExactHistoricalQuotesInBothDirections(t *testing.T) {
	old := time.Date(1995, time.January, 2, 3, 4, 5, 0, time.UTC)
	quotes := []fx.Quote{
		{Base: "USD", Target: "KRW", Rate: "1350.250000000000000001", Source: "original", AsOf: old},
		{Base: "KRW", Target: "USD", Rate: "0.000740000000000001", Source: "original", AsOf: old},
	}
	provider, err := fx.NewSnapshot(quotes)
	if err != nil {
		t.Fatal(err)
	}
	wantForward := quotes[0]
	quotes[0].Rate = "1"
	for _, want := range []fx.Quote{wantForward, quotes[1]} {
		if got, err := provider.Latest(want.Base, want.Target); err != nil || got != want {
			t.Fatalf("replay = %+v, %v; want %+v", got, err, want)
		}
	}
	if _, err := provider.Latest("USD", "EUR"); diagnostic.CodeOf(err) != "E_FX_PAIR" {
		t.Fatalf("missing direction: %v", err)
	}
}

func TestSnapshotRejectsAmbiguousOrInvalidQuotes(t *testing.T) {
	quote := fx.Quote{Base: "USD", Target: "KRW", Rate: "1350", Source: "original", AsOf: time.Now()}
	bad := quote
	bad.Rate = "0"
	for _, quotes := range [][]fx.Quote{{quote, quote}, {quote, quote, quote}, {bad}} {
		if provider, err := fx.NewSnapshot(quotes); provider != nil || diagnostic.CodeOf(err) != "E_FX_QUOTE" {
			t.Fatalf("NewSnapshot(%v) = %v, %v", quotes, provider, err)
		}
	}
}
