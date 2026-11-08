// Package transfer implements the SQLite ledger for the local transfer demo.
package transfer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

var (
	ErrInvalid           = errors.New("invalid transfer")
	ErrNotFound          = errors.New("account or transfer not found")
	ErrCurrency          = errors.New("accounts and transfer must share a currency")
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrConflict          = errors.New("request ID is already used for another transfer")
	ErrPlan              = errors.New("invalid transfer plan")
	ErrAuditMismatch     = errors.New("stored transfer did not match audit replay")
)

type Request struct {
	ID       string `json:"request_id"`
	From     string `json:"from_account"`
	To       string `json:"to_account"`
	Currency string `json:"currency"`
	Amount   int64  `json:"amount_minor"`
}

type Result struct {
	ID       string `json:"request_id"`
	From     string `json:"from_account"`
	To       string `json:"to_account"`
	Currency string `json:"currency"`
	Amount   int64  `json:"amount_minor"`
	Fee      int64  `json:"fee_minor"`
	Debit    int64  `json:"debit_minor"`
}

type Store struct {
	db *sql.DB
}

// Open creates the local demo schema without resetting existing state.
func Open(ctx context.Context, path string) (*Store, error) {
	store, err := open(ctx, path, false)
	if err != nil {
		return nil, err
	}
	if err := store.initSchema(ctx); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}

// OpenReadOnly opens an existing ledger without creating or migrating it.
func OpenReadOnly(ctx context.Context, path string) (*Store, error) {
	return open(ctx, path, true)
}

func open(ctx context.Context, path string, readOnly bool) (*Store, error) {
	if path == "" {
		return nil, errors.New("a SQLite database path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	slashPath := filepath.ToSlash(absolute)
	if !strings.HasPrefix(slashPath, "/") {
		slashPath = "/" + slashPath
	}
	dsn := url.URL{Scheme: "file", Path: slashPath}
	params := url.Values{"_busy_timeout": {"5000"}, "_pragma": {"foreign_keys(1)"}}
	if readOnly {
		params.Set("mode", "ro")
	} else {
		// Obtain the write lock before checking request IDs and balances.
		params.Set("_txlock", "immediate")
	}
	dsn.RawQuery = params.Encode()
	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) initSchema(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("initialize transfer schema: %w", err)
	}
	return tx.Commit()
}

// SeedDemo inserts fictitious accounts once, retaining any subsequent balances.
func (s *Store) SeedDemo(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO accounts (id, currency, balance) VALUES
			('A', 'KRW', 20000), ('B', 'KRW', 2000), ('fee-KRW', 'KRW', 0),
			('U1', 'USD', 20000), ('U2', 'USD', 2000), ('fee-USD', 'USD', 0)
		ON CONFLICT(id) DO NOTHING
	`)
	return err
}

const schema = `
CREATE TABLE IF NOT EXISTS accounts (
	id TEXT NOT NULL PRIMARY KEY,
	currency TEXT NOT NULL CHECK (currency IN ('KRW', 'USD')),
	balance INTEGER NOT NULL CHECK (balance >= 0),
	UNIQUE (id, currency)
) STRICT;

CREATE TABLE IF NOT EXISTS transfers (
	request_id TEXT NOT NULL PRIMARY KEY,
	from_account TEXT NOT NULL,
	to_account TEXT NOT NULL CHECK (to_account <> from_account),
	currency TEXT NOT NULL CHECK (currency IN ('KRW', 'USD')),
	amount INTEGER NOT NULL CHECK (amount > 0),
	fee INTEGER NOT NULL CHECK (fee >= 0),
	debit INTEGER NOT NULL CHECK (debit - fee = amount),
	audit BLOB NOT NULL CHECK (length(audit) > 0),
	UNIQUE (request_id, currency),
	FOREIGN KEY (from_account, currency) REFERENCES accounts (id, currency),
	FOREIGN KEY (to_account, currency) REFERENCES accounts (id, currency)
) STRICT;

CREATE TABLE IF NOT EXISTS postings (
	request_id TEXT NOT NULL,
	kind TEXT NOT NULL CHECK (kind IN ('debit', 'credit', 'fee')),
	account TEXT NOT NULL,
	currency TEXT NOT NULL,
	delta INTEGER NOT NULL CHECK (
		(kind = 'debit' AND delta < 0) OR
		(kind = 'credit' AND delta > 0) OR
		(kind = 'fee' AND delta >= 0)
	),
	PRIMARY KEY (request_id, kind),
	FOREIGN KEY (request_id, currency) REFERENCES transfers (request_id, currency),
	FOREIGN KEY (account, currency) REFERENCES accounts (id, currency)
) STRICT;

CREATE TABLE IF NOT EXISTS transfer_snapshots (
	request_id TEXT NOT NULL PRIMARY KEY REFERENCES transfers (request_id),
	from_before INTEGER NOT NULL CHECK (from_before >= 0),
	to_before INTEGER NOT NULL CHECK (to_before >= 0),
	fee_before INTEGER NOT NULL CHECK (fee_before >= 0),
	from_after INTEGER NOT NULL CHECK (from_after >= 0),
	to_after INTEGER NOT NULL CHECK (to_after >= 0),
	fee_after INTEGER NOT NULL CHECK (fee_after >= 0)
) STRICT;

CREATE TABLE IF NOT EXISTS transfer_plan_artifacts (
	kind TEXT NOT NULL CHECK (kind IN ('rule', 'contract')),
	rule_name TEXT NOT NULL,
	version TEXT NOT NULL,
	sha256 TEXT NOT NULL CHECK (length(sha256) = 64),
	content BLOB NOT NULL CHECK (length(content) > 0),
	PRIMARY KEY (kind, rule_name, version)
) STRICT;
`
