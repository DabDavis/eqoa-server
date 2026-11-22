#!/usr/bin/env python3
"""Sarek the Betrayer and the Anvil of Sarek as a GM's work board entry 709 has them (the owner, 2026-11-22: "build all
of #709? yes"), and Arios the Battered's last fight sized as the quest Sarek (a player's report of 2026-11-22: he died
fast to one player with mercenaries; no figure for him on the board or in the entry's sources, so ours). The kill hit
(-505) and the random Sarek's respawn are tools/jib_batch1_20261121.py's part 709, already in; this is the rest. Every
row is found by NAME and place and checked against what the entry read; anything else REFUSES, nothing written.

SAREK THE BETRAYER (level 67; the HP a point of hp_pct at 67 is 177.7; a level 60 raid deals him half):
  - The random one: the standing row in Sarek's tower (the Black Lotus port's room), his own template "sarek the
    betrayer" (was the roamer's copy): hp_pct 4221 (750,000: ~1.5M effective, three times the quest one's for "min
    15"), the roamer's Critical Hit gone (his level's melee alone), and Sarek's Blight: a fixed 8,000 disease hit on his
    target every 20 s, cut by DR (the entry's; on the random Sarek ONLY, the GM's answer to its open question 1 of
    2026-11-20). His resists (PR, DR, AR 50) and his long respawn kept.
  - The quest one: a new npcs row of his, spawn-only by row, his row key's template "sarek the betrayer quest":
    hp_pct 1407 (250,000: ~500k effective, "one group of 60s"), no crit, no Blight. The entry puts him in "a different
    tower": the ground has three tower rooms here, each reached only by a port (Sarek's, where the random one stands,
    and the two constructs' towers); he stands in Archon Aereth's (the top floor at 110.4, the room's middle), and quest
    101029 step 19's teleport goes there (it went beside the random Sarek). He is raised by the new trigger "Sarek the
    Betrayer" (one at a time: its lock), fired by step 18's hand-in to Arios: Aereth (a 90 s respawn in a room of 28)
    is held out of view for the run, five seconds, then Sarek, hostile, for an hour at most (ten minutes with no one in
    the room end it). A run that ends without his kill puts the quester back to step 18, the channeling stone handed
    back (handlers/questencounter.go), so nobody is stranded in a room no one walks to.
  - Archon Seradesh beside the random Sarek (her row there) is spawn-only by row; step 19's done raises her beside the
    quester for 30 minutes. Step 20 (talk to her) is done at her other row too, by the Anvil, so no quester waits on
    the copy.
  - Loot, one table pair for both (npcloot by name): "boss sarek class", one class item every kill (100% x1, weight 1
    each): the thirteen records the entry lists and two made here, Earthenguard Trident (paladin: two-handed, 815, HP
    100, DEX 30, STA 20, "Procs 100 power to its wielder": the power proc's figure ours; the Trident of the Merkings'
    look) and Serpent's Heart (alchemist: a usable stone, "Self, Summon Miraculous Draught x3, Unlimited Uses", 300 s
    apart: three of the level 60 draughts; an item with a use is used, not worn, so it is no gear); "boss sarek
    corpse", one of brimstone band, Worldbreaker Sledge, earthen garb and Focus & Desire (100% x1; the roll ours). Gear
    as its Ascended record (petition 1446's rule, database/ascended.go, tools/ascend_boss_loot.py's SQL), the scroll and
    the stone as they are. npccoins 800 (his 2009 corpse's ~800 tunar).

THE ANVIL OF SAREK (quest 101029 steps 3 and 10):
  - His kick zone (tools/anvil_1956_20261113.py: 19.9 across, every second) hits for 99,999, the entry's bolt, and
    spares the one a quest sends to him (spare_quest, entity/kickzone.go: a player whose step to do is a talk, show
    or give at him): before, the zone also struck down the quester, who must come within 10 to talk.
  - Steps 3 and 10 done: he is held out of view 30 minutes and, where he stood, a copy of a new row of his (spawn-only
    by row, template "anvil of sarek fight": hp_pct 805, 135,000 at 65, no zone) rises hostile with three Minions of
    Sarek (a new row, level 63, a terran caretaker's body; template "minion of sarek": hp_pct 190, 30,000), 20 minutes,
    one set for a group. No step waits on their kills.

ARIOS THE BATTERED (step 26, the row by the Rallosian Arbiter, level 60): his row key's template "arios the battered",
hp_pct 3472 (500,000 at 60, where a group of 60s deals all of it).

What takes effect when: npcs rows, spawn-only rows, templates, quests and triggers are read at a start, and the Anvil's
spare_quest is a field an older server refuses (the template unread: the Anvil harmless): apply it in the before.sh of
the keep restart that brings entity/kickzone.go's spare_quest. Loot tables, npcloot, npccoins and items at once.

Dry run by default. --apply writes in one transaction, a gm_audit line a row (account 1, dab, via tool), every row's
before and after in sarek_709_1122_log; a second --apply writes nothing. --undo puts back what the log holds, latest
first; a row changed since is left and said, and an Ascended record or a made item something holds is kept. --dump
prints the trigger, the steps and the templates as JSON.

    python3 -E -s tools/sarek_709_20261122.py --db eqoa.db --shard heirloom|live [--apply | --undo | --dump]
"""
import argparse
import datetime
import json
import math
import os
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import ascend_boss_loot as asc  # noqa: E402  (the server's Ascended rule)

LOG = "sarek_709_1122_log"
WHO, ACCOUNT, VIA = "dab", 1, "tool"
TOOL = "tools/sarek_709_20261122.py"
SRC = "board 709 (sarek_709_20261122)"
DETAIL = f"a GM's work board entry 709, Sarek the Betrayer and the Anvil of Sarek (the owner, 2026-11-22): {TOOL}"

