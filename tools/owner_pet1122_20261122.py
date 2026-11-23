#!/usr/bin/env python3
"""Three of the owner's calls of 2026-11-22 on Heirloom petitions, as data, for either shard (--shard heirloom|live).
Every row is found by NAME and checked against what was read; a part whose rows are neither as read nor as asked
REFUSES (nothing is written). A part already in place, or whose rows this shard does not have (the homelands are
Heirloom's alone), writes nothing and says so, so the same run does each shard.

  2677  "13 spells shouldnt drop at all": the "level 13 rogue spell" a level 59 Jal'Raeth Patriarch dropped is
        "ethereal breath", Envaric Ascension's loot step item (a druid, necromancer and wizard quest of level 50).
        It and its twin "astral breath" (High Priest Vorin's) were made in the likeness of the rogue scroll Night
        Breath (tools/quests/data/portal.json: "like"), and kept its level 13 and its rogue-only class mask, so
        every client shows them as a level 13 rogue spell. Both become what their siblings of the quest are
        (ethereal essence, astral essence of Envar): level 1, every class, every race. The scroll art stays, as
        theirs. (The rule for real class spell scrolls off creatures far above them is code: handlers/loot.go
        lootScrollReach.)
  2681  The level 4 goblin ranger quest "Arms for the Old Road" (Pitmaster Grukk, step 3) gives a ranger the glade
        crafted longbow 5647 and now 200 of the plain "arrow" 1805 with it; Grocer Mudgrub (beside Grukk) sells
        that arrow, after his breads.
  2684  The level 5 goblin quest "Mother Mossweb": its spider fights by "roamer minor poison" (npctemplate), Prima's
        minor poison as tools/creature_venom_20261114.py gave the level 4-7 widow hatchlings: Toxic Bolt at 8%,
        9 a tick, kept up every 24 s from 4 s into a fight; no crit below level 10, as every roamer.

What takes effect when: item records at a client's next login; npcitem (the shop) at once; queststeps at the panel's
quest reload (quest_set_enabled off and on) or the server's next start; npctemplate at the panel's reload_npcs or the
server's next start.

Dry run by default. --apply writes in one transaction, a gm_audit line a row (accountid 1, dab, via tool), every row's
before and after in owner_pet1122_log; a second --apply writes nothing. --undo puts back what the log holds, latest
first; a row changed since is left and said.

    python3 -E -s tools/owner_pet1122_20261122.py --db eqoa.db --shard heirloom|live [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "owner_pet1122_log"
SCROLL_ICON = -607649328

BREATHS = ("astral breath", "ethereal breath")
BREATH_WAS = {"levelreq": 13, "classuse": 64, "raceuse": 2015}
BREATH_NOW = {"levelreq": 1, "classuse": 32767, "raceuse": 1023}

QUEST = "Arms for the Old Road"
GIVER = "Pitmaster Grukk"
RANGER = "1"
BOW, ARROW, ARROWS = 5647, 1805, 200
GROCER = "Grocer Mudgrub"

SPIDER = "mother mossweb"
POISON = "roamer minor poison"

DETAIL = {
    "2677": "petition 2677 (owner 2026-11-22: \"13 spells shouldnt drop at all\"): the breaths of Envaric Ascension "
            "were Night Breath's level 13 rogue mask: tools/owner_pet1122_20261122.py",
    "2681": "petition 2681 (owner 2026-11-22): 200 arrows with the goblin ranger's bow, arrows at Grocer Mudgrub: "
            "tools/owner_pet1122_20261122.py",
    "2684": "petition 2684 (owner 2026-11-22): Mother Mossweb poisons (roamer minor poison): "
            "tools/owner_pet1122_20261122.py",
}


class Refused(Exception):
    pass


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def rows(db, sql, args=()):
    cur = db.execute(sql, args)
    cols = [c[0] for c in cur.description]
    return [dict(zip(cols, r)) for r in cur.fetchall()]


def has_table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


class Plan:
    """The writes a run would make: (part, action, table, key, before, after), each one checked."""

    def __init__(self, db):
        self.db = db
        self.writes = []
        self.notes = []

    def say(self, part, line):
        self.notes.append(f"{part}: {line}")

    def write(self, part, action, tbl, key, before, after):
        self.writes.append((part, action, tbl, key, before, after))


def plan_breaths(p):
    for name in BREATHS:
        found = rows(p.db, "SELECT patternid, itemname, itemicon, equipslot, levelreq, classuse, raceuse FROM itempattern "
                           "WHERE itemname = ?", (name,))
        if not found:
            p.say("2677", f"no record {name!r} on this shard: nothing to do")
            continue
        if len(found) > 1:
            raise Refused(f"2677: {len(found)} records named {name!r}")
        r = found[0]
        if r["itemicon"] != SCROLL_ICON or r["equipslot"] != -1:
            raise Refused(f"2677: {name!r} {r['patternid']} is not in the scroll's art, worn nowhere: {r}")
        cur = {k: r[k] for k in BREATH_WAS}
        if cur == BREATH_NOW:
            p.say("2677", f"{name!r} {r['patternid']} already level 1, every class and race")
            continue
        if cur != BREATH_WAS:
            raise Refused(f"2677: {name!r} {r['patternid']} is {cur}, not as read {BREATH_WAS}")
        p.write("2677", "update", "itempattern", {"patternid": r["patternid"]}, cur, dict(BREATH_NOW))
        p.say("2677", f"{name!r} {r['patternid']}: level 13 rogue -> level 1, every class and race")


def plan_arrows(p):
    arrow = rows(p.db, "SELECT patternid, itemname, equipslot, maxstack FROM itempattern WHERE patternid = ?", (ARROW,))
    if not arrow or arrow[0]["itemname"] != "arrow" or arrow[0]["equipslot"] != -1 or arrow[0]["maxstack"] < ARROWS:
        raise Refused(f"2681: record {ARROW} is not the plain stackable arrow: {arrow}")
    qs = rows(p.db, "SELECT questid FROM quests WHERE name = ? AND giver = ?", (QUEST, GIVER))
    if not qs:
        p.say("2681", f"no quest {QUEST!r} of {GIVER} on this shard: nothing to do")
    elif len(qs) > 1:
        raise Refused(f"2681: {len(qs)} quests {QUEST!r} of {GIVER}")
    else:
        qid = qs[0]["questid"]
        steps = [s for s in rows(p.db, "SELECT questid, step, byclass FROM queststeps WHERE questid = ? AND kind = 'give' "
                                       "AND target = ?", (qid, GIVER)) if s["byclass"]]
        if len(steps) != 1:
            raise Refused(f"2681: quest {qid} has {len(steps)} give steps to {GIVER} with class rewards")
        st = steps[0]
        bc = json.loads(st["byclass"])
        items = bc.get(RANGER, {}).get("items", "")
        want = f"{BOW}, {ARROW}*{ARROWS}"
        if items == want:
            p.say("2681", f"quest {qid} step {st['step']} already gives a ranger {want}")
        elif items != str(BOW):
            raise Refused(f"2681: quest {qid} step {st['step']} gives a ranger {items!r}, not {BOW}")
        else:
            bc[RANGER]["items"] = want
            after = json.dumps(bc, sort_keys=True, separators=(",", ":"))
            p.write("2681", "update", "queststeps", {"questid": qid, "step": st["step"]},
                    {"byclass": st["byclass"]}, {"byclass": after})
            p.say("2681", f"quest {qid} step {st['step']}: a ranger gets {BOW} and {ARROWS} arrows ({ARROW})")
    grocer = rows(p.db, "SELECT id, npc_id, npc_type FROM npcs WHERE npc_name = ?", (GROCER,))
    if not grocer:
        p.say("2681", f"no {GROCER} on this shard: nothing to do")
        return
    ids = {g["npc_id"] for g in grocer}
    if len(ids) != 1 or 0 in ids or any(g["npc_type"] & 1 == 0 for g in grocer):
        raise Refused(f"2681: {GROCER}'s rows are not one merchant: {grocer}")
    npcid = ids.pop()
    stock = rows(p.db, "SELECT itemid, patternid, listnumber FROM npcitem WHERE npcid = ? ORDER BY listnumber", (npcid,))
    if any(s["patternid"] == ARROW for s in stock):
        p.say("2681", f"{GROCER} ({npcid}) already sells the arrow")
        return
    nxt = max([s["listnumber"] for s in stock], default=-1) + 1
    row = {"npcid": npcid, "stackleft": 1, "remainHP": 0, "remaincharge": 0, "patternid": ARROW, "equiploc": -1,
           "location": 1, "listnumber": nxt}
    p.write("2681", "insert", "npcitem", None, None, row)
    p.say("2681", f"{GROCER} ({npcid}) sells the arrow, list number {nxt}")


def plan_spider(p):
    if not rows(p.db, "SELECT 1 FROM npcs WHERE lower(npc_name) = ?", (SPIDER,)):
        p.say("2684", f"no {SPIDER!r} on this shard: nothing to do")
        return
    if not rows(p.db, "SELECT 1 FROM classtemplates WHERE name = ?", (POISON,)):
        raise Refused(f"2684: no class template {POISON!r} (tools/creature_venom_20261114.py)")
    t = rows(p.db, "SELECT name, template FROM npctemplate WHERE lower(name) = ?", (SPIDER,))
    if t and t[0]["template"] == POISON:
        p.say("2684", f"{SPIDER!r} already fights by {POISON!r}")
        return
    if t:
        raise Refused(f"2684: {SPIDER!r} fights by {t[0]['template']!r} (row {t[0]['name']!r}), not by none")
    row = {"name": SPIDER, "template": POISON, "note": "petition 2684 (owner 2026-11-22): the level 5 goblin quest's "
           "spider poisons, as the widow hatchlings (Prima's minor poison)", "source": "pet1122o"}
    p.write("2684", "insert", "npctemplate", {"name": SPIDER}, None, row)
    p.say("2684", f"{SPIDER!r} fights by {POISON!r}")


def plan(db):
    p = Plan(db)
    plan_breaths(p)
    plan_arrows(p)
    plan_spider(p)
    return p


def audit(db, part, action, tbl, key, before, after):
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, 1, 'dab', 'tool', '', ?, ?, ?, ?, ?, ?)",
               (now(), action, tbl, json.dumps(key, sort_keys=True),
                None if before is None else json.dumps(before), None if after is None else json.dumps(after),
                DETAIL[part]))


def where(key):
    cols = sorted(key)
    return " AND ".join(f"{c} = ?" for c in cols), [key[c] for c in cols]


def apply(db, p):
    db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (seq INTEGER PRIMARY KEY AUTOINCREMENT, part TEXT NOT NULL, "
               "action TEXT NOT NULL, tbl TEXT NOT NULL, rowkey TEXT NOT NULL, before TEXT, after TEXT NOT NULL)")
    for part, action, tbl, key, before, after in p.writes:
        if action == "update":
            w, args = where(key)
            sets = ", ".join(f"{c} = ?" for c in after)
            db.execute(f"UPDATE {tbl} SET {sets} WHERE {w}", list(after.values()) + args)
        else:
            cols = list(after)
            cur = db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                             [after[c] for c in cols])
            if key is None:
                key = {"itemid": cur.lastrowid}
        audit(db, part, action, tbl, key, before, after)
        db.execute(f"INSERT INTO {LOG} (part, action, tbl, rowkey, before, after) VALUES (?, ?, ?, ?, ?, ?)",
                   (part, action, tbl, json.dumps(key, sort_keys=True),
                    None if before is None else json.dumps(before), json.dumps(after)))
    return len(p.writes)


def undo(db):
    if not has_table(db, LOG):
        return 0, []
    n, left = 0, []
    for seq, part, action, tbl, rowkey, before, after in db.execute(
            f"SELECT seq, part, action, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key, after = json.loads(rowkey), json.loads(after)
        w, args = where(key)
        cur = rows(db, f"SELECT * FROM {tbl} WHERE {w}", args)
        if len(cur) != 1 or any(cur[0].get(c) != v for c, v in after.items()):
            left.append(f"{part} {tbl} {rowkey}: changed since, left")
            continue
        if action == "update":
            b = json.loads(before)
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in b)} WHERE {w}", list(b.values()) + args)
            audit(db, part, "update", tbl, key, after, b)
        else:
            db.execute(f"DELETE FROM {tbl} WHERE {w}", args)
            audit(db, part, "delete", tbl, key, after, None)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        n += 1
    return n, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--shard", required=True, choices=["heirloom", "live"])
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, timeout=5)
    try:
        if a.undo:
            n, left = undo(db)
            db.commit()
            for line in left:
                print(line)
            print(f"{a.shard}: undo: {n} rows put back")
            return 0
        try:
            p = plan(db)
        except Refused as e:
            print(f"{a.shard}: REFUSED, nothing written: {e}")
            return 2
        for line in p.notes:
            print(f"{a.shard}: {line}")
        if not a.apply:
            print(f"{a.shard}: dry run: {len(p.writes)} rows would change")
            return 0
        n = apply(db, p)
        db.commit()
        print(f"{a.shard}: applied: {n} rows, a gm_audit line each, logged in {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
