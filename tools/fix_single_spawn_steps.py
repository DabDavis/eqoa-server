#!/usr/bin/env python3
"""Quest loot steps that farm one respawning creature for two or more items
drop their item on every kill (the user, 2026-10-30, of tools/drop_rates.py's
report: a goblin commander's four idols at 0.5 took eight kills of one spawn,
each a respawn's wait, about twelve minutes).

A step is one when its quest is in the game, it is a loot step of chance
below 1 wanting two or more items, its camp (tools/drop_rates.py: the spawns
of its creatures, its target and its others, within tools/fix_quest_targets.py's
NEAR of the one nearest the quest's places) is one spawn, and no spawn event
of the quest brings its creature (a spawned one is the step's own each time).
Its chance becomes 1.0 (every kill: handlers/questevents.go's LootChance).

Measured on the live data of 2026-10-30 01:30 UTC: 13 steps, their rough
minutes alone (drop_rates.py's model) from 138 to 63 in all. The report of
the evening before counted 17: tools/fix_quest_targets.py has since given
Frog Legs for Grobb's step 2 (its four quests) a second creature, star crab.

Every step changed is logged in single_spawn_step_log (its chance before);
--undo puts back each one still at 1.0. Report only by default. The quest
engine reads the quests at its start and when the GM panel saves one: apply
before a (re)start, or save any quest in the panel after.

    python3 -E -s tools/fix_single_spawn_steps.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import os
import shutil
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import drop_rates  # noqa: E402

LOG_DDL = """CREATE TABLE IF NOT EXISTS single_spawn_step_log (
    questid INTEGER NOT NULL,
    step    INTEGER NOT NULL,
    before  REAL NOT NULL,
    after   REAL NOT NULL,
    applied TEXT NOT NULL,
    PRIMARY KEY (questid, step)
)"""


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def plan(db):
    """[(quest, step, chance, count, names, kills, minutes before, after)] for every step to change."""
    out = []
    for q, st, c, need, names, camp, kills, minutes, spawned in drop_rates.quest_loot(db):
        if not q["enabled"] or spawned or camp != 1 or c >= 1 or need < 2:
            continue
        after = need * max(drop_rates.KILL_MINUTES / camp, drop_rates.FASTEST_KILL)
        out.append((q, st, c, need, names, kills, minutes, after))
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="set the steps' chance to 1.0 (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply changed")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        if not has_table(db, "single_spawn_step_log"):
            print("nothing logged: nothing to undo")
            return
        if not a.no_backup:
            print(f"copy: {backup(a.db)}")
        db.execute("BEGIN IMMEDIATE")
        try:
            back, kept = 0, []
            for qid, n, old, new in db.execute("SELECT questid, step, before, after FROM single_spawn_step_log").fetchall():
                cur = db.execute("SELECT chance FROM queststeps WHERE questid = ? AND step = ?", (qid, n)).fetchone()
                if cur is not None and cur[0] == new:
                    db.execute("UPDATE queststeps SET chance = ? WHERE questid = ? AND step = ?", (old, qid, n))
                    back += 1
                elif cur is not None:
                    kept.append(f"{qid} step {n}")
            db.execute("DROP TABLE single_spawn_step_log")
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"put back {back} steps" + (f"; {len(kept)} changed since left as they are: " + ", ".join(kept) if kept else ""))
        return
    if a.apply and has_table(db, "single_spawn_step_log"):
        sys.exit("single_spawn_step_log is there: applied already (--undo first)")
    rows = plan(db)
    for q, st, c, need, names, kills, before, after in rows:
        print(f"{q['questid']} {q['name']!r} L{q['minlevel']} step {st['step']}: {c:g} x {need} off {', '.join(names)}: "
              f"{kills:.0f} kills, ~{before:.0f} min -> {need} kills, ~{after:.0f} min")
    print(f"{len(rows)} steps to 1.0; minutes alone in all {sum(r[6] for r in rows):.0f} -> {sum(r[7] for r in rows):.0f}")
    if not a.apply:
        print("report only: --apply to write")
        return
    if not rows:
        return
    if not a.no_backup:
        print(f"copy: {backup(a.db)}")
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_DDL)
        for q, st, c, need, names, kills, before, after in rows:
            db.execute("UPDATE queststeps SET chance = 1.0 WHERE questid = ? AND step = ?", (q["questid"], st["step"]))
            db.execute("INSERT INTO single_spawn_step_log VALUES (?, ?, ?, 1.0, ?)", (q["questid"], st["step"], st["chance"], when))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"set {len(rows)} steps to 1.0; save any quest in the GM panel (or restart) for the engine to read them")


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-singlespawn"
    shutil.copy2(path, dst)
    return dst


if __name__ == "__main__":
    main()
