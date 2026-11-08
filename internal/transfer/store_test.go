package transfer

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var demoRequest = Request{ID: "transfer-1", From: "A", To: "B", Currency: "KRW", Amount: 10000}

func testStore(t *testing.T) (context.Context, *Store, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	path := filepath.Join(t.TempDir(), "이체 시험 #1.sqlite")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.SeedDemo(ctx); err != nil {
		t.Fatal(err)
	}
	return ctx, store, path
}

func assertBalances(t *testing.T, ctx context.Context, store *Store, ids []string, balances ...int64) {
	t.Helper()
	for i, id := range ids {
		var balance int64
		if err := store.db.QueryRowContext(ctx, "SELECT balance FROM accounts WHERE id = ?", id).Scan(&balance); err != nil {
			t.Fatal(err)
		}
		if balance != balances[i] {
			t.Errorf("balance %s = %d, want %d", id, balance, balances[i])
		}
	}
}

func assertLedgerCounts(t *testing.T, ctx context.Context, store *Store, transfers, postings int) {
	t.Helper()
	var gotTransfers, gotPostings int
	if err := store.db.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM transfers), (SELECT count(*) FROM postings)").Scan(&gotTransfers, &gotPostings); err != nil {
		t.Fatal(err)
	}
	if gotTransfers != transfers || gotPostings != postings {
		t.Fatalf("got %d transfers and %d postings, want %d and %d", gotTransfers, gotPostings, transfers, postings)
	}
}

func TestTransferCommitsAndReplaysReadOnly(t *testing.T) {
	ctx, store, path := testStore(t)
	result, err := store.Transfer(ctx, demoRequest)
	want := Result{ID: "transfer-1", From: "A", To: "B", Currency: "KRW", Amount: 10000, Fee: 500, Debit: 10500}
	if err != nil || result != want {
		t.Fatalf("got %+v, %v, want %+v", result, err, want)
	}
	ids := []string{"A", "B", "fee-KRW"}
	assertBalances(t, ctx, store, ids, 9500, 12000, 500)
	assertLedgerCounts(t, ctx, store, 1, 3)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	got, err := reader.Replay(ctx, demoRequest.ID)
	if err != nil || got != want {
		t.Fatalf("replay %+v, %v, want %+v", got, err, want)
	}
	if _, err := reader.db.ExecContext(ctx, "DELETE FROM postings"); err == nil {
		t.Fatal("read-only replay database accepted a write")
	}
	assertBalances(t, ctx, reader, ids, 9500, 12000, 500)
	assertLedgerCounts(t, ctx, reader, 1, 3)
}

func TestTransferRollsBackAfterDatabaseFailure(t *testing.T) {
	for _, tc := range []struct{ name, trigger string }{
		{"credit fails after debit", `
			CREATE TRIGGER fail_transfer BEFORE UPDATE ON accounts
			WHEN NEW.id = 'B' AND (SELECT balance FROM accounts WHERE id = 'A') = 9500
			BEGIN SELECT RAISE(ABORT, 'injected credit failure'); END
		`},
		{"fee posting fails after receipt and balance updates", `
			CREATE TRIGGER fail_transfer BEFORE INSERT ON postings
			WHEN NEW.kind = 'fee' AND (SELECT balance FROM accounts WHERE id = 'fee-KRW') = 500
			AND (SELECT count(*) FROM transfers WHERE request_id = NEW.request_id) = 1
			BEGIN SELECT RAISE(ABORT, 'injected fee posting failure'); END
		`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, store, _ := testStore(t)
			if _, err := store.db.ExecContext(ctx, tc.trigger); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Transfer(ctx, demoRequest); err == nil || !strings.Contains(err.Error(), "injected") {
				t.Fatalf("expected injected DB error, got %v", err)
			}
			assertBalances(t, ctx, store, []string{"A", "B", "fee-KRW"}, 20000, 2000, 0)
			assertLedgerCounts(t, ctx, store, 0, 0)
			if _, err := store.Replay(ctx, demoRequest.ID); !errors.Is(err, ErrNotFound) {
				t.Fatalf("rolled-back transfer has audit result: %v", err)
			}
			if _, err := store.db.ExecContext(ctx, "DROP TRIGGER fail_transfer"); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Transfer(ctx, demoRequest); err != nil {
				t.Fatalf("retry after rollback: %v", err)
			}
			assertBalances(t, ctx, store, []string{"A", "B", "fee-KRW"}, 9500, 12000, 500)
		})
	}
}

