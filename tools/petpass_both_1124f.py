#!/usr/bin/env python3
"""Heirloom petition 2833 (Aeris, 2026-11-24, Quest): "The Highbourne level 17 quest npc Ulira Veshine is the wrong
race, not hostile, the quest item didn't drop on the first kill, and her wizard friend isn't there."

Ulria Veshiue (npcs 122503, world 0, about (12456, 16562), north end of Alseop's Wall) is the target of Highbourne's
level 17 quest "Ulria Veshiue" (100751), step 1 a loot step for the Veshiue Insignia Ring (700726, chance 1.0).

  - Race. The sources make her Indoran Veshiue's sister (questmasters id161: "Indoran's sister was entranced by a dark
    elf mage"; the GameFAQs walkthrough: "Indoran wants you to find his sister"), and Indoran, like Highbourne's
    people, is an Erudite (npcs 106017: the Erudite woman's body 0x4F1A3CBE, race 8). Her row wore the elf woman's
    body (0xE18919F7) with race 6 (Barbarian), and so dropped Elf Ears. Now the Erudite woman's body, 1327119550,
    copied from Indoran's row and charactermodel (race 8, sex 1), race 8, and size 1.0 (199 of the 200 rows of that
    body; her 0.89 was an elf's).
  - Not hostile. A players' race's body attacks no one by its nature (entity/aggro.go, hostileNatureLocked), and no
    rule named her. The guides have her pulled and killed ("You do not have to engage the necromancer, so you'll be
    able to pull Ulria by herself"; EQOA-Data: "CONs blue ... Kill, loot, return"), but no source puts her in a
    faction, so none is invented: an npcaggro row (aggressive, radius and leash her own), as Live 5572's decaying
    skeletons had (tools/petpass_live_1123u.py).
  - No drop on the first kill: not the data (step 1 names her, chance 1.0). The log shows the quest engine's kill
    queue placed the ring 2 s after the kill (00:44:44), after Aeris had opened and emptied the corpse (00:44:42-44);
    on the second kill it was placed at the kill and she looted it. A code matter, reported, not changed here.
  - Her companion: the guides' necromancer Lucariel K'Tral (questmasters: "the mage and his pet"). No row of that
    name on either shard, spawn-only or otherwise: a new spawn, proposed to the GMs, not added here.

Live's rows are the same (npcs 122503, no npcaggro row): mirrored. Report only by default; --apply in one
transaction, a gm_audit line a row, log petpass_both_1124f_log; --undo exact. npcs and npcaggro are read at the
server's start.

    python3 -E -s tools/petpass_both_1124f.py --db eqoa.db [--apply | --undo]
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_both_1124f_log"
DETAIL = ("Heirloom petition 2833: Ulria Veshiue an Erudite woman (Indoran Veshiue's sister) and aggressive "
          "(tools/petpass_both_1124f.py)")
SOURCE = "petpass-1124f"
NAME = "Ulria Veshiue"
ROW = 122503
ELF_F, ERUDITE_F = 3783858679, 1327119550
BROTHER = 106017  # Indoran Veshiue


def changes(db):
    out, lines, refusals = [], [], []

    cm = tr.rows(db, "SELECT modelid FROM charactermodel WHERE race = 8 AND sex = 1")
    if not cm or cm[0]["modelid"] != ERUDITE_F:
        refusals.append(f"charactermodel's Erudite woman is {cm}, not {ERUDITE_F}")
    bro = tr.get(db, "npcs", {"id": BROTHER})
    if bro is None or bro["npc_name"] != "Indoran Veshiue" or bro["modelid"] != ERUDITE_F:
        refusals.append(f"npcs {BROTHER} is not Indoran Veshiue in the Erudite woman's body: {bro and bro['modelid']}")

    key = {"id": ROW}
    n = tr.get(db, "npcs", key)
    if n is None or n["npc_name"] != NAME:
        refusals.append(f"npcs {ROW} is not {NAME}: {n and n['npc_name']}")
    elif n["modelid"] not in (ELF_F, ERUDITE_F):
        refusals.append(f"npcs {ROW}'s body is {n['modelid']}, neither the elf's nor the Erudite's (a GM's edit?)")
    else:
        race = "8" if isinstance(n["race"], str) else 8
        out.append(("npcs", key, dict(n, modelid=ERUDITE_F, race=race, size=1.0)))
        lines.append(f"2833: npcs {ROW} {NAME} modelid {n['modelid']} -> {ERUDITE_F} (the Erudite woman, as Indoran "
                     f"{BROTHER}), race {n['race']!r} -> {race!r}, size {n['size']} -> 1.0")
    others = tr.rows(db, "SELECT count(*) AS c FROM npcs WHERE npc_name = ? AND id != ?", (NAME, ROW))[0]["c"]
    if others:
        refusals.append(f"{others} other npcs rows are named {NAME}")

    k2 = {"npc_name": NAME}
    rule = tr.get(db, "npcaggro", k2)
    want = dict(k2, aggressive=1, radius=0.0, leash=0.0, holding=0, see_invis=0, holds=0,
                note="Heirloom petition 2833: the quest's target, pulled and killed in the guides; her players' race "
                     "body (an Erudite) left her peaceful", source=SOURCE)
    if rule is not None and rule.get("source") != SOURCE:
        refusals.append(f"npcaggro already has {NAME}: {rule}")
    else:
        out.append(("npcaggro", k2, want))
        lines.append(f"2833: npcaggro {NAME} -> aggressive (no faction: none in the sources)")
    fr = tr.rows(db, "SELECT faction FROM npcfaction WHERE lower(name) = lower(?)", (NAME,))
    if fr:
        lines.append(f"note: npcfaction names {NAME}: {fr}")

    if refusals:
        out = []
    applied = bool(out) and all(tr.get(db, t, k) is not None and
                                all(tr.get(db, t, k).get(c) == v for c, v in a.items()) for t, k, a in out)
    lines.insert(0, f"{len(out)} rows")
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("npcs", "npcaggro", "npcfaction", "charactermodel"),
        "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
