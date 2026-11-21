#!/usr/bin/env python3
"""jibrish's requests of 2026-11-21 on his board (posts #196-#200), the owner approving them as submitted. Three
parts, each run alone with --part (default: every part the shard takes):

  196  (Live)  board #196-#198, work board 1865: the necromancer's level 60 pets as Heirloom has them (Heirloom done,
       entry 1864, audits 131600-131605, 131728-131730, 131742-131744: left as it is). Rows found by NAME as
       petpass_spells_20261120 made them: the spell "Swarming Death" (Heirloom 700823) and Unholy Sacrament 4's pet
       (the cmcatalog row "Unholy Sacrament 4"'s add_spells: Heirloom 906301), Crimson Unholy Sacrament's (906300),
       and the Pet Graphic looks Bone Collection, Unholy Ghoulish Sacrament and Wraith's Unholy Sacrament give
       (901004, 900994, 900995). #196: petstats model 6E5E0E2A (the red cloud) on Swarming Death and Unholy Sacrament
       4's pet, the pet named "Red Swarming Death", its spell "Red Swarming Death (60)" with "CM: Summons a Red
       Swarming Death to aid you.", its spelleffect line "Summons a Red Swarming Death to aid you", and cmcatalog
       "Unholy Sacrament 4" adds / "Bone Collection" removes "Red Swarming Death (60)@60" (spell ids unchanged).
       #197/#198: petstats level 54 on those six. Nothing else (the owner's models on 900994/900995 of 2026-11-21 and
       cmcatalog 6371-6374's remove_spells are not touched). A column neither as it was nor as asked refuses.
  199  (Live)  board #199, work board 1869: the human Freeport alchemist, bard and cleric chains copied from Heirloom
       (--source, read only; its CURRENT rows): itempattern 991500 Acolyte's Mace made at that id; 8354, 8327 (itemname,
       itemdesc) and 8407 (itemname) renamed; quests, queststeps and queststeptargets of 100071-100080, 100086-100100
       whole; 100081 name and decline, 100082-100085 decline. Section 2's wares are checked only (a merchant who does not
       sell one refuses). Section 4's characters part-way are moved (STEP_MOVES; a count kept only where the old and new
       step loot the same thing) and given 700279 / 8357 where it says so (a full pack is reported for a GM). Entry 894
       (the bard's L1-L5) must be on Live first: 100081-100085's rows equal Heirloom's but for the columns copied here.
       Every target row must be as Heirloom's was BEFORE its audits (128384-128486, 128489-128529, 128533-128534,
       130203-130279, 131796-131827: "stop and ask" otherwise) or as Heirloom's is now; an id taken refuses. Item ids
       the rows name are Live's: an id whose Live record has another name maps to Live's one record of Heirloom's
       name (991500 is made at its own id), else refuses (Live's alchemical tome is 981971, Heirloom's 997854).
  200  (both)  board #200, work board 1872: the guide loot tool's name bindings (npcloot, source guide-loot-20261121)
       of eight names (bandit, boss hurg, crocodile, deathfist legionnaire, nasehir cleric/rogue/warrior/wizard; 19
       rows) become the same table, chance, rolls, note and source on the passing spawns' row keys ("#<npcs.id>",
       handlers/loot.go lootNames: an open-world creature rolls its name's and its row's), and the name bindings go.
       The spawns are jibrish's table: Heirloom's are his 210 ids (KEEP below); Live's the same spawns matched by id,
       else by place and level (208: Live lacks two of Heirloom's Lakeside bandits). Boss Hurg's stray L44 row (111051)
       was removed by guide_camps_20261121 today: it is off his list, and gone. Refused unless the 19 bindings are there
       (or none and every row key in place), every listed spawn stands under its name, and every spawn of the name is
       on the list (KEEP or OFF). guide_loot_20261121.py run again binds none of them by name (a row of the name
       already drops each item, its drops()).

What takes effect when: npcloot at each corpse (part 200 at once); petstats, spell records, cmcatalog, quests and
items at the server's start; a player's quests and pack are held while they play: run 196 and 199 in a keep
restart's before.sh.

Dry run by default (every change printed). --apply writes in one transaction, a gm_audit line a row (accountid 1,
dab, via tool), every row's before and after in jib_requests_1121_log; a second --apply writes nothing. --undo puts back
what the log holds for the parts named (a row changed since is left and said).

    python3 -E -s tools/jib_requests_20261121.py --db eqoa.db --shard heirloom|live [--part 196,199,200]
        [--source HEIRLOOM.db] [--apply | --undo]
"""
import argparse
import datetime
import json
import re
import sqlite3
import sys

LOG = "jib_requests_1121_log"
WHO, ACCOUNT, VIA = "dab", 1, "tool"
PARTS = ("196", "199", "200")
SHARD_PARTS = {"heirloom": ("200",), "live": ("196", "199", "200")}


def detail(post, entry):
    return f"jibrish's request (board #{post}, work board {entry}), the owner approves as submitted"


KEYS = {"petstats": ("spellid",), "spellpattern": ("spellid",), "spelleffect": ("spellid",), "cmcatalog": ("id",),
        "itempattern": ("patternid",), "quests": ("questid",), "queststeps": ("questid", "step"),
        "queststeptargets": ("questid", "step", "target"), "charquests": ("serverid", "questid"),
        "charinventory": ("itemid",), "npcloot": ("npc_name", "tablename")}

LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    part   TEXT NOT NULL,
    tbl    TEXT NOT NULL,
    rowkey TEXT NOT NULL,   -- the key (JSON)
    before TEXT,            -- the whole row before, its rowid as _rowid_ (JSON; NULL: there was none)
    after  TEXT,            -- the whole row after, its rowid as _rowid_ (JSON; NULL: removed)
    audit  INTEGER NOT NULL,
    at     TEXT NOT NULL
)"""

# ---------------------------------------------------------------------------------------------- 196
RED_CLOUD = 0x6E5E0E2A  # 1851657770
NECRO_ROWS = {  # role -> (how it is found, its name)
    "swarming": "the spell \"Swarming Death\" (Unholy Sacrament 4's requires)",
    "sacrament4": "Unholy Sacrament 4's pet",
    "crimson": "Crimson Unholy Sacrament's pet",
    "bone": "Bone Collection's look",
    "ghoulish": "Unholy Ghoulish Sacrament's look",
    "haunting": "Wraith's Unholy Sacrament's look",
}
CM_NAMES = {"sacrament4": "Unholy Sacrament 4", "bonecoll": "Bone Collection", "crimson": "Crimson Unholy Sacrament",
            "ghoulish": "Unholy Ghoulish Sacrament", "haunting": "Wraith's Unholy Sacrament"}
SPELL_NAMES = {"swarming": "Swarming Death", "bone": "Bone Swarming Death (60)", "crimson": "Red Swarming Death (60)",
               "ghoulish": "Ghoulish Death", "haunting": "Haunting Death"}
OLD_PET, NEW_PET = "Swarming Death", "Red Swarming Death"
OLD_SPELL, NEW_SPELL = "Swarming Death (60)", "Red Swarming Death (60)"
OLD_DESC, NEW_DESC = "CM: Summons a Swarming Death to aid you.", "CM: Summons a Red Swarming Death to aid you."
OLD_LINE, NEW_LINE = "Summons a Swarming Death to aid you", "Summons a Red Swarming Death to aid you"
OLD_CM, NEW_CM = "Swarming Death (60)@60", "Red Swarming Death (60)@60"
OLD_BODY = -474126599  # 0xE3BD66F9, the body of 2026-11-20 21:16 UTC (Heirloom audits 129289/129290)
LEVEL = 54
OLD_LEVELS = {"swarming": 53, "sacrament4": 55, "crimson": 55, "bone": 55, "ghoulish": 55, "haunting": 55}

# ---------------------------------------------------------------------------------------------- 199
SOURCE_AUDITS = ((128384, 128486), (128489, 128529), (128533, 128534), (130203, 130279), (131796, 131827))
SOURCE_TABLES = ("itempattern", "quests", "queststeps", "queststeptargets")
SOURCE_WHO = "jibrish"
WHOLE = list(range(100071, 100081)) + list(range(100086, 100101))
COLUMNS = {100081: ("name", "decline"), 100082: ("decline",), 100083: ("decline",), 100084: ("decline",),
           100085: ("decline",)}
NEW_ITEM = 991500
RENAMES = {8354: ("itemname", "itemdesc"), 8327: ("itemname", "itemdesc"), 8407: ("itemname",)}
MUST_EXIST = (700279, 8357)
WARES = (("Merchant Yulia", 8320, 100074), ("Alana", 8322, 100077), ("Merchant Lorili Ashgo", 8357, 100078),
         ("Merchant Charlie", 8325, 100079), ("Merchant Shohan", 8318, 100094), ("Merchant Shohan", 8316, 100094))
# section 4: quest -> {old step: new step}; a step not listed keeps its number
STEP_MOVES = {
    100073: {1: 1, 2: 2, 3: 3, 4: 4},
    100074: {1: 1, 2: 1, 3: 2},
    100075: {1: 2, 2: 3},
    100077: {1: 1, 2: 2, 3: 3, 4: 3},
    100078: {1: 1, 2: 2, 3: 3, 4: 4, 5: 4},
    100079: {1: 1, 2: 1, 3: 2, 4: 3, 5: 4, 6: 5, 7: 6, 8: 7},
    100080: {1: 1, 2: 2, 3: 3, 4: 4, 5: 5, 6: 6, 7: 6, 8: 7, 9: 9},
    100090: dict([(1, 1)] + [(s, s + 1) for s in range(2, 13)]),
    100094: {1: 1, 2: 1, 3: 2},
}
GIVES = {(100073, 3): 700279, (100073, 4): 700279, (100078, 5): 8357}  # (quest, OLD step) -> item given if not held
INV_CAP = 100
ITEM_FIELDS = ("items", "rewarditems")
JSON_ITEM_FIELDS = ("events", "choices", "byclass")
SPELL_FIELDS = ("spells",)

# ---------------------------------------------------------------------------------------------- 200
LOOT_SRC = "guide-loot-20261121"
LOOT_NAMES = ("bandit", "boss hurg", "crocodile", "deathfist legionnaire", "nasehir cleric", "nasehir rogue",
              "nasehir warrior", "nasehir wizard")
LOOT_BINDINGS = 19
# The spawns that keep a camp's trophies (KEEP) and those taken off (OFF), each shard's ids. Heirloom's KEEP is
# jibrish's 210 (board #200's SQL); Live's the same spawns (same id and name, else the nearest of the name within 200
# at the same level: Live's later rows have their own ids; Live lacks Heirloom's bandits 162973 and 162974). OFF is
# every other spawn of the names on 2026-11-21 08:00 UTC (his table's "Taken off"), Boss Hurg's stray 111051 among
# them (removed by guide_camps_20261121).
SPAWNS = {
    'heirloom': {
      "keep": {
        'bandit': [779, 780, 781, 782, 783, 784, 785, 786, 787, 788, 789, 790, 791, 792, 793, 794, 103541, 103542,
            103543, 103558, 103559, 107603, 107604, 107605, 107606, 107607, 107608, 107609, 107610, 107611, 107612,
            107613, 107614, 107615, 111676, 111677, 111679, 111680, 115098, 115099, 115100, 162973, 162974, 162975,
            190730, 190731, 190737, 190738, 190739, 190740, 190741, 190742, 190743, 190744, 190745, 190746, 190747,
            190748, 190749, 190750, 190751, 190752, 190753, 190754, 190755, 190756, 190757, 190758, 190759, 190760,
            190761, 190762, 190763, 190764, 190765, 190766, 190767, 190768, 190769, 190770, 190771, 190772, 190773,
            190774, 190775, 190776, 190777, 190778, 190779, 190780, 190781, 190782, 190783, 190784, 190785, 190786,
            190877, 191005],
        'boss hurg': [103809],
        'crocodile': [1038, 3216, 115855, 168973, 190998, 190999, 191000],
        'deathfist legionnaire': [100481, 100500, 100501, 100502, 100508, 100511, 100514, 100520, 100580, 100583,
            100590, 100593, 100596, 100598, 111204, 111206, 115820, 115826, 115853, 115931, 115932, 115958],
        'nasehir cleric': [2010, 2011, 2012, 2013, 2014, 2015, 2016, 104577, 104580, 104583, 104585, 104600, 104603,
            104606, 115719, 115814, 115839, 190919],
        'nasehir rogue': [2035, 2036, 2037, 2038, 2039, 104570, 104575, 104579, 104593, 104594, 104598, 104602,
            104616, 104617, 115718, 115816, 115840, 190921],
        'nasehir warrior': [2043, 2044, 2045, 2046, 2047, 104347, 104348, 104351, 104354, 104554, 104555, 104578,
            104584, 104595, 104596, 104601, 104607, 104618, 104619, 115716, 115717, 115721, 115731, 115784, 115788,
            115812, 115817, 115837, 115838],
        'nasehir wizard': [2049, 2050, 2051, 2052, 2053, 2054, 2055, 2056, 104574, 104576, 104591, 104597, 104599,
            104614, 115815, 190920, 190922],
      },
      "off": {
        'bandit': [104833, 104838, 108659, 108660, 108663, 108677, 108678, 108679, 108680, 108684, 108800, 108801,
            108803, 110797, 110798, 110799, 110806, 162459, 162460, 162461, 162462, 190822],
        'boss hurg': [111051],
        'crocodile': [107931, 107932, 107933, 163697],
        'deathfist legionnaire': [100613],
        'nasehir cleric': [2008, 2009, 100597, 101920, 111205],
        'nasehir rogue': [2032, 2033, 2034, 100585, 101935, 111203],
        'nasehir warrior': [2041, 2042],
        'nasehir wizard': [2048, 100503, 100507, 100509, 101924, 101925],
      },
    },
    'live': {
      "keep": {
        'bandit': [779, 780, 781, 782, 783, 784, 785, 786, 787, 788, 789, 790, 791, 792, 793, 794, 103541, 103542,
            103543, 103558, 103559, 107603, 107604, 107605, 107606, 107607, 107608, 107609, 107610, 107611, 107612,
            107613, 107614, 107615, 111676, 111677, 111679, 111680, 115098, 115099, 115100, 191097, 192116, 192117,
            192123, 192124, 192125, 192126, 192127, 192128, 192129, 192130, 192131, 192132, 192133, 192134, 192135,
            192136, 192137, 192138, 192139, 192140, 192141, 192142, 192143, 192144, 192145, 192146, 192147, 192148,
            192149, 192150, 192151, 192152, 192153, 192154, 192155, 192156, 192157, 192158, 192159, 192160, 192161,
            192162, 192163, 192164, 192165, 192166, 192167, 192168, 192169, 192170, 192171, 192172, 192259, 192356],
        'boss hurg': [103809],
        'crocodile': [1038, 3216, 115855, 162787, 192349, 192350, 192351],
        'deathfist legionnaire': [100481, 100500, 100501, 100502, 100508, 100511, 100514, 100520, 100580, 100583,
            100590, 100593, 100596, 100598, 111204, 111206, 115820, 115826, 115853, 115931, 115932, 115958],
        'nasehir cleric': [2010, 2011, 2012, 2013, 2014, 2015, 2016, 104577, 104580, 104583, 104585, 104600, 104603,
            104606, 115719, 115814, 115839, 192303],
        'nasehir rogue': [2035, 2036, 2037, 2038, 2039, 104570, 104575, 104579, 104593, 104594, 104598, 104602,
            104616, 104617, 115718, 115816, 115840, 192305],
        'nasehir warrior': [2043, 2044, 2045, 2046, 2047, 104347, 104348, 104351, 104354, 104554, 104555, 104578,
            104584, 104595, 104596, 104601, 104607, 104618, 104619, 115716, 115717, 115721, 115731, 115784, 115788,
            115812, 115817, 115837, 115838],
        'nasehir wizard': [2049, 2050, 2051, 2052, 2053, 2054, 2055, 2056, 104574, 104576, 104591, 104597, 104599,
            104614, 115815, 192304, 192306],
      },
      "off": {
        'bandit': [104833, 104838, 108659, 108660, 108663, 108677, 108678, 108679, 108680, 108684, 108800, 108801,
            108803, 110797, 110798, 110799, 110806, 162459, 162460, 162461, 162462, 192208],
        'boss hurg': [111051],
        'crocodile': [107931, 107932, 107933, 163697],
        'deathfist legionnaire': [100613],
        'nasehir cleric': [2008, 2009, 100597, 101920, 111205],
        'nasehir rogue': [2032, 2033, 2034, 100585, 101935, 111203],
        'nasehir warrior': [2041, 2042],
        'nasehir wizard': [2048, 100503, 100507, 100509, 101924, 101925],
      },
    },
}


class Refused(Exception):
    pass


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def dumps(v):
    return json.dumps(v, sort_keys=True, separators=(",", ":"), ensure_ascii=False)


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def columns(db, tbl):
    return [r[1] for r in db.execute(f"PRAGMA table_info({tbl})")]


def rows(db, sql, args=()):
    cur = db.execute(sql, args)
    cols = [d[0] for d in cur.description]
    return [dict(zip(cols, r)) for r in cur.fetchall()]


def where(tbl):
    return " AND ".join(f'"{c}" = ?' for c in KEYS[tbl])


def keyof(tbl, r):
    return tuple(r[c] for c in KEYS[tbl])


def full(db, tbl, key):
    """the row with its rowid (_rowid_), or None."""
    r = rows(db, f"SELECT rowid AS _rowid_, * FROM {tbl} WHERE {where(tbl)}", tuple(key))
    return r[0] if r else None


def plain(r):
    return None if r is None else {c: v for c, v in r.items() if c != "_rowid_"}


def same(a, b):
    """the same value of the same storage class (what a copy must leave)."""
    return type(a) is type(b) and a == b


def same_row(a, b):
    if a is None or b is None:
        return a is None and b is None
    a, b = plain(a), plain(b)
    return a.keys() == b.keys() and all(same(a[c], b[c]) for c in a)


def loose_row(a, b, cols=None):
    """the same values (gm_audit's JSON writes a whole REAL as an integer)."""
    if a is None or b is None:
        return a is None and b is None
    a, b = plain(a), plain(b)
    cols = cols or a.keys()
    return all(c in b and a[c] == b[c] for c in cols)


class Plan:
    def __init__(self):
        self.ops = []      # (part, tbl, key, before row|None, after values|None, post); after None: delete
        self.lines = []
        self.refusals = []

    def say(self, s):
        self.lines.append(s)

    def refuse(self, s):
        self.refusals.append(s)

    def set(self, part, post, db, tbl, key, values):
        """the row at key made values (every column given); nothing when it is so. True when it changes."""
        have = full(db, tbl, key)
        if have is not None and all(same(have[c], values[c]) for c in values):
            return False
        self.ops.append((part, tbl, tuple(key), have, dict(values), post))
        return True

    def delete(self, part, post, db, tbl, key):
        have = full(db, tbl, key)
        if have is not None:
            self.ops.append((part, tbl, tuple(key), have, None, post))
            return True
        return False


# ============================================================================================== 196
def plan_196(db, p):
    part = "196"
    for t in ("petstats", "spellpattern", "spelleffect", "cmcatalog"):
        if not table(db, t):
            p.refuse(f"196: no {t} table")
            return
    cm = {}
    for role, name in CM_NAMES.items():
        r = rows(db, "SELECT * FROM cmcatalog WHERE name = ?", (name,))
        if len(r) != 1:
            p.refuse(f"196: cmcatalog {name!r}: {len(r)} rows (one wanted)")
            return
        cm[role] = r[0]

    def one_id(text, what):
        ids = [x for x in (text or "").split(",") if x.strip()]
        if len(ids) != 1 or not ids[0].strip().isdigit():
            raise Refused(f"196: {what} is {text!r} (one spell id wanted)")
        return int(ids[0])

    try:
        ids = {
            "swarming": one_id(cm["sacrament4"]["require_spells"], "Unholy Sacrament 4's require_spells"),
            "sacrament4": one_id(cm["sacrament4"]["add_spells"], "Unholy Sacrament 4's add_spells"),
            "crimson": one_id(cm["crimson"]["add_spells"], "Crimson Unholy Sacrament's add_spells"),
            "bone": one_id(cm["bonecoll"]["add_spells"], "Bone Collection's add_spells"),
            "ghoulish": one_id(cm["ghoulish"]["add_spells"], "Unholy Ghoulish Sacrament's add_spells"),
            "haunting": one_id(cm["haunting"]["add_spells"], "Wraith's Unholy Sacrament's add_spells"),
        }
    except Refused as e:
        p.refuse(str(e))
        return
    if one_id(cm["bonecoll"]["remove_spells"], "Bone Collection's remove_spells") != ids["sacrament4"]:
        p.refuse("196: Bone Collection does not remove Unholy Sacrament 4's pet")
        return
    sp = {}
    for role, i in ids.items():
        s = full(db, "spellpattern", (i,))
        ps = full(db, "petstats", (i,))
        if s is None or ps is None:
            p.refuse(f"196: {NECRO_ROWS[role]} ({i}) has no {'spellpattern' if s is None else 'petstats'} row")
            return
        sp[role] = s
        want = SPELL_NAMES.get(role)
        if want is not None and s["spellname"] != want:
            p.refuse(f"196: {NECRO_ROWS[role]} ({i}) is named {s['spellname']!r}, not {want!r}")
            return
    if sp["sacrament4"]["spellname"] not in (OLD_SPELL, NEW_SPELL):
        p.refuse(f"196: Unholy Sacrament 4's pet ({ids['sacrament4']}) is {sp['sacrament4']['spellname']!r}")
        return
    p.say("196: rows by name: " + ", ".join(f"{NECRO_ROWS[r]} {i}" for r, i in ids.items()))

    def change(tbl, key, new, olds, post):
        """new {col: value}; olds {col: value it may be before}; refuses a column neither."""
        have = full(db, tbl, key)
        bad = [c for c in new if have[c] != new[c] and have[c] != olds[c]]
        if bad:
            p.refuse(f"196: {tbl} {key[0]}: " + "; ".join(f"{c} is {have[c]!r}, neither {olds[c]!r} nor {new[c]!r}"
                                                         for c in bad))
            return
        ch = {c: v for c, v in new.items() if have[c] != v}
        if ch:
            p.ops.append((part, tbl, tuple(key), have, dict(plain(have), **ch), post))
            p.say(f"  {tbl} {key[0]}: " + ", ".join(f"{c} {have[c]!r} -> {v!r}" for c, v in ch.items()))

    s4 = ids["sacrament4"]
    # #196
    change("petstats", (ids["swarming"],), {"model": RED_CLOUD}, {"model": OLD_BODY}, 196)
    # 906301's petstats: model and name (#196) with its level (#197) in one row change
    change("petstats", (s4,), {"model": RED_CLOUD, "name": NEW_PET, "level": LEVEL},
           {"model": OLD_BODY, "name": OLD_PET, "level": OLD_LEVELS["sacrament4"]}, 196)
    change("spellpattern", (s4,), {"spellname": NEW_SPELL, "spelldesc": NEW_DESC},
           {"spellname": OLD_SPELL, "spelldesc": OLD_DESC}, 196)
    if full(db, "spelleffect", (s4,)) is None:
        p.refuse(f"196: spelleffect {s4} is missing")
    else:
        change("spelleffect", (s4,), {"line": NEW_LINE}, {"line": OLD_LINE}, 196)
    change("cmcatalog", (cm["sacrament4"]["id"],), {"adds": NEW_CM}, {"adds": OLD_CM}, 196)
    change("cmcatalog", (cm["bonecoll"]["id"],), {"removes": NEW_CM}, {"removes": OLD_CM}, 196)
    # #197 (906301's level went with its row above) and #198
    change("petstats", (ids["swarming"],), {"level": LEVEL}, {"level": OLD_LEVELS["swarming"]}, 197)
    change("petstats", (ids["crimson"],), {"level": LEVEL}, {"level": OLD_LEVELS["crimson"]}, 197)
    for role in ("bone", "ghoulish", "haunting"):
        change("petstats", (ids[role],), {"level": LEVEL}, {"level": OLD_LEVELS[role]}, 198)
    merge_ops(p, part)


def merge_ops(p, part):
    """two changes of one row in a part become one (the second built on the first's after)."""
    out, seen = [], {}
    for op in p.ops:
        if op[0] != part:
            out.append(op)
            continue
        k = (op[1], op[2])
        if k in seen:
            i = seen[k]
            prev = out[i]
            after = dict(prev[4])
            after.update({c: v for c, v in op[4].items() if op[3] is None or not same(op[3].get(c), v)})
            out[i] = (prev[0], prev[1], prev[2], prev[3], after, prev[5] if prev[5] == op[5] else f"{prev[5]}/{op[5]}")
        else:
            seen[k] = len(out)
            out.append(op)
    p.ops = out


# ============================================================================================== 199
class Source:
    """Heirloom's rows now and as they were before the entries' audits."""

    def __init__(self, src):
        self.src = src
        self.first = {}  # (tbl, key) -> the row before its first audit (None: the audit made it)
        for lo, hi in SOURCE_AUDITS:
            for r in rows(src, "SELECT id, action, tbl, rowkey, before FROM gm_audit WHERE id BETWEEN ? AND ? "
                               "AND username = ? ORDER BY id", (lo, hi, SOURCE_WHO)):
                if r["tbl"] not in SOURCE_TABLES:
                    continue
                k = json.loads(r["rowkey"])
                key = (r["tbl"], tuple(k[c] for c in KEYS[r["tbl"]]))
                if key not in self.first:
                    self.first[key] = json.loads(r["before"]) if r["before"] else None

    def now(self, tbl, key):
        return plain(full(self.src, tbl, key))

    def before(self, tbl, key):
        k = (tbl, tuple(key))
        return self.first[k] if k in self.first else self.now(tbl, key)

    def audited(self, tbl, key):
        return (tbl, tuple(key)) in self.first

    def steps(self, q, when):
        """{step: row} of quest q now or before."""
        keys = {(q, s) for (s,) in self.src.execute("SELECT step FROM queststeps WHERE questid = ?", (q,))}
        keys |= {k for (t, k) in self.first if t == "queststeps" and k[0] == q}
        out = {}
        for k in keys:
            r = self.now("queststeps", k) if when == "now" else self.before("queststeps", k)
            if r is not None:
                out[k[1]] = r
        return out

    def targets(self, q, when):
        keys = {(q, s, t) for (s, t) in self.src.execute("SELECT step, target FROM queststeptargets WHERE questid = ?",
                                                         (q,))}
        keys |= {k for (t, k) in self.first if t == "queststeptargets" and k[0] == q}
        out = set()
        for k in keys:
            r = self.now("queststeptargets", k) if when == "now" else self.before("queststeptargets", k)
            if r is not None:
                out.add(k)
        return out


ITEM_TOKEN = re.compile(r"(?<![*\d])(\d+)")
JSON_ITEMS = re.compile(r'("items"\s*:\s*")([^"]*)(")')


def map_items_text(text, m):
    return ITEM_TOKEN.sub(lambda g: str(m.get(int(g.group(1)), int(g.group(1)))), text)


def map_row(r, m):
    """a queststeps row with Heirloom's item ids made Live's (m: Heirloom id -> Live id)."""
    if r is None or not m:
        return r
    r = dict(r)
    for c in ITEM_FIELDS:
        if isinstance(r.get(c), str) and r[c]:
            r[c] = map_items_text(r[c], m)
    for c in JSON_ITEM_FIELDS:
        if isinstance(r.get(c), str) and r[c]:
            r[c] = JSON_ITEMS.sub(lambda g: g.group(1) + map_items_text(g.group(2), m) + g.group(3), r[c])
    return r


def step_items(r):
    """the item ids a step names (items, rewarditems, its events' and choices' items)."""
    out = set()
    for c in ITEM_FIELDS:
        out |= {int(x) for x in ITEM_TOKEN.findall(r.get(c) or "")}
    for c in JSON_ITEM_FIELDS:
        for g in JSON_ITEMS.finditer(r.get(c) or ""):
            out |= {int(x) for x in ITEM_TOKEN.findall(g.group(2))}
    return out


def step_spells(r):
    out = {int(x) for x in re.findall(r"\d+", r.get("spells") or "")}
    for g in re.finditer(r'"spells"\s*:\s*"([^"]*)"', r.get("choices") or ""):
        out |= {int(x) for x in re.findall(r"\d+", g.group(1))}
    return out


def step_npcs(r):
    out = set()
    if r.get("target") and r.get("kind") != "reach":
        out.add(r["target"])
    for g in re.finditer(r'"npc"\s*:\s*"([^"]*)"', r.get("events") or ""):
        out.add(g.group(1))
    return out


def plan_199(db, srcdb, p):
    part, post = "199", 199
    for side, d in (("Live", db), ("Heirloom", srcdb)):
        for t in ("itempattern", "quests", "queststeps", "queststeptargets", "npcitem", "npcs", "charquests",
                  "charinventory", "spellpattern", "gm_audit"):
            if not table(d, t):
                p.refuse(f"199: {side} has no {t} table")
    if p.refusals:
        return
    for t in ("itempattern", "quests", "queststeps", "queststeptargets"):
        if columns(db, t) != columns(srcdb, t):
            p.refuse(f"199: {t}: Live's columns differ from Heirloom's")
    if p.refusals:
        return
    S = Source(srcdb)

    # entry 894 first: 100081-100085 as Heirloom's but for the columns copied here
    for q in sorted(COLUMNS):
        h, l = S.now("quests", (q,)), plain(full(db, "quests", (q,)))
        if l is None or h is None:
            p.refuse(f"199: quest {q} missing on {'Live' if l is None else 'Heirloom'} (entry 894 first)")
            continue
        diff = [c for c in h if c not in COLUMNS[q] and h[c] != l[c]]
        hs, ls = S.steps(q, "now"), {r["step"]: plain(r) for r in rows(db, "SELECT * FROM queststeps WHERE questid = ?", (q,))}
        if diff or hs.keys() != ls.keys() or any(not loose_row(hs[s], ls[s]) for s in hs) \
                or S.targets(q, "now") != set(db.execute("SELECT questid, step, target FROM queststeptargets "
                                                         "WHERE questid = ?", (q,)).fetchall()):
            p.refuse(f"199: quest {q} on Live is not Heirloom's (entry 894 is not done there: ask jibrish)"
                     + (f"; quests columns {diff}" if diff else ""))
    if p.refusals:
        return
    p.say("199: entry 894 (100081-100085) is on Live as on Heirloom")

    # item ids: Heirloom's -> Live's
    hsteps = {q: S.steps(q, "now") for q in WHOLE}
    osteps = {q: S.steps(q, "before") for q in WHOLE}
    named = set()
    for q in WHOLE:
        for r in list(hsteps[q].values()) + list(osteps[q].values()):
            named |= step_items(r)
    m = {}
    for i in sorted(named):
        h = S.now("itempattern", (i,))
        hb = S.before("itempattern", (i,))
        if i == NEW_ITEM:
            continue
        if h is None:
            p.refuse(f"199: item {i}, named by the quests, is not on Heirloom")
            continue
        l = full(db, "itempattern", (i,))
        if l is not None and l["itemname"] in {h["itemname"], (hb or {}).get("itemname")}:
            continue
        c = db.execute("SELECT patternid FROM itempattern WHERE itemname = ?", (h["itemname"],)).fetchall()
        if len(c) == 1:
            m[i] = c[0][0]
            p.say(f"  item {i} {h['itemname']!r} is Live's {c[0][0]} (Live's {i} is "
                  f"{None if l is None else l['itemname']!r})")
        else:
            p.refuse(f"199: item {i} {h['itemname']!r}: Live's {i} is {None if l is None else l['itemname']!r} "
                     f"and Live has {len(c)} records of the name")
    for q in WHOLE:
        for r in hsteps[q].values():
            for s in step_spells(r):
                h, l = S.now("spellpattern", (s,)), full(db, "spellpattern", (s,))
                if h is None or l is None or h["spellname"] != l["spellname"]:
                    p.refuse(f"199: spell {s} ({q}/{r['step']}) is not Heirloom's on Live")
            for n in step_npcs(r):
                if not db.execute("SELECT 1 FROM npcs WHERE lower(npc_name) = lower(?) LIMIT 1", (n,)).fetchone():
                    p.refuse(f"199: no NPC named {n!r} on Live ({q}/{r['step']})")
        g = S.now("quests", (q,))["giver"]
        if g and not db.execute("SELECT 1 FROM npcs WHERE lower(npc_name) = lower(?) LIMIT 1", (g,)).fetchone():
            p.refuse(f"199: no NPC named {g!r} on Live (quest {q}'s giver)")
    if p.refusals:
        return

    # 1. items
    h = S.now("itempattern", (NEW_ITEM,))
    l = plain(full(db, "itempattern", (NEW_ITEM,)))
    if h is None:
        p.refuse(f"199: Heirloom has no {NEW_ITEM}")
    elif l is None:
        p.set(part, post, db, "itempattern", (NEW_ITEM,), h)
        p.say(f"  itempattern {NEW_ITEM} {h['itemname']!r}: made")
    elif not same_row(l, h):
        p.refuse(f"199: itempattern {NEW_ITEM} is taken on Live by {l['itemname']!r} (not Heirloom's row): ask jibrish")
    for i, cols in RENAMES.items():
        h, hb, l = S.now("itempattern", (i,)), S.before("itempattern", (i,)), full(db, "itempattern", (i,))
        if h is None or l is None:
            p.refuse(f"199: itempattern {i} missing on {'Heirloom' if h is None else 'Live'}")
            continue
        if all(same(l[c], h[c]) for c in cols):
            continue
        if not loose_row(l, hb, cols):
            p.refuse(f"199: itempattern {i}: Live's {', '.join(f'{c} {l[c]!r}' for c in cols)} is not what Heirloom "
                     f"had before: ask jibrish")
            continue
        p.set(part, post, db, "itempattern", (i,), dict(plain(l), **{c: h[c] for c in cols}))
        p.say(f"  itempattern {i}: {l['itemname']!r} -> {h['itemname']!r}" + (" (and its description)" if "itemdesc" in cols else ""))
    for i in MUST_EXIST:
        h, l = S.now("itempattern", (i,)), full(db, "itempattern", (i,))
        if l is None or h is None or l["itemname"] != h["itemname"]:
            p.refuse(f"199: itempattern {i} {None if h is None else h['itemname']!r} is not on Live as on Heirloom")

    # 2. wares (checked only)
    for npc, pid, q in WARES:
        ids = [r[0] for r in db.execute("SELECT id FROM npcs WHERE npc_name = ?", (npc,))]
        if not ids:
            p.refuse(f"199: no {npc} on Live ({q})")
        elif not db.execute(f"SELECT 1 FROM npcitem WHERE patternid = ? AND npcid IN ({','.join('?' * len(ids))})",
                            [pid] + ids).fetchone():
            p.refuse(f"199: {npc} does not sell {pid} on Live ({q}): ask jibrish")
    p.say("  section 2: every merchant sells its ware" if not p.refusals else "")

    # 3. quests
    nq = ns = nd = nt = 0
    p.copied = set()
    for q in WHOLE:
        h, hb, l = S.now("quests", (q,)), S.before("quests", (q,)), plain(full(db, "quests", (q,)))
        if l is None:
            p.refuse(f"199: quest {q} is not on Live")
            continue
        if not same_row(l, h):
            if not loose_row(l, hb):
                p.refuse(f"199: quest {q}: Live's row is neither Heirloom's before nor now (columns "
                         f"{[c for c in l if l[c] != hb.get(c)]}): ask jibrish")
                continue
            p.set(part, post, db, "quests", (q,), h)
            nq += 1
        new = {s: map_row(r, m) for s, r in hsteps[q].items()}
        old = {s: map_row(r, m) for s, r in osteps[q].items()}
        have = {r["step"]: plain(r) for r in rows(db, "SELECT * FROM queststeps WHERE questid = ?", (q,))}
        bad = []
        for s in sorted(set(new) | set(old) | set(have)):
            n, o, x = new.get(s), old.get(s), have.get(s)
            if same_row(x, n) if x is not None and n is not None else (x is None and n is None):
                continue
            if not loose_row(x, o):
                bad.append(s)
        if bad:
            p.refuse(f"199: quest {q} steps {bad}: Live's are neither Heirloom's before nor now: ask jibrish")
            continue
        for s in sorted(set(new) | set(have)):
            if s in new:
                if p.set(part, post, db, "queststeps", (q, s), new[s]):
                    ns += 1
                    p.copied.add(q)
            elif p.delete(part, post, db, "queststeps", (q, s)):
                nd += 1
                p.copied.add(q)
        ht, hbt = S.targets(q, "now"), S.targets(q, "before")
        lt = set(db.execute("SELECT questid, step, target FROM queststeptargets WHERE questid = ?", (q,)).fetchall())
        if lt != ht and lt != hbt:
            p.refuse(f"199: quest {q}'s step targets on Live {sorted(lt)} are neither Heirloom's before nor now")
            continue
        for k in sorted(lt - ht):
            p.delete(part, post, db, "queststeptargets", k)
            nt += 1
        for k in sorted(ht - lt):
            p.set(part, post, db, "queststeptargets", k, {"questid": k[0], "step": k[1], "target": k[2]})
            nt += 1
    for q, cols in COLUMNS.items():
        h, hb, l = S.now("quests", (q,)), S.before("quests", (q,)), full(db, "quests", (q,))
        if all(same(l[c], h[c]) for c in cols):
            continue
        if not loose_row(l, hb, cols):
            p.refuse(f"199: quest {q}: Live's {cols} not Heirloom's before: ask jibrish")
            continue
        p.set(part, post, db, "quests", (q,), dict(plain(l), **{c: h[c] for c in cols}))
        nq += 1
    p.say(f"199: quests: {nq} quests rows, {ns} steps written, {nd} steps removed, {nt} step targets")
    if p.refusals:
        return
    plan_chars_199(db, p, part, post)


def after_steps(db, p, q):
    steps = {s["step"]: plain(s) for s in rows(db, "SELECT * FROM queststeps WHERE questid = ?", (q,))}
    for op in p.ops:
        if op[1] == "queststeps" and op[2][0] == q:
            if op[4] is None:
                steps.pop(op[2][1], None)
            else:
                steps[op[2][1]] = op[4]
    return steps


def holds(db, sid, pid):
    return db.execute("SELECT count(*) FROM charinventory WHERE serverid = ? AND patternid = ?", (sid, pid)).fetchone()[0]


def plan_chars_199(db, p, part, post):
    copied = sorted(p.copied)
    if not copied:
        return
    marks = ",".join("?" * len(copied))
    n = 0
    for c in rows(db, f"SELECT rowid AS _rowid_, * FROM charquests WHERE questid IN ({marks}) AND step > 0 AND done = 0 "
                      "ORDER BY questid, serverid", copied):
        q, sid, step = c["questid"], c["serverid"], c["step"]
        old = {s["step"]: plain(s) for s in rows(db, "SELECT * FROM queststeps WHERE questid = ?", (q,))}
        new = after_steps(db, p, q)
        to = STEP_MOVES.get(q, {}).get(step, step)
        if to not in new:
            p.refuse(f"199: character {sid} on {q} step {step}: no step {to} after the copy: ask jibrish")
            continue
        o, nw = old.get(step), new[to]
        keep = o is not None and o["kind"] == nw["kind"] and o["items"] == nw["items"] and \
            (o["kind"] == "loot" or o["target"] == nw["target"])
        count = c["count"] if keep else 0
        if to != step or count != c["count"]:
            p.ops.append((part, "charquests", (sid, q), c, dict(plain(c), step=to, count=count), post))
            p.say(f"  character {sid}: {q} step {step} (count {c['count']}) -> step {to} (count {count})")
            n += 1
        give = GIVES.get((q, step))
        if give is not None:
            if holds(db, sid, give):
                p.say(f"  character {sid}: {q} step {to}, holds {give} already")
            elif db.execute("SELECT count(*) FROM charinventory WHERE serverid = ? AND location = 0",
                            (sid,)).fetchone()[0] >= INV_CAP:
                p.say(f"  character {sid}: {q} step {to}: pack full, NOT given {give} (a GM gives it)")
            else:
                p.ops.append((part, "give", (sid, give), None, None, post))
                p.say(f"  character {sid}: {q} step {to}: given {give}")
    p.say(f"199: characters part-way: {n} moved")


# ============================================================================================== 200
def plan_200(db, shard, p):
    part, post = "200", 200
    keep, off = SPAWNS[shard]["keep"], SPAWNS[shard]["off"]
    marks = ",".join("?" * len(LOOT_NAMES))
    binds = rows(db, f"SELECT rowid AS _rowid_, * FROM npcloot WHERE source = ? AND npc_name IN ({marks}) "
                     "ORDER BY npc_name, tablename", (LOOT_SRC,) + LOOT_NAMES)
    bad = False
    for n in LOOT_NAMES:
        have = {r[0] for r in db.execute("SELECT id FROM npcs WHERE lower(npc_name) = ?", (n,))}
        missing = [i for i in keep[n] if i not in have]
        stray = sorted(have - set(keep[n]) - set(off[n]))
        if missing or stray:
            p.refuse(f"200: {n!r}: " + "; ".join(x for x in (
                f"listed spawns not standing under the name {missing}" if missing else "",
                f"spawns on neither list {stray}" if stray else "") if x) + ": ask jibrish")
            bad = True
    if bad:
        return
    if not binds:
        # done already? every row key in place
        made = db.execute(f"SELECT count(*) FROM npcloot WHERE source = ? AND npc_name LIKE '#%'", (LOOT_SRC,)).fetchone()[0]
        p.say(f"200: no name bindings of the eight names left ({made} row-key bindings of {LOOT_SRC} on the shard)")
        if table(db, LOG) and db.execute(f"SELECT count(*) FROM {LOG} WHERE part = '200'").fetchone()[0]:
            return
        p.refuse("200: none of the 19 name bindings is there and this tool did not remove them: ask")
        return
    if len(binds) != LOOT_BINDINGS:
        p.refuse(f"200: {len(binds)} name bindings of the eight names (19 wanted, as jibrish listed): ask")
        return
    ins = 0
    for b in binds:
        for i in keep[b["npc_name"]]:
            key = (f"#{i}", b["tablename"])
            v = {"npc_name": key[0], "tablename": b["tablename"], "chance": b["chance"], "rolls": b["rolls"],
                 "note": b["note"], "source": b["source"]}
            have = full(db, "npcloot", key)
            if have is not None:
                if all(same(have[c], v[c]) for c in v):
                    continue
                p.refuse(f"200: npcloot {key} is there already as something else: {plain(have)}")
                continue
            p.set(part, post, db, "npcloot", key, v)
            ins += 1
    for b in binds:
        p.delete(part, post, db, "npcloot", (b["npc_name"], b["tablename"]))
    per = {n: (len(keep[n]), sum(1 for b in binds if b["npc_name"] == n)) for n in LOOT_NAMES}
    p.say(f"200 ({shard}): {ins} row-key bindings made, {len(binds)} name bindings removed")
    for n, (k, t) in per.items():
        p.say(f"  {n}: {t} table(s) x {k} spawns kept; {len([i for i in off[n]])} spawns off the list")


# ============================================================================================== apply / undo
def audit(db, action, tbl, key, before, after, post, entry):
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (now(), ACCOUNT, WHO, VIA, action, tbl, dumps(dict(zip(KEYS[tbl], key))),
                      None if before is None else dumps(before), None if after is None else dumps(after),
                      detail(post, entry)))
    return cur.lastrowid


