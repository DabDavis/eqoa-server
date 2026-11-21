#!/usr/bin/env python3
"""Camp loot from MasterVG782's GameFAQs guide (the owner, 2026-11-21: all four follow-ups of the loot audit
~/.cache/lootaudit-20261121/report.md approved).

Four parts, all in one run:

1. WIRE existing records that drop from nothing (or not from the guide's camp):
   - rares, at a rare chance a kill: Rhino Horn Axe of Lightning (2085) from the 51-60 ratmen, the Grizzes' tunarian
     shield (6118), Poisoned Fang (6012) from the Grimrik trolls, goblin assassin's dirk (5350) from the goblins;
   - the named "always drops", 100%: Quintashi's Trident (5365) from Quintashi, slayer's claw earring (1035) from
     Slayer Demaeog, gold ingot (1034) from Augur Rurrg. Archon's Heart (1010) is NOT wired unless --archon-1010: the
     Avatar of Fear chain (quest 101282 step 1) wants record 701422 "Archon Heart", which the Tae Ew Archon already
     drops at 100% by that loot step; 1010 is a second, Lore record the chain never asks for;
   - every other existing record a camp lists that its mobs do not drop: a common drop.
2. CREATE the camp trash the guide lists and we lack: a stackable vendor-trash record copied from Bandit Ear (707134),
   the guide's name (lowercase as it writes it), itemcost round(guide price x 0.32) (no price: the camp's median price
   x 0.32, or 50), a one-sentence description, an icon taken from an existing stackable of the same kind by name
   (printed), patternids from 720200 up (the first free ones); each a common drop from its camp's mobs. Gear (weapons,
   armour, held and non-stacking pieces) and ambiguous names are not made: they are listed.
3. LIVE's Aktana soul: quests 101058 and 101087 step 5 (kind loot) target "Aktana the Flamekissed"; a step's other
   creatures are table queststeptargets (handlers/quests.go stepNPC: the target or one of its others, by name, any
   case, for kill and loot steps; the kill credit (killMatches) and the pre-loot use it). Where the database has a
   creature named "Aktrana the Flamekissed" (Live: row 121434, the double Heirloom deleted 2026-11-20, board 1833),
   it becomes the steps' other creature, so its kill drops the soul too. Heirloom (no such row): nothing.
4. (report only: the guide's quest items with no record are not made.)

How a camp item drops: a table per item, "guide1121 <item>" (loottables: the record, weight 1, levels 1-70), bound by
npcloot (rolls 1) to each of the camp's mobs: by name (npcclass key: lowercase) where every row of that name stands in
the camp's world, else by row key ("#<npcs.id>") for the rows in the camp's world only (a name shared with another
world's creatures, or a dungeon world's, which rolls row keys alone: handlers/loot.go lootNames). A mob that already
drops the record (its npcitem rows, a table bound to its name or row, its own lootpool rows, a quest loot step on it)
is left as it is. Chances: rares as RARES below; commons 25% at a guide price of 50 down to 15% at 1350 and over
(rounded). Equipment more than 10 levels above a creature never drops from it (handlers/loot.go lootFits): such a
binding is left out and said.

Mob and item names are read from the database given (Heirloom and Live differ); the guide's spellings map to ours
by MOB_ALIAS and ITEM_ALIAS (hand-checked, 2026-11-21).

Dry run by default: every change printed. --apply writes it in one transaction, each row with a gm_audit line; a
second --apply finds everything in place and writes nothing. Loot (itempattern, loottables, npcloot) takes effect at
once (read at each corpse's first opening); part 3 (queststeptargets) at the next restart (quests are read at start).

    python3 -E -s tools/guide_loot_20261121.py --db eqoa.db [--apply | --undo] [--archon-1010]

Every row written is logged in guide_loot_1121_log (before and after); --undo deletes exactly those rows (a row changed
since is left and said), each with a gm_audit line, and drops the log.
"""
import argparse
import collections
import datetime
import difflib
import json
import re
import sqlite3
import statistics
import sys

TOOL = "tools/guide_loot_20261121.py"
SRC = "guide-loot-20261121"
LOG = "guide_loot_1121_log"
WHO, ACCOUNT, VIA = "dab", 1, "tool"
TEMPLATE = 707134          # Bandit Ear: stackable vendor trash
FIRST_ID = 720200          # 720101-720104 are Saftemoot's (2026-11-21)
SAFTEMOOT_IDS = (720101, 720102, 720103, 720104)
LEVELS = (1, 70)
REACH = 10                 # handlers/loot.go lootLevelReach
GEAR_SLOTS = 19            # handlers/loot.go lootGearSlots

def detail(camp):
    return f"the owner 2026-11-21: MasterVG782's GameFAQs guide (camp {camp}): {TOOL}"


# The 51-60 ratmen camp's generic "ratman": our level 50-62 ratmen (all in Odus).
RATMEN_5160 = ["elite ratman", "ratman augur", "ratman channeler", "ratman diviner", "ratman slayer", "ratman sorcerer",
               "ratman spiritist", "ratman trickster", "ratman warrior", "ratman plaguespreader", "fishmaster ratman"]
