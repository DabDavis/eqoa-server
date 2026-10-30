#!/usr/bin/env python3
"""The pets' fit of 2026-10-30: the pet classes' pets, each by its own class template (the pet lane's
entity/pettemplate.go), fitted against SOE's roamer from the state tools/retune_balance.py leaves.

With the pets on their templates (balancesim against a roamer, eqoaclassic's data of 07:10 with the roamers and
the re-fit in it, the pet's own low point: the fewest hit points it had in a fight won, as a pet that heals ends
higher than it was taken down to) three things were out of line: an enchanter's animation, with 50% more hit points
and 30% less of a melee hit landing on top of its four self heals, kept 95-98% of its hit points through every
yellow and won every red alone, at any pet_hp_pct (the fit's floor of 20 left it at 50-84%); the necromancer's
weapon pets hit 20% harder than their base pets on top of Sandstorm's recorded rounds, and won reds alone at 25-40;
the spectre's instant landed three of its melee hits in one blow, over twice a shadow knight's Harm Touch at 44, where
the user had asked for "the harm touch like move, not as much damage but sizable"; and the magician's mastery pets
(Summon Badi, Efreet and Djinn at 49, Badi Elder, Efreeti Lord and Ghilan Noble at 55), on their kinds' templates,
kept 65-87% through a yellow and won every red alone where the book's Summon Ghilan was near a quarter (the
enchanter's mastery animations too: Gymon's Animation kept 64% and won 79% of its reds).

It writes, each change only while the row holds what the state before it left:

  balanceclasslevel  pet_hp_pct of the magician (10), necromancer (11) and enchanter (12) by level, PETS: fitted at
                     every level 1-60 on the pet the book gives (the highest summoning of its level; the weapon
                     pet from 25), its low point averaged over one and two up near 25%, one up won 80%, two up 65%;
                     levels 1-11 each a row of their own, above them the fewest within 7% of every level. A level
                     the re-fit has no row at gets one, its other columns the re-fit's line there (no column but the
                     pet's moves). The enchanter's now 19-50 above 7, the plain animation's heals its tankiness.
  balancelevel       pet_special_hits 3 -> 1.5 on every row: the spectre's instant one and a half of its hits (about
                     2x one, under a Harm Touch at 44-60).
  classtemplates     once the server has made the pets' templates (pettemplate, by a server of 2026-10-30 08:00 on):
                     the weapon pets' melee_pct 120 -> 100; the animation's hp_pct 150 -> 100 and taken_pct 70 ->
                     100; nine mastery templates, each its kind's with fewer hit points (MASTERY: the magician's
                     Badi, Efreet, Djinn, Badi Elder, Efreeti Lord and Ghilan Noble, the enchanter's Gymon's
                     Animation, Earthen and Ferrous Golem); and pettemplate's rows for those nine summonings. A database the server has not opened since has no
                     pettemplate: the server makes it at its next start out of its seeds, which carry the same.

Every change is logged in balance_pets_log (the table, the row's key, the whole row before and after: NULL for one
added, why); --undo puts back each row still as this left it, takes out the rows it added, and drops the log; a
second --apply does only what is left. Both copy the database first (<db>.bak-<when>-pre-balancepets) unless
--no-backup. Report only by default. tools/retune_balance.py first: before it, this does nothing.

The server reads the balance tables at its start and at a Balance page's save in the GM panel, and the templates at
its start, a template edit or reload_npcs; a pet takes its numbers at its next summoning.

    tools/tune_pets.py [--db eqoa.db] [--apply | --undo] [--no-backup]
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
import retune_balance as retune  # noqa: E402  the state this goes from

DEFAULT_DB = os.path.join(HERE, "..", "eqoa.db")
LOG = "balance_pets_log"
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,   -- balanceclasslevel | balancelevel | classtemplates | pettemplate
    place   TEXT NOT NULL,   -- the row's key, JSON
    before  TEXT,            -- the whole row before, JSON; NULL: added
    after   TEXT,            -- the whole row after, JSON
    why     TEXT NOT NULL,
    applied TEXT NOT NULL
)"""
WHY = "the pets' fit of 2026-10-30 (tools/tune_pets.py; claude-notes/balance-20261029.md)"
SOURCE = "seed"  # the mastery templates are the server's seeds too (database.SeedPetClassTemplates)

