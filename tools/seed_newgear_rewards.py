#!/usr/bin/env python3
"""The rewards of Neriak's and Oggok's "New Gear" quests (the user, 2026-10-30: Dust turned in the
iron ore, the wooden shaft and the chipped raven beak and was given no weapon).

EQOA-Data's guides (the quests' source, tools/quests/data/) name the item each "New Gear" makes in
Freeport only: Neriak's and Oggok's say "I will make you a new weapon" (or robe) and the importer
gave their last step the experience alone. The user chose Freeport's counterparts ("Freeport's
rewards for all 11"), a shaman's equivalent for Oggok's shaman ("make shaman equiv"), and each
named for its quest ("name the items according to the quest, like black raven sword"). So each item
here is NEW, a copy of its counterpart's record (level, slot, damage, classes, look) under its
quest's name and a line saying who made it of what: OURS, not SOE's. The Black Raven Sword keeps the
Iron Hatchet's numbers and takes the Stone Sword's model and icon (the user's "sword"); Oggok's
necromancer is brought iron and leather, not cloth, so its item is a weapon (Muddy Shank's record),
not Freeport's robe; Oggok's shaman's is Corse Spiked Club's (a shaman's blunt weapon of level 5
and 50 damage, as the Iron Hatchet and the Acolyte's Hammer).

Each quest's last step (a give step with no reward item) is given the item, and a character who
finished the quest before (charquests done) is given it the way a quest gives one (GiveItemNow:
remainHP the record's maxhp, the inventory's next place). Every row written or changed is logged in
newgear_seed; --undo puts back only what is still as this left it. Report only by default. Run it
with the server stopped (or reload the quests and the characters) when anyone it gives to may be in
the world.

    tools/seed_newgear_rewards.py [--db eqoa.db] [--apply | --undo]
"""

import argparse
import json
import sqlite3
import sys

FIRST_ID = 706000

# questid, the new item's id, its name, the record it copies, the record whose model and icon it
# takes (None: its own), and its line
ITEMS = [
    (100430, 706000, "Batwing Hammer", 6055, None,
     "Forged by Denouncer V`Thoth from a leather strip, iron ore and a ruined bat wing."),
    (100450, 706001, "Blue Raven Robe", 4927, None,
     "Sewn by Magician Ka`Zuth from a cloth yard, common thread and a chipped raven beak."),
    (100460, 706002, "Black Tarantula Robe", 4922, None,
     "Sewn by Necromancer D'Unarr from a cloth yard, common thread and a flattened tarantula eye."),
    (100470, 706003, "Tarantula Dirk", 8540, None,
     "Forged by Agent Cythani from iron ore, a leather strap and a flattened tarantula eye."),
    (100480, 706004, "Black Raven Sword", 5703, 5859,
     "Forged by Sythonis U`Dedne from iron ore, a wooden shaft and a chipped raven beak."),
    (100490, 706005, "Firefly Blade", 8571, None,
     "Forged by Scornblade D`Eleth from a wooden shaft, iron ore and a firefly leg segment."),
    (100500, 706006, "Red Serpent Robe", 5012, None,
     "Sewn by Wizard S`Kor from a cloth yard, common thread and a sliver of snake meat."),
    (100510, 706007, "Raven Beak Shank", 5761, None,
     "Forged by Asogwe Greth from a leather strip, iron ore and a chipped raven beak."),
    (100520, 706008, "Centipede Hatchet", 5703, None,
     "Forged by Greenblood Yurgat from a cord of rotwood, a block of Rathestone and a damaged centipede eye."),
    (100530, 706009, "Eagle Talon Club", 5575, None,
     "Forged by Oligan from a cord of rotwood, iron ore and a chipped eagle talon."),
    (100540, 706010, "Eagle Feather Blade", 8571, None,
     "Forged by Warlord Brogar from a block of Rathestone, a cord of rotwood and broken eagle feathers."),
]

LOG_DDL = """CREATE TABLE IF NOT EXISTS newgear_seed (
    seq INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl TEXT NOT NULL,
    rid TEXT NOT NULL,
    col TEXT NOT NULL,
    old TEXT,
    new TEXT
)"""


def row(db, sql, args=()):
    cur = db.execute(sql, args)
    r = cur.fetchone()
    return None if r is None else dict(zip([c[0] for c in cur.description], r))


def last_give_step(db, questid):
    return row(db, "SELECT questid, step, kind, rewarditems FROM queststeps WHERE questid = ? "
                   "ORDER BY step DESC LIMIT 1", (questid,))