MOB_ALIAS = {  # the guide's spelling -> ours (the first that exists in the database)
    "deathfist legionairre": ["deathfist legionnaire"],
    "clan kobolk scout": ["clan kolbok scout"],
    "golgarhet": ["golgareht"],
    "froglok guk channeler": ["froglok channeler"],
    "arctic spiritcaller": ["arctic spiritcaller", "artic spiritcaller"],
}
ITEM_ALIAS = {  # the guide's name -> an existing record's (wired, never made again)
    "bandit necklace": "bandit's necklace",
    "ghoulish preservatives": "ghoulish preservative",
    "chared bone": "charred bones",
    "flavored mushroom caps": "flavored mushroom",
    "canteen of purified water": "purified water canteen",
    "blood of a madman": "blood of the desert madman",
    "ratman whiskers": "rat whisker",
    "crocodile scales": "crocodile scale",
    "gargoyle dust": "mound of gargoyle dust",
}
# Not made: gear (weapons, armour, held items, the guide's non-stacking pieces) -> why
GEAR = {
    "jeweled kris": "a dagger; the guide: doesn't stack",
    "seeing orb": "a held orb; the guide: doesn't stack",
    "carved mammoth tusk": "the guide: doesn't stack (a held or worn piece)",
    "mammoth hide tunic": "armour; the guide: doesn't stack",
    "granite bracer": "armour (the kappas' granite set)",
    "granite sword": "a weapon (the kappas' granite set)",
    "granite two-handed sword": "a weapon (the kappas' granite set)",
    "granite stone ring": "jewellery of the kappas' granite set",
    "battered shield": "a shield",
    "goblin staff": "a weapon",
    "frost giant helm": "armour",
    "frosteye bootdagger": "a weapon",
    "orcish icepick": "a weapon",
    "crystal mining pick": "a weapon or tool",
    "sweaty bandana": "head armour",
    "lantern": "a held light",
    "orcish lantern": "a held light",
    "mercurial construct rod": "a weapon or held rod",
}
AMBIGUOUS = {
    "various mundane weapons": "no item named",
    "patch of ratman fur": "we have 'patch of rat fur' (1278), a rat's: the same thing or not?",
    "lava ruby": "we have 'glowing lava ruby': the same gem or not?",
    "rotting skull": "we have 'fetid rotting skull': the same thing or not?",
}
QUEST_NOTES = ("quest item",)
RARES = [  # (patternid, its name, camp, guide mobs, chance %)
    (2085, "rhino horn axe of lightning", "RATMEN (51-60)", ["ratman"], 1),
    (6118, "tunarian shield", "GRIZZES", ["grizz shaman", "grizz warrior"], 2),
    (6012, "poisoned fang", "GRIMRIK TROLLS",
     ["Chief Grimrik", "Grimrik shadowknight", "Grimrik shaman", "Grimrik warrior", "Morgugk the Wise"], 2),
    (5350, "goblin assassin's dirk", "GOBLINS", ["Goblin Assassin", "Goblin Soothsayer", "Goblin Elder"], 1.5),
]
NAMED = [  # always drops: (patternid, its name, camp, mob, 100)
    (5365, "quintashi's trident", "QUINTASHI'S LAIR", ["Quintashi"], 100),
    (1035, "slayer's claw earring", "SLAYERS", ["Slayer Demaeog"], 100),
    (1034, "gold ingot", "SLAYERS", ["Augur Rurrg"], 100),
]
ARCHON = (1010, "archon's heart", "PERMAFROST/AVATAR OF FEAR (Tae Ew Archons)", ["Tae Ew Archon"], 100)

