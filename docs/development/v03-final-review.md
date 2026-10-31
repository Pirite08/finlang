# v0.3 final whole-branch review

Date: 2026-10-31. Reviewer: sole fresh final reviewer. Base: `bbe5eef` (v0.2.0). Reviewed final staged v0.3 implementation plus the coordinator's documentation corrections before publication; no commit/tag/remote operation was part of this review.

## Scope and verification

Read design sections 10–12, original implementation Tasks 13–14, execution decisions, the review diff, every new Go/COBOL source, relevant unchanged contracts/rules/output encoder, all new explanatory reports, README changes, and plan bookkeeping. The reviewer did not edit implementation/index/HEAD, dispatch another reviewer, or rerun broad tests, fuzzing, or benchmarks. This report is the reviewer's only write.

Independently inspected the existing evidence: all 14 JSONL source hashes equal the exact bytes returned by `git show :<path>`; 3,117 records comprise one header, 40 rejections, 3,072 cases, three rule summaries and one total summary. All totals reconcile: 3,072 matches, zero mismatches, 757 matching receiver errors and 54 matching literal anchors. All 28 benchmark groups and 140 raw measurements match the JSON samples, medians, minima, maxima and allocation summaries. The separate local 38-case COBOL preparation record was inspected to distinguish it from the durable differential corpus.

The coordinator's reported full regression (108 top-level / 491 including subtests, no failure or skip with `FINLANG_REQUIRE_COBOL=1`), vet/build and sustained fuzz results were not independently rerun. They remain coordinator execution evidence, not newly executed reviewer tests.

## Strengths

- Every reference preserves the specified packed input, exact intermediate and receiver widths. Interest retains up to 14 fractional places; sum settles only after all three terms. Receiver rounding and `ON SIZE ERROR` are explicit, including rounding-induced overflow and signed HALF_EVEN cases.
- The harness compares actual evaluation/encoding with compiled COBOL output. Fee branches come from execution trace, while canonical amount strings, status, errors and fixed currency contracts are checked separately. The 54 expected anchors are literal values independent of either implementation's rounding helper.
- Contract-rejected cases never enter subprocess stdin or the match denominator. Strict output parsing rejects missing/duplicate/reordered/malformed records. Compiler absence is a release failure when required; compiler/version/run failures, unexpected stderr and timeouts cannot turn into passing comparisons.
- Fixed seed, commands, environment, compiler identity and source hashes make the evidence inspectable. Canonical LF hashes match staged publication bytes. Compiler build C version and the actual selected C backend are distinguished in the final reports.
- Benchmarks separate parsing, prepared arithmetic and interpreter execution; work units are complete input lists, and float error is reported. No speed advantage is manufactured. The human evaluation is explicitly unrun, with the original human-data gate still unfulfilled; the user's narrowed actual-comparison deliverable is not confused with that gate.

## Issues

### Critical (Must Fix)

None.

### Important (Should Fix)

1. **Standalone PowerShell reproduction required an absent output directory — resolved during review.** Original locations: `README.md:237`, `docs/evaluation/compatibility.md:69`, `docs/development/v03-compat-report.md:33`; `tests/compat/cobol_test.go:257` opens the supplied path directly with `os.Create`. A fresh clone has no tracked `build` directory, so following the standalone comparison block with a working compiler fails before comparison. The coverage reproduction also writes into that directory. Cost: the published reproduction instructions fail for a normal fresh checkout. Fix: create `build` explicitly before these command blocks. The coordinator added `New-Item -ItemType Directory -Force build | Out-Null`; the reviewer inspected all four corrected blocks, including `docs/evaluation/v0.3-verification.md:30`. No runtime/evidence change was needed.

### Minor (Nice to Have)

1. **Tie-case examples and generation wording should identify the durable corpus precisely — resolved during review.** `docs/evaluation/compatibility.md:30` lists `±0.025` and `±0.035`, which belong to the separate local COBOL-only preparation batch, not the 3,072-case published differential corpus. That corpus instead includes `±1.005`, `±1.015` and `-0.005`. Line 49 also describes a boundary grid, while the implementation appends literal boundary anchors and fixed-seed random cases. Cost: readers could attribute preparation-only examples or a systematic grid to the published differential evidence. Fix: use the durable corpus's tie inputs and describe literal boundary anchors plus fixed-seed generated cases, or explicitly label the separate preparation batch. The coordinator changed the row to `±1.005, ±1.015, -0.005` and replaced boundary-grid wording with literal boundary anchors plus fixed-seed generation. The reviewer inspected both corrections. This was documentation precision, not a numeric defect.

## Recommendations

Both documentation corrections are applied; no unresolved findings remain. Preserve the current explicit limits on compatibility, benchmark interpretation and unrun human evaluation. No additional runtime changes or broad reruns are warranted for these documentation-only corrections.

## Declined to judge

- Defensive behavior for malformed, manually authored COBOL stdin: these references are specified to consume only validated host-contract inputs; the real output protocol and host rejection path were reviewed. They are not shipped as an independently validating user-facing COBOL CLI.

## Assessment

**Ready to merge? Yes.**

The actual three-rule comparison implementation and recorded evidence satisfy the user's narrowed v0.3 request. The reproduction blocker is corrected; no unresolved numeric, protocol, release-failure handling, evidence-hash or benchmark-boundary defect was found. The original participant-data completion gate remains explicitly open and must not be claimed complete.
