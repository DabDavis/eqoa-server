#!/usr/bin/env python3
"""Spell records, class links and lines put right where a public source says
ours are wrong (jib's review, F3 pass 19).

The sources, named in every entry below:

  - Allakhazam's EQOA spell database (eqoa.allakhazam.com, its import of
    2004-04-28) as the Wayback Machine archived it in 2006: a class's list
    (spell.html?scl=<n>) and a page a spell (spell.html?eqoaspell=<id>, "AK
    <id>" below) with its class and level, cast time, recast, power cost,
    scope ("Single Target", "Self Only", "Group"), duration and effect.
  - EQOA-Data's class lists (external/EQOA-Data, Information/databases/
    spells/<class>.json): the lines spelleffect holds.
  - Sandstorm's spell vendors as harvests/sandstorm-spells-20261021 recorded
    them (scrolls_by_class.tsv: each scroll's classes).

What it changes:

  1. Records (spellpattern): the cast time, recast and power of records
     tools/import_eqoadata_spells.py made with numbers of its own (another
     rank's, or a kind's): Lay Hands had recast 90 and power 38 and Quick
     Blade 3600 and 0, near enough each other's (their pages: Lay Hands 3600
     and 0, Quick Blade 30 and 38), and the shaman's first three spells
     Return Home's (cast 60, recast 5, power 25). Each now its page's.
  2. Scopes (spellpattern): shaman spells the importer made as self spells
     (scope 0: EQOA-Data gives most of the shaman's no line, and so no target
     word) that their pages say are cast at a target or a group: scope 1 or
     2, and that scope's range and icon frame as the importer gives a record
     of it (30.0, and the frame every record of the scope here has: ours, the
     importer's rule; Sandstorm's records of those scopes mostly have them).
  3. Links (classspells): rows of the old dump that give a class a spell its
     lists never give it (seven warrior spells the magician had, the
     warrior's Call to Arms the druid), and the shaman's and wizard's second
     invisibility, 700067 "Invisibilty": EQOA-Data's spelling, which the
     importer made a record of beside the Invisibility both classes already
     had (509, the spell Sandstorm's "Invisibility (SHA, WIZ)" scroll teaches
     here: scribe.go finds a scroll's spell by its name, so a class whose
     spell were 700067 would be given 509 by its scroll and hold two again).
     509 stays each one's spell. Its numbers are the enchanter's (AK 97: cast
     2, power 25), where Allakhazam gives the shaman and the wizard a record
     of their own (AK 343, AK 30: cast 3, power 15; both 5.5 minutes). No
     book is changed: a character of the class that knows a spell unlinked
     keeps it, and is named.
  4. Durations (spelleffect): lines that name no time, which the server
     makes last 30 minutes (spellnotes.go's buffDuration), where the spell's
     page gives it another: that time added at the line's end, as the lists
     write one ("30sec", "20min").

A row is changed only while it holds what this expects: a record whose
numbers or scope are no longer the ones named, a link from another source, a
line not the one named, are left as they are and said. So is a row a GM has
changed (gm_audit), and a record another class shares (its other class's
spell would change too). Every change is logged in spell_records_fix_log (the
whole row before and after, and why); --undo puts back each row still as
this left it (one changed since is left, and said) and drops the log. Both
take a copy of the database first (<db>.bak-<when>-pre-recordsfix). Report
only by default.

The server reads a spell's record and line at each cast, and classspells
whenever #template or a grant runs; a client shows a record as its world
entry sent it, so players see a record's change at their next login. Run it
again after an import that writes these rows (tools/import_eqoadata_spells.py,
tools/import_spell_effects.py).

    tools/fix_spell_records.py [--db eqoa.db] [--apply | --undo]
"""
import argparse
import datetime
import json
import os
import re
import shutil
import sqlite3
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")

CLASSES = "WAR RAN PAL SK MNK BRD RGE DRD SHA CL MAG NEC ENC WIZ ALC".split()
PAL, MNK, RGE, DRD, SHA, CLR, MAG, NEC, WIZ, ALC = 2, 4, 6, 7, 8, 9, 10, 11, 13, 14
TAG = "records19"

