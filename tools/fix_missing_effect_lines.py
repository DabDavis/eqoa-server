#!/usr/bin/env python3
"""Effect lines for the class spells that have none, out of public sources.

What a spell does is its line in spelleffect, which the server reads at each
cast (internal/entity/spelltext.go); a class spell with no row there, or an
empty line, does nothing when cast. On eqoaclassic (jib's review, F3 pass 19)
that was 84 shaman spells, whose class list in EQOA-Data words 18 of its 125
abilities, so tools/import_eqoadata_spells.py made their records with no
line, and 20 other class links (magician 8, shadow knight 4, wizard 3,
alchemist 2, warrior, bard and enchanter 1 each), plus three spells whose
rows hold a visual and an empty line (Remove Illusion, Surefall Potion,
Create Transformative).

The lines are tools/missing_effect_lines.json's, one a spell by its classes,
name and level, each with its source and the page it was read off:
  - the eqoa.allakhazam.com spell database as web.archive.org keeps it: its
    pages of the 2004-04-28 import (captured 2006-2012) give a spell's
    effects in the reader's own words ("Decrease Hitpoints by 10 ; Increase
    5% based on DEX", "Increase STR by 15"), its class and level, scope,
    duration, resist and area. A line is the page's effects in its order,
    after the scope's word ("Target", "Self", "Group") and before its
    duration; an area spell's radius is kept ("Radius 15", which the reader
    reads and leaves: an area spell lands on its one target); the hate
    rating is left out (a landing angers its target already, cast.go);
  - EQOA-Data's per-class lists where they word a spell under another name
    than its record's (the wizard's "O'Keils Radiation", "Arcane Bindings");
  - Sandstorm's harvest (harvests/sandstorm-spells-20261021): its scrolls'
    words name the creature a pet is ("Summons a wolf to aid you.") and say
    what a spell is where no page has it.
internal/entity/missinglines_test.go reads every line as the server does and
holds it to what its words say. The file also lists the spells no source
gives numbers or words for, with why: they keep no line, and no number here
is invented. What is ours rather than a source's (a reading, a match of two
names) says so in the row's source.

A line is written only where its spell has none: no row (one is made, the
visual 0, the server's stand-in of its kind) or an empty line (filled, its
visual and a GM's note kept). A line that exists is never changed, whoever
wrote it. Held, with why:
  - a line the file holds ("hold"): Savage and Tribal Healing, whose pages
    are effects with no cast time and no power (Tribal Healing is what
    Tribal Ward fires when its bearer is hit), not spells cast;
  - a line cast at an enemy ("Target" or "Group", and harmful) on a record of
    scope 0 (self): the server lands a cast by its record's scope
    (entity/cast.go, castTargetsLocked), so it would land on its own caster.
    eqoaclassic's imported shaman records all have scope 0 (the importer's,
    not a source's; each entry's "page" has the source's scope); run this
    again once a record says whom it is cast at;
  - a spell whose record's description names a proc (PROC_DESC, the reader's
    reProcDesc): the reader would take the line's hit for a melee proc;
  - a spell whose line a GM took away (gm_audit: the last change to its row
    deleted a line or emptied one). A GM's delete of a row with no line (a
    wrong visual taken back) takes no line away: the row written has no
    visual either.
A "Self" line that costs its caster (Cannibalize's hit points) is not held:
scope 0 is its own. A line cast at a friend on a record of scope 0 is written
and lands on its caster alone until the record is mended: the report says
which.

Every row written is logged in missing_line_log (the row before and after,
and whether there was one); --undo puts back each row still as this left it
(a row this made is deleted) and says which it left alone because they
changed since. Both take a copy of the database first
(<db>.bak-<when>-pre-missinglines). Report only by default. The server reads
a line at each cast, so no restart is needed. tools/import_spell_effects.py
--apply replaces the whole table: run this again after it.

    tools/fix_missing_effect_lines.py [--db eqoa.db] [--lines file.json] [--apply | --undo]
"""
import argparse
import datetime
import json
import os
import re
import shutil
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
DEFAULT_DB = os.path.join(HERE, "..", "eqoa.db")
DEFAULT_LINES = os.path.join(HERE, "missing_effect_lines.json")

CLASSES = ["warrior", "ranger", "paladin", "shadowknight", "monk", "bard", "rogue", "druid",
           "shaman", "cleric", "magician", "necromancer", "enchanter", "wizard", "alchemist"]

TAG = "lines19"

# the reader's words for a proc in a record's description (spelltext.go's
# reProcDesc; missinglines_test.go keeps the two the same)
PROC_DESC = re.compile(r"\bproc\b|chance to deal additional")

LOG_SCHEMA = """CREATE TABLE IF NOT EXISTS missing_line_log (
    spellid       INTEGER PRIMARY KEY,
    existed       INTEGER NOT NULL,
    line_before   TEXT NOT NULL,
    source_before TEXT NOT NULL,
    line_after    TEXT NOT NULL,
    source_after  TEXT NOT NULL,
    applied       TEXT NOT NULL
)"""


