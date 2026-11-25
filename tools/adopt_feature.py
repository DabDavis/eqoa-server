#!/usr/bin/env python3
"""Mark a petition's feature as adopted, for the Idea Contributors board (handlers/ideaboard.go; petition 3016,
Cucumber, approved by the owner 2026-11-25).

A row of petitionadopted (database/ideaboard.go) a feature a petition asked for that the game took up, by its name as
the board lists it. Two players who asked for the same feature each get a row (each petition its own), and each has
it on the board. The GM panel's table "Adopted features" is the same rows.

    python3 -E -s tools/adopt_feature.py --db eqoa.db --id 3008 --feature "Auto Loot" [--by dab] [--note ...] [--apply]
    python3 -E -s tools/adopt_feature.py --db eqoa.db --file backfill-heirloom.tsv [--apply]
    python3 -E -s tools/adopt_feature.py --db eqoa.db --list
    python3 -E -s tools/adopt_feature.py --db eqoa.db --remove --id 3008 --feature "Auto Loot" [--apply]

A --file is tab-separated: petition id, feature, and optionally the character that filed it and a note; a line
starting with # is a comment. A character named must be the petition's (charname, any case): a file made for the
other shard is refused, nothing written. A report by default; --apply writes it, in one transaction, with a gm_audit
line a row (keyed {"id": n}, as the panel keys a row, so the panel's revert_change undoes it). Idempotent: a mark
already there (the same petition and feature in any case) is left alone. A petition that is not a Feature Request
(category 1, subcategory 100) is said, not refused (the mark is our decision). The table is made here if the server
has not made it yet, so this can run before the deploy's restart. The board counts again within a minute: no restart.
"""
import argparse
import datetime
import json
import sqlite3
import sys

SCHEMA = """
CREATE TABLE IF NOT EXISTS petitionadopted (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    petitionid INTEGER NOT NULL,
    feature    TEXT NOT NULL,
    adopted_at TEXT NOT NULL DEFAULT '',
    adopted_by TEXT NOT NULL DEFAULT '',
    note       TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_petitionadopted ON petitionadopted(petitionid, feature COLLATE NOCASE);
"""
VIA = "tool"
DETAIL = "an adopted feature for the Idea Contributors board (petition 3016): tools/adopt_feature.py"


