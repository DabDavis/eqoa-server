#!/usr/bin/env python3
"""The balance re-fitted again for the EQOAEmu follow-up fixes (2026-11-01, after the 10:20 UTC import and its re-fit,
tools/refit_eqoaemu_balance.py): the classes those fixes moved, fitted again by that re-fit's method to the same targets.

What the follow-up moved, measured with the fight simulator (cmd/balancesim against SOE's roamer, every level 1-60, 40
fights a cell, the player's low point averaged over the fights won one and two up) on eqoaclassic's database of 14:07
UTC with the follow-up's data in it (tools/fix_eqoaemu_followup.py, tools/seed_spell_gates.py), under the 10:20
re-fit's rows, against the 10:20 state (that database before the follow-up, the 10:20 server):

  a monk's bare hands roll a hand-to-hand DMG by its level (offence.go) and its kit is bare-handed, so its 30 punches
  are cast: a yellow left it 34-50% at 1-59 (43 levels off the targets), and at 60, its share fitted on a kit holding a
  broken record (a DMG of 999, which no kit wears now), it won a yellow one up 20% of the time;
  a ranger dual wields its slashing blades and switches to its bow for its arrow arts as the server now switches a
  player's hands (handlers/handswitch.go: on demand, never back by itself, so after an arrow art in melee it swings its
  bow until a blade art), its bow rolling its DMG: 26 levels off, a yellow two up won 0-55% at 8-25, every fight lost at
  60 (its share fitted on another broken record, Sapphire's Sword);
  a paladin's kit switches to a blunt weapon for its strikes, and Smite Undead's proc lands on the undead alone: 2-10
  too easy (35-46% left), 11, 17, 33, 52 and 55 too hard;
  a bard of 3-13, whose first arts do not choose its weapon (fewer than three name one: balancekit.go's simHandsLine),
  easier, at 7-8 a yellow two up lost, and at 60 a yellow two up won 35%;
  Avatar (a shaman's of 51) heals once at its landing, not 1500 a tick: at the 10:20 re-fit's 4-5x (fitted on the
  tick) a shaman of 51-54 lost every yellow;
  a rooting wizard of 50, its staff the Phase Staff until the follow-up made it NO RENT (no kit wears one: INT 175 ->
  150, power 659 -> 624), won a yellow two up 48% (68% at 10:20), and one of 60 57%;
  at the edges of the targets: a shadow knight of 53 and 60 (one up won 75-78%), a druid of 41 (its red won alone 57%)
  and of 53 (38% left), an enchanter's pet at 52 and 60 (one up won 75% and 62%), an alchemist of 16 rooting (two up
  52%).

The rows, by the 10:20 re-fit's stages (yellow, dealt, red, low, the cut; tools/refit_eqoaemu_balance.py says them):
the monk's, the ranger's and the paladin's at every level 1-60, the bard's 1-13 and 60, the shaman's 51-54 (shares
146-200, from 301-476; its pet's column kept), levels 1-12 each a row and above them the fewest within 4% of every
level's fit, dealt exact. Cells set after, each a row and checked with the simulator: five at the cut's edge a step of
0.9 lower (a ranger of 33, a paladin of 33 and 52, a monk of 12, a bard of 8), a ranger of 9-11 at the share search's
floor (15-18, a level 10 roamer's crits still leaving it 12-16%) dealing the least of 115 and 130 that brings it within
6 of the target, and the edges above, each a one-level span: a shadow knight of 53 and 60 a step of 0.9 lower, a druid
of 41 times 1.15 (its red) and of 53 times 1.15 (too easy), an enchanter's pet at 52 and 60 at 40 and 33 (a point or
two more wins its reds alone), an alchemist of 16 a step of 0.9 lower, a rooting wizard of 50 and 60 at the yellow
stage's 65 and 110 (its win rates met, its low point the root play's 46% and 76%, as at its other levels: won with the
root held, lost when it breaks). Under them every cell on the targets at 10:20 is on them; as at 10:20, a level 1's
yellow one up is too easy for a paladin and a bard, and a shaman's pet at 52 and 54 comes out of its yellows at
12-14%.

It writes balanceclasslevel's rows of each class in SPAN (its spans of levels lo..hi) to FOLLOWUP's, each change only while
the row holds what tools/refit_eqoaemu_balance.py left it (a row a GM changed is left, and said); the other classes'
rows and the rows outside a class's span are not touched, nor balancelevel. Every change is logged in
balance_eqoaemu_followup_log (the row's key, the whole row before and after: NULL for one added or taken out, why);
--undo puts back each row still as this left it and drops the log; a second --apply only does what is still to do.
Both copy the database first (<db>.bak-<when>-pre-eqoaemufollowup) unless --no-backup. Report only by default.
Nothing is done on a database the 10:20 re-fit has not been applied to (tools/refit_eqoaemu_balance.py first). The
server reads the balance tables at its start and at once at a save of any Balance page in the GM panel. Apply it
with the follow-up's server and data (tools/fix_eqoaemu_followup.py, tools/seed_spell_gates.py): the rows are fitted
on them. internal/database/balanceseed.go holds the same rows (a table made new).

    tools/refit_eqoaemu_followup.py [--db eqoa.db] [--apply | --undo] [--no-backup]
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
import refit_eqoaemu_balance as rf  # noqa: E402  the 10:20 re-fit: what this goes from
import retune_balance as retune  # noqa: E402  balanceclasslevel's order

DEFAULT_DB = os.path.join(HERE, "..", "eqoa.db")
LOG = "balance_eqoaemu_followup_log"
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
WHY = ("the balance re-fitted for the EQOAEmu follow-up fixes (tools/refit_eqoaemu_followup.py; 2026-11-01: a monk's "
       "bare hands, a ranger's blades and bow, undead-only procs, Avatar once, no NO RENT kits)")

# class -> [(lo, hi), ...]: the levels fitted again; FOLLOWUP's rows there take the place of the 10:20 re-fit's
SPAN = {
    1: [(1, 60)],
    2: [(1, 60)],
    3: [(53, 53), (60, 60)],
    4: [(1, 60)],
    5: [(1, 13), (60, 60)],
    7: [(41, 41), (52, 53)],
    8: [(51, 54)],
    12: [(51, 53), (59, 60)],
    13: [(50, 50), (60, 60)],
    14: [(16, 16)],
}
# balanceclasslevel: class -> {level: (melee_taken_pct, pet_hp_pct, dealt_pct)}, the follow-up's rows in its span
FOLLOWUP = {
    1: {1: (51, 100, 100), 2: (42, 100, 100), 3: (51, 100, 100), 4: (65, 100, 100), 5: (60, 100, 100), 6: (63, 100, 100), 7: (63, 100, 100), 8: (54, 100, 150), 9: (18, 100, 115), 10: (15, 100, 130), 11: (16, 100, 115), 12: (19, 100, 100), 13: (25, 100, 100), 14: (28, 100, 100), 15: (28, 100, 100), 16: (19, 100, 100), 17: (25, 100, 100), 18: (19, 100, 100), 20: (19, 100, 100), 21: (17, 100, 100), 22: (19, 100, 100), 23: (17, 100, 100), 24: (32, 100, 100), 25: (25, 100, 100), 26: (25, 100, 100), 27: (19, 100, 100), 28: (25, 100, 100), 29: (19, 100, 115), 30: (52, 100, 100), 31: (59, 100, 100), 33: (41, 100, 100), 34: (49, 100, 100), 35: (38, 100, 100), 38: (49, 100, 100), 39: (40, 100, 100), 40: (52, 100, 100), 41: (36, 100, 100), 43: (46, 100, 100), 44: (46, 100, 100), 45: (30, 100, 100), 46: (36, 100, 100), 47: (36, 100, 100), 48: (28, 100, 100), 49: (43, 100, 100), 50: (32, 100, 100), 51: (38, 100, 100), 52: (40, 100, 100), 53: (55, 100, 100), 54: (32, 100, 100), 55: (34, 100, 100), 56: (46, 100, 100), 57: (38, 100, 100), 58: (40, 100, 100), 59: (28, 100, 100), 60: (71, 100, 100)},
    2: {1: (78, 100, 100), 2: (78, 100, 100), 3: (94, 100, 100), 4: (95, 100, 100), 5: (143, 100, 100), 6: (117, 100, 100), 7: (181, 100, 100), 8: (149, 100, 100), 9: (117, 100, 100), 10: (117, 100, 100), 11: (97, 100, 100), 12: (124, 100, 100), 13: (110, 100, 100), 15: (110, 100, 100), 16: (91, 100, 100), 17: (85, 100, 100), 18: (59, 100, 100), 19: (67, 100, 100), 20: (110, 100, 100), 21: (91, 100, 100), 22: (103, 100, 100), 23: (85, 100, 100), 24: (117, 100, 100), 25: (97, 100, 100), 26: (91, 100, 100), 27: (113, 100, 100), 28: (97, 100, 100), 29: (91, 100, 100), 30: (110, 100, 100), 31: (103, 100, 100), 32: (110, 100, 100), 33: (99, 100, 100), 34: (110, 100, 100), 35: (91, 100, 100), 36: (91, 100, 100), 37: (113, 100, 100), 38: (124, 100, 100), 39: (117, 100, 100), 41: (124, 100, 100), 42: (117, 100, 100), 44: (150, 100, 100), 47: (115, 100, 100), 48: (115, 100, 100), 49: (132, 100, 100), 51: (150, 100, 100), 52: (135, 100, 100), 53: (150, 100, 100), 54: (140, 100, 100), 55: (154, 100, 100), 56: (132, 100, 100), 57: (169, 100, 100), 58: (175, 100, 100), 60: (224, 100, 100)},
    3: {53: (369, 100, 130), 60: (364, 100, 130)},
    4: {1: (95, 100, 100), 2: (80, 100, 115), 3: (111, 100, 100), 4: (205, 100, 175), 5: (105, 100, 105), 6: (105, 100, 100), 7: (86, 100, 100), 8: (82, 100, 115), 9: (66, 100, 115), 10: (63, 100, 115), 11: (46, 100, 100), 12: (51, 100, 100), 13: (85, 100, 100), 14: (67, 100, 100), 15: (110, 100, 100), 17: (85, 100, 100), 18: (52, 100, 100), 19: (52, 100, 100), 20: (76, 100, 100), 22: (76, 100, 100), 23: (67, 100, 100), 24: (76, 100, 100), 26: (80, 100, 100), 27: (71, 100, 100), 28: (71, 100, 100), 29: (76, 100, 100), 30: (97, 100, 100), 32: (103, 100, 100), 33: (132, 100, 100), 34: (110, 100, 100), 35: (140, 100, 100), 39: (140, 100, 100), 40: (124, 100, 100), 41: (140, 100, 100), 42: (110, 100, 100), 43: (124, 100, 100), 44: (124, 100, 100), 45: (140, 100, 100), 47: (140, 100, 100), 48: (110, 100, 100), 49: (159, 100, 100), 50: (140, 100, 100), 51: (159, 100, 100), 52: (150, 100, 100), 53: (180, 100, 100), 54: (159, 100, 100), 55: (159, 100, 100), 56: (180, 100, 100), 57: (169, 100, 100), 58: (140, 100, 100), 59: (124, 100, 100), 60: (192, 100, 100)},
    5: {1: (34, 100, 100), 2: (51, 100, 100), 3: (70, 100, 100), 4: (95, 100, 100), 5: (89, 100, 100), 6: (86, 100, 100), 7: (78, 100, 100), 8: (98, 100, 175), 9: (66, 100, 105), 10: (42, 100, 100), 11: (52, 100, 100), 12: (40, 100, 100), 13: (76, 100, 100), 60: (67, 100, 100)},
    7: {41: (86, 100, 100), 52: (48, 100, 100), 53: (62, 100, 100)},
    8: {51: (200, 113, 100), 52: (188, 106, 100), 53: (176, 126, 100), 54: (146, 126, 100)},
    12: {51: (100, 37.5, 100), 52: (100, 40, 100), 53: (100, 38.5, 100), 59: (100, 32, 100), 60: (100, 33, 100)},
    13: {50: (65, 100, 100), 60: (110, 100, 100)},
    14: {16: (35, 100, 100)},
}


def in_span(c, level):
    return any(lo <= level <= hi for lo, hi in SPAN.get(c, ()))


def before_rows():
    """the 10:20 re-fit's rows (tools/refit_eqoaemu_balance.py's REFIT) in each span"""
    was = rf.after_rows()
    return {c: {l: v for l, v in was.get(c, {}).items() if in_span(c, l)} for c in SPAN}


def after_rows():
    return {c: {l: tuple(map(float, v)) for l, v in by.items()} for c, by in FOLLOWUP.items()}


def whole_rows():
    """balanceclasslevel as the two re-fits leave it: the 10:20 re-fit's rows, each span's FOLLOWUP's (the seeds)"""
    out = rf.after_rows()
    for c, by in after_rows().items():
        out[c] = {l: v for l, v in out.get(c, {}).items() if not in_span(c, l)}
        out[c].update(by)
        out[c] = dict(sorted(out[c].items()))
    return out


