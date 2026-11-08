package cli

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/contract"
	"github.com/Pirite08/finlang/internal/decimal"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/eval"
	"github.com/Pirite08/finlang/internal/money"
	"github.com/Pirite08/finlang/internal/transfer"
)

func TestPlannedTransferValidatesContractShape(t *testing.T) {
	source, err := os.ReadFile("../../contracts/transfer_plan_krw_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	spec, err := contract.Load(bytes.NewReader(source), config.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if err := validateTransferRoute(servedRule{spec: spec}); err != nil {
		t.Fatalf("example transfer contract rejected: %v", err)
	}
	for _, tc := range []struct{ name, from, to string }{
		{"wrong input field", `"to_account"`, `"target_account"`},
		{"wrong output field", `"credit"`, `"amount"`},
		{"wrong currency", `["KRW"]`, `["USD"]`},
		{"wrong input scale", `"scale_max": 0`, `"scale_max": 2`},
		{"wrong output scale", `"scale": 0`, `"scale": 2`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := strings.Replace(string(source), tc.from, tc.to, 1)
			if changed == string(source) {
				t.Fatalf("fixture no longer contains %q", tc.from)
			}
			spec, err := contract.Load(strings.NewReader(changed), config.DefaultLimits())
			if err != nil {
				t.Fatalf("mutation must still be a valid generic contract: %v", err)
			}
			if err := validateTransferRoute(servedRule{spec: spec}); diagnostic.CodeOf(err) != "E_SERVE_CONFIG" {
				t.Fatalf("bad transfer contract accepted, err=%v", err)
			}
		})
	}
}

func TestPlannedTransferDecodesExactRequest(t *testing.T) {
	limits := config.DefaultLimits()
	record := func(fields ...eval.RecordField) []eval.Value {
		return []eval.Value{eval.NewRecordValue(fields)}
	}
	id := eval.RecordField{Name: "request_id", Value: eval.StringValue("quote-001")}
	from := eval.RecordField{Name: "from_account", Value: eval.StringValue("A")}
	to := eval.RecordField{Name: "to_account", Value: eval.StringValue("B")}
	amount := eval.RecordField{Name: "amount", Value: plannedTestMoney(t, "KRW", "8784163844623596007")}
	got, err := transferRequestFromRecord(record(amount, to, id, from), limits)
	want := transfer.Request{ID: "quote-001", From: "A", To: "B", Currency: "KRW", Amount: 8784163844623596007}
	if err != nil || got != want {
		t.Fatalf("request=%+v err=%v, want %+v", got, err, want)
	}
	for name, args := range map[string][]eval.Value{
		"duplicate account": record(id, from, from, amount),
		"wrong account":     record(id, from, eval.RecordField{Name: "to_account", Value: amount.Value}, amount),
		"wrong currency":    record(id, from, to, eval.RecordField{Name: "amount", Value: plannedTestMoney(t, "USD", "100")}),
		"fractional won":    record(id, from, to, eval.RecordField{Name: "amount", Value: plannedTestMoney(t, "KRW", "1.01")}),
		"overflow":          record(id, from, to, eval.RecordField{Name: "amount", Value: plannedTestMoney(t, "KRW", "9223372036854775808")}),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := transferRequestFromRecord(args, limits)
			if diagnostic.CodeOf(err) != "E_CONTRACT_INPUT" {
				t.Fatalf("request error=%v", err)
			}
		})
	}
}

func TestPlannedTransferDecodesExactPlan(t *testing.T) {
	limits := config.DefaultLimits()
	debit := eval.RecordField{Name: "debit", Value: plannedTestMoney(t, "KRW", "10500")}
	credit := eval.RecordField{Name: "credit", Value: plannedTestMoney(t, "KRW", "10000")}
	fee := eval.RecordField{Name: "fee", Value: plannedTestMoney(t, "KRW", "500")}
	got, err := transferPlanFromRecord(eval.NewRecordValue([]eval.RecordField{fee, debit, credit}), "KRW", limits)
	if err != nil || got.Debit != 10500 || got.Credit != 10000 || got.Fee != 500 {
		t.Fatalf("plan=%+v err=%v", got, err)
	}
	for name, fields := range map[string][]eval.RecordField{
		"duplicate": {debit, debit, fee},
		"extra":     {debit, credit, fee, {Name: "adjustment", Value: fee.Value}},
		"currency":  {debit, credit, {Name: "fee", Value: plannedTestMoney(t, "USD", "500")}},
		"fraction":  {debit, credit, {Name: "fee", Value: plannedTestMoney(t, "KRW", "500.01")}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := transferPlanFromRecord(eval.NewRecordValue(fields), "KRW", limits)
			if !errors.Is(err, transfer.ErrPlan) {
				t.Fatalf("plan error=%v", err)
			}
		})
	}
}

func plannedTestMoney(t *testing.T, currency, text string) eval.Value {
	t.Helper()
	amount, err := decimal.Parse(text, config.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	value, err := money.New(currency, amount)
	if err != nil {
		t.Fatal(err)
	}
	return eval.MoneyValue{Value: value}
}
