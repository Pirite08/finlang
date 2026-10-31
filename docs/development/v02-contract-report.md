# v0.2 JSON contract implementation report

Implemented the Task 11 contract boundary and the three educational rule fixtures. Public entry points are `Load`, `DecodeInput`, `EncodeResult`, and `EncodeError`; exported metadata fields follow the contract brief. No dependencies beyond the Go standard library were added.

`Load` returns `E_CONTRACT` for invalid metadata. `DecodeInput` consistently returns `E_CONTRACT_INPUT` for malformed JSON, wrapper mismatches, missing/unknown inputs, wrong types/currencies, numeric syntax/limits, raw fractional scale, and inclusive input range failures. Neither function calls the evaluator. Arguments retain `parameters` order, regardless of input object order.

The shared JSON scanner reads at most `MaxSourceBytes + 1` bytes, validates raw UTF-8, uses `json.Decoder.UseNumber`, enforces `MaxSyntaxDepth`, and rejects duplicate decoded keys at every object level. Thus escaped-equivalent keys are duplicates. Structural validation then rejects unknown/missing fields in the root, parameter, output and input Money objects. A second JSON document is rejected; final whitespace is accepted.

Success requires Money in the declared output currency, an amount inside the inclusive receiving range, and exact formatting at the declared scale. The corresponding failures are `E_CONTRACT_OUTPUT`, `E_LEGACY_RANGE`, and `E_OUTPUT_SCALE`. All validation completes before the writer receives any success bytes. No output adapter rounding occurs. `EncodeError` emits a single error record, unwraps diagnostic errors, and emits null file/line/column when no span exists. Both encoders return writer errors unchanged.

The scale-18 test-only contract preserves `0.100000000000000001` and signed Money `-12.34567890123456789` exactly. Production `interest_v1` separately enforces rate/period scale 6 and principal scale 2. Fixture sources implement `main`, Money arithmetic and explicit final `settle`: interest uses HALF_UP at 0, progressive fee uses HALF_UP at 0, and aggregate three-item USD sum uses HALF_EVEN at 2.

## Red / green evidence

Commands were run from the repository root in PowerShell on 2026-10-31.

1. Added metadata/strict scanner tests before implementation. `go test ./internal/contract` failed with `undefined: Load`. After implementing metadata and scanner, the same command passed (`ok .../internal/contract 0.649s`).
2. Added input/output tests before those implementations. `go test ./internal/contract` failed with `undefined: DecodeInput`, `undefined: EncodeResult`, `undefined: EncodeError`, plus the pending evaluator `eval.MoneyValue` integration dependency. After the implementations, only `eval.MoneyValue` remained undefined. No evaluator stub was introduced.
3. Once the root agent supplied real `eval.MoneyValue`, `go test ./internal/contract` passed (`0.843s`).
4. Added source/schema/input fixtures and exercised the real parser, one shared evaluator's `Run` and `Call`, and result encoding. `go test ./internal/contract ./internal/cli` passed (`contract 0.890s`; CLI cached). `go test ./...` passed for every package at that point, with no failures.

Coverage includes escaped-equivalent and nested duplicate keys; unknown/missing fields; JSON numeric amount/bounds; null/fractional scale metadata; bad schema/type/currency/name/range; byte/depth/UTF-8 limits; raw trailing-zero scale rejection; no trailing document; exact output padding; output type/currency/precision/range; no success bytes on validation errors; wrapped diagnostic span and null span encoding; writer failures; actual rule fixture results `25000`, `1500`, `1.00`; interest zero/half/overflow/rate-scale; fee below/equal/above threshold and half-up; negative/near-zero sum and receiver overflow.

## Scope and limits

Only contract code, the three contract/source/input fixtures and this report were changed by this worker. JSON CLI stdout/stderr/exit behavior belongs to the root agent's CLI task. This worker's passing CLI suite was the current shared checkout at the verification time; root-side CLI changes may continue afterward. `EncodeResult` has no limits argument, so it uses the project's default numeric limits for output range parsing and exact formatting. The normal call sequence is `Load` before `DecodeInput`/`EncodeResult`; loaded metadata is fully validated. No known contract test failures or unresolved dependency blockers remain.
