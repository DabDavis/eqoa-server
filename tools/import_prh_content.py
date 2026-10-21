#!/usr/bin/env python3
"""
Import the static content of a Return Home server database into eqoa.db, in
THIS server's format.

The source is a SQLite database in this server's schema whose static tables are
Return Home's EQOA_Master.sql (EQOAReturnHome/EQOAGameServer), such as the
2026-10-20 server export. Only its static tables are read. Accounts, characters
and everything else a player owns are never touched, so this adds content to a
running database rather than replacing one.

Return Home's tables share our column NAMES but not always their MEANING, and
the dump carries damage of its own (claude-notes/jib-server-export-20261021.md).
Every row is converted on the way in.

npcs
  npc_type    := the source's npc_id & 0xFFFF. Return Home's loader (GetAllNPCs)
                 reads the NPC flags out of npc_id; ours reads them out of
                 npc_type, and statesync.go puts its 16 bits on the wire (our
                 4224 is their 1052800). The source's npc_type (0-14) has no
                 column here and is dropped.
  npc_id      := the row's own new id: the key npcitem names (GetNPCItems).
                 Return Home keys npcitem by row id.
  id          := ID_OFFSET + the source id. That is traceable, and clear of our
                 rows, character serverids and the #spawn counter.
  robe_style, race '25' := '255'. Both are varchar(2) in Return Home's MySQL,
                 which cut 255 (no robe) to 25, and CLIENT's
                 VICreatureEntity::ApplyLook switches a robe on for any value >= 0.
  y > 65536   := y - 131072. These are negative heights stored unsigned (2^17 is
                 the 24-bit position's range); every such row is a cave, dungeon
                 or underwater creature, and nothing else is below 0.
  size < 0.01 := 1.0. The dump wrote these as denormals ('5.9e-39'), so the low
                 bits are gone; 1.0 is the model's own scale.
  facing      := facing mod 256, because a few are negative.
  npc_name    : a trailing '$' (Return Home's mark on its generic creature
                 names) is dropped, since CLIENT shows the name as it stands. The
                 name is cut to 23 bytes: the wire field is 24, with no room for
                 a NUL.
  dt_kill     := NULL, because '0000-00-00' is not a date.
  The columns the server does not read (zone, unk_*, hp, pd_icon) are copied as
  they are.

  Held, not imported (the reason is in prh_import_npcs):
    - a row with no name, or whose model is not one of CHAR.ESF's CSprites
      (--models). A model the disc does not have cannot be drawn.
    - the same NPC as one of ours, ours being the rows whose flags, robe and
      race were checked against the wire. That is: the same name (case and '$'
      aside) in the same world within DUP_RADIUS; a name each table holds
      exactly once in that world (a named NPC placed elsewhere); or a different
      name with the same model within SAME_SPOT units.
    - a 'Gatekeeper ' or 'Harbormaster ' that is not ours. Our server makes
      every such NPC a cross-world travel destination (entity.IsWarpNPC), and
      Return Home's are gate guards.
    - an exact copy of another source row.

itempattern
  A pattern id we already hold under the same name is the same item: ours is
  kept, and the source's rows that name it point at ours. A held id with a
  DIFFERENT item comes in as ID_OFFSET + id. Every other pattern keeps its own
  id. The source's itemcost 0 means "price unknown", and merchants sell at
  that price.

spellpattern
  Handled as itempattern. Six of our ids name different spells there (179 is
  our Power Twist and their Kick).

npcitem (--no-catalogs leaves it out: every item it names is priced 0)
  Re-keyed to the imported NPC. When the source NPC was held as one of ours,
  the catalog goes to OUR row, whose npc_id is set to its own id if it was 0.
  That is how our existing merchants get their stock.

defaultcharinventory, defaultspells, charactermodel
  Copied with their ids re-pointed, and only into an empty table. Our server
  does not read these three today.

defaultcharacter
  Untouched: both tables hold the same 75 rows, and ours are the ones the
  creation page serves.

Everything happens in one transaction and is logged in prh_import_* tables, so
--undo takes it back out. Run it against a copy, or with the server stopped.

Usage:
    python3 tools/import_prh_content.py --src export.db --models char_sprites.json
    python3 tools/import_prh_content.py --src export.db --models char_sprites.json --apply
    python3 tools/import_prh_content.py --undo

Make --models with go-eqoa-esf-tools from the disc's own CHAR.ESF:
    esfextract -sprites -json CHAR.ESF > char_sprites.json
"""

