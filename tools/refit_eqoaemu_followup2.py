#!/usr/bin/env python3
"""The balance re-fitted again for the EQOAEmu follow-ups' second round (2026-11-01, evening, after the 16:38 UTC re-fit,
tools/refit_eqoaemu_followup.py): the classes that round moved, fitted again by the same method to the same targets.

What the second round moved, measured with the fight simulator (cmd/balancesim against SOE's roamer, every level 1-60, 40
fights a cell, the player's low point averaged over the fights won one and two up) on eqoaclassic's database of 18:59
UTC with the round's data in it (tools/fix_eqoaemu_followup2.py, fix_eqoaemu_places.py, fix_broken_records.py,
fix_scrolls.py, remove_leftover_spells.py, fix_eqoaemu_pet_abilities.py), under the 16:38 re-fit's rows, against the
state live at 18:59 (that database before the round, the 18:50 server):

  a ranger is back on its blades after an arrow art in melee reach (handlers/handswitch.go, the simulator too), so from
  level 8, its first arrow art, its fights are shorter: through a yellow it kept 3-10 points more (ten levels over the
  targets' 36%), and at 20 it won a red alone 42% of the time;
  the simulator's player casts its book's pure debuffs and drains in a long fight (simDebuffs), and a creature's effects
  count in its fights (a player's debuffs: STR and DEX its rolls, AC and AGI the rolls on it, a maximum its hit points):
  a shaman of 4-60 (its Wind and Malady lines) won its reds alone at nine levels of 4-16 (42-95%) and at 37-60
  (45-100%, 98-100% from 45) and kept 3-15 points more through a yellow, its pet 4-19 points more (its reds won alone at
  57-59); a rogue of 19-60 kept 4-10 points more; a shadow knight of 21-50, which spends power and time on its debuffs
  and drains where its taps were, kept less and won less (a yellow one up won 57-78% at 24 and 29-31, two up 22-60% at
  29-33, 8-14% left at 34-38); a warrior's Rend (46: it lowers the armour of what it strikes) and a bard's Deadly
  Verses (51, AC) count on the creature now: a warrior of 46-60 kept 4-6 points more (54, 55 and 58 over 36%), a bard
  of 58-59 7 more;
  the pets' abilities (the necromancer's pets from 30 cast Undead Bane, and one with Undead Blood no longer its owner's
  tap; the magician's mastery pets buff themselves, which no kit's book summons): a necromancer's pet of 34-43 kept
  40-48% and won its reds alone 45-98% at seven of those levels, one of 56-60 kept 38-42%; an enchanter's pet of 5-6 and
  36-43 kept 36-46% and won its reds alone at 44-45, 49, 55-56 and 60;
  the druid's four EQOA-Data leftovers are out of its list (Nature Form, which roots its caster, among them): a druid of
  50-60 won its reds alone 98-100% of the time;
  the broken records take real figures or lose the template (the eight test pieces, plain level 60 gear now, are what
  level 60 kits wear): every class of 60 moved a little, a bard of 60 most (24% -> 38% on the data alone, 44% in all).
The paladin, the monk, the cleric, the magician, the wizard and the alchemist moved a point or two or not at all, and
stay on the targets as they were.

The rows, by the re-fits' stages (yellow, dealt, red, pets, low, the cut; tools/refit_eqoaemu_balance.py says them):
the ranger's 8-60, the shaman's 4-60 and its pet's, the rogue's 19-60, the shadow knight's 21-51 (its share alone, at the
undead check's dealt bands of 150 and 130), the warrior's 46-60, the bard's 57-60, the druid's 50-60, the enchanter's
pet's 5-60 and the necromancer's pet's 5-7 and 34-60, levels 1-12 each a row and above them the fewest within 4% of every
level's fit (the pets' 7%), dealt exact. Cells set after, each a row and checked with the simulator: a ranger of 10 at 16
dealing 115 (the cut's 17 at 100 left it 14%: a level 10 roamer's crits, the share alike at 15 and 16), a shadow knight
of 24 at the yellow stage's own 322 and of 28 and 50 a step of 0.9 lower (the cut's line left a yellow's win rate just
under its bar), a druid of 60 at 135 (the cut's 131 left its red won alone 42%, the red stage's 131.1 40%), and the
enchanter's pet at 45 and at every level 49-60 its own row of 31-40 (the knife edge between its yellows and its reds
alone: each the hit points that win the yellows and lose the red, under where the red is won).
DOCSTRING-OUTCOME

It writes balanceclasslevel's rows of each class in SPAN (its spans of levels lo..hi) to FOLLOWUP's, each change only
while the row holds what tools/refit_eqoaemu_followup.py left it (a row a GM changed is left, and said); the other
classes' rows and the rows outside a class's span are not touched, nor balancelevel. Every change is logged in
balance_eqoaemu_followup2_log (the row's key, the whole row before and after: NULL for one added or taken out, why);
--undo puts back each row still as this left it and drops the log; a second --apply only does what is still to do.
Both copy the database first (<db>.bak-<when>-pre-eqoaemufollowup2) unless --no-backup. Report only by default.
Nothing is done on a database the 16:38 re-fit has not been applied to (tools/refit_eqoaemu_followup.py first). The
server reads the balance tables at its start and at once at a save of any Balance page in the GM panel. Apply it with
the second round's server and data (tools/fix_eqoaemu_followup2.py, fix_eqoaemu_places.py, fix_broken_records.py,
fix_scrolls.py, remove_leftover_spells.py, fix_eqoaemu_pet_abilities.py): the rows are fitted on them.
internal/database/balanceseed.go holds the same rows (a table made new).

    tools/refit_eqoaemu_followup2.py [--db eqoa.db] [--apply | --undo] [--no-backup]
"""
import argparse
import datetime
import json
import os
import shutil
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import refit_eqoaemu_balance as rf  # noqa: E402  the 10:20 re-fit: the row helpers
import refit_eqoaemu_followup as fu  # noqa: E402  the 16:38 re-fit: what this goes from
import retune_balance as retune  # noqa: E402  balanceclasslevel's order

