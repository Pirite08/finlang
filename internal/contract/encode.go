package contract

import (
	"encoding/json"
	"errors"
	"io"
	"slices"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/decimal"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/eval"
)

type moneyResult struct {
	Currency string `json:"currency"`
	Amount   string `json:"amount"`
}

type resultRecord struct {
	SchemaVersion string `json:"schema_version"`
	Rule          string `json:"rule"`
	Status        string `json:"status"`
	Result        any    `json:"result"`
}

// EncodeResult performs every validation before writing a success record. It
// formats exactly; settlement and rounding must already have happened in source.
func EncodeResult(w io.Writer, spec Spec, result eval.Value) error {
	if spec.SchemaVersion == "2" {
		return encodeMapResult(w, spec, result)
	}
	if spec.SchemaVersion == "3" {
		return encodeRecordResult(w, spec, result)
	}
	value, ok := result.(eval.MoneyValue)
	if !ok || spec.Output.Type != "money" || !currency(spec.Output.Currency) || value.Value.Currency() != spec.Output.Currency {
		return contractError("E_CONTRACT_OUTPUT", "result must be Money in the output currency")
	}
	limits := config.DefaultLimits()
	lo, hi, err := bounds(spec.Output.Min, spec.Output.Max, limits)
	if err != nil {
		return contractError("E_CONTRACT", "invalid output bounds: %v", err)
	}
	formatted, err := formatResultAmount(value.Value.Amount(), amountSpec{lo: lo, hi: hi, scale: spec.Output.Scale}, limits)
	if err != nil {
		return err
	}
	record := resultRecord{
		SchemaVersion: "1", Rule: spec.Rule, Status: "ok",
		Result: moneyResult{Currency: spec.Output.Currency, Amount: formatted},
	}
	return json.NewEncoder(w).Encode(record)
}

func formatResultAmount(amount decimal.Decimal, spec amountSpec, limits config.Limits) (string, error) {
	if decimal.Cmp(amount, spec.lo) < 0 || decimal.Cmp(amount, spec.hi) > 0 {
		return "", contractError("E_LEGACY_RANGE", "result outside inclusive output range")
	}
	formatted, err := decimal.FormatAt(amount, spec.scale, limits)
	if err != nil {
		return "", contractError("E_OUTPUT_SCALE", "result cannot be represented exactly at output scale")
	}
	return formatted, nil
}

func encodeMapResult(w io.Writer, spec Spec, result eval.Value) error {
	output := spec.Output.currencyMap
	if spec.Output.Type != "map" || output == nil || output.maxItems < 1 {
		return contractError("E_CONTRACT_OUTPUT", "invalid currency map output contract")
	}
	entries, ok := eval.MapEntries(result)
	if !ok {
		return contractError("E_CONTRACT_OUTPUT", "result must be a currency map")
	}
	if maxItems := min(output.maxItems, config.MaxRuleCollectionItems); len(entries) > maxItems {
		return contractError("E_CONTRACT_OUTPUT", "result exceeds max_items %d", maxItems)
	}
	limits := config.DefaultLimits()
	values := make(map[string]moneyResult, len(entries))
	for _, entry := range entries {
		value, ok := entry.Value.(eval.MoneyValue)
		if !ok || entry.Key != value.Value.Currency() || !slices.Contains(output.currencies, entry.Key) {
			return contractError("E_CONTRACT_OUTPUT", "result entries must be Money keyed by an allowed currency matching the amount")
		}
		formatted, err := formatResultAmount(value.Value.Amount(), output.amount, limits)
		if err != nil {
			return err
		}
		values[entry.Key] = moneyResult{Currency: entry.Key, Amount: formatted}
	}
	record := resultRecord{SchemaVersion: "2", Rule: spec.Rule, Status: "ok", Result: values}
	return json.NewEncoder(w).Encode(record)
}

// EncodeError writes one error record, retaining source locations when present.
// Encoder/writer errors are returned unchanged to the host CLI.
func EncodeError(w io.Writer, rule string, err error) error {
	return EncodeErrorVersion(w, "1", rule, err)
}

// EncodeErrorVersion uses the identified contract schema for the error envelope.
// Errors before contract validation use schema 1.
func EncodeErrorVersion(w io.Writer, version, rule string, err error) error {
	if !supportedSchema(version) {
		version = "1"
	}
	var typed *diagnostic.Error
	record := struct {
		SchemaVersion string `json:"schema_version"`
		Rule          string `json:"rule"`
		Status        string `json:"status"`
		Error         struct {
			Code    diagnostic.Code `json:"code"`
			Message string          `json:"message"`
			File    *string         `json:"file"`
			Line    *int            `json:"line"`
			Column  *int            `json:"column"`
		} `json:"error"`
	}{SchemaVersion: version, Rule: rule, Status: "error"}
	record.Error.Code = diagnostic.CodeOf(err)
	if err != nil {
		record.Error.Message = err.Error()
	}
	if errors.As(err, &typed) {
		record.Error.Message = typed.Message
		if typed.Span != nil {
			record.Error.File = &typed.Span.File
			record.Error.Line = &typed.Span.Line
			record.Error.Column = &typed.Span.Column
		}
	}
	return json.NewEncoder(w).Encode(record)
}
