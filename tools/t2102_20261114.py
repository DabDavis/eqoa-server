#!/usr/bin/env python3
"""Petition 2102 (Kyros, Heirloom, 2026-11-14; batch tix5, lane t2102): "Pickclaws outside HP attack immediately when you
get within range making it impossible to leave HP via road without invis." Both shards.

Highpass Hold's east way out runs from the hold's east end among its guards (y 196, Captain Maehler's post), down a
ramp to the lower gate (Guards Jergensen, Holden, Pickins, (17170, 95, 15310)) and out along the canyon, a side arm
of it running north. Along all of it stand the Pickclaw tiers of every level: the 1-30 ones and, among them, the 41-60
tiers' (pickclaw soldier and elder 48, adept pickclaw soldier and elder 58): rows 361, 3008, 3009 among the hold's
guards, 360 and 362 on the ramp, 2080 15 from the lower gate's guards on the canyon floor, and the soldiers 2088 and
190076-190078 (the last three tools/faq_quest_fixes_20261114.py's, for quest 101185's sashes) in the north arm by the
gate. Each sees a player 21 off (entity/aggro.go DefaultAggroRadius) and attacks below three times its level, so a
level 24 bard (Kyros: a level 48 elder from 19 at 12:17:39) cannot pass them on a way 30-40 wide; petition 1580
(Noxx) said the same of the 58s ("stand on the East path of Highpass all the time").

  - Those four tiers' sight -> 10.5 (npcaggro by name, as the crabs' of petition 1129): they still attack whoever
    comes to them, a player keeping to the other side of the way passes. Their camps, levels and the quests that
    hunt them are kept.
  - npcs 2080 (pickclaw elder 48), standing in the lower gate's mouth 15 from its guards, -> (17200.0, 15245.0) in
    the north arm beside soldier 2088, its height (94.9, the canyon floor) and facing kept.

Refused, nothing written, when a row is not as expected (a GM's edit since). Report only by default; --apply in one
transaction, a gm_audit line a row, log t2102_log; a second --apply does nothing; --undo exact. npcs and npcaggro are
read at the server's start: a restart (a keep restart's before.sh).

    python3 -E -s tools/t2102_20261114.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "t2102_log"
WHO, VIA = "dab", "tool"

NAMES = ("pickclaw soldier", "pickclaw elder", "adept pickclaw soldier", "adept pickclaw elder")
RADIUS = 10.5
ELDER = (2080, (17155.4, 15320.5), (17200.0, 15245.0))

DETAILS = {
    2102: "petition 2102 (Kyros): Highpass's level 48-58 pickclaws see 10.5, not 21, so a player can leave by the east way",
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


def keyof(v):
    """a key column's value: a number as an int, a name as itself"""
    try:
        return int(v)
    except ValueError:
        return v


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
                      compact(dict(zip(str(keycol).split(","), (keyof(v) for v in str(keyval).split(","))))),
                      compact(before if col == ROW else {col: before}), compact(after if col == ROW else {col: after}), DETAILS[ticket] + ": tools/t2102_20261114.py"))
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


def plan(db, only):
    p = Plan()
    p.db = db
    if 2102 in only:
        for n in NAMES:
            r = rowdict(db, "SELECT * FROM npcaggro WHERE npc_name = ?", (n,))
            if r is None:
                p.changes.append((2102, "npcaggro", "npc_name", n, ROW, None,
                                  {"npc_name": n, "aggressive": 1, "radius": RADIUS, "note": "petition 2102 (Kyros): half the sight, "
                                   "so a player can leave Highpass by its east way", "source": "t2102", "leash": 0.0,
                                   "holding": 0, "see_invis": 0}))
                p.lines.append(f"2102: npcaggro {n!r}: aggressive, sight {RADIUS}")
            elif r["radius"] == RADIUS:
                p.lines.append(f"2102: npcaggro {n!r} sight {RADIUS} already")
            else:
                p.refusals.append(f"2102: npcaggro {n!r} is there already: {r}")
        rid, was, to = ELDER
        chk = lambda r: None if r["npc_name"] == "pickclaw elder" and r["world"] == 0 else f"{r['npc_name']!r} world {r['world']}"
        for col, old, new in (("x_coord", was[0], to[0]), ("z_coord", was[1], to[1])):
            p.column(2102, "npcs", "id", rid, col, (old,), new, "pickclaw elder at the lower gate", chk)
    return p



ROW = "*row*"  # a change of a whole row: before None an insert, after None a delete (the row as a dict)


def write(db, tbl, keycol, keyval, col, before, after):
    cols, vals = str(keycol).split(","), [keyof(v) for v in str(keyval).split(",")]
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
