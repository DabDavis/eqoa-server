#!/usr/bin/env python3
"""teamjosh.com's Hunt and Gather quests against ours (the owner, 2026-11-15, sharing the archived 2007 page
web.archive.org/web/20070711171347/http://teamjosh.com/eqoa/HandG/ as the authority for these quests): its 77
repeatable turn-ins, each page parsed into tools/handg/teamjosh_handg_20261115.json, compared with a shard's quests
and put right. Each shard's own rows: run it on each database.

For every source quest the report says PRESENT (levels, experience, tunar, drop rate and creatures as the source
has them), DIFFERENT (each difference; the ones written here are marked "fix") or MISSING (whether the giver, the
item and the creature are in this database). What it writes:

  - levels, experience and tunar to the source's on the quests we have (the source's figures; where it gives none,
    ours stay), and its drop rate where it gives one (Barnaby Priceton's "about 40%", Corison's spiders' "1 out of
    15");
  - the creatures the source names that a quest's loot step lacks, where the creature is in the database
    (queststeptargets: the item drops off them too, for a player on the step);
  - Udrik Lazow's pickclaw tiers: each tier's levels the source's (the first 1-30), every weapon worth its tier's
    tunar (the totems and orbs had none), and a sash worth half a weapon's experience and its tier's sash tunar,
    so the four-sash quests pay four sashes' worth;
  - the quests we lack whose giver and creature we have: Chef Lukkif's Flavored Mushrooms (Guk's fungal folk) and
    Basilisk Claws (the fire basilisks by him), Terison Eydillo's Preserved Ghast Eyes (Collonridge's lesser ghouls:
    no ghast stands there), and, on a shard without them, the eight Heirloom has from this source (board entry
    1081: Charred Bones, Unkempt Bracelets, Tiny Daggers, Drachnid Leg Tips, Chipped Badger Claws, Preserved Zombie
    Eyes, Cinder Cat Wings and Fangs), copied as Heirloom has them with their items. Their items are made where no
    item has the name (no stats, NO TRADE where the source says so or the giver's other items are); the quest item
    drops only for a player on the loot step, at the source's rate or our turn-ins' 0.5.

Not written (said in the report): a quest whose giver is not in the game (no new NPCs), one whose page names no
item, and the differences that are not figures (faction rewards, reward items, stack sizes, a giver's spelling).

Every row it changes is found by comparing the tables before and after (tools/seed_guide_gaps.py's machinery, set
"handg-20261115" in guide_set_log), so --undo is exact; a gm_audit line a row, and one for an undo. Report only by
default; --apply in one transaction; a second --apply does nothing; --undo, again nothing. Quests are read at the
server's start: what it writes takes effect at the next restart.

    tools/handg_quests_20261115.py --db eqoa.db [--apply | --undo] [--json out.json]
"""
import argparse
import datetime
import json
import os
import re
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import seed_guide_gaps as sgg  # noqa: E402

SET = "handg-20261115"
WHO, VIA = "dab", "tool"
DETAIL = "teamjosh.com's Hunt and Gather quests (the owner's source, 2026-11-15): tools/handg_quests_20261115.py"
QSOURCE = "teamjosh H&G (tools/handg_quests_20261115.py)"
DATA = os.path.join(HERE, "handg", "teamjosh_handg_20261115.json")
# what it touches, compared whole (small tables: the lock is held a moment)
sgg.TABLES = ("itempattern", "quests", "queststeps", "queststeptargets", "sqlite_sequence")


class Q:
    """One of our quests standing for a source quest: found by its giver and its hand-in's item (and its loot step's
    creature, where one giver has several of the item), with what it should be when the source's own figures are
    not it (a pickclaw sash's worth)."""

    def __init__(self, giver, item, target=None, xp=None, tunar=None, chance=None, also=(), levels=None):
        self.giver, self.item, self.target = giver, item, target
        self.xp, self.tunar, self.chance, self.also, self.levels = xp, tunar, chance, list(also), levels


# ------------------------------------------------------------------------------------------------ the source's quests
# Udrik Lazow's tiers (keys 85-90): "Weapons are worth X XP and T Tunar and Sash is worth X/2 XP and T-100 Tunar".
# Our sash quests take four sashes: four sashes' worth.
def tier(lo, hi, melee, caster, sash, sword, totem, wxp, wt, also):
    sxp, st = wxp // 2, wt - 100
    return [Q("Udrik Lazow", sash, xp=4 * sxp, tunar=4 * st, also=also, levels=(lo, hi)),
            Q("Udrik Lazow", sword, xp=wxp, tunar=wt, levels=(lo, hi)),
            Q("Udrik Lazow", totem, xp=wxp, tunar=wt, levels=(lo, hi))]