def plan(db):
    """What --apply would do, and what it would refuse."""
    todo, problems = [], []
    for questid, pid, name, base, look, desc in ITEMS:
        b = row(db, "SELECT * FROM itempattern WHERE patternid = ?", (base,))
        if b is None:
            problems.append(f"quest {questid}: the record {base} to copy is not here")
            continue
        here = row(db, "SELECT itemname FROM itempattern WHERE patternid = ?", (pid,))
        if here is not None and here["itemname"] != name:
            problems.append(f"quest {questid}: id {pid} is {here['itemname']!r} here, not {name!r}")
            continue
        st = last_give_step(db, questid)
        if st is None or st["kind"] != "give":
            problems.append(f"quest {questid}: its last step is not a give step")
            continue
        if st["rewarditems"] not in ("", str(pid)):
            problems.append(f"quest {questid}: step {st['step']} rewards {st['rewarditems']!r} already: left")
            continue
        item = dict(b)
        item.update(patternid=pid, itemname=name, itemdesc=desc)
        if look is not None:
            lk = row(db, "SELECT model, itemicon FROM itempattern WHERE patternid = ?", (look,))
            if lk is None:
                problems.append(f"quest {questid}: the record {look} to take a look from is not here")
                continue
            item.update(model=lk["model"], itemicon=lk["itemicon"])
        owed = [r[0] for r in db.execute(
            "SELECT c.serverid FROM charquests c WHERE c.questid = ? AND c.done = 1 AND NOT EXISTS "
            "(SELECT 1 FROM charinventory i WHERE i.serverid = c.serverid AND i.patternid = ?)", (questid, pid))]
        todo.append({"questid": questid, "step": st["step"], "item": item, "new": here is None,
                     "reward": st["rewarditems"] == "", "owed": owed})
    return todo, problems


def log(db, tbl, rid, col, old, new):
    db.execute("INSERT INTO newgear_seed (tbl, rid, col, old, new) VALUES (?, ?, ?, ?, ?)",
               (tbl, str(rid), col, None if old is None else json.dumps(old), None if new is None else json.dumps(new)))


def apply(db, todo):
    db.execute(LOG_DDL)
    n = {"items": 0, "rewards": 0, "given": 0}
    for t in todo:
        it = t["item"]
        if t["new"]:
            cols = list(it)
            db.execute(f"INSERT INTO itempattern ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                       [it[c] for c in cols])
            log(db, "itempattern", it["patternid"], "*", None, it)
            n["items"] += 1
        if t["reward"]:
            db.execute("UPDATE queststeps SET rewarditems = ? WHERE questid = ? AND step = ?",
                       (str(it["patternid"]), t["questid"], t["step"]))
            log(db, "queststeps", f"{t['questid']}/{t['step']}", "rewarditems", "", str(it["patternid"]))
            n["rewards"] += 1
        for sid in t["owed"]:
            cur = db.execute(
                "INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, "
                "location, listnumber) VALUES (?, 1, ?, 0, ?, -1, 0, (SELECT COALESCE(MAX(listnumber), -1) + 1 "
                "FROM charinventory WHERE serverid = ? AND location = 0))",
                (sid, it.get("maxhp") or 0, it["patternid"], sid))
            log(db, "charinventory", cur.lastrowid, "*", None, {"serverid": sid, "patternid": it["patternid"]})
            n["given"] += 1
    return n


def undo(db):
    if db.execute("SELECT count(*) FROM sqlite_master WHERE name = 'newgear_seed'").fetchone()[0] == 0:
        print("nothing logged: nothing to undo")
        return
    back = kept = 0
    for seq, tbl, rid, col, old, new in db.execute(
            "SELECT seq, tbl, rid, col, old, new FROM newgear_seed ORDER BY seq DESC").fetchall():
        new = json.loads(new) if new else None
        if tbl == "itempattern":
            now = row(db, "SELECT * FROM itempattern WHERE patternid = ?", (int(rid),))
            if now == new:
                db.execute("DELETE FROM itempattern WHERE patternid = ?", (int(rid),))
                back += 1
            else:
                kept += 1
        elif tbl == "queststeps":
            q, s = rid.split("/")
            cur = db.execute("UPDATE queststeps SET rewarditems = '' WHERE questid = ? AND step = ? AND rewarditems = ?",
                             (int(q), int(s), new))
            back, kept = (back + 1, kept) if cur.rowcount else (back, kept + 1)
        elif tbl == "charinventory":
            cur = db.execute("DELETE FROM charinventory WHERE itemid = ? AND serverid = ? AND patternid = ?",
                             (int(rid), new["serverid"], new["patternid"]))
            back, kept = (back + 1, kept) if cur.rowcount else (back, kept + 1)
    db.execute("DROP TABLE newgear_seed")
    print(f"put back {back}; left {kept} changed since")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", default="eqoa.db")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 10000")
    if a.undo:
        db.execute("BEGIN IMMEDIATE")
        undo(db)
        db.execute("COMMIT")
        return
    todo, problems = plan(db)
    for t in todo:
        it = t["item"]
        print(f"quest {t['questid']} step {t['step']}: {it['patternid']} {it['itemname']!r} "
              f"(level {it['levelreq']}, slot {it['equipslot']}, damage {it['weapondamage']}, classes {it['classuse']})"
              f"{'' if t['new'] else ' here already'}{'' if t['reward'] else ', reward set already'}"
              f"{', given to ' + ', '.join(map(str, t['owed'])) if t['owed'] else ''}")
    for p in problems:
        print("PROBLEM:", p)
    if not a.apply:
        print("report only: --apply to write")
        return
    if problems:
        sys.exit("refused: the problems above first")
    db.execute("BEGIN IMMEDIATE")
    n = apply(db, todo)
    db.execute("COMMIT")
    print(f"wrote {n['items']} items, {n['rewards']} rewards, {n['given']} given; reload the quests (a restart)")


if __name__ == "__main__":
    main()
