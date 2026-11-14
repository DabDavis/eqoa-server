#!/usr/bin/env python3
"""The magician's level 16 earth pet on par with the necromancer's (the owner, 2026-11-14: "earth elemental lvl 16 pet
for magicians is weaker than the necro level 16 pet by a lot").

A summoned pet's hit points are its petstats row's times its owner's class and level's balanceclasslevel.pet_hp_pct
(entity/balance.go PetHPPctAt; between two rows, a straight line). The magician's row at 16 was 63% (at 15: 284%, at
19: 126%), so the Earth Elementalkin (spell 657, level 16, 837 on its sheet, a tank) stood with 527 at 16 (the Live
log: Groceries' 527/527 at 05:29 UTC), 703 at 17 and 879 at 18 against the necromancer's Hulking Bones' 642, 719 and
1,001, and 737-870 at 20-23 against Wilting Flesh's 879-1,312; its weapon 135 against their 156 and 192 (and their
lifetaps). From 24 the magician's pets are ahead (Earth Elementalkin 21: 1,177 against Decaying Flesh's 822), and at
4-15 its best (the Fire and Air Elementalings) are within reach of the necromancer's: left as they are.

  balanceclasslevel class 10 (magician), levels 16-23: pet_hp_pct so that its earth pet has 15% over the
    necromancer's best pet at the same owner level (the tank's margin; necromancer as its rows give it):
      16: 63 -> 88, 17 (new) 99, 18 (new) 138, 19: 126 -> 176, 20: 88 -> 121, 21: 93.15 -> 141, 22: 103.075 -> 160,
      23: 103.96 -> 180. Every magician pet at those levels gets the same share (its water, fire and air pets of those
      levels were further behind the necromancer's still).
  petstats 657 (Earth Elementalkin): wpn1_dmg 135 -> 160, the necromancer's 156 at those levels, still under Wilting
    Flesh's 192 (a tank's, with its taunt).

The balance and petstats are read at the server's start: a restart (a keep restart's before.sh); a pet out at the
time keeps its numbers until summoned again. Refused, nothing written, when a row is not as above. Report only by
default; --apply in one transaction, a gm_audit line a row, log mage_earth_pet_1114_log; --undo exact.

    tools/mage_earth_pet_20261114.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "mage_earth_pet_1114_log"
DETAIL = "the owner (2026-11-14): the magician's level 16 earth pet on par with the necromancer's: tools/mage_earth_pet_20261114.py"
WHO, VIA = "claude", "tool"
SRC = "mageearth1114"
# level: (now, new); None now: a row added
PCT = {16: (63.0, 88.0), 17: (None, 99.0), 18: (None, 138.0), 19: (126.0, 176.0), 20: (88.0, 121.0),
       21: (93.15, 141.0), 22: (103.075, 160.0), 23: (103.96, 180.0)}
PET, WPN = 657, (135, 160)

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    rowkey TEXT NOT NULL,
    before TEXT,
    after  TEXT NOT NULL,
    audit  INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def compact(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False, sort_keys=True)


def audit(db, action, tbl, key, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, compact(key), compact(before or {}), compact(after or {}), DETAIL))
    return cur.lastrowid


def plan(db):
    changes, lines, refusals = [], [], []
    for lvl, (now, new) in sorted(PCT.items()):
        r = db.execute("SELECT pet_hp_pct FROM balanceclasslevel WHERE class = 10 AND level = ?", (lvl,)).fetchone()
        have = None if r is None else r[0]
        if have == new:
            lines.append(f"magician {lvl}: {new} already")
            continue
        if have != now:
            refusals.append(f"balanceclasslevel 10/{lvl}: pet_hp_pct {have}, want {now}")
            continue
        changes.append(("balanceclasslevel", {"class": 10, "level": lvl}, now, new))
        lines.append(f"magician {lvl}: pet_hp_pct {now if now is not None else '(no row)'} -> {new}")
    r = db.execute("SELECT name, wpn1_dmg FROM petstats WHERE spellid = ?", (PET,)).fetchone()
    if r is None or r[0] != "Earth Elementalkin":
        refusals.append(f"petstats {PET}: {r}")
    elif r[1] == WPN[1]:
        lines.append("Earth Elementalkin's weapon 160 already")
    elif r[1] != WPN[0]:
        refusals.append(f"petstats {PET} wpn1_dmg {r[1]}, want {WPN[0]}")
    else:
        changes.append(("petstats", {"spellid": PET}, WPN[0], WPN[1]))
        lines.append(f"Earth Elementalkin wpn1_dmg {WPN[0]} -> {WPN[1]}")
    return changes, lines, refusals


def apply(db, changes):
    db.execute(LOG_SCHEMA)
    for tbl, key, now, new in changes:
        if tbl == "petstats":
            if db.execute("UPDATE petstats SET wpn1_dmg = ? WHERE spellid = ? AND wpn1_dmg = ?", (new, key["spellid"], now)).rowcount != 1:
                raise RuntimeError("petstats changed under the update")
            a = audit(db, "update", tbl, key, {"wpn1_dmg": now}, {"wpn1_dmg": new})
        elif now is None:
            db.execute("INSERT INTO balanceclasslevel (class, level, pet_hp_pct, note, source) VALUES (?, ?, ?, ?, ?)",
                       (key["class"], key["level"], new, "the owner's earth pet on par (tools/mage_earth_pet_20261114.py)", SRC))
            a = audit(db, "insert", tbl, key, None, {"pet_hp_pct": new})
        else:
            if db.execute("UPDATE balanceclasslevel SET pet_hp_pct = ? WHERE class = ? AND level = ? AND pet_hp_pct = ?",
                          (new, key["class"], key["level"], now)).rowcount != 1:
                raise RuntimeError(f"balanceclasslevel {key} changed under the update")
            a = audit(db, "update", tbl, key, {"pet_hp_pct": now}, {"pet_hp_pct": new})
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                   (tbl, compact(key), json.dumps(now), json.dumps(new), a))


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, tbl, key, before, after in db.execute(f"SELECT seq, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key, before, after = json.loads(key), json.loads(before), json.loads(after)
        if tbl == "petstats":
            ok = db.execute("UPDATE petstats SET wpn1_dmg = ? WHERE spellid = ? AND wpn1_dmg = ?", (before, key["spellid"], after)).rowcount == 1
        elif before is None:
            ok = db.execute("DELETE FROM balanceclasslevel WHERE class = ? AND level = ? AND pet_hp_pct = ? AND source = ?",
                            (key["class"], key["level"], after, SRC)).rowcount == 1
        else:
            ok = db.execute("UPDATE balanceclasslevel SET pet_hp_pct = ? WHERE class = ? AND level = ? AND pet_hp_pct = ?",
                            (before, key["class"], key["level"], after)).rowcount == 1
        if not ok:
            print(f"  left: {tbl} {key} changed since")
            left += 1
            continue
        audit(db, "update" if before is not None else "delete", tbl, key, {"v": after}, {"v": before})
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
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
        db.execute("BEGIN IMMEDIATE")
        if a.undo:
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"put back {back}, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        if table(db, LOG):
            db.execute("ROLLBACK")
            print(f"applied already ({LOG} is there): --undo first")
            return 0
        changes, lines, refusals = plan(db)
        for line in lines:
            print(line)
        if refusals:
            db.execute("ROLLBACK")
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not changes:
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"report only: {len(changes)} rows would be written (--apply writes them)")
            return 0
        apply(db, changes)
        db.execute("COMMIT")
        print(f"applied: {len(changes)} rows, log {LOG}")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
