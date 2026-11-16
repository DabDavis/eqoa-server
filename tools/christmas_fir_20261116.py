#!/usr/bin/env python3
"""Freeport's Christmas tree as the fir where Dust stood by Castle Lightwolf (the owner, 2026-11-16: "the christmas tree npc
model should be a fir tree model like where dust is standing, but cut the stock down so boughs are on the ground", then
"with the tree you can place lower Y so trunk goes through ground instead of cutting").

Dust stood at world 0 (9164.2, 54.1, 6599.5); the fir 4.3 from him is TUNARIA.ESF's 0xC4BE0726, a snow-dusted fir of zone
24 (the same mesh as the green 0x0E318233 the tree wore, 21.8 across, its lowest boughs' tips 10.9 over its foot, other
textures; six 0x444153A7 and one 0xC4BE0727 stand round him too). The tree's decorart row (tools/christmas_season_20261116.py)
names the art a decorating client (vanilla) is sent in the NPC's body and the zone it reads it from: now 0xC4BE0726 of zone
24, drawn SINK lower than the NPC stands, so its trunk goes down through the ground and its lowest boughs rest on it. A
creature stands on the floor whatever its row's y (both clients' ground probe), so the depth is the art's offset (decorart's
sink), as the bulletin board is placed by its row and drawn by its art. A PS2 still sees the row's treant (Oakhollow): no
character file holds a fir, and zone 24 is never loaded in Freeport. The four light strings (lightstrings, on the NPC's
name) re-fit to the boughs at the new depth.

A report by default; --apply writes it with a gm_audit line a change and keeps the rows as they were in --applied;
--undo puts them back. The tables are read live: the GM panel's reload, or a season turn; a client already showing the
tree is sent the new art when the tree comes into its view again (a relog), the lights at once.

    christmas_fir_20261116.py --db eqoa.db [--apply | --undo] --applied FILE
"""
import argparse, json, os, sqlite3, sys, time

TREE, SRC = "Christmas Tree", "christmas-2026"
OLD_FIR, FIR, ZONE = 0x0E318233, 0xC4BE0726, 24
SINK = 11.0  # the lowest boughs' tips are 10.92 over the fir's foot: they meet the sand (measured by shots: 10.5 shows stem)
NOTE = ("the snow-dusted fir by Castle Lightwolf where Dust stood (TUNARIA.ESF 0xC4BE0726, zone 24), its trunk through the "
        "ground; a PS2 sees the row's treant")
# the four tiers in the fir's own frame at size 1, its foot SINK under the sand: r0 at y0 to r1 at y1, turns: the live
# strings (fitted to the same mesh sunk 13, seen on its boughs in the shots) raised the 2 the fir now stands higher
TIERS = [
    {"r0": 11.0, "y0": 0.9, "r1": 6.0, "y1": 2.6, "turns": 1.5},
    {"r0": 11.0, "y0": 6.6, "r1": 4.0, "y1": 11.0, "turns": 2.0},
    {"r0": 10.0, "y0": 14.5, "r1": 3.0, "y1": 18.8, "turns": 2.0},
    {"r0": 7.5, "y0": 20.5, "r1": 0.8, "y1": 35.2, "turns": 3.5, "top": "FFE070"},
]

ap = argparse.ArgumentParser()
ap.add_argument("--db", required=True)
ap.add_argument("--applied", required=True)
g = ap.add_mutually_exclusive_group()
g.add_argument("--apply", action="store_true")
g.add_argument("--undo", action="store_true")
a = ap.parse_args()
db = sqlite3.connect(a.db, timeout=30)
now = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def audit(action, tbl, key, before, after):
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) VALUES (?, 1, 'dab', 'tool', '', ?, ?, ?, ?, ?, ?)",
               (now, action, tbl, json.dumps(key), None if before is None else json.dumps(before, sort_keys=True),
                None if after is None else json.dumps(after, sort_keys=True),
                "the owner: the Christmas tree as the fir by Castle Lightwolf, its trunk through the ground; tools/christmas_fir_20261116.py"))


def art():
    r = db.execute("SELECT model, zone, sink, note FROM decorart WHERE name = ?", (TREE,)).fetchone()
    return None if r is None else {"model": r[0] & 0xFFFFFFFF, "zone": r[1], "sink": r[2], "note": r[3]}


def strings():
    return [(i, json.loads(p)) for i, p in db.execute(
        "SELECT id, params FROM lightstrings WHERE npc = ? AND source = ? AND shape = 'spiral' ORDER BY id", (TREE, SRC))]


if a.undo:
    j = json.load(open(a.applied))
    was = j["art"]
    db.execute("UPDATE decorart SET model = ?, zone = ?, sink = ?, note = ? WHERE name = ?", (was["model"], was["zone"], was["sink"], was["note"], TREE))
    audit("update", "decorart", {"name": TREE}, art(), was)
    for sid, p in j["strings"]:
        cur = db.execute("SELECT params FROM lightstrings WHERE id = ?", (sid,)).fetchone()
        db.execute("UPDATE lightstrings SET params = ? WHERE id = ?", (json.dumps(p), sid))
        audit("update", "lightstrings", {"id": sid}, {"params": cur and cur[0]}, {"params": json.dumps(p)})
    db.commit(); os.rename(a.applied, a.applied + ".undone"); print("undone"); sys.exit()

if a.apply and os.path.exists(a.applied):
    sys.exit("applied already")
was = art()
if was is None or was["model"] != OLD_FIR:
    sys.exit(f"the tree's decorart row is {was}, not the green fir {OLD_FIR:#010x}: nothing done")
ss = strings()
if len(ss) != len(TIERS):
    sys.exit(f"{len(ss)} spiral light strings on the tree, not {len(TIERS)}: nothing done")
db.execute("UPDATE decorart SET model = ?, zone = ?, sink = ?, note = ? WHERE name = ?", (FIR, ZONE, SINK, NOTE, TREE))
audit("update", "decorart", {"name": TREE}, was, art())
for (sid, p), t in zip(ss, TIERS):
    q = dict(p)
    q.update(t)
    db.execute("UPDATE lightstrings SET params = ? WHERE id = ?", (json.dumps(q), sid))
    audit("update", "lightstrings", {"id": sid}, {"params": json.dumps(p)}, {"params": json.dumps(q)})
print(f"{TREE}: art {was['model']:#010x} zone {was['zone']} sink {was['sink']} -> {FIR:#010x} zone {ZONE} sink {SINK}; "
      f"light strings {[s for s, _ in ss]} re-fit")
if a.apply:
    json.dump({"art": was, "strings": ss}, open(a.applied, "w")); db.commit(); print("applied")
else:
    db.rollback()