LOG_SCHEMA = """CREATE TABLE IF NOT EXISTS spell_records_fix_log (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,   -- spellpattern | classspells | spelleffect
    rowkey  TEXT NOT NULL,   -- the row's key, JSON, as gm_audit keys it
    before  TEXT NOT NULL,   -- the whole row before, JSON
    after   TEXT,            -- the whole row after, JSON; NULL: deleted
    why     TEXT NOT NULL,
    applied TEXT NOT NULL
)"""

# The importer's self record (tools/import_eqoadata_spells.py SCOPE_RANGE and
# SCOPE_FRAME) and the range and frame it gives a record of each other scope.
SELF_RANGE, SELF_FRAME = 16528, -1809115846
SCOPE_RANGE = {1: 16880, 2: 16880}
SCOPE_FRAME = {1: 486870879, 2: 593297002}

# 1. Records, by (class, name, level): ((cast, recast, power) now, the page's,
# the page). The shaman's three had Return Home's (AK 1541: 60, 5, 25).
RECORDS = {
    (PAL, "lay hands", 5): ((0, 90, 38), (0, 3600, 0), 152),
    (RGE, "quick blade", 5): ((0, 3600, 0), (0, 30, 38), 249),
    (SHA, "blighting aura", 1): ((60, 5, 25), (2, 3, 12), 568),
    (SHA, "inner fire", 1): ((60, 5, 25), (3, 2, 10), 567),
    (SHA, "sleet", 1): ((60, 5, 25), (2, 3, 7), 342),
}

# 2. The shaman's scopes, by (name, level): (the page's scope, the page): 49
# "Single Target", 9 "Group". Two the importer made scope 0 have no page in
# the archive (Spiritual Renewal, AK 3663; Frenzied Spirit, AK 536) and keep
# it; Invisibilty's is 3 below instead.
SCOPES = {
    ("blighting aura", 1): (1, 568), ("inner fire", 1): (1, 567), ("sleet", 1): (1, 342),
    ("tribal toughness", 3): (1, 143), ("shock of spirits", 4): (1, 147), ("stumbling fever", 4): (1, 331),
    ("blighting mist", 5): (1, 142), ("minor malady", 7): (1, 246), ("corrosive spray", 8): (1, 149),
    ("finesse", 8): (1, 148), ("stinging sleet", 12): (1, 328), ("tribal brawn", 13): (1, 330),
    ("endure poison", 15): (1, 1438), ("lesser malady", 15): (1, 337), ("deftness", 16): (1, 333),
    ("group deftness", 16): (2, 3673), ("spirit slam", 16): (1, 340), ("corrosive jet", 20): (1, 341),
    ("decaying wind", 20): (1, 1598), ("group tribal strength", 23): (2, 3680),
    ("tribal strength", 23): (1, 1645), ("accuracy", 24): (1, 543), ("group accuracy", 24): (2, 3674),
    ("spirit strike", 24): (1, 537), ("affliction", 27): (1, 3666), ("malady", 27): (1, 1648),
    ("venom cloud", 29): (1, 540), ("sickening wind", 30): (1, 1686), ("group tribal force", 33): (2, 3678),
    ("tribal force", 33): (1, 1646), ("group precision", 34): (2, 3675), ("precision", 34): (1, 544),
    ("spirit blast", 34): (1, 538), ("greater affliction", 37): (1, 3665), ("greater malady", 37): (1, 1649),
    ("gale of poison", 39): (1, 541), ("wilting wind", 40): (1, 1687), ("group tribal might", 43): (2, 3679),
    ("tribal might", 43): (1, 1647), ("exactness", 44): (1, 545), ("group exactness", 44): (2, 3676),
    ("shrieking spirits", 44): (1, 539), ("ancient affliction", 47): (1, 3664),
    ("ancient malady", 47): (1, 1650), ("wailing spirits", 48): (1, 1656), ("atrophic wind", 49): (1, 1689),
    ("venom bolt", 49): (1, 542), ("withering wind", 49): (1, 1688), ("avatar", 51): (1, 3167),
    ("group tribal thew", 51): (2, 3681), ("tribal thew", 51): (1, 3071), ("spirit's boon", 54): (1, 3072),
    ("acumen", 55): (1, 2749), ("group acumen", 55): (2, 3677), ("howling spirits", 55): (1, 2751),
    ("primal affliction", 57): (1, 3667), ("primal malady", 57): (1, 3073), ("acidic storm", 60): (1, 4244),
}

