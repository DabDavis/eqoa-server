#!/usr/bin/env python3
"""The spells that carried two classes past the balance's targets (the user,
2026-10-29: "Vampiric Bond Effect gets a recast" and the alchemist's potions
"toned to the targets"), found with the fight simulator (entity/balancesim.go,
cmd/balancesim; claude-notes/balance-20261029.md).

The source for the records, named in every entry: Allakhazam's EQOA spell
database (its import of the game's records of 2004-04-28, "AK <id>"), as
tools/sweep_spell_pages.py and tools/fix_tested_spells.py read it.

  1. Vampiric Bond Effect (shadow knight 46): no cast time, no recast, no
     power cost, a 96 drain (plus 5% of INT). Its 2004 record is the same
     (AK 4030): the effect of something else, a proc's, never a spell a player
     casts at will; here it is in the book, and a cast every 0.4 s (the busy
     window) made a level 50-60 shadow knight win every fight to five levels
     up with all its hit points (121 casts in one fight). It gets a recast of
     30 seconds (OURS, the simulator's: a shadow knight of 50-60 against a red
     three and five up kept 99-100% of its hit points with none, 70-89% and
     38-67% with 30, and no less with 60, the other taps of its book holding
     it there).
  2. The alchemist's infusions, health turned into power (Tainted 5,
     Sickening 15, Poisoned 25, Venomous 35, Virulent 45, Toxic 54): every
     one's record (AK 3750-3756) is Self Only, "Decrease Hitpoints by N,
     Increase Power per tic by M for 18 seconds", cast 2 s, no recast, no
     cost. Here three were wrong (EQOA-Data's words, which the importer
     kept): Tainted and Poisoned were cast at a target, a free 90 and 400
     damage every 2.3 s; Virulent read "Buff HP by 1000", a free heal of
     1,000 every 2.3 s that kept a level 45-60 alchemist alive against any
     red (four casts in one fight). And the server read none of the six
     prices: "SELF Decrease HP by N" lands on its caster as a heal of a
     negative amount, which heals nothing, so each was free power. Each now
     says it as a pact does ("Self, Decrease Hitpoints by N to Self, Increase
     Power per tic by M, 18sec"): the price is paid at the landing, less the
     caster's arcane resist's share as a pact's is (entity/lifetap.go), and
     the power ticks for 18 seconds. The two target ones are made Self Only
     with the self record's range and frame. The simulator's level 45-60
     alchemist (its mixes and buffs, hand to hand, under the seed balance)
     won a red three up every time with 61-70% of its hit points left and
     one five up 92-100% of the time; with these records 0-58% and never.
     Its mix potions, their records EQOA-Data's, are left as they are.

A value is changed only while it is what this expects (a GM's change is left,
and said). Every change is logged in balance_spells_fix_log (the row's key,
the whole row before and after, why); --undo puts back each row still as this
left it (one changed since is left, and said) and drops the log, a second
--apply only adds what is still to do. Both copy the database first
(<db>.bak-<when>-pre-balancefix) unless --no-backup. Report only by default.
The server reads the records and lines at each cast: no restart; a client
reads the records with its spell book, at its next login.

    tools/fix_balance_spells.py [--db eqoa.db] [--apply | --undo] [--no-backup]
"""
import argparse
import datetime
import json
import os
import shutil
import sqlite3
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
CLASSES = "WAR RAN PAL SK MNK BRD RGE DRD SHA CL MAG NEC ENC WIZ ALC".split()
SK, ALC = 3, 14