# balanceclasslevel: class -> {level: (melee_taken_pct, pet_hp_pct, dealt_pct)} for the pet classes (see the doc)
PETS = {
    10: {1: (15, 25, 400), 2: (17, 50, 400), 3: (16, 80, 240), 4: (19, 60, 144), 5: (18, 84, 100), 6: (27, 113, 100), 7: (33, 150, 100), 8: (29, 95, 100), 9: (39, 113, 100), 10: (24, 126, 100), 11: (100, 150, 100), 12: (100, 106, 100), 15: (100, 159, 100), 16: (100, 53, 100), 17: (100, 63, 100), 19: (100, 126, 100), 20: (100, 90, 100), 23: (100, 134, 100), 24: (100, 71, 100), 25: (100, 84, 100), 28: (100, 125, 100), 29: (100, 138, 100), 32: (100, 179, 100), 33: (100, 225, 100), 34: (100, 56, 100), 38: (100, 100, 100), 39: (100, 80, 100), 43: (100, 113, 100), 44: (100, 63, 100), 45: (100, 106, 100), 46: (100, 113, 100), 47: (100, 84, 100), 52: (100, 119, 100), 60: (100, 174, 100)},
    11: {1: (18, 68, 300), 2: (15, 113, 300), 3: (22, 179, 180), 4: (17, 71, 108), 5: (18, 100, 100), 6: (28, 126, 100), 7: (32, 164, 100), 8: (32, 113, 100), 9: (40, 134, 100), 10: (17, 150, 100), 11: (100, 179, 100), 12: (100, 113, 100), 15: (100, 164, 100), 16: (100, 106, 100), 17: (100, 126, 100), 19: (100, 225, 100), 20: (100, 84, 100), 23: (100, 126, 100), 24: (100, 67, 100), 25: (100, 67, 100), 26: (100, 80, 100), 28: (100, 106, 100), 29: (100, 80, 100), 30: (100, 80, 100), 31: (100, 91, 100), 33: (100, 113, 100), 34: (100, 80, 100), 35: (100, 71, 100), 38: (100, 90, 100), 39: (100, 80, 100), 40: (100, 71, 100), 43: (100, 90, 100), 44: (100, 80, 100), 48: (100, 106, 100), 49: (100, 77, 100), 50: (100, 75, 100), 54: (100, 90, 100), 55: (100, 67, 100), 59: (100, 90, 100), 60: (100, 75, 100)},
    12: {1: (32, 25, 400), 2: (15, 34, 400), 3: (17, 46, 360), 4: (22, 56, 216), 5: (17, 90, 130), 6: (26, 115, 100), 7: (31, 148, 100), 8: (24, 21, 100), 9: (28, 24, 100), 10: (16, 25, 115), 11: (100, 28, 100), 15: (100, 43, 100), 16: (100, 19, 100), 17: (100, 21, 100), 18: (100, 30, 100), 19: (100, 33, 100), 23: (100, 43, 100), 24: (100, 23, 100), 32: (100, 38, 100), 33: (100, 50, 100), 34: (100, 28, 100), 43: (100, 38, 100), 44: (100, 25, 100), 48: (100, 28, 100), 49: (100, 23, 100), 54: (100, 27, 100), 55: (100, 23, 100), 59: (100, 25, 100), 60: (100, 22, 100)},
}

SPECIAL_HITS = (3.0, 1.5)  # balancelevel.pet_special_hits: the pet lane's default, the fit's
TEMPLATE_CHANGES = {  # classtemplates: name -> {column: (the seed's, the fit's)}
    "pet necromancer weapon": {"melee_pct": (120, 100)},
    "pet animation": {"hp_pct": (150, 100), "taken_pct": (70, 100)},
}
# the mastery pets (the magician's and the enchanter's): their own template, its kind's with fewer hit points, and
# their summoning's row
MASTERY = (  # (template, its kind's, hp_pct, summoning)
    ("pet badi", "pet earth", 81, "summon badi"),
    ("pet efreet", "pet fire", 65, "summon efreet"),
    ("pet djinn", "pet air", 67, "summon djinn"),
    ("pet badi elder", "pet earth", 61, "summon badi elder"),
    ("pet efreeti lord", "pet fire", 47, "summon efreeti lord"),
    ("pet ghilan noble", "pet water", 64, "summon ghilan noble"),
    ("pet gymon's animation", "pet animation", 70, "gymon's animation"),
    ("pet earthen golem", "pet animation", 72, "earthen golem"),
    ("pet ferrous golem", "pet animation", 80, "ferrous golem"),
)


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
    return None if r is None else (r["melee_taken_pct"], r["pet_hp_pct"], r.get("dealt_pct", 100.0))


