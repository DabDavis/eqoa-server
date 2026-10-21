#!/usr/bin/env python3
"""
Import what `eqoa harvest` (eqoa-rs) collected from a server into eqoa.db: its
spell vendors' catalogs, the items in them, and every ability record the
harvest's characters held.

A client cannot ask a server for a spell list. CLIENT's only readers of an
ability record are Msg00B3 (one ability pushed) and the MemoryDump, so a
client learns its own character's spells and nothing more. What a server does
show any client is its merchants. On Sandstorm the spell vendors are every
class's spell list: three a class, their wares scroll items, any class's
catalog open to anyone (claude-notes/spell-harvest-sandstorm-20261021.md). The
harvest's JSON holds each ware as `ReadNetItem` read it and each ability as
`VIAbility::Read` read it, every field, so the rows below are the wire's.

itempattern (every ware of an imported merchant)
  The ware's VIItem fields in writeItemPattern's order: id -> patternid,
  value -> itemcost, p08 -> unk1, icon -> itemicon, icon2 -> unk2, wear_slot
  -> equipslot, p18 -> unk3, no_trade -> trade, rent, p24 -> unk4,
  attack_type, damage -> weapondamage, range -> unk5, level_req, max_stack,
  max_hp, durability -> duration, class_mask -> classuse, race_mask ->
  raceuse, p4c -> procanim, lore, p58 -> unk6, p54 -> craft, name, desc.
  The stat pairs go to the columns itemPatternStats reads (0 str .. 27 AR);
  28, the fishing rating of poles and bait, to `fish`, which the server does
  not send yet. Any other stat index is dropped, and the report counts it.
    - an id we do not hold comes in under its own id;
    - an id we hold under the same name (a trailing class tag aside: Return
      Home's "Taunt" is Sandstorm's "Taunt (SK/PAL)") is the same item: its wire columns
      take the harvest's values (the price above all: Return Home's are 0),
      and model, color and weaponproc stay ours, since the wire does not carry
      them;
    - an id we hold under a DIFFERENT name comes in as ID_OFFSET + id.

spellpattern (every ability record)
  id -> spellid, level -> abilitylvl, cast_time -> casttime, power_cost ->
  power, icon_back -> iconColor and icon_front -> icon (as signed 32-bit, as
  writeAbilityRecord writes them), scope, cooldown -> recast, equip_req ->
  eqprequire, name, desc. The range is a raw float32 on the wire, which
  writeAbilityRecord builds from three columns: the high 16 bits in `range`,
  the two low bytes from unk3 and unk2 (each written as v << 1, so only an
  even byte under 128 can be held). An exact float is stored exactly; any
  other is reported, and stored with its low half cleared. The same id/name
  rules as itempattern apply; per-character fields (display index, bar slot,
  lockout) are not pattern fields and stay out.

npcitem (the catalogs)
  A merchant is a spell vendor when one of its wares has the scroll icon
  (-607649328); --all-merchants takes every merchant with a catalog. Each is
  matched to an NPC of ours in the same world, with the same name (case, a
  trailing '$' and backtick against apostrophe aside), within MATCH_RADIUS;
  failing that, a name one letter off (Return Home's "Merchant Liliac" is
  Sandstorm's "Merchant Lilac") within NEAR_RADIUS, which the report lists.
  A merchant with no such NPC is reported and left out. The catalog is keyed by
  npcs.npc_id (GetNPCItems): an NPC whose npc_id is 0 gets its own row id, as
  the Return Home import keys its own. Rows keep the merchant's order
  (listnumber) and the ware's quantity, condition and charges. An NPC that
  already has a catalog keeps it unless --replace-catalogs.

The report also counts the scrolls whose spell has an exact record here (by
name, a trailing class tag such as " (SHA, WIZ)" aside): those are the ones a
scribe could hand out. The server does not scribe a scroll yet.

Everything happens in one transaction and is logged in sandstorm_import_*
tables, so --undo takes it back out: updated rows get their old values back,
and the AUTOINCREMENT counters theirs, so the database is as it was.
Run it against a copy, or with the server stopped.

Usage:
    python3 tools/import_sandstorm_harvest.py --harvest DIR_OR_JSON [...]
    python3 tools/import_sandstorm_harvest.py --harvest DIR --apply
    python3 tools/import_sandstorm_harvest.py --undo
"""