DEFAULT_DB = os.path.join(HERE, "..", "eqoa.db")
LOG = "balance_eqoaemu_followup2_log"
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,   -- balanceclasslevel
    place   TEXT NOT NULL,   -- the row's key, JSON
    before  TEXT,            -- the whole row before, JSON; NULL: added
    after   TEXT,            -- the whole row after, JSON; NULL: taken out
    why     TEXT NOT NULL,
    applied TEXT NOT NULL
)"""
CL = rf.CL  # balanceclasslevel
WHY = ("the balance re-fitted for the EQOAEmu follow-ups' second round (tools/refit_eqoaemu_followup2.py; 2026-11-01 "
       "evening: a ranger's blades back after an arrow art, creatures' effects counting, the simulator's debuffs, roots "
       "held until a hit, the pets' abilities, the druid's leftovers out, the broken records fixed)")

# class -> [(lo, hi), ...]: the levels fitted again; FOLLOWUP's rows there take the place of the 16:38 re-fit's
SPAN = {
    0: [(46, 60)],
    1: [(8, 60)],
    3: [(21, 51)],
    4: [(60, 60)],
    5: [(57, 60)],
    6: [(19, 60)],
    7: [(50, 60)],
    8: [(4, 48), (49, 60)],
    11: [(5, 7), (34, 60)],
    12: [(5, 60)],
}
# balanceclasslevel: class -> {level: (melee_taken_pct, pet_hp_pct, dealt_pct)}, this re-fit's rows in its span
FOLLOWUP = {
    0: {46: (97, 100, 100), 47: (91, 100, 100), 48: (71, 100, 100), 49: (85, 100, 100), 51: (91, 100, 100), 52: (117, 100, 100), 53: (91, 100, 100), 54: (124, 100, 100), 55: (117, 100, 100), 56: (91, 100, 100), 57: (132, 100, 100), 58: (124, 100, 100), 59: (103, 100, 100), 60: (110, 100, 100)},
    1: {8: (54, 100, 130), 9: (18, 100, 100), 10: (16, 100, 115), 11: (16, 100, 100), 12: (25, 100, 100), 13: (25, 100, 100), 14: (32, 100, 100), 15: (32, 100, 100), 16: (19, 100, 100), 17: (25, 100, 100), 18: (19, 100, 100), 19: (19, 100, 100), 20: (28, 100, 100), 21: (19, 100, 100), 22: (25, 100, 100), 23: (25, 100, 100), 24: (40, 100, 100), 25: (28, 100, 100), 27: (22, 100, 100), 28: (25, 100, 100), 29: (19, 100, 100), 30: (55, 100, 100), 31: (63, 100, 100), 33: (52, 100, 100), 34: (52, 100, 100), 35: (40, 100, 100), 36: (43, 100, 100), 38: (59, 100, 100), 39: (52, 100, 100), 40: (67, 100, 100), 41: (43, 100, 100), 42: (46, 100, 100), 44: (59, 100, 100), 45: (36, 100, 100), 47: (43, 100, 100), 48: (32, 100, 100), 49: (49, 100, 100), 50: (36, 100, 100), 52: (52, 100, 100), 53: (67, 100, 100), 54: (36, 100, 100), 55: (38, 100, 100), 56: (52, 100, 100), 57: (43, 100, 100), 58: (46, 100, 100), 59: (32, 100, 100), 60: (76, 100, 100)},
    3: {21: (315, 100, 150), 22: (335, 100, 150), 24: (322, 100, 150), 26: (335, 100, 150), 27: (296, 100, 150), 28: (286, 100, 150), 29: (204, 100, 130), 32: (231, 100, 130), 33: (180, 100, 130), 34: (204, 100, 130), 35: (169, 100, 130), 37: (140, 100, 130), 38: (159, 100, 130), 39: (192, 100, 130), 41: (217, 100, 130), 43: (217, 100, 130), 44: (180, 100, 130), 45: (159, 100, 130), 46: (180, 100, 130), 47: (180, 100, 130), 48: (150, 100, 130), 49: (315, 100, 130), 50: (302, 100, 130), 51: (357, 100, 130)},
    4: {60: (173, 100, 100)},
    5: {57: (83, 100, 100), 58: (94, 100, 100), 59: (73, 100, 100), 60: (76, 100, 100)},
    6: {19: (19, 100, 100), 20: (40, 100, 100), 21: (32, 100, 100), 22: (36, 100, 100), 23: (28, 100, 100), 24: (59, 100, 100), 25: (40, 100, 100), 26: (28, 100, 100), 27: (22, 100, 100), 28: (28, 100, 100), 29: (25, 100, 100), 30: (28, 100, 100), 33: (28, 100, 100), 34: (36, 100, 100), 35: (40, 100, 100), 36: (28, 100, 100), 37: (40, 100, 100), 39: (32, 100, 100), 40: (46, 100, 100), 41: (52, 100, 100), 42: (52, 100, 100), 43: (40, 100, 100), 45: (52, 100, 100), 46: (46, 100, 100), 47: (52, 100, 100), 48: (34, 100, 100), 49: (67, 100, 100), 50: (52, 100, 100), 51: (52, 100, 100), 53: (64, 100, 100), 54: (40, 100, 100), 55: (55, 100, 100), 57: (52, 100, 100), 58: (46, 100, 100), 59: (22, 100, 100), 60: (52, 100, 100)},
    7: {50: (79, 100, 100), 51: (89, 100, 100), 52: (89, 100, 100), 53: (114, 100, 100), 54: (70, 100, 100), 55: (70, 100, 100), 56: (79, 100, 100), 57: (61, 100, 100), 58: (116, 100, 100), 59: (79, 100, 100), 60: (135, 100, 100)},
    8: {4: (78, 268, 100), 5: (95, 268, 100), 6: (117, 268, 100), 7: (143, 268, 100), 8: (143, 268, 100), 9: (95, 268, 100), 10: (95, 268, 100), 11: (99, 268, 100), 12: (114, 268, 100), 13: (114, 295, 100), 14: (129, 321, 100), 15: (129, 348, 100), 16: (101, 374, 100), 17: (114, 401, 100), 18: (101, 537, 100), 19: (79, 673, 100), 20: (89, 378, 100), 21: (79, 415, 100), 23: (101, 489, 100), 25: (107, 562, 100), 26: (101, 599, 100), 27: (114, 636, 100), 28: (114, 673, 100), 29: (114, 134, 100), 30: (114, 95, 100), 31: (129, 104, 100), 32: (129, 113, 100), 33: (114, 135, 100), 35: (121, 179, 100), 36: (106, 186, 100), 38: (129, 201, 100), 39: (122, 119, 100), 40: (116, 80, 100), 42: (114, 90, 100), 43: (129, 95, 100), 44: (129, 100, 100), 45: (114, 110, 100), 47: (114, 132, 100), 48: (171, 142, 100), 49: (129, 100, 100), 50: (129, 107, 100), 51: (272, 114, 100), 52: (240, 121, 100), 54: (226, 134, 100), 55: (129, 141, 100), 56: (146, 148, 100), 57: (197, 155, 100), 58: (216, 162, 100), 59: (166, 169, 100), 60: (219, 113, 100)},
    11: {5: (18, 253, 100), 6: (28, 284, 100), 7: (32, 357, 100), 34: (100, 71, 100), 35: (100, 63, 100), 36: (100, 71, 100), 39: (100, 67, 100), 40: (100, 50, 100), 43: (100, 63, 100), 46: (100, 63, 100), 48: (100, 75, 100), 49: (100, 53, 100), 51: (100, 50, 100), 54: (100, 63, 100), 55: (100, 56, 100), 59: (100, 84, 100), 60: (100, 60, 100)},
    12: {5: (17, 504, 100), 6: (26, 566, 100), 7: (29, 734, 100), 8: (24, 71, 100), 9: (26, 95, 100), 10: (16, 113, 115), 11: (100, 142, 100), 12: (100, 178, 100), 15: (100, 284, 100), 16: (100, 71, 100), 17: (100, 80, 100), 19: (100, 179, 100), 23: (100, 268, 100), 24: (100, 63, 100), 29: (100, 100, 100), 32: (100, 142, 100), 33: (100, 201, 100), 34: (100, 56, 100), 40: (100, 75, 100), 43: (100, 106, 100), 44: (100, 45, 100), 45: (100, 45, 100), 48: (100, 50, 100), 49: (100, 37.5, 100), 50: (100, 39, 100), 51: (100, 39, 100), 52: (100, 40, 100), 53: (100, 39, 100), 54: (100, 39, 100), 55: (100, 32, 100), 56: (100, 32, 100), 57: (100, 32, 100), 58: (100, 33, 100), 59: (100, 33, 100), 60: (100, 31, 100)},
}


def in_span(c, level):
    return any(lo <= level <= hi for lo, hi in SPAN.get(c, ()))


def before_rows():
    """the 16:38 re-fit's rows (tools/refit_eqoaemu_followup.py's whole table) in each span"""
    was = fu.whole_rows()
    return {c: {l: v for l, v in was.get(c, {}).items() if in_span(c, l)} for c in SPAN}


def after_rows():
    return {c: {l: tuple(map(float, v)) for l, v in by.items()} for c, by in FOLLOWUP.items()}


def whole_rows():
    """balanceclasslevel as the three re-fits leave it: the 16:38 re-fit's table, each span's FOLLOWUP's (the seeds)"""
    out = fu.whole_rows()
    for c, by in after_rows().items():
        out[c] = {l: v for l, v in out.get(c, {}).items() if not in_span(c, l)}
        out[c].update(by)
        out[c] = dict(sorted(out[c].items()))
    return out


