#!/usr/bin/env python3
"""Heirloom petitions 2844 and 2846 (Aeris, 2026-11-24, Creature or NPC): "Bloodfoot bandits should be trolls not
orcs" and "the bloodfoot heavy's are not hostile".

The Bloodfoot camp, Mount Hatespike (W), world 0 about (12100-12440, 10970-11290). Its rows: Bloodfoot bandit 20
(L28-30, the orc body 0x8214A32D), Bloodfoot heavy 11 (L30-31, the troll male body 0x4C6FA532 already, race "0"),
jailor, healers, mystics, warlocks, Boss Hurg, Foreman Sogar (orc bodies), and the talkers (armorer, blacksmith,
merchants, Bartender Grag: the NPC bit).

Sources: Prima's guide (2003), the faction table: Bloodfoot Bandits, "Orc and Troll bandits with a large encampment
in Mount Hatespike" (tools/dialog/LORE-FACTS.md GROBB-4). The GameFAQs camp guide and questmasters list the camp's
mobs with no race; EQOA-Data (Quests/30,40,49/group-3-good.json) calls it "a small orc stronghold". No source names
the bandits orcs; the player says they are trolls (taken as truth for a look), so the camp's rank and file, bandits
and heavies, are the trolls and its casters, jailor and named are the orcs.

Step 1, the bodies (npcs, by name, each row checked):
  - every Bloodfoot bandit row: modelid 0x8214A32D (orc) -> 1282385202 (0x4C6FA532, the troll male: charactermodel
    race 5 sex 0, the body the camp's heavies wear), race "0" -> "5". Size 1.0 (the heavies' and most trolls'); held
    weapons (none) and hair kept.
  - every Bloodfoot heavy row: race "0" -> "5" (194 troll-bodied rows say 5), the body as it is.
Step 1, the hostility (npcaggro): a players' race body attacks no one without a GM's rule (entity/aggro.go
hostileNatureLocked), so the heavies stood peaceful (2846) and the bandits would have: an aggressive row for each
name (radius 0 = the default 21, leash 0), as Creepyman's (petpass_both_1124k.py). The twenty new troll bodies also
make the camp's middle a town (towns.go, 20 folk within 60 of each other); the rule decides before the town, and
no orc of the camp stands within the town's 40 margin.

Live's rows are the same: mirrored. Report only by default; --apply in one transaction, a gm_audit line a row, log
petpass_both_1124m_log; --undo exact. npcs and npcaggro are read at the start.

    python3 -E -s tools/petpass_both_1124m.py --db eqoa.db [--apply | --undo]
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_both_1124m_log"
DETAIL = ("Heirloom petitions 2844/2846: Bloodfoot bandits given the troll body (race 5), heavies race 5, both "
          "aggressive by npcaggro (tools/petpass_both_1124m.py)")
SOURCE = "petpass-1124m"

ORC = 2182390573    # 0x8214A32D, the orc body the bandits wore (orc brute, orc henchman, ...)
TROLL = 1282385202  # 0x4C6FA532, the troll male (charactermodel race 5 sex 0)

# name -> (rows expected, body before, body after)
ROWS = {
    "Bloodfoot bandit": (20, ORC, TROLL),
    "Bloodfoot heavy": (11, TROLL, TROLL),
}
AGGRO = {
    "Bloodfoot bandit": "Heirloom petition 2844: troll body (Prima: orc and troll bandits); a players' race body "
                        "would have left them peaceful",
    "Bloodfoot heavy": "Heirloom petition 2846: a troll body stood peaceful; the camp's bandits attack on sight",
}


def changes(db):
    out, lines, refusals = [], [], []
    m = tr.rows(db, "SELECT race, sex FROM charactermodel WHERE modelid = ?", (TROLL,))
    if not m or m[0]["race"] != 5 or m[0]["sex"] != 0:
        refusals.append(f"charactermodel {TROLL:#x} is not the troll male: {m}")
    for name, (want, before, after) in ROWS.items():
        rs = tr.rows(db, "SELECT id FROM npcs WHERE npc_name = ? ORDER BY id", (name,))
        if len(rs) != want:
            refusals.append(f"{len(rs)} npcs rows named {name}, not {want}")
            continue
        n_changed = 0
        for r in rs:
            key = {"id": r["id"]}
            n = tr.get(db, "npcs", key)
            if (n["modelid"] or 0) & 0xFFFFFFFF not in (before, after) or n["world"] != 0:
                refusals.append(f"npcs {r['id']} {name}: body {n['modelid']} world {n['world']} (a GM's edit?)")
                continue
            if str(n["race"]) not in ("0", "5"):
                refusals.append(f"npcs {r['id']} {name}: race {n['race']!r}")
                continue
            out.append(("npcs", key, dict(n, modelid=after, race="5")))
            n_changed += 1
        lines.append(f"npcs {name}: {n_changed} rows -> body {after:#010x}, race 5")
    for name, note in AGGRO.items():
        k = {"npc_name": name}
        rule = tr.get(db, "npcaggro", k)
        if rule is not None and rule.get("source") != SOURCE:
            refusals.append(f"npcaggro already has {name}: {rule}")
            continue
        out.append(("npcaggro", k, dict(k, aggressive=1, radius=0.0, leash=0.0, holding=0, see_invis=0, holds=0,
                                        note=note, source=SOURCE)))
        lines.append(f"npcaggro {name} -> aggressive")
    if refusals:
        out = []
    applied = bool(out) and all(tr.get(db, t, k) is not None and
                                all(str(tr.get(db, t, k).get(c)) == str(v) for c, v in a.items()) for t, k, a in out)
    lines.insert(0, f"{len(out)} rows")
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("npcs", "npcaggro", "charactermodel"), "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, argv=sys.argv[1:], doc=__doc__))
