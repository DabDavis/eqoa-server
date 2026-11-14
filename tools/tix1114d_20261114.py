#!/usr/bin/env python3
"""Three petitions' data of 2026-11-14 (batch tix5, lane tix-1114d), each checked on world 0's ground and walls files.

  2098 (Fullmetal, Heirloom): "Harbingers of Khalimet: Next part after defeating Armis didnt drop required loot and quest
       marker reset to part of canyon that cannot be reached". Step 3 spawns Armis beside the player (the tome); step 4
       wants the Wand of Shattered Souls off Armis too, and the spawned Armis is gone once looted, so step 4's mark is
       Armis's standing row, npcs 123040 at (20937, 136.2, 20722): the mesa top 2073 moved the camp off (no walk from
       the canyon floor comes nearer than ~110). npcs 123040 -> (20929.5, 6.68, 20889.4), Box Canyon's floor 52 north of
       the camp (step 2, radius 40: outside it), a walk from where Fullmetal stood (a flood over the ground and walls
       files). Its 90 s respawn is kept. Since build217's pre-looting a kill of Armis on step 3 drops the wand too (the
       test TestTwoLootStepsOffOneCreatureDropBothOnOneKill); Fullmetal's kill (07:07) was before it. Both shards.
  2099 (Zerkziee, Heirloom): "professor dugan quest sphere of annihilation spawns seem to be missing/sparse and not
       dropping quest loot". Archaeologist Duggan's Wooden Cylinder Gaskets (quest 101140: four gaskets off spheres of
       annihilation, 0.5 each kill) had four spheres (108091, 108109, 108123, 108153) 850 apart on a 30-minute respawn
       (npcrespawn 1800, "757 section3 stopgap"): Zerkziee and Zaeire's kills 07:34-07:57 dropped none of four rolls. Four
       more spheres, each a copy of 108123 (its level, body and the rest) on walkable ground round Duggan, none within 40
       of another creature: (15106.6, 19.58, 27306.5), (14700.6, 58.97, 27300.5), (14750.6, 54.19, 27100.5), (15050.6,
       52.97, 27150.5), each its npcrespawn row of 1800 as the others'; the step's chance 0.5 -> 1.0 (a rare creature,
       as Tasty Froglok Skins' 1.0). Heirloom only: Live's four spheres respawn on the default (no npcrespawn row).
  5040 (Innoruuk, Live): "Freeport house crocs need tweaked, not spawned where they should, one spawning over itself".
       Croc Island's hut is a room of placed walls round (25371, 17326), its floor 69.95 running out past them as a
       porch. Croc (122113) stands in the room; the three crocodiles stood on the porch outside its walls: 1038 east of
       the east wall, 3216 south-east of the south-east wall, and Jibrish's /place 162787 (Heirloom's mirror 168973) 1.5
       west of the west wall, its body into it. Into the room, each at least 4 from the others and Croc and 2-3 from the
       walls, on the side each stood: 162787/168973 -> (25367.5, 17326.5), 1038 -> (25376.0, 17325.0), 3216 ->
       (25372.8, 17330.2), height and facing kept (no two rows were within a unit of each other: none deleted). Both
       shards.

Refused, nothing written, when a row is not as expected (a GM's edit since). Report only by default; --apply in one
transaction, a gm_audit line a row, log tix1114d_log; a second --apply does nothing; --undo exact (a row changed since
is left and said). --only limits it to those petitions. npcs, npcrespawn and quests are read at the server's start: a
restart (a keep restart's before.sh).

    python3 -E -s tools/tix1114d_20261114.py --db eqoa.db [--apply | --undo] [--only 2098,2099,5040]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "tix1114d_log"
WHO, VIA = "dab", "tool"

ARMIS = (123040, (20937.0, 136.1622, 20722.0), (20929.5, 6.68, 20889.4))
SPHERE_COPY = 108123
SPHERES_OLD = (108091, 108109, 108123, 108153)
SPHERES_NEW = ((15106.6, 19.58, 27306.5), (14700.6, 58.97, 27300.5), (14750.6, 54.19, 27100.5), (15050.6, 52.97, 27150.5))
SPHERE_RESPAWN = 1800.0
GASKETS = 101140
# (rows, name, (x, z) before, (x, z) after); the first row that is there is moved
CROCS = (((162787, 168973), "crocodile", (25362.625, 17326.041015625), (25367.5, 17326.5)),
         ((1038,), "crocodile", (25382.4, 17326.24), (25376.0, 17325.0)),
         ((3216,), "crocodile", (25378.31, 17332.77), (25372.8, 17330.2)))

DETAILS = {
    2098: "petition 2098 (Fullmetal): Armis stands on Box Canyon's floor by the camp, not on the mesa top",
    2099: "petition 2099 (Zerkziee): four more spheres of annihilation round Archaeologist Duggan, the gasket at every kill",
    5040: "petition 5040 (Innoruuk): Croc Island's crocodiles in the hut's room, apart, not on the porch outside its walls",
}

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    ticket  INTEGER NOT NULL,
    tbl     TEXT NOT NULL,
    keycol  TEXT NOT NULL,
    keyval  TEXT NOT NULL,
    col     TEXT NOT NULL,
    before  TEXT,
    after   TEXT,
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def compact(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False)


def audit(db, ticket, tbl, keycol, keyval, col, before, after):
    action = "update" if col != ROW else ("delete" if after is None else "insert")
    if col == ROW:
        before, after = ({} if before is None else before), ({} if after is None else after)
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl,
                      compact(dict(zip(str(keycol).split(","), (int(v) for v in str(keyval).split(","))))),
                      compact(before if col == ROW else {col: before}), compact(after if col == ROW else {col: after}), DETAILS[ticket] + ": tools/tix1114d_20261114.py"))
    return cur.lastrowid


class Plan:
    def __init__(self):
        self.changes, self.lines, self.refusals = [], [], []

    def column(self, ticket, tbl, keycol, key, col, want, after, label, check=None):
        """want: the values the column may have before (a tuple); after: the new value."""
        row = db_row(self.db, tbl, keycol, key)
        if row is None:
            self.lines.append(f"{ticket}: {tbl} {key} ({label}) not on this shard: skipped")
            return
        if check:
            bad = check(row)
            if bad:
                self.refusals.append(f"{ticket}: {tbl} {key}: {bad}")
                return
        v = row[col]
        if v == after:
            self.lines.append(f"{ticket}: {tbl} {key} ({label}) {col} = {after!r} already")
            return
        if v not in want:
            self.refusals.append(f"{ticket}: {tbl} {key} ({label}) {col} = {v!r}, want one of {want!r}")
            return
        self.changes.append((ticket, tbl, keycol, key, col, v, after))
        self.lines.append(f"{ticket}: {tbl} {key} ({label}) {col} {v!r} -> {after!r}")

    def column_where(self, ticket, tbl, keycols, keys, col, want, after, label, check=None):
        """column on a row of a two-column key: logged with keycol "a,b" and keyval "1,2"."""
        self.db.row_factory = sqlite3.Row
        try:
            row = self.db.execute(f"SELECT * FROM {tbl} WHERE {keycols[0]} = ? AND {keycols[1]} = ?", keys).fetchone()
        finally:
            self.db.row_factory = None
        if row is None:
            self.lines.append(f"{ticket}: {tbl} {keys} ({label}) not on this shard: skipped")
            return
        if check and check(row):
            self.refusals.append(f"{ticket}: {tbl} {keys}: {check(row)}")
            return
        v = row[col]
        if v == after:
            self.lines.append(f"{ticket}: {tbl} {keys} ({label}) {col} = {after!r} already")
            return
        if v not in want:
            self.refusals.append(f"{ticket}: {tbl} {keys} ({label}) {col} = {v!r}, want one of {want!r}")
            return
        self.changes.append((ticket, tbl, ",".join(keycols), ",".join(str(k) for k in keys), col, v, after))
        self.lines.append(f"{ticket}: {tbl} {keys} ({label}) {col} {v!r} -> {after!r}")

    def delete(self, ticket, tbl, keycol, key, label, check):
        row = rowdict(self.db, f"SELECT * FROM {tbl} WHERE {keycol} = ?", (key,))
        if row is None:
            self.lines.append(f"{ticket}: {tbl} {key} ({label}) not there: nothing to delete")
            return
        bad = check(row)
        if bad:
            self.refusals.append(f"{ticket}: {tbl} {key}: {bad}")
            return
        self.changes.append((ticket, tbl, keycol, key, ROW, row, None))
        self.lines.append(f"{ticket}: {tbl} {key} ({label}) deleted")

    def stack(self, ticket, spellid, name, want, new):
        r = self.db.execute("SELECT p.spellname, e.effects FROM spellpattern p JOIN spelleffect e USING(spellid) "
                            "WHERE p.spellid = ?", (spellid,)).fetchone()
        if r is None or r[0] != name:
            self.refusals.append(f"{ticket}: spell {spellid}: {r[0] if r else 'missing'!r}, want {name}")
            return
        text = r[1]
        try:
            eff = json.loads(text)
        except ValueError:
            self.refusals.append(f"{ticket}: spelleffect {spellid} ({name}): effects are not JSON")
            return
        if compact(eff) != text:
            self.refusals.append(f"{ticket}: spelleffect {spellid} ({name}): effects not compact (would not undo exactly)")
            return
        s = eff.get("stack")
        if s == new:
            self.lines.append(f"{ticket}: spelleffect {spellid} ({name}) stack {new} already")
            return
        if s != want:
            self.refusals.append(f"{ticket}: spelleffect {spellid} ({name}): stack {s!r}, want {want}")
            return
        eff["stack"] = new
        self.changes.append((ticket, "spelleffect", "spellid", spellid, "effects", text, compact(eff)))
        self.lines.append(f"{ticket}: spelleffect {spellid} ({name}) stack {want} -> {new}")


def rowdict(db, sql, args):
    cur = db.execute(sql, args)
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def db_row(db, tbl, keycol, key):
    db.row_factory = sqlite3.Row
    try:
        return db.execute(f"SELECT * FROM {tbl} WHERE {keycol} = ?", (key,)).fetchone()
    finally:
        db.row_factory = None


def near(a, b, eps=0.01):
    return abs(a - b) <= eps


def plan(db, only):
    p = Plan()
    p.db = db
    if 2098 in only:
        rid, was, to = ARMIS
        chk = lambda r: None if r["npc_name"] == "Armis" and r["world"] == 0 else f"{r['npc_name']!r} world {r['world']}"
        for col, old, new in zip(("x_coord", "y_coord", "z_coord"), was, to):
            p.column(2098, "npcs", "id", rid, col, (old,), new, "Armis", chk)
    if 2099 in only:
        rows = [rowdict(db, "SELECT n.id, n.npc_name, r.seconds FROM npcs n LEFT JOIN npcrespawn r ON r.npc_row = n.id "
                            "WHERE n.id = ?", (i,)) for i in SPHERES_OLD]
        if any(r is None or r["npc_name"].lower() != "sphere of annihilation" for r in rows):
            p.refusals.append(f"2099: the four spheres {SPHERES_OLD} are not all there: {rows}")
        elif any(r["seconds"] is None for r in rows):
            p.lines.append("2099: the spheres respawn on the default here (no npcrespawn rows: the Live shard): skipped")
        else:
            src = rowdict(db, "SELECT * FROM npcs WHERE id = ?", (SPHERE_COPY,))
            have = db.execute("SELECT COUNT(*) FROM npcs WHERE lower(npc_name) = 'sphere of annihilation'").fetchone()[0]
            placed = [xyz for xyz in SPHERES_NEW if db.execute(
                "SELECT 1 FROM npcs WHERE lower(npc_name) = 'sphere of annihilation' AND abs(x_coord - ?) < 0.5 "
                "AND abs(z_coord - ?) < 0.5", (xyz[0], xyz[2])).fetchone()]
            if len(placed) == len(SPHERES_NEW):
                p.lines.append(f"2099: the four new spheres are there already ({have} in all)")
            elif placed:
                p.refusals.append(f"2099: {len(placed)} of the four new spheres are there already: {placed}")
            else:
                nid = db.execute("SELECT MAX(id) FROM npcs").fetchone()[0] + 1
                for x, y, z in SPHERES_NEW:
                    row = dict(src, id=nid, npc_id=nid, x_coord=x, y_coord=y, z_coord=z, dt_kill=None)
                    p.changes.append((2099, "npcs", "id", nid, ROW, None, row))
                    p.changes.append((2099, "npcrespawn", "npc_row", nid, ROW, None,
                                      {"npc_row": nid, "seconds": SPHERE_RESPAWN,
                                       "note": "petition 2099: a sphere of annihilation added, the others' 30 minutes",
                                       "source": "tix1114d"}))
                    p.lines.append(f"2099: npcs {nid} a sphere of annihilation (a copy of {SPHERE_COPY}) at ({x}, {y}, {z}), "
                                   f"respawn {SPHERE_RESPAWN:.0f} s")
                    nid += 1
        if not p.refusals and not any("skipped" in l for l in p.lines if l.startswith("2099")):
            p.column_where(2099, "queststeps", ("questid", "step"), (GASKETS, 1), "chance", (0.5,), 1.0,
                           "Wooden Cylinder Gaskets step 1",
                           check=lambda r: None if r["kind"] == "loot" and r["target"].lower() == "sphere of annihilation"
                           else f"kind {r['kind']!r} target {r['target']!r}")
    if 5040 in only:
        for ids, name, was, to in CROCS:
            rid = next((i for i in ids if db_row(db, "npcs", "id", i) is not None), None)
            if rid is None:
                p.refusals.append(f"5040: none of npcs {ids} ({name}) is here")
                continue
            def chk(r, name=name):
                if r["npc_name"] != name or r["world"] != 0:
                    return f"{r['npc_name']!r} world {r['world']}, want {name!r} in world 0"
                return None
            for col, old, new in (("x_coord", was[0], to[0]), ("z_coord", was[1], to[1])):
                p.column(5040, "npcs", "id", rid, col, (old,), new, f"Croc Island's {name}", chk)
    return p



ROW = "*row*"  # a change of a whole row: before None an insert, after None a delete (the row as a dict)


def write(db, tbl, keycol, keyval, col, before, after):
    cols, vals = str(keycol).split(","), [int(v) for v in str(keyval).split(",")]
    where = " AND ".join(f"{c} = ?" for c in cols)
    if col == ROW:
        now = rowdict(db, f"SELECT * FROM {tbl} WHERE {where}", vals)
        if after is None:  # delete: the row must be as it was read
            if now != before:
                return False
            return db.execute(f"DELETE FROM {tbl} WHERE {where}", vals).rowcount == 1
        if now is not None:  # insert: nothing there
            return False
        names = list(after)
        db.execute(f"INSERT INTO {tbl} ({', '.join(names)}) VALUES ({', '.join('?' * len(names))})", [after[n] for n in names])
        return True
    cur = db.execute(f"UPDATE {tbl} SET {col} = ? WHERE {where} AND {col} IS ?", (after, *vals, before))
    return cur.rowcount == 1


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for _, ticket, tbl, keycol, keyval, col, before, after, _a in db.execute(f"SELECT * FROM {LOG} ORDER BY seq DESC").fetchall():
        before, after = json.loads(before), json.loads(after)
        if not write(db, tbl, keycol, keyval, col, after, before):
            print(f"left: {ticket} {tbl} {keyval} {col} changed since")
            left += 1
            continue
        audit(db, ticket, tbl, keycol, keyval, col, after, before)
        back += 1
    if not left:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    ap.add_argument("--only", default=",".join(str(t) for t in DETAILS))
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    only = {int(t) for t in a.only.split(",") if t.strip()}
    if not only <= set(DETAILS):
        sys.exit(f"--only: unknown petitions {sorted(only - set(DETAILS))}")
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        if a.undo:
            db.execute("BEGIN IMMEDIATE")
            try:
                back, left = undo(db)
                db.execute("COMMIT")
            except BaseException:
                db.execute("ROLLBACK")
                raise
            print(f"put back {back} rows, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        if table(db, LOG):
            print(f"applied already ({LOG} is there): --undo first")
            return 0
        p = plan(db, only)
        for line in p.lines:
            print(line)
        if p.refusals:
            print("refused, nothing done:")
            for r in p.refusals:
                print("  " + r)
            return 1
        if not p.changes:
            print("nothing to do")
            return 0
        if not a.apply:
            print(f"report only: {len(p.changes)} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            db.execute(LOG_SCHEMA)
            for ticket, tbl, keycol, keyval, col, before, after in p.changes:
                if not write(db, tbl, keycol, keyval, col, before, after):
                    raise RuntimeError(f"{tbl} {keyval} changed under the update")
                a_id = audit(db, ticket, tbl, keycol, keyval, col, before, after)
                db.execute(f"INSERT INTO {LOG} (ticket, tbl, keycol, keyval, col, before, after, audit) "
                           "VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
                           (ticket, tbl, keycol, str(keyval), col, json.dumps(before), json.dumps(after), a_id))
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {len(p.changes)} rows, log {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
