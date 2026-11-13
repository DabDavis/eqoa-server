#!/usr/bin/env python3
"""Three kobold camps in Odus that wildlife never walks into or through, as the GM Jibwiz asked in petitions 1963-1965
(2026-11-13): "This kobold camp beneath me needs to not have wildlife in this area pathing into it. It can get close,
but never through the tented camp" (1963), "... wildlife in this zone roam through the camp. They can get close, but
never go through it" (1964), "This kobold slayer camp needs to not have wildlife path into it" (1965).

Table noroam (entity/noroam.go): a wander leg that ends in one of its circles or passes through it is not taken, unless
the creature spawned in it (the camp's kobolds) or stands in it. Each camp's circle is its tents' as the disc places
them (ODUS.ESF's zone actors of the tent sprite 0x5C08C48E and the racks 0xDBBE83F2 near where Jibwiz stood: the
smallest circle round every point of their meshes) and 3 more:

  petition  Jibwiz stood        tents' circle (middle, radius)   kept out
  1963      (5467.6, 10515.7)   (5454.3, 10520.8) 36.2           39
  1964      (5916.6, 10243.9)   (5915.7, 10259.0) 18.9           22
  1965      (5784.4, 11134.8)   (5775.4, 11137.9) 31.3           34

Table noroam is made here when there is none (as the server makes it at its start). Rows of source camps1963; a run
when they are in already writes nothing. Report only by default; --apply in one transaction, a gm_audit line a row, log
noroam_camps_1963_log; --undo exact. The server reads table noroam at its start and at a GM panel edit of a noroam
row (No-roam areas). Needs the server with entity/noroam.go (an older one never reads the table).

    python3 -E -s tools/noroam_camps_1963_20261113.py --db eqoa.db [--apply | --undo]
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "noroam_camps_1963_log"
DETAIL = "Three Odus kobold camps no wanderer walks into or through (the GM Jibwiz's petitions 1963-1965): " \
         "tools/noroam_camps_1963_20261113.py"
SOURCE = "camps1963"
SCHEMA = """
	CREATE TABLE IF NOT EXISTS noroam (
		id     INTEGER PRIMARY KEY,
		name   TEXT NOT NULL DEFAULT '',
		world  INTEGER NOT NULL DEFAULT 0,
		x      REAL NOT NULL DEFAULT 0,
		z      REAL NOT NULL DEFAULT 0,
		radius REAL NOT NULL DEFAULT 0,
		note   TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm'
	);"""
CAMPS = [
    ("Kobold camp (petition 1963)", 5454.3, 10520.8, 39.0, 1963, 36.2),
    ("Kobold camp (petition 1964)", 5915.7, 10259.0, 22.0, 1964, 18.9),
    ("Kobold slayer camp (petition 1965)", 5775.4, 11137.9, 34.0, 1965, 31.3),
]


def changes(db):
    out, lines = [], []
    have = tr.rows(db, "SELECT * FROM noroam WHERE source = ? ORDER BY id", (SOURCE,)) if tr.table(db, "noroam") else []
    if len(have) == len(CAMPS) and all(h["name"] == c[0] for h, c in zip(have, CAMPS)):
        return [], lines, [], True
    if have:
        return [], lines, [f"{len(have)} noroam rows of source {SOURCE} are there, not as this tool writes them"], False
    nxt = 1
    if tr.table(db, "noroam"):
        nxt = (db.execute("SELECT MAX(id) FROM noroam").fetchone()[0] or 0) + 1
    for i, (name, x, z, r, pet, tents) in enumerate(CAMPS):
        row = {"id": nxt + i, "name": name, "world": 2, "x": x, "z": z, "radius": r,
               "note": f"petition {pet}: the camp's tents within {tents} of here (ODUS.ESF's tents and racks), and 3 more",
               "source": SOURCE}
        out.append(("noroam", {"id": row["id"]}, row))
        lines.append(f"noroam {row['id']}: {name}, world 2 ({x}, {z}) radius {r}")
    if not tr.table(db, "noroam"):
        lines.append("noroam: no table yet; made at --apply (as the server makes it)")
    return out, lines, [], False


def prepare(db):
    if tr.table(db, "noroam"):
        return []
    db.execute(SCHEMA)
    return ["noroam made"]


SPEC = {"log": LOG, "detail": DETAIL, "needs": (), "changes": changes, "prepare": prepare}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
