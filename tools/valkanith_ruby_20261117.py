#!/usr/bin/env python3
"""The Lord Valkanith quest's ruby from the rallosian spectres and warlords of Geomancer's Citadel (the owner,
2026-11-17, at Dust's place in the citadel, world 1 (4703.7, 4474.8): "these mobs are named wrong should be the
rallosian spectres and warlords that drop tal thex ruby for lord valkanith quest"; Ronson's post, forum t171: the ruby
"from rallosian specters and warlords in Geomancers Citadel").

THE CITADEL, world 1: x 4550-4950, z 4400-4950, standing at 100 or higher. worlds/ground/1.walls's zone 7 (x 4174-5994,
z 4005-5994) is mostly open ground; its placements cluster in one built core at x 4550-4950, z 4450-4900, the raised
citadel (its creatures stand at 121-144: the Anvil of Sarek, Archon Seradesh, the terran caretakers). Inside it: 22
"spectral guardian" (L61, ids 124825-124847) and 1 "spectral warlord" (L64, 124824).

WHAT CHANGES:
  - Those 23 rows are renamed: "spectral guardian" to "rallosian spectre", "spectral warlord" to "rallosian warlord".
  - Every name-keyed row of the two old names is COPIED to the new ones, not moved, because the old names still stand
    elsewhere: the world 0 spectral guardians (the Tomb of Kings' L35, which quest 100777 step 2 loots; one L44) and
    the three L62 spectral warlords of Geomancer's Pass (z 5058-5223, which quest 101276 step 4 loots). Copied:
    npcclass, npcundead, npcloot, npcmove, npctemplate, lootpool and lootcreature. lootcreature's region for both
    new names is "rathe mountains" (the spectral warlord's): the spectral guardian's "desert" was the Tomb of Kings'.
    No npcfaction, npcrespawn-by-name, npctint, npcdialog, trigger or quest event names them.
  - The 18 "spectral rallosian" rows (L58-60, z 5090-5247, standing at 65-75: Geomancer's Pass, the seeder's "spectral
    rallosian cave", below and south of the citadel) are NOT in the citadel and keep their name.
  - Quest 101269 step 2 (the Tal'Thex Ruby 701384) loots "rallosian spectre" and, through queststeptargets, "rallosian
    warlord", in place of "spectral rallosian"; its chance 0.25 -> 0.10 (the owner, 2026-11-17: "make the ruby 10% too").

Report only by default; --apply in one transaction with gm_audit lines and the log valkanith_ruby_1117_log (every row
before and after); a second --apply does nothing; --undo puts back exactly what it wrote and drops the log. Refused
whole if a row is not as this tool expects.

WHEN IT TAKES EFFECT: the names, templates, classes, moves and loot at the GM panel's Reload every NPC or a start; the
quest step at a start (a keep restart's before.sh, the server stopped).

    python3 -E -s tools/valkanith_ruby_20261117.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "valkanith_ruby_1117_log"
WHO, VIA = "dab", "tool"
SRC = "valkanith-ruby1117"
DETAIL = ("the Tal'Thex ruby from the rallosian spectres and warlords of Geomancer's Citadel (the owner, 2026-11-17): "
          "tools/valkanith_ruby_20261117.py")

WORLD = 1
BOX = (4550.0, 4950.0, 4400.0, 4950.0)  # x0, x1, z0, z1
MIN_Y = 100.0
RENAME = {"spectral guardian": ("rallosian spectre", "rallosian spectre"),
          "spectral warlord": ("rallosian warlord", "rallosian warlord")}
WANT_ROWS = {"spectral guardian": 22, "spectral warlord": 1}
REGION = "rathe mountains"

QUEST, STEP = 101269, 2
RUBY = 701384
STEP_OLD = {"kind": "loot", "target": "spectral rallosian", "items": str(RUBY), "chance": 0.25,
            "journal": "Take the Tal'Thex ruby from the rallosian spectres of Geomancer's Citadel."}
STEP_NEW = {"target": "rallosian spectre", "chance": 0.10,
            "journal": "Take the Tal'Thex ruby from the rallosian spectres and warlords of Geomancer's Citadel."}
ALSO = "rallosian warlord"

# name-keyed tables: table -> (name column, key columns other than the name, or None for an autoincrement id)
KEYED = {"npcclass": ("name", ()), "npcundead": ("name", ()), "npcloot": ("npc_name", ("tablename",)),
         "npcmove": ("name", ()), "npctemplate": ("name", ()), "lootpool": ("npc_name", None),
         "lootcreature": ("npc_name", ())}


def has(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone() is not None


def rows(db, sql, args=()):
    cur = db.execute(sql, args)
    cols = [d[0] for d in cur.description]
    return [dict(zip(cols, r)) for r in cur.fetchall()]


def one(db, tbl, key):
    r = rows(db, f'SELECT * FROM "{tbl}" WHERE {" AND ".join(f"{c} = ?" for c in key)}', tuple(key.values()))
    return r[0] if r else None


def js(v):
    return json.dumps(v, separators=(",", ":")) if v is not None else None


def citadel_rows(db, old):
    x0, x1, z0, z1 = BOX
    return rows(db, "SELECT id, npc_name, npc_level FROM npcs WHERE world = ? AND lower(npc_name) = ? AND x_coord BETWEEN ? "
                    "AND ? AND z_coord BETWEEN ? AND ? AND y_coord >= ? ORDER BY id", (WORLD, old, x0, x1, z0, z1, MIN_Y))


def check(db):
    bad = []
    for old, (new, _) in RENAME.items():
        n = len(citadel_rows(db, old))
        if n != WANT_ROWS[old]:
            bad.append(f"{n} {old!r} rows in the citadel, want {WANT_ROWS[old]}")
        if db.execute("SELECT count(*) FROM npcs WHERE lower(npc_name) = ?", (new,)).fetchone()[0]:
            bad.append(f"npcs named {new!r} are there already")
        for tbl, (col, _) in KEYED.items():
            if db.execute(f'SELECT count(*) FROM "{tbl}" WHERE lower({col}) = ?', (new,)).fetchone()[0]:
                bad.append(f"{tbl} has {new!r} already")
    s = one(db, "queststeps", {"questid": QUEST, "step": STEP})
    if s is None:
        bad.append(f"no queststeps {QUEST} step {STEP}")
    else:
        bad += [f"queststeps {QUEST} step {STEP} {c}: {s[c]!r}, want {v!r}" for c, v in STEP_OLD.items() if s[c] != v]
    if db.execute("SELECT count(*) FROM queststeptargets WHERE questid = ? AND step = ?", (QUEST, STEP)).fetchone()[0]:
        bad.append(f"queststeptargets {QUEST} step {STEP} has rows already")
    return bad


def report(db):
    for name in ("spectral guardian", "spectral warlord", "spectral rallosian", "rallosian spectre", "rallosian warlord"):
        r = db.execute("SELECT world, count(*) FROM npcs WHERE lower(npc_name) = ? GROUP BY world", (name,)).fetchall()
        print(f"  {name}: {dict(r)} (world: rows)")
    s = one(db, "queststeps", {"questid": QUEST, "step": STEP})
    also = [r[0] for r in db.execute("SELECT target FROM queststeptargets WHERE questid = ? AND step = ?", (QUEST, STEP))]
    print(f"  quest {QUEST} step {STEP}: {s and s['target']!r} also {also} chance {s and s['chance']}")


def apply(db):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    db.execute(f"CREATE TABLE {LOG} (seq INTEGER PRIMARY KEY, tbl TEXT NOT NULL, rowkey TEXT NOT NULL, before TEXT, after TEXT)")

    def done(tbl, key, before):
        after = one(db, tbl, key)
        action = "update" if before and after else "insert" if after else "delete"
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES (?, ?, ?, ?)", (tbl, js(key), js(before), js(after)))
        db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                   "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                   (at, acct[0] if acct else 0, WHO, VIA, action, tbl, js(key), js(before), js(after), DETAIL))

    def update(tbl, key, values):
        before = one(db, tbl, key)
        db.execute(f'UPDATE "{tbl}" SET {", ".join(f"{c} = ?" for c in values)} WHERE {" AND ".join(f"{c} = ?" for c in key)}',
                   list(values.values()) + list(key.values()))
        done(tbl, key, before)

    for old, (new, shown) in RENAME.items():
        for r in citadel_rows(db, old):
            update("npcs", {"id": r["id"]}, {"npc_name": shown})
        for tbl, (col, keys) in KEYED.items():
            for r in rows(db, f'SELECT * FROM "{tbl}" WHERE lower({col}) = ?', (old,)):
                r = dict(r, **{col: new})
                if "source" in r:
                    r["source"] = SRC
                if tbl == "lootcreature":
                    r["region"] = REGION
                if keys is None:
                    r.pop("id")
                    db.execute(f'INSERT INTO "{tbl}" ({", ".join(r)}) VALUES ({", ".join("?" * len(r))})', list(r.values()))
                    done(tbl, {"id": db.execute("SELECT last_insert_rowid()").fetchone()[0]}, None)
                else:
                    db.execute(f'INSERT INTO "{tbl}" ({", ".join(r)}) VALUES ({", ".join("?" * len(r))})', list(r.values()))
                    done(tbl, {col: new, **{k: r[k] for k in keys}}, None)

    update("queststeps", {"questid": QUEST, "step": STEP}, STEP_NEW)
    db.execute("INSERT INTO queststeptargets (questid, step, target) VALUES (?, ?, ?)", (QUEST, STEP, ALSO))
    done("queststeptargets", {"questid": QUEST, "step": STEP, "target": ALSO}, None)


def undo(db):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    n = 0
    for tbl, rowkey, before, after in db.execute(f"SELECT tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key = json.loads(rowkey)
        where = " AND ".join(f"{c} = ?" for c in key)
        if js(one(db, tbl, key)) != after:
            print(f"  {tbl} {rowkey}: changed since this tool wrote it, left as it is")
            continue
        if before is None:
            db.execute(f'DELETE FROM "{tbl}" WHERE {where}', tuple(key.values()))
        else:
            b = json.loads(before)
            db.execute(f'UPDATE "{tbl}" SET {", ".join(f"{c} = ?" for c in b)} WHERE {where}', list(b.values()) + list(key.values()))
        n += 1
    db.execute(f"DROP TABLE {LOG}")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', ?, '{}', NULL, NULL, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, LOG, f"undone, {n} rows put back: " + DETAIL))
    print(f"  {n} rows put back")


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
        if a.undo:
            if not has(db, LOG):
                print("nothing to undo")
                return 0
            db.execute("BEGIN IMMEDIATE")
            undo(db)
            db.execute("COMMIT")
            print("undone; takes effect at a reload of the NPCs and a restart")
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
        apply(db)
        db.execute("COMMIT")
        print("applied; the names at a reload of the NPCs, the quest step at a restart")
        report(db)
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
