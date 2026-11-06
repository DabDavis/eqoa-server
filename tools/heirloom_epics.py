#!/usr/bin/env python3
"""The heirloom weapons as epics (the user, 2026-11-06: "the heirloom weapons for every class lets make their lvl 60
epics. also add extra choices of weapons to choose from that are epic also. including some level 50 epics also", then
"also the bow heirloom needs to be epic also"; petition 1023: "Rogue heirloom dagger is a basic dagger. Can it be changed
to the rogue 60 weapon to match other classes?"). Over the heirlooms tools/seed_heirlooms.py wrote.

What it writes (a report by default; --apply writes, --undo takes back exactly what the last run wrote):
- **Each class's heirloom weapons become epics** (heirlooms.epic, handlers/heirloom.go): from the epic's own level (60;
  49 for the path quests' epics) the wearer's view of the record is that epic's figures and its proc, below it the best
  of its role as before. The records keep their ids (characters hold them) and take the epic's name ("Heirloom Sword of
  Virtue"), look and attack type. The level 60 epics are the reward quests' (60/<archetype>/reward-<class>) and the
  classes' own epic-tier weapons of level 60 (the paladin's Sword of Virtue, the shadow knight's Sword of the
  Contender, the ranger's Blades of Nature and Quicksilver longbow, the druid's Cosmos pair, the monk's Mountain Path);
  the warrior, rogue, necromancer, enchanter and alchemist have none, so they take their archetype's. The shadow
  knight's two-hander keeps the Sword of Dreams' look the user chose and is the tanks' great sword from 60; the rogue's
  dagger keeps piercing (its backstabs need it), its look now the epics' blade, the best of its kind at every level.
- **Extra epic choices**: every class's Heirloom Outfitter also sells, as that class's heirlooms, its archetype's other
  level 60 epic weapons and its own level 49 ("50") epics (the path quests' 49 rewards; a caster with none of its own,
  the magician's). New records from 714300.
- The outfitters' stock: each new record added to the outfitters already selling its class's set.
- **Every heirloom's durability 80** (itempattern.duration, the item window's DUR; the user, the same night: "lets make
  heirloom items 80 DUR"), the armour and jewellery too: they were their looks' (50 to 80).

The records are NO TRADE, worth nothing, level 1, the class's alone, as seed_heirlooms.py's. Data only: the server reads
heirlooms again within ten minutes (its cache), merchants' stock at once, and a client the records at its next login.
Logged in heirloom_epics_log; --undo puts every changed column back and deletes what it made (a record a character
holds is kept, and said).

    tools/heirloom_epics.py [--db eqoa.db] [--list] [--apply | --undo]
"""

import argparse
import datetime
import json
import os
import shutil
import sqlite3

LOG = "heirloom_epics_log"
FIRST_NEW = 714300
CLASSES = ["warrior", "ranger", "paladin", "shadowknight", "monk", "bard", "rogue", "druid", "shaman", "cleric",
           "magician", "necromancer", "enchanter", "wizard", "alchemist"]
DUAL = {0, 1, 4, 5, 6}  # CLIENT's off hand: warrior, ranger, monk, bard, rogue: a one-hander either hand (14)
EPIC_LOOK = (1205436896, 445518418)  # the level 60 epics' blade: model, icon (the bard's, ranger's, knight's, monk's)

