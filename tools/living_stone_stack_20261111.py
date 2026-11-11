#!/usr/bin/env python3
"""Living Stone stacks with every buff (petition 1579, Tom, 2026-11-11: "Living stone MCA for gargoyle doesnt stack on
raid buffs").

Living Stone (spell 760156, the gargoyle's mastery: 2000 health at once and 400 AC for a minute) was of the workbook's
self stack type (2) with +AC, so it was one line ("stack2 player +AC") with every self AC buff a gargoyle could wear
(entity/buffline.go): a shadow knight's Hateweave and Ebonweave, a warrior's Steel, Mithril and Immortal Skin, Guard
and Immortal Guard, an alchemist's Petrifying Agent, a ranger's Spikecoat and Bladecoat; the stances (Iron, Steel,
Diamond, Melinis') and Nine Lives and Deathless above it. The higher rank took the other off or was refused over it,
so Living Stone took the knight's own AC buff off, and fell under a rank-60 one. A nested line (Hand of Innoruuk, +AC
+INT +STR) kept both but counted only the bigger AC. Group and single-target buffs (other stack types: Order, Pure
Heart, Mega's Clutch Ward, Avatar, the group AC lines) never clashed with it.

  spelleffect 760156 (Living Stone): effects' "stack" 2 -> 760156, a stack type of its own (as GM Armor's 905120), so
    its line ("stack760156 player +AC") is no other spell's: it stacks with every buff, and a recast replaces itself.

Refused, nothing written, when the row is not as expected (not Living Stone, not stack 2, effects not compact JSON).
Report only by default; --apply in one transaction, a gm_audit line, log living_stone_stack_1111_log; --undo exact (a
row changed since is left and said). The spell is read at every cast: no restart (a Living Stone already on keeps
its old line for the minute it lasts).

    tools/living_stone_stack_20261111.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "living_stone_stack_1111_log"
WHO, VIA = "dab", "tool"
DETAIL = ("Living Stone a stack type of its own (petition 1579, Tom: \"Living stone MCA for gargoyle doesnt stack on raid "
          "buffs\"): tools/living_stone_stack_20261111.py")
SPELL = 760156
NAME = "Living Stone"
STACK_NOW = 2
STACK_NEW = SPELL

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


def plan(db):
    """(change or None, lines, refusals)."""
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
        change, lines, refusals = plan(db)
        for line in lines:
            print(line)
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not change:
            print("nothing to do")
            return 0
        if not a.apply:
            print("report only: 1 row would be written (--apply writes it)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            db.execute(LOG_SCHEMA)
            tbl, keycol, keyval, col, before, after = change
            if not write(db, tbl, keycol, keyval, col, before, after):
                raise RuntimeError(f"{tbl} {keyval} changed under the update")
            a_id = audit(db, tbl, keycol, keyval, col, before, after)
            db.execute(f"INSERT INTO {LOG} (tbl, keycol, keyval, col, before, after, audit) VALUES (?, ?, ?, ?, ?, ?, ?)",
                       (tbl, keycol, str(keyval), col, before, after, a_id))
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: 1 row, log {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
