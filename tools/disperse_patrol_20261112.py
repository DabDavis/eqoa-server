#!/usr/bin/env python3
"""Spread stacked camps and make standing creatures roam, world by world (the owner, 2026-11-12: "there are many zones
with mobs not dispersed or patroling, lets fix that disperse and patrol"; "skeletal giant and others are not roaming
correctly, they only move a small distance"; "ship the disperse fix world by world"; the GM Jibrish's petitions
1734-1737, tools/disperse_patrol_20261112_gm.tsv).

The plan is cmd/disperseplan's (tools/disperse_patrol_20261112_plan.json, made from Heirloom's database of 2026-11-12
04:10 UTC and the baked ground; every new spot is one the creature's wander could walk to from its old one: a floor all
the way, no wall, no fall, not into the water, within 12 up or down, a body's width and more from every other creature):

  - moves: npcs rows' x_coord, y_coord, z_coord, facing. A camp stacked on one spot has its stacked rows spread (one
    of each stack keeps its place); a camp laid out in rows (an import's grid: one of each kind in even lines) and a
    GM's camp are spread whole over their ground. A creature's own pet goes with it.
  - wander: table npcwander (a row's own wander, database/npcwander.go; the server from commit 5e287de8 reads it at
    the start): a GM's camps roam (radius 100+); one in four of a standing kind's camp wanders round it (45); a lone
    creature of a standing kind wanders (the server's 70); the far roamers (giants, cyclopes, the guides' named
    roamers) roam their ground (150-300, walking longer legs).

Left alone (the plan's "left"): rows a GM or a camp report placed or moved (gm_audit), rows within 60 of a camp
report, rows a quest ties by id (queststeps.npcid, questwalls), rows with a path, a /camp spawn or a wander row, pets
(but with their owners), townsfolk, guards and kinds keeping a post, the NPC bit, named creatures (a name of two rows
or fewer in its world), kinds a GM's npcmove row stands, creatures off the ground or in the water (not moved).

--world N applies, or undoes, one world alone; each world's apply is its own and its undo touches nothing else.
A row not as the plan found it (a GM moved it since, it is gone, it has a wander row now) is skipped and named, never
touched. Each change gets a gm_audit line and a line of the log table disperse_1112_log (its before, its after, its
audit line); --undo restores each row unchanged since (one changed since is left and named), deletes its wander row,
its audit lines and its log lines; the table npcwander is dropped again when this tool made it, every world is undone and it is empty.

Needs a restart: NPC positions and npcwander are read at the start (and the binary that reads npcwander).

    tools/disperse_patrol_20261112.py --db eqoa.db [--world N] [--apply | --undo] [--plan PLAN] [--maps DIR]
"""
import argparse
import datetime
import json
import math
import os
import sqlite3
import sys

LOG = "disperse_1112_log"
WHO, VIA = "dab", "tool"
SOURCE = "disperse"
DETAIL = "spread stacked camps, standing creatures roam (the owner, 2026-11-12; petitions 1734-1737): tools/disperse_patrol_20261112.py"
HERE = os.path.dirname(os.path.abspath(__file__))
PLAN = os.path.join(HERE, "disperse_patrol_20261112_plan.json")
WORLDS = {0: "Tunaria", 1: "Rathe", 2: "Odus", 3: "Lavastorm", 4: "Plane of Sky", 5: "Plane of Disease", 10: "Homelands"}
EPS = 0.01

WANDER_SCHEMA = """
	CREATE TABLE IF NOT EXISTS npcwander (
		npcid  INTEGER PRIMARY KEY,
		radius REAL NOT NULL DEFAULT 0,
		note   TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm'
	);"""

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    world  INTEGER NOT NULL,
    kind   TEXT NOT NULL,      -- move | wander | made (npcwander made by this tool)
    npcid  INTEGER NOT NULL,
    before TEXT,
    after  TEXT,
    audit  INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def compact(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False, sort_keys=True)


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def audit(db, action, tbl, rowid, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (now(), acct[0] if acct else 0, WHO, VIA, action, tbl, compact({"id" if tbl == "npcs" else "npcid": rowid}),
                      compact(before) if before else "", compact(after) if after else "", DETAIL))
    return cur.lastrowid


def load_plan(path):
    with open(path) as f:
        return json.load(f)


def pos_of(db, npcid):
    r = db.execute("SELECT x_coord, y_coord, z_coord, facing, npc_name, world FROM npcs WHERE id = ?", (npcid,)).fetchone()
    if r is None:
        return None
    return {"X": r[0], "Y": r[1], "Z": r[2], "Facing": r[3], "name": r[4], "world": r[5]}


def same(p, q):
    return all(abs(float(p[k]) - float(q[k])) <= EPS for k in ("X", "Y", "Z")) and int(p["Facing"]) == int(q["Facing"])


