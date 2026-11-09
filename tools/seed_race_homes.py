#!/usr/bin/env python3
"""The races' homelands' people and starts, out of a composed world's recipe
(tools/worlds/homelands.json; cmd/worldbuild builds the world itself from the
operator's disc, handlers/serverworlds.go offers it to clients that build it
too). For each zone of the recipe with a `home`:

- its race's flavour (racedefs.description, the create page's words) is the
  home's `flavor`;
- its race's start rows (defaultcharacter, every class) stand at the home's
  `start` in the recipe's world, facing `toward` (a point, x and z) or
  `facing` (radians): a new character of the race starts there and is bound
  there (database.CreateNew), and a login of one from a client that does not
  hold the world goes to its wire race's row instead (MainlandPlace);
- its `npcs` are placed in the recipe's world: each a copy of an npcs row (`like`,
  an id or a name; else a row of its `body`) named `name`, at `at` (x, y, z: a
  spot on the floor, as the disc's own NPCs of the zone stand), facing `toward`
  or `facing` (the npcs byte, 0-255), of `level`, a `talker` (npc_type 128, the
  NPC bit: it is talked to, never fought) or a `foe` (0); `body` a CHAR.ESF
  DictID ("0x9E004757") worn instead of the copy's, `size` its size, `stock` an
  NPC whose wares (npcitem) it sells too;
- each name in `peaceful` gets an npcaggro row of aggressive 0 (a starter
  quest's critters, which a fledgling hunts, never hunt it).

The quests (who gives what, the gatekeeper's port) are tools/quests/data/
homelands.json's, written by tools/import_quests.py after this.

Everything written is logged in race_homes_log (each row's columns before,
each row added); --undo puts it all back as it was and drops the log. A
report by default; --apply writes. Stop the server to apply or undo (the
start rows and the people are read at its start), start it after.

    tools/seed_race_homes.py [--db eqoa.db] [--homes tools/worlds/homelands.json]            report
    tools/seed_race_homes.py [--db eqoa.db] --apply                                          write it
    tools/seed_race_homes.py [--db eqoa.db] --undo                                           take it back
"""
import argparse
import json
import math
import os
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
DEFAULT_DB = "eqoa.db"
DEFAULT_HOMES = os.path.join(HERE, "worlds", "homelands.json")
LOG = "race_homes_log"
LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq   INTEGER PRIMARY KEY AUTOINCREMENT,
    what  TEXT NOT NULL,   -- 'racedef' | 'start' | 'npc' | 'npcitem' | 'aggro'
    key   TEXT NOT NULL,   -- the row: racedefs id, defaultcharacter id, npcs id, npcitem id, npcaggro name
    old   TEXT,            -- its columns before (JSON), none for a row added
    note  TEXT NOT NULL DEFAULT ''
)"""
TALKER, FOE = 128, 0


def heading_to(x, z, tx, tz):
    """CLIENT's heading from (x, z) toward (tx, tz): the direction (sin h, cos h)."""
    return math.atan2(tx - x, tz - z) % (2 * math.pi)


def byte_heading(h):
    """A heading as the npcs facing byte (0-255 a turn)."""
    return int(round(h / (2 * math.pi) * 256)) % 256


def row_dict(db, table, where, args):
    cur = db.execute(f"SELECT * FROM {table} WHERE {where}", args)
    r = cur.fetchone()
    if r is None:
        return None
    return dict(zip([c[0] for c in cur.description], r))


def template(db, spec):
    """The npcs row a spec copies: its `like` (an id or a name), else a row of its body."""
    like = spec.get("like")
    if isinstance(like, int):
        r = row_dict(db, "npcs", "id = ?", (like,))
    elif like:
        r = row_dict(db, "npcs", "npc_name = ? ORDER BY id", (like,))
    elif spec.get("body"):
        r = row_dict(db, "npcs", "(modelid & 4294967295) = ? ORDER BY id", (int(spec["body"], 16) & 0xFFFFFFFF,))
    else:
        raise SystemExit(f"{spec['name']}: no `like` and no `body`")
    if r is None:
        raise SystemExit(f"{spec['name']}: no npcs row like {like or spec.get('body')!r}")
    return r


