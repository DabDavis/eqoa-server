#!/usr/bin/env python3
"""The rehabilitators' data (the user, 2026-10-30; handlers/rehab.go): "Rehabilitator Anja in Muniels tea
garden and another near freeport NPC, talk to them", "the rehab person sells it for 10 million tunar. Rose of
Rehabilitation", "Roses for paths", and "1 free path respec w/ 60 epic. Or rose as reward".

  - The Rose of Rehabilitation, item 709300 (ROSE_ID; Go's database.RoseOfRehabilitation): what pays a rehabilitator
    for a path chosen again once a character's free path respecs are used. It takes "rose of renewal"'s look (1001: its icon
    759960290 is a pink rose on the disc's ITEMICON.ESF, where Oceanic Rose's 1342617881 is a coil of rope, the
    quest import's art for 163 items), is worth 10,000,000 (a merchant will not buy it: handlers/merchant.go),
    NO TRADE (trade 1) and one a record.
  - Rehabilitator Frejo talks: the NPC bit (npc_type 0x80, a talker's: not attacked, never attacking) on every
    rehabilitator's npcs row that lacks it. Frejo's was 0, the flags a client draws a creature to fight with; Anja's
    is 128.
  - A class's level 60 epic gives one: the last step of each epic's reward quest (60/<archetype>/reward-<class>,
    Ediopolus's, where the epic ends: 23 of them) has the rose among its rewards; a character who finished one
    before is given a rose for each such quest it finished, less every rose it holds, has used at a rehabilitator
    (charresets: a path paid for) or this gave it before, so no run gives one twice (one a character an epic; an
    epic's reward quest is not repeatable, so it is finished once), the way a quest gives an item (the inventory's
    next place).

Every row written or changed is logged in rehab_seed; --undo puts back only what is still as this left it. One
transaction. A report by default. --write-data DIR names the rose in the epics' quest file there (60.json, written
in its own layout: json.dump with indent 1), so a re-import writes what --apply does. The server reads the npcs and
the quests at its start: apply this before the restart that brings the rehabilitators' code (an item record is read
at each request).

    tools/seed_rehabilitation.py [--db eqoa.db] [--apply | --undo] [--write-data tools/quests/data]
"""

import argparse
import json
import os
import re
import sqlite3
import sys

ROSE_ID = 709300
ROSE_NAME = "Rose of Rehabilitation"
ROSE_LOOK = 1001  # rose of renewal
ROSE_ICON = 759960290
ROSE_VALUE = 10_000_000
ROSE_DESC = ("Bring this rose to Rehabilitator Anja in Muniel's Tea Garden or Rehabilitator Frejo near Freeport "
             "to choose a path again once your free path respecs are spent.")
NPC_BIT = 0x80
EPIC_KEY = re.compile(r"^EQOA-Data (60/[^/ ]+/reward-[a-z]+) \(tools/import_quests\.py\)$")
EPIC_FILE_KEY = re.compile(r"^60/[^/]+/reward-[a-z]+$")

LOG_DDL = """CREATE TABLE IF NOT EXISTS rehab_seed (
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


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def item_ids(s):
    """A rewards column's ids ("1081, 709300*2"), each with its count, in order."""
    out = []
    for f in (s or "").split(","):
        f = f.strip()
        if not f:
            continue
        pid, _, n = f.partition("*")
        try:
            out.append((int(pid), int(n) if n else 1))
        except ValueError:
            pass
    return out


def the_rose(db):
    """The rose's record as --apply writes it: rose of renewal's columns, else the schema's defaults."""
    base = row(db, "SELECT * FROM itempattern WHERE patternid = ?", (ROSE_LOOK,))
    if base is None:
        cols = [r[1] for r in db.execute("PRAGMA table_info(itempattern)")]
        base = {c: 0 for c in cols}
        base.update(itemicon=ROSE_ICON, color=4294967295, rent=1, classuse=32767, raceuse=1023)
    rose = dict(base)
    rose.update(patternid=ROSE_ID, itemname=ROSE_NAME, itemdesc=ROSE_DESC, itemcost=ROSE_VALUE, trade=1, lore=0,
                levelreq=1, maxstack=1, maxhp=0, duration=0, equipslot=-1, craft=0)
    for c in ("str", "sta", "agi", "wis", "dex", "cha", "intelligence", "HPMAX", "POWMAX", "PoT", "HoT", "AC",
              "PR", "DR", "FR", "CR", "LR", "AR", "weaponproc", "fish"):
        if c in rose:
            rose[c] = None
    return rose


