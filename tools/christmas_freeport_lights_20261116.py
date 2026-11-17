#!/usr/bin/env python3
"""Christmas lights on Freeport's walls, well, coach house, lamp posts and bank stalls (the owner, 2026-11-16: "we also need
to make the christmas lights able to be put on any model. in freeport we can decorate put lights on the fence/low wall near
freeport coach, the well, etc.").

Each string of christmas_freeport_lights_20261116.json is a path of bulbs in world 0's coordinates, fitted to a zone
actor's mesh (tools/decor_fit.py over vanilla's `eqoa actormesh`: a wall's top, a well's rim and eaves, a lamp post wound
round, a canopy's edge). This writes each as a lightstrings row (shape 'path', npc blank: in the world, world 0, the
"Christmas lights" template: its colours, twinkle and cord), of the Christmas season: lit with the tree, out when it is.
Data only (the server and vanilla of christmas3 draw world paths); a PS2 draws none.

A report by default; --apply writes it with a gm_audit line a row and keeps the ids in --applied; --undo deletes them. Live
by the GM panel: touch a Light strings row; a restart reads them too.

    christmas_freeport_lights_20261116.py --db eqoa.db [--apply | --undo] --applied FILE [--strings FILE.json]
"""
import argparse, json, os, sqlite3, sys, time

SRC, SEASON, TEMPLATE = "christmas-2026", "Christmas", "Christmas lights"
ap = argparse.ArgumentParser()
ap.add_argument("--db", required=True)
ap.add_argument("--applied", required=True)
ap.add_argument("--strings", default=os.path.join(os.path.dirname(os.path.abspath(__file__)), "christmas_freeport_lights_20261116.json"))
g = ap.add_mutually_exclusive_group()
g.add_argument("--apply", action="store_true")
g.add_argument("--undo", action="store_true")
a = ap.parse_args()
db = sqlite3.connect(a.db, timeout=30)
now = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
COLS = ["id", "template", "npc", "world", "shape", "params", "season", "note", "source"]


def audit(action, key, before, after):
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) VALUES (?, 1, 'dab', 'tool', '', ?, 'lightstrings', ?, ?, ?, ?)",
               (now, action, json.dumps(key), None if before is None else json.dumps(before), None if after is None else json.dumps(after),
                "the owner: Christmas lights on Freeport's walls, well and posts; tools/christmas_freeport_lights_20261116.py"))


if a.undo:
    j = json.load(open(a.applied))
    for sid in j["ids"]:
        r = db.execute(f"SELECT {', '.join(COLS)} FROM lightstrings WHERE id = ?", (sid,)).fetchone()
        if r is not None and r[COLS.index("source")] == SRC and r[COLS.index("npc")] == "":
            db.execute("DELETE FROM lightstrings WHERE id = ?", (sid,))
            audit("delete", {"id": sid}, dict(zip(COLS, r)), None)
    db.commit(); os.rename(a.applied, a.applied + ".undone"); print("undone"); sys.exit()
if a.apply and os.path.exists(a.applied):
    sys.exit("applied already")
if not db.execute("SELECT count(*) FROM lighttemplates WHERE name = ? COLLATE NOCASE", (TEMPLATE,)).fetchone()[0]:
    sys.exit(f"no light template {TEMPLATE!r} (tools/christmas_season_20261116.py first): nothing done")
strings = json.load(open(a.strings))["strings"]
notes = {s["note"] for s in strings}
have = [n for (n,) in db.execute("SELECT note FROM lightstrings WHERE source = ? AND npc = ''", (SRC,)) if n in notes]
if have:
    sys.exit(f"{len(have)} of these strings are here already ({have[0]!r}): nothing done")
ids, bulbs = [], 0
for s in strings:
    params = json.dumps({"points": s["points"]}, separators=(",", ":"))
    sid = db.execute("INSERT INTO lightstrings (template, npc, world, shape, params, season, note, source) VALUES (?, '', 0, 'path', ?, ?, ?, ?)",
                     (TEMPLATE, params, SEASON, s["note"], SRC)).lastrowid
    audit("insert", {"id": sid}, None, {"note": s["note"], "sprite": s["sprite"], "zone": s["zone"], "fit": s["fit"], "points": len(s["points"])})
    ids.append(sid)
    bulbs += len(s["points"])
print(f"{len(ids)} strings of lights in Freeport ({bulbs} bulbs): rows {ids[0]}..{ids[-1]}")
if a.apply:
    json.dump({"ids": ids}, open(a.applied, "w")); db.commit(); print("applied")
else:
    db.rollback()
