package contract

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/decimal"
	"github.com/Pirite08/finlang/internal/eval"
	"github.com/Pirite08/finlang/internal/money"
)

// DecodeInput validates all inputs and returns arguments in contract order.
// It does not evaluate a program or call the contract entry function.
func DecodeInput(r io.Reader, spec Spec, limits config.Limits) ([]eval.Value, error) {
	maxItems := 0
	if spec.SchemaVersion == "2" || spec.SchemaVersion == "3" {
		maxItems = collectionItemLimit(limits)
	}
	value, err := readBoundedJSON(r, limits, maxItems)
	if err != nil {
		return nil, contractError("E_CONTRACT_INPUT", "%v", err)
	}
	root, err := object(value, []string{"schema_version", "rule", "inputs"}, nil)
	if err != nil {
		return nil, contractError("E_CONTRACT_INPUT", "%v", err)
	}
	version, err := stringField(root, "schema_version")
	if err != nil || !supportedSchema(version) || version != spec.SchemaVersion {
		return nil, contractError("E_CONTRACT_INPUT", "schema version does not match contract")
	}
	rule, err := stringField(root, "rule")
	if err != nil || rule != spec.Rule {
		return nil, contractError("E_CONTRACT_INPUT", "rule does not match contract")
	}
	if version != "1" && len(spec.Parameters) > collectionItemLimit(limits) {
		return nil, contractError("E_CONTRACT_INPUT", "too many parameters")
	}
	names := make([]string, len(spec.Parameters))
	for i, p := range spec.Parameters {
		names[i] = p.Name
	}
	inputs, err := object(root["inputs"], names, nil)
	if err != nil {
		return nil, contractError("E_CONTRACT_INPUT", "inputs: %v", err)
	}
	values := make([]eval.Value, 0, len(spec.Parameters))
	for _, p := range spec.Parameters {
		var text string
		switch p.Type {
		case "list":
			if version != "2" {
				return nil, contractError("E_CONTRACT_INPUT", "%s: unsupported parameter type", p.Name)
			}
			list, err := decodeListInput(inputs[p.Name], p.list, limits)
			if err != nil {
				return nil, contractError("E_CONTRACT_INPUT", "%s: %v", p.Name, err)
			}
			values = append(values, list)
			continue
		case "record":
			if version != "3" || len(p.record) == 0 || len(p.record) > collectionItemLimit(limits) {
				return nil, contractError("E_CONTRACT_INPUT", "%s: invalid record contract", p.Name)
			}
			record, err := decodeRecordInput(inputs[p.Name], p.record, limits, true)
			if err != nil {
				return nil, contractError("E_CONTRACT_INPUT", "%s: %v", p.Name, err)
			}
			values = append(values, record)
			continue
		case "decimal":
			text, err = stringField(inputs, p.Name)
		case "money":
			var fields map[string]any
			fields, err = object(inputs[p.Name], []string{"currency", "amount"}, nil)
			if err == nil {
				var actualCurrency string
				actualCurrency, err = stringField(fields, "currency")
				if err == nil && (actualCurrency != p.Currency || !currency(actualCurrency)) {
					return nil, contractError("E_CONTRACT_INPUT", "%s: currency does not match contract", p.Name)
				}
				if err == nil {
					text, err = stringField(fields, "amount")
				}
			}
		default:
			return nil, contractError("E_CONTRACT_INPUT", "%s: unsupported parameter type", p.Name)
		}
		if err != nil {
			return nil, contractError("E_CONTRACT_INPUT", "%s: %v", p.Name, err)
		}
		amount, e := parseInputAmount(text, p.ScaleMax, limits)
		if e != nil {
			return nil, contractError("E_CONTRACT_INPUT", "%s: %v", p.Name, e)
		}
		lo, hi, e := bounds(p.Min, p.Max, limits)
		if e != nil {
			return nil, contractError("E_CONTRACT_INPUT", "%s: invalid contract bounds", p.Name)
		}
		if decimal.Cmp(amount, lo) < 0 || decimal.Cmp(amount, hi) > 0 {
			return nil, contractError("E_CONTRACT_INPUT", "%s: value outside inclusive contract range", p.Name)
		}
		if p.Type == "money" {
			m, e := money.New(p.Currency, amount)
			if e != nil {
				return nil, contractError("E_CONTRACT_INPUT", "%s: %v", p.Name, e)
			}
			values = append(values, eval.MoneyValue{Value: m})
		} else {
			values = append(values, eval.DecimalValue{Value: amount})
		}
	}
	return values, nil
}

