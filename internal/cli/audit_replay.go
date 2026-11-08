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
	"strings"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/decimal"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/fx"
	"github.com/Pirite08/finlang/internal/money"
	"github.com/Pirite08/finlang/internal/trace"
)

func loadAuditReplay(path string, limits config.Limits) (trace.Header, error) {
	file, err := os.Open(path)
	if err != nil {
		return trace.Header{}, diagnostic.New("E_INPUT_IO", "could not open audit record")
	}
	defer file.Close()

	// Legacy embedded input may expand when JSON-escaped. The remaining space holds
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
	if record.SchemaVersion != "1" || record.RunnerVersion != "fin "+Version ||
		record.FXPolicy == nil || record.FXPolicy.Version != fx.PolicyVersion ||
		!auditExecutionMatches(record.Header, limits) {
		return trace.Header{}, diagnostic.New("E_AUDIT_REPLAY", "unsupported audit versions or execution limits")
	}
	if record.RunID == "" || record.Rule == "" || !validSHA256(record.ProgramSHA256) || !validSHA256(record.ContractSHA256) ||
		!isAuditObject(record.Input) || record.ErrorCode != "" || !isAuditObject(record.Result) {
		return trace.Header{}, diagnostic.New("E_AUDIT_REPLAY", "audit header lacks successful execution data")
	}
	if record.ContractVersion == "2" {
		if len(record.Input) > limits.MaxSourceBytes || !validCollectionAuditResult(record.Header) {
			return trace.Header{}, diagnostic.New("E_AUDIT_REPLAY", "invalid collection audit input or result")
		}
	} else {
		var outcome struct {
			Status string          `json:"status"`
			Result json.RawMessage `json:"result"`
		}
		if err := jsonv2.Unmarshal(record.Result, &outcome); err != nil || outcome.Status != "ok" || !isAuditObject(outcome.Result) {
			return trace.Header{}, diagnostic.New("E_AUDIT_REPLAY", "audit record does not contain a successful result")
		}
	}
	return record.Header, nil
}

func auditExecutionMatches(header trace.Header, limits config.Limits) bool {
	switch header.ContractVersion {
	case "1":
		return header.Limits == limits && (header.ExecutionProfile == "" || header.ExecutionProfile == ruleOnlyProfile)
	case "2":
		return header.Limits == config.RuleCollectionLimits(limits) && header.ExecutionProfile == ruleCollectionProfile &&
			validCollectionAuditFX(header)
	default:
		return false
	}
}

func validCollectionAuditFX(header trace.Header) bool {
	policy := header.FXPolicy
	if policy.MaxAge != "" || policy.MinRate != "" || policy.MaxRate != "" {
		return false
	}
	switch policy.Mode {
	case "none":
		return len(header.FXQuotes) == 0
	case "fixed":
		return len(header.FXQuotes) <= 1
	case "snapshot", "replay":
		return true
	default:
		return false
	}
}

func validCollectionAuditResult(header trace.Header) bool {
	var result struct {
		SchemaVersion string `json:"schema_version"`
		Rule          string `json:"rule"`
		Status        string `json:"status"`
		Result        map[string]struct {
			Currency string `json:"currency"`
			Amount   string `json:"amount"`
		} `json:"result"`
	}
	if err := jsonv2.Unmarshal(header.Result, &result, jsonv2.RejectUnknownMembers(true)); err != nil ||
		result.SchemaVersion != "2" || result.Rule != header.Rule || result.Status != "ok" ||
		result.Result == nil || len(result.Result) > header.Limits.MaxCollectionItems {
		return false
	}
	for code, value := range result.Result {
		if code != value.Currency {
			return false
		}
		amount, err := decimal.Parse(value.Amount, header.Limits)
		if err != nil {
			return false
		}
		if _, err := money.New(code, amount); err != nil {
			return false
		}
		_, fraction, _ := strings.Cut(value.Amount, ".")
		canonical, err := decimal.FormatAt(amount, uint32(len(fraction)), header.Limits)
		if err != nil || canonical != value.Amount {
			return false
		}
	}
	return true
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