def logged(db, world):
    if not table(db, LOG):
        return set()
    return {(k, i) for k, i in db.execute(f"SELECT kind, npcid FROM {LOG} WHERE world = ?", (world,))}


def check(db, plan, world):
    """What an apply of world would do: (moves, wanders, skipped [(id, name, why)], done already)."""
    done = logged(db, world)
    moves, wanders, skipped = [], [], []
    have_wander = table(db, "npcwander")
    for m in plan["moves"]:
        if m["world"] != world:
            continue
        if ("move", m["id"]) in done:
            continue
        p = pos_of(db, m["id"])
        if p is None:
            skipped.append((m["id"], m["name"], "gone"))
        elif p["world"] != world or p["name"] != m["name"]:
            skipped.append((m["id"], m["name"], f"now {p['name']!r} in world {p['world']}"))
        elif not same(p, m["before"]):
            skipped.append((m["id"], m["name"], f"moved since the plan (at {p['X']:.1f}, {p['Z']:.1f})"))
        else:
            moves.append(m)
    for wd in plan["wander"]:
        if wd["world"] != world:
            continue
        if ("wander", wd["id"]) in done:
            continue
        p = pos_of(db, wd["id"])
        if p is None:
            skipped.append((wd["id"], wd["name"], "gone (wander)"))
        elif p["world"] != world or p["name"] != wd["name"]:
            skipped.append((wd["id"], wd["name"], f"now {p['name']!r} in world {p['world']} (wander)"))
        elif have_wander and db.execute("SELECT 1 FROM npcwander WHERE npcid = ?", (wd["id"],)).fetchone():
            skipped.append((wd["id"], wd["name"], "has a wander row already"))
        else:
            wanders.append(wd)
    return moves, wanders, skipped, len(done)


def apply(db, plan, world):
    moves, wanders, skipped, _ = check(db, plan, world)
    db.execute(LOG_SCHEMA)
    if not table(db, "npcwander"):
        db.execute(WANDER_SCHEMA)
        db.execute(f"INSERT INTO {LOG} (world, kind, npcid) VALUES (-1, 'made', 0)")  # any world's
    for m in moves:
        b, a = m["before"], m["after"]
        before = {"x_coord": b["X"], "y_coord": b["Y"], "z_coord": b["Z"], "facing": b["Facing"]}
        after = {"x_coord": round(a["X"], 3), "y_coord": round(a["Y"], 3), "z_coord": round(a["Z"], 3), "facing": a["Facing"]}
        cur = db.execute("SELECT x_coord, y_coord, z_coord, facing FROM npcs WHERE id = ?", (m["id"],)).fetchone()
        before = {"x_coord": cur[0], "y_coord": cur[1], "z_coord": cur[2], "facing": cur[3]}  # exactly as it was
        db.execute("UPDATE npcs SET x_coord = ?, y_coord = ?, z_coord = ?, facing = ? WHERE id = ?",
                   (after["x_coord"], after["y_coord"], after["z_coord"], after["facing"], m["id"]))
        aid = audit(db, "update", "npcs", m["id"], before, after)
        db.execute(f"INSERT INTO {LOG} (world, kind, npcid, before, after, audit) VALUES (?, 'move', ?, ?, ?, ?)",
                   (world, m["id"], compact(before), compact(after), aid))
    for wd in wanders:
        row = {"npcid": wd["id"], "radius": round(wd["radius"], 1), "note": wd["note"][:200], "source": SOURCE}
        db.execute("INSERT INTO npcwander (npcid, radius, note, source) VALUES (?, ?, ?, ?)",
                   (row["npcid"], row["radius"], row["note"], row["source"]))
        aid = audit(db, "insert", "npcwander", wd["id"], None, row)
        db.execute(f"INSERT INTO {LOG} (world, kind, npcid, after, audit) VALUES (?, 'wander', ?, ?, ?)",
                   (world, wd["id"], compact(row), aid))
    return len(moves), len(wanders), skipped