ENTRY = {"196": 1865, "199": 1869, "200": 1872}


def write_row(db, tbl, values, rowid=None):
    cols = list(values)
    vals = [values[c] for c in cols]
    if rowid is not None:
        cols, vals = ["rowid"] + cols, [rowid] + vals
    return db.execute(f"INSERT INTO {tbl} ({', '.join(c if c == 'rowid' else chr(34) + c + chr(34) for c in cols)}) "
                      f"VALUES ({', '.join('?' * len(cols))})", vals).lastrowid


def log(db, part, tbl, key, before, after, a):
    db.execute(f"INSERT INTO {LOG} (part, tbl, rowkey, before, after, audit, at) VALUES (?, ?, ?, ?, ?, ?, ?)",
               (part, tbl, dumps(dict(zip(KEYS[tbl], key))), None if before is None else dumps(before),
                None if after is None else dumps(after), a, now()))


def apply(db, p):
    db.execute(LOG_DDL)
    n = 0
    for part, tbl, key, before, after, post in p.ops:
        entry = ENTRY[part]
        if tbl == "give":
            sid, pid = key
            cur = db.execute(
                "INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, "
                "listnumber) VALUES (?, 1, (SELECT COALESCE(maxhp, 0) FROM itempattern WHERE patternid = ?), 0, ?, -1, 0, "
                "(SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory WHERE serverid = ? AND location = 0))",
                (sid, pid, pid, sid))
            k = (cur.lastrowid,)
            row = full(db, "charinventory", k)
            log(db, part, "charinventory", k, None, row,
                audit(db, "insert", "charinventory", k, None, plain(row), post, entry))
            n += 1
            continue
        now_ = full(db, tbl, key)
        if before is None:
            if now_ is not None:
                raise RuntimeError(f"{tbl} {key}: there under us")
            write_row(db, tbl, after)
        else:
            if now_ != before:
                raise RuntimeError(f"{tbl} {key}: changed under us")
            if after is None:
                db.execute(f"DELETE FROM {tbl} WHERE {where(tbl)}", key)
            else:
                sets = [c for c in after if c != "_rowid_" and not same(before.get(c), after[c])]
                db.execute(f"UPDATE {tbl} SET {', '.join(chr(34) + c + chr(34) + ' = ?' for c in sets)} WHERE {where(tbl)}",
                           [after[c] for c in sets] + list(key))
        row = full(db, tbl, key) if after is not None else None
        action = "insert" if before is None else ("delete" if after is None else "update")
        log(db, part, tbl, key, before, row, audit(db, action, tbl, key, plain(before), plain(row), post, entry))
        n += 1
    return n


