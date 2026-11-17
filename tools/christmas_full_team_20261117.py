#!/usr/bin/env python3
"""Santa's whole team: Comet, Cupid, Donner and Blitzen join the sleigh (the owner, 2026-11-17: "lets add donner and
blitzen to santas sleigh", then "add comet and cupid too").

From the sleigh (25285.1, 15768.9, its team facing east into the square): Prancer/Vixen (25291.7), Dasher/Dancer
(25294.2), Comet/Cupid (25296.7), Donner/Blitzen (25299.2), then Rudolph alone in the lead, moved forward two pair
spacings (2.5 each) to 25301.7; his nose is a string on him (lightstrings npc Rudolph) and comes with him. Each new
horse is a copy of Dasher's row (CHAR.ESF's white winged horse 0x2F8721C0 at size 0.65, friendly npc_type 128, the
team's row as christmas4 made it), of the Christmas season (npcseason), friendly (npcaggro 0), standing (npcmove), with
a line of its own. The harness (the four line strings on Santa's Sleigh, christmas4's) is lengthened to reach Rudolph's
new place: its far end 5 further ahead in the sleigh's frame (its -x is the world's east).

The longer footprint is clear: the nearest NPC to it is Rehabilitator Sela at (25305.5, 15762.3), 7.6 from Rudolph;
the sand runs on to x 25306 at the team's z (the harbor's water starts south of it).

A report by default; --apply writes it with gm_audit lines and keeps what it did in --applied; --undo puts it back.
New NPC rows and Rudolph's moved spawn are live after a restart (a keep restart: before.sh or after the binary); the
harness's strings are read at the next decorations' load (a GM's lights edit in the panel, the season trigger, a
restart).

    christmas_full_team_20261117.py --db eqoa.db --applied FILE [--apply | --undo]
"""
import argparse, json, os, sqlite3, sys, time

SRC, SEASON, SLEIGH_SRC = "christmas-2026-full-team", "Christmas", "christmas-2026-sleigh"
NEW = [  # name, x, z, line
    ("Comet", 25296.7, 15768.0, "Comet's mane streams back as if she were already flying."),
    ("Cupid", 25296.7, 15769.8, "Cupid nuzzles your shoulder, hoping for an apple."),
    ("Donner", 25299.2, 15768.0, "Donner stamps a hoof, and the harness bells jingle."),
    ("Blitzen", 25299.2, 15769.8, "Blitzen snorts a puff of frost and flexes his wings."),
]
RUDOLPH_FROM, RUDOLPH_TO = (25296.7, 15768.9), (25301.7, 15768.9)
HARNESS_FROM, HARNESS_TO = -10.8, -15.8  # the harness's far end, the sleigh's frame x

ap = argparse.ArgumentParser()
ap.add_argument("--db", required=True)
ap.add_argument("--applied", required=True)
g = ap.add_mutually_exclusive_group()
g.add_argument("--apply", action="store_true")
g.add_argument("--undo", action="store_true")
a = ap.parse_args()
db = sqlite3.connect(a.db, timeout=30)
db.row_factory = sqlite3.Row
now = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def audit(action, tbl, key, before, after):
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) VALUES (?, 1, 'dab', 'tool', '', ?, ?, ?, ?, ?, ?)",
               (now, action, tbl, json.dumps(key), None if before is None else json.dumps(before), None if after is None else json.dumps(after),
                "the owner: Santa's whole team (Comet, Cupid, Donner, Blitzen); tools/christmas_full_team_20261117.py"))


def harness():
    return db.execute("SELECT * FROM lightstrings WHERE npc = ? AND shape = 'line' AND source = ?", ("Santa's Sleigh", SLEIGH_SRC)).fetchall()


if a.undo:
    j = json.load(open(a.applied))
    for nid in j["npcs"]:
        r = db.execute("SELECT * FROM npcs WHERE id = ?", (nid,)).fetchone()
        if r is not None and r["npc_name"] in [n[0] for n in NEW]:
            db.execute("DELETE FROM npcs WHERE id = ?", (nid,))
            audit("delete", "npcs", {"id": nid}, dict(r), None)
    for tbl in ("npcseason", "npcaggro", "npcmove", "npcdialog"):
        db.execute(f"DELETE FROM {tbl} WHERE source = ?", (SRC,))
    r = db.execute("SELECT * FROM npcs WHERE id = ?", (j["rudolph"],)).fetchone()
    if r is not None:
        db.execute("UPDATE npcs SET x_coord = ?, z_coord = ? WHERE id = ?", (RUDOLPH_FROM[0], RUDOLPH_FROM[1], j["rudolph"]))
        audit("update", "npcs", {"id": j["rudolph"]}, {"x_coord": r["x_coord"]}, {"x_coord": RUDOLPH_FROM[0]})
    for sid, params in j["harness"].items():
        db.execute("UPDATE lightstrings SET params = ? WHERE id = ?", (params, int(sid)))
        audit("update", "lightstrings", {"id": int(sid)}, None, {"params": params})
    db.commit()
    os.rename(a.applied, a.applied + ".undone")
    print("undone")
    sys.exit()