# 3. Links, by (class, name, level): (the row's source now, why, the spell
# (name, level) the class must keep a link to, or None). The dump's: each
# spell is on Allakhazam's and EQOA-Data's lists of the classes named, and on
# no other's, and Sandstorm sells its scroll to those classes alone.
WARRIORS = "the warrior's (AK {}), on no magician's list; Sandstorm's scroll is the warrior's"
TAUNT = ("the warrior's, paladin's and shadow knight's (AK {}), on no magician's list; Sandstorm's scroll is "
         "theirs")
LINKS = {
    (MAG, "fortitude", 1): ("dump", WARRIORS.format(686), None),
    (MAG, "tough skin", 4): ("dump", WARRIORS.format(687), None),
    (MAG, "critical assault", 29): ("dump", WARRIORS.format(681), None),
    (MAG, "iron resolve", 29): ("dump", WARRIORS.format(684), None),
    (MAG, "bait", 34): ("dump", TAUNT.format("675, 753, 757"), None),
    (MAG, "incite", 44): ("dump", TAUNT.format("676, 754, 758"), None),
    (MAG, "focused attention", 49): ("dump", WARRIORS.format(679), None),
    (DRD, "call to arms", 16): ("dump", "the warrior's (AK 358), on no druid's list; Sandstorm's scroll is the "
                                "warrior's", None),
    # the second invisibility (AK 343 the shaman's, AK 30 the wizard's, both
    # spelt so): 509, Invisibility, stays each class's
    (SHA, "invisibilty", 8): ("eqoa-data", "a second record of Invisibility, which the class keeps (509: the "
                              "spell its vendors' scroll teaches)", ("invisibility", 8)),
    (WIZ, "invisibilty", 8): ("eqoa-data", "a second record of Invisibility, which the class keeps (509: the "
                              "spell its vendors' scroll teaches)", ("invisibility", 8)),
}

# 4. Durations, by (class, name, level): (the line now, the time to add, the
# page). Each page's effect is its line's, numbers and all: the time is all
# that is added. Of the other lines that take the 30 minutes, 27 are 30
# minutes on their pages too (the cleric's wards and hit point lines, the
# alchemist's AGI/INT and DEX/WIS lines, the groups' too); Group Intent,
# Applied Intent and Group Cunning (AK 3142, 2460, 4142) were never archived,
# and Petrifying Agent is on no Allakhazam list: they keep the 30 minutes.
DURATIONS = {
    (PAL, "fortify", 20): ("Target, Increase AC by 300, Decrease AC by 150 to Self", "12sec", 1590),
    (PAL, "secure", 30): ("Target, Increase AC by 400, Decrease AC to self by 200", "12sec", 1756),
    (MNK, "striking adder", 12): ("Self, Increase DEX by 10", "5min", 206),
    (RGE, "acrobatics", 7): ("Self, Increase AGI by 10 ; Increase 10% based on AGI", "30sec", 190),
    (RGE, "vaulter's balance", 15): ("Self, Increase AGI by 20 ; Increase 10% based on AGI", "30sec", 1444),
    (RGE, "performer's finesse", 23): ("Self, Increase AGI by 30 ; Increase 10% based on AGI", "30sec", 2085),
    (RGE, "aerialist's form", 33): ("Self, Increase AGI by 40 ; Increase 10% based on AGI", "30sec", 1880),
    (RGE, "trapezist's poise", 43): ("Self, Increase AGI by 50 ; Increase 10% based on AGI", "30sec", 2086),
    (RGE, "gymnast's acuity", 51): ("Self, Increase AGI by 60 ; Increase 10% based on AGI", "30sec", 3093),
    (NEC, "flesh mastery", 23): ("Pet, Increase DEX & Str by 20", "5min", 114),
    (NEC, "maliceweave", 25): ("Self, Increase AC by 110", "20min", 1638),
    (NEC, "hateweave", 45): ("Self, Increase AC by 210", "20min", 1640),
    (NEC, "ebonweave", 54): ("Self, Increase AC by 320", "20min", 3062),
    (WIZ, "element guard", 13): ("Target, Increase Resist Fire & Ice by 45", "20min", 444),
    (WIZ, "mental focus", 20): ("Self, Increase Maximum Power by 110 and Increase Power by 110", "20min", 1574),
    (WIZ, "mental surge", 30): ("Self, Buff MaxPower by 210 and Power by 210", "20min", 1682),
    (WIZ, "mental boost", 40): ("Self, Increase Maximum Power by 310 and Increase Power by 310", "20min", 1683),
    (WIZ, "element aegis", 48): ("Self, Increase Resist Fire by 120 and Increase CR by 120", "20min", 445),
    (WIZ, "heightened mind", 49): ("Self, Increase Maximum Power by 390 and Increase Power by 390", "20min", 1684),
    (WIZ, "towering mind", 49): ("Self, Increase Maximum Power by 450 and Increase Power by 450", "20min", 1685),
    (ALC, "minor tonic", 3): ("SELF Buff AR by 15", "10min", 2227),
    (ALC, "lesser tonic", 12): ("SELF Buff Resist Arcane by 30", "10min", 2247),
    (ALC, "student's knack", 20): ("SELF Buff Max HP by 200 and Power per tic by 20", "5min", 2474),
    (ALC, "teacher's knack", 30): ("SELF Buff Max HP by 300 and Increase Power per tic by 30", "5min", 2475),
    (ALC, "professor's knack", 40): ("SELF Buff Max HP by 400 and Power per tic by 40", "5min", 2476),
    (ALC, "dean's knack", 49): ("SELF Buff Power per tic by 50 and Max HP by 500", "5min", 2477),
    (ALC, "inventor's knack", 49): ("SELF Buff Power per tic by 60 and Max HP by 600", "5min", 2478),
}


