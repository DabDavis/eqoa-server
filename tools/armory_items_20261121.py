#!/usr/bin/env python3
"""Item records' figures to the forum armory guides (the owner, 2026-11-21: check our item records against the
eqoa.forumotion.com "The Armory" section, its item and loot table threads).

Each of the section's threads was read and every item with figures taken out of it (name, slot, level, classes, AC,
HP, power, the seven stats, the six resists, HoT/PoT, DMG, range, condition and DUR). Our records were matched to
them by name on both shards; where an EXISTING record's figure differs from the guide's, the guide's is the one
written here (armory_items_20261121_plan.json, by our records' names). Only the figures the guide states are
touched: a figure the guide leaves out is left as it is, never zeroed, save on a SCRAMBLED record (the plan's
"scrambled": 67 names, ids 2097-2112, 4091-4140, 4257-4308, 4527-4536 and a few more, mostly robes): its figures
were written into str, sta, agi, dex, ... in the order the source listed them, whatever they were (Omnipotent Robes'
INT 20 POW 20 POT 15 AC 20 stood as STR 20 STA 20 AGI 15 DEX 20), so there the guide's full stat line is written and
the misplaced columns it does not list are set to 0. No record is made, no class or race mask is
touched (the guides word them loosely: "tank/melee (no mnk)"), and no loot or drop table is touched (the owner
decides drops).

A guide figure that looks like the guide's own slip (far off its set-mates or tier, ten times or a tenth of ours,
the sign flipped, a stat ours holds under the next stat's name, or two of the guide's threads disagreeing) is marked
suspect in the plan and left out unless --with-suspect. A figure an earlier tool or GM edit set on purpose (a
gm_audit line for that record and column: the owner's epic tables, petitions, GM panel edits) is marked prior and
left out unless --with-prior.

Each record is matched by name, any case, spaces round it; an imported copy (itemimported) is left out, as it is made
from its base again at the server's start. A record is changed only if every figure to change is still one of the
values read off the shards when the plan was made (or already the guide's): a record changed since is refused whole,
and named.

Report only by default; --apply writes, a gm_audit line a record, each old figure kept in armory_items_1121_log;
--undo puts back each figure still as this tool left it. A second --apply finds nothing to do. --skip NAME (again
for more) leaves a record of that name alone.

WHEN IT TAKES EFFECT: item records are read at each request: a merchant's window at once, a player's own items at
their next login; imported copies at the server's next start.

    tools/armory_items_20261121.py --db eqoa.db [--apply | --undo] [--with-suspect] [--with-prior] [--skip NAME ...]
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

LOG = "armory_items_1121_log"
PLAN = os.path.join(os.path.dirname(os.path.abspath(__file__)), "armory_items_20261121_plan.json")
DETAIL = "item figures to the forum armory guides: tools/armory_items_20261121.py"
ACCOUNT, WHO, VIA = 1, "dab", "tool"
# The itempattern columns the tool may write (each a figure a guide states).
COLUMNS = ("str", "sta", "agi", "dex", "wis", "intelligence", "cha", "HPMAX", "POWMAX", "HoT", "PoT", "AC",
           "PR", "DR", "FR", "CR", "LR", "AR", "weapondamage", "unk5", "levelreq", "maxhp", "duration")


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def audit(db, pid, before, after):
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', 'itempattern', ?, ?, ?, ?)",
               (now(), ACCOUNT, WHO, VIA, go_json({"patternid": pid}), go_json(before), go_json(after), DETAIL))


def load_plan(path=PLAN):
    with open(path) as f:
        return json.load(f)["entries"]


def key(name):
    return " ".join((name or "").lower().split())


def has_table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def records(db, name):
    """The records of a name (any case, spaces round it), imported copies left out."""
    copies = set()
    if has_table(db, "itemimported"):
        copies = {r[0] for r in db.execute("SELECT patternid FROM itemimported")}
    cols = ", ".join(COLUMNS)
    out = []
    for row in db.execute(f"SELECT patternid, itemname, {cols} FROM itempattern WHERE lower(trim(itemname)) = ? "
                          "ORDER BY patternid", (key(name),)):
        if row[0] in copies:
            continue
        out.append((row[0], row[1], dict(zip(COLUMNS, row[2:]))))
    return out


def changes(db, entries, with_suspect=False, skip=(), with_prior=False):
    """(patternid, name, {col: (old, new)}) to write, and (patternid, name, why) refused."""
    skip = {key(s) for s in skip}
    todo, refused = [], []
    for e in entries:
        if key(e["name"]) in skip:
            continue
        want = {c: f for c, f in e["fields"].items() if c in COLUMNS and (with_suspect or not f.get("suspect"))
                and (with_prior or not f.get("prior"))}
        if not want:
            continue
        for pid, name, row in records(db, e["name"]):
            ch, bad = {}, []
            for c, f in want.items():
                cur = row[c] if row[c] is not None else 0
                if cur == f["guide"]:
                    continue
                if cur not in f["read"]:
                    bad.append(f"{c} is {cur}, read {f['read']}")
                    continue
                ch[c] = (row[c], f["guide"])
            if bad:
                refused.append((pid, name, "; ".join(bad)))
            elif ch:
                todo.append((pid, name, ch))
    return todo, refused


def apply(db, todo):
    db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (patternid INTEGER NOT NULL, col TEXT NOT NULL, old INTEGER, "
               "new INTEGER NOT NULL, at TEXT NOT NULL, PRIMARY KEY (patternid, col))")
    at = now()
    for pid, _, ch in todo:
        for c, (old, new) in ch.items():
            db.execute(f"UPDATE itempattern SET {c} = ? WHERE patternid = ?", (new, pid))
            db.execute(f"INSERT OR REPLACE INTO {LOG} (patternid, col, old, new, at) VALUES (?, ?, ?, ?, ?)",
                       (pid, c, old, new, at))
        audit(db, pid, {c: o for c, (o, _) in ch.items()}, {c: n for c, (_, n) in ch.items()})
    return len(todo)


def undo(db):
    if not has_table(db, LOG):
        return 0, 0
    back, left = {}, 0
    for pid, c, old, new in db.execute(f"SELECT patternid, col, old, new FROM {LOG} ORDER BY patternid, col").fetchall():
        if c not in COLUMNS:
            continue
        row = db.execute(f"SELECT {c} FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
        if row is None or row[0] != new:
            left += 1
            print(f"  item {pid} {c}: changed since ({row[0] if row else 'gone'!r}), left")
            continue
        db.execute(f"UPDATE itempattern SET {c} = ? WHERE patternid = ?", (old, pid))
        db.execute(f"DELETE FROM {LOG} WHERE patternid = ? AND col = ?", (pid, c))
        b = back.setdefault(pid, ({}, {}))
        b[0][c], b[1][c] = new, old
    for pid, (before, after) in back.items():
        audit(db, pid, before, after)
    return len(back), left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--plan", default=PLAN)
    ap.add_argument("--with-suspect", action="store_true", help="also the guide figures marked suspect")
    ap.add_argument("--with-prior", action="store_true", help="also the figures an earlier tool or GM edit set on purpose")
    ap.add_argument("--skip", action="append", default=[], metavar="NAME", help="leave the records of this name alone")
    ap.add_argument("-v", "--verbose", action="store_true", help="list every change")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, timeout=5)
    try:
        if a.undo:
            back, left = undo(db)
            db.commit()
            print(f"undo: {back} records put back, {left} figures left as changed since")
            return 0
        todo, refused = changes(db, load_plan(a.plan), a.with_suspect, a.skip, a.with_prior)
        nfig = sum(len(ch) for _, _, ch in todo)
        print(f"{len(todo)} records, {nfig} figures to the guides' ({'suspect figures included' if a.with_suspect else 'suspect figures left out'})")
        for pid, name, ch in todo if a.verbose else todo[:15]:
            print(f"  {pid} {name}: " + ", ".join(f"{c} {o}->{n}" for c, (o, n) in ch.items()))
        if not a.verbose and len(todo) > 15:
            print(f"  ... {len(todo) - 15} more (-v lists them)")
        for pid, name, why in refused:
            print(f"  REFUSED {pid} {name}: not as read: {why}")
        if a.apply:
            n = apply(db, todo)
            db.commit()
            print(f"applied: {n} records")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
