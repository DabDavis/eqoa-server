#!/usr/bin/env python3
"""The wizard's hate-lowering line as the GM Jibwiz asked in petition 1949 (2026-11-13): "Verify the agro reducing spell
line for wizard, for example stupify, actually removes threat. It should be about equal to removing 1 sorcerer MCA nuke
of threat".

Measured (internal/entity/stupefy_test.go, the live records, a level 60 wizard of DEX 620, Jibwiz's, on a level 60
creature): the line does take hate off, but Stupefy took 399 where one Sorcerer nuke (the Fury: Icy, Flaming or
Thunderous, 1455 + 21.39 a level + 40% DEX and its second hit on the target 856 + 31.37 a level + 40% DEX) puts on
5,797. So Stupefy (700735) and Stupefy + (901747) take off the Fury's own figures, both hits as one: 2311 + 52.76 a
level + 80% DEX (5,972 at that wizard: 103% of the nuke's hate, as the nuke's own con cut is not on a hate figure).
The rest of the line keeps its share of Stupefy's old 400 of that, its shape as it was:

  spellid  spell        was                       now
  1630     Concussion   -25                       6.25%: -144, 3.30 a level, 5% DEX
  700153   Impact       -100, -100 (two effects)  25% each: -578, 13.19 a level, 20% DEX
  700509   Shock        -300 (lr)                 75%: -1733, 39.57 a level, 60% DEX (lr kept)
  901717   Shock +      -300 (lr)                 as Shock
  700735   Stupefy      -400                      -2311, 52.76 a level, 80% DEX
  901747   Stupefy +    -400                      as Stupefy

spelleffect.effects is rewritten (each hate effect's amount, level_pct, scale and scale_pct) and its line says the new
figure. Refused, nothing written, unless each record's effects are as above ("was"). Report only by default; --apply
in one transaction, a gm_audit line a row, log stupefy_1949_log; --undo exact. The spells are read at the server's
start (and at a GM panel edit of spelleffect).

    python3 -E -s tools/stupefy_1949_20261113.py --db eqoa.db [--apply | --undo]
"""
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "stupefy_1949_log"
DETAIL = "The wizard's hate line as one Sorcerer nuke's hate (the GM Jibwiz's petition 1949): tools/stupefy_1949_20261113.py"

# Stupefy's whole: the Fury's two hits as one
FULL = {"amount": -2311, "level_pct": 5276, "scale_pct": 80}
# spellid: (the old amounts, the share of FULL each effect now takes, the line's words)
LINE = {
    1630: ([-25], 0.0625),
    700153: ([-100, -100], 0.25),
    700509: ([-300], 0.75),
    901717: ([-300], 0.75),
    700735: ([-400], 1.0),
    901747: ([-400], 1.0),
}


def figures(share):
    return {"amount": round(FULL["amount"] * share), "level_pct": round(FULL["level_pct"] * share),
            "scale": "dex", "scale_pct": round(FULL["scale_pct"] * share)}


def line_of(share, n):
    f = figures(share)
    one = f"Decrease Hate by {-f['amount']} ; Increase {f['scale_pct']}% based on DEX"
    return "Target, " + ", ".join([one] * n)


def changes(db):
    out, lines, refusals = [], [], []
    done = 0
    for sid, (was, share) in LINE.items():
        r = tr.get(db, "spelleffect", {"spellid": sid})
        if r is None:
            refusals.append(f"spelleffect {sid}: no row")
            continue
        try:
            eff = json.loads(r["effects"])
        except ValueError:
            refusals.append(f"spelleffect {sid}: effects not JSON")
            continue
        hates = [e for e in eff.get("effects", []) if e.get("kind") == "hate"]
        want = figures(share)
        if [e.get("amount") for e in hates] == [want["amount"]] * len(was) and \
                all(e.get("level_pct") == want["level_pct"] and e.get("scale_pct") == want["scale_pct"] for e in hates):
            done += 1
            continue
        if [e.get("amount") for e in hates] != was or any(e.get("level_pct") or e.get("scale_pct") for e in hates):
            refusals.append(f"spelleffect {sid}: hate effects {hates}, not {was}")
            continue
        for e in hates:
            e.update(want)
        new = {**r, "effects": json.dumps(eff, separators=(",", ":")), "line": line_of(share, len(hates))}
        out.append(("spelleffect", {"spellid": sid}, new))
        lines.append(f"spelleffect {sid}: {r['line']!r} -> {new['line']!r} (each: {want})")
    return out, lines, refusals, done == len(LINE)


SPEC = {"log": LOG, "detail": DETAIL, "keys": {"spelleffect": ("spellid",)}, "needs": ("spelleffect",), "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