PRESENT = {
    68: [Q("Hajyd Golemon", "Zombie Patron Skin")],
    31: [Q("Jeweler Yilt", "charred bones")],
    37: [Q("Giant Slayer Derkev", "ice giant shield")],
    29: [Q("Archaeologist Duggan", "wooden cylinder gasket")],
    38: [Q("Corison", "ice glyph")],
    90: tier(51, 60, "adept pickclaw soldier", "adept pickclaw elder", "pristine pickclaw sash", "pickclaw soldier axe",
             "pickclaw elder orb", 700000, 700, ["adept pickclaw elder"]),
    78: [Q("Chef Lukkif", "Bandit Herbs")],
    74: [Q("Chef Lukkif", "tasty froglok skins", also=["froglok inquisitor"])],
    70: [Q("Chef Lukkif", "savory treant roots")],
    71: [Q("Chef Lukkif", "Ghoulish Preservative")],
    43: [Q("Giant Slayer Derkev", "Cold Iron Nosering")],
    72: [Q("Chef Lukkif", "Purified Water Canteen")],
    73: [Q("Chef Lukkif", "Froglok Stomach")],
    69: [Q("Chef Lukkif", "crusty goblin eye")],
    75: [Q("Chef Lukkif", "Coarse Salt")],
    77: [Q("Chef Lukkif", "Fiery Hot Pepper")],
    79: [Q("Chef Lukkif", "evil eye cornea")],
    27: [Q("Gypsy Taylor", "Small Chipped Ruby")],
    54: [Q("Corison", "Giant Blue Pearl Ring", also=["ice giant refugee"])],  # "you can also hunt refugee's ... they drop rings also"
    26: [Q("Counciler Dreda", "unkempt bracelet", also=["unkempt druid", "unkempt ranger"])],
    84: [Q("Captain Riloh", "Regal Siloth Band")],
    80: [Q("Chef Lukkif", "frost giant toes")],
    63: [Q("Vargilder Tonja", "Broken Totem")],
    20: [Q("Jenya Gooten", "tiny dagger", chance=0.14)],  # "the daggers only drop 1 out of every 6-8 mobs"
    21: [Q("Farmer Steveson", "Nixie Cap")],
    22: [Q("Larn Yolger", "Nixie Cap")],
    24: [Q("Commander Pearson", "Witchking Collar", chance=0.5)],  # "Collars drop on about half the cultists"
    89: tier(41, 50, "pickclaw soldier", "pickclaw elder", "pickclaw sash", "pickclaw soldier sword", "pickclaw elder totem",
             400000, 600, ["pickclaw elder"]),
    53: [Q("Corison", "Flawed Black Diamond")],
    56: [Q("Giant Slayer Derkev", "Giant Blue Pearl Ring", also=["ice giant"])],  # "Ice Giant and/or Refugees"
    47: [Q("Yarn Molsen", "Dark Skull Ring")],
    55: [Q("Tinker Z.", "drachnid leg tip")],
    17: [Q("Barnaby Priceton", "Serpent Fang Ring", chance=0.4, also=["blackscourge fryar"])],  # "about 40% kill/drop"
    52: [Q("Corison", "Ice Worm Chitlin")],
    40: [Q("Gypsy Leigh", "Mutant Ant Carapace", target="red ant leafcutter")],
    88: tier(31, 40, "adept pickclaw warrior", "adept pickclaw mystic", "soiled pickclaw sash", "pickclaw warrior's axe",
             "pickclaw mystic's orb", 240000, 500, ["adept pickclaw mystic"]),
    18: [Q("Seargent Strugol", "Siloth Insignia Ring", also=["siloth slaver", "siloth guard", "siloth ritualist"])],
    13: [Q("Gertrude Ross", "Petrified Rose")],
    36: [Q("Recruiter Deryn", "Platinum Gnoll Pendant")],
    39: [Q("Gypsy Leigh", "Mutant Ant Carapace", target="red ant worker")],
    14: [Q("Gypsy Leigh", "Mutant Ant Carapace", target="red ant scout")],
    42: [Q("Corison", "Dark Ore", chance=0.067)],  # "Drops from about 1 out of 15 spiders."
    35: [Q("Recruiter Deryn", "Blood Paw Collar")],
    9: [Q("Katrina Finney", "Chipped Badger Claw")],
    87: tier(21, 30, "pickclaw warrior", "pickclaw mystic", "torn pickclaw sash", "pickclaw warrior sword", "pickclaw mystic totem",
             120000, 400, ["pickclaw mystic"]),
    83: [Q("Terison Eydillo", "Preserved Zombie Eye")],
    51: [Q("Elzilor Marbe", "Rune of Crethley Manor", also=["decaying zombie", "hungry zombie"])],  # "zombies and/or skeletons"
    82: [Q("Terison Eydillo", "Preserved Ghast Eye")],
    49: [Q("Corison", "Broken Leopard Claw")],
    34: [Q("Recruiter Deryn", "Black Gnoll Ring")],
    7: [Q("Ingrid Adder", "Matted Bear Hide")],
    48: [Q("Corison", "Heavy Venom Sac", also=["arctic spider"])],  # "Arctic or Freezefang Spiders"
    33: [Q("Recruiter Deryn", "Gnoll Ear Cuff")],
    86: tier(11, 20, "adept pickclaw brawler", "adept pickclaw shaman", "ripped pickclaw sash", "pickclaw brawler's axe",
             "pickclaw shaman orb", 50000, 300, ["adept pickclaw shaman"]),
    1: [Q("Zaraznia", "Sunbeetle Carapace")],
    6: [Q("Neshki Widdlewick", "Ant Chitin")],
    5: [Q("Tresca Goodmorrow", "Pike Meat")],
    4: [Q("Tugren Fyrebrand", "pristine raven feathers")],
    3: [Q("Sirnal Baubbletink", "torn snake scales")],
    85: tier(1, 30, "pickclaw brawler", "pickclaw shaman", "destroyed pickclaw sash", "pickclaw brawler's sword",
             "pickclaw shawman's totem", 20000, 200, ["pickclaw shaman"]),
    45: [Q("Viltharus Battleprayer", "Fangs", chance=0.8)],  # "Drop rate high. Approx 8 out of 10."
    44: [Q("Viltharus Battleprayer", "Wings", chance=0.8)],
    81: [Q("Chef Lukkif", "Flavored Mushroom")],
    76: [Q("Chef Lukkif", "Basilisk Claw")],
}

