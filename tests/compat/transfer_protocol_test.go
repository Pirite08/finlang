package compat

import (
	"reflect"
	"strings"
	"testing"
)

func TestCOBOLTransferProtocol(t *testing.T) {
	const accepted = "one|ok|KRW|500|10500|9500|12000|500\n" +
		"one|debit|A|KRW|-10500\n" +
		"one|credit|B|KRW|10000\n" +
		"one|fee|fee-KRW|KRW|500\n"
	const rejected = "two|insufficient_funds|USD|0|0|10|20|30\n"
	cases := []transferCase{{ID: "one"}, {ID: "two"}}
	got, err := parseCOBOLTransfers([]byte(accepted+rejected), cases)
	if err != nil {
		t.Fatal(err)
	}
	want := []transferOutcome{
		{Status: "ok", Currency: "KRW", Fee: 500, Debit: 10500,
			Final: transferBalances{From: 9500, To: 12000, Fee: 500},
			Postings: []transferPosting{
				{Kind: "debit", Account: "A", Currency: "KRW", Delta: -10500},
				{Kind: "credit", Account: "B", Currency: "KRW", Delta: 10000},
				{Kind: "fee", Account: "fee-KRW", Currency: "KRW", Delta: 500},
			}},
		{Status: "insufficient_funds", Currency: "USD", Final: transferBalances{From: 10, To: 20, Fee: 30}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("outcomes = %+v, want %+v", got, want)
	}
	for name, stream := range map[string]string{
		"unknown status":  strings.Replace(accepted, "|ok|", "|success|", 1) + rejected,
		"missing posting": strings.Replace(accepted, "one|fee|fee-KRW|KRW|500\n", "", 1) + rejected,
		"wrong posting":   strings.Replace(accepted, "|credit|", "|fee|", 1) + rejected,
		"wrong case ID":   strings.Replace(accepted, "one|credit|", "two|credit|", 1) + rejected,
		"fractional":      strings.Replace(accepted, "|-10500", "|-10500.0", 1) + rejected,
		"leading zero":    strings.Replace(accepted, "|9500|", "|09500|", 1) + rejected,
		"extra posting":   accepted + rejected + "two|fee|fee-USD|USD|0\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseCOBOLTransfers([]byte(stream), cases); err == nil {
				t.Fatalf("accepted malformed output %q", stream)
			}
		})
	}
}