QUEST = 101029
SAREK, SERADESH, ANVIL, ARIOS, AERETH = "Sarek the Betrayer", "Archon Seradesh", "Anvil of Sarek", "Arios the Battered", "Archon Aereth"
MINION, CARETAKER = "Minion of Sarek", "terran caretaker"
RANDOM_AT = (1, 4306.85, 2570.09)       # the standing Sarek, Sarek's tower (the Black Lotus port's room)
SERADESH_AT = (1, 4282.56, 2586.42)     # Archon Seradesh beside him
SERADESH_ANVIL_AT = (1, 4744.93, 4811.66)
ANVIL_AT = (1, 4738.44, 4651.91)
ARIOS_AT = (5, 4944.3, 7057.4)          # step 26's Arios, before the Rallosian Arbiter
AERETH_AT = (1, 4699.2, 2512.7)
CARETAKER_AT = (1, 4793.76, 4675.14)    # the level 60 terran caretaker whose body the Minions take
TOWER = (1, 4700.0, 110.375, 2500.0)    # the quest Sarek: Aereth's tower room, its middle
LANDING = (1, 4700.0, 110.375, 2490.0)  # step 19's teleport, the room's north side, facing him
SERADESH_RISE = (1, 4704.0, 110.375, 2496.0)
NEAR = 3.0

TPL_RANDOM, TPL_QUEST = "sarek the betrayer", "sarek the betrayer quest"
TPL_ANVIL, TPL_ANVIL_FIGHT, TPL_MINION, TPL_ARIOS = "anvil of sarek", "anvil of sarek fight", "minion of sarek", "arios the battered"
TRIGGER, LOCK = "Sarek the Betrayer", "sarek the betrayer"
T_CLASS, T_CORPSE = "boss sarek class", "boss sarek corpse"

RANDOM_HP, QUEST_HP, ANVIL_HP, MINION_HP, ARIOS_HP = 4221, 1407, 805, 190, 3472
DISEASE_FX = 0x0CACDEE1  # the disease hit's look (a template proc's, tools/fix_innoruuk_visuals_20261110.py)
BLIGHT = {"kind": "nuke", "name": "Sarek's Blight", "damage": 8000, "resist": "DR", "fx": DISEASE_FX, "every": 20, "first": 10}
OLD_ZONE = [{"kind": "kickzone", "radius": 19.9, "damage": 10000, "every": 1, "name": "Kick"}]
NEW_ZONE = [{"kind": "kickzone", "radius": 19.9, "damage": 99999, "every": 1, "name": "Kick", "spare_quest": True}]

CLASS_ITEMS = [(5367, "earthshard"), (5248, "Tears of the Rathe"), (5250, "communion stone"), (5052, "robe of the chasm"),
               (5033, "robe of geomancy"), (5345, "Lodestone of Sarek"), (4978, "heartsblood robe"),
               (5359, "Quicksilver longbow"), (5351, "mountainspine dagger"), (6089, "Tower of Woe"),
               (5382, "claws of the beast"), (5375, "earthbound katana"), (5126, "starless night robe")]
CORPSE_ITEMS = [(8132, "brimstone band"), (5479, "Worldbreaker Sledge"), (6518, "earthen garb"), (716230, "Focus & Desire")]
TRIDENT, HEART = "Earthenguard Trident", "Serpent's Heart"
TRIDENT_LOOK, HEART_LOOK, DRAUGHT = (5451, "Trident of the Merkings"), (718019, "Superior Mana Stone"), "Miraculous Draught"
NEW_ITEMS = (700000, 759999)
PAL, ALC = 2, 14
COINS = 800

LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    rid    INTEGER NOT NULL,  -- the row's rowid
    rowkey TEXT NOT NULL,     -- its key (JSON)
    before TEXT,              -- the whole row before (JSON; NULL: none)
    after  TEXT,              -- the whole row after (JSON; NULL: removed)
    audit  INTEGER NOT NULL,
    at     TEXT NOT NULL
)"""


class Refused(Exception):
    pass


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def dumps(v):
    s = json.dumps(v, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def jsc(v):
    """A JSON column as the rows keep it (key order kept)."""
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False)


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def pk(db, tbl):
    p = sorted((r[5], r[1]) for r in db.execute(f'PRAGMA table_info("{tbl}")') if r[5])
    return [c for _, c in p]


def rows(db, tbl, where):
    w = " AND ".join(f'"{c}" = ?' for c in where) or "1"
    cur = db.execute(f'SELECT rowid AS _rowid_, * FROM "{tbl}" WHERE {w} ORDER BY rowid', tuple(where.values()))
    names = [d[0] for d in cur.description]
    return [dict(zip(names, r)) for r in cur.fetchall()]


def by_rowid(db, tbl, rid):
    r = rows(db, tbl, {"rowid": rid})
    return r[0] if r else None


def plain(r):
    return None if r is None else {k: v for k, v in r.items() if k != "_rowid_"}


def same(a, b):
    if isinstance(a, (int, float)) and isinstance(b, (int, float)) and not isinstance(a, bool):
        return abs(float(a) - float(b)) < 1e-6
    return a == b


def named_near(db, name, at, reach=NEAR):
    w, x, z = at
    return [r for r in rows(db, "npcs", {"npc_name": name, "world": w}) if math.hypot(r["x_coord"] - x, r["z_coord"] - z) <= reach]


def between(pairs, level):
    """entity/balance.go's Level: the row of the level, the last past the end, the first before it, else the line between."""
    pairs = sorted(pairs)
    if not pairs:
        return 100.0
    for i, (lv, v) in enumerate(pairs):
        if lv >= level:
            if lv == level or i == 0:
                return v
            plv, pv = pairs[i - 1]
            return pv + (v - pv) * (level - plv) / (lv - plv)
    return pairs[-1][1]


def max_hp(db, level, hp_pct):
    """An NPC's hit points at its level and template share: 60 a level, its level's balance, the template's hp_pct."""
    bal = between(list(db.execute("SELECT level, hp_pct FROM balancelevel")) if table(db, "balancelevel") else [], level)
    return round(max(level * 60, 10) * bal / 100 * hp_pct / 100)