# The guide's camps with a LOOT list (~/.cache/gfaqs-20261121/guide.txt, section XVI): (camp, band, mobs, items)
CAMPS = [
    ('BBW CROCS', 'LEVELS 1-20', ['caiman', 'young crocodile'], [('caiman claw', 79, ''), ('caiman egg', 86, ''), ('caiman scales', 76, ''), ('chipped crocodile tooth', 43, ''), ('cracked caiman tooth', 44, ''), ('damaged crocodile scale', 74, ''), ('scratched crocodile claw', 78, '')]),
    ('COLDPAW GNOLLS', 'LEVELS 1-20', ['coldpaw brawler', 'coldpaw shaman', 'coldpaw warrior'], [('cracked tooth', 21, ''), ('torn sash', 16, '')]),
    ('DESERT MADMEN', 'LEVELS 1-20', ['decaying skeleton', 'desert madman'], [('blood of a madman', None, 'quest item'), ('small bone chips', 29, ''), ('withered bones', 22, '')]),
    ('FP CROCS', 'LEVELS 1-20', ['crocodile', 'tame crocodile'], [('caiman scales', 76, ''), ('chipped crocodile tooth', 43, ''), ('cracked caiman tooth', 44, ''), ('crocodile claw', 124, ''), ('crocodile scales', 105, ''), ('crocodile tooth', 76, ''), ('damaged crocodile scale', 74, ''), ('scratched crocodile claw', 78, '')]),
    ('BLOODFOOT BANDITS', 'LEVELS 21-30', ['Bloodfoot armorer', 'Bloodfoot bandit', 'Bloodfoot healer', 'Bloodfoot heavy', 'Bloodfoot jailor', 'Bloodfoot mystic', 'Bloodfoot warlock', 'Boss Hurg'], [('bloodfoot pendant', 132, ''), ('bloodfoot swill', 101, '')]),
    ('CATAMOUNT', 'LEVELS 21-30', ['catamount'], [('various mundane weapons', None, 'no price')]),
    ('CAVE CRAWLERS', 'LEVELS 21-30', ['cave crawler', 'queen crawler'], [('cave spider eye', 119, ''), ('cave spider silk', 211, ''), ('cave spider fang', 94, ''), ('dark ore', 201, '')]),
    ('LAKESIDE BANDITS', 'LEVELS 21-30', ['bandit', 'Furious John'], [('bandit necklace', 150, ''), ('bandit scarf', 90, ''), ('jeweled kris', 125, "doesn't stack")]),
    ('MERKANS', 'LEVELS 21-30', ['merkan guard', 'merkan shaman', 'merkan villager', 'merkan warrior'], [('morganite necklace', 345, ''), ('morganite ring', 170, '')]),
    ('MTG DEATHFISTS', 'LEVELS 21-30', ['deathfist legionairre', 'deathfist healer', 'deathfist oracle', 'Chief Kimlosh'], [('dark silver ring', 92, ''), ('worn net', 83, '')]),
    ('OTHER BANDITS', 'LEVELS 21-30', ['bandit', 'Allyn Fairfarm', 'Gregor Skyclad', 'Karina Skyclad', 'Franco Windforge'], [('bandit necklace', 150, ''), ('bandit scarf', 90, ''), ('jeweled kris', 125, "doesn't stack")]),
    ('GRIMRIK TROLLS', 'LEVELS 31-40', ['Chief Grimrik', 'Grimrik shadowknight', 'Grimrik shaman', 'Grimrik warrior', 'Morgugk the Wise'], [('cave spider silk', 211, ''), ("Grimrik's clan stone", 108, ''), ("Grimrik's stone", 159, ''), ('troll shadowknight sash', 171, ''), ('troll shaman sash', 173, ''), ('troll warrior sash', 143, '')]),
    ('HP BANDITS', 'LEVELS 31-40', ['highway bandit', 'highway thug'], [('bandit belt', 150, ''), ('lantern', 198, '')]),
    ('LIZARDMEN -- AGGRESSORS', 'LEVELS 31-40', ['tae ew aggressor', 'tae ew sacrificer'], [('bone spiked tiara', 200, ''), ('silver chain sash', 148, '')]),
    ('MTG GHARIALS', 'LEVELS 31-40', ['ancient gharial', 'Mudscale'], [('gharial claw', 320, ''), ('gharial egg', 349, ''), ('gharial scales', 285, ''), ('serrated gharial tooth', 190, '')]),
    ('AVIAKS', 'LEVELS 41-50', ['aviak', 'aviak scouter'], [('aviak egg shell', 527, ''), ('chipped aviak talon', 378, ''), ('chunk of aviak meat', 258, ''), ('faded aviak feathers', 332, ''), ('twisted aviak tail', 432, '')]),
    ('GARGOYLES', 'LEVELS 41-50', ['gargoyle soldier', 'gargoyle watcher', 'Garhuuga', 'leering gargoyle', 'statue guardian'], [('chipped gargoyle claw', 276, ''), ('gargoyle dust', 243, ''), ('gargoyle tooth', 450, ''), ('jeweled gargoyle eye', 331, ''), ('seeing orb', 1145, "doesn't stack")]),
    ('GREAT EAGLES', 'LEVELS 41-50', ['great eagle'], [('great eagle feathers (poor)', 452, ''), ('great eagle feathers (good)', 593, ''), ('great eagle feathers (excellent)', 690, ''), ('great eagle talon', 367, '')]),
    ('GRIFFON WARRIORS', 'LEVELS 41-50', ['griffon elder', 'griffon nest protector', 'griffon warrior', 'Windtuft'], [('large griffon eye', 245, ''), ('mountain feather', 165, ''), ('mountain griffon talon', 1174, '')]),
    ('HEXBONES', 'LEVELS 41-50', ['hexbone'], [('bones of the void', 233, ''), ('skull of the void', 296, '')]),
    ('ICE GIANT REFS', 'LEVELS 41-50', ['ice giant refugee', 'ice giant renegade', 'Tuggrin Bigeye'], [('carved mammoth tusk', 376, "doesn't stack"), ('cold iron nosering', 402, ''), ('giant blue pearl ring', 257, ''), ('giant bone necklace', 558, ''), ('large silver collar', 318, ''), ('mammoth hide tunic', 478, "doesn't stack")]),
    ('NASEHIRS', 'LEVELS 41-50', ['Nasehir cleric', 'Nasehir rogue', 'Nasehir warrior', 'Nasehir wizard'], [('platinum necklace', 310, ''), ('sandstone pendant', 245, '')]),
    ('PHANTOMS', 'LEVELS 41-50', ['elder spirit', 'Himlar', 'kappa drudge', 'phantom', 'spirit', 'The Ancient One'], [('brass chalice', 235, ''), ('broken arrowhead', 190, ''), ("merchant's flute", 455, ''), ('sandy brick slab', 380, '')]),
    ('RATMEN (41-50)', 'LEVELS 41-50', ['ratman fighter', 'ratman scavenger'], [('chipped ratman claw', 373, ''), ('patch of ratman fur', 280, '')]),
    ('SNOW GRIFFONS', 'LEVELS 41-50', ['Elder Squeeks', 'snow griffon', 'Whitefeather'], [('slab of snow griffon meat', 326, ''), ('snow griffon beak', None, ''), ('snow griffon claw', 535, ''), ('snow griffon egg', 419, ''), ('snow griffon feather', 603, ''), ('snow griffon talon', 530, '')]),
    ('TENDORA-BUKI VILLAGERS', 'LEVELS 41-50', ['Tendora-buki guard', 'Tendora-buki villager'], [('tendora-buki earring', 750, ''), ('tendora-buki necklace', 380, '')]),
    ('ANIMATED LAVAS', 'LEVELS 51-60', ['animated lava'], [('heated stone', 721, ''), ('smoldering embers', 690, '')]),
    ('CORRODED CLOCKWORKS', 'LEVELS 51-60', ['corroded clockwork'], [('1 sided spinner', 408, ''), ('multi-toothed clanger', 522, ''), ('platinum sprockets', 589, '')]),
    ('CV UNDEAD', 'LEVELS 51-60', ['Wandering Ghast', 'Wandering Ghost'], [('battered shield', 701, ''), ('ghoulish preservatives', None, 'chef loot'), ('rotting skull', 675, '')]),
    ('ENCHANTING SIRENS', 'LEVELS 51-60', ['enchanting siren', 'Plagued Siren'], [("a siren's crown", 670, ''), ("siren's sash", 740, '')]),
    ('FIRE BASILISKS', 'LEVELS 51-60', ['fire basilisk'], [('lava ruby', 725, ''), ('singed tooth', 690, '')]),
    ('FROSTEYE ORCS', 'LEVELS 51-60', ['frosteye centurion', 'frosteye patroller', 'frosteye sage'], [('broken frosteye totem', 663, 'HnG'), ('frosteye bootdagger', 402, ''), ('frosteye legion medal', 663, ''), ('intact orc tusk', 864, ''), ('orcish icepick', 472, ''), ('orcish lantern', 558, '')]),
    ("GEOMANCER'S CITADEL", 'LEVELS 51-60', [], [('coarse salt', None, 'chef loot'), ('mercurial construct rod', 678, ''), ('mercurial stone', 778, ''), ('mithril flakes', 778, ''), ('mithriled gargoyle eye', 678, '')]),
    ('GOBLINS', 'LEVELS 51-60', ['Goblin Assassin', 'Goblin Soothsayer', 'Goblin Elder'], [('crusty goblin eye', None, 'chef loot'), ('goblin eye', 701, ''), ('goblin staff', 675, '')]),
    ('GRIZZES', 'LEVELS 51-60', ['grizz shaman', 'grizz warrior'], [('chared bone', 800, 'HnG'), ('glowing lava rock', 400, '')]),
    ('GUK FROGLOKS', 'LEVELS 51-60', ['froglok guk channeler', 'froglok guk forager'], [('froglok legs', 690, ''), ('preserver froglok liver', 721, ''), ('froglok stomach', None, 'chef item')]),
    ('GUK FUNGI', 'LEVELS 51-60', ['fungal cavalier', 'fungal hunter'], [('cracked mycanoid cap', 721, ''), ('flavored mushroom caps', None, 'chef item'), ('mycanoid stem', 690, '')]),
    ('HARPIES', 'LEVELS 51-60', ['bloodthirsty harpy', 'shiran harpy', 'shiran harpy queen'], [('harpy eyeball', 700, ''), ('harpy feathers', 600, ''), ('harpy foot', 1350, ''), ('harpy quill', 450, '')]),
    ('KAPPAS', 'LEVELS 51-60', ['kappa guard', 'kappa worker'], [('cloth pants', 454, ''), ('granite bracer', 543, ''), ('granite sword', 503, ''), ('small coil of rope', 488, '')]),
    ('KAPPA ELITES', 'LEVELS 51-60', ['kappa elite guard'], [('granite stone ring', 536, ''), ('granite two-handed sword', 579, '')]),
    ('KAPPA WARRIORS', 'LEVELS 51-60', ['kappa warrior'], [('granite bracer', 543, ''), ('granite sword', 503, '')]),
    ('MINERS', 'LEVELS 51-60', ['Taksehir Crystal Runner', 'Taksehir Foreman', 'Taksehir Miner'], [('canteen of purified water', None, 'Chef'), ('crystal mining pick', 675, ''), ('sweaty bandana', 701, '')]),
    ('MOOTS', 'LEVELS 51-60', ['guardian of saftemoot', 'Saftemoot'], [('elemental fire leaves', 450, ''), ('fire elemental bones', 700, ''), ('fire elemental match', 1350, ''), ('fire elemental stick', 600, '')]),
    ('RATMEN (51-60)', 'LEVELS 51-60', ['ratman'], [('moldy cheese', 1007, ''), ('ratman whiskers', 467, ''), ('ratman tooth', 405, '')]),
    ('SLAYERS', 'LEVELS 51-60', ['kobold augur', 'kobold slayer', 'Slayer Demaeog', 'Augur Rurrg'], [('fanged collar', 736, ''), ('kobold claw', 475, ''), ('wetland berries', 536, '')]),
    ('TFGS - TENACIOUS FROST GIANTS', 'LEVELS 51-60', ['tenacious frost giant', 'Arctic Spiritcaller', 'Arctic Adept'], [('frost giant eye', 325, ''), ('frost giant helm', 325, ''), ('frost giant staff', 325, ''), ('frost shawl', 567, ''), ('ice giant loincloth', 325, ''), ('ice giant shield', 622, ''), ('ice glyph', 627, '')]),
    ('TREANTS', 'LEVELS 51-60', ['Treant Defender', 'Treant Diviner', 'Treant Elder'], [('gnarled treant bark', 675, ''), ('gnarled treant branch', 701, ''), ('savory treant root', None, 'chef loot')]),
]

