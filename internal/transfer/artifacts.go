package transfer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/Pirite08/finlang/internal/config"
)

// ErrPlanArtifactConflict means a revision was already registered with different bytes.
var ErrPlanArtifactConflict = errors.New("transfer plan artifact version already has different content")

// PlanArtifact contains the original bytes of an independently versioned rule or contract.
type PlanArtifact struct {
	Kind, Rule, Version, SHA256 string
	Data                        []byte
}

// RegisterPlanArtifacts atomically stores immutable versions. Registering the
// same version with the same content is safe when the server restarts.
func (s *Store) RegisterPlanArtifacts(ctx context.Context, artifacts []PlanArtifact) error {
	if len(artifacts) == 0 {
		return nil
	}
	limit := config.DefaultLimits().MaxSourceBytes
	for _, artifact := range artifacts {
		if !validPlanArtifactKey(artifact.Kind, artifact.Rule, artifact.Version) ||
			!matchesPlanArtifactData(artifact.Data, artifact.SHA256, limit) {
			return fmt.Errorf("%w: invalid archived rule or contract", ErrPlan)
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, artifact := range artifacts {
		inserted, err := tx.ExecContext(ctx, `
			INSERT INTO transfer_plan_artifacts (kind, rule_name, version, sha256, content)
			VALUES (?, ?, ?, ?, ?) ON CONFLICT (kind, rule_name, version) DO NOTHING
		`, artifact.Kind, artifact.Rule, artifact.Version, artifact.SHA256, artifact.Data)
		if err != nil {
			return err
		}
		rows, err := inserted.RowsAffected()
		if err != nil {
			return err
		}
		if rows == 1 {
			continue
		}
		var digest string
		var content []byte
		err = tx.QueryRowContext(ctx, `
			SELECT sha256, CASE WHEN length(content) <= ? THEN content END
			FROM transfer_plan_artifacts WHERE kind = ? AND rule_name = ? AND version = ?
		`, limit, artifact.Kind, artifact.Rule, artifact.Version).Scan(&digest, &content)
		if err != nil {
			return err
		}
		if digest != artifact.SHA256 || !bytes.Equal(content, artifact.Data) {
			return fmt.Errorf("%w: %s %q version %q", ErrPlanArtifactConflict, artifact.Kind, artifact.Rule, artifact.Version)
		}
	}
	return tx.Commit()
}

// LoadPlanArtifact verifies the expected digest against both the archive
// metadata and the original bytes. An empty version permits exact-digest lookup
// for older audit records whose metadata predates version identifiers.
func (s *Store) LoadPlanArtifact(ctx context.Context, kind, rule, version, digest string) (PlanArtifact, error) {
	if !validPlanArtifactName(kind, rule) || (version != "" && !validArtifactLabel(version, 128)) || !validArtifactDigest(digest) {
		return PlanArtifact{}, ErrAuditMismatch
	}
	limit := config.DefaultLimits().MaxSourceBytes
	const query = `
		SELECT version, sha256, CASE WHEN length(content) <= ? THEN content END
		FROM transfer_plan_artifacts WHERE kind = ? AND rule_name = ?
	`
	var row *sql.Row
	if version == "" {
		row = s.db.QueryRowContext(ctx, query+" AND sha256 = ? ORDER BY version LIMIT 1", limit, kind, rule, digest)
	} else {
		row = s.db.QueryRowContext(ctx, query+" AND version = ?", limit, kind, rule, version)
	}
	artifact := PlanArtifact{Kind: kind, Rule: rule}
	if err := row.Scan(&artifact.Version, &artifact.SHA256, &artifact.Data); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PlanArtifact{}, fmt.Errorf("%w: archived %s for %q", ErrNotFound, kind, rule)
		}
		return PlanArtifact{}, err
	}
	if !validArtifactLabel(artifact.Version, 128) || artifact.SHA256 != digest ||
		!matchesPlanArtifactData(artifact.Data, digest, limit) {
		return PlanArtifact{}, ErrAuditMismatch
	}
	return artifact, nil
}

func validPlanArtifactKey(kind, rule, version string) bool {
	return validPlanArtifactName(kind, rule) && validArtifactLabel(version, 128)
}

func validPlanArtifactName(kind, rule string) bool {
	return (kind == "rule" || kind == "contract") && validArtifactLabel(rule, 256)
}

func validArtifactLabel(label string, maxLength int) bool {
	if len(label) == 0 || len(label) > maxLength {
		return false
	}
	for _, ch := range label {
		if !(ch >= '0' && ch <= '9' || ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' ||
			ch == '-' || ch == '_' || ch == '.') {
			return false
		}
	}
	return true
}

func validArtifactDigest(digest string) bool {
	if len(digest) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(digest)
	return err == nil && hex.EncodeToString(decoded) == digest
}

func matchesPlanArtifactData(data []byte, digest string, limit int) bool {
	if len(data) == 0 || len(data) > limit {
		return false
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]) == digest
}

// ReadPlannedTransfer verifies the historical receipt and reads its audit
// within one SQLite snapshot. It never runs the posting transaction.
func (s *Store) ReadPlannedTransfer(ctx context.Context, id string) (Receipt, []byte, error) {
	if !validID(id) {
		return Receipt{}, nil, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Receipt{}, nil, err
	}
	defer tx.Rollback()
	result, err := findResult(ctx, tx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Receipt{}, nil, ErrNotFound
		}
		return Receipt{}, nil, err
	}
	// JSON escaping can expand each raw input byte to six bytes. The result
	// Record and its metadata are small and fit in the remaining allowance.
	maxAuditBytes := 6*config.DefaultLimits().MaxSourceBytes + (64 << 10)
	var audit []byte
	err = tx.QueryRowContext(ctx, `
		SELECT CASE WHEN length(audit) <= ? THEN audit END FROM transfers WHERE request_id = ?
	`, maxAuditBytes, id).Scan(&audit)
	if err != nil {
		return Receipt{}, nil, err
	}
	if len(audit) == 0 {
		return Receipt{}, nil, ErrAuditMismatch
	}
	receipt, err := readReceipt(ctx, tx, result)
	if err != nil {
		return Receipt{}, nil, err
	}
	return receipt, audit, nil
}
