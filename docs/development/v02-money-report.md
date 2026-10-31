# v0.2 Money module implementation

Owned files: `internal/money/money.go`, `internal/money/money_test.go`, and this report. No evaluator, Decimal, config, contract, or trace files were modified by this worker. No commits were created.

## Behavior

The pure Money API from Task 10 is implemented with private currency and amount fields. `New` accepts only exact, case-sensitive `KRW` and `USD` codes and preserves sub-unit amounts. It validates the Decimal against the default 128 coefficient-digit / scale-36 limits through exact Decimal addition with zero; it does not round. Invalid or zero-value Money is rejected by every operation with `E_UNSUPPORTED_CURRENCY`. A properly constructed zero amount is valid.

Same-currency `Add`, `Sub`, and `Cmp` preserve currency and return `E_CURRENCY_MISMATCH` for different currencies. `Mul` takes a dimensionless Decimal. `Div` and `Settle` delegate explicit scale and mode handling to the unchanged Decimal API and return the Decimal `Inexact` flag. Error returns use an empty Money and `false` flag. `FormatAt` returns the currency prefix, a space, and exact zero-padded Decimal output; insufficient places produce `E_INEXACT`.

The implementation introduces no conversion, currency inference, hidden KRW/USD scale, or mutable exposed amount. Operations use the supplied Decimal limits. Runtime rejection of Money*Money, Money+Decimal, and Money/Money belongs to evaluator integration; the pure Go API uses concrete argument types and does not provide those operations.

## TDD evidence

Tests were added before the implementation. The initial exact command `go test ./internal/money` exited 1 because the package had no non-test Go file. API scaffolds were then added and the same command was rerun to obtain observable behavioral RED rather than relying on a compilation failure:

```text
--- FAIL: TestCurrencyRules
    got  0, <nil>; want USD 1
--- FAIL: TestInvalidMoneyRejected
    error = <nil>; want E_UNSUPPORTED_CURRENCY
--- FAIL: TestSettlementTiming
    got  0, <nil>; want USD 0.34
--- FAIL: TestMoneyRounding
    got  0, <nil>; want USD -1
--- FAIL: TestMoneyDivision
    got  0, <nil>; want KRW 0.33
--- FAIL: TestMoneyFormat
    FormatAt = "",<nil>; want "USD 1.00"
--- FAIL: TestMoneyLimits
    error = <nil>; want E_NUMERIC_LIMIT
FAIL example.com/finlang/internal/money
```

After implementing the Money wrappers, `gofmt -w internal/money/money.go internal/money/money_test.go` completed successfully and `go test ./internal/money` exited 0:

```text
ok example.com/finlang/internal/money 0.819s
```

The tests use actual Money and Decimal operations with hand-derived expected values. They cover USD 0.335 individual settlement (0.34 * 3 = 1.02), aggregate settlement (0.335 * 3 = 1.005 -> 1.00), negative HALF_EVEN and HALF_UP ties, TOWARD_ZERO, exact results and EXACT failures, division by zero, unsupported modes and currencies, cross-currency errors, invalid Money on either side of binary operations, valid zero amounts, exact formatting, immutable operands and Decimal aliases, coefficient overflow, the scale-36 boundary, invalid scale 37 / uint32 maximum, and tighter caller-provided limits.

## Verification

Commands and observed results:

| Command | Result |
|---|---|
| `go test ./internal/money` | PASS after implementation |
| `go test -race ./internal/money` | PASS, `ok .../internal/money 1.538s` |
| `go test ./internal/money -count=1` | PASS, `ok .../internal/money 0.455s` |
| `go vet ./internal/money` | PASS, no output |
| `go test ./internal/money ./internal/eval` | PASS after evaluator integration appeared; Money 0.462s, eval 0.481s |
| `go test ./...` (initial integration snapshot) | FAIL during concurrent integration; Money passed |
| `go test ./...` (subsequent run) | PASS for every package after concurrent integrations appeared |

The full-suite run was a shared-workspace snapshot while other workers were still implementing. All failures observed in that run were reported to the root agent:

- `internal/diagnostic`: `TestSpanJSONContract` expected snake_case source-location JSON tags but saw Go field names.
- `internal/eval`: `TestMoneyExpressions` (all ten subcases), `TestMoneySettlementTiming`, and `TestMoneyErrors` saw `E_UNDEFINED_NAME` because `money`, `settle`, and `format_money` were not wired at that snapshot.
- `internal/trace`: `TestWriteJSONLDeterministicSchema` expected snake_case config/source-location tags but saw Go field names.

These external files remained under their respective owners. A subsequent `go test ./...` exited 0 across all packages, including diagnostic, eval, and trace. The Money package has no unresolved known failure. The root agent should verify again after its final edits.