def go_json(m):
    return json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def row(db, rid):
    cur = db.execute("SELECT * FROM petitionadopted WHERE id = ?", (rid,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def audit(db, who, action, rid, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (who,)).fetchone()
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', ?, 'petitionadopted', ?, ?, ?, ?)",
               (now(), acct[0] if acct else 0, who, VIA, action, go_json({"id": rid}),
                go_json(before) if before is not None else "", go_json(after) if after is not None else "", DETAIL))


def read_file(path):
    out = []
    with open(path, encoding="utf-8") as f:
        for n, line in enumerate(f, 1):
            line = line.rstrip("\n")
            if not line.strip() or line.lstrip().startswith("#"):
                continue
            cols = line.split("\t")
            if len(cols) < 2:
                sys.exit(f"{path}:{n}: want petition id <tab> feature [<tab> character [<tab> note]]")
            try:
                pid = int(cols[0].strip())
            except ValueError:
                sys.exit(f"{path}:{n}: {cols[0]!r} is no petition id")
            out.append((pid, cols[1].strip(), cols[2].strip() if len(cols) > 2 else "",
                        cols[3].strip() if len(cols) > 3 else "", f"{path}:{n}"))
    return out


def plan(db, marks):
    """(adds, refusals): adds are (petition, feature, note, where) not yet there."""
    adds, refusals = [], []
    seen = set()
    for pid, feature, who, note, where in marks:
        if not feature:
            refusals.append(f"{where}: petition {pid}: no feature named")
            continue
        p = db.execute("SELECT charname, category, subcategory, status FROM petitions WHERE id = ?", (pid,)).fetchone()
        if p is None:
            refusals.append(f"{where}: no petition {pid} here")
            continue
        if who and who.lower() != (p[0] or "").lower():
            refusals.append(f"{where}: petition {pid} is {p[0]!r}'s, not {who!r}: the other shard's file?")
            continue
        key = (pid, feature.lower())
        if key in seen:
            continue
        seen.add(key)
        if db.execute("SELECT 1 FROM petitionadopted WHERE petitionid = ? AND feature = ? COLLATE NOCASE",
                      (pid, feature)).fetchone():
            print(f"  have: petition {pid} ({p[0]}): {feature}")
            continue
        kind = "" if (p[1], p[2]) == (1, 100) else f"  (not a Feature Request: category {p[1]}/{p[2]})"
        print(f"  add:  petition {pid} ({p[0]}, {p[3]}): {feature}{kind}")
        adds.append((pid, feature, note, where))
    return adds, refusals


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--id", type=int)
    ap.add_argument("--feature")
    ap.add_argument("--file")
    ap.add_argument("--by", default="dab", help="the GM the marks are said to be by (and audited as): dab")
    ap.add_argument("--note", default="")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--list", action="store_true")
    g.add_argument("--remove", action="store_true")
    ap.add_argument("--apply", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 5000")
    if a.list:
        if not db.execute("SELECT 1 FROM sqlite_master WHERE type='table' AND name='petitionadopted'").fetchone():
            print("no petitionadopted table yet")
            return
        for r in db.execute("""SELECT f.id, f.petitionid, COALESCE(p.charname, '?'), f.feature, f.adopted_at, f.adopted_by
                               FROM petitionadopted f LEFT JOIN petitions p ON p.id = f.petitionid ORDER BY f.id"""):
            print("\t".join(str(x) for x in r))
        return
    if a.file:
        if a.id or a.feature:
            sys.exit("--file, or --id with --feature")
        marks = read_file(a.file)
    elif a.id and a.feature:
        marks = [(a.id, a.feature.strip(), "", a.note, "the command line")]
    else:
        sys.exit("--id with --feature, or --file, or --list")
    db.execute("BEGIN IMMEDIATE")
    try:
        for stmt in SCHEMA.split(";"):
            if stmt.strip():
                db.execute(stmt)
        if a.remove:
            n = 0
            for pid, feature, _, _, _ in marks:
                r = db.execute("SELECT id FROM petitionadopted WHERE petitionid = ? AND feature = ? COLLATE NOCASE",
                               (pid, feature)).fetchone()
                if r is None:
                    print(f"  none: petition {pid}: {feature}")
                    continue
                before = row(db, r[0])
                print(f"  remove: #{r[0]} petition {pid}: {before['feature']}")
                if a.apply:
                    db.execute("DELETE FROM petitionadopted WHERE id = ?", (r[0],))
                    audit(db, a.by, "delete", r[0], before, None)
                n += 1
            if a.apply:
                db.execute("COMMIT")
                print(f"removed {n}")
            else:
                db.execute("ROLLBACK")
                print(f"{n} to remove (report only: --apply removes them)")
            return
        adds, refusals = plan(db, marks)
        if refusals:
            for r in refusals:
                print("REFUSED:", r)
            db.execute("ROLLBACK")
            sys.exit(1)
        if not adds:
            print("nothing to do")
            db.execute("ROLLBACK")
            return
        if not a.apply:
            print(f"{len(adds)} to add (report only: --apply writes them)")
            db.execute("ROLLBACK")
            return
        at = now()
        for pid, feature, note, _ in adds:
            rid = db.execute("INSERT INTO petitionadopted (petitionid, feature, adopted_at, adopted_by, note) VALUES (?, ?, ?, ?, ?)",
                             (pid, feature, at, a.by, note or a.note)).lastrowid
            audit(db, a.by, "insert", rid, None, row(db, rid))
        db.execute("COMMIT")
        print(f"applied: {len(adds)} added")
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    main()
