#!/usr/bin/env python3
"""Nine Lives, Barkskin and Deathless stack with every buff (the owner, 2026-11-11, after petition 1579's Living Stone:
"fix nine lives, barkskin and deathless too").

The three race masteries (Nine Lives 760123: 3000 health and 300 AC; Barkskin 760150: 300 AC and 150 HoT; Deathless
760152: 2500 health and 250 AC) were of the workbook's self stack type (2) with +AC, so each was one line ("stack2
player +AC") with every self AC buff its wearer could have, as Living Stone was (tools/living_stone_stack_20261111.py):
the higher rank took the other off or was refused over it.
  spelleffect <id>: effects' "stack" 2 -> <id>, a stack type of its own (as Living Stone's and GM Armor's), so its line
    is no other spell's: it stacks with every buff, and a recast replaces itself.
Refused, nothing written, when a row is not as expected. Report only by default; --apply in one transaction, a
gm_audit line a row, log race_mca_stack_1111_log; --undo exact. Read at every cast: no restart.
    tools/race_mca_stack_20261111.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "race_mca_stack_1111_log"
WHO, VIA = "dab", "tool"
DETAIL = ("Nine Lives, Barkskin, Deathless each a stack type of its own (the owner, after petition 1579): "
          "tools/race_mca_stack_20261111.py")
SPELLS = [(760123, "Nine Lives"), (760150, "Barkskin"), (760152, "Deathless")]
STACK_NOW = 2

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    keycol TEXT NOT NULL,
    keyval TEXT NOT NULL,
    col    TEXT NOT NULL,
    before TEXT NOT NULL,
    after  TEXT NOT NULL,
    audit  INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def compact(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False)


def audit(db, tbl, keycol, keyval, col, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', 'update', ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, tbl, compact({keycol: keyval}),
                      compact({col: before}), compact({col: after}), DETAIL))
    return cur.lastrowid


def plan_one(db, SPELL, NAME):
    """(change or None, lines, refusals)."""
    STACK_NEW = SPELL
    r = db.execute("SELECT effects FROM spelleffect WHERE spellid = ?", (SPELL,)).fetchone()
    name = db.execute("SELECT spellname FROM spellpattern WHERE spellid = ?", (SPELL,)).fetchone()
    if r is None or name is None or name[0] != NAME:
        return None, [], [f"spell {SPELL}: {name[0] if name else 'no spellpattern'!r}, effects row "
                          f"{'there' if r else 'missing'}: want {NAME} with its effects"]
    text = r[0]
    try:
        eff = json.loads(text)
    except ValueError:
        return None, [], [f"spelleffect {SPELL}: effects are not JSON: {text!r}"]
    if compact(eff) != text:
        return None, [], [f"spelleffect {SPELL}: effects not in their compact form (would not undo exactly)"]
    stack = eff.get("stack")
    if stack == STACK_NEW:
        return None, [f"spelleffect {SPELL} ({NAME}): stack {STACK_NEW} already"], []
    if stack != STACK_NOW:
        return None, [], [f"spelleffect {SPELL} ({NAME}): stack {stack!r}, want {STACK_NOW}"]
    eff["stack"] = STACK_NEW
    return (("spelleffect", "spellid", SPELL, "effects", text, compact(eff)),
            [f"spelleffect {SPELL} ({NAME}): stack {STACK_NOW} -> {STACK_NEW} (a line of its own)"], [])


def plan(db):
    changes, lines, refusals = [], [], []
    for sid, nm in SPELLS:
        c, l, r = plan_one(db, sid, nm)
        if c:
            changes.append(c)
        lines += l
        refusals += r
    return changes, lines, refusals


def write(db, tbl, keycol, keyval, col, before, after):
    cur = db.execute(f"UPDATE {tbl} SET {col} = ? WHERE {keycol} = ? AND {col} = ?", (after, keyval, before))
    return cur.rowcount == 1


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, tbl, keycol, keyval, col, before, after, _ in db.execute(f"SELECT * FROM {LOG} ORDER BY seq DESC").fetchall():
        if not write(db, tbl, keycol, int(keyval), col, after, before):
            print(f"left: {tbl} {keyval} changed since")
            left += 1
            continue
        audit(db, tbl, keycol, int(keyval), col, after, before)
        back += 1
    if not left:
        db.execute(f"DROP TABLE {LOG}")
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
            print(f"put back {back} rows, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        if table(db, LOG):
            print(f"applied already ({LOG} is there): --undo first")
            return 0
        changes, lines, refusals = plan(db)
        for line in lines:
            print(line)
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not changes:
            print("nothing to do")
            return 0
        if not a.apply:
            print(f"report only: {len(changes)} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            db.execute(LOG_SCHEMA)
            for tbl, keycol, keyval, col, before, after in changes:
                if not write(db, tbl, keycol, keyval, col, before, after):
                    raise RuntimeError(f"{tbl} {keyval} changed under the update")
                a_id = audit(db, tbl, keycol, keyval, col, before, after)
                db.execute(f"INSERT INTO {LOG} (tbl, keycol, keyval, col, before, after, audit) VALUES (?, ?, ?, ?, ?, ?, ?)",
                           (tbl, keycol, str(keyval), col, before, after, a_id))
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {len(changes)} rows, log {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