# what the source says that is not a figure, where ours differs (said, not written)
NOTES = {
    68: "the source's giver is Hajyd Golemcrafter; ours is the game's Hajyd Golemon",
    37: "the source's reward has Anu Village +8 faction; ours none",
    38: "the source's reward has Zentar's Keep +5; ours none",
    43: "the source's creatures are ice giant soldiers and mages; ours the ice giant elite (no soldier or mage here)",
    73: "the source's creatures are frogloks; ours the froglok dar warriors",
    75: "the source's creatures are Geo Citadel's gargoyles and elements; ours the mithril gargoyles",
    54: "the source's reward has Zentar's Keep +5; ours none",
    63: "the source's reward has a level 42 drink and Anu faction; ours none",
    21: "the source's reward has a Murnf cabbage (level 40 food) and Murnf/Treant/Unkempt factions; ours none",
    22: "the source's giver is Yarn Lolger; ours is the game's Larn Yolger",
    24: "the source: the collar stacks to 20 (ours 99)",
    49: "the source's creatures are frost or tundra leopards (no tundra leopard here); its reward has Zentar's Keep +5",
    42: "the source's creatures are Spider Mine's spiders; ours the cave crawlers; its reward has Zentar's Keep +5",
    48: "the source's reward has Zentar's Keep +5; ours none",
    51: "the source: twenty to a stack (ours 99); the count of ten is ours",
    1: "the source's reward is food or drink; ours none",
    6: "the source's reward is food or drink; ours none",
    5: "the source's reward has Freeport faction; ours none",
    4: "the source's giver is Tungren Fyrebrunt; ours is the game's Tugren Fyrebrand; reward food or drink, ours none",
    3: "the source's giver is Sirnal Baubletink; ours is the game's Sirnal Baubbletink; reward food or drink, ours none",
    13: "the source's creature is the Desert Gypsy; ours the gypsies",
}
# what is worth saying and is no difference
ASIDES = {
    **{k: "the source has one quest for the tier's sash and weapons; ours a sash quest and a hand-in a weapon (a sash four "
           "to a hand-in, worth four)" for k in (85, 86, 87, 88, 89, 90)},
    20: "the source's reward has Citizens of Bogman +1 (ours too); its 1 in 6-8 is our 0.14",
    29: "the source: a frequent NO TRADE drop, four to a stack (ours: NO TRADE, four)",
    83: "the source: loot NO TRADE, twenty to a stack (ours the same)",
    45: "the index says 975,000 experience; the quest's page 928,908 (ours, the page's)",
    44: "the index says 1,000,000 experience; the quest's page 1,012,662 (ours, the page's)",
}

