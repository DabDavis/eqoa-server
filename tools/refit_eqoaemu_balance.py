#!/usr/bin/env python3
"""The balance re-fitted after EQOAEmu's spell workbook went in (2026-11-01; the user, asked whether the workbook's
combat numbers replace ours: "Workbook wins, re-fit"): its spells, effects, masteries and pets in the place of ours
(tools/import_eqoaemu_spells.py, import_eqoaemu_cms.py, import_eqoaemu_pets.py), and the balance tables fitted again
to the user's targets of 2026-10-30, which the four tunings before had met on our own spells
(claude-notes/balance-20261029.md).

What the workbook moved, measured with the fight simulator on eqoaclassic's database of 07:15 UTC 2026-11-01 with
the whole import in it (cmd/balancesim against SOE's roamer, the player's low point averaged over the fights won one
and two up; the tables as the four tunings left them):

  the melee classes and the tanks got stronger (the buff lines stack by the workbook's stack types, its heals and
  procs land): a warrior, a paladin, a shadow knight, a bard and a rogue kept 40-65% of their hit points through a
  yellow where the targets want a quarter, and a paladin of 37-60 and a bard of 13-59 won nearly every red three up
  alone; a monk got weaker (20-29 and 40-60 lose a yellow two up more often than not);
  the priests out-healed their reds: a druid, a shaman and a cleric of 10-50 won 70-100% of their fights three up
  alone, in two to eight minutes;
  the pets went over the top: a workbook pet fights as a player of its kind with the workbook's weapon and armour
  (petstats.go), and balancelevel's pet columns (pet_hp_pct 100-540, pet_damage_pct 225-560) had been fitted for the
  server's own pets, which hit with a creature's roll: a magician's and a necromancer's pet won every fight to three
  up alone keeping 60-99% of its hit points from level 8, in 6-30 s; an enchanter's lost every fight at 6-23;
  a rooting wizard of 45-60 lost every fight: the workbook's roots end at a hit (DisspellOnContact), and the
  simulator's root play recast Arcane Shackles at every nuke (balancesim.go now roots with one that holds).

It writes, each change only while the row holds what the four tunings left it (the post-import database has them so;
a row a GM changed is left, and said):

  balancelevel       pet_hp_pct and pet_damage_pct 100 at every row (PET_LEVEL): a workbook pet's hits land as a
                     player's do, its hit points the workbook's times its owner's class and level share (below)
  balanceclasslevel  every class's rows (REFIT), fitted by the simulator at every level 1-60 as the first tunings
                     were (tools/retune_balance.py and tools/tune_pets.py name the method): melee_taken_pct to both
                     yellows at once in the class's own play (by hand, a wizard's and an alchemist's root), the
                     low point averaged over one and two up near 25%, one up won 80% (a healer 90%, then eased 8%),
                     two up 65%; dealt_pct where no share met both win rates; a red three up won alone over 40%
                     made a loss by the share times 1.15^k; pet_hp_pct for every summoner's pet (a shaman's and a
                     wizard's too) on the pet the book gives, near a quarter over both yellows; and levels 1-10
                     against the low-level check (even fights won 95%, blue and green 97%, by hand AND in the
                     class's own play, in a new character's starting gear at 1-5). Levels 1-12 are each a row, above
                     them each curve the fewest rows within 4% of every level's fit (the pets' 7%); dealt_pct exact.

Every change is logged in balance_eqoaemu_log (the table, the row's key, the whole row before and after: NULL for one
added or taken out, why); --undo puts back each row still as this left it and drops the log; a second --apply only
does what is still to do. Both copy the database first (<db>.bak-<when>-pre-eqoaemurefit) unless --no-backup. Report
only by default. Nothing is done on a database the four tunings have not been applied to (tools/tune_balance.py,
retune_balance.py, tune_pets.py, fit_pet_selfbuffs.py first). The server reads the balance tables at its start and
at once at a save of any Balance page in the GM panel; a pet takes its numbers at its next summoning. Apply it with
the workbook's import (its spells and pets): the rows are fitted on them, and on the server's spells and pets
without them every class is far off its targets again. internal/database/balanceseed.go holds the same rows (a
table made new).

    tools/refit_eqoaemu_balance.py [--db eqoa.db] [--apply | --undo] [--no-backup]
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
sys.path.insert(0, HERE)
import fit_pet_selfbuffs as petself  # noqa: E402  the magician's 34-38 as the pets' fit again left them
import retune_balance as retune  # noqa: E402  the re-fit of 2026-10-30, and balanceclasslevel's order
import tune_pets as pets  # noqa: E402  the pet classes' rows over it

DEFAULT_DB = os.path.join(HERE, "..", "eqoa.db")
LOG = "balance_eqoaemu_log"
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,   -- balancelevel | balanceclasslevel
    place   TEXT NOT NULL,   -- the row's key, JSON
    before  TEXT,            -- the whole row before, JSON; NULL: added
    after   TEXT,            -- the whole row after, JSON; NULL: taken out
    why     TEXT NOT NULL,
    applied TEXT NOT NULL
)"""
CL = retune.TBL  # balanceclasslevel
WHY = "the balance re-fitted after EQOAEmu's workbook (tools/refit_eqoaemu_balance.py; 2026-11-01, \"Workbook wins, re-fit\")"