import argparse
import datetime
import glob
import hashlib
import json
import math
import os
import re
import sqlite3
import struct
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
SERVER = os.path.dirname(HERE)
DEFAULT_DB = os.path.join(SERVER, "eqoa.db")

ID_OFFSET = 300000      # a harvested id held here by a different item or spell
MATCH_RADIUS = 15.0     # a merchant and our NPC of its name this close are one
NEAR_RADIUS = 25.0      # ... and one whose name is a letter off, this close
SCROLL_ICON = -607649328

# The ware's VIItem fields (net::dump::Item) and the itempattern column each
# fills, in writeItemPattern's order.
ITEM_FIELDS = [
    ("id", "patternid"), ("value", "itemcost"), ("p08", "unk1"), ("icon", "itemicon"),
    ("icon2", "unk2"), ("wear_slot", "equipslot"), ("p18", "unk3"), ("no_trade", "trade"),
    ("rent", "rent"), ("p24", "unk4"), ("attack_type", "attacktype"), ("damage", "weapondamage"),
    ("range", "unk5"), ("level_req", "levelreq"), ("max_stack", "maxstack"), ("max_hp", "maxhp"),
    ("durability", "duration"), ("class_mask", "classuse"), ("race_mask", "raceuse"),
    ("p4c", "procanim"), ("lore", "lore"), ("p58", "unk6"), ("p54", "craft"),
    ("name", "itemname"), ("desc", "itemdesc"),
]
# Stat index -> column (itemPatternStats), and 28 -> fish.
STAT_COLUMNS = {
    0: "str", 1: "sta", 2: "agi", 3: "dex", 4: "wis", 5: "intelligence", 6: "cha",
    8: "HPMAX", 10: "POWMAX", 12: "PoT", 13: "HoT", 14: "AC",
    22: "PR", 23: "DR", 24: "FR", 25: "CR", 26: "LR", 27: "AR", 28: "fish",
}
SPELL_COLS = ["spellid", "abilitylvl", "unk2", "unk3", "range", "casttime", "power",
              "iconColor", "icon", "scope", "recast", "eqprequire", "spellname", "spelldesc"]

LOG_DDL = """
CREATE TABLE IF NOT EXISTS sandstorm_import_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
-- Every change, in order: what --undo takes back, last first.
CREATE TABLE IF NOT EXISTS sandstorm_import_rows (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,     -- itempattern | spellpattern | npcitem | npcs
    key    INTEGER NOT NULL,  -- patternid, spellid, npcitem.itemid or npcs.id
    action TEXT NOT NULL,     -- inserted | updated | deleted | keyed
    old    TEXT               -- the row before (json), or the old npc_id
);
"""


def norm_name(name):
    """A name compared across tables: no '$' mark, no case, no padding, a
    backtick the same as an apostrophe."""
    n = (name or "").strip()
    if n.endswith("$"):
        n = n[:-1].rstrip()
    return n.replace("`", "'").lower()


def spell_name(name):
    """A scroll's or an ability's name as the other spells it: no trailing class
    tag ("Invisibility (SHA, WIZ)", "Taunt (SK/PAL)"), no bard '+'."""
    n = re.sub(r"\s*\((?:[A-Za-z]{2,3}(?:\s*[,/]\s*)?)+\)\s*$", "", name or "")
    return norm_name(n.replace("+", ""))


def one_letter_off(a, b):
    """Whether two names differ by one letter added, dropped or changed."""
    if a == b or abs(len(a) - len(b)) > 1:
        return False
    if len(a) > len(b):
        a, b = b, a
    i = 0
    while i < len(a) and a[i] == b[i]:
        i += 1
    return a[i + (len(a) == len(b)):] == b[i + 1:]


def signed32(v):
    v &= 0xFFFFFFFF
    return v - (1 << 32) if v >= 1 << 31 else v


def range_columns(r):
    """The float as writeAbilityRecord's (range, unk2, unk3), and whether that
    holds it exactly."""
    bits = struct.unpack("<I", struct.pack("<f", float(r)))[0]
    b0, b1 = bits & 0xFF, (bits >> 8) & 0xFF

    def held(b):
        return b % 2 == 0 and b < 128

    exact = held(b0) and held(b1)
    return (bits >> 16, b0 >> 1 if exact else 0, b1 >> 1 if exact else 0), exact


