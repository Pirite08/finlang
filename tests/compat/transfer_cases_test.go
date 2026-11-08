package compat

import (
	"math"
	"strings"

	"github.com/Pirite08/finlang/internal/transfer"
)

// All amounts are in minor units (KRW won or USD cents). Expected fees and
// final balances are manually specified, independently of both implementations.
func transferCases() []transferCase {
	standard := transferBalances{From: 20000, To: 2000, Fee: 0}
	rounding := []struct {
		name        string
		amount, fee int64
		final       transferBalances
	}{
		{"normal", 10000, 500, transferBalances{From: 9500, To: 12000, Fee: 500}},
		{"zero-tie", 10, 0, transferBalances{From: 19990, To: 2010, Fee: 0}},
		{"odd-below-tie", 29, 1, transferBalances{From: 19970, To: 2029, Fee: 1}},
		{"odd-tie", 30, 2, transferBalances{From: 19968, To: 2030, Fee: 2}},
		{"odd-above-tie", 31, 2, transferBalances{From: 19967, To: 2031, Fee: 2}},
		{"even-below-tie", 49, 2, transferBalances{From: 19949, To: 2049, Fee: 2}},
		{"even-tie", 50, 2, transferBalances{From: 19948, To: 2050, Fee: 2}},
		{"even-above-tie", 51, 3, transferBalances{From: 19946, To: 2051, Fee: 3}},
	}
	var cases []transferCase
	for _, currency := range []string{"KRW", "USD"} {
		for _, row := range rounding {
			cases = append(cases, successfulTransferCase(row.name, currency, row.amount, row.fee, standard, row.final))
		}
	}

	const maxBalance = math.MaxInt64
	cases = append(cases,
		successfulTransferCase("exact-funds", "KRW", 10000, 500,
			transferBalances{From: 10500, To: 2000, Fee: 100},
			transferBalances{From: 0, To: 12000, Fee: 600}),
		rejectedTransferCase("one-unit-short", "KRW", 10000, "insufficient_funds",
			transferBalances{From: 10499, To: 2000, Fee: 100}),
		rejectedTransferCase("principal-only", "USD", 10000, "insufficient_funds",
			transferBalances{From: 10000, To: 2000, Fee: 100}),
		rejectedTransferCase("principal-short", "USD", 10000, "insufficient_funds",
			transferBalances{From: 9999, To: 2000, Fee: 100}),
		successfulTransferCase("max-total-debit", "KRW", 8_784_163_844_623_596_007, 439_208_192_231_179_800,
			transferBalances{From: maxBalance},
			transferBalances{From: 0, To: 8_784_163_844_623_596_007, Fee: 439_208_192_231_179_800}),
		rejectedTransferCase("total-debit-overflow", "USD", 8_784_163_844_623_596_008, "invalid",
			transferBalances{From: maxBalance}),
		rejectedTransferCase("destination-overflow", "KRW", 10, "invalid",
			transferBalances{From: 10, To: maxBalance - 9, Fee: 100}),
		rejectedTransferCase("fee-account-overflow", "USD", 11, "invalid",
			transferBalances{From: 12, To: 2000, Fee: maxBalance}),
		rejectedTransferCase("insufficient-before-credit-overflow", "KRW", 11, "insufficient_funds",
			transferBalances{From: 11, To: maxBalance, Fee: 100}),
		successfulTransferCase("max-credit-balances", "USD", 11, 1,
			transferBalances{From: 12, To: maxBalance - 11, Fee: maxBalance - 1},
			transferBalances{From: 0, To: maxBalance, Fee: maxBalance}),
	)
	return cases
}

func successfulTransferCase(name, currency string, amount, fee int64, initial, final transferBalances) transferCase {
	request := transferCaseRequest(name, currency, amount)
	debit := amount + fee
	return transferCase{
		ID: request.ID, Request: request, Initial: initial,
		Want: transferOutcome{
			Status: "ok", Currency: currency, Fee: fee, Debit: debit, Final: final,
			Postings: []transferPosting{
				{Kind: "debit", Account: request.From, Currency: currency, Delta: -debit},
				{Kind: "credit", Account: request.To, Currency: currency, Delta: amount},
				{Kind: "fee", Account: "fee-" + currency, Currency: currency, Delta: fee},
			},
		},
	}
}

func rejectedTransferCase(name, currency string, amount int64, status string, initial transferBalances) transferCase {
	request := transferCaseRequest(name, currency, amount)
	return transferCase{
		ID: request.ID, Request: request, Initial: initial,
		Want: transferOutcome{Status: status, Currency: currency, Final: initial},
	}
}

func transferCaseRequest(name, currency string, amount int64) transfer.Request {
	request := transfer.Request{
		ID: strings.ToLower(currency) + "-" + name, From: "A", To: "B",
		Currency: currency, Amount: amount,
	}
	if currency == "USD" {
		request.From, request.To = "U1", "U2"
	}
	return request
}