# the epics by name: (record id, short heirloom name, role); the role is the hands it takes
E = {
    "virtue": (1081, "Heirloom Sword of Virtue", "main"),
    "contender": (5379, "Heirloom Sword of the Contender", "main"),
    "challenger": (5373, "Heirloom Sword of the Challenger", "twohand"),
    "refrain": (5369, "Heirloom Lyrical Refrain", "main"),
    "harmony": (5368, "Heirloom Lyrical Harmony", "main"),
    "axe_innocence": (5353, "Heirloom Axe of Innocence", "main"),
    "axe_purity": (5354, "Heirloom Axe of Purity", "main"),
    "blade_innocence": (5355, "Heirloom Blade of Innocence", "main"),
    "blade_purity": (5356, "Heirloom Blade of Purity", "main"),
    "quicksilver": (5359, "Heirloom Quicksilver Longbow", "ranged"),
    "insurrection": (701197, "Heirloom Claws of Insurrection", "main"),
    "mayhem": (701198, "Heirloom Claws of Mayhem", "main"),
    "mountain": (5348, "Heirloom Mountain Path", "twohand"),
    "divinity": (701193, "Heirloom Staff of Divinity", "twohand"),
    "ethereal": (701194, "Heirloom Ethereal Hammer", "main"),
    "nature": (701195, "Heirloom Staff of Nature", "twohand"),
    "bedlam": (701196, "Heirloom Bedlam", "main"),
    "spiritual": (701199, "Heirloom Spiritual Staff", "twohand"),
    "mystical": (701200, "Heirloom Mystical Hammer", "main"),
    "cosmos_scimitar": (5386, "Heirloom Scimitar of the Cosmos", "main"),
    "cosmos_sickle": (5387, "Heirloom Sickle of the Cosmos", "main"),
    "summoner": (5392, "Heirloom Staff of the Summoner", "twohand"),
    "enlightened": (5390, "Heirloom Enlightened Staff", "twohand"),
    # the path quests' level 49 epics
    "steelwind": (1026, "Heirloom Steelwind", "main"),
    "foereaver": (5631, "Heirloom Foereaver", "twohand"),
    "judgement": (700984, "Heirloom Judgement", "twohand"),
    "virtus": (1083, "Heirloom Virtus Ferrum", "twohand"),
    "beheader": (5378, "Heirloom Beheader of Isthiak", "twohand"),
    "downfall": (1167, "Heirloom Downfall", "twohand"),
    "ballad": (5370, "Heirloom Ballad of Mithril", "main"),
    "selo": (8892, "Heirloom Selo's Jeweled Rapier", "main"),
    "emerald": (700913, "Heirloom Pure Emerald Longsword", "main"),
    "fury": (701006, "Heirloom Scimitar of Fury", "main"),
    "ascendant": (5340, "Heirloom Ascendant Staff", "twohand"),
    "sunsilver": (701007, "Heirloom Sunsilver Blade", "twohand"),
    "heartsbane": (5996, "Heirloom Heartsbane", "main"),
    "blacktongue": (700995, "Heirloom Blacktongue", "main"),
    "absolution": (700860, "Heirloom Hammer of Absolution", "main"),
    "repentence": (700985, "Heirloom Mace of Repentence", "main"),
    "eilerawdl": (701008, "Heirloom Oak Staff of Eilerawdl", "main"),
    "triad": (700912, "Heirloom Triad Scimitar", "main"),
    "cudgel": (5383, "Heirloom Spirit Guide Cudgel", "main"),
    "fate": (700996, "Heirloom Spear of Fate", "main"),
    "crosier": (5393, "Heirloom Crystalmana Crosier", "twohand"),
    "union": (701009, "Heirloom Rod of Elemental Union", "twohand"),
    "escron": (5388, "Heirloom Sickle of Avhi Escron", "main"),
}

MELEE60 = ["refrain", "harmony", "axe_innocence", "axe_purity", "insurrection", "mayhem"]
HEALER60 = ["divinity", "ethereal", "nature", "bedlam", "spiritual", "mystical"]
CASTER60 = ["summoner", "enlightened"]
CASTER49 = ["crosier", "union"]

