#!/usr/bin/env python3
"""The pets' fit again, for their buffs on themselves (2026-10-30; the user: "the earth pet for eqoaclassic is
casting the ac buff on the player instead of itself, if fire does the same should also be on itself", and, shown what
that did to the balance, "Re-fit to the old targets").

A pet's buff lands on the pet now and counts in its fights (entity/pettemplate.go, pet.go): the earth pets' skin (AC)
and AGI lines take off a creature's hits on them, the fire pets' STR and DEX lines raise their own. On the pets' fit
of the same day (tools/tune_pets.py) that made them stronger (balancesim against the roamer on eqoaclassic's data of
22:25, the pet's low point averaged over one and two up): Summon Badi and Badi Elder kept 77-82% of their hit points
through a yellow and won every red alone (Natureskin's 625 AC at 49), the Earth Elemental 40-47% at 34-45, the fire
pets 22-47%. This puts them back on the old fit's numbers, each where that fit was made:

  classtemplates     hp_pct: the fire pets' 100 -> 91 (the book's Fire Elementaling, Elementalkin and Elemental at
                     8-11, 24-28 and 44: 25.8% against the old 25.9%), Summon Badi's 81 -> 36, Badi Elder's 61 -> 32,
                     Efreet's 65 -> 58, Efreeti Lord's 47 -> 42 (each near a third, as the mastery pets were);
                     the earth and fire templates' notes say where their buffs land, the four mastery templates' their
                     new hit points (the server's seeds word for word: database/pettemplates.go).
  balanceclasslevel  the magician's pet_hp_pct at 34-38, where the old fit was made on the Earth Elemental (the book's
                     pet there): 34 56 -> 41, 35-37 (the line from 56 to 100) -> rows of 44, 46 and 53, 38 100 -> 58,
                     the old fit's numbers level by level (26/20 .. 31/23 then, 24/27 .. 29/23 now). A buffed pet's
                     hits taken are flatter by level, so its template's hit points could not do it (34 stayed near
                     10% whatever they were).

Nothing is done before tools/tune_pets.py (its log, or the magician's rows at 34 and 38 as it left them). Each
change is made only while the row holds what tools/tune_pets.py left; a GM's value is left and said. Every
change is logged in balance_petself_log (the table, the row's key, the whole row before and after: NULL for one
added); --undo puts back each row still as this left it, takes out the rows it added, and drops the log; a second
--apply does only what is left. Both copy the database first (<db>.bak-<when>-pre-petself) unless --no-backup.
Report only by default. Apply it with the server stopped and the binary that puts a pet's buffs on itself starting
after (the server reads these tables at its start); an older binary with these rows has weaker pets.

    tools/fit_pet_selfbuffs.py [--db eqoa.db] [--apply | --undo] [--no-backup]
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
import retune_balance as retune  # noqa: E402  balanceclasslevel's order

DEFAULT_DB = os.path.join(HERE, "..", "eqoa.db")
LOG = "balance_petself_log"
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,   -- balanceclasslevel | classtemplates
    place   TEXT NOT NULL,   -- the row's key, JSON
    before  TEXT,            -- the whole row before, JSON; NULL: added
    after   TEXT,            -- the whole row after, JSON
    why     TEXT NOT NULL,
    applied TEXT NOT NULL
)"""
WHY = "the pets' fit again, their buffs on themselves (tools/fit_pet_selfbuffs.py; 2026-10-30)"
PETS_LOG = "balance_pets_log"  # tools/tune_pets.py's: the state this goes from


def mastery_note(spell, kind, hp, kind_hp):
    """A mastery template's note as database/pettemplates.go's petMasteryTemplates writes it."""
    return (f"A mastery pet, {spell}: {kind}'s template with {hp}% hit points where its kind's has {kind_hp}% "
            "(the balance's pet fit of 2026-10-30: near a third of its hit points through a yellow, no red won alone)")


