#!/usr/bin/env python3
"""The FAQ's missing camps and kinds, and its wrong-level camps at its levels (the owner, 2026-11-14: "add the missing
camps from the camps report", then "fix the wrong level camps too").

The camps report (lane quest-guides-17: Jarrod Garripoli's EQOA Frontiers FAQ, GameFAQs 48152, section XVI "Camps &
Unique Places", against our spawns) named four camps we lacked, camps missing some of their kinds, and eight camps
standing outside the FAQ's level band. This writes, on each shard from that shard's own rows:

  new camps   Frosteye Orcs (Snowblind Plains SE, the excavation site: centurions, patrollers, sages L54-57), the Tae Ew
              aggressors (Stone Watchers NW: aggressors, sacrificers L33-37), the Desert Bandits (henchmen, pillagers L60-62
              round Sothimus the Brigand in Tak Xiv, aggro-linked: npcsocial "Desert Bandits"), the Burial Mounds frogloks
              (froglok slayers to six, Froglok Inquisitors L60-62)
  kinds added Mudscale and four ancient gharials (the island south of Muniel's Tea Garden), Garhuuga, Himlar and two
              elder spirits again (the rows the Odus census of 2026-10-31 removed, back on their spots), The Ancient One
              (West Plateau), Elder Squeeks (Zentar's Keep NW), Tendora-Buki guards and villagers (West Toxxulia), the
              Arctic Adept (by the Arctic Spiritcaller), frost drakes (Permafrost), Quintashi's guardians (his lair), and
              in The Hunt plagued insect clouds, sabretooth huntresses and vercozian wyrms
  levels      Mossmouth goblins 32-38 -> 21-30, Cracktooth orcs 12-16 -> 21-30, Brokenskull Rock's Tae Ew fort (soldiers,
              shamans and their spirits, level 2) -> 41-47, the Cyclops' Fortress hobgoblin mercenaries 53-55 -> 46-50,
              Odus's South Barren Coast gargoyles 46-55 -> 41-50, its ratman fighters and scavengers 55-57 -> 48-50, The
              Warrens' clan kolbok scouts and shamans 51-55 -> 46-50, the Tendora-Buki 51-53 -> 46-50 (each camp's rows
              spread across the new band as they were across the old; hit points and damage follow the level)

A new creature's row is a copy of a row of its body's kind on that shard (gear, flags) at its own spot, picked on the
ground files' floors (level ground, out of the water, apart from every other NPC), with a 90 s respawn (npcrespawn) as
its camp's; a new kind takes its kin's class (npcclass, a caster's class where the FAQ says so), template (npctemplate)
and loot region (lootcreature). Bodies were chosen by sight from CHAR.ESF renders. The plan, per shard
(tools/faq_camps_20261114.json, made by the lane's gen.py on 11:09 UTC copies of both shards), names every row and its
value before; a row not as the plan has it (a GM's edit since) refuses everything.

Report only by default; --apply in one transaction, a gm_audit line a row, log faq_camps_log; a second --apply does
nothing; --undo exact (a row changed since is left and said). The shard is told by its rows, else by Heirloom's heirloom_seed_log (--shard h|l to say it).
npcs and their tables are read at the server's start: a restart (a keep restart).

    python3 -E -s tools/faq_camps_20261114.py --db eqoa.db [--apply | --undo] [--shard h|l]
"""
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "faq_camps_log"
DETAIL = "the FAQ's missing camps and kinds and its wrong-level camps (GameFAQs 48152): tools/faq_camps_20261114.py"
PLAN = os.path.join(os.path.dirname(os.path.abspath(__file__)), "faq_camps_20261114.json")
SHARD = None


def state(db, ops):
    """(todo, done, conflicts) of a shard's ops against db."""
    todo, done, bad = [], 0, []
    for tbl, key, before, after in ops:
        now = tr.get(db, tbl, key)
        if before is None:
            if now is None:
                todo.append((tbl, key, after))
            elif all(now.get(c) == v for c, v in after.items()):
                done += 1
            else:
                bad.append(f"{tbl} {key}: a row is there already, not this tool's")
        else:
            if now is None:
                bad.append(f"{tbl} {key}: gone")
            elif all(now.get(c) == v for c, v in after.items()):
                done += 1
            elif all(now.get(c) == v for c, v in before.items()):
                todo.append((tbl, key, after))
            else:
                bad.append(f"{tbl} {key}: {', '.join(f'{c} {now.get(c)!r}' for c in before)}, not the plan's "
                           f"{', '.join(f'{c} {v!r}' for c, v in before.items())}")
    return todo, done, bad


def changes(db):
    plan = json.load(open(PLAN))
    shards = [SHARD] if SHARD else list(plan["shards"])
    seen = {sh: state(db, plan["shards"][sh]) for sh in shards}
    fits = [sh for sh in shards if not seen[sh][2]]
    if len(fits) == 2:
        # the new rows fit both, as most re-levelled rows do: Heirloom is the shard with heirloom_seed_log
        fits = ["h" if tr.table(db, "heirloom_seed_log") else "l"]
    if len(fits) != 1:
        best = min(shards, key=lambda sh: len(seen[sh][2]))
        return [], [], (["both shards' plans fit: say --shard h or l"] if fits else
                        [f"(nearest: the {best} plan) " + b for b in seen[best][2][:40]]), False
    sh = fits[0]
    todo, done, _ = seen[sh]
    lines = [f"shard {sh}: {len(todo)} rows to write, {done} as the plan has them already"]
    for k, v in plan["summary"][sh].items():
        lines.append(f"  {k}: " + "; ".join(v))
    return todo, lines, [], not todo and done > 0


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("npcs", "npcrespawn", "npcclass", "npctemplate", "lootcreature", "npcsocial"),
        "changes": changes}

if __name__ == "__main__":
    argv = sys.argv[1:]
    if "--shard" in argv:
        i = argv.index("--shard")
        SHARD = argv[i + 1]
        del argv[i:i + 2]
    sys.exit(tr.main(SPEC, argv, doc=__doc__))
