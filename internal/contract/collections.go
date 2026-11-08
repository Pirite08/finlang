package contract

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/decimal"
)

type listSpec struct {
	maxItems int
	fields   []fieldSpec
}

type fieldSpec struct {
	name       string
	kind       string
	maxBytes   int
	currencies []string
	amount     amountSpec
}

type currencyMapSpec struct {
	maxItems   int
	currencies []string
	amount     amountSpec
}

type amountSpec struct {
	lo, hi decimal.Decimal
	scale  uint32
}

func collectionItemLimit(limits config.Limits) int {
	return config.RuleCollectionLimits(limits).MaxCollectionItems
}

func sizeField(fields map[string]any, key string, maxSize int) (int, error) {
	number, ok := fields[key].(json.Number)
	if !ok {
		return 0, fmt.Errorf("field %q must be an integer", key)
	}
	n, err := strconv.Atoi(string(number))
	if err != nil || n < 1 || n > maxSize {
		return 0, fmt.Errorf("field %q must be between 1 and %d", key, maxSize)
	}
	return n, nil
}

func currenciesField(fields map[string]any) ([]string, error) {
	values, ok := fields["currencies"].([]any)
	if !ok || len(values) == 0 || len(values) > 2 {
		return nil, fmt.Errorf("currencies must be a non-empty array of supported currencies")
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		code, ok := value.(string)
		if !ok || !currency(code) || slices.Contains(result, code) {
			return nil, fmt.Errorf("currencies must be supported and unique")
		}
		result = append(result, code)
	}
	return result, nil
}

func loadAmountSpec(fields map[string]any, scaleKey string, limits config.Limits) (amountSpec, error) {
	var spec amountSpec
	minText, err := stringField(fields, "min")
	if err != nil {
		return spec, err
	}
	maxText, err := stringField(fields, "max")
	if err != nil {
		return spec, err
	}
	spec.lo, spec.hi, err = bounds(minText, maxText, limits)
	if err != nil {
		return spec, err
	}
	spec.scale, err = scaleField(fields, scaleKey, limits)
	return spec, err
}

func loadListParameter(value any, limits config.Limits) (Parameter, error) {
	var p Parameter
	fields, err := object(value, []string{"name", "type", "max_items", "item"}, nil)
	if err != nil {
		return p, err
	}
	p.Name, err = stringField(fields, "name")
	if err != nil || !identifier(p.Name) || len(p.Name) > limits.MaxTokenBytes {
		return p, fmt.Errorf("invalid parameter name %q", p.Name)
	}
	p.Type = "list"
	maxItems, err := sizeField(fields, "max_items", collectionItemLimit(limits))
	if err != nil {
		return p, err
	}
	item, err := object(fields["item"], []string{"type", "fields"}, nil)
	if err != nil {
		return p, err
	}
	if item["type"] != "record" {
		return p, fmt.Errorf("list item must have type record")
	}
	recordFields, err := loadRecordFields(item["fields"], limits, false)
	if err != nil {
		return p, err
	}
	p.list = &listSpec{maxItems: maxItems, fields: recordFields}
	return p, nil
}

func loadRecordFields(value any, limits config.Limits, output bool) ([]fieldSpec, error) {
	recordFields, ok := value.([]any)
	if !ok || len(recordFields) == 0 || len(recordFields) > collectionItemLimit(limits) {
		return nil, fmt.Errorf("record fields must be a non-empty array within the collection limit")
	}
	fields := make([]fieldSpec, 0, len(recordFields))
	names := make(map[string]bool, len(recordFields))
	for _, value := range recordFields {
		field, err := loadRecordField(value, limits, output)
		if err != nil {
			return nil, fmt.Errorf("record field: %w", err)
		}
		if names[field.name] {
			return nil, fmt.Errorf("duplicate record field %q", field.name)
		}
		names[field.name] = true
		fields = append(fields, field)
	}
	return fields, nil
}

func loadRecordField(value any, limits config.Limits, output bool) (fieldSpec, error) {
	var field fieldSpec
	metadata, ok := value.(map[string]any)
	if !ok {
		return field, fmt.Errorf("expected JSON object")
	}
	var err error
	field.name, err = stringField(metadata, "name")
	if err != nil || !identifier(field.name) || len(field.name) > limits.MaxTokenBytes {
		return field, fmt.Errorf("invalid field name %q", field.name)
	}
	field.kind, err = stringField(metadata, "type")
	if err != nil {
		return field, err
	}
	if output && field.kind != "money" {
		return field, fmt.Errorf("output record field %q must have type money", field.name)
	}
	keys := []string{"name", "type"}
	switch field.kind {
	case "string":
		if _, err = object(value, append(keys, "max_bytes"), nil); err != nil {
			return field, err
		}
		field.maxBytes, err = sizeField(metadata, "max_bytes", limits.MaxTokenBytes)
	case "decimal", "money":
		scaleKey := "scale_max"
		if output {
			scaleKey = "scale"
		}
		keys = append(keys, "min", "max", scaleKey)
		if field.kind == "money" {
			keys = append(keys, "currencies")
		}
		if _, err = object(value, keys, nil); err != nil {
			return field, err
		}
		if field.kind == "money" {
			field.currencies, err = currenciesField(metadata)
			if err != nil {
				return field, err
			}
		}
		field.amount, err = loadAmountSpec(metadata, scaleKey, limits)
	default:
		return field, fmt.Errorf("unsupported record field type %q", field.kind)
	}
	return field, err
}

func loadCurrencyMapOutput(value any, limits config.Limits) (OutputSpec, error) {
	var output OutputSpec
	fields, err := object(value, []string{"type", "max_items", "value"}, nil)
	if err != nil {
		return output, err
	}
	if fields["type"] != "map" {
		return output, fmt.Errorf("output must have type map")
	}
	maxItems, err := sizeField(fields, "max_items", collectionItemLimit(limits))
	if err != nil {
		return output, err
	}
	item, err := object(fields["value"], []string{"type", "currencies", "min", "max", "scale"}, nil)
	if err != nil {
		return output, err
	}
	if item["type"] != "money" {
		return output, fmt.Errorf("map value must have type money")
	}
	currencies, err := currenciesField(item)
	if err != nil {
		return output, err
	}
	amount, err := loadAmountSpec(item, "scale", limits)
	if err != nil {
		return output, err
	}
	return OutputSpec{Type: "map", currencyMap: &currencyMapSpec{maxItems: maxItems, currencies: currencies, amount: amount}}, nil
}
