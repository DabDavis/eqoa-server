#!/usr/bin/env python3
"""Cleave hits up to 3 in a frontal cone (the owner, 2026-11-11: "lets make cleave hit up to 3 in a frontal cone"), on
the server's code of the same branch (entity/area.go: spellarea centre 3, a cone in front of its caster aimed at its
target, and column cone, its half-angle in degrees; an older server reads centre 3 as "as its line reads": Cleave
stays a strike of one).

  spellarea 700534 (Cleave, the paladin's level-46 ability): centre 3 (a cone), radius 6 (melee reach past the caster,
    a large creature +3, a huge +5), maxtargets 3 (its target and the two nearest the caster in the cone), cone 60
    (60 degrees either side of the line to its target). Each takes the whole strike (600 ; 30% STR), as its target.

The other cleaves are left as they are (Warchief's Cleave is round its caster already; Bloodcleave, Doomcleave,
Paincleave, Sorrowcleave and Deathcleave are drains of one target). A GM can give any of them a cone in the panel's
"Spell areas" (centre "A cone in front of its caster").

Refused, nothing written, unless spellpattern 700534 is "Cleave" and it has no spellarea row (one exactly this tool's:
applied already). A spellarea made before column cone gets it here (as the server's start would: REAL NOT NULL
DEFAULT 0); the undo leaves the column (the server of this code reads it at every cast, and 0 on every row is as
each was): the rows come back exactly. Report only by default; --apply in one
transaction, a gm_audit line, log cleave_cone_1111_log; --undo exact (a row changed since is left and said). spellarea
is read at every cast: no restart for the row (the code with the cone needs one).

    tools/cleave_cone_20261111.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "cleave_cone_1111_log"
DETAIL = "Cleave hits up to 3 in a frontal cone (the owner, 2026-11-11): tools/cleave_cone_20261111.py"
WHO, VIA = "dab", "tool"
SPELL, NAME = 700534, "Cleave"
ROW = {"spellid": SPELL, "centre": 3, "radius": 6.0, "maxtargets": 3, "cone": 60.0,
       "note": "the owner, 2026-11-11: \"lets make cleave hit up to 3 in a frontal cone\"; a cone of 60 degrees either side "
               "of its target, radius 6 (melee reach), 3 in all (tools/cleave_cone_20261111.py)"}
CONE_DDL = "ALTER TABLE spellarea ADD COLUMN cone REAL NOT NULL DEFAULT 0"
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    what    TEXT NOT NULL,      -- 'row' or 'column'
    after   TEXT NOT NULL,      -- the row as written (JSON); for 'column', the column added
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def has_column(db, t, c):
    return any(r[1] == c for r in db.execute(f"PRAGMA table_info({t})"))


def row_of(db, spellid):
    cur = db.execute("SELECT * FROM spellarea WHERE spellid = ?", (spellid,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, 'spellarea', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, go_json({"spellid": SPELL}),
                      None if before is None else go_json(before), None if after is None else go_json(after), detail))
    return cur.lastrowid


def same(row):
    return row is not None and all(row.get(k) == v for k, v in ROW.items())


def plan(db):
    """(lines, refusals, applied)."""
    lines, refusals = [], []
    if not table(db, "spellarea") or not table(db, "spellpattern"):
        return lines, ["no spellarea or spellpattern table here"], False
    sp = db.execute("SELECT spellname, scope FROM spellpattern WHERE spellid = ?", (SPELL,)).fetchone()
    if sp is None or sp[0] != NAME:
        return lines, [f"spellpattern {SPELL} is {sp!r}, not {NAME!r}"], False
    if not has_column(db, "spellarea", "cone"):
        lines.append("spellarea has no column cone: --apply adds it (REAL NOT NULL DEFAULT 0, as the server's start does)")
    elif same(row_of(db, SPELL)):
        return lines, [], True
    if has_column(db, "spellarea", "cone") and row_of(db, SPELL) is not None:
        refusals.append(f"spellarea {SPELL} has a row already: {row_of(db, SPELL)}")
    lines.append(f"spellarea {SPELL} ({NAME}, scope {sp[1]}): a cone in front of its caster aimed at its target, radius "
                 f"{ROW['radius']:g}, {ROW['maxtargets']} in all, half-angle {ROW['cone']:g} degrees")
    for sid, name, scope in db.execute("SELECT spellid, spellname, scope FROM spellpattern WHERE spellname LIKE '%cleave%' "
                                       "AND spellid != ? ORDER BY spellid", (SPELL,)):
        lines.append(f"  left as it is: {sid} {name} (scope {scope})")
    return lines, refusals, False


def apply(db):
    db.execute(LOG_SCHEMA)
    if not has_column(db, "spellarea", "cone"):
        db.execute(CONE_DDL)
        db.execute(f"INSERT INTO {LOG} (what, after) VALUES ('column', 'cone')")
    cols = list(ROW)
    db.execute(f"INSERT INTO spellarea ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [ROW[c] for c in cols])
    row = row_of(db, SPELL)
    a = audit(db, "insert", None, row, DETAIL)
    db.execute(f"INSERT INTO {LOG} (what, after, audit) VALUES ('row', ?, ?)", (json.dumps(row, sort_keys=True), a))


def undo(db):
    """(back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, what, after in db.execute(f"SELECT seq, what, after FROM {LOG} ORDER BY seq DESC").fetchall():
        if what == "row":
            now = row_of(db, SPELL)
            if now is None or json.dumps(now, sort_keys=True) != after:
                left += 1
                print(f"  left: spellarea {SPELL}, changed since it was written: {now}")
                continue
            db.execute("DELETE FROM spellarea WHERE spellid = ?", (SPELL,))
            audit(db, "delete", now, None, "undo: " + DETAIL)
        # what == "column": the column stays (the server with the cone reads it at every cast; 0 on every row is
        # as each was)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if left == 0:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        if a.undo:
            db.execute("BEGIN IMMEDIATE")
            try:
                back, left = undo(db)
                db.execute("COMMIT")
            except BaseException:
                db.execute("ROLLBACK")
                raise
            print(f"put back {back}, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        if table(db, LOG):
            print(f"applied already ({LOG} is there): --undo first")
            return 1
        lines, refusals, applied = plan(db)
        for line in lines:
            print(line)
        if applied:
            print("applied already (the row is this tool's): nothing done")
            return 0
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not a.apply:
            print("report only: 1 row would be written (--apply writes it)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            apply(db)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: 1 row, in gm_audit and {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
