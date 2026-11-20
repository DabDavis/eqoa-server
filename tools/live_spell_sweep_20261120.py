#!/usr/bin/env python3
"""Every class's level 50+ scrolls no merchant on Live sold, at that class's guild merchants (the owner, 2026-11-20:
"sweep every class for unsold 50+ spells on live", after Live petitions 5368 and 5370 found the cleric's heals and
Goad sold by no one there).

Heirloom sells each class's level 46-60 vendor spells at its Spell Archivists (2026-11-08, petition 1273: classspells
less the masteries', master classes' and epics'), which Live does not have (tools/shard_sync_20261115.py leaves their
stock Heirloom's), so on Live a scroll the Archivists sell and no other merchant does could not be bought. The list is
the Archivists' stock read off Heirloom (--archivist FILE: one patternid a line, as `SELECT DISTINCT i.patternid FROM
npcitem i JOIN npcs n ON n.id = i.npcid WHERE n.npc_name = 'Spell Archivist'` prints it); each record of it Live has
and no Live merchant stocks, of level 46 and up, goes last into the stock of each guild merchant of every class its
class mask names: a merchant that stocks a scroll of that class alone (classuse the class's bit) of level 40-49. A
scroll a loot table or pool holds is no unsold scroll: Live's level 51-57 spells drop off creatures (tables "t303 51-54
spells", "t303 57 spells", "uwu ... spells", as the live game's did: its class vendors ended at 49), its level 60s
off the raids ("pod spells", "zp spells"), and a merchant selling them would make the drops pointless; so is one a
quest gives. A
merchant whose list would pass the client's 40 (VIItemList's ItemInternal[40]) gets none, and is said. A scroll of
value 0 (never priced: no merchant sold it) gets the median value of Live's stocked scrolls of its level.

Records Live does not have (Heirloom's own) are left out and listed. Live only. Refused when a merchant already stocks
one of its scrolls (run twice: nothing to do). Report only by default; --apply in one transaction, a gm_audit line a
row, logged in live_spell_sweep_log; --undo exact.

WHEN IT TAKES EFFECT: a merchant's stock and prices are read at each window; a record's value at the next login.

    tools/live_spell_sweep_20261120.py --db eqoa-live/eqoa.db --archivist arch.txt [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import statistics
import sys

LOG = "live_spell_sweep_log"
DETAIL = "the owner's sweep: every class's unsold level 50+ scrolls at its guild merchants on Live: tools/live_spell_sweep_20261120.py"
WHO, VIA = "claude", "tool"
LIST_MOST = 40
MIN_LEVEL = 46
CLASSES = {0: "warrior", 1: "ranger", 2: "paladin", 3: "shadow knight", 4: "monk", 5: "bard", 6: "rogue", 7: "druid",
           8: "shaman", 9: "cleric", 10: "magician", 11: "necromancer", 12: "enchanter", 13: "wizard", 14: "alchemist"}


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def audit(db, action, tbl, key, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key), go_json(before), go_json(after), detail))


def plan(db, arch):
    """(adds {npcid: [pid]}, prices {pid: (old, new)}, notes [str])."""
    notes = []
    have = {r[0]: r[1:] for r in db.execute("SELECT patternid, itemname, classuse, levelreq, itemcost FROM itempattern")}
    absent = [p for p in arch if p not in have]
    if absent:
        notes.append(f"{len(absent)} Archivist records Live does not have (left out): {absent}")
    stocked = {r[0] for r in db.execute("SELECT DISTINCT patternid FROM npcitem")}
    dropped = set()
    for t in ("loottables", "lootpool"):
        if table(db, t):
            dropped.update(r[0] for r in db.execute(f"SELECT DISTINCT patternid FROM {t}"))
    given = set()
    if table(db, "queststeps"):
        for items, byclass, choices in db.execute("SELECT rewarditems, byclass, choices FROM queststeps"):
            text = ",".join(x or "" for x in (items, byclass, choices))
            given.update(int(t) for t in "".join(c if c.isdigit() else " " for c in text).split() if t.isdigit() and int(t) in set(arch))
    left = [p for p in arch if p in have and p not in stocked and have[p][2] >= MIN_LEVEL]
    drops = [p for p in left if p in dropped or p in given]
    if drops:
        notes.append(f"{len(drops)} unsold scrolls a loot table or a quest gives (left out): " + ", ".join(have[p][0] for p in drops))
    todo = sorted((p for p in left if p not in dropped and p not in given), key=lambda p: (have[p][2], have[p][0]))
    # the median value of Live's stocked scrolls of a level (a scroll: a record of an Archivist's or a class's alone)
    by_level = {}
    for pid, name, cls, lvl, cost in db.execute(
            "SELECT DISTINCT p.patternid, p.itemname, p.classuse, p.levelreq, p.itemcost FROM itempattern p JOIN npcitem i USING (patternid) "
            "WHERE p.equipslot = -1 AND p.itemcost > 0 AND p.levelreq >= ? AND p.classuse <> 32767", (MIN_LEVEL,)):
        by_level.setdefault(lvl, []).append(cost)
    prices = {}
    for p in todo:
        if have[p][3] == 0:
            costs = by_level.get(have[p][2])
            if not costs:
                notes.append(f"{have[p][0]} ({p}, level {have[p][2]}): no stocked scroll of its level to price it by; left at 0")
                continue
            prices[p] = (0, int(statistics.median(costs)))
    # each class's guild merchants
    merchants = {}
    for cls in CLASSES:
        bit = 1 << cls
        merchants[cls] = [r[0] for r in db.execute(
            "SELECT DISTINCT i.npcid FROM npcitem i JOIN itempattern p USING (patternid) WHERE p.classuse = ? AND p.levelreq BETWEEN 40 AND 49 ORDER BY i.npcid", (bit,))]
    count = {n: c for n, c in db.execute("SELECT npcid, COUNT(*) FROM npcitem GROUP BY npcid")}
    want = {}
    for p in todo:
        for cls in CLASSES:
            if have[p][1] & (1 << cls):
                for n in merchants[cls]:
                    want.setdefault(n, [])
                    if p not in want[n]:
                        want[n].append(p)
    adds = {}
    for n, pids in sorted(want.items()):
        name = db.execute("SELECT npc_name FROM npcs WHERE id = ?", (n,)).fetchone()[0]
        if count.get(n, 0) + len(pids) > LIST_MOST:
            notes.append(f"{name} ({n}): {count.get(n, 0)} listed + {len(pids)} would pass {LIST_MOST}: none put there")
            continue
        adds[n] = pids
    for p in todo:
        if not any(p in v for v in adds.values()):
            notes.append(f"{have[p][0]} ({p}): no merchant of its classes takes it")
    return todo, adds, prices, notes, have


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    ap.add_argument("--archivist")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            if not table(db, LOG):
                db.execute("ROLLBACK")
                print("nothing to undo")
                return 0
            rows = db.execute(f"SELECT tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall()
            for tbl, key, before, after in rows:
                key, after = json.loads(key), json.loads(after)
                if tbl == "npcitem":
                    now = db.execute("SELECT npcid, patternid FROM npcitem WHERE itemid = ?", (key["itemid"],)).fetchone()
                    ok = now == (after["npcid"], after["patternid"])
                else:
                    ok = db.execute("SELECT itemcost FROM itempattern WHERE patternid = ?", (key["patternid"],)).fetchone() == (after["itemcost"],)
                if not ok:
                    db.execute("ROLLBACK")
                    print(f"REFUSED: {tbl} {key} changed since")
                    return 1
            for tbl, key, before, after in rows:
                key, before, after = json.loads(key), json.loads(before) if before else None, json.loads(after)
                if tbl == "npcitem":
                    db.execute("DELETE FROM npcitem WHERE itemid = ?", (key["itemid"],))
                    audit(db, "delete", tbl, key, after, None, "undo: " + DETAIL)
                else:
                    db.execute("UPDATE itempattern SET itemcost = ? WHERE patternid = ?", (before["itemcost"], key["patternid"]))
                    audit(db, "update", tbl, key, after, before, "undo: " + DETAIL)
            db.execute(f"DROP TABLE {LOG}")
            db.execute("COMMIT")
            print(f"put back {len(rows)}")
            return 0
        if table(db, LOG):
            db.execute("ROLLBACK")
            print("nothing to do (applied: live_spell_sweep_log holds it)")
            return 0
        if not a.archivist:
            db.execute("ROLLBACK")
            print("REFUSED: --archivist FILE is needed (the Archivists' patternids, read off Heirloom)")
            return 1
        arch = sorted({int(l) for l in open(a.archivist) if l.strip()})
        if table(db, "npcs") and db.execute("SELECT COUNT(*) FROM npcs WHERE npc_name = 'Spell Archivist'").fetchone()[0]:
            db.execute("ROLLBACK")
            print("REFUSED: this database has Spell Archivists (Heirloom): the sweep is Live's")
            return 1
        todo, adds, prices, notes, have = plan(db, arch)
        print(f"{len(todo)} unsold scrolls of level {MIN_LEVEL}+:")
        for p in todo:
            name, cls, lvl, cost = have[p]
            print(f"  {p} {name} (level {lvl}, classes {cls}){'; value 0 -> %d' % prices[p][1] if p in prices else ''}")
        for n, pids in adds.items():
            name = db.execute("SELECT npc_name FROM npcs WHERE id = ?", (n,)).fetchone()[0]
            print(f"  {name} ({n}): +{len(pids)}")
        for s in notes:
            print("NOTE:", s)
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"{sum(len(v) for v in adds.values())} stock rows to {len(adds)} merchants, {len(prices)} values: report only (--apply to write)")
            return 0
        db.execute(f"CREATE TABLE {LOG} (seq INTEGER PRIMARY KEY AUTOINCREMENT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT)")
        for n, pids in adds.items():
            last = db.execute("SELECT COALESCE(MAX(listnumber), -1) FROM npcitem WHERE npcid = ?", (n,)).fetchone()[0]
            for p in pids:
                last += 1
                row = {"npcid": n, "stackleft": 1, "remainHP": 0, "remaincharge": 0, "patternid": p, "equiploc": -1,
                       "location": 1, "listnumber": last}
                cur = db.execute(f"INSERT INTO npcitem ({', '.join(row)}) VALUES ({', '.join('?' for _ in row)})", list(row.values()))
                key = {"itemid": cur.lastrowid}
                db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES ('npcitem', ?, NULL, ?)", (json.dumps(key), json.dumps(row)))
                audit(db, "insert", "npcitem", key, None, dict(row, itemid=cur.lastrowid), DETAIL)
        for p, (old, new) in prices.items():
            db.execute("UPDATE itempattern SET itemcost = ? WHERE patternid = ?", (new, p))
            key = {"patternid": p}
            db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES ('itempattern', ?, ?, ?)",
                       (json.dumps(key), json.dumps({"itemcost": old}), json.dumps({"itemcost": new})))
            audit(db, "update", "itempattern", key, {"itemcost": old}, {"itemcost": new}, DETAIL)
        db.execute("COMMIT")
        print(f"applied: {sum(len(v) for v in adds.values())} stock rows, {len(prices)} values")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
