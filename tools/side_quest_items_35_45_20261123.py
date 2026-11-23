#!/usr/bin/env python3
"""The level 35-45 side quests' class items: every quest of the guides' 23-47 family at levels 35-45 that gave no item
(Live petitions 5522, 5523, 5525, 5527, 5528, 5529, 5534, 5536 and 5538; the owner, 2026-11-23: "Yes, all of them").

tools/lvl35_item_rewards_20261123.py gave Highpass Quests 1, 2, 3 and 5 (100775-100777, 100779) an item a class.
This gives the other eleven of the family at 35-45 one the same way: Highpass Quests 4 and 6 (The Highpass Road,
Rin's Clockworks), the Oasis level-37 three (Min's Treasure, Poisontongue, Svenle's Debts), the Eternal Desert
level-43 three (The Monk Tower, The Three Trials, The Restless) and the Castle Lightwolf level-45 three (Old Man
Antiok, Three Ambassadors, The Sword of Olivia McFadden). EQOA-Data Quests/23-47/37.json, 43.json and 45.json name
"item" in the ability quests' rewards (Quest 1 of each) and "rewards you" in the rest, naming none.

The picks follow the lvl35 tool's rule, at each quest's own level L: the class's own records of levels L-2..L+2, worn
somewhere (equipslot above 0, no stack), not GM-made (id below 1,000,000), that nothing else gives (no loot table,
pool, NPC's held item, NPC item list, quest reward, craft recipe, token shop or start item) and no character holds,
ranked: the fewest classes in the mask, nearest L, a belt, then bracers, gloves, boots, chest, legs, head, the rest,
then the record's id; left out besides 6629 (as the lvl35 tool did): 5596 "Deadwood Staff Lvl 47" and 5759 "Moss
Covered Mace (lvl 45)" (import labels in the name) and 8254 "Protecotrate Ring" (the misspelt twin of 2290). At level
35 Quest 4 takes each class's fifth pick and Quest 6 its sixth (the first four are the lvl35 tool's); at 37, 43 and
45 the quests take the first, second and third in the guide's order; a record picked at one level is not picked again
at another for that class (the ranges overlap). The picks below were made so on Live's data of 2026-11-23 (rule
re-checked: it gives the lvl35 tool's 60 picks exactly).

Each quest's last step (found as the LAST step, its kind and NPC checked) gets the items in its byclass, beside any
spells there (the ability quests'). Each character who finished one before this (charquests done) and holds none of
its item is given it at its next login, as a GM grant: one approved chartransfers row a character ("GM grant (side
quests 35-45)"), the items of every such quest in it. A character's approved rows apply one a login, oldest first.
Refused, nothing written, if a quest, step or record is not as read here, or a class's cell already gives an item.

Quests are read at a start or a GM panel quest reload; transfers at the character's login. Dry run by default;
--apply writes in one transaction, a gm_audit line a row, before and after in side_quest_items_1123_log; a second
--apply writes nothing; --undo puts back what the log holds (a grant already applied stays given).

    python3 -E -s tools/side_quest_items_35_45_20261123.py --db eqoa.db --shard live [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "side_quest_items_1123_log"
WHO, ACCOUNT, VIA = "dab", 1, "tool"
TOOL = "tools/side_quest_items_35_45_20261123.py"
GRANT = "GM grant (side quests 35-45)"
DETAIL = f"the owner, 2026-11-23 (Live petitions 5522-5538): the level 35-45 side quests' class items: {TOOL}"

# (quest id, name, its last step's kind and NPC)
QUESTS = [(100778, "The Highpass Road", "talk", "Lord Voriander"),
          (100780, "Rin's Clockworks", "give", "Rin Tintoggle"),
          (100781, "Min's Treasure", "reach", ""),
          (100782, "Poisontongue", "talk", "Rigo Dartar"),
          (100783, "Svenle's Debts", "talk", "Verin Dormio"),
          (100784, "The Monk Tower", "talk", "Royce Sandstalker"),
          (100785, "The Three Trials", "talk", "Joyce Sandstalker"),
          (100786, "The Restless", "talk", "Broyce Sandstalker"),
          (100787, "Old Man Antiok", "talk", "Gate Guard Chastis"),
          (100788, "Three Ambassadors", "give", "Ambassador Kizden G'Lux"),
          (100789, "The Sword of Olivia McFadden", "give", "Pandrig Fultin")]
CLASS_NAMES = ["Warrior", "Ranger", "Paladin", "Shadowknight", "Monk", "Bard", "Rogue", "Druid", "Shaman", "Cleric",
               "Magician", "Necromancer", "Enchanter", "Wizard", "Alchemist"]
# quest -> class -> (record, its name)
PICKS = {
    100778: {
        0: (6955, "Rowen'Dal Guard's Gloves"),
        1: (5635, 'Froglok Crossbow'),
        2: (6374, 'Imperial Helmet'),
        3: (7664, 'Blood Scale Earring'),
        4: (5006, 'pslignium robe of fire'),
        5: (6957, "Rowen'Dal Minstrel's Gloves"),
        6: (5914, 'Whiptail Shamshir'),
        7: (6166, "Mountain Giant Priest's Shield"),
        8: (6166, "Mountain Giant Priest's Shield"),
        9: (6676, "Rowen'Dal Templar Breastplate"),
        10: (8053, 'Disgusting Choker'),
        11: (6958, "Rowen'Dal Necromancer's Gloves"),
        12: (8053, 'Disgusting Choker'),
        13: (7832, 'Carved Ivory Bracelet'),
        14: (5147, 'Synergy Robe'),
    },
    100780: {
        0: (6673, "Rowen'Dal Guard's Breastplate"),
        1: (5667, 'Granite War Club'),
        2: (5785, 'Qeynos Halberd'),
        3: (5668, 'Gravecarver'),
        4: (5508, 'Barbed Emerald Staff'),
        5: (6674, "Rowen'Dal Minstrel's Tunic"),
        6: (5599, "Defender's Bow"),
        7: (7905, 'Protectorate Plate Bracers'),
        8: (7905, 'Protectorate Plate Bracers'),
        9: (6366, 'Helm of the Martyr'),
        10: (7104, "Orkol's Belt"),
        11: (6651, 'Nefarious Rags'),
        12: (7104, "Orkol's Belt"),
        13: (8053, 'Disgusting Choker'),
        14: (5204, 'Robe of the Eyes'),
    },
    100781: {
        0: (6383, 'Mithron Banded Helm'),
        1: (7852, 'Dustscale Bracers'),
        2: (6552, 'Battered Chestguard'),
        3: (7580, 'Sandstorm Boots'),
        4: (5926, "Worn Guard's Axe"),
        5: (6391, "Performer's Coif"),
        6: (7515, 'Dustscale Boots'),
        7: (5287, 'Tome of Eternal Secrets'),
        8: (5287, 'Tome of Eternal Secrets'),
        9: (3016, 'Protectorate Chain Boots'),
        10: (4110, 'Shroud of Elemental Flame'),
        11: (6323, 'Cowl of the Grave'),
        12: (7851, 'Dustsage Bracer'),
        13: (7851, 'Dustsage Bracer'),
        14: (1780, 'Nefarios Rags'),
    },
    100782: {
        0: (2668, 'Fyreblood Forged Greaves'),
        1: (7580, 'Sandstorm Boots'),
        2: (1340, 'sinhalite mithril bracers'),
        3: (6552, 'Battered Chestguard'),
        4: (6639, 'Loam Covered Tunic'),
        5: (7657, 'Azure Scale Earring'),
        6: (6601, 'Dustscale Mail'),
        7: (5158, 'Tidian Prophet Robe'),
        8: (5158, 'Tidian Prophet Robe'),
        9: (7905, 'Protectorate Plate Bracers'),
        10: (7851, 'Dustsage Bracer'),
        11: (5011, 'Raiment of the Grave'),
        12: (7514, 'Dustsage Boots'),
        13: (7514, 'Dustsage Boots'),
        14: (4958, "Embalmer's Smock"),
    },
    100783: {
        0: (7580, 'Sandstorm Boots'),
        1: (6341, 'Dustscale Helm'),
        2: (7590, 'Skirmishers Boots'),
        3: (1340, 'sinhalite mithril bracers'),
        4: (7270, 'Crushers Leggings'),
        5: (7852, 'Dustscale Bracers'),
        6: (7361, 'Shademail Greaves'),
        7: (4111, 'Nitemorrow Silk Shroud'),
        8: (4111, 'Nitemorrow Silk Shroud'),
        9: (5287, 'Tome of Eternal Secrets'),
        10: (7514, 'Dustsage Boots'),
        11: (7696, 'Gray Scale Earring'),
        12: (1778, "Beguiler's Gown"),
        13: (1778, "Beguiler's Gown"),
        14: (4955, 'Dustsage Robe'),
    },
    100784: {
        0: (7887, 'Mithron Banded Bracer'),
        1: (5981, 'Glowing Scimitar of the Woods'),
        2: (5681, 'Halbred of Faith'),
        3: (5504, 'Axe of the Umbral Knight'),
        4: (6636, 'Iron Skin Tunic'),
        5: (7896, "Performer's Bracers"),
        6: (6290, "Assassin's Mask"),
        7: (4114, 'Rotting Hide Robe'),
        8: (7057, 'Belt of Four WInds'),
        9: (6412, 'Silver Defenders Helm'),
        10: (4115, 'Fire Savant Robe'),
        11: (7487, 'Boots of the Grave'),
        12: (7857, 'Epurating Bracers'),
        13: (7857, 'Epurating Bracers'),
        14: (5845, 'Staff of Fire'),
    },
    100785: {
        0: (7549, 'Mithron Banded Boots'),
        1: (27753, 'Robe of the Gust'),
        2: (7309, 'Ice Champions Greaves'),
        3: (5681, 'Halbred of Faith'),
        4: (2674, 'Liteshimmer Iron Leggings'),
        5: (7564, "Performer's Boots"),
        6: (7260, "Bruinen's Kilt"),
        7: (1303, 'Tattered Leather Strap'),
        8: (4114, 'Rotting Hide Robe'),
        9: (5611, 'Dwarven Hammer of Grace'),
        10: (1243, 'staff of creation'),
        11: (7812, 'Bracelet of the Grave'),
        12: (6315, 'Clearwater Helm'),
        13: (6315, 'Clearwater Helm'),
        14: (4957, 'elemental shrouds'),
    },
    100786: {
        0: (7058, 'Belt of Giants'),
        1: (5648, 'Gleaming Claymore'),
        2: (7827, 'Bronze Shackles'),
        3: (7309, 'Ice Champions Greaves'),
        4: (2039, 'Slime Covered Bear Paw'),
        5: (6290, "Assassin's Mask"),
        6: (27753, 'Robe of the Gust'),
        7: (7137, 'Transcendant Belt'),
        8: (1303, 'Tattered Leather Strap'),
        9: (4114, 'Rotting Hide Robe'),
        10: (7857, 'Epurating Bracers'),
        11: (7064, 'Belt of the Deathwalker'),
        12: (6327, 'Crown of Provocation'),
        13: (6327, 'Crown of Provocation'),
        14: (1490, 'Green Silken Robe'),
    },
    100787: {
        0: (6311, 'Champions Helmet'),
        1: (6356, 'Golden Spiked Helm'),
        2: (6916, 'Heroic Gauntlets'),
        3: (6062, "Gamma's Mattock"),
        4: (7860, "Forest Kin's Bracers"),
        5: (8232, 'Harmonic Band'),
        6: (6936, "Nightstalkers' Gloves"),
        7: (6131, 'Aged Tower'),
        8: (7260, "Bruinen's Kilt"),
        9: (8243, 'Lifelight Band'),
        10: (6860, 'Blightmesh Gloves'),
        11: (6431, 'Tiara of Spirit'),
        12: (6860, 'Blightmesh Gloves'),
        13: (6860, 'Blightmesh Gloves'),
        14: (7883, 'Kinetic Bracelet'),
    },
    100788: {
        0: (6005, 'Longsword of Defiance'),
        1: (5698, 'Iceblade'),
        2: (7307, 'Heroic Greaves'),
        3: (1155, 'ancient runic belt'),
        4: (6422, 'Starfire Helm'),
        5: (27753, 'Robe of the Gust'),
        6: (7334, "Nightstalkers' Leggings"),
        7: (7860, "Forest Kin's Bracers"),
        8: (6131, 'Aged Tower'),
        9: (6710, "Tempest Knight's Breast Plate"),
        10: (6940, 'Niteshade Gloves'),
        11: (5201, 'Ravenblack Robe'),
        12: (6940, 'Niteshade Gloves'),
        13: (6940, 'Niteshade Gloves'),
        14: (8241, 'Kinetic Band'),
    },
    100789: {
        0: (6929, 'Mighty Gauntlets'),
        1: (7506, 'Doomscale Boots'),
        2: (6710, "Tempest Knight's Breast Plate"),
        3: (5620, "Executioner's Axe"),
        4: (5739, 'Mammoth Tusk Club'),
        5: (6356, 'Golden Spiked Helm'),
        6: (8247, "Nightstalkers' Band"),
        7: (4100, 'Wormsilk Robe'),
        8: (4100, 'Wormsilk Robe'),
        9: (6131, 'Aged Tower'),
        10: (7475, 'Blightmesh Boots'),
        11: (5842, 'Staff of Blood'),
        12: (7475, 'Blightmesh Boots'),
        13: (7475, 'Blightmesh Boots'),
        14: (7860, "Forest Kin's Bracers"),
    },
}

LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    rid    INTEGER NOT NULL,
    rowkey TEXT NOT NULL,
    before TEXT,
    after  TEXT,
    audit  INTEGER NOT NULL,
    at     TEXT NOT NULL
)"""


