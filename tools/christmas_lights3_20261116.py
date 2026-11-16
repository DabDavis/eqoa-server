#!/usr/bin/env python3
"""Freeport's Christmas tree: its lights fitted to the fir's boughs, presents under it, the tree raised for them (the owner,
2026-11-16: "the lights for the xmas tree are not quite lined on the tree perfectly. also we need some different color
presents under the tree. we might have to raise the tree a bit to accomodate").

- The tree's art (decorart, the Lightwolf fir 0xC4BE0726 of tools/christmas_fir_20261116.py) sunk SINK in place of 11: the
  lowest boughs' tips (10.92 over the fir's foot) stand some 1.3 over the sand, room for the presents under them.
- Its light strings: the four spirals become five paths (lightstrings shape 'path': the bulbs in order on the cord), each
  bulb 0.22 out from the boughs' surface as measured on the fir's mesh (christmas_fir_garlands_20261116.json), lowered by
  SINK; the star over the top on the fifth.
- Presents: a lightstrings row of shape 'presents' on the tree's name, of the season: ten gift boxes (red, green, blue,
  gold, silver, purple; a contrasting ribbon, most with a bow; two stacked) round its base under the lowest boughs. Vanilla
  draws them; a PS2 draws none.

A report by default; --apply writes it with gm_audit lines and keeps what it changed in --applied; --undo puts it back.
The server reads the tables at its start, at a season's turn and at the GM panel's edit of them: a keep restart's
before.sh, or touch the Light strings in the panel after. A client showing the tree sees the new art's depth when the tree
comes into its view again (a relog), the lights and presents at once.

    christmas_lights3_20261116.py --db eqoa.db [--apply | --undo] --applied FILE [--fit christmas_fir_garlands_20261116.json]
"""
import argparse, json, os, sqlite3, sys, time

TREE, SRC, SEASON, TEMPLATE = "Christmas Tree", "christmas-2026", "Christmas", "Christmas lights"
FIR, OLD_SINK = 0xC4BE0726, 11.0
SINK = 9.6  # by shots: the lowest boughs some 1.3 over the sand, the trunk's foot showing a little under them behind the presents
STAR = "FFE070"

ap = argparse.ArgumentParser()
ap.add_argument("--db", required=True)
ap.add_argument("--applied", required=True)
ap.add_argument("--fit", default=os.path.join(os.path.dirname(os.path.abspath(__file__)), "christmas_fir_garlands_20261116.json"))
g = ap.add_mutually_exclusive_group()
g.add_argument("--apply", action="store_true")
g.add_argument("--undo", action="store_true")
a = ap.parse_args()
db = sqlite3.connect(a.db, timeout=30)
now = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
COLS = ["id", "template", "npc", "world", "shape", "params", "season", "note", "source"]


def audit(action, tbl, key, before, after):
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) VALUES (?, 1, 'dab', 'tool', '', ?, ?, ?, ?, ?, ?)",
               (now, action, tbl, json.dumps(key), None if before is None else json.dumps(before, sort_keys=True),
                None if after is None else json.dumps(after, sort_keys=True),
                "the owner: the tree's lights fitted to its boughs, presents under it, the tree raised; tools/christmas_lights3_20261116.py"))


def tree_rows():
    return [dict(zip(COLS, r)) for r in db.execute(f"SELECT {', '.join(COLS)} FROM lightstrings WHERE npc = ? AND source = ? ORDER BY id", (TREE, SRC))]


if a.undo:
    j = json.load(open(a.applied))
    for sid in j["inserted"]:
        r = db.execute(f"SELECT {', '.join(COLS)} FROM lightstrings WHERE id = ?", (sid,)).fetchone()
        db.execute("DELETE FROM lightstrings WHERE id = ?", (sid,))
        audit("delete", "lightstrings", {"id": sid}, r and dict(zip(COLS, r)), None)
    for r in j["rows"]:
        db.execute("UPDATE lightstrings SET shape = ?, params = ?, note = ? WHERE id = ?", (r["shape"], r["params"], r["note"], r["id"]))
        audit("update", "lightstrings", {"id": r["id"]}, None, r)
    db.execute("UPDATE decorart SET sink = ? WHERE name = ?", (j["sink"], TREE))
    audit("update", "decorart", {"name": TREE}, {"sink": SINK}, {"sink": j["sink"]})
    db.commit(); os.rename(a.applied, a.applied + ".undone"); print("undone"); sys.exit()

