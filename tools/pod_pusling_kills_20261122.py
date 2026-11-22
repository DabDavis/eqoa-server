#!/usr/bin/env python3
"""Every kill in the Plane of Disease raises The Pusling (Heirloom petition 2672: "every kill in the Plane of disease
raid area should give The Pusling faction +1. It is one of the three primary methods of increasing The Pusling
faction, which is required for several Plane of Disease quests").

The wiki's creature pages (claude-notes/pod-mobs-wiki-audit-20261113.md, its Factions column) give The Pusling as the
opposing faction of every PoD creature but the pusling lacers (of The Pusling themselves), and +25 a kill for the five
65 named. Already in: the diseased four +2, Bertoxxulous, Noroxus and the twins +100, the Queen +75, scuzz toad +1
(factionhits rows of other sources, left as they are).

  - factionhits, +1 The Pusling, for each attackable creature name that spawns only in PoD (world 5's zone 0) and has
    no Pusling hit; +25 for Fleshripper, Gluboxious the Fetid, Rallius Rattican, Rawlg the Carnage Hound, Rotwood the
    Hollow. Not the pusling lacer.
  - factionhitareas, +1 The Pusling, for the plague wasp in PoD alone: it spawns in the Rathe too.

What takes effect when: factionhits and factionhitareas are read at a start and whenever a GM edits factionhits in the
panel (LoadFactions reads both). factionhitareas is an admin's table, so the plague wasp's row waits for the next start
unless a factionhits edit follows it. The factionhits rows may go in through the panel first (insert_row); this tool
then finds them as written ("already there") and adds the rest.

Dry run by default. --apply writes in one transaction, a gm_audit line a row (account 1, dab, via tool), every row in
pod_pusling_1122_log; a second --apply writes nothing. --undo deletes what the log holds (a row changed since is left
and said).

    python3 -E -s tools/pod_pusling_kills_20261122.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "pod_pusling_1122_log"
WHO, ACCOUNT, VIA = "dab", 1, "tool"
TOOL = "tools/pod_pusling_kills_20261122.py"
SRC = "petition 2672"
DETAIL = f"Heirloom petition 2672, every PoD kill raises The Pusling: {TOOL}"
FACTION = "The Pusling"

# the attackable creature names of PoD (world 5, zone 0: x 4031-5985, z 2000-4000) with no Pusling hit, spawning
# nowhere else
ONE = [
    "Grum", "Last House Healer", "Last House Orderly", "Last House Pharmacist", "Last House Physician",
    "Last House Surgeon", "Lich Overlord", "Master Surgeon", "Sthlumchoxius", "cyanea virullata", "deadwood arborian",
    "filth squid", "last house toxicologist", "last house zombie", "locust larva", "locust monarch", "mantis larva",
    "mantis matriarch", "millipede larva", "millipede matriarch", "moth larva", "moth monarch",
    "patron of Bertoxxulous", "plague eel", "plague jelly", "plague piranha", "plague swine", "scourge rat",
    "surgeon's zombie", "toxic pollutive slime", "viscera scavenger",
]
NAMED = ["Fleshripper", "Gluboxious the Fetid", "Rallius Rattican", "Rawlg the Carnage Hound", "Rotwood the Hollow"]
HITS = [(n, 1) for n in ONE] + [(n, 25) for n in NAMED]
# (name, delta, world, x, z, radius): zone 0's box (x 4031-5985, z 2000-4000) inside the circle; Zaoran Plateau
# (z 6300 on) outside it
AREAS = [("plague wasp", 1, 5, 5008.0, 3000.0, 1400.0)]
POD = (5, 4031.0, 5985.0, 2000.0, 4000.0)
NOT = ("pusling lacer",)

LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq   INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl   TEXT NOT NULL,
    rid   INTEGER NOT NULL,
    after TEXT NOT NULL,
    audit INTEGER NOT NULL,
    at    TEXT NOT NULL
)"""


class Refused(Exception):
    pass


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def dumps(v):
    return json.dumps(v, sort_keys=True, separators=(",", ":"), ensure_ascii=False)


