package contract

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/decimal"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/eval"
)

// EncodeResult performs every validation before writing a success record. It
// formats exactly; settlement and rounding must already have happened in source.
func EncodeResult(w io.Writer, spec Spec, result eval.Value) error {
	value, ok := result.(eval.MoneyValue)
	if !ok || spec.Output.Type != "money" || !currency(spec.Output.Currency) || value.Value.Currency() != spec.Output.Currency {
		return contractError("E_CONTRACT_OUTPUT", "result must be Money in the output currency")
	}
	limits := config.DefaultLimits()
	lo, hi, err := bounds(spec.Output.Min, spec.Output.Max, limits)
	if err != nil {
		return contractError("E_CONTRACT", "invalid output bounds: %v", err)
	}
	amount := value.Value.Amount()
	if decimal.Cmp(amount, lo) < 0 || decimal.Cmp(amount, hi) > 0 {
		return contractError("E_LEGACY_RANGE", "result outside inclusive output range")
	}
	formatted, err := decimal.FormatAt(amount, spec.Output.Scale, limits)
	if err != nil {
		return contractError("E_OUTPUT_SCALE", "result cannot be represented exactly at output scale")
	}
	record := struct {
		SchemaVersion string `json:"schema_version"`
		Rule          string `json:"rule"`
		Status        string `json:"status"`
		Result        struct {
			Currency string `json:"currency"`
			Amount   string `json:"amount"`
		} `json:"result"`
	}{SchemaVersion: "1", Rule: spec.Rule, Status: "ok"}
	record.Result.Currency = spec.Output.Currency
	record.Result.Amount = formatted
	return json.NewEncoder(w).Encode(record)
}

// EncodeError writes one error record, retaining source locations when present.
// Encoder/writer errors are returned unchanged to the host CLI.
func EncodeError(w io.Writer, rule string, err error) error {
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
	}{SchemaVersion: "1", Rule: rule, Status: "error"}
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
