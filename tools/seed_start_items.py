#!/usr/bin/env python3
"""A new character's starting items as Sandstorm's (the user, 2026-10-29: "we
need starting weapons, sandstorm has it correct").

Sandstorm's new characters each carry a starting weapon (an Apprentice
Sword, Club, Dagger or Staff) and twenty of their race's rations, none
worn; this server's started with nothing, so a new player bought a weapon
(the Worn Club, which could not strike). eqoa-rust-headless's faction survey
made a new character of each of defaultcharacter's combos on Sandstorm and
kept its inventory (--measured: its folder of <race>-<class>-<humtype>.json):
each combo's items go into startitems (database/startitems.go), which
CreateNew gives, carried.

An item is ours by Sandstorm's id where our record of that id has its name,
else by its name; one this database lacks is made from Sandstorm's record
(tools/import_sandstorm_harvest.py's item_row), as its id where that is free
(the froglok's toad snacks). Rows are written with source "sandstorm" and
logged in startitems_seed, with the items made; --undo deletes the rows and
the items no character holds (said), and drops the log; --apply is refused
while the log is there. Both copy the database first
(<db>.bak-<when>-pre-startitems). Report only by default. No restart: the
server reads startitems at each creation.

    tools/seed_start_items.py [--db eqoa.db] --measured <survey folder> [--apply | --undo]
"""
import argparse
import datetime
import glob
import json
import os
import shutil
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import import_sandstorm_harvest as harvest  # noqa: E402

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
SOURCE = "sandstorm"
LOG_SCHEMA = """CREATE TABLE startitems_seed (
    what TEXT NOT NULL,     -- 'row' a startitems row written, 'item' an itempattern row made
    id   INTEGER NOT NULL   -- its id
)"""


def key(name):
    return " ".join((name or "").replace("`", "'").lower().split())


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def survey(folder):
    """{(race, class, humtype): [ware]} off the survey's files."""
    out = {}
    for f in sorted(glob.glob(os.path.join(folder, "*.json"))):
        with open(f) as fh:
            d = json.load(fh)
        c = d.get("combo")
        if not c or "inventory" not in d:
            continue
        out[(c["race"], c["class"], c.get("humtype", 0))] = d["inventory"]
    return out


def plan(db, combos):
    """The rows [(race, class, humtype, patternid, count, name)], the items to
    make {sandstorm id: (row, name)}, and notes."""
    cols = [r[1] for r in db.execute("PRAGMA table_info(itempattern)")]
    by_name = {}
    for pid, name in db.execute("SELECT patternid, itemname FROM itempattern ORDER BY patternid"):
        by_name.setdefault(key(name), []).append(pid)
    rows, make, notes = [], {}, []
    for (race, cls, hum), inv in sorted(combos.items()):
        for w in inv:
            same = db.execute("SELECT itemname FROM itempattern WHERE patternid = ?", (w["id"],)).fetchone()
            if same and key(same[0]) == key(w["name"]):
                pid = w["id"]
            elif len(by_name.get(key(w["name"]), [])) >= 1:
                pid = by_name[key(w["name"])][0]
            else:
                if w["id"] not in make:
                    row, _ = harvest.item_row(w)
                    free = not same
                    row = {c: v for c, v in row.items() if c in cols}
                    if not free:
                        row.pop("patternid")
                    row.setdefault("model", 0)
                    row.setdefault("color", 0)
                    make[w["id"]] = (row, w["name"])
                pid = ("made", w["id"])
            rows.append((race, cls, hum, pid, max(int(w.get("quantity", 1)), 1), w["name"]))
    no_weapon = [c for c, inv in combos.items() if not any(w.get("attack_type", 0) > 0 for w in inv)]
    if no_weapon:
        notes.append(f"combos Sandstorm gives no weapon: {sorted(no_weapon)}")
    return rows, make, notes


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-startitems"
    shutil.copy2(path, dst)
    return dst


def apply(db, rows, make):
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_SCHEMA)
        made = {}
        for sid, (row, name) in make.items():
            cols = list(row)
            cur = db.execute(f"INSERT INTO itempattern ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                             [row[c] for c in cols])
            made[sid] = cur.lastrowid
            db.execute("INSERT INTO startitems_seed VALUES ('item', ?)", (cur.lastrowid,))
        for race, cls, hum, pid, n, name in rows:
            if isinstance(pid, tuple):
                pid = made[pid[1]]
            cur = db.execute("INSERT INTO startitems (race, class, humtype, patternid, count, note, source) VALUES (?, ?, ?, ?, ?, ?, ?)",
                             (race, cls, hum, pid, n, f"Sandstorm's: {name} (faction survey)", SOURCE))
            db.execute("INSERT INTO startitems_seed VALUES ('row', ?)", (cur.lastrowid,))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db):
    if not has_table(db, "startitems_seed"):
        print("nothing logged: nothing to undo")
        return
    db.execute("BEGIN IMMEDIATE")
    try:
        rows = [i for (i,) in db.execute("SELECT id FROM startitems_seed WHERE what = 'row'")]
        for i in rows:
            db.execute("DELETE FROM startitems WHERE id = ? AND source = ?", (i, SOURCE))
        kept = 0
        for (i,) in db.execute("SELECT id FROM startitems_seed WHERE what = 'item'").fetchall():
            held = db.execute("SELECT count(*) FROM charinventory WHERE patternid = ?", (i,)).fetchone()[0]
            if held:
                print(f"  item {i}: {held} characters hold it: left")
                kept += 1
                continue
            db.execute("DELETE FROM itempattern WHERE patternid = ?", (i,))
        db.execute("DROP TABLE startitems_seed")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"took back {len(rows)} rows; {kept} items made left (held)")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--measured", help="the faction survey's folder (<race>-<class>-<humtype>.json)")
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the rows (logged)")
    g.add_argument("--undo", action="store_true", help="take back what --apply wrote")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if not has_table(db, "startitems"):
        sys.exit(f"{a.db}: no startitems table (start the server once: database/startitems.go makes it)")
    if a.undo:
        if not a.no_backup and has_table(db, "startitems_seed"):
            print(f"copy: {backup(a.db)}")
        undo(db)
        return
    if a.apply and has_table(db, "startitems_seed"):
        sys.exit("startitems_seed is there: seeded already (--undo first)")
    if not a.measured:
        sys.exit("--measured is required (the faction survey's folder)")
    combos = survey(a.measured)
    rows, make, notes = plan(db, combos)
    for c in sorted(combos):
        print(f"  {c}: " + ", ".join(f"{n} x{cnt}" for r, cl, h, p, cnt, n in rows if (r, cl, h) == c))
    for sid, (row, name) in make.items():
        print(f"  made from Sandstorm's record: {name} ({'as its id ' + str(sid) if 'patternid' in row else 'a new id'})")
    for n in notes:
        print(f"note: {n}")
    print(f"{len(combos)} combos, {len(rows)} rows, {len(make)} items to make")
    if not a.apply:
        print("report only: --apply to write")
        return
    if not a.no_backup:
        print(f"copy: {backup(a.db)}")
    apply(db, rows, make)
    print(f"wrote {len(rows)} rows; a new character carries them from now on")


if __name__ == "__main__":
    main()