EARTH_NOTE = (
    "The magician's earth pet: a tank (a warrior's round, four hits from 35; 30% more hit points, 20% less of a melee "
    "hit lands, its own 10% lighter) that taunts every 9 s (a melee hit's damage, the top of the hate) and keeps ",
    "its owner in the druid's skin line (AC) and the ranger's AGI line of its level (ours)",
    "itself in the druid's skin line (AC) and the ranger's AGI line of its level, which take off a creature's hits "
    "on it as a player's AC and AGI do (ours; on its owner until 2026-10-30)")
FIRE_NOTE = (
    "The magician's fire pet: the hardest melee (a warrior's round, 25% harder",
    "), a magician's fire spell of its level (Lava Burst ... Inferno) every 12 s at 25%, and its owner kept in the "
    "shaman's STR (Tribal) and DEX (Finesse) lines (ours)",
    "; 9% fewer hit points since its buffs are its own: the pets' fit again, tools/fit_pet_selfbuffs.py), a "
    "magician's fire spell of its level (Lava Burst ... Inferno) every 12 s at 25%, and itself kept in the shaman's "
    "STR (Tribal) and DEX (Finesse) lines, which raise its own hits as a tank's STR and DEX do (ours; on its owner "
    "until 2026-10-30)")

# classtemplates: name -> (hp_pct before, after; note before, after)
TEMPLATES = {
    "pet earth": (130, 130, EARTH_NOTE[0] + EARTH_NOTE[1], EARTH_NOTE[0] + EARTH_NOTE[2]),
    "pet fire": (100, 91, FIRE_NOTE[0] + FIRE_NOTE[1], FIRE_NOTE[0] + FIRE_NOTE[2]),
}
for name, spell, kind, (old_hp, new_hp), (old_kind, new_kind) in (
        ("pet badi", "Summon Badi (49)", "pet earth", (81, 36), (130, 130)),
        ("pet badi elder", "Summon Badi Elder (55)", "pet earth", (61, 32), (130, 130)),
        ("pet efreet", "Summon Efreet (49)", "pet fire", (65, 58), (100, 91)),
        ("pet efreeti lord", "Summon Efreeti Lord (55)", "pet fire", (47, 42), (100, 91))):
    TEMPLATES[name] = (old_hp, new_hp, mastery_note(spell, kind, old_hp, old_kind), mastery_note(spell, kind, new_hp, new_kind))