def row(db, tbl, rid):
    cur = db.execute(f'SELECT * FROM "{tbl}" WHERE rowid = ?', (rid,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def in_pod(world, x, z):
    w, x0, x1, z0, z1 = POD
    return world == w and x0 <= x <= x1 and z0 <= z <= z1


def plan(db):
    ops, lines = [], []
    if not db.execute("SELECT 1 FROM factions WHERE name = ?", (FACTION,)).fetchone():
        raise Refused(f"no faction {FACTION!r}")
    for name, delta in HITS:
        spots = db.execute("SELECT world, x_coord, z_coord, npc_type FROM npcs WHERE npc_name = ?", (name,)).fetchall()
        if not spots:
            raise Refused(f"{name}: no npcs row")
        out = [s for s in spots if not in_pod(s[0], s[1], s[2])]
        if out:
            raise Refused(f"{name}: {len(out)} of {len(spots)} rows outside PoD (an area row, not a name's)")
        if any((s[3] or 0) & 0x80 for s in spots):
            raise Refused(f"{name}: an NPC-bit row (no one may attack it)")
        have = db.execute("SELECT delta, source FROM factionhits WHERE npc_name = ? AND faction = ?",
                          (name, FACTION)).fetchone()
        if have:
            if have[0] == delta:
                lines.append(f"  {name}: {FACTION} {delta:+d} already there ({have[1]})")
                continue
            raise Refused(f"{name}: {FACTION} {have[0]:+d} there ({have[1]}), not {delta:+d}")
        ops.append(("factionhits", {"npc_name": name, "faction": FACTION, "delta": delta, "source": SRC}))
        lines.append(f"  {name}: {FACTION} {delta:+d} a kill ({len(spots)} spawns)")
    for name, delta, w, x, z, r in AREAS:
        have = db.execute("SELECT delta FROM factionhitareas WHERE npc_name = ? AND faction = ? AND world = ? "
                          "AND abs(x - ?) < 0.01 AND abs(z - ?) < 0.01", (name, FACTION, w, x, z)).fetchall()
        if have:
            if [h[0] for h in have] == [delta]:
                lines.append(f"  {name} in PoD: already there")
                continue
            raise Refused(f"{name}: area rows {have} there")
        ops.append(("factionhitareas", {"npc_name": name, "faction": FACTION, "delta": delta, "world": w, "x": x,
                                        "z": z, "radius": r, "source": SRC,
                                        "note": f"petition 2672: every PoD kill raises {FACTION} (it spawns in the "
                                                f"Rathe too). {TOOL}"}))
        lines.append(f"  {name} in PoD (world {w}, ({x:.0f}, {z:.0f}) within {r:.0f}): {FACTION} {delta:+d} a kill")
    for n in NOT:
        if db.execute("SELECT 1 FROM factionhits WHERE npc_name = ? AND faction = ? AND delta > 0", (n, FACTION)).fetchone():
            raise Refused(f"{n} raises {FACTION}: a Pusling's own kill")
    return ops, lines


def audit(db, action, tbl, key, before, after):
    return db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                      "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                      (now(), ACCOUNT, WHO, VIA, action, tbl, dumps(key), None if before is None else dumps(before),
                       None if after is None else dumps(after), DETAIL)).lastrowid


def apply(db, ops):
    db.execute(LOG_DDL)
    at = now()
    for tbl, v in ops:
        cs = list(v)
        rid = db.execute(f'INSERT INTO "{tbl}" ({", ".join(cs)}) VALUES ({", ".join("?" * len(cs))})',
                         [v[c] for c in cs]).lastrowid
        after = row(db, tbl, rid)
        a = audit(db, "insert", tbl, {"id": after["id"]}, None, after)
        db.execute(f"INSERT INTO {LOG} (tbl, rid, after, audit, at) VALUES (?, ?, ?, ?, ?)", (tbl, rid, dumps(after), a, at))
    return len(ops)


def undo(db):
    if not db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (LOG,)).fetchone():
        return 0, []
    back, left = 0, []
    for seq, tbl, rid, after in db.execute(f"SELECT seq, tbl, rid, after FROM {LOG} ORDER BY seq DESC").fetchall():
        have = row(db, tbl, rid)
        if have is None or dumps(have) != after:
            left.append(f"{tbl} row {rid}: changed since, left as it is")
            continue
        db.execute(f'DELETE FROM "{tbl}" WHERE rowid = ?', (rid,))
        audit(db, "delete", tbl, {"id": have["id"]}, have, None)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if not db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0]:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("PRAGMA busy_timeout = 5000")
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"undone: {back} rows deleted" + "".join(f"\n  left: {w}" for w in left))
            return 0
        try:
            ops, lines = plan(db)
        except Refused as e:
            db.execute("ROLLBACK")
            print(f"REFUSED, nothing written: {e}")
            return 1
        print("\n".join(lines))
        print(f"totals: {sum(t == 'factionhits' for t, _ in ops)} factionhits, "
              f"{sum(t == 'factionhitareas' for t, _ in ops)} factionhitareas to insert")
        if not a.apply or not ops:
            db.execute("ROLLBACK")
            print("dry run: nothing written (--apply to write)" if not a.apply else "nothing to write: all in place")
            return 0
        n = apply(db, ops)
        db.execute("COMMIT")
        print(f"applied: {n} rows, a gm_audit line each, logged in {LOG}")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
