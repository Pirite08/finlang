package transfer

import (
	"bytes"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
)

func testPlan() (Plan, error) {
	return Plan{Debit: 10175, Credit: 10000, Fee: 175, Audit: []byte(`{"rule":"test-plan"}`)}, nil
}

func assertSnapshotCount(t *testing.T, store *Store, want int) {
	t.Helper()
	var count int
	if err := store.db.QueryRowContext(t.Context(), "SELECT count(*) FROM transfer_snapshots").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("balance snapshots = %d, want %d", count, want)
	}
}

func TestPlannedReceiptSurvivesReopenAndLaterTransfer(t *testing.T) {
	ctx, store, path := testStore(t)
	first, err := store.TransferPlanned(ctx, demoRequest, testPlan)
	if err != nil {
		t.Fatal(err)
	}
	want := Receipt{
		Result: Result{ID: demoRequest.ID, From: "A", To: "B", Currency: "KRW", Amount: 10000, Fee: 175, Debit: 10175},
		Before: &Balances{From: 20000, To: 2000, Fee: 0},
		After:  &Balances{From: 9825, To: 12000, Fee: 175},
		Postings: []Posting{
			{Kind: "debit", Account: "A", Currency: "KRW", Delta: -10175},
			{Kind: "credit", Account: "B", Currency: "KRW", Delta: 10000},
			{Kind: "fee", Account: "fee-KRW", Currency: "KRW", Delta: 175},
		},
	}
	if !reflect.DeepEqual(first, want) {
		t.Fatalf("receipt %+v, want %+v", first, want)
	}
	var audit []byte
	plan, _ := testPlan()
	if err := store.db.QueryRowContext(ctx, "SELECT audit FROM transfers WHERE request_id = ?", demoRequest.ID).Scan(&audit); err != nil || !bytes.Equal(audit, plan.Audit) {
		t.Fatalf("stored plan evidence = %s, err = %v", audit, err)
	}
	later := demoRequest
	later.ID, later.Amount = "transfer-2", 1000
	if _, err := store.TransferPlanned(ctx, later, func() (Plan, error) {
		return Plan{Debit: 1017, Credit: 1000, Fee: 17, Audit: plan.Audit}, nil
	}); err != nil {
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
	mustNotBuild := func() (Plan, error) {
		t.Fatal("duplicate request rebuilt the plan")
		return Plan{}, nil
	}
	got, err := reopened.TransferPlanned(ctx, demoRequest, mustNotBuild)
	if err != nil || !reflect.DeepEqual(got, first) {
		t.Fatalf("duplicate receipt %+v, err = %v, want %+v", got, err, first)
	}
	conflict := demoRequest
	conflict.Amount++
	if _, err := reopened.TransferPlanned(ctx, conflict, mustNotBuild); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting duplicate = %v, want ErrConflict", err)
	}
	assertBalances(t, ctx, reopened, []string{"A", "B", "fee-KRW"}, 8808, 13000, 192)
	assertLedgerCounts(t, ctx, reopened, 2, 6)
	assertSnapshotCount(t, reopened, 2)
}