import argparse
import collections
import datetime
import hashlib
import json
import math
import os
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
SERVER = os.path.dirname(HERE)
DEFAULT_DB = os.path.join(SERVER, "eqoa.db")

ID_OFFSET = 100000      # imported npcs.id, and renumbered pattern/spell ids
DUP_RADIUS = 50.0       # the same name this close (x/z) is the same spawn
SAME_SPOT = 1.0         # a different name with the same model this close, too
NAME_BYTES = 23         # the wire's name field is 24 bytes, NUL included
Y_WRAP = 131072.0       # 2^24 / 128: the position's range
TRAVEL_PREFIXES = ("Gatekeeper ", "Harbormaster ")

# The static tables read, and the columns each must have. The source is
# checked against these before anything is read.
NPC_COLS = [
    "id", "npc_name", "zone", "unk_a", "unk_b", "x_coord", "y_coord", "z_coord",
    "facing", "world", "dt_kill", "hp", "hp_percent", "modelid", "size",
    "unk_d", "unk_f", "unk_g", "primary_weapon", "secondary_weapon", "shield",
    "unk_h", "torso", "forearm", "gloves", "legs", "feet", "head",
    "hair_color", "hair_length", "hair_style", "face", "robe_style", "race",
    "unk_j", "npc_level", "npc_type", "npc_id", "unk_k", "unk_i", "pd_icon",
    "unk_m", "unk_n", "unk_o",
]

LOG_DDL = """
CREATE TABLE IF NOT EXISTS prh_import_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
-- One row per source NPC: imported, the same as one of ours, or held.
CREATE TABLE IF NOT EXISTS prh_import_npcs (
    src_id   INTEGER PRIMARY KEY,
    npc_name TEXT,
    world    INTEGER,
    status   TEXT NOT NULL,   -- 'imported' | 'ours' | 'held'
    reason   TEXT,            -- how it matched ours, or why it was held
    dst_id   INTEGER,         -- npcs.id created, or the row of ours it is
    fixes    TEXT             -- the conversions applied: 'robe,y,size,$,...'
);
-- Patterns and spells: 'imported' (dst_id the new id) or 'ours' (dst_id ours).
-- The other tables: every row inserted, by its key, for --undo.
CREATE TABLE IF NOT EXISTS prh_import_ids (
    tbl    TEXT NOT NULL,
    src_id INTEGER,
    dst_id INTEGER NOT NULL,
    status TEXT NOT NULL
);
-- Rows of ours whose npc_id was set so a catalog could name them.
CREATE TABLE IF NOT EXISTS prh_import_keyed (
    npc_row    INTEGER PRIMARY KEY,
    old_npc_id INTEGER
);
"""


def norm_name(name):
    """A name compared across the two tables: no '$' mark, no case, no padding,
    and a backtick the same as an apostrophe (ours spell Neriak's names both
    ways, Return Home's mostly with the apostrophe)."""
    n = (name or "").strip()
    if n.endswith("$"):
        n = n[:-1].rstrip()
    return n.replace("`", "'").lower()


def clean_name(name):
    """The name as it is stored: the '$' mark dropped, at most NAME_BYTES."""
    n = name or ""
    fixes = []
    if n.rstrip().endswith("$"):
        n = n.rstrip()[:-1].rstrip()
        fixes.append("$")
    b = n.encode("latin-1", "replace")
    if len(b) > NAME_BYTES:
        n = b[:NAME_BYTES].decode("latin-1")
        fixes.append("name-cut")
    return n, fixes


def load_models(path):
    """CHAR.ESF's CSprite DictIDs, as unsigned 32-bit values. Takes esfextract's
    `-sprites -json` output, or a plain list (one id a line, decimal or 0x hex)."""
    with open(path) as f:
        text = f.read()
    try:
        entries = json.loads(text)
    except ValueError:
        entries = None
    if isinstance(entries, list):
        ids = {e["dict_id"] & 0xFFFFFFFF for e in entries if e.get("type") == "CSprite"}
    else:
        ids = {int(t, 0) & 0xFFFFFFFF for t in text.split() if t.strip()}
    if not ids:
        sys.exit(f"{path}: no model ids in it")
    return ids


def columns(db, schema, table):
    return [r[1] for r in db.execute(f"PRAGMA {schema}.table_info({table})")]


