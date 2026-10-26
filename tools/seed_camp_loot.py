#!/usr/bin/env python3
"""Loot tables for the XP camps' creatures: rows of loottables and npcloot
(database/loottables.go, handlers/loot.go; F3 pass 18, the user: "if you see
the loot tables for them add them").

EQOA-Data (github.com/Jadiction/EQOA-Data) names where 3,324 of its items
drop: almost all one of thirteen biome loot tables ("Arctic Loot Table",
"Desert Loot Table", ... "Generic Loot Table"), a few a named creature (Khal
Viceroy, the Khal Warlord); its map's zones list their quadrants' biomes
(NW, NE, SW, SE: the order that agrees best with each zone's neighbours, 86%
of neighbouring quadrants, against 77% for the next). So:

  - every table is written to loottables under the biome's name, lower case
    ("arctic", "rathe mountains", "generic", "khal viceroy"), each item by
    its name in itempattern (the lowest id of a name, never a spell scroll)
    for creatures from 5 levels below its own to 5 above (an item of level 55
    or more up to 70, for the raid creatures above 60; one with no level for
    any), weight 1;
  - each XP camp of tools/gen_camp_teles.py's (the #tele camps) binds its
    creatures (every name of its kind within 150 of its anchor) in npcloot to
    its zone's biome table, the quadrant its point on EQOA-Data's map falls
    in, at 25% a kill, and to the generic table at 15%. A name in several
    camps takes the biome of the one it is most of. Odus's zones have no
    biomes on the map, so its camps get the generic table alone.

The chances, the one roll a table a kill and the level window are ours, not
the live game's (no record of them exists); GMs change any row in the panel
(Loot tables, Creature loot). Rows are written with source camp-seed and never
over a GM's; each is logged in camp_loot_seed, and --undo deletes those still
as written (a row a GM changed is kept, and named). Report only by default;
--apply and --undo copy the database first (<db>.bak-<when>-pre-campseed).
The server reads both tables at every corpse: no restart is needed.

    python3 -E -s tools/seed_camp_loot.py --db eqoa.db \\
        --eqoa-data ../external/EQOA-Data [--apply | --undo] [--quiet]
"""

import argparse
import collections
import datetime
import glob
import json
import math
import os
import re
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import gen_camp_teles as camps  # noqa: E402

SOURCE = "camp-seed"
BIOME_CHANCE, GENERIC_CHANCE = 25, 15
WINDOW = 5
CAMP_REACH = 150.0
SCROLL_ART = -607649328  # database.ScrollArt: a spell scroll's icon
QUADRANTS = ("NW", "NE", "SW", "SE")

# the tables as database/loottables.go makes them
SCHEMA = """
CREATE TABLE IF NOT EXISTS loottables (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    tablename TEXT NOT NULL,
    patternid INTEGER NOT NULL,
    weight    INTEGER NOT NULL DEFAULT 1,
    minlevel  INTEGER NOT NULL DEFAULT 1,
    maxlevel  INTEGER NOT NULL DEFAULT 60,
    source    TEXT NOT NULL DEFAULT 'gm'
);
CREATE INDEX IF NOT EXISTS idx_loottables_table ON loottables(tablename);
CREATE TABLE IF NOT EXISTS npcloot (
    npc_name  TEXT NOT NULL,
    tablename TEXT NOT NULL,
    chance    INTEGER NOT NULL DEFAULT 25,
    rolls     INTEGER NOT NULL DEFAULT 1,
    note      TEXT NOT NULL DEFAULT '',
    source    TEXT NOT NULL DEFAULT 'gm',
    PRIMARY KEY (npc_name, tablename)
);
CREATE TABLE IF NOT EXISTS camp_loot_seed (
    kind      TEXT NOT NULL,
    key       TEXT NOT NULL,
    row       TEXT NOT NULL,
    PRIMARY KEY (kind, key)
);
"""


def npc_key(name):
    """database.NPCClassKey: lower case, trimmed, no trailing $."""
    n = name.strip().lower()
    return n[:-1].strip() if n.endswith("$") else n


def table_of(source_name):
    n = source_name.strip()
    n = re.sub(r"\s+Loot Table$", "", n, flags=re.I)
    n = re.sub(r"^the\s+", "", n, flags=re.I)
    n = re.sub(r"'s$", "", n)
    return n.lower()


