#!/usr/bin/env python3
"""Death Sacrament swaps in the weapon versions of the necromancer's pets
(the user, 2026-10-30: "Death Sacrament swapping in the weapon versions of
the necromancer pets").

The mastery (cmcatalog's Death Sacrament, ranks 1-8, levels 25-60, "Pet
weapons (Increases melee damage)") takes a pet of the necromancer's out of
the book and puts its version "with weapons" / "with a weapon" in. EQOA-Data's
mastery list (cms/necromancer.json) names each rank's abilities by name and
level, "removeAbilities: Howling Death 44, addAbilities: Howling Death 45";
the game's own records hold two spells of each name, the one summoning the
pet "with weapons" or "with a weapon" (spellpattern's description), a level
on or none (Howling Death's two are both 44), at the rank's own level; and
Sandstorm's necromancer of 51 (lane-spellfx24's Lagerbottoms) held those six
alone, 918 932 939 950 956 973, and none of the six it replaces.
tools/seed_masteries.py found the abilities by name, the base spell each
time, so every rank added and removed the one spell, which a purchase leaves
in the book (entity/cmbuy.go BookChange): the ranks swapped nothing.

Each rank is made to (the weapon version found as the server finds one,
database/pettemplates.go PetWeaponSpells: a spell of the base's name whose
description says "with weapons" or "with a weapon", in the necromancer's
classspells):

  1 (25) Decaying Flesh 914 -> 918      4 (40) Preserved Flesh 942 -> 950
  2 (30) Rotting Flesh 925 -> 932       5 (45) Howling Death 955 -> 956
  3 (35) Putrid Flesh 936 -> 939        6 (50) Ancient Death 963 -> 973
  7 (55) EQOA-Data's Bone Swarm in place of Swarming Bones: no Bone Swarm is
         in the game's records here, so the rank takes nothing (its row took
         Swarming Bones and gave nothing) and still shows Bone Swarm, which a
         GM's spell id gives once there is one;
  8 (60) Swarming Death added and removed, one record of it here: left, the
         book keeps it (BookChange), as before.

The characters owning a rank (charcms) are given what the fixed rank gives,
as buying it now would: the base pet out of their book and its weapon version
in (at the book's next display index, off the bar, as a mastery's spell is
added: database/spellbook.go AddSpell); and a rank 7 owner without Swarming
Bones has it back (rank 7 needs it, and its row took it). A character not
owning a rank is not touched.

Every change is logged in death_sacrament_log (a rank's columns before and
after, each book row taken out whole, each one put in by its id, and the
book table's id counter); --undo puts back each one still as this left it
(one changed since is left, and said) and drops the log, the database then
as it was; a second --apply is refused while the log is there. Both take a
copy of the database first (<db>.bak-<when>-pre-deathsac). Report only by
default. The server reads the catalogue at its start (and when a GM edits a
mastery in the panel): run this with the server stopped, or restart it
after; a character's book is read at its login (the owner offline, or it
sees the swap at its next).

    tools/fix_death_sacrament.py [--db eqoa.db] [--apply | --undo]
"""
import argparse
import datetime
import json
import os
import re
import sqlite3
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
MASTERY = "Death Sacrament"
NECROMANCER = 11
WEAPON = re.compile(r"\bwith (a )?weapons?\b", re.I)
BOOK_TOP = 252  # display indices end at 252 (handlers/scribe.go nextDisplayIndex)
COLUMNS = ("adds", "add_spells", "removes", "remove_spells")

LOG_SCHEMA = """CREATE TABLE death_sacrament_log (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    kind     TEXT NOT NULL,
    cmid     INTEGER NOT NULL DEFAULT 0,
    serverid INTEGER NOT NULL DEFAULT 0,
    spellid  INTEGER NOT NULL DEFAULT 0,
    before   TEXT NOT NULL DEFAULT '',
    after    TEXT NOT NULL DEFAULT '',
    applied  TEXT NOT NULL
)"""


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def ids(text):
    out = []
    for f in str(text or "").split(","):
        f = f.strip()
        if f.isdigit():
            out.append(int(f))
    return out


def pattern(db, spellid):
    """(spellid, name, level, desc) of a spell, or None."""
    return db.execute("SELECT spellid, spellname, abilitylvl, spelldesc FROM spellpattern WHERE spellid = ?",
                      (spellid,)).fetchone()


def weapon_of(db, base):
    """The weapon versions of a base spell: of its name, described "with
    weapons" or "with a weapon", the necromancer's (classspells)."""
    sid, name, _, _ = base
    rows = db.execute("SELECT p.spellid, p.spellname, p.abilitylvl, p.spelldesc FROM spellpattern p "
                      "JOIN classspells c ON c.spellid = p.spellid AND c.class = ? "
                      "WHERE lower(trim(p.spellname)) = lower(trim(?)) AND p.spellid != ? ORDER BY p.spellid",
                      (NECROMANCER, name, sid)).fetchall()
    return [r for r in rows if WEAPON.search(r[3] or "")]


