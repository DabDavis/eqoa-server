#!/usr/bin/env python3
"""Dust's armour trim a BLACK aura (the owner, 2026-11-13: "lets also let armor trim glow also. and make dusts armor
trim glow a black aura").

Dust alone wears the Ornate Breastplate, Vambraces, Gauntlets, Greaves and Sabatons, itempattern 711000-711004 (set 9,
the ornate plate whose gold trim vanilla recolours). This gives each an itemaura row: kind 5 (AuraTrim: the trim lit, a
soft halo off it) in 0x000000FF, black, which vanilla draws as a DARK aura (database.AuraDark: every channel at most
0x30): the trim darkened toward black under a dark sheen, a smoky halo off it. Source and nodes 0 (no particles).
Gauntlets have no gold on set 9: their row is written all the same (the piece shows nothing of it), so a GM who changes
the kind to a shell later finds all five.

Checked as found, else REFUSED and nothing written: each id an itempattern row named "Ornate ..." of model 9. A row
there already (a GM's) is changed and its old values kept in the log. Report by default; --apply in one transaction,
a gm_audit line a row (as the panel's: insert or update, the whole row before and after), logged in
dust_trim_aura_1113_log; --undo puts back exactly what was there (a row this made deleted, a row it changed given its
values back, the itemaura table dropped when this made it and it is empty again), each a gm_audit line, a row changed
since left and listed. Run it on each shard.

The server reads itemaura at its start and on the GM panel's "Reload every NPC" (reload_npcs): after --apply (or
--undo) on a running server, press that; every wearer's channel carries it at the next tick. A client before trim-glow
reads kind 5 as the default (the shell while the colour lights: black lights nothing), so it draws nothing.

    python3 -E -s tools/dust_trim_aura_20261113.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "dust_trim_aura_1113_log"
DETAIL = "Dust's ornate plate's trim a black aura (the owner, 2026-11-13); tools/dust_trim_aura_20261113.py"
WHO, VIA = "dab", "tool"
IDS = (711000, 711001, 711002, 711003, 711004)
COLOR, KIND, SOURCE, NODES = 0x000000FF, 5, 0, 0
NOTE = "Dust's ornate plate: the trim a black (dark) aura, the owner 2026-11-13"

# as database/itemaura.go makes it
SCHEMA = """CREATE TABLE IF NOT EXISTS itemaura (
    patternid INTEGER PRIMARY KEY,
    color     INTEGER NOT NULL DEFAULT 0,
    kind      INTEGER NOT NULL DEFAULT 0,
    source    INTEGER NOT NULL DEFAULT 0,
    nodes     INTEGER NOT NULL DEFAULT 0,
    note      TEXT NOT NULL DEFAULT ''
)"""

LOG_DDL = f"""CREATE TABLE {LOG} (
    seq        INTEGER PRIMARY KEY AUTOINCREMENT,
    patternid  INTEGER NOT NULL,
    before     TEXT,              -- the row before (JSON), NULL none
    after      TEXT NOT NULL,     -- the row written (JSON)
    made_table INTEGER NOT NULL,
    audit      INTEGER NOT NULL
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def row(db, pid):
    if not table(db, "itemaura"):
        return None
    cur = db.execute("SELECT * FROM itemaura WHERE patternid = ?", (pid,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return None if m is None else json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, pid, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, 'itemaura', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, go_json({"patternid": pid}), go_json(before), go_json(after), DETAIL))
    return cur.lastrowid


def wanted(pid):
    """Item pid's row as this writes it."""
    return {"patternid": pid, "color": COLOR, "kind": KIND, "source": SOURCE, "nodes": NODES, "note": NOTE}


def plan(db):
    """(writes: [(patternid, before or None)], lines, refusals)."""
    refusals, lines, writes = [], [], []
    for t in ("itempattern", "gm_audit"):
        if not table(db, t):
            refusals.append(f"no {t} table here")
    if refusals:
        return writes, lines, refusals
    for pid in IDS:
        it = db.execute("SELECT itemname, model FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
        if it is None:
            refusals.append(f"no itempattern {pid}")
            continue
        name, model = it
        if not str(name).startswith("Ornate ") or model != 9:
            refusals.append(f"itempattern {pid} is {name!r} model {model}, not an Ornate piece of set 9")
            continue
        before = row(db, pid)
        want = wanted(pid)
        if before == want:
            lines.append(f"{pid} {name}: already the black trim aura")
            continue
        was = "none" if before is None else f"0x{before['color'] & 0xFFFFFFFF:08X} kind {before['kind']} (kept in the log)"
        lines.append(f"{pid} {name}: itemaura {was} -> 0x{COLOR:08X} kind {KIND} (trim, dark)")
        writes.append((pid, before))
    return writes, lines, refusals


def apply(db, writes):
    made = 0 if table(db, "itemaura") else 1
    db.execute(SCHEMA)
    db.execute(LOG_DDL)
    for pid, before in writes:
        if before is None:
            db.execute("INSERT INTO itemaura (patternid, color, kind, source, nodes, note) VALUES (?, ?, ?, ?, ?, ?)",
                       (pid, COLOR, KIND, SOURCE, NODES, NOTE))
            action = "insert"
        else:
            db.execute("UPDATE itemaura SET color = ?, kind = ?, source = ?, nodes = ?, note = ? WHERE patternid = ?",
                       (COLOR, KIND, SOURCE, NODES, NOTE, pid))
            action = "update"
        after = row(db, pid)
        a = audit(db, action, pid, before, after)
        db.execute(f"INSERT INTO {LOG} (patternid, before, after, made_table, audit) VALUES (?, ?, ?, ?, ?)",
                   (pid, go_json(before), go_json(after), made, a))
    return len(writes)


def undo(db):
    """(back, left: [why])."""
    if not table(db, LOG):
        return 0, []
    back, left, made = 0, [], False
    for pid, before, after, made_table in db.execute(f"SELECT patternid, before, after, made_table FROM {LOG} ORDER BY seq DESC").fetchall():
        made = made or bool(made_table)
        now = row(db, pid)
        if go_json(now) != after:
            left.append(f"itemaura {pid}: changed since, left as it is")
            continue
        before = json.loads(before) if before else None
        if before is None:
            db.execute("DELETE FROM itemaura WHERE patternid = ?", (pid,))
            audit(db, "delete", pid, now, None)
        else:
            db.execute("UPDATE itemaura SET color = ?, kind = ?, source = ?, nodes = ?, note = ? WHERE patternid = ?",
                       (before["color"], before["kind"], before["source"], before["nodes"], before["note"], pid))
            audit(db, "update", pid, now, row(db, pid))
        back += 1
    if not left:
        if made and db.execute("SELECT count(*) FROM itemaura").fetchone()[0] == 0:
            db.execute("DROP TABLE itemaura")
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"undone: {back} rows back" + "".join(f"\n  left: {w}" for w in left) +
                  ("; press the GM panel's Reload every NPC" if back else ""))
            return 0 if not left else 1
        if table(db, LOG):
            db.execute("ROLLBACK")
            print(f"applied already ({LOG} is there): --undo first")
            return 0
        writes, lines, refusals = plan(db)
        for line in lines:
            print(line)
        if refusals:
            db.execute("ROLLBACK")
            print("REFUSED, nothing written:\n  " + "\n  ".join(refusals))
            return 1
        if not writes:
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"(report only: {len(writes)} rows; --apply to write, then the GM panel's Reload every NPC)")
            return 0
        n = apply(db, writes)
        db.execute("COMMIT")
        print(f"applied: {n} rows, in gm_audit and {LOG}; press the GM panel's Reload every NPC (or restart)")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
