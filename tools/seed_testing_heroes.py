#!/usr/bin/env python3
"""The Testing Heroes' data (entity/heroes.go, handlers/heroes.go; petition 1471, the GM Jibrish, 2026-11-10: "For all
mobs that give the player XP, add a new faction "Testing Heroes". This starts at max negative for all players ... Add
a vendor in freeport near the well and dabrish that allows you to buy items from him with that faction. The items
should cost 25, 100, 250, 500 and 1000 faction"), for a server whose config sets world.testing_heroes:

  factions "Testing Heroes" at -2000, the floor: every character starts there, and what it may spend is its standing
    above it;
  itempattern 965001 "Elixir of the Giant Gnome" (25): a use item of five charges (itemeffect: "Self, Body Size 4,
    30min", the gnome man's body, a woman's her own: the drinker a gnome the height of a Hill Giant's player, whose
    body at 1.0 a gnome's at 4.0 stands level with, measured side by side in the client; thirty minutes, and a death
    ends it as it ends every effect);
  itempattern 965002 "Tome of Heroic Training" (100): a use item of one charge (itemeffect: "Self, Increase Training
    Points by 5": five unspent points, kept apart from the levels' own in charbonustp; refused, the tome kept, where
    the reader's stats have no room for five more);
  itempattern 965003 "Weakened Breastplate of the Anointed" (250): a copy of the Breastplate's record (1076), every
    column but its name and words; the server reads its heal by its name at 70% (entity/wornshield.go);
  the Breastplate of the Anointed itself, record 1076 (500);
  its Ascended record (1000), made here by the Ascended records' own rule (tools/ascend_boss_loot.py's, which is
    database/ascended.go's: each stat a tenth more, the heal a tenth more by entity/ascended.go) with its itemascended
    row, or the one the server already has;
  heroes_stock: the five, with what the vendor says of each (source "testing-heroes"; the table made as the server
    makes it where a server has not yet);
  npcs "Testing Heroes Vendor": talked to and never attacked (npc_type 128), level 60, Merchant Kari's look, facing as
    Dabrish does, at his height, on the first free spot of SPOTS between the well (its roof (25291..25301, 15747..15753))
    and Dabrish; npcmove: stands; npcaggro: never on sight. A spot is free when no other NPC's row is within 3.5 of it.

Refused, nothing written, where any of it is there already (the faction, a record of an id or a name it makes, a stock
row, a vendor), where record 1076 is not the Breastplate of the Anointed or is itself a copy, where Dabrish does not
stand within 25 of the well, where Merchant Kari is not a row, or where no spot is free.

WHEN IT TAKES EFFECT: the server reads factions and npcs at its start. Run it as the before.sh of the keep restart
that brings the code (tools/swap_keep.sh), with world.testing_heroes: true in the config; never under an older server
(it has no heroes_stock and reads the two use lines as unread words).

Report only by default; --apply writes it in one transaction, every row with its gm_audit line and its own line in
testing_heroes_log; --undo takes every row away again, and the ids its new rows took when no other row took one since:
all of it or nothing. An undo is refused, nothing done, while a row is not as it was written (a GM's edit since: each
is said), while a character holds one of the records it made, or once the vendor has sold anything (heroes_purchases).

    python3 -E -s tools/seed_testing_heroes.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import math
import os
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import ascend_boss_loot as asc  # noqa: E402  the Ascended records' rule

FACTION, FLOOR = "Testing Heroes", -2000
SRC = "testing-heroes"
WHO, VIA = "dab's sessions", "tool"
DETAIL = "petition 1471 (Jibrish): the Testing Heroes' faction, vendor and items; tools/seed_testing_heroes.py"
LOG = "testing_heroes_log"
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,      -- the table, or sqlite_sequence for the ids handed out
    rowkey  TEXT NOT NULL,      -- the row's key (JSON)
    before  TEXT,               -- the row before (JSON); NULL: made here
    after   TEXT,               -- the row as written (JSON)
    audit   INTEGER             -- the gm_audit line written
)"""
# database.HeroesStockSchema, as the server makes it (TestSeedTestingHeroes.test_the_stock_schema_is_the_servers)
STOCK_SCHEMA = """CREATE TABLE IF NOT EXISTS heroes_stock (
	id        INTEGER PRIMARY KEY AUTOINCREMENT,
	patternid INTEGER NOT NULL UNIQUE,
	cost      INTEGER NOT NULL,
	says      TEXT NOT NULL DEFAULT '',
	source    TEXT NOT NULL DEFAULT 'gm'
)"""
KEYS = {"factions": ("name",), "itempattern": ("patternid",), "itemeffect": ("patternid",), "itemascended": ("patternid",),
        "heroes_stock": ("patternid",), "npcs": ("id",), "npcmove": ("name",), "npcaggro": ("npc_name",)}