def undo(db, world):
    if not table(db, LOG):
        return 0, 0, []
    back = 0
    left = []
    audits = []
    for seq, kind, npcid, before, after, aid in db.execute(
            f"SELECT seq, kind, npcid, before, after, audit FROM {LOG} WHERE world = ? ORDER BY seq DESC", (world,)).fetchall():
        if kind == "move":
            a, b = json.loads(after), json.loads(before)
            cur = db.execute("SELECT x_coord, y_coord, z_coord, facing FROM npcs WHERE id = ?", (npcid,)).fetchone()
            if cur is None or any(abs(float(cur[i]) - float(a[k])) > EPS for i, k in enumerate(("x_coord", "y_coord", "z_coord"))) \
                    or int(cur[3]) != int(a["facing"]):
                left.append((npcid, "gone" if cur is None else "moved since"))
                continue
            db.execute("UPDATE npcs SET x_coord = ?, y_coord = ?, z_coord = ?, facing = ? WHERE id = ?",
                       (b["x_coord"], b["y_coord"], b["z_coord"], b["facing"], npcid))
        elif kind == "wander":
            a = json.loads(after)
            cur = db.execute("SELECT npcid, radius, note, source FROM npcwander WHERE npcid = ?", (npcid,)).fetchone()
            if cur is None or dict(zip(("npcid", "radius", "note", "source"), cur)) != a:
                left.append((npcid, "wander row gone or changed since"))
                continue
            db.execute("DELETE FROM npcwander WHERE npcid = ?", (npcid,))
        elif kind == "made":
            continue
        audits.append(aid)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    for aid in audits:
        db.execute("DELETE FROM gm_audit WHERE id = ?", (aid,))
    if audits and table(db, "sqlite_sequence"):
        top = db.execute("SELECT COALESCE(max(id), 0) FROM gm_audit").fetchone()[0]
        db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = 'gm_audit' AND seq > ?", (top, top))
    if db.execute(f"SELECT count(*) FROM {LOG} WHERE kind != 'made'").fetchone()[0] == 0:
        # no world's changes left: the table npcwander goes when this tool made it and it is empty, and the log
        made = db.execute(f"SELECT 1 FROM {LOG} WHERE kind = 'made'").fetchone()
        if made and db.execute("SELECT count(*) FROM npcwander").fetchone()[0] == 0:
            db.execute("DROP TABLE npcwander")
        elif made:
            return back, len(left), left  # rows of someone else's in it: the mark stays
        db.execute(f"DROP TABLE {LOG}")
        if table(db, "sqlite_sequence"):
            db.execute("DELETE FROM sqlite_sequence WHERE name = ?", (LOG,))
    return back, len(left), left


def summary(plan, world):
    mv = [m for m in plan["moves"] if m["world"] == world]
    wd = [w for w in plan["wander"] if w["world"] == world]
    camps = [c for c in plan["camps"] if c["world"] == world]
    by = {}
    for w in wd:
        k = w["note"].split(":")[0] if w["note"].startswith("GM") else w["note"]
        by[k] = by.get(k, 0) + 1
    print(f"world {world} ({WORLDS.get(world, '?')}): {len(camps)} camps spread "
          f"({sum(1 for c in camps if c['why'] == 'stacked')} stacked, {sum(1 for c in camps if c['why'] == 'rows')} in rows, "
          f"{sum(1 for c in camps if c['why'] == 'gm')} a GM's), {len(mv)} rows moved; {len(wd)} wander rows:")
    for k, n in sorted(by.items(), key=lambda kv: -kv[1]):
        print(f"    {n:5d}  {k}")
    for c in camps:
        if c.get("gm"):
            ids = [m["id"] for m in mv if m["camp"] == c["id"]]
            print(f"  GM camp {c['gm'][:60]}: {c['n']} rows ({c['kinds']}), {c['moved']} moved over radius {c['radius']:.0f}, rows {ids}")


