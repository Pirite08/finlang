# v0.2 integration review

Reviewed 2026-10-31 against baseline `8cc70ba`, the supplied review diff, design sections 5–9 and 12, and implementation-plan Tasks 10–12. This is a read-only source review; no production source or tests were changed. COBOL comparison and production financial certification are outside this review.

## Verdict

- **Spec compliance: one correction required.** Money, explicit settlement, contract execution, and bounded actual-execution traces implement the intended v0.2 boundaries. A CLI parse error can break the promised single JSON error record when `-json` occurs after the invalid option.
- **Code quality: sound module separation with one actionable CLI edge defect.** Decimal arithmetic is reused rather than duplicated. Money and retained trace events preserve ownership. CLI orchestration keeps output validation and trace-file success ahead of JSON success publication. The separate strict-contract review remains authoritative for detailed JSON parser findings.

## Finding

### [P2] JSON usage errors depend on option order

Location: `internal/cli/cli.go:45-46` (`flags.Parse` error handling), with emission controlled by `c.json` in `command.fail`.

`flag.Parse` stops at the first invalid option. If `-json` appears later, the boolean has not yet been assigned, so `fail` emits only stderr and leaves stdout empty. The same invalid invocation with `-json` first emits the required JSON error record. This makes machine-readable failure handling depend on flag order and violates the Task 11 / progress-ledger single-record output contract.

Targeted live probe:

```powershell
go run ./cmd/fin -unknown -json
# stdout: empty

go run ./cmd/fin -json -unknown
# stdout: one status="error", code="E_CLI_USAGE" JSON record
```

Both paths select CLI exit 2 in source. `go run` itself reports exit 1 when its child exits 2; the probe captured stdout separately and did not mistake the wrapper's status for the interpreter's status.

Suggested correction: resolve structured-error output intent even if normal option parsing fails, while honoring boolean forms, flag values, and the `--` terminator; add regression cases for invalid options both before and after `-json`. A naive substring search over argument values would misinterpret a filename equal to `-json`.

## Reviewed behavior

- Money creation preserves sub-unit amounts and has no currency-dependent rounding. Same-currency add/subtract/compare, scalar multiplication in either order, and explicit Money division match the type rules. Cross-currency and mixed-type rejection is present.
- Signed settlement delegates to the established Decimal rounding engine. `format_money` formats exactly and does not settle implicitly. Existing Money tests cover negative ties, EXACT failures, limits, immutable operands, and the required 1.02 versus 1.00 example.
- Evaluator events are emitted from values actually computed and from the chosen branch. Disabled sinks return before formatting. Recorder copies slices and target-scale pointers on both insertion and snapshot access. Caps discard subsequent events and set truncation without changing evaluation.
- JSON execution uses one evaluator for source evaluation and entry invocation, retaining a shared execution budget. Contract output is buffered and validated before publication. Requested trace-write failure prevents success publication. A failed final stdout write does not append a second JSON record.
- Trace destinations are checked against source/contract/input through absolute paths, Windows case folding, and existing-file identity; the check is repeated immediately before opening the destination. This is ordinary overwrite prevention, not a claim of race-proof hostile-filesystem protection.
- Runtime diagnostics retain inner source positions through `diagnostic.WithSpan`; locationless contract errors remain null in JSON. Existing source/evaluator tests and implementation were inspected.
- README descriptions of numeric limits, explicit settlement, trace truncation, language scope, historical v0.1 oracle counts, and deferred COBOL work match implementation. At review time its line 201 link to `docs/evaluation/v0.2-verification.md` did not yet exist; the coordinator was notified to supply the planned report before release.

## Evidence boundaries

The coordinator reported a fresh successful `go test ./... -count=1` after the module-path migration. Worker red/green, race, and vet evidence was read from the Money/trace reports and progress ledger. This review did not redundantly rerun those suites. Its only new execution was the two-order CLI failure probe above. Existing trace tests cover immutable snapshots, deterministic JSONL, I/O/short-write failure, and cap behavior; CLI tests cover real rules, single result/error records on successfully parsed invocations, path collision, and a 10,000-event cap. Final verification after any fixes remains the coordinator's responsibility.
