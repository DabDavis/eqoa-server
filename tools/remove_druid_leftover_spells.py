#!/usr/bin/env python3
"""The druid's four EQOA-Data spells nothing teaches any more, out of the
class's list and out of every book (the user, 2026-11-01: "remove them from
both").

EQOA-Data's import gave the druid Nature Form (701064), Wild Nature (701258),
Bixie Form (701107) and Sacred Spring (700670). Since the EQOAEmu workbook's
import (2026-11-01) the masteries give the workbook's own spells in their
place (the Nature Form mastery its Spirit Form, the Wild Nature mastery Wild
Nature: Faun and Wild Nature: Satyr, each with its body), and no mastery,
scroll or quest teaches any of the four; the characters holding one (five on
live, 2026-11-01) had it from the start-with-every-spell setting. The four
were still in the class's list (classspells), where the balance simulator's
kits read a class's spells, and in those books.

So the druid's classspells row of each and every book's row of it (spells)
go, each logged whole in druid_leftover_spells_log; their records
(spellpattern, spelleffect) stay, so an effect of one still on a character
runs out as it is. A spell whose record is no longer the one named here is
refused, not removed. --undo puts back every row still gone, at its own
rowid, and drops the log. Report only by default; a copy of the database
first (SQLite's backup API) unless --no-backup. The server reads a book at a
login and the class's list at its start.

    tools/remove_druid_leftover_spells.py [--db eqoa.db] [--apply | --undo] [--no-backup]
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
DRUID = 7
LEFTOVERS = [(701064, "Nature Form"), (701258, "Wild Nature"), (701107, "Bixie Form"), (700670, "Sacred Spring")]
LOG = "druid_leftover_spells_log"
LOG_SCHEMA = f"""CREATE TABLE {LOG} (
    seq     INTEGER PRIMARY KEY,
    tbl     TEXT NOT NULL,     -- classspells or spells
    rid     INTEGER NOT NULL,  -- the row's rowid
    data    TEXT NOT NULL,     -- the row, JSON of its columns
    note    TEXT NOT NULL,
    applied TEXT NOT NULL
)"""


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def columns(db, table):
    return [r[1] for r in db.execute(f"PRAGMA table_info({table})")]


def plan(db):
    """(rows, refused): rows [(table, rowid, {column: value}, note)] to remove,
    refused [why] for a spell whose record is not the one named."""
    rows, refused = [], []
    ccols, scols = columns(db, "classspells"), columns(db, "spells")
    for sid, name in LEFTOVERS:
        rec = db.execute("SELECT spellname FROM spellpattern WHERE spellid = ?", (sid,)).fetchone()
        if rec is None or (rec[0] or "").strip() != name:
            refused.append(f"{sid}: its record is {rec[0] if rec else 'gone'!r}, not {name!r}")
            continue
        for r in db.execute(f"SELECT rowid, {', '.join(ccols)} FROM classspells WHERE class = ? AND spellid = ?",
                            (DRUID, sid)).fetchall():
            rows.append(("classspells", r[0], dict(zip(ccols, r[1:])), f"{name}: the druid's list"))
        for r in db.execute(f"SELECT s.rowid, {', '.join('s.' + c for c in scols)}, coalesce(c.charName, '?'), "
                            "coalesce(c.level, 0), coalesce(c.tclass, -1) FROM spells s "
                            "LEFT JOIN characters c ON c.serverid = s.serverid WHERE s.spellid = ? ORDER BY s.rowid",
                            (sid,)).fetchall():
            data = dict(zip(scols, r[1:1 + len(scols)]))
            who, lvl, cls = r[1 + len(scols):]
            rows.append(("spells", r[0], data, f"{name}: the book of {who} (level {lvl}, class {cls})"))
    return rows, refused


def backup(path):
    stamp = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{stamp}-pre-druid-leftovers"
    src = sqlite3.connect(path)
    out = sqlite3.connect(dst)
    with out:
        src.backup(out)
    out.close()
    src.close()
    return dst


def apply(db, rows):
    now = datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")
    db.execute("BEGIN IMMEDIATE")
    if not has_table(db, LOG):
        db.execute(LOG_SCHEMA)
    for tbl, rid, data, note in rows:
        db.execute(f"INSERT INTO {LOG} (tbl, rid, data, note, applied) VALUES (?, ?, ?, ?, ?)",
                   (tbl, rid, json.dumps(data, sort_keys=True), note, now))
        db.execute(f"DELETE FROM {tbl} WHERE rowid = ?", (rid,))
    db.execute("COMMIT")


def undo(db):
    """(put back, left): a row goes back at its rowid unless that rowid or its
    key is taken again since."""
    db.execute("BEGIN IMMEDIATE")
    back = left = 0
    for tbl, rid, data in db.execute(f"SELECT tbl, rid, data FROM {LOG} ORDER BY seq").fetchall():
        data = json.loads(data)
        taken = db.execute(f"SELECT count(*) FROM {tbl} WHERE rowid = ?", (rid,)).fetchone()[0]
        if tbl == "classspells":
            taken += db.execute("SELECT count(*) FROM classspells WHERE class = ? AND spellid = ?",
                                (data["class"], data["spellid"])).fetchone()[0]
        else:
            taken += db.execute("SELECT count(*) FROM spells WHERE serverid = ? AND spellid = ?",
                                (data["serverid"], data["spellid"])).fetchone()[0]
        if taken:
            left += 1
            continue
        cols = list(data)
        if tbl == "spells":  # its id is the rowid itself
            db.execute(f"INSERT INTO spells ({', '.join(cols)}) VALUES ({', '.join('?' for _ in cols)})",
                       [data[c] for c in cols])
        else:
            db.execute(f"INSERT INTO {tbl} (rowid, {', '.join(cols)}) VALUES (?, {', '.join('?' for _ in cols)})",
                       [rid] + [data[c] for c in cols])
        back += 1
    db.execute(f"DROP TABLE {LOG}")
    db.execute("COMMIT")
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--no-backup", action="store_true")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="remove the rows (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply removed")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    if (a.apply or a.undo) and not a.no_backup:
        print("backup:", backup(a.db))
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        if not has_table(db, LOG):
            print("nothing logged: nothing to undo")
            return
        back, left = undo(db)
        print(f"put back {back} rows, {left} left (their place taken since)")
        return
    rows, refused = plan(db)
    for why in refused:
        print("refused:", why)
    for tbl, rid, data, note in rows:
        print(f"  {tbl:11} rowid {rid:>7}  {note}")
    print(f"{len(rows)} rows to remove ({sum(1 for r in rows if r[0] == 'classspells')} from the druid's list, "
          f"{sum(1 for r in rows if r[0] == 'spells')} from books), {len(refused)} spells refused")
    if a.apply:
        apply(db, rows)
        print(f"removed {len(rows)} rows, logged in {LOG}")


if __name__ == "__main__":
    main()
