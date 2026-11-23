#!/usr/bin/env python3
"""Heirloom petition 2812 of 2026-11-23 (the thirteenth batch of the day), as data: the mead wasp stinger's icon.

Every row is found by its name (and what else names it), never by an id alone; a row already as this writes it is left
("already"); anything else refuses the whole run.

  2812  "Loot: mead wasp stinger. It has a poison sac icon in inventory and should probably have the typical stinger
        one." The record (991016, Heirloom only: Live has none) wore ITEMICON 0xB8E7D93C (-1192765124), the purple
        poison sac that the poison sacs wear. The stinger icon is the curved spike Queen Qy'tha's stinger (701441)
        wears, 0xC5E348B1 (-974960463, copied from that record, never worked out by hand); both rendered from
        ITEMICON.ESF with ~/.cache/iconsheet before choosing.

  --step 2 (the same claim, at the coordinator's word: a stinger looks like a stinger): the five other stingers
        wearing the sac, brush wasp stinger, Redsting's stinger, Zapper Stinger, Defiler Prince's Stinger and Poison
        Stinger, get the same icon, each found by its name (one record a name; a name the shard lacks is "absent":
        Live has only Zapper, Defiler Prince's and its own Poison Stinger). Its own log (petpass_heirloom_1123m_s2_log)
        and its own --undo (--step 2 --undo); it runs on Live as well.

  2811 (the elder orc sapper at Freeport's bank) and 2810 (a client stop in Highpass) need no data: the sapper was a
  player's charmed pet let go at the bank when its master's client went silent (fixed in code, entity/charm.go).

Every row written is a gm_audit line (via "tool") and a row of the log; --undo puts each back as it was (a row
changed since is left and said) and drops the log. A report by default; --apply writes once (a second finds the log
and writes nothing); --apply and --undo copy the database first (<db>.bak-<when>-pre-pet1123m) unless --no-backup.

WHEN IT TAKES EFFECT: item records are sent at login: the player's next login.

    python3 -E -s tools/petpass_heirloom_1123m.py --db eqoa.db            report
    python3 -E -s tools/petpass_heirloom_1123m.py --db eqoa.db --apply    write it
    python3 -E -s tools/petpass_heirloom_1123m.py --db eqoa.db --undo     take it back
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "petpass_heirloom_1123m_log"
WHO, VIA = "claude", "tool"
DETAIL = "Heirloom petition 2812 (2026-11-23): the mead wasp stinger wears the stinger icon: tools/petpass_heirloom_1123m.py"

TARGET = ("mead wasp stinger", 991016)
STEP2 = ["brush wasp stinger", "Redsting's stinger", "Zapper Stinger", "Defiler Prince's Stinger", "Poison Stinger"]
LOG2 = "petpass_heirloom_1123m_s2_log"
DETAIL2 = ("Heirloom petition 2812, step 2 (2026-11-23): the other stingers on the poison sac's icon wear the stinger "
           "icon: tools/petpass_heirloom_1123m.py --step 2")
SAC = -1192765124  # the poison sac's icon, as the record wears it now
SOURCE = ("Queen Qy'tha's stinger", 701441)  # the stinger icon's record: its itemicon is copied

KEYS = {"itempattern": ("patternid",)}
LOG_DDL = """CREATE TABLE IF NOT EXISTS {log} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    ref    TEXT NOT NULL,
    before TEXT,
    after  TEXT,
    audit  INTEGER,
    at     TEXT NOT NULL
)"""
AUDIT_DDL = """CREATE TABLE IF NOT EXISTS gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER,
    username TEXT, via TEXT, ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT)"""


STEP = [1]  # the step this run writes (--step), for the audit lines' detail


class Refused(Exception):
    pass


def go_json(m):
    """A row as Go's json.Marshal writes a map (keys sorted, <>& escaped), as the panel's audit lines are."""
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def rows(db, sql, args=()):
    cur = db.execute(sql, args)
    names = [c[0] for c in cur.description]
    return [dict(zip(names, r)) for r in cur.fetchall()]


def one(db, what, sql, args=()):
    got = rows(db, sql, args)
    if len(got) != 1:
        raise Refused(f"{what}: {len(got)} rows, want one")
    return got[0]


def has_table(db, name):
    return bool(db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone())


class Plan:
    def __init__(self, db, step=1):
        self.db = db
        self.step = step
        self.changes = []  # (tbl, key dict, before row, values)
        self.lines = []

    def build(self):
        db = self.db
        src = one(db, "the stinger icon's record", "SELECT patternid, itemicon FROM itempattern WHERE itemname = ?",
                  (SOURCE[0],))
        if src["patternid"] != SOURCE[1]:
            raise Refused(f"{SOURCE[0]} is record {src['patternid']}, not {SOURCE[1]}")
        icon = src["itemicon"]
        if icon == SAC:
            raise Refused(f"{SOURCE[0]} wears the poison sac's icon")
        if self.step == 2:
            for name in STEP2:
                got = rows(db, "SELECT * FROM itempattern WHERE itemname = ?", (name,))
                what = f"2812 step 2 itempattern {name}"
                if not got:
                    self.lines.append(f"absent   {what}")
                    continue
                if len(got) != 1:
                    raise Refused(f"{what}: {len(got)} records")
                before = got[0]
                if before["itemicon"] == icon:
                    self.lines.append(f"already  {what} ({before['patternid']}): itemicon {icon}")
                    continue
                if before["itemicon"] != SAC:
                    raise Refused(f"{what}: itemicon {before['itemicon']}, not the poison sac's {SAC}")
                self.changes.append(("itempattern", {"patternid": before["patternid"]}, before, {"itemicon": icon}))
                self.lines.append(f"update   {what} ({before['patternid']}): itemicon {SAC} (poison sac) -> {icon}")
            return
        what = f"2812 itempattern {TARGET[1]} {TARGET[0]}"
        before = one(db, what, "SELECT * FROM itempattern WHERE itemname = ?", (TARGET[0],))
        if before["patternid"] != TARGET[1]:
            raise Refused(f"{what}: the record is {before['patternid']}")
        if before["itemicon"] == icon:
            self.lines.append(f"already  {what}: itemicon {icon}")
            return
        if before["itemicon"] != SAC:
            raise Refused(f"{what}: itemicon {before['itemicon']}, not the poison sac's {SAC}")
        self.changes.append(("itempattern", {"patternid": TARGET[1]}, before, {"itemicon": icon}))
        self.lines.append(f"update   {what}: itemicon {SAC} (poison sac) -> {icon} ({SOURCE[0]}'s)")


def audit(db, action, tbl, key, before, after):
    detail = DETAIL2 if STEP[0] == 2 else DETAIL
    db.execute(AUDIT_DDL)
    cur = db.execute("""INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after,
        detail) VALUES (?, 0, ?, ?, '', ?, ?, ?, ?, ?, ?)""",
                     (now(), WHO, VIA, action, tbl, go_json(key), go_json(before) if before else None,
                      go_json(after) if after else None, detail))
    return cur.lastrowid


def where(key):
    return " AND ".join(f"{k} = ?" for k in key), list(key.values())


def keyof(tbl, row):
    return {k: row[k] for k in KEYS[tbl]}


def apply(db, plan, log):
    db.execute(LOG_DDL.format(log=log))
    for tbl, key, before, values in plan.changes:
        if values is None:  # a delete
            w, args = where(key)
            db.execute(f"DELETE FROM {tbl} WHERE {w}", args)
            a, after = audit(db, "delete", tbl, key, before, None), None
        elif before is None:
            cols = list(values)
            cur = db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                             [values[c] for c in cols])
            key = keyof(tbl, values) if all(k in values for k in KEYS[tbl]) else {KEYS[tbl][0]: cur.lastrowid}
            after = rows(db, f"SELECT * FROM {tbl} WHERE {where(key)[0]}", where(key)[1])[0]
            a = audit(db, "insert", tbl, key, None, after)
        else:
            w, args = where(key)
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in values)} WHERE {w}",
                       list(values.values()) + args)
            after = rows(db, f"SELECT * FROM {tbl} WHERE {w}", args)[0]
            a = audit(db, "update", tbl, key, before, after)
        db.execute(f"INSERT INTO {log} (tbl, ref, before, after, audit, at) VALUES (?, ?, ?, ?, ?, ?)",
                   (tbl, json.dumps(key), json.dumps(before) if before else None,
                    json.dumps(after) if after else None, a, now()))


def undo(db, log):
    out = []
    for r in rows(db, f"SELECT * FROM {log} ORDER BY seq DESC"):
        tbl, key = r["tbl"], json.loads(r["ref"])
        w, args = where(key)
        now_rows = rows(db, f"SELECT * FROM {tbl} WHERE {w}", args)
        if r["after"] is None:  # a delete: the row goes back unless one holds its key now
            if now_rows:
                out.append(f"left     {tbl} {key}: a row holds its key since")
                continue
            before = json.loads(r["before"])
            cols = list(before)
            db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                       [before[c] for c in cols])
            audit(db, "insert", tbl, key, None, before)
            out.append(f"restored {tbl} {key}")
            continue
        after = json.loads(r["after"])
        if len(now_rows) != 1 or now_rows[0] != after:
            out.append(f"left     {tbl} {key}: changed since")
            continue
        if r["before"] is None:
            db.execute(f"DELETE FROM {tbl} WHERE {w}", args)
            audit(db, "delete", tbl, key, after, None)
        else:
            before = json.loads(r["before"])
            cols = list(before)
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in cols)} WHERE {w}",
                       [before[c] for c in cols] + args)
            audit(db, "update", tbl, keyof(tbl, before), after, before)
        out.append(f"restored {tbl} {key}")
    db.execute(f"DROP TABLE {log}")
    return out


def backup(path):
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-pet1123m{'-s2' if STEP[0] == 2 else ''}"
    src = sqlite3.connect(path)
    out = sqlite3.connect(dst)
    src.backup(out)
    out.close()
    src.close()
    return dst


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--undo", action="store_true")
    ap.add_argument("--no-backup", action="store_true")
    ap.add_argument("--step", type=int, choices=(1, 2), default=1)
    a = ap.parse_args(argv)
    STEP[0] = a.step
    log = LOG2 if a.step == 2 else LOG
    db = sqlite3.connect(a.db, timeout=5, isolation_level=None)
    db.execute("BEGIN IMMEDIATE")
    try:
        logged = has_table(db, log)
        if a.undo:
            if not logged:
                print("nothing to undo: no log")
                db.execute("ROLLBACK")
                return 0
            if not a.no_backup:
                print("backup", backup(a.db))
            for line in undo(db, log):
                print(line)
            db.execute("COMMIT")
            return 0
        if logged and a.apply:
            print(f"applied already ({log} exists): nothing written")
            db.execute("ROLLBACK")
            return 0
        plan = Plan(db, a.step)
        plan.build()
        for line in plan.lines:
            print(line)
        if not a.apply:
            print(f"dry run: {len(plan.changes)} rows would change")
            db.execute("ROLLBACK")
            return 0
        if not a.no_backup:
            print("backup", backup(a.db))
        apply(db, plan, log)
        db.execute("COMMIT")
        print(f"applied: {len(plan.changes)} rows")
        return 0
    except Refused as e:
        db.execute("ROLLBACK")
        print("refused:", e, file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