LOG = "balance_spells_fix_log"
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,   -- spellpattern | spelleffect
    rowkey  TEXT NOT NULL,   -- the row's key, JSON
    before  TEXT NOT NULL,   -- the whole row before, JSON
    after   TEXT NOT NULL,   -- the whole row after, JSON
    why     TEXT NOT NULL,
    applied TEXT NOT NULL
)"""
KEYS = {"spellpattern": ("spellid",), "spelleffect": ("spellid",)}

SELF_RANGE, SELF_FRAME = 16528, -1809115846
TARGET_RANGE, TARGET_FRAME = 16880, 486870879

VBE_RECAST = 30

# (class, name, level): the record's recast now and wanted, and its page
RECASTS = {
    (SK, "vampiric bond effect", 46): (0, VBE_RECAST, 4030),
}

# (class, name, level): (scope now, the line now, its page, price, power a tick)
INFUSIONS = {
    (ALC, "tainted infusion", 5): (1, "TARGET Debuff HP by 90 and Power per tic by 75 for 18 seconds", 3750, 90, 75),
    (ALC, "sickening infusion", 15): (0, "SELF Decrease HP by 150 and Increase Power per tic by 150 for 18 seconds", 3752, 150, 150),
    (ALC, "poisoned infusion", 25): (1, "TARGET Debuff HP by 400 and Buff Power per tic by 300 for 18 seconds", 3753, 400, 300),
    (ALC, "venomous infusion", 35): (0, "SELF Decrease HP by 800 and Increase Power per tic by 450 for 18 seconds", 3754, 800, 450),
    (ALC, "virulent infusion", 45): (0, "SELF Buff HP by 1000 and Power per tic by 600 for 18 seconds", 3755, 1000, 600),
    (ALC, "toxic infusion", 54): (0, "SELF Decrease HP by 1600 and Increase Power per tic by 800 for 18 seconds", 3756, 1600, 800),
}


def infusion_line(price, tick):
    """An infusion as a pact words its price (paid at the landing, entity/lifetap.go)."""
    return f"Self, Decrease Hitpoints by {price} to Self, Increase Power per tic by {tick}, 18sec"


def key_text(key):
    return json.dumps(key, sort_keys=True, separators=(",", ":"))


def row_of(db, tbl, key):
    where = " AND ".join(f"{c} = ?" for c in key)
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", list(key.values()))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def spell_of(db, cls, name, lvl):
    """The one spell of a class of that name (any case) and level, else the
    reason there is none."""
    ids = [i for (i,) in db.execute(
        "SELECT p.spellid FROM classspells c JOIN spellpattern p ON p.spellid = c.spellid "
        "WHERE c.class = ? AND lower(trim(p.spellname)) = ? AND p.abilitylvl = ?", (cls, name, lvl))]
    if len(ids) != 1:
        return None, f"{len(ids)} {CLASSES[cls]} spells of that name and level"
    return ids[0], None


def plan(db):
    """The changes [(table, key, row before, row after, why)] and the spells
    left as they are, with why [(what, why)]."""
    changes, kept = [], []

    for (cls, name, lvl), (now, want, page) in RECASTS.items():
        what = f"{CLASSES[cls]} L{lvl} {name}"
        sid, why = spell_of(db, cls, name, lvl)
        if sid is None:
            kept.append((what, why))
            continue
        r = row_of(db, "spellpattern", {"spellid": sid})
        if r["recast"] == want:
            continue
        if r["recast"] != now:
            kept.append((what, f"record {sid} has recast {r['recast']}, not {now}: a GM's"))
            continue
        a = dict(r, recast=want)
        changes.append(("spellpattern", {"spellid": sid}, r, a,
                        f"recast {want} s (ours, the simulator's; its record AK {page} has none: an effect, not a spell)"))

    for (cls, name, lvl), (scope, line, page, price, tick) in INFUSIONS.items():
        what = f"{CLASSES[cls]} L{lvl} {name}"
        sid, why = spell_of(db, cls, name, lvl)
        if sid is None:
            kept.append((what, why))
            continue
        want = infusion_line(price, tick)
        e = row_of(db, "spelleffect", {"spellid": sid})
        p = row_of(db, "spellpattern", {"spellid": sid})
        if e is None:
            kept.append((what, f"record {sid} has no line"))
            continue
        if e["line"] == want and p["scope"] == 0:
            continue
        if e["line"] != line or p["scope"] != scope:
            kept.append((what, f"record {sid}: scope {p['scope']}, line {e['line']!r}: not as expected, a GM's"))
            continue
        if scope != 0 and (p["range"], p["iconColor"]) != (TARGET_RANGE, TARGET_FRAME):
            kept.append((what, f"record {sid}'s range and frame are not the importer's target record's"))
            continue
        a = dict(e, line=want, source=f"{e['source']}; line AK {page} as a pact's price (balancefix)")
        changes.append(("spelleffect", {"spellid": sid}, e, a,
                        f"AK {page}: Self Only, Decrease Hitpoints by {price}, Increase Power per tic by {tick} for 18 s; "
                        "worded as a pact's price, which the server pays"))
        pa = dict(p)
        if p["spelldesc"] == line:
            pa["spelldesc"] = want
        if scope != 0:
            pa.update(scope=0, range=SELF_RANGE, iconColor=SELF_FRAME)
        if pa != p:
            changes.append(("spellpattern", {"spellid": sid}, p, pa, f"AK {page}: Self Only"))
    return changes, kept


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-balancefix"
    shutil.copy2(path, dst)
    return dst


def write_row(db, tbl, key, row):
    where = " AND ".join(f"{c} = ?" for c in key)
    cols = [c for c in row if c not in key]
    db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in cols)} WHERE {where}",
               [row[c] for c in cols] + list(key.values()))


def apply(db, changes):
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_SCHEMA)
        for tbl, key, before, after, why in changes:
            db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, why, applied) VALUES (?, ?, ?, ?, ?, ?)",
                       (tbl, key_text(key), json.dumps(before, sort_keys=True), json.dumps(after, sort_keys=True), why, when))
            write_row(db, tbl, key, after)
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db):
    if not has_table(db, LOG):
        print("nothing logged: nothing to undo")
        return
    logged = db.execute(f"SELECT seq, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall()
    back = 0
    db.execute("BEGIN IMMEDIATE")
    try:
        for seq, tbl, rowkey, before, after in logged:
            if tbl not in KEYS:
                raise SystemExit(f"{LOG} names table {tbl!r}, which this tool never writes")
            key = json.loads(rowkey)
            now = row_of(db, tbl, key)
            if now != json.loads(after):
                print(f"  {tbl} {rowkey}: changed since ({'gone' if now is None else 'not as this left it'}): left as it is")
                continue
            write_row(db, tbl, key, json.loads(before))
            back += 1
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"put back {back} of {len(logged)} rows")


def describe(change):
    tbl, key, before, after, why = change
    diff = ", ".join(f"{c} {before[c]!r} -> {after[c]!r}" for c in after if after[c] != before[c])
    return f"{tbl} {key_text(key)}: {diff}\n          ({why})"


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the changes (logged)")
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
    print(f"changed {len(changes)} rows: the next cast of each reads it")


if __name__ == "__main__":
    main()
