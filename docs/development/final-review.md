# v0.1 whole-project review

Reviewed 2026-10-31. Scope: the complete additions in `v0.1-review-package.diff`, current production files and tests, the approved design's v0.1 scope, implementation-plan Tasks 1–9, and the numeric/syntax/CLI lane reports. Money, JSON contracts, trace recording, COBOL compatibility and operational finance readiness are outside this review.

## Verdicts

- **Spec compliance: changes requested for one host I/O error-handling gap.** The requested Decimal, syntax, evaluation, function, bounded-input and ordinary CLI/REPL behavior is present. A failed output write can currently be reported as success or as a language error, contrary to the documented host I/O contract.
- **Code quality: changes requested for the same P2 finding.** No additional important numeric, parser, closure, resource-limit or oracle-independence defect was found. The implementation is small and respects the requested module boundaries. This verdict does not introduce requirements for deferred features.

## Finding 1 — P2: propagate output failures as fatal host I/O

Primary location: `internal/repl/repl.go:50–56`. Related locations: `internal/cli/cli.go:39–45`, `internal/cli/cli.go:85–87`, and `internal/eval/builtins.go:33–34`.

`print` correctly reports a failed writer as `E_OUTPUT_IO`, but the REPL puts that error through its recoverable source-error path, writes a diagnostic, continues consuming input, and ultimately returns nil. Consequently the CLI returns success even though requested output was not delivered. In file mode the same error is unconditionally mapped to exit 1 instead of the documented I/O exit 2. Help and version output discard the writer result and also return success on write failure.

This occurs with ordinary broken output pipes or failed redirected output, not only with a deliberately malformed writer. A bare REPL expression already returns its writer error, so replacing `1` with `print(1)` unexpectedly changes failure handling.

The following targeted probe was executed from the workspace on Windows. No project source or test files were changed:

```python
import subprocess
for args, source in [
    ([], b'print(1)\nlet still_executed=2;\n'),
    (['-version'], b''),
    (['-file', 'examples/decimal.fin'], b''),
]:
    p = subprocess.Popen(
        ['go', 'run', './cmd/fin', *args],
        stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    p.stdout.close()
    p.stdin.write(source)
    p.stdin.close()
    diagnostic = p.stderr.read().decode()
    print(args, p.wait(), diagnostic)
```

Observed results:

| Invocation | Observed result |
| --- | --- |
| REPL, `print(1)` | Exit 0; stderr contains `<repl:1>:1:1: E_OUTPUT_IO: write /dev/stdout: The pipe is being closed.` |
| `-version` | Exit 0; no diagnostic |
| `-file examples/decimal.fin` | `go run` exits 1; its stderr explicitly reports interpreter `exit status 1`, after the located `E_OUTPUT_IO` diagnostic |

The `go run` wrapper itself maps nonzero child exits; use `cli.Run` tests or a built executable to assert the corrected child exit code 2. Classify `E_OUTPUT_IO` separately from recoverable source/evaluation errors, return it from the REPL, map it to exit 2 in file mode, and check the help/version write result. Add focused failing-writer coverage for those paths, including stopping before the next REPL statement. Existing `TestJSONPrintGuardAndOutputError` only exercises the JSON print guard; it does not currently supply a failing writer.

## Reviewed interactions and evidence

### Decimal ownership and semantics

The coefficient is private. Construction and arithmetic copy through `big.Int.Set`; arithmetic receivers operate on private copies. Normalization removes only fractional trailing zeros and clears negative zero before the result-limit check. Exact addition/subtraction/multiplication align or combine scales without passing through binary floating point. Scale and digit overflow are diagnosed after normalization, including products whose raw scale exceeds the final allowed scale. The zero-value Decimal works through the same public operations.

Round and Div share quotient/remainder rounding on positive magnitudes and restore sign afterward. HALF_EVEN, HALF_UP, TOWARD_ZERO and EXACT match the specified rules; division scales its integers before the single rounding step. FormatAt pads exactly and rejects significant discarded places. Scale arguments are validated before allocation. Default CLI/evaluator limits constrain operands and intermediate growth; arbitrary host-provided limits are trusted configuration, as documented in the numeric report.

### Independent numerical references

`TestRatAgreement` forms reference arithmetic from the original strings using `big.Rat`, and independently derives output admissibility from the rational denominator's factors of 2 and 5. It does not obtain the expected result through Decimal arithmetic or normalization. The fixed-seed 10,000-pair test distinguishes valid arithmetic from expected range errors.

The Python fixture generator uses Python decimal, original string inputs, explicit rounding modes, 512-digit context, an Inexact trap for EXACT, and sticky division/quantization flags. It does not call the Go implementation or share its rounding helper. The limited source operand/scale sizes make the reference precision conservative for these cases. Manual assertions precede random generation. The Go fixture test compares values and Inexact on success and diagnostic categories on failure.

The review independently checked the current fixture hash: `78DE3A9E32DB65A90492BB611670850B788674C97AE8FDCD3D598F3A1AA1B18E`, matching the numeric report. The previously reported 30,000 rational outcomes and 12,041 oracle cases were not rerun during this review.

### Parser, evaluation and closure state

Numeric text is preserved by the UTF-8 lexer; identifier/string/number limits and source limits are enforced. Pratt precedence and left associativity match the approved grammar. Both recursive parse depth and generated tree height are bounded, including left-associated and repeated-call chains. Slash diagnostics cannot become silent default division.

Evaluation uses strict type checks and does not add automatic conversion or string concatenation. Functions capture their declaration environment; nested blocks create child environments; function parameters and body bindings share the call frame. Same-environment duplicate declarations are rejected before evaluating their initializer. Return signals propagate through selected branches, missing returns are diagnosed, and inner error locations survive wrapping. Function calls share the evaluator's node budget and active-call-depth limit, with depth restored on error. REPL lines preserve the environment but use a fresh evaluator budget.

The design explicitly specifies environment references, so later declarations in a captured environment are not treated as a snapshot-capture defect. The language has no transaction/rollback promise for already-executed statements preceding a REPL error. Neither behavior is flagged as a scope change.

### Host boundaries and scope

File reads use a limit of maximum source bytes plus one. REPL lines are read and oversized lines drained in bounded chunks; source errors permit later lines. Ordinary file/read/usage errors and source-location diagnostics are covered in existing tests. The finding above concerns the missing distinction between output I/O failure and a recoverable language error.

Runtime imports use the Go standard library and internal project packages. The trace event/Sink types are placeholders for the planned later recorder, and no missing recorder, Money, JSON contract, COBOL comparison or broader performance study is counted as a v0.1 defect. Single-run benchmarks are clearly labeled exploratory and not evidence of general speed superiority.

## Verification provenance and limits

The CLI lane reports `go test ./... -count=1`, `go vet ./...`, build and example runs passing on the reviewed code. Numeric and syntax reports document their own targeted tests, finite fuzz runs and race checks, and distinguish their earlier integration failures from later success. Root's evaluator red phase was a missing-implementation compile failure; this review does not relabel it as a behavioral assertion failure.

Those full suites were intentionally not rerun in this review. New execution was limited to the three failing-output probes above, plus a read-only fixture hash check. No production/test files were edited and no agents were spawned. This report records the pre-fix review; the P2 finding requires implementation and focused regression verification before the verdict becomes approval.