# each class: its existing heirloom weapons (record id -> the epic it becomes; None: kept the best of its kind, a look
# given), then every new choice, in the order the outfitter lists them
PLAN = {
    0: ({714013: "contender"}, ["virtue", "challenger", "steelwind", "foereaver"]),
    # the ranger's: the Scimitar of Fury (the user, with its picture: "this is scimitar of fury, the ranger epic. lets
    # make the ranger heirloom that. right now it is the rogue epic"), the axes and blades among its choices
    1: ({714027: "fury", 714028: "quicksilver"},
        ["axe_innocence", "axe_purity", "blade_innocence", "blade_purity", "refrain", "harmony", "insurrection", "mayhem",
         "emerald"]),
    2: ({714042: "virtue"}, ["contender", "challenger", "judgement", "virtus"]),
    3: ({714056: "contender", 714057: "challenger"}, ["virtue", "beheader", "downfall"]),
    4: ({714071: "insurrection", 714072: "mountain"},
        ["mayhem", "refrain", "harmony", "axe_innocence", "axe_purity", "ascendant", "sunsilver"]),
    5: ({714086: "harmony"},
        ["refrain", "axe_innocence", "axe_purity", "insurrection", "mayhem", "ballad", "selo"]),
    6: ({714100: None}, MELEE60 + ["heartsbane", "blacktongue"]),
    7: ({714114: "nature", 714115: "cosmos_scimitar"},
        ["bedlam", "cosmos_sickle", "divinity", "ethereal", "spiritual", "mystical", "eilerawdl", "triad"]),
    8: ({714129: "spiritual", 714130: "mystical"}, ["divinity", "ethereal", "nature", "bedlam", "cudgel", "fate"]),
    9: ({714144: "divinity", 714145: "ethereal"},
        ["nature", "bedlam", "spiritual", "mystical", "absolution", "repentence"]),
    10: ({714159: "summoner"}, ["enlightened"] + CASTER49),
    11: ({714173: "enlightened"}, ["summoner", "escron"] + CASTER49),
    12: ({714187: "enlightened"}, ["summoner"] + CASTER49),
    13: ({714201: "enlightened"}, ["summoner"] + CASTER49),
    14: ({714215: "summoner"}, ["enlightened"] + CASTER49),
}
# the converted records whose look stays their own: the shadow knight's two-hander, the Sword of Dreams' (the user's)
KEEP_LOOK = {714057: "Heirloom Sword of Dreams"}
# a converted record's look from another record (model, icon, colour): the ranger's bow, the disc's most ornate bow
# (the user: "lets give heirloom bow an epic ranger bow look that is on disc and make range 55"; ITEM.ESF's 200
# models hold five bows, none glowing: the Bow of the Elder Elves' 0x361DAC84, pale wood and a red grip, and the
# Quicksilver's 0xDEBA8F46, silver bands, are the two of note)
LOOK_FROM_NAME = {714028: "Bow of the Elder Elves"}
# new heirlooms of a look and no epic (the best of their kind at every level): the ranger's second bow, the
# Quicksilver's look ("if you find other epic looking bows also add those as another heirloom bow")
PLAIN = {1: [("ranged", 16, "Heirloom Longbow", 5359)]}
# a ranged heirloom's reach (itempattern.unk5), kept over its best's or epic's shorter (handlers/heirloom.go)
RANGE = 55
ROGUE_DAGGER = 714100

WEAR = {"main": None, "twohand": 15, "ranged": 16}  # main: 14 for a dual wielder, else 12
DUR = 80  # every heirloom's durability (the user: "lets make heirloom items 80 DUR")


DESC_PLAIN = "Heirloom gear for testing: always the best of its kind your class can wear at your level."


def desc_of(epic_name, level):
    return (f"Heirloom gear for testing: always the best of its kind your class can wield at your level, and from "
            f"level {level} the {epic_name} itself.")


def rows_of(db, sql, *args):
    cur = db.execute(sql, args)
    cols = [d[0] for d in cur.description]
    return [dict(zip(cols, r)) for r in cur.fetchall()]


