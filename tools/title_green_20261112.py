#!/usr/bin/env python3
"""Dabrish's and the Testing Heroes Vendor's titles green (the GM Jibrish's petitions 1790 and 1791, 2026-11-12: "Color
the XP/CM title on Dabrish green, semi bright green so it shows but not too green as to be unreadable", and the same for
the Testing Heroes Vendor's title).

What it writes (a report by default; --apply writes, in one transaction with a gm_audit line a row; --undo takes it back):
- npctitle.color = 'green' on rows 162568 (Dabrish, "XP/CM's") and 163008 (Testing Heroes Vendor, "XP Grinding
  Rewards"): CLIENT's third name colour, NameColors[2] (90,255,80), the same for every viewer (entity/npctitle.go);
- the color column itself, when the table has none yet (the server that reads it adds it at its start; this adds the
  same column, so the tool can run before or after that start).

Needs the server with npctitle.go's colours (an older one ignores the column: the titles stay gold). No restart beyond
that server's own: a running new server reads it again at the GM panel's edit of npctitle, else at its next start; a
player sees it as the NPC next comes into view.

Refused, nothing written, unless both rows are there with those titles and a colour of '' (or both 'green': applied
already). Log title_green_1112_log; --undo puts each colour back only where it is still 'green', and drops the column
when this tool added it and every colour is '' again.

    tools/title_green_20261112.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "title_green_1112_log"
DETAIL = "Titles green (the GM Jibrish's petitions 1790/1791, 2026-11-12): tools/title_green_20261112.py"
WHO, VIA = "dab", "tool"
COLOR = "green"
ROWS = [(162568, "XP/CM's", "petition 1790 (Jibrish)"), (163008, "XP Grinding Rewards", "petition 1791 (Jibrish)")]
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq   INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl   TEXT NOT NULL,      -- npctitle, or 'made' for the color column this tool added
    npcid INTEGER,
    before TEXT,
    audit INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def has_color(db):
    return any(r[1] == "color" for r in db.execute("PRAGMA table_info(npctitle)"))


def row_of(db, nid):
    cur = db.execute("SELECT * FROM npctitle WHERE npcid = ?", (nid,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, nid, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', 'update', 'npctitle', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, go_json({"npcid": nid}), go_json(before), go_json(after), DETAIL))
    return cur.lastrowid


def plan(db):
    """(lines, refusals, applied)."""
    for t in ("npctitle", "gm_audit"):
        if not table(db, t):
            return [], [f"no {t} table here"], False
    col = has_color(db)
    refusals, lines, colors = [], [], []
    for nid, title, why in ROWS:
        r = row_of(db, nid)
        if r is None:
            refusals.append(f"npctitle {nid}: no row")
            continue
        if r["title"] != title:
            refusals.append(f"npctitle {nid}: title {r['title']!r}, not {title!r}")
        c = r.get("color", "") if col else ""
        colors.append(c)
        if c not in ("", COLOR):
            refusals.append(f"npctitle {nid}: colour {c!r} already (a GM's?)")
        lines.append(f"npctitle {nid} {title!r}: colour {c!r} -> {COLOR!r} ({why})")
    if not refusals and colors and all(c == COLOR for c in colors):
        return [], [], True
    if not refusals and any(c == COLOR for c in colors):
        refusals.append("one row green, the other not: half applied by someone else?")
    if not col:
        lines.insert(0, "npctitle: add column color TEXT NOT NULL DEFAULT '' (as the server's own start does)")
    return lines, refusals, False


def apply(db):
    db.execute(LOG_SCHEMA)
    if not has_color(db):
        db.execute("ALTER TABLE npctitle ADD COLUMN color TEXT NOT NULL DEFAULT ''")
        db.execute(f"INSERT INTO {LOG} (tbl) VALUES ('made')")
    for nid, _, _ in ROWS:
        before = row_of(db, nid)
        db.execute("UPDATE npctitle SET color = ? WHERE npcid = ?", (COLOR, nid))
        a = audit(db, nid, before, row_of(db, nid))
        db.execute(f"INSERT INTO {LOG} (tbl, npcid, before, audit) VALUES ('npctitle', ?, ?, ?)", (nid, json.dumps(before), a))
    return len(ROWS)


def undo(db):
    """(back, left, dropped)."""
    if not table(db, LOG):
        return 0, 0, False
    back = left = 0
    made = False
    for seq, tbl, nid, before in db.execute(f"SELECT seq, tbl, npcid, before FROM {LOG} ORDER BY seq DESC").fetchall():
        if tbl == "made":
            made = True
            continue
        was = json.loads(before)
        now = row_of(db, nid)
        if now is None or now.get("color") != COLOR:
            left += 1
            continue
        db.execute("UPDATE npctitle SET color = ? WHERE npcid = ?", (was.get("color", ""), nid))
        audit(db, nid, now, row_of(db, nid))
        back += 1
    dropped = False
    if made and not left and db.execute("SELECT count(*) FROM npctitle WHERE color != ''").fetchone()[0] == 0:
        db.execute("ALTER TABLE npctitle DROP COLUMN color")
        dropped = True
    db.execute(f"DROP TABLE {LOG}")
    return back, left, dropped


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left, dropped = undo(db)
            db.execute("COMMIT")
            print(f"undone: {back} colours back" + (f", {left} changed since (left)" if left else "") +
                  (", the color column dropped" if dropped else ""))
            return 0
        lines, refusals, applied = plan(db)
        if applied:
            db.execute("ROLLBACK")
            print("applied already: nothing to do")
            return 0
        for l in lines:
            print(l)
        if refusals:
            db.execute("ROLLBACK")
            print("REFUSED:\n  " + "\n  ".join(refusals))
            return 1
        if not a.apply:
            db.execute("ROLLBACK")
            print("(report only: --apply to write)")
            return 0
        n = apply(db)
        db.execute("COMMIT")
        print(f"applied: {n} rows, in gm_audit and {LOG}")
        return 0
    except Exception:
        db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