SEQS = ("npcs", "heroes_stock")

BASE, BASE_NAME = 1076, "Breastplate of the Anointed"
ELIXIR, TOME, WEAKENED = 965001, 965002, 965003
ELIXIR_NAME, TOME_NAME = "Elixir of the Giant Gnome", "Tome of Heroic Training"
WEAKENED_NAME = "Weakened " + BASE_NAME
GNOME_BODY = 2845600533      # the gnome man's CSprite (0xA99C6B15), as npcs.modelid and charactermodel hold it
GNOME_SIZE = 4               # a gnome's body at 4.0 stands level with a Hill Giant's (0xFFB1EA2C) at a player's 1.0
POTION_ICON = -649479524     # the Reconstructive's
BOOK_ICON = 1856243648       # the book of records'
EFFECT_ICON = 1507275072     # an illusion's, on the effects list (the Transformative Pill's)
ELIXIR_LINE = f"Self, Body Size {GNOME_SIZE}, 30min"
TOME_LINE = "Self, Increase Training Points by 5"
COSTS = (25, 100, 250, 500, 1000)

VENDOR, BESIDE, TEMPLATE = "Testing Heroes Vendor", "Dabrish", "Merchant Kari"
LEVEL, TYPE, ROOM = 60, 128, 3.5
WELL = (25296.0, 15750.0)    # world 0: the well's middle, 10 west of Dabrish
WELL_REACH = 25.0
# world 0 (x, z), +x east, +z south: south-east of the well's roof, toward Dabrish; then round the well
SPOTS = [(25299.5, 15755.5), (25296.5, 15743.5), (25289.5, 15746.0), (25289.5, 15752.5), (25300.5, 15757.5)]

USE_ITEM = {  # a use item's record, as the Reconstructive's and the Ring of Speed's are
    "itemcost": 0, "unk1": 0, "unk2": 0, "equipslot": -1, "unk3": 0, "trade": 0, "rent": 1, "unk4": 0, "attacktype": 0,
    "weapondamage": 0, "unk5": 0, "levelreq": 1, "maxstack": 1, "maxhp": 0, "duration": 50, "classuse": 32767,
    "raceuse": 1023, "procanim": 7, "lore": 0, "unk6": 0, "craft": 0, "model": 0, "color": 4294967295,
}
# A box draws no percent sign (CLIENT shows "(?)"): the words say percent.
SAYS = {
    ELIXIR: "Five draughts. Each makes you a gnome as tall as a hill giant for thirty minutes, or until you die.",
    TOME: "Read it once for 5 training points, unspent, yours to place. It stays shut while your stats have no room for five more.",
    WEAKENED: "The Breastplate of the Anointed, weakened: all of its stats, but its heal at each hit is 30 percent less.",
    BASE: "The Breastplate of the Anointed, whole: 15 and a tenth of your charisma healed at each hit you take.",
    "ascended": "The Breastplate of the Anointed, Ascended: its stats and its heal a tenth more.",
}


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def columns(db, tbl):
    return [r[1] for r in db.execute(f"PRAGMA table_info({tbl})")]


def row_of(db, tbl, key):
    where = " AND ".join(f"{c} = ?" for c in KEYS[tbl])
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", [key[c] for c in KEYS[tbl]])
    cols = [d[0] for d in cur.description]
    r = cur.fetchone()
    return None if r is None else dict(zip(cols, r))


def go_json(m):
    return json.dumps(m, sort_keys=True, separators=(",", ":"), default=str)


