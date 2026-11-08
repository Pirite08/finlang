package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServeRecordTransferQuotes(t *testing.T) {
	app, err := loadServeApp("../../examples/transfer_quotes_record_app.json")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(newRuleHandler(app))
	defer server.Close()

	page, err := server.Client().Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	html, err := io.ReadAll(page.Body)
	page.Body.Close()
	if err != nil || page.StatusCode != http.StatusOK || !strings.HasPrefix(page.Header.Get("Content-Type"), "text/html") ||
		!bytes.Contains(html, []byte("/transfer-quotes")) {
		t.Fatalf("preview status=%d type=%q err=%v", page.StatusCode, page.Header.Get("Content-Type"), err)
	}

	const valid = `{"schema_version":"3","rule":"transfer_quote_krw_v1","inputs":{"transfer":{"request_id":"quote-001","from_account":"A","to_account":"B","amount":{"currency":"KRW","amount":"10000"}}}}`
	const invalid = `"code":"E_CONTRACT_INPUT"`
	for _, tc := range []struct {
		name, body, want string
		code             int
	}{
		{"quote", valid, `"result":{"fee":{"currency":"KRW","amount":"500"},"total_debit":{"currency":"KRW","amount":"10500"}}`, http.StatusOK},
		{"at limit", strings.Replace(valid, "10000", "8784163844623596007", 1), `"total_debit":{"currency":"KRW","amount":"9223372036854775807"}`, http.StatusOK},
		{"missing account", strings.Replace(valid, `"to_account":"B",`, "", 1), invalid, http.StatusBadRequest},
		{"missing ID", strings.Replace(valid, `"request_id":"quote-001",`, "", 1), invalid, http.StatusBadRequest},
		{"wrong currency", strings.Replace(valid, "KRW", "USD", 1), invalid, http.StatusBadRequest},
		{"above limit", strings.Replace(valid, "10000", "8784163844623596008", 1), invalid, http.StatusBadRequest},
		{"below limit", strings.Replace(valid, "10000", "0", 1), invalid, http.StatusBadRequest},
		{"duplicate nested field", strings.Replace(valid, `"to_account":"B"`, `"to_account":"B","to_account":"C"`, 1), invalid, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response, err := server.Client().Post(server.URL+"/transfer-quotes", "application/json", strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil || response.StatusCode != tc.code || !bytes.Contains(body, []byte(tc.want)) ||
				!bytes.Contains(body, []byte(`"schema_version":"3"`)) {
				t.Fatalf("status=%d body=%s err=%v, want %d containing %s", response.StatusCode, body, err, tc.code, tc.want)
			}
		})
	}
}
