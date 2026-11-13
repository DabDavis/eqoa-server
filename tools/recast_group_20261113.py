#!/usr/bin/env python3
"""The ranger's Wolf's Instinct and Hunter's Instinct share one recast (ticket 2011, Brother, 2026-11-13: "should Not
be able to cast Hunter Form or Wolf form while one or the other is on Cooldown ... should Not be able to have both").

Wolf's Instinct (spell 701068, mastery 3371, L51) and its free upgrade Hunter's Instinct (760110, mastery 900645, the
same buff with no wolf form, petition 1183) both stay in the book (ticket 1241 asked the upgrade to keep the old
ability), each with its own 1800 s recast, so a ranger cast one and then the other. This puts both in recast group
"ranger instinct" (table spellrecastgroup, entity/recastgroup.go): casting either locks out both for the cast
spell's own lockout (each button greyed by its own 0x00CD), and their buffs are one line, the newer taking the older
off.

Refused, nothing written, unless both spells are there as named, of level 51 with a recast of 1800, the two
masteries add them (900645 a child of 3371 that removes nothing), and neither spell is in another group. The table
is made (the server's own schema) when the database has none yet (a server before this build). Report only by
default; --apply in one transaction, a gm_audit line a row, log recast_group_1113_log; --undo exact (a row changed
since is left and said; the table dropped again when this tool made it and it is empty). Read at each cast: no
restart for the data (the code that reads it needs this build).

    tools/recast_group_20261113.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "recast_group_1113_log"
DETAIL = "Wolf's Instinct and Hunter's Instinct share one recast (ticket 2011): tools/recast_group_20261113.py"
WHO, VIA = "dab", "tool"
GROUP = "ranger instinct"
# spellid -> (name, mastery that adds it)
SPELLS = {701068: ("Wolf's Instinct", 3371), 760110: ("Hunter's Instinct", 900645)}
LEVEL, RECAST = 51, 1800
PARENT, CHILD = 3371, 900645

TABLE_SCHEMA = """CREATE TABLE IF NOT EXISTS spellrecastgroup (
		spellid INTEGER PRIMARY KEY,
		grp     TEXT NOT NULL
	)"""

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    spellid INTEGER NOT NULL,
    after   TEXT NOT NULL,      -- the row as written (JSON)
    created INTEGER NOT NULL,   -- 1: this tool made table spellrecastgroup
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def rows(db, sql, args=()):
    cur = db.execute(sql, args)
    cols = [d[0] for d in cur.description]
    return [dict(zip(cols, r)) for r in cur.fetchall()]


def get(db, sid):
    if not table(db, "spellrecastgroup"):
        return None
    r = rows(db, "SELECT * FROM spellrecastgroup WHERE spellid = ?", (sid,))
    return r[0] if r else None


def go_json(m):
    return json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, sid, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() \
        if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, 'spellrecastgroup', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, go_json({"spellid": sid}),
                      go_json(before) if before else None, go_json(after) if after else None, detail))
    return cur.lastrowid


def ids(s):
    return [int(x) for x in str(s).replace(";", ",").split(",") if x.strip().lstrip("-").isdigit()]


def changes(db):
    """(changes, lines, refusals, applied): changes the spell ids to write."""
    lines, refusals = [], []
    for t in ("spellpattern", "cmcatalog", "gm_audit"):
        if not table(db, t):
            return [], lines, [f"no {t} table here"], False
    for sid, (name, cm) in SPELLS.items():
        sp = rows(db, "SELECT spellid, spellname, abilitylvl, recast FROM spellpattern WHERE spellid = ?", (sid,))
        if len(sp) != 1 or sp[0]["spellname"] != name or sp[0]["abilitylvl"] != LEVEL or sp[0]["recast"] != RECAST:
            refusals.append(f"spellpattern {sid}: {sp}, not {name!r} L{LEVEL} recast {RECAST}")
        m = rows(db, "SELECT id, parent, add_spells, remove_spells FROM cmcatalog WHERE id = ?", (cm,))
        if len(m) != 1 or sid not in ids(m[0]["add_spells"]):
            refusals.append(f"cmcatalog {cm}: {m}, does not add {sid}")
    child = rows(db, "SELECT parent, remove_spells FROM cmcatalog WHERE id = ?", (CHILD,))
    if child and (child[0]["parent"] != PARENT or ids(child[0]["remove_spells"])):
        refusals.append(f"cmcatalog {CHILD}: {child[0]}, not a child of {PARENT} that removes nothing")
    out, now = [], 0
    for sid, (name, _) in SPELLS.items():
        r = get(db, sid)
        if r is None:
            out.append(sid)
            lines.append(f"spellrecastgroup {sid} {name!r}: none -> {GROUP!r}")
        elif r["grp"] == GROUP:
            now += 1
        else:
            refusals.append(f"spellrecastgroup {sid} {name!r}: in group {r['grp']!r} already, not {GROUP!r}")
    if table(db, "spellrecastgroup"):
        for r in rows(db, "SELECT spellid, grp FROM spellrecastgroup WHERE grp = ?", (GROUP,)):
            if r["spellid"] not in SPELLS:
                refusals.append(f"spellrecastgroup {r['spellid']}: in {GROUP!r}, a spell this tool does not know")
    else:
        lines.append("table spellrecastgroup made (none here yet)")
    return out, lines, refusals, not out and not refusals and now == len(SPELLS)


def apply(db, out):
    created = 0 if table(db, "spellrecastgroup") else 1
    db.execute(TABLE_SCHEMA)
    db.execute(LOG_SCHEMA)
    n = 0
    for sid in out:
        db.execute("INSERT INTO spellrecastgroup (spellid, grp) VALUES (?, ?)", (sid, GROUP))
        now = get(db, sid)
        a = audit(db, "insert", sid, None, now, DETAIL)
        db.execute(f"INSERT INTO {LOG} (spellid, after, created, audit) VALUES (?, ?, ?, ?)",
                   (sid, json.dumps(now, sort_keys=True), created, a))
        n += 1
    return n


def undo(db):
    """(back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    created = False
    for seq, sid, after, made in db.execute(
            f"SELECT seq, spellid, after, created FROM {LOG} ORDER BY seq DESC").fetchall():
        created = created or bool(made)
        now = get(db, sid)
        if now is None or json.dumps(now, sort_keys=True) != after:
            left += 1
            print(f"  left: spellrecastgroup {sid}, changed since it was written")
            continue
        db.execute("DELETE FROM spellrecastgroup WHERE spellid = ?", (sid,))
        audit(db, "undo", sid, now, None, "undo: " + DETAIL)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if left == 0:
        db.execute(f"DROP TABLE {LOG}")
        if created and table(db, "spellrecastgroup") and \
                db.execute("SELECT COUNT(*) FROM spellrecastgroup").fetchone()[0] == 0:
            db.execute("DROP TABLE spellrecastgroup")
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
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
            print(f"put back {back}, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        out, lines, refusals, applied = changes(db)
        for line in lines:
            print(line)
        if applied:
            print("applied already (the rows are as this tool writes them): nothing done")
            return 0
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not a.apply:
            print(f"report only: {len(out)} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            n = apply(db, out)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {n} rows, in gm_audit and {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
