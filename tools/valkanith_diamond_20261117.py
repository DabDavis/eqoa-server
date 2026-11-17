#!/usr/bin/env python3
"""The Lord Valkanith quest's diamond from any creature of the dark elf tower of Sycamore Joy's Rest, at 10% (the owner,
2026-11-17, at Dust's place in the tower, world 0 (23607.4, 73.1, 23714.0): "tal thex diamond isnt dropping for dust in
the tower he is in"; Ronson's post, forum t171: the diamond "from Sycamore Joy's Rest, the dark elf tower on the east
coast").

WHAT DIFFERED (quest 101269 step 3, loot of the Tal'Thex Diamond 701385): the Teir'Dal knights alone dropped it, at
25%, and none stands in the tower (four stand round x 22167-22822, z 22180-22851 and two by 23170, 22683, in Sycamore
Joy's Rest's open ground; two by Takish'Hiz). The tower holds house n'ryt knights (L50-51), n'ryt necromancers (L50-53),
Nightseer Kevros (L49), a spectral slayer, minion and guardian. Now the step's target is "*" (database.AnyCreature, the
build328 server): any creature killed within the tower's circle drops it for a player on the step at 10%, and the
Teir'Dal knights keep dropping it wherever they stand (queststeptargets, at the step's 10%).

THE TOWER, world 0, a circle of radius 45 round (23601.5, 23711): it IS in Sycamore Joy's Rest (worlds/ground/0.walls's
zone 132 is the client's "Sycamore Joys Rest" tile, x 21989-23894, z 21989-24004). The tower is that tile's one placed
structure of its own buffer (1219), x 23577-23626, y 65-114, z 23686-23736 (the others round it are the trees' and
rocks' repeated buffers): the circle is centred on it and reaches 10 past its corners (35), for a corpse at its door.
Within it stand the tower's ten creatures and nothing else; the desert's griffons, drakes and spiders count only if
they die inside.

Report only by default; --apply in one transaction with gm_audit lines and the log valkanith_diamond_1117_log; a second
--apply does nothing; --undo puts back exactly what it wrote and drops the log. Refused whole if the step is not as this
tool expects.

WHEN IT TAKES EFFECT: the quests are read at a start: run it in a keep restart's before.sh, the server stopped, with a
server that has AnyCreature (build328 on).

    python3 -E -s tools/valkanith_diamond_20261117.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "valkanith_diamond_1117_log"
WHO, VIA = "dab", "tool"
DETAIL = ("the Tal'Thex diamond from any creature of Sycamore Joy's Rest's dark elf tower at 10% (the owner, 2026-11-17): "
          "tools/valkanith_diamond_20261117.py")

QUEST, STEP = 101269, 3
DIAMOND = 701385  # the diamond
OLD = {"kind": "loot", "target": "Teir'Dal knight", "items": str(DIAMOND), "chance": 0.25, "world": 0, "x": 0.0, "z": 0.0,
       "radius": 15.0, "journal": "Take the Tal'Thex diamond from the dark elves of Sycamore Joy's Rest, on the east coast."}
NEW = {"target": "*", "chance": 0.10, "world": 0, "x": 23601.5, "z": 23711.0, "radius": 45.0,
       "journal": "Take the Tal'Thex diamond from the dark elf tower of Sycamore Joy's Rest, on the east coast: any "
                  "creature in it may carry it, as may the Teir'Dal knights."}
ALSO = "Teir'Dal knight"


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
    it = db.execute("SELECT itemname FROM itempattern WHERE patternid = ?", (DIAMOND,)).fetchone()
    if not it or it[0] != "Tal'Thex Diamond":
        bad.append(f"itempattern {DIAMOND}: {it}")
    if db.execute("SELECT count(*) FROM queststeptargets WHERE questid = ? AND step = ?", (QUEST, STEP)).fetchone()[0]:
        bad.append(f"queststeptargets {QUEST} step {STEP} has rows already")
    return bad


def report(db):
    r = row(db)
    if r:
        print(f"  quest {QUEST} step {STEP}: {r['kind']} {r['target']!r} item {r['items']} chance {r['chance']} "
              f"area world {r['world']} ({r['x']}, {r['z']}) radius {r['radius']}, also "
              f"{[t[0] for t in db.execute('SELECT target FROM queststeptargets WHERE questid = ? AND step = ?', (QUEST, STEP))]}")


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
            db.execute("DELETE FROM queststeptargets WHERE questid = ? AND step = ? AND target = ?", (QUEST, STEP, ALSO))
            db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                       "VALUES (?, ?, ?, ?, '', 'delete', 'queststeptargets', ?, ?, NULL, ?)",
                       (at, acct, WHO, VIA, js({"questid": QUEST, "step": STEP, "target": ALSO}),
                        js({"questid": QUEST, "step": STEP, "target": ALSO}), "undone: " + DETAIL))
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
        db.execute("INSERT INTO queststeptargets (questid, step, target) VALUES (?, ?, ?)", (QUEST, STEP, ALSO))
        db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                   "VALUES (?, ?, ?, ?, '', 'insert', 'queststeptargets', ?, NULL, ?, ?)",
                   (at, acct, WHO, VIA, js({"questid": QUEST, "step": STEP, "target": ALSO}),
                    js({"questid": QUEST, "step": STEP, "target": ALSO}), DETAIL))
        db.execute("COMMIT")
        print("applied; takes effect at a restart (the quests are read at a start)")
        report(db)
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
