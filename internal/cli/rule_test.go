package cli

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func fxRuleArgs() []string {
	return []string{"-file", "../../examples/usd_to_krw.fin", "-contract", "../../contracts/usd_to_krw.json", "-input", "../../testdata/inputs/usd_to_krw.json", "-json"}
}

func runFXRule(t *testing.T, args []string, wantAmount string) []map[string]any {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fx.trace.jsonl")
	args = append(args, "-trace", path)
	var out, stderr bytes.Buffer
	if code := Run(args, strings.NewReader(""), &out, &stderr); code != 0 || stderr.Len() != 0 {
		t.Fatalf("exit=%d, stderr=%s", code, &stderr)
	}
	got := decodeRecord(t, out.String())
	want := map[string]any{"currency": "KRW", "amount": wantAmount}
	if got["status"] != "ok" || !reflect.DeepEqual(got["result"], want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		records = append(records, decodeRecord(t, line))
	}
	return records
}

func TestRuleFixedFXSteps(t *testing.T) {
	records := runFXRule(t, append(fxRuleArgs(), "-fx-fixed", "../../testdata/fx/usd_to_krw.json"), "13502")
	var steps []string
	bindings := map[string]any{}
	for _, record := range records {
		if record["kind"] == "step_end" && record["status"] == "ok" {
			steps = append(steps, record["step"].(string))
		}
		if record["kind"] == "binding" {
			bindings[record["binding"].(string)] = record["result"]
		}
	}
	if want := []string{"check_amount", "get_rate", "convert_amount", "settle_amount"}; !reflect.DeepEqual(steps, want) {
		t.Fatalf("finished steps = %v, want %v", steps, want)
	}
	if bindings["converted"] != "KRW 13502.5" || bindings["result"] != "KRW 13502" {
		t.Fatalf("wrong intermediate amounts: %v", bindings)
	}
	want := []any{map[string]any{"base": "USD", "target": "KRW", "rate": "1350.25", "source": "fixed-test-quote", "as_of": "2026-01-01T00:00:00Z"}}
	if !reflect.DeepEqual(records[0]["fx_quotes"], want) {
		t.Fatalf("trace did not retain the exact quote: %v", records[0])
	}
}

func TestRuleHTTPRefreshesAcrossExecutions(t *testing.T) {
	// A second rate() in the same rule catches a missing execution snapshot.
	data, err := os.ReadFile("../../examples/usd_to_krw.fin")
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Replace(string(data), "let converted = convert(amount, quote)",
		"let converted = convert(amount, quote)\n        require convert(amount, rate(\"USD\", \"KRW\")) == converted", 1)
	file := filepath.Join(t.TempDir(), "refresh.fin")
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	requests := make(chan string, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Query().Get("base") != "USD" || r.URL.Query().Get("target") != "KRW" {
			t.Errorf("unexpected quote request: %s %s", r.Method, r.URL)
		}
		rate := "1350.25"
		if len(requests) > 0 {
			rate = "1350.35"
		}
		requests <- rate
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"base":"USD","target":"KRW","rate":%q,"source":"http-test","as_of":%q}`, rate, time.Now().UTC().Format(time.RFC3339Nano))
	}))
	defer server.Close()

	for _, tc := range []struct{ rate, amount string }{{"1350.25", "13502"}, {"1350.35", "13504"}} {
		args := append(fxRuleArgs(), "-fx-url", server.URL)
		args[1] = file
		records := runFXRule(t, args, tc.amount)
		quotes, ok := records[0]["fx_quotes"].([]any)
		if !ok || len(quotes) != 1 {
			t.Fatalf("want one quote per execution: %v", records[0])
		}
		quote := quotes[0].(map[string]any)
		if quote["rate"] != tc.rate || quote["source"] != "http-test" || quote["as_of"] == nil {
			t.Fatalf("wrong HTTP quote in trace: %v", quote)
		}
	}
	if got := len(requests); got != 2 {
		t.Fatalf("provider received %d requests, want exactly one per execution", got)
	}
}

func TestRuleFailedRequirementStopsBeforeQuote(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("failed amount check must stop before fetching a quote")
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()
	input := filepath.Join(t.TempDir(), "zero.json")
	if err := os.WriteFile(input, []byte(`{"schema_version":"1","rule":"usd_to_krw","inputs":{"amount":{"currency":"USD","amount":"0"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	traceFile := filepath.Join(t.TempDir(), "failure.jsonl")
	args := append(fxRuleArgs(), "-fx-url", server.URL, "-trace", traceFile)
	args[5] = input
	var out, stderr bytes.Buffer
	if code := Run(args, strings.NewReader(""), &out, &stderr); code != 1 {
		t.Fatalf("exit=%d, stderr=%s", code, &stderr)
	}
	record := decodeRecord(t, out.String())
	got, ok := record["error"].(map[string]any)
	if !ok || got["code"] != "E_REQUIRE_FAILED" || got["line"] != float64(6) {
		t.Fatalf("expected a positioned requirement failure, got %v", record)
	}
	data, err := os.ReadFile(traceFile)
	if err != nil {
		t.Fatal(err)
	}
	var end map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		event := decodeRecord(t, line)
		if event["kind"] == "step_end" {
			if end != nil {
				t.Fatal("a later step ran after the failure")
			}
			end = event
		}
	}
	if end["step"] != "check_amount" || end["status"] != "error" || end["error_code"] != "E_REQUIRE_FAILED" {
		t.Fatalf("expected failed step in trace, got %v", end)
	}
}

func TestRuleTraceProtectsFixedQuote(t *testing.T) {
	quote := filepath.Join(t.TempDir(), "quote.json")
	fixture, err := os.ReadFile("../../testdata/fx/usd_to_krw.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(quote, fixture, 0600); err != nil {
		t.Fatal(err)
	}
	args := append(fxRuleArgs(), "-fx-fixed", quote, "-trace", quote)
	var out, stderr bytes.Buffer
	if code := Run(args, strings.NewReader(""), &out, &stderr); code != 2 {
		t.Fatalf("exit=%d, stderr=%s", code, &stderr)
	}
	record := decodeRecord(t, out.String())
	if got, ok := record["error"].(map[string]any); !ok || got["code"] != "E_TRACE_IO" {
		t.Fatalf("want trace destination error, got %v", record)
	}
	data, err := os.ReadFile(quote)
	if err != nil || !bytes.Equal(fixture, data) {
		t.Fatalf("fixed quote was changed: %v", err)
	}
}