if a.apply and os.path.exists(a.applied):
    sys.exit("applied already")
art = db.execute("SELECT model, sink FROM decorart WHERE name = ?", (TREE,)).fetchone()
if art is None or (art[0] & 0xFFFFFFFF) != FIR or art[1] != OLD_SINK:
    sys.exit(f"the tree's decorart row is {art}, not the Lightwolf fir {FIR:#010x} sunk {OLD_SINK}: run christmas_fir_20261116.py first; nothing done")
rows = tree_rows()
spirals = [r for r in rows if r["shape"] == "spiral"]
if len(spirals) != 4 or len(rows) != 4:
    sys.exit(f"the tree's light strings are {[(r['id'], r['shape']) for r in rows]}, not its four spirals: nothing done")
fit = json.load(open(a.fit))
tiers, star, boxes = fit["tiers"], fit["star"], fit["presents"]
paths = [[[p[0], round(p[1] - SINK, 3), p[2]] for p in t] for t in tiers]
j = {"sink": art[1], "rows": rows, "inserted": []}
db.execute("UPDATE decorart SET sink = ? WHERE name = ?", (SINK, TREE))
audit("update", "decorart", {"name": TREE}, {"sink": art[1]}, {"sink": SINK})
for i, (r, pts) in enumerate(zip(spirals, paths)):
    note = f"Freeport's Christmas tree, garland {i + 1} of {len(paths)}: fitted to the fir 0xC4BE0726's boughs, sunk {SINK}"
    params = json.dumps({"points": pts}, separators=(",", ":"))
    db.execute("UPDATE lightstrings SET shape = 'path', params = ?, note = ? WHERE id = ?", (params, note, r["id"]))
    audit("update", "lightstrings", {"id": r["id"]}, r, {"shape": "path", "points": len(pts)})
for pts in paths[len(spirals):]:
    i = len(spirals) + 1
    params = {"points": pts, "top": STAR, "star": [star[0], round(star[1] - SINK, 3), star[2]]}
    sid = db.execute("INSERT INTO lightstrings (template, npc, world, shape, params, season, note, source) VALUES (?, ?, 0, 'path', ?, ?, ?, ?)",
                     (TEMPLATE, TREE, json.dumps(params, separators=(",", ":")), SEASON,
                      f"Freeport's Christmas tree, garland {i} of {len(paths)} and the star: fitted to the fir 0xC4BE0726's boughs, sunk {SINK}", SRC)).lastrowid
    audit("insert", "lightstrings", {"id": sid}, None, {"shape": "path", "points": len(pts), "star": True})
    j["inserted"].append(sid)
sid = db.execute("INSERT INTO lightstrings (template, npc, world, shape, params, season, note, source) VALUES ('', ?, 0, 'presents', ?, ?, ?, ?)",
                 (TREE, json.dumps({"boxes": boxes}, separators=(",", ":")), SEASON,
                  "Freeport's Christmas tree: presents round its base under the lowest boughs", SRC)).lastrowid
audit("insert", "lightstrings", {"id": sid}, None, {"shape": "presents", "boxes": len(boxes)})
j["inserted"].append(sid)
print(f"{TREE}: art sunk {art[1]} -> {SINK}; strings {[r['id'] for r in spirals]} -> paths "
      f"({', '.join(str(len(p)) for p in paths)} bulbs), inserted {j['inserted']} (the fifth garland and the star; {len(boxes)} presents)")
if a.apply:
    json.dump(j, open(a.applied, "w")); db.commit(); print("applied")
else:
    db.rollback()