def is_whole(db):
    """Whether balanceclasslevel is the three re-fits' rows already, every one (a table made new: the seeds are them)."""
    have = fu.table_rows(db)
    want = {(c, l): v for c, by in whole_rows().items() for l, v in by.items()}
    return set(have) == set(want) and all(rf.same(have[k], want[k]) for k in want)


def followup_done(db):
    """Whether the 16:38 re-fit is what this goes from: its log (or this tool's), or its rows, every one."""
    return rf.has_table(db, fu.LOG) or rf.has_table(db, LOG) or fu.is_whole(db)


def plan(db):
    """The changes [(table, key, row before or None, row after or None)] and the rows left, with why."""
    changes, kept = [], []
    if not FOLLOWUP:
        return changes, [("everything", "no re-fit in this copy of the tool (FOLLOWUP is empty)")]
    if not rf.has_table(db, CL) or "dealt_pct" not in rf.columns(db, CL):
        return changes, [(CL, "no such table or no dealt_pct: tools/tune_balance.py and tools/retune_balance.py first")]
    if not followup_done(db) and not is_whole(db):
        return changes, [("everything", "not the state the 16:38 re-fit leaves: tools/refit_eqoaemu_followup.py first")]
    was, want = before_rows(), after_rows()
    places = [(c, l) for c in sorted(SPAN) for l in sorted(set(was.get(c, {})) | set(want.get(c, {})))]
    for c, l in places:
        old, new = was.get(c, {}).get(l), want.get(c, {}).get(l)
        if rf.same(old, new):
            continue
        r = rf.row_of(db, CL, {"class": c, "level": l})
        have = rf.cl_values(r)
        if rf.same(have, new):
            continue  # done already
        if not rf.same(have, old):
            kept.append((f"{CL} {c} {l}", f"{have if have else 'no row'}, not the 16:38 re-fit's "
                                          f"{old if old else 'no row'}: a GM's"))
            continue
        after = None
        if new is not None:
            after = dict(r) if r else {"class": c, "level": l, "note": "", "source": "seed"}
            after.update({"melee_taken_pct": new[0], "pet_hp_pct": new[1], "dealt_pct": new[2]})
        changes.append((CL, {"class": c, "level": l}, r, after))
    return changes, kept


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst, n = f"{path}.bak-{when}-pre-eqoaemufollowup2", 1
    while os.path.exists(dst):
        n += 1
        dst = f"{path}.bak-{when}-{n}-pre-eqoaemufollowup2"
    shutil.copy2(path, dst)
    return dst


