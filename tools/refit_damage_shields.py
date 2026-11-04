#!/usr/bin/env python3
"""The balance fitted again once every workbook damage shield answers each hit (the user, 2026-11-04: "re-fit the other
damage shields").

EQOAEmu's workbook files every damage shield as a defensive proc, which the server read as one at 10% of the hits
(dshield.go's defProcChancePct), and the balance was fitted with them so. The ranger's coats became damage shields that
morning (every hit, unresisted, the bearer's strongest); since this re-fit all 57 are (spelljson.go readProcsJSON):
the druid's thorns, the magician's auras and its pets' marks, the monk's stances, the cleric's Divine Protection, the
shadow knight's Chilling Aura and Shroud of Hate, the wizard's O'Keil's lines and the rest. At every hit the classes
that carry them won more of their fights, and against the server of 2026-11-04 06:51 UTC (main a1fa112, the coats
alone) on the same kits at 13-60 only two moved: the druid by hand (its yellows 5-8 points easier at 20-32, its reds
alone won 44-100% from 41, against 0-51%) and the magician's pet (5-13 points easier at 13-37, its reds alone won up to
97% from 21); the warrior (the control), the shadow knight, monk, shaman, cleric and wizard did not. Their rows of
balanceclasslevel are fitted again by a grid: the druid's melee_taken_pct the smallest of 1.0-1.85 times its own that
makes its red three up a lone loss (at most 38% won) with its yellows no easier than the old server's (one and two up,
the low point averaged over the fights won), 1.12-1.4 at 26 levels: its yellows won 94-100%, their low point 27% on
average (the old 29%), its reds at most 36% (its 45, 51% under the old server, too); the magician's pet_hp_pct its
yellows back to the old server's, 0.67-0.93 of what it was at 28 cells: its yellows' low point about 25% on average as before,
its reds at most 32% (where the shields raised them 18-34 points, at 20, 30, 47 and 50, 0-14%). Rows are added where a class has none at a level whose line the changes would move. Fitted with
cmd/balancesim (-try-classlevel; the roamer, the kits at 0.5, 160 fights a cell) on eqoaclassic's copy of 2026-11-04
05:42 UTC. Levels 1-12 are tools/refit_new_players.py's, fitted with the shields in.

Each row is written only while it holds the values it had (a GM's change is left, and said); a row the fit needed at a
level the class had none is added. Logged in balance_damage_shields_log (the row's key, the whole row before and
after); --undo puts back each row still as this left it (an added row taken out) and drops the log. Both copy the
database first (<db>.bak-<when>-pre-dshields) unless --no-backup. Report only by default. Apply it with the server
that reads every damage shield as one (2026-11-04 or later): under an older one these rows leave the classes short. The
server reads the balance tables at its start and at once at a save of any Balance page in the GM panel.
internal/database/balanceseed.go holds the same rows (a table made new).

    tools/refit_damage_shields.py [--db eqoa.db] [--apply | --undo] [--no-backup]
"""
import argparse
import datetime
import json
import math
import os
import shutil
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
DEFAULT_DB = os.path.join(HERE, "..", "eqoa.db")
CL = "balanceclasslevel"
LOG = "balance_damage_shields_log"
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,   -- balanceclasslevel
    place   TEXT NOT NULL,   -- the row's key, JSON
    before  TEXT,            -- the whole row before, JSON (none: the row was added)
    after   TEXT,            -- the whole row after, JSON
    why     TEXT NOT NULL,
    applied TEXT NOT NULL
)"""
WHY = "the balance fitted again once every damage shield answers each hit (tools/refit_damage_shields.py, 2026-11-04)"

# (class, level): ((melee_taken_pct, pet_hp_pct, dealt_pct) before, or None for a row added; after)
CHANGES = {
    (7, 13): ((90, 100, 100), (100.8, 100, 100)),
    (7, 14): (None, (110.32, 100, 100)),
    (7, 20): ((79, 100, 100), (88.48, 100, 100)),
    (7, 23): ((61, 100, 100), (68.32, 100, 100)),
    (7, 26): (None, (76, 100, 100)),
    (7, 27): (None, (81.76, 100, 100)),
    (7, 29): ((61, 100, 100), (76.25, 100, 100)),
    (7, 30): ((79, 100, 100), (88.48, 100, 100)),
    (7, 32): ((79, 100, 100), (88.48, 100, 100)),
    (7, 35): ((79, 100, 100), (88.48, 100, 100)),
    (7, 41): ((86, 100, 100), (96.32, 100, 100)),
    (7, 45): ((61, 100, 100), (85.4, 100, 100)),
    (7, 46): ((79, 100, 100), (98.75, 100, 100)),
    (7, 47): ((79, 100, 100), (88.48, 100, 100)),
    (7, 48): ((61, 100, 100), (76.25, 100, 100)),
    (7, 49): ((89, 100, 100), (111.25, 100, 100)),
    (7, 50): ((79, 100, 100), (98.75, 100, 100)),
    (7, 51): ((89, 100, 100), (111.25, 100, 100)),
    (7, 52): ((89, 100, 100), (111.25, 100, 100)),
    (7, 54): ((70, 100, 100), (87.5, 100, 100)),
    (7, 55): ((70, 100, 100), (87.5, 100, 100)),
    (7, 56): ((79, 100, 100), (110.6, 100, 100)),
    (7, 57): ((61, 100, 100), (85.4, 100, 100)),
    (7, 58): ((116, 100, 100), (129.92, 100, 100)),
    (7, 59): ((79, 100, 100), (98.75, 100, 100)),
    (7, 60): ((135, 100, 100), (168.75, 100, 100)),
    (10, 13): (None, (100, 184.59, 100)),
    (10, 14): (None, (100, 223.35, 100)),
    (10, 20): ((100, 100, 100), (100, 88, 100)),
    (10, 21): (None, (100, 93.15, 100)),
    (10, 22): (None, (100, 103.075, 100)),
    (10, 23): ((100, 113, 100), (100, 103.96, 100)),
    (10, 27): (None, (100, 95.31, 100)),
    (10, 28): ((100, 113, 100), (100, 102.16, 100)),
    (10, 29): ((100, 100, 100), (100, 91.56, 100)),
    (10, 30): ((100, 113, 100), (100, 89.36, 100)),
    (10, 31): ((100, 100, 100), (100, 92, 100)),
    (10, 32): (None, (100, 117.21, 100)),
    (10, 33): ((100, 159, 100), (100, 131.37, 100)),
    (10, 34): ((100, 56, 100), (100, 51.64, 100)),
    (10, 35): ((100, 71, 100), (100, 59.85, 100)),
    (10, 36): ((100, 63, 100), (100, 52.77, 100)),
    (10, 37): ((100, 80, 100), (100, 62.17, 100)),
    (10, 44): ((100, 44, 100), (100, 29.42, 100)),
    (10, 45): ((100, 90, 100), (100, 83.58, 100)),
    (10, 47): ((100, 63, 100), (100, 49.5, 100)),
    (10, 48): (None, (100, 53.6, 100)),
    (10, 49): ((100, 71, 100), (100, 56.8, 100)),
    (10, 50): (None, (100, 49.98, 100)),
    (10, 51): (None, (100, 56.8, 100)),
    (10, 55): (None, (100, 56.8, 100)),
    (10, 58): (None, (100, 64.4, 100)),
    (10, 59): (None, (100, 76.12, 100)),
    (10, 60): ((100, 90, 100), (100, 76, 100)),
}


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def columns(db, tbl):
    return [r[1] for r in db.execute(f"PRAGMA table_info({tbl})")]


def row_of(db, tbl, key):
    where = " AND ".join(f"{k} = ?" for k in key)
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", list(key.values()))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def same(a, b):
    if a is None or b is None:
        return a is b
    return len(a) == len(b) and all(math.isclose(x, y, rel_tol=1e-9, abs_tol=1e-6) for x, y in zip(a, b))


def cl_values(r):
    return None if r is None else (float(r["melee_taken_pct"]), float(r["pet_hp_pct"]), float(r["dealt_pct"]))


def plan(db):
    """The changes [(table, key, row before or None, row after)] and the rows left, with why."""
    changes, kept = [], []
    if not has_table(db, CL) or "dealt_pct" not in columns(db, CL):
        return changes, [(CL, "no such table or no dealt_pct")]
    cols = columns(db, CL)
    for (c, l), (old, new) in sorted(CHANGES.items()):
        key = {"class": c, "level": l}
        r = row_of(db, CL, key)
        have = cl_values(r)
        if same(have, tuple(map(float, new))):
            continue  # done already
        if old is None:
            if r is not None:
                kept.append((f"{CL} {c} {l}", f"{have}, where the fit added none: a GM's"))
                continue
            after = {k: v for k, v in {"class": c, "level": l, "melee_taken_pct": float(new[0]), "pet_hp_pct": float(new[1]),
                                       "dealt_pct": float(new[2]), "melee_dealt_pct": 100.0, "note": "", "source": "seed"}.items()
                     if k in cols}
            changes.append((CL, key, None, after))
            continue
        if not same(have, tuple(map(float, old))):
            kept.append((f"{CL} {c} {l}", f"{have if have else 'no row'}, not {old}: a GM's"))
            continue
        after = dict(r)
        after.update({"melee_taken_pct": float(new[0]), "pet_hp_pct": float(new[1]), "dealt_pct": float(new[2])})
        changes.append((CL, key, r, after))
    return changes, kept


def describe(change):
    tbl, key, before, after = change
    k = f"class {key['class']} level {key['level']}"
    if before is None:
        return (f"{tbl} {k}: added, melee_taken_pct {after['melee_taken_pct']:g}, pet_hp_pct {after['pet_hp_pct']:g}, "
                f"dealt_pct {after['dealt_pct']:g}")
    diff = [f"{c} {before[c]:g} -> {after[c]:g}" for c in ("melee_taken_pct", "pet_hp_pct", "dealt_pct")
            if not math.isclose(before[c], after[c])]
    return f"{tbl} {k}: " + ", ".join(diff)


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst, n = f"{path}.bak-{when}-pre-dshields", 1
    while os.path.exists(dst):
        n += 1
        dst = f"{path}.bak-{when}-{n}-pre-dshields"
    shutil.copy2(path, dst)
    return dst


def write_row(db, key, row, insert):
    where = " AND ".join(f"{k} = ?" for k in key)
    if insert:
        cols = list(row)
        db.execute(f"INSERT INTO {CL} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [row[c] for c in cols])
    elif row is None:
        db.execute(f"DELETE FROM {CL} WHERE {where}", list(key.values()))
    else:
        cols = [c for c in row if c not in key]
        db.execute(f"UPDATE {CL} SET {', '.join(f'{c} = ?' for c in cols)} WHERE {where}",
                   [row[c] for c in cols] + list(key.values()))


def reorder(db):
    """balanceclasslevel rewritten in (class, level) order, as the tunings left it: an added row would otherwise go to the
    table's end, and a database's dump would differ by the order alone (tools/retune_balance.py's reorder)."""
    cols = ", ".join(columns(db, CL))
    db.execute(f"CREATE TEMP TABLE dshield_order AS SELECT {cols} FROM {CL} ORDER BY class, level")
    db.execute(f"DELETE FROM {CL}")
    db.execute(f"INSERT INTO {CL} ({cols}) SELECT {cols} FROM dshield_order ORDER BY class, level")
    db.execute("DROP TABLE dshield_order")


def apply(db, changes):
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_SCHEMA)
        for tbl, key, before, after in changes:
            db.execute(f"INSERT INTO {LOG} (tbl, place, before, after, why, applied) VALUES (?, ?, ?, ?, ?, ?)",
                       (tbl, json.dumps(key, sort_keys=True), None if before is None else json.dumps(before, sort_keys=True),
                        json.dumps(after, sort_keys=True), WHY, when))
            write_row(db, key, after, insert=before is None)
        reorder(db)
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db):
    if not has_table(db, LOG):
        print("nothing logged: nothing to undo")
        return
    logged = db.execute(f"SELECT seq, tbl, place, before, after FROM {LOG} ORDER BY seq DESC").fetchall()
    back = 0
    db.execute("BEGIN IMMEDIATE")
    try:
        for _seq, tbl, place, before, after in logged:
            if tbl != CL:
                raise SystemExit(f"{LOG} names table {tbl!r}, which this tool never writes")
            key = json.loads(place)
            if row_of(db, tbl, key) != json.loads(after):
                print(f"  {tbl} {place}: changed since: left as it is")
                continue
            write_row(db, key, None if before is None else json.loads(before), insert=False)
            back += 1
        reorder(db)
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"put back {back} of {len(logged)} rows; the server takes them at its next start or a Balance page's next save")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the rows (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply changed")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        if not a.no_backup and has_table(db, LOG):
            print(f"copy: {backup(a.db)}")
        undo(db)
        return
    changes, kept = plan(db)
    for c in changes:
        print(describe(c))
    for what, why in kept:
        print(f"left as it is: {what} ({why})")
    print(f"{len(changes)} rows to change, {len(kept)} left")
    if not a.apply or not changes:
        if not a.apply:
            print("report only: --apply to write")
        return
    if not a.no_backup:
        print(f"copy: {backup(a.db)}")
    apply(db, changes)
    print(f"changed {len(changes)} rows: the server takes them at its next start, or at once at the next save of a "
          "Balance page in the GM panel")


if __name__ == "__main__":
    main()