def undo(db, parts):
    """(rows put back, [left])."""
    if not table(db, LOG):
        return 0, []
    back, left = 0, []
    marks = ",".join("?" * len(parts))
    for seq, part, tbl, rowkey, before, after in db.execute(
            f"SELECT seq, part, tbl, rowkey, before, after FROM {LOG} WHERE part IN ({marks}) ORDER BY seq DESC",
            tuple(parts)).fetchall():
        k = json.loads(rowkey)
        key = tuple(k[c] for c in KEYS[tbl])
        have = full(db, tbl, key)
        if (have is None) != (after is None) or (have is not None and dumps(have) != after):
            left.append(f"{tbl} {rowkey}: changed since, left as it is")
            continue
        b = None if before is None else json.loads(before)
        if have is not None:
            db.execute(f"DELETE FROM {tbl} WHERE {where(tbl)}", key)
        if b is not None:
            rid = b.pop("_rowid_", None)
            write_row(db, tbl, b, rid)
        action = "delete" if b is None else ("insert" if have is None else "update")
        audit(db, action, tbl, key, plain(have), b, f"{part}, undone", ENTRY[part])
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if not db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0]:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--shard", required=True, choices=("heirloom", "live"))
    ap.add_argument("--part", help="comma list of 196, 199, 200 (default: every part the shard takes)")
    ap.add_argument("--source", help="Heirloom's database, read only (part 199)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    parts = SHARD_PARTS[a.shard] if not a.part else tuple(x.strip() for x in a.part.split(",") if x.strip())
    for x in parts:
        if x not in PARTS:
            ap.error(f"no part {x}")
        if x not in SHARD_PARTS[a.shard]:
            ap.error(f"part {x} is not for {a.shard}" + (" (Heirloom is done: entry 1864)" if x == "196" else
                                                         " (Heirloom is the source)" if x == "199" else ""))
    if "199" in parts and not a.undo and not a.source:
        ap.error("part 199 needs --source (Heirloom's database)")
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("PRAGMA busy_timeout = 5000")
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = undo(db, parts)
            db.execute("COMMIT")
            print(f"undone ({','.join(parts)}): {back} rows put back" + "".join(f"\n  left: {w}" for w in left))
            return 0 if not left else 1
        p = Plan()
        if "196" in parts:
            plan_196(db, p)
        if "199" in parts:
            src = sqlite3.connect(f"file:{a.source}?mode=ro", uri=True)
            try:
                plan_199(db, src, p)
            finally:
                src.close()
        if "200" in parts:
            plan_200(db, a.shard, p)
        for line in p.lines:
            if line:
                print(line)
        if p.refusals:
            db.execute("ROLLBACK")
            print("REFUSED, nothing written:\n  " + "\n  ".join(p.refusals))
            return 1
        counts = {}
        for op in p.ops:
            act = "give" if op[1] == "give" else "insert" if op[3] is None else "delete" if op[4] is None else "update"
            counts[(op[0], op[1], act)] = counts.get((op[0], op[1], act), 0) + 1
        print("totals: " + (", ".join(f"{k[0]} {k[2]} {k[1]} {v}" for k, v in sorted(counts.items())) or "nothing"))
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