def items_by_table(data):
    out = collections.defaultdict(list)  # table -> [(name, level)]
    for f in sorted(glob.glob(os.path.join(data, "Information", "databases", "items", "*.json"))):
        d = json.load(open(f))
        items = d["items"] if isinstance(d, dict) else d
        for it in items:
            src = it.get("source")
            if not src:
                continue
            for s in src if isinstance(src, list) else [src]:
                if s.get("type") == "mobDrop" and s.get("name"):
                    lv = (it.get("requirements") or {}).get("level")
                    out[table_of(s["name"])].append((it["name"].strip(), lv if isinstance(lv, int) else None))
    return out


def window(level):
    if level is None:
        return 1, 70
    lo, hi = max(1, level - WINDOW), level + WINDOW
    return lo, 70 if level >= 55 else hi


def biome_at(zones, mx, my):
    """The biome EQOA-Data's map gives a point: its nearest zone's (or town's)
    quadrant; None where the nearest has none (Odus's)."""
    best = min(zones["zones"] + zones["towns"], key=lambda z: math.hypot(z["x"] - mx, z["y"] - my))
    b = best.get("biomes")
    if not b or len(b) != 4:
        return None, best["name"]
    q = ("S" if my >= best["y"] else "N") + ("E" if mx >= best["x"] else "W")
    return b[QUADRANTS.index(q)], best["name"]


def plan(db, data):
    zones, rows = camps.load(db, data)
    fitted = camps.fits(zones, rows)
    pattern = {}
    for pid, name, icon, slot in db.execute("SELECT patternid, itemname, itemicon, equipslot FROM itempattern ORDER BY patternid"):
        if not name or (icon == SCROLL_ART and slot == -1):
            continue
        pattern.setdefault(name.strip().lower(), pid)
    entries, unmatched = [], collections.Counter()
    for table, items in sorted(items_by_table(data).items()):
        seen = set()
        for name, lv in items:
            pid = pattern.get(name.lower())
            if pid is None:
                unmatched[table] += 1
                continue
            if pid in seen:
                continue
            seen.add(pid)
            lo, hi = window(lv)
            entries.append((table, pid, 1, lo, hi))
    # each camp's creatures, and its biome
    votes = collections.defaultdict(collections.Counter)  # name -> biome -> count
    camp_of = collections.defaultdict(list)
    report = []
    for spec in camps.CAMPS:
        p, why = camps.place(spec, zones, rows, fitted)
        if p is None:
            continue
        pts = [q for q in zones["pois"] if q["category"] == "camp" and q["name"] == spec[2] and q.get("note")]
        if spec[3] is not None:
            pts = [pts[i] for i in spec[3] if i < len(pts)]
        mx = sum(q["x"] for q in pts) / len(pts)
        my = sum(q["y"] for q in pts) / len(pts)
        biome, zone = biome_at(zones, mx, my) if p["world"] == 0 else (None, "Odus")
        rx = re.compile(spec[4], re.I)
        kinds = collections.Counter(
            npc_key(r[1]) for r in rows
            if r[5] == p["world"] and rx.search(r[1].strip()) and math.hypot(r[2] - p["x"], r[4] - p["z"]) <= CAMP_REACH)
        for k, n in kinds.items():
            votes[k][biome.lower() if biome else ""] += n
            camp_of[k].append(p["key"])
        report.append((p["key"], zone, biome, len(kinds)))
    bindings = []
    for k in sorted(votes):
        biome = votes[k].most_common(1)[0][0]
        note = "XP camp " + ", ".join(sorted(set(camp_of[k])))
        if biome:
            bindings.append((k, biome, BIOME_CHANCE, 1, note + f": its zone's biome on EQOA-Data's map, {biome}"))
        bindings.append((k, "generic", GENERIC_CHANCE, 1, note + ": EQOA-Data's generic table"))
    return entries, bindings, unmatched, report


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out, n = f"{path}.bak-{when}-pre-campseed", 2
    while os.path.exists(out):
        out, n = f"{path}.bak-{when}-{n}-pre-campseed", n + 1
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def undo(db):
    kept, gone = [], 0
    for kind, key, row in db.execute("SELECT kind, key, row FROM camp_loot_seed").fetchall():
        want = json.loads(row)
        if kind == "entry":
            cur = db.execute("SELECT tablename, patternid, weight, minlevel, maxlevel, source FROM loottables WHERE id = ?", (int(key),)).fetchone()
            if cur is None:
                db.execute("DELETE FROM camp_loot_seed WHERE kind = ? AND key = ?", (kind, key))
            elif list(cur) == want:
                db.execute("DELETE FROM loottables WHERE id = ?", (int(key),))
                db.execute("DELETE FROM camp_loot_seed WHERE kind = ? AND key = ?", (kind, key))
                gone += 1
            else:
                kept.append(f"loottables row {key}")
        else:
            name, table = json.loads(key)
            cur = db.execute("SELECT npc_name, tablename, chance, rolls, note, source FROM npcloot WHERE npc_name = ? AND tablename = ?", (name, table)).fetchone()
            if cur is None:
                db.execute("DELETE FROM camp_loot_seed WHERE kind = ? AND key = ?", (kind, key))
            elif list(cur) == want:
                db.execute("DELETE FROM npcloot WHERE npc_name = ? AND tablename = ?", (name, table))
                db.execute("DELETE FROM camp_loot_seed WHERE kind = ? AND key = ?", (kind, key))
                gone += 1
            else:
                kept.append(f"npcloot {name!r} on {table!r}")
    db.commit()
    print(f"deleted {gone} seed rows" + (f"; kept {len(kept)} a GM changed: " + ", ".join(kept) if kept else ""))


