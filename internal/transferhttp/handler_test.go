package transferhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/transfer"
)

type storeFunc func(context.Context, transfer.Request) (transfer.Result, error)

func (f storeFunc) Transfer(ctx context.Context, req transfer.Request) (transfer.Result, error) {
	return f(ctx, req)
}

const requestJSON = `{"request_id":"demo-1","from_account":"A","to_account":"B","currency":"KRW","amount_minor":10000}`

func TestTransferRoutesExactIntegerAndResult(t *testing.T) {
	store := storeFunc(func(ctx context.Context, req transfer.Request) (transfer.Result, error) {
		if req != (transfer.Request{ID: "demo-1", From: "A", To: "B", Currency: "KRW", Amount: 9007199254740993}) {
			t.Fatalf("request was changed during decoding: %+v", req)
		}
		return transfer.Result{ID: req.ID, From: req.From, To: req.To, Currency: req.Currency, Amount: req.Amount, Fee: 500, Debit: req.Amount + 500}, nil
	})
	response := call(t, New(store), http.MethodPost, "/transfers", "application/json; charset=utf-8", strings.Replace(requestJSON, "10000", "9007199254740993", 1))
	var fields map[string]json.RawMessage
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &fields) != nil || len(fields) != 7 {
		t.Fatalf("response was %d %s, want a successful transfer result", response.Code, response.Body.String())
	}
	for key, want := range map[string]string{
		"request_id": `"demo-1"`, "from_account": `"A"`, "to_account": `"B"`, "currency": `"KRW"`,
		"amount_minor": "9007199254740993", "fee_minor": "500", "debit_minor": "9007199254741493",
	} {
		if got := string(fields[key]); got != want {
			t.Fatalf("%s = %s, want %s", key, got, want)
		}
	}
	if response.Header().Get("Content-Type") != "application/json" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("response headers: %v", response.Header())
	}
}

func TestTransferRejectsInvalidHTTPRequests(t *testing.T) {
	h := New(storeFunc(func(context.Context, transfer.Request) (transfer.Result, error) {
		t.Fatal("invalid HTTP request reached the store")
		return transfer.Result{}, nil
	}))
	for _, tc := range []struct {
		name, method, path, content, body string
		status                            int
	}{
		{"method", http.MethodGet, "/transfers", "application/json", "", http.StatusMethodNotAllowed},
		{"unknown route", http.MethodPost, "/transfer", "application/json", requestJSON, http.StatusNotFound},
		{"media type", http.MethodPost, "/transfers", "text/plain", requestJSON, http.StatusUnsupportedMediaType},
		{"oversized", http.MethodPost, "/transfers", "application/json", strings.Repeat(" ", 20_000) + requestJSON, http.StatusRequestEntityTooLarge},
		{"unknown key", http.MethodPost, "/transfers", "application/json", strings.Replace(requestJSON, "to_account", "target", 1), http.StatusBadRequest},
		{"wrong case", http.MethodPost, "/transfers", "application/json", strings.Replace(requestJSON, "to_account", "TO_ACCOUNT", 1), http.StatusBadRequest},
		{"duplicate escaped key", http.MethodPost, "/transfers", "application/json", strings.TrimSuffix(requestJSON, "}") + `,"\u0061mount_minor":200}`, http.StatusBadRequest},
		{"trailing JSON", http.MethodPost, "/transfers", "application/json", requestJSON + "{}", http.StatusBadRequest},
		{"missing key", http.MethodPost, "/transfers", "application/json", `{"request_id":"demo-1"}`, http.StatusBadRequest},
		{"null key", http.MethodPost, "/transfers", "application/json", strings.Replace(requestJSON, `"B"`, "null", 1), http.StatusBadRequest},
		{"decimal amount", http.MethodPost, "/transfers", "application/json", strings.Replace(requestJSON, "10000", "10000.0", 1), http.StatusBadRequest},
		{"string amount", http.MethodPost, "/transfers", "application/json", strings.Replace(requestJSON, "10000", `"10000"`, 1), http.StatusBadRequest},
		{"out of range", http.MethodPost, "/transfers", "application/json", strings.Replace(requestJSON, "10000", "9223372036854775808", 1), http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := call(t, h, tc.method, tc.path, tc.content, tc.body)
			if response.Code != tc.status {
				t.Fatalf("response was %d %s, want %d", response.Code, response.Body.String(), tc.status)
			}
			if tc.status == http.StatusMethodNotAllowed && response.Header().Get("Allow") != "POST" {
				t.Fatalf("Allow header = %q", response.Header().Get("Allow"))
			}
		})
	}
}

func TestTransferMapsStoreErrorsWithoutLeakingDetails(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{transfer.ErrInvalid, http.StatusBadRequest, "invalid_request"},
		{transfer.ErrCurrency, http.StatusBadRequest, "currency_mismatch"},
		{transfer.ErrNotFound, http.StatusNotFound, "account_not_found"},
		{transfer.ErrConflict, http.StatusConflict, "request_conflict"},
		{transfer.ErrInsufficientFunds, http.StatusConflict, "insufficient_funds"},
		{errors.New("private DB path and account details"), http.StatusInternalServerError, "internal_error"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			store := storeFunc(func(context.Context, transfer.Request) (transfer.Result, error) {
				return transfer.Result{}, tc.err
			})
			response := call(t, New(store), http.MethodPost, "/transfers", "application/json", requestJSON)
			if body := response.Body.String(); response.Code != tc.status || !strings.Contains(body, `"code":"`+tc.code+`"`) || strings.Contains(body, "private DB path") {
				t.Fatalf("response was %d %s, want %d with %s", response.Code, body, tc.status, tc.code)
			}
		})
	}
}

func call(t *testing.T, h http.Handler, method, path, contentType, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	response := httptest.NewRecorder()
	h.ServeHTTP(response, req)
	return response
}
