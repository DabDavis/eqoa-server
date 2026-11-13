#!/usr/bin/env python3
"""Bosses that set hit points to a figure (2026-11-13; the owner: "dreadnaughts are supposed to have an AoE which reduces
hp to 700 for players and mobs. Cazic Thule sets to 10hp for his debuff, etc. we may be missing that mechanic", then
"dreadnaught's aoe that debuffs to 700hp also has a damage over time ticks 650 with no CR, with max CR is ticking around
80"), on the server's code of the same branch (entity/bossmoments.go: the "sethp" moment; an older server reads these
templates as unreadable, so apply it with the restart that brings the code, or after it and then reload_npcs).

  classtemplates "dreadnaught" (QuestMasters' Dreadnaught guide, 2006: "The tank will be debuffed by the dreadnaught to
    700hps", "otherwise all in area will be debuffed to less than 700hps", "Debuffs everyone in the area and hits them
    with that deadly DoT"; its template note: "a hit every ~30 s, sight-blocked"): Dread Curse every 30 s from 2 s into
    the fight, everyone within 30 in its sight, players and creatures, to 700, and a cold damage over time of 650 a
    tick at no CR, 80 at the cap (315), every 6 s for 24 s.
  classtemplates "iod cazicthule" (QuestMasters' "The Basics to MT Cazic Thule", Beowoulf: "CT has 2 main debuffs, one
    is aoe, other is tank only" and his death touch a random proc, "resisting the debuff to 10 HPs"): Touch of Cazic,
    the one he fights to 10, a 25% chance every 30 s (random, on average every 120 s as the death touch's stand-in, the
    6,500 kick, comes: that kick is left as it is).
  classtemplates "siliskor" (the same site's Siliskor guides: "an AE debuff, that will drop your HP Max to 700,
    regardless of class, level or resists", "a DR based AoE, which debuffs you to 700, and does 5 ticks of 150 damage",
    "reports of the tick being reduced to 15-16 with 315 DR", "They can stand at 30 range and not get hit", "every 5
    melee rounds or so"): Siliskor's Blight every 20 s from 6 s, within 25 in its sight, to 700, 150 a tick of disease
    at no DR, 16 at the cap, for 27 s (five ticks).

Each is added after the template's abilities as they are; a template that already holds a sethp, or would pass
the 12 the server reads, refuses the run. A name whose npctemplate row names a template OTHER names share (the Live
shard's dreadnaught and cazicthule on "roamer") is left as it is unless --fork-shared: then a template of that name is
made, a copy of the shared row (its figures, melee, crit) with the sethp, and the name's npctemplate row pointed at it.
A template not there: another shard, said and passed by. Report only by default; --apply in one transaction, a
gm_audit line a row, log sethp_bosses_1113_log; --undo exact (a row changed since left and said).

    tools/sethp_bosses_20261113.py --db eqoa.db [--fork-shared] [--apply | --undo]
"""
import argparse
import os
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import fix_moogie_tunare_20261110 as fm  # noqa: E402  the writer (gm_audit, log, exact undo)

LOG = "sethp_bosses_1113_log"
DETAIL = "bosses that set hit points (the owner, 2026-11-13: dreadnaught 700 + DoT, Cazic Thule 10): tools/sethp_bosses_20261113.py"

# (npctemplate name, its own template on Heirloom, the sethp)
BOSSES = [
    ("dreadnaught", "dreadnaught",
     {"kind": "sethp", "name": "Dread Curse", "set_hp": 700, "radius": 30, "every": 30, "first": 2,
      "tick": 650, "tick_at_cap": 80, "resist": "CR"}),
    ("cazicthule", "iod cazicthule",
     {"kind": "sethp", "name": "Touch of Cazic", "set_hp": 10, "every": 30, "first": 10, "chance": 25}),
    ("siliskor", "siliskor",
     {"kind": "sethp", "name": "Siliskor's Blight", "set_hp": 700, "radius": 25, "every": 20, "first": 6,
      "tick": 150, "tick_at_cap": 16, "resist": "DR", "tick_for": 27}),
]
MOST = 12


