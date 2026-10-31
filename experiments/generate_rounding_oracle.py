#!/usr/bin/env python3
"""Generate an independent standard-library decimal oracle; no Go code is used.

Run from any directory: python experiments/generate_rounding_oracle.py
The random seed and case order are stable. Python's version is recorded so that
the oracle's provenance remains inspectable when regenerated on another host.
"""

import decimal
import json
from pathlib import Path
import platform
import random

SEED = 20261031
PRECISION = 512
MODES = {
    "HALF_EVEN": decimal.ROUND_HALF_EVEN,
    "HALF_UP": decimal.ROUND_HALF_UP,
    "TOWARD_ZERO": decimal.ROUND_DOWN,
    "EXACT": decimal.ROUND_DOWN,
}


def canonical(value):
    if value.is_zero():
        return "0"
    text = format(value, "f")
    if "." in text:
        text = text.rstrip("0").rstrip(".")
    return text


def result_within_limits(value):
    text = canonical(value).lstrip("-")
    integer, _, fraction = text.partition(".")
    coefficient = (integer + fraction).lstrip("0")
    return len(coefficient) <= 128 and len(fraction) <= 36


def evaluate(case):
    # Parse strings directly: binary floating-point never enters the oracle.
    a = decimal.Decimal(case["a"])
    b = decimal.Decimal(case["b"]) if case["op"] == "div" else None
    if b is not None and b.is_zero():
        return {"error": "E_DIVISION_BY_ZERO", "inexact": False}
    context = decimal.Context(prec=PRECISION, rounding=MODES[case["mode"]])
    context.traps[decimal.Inexact] = case["mode"] == "EXACT"
    context.clear_flags()
    try:
        with decimal.localcontext(context) as active:
            value = a if b is None else active.divide(a, b)
            quantum = decimal.Decimal((0, (1,), -case["digits"]))
            value = active.quantize(value, quantum)
            inexact = bool(active.flags[decimal.Inexact])
    except decimal.Inexact:
        return {"error": "E_INEXACT", "inexact": False}
    if not result_within_limits(value):
        return {"error": "E_NUMERIC_LIMIT", "inexact": False}
    return {"value": canonical(value), "inexact": inexact, "error": ""}


def build_cases():
    cases = []

    def add(op, a, b, digits, mode, manual=False, expected=None):
        case = {"id": f"{'manual' if manual else 'random'}-{len(cases):05d}",
                "op": op, "a": a, "b": b, "digits": digits, "mode": mode,
                "manual": manual}
        result = evaluate(case)
        if expected is not None:
            assert result == expected, (case, result, expected)
        case.update(result)
        cases.append(case)

    def hand(op, a, b, digits, mode, value=None, inexact=False, error=""):
        expected = {"inexact": inexact, "error": error}
        if not error:
            expected["value"] = value
        add(op, a, b, digits, mode, manual=True, expected=expected)

    for mode, positive, negative in [
        ("HALF_EVEN", "2", "-2"), ("HALF_UP", "3", "-3"),
        ("TOWARD_ZERO", "2", "-2"),
    ]:
        hand("round", "2.5", "", 0, mode, positive, True)
        hand("round", "-2.5", "", 0, mode, negative, True)
    for a, wanted in [("2.345", "2.34"), ("2.355", "2.36"),
                       ("-2.345", "-2.34"), ("9.995", "10"),
                       ("-9.995", "-10"), ("0.005", "0"),
                       ("-0.005", "0"), ("1.2451", "1.25"),
                       ("1.245", "1.24")]:
        hand("round", a, "", 2, "HALF_EVEN", wanted, True)
    for mode in MODES:
        hand("round", "1.20", "", 36, mode, "1.2")
        hand("round", "-0", "", 0, mode, "0")
        hand("div", "0", "3", 36, mode, "0")
    hand("round", "2.5", "", 0, "EXACT", error="E_INEXACT")
    hand("round", "0." + "0" * 35 + "1", "", 36, "EXACT",
         "0." + "0" * 35 + "1")
    hand("div", "1", "8", 3, "EXACT", "0.125")
    hand("div", "-2", "3", 2, "HALF_EVEN", "-0.67", True)
    hand("div", "1", "3", 2, "EXACT", error="E_INEXACT")
    hand("div", "1", "0", 0, "HALF_UP", error="E_DIVISION_BY_ZERO")
    hand("div", "0", "0", 36, "EXACT", error="E_DIVISION_BY_ZERO")
    hand("div", "1", "2", 0, "HALF_EVEN", "0", True)
    hand("div", "1", "2", 0, "HALF_UP", "1", True)
    hand("div", "-1", "2", 0, "TOWARD_ZERO", "0", True)
    hand("div", "0.001", "0.2", 2, "HALF_EVEN", "0", True)
    hand("div", "2", "0.001", 0, "EXACT", "2000")
    hand("div", "9" * 128, "0.1", 0, "EXACT", error="E_NUMERIC_LIMIT")
    hand("div", "9" * 128, "1", 36, "EXACT", "9" * 128)

    rng = random.Random(SEED)

    def random_text():
        count = rng.randrange(1, 129)
        digits = str(rng.randrange(1, 10)) + "".join(
            str(rng.randrange(10)) for _ in range(count - 1))
        scale = rng.randrange(37)
        if scale:
            digits = digits.zfill(scale + 1)
            digits = digits[:-scale] + "." + digits[-scale:]
        return ("-" if rng.randrange(2) else "") + digits

    names = tuple(MODES)
    for i in range(6000):
        a = random_text()
        target = (0, 36, rng.randrange(37))[i % 3]
        # Deliberately construct exact halfway and carry cases as well as
        # broadly sampled coefficients, preserving the scale bound.
        if i % 5 == 0:
            target = rng.randrange(36)
            a = ("-" if rng.randrange(2) else "") + str(rng.randrange(10000))
            a += "." + "9" * target + "5"
        add("round", a, "", target, names[i % 4])
    for i in range(6000):
        a, b = random_text(), random_text()
        if i % 101 == 0:
            b = "0"
        elif i % 7 == 0:
            b = rng.choice(["2", "8", "0.001", "0.2", "-0.00001", "-5"])
        target = (0, 36, rng.randrange(37))[i % 3]
        add("div", a, b, target, names[i % 4])
    return cases


def main():
    cases = build_cases()
    payload = {
        "metadata": {
            "generator": "experiments/generate_rounding_oracle.py",
            "python_version": platform.python_version(), "seed": SEED,
            "context": {"precision": PRECISION, "Emin": -999999,
                        "Emax": 999999, "rounding_by_mode": MODES,
                        "EXACT_traps": ["Inexact"],
                        "inexact": "sticky Inexact from division and quantize"},
            "case_count": len(cases),
            "manual_count": sum(c["manual"] for c in cases),
            "runtime_dependency": "none; Python is only a fixture generation tool",
        },
        "cases": cases,
    }
    output = Path(__file__).resolve().parents[1] / "testdata" / "rounding_oracle.json"
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"wrote {len(cases)} cases ({payload['metadata']['manual_count']} manually checked) to {output}")


if __name__ == "__main__":
    main()
