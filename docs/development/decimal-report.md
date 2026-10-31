# v0.1 decimal engine verification report

Verified on 2026-10-31 with Go 1.25.5, windows/amd64, Intel i5-11400, and Python 3.13.3. This report covers only the independently assigned numeric lane, Tasks 2–4 in `decimal-brief.md`.

## Delivered scope

- `internal/decimal`: immutable coefficient-plus-scale Decimal, input validation, canonical display, exact formatting, exact addition/subtraction/multiplication, comparison, explicit rounding and division, tests, benchmarks, and bounded parsing fuzz target.
- `experiments/generate_rounding_oracle.py`: an independent standard-library Python decimal oracle generator.
- `testdata/rounding_oracle.json`: 12,041 generated cases, including 41 literal, independently hand-checked cases first.
- This report. No shared configuration, diagnostics, module files, syntax/runtime files, or unrelated directories were edited. No commit was created.

All requested numeric API signatures and named modes are available without additions to the runtime dependency set. Runtime imports are standard-library packages plus the project's existing config/diagnostic packages. Python is a development tool only.

## Numeric contracts and implementation choices

`Decimal` stores a private `*big.Int` and private `uint32` scale. Its zero value behaves as canonical `0`. Construction copies the input with `big.Int.Set`; normalization and arithmetic operate on exclusively owned copies. No coefficient pointer or mutable numeric getter is exported. An ownership test also confirms normalization does not mutate the supplied integer, a later caller mutation cannot change the decimal, and the internal copying helper returns an independent integer.

Normalization removes fractional trailing zeros and clears signed zero before checking coefficient digits and scale. Default normalized limits are 128 coefficient digits and scale 36. Thus `1.` followed by 100 zeros is accepted and becomes `1`; a nonzero digit at fractional position 37 is rejected. Integer zeros are significant coefficient positions: a 129-digit integer is outside the limit even if it ends in zeros. The raw input byte limit is checked before parsing, so normalization cannot bypass the default 256-byte token limit.

Per root integration guidance, parsing accepts ASCII digits, an optional leading minus, and an optional fractional part with digits on both sides of the decimal point. A leading plus is rejected, as are whitespace, exponent syntax, grouping separators, Unicode digits and nonfinite values. `E_INVALID_NUMBER` denotes malformed syntax; a token or normalized-value limit failure is `E_NUMERIC_LIMIT`. Numeric input never passes through `float64`.

Addition and subtraction align scales using integer powers of ten. Multiplication computes the full exact integer product, then normalizes and checks limits. Intermediate coefficients may be larger than the output limit; the final normalized value determines admissibility. Comparison aligns values without rounding.

`String()` emits canonical decimal notation. `FormatAt` only pads zeros; discarding a significant fractional digit returns `E_INEXACT`. Requested scales above the supplied limit return `E_NUMERIC_LIMIT`.

Rounding and division use positive integer magnitudes and quotient/remainder. `HALF_EVEN` increments a tie only when the quotient is odd, `HALF_UP` increments a tie away from zero, `TOWARD_ZERO` discards the remainder, and `EXACT` rejects any nonzero remainder. Sign is applied only after rounding. `Inexact` records loss of nonzero information, including a rounded result of zero; it is false for merely adding display precision. On error, the returned `Rounded` is its zero value and should be ignored.

Division computes `e = targetScale + denominatorScale - numeratorScale` in a signed integer, multiplying the numerator by `10^e` if nonnegative and the denominator by `10^-e` otherwise. No preliminary decimal division or double rounding occurs. A zero divisor, including `0/0`, returns `E_DIVISION_BY_ZERO`. Unknown case-sensitive mode names return `E_ROUNDING_MODE`. For multiple simultaneous invalid arguments, mode validation precedes target-scale validation, which precedes zero-divisor validation. Limits are explicit trusted configuration; the package does not replace a zero Limits struct with defaults or impose an additional hidden global limit.

## Observed test-first cycles

Each production capability group was specified in tests before its implementation. Temporary unimplemented stubs were used to establish behavior-level red results after confirming the missing API. The stubs are absent from the final code. Supplemental ownership assertions were added after the core behavior already passed; they are additional verification, not claimed as an additional red-green cycle.

| Cycle | Red command and observed reason | Green result |
|---|---|---|
| Task 2 parsing/formatting | `go test ./internal/decimal -run 'TestParse\|TestFormat\|TestDecimalBoundaries'`: initially no non-test Go files; then temporary stubs failed canonical parse, token-limit, format and boundary expectations (`Parse("1.2"): E_INVALID_NUMBER: not implemented`) | Same command passed after implementation |
| Task 3 arithmetic/comparison | `go test ./internal/decimal -run 'TestArithmetic\|TestOperands\|TestRatAgreement'`: initially undefined Add/Sub/Mul/Cmp; temporary zero-return stubs then failed `0.1+0.2 = 0; want 0.3`, comparisons, boundaries and the first rational reference case | Same command with `-v` passed; 30,000 arithmetic comparisons completed |
| Task 4 modes/rounding/division | `go test ./internal/decimal -run 'TestSigned\|TestHalfEven\|TestRoundingValidation\|TestDoubleRounding\|TestDivision'`: stubs failed signed ties, half-even boundaries, mode recognition, exact errors, division values and double rounding | `go test ./internal/decimal -v` passed after implementation |
| Independent oracle | `go test ./internal/decimal -run TestRoundingOracleAgreement`: failed because `../../testdata/rounding_oracle.json` did not exist | After generating the independent fixture, the same test with `-v` passed all cases; no production correction was needed |