# ---------------------------------------------------------------------------------------------- the ones we lack
# made where no quest stands for them (and the giver and the creatures are here)
NEW = {
    81: dict(name="Flavored Mushrooms", giver="Chef Lukkif", levels=(50, 60), xp=2000000,
             item=("Flavored Mushroom", "Digested Mushrooms", "A flavorful mushroom off one of Guk's fungal folk. Chef Lukkif cooks with them.",
                   {"trade": 1, "maxstack": 99}),
             target="fungal cavalier",
             also=["fungal adept", "fungal diviner", "fungal elder", "fungal warrior", "fungal hunter", "fungal devout",
                   "fungal priest", "fungal sporecaster", "fungal gatherer", "elder fungal cavalier"],
             offer="The fungal folk of Guk grow the most flavorful mushrooms. Bring me four.",
             loot="Kill the fungal folk of Guk for four flavored mushrooms.",
             give="Bring four flavored mushrooms to Chef Lukkif.", says="Such flavor!",
             notes="teamjosh H&G key 81 (the owner's source, 2026-11-15): 50-60, Chef Lukkif, flavored mushrooms off the "
                   "Mushroom Dudes of Guk. The page gives no experience: 2,000,000 is ours, as Chef Lukkif's other rows "
                   "without one. Its Mushroom Dudes are Guk's fungal folk (the fungal cavalier and its kin, 37-45, the "
                   "elder fungal cavalier 62); count 4 and chance 0.5 ours, as his other quests."),
    76: dict(name="Basilisk Claws", giver="Chef Lukkif", levels=(50, 60), xp=2000000,
             item=("Basilisk Claw", "Broken Leopard Claw", "A claw off a fire basilisk. Chef Lukkif cooks with them.",
                   {"trade": 1, "maxstack": 99}),
             target="fire basilisk", also=[],
             offer="The fire basilisks near my fire have claws that crack into a fine broth. Bring me four.",
             loot="Kill fire basilisks for four basilisk claws.",
             give="Bring four basilisk claws to Chef Lukkif.", says="A fine broth!",
             notes="teamjosh H&G key 76 (the owner's source, 2026-11-15): 50-60, Chef Lukkif, Basalik claws off the Fire "
                   "Basalik (the page's spelling; the game's fire basilisks, by him in Lavastorm). The page gives no "
                   "experience: 2,000,000 is ours, as Chef Lukkif's other rows without one; count 4 and chance 0.5 ours."),
    82: dict(name="Preserved Ghast Eyes", giver="Terison Eydillo", levels=(20, 20), xp=300000,
             item=("Preserved Ghast Eye", "Preserved Zombie Eye", "A ghast's eye, preserved. Terison Eydillo of Collonridge pays for them.",
                   {"trade": 1, "maxstack": 20}),
             target="lesser ghoul", also=[],
             offer="The ghasts of Collonridge Cemetery carry preserved ghast eyes. Bring me four.",
             loot="Kill the ghouls of Collonridge Cemetery for four preserved ghast eyes.",
             give="Bring four preserved ghast eyes to Terison Eydillo.", says="Well done.",
             notes="teamjosh H&G key 82 (the owner's source, 2026-11-15): 20, 300,000, Terison Eydillo, preserved ghast "
                   "eyes (NO TRADE, twenty to a stack) off the Enraged Ghasts of Collonridge Cemetery. No ghast stands "
                   "there: Collonridge's lesser ghouls (23-25) stand for them. Count 4 and chance 0.5 ours, as Preserved "
                   "Zombie Eyes."),
}
COPIES = (31, 26, 20, 55, 9, 83, 45, 44)  # Heirloom's (board entry 1081), in the data file

# not made: why, and the names to look for (giver, item, creature: SQL LIKE patterns)
CANNOT = {
    30: ("its giver is 'NPC - near banker in PoD', unnamed", ["%near banker%"], ["toad eye%"], ["%toad%"]),
    28: ("its giver Handyman Hilty is not in the game", ["%Hilty%", "Handyman%"], ["rat whisker%"], ["ratman%"]),
    25: ("its giver Tuskcarver is not in the game", ["%Tuskcarver%"], ["tusk", "tusks"], ["%swine", "boar", "great boar"]),
    23: ("its giver is unnamed ('- Oasis')", [], ["poison stinger%"], ["%singetail%", "%scorpion%"]),
    19: ("its giver Escaping Slave is not in the game; and it is no turn-in (keys free the enslaved miners)",
         ["%Escaping Slave%"], ["siloth slaver key%"], ["siloth slaver", "siloth guard", "siloth knight", "siloth ritualist"]),
    16: ("its giver Fist of Marr Recruiter is not in the game", ["%Fist of Marr%"], ["platinum insignia%"], ["%deathfist reaver%", "%deathfist slayer%"]),
    15: ("its giver Fist of Marr Recruiter is not in the game", ["%Fist of Marr%"], ["insignia ring%"], ["deathfist champion", "deathfist cleric"]),
    32: ("its giver Fist of Marr Recruiter is not in the game", ["%Fist of Marr%"], ["deathfist seal%"], ["deathfist myrmidon"]),
    10: ("its giver Fist of Marr Recruiter is not in the game", ["%Fist of Marr%"], ["dark silver ring%"], ["deathfist legionnaire"]),
    8: ("its giver Fist of Marr Recruiter is not in the game", ["%Fist of Marr%"], ["blackened earring%"], ["plainsraider%"]),
    50: ("its giver Abrion Oleken is not in the game", ["%Oleken%", "Abrion%"], ["tattered shark skin%"], ["%sandshark%", "%sand shark%"]),
    11: ("its page names no item (and no experience)", ["Big Basher Biggum"], [], ["froglok%"]),
    2: ("its page names no item (its reward a pie)", ["Sprexy%"], [], ["%lioness%"]),
}


