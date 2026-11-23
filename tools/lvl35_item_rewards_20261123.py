#!/usr/bin/env python3
"""The Highpass Hold level-35 side quests' unnamed class items (Live petitions 5517 and 5519, the owner, 2026-11-23:
"Did not get item reward from Dametrios for completing level 35 side quest part 1", "... part 2 from Ambrose Castor").

EQOA-Data Quests/23-47/35.json: Quest 1 (The Idols of Zsetikaowa, Dametrios) "Rewards: 3,768,632 XP, item, and new
ability"; Quest 3 (The Faded Silk Gown, Dunn McCabe) "then Tailor Portia for item reward"; Quest 5 (Fender's Stone)
"Tailor Portia rewards you" after the smith's experience; Quest 2 (The Seal of Deiphobus, Ambrose Castor) "Ambrose
rewards you", given an item too by the owner's word (petition 5519). The guides name none, so the import left them out.
Quests 4 (Voriander) and 6 (Rin Tintoggle) say only "rewards you" and keep their experience alone (the owner's call).

Each class is given an item on the quest's last step (its byclass, beside the spells Quest 1's gives): the class's own
records of levels 33-37, worn somewhere, that nothing else gives (no loot table, pool, NPC's held item or quest reward)
and no character holds, ranked: the fewest classes in the mask (the class's own first), nearest level 35, a belt, then
bracers, gloves, boots, chest, legs, head, the rest, then the record's id; "Honor's Guard Breastplate" (6629) left out
as the misspelt twin of 6628. Quest 1 takes each class's first, Quest 2 its second, Quest 3 its third, Quest 5 its
fourth (the picks below, the same records on both shards on 2026-11-23). A step is found as the quest's LAST step of
its kind at its NPC, so steps added before it (Heirloom's part-2 edit) change nothing.

Each character who finished one of the four before this (charquests done) and holds none of its item is given it at
its next login, as a GM grant: one approved chartransfers row a character ("GM grant (petitions 5517/5519)"), the
items of every such quest in it. Refused, nothing written, if a quest, step or record is not as read here, or a
class's cell already gives an item.

Quests are read at a start or a GM panel quest reload; transfers at the character's login. Dry run by default;
--apply writes in one transaction, a gm_audit line a row, before and after in lvl35_items_1123_log; a second --apply
writes nothing; --undo puts back what the log holds (a grant already applied stays given).

    python3 -E -s tools/lvl35_item_rewards_20261123.py --db eqoa.db --shard heirloom|live [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "lvl35_items_1123_log"
WHO, ACCOUNT, VIA = "dab", 1, "tool"
TOOL = "tools/lvl35_item_rewards_20261123.py"
SRC = "Live petitions 5517/5519 (lvl35_item_rewards_20261123)"
DETAIL = f"the owner, 2026-11-23 (Live petitions 5517, 5519): the level-35 side quests' class items: {TOOL}"

# (quest id, name, its last reward step's kind and NPC, the pick it takes)
QUESTS = [(100775, "The Idols of Zsetikaowa", "talk", "Dametrios Velspane", 0),
          (100776, "The Seal of Deiphobus", "give", "Ambrose Castor", 1),
          (100777, "The Faded Silk Gown", "talk", "Tailor Portia", 2),
          (100779, "Fender's Stone", "talk", "Tailor Portia", 3)]
CLASS_NAMES = ["Warrior", "Ranger", "Paladin", "Shadowknight", "Monk", "Bard", "Rogue", "Druid", "Shaman", "Cleric",
               "Magician", "Necromancer", "Enchanter", "Wizard", "Alchemist"]
PICKS = {
    0: [(7142, 'Warblood Belt'), (1326, 'Bracers of Endurance'), (1317, 'Gloves of Endurance'), (1318, 'Boots of Endurance')],
    1: [(7518, "Earthrunner's Boots"), (5758, 'Moss Covered Longbow'), (7085, 'Eldervine Belt'), (7516, 'Dustweave Boots')],
    2: [(6628, "Honor Guard's Breastplate"), (6296, 'Blessed Helmet of Might'), (7117, 'Sand Giants Girdle'), (5641, 'Fyrebrand Blade')],
    3: [(7799, 'Black Bracers of Spite'), (7473, 'Black Boots of Spite'), (6559, 'Black Breastplate of Spite'), (7249, 'Black Legplates of Spite')],
    4: [(6926, 'Lotusweave Gloves'), (5705, 'Iron Star Fist'), (5945, "Champion's Duster"), (6438, 'Wind of Ro Cowl')],
    5: [(7945, "Tune Seeker's Bracers"), (6874, 'Cloudsong Gauntlets'), (6991, "Tune Seeker's Gloves"), (7607, "Tune Seeker's Boots")],
    6: [(7794, "Battlestalker's Bracers"), (7802, "Blackened Thief's Bracers"), (6859, "Blackened Thief's Gloves"), (5934, "Blackened Thief's Dirk")],
    7: [(7141, 'Vinesorrow Belt'), (6937, 'Nite Jaguar-Hide Gloves'), (7103, 'Oaken Belt Buckle'), (6392, 'Petrified Bark Helm')],
    8: [(6554, "Battlespirit's Tunic"), (5322, 'Goblin Skull Fetish'), (7003, "Rowen'Dal Shaman's Gloves"), (6675, "Rowen'Dal Shaman's Tunic")],
    9: [(7102, 'Nitestar Belt'), (7913, 'Runed Defenders Bracers'), (6963, "Runed Defender's Gloves"), (7577, "Runed Defender's Boots")],
    10: [(7126, 'Shimmerveil Sash'), (4972, "Geomancer's Robe"), (6889, 'Frost Covered Gloves'), (6685, "Seeker's Tunic")],
    11: [(7922, 'Silken Bracers of Death'), (6935, 'Nightmorrow Gloves'), (6939, 'Nitemorrow Gloves'), (6972, 'Silken Gloves of Death')],
    12: [(7135, 'Sunblaze Sash'), (4974, 'Glyphmagus Robe'), (6889, 'Frost Covered Gloves'), (6685, "Seeker's Tunic")],
    13: [(7120, 'Sash of the Cosmos'), (6960, "Rowen'Dal Wizard's Gloves"), (1783, 'Vestige of Evocaction'), (5074, 'robe of the tempest')],
    14: [(7855, 'Elemental Bracers'), (6604, 'Elemental Shirt'), (7285, 'Elemental Pants'), (6295, 'Blackened Cap')],
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
    for c, ps in PICKS.items():
        for pid, name in ps:
            r = db.execute("SELECT itemname, classuse, levelreq FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
            if not r or r[0] != name or not r[1] & (1 << c):
                raise Refused(f"record {pid} is {r}, not {name!r} for class {c}")
    grants = {}  # serverid -> [patternid]
    for qid, qname, kind, npc, k in QUESTS:
        q = rows(db, "quests", {"questid": qid})
        if len(q) != 1 or q[0]["name"] != qname:
            raise Refused(f"quest {qid} is {[r['name'] for r in q]}, not {qname!r}")
        steps = rows(db, "queststeps", {"questid": qid})
        last = max(steps, key=lambda s: s["step"]) if steps else None
        if not last or last["kind"] != kind or last["target"] != npc:
            raise Refused(f"quest {qid}'s last step is {last and (last['kind'], last['target'])}, not {kind} at {npc}")
        bc = json.loads(last["byclass"]) if last["byclass"] else {}
        want = {str(c): dict(bc.get(str(c), {})) for c in range(15)}
        for c in range(15):
            cell = want[str(c)]
            pid = PICKS[c][k][0]
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
            pid = PICKS[cls][k][0]
            if db.execute("SELECT 1 FROM charinventory WHERE serverid = ? AND patternid = ?", (sid, pid)).fetchone():
                continue
            grants.setdefault(sid, []).append(pid)
    now = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    for sid, pids in sorted(grants.items()):
        acct, name, cls = db.execute("SELECT accountid, charName, tclass FROM characters WHERE serverid = ?", (sid,)).fetchone()
        note = "petitions 5517/5519: the level-35 side quests' class items, for quests finished before they gave one"
        if rows(db, "chartransfers", {"serverid": sid, "from_server": "GM grant (petitions 5517/5519)"}):
            p.say(f"character {sid}: granted already")
            continue
        plan = {"character": name, "serverid": sid, "note": note, "level": 0, "mastery_points": 0,
                "class_spells": False, "full_vitals": False, "to": "auto", "quests": [],
                "items": [{"pattern": pid, "count": 1} for pid in pids]}
        p.add("chartransfers", {"accountid": acct, "serverid": sid, "charname": name,
                                "from_server": "GM grant (petitions 5517/5519)", "from_name": name,
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
