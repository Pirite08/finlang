package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/contract"
	"github.com/Pirite08/finlang/internal/parser"
)

const quoteJSONPrefix = `{"schema_version":"1","rule":"transfer_fee_krw_v1","inputs":{"amount":`

func quoteHandler(t *testing.T) http.Handler {
	t.Helper()
	const ruleFile = "../../internal/transferfee/rules/krw.fin"
	const contractFile = "../../internal/transferfee/rules/krw.json"
	source, err := os.ReadFile(ruleFile)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := os.ReadFile(contractFile)
	if err != nil {
		t.Fatal(err)
	}
	limits := config.DefaultLimits()
	program, err := parser.Parse(ruleFile, string(source), limits)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := contract.Load(bytes.NewReader(definition), limits)
	if err != nil {
		t.Fatal(err)
	}
	return newRuleHandler(servedApp{routes: map[string]servedRule{"/transfer-quotes": {
		rule: program.Rule, spec: spec, limits: limits, inputOrder: []int{0},
	}}})
}

func TestServedRuleHTTPQuote(t *testing.T) {
	handler := quoteHandler(t)
	for _, test := range []struct{ amount, fee string }{
		{"10000", "500"}, {"20000", "1000"}, {"10", "0"}, {"30", "2"},
	} {
		t.Run(test.amount, func(t *testing.T) {
			t.Parallel()
			body := quoteJSONPrefix + `{"currency":"KRW","amount":"` + test.amount + `"}}}`
			req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/transfer-quotes", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json; charset=utf-8")
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			want := `{"schema_version":"1","rule":"transfer_fee_krw_v1","status":"ok","result":{"currency":"KRW","amount":"` + test.fee + `"}}`
			if res.Code != http.StatusOK || strings.TrimSpace(res.Body.String()) != want {
				t.Fatalf("status=%d body=%s; want 200 %s", res.Code, res.Body.String(), want)
			}
			if res.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("unexpected response headers: %v", res.Header())
			}
		})
	}
}

func TestServedRuleHTTPRejectsContractInput(t *testing.T) {
	handler := quoteHandler(t)
	for _, test := range []struct{ name, body string }{
		{"wrong currency", quoteJSONPrefix + `{"currency":"USD","amount":"10000"}}}`},
		{"fractional won", quoteJSONPrefix + `{"currency":"KRW","amount":"10000.5"}}}`},
		{"nonpositive amount", quoteJSONPrefix + `{"currency":"KRW","amount":"0"}}}`},
		{"out of range", quoteJSONPrefix + `{"currency":"KRW","amount":"9223372036854775808"}}}`},
		{"numeric amount", quoteJSONPrefix + `{"currency":"KRW","amount":10000}}}`},
		{"duplicate amount", quoteJSONPrefix + `{"currency":"KRW","amount":"10000","amount":"20000"}}}`},
		{"missing amount", quoteJSONPrefix + `{"currency":"KRW"}}}`},
		{"rule override", `{"schema_version":"1","rule":"unsafe_rule","inputs":{}}`},
		{"source injection", quoteJSONPrefix + `{"currency":"KRW","amount":"10000"}},"source":"print(99)"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "http://localhost/transfer-quotes", strings.NewReader(test.body))
			req.Header.Set("Content-Type", "application/json")
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			if res.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s; want 400", res.Code, res.Body.String())
			}
			var got struct {
				Status string
				Error  struct {
					Code string
					File *string
				}
			}
			if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil || got.Status != "error" || got.Error.Code != "E_CONTRACT_INPUT" || got.Error.File != nil {
				t.Fatalf("body=%s error=%v; want sanitized E_CONTRACT_INPUT", res.Body.String(), err)
			}
			if strings.Contains(res.Body.String(), "unsafe_rule") || strings.Contains(res.Body.String(), "print(99)") {
				t.Fatalf("response echoes request content: %s", res.Body.String())
			}
		})
	}
}

func TestServedRuleHTTPBoundary(t *testing.T) {
	handler := quoteHandler(t)
	for _, test := range []struct {
		name, method, host, path, contentType, body string
		status                                      int
	}{
		{"nonlocal host", "POST", "example.test", "/transfer-quotes", "application/json", "{}", 403},
		{"unknown path", "POST", "localhost", "/unconfigured", "application/json", "{}", 404},
		{"encoded path", "POST", "localhost", "/transfer%2Dquotes", "application/json", "{}", 404},
		{"wrong method", "GET", "localhost", "/transfer-quotes", "application/json", "{}", 405},
		{"wrong media type", "POST", "localhost", "/transfer-quotes", "text/plain", "{}", 415},
		{"oversized body", "POST", "localhost", "/transfer-quotes", "application/json", strings.Repeat(" ", config.DefaultLimits().MaxSourceBytes+1), 413},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(test.method, "http://"+test.host+test.path, strings.NewReader(test.body))
			req.Header.Set("Content-Type", test.contentType)
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			if res.Code != test.status {
				t.Fatalf("status=%d body=%s; want %d", res.Code, res.Body.String(), test.status)
			}
			if test.status == http.StatusMethodNotAllowed && res.Header().Get("Allow") != "POST" {
				t.Fatalf("unexpected Allow header: %v", res.Header())
			}
		})
	}
}