def spell_name(name):
    """A spell's name as the server compares it (scribe.go's spellKey)."""
    n = (name or "").strip()
    if n.endswith("$"):
        n = n[:-1].rstrip()
    n = re.sub(r"\s*\((?:[A-Za-z]{2,3}(?:\s*[,/]\s*)?)+\)\s*$", "", n)
    return n.replace("+", "").replace("`", "'").strip().lower()


def key_text(key):
    """A key as gm_audit writes it (Go's json.Marshal of a map: sorted, no
    spaces)."""
    return json.dumps(key, sort_keys=True, separators=(",", ":"))


def gm_changed(db):
    """The rows a GM has changed: {(table, key text)}."""
    try:
        rows = db.execute("SELECT tbl, rowkey FROM gm_audit").fetchall()
    except sqlite3.OperationalError:
        return set()
    out = set()
    for tbl, k in rows:
        try:
            out.add((tbl, key_text(json.loads(k))))
        except (ValueError, TypeError):
            continue
    return out


def row_of(db, tbl, key):
    where = " AND ".join(f"{c} = ?" for c in key)
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", list(key.values()))
    r = cur.fetchone()
    if r is None:
        return None
    return dict(zip([d[0] for d in cur.description], r))


def links(db):
    """{spellid: {class: source}} and {(class, name, level): [spellid]}."""
    by_spell, by_name = {}, {}
    for cls, sid, source, name, lvl in db.execute("""
            SELECT cs.class, cs.spellid, cs.source, sp.spellname, sp.abilitylvl
            FROM classspells cs JOIN spellpattern sp ON sp.spellid = cs.spellid
            ORDER BY cs.spellid, cs.class"""):
        by_spell.setdefault(sid, {})[cls] = source
        by_name.setdefault((cls, spell_name(name), lvl), []).append(sid)
    return by_spell, by_name


def holders(db, cls, sid):
    """The characters of a class whose book holds a spell."""
    return [f"{n} ({s})" for s, n in db.execute(
        "SELECT c.serverid, c.charName FROM spells b JOIN characters c ON c.serverid = b.serverid "
        "WHERE b.spellid = ? AND c.tclass = ? ORDER BY c.serverid", (sid, cls))]


