// Package contract validates versioned JSON contracts without floating point conversion.
package contract

import (
	"fmt"
	"io"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/decimal"
	"github.com/Pirite08/finlang/internal/diagnostic"
)

type Spec struct {
	SchemaVersion string      `json:"schema_version"`
	Rule          string      `json:"rule"`
	Entry         string      `json:"entry"`
	Parameters    []Parameter `json:"parameters"`
	Output        OutputSpec  `json:"output"`
}

type Parameter struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Currency string `json:"currency,omitempty"`
	Min      string `json:"min"`
	Max      string `json:"max"`
	ScaleMax uint32 `json:"scale_max"`
	list     *listSpec
	record   []fieldSpec
}

type OutputSpec struct {
	Type        string `json:"type"`
	Currency    string `json:"currency"`
	Min         string `json:"min"`
	Max         string `json:"max"`
	Scale       uint32 `json:"scale"`
	currencyMap *currencyMapSpec
	record      []fieldSpec
}

func contractError(code diagnostic.Code, format string, args ...any) error {
	return diagnostic.New(code, fmt.Sprintf(format, args...))
}

func identifier(s string) bool {
	if s == "" {
		return false
	}
	for i, c := range []byte(s) {
		if c != '_' && !(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') && !(i > 0 && c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

func currency(s string) bool { return s == "KRW" || s == "USD" }

func bounds(min, max string, limits config.Limits) (decimal.Decimal, decimal.Decimal, error) {
	lo, err := decimal.Parse(min, limits)
	if err != nil {
		return lo, decimal.Decimal{}, err
	}
	hi, err := decimal.Parse(max, limits)
	if err != nil {
		return lo, hi, err
	}
	if decimal.Cmp(lo, hi) > 0 {
		return lo, hi, fmt.Errorf("minimum exceeds maximum")
	}
	return lo, hi, nil
}

// Load rejects malformed, ambiguous and unsupported contract metadata.
func Load(r io.Reader, limits config.Limits) (Spec, error) {
	var spec Spec
	value, err := readJSON(r, limits)
	if err != nil {
		return spec, contractError("E_CONTRACT", "%v", err)
	}
	root, err := object(value, []string{"schema_version", "rule", "entry", "parameters", "output"}, nil)
	if err != nil {
		return spec, contractError("E_CONTRACT", "%v", err)
	}
	spec.SchemaVersion, err = stringField(root, "schema_version")
	if err == nil {
		spec.Rule, err = stringField(root, "rule")
	}
	if err == nil {
		spec.Entry, err = stringField(root, "entry")
	}
	if err != nil {
		return spec, contractError("E_CONTRACT", "%v", err)
	}
	if !supportedSchema(spec.SchemaVersion) || !identifier(spec.Rule) || !identifier(spec.Entry) || len(spec.Rule) > limits.MaxTokenBytes || len(spec.Entry) > limits.MaxTokenBytes {
		return spec, contractError("E_CONTRACT", "invalid schema version, rule or entry")
	}
	parameters, ok := root["parameters"].([]any)
	if !ok {
		return spec, contractError("E_CONTRACT", "parameters must be an array")
	}
	if spec.SchemaVersion != "1" && len(parameters) > collectionItemLimit(limits) {
		return spec, contractError("E_CONTRACT", "too many parameters")
	}
	spec.Parameters = make([]Parameter, 0, len(parameters))
	names := map[string]bool{}
	for _, value := range parameters {
		fields, _ := value.(map[string]any)
		isList := spec.SchemaVersion == "2" && fields["type"] == "list"
		isRecord := spec.SchemaVersion == "3" && fields["type"] == "record"
		if isList || isRecord {
			var p Parameter
			var err error
			if isList {
				p, err = loadListParameter(fields, limits)
			} else {
				p, err = loadRecordParameter(fields, limits)
			}
			if err != nil {
				return spec, contractError("E_CONTRACT", "%s parameter: %v", fields["type"], err)
			}
			if names[p.Name] {
				return spec, contractError("E_CONTRACT", "invalid or duplicate parameter name %q", p.Name)
			}
			names[p.Name] = true
			spec.Parameters = append(spec.Parameters, p)
			continue
		}
		pmap, e := object(value, []string{"name", "type", "min", "max", "scale_max"}, []string{"currency"})
		if e != nil {
			return spec, contractError("E_CONTRACT", "parameter: %v", e)
		}
		var p Parameter
		for _, field := range []struct {
			name   string
			target *string
		}{{"name", &p.Name}, {"type", &p.Type}, {"min", &p.Min}, {"max", &p.Max}} {
			*field.target, e = stringField(pmap, field.name)
			if e != nil {
				return spec, contractError("E_CONTRACT", "parameter: %v", e)
			}
		}
		p.ScaleMax, e = scaleField(pmap, "scale_max", limits)
		if e != nil {
			return spec, contractError("E_CONTRACT", "parameter: %v", e)
		}
		if !identifier(p.Name) || len(p.Name) > limits.MaxTokenBytes || names[p.Name] {
			return spec, contractError("E_CONTRACT", "invalid or duplicate parameter name %q", p.Name)
		}
		names[p.Name] = true
		if p.Type == "money" {
			p.Currency, e = stringField(pmap, "currency")
			if e != nil || !currency(p.Currency) {
				return spec, contractError("E_CONTRACT", "unsupported money currency")
			}
		} else if p.Type == "decimal" {
			if _, present := pmap["currency"]; present {
				return spec, contractError("E_CONTRACT", "decimal parameter cannot have currency")
			}
		} else {
			return spec, contractError("E_CONTRACT", "unsupported parameter type %q", p.Type)
		}
		if _, _, e = bounds(p.Min, p.Max, limits); e != nil {
			return spec, contractError("E_CONTRACT", "parameter bounds: %v", e)
		}
		spec.Parameters = append(spec.Parameters, p)
	}
	if spec.SchemaVersion == "2" {
		output, err := loadCurrencyMapOutput(root["output"], limits)
		if err != nil {
			return spec, contractError("E_CONTRACT", "output: %v", err)
		}
		spec.Output = output
		return spec, nil
	}
	if spec.SchemaVersion == "3" {
		output, err := loadRecordOutput(root["output"], limits)
		if err != nil {
			return spec, contractError("E_CONTRACT", "output: %v", err)
		}
		spec.Output = output
		return spec, nil
	}
	omap, e := object(root["output"], []string{"type", "currency", "min", "max", "scale"}, nil)
	if e != nil {
		return spec, contractError("E_CONTRACT", "output: %v", e)
	}
	for _, field := range []struct {
		name   string
		target *string
	}{{"type", &spec.Output.Type}, {"currency", &spec.Output.Currency}, {"min", &spec.Output.Min}, {"max", &spec.Output.Max}} {
		*field.target, e = stringField(omap, field.name)
		if e != nil {
			return spec, contractError("E_CONTRACT", "output: %v", e)
		}
	}
	spec.Output.Scale, e = scaleField(omap, "scale", limits)
	if e != nil {
		return spec, contractError("E_CONTRACT", "output: %v", e)
	}
	if spec.Output.Type != "money" || !currency(spec.Output.Currency) {
		return spec, contractError("E_CONTRACT", "output must be money in a supported currency")
	}
	if _, _, e = bounds(spec.Output.Min, spec.Output.Max, limits); e != nil {
		return spec, contractError("E_CONTRACT", "output bounds: %v", e)
	}
	return spec, nil
}

func supportedSchema(version string) bool {
	return version == "1" || version == "2" || version == "3"
}
