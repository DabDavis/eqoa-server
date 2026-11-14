#!/usr/bin/env python3
"""Buff lines across every class (the owner, 2026-11-14: "check the lines across every class"), both shards.

A workbook spell's line is its stack type, its bearer and its stats (internal/entity/buffline.go): two buffs of one
stack type that share a stat are one line, and only the better counts. The workbook's stack type is only the scope's
class (1 a single target, 2 self, 3 a group), so every class's single-target (or group) buffs of a stat were one line
with every other class's: a cleric's Courage refused under a druid's Natureskin (AC), a shaman's Finesse took an
alchemist's Clever Thought off (AGI), a warrior's Iron Will, a paladin's chants and a bard's Dirges one line with the
cleric's and druid's group buffs (AC), the bard's Anthems with the shaman's Tribal buffs (STR, STA), the alchemist's
group buffs with the shaman's and the bard's. The live game let different classes' buff families stack: SOE's notes
of 2006 (questmasters, the class pages' "Buff Stacking"): "Cleric HP/AC buffs (target, and CM group/AoE upgrades) now
stack with other self, targeted, and form buffs", "Druid HP/AC/Power buffs are set to stack with other self,
group/AoE, and form buffs", "Shaman DEX/AGI/STR/STA buffs are set to stack with other self, group/AoE, and form
buffs", "Alchemist INT/AGI and DEX/WIS buffs are all set to stack with other self, targeted, and form buffs".

  spelleffect <id>: effects' "stack" 1/2/3 -> the family's own number (its lowest spell id), for every family of
    stat buffs (STR STA AGI DEX WIS INT CHA AC max HP/power) one class's that shared a stat with another class's
    (tools/buff_lines_20261114.json, from internal/entity/bufflines_audit_test.go's dump of every spell's line as
    the engine reads it; a family = the class's spells of one stack type linked by the line rule, so a class's own
    lines are as they were: Courage to Order, Group Courage to United Faith, the Dirges, each still one line).
    Families of one effect across classes keep sharing: resists ("+65 AR/PR/DR doesn't stack w/ other resists
    buffs"), heals and power over time ("doesn't stack with other ticks"), damage shields ("the racial damage shield
    will not stack with another 'spell' damage shield"), procs, speed and invisibility.
  Bladecoat (700707, the ranger's self coat) and Bladecoat + (901177, its single-target CM form) had each other's
    stack types (2 and 1): swapped, so that Bladecoat + is no longer one line with every class's self AC buff.
  Concentration (900127) and Inspiration (700719), the alchemist's level 60 CMs, a line each apart from the group
    thinking and Unified Intent CM lines (one stat shared each: the one refused the other).
  The bard's Concerto line (Concerto, Rhapsody, Harmony and their Perfect forms) -> 171, Symphony's (given 171 on
    2026-11-14 so that it no longer blocked Power Dance): one line apart from the Power songs, as Symphony is.

A spell the shard lacks is skipped (said): Ascended Jakai's Rallying Cry is 906759 on Heirloom, 906640 on Live.
Refused, nothing written, when a row is not as expected. Report only by default; --apply in one transaction, a gm_audit
line a row, log buff_lines_1114_log; --undo exact (a row changed since is left and said). spelleffect is read at
every cast: no restart (an effect already on a player keeps the line it landed with until it ends).

    python3 -E -s tools/buff_lines_20261114.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

LOG = "buff_lines_1114_log"
WHO, VIA = "dab", "tool"
DETAIL = "Buff lines across every class: each class's buff family a stack of its own (the owner): tools/buff_lines_20261114.py"
PLAN = os.path.join(os.path.dirname(os.path.abspath(__file__)), "buff_lines_20261114.json")

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
    """(changes, lines, refusals)."""
    changes, lines, refusals = [], [], []
    rows = json.load(open(PLAN, encoding="utf-8"))
    skipped = []
    for r in rows:
        sid, name, old, new = r["spellid"], r["name"], r["from"], r["to"]
        got = db.execute("SELECT p.spellname, e.effects FROM spellpattern p JOIN spelleffect e USING (spellid) "
                         "WHERE p.spellid = ?", (sid,)).fetchone()
        if got is None:
            skipped.append(f"{sid} {name}")
            continue
        if got[0] != name:
            refusals.append(f"spell {sid}: named {got[0]!r}, want {name!r}")
            continue
        text = got[1]
        try:
            eff = json.loads(text)
        except ValueError:
            refusals.append(f"spelleffect {sid} ({name}): effects are not JSON")
            continue
        if compact(eff) != text:
            refusals.append(f"spelleffect {sid} ({name}): effects not in their compact form (would not undo exactly)")
            continue
        stack = eff.get("stack")
        if stack == new:
            lines.append(f"  {r['classes']:<4} {sid} {name}: stack {new} already")
            continue
        if stack != old:
            refusals.append(f"spelleffect {sid} ({name}): stack {stack!r}, want {old}")
            continue
        eff["stack"] = new
        changes.append(("spelleffect", "spellid", sid, "effects", text, compact(eff)))
        lines.append(f"  {r['classes']:<4} {sid} {name}: stack {old} -> {new}")
    if skipped:
        lines.append(f"  not on this shard, skipped: {', '.join(skipped)}")
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