def plan(db):
    """The changes [(table, key, row before, row after or None, why)] and
    the rows left as they are, with why [(what, why)]."""
    gm = gm_changed(db)
    by_spell, by_name = links(db)
    changes, kept, notes = [], [], []
    pattern = {}  # spellid -> (before, after, [why])

    def one(cls, name, lvl, what):
        ids = by_name.get((cls, name, lvl), [])
        if not ids:
            kept.append((what, f"no {CLASSES[cls]} spell of that name and level here"))
            return None
        if len(ids) > 1:
            kept.append((what, f"{len(ids)} {CLASSES[cls]} spells of that name and level ({ids})"))
            return None
        sid = ids[0]
        others = sorted(set(by_spell.get(sid, {})) - {cls})
        if others:
            kept.append((what, f"spell {sid} is also {', '.join(CLASSES[c] for c in others)}'s"))
            return None
        return sid

    def record(sid, what):
        if ("spellpattern", key_text({"spellid": sid})) in gm:
            kept.append((what, f"a GM changed record {sid}"))
            return None
        if sid not in pattern:
            r = row_of(db, "spellpattern", {"spellid": sid})
            pattern[sid] = (r, dict(r), [])
        return pattern[sid]

    for (cls, name, lvl), (now, want, page) in sorted(RECORDS.items()):
        what = f"{CLASSES[cls]} L{lvl} {name}: cast, recast, power"
        sid = one(cls, name, lvl, what)
        p = sid and record(sid, what)
        if not p:
            continue
        have = (p[1]["casttime"], p[1]["recast"], p[1]["power"])
        if have == want:
            continue
        if have != now:
            kept.append((what, f"record {sid} has {have}, not {now}"))
            continue
        p[1]["casttime"], p[1]["recast"], p[1]["power"] = want
        p[2].append(f"cast, recast, power {now} -> {want}, AK {page}")

    for (name, lvl), (scope, page) in sorted(SCOPES.items()):
        what = f"SHA L{lvl} {name}: scope"
        sid = one(SHA, name, lvl, what)
        p = sid and record(sid, what)
        if not p:
            continue
        r = p[1]
        if r["scope"] == scope:
            continue
        if r["scope"] != 0:
            kept.append((what, f"record {sid} has scope {r['scope']}, not 0"))
            continue
        line = db.execute("SELECT line FROM spelleffect WHERE spellid = ?", (sid,)).fetchone()
        word = (line[0] if line else "").strip().lower()
        if word and not word.startswith({1: "target", 2: "group"}[scope]):
            kept.append((what, f"its line says otherwise ({line[0]!r})"))
            continue
        r["scope"] = scope
        why = f"scope 0 -> {scope}, AK {page}"
        if (r["range"], r["iconColor"]) == (SELF_RANGE, SELF_FRAME):
            r["range"], r["iconColor"] = SCOPE_RANGE[scope], SCOPE_FRAME[scope]
            why += " (the scope's range and frame: ours)"
        p[2].append(why)

    for sid, (before, after, why) in sorted(pattern.items()):
        if why and after != before:
            changes.append(("spellpattern", {"spellid": sid}, before, after, "; ".join(why)))

    for (cls, name, lvl), (source, why, keep) in sorted(LINKS.items()):
        what = f"{CLASSES[cls]} L{lvl} {name}: link"
        if keep and not by_name.get((cls,) + keep):
            kept.append((what, f"{CLASSES[cls]} has no {keep[0].title()} of level {keep[1]} to keep"))
            continue
        for sid in by_name.get((cls, name, lvl), []):
            key = {"class": cls, "spellid": sid}
            row = row_of(db, "classspells", key)
            if ("classspells", key_text(key)) in gm:
                kept.append((what, f"a GM changed the link to {sid}"))
                continue
            if row["source"] != source:
                kept.append((what, f"the link to {sid} is {row['source']!r}'s, not {source!r}'s"))
                continue
            # its rowid too, so --undo puts it back where it was
            (rowid,) = db.execute("SELECT rowid FROM classspells WHERE class = ? AND spellid = ?", (cls, sid)).fetchone()
            changes.append(("classspells", key, dict(row, _rowid=rowid), None, why))
            for h in holders(db, cls, sid):
                notes.append(f"{CLASSES[cls]} {h} knows {name.title()} ({sid}): its book is left as it is")

    for (cls, name, lvl), (line, time, page) in sorted(DURATIONS.items()):
        what = f"{CLASSES[cls]} L{lvl} {name}: duration"
        sid = one(cls, name, lvl, what)
        if not sid:
            continue
        key = {"spellid": sid}
        row = row_of(db, "spelleffect", key)
        want = f"{line}, {time}"
        if row is None:
            kept.append((what, f"spell {sid} has no line"))
            continue
        if row["line"] == want:
            continue
        if ("spelleffect", key_text(key)) in gm:
            kept.append((what, f"a GM changed the row of {sid} (gm_audit)"))
            continue
        if row["line"] != line:
            kept.append((what, f"its line is not the one expected ({row['line']!r})"))
            continue
        after = dict(row, line=want, source=f"{row['source']}; {time} as AK {page} ({TAG})")
        changes.append(("spelleffect", key, row, after, f"{time}, AK {page}"))
    return changes, kept, notes


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-recordsfix"
    shutil.copy2(path, dst)
    return dst


