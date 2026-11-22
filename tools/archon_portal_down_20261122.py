#!/usr/bin/env python3
"""A portal down from each Archon's floor in Geomancer's Citadel (Heirloom petition 2670: "When killing Archon Vetash, no
portal spawns to allow you to go back down to the Geomancers Citadel"). The guide (EQOA-Data Quests/Misc/archon-aereth-
archon-vetash-and-primeval-elements) has the portals up and says nothing of a way down: this is ours.

  - Two spawn-only npcs rows, copies of the Portal to Archon Aereth's (its body, size, level and type 384) named
    "Portal down from Aereth" and "Portal down from Vetash" (23 letters at most: the state channel's name is 24 bytes),
    each on its Archon's floor away from what stands there: Aereth's at (4695, 110.375, 2493), by the portal up's
    landing (4702, 2492) and clear of the quest Sarek's spot (tools/sarek_709_20261122.py: the room's middle); Vetash's
    at (4697, 106, 2353), by his landing (4700, 2357). Both on the floor as cmd/campground reads the ground.
  - Two triggers, "Aereth Portal Down" and "Vetash Portal Down": a lock of their own (one portal at a time, though the
    Archon rises again inside the ten minutes), the portal stood at its spot for 10 minutes, a line to the room.
  - npcdeath "archon aereth" and "archon vetash", 100%, firing them.
  The talk (handlers/waysender.go, with the code) sets a player down on the tower's ground floor between the portals up.

Sarek's quest run hides Aereth by name and holds the lock "sarek the betrayer": neither touches the portal down, and
none of its rows is touched here.

What takes effect when: npcs rows, spawn-only names, triggers and npcdeath are read at a start; the waysender entries
come with the binary. Apply in the before.sh of the keep restart that brings them (a portal of the old binary would say
nothing).

Dry run by default. --apply writes in one transaction, a gm_audit line a row (account 1, dab, via tool), every row's
before and after in archon_down_1122_log; a second --apply writes nothing. --undo puts back what the log holds, latest
first; a row changed since is left and said.

    python3 -E -s tools/archon_portal_down_20261122.py --db eqoa.db --shard heirloom|live [--apply | --undo | --dump]
"""
import argparse
import datetime
import json
import math
import sqlite3
import sys

LOG = "archon_down_1122_log"
WHO, ACCOUNT, VIA = "dab", 1, "tool"
TOOL = "tools/archon_portal_down_20261122.py"
SRC = "petition 2670 (archon_portal_down_20261122)"
DETAIL = f"Heirloom petition 2670, a portal down from each Archon's floor in Geomancer's Citadel: {TOOL}"

UP = "Portal to Archon Aereth"
UP_AT = (1, 4692.0, 2512.0)
UP_SHAPE = {"modelid": 2012826467, "npc_type": 384}
NEAR = 3.0

# (Archon, his spot, the portal's name, its spot, the facing byte, the compass word, the trigger, its lock)
TOWERS = [
    ("Archon Aereth", (1, 4699.23, 2512.66), "Portal down from Aereth", (1, 4695.0, 110.375, 2493.0), 0, "south",
     "Aereth Portal Down", "aereth portal down"),
    ("Archon Vetash", (1, 4699.55, 2342.21), "Portal down from Vetash", (1, 4697.0, 106.0, 2353.0), 128, "north",
     "Vetash Portal Down", "vetash portal down"),
]
MINUTES = 10
RADIUS = 25

LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    rid    INTEGER NOT NULL,  -- the row's rowid
    rowkey TEXT NOT NULL,     -- its key (JSON)
    before TEXT,              -- the whole row before (JSON; NULL: none)
    after  TEXT,              -- the whole row after (JSON; NULL: removed)
    audit  INTEGER NOT NULL,
    at     TEXT NOT NULL
)"""


class Refused(Exception):
    pass


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def dumps(v):
    s = json.dumps(v, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def jsc(v):
    """A JSON column as the rows keep it (key order kept)."""
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False)


def num(v):
    """A whole float as an int, for the steps' JSON (4695.0 -> 4695)."""
    return int(v) if float(v).is_integer() else v


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def pk(db, tbl):
    p = sorted((r[5], r[1]) for r in db.execute(f'PRAGMA table_info("{tbl}")') if r[5])
    return [c for _, c in p]


def rows(db, tbl, where):
    w = " AND ".join(f'"{c}" = ?' for c in where) or "1"
    cur = db.execute(f'SELECT rowid AS _rowid_, * FROM "{tbl}" WHERE {w} ORDER BY rowid', tuple(where.values()))
    names = [d[0] for d in cur.description]
    return [dict(zip(names, r)) for r in cur.fetchall()]


def by_rowid(db, tbl, rid):
    r = rows(db, tbl, {"rowid": rid})
    return r[0] if r else None


def plain(r):
    return None if r is None else {k: v for k, v in r.items() if k != "_rowid_"}


def same(a, b):
    if isinstance(a, (int, float)) and isinstance(b, (int, float)) and not isinstance(a, bool):
        return abs(float(a) - float(b)) < 1e-6
    return a == b