def spell_name(name):
    """A spell's name as the server compares it."""
    n = (name or "").strip()
    if n.endswith("$"):
        n = n[:-1].rstrip()
    return re.sub(r"\s+", " ", n.replace("+", "").replace("`", "'")).strip().lower()


def load_lines(path):
    with open(path) as f:
        data = json.load(f)
    return data.get("lines", []), data.get("none", [])


def gm_took_line(db):
    """The spell ids whose spelleffect line a GM took away on purpose: the
    last GM change to the row deleted a row that had a line, or emptied
    one. A row a GM deleted that had no line (a visual taken back, as the
    revert of a wrong community pick) is not one: its spell still has no
    line, and the line this writes keeps the visual the GM left, none."""
    try:
        rows = db.execute("SELECT rowkey, action, before, after FROM gm_audit WHERE tbl = 'spelleffect' ORDER BY id").fetchall()
    except sqlite3.OperationalError:
        return set()

    def line_of(blob):
        try:
            return (json.loads(blob or "null") or {}).get("line") or ""
        except (ValueError, AttributeError):
            return ""

    last = {}
    for k, _, before, after in rows:
        try:
            sid = int(json.loads(k)["spellid"])
        except (ValueError, KeyError, TypeError):
            m = re.search(r"\d+", k or "")
            if not m:
                continue
            sid = int(m.group())
        last[sid] = (line_of(before).strip(), line_of(after).strip())
    return {sid for sid, (before, after) in last.items() if before and not after}


def class_spells(db):
    """{(class, name, level): [spell row]} over every class link, a row a
    dict of the record and its spelleffect row (None for none)."""
    out = {}
    for cls, sid, lvl, name, scope, desc, line, fx, src in db.execute("""
            SELECT cs.class, sp.spellid, sp.abilitylvl, sp.spellname, sp.scope, sp.spelldesc,
                   se.line, se.fxdict, se.source
            FROM classspells cs
            JOIN spellpattern sp ON sp.spellid = cs.spellid
            LEFT JOIN spelleffect se ON se.spellid = sp.spellid
            ORDER BY cs.class, sp.abilitylvl, sp.spellid"""):
        out.setdefault((cls, spell_name(name), lvl), []).append({
            "spellid": sid, "name": name, "level": lvl, "scope": scope, "desc": desc or "",
            "row": None if line is None else {"line": line, "fxdict": fx, "source": src or ""}})
    return out


def row_source(entry):
    """What the row's source says: the line's source, ours marked."""
    s = entry["source"]
    if entry.get("ours"):
        s += f"; {entry['ours']} (ours)"
    return f"{s}, {TAG}"


def plan(db, lines, none):
    """The changes [(spellid, name, classes, existed, line before, source
    before, line after, source after)], the rows held or left with why
    [(spellid, name, why)], the notes on lines written [(spellid, name,
    note)] and the spells the file gives no line [(spellid, name, level,
    why)]."""
    by_key = class_spells(db)
    gm = gm_took_line(db)
    changes, kept, notes, uncovered = [], [], [], []
    done = set()
    for e in lines:
        rows = {}
        for cls in e["classes"]:
            for r in by_key.get((cls, spell_name(e["name"]), e["level"]), []):
                rows.setdefault(r["spellid"], (r, []))[1].append(cls)
        for sid, (r, classes) in sorted(rows.items()):
            if sid in done:
                continue
            done.add(sid)
            row, line = r["row"], e["line"]
            if row is not None and row["line"].strip():
                if row["line"] != line:
                    kept.append((sid, r["name"], f"it has a line ({row['line']!r})"))
                continue
            if e.get("hold"):
                kept.append((sid, r["name"], f"held: {e['hold']}"))
                continue
            if e["expect"]["harmful"] and e["expect"]["target"] != "self" and r["scope"] == 0:
                kept.append((sid, r["name"], "held: its line is cast at an enemy and its record's scope is 0 (self), "
                             "so it would land on its caster; run again once the record's scope is mended"))
                continue
            if PROC_DESC.search(r["desc"].lower()):
                kept.append((sid, r["name"], f"held: its record's description names a proc ({r['desc']!r}), "
                             "so the reader would take its hit for a melee proc"))
                continue
            if sid in gm:
                kept.append((sid, r["name"], "a GM took its line away"))
                continue
            src = row_source(e)
            if row is not None and row["source"]:
                # an empty line beside a visual (a GM's, the community's, a
                # recording's): the line is added, the visual and its note kept
                src = f"{row['source']}; line {src}"
            if r["scope"] == 0 and e["expect"]["target"] in ("target", "group"):
                notes.append((sid, r["name"], f"its record's scope is 0 (self): the {e['expect']['target']} line "
                              "lands on its caster alone until the record is mended"))
            changes.append((sid, r["name"], classes, row is not None, row["line"] if row else "",
                            row["source"] if row else "", line, src))
    for n in none:
        for cls in n["classes"]:
            for r in by_key.get((cls, spell_name(n["name"]), n["level"]), []):
                if r["spellid"] in done:
                    continue
                done.add(r["spellid"])
                if r["row"] is None or not r["row"]["line"].strip():
                    uncovered.append((r["spellid"], r["name"], r["level"], n["why"]))
    return changes, kept, notes, uncovered


