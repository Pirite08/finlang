package contract

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/eval"
)

const quoteRecordContract = `{
	"schema_version":"3", "rule":"transfer_quote_krw_v1", "entry":"transfer_quote_krw_v1",
	"parameters":[{"name":"transfer","type":"record","fields":[
		{"name":"request_id","type":"string","max_bytes":64},
		{"name":"from_account","type":"string","max_bytes":64},
		{"name":"to_account","type":"string","max_bytes":64},
		{"name":"amount","type":"money","currencies":["KRW"],"min":"1","max":"9223372036854775807","scale_max":0}
	]}],
	"output":{"type":"record","fields":[
		{"name":"fee","type":"money","currencies":["KRW"],"min":"0","max":"9223372036854775807","scale":0},
		{"name":"total_debit","type":"money","currencies":["KRW"],"min":"1","max":"9223372036854775807","scale":0}
	]}
}`

const quoteRecordInput = `{"schema_version":"3","rule":"transfer_quote_krw_v1","inputs":{"transfer":{"request_id":"quote-1","from_account":"A","to_account":"B","amount":{"currency":"KRW","amount":"10000"}}}}`

func quoteRecordSpec(t *testing.T) Spec {
	t.Helper()
	spec, err := Load(strings.NewReader(quoteRecordContract), config.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	return spec
}

func TestRecordContractMetadata(t *testing.T) {
	spec := quoteRecordSpec(t)
	if spec.SchemaVersion != "3" || len(spec.Parameters) != 1 || spec.Parameters[0].Type != "record" || spec.Output.Type != "record" {
		t.Fatalf("unexpected contract: %+v", spec)
	}
	for _, tc := range []struct{ name, before, after string }{
		{"duplicate input field", `"name":"to_account"`, `"name":"from_account"`},
		{"duplicate output field", `"name":"total_debit"`, `"name":"fee"`},
		{"zero string byte limit", `"max_bytes":64`, `"max_bytes":0`},
		{"missing output scale", `"scale":0`, `"scale_max":0`},
		{"unknown field metadata", `"scale_max":0`, `"scale_max":0,"unexpected":true`},
		{"wrong output field type", `"name":"fee","type":"money"`, `"name":"fee","type":"decimal"`},
		{"wrong output shape", `"output":{"type":"record"`, `"output":{"type":"map"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metadata := strings.Replace(quoteRecordContract, tc.before, tc.after, 1)
			if _, err := Load(strings.NewReader(metadata), config.DefaultLimits()); diagnostic.CodeOf(err) != "E_CONTRACT" {
				t.Fatalf("got %v", err)
			}
		})
	}
	limits := config.DefaultLimits()
	limits.MaxCollectionItems = 3
	if _, err := Load(strings.NewReader(quoteRecordContract), limits); diagnostic.CodeOf(err) != "E_CONTRACT" {
		t.Fatalf("record wider than collection cap: %v", err)
	}
}

func TestRecordContractInput(t *testing.T) {
	spec := quoteRecordSpec(t)
	limits := config.DefaultLimits()
	for _, text := range []string{quoteRecordInput, strings.Replace(quoteRecordInput, `"10000"`, `"9223372036854775807"`, 1)} {
		values, err := DecodeInput(strings.NewReader(text), spec, limits)
		if err != nil || len(values) != 1 || values[0].Kind() != eval.KindRecord {
			t.Fatalf("got %v, %v", values, err)
		}
	}
	for _, tc := range []struct{ name, before, after string }{
		{"missing request id", `"request_id":"quote-1",`, ``},
		{"missing account", `"to_account":"B",`, ``},
		{"unknown field", `"to_account"`, `"destination"`},
		{"escaped duplicate field", `"to_account":"B",`, `"to_account":"B","to_\u0061ccount":"C",`},
		{"empty account", `"to_account":"B"`, `"to_account":""`},
		{"invalid Unicode surrogate", `"to_account":"B"`, `"to_account":"\uD800"`},
		{"string too long in bytes", `"request_id":"quote-1"`, `"request_id":"` + strings.Repeat("거", 22) + `"`},
		{"currency outside allowlist", `"KRW"`, `"USD"`},
		{"numeric amount", `"10000"`, `10000`},
		{"fractional won", `"10000"`, `"10000.00"`},
		{"amount below minimum", `"10000"`, `"0"`},
		{"amount above maximum", `"10000"`, `"9223372036854775808"`},
		{"wrong schema version", `"schema_version":"3"`, `"schema_version":"2"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := strings.Replace(quoteRecordInput, tc.before, tc.after, 1)
			values, err := DecodeInput(strings.NewReader(input), spec, limits)
			if values != nil || diagnostic.CodeOf(err) != "E_CONTRACT_INPUT" {
				t.Fatalf("got %v, %v", values, err)
			}
		})
	}
}

func TestRecordContractResult(t *testing.T) {
	spec := quoteRecordSpec(t)
	fee := eval.RecordField{Name: "fee", Value: testMoney(t, "KRW", "500")}
	debit := eval.RecordField{Name: "total_debit", Value: testMoney(t, "KRW", "10500")}
	for _, tc := range []struct {
		name string
		val  eval.Value
		code diagnostic.Code
	}{
		{"complete record", eval.NewRecordValue([]eval.RecordField{debit, fee}), ""},
		{"scalar", fee.Value, "E_CONTRACT_OUTPUT"},
		{"missing field", eval.NewRecordValue([]eval.RecordField{fee}), "E_CONTRACT_OUTPUT"},
		{"unknown field", eval.NewRecordValue([]eval.RecordField{fee, {Name: "debit", Value: debit.Value}}), "E_CONTRACT_OUTPUT"},
		{"duplicate field", eval.NewRecordValue([]eval.RecordField{fee, fee}), "E_CONTRACT_OUTPUT"},
		{"wrong field type", eval.NewRecordValue([]eval.RecordField{{Name: "fee", Value: eval.StringValue("500")}, debit}), "E_CONTRACT_OUTPUT"},
		{"wrong currency", eval.NewRecordValue([]eval.RecordField{{Name: "fee", Value: testMoney(t, "USD", "500")}, debit}), "E_CONTRACT_OUTPUT"},
		{"wrong scale", eval.NewRecordValue([]eval.RecordField{{Name: "fee", Value: testMoney(t, "KRW", "500.1")}, debit}), "E_OUTPUT_SCALE"},
		{"wrong range", eval.NewRecordValue([]eval.RecordField{fee, {Name: "total_debit", Value: testMoney(t, "KRW", "9223372036854775808")}}), "E_LEGACY_RANGE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			err := EncodeResult(&out, spec, tc.val)
			if tc.code != "" {
				if diagnostic.CodeOf(err) != tc.code || out.Len() != 0 {
					t.Fatalf("got %v, %q", err, out.String())
				}
				return
			}
			want := `{"schema_version":"3","rule":"transfer_quote_krw_v1","status":"ok","result":{"fee":{"currency":"KRW","amount":"500"},"total_debit":{"currency":"KRW","amount":"10500"}}}` + "\n"
			if err != nil || out.String() != want {
				t.Fatalf("got %v, %q want %q", err, out.String(), want)
			}
		})
	}
}

func TestRecordContractErrorVersion(t *testing.T) {
	var out bytes.Buffer
	if err := EncodeErrorVersion(&out, "3", "transfer_quote_krw_v1", diagnostic.New("E_CONTRACT_INPUT", "invalid")); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), `{"schema_version":"3","rule":"transfer_quote_krw_v1","status":"error"`) {
		t.Fatalf("unexpected error envelope: %s", &out)
	}
}
