#!/usr/bin/env python3
"""Sunder takes a melee weapon in hand, as Cleave does (work board #1775, jibrish's era sources, the owner 2026-11-21:
"lets do it"; SOE's Game Update Notes 10-31-07: "Paladin Cleave and Sunder should now only work with any melee type
weapon equipped. No shields or ranged."). It undoes build252's choice (petition 2115: Sunder took any weapon).

  spelleffect 700700 (Sunder): effects.req_equipment "Any" -> "1HS,2HS,1HB,2HB,1HP,2HP" (Cleave 700534's), nothing
    else in the JSON; the line names no equipment. entity/workbookmore.go equipFits refuses a shield, a ranged weapon
    or empty hands for those codes. Read at every cast (handlers/cast.go): no restart.

Refused, nothing written, unless spellpattern 700700 is "Sunder" and its effects' req_equipment is "Any" (already
the melee codes: applied, nothing done). Report only by default; --apply in one transaction, a gm_audit line, log
sunder_melee_1121_log; --undo exact (a row changed since is left and said). Run on each shard's database.

    tools/sunder_melee_20261121.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "sunder_melee_1121_log"
DETAIL = "Sunder takes a melee weapon, as Cleave (work board #1775): tools/sunder_melee_20261121.py"
WHO, VIA = "dab", "tool"
SPELL, NAME = 700700, "Sunder"
FROM, TO = "Any", "1HS,2HS,1HB,2HB,1HP,2HP"
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    spellid INTEGER PRIMARY KEY,
    before  TEXT NOT NULL,      -- effects as they were
    after   TEXT NOT NULL,      -- effects as written
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def effects_of(db):
    r = db.execute("SELECT effects FROM spelleffect WHERE spellid = ?", (SPELL,)).fetchone()
    return None if r is None else r[0]


def audit(db, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', 'update', 'spelleffect', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, json.dumps({"spellid": SPELL}, separators=(",", ":")),
                      json.dumps({"effects": before}), json.dumps({"effects": after}), detail))
    return cur.lastrowid


def plan(db):
    """(new effects or None, refusals, applied)."""
    if not table(db, "spelleffect") or not table(db, "spellpattern"):
        return None, ["no spelleffect or spellpattern table here"], False
    sp = db.execute("SELECT spellname FROM spellpattern WHERE spellid = ?", (SPELL,)).fetchone()
    if sp is None or sp[0] != NAME:
        return None, [f"spellpattern {SPELL} is {sp!r}, not {NAME!r}"], False
    eff = effects_of(db)
    if not eff:
        return None, [f"spelleffect {SPELL} has no effects"], False
    d = json.loads(eff)
    req = d.get("req_equipment")
    if req == TO:
        return None, [], True
    if req != FROM:
        return None, [f"spelleffect {SPELL}'s req_equipment is {req!r}, not {FROM!r}: changed since, left to a person"], False
    d["req_equipment"] = TO
    return json.dumps(d, separators=(",", ":")), [], False


def undo(db):
    """(back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for spellid, before, after in db.execute(f"SELECT spellid, before, after FROM {LOG}").fetchall():
        now = effects_of(db)
        if now != after:
            left += 1
            print(f"  left: spelleffect {spellid}, changed since it was written: {now}")
            continue
        db.execute("UPDATE spelleffect SET effects = ? WHERE spellid = ?", (before, spellid))
        audit(db, after, before, "undo: " + DETAIL)
        db.execute(f"DELETE FROM {LOG} WHERE spellid = ?", (spellid,))
        back += 1
    if left == 0:
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
            print(f"put back {back}, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        new, refusals, applied = plan(db)
        if applied:
            print(f"applied already (spelleffect {SPELL}'s req_equipment is {TO}): nothing done")
            return 0
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        print(f"spelleffect {SPELL} ({NAME}): req_equipment {FROM} -> {TO}")
        if not a.apply:
            print("report only: 1 row would be written (--apply writes it)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            before = effects_of(db)
            db.execute(LOG_SCHEMA)
            db.execute("UPDATE spelleffect SET effects = ? WHERE spellid = ?", (new, SPELL))
            aid = audit(db, before, new, DETAIL)
            db.execute(f"INSERT INTO {LOG} (spellid, before, after, audit) VALUES (?, ?, ?, ?)", (SPELL, before, new, aid))
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: 1 row, in gm_audit and {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