# ------------------------------------------------------------------------------------------------------- helpers
def key(n):
    return (n or "").strip().lower()


def fold(s):
    return re.sub(r"[^a-z0-9]", "", (s or "").lower())


def cols(db, t):
    return [r[1] for r in db.execute(f'PRAGMA table_info("{t}")')]


def has(db, t):
    return sgg.has(db, t)


def source():
    with open(DATA) as f:
        d = json.load(f)
    return {s["key"]: s for s in d["source"]}, {c["key"]: c for c in d["copies"]}, d["items"]


_NAMES = {}


def npc_name(db, name):
    """The creature's or NPC's name as this database spells it (its first row's), or None. Read once: npcs has no
    index on the name, and the apply's write lock is held while it runs."""
    if id(db) not in _NAMES:
        names = {}
        for (n,) in db.execute("SELECT trim(npc_name) FROM npcs ORDER BY id"):
            names.setdefault(key(n), n)
        _NAMES[id(db)] = names
    return _NAMES[id(db)].get(key(name))


_ITEMS = {}


def item_ids(db, name):
    """The records of the name, read once (and told of the ones made here)."""
    if id(db) not in _ITEMS:
        items = {}
        for pid, n in db.execute("SELECT patternid, itemname FROM itempattern ORDER BY patternid"):
            items.setdefault(key(n), []).append(pid)
        _ITEMS[id(db)] = items
    return list(_ITEMS[id(db)].get(key(name), []))


def parse_items(s):
    out = []
    for part in [p.strip() for p in (s or "").split(",") if p.strip()]:
        pid, _, n = part.partition("*")
        out.append((int(pid), int(n or 1)))
    return out


def counts(pairs):
    return ", ".join(str(p) if n == 1 else f"{p}*{n}" for p, n in pairs)


def steps(db, q):
    c = cols(db, "queststeps")
    return [dict(zip(c, r)) for r in db.execute("SELECT * FROM queststeps WHERE questid = ? ORDER BY step", (q,))]


def find(db, m):
    """Our quests standing for a matcher: its giver's, handing in its item (and looting off its creature)."""
    ids = set(item_ids(db, m.item))
    out = []
    for qid, giver, mn, mx in db.execute("SELECT questid, giver, minlevel, maxlevel FROM quests WHERE lower(trim(giver)) = ?",
                                         (key(m.giver),)):
        st = steps(db, qid)
        give = [s for s in st if s["kind"] == "give" and key(s["target"]) == key(giver)
                and any(p in ids for p, _ in parse_items(s["items"]))]
        if not give:
            continue
        loot = [s for s in st if s["kind"] == "loot" and any(p in ids for p, _ in parse_items(s["items"]))]
        if m.target and not any(key(s["target"]) == key(m.target) for s in loot):
            continue
        out.append(dict(questid=qid, minlevel=mn, maxlevel=mx, give=give[-1], loot=loot[0] if loot else None))
    return out


def also_of(db, qid, step):
    return {key(t) for (t,) in db.execute("SELECT target FROM queststeptargets WHERE questid = ? AND step = ?", (qid, step))}


def money(n):
    return f"{n:,}"


# ------------------------------------------------------------------------------------------------- the comparison
def want(src, m):
    """What a matcher's quest should be: (minlevel, maxlevel, xp or None, tunar or None, chance or None)."""
    lo, hi = m.levels or (max(src["minlevel"], 1), src["maxlevel"])
    xp = m.xp if m.xp is not None else src["xp"]
    if src["xp_text"].endswith("ish") and m.xp is None:
        xp = None  # "950,000ish": the poster's guess, ours stands
    tunar = m.tunar if m.tunar is not None else src["tunar"]
    if src["tunar_text"].endswith("ish") and m.tunar is None:
        tunar = None
    return lo, hi, xp, tunar, m.chance


