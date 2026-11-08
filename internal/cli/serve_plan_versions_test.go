package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServePlannedTransferReplaysHistoricalVersionsReadOnly(t *testing.T) {
	dir := t.TempDir()
	ledger := filepath.Join(dir, "ledger.sqlite")
	manifestV1, ruleV1, contractV1 := copyTransferVersionFixture(t, dir)
	address, stop := startTransferVersionServer(t, manifestV1, ledger, true)
	withTransferVersionDB(t, ledger, func(db *sql.DB) {
		if _, err := db.ExecContext(t.Context(), "UPDATE accounts SET balance = 25000 WHERE id = 'A'"); err != nil {
			t.Fatal(err)
		}
	})
	client := &http.Client{Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()
	old := postVersionedTransfer(client, address, "old-policy")
	old.check(t, http.StatusOK, `"delta_minor":"-10500"`, `"delta_minor":"500"`)
	stop()
	for _, path := range []string{manifestV1, ruleV1, contractV1} {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}

	address, stop = startTransferVersionServer(t, "../../examples/transfer_app_v2.json", ledger, false)
	postVersionedTransfer(client, address, "new-policy").check(t, http.StatusOK, `"delta_minor":"-11000"`, `"delta_minor":"1000"`)
	retry := postVersionedTransfer(client, address, "old-policy")
	retry.check(t, http.StatusOK)
	if retry.body != old.body {
		t.Fatalf("retry after policy deployment changed the original receipt: %s", retry.body)
	}
	stop()

	var oldAudit, newAudit transferVersionAudit
	withTransferVersionDB(t, ledger, func(db *sql.DB) {
		assertPlannedDB(t, db, [3]int64{3500, 22000, 1500}, 2, 6)
		oldAudit = loadTransferVersionAudit(t, db, "old-policy")
		newAudit = loadTransferVersionAudit(t, db, "new-policy")
	})
	oldAudit.check(t, "1")
	newAudit.check(t, "2")
	if oldAudit.ProgramSHA256 == newAudit.ProgramSHA256 || oldAudit.ContractSHA256 == newAudit.ContractSHA256 {
		t.Fatal("the two deployments did not archive distinct rule and contract bytes")
	}
	beforeReplay := transferLedgerHash(t, ledger)
	var oldReplay string
	for _, tc := range []struct{ id, version, fee, debit string }{
		{"old-policy", "1", "500", "10500"},
		{"new-policy", "2", "1000", "11000"},
	} {
		code, stdout, stderr := replayTransferVersion(ledger, tc.id)
		if code != 0 || stderr != "" {
			t.Fatalf("replay %s: code=%d stdout=%s stderr=%s", tc.id, code, stdout, stderr)
		}
		if tc.id == "old-policy" {
			oldReplay = stdout
		}
		var replay struct {
			Status                  string                       `json:"status"`
			RequestID               string                       `json:"request_id"`
			Rule                    string                       `json:"rule"`
			RuleVersion             string                       `json:"rule_version"`
			ContractArtifactVersion string                       `json:"contract_artifact_version"`
			Result                  map[string]map[string]string `json:"result"`
		}
		if err := json.Unmarshal([]byte(stdout), &replay); err != nil {
			t.Fatalf("invalid replay response %s: %v", stdout, err)
		}
		if replay.Status != "ok" || replay.RequestID != tc.id || replay.Rule != "transfer_plan_krw_v1" ||
			replay.RuleVersion != tc.version || replay.ContractArtifactVersion != tc.version || len(replay.Result) != 3 {
			t.Fatalf("unexpected replay metadata for %s: %s", tc.id, stdout)
		}
		for field, amount := range map[string]string{"debit": tc.debit, "credit": "10000", "fee": tc.fee} {
			if got := replay.Result[field]; got["amount"] != amount || got["currency"] != "KRW" {
				t.Fatalf("replay %s %s=%v, want %s KRW", tc.id, field, got, amount)
			}
		}
	}
	if afterReplay := transferLedgerHash(t, ledger); afterReplay != beforeReplay {
		t.Fatal("successful replay modified the SQLite file")
	}
	withTransferVersionDB(t, ledger, func(db *sql.DB) {
		assertPlannedDB(t, db, [3]int64{3500, 22000, 1500}, 2, 6)
		var audit []byte
		if err := db.QueryRowContext(t.Context(), "SELECT audit FROM transfers WHERE request_id = 'old-policy'").Scan(&audit); err != nil {
			t.Fatal(err)
		}
		var legacy map[string]json.RawMessage
		if err := json.Unmarshal(audit, &legacy); err != nil {
			t.Fatal(err)
		}
		legacy["schema_version"] = json.RawMessage(`"1"`)
		delete(legacy, "rule_version")
		delete(legacy, "contract_artifact_version")
		encoded, err := json.Marshal(legacy)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(), "UPDATE transfers SET audit = ? WHERE request_id = 'old-policy'", encoded); err != nil {
			t.Fatal(err)
		}
	})
	beforeLegacyReplay := transferLedgerHash(t, ledger)
	if code, stdout, stderr := replayTransferVersion(ledger, "old-policy"); code != 0 || stdout != oldReplay || stderr != "" {
		t.Fatalf("legacy replay by digest: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if transferLedgerHash(t, ledger) != beforeLegacyReplay {
		t.Fatal("legacy replay modified the SQLite file")
	}
	withTransferVersionDB(t, ledger, func(db *sql.DB) {
		var audit []byte
		if err := db.QueryRowContext(t.Context(), "SELECT audit FROM transfers WHERE request_id = 'old-policy'").Scan(&audit); err != nil {
			t.Fatal(err)
		}
		tampered := bytes.Replace(audit, []byte(`"amount":"500"`), []byte(`"amount":"501"`), 1)
		if bytes.Equal(tampered, audit) {
			t.Fatal("recorded audit has no fee to tamper with")
		}
		if _, err := db.ExecContext(t.Context(), "UPDATE transfers SET audit = ? WHERE request_id = 'old-policy'", tampered); err != nil {
			t.Fatal(err)
		}
	})
	beforeTamperedReplay := transferLedgerHash(t, ledger)
	if code, stdout, stderr := replayTransferVersion(ledger, "old-policy"); code == 0 || stdout != "" || !strings.Contains(stderr, "E_AUDIT_REPLAY") {
		t.Fatalf("tampered replay: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if transferLedgerHash(t, ledger) != beforeTamperedReplay {
		t.Fatal("failed replay modified the SQLite file")
	}
}

type transferVersionAudit struct {
	Record                  string `json:"record"`
	SchemaVersion           string `json:"schema_version"`
	RuleVersion             string `json:"rule_version"`
	ContractArtifactVersion string `json:"contract_artifact_version"`
	ProgramSHA256           string `json:"program_sha256"`
	ContractSHA256          string `json:"contract_sha256"`
}

func TestServePlannedTransferRetriesUnderCommittedContract(t *testing.T) {
	dir := t.TempDir()
	ledger := filepath.Join(dir, "ledger.sqlite")
	manifestV1, _, _ := copyTransferVersionFixture(t, dir)
	address, stop := startTransferVersionServer(t, manifestV1, ledger, true)
	client := &http.Client{Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()
	first := postVersionedTransfer(client, address, "historical-retry")
	first.check(t, http.StatusOK, `"delta_minor":"500"`)
	stop()

	const exampleV2 = "../../examples/transfer_app_v2.json"
	data, err := os.ReadFile(exampleV2)
	if err != nil {
		t.Fatal(err)
	}
	var manifest serveManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	route := &manifest.Routes[0]
	route.Rule, err = filepath.Abs(filepath.Join(filepath.Dir(exampleV2), route.Rule))
	if err != nil {
		t.Fatal(err)
	}
	contractData, err := os.ReadFile(filepath.Join(filepath.Dir(exampleV2), route.Contract))
	if err != nil {
		t.Fatal(err)
	}
	narrowed := strings.Replace(string(contractData), `"max": "8384883669867978006"`, `"max": "9999"`, 1)
	if narrowed == string(contractData) {
		t.Fatal("failed to narrow test contract")
	}
	route.Contract = filepath.Join(dir, "v2.contract.json")
	writeServeTestFile(t, route.Contract, narrowed)
	data, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestV2 := filepath.Join(dir, "v2.app.json")
	writeServeTestFile(t, manifestV2, string(data))
	address, stop = startTransferVersionServer(t, manifestV2, ledger, false)

	retry := postVersionedTransfer(client, address, "historical-retry")
	retry.check(t, http.StatusOK)
	if retry.body != first.body {
		t.Fatalf("historical retry changed the receipt: %s", retry.body)
	}
	postVersionedTransfer(client, address, "new-invalid").check(t, http.StatusBadRequest, "E_CONTRACT_INPUT")
	const changed = `{"schema_version":"3","rule":"transfer_plan_krw_v1","inputs":{"transfer":{"request_id":"historical-retry","from_account":"A","to_account":"B","amount":{"currency":"KRW","amount":"10001"}}}}`
	postTransferVersionBody(client, address, changed).check(t, http.StatusConflict, "E_REQUEST_CONFLICT")
	const duplicate = `{"schema_version":"3","rule":"transfer_plan_krw_v1","inputs":{"transfer":{"request_id":"historical-retry","from_account":"A","to_account":"B","amount":{"currency":"KRW","amount":"10000","amount":"10000"}}}}`
	postTransferVersionBody(client, address, duplicate).check(t, http.StatusBadRequest, "E_CONTRACT_INPUT")
	stop()
	withTransferVersionDB(t, ledger, func(db *sql.DB) {
		assertPlannedDB(t, db, [3]int64{9500, 12000, 500}, 1, 3)
	})
}

func (audit transferVersionAudit) check(t *testing.T, version string) {
	t.Helper()
	if audit.Record != "transfer_plan" || audit.SchemaVersion != "2" ||
		audit.RuleVersion != version || audit.ContractArtifactVersion != version ||
		!validSHA256(audit.ProgramSHA256) || !validSHA256(audit.ContractSHA256) {
		t.Fatalf("unexpected transfer audit for revision %s: %+v", version, audit)
	}
}

func loadTransferVersionAudit(t *testing.T, db *sql.DB, id string) transferVersionAudit {
	t.Helper()
	var encoded []byte
	if err := db.QueryRowContext(t.Context(), "SELECT audit FROM transfers WHERE request_id = ?", id).Scan(&encoded); err != nil {
		t.Fatal(err)
	}
	var audit transferVersionAudit
	if err := json.Unmarshal(encoded, &audit); err != nil {
		t.Fatal(err)
	}
	return audit
}

func copyTransferVersionFixture(t *testing.T, dir string) (manifestPath, rulePath, contractPath string) {
	t.Helper()
	const example = "../../examples/transfer_app.json"
	encoded, err := os.ReadFile(example)
	if err != nil {
		t.Fatal(err)
	}
	var manifest serveManifest
	if err := json.Unmarshal(encoded, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, route := range manifest.Routes {
		if route.Path != "/transfers" {
			continue
		}
		copyAsset := func(name, target string) string {
			asset, err := os.ReadFile(filepath.Join(filepath.Dir(example), name))
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, target)
			if err := os.WriteFile(path, asset, 0o600); err != nil {
				t.Fatal(err)
			}
			return path
		}
		rulePath = copyAsset(route.Rule, "v1.fin")
		contractPath = copyAsset(route.Contract, "v1.contract.json")
		route.Rule = rulePath
		route.Contract = contractPath
		manifest.Routes = []serveRouteSpec{route}
		manifest.Preview = ""
		encoded, err = json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		manifestPath = filepath.Join(dir, "v1.app.json")
		if err := os.WriteFile(manifestPath, encoded, 0o600); err != nil {
			t.Fatal(err)
		}
		return manifestPath, rulePath, contractPath
	}
	t.Fatal("v1 fixture has no transfer route")
	return "", "", ""
}

func startTransferVersionServer(t *testing.T, manifest, ledger string, seed bool) (string, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	log := make(serveStartup, 4)
	done := make(chan struct{})
	var code int
	args := []string{"-app", manifest, "-db", ledger, "-port", "0"}
	if seed {
		args = append(args, "-seed-demo")
	}
	go func() {
		defer close(done)
		code = runServe(ctx, args, log)
	}()
	stop := func() {
		cancel()
		select {
		case <-done:
			if code != 0 {
				t.Errorf("serve shutdown exit=%d", code)
			}
		case <-time.After(10 * time.Second):
			t.Error("serve did not stop")
		}
	}
	t.Cleanup(stop)
	const prefix = "fin serve listening on "
	select {
	case line := <-log:
		if !strings.HasPrefix(line, prefix+"http://127.0.0.1:") {
			t.Fatalf("expected loopback listener, got %q", line)
		}
		return strings.TrimSpace(strings.TrimPrefix(line, prefix)), stop
	case <-done:
		select {
		case line := <-log:
			t.Fatalf("serve exited before listening: %s", line)
		default:
			t.Fatalf("serve exited before listening, code=%d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not start")
	}
	return "", nil
}

func postVersionedTransfer(client *http.Client, address, id string) plannedResponse {
	body := fmt.Sprintf(`{"schema_version":"3","rule":"transfer_plan_krw_v1","inputs":{"transfer":{"request_id":%q,"from_account":"A","to_account":"B","amount":{"currency":"KRW","amount":"10000"}}}}`, id)
	return postTransferVersionBody(client, address, body)
}

func postTransferVersionBody(client *http.Client, address, body string) plannedResponse {
	response, err := client.Post(address+"/transfers", "application/json", strings.NewReader(body))
	if err != nil {
		return plannedResponse{err: err}
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	return plannedResponse{status: response.StatusCode, body: string(data), err: err}
}

func withTransferVersionDB(t *testing.T, path string, check func(*sql.DB)) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	check(db)
}

func transferLedgerHash(t *testing.T, path string) [sha256.Size]byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(data)
}

func replayTransferVersion(ledger, id string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"replay-transfer", "-db", ledger, "-id", id}, strings.NewReader(""), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}