def table_rows(db):
    return {(c, l): (float(m), float(p), float(d)) for c, l, m, p, d in
            db.execute(f"SELECT class, level, melee_taken_pct, pet_hp_pct, dealt_pct FROM {CL}")}


def is_whole(db):
    """Whether balanceclasslevel is the two re-fits' rows already, every one (a table made new: the seeds are them)."""
    have = table_rows(db)
    want = {(c, l): v for c, by in whole_rows().items() for l, v in by.items()}
    return set(have) == set(want) and all(rf.same(have[k], want[k]) for k in want)


def refit_done(db):
    """Whether the 10:20 re-fit is what this goes from: its log (or this tool's), or its rows, every one."""
    return rf.has_table(db, rf.LOG) or rf.has_table(db, LOG) or rf.is_refit(db)


def plan(db):
    """The changes [(table, key, row before or None, row after or None)] and the rows left, with why."""
    changes, kept = [], []
    if not FOLLOWUP:
        return changes, [("everything", "no re-fit in this copy of the tool (FOLLOWUP is empty)")]
    if not rf.has_table(db, CL) or "dealt_pct" not in rf.columns(db, CL):
        return changes, [(CL, "no such table or no dealt_pct: tools/tune_balance.py and tools/retune_balance.py first")]
    if not refit_done(db) and not is_whole(db):
        return changes, [("everything", "not the state the 10:20 re-fit leaves: tools/refit_eqoaemu_balance.py first")]
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
            kept.append((f"{CL} {c} {l}", f"{have if have else 'no row'}, not the 10:20 re-fit's "
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
    dst, n = f"{path}.bak-{when}-pre-eqoaemufollowup", 1
    while os.path.exists(dst):
        n += 1
        dst = f"{path}.bak-{when}-{n}-pre-eqoaemufollowup"
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
