#!/usr/bin/env python3
"""Levels 1-12 balanced apart, for new players (the user, 2026-11-04: "lets make levels 1-10 mobs have less hp and
players melee damage lower also. make healers/casters do more melee damage also during those levels", "Players hit for
less", "Melee only", then "we can have a seperate 1-12 balancing. Lets make it easier for new players, 1-5 mobs should
really be easy and 6-9 little tougher. 10-12 stronger, the mob crits at 10 do make it more difficult").

balancelevel, by the creature's level:
- hp_pct at 1-10 three quarters of what it was;
- player_melee_pct (a column of 2026-11-04: the share of a player's own auto-attack melee that lands on a creature of
  the level, rounded by chance so it holds for a low level's small hits) 75 at 1-10, so a melee class's fights there are
  as long as they were (the simulator: the melee classes' median even fight within a second or two at every level);
- damage_pct (the creature's melee) fitted level by level to the user's three steps: the yellows against it (players one
  and two levels under it at +1 and +2, the median over every class's own play) end at about 60-80% of a player's hit
  points against creatures of 1-5, 39-50% against 6-9 and 27-33% against 10-12 (the roamer's Critical Hit from 10, level
  x 7.4 every 30 s through armour, as it is), even fights keeping 93-96%, 84-90% and 62-67% (the median class); each
  level's figure is the old one's 0.27-1.0, never past it. A creature's hit is scaled after its floor and rounded at each
  step, so at 2-5 the share moves the fights only where a rounding step flips: each level's is taken inside a step.
balanceclasslevel, by the player's class and level:
- melee_dealt_pct (a column of 2026-11-04: the share of a player's own melee that lands, times dealt_pct's) for the
  healers at 1-12 and the casters at 1-10: their melee raised until their even fights by hand are about a melee class's
  (1.15 times the melee classes' median, smoothed over the levels either side, at 1-10; the healers' 1.3 and 1.45 times
  at 11 and 12, toward 13, where a healer's even fight is about 1.6 times a melee class's), in a new character's starting
  gear at 1-5 and the middling gear of its last ten levels at 6-12; never below 100, a caster's at most 250 (a necromancer
  of 8's fight by hand barely follows its melee: 28 s at 100, 22 s at 458).

At every level 1-10 every class wins 95% of its even fights and 97% of its blue and green ones, by hand and in its own
play (entity/balancesim_real_test.go TestRealLowLevelsEvenAndBelow).

Fitted with cmd/balancesim (-try-level, -try-playermelee, -try-classlevel; the roamer, -start 5) on eqoaclassic's copy
of 2026-11-04 05:42 UTC, every hp damage shield answering each hit (tools/refit_damage_shields.py's server). The
targets at 1-12 are these, not the 2026-10-30 ones (the user: "thats okay if its unbalanced 1-12").

Each row is written only while it holds the values it had (a GM's change is left, and said). The two columns are added
when missing, as the server adds them at its start (at 100 they change nothing; --undo leaves them). Logged in
balance_new_players_log (the row's key, the whole row before and after); --undo puts back each row still as this left
it and drops the log. Both copy the database first (<db>.bak-<when>-pre-newplayers) unless --no-backup. Report only by
default. Needs the server of 2026-11-04 or later to mean anything (an older one reads neither column); it reads the
balance tables at its start and at once at a save of any Balance page in the GM panel. internal/database/balanceseed.go
holds the same rows (a table made new).

    tools/refit_new_players.py [--db eqoa.db] [--apply | --undo] [--no-backup]
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
LV, CL = "balancelevel", "balanceclasslevel"
PM, MD = "player_melee_pct", "melee_dealt_pct"
COL_DECL = "REAL NOT NULL DEFAULT 100"
LOG = "balance_new_players_log"
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,   -- balancelevel or balanceclasslevel
    place   TEXT NOT NULL,   -- the row's key, JSON
    before  TEXT,            -- the whole row before, JSON
    after   TEXT,            -- the whole row after, JSON
    why     TEXT NOT NULL,
    applied TEXT NOT NULL
)"""
WHY = "levels 1-12 balanced apart for new players (tools/refit_new_players.py, 2026-11-04)"
NOTE = WHY  # balancelevel's note on the rows it writes (the seed's balanceNewPlayersNote)

