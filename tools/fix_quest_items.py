#!/usr/bin/env python3
"""Quest items made as copies of another item take Sandstorm's own record
where it has one (the user, 2026-10-29: "bone bracers for sk level 1 quest is
wrong, they are level 42. sandstorm harvest prob fixed it").

tools/import_quests.py makes a quest file's item that no item of its name is
as a copy of its "like" item with its name and description (trade 1 when
notrade), so the copy keeps the like item's level, classes, races, slot,
stats and icon: Bone Bracers, a copy of Whale Bone Bracers, was a level 42
item of other classes in the shadow knight's level 1 quest. Sandstorm's
records (its merchants' wares in the harvests, --prices, as
tools/stock_merchants.py reads them) hold 61 of the 1,402 under the same
name: mostly starting-quest gear of one race and class at level 1 (Bone
Bracers: a shadow knight's and a dark elf's, level 1, +3 STR, free from a
Neriak merchant), tradeskill parts and handouts. Each of those takes
Sandstorm's record: every column a ware fills
(tools/import_sandstorm_harvest.py's item_row: value, icons, slot, trade,
attack type, damage, range, level, stack, condition, classes, races and the
rest) and its stats, its id and name kept (the quests name it by id) and its
model and colour too (no ware carries them). An item Sandstorm gives two
records, or one a GM changed (gm_audit), is left and said.

Every change is logged in quest_items_fix_log (the pattern, the row before
and after); --undo puts back each row still as this left it and drops the
log. Report only by default. Run it before tools/stock_merchants.py (which
prices only items at 0); restart the server after (clients get item records
at their login).

    tools/fix_quest_items.py [--db eqoa.db] --prices <harvest JSON or folder> ... [--apply | --undo]
"""
import argparse
import datetime
import glob
import json
import os
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import import_sandstorm_harvest as harvest  # noqa: E402

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
KEEP = ("patternid", "itemname")

LOG_SCHEMA = """CREATE TABLE quest_items_fix_log (
    patternid INTEGER PRIMARY KEY,
    before    TEXT NOT NULL,   -- the changed columns before, JSON
    after     TEXT NOT NULL,   -- and after
    applied   TEXT NOT NULL
)"""


def key(name):
    return " ".join((name or "").replace("`", "'").lower().split())


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def records(paths):
    """Sandstorm's item records by name: {name: [itempattern columns]}, one
    entry a distinct record."""
    files = []
    for p in paths:
        files += glob.glob(os.path.join(p, "**", "*.json"), recursive=True) if os.path.isdir(p) else [p]
    out = {}

    def walk(o):
        if isinstance(o, dict):
            if all(k in o for k in ("name", "level_req", "stats", "wear_slot", "class_mask")):
                yield o
            for v in o.values():
                yield from walk(v)
        elif isinstance(o, list):
            for v in o:
                yield from walk(v)

    for f in files:
        try:
            with open(f) as fh:
                d = json.load(fh)
        except (OSError, ValueError):
            continue
        for w in walk(d):
            row, _ = harvest.item_row(w)
            row = {c: v for c, v in row.items() if c not in KEEP}
            seen = out.setdefault(key(w["name"]), [])
            if row not in seen:
                seen.append(row)
    return out


def plan(db, recs):
    """[(patternid, name, before, after)], and what was left [(name, why)]."""
    gm = set()
    if has_table(db, "gm_audit"):
        for (k,) in db.execute("SELECT rowkey FROM gm_audit WHERE tbl = 'itempattern'"):
            try:
                gm.add(int(json.loads(k)["patternid"]))
            except (ValueError, KeyError, TypeError):
                pass
    cols = [r[1] for r in db.execute("PRAGMA table_info(itempattern)")]
    changes, left = [], []
    for pid, name in db.execute("SELECT patternid, name FROM quest_import_items ORDER BY patternid").fetchall():
        found = recs.get(key(name))
        if not found:
            continue
        if len(found) > 1:
            left.append((name, f"Sandstorm has {len(found)} records of that name"))
            continue
        if pid in gm:
            left.append((name, "a GM changed it"))
            continue
        r = db.execute("SELECT * FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
        if r is None:
            continue
        have = dict(zip(cols, r))
        # a stat Sandstorm's record lacks is NULL; ours 0 reads the same (no stat)
        want = {c: v for c, v in found[0].items()
                if c in have and have[c] != v and not (v is None and have[c] in (0, None))}
        if want:
            changes.append((pid, name, {c: have[c] for c in want}, want))
    return changes, left


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--prices", nargs="*", default=[], help="eqoa harvest JSON files or folders of them")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the records (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply changed")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        if not has_table(db, "quest_items_fix_log"):
            print("nothing logged: nothing to undo")
            return
        db.execute("BEGIN IMMEDIATE")
        back = left = 0
        for pid, before, after in db.execute("SELECT patternid, before, after FROM quest_items_fix_log").fetchall():
            before, after = json.loads(before), json.loads(after)
            c = list(after)
            now = db.execute(f"SELECT {', '.join(c)} FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
            if now is None or dict(zip(c, now)) != after:
                left += 1
                continue
            db.execute(f"UPDATE itempattern SET {', '.join(x + ' = ?' for x in c)} WHERE patternid = ?",
                       [before[x] for x in c] + [pid])
            back += 1
        db.execute("DROP TABLE quest_items_fix_log")
        db.execute("COMMIT")
        print(f"put back {back} items, {left} changed since left as they are")
        return
    if not has_table(db, "quest_import_items"):
        sys.exit(f"{a.db}: no quest_import_items (tools/import_quests.py has made no items)")
    if a.apply and has_table(db, "quest_items_fix_log"):
        sys.exit("quest_items_fix_log is there: applied already (--undo first)")
    recs = records(a.prices)
    if not recs:
        sys.exit("no Sandstorm item records: give --prices (eqoa harvest JSON)")
    changes, left = plan(db, recs)
    for pid, name, before, after in changes:
        print(f"{pid:7d} {name}: " + ", ".join(f"{c} {before[c]!r} -> {after[c]!r}" for c in after))
    for name, why in left:
        print(f"left as it is: {name} ({why})")
    print(f"{len(changes)} quest items to give Sandstorm's record, {len(left)} left; {len(recs)} of its items' records read")
    if not a.apply or not changes:
        if not a.apply:
            print("report only: --apply to write")
        return
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_SCHEMA)
        for pid, name, before, after in changes:
            db.execute("INSERT INTO quest_items_fix_log VALUES (?, ?, ?, ?)",
                       (pid, json.dumps(before, sort_keys=True), json.dumps(after, sort_keys=True), when))
            c = list(after)
            db.execute(f"UPDATE itempattern SET {', '.join(x + ' = ?' for x in c)} WHERE patternid = ?",
                       [after[x] for x in c] + [pid])
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"changed {len(changes)} quest items; restart the server")


if __name__ == "__main__":
    main()
