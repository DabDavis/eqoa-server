#!/usr/bin/env python3
"""The hazards: table hazards (entity/hazards.go, database/hazards.go).

The GM Hoofbite's petition 1937, 2026-11-13: "spikey vines in toxxulia cave should damage player for 1000 dmg when
walking through them. player must navigate around vines". tools/hazards/toxxulia_vines.json is cmd/hazardscan's
reading of the disc: every zone actor of the thorny vine sprites (SimpleSprites B9F5C1A9, B9F5C1AA and B9F5C1AB,
each a thorned stem 14 long; 144 actors, all of them in Odus's zone 20, the Toxxulia cave, rooms 205-268 and 617, x
7007-7381, z 6930-7362, floors -41 to 10) as a capsule along the vine's long axis across, its radius the distance
across that holds 80% of the mesh (1 to 3), from 2 under the vine's floor to 4 over it; each hits for 1000 every 2 s
while a player stands in it ("The spiky vines tear at you for 1000.").

    python3 -E -s tools/seed_hazards.py --db eqoa.db                      report
    python3 -E -s tools/seed_hazards.py --db eqoa.db --apply              write it
    python3 -E -s tools/seed_hazards.py --db eqoa.db --undo               take it back
    --data tools/hazards/toxxulia_vines.json   one file (default: every file of tools/hazards/)

Rows go in in one transaction, source vines, each logged in hazard_seed with what was written; a run when the file's
rows are in already writes nothing. --undo deletes the rows it wrote that no GM changed since (a changed one is said
and kept) and its log, and the table when the run made it and it is empty again. The server reads the table at its
start and at a GM's edit in the panel ("Hazards"): a running server takes the rows at its next restart (or open and
save a row in the panel). Needs the server with entity/hazards.go (an older one never reads the table).
"""
import argparse
import glob
import json
import os
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
DATA_DIR = os.path.join(HERE, "hazards")
SOURCE = "vines"
LOG = "hazard_seed"
MADE = "#made the table"

# the server's own schema (database/hazards.go), for a database it has not opened yet
SCHEMA = """
	CREATE TABLE IF NOT EXISTS hazards (
		id       INTEGER PRIMARY KEY,
		name     TEXT NOT NULL DEFAULT '',
		world    INTEGER NOT NULL DEFAULT 0,
		x1       REAL NOT NULL DEFAULT 0,
		y1       REAL NOT NULL DEFAULT 0,
		z1       REAL NOT NULL DEFAULT 0,
		x2       REAL NOT NULL DEFAULT 0,
		y2       REAL NOT NULL DEFAULT 0,
		z2       REAL NOT NULL DEFAULT 0,
		radius   REAL NOT NULL DEFAULT 0,
		below    REAL NOT NULL DEFAULT 0,
		above    REAL NOT NULL DEFAULT 0,
		damage   INTEGER NOT NULL DEFAULT 0,
		every_ms INTEGER NOT NULL DEFAULT 0,
		words    TEXT NOT NULL DEFAULT '',
		note     TEXT NOT NULL DEFAULT '',
		source   TEXT NOT NULL DEFAULT 'gm'
	);"""
COLS = ("name", "world", "x1", "y1", "z1", "x2", "y2", "z2", "radius", "below", "above", "damage", "every_ms", "words", "note")
LOG_SCHEMA = f"CREATE TABLE IF NOT EXISTS {LOG} (seq INTEGER PRIMARY KEY AUTOINCREMENT, hazard INTEGER, row TEXT NOT NULL)"


def has_table(db, t):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone()[0] > 0


def read_rows(paths):
    rows = []
    for p in paths:
        with open(p) as f:
            for r in json.load(f):
                if not (1 <= r["damage"] <= 1000000) or not (0 < r["radius"] <= 100):
                    raise SystemExit(f"{p}: a row the server would refuse: {r}")
                rows.append({c: r[c] for c in COLS})
    return rows


def written(db):
    """The rows an earlier run wrote that are there as written: id -> row."""
    if not has_table(db, LOG):
        return {}
    out = {}
    for hid, row in db.execute(f"SELECT hazard, row FROM {LOG} WHERE hazard IS NOT NULL"):
        out[hid] = json.loads(row)
    return out


def current(db, hid):
    cur = db.execute(f"SELECT {', '.join(COLS)}, source FROM hazards WHERE id = ?", (hid,))
    r = cur.fetchone()
    return None if r is None else dict(zip(list(COLS) + ["source"], r))


def same(a, b):
    return all((abs(a[c] - b[c]) < 1e-3) if isinstance(a[c], float) else a[c] == b[c] for c in COLS)


def apply(db, rows):
    made = not has_table(db, "hazards")
    db.execute(SCHEMA)
    db.execute(LOG_SCHEMA)
    if made:
        db.execute(f"INSERT INTO {LOG} (hazard, row) VALUES (NULL, ?)", (MADE,))
    for r in rows:
        cur = db.execute(f"INSERT INTO hazards ({', '.join(COLS)}, source) VALUES ({', '.join('?' * len(COLS))}, ?)",
                         [r[c] for c in COLS] + [SOURCE])
        db.execute(f"INSERT INTO {LOG} (hazard, row) VALUES (?, ?)", (cur.lastrowid, json.dumps(r)))


def undo(db):
    if not has_table(db, LOG):
        print("nothing to undo")
        return
    kept = deleted = 0
    for hid, r in written(db).items():
        now = current(db, hid)
        if now is None:
            continue
        if not same(now, r) or now["source"] != SOURCE:
            print(f"kept hazard {hid}: changed since ({now['name']!r})")
            kept += 1
            continue
        db.execute("DELETE FROM hazards WHERE id = ?", (hid,))
        deleted += 1
    made = db.execute(f"SELECT count(*) FROM {LOG} WHERE row = ?", (MADE,)).fetchone()[0] > 0
    db.execute(f"DROP TABLE {LOG}")
    dropped = made and db.execute("SELECT count(*) FROM hazards").fetchone()[0] == 0
    if dropped:
        db.execute("DROP TABLE hazards")
    print(f"undone: {deleted} rows deleted, {kept} kept" + (", the table dropped" if dropped else ""))


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--data", action="append")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            undo(db)
            db.execute("COMMIT")
            return 0
        rows = read_rows(a.data or sorted(glob.glob(os.path.join(DATA_DIR, "*.json"))))
        if written(db):
            db.execute("ROLLBACK")
            print("applied already: nothing to do (--undo first to write it again)")
            return 0
        worlds = sorted({r["world"] for r in rows})
        print(f"{len(rows)} hazards to write (worlds {worlds}), e.g.:")
        for r in rows[:3]:
            print(f"  {r['name']}: world {r['world']} ({r['x1']}, {r['z1']})-({r['x2']}, {r['z2']}) at {r['y1']}, radius {r['radius']}, "
                  f"{r['damage']} every {r['every_ms']} ms")
        if not a.apply:
            db.execute("ROLLBACK")
            print("(report only: --apply to write)")
            return 0
        apply(db, rows)
        db.execute("COMMIT")
        print(f"applied: {len(rows)} rows, source {SOURCE}, logged in {LOG}; restart the server (or save a row in the panel's Hazards)")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
