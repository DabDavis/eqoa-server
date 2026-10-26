#!/usr/bin/env python3
"""Four enchanter spell lines in spelleffect, made to say what the spell does.

internal/entity/enchanter.go makes the enchanter's lines work (the procs by
their names, the group's procs on the members' pets, the clarity line one at
a time, the Paralytic line's arcane resist debuff in the creatures' resist
rolls); these are the lines that need their words, by the enchanter's
(class 12) spell of each name:

  - Entrancing Charm: no line at all (the dump record's name; EQOA-Data lists
    the spell as "Entrancing Charms"): that list's line, so the charm charms.
  - Mircyl's Animation: an empty line (EQOA-Data spells it "Mercyls
    Animation", "Summons a pet"): that line, so it summons.
  - Anarchic Glyph: the list's "Decrease Hitpoints by 15" is Chaotic Glyph's
    156 cut short (every other Anarchic rank is its Chaotic rank's number:
    64, 96, 256): 156. Ours.
  - Rune of Animation: the list says "Target, Weapon proc, 30min" and gives no
    numbers: Stormlord's Rune's (the Animator's other rune, the same level),
    on the enchanter's INT, "Decrease Hitpoints by 336 ; Increase 10% based on
    INT". Ours: a GM may change it.

A row is changed only while it says what this expects it to (no line, or the
list's words). A row a GM has changed (gm_audit) keeps its line, except an
empty one, which is filled with the GM's visual and note kept (a community
pick accepted for a spell with no row makes such a row); a row of the private
migration (its source says so) is never touched. Every change is logged in
ench_fix_log (the row before and after, and whether there was one); --undo
puts each back as it was (a row this made is deleted) and drops the log. Both
take a copy of the database first (<db>.bak-<when>-pre-enchfix). Report only
by default. The server reads a line at each cast.

    tools/fix_enchanter_lines.py [--db eqoa.db] [--apply | --undo]
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

ENCHANTER = 12
TAG = "ench17"

LOG_SCHEMA = """CREATE TABLE IF NOT EXISTS ench_fix_log (
    spellid       INTEGER PRIMARY KEY,
    existed       INTEGER NOT NULL,
    line_before   TEXT NOT NULL,
    source_before TEXT NOT NULL,
    line_after    TEXT NOT NULL,
    source_after  TEXT NOT NULL,
    applied       TEXT NOT NULL
)"""

# by the spell's name (lower case): (the line it may have now, None for no
# line or an empty one; the line it gets; the source it then says)
FIXES = {
    "entrancing charm": (None, "Target, Charm ; Increase 25% based on CHA",
                         f"eqoa-data enchanter (CM), as Entrancing Charms, {TAG}"),
    "mircyl's animation": (None, "Summons a pet",
                           f"eqoa-data enchanter (Merchant), as Mercyls Animation, {TAG}"),
    "anarchic glyph": ("Group, Decrease Hitpoints by 15, 5min", "Group, Decrease Hitpoints by 156, 5min",
                       f"eqoa-data enchanter (CM), 15 read as Chaotic Glyph's 156 (ours), {TAG}"),
    "rune of animation": ("Target, Weapon proc, 30min",
                          "Target, Decrease Hitpoints by 336 ; Increase 10% based on INT, 30min",
                          f"eqoa-data enchanter (Animator MCA), no numbers in the list: Stormlord's Rune's on INT (ours), {TAG}"),
}

PRIVATE = re.compile(r"private migration", re.I)


def spell_name(name):
    """A spell's name as the server compares it."""
    n = (name or "").strip()
    if n.endswith("$"):
        n = n[:-1].rstrip()
    return n.replace("+", "").replace("`", "'").strip().lower()


def gm_changed(db):
    """The spell ids whose spelleffect row a GM has changed."""
    try:
        rows = db.execute("SELECT rowkey FROM gm_audit WHERE tbl = 'spelleffect'").fetchall()
    except sqlite3.OperationalError:
        return set()
    ids = set()
    for (k,) in rows:
        try:
            ids.add(int(json.loads(k)["spellid"]))
        except (ValueError, KeyError, TypeError):
            m = re.search(r"\d+", k or "")
            if m:
                ids.add(int(m.group()))
    return ids


