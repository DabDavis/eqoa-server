#!/usr/bin/env python3
"""Quil'Eyne tuned to the evil-tank guide (2026-11-12; the owner, after his group killed her: "he needs tuned.
https://jadiction.com/en/eqoa/guides/60/evil-tank").

The guide (episode 3, step 7): "Quil'Eyne is extremely hard — with a 2000 hit quad, 2000 nuke, 2000 critical, 2000
damage (looks like taunt) spell, and a small proc — he is very hard to MT", "Bring an army for this — preferably
petters", "Quil'Eyne will drop 4 bloods and 4 journals". It names no level, hit points or respawn.

Her class template "epic quileyne" (the GM Jibrish's epic audit, board entries 633/1130, applied 2026-11-12 03:02 UTC)
already has the 2,000 quad (melee_pct 1497 on a level 65 warrior target), the 2,000 critical (special Critical Hit),
a 2,000 nuke (Blood Strike) and the small proc (Drain, 500 every 10.5 s), at level 65 and 1,619,982 hit points (hp_pct
9660: the army the guide asks for). The guide's fourth, the "2000 damage (looks like taunt) spell", was missing: it is
added as Vampiric Taunt (a 2,000 on her target; the name ours). The three 2,000s each come every 120 s, 40 s apart
(the critical first at 6 s, the nuke at 46 s, the taunt at 86 s): the tapes' one 2,000 every ~40 s (6.0, 44.5 and
87.2 s, entry 633) kept, now as the guide's three kinds where it was two kinds every 80 s. Level, hit points, melee,
the Drain and her respawn (90 s, the owner's named rule of 2026-11-11, petfix259) are left as they are.

Nothing is done unless the template is as the audit wrote it (its abilities, special and figures) and npctemplate
"quil'eyne" names it; no such template: another shard, nothing done. Report only by default; --apply in one
transaction with a gm_audit line, log quil_1112_log; --undo exact (a row changed since is left and said). The
templates are read at the server's start or a GM panel template edit (reload_npcs): no restart needed with the panel's
reload, else at the next restart.

    tools/quil_20261112.py --db eqoa.db [--apply | --undo]
"""
import argparse
import os
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import fix_moogie_tunare_20261110 as fm  # noqa: E402  the house writer (apply, undo, gm_audit)

LOG = "quil_1112_log"
DETAIL = ("Quil'Eyne tuned to jadiction's evil-tank guide (the owner, 2026-11-12: \"he needs tuned\"): "
          "tools/quil_20261112.py")
NPC_KEY, TEMPLATE = "quil'eyne", "epic quileyne"

ABILITIES_NOW = [
    {"kind": "nuke", "damage": 2000, "every": 80, "first": 46, "name": "Blood Strike"},
    {"kind": "nuke", "damage": 500, "every": 10.5, "first": 8.8, "name": "Drain"},
]
ABILITIES_NEW = [
    {"kind": "nuke", "damage": 2000, "every": 120, "first": 46, "name": "Blood Strike"},
    {"kind": "nuke", "damage": 2000, "every": 120, "first": 86, "name": "Vampiric Taunt"},
    {"kind": "nuke", "damage": 500, "every": 10.5, "first": 8.8, "name": "Drain"},
]
# the template's columns as the audit wrote them, and as this tool leaves them
EXPECT = {"class": 0, "hp_pct": 9660, "taken_pct": 100, "melee_pct": 1497, "special": "Critical Hit",
          "special_damage": 30.77, "special_every": 80.0, "special_first": 6.0}
CHANGE = {"special_every": 120.0}
NOTE_ADD = (" | quil_20261112 (the owner, 2026-11-12; jadiction's evil-tank guide: \"2000 hit quad, 2000 nuke, 2000 "
            "critical, 2000 damage (looks like taunt) spell, and a small proc\"): the taunt-like 2,000 added (Vampiric "
            "Taunt, name ours); the crit, the nuke and it each every 120 s, 40 s apart (6/46/86), the tapes' one 2,000 "
            "every ~40 s kept")


def plan(db):
    """(changes, lines, refusals, here)."""
    m = db.execute("SELECT template FROM npctemplate WHERE name = ?", (NPC_KEY,)).fetchone()
    t = fm.row_of(db, "classtemplates", {"name": TEMPLATE})
    if m is None and t is None:
        return [], [f"no npctemplate {NPC_KEY!r} nor class template {TEMPLATE!r} here: not this shard"], [], False
    refusals = []
    if m is None or m[0].strip().lower() != TEMPLATE:
        refusals.append(f"npctemplate {NPC_KEY!r} names {None if m is None else m[0]!r}, not {TEMPLATE!r}")
    if t is None:
        refusals.append(f"no class template {TEMPLATE!r}")
        return [], [], refusals, True
    for c, v in EXPECT.items():
        if (round(float(t[c]), 4) if isinstance(v, float) else t[c]) != v:
            refusals.append(f"{TEMPLATE!r}.{c} is {t[c]!r}, not the audit's {v!r}")
    if not fm.same_abilities(t["abilities"], ABILITIES_NOW):
        refusals.append(f"{TEMPLATE!r}'s abilities are not the audit's two: {t['abilities']}")
    if refusals:
        return [], [], refusals, True
    after = dict(t, abilities=fm.abilities_json(ABILITIES_NEW), note=t["note"] + NOTE_ADD, **CHANGE)
    lines = [
        f"classtemplates {TEMPLATE!r}: Critical Hit (special, 2,000 at L65) every 80 -> 120 s, first 6 s",
        "  Blood Strike (2,000 nuke) every 80 -> 120 s, first 46 s",
        "  + Vampiric Taunt (2,000 nuke on her target, the guide's 'looks like taunt' spell) every 120 s, first 86 s",
        "  Drain (500 every 10.5 s), level 65, hp_pct 9660 (1,619,982 HP), melee_pct 1497 and the 90 s respawn as they are",
    ]
    return [("classtemplates", t, after)], lines, [], True


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    fm.LOG_SCHEMA = fm.LOG_SCHEMA.replace(fm.LOG, LOG)
    fm.LOG, fm.DETAIL = LOG, DETAIL
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
        changes, lines, refusals, here = plan(db)
        for line in lines:
            print(line)
        if not here:
            print("nothing done")
            return 0
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not a.apply:
            print(f"report only: {len(changes)} row would be written (--apply writes it)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            n = fm.apply(db, changes)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {n} row, in gm_audit and {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