class Refused(Exception):
    pass


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def dumps(v):
    return json.dumps(v, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def rows(db, tbl, where):
    w = " AND ".join(f'"{c}" = ?' for c in where) or "1"
    cur = db.execute(f'SELECT rowid AS _rowid_, * FROM "{tbl}" WHERE {w} ORDER BY rowid', tuple(where.values()))
    names = [d[0] for d in cur.description]
    return [dict(zip(names, r)) for r in cur.fetchall()]


def plain(r):
    return None if r is None else {k: v for k, v in r.items() if k != "_rowid_"}


def same(a, b):
    if isinstance(a, (int, float)) and isinstance(b, (int, float)) and not isinstance(a, bool):
        return abs(float(a) - float(b)) < 1e-6
    return a == b


def num(v):
    return int(v) if float(v).is_integer() else v


class Plan:
    def __init__(self, db):
        self.db = db
        self.ops = []  # (tbl, rid or None, before or None, after values)
        self.lines = []

    def say(self, s):
        self.lines.append(f"  {s}")

    def add(self, tbl, values, match, what):
        have = rows(self.db, tbl, {c: values[c] for c in match})
        if have:
            if len(have) == 1 and all(same(have[0].get(c), v) for c, v in values.items()):
                self.say(f"{what}: already there")
                return
            raise Refused(f"{what}: {tbl} has {dumps([plain(h) for h in have])[:300]}, not {dumps(values)[:300]}")
        self.ops.append((tbl, None, None, dict(values)))
        self.say(f"{what}: insert {tbl}")

    def change(self, tbl, row, was, now_, what):
        """Set now_'s columns on row, which must read was (or now_ already)."""
        if all(same(row.get(c), v) for c, v in now_.items()):
            self.say(f"{what}: already so")
            return
        for c, v in was.items():
            if not same(row.get(c), v):
                raise Refused(f"{what}: {c} reads {row.get(c)!r}, not {v!r}")
        self.ops.append((tbl, row["_rowid_"], plain(row), dict(now_)))
        self.say(f"{what}: {dumps({c: row.get(c) for c in now_})[:160]} -> {dumps(now_)[:160]}")



def build(p, shard):
    db = p.db
    for t in ("quests", "queststeps", "itempattern", "charquests", "characters", "charinventory", "chartransfers",
              "gm_audit"):
        if not table(db, t):
            raise Refused(f"no table {t}")
    for qid, cells in PICKS.items():
        if sorted(cells) != list(range(15)):
            raise Refused(f"quest {qid}: picks for classes {sorted(cells)}")
        for c, (pid, name) in cells.items():
            r = db.execute("SELECT itemname, classuse FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
            if not r or r[0] != name or not r[1] & (1 << c):
                raise Refused(f"record {pid} is {r}, not {name!r} for class {c}")
    grants = {}  # serverid -> [patternid]
    for qid, qname, kind, npc in QUESTS:
        q = rows(db, "quests", {"questid": qid})
        if len(q) != 1 or q[0]["name"] != qname:
            raise Refused(f"quest {qid} is {[r['name'] for r in q]}, not {qname!r}")
        steps = rows(db, "queststeps", {"questid": qid})
        last = max(steps, key=lambda s: s["step"]) if steps else None
        if not last or last["kind"] != kind or last["target"] != npc:
            raise Refused(f"quest {qid}'s last step is {last and (last['kind'], last['target'])}, not {kind} at {npc!r}")
        bc = json.loads(last["byclass"]) if last["byclass"] else {}
        want = {str(c): dict(bc.get(str(c), {})) for c in range(15)}
        for c in range(15):
            cell = want[str(c)]
            pid = PICKS[qid][c][0]
            if cell.get("items") and cell["items"] != str(pid):
                raise Refused(f"quest {qid} gives class {c} items {cell['items']!r} already")
            cell["items"] = str(pid)
        new = json.dumps(want, sort_keys=True, separators=(",", ":"))
        p.change("queststeps", last, {"byclass": last["byclass"]}, {"byclass": new},
                 f"quest {qid} {qname!r} step {last['step']} ({kind} {npc}): an item a class")
        for sid, cls in db.execute("SELECT c.serverid, c.tclass FROM charquests q JOIN characters c USING (serverid) "
                                   "WHERE q.questid = ? AND q.done > 0", (qid,)).fetchall():
            if not 0 <= cls < 15:
                continue
            pid = PICKS[qid][cls][0]
            if db.execute("SELECT 1 FROM charinventory WHERE serverid = ? AND patternid = ?", (sid, pid)).fetchone():
                continue
            if pid not in grants.setdefault(sid, []):
                grants[sid].append(pid)
    now = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    for sid, pids in sorted(grants.items()):
        acct, name, cls = db.execute("SELECT accountid, charName, tclass FROM characters WHERE serverid = ?", (sid,)).fetchone()
        note = "the level 35-45 side quests' class items, for quests finished before they gave one (petitions 5522-5538)"
        if rows(db, "chartransfers", {"serverid": sid, "from_server": GRANT}):
            p.say(f"character {sid}: granted already")
            continue
        plan = {"character": name, "serverid": sid, "note": note, "level": 0, "mastery_points": 0,
                "class_spells": False, "full_vitals": False, "to": "auto", "quests": [],
                "items": [{"pattern": pid, "count": 1} for pid in pids]}
        p.add("chartransfers", {"accountid": acct, "serverid": sid, "charname": name,
                                "from_server": GRANT, "from_name": name,
                                "from_class": CLASS_NAMES[cls], "notes": note, "status": "approved", "filed": now,
                                "updated": now, "plan": json.dumps(plan), "decided": now, "decided_by": WHO},
              ("serverid", "from_server"), f"character {sid} ({CLASS_NAMES[cls]}): {len(pids)} items at next login")


def keyof(db, tbl, row, rid):
    pk = sorted((r[5], r[1]) for r in db.execute(f'PRAGMA table_info("{tbl}")') if r[5])
    return {c: row[c] for _, c in pk} if pk else {"rowid": rid}


def audit(db, action, tbl, key, before, after):
    return db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                      "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                      (now(), ACCOUNT, WHO, VIA, action, tbl, dumps(key), None if before is None else dumps(before),
                       None if after is None else dumps(after), DETAIL)).lastrowid


