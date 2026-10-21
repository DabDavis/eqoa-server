#!/usr/bin/env python3
"""What each spell does, into the server's spelleffect table: the line that
says so, out of the EQOA-Data project's per-class spell lists, and the spell
visual Sandstorm plays for it, out of `eqoa harvest` recordings.

A spell's ability record (spellpattern) says how it is cast and never what it
does. EQOA-Data's lists (https://github.com/Jadiction/EQOA-Data,
Information/databases/spells/<class>.json) give one line a spell, "Target,
Increase AC by 150, Max Power by 60, MaxHP by 30, HP by 30, 30min", which the
server reads (internal/entity/spelltext.go) when the spell lands. A line goes
to every spellpattern row of its spell's name, compared without a trailing
class tag ("Root (SHA/DRD)"), the bard '+' or case. When the lists give one
name different lines, the line most of them give is taken (ties: the first in
class order), and the report names the disagreements.

The visual is CastingCombat's (0x00B1) last field: a spell effect id the
server must choose, since nothing in the ability says which. `eqoa harvest`
records casts (EQOA_HARVEST_SPELLS); a step "cast <name> ..." that started
the cast holds its 0x00CD (the lockout) and its 0x00B1 at the same moment,
in one datagram, and that 0x00B1 gives the spell's (another 0x00B1 in the
step can be a creature's swing). Spells without one get the server's
stand-in for their kind (entity/cast.go). --visuals adds others recorded
some other way, one `name<TAB>0xDICTID` a line (# comments), as from a
capture's casts.

The table is this tool's alone: --apply replaces every row in one
transaction, --clear empties it. Report only by default.

    tools/import_spell_effects.py --eqoa-data ../external/EQOA-Data/Information/databases/spells \\
        [--harvest <recording.json> ...] [--visuals <file.tsv>] [--db eqoa.db] [--apply | --clear]
"""
import argparse
import collections
import glob
import json
import os
import re
import sqlite3
import struct
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")

# The class lists, in class-id order (WAR=0 ... ALC=14).
CLASS_FILES = ["warrior", "ranger", "paladin", "shadowknight", "monk", "bard", "rogue", "druid",
               "shaman", "cleric", "magician", "necromancer", "enchanter", "wizard", "alchemist"]

DDL = """
CREATE TABLE IF NOT EXISTS spelleffect (
    spellid INTEGER PRIMARY KEY,
    line    TEXT NOT NULL,     -- what the spell does, as EQOA-Data words it
    fxdict  INTEGER NOT NULL,  -- CastingCombat's spell visual; 0: the server's stand-in
    source  TEXT NOT NULL      -- where the line and the visual came from
);
"""


def spell_name(name):
    """A spell's name as another table spells it: no '$' mark, no trailing
    class tag, no bard '+', a backtick as an apostrophe, any case (the
    server's spellKey)."""
    n = (name or "").strip()
    if n.endswith("$"):
        n = n[:-1].rstrip()
    n = re.sub(r"\s*\((?:[A-Za-z]{2,3}(?:\s*[,/]\s*)?)+\)\s*$", "", n)
    return n.replace("+", "").replace("`", "'").strip().lower()


def load_lines(directory):
    """{spell name: [(line, class file, source)]} over every class list."""
    lines = collections.defaultdict(list)
    for cls in CLASS_FILES:
        path = os.path.join(directory, cls + ".json")
        if not os.path.exists(path):
            sys.exit(f"{path}: missing (want EQOA-Data's Information/databases/spells)")
        with open(path) as f:
            data = json.load(f)
        for level in data["list"]:
            for a in level["abilities"]:
                line = (a.get("description") or "").strip()
                if line:
                    lines[spell_name(a["name"])].append((line, cls, a.get("source") or ""))
    return lines