def ascended_values(vals):
    """database/ascended.go's record of a base not yet written (asc.copy_columns's SQL, here)."""
    out = {}
    for c, v in vals.items():
        low = c.lower()
        if low == "patternid":
            continue
        if low in asc.SCALED:
            out[c] = v if v is None or int(v) <= 0 else (int(v) * (100 + asc.PERCENT) + 50) // 100
        elif low == "itemname":
            out[c] = asc.ascended_name(v)
        elif low == "rent":
            out[c] = 1 if v == 0 else v
        else:
            out[c] = v
    return out


class Plan:
    def __init__(self, db, shard):
        self.db, self.shard = db, shard
        self.ops = []      # (tbl, rid or None, before row or None, after values or None)
        self.lines = []
        self.next_asc = None
        self.next_item = None
        self.next_npc = None
        self.world = None
        self.made = {}     # what this plan makes: names -> ids

    def say(self, s):
        self.lines.append(f"  {s}")

    def one(self, tbl, where, what=None):
        r = rows(self.db, tbl, where)
        if len(r) != 1:
            raise Refused(f"{what or tbl} {dumps(where)}: {len(r)} rows, not one")
        return r[0]

    def set(self, tbl, row, changes, what):
        """changes {col: (old, new)}: each column as read (old; a tuple of olds allowed) or already as asked."""
        new = {}
        for c, (old, want) in changes.items():
            have = row[c]
            olds = old if isinstance(old, tuple) else (old,)
            if same(have, want):
                continue
            if not any(same(have, o) for o in olds):
                raise Refused(f"{what}: {tbl}.{c} is {have!r}, neither as read ({old!r}) nor as asked ({want!r})")
            new[c] = want
        if not new:
            self.say(f"{what}: already as asked")
            return False
        self.ops.append((tbl, row["_rowid_"], row, new))
        self.say(f"{what}: " + ", ".join(f"{c} {short(row[c])} -> {short(v)}" for c, v in new.items()))
        return True

    def add(self, tbl, values, match, what):
        """Insert values unless a row matching match is there: as values (done) or otherwise (refused)."""
        have = rows(self.db, tbl, {c: values[c] for c in match})
        if have:
            if len(have) == 1 and all(same(have[0][c], v) for c, v in values.items()):
                self.say(f"{what}: already there")
                return False
            raise Refused(f"{what}: {tbl} has {dumps([plain(h) for h in have])[:400]}, not {dumps(values)[:400]}")
        self.ops.append((tbl, None, None, dict(values)))
        self.say(f"{what}: insert {tbl} {short(dumps(values), 240)}")
        return True

    # ---- new ids, taken on this shard
    def npc_id(self):
        if self.next_npc is None:
            seq = self.db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'npcs'").fetchone() \
                if table(self.db, "sqlite_sequence") else None
            self.next_npc = max(seq[0] if seq else 0, self.db.execute("SELECT COALESCE(MAX(id), 0) FROM npcs").fetchone()[0]) + 1
        n, self.next_npc = self.next_npc, self.next_npc + 1
        return n

    def item_id(self):
        if self.next_item is None:
            self.next_item = self.db.execute("SELECT COALESCE(MAX(patternid), ?) + 1 FROM itempattern WHERE patternid "
                                             "BETWEEN ? AND ?", (NEW_ITEMS[0] - 1, *NEW_ITEMS)).fetchone()[0]
        if self.next_item > NEW_ITEMS[1]:
            raise Refused(f"no item id left in {NEW_ITEMS}")
        n, self.next_item = self.next_item, self.next_item + 1
        return n

    def asc_id(self):
        if self.next_asc is None:
            self.next_asc = self.db.execute("SELECT COALESCE(MAX(patternid), ?) + 1 FROM itempattern WHERE patternid "
                                            "BETWEEN ? AND ?", (asc.FIRST_ID - 1, asc.FIRST_ID, asc.LAST_ID)).fetchone()[0]
        if self.next_asc > asc.LAST_ID:
            raise Refused(f"no id left for an Ascended record ({asc.FIRST_ID}-{asc.LAST_ID})")
        n, self.next_asc = self.next_asc, self.next_asc + 1
        return n

    # ---- the Ascended records (database/ascended.go's rule)
    def ascended_of(self, base, vals=None, effects=()):
        """base's Ascended record: the one there, else made (vals: a base this plan makes, with its effects)."""
        db = self.db
        r = db.execute("SELECT patternid FROM itemascended WHERE base = ?", (base,)).fetchone() if table(db, "itemascended") else None
        if r:
            return r[0]
        if vals is None:
            if self.world is None:
                self.world = asc.World(db)
            rec = self.world.rec(base)
            if rec is None or not asc.is_gear(rec):
                raise Refused(f"{base} is no gear this shard can make Ascended")
            new = self.world.would_be(base)
            effects = [plain(e) for e in rows(db, "itemeffect", {"patternid": base})] if table(db, "itemeffect") else []
        else:
            if not asc.is_gear(vals):
                raise Refused(f"{vals['itemname']!r} is no gear to make Ascended")
            new = ascended_values(vals)
        nid = self.asc_id()
        new = dict(new, patternid=nid)
        self.ops.append(("itempattern", None, None, new))
        self.say(f"make {nid} {new['itemname']!r} (base {base})")
        for e in effects:
            e = dict(e, patternid=nid)
            for c in ("line", "target_line"):
                if c in e:
                    e[c] = asc.scaled_line(e[c])
            self.ops.append(("itemeffect", None, None, e))
        self.ops.append(("itemascended", None, None, {"patternid": nid, "base": base, "made": now(), "source": TOOL}))
        return nid


def short(v, n=120):
    s = v if isinstance(v, str) else repr(v)
    return s if len(s) <= n else s[:n] + "..."


