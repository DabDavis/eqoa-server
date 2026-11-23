#!/usr/bin/env python3
"""Live petitions 5547 and 5558 of 2026-11-23, as data, for the Live shard.

  5547  the Eye of the Sphynx (701463, the Endless Tower's gem to the Plane of Sky) could not be used from the pack: its
        record was CLIENT's plain item type (itempattern.procanim 0), which the item menu offers no Use for. It takes
        the gem type 7, as Magi'Kot's summoning gem and the Ring of Speed (VIInfoText::IsUsable; Prepare puts it on the
        gem quick slot). Its Use is already the gate to the Sky Portal and it is never spent (tools/sky_eye_20261123.py).
  5558  the elf starting quests' words: four of Itheria Silverwind's quests for the elf warriors and shadow
        knights of Fayspires (103522/103525 "Bound to Fayspire", 103523/103526 "Slivers of Bat Meat", petition 2540's
        offrace quests) have on Heirloom the starting city's longer lines (the giver's and the steps' NPCs' words),
        which the shard sync of 2026-11-22 left off Live. Their steps are the same on both shards (kind, target, items
        and rewards each), so they take Heirloom's words: the name, offer, answers and started line, and each step's
        journal, answer, says, waiting, notyet and reminder. The other Fayspires quests came over with the Live mirrors
        of 2026-11-23 (work board 2061); the duplicated offrace quests at Priest Elgares, Priest Lighthand and Professor
        Skysign (103528/109819 and the like, on both shards) are left.

The rows' Heirloom values are tools/petpass_live_1123d_data.json (read off Heirloom on 2026-11-23, Live's values then
beside them). Every row must read as Live's did then, or as this writes it ("already"); anything else refuses the
whole run. Every row written is a gm_audit line (via "tool") and a row of petpass_live_1123d_log; --undo puts each
back as it was (an inserted row taken out, a deleted row put back), and drops the log. A report by default; --apply
writes once (a second finds the log and writes nothing).

WHEN IT TAKES EFFECT: the Eye at each player's next login (item records go at a login); the quests at the server's
next start.

    python3 -E -s tools/petpass_live_1123d.py --db eqoa.db            report
    python3 -E -s tools/petpass_live_1123d.py --db eqoa.db --apply    write it
    python3 -E -s tools/petpass_live_1123d.py --db eqoa.db --undo     take it back
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

LOG = "petpass_live_1123d_log"
WHO, VIA = "claude", "tool"
DETAIL = "Live petitions 5547/5558 (2026-11-23): tools/petpass_live_1123d.py"
DATA = os.path.join(os.path.dirname(os.path.abspath(__file__)), "petpass_live_1123d_data.json")

EYE, EYE_NAME = 701463, "Eye of the Sphynx"
GEM_TYPE, PLAIN_TYPE = 7, 0

KEYS = {"itempattern": ("patternid",), "quests": ("questid",), "queststeps": ("questid", "step"),
        "queststeptargets": ("questid", "step", "target")}
LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
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


def where(key):
    return " AND ".join(f"{k} = ?" for k in key), list(key.values())


def has_table(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone() is not None


def same(a, b):
    if isinstance(a, float) or isinstance(b, float):
        try:
            return abs(float(a) - float(b)) < 1e-4
        except (TypeError, ValueError):
            return False
    return a == b


class Plan:
    def __init__(self, db, data):
        self.db = db
        self.data = data
        self.changes = []  # (op, tbl, key, before row, values)
        self.lines = []

    def get(self, tbl, key):
        w, args = where(key)
        got = rows(self.db, f"SELECT * FROM {tbl} WHERE {w}", args)
        if len(got) > 1:
            raise Refused(f"{tbl} {key}: {len(got)} rows")
        return got[0] if got else None

    def set(self, tbl, key, old, new, what):
        """new over the row of key, when it reads old; already as new: left."""
        row = self.get(tbl, key)
        if row is None:
            raise Refused(f"{what}: no {tbl} row {key}")
        if all(same(row[c], v) for c, v in new.items()):
            self.lines.append(f"already  {what}")
            return
        for c, v in old.items():
            if not same(row[c], v):
                raise Refused(f"{what}: {c} reads {row[c]!r}, not {v!r} as read when this was made")
        vals = {c: v for c, v in new.items() if not same(row[c], v)}
        self.changes.append(("update", tbl, key, row, vals))
        self.lines.append(f"change   {what}: " + ", ".join(vals))

    def insert(self, tbl, key, values, what):
        row = self.get(tbl, key)
        if row is not None:
            if all(same(row[c], v) for c, v in values.items()):
                self.lines.append(f"already  {what}")
                return
            raise Refused(f"{what}: {tbl} already has {row}")
        self.changes.append(("insert", tbl, key, None, {**key, **values}))
        self.lines.append(f"insert   {what}")

    def delete(self, tbl, key, old, what):
        row = self.get(tbl, key)
        if row is None:
            self.lines.append(f"already  {what} (gone)")
            return
        for c, v in old.items():
            if not same(row[c], v):
                raise Refused(f"{what}: {c} reads {row[c]!r}, not {v!r}")
        self.changes.append(("delete", tbl, key, row, None))
        self.lines.append(f"delete   {what}")

    def build(self):
        # 5547: the Eye's item type
        self.set("itempattern", {"patternid": EYE}, {"procanim": PLAIN_TYPE, "itemname": EYE_NAME, "maxstack": 1},
                 {"procanim": GEM_TYPE}, f"5547 {EYE_NAME} ({EYE}): CLIENT's gem type, usable from the pack")
        # 5558: the quests' words
        for q in self.data["quests"]:
            qid = q["questid"]
            tag = "5558"
            if q["whole"]:
                raise Refused(f"quest {qid}: this tool writes no whole quest")
            self.set("quests", {"questid": qid}, q["quest_old"], q["quest_new"],
                     f"{tag} quest {qid} {q['quest_new']['name']!r}: its words")
            for st in q["steps"]:
                key = {"questid": qid, "step": st["step"]}
                what = f"{tag} quest {qid} step {st['step']}"
                if st["old"] is None:
                    self.insert("queststeps", key, st["new"], what + " (new)")
                elif st["new"] is None:
                    raise Refused(f"{what}: Heirloom has no such step (not written by this tool)")
                else:
                    self.set("queststeps", key, st["old"], st["new"], what)
            old = {(t["step"], t["target"]) for t in q["targets_old"]}
            new = {(t["step"], t["target"]) for t in q["targets_new"]}
            for s, t in sorted(old - new):
                self.delete("queststeptargets", {"questid": qid, "step": s, "target": t}, {},
                            f"{tag} quest {qid} step {s} no longer counts {t!r}")
            for s, t in sorted(new - old):
                self.insert("queststeptargets", {"questid": qid, "step": s, "target": t}, {},
                            f"{tag} quest {qid} step {s} counts {t!r}")


def audit(db, action, tbl, key, before, after):
    db.execute(AUDIT_DDL)
    cur = db.execute("""INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after,
        detail) VALUES (?, 0, ?, ?, '', ?, ?, ?, ?, ?, ?)""",
                     (now(), WHO, VIA, action, tbl, go_json(key), go_json(before) if before else None,
                      go_json(after) if after else None, DETAIL))
    return cur.lastrowid


def apply(db, plan):
    db.execute(LOG_DDL)
    for op, tbl, key, before, values in plan.changes:
        w, args = where(key)
        if op == "insert":
            cols = list(values)
            db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                       [values[c] for c in cols])
            after = rows(db, f"SELECT * FROM {tbl} WHERE {w}", args)[0]
        elif op == "update":
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in values)} WHERE {w}",
                       list(values.values()) + args)
            after = rows(db, f"SELECT * FROM {tbl} WHERE {w}", args)[0]
        else:
            db.execute(f"DELETE FROM {tbl} WHERE {w}", args)
            after = None
        a = audit(db, op, tbl, key, before, after)
        db.execute(f"INSERT INTO {LOG} (tbl, ref, before, after, audit, at) VALUES (?, ?, ?, ?, ?, ?)",
                   (tbl, json.dumps(key), json.dumps(before) if before else None,
                    json.dumps(after) if after else None, a, now()))


def undo(db):
    out = []
    for r in rows(db, f"SELECT * FROM {LOG} ORDER BY seq DESC"):
        tbl, key = r["tbl"], json.loads(r["ref"])
        w, args = where(key)
        cur = rows(db, f"SELECT * FROM {tbl} WHERE {w}", args)
        before = json.loads(r["before"]) if r["before"] else None
        after = json.loads(r["after"]) if r["after"] else None
        if before is None:  # an insert: take it out
            if not cur:
                out.append(f"gone     {tbl} {key}: deleted since")
                continue
            db.execute(f"DELETE FROM {tbl} WHERE {w}", args)
            audit(db, "delete", tbl, key, cur[0], None)
            out.append(f"deleted  {tbl} {key}" + ("" if cur[0] == after else " (it had changed since)"))
        elif after is None:  # a delete: put it back
            if cur:
                out.append(f"left     {tbl} {key}: a row is there again")
                continue
            cols = list(before)
            db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                       [before[c] for c in cols])
            audit(db, "insert", tbl, key, None, before)
            out.append(f"restored {tbl} {key} (put back)")
        else:
            if len(cur) != 1 or cur[0] != after:
                out.append(f"left     {tbl} {key}: changed since")
                continue
            cols = [c for c in before if c not in key]
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in cols)} WHERE {w}",
                       [before[c] for c in cols] + args)
            audit(db, "update", tbl, key, after, before)
            out.append(f"restored {tbl} {key}")
    db.execute(f"DROP TABLE {LOG}")
    return out


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--data", default=DATA)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    with open(a.data, encoding="utf-8") as f:
        data = json.load(f)
    db = sqlite3.connect(a.db, timeout=5, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 5000")
    db.execute("BEGIN IMMEDIATE")
    try:
        logged = has_table(db, LOG)
        if a.undo:
            if not logged:
                print("nothing to undo: no log")
                db.execute("ROLLBACK")
                return 0
            for line in undo(db):
                print(line)
            db.execute("COMMIT")
            return 0
        if logged and a.apply:
            print(f"applied already ({LOG} exists): nothing written")
            db.execute("ROLLBACK")
            return 0
        plan = Plan(db, data)
        plan.build()
        for line in plan.lines:
            print(line)
        if not a.apply:
            print(f"dry run: {len(plan.changes)} rows would change")
            db.execute("ROLLBACK")
            return 0
        apply(db, plan)
        db.execute("COMMIT")
        print(f"applied: {len(plan.changes)} rows, a gm_audit line each, logged in {LOG}; the Eye at each player's "
              "next login, the quests at the next start")
        return 0
    except Refused as e:
        db.execute("ROLLBACK")
        print("refused:", e, file=sys.stderr)
        return 2
    except BaseException:
        db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
