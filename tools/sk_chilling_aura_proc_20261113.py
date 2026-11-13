#!/usr/bin/env python3
"""The shadow knight's Chilling Aura an offensive proc (Smoke's Live petition 5006, 2026-11-13: "chilling aura is acting as
a damage shield not a proc at the moment!").

Its own words are a weapon's ("Summons an aura that surrounds your weapon, inflicting unholy damage."), and the shadow
knight's patch notes (questmasters id273/id281: "The four procs (Chilling Aura, Cursed Hand, Veil of Death, Shroud of
Death) now use the same mechanic") make it one of the four procs; Cursed Hand and Veil of Death are offensive procs here.
Its record (1420, the old dump's, with EQOAEmu's workbook effects) is a Defensive Proc (def_proc, "on": "defense"), which
the server lands as a damage shield's answer at each hit taken ("Shield: Smoke's \"Chilling Aura\" answers bandit for 2").
This makes it an Offensive Proc with the same figure, resist (CR) and time (5 min):

- spelleffect 1420's effects: sheet_type and the effect's type "Offensive Proc", its kind "proc", its proc's "on"
  "offense" (Cursed Hand's frame); everything else as it is;
- its line "Self, Damage Shield Decrease Hitpoints by 2, 5min" -> "Self, Add Offensive Proc: Decrease Hitpoints by 2, 5min".

Found by name (class 3's "Chilling Aura") and its effects checked as found, else refused, nothing written. Report by
default; --apply in one transaction, a gm_audit line, logged in sk_chilling_aura_1113_log; --undo puts it back while still
as written. A spell's effects are read at the server's start: RESTART after --apply (a buff already on keeps its old rules
until cast again). Run it on each shard.

    python3 -E -s tools/sk_chilling_aura_proc_20261113.py --db eqoa.db [--apply | --undo]
"""
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import sk_cursed_hand_level_20261113 as w  # noqa: E402  the writer

LOG = "sk_chilling_aura_1113_log"
DETAIL = "Live petition 5006 (Smoke): Chilling Aura an offensive proc, not a damage shield; tools/sk_chilling_aura_proc_20261113.py"
LINE_FROM = "Self, Damage Shield Decrease Hitpoints by 2, 5min"
LINE_TO = "Self, Add Offensive Proc: Decrease Hitpoints by 2, 5min"


def offensive(text):
    """The effects JSON as an offensive proc, or (None, why)."""
    j = json.loads(text)
    effs = j.get("effects") or []
    if j.get("sheet_type") != "Defensive Proc" or len(effs) != 1 or effs[0].get("kind") != "def_proc":
        return None, f"not the one defensive proc found: {text}"
    e = effs[0]
    if any(p.get("on") != "defense" for p in e.get("procs") or []) or not e.get("procs"):
        return None, f"its procs are not on defense: {text}"
    j["sheet_type"] = "Offensive Proc"
    e["type"], e["kind"] = "Offensive Proc", "proc"
    for p in e["procs"]:
        p["on"] = "offense"
    return json.dumps(j, separators=(",", ":")), ""


def plan(db):
    for t in ("spellpattern", "spelleffect", "classspells", "gm_audit"):
        if not w.table(db, t):
            return [], [], [f"no {t} table here"]
    sp = db.execute("SELECT p.spellid FROM spellpattern p JOIN classspells c ON c.spellid = p.spellid AND c.class = 3 "
                    "WHERE p.spellname = 'Chilling Aura'").fetchall()
    if len(sp) != 1:
        return [], [], [f"{len(sp)} shadow knight spells named 'Chilling Aura', want one"]
    sid = sp[0][0]
    se = w.row(db, "spelleffect", sid)
    if se is None:
        return [], [], [f"spelleffect {sid}: none"]
    eff, why = offensive(se["effects"] or "{}")
    if eff is None:
        return [], [], [f"spelleffect {sid}: {why}"]
    if se["line"] != LINE_FROM:
        return [], [], [f"spelleffect {sid}: line {se['line']!r}, not {LINE_FROM!r}"]
    return ([("spelleffect", sid, {"effects": eff, "line": LINE_TO})],
            [f"spelleffect {sid} 'Chilling Aura': a defensive proc (a damage shield's answer) -> an offensive proc, 2 CR, 5 min",
             f"  line {LINE_FROM!r} -> {LINE_TO!r}"], [])


if __name__ == "__main__":
    sys.exit(w.run(plan, LOG, DETAIL, __doc__))
