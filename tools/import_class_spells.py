#!/usr/bin/env python3
"""Which spells each class has, into the server's classspells table: the
spells a new character of the class is given when world.start_all_spells is
on (handlers/character.go).

An ability record (spellpattern) names no class. Three sources do:

- Sandstorm's dumps (`eqoa harvest` recordings): the abilities a character
  of a class owns, by their own ids. These win: a class's spell of a name is
  the id its dump gives.
- Sandstorm's spell vendors (the same recordings): each scroll's class mask,
  and the scroll's name is its spell's.
- The EQOA-Data project's per-class lists (Information/databases/spells/
  <class>.json), by name: merchant, quest, CM and other spells alike. A
  spell the lists give a class only as a mastery ("CM"), a level-60 epic or a
  master class's ("MCA", "<master class> MCA") gets its row with that kind's
  source ("eqoa-data cm", "eqoa-data epic", "eqoa-data mca", "eqoa-data
  master <name>": tools/import_eqoadata_mastery_spells.py's kind_source),
  which no class grant hands out (internal/database/grantsource.go); a name
  the lists give the class by a level too has a plain "eqoa-data" row.

A were form or a were's ability (Ratwere, Feral Lion Form, "Wererat Melee
Ability"...) is no class's, whoever's dump holds it: the were quests are any
class's and the forms past the first are masteries (F3 20261025-220040). It
is left out, by the server's own rule (database.IsWereAbility).

A name is compared as the server's scribe compares it (no trailing class
tag, no bard '+', any case). A name with several spellpattern rows gives the
class one of them: the class's own dump's id, else an id any class's dump
uses (Sandstorm's numbering: Return Home is 1623 for every class, not Return
Home's 1542), else the lowest; so a class's book never holds one spell twice
over.

The table is this tool's alone: --apply replaces every row in one
transaction, --clear empties it. Report only by default.

    tools/import_class_spells.py --eqoa-data ../external/EQOA-Data/Information/databases/spells \\
        [--harvest <recording.json or dir> ...] [--db eqoa.db] [--apply | --clear]
"""
import argparse
import collections
import glob
import json
import os
import re
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from import_eqoadata_mastery_spells import kind_source, leveled  # noqa: E402

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
CLASSES = "WAR RAN PAL SK MNK BRD RGE DRD SHA CL MAG NEC ENC WIZ ALC".split()
CLASS_FILES = ["warrior", "ranger", "paladin", "shadowknight", "monk", "bard", "rogue", "druid",
               "shaman", "cleric", "magician", "necromancer", "enchanter", "wizard", "alchemist"]
SCROLL_ICON = -607649328
# database.IsWereAbility's pattern, over a spell's name and description
WERE = re.compile(r"(?i)\bwere(rat|wolf|bear|lion|gator)|(rat|lion|wolf|bear|gator)were\b"
                  r"|power of the beast|^assume the form of|^cm: illusion\.?$")

DDL = """
CREATE TABLE IF NOT EXISTS classspells (
    class   INTEGER NOT NULL,  -- WAR=0 ... ALC=14
    spellid INTEGER NOT NULL,
    source  TEXT NOT NULL,     -- dump, vendor, eqoa-data, or eqoa-data cm|epic|mca|master <name>
    PRIMARY KEY (class, spellid)
);
"""


def spell_name(name):
    """The name a scroll and its spell share (the server's spellKey)."""
    n = (name or "").strip()
    if n.endswith("$"):
        n = n[:-1].rstrip()
    n = re.sub(r"\s*\((?:[A-Za-z]{2,3}(?:\s*[,/]\s*)?)+\)\s*$", "", n)
    return n.replace("+", "").replace("`", "'").strip().lower()