def compare(db, src, ms):
    """(found, rows): each matcher's quest and its differences, as (field, ours, source's, written here?)."""
    rows = []
    for m in ms:
        got = find(db, m)
        if not got:
            rows.append((m, None, []))
            continue
        for g in got:
            lo, hi, xp, tunar, chance = want(src, m)
            d = []
            if (g["minlevel"], g["maxlevel"]) != (lo, hi):
                d.append(("levels", f"{g['minlevel']}-{g['maxlevel']}", f"{lo}-{hi}", True))
            if xp is not None and g["give"]["xp"] != xp:
                d.append(("xp", money(g["give"]["xp"]), money(xp), True))
            if tunar is not None and g["give"]["rewardtunar"] != tunar:
                d.append(("tunar", money(g["give"]["rewardtunar"]), money(tunar), True))
            if chance is not None and g["loot"] and abs(g["loot"]["chance"] - chance) > 1e-9:
                d.append(("drop rate", g["loot"]["chance"], chance, True))
            if m.also and g["loot"]:
                have = also_of(db, g["questid"], g["loot"]["step"]) | {key(g["loot"]["target"])}
                lack = [a for a in m.also if key(a) not in have]
                here = [a for a in lack if npc_name(db, a)]
                gone = [a for a in lack if not npc_name(db, a)]
                if here:
                    d.append(("creatures", g["loot"]["target"], "+ " + ", ".join(here), True))
                if gone:
                    d.append(("creatures", g["loot"]["target"], "+ " + ", ".join(gone) + " (not in this database)", False))
            rows.append((m, g, d))
    return rows


def missing_names(db, why, givers, itemsp, mobs):
    def like(t, col, pats):
        found = []
        for p in pats:
            found += [r[0] for r in db.execute(f"SELECT DISTINCT trim({col}) FROM {t} WHERE {col} LIKE ? LIMIT 6", (p,))]
        return sorted(set(found))[:6]
    g = like("npcs", "npc_name", givers) if givers else []
    i = like("itempattern", "itemname", itemsp) if itemsp else []
    c = like("npcs", "npc_name", mobs) if mobs else []
    return (f"{why}; giver {'here: ' + ', '.join(g) if g else 'not here'}; item {'here: ' + ', '.join(i) if i else 'not here'}; "
            f"creature {'here: ' + ', '.join(c) if c else 'not here'}")


def report(db, src, copies):
    """Every source quest's state on this database: [(key, status, lines)]."""
    out = []
    for k in sorted(src, key=lambda k: (-src[k]["minlevel"], -src[k]["maxlevel"], k)):
        s = src[k]
        head = f"key {k:>2} {s['minlevel']}-{s['maxlevel']} {s['giver'] or '(unnamed)'}: {s['item'] or '(no item)'} / {s['mob']}"
        if k in CANNOT:
            out.append((k, "MISSING", head, [missing_names(db, *CANNOT[k])]))
            continue
        rows = compare(db, s, PRESENT[k])
        found = [r for r in rows if r[1]]
        if not found:
            if k in NEW:
                n = NEW[k]
                lack = [x for x in (n["giver"], n["target"]) if not npc_name(db, x)]
                line = (f"we lack it: giver {n['giver']} {'here' if npc_name(db, n['giver']) else 'NOT here'}, creature "
                        f"{n['target']} {'here' if npc_name(db, n['target']) else 'NOT here'}, item {n['item'][0]} "
                        f"{'here' if item_ids(db, n['item'][0]) else 'to make'}")
                out.append((k, "MISSING", head, [line + ("; cannot be made: " + ", ".join(lack) + " not here" if lack else "; made (fix)")]))
            elif k in copies:
                c = copies[k]
                g = c["quest"]["giver"]
                t = c["steps"][0]["target"]
                lack = [x for x in (g, t) if not npc_name(db, x)]
                line = (f"we lack it (Heirloom has it): giver {g} {'here' if npc_name(db, g) else 'NOT here'}, creature {t} "
                        f"{'here' if npc_name(db, t) else 'NOT here'}")
                out.append((k, "MISSING", head, [line + ("; cannot be made: " + ", ".join(lack) + " not here" if lack else "; copied (fix)")]))
            else:
                out.append((k, "MISSING", head, ["no quest of ours found (no rule for it)"]))
            continue
        lines, diff = [], False
        for m, g, d in rows:
            if not g:
                lines.append(f"no quest of {m.giver} for {m.item} here")
                diff = True
                continue
            label = f"quest {g['questid']}"
            for f, ours, theirs, fix in d:
                lines.append(f"{label}: {f} {ours} -> {theirs}{' (fix)' if fix else ' (said)'}")
                diff = True
        if NOTES.get(k):
            lines.append("not a figure: " + NOTES[k])
            diff = True
        if ASIDES.get(k):
            lines.append("aside: " + ASIDES[k])
        ids = ", ".join(str(g["questid"]) for _, g, _ in rows if g)
        out.append((k, "DIFFERENT" if diff else "PRESENT", head + f"  [ours {ids}]", lines))
    return out


