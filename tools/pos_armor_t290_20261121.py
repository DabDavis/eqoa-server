#!/usr/bin/env python3
"""The Plane of Sky armour quest's three figures that still differ from the forum post t290 ("PoS Armor"), set to it.

The owner, 2026-11-21: audit the Plane of Sky armour quests against https://eqoa.forumotion.com/t290-pos-armor. The
quests (101446-101469: Stark, Verna, Toi and Singe Goldencrest/Razorbeak/Burnfeathers, a talisman and two parts a
piece), every class's set, the common creatures' five sets, the "various" and class drops, the Quintessence's list and
the named rares already carry the post's figures (the 2026-11-07 build), but for three cells the 11-07 build read as
the post's slips and kept at their siblings' figures. The post's own figures, as the owner asked for them:

    bracers of the Traveler (BRD)   AC      151 -> 123   ("Bracers 42sta 9agi 100hp 35pow 20fr 123ac")
    Leggings of the Assasin (RGE)   POWMAX   35 ->  30   ("Legs 42dex 9agi 100hp 30pow 20pr 201ac")
    Boots of the Mystic     (SHA)   AC      101 ->  10   ("Boots 42sta 9wis 35hp 100pow 20lr 10ac")

Each may be a slip of the post's (the same pieces of the RAN/RGE/BRD and SHA chain sets carry 151 AC, 35 power and
101 AC); --skip NAME leaves one as it is. Nothing else is touched: drops and drop rates are the owner's call, and this
tool changes no loot table, pool or rate.

A record is found by its NAME (any case), the base record only: an imported copy (itemimported) or an Ascended one is
never matched, and follows its base at the server's next start (SyncImportedCopies). The tool refuses, doing nothing,
when a name finds no record or several, when its class mask is not the set's class, or when a cell holds neither the
figure it expects nor the post's.

Report only by default; --apply writes, a gm_audit line a record, the old figures kept in pos_armor_t290_20261121_log;
--undo puts back each cell still as this tool left it. A second --apply finds nothing to do.

WHEN IT TAKES EFFECT: an item record is read at each request and a client gets item records at its login: no
restart (an imported copy of one of these takes the new figure at the server's next start).

    tools/pos_armor_t290_20261121.py --db eqoa.db [--apply | --undo] [--skip NAME ...]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "pos_armor_t290_20261121_log"
DETAIL = "the Plane of Sky armour against the forum post t290 (the owner, 2026-11-21): tools/pos_armor_t290_20261121.py"
ACCOUNT, WHO, VIA = 1, "dab", "tool"
BRD, RGE, SHA = 5, 6, 8

# (record's name, its class, column, the figure it holds, the post's figure)
CHANGES = [
    ("bracers of the Traveler", BRD, "AC", 151, 123),
    ("Leggings of the Assasin", RGE, "POWMAX", 35, 30),
    ("Boots of the Mystic", SHA, "AC", 101, 10),
]
COLUMNS = {"AC", "POWMAX"}


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def audit(db, pid, before, after):
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', 'itempattern', ?, ?, ?, ?)",
               (now(), ACCOUNT, WHO, VIA, go_json({"patternid": pid}), go_json(before), go_json(after), DETAIL))


def has_table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def find(db, name):
    """The base records of the name: never an imported copy or an Ascended one."""
    copies = set()
    if has_table(db, "itemimported"):
        copies = {r[0] for r in db.execute("SELECT patternid FROM itemimported")}
    return [r for r in db.execute("SELECT patternid, classuse FROM itempattern WHERE lower(itemname) = lower(?) "
                                  "ORDER BY patternid", (name,)) if r[0] not in copies]


def plan(db, skip):
    """[(pid, name, column, old, new)] to change, [lines] already done; exits on anything unexpected."""
    todo, done, bad = [], [], []
    skip = {s.lower() for s in skip}
    unknown = skip - {c[0].lower() for c in CHANGES}
    if unknown:
        sys.exit(f"--skip names no change of this tool: {sorted(unknown)}")
    for name, cls, col, old, new in CHANGES:
        if name.lower() in skip:
            continue
        assert col in COLUMNS
        rows = find(db, name)
        if len(rows) != 1:
            bad.append(f"{name}: {len(rows)} base records of the name")
            continue
        pid, classuse = rows[0]
        if classuse != 1 << cls:
            bad.append(f"{name} ({pid}): class mask {classuse}, not {1 << cls}")
            continue
        have = db.execute(f"SELECT {col} FROM itempattern WHERE patternid = ?", (pid,)).fetchone()[0]
        if have == new:
            done.append(f"{name} ({pid}): {col} already {new}")
        elif have == old:
            todo.append((pid, name, col, old, new))
        else:
            bad.append(f"{name} ({pid}): {col} is {have}, neither {old} nor the post's {new}")
    if bad:
        sys.exit("refused, nothing done:\n  " + "\n  ".join(bad))
    return todo, done


def ensure_log(db):
    db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (patternid INTEGER NOT NULL, itemname TEXT NOT NULL, col TEXT NOT NULL, "
               "old INTEGER NOT NULL, new INTEGER NOT NULL, at TEXT NOT NULL, PRIMARY KEY (patternid, col))")


def apply(db, todo):
    ensure_log(db)
    at = now()
    for pid, name, col, old, new in todo:
        db.execute(f"UPDATE itempattern SET {col} = ? WHERE patternid = ?", (new, pid))
        db.execute(f"INSERT OR REPLACE INTO {LOG} (patternid, itemname, col, old, new, at) VALUES (?, ?, ?, ?, ?, ?)",
                   (pid, name, col, old, new, at))
        audit(db, pid, {col: old}, {col: new})
    return len(todo)


def undo(db):
    if not has_table(db, LOG):
        return 0, 0
    back, left = 0, 0
    for pid, name, col, old, new in db.execute(f"SELECT patternid, itemname, col, old, new FROM {LOG}").fetchall():
        if col not in COLUMNS:
            sys.exit(f"{LOG}: unknown column {col!r}")
        row = db.execute(f"SELECT {col} FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
        if row is None or row[0] != new:
            left += 1
            print(f"  {name} ({pid}) {col}: changed since ({row[0] if row else 'gone'}), left")
            continue
        db.execute(f"UPDATE itempattern SET {col} = ? WHERE patternid = ?", (old, pid))
        audit(db, pid, {col: new}, {col: old})
        db.execute(f"DELETE FROM {LOG} WHERE patternid = ? AND col = ?", (pid, col))
        back += 1
    return back, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--skip", action="append", default=[], metavar="NAME", help="leave this record as it is")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, timeout=5)
    try:
        if a.undo:
            back, left = undo(db)
            db.commit()
            print(f"undo: {back} figures put back, {left} left as changed since")
            return 0
        todo, done = plan(db, a.skip)
        for line in done:
            print(f"  {line}")
        for pid, name, col, old, new in todo:
            print(f"  {name} ({pid}): {col} {old} -> {new}")
        print(f"{len(todo)} figures to set to the post's, {len(done)} already")
        if a.apply:
            n = apply(db, todo)
            db.commit()
            print(f"applied: {n} figures")
        else:
            print("(a report: --apply writes it)")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
