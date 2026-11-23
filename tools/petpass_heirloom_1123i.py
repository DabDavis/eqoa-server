#!/usr/bin/env python3
"""Heirloom petition 2806 of 2026-11-23, as data, for the Heirloom shard: the alchemist's thrown potions drew the
clay drink jug (icon -506671519, the arcadin brandy's).

The same mapping as Live's petition 5531 (tools/petpass_live_1123b.py): those that strike at once draw the blue
flask (icon 209763529, as Fulminant Potion, Moradhim and Freeport Cocktail and Freezing Potion already did), those
that burn over time the red one (824019198, the tonics' vial), each read off its itemeffect line ("HP per tic":
over time; the Cocktail of Death, a strike and a burn, goes red as on Live). Every record is found by its NAME as an
alchemist's (classuse bit 14) thrown potion (equipslot 16) and must read the jug; one already on its flask is left
("already"); anything else refuses the whole run.

Every row written is a gm_audit line (via "tool") and a row of petpass_heirloom_1123i_log; --undo puts each back as
it was (a row changed since is left and said) and drops the log. A report by default; --apply writes once (a second
finds the log and writes nothing).

WHEN IT TAKES EFFECT: at each player's next login.

    python3 -E -s tools/petpass_heirloom_1123i.py --db eqoa.db            report
    python3 -E -s tools/petpass_heirloom_1123i.py --db eqoa.db --apply    write it
    python3 -E -s tools/petpass_heirloom_1123i.py --db eqoa.db --undo     take it back
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "petpass_heirloom_1123i_log"
WHO, VIA = "claude", "tool"
DETAIL = "Heirloom petition 2806 (2026-11-23): tools/petpass_heirloom_1123i.py"

VIAL = 824019198
JUG = -506671519
BLUE = 209763529
STRIKE = ["Unstable Potion", "Volatile Potion", "Explosive Potion", "Klik'Anon Cocktail", "Detonating Potion",
          "Frosty Potion", "Grobb Cocktail", "Meteoric Potion", "Icy Potion", "Qeynos Cocktail", "Kinetic Potion",
          "Severupidus' Cocktail", "Ascended Severupidus' Cocktail"]
OVER_TIME = ["Acidic Potion", "Caustic Potion", "Corroding Potion", "Corrosive Potion", "Acerbic Potion",
             "Virulent Potion", "Septic Potion", "Cocktail of Death Potion", "Ascended Cocktail of Death Potion"]

KEYS = {"itempattern": ("patternid",)}
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


def one(db, what, sql, args=()):
    got = rows(db, sql, args)
    if len(got) != 1:
        raise Refused(f"{what}: {len(got)} rows, want one")
    return got[0]


def near(a, b):
    return abs(float(a) - float(b)) < 0.05


def same(a, b):
    return near(a, b) if isinstance(b, float) else a == b


class Plan:
    def __init__(self, db):
        self.db = db
        self.changes = []  # (tbl, key dict, before row or None, values or None: a delete)
        self.lines = []

    def set(self, tbl, row, values, olds, what):
        """values over row, when each column of olds reads its old; already as values: left."""
        if all(same(row[c], v) for c, v in values.items()):
            self.lines.append(f"already  {what}")
            return
        for c, old in olds.items():
            if not same(row[c], old):
                raise Refused(f"{what}: {c} reads {row[c]!r}, not {old!r} as read when this was made")
        key = {k: row[k] for k in KEYS[tbl]}
        self.changes.append((tbl, key, dict(row), dict(values)))
        self.lines.append(f"change   {what}: " + ", ".join(f"{c} {row[c]!r} -> {v!r}" for c, v in values.items()))

    def build(self):
        db = self.db
        for names, icon, why in ((STRIKE, BLUE, "strikes at once: the blue flask"),
                                 (OVER_TIME, VIAL, "burns over time: the red flask")):
            for name in names:
                r = one(db, f"the thrown potion {name!r}", "SELECT * FROM itempattern WHERE itemname = ? AND "
                        "equipslot = 16", (name,))
                eff = one(db, f"{name!r}'s effect", "SELECT line FROM itemeffect WHERE patternid = ?", (r["patternid"],))
                if ("HP per tic" in eff["line"]) != (icon == VIAL):
                    raise Refused(f"2806 {name!r}: its line {eff['line']!r} does not read as {why}")
                if r["equipslot"] != 16 or not r["classuse"] & (1 << 14):
                    raise Refused(f"2806 {name!r} is not an alchemist's thrown potion")
                self.set("itempattern", r, {"itemicon": icon}, {"itemicon": JUG}, f"2806 {name!r}'s icon ({why})")


def audit(db, action, tbl, key, before, after):
    db.execute(AUDIT_DDL)
    cur = db.execute("""INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after,
        detail) VALUES (?, 0, ?, ?, '', ?, ?, ?, ?, ?, ?)""",
                     (now(), WHO, VIA, action, tbl, go_json(key), go_json(before) if before else None,
                      go_json(after) if after else None, DETAIL))
    return cur.lastrowid


def where(key):
    return " AND ".join(f"{k} = ?" for k in key), list(key.values())


def apply(db, plan):
    db.execute(LOG_DDL)
    for tbl, key, before, values in plan.changes:
        w, args = where(key)
        if values is None:
            db.execute(f"DELETE FROM {tbl} WHERE {w}", args)
            a = audit(db, "delete", tbl, key, before, None)
            after = None
        else:
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in values)} WHERE {w}",
                       list(values.values()) + args)
            after = rows(db, f"SELECT * FROM {tbl} WHERE {w}", args)[0]
            a = audit(db, "update", tbl, key, before, after)
        db.execute(f"INSERT INTO {LOG} (tbl, ref, before, after, audit, at) VALUES (?, ?, ?, ?, ?, ?)",
                   (tbl, json.dumps(key), json.dumps(before), json.dumps(after) if after else None, a, now()))


def undo(db):
    out = []
    for r in rows(db, f"SELECT * FROM {LOG} ORDER BY seq DESC"):
        tbl, key = r["tbl"], json.loads(r["ref"])
        w, args = where(key)
        now_rows = rows(db, f"SELECT * FROM {tbl} WHERE {w}", args)
        before = json.loads(r["before"])
        if r["after"] is None:  # a delete: put the row back
            if now_rows:
                out.append(f"left     {tbl} {key}: a row is there again")
                continue
            cols = list(before)
            db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                       [before[c] for c in cols])
            audit(db, "insert", tbl, key, None, before)
        else:
            after = json.loads(r["after"])
            if len(now_rows) != 1 or now_rows[0] != after:
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
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, timeout=5, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 5000")
    db.execute("BEGIN IMMEDIATE")
    try:
        logged = db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (LOG,)).fetchone()
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
        plan = Plan(db)
        plan.build()
        for line in plan.lines:
            print(line)
        if not a.apply:
            print(f"dry run: {len(plan.changes)} rows would change")
            db.execute("ROLLBACK")
            return 0
        apply(db, plan)
        db.execute("COMMIT")
        print(f"applied: {len(plan.changes)} rows, a gm_audit line each, logged in {LOG}")
        return 0
    except Refused as e:
        db.execute("ROLLBACK")
        print("refused:", e, file=sys.stderr)
        return 2
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
