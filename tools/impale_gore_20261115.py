#!/usr/bin/env python3
"""Impale's Impaling Gore (spell 760163) from AC -2,500 to AC -1,500 (Heirloom petition 2189, Zerkziee, and the owner,
2026-11-15: "Impale is killing players in 1 hit").

Impale (the Isle of Dread, L55; class template "iod impale", melee_pct 928) casts Impaling Gore at what it fights every
30 s from 3 s in: "Target, Decrease AC by 2500, 1min", the guide's "0 AC debuff". Its melee_pct was fitted where AC 2-3k
puts every roll of a L55 creature under its floor (softHit's window, entity/defence.go: 175-199 a hit on Zerkziee, a L60
ranger of AC 2,520 and AGI 520, before the level's balance of 175%), and the Gore took him to AC 20, where the roll lands
over the floor: 246-1,578 a hit, a quad round up to 6,312, 11,046 after the balance, against his 7,960 hit points. A
player of less AC went under 0 (a wizard of 1,000 at -1,500: 5,668 a hit after the balance); since this branch a
creature's AC debuff takes a player to AC 0 at most (entity/classtemplate.go templateDebuffLocked), so that case is the
AC 0 one.

At AC -1,500 on Zerkziee: 248-520 a hit, mean 254, a quad round at most 2,080 (3,640 after the balance); without it
175-199, as before. A tank of AC 2,000 under it: at most 1,067 a hit before the balance. Hard, not a one-shot.
entity/impale_test.go holds the figures.

Report only by default; --apply in one transaction with a gm_audit line and the log impale_gore_1115_log (the row
before); a second --apply does nothing; --undo puts the line and source back exactly when the row is still this tool's
(else it says so and leaves it) and drops the log.

WHEN IT TAKES EFFECT. The templates' spells are read at the start or by the GM panel's reload_npcs (which reads the
class templates); a Gore already on a player runs out at its minute.

    python3 -E -s tools/impale_gore_20261115.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

SPELL = 760163
OLD_LINE = "Target, Decrease AC by 2500, 1min"
NEW_LINE = "Target, Decrease AC by 1500, 1min"
SOURCE_ADD = (" (AC -2,500 -> -1,500, 2026-11-15, petition 2189: at -2,500 a L60 tank reached AC 0 and Impale's 928% "
              "melee hit for up to 11,046 a round: tools/impale_gore_20261115.py)")
LOG = "impale_gore_1115_log"
WHO, VIA = "dab", "tool"
DETAIL = "Impaling Gore AC -2,500 -> -1,500 (petition 2189: Impale one-shot players): tools/impale_gore_20261115.py"


def has(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone() is not None


def row(db):
    r = db.execute("SELECT spellid, line, fxdict, source, effects FROM spelleffect WHERE spellid = ?", (SPELL,)).fetchone()
    return dict(zip(("spellid", "line", "fxdict", "source", "effects"), r)) if r else None


def audit(db, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', 'spelleffect', ?, ?, ?, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, json.dumps({"spellid": SPELL}),
                json.dumps(before, separators=(",", ":")), json.dumps(after, separators=(",", ":")), detail))


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        r = row(db)
        print(f"spelleffect {SPELL}: {r['line'] if r else 'NO ROW'!r}")
        users = [n for (n,) in db.execute("SELECT name FROM classtemplates WHERE abilities LIKE ?", (f"%{SPELL}%",))]
        print(f"  cast by the class templates: {', '.join(users) or 'none'}")
        if a.undo:
            if not has(db, LOG):
                print("nothing to undo")
                return 0
            old = json.loads(db.execute(f"SELECT before FROM {LOG}").fetchone()[0])
            db.execute("BEGIN IMMEDIATE")
            r = row(db)
            if r is None or r["line"] != NEW_LINE or r["source"] != old["source"] + SOURCE_ADD:
                print(f"  the row changed since this tool wrote it: left as it is ({r})")
            else:
                db.execute("UPDATE spelleffect SET line = ?, source = ? WHERE spellid = ?", (old["line"], old["source"], SPELL))
                audit(db, r, row(db), "undone: " + DETAIL)
                print(f"  put back: {old['line']!r}")
            db.execute(f"DROP TABLE {LOG}")
            db.execute("COMMIT")
            print("takes effect at the GM panel's reload_npcs or a restart")
            return 0
        if has(db, LOG):
            print(f"applied already ({LOG} is there): nothing to do (--undo to take it back)")
            return 0
        if r is None or r["line"] != OLD_LINE or r["effects"].strip():
            print(f"refused, nothing done: the row is not the one this tool was made for (line {OLD_LINE!r}, no effects JSON)")
            return 1
        print(f"  -> {NEW_LINE!r}")
        if not a.apply:
            print("report only: --apply to write it")
            return 0
        db.execute("BEGIN IMMEDIATE")
        db.execute(f"CREATE TABLE {LOG} (before TEXT NOT NULL)")
        db.execute(f"INSERT INTO {LOG} (before) VALUES (?)", (json.dumps(r, separators=(",", ":")),))
        db.execute("UPDATE spelleffect SET line = ?, source = source || ? WHERE spellid = ?", (NEW_LINE, SOURCE_ADD, SPELL))
        audit(db, r, row(db), DETAIL)
        db.execute("COMMIT")
        print("applied; takes effect at the GM panel's reload_npcs or a restart")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