def ensure(db):
    if not db.execute("SELECT COUNT(*) FROM pragma_table_info('heirlooms') WHERE name = 'epic'").fetchone()[0]:
        db.execute("ALTER TABLE heirlooms ADD COLUMN epic INTEGER NOT NULL DEFAULT 0")
    db.execute(f"""CREATE TABLE IF NOT EXISTS {LOG} (seq INTEGER PRIMARY KEY, run TEXT NOT NULL, tbl TEXT NOT NULL,
                   rowkey INTEGER NOT NULL, action TEXT NOT NULL, old TEXT NOT NULL DEFAULT '')""")


def plan(db):
    for n in (name for _, name, _ in E.values()):
        assert len(n) <= 32, n
    if db.execute(f"SELECT COUNT(*) FROM {LOG}").fetchone()[0]:
        raise SystemExit("the epics are in this database already: --undo first")
    epics = {k: (rows_of(db, "SELECT * FROM itempattern WHERE patternid = ?", v[0]) or [None])[0] for k, v in E.items()}
    missing = [f"{k} ({E[k][0]})" for k, r in epics.items() if r is None]
    if missing:
        raise SystemExit(f"epic records missing: {', '.join(missing)}")
    held = {r["patternid"]: r for r in rows_of(db, "SELECT h.patternid, h.class, h.role, p.* FROM heirlooms h "
                                                    "JOIN itempattern p ON p.patternid = h.patternid")}
    cols = [r[1] for r in db.execute("PRAGMA table_info(itempattern)")]
    changes, made, nid = [], [], FIRST_NEW
    if db.execute("SELECT COUNT(*) FROM itempattern WHERE patternid >= ? AND patternid < 715000", (FIRST_NEW,)).fetchone()[0]:
        raise SystemExit(f"records already hold ids from {FIRST_NEW}")
    for cls, (convert, extra) in PLAN.items():
        for pid, key in convert.items():
            h = held.get(pid)
            if h is None or h["class"] != cls:
                raise SystemExit(f"heirloom {pid} is not {CLASSES[cls]}'s: {h and (h['class'], h['role'])}")
            if key is None:  # the rogue's dagger: the epics' look, its piercing kept, the best of its kind
                changes.append(dict(pid=pid, cls=cls, epic=0, set={"model": EPIC_LOOK[0], "itemicon": EPIC_LOOK[1]},
                                    note=f"{CLASSES[cls]} {h['itemname']}: the epics' blade look, piercing kept"))
                continue
            eid, name, role = E[key]
            e = epics[key]
            if role != h["role"]:
                raise SystemExit(f"{pid} is a {h['role']}, {key} a {role}")
            s = {"itemname": KEEP_LOOK.get(pid, name), "itemdesc": desc_of(e["itemname"], e["levelreq"]),
                 "attacktype": e["attacktype"]}
            if pid in LOOK_FROM_NAME:
                lk = (rows_of(db, "SELECT model, itemicon, color FROM itempattern WHERE itemname = ? LIMIT 1",
                              LOOK_FROM_NAME[pid]) or [None])[0]
                if lk is None:
                    raise SystemExit(f"no record named {LOOK_FROM_NAME[pid]!r} for {pid}'s look")
                s.update(model=lk["model"], itemicon=lk["itemicon"], color=lk["color"])
            elif pid not in KEEP_LOOK:
                s.update(model=e["model"], itemicon=e["itemicon"], color=e["color"])
            if role == "ranged":
                s["unk5"] = RANGE
            changes.append(dict(pid=pid, cls=cls, epic=eid, set=s, note=f"{CLASSES[cls]} {h['itemname']} -> {s['itemname']}"
                                                                         f" ({e['itemname']} {eid} from {e['levelreq']})"))
        for key in extra:
            eid, name, role = E[key]
            e = epics[key]
            slot = WEAR[role] if WEAR[role] is not None else (14 if cls in DUAL else 12)
            r = dict(e)
            for c in ("str", "sta", "agi", "dex", "wis", "intelligence", "cha", "HPMAX", "POWMAX", "PoT", "HoT", "AC",
                      "PR", "DR", "FR", "CR", "LR", "AR"):
                if c in r:
                    r[c] = None
            r.update(patternid=nid, itemname=name, itemdesc=desc_of(e["itemname"], e["levelreq"]), itemcost=0, trade=1,
                     rent=1, levelreq=1, maxstack=1, classuse=1 << cls, raceuse=32767, lore=0, craft=0, equipslot=slot,
                     weapondamage=0, duration=DUR)
            for c in ("weaponproc", "fish"):
                if c in r:
                    r[c] = 0
            if role == "ranged":
                r["unk5"] = RANGE
            made.append(dict(rec={c: r.get(c) for c in cols}, cls=cls, role=role, epic=eid,
                             note=f"{CLASSES[cls]} {name} ({e['itemname']} {eid} from {e['levelreq']})"))
            nid += 1
        for role, slot, name, look_id in PLAIN.get(cls, []):
            look = (rows_of(db, "SELECT * FROM itempattern WHERE patternid = ?", look_id) or [None])[0]
            if look is None:
                raise SystemExit(f"no record {look_id} for {name}'s look")
            r = dict(look)
            for c in ("str", "sta", "agi", "dex", "wis", "intelligence", "cha", "HPMAX", "POWMAX", "PoT", "HoT", "AC",
                      "PR", "DR", "FR", "CR", "LR", "AR"):
                if c in r:
                    r[c] = None
            r.update(patternid=nid, itemname=name, itemdesc=DESC_PLAIN, itemcost=0, trade=1, rent=1, levelreq=1,
                     maxstack=1, classuse=1 << cls, raceuse=32767, lore=0, craft=0, equipslot=slot, weapondamage=0,
                     duration=DUR, unk5=RANGE if role == "ranged" else r.get("unk5"))
            for c in ("weaponproc", "fish"):
                if c in r:
                    r[c] = 0
            made.append(dict(rec={c: r.get(c) for c in cols}, cls=cls, role=role, epic=0,
                             note=f"{CLASSES[cls]} {name} (the look of {look['itemname']} {look_id}, the best at every level)"))
            nid += 1
    # the outfitters selling each class's set
    stock = {}
    for npc, pid, listn in db.execute("SELECT i.npcid, i.patternid, i.listnumber FROM npcitem i JOIN npcs n ON n.npc_id = i.npcid "
                                      "WHERE n.npc_name = 'Heirloom Outfitter'"):
        stock.setdefault(npc, {"pids": set(), "last": -1})
        stock[npc]["pids"].add(pid)
        stock[npc]["last"] = max(stock[npc]["last"], listn)
    sells = []
    for npc, s in sorted(stock.items()):
        classes = sorted({held[p]["class"] for p in s["pids"] if p in held})
        n = s["last"]
        for m in made:
            if m["cls"] in classes:
                n += 1
                sells.append((npc, m["rec"]["patternid"], n))
    durs = [pid for pid, h in sorted(held.items()) if h["duration"] != DUR]
    return dict(changes=changes, made=made, sells=sells, outfitters=len(stock), durs=durs)