AKTANA_QUESTS = [(101058, 5), (101087, 5)]
AKTANA_TARGET, AKTRANA = "aktana the flamekissed", "aktrana the flamekissed"


# ---- helpers ---------------------------------------------------------------------------------

def classkey(name):
    n = (name or "").strip().lower()
    return n[:-1].strip() if n.endswith("$") else n


def norm(s):
    return re.sub(r"\s+", " ", (s or "").lower().replace("’", "'").strip())


def js(v):
    return json.dumps(v, separators=(",", ":"), sort_keys=True) if v is not None else None


def row(db, tbl, key):
    where = " AND ".join(f'"{k}" = ?' for k in key)
    cur = db.execute(f'SELECT * FROM "{tbl}" WHERE {where}', list(key.values()))
    r = cur.fetchone()
    return dict(zip([d[0] for d in cur.description], r)) if r else None


def common_chance(price):
    t = min(1.0, max(0.0, (price - 50) / 1300.0))
    return int(round(25 - 10 * t))


def is_gear(p):
    return 1 <= p["equipslot"] <= GEAR_SLOTS and p["attacktype"] != 10 and p["levelreq"] > 0


MEASURES = {"chunk", "slab", "patch", "piece", "pinch", "mound", "heap", "small", "coil"}
MASS = {"silk", "swill", "salt", "dust", "fur", "meat", "bark"}
ALT = {  # a head word with no stackable of its own -> words whose stackables lend their icon, in order
    "scarf": ["sash"], "tiara": ["earring", "ring"], "crown": ["earring", "ring"], "flute": ["horn", "totem"],
    "slab": ["stone"], "swill": ["potion", "vial", "flask"], "spinner": ["gear"], "clanger": ["gear", "cog"],
    "sprocket": ["gear", "spring"], "arrowhead": ["arrow"], "collar": ["necklace"], "berry": ["root", "herb"],
    "cheese": ["meat", "ration"], "match": ["ember", "shard"], "leaf": ["bark", "root"], "quill": ["feather"],
    "rope": ["silk", "thread"], "chip": ["bone"], "rock": ["stone"], "cap": ["mushroom", "spore"],
    "stem": ["root", "mushroom"], "branch": ["bark"], "skull": ["skull", "bone"], "eyeball": ["eye"],
    "egg": ["egg", "shell"], "medal": ["medal", "medallion"], "stick": ["stick", "branch"], "liver": ["liver", "heart"],
}