def by_rowid(db, tbl, rid):
    r = rows(db, tbl, {"rowid": rid})
    return plain(r[0]) if r else None


def apply(db, p):
    db.execute(LOG_DDL)
    at = now()
    for tbl, rid, before, new in p.ops:
        if rid is None:
            cs = list(new)
            rid = db.execute(f'INSERT INTO "{tbl}" ({", ".join(chr(34) + c + chr(34) for c in cs)}) '
                             f'VALUES ({", ".join("?" * len(cs))})', [new[c] for c in cs]).lastrowid
            action = "insert"
        else:
            db.execute(f'UPDATE "{tbl}" SET {", ".join(chr(34) + c + chr(34) + " = ?" for c in new)} WHERE rowid = ?',
                       [*new.values(), rid])
            action = "update"
        after = by_rowid(db, tbl, rid)
        key = keyof(db, tbl, after, rid)
        a = audit(db, action, tbl, key, before, after)
        db.execute(f"INSERT INTO {LOG} (tbl, rid, rowkey, before, after, audit, at) VALUES (?, ?, ?, ?, ?, ?, ?)",
                   (tbl, rid, dumps(key), None if before is None else dumps(before), dumps(after), a, at))
    return len(p.ops)


def undo(db):
    if not table(db, LOG):
        return 0, []
    back, left = 0, []
    for seq, tbl, rid, rowkey, before, after in db.execute(
            f"SELECT seq, tbl, rid, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        have = by_rowid(db, tbl, rid)
        if have is None or dumps(have) != after:
            left.append(f"{tbl} {rowkey}: changed since, left as it is")
            continue
        b = None if before is None else json.loads(before)
        if b is None:
            db.execute(f'DELETE FROM "{tbl}" WHERE rowid = ?', (rid,))
            action = "delete"
        else:
            db.execute(f'UPDATE "{tbl}" SET {", ".join(chr(34) + c + chr(34) + " = ?" for c in b)} WHERE rowid = ?',
                       [*b.values(), rid])
            action = "update"
        audit(db, action, tbl, json.loads(rowkey), have, b)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if not db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0]:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--shard", required=True, choices=("heirloom", "live"))
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("PRAGMA busy_timeout = 5000")
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"undone: {back} rows put back" + "".join(f"\n  left: {w}" for w in left))
            return 0
        p = Plan(db)
        try:
            build(p, a.shard)
        except Refused as e:
            db.execute("ROLLBACK")
            print("\n".join(p.lines))
            print(f"REFUSED, nothing written: {e}")
            return 1
        print(f"{a.shard}:")
        print("\n".join(p.lines))
        print(f"totals: {len(p.ops)} rows to write")
        if not a.apply or not p.ops:
            db.execute("ROLLBACK")
            print("dry run: nothing written (--apply to write)" if not a.apply else "nothing to write: all in place")
            return 0
        n = apply(db, p)
        db.execute("COMMIT")
        print(f"applied: {n} rows, a gm_audit line each, logged in {LOG}")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
