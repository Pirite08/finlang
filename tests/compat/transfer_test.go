package compat

import (
	"bufio"
	"bytes"
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/transfer"
)

type transferCase struct {
	ID      string           `json:"id"`
	Request transfer.Request `json:"request"`
	Initial transferBalances `json:"initial"`
	Want    transferOutcome  `json:"expected"`
}

type transferBalances struct {
	From int64 `json:"from_balance_minor"`
	To   int64 `json:"to_balance_minor"`
	Fee  int64 `json:"fee_balance_minor"`
}

type transferPosting struct {
	Kind     string `json:"kind"`
	Account  string `json:"account"`
	Currency string `json:"currency"`
	Delta    int64  `json:"delta_minor"`
}

type transferOutcome struct {
	Status   string            `json:"status"`
	Currency string            `json:"currency"`
	Fee      int64             `json:"fee_minor"`
	Debit    int64             `json:"debit_minor"`
	Final    transferBalances  `json:"final"`
	Postings []transferPosting `json:"postings"`
}

func TestCOBOLTransferCompatibility(t *testing.T) {
	compiler := findCOBOLCompiler(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	const source = "testdata/cobol/transfer_v1.cob"
	binary := filepath.Join(t.TempDir(), "transfer_v1")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	commands := []commandRecord{
		{Phase: "version", Executable: compiler, Arguments: []string{"--version"}, WorkingDirectory: root, TimeoutSeconds: 10},
		{Phase: "compile", Rule: "transfer_v1", Executable: compiler, WorkingDirectory: root, TimeoutSeconds: 60,
			Arguments: []string{"-x", "-free", "-std=cobol2014", "-freserved=NEAREST-EVEN", "-o", binary, filepath.FromSlash(source)}},
		{Phase: "run", Rule: "transfer_v1", Executable: binary, WorkingDirectory: root, TimeoutSeconds: 30},
	}
	version, err := runCommand(commands[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	evidence := newEvidence(t, "FINLANG_TRANSFER_COMPAT_REPORT")
	if _, err := runCommand(commands[1], nil); err != nil {
		evidence.write(t, map[string]any{"record": "infrastructure_error", "phase": "compile", "error": err.Error()})
		t.Fatal(err)
	}
	var files []fileHash
	for _, path := range []string{
		source, "internal/transfer/transfer.go", "internal/transfer/store.go",
		"internal/transferfee/fee.go", "internal/transferfee/rules/krw.fin",
		"internal/transferfee/rules/krw.json", "internal/transferfee/rules/usd.fin",
		"internal/transferfee/rules/usd.json", "tests/compat/transfer_test.go",
		"tests/compat/transfer_cases_test.go", "tests/compat/transfer_backend_test.go",
	} {
		files = append(files, hashFile(t, root, path))
	}
	evidence.write(t, map[string]any{
		"record": "header", "schema_version": "1", "rule": "transfer_v1",
		"go_version": runtime.Version(), "goos": runtime.GOOS, "goarch": runtime.GOARCH,
		"compiler": compiler, "compiler_version": strings.TrimSpace(string(version)),
		"compiler_sha256": hashFile(t, filepath.Dir(compiler), filepath.Base(compiler)).SHA256,
		"commands":        commands, "files": files, "environment": compatibilityEnvironment(),
		"units": map[string]string{"KRW": "won", "USD": "cent"},
	})

	cases := transferCases()
	if len(cases) == 0 {
		t.Fatal("no transfer fixtures")
	}
	var batch bytes.Buffer
	fmt.Fprintln(&batch, len(cases))
	for _, tc := range cases {
		req, initial := tc.Request, tc.Initial
		fmt.Fprintf(&batch, "%s|%s|%s|%s|%d|%d|%d|%d\n",
			tc.ID, req.Currency, req.From, req.To, req.Amount, initial.From, initial.To, initial.Fee)
	}
	output, err := runCommand(commands[2], batch.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	reference, err := parseCOBOLTransfers(output, cases)
	if err != nil {
		t.Fatal(err)
	}
	matched := 0
	statuses := make(map[string]int)
	for i, tc := range cases {
		if t.Run(tc.ID, func(t *testing.T) {
			cobol := reference[i]
			backend := runTransferBackend(t, tc)
			equal := reflect.DeepEqual(backend, cobol)
			cobolAnchor := reflect.DeepEqual(cobol, tc.Want)
			backendAnchor := reflect.DeepEqual(backend, tc.Want)
			evidence.write(t, map[string]any{
				"record": "case", "fixture": tc, "cobol": cobol, "backend": backend,
				"match": equal, "cobol_anchor": cobolAnchor, "backend_anchor": backendAnchor,
			})
			if !equal || !cobolAnchor || !backendAnchor {
				t.Fatalf("transfer mismatch\nCOBOL:   %+v\nbackend: %+v\nwant:    %+v", cobol, backend, tc.Want)
			}
		}) {
			matched++
			statuses[tc.Want.Status]++
		}
	}
	evidence.write(t, map[string]any{
		"record": "total_summary", "rule": "transfer_v1", "cases": len(cases),
		"matched": matched, "mismatched": len(cases) - matched, "matched_statuses": statuses,
	})
	t.Logf("transfers: cases=%d matched=%d mismatched=%d statuses=%v", len(cases), matched, len(cases)-matched, statuses)
}

func parseCOBOLTransfers(data []byte, cases []transferCase) ([]transferOutcome, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	results := make([]transferOutcome, 0, len(cases))
	for _, tc := range cases {
		fields, err := readTransferLine(scanner, tc.ID, 8)
		if err != nil {
			return nil, err
		}
		result := transferOutcome{Status: fields[1], Currency: fields[2]}
		if result.Status != "ok" && result.Status != "insufficient_funds" && result.Status != "invalid" {
			return nil, fmt.Errorf("%s: unexpected COBOL status %q", tc.ID, result.Status)
		}
		amounts := []*int64{&result.Fee, &result.Debit, &result.Final.From, &result.Final.To, &result.Final.Fee}
		for i, amount := range amounts {
			*amount, err = parseMinorUnits(fields[i+3])
			if err != nil {
				return nil, fmt.Errorf("%s: %w", tc.ID, err)
			}
		}
		if result.Status == "ok" {
			for _, kind := range []string{"debit", "credit", "fee"} {
				posting, err := readTransferLine(scanner, tc.ID, 5)
				if err != nil {
					return nil, err
				}
				if posting[1] != kind {
					return nil, fmt.Errorf("%s: expected posting %s, got %q", tc.ID, kind, posting[1])
				}
				delta, err := parseMinorUnits(posting[4])
				if err != nil {
					return nil, fmt.Errorf("%s %s: %w", tc.ID, kind, err)
				}
				result.Postings = append(result.Postings, transferPosting{
					Kind: kind, Account: posting[2], Currency: posting[3], Delta: delta,
				})
			}
		}
		results = append(results, result)
	}
	if scanner.Scan() {
		return nil, fmt.Errorf("unexpected extra COBOL transfer output %q", scanner.Text())
	}
	return results, scanner.Err()
}

func readTransferLine(scanner *bufio.Scanner, id string, fields int) ([]string, error) {
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%s: incomplete COBOL transfer output", id)
	}
	parts := strings.Split(scanner.Text(), "|")
	if len(parts) != fields || parts[0] != id {
		return nil, fmt.Errorf("%s: unexpected COBOL transfer record %q", id, scanner.Text())
	}
	return parts, nil
}

func parseMinorUnits(text string) (int64, error) {
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil || strconv.FormatInt(value, 10) != text {
		return 0, fmt.Errorf("invalid integer minor units %q", text)
	}
	return value, nil
}
