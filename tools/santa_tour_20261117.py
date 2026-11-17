#!/usr/bin/env python3
"""Santa's sleigh tour of Freeport (the owner, 2026-11-17: "lets have santa claus give a scenic tour of freeport for anyone
on the sled, he announces it in chat. he will initiate once a player talks to him and chooses to start"; "the sled with
the pegasus and santa will altogether fly together"; "let players run onto the sled without jumping").

Writes the tour (table npctours: entity/npctour.go flies it) and the walk-ons (table npcwalkon: the sleigh's deck stood
on, the team walked through) from tools/santa_tour_20261117.json: the route over the harbor, the eastern gate, the temple
court, the north quarter, the north gate, the round hall and the west gate and back to the tree, Santa's line at each, the
deck's places, the drop beside the tree. Needs a server with npctours (branch santa-tour); the NPCs are christmas4's
(Santa Claus, Santa's Sleigh, Rudolph, Dasher, Dancer, Prancer, Vixen) and must stand in the world.

A report by default; --apply writes it with gm_audit lines and keeps the rows in --applied; --undo deletes them. Data
read at the start and at each season turn or decorations' reload (LoadDecorLights): a keep restart, or the Christmas
Season trigger off and on, makes it live.

    santa_tour_20261117.py --db eqoa.db --applied FILE [--json tools/santa_tour_20261117.json] [--apply | --undo]
"""
import argparse, json, os, sqlite3, sys, time

SRC = "christmas-2026-santa-tour"
HERE = os.path.dirname(os.path.abspath(__file__))

ap = argparse.ArgumentParser()
ap.add_argument("--db", required=True)
ap.add_argument("--applied", required=True)
ap.add_argument("--json", default=os.path.join(HERE, "santa_tour_20261117.json"))
g = ap.add_mutually_exclusive_group()
g.add_argument("--apply", action="store_true")
g.add_argument("--undo", action="store_true")
a = ap.parse_args()
spec = json.load(open(a.json))
db = sqlite3.connect(a.db, timeout=30)
db.row_factory = sqlite3.Row
now = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def audit(action, tbl, key, before, after):
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) VALUES (?, 1, 'dab', 'tool', '', ?, ?, ?, ?, ?, ?)",
               (now, action, tbl, json.dumps(key), None if before is None else json.dumps(before), None if after is None else json.dumps(after),
                "the owner: Santa's sleigh tour of Freeport; tools/santa_tour_20261117.py"))


def ensure():
    # the server makes these at its start (database/npctours.go); a database it has not opened yet gets them here
    db.executescript("""
    CREATE TABLE IF NOT EXISTS npctours (name TEXT PRIMARY KEY COLLATE NOCASE, host TEXT NOT NULL DEFAULT '', vehicle TEXT NOT NULL DEFAULT '',
        members TEXT NOT NULL DEFAULT '[]', world INTEGER NOT NULL DEFAULT 0, route TEXT NOT NULL DEFAULT '[]', deck TEXT NOT NULL DEFAULT '[]',
        drop_at TEXT NOT NULL DEFAULT '[]', speed REAL NOT NULL DEFAULT 10, countdown REAL NOT NULL DEFAULT 30, cooldown REAL NOT NULL DEFAULT 120,
        board REAL NOT NULL DEFAULT 6, lines TEXT NOT NULL DEFAULT '{}', season TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '',
        source TEXT NOT NULL DEFAULT 'gm');
    CREATE TABLE IF NOT EXISTS npcwalkon (name TEXT PRIMARY KEY COLLATE NOCASE, mode TEXT NOT NULL DEFAULT 'deck', radius REAL NOT NULL DEFAULT 3,
        height REAL NOT NULL DEFAULT 1, season TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
    """)


if a.undo:
    j = json.load(open(a.applied))
    for name in j["tours"]:
        r = db.execute("SELECT * FROM npctours WHERE name = ? AND source = ?", (name, SRC)).fetchone()
        if r is not None:
            db.execute("DELETE FROM npctours WHERE name = ? AND source = ?", (name, SRC))
            audit("delete", "npctours", {"name": name}, dict(r), None)
    for name in j["walkon"]:
        r = db.execute("SELECT * FROM npcwalkon WHERE name = ? AND source = ?", (name, SRC)).fetchone()
        if r is not None:
            db.execute("DELETE FROM npcwalkon WHERE name = ? AND source = ?", (name, SRC))
            audit("delete", "npcwalkon", {"name": name}, dict(r), None)
    db.commit()
    os.rename(a.applied, a.applied + ".undone")
    print("undone")
    sys.exit()