def sha256(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


class Grid:
    """Our NPCs by world and coarse cell, for the near-spot tests."""

    def __init__(self, cell):
        self.cell = cell
        self.rows = collections.defaultdict(list)

    def add(self, world, x, z, row):
        self.rows[(world, int(x // self.cell), int(z // self.cell))].append(row)

    def near(self, world, x, z, radius):
        cx, cz = int(x // self.cell), int(z // self.cell)
        reach = int(radius // self.cell) + 1
        for i in range(cx - reach, cx + reach + 1):
            for j in range(cz - reach, cz + reach + 1):
                for row in self.rows.get((world, i, j), ()):
                    if math.hypot(row["x"] - x, row["z"] - z) <= radius:
                        yield row


def plan_npcs(db, models):
    """Decide every source NPC, without writing anything."""
    ours = [dict(zip(("id", "name", "world", "x", "z", "model", "npc_id"), r)) for r in db.execute(
        "SELECT id, npc_name, world, x_coord, z_coord, modelid, npc_id FROM main.npcs")]
    by_name = collections.defaultdict(list)
    grid = Grid(10.0)
    for r in ours:
        r["model"] &= 0xFFFFFFFF
        by_name[(norm_name(r["name"]), r["world"])].append(r)
        grid.add(r["world"], r["x"], r["z"], r)

    src = [dict(zip(NPC_COLS, r)) for r in db.execute(
        f"SELECT {', '.join(NPC_COLS)} FROM src.npcs ORDER BY id")]
    src_count = collections.Counter((norm_name(r["npc_name"]), r["world"]) for r in src)

    seen = {}
    plans = []
    for r in src:
        p = {"src": r, "status": None, "kind": "", "reason": None, "dst_id": None, "fixes": []}
        plans.append(p)
        key = (norm_name(r["npc_name"]), r["world"])
        y = r["y_coord"] if r["y_coord"] <= Y_WRAP / 2 else r["y_coord"] - Y_WRAP
        model = (r["modelid"] or 0) & 0xFFFFFFFF

        if not key[0]:
            p["status"], p["kind"], p["reason"] = "held", "no name", "no name"
            continue

        mates = by_name.get(key, [])
        if mates:
            best = min(mates, key=lambda m: math.hypot(m["x"] - r["x_coord"], m["z"] - r["z_coord"]))
            d = math.hypot(best["x"] - r["x_coord"], best["z"] - r["z_coord"])
            if d <= DUP_RADIUS:
                p["status"], p["kind"], p["dst_id"] = "ours", "same name nearby", best["id"]
                p["reason"] = f"same name, {d:.0f} away"
                continue
            if len(mates) == 1 and src_count[key] == 1:
                p["status"], p["kind"], p["dst_id"] = "ours", "named NPC placed elsewhere", best["id"]
                p["reason"] = f"named NPC, placed {d:.0f} away"
                continue
        spot = [m for m in grid.near(r["world"], r["x_coord"], r["z_coord"], SAME_SPOT) if m["model"] == model]
        if spot:
            p["status"], p["kind"], p["dst_id"] = "ours", "same model at the same spot", spot[0]["id"]
            p["reason"] = f"same model at the same spot as {spot[0]['name']!r}"
            continue

        if model not in models:
            p["status"], p["kind"], p["reason"] = "held", "model not on the disc", f"model {model:#010x} is not a CHAR.ESF CSprite"
            continue
        name, _ = clean_name(r["npc_name"])
        if name.startswith(TRAVEL_PREFIXES):
            p["status"], p["kind"], p["reason"] = "held", "travel name", "travel name: would become a cross-world route"
            continue

        exact = (key, round(r["x_coord"], 1), round(y, 1), round(r["z_coord"], 1), model, r["npc_level"])
        if exact in seen:
            p["status"], p["kind"], p["reason"] = "held", "copy of another source row", f"copy of source row {seen[exact]}"
            continue
        seen[exact] = r["id"]
        p["status"] = "imported"
        p["dst_id"] = ID_OFFSET + r["id"]
    return plans


def convert_npc(r, dst_id):
    """A source row in our format, and the conversions it took."""
    out = dict(r)
    fixes = []
    out["id"] = dst_id
    out["npc_id"] = dst_id
    out["npc_type"] = (r["npc_id"] or 0) & 0xFFFF
    name, name_fixes = clean_name(r["npc_name"])
    out["npc_name"] = name
    fixes += name_fixes
    if str(r["robe_style"] or "").strip() == "25":
        out["robe_style"] = "255"
        fixes.append("robe")
    if str(r["race"] or "").strip() == "25":
        out["race"] = "255"
        fixes.append("race")
    if r["y_coord"] is not None and r["y_coord"] > Y_WRAP / 2:
        out["y_coord"] = r["y_coord"] - Y_WRAP
        fixes.append("y")
    if r["size"] is None or r["size"] < 0.01:
        out["size"] = 1.0
        fixes.append("size")
    if r["facing"] is not None and not 0 <= r["facing"] < 256:
        out["facing"] = r["facing"] % 256
        fixes.append("facing")
    out["modelid"] = (r["modelid"] or 0) & 0xFFFFFFFF
    out["dt_kill"] = None
    return out, fixes


def plan_ids(db, table, key, name_col):
    """Patterns or spells: key -> (status, dst key). 'ours' maps a held id to our
    row of the same name; a held id naming something else moves to ID_OFFSET+id."""
    ours = {k: n for k, n in db.execute(f"SELECT {key}, {name_col} FROM main.{table}")}
    plan = {}
    for k, n in db.execute(f"SELECT {key}, {name_col} FROM src.{table} ORDER BY {key}"):
        if not (n or "").strip():
            plan[k] = ("held", None)
        elif k in ours:
            if (ours[k] or "").strip().lower() == n.strip().lower():
                plan[k] = ("ours", k)
            else:
                plan[k] = ("renumbered", ID_OFFSET + k)
        else:
            plan[k] = ("imported", k)
    return plan


def report(plans, items, spells):
    by = collections.Counter((p["status"], p["kind"]) for p in plans)
    print(f"npcs: {len(plans)} in the source")
    for (status, reason), n in sorted(by.items(), key=lambda t: -t[1]):
        print(f"  {status:9s} {n:6d}  {reason}")
    worlds = collections.Counter(p["src"]["world"] for p in plans if p["status"] == "imported")
    print("  imported by world:", dict(sorted(worlds.items())))
    fixes = collections.Counter(f for p in plans if p["status"] == "imported" for f in p["fixes"])
    print("  conversions on imported rows:", dict(fixes))
    for label, plan in (("itempattern", items), ("spellpattern", spells)):
        c = collections.Counter(s for s, _ in plan.values())
        print(f"{label}: {len(plan)} in the source:", dict(c))


def apply(db, plans, items, spells, src_path, catalogs=True):
    cur = db.cursor()
    cur.executescript(LOG_DDL)
    if cur.execute("SELECT count(*) FROM prh_import_npcs").fetchone()[0]:
        sys.exit("this database already holds an import (prh_import_npcs); --undo it first")
    cols = columns(db, "main", "npcs")
    if cols != NPC_COLS:
        sys.exit(f"main.npcs columns differ from this tool's: {cols}")

    db.execute("BEGIN IMMEDIATE")
    try:
        # Patterns and spells first: the catalogs and defaults name them.
        for table, key, plan in (("itempattern", "patternid", items), ("spellpattern", "spellid", spells)):
            tcols = columns(db, "main", table)
            if tcols != columns(db, "src", table):
                sys.exit(f"{table}: the source's columns differ from ours")
            for k, (status, dst) in plan.items():
                if status in ("imported", "renumbered"):
                    row = list(db.execute(f"SELECT * FROM src.{table} WHERE {key} = ?", (k,)).fetchone())
                    row[tcols.index(key)] = dst
                    db.execute(f"INSERT INTO main.{table} ({', '.join(tcols)}) VALUES ({', '.join('?' * len(tcols))})", row)
                if status != "held":
                    db.execute("INSERT INTO prh_import_ids VALUES (?, ?, ?, ?)", (table, k, dst, status))

        # The NPCs.
        for p in plans:
            r = p["src"]
            if p["status"] == "imported":
                out, p["fixes"] = convert_npc(r, p["dst_id"])
                db.execute(f"INSERT INTO main.npcs ({', '.join(NPC_COLS)}) VALUES ({', '.join('?' * len(NPC_COLS))})",
                           [out[c] for c in NPC_COLS])
            db.execute("INSERT INTO prh_import_npcs VALUES (?, ?, ?, ?, ?, ?, ?)",
                       (r["id"], r["npc_name"], r["world"], p["status"], p["reason"], p["dst_id"], ",".join(p["fixes"])))

        # The catalogs: to the imported NPC, or to the row of ours it is.
        target = {}      # source npc id -> npcitem key
        claimed = set()  # rows of ours already given a catalog
        for p in sorted(plans, key=lambda p: p["src"]["id"]):
            sid = p["src"]["id"]
            if p["status"] == "imported":
                target[sid] = p["dst_id"]
            elif p["status"] == "ours":
                target[sid] = ("ours", p["dst_id"])
        rows = db.execute("SELECT itemid, npcid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber "
                          "FROM src.npcitem ORDER BY npcid, listnumber, itemid").fetchall() if catalogs else []
        has_items = {r[1] for r in rows}
        dropped = collections.Counter()
        for sid in sorted(has_items):
            t = target.get(sid)
            if isinstance(t, tuple):
                row_id = t[1]
                if row_id in claimed:
                    target[sid] = None   # a second source copy of the same NPC of ours
                    continue
                (old,) = db.execute("SELECT npc_id FROM main.npcs WHERE id = ?", (row_id,)).fetchone()
                if not old:
                    db.execute("UPDATE main.npcs SET npc_id = ? WHERE id = ?", (row_id, row_id))
                    db.execute("INSERT INTO prh_import_keyed VALUES (?, ?)", (row_id, old))
                    key = row_id
                else:
                    key = old
                    if db.execute("SELECT count(*) FROM main.npcitem WHERE npcid = ?", (key,)).fetchone()[0]:
                        target[sid] = None   # it has a catalog of its own already
                        continue
                claimed.add(row_id)
                target[sid] = key
        for itemid, sid, stack, rhp, charge, pattern, eloc, loc, listn in rows:
            key = target.get(sid)
            status, dst_pattern = items.get(pattern, ("held", None))
            if key is None or status == "held":
                dropped["no NPC" if key is None else "no pattern"] += 1
                continue
            c = db.execute("INSERT INTO main.npcitem (npcid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber) "
                           "VALUES (?, ?, ?, ?, ?, ?, ?, ?)", (key, stack, rhp, charge, dst_pattern, eloc, loc, listn))
            db.execute("INSERT INTO prh_import_ids VALUES ('npcitem', ?, ?, 'imported')", (itemid, c.lastrowid))

        # Starting items, starting spells and the race models: only into an empty table.
        def copy_if_empty(table, key, fix):
            if db.execute(f"SELECT count(*) FROM main.{table}").fetchone()[0]:
                print(f"{table}: not empty here, left alone")
                return
            tcols = columns(db, "main", table)
            for row in db.execute(f"SELECT {', '.join(tcols)} FROM src.{table}").fetchall():
                row = fix(dict(zip(tcols, row)))
                if row is None:
                    continue
                c = db.execute(f"INSERT INTO main.{table} ({', '.join(tcols)}) VALUES ({', '.join('?' * len(tcols))})",
                               [row[c] for c in tcols])
                db.execute("INSERT INTO prh_import_ids VALUES (?, NULL, ?, 'imported')",
                           (table, row[key] if key == "modelid" else c.lastrowid))

        def fix_item(r):
            status, dst = items.get(r["patternid"], ("held", None))
            if status == "held":
                return None
            r["patternid"] = dst
            r["itemid"] = None
            return r

        def fix_spell(r):
            status, dst = spells.get(r["spellid"], ("held", None))
            if status == "held":
                return None
            r["spellid"] = dst
            r["def_spell_id"] = None
            return r

        def fix_model(r):
            r["modelid"] &= 0xFFFFFFFF
            return r

        copy_if_empty("defaultcharinventory", "itemid", fix_item)
        copy_if_empty("defaultspells", "def_spell_id", fix_spell)
        copy_if_empty("charactermodel", "modelid", fix_model)

        meta = {
            "source": os.path.basename(src_path),
            "source_sha256": sha256(src_path),
            "applied_at": datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds"),
            "npcitem_dropped": json.dumps(dict(dropped)),
            "catalogs": "imported" if catalogs else "left out (--no-catalogs)",
        }
        for k, v in meta.items():
            db.execute("INSERT OR REPLACE INTO prh_import_meta VALUES (?, ?)", (k, v))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return dropped


def undo(db, force):
    if not db.execute("SELECT count(*) FROM sqlite_master WHERE name = 'prh_import_npcs'").fetchone()[0]:
        sys.exit("no import to undo here")
    owned = {
        "charinventory": db.execute(
            "SELECT count(*) FROM charinventory WHERE patternid IN "
            "(SELECT dst_id FROM prh_import_ids WHERE tbl = 'itempattern' AND status IN ('imported', 'renumbered'))").fetchone()[0],
        "spells": db.execute(
            "SELECT count(*) FROM spells WHERE spellid IN "
            "(SELECT dst_id FROM prh_import_ids WHERE tbl = 'spellpattern' AND status IN ('imported', 'renumbered'))").fetchone()[0],
    }
    if any(owned.values()) and not force:
        sys.exit(f"players hold imported patterns/spells ({owned}); --force to take them out anyway")
    db.execute("BEGIN IMMEDIATE")
    try:
        for table, key in (("npcitem", "itemid"), ("defaultcharinventory", "itemid"),
                           ("defaultspells", "def_spell_id"), ("charactermodel", "modelid")):
            db.execute(f"DELETE FROM main.{table} WHERE {key} IN (SELECT dst_id FROM prh_import_ids WHERE tbl = ?)", (table,))
        db.execute("DELETE FROM main.npcs WHERE id IN (SELECT dst_id FROM prh_import_npcs WHERE status = 'imported')")
        db.execute("UPDATE main.npcs SET npc_id = (SELECT old_npc_id FROM prh_import_keyed k WHERE k.npc_row = npcs.id) "
                   "WHERE id IN (SELECT npc_row FROM prh_import_keyed)")
        for table, key in (("itempattern", "patternid"), ("spellpattern", "spellid")):
            db.execute(f"DELETE FROM main.{table} WHERE {key} IN (SELECT dst_id FROM prh_import_ids "
                       f"WHERE tbl = ? AND status IN ('imported', 'renumbered'))", (table,))
        for t in ("prh_import_meta", "prh_import_npcs", "prh_import_ids", "prh_import_keyed"):
            db.execute(f"DROP TABLE {t}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print("undone")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB, help="the database to import into (default: eqoa.db beside tools/)")
    ap.add_argument("--src", help="the Return Home database to read (opened read-only)")
    ap.add_argument("--models", help="CHAR.ESF's sprites: esfextract -sprites -json output, or a list of ids")
    ap.add_argument("--apply", action="store_true", help="write the import (default: report only)")
    ap.add_argument("--undo", action="store_true", help="take a previous import back out")
    ap.add_argument("--force", action="store_true", help="--undo even if players hold imported items or spells")
    ap.add_argument("--no-catalogs", action="store_true",
                    help="leave the merchant catalogs and loot (npcitem) out: every item in them is priced 0")
    args = ap.parse_args()

    writing = args.apply or args.undo
    target = os.path.abspath(args.db) if writing else f"file:{os.path.abspath(args.db)}?mode=ro"
    db = sqlite3.connect(target, isolation_level=None, uri=True)
    db.execute("PRAGMA foreign_keys = OFF")
    if args.undo:
        undo(db, args.force)
        return
    if not args.src or not args.models:
        ap.error("--src and --models are required (or --undo)")
    db.execute("ATTACH DATABASE ? AS src", (f"file:{os.path.abspath(args.src)}?mode=ro&immutable=1",))
    for t in ("npcs", "npcitem", "itempattern", "spellpattern", "defaultcharinventory", "defaultspells", "charactermodel"):
        if columns(db, "src", t) != columns(db, "main", t):
            sys.exit(f"{t}: the source's columns differ from ours; this tool reads our schema only")

    models = load_models(args.models)
    plans = plan_npcs(db, models)
    items = plan_ids(db, "itempattern", "patternid", "itemname")
    spells = plan_ids(db, "spellpattern", "spellid", "spellname")
    if args.apply:
        dropped = apply(db, plans, items, spells, args.src, catalogs=not args.no_catalogs)
        report(plans, items, spells)
        print("npcitem rows not carried over:", dict(dropped))
        print(f"applied to {args.db}")
    else:
        for p in plans:
            if p["status"] == "imported":
                _, p["fixes"] = convert_npc(p["src"], p["dst_id"])
        report(plans, items, spells)
        print("(report only: --apply to write it)")


if __name__ == "__main__":
    main()