# balanceclasslevel: the magician's (class, level) -> (melee_taken_pct, pet_hp_pct, dealt_pct) before (None: no row,
# the line between 34's and 38's), after
MAGICIAN = 10
CLASS_LEVELS = {
    34: ((100.0, 56.0, 100.0), (100.0, 41.0, 100.0)),
    35: (None, (100.0, 44.0, 100.0)),
    36: (None, (100.0, 46.0, 100.0)),
    37: (None, (100.0, 53.0, 100.0)),
    38: ((100.0, 100.0, 100.0), (100.0, 58.0, 100.0)),
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


def cl_values(r):
    return None if r is None else (float(r["melee_taken_pct"]), float(r["pet_hp_pct"]), float(r.get("dealt_pct", 100.0)))


def pets_fitted(db):
    """Whether tools/tune_pets.py's fit is what this goes from: its log, or the magician's rows at 34 and 38 as it
    (or this) left them (a database made new: its seeds carry both fits)."""
    if has_table(db, PETS_LOG):
        return True
    if not has_table(db, retune.TBL) or "dealt_pct" not in columns(db, retune.TBL):
        return False
    ends = tuple(cl_values(row_of(db, retune.TBL, {"class": MAGICIAN, "level": level})) for level in (34, 38))
    return ends in ((CLASS_LEVELS[34][0], CLASS_LEVELS[38][0]), (CLASS_LEVELS[34][1], CLASS_LEVELS[38][1]))


def plan(db):
    """The changes [(table, key, row before or None, row after)] and the rows left, with why."""
    changes, kept = [], []
    tbl = retune.TBL
    if not pets_fitted(db):
        return changes, [("everything", "not the pets' fit's state: tools/tune_pets.py first")]
    if not has_table(db, tbl) or "dealt_pct" not in columns(db, tbl):
        kept.append((tbl, "no dealt_pct: tools/retune_balance.py and tools/tune_pets.py first"))
    else:
        rows = {level: row_of(db, tbl, {"class": MAGICIAN, "level": level}) for level in CLASS_LEVELS}
        now = {level: cl_values(r) for level, r in rows.items()}
        if all(now[level] == new for level, (old, new) in CLASS_LEVELS.items()):
            pass  # done already
        elif any(now[level] != old and now[level] != new for level, (old, new) in CLASS_LEVELS.items()):
            # one level not as the fit left it nor as this makes it: the levels between are a line through the
            # ends, so the five go together or not at all
            odd = ", ".join(f"{level} {now[level] if now[level] else 'no row'}" for level, (old, new)
                            in CLASS_LEVELS.items() if now[level] not in (old, new))
            kept.append((f"{tbl} {MAGICIAN} 34-38", f"{odd}: not the pets' fit's (a GM's): all five left"))
        else:
            for level, (old, new) in CLASS_LEVELS.items():
                if now[level] == new:
                    continue
                r = rows[level]
                after = dict(r) if r else {"class": MAGICIAN, "level": level, "note": "", "source": "seed"}
                after.update({"melee_taken_pct": new[0], "pet_hp_pct": new[1], "dealt_pct": new[2]})
                changes.append((tbl, {"class": MAGICIAN, "level": level}, r, after))
    if not has_table(db, "classtemplates") or not has_table(db, "pettemplate"):
        kept.append(("classtemplates", "no pettemplate yet: the server makes the pets' templates at its next start "
                                       "out of its seeds, which carry this fit"))
        return changes, kept
    for name, (old_hp, new_hp, old_note, new_note) in TEMPLATES.items():
        r = row_of(db, "classtemplates", {"name": name})
        if r is None:
            kept.append((f"classtemplates {name!r}", "no such row: a GM's delete"))
            continue
        after = dict(r)
        if r["hp_pct"] == old_hp:
            after["hp_pct"] = new_hp
        elif r["hp_pct"] != new_hp:
            kept.append((f"classtemplates {name!r} hp_pct", f"{r['hp_pct']}, not the fit's {old_hp}: a GM's"))
        if r["note"] == old_note:
            after["note"] = new_note
        elif r["note"] != new_note:
            kept.append((f"classtemplates {name!r} note", "not the seed's: a GM's"))
        if after != r:
            changes.append(("classtemplates", {"name": name}, r, after))
    return changes, kept


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst, n = f"{path}.bak-{when}-pre-petself", 1
    while os.path.exists(dst):
        n += 1
        dst = f"{path}.bak-{when}-{n}-pre-petself"
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
            if tbl not in ("balanceclasslevel", "classtemplates"):
                raise SystemExit(f"{LOG} names table {tbl!r}, which this tool never writes")
            key = json.loads(place)
            if row_of(db, tbl, key) != json.loads(after):
                print(f"  {tbl} {place}: changed since: left as it is")
                continue
            write_row(db, tbl, key, None if before is None else json.loads(before), insert=False)
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
        shown = {c: after[c] for c in after if c in ("melee_taken_pct", "pet_hp_pct", "dealt_pct")}
        return f"{tbl} {k}: added {shown}"
    diff = {c: (before[c], after[c]) for c in after if before.get(c) != after[c]}
    return f"{tbl} {k}: " + ", ".join(
        f"{c} {a:g} -> {b:g}" if isinstance(a, (int, float)) and isinstance(b, (int, float))
        else f"{c} rewritten" for c, (a, b) in diff.items())


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
