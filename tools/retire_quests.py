#!/usr/bin/env python3
"""Take out the quests a content file no longer has.

tools/import_quests.py keeps a quest by its key (quest_import_quests) and writes what its files hold; a quest whose
key a file DROPS stays in the database as it was, offered as before. This takes those out: for each file given, the
quests the import logged for it (by the file's name) whose keys it no longer has, with their steps, the trips they
unlocked, their steps' other creatures, the characters' progress in them (a journal line under way, a finished mark)
and their lines of the import's log. The items an import made for them stay (a character may hold one).

The homelands' first three starter quests went this way when the homes took the cities' template (2026-11-10:
tools/quests/gen_homelands.py; "welcome" and the hunt of each of the 21 homes, 42 quests).

A quest another quest still requires, or a mastery rank asks for (cmcatalog.require_quests), is not taken out:
that is said, and nothing is written (--apply refuses the whole run).

Everything taken out is kept, row for row, in quest_retire_log; --undo puts every row back as it was and drops
the log (a second retiring before an undo adds to the same log). A report by default; --apply writes. Stop the
server to apply or undo (the quest engine reads the quests at its start), start it after; a character in the
world with one of them under way has the line in its journal until its next login.

    tools/retire_quests.py --db eqoa.db tools/quests/data/homelands.json            the report
    tools/retire_quests.py --db eqoa.db --apply tools/quests/data/homelands.json    take them out
    tools/retire_quests.py --db eqoa.db --undo                                      put them all back
"""
import argparse
import json
import os
import sqlite3
import sys

LOG = "quest_retire_log"
LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq  INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl  TEXT NOT NULL,     -- the table the row was in
    row  TEXT NOT NULL,     -- its columns (JSON)
    key  TEXT NOT NULL DEFAULT '',   -- the quest's import key
    at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
)"""
# the tables that hold a quest's rows, by its id (those a database has)
TABLES = ("queststeps", "questunlocks", "queststeptargets", "charquests", "quests", "quest_import_quests")


def has_table(db, t):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone()[0] > 0


def columns(db, t):
    return [r[1] for r in db.execute(f"PRAGMA table_info({t})")]


def own_rowid(db, t):
    """Whether the table's rows have a rowid that is no column of theirs (none an INTEGER PRIMARY KEY of its own):
    kept and put back with the row, so a step's other creatures come back in the order they were written."""
    info = db.execute(f"PRAGMA table_info({t})").fetchall()
    pk = [r for r in info if r[5]]
    alias = len(pk) == 1 and pk[0][2].upper() == "INTEGER"
    without = "WITHOUT ROWID" in (db.execute("SELECT sql FROM sqlite_master WHERE name = ?", (t,)).fetchone()[0] or "").upper()
    return not alias and not without


def requires_of(text):
    """The quest ids a requires column names (database.ParseRequires: "a, b|c")."""
    out = set()
    for part in str(text or "").replace("|", ",").split(","):
        if part.strip().isdigit():
            out.add(int(part))
    return out


def plan(db, paths):
    """What a retiring would take out: [(key, questid, name)], and why not (problems)."""
    if not has_table(db, "quest_import_quests"):
        raise SystemExit("no quest import in this database: nothing to retire")
    gone, problems = [], []
    for path in paths:
        fname = os.path.basename(path)
        with open(path) as f:
            keys = {q.get("key") for q in json.load(f).get("quests") or []}
        logged = db.execute("SELECT key, questid FROM quest_import_quests WHERE file = ? ORDER BY questid", (fname,)).fetchall()
        if not logged:
            problems.append(f"{fname}: the import logged no quest for a file of that name")
        for key, qid in logged:
            if key in keys:
                continue
            row = db.execute("SELECT name FROM quests WHERE questid = ?", (qid,)).fetchone()
            gone.append((key, qid, row[0] if row else "(its quest row is gone already)"))
    ids = {qid for _, qid, _ in gone}
    if ids:
        for qid, name, req in db.execute("SELECT questid, name, requires FROM quests"):
            if qid not in ids and requires_of(req) & ids:
                problems.append(f"quest {qid} {name!r} requires {sorted(requires_of(req) & ids)}, to be taken out: mend it first")
        if has_table(db, "cmcatalog") and "require_quests" in columns(db, "cmcatalog"):
            for cid, name, req in db.execute("SELECT id, name, require_quests FROM cmcatalog WHERE require_quests != ''"):
                if requires_of(req) & ids:
                    problems.append(f"mastery rank {cid} {name!r} asks for {sorted(requires_of(req) & ids)}, to be taken out")
    return gone, problems


def apply(db, gone):
    """Take the quests out, each row kept in the log; returns rows taken by table."""
    n = {}
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_DDL)
        for key, qid, _ in gone:
            for t in TABLES:
                if not has_table(db, t):
                    continue
                cols = columns(db, t)
                sel = (["rowid"] if own_rowid(db, t) else []) + cols
                for r in db.execute(f"SELECT {', '.join(sel)} FROM {t} WHERE questid = ? ORDER BY rowid", (qid,)).fetchall():
                    db.execute(f"INSERT INTO {LOG} (tbl, row, key) VALUES (?, ?, ?)", (t, json.dumps(dict(zip(sel, r))), key))
                n[t] = n.get(t, 0) + db.execute(f"DELETE FROM {t} WHERE questid = ?", (qid,)).rowcount
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return n


def undo(db):
    """Put every row of the log back and drop it; returns rows put back by table."""
    if not has_table(db, LOG):
        raise SystemExit("nothing retired here: nothing to undo")
    n = {}
    db.execute("BEGIN IMMEDIATE")
    try:
        for t, row in db.execute(f"SELECT tbl, row FROM {LOG} ORDER BY seq DESC").fetchall():
            row = json.loads(row)
            have = set(columns(db, t)) | ({"rowid"} if own_rowid(db, t) else set())
            cols = [c for c in row if c in have]
            db.execute(f"INSERT INTO {t} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [row[c] for c in cols])
            n[t] = n.get(t, 0) + 1
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return n


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("files", nargs="*", help="the content files whose dropped quests go")
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None)
    said = lambda n: ", ".join(f"{v} of {t}" for t, v in sorted(n.items()) if v) or "nothing"
    if a.undo:
        print(f"{a.db}: put back {said(undo(db))}; restart the server")
        return 0
    if not a.files:
        ap.error("a content file (or --undo)")
    gone, problems = plan(db, a.files)
    for key, qid, name in gone:
        under, done = (db.execute("SELECT COALESCE(sum(step > 0), 0), COALESCE(sum(done > 0), 0) FROM charquests WHERE questid = ?",
                                  (qid,)).fetchone() if has_table(db, "charquests") else (0, 0))
        print(f"  {qid} {key}: {name!r}" + (f" ({under} under way, {done} finished)" if under or done else ""))
    for p in problems:
        print("ERROR:", p)
    if a.apply:
        if problems:
            print("nothing written")
            return 1
        print(f"{a.db}: {len(gone)} quests taken out ({said(apply(db, gone))}), kept in {LOG}; restart the server")
    else:
        print(f"{len(gone)} quests to take out (--apply takes them)" + (f"; {len(problems)} errors" if problems else ""))
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main())