# ------------------------------------------------------------------------------------------------------- writing
def make_item(db, name, row_like, desc, sets):
    c = cols(db, "itempattern")
    if isinstance(row_like, dict):
        row = {k: v for k, v in row_like.items() if k in c and k != "patternid"}
    else:
        ids = item_ids(db, row_like)
        if not ids:
            raise SystemExit(f"item {name!r}: no {row_like!r} to copy")
        row = dict(zip(c, db.execute("SELECT * FROM itempattern WHERE patternid = ?", (ids[0],)).fetchone()))
        row.pop("patternid")
    row.update(itemname=name)
    if desc is not None:
        row["itemdesc"] = desc
    row.update(sets)
    cur = db.execute(f'INSERT INTO itempattern ({",".join(row)}) VALUES ({",".join("?" * len(row))})', list(row.values()))
    item_ids(db, name)
    _ITEMS[id(db)].setdefault(key(name), []).append(cur.lastrowid)
    return cur.lastrowid


def item_for(db, name, row_like, desc, sets, say):
    ids = item_ids(db, name)
    if ids:
        return ids[0]
    pid = make_item(db, name, row_like, desc, sets)
    say(f"  item {name!r} made ({pid})")
    return pid


def next_quest(db):
    return db.execute("SELECT COALESCE(max(questid), 0) + 1 FROM quests").fetchone()[0]


def put_row(db, t, row):
    row = {k: v for k, v in row.items() if k in cols(db, t)}
    db.execute(f'INSERT INTO {t} ({",".join(row)}) VALUES ({",".join("?" * len(row))})', list(row.values()))


def add_new(db, k, n, say):
    pid = item_for(db, n["item"][0], n["item"][1], n["item"][2], n["item"][3], say)
    qid = next_quest(db)
    put_row(db, "quests", dict(questid=qid, name=n["name"], enabled=1, giver=npc_name(db, n["giver"]), minlevel=n["levels"][0],
                               maxlevel=n["levels"][1], repeatable=1, offer=n["offer"], accept="I will do it.", decline="Not now.",
                               declined="Then go. Come back when you are ready.", started="Off you go. I will be here.",
                               source=QSOURCE, notes=n["notes"]))
    target = npc_name(db, n["target"])
    put_row(db, "queststeps", dict(questid=qid, step=1, kind="loot", target=target, items=f"{pid}*4", count=4, chance=0.5,
                                   journal=n["loot"]))
    put_row(db, "queststeps", dict(questid=qid, step=2, kind="give", target=npc_name(db, n["giver"]), items=f"{pid}*4", count=1,
                                   journal=n["give"], says=n["says"], xp=n["xp"]))
    for a in n["also"]:
        if npc_name(db, a):
            db.execute("INSERT OR IGNORE INTO queststeptargets (questid, step, target) VALUES (?, 1, ?)", (qid, npc_name(db, a)))
    say(f"  quest {qid} {n['name']} (key {k}) made")


def add_copy(db, k, c, items, say):
    ids = {}
    for st in c["steps"]:
        for name, _ in st["items"] + st["rewarditems"]:
            if name not in ids:
                ids[name] = item_for(db, name, items[name], None, {}, say)
    qid = next_quest(db)
    q = dict(c["quest"], questid=qid, source=QSOURCE,
             notes=c["quest"]["notes"] + " Copied from Heirloom's (board entry 1081) by tools/handg_quests_20261115.py.")
    put_row(db, "quests", q)
    for st in c["steps"]:
        row = dict(st, questid=qid, items=counts((ids[n], x) for n, x in st["items"]),
                   rewarditems=counts((ids[n], x) for n, x in st["rewarditems"]))
        put_row(db, "queststeps", row)
    for step, t in c["also"]:
        if npc_name(db, t):
            db.execute("INSERT OR IGNORE INTO queststeptargets (questid, step, target) VALUES (?, ?, ?)", (qid, step, npc_name(db, t)))
    say(f"  quest {qid} {q['name']} (key {k}) copied from Heirloom's")