def trigger_steps(name, at, face, lock, archon):
    w, x, y, z = at
    return [
        {"do": "lock", "lock": lock},
        {"do": "spawn", "npc": name, "spread": 0.01, "minutes": MINUTES, "world": w, "x": num(x), "y": num(y),
         "z": num(z), "face": face},
        {"do": "broadcast", "text": f"A portal shimmers open where {archon} fell. It leads down to the tower's floor "
                                    f"and closes in ten minutes."},
    ]


class Plan:
    def __init__(self, db, shard):
        self.db, self.shard = db, shard
        self.ops = []      # (tbl, rid or None, before row or None, after values or None)
        self.lines = []
        self.next_npc = None
        self.made = {}

    def say(self, s):
        self.lines.append(f"  {s}")

    def add(self, tbl, values, match, what):
        """Insert values unless a row matching match is there: as values (done) or otherwise (refused)."""
        have = rows(self.db, tbl, {c: values[c] for c in match})
        if have:
            if len(have) == 1 and all(same(have[0][c], v) for c, v in values.items()):
                self.say(f"{what}: already there")
                return False
            raise Refused(f"{what}: {tbl} has {dumps([plain(h) for h in have])[:400]}, not {dumps(values)[:400]}")
        self.ops.append((tbl, None, None, dict(values)))
        self.say(f"{what}: insert {tbl} {short(dumps(values), 240)}")
        return True

    def npc_id(self):
        if self.next_npc is None:
            seq = self.db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'npcs'").fetchone() \
                if table(self.db, "sqlite_sequence") else None
            self.next_npc = max(seq[0] if seq else 0,
                                self.db.execute("SELECT COALESCE(MAX(id), 0) FROM npcs").fetchone()[0]) + 1
        n, self.next_npc = self.next_npc, self.next_npc + 1
        return n


def short(v, n=120):
    s = v if isinstance(v, str) else repr(v)
    return s if len(s) <= n else s[:n] + "..."


def made_by_us(db, tbl, key):
    return table(db, LOG) and db.execute(f"SELECT 1 FROM {LOG} WHERE tbl = ? AND before IS NULL AND rowkey = ?",
                                         (tbl, dumps(key))).fetchone() is not None


def trigger_slot(db, name, taken):
    r = db.execute("SELECT slot FROM gmtriggers WHERE name = ?", (name,)).fetchone()
    if r:
        return r[0]
    return max((db.execute("SELECT max(slot) FROM gmtriggers").fetchone()[0] or 8), *taken, 0) + 1


def build(p):
    db = p.db
    for t in ("npcs", "npcspawnonly", "gmtriggers", "npcdeath", "gm_audit"):
        if not table(db, t):
            raise Refused(f"no table {t}")
    ups = [r for r in rows(db, "npcs", {"npc_name": UP, "world": UP_AT[0]})
           if math.hypot(r["x_coord"] - UP_AT[1], r["z_coord"] - UP_AT[2]) <= NEAR]
    if len(ups) != 1 or any(ups[0][c] != v for c, v in UP_SHAPE.items()):
        raise Refused(f"{UP}: {[(r['id'], r['modelid'], r['npc_type']) for r in ups]} at its place, not one as read "
                      f"{UP_SHAPE}")
    up = ups[0]
    slots = []
    for archon, a_at, name, at, facing, face, trig, lock in TOWERS:
        w, x, y, z = at
        here = [r for r in rows(db, "npcs", {"npc_name": archon, "world": a_at[0]})
                if math.hypot(r["x_coord"] - a_at[1], r["z_coord"] - a_at[2]) <= NEAR]
        if len(here) != 1:
            raise Refused(f"{archon}: {len(here)} rows at his spot, not one")
        if db.execute("SELECT count(*) FROM npcs WHERE lower(npc_name) = lower(?)", (archon,)).fetchone()[0] != 1:
            raise Refused(f"{archon}: more than one row by the name (npcdeath would fire at each)")

        # the portal's row: this tool's, else made
        named = rows(db, "npcs", {"npc_name": name})
        ours = [r for r in named if made_by_us(db, "npcs", {"id": r["id"]})]
        if len(named) != len(ours) or len(ours) > 1:
            raise Refused(f"rows named {name!r}: {[r['id'] for r in named]}, not this tool's one")
        if ours:
            rid = ours[0]["id"]
            p.say(f"{name}: row {rid} there")
        else:
            r = plain(up)
            rid = p.npc_id()
            if r.get("npc_id") == up["id"]:
                r["npc_id"] = rid
            r.update(id=rid, npc_name=name, world=w, x_coord=x, y_coord=y, z_coord=z, facing=facing, dt_kill=None)
            p.add("npcs", r, ("id",), f"{name} row {rid} on {archon}'s floor, a copy of {UP} #{up['id']}")
        p.add("npcspawnonly", {"name": name.lower(), "note": f"Stood on {archon}'s floor {MINUTES} minutes at his death "
                                                             f"by the trigger {trig!r} (npcdeath); handlers/waysender.go "
                                                             f"sends from it to the tower's ground floor. {TOOL}",
                               "source": SRC}, ("name",), f"{name} spawn-only")

        slot = trigger_slot(db, trig, slots)
        slots.append(slot)
        p.add("gmtriggers", {"slot": slot, "name": trig, "enabled": 1, "scope": "near", "radius": RADIUS, "world": w,
                             "x": x, "y": y, "z": z, "steps": jsc(trigger_steps(name, at, face, lock, archon)),
                             "notes": f"Heirloom petition 2670: {archon}'s death (npcdeath) stands {name} on his floor "
                                      f"{MINUTES} minutes, one at a time (its lock); it sets a player down on the "
                                      f"tower's ground floor (handlers/waysender.go). {TOOL}"},
              ("name",), f"trigger {trig!r} (slot {slot})")
        others = [r["trigger"] for r in rows(db, "npcdeath", {"name": archon.lower()}) if r["trigger"] != trig]
        if others:
            p.say(f"{archon}'s death fires as well: {others}")
        p.add("npcdeath", {"name": archon.lower(), "trigger": trig, "chance": 100,
                           "note": f"Heirloom petition 2670: the portal down from his floor. {TOOL}", "source": SRC},
              ("name", "trigger"), f"npcdeath {archon.lower()!r} -> {trig!r}")
        p.made[name] = rid