def unlisted(db, lines, none):
    """Class spells with no line that the file does not name at all."""
    named = {(cls, spell_name(e["name"]), e["level"]) for e in list(lines) + list(none) for cls in e["classes"]}
    out = []
    for (cls, name, lvl), rows in sorted(class_spells(db).items()):
        if (cls, name, lvl) in named:
            continue
        for r in rows:
            if r["row"] is None or not r["row"]["line"].strip():
                out.append((r["spellid"], CLASSES[cls] if 0 <= cls < len(CLASSES) else cls, r["name"], lvl))
    return out


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-missinglines"
    shutil.copy2(path, dst)
    return dst


def undo(db, path):
    try:
        logged = db.execute("SELECT spellid, existed, line_before, source_before, line_after, source_after "
                            "FROM missing_line_log ORDER BY spellid").fetchall()
    except sqlite3.OperationalError:
        logged = []
    if not logged:
        print("nothing logged: nothing to undo")
        return
    print(f"backup: {backup(path)}")
    db.execute("BEGIN")
    put, left, gone = 0, 0, 0
    for sid, existed, line_before, source_before, line_after, source_after in logged:
        now = db.execute("SELECT line, fxdict, source FROM spelleffect WHERE spellid = ?", (sid,)).fetchone()
        if now is None and not existed:
            # the row this made is gone already (an import that replaced the
            # table): nothing of this is left to put back
            db.execute("DELETE FROM missing_line_log WHERE spellid = ?", (sid,))
            gone += 1
            continue
        if now is None or now[0] != line_after or now[2] != source_after:
            print(f"  {sid}: changed since ({'no row' if now is None else repr(now[0])}): left as it is")
            left += 1
            continue
        if existed:
            db.execute("UPDATE spelleffect SET line = ?, source = ? WHERE spellid = ?", (line_before, source_before, sid))
        elif now[1] != 0:
            print(f"  {sid}: its visual was set since ({now[1]:#x}): left as it is")
            left += 1
            continue
        else:
            db.execute("DELETE FROM spelleffect WHERE spellid = ?", (sid,))
        db.execute("DELETE FROM missing_line_log WHERE spellid = ?", (sid,))
        put += 1
    if left == 0:
        db.execute("DROP TABLE missing_line_log")
    db.execute("COMMIT")
    print(f"put back {put} rows" + (f", {gone} gone already" if gone else "") +
          (f", {left} left as they are (still logged)" if left else ""))


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--lines", default=DEFAULT_LINES, help="the lines (default tools/missing_effect_lines.json)")
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

    lines, none = load_lines(a.lines)
    changes, kept, notes, uncovered = plan(db, lines, none)
    for sid, name, classes, existed, before, _, after, src in changes:
        who = "/".join(CLASSES[c] for c in classes)
        print(f"{sid:6d} {who} {name}\n         {repr(before) if existed else '(no row)'}\n      -> {after!r}\n         [{src}]")
    for sid, name, note in notes:
        print(f"{sid:6d} {name}: {note}")
    for sid, name, why in kept:
        print(f"{sid:6d} {name}: left as it is ({why})")
    for sid, name, lvl, why in uncovered:
        print(f"{sid:6d} {name} (L{lvl}): no line written: {why}")
    for sid, cls, name, lvl in unlisted(db, lines, none):
        print(f"{sid:6d} {cls} {name} (L{lvl}): no line, and the file does not name it")
    print(f"{len(changes)} rows to write, {len(kept)} left or held, {len(uncovered)} without a line")
    if not a.apply or not changes:
        if not a.apply:
            print("report only: --apply to write")
        return
    print(f"backup: {backup(a.db)}")
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN")
    db.execute(LOG_SCHEMA)
    for sid, _, _, existed, before, source, after, src in changes:
        # a spell logged before is one whose row went since (an import that
        # replaced the table): the row as it is now is the one to put back
        db.execute("INSERT OR REPLACE INTO missing_line_log (spellid, existed, line_before, source_before, line_after, source_after, applied) "
                   "VALUES (?, ?, ?, ?, ?, ?, ?)", (sid, 1 if existed else 0, before, source, after, src, when))
        if existed:
            db.execute("UPDATE spelleffect SET line = ?, source = ? WHERE spellid = ?", (after, src, sid))
        else:
            db.execute("INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (?, ?, 0, ?)", (sid, after, src))
    db.execute("COMMIT")
    print(f"wrote {len(changes)} rows")


if __name__ == "__main__":
    main()