def plan(db):
    """(changes, left): each change (cmid, rank, rank name, before {column:
    value}, after {column: value}, the base spell, the weapon version or
    None, why); each rank left (rank name, why)."""
    changes, left = [], []
    ranks = db.execute("SELECT id, rank, name, require_spells, adds, add_spells, removes, remove_spells FROM cmcatalog "
                       "WHERE lower(trim(mastery)) = lower(?) ORDER BY rank", (MASTERY,)).fetchall()
    if not ranks:
        left.append((MASTERY, "no ranks in cmcatalog"))
    for rid, rank, name, req, adds, add_spells, removes, remove_spells in ranks:
        before = dict(zip(COLUMNS, (adds, add_spells, removes, remove_spells)))
        base_ids = ids(req) or ids(remove_spells)
        base = pattern(db, base_ids[0]) if len(base_ids) == 1 else None
        if base is None:
            left.append((name, f"needs one spell, not {req!r}: left"))
            continue
        weapons = weapon_of(db, base)
        if len(weapons) > 1:
            left.append((name, f"{len(weapons)} weapon versions of {base[1]}: {[w[0] for w in weapons]}: left"))
            continue
        if weapons:
            w = weapons[0]
            after = {"adds": f"{w[1]}@{w[2]}", "add_spells": str(w[0]),
                     "removes": f"{base[1]}@{base[2]}", "remove_spells": str(base[0])}
            why = f"{base[1]} {base[0]} (level {base[2]}) out, {w[1]} {w[0]} (level {w[2]}, \"{w[3]}\") in"
            weapon = w
        elif set(ids(add_spells)) == set(ids(remove_spells)):
            left.append((name, f"one {base[1]} in the game's records: added and removed as one, the book keeps it"))
            continue
        else:
            # it takes the base and gives nothing with a spell: it takes nothing
            after = dict(before, removes="", remove_spells="")
            why = f"no {adds or 'spell'} in the game's records: {base[1]} {base[0]} is kept (it took it and gave nothing)"
            weapon = None
        if after == before:
            left.append((name, "already so"))
            continue
        changes.append((rid, rank, name, before, after, base, weapon, why))
    return changes, left


def next_index(db, serverid):
    n = 0
    for (order,) in db.execute("SELECT addedorder FROM spells WHERE serverid = ?", (serverid,)):
        if order < BOOK_TOP and order >= n:
            n = order + 1
    return n


def book_plan(db, changes):
    """Each owner's book changes, as buying its fixed rank now would make
    them: [(serverid, character, rank name, "out"|"in", spellid, spell
    name)]."""
    out = []
    if not has_table(db, "charcms") or not has_table(db, "spells"):
        return out
    for rid, _, name, _, _, base, weapon, _ in changes:
        owners = db.execute("SELECT cc.serverid, coalesce(c.charName, '') FROM charcms cc "
                            "LEFT JOIN characters c ON c.serverid = cc.serverid WHERE cc.cmid = ? ORDER BY cc.serverid",
                            (rid,)).fetchall()
        for sid, who in owners:
            has = {s for (s,) in db.execute("SELECT spellid FROM spells WHERE serverid = ?", (sid,))}
            if weapon is not None:
                if base[0] in has:
                    out.append((sid, who, name, "out", base[0], base[1]))
                if weapon[0] not in has:
                    out.append((sid, who, name, "in", weapon[0], weapon[1]))
            elif base[0] not in has:
                out.append((sid, who, name, "in", base[0], base[1]))
    return out


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-deathsac"
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def report(db):
    changes, left = plan(db)
    for rid, rank, name, before, after, _, _, why in changes:
        print(f"{rid:6d} {name}: {why}")
        for col in COLUMNS:
            if before[col] != after[col]:
                print(f"         {col}: {before[col]!r} -> {after[col]!r}")
    for name, why in left:
        print(f"       {name}: {why}")
    books = book_plan(db, changes)
    for sid, who, name, way, spellid, spell in books:
        print(f"  character {sid} {who} ({name}): {spell} {spellid} {way}")
    print(f"{len(changes)} ranks to fix, {len(left)} left; {len(books)} book changes for "
          f"{len({b[0] for b in books})} owners")
    return changes, books