def item_name(db, pid):
    r = db.execute("SELECT itemname FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
    return None if r is None else r[0]


def item_named(db, name):
    found = [r[0] for r in db.execute("SELECT patternid FROM itempattern WHERE itemname = ?", (name,))]
    return found


# ---------------------------------------------------------------------------------------------- the trigger, the steps

def trigger_steps(quest_row):
    tw, tx, ty, tz = TOWER
    return [
        {"do": "lock", "lock": LOCK},
        {"do": "clock", "seconds": 3600, "empty": 600},
        {"do": "hide", "npc": AERETH, "radius": 30, "minutes": 65},
        {"do": "wait", "seconds": 5},
        {"do": "spawn", "npc": f"#{quest_row}", "hostile": True, "spread": 0.5, "minutes": 65, "world": tw, "x": tx,
         "y": ty, "z": tz},
        {"do": "broadcast", "text": "Sarek the Betrayer: \"Arios sends me another of his pups? Then die in my tower.\""},
        {"do": "awaitdead"},
        {"do": "broadcast", "text": "Sarek the Betrayer falls, and the stone of the tower is still."},
        {"do": "end"},
        {"do": "despawn"},
        {"do": "hide", "npc": AERETH, "radius": 30, "minutes": 0},
    ]


TRIGGER_NOTE = ("Quest 101029 (Sarek the Betrayer) step 18's hand-in to Arios fires it; step 19's teleport sets the "
                "quester in this room (Archon Aereth's tower, a different tower from the random Sarek's: a GM's work "
                "board entry 709, the forum's \"a different tower\"): Aereth out of view for the run, five seconds, "
                "then the quest Sarek (his row, spawn-only by row) hostile for an hour at most, ten minutes with no "
                "one here ending it; one at a time. A run ended without his kill puts the quester back to step 18. "
                f"{TOOL}")
BUSY = "Sarek the Betrayer stands against another already: wait until he falls."


def anvil_events(old, fight_row, minion_row):
    w, x, z = ANVIL_AT
    evs = json.loads(old) if old else []
    return evs + [
        {"on": "done", "do": "hide", "npc": ANVIL, "minutes": 30},
        {"on": "done", "do": "spawn", "npc": f"#{fight_row}", "hostile": True, "group": True, "minutes": 20, "world": w,
         "x": x, "y": 132.031, "z": z},
        {"on": "done", "do": "spawn", "npc": f"#{minion_row}", "hostile": True, "group": True, "count": 3, "minutes": 20,
         "world": w, "x": x, "y": 132.031, "z": z},
    ]


def step19_events(seradesh_row):
    lw, lx, ly, lz = LANDING
    sw, sx, sy, sz = SERADESH_RISE
    return [{"on": "start", "do": "teleport", "world": lw, "x": lx, "y": ly, "z": lz},
            {"on": "done", "do": "spawn", "npc": f"#{seradesh_row}", "minutes": 30, "world": sw, "x": sx, "y": sy, "z": sz}]


STEP18_EVENTS = [{"on": "done", "do": "trigger", "trigger": TRIGGER, "text": BUSY}]
OLD19 = [{"on": "start", "do": "teleport", "world": 1, "x": 4307, "y": 219.3, "z": 2576}]
OLD10 = [{"on": "done", "do": "popup", "text": "The Anvil breaks. Sarek's walls weaken."}]
J19_OLD = "The geode talisman takes you to Sarek's tower: defeat Sarek the Betrayer."
J19_NEW = ("The geode talisman takes you to the tower where Sarek waits for you, a construct tower of Geomancer's "
           "Citadel: defeat Sarek the Betrayer.")
J20_OLD = "Speak with Archon Seradesh in Sarek's tower."
J20_NEW = ("Speak with Archon Seradesh, who comes to you when Sarek falls; she also keeps watch near the Anvil of "
           "Sarek.")


def template_row(db, name):
    r = rows(db, "classtemplates", {"name": name})
    return r[0] if r else None


def new_template(base, name, changes, note):
    t = {k: v for k, v in plain(base).items()}
    t.update(changes)
    t.update(name=name, note=note, source=SRC)
    return t


def bare_template(db, name, hp_pct, note):
    """A template of its creature's own class and melee: every column its default, hp_pct and the words."""
    cols = [r[1] for r in db.execute('PRAGMA table_info("classtemplates")')]
    dflt = {r[1]: r[4] for r in db.execute('PRAGMA table_info("classtemplates")')}
    t = {}
    for c in cols:
        d = dflt[c]
        if d is None:
            t[c] = None
        elif d.startswith("'"):
            t[c] = d.strip("'")
        else:
            t[c] = float(d) if "." in d else int(d)
    t.update(name=name, **{"class": -1}, hp_pct=hp_pct, abilities="[]", note=note, source=SRC)
    return t


def clone_npc(p, base, name, at, level, npc_type, **more):
    w, x, y, z = at
    r = plain(base)
    nid = p.npc_id()
    if r.get("npc_id") == base["id"]:
        r["npc_id"] = nid  # a row's own id, as its original's is
    r.update(id=nid, npc_name=name, world=w, x_coord=x, y_coord=y, z_coord=z, npc_level=level, npc_type=npc_type,
             **more)
    return r


# ---------------------------------------------------------------------------------------------- the plan

def find_made_npc(p, name, at, reach=NEAR):
    """The row of the name within reach of at (world, x, z) that this tool made (its log), else None."""
    for r in named_near(p.db, name, at, reach):
        if table(p.db, LOG) and p.db.execute(f"SELECT 1 FROM {LOG} WHERE tbl = 'npcs' AND before IS NULL AND rowkey = ?",
                                              (dumps({"id": r["id"]}),)).fetchone():
            return r
    return None


def build(p):
    db = p.db
    for t in ("npcs", "npctemplate", "classtemplates", "npcspawnonly", "quests", "queststeps", "gmtriggers", "loottables",
              "npcloot", "npccoins", "itempattern", "itemeffect", "itemascended", "gm_audit", "balancelevel", "charquests"):
        if not table(db, t):
            raise Refused(f"no table {t}")

    # ---- the rows as the entry read them
    rnd = named_near(db, SAREK, RANDOM_AT)
    if len(rnd) != 1 or rnd[0]["npc_level"] != 67:
        raise Refused(f"the random Sarek: {[(r['id'], r['npc_level']) for r in rnd]} at his place, not one of level 67")
    rnd = rnd[0]
    tw, tx, ty, tz = TOWER
    others = [r["id"] for r in rows(db, "npcs", {"npc_name": SAREK})
              if r["id"] != rnd["id"] and not (r["world"] == tw and math.hypot(r["x_coord"] - tx, r["z_coord"] - tz) <= NEAR)]
    if others:
        raise Refused(f"other rows named {SAREK}: {others}")
    sera = named_near(db, SERADESH, SERADESH_AT)
    sera_anvil = named_near(db, SERADESH, SERADESH_ANVIL_AT)
    anvil = named_near(db, ANVIL, ANVIL_AT)
    aereth = named_near(db, AERETH, AERETH_AT)
    arios = named_near(db, ARIOS, ARIOS_AT)
    care = named_near(db, CARETAKER, CARETAKER_AT)
    for what, rs in (("Archon Seradesh by Sarek", sera), ("Archon Seradesh by the Anvil", sera_anvil),
                     ("Archon Aereth", aereth), ("step 26's Arios", arios), ("the level 60 terran caretaker", care)):
        if len(rs) != 1:
            raise Refused(f"{what}: {len(rs)} rows at the entry's place")
    sera, aereth, arios, care = sera[0], aereth[0], arios[0], care[0]
    standing_anvil = [r for r in anvil if r["npc_type"] & 0x80]
    if len(standing_anvil) != 1:
        raise Refused(f"the Anvil of Sarek: {[(r['id'], r['npc_type']) for r in anvil]} at his place, not one with the NPB bit")
    anvil = standing_anvil[0]
    if (arios["npc_level"], arios["npc_type"]) != (60, 0):
        raise Refused(f"step 26's Arios is level {arios['npc_level']}, type {arios['npc_type']}: not as read (60, 0)")
    if sera["npc_type"] & 0x80 == 0:
        raise Refused("Archon Seradesh by Sarek is no talker")

    # ---- the quest's steps as read
    q = p.one("quests", {"questid": QUEST}, "quest 101029")
    if q["name"] != SAREK or q["giver"] != ARIOS:
        raise Refused(f"quest {QUEST} is {q['name']!r} of {q['giver']!r}")
    st = {s["step"]: s for s in rows(db, "queststeps", {"questid": QUEST})}
    shape = {3: ("talk", ANVIL), 10: ("show", ANVIL), 18: ("give", ARIOS), 19: ("kill", SAREK), 20: ("talk", SERADESH),
             26: ("kill", ARIOS)}
    for n, (kind, target) in shape.items():
        if n not in st or (st[n]["kind"], st[n]["target"]) != (kind, target):
            raise Refused(f"quest {QUEST} step {n} is {st.get(n) and (st[n]['kind'], st[n]['target'])}, not {(kind, target)}")
    if st[18]["items"] != "701101":
        raise Refused(f"step 18 takes {st[18]['items']!r}, not the channeling stone 701101")

    # ---- the records the loot names
    for pid, name in CLASS_ITEMS + CORPSE_ITEMS + [TRIDENT_LOOK, HEART_LOOK]:
        if item_name(db, pid) != name:
            raise Refused(f"item {pid} is {item_name(db, pid)!r}, not {name!r}")
    if len(item_named(db, DRAUGHT)) != 1:
        raise Refused(f"not one record {DRAUGHT!r} (Serpent's Heart summons it by name)")

    # ================================================================ the random Sarek
    rt = template_row(db, TPL_RANDOM)
    u = rows(db, "npctemplate", {"name": TPL_RANDOM})
    if rt is None or not u or u[0]["template"] != TPL_RANDOM:
        raise Refused(f"npctemplate/classtemplates {TPL_RANDOM!r} not as read (his own copy of the roamer)")
    if (rt["resist_kind"], rt["resist_pct"]) != ("PR,DR,AR", 50):
        raise Refused(f"{TPL_RANDOM!r}'s resists are {rt['resist_kind']!r} {rt['resist_pct']}, not as read")
    random_note = (rt["note"] if TOOL in rt["note"] else
                   f"A GM's work board entry 709 (the owner, 2026-11-22): the random Sarek, ~1.5M effective (hp_pct "
                   f"{RANDOM_HP}, {max_hp(db, 67, RANDOM_HP):,} at 67), no Critical Hit (his level's melee), Sarek's "
                   f"Blight 8,000 disease (DR) on his target every 20 s; resists kept. {TOOL}. Before: " + rt["note"])
    p.set("classtemplates", rt, {"hp_pct": (100, RANDOM_HP), "special_every": (30.0, 0), "special_chance": (0, 0),
                                 "special_first": (15.0, 0), "special_jitter": (5.0, 0),
                                 "abilities": ("[]", jsc([BLIGHT])), "note": (rt["note"], random_note)},
          f"the random Sarek's template {TPL_RANDOM!r}")

    # ================================================================ the quest Sarek
    made = find_made_npc(p, SAREK, (tw, tx, tz))
    if made is None and named_near(db, SAREK, (tw, tx, tz)):
        raise Refused(f"a {SAREK} stands in Aereth's tower room already, not this tool's")
    qrow = made["id"] if made else None
    if made is None:
        r = clone_npc(p, rnd, SAREK, TOWER, 67, 0)
        qrow = r["id"]
        p.add("npcs", r, ("id",), f"the quest Sarek, row {qrow} in Aereth's tower room")
    p.add("npcspawnonly", {"name": f"#{qrow}", "note": "the quest Sarek: raised by quest 101029's trigger "
                                                         f"{TRIGGER!r} (board 709)", "source": SRC}, ("name",),
          f"the quest Sarek #{qrow} spawn-only")
    qt = new_template(template_row(db, TPL_QUEST) or rt, TPL_QUEST,
                      {"hp_pct": QUEST_HP, "special_every": 0, "special_chance": 0, "special_first": 0,
                       "special_jitter": 0, "abilities": "[]"},
                      f"A GM's work board entry 709: the quest Sarek, ~500k effective (\"one group of 60s\"): hp_pct "
                      f"{QUEST_HP} ({max_hp(db, 67, QUEST_HP):,} at 67), no Critical Hit, no Blight (the random one's "
                      f"alone); resists as the random one's. {TOOL}")
    qt["resist_kind"], qt["resist_pct"] = rt["resist_kind"], rt["resist_pct"]
    if template_row(db, TPL_QUEST) is None:
        p.add("classtemplates", qt, ("name",), f"template {TPL_QUEST!r}")
    else:
        p.say(f"template {TPL_QUEST!r}: there")
    p.add("npctemplate", {"name": f"#{qrow}", "template": TPL_QUEST, "note": "the quest Sarek (board 709)", "source": SRC},
          ("name",), f"the quest Sarek's row key -> {TPL_QUEST!r}")
    p.add("gmtriggers", {"slot": trigger_slot(db), "name": TRIGGER, "enabled": 1, "scope": "near", "radius": 40,
                         "world": tw, "x": tx, "y": ty, "z": tz, "steps": jsc(trigger_steps(qrow)), "notes": TRIGGER_NOTE},
          ("name",), f"trigger {TRIGGER!r}")

    # Archon Seradesh beside the random Sarek: spawn-only, raised at step 19's done
    p.add("npcspawnonly", {"name": f"#{sera['id']}", "note": "raised by quest 101029 step 19's done (board 709)",
                           "source": SRC}, ("name",), f"Archon Seradesh #{sera['id']} spawn-only")

    # the quest's steps 18-20
    p.set("queststeps", st[18], {"events": ("", jsc(STEP18_EVENTS))}, "step 18 fires the trigger")
    p.set("queststeps", st[19], {"events": (jsc(OLD19), jsc(step19_events(sera["id"]))), "journal": (J19_OLD, J19_NEW)},
          "step 19: the teleport to Aereth's tower, Seradesh at its done")
    p.set("queststeps", st[20], {"journal": (J20_OLD, J20_NEW)}, "step 20's journal")

    # ================================================================ the Anvil
    at = template_row(db, TPL_ANVIL)
    if at is None or json.loads(at["abilities"]) not in (OLD_ZONE, NEW_ZONE):
        raise Refused(f"template {TPL_ANVIL!r} is {at and at['abilities']!r}, not tools/anvil_1956_20261113.py's zone")
    anvil_note = (at["note"] if TOOL in at["note"] else
                  at["note"] + f"; board 709: 99,999 (the entry's bolt), the one a quest sends to him spared. {TOOL}")
    p.set("classtemplates", at, {"abilities": (jsc(OLD_ZONE), jsc(NEW_ZONE)), "note": (at["note"], anvil_note)},
          "the Anvil's zone 99,999, sparing the quester")
    aw, ax, az = ANVIL_AT
    fight = [r for r in rows(db, "npcs", {"npc_name": ANVIL}) if r["id"] != anvil["id"]]
    madef = find_made_npc(p, ANVIL, ANVIL_AT)
    if fight and (len(fight) != 1 or madef is None or fight[0]["id"] != madef["id"]):
        raise Refused(f"other rows named {ANVIL}: {[r['id'] for r in fight]}")
    if madef is None:
        r = clone_npc(p, anvil, ANVIL, (aw, ax, anvil["y_coord"], az), anvil["npc_level"], 0)
        frow = r["id"]
        p.add("npcs", r, ("id",), f"the Anvil's fighting row {frow}")
    else:
        frow = madef["id"]
    p.add("npcspawnonly", {"name": f"#{frow}", "note": "the Anvil that fights: steps 3 and 10 of quest 101029 (board 709)",
                           "source": SRC}, ("name",), f"#{frow} spawn-only")
    p.add("classtemplates", bare_template(db, TPL_ANVIL_FIGHT, ANVIL_HP,
                                          f"A GM's work board entry 709: the Anvil of Sarek turned hostile at the "
                                          f"questor's talk, ~150k effective: hp_pct {ANVIL_HP} "
                                          f"({max_hp(db, 65, ANVIL_HP):,} at 65); no kick zone. {TOOL}"),
          ("name",), f"template {TPL_ANVIL_FIGHT!r}")
    p.add("npctemplate", {"name": f"#{frow}", "template": TPL_ANVIL_FIGHT, "note": "board 709", "source": SRC}, ("name",),
          f"#{frow} -> {TPL_ANVIL_FIGHT!r}")
    mins = rows(db, "npcs", {"npc_name": MINION})
    madem = find_made_npc(p, MINION, ANVIL_AT, 10)
    if mins and (len(mins) != 1 or madem is None):
        raise Refused(f"rows named {MINION}: {[r['id'] for r in mins]}")
    if madem is None:
        r = clone_npc(p, care, MINION, (aw, ax + 4, anvil["y_coord"], az), 63, 0, size=1.0)
        mrow = r["id"]
        p.add("npcs", r, ("id",), f"{MINION} row {mrow} (a terran caretaker's body, level 63)")
    else:
        mrow = madem["id"]
    p.add("npcspawnonly", {"name": MINION.lower(), "note": "raised with the Anvil (board 709)", "source": SRC}, ("name",),
          f"{MINION} spawn-only")
    p.add("classtemplates", bare_template(db, TPL_MINION, MINION_HP,
                                          f"A GM's work board entry 709: a Minion of Sarek, three a fight at the Anvil, "
                                          f"~30k: hp_pct {MINION_HP} ({max_hp(db, 63, MINION_HP):,} at 63). {TOOL}"),
          ("name",), f"template {TPL_MINION!r}")
    p.add("npctemplate", {"name": TPL_MINION, "template": TPL_MINION, "note": "board 709", "source": SRC}, ("name",),
          f"{MINION} -> {TPL_MINION!r}")
    for n, old in ((3, ""), (10, jsc(OLD10))):
        want = jsc(anvil_events(old, frow, mrow))
        p.set("queststeps", st[n], {"events": (old, want)}, f"step {n}: the Anvil turns at its done")

    # ================================================================ Arios's last fight
    if rows(db, "npctemplate", {"name": "arios the battered"}):
        raise Refused("npctemplate has a row for every Arios by name")
    p.add("classtemplates", bare_template(db, TPL_ARIOS, ARIOS_HP,
                                          f"Quest 101029 step 26's Arios before the Rallosian Arbiter, sized as the "
                                          f"quest Sarek (a player's report of 2026-11-22, no figure for him: ours): "
                                          f"~500k at 60, a group of 60s dealing all of it: hp_pct {ARIOS_HP} "
                                          f"({max_hp(db, 60, ARIOS_HP):,}). {TOOL}"),
          ("name",), f"template {TPL_ARIOS!r}")
    p.add("npctemplate", {"name": f"#{arios['id']}", "template": TPL_ARIOS, "note": "step 26's Arios (board 709)",
                          "source": SRC}, ("name",), f"#{arios['id']} -> {TPL_ARIOS!r}")

    # ================================================================ the loot
    trident = make_item(p, TRIDENT, TRIDENT_LOOK[0], {
        "classuse": 1 << PAL, "weapondamage": 815, "levelreq": 60, "maxhp": 10578, "itemcost": 4792, "trade": 0,
        "lore": 0, "rent": 1, "weaponproc": 0, "procanim": 0,
        "str": 0, "sta": 20, "agi": 0, "wis": 0, "dex": 30, "cha": 0, "intelligence": 0, "HPMAX": 100, "POWMAX": 0,
        "PoT": 0, "HoT": 0, "AC": 0, "PR": 0, "DR": 0, "FR": 0, "CR": 0, "LR": 0, "AR": 0,
        "itemdesc": "A trident of living stone that Sarek the Betrayer carried.\nProcs 100 power to its wielder."})
    heart = make_item(p, HEART, HEART_LOOK[0], {
        "classuse": 1 << ALC, "levelreq": 60, "maxstack": 1, "trade": 0, "lore": 0, "rent": 1, "itemcost": 3614,
        "itemdesc": "A serpent's heart turned to stone in Sarek's tower. Summons three level 60 draughts."},
        effect={"line": f"Self, Summon {DRAUGHT} x3, Unlimited Uses", "name": HEART, "icon": 0, "breaks": "", "wear": 0,
                "area": 0.0, "source": SRC, "body": 0, "target_line": "", "charges": 0, "cooldown": 300.0})
    class_rows = []
    for pid, _ in CLASS_ITEMS:
        class_rows.append(p.ascended_of(pid))
    class_rows.append(p.ascended_of(trident[0], trident[1]) if trident[1] else p.ascended_of(trident[0]))
    class_rows.append(heart[0])  # a stone with a use: no gear, as it is
    corpse_rows = [p.ascended_of(pid) for pid, _ in CORPSE_ITEMS[:3]] + [CORPSE_ITEMS[3][0]]
    for tname, pids in ((T_CLASS, class_rows), (T_CORPSE, corpse_rows)):
        have = sorted(r["patternid"] for r in rows(db, "loottables", {"tablename": tname}))
        if have and have != sorted(pids):
            raise Refused(f"table {tname!r} has {have}, not {sorted(pids)}")
        if have:
            p.say(f"table {tname!r}: there")
            continue
        for pid in pids:
            p.ops.append(("loottables", None, None, {"tablename": tname, "patternid": pid, "weight": 1, "minlevel": 1,
                                                     "maxlevel": 70, "source": SRC}))
        p.say(f"table {tname!r}: {len(pids)} rows {pids}")
    for tname, note in ((T_CLASS, "one class item a kill (board 709)"),
                        (T_CORPSE, "one of his 2009 corpse's four a kill (board 709; the roll ours)")):
        p.add("npcloot", {"npc_name": SAREK.lower(), "tablename": tname, "chance": 100, "rolls": 1, "note": note,
                          "source": SRC}, ("npc_name", "tablename"), f"npcloot {tname!r}")
    p.add("npccoins", {"npc_name": SAREK.lower(), "coins_min": COINS, "coins_max": COINS,
                       "note": "live-era footage: ~800 tunar on his 2009 corpse (board 709)", "source": "video-audit"},
          ("npc_name",), "his corpse's tunar")

    # ---- what the report says
    held = db.execute("SELECT step, done, count(*) FROM charquests WHERE questid = ? GROUP BY 1, 2", (QUEST,)).fetchall()
    p.say(f"quest {QUEST} holders (step, done, n): {held or 'none'}; one on step 19 with no run is put back to 18 "
          f"(the stone handed back) 10 s after the start")
    p.say(f"hit points: random {max_hp(db, 67, RANDOM_HP):,}, quest {max_hp(db, 67, QUEST_HP):,}, Anvil "
          f"{max_hp(db, 65, ANVIL_HP):,}, Minion {max_hp(db, 63, MINION_HP):,}, Arios {max_hp(db, 60, ARIOS_HP):,}")
    p.made.update(quest_row=qrow, fight_row=frow, minion_row=mrow, trident=trident[0], heart=heart[0])


def trigger_slot(db):
    r = db.execute("SELECT slot FROM gmtriggers WHERE name = ?", (TRIGGER,)).fetchone()
    if r:
        return r[0]
    return (db.execute("SELECT max(slot) FROM gmtriggers").fetchone()[0] or 8) + 1


def make_item(p, name, look, changes, effect=None):
    """The record of the name: the one there (this tool's), else made from look's record with changes. (id, values
    when made else None)."""
    db = p.db
    have = item_named(db, name)
    if have:
        if len(have) != 1 or not (table(db, LOG) and db.execute(
                f"SELECT 1 FROM {LOG} WHERE tbl = 'itempattern' AND before IS NULL AND rowkey = ?",
                (dumps({"patternid": have[0]}),)).fetchone()):
            raise Refused(f"a record {name!r} exists already, not this tool's: {have}")
        p.say(f"{name}: {have[0]} there")
        return have[0], None
    base = plain(rows(db, "itempattern", {"patternid": look})[0])
    vals = dict(base, **changes, itemname=name, patternid=p.item_id())
    p.ops.append(("itempattern", None, None, vals))
    p.say(f"make {vals['patternid']} {name!r} (the look of {look})")
    effects = []
    if effect:
        e = dict(effect, patternid=vals["patternid"])
        p.ops.append(("itemeffect", None, None, e))
        effects.append({k: v for k, v in e.items() if k != "patternid"})
    return vals["patternid"], vals


# ---------------------------------------------------------------------------------------------- writing

def keyof(db, tbl, row, rid):
    k = pk(db, tbl)
    return {c: row[c] for c in k} if k else {"rowid": rid}


def audit(db, action, tbl, key, before, after):
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (now(), ACCOUNT, WHO, VIA, action, tbl, dumps(key), None if before is None else dumps(before),
                      None if after is None else dumps(after), DETAIL))
    return cur.lastrowid


