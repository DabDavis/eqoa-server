#!/usr/bin/env python3
"""The Plane of Sky's chains at raid strength (Live petition 5513, 2026-11-23: "after testing Rain chain the PoS chains
are very very easy to do, they should be raid style mobs"; the owner: yes, raid-style).

Before: the chains' isle bosses fought by the roamer's kit at a level's hit points (~16k at 64), Granavik at 632%
(~97k), Chimareon at 600% (~98k), Enraged Aeyinar at 500%, every one a melee of 100%: one group of four killed them, and
a geared tank took the floor of their hits. The other raid bosses of 60+: the Primevals 2533% hit points (~365k at 60),
Dynmath the Tyrant 3000% (~430k) with 175% melee, Lord Nagafen 4217%, Vallon and Tallon Zek 5471% and 5963%, the
Quintessence 7371% at the chains' end, Ahriaana 8688%.

  - The chains' bosses (each isle's key or shard boss and the named the chains' steps and events raise: the rain chain's
    Artiokanis, Kaglanor, Ghizilanve, Granavik, the necrologist, his spirit and Chimareon; the sun's brightstar
    princess, Imerzan, the torrential wind, the rancorous drake and Randarin; night's Ancient Jester, iron knight, Arch
    Bishops Millian and Lancel, the dark baroness and King Ruthra; snow's Wind Mephit, Whirlwind, glacial champion and
    both Aeyinars; desolation's Erych Arbiter, the warlord of Narcillian and the elite harpy queen): template "sky raid
    boss", 2000% hit points (~290k at 60, ~326k at 64: a raid fight, below the single-encounter raid bosses, as a chain
    has four or five of them back to back before the Quintessence), 300% melee (a geared tank's AC no longer takes
    every hit to the floor), Granavik's Critical Hit (10.9375 a level, every 25-35 s), a rampage on 15% of its rounds
    at 80% (Tallon Zek's) and Frost Burst (the constructs' and Aeyinar's area spell, 905003) at 67% every 30 s from 20 s.
    Those with no template (the necrologist, the two arch bishops: casters by their own class) take "sky raid boss
    caster", the same but their own class. The three with a template of their own (Aeyinar, Enraged Aeyinar, the
    rancorous drake) keep it, its hit points and melee raised to the same 2000% and 300% (the drake's empty kit given
    the rampage and Frost Burst; the Aeyinars keep theirs). The Quintessence is left as she is.
  - The chains' isle creatures (the trash on a raid's way: the rain isles' flesh fiends, qex, maggots, demions and
    gargoyles; the sun's fairies, sycophants, winds and drakes; night's ancients, golems, servants, barons, Ruthra's
    court; snow's gargoyles, behemoths, brutes and drakes; desolation's erych, Narcillian's men and harpies): template
    "sky raid creature", 300% hit points (~49k at 64) and 175% melee, the roamer's Critical Hit; Ruthra's bishops and
    mages and the night duke (no template: their own class) "sky raid creature caster", the same with no special.
    The flesh fiends by their rows in world 4 (npctemplate "#id"): a flesh fiend of world 0 at 25 shares the name.

Loot is not touched. balancelevel (level-wide) is not touched. Every row is found by name and checked against what was
read here (2026-11-23, both shards alike); anything else REFUSES, nothing written. Templates are read at a start: apply in
the before.sh of the next keep restart, on each shard.

Dry run by default. --apply writes in one transaction, a gm_audit line a row (account 1, dab, via tool), each row's
before and after in sky_raid_1123_log; a second --apply writes nothing. --undo puts back what the log holds.

    python3 -E -s tools/sky_raid_20261123.py --db eqoa.db --shard heirloom|live [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "sky_raid_1123_log"
WHO, ACCOUNT, VIA = "dab", 1, "tool"
TOOL = "tools/sky_raid_20261123.py"
SRC = "Live petition 5513 (sky_raid_20261123)"
DETAIL = f"the owner, 2026-11-23 (Live petition 5513): the Plane of Sky's chains at raid strength: {TOOL}"

BOSS_HP, BOSS_MELEE = 2000, 300
TRASH_HP, TRASH_MELEE = 300, 175
BOSS_KIT = [{"kind": "rampage", "chance": 15, "damage_pct": 80},
            {"kind": "aoe", "spell_id": 905003, "damage_pct": 67, "every": 30, "first": 20}]
CRIT = {"special": "Critical Hit", "special_every": 30.0, "special_first": 15.0, "special_jitter": 5.0, "special_from": 10}


def kit(abilities):
    return json.dumps(abilities, separators=(",", ":"))


BOSS_NOTE = ("Live petition 5513 (the owner, 2026-11-23: the Plane of Sky's chains raid-style): a chain's boss, 2000% hit "
             "points (~326k at 64; the Primevals 2533%, Dynmath 3000%, the Quintessence 7371%), 300% melee, Granavik's "
             "Critical Hit, a rampage (15% of rounds, 80%) and Frost Burst (905003, 67%) every 30 s. " + TOOL)
TRASH_NOTE = ("Live petition 5513: a Plane of Sky chain isle's creature at raid strength, 300% hit points (~49k at 64), "
              "175% melee. " + TOOL)
TEMPLATES = [
    dict(name="sky raid boss", **{"class": 0}, hp_pct=BOSS_HP, taken_pct=100, melee_pct=BOSS_MELEE, special_damage=10.9375,
         abilities=kit(BOSS_KIT), note=BOSS_NOTE, source=SRC, **CRIT),
    dict(name="sky raid boss caster", **{"class": -1}, hp_pct=BOSS_HP, taken_pct=100, melee_pct=BOSS_MELEE,
         special_damage=10.9375, abilities=kit(BOSS_KIT), note=BOSS_NOTE + " Its own class (a caster's).", source=SRC,
         **CRIT),
    dict(name="sky raid creature", **{"class": 0}, hp_pct=TRASH_HP, taken_pct=100, melee_pct=TRASH_MELEE,
         special_damage=7.4, abilities="[]", note=TRASH_NOTE + " The roamer's Critical Hit.", source=SRC, **CRIT),
    dict(name="sky raid creature caster", **{"class": -1}, hp_pct=TRASH_HP, taken_pct=100, melee_pct=TRASH_MELEE,
         abilities="[]", note=TRASH_NOTE + " Its own class (a caster's), no special, as before.", source=SRC),
]

# name -> (its npctemplate now, the template it takes); None: no row now
BOSSES = {
    "artiokanis": "roamer", "kaglanor": "roamer", "ghizilanve": "roamer", "granavik": "sky chain boss",
    "spirit of narciliisisin": "roamer", "chimareon the tyrant": "chimareon the tyrant",
    "brightstar princess": "roamer", "imerzan": "roamer", "torrential wind": "roamer",
    "randarin the relentless": "roamer", "ancient jester": "roamer", "iron knight": "roamer", "dark baroness": "roamer",
    "king ruthra": "roamer", "wind mephit": "roamer", "whirlwind": "roamer", "glacial champion": "roamer",
    "erych arbiter": "roamer", "warlord of narcillian": "roamer", "elite harpy queen": "roamer",
}
BOSS_CASTERS = ["necrologist", "arch bishop millian", "arch bishop lancel"]
# their own templates, raised in place: (hp_pct, melee_pct, abilities) as read
OWN = {"aeyinar": (100, 100, None), "enraged aeyinar": (500, 250, None), "rancorous drake": (100, 100, "[]")}
TRASH = ["forest qex", "flesh maggot", "demion", "guardian gargoyle", "brightstar fairy", "imerzan sycophant",
         "whispering wind", "whistling wind", "wailing wind", "rapacious drake", "ancient peasant", "ancient vassal",
         "ancient denizen", "ancient squire", "night golem", "dark servant", "dark baron", "ruthra's squire",
         "icebound gargoyle", "glacial behemoths", "snow brute", "celestial drake", "erych scout", "erych scavenger",
         "erych patroller", "erych guardian", "scout of narcillian", "warrior of narcillian", "dark harpy", "light harpy"]
TRASH_CASTERS = ["night duke", "ruthra's bishop", "ruthra's mage"]
# a name shared with another world's creatures: its world-4 rows by row key
BY_ROW = ["flesh fiend"]
SKY = 4

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
    for t in ("npcs", "npctemplate", "classtemplates", "gm_audit"):
        if not table(db, t):
            raise Refused(f"no table {t}")
    for tpl in TEMPLATES:
        p.add("classtemplates", dict(tpl), ("name",), f"template {tpl['name']!r}")

    def only_sky(n):
        worlds = {r[0] for r in db.execute("SELECT DISTINCT world FROM npcs WHERE lower(npc_name) = ?", (n,))}
        if worlds != {SKY}:
            raise Refused(f"{n!r}: rows in worlds {sorted(worlds)}, not the Plane's alone")

    def use(n, was, now_):
        only_sky(n)
        have = rows(db, "npctemplate", {"name": n})
        if was is None:
            if have and have[0]["template"] != now_:
                raise Refused(f"{n!r}: npctemplate {have[0]['template']!r}, not none")
            p.add("npctemplate", {"name": n, "template": now_, "note": TOOL, "source": SRC}, ("name",),
                  f"{n!r} -> {now_!r}")
        else:
            if len(have) != 1:
                raise Refused(f"{n!r}: no npctemplate row (want {was!r})")
            p.change("npctemplate", have[0], {"template": was}, {"template": now_, "source": SRC}, f"{n!r}")

    for n, was in BOSSES.items():
        use(n, was, "sky raid boss")
    for n in BOSS_CASTERS:
        use(n, None, "sky raid boss caster")
    for n in TRASH:
        use(n, "roamer", "sky raid creature")
    for n in TRASH_CASTERS:
        use(n, None, "sky raid creature caster")
    for n, (hp, melee, abil) in OWN.items():
        only_sky(n)
        have = rows(db, "npctemplate", {"name": n})
        if len(have) != 1 or have[0]["template"] != n:
            raise Refused(f"{n!r}: npctemplate {[h['template'] for h in have]}, not its own")
        t = rows(db, "classtemplates", {"name": n})
        if len(t) != 1:
            raise Refused(f"no template {n!r}")
        was = {"hp_pct": hp, "melee_pct": melee}
        now_ = {"hp_pct": BOSS_HP, "melee_pct": BOSS_MELEE}
        if abil is not None:
            was["abilities"], now_["abilities"] = abil, kit(BOSS_KIT)
        p.change("classtemplates", t[0], was, now_, f"template {n!r} (its own, raised)")
    for n in BY_ROW:
        ids = [r[0] for r in db.execute("SELECT id FROM npcs WHERE world = ? AND lower(npc_name) = ? ORDER BY id", (SKY, n))]
        if not ids:
            raise Refused(f"no {n!r} in world {SKY}")
        for i in ids:
            key = f"#{i}"
            have = rows(db, "npctemplate", {"name": key})
            if have and have[0]["template"] != "sky raid creature":
                raise Refused(f"row {key} ({n}) has template {have[0]['template']!r}")
            p.add("npctemplate", {"name": key, "template": "sky raid creature", "note": f"{n}, world {SKY}. {TOOL}",
                                  "source": SRC}, ("name",), f"{n!r} row {key} -> 'sky raid creature'")


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