def epic_quests(db):
    """The epics' reward quests: (questid, key, its last step, that step's rewarditems)."""
    out = []
    for qid, src in db.execute("SELECT questid, source FROM quests ORDER BY questid").fetchall():
        m = EPIC_KEY.match(src or "")
        if not m:
            continue
        st = row(db, "SELECT step, rewarditems FROM queststeps WHERE questid = ? ORDER BY step DESC LIMIT 1", (qid,))
        if st is not None:
            out.append((qid, m.group(1), st["step"], st["rewarditems"]))
    return out


def plan(db):
    """What --apply would do, and what it would refuse."""
    p = {"rose": None, "npcs": [], "rewards": [], "given": [], "problems": []}
    here = row(db, "SELECT itemname FROM itempattern WHERE patternid = ?", (ROSE_ID,))
    if here is None:
        p["rose"] = the_rose(db)
    elif here["itemname"] != ROSE_NAME:
        p["problems"].append(f"item {ROSE_ID} is {here['itemname']!r} here, not {ROSE_NAME!r}")
    for nid, name, t in db.execute("SELECT id, npc_name, npc_type FROM npcs WHERE lower(trim(npc_name)) LIKE "
                                   "'rehabilitator %' ORDER BY id").fetchall():
        if (t or 0) & NPC_BIT == 0:
            p["npcs"].append((nid, name, t or 0))
    epics = epic_quests(db)
    for qid, key, step, items in epics:
        if not any(pid == ROSE_ID for pid, _ in item_ids(items)):
            new = ", ".join(x for x in [items.strip(), str(ROSE_ID)] if x)
            p["rewards"].append((qid, key, step, items, new))
    if has_table(db, "charquests") and epics:
        ids = ",".join(str(e[0]) for e in epics)
        given_before = {}
        if has_table(db, "rehab_seed"):
            for (new,) in db.execute("SELECT new FROM rehab_seed WHERE tbl = 'charinventory'"):
                sid = json.loads(new)["serverid"]
                given_before[sid] = given_before.get(sid, 0) + 1
        used = {}
        if has_table(db, "charresets"):
            used = dict(db.execute("SELECT serverid, paid FROM charresets WHERE kind = 'path'").fetchall())
        for sid, done in db.execute(f"SELECT serverid, count(*) FROM charquests WHERE questid IN ({ids}) AND done > 0 "
                                    "GROUP BY serverid ORDER BY serverid").fetchall():
            held = db.execute("SELECT COALESCE(SUM(stackleft), 0) FROM charinventory WHERE serverid = ? AND patternid = ?",
                              (sid, ROSE_ID)).fetchone()[0]
            owed = done - held - given_before.get(sid, 0) - used.get(sid, 0)
            if owed > 0:
                p["given"].append((sid, owed))
    return p


def log(db, tbl, rid, col, old, new):
    db.execute("INSERT INTO rehab_seed (tbl, rid, col, old, new) VALUES (?, ?, ?, ?, ?)",
               (tbl, str(rid), col, None if old is None else json.dumps(old), None if new is None else json.dumps(new)))


