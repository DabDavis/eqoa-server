#!/usr/bin/env python3
"""Petition 2217 (L60 ranger, Heirloom, 2026-11-16): "Flamelick forester mca is 0 cast and 6 recast post
cb". Flame Lick, the Forester Master Class's ability (cmcatalog 3513), is cast as its record has it: 2 s cast, 15 s
recast (the log: "begins "Flame Lick" (spell 701140) ... 2s cast"), from the import of EQOAEmu's workbook, whose
record is Allakhazam's 2004 one (2 s, 15 s, 487 power) with the Combat Balance's cost and hate (325 power, 1000 hate,
DEX). The player's claim, taken as the truth (the records we hold are silent: the CB notes' line for Flame Lick names
its damage, cost, stat mod and hate and no times; the workbook and Allakhazam keep the old ones): after the Combat
Balance it is an instant with a 6 s recast, as the CB made the other melee classes' masteries' hits (Manticore Tail,
Brilliant Strike: recast 6). This writes, on the one spellpattern record named Flame Lick:

  casttime 2 -> 0, recast 15 -> 6

each column only while it is still the import's (a GM's change since is left and said), with its gm_audit line and its
own line in flame_lick_2217_20261116_log. A shard without the record says so and writes nothing. It is idempotent.

WHEN IT TAKES EFFECT: a player's book is read at each login (the client's ability record, the server's cast), so a
ranger who holds it sees the change at the next login; no restart.

--dry (the default) prints what it would write; --apply writes it in one transaction; --undo puts back each column
still as this wrote it.

    tools/flame_lick_2217_20261116.py [--db ~/eqoa-server/eqoa.db] [--dry | --apply | --undo]
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

LOG = "flame_lick_2217_20261116_log"
WHO, VIA = "dab", "tool"
DETAIL = "petition 2217 (Mightyeagle): Flame Lick an instant with a 6 s recast after the Combat Balance, tools/flame_lick_2217_20261116.py"
DEFAULT_DB = os.path.expanduser("~/eqoa-server/eqoa.db")  # Heirloom's; Live's is another --db
NAME = "Flame Lick"
CHANGES = {"casttime": (2, 0), "recast": (15, 6)}  # column: (the import's, as it is to be)

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    spellid INTEGER NOT NULL,
    col     TEXT NOT NULL,
    before  INTEGER,
    after   INTEGER,
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def rows(db, q, args=()):
    cur = db.execute(q, args)
    cols = [d[0] for d in cur.description]
    return [dict(zip(cols, r)) for r in cur.fetchall()]


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def plan(db):
    """(step, lines, refusals): a step is (spellid, {col: (before, after)}), None for nothing to write."""
    recs = rows(db, "SELECT * FROM spellpattern WHERE lower(trim(spellname)) = ? ORDER BY spellid", (NAME.lower(),))
    if not recs:
        return None, [f"no spellpattern record named {NAME!r} here: nothing to do"], []
    if len(recs) != 1:
        return None, [], [f"{len(recs)} spellpattern records named {NAME!r}, want one"]
    rec = recs[0]
    change, lines = {}, []
    for col, (was, want) in CHANGES.items():
        if rec[col] == want:
            continue
        if rec[col] != was:
            lines.append(f"{NAME} ({rec['spellid']}): {col} is {rec[col]}, not the import's {was}: LEFT (a GM's?)")
            continue
        change[col] = (was, want)
    if not change:
        lines.append(f"{NAME} ({rec['spellid']}): nothing to change")
        return None, lines, []
    lines.append(f"{NAME} ({rec['spellid']}): " + ", ".join(f"{c} {b} -> {a}" for c, (b, a) in change.items()))
    return (rec["spellid"], change), lines, []


def audit(db, key, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', 'update', 'spellpattern', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, go_json(key), go_json(before), go_json(after), DETAIL))
    return cur.lastrowid


def apply(db, step):
    sid, change = step
    db.execute(LOG_SCHEMA)
    sets = ", ".join(f"{c} = ?" for c in change)
    db.execute(f"UPDATE spellpattern SET {sets} WHERE spellid = ?", [a for _, a in change.values()] + [sid])
    a = audit(db, {"spellid": sid}, {c: b for c, (b, _) in change.items()}, {c: x for c, (_, x) in change.items()})
    for c, (b, x) in change.items():
        db.execute(f"INSERT INTO {LOG} (spellid, col, before, after, audit) VALUES (?, ?, ?, ?, ?)", (sid, c, b, x, a))


def undo(db):
    if not table(db, LOG):
        return ["nothing logged: nothing to undo"]
    out, by_spell = [], {}
    for seq, sid, col, before, after in db.execute(f"SELECT seq, spellid, col, before, after FROM {LOG} ORDER BY seq DESC"):
        by_spell.setdefault(sid, []).append((seq, col, before, after))
    for sid, logged in by_spell.items():
        rec = rows(db, "SELECT * FROM spellpattern WHERE spellid = ?", (sid,))
        back, done = {}, []
        for seq, col, before, after in logged:
            if not rec:
                out.append(f"spell {sid}: gone, {col} not put back")
            elif rec[0][col] != after:
                out.append(f"spell {sid}: {col} is {rec[0][col]}, changed since: LEFT")
                continue
            else:
                back[col] = (after, before)
            done.append(seq)
        if back:
            sets = ", ".join(f"{c} = ?" for c in back)
            db.execute(f"UPDATE spellpattern SET {sets} WHERE spellid = ?", [b for _, b in back.values()] + [sid])
            audit(db, {"spellid": sid}, {c: a for c, (a, _) in back.items()}, {c: b for c, (_, b) in back.items()})
            out.append(f"spell {sid}: " + ", ".join(f"{c} {a} -> {b}" for c, (a, b) in back.items()))
        for seq in done:
            db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB, help=f"the server's database (default Heirloom's {DEFAULT_DB})")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--dry", action="store_true", help="print what it would write (the default)")
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    args = ap.parse_args()
    if not os.path.exists(args.db):
        sys.exit(f"no database at {args.db}")
    db = sqlite3.connect(args.db, timeout=5)
    if args.undo:
        with db:
            for line in undo(db):
                print(line)
        return
    step, lines, refusals = plan(db)
    for line in lines:
        print(line)
    if refusals:
        for r in refusals:
            print("REFUSED:", r)
        sys.exit(2)
    if step is None:
        print("nothing to write")
        return
    if not args.apply:
        print("--dry: 1 record would be changed; --apply writes it")
        return
    with db:
        apply(db, step)
    print(f"applied: spell {step[0]}, logged in {LOG}")


if __name__ == "__main__":
    main()