if a.apply and os.path.exists(a.applied):
    sys.exit("applied already")
for n in NEW:
    if db.execute("SELECT count(*) FROM npcs WHERE npc_name = ?", (n[0],)).fetchone()[0]:
        sys.exit(f"an NPC {n[0]!r} is here already: nothing done")
dasher = db.execute("SELECT * FROM npcs WHERE npc_name = 'Dasher' ORDER BY id LIMIT 1").fetchone()
rud = db.execute("SELECT * FROM npcs WHERE npc_name = 'Rudolph' ORDER BY id LIMIT 1").fetchone()
if dasher is None or rud is None:
    sys.exit("no Dasher or Rudolph (christmas4's sleigh tool first): nothing done")
if abs(rud["x_coord"] - RUDOLPH_FROM[0]) > 0.05 or abs(rud["z_coord"] - RUDOLPH_FROM[1]) > 0.05:
    sys.exit(f"Rudolph is not where christmas4 stood him ({rud['x_coord']}, {rud['z_coord']}): nothing done")
hs = harness()
if len(hs) != 4:
    sys.exit(f"the harness is {len(hs)} line strings, not christmas4's 4: nothing done")
# the footprint: nothing else standing within 1.5 of a new place or Rudolph's
team = {"Santa's Sleigh", "Santa Claus", "Rudolph", "Dasher", "Dancer", "Prancer", "Vixen"}
for name, x, z in [(n[0], n[1], n[2]) for n in NEW] + [("Rudolph", *RUDOLPH_TO)]:
    near = [r["npc_name"] for r in db.execute("SELECT npc_name FROM npcs WHERE world = 0 AND abs(x_coord - ?) < 1.5 AND abs(z_coord - ?) < 1.5", (x, z))
            if r["npc_name"] not in team]
    if near:
        sys.exit(f"{', '.join(near)} stands where {name} would: nothing done")
cols = [c for c in dasher.keys() if c != "id"]
done = {"npcs": [], "rudolph": rud["id"], "harness": {}}
for name, x, z, line in NEW:
    nid = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) SELECT {', '.join(cols)} FROM npcs WHERE id = ?", (dasher["id"],)).lastrowid
    db.execute("UPDATE npcs SET npc_name = ?, x_coord = ?, z_coord = ?, npc_id = ? WHERE id = ?", (name, x, z, nid, nid))
    audit("insert", "npcs", {"id": nid}, None, {"npc_name": name, "at": [x, dasher["y_coord"], z], "copy_of": dasher["id"]})
    done["npcs"].append(nid)
    db.execute("INSERT INTO npcseason (name, season, note, source) VALUES (?, ?, 'Santa''s team', ?)", (name, SEASON, SRC))
    db.execute("INSERT INTO npcaggro (npc_name, aggressive, radius, note, source) VALUES (?, 0, 0, 'the Christmas season: friendly', ?)", (name, SRC))
    db.execute("INSERT INTO npcmove (name, roam, radius, water, note, source) VALUES (?, 0, 0, '', 'stands (Santa''s team)', ?)", (name.lower(), SRC))
    db.execute("INSERT INTO npcdialog (town, role, name, line, weight, source) VALUES ('', '', ?, ?, 2, ?)", (name, line, SRC))
db.execute("UPDATE npcs SET x_coord = ?, z_coord = ? WHERE id = ?", (RUDOLPH_TO[0], RUDOLPH_TO[1], rud["id"]))
audit("update", "npcs", {"id": rud["id"]}, {"x_coord": rud["x_coord"]}, {"x_coord": RUDOLPH_TO[0]})
for h in hs:
    p = json.loads(h["params"])
    if abs(p["to"][0] - HARNESS_FROM) > 0.01:
        sys.exit(f"harness string {h['id']} ends at {p['to'][0]}, not {HARNESS_FROM}: nothing done")
    done["harness"][str(h["id"])] = h["params"]
    p["to"][0] = HARNESS_TO
    np_ = json.dumps(p, separators=(",", ":"))
    db.execute("UPDATE lightstrings SET params = ? WHERE id = ?", (np_, h["id"]))
    audit("update", "lightstrings", {"id": h["id"]}, {"params": h["params"]}, {"params": np_})
print(f"{', '.join(n[0] for n in NEW)} in the team; Rudolph to ({RUDOLPH_TO[0]}, {RUDOLPH_TO[1]}); harness strings {sorted(done['harness'])} to x {HARNESS_TO}")
if a.apply:
    json.dump(done, open(a.applied, "w"))
    db.commit()
    print("applied")
else:
    db.rollback()
