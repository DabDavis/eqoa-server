#!/usr/bin/env python3
"""Heirloom petition 2807 of 2026-11-23 (the tenth batch of the day), as data: the Hunt & Gather givers' "!".

Every row is found by its name (and what else names it), never by an id alone; a row already as this writes it is left
("already"); anything else refuses the whole run.

  2807  "Fist of Marr recruiter doesnt have '!' over his head" (a level 17 Human Warrior beside him at Marr's Fist,
        who took his Blackened Earrings, levels 13-18, 85 s before filing). The quest marks (handlers/questheadmarks.go)
        mark, under quests.markers "flagged" (this shard's), only the quests of table questmarks: each race's and
        class's first quest (petition 1813) and the city level 17 chain heads (petition 2773). No Hunt & Gather quest
        was flagged, so none of their givers ever showed a "!", though the player qualified (no class, race, quest or
        standing gate; the level 13-18 he is in). Every enabled Hunt & Gather quest (source "EQOA-Data
        HuntAndGather/..." or "teamjosh H&G ...", all repeatable, none requiring another) is flagged by the same
        rule: 82 quests of 43 givers, the Fist of Marr Recruiter's five among them. A repeatable quest's "!" stands
        while the player is in its levels and does not hold it (mayTakeLocked), as the turn-in quests' do.

Every row written is a gm_audit line (via "tool") and a row of the log; --undo puts each back as it was (a row
changed since is left and said) and drops the log. A report by default; --apply writes once (a second finds the log
and writes nothing); --apply and --undo copy the database first (<db>.bak-<when>-pre-pet1123j) unless --no-backup.

WHEN IT TAKES EFFECT: questmarks is read at the server's start (and at a GM panel quest edit): the next restart.

    python3 -E -s tools/petpass_heirloom_1123j.py --db eqoa.db            report
    python3 -E -s tools/petpass_heirloom_1123j.py --db eqoa.db --apply    write it
    python3 -E -s tools/petpass_heirloom_1123j.py --db eqoa.db --undo     take it back
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "petpass_heirloom_1123j_log"
WHO, VIA = "claude", "tool"
DETAIL = "Heirloom petition 2807 (2026-11-23): the Hunt & Gather quests flagged for the quest marks: tools/petpass_heirloom_1123j.py"

# 2807
SOURCES = ("EQOA-Data HuntAndGather/", "teamjosh H&G")
MARR, MARR_QUEST = "Fist of Marr Recruiter", "Blackened Earrings"
WANT_AT_LEAST = 82  # Heirloom's count when this was made
MARK_NOTE = "petition 2807: a Hunt & Gather quest (%s's)"

KEYS = {"questmarks": ("questid",)}
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
    def __init__(self, db):
        self.db = db
        self.changes = []  # (tbl, key dict or None, before row or None, values or None: a delete)
        self.lines = []

    def insert(self, tbl, values, what, present):
        """a new row, unless present (a row already there as wanted: left; another: refused)."""
        if present is not None:
            if all(present[c] == v for c, v in values.items()):
                self.lines.append(f"already  {what}")
                return
            if present.get("questid") == values.get("questid"):
                self.lines.append(f"already  {what} (flagged before: {present['note']!r})")
                return
            raise Refused(f"{what}: a row is there already: {present}")
        self.changes.append((tbl, None, None, dict(values)))
        self.lines.append(f"insert   {what}")

    def build(self):
        db = self.db
        one(db, f"{MARR}'s {MARR_QUEST}", """SELECT questid FROM quests WHERE giver = ? AND name = ? AND enabled = 1
            AND repeatable = 1""", (MARR, MARR_QUEST))
        hg = rows(db, """SELECT questid, name, giver, minlevel, maxlevel, repeatable, requires, source FROM quests
            WHERE enabled = 1 AND (source LIKE ? OR source LIKE ?) ORDER BY giver, minlevel, questid""",
                  tuple(s + "%" for s in SOURCES))
        if len(hg) < WANT_AT_LEAST:
            raise Refused(f"{len(hg)} enabled Hunt & Gather quests, want {WANT_AT_LEAST} or more")
        for q in hg:
            if not q["repeatable"] or str(q["requires"] or "").strip():
                raise Refused(f"{q['questid']} {q['name']}: not a plain repeatable Hunt & Gather quest: {q}")
            have = rows(db, "SELECT * FROM questmarks WHERE questid = ?", (q["questid"],))
            self.insert("questmarks", {"questid": q["questid"], "note": MARK_NOTE % q["giver"]},
                        f"2807 questmarks {q['questid']} {q['name']} ({q['giver']}, {q['minlevel']}-{q['maxlevel']})",
                        have[0] if have else None)


def audit(db, action, tbl, key, before, after):
    db.execute(AUDIT_DDL)
    cur = db.execute("""INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after,
        detail) VALUES (?, 0, ?, ?, '', ?, ?, ?, ?, ?, ?)""",
                     (now(), WHO, VIA, action, tbl, go_json(key), go_json(before) if before else None,
                      go_json(after) if after else None, DETAIL))
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
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-pet1123j"
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
    a = ap.parse_args(argv)
    log = LOG
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
        plan = Plan(db)
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