func TestDuplicateTransferSurvivesReopen(t *testing.T) {
	ctx, store, path := testStore(t)
	want, err := store.Transfer(ctx, demoRequest)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.SeedDemo(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Transfer(ctx, demoRequest)
	if err != nil || got != want {
		t.Fatalf("duplicate %+v, %v, want %+v", got, err, want)
	}
	conflict := demoRequest
	conflict.Amount++
	if _, err := reopened.Transfer(ctx, conflict); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting duplicate: %v", err)
	}
	assertLedgerCounts(t, ctx, reopened, 1, 3)
	assertBalances(t, ctx, reopened, []string{"A", "B", "fee-KRW"}, 9500, 12000, 500)
}

func TestConcurrentTransfersUseDatabaseIsolation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		secondID string
		winners  int
	}{
		{"distinct requests compete for the same balance", "transfer-2", 1},
		{"same request is idempotent", demoRequest.ID, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, store, path := testStore(t)
			second, err := Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer second.Close()

			start := make(chan struct{})
			results := make(chan error, 2)
			for i, db := range []*Store{store, second} {
				request := demoRequest
				if i == 1 {
					request.ID = tc.secondID
				}
				go func() {
					<-start
					_, err := db.Transfer(ctx, request)
					results <- err
				}()
			}
			close(start)
			var winners int
			for range 2 {
				switch err := <-results; {
				case err == nil:
					winners++
				case errors.Is(err, ErrInsufficientFunds):
				default:
					t.Errorf("unexpected concurrent transfer error: %v", err)
				}
			}
			if winners != tc.winners {
				t.Fatalf("got %d successful responses, want %d", winners, tc.winners)
			}
			assertLedgerCounts(t, ctx, store, 1, 3)
			assertBalances(t, ctx, store, []string{"A", "B", "fee-KRW"}, 9500, 12000, 500)
		})
	}
}

func TestTransferRejectsInvalidAccountsAndOverflow(t *testing.T) {
	ctx, store, _ := testStore(t)
	for _, tc := range []struct {
		from, to, currency string
		amount             int64
		err                error
	}{
		{"A", "U1", "KRW", 1000, ErrCurrency},
		{"A", "B", "USD", 1000, ErrCurrency},
		{"A", "B", "KRW", 0, ErrInvalid},
		{"A", "A", "KRW", 1000, ErrInvalid},
		{"A", "fee-KRW", "KRW", 1000, ErrInvalid},
		{"A", "missing", "KRW", 1000, ErrNotFound},
		{"A", "B", "KRW", 20000, ErrInsufficientFunds},
		{"A", "B", "KRW", math.MaxInt64, ErrInvalid},
	} {
		request := Request{ID: demoRequest.ID, From: tc.from, To: tc.to, Currency: tc.currency, Amount: tc.amount}
		if _, err := store.Transfer(ctx, request); !errors.Is(err, tc.err) {
			t.Errorf("transfer %+v got %v, want %v", request, err, tc.err)
		}
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE accounts SET balance = ? WHERE id = 'B'", int64(math.MaxInt64)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Transfer(ctx, demoRequest); !errors.Is(err, ErrInvalid) {
		t.Fatalf("credit overflow: %v", err)
	}
	assertLedgerCounts(t, ctx, store, 0, 0)
	assertBalances(t, ctx, store, []string{"A", "B", "fee-KRW"}, 20000, math.MaxInt64, 0)
}

func TestTransferReplayRejectsChangedPostingOrAudit(t *testing.T) {
	for _, query := range []string{
		"UPDATE postings SET delta = 499 WHERE kind = 'fee'",
		"UPDATE transfers SET audit = x'01'",
	} {
		t.Run(query, func(t *testing.T) {
			ctx, store, _ := testStore(t)
			if _, err := store.Transfer(ctx, demoRequest); err != nil {
				t.Fatal(err)
			}
			if _, err := store.db.ExecContext(ctx, query); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Replay(ctx, demoRequest.ID); !errors.Is(err, ErrAuditMismatch) {
				t.Fatalf("changed record replay: %v", err)
			}
			assertLedgerCounts(t, ctx, store, 1, 3)
			assertBalances(t, ctx, store, []string{"A", "B", "fee-KRW"}, 9500, 12000, 500)
		})
	}
}

func TestUSDCentTransfer(t *testing.T) {
	ctx, store, _ := testStore(t)
	request := Request{ID: "usd-1", From: "U1", To: "U2", Currency: "USD", Amount: 10010}
	got, err := store.Transfer(ctx, request)
	if err != nil || got.Fee != 500 || got.Debit != 10510 {
		t.Fatalf("USD transfer: %+v, %v", got, err)
	}
	assertBalances(t, ctx, store, []string{"U1", "U2", "fee-USD"}, 9490, 12010, 500)
}