# balancelevel's pet columns as the four tunings left them (tools/tune_balance.py's), level -> (pet_hp_pct,
# pet_damage_pct), and the re-fit's at every row
PET_LEVEL_BEFORE = {
    1: (100.0, 250.0), 2: (100.0, 253.75), 3: (100.0, 257.5), 4: (100.0, 261.25), 5: (100.0, 265.0),
    6: (100.0, 263.3333333333333), 7: (100.0, 261.6666666666667), 8: (100.0, 260.0), 9: (120.0, 242.5),
    10: (140.0, 225.0), 11: (143.0, 236.0), 12: (146.0, 247.0), 13: (149.0, 258.0), 14: (152.0, 269.0),
    15: (155.0, 280.0), 18: (155.0, 300.0), 25: (190.0, 285.0), 30: (205.0, 300.0), 35: (250.0, 340.0),
    45: (375.0, 430.0), 50: (450.0, 475.0), 60: (540.0, 560.0),
}
PET_LEVEL = (100.0, 100.0)

# balanceclasslevel: class -> {level: (melee_taken_pct, pet_hp_pct, dealt_pct)}, the re-fit (see the doc)
REFIT = {}  # REFIT-ROWS: written by the fit's driver (NOTES.md), not by hand


def before_rows():
    """balanceclasslevel as the four tunings left it: the re-fit of 2026-10-30, the pet classes' rows of the pets' fit
    over it, the magician's 34-38 of the pets' fit again"""
    out = {c: {l: tuple(map(float, v)) for l, v in by.items()} for c, by in retune.RETUNED.items()}
    for c, by in pets.PETS.items():
        out[c] = {l: tuple(map(float, v)) for l, v in by.items()}
    for level, (old, new) in petself.CLASS_LEVELS.items():
        out[petself.MAGICIAN][level] = new
    return out


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def columns(db, tbl):
    return [r[1] for r in db.execute(f"PRAGMA table_info({tbl})")]


def row_of(db, tbl, key):
    where = " AND ".join(f"{k} = ?" for k in key)
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", list(key.values()))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def cl_values(r):
    return None if r is None else (float(r["melee_taken_pct"]), float(r["pet_hp_pct"]), float(r["dealt_pct"]))


def same(a, b):
    """two rows' figures alike (a seed's thirds read back from the database)"""
    if a is None or b is None:
        return a is b
    return len(a) == len(b) and all(math.isclose(x, y, rel_tol=1e-9, abs_tol=1e-9) for x, y in zip(a, b))


def after_rows():
    return {c: {l: tuple(map(float, v)) for l, v in by.items()} for c, by in REFIT.items()}


def tunings_done(db):
    """Whether the four tunings are what this goes from: tools/fit_pet_selfbuffs.py's log (or this tool's), or the
    magician's rows at 34 and 38 as it left them."""
    if has_table(db, petself.LOG) or has_table(db, LOG):
        return True
    ends = [cl_values(row_of(db, CL, {"class": petself.MAGICIAN, "level": level})) for level in (34, 38)]
    return same(ends[0], petself.CLASS_LEVELS[34][1]) and same(ends[1], petself.CLASS_LEVELS[38][1])


def is_refit(db):
    """Whether balanceclasslevel is the re-fit's rows already, every one (a table made new: the seeds are them)."""
    have = {(c, l): (float(m), float(p), float(d)) for c, l, m, p, d in
            db.execute(f"SELECT class, level, melee_taken_pct, pet_hp_pct, dealt_pct FROM {CL}")}
    want = {(c, l): v for c, by in after_rows().items() for l, v in by.items()}
    return set(have) == set(want) and all(same(have[k], want[k]) for k in want)


