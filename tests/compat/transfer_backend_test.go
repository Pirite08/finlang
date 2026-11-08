package compat

import (
	"database/sql"
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/transfer"
)

func runTransferBackend(t *testing.T, tc transferCase) transferOutcome {
	t.Helper()
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "ledger.sqlite")
	store, err := transfer.Open(ctx, path)
	if err != nil {
		t.Fatalf("open transfer backend: %v", err)
	}
	defer store.Close()

	// Inspect the persisted state through a connection independent of Store.
	slashPath := filepath.ToSlash(path)
	if !strings.HasPrefix(slashPath, "/") {
		slashPath = "/" + slashPath
	}
	dsn := url.URL{Scheme: "file", Path: slashPath, RawQuery: "mode=rw"}
	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		t.Fatalf("open transfer fixture connection: %v", err)
	}
	defer db.Close()

	req := tc.Request
	feeAccount := "fee-" + req.Currency
	if _, err := db.ExecContext(ctx, `
		INSERT INTO accounts (id, currency, balance) VALUES (?, ?, ?), (?, ?, ?), (?, ?, ?)
	`, req.From, req.Currency, tc.Initial.From,
		req.To, req.Currency, tc.Initial.To,
		feeAccount, req.Currency, tc.Initial.Fee); err != nil {
		t.Fatalf("seed transfer fixture: %v", err)
	}

	outcome := transferOutcome{Currency: req.Currency}
	result, err := store.Transfer(ctx, req)
	switch {
	case err == nil:
		if result.ID != req.ID || result.From != req.From || result.To != req.To || result.Amount != req.Amount {
			t.Fatalf("transfer response did not echo the request: %+v", result)
		}
		outcome.Status, outcome.Currency = "ok", result.Currency
		outcome.Fee, outcome.Debit = result.Fee, result.Debit
	case errors.Is(err, transfer.ErrInsufficientFunds):
		outcome.Status = "insufficient_funds"
	case errors.Is(err, transfer.ErrInvalid):
		outcome.Status = "invalid"
	default:
		t.Fatalf("unexpected backend failure: %v", err)
	}

	for _, row := range []struct {
		account string
		balance *int64
	}{
		{req.From, &outcome.Final.From},
		{req.To, &outcome.Final.To},
		{feeAccount, &outcome.Final.Fee},
	} {
		var currency string
		err := db.QueryRowContext(ctx, `SELECT currency, balance FROM accounts WHERE id = ?`, row.account).
			Scan(&currency, row.balance)
		if err != nil || currency != req.Currency {
			t.Fatalf("read final balance for %s: currency=%q err=%v", row.account, currency, err)
		}
	}

	rows, err := db.QueryContext(ctx, `
		SELECT request_id, kind, account, currency, delta FROM postings
		ORDER BY CASE kind WHEN 'debit' THEN 0 WHEN 'credit' THEN 1 ELSE 2 END
	`)
	if err != nil {
		t.Fatalf("query ledger postings: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var posting transferPosting
		if err := rows.Scan(&id, &posting.Kind, &posting.Account, &posting.Currency, &posting.Delta); err != nil {
			t.Fatalf("read ledger posting: %v", err)
		}
		if id != req.ID {
			t.Fatalf("posting belongs to request %q, want %q", id, req.ID)
		}
		outcome.Postings = append(outcome.Postings, posting)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate ledger postings: %v", err)
	}

	var receipts, audits int
	if err := db.QueryRowContext(ctx, `SELECT count(*), count(CASE WHEN length(audit) > 0 THEN 1 END) FROM transfers`).
		Scan(&receipts, &audits); err != nil {
		t.Fatalf("query transfer receipts and audits: %v", err)
	}
	if outcome.Status != "ok" {
		if receipts != 0 || audits != 0 || len(outcome.Postings) != 0 {
			t.Fatalf("rejected transfer persisted %d receipts, %d audits and %d postings", receipts, audits, len(outcome.Postings))
		}
		return outcome
	}
	if receipts != 1 || audits != 1 {
		t.Fatalf("committed transfer persisted %d receipts and %d audits, want one each", receipts, audits)
	}
	replayed, err := store.Replay(ctx, req.ID)
	if err != nil || replayed != result {
		t.Fatalf("replay committed transfer: result=%+v err=%v want=%+v", replayed, err, result)
	}
	return outcome
}
