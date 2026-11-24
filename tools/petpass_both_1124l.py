#!/usr/bin/env python3
"""Heirloom petition 2843 (Lestat, 2026-11-24, Spell or ability): "Vampire Form cast is not permanent. it should be."

Vampire Form (760002) ended after ten minutes: its spelleffect row gives "duration_s":600 (the effect's and the
spell's) and its line ", 10min". Board #1345 (wereforms director2, 2026-11-13) wrote that 600 to keep the parser's
default illusion time of then, "historical duration unknown": no source gives a time. The vampire guide (EQOA-Data
Quests/Weres/vampire.json) gives Bat Form 30 s, Mist Form 30 s, Vitality and Frenzy 60 s, and Vampire Form none,
"Visual form only (no stat buffs)". The owner ruled on 2026-11-20 that the were forms, the other cursed bloodline's
forms, last until dropped (internal/entity/illusion.go wereFormLasts, commit b4f5047c); the vampire's own form is
taken the same way.

So 760002's two duration_s go 600 -> 315360000 (ten years, wereFormLasts exactly: the buff window shows a line past a
year with no end, buffwindow.go noEndShown) and its line drops ", 10min". It still ends as any form does: death, a
click off, True Form/Remove Illusion, another form or illusion. Sprite, reagent and the rest of the row unchanged.

Live's row is the same: mirrored. Report only by default; --apply in one transaction, a gm_audit line, log
petpass_both_1124l_log; --undo exact. Spells are read at the next cast.

    python3 -E -s tools/petpass_both_1124l.py --db eqoa.db [--apply | --undo]
"""
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_both_1124l_log"
DETAIL = ("Heirloom petition 2843: Vampire Form (760002) lasts until dropped, as the were forms (owner 2026-11-20), "
          "not 10 minutes (tools/petpass_both_1124l.py)")
SPELL, NAME = 760002, "Vampire Form"
UNTIL_DROPPED = 10 * 365 * 24 * 3600  # illusion.go wereFormLasts, in seconds
OLD_LINE = "Self, Gives Form, Reagent Unit of Blood x2, 10min"
NEW_LINE = "Self, Gives Form, Reagent Unit of Blood x2"
NOTE = "; petition 2843 (2026-11-24): until dropped, as the were forms (owner 2026-11-20), was explicit 600s"


def changes(db):
    out, lines, refusals = [], [], []
    key = {"spellid": SPELL}
    p = tr.get(db, "spellpattern", key)
    if p is None or p["spellname"] != NAME:
        refusals.append(f"spellpattern {SPELL} is not {NAME}: {p and p['spellname']}")
    r = tr.get(db, "spelleffect", key)
    if r is None:
        refusals.append(f"no spelleffect row for {SPELL}")
    else:
        e = json.loads(r["effects"])
        fx = [x for x in e.get("effects", []) if x.get("kind") == "illusion"]
        if len(e.get("effects", [])) != 1 or len(fx) != 1:
            refusals.append(f"{SPELL}'s effects are not one illusion: {e.get('effects')}")
        elif r["line"] == NEW_LINE and e.get("duration_s") == UNTIL_DROPPED and fx[0].get("duration_s") == UNTIL_DROPPED:
            out.append(("spelleffect", key, dict(r)))  # as written: reported applied below
        elif r["line"] != OLD_LINE or e.get("duration_s") != 600 or fx[0].get("duration_s") != 600:
            refusals.append(f"{SPELL} is not as #1345 left it (line {r['line']!r}, duration_s "
                            f"{e.get('duration_s')}/{fx[0].get('duration_s')}): a GM's edit?")
        else:
            fx[0]["duration_s"] = UNTIL_DROPPED
            e["duration_s"] = UNTIL_DROPPED
            out.append(("spelleffect", key, dict(r, line=NEW_LINE, source=r["source"] + NOTE,
                                                 effects=json.dumps(e, separators=(",", ":")))))
            lines.append(f"spelleffect {SPELL} {NAME}: duration_s 600 -> {UNTIL_DROPPED} (until dropped), "
                         f"line {OLD_LINE!r} -> {NEW_LINE!r}")
    if refusals:
        out = []
    applied = bool(out) and all(tr.get(db, t, k) is not None and
                                all(tr.get(db, t, k).get(c) == v for c, v in a.items()) for t, k, a in out)
    lines.insert(0, f"{len(out)} rows")
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("spellpattern", "spelleffect"), "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, argv=sys.argv[1:], doc=__doc__))
