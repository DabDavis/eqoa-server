#!/usr/bin/env python3
"""Heirloom petition 2885 (Mightyeagle, 2026-11-24, Spell or ability): "the cl aoe undead cm nuke mods are not quite
right. most of them have 60% wis on both target and aoe atm, but some have 30% on both ... according to prima the
target is 60 but aoe is only 30%."

The cleric's Turn Undead mastery spells (Turn, Disable, Harm, Destroy, Annihilate Undead: levels 20-60) are target
area nukes: spelleffect.effects holds an "hp" part (the target's hit) and an "hp2" part (the area's hit, which the
target takes too: spelljson.go), each with its own scale_pct read by spellAmount (cast.go). The import gave both
parts the scale of whichever of Prima's two rows came first: 30/30 on Turn and Harm, 60/60 on the other three.

Prima's eGuide (Dec 2004), Cleric Abilities, pp. 400-402 (PDF 401-403), prints two rows each: the target row "Wis 60"
(radius 0) and the area row "Wis 30" (radius 20), with exactly our amounts (284/148, 450/236, 834/436, 1,166/608,
1,368/712). So each hp part goes to 60, each hp2 part to 30. The mastery table (p. 38) gives the same 2:1 ratio
(30wis / 15wis on the older, smaller figures the descriptions still quoted). The same pages print Persecute Death
(p. 402, level 44) and Torture Death (p. 402, level 55), the merchant undead nukes, at Wis 60, as the rest of the
Death line: both go 30 -> 60.

Lines (spelleffect.line, read only when effects is empty) and the spell book descriptions (spellpattern.spelldesc,
which quoted the mastery table's 160/88 at 30%/15%) say what the spell now does. Amounts, radius, timings unchanged.

Both shards' rows are the same: mirrored. Report only by default; --apply in one transaction, a gm_audit line a row,
log petpass_both_1124ar_log; --undo exact. Spells are read at the next cast; descriptions at the next login.

    python3 -E -s tools/petpass_both_1124ar.py --db eqoa.db [--apply | --undo]
"""
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_both_1124ar_log"
DETAIL = ("Heirloom petition 2885: the cleric's Turn Undead mastery nukes' target 60% WIS and area 30% WIS, Persecute "
          "and Torture Death 60% WIS, as Prima (tools/petpass_both_1124ar.py)")
NOTE = "; petition 2885 (2026-11-24): WIS scale target 60 / area 30 as Prima pp. 400-402"
NOTE1 = "; petition 2885 (2026-11-24): WIS scale 60 as Prima p. 402"

# spellid: (name, target amount, area amount, page) -- amounts as the rows hold them, checked
AREA = {
    700902: ("Turn Undead", -284, -148, 400),
    700943: ("Disable Undead", -450, -236, 401),
    700989: ("Harm Undead", -834, -436, 401),
    701041: ("Destroy Undead", -1166, -608, 402),
    701101: ("Annihilate Undead", -1368, -712, 402),
}
SINGLE = {
    337: ("Persecute Death", -992),
    700738: ("Torture Death", -1488),
}
TARGET_PCT, AREA_PCT, SINGLE_PCT = 60, 30, 60


def area_line(t, a):
    return (f"Target undead, Decrease Hitpoints by {-t} ; Increase {TARGET_PCT}% based on WIS aoe Decrease Hitpoints "
            f"by {-a} ; Increase {AREA_PCT}% based on WIS")


def area_desc(t, a):
    return (f"Target undead, Decrease HP by {-t} ; Increase {TARGET_PCT}% based on WIS, AOE Decrease HP by {-a} ; "
            f"Increase {AREA_PCT}% based on WIS")


def single_line(t):
    return f"Target undead, Decrease Hitpoints by {-t} ; Increase {SINGLE_PCT}% based on WIS"


def changes(db):
    out, lines, refusals = [], [], []
    for sid, spec in list(AREA.items()) + list(SINGLE.items()):
        name = spec[0]
        key = {"spellid": sid}
        p = tr.get(db, "spellpattern", key)
        r = tr.get(db, "spelleffect", key)
        if p is None or p["spellname"] != name or r is None:
            refusals.append(f"{sid}: not {name} with a spelleffect row: {p and p['spellname']}")
            continue
        e = json.loads(r["effects"] or "{}")
        fx = e.get("effects", [])
        if sid in AREA:
            _, t, a, page = spec
            hp = [x for x in fx if x.get("kind") == "hp"]
            hp2 = [x for x in fx if x.get("kind") == "hp2"]
            if (len(fx) != 2 or len(hp) != 1 or len(hp2) != 1 or hp[0].get("amount") != t or hp2[0].get("amount") != a
                    or hp[0].get("scale") != "wis" or hp2[0].get("scale") != "wis" or e.get("scope") != "target_aoe"):
                refusals.append(f"{sid} {name}: effects not a target {t} + area {a} on WIS: {fx}")
                continue
            was = (hp[0].get("scale_pct"), hp2[0].get("scale_pct"))
            if was not in ((30, 30), (60, 60), (TARGET_PCT, AREA_PCT)):
                refusals.append(f"{sid} {name}: scales {was}, not as imported: a GM's edit?")
                continue
            hp[0]["scale_pct"], hp2[0]["scale_pct"] = TARGET_PCT, AREA_PCT
            line, desc, note = area_line(t, a), area_desc(t, a), NOTE
            lines.append(f"{sid} {name}: WIS target/area {was[0]}/{was[1]} -> {TARGET_PCT}/{AREA_PCT} "
                         f"(Prima p. {page}); desc {p['spelldesc']!r} -> {desc!r}")
        else:
            _, t = spec
            if len(fx) != 1 or fx[0].get("kind") != "hp" or fx[0].get("amount") != t or fx[0].get("scale") != "wis":
                refusals.append(f"{sid} {name}: effects not one target {t} on WIS: {fx}")
                continue
            was = fx[0].get("scale_pct")
            if was not in (30, SINGLE_PCT):
                refusals.append(f"{sid} {name}: scale {was}, not as imported")
                continue
            fx[0]["scale_pct"] = SINGLE_PCT
            line = single_line(t)
            desc = p["spelldesc"].replace("30% based on WIS", f"{SINGLE_PCT}% based on WIS")
            note = NOTE1
            if f"{SINGLE_PCT}% based on WIS" not in desc:
                refusals.append(f"{sid} {name}: the description does not quote a WIS %: {p['spelldesc']!r}")
                continue
            lines.append(f"{sid} {name}: WIS {was} -> {SINGLE_PCT} (Prima p. 402); desc {p['spelldesc']!r} -> {desc!r}")
        eff = json.dumps(e, separators=(",", ":"))
        if r["effects"] == eff and r["line"] == line:
            out.append(("spelleffect", key, dict(r)))  # as written
        else:
            src = r["source"] if r["source"].endswith(note) else r["source"] + note
            out.append(("spelleffect", key, dict(r, line=line, effects=eff, source=src)))
        out.append(("spellpattern", key, {"spelldesc": desc}))
    if refusals:
        out = []
    applied = bool(out) and all(tr.get(db, t, k) is not None and
                                all(tr.get(db, t, k).get(c) == v for c, v in a.items()) for t, k, a in out)
    lines.insert(0, f"{len(out)} rows")
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("spellpattern", "spelleffect"), "changes": changes,
        "basis": "era", "basis_ref": "Prima pp.400-402"}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, argv=sys.argv[1:], doc=__doc__))