def apply(db, p):
    db.execute(LOG_DDL)
    n = {"items": 0, "npcs": 0, "rewards": 0, "given": 0}
    if p["rose"] is not None:
        rose = p["rose"]
        cols = list(rose)
        db.execute(f"INSERT INTO itempattern ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [rose[c] for c in cols])
        log(db, "itempattern", ROSE_ID, "*", None, rose)
        n["items"] += 1
    for nid, _, t in p["npcs"]:
        db.execute("UPDATE npcs SET npc_type = ? WHERE id = ?", (t | NPC_BIT, nid))
        log(db, "npcs", nid, "npc_type", t, t | NPC_BIT)
        n["npcs"] += 1
    for qid, _, step, old, new in p["rewards"]:
        db.execute("UPDATE queststeps SET rewarditems = ? WHERE questid = ? AND step = ?", (new, qid, step))
        log(db, "queststeps", f"{qid}/{step}", "rewarditems", old, new)
        n["rewards"] += 1
    for sid, owed in p["given"]:
        for _ in range(owed):
            cur = db.execute(
                "INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, "
                "listnumber) VALUES (?, 1, 0, 0, ?, -1, 0, (SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory "
                "WHERE serverid = ? AND location = 0))", (sid, ROSE_ID, sid))
            log(db, "charinventory", cur.lastrowid, "*", None, {"serverid": sid, "patternid": ROSE_ID})
            n["given"] += 1
    return n


def undo(db):
    if not has_table(db, "rehab_seed"):
        print("nothing logged: nothing to undo")
        return
    back = kept = 0
    for _, tbl, rid, col, old, new in db.execute(
            "SELECT seq, tbl, rid, col, old, new FROM rehab_seed ORDER BY seq DESC").fetchall():
        old = json.loads(old) if old else None
        new = json.loads(new) if new else None
        ok = False
        if tbl == "itempattern":
            if row(db, "SELECT * FROM itempattern WHERE patternid = ?", (int(rid),)) == new:
                db.execute("DELETE FROM itempattern WHERE patternid = ?", (int(rid),))
                ok = True
        elif tbl == "npcs":
            ok = db.execute("UPDATE npcs SET npc_type = ? WHERE id = ? AND npc_type = ?", (old, int(rid), new)).rowcount > 0
        elif tbl == "queststeps":
            q, s = rid.split("/")
            ok = db.execute("UPDATE queststeps SET rewarditems = ? WHERE questid = ? AND step = ? AND rewarditems = ?",
                            (old, int(q), int(s), new)).rowcount > 0
        elif tbl == "charinventory":
            ok = db.execute("DELETE FROM charinventory WHERE itemid = ? AND serverid = ? AND patternid = ?",
                            (int(rid), new["serverid"], new["patternid"])).rowcount > 0
        back, kept = (back + 1, kept) if ok else (back, kept + 1)
    db.execute("DROP TABLE rehab_seed")
    print(f"put back {back}; left {kept} changed since")


def write_data(folder):
    """The rose among the rewards of the last step of every epic's reward quest in 60.json, the file's own layout.
    The number of quests changed."""
    path = os.path.join(folder, "60.json")
    with open(path, encoding="utf-8") as f:
        raw = f.read()
    data = json.loads(raw)
    if json.dumps(data, indent=1, ensure_ascii=False) + "\n" != raw:
        sys.exit(f"{path} is not in json.dump's indent-1 layout: not rewritten")
    changed = 0
    for q in data["quests"]:
        if not EPIC_FILE_KEY.match(q.get("key", "")) or not q.get("steps"):
            continue
        rewards = q["steps"][-1].setdefault("rewards", {})
        items = rewards.setdefault("items", [])
        if ROSE_NAME not in items:
            items.append(ROSE_NAME)
            changed += 1
    out = json.dumps(data, indent=1, ensure_ascii=False) + "\n"
    if out != raw:
        with open(path, "w", encoding="utf-8") as f:
            f.write(out)
    return changed


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", default="eqoa.db")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    ap.add_argument("--write-data", metavar="DIR", help="name the rose in DIR/60.json's epic rewards")
    a = ap.parse_args()
    if a.write_data:
        print(f"{a.write_data}/60.json: the rose added to {write_data(a.write_data)} epic rewards")
        if not (a.apply or a.undo):
            return
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 10000")
    if a.undo:
        db.execute("BEGIN IMMEDIATE")
        undo(db)
        db.execute("COMMIT")
        return
    p = plan(db)
    if p["rose"] is not None:
        print(f"item {ROSE_ID} {ROSE_NAME!r}: icon {p['rose']['itemicon']}, worth {ROSE_VALUE}, no trade")
    for nid, name, t in p["npcs"]:
        print(f"npc {nid} {name!r}: npc_type {t} -> {t | NPC_BIT} (talks)")
    for qid, key, step, old, new in p["rewards"]:
        print(f"quest {qid} {key} step {step}: rewards {old!r} -> {new!r}")
    for sid, owed in p["given"]:
        print(f"character {sid}: given {owed} rose(s) for epics finished before")
    for x in p["problems"]:
        print("PROBLEM:", x)
    if not a.apply:
        print("report only: --apply to write")
        return
    if p["problems"]:
        sys.exit("refused: the problems above first")
    db.execute("BEGIN IMMEDIATE")
    n = apply(db, p)
    db.execute("COMMIT")
    print(f"wrote {n['items']} items, {n['npcs']} npcs, {n['rewards']} rewards, {n['given']} given; "
          "the npcs and quests at the server's next start")


if __name__ == "__main__":
    main()
