#!/usr/bin/env python3
"""Live petition 5581 (Life, 2026-11-24, Creature or NPC): "Taksehir Warrior, Taksehir Ranger, Taksehir Cleric,
Taksehir Cleric: all of these creatures are not assigned the correct faction, and do not give the correct faction
on kill".

The Taksehir are the desert people of the Great Waste and the Burial Mounds (world 0: the camps of zones 837-845 and
941-950, the village of zone 1082, the crystal mines of 1080-1088; on Heirloom three more in world 14). Fourteen names
(bandit, citizen, cleric, Craftsman, crystal runner, foreman, guard, lookout, miner, ranger, rogue, scout, warrior,
wizard), and on both shards none had an npcfaction row, so none was of any faction; and only "taksehir bandit" had kill
hits (petpass_kill_factions_20261120's, Prima's): a kill of any other gave nothing.

Prima's faction tables (pp. 383-392): The Taksehir Nation, kill penalty 10 (Table 1); one associate, Nasehir
Cutthroats at -10 care (Table 2): an enemy, so a kill raises it. petfix582's rule (prima_hits: the faction's own
penalty, each associate rating% of it, a share under 1 point no row, a fraction cut toward 0): The Taksehir Nation
-10, Nasehir Cutthroats -(10 x -10%) = +1.

So every name: npcfaction -> The Taksehir Nation (world -1, any place), and its kill hits -> The Taksehir Nation -10,
Nasehir Cutthroats +1 (taksehir bandit's two rows are these already and stay as they are).

On sight: they wear players' races' bodies (no GM aggro rule), so they attacked no one before; bound, they attack a
player whose standing with The Taksehir Nation is -350 or lower. Characters start at 200 (factions), or 0 / -200 by
race (factionstart: gnomes -200): no character on either shard stands at -350 or below now (lowest -199), so nothing
changes at once; about 35 kills from 0 (55 from 200) turn them red. The one Taksehir miner with a creature's body
(npcs 3240, zone 0) was hostile by nature; bound, it goes by the faction too.

Quests: seven loot steps name Taksehir targets (101034/101086/101096 miners, 101153 miners, 101079 crystal runner,
100786 the Craftsman, 101158 bandits); none gates on or rewards the faction. Their kills now cost Taksehir standing.

Report only by default; --apply in one transaction, a gm_audit line a row, log petpass_both_1124u_log; --undo exact.
factionhits and npcfaction are read at the server's start.

    python3 -E -s tools/petpass_both_1124u.py --db eqoa.db [--apply | --undo]
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_both_1124u_log"
DETAIL = ("Live petition 5581: the Taksehir bound to The Taksehir Nation, Prima's kill hits (The Taksehir Nation -10, "
          "Nasehir Cutthroats +1) (tools/petpass_both_1124u.py)")
SOURCE = "prima-2003 petpass-1124u"
FACTION, ENEMY = "The Taksehir Nation", "Nasehir Cutthroats"
NAMES = ("taksehir bandit", "taksehir citizen", "taksehir cleric", "taksehir craftsman", "taksehir crystal runner",
         "taksehir foreman", "taksehir guard", "taksehir lookout", "taksehir miner", "taksehir ranger",
         "taksehir rogue", "taksehir scout", "taksehir warrior", "taksehir wizard")
NOTE = "the Taksehir of the Great Waste and the Burial Mounds (Prima: The Taksehir Nation); Live petition 5581"

# Prima Table 1 / Table 2 for the faction (as petfix582's PRIMA_T1 / PRIMA_T2)
PRIMA_T1 = {FACTION: 10}
PRIMA_T2 = {FACTION: [[ENEMY, -10]]}


def prima_hits(f):
    """petfix582's prima_hits, unchanged."""
    p = PRIMA_T1[f]
    h = {f: -p}
    for a, r in PRIMA_T2.get(f, []):
        s = p * r / 100
        if abs(s) < 1:
            continue
        h[a] = h.get(a, 0) - int(s)
    return h


def changes(db):
    out, lines, refusals = [], [], []
    want = prima_hits(FACTION)
    if want != {FACTION: -10, ENEMY: 1}:
        refusals.append(f"Prima's hits computed as {want}")
    for f in (FACTION, ENEMY):
        if not tr.rows(db, "SELECT 1 FROM factions WHERE name = ?", (f,)):
            refusals.append(f"no faction {f!r} in factions")
    found = {r["n"] for r in tr.rows(db, "SELECT DISTINCT lower(trim(npc_name)) AS n FROM npcs "
                                         "WHERE lower(npc_name) LIKE 'taksehir %'")}
    if found != set(NAMES):
        refusals.append(f"Taksehir names in npcs differ: more {sorted(found - set(NAMES))}, "
                        f"fewer {sorted(set(NAMES) - found)}")
    for name in NAMES:
        k = {"name": name, "world": -1, "x": 0.0, "z": 0.0, "radius": 0.0}
        nf = tr.rows(db, "SELECT * FROM npcfaction WHERE lower(trim(name)) = ?", (name,))
        mine = [r for r in nf if r["faction"] == FACTION and r["source"] == SOURCE and r["name"] == name
                and r["world"] == -1 and r["radius"] == 0]
        if len(nf) != len(mine):
            refusals.append(f"npcfaction has rows for {name}: {nf}")
        out.append(("npcfaction", k, dict(k, faction=FACTION, note=NOTE, source=SOURCE)))
        lines.append(f"npcfaction {name}: {nf[0]['faction'] if nf else '(none)'} -> {FACTION}")
        if tr.rows(db, "SELECT 1 FROM factionhitareas WHERE lower(trim(npc_name)) = ?", (name,)):
            refusals.append(f"factionhitareas has rows for {name}")
        have = {h["faction"]: h for h in tr.rows(db, "SELECT * FROM factionhits WHERE lower(trim(npc_name)) = ?",
                                                 (name,))}
        odd = [h for f, h in have.items() if f not in want or h["delta"] != want[f] or h["npc_name"] != name]
        if odd:
            refusals.append(f"factionhits {name}: rows other than Prima's: {odd}")
        for f, d in want.items():
            if f in have and have[f]["source"] != SOURCE:
                lines.append(f"factionhits {name} / {f}: {have[f]['delta']:+d} kept ({have[f]['source']})")
                continue
            out.append(("factionhits", {"npc_name": name, "faction": f},
                        {"npc_name": name, "faction": f, "delta": d, "source": SOURCE}))
            lines.append(f"factionhits {name} / {f}: {have[f]['delta'] if f in have else '(none)'} -> {d:+d}")
    if refusals:
        out = []
    applied = bool(out) and all(
        tr.get(db, t, k) is not None and all(tr.get(db, t, k).get(c) == v for c, v in a.items()) for t, k, a in out)
    lines.insert(0, f"{len(out)} rows")
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("npcs", "npcfaction", "factionhits", "factionhitareas", "factions"),
        "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
