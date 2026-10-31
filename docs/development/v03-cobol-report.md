# v0.3 COBOL reference implementation record

Date: 2026-10-31. This record covers the three education-only reference rules, not a bank program or another COBOL compiler's behavior.

## Contract and implementation

The declarations follow the unchanged physical field profile in design section 10. Every business input, the exact `S9(15)V9(14)` intermediate and the final signed receiver uses `USAGE PACKED-DECIMAL` (the standard spelling for the planned `COMP-3` storage). Interest and fee receive into `S9(9)` with explicit `NEAREST-AWAY-FROM-ZERO`; sum receives into `S9(9)V99` with explicit `NEAREST-EVEN`. Each input conversion and each intermediate/final computation includes `ON SIZE ERROR`. An arithmetic size error emits `E_LEGACY_RANGE` without an amount. The exact versioned declarations and compiler adaptation are recorded in [edu-packed-v1](../../testdata/cobol/profile.md).

The explicit modes and the placement of `ROUNDED`/`ON SIZE ERROR` were checked against the [GnuCOBOL Programmer's Guide](https://gnucobol.sourceforge.io/HTML/gnucobpg.html#ROUNDED). Runtime anchors below remain the evidence for this installed compiler's actual behavior.

Only host-contract-accepted decimal strings enter these programs. Contract rejection and malformed protocol handling are the host's responsibility. The references do not reinterpret rejected strings as valid cases. The exact intermediate fields retain all input decimal places: interest needs at most 14, fee at most 6, and sum at most 3. The numeric-edited output fields format a previously settled receiver; they do not perform another settlement.

Input begins with a count and then exactly that many case lines. Interest consumes `id|principal|annual_rate|years`; fee consumes `id|amount`; sum consumes `id|a|b|c`. Output is `id|ok|amount|branch` or `id|error|E_LEGACY_RANGE|branch`. Interest/sum use `none`; fee uses `low` for `amount <= 1000000` and `high` otherwise. Integer output omits a decimal suffix; sum always emits two decimal places and emits zero without a minus sign.

## RED evidence before implementation

The following read-only check ran before adding any reference source:

```powershell
$referenceSources = @('testdata/cobol/interest_v1.cob', 'testdata/cobol/fee_v1.cob', 'testdata/cobol/sum_v1.cob')
foreach ($referenceSource in $referenceSources) {
    if (-not (Test-Path -LiteralPath $referenceSource)) {
        Write-Output "RED: required COBOL source absent: $referenceSource"
    }
}
Get-Command cobc -ErrorAction SilentlyContinue
```

The three source checks each reported `RED: required COBOL source absent`. `Get-Command cobc` returned no compiler and the command exited 1. This is absence evidence, not a falsely reported arithmetic test failure.

## Actual toolchain failures and resolution

The official native MSYS2 UCRT64 package `mingw-w64-ucrt-x86_64-gnucobol`, version `1~3.2-10`, was extracted into `C:/Users/user/.cache/finlang/gnucobol-3.2/ucrt64`. This leaves the global MSYS2 installation unchanged. Initial `cobc --version` terminated with Windows status `0xC0000139` (entry point not found).

Import/export inspection using the existing `C:/msys64/ucrt64/bin/objdump.exe -p` isolated two mismatches: `libcob-4.dll` requires `nanosleep64`, absent in the old global `libwinpthread-1.dll`; the old global `libncursesw6.dll` requires `DllMain` from `libintl-8.dll`, which the new gettext runtime does not export. The following official packages were verified against published SHA-256 values and extracted into the same portable prefix:

| Package | SHA-256 |
| --- | --- |
| [libwinpthread 14.0.0.r569.g34eeafb7d-1](https://packages.msys2.org/packages/mingw-w64-ucrt-x86_64-libwinpthread) | `b4ebe2826e848c5e790e06ea7b37d3e0594e005f42382b0506886f17df8127da` |
| [ncurses 6.6-4](https://packages.msys2.org/packages/mingw-w64-ucrt-x86_64-ncurses) | `88331fe78470ef2c5e0280652fee5e10b6dcaa35f8037725c359407f2702309c` |

After those changes, `cobc --version` exited 0 and reported `cobc (GnuCOBOL) 3.2.0`, built `Jul 31 2026 17:21:33`, with build C compiler `(MinGW) 16.1.0`. The C backend explicitly selected for these reference builds was the existing `gcc.exe (Rev3, Built by MSYS2 project) 13.2.0`. Host: `Windows-10-10.0.19045-SP0`, `AMD64`.

Actual source compilation exposed two syntax differences before any numeric comparison:

1. Under `-std=cobol2014`, `COMP-3` is not a reserved word. Initial source compilation failed at every packed field. A minimal syntax probe returned 1 for `COMP-3`, returned 1 for `USAGE COMP-3` with the diagnostic that it is reserved in another dialect, and returned 0 for `USAGE PACKED-DECIMAL`. All fields now use the latter spelling; physical fields are unchanged.
2. The installed `cobol2014.words` omits `NEAREST-EVEN`, although the parser implements it. Sum therefore failed at its explicit mode. Adding the documented `-freserved=NEAREST-EVEN` keyword option compiled it successfully. The mode was preserved and the same option is used for all three sources.

These are compiler/profile adaptations, not numeric mismatches hidden by changing expected amounts. The versioned profile is `edu-packed-v1`.

## Actual compiler and anchor execution

The environment used for the final builds was:

```powershell
$prefix = 'C:/Users/user/.cache/finlang/gnucobol-3.2/ucrt64'
$env:PATH = "$prefix/bin;C:/msys64/ucrt64/bin;$env:PATH"
$env:COB_CONFIG_DIR = "$prefix/share/gnucobol/config"
$env:COB_COPY_DIR = "$prefix/share/gnucobol/copy"
$env:COB_CC = 'C:/msys64/ucrt64/bin/gcc.exe'
$env:COB_CFLAGS = "-I$prefix/include"
$env:COB_LDFLAGS = "-L$prefix/lib"
$env:COB_LIBS = '-lcob'
```

Python `subprocess.run` received an argument list directly; no shell command was assembled. The working directory was the repository root. These were the final compilation arguments, with `cobc` resolved to `$prefix/bin/cobc.exe`:

```text
cobc -x -free -std=cobol2014 -freserved=NEAREST-EVEN -o build/toolchain/cobol-anchors/interest_v1.exe testdata/cobol/interest_v1.cob
cobc -x -free -std=cobol2014 -freserved=NEAREST-EVEN -o build/toolchain/cobol-anchors/fee_v1.exe testdata/cobol/fee_v1.cob
cobc -x -free -std=cobol2014 -freserved=NEAREST-EVEN -o build/toolchain/cobol-anchors/sum_v1.exe testdata/cobol/sum_v1.cob
```

Each final compilation exited 0 with empty stdout/stderr. Each executable was then run once with its complete manual batch; the 38 literal expected results all matched byte-for-byte after splitting newline records. Interest had 10 cases, fee 9, and sum 19. Every run exited 0 with empty stderr. The local raw record is `build/toolchain/cobol-anchors/manual-anchors.json` (a generated build artifact; the durable differential harness records its own evidence).

| Rule | Manual coverage | Observed result |
| --- | --- | --- |
| interest | Zero; 1,000,000 × 0.05 × 0.5; positive HALF_UP tie; subunit result; exact receiver maximum; 999999999.49 below carry; 999999999.50 carry overflow; maximum principal × 1 × 10; full 14-place product; 256-character accepted token with leading zeroes | 10/10 matched, including 25,000 and both intended range errors |
| fee | Zero; 499.99 below tie; 500 tie; boundary −0.01/equal/+0.01; 1001000 tie; 1002000; maximum input | 9/9 matched, including 1000 at all three boundary values, correct low/high branches, 1001 at 1001000/1002000 and 500500 at maximum input |
| sum | 0.335 three times; negative adjustment; zero and negative input zero; ±0.005, ±0.015, ±0.025, ±0.035; ±999999999.994; ±999999999.995 carry overflow; positive/negative aggregate overflow; large cancellation | 19/19 matched, including 1.00, canonical 0.00, signed even ties and intended range errors |

An additional zero-count batch followed by an extra ignored line was run against each executable. All three exited 0 with empty stdout/stderr, confirming no prompt and no output beyond the declared count.

Source hashes at manual execution:

| Source | SHA-256 |
| --- | --- |
| interest_v1.cob | `e1018759621a838639003d86b84276ee84c564e9313039ccacc871d683f0adba` |
| fee_v1.cob | `34753d01fe7b96f3da94c40658910db4a08dd8bbd21f9fbd8a41e8e5a4dac479` |
| sum_v1.cob | `639703b97a9a71e8d7391654b57cc134f943320471f665d0e477aa0b6afe657e` |

The manual execution demonstrates these anchors on this native compiler. The separate differential harness supplies the larger fixed-seed comparison and original/staged hashes. Neither proves equivalence over every possible input or validates real bank rules.

Publication normalization: the final source files use LF, matching Git text normalization. The hash table identifies these published bytes; the coordinator reran all 3,072 COBOL cases on the LF files before publication. This changes no COBOL statements or numeric fields.