def plan(db):
    """The changes [(table, key, row before or None, row after or None)] and the rows left, with why."""
    changes, kept = [], []
    if not REFIT:
        return changes, [("everything", "no re-fit in this copy of the tool (REFIT is empty)")]
    if not has_table(db, CL) or "dealt_pct" not in columns(db, CL) or not has_table(db, "balancelevel"):
        return changes, [(CL, "no such table or no dealt_pct: tools/tune_balance.py and tools/retune_balance.py first")]
    if not tunings_done(db) and not is_refit(db):
        return changes, [("everything", "not the state the four tunings leave: tools/tune_balance.py, "
                                        "retune_balance.py, tune_pets.py and fit_pet_selfbuffs.py first")]
    was, want = before_rows(), after_rows()
    places = [(c, l) for c in sorted(set(was) | set(want)) for l in sorted(set(was.get(c, {})) | set(want.get(c, {})))]
    now = {p: cl_values(row_of(db, CL, {"class": p[0], "level": p[1]})) for p in places}
    for c, l in places:
        old, new = was.get(c, {}).get(l), want.get(c, {}).get(l)
        if same(old, new):
            continue
        have = now[(c, l)]
        if same(have, new):
            continue  # done already
        if not same(have, old):
            kept.append((f"{CL} {c} {l}", f"{have if have else 'no row'}, not the tunings' {old if old else 'no row'}: a GM's"))
            continue
        r = row_of(db, CL, {"class": c, "level": l})
        after = None
        if new is not None:
            after = dict(r) if r else {"class": c, "level": l, "note": "", "source": "seed"}
            after.update({"melee_taken_pct": new[0], "pet_hp_pct": new[1], "dealt_pct": new[2]})
        changes.append((CL, {"class": c, "level": l}, r, after))
    for level, old in sorted(PET_LEVEL_BEFORE.items()):
        r = row_of(db, "balancelevel", {"level": level})
        if r is None:
            kept.append((f"balancelevel {level}", "no such row: a GM took it out"))
            continue
        have = (float(r["pet_hp_pct"]), float(r["pet_damage_pct"]))
        if same(have, PET_LEVEL):
            continue
        if not same(have, old):
            kept.append((f"balancelevel {level}", f"pet_hp_pct/pet_damage_pct {have[0]:g}/{have[1]:g}, not the "
                                                  f"tunings' {old[0]:g}/{old[1]:g}: a GM's"))
            continue
        changes.append(("balancelevel", {"level": level}, r, dict(r, pet_hp_pct=PET_LEVEL[0], pet_damage_pct=PET_LEVEL[1])))
    return changes, kept


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst, n = f"{path}.bak-{when}-pre-eqoaemurefit", 1
    while os.path.exists(dst):
        n += 1
        dst = f"{path}.bak-{when}-{n}-pre-eqoaemurefit"
    shutil.copy2(path, dst)
    return dst


def write_row(db, tbl, key, row, insert):
    where = " AND ".join(f"{k} = ?" for k in key)
    if insert:
        cols = list(row)
        db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [row[c] for c in cols])
    elif row is None:
        db.execute(f"DELETE FROM {tbl} WHERE {where}", list(key.values()))
    else:
        cols = [c for c in row if c not in key]
        db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in cols)} WHERE {where}",
                   [row[c] for c in cols] + list(key.values()))


def apply(db, changes):
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_SCHEMA)
        for tbl, key, before, after in changes:
            db.execute(f"INSERT INTO {LOG} (tbl, place, before, after, why, applied) VALUES (?, ?, ?, ?, ?, ?)",
                       (tbl, json.dumps(key, sort_keys=True), None if before is None else json.dumps(before, sort_keys=True),
                        None if after is None else json.dumps(after, sort_keys=True), WHY, when))
            write_row(db, tbl, key, after, insert=before is None)
        retune.reorder(db)
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
        for seq, tbl, place, before, after in logged:
            if tbl not in (CL, "balancelevel"):
                raise SystemExit(f"{LOG} names table {tbl!r}, which this tool never writes")
            key = json.loads(place)
            now = row_of(db, tbl, key)
            if now != (None if after is None else json.loads(after)):
                print(f"  {tbl} {place}: changed since: left as it is")
                continue
            write_row(db, tbl, key, None if before is None else json.loads(before), insert=now is None)
            back += 1
        retune.reorder(db)
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"put back {back} of {len(logged)} rows; the server takes them at its next start or a Balance page's next save")


def describe(change):
    tbl, key, before, after = change
    k = " ".join(str(v) for v in key.values())
    if tbl == "balancelevel":
        return (f"balancelevel {k}: pet_hp_pct {before['pet_hp_pct']:g} -> {after['pet_hp_pct']:g}, pet_damage_pct "
                f"{before['pet_damage_pct']:g} -> {after['pet_damage_pct']:g}")
    if before is None:
        return (f"{tbl} class {key['class']} level {key['level']}: added, melee_taken_pct {after['melee_taken_pct']:g}, "
                f"pet_hp_pct {after['pet_hp_pct']:g}, dealt_pct {after['dealt_pct']:g}")
    if after is None:
        return (f"{tbl} class {key['class']} level {key['level']}: taken out (was {before['melee_taken_pct']:g}/"
                f"{before['pet_hp_pct']:g}/{before['dealt_pct']:g})")
    diff = [f"{c} {before[c]:g} -> {after[c]:g}" for c in ("melee_taken_pct", "pet_hp_pct", "dealt_pct") if before[c] != after[c]]
    return f"{tbl} class {key['class']} level {key['level']}: " + ", ".join(diff)


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the re-fit (logged)")
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
          "Balance page in the GM panel (a pet at its next summoning)")


if __name__ == "__main__":
    main()
