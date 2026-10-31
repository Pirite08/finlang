# Numeric lane review: v0.1 Tasks 2–4

Reviewed 2026-10-31 against `decimal-brief.md`, the numeric files in `v0.1-review-package.diff`, current `internal/decimal/**`, the Python generator, and its committed fixture.

## Verdict

- Specification compliance: accepted for the reviewed numeric lane with the default bounded limits. No actionable correctness defect found.
- Code quality: accepted. Ownership, normalization, scale alignment, positive-magnitude rounding and error propagation are small and locally auditable. No production/test edits requested.
- This is a scoped review, not a production-readiness or whole-interpreter verdict. Caller-supplied limits are trusted configuration, as documented in `decimal-report.md`.

## Checks supporting the verdict

- `decimal.go:17–21,29–47`: coefficient copies prevent caller/result aliasing; zero value behaves as zero; trailing fractional zeros are removed before coefficient/scale limits. Raw integer zeros remain significant to the nonnegative-scale representation.
- `parse.go:11–49`: original byte length is checked first; grammar permits optional minus, ASCII digits and an optional nonempty fraction; plus signs, exponent notation, whitespace and Unicode digits are rejected before `big.Int.SetString`.
- `arithmetic.go`: arithmetic uses exact integers with aligned scales, then normalizes the result. Intermediate coefficient growth is not prematurely rejected; multiplication scale can reach 72 safely under default limits. Comparison mutates only copies.
- `round.go:33–84`, `division.go:9–26`: all four modes and scale bounds are validated; zero divisor is checked before quotient calculation. Quotient/remainder use absolute magnitudes, HALF_EVEN tests quotient parity, HALF_UP ties increment magnitude, and sign is restored once. Inexact follows a nonzero remainder, including results rounded to zero. Division uses one integer scaling and one rounding step. Default division exponent lies in [-36,72].
- `format.go`: formatting pads zeros and rejects significant digit loss; it never invokes rounding.
- Reviewed tests cover requested signed ties, carry, zero, EXACT failures, normalization boundaries, double rounding, coefficient ownership, random rational arithmetic and fixture agreement. The rational admissibility helper independently factors the reduced denominator into powers of 2 and 5 rather than calling production normalization.

## Oracle review and fresh evidence

The generator uses Python standard-library Decimal strings and explicit contexts, not Go APIs. Its manual cases have literal expected values/errors asserted before random generation. Sticky Inexact across division and quantization correctly preserves information loss; EXACT traps Inexact rather than treating a rounded intermediate as exact.

Precision 512 has adequate margin for the fixed input bounds. For scaled division, the integer numerator has at most 200 digits and denominator at most 164; a nonzero distance to a half-integer boundary is at least 1/(2D). The quotient's rounding error at precision 512 is far below that bound. Terminating decimal quotients for these bounded operands also fit within 512 significant digits, so EXACT is not rejected merely by the reference precision.

Fresh review commands loaded the generator through `runpy.run_path` and called `build_cases()` in memory, without rewriting the fixture, and independently checked every fixture result with exact `fractions.Fraction` integer arithmetic:

- Regenerated case objects match the committed case objects exactly; all 41 manual assertions passed.
- 12,041 Fraction cross-checks: zero value, Inexact or error mismatches.
- Fixture SHA-256: `78de3a9e32db65a90492bb611670850b788674c97ae8fdcd3d598f3a1aa1b18e`.
- Fixture metadata: Python 3.13.3, seed 20261031, precision 512, 12,041 cases, 41 manual cases.
- Confirmed totals: 2,956 exact successes; 6,692 inexact successes; 2,246 E_INEXACT; 62 E_DIVISION_BY_ZERO; 85 E_NUMERIC_LIMIT.

The worker's Go suite, 30,000 rational arithmetic outcomes, race run, fuzz run and benchmarks were read in `decimal-report.md`; they were not rerun by this reviewer. Fresh evidence here concerns independent fixture integrity and mathematical review. Finite reference coverage does not establish exhaustive correctness.

## Findings

No actionable finding with a reproducer was identified. No production or test file was changed during review.