def describe(a):
    who = f"everyone within {a['radius']} in its sight" if a.get("radius") else "the one it fights"
    s = f"{a['name']}: {who} to {a['set_hp']} HP, every {a['every']} s from {a.get('first', 0)} s"
    if a.get("chance"):
        s += f" at {a['chance']}%"
    if a.get("tick"):
        s += f"; {a['tick']} a tick of {a['resist']} ({a['tick_at_cap']} at the cap) for {a.get('tick_for', 24)} s"
    return s


def plan(db, fork_shared):
    """(changes, lines, refusals): the rows to write."""
    changes, lines, refusals = [], [], []
    for name, own, ability in BOSSES:
        link = fm.row_of(db, "npctemplate", {"name": name})
        if link is None:
            lines.append(f"{name}: no npctemplate row here: passed by")
            continue
        tname = link["template"]
        tmpl = fm.row_of(db, "classtemplates", {"name": tname})
        if tmpl is None:
            lines.append(f"{name}: its template {tname!r} is not here: passed by")
            continue
        shared = db.execute("SELECT count(*) FROM npctemplate WHERE template = ? AND name != ?", (tname, name)).fetchone()[0]
        try:
            abilities = fm.json.loads(tmpl["abilities"] or "[]")
        except ValueError:
            refusals.append(f"{tname!r}'s abilities are not JSON: {tmpl['abilities']}")
            continue
        if any(a.get("kind") == "sethp" for a in abilities):
            refusals.append(f"{tname!r} holds a sethp already: {tmpl['abilities']}")
            continue
        if len(abilities) + 1 > MOST:
            refusals.append(f"{tname!r} has {len(abilities)} abilities: one more passes the {MOST} the server reads")
            continue
        if shared and tname != own:
            if not fork_shared:
                lines.append(f"{name}: on {tname!r}, which {shared} other names share: left (--fork-shared makes it its own)")
                continue
            if fm.row_of(db, "classtemplates", {"name": name}) is not None:
                refusals.append(f"{name}: a template {name!r} is there already, not its npctemplate's: look first")
                continue
            new = dict(tmpl, name=name, abilities=fm.abilities_json(abilities + [ability]), source="sethp1113",
                       note=f"{name}: {tname!r}'s figures as they were on 2026-11-13 and a sethp "
                            f"(tools/sethp_bosses_20261113.py). {tname!r}'s note: " + (tmpl["note"] or ""))
            changes.append(("classtemplates", None, new))
            changes.append(("npctemplate", link, dict(link, template=name)))
            lines.append(f"{name}: a template of its own, {tname!r}'s copy, its npctemplate row pointed at it; {describe(ability)}")
            continue
        changes.append(("classtemplates", tmpl, dict(tmpl, abilities=fm.abilities_json(abilities + [ability]))))
        lines.append(f"classtemplates {tname!r} ({len(abilities)} abilities before): {describe(ability)}")
    return changes, lines, refusals


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    ap.add_argument("--fork-shared", action="store_true", help="give a name on a shared template one of its own")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    fm.LOG, fm.DETAIL = LOG, DETAIL  # its log and its audit's words, through the raid gods' tool's writer
    fm.LOG_SCHEMA = fm.LOG_SCHEMA.replace("moogie_tunare_1110_log", LOG)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        if a.undo:
            db.execute("BEGIN IMMEDIATE")
            try:
                back, left = fm.undo(db)
                db.execute("COMMIT")
            except BaseException:
                db.execute("ROLLBACK")
                raise
            print(f"put back {back} rows, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        if fm.table(db, LOG):
            print(f"applied already ({LOG} is there): --undo first")
            return 1
        changes, lines, refusals = plan(db, a.fork_shared)
        for line in lines:
            print(line)
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not changes:
            print("nothing to do")
            return 0
        if not a.apply:
            print(f"report only: {len(changes)} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            n = fm.apply(db, changes)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {n} rows, each in gm_audit and {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
