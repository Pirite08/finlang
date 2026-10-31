package contract

import (
	"io"
	"strings"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/decimal"
	"github.com/Pirite08/finlang/internal/eval"
	"github.com/Pirite08/finlang/internal/money"
)

// DecodeInput validates all inputs and returns arguments in contract order.
// It does not evaluate a program or call the contract entry function.
func DecodeInput(r io.Reader, spec Spec, limits config.Limits) ([]eval.Value, error) {
	value, err := readJSON(r, limits)
	if err != nil {
		return nil, contractError("E_CONTRACT_INPUT", "%v", err)
	}
	root, err := object(value, []string{"schema_version", "rule", "inputs"}, nil)
	if err != nil {
		return nil, contractError("E_CONTRACT_INPUT", "%v", err)
	}
	version, err := stringField(root, "schema_version")
	if err != nil || version != "1" || version != spec.SchemaVersion {
		return nil, contractError("E_CONTRACT_INPUT", "schema version does not match contract")
	}
	rule, err := stringField(root, "rule")
	if err != nil || rule != spec.Rule {
		return nil, contractError("E_CONTRACT_INPUT", "rule does not match contract")
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
		if point := strings.IndexByte(text, '.'); point >= 0 && uint64(len(text)-point-1) > uint64(p.ScaleMax) {
			return nil, contractError("E_CONTRACT_INPUT", "%s: raw fractional scale exceeds scale_max", p.Name)
		}
		amount, e := decimal.Parse(text, limits)
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