def words_of(name):
    return re.sub(r"\(.*?\)", "", name).strip().lower().split()


def singular(w):
    for suf, rep in (("ies", "y"), ("ves", "f"), ("es", "e"), ("s", "")):
        if w.endswith(suf) and len(w) > 3 and not w.endswith("ss"):
            w2 = w[: -len(suf)] + rep
            return w2[:-1] if suf == "es" and w2.endswith(("che", "she", "xe", "se")) and w2[:-1] else w2
    return w


def lead_word(name):
    """The word a name's article goes by: the one before 'of' ('bones' of the void), else the last."""
    ws = words_of(name)
    return ws[ws.index("of") - 1] if "of" in ws[1:] else ws[-1]


def head_word(name):
    """The thing a name names, for its icon: 'chunk of aviak meat' -> meat, 'bones of the void' -> bone."""
    ws = words_of(name)
    if "of" in ws[1:]:
        i = ws.index("of")
        w = ws[-1] if ws[i - 1] in MEASURES or ws[0] in MEASURES else ws[i - 1]
    else:
        w = ws[-1]
    return singular(w)


def article(name):
    if name.startswith("a "):
        return ""
    w = lead_word(name)
    if (w.endswith("s") and not w.endswith("ss")) or w in MASS:
        return "Some"
    return "An" if name[:1] in "aeiou" else "A"


class DB:
    def __init__(self, db):
        self.db = db
        self.items = {}
        self.by_name = collections.defaultdict(list)
        for r in db.execute("SELECT patternid, itemname, itemcost, equipslot, attacktype, levelreq, maxstack, itemicon, "
                            "trade, lore FROM itempattern"):
            p = dict(zip(("patternid", "itemname", "itemcost", "equipslot", "attacktype", "levelreq", "maxstack",
                          "itemicon", "trade", "lore"), r))
            self.items[p["patternid"]] = p
            self.by_name[norm(p["itemname"])].append(p["patternid"])
        self.npcs = collections.defaultdict(list)
        for i, name, lvl, world, npcid in db.execute("SELECT id, npc_name, npc_level, world, npc_id FROM npcs"):
            self.npcs[classkey(name)].append(dict(id=i, level=lvl or 1, world=world, npcid=npcid or 0))
        self.npcitem = collections.defaultdict(set)
        for npcid, pid in db.execute("SELECT npcid, patternid FROM npcitem"):
            self.npcitem[npcid].add(pid)
        self.reload_loot()
        self.pool = collections.defaultdict(set)
        for n, pid in db.execute("SELECT npc_name, patternid FROM lootpool WHERE npc_name <> '' AND weight > 0"):
            self.pool[n].add(pid)
        self.qloot = collections.defaultdict(set)
        also = collections.defaultdict(list)
        for q, s, t in db.execute("SELECT questid, step, target FROM queststeptargets"):
            also[(q, s)].append(t)
        for q, s, t, items in db.execute("SELECT s.questid, s.step, s.target, s.items FROM queststeps s "
                                         "JOIN quests q USING (questid) WHERE s.kind = 'loot' AND q.enabled = 1"):
            for name in [t] + also.get((q, s), []):
                for part in items.split(","):
                    if part.strip():
                        self.qloot[classkey(name)].add(int(part.strip().split("*")[0]))

    def reload_loot(self):
        self.npcloot = collections.defaultdict(set)
        for n, t in self.db.execute("SELECT npc_name, tablename FROM npcloot"):
            self.npcloot[n].add(t)
        self.tables = collections.defaultdict(set)
        for t, pid in self.db.execute("SELECT tablename, patternid FROM loottables WHERE weight > 0"):
            self.tables[t].add(pid)

    def item(self, name):
        """An existing record for the guide's name: exact (any case), by ITEM_ALIAS, or the same words but for
        articles, punctuation and a plural."""
        n = norm(name)
        for cand in (n, ITEM_ALIAS.get(n)):
            if cand and cand in self.by_name:
                return min(self.by_name[cand])
        return None

    def mobs(self, names):
        """The guide's mob names -> [(guide name, our key or None)]."""
        out = []
        for g in names:
            k = classkey(g)
            if k == "ratman":
                out += [(g, r) for r in RATMEN_5160 if r in self.npcs]
                continue
            for cand in [k] + MOB_ALIAS.get(k, []):
                if cand in self.npcs:
                    out.append((g, cand))
                    break
            else:
                out.append((g, None))
        return out

    def drops(self, binding, rows, pid):
        """binding (a name or a row key) already drops pid: its npcitem, a table bound to it (or to its name, or to a
        row of it), its own lootpool rows, a quest loot step on it."""
        names = {binding} | {f"#{r['id']}" for r in rows}
        if not binding.startswith("#"):
            names.add(binding)
        for n in names:
            if any(pid in self.tables.get(t, ()) for t in self.npcloot.get(n, ())):
                return True
        key = binding if not binding.startswith("#") else None
        if key and (pid in self.pool.get(key, ()) or pid in self.qloot.get(key, ())):
            return True
        return any(pid in self.npcitem.get(r["npcid"], ()) for r in rows)


