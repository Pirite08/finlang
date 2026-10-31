package contract

import (
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
)

const validContract = `{"schema_version":"1","rule":"test_v1","entry":"main","parameters":[{"name":"amount","type":"money","currency":"USD","min":"-100","max":"100","scale_max":18},{"name":"rate","type":"decimal","min":"0","max":"1","scale_max":18}],"output":{"type":"money","currency":"USD","scale":18,"min":"-100","max":"100"}}`

func TestLoadContract(t *testing.T) {
	spec, err := Load(strings.NewReader(validContract), config.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if spec.Rule != "test_v1" || spec.Entry != "main" || len(spec.Parameters) != 2 || spec.Parameters[0].Name != "amount" || spec.Parameters[1].ScaleMax != 18 {
		t.Fatalf("unexpected contract: %+v", spec)
	}
}

func TestStrictJSONMetadata(t *testing.T) {
	cases := map[string]string{
		"duplicate":            strings.Replace(validContract, `"rule":"test_v1"`, `"rule":"a","rule":"test_v1"`, 1),
		"escaped duplicate":    strings.Replace(validContract, `"rule":"test_v1"`, `"rule":"a","r\u0075le":"test_v1"`, 1),
		"nested duplicate":     strings.Replace(validContract, `"currency":"USD"`, `"currency":"USD","currency":"USD"`, 1),
		"unknown root":         strings.Replace(validContract, `"entry":"main"`, `"entry":"main","other":true`, 1),
		"unknown nested":       strings.Replace(validContract, `"name":"amount"`, `"name":"amount","extra":null`, 1),
		"numeric bound":        strings.Replace(validContract, `"min":"-100"`, `"min":-100`, 1),
		"missing scale":        strings.Replace(validContract, `,"scale_max":18`, ``, 1),
		"null scale":           strings.Replace(validContract, `"scale_max":18`, `"scale_max":null`, 1),
		"fraction scale":       strings.Replace(validContract, `"scale_max":18`, `"scale_max":1.0`, 1),
		"wrong version":        strings.Replace(validContract, `"schema_version":"1"`, `"schema_version":"2"`, 1),
		"bad rule":             strings.Replace(validContract, `"rule":"test_v1"`, `"rule":"bad rule"`, 1),
		"duplicate parameter":  strings.Replace(validContract, `"name":"rate"`, `"name":"amount"`, 1),
		"unsupported type":     strings.Replace(validContract, `"type":"decimal"`, `"type":"number"`, 1),
		"unsupported currency": strings.Replace(validContract, `"currency":"USD"`, `"currency":"EUR"`, 1),
		"reversed bounds":      strings.Replace(validContract, `"min":"-100"`, `"min":"101"`, 1),
		"nonmoney output":      strings.Replace(validContract, `"output":{"type":"money"`, `"output":{"type":"decimal"`, 1),
		"trailing document":    validContract + `{}`,
		"invalid utf8":         validContract + string([]byte{0xff}),
		"null parameter":       strings.Replace(validContract, `{"name":"rate","type":"decimal","min":"0","max":"1","scale_max":18}`, `null`, 1),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Load(strings.NewReader(input), config.DefaultLimits())
			if diagnostic.CodeOf(err) != "E_CONTRACT" {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestStrictJSONResourceLimits(t *testing.T) {
	limits := config.DefaultLimits()
	limits.MaxSourceBytes = len(validContract) - 1
	if _, err := Load(strings.NewReader(validContract), limits); diagnostic.CodeOf(err) != "E_CONTRACT" {
		t.Fatalf("byte limit: %v", err)
	}
	limits = config.DefaultLimits()
	limits.MaxSyntaxDepth = 2
	if _, err := Load(strings.NewReader(validContract), limits); diagnostic.CodeOf(err) != "E_CONTRACT" {
		t.Fatalf("depth limit: %v", err)
	}
	limits = config.DefaultLimits()
	if _, err := Load(strings.NewReader(strings.Repeat(" ", 1<<20)+validContract), limits); diagnostic.CodeOf(err) != "E_CONTRACT" {
		t.Fatalf("default byte limit: %v", err)
	}
}
