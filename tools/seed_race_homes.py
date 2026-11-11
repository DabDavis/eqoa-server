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
  NPC whose wares (npcitem) it sells too, which makes it a merchant (npc_type
  129: CLIENT opens a merchant's window by the flag 0x01, and asks the server
  for nothing at a talker of 128 but a talk); `optional` true: one whose
  `like` or whose `stock`'s seller this database lacks is left out, and said
  (the homes' Spell Archivist, whose wares are the Heirloom server's own);
- each name in `peaceful` gets an npcaggro row of aggressive 0 (a starter
  quest's critters, which a fledgling hunts, never hunt it), each name in
  `hostile` one of aggressive 1 (the level 5 quest's foe: wildlife of level 5
  and under attacks no one unless a rule says so). A name that has a row
  already (a GM's) keeps it as it is, and that is said. A foe's name is its
  home's alone: npcaggro and a quest's kill are by name, so a name NPCs of
  another world bear too is refused;
- `lines`, {name: [line, ...]}: the words of a home's people of that name when
  nothing else answers (npcdialog rows of their name: a speaker of its own
  lines says them alone, entity/dialog.go; %p the listener's name, %r its
  race, %c its class): a gatekeeper's before the starter quests are done, the
  sentinels', the elder's after.

The quests (who gives what, the gatekeeper's port) are tools/quests/data/
homelands.json's, written by tools/import_quests.py after this.

Everything written is logged in race_homes_log (each row's columns before,
each row added); --undo puts it all back as it was and drops the log. A
report by default; --apply writes. Stop the server to apply or undo (the
start rows and the people are read at its start), start it after.

    tools/seed_race_homes.py [--db eqoa.db] [--homes tools/worlds/homelands.json]            report
    tools/seed_race_homes.py [--db eqoa.db] --apply                                          write it
    tools/seed_race_homes.py [--db eqoa.db] --undo                                           take it back
    tools/seed_race_homes.py --format [--homes FILE]                                         the recipe file in its own layout again

A new recipe over a seeding that is there (tools/quests/homelands/README.md): --undo with the old one, --apply
with the new one, the server stopped (a keep restart's before.sh). The people get new npcs ids (rows added past
whatever is there); nothing else names them by id.
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
    what  TEXT NOT NULL,   -- 'racedef' | 'start' | 'npc' | 'npcitem' | 'aggro' | 'dialog' | 'dialogtable' | 'template' | 'loot'
    key   TEXT NOT NULL,   -- the row: racedefs id, defaultcharacter id, npcs id, npcitem id, npcaggro name, npcdialog id
    old   TEXT,            -- its columns before (JSON), none for a row added
    note  TEXT NOT NULL DEFAULT ''
)"""
TALKER, FOE = 128, 0
MERCHANT = 0x01  # the npc flag CLIENT opens a merchant's window by (handlers/merchant.go)
# npcdialog as the server makes it (database/npcdialog.go), for a database it has not run on
NPCDIALOG_DDL = """CREATE TABLE IF NOT EXISTS npcdialog (
    id     INTEGER PRIMARY KEY AUTOINCREMENT,
    town   TEXT NOT NULL DEFAULT '',
    role   TEXT NOT NULL DEFAULT '',
    name   TEXT NOT NULL DEFAULT '',
    mood   TEXT NOT NULL DEFAULT '',
    race   INTEGER NOT NULL DEFAULT -1,
    sex    TEXT NOT NULL DEFAULT '',
    line   TEXT NOT NULL,
    weight INTEGER NOT NULL DEFAULT 1,
    source TEXT NOT NULL DEFAULT 'gm',
    speaker_race INTEGER NOT NULL DEFAULT -1,
    away   INTEGER NOT NULL DEFAULT 0
)"""


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


_FIRST = {}


def first_ids(db):
    """One reading of npcs for every lookup by name or body (npcs has no index on either, and a recipe asks some
    six hundred times: 7 s of a keep restart's 12 before this): the lowest id of each name, of each name outside
    each world, of each body, and how many rows of each world bear each name (lower case)."""
    key = (id(db), db.total_changes)  # read again after any write through this connection
    if key not in _FIRST:
        by_name, by_body, by_name_world, lower = {}, {}, {}, {}
        for i, name, model, world in db.execute("SELECT id, npc_name, modelid, world FROM npcs ORDER BY id"):
            by_name.setdefault(name, i)
            by_name_world.setdefault(name, {}).setdefault(world, i)
            if model is not None:
                by_body.setdefault(int(model) & 0xFFFFFFFF, i)
            w = lower.setdefault(str(name).lower(), {})
            w[world] = w.get(world, 0) + 1
        _FIRST.clear()
        _FIRST[key] = (by_name, by_body, by_name_world, lower)
    return _FIRST[key]


def template(db, spec, missing_ok=False):
    """The npcs row a spec copies: its `like` (an id or a name), else a row of its body. missing_ok: None when
    this database has none (an optional one's)."""
    like = spec.get("like")
    by_name, by_body, _, _ = first_ids(db)
    if isinstance(like, int):
        i = like
    elif like:
        i = by_name.get(like)
    elif spec.get("body"):
        i = by_body.get(int(spec["body"], 16) & 0xFFFFFFFF)
    else:
        raise SystemExit(f"{spec['name']}: no `like` and no `body`")
    r = None if i is None else row_dict(db, "npcs", "id = ?", (i,))
    if r is None and not missing_ok:
        raise SystemExit(f"{spec['name']}: no npcs row like {like or spec.get('body')!r}")
    return r


def plan(db, recipe):
    """What --apply writes: [(what, ...)], in order."""
    world = recipe["world"]
    out = []
    for z in recipe["zones"]:
        home, shared = z.get("home"), z.get("shared")
        if home and shared:
            raise SystemExit(f"{z['name']}: a zone is a race's home or the homes' shared ground, not both")
        if shared:
            out += people(db, world, z, shared)
            continue
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
        out += people(db, world, z, home)
    return out


def people(db, world, z, home):
    """The writes of a block of a zone's people (a home's, or the shared ground's): its npcs, their npcaggro rules, their
    lines, and the shared ground's creatures' combat templates (npctemplate) and loot regions (lootcreature)."""
    out = []
    for n in home.get("npcs", []):
        optional = bool(n.get("optional"))
        t = template(db, n, missing_ok=optional)
        if t is None:
            out.append(("skip", n["name"], None, None, f"{n['name']} in {z['name']}: optional, and this database has no "
                                                       f"npcs row like {n.get('like') or n.get('body')!r}: left out"))
            continue
        if n.get("role", "talker") == "foe":
            n_else = sum(c for w, c in first_ids(db)[3].get(n["name"].lower(), {}).items() if w != world)
            if n_else:
                raise SystemExit(f"{z['name']}: {n_else} NPCs of other worlds are named {n['name']!r} too: a home's creature "
                                 "has a name of its own (npcaggro and a quest's kill go by name)")
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
            # the seller of another world: a home's own seller of the name (an earlier seeding's) is no source
            ids = [i for w, i in first_ids(db)[2].get(n["stock"], {}).items() if w != world]
            src = row_dict(db, "npcs", "id = ?", (min(ids),)) if ids else None
            if src is not None:
                cur = db.execute("SELECT * FROM npcitem WHERE npcid = ? ORDER BY listnumber, itemid", (src["id"],))
                cols = [c[0] for c in cur.description]
                stock = [dict(zip(cols, r)) for r in cur.fetchall()]
            if src is None or not stock:
                why = f"no NPC {n['stock']!r} to take wares from" if src is None else f"{n['stock']!r} sells nothing"
                if not optional:
                    raise SystemExit(f"{n['name']}: {why}")
                out.append(("skip", n["name"], None, None, f"{n['name']} in {z['name']}: optional, and {why}: left out"))
                continue
            row["npc_type"] |= MERCHANT
        out.append(("npc", None, None, {"row": row, "stock": stock}, f"{n['name']} in {z['name']}: {n.get('note', '')}".rstrip(": ")))
    people = {n["name"] for n in home.get("npcs", [])}
    for key, aggressive, what in (("peaceful", 0, "a quest's critter"), ("hostile", 1, "a quest's foe")):
        for name in home.get(key, []):
            if name not in people:
                raise SystemExit(f"{z['name']}: {key} names {name!r}, who is none of its people")
            if name in home.get("hostile", []) and name in home.get("peaceful", []):
                raise SystemExit(f"{z['name']}: {name!r} is peaceful and hostile")
            old = row_dict(db, "npcaggro", "npc_name = ?", (name,)) if has_table(db, "npcaggro") else None
            if old is not None:
                # a rule that is there is someone's (a GM's): kept as it is
                out.append(("skip", name, None, None, f"{name} ({z['name']}): npcaggro has a row for it already (aggressive "
                                                      f"{old.get('aggressive')}, source {old.get('source')!r}): kept as it is"))
                continue
            out.append(("aggro", name, None, {"aggressive": aggressive}, f"{name}: {what} ({z['name']})"))
    for name, lines in home.get("lines", {}).items():
        if name not in people:
            raise SystemExit(f"{z['name']}: lines for {name!r}, who is none of its people")
        for line in lines:
            out.append(("dialog", name, None, {"line": line}, f"{name}'s words ({z['name']})"))
    # the house's creature patterns (as the roamers and the loot categories seed them): a name's combat template and
    # its loot region; a name that has a row already (a GM's) keeps it
    for key, table, col, what in (("templates", "npctemplate", "name", "template"), ("loot", "lootcreature", "npc_name", "loot")):
        for name, value in sorted(home.get(key, {}).items()):
            if name not in people or not all(n.get("role") == "foe" for n in home["npcs"] if n["name"] == name):
                raise SystemExit(f"{z['name']}: {key} names {name!r}, who is none of its creatures")
            old = row_dict(db, table, f"{col} = ?", (name,)) if has_table(db, table) else None
            if old is not None:
                out.append(("skip", name, None, None, f"{name} ({z['name']}): {table} has a row for it already: kept as it is"))
                continue
            out.append((what, name, None, {"value": value}, f"{name}: {table} {value!r} ({z['name']})"))
    return out


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def format_recipe(recipe):
    """A recipe as its file is laid out: two spaces a level, a value a line, but the extra files' names on one."""
    text = json.dumps(recipe, indent=2, ensure_ascii=False) + "\n"
    if isinstance(recipe.get("extraFiles"), list):
        spread = json.dumps({"extraFiles": recipe["extraFiles"]}, indent=2, ensure_ascii=False)[2:-2]
        text = text.replace(spread, '  "extraFiles": ' + json.dumps(recipe["extraFiles"], ensure_ascii=False), 1)
    return text


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
                # a name two homes list is written once (the plan saw no row for either)
                if db.execute("SELECT count(*) FROM npcaggro WHERE npc_name = ?", (key,)).fetchone()[0]:
                    continue
                db.execute("INSERT INTO npcaggro (npc_name, aggressive, radius, note, source) VALUES (?, ?, 0, ?, 'racehomes')",
                           (key, new["aggressive"], note))
                log(what, key, None, note)
            elif what == "template":
                if db.execute("SELECT count(*) FROM npctemplate WHERE name = ?", (key,)).fetchone()[0]:
                    continue
                db.execute("INSERT INTO npctemplate (name, template, note, source) VALUES (?, ?, ?, 'racehomes')", (key, new["value"], note))
                log(what, key, None, note)
            elif what == "loot":
                if db.execute("SELECT count(*) FROM lootcreature WHERE npc_name = ?", (key,)).fetchone()[0]:
                    continue
                db.execute("INSERT INTO lootcreature (npc_name, region, profile, source) VALUES (?, ?, '', 'racehomes')", (key, new["value"]))
                log(what, key, None, note)
            elif what == "dialog":
                if not has_table(db, "npcdialog"):
                    db.execute(NPCDIALOG_DDL)
                    log("dialogtable", "npcdialog", None, "made for the homes' words (dropped at the undo)")
                cur = db.execute("INSERT INTO npcdialog (name, line, weight, source) VALUES (?, ?, 1, 'racehomes')", (key, new["line"]))
                log(what, cur.lastrowid, None, note)
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
            elif what == "dialog":
                db.execute("DELETE FROM npcdialog WHERE id = ?", (int(key),))
            elif what == "dialogtable":
                db.execute("DROP TABLE IF EXISTS npcdialog")
            elif what == "template":
                db.execute("DELETE FROM npctemplate WHERE name = ?", (key,))
            elif what == "loot":
                db.execute("DELETE FROM lootcreature WHERE npc_name = ?", (key,))
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
    g.add_argument("--format", action="store_true", help="write the recipe file again in its own layout (no database)")
    a = ap.parse_args(argv)
    if a.format:
        text = format_recipe(json.load(open(a.homes)))
        if text != open(a.homes).read():
            with open(a.homes, "w") as f:
                f.write(text)
            print(f"{a.homes}: laid out again")
        else:
            print(f"{a.homes}: as it was")
        return 0
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
        elif what == "dialog":
            print(f"  dialog  {key}: {new['line']!r}")
        else:
            print(f"  {what:7} {key}: {note}")
    writes = [st for st in steps if st[0] != "skip"]
    left = len(steps) - len(writes)
    if a.apply:
        apply(db, writes)
        print(f"{a.db}: {len(writes)} written, logged in {LOG}" + (f"; {left} left out or kept as they are (skip, above)" if left else ""))
    else:
        print(f"{len(writes)} to write (--apply writes them)" + (f"; {left} left out or kept as they are (skip, above)" if left else ""))
    return 0


if __name__ == "__main__":
    sys.exit(main())