# ---- planning ----------------------------------------------------------------------------------

class Plan:
    def __init__(self):
        self.made = []        # dict(pid, name, cost, icon, icon_from, desc, camp, price)
        self.tables = []      # (table, pid, camp)
        self.binds = []       # (binding, table, chance, camp, pid, note)
        self.left = []        # (camp, item, why)
        self.gear = []        # (camp, item, price, why)
        self.ambiguous = []   # (camp, item, why)
        self.already = []     # (camp, item, pid, where)
        self.targets = []     # (questid, step, target)
        self.saw_table = set()


def camp_bindings(d, guide_mobs, camp):
    """The npcloot names a camp's mobs are bound by, each with its rows: [(binding, rows, key)]."""
    resolved = d.mobs(guide_mobs)
    rows_all = [r for _, k in resolved if k for r in d.npcs[k]]
    if not rows_all:
        return [], resolved, None
    world = collections.Counter(r["world"] for r in rows_all).most_common(1)[0][0]
    out = []
    for _, k in resolved:
        if not k:
            continue
        rows = d.npcs[k]
        if all(r["world"] == world for r in rows):
            out.append((k, rows, k))
        else:
            out += [(f"#{r['id']}", [r], k) for r in rows if r["world"] == world]
    return out, resolved, world


def table_name(name):
    return f"guide1121 {norm(name)}"


def wire(d, plan, camp, guide_mobs, pid, chance, note):
    binds, resolved, world = camp_bindings(d, guide_mobs, camp)
    p = d.items[pid]
    t = table_name(p["itemname"])
    if not binds:
        plan.left.append((camp, p["itemname"], "none of the camp's mobs exist: " + ", ".join(guide_mobs)))
        return
    wired = 0
    for b, rows, key in binds:
        if is_gear(p) and p["levelreq"] > min(r["level"] for r in rows) + REACH:
            plan.left.append((camp, p["itemname"], f"{b}: level {min(r['level'] for r in rows)}, the record's "
                                                   f"levelreq {p['levelreq']} is more than {REACH} over it: never drops"))
            continue
        if d.drops(b, rows, pid) or (b, t) in [(x[0], x[1]) for x in plan.binds]:
            plan.already.append((camp, p["itemname"], pid, b))
            continue
        plan.binds.append((b, t, chance, camp, pid, note))
        wired += 1
    if wired and t not in d.tables and t not in plan.saw_table:
        plan.saw_table.add(t)
        plan.tables.append((t, pid, camp))
    elif wired and t in d.tables and pid not in d.tables[t]:
        plan.tables.append((t, pid, camp))


def icon_for(d, name):
    """An icon from an existing stackable of the same kind: the name's head word, else ALT's words for it, then the
    closest name among them; the source is printed."""
    stack = [p for p in d.items.values() if p["equipslot"] == -1 and p["maxstack"] >= 99 and p["itemicon"]]
    w = head_word(name)
    for word in [w] + ALT.get(w, []):
        same = [p for p in stack if re.search(r"\b" + re.escape(word) + r"(s|es)?\b", p["itemname"].lower())]
        if same:
            break
    else:
        same, word = [], None
    pool = same or stack
    best = max(pool, key=lambda p: (difflib.SequenceMatcher(None, norm(name), norm(p["itemname"])).ratio(),
                                    -p["patternid"]))
    how = f"by '{word}'" if same else "[no same kind: closest name]"
    return best["itemicon"], f"{best['itemname']} ({best['patternid']}) {how}"


