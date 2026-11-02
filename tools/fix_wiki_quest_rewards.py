#!/usr/bin/env python3
"""Four quests given the rewards the EQOA wiki names for them, the items tools/import_wiki_items.py made (the user,
2026-11-02: "make the quest rewards give those four items").

The wiki's Item List names a quest for each of four NO TRADE items the import made and placed nowhere:
- Cauldron Bracers: "level 15 Dark Elf Warrior Quest (once)", Neriak's Hatestone Dust (1-20/neriak-nektulos/warrior/L15),
  whose last step gives the Stomp scroll: the bracers beside it;
- Traveling Boots: the Qeynos bard's level 7 quest, Messages (1-20/qeynos/bard/L7): EQOA-Data's own guide says
  "receive the Artful Strike scroll and Traveling Boots", and the quest file left the boots out while the database
  lacked them;
- Chromatic Dagger: the "Halas Magician Armor Quest", Bloodmaw (17/halas/main), and Chromatic Gloves: the "Fayspires
  Magician Armor Quest", The Necromancer of Sslathis (17/fayspire/main): each quest's last step gives every class
  but the magician an item of its own (byclass), the magician's place empty.

Each quest by its import key (quest_import_quests), each item by its name (one record of it, else refused): an item
reward is added to the last step's rewarditems, a class's to the byclass map of the step that holds one, only while
it is not there already and the magician's place is empty (a GM's change is left, and said). Logged in
wiki_quest_rewards_log (the step, the column, before and after); --undo puts back each still as this left it, then
drops the log. Report only by default. The quest engine reads the quests at its start and at once at a quest's save in
the GM panel. tools/quests/data/ holds the same rewards for a fresh import (run tools/import_wiki_items.py first, or the
four quests import as drafts).

    tools/fix_wiki_quest_rewards.py [--db eqoa.db] [--apply | --undo]
"""
import argparse
import json
import os
import sqlite3
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
MAGICIAN = 10
# (quest key, item name, class or None: the step's own reward)
REWARDS = [
    ("1-20/neriak-nektulos/warrior/L15", "Cauldron Bracers", None),
    ("1-20/qeynos/bard/L7", "Traveling Boots", None),
    ("17/halas/main", "Chromatic Dagger", MAGICIAN),
    ("17/fayspire/main", "Chromatic Gloves", MAGICIAN),
]
LOG_DDL = """CREATE TABLE IF NOT EXISTS wiki_quest_rewards_log (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    questid INTEGER NOT NULL,
    step    INTEGER NOT NULL,
    col     TEXT NOT NULL,     -- rewarditems | byclass
    before  TEXT NOT NULL,
    after   TEXT NOT NULL,
    applied TEXT NOT NULL DEFAULT (datetime('now'))
)"""


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def plan(db):
    """[(questid, step, column, before, after, what)] and [(what, why)] left."""
    todo, left = [], []
    for key, item, cls in REWARDS:
        what = f"{key}: {item}" + (f" for class {cls}" if cls is not None else "")
        row = db.execute("SELECT questid FROM quest_import_quests WHERE key = ?", (key,)).fetchone()
        if not row:
            left.append((what, "no such quest imported"))
            continue
        qid = row[0]
        ids = [pid for (pid,) in db.execute("SELECT patternid FROM itempattern WHERE lower(itemname) = lower(?)", (item,))]
        if len(ids) != 1:
            left.append((what, f"{len(ids)} records of that name (tools/import_wiki_items.py makes one)"))
            continue
        pid = ids[0]
        if cls is None:
            step, have = db.execute("SELECT step, rewarditems FROM queststeps WHERE questid = ? ORDER BY step DESC LIMIT 1",
                                    (qid,)).fetchone()
            parts = [p.strip() for p in (have or "").split(",") if p.strip()]
            if any(p.split("*")[0].strip() == str(pid) for p in parts):
                continue  # given already
            todo.append((qid, step, "rewarditems", have or "", ",".join(parts + [str(pid)]), what))
        else:
            r = db.execute("SELECT step, byclass FROM queststeps WHERE questid = ? AND byclass != '' ORDER BY step DESC LIMIT 1",
                           (qid,)).fetchone()
            if not r:
                left.append((what, "no step gives the classes their own"))
                continue
            step, have = r
            m = json.loads(have)
            if str(cls) in m:
                if str(pid) not in str(m[str(cls)].get("items", "")):
                    left.append((what, f"class {cls} has a reward already: {m[str(cls)]}"))
                continue
            m[str(cls)] = {"items": str(pid)}
            todo.append((qid, step, "byclass", have, json.dumps(dict(sorted(m.items())), separators=(",", ":")), what))
    return todo, left


def apply(db, todo):
    db.execute(LOG_DDL)
    done = 0
    for qid, step, col, before, after, _what in todo:
        cur = db.execute(f"UPDATE queststeps SET {col} = ? WHERE questid = ? AND step = ? AND {col} = ?", (after, qid, step, before))
        if cur.rowcount:
            db.execute("INSERT INTO wiki_quest_rewards_log (questid, step, col, before, after) VALUES (?, ?, ?, ?, ?)",
                       (qid, step, col, before, after))
            done += 1
    return done


def undo(db):
    if not has_table(db, "wiki_quest_rewards_log"):
        print("nothing logged: nothing to undo")
        return
    back = kept = 0
    for qid, step, col, before, after in db.execute(
            "SELECT questid, step, col, before, after FROM wiki_quest_rewards_log ORDER BY seq DESC").fetchall():
        cur = db.execute(f"UPDATE queststeps SET {col} = ? WHERE questid = ? AND step = ? AND {col} = ?", (before, qid, step, after))
        back, kept = (back + 1, kept) if cur.rowcount else (back, kept + 1)
    db.execute("DROP TABLE wiki_quest_rewards_log")
    print(f"put back {back}; left {kept} changed since")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", default=DEFAULT_DB)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the rewards (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply wrote")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 10000")
    if a.undo:
        db.execute("BEGIN IMMEDIATE")
        undo(db)
        db.execute("COMMIT")
        return
    todo, left = plan(db)
    for qid, step, col, before, after, what in todo:
        print(f"  quest {qid} step {step}: {what} ({col} {before!r} -> {after!r})")
    for what, why in left:
        print(f"  left: {what}: {why}")
    print(f"{len(todo)} rewards to add, {len(left)} left")
    if not a.apply:
        print("report only: --apply to write")
        return
    db.execute("BEGIN IMMEDIATE")
    done = apply(db, todo)
    db.execute("COMMIT")
    print(f"added {done} rewards: the quest engine reads them at its next start, or at once at a quest's save in the GM panel")


if __name__ == "__main__":
    main()
