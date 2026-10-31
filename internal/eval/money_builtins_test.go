package eval

import (
	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"testing"
)

func TestMoneyExpressions(t *testing.T) {
	cases := []struct{ source, want string }{
		{"money(\"USD\",1.2)+money(\"USD\",2.03)", "USD 3.23"},
		{"money(\"KRW\",1)-money(\"KRW\",3)", "KRW -2"},
		{"money(\"KRW\",1000000)*0.05*0.5", "KRW 25000"},
		{"0.05*money(\"KRW\",1000000)", "KRW 50000"},
		{"div(money(\"USD\",1),3,2,\"HALF_EVEN\")", "USD 0.33"},
		{"settle(money(\"USD\",0.335),2,\"HALF_EVEN\")", "USD 0.34"},
		{"settle(money(\"USD\",-0.005),2,\"HALF_UP\")", "USD -0.01"},
		{"format_money(settle(money(\"USD\",1.005),2,\"HALF_EVEN\"),2)", "USD 1.00"},
		{"money(\"USD\",1.20)==money(\"USD\",1.2)", "true"},
		{"money(\"KRW\",1)<money(\"KRW\",2)", "true"},
	}
	for _, tc := range cases {
		t.Run(tc.source, func(t *testing.T) {
			result := evaluate(tc.source, config.DefaultLimits())
			if result.err != nil {
				t.Fatal(result.err)
			}
			if result.value.Inspect() != tc.want {
				t.Fatalf("got %s; want %s", result.value.Inspect(), tc.want)
			}
		})
	}
}

func TestMoneySettlementTiming(t *testing.T) {
	individually := "let x=money(\"USD\",0.335);let a=settle(x,2,\"HALF_EVEN\");format_money(a+a+a,2)"
	aggregate := "let x=money(\"USD\",0.335);format_money(settle(x+x+x,2,\"HALF_EVEN\"),2)"
	for _, tc := range []struct{ source, want string }{{individually, "USD 1.02"}, {aggregate, "USD 1.00"}} {
		got := evaluate(tc.source, config.DefaultLimits())
		if got.err != nil {
			t.Fatal(got.err)
		}
		if got.value.Inspect() != tc.want {
			t.Fatalf("got %s; want %s", got.value.Inspect(), tc.want)
		}
	}
}

func TestMoneyErrors(t *testing.T) {
	cases := []struct {
		source string
		code   diagnostic.Code
	}{
		{"money(\"KRW\",1)+money(\"USD\",1)", "E_CURRENCY_MISMATCH"},
		{"money(\"KRW\",1)==money(\"USD\",1)", "E_CURRENCY_MISMATCH"},
		{"money(\"KRW\",1)*money(\"KRW\",2)", "E_TYPE_MISMATCH"},
		{"money(\"KRW\",1)+1", "E_TYPE_MISMATCH"},
		{"1+money(\"KRW\",1)", "E_TYPE_MISMATCH"},
		{"div(money(\"KRW\",1),money(\"KRW\",2),2,\"HALF_EVEN\")", "E_TYPE_MISMATCH"},
		{"money(\"EUR\",1)", "E_UNSUPPORTED_CURRENCY"},
		{"money(1,2)", "E_TYPE_MISMATCH"},
		{"money(\"USD\",\"1\")", "E_TYPE_MISMATCH"},
		{"settle(money(\"USD\",1.005),2,\"EXACT\")", "E_INEXACT"},
		{"format_money(money(\"USD\",1.234),2)", "E_INEXACT"},
		{"div(money(\"USD\",1),0,2,\"HALF_EVEN\")", "E_DIVISION_BY_ZERO"},
		{"settle(money(\"USD\",1),37,\"HALF_EVEN\")", "E_NUMERIC_LIMIT"},
		{"round(money(\"USD\",1),2,\"HALF_EVEN\")", "E_TYPE_MISMATCH"},
	}
	for _, tc := range cases {
		got := evaluate(tc.source, config.DefaultLimits())
		if diagnostic.CodeOf(got.err) != tc.code {
			t.Fatalf("%s: got %v; want %s", tc.source, got.err, tc.code)
		}
	}
}