# ---------------------------------------------------------------------------------------------- writing

def keyof(db, tbl, row, rid):
    k = pk(db, tbl)
    return {c: row[c] for c in k} if k else {"rowid": rid}


def audit(db, action, tbl, key, before, after):
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (now(), ACCOUNT, WHO, VIA, action, tbl, dumps(key), None if before is None else dumps(before),
                      None if after is None else dumps(after), DETAIL))
    return cur.lastrowid


def apply(db, p):
    db.execute(LOG_DDL)
    at, n = now(), 0
    for tbl, _, _, new in p.ops:
        cs = list(new)
        cur = db.execute(f'INSERT INTO "{tbl}" ({", ".join(chr(34) + c + chr(34) for c in cs)}) '
                         f'VALUES ({", ".join("?" * len(cs))})', [new[c] for c in cs])
        rid = cur.lastrowid
        after = plain(by_rowid(db, tbl, rid))
        key = keyof(db, tbl, after, rid)
        a = audit(db, "insert", tbl, key, None, after)
        db.execute(f"INSERT INTO {LOG} (tbl, rid, rowkey, before, after, audit, at) VALUES (?, ?, ?, NULL, ?, ?, ?)",
                   (tbl, rid, dumps(key), dumps(after), a, at))
        n += 1
    return n


def undo(db):
    if not table(db, LOG):
        return 0, []
    back, left = 0, []
    for seq, tbl, rid, rowkey, before, after in db.execute(
            f"SELECT seq, tbl, rid, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        have = plain(by_rowid(db, tbl, rid))
        if have is None or dumps(have) != after:
            left.append(f"{tbl} {rowkey}: changed since, left as it is")
            continue
        b = None if before is None else json.loads(before)
        if b is None:
            db.execute(f'DELETE FROM "{tbl}" WHERE rowid = ?', (rid,))
            action = "delete"
        else:
            db.execute(f'UPDATE "{tbl}" SET {", ".join(chr(34) + c + chr(34) + " = ?" for c in b)} WHERE rowid = ?',
                       [*b.values(), rid])
            action = "update"
        audit(db, action, tbl, json.loads(rowkey), have, b)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if not db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0]:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def dump(p):
    out = {"made": p.made, "triggers": {}}
    for tbl, _, _, v in p.ops:
        if tbl == "gmtriggers":
            out["triggers"][v["name"]] = v
    return out


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--shard", required=True, choices=("heirloom", "live"))
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    g.add_argument("--dump", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("PRAGMA busy_timeout = 5000")
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"undone: {back} rows put back" + "".join(f"\n  left: {w}" for w in left))
            return 0
        p = Plan(db, a.shard)
        try:
            build(p)
        except Refused as e:
            db.execute("ROLLBACK")
            print("\n".join(p.lines))
            print(f"REFUSED, nothing written: {e}")
            return 1
        if a.dump:
            db.execute("ROLLBACK")
            print(json.dumps(dump(p), indent=1, default=str))
            return 0
        print(f"{a.shard}:")
        print("\n".join(p.lines))
        counts = {}
        for tbl, _, _, _ in p.ops:
            counts[tbl] = counts.get(tbl, 0) + 1
        print("totals: " + (", ".join(f"insert {k} {v}" for k, v in sorted(counts.items())) or "nothing"))
        if not a.apply:
            db.execute("ROLLBACK")
            print("dry run: nothing written (--apply to write)")
            return 0
        if not p.ops:
            db.execute("ROLLBACK")
            print("nothing to write: all in place")
            return 0
        n = apply(db, p)
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