def plan(db, recipe):
    """What --apply writes: [(what, ...)], in order."""
    world = recipe["world"]
    out = []
    for z in recipe["zones"]:
        home = z.get("home")
        if not home:
            continue
        race = z["race"]
        rd = row_dict(db, "racedefs", "id = ?", (race,))
        if rd is None:
            raise SystemExit(f"race {race} has no racedefs row: {z['name']}'s home is for no race")
        if home.get("flavor") and rd.get("description") != home["flavor"]:
            out.append(("racedef", race, rd, {"description": home["flavor"]}, f"{rd['name']}: {z['name']}"))
        s = home["start"]
        facing = s["facing"] if "facing" in s else heading_to(s["x"], s["z"], *s["toward"])
        rows = db.execute("SELECT defaultCharacterID FROM defaultcharacter WHERE race = ? ORDER BY tclass", (race,)).fetchall()
        if not rows:
            raise SystemExit(f"race {race} ({rd['name']}) has no defaultcharacter rows to move")
        for (rid,) in rows:
            old = row_dict(db, "defaultcharacter", "defaultCharacterID = ?", (rid,))
            new = {"x": s["x"], "y": s["y"], "z": s["z"], "facing": round(facing, 6), "world": world}
            out.append(("start", rid, old, new, f"{rd['name']} class {old['tclass']}: {z['name']}"))
        for n in home.get("npcs", []):
            t = template(db, n)
            row = {k: v for k, v in t.items() if k != "id"}
            x, y, zz = n["at"]
            if "toward" in n:
                face = byte_heading(heading_to(x, zz, *n["toward"]))
            else:
                face = int(n.get("facing", t.get("facing") or 0))
            row.update(npc_name=n["name"], x_coord=x, y_coord=y, z_coord=zz, facing=face, world=world,
                       npc_level=int(n.get("level", t.get("npc_level") or 1)),
                       npc_type=TALKER if n.get("role", "talker") == "talker" else FOE, dt_kill=None)
            if n.get("body"):
                row["modelid"] = int(n["body"], 16) & 0xFFFFFFFF
                for k in ("primary_weapon", "secondary_weapon", "shield", "torso", "forearm", "gloves", "legs", "feet", "head"):
                    row[k] = 0
            if n.get("size"):
                row["size"] = float(n["size"])
            stock = []
            if n.get("stock"):
                src = row_dict(db, "npcs", "npc_name = ? ORDER BY id", (n["stock"],))
                if src is None:
                    raise SystemExit(f"{n['name']}: no NPC {n['stock']!r} to take wares from")
                cur = db.execute("SELECT * FROM npcitem WHERE npcid = ? ORDER BY listnumber, itemid", (src["id"],))
                cols = [c[0] for c in cur.description]
                stock = [dict(zip(cols, r)) for r in cur.fetchall()]
            out.append(("npc", None, None, {"row": row, "stock": stock}, f"{n['name']} in {z['name']}: {n.get('note', '')}".rstrip(": ")))
        for name in home.get("peaceful", []):
            old = row_dict(db, "npcaggro", "npc_name = ?", (name,)) if has_table(db, "npcaggro") else None
            out.append(("aggro", name, old, {"aggressive": 0}, f"{name}: a starter quest's critter ({z['name']})"))
    return out


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def apply(db, steps):
    db.execute("BEGIN IMMEDIATE")
    try:
        if has_table(db, LOG) and db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0]:
            raise SystemExit(f"{LOG} holds a seeding already: --undo it first")
        db.execute(LOG_DDL)
        log = lambda what, key, old, note: db.execute(f"INSERT INTO {LOG} (what, key, old, note) VALUES (?, ?, ?, ?)",
                                                     (what, str(key), None if old is None else json.dumps(old), note))
        for what, key, old, new, note in steps:
            if what == "racedef":
                db.execute("UPDATE racedefs SET description = ? WHERE id = ?", (new["description"], key))
                log(what, key, {"description": old["description"]}, note)
            elif what == "start":
                db.execute("UPDATE defaultcharacter SET x = ?, y = ?, z = ?, facing = ?, world = ? WHERE defaultCharacterID = ?",
                           (new["x"], new["y"], new["z"], new["facing"], new["world"], key))
                log(what, key, {k: old[k] for k in ("x", "y", "z", "facing", "world")}, note)
            elif what == "npc":
                row = new["row"]
                cols = list(row)
                cur = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [row[c] for c in cols])
                nid = cur.lastrowid
                db.execute("UPDATE npcs SET npc_id = ? WHERE id = ?", (nid, nid))
                log(what, nid, None, note)
                for it in new["stock"]:
                    it = {k: v for k, v in it.items() if k != "itemid"}
                    it["npcid"] = nid
                    c2 = db.execute(f"INSERT INTO npcitem ({', '.join(it)}) VALUES ({', '.join('?' * len(it))})", list(it.values()))
                    log("npcitem", c2.lastrowid, None, f"{row['npc_name']}'s wares")
            elif what == "aggro":
                db.execute("""CREATE TABLE IF NOT EXISTS npcaggro (npc_name TEXT PRIMARY KEY, aggressive INTEGER NOT NULL DEFAULT 1,
                    radius REAL NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm')""")
                db.execute("""INSERT INTO npcaggro (npc_name, aggressive, radius, note, source) VALUES (?, 0, 0, ?, 'racehomes')
                    ON CONFLICT(npc_name) DO UPDATE SET aggressive = 0, radius = 0, note = excluded.note, source = excluded.source""",
                           (key, note))
                log(what, key, old, note)
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db):
    if not has_table(db, LOG):
        raise SystemExit("nothing to undo")
    db.execute("BEGIN IMMEDIATE")
    try:
        for what, key, old in db.execute(f"SELECT what, key, old FROM {LOG} ORDER BY seq DESC").fetchall():
            old = None if old is None else json.loads(old)
            if what == "racedef":
                db.execute("UPDATE racedefs SET description = ? WHERE id = ?", (old["description"], int(key)))
            elif what == "start":
                db.execute("UPDATE defaultcharacter SET x = ?, y = ?, z = ?, facing = ?, world = ? WHERE defaultCharacterID = ?",
                           (old["x"], old["y"], old["z"], old["facing"], old["world"], int(key)))
            elif what == "npcitem":
                db.execute("DELETE FROM npcitem WHERE itemid = ?", (int(key),))
            elif what == "npc":
                db.execute("DELETE FROM npcs WHERE id = ?", (int(key),))
            elif what == "aggro":
                if old is None:
                    db.execute("DELETE FROM npcaggro WHERE npc_name = ?", (key,))
                else:
                    cols = list(old)
                    db.execute(f"INSERT OR REPLACE INTO npcaggro ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                               [old[c] for c in cols])
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--homes", default=DEFAULT_HOMES)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None)
    if a.undo:
        undo(db)
        print(f"{a.db}: the homes' seeding taken back")
        return 0
    recipe = json.load(open(a.homes))
    steps = plan(db, recipe)
    for what, key, old, new, note in steps:
        if what == "npc":
            r = new["row"]
            print(f"  npc     {r['npc_name']!r} world {r['world']} ({r['x_coord']}, {r['y_coord']}, {r['z_coord']}) facing {r['facing']} "
                  f"level {r['npc_level']} type {r['npc_type']} body {r['modelid'] & 0xFFFFFFFF:08X}"
                  + (f", {len(new['stock'])} wares" if new["stock"] else ""))
        else:
            print(f"  {what:7} {key}: {note}")
    if a.apply:
        apply(db, steps)
        print(f"{a.db}: {len(steps)} written, logged in {LOG}")
    else:
        print(f"{len(steps)} to write (--apply writes them)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