The targeted examples include `-2.5` in all rounding modes, positive/negative half-even ties, a carry from `9.995` to `10`, exact terminating division, repeating division, zero divisors, target scale 0 and 36, coefficient overflow, multiplication scale normalization from 37 to 36, and distinct double-rounded/direct results (`1.2451 → 1.245 → 1.24`, directly `1.25`). Operand immutability is checked across arithmetic, rounding and division.

## Independent reference checks

### Rational arithmetic

`TestRatAgreement` generates 10,000 numeric pairs with Go's local PRNG seed `20261031`, both signs, up to 128 coefficient digits and scales 0–36. Each pair is compared for all three exact operations against `math/big.Rat`, giving 30,000 arithmetic outcomes. Rational admissibility is derived independently by factoring the reduced denominator into powers of 2 and 5 and reconstructing the minimal base-ten coefficient; it does not call production Decimal normalization. Successful results are compared numerically as rationals, and out-of-range results must specifically produce `E_NUMERIC_LIMIT`. Comparisons are also checked against rational ordering on admissible addition cases.

Observed counts: **20,835 exact successes; 9,165 intended range errors**. Both categories must be nonempty for the test to pass.

### Python rounding/division oracle

The Python generator parses original strings directly with `decimal.Decimal`, uses precision **512**, explicit `ROUND_HALF_EVEN`, `ROUND_HALF_UP` and `ROUND_DOWN`, and traps `decimal.Inexact` for EXACT mode. Division and quantization share sticky Inexact flags so a repeating quotient cannot appear exact merely because its intermediate approximation is quantized. Zero division is a separate expected error. The fixed input sizes are far below the precision used for reference computation. Go operations or their rounding helpers are never called by the generator.

The 41 manual cases assert literal values, inexact flags and error codes inside the generator before random generation. The Go test reads them before random cases and checks fixture count, seed, Python version and precision. The fixture combines 6,000 seeded round cases and 6,000 seeded division cases, including deliberately constructed signed halfway/carry cases, endpoints 0/36, exact results, exact failures and numeric-limit outputs.

| Operation | Exact success | Inexact success | E_INEXACT | E_DIVISION_BY_ZERO | E_NUMERIC_LIMIT | Total |
|---|---:|---:|---:|---:|---:|---:|
| round | 2,491 | 2,652 | 882 | 0 | 0 | 6,025 |
| div | 465 | 4,040 | 1,364 | 62 | 85 | 6,016 |
| Total | 2,956 | 6,692 | 2,246 | 62 | 85 | **12,041** |

Fixture size: 4,300,824 bytes. SHA-256: `78DE3A9E32DB65A90492BB611670850B788674C97AE8FDCD3D598F3A1AA1B18E`. Regenerating on the same Python version produced the identical hash. Metadata records Python version, seed, context precision/exponent bounds, explicit rounding map, EXACT trap and sticky-Inexact policy. A different Python version will intentionally alter its version metadata, even if the numeric cases are identical.

## Final verification commands and results

- `python experiments/generate_rounding_oracle.py`: passed all 41 manual assertions and generated 12,041 cases. Repeated once for deterministic hash comparison, which passed.
- `go test ./internal/decimal -count=1 -v`: passed all numeric tests, the 30,000 rational outcomes, 12,041 oracle cases, ownership tests and all seven fuzz seeds.
- `go vet ./internal/decimal`: passed with no diagnostics.
- `go test ./internal/decimal -race -count=1`: passed (`4.513s` reported package time); no race diagnostics.
- `go test ./internal/decimal -run '^$' -fuzz '^FuzzDecimalParse$' -fuzztime=5s -parallel=2`: passed **338,823 fuzz executions**, 21 newly interesting cases, 28 total corpus entries. Inputs longer than 512 bytes are skipped; production parsing still enforces the 256-byte token limit. This is a bounded run, not an exhaustive proof.
- `go test ./internal/decimal -run '^$' -bench . -benchmem -benchtime=200ms`: passed all eight benchmarks.
- `go test ./...`: attempted during concurrent implementation. Numeric/config/diagnostic/experiments/lexer/parser packages passed. The suite was not globally green at that point: `internal/eval` had no non-test Go files, producing build failures in `internal/eval`, `internal/cli`, `internal/repl`, and `cmd/fin`. Those packages are outside this lane and were still being implemented by root. Root must provide the final integrated suite result; this report does not claim integrated completion.

### Benchmark sample

These are single local measurements on a shared development machine, not performance guarantees or an optimization claim. Setup parsing is excluded from operation timing via `testing.B.Loop`.

| Workload | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| Parse ordinary money-like value | 482.8 | 168 | 10 |
| Parse 128 coefficient digits | 1,879 | 1,040 | 14 |
| Add with a 30-place scale gap | 951.8 | 784 | 18 |
| Multiply long exact decimals | 645.5 | 616 | 12 |
| Compare with a 30-place scale gap | 396.0 | 328 | 10 |
| Round long fraction to two places | 1,055 | 744 | 18 |
| Terminating EXACT division | 1,012 | 544 | 16 |
| Recurring division to 36 places | 1,077 | 840 | 20 |

## Remaining limits and concerns

No unresolved numeric mismatch, failing numeric test or race was observed. The reference and fuzz checks are finite and do not establish correctness for every possible input. The API trusts caller-supplied Limits; default bounded behavior depends on callers using the shared configured limits. Formatting intentionally has no rounding policy. The 4.3 MB oracle is a development fixture, not a runtime data dependency. Final interpreter integration and source-location behavior are outside the owned numeric lane and remain for root's integrated validation.
