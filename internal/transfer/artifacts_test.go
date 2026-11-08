package transfer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"testing"
)

func archiveFixture(kind, version, content string) PlanArtifact {
	sum := sha256.Sum256([]byte(content))
	return PlanArtifact{
		Kind: kind, Rule: "transfer_plan_krw_v1", Version: version,
		SHA256: hex.EncodeToString(sum[:]), Data: []byte(content),
	}
}

func TestPlanArtifactsRetainImmutableVersions(t *testing.T) {
	ctx, store, path := testStore(t)
	rule1 := archiveFixture("rule", "1", "rate 5%")
	rule2 := archiveFixture("rule", "2", "rate 10%")
	contract1 := archiveFixture("contract", "1", "contract one")
	contract2 := archiveFixture("contract", "2", "contract two")
	initial := []PlanArtifact{rule1, contract1}
	for range 2 {
		if err := store.RegisterPlanArtifacts(ctx, initial); err != nil {
			t.Fatal(err)
		}
	}
	reused := archiveFixture("rule", rule1.Version, "rate 20%")
	if err := store.RegisterPlanArtifacts(ctx, []PlanArtifact{contract2, reused}); !errors.Is(err, ErrPlanArtifactConflict) {
		t.Fatalf("version reuse error = %v", err)
	}
	if _, err := store.LoadPlanArtifact(ctx, contract2.Kind, contract2.Rule, contract2.Version, contract2.SHA256); !errors.Is(err, ErrNotFound) {
		t.Fatalf("partially registered contract = %v", err)
	}
	if err := store.RegisterPlanArtifacts(ctx, []PlanArtifact{rule2, contract2}); err != nil {
		t.Fatal(err)
	}
	alias := rule1
	alias.Version = "3"
	if err := store.RegisterPlanArtifacts(ctx, []PlanArtifact{alias}); err != nil {
		t.Fatal(err)
	}

	readOnly, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer readOnly.Close()
	for _, expected := range []PlanArtifact{rule1, rule2, contract1, contract2} {
		actual, err := readOnly.LoadPlanArtifact(ctx, expected.Kind, expected.Rule, expected.Version, expected.SHA256)
		if err != nil || !reflect.DeepEqual(actual, expected) {
			t.Fatalf("loaded %+v, err=%v, want %+v", actual, err, expected)
		}
	}
	actual, err := readOnly.LoadPlanArtifact(ctx, rule1.Kind, rule1.Rule, "", rule1.SHA256)
	if err != nil || !reflect.DeepEqual(actual, rule1) {
		t.Fatalf("legacy lookup %+v, err=%v, want %+v", actual, err, rule1)
	}
	if _, err := readOnly.LoadPlanArtifact(ctx, rule1.Kind, rule1.Rule, rule1.Version, rule2.SHA256); !errors.Is(err, ErrAuditMismatch) {
		t.Fatalf("digest substitution error = %v", err)
	}
	if err := readOnly.RegisterPlanArtifacts(ctx, []PlanArtifact{archiveFixture("rule", "4", "rate 40%")}); err == nil {
		t.Fatal("read-only connection accepted archive registration")
	}
}

func TestPlanArtifactsRejectInvalidAndTamperedBytes(t *testing.T) {
	ctx, store, _ := testStore(t)
	valid := archiveFixture("rule", "1", "rate 5%")
	invalid := valid
	invalid.Data = []byte("wrong source")
	if err := store.RegisterPlanArtifacts(ctx, []PlanArtifact{invalid}); !errors.Is(err, ErrPlan) {
		t.Fatalf("bad checksum registration = %v", err)
	}
	invalid = archiveFixture("other", "1", "rate 5%")
	if err := store.RegisterPlanArtifacts(ctx, []PlanArtifact{invalid}); !errors.Is(err, ErrPlan) {
		t.Fatalf("bad kind registration = %v", err)
	}
	invalid = archiveFixture("rule", "1", string(make([]byte, (1<<20)+1)))
	if err := store.RegisterPlanArtifacts(ctx, []PlanArtifact{invalid}); !errors.Is(err, ErrPlan) {
		t.Fatalf("oversized registration = %v", err)
	}
	if err := store.RegisterPlanArtifacts(ctx, []PlanArtifact{valid}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE transfer_plan_artifacts SET content = ?", []byte("tampered")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadPlanArtifact(ctx, valid.Kind, valid.Rule, valid.Version, valid.SHA256); !errors.Is(err, ErrAuditMismatch) {
		t.Fatalf("tampered archive lookup = %v", err)
	}
}

func TestReadPlannedTransferChecksStoredReceiptOnReadOnlyStore(t *testing.T) {
	ctx, store, path := testStore(t)
	req := Request{ID: "archived-receipt", From: "A", To: "B", Currency: "KRW", Amount: 10000}
	audit := []byte(`{"plan":"historical"}`)
	want, err := store.TransferPlanned(ctx, req, func() (Plan, error) {
		return Plan{Debit: 10500, Credit: 10000, Fee: 500, Audit: audit}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	readOnly, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer readOnly.Close()
	got, savedAudit, err := readOnly.ReadPlannedTransfer(ctx, req.ID)
	if err != nil || !reflect.DeepEqual(got, want) || !bytes.Equal(savedAudit, audit) {
		t.Fatalf("loaded %+v audit=%s, err=%v, want %+v", got, savedAudit, err, want)
	}
	if _, _, err := readOnly.ReadPlannedTransfer(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing receipt lookup = %v", err)
	}
	assertBalances(t, ctx, store, []string{"A", "B", "fee-KRW"}, 9500, 12000, 500)
	assertLedgerCounts(t, ctx, store, 1, 3)
	if _, err := store.db.ExecContext(ctx, "UPDATE postings SET delta = 499 WHERE kind = 'fee'"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readOnly.ReadPlannedTransfer(ctx, req.ID); !errors.Is(err, ErrAuditMismatch) {
		t.Fatalf("inconsistent posting error = %v", err)
	}
}
