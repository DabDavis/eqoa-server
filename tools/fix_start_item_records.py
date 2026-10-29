#!/usr/bin/env python3
"""The starting items' records as Sandstorm's (the user, 2026-10-29: "the
starting rations are in the wrong slot, they are showing as Misc stacks.
should be magic slot as usable").

tools/seed_start_items.py gives a new character Sandstorm's starting items,
by this database's records of them, and those were not Sandstorm's:
- **The rations' item type was 0.** That is the record's +0x4C,
  itempattern.procanim. CLIENT files an item under Misc unless its type is
  2, 5, 7 or 8 (VIInfoText::IsMisc 0x00C0A3A0, IsUsable 0x00C0A3F0), and a
  type 2 item also fits the HUD's first two quick slots. Sandstorm's rations
  are type 2.
- **The rations' masks were 0, and their level 0.** Their descriptions were
  the older "Simple field rations.".
- **The Apprentice Sword left out warriors** (class mask 14 where
  Sandstorm's is 15) and gnomes, barbarians and halflings (race mask 823
  where Sandstorm's is 1023). The wear rule of the same day refused the
  sword to the warriors who start with it.

The faction survey (--measured: a new character of every combo on Sandstorm,
with its inventory) holds Sandstorm's record of each. Each item a new
character carries there takes it where ours differs: every column a ware
fills (tools/import_sandstorm_harvest.py's item_row) and its stats. Its id,
model and colour are kept (no ware carries the last two). An item is ours by
Sandstorm's id where our record of that id has its name, else by its name,
as seed_start_items.py finds it. One a GM changed (gm_audit) is left and
said.

Every change is logged in start_item_records_log (the pattern, the columns
before and after); --undo puts back each row still as this left it and drops
the log. Report only by default. A client reads item records at its login,
so a player sees the change at the next one.

    tools/fix_start_item_records.py [--db eqoa.db] --measured <survey folder> [--apply | --undo]
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import import_sandstorm_harvest as harvest  # noqa: E402
import seed_start_items as seed  # noqa: E402

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
KEEP = ("patternid", "model", "color")

LOG_SCHEMA = """CREATE TABLE start_item_records_log (
    patternid INTEGER PRIMARY KEY,
    before    TEXT NOT NULL,   -- the changed columns before, JSON
    after     TEXT NOT NULL,   -- and after
    applied   TEXT NOT NULL
)"""


def ours(db, ware):
    """This database's patternid of a ware, as seed_start_items.py finds it, or None."""
    same = db.execute("SELECT itemname FROM itempattern WHERE patternid = ?", (ware["id"],)).fetchone()
    if same and seed.key(same[0]) == seed.key(ware["name"]):
        return ware["id"]
    r = db.execute("SELECT patternid FROM itempattern WHERE lower(itemname) = ? ORDER BY patternid LIMIT 1",
                   (seed.key(ware["name"]),)).fetchone()
    return r[0] if r else None


def plan(db, combos):
    """[(patternid, name, before, after)], and what was left [(name, why)]."""
    gm = set()
    if seed.has_table(db, "gm_audit"):
        for (k,) in db.execute("SELECT rowkey FROM gm_audit WHERE tbl = 'itempattern'"):
            try:
                gm.add(int(json.loads(k)["patternid"]))
            except (ValueError, KeyError, TypeError):
                pass
    cols = [r[1] for r in db.execute("PRAGMA table_info(itempattern)")]
    wares = {}
    for inv in combos.values():
        for w in inv:
            wares.setdefault(w["id"], w)
    changes, left = [], []
    for sid, w in sorted(wares.items()):
        pid = ours(db, w)
        if pid is None:
            left.append((w["name"], "this database has no such item (seed_start_items.py makes it)"))
            continue
        if pid in gm:
            left.append((w["name"], "a GM changed it"))
            continue
        have = dict(zip(cols, db.execute("SELECT * FROM itempattern WHERE patternid = ?", (pid,)).fetchone()))
        row, _ = harvest.item_row(w)
        # a stat Sandstorm's record lacks is NULL; ours 0 reads the same (no stat)
        want = {c: v for c, v in row.items()
                if c in have and c not in KEEP and have[c] != v and not (v is None and have[c] in (0, None))}
        if want:
            changes.append((pid, w["name"], {c: have[c] for c in want}, want))
    return changes, left


def undo(db):
    if not seed.has_table(db, "start_item_records_log"):
        print("nothing logged: nothing to undo")
        return
    db.execute("BEGIN IMMEDIATE")
    try:
        back = left = 0
        for pid, before, after in db.execute("SELECT patternid, before, after FROM start_item_records_log").fetchall():
            before, after = json.loads(before), json.loads(after)
            c = list(after)
            now = db.execute(f"SELECT {', '.join(c)} FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
            if now is None or dict(zip(c, now)) != after:
                left += 1
                continue
            db.execute(f"UPDATE itempattern SET {', '.join(x + ' = ?' for x in c)} WHERE patternid = ?",
                       [before[x] for x in c] + [pid])
            back += 1
        db.execute("DROP TABLE start_item_records_log")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"put back {back} items, {left} changed since left as they are")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--measured", help="the faction survey's folder (<race>-<class>-<humtype>.json)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the records (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply changed")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        undo(db)
        return
    if a.apply and seed.has_table(db, "start_item_records_log"):
        sys.exit("start_item_records_log is there: applied already (--undo first)")
    if not a.measured:
        sys.exit("--measured is required (the faction survey's folder)")
    changes, left = plan(db, seed.survey(a.measured))
    for pid, name, before, after in changes:
        print(f"{pid:7d} {name}: " + ", ".join(f"{c} {before[c]!r} -> {after[c]!r}" for c in after))
    for name, why in left:
        print(f"left as it is: {name} ({why})")
    print(f"{len(changes)} starting items to give Sandstorm's record, {len(left)} left")
    if not a.apply or not changes:
        if not a.apply:
            print("report only: --apply to write")
        return
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_SCHEMA)
        for pid, name, before, after in changes:
            db.execute("INSERT INTO start_item_records_log VALUES (?, ?, ?, ?)",
                       (pid, json.dumps(before, sort_keys=True), json.dumps(after, sort_keys=True), when))
            c = list(after)
            db.execute(f"UPDATE itempattern SET {', '.join(x + ' = ?' for x in c)} WHERE patternid = ?",
                       [after[x] for x in c] + [pid])
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"changed {len(changes)} starting items; a player sees them at the next login")


if __name__ == "__main__":
    main()