def use_record(db, pid, name, desc, icon):
    """A use item's whole record: USE_ITEM, its own name, words and icon, every figure 0."""
    rec = {}
    for c in columns(db, "itempattern"):
        low = c.lower()
        if low == "patternid":
            rec[c] = pid
        elif low == "itemname":
            rec[c] = name
        elif low == "itemdesc":
            rec[c] = desc
        elif low == "itemicon":
            rec[c] = icon
        else:
            rec[c] = USE_ITEM.get(low, 0)
    return rec


def effect_row(db, pid, line, name, icon, body, charges):
    row = {"patternid": pid, "line": line, "name": name, "icon": icon, "breaks": "", "wear": 0, "area": 0.0, "source": SRC,
           "body": body, "target_line": "", "charges": charges, "cooldown": 0.0}
    return {c: row[c] for c in columns(db, "itemeffect") if c in row}


def free(db, x, z):
    """Why the spot is taken, or None."""
    for n, nx, nz in db.execute("SELECT npc_name, x_coord, z_coord FROM npcs WHERE world = 0 AND abs(x_coord - ?) < 30 AND abs(z_coord - ?) < 30",
                                (x, z)).fetchall():
        d = math.hypot(nx - x, nz - z)
        if d < ROOM:
            return f"{n} stands {d:.1f} from it"
    return None


