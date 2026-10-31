# v0.3 compatibility harness implementation

Date: 2026-10-31. Scope: `tests/compat/*.go`; approved design sections 10–12 and implementation Task 13. The existing contracts, DSL programs and runtime are unchanged by this lane.

## Cases and independent checks

Each rule has exactly 1,024 distinct contract-accepted inputs. Generation uses `math/rand.New(rand.NewSource(20261031 + ruleIndex))`, with rule order `interest_v1`, `fee_v1`, `sum_v1`. Literal, manually calculated anchors precede generated cases. Random decimal inputs are built from integers and strings without float conversion or interpreter rounding helpers.

| Rule | Accepted | Manual anchors | Contract rejected |
|---|---:|---:|---:|
| interest_v1 | 1,024 | 16 | 16 |
| fee_v1 | 1,024 | 13 | 12 |
| sum_v1 | 1,024 | 25 | 12 |
| Total | 3,072 | 54 | 40 |

Interest anchors cover zero operands, fractional periods, before/at/after HALF_UP, the fourteen-place product, receiver maximum and rounding-induced overflow. Fee anchors cover the one-million boundary on both branches even where the output is the same, HALF_UP on both branches and maximum input. Sum anchors cover `0.335` three times, negative adjustments, positive and negative even/odd halfway values, negative zero, cancellation and both signs of receiver overflow.

Rejected cases include input range, raw input scale, exponent strings, wrong currency, JSON numbers, schema/rule mismatch, unknown and duplicate fields, missing parameters and malformed JSON. They are validated and counted separately. They are never appended to COBOL stdin or counted as executed matches.

DSL evaluation parses the actual example source, calls `Evaluator.Run` and `Evaluator.Call`, and reads successful output through `contract.EncodeResult`. Fee branch labels come from the actual boolean branch event in the evaluator trace. Result comparison requires exact status, error class, currency, fixed-scale amount string and branch. COBOL's protocol has no currency field, so its currency is assigned from the hashed fixed contract; the DSL currency is read from encoded output. This does not test a separate COBOL currency-conversion implementation.

## Subprocess and evidence behavior

`COBC` selects a compiler executable, defaulting to `cobc` on PATH. A missing compiler explicitly skips the actual comparison unless `FINLANG_REQUIRE_COBOL=1`, which fails it. Each reference is compiled once with direct arguments `-x -free -std=cobol2014 -freserved=NEAREST-EVEN -o <executable> <source>`. The explicit reserved-word extension preserves HALF_EVEN: GnuCOBOL 3.2's strict COBOL2014 word list omits the parser's `NEAREST-EVEN` token. The references use the synonymous `USAGE PACKED-DECIMAL` spelling for the specified COMP-3 fields because that strict word list also excludes the `COMP-3` alias. Root approved this dialect profile; field widths and rounding policies remain fixed. Version, compilation and execution all use `exec.CommandContext`, separate argument vectors and time limits. Compilation/run failures, timeout, unexpected stderr and malformed/missing/duplicate/out-of-order outputs fail the test.

`FINLANG_COMPAT_REPORT` may name an absolute JSONL output path. One sequential test owns the writer. The header records seed, rule order, Go/platform/compiler/version, compiler hash, selected toolchain environment, exact commands, and hashes of the contracts, DSL, COBOL and harness sources. Rejection records are followed by ordered executed cases, rule summaries and the overall summary. Each executed case includes its raw JSON input, arguments, DSL/COBOL outcomes, exact comparison and any independent manual anchor.

Run from the repository root after configuring the native compiler environment:

```powershell
New-Item -ItemType Directory -Force build | Out-Null
$env:COBC = '<absolute path to cobc.exe>'
$env:FINLANG_REQUIRE_COBOL = '1'
$env:FINLANG_COMPAT_REPORT = Join-Path (Get-Location) 'build/v0.3-compatibility.jsonl'
go test ./tests/compat -run TestCOBOLCompatibility -count=1 -v
```

The compiler's DLL path, configuration directory, C compiler and linker environment must be configured for that installation. The header records the environment used, rather than assuming Linux/WSL execution. Compilation uses the repository root as its working directory and a relative ASCII source argument. Native GnuCOBOL 3.2 rejects the equivalent absolute source path containing this checkout's Korean characters; the relative argument was verified to compile the unchanged original file. Executables use Go's temporary directory, which was an ASCII path on this host. A native installation with a non-ASCII temporary-directory path may need an ASCII `TEMP`/`TMP` location.

## Verification performed

- RED: a temporary always-equal comparator allowed six corrupt success outcomes and a wrong error class; `TestComparatorDetectsDifferences` failed on all seven. GREEN: structural string/field comparison passes the tests and rejects every mutation.
- RED: a temporary empty stream parser accepted twenty corrupt streams and failed six valid records. GREEN: strict protocol parsing passes all twenty rejection cases and six valid records.
- `go test ./tests/compat -count=1 -v`: package passes; actual COBOL comparison initially skipped explicitly because `cobc` was unavailable on PATH. All generator acceptance/determinism, 54 literal DSL anchors, 40 rejection cases and trace branch checks pass.
- `go test ./... -count=1`: all packages pass at this checkpoint; actual COBOL comparison remains skipped at that checkpoint.
- `go vet ./tests/compat`: passes.
- Deliberately missing `COBC` with `FINLANG_REQUIRE_COBOL=1`: fails rather than skips, as required for release verification.

## Actual native comparison

GnuCOBOL 3.2.0 (built Jul 31 2026; MinGW C 16.1.0) ran on Windows/amd64. The root-provided `build/v03-env.ps1` configured the native compiler and dependencies. `FINLANG_REQUIRE_COBOL=1 go test ./tests/compat -run '^TestCOBOLCompatibility$' -count=1 -v` passed with the dialect flags above.

| Rule | Executed | Matched | Mismatched | Matching range errors | Manual anchors | Separate rejections |
|---|---:|---:|---:|---:|---:|---:|
| interest_v1 | 1,024 | 1,024 | 0 | 414 | 16 | 16 |
| fee_v1 | 1,024 | 1,024 | 0 | 0 | 13 | 12 |
| sum_v1 | 1,024 | 1,024 | 0 | 343 | 25 | 12 |
| Total | 3,072 | 3,072 | 0 | 757 | 54 | 40 |

Fee branches were `low=511`, `high=513`, and all matched the actual DSL trace. Every manual anchor matched both implementations. The executed match rate for this case set was 100%; forty contract rejections are excluded from its denominator.

The successful first comparison evidence is `build/v0.3-compat-initial.jsonl`. The initial absolute-path failure is preserved separately in `build/v0.3-compat-absolute-path-failure.jsonl`: it contains compiler metadata and an infrastructure error before any case ran. The narrow relative-path probe compiled successfully before the harness changed its source argument, and then all three rules passed. Neither expected numeric outcomes nor reference formulas changed to make the run pass. The parent lane will generate the durable release evidence from the final sources.

After that run, `go test ./... -count=1` with the native environment and `FINLANG_REQUIRE_COBOL=1` passed for all packages, including the actual compatibility execution. This worker run completed before the parent's final verification freeze; the parent owns the fresh release run.

This test suite samples the specified educational profiles; it does not prove equivalence over every valid input, cover arbitrary COBOL programs, establish support for every GnuCOBOL dialect/platform, or establish usability results.
