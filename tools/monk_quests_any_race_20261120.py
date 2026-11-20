#!/usr/bin/env python3
"""The western monastery's monk quests for a monk of any race (petition 2458, Heirloom, a player: "Fungusman monk spawns
near Qeynos and the can't get starter quest.").

Every race's monks start at the western monastery (tools/monk_start.py, the owner 2026-11-17), but its class quests, Master
T`an Chen's and Mu Nan's chain from The Guild's Gift (level 1) to The Prison Uprising (level 20), name race 0 alone, the
western human, so a monk of any other race stands among them offered nothing. Their races become every race of the game
(0 to 10): a race of the server's own (racedefs) passes as its wire race (handlers.questRaceTakes), so every race's monk
takes them; the class (4) still keeps them a monk's. A quest of these that names races is still a city's class quest
(handlers.cityClassQuestsLocked), so the homelands' classquest events go on finding them.

The quests are found by giver, class and races (classes "4", races "0", given by Master T`an Chen or Mu Nan), never by
id. For both shards: on Live only western humans play monks today, and the change takes nothing from them.

WHEN IT TAKES EFFECT: the quest engine reads quests only at its start or a GM panel save that changes a quest row: run it
as a keep restart's before.sh (or save one of these rows in the panel after it).

Report only by default; --apply writes it, each row with its gm_audit line and its line in monk_quests_1120_log; --undo
puts every races column back as it was (refused for a row changed since) and drops the log.

    python3 -E -s tools/monk_quests_any_race_20261120.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

LOG = "monk_quests_1120_log"
GIVERS = ("Master T`an Chen", "Mu Nan")
RACES = "0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10"
DETAIL = "petition 2458 (Heirloom): the western monastery's monk quests for a monk of any race; tools/monk_quests_any_race_20261120.py"


def audit(db, qid, before, after, detail):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    return db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                      "VALUES (?, 0, 'dab''s sessions', 'tool', '', 'update', 'quests', ?, ?, ?, ?)",
                      (at, json.dumps({"questid": qid}), json.dumps({"races": before}), json.dumps({"races": after}), detail)).lastrowid


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"no database at {a.db}")
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    has_log = db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (LOG,)).fetchone() is not None
    if a.undo:
        if not has_log:
            sys.exit(f"nothing to undo: no {LOG}")
        rows = db.execute(f"SELECT questid, before, after FROM {LOG} ORDER BY questid").fetchall()
        for qid, before, after in rows:
            now = db.execute("SELECT races FROM quests WHERE questid = ?", (qid,)).fetchone()
            if now is None or now[0] != after:
                sys.exit(f"undo refused: quest {qid}'s races are {now and now[0]!r}, not {after!r} as written: nothing done")
        db.execute("BEGIN IMMEDIATE")
        for qid, before, after in rows:
            db.execute("UPDATE quests SET races = ? WHERE questid = ?", (before, qid))
            audit(db, qid, after, before, "undo: " + DETAIL)
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
        print(f"undone: {len(rows)} quests' races put back")
        return
    if has_log:
        sys.exit(f"{LOG} is there already: applied before (--undo first, or look)")
    q = ", ".join("?" * len(GIVERS))
    rows = db.execute(f"SELECT questid, name, giver, minlevel, races FROM quests WHERE giver IN ({q}) "
                      "AND replace(classes, ' ', '') = '4' AND replace(races, ' ', '') = '0' ORDER BY minlevel, questid", GIVERS).fetchall()
    if not rows:
        sys.exit("no monastery monk quest names race 0 alone: nothing to do")
    for qid, name, giver, lvl, races in rows:
        print(f"quest {qid} {name!r} ({giver}, level {lvl}): races {races!r} -> {RACES!r}")
    monks = db.execute("SELECT count(*) FROM characters WHERE tclass = 4 AND race != 0").fetchone()[0] \
        if "tclass" in {r[1] for r in db.execute("PRAGMA table_info(characters)")} else "?"
    print(f"{len(rows)} quests; monks of a race other than 0 on this shard: {monks}")
    if not a.apply:
        print("report only: --apply writes it")
        return
    db.execute("BEGIN IMMEDIATE")
    db.execute(f"CREATE TABLE {LOG} (questid INTEGER PRIMARY KEY, before TEXT NOT NULL, after TEXT NOT NULL, audit INTEGER)")
    for qid, name, giver, lvl, races in rows:
        db.execute("UPDATE quests SET races = ? WHERE questid = ?", (RACES, qid))
        db.execute(f"INSERT INTO {LOG} (questid, before, after, audit) VALUES (?, ?, ?, ?)",
                   (qid, races, RACES, audit(db, qid, races, RACES, DETAIL)))
    db.execute("COMMIT")
    print(f"applied: {len(rows)} quests")


if __name__ == "__main__":
    main()