def sha256(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def columns(db, table):
    return [r[1] for r in db.execute(f"PRAGMA main.table_info({table})")]


def load_harvest(paths):
    """Every harvest JSON named (a directory means its *.json): the merchants,
    with the world of the character that saw them, and the ability records."""
    files = []
    for p in paths:
        files += sorted(glob.glob(os.path.join(p, "*.json"))) if os.path.isdir(p) else [p]
    if not files:
        sys.exit("no harvest files")
    merchants, abilities = [], []
    for f in files:
        with open(f) as fh:
            d = json.load(fh)
        if "merchants" not in d or "character" not in d:
            sys.exit(f"{f}: not an eqoa harvest file")
        ch = d["character"]
        for m in d["merchants"]:
            if m.get("wares"):
                merchants.append(dict(m, world=ch["world"], file=f, when=d.get("when", 0)))
        for a in d["abilities"] + [x["ability"] for x in d.get("learned", [])]:
            abilities.append(dict(a, cls=ch["class"], file=f))
    return files, merchants, abilities


def item_row(w):
    """The itempattern columns a ware fills, and the stat indices it drops."""
    row = {col: w[key] for key, col in ITEM_FIELDS}
    dropped = []
    stats = {c: None for c in STAT_COLUMNS.values()}
    for idx, val in w["stats"]:
        col = STAT_COLUMNS.get(idx)
        if col is None:
            dropped.append(idx)
        else:
            stats[col] = val
    row.update(stats)
    return row, dropped


def spell_row(a):
    (hi, unk2, unk3), exact = range_columns(a["range"])
    return {
        "spellid": a["id"], "abilitylvl": a["level"], "unk2": unk2, "unk3": unk3, "range": hi,
        "casttime": a["cast_time"], "power": a["power_cost"], "iconColor": signed32(a["icon_back"]),
        "icon": signed32(a["icon_front"]), "scope": a["scope"], "recast": a["cooldown"],
        "eqprequire": a["equip_req"], "spellname": a["name"], "spelldesc": a["desc"],
    }, exact


def plan_ids(db, table, key, name_col, rows):
    """Each harvested row's fate here: ('insert', id), ('update', id) for the
    same name under the same id, ('renumber', ID_OFFSET + id) for another."""
    plan = {}
    for k, row in rows.items():
        r = db.execute(f"SELECT {name_col} FROM main.{table} WHERE {key} = ?", (k,)).fetchone()
        if r is None:
            plan[k] = ("insert", k)
        elif spell_name(r[0]) == spell_name(row[name_col]):
            plan[k] = ("update", k)
        else:
            n = ID_OFFSET + k
            r2 = db.execute(f"SELECT {name_col} FROM main.{table} WHERE {key} = ?", (n,)).fetchone()
            if r2 is not None and spell_name(r2[0]) != spell_name(row[name_col]):
                sys.exit(f"{table} {k} and {n} are both held by other rows ({r[0]!r}, {r2[0]!r})")
            plan[k] = ("update" if r2 is not None else "insert", n)
    return plan


def plan(db, merchants, abilities, all_merchants):
    """What would change: the patterns, the spells, the merchants matched."""
    report = {"stats_dropped": {}, "range_inexact": [], "item_conflicts": 0, "spell_conflicts": 0}

    chosen = [m for m in merchants if all_merchants or any(w["icon"] == SCROLL_ICON for w in m["wares"])]
    # One harvest of an NPC: the latest, when two saw the same one.
    latest = {}
    for m in sorted(chosen, key=lambda m: m["when"]):
        latest[(m["world"], norm_name(m["name"]), round(m["pos"][0]), round(m["pos"][2]))] = m
    chosen = list(latest.values())

    items = {}
    for m in chosen:
        for w in m["wares"]:
            row, dropped = item_row(w)
            for i in dropped:
                report["stats_dropped"][i] = report["stats_dropped"].get(i, 0) + 1
            if w["id"] in items and items[w["id"]] != row:
                report["item_conflicts"] += 1
                continue
            items[w["id"]] = row

    spells = {}
    for a in abilities:
        row, exact = spell_row(a)
        if not exact:
            report["range_inexact"].append((a["id"], a["name"], a["range"]))
        if a["id"] in spells and spells[a["id"]] != row:
            report["spell_conflicts"] += 1
            continue
        spells[a["id"]] = row

    item_plan = plan_ids(db, "itempattern", "patternid", "itemname", items)
    spell_plan = plan_ids(db, "spellpattern", "spellid", "spellname", spells)

    # The merchants, matched to our NPCs.
    matched, unmatched, report["near_names"] = [], [], []
    for m in chosen:
        x, y, z = m["pos"]
        best = near = None
        for row in db.execute(
                "SELECT id, npc_name, x_coord, y_coord, z_coord, npc_id FROM main.npcs "
                "WHERE world = ? AND x_coord BETWEEN ? AND ? AND z_coord BETWEEN ? AND ?",
                (m["world"], x - NEAR_RADIUS, x + NEAR_RADIUS, z - NEAR_RADIUS, z + NEAR_RADIUS)):
            d = math.dist((x, y, z), (row[2], row[3], row[4]))
            if norm_name(row[1]) == norm_name(m["name"]):
                if d <= MATCH_RADIUS and (best is None or d < best[0]):
                    best = (d, row)
            elif one_letter_off(norm_name(row[1]), norm_name(m["name"])) and d <= NEAR_RADIUS:
                if near is None or d < near[0]:
                    near = (d, row)
        if best is None and near is not None:
            best = near
            report["near_names"].append((m["name"], near[1][1], round(near[0], 1)))
        if best is None:
            unmatched.append(m)
        else:
            matched.append((m, best[1]))
    # Two merchants of the harvest on one NPC of ours: the nearer keeps it.
    by_npc = {}
    for m, row in matched:
        d = math.dist(m["pos"], (row[2], row[3], row[4]))
        if row[0] not in by_npc or d < by_npc[row[0]][0]:
            by_npc[row[0]] = (d, m, row)
    matched = [(m, row) for _, m, row in by_npc.values()]

    # The scrolls whose spell has a record (by name): one of the harvest's,
    # exact, or only one this database already held.
    harvested = {spell_name(r["spellname"]) for r in spells.values()}
    held = {spell_name(r[0]) for r in db.execute("SELECT spellname FROM main.spellpattern")} - harvested
    scrolls = {w["id"]: w for m in chosen for w in m["wares"] if w["icon"] == SCROLL_ICON}
    report["scrolls"] = len(scrolls)
    report["scrolls_exact"] = sum(1 for w in scrolls.values() if spell_name(w["name"]) in harvested)
    report["scrolls_held"] = sum(1 for w in scrolls.values() if spell_name(w["name"]) in held)
    return items, item_plan, spells, spell_plan, matched, unmatched, report


def log(db, tbl, key, action, old=None):
    db.execute("INSERT INTO sandstorm_import_rows (tbl, key, action, old) VALUES (?, ?, ?, ?)",
               (tbl, key, action, old))


def write_rows(db, table, key, rows, plan_):
    """Insert or update each row by its plan, logging the old row of an update."""
    tcols = columns(db, table)
    counts = {"insert": 0, "update": 0, "renumber": 0}
    for k, row in rows.items():
        action, dst = plan_[k]
        row = dict(row, **{key: dst})
        if dst != k:
            counts["renumber"] += 1
        old = db.execute(f"SELECT * FROM main.{table} WHERE {key} = ?", (dst,)).fetchone()
        if old is None:
            full = {c: None for c in tcols}
            if table == "itempattern":
                full.update(model=0, color=0)
            full.update(row)
            db.execute(f"INSERT INTO main.{table} ({', '.join(tcols)}) VALUES ({', '.join('?' * len(tcols))})",
                       [full[c] for c in tcols])
            log(db, table, dst, "inserted")
            counts["insert"] += 1
        else:
            if table == "itempattern" and row.get("fish") is None:
                row.pop("fish", None)   # a pole's rating stays when the ware has none
            log(db, table, dst, "updated", json.dumps(dict(zip(tcols, old))))
            sets = [c for c in row if c != key]
            db.execute(f"UPDATE main.{table} SET {', '.join(f'{c} = ?' for c in sets)} WHERE {key} = ?",
                       [row[c] for c in sets] + [dst])
            counts["update"] += 1
    return counts


def apply(db, files, items, item_plan, spells, spell_plan, matched, replace):
    cur = db.cursor()
    cur.executescript(LOG_DDL)
    if cur.execute("SELECT count(*) FROM sandstorm_import_rows").fetchone()[0]:
        sys.exit("this database already holds a harvest import (sandstorm_import_rows); --undo it first")
    for table, want in (("itempattern", [c for _, c in ITEM_FIELDS] + list(STAT_COLUMNS.values())),
                        ("spellpattern", SPELL_COLS)):
        missing = [c for c in want if c not in columns(db, table)]
        if missing:
            sys.exit(f"main.{table} lacks {missing}")

    db.execute("BEGIN IMMEDIATE")
    try:
        # The AUTOINCREMENT counters, which an insert moves on: --undo puts them back.
        seqs = dict(db.execute("SELECT name, seq FROM sqlite_sequence WHERE name IN ('itempattern', 'spellpattern', 'npcitem')").fetchall())
        item_counts = write_rows(db, "itempattern", "patternid", items, item_plan)
        spell_counts = write_rows(db, "spellpattern", "spellid", spells, spell_plan)
        catalogs = {"written": 0, "kept": 0, "replaced": 0, "rows": 0}
        for m, row in matched:
            npc_row, npc_id = row[0], row[5]
            key = npc_id
            if not npc_id:
                key = npc_row
                db.execute("UPDATE main.npcs SET npc_id = ? WHERE id = ?", (key, npc_row))
                log(db, "npcs", npc_row, "keyed", str(npc_id))
            existing = db.execute("SELECT * FROM main.npcitem WHERE npcid = ?", (key,)).fetchall()
            if existing and not replace:
                catalogs["kept"] += 1
                continue
            if existing:
                icols = columns(db, "npcitem")
                for old in existing:
                    log(db, "npcitem", old[0], "deleted", json.dumps(dict(zip(icols, old))))
                db.execute("DELETE FROM main.npcitem WHERE npcid = ?", (key,))
                catalogs["replaced"] += 1
            for n, w in enumerate(m["wares"]):
                c = db.execute(
                    "INSERT INTO main.npcitem (npcid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber) "
                    "VALUES (?, ?, ?, ?, ?, -1, 1, ?)",
                    (key, w["quantity"], w["cur_hp"], w["charges"], item_plan[w["id"]][1], n))
                log(db, "npcitem", c.lastrowid, "inserted")
                catalogs["rows"] += 1
            catalogs["written"] += 1
        meta = {
            "harvest": json.dumps([os.path.basename(f) for f in files]),
            "harvest_sha256": json.dumps({os.path.basename(f): sha256(f) for f in files}),
            "applied_at": datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds"),
            "itempattern": json.dumps(item_counts),
            "spellpattern": json.dumps(spell_counts),
            "catalogs": json.dumps(catalogs),
            "sqlite_sequence": json.dumps(seqs),
        }
        for k, v in meta.items():
            db.execute("INSERT OR REPLACE INTO sandstorm_import_meta VALUES (?, ?)", (k, v))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return item_counts, spell_counts, catalogs


def undo(db, force):
    if not db.execute("SELECT count(*) FROM sqlite_master WHERE name = 'sandstorm_import_rows'").fetchone()[0]:
        sys.exit("no harvest import to undo here")
    owned = {
        "charinventory": db.execute(
            "SELECT count(*) FROM charinventory WHERE patternid IN "
            "(SELECT key FROM sandstorm_import_rows WHERE tbl = 'itempattern' AND action = 'inserted')").fetchone()[0],
        "spells": db.execute(
            "SELECT count(*) FROM spells WHERE spellid IN "
            "(SELECT key FROM sandstorm_import_rows WHERE tbl = 'spellpattern' AND action = 'inserted')").fetchone()[0],
    }
    if any(owned.values()) and not force:
        sys.exit(f"players hold imported items/spells ({owned}); --force to take them out anyway")
    keys = {"itempattern": "patternid", "spellpattern": "spellid", "npcitem": "itemid", "npcs": "id"}
    db.execute("BEGIN IMMEDIATE")
    try:
        for tbl, key, action, old in db.execute(
                "SELECT tbl, key, action, old FROM sandstorm_import_rows ORDER BY seq DESC").fetchall():
            k = keys[tbl]
            if action == "inserted":
                db.execute(f"DELETE FROM main.{tbl} WHERE {k} = ?", (key,))
            elif action == "updated":
                row = json.loads(old)
                sets = [c for c in row if c != k]
                db.execute(f"UPDATE main.{tbl} SET {', '.join(f'{c} = ?' for c in sets)} WHERE {k} = ?",
                           [row[c] for c in sets] + [key])
            elif action == "deleted":
                row = json.loads(old)
                db.execute(f"INSERT INTO main.{tbl} ({', '.join(row)}) VALUES ({', '.join('?' * len(row))})",
                           list(row.values()))
            elif action == "keyed":
                db.execute("UPDATE main.npcs SET npc_id = ? WHERE id = ?", (int(old), key))
        r = db.execute("SELECT value FROM sandstorm_import_meta WHERE key = 'sqlite_sequence'").fetchone()
        if r is not None:
            seqs = json.loads(r[0])
            for t in ("itempattern", "spellpattern", "npcitem"):
                if t in seqs:
                    db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = ?", (seqs[t], t))
                else:
                    db.execute("DELETE FROM sqlite_sequence WHERE name = ?", (t,))
        db.execute("DROP TABLE sandstorm_import_rows")
        db.execute("DROP TABLE sandstorm_import_meta")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print("undone")


def print_report(db, items, item_plan, spells, spell_plan, matched, unmatched, report):
    def tally(p):
        t = {}
        for action, dst in p.values():
            t[action] = t.get(action, 0) + 1
        t["renumbered"] = sum(1 for k, (_, dst) in p.items() if dst != k)
        return t
    print(f"item patterns: {len(items)} {tally(item_plan)}")
    print(f"spell patterns: {len(spells)} {tally(spell_plan)}")
    for table, key, name_col, rows, p in (("itempattern", "patternid", "itemname", items, item_plan),
                                         ("spellpattern", "spellid", "spellname", spells, spell_plan)):
        for k, (_, dst) in sorted(p.items()):
            if dst != k:
                (held,) = db.execute(f"SELECT {name_col} FROM main.{table} WHERE {key} = ?", (k,)).fetchone()
                print(f"    {table} {k} is our {held!r}: {rows[k][name_col]!r} comes in as {dst}")
    print(f"merchants: {len(matched)} matched to our NPCs, {len(unmatched)} not in this database")
    for m in sorted(unmatched, key=lambda m: m["name"])[:40]:
        print(f"    not here: {m['name']} at {[round(v) for v in m['pos']]} (world {m['world']})")
    if len(unmatched) > 40:
        print(f"    ... and {len(unmatched) - 40} more")
    for theirs, ours, d in report["near_names"]:
        print(f"    matched by a near name: {theirs!r} is our {ours!r}, {d} away")
    print(f"catalog rows: {sum(len(m['wares']) for m, _ in matched)}")
    print(f"scrolls: {report['scrolls']}; with the harvest's exact spell record: {report['scrolls_exact']}; "
          f"with only a record already here: {report['scrolls_held']}")
    if report["stats_dropped"]:
        print(f"stat indices with no column, dropped: {report['stats_dropped']}")
    if report["range_inexact"]:
        print(f"ranges writeAbilityRecord cannot hold exactly (low half cleared): {report['range_inexact']}")
    if report["item_conflicts"] or report["spell_conflicts"]:
        print(f"one id with two different records (the first kept): items {report['item_conflicts']}, spells {report['spell_conflicts']}")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB, help="the database to import into (default: eqoa.db beside tools/)")
    ap.add_argument("--harvest", nargs="+", help="eqoa harvest JSON files, or directories of them")
    ap.add_argument("--all-merchants", action="store_true", help="every harvested merchant's catalog, not only the spell vendors'")
    ap.add_argument("--replace-catalogs", action="store_true", help="replace a matched NPC's existing catalog (default: keep it)")
    ap.add_argument("--apply", action="store_true", help="write the import (default: report only)")
    ap.add_argument("--undo", action="store_true", help="take a previous import back out")
    ap.add_argument("--force", action="store_true", help="--undo even if players hold imported items or spells")
    args = ap.parse_args()

    writing = args.apply or args.undo
    target = os.path.abspath(args.db) if writing else f"file:{os.path.abspath(args.db)}?mode=ro"
    db = sqlite3.connect(target, isolation_level=None, uri=True)
    db.execute("PRAGMA foreign_keys = OFF")
    if args.undo:
        undo(db, args.force)
        return
    if not args.harvest:
        ap.error("--harvest is required (or --undo)")
    files, merchants, abilities = load_harvest(args.harvest)
    planned = plan(db, merchants, abilities, args.all_merchants)
    print_report(db, *planned)
    if args.apply:
        items, item_plan, spells, spell_plan, matched, _, _ = planned
        ic, sc, cc = apply(db, files, items, item_plan, spells, spell_plan, matched, args.replace_catalogs)
        print(f"applied to {args.db}: itempattern {ic}, spellpattern {sc}, catalogs {cc}")
    else:
        print("(report only: --apply to write it)")


if __name__ == "__main__":
    main()
