package transfer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/Pirite08/finlang/internal/transferfee"
)

// Transfer commits the receipt, three postings, balances and fee audit together.
func (s *Store) Transfer(ctx context.Context, req Request) (Result, error) {
	receipt, err := s.transfer(ctx, req, func() (Plan, error) {
		fee, audit, err := transferfee.Calculate(req.Currency, req.Amount)
		if err != nil {
			return Plan{}, fmt.Errorf("calculate transfer fee: %w", err)
		}
		if fee < 0 || req.Amount > math.MaxInt64-fee {
			return Plan{}, ErrInvalid
		}
		return Plan{Debit: req.Amount + fee, Credit: req.Amount, Fee: fee, Audit: audit}, nil
	}, false)
	return receipt.Result, err
}

func (s *Store) transfer(ctx context.Context, req Request, build func() (Plan, error), withSnapshot bool) (Receipt, error) {
	if !validRequest(req) {
		return Receipt{}, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Receipt{}, err
	}
	defer tx.Rollback()

	result, err := findResult(ctx, tx, req.ID)
	switch {
	case err == nil:
		if result.request() != req {
			return Receipt{}, ErrConflict
		}
		if withSnapshot {
			return readReceipt(ctx, tx, result)
		}
		return Receipt{Result: result}, nil
	case !errors.Is(err, sql.ErrNoRows):
		return Receipt{}, err
	}

	before, err := readBalances(ctx, tx, req, withSnapshot)
	if err != nil {
		return Receipt{}, err
	}
	if build == nil {
		return Receipt{}, ErrPlan
	}
	plan, err := build()
	if err != nil {
		return Receipt{}, err
	}
	if plan.Credit != req.Amount || plan.Fee < 0 || plan.Credit > math.MaxInt64-plan.Fee ||
		plan.Debit != plan.Credit+plan.Fee || len(plan.Audit) == 0 {
		return Receipt{}, ErrPlan
	}
	result = Result{
		ID: req.ID, From: req.From, To: req.To, Currency: req.Currency,
		Amount: plan.Credit, Fee: plan.Fee, Debit: plan.Debit,
	}
	if err := postTransfer(ctx, tx, result, plan.Audit); err != nil {
		return Receipt{}, err
	}
	receipt := Receipt{Result: result}
	if withSnapshot {
		receipt, err = saveReceipt(ctx, tx, result, before)
		if err != nil {
			return Receipt{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}

func validRequest(req Request) bool {
	return validID(req.ID) && validID(req.From) && validID(req.To) &&
		!strings.HasPrefix(req.From, "fee-") && !strings.HasPrefix(req.To, "fee-") &&
		req.From != req.To && req.Amount > 0 && (req.Currency == "KRW" || req.Currency == "USD")
}

func validID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}
	for _, ch := range id {
		if !(ch >= '0' && ch <= '9' || ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' || ch == '-' || ch == '_' || ch == '.') {
			return false
		}
	}
	return true
}

func (r Result) request() Request {
	return Request{ID: r.ID, From: r.From, To: r.To, Currency: r.Currency, Amount: r.Amount}
}

func findResult(ctx context.Context, tx *sql.Tx, id string) (Result, error) {
	var r Result
	err := tx.QueryRowContext(ctx, `
		SELECT request_id, from_account, to_account, currency, amount, fee, debit
		FROM transfers WHERE request_id = ?
	`, id).Scan(&r.ID, &r.From, &r.To, &r.Currency, &r.Amount, &r.Fee, &r.Debit)
	return r, err
}

func postTransfer(ctx context.Context, tx *sql.Tx, r Result, audit []byte) error {
	changed, err := tx.ExecContext(ctx, `
		UPDATE accounts SET balance = balance - ?
		WHERE id = ? AND currency = ? AND balance >= ?
	`, r.Debit, r.From, r.Currency, r.Debit)
	if err := requireChange(changed, err, ErrInsufficientFunds); err != nil {
		return err
	}
	if err := credit(ctx, tx, r.To, r.Currency, r.Amount); err != nil {
		return err
	}
	if err := credit(ctx, tx, feeAccount(r.Currency), r.Currency, r.Fee); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO transfers (request_id, from_account, to_account, currency, amount, fee, debit, audit)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, r.ID, r.From, r.To, r.Currency, r.Amount, r.Fee, r.Debit, audit); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO postings (request_id, kind, account, currency, delta)
		VALUES (?, 'debit', ?, ?, ?), (?, 'credit', ?, ?, ?), (?, 'fee', ?, ?, ?)
	`, r.ID, r.From, r.Currency, -r.Debit, r.ID, r.To, r.Currency, r.Amount, r.ID, feeAccount(r.Currency), r.Currency, r.Fee)
	return err
}

func feeAccount(currency string) string { return "fee-" + currency }

func credit(ctx context.Context, tx *sql.Tx, id, currency string, amount int64) error {
	changed, err := tx.ExecContext(ctx, `
		UPDATE accounts SET balance = balance + ?
		WHERE id = ? AND currency = ? AND balance <= ?
	`, amount, id, currency, math.MaxInt64-amount)
	return requireChange(changed, err, ErrInvalid)
}

func requireChange(result sql.Result, execErr, missing error) error {
	if execErr != nil {
		return execErr
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return missing
	}
	return nil
}