def apply(db, p):
    db.execute(LOG_DDL)
    at, n = now(), 0
    for tbl, rid, before, new in p.ops:
        if before is None:
            cs = list(new)
            cur = db.execute(f'INSERT INTO "{tbl}" ({", ".join(chr(34) + c + chr(34) for c in cs)}) '
                             f'VALUES ({", ".join("?" * len(cs))})', [new[c] for c in cs])
            rid = cur.lastrowid
            action = "insert"
        else:
            if plain(by_rowid(db, tbl, rid)) != plain(before):
                raise Refused(f"{tbl} rowid {rid} changed while planning")
            db.execute(f'UPDATE "{tbl}" SET {", ".join(chr(34) + c + chr(34) + " = ?" for c in new)} WHERE rowid = ?',
                       [*new.values(), rid])
            action = "update"
        after = plain(by_rowid(db, tbl, rid))
        b = plain(before)
        key = keyof(db, tbl, after, rid)
        a = audit(db, action, tbl, key, b, after)
        db.execute(f"INSERT INTO {LOG} (tbl, rid, rowkey, before, after, audit, at) VALUES (?, ?, ?, ?, ?, ?, ?)",
                   (tbl, rid, dumps(key), None if b is None else dumps(b), dumps(after), a, at))
        n += 1
    return n


