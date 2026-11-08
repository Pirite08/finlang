package cli

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/transfer"
)

func plannedServer(t *testing.T, mutateRule func(string) string) (*httptest.Server, *sql.DB) {
	t.Helper()
	app, err := loadServeApp("../../examples/transfer_app.json")
	if err != nil {
		t.Fatal(err)
	}
	if mutateRule != nil {
		data, err := os.ReadFile("../../examples/transfer_plan_krw_v1.fin")
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "plan.fin")
		if err := os.WriteFile(path, []byte(mutateRule(string(data))), 0o600); err != nil {
			t.Fatal(err)
		}
		route, err := loadServedRule(path, "../../contracts/transfer_plan_krw_v1.json", config.DefaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		route.transfer = true
		app.routes["/transfers"] = route
	}
	path := filepath.Join(t.TempDir(), "ledger.sqlite")
	store, err := transfer.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.SeedDemo(t.Context()); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	app.ledger = store
	server := httptest.NewServer(newRuleHandler(app))
	t.Cleanup(server.Close)
	return server, db
}

type plannedResponse struct {
	status int
	body   string
	err    error
}

func postPlanned(server *httptest.Server, id, amount string) plannedResponse {
	body := fmt.Sprintf(`{"schema_version":"3","rule":"transfer_plan_krw_v1","inputs":{"transfer":{"request_id":%q,"from_account":"A","to_account":"B","amount":{"currency":"KRW","amount":%q}}}}`, id, amount)
	response, err := server.Client().Post(server.URL+"/transfers", "application/json", strings.NewReader(body))
	if err != nil {
		return plannedResponse{err: err}
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	return plannedResponse{status: response.StatusCode, body: string(data), err: err}
}

func (response plannedResponse) check(t *testing.T, status int, expected ...string) {
	t.Helper()
	if response.err != nil || response.status != status {
		t.Fatalf("status=%d body=%s err=%v, want %d", response.status, response.body, response.err, status)
	}
	for _, value := range expected {
		if !strings.Contains(response.body, value) {
			t.Fatalf("response %s omitted %s", response.body, value)
		}
	}
}

func assertPlannedDB(t *testing.T, db *sql.DB, wantBalances [3]int64, transfers, postings int) {
	t.Helper()
	for i, account := range []string{"A", "B", "fee-KRW"} {
		var balance int64
		if err := db.QueryRowContext(t.Context(), "SELECT balance FROM accounts WHERE id = ?", account).Scan(&balance); err != nil ||
			balance != wantBalances[i] {
			t.Fatalf("%s balance=%d err=%v, want %d", account, balance, err, wantBalances[i])
		}
	}
	var receipts, entries int
	if err := db.QueryRowContext(t.Context(), "SELECT (SELECT count(*) FROM transfers), (SELECT count(*) FROM postings)").
		Scan(&receipts, &entries); err != nil || receipts != transfers || entries != postings {
		t.Fatalf("transfers=%d postings=%d err=%v, want %d and %d", receipts, entries, err, transfers, postings)
	}
}

func TestServePlannedTransferAndDuplicate(t *testing.T) {
	server, db := plannedServer(t, nil)
	options, err := http.NewRequest(http.MethodOptions, server.URL+"/transfers", nil)
	if err != nil {
		t.Fatal(err)
	}
	probe, err := server.Client().Do(options)
	if err != nil {
		t.Fatal(err)
	}
	probe.Body.Close()
	if probe.StatusCode != http.StatusNoContent {
		t.Fatalf("console capability probe returned %d", probe.StatusCode)
	}
	assertPlannedDB(t, db, [3]int64{20000, 2000, 0}, 0, 0)
	first := postPlanned(server, "plan-001", "10000")
	first.check(t, http.StatusOK,
		`"before":{"from_minor":"20000","to_minor":"2000","fee_minor":"0"}`,
		`"after":{"from_minor":"9500","to_minor":"12000","fee_minor":"500"}`,
		`"kind":"debit","account":"A","currency":"KRW","delta_minor":"-10500"`,
		`"kind":"credit","account":"B","currency":"KRW","delta_minor":"10000"`,
		`"kind":"fee","account":"fee-KRW","currency":"KRW","delta_minor":"500"`)
	repeat := postPlanned(server, "plan-001", "10000")
	repeat.check(t, http.StatusOK)
	if repeat.body != first.body {
		t.Fatalf("retry changed the receipt: %s", repeat.body)
	}
	postPlanned(server, "plan-001", "1").check(t, http.StatusConflict, "E_REQUEST_CONFLICT")
	assertPlannedDB(t, db, [3]int64{9500, 12000, 500}, 1, 3)
	var audit string
	if err := db.QueryRowContext(t.Context(), "SELECT CAST(audit AS TEXT) FROM transfers WHERE request_id = 'plan-001'").Scan(&audit); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"record":"transfer_plan"`, `"program_sha256":`, `"contract_sha256":`,
		`"debit":{"currency":"KRW","amount":"10500"}`} {
		if !strings.Contains(audit, want) {
			t.Fatalf("audit omitted %s: %s", want, audit)
		}
	}
}

func TestServePlannedTransferUsesFinlangPlan(t *testing.T) {
	server, db := plannedServer(t, func(source string) string {
		return strings.Replace(source, "0.05", "0.10", 1)
	})
	postPlanned(server, "changed-rule", "10000").check(t, http.StatusOK,
		`"after":{"from_minor":"9000","to_minor":"12000","fee_minor":"1000"}`,
		`"delta_minor":"-11000"`)
	assertPlannedDB(t, db, [3]int64{9000, 12000, 1000}, 1, 3)
}

func TestServePlannedTransferRejectsRuleThatChangesPrincipal(t *testing.T) {
	server, db := plannedServer(t, func(source string) string {
		return strings.Replace(source, "debit: transfer.amount + fee, credit: transfer.amount",
			"debit: transfer.amount + fee + fee, credit: transfer.amount + fee", 1)
	})
	postPlanned(server, "invalid-plan", "10000").check(t, http.StatusInternalServerError, "E_TRANSFER_PLAN")
	assertPlannedDB(t, db, [3]int64{20000, 2000, 0}, 0, 0)
}

func TestServePlannedTransferRollsBack(t *testing.T) {
	server, db := plannedServer(t, nil)
	_, err := db.ExecContext(t.Context(), `
		CREATE TRIGGER fail_planned_fee BEFORE INSERT ON postings WHEN NEW.kind = 'fee'
		BEGIN SELECT RAISE(ABORT, 'injected planned posting failure'); END
	`)
	if err != nil {
		t.Fatal(err)
	}
	postPlanned(server, "rollback", "10000").check(t, http.StatusInternalServerError)
	assertPlannedDB(t, db, [3]int64{20000, 2000, 0}, 0, 0)
	if _, err := db.ExecContext(t.Context(), "DROP TRIGGER fail_planned_fee"); err != nil {
		t.Fatal(err)
	}
	postPlanned(server, "rollback", "10000").check(t, http.StatusOK)
	assertPlannedDB(t, db, [3]int64{9500, 12000, 500}, 1, 3)
}

func TestServePlannedTransferConcurrentDebit(t *testing.T) {
	server, db := plannedServer(t, nil)
	start := make(chan struct{})
	results := make(chan plannedResponse, 2)
	for _, id := range []string{"concurrent-1", "concurrent-2"} {
		go func() {
			<-start
			results <- postPlanned(server, id, "10000")
		}()
	}
	close(start)
	var success, insufficient int
	for range 2 {
		result := <-results
		switch result.status {
		case http.StatusOK:
			result.check(t, http.StatusOK, `"delta_minor":"-10500"`)
			success++
		case http.StatusConflict:
			result.check(t, http.StatusConflict, "E_INSUFFICIENT_FUNDS")
			insufficient++
		default:
			t.Fatalf("unexpected concurrent result: %+v", result)
		}
	}
	if success != 1 || insufficient != 1 {
		t.Fatalf("successful=%d, insufficient=%d", success, insufficient)
	}
	assertPlannedDB(t, db, [3]int64{9500, 12000, 500}, 1, 3)
}
