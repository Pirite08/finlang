package eval

import (
	"testing"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
)

func TestBasicBuiltinValues(t *testing.T) {
	cases := []struct {
		source, want string
		kind         Kind
	}{
		{`abs(-2.35)`, "2.35", KindDecimal},
		{`abs(money("KRW",-2.35))`, "KRW 2.35", KindMoney},
		{`sign(money("USD",-0.01))`, "-1", KindDecimal},
		{`sign(-0)`, "0", KindDecimal},
		{`sign(0.0001)`, "1", KindDecimal},
		{`min(1.20,1.201,-0.03)`, "-0.03", KindDecimal},
		{`max(money("USD",-1),money("USD",0.01),money("USD",0))`, "USD 0.01", KindMoney},
		{`clamp(money("KRW",0.335),money("KRW",1),money("KRW",10))`, "KRW 1", KindMoney},
		{`clamp(10,1,2)`, "2", KindDecimal},
		{`clamp(1.23,1,2)`, "1.23", KindDecimal},
		{`sum(0.1,0.2,-0.3)`, "0", KindDecimal},
		{`sum(money("USD",0.335),money("USD",0.335),money("USD",0.335))`, "USD 1.005", KindMoney},
		{`sum(money("USD",0.335))`, "USD 0.335", KindMoney},
		{`floor(-2.5)`, "-3", KindDecimal},
		{`ceil(-2.5)`, "-2", KindDecimal},
		{`ceil(0.0001)`, "1", KindDecimal},
		{`trunc(-2.5)`, "-2", KindDecimal},
		{`decimal("-0012.30")`, "-12.3", KindDecimal},
		{`amount(money("USD",0.335))`, "0.335", KindDecimal},
		{`currency(money("KRW",0))`, "KRW", KindString},
	}
	for _, tc := range cases {
		t.Run(tc.source, func(t *testing.T) {
			got := evaluate(tc.source, config.DefaultLimits())
			if got.err != nil {
				t.Fatal(got.err)
			}
			if got.value.Kind() != tc.kind || got.value.Inspect() != tc.want {
				t.Fatalf("got %s %q; want %s %q", got.value.Kind(), got.value.Inspect(), tc.kind, tc.want)
			}
		})
	}
}

func TestBasicBuiltinErrors(t *testing.T) {
	cases := []struct {
		source string
		code   diagnostic.Code
	}{
		{`sum()`, "E_ARITY"},
		{`min()`, "E_ARITY"},
		{`max()`, "E_ARITY"},
		{`min("3")`, "E_TYPE_MISMATCH"},
		{`sum(true)`, "E_TYPE_MISMATCH"},
		{`sum(money("USD",1),2)`, "E_TYPE_MISMATCH"},
		{`sum(money("USD",1),money("KRW",2))`, "E_CURRENCY_MISMATCH"},
		{`max(money("USD",1),money("KRW",2))`, "E_CURRENCY_MISMATCH"},
		{`clamp(money("USD",-1),money("USD",1),money("KRW",2))`, "E_CURRENCY_MISMATCH"},
		{`clamp(1,2,0)`, "E_INVALID_RANGE"},
		{`floor(money("USD",0.9))`, "E_TYPE_MISMATCH"},
		{`decimal("1e3")`, "E_INVALID_NUMBER"},
		{`decimal(1)`, "E_TYPE_MISMATCH"},
		{`amount(1)`, "E_TYPE_MISMATCH"},
	}
	for _, tc := range cases {
		t.Run(tc.source, func(t *testing.T) {
			got := evaluate(tc.source, config.DefaultLimits())
			if diagnostic.CodeOf(got.err) != tc.code {
				t.Fatalf("got %v; want %s", got.err, tc.code)
			}
		})
	}
	limits := config.DefaultLimits()
	limits.MaxCoefficientDigits = 3
	got := evaluate(`sum(999,1,-1)`, limits)
	if diagnostic.CodeOf(got.err) != "E_NUMERIC_LIMIT" {
		t.Fatalf("intermediate sum bypassed numeric limits: %v", got.err)
	}
}

func TestBasicBuiltinTraceRecordsIntegerRounding(t *testing.T) {
	result, recorder := traced(`floor(-2.5)`, 10)
	if result.err != nil {
		t.Fatal(result.err)
	}
	for _, event := range recorder.Events() {
		if event.Kind == "floor" && event.Result == "-3" && event.Inexact &&
			event.TargetScale != nil && *event.TargetScale == 0 {
			return
		}
	}
	t.Fatalf("integer rounding missing from trace: %+v", recorder.Events())
}