def plan_all(d, archon):
    plan = Plan()
    used = set(d.items)
    next_id = [FIRST_ID]
    saftemoot_there = [i for i in SAFTEMOOT_IDS if i in d.items]

    def new_id():
        while next_id[0] in used:
            next_id[0] += 1
        used.add(next_id[0])
        return next_id[0]

    # 1a. rares and named
    for pid, name, camp, mobs, chance in RARES + NAMED + ([ARCHON] if archon else []):
        p = d.items.get(pid)
        if not p or norm(p["itemname"]) != name:
            plan.left.append((camp, name, f"record {pid} is {p['itemname'] if p else 'missing'!r}, not {name!r}: refused"))
            continue
        kind = "rare" if chance < 100 else "always"
        wire(d, plan, camp, mobs, pid, chance, f"{kind} ({chance}%)")
    if not archon:
        plan.left.append(("PERMAFROST/AVATAR OF FEAR", "archon's heart (1010)",
                          "not wired (--archon-1010 to wire): the Avatar of Fear chain (quest 101282 step 1) takes "
                          "701422 'Archon Heart', which the Tae Ew Archon already drops at 100%"))

    # 1b + 2. the camps' items
    for camp, band, mobs, items in CAMPS:
        priced = [pr for _, pr, _ in items if pr]
        median = statistics.median(priced) if priced else None
        for name, price, note in items:
            n = norm(name)
            eff = price if price else (median if median else None)
            if n in AMBIGUOUS:
                plan.ambiguous.append((camp, name, AMBIGUOUS[n]))
                continue
            pid = d.item(name)
            if pid is None and n in GEAR:
                plan.gear.append((camp, name, price, GEAR[n]))
                continue
            if not mobs:
                plan.left.append((camp, name, "the guide names no mobs for this camp" +
                                  (f" (record {pid} exists)" if pid else "")))
                continue
            chance = common_chance(eff if eff else 50)
            if note in QUEST_NOTES:
                plan.left.append((camp, name, "a quest item: left to its quest" +
                                  (f" (record {pid} exists)" if pid else ", no record (not made)")))
                continue
            if pid is not None:
                wire(d, plan, camp, mobs, pid, chance, f"common ({chance}%)")
                continue
            if camp == "MOOTS" and saftemoot_there:
                plan.left.append((camp, name, f"Saftemoot's records {saftemoot_there} exist under other names: "
                                              "match them by hand, not made"))
                continue
            if any(m["name"] == n for m in plan.made):
                m = next(m for m in plan.made if m["name"] == n)
                wire_new(d, plan, camp, mobs, m, chance)
                continue
            cost = round(eff * 0.32) if eff else 50
            icon, src = icon_for(d, name)
            mob0 = desc_mob(d, mobs)
            a = article(n)
            thing = f"{a} {n}" if a else n[0].upper() + n[1:]
            desc = f"{thing} taken from {'an' if mob0[:1] in 'aeiou' else 'a'} {mob0}."
            m = dict(pid=new_id(), name=n, cost=cost, icon=icon, icon_from=src, desc=desc, camp=camp, price=price,
                     median=None if price else median)
            plan.made.append(m)
            wire_new(d, plan, camp, mobs, m, chance)

    # 3. Aktana's soul
    if AKTRANA in d.npcs:
        for q, s in AKTANA_QUESTS:
            st = row(d.db, "queststeps", {"questid": q, "step": s})
            if not st or st["kind"] != "loot" or classkey(st["target"]) != AKTANA_TARGET:
                plan.left.append(("AKTANA", f"quest {q} step {s}", f"not as expected ({st and (st['kind'], st['target'])}): refused"))
                continue
            have = {classkey(t) for (t,) in d.db.execute("SELECT target FROM queststeptargets WHERE questid = ? AND step = ?", (q, s))}
            if AKTRANA not in have:
                plan.targets.append((q, s, "Aktrana the Flamekissed"))
    return plan


def desc_mob(d, mobs):
    """The mob a made item's description names: the guide's first common (lowercase) name, else ours with the most rows."""
    for m in mobs:
        if m[:1].islower():
            k = next((k for _, k in d.mobs([m]) if k), None)
            return k if k and m != "ratman" else m.lower()
    keys = [k for _, k in d.mobs(mobs) if k]
    return max(keys, key=lambda k: len(d.npcs[k])) if keys else mobs[0].lower()


def wire_new(d, plan, camp, mobs, m, chance):
    """wire() for a record this run makes (not in the database yet)."""
    binds, resolved, world = camp_bindings(d, mobs, camp)
    t = table_name(m["name"])
    if not binds:
        plan.left.append((camp, m["name"], "none of the camp's mobs exist"))
        return
    if t not in plan.saw_table:
        plan.saw_table.add(t)
        plan.tables.append((t, m["pid"], camp))
    for b, rows, key in binds:
        if (b, t) not in [(x[0], x[1]) for x in plan.binds]:
            plan.binds.append((b, t, chance, camp, m["pid"], f"common ({chance}%)"))


# ---- report and apply --------------------------------------------------------------------------------

def report(d, plan):
    name = lambda pid: d.items[pid]["itemname"] if pid in d.items else next(m["name"] for m in plan.made if m["pid"] == pid)
    print(f"PART 2: {len(plan.made)} records to make")
    for m in plan.made:
        pr = m["price"] if m["price"] else f"none (camp median {m['median']})"
        print(f"  {m['pid']} {m['name']!r} cost {m['cost']} (guide {pr}) icon {m['icon']} from {m['icon_from']} | {m['camp']}")
        print(f"      desc: {m['desc']}")
    print(f"PART 1+2: {len(plan.tables)} loot tables, {len(plan.binds)} npcloot bindings")
    by = collections.defaultdict(list)
    for b, t, ch, camp, pid, note in plan.binds:
        by[(camp, pid, ch, note)].append(b)
    for (camp, pid, ch, note), bs in by.items():
        made = " (made here)" if pid not in d.items else ""
        lst = ", ".join(bs) if len(bs) <= 6 else ", ".join(bs[:6]) + f" ... ({len(bs)} in all)"
        print(f"  [{camp}] {pid} {name(pid)!r}{made}: {note} from {lst}")
    if plan.already:
        print(f"ALREADY DROPPING (left as they are): {len(plan.already)}")
        seen = collections.defaultdict(list)
        for camp, it, pid, b in plan.already:
            seen[(camp, it, pid)].append(b)
        for (camp, it, pid), bs in seen.items():
            print(f"  [{camp}] {pid} {it!r}: {', '.join(bs[:6])}{' ...' if len(bs) > 6 else ''}")
    print(f"GEAR, not made: {len(plan.gear)}")
    for camp, it, pr, why in plan.gear:
        print(f"  [{camp}] {it!r} ({pr}): {why}")
    print(f"AMBIGUOUS, not made: {len(plan.ambiguous)}")
    for camp, it, why in plan.ambiguous:
        print(f"  [{camp}] {it!r}: {why}")
    print(f"LEFT OUT: {len(plan.left)}")
    for camp, it, why in plan.left:
        print(f"  [{camp}] {it}: {why}")
    print(f"PART 3: {len(plan.targets)} queststeptargets rows")
    for q, s, t in plan.targets:
        print(f"  quest {q} step {s}: + other creature {t!r}")


