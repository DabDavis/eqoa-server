#!/usr/bin/env python3
"""Dynmath the Tyrant as a raid boss with his dead (the owner, 2026-11-22, over a Live tester's report: "This boss
should be strong and have adds that are apart of the fight. DTT is suppoused to be a Giant zombie with Zombie minions.
right now you can solo DTT"). The guide (EQOA-Data Quests/Misc/dynmath-the-tyrant): "Speak to the portal, spawn
Dynmath the Tyrant, he will spawn with a lot of undead minions ... It's a zergefest so bring petters".

Before: quests 101257 (Dynmath the Tyrant) and 101258 (Another Totem of Dynmath) give the Totem of Dynmath to the
Etheneric Portal on Whale Hill, and the kill step's start spawns the spawn-only row 122985 "Dynmath the Tyrant", a
level 60 ghost's body at size 1, the roamer's template (a warrior with a crit): one player soloed him.

  - His row (122985, found by name and place): the body 0x001D9EDF, a hulking rotting zombie no NPC wore (picked by
    sight off CHAR.ESF's renders, `eqoa shot`), at size 2.5; level 60 kept.
  - His template "dynmath the tyrant raid" (npctemplate "dynmath the tyrant" from "roamer"): a necromancer's (class
    11), hp_pct 3000 (~430,000 at 60, beside the Fire Primeval's ~450,000 and the quest Sarek's 250,000), melee 175%,
    the roamer's Critical Hit, a rampage, his book's lifetap and damage over time. Undead (npcundead).
  - His dead: a new spawn-only row "minion of Dynmath" (a copy of his row: the zombies' body 0x1BBB898C, size 1.1,
    level 57), template "minion of dynmath" (a warrior's, hp_pct 250, melee 120%), undead.
  - The trigger "Dynmath the Tyrant" (its own lock: one fight at a time, 30 minutes, ten with no one by the portal
    end it): him and eight of his dead, hostile, where he stood; while he lives one more of the dead every 15 s after
    the last fell (resummon: his pets, dying with him); at his death his dead are taken away, his corpse left to loot;
    a run that ends without his death takes all away. The dead carry no loot (noloot; the resummoned are pets).
  - The quests: the hand-in at the portal (101257 step 1, 101258 step 2) fires the trigger ("already stands against
    another" while a fight lives, the totem kept), and the kill step's own spawn goes. A run that ends without his kill
    puts the quester back to the hand-in with the totem (handlers/questencounter.go), as the other encounters.

Loot is left as it is (the owner's). Every row is found by NAME (and place) and checked against what was read here;
anything else REFUSES, nothing written.

What takes effect when: npcs rows, spawn-only names, templates, undead, triggers and quests are read at a start:
apply in the before.sh of the next keep restart, on each shard.

Dry run by default. --apply writes in one transaction, a gm_audit line a row (account 1, dab, via tool), every row's
before and after in dynmath_raid_1122_log; a second --apply writes nothing. --undo puts back what the log holds,
latest first; a row changed since is left and said. --dump prints the trigger's steps.

    python3 -E -s tools/dynmath_raid_20261122.py --db eqoa.db --shard heirloom|live [--apply | --undo | --dump]
"""
import argparse
import datetime
import json
import math
import sqlite3
import sys

LOG = "dynmath_raid_1122_log"
WHO, ACCOUNT, VIA = "dab", 1, "tool"
TOOL = "tools/dynmath_raid_20261122.py"
SRC = "Live petition 5500 (dynmath_raid_20261122)"
DETAIL = f"the owner, 2026-11-22 (Live petition 5500): Dynmath the Tyrant a raid boss with zombie adds: {TOOL}"

BOSS = "Dynmath the Tyrant"
BOSS_AT = (0, 4765.387, 58.9529, 14028.429)
BOSS_WAS = {"modelid": 3965346723, "size": 1.0, "npc_level": 60}
BOSS_NOW = {"modelid": 0x001D9EDF, "size": 2.5}
MINION = "minion of Dynmath"
MINION_ROW = {"modelid": 0x1BBB898C, "size": 1.1, "npc_level": 57}
TRIGGER = "Dynmath the Tyrant"
LOCK = "dynmath the tyrant"
TOTEM = "701360"
# (quest id, its name, the hand-in step, the kill step)
QUESTS = [(101257, "Dynmath the Tyrant", 1, 2), (101258, "Another Totem of Dynmath", 2, 3)]
OLD_KILL_EVENTS = '[{"on":"start","do":"spawn","npc":"Dynmath the Tyrant","hostile":true}]'
HANDIN_EVENTS = json.dumps([{"on": "done", "do": "trigger", "trigger": TRIGGER,
                             "text": "Dynmath the Tyrant already stands against another: wait until he falls."}],
                           separators=(",", ":"))