HOLDERS = (("charinventory", "patternid"), ("charbank", "patternid"), ("npcitem", "patternid"), ("lootpool", "patternid"),
           ("npcheld", "patternid"), ("auctions", "patternid"), ("mailitems", "patternid"))


def held(db, pid):
    for t, c in HOLDERS:
        if table(db, t) and c in [r[1] for r in db.execute(f'PRAGMA table_info("{t}")')] and \
                db.execute(f"SELECT 1 FROM {t} WHERE {c} = ? LIMIT 1", (pid,)).fetchone():
            return t
    return None


def undo(db):
    if not table(db, LOG):
        return 0, []
    back, left, kept = 0, [], set()
    for seq, tbl, rid, rowkey, before, after in db.execute(
            f"SELECT seq, tbl, rid, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        have = plain(by_rowid(db, tbl, rid))
        if have is None or dumps(have) != after:
            left.append(f"{tbl} {rowkey}: changed since, left as it is")
            continue
        if before is None and tbl in ("itempattern", "itemeffect", "itemascended"):
            pid = json.loads(after)["patternid"]
            h = pid in kept or held(db, pid)
            if h:
                kept.add(pid)
                left.append(f"{tbl} {pid}: kept, " + (f"{h} holds it" if isinstance(h, str) else "with its record"))
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


def dump(p):
    """What the Go side reads, as JSON: the ids made, the trigger, the templates and the steps' events as written."""
    out = {"made": p.made, "templates": {}, "events": {}, "trigger": None}
    for tbl, _, before, v in p.ops:
        row = dict(plain(before) or {}, **v)
        if tbl == "gmtriggers":
            out["trigger"] = row
        elif tbl == "classtemplates":
            out["templates"][row["name"]] = row
        elif tbl == "queststeps" and "events" in v:
            out["events"][row["step"]] = json.loads(row["events"])
    return out


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--shard", required=True, choices=("heirloom", "live"))
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    g.add_argument("--dump", action="store_true")
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
        p = Plan(db, a.shard)
        try:
            build(p)
        except Refused as e:
            db.execute("ROLLBACK")
            print("\n".join(p.lines))
            print(f"REFUSED, nothing written: {e}")
            return 1
        if a.dump:
            db.execute("ROLLBACK")
            print(json.dumps(dump(p), indent=1, default=str))
            return 0
        print(f"{a.shard}:")
        print("\n".join(p.lines))
        counts = {}
        for tbl, _, before, _ in p.ops:
            k = (tbl, "insert" if before is None else "update")
            counts[k] = counts.get(k, 0) + 1
        print("totals: " + (", ".join(f"{k[1]} {k[0]} {v}" for k, v in sorted(counts.items())) or "nothing"))
        if not a.apply:
            db.execute("ROLLBACK")
            print("dry run: nothing written (--apply to write)")
            return 0
        if not p.ops:
            db.execute("ROLLBACK")
            print("nothing to write: all in place")
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
