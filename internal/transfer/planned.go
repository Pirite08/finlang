package transfer

import (
	"context"
	"database/sql"
	"errors"
	"slices"
)

// Plan contains validated rule output in the request currency's minor unit.
type Plan struct {
	Debit, Credit, Fee int64
	Audit              []byte
}

type Balances struct {
	From int64 `json:"from_minor,string"`
	To   int64 `json:"to_minor,string"`
	Fee  int64 `json:"fee_minor,string"`
}

type Posting struct {
	Kind     string `json:"kind"`
	Account  string `json:"account"`
	Currency string `json:"currency"`
	Delta    int64  `json:"delta_minor,string"`
}

type Receipt struct {
	Result        Result
	Before, After *Balances
	Postings      []Posting
}

// TransferPlanned validates and applies a plan built inside the ledger
// transaction. A matching request ID returns the original stored receipt.
func (s *Store) TransferPlanned(ctx context.Context, req Request, build func() (Plan, error)) (Receipt, error) {
	return s.transfer(ctx, req, build, true)
}

func readBalances(ctx context.Context, tx *sql.Tx, req Request, includeFee bool) (Balances, error) {
	var balances Balances
	accounts := []struct {
		id      string
		balance *int64
	}{
		{req.From, &balances.From},
		{req.To, &balances.To},
		{feeAccount(req.Currency), &balances.Fee},
	}
	if !includeFee {
		accounts = accounts[:2]
	}
	for _, account := range accounts {
		var currency string
		err := tx.QueryRowContext(ctx, "SELECT currency, balance FROM accounts WHERE id = ?", account.id).
			Scan(&currency, account.balance)
		if errors.Is(err, sql.ErrNoRows) {
			return Balances{}, ErrNotFound
		}
		if err != nil {
			return Balances{}, err
		}
		if currency != req.Currency {
			return Balances{}, ErrCurrency
		}
	}
	return balances, nil
}

func saveReceipt(ctx context.Context, tx *sql.Tx, result Result, before Balances) (Receipt, error) {
	after, err := readBalances(ctx, tx, result.request(), true)
	if err != nil {
		return Receipt{}, err
	}
	if !balanceDeltasMatch(result, before, after) {
		return Receipt{}, ErrAuditMismatch
	}
	postings, err := readReceiptPostings(ctx, tx, result)
	if err != nil {
		return Receipt{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO transfer_snapshots (
			request_id, from_before, to_before, fee_before, from_after, to_after, fee_after
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, result.ID, before.From, before.To, before.Fee, after.From, after.To, after.Fee); err != nil {
		return Receipt{}, err
	}
	return Receipt{Result: result, Before: &before, After: &after, Postings: postings}, nil
}

func readReceipt(ctx context.Context, tx *sql.Tx, result Result) (Receipt, error) {
	var before, after Balances
	err := tx.QueryRowContext(ctx, `
		SELECT from_before, to_before, fee_before, from_after, to_after, fee_after
		FROM transfer_snapshots WHERE request_id = ?
	`, result.ID).Scan(&before.From, &before.To, &before.Fee, &after.From, &after.To, &after.Fee)
	receipt := Receipt{Result: result}
	switch {
	case err == nil:
		if !balanceDeltasMatch(result, before, after) {
			return Receipt{}, ErrAuditMismatch
		}
		receipt.Before, receipt.After = &before, &after
	case !errors.Is(err, sql.ErrNoRows):
		return Receipt{}, err
	}
	postings, err := readReceiptPostings(ctx, tx, result)
	if err != nil {
		return Receipt{}, err
	}
	receipt.Postings = postings
	return receipt, nil
}

func balanceDeltasMatch(result Result, before, after Balances) bool {
	return after.From-before.From == -result.Debit && after.To-before.To == result.Amount && after.Fee-before.Fee == result.Fee
}

func readReceiptPostings(ctx context.Context, tx *sql.Tx, result Result) ([]Posting, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT kind, account, currency, delta FROM postings
		WHERE request_id = ?
		ORDER BY CASE kind WHEN 'debit' THEN 0 WHEN 'credit' THEN 1 ELSE 2 END
	`, result.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	postings := make([]Posting, 0, 3)
	for rows.Next() {
		var posting Posting
		if err := rows.Scan(&posting.Kind, &posting.Account, &posting.Currency, &posting.Delta); err != nil {
			return nil, err
		}
		postings = append(postings, posting)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	expected := []Posting{
		{Kind: "debit", Account: result.From, Currency: result.Currency, Delta: -result.Debit},
		{Kind: "credit", Account: result.To, Currency: result.Currency, Delta: result.Amount},
		{Kind: "fee", Account: feeAccount(result.Currency), Currency: result.Currency, Delta: result.Fee},
	}
	if !slices.Equal(postings, expected) {
		return nil, ErrAuditMismatch
	}
	return postings, nil
}
