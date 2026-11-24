#!/usr/bin/env python3
"""Tak Frostbiter's placeholders, both shards (Heirloom petition 2568, reopened 2026-11-24: "no placeholders
spawning in, no Tak in sight either").

~/charmigrate/tak_frostbiter_2568_20261121.py made Tak a rare on five Freezeblood berserkers (npcrare rows
100236, 100243, 100245, 100247, 100252, 10% each) and kept his own row out of the world with an npcspawnonly row
by NAME, "Tak Frostbiter". npcrare's contract (database/npcrare.go, entity/npcrare.go) is the rare's row out of
the world by its ROW, "#<id>": a placeholder that rolls its rare is remade from the rare's row (makeKindLocked),
and applyNPCSettingsLocked hides every NPC whose NAME is spawn-only. So each placeholder that rolled Tak became
a spawn-only Tak: out of every view, never fought, never killed, so never back as a berserker, and not counted
by rareAliveLocked, so the next placeholder could roll him too. One by one the tents emptied and no Tak showed.
A keep restart carries a placeholder standing as its rare (keepnpc.go), so restarts did not bring them back.

This swaps the name row for the row "#100241" (Tak's own row stays out of the world; his copies on the
placeholders stand in it), the way Lord Koreshaa's is (koreshaa1113). Nothing else changes.

Refused, nothing written, if Tak's row is not 100241 "Tak Frostbiter" or the npcrare rows naming him are not the
five. Every row written is a gm_audit line (via "tool", [basis:shared petition 2568]) and a row of the log;
--undo puts each back (a row changed since is left and said) and drops the log. A report by default; --apply
writes once (a second finds the log and writes nothing).

WHEN IT TAKES EFFECT: npcspawnonly is read at the start and at a GM panel edit of the table (gmweb liveSpawnOnly):
touch the "#100241" row's note in the panel, or restart. A placeholder hidden as Tak comes back into view then.

    python3 -E -s tools/petpass_both_1124al.py --db eqoa.db             report
    python3 -E -s tools/petpass_both_1124al.py --db eqoa.db --apply     write it
    python3 -E -s tools/petpass_both_1124al.py --db eqoa.db --undo      take it back
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "petpass_both_1124al_log"
WHO, VIA = "claude", "tool"
DETAIL = ("[basis:shared petition 2568] Tak Frostbiter out of the world by his row (#100241), not his name: a "
          "placeholder that rolled him was hidden as spawn-only and never came back: tools/petpass_both_1124al.py")
TAK, NAME = 100241, "Tak Frostbiter"
PH = [100236, 100243, 100245, 100247, 100252]
ROWKEY = f"#{TAK}"
NOTE = ("Tak Frostbiter's own row, out of the world by its row: his copies on the Freezeblood tents' placeholders "
        "(npcrare) stand in it; petition 2568")
LOG_DDL = """CREATE TABLE IF NOT EXISTS {log} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    action TEXT NOT NULL,
    tbl    TEXT NOT NULL,
    ref    TEXT NOT NULL,
    before TEXT,
    after  TEXT,
    audit  INTEGER,
    at     TEXT NOT NULL
)"""


class Refused(Exception):
    pass


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def row(db, name):
    r = db.execute("SELECT name, note, source FROM npcspawnonly WHERE name = ?", (name,)).fetchone()
    return dict(zip(("name", "note", "source"), r)) if r else None


def audit(db, action, key, before, after):
    cur = db.execute("""INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after,
        detail) VALUES (?, 0, ?, ?, '', ?, 'npcspawnonly', ?, ?, ?, ?)""",
                     (now(), WHO, VIA, action, go_json(key), go_json(before) if before else None,
                      go_json(after) if after else None, DETAIL))
    return cur.lastrowid


def log_row(db, action, key, before, after, a):
    db.execute(f"INSERT INTO {LOG} (action, tbl, ref, before, after, audit, at) VALUES (?, 'npcspawnonly', ?, ?, ?, ?, ?)",
               (action, json.dumps(key), json.dumps(before) if before else None,
                json.dumps(after) if after else None, a, now()))


def plan(db):
    r = db.execute("SELECT npc_name FROM npcs WHERE id = ?", (TAK,)).fetchone()
    if not r or r[0] != NAME:
        raise Refused(f"npcs {TAK} is {r}, not {NAME!r}")
    named = db.execute("SELECT min(id) FROM npcs WHERE npc_name = ?", (NAME,)).fetchone()[0]
    if named != TAK:
        raise Refused(f"the lowest row called {NAME!r} is {named}, not {TAK}")
    phs = sorted(x[0] for x in db.execute("SELECT npc_row FROM npcrare WHERE rare = ?", (NAME,)))
    if phs != PH:
        raise Refused(f"npcrare rows for {NAME!r} are {phs}, not {PH}")
    steps = []
    old = row(db, NAME)
    if old:
        steps.append(("delete", {"name": NAME}, old, None))
    if not row(db, ROWKEY):
        steps.append(("insert", {"name": ROWKEY}, None, {"name": ROWKEY, "note": NOTE, "source": "petpass_both_1124al"}))
    return steps


def apply(db, steps):
    db.execute(LOG_DDL.format(log=LOG))
    for action, key, before, after in steps:
        if action == "delete":
            db.execute("DELETE FROM npcspawnonly WHERE name = ?", (key["name"],))
        else:
            db.execute("INSERT INTO npcspawnonly (name, note, source) VALUES (?, ?, ?)",
                       (after["name"], after["note"], after["source"]))
        log_row(db, action, key, before, after, audit(db, action, key, before, after))


def undo(db):
    out = []
    for r in db.execute(f"SELECT action, ref, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        action, key = r[0], json.loads(r[1])
        before = json.loads(r[2]) if r[2] else None
        after = json.loads(r[3]) if r[3] else None
        cur = row(db, key["name"])
        if action == "insert":
            if cur != after:
                out.append(f"left     {key}: changed since")
                continue
            db.execute("DELETE FROM npcspawnonly WHERE name = ?", (key["name"],))
            audit(db, "delete", key, after, None)
            out.append(f"removed  {key}")
        else:
            if cur is not None:
                out.append(f"left     {key}: a row is there again")
                continue
            db.execute("INSERT INTO npcspawnonly (name, note, source) VALUES (?, ?, ?)",
                       (before["name"], before["note"], before["source"]))
            audit(db, "insert", key, None, before)
            out.append(f"restored {key}")
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
    db.execute("BEGIN IMMEDIATE")
    try:
        logged = bool(db.execute("SELECT 1 FROM sqlite_master WHERE type='table' AND name=?", (LOG,)).fetchone())
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
        steps = plan(db)
        for action, key, before, after in steps:
            print(f"{action:7s} npcspawnonly {key}")
        if not steps:
            print("already as wanted: nothing to do")
        if a.apply and steps:
            apply(db, steps)
            db.execute("COMMIT")
            print(f"applied: {len(steps)} rows ({LOG})")
        else:
            db.execute("ROLLBACK")
            print("report only" if not a.apply else "nothing written")
        return 0
    except Refused as e:
        db.execute("ROLLBACK")
        print("refused:", e, file=sys.stderr)
        return 2
    except Exception:
        db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
