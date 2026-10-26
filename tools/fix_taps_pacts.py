#!/usr/bin/env python3
"""The life taps' and pacts' lines in spelleffect, each its own class's, at
its own level.

tools/import_spell_effects.py gives a line to every spellpattern row of its
spell's NAME, the line most of EQOA-Data's class lists give winning (ties:
the first in class order). Where two classes, or two levels of one class,
share a name, a row can carry another's line: on eqoaclassic the
necromancer's level 16 Siphon Life had the level 1 one's ("Target, Decrease
Drain HP by 5 ; Increase 8% based on STA", for "Target, Drain HP by 29 and
gives to self"), and its level 4 Siphon Strength the shadow knight's level 24
("Target, Drain STR by 30 give self, 60sec", for "Target, Decrease Drain STR
by 10, 5min"). For the taps and the pacts (internal/entity/lifetap.go: a line
that takes for its caster or costs it), this gives each row the line its own
class's list gives the spell at its level, the class being the classspells
row that gives the spell; the row's source says so.

Two lines are corrected on top (F3 20261025-214751, jib: "Look up the values
and correct this"):
  - Infernal Pact: every pact in the lists costs twice its heal and every
    covenant three times (Gloom 25/50, Dark 45/90, Shadow 90/180, Tenebrous
    180/360, Umbral 270/540, Loathsome 680/1360; Infernal Covenant 540/1620);
    Infernal Pact's "1820" is the one that does not fit: 1080.
  - Drain Soul: the necromancer's level 48 tap is "Target, Decrease HP by
    231", the one tap of the line whose words take nothing for the caster:
    "Target, Drain HP by 231".

A row a GM has changed (an entry for it in gm_audit) is left as it is, and so
is a spell no class's list gives at its level. Only existing rows are changed;
a spell with no line is the importers' to fill. Run it after any
import_spell_effects.py --apply (which rewrites every row) and after an import
that adds spells.

Every change is logged in taps_fix_log (the row before and after); --undo puts
each logged row back as it was and drops the log. Both take a copy of the
database first (<db>.bak-<when>-pre-tapsfix). Report only by default. The
server reads a line at each cast; a necromancer's pets read their taps again
at the next start or GM edit of a spell.

    tools/fix_taps_pacts.py --eqoa-data ../external/EQOA-Data/Information/databases/spells \\
        [--db eqoa.db] [--apply | --undo]
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

# the class lists, in class-id order (WAR=0 ... ALC=14), as import_spell_effects.py
CLASS_FILES = ["warrior", "ranger", "paladin", "shadowknight", "monk", "bard", "rogue", "druid",
               "shaman", "cleric", "magician", "necromancer", "enchanter", "wizard", "alchemist"]

LOG_SCHEMA = """CREATE TABLE IF NOT EXISTS taps_fix_log (
    spellid       INTEGER PRIMARY KEY,
    line_before   TEXT NOT NULL,
    source_before TEXT NOT NULL,
    line_after    TEXT NOT NULL,
    source_after  TEXT NOT NULL,
    applied       TEXT NOT NULL
)"""

# a line of the family: what lifetap.go reads as taking for the caster or
# costing it
FAMILY = re.compile(r"\bdrain\b|transfer to caster|\bgives? (?:to )?self\b|\bto self\b|\bself only\b|"
                    r"\bcaster hp\b|\bgives? to group\b", re.I)

# corrections on top of the lists, by (class, name): (the list's line, ours)
CORRECTIONS = {
    (11, "infernal pact"): ("Target, Increase Hitpoints by 540, Decrease Hitpoints 1820 to Self",
                            "Target, Increase Hitpoints by 540, Decrease Hitpoints 1080 to Self"),
    (11, "drain soul"): ("Target, Decrease HP by 231", "Target, Drain HP by 231"),
}


def spell_name(name):
    """A spell's name as the server compares it (import_spell_effects.py's)."""
    n = (name or "").strip()
    if n.endswith("$"):
        n = n[:-1].rstrip()
    n = re.sub(r"\s*\((?:[A-Za-z]{2,3}(?:\s*[,/]\s*)?)+\)\s*$", "", n)
    return n.replace("+", "").replace("`", "'").strip().lower()


def load_lists(directory):
    """{(class, name): [(level, line, source)]} for the family's lines, with
    the corrections' own names whatever their words."""
    out = {}
    for cls, f in enumerate(CLASS_FILES):
        path = os.path.join(directory, f + ".json")
        if not os.path.exists(path):
            sys.exit(f"{path}: missing (want EQOA-Data's Information/databases/spells)")
        with open(path) as fh:
            data = json.load(fh)
        for level in data["list"]:
            for a in level["abilities"]:
                line = (a.get("description") or "").strip()
                key = (cls, spell_name(a.get("name")))
                if key in CORRECTIONS and line == CORRECTIONS[key][0]:
                    line = CORRECTIONS[key][1]
                elif not FAMILY.search(line):
                    continue
                out.setdefault(key, []).append((int(level["level"]), line, a.get("source") or "?"))
    return out


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


