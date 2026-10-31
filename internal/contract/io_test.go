package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/decimal"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/eval"
	"github.com/Pirite08/finlang/internal/money"
)

const validInput = `{"schema_version":"1","rule":"test_v1","inputs":{"rate":"0.100000000000000001","amount":{"currency":"USD","amount":"-12.34567890123456789"}}}`

func testSpec(t *testing.T) Spec {
	t.Helper()
	s, e := Load(strings.NewReader(validContract), config.DefaultLimits())
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func testMoney(t *testing.T, currency, amount string) eval.Value {
	t.Helper()
	d, e := decimal.Parse(amount, config.DefaultLimits())
	if e != nil {
		t.Fatal(e)
	}
	m, e := money.New(currency, d)
	if e != nil {
		t.Fatal(e)
	}
	return eval.MoneyValue{Value: m}
}

func TestContractRoundTrip(t *testing.T) {
	spec := testSpec(t)
	values, err := DecodeInput(strings.NewReader(validInput), spec, config.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 {
		t.Fatalf("argument count %d", len(values))
	}
	amount, ok := values[0].(eval.MoneyValue)
	if !ok || amount.Value.Currency() != "USD" || amount.Value.Amount().String() != "-12.34567890123456789" {
		t.Fatalf("first argument: %#v", values[0])
	}
	rate, ok := values[1].(eval.DecimalValue)
	if !ok || rate.Value.String() != "0.100000000000000001" {
		t.Fatalf("second argument: %#v", values[1])
	}
	var out bytes.Buffer
	if err := EncodeResult(&out, spec, values[0]); err != nil {
		t.Fatal(err)
	}
	want := "{\"schema_version\":\"1\",\"rule\":\"test_v1\",\"status\":\"ok\",\"result\":{\"currency\":\"USD\",\"amount\":\"-12.345678901234567890\"}}\n"
	if out.String() != want {
		t.Fatalf("got %s want %s", out.String(), want)
	}
}

func TestStrictJSONInput(t *testing.T) {
	cases := map[string]string{
		"numeric decimal":      strings.Replace(validInput, `"0.100000000000000001"`, `0.100000000000000001`, 1),
		"numeric money":        strings.Replace(validInput, `"-12.34567890123456789"`, `-12.3`, 1),
		"wrong string":         strings.Replace(validInput, `"0.100000000000000001"`, `"1e-2"`, 1),
		"duplicate wrapper":    strings.Replace(validInput, `"rule":"test_v1"`, `"rule":"test_v1","rule":"test_v1"`, 1),
		"escaped duplicate":    strings.Replace(validInput, `"rate":`, `"r\u0061te":"0.2","rate":`, 1),
		"duplicate money":      strings.Replace(validInput, `"currency":"USD"`, `"currency":"USD","currency":"USD"`, 1),
		"unknown wrapper":      strings.Replace(validInput, `"inputs":`, `"unknown":0,"inputs":`, 1),
		"unknown parameter":    strings.Replace(validInput, `"inputs":{`, `"inputs":{"other":"1",`, 1),
		"unknown money":        strings.Replace(validInput, `"currency":"USD"`, `"currency":"USD","other":"1"`, 1),
		"missing value":        strings.Replace(validInput, `"rate":"0.100000000000000001",`, ``, 1),
		"null value":           strings.Replace(validInput, `"0.100000000000000001"`, `null`, 1),
		"wrong version":        strings.Replace(validInput, `"schema_version":"1"`, `"schema_version":"2"`, 1),
		"wrong rule":           strings.Replace(validInput, `"rule":"test_v1"`, `"rule":"other"`, 1),
		"wrong currency":       strings.Replace(validInput, `"currency":"USD"`, `"currency":"KRW"`, 1),
		"unsupported currency": strings.Replace(validInput, `"currency":"USD"`, `"currency":"EUR"`, 1),
		"raw scale":            strings.Replace(validInput, `"0.100000000000000001"`, `"0.1000000000000000000"`, 1),
		"range":                strings.Replace(validInput, `"-12.34567890123456789"`, `"-100.000000000000000001"`, 1),
		"trailing document":    validInput + `{}`,
		"invalid utf8":         validInput + string([]byte{0xff}),
		"too many bytes":       strings.Repeat(" ", 1<<20) + validInput,
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			values, err := DecodeInput(strings.NewReader(input), testSpec(t), config.DefaultLimits())
			if diagnostic.CodeOf(err) != "E_CONTRACT_INPUT" || values != nil {
				t.Fatalf("values=%v err=%v", values, err)
			}
		})
	}
	limits := config.DefaultLimits()
	limits.MaxSyntaxDepth = 2
	if _, err := DecodeInput(strings.NewReader(validInput), testSpec(t), limits); diagnostic.CodeOf(err) != "E_CONTRACT_INPUT" {
		t.Fatalf("depth limit: %v", err)
	}
}

func TestOutputContract(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value eval.Value
		scale uint32
		code  diagnostic.Code
	}{
		{"wrong type", eval.StringValue("1"), 2, "E_CONTRACT_OUTPUT"},
		{"wrong currency", testMoney(t, "KRW", "1"), 2, "E_CONTRACT_OUTPUT"},
		{"too precise", testMoney(t, "USD", "1.001"), 2, "E_OUTPUT_SCALE"},
		{"above range", testMoney(t, "USD", "100.001"), 2, "E_LEGACY_RANGE"},
		{"below range", testMoney(t, "USD", "-100.01"), 2, "E_LEGACY_RANGE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spec := testSpec(t)
			spec.Output.Scale = tc.scale
			var out bytes.Buffer
			err := EncodeResult(&out, spec, tc.value)
			if diagnostic.CodeOf(err) != tc.code || out.Len() != 0 {
				t.Fatalf("got %v, output %q", err, out.String())
			}
		})
	}
	spec := testSpec(t)
	spec.Output.Scale = 2
	for _, amount := range []string{"-100", "100", "0"} {
		var out bytes.Buffer
		if err := EncodeResult(&out, spec, testMoney(t, "USD", amount)); err != nil {
			t.Fatal(err)
		}
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestJSONWriterErrors(t *testing.T) {
	sentinel := errors.New("host write failed")
	if err := EncodeResult(failingWriter{sentinel}, testSpec(t), testMoney(t, "USD", "1")); !errors.Is(err, sentinel) {
		t.Fatalf("result writer: %v", err)
	}
	if err := EncodeError(failingWriter{sentinel}, "test_v1", diagnostic.New("E_CONTRACT", "invalid")); !errors.Is(err, sentinel) {
		t.Fatalf("error writer: %v", err)
	}
}

func TestEncodeError(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      error
		location string
	}{
		{"no span", diagnostic.New("E_CONTRACT", "invalid input"), `"file":null,"line":null,"column":null`},
		{"span", fmt.Errorf("wrapped: %w", diagnostic.WithSpan(diagnostic.New("E_TYPE", "wrong type"), diagnostic.Span{File: "rule.fin", Line: 3, Column: 9})), `"file":"rule.fin","line":3,"column":9`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := EncodeError(&out, "test_v1", tc.err); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), tc.location) || !strings.Contains(out.String(), `"status":"error"`) || strings.Contains(out.String(), `"result"`) {
				t.Fatal(out.String())
			}
			d := json.NewDecoder(&out)
			var value any
			if err := d.Decode(&value); err != nil {
				t.Fatal(err)
			}
			if err := d.Decode(&value); err != io.EOF {
				t.Fatalf("extra record: %v", err)
			}
		})
	}
}
