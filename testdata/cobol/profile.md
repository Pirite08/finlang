# edu-packed-v1

This versioned educational comparison profile is implemented by `interest_v1.cob`, `fee_v1.cob` and `sum_v1.cob`. It is specific to the tested GnuCOBOL configuration and is not an IBM COBOL or bank-system compatibility claim.

## Exact physical fields

Every field below uses `USAGE PACKED-DECIMAL`, the standard spelling of the packed-decimal storage described as `COMP-3` in the original plan. No sign, digit count, scale, arithmetic storage boundary or receiver range was changed.

| Rule | Business input PICTURE | Exact intermediate PICTURE | Final receiver PICTURE | Receiver rounding |
| --- | --- | --- | --- | --- |
| interest_v1 | principal `S9(9)V99`; annual_rate `9V9(6)`; years `99V9(6)` | `S9(15)V9(14)` | `S9(9)` | `NEAREST-AWAY-FROM-ZERO` / HALF_UP |
| fee_v1 | amount `S9(9)V99` | `S9(15)V9(14)` | `S9(9)` | `NEAREST-AWAY-FROM-ZERO` / HALF_UP |
| sum_v1 | each of a, b, c `S9(9)V9(3)` | `S9(15)V9(14)` | `S9(9)V99` | `NEAREST-EVEN` / HALF_EVEN |

The unchanged JSON contracts define accepted signs, scales and inclusive input/output ranges. The host checks them before sending decimal strings. Input conversion, exact intermediate computation and receiver computation each have `ON SIZE ERROR`. A size error produces `E_LEGACY_RANGE` without a numeric amount. The references contain no division.

## Compiler profile and actual spelling adaptations

The verified compiler is the official MSYS2 UCRT64 GnuCOBOL package `1~3.2-10`; `cobc --version` reports `GnuCOBOL 3.2.0`, built July 31, 2026. Compile each program once using these same flags:

```text
cobc -x -free -std=cobol2014 -freserved=NEAREST-EVEN -o <executable> <source>
```

The initial `COMP-3` declarations failed under this compiler's strict COBOL2014 word list. A minimal probe confirmed that even `USAGE COMP-3` is rejected as a keyword from another dialect; `USAGE PACKED-DECIMAL` compiles. The original plan's spelling is therefore adapted explicitly in this versioned profile.

The same installed COBOL2014 word list omits `NEAREST-EVEN` even though GnuCOBOL's parser implements it. Without the extra reserved-word flag, sum failed to compile at that mode. `-freserved=NEAREST-EVEN` enables that existing keyword while preserving the mode, the `-std=cobol2014` setting and every numeric field. This profile claims the tested GnuCOBOL 3.2 configuration, not unqualified pure COBOL2014 conformance. See the [compiler option documentation](https://gnucobol.sourceforge.io/doc/gnucobol.html) and [rounding guide](https://gnucobol.sourceforge.io/HTML/gnucobpg.html#ROUNDED).

## Wire protocol

Standard input begins with a case count, then exactly that many newline-terminated records:

```text
id|principal|annual_rate|years
id|amount
id|a|b|c
```

The three shapes correspond to interest, fee and sum. IDs in the comparison corpus are ASCII, at most 128 characters, and contain no `|` or newline. Each decimal token is at most 256 characters. Malformed wire records are outside this contract; rejected JSON inputs are counted by the host and do not enter COBOL.

Each case produces exactly one standard-output line and no prompt:

```text
id|ok|amount|branch
id|error|E_LEGACY_RANGE|branch
```

Interest/sum use `none`. Fee uses `low` for an input at or below 1,000,000 and `high` above it. Successful integer amounts have no decimal suffix; sum always emits two fractional digits. Zero has no minus sign. Each process exits normally after the declared count, including a count of zero.

Windows native `cobc` rejected an absolute source path containing Korean characters with `Illegal byte sequence`. Compiling with a relative source path worked. The host harness may stage identical source bytes in an ASCII temporary directory; its evidence must retain the original hash and actual command.