func TestPlannedTransferRollsBackAfterLedgerWrites(t *testing.T) {
	ctx, store, _ := testStore(t)
	if _, err := store.db.ExecContext(ctx, `
		CREATE TRIGGER fail_snapshot BEFORE INSERT ON transfer_snapshots
		WHEN (SELECT count(*) FROM postings WHERE request_id = NEW.request_id) = 3
			AND (SELECT balance FROM accounts WHERE id = 'A') = 9825
		BEGIN SELECT RAISE(ABORT, 'injected snapshot failure'); END
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.TransferPlanned(ctx, demoRequest, testPlan); err == nil || !strings.Contains(err.Error(), "injected snapshot failure") {
		t.Fatalf("expected injected failure, got %v", err)
	}
	assertBalances(t, ctx, store, []string{"A", "B", "fee-KRW"}, 20000, 2000, 0)
	assertLedgerCounts(t, ctx, store, 0, 0)
	assertSnapshotCount(t, store, 0)
	if _, err := store.db.ExecContext(ctx, "DROP TRIGGER fail_snapshot"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.TransferPlanned(ctx, demoRequest, testPlan); err != nil {
		t.Fatalf("retry after rollback = %v", err)
	}
	assertLedgerCounts(t, ctx, store, 1, 3)
	assertSnapshotCount(t, store, 1)
}

func TestPlannedTransferRejectsInvalidPlans(t *testing.T) {
	ctx, store, _ := testStore(t)
	for _, tc := range []struct {
		name  string
		req   Request
		alter func(Plan) Plan
	}{
		{"unbalanced", demoRequest, func(p Plan) Plan { p.Debit++; return p }},
		{"different credit", demoRequest, func(p Plan) Plan { p.Credit++; p.Debit++; return p }},
		{"negative fee", demoRequest, func(p Plan) Plan { p.Fee = -1; p.Debit = p.Credit - 1; return p }},
		{"empty audit", demoRequest, func(p Plan) Plan { p.Audit = nil; return p }},
		{"overflow", Request{ID: "overflow", From: "A", To: "B", Currency: "KRW", Amount: math.MaxInt64}, func(p Plan) Plan {
			p.Credit, p.Fee, p.Debit = math.MaxInt64, 1, math.MinInt64
			return p
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := store.TransferPlanned(ctx, tc.req, func() (Plan, error) {
				p, _ := testPlan()
				return tc.alter(p), nil
			})
			if !errors.Is(err, ErrPlan) {
				t.Fatalf("invalid plan = %v, want ErrPlan", err)
			}
		})
	}
	assertBalances(t, ctx, store, []string{"A", "B", "fee-KRW"}, 20000, 2000, 0)
	assertLedgerCounts(t, ctx, store, 0, 0)
	assertSnapshotCount(t, store, 0)
}

func TestPlannedConcurrentDebitsUseSameDatabaseLock(t *testing.T) {
	ctx, store, path := testStore(t)
	second, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	start := make(chan struct{})
	results := make(chan error, 2)
	for i, db := range []*Store{store, second} {
		req := demoRequest
		if i == 1 {
			req.ID = "transfer-2"
		}
		go func() {
			<-start
			_, err := db.TransferPlanned(ctx, req, testPlan)
			results <- err
		}()
	}
	close(start)
	var wins, insufficient int
	for range 2 {
		switch err := <-results; {
		case err == nil:
			wins++
		case errors.Is(err, ErrInsufficientFunds):
			insufficient++
		default:
			t.Errorf("unexpected transfer error = %v", err)
		}
	}
	if wins != 1 || insufficient != 1 {
		t.Fatalf("got %d commits and %d insufficient funds results", wins, insufficient)
	}
	assertBalances(t, ctx, store, []string{"A", "B", "fee-KRW"}, 9825, 12000, 175)
	assertLedgerCounts(t, ctx, store, 1, 3)
	assertSnapshotCount(t, store, 1)
}

func TestPlannedRetryOfLegacyTransferRetainsStoredPostings(t *testing.T) {
	ctx, store, _ := testStore(t)
	legacy, err := store.Transfer(ctx, demoRequest)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.TransferPlanned(ctx, demoRequest, nil)
	if err != nil || got.Result != legacy || got.Before != nil || got.After != nil || len(got.Postings) != 3 || got.Postings[2].Delta != legacy.Fee {
		t.Fatalf("legacy retry receipt %+v, err = %v", got, err)
	}
	replayed, err := store.Replay(ctx, demoRequest.ID)
	if err != nil || replayed != legacy {
		t.Fatalf("legacy audit replay %+v, err = %v", replayed, err)
	}
	assertSnapshotCount(t, store, 0)
}
