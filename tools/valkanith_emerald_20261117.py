#!/usr/bin/env python3
"""The Lord Valkanith quest's emerald from any creature of the city of Takish'Hiz, at 10% (the owner, 2026-11-17: "make
sure tal thex emerald 10% chance to drop in this city only where [a GM] is for lord valkanith"; then, a GM moved to the
star knights: "this area should still drop the emerald also and also where he was").

WHAT DIFFERED (quest 101269 step 1, loot of the Tal'Thex Emerald 701383): the star knights alone dropped it, at 25%.
Now the step's target is "*" (database.AnyCreature, branch valkanith-emerald): a creature of any name killed within its
area drops it for a player on the step at 10%, none outside. The star knights are inside it, so they drop it at 10% too.

THE AREA, world 0, a circle of radius 550 round (19350, 27240), from the city's own geometry: worlds/ground/0.walls's
zone 146 (the client's "Takish Hiz" tile, x 18032-19972, z 26017-27976) holds the city's 28 large placed structures
(each over 60 across) within x 18998-19702, z 26820-27658, but one: the circle is centred on that box and reaches its
farthest corner (547). The tile itself is 2,000 across and most of it open desert, so the tile is not the area. A GM's
first place (19650, 26945) is 421 from the centre and his second, the star knights (19508, 27521), 323; the star
knights' camp (19498-19518, 27587-27670) is within 460. Left out: the one outlying structure (18751-18825, 27369-27484,
about 570 off), Deathknight Endraval (18748, 26830), the Truearrow camp in the desert south (19814, 26192) and the open
desert round the city; the desert's roamers (chaos hounds, scarabs, scorpions, drakes) count only when they die inside.

The ruby (step 2, spectral rallosians, Geomancer's Citadel) and the diamond (step 3, Teir'Dal knights, Sycamore Joy's
Rest) are left as they are, at 25% off their named creatures.

Report only by default; --apply in one transaction with gm_audit lines and the log valkanith_emerald_1117_log; a second
--apply does nothing; --undo puts back exactly what it wrote and drops the log. Refused whole if the step is not as this
tool expects. It needs the server of branch valkanith-emerald (an older server reads "*" as a creature's name, and the
step would drop nothing).

WHEN IT TAKES EFFECT: the quests are read at a start: run it in a keep restart's before.sh, the server stopped, with the
server that has AnyCreature.

    python3 -E -s tools/valkanith_emerald_20261117.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "valkanith_emerald_1117_log"
WHO, VIA = "dab", "tool"
DETAIL = "the Tal'Thex emerald from any creature of Takish'Hiz at 10% (the owner, 2026-11-17): tools/valkanith_emerald_20261117.py"

QUEST, STEP = 101269, 1
EMERALD = 701383
OLD = {"kind": "loot", "target": "star knight", "items": str(EMERALD), "chance": 0.25, "world": 0, "x": 0.0, "z": 0.0,
       "radius": 15.0, "journal": "Take the Tal'Thex emerald from the creatures inside the city of Takish'Hiz."}
NEW = {"target": "*", "chance": 0.10, "world": 0, "x": 19350.0, "z": 27240.0, "radius": 550.0,
       "journal": "Take the Tal'Thex emerald from the creatures inside the city of Takish'Hiz: any of them may carry it."}


def has(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone() is not None


def row(db):
    cur = db.execute("SELECT * FROM queststeps WHERE questid = ? AND step = ?", (QUEST, STEP))
    r = cur.fetchone()
    return dict(zip([d[0] for d in cur.description], r)) if r else None


def js(v):
    return json.dumps(v, separators=(",", ":")) if v is not None else None


def check(db):
    r = row(db)
    if r is None:
        return [f"no queststeps {QUEST} step {STEP}"]
    bad = [f"queststeps {QUEST} step {STEP} {c}: {r[c]!r}, want {v!r}" for c, v in OLD.items() if r[c] != v]
    it = db.execute("SELECT itemname FROM itempattern WHERE patternid = ?", (EMERALD,)).fetchone()
    if not it or it[0] != "Tal'Thex Emerald":
        bad.append(f"itempattern {EMERALD}: {it}")
    return bad


def report(db):
    r = row(db)
    if r:
        print(f"  quest {QUEST} step {STEP}: {r['kind']} {r['target']!r} item {r['items']} chance {r['chance']} "
              f"area world {r['world']} ({r['x']}, {r['z']}) radius {r['radius']}")


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        report(db)
        at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
        acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
        acct = acct[0] if acct else 0
        key = js({"questid": QUEST, "step": STEP})
        if a.undo:
            if not has(db, LOG):
                print("nothing to undo")
                return 0
            db.execute("BEGIN IMMEDIATE")
            before, after = db.execute(f"SELECT before, after FROM {LOG}").fetchone()
            if js(row(db)) != after:
                print("  the step changed since this tool wrote it: left as it is")
            else:
                b = json.loads(before)
                db.execute(f'UPDATE queststeps SET {", ".join(f"{c} = ?" for c in b)} WHERE questid = ? AND step = ?',
                           list(b.values()) + [QUEST, STEP])
                db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                           "VALUES (?, ?, ?, ?, '', 'update', 'queststeps', ?, ?, ?, ?)",
                           (at, acct, WHO, VIA, key, after, js(row(db)), "undone: " + DETAIL))
            db.execute(f"DROP TABLE {LOG}")
            db.execute("COMMIT")
            print("undone; takes effect at a restart")
            report(db)
            return 0
        if has(db, LOG):
            print(f"applied already ({LOG} is there): nothing to do (--undo to take it back)")
            return 0
        bad = check(db)
        if bad:
            print("refused, nothing done:\n  " + "\n  ".join(bad))
            return 1
        if not a.apply:
            print("report only: --apply to write it")
            return 0
        db.execute("BEGIN IMMEDIATE")
        before = row(db)
        db.execute(f'UPDATE queststeps SET {", ".join(f"{c} = ?" for c in NEW)} WHERE questid = ? AND step = ?',
                   list(NEW.values()) + [QUEST, STEP])
        after = row(db)
        db.execute(f"CREATE TABLE {LOG} (tbl TEXT NOT NULL, rowkey TEXT NOT NULL, before TEXT, after TEXT)")
        db.execute(f"INSERT INTO {LOG} VALUES ('queststeps', ?, ?, ?)", (key, js(before), js(after)))
        db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                   "VALUES (?, ?, ?, ?, '', 'update', 'queststeps', ?, ?, ?, ?)",
                   (at, acct, WHO, VIA, key, js(before), js(after), DETAIL))
        db.execute("COMMIT")
        print("applied; takes effect at a restart (the quests are read at a start)")
        report(db)
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