def apply(db, changes):
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_SCHEMA)
        for tbl, key, before, after in changes:
            db.execute(f"INSERT INTO {LOG} (tbl, place, before, after, why, applied) VALUES (?, ?, ?, ?, ?, ?)",
                       (tbl, json.dumps(key, sort_keys=True), None if before is None else json.dumps(before, sort_keys=True),
                        None if after is None else json.dumps(after, sort_keys=True), WHY, when))
            rf.write_row(db, tbl, key, after, insert=before is None)
        retune.reorder(db)
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db):
    if not rf.has_table(db, LOG):
        print("nothing logged: nothing to undo")
        return
    logged = db.execute(f"SELECT seq, tbl, place, before, after FROM {LOG} ORDER BY seq DESC").fetchall()
    back = 0
    db.execute("BEGIN IMMEDIATE")
    try:
        for seq, tbl, place, before, after in logged:
            if tbl != CL:
                raise SystemExit(f"{LOG} names table {tbl!r}, which this tool never writes")
            key = json.loads(place)
            now = rf.row_of(db, tbl, key)
            if now != (None if after is None else json.loads(after)):
                print(f"  {tbl} {place}: changed since: left as it is")
                continue
            rf.write_row(db, tbl, key, None if before is None else json.loads(before), insert=now is None)
            back += 1
        retune.reorder(db)
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"put back {back} of {len(logged)} rows; the server takes them at its next start or a Balance page's next save")


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
        if not a.no_backup and rf.has_table(db, LOG):
            print(f"copy: {backup(a.db)}")
        undo(db)
        return
    changes, kept = plan(db)
    for c in changes:
        print(rf.describe(c))
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