def plan(db):
    """The changes [(table, key, row before or None, row after)] and the rows left, with why."""
    changes, kept = [], []
    tbl = retune.TBL
    if not has_table(db, tbl) or "dealt_pct" not in columns(db, tbl):
        return changes, [(tbl, "no dealt_pct: tools/retune_balance.py first")]
    if not any(cl_values(row_of(db, tbl, {"class": c, "level": l})) == tuple(map(float, v))
               for c in PETS for l, v in retune.RETUNED.get(c, {}).items()):
        return changes, [(tbl, "none of the pet classes' rows is the re-fit's: tools/retune_balance.py first")]
    for cls, want in sorted(PETS.items()):
        was = retune.RETUNED.get(cls, {})
        for level in sorted(set(was) | set(want)):
            old = tuple(map(float, was[level])) if level in was else None
            new = tuple(map(float, want[level])) if level in want else None
            if old == new or new is None:
                continue
            key = {"class": cls, "level": level}
            r = row_of(db, tbl, key)
            have = cl_values(r)
            if have == new:
                continue  # done already
            if have != old:
                kept.append((f"{tbl} {cls} {level}", f"{have if have else 'no row'}, not the re-fit's "
                                                     f"{old if old else 'no row'}: a GM's"))
                continue
            after = dict(r) if r else {"class": cls, "level": level, "note": "", "source": "seed"}
            after.update({"melee_taken_pct": new[0], "pet_hp_pct": new[1], "dealt_pct": new[2]})
            changes.append((tbl, key, r, after))
    if has_table(db, "balancelevel") and "pet_special_hits" in columns(db, "balancelevel"):
        for (level,) in db.execute("SELECT level FROM balancelevel ORDER BY level").fetchall():
            r = row_of(db, "balancelevel", {"level": level})
            if r["pet_special_hits"] == SPECIAL_HITS[1]:
                continue
            if r["pet_special_hits"] != SPECIAL_HITS[0]:
                kept.append((f"balancelevel {level}", f"pet_special_hits {r['pet_special_hits']:g}, not "
                                                      f"{SPECIAL_HITS[0]:g}: a GM's"))
                continue
            changes.append(("balancelevel", {"level": level}, r, dict(r, pet_special_hits=SPECIAL_HITS[1])))
    if not has_table(db, "pettemplate"):
        kept.append(("classtemplates, pettemplate", "no pettemplate yet: the server makes the pets' templates at its "
                                                     "next start out of its seeds, which carry this fit"))
        return changes, kept
    for name, cols in TEMPLATE_CHANGES.items():
        r = row_of(db, "classtemplates", {"name": name})
        if r is None:
            kept.append((f"classtemplates {name!r}", "no such row: a GM's delete"))
            continue
        if all(r[c] == new for c, (old, new) in cols.items()):
            continue
        if any(r[c] != old for c, (old, new) in cols.items()):
            kept.append((f"classtemplates {name!r}", "not the seed's numbers: a GM's"))
            continue
        changes.append(("classtemplates", {"name": name}, r, dict(r, **{c: new for c, (old, new) in cols.items()})))
    for name, kind, hp, spell in MASTERY:
        if row_of(db, "classtemplates", {"name": name}) is None:
            base = row_of(db, "classtemplates", {"name": kind})
            if base is None:
                kept.append((f"classtemplates {name!r}", f"no {kind!r} to copy: a GM's delete"))
                continue
            for c, (old, new) in TEMPLATE_CHANGES.get(kind, {}).items():
                if base[c] == old:
                    base[c] = new  # its kind as this leaves it (the animation's armour off before it is copied)
            note = (f"A mastery pet, {spell.title()}: {kind}'s template with {hp}% hit points where its "
                    f"kind's has {base['hp_pct']}% (the balance's pet fit of 2026-10-30; tools/tune_pets.py)")
            changes.append(("classtemplates", {"name": name}, None, dict(base, name=name, hp_pct=hp, note=note, source=SOURCE)))
        key = {"spell": spell, "weapon": 0}
        r = row_of(db, "pettemplate", key)
        if r is None or r["template"] == name:
            continue
        if r["template"] != kind:
            kept.append((f"pettemplate {spell!r}", f"template {r['template']!r}, not {kind!r}: a GM's"))
            continue
        changes.append(("pettemplate", key, r, dict(r, template=name)))
    return changes, kept


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst, n = f"{path}.bak-{when}-pre-balancepets", 1
    while os.path.exists(dst):
        n += 1
        dst = f"{path}.bak-{when}-{n}-pre-balancepets"
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
                        json.dumps(after, sort_keys=True), WHY, when))
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
            if tbl not in ("balanceclasslevel", "balancelevel", "classtemplates", "pettemplate"):
                raise SystemExit(f"{LOG} names table {tbl!r}, which this tool never writes")
            key = json.loads(place)
            now = row_of(db, tbl, key)
            if now != json.loads(after):
                print(f"  {tbl} {place}: changed since: left as it is")
                continue
            b = None if before is None else json.loads(before)
            write_row(db, tbl, key, b, insert=False)
            back += 1
        retune.reorder(db)
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"put back {back} of {len(logged)} rows; the server takes them at its next start")


def describe(change):
    tbl, key, before, after = change
    k = " ".join(str(v) for v in key.values())
    if before is None:
        shown = {c: after[c] for c in after if c in ("melee_taken_pct", "pet_hp_pct", "dealt_pct", "hp_pct", "template")}
        return f"{tbl} {k}: added {shown}"
    diff = {c: (before[c], after[c]) for c in after if before.get(c) != after[c]}
    return f"{tbl} {k}: " + ", ".join(f"{c} {a:g} -> {b:g}" if isinstance(a, (int, float)) and isinstance(b, (int, float))
                                      else f"{c} {a!r} -> {b!r}" for c, (a, b) in diff.items())


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the fit (logged)")
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
    print(f"changed {len(changes)} rows: the server takes them at its next start (a pet at its next summoning)")


if __name__ == "__main__":
    main()
