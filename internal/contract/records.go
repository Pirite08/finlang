package contract

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/eval"
)

// MatchesRecordShape reports whether a host adapter can bind the contract's
// single Record input and its Record output to the specified field shapes.
func (s Spec) MatchesRecordShape(parameter string, input, output map[string]string, currency string, scale uint32) bool {
	if s.SchemaVersion != "3" || len(s.Parameters) != 1 || s.Output.Type != "record" {
		return false
	}
	p := s.Parameters[0]
	return p.Name == parameter && p.Type == "record" &&
		matchesRecordFields(p.record, input, currency, scale) &&
		matchesRecordFields(s.Output.record, output, currency, scale)
}

func matchesRecordFields(fields []fieldSpec, expected map[string]string, currency string, scale uint32) bool {
	if len(fields) != len(expected) {
		return false
	}
	for _, field := range fields {
		if expected[field.name] != field.kind {
			return false
		}
		if field.kind == "money" && (field.amount.scale != scale || len(field.currencies) != 1 || field.currencies[0] != currency) {
			return false
		}
	}
	return true
}

func loadRecordParameter(value any, limits config.Limits) (Parameter, error) {
	fields, err := object(value, []string{"name", "type", "fields"}, nil)
	if err != nil {
		return Parameter{}, err
	}
	name, err := stringField(fields, "name")
	if err != nil || !identifier(name) || len(name) > limits.MaxTokenBytes {
		return Parameter{}, fmt.Errorf("invalid parameter name %q", name)
	}
	record, err := loadRecordFields(fields["fields"], limits, false)
	if err != nil {
		return Parameter{}, err
	}
	return Parameter{Name: name, Type: "record", record: record}, nil
}

func loadRecordOutput(value any, limits config.Limits) (OutputSpec, error) {
	fields, err := object(value, []string{"type", "fields"}, nil)
	if err != nil {
		return OutputSpec{}, err
	}
	if fields["type"] != "record" {
		return OutputSpec{}, fmt.Errorf("output must have type record")
	}
	record, err := loadRecordFields(fields["fields"], limits, true)
	if err != nil {
		return OutputSpec{}, err
	}
	return OutputSpec{Type: "record", record: record}, nil
}

func encodeRecordResult(w io.Writer, spec Spec, result eval.Value) error {
	output := spec.Output.record
	fields, ok := eval.RecordFields(result)
	if spec.Output.Type != "record" || len(output) == 0 || len(output) > config.MaxRuleCollectionItems ||
		!ok || len(fields) != len(output) {
		return contractError("E_CONTRACT_OUTPUT", "result must be a record with exactly the contract fields")
	}
	metadata := make(map[string]fieldSpec, len(output))
	for _, field := range output {
		metadata[field.name] = field
	}
	limits := config.DefaultLimits()
	values := make(map[string]moneyResult, len(fields))
	for _, field := range fields {
		expected, known := metadata[field.Name]
		if !known {
			return contractError("E_CONTRACT_OUTPUT", "unexpected or duplicate result field %q", field.Name)
		}
		delete(metadata, field.Name)
		value, ok := field.Value.(eval.MoneyValue)
		if !ok || !slices.Contains(expected.currencies, value.Value.Currency()) {
			return contractError("E_CONTRACT_OUTPUT", "result field %q must be Money in an allowed currency", field.Name)
		}
		formatted, err := formatResultAmount(value.Value.Amount(), expected.amount, limits)
		if err != nil {
			return fmt.Errorf("result field %q: %w", field.Name, err)
		}
		values[field.Name] = moneyResult{Currency: value.Value.Currency(), Amount: formatted}
	}
	record := resultRecord{SchemaVersion: "3", Rule: spec.Rule, Status: "ok", Result: values}
	return json.NewEncoder(w).Encode(record)
}