def write_row(db, tbl, key, row):
    """Updates a row to row's values, or deletes it (row None)."""
    where = " AND ".join(f"{c} = ?" for c in key)
    if row is None:
        db.execute(f"DELETE FROM {tbl} WHERE {where}", list(key.values()))
        return
    cols = [c for c in row if c not in key]
    db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in cols)} WHERE {where}",
               [row[c] for c in cols] + list(key.values()))


def apply(db, path, changes):
    print(f"backup: {backup(path)}")
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_SCHEMA)
        for tbl, key, before, after, why in changes:
            db.execute("INSERT INTO spell_records_fix_log (tbl, rowkey, before, after, why, applied) VALUES (?, ?, ?, ?, ?, ?)",
                       (tbl, key_text(key), json.dumps(before, sort_keys=True),
                        None if after is None else json.dumps(after, sort_keys=True), why, when))
            write_row(db, tbl, key, after)
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"changed {len(changes)} rows")


def undo(db, path):
    try:
        logged = db.execute("SELECT seq, tbl, rowkey, before, after FROM spell_records_fix_log ORDER BY seq DESC").fetchall()
    except sqlite3.OperationalError:
        logged = []
    if not logged:
        print("nothing logged: nothing to undo")
        return
    print(f"backup: {backup(path)}")
    back = 0
    db.execute("BEGIN IMMEDIATE")
    try:
        for seq, tbl, rowkey, before, after in logged:
            key, before = json.loads(rowkey), json.loads(before)
            now = row_of(db, tbl, key)
            if after is None:
                if now is not None:
                    print(f"  {tbl} {rowkey}: there again since: left as it is")
                    continue
                if row_of(db, "spellpattern", {"spellid": before["spellid"]}) is None:
                    print(f"  {tbl} {rowkey}: its spell is gone: left out")
                    continue
                rowid = before.pop("_rowid", None)
                if rowid is not None and db.execute(f"SELECT 1 FROM {tbl} WHERE rowid = ?", (rowid,)).fetchone():
                    rowid = None  # taken since: anywhere
                cols = list(before) + (["rowid"] if rowid is not None else [])
                vals = list(before.values()) + ([rowid] if rowid is not None else [])
                db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", vals)
            else:
                if now != json.loads(after):
                    print(f"  {tbl} {rowkey}: changed since ({'gone' if now is None else 'not as this left it'}): left as it is")
                    continue
                write_row(db, tbl, key, before)
            back += 1
        db.execute("DROP TABLE spell_records_fix_log")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"put back {back} of {len(logged)} rows")


def describe(change):
    tbl, key, before, after, why = change
    if tbl == "spellpattern":
        diff = ", ".join(f"{c} {before[c]} -> {after[c]}" for c in after if after[c] != before[c])
        return f"{before['spellid']:7d} {before['spellname']}: {diff}\n          ({why})"
    if tbl == "classspells":
        return f"{key['spellid']:7d} unlinked from {CLASSES[key['class']]} (source {before['source']!r}): {why}"
    return f"{key['spellid']:7d} {before['line']!r}\n       -> {after['line']!r} ({why})"


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)

    if a.undo:
        undo(db, a.db)
        return

    changes, kept, notes = plan(db)
    for c in changes:
        print(describe(c))
    for what, why in kept:
        print(f"left as it is: {what} ({why})")
    for n in notes:
        print(n)
    print(f"{len(changes)} rows to change, {len(kept)} left")
    if not a.apply or not changes:
        if not a.apply:
            print("report only: --apply to write")
        return
    apply(db, a.db, changes)


if __name__ == "__main__":
    main()
