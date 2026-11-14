#!/usr/bin/env python3
"""Petition 2103 (Kyros, Heirloom, 2026-11-14): "Mummy for FP 17 quest should be Ancient Mummy. Location seems
accurate." (he stood at world 0 (22900.1, 67.9, 16232.5), on mummy row 115916).

The level 17 quest The Scribe's Inks (100748) asks for embalming fluid. The players' guide (eqoa.forumotion.com,
Level 17 Side Quests, Freeport): "Kill an Ancient Mummy (CONs blue to yel), which can be found in Northern Ro and
Northwestern Ro zones (SW of FP) ... head towards the hills north of this camp". We had no "ancient mummy": the quest's
step took any "mummy" (build228, tools/quest_guides17_20261114.py, wrote "mummies"). This makes the kind:

  npcs        the seven mummies of Northern and North-western Ro (rows 1999, 115694, 115866, 115873, 115879, 115916,
              115977; the Temple of Light's two and Hazinak's six stay "mummy") renamed "ancient mummy", levels 17-19
              (blue to yellow to a 17), on the mummy body's darker variant (BB46D88D, the tainted and defiled mummies'),
              a tenth larger; their wander rows (npcwander, by row) as they were
  the kind    "ancient mummy" takes the mummy's class (npcclass), template ("roamer lesser infection": the infection of
              build215's venom templates, npctemplate), loot (npcloot "wiki undead", lootcreature desert) and undead
              (npcundead)
  the quest   step 1's target "ancient mummy", its journal and the offer saying so

Refused, nothing written, when a row is not as expected (a GM's edit since). Report only by default; --apply in one
transaction, a gm_audit line a row, log ancient_mummy_log; a second --apply does nothing; --undo exact (a row
changed since is left and said). npcs, their tables and quests are read at the server's start: a restart (a keep
restart).

    python3 -E -s tools/ancient_mummy_20261114.py --db eqoa.db [--apply | --undo]
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "ancient_mummy_log"
DETAIL = "petition 2103 (Kyros): the Freeport level 17 quest's ancient mummies west of Freeport: tools/ancient_mummy_20261114.py"
SRC = "petition 2103"
NAME = "ancient mummy"
BODY_FROM, BODY_TO = 3141982348, 3141982349  # 0xBB46D88C -> 0xBB46D88D
ROWS = {1999: 17, 115694: 18, 115866: 19, 115873: 17, 115879: 18, 115916: 19, 115977: 18}
QUEST = 100748
JOURNAL = ("Kill mummies in the desert west of Freeport (north-western Ro) or just north of Hazinak for embalming fluid.",
           "Kill ancient mummies in the deserts west of Freeport (Northern and North-western Ro) for embalming fluid.")
OFFER = ("I need two things for my work: embalming fluid, from the mummies in the desert west of the city or just north "
         "of Hazinak, and cured drake's blood from Talya Montar in Highpass.",
         "I need two things for my work: embalming fluid, from the ancient mummies in the deserts west of the city, and "
         "cured drake's blood from Talya Montar in Highpass.")
KIND = (("npcclass", "name", None), ("npctemplate", "name", None), ("npcloot", "npc_name", "tablename"),
        ("lootcreature", "npc_name", None), ("npcundead", "name", None))


def changes(db):
    out, lines, refusals = [], [], []
    done = todo = 0

    def want(tbl, key, before, after):
        nonlocal done, todo
        now = tr.get(db, tbl, key)
        if after is not None and now is not None and all(now.get(c) == v for c, v in after.items()):
            done += 1
        elif (before is None and now is None) or (before is not None and now is not None and
                                                 all(now.get(c) == v for c, v in before.items())):
            out.append((tbl, key, after))
            todo += 1
        else:
            refusals.append(f"{tbl} {key}: {now} is not as expected ({before})")

    for rid, lv in ROWS.items():
        want("npcs", {"id": rid}, {"npc_name": "mummy", "modelid": BODY_FROM, "npc_level": 18, "size": 1.0},
             {"npc_name": NAME, "modelid": BODY_TO, "npc_level": lv, "size": 1.1})
    for tbl, col, extra in KIND:
        src = tr.rows(db, f"SELECT * FROM {tbl} WHERE lower({col}) = 'mummy'")
        if not src:
            refusals.append(f"{tbl}: no row of the mummy to copy")
            continue
        for r in src:
            r = dict(r)
            r[col] = NAME
            if "source" in r:
                r["source"] = SRC
            if "note" in r and tbl == "npctemplate":
                r["note"] = "generic, level 17-19 (the mummy's)"
            key = {col: NAME}
            if extra:
                key[extra] = r[extra]
            want(tbl, key, None, r)
    want("queststeps", {"questid": QUEST, "step": 1}, {"target": "mummy", "journal": JOURNAL[0]},
         {"target": NAME, "journal": JOURNAL[1]})
    want("quests", {"questid": QUEST}, {"offer": OFFER[0]}, {"offer": OFFER[1]})
    lines.append(f"{todo} rows to write, {done} as this tool writes them already: seven mummies west of Freeport "
                 f"made ancient mummies (L17-19, body BB46D88D, size 1.1), the kind's class/template/loot/undead rows, "
                 f"quest {QUEST} step 1's target and journal, its offer")
    return out, lines, refusals, todo == 0 and done > 0


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("npcs", "npcclass", "npctemplate", "npcloot", "lootcreature",
                                               "npcundead", "quests", "queststeps"), "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
