#!/usr/bin/env python3
"""Casting creatures as a GM asked in three petitions (2026-11-20; Live, the GM's requests approved as submitted):

  5322  "All cleric NPC's and Mystic, with either in their name, should cast a small non tickheal (instant, 1 shot)
        level appropriate heal that does about 15% of an @ level mobs health. They will heal other monsters they are
        social with, so freezeblood clerics will heal other freezeblood clerics. Again, about every 15 seconds"
  5339  "Deathfist prophers, cleric's should be cleric class. They should cast a level appropriate insta heal (no HoT),
        every 20 seconds, and a level appropriate AR nuke every 30 seconds"
  5342  "All NPC's in game that end with "wizard", such a nasehir wizard, should chain 2 second cast time level
        appropriate spells, with a 2 second delay between casts."

Three class templates (classtemplates) and the names that fight by them (npctemplate):

  'kin healer'        class -1 (its own: npcclass's cleric or shaman), a heal of 15% of its own most hit points (no
                      spell: instant, one shot) every 15 s from 2 s into a fight, on itself or the worst off of its
                      kin near it below 85% (target "kin", entity/kinheal.go: the creatures of the faction it helps,
                      npcsocial or the one its kill lowers most, within its social radius, in its sight). Every name
                      with "cleric" or "mystic" in it (a whole word), but the Deathfist's below.
  'deathfist priest'  class 9 (cleric), the same heal every 20 s, and a cleric's arcane nuke of its level (its own
                      recast 30 s at most, cast 5 s at most) every 30 s from 5 s in, at half its figures (the GM's
                      necromancer and shadow knight's level): deathfist prophet, deathfist cleric.
  'chain wizard'      class 13 (wizard), a wizard's nuke of its level every 4 s from 1 s into a fight, cast in 2 s
                      whatever its record's cast time (so 2 s casting, 2 s between), at 35% of its figures (the
                      GM's wizard template's share; recast 10 s at most, record cast 5 s at most, as that one's):
                      every name ending in "wizard".

A name whose every row has the NPC bit (it fights no one) is left out. A name with a template already is changed only
from 'roamer' (tools/seed_roamers.py's, the warrior's crit: a caster now) or from 'nasehir cleric' (petition 5237's
self heal, which 5322 replaces); any other template is refused. Report only by default; --apply in one transaction,
a gm_audit line a row, log petpass_npc_casters_log; --undo exact. The templates are read at the start and at a GM
panel edit of classtemplates or npctemplate. Needs the server with target "kin" (an older one refuses 'kin healer'
and 'deathfist priest' whole: those creatures then fight by no template).

    python3 -E -s tools/petpass_npc_casters_20261120.py --db eqoa.db [--apply | --undo]
"""
import json
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_npc_casters_log"
DETAIL = "Casting creatures (a GM's petitions 5322, 5339, 5342): tools/petpass_npc_casters_20261120.py"
SOURCE = "petpass-1120"

KIN_HEAL = {"kind": "heal", "every": 15, "first": 2, "heal_pct": 15, "below": 85, "target": "kin"}
TEMPLATES = {
    "kin healer": (-1, [KIN_HEAL],
                   "petition 5322 (a GM): clerics and mystics heal 15% of their own hit points, instant, every 15 s, "
                   "on themselves or the worst off of their kin near them (target kin)"),
    "deathfist priest": (9, [dict(KIN_HEAL, every=20),
                             {"kind": "nuke", "every": 30, "first": 5, "element": "arcane", "damage_pct": 50,
                              "max_recast": 30, "max_cast": 5}],
                         "petition 5339 (a GM): Deathfist prophets and clerics are clerics: an instant heal every 20 s "
                         "(as the kin healer's), an arcane nuke of their level every 30 s"),
    "chain wizard": (13, [{"kind": "nuke", "every": 4, "first": 1, "cast": 2, "damage_pct": 35, "max_recast": 10,
                           "max_cast": 5}],
                     "petition 5342 (a GM): wizards chain 2 s casts of their level's nukes, 2 s between"),
}
REPLACEABLE = ("roamer", "nasehir cleric")
DEATHFIST = ("deathfist prophet", "deathfist cleric")


def template_for(name):
    if name in DEATHFIST:
        return "deathfist priest"
    if re.search(r"wizard$", name):
        return "chain wizard"
    if re.search(r"\b(cleric|mystic)\b", name):
        return "kin healer"
    return None


def row(name):
    cls, abilities, note = TEMPLATES[name]
    return {"name": name, "class": cls, "hp_pct": 100, "taken_pct": 100, "melee_pct": 100,
            "abilities": json.dumps(abilities, separators=(",", ":")), "note": note + ": " +
            "tools/petpass_npc_casters_20261120.py", "source": SOURCE}


def changes(db):
    out, lines, refusals = [], [], []
    for name in TEMPLATES:
        want = row(name)
        have = tr.get(db, "classtemplates", {"name": name})
        if have is not None and any(have.get(k) != v for k, v in want.items()) and have.get("source") != SOURCE:
            refusals.append(f"classtemplates {name!r} is there already and not this tool's")
        out.append(("classtemplates", {"name": name}, want))
        lines.append(f"classtemplates {name!r}: class {want['class']}, {want['abilities']}")
    names = {}
    for r in tr.rows(db, "SELECT lower(trim(npc_name)) AS n, count(*) AS c, sum((npc_type & 128) = 0) AS f FROM npcs "
                         "GROUP BY 1"):
        t = template_for(r["n"] or "")
        if t and r["f"]:
            names[r["n"]] = (t, r["c"])
    per = {t: [] for t in TEMPLATES}
    for n, (t, c) in sorted(names.items()):
        have = tr.get(db, "npctemplate", {"name": n})
        if have is not None and have["template"] not in REPLACEABLE + (t,):
            refusals.append(f"npctemplate {n!r} is {have['template']!r}: left for a GM")
            continue
        was = f" (was {have['template']!r})" if have is not None and have["template"] != t else ""
        per[t].append(f"{n} x{c}{was}")
        out.append(("npctemplate", {"name": n}, {"name": n, "template": t,
                                                 "note": TEMPLATES[t][2].split(":")[0], "source": SOURCE}))
    for t, l in per.items():
        lines.append(f"npctemplate -> {t!r}: {len(l)} names: " + ", ".join(l))
    applied = all(tr.get(db, tbl, key) is not None and all(tr.get(db, tbl, key).get(k) == v for k, v in after.items())
                  for tbl, key, after in out)
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("classtemplates", "npctemplate", "npcs"), "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
