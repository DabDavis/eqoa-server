#!/usr/bin/env python3
"""Star crabs and Grobb crabs all along the beach round Grobb (the user, 2026-10-30: Dust, on the
level 7 "Frog Legs for Grobb", found one of each: "should be all along beach around grobb including
the grobb crab also"; "Sandstorm also does it, but we dont need an oracle").

Every source we hold saw one of each (FrontiersForge's journal, then the import): a Grobb Crab and a
star crab 28 apart on the east beach, the two the quests send players to ("Frog Legs for Grobb": a
Grobb Crab's or a star crab's leg; Grothmal's star crab legs). So these places are OURS, read off the
server's own ground (cmd/groundbake's files, internal/ground's Floor and its water): the dry shore
within two cells (16) of the sea round Grobb's coach (25257, 31226), on sand 0.4 to 5.0 above the
sea's 51.0, 1,600 at most from the coach, 120 from any town's people (entity/towns.go's chains, as
tools/seed_wildlife.py keeps), 80 from any other player-race body (so a crab's wander of 75 keeps off
the shore's crusaders) and 20 from every other NPC, then one every 125 along the coast from the
east beach's north end round the south beach to its west end. Star crabs and Grobb crabs take turns.

Each spawn is a copy of its kind's own row (body, level 7, size and the rest), so loot, aggro,
classes and quest credit, all by name, are the same; dt_kill is cleared and the facing spread. Every
row is logged in beach_crab_seed; a second --apply is refused while it holds a seed; --undo deletes
exactly those rows still where this put them, puts npcs' sqlite_sequence back when nothing was
added after, and drops the log. Report only by default. The server reads npcs at start: apply with
the server stopped, or restart it after.

    tools/seed_grobb_beach_crabs.py [--db eqoa.db] [--apply | --undo]
"""

import argparse
import math
import sqlite3
import sys

# (x, y, z) along the coast; even places star crabs, odd Grobb Crabs
SPAWNS = [
    (23784, 52.63, 31224),
    (23752, 51.55, 31096),
    (25680, 53.45, 29688),
    (25680, 53.45, 29816),
    (25720, 54.12, 30168),
    (25736, 54.12, 30296),
    (25744, 53.47, 30424),
    (25768, 54.12, 30552),
    (25792, 53.29, 30680),
    (25800, 54.12, 30808),
    (25816, 54.01, 30936),
    (25808, 53.22, 31064),
    (25816, 52.56, 31192),
    (25880, 51.77, 31304),
    (25896, 51.70, 31432),
    (25904, 51.49, 31560),
    (25904, 51.49, 31688),
    (25896, 52.88, 31816),
    (25792, 54.12, 31888),
    (25664, 54.12, 31888),
    (25536, 53.28, 31896),
    (25408, 54.13, 31904),
    (25280, 53.17, 31912),
    (25152, 52.53, 31912),
    (25032, 54.12, 31872),
    (24920, 53.32, 31816),
    (24816, 52.91, 31744),
    (24696, 51.70, 31704),
    (24576, 52.46, 31584),
    (24480, 53.28, 31496),
    (24368, 52.20, 31432),
    (24256, 53.06, 31376),
    (24128, 52.58, 31352),
    (24000, 52.66, 31336),
]
KINDS = ("star crab", "Grobb Crab")
HOME = (25684.0, 30029.0)  # the two crabs every source saw

LOG_DDL = """CREATE TABLE beach_crab_seed (
    seq INTEGER PRIMARY KEY AUTOINCREMENT,
    npc INTEGER NOT NULL,
    name TEXT NOT NULL,
    x REAL NOT NULL,
    z REAL NOT NULL,
    seq_before INTEGER
)"""


def logged(db):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE name = 'beach_crab_seed'").fetchone()[0] > 0


def template(db, name):
    """Its kind's own row: world 0, the one nearest where every source saw it."""
    rows = db.execute("SELECT * FROM npcs WHERE lower(npc_name) = lower(?) AND world = 0", (name,))
    cols = [c[0] for c in rows.description]
    best = None
    for r in rows.fetchall():
        d = dict(zip(cols, r))
        dist = math.dist((d["x_coord"], d["z_coord"]), HOME)
        if dist <= 500 and (best is None or dist < best[0]):
            best = (dist, d)
    return None if best is None else best[1]


def plan(db):
    todo, problems = [], []
    kinds = {}
    for name in KINDS:
        t = template(db, name)
        if t is None:
            problems.append(f"no {name!r} of world 0 within 500 of {HOME}: nothing to copy")
        kinds[name] = t
    if problems:
        return todo, problems
    for i, (x, y, z) in enumerate(SPAWNS):
        t = kinds[KINDS[i % 2]]
        row = dict(t)
        del row["id"]
        row.update(x_coord=x, y_coord=y, z_coord=z, facing=(i * 97 + 31) % 256, dt_kill=None)
        todo.append(row)
    return todo, problems


def apply(db, todo):
    before = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'npcs'").fetchone()
    before = before[0] if before else None
    db.execute(LOG_DDL)
    for row in todo:
        cols = list(row)
        cur = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                         [row[c] for c in cols])
        db.execute("INSERT INTO beach_crab_seed (npc, name, x, z, seq_before) VALUES (?, ?, ?, ?, ?)",
                   (cur.lastrowid, row["npc_name"], row["x_coord"], row["z_coord"], before))
    return len(todo)


def undo(db):
    if not logged(db):
        print("nothing logged: nothing to undo")
        return
    back = kept = 0
    before = None
    for npc, name, x, z, sb in db.execute("SELECT npc, name, x, z, seq_before FROM beach_crab_seed ORDER BY seq DESC").fetchall():
        before = sb
        cur = db.execute("DELETE FROM npcs WHERE id = ? AND npc_name = ? AND x_coord = ? AND z_coord = ?", (npc, name, x, z))
        back, kept = (back + 1, kept) if cur.rowcount else (back, kept + 1)
    top = db.execute("SELECT COALESCE(MAX(id), 0) FROM npcs").fetchone()[0]
    if before is not None and top <= before:
        db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = 'npcs'", (before,))
    db.execute("DROP TABLE beach_crab_seed")
    print(f"took away {back}; left {kept} moved or renamed since")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", default="eqoa.db")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 10000")
    if a.undo:
        db.execute("BEGIN IMMEDIATE")
        undo(db)
        db.execute("COMMIT")
        return
    todo, problems = plan(db)
    for p in problems:
        print("PROBLEM:", p)
    n = {k: sum(1 for r in todo if r["npc_name"].lower() == k.lower()) for k in KINDS}
    print(f"{len(todo)} spawns along Grobb's beach: " + ", ".join(f"{v} {k}" for k, v in n.items()))
    if not a.apply:
        print("report only: --apply to write")
        return
    if problems:
        sys.exit("refused: the problems above first")
    if logged(db):
        sys.exit("refused: beach_crab_seed holds a seed already (--undo it first)")
    db.execute("BEGIN IMMEDIATE")
    k = apply(db, todo)
    db.execute("COMMIT")
    print(f"wrote {k} spawns; restart the server for the world to have them")


if __name__ == "__main__":
    main()
