package transfer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"

	"github.com/Pirite08/finlang/internal/transferfee"
)

// Replay recalculates the archived fee and validates the stored receipt and postings.
// It only reads the ledger and is safe to call on a read-only connection.
func (s *Store) Replay(ctx context.Context, id string) (Result, error) {
	if !validID(id) {
		return Result{}, ErrInvalid
	}
	var result Result
	var audit []byte
	var debitAccount, creditAccount, feesAccount string
	var debit, credit, fees int64
	// One query reads the receipt and postings from the same SQLite snapshot.
	err := s.db.QueryRowContext(ctx, `
		SELECT t.request_id, t.from_account, t.to_account, t.currency, t.amount, t.fee, t.debit, t.audit,
			COALESCE(d.account, ''), COALESCE(d.delta, 0),
			COALESCE(c.account, ''), COALESCE(c.delta, 0),
			COALESCE(f.account, ''), COALESCE(f.delta, 0)
		FROM transfers t
		LEFT JOIN postings d ON d.request_id = t.request_id AND d.currency = t.currency AND d.kind = 'debit'
		LEFT JOIN postings c ON c.request_id = t.request_id AND c.currency = t.currency AND c.kind = 'credit'
		LEFT JOIN postings f ON f.request_id = t.request_id AND f.currency = t.currency AND f.kind = 'fee'
		WHERE t.request_id = ?
	`, id).Scan(
		&result.ID, &result.From, &result.To, &result.Currency, &result.Amount, &result.Fee, &result.Debit, &audit,
		&debitAccount, &debit, &creditAccount, &credit, &feesAccount, &fees,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Result{}, ErrNotFound
	}
	if err != nil {
		return Result{}, err
	}
	fee, err := transferfee.Replay(audit, result.Currency, result.Amount)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrAuditMismatch, err)
	}
	if !validRequest(result.request()) || fee < 0 || fee != result.Fee ||
		result.Amount > math.MaxInt64-fee || result.Amount+fee != result.Debit ||
		debitAccount != result.From || debit != -result.Debit ||
		creditAccount != result.To || credit != result.Amount ||
		feesAccount != feeAccount(result.Currency) || fees != fee {
		return Result{}, ErrAuditMismatch
	}
	return result, nil
}
