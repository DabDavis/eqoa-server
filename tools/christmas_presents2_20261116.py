#!/usr/bin/env python3
"""Way more presents under Freeport's Christmas tree (the owner, 2026-11-16: "lets add way more presents under tree").

The tree's presents row (lightstrings shape 'presents' on its name, tools/christmas_lights3_20261116.py) takes the boxes
of christmas_presents2_20261116.json in place of its ten: a ring round the base under the lowest boughs and a few
spilling out to the east, small, medium and big, tall thin and flat wide, stacks of 2-3, turned at angles; red, green,
blue, gold, silver, purple, white, pink and teal, no two neighbours alike, some with no ribbon or bow. Data only: the
server and vanilla of christmas3 draw it (a PS2 draws none).

A report by default; --apply writes it with a gm_audit line and keeps the old params in --applied; --undo puts them back.
Live by the GM panel: touch the Light strings row (or any Seasons / Light strings edit) and the set is sent to every
client that draws it; a restart reads it too.

    christmas_presents2_20261116.py --db eqoa.db [--apply | --undo] --applied FILE [--boxes christmas_presents2_20261116.json]
"""
import argparse, json, os, sqlite3, sys, time

TREE, SRC = "Christmas Tree", "christmas-2026"
ap = argparse.ArgumentParser()
ap.add_argument("--db", required=True)
ap.add_argument("--applied", required=True)
ap.add_argument("--boxes", default=os.path.join(os.path.dirname(os.path.abspath(__file__)), "christmas_presents2_20261116.json"))
g = ap.add_mutually_exclusive_group()
g.add_argument("--apply", action="store_true")
g.add_argument("--undo", action="store_true")
a = ap.parse_args()
db = sqlite3.connect(a.db, timeout=30)
now = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def audit(key, before, after):
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) VALUES (?, 1, 'dab', 'tool', '', 'update', 'lightstrings', ?, ?, ?, ?)",
               (now, json.dumps(key), json.dumps(before), json.dumps(after),
                "the owner: way more presents under the Christmas tree; tools/christmas_presents2_20261116.py"))


rows = db.execute("SELECT id, params, note FROM lightstrings WHERE npc = ? AND source = ? AND shape = 'presents'", (TREE, SRC)).fetchall()
if a.undo:
    j = json.load(open(a.applied))
    cur = db.execute("SELECT params FROM lightstrings WHERE id = ?", (j["id"],)).fetchone()
    db.execute("UPDATE lightstrings SET params = ?, note = ? WHERE id = ?", (j["params"], j["note"], j["id"]))
    audit({"id": j["id"]}, {"boxes": len(json.loads(cur[0])["boxes"]) if cur else None}, {"boxes": len(json.loads(j["params"])["boxes"])})
    db.commit(); os.rename(a.applied, a.applied + ".undone"); print("undone"); sys.exit()
if a.apply and os.path.exists(a.applied):
    sys.exit("applied already")
if len(rows) != 1:
    sys.exit(f"{len(rows)} presents rows on the tree, not one (tools/christmas_lights3_20261116.py first): nothing done")
sid, old, oldnote = rows[0]
boxes = json.load(open(a.boxes))["boxes"]
if not 1 <= len(boxes) <= 64:
    sys.exit(f"{len(boxes)} boxes: 1 to 64 (the server's DecorMaxGifts)")
new = json.dumps({"boxes": boxes}, separators=(",", ":"))
db.execute("UPDATE lightstrings SET params = ?, note = ? WHERE id = ?",
           (new, "Freeport's Christmas tree: presents round its base under the lowest boughs, a few spilling out to the east", sid))
audit({"id": sid}, {"boxes": len(json.loads(old)["boxes"])}, {"boxes": len(boxes)})
print(f"{TREE}'s presents (lightstrings {sid}): {len(json.loads(old)['boxes'])} -> {len(boxes)} boxes")
if a.apply:
    json.dump({"id": sid, "params": old, "note": oldnote}, open(a.applied, "w")); db.commit(); print("applied")
else:
    db.rollback()