def apply(db, changes, books):
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_SCHEMA)
        seq = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'spells'").fetchone()
        for rid, _, _, before, after, _, _, _ in changes:
            db.execute("INSERT INTO death_sacrament_log (kind, cmid, before, after, applied) VALUES ('rank', ?, ?, ?, ?)",
                       (rid, json.dumps(before, sort_keys=True), json.dumps(after, sort_keys=True), when))
            db.execute("UPDATE cmcatalog SET adds = ?, add_spells = ?, removes = ?, remove_spells = ? WHERE id = ?",
                       (after["adds"], after["add_spells"], after["removes"], after["remove_spells"], rid))
        for sid, _, _, way, spellid, _ in books:
            if way == "out":
                for row in db.execute("SELECT id, serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide "
                                      "FROM spells WHERE serverid = ? AND spellid = ?", (sid, spellid)).fetchall():
                    db.execute("INSERT INTO death_sacrament_log (kind, serverid, spellid, before, applied) "
                               "VALUES ('out', ?, ?, ?, ?)", (sid, spellid, json.dumps(row), when))
                db.execute("DELETE FROM spells WHERE serverid = ? AND spellid = ?", (sid, spellid))
            else:
                cur = db.execute("INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide) "
                                 "VALUES (?, ?, ?, 0, -1, 0, -1)", (sid, spellid, next_index(db, sid)))
                db.execute("INSERT INTO death_sacrament_log (kind, serverid, spellid, after, applied) VALUES ('in', ?, ?, ?, ?)",
                           (sid, spellid, str(cur.lastrowid), when))
        if seq is not None:
            now = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'spells'").fetchone()
            db.execute("INSERT INTO death_sacrament_log (kind, before, after, applied) VALUES ('seq', ?, ?, ?)",
                       (str(seq[0]), str(now[0]), when))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db, path, keep_copy):
    if not has_table(db, "death_sacrament_log"):
        print("nothing logged: nothing to undo")
        return
    logged = db.execute("SELECT kind, cmid, serverid, spellid, before, after FROM death_sacrament_log ORDER BY id DESC").fetchall()
    if keep_copy:
        print(f"copy: {backup(db, path)}")
    db.execute("BEGIN IMMEDIATE")
    try:
        back, kept = 0, []
        for kind, cmid, sid, spellid, before, after in logged:
            if kind == "rank":
                b, a = json.loads(before), json.loads(after)
                now = db.execute("SELECT adds, add_spells, removes, remove_spells FROM cmcatalog WHERE id = ?",
                                 (cmid,)).fetchone()
                if now is None or dict(zip(COLUMNS, now)) != a:
                    kept.append(f"rank {cmid}: changed since, left")
                    continue
                db.execute("UPDATE cmcatalog SET adds = ?, add_spells = ?, removes = ?, remove_spells = ? WHERE id = ?",
                           (b["adds"], b["add_spells"], b["removes"], b["remove_spells"], cmid))
                back += 1
            elif kind == "in":
                gone = db.execute("DELETE FROM spells WHERE id = ? AND serverid = ? AND spellid = ?",
                                  (int(after), sid, spellid)).rowcount
                if gone:
                    back += 1
                else:
                    kept.append(f"character {sid}'s spell {spellid} (row {after}): gone since, left")
            elif kind == "out":
                row = json.loads(before)
                if db.execute("SELECT count(*) FROM spells WHERE id = ? OR (serverid = ? AND spellid = ?)",
                              (row[0], sid, spellid)).fetchone()[0]:
                    kept.append(f"character {sid}'s spell {spellid}: in the book again since, left")
                    continue
                db.execute("INSERT INTO spells (id, serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide) "
                           "VALUES (?, ?, ?, ?, ?, ?, ?, ?)", row)
                back += 1
            elif kind == "seq":
                now = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'spells'").fetchone()
                if now is not None and str(now[0]) == after:
                    db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = 'spells'", (int(before),))
                else:
                    kept.append("the book's id counter: moved on since, left")
        db.execute("DROP TABLE death_sacrament_log")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"put back {back}; {len(kept)} left")
    for k in kept:
        print(f"  {k}")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--undo", action="store_true")
    ap.add_argument("--no-copy", action="store_true", help="no copy of the database first (tests)")
    args = ap.parse_args()
    if args.apply and args.undo:
        sys.exit("--apply or --undo, not both")
    db = sqlite3.connect(args.db, isolation_level=None)
    if args.undo:
        undo(db, args.db, not args.no_copy)
        return
    if args.apply and has_table(db, "death_sacrament_log"):
        sys.exit("death_sacrament_log is there: --undo first")
    changes, books = report(db)
    if not args.apply:
        print("report only: --apply writes it")
        return
    if not changes and not books:
        print("nothing to write")
        return
    if not args.no_copy:
        print(f"copy: {backup(db, args.db)}")
    apply(db, changes, books)
    print(f"written: {len(changes)} ranks, {len(books)} book changes (logged in death_sacrament_log)")


if __name__ == "__main__":
    main()
