package contract

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/eval"
	"github.com/Pirite08/finlang/internal/parser"
)

const collectionContract = `{
  "schema_version":"2", "rule":"totals_v2", "entry":"main",
  "parameters":[{"name":"transactions","type":"list","max_items":2,"item":{"type":"record","fields":[
    {"name":"id","type":"string","max_bytes":8},
    {"name":"rate","type":"decimal","min":"0","max":"1","scale_max":2},
    {"name":"amount","type":"money","currencies":["USD","KRW"],"min":"-100","max":"100","scale_max":2}
  ]}}],
  "output":{"type":"map","max_items":2,"value":{"type":"money","currencies":["USD","KRW"],"min":"-100","max":"100","scale":2}}
}`

const transaction = `{"id":"거래","rate":"0.10","amount":{"currency":"USD","amount":"0.20"}}`

func transactionInput(records string) string {
	return `{"schema_version":"2","rule":"totals_v2","inputs":{"transactions":[` + records + `]}}`
}

func collectionSpec(t *testing.T) Spec {
	t.Helper()
	spec, err := Load(strings.NewReader(collectionContract), config.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	return spec
}

func TestCollectionContractSchema(t *testing.T) {
	spec := collectionSpec(t)
	if spec.SchemaVersion != "2" || len(spec.Parameters) != 1 || spec.Parameters[0].Type != "list" || spec.Output.Type != "map" {
		t.Fatalf("unexpected contract: %+v", spec)
	}
	for _, tc := range []struct{ name, before, after string }{
		{"zero item limit", `"max_items":2`, `"max_items":0`},
		{"too large item limit", `"max_items":2`, `"max_items":1025`},
		{"fractional item limit", `"max_items":2`, `"max_items":2.0`},
		{"non-record item", `"type":"record"`, `"type":"list"`},
		{"duplicate field", `"name":"rate"`, `"name":"id"`},
		{"unsupported record field type", `"type":"string"`, `"type":"list"`},
		{"zero string byte limit", `"max_bytes":8`, `"max_bytes":0`},
		{"duplicate currency", `["USD","KRW"]`, `["USD","USD"]`},
		{"unsupported currency", `["USD","KRW"]`, `["USD","EUR"]`},
		{"empty currency allowlist", `["USD","KRW"]`, `[]`},
		{"wrong output value type", `"value":{"type":"money"`, `"value":{"type":"record"`},
		{"extra nested metadata", `"max_bytes":8`, `"max_bytes":8,"extra":true`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metadata := strings.Replace(collectionContract, tc.before, tc.after, 1)
			_, err := Load(strings.NewReader(metadata), config.DefaultLimits())
			if diagnostic.CodeOf(err) != "E_CONTRACT" {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestCollectionContractInput(t *testing.T) {
	spec := collectionSpec(t)
	for _, input := range []string{"", transaction + "," + transaction, strings.Replace(transaction, `"거래"`, `"\uD83D\uDE00"`, 1)} {
		values, err := DecodeInput(strings.NewReader(transactionInput(input)), spec, config.DefaultLimits())
		if err != nil || len(values) != 1 || values[0].Kind() != eval.KindList {
			t.Fatalf("got %v, %v", values, err)
		}
	}
	for _, tc := range []struct{ name, before, after string }{
		{"unknown row field", `"id":`, `"extra":0,"id":`},
		{"missing row field", `"id":"거래",`, ``},
		{"duplicate row field", `"id":`, `"\u0069d":"x","id":`},
		{"string byte length", `"거래"`, `"거래목록"`},
		{"invalid Unicode surrogate", `"거래"`, `"\uD800"`},
		{"unquoted decimal", `"0.10"`, `0.10`},
		{"out of range decimal", `"0.10"`, `"1.01"`},
		{"money currency outside allowlist", `"USD"`, `"EUR"`},
		{"duplicate money currency", `"currency":"USD"`, `"currency":"USD","currency":"USD"`},
		{"money raw scale", `"0.20"`, `"0.200"`},
		{"money range", `"0.20"`, `"100.01"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := transactionInput(strings.Replace(transaction, tc.before, tc.after, 1))
			values, err := DecodeInput(strings.NewReader(input), spec, config.DefaultLimits())
			if values != nil || diagnostic.CodeOf(err) != "E_CONTRACT_INPUT" {
				t.Fatalf("got %v, %v", values, err)
			}
		})
	}
	input := transactionInput(strings.TrimSuffix(strings.Repeat(transaction+",", 3), ","))
	if _, err := DecodeInput(strings.NewReader(input), spec, config.DefaultLimits()); diagnostic.CodeOf(err) != "E_CONTRACT_INPUT" {
		t.Fatalf("got %v", err)
	}
}

func TestCollectionContractSmallerLimits(t *testing.T) {
	limits := config.DefaultLimits()
	limits.MaxCollectionItems = 1
	if _, err := Load(strings.NewReader(collectionContract), limits); diagnostic.CodeOf(err) != "E_CONTRACT" {
		t.Fatalf("contract above custom cap: %v", err)
	}
	metadata := strings.ReplaceAll(collectionContract, `"max_items":2`, `"max_items":1`)
	metadata = strings.ReplaceAll(metadata, `{"name":"id","type":"string","max_bytes":8},`, ``)
	metadata = strings.ReplaceAll(metadata, `{"name":"rate","type":"decimal","min":"0","max":"1","scale_max":2},`, ``)
	spec, err := Load(strings.NewReader(metadata), limits)
	if err != nil {
		t.Fatal(err)
	}
	record := `{"amount":{"currency":"USD","amount":"0.20"}}`
	if _, err := DecodeInput(strings.NewReader(transactionInput(record)), spec, limits); err != nil {
		t.Fatalf("one row with one Money field: %v", err)
	}
	if _, err := DecodeInput(strings.NewReader(transactionInput(record+","+record)), spec, limits); diagnostic.CodeOf(err) != "E_CONTRACT_INPUT" || !strings.Contains(err.Error(), "JSON array exceeds") {
		t.Fatalf("input above scan cap: %v", err)
	}
}

func TestCollectionContractResult(t *testing.T) {
	spec := collectionSpec(t)
	for _, tc := range []struct {
		name, source, result string
		code                 diagnostic.Code
	}{
		{"currency order and exact scale", `map {"USD": money("USD", 1.2), "KRW": money("KRW", 2)};`, `{"KRW":{"currency":"KRW","amount":"2.00"},"USD":{"currency":"USD","amount":"1.20"}}`, ""},
		{"empty map", `map {};`, `{}`, ""},
		{"wrong result type", `1;`, ``, "E_CONTRACT_OUTPUT"},
		{"wrong value type", `map {"USD": 1};`, ``, "E_CONTRACT_OUTPUT"},
		{"wrong key", `map {"KRW": money("USD", 1)};`, ``, "E_CONTRACT_OUTPUT"},
		{"excess entries", `map {"USD": money("USD", 1), "KRW": money("KRW", 1), "other": 1};`, ``, "E_CONTRACT_OUTPUT"},
		{"output range", `map {"USD": money("USD", 101)};`, ``, "E_LEGACY_RANGE"},
		{"output precision", `map {"USD": money("USD", 1.001)};`, ``, "E_OUTPUT_SCALE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := config.DefaultLimits()
			program, err := parser.Parse("test.fin", tc.source, limits)
			if err != nil {
				t.Fatal(err)
			}
			value, err := eval.New(eval.Options{Limits: limits}).Run(program, eval.NewEnvironment(nil))
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			err = EncodeResult(&out, spec, value)
			if tc.code != "" {
				if diagnostic.CodeOf(err) != tc.code || out.Len() != 0 {
					t.Fatalf("got %v, %q", err, out.String())
				}
				return
			}
			want := `{"schema_version":"2","rule":"totals_v2","status":"ok","result":` + tc.result + "}\n"
			if err != nil || out.String() != want {
				t.Fatalf("got %v, %q want %q", err, out.String(), want)
			}
		})
	}
}