if a.apply and os.path.exists(a.applied):
    sys.exit("applied already")
ensure()
if db.execute("SELECT count(*) FROM npctours WHERE name = ?", (spec["name"],)).fetchone()[0]:
    sys.exit(f"a tour {spec['name']!r} is here already: nothing done")
for w in spec["walkon"]:
    if db.execute("SELECT count(*) FROM npcwalkon WHERE name = ?", (w["name"],)).fetchone()[0]:
        sys.exit(f"a walk-on {w['name']!r} is here already: nothing done")
missing = [n for n in [spec["host"], spec["vehicle"]] + spec["members"]
           if not db.execute("SELECT count(*) FROM npcs WHERE npc_name = ? AND world = ?", (n, spec["world"])).fetchone()[0]]
if missing:
    sys.exit(f"no {', '.join(missing)} in world {spec['world']} (christmas4's sleigh tool first): nothing done")
# the route's sanity: it starts and ends on the sleigh where its row stands, and Santa's lines fit a chat line
veh = db.execute("SELECT x_coord, y_coord, z_coord FROM npcs WHERE npc_name = ? AND world = ? ORDER BY id LIMIT 1", (spec["vehicle"], spec["world"])).fetchone()
for end in (spec["route"][0], spec["route"][-1]):
    if abs(end["x"] - veh[0]) > 0.5 or abs(end["z"] - veh[2]) > 0.5:
        sys.exit(f"the route's ends ({end['x']}, {end['z']}) are not the sleigh's place ({veh[0]:.1f}, {veh[2]:.1f}): nothing done")
for p in spec["route"]:
    if len(p.get("say", "")) > 180 or "|" in p.get("say", ""):
        sys.exit(f"a line too long for a chat line, or with a bar (a PS2 dies on a run of them): {p['say']!r}")
done = {"tours": [], "walkon": []}
row = {
    "name": spec["name"], "host": spec["host"], "vehicle": spec["vehicle"], "members": json.dumps(spec["members"]),
    "world": spec["world"], "route": json.dumps(spec["route"], separators=(",", ":")), "deck": json.dumps(spec["deck"]),
    "drop_at": json.dumps(spec["drop"]), "speed": spec["speed"], "countdown": spec["countdown"], "cooldown": spec["cooldown"],
    "board": spec["board"], "lines": json.dumps(spec.get("lines", {})), "season": spec.get("season", ""),
    "note": "Santa's sleigh tour of Freeport: talk to Santa Claus by the tree", "source": SRC,
}
db.execute(f"INSERT INTO npctours ({', '.join(row)}) VALUES ({', '.join('?' * len(row))})", tuple(row.values()))
audit("insert", "npctours", {"name": spec["name"]}, None, {k: row[k] for k in ("host", "vehicle", "members", "speed", "countdown", "cooldown", "board")})
done["tours"].append(spec["name"])
for w in spec["walkon"]:
    db.execute("INSERT INTO npcwalkon (name, mode, radius, height, season, note, source) VALUES (?, ?, ?, ?, ?, ?, ?)",
               (w["name"], w["mode"], w.get("radius", 3), w.get("height", 1), spec.get("season", ""), w.get("note", ""), SRC))
    audit("insert", "npcwalkon", {"name": w["name"]}, None, {"mode": w["mode"], "radius": w.get("radius", 3), "height": w.get("height", 1)})
    done["walkon"].append(w["name"])
legs = sum(((b["x"] - p["x"]) ** 2 + (b["y"] - p["y"]) ** 2 + (b["z"] - p["z"]) ** 2) ** 0.5 for p, b in zip(spec["route"], spec["route"][1:]))
print(f"{spec['name']}: {len(spec['route'])} points, {legs:.0f} along the waypoints, {sum(p.get('pause', 0) for p in spec['route']):.0f} s of pauses, "
      f"{sum(1 for p in spec['route'] if p.get('say'))} lines; {len(spec['walkon'])} walk-ons")
if a.apply:
    json.dump(done, open(a.applied, "w"))
    db.commit()
    print("applied")
else:
    db.rollback()