def fix_present(db, src, say):
    n = 0
    for k, ms in PRESENT.items():
        s = src[k]
        for m, g, d in compare(db, s, ms):
            if not g:
                continue
            lo, hi, xp, tunar, chance = want(s, m)
            for f, ours, theirs, fix in d:
                if not fix:
                    continue
                q, gs, ls = g["questid"], g["give"]["step"], g["loot"]["step"] if g["loot"] else None
                if f == "levels":
                    db.execute("UPDATE quests SET minlevel = ?, maxlevel = ? WHERE questid = ?", (lo, hi, q))
                elif f == "xp":
                    db.execute("UPDATE queststeps SET xp = ? WHERE questid = ? AND step = ?", (xp, q, gs))
                elif f == "tunar":
                    db.execute("UPDATE queststeps SET rewardtunar = ? WHERE questid = ? AND step = ?", (tunar, q, gs))
                elif f == "drop rate":
                    db.execute("UPDATE queststeps SET chance = ? WHERE questid = ? AND step = ?", (chance, q, ls))
                elif f == "creatures":
                    for a in m.also:
                        if npc_name(db, a) and key(a) != key(g["loot"]["target"]):
                            db.execute("INSERT OR IGNORE INTO queststeptargets (questid, step, target) VALUES (?, ?, ?)",
                                       (q, ls, npc_name(db, a)))
                n += 1
                say(f"  key {k} quest {q}: {f} {ours} -> {theirs}")
                line = f" teamjosh H&G key {k} (the owner's source, 2026-11-15): its figures and creatures, tools/handg_quests_20261115.py."
                db.execute("UPDATE quests SET notes = notes || ? WHERE questid = ? AND instr(notes, ?) = 0", (line, q, line.strip()))
    return n


def run_on(db, say):
    src, copies, items = source()
    say("fixes:")
    fixed = fix_present(db, src, say)
    say("added:")
    added = 0
    for k in sorted(set(NEW) | set(COPIES)):
        if any(find(db, m) for m in PRESENT[k]):
            continue
        if k in NEW:
            n = NEW[k]
            if npc_name(db, n["giver"]) and npc_name(db, n["target"]):
                add_new(db, k, n, say)
                added += 1
        else:
            c = copies[k]
            if npc_name(db, c["quest"]["giver"]) and npc_name(db, c["steps"][0]["target"]):
                add_copy(db, k, c, items, say)
                added += 1
    say(f"{fixed} figures put right, {added} quests added")


def audit_rows(db, counts_):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    for tbl, op, rid, old in db.execute(f"SELECT tbl, op, rid, old FROM {sgg.LOG} WHERE setname = ? ORDER BY seq", (SET,)).fetchall():
        if tbl in ("sqlite_sequence", sgg.LOG) or op == "made":
            continue
        new = db.execute(f'SELECT * FROM "{tbl}" WHERE rowid = ?', (rid,)).fetchone()
        after = json.dumps(dict(zip(cols(db, tbl), new)), separators=(",", ":")) if new else None
        db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                   "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                   (at, acct[0] if acct else 0, WHO, VIA, {"added": "insert", "changed": "update", "gone": "delete"}[op], tbl,
                    json.dumps({"rowid": rid}), old or None, after, DETAIL))


def audit_undo(db, n):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', ?, ?, NULL, NULL, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, sgg.LOG, json.dumps({"setname": SET}), f"undone, {n} rows put back: " + DETAIL))


def is_in(db):
    return has(db, sgg.LOG) and db.execute(f"SELECT 1 FROM {sgg.LOG} WHERE setname = ?", (SET,)).fetchone() is not None


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    ap.add_argument("--json", help="the comparison, as JSON, to this file")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    say = print
    try:
        for t in ("quests", "queststeps", "queststeptargets", "itempattern", "npcs", "gm_audit"):
            if not has(db, t):
                print(f"refused, nothing done: no table {t}")
                return 1
        if a.undo:
            if not is_in(db):
                print("nothing to undo")
                return 0
            sgg.undo(db, SET, say, extra=audit_undo)
            print("restart the server for the quests (read at its start)")
            return 0
        src, copies, _ = source()
        rep = report(db, src, copies)
        tally = {}
        for k, status, head, lines in rep:
            tally[status] = tally.get(status, 0) + 1
            print(f"{status:9} {head}")
            for line in lines:
                print(f"            {line}")
        print(f"\n{len(rep)} source quests: " + ", ".join(f"{v} {k}" for k, v in sorted(tally.items())))
        if a.json:
            json.dump([dict(key=k, status=s, head=h, lines=ls) for k, s, h, ls in rep], open(a.json, "w"), indent=1)
        if is_in(db):
            print(f"applied already ({SET} is in {sgg.LOG}): nothing to do (--undo to take it back)")
            return 0
        print()
        sgg.run = lambda db_, spec, say_: run_on(db_, say_)  # this tool's rows, through seed_guide_gaps' snapshot, log and undo
        sgg.apply(db, {"set": SET}, a.apply, say, extra=audit_rows)
        if a.apply:
            print("quests are read at the server's start: this takes effect at the next restart")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
