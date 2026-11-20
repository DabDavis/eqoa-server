#!/usr/bin/env python3
"""Form of the Predator as the dire winter wolf, in the byte order a spell's sprite takes (Heirloom petition 2301:
"Highest level Wolf form was a White wolf ... the dire winter wolf (Lyria Whitepaw), DictID 0xF23F517F", then "It
appears the form is just applying stats now, no wolf").

A GM's panel edit of 2026-11-20 16:56 gave the druid's Form of the Predator (700783) the dire winter wolf as its
sprite "0xF23F517F", the DictID as npcs.modelid holds it; a spell's sprite is written byte-reversed (spelljson.go
spriteBody), so the server looked for a body 0x7F513FF2, which no CSprite is, and the form landed as its stats alone
("the disc has no body 0x7f513ff2: no illusion"). The sprite becomes 0x7F513FF2, which reads as 0xF23F517F, Lyria
Whitepaw's body: the GM's choice, read as meant.

Heirloom (the GM's edit was Heirloom's). Refused unless the effects hold the GM's sprite once (or the new: done).
Report only by default; --apply a gm_audit line, logged in predator_2301_log; --undo exact.

WHEN IT TAKES EFFECT: a spell's effects are read at each cast.

    tools/predator_look_2301_20261120.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "predator_2301_log"
DETAIL = "petition 2301: Form of the Predator's dire winter wolf in the byte order a spell's sprite takes: tools/predator_look_2301_20261120.py"
WHO, VIA = "claude", "tool"
SPELL, NAME = 700783, "Form of the Predator"
OLD, NEW = '"sprite":"0xF23F517F"', '"sprite":"0x7F513FF2"'


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def audit(db, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', 'spelleffect', ?, ?, ?, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, go_json({"spellid": SPELL}), go_json(before), go_json(after), detail))


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("BEGIN IMMEDIATE")
    try:
        name = db.execute("SELECT spellname FROM spellpattern WHERE spellid = ?", (SPELL,)).fetchone()
        r = db.execute("SELECT effects FROM spelleffect WHERE spellid = ?", (SPELL,)).fetchone()
        if a.undo:
            if not table(db, LOG):
                db.execute("ROLLBACK")
                print("nothing to undo")
                return 0
            before, after = db.execute(f"SELECT before, after FROM {LOG}").fetchone()
            if r is None or r[0] != after:
                db.execute("ROLLBACK")
                print("REFUSED: the spell's effects changed since")
                return 1
            db.execute("UPDATE spelleffect SET effects = ? WHERE spellid = ?", (before, SPELL))
            audit(db, {"effects": after}, {"effects": before}, "undo: " + DETAIL)
            db.execute(f"DROP TABLE {LOG}")
            db.execute("COMMIT")
            print("put back")
            return 0
        if name is None or name[0] != NAME or r is None:
            db.execute("ROLLBACK")
            print(f"REFUSED: spell {SPELL} is {name and name[0]!r}, not {NAME!r}, or has no effects")
            return 1
        if r[0].count(NEW) == 1 and OLD not in r[0]:
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if r[0].count(OLD) != 1:
            db.execute("ROLLBACK")
            print(f"REFUSED: {NAME}'s effects do not hold {OLD} once")
            return 1
        print(f"{NAME}: {OLD} -> {NEW}")
        if not a.apply:
            db.execute("ROLLBACK")
            print("report only (--apply to write)")
            return 0
        new = r[0].replace(OLD, NEW)
        db.execute("UPDATE spelleffect SET effects = ? WHERE spellid = ?", (new, SPELL))
        db.execute(f"CREATE TABLE {LOG} (before TEXT NOT NULL, after TEXT NOT NULL)")
        db.execute(f"INSERT INTO {LOG} VALUES (?, ?)", (r[0], new))
        audit(db, {"effects": r[0]}, {"effects": new}, DETAIL)
        db.execute("COMMIT")
        print("applied")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