def harvest_files(paths):
    files = []
    for p in paths:
        files += sorted(glob.glob(os.path.join(p, "*.json"))) if os.path.isdir(p) else [p]
    return files


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--eqoa-data", help="EQOA-Data's Information/databases/spells directory")
    ap.add_argument("--harvest", nargs="*", default=[], help="eqoa harvest recordings (files or directories)")
    ap.add_argument("--apply", action="store_true", help="replace the table's rows")
    ap.add_argument("--clear", action="store_true", help="empty the table")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    if a.clear:
        db.executescript(DDL)
        db.execute("DELETE FROM main.classspells")
        print("classspells emptied")
        return
    if not a.eqoa_data:
        sys.exit("--eqoa-data is required")

    ids_by_name = collections.defaultdict(list)
    level = {}
    were = set()
    for sid, name, lvl, desc in db.execute(
            "SELECT spellid, spellname, abilitylvl, COALESCE(spelldesc, '') FROM main.spellpattern ORDER BY spellid"):
        if WERE.search(name or "") or WERE.search(desc):
            were.add(sid)
            continue
        ids_by_name[spell_name(name)].append(sid)
        level[sid] = lvl

    # class -> {name: (spellid, source)}
    got = [dict() for _ in CLASSES]

    def give(cls, name, sid, source):
        if name not in got[cls]:
            got[cls][name] = (sid, source)

    # 1. the dumps, by id
    dumped = collections.defaultdict(dict)  # class -> name -> [ids]
    for f in harvest_files(a.harvest):
        with open(f) as fh:
            d = json.load(fh)
        cls = d.get("character", {}).get("class")
        if cls is None or not 0 <= cls < len(CLASSES):
            continue
        for ab in d.get("abilities", []) + [x["ability"] for x in d.get("learned", [])]:
            if ab["id"] in level:
                dumped[cls].setdefault(spell_name(ab["name"]), set()).add(ab["id"])
    for cls, names in dumped.items():
        for name, ids in names.items():
            for sid in sorted(ids):  # a dump may hold one name twice (Mouse Form): both are the class's
                got[cls][f"{name}#{sid}"] = (sid, "dump")
            got[cls][name] = (min(ids), "dump")

    any_dump = {sid for names in dumped.values() for ids in names.values() for sid in ids}

    def by_name(cls, name, source):
        if name in got[cls]:
            return
        ids = ids_by_name.get(name)
        if ids:
            give(cls, name, next((s for s in ids if s in any_dump), ids[0]), source)

    # 2. the vendors' scrolls, by class mask
    for f in harvest_files(a.harvest):
        with open(f) as fh:
            d = json.load(fh)
        for m in d.get("merchants") or []:
            for w in m.get("wares") or []:
                if w.get("icon") != SCROLL_ICON:
                    continue
                for cls in range(len(CLASSES)):
                    if w["class_mask"] >> cls & 1:
                        by_name(cls, spell_name(w["name"]), "vendor")

    # 3. EQOA-Data's lists: what a level gives first, then what only a
    # mastery, an epic or a master class gives (a row of that kind's source)
    entries = []
    for cls, fname in enumerate(CLASS_FILES):
        path = os.path.join(a.eqoa_data, fname + ".json")
        if not os.path.exists(path):
            sys.exit(f"{path}: missing")
        with open(path) as fh:
            data = json.load(fh)
        for lv in data["list"]:
            for ab in lv["abilities"]:
                entries.append((cls, spell_name(ab["name"]), (ab.get("source") or "").strip()))
    for cls, name, how in entries:
        if leveled(how) or kind_source(how) is None:
            by_name(cls, name, "eqoa-data")
    for cls, name, how in entries:
        if not leveled(how) and kind_source(how) is not None:
            by_name(cls, name, kind_source(how))

    rows = sorted({(cls, sid, src) for cls in range(len(CLASSES)) for sid, src in got[cls].values()})
    # one name's two dump ids are both kept; the plain-name entry repeats one of them
    rows = sorted({(c, s): src for c, s, src in rows}.items())
    per = collections.Counter(c for (c, _), _ in rows)
    srcs = collections.Counter((c, src) for (c, _), src in rows)
    print(f"spellpattern rows {len(level) + len(were)} ({len(were)} were abilities, no class's); class spells {len(rows)}")
    for cls, name in enumerate(CLASSES):
        top = max([level[s] for (c, s), _ in rows if c == cls] or [0])
        kept = sum(n for (c, src), n in srcs.items() if c == cls and src.startswith("eqoa-data "))
        print(f"  {name:3s} {per[cls]:3d} spells (dump {srcs[(cls, 'dump')]}, vendor {srcs[(cls, 'vendor')]}, "
              f"eqoa-data {srcs[(cls, 'eqoa-data')]}, not given out {kept}), levels up to {top}")
    if not a.apply:
        print("(report only: --apply to write it)")
        return
    db.executescript(DDL)
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute("DELETE FROM main.classspells")
        db.executemany("INSERT INTO main.classspells (class, spellid, source) VALUES (?, ?, ?)",
                       [(c, s, src) for (c, s), src in rows])
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"applied to {a.db}: {len(rows)} rows")


if __name__ == "__main__":
    main()