def plan(db):
    """(steps, lines, refusals): what would be written, in order, what to print, and why nothing may be. A step is
    (tbl, row) for a row made as given, or ("ascend", base) for the Ascended record made by the server's own SQL."""
    steps, lines, refusals = [], [], []
    need = ("factions", "itempattern", "itemeffect", "npcs", "npcmove", "npcaggro", "gm_audit")
    missing = [t for t in need if not table(db, t)]
    if missing:
        return [], [], [f"no table {', '.join(missing)}: not a server's database"]

    # the faction
    if db.execute("SELECT 1 FROM factions WHERE lower(name) = lower(?)", (FACTION,)).fetchone():
        refusals.append(f"factions has {FACTION!r} already")
    steps.append(("factions", {"name": FACTION, "standing": FLOOR}))
    lines.append(f"factions: {FACTION} at {FLOOR}")

    # the records
    base = row_of(db, "itempattern", {"patternid": BASE})
    name_col = next((c for c in columns(db, "itempattern") if c.lower() == "itemname"), "itemname")
    desc_col = next((c for c in columns(db, "itempattern") if c.lower() == "itemdesc"), "itemdesc")
    if base is None or (base[name_col] or "").strip() != BASE_NAME:
        refusals.append(f"itempattern {BASE} is {base and base[name_col]!r}, not {BASE_NAME!r}")
        base = None
    for t in ("itemascended", "itemimported"):
        if table(db, t) and db.execute(f"SELECT 1 FROM {t} WHERE patternid = ?", (BASE,)).fetchone():
            refusals.append(f"itempattern {BASE} is a copy ({t})")
    for pid, name in ((ELIXIR, ELIXIR_NAME), (TOME, TOME_NAME), (WEAKENED, WEAKENED_NAME)):
        if db.execute("SELECT 1 FROM itempattern WHERE patternid = ?", (pid,)).fetchone():
            refusals.append(f"itempattern {pid} is taken")
        if db.execute("SELECT 1 FROM itemeffect WHERE patternid = ?", (pid,)).fetchone():
            refusals.append(f"itemeffect {pid} is taken")
        other = db.execute(f"SELECT patternid FROM itempattern WHERE lower(trim({name_col})) = lower(?)", (name,)).fetchone()
        if other:
            refusals.append(f"a record named {name!r} is there already ({other[0]})")
    steps.append(("itempattern", use_record(db, ELIXIR, ELIXIR_NAME,
                                            "Five draughts: each makes its drinker a gnome as tall as a hill giant for thirty minutes. A death ends it.",
                                            POTION_ICON)))
    steps.append(("itemeffect", effect_row(db, ELIXIR, ELIXIR_LINE, "Giant Gnome", EFFECT_ICON, GNOME_BODY, 5)))
    lines.append(f"itempattern {ELIXIR}: {ELIXIR_NAME}, 5 charges: {ELIXIR_LINE!r}, body {GNOME_BODY:#x}")
    steps.append(("itempattern", use_record(db, TOME, TOME_NAME,
                                            "Read once: 5 training points, unspent. It stays shut while your stats have no room for five more.",
                                            BOOK_ICON)))
    steps.append(("itemeffect", effect_row(db, TOME, TOME_LINE, "", 0, 0, 1)))
    lines.append(f"itempattern {TOME}: {TOME_NAME}, 1 charge: {TOME_LINE!r}")
    if base is not None:
        weak = dict(base)
        weak["patternid" if "patternid" in weak else "patternID"] = WEAKENED
        weak[name_col] = WEAKENED_NAME
        weak[desc_col] = "A Breastplate of the Anointed whose blessing has thinned: its heal at each hit is 30 percent less."
        steps.append(("itempattern", weak))
        lines.append(f"itempattern {WEAKENED}: {WEAKENED_NAME}, a copy of {BASE} (the server reads its heal at 70% by its name)")

    # the Ascended record: the server's, or made here
    asc_id = None
    if table(db, "itemascended"):
        r = db.execute("SELECT patternid FROM itemascended WHERE base = ?", (BASE,)).fetchone()
        asc_id = r[0] if r else None
    if asc_id is not None:
        lines.append(f"the Ascended {BASE_NAME} is record {asc_id} already: sold as it is")
    elif base is not None:
        if not asc.is_gear({k.lower(): v for k, v in base.items()}):
            refusals.append(f"itempattern {BASE} is not gear an Ascended record is made of")
        steps.append(("ascend", BASE))
        lines.append(f"an Ascended record of {BASE} is made (the Ascended records' rule), with its itemascended row")

    # the stock
    if table(db, "heroes_stock") and db.execute("SELECT 1 FROM heroes_stock WHERE source = ? OR patternid IN (?, ?, ?, ?)",
                                                (SRC, ELIXIR, TOME, WEAKENED, BASE)).fetchone():
        refusals.append("heroes_stock holds the Testing Heroes' rows already")
    if asc_id is not None and table(db, "heroes_stock") and db.execute("SELECT 1 FROM heroes_stock WHERE patternid = ?", (asc_id,)).fetchone():
        refusals.append(f"heroes_stock sells the Ascended record {asc_id} already")
    wares = [(ELIXIR, SAYS[ELIXIR]), (TOME, SAYS[TOME]), (WEAKENED, SAYS[WEAKENED]), (BASE, SAYS[BASE]),
             (asc_id if asc_id is not None else "ascended", SAYS["ascended"])]
    for (pid, says), cost in zip(wares, COSTS):
        steps.append(("heroes_stock", {"patternid": pid, "cost": cost, "says": says, "source": SRC}))
    lines.append("heroes_stock: " + ", ".join(f"{c}" for c in COSTS) + f" for the elixir, the tome, the Weakened, record {BASE} and its Ascended")

    # the vendor
    if db.execute("SELECT 1 FROM npcs WHERE lower(trim(npc_name)) = lower(?)", (VENDOR,)).fetchone():
        refusals.append(f"a {VENDOR} stands already")
    if db.execute("SELECT 1 FROM npcmove WHERE name = ?", (VENDOR.lower(),)).fetchone() or \
            db.execute("SELECT 1 FROM npcaggro WHERE lower(npc_name) = ?", (VENDOR.lower(),)).fetchone():
        refusals.append(f"the {VENDOR} has an npcmove or npcaggro row already")
    ds = db.execute("SELECT id, x_coord, y_coord, z_coord, facing, zone, world FROM npcs WHERE npc_name = ? ORDER BY id", (BESIDE,)).fetchall()
    t = db.execute("SELECT * FROM npcs WHERE npc_name = ? ORDER BY id", (TEMPLATE,))
    tcols = [d[0] for d in t.description]
    t = t.fetchone()
    if len(ds) != 1:
        refusals.append(f"{len(ds)} rows named {BESIDE}")
    elif ds[0][6] != 0 or math.hypot(ds[0][1] - WELL[0], ds[0][3] - WELL[1]) > WELL_REACH:
        refusals.append(f"{BESIDE} stands at world {ds[0][6]} ({ds[0][1]}, {ds[0][3]}), not within {WELL_REACH:g} of the well")
    elif t is None:
        refusals.append(f"no row named {TEMPLATE} to take the vendor's look from")
    else:
        d = ds[0]
        chosen = None
        for x, z in SPOTS:
            why = free(db, x, z)
            lines.append(f"  spot ({x}, {z}): {why or 'free'}")
            if why is None:
                chosen = (x, z)
                break
        if chosen is None:
            refusals.append("no spot by the well is free")
        else:
            row = dict(zip(tcols, t))
            row.pop("id")
            row.update({"npc_name": VENDOR, "zone": d[5], "unk_a": 0, "unk_b": 0, "world": 0, "x_coord": chosen[0], "y_coord": d[2],
                        "z_coord": chosen[1], "facing": d[4], "npc_type": TYPE, "npc_level": LEVEL, "size": 1.0})
            steps.append(("npcs", row))
            steps.append(("npcmove", {"name": VENDOR.lower(), "roam": 0, "radius": 0, "water": "", "note": "stands (the Testing Heroes Vendor)", "source": SRC}))
            steps.append(("npcaggro", {"npc_name": VENDOR, "aggressive": 0, "radius": 0, "note": "the Testing Heroes Vendor: talks only", "source": SRC}))
            lines.append(f"npcs: {VENDOR} at world 0 ({chosen[0]}, {d[2]}, {chosen[1]}) facing {d[4]}, "
                         f"{math.hypot(chosen[0] - d[1], chosen[1] - d[3]):.1f} from {BESIDE} (npcs {d[0]}), "
                         f"{math.hypot(chosen[0] - WELL[0], chosen[1] - WELL[1]):.1f} from the well's middle, the look of {TEMPLATE}")
    return steps, lines, refusals


