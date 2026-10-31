# v0.2 strict JSON contract review

Reviewed on 2026-10-31 against `v02-contract-brief.md`, the staged review package, current `internal/contract`, contracts, example sources, input fixtures, and `v02-contract-report.md`. Module path: `github.com/Pirite08/finlang`.

**Spec verdict: pass for the reviewed contract boundary.** No concrete Task 11 defect was found in the contract implementation or supplied fixtures.

**Quality verdict: acceptable for the stated educational v0.2 scope.** The scanner, metadata validation, input conversion, and result adapter have separate responsibilities and reuse the exact Decimal/Money implementation. No style-only blocker is raised. This is not a production-readiness assessment.

## Checked behavior

- `json.go:16-43` bounds the read to the configured byte cap plus one byte, rejects invalid raw UTF-8, selects `UseNumber`, and requires EOF after one JSON value. `scanValue` counts object/array depth and rejects duplicate decoded object keys at every level, including escaped-equivalent keys. `object` rejects unknown and missing fields.
- `contract.go:72-168` validates metadata types, version, names, duplicate parameter names, supported currencies, inclusive bounds, and integer scale metadata. `json.go:138-148` does not accept null, fractional, exponent, string, negative, or out-of-range scale values.
- `decode.go:15-93` checks the wrapper and complete inputs object, then appends arguments in `Spec.Parameters` order. Amounts and rates must be strings and pass directly to `decimal.Parse`; the production contract path contains no float conversion. The raw fractional length is checked before normalization, so trailing zeros cannot bypass `scale_max`.
- `encode.go:16-45` checks Money type/currency, inclusive receiving range, and exact output formatting before constructing/writing success JSON. Range and scale errors cannot include a normal amount. `EncodeError` unwraps diagnostic errors and preserves a real span; locationless errors emit null file/line/column.
- `io_test.go` contains the scale-18 signed Money/rate round trip, reversed input-object order, strict malformed input cases, output failure/no-bytes checks, writer failures, and wrapped/null locations. `contract_test.go` contains strict metadata and resource-limit cases. `fixtures_test.go` invokes the real parser and one evaluator's `Run` and `Call`, with all three checked-in contracts and inputs; it covers the documented `25000`, `1500`, and `1.00` results and representative rounding/range boundaries.

## Findings and verification limits

No actionable contract finding is reported. The normal API sequence is `Load` followed by `DecodeInput`/`EncodeResult`; arbitrary caller-constructed or subsequently mutated `Spec` values are outside the fully validated metadata path. `EncodeResult` uses default numeric limits because its required API has no limits argument; this matches the CLI's default-limit execution and is disclosed in the implementation report.

The CLI option-order defect affecting a parse error before a later `-json` flag is already recorded in `v02-final-review.md`; this review adds no separate duplicate finding. CLI pre-reading also reports oversized contract/input files as `E_SOURCE_LIMIT` before the contract adapter, while direct adapters use `E_CONTRACT`/`E_CONTRACT_INPUT`. The brief requires rejection and exit 1 but does not require a specific error code for this CLI pre-read case, so this is not treated as a defect.

No source or test was edited, and no suite was rerun. The coordinator supplied the integrated full-suite pass; this review inspected implementation and existing test assertions and did not find a bug requiring a targeted reproducer.