def load_visuals(paths):
    """{spell name: fx} out of harvest recordings' cast steps."""
    fx = {}
    for p in paths:
        files = sorted(glob.glob(os.path.join(p, "*.json"))) if os.path.isdir(p) else [p]
        for f in files:
            with open(f) as fh:
                d = json.load(fh)
            for st in d.get("steps", []):
                m = re.match(r"cast (.+?) \(slot \d+\) on ", st.get("what", ""))
                if not m:
                    continue
                msgs = [(x.get("t"), bytes.fromhex(x["hex"])) for x in st.get("messages", []) if isinstance(x, dict)]
                start = next((t for t, raw in msgs if raw[:2] == b"\xcd\x00"), None)
                for t, raw in msgs:
                    # 0x00B1: caster, target, count, count ids, cast ms, fx
                    if start is not None and t == start and raw[:2] == b"\xb1\x00" and len(raw) >= 19:
                        n = raw[10]
                        if len(raw) >= 11 + 4 * n + 8:
                            fx[spell_name(m.group(1))] = struct.unpack_from("<I", raw, 11 + 4 * n + 4)[0]
                        break
    return fx


def load_visual_table(path):
    """{spell name: fx} out of a `name<TAB>0xDICTID` file."""
    fx = {}
    with open(path) as f:
        for n, raw in enumerate(f, 1):
            line = raw.split("#", 1)[0].strip()
            if not line:
                continue
            name, _, value = line.rpartition("\t")
            if not name:
                sys.exit(f"{path}:{n}: want name<TAB>0xDICTID")
            fx[spell_name(name)] = int(value, 0)
    return fx


def plan(db, lines, visuals):
    rows, missing, conflicts = {}, [], []
    names = collections.defaultdict(list)
    for sid, name in db.execute("SELECT spellid, spellname FROM main.spellpattern ORDER BY spellid"):
        names[spell_name(name)].append(sid)
    for key, ids in sorted(names.items()):
        cands = lines.get(key)
        if not cands:
            missing.append(key)
            # a visual still goes in, under an empty line (the cast lands
            # as nothing, but looks as it does)
            if visuals.get(key):
                for sid in ids:
                    rows[sid] = ("", visuals[key], "visual recorded; no eqoa-data line")
            continue
        counts = collections.Counter(c[0] for c in cands)
        best = max(counts.values())
        line, cls, src = next(c for c in cands if counts[c[0]] == best)
        if len(counts) > 1:
            conflicts.append((key, len(counts), cls))
        fx = visuals.get(key, 0)
        source = f"eqoa-data {cls}" + (f" ({src})" if src else "") + ("; visual recorded" if fx else "")
        for sid in ids:
            rows[sid] = (line, fx, source)
    return rows, missing, conflicts


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--eqoa-data", help="EQOA-Data's Information/databases/spells directory")
    ap.add_argument("--harvest", nargs="*", default=[], help="eqoa harvest recordings with cast steps (files or directories)")
    ap.add_argument("--visuals", help="more recorded visuals: name<TAB>0xDICTID a line")
    ap.add_argument("--apply", action="store_true", help="replace the table's rows")
    ap.add_argument("--clear", action="store_true", help="empty the table")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    if a.clear:
        db.executescript(DDL)
        db.execute("DELETE FROM main.spelleffect")
        print("spelleffect emptied")
        return
    if not a.eqoa_data:
        sys.exit("--eqoa-data is required")
    lines = load_lines(a.eqoa_data)
    visuals = load_visuals(a.harvest)
    if a.visuals:
        visuals.update(load_visual_table(a.visuals))
    rows, missing, conflicts = plan(db, lines, visuals)
    total = db.execute("SELECT count(*) FROM main.spellpattern").fetchone()[0]
    print(f"spellpattern rows {total}: with a line {sum(1 for r in rows.values() if r[0])}, with a recorded visual {sum(1 for r in rows.values() if r[1])}")
    print(f"spell names with no line: {len(missing)}" + (f" (e.g. {', '.join(missing[:8])})" if missing else ""))
    for key, n, cls in conflicts:
        print(f"    {key!r}: {n} different lines in the lists; took {cls}'s")
    unused = sorted(k for k in visuals if not any(spell_name(n) == k for (n,) in db.execute("SELECT spellname FROM main.spellpattern")))
    if unused:
        print(f"recorded visuals of spells not in spellpattern: {unused}")
    if not a.apply:
        print("(report only: --apply to write it)")
        return
    db.executescript(DDL)
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute("DELETE FROM main.spelleffect")
        db.executemany("INSERT INTO main.spelleffect (spellid, line, fxdict, source) VALUES (?, ?, ?, ?)",
                       [(sid, line, fx, src) for sid, (line, fx, src) in sorted(rows.items())])
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"applied to {a.db}: {len(rows)} rows")


if __name__ == "__main__":
    main()