def report(p, full):
    print(f"{len(p['changes'])} heirloom weapons changed, {len(p['made'])} epic choices made from {FIRST_NEW}, "
          f"{len(p['sells'])} stock rows over {p['outfitters']} outfitters, {len(p['durs'])} heirlooms to {DUR} DUR")
    if full:
        for c in p["changes"]:
            print("  change", c["note"])
        for m in p["made"]:
            print("  new", m["rec"]["patternid"], m["note"])


def apply(db, p):
    run = datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")
    db.execute("BEGIN IMMEDIATE")
    try:
        log = lambda tbl, key, action, old="": db.execute(
            f"INSERT INTO {LOG} (run, tbl, rowkey, action, old) VALUES (?, ?, ?, ?, ?)", (run, tbl, key, action, old))
        for c in p["changes"]:
            cols = list(c["set"])
            old = db.execute(f"SELECT {', '.join(cols)} FROM itempattern WHERE patternid = ?", (c["pid"],)).fetchone()
            log("itempattern", c["pid"], "update", json.dumps(dict(zip(cols, old))))
            db.execute(f"UPDATE itempattern SET {', '.join(f'{k} = ?' for k in cols)} WHERE patternid = ?",
                       [c["set"][k] for k in cols] + [c["pid"]])
            old_epic = db.execute("SELECT epic FROM heirlooms WHERE patternid = ?", (c["pid"],)).fetchone()[0]
            log("heirlooms", c["pid"], "update", json.dumps({"epic": old_epic}))
            db.execute("UPDATE heirlooms SET epic = ? WHERE patternid = ?", (c["epic"], c["pid"]))
        for pid in p["durs"]:
            old = db.execute("SELECT duration FROM itempattern WHERE patternid = ?", (pid,)).fetchone()[0]
            log("itempattern", pid, "update", json.dumps({"duration": old}))
            db.execute("UPDATE itempattern SET duration = ? WHERE patternid = ?", (DUR, pid))
        for m in p["made"]:
            rec = m["rec"]
            cols = list(rec)
            db.execute(f"INSERT INTO itempattern ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                       [rec[k] for k in cols])
            log("itempattern", rec["patternid"], "insert")
            db.execute("INSERT INTO heirlooms (patternid, class, role, epic) VALUES (?, ?, ?, ?)",
                       (rec["patternid"], m["cls"], m["role"], m["epic"]))
            log("heirlooms", rec["patternid"], "insert")
        for npc, pid, listn in p["sells"]:
            cur = db.execute("INSERT INTO npcitem (npcid, stackleft, patternid, equiploc, location, listnumber) "
                             "VALUES (?, 1, ?, 0, 1, ?)", (npc, pid, listn))
            log("npcitem", cur.lastrowid, "insert")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"wrote it (run {run}): no restart needed; the records reach a client at its next login")


