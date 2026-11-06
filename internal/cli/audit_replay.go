package cli

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"io"
	"os"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/fx"
	"github.com/Pirite08/finlang/internal/trace"
)

func loadAuditReplay(path string, limits config.Limits) (trace.Header, error) {
	file, err := os.Open(path)
	if err != nil {
		return trace.Header{}, diagnostic.New("E_INPUT_IO", "could not open audit record")
	}
	defer file.Close()

	// Embedded input may expand when JSON-escaped. The remaining space holds
	// the execution metadata, result, and snapshots of both FX directions.
	maxBytes := int64(limits.MaxSourceBytes)*6 + (64 << 10)
	line, err := bufio.NewReader(io.LimitReader(file, maxBytes+1)).ReadBytes('\n')
	if err != nil && err != io.EOF {
		return trace.Header{}, diagnostic.New("E_INPUT_IO", "could not read audit header")
	}
	if int64(len(line)) > maxBytes {
		return trace.Header{}, diagnostic.New("E_AUDIT_REPLAY", "audit header exceeds byte limit")
	}

	var record struct {
		Record string `json:"record"`
		trace.Header
	}
	if err := jsonv2.Unmarshal(line, &record, jsonv2.RejectUnknownMembers(true)); err != nil || record.Record != "header" {
		return trace.Header{}, diagnostic.New("E_AUDIT_REPLAY", "invalid audit header")
	}
	if record.SchemaVersion != "1" || record.ContractVersion != "1" || record.RunnerVersion != "fin "+Version ||
		record.Limits != limits || record.FXPolicy == nil || record.FXPolicy.Version != fx.PolicyVersion ||
		(record.ExecutionProfile != "" && record.ExecutionProfile != ruleOnlyProfile) {
		return trace.Header{}, diagnostic.New("E_AUDIT_REPLAY", "unsupported audit versions or execution limits")
	}
	if record.RunID == "" || record.Rule == "" || !validSHA256(record.ProgramSHA256) || !validSHA256(record.ContractSHA256) ||
		!isAuditObject(record.Input) || record.ErrorCode != "" || !isAuditObject(record.Result) {
		return trace.Header{}, diagnostic.New("E_AUDIT_REPLAY", "audit header lacks successful execution data")
	}
	var outcome struct {
		Status string          `json:"status"`
		Result json.RawMessage `json:"result"`
	}
	if err := jsonv2.Unmarshal(record.Result, &outcome); err != nil || outcome.Status != "ok" || !isAuditObject(outcome.Result) {
		return trace.Header{}, diagnostic.New("E_AUDIT_REPLAY", "audit record does not contain a successful result")
	}
	return record.Header, nil
}

func validSHA256(text string) bool {
	if len(text) != hex.EncodedLen(sha256.Size) {
		return false
	}
	_, err := hex.DecodeString(text)
	return err == nil
}

func isAuditObject(data json.RawMessage) bool {
	data = bytes.TrimSpace(data)
	return len(data) != 0 && data[0] == '{'
}

func verifyAuditReplayResult(want json.RawMessage, actual []byte) error {
	var expected, got bytes.Buffer
	if json.Compact(&expected, want) != nil || json.Compact(&got, actual) != nil || !bytes.Equal(expected.Bytes(), got.Bytes()) {
		return diagnostic.New("E_AUDIT_REPLAY", "replayed result differs from the audit record")
	}
	return nil
}