# the creature's level: ((hp_pct, damage_pct, player_melee_pct) before, after)
LEVELS = {
    1: ((300, 188, 100), (225, 79, 75)),
    2: ((278.75, 317, 100), (209.0625, 133, 75)),
    3: ((257.5, 225, 100), (193.125, 61, 75)),
    4: ((236.25, 294, 100), (177.1875, 112, 75)),
    5: ((215, 243, 100), (161.25, 102, 75)),
    6: ((655 / 3, 285, 100), (163.75, 168, 75)),
    7: ((665 / 3, 245, 100), (166.25, 162, 75)),
    8: ((225, 276, 100), (168.75, 248, 75)),
    9: ((222.5, 230, 100), (166.875, 230, 75)),
    10: ((220, 236, 100), (165, 229, 75)),
    11: ((222, 221, 100), (222, 208, 100)),
    12: ((224, 247, 100), (224, 232, 100)),
}
# (class, level): ((melee_taken_pct, pet_hp_pct, dealt_pct, melee_dealt_pct) before, after)
CLASSES = {
    (7, 1): ((16, 100, 100, 100), (16, 100, 100, 199)),
    (7, 2): ((32, 100, 130, 100), (32, 100, 130, 241)),
    (7, 3): ((45, 100, 100, 100), (45, 100, 100, 164)),
    (7, 4): ((55, 100, 100, 100), (55, 100, 100, 239)),
    (7, 5): ((59, 100, 100, 100), (59, 100, 100, 289)),
    (7, 6): ((63, 100, 100, 100), (63, 100, 100, 141)),
    (7, 7): ((78, 100, 100, 100), (78, 100, 100, 203)),
    (7, 8): ((57, 100, 100, 100), (57, 100, 100, 140)),
    (7, 9): ((34, 100, 100, 100), (34, 100, 100, 151)),
    (7, 10): ((39, 100, 100, 100), (39, 100, 100, 151)),
    (7, 11): ((48, 100, 100, 100), (48, 100, 100, 111)),
    (8, 1): ((17, 284, 100, 100), (17, 284, 100, 242)),
    (8, 2): ((31, 284, 130, 100), (31, 284, 130, 268)),
    (8, 3): ((63, 284, 100, 100), (63, 284, 100, 440)),
    (8, 4): ((78, 268, 100, 100), (78, 268, 100, 371)),
    (8, 5): ((95, 268, 100, 100), (95, 268, 100, 495)),
    (8, 6): ((117, 268, 100, 100), (117, 268, 100, 234)),
    (8, 7): ((143, 268, 100, 100), (143, 268, 100, 309)),
    (8, 8): ((143, 268, 100, 100), (143, 268, 100, 245)),
    (8, 9): ((95, 268, 100, 100), (95, 268, 100, 259)),
    (8, 10): ((95, 268, 100, 100), (95, 268, 100, 263)),
    (8, 11): ((99, 268, 100, 100), (99, 268, 100, 175)),
    (8, 12): ((114, 268, 100, 100), (114, 268, 100, 123)),
    (9, 1): ((17, 100, 100, 100), (17, 100, 100, 236)),
    (9, 2): ((27, 100, 130, 100), (27, 100, 130, 253)),
    (9, 3): ((38, 100, 100, 100), (38, 100, 100, 440)),
    (9, 4): ((138, 100, 100, 100), (138, 100, 100, 440)),
    (9, 5): ((143, 100, 100, 100), (143, 100, 100, 449)),
    (9, 6): ((117, 100, 100, 100), (117, 100, 100, 214)),
    (9, 7): ((176, 100, 100, 100), (176, 100, 100, 298)),
    (9, 8): ((117, 100, 100, 100), (117, 100, 100, 316)),
    (9, 9): ((78, 100, 100, 100), (78, 100, 100, 315)),
    (9, 10): ((63, 100, 100, 100), (63, 100, 100, 208)),
    (9, 11): ((79, 100, 100, 100), (79, 100, 100, 161)),
    (9, 12): ((129, 100, 100, 100), (129, 100, 100, 139)),
    (10, 6): ((26, 159, 100, 100), (26, 159, 100, 118)),
    (11, 7): ((32, 357, 100, 100), (32, 357, 100, 234)),
    (11, 8): ((32, 126, 100, 100), (32, 126, 100, 250)),
    (11, 9): ((40, 159, 100, 100), (40, 159, 100, 158)),
    (11, 10): ((17, 201, 100, 100), (17, 201, 100, 155)),
    (12, 6): ((26, 566, 100, 100), (26, 566, 100, 118)),
    (12, 8): ((24, 71, 118, 100), (24, 71, 118, 190)),
    (12, 9): ((26, 95, 100, 100), (26, 95, 100, 196)),
    (12, 10): ((16, 113, 115, 100), (16, 113, 115, 119)),
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


def lv_values(r):
    return None if r is None else (float(r["hp_pct"]), float(r["damage_pct"]), float(r.get(PM, 100)))


def cl_values(r):
    return None if r is None else (float(r["melee_taken_pct"]), float(r["pet_hp_pct"]), float(r["dealt_pct"]),
                                   float(r.get(MD, 100)))


def plan(db):
    """The changes [(table, key, row before, row after)] and the rows left, with why."""
    changes, kept = [], []
    for tbl in (LV, CL):
        if not has_table(db, tbl):
            return [], [(tbl, "no such table")]
    if "dealt_pct" not in columns(db, CL):
        return [], [(CL, "no dealt_pct: an older balance (tools/retune_balance.py first)")]
    for l, (old, new) in sorted(LEVELS.items()):
        r = row_of(db, LV, {"level": l})
        have = lv_values(r)
        if same(have, tuple(map(float, new))):
            continue
        if not same(have, tuple(map(float, old))):
            kept.append((f"{LV} {l}", f"{have if have else 'no row'}, not {old}: a GM's"))
            continue
        after = dict(r)
        after.update({"hp_pct": float(new[0]), "damage_pct": float(new[1]), PM: float(new[2])})
        if "note" in after:
            after["note"] = NOTE
        changes.append((LV, {"level": l}, r, after))
    for (c, l), (old, new) in sorted(CLASSES.items()):
        r = row_of(db, CL, {"class": c, "level": l})
        have = cl_values(r)
        if same(have, tuple(map(float, new))):
            continue
        if not same(have, tuple(map(float, old))):
            kept.append((f"{CL} {c} {l}", f"{have if have else 'no row'}, not {old}: a GM's"))
            continue
        after = dict(r)
        after.update({"melee_taken_pct": float(new[0]), "pet_hp_pct": float(new[1]), "dealt_pct": float(new[2]),
                      MD: float(new[3])})
        changes.append((CL, {"class": c, "level": l}, r, after))
    return changes, kept


def describe(change):
    tbl, key, before, after = change
    k = " ".join(str(v) for v in key.values())
    cols = ("hp_pct", "damage_pct", PM) if tbl == LV else ("melee_taken_pct", "pet_hp_pct", "dealt_pct", MD)
    diff = [f"{c} {float(before.get(c, 100)):g} -> {float(after[c]):g}" for c in cols
            if not math.isclose(float(before.get(c, 100)), float(after[c]))]
    return f"{tbl} {k}: " + ", ".join(diff)


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst, n = f"{path}.bak-{when}-pre-newplayers", 1
    while os.path.exists(dst):
        n += 1
        dst = f"{path}.bak-{when}-{n}-pre-newplayers"
    shutil.copy2(path, dst)
    return dst


def write_row(db, tbl, key, row):
    where = " AND ".join(f"{k} = ?" for k in key)
    cols = [c for c in row if c not in key]
    db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in cols)} WHERE {where}",
               [row[c] for c in cols] + list(key.values()))


def apply(db, changes):
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        for tbl, col in ((LV, PM), (CL, MD)):  # as the server adds them (database/balance.go balanceColumns)
            if col not in columns(db, tbl):
                db.execute(f"ALTER TABLE {tbl} ADD COLUMN {col} {COL_DECL}")
        db.execute(LOG_SCHEMA)
        for tbl, key, before, after in changes:
            before = dict(before)
            before.setdefault(PM if tbl == LV else MD, 100.0)
            db.execute(f"INSERT INTO {LOG} (tbl, place, before, after, why, applied) VALUES (?, ?, ?, ?, ?, ?)",
                       (tbl, json.dumps(key, sort_keys=True), json.dumps(before, sort_keys=True),
                        json.dumps(after, sort_keys=True), WHY, when))
            write_row(db, tbl, key, after)
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
            if tbl not in (LV, CL):
                raise SystemExit(f"{LOG} names table {tbl!r}, which this tool never writes")
            key = json.loads(place)
            if row_of(db, tbl, key) != json.loads(after):
                print(f"  {tbl} {place}: changed since: left as it is")
                continue
            write_row(db, tbl, key, json.loads(before))
            back += 1
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