def apply(db, entries, bindings):
    have = {(t, p) for t, p in db.execute("SELECT tablename, patternid FROM loottables")}
    wrote = 0
    for t, p, w, lo, hi in entries:
        if (t, p) in have:
            continue
        cur = db.execute("INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel, source) VALUES (?, ?, ?, ?, ?, ?)",
                         (t, p, w, lo, hi, SOURCE))
        db.execute("INSERT INTO camp_loot_seed (kind, key, row) VALUES ('entry', ?, ?)",
                   (str(cur.lastrowid), json.dumps([t, p, w, lo, hi, SOURCE])))
        wrote += 1
    bound = {(n, t) for n, t in db.execute("SELECT npc_name, tablename FROM npcloot")}
    tied = 0
    for name, t, chance, rolls, note in bindings:
        if (name, t) in bound:
            continue
        db.execute("INSERT INTO npcloot (npc_name, tablename, chance, rolls, note, source) VALUES (?, ?, ?, ?, ?, ?)",
                   (name, t, chance, rolls, note, SOURCE))
        db.execute("INSERT INTO camp_loot_seed (kind, key, row) VALUES ('binding', ?, ?)",
                   (json.dumps([name, t]), json.dumps([name, t, chance, rolls, note, SOURCE])))
        tied += 1
    db.commit()
    print(f"wrote {wrote} loottables rows and {tied} npcloot rows")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--eqoa-data", required=True)
    ap.add_argument("--apply", action="store_true", help="write the rows")
    ap.add_argument("--undo", action="store_true", help="delete the rows written, as written")
    ap.add_argument("--no-backup", action="store_true", help="write without a copy first")
    ap.add_argument("--quiet", action="store_true", help="the counts only")
    args = ap.parse_args()

    db = sqlite3.connect(args.db)
    if args.undo:
        try:
            db.execute("SELECT 1 FROM camp_loot_seed LIMIT 1")
        except sqlite3.OperationalError:
            print("nothing to undo: no camp_loot_seed table")
            return
        if not args.no_backup:
            print("copy:", backup(db, args.db))
        undo(db)
        return

    entries, bindings, unmatched, report = plan(db, args.eqoa_data)
    per_table = collections.Counter(t for t, *_ in entries)
    print(f"{len(entries)} items in {len(per_table)} tables: " + ", ".join(f"{t} {n}" for t, n in sorted(per_table.items())))
    if unmatched:
        print("  not in itempattern: " + ", ".join(f"{t} {n}" for t, n in sorted(unmatched.items())))
    names = sorted({b[0] for b in bindings})
    print(f"{len(names)} creature names of {len(report)} camps bound, {len(bindings)} rows")
    if not args.quiet:
        for key, zone, biome, n in report:
            print(f"  {key:15s} {n:3d} names, zone {zone}: {biome or 'no biome: generic only'}")
        for b in bindings:
            print(f"      {b[0]!r} -> {b[1]} {b[2]}%")
    if not (args.apply):
        print("(report only: --apply writes them)")
        return
    db.executescript(SCHEMA)
    if not args.no_backup:
        print("copy:", backup(db, args.db))
    apply(db, entries, bindings)


if __name__ == "__main__":
    main()