def undo(db):
    rows = db.execute(f"SELECT tbl, rowkey, action, old FROM {LOG} ORDER BY seq DESC").fetchall()
    if not rows:
        raise SystemExit("nothing to undo")
    kept = []
    db.execute("BEGIN IMMEDIATE")
    try:
        for tbl, key, action, old in rows:
            if tbl == "npcitem":
                db.execute("DELETE FROM npcitem WHERE itemid = ?", (key,))
            elif tbl == "heirlooms" and action == "insert":
                if db.execute("SELECT COUNT(*) FROM charinventory WHERE patternid = ?", (key,)).fetchone()[0]:
                    continue
                db.execute("DELETE FROM heirlooms WHERE patternid = ?", (key,))
            elif tbl == "itempattern" and action == "insert":
                if db.execute("SELECT COUNT(*) FROM charinventory WHERE patternid = ?", (key,)).fetchone()[0]:
                    kept.append(key)
                    continue
                db.execute("DELETE FROM itempattern WHERE patternid = ?", (key,))
            elif action == "update":
                vals = json.loads(old)
                db.execute(f"UPDATE {tbl} SET {', '.join(f'{k} = ?' for k in vals)} WHERE patternid = ?",
                           list(vals.values()) + [key])
        db.execute(f"DELETE FROM {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"took back {len(rows)} rows" + (f"; kept {len(kept)} records characters hold: {kept}" if kept else ""))


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--list", action="store_true")
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--undo", action="store_true")
    ap.add_argument("--no-backup", action="store_true")
    a = ap.parse_args()
    if not os.path.isfile(a.db):
        raise SystemExit(f"no database at {a.db}")
    db = sqlite3.connect(a.db, isolation_level=None, timeout=10)
    if (a.apply or a.undo) and not a.no_backup:
        stamp = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
        db.execute(f"VACUUM INTO '{a.db}.bak-{stamp}-pre-heirloom-epics'")
    ensure(db)
    if a.undo:
        undo(db)
        return
    p = plan(db)
    report(p, a.list)
    if a.apply:
        apply(db, p)


if __name__ == "__main__":
    main()