def plan(db, lists):
    """The changes: [(spellid, name, class, level, line before, source before,
    line after, source after)], and the rows left as they are, with why."""
    gm = gm_changed(db)
    rows = db.execute("""
        SELECT sp.spellid, sp.spellname, sp.abilitylvl, cs.class, se.line, se.source
        FROM spelleffect se
        JOIN spellpattern sp ON sp.spellid = se.spellid
        JOIN classspells cs ON cs.spellid = se.spellid
        ORDER BY cs.class, sp.abilitylvl, sp.spellid""").fetchall()
    by_id = {}
    for sid, name, lvl, cls, line, source in rows:
        by_id.setdefault(sid, []).append((name, lvl, cls, line, source))
    changes, kept = [], []
    for sid, owners in sorted(by_id.items()):
        name, lvl, _, line, source = owners[0]
        wants = set()
        for _, _, cls, _, _ in owners:
            for level, want, src in lists.get((cls, spell_name(name)), []):
                if level == lvl:
                    wants.add((want, f"eqoa-data {CLASS_FILES[cls]} ({src}) at level {level}, taps17", cls))
        if not wants:
            continue
        if len({w[0] for w in wants}) > 1:
            kept.append((sid, name, lvl, "the classes that have it give it different lines"))
            continue
        want, src, cls = sorted(wants)[0]
        if want == line:
            continue
        if sid in gm:
            kept.append((sid, name, lvl, "a GM changed its row"))
            continue
        changes.append((sid, name, cls, lvl, line, source, want, src))
    return changes, kept


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-tapsfix"
    shutil.copy2(path, dst)
    return dst


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--eqoa-data", help="EQOA-Data's Information/databases/spells")
    ap.add_argument("--db", default=DEFAULT_DB)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)

    if a.undo:
        try:
            logged = db.execute("SELECT spellid, line_before, source_before, line_after FROM taps_fix_log").fetchall()
        except sqlite3.OperationalError:
            logged = []
        if not logged:
            print("nothing logged: nothing to undo")
            return
        print(f"backup: {backup(a.db)}")
        db.execute("BEGIN")
        for sid, before, source, after in logged:
            now = db.execute("SELECT line FROM spelleffect WHERE spellid = ?", (sid,)).fetchone()
            if now is None or now[0] != after:
                print(f"  {sid}: changed since ({now[0] if now else 'no row'!r}); put back anyway")
            db.execute("UPDATE spelleffect SET line = ?, source = ? WHERE spellid = ?", (before, source, sid))
        db.execute("DROP TABLE taps_fix_log")
        db.execute("COMMIT")
        print(f"put back {len(logged)} rows")
        return

    if not a.eqoa_data:
        sys.exit("--eqoa-data is needed")
    changes, kept = plan(db, load_lists(a.eqoa_data))
    for sid, name, cls, lvl, before, _, after, _ in changes:
        print(f"{sid:6d} {CLASS_FILES[cls]:12s} L{lvl:<2d} {name}\n         {before!r}\n      -> {after!r}")
    for sid, name, lvl, why in kept:
        print(f"{sid:6d} L{lvl:<2d} {name}: left as it is ({why})")
    print(f"{len(changes)} rows to change, {len(kept)} left")
    if not a.apply or not changes:
        if not a.apply:
            print("report only: --apply to write")
        return
    print(f"backup: {backup(a.db)}")
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN")
    db.execute(LOG_SCHEMA)
    for sid, _, _, _, before, source, after, src in changes:
        db.execute("INSERT OR IGNORE INTO taps_fix_log (spellid, line_before, source_before, line_after, source_after, applied) "
                   "VALUES (?, ?, ?, ?, ?, ?)", (sid, before, source, after, src, when))
        db.execute("UPDATE taps_fix_log SET line_after = ?, source_after = ?, applied = ? WHERE spellid = ?",
                   (after, src, when, sid))
        db.execute("UPDATE spelleffect SET line = ?, source = ? WHERE spellid = ?", (after, src, sid))
    db.execute("COMMIT")
    print(f"changed {len(changes)} rows")


if __name__ == "__main__":
    main()