def audit(db, action, tbl, key, before, after, detail):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, 0, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, WHO, VIA, action, tbl, go_json(key), None if before is None else go_json(before),
                      None if after is None else go_json(after), detail))
    return cur.lastrowid


def seqs(db):
    if not table(db, "sqlite_sequence"):
        return {}
    return {t: (lambda r: None if r is None else r[0])(db.execute("SELECT seq FROM sqlite_sequence WHERE name = ?", (t,)).fetchone())
            for t in SEQS}


def logged(db, tbl, key):
    row = row_of(db, tbl, key)
    a = audit(db, "insert", tbl, key, None, row, DETAIL)
    db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, NULL, ?, ?)",
               (tbl, json.dumps(key), json.dumps(row, sort_keys=True, default=str), a))


def apply(db, steps):
    """The plan written, in the caller's transaction: each row with its gm_audit line and its line in the log. Returns
    how many rows."""
    db.execute(LOG_SCHEMA)
    had_stock = table(db, "heroes_stock")
    before_seq = seqs(db)
    db.execute(STOCK_SCHEMA)
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    asc_id, n = None, 0
    for tbl, row in steps:
        if tbl == "ascend":
            db.execute(asc.SCHEMA)
            sets, froms, _ = asc.copy_columns(columns(db, "itempattern"))
            asc_id = db.execute("SELECT COALESCE(MAX(patternid), ?) + 1 FROM itempattern WHERE patternid BETWEEN ? AND ?",
                                (asc.FIRST_ID - 1, asc.FIRST_ID, asc.LAST_ID)).fetchone()[0]
            if asc_id > asc.LAST_ID:
                raise SystemExit(f"no id left for an Ascended record ({asc.FIRST_ID}-{asc.LAST_ID})")
            db.execute(f'INSERT INTO itempattern ("patternid", {sets}) SELECT ?, {froms} FROM itempattern WHERE patternid = ?', (asc_id, row))
            logged(db, "itempattern", {"patternid": asc_id})
            db.execute("INSERT INTO itemascended (patternid, base, made, source) VALUES (?, ?, ?, ?)",
                       (asc_id, row, at, "tools/seed_testing_heroes.py"))
            logged(db, "itemascended", {"patternid": asc_id})
            n += 2
            continue
        row = dict(row)
        if tbl == "heroes_stock" and row["patternid"] == "ascended":
            row["patternid"] = asc_id
        cols = list(row)
        cur = db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [row[c] for c in cols])
        if tbl == "npcs":
            db.execute("UPDATE npcs SET npc_id = ? WHERE id = ?", (cur.lastrowid, cur.lastrowid))
            key = {"id": cur.lastrowid}
        else:
            key = {c: row[c] for c in KEYS[tbl]}
        logged(db, tbl, key)
        n += 1
    db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES ('sqlite_sequence', ?, ?, ?)",
               (json.dumps({"had_stock": had_stock}), json.dumps(before_seq), json.dumps(seqs(db))))
    return n