NEAR = 3.0
MINUTES = 35
ADDS = 8

BOSS_TEMPLATE = {
    "name": "dynmath the tyrant raid", "class": 11, "hp_pct": 3000, "taken_pct": 100, "melee_pct": 175,
    "special": "Critical Hit", "special_damage": 7.4, "special_every": 30.0, "special_first": 15.0,
    "special_jitter": 5.0, "special_from": 10,
    "abilities": json.dumps([{"kind": "rampage", "chance": 15},
                             {"kind": "lifetap", "every": 18, "first": 6},
                             {"kind": "dot", "every": 24, "first": 10}], separators=(",", ":")),
    "note": "Live petition 5500 (the owner, 2026-11-22): Dynmath the Tyrant a raid boss, ~430k at 60 (hp_pct 3000; "
            "the Fire Primeval ~450k, the quest Sarek 250k), a necromancer's lifetap and damage over time, the "
            "roamer's Critical Hit and a rampage; his dead are the trigger's. " + TOOL,
    "source": SRC,
}
MINION_TEMPLATE = {
    "name": "minion of dynmath", "class": 0, "hp_pct": 250, "taken_pct": 100, "melee_pct": 120,
    "abilities": "[]",
    "note": "Live petition 5500: Dynmath the Tyrant's dead, eight at his rising and one more every 15 s after the last "
            "fell while he lives (the trigger 'Dynmath the Tyrant'). " + TOOL,
    "source": SRC,
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


def trigger_steps():
    w, x, y, z = BOSS_AT
    at = {"world": w, "x": round(x, 2), "y": round(y, 2), "z": round(z, 2)}
    return [
        {"do": "lock", "lock": LOCK},
        {"do": "clock", "seconds": 1800, "empty": 600},
        {"do": "broadcast", "text": "The Etheneric Portal tears open. Dynmath the Tyrant strides through, his dead "
                                    "behind him."},
        {"do": "spawn", "npc": BOSS, "hostile": True, "spread": 0.5, "minutes": MINUTES, **at},
        {"do": "spawn", "npc": MINION, "hostile": True, "count": ADDS, "spread": 12, "minutes": MINUTES, "noloot": True,
         **at},
        {"do": "resummon", "npc": MINION, "by": BOSS, "seconds": 15, "hostile": True, "minutes": MINUTES,
         "text": "Dynmath the Tyrant calls another of his dead out of the portal."},
        {"do": "awaitdead", "npc": BOSS},
        {"do": "broadcast", "text": "Dynmath the Tyrant falls, and his dead crumble to dust."},
        {"do": "despawn", "npc": MINION},
        {"do": "end"},
        {"do": "despawn"},
    ]


class Plan:
    def __init__(self, db):
        self.db = db
        self.ops = []  # (tbl, rid or None, before or None, after values)
        self.lines = []
        self.next_npc = None

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

    def npc_id(self):
        if self.next_npc is None:
            seq = self.db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'npcs'").fetchone() \
                if table(self.db, "sqlite_sequence") else None
            self.next_npc = max(seq[0] if seq else 0,
                                self.db.execute("SELECT COALESCE(MAX(id), 0) FROM npcs").fetchone()[0]) + 1
        n, self.next_npc = self.next_npc, self.next_npc + 1
        return n


def made_by_us(db, tbl, key):
    return table(db, LOG) and db.execute(f"SELECT 1 FROM {LOG} WHERE tbl = ? AND before IS NULL AND rowkey = ?",
                                         (tbl, dumps(key))).fetchone() is not None


def trigger_slot(db, name):
    r = db.execute("SELECT slot FROM gmtriggers WHERE name = ?", (name,)).fetchone()
    if r:
        return r[0]
    return max(db.execute("SELECT COALESCE(MAX(slot), 8) FROM gmtriggers").fetchone()[0], 8) + 1


def build(p):
    db = p.db
    for t in ("npcs", "npcspawnonly", "gmtriggers", "classtemplates", "npctemplate", "npcundead", "quests",
              "queststeps", "gm_audit"):
        if not table(db, t):
            raise Refused(f"no table {t}")
    w, x, _, z = BOSS_AT
    named = rows(db, "npcs", {"npc_name": BOSS})
    if len(named) != 1 or named[0]["world"] != w or math.hypot(named[0]["x_coord"] - x, named[0]["z_coord"] - z) > NEAR:
        raise Refused(f"{BOSS}: {[(r['id'], r['world'], r['x_coord'], r['z_coord']) for r in named]}, not one row "
                      f"at his spot")
    boss = named[0]
    p.change("npcs", boss, BOSS_WAS, BOSS_NOW, f"{BOSS} row {boss['id']} (body, size)")
    if not rows(db, "npcspawnonly", {"name": BOSS}) and not rows(db, "npcspawnonly", {"name": BOSS.lower()}):
        raise Refused(f"{BOSS} is not spawn-only by name")

    # his dead: a row of this tool's, else a copy of his row
    mins = rows(db, "npcs", {"npc_name": MINION})
    ours = [r for r in mins if made_by_us(db, "npcs", {"id": r["id"]})]
    if len(mins) != len(ours) or len(ours) > 1:
        raise Refused(f"rows named {MINION!r}: {[r['id'] for r in mins]}, not this tool's one")
    if ours:
        p.say(f"{MINION}: row {ours[0]['id']} there")
    else:
        r = plain(boss)
        r.update(MINION_ROW, id=p.npc_id(), npc_name=MINION, dt_kill=None)
        p.add("npcs", r, ("id",), f"{MINION} row {r['id']}, a copy of {BOSS}'s with the zombies' body")
    p.add("npcspawnonly", {"name": MINION.lower(), "source": SRC,
                           "note": f"Dynmath the Tyrant's dead: the trigger {TRIGGER!r} raises them. {TOOL}"},
          ("name",), f"{MINION} spawn-only")

    # templates
    for tpl in (BOSS_TEMPLATE, MINION_TEMPLATE):
        p.add("classtemplates", tpl, ("name",), f"template {tpl['name']!r}")
    have = rows(db, "npctemplate", {"name": BOSS.lower()})
    if have:
        p.change("npctemplate", have[0], {"template": "roamer"}, {"template": BOSS_TEMPLATE["name"]},
                 f"npctemplate {BOSS.lower()!r}")
    else:
        p.add("npctemplate", {"name": BOSS.lower(), "template": BOSS_TEMPLATE["name"], "note": TOOL, "source": SRC},
              ("name",), f"npctemplate {BOSS.lower()!r}")
    p.add("npctemplate", {"name": MINION.lower(), "template": MINION_TEMPLATE["name"], "note": TOOL, "source": SRC},
          ("name",), f"npctemplate {MINION.lower()!r}")
    for n in (BOSS.lower(), MINION.lower()):
        p.add("npcundead", {"name": n, "undead": 1, "note": f"a zombie (Live petition 5500). {TOOL}", "source": SRC},
              ("name",), f"undead {n!r}")

    # the trigger
    slot = trigger_slot(db, TRIGGER)
    p.add("gmtriggers", {"slot": slot, "name": TRIGGER, "enabled": 1, "scope": "near", "radius": 80, "world": w,
                         "x": round(x, 2), "y": round(BOSS_AT[2], 2), "z": round(z, 2),
                         "steps": json.dumps(trigger_steps(), separators=(",", ":")),
                         "notes": "Live petition 5500 (the owner, 2026-11-22): the Totem of Dynmath handed to the "
                                  "Etheneric Portal (quests 101257 step 1, 101258 step 2) raises Dynmath the Tyrant "
                                  f"and eight of his dead; one more every 15 s while he lives. {TOOL}"},
          ("name",), f"trigger {TRIGGER!r} (slot {slot})")

    # the quests
    for qid, name, handin, kill in QUESTS:
        q = rows(db, "quests", {"questid": qid})
        if len(q) != 1 or q[0]["name"] != name:
            raise Refused(f"quest {qid}: {[r['name'] for r in q]}, not {name!r}")
        h = rows(db, "queststeps", {"questid": qid, "step": handin})
        k = rows(db, "queststeps", {"questid": qid, "step": kill})
        if len(h) != 1 or h[0]["target"] != "Etheneric Portal" or h[0]["items"] != TOTEM or h[0]["kind"] != "give":
            raise Refused(f"quest {qid} step {handin}: not the totem's hand-in at the Etheneric Portal")
        if len(k) != 1 or k[0]["target"] != BOSS or k[0]["kind"] != "kill":
            raise Refused(f"quest {qid} step {kill}: not the kill of {BOSS}")
        p.change("queststeps", h[0], {"events": ""}, {"events": HANDIN_EVENTS},
                 f"quest {qid} step {handin} (the hand-in fires the trigger)")
        p.change("queststeps", k[0], {"events": OLD_KILL_EVENTS}, {"events": ""},
                 f"quest {qid} step {kill} (its own spawn goes)")


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
    g.add_argument("--dump", action="store_true")
    a = ap.parse_args(argv)
    if a.dump:
        print(json.dumps(trigger_steps(), separators=(",", ":")))
        return 0
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
            build(p)
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