def plan(db):
    """The changes: [(spellid, name, existed, line before, source before, line
    after, source after)], and the rows left as they are, with why."""
    gm = gm_changed(db)
    rows = db.execute("""
        SELECT sp.spellid, sp.spellname, se.spellid IS NOT NULL, COALESCE(se.line, ''), COALESCE(se.source, '')
        FROM classspells cs
        JOIN spellpattern sp ON sp.spellid = cs.spellid
        LEFT JOIN spelleffect se ON se.spellid = sp.spellid
        WHERE cs.class = ?
        ORDER BY sp.spellid""", (ENCHANTER,)).fetchall()
    changes, kept = [], []
    seen = set()
    for sid, name, existed, line, source in rows:
        if sid in seen:
            continue
        seen.add(sid)
        fix = FIXES.get(spell_name(name))
        if fix is None:
            continue
        want_before, after, src = fix
        if line == after:
            continue
        if PRIVATE.search(source):
            kept.append((sid, name, "its row is the private migration's"))
            continue
        if (want_before is None and line.strip()) or (want_before is not None and line != want_before):
            kept.append((sid, name, f"its line is not the one expected ({line!r})"))
            continue
        if sid in gm:
            if want_before is not None:
                kept.append((sid, name, "a GM changed its row"))
                continue
            # a GM gave it a visual and no line (a community pick accepted
            # for a spell with no row): the empty line is filled, the visual
            # and the GM's note kept
            src = f"{source}; line {src}" if source else src
        changes.append((sid, name, bool(existed), line, source, after, src))
    return changes, kept


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-enchfix"
    shutil.copy2(path, dst)
    return dst


def undo(db, path):
    try:
        logged = db.execute("SELECT spellid, existed, line_before, source_before, line_after FROM ench_fix_log").fetchall()
    except sqlite3.OperationalError:
        logged = []
    if not logged:
        print("nothing logged: nothing to undo")
        return
    print(f"backup: {backup(path)}")
    db.execute("BEGIN")
    for sid, existed, before, source, after in logged:
        now = db.execute("SELECT line FROM spelleffect WHERE spellid = ?", (sid,)).fetchone()
        if now is None or now[0] != after:
            print(f"  {sid}: changed since ({now[0] if now else 'no row'!r}); put back anyway")
        if existed:
            db.execute("UPDATE spelleffect SET line = ?, source = ? WHERE spellid = ?", (before, source, sid))
        else:
            db.execute("DELETE FROM spelleffect WHERE spellid = ?", (sid,))
    db.execute("DROP TABLE ench_fix_log")
    db.execute("COMMIT")
    print(f"put back {len(logged)} rows")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, isolation_level=None)

    if a.undo:
        undo(db, a.db)
        return

    changes, kept = plan(db)
    for sid, name, existed, before, _, after, _ in changes:
        print(f"{sid:6d} {name}\n         {before if existed else '(no row)'!r}\n      -> {after!r}")
    for sid, name, why in kept:
        print(f"{sid:6d} {name}: left as it is ({why})")
    print(f"{len(changes)} rows to change, {len(kept)} left")
    if not a.apply or not changes:
        if not a.apply:
            print("report only: --apply to write")
        return
    print(f"backup: {backup(a.db)}")
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN")
    db.execute(LOG_SCHEMA)
    for sid, _, existed, before, source, after, src in changes:
        db.execute("INSERT OR REPLACE INTO ench_fix_log (spellid, existed, line_before, source_before, line_after, source_after, applied) "
                   "VALUES (?, ?, ?, ?, ?, ?, ?)", (sid, 1 if existed else 0, before, source, after, src, when))
        if existed:
            db.execute("UPDATE spelleffect SET line = ?, source = ? WHERE spellid = ?", (after, src, sid))
        else:
            db.execute("INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (?, ?, 0, ?)", (sid, after, src))
    db.execute("COMMIT")
    print(f"changed {len(changes)} rows")


if __name__ == "__main__":
    main()