def apply(d, plan):
    db = d.db
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    n = [0]

    db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (seq INTEGER PRIMARY KEY, tbl TEXT NOT NULL, rowkey TEXT NOT NULL, "
               "before TEXT, after TEXT, at TEXT NOT NULL)")

    def audit(tbl, key, before, camp):
        after = row(db, tbl, key)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, at) VALUES (?, ?, ?, ?, ?)",
                   (tbl, js(key), js(before), js(after), at))
        action = "insert" if before is None else "delete" if after is None else "update"
        db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                   "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                   (at, ACCOUNT, WHO, VIA, action, tbl, js(key), js(before), js(after), detail(camp)))
        n[0] += 1

    tmpl = row(db, "itempattern", {"patternid": TEMPLATE})
    if not tmpl or tmpl["maxstack"] != 999 or tmpl["equipslot"] != -1 or tmpl["trade"] != 0 or tmpl["lore"] != 0:
        raise SystemExit(f"template {TEMPLATE} is not as expected: {tmpl and {k: tmpl[k] for k in ('itemname', 'maxstack', 'equipslot', 'trade', 'lore')}}")
    for m in plan.made:
        if row(db, "itempattern", {"patternid": m["pid"]}):
            raise SystemExit(f"patternid {m['pid']} taken")
        r = dict(tmpl, patternid=m["pid"], itemname=m["name"], itemdesc=m["desc"], itemcost=m["cost"], itemicon=m["icon"])
        cols = ", ".join(f'"{c}"' for c in r)
        db.execute(f"INSERT INTO itempattern ({cols}) VALUES ({', '.join('?' * len(r))})", list(r.values()))
        audit("itempattern", {"patternid": m["pid"]}, None, m["camp"])
    for t, pid, camp in plan.tables:
        cur = db.execute("INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel, source) "
                         "VALUES (?, ?, 1, ?, ?, ?)", (t, pid, LEVELS[0], LEVELS[1], SRC))
        audit("loottables", {"id": cur.lastrowid}, None, camp)
    for b, t, ch, camp, pid, note in plan.binds:
        db.execute("INSERT INTO npcloot (npc_name, tablename, chance, rolls, note, source) VALUES (?, ?, ?, 1, ?, ?)",
                   (b, t, ch, f"MasterVG782's GameFAQs guide, camp {camp}: {note} (the owner 2026-11-21)", SRC))
        audit("npcloot", {"npc_name": b, "tablename": t}, None, camp)
    for q, s, t in plan.targets:
        db.execute("INSERT INTO queststeptargets (questid, step, target) VALUES (?, ?, ?)", (q, s, t))
        audit("queststeptargets", {"questid": q, "step": s, "target": t}, None,
              "Aktana the Flamekissed: Live's 'Aktrana the Flamekissed' row drops the soul")
    return n[0]


def undo(db):
    """Deletes exactly the rows the log says this tool inserted (newest first), each with its gm_audit line; a row
    changed since is left and said; then drops the log."""
    has = db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (LOG,)).fetchone()
    if not has:
        print("nothing to undo: no log")
        return 0
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    n = left = 0
    db.execute("BEGIN IMMEDIATE")
    try:
        for tbl, key, after in db.execute(f"SELECT tbl, rowkey, after FROM {LOG} ORDER BY seq DESC").fetchall():
            k = json.loads(key)
            now = row(db, tbl, k)
            if now is None:
                continue
            if js(now) != after:
                print(f"  left (changed since): {tbl} {key}")
                left += 1
                continue
            where = " AND ".join(f'"{c}" = ?' for c in k)
            db.execute(f'DELETE FROM "{tbl}" WHERE {where}', list(k.values()))
            db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                       "VALUES (?, ?, ?, ?, '', 'delete', ?, ?, ?, NULL, ?)",
                       (at, ACCOUNT, WHO, VIA, tbl, key, after, "undone: " + detail("all")))
            n += 1
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"undone: {n} rows deleted, {left} left")
    return 0


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true", help="delete exactly the rows the log says were written")
    ap.add_argument("--archon-1010", action="store_true", help="also wire record 1010 archon's heart to the Tae Ew Archon")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db)
    db.execute("PRAGMA busy_timeout = 5000")
    db.isolation_level = None
    if a.undo:
        return undo(db)
    d = DB(db)
    plan = plan_all(d, a.archon_1010)
    report(d, plan)
    if not a.apply:
        print("\ndry run: nothing written (--apply to write)")
        return 0
    if not (plan.made or plan.tables or plan.binds or plan.targets):
        print("\nnothing to write: all in place")
        return 0
    try:
        db.execute("BEGIN IMMEDIATE")
        n = apply(d, plan)
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"\napplied: {len(plan.made)} records, {len(plan.tables)} tables, {len(plan.binds)} bindings, "
          f"{len(plan.targets)} step targets; {n} gm_audit lines")
    return 0


if __name__ == "__main__":
    sys.exit(main())
