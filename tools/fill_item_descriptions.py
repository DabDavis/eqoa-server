#!/usr/bin/env python3
"""Item records whose description is the placeholder "Add a real description." given a real one (the user,
2026-11-03, on a troll warrior: "also the basher shirt says Add a real description.").

2,043 of eqoaclassic's records carry the placeholder, the old dump's; Sandstorm's own records of the same items
carry it too (its Basher Shirt does), and so do 13 of EQOA-Data's, so some of SOE's live records had it. CLIENT
shows the description in the item's info window. A placeholder is replaced by, in this order:

1. **DECIDED**: the user's item, a description of ours where no record has one (the Basher Shirt: the Bashers
   are Grobb's warriors, whose guards are "Basher ..."; written as the other races' first shirts and tunics are,
   "A light tunic worn by a member of the Stormreapers.").
2. **Sandstorm's record of the same name** (the harvests, `--prices`), when it is not the placeholder.
3. **EQOA-Data's** (`--eqoa-data`, its Information/databases/items), one description for the name, unless it
   is a contributor's note rather than the game's text (NOTE: "Alignment: Good.", "Level 49 Epic - Rod.",
   "*Proc*", "Rediculously Rare Robe, most would kill to have it").
4. **Allakhazam's item page** (`--pages`, fetch_item_pages.py's pages.json), its "Dropped by ..." left out
   (the submitter's, not the game's).

A name two records of one source describe differently takes neither. The rest keep the placeholder and are
counted. A record a GM edited (gm_audit) is listed and left. Logged in item_desc_fill_log with the text it
replaced; --undo puts back each record still as this left it, then drops the log. Report only by default. The
client reads a record's description at its login: no restart; a player sees it at the next login.

    tools/fill_item_descriptions.py [--db eqoa.db] [--prices <harvests>] [--eqoa-data <folder>] [--pages <folder>]
                                    [--only PATTERNID ...] [--apply | --undo]
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
from fix_dump_weapons import key  # noqa: E402

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
PLACEHOLDER = re.compile(r"^\s*add a real description\.?\s*$", re.I)
# EQOA-Data's notes of its contributors, which its description field holds beside the game's text
NOTE = re.compile(r"^(alignment\s*:|level\s*\d+\s*epic)|\*|\bre?diculously\b|\bridiculously\b", re.I)
# (name, wear slot): description, why (OURS where no record has one)
DECIDED = {("basher shirt", 5): ("A coarse shirt worn by the Bashers of Grobb.",
                                  "ours: the user's item; no record describes it (Sandstorm's is the placeholder)")}

LOG_DDL = """CREATE TABLE IF NOT EXISTS item_desc_fill_log (
    seq       INTEGER PRIMARY KEY AUTOINCREMENT,
    patternid INTEGER NOT NULL,
    old_desc  TEXT NOT NULL,
    new_desc  TEXT NOT NULL,
    source    TEXT NOT NULL,
    applied   TEXT NOT NULL DEFAULT (datetime('now'))
)"""


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def gm_patterns(db):
    out = set()
    if not has_table(db, "gm_audit"):
        return out
    for (k,) in db.execute("SELECT rowkey FROM gm_audit WHERE tbl = 'itempattern'"):
        try:
            out.add(int(json.loads(k)["patternid"]))
        except (ValueError, KeyError, TypeError):
            pass
    return out


def real(text):
    t = " ".join((text or "").split())
    return t if t and not PLACEHOLDER.match(t) else ""


def files_of(paths, pattern="*.json"):
    out = []
    for p in paths or []:
        out += sorted(glob.glob(os.path.join(p, "**", pattern), recursive=True)) if os.path.isdir(p) else [p]
    return out


def walk(o):
    if isinstance(o, dict):
        if o.get("name") and "wear_slot" in o and "icon" in o:
            yield o
        for v in o.values():
            yield from walk(v)
    elif isinstance(o, list):
        for v in o:
            yield from walk(v)


def sandstorm_descs(paths):
    seen = collections.defaultdict(set)
    for f in files_of(paths):
        try:
            with open(f) as fh:
                d = json.load(fh)
        except (OSError, ValueError):
            continue
        for w in walk(d):
            if real(w.get("desc")):
                seen[key(w["name"])].add(real(w["desc"]))
    return seen


def eqoadata_descs(folder):
    seen = collections.defaultdict(set)
    if not folder:
        return seen
    for f in sorted(glob.glob(os.path.join(folder, "Information", "databases", "items", "*.json"))):
        with open(f) as fh:
            for it in json.load(fh).get("items", []):
                d = real(it.get("description"))
                if d and not NOTE.search(d):
                    seen[key(it.get("name"))].add(d)
    return seen


def allakhazam_descs(folder):
    seen = collections.defaultdict(set)
    path = os.path.join(folder, "pages.json") if folder else ""
    if not path or not os.path.exists(path):
        return seen
    with open(path) as fh:
        pages = json.load(fh)
    for page in pages.values():
        d = re.sub(r"\s*Dropped by [^.]*\.?\s*$", "", real(page.get("Description")))
        if d:
            seen[key(page.get("name"))].add(d)
    return seen


def plan(db, sources, only=None):
    """[(patternid, name, old, new, source)] and [(patternid, name, why)] left, and the placeholders kept."""
    gm = gm_patterns(db)
    todo, left = [], []
    kept = 0
    for pid, name, slot, desc in db.execute("SELECT patternid, itemname, equipslot, itemdesc FROM itempattern "
                                            "ORDER BY patternid").fetchall():
        if not PLACEHOLDER.match(desc or "") or (only and pid not in only):
            continue
        k = key(name)
        new = src = None
        if (k, slot) in DECIDED:
            new, src = DECIDED[(k, slot)]
        else:
            for label, seen in sources:
                got = seen.get(k, ())
                if len(got) == 1:
                    new, src = next(iter(got)), label
                    break
        if new is None:
            kept += 1
            continue
        if pid in gm:
            left.append((pid, name, "a GM changed it"))
            continue
        todo.append((pid, name, desc, new, src))
    return todo, left, kept


def apply(db, todo):
    db.execute(LOG_DDL)
    done = 0
    for pid, _name, old, new, src in todo:
        cur = db.execute("UPDATE itempattern SET itemdesc = ? WHERE patternid = ? AND itemdesc = ?", (new, pid, old))
        if cur.rowcount:
            db.execute("INSERT INTO item_desc_fill_log (patternid, old_desc, new_desc, source) VALUES (?, ?, ?, ?)",
                       (pid, old, new, src))
            done += 1
    return done


def undo(db):
    if not has_table(db, "item_desc_fill_log"):
        print("nothing logged: nothing to undo")
        return
    back = kept = 0
    for pid, old, new in db.execute("SELECT patternid, old_desc, new_desc FROM item_desc_fill_log "
                                    "ORDER BY seq DESC").fetchall():
        cur = db.execute("UPDATE itempattern SET itemdesc = ? WHERE patternid = ? AND itemdesc = ?", (old, pid, new))
        back, kept = (back + 1, kept) if cur.rowcount else (back, kept + 1)
    db.execute("DROP TABLE item_desc_fill_log")
    print(f"put back {back}; left {kept} changed since")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--prices", nargs="*", default=[], help="eqoa harvest JSON files or folders (Sandstorm's records)")
    ap.add_argument("--eqoa-data", help="a checkout of EQOA-Data")
    ap.add_argument("--pages", help="fetch_item_pages.py's folder (its pages.json)")
    ap.add_argument("--only", type=int, nargs="+", help="only these patternids")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the descriptions (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply wrote")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 10000")
    if a.undo:
        db.execute("BEGIN IMMEDIATE")
        undo(db)
        db.execute("COMMIT")
        return
    sources = [("Sandstorm's record", sandstorm_descs(a.prices)), ("EQOA-Data", eqoadata_descs(a.eqoa_data)),
               ("Allakhazam's page", allakhazam_descs(a.pages))]
    todo, left, kept = plan(db, sources, set(a.only) if a.only else None)
    by = collections.Counter(src.split(":")[0] for *_x, src in todo)
    print(f"{len(todo)} records change: " + ", ".join(f"{s}: {n}" for s, n in by.most_common()))
    for pid, name, _old, new, src in todo:
        print(f"  {pid} {name!r}: {new!r} ({src})")
    for pid, name, why in left:
        print(f"  left: {pid} {name!r}: {why}")
    print(f"{kept} records keep the placeholder: no source describes them")
    if not a.apply:
        print("report only: --apply to write")
        return
    db.execute("BEGIN IMMEDIATE")
    done = apply(db, todo)
    db.execute("COMMIT")
    print(f"wrote {done} records")


if __name__ == "__main__":
    main()