def held(db):
    """Why an undo may not be: who holds a record made here, what was sold."""
    why = []
    made = [json.loads(k)["patternid"] for (k,) in db.execute(f"SELECT rowkey FROM {LOG} WHERE tbl = 'itempattern'")]
    if made and table(db, "charinventory"):
        q = ", ".join("?" * len(made))
        for pid, n in db.execute(f"SELECT patternid, count(*) FROM charinventory WHERE patternid IN ({q}) GROUP BY patternid", made):
            why.append(f"{n} of record {pid} held by characters")
    if table(db, "heroes_purchases"):
        n = db.execute("SELECT count(*) FROM heroes_purchases").fetchone()[0]
        if n:
            why.append(f"{n} purchases in heroes_purchases")
    return why


def changed(db):
    """The logged rows that are no longer as written (a GM's edit since), each said."""
    out = []
    for tbl, key, after in db.execute(f"SELECT tbl, rowkey, after FROM {LOG} WHERE tbl != 'sqlite_sequence' ORDER BY seq").fetchall():
        now = row_of(db, tbl, json.loads(key)) if table(db, tbl) else None
        if now is None or json.dumps(now, sort_keys=True, default=str) != after:
            out.append(f"{tbl} {key} changed since it was written")
    return out


def undo(db):
    """Every logged row taken away, the last first, with its gm_audit line; then the ids the new rows took, when no
    other row took one since. The caller has seen that each row is still as written (changed). Returns how many."""
    back = 0
    seq_row = db.execute(f"SELECT rowkey, before, after FROM {LOG} WHERE tbl = 'sqlite_sequence'").fetchone()
    for tbl, key in db.execute(f"SELECT tbl, rowkey FROM {LOG} WHERE tbl != 'sqlite_sequence' ORDER BY seq DESC").fetchall():
        key = json.loads(key)
        now = row_of(db, tbl, key)
        db.execute(f"DELETE FROM {tbl} WHERE " + " AND ".join(f"{c} = ?" for c in KEYS[tbl]), [key[c] for c in KEYS[tbl]])
        audit(db, "delete", tbl, key, now, None, "undo: " + DETAIL)
        back += 1
    if seq_row is not None:
        info, was, written = json.loads(seq_row[0]), json.loads(seq_row[1]), json.loads(seq_row[2])
        cur = seqs(db)
        for t in SEQS:
            if cur.get(t) is None or cur.get(t) != written.get(t):
                continue
            if was.get(t) is None:
                db.execute("DELETE FROM sqlite_sequence WHERE name = ?", (t,))
            else:
                db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = ?", (was[t], t))
        if not info.get("had_stock") and db.execute("SELECT count(*) FROM heroes_stock").fetchone()[0] == 0:
            db.execute("DROP TABLE heroes_stock")
    db.execute(f"DROP TABLE {LOG}")
    return back


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        if a.undo:
            if not table(db, LOG):
                print("nothing to undo")
                return 0
            why = held(db) + changed(db)
            if why:
                print("refused, nothing done: " + "; ".join(why))
                return 1
            db.execute("BEGIN IMMEDIATE")
            try:
                back = undo(db)
                db.execute("COMMIT")
            except BaseException:
                db.execute("ROLLBACK")
                raise
            print(f"undone: {back} rows taken away")
            return 0
        if table(db, LOG):
            print(f"applied already ({LOG} is there): --undo first")
            return 1
        steps, lines, refusals = plan(db)
        for line in lines:
            print(line)
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not a.apply:
            print(f"report only: {len(steps)} steps would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            n = apply(db, steps)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {n} rows, each in gm_audit and {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