func parseInputAmount(text string, scaleMax uint32, limits config.Limits) (decimal.Decimal, error) {
	if point := strings.IndexByte(text, '.'); point >= 0 && uint64(len(text)-point-1) > uint64(scaleMax) {
		return decimal.Decimal{}, fmt.Errorf("raw fractional scale exceeds scale_max")
	}
	return decimal.Parse(text, limits)
}

func decodeListInput(value any, spec *listSpec, limits config.Limits) (eval.Value, error) {
	cap := collectionItemLimit(limits)
	if spec == nil || spec.maxItems < 1 || len(spec.fields) == 0 || len(spec.fields) > cap {
		return nil, fmt.Errorf("invalid record list contract")
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("expected a JSON array of records")
	}
	if maxItems := min(spec.maxItems, cap); len(items) > maxItems {
		return nil, fmt.Errorf("list exceeds max_items %d", maxItems)
	}
	list := make([]eval.Value, len(items))
	for i, item := range items {
		record, err := decodeRecordInput(item, spec.fields, limits, false)
		if err != nil {
			return nil, fmt.Errorf("item %d: %w", i, err)
		}
		list[i] = record
	}
	return eval.NewListValue(list), nil
}

func decodeRecordInput(value any, spec []fieldSpec, limits config.Limits, nonemptyStrings bool) (eval.Value, error) {
	fields, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("record must be an object")
	}
	if len(fields) != len(spec) {
		return nil, fmt.Errorf("record must have exactly the contract fields")
	}
	record := make([]eval.RecordField, len(spec))
	for i, field := range spec {
		value, ok := fields[field.name]
		if !ok {
			return nil, fmt.Errorf("missing field %q", field.name)
		}
		if nonemptyStrings && field.kind == "string" && value == "" {
			return nil, fmt.Errorf("field %q must not be empty", field.name)
		}
		decoded, err := decodeRecordField(value, field, limits)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", field.name, err)
		}
		record[i] = eval.RecordField{Name: field.name, Value: decoded}
	}
	return eval.NewRecordValue(record), nil
}

func decodeRecordField(value any, spec fieldSpec, limits config.Limits) (eval.Value, error) {
	if spec.kind == "string" {
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("expected a string")
		}
		if len(text) > min(spec.maxBytes, limits.MaxTokenBytes) {
			return nil, fmt.Errorf("string exceeds max_bytes")
		}
		return eval.StringValue(text), nil
	}
	text, _ := value.(string)
	var code string
	if spec.kind == "money" {
		fields, err := object(value, []string{"currency", "amount"}, nil)
		if err != nil {
			return nil, err
		}
		code, err = stringField(fields, "currency")
		if err != nil || !slices.Contains(spec.currencies, code) {
			return nil, fmt.Errorf("currency does not match contract")
		}
		text, err = stringField(fields, "amount")
		if err != nil {
			return nil, err
		}
	} else if _, ok := value.(string); !ok {
		return nil, fmt.Errorf("decimal must be a string")
	}
	amount, err := parseInputAmount(text, min(spec.amount.scale, limits.MaxScale), limits)
	if err != nil {
		return nil, err
	}
	if decimal.Cmp(amount, spec.amount.lo) < 0 || decimal.Cmp(amount, spec.amount.hi) > 0 {
		return nil, fmt.Errorf("value outside inclusive contract range")
	}
	if spec.kind == "money" {
		value, err := money.New(code, amount)
		if err != nil {
			return nil, err
		}
		return eval.MoneyValue{Value: value}, nil
	}
	return eval.DecimalValue{Value: amount}, nil
}
