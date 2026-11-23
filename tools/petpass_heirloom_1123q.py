#!/usr/bin/env python3
"""Heirloom petition 2816 of 2026-11-23, as data: Hunt & Gather quests' levels against their sources.

Every row is found by its giver, name and source, never by an id alone; a row already as this writes it is left
("already"); anything else refuses the whole run. The same rows stand on Live (same ids, matched by name here too).

  2816  "Udrik Lazow in HP keeps giving me the 1-10 destroyed pickclaw sashes quest even though I'm in my 20's. He is
        also giving me the 21-30 torn pickclaw sashes quest" (Dimipots, level 27). The offer is gated by level
        (handlers/quests.go mayTakeAtLocked: minlevel..maxlevel), and a giver offers each quest the player may take in
        turn, the fewest-done first (nextOfferLocked): at 27 that is the first tier and the third. The first tier's
        levels are 1-30 on purpose: teamjosh.com's H&G page (the owner's authority for these quests, 2026-11-15,
        tools/handg_quests_20261115.py key 85: "Pickclaw Brawler and Shaman, 1-30") and EQOA-Data's own guide
        ("Farm camps to level 30, then ... speak to NPC for harder quest"). Its NAME said "(1-10)", the import's
        guess: renamed "Pickclaw Sashes (1-30)", so the journal says what the giver does.

  The sweep: all 82 enabled Hunt & Gather quests against teamjosh (handg_quests_20261115.py's report: one levels
  difference) and EQOA-Data (tools/quests/data/hunt-and-gather.json: the rest are teamjosh's own, owner-set). One
  range was wrong: Corison's Dark Ore 26-60, widened by a panel edit 2026-11-21 09:46 (no reason given) from 26-32,
  which teamjosh (key 42), EQOA-Data (corison-dark-ore) and MasterVG782's GameFAQs guide ("LEVELS -- 26-32") all
  give. Back to 26-32. No character holds it.

Every row written is a gm_audit line (via "tool") and a row of the log; --undo puts each back as it was (a row
changed since is left and said) and drops the log. A report by default; --apply writes once (a second finds the log
and writes nothing); --apply and --undo copy the database first (<db>.bak-<when>-pre-pet1123q, before the write
lock is taken) unless --no-backup.

WHEN IT TAKES EFFECT: quests are read at the server's start (and at a GM panel quest edit): the next restart.

    python3 -E -s tools/petpass_heirloom_1123q.py --db eqoa.db            report
    python3 -E -s tools/petpass_heirloom_1123q.py --db eqoa.db --apply    write it
    python3 -E -s tools/petpass_heirloom_1123q.py --db eqoa.db --undo     take it back
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "petpass_heirloom_1123q_log"
WHO, VIA = "claude", "tool"
DETAIL = "Heirloom petition 2816 (2026-11-23): Hunt & Gather levels against their sources (Udrik's first tier named 1-30, Corison's Dark Ore 26-32): tools/petpass_heirloom_1123q.py"

# 2816: (giver, name now, source prefix, the levels it must have, {column: (before, after)})
EDITS = [
    ("Udrik Lazow", "Pickclaw Sashes (1-10)", "EQOA-Data HuntAndGather/udrik-pickclaw-1 ", (1, 30),
     {"name": ("Pickclaw Sashes (1-10)", "Pickclaw Sashes (1-30)")}),
    ("Corison", "Dark Ore", "EQOA-Data HuntAndGather/corison-dark-ore ", (26, None),
     {"maxlevel": (60, 32)}),
]

KEYS = {"quests": ("questid",)}
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

    def build(self):
        db = self.db
        for giver, name, source, levels, cols in EDITS:
            new_name = cols.get("name", (name, name))[1]
            got = rows(db, """SELECT * FROM quests WHERE giver = ? AND name IN (?, ?) AND source LIKE ?""",
                       (giver, name, new_name, source + "%"))
            what = f"2816 quests {giver}'s {name}"
            if len(got) != 1:
                raise Refused(f"{what}: {len(got)} rows, want one")
            q = got[0]
            what = f"2816 quests {q['questid']} {giver}'s {name} ({q['minlevel']}-{q['maxlevel']})"
            if not q["enabled"] or not q["repeatable"]:
                raise Refused(f"{what}: not an enabled repeatable quest")
            if q["minlevel"] != levels[0] or (levels[1] is not None and q["maxlevel"] != levels[1]):
                raise Refused(f"{what}: levels {q['minlevel']}-{q['maxlevel']}, want {levels[0]}-{levels[1]}")
            if all(q[c] == after for c, (_, after) in cols.items()):
                self.lines.append(f"already  {what}")
                continue
            bad = [c for c, (before, _) in cols.items() if q[c] != before]
            if bad:
                raise Refused(f"{what}: {', '.join(f'{c} is {q[c]!r}' for c in bad)}, not as read")
            values = {c: after for c, (_, after) in cols.items()}
            self.changes.append(("quests", {"questid": q["questid"]}, q, values))
            self.lines.append(f"update   {what}: " + ", ".join(f"{c} {q[c]!r} -> {v!r}" for c, v in values.items()))


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
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-pet1123q"
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
    if (a.apply or a.undo) and not a.no_backup:
        print("backup", backup(a.db))
    db = sqlite3.connect(a.db, timeout=5, isolation_level=None)
    db.execute("BEGIN IMMEDIATE")
    try:
        logged = has_table(db, log)
        if a.undo:
            if not logged:
                print("nothing to undo: no log")
                db.execute("ROLLBACK")
                return 0
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