def maps(db, plan, outdir, worlds):
    import matplotlib
    matplotlib.use("Agg")
    import matplotlib.pyplot as plt
    os.makedirs(outdir, exist_ok=True)
    for world in worlds:
        mv = [m for m in plan["moves"] if m["world"] == world]
        wd = [w for w in plan["wander"] if w["world"] == world]
        camps = [c for c in plan["camps"] if c["world"] == world]
        if not mv and not wd:
            continue
        pts = db.execute("SELECT x_coord, z_coord FROM npcs WHERE world = ?", (world,)).fetchall()
        fig, ax = plt.subplots(figsize=(16, 16), dpi=150)
        ax.scatter([p[0] for p in pts], [p[1] for p in pts], s=0.3, c="#bbbbbb", lw=0, label="every NPC (now)")
        ax.scatter([m["before"]["X"] for m in mv], [m["before"]["Z"] for m in mv], s=2, c="#d62728", lw=0, label="moved: before")
        ax.scatter([m["after"]["X"] for m in mv], [m["after"]["Z"] for m in mv], s=2, c="#2ca02c", lw=0, label="moved: after")
        kinds = {"GM": "#9467bd", "a far": "#ff7f0e", "one in": "#1f77b4", "a lone": "#17becf"}
        for k, col in kinds.items():
            sel = [w for w in wd if w["note"].startswith(k)]
            if sel:
                ax.scatter([w["x"] for w in sel], [w["z"] for w in sel], s=4, c=col, marker="x", lw=0.5,
                           label=f"wanders: {k}... ({len(sel)})")
        for c in camps:
            if c.get("gm"):
                ax.annotate(c["gm"][:14], (c["x"], c["z"]), fontsize=7, color="#9467bd")
        ax.set_aspect("equal")
        ax.invert_yaxis()  # +z is south: north up
        ax.set_title(f"world {world} ({WORLDS.get(world, '?')}): {len(mv)} rows spread, {len(wd)} wander rows (north up)")
        ax.legend(loc="upper right", markerscale=4, fontsize=8)
        fig.savefig(os.path.join(outdir, f"world{world}.png"))
        plt.close(fig)
        # the GMs' camps and the biggest spreads, close up: before (red) to after (green), wander radius rings
        zoom = [c for c in camps if c.get("gm")] + sorted([c for c in camps if not c.get("gm")], key=lambda c: -c["moved"])[:8]
        for c in zoom:
            fig, ax = plt.subplots(figsize=(7, 7), dpi=120)
            r = max(c["radius"] * 1.6, 60)
            near = db.execute("SELECT x_coord, z_coord FROM npcs WHERE world = ? AND x_coord BETWEEN ? AND ? AND z_coord BETWEEN ? AND ?",
                              (world, c["x"] - r, c["x"] + r, c["z"] - r, c["z"] + r)).fetchall()
            ax.scatter([p[0] for p in near], [p[1] for p in near], s=6, c="#bbbbbb", lw=0)
            for m in mv:
                if m["camp"] == c["id"]:
                    ax.plot([m["before"]["X"], m["after"]["X"]], [m["before"]["Z"], m["after"]["Z"]], c="#999999", lw=0.5)
                    ax.scatter([m["before"]["X"]], [m["before"]["Z"]], s=14, c="#d62728", lw=0)
                    ax.scatter([m["after"]["X"]], [m["after"]["Z"]], s=14, c="#2ca02c", lw=0)
                    a = math.radians(m["after"]["Facing"] * 360 / 256)
                    ax.plot([m["after"]["X"], m["after"]["X"] + 3 * math.sin(a)], [m["after"]["Z"], m["after"]["Z"] + 3 * math.cos(a)], c="#2ca02c", lw=0.8)
            ax.add_patch(plt.Circle((c["x"], c["z"]), c["radius"], fill=False, ls="--", color="#555555"))
            ax.set_xlim(c["x"] - r, c["x"] + r)
            ax.set_ylim(c["z"] + r, c["z"] - r)
            ax.set_aspect("equal")
            ax.set_title(f"w{world} camp {c['id']} ({c['why']}{', ' + c['gm'][:30] if c.get('gm') else ''})\n{c['kinds'][:70]}\nred before, green after (tick = facing)", fontsize=8)
            name = f"world{world}-camp{c['id']}{'-gm' if c.get('gm') else ''}.png"
            fig.savefig(os.path.join(outdir, name))
            plt.close(fig)
        print(f"maps of world {world} in {outdir}")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    ap.add_argument("--plan", default=PLAN)
    ap.add_argument("--world", type=int, help="this world alone (default: every world in the plan)")
    ap.add_argument("--maps", help="draw the plan's maps into this directory")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    plan = load_plan(a.plan)
    worlds = sorted({m["world"] for m in plan["moves"]} | {w["world"] for w in plan["wander"]})
    if a.world is not None:
        worlds = [a.world]
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    if a.maps:
        maps(db, plan, a.maps, worlds)
        return 0
    status = 0
    for world in worlds:
        if a.undo:
            db.execute("BEGIN IMMEDIATE")
            try:
                back, nleft, left = undo(db, world)
                db.execute("COMMIT")
            except BaseException:
                db.execute("ROLLBACK")
                raise
            for npcid, why in left:
                print(f"  left: npcs {npcid}: {why}")
            print(f"world {world}: undid {back} changes, {nleft} left" if back or nleft else f"world {world}: nothing to undo")
            status |= 1 if nleft else 0
            continue
        moves, wanders, skipped, done = check(db, plan, world)
        if not a.apply:
            summary(plan, world)
        for i, n, why in skipped:
            print(f"  skipped: npcs {i} {n!r}: {why}")
        if not moves and not wanders:
            print(f"world {world}: nothing to do" + (f" ({done} changes applied already)" if done else ""))
            continue
        if not a.apply:
            print(f"world {world}: report only: {len(moves)} rows would move, {len(wanders)} wander rows be written, "
                  f"{len(skipped)} skipped (--apply --world {world} writes them)")
            continue
        db.execute("BEGIN IMMEDIATE")
        try:
            nm, nw, skipped = apply(db, plan, world)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"world {world}: moved {nm} rows, wrote {nw} wander rows, skipped {len(skipped)}")
    return status


if __name__ == "__main__":
    sys.exit(main())
