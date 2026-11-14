#!/usr/bin/env python3
"""Master crafting as the forum's "Master Craft Guide Chart" (eqoa.forumotion.com t392, Ronson, 31 July 2010, sent by
the owner 2026-11-14): at levels 30, 40, 50 and 60 a pattern, its raw material and a class's modifier make that class's
master-crafted armor.

The chart (its HTML table, reconstructed row by row; CHART below), one row a class:
  plate   Bronzed / Sultar Plate / Complex Plate / Elaborate Plate; Cobalt Spring / Nickel Ring / Vellium Ore /
          Platinum Ore Chunk; Paladin, Warrior, Shadowknight, Cleric
  chain   Leaf Chain / Raven Chain / Silver Fitted / Intricate Chain; the same metals; Bard, Ranger, Rogue, Shaman
  leather Pigskin / Shaded Leather / Trimmed Leather / Superior; Intact Fur Hide / Double Layered Hide / Flakey Hide /
          Brown Split Hide; Druid, Alchemist
  cloth   Tweed / Vineweb Silk / Red Lined / Embroidered; Silky Thread / Vineweb Silk Thread / Blood Drenched Thread /
          Silklined Swatch; Monk, Necromancer, Magician, Wizard, Enchanter
and the merchants: at Arcadin and at SoM a Master Craftsman a level sells plate and chain, another cloth and leather.
Its spellings are kept but two the other sources correct (Platinum Ore "Chunck", "Rythmic" Scrolled Rune); its Rogue
level 30 cell is "No Gem": the pattern and the metal alone.

Before this the server had no master crafting: no recipe, none of the chart's parts as an item, the Master Craftsmen
selling nothing. What the chart does not say is ours:
  - the set each class's recipe makes is its name in QuestMasters' "Master Crafted Armor" (id527, 2006) and EQOA-Data's
    Information/mastercraft.json (Jadiction), seven pieces (tunic, helm, leggings, gloves, boots, belt, bracers); the
    four sets neither names (Rogue 30, Druid, Enchanter and Necromancer 50) take their pattern's or modifier's words;
  - one pattern a piece ("Bronzed Tunic Pattern"): a recipe is an exact set of items, as the armorsmiths' molds are a
    piece each;
  - Armor Crafting makes all four kinds (Tailoring has no kits: it is not live), on the Recipe Book's Special tab;
    trade level 7, 8, 9, 10 for the chart's 30, 40, 50, 60 (a Craftsman, Artisan, Master, Grandmaster hammer);
    rarity uncommon at 30 and 40, rare at 50 and 60 (the Rogue's no-gem 30 common); difficulty and XP by
    tools/seed_tradeskills.py's rules; an Experiment discovers each;
  - a product is this server's record of its name at its level and slot where there is one (the level 60 pieces the
    loot already had: champion's, fervored, soulripper's ...), kept as it is; else a new record with EQOA-Data's stats
    for the piece, and for a piece it has none for, the class's level 60 piece scaled by the set's known pieces
    against theirs (or the level's middle ratio over every class); only the class may wear a new one;
  - obtainable: each Master Craftsman sells his level's patterns and raw materials (a missing one's wares go to his
    level's plate and chain craftsman there); the modifiers drop from creatures (the loot pool "modifiers", weight 1,
    from five levels under their set); and every new part stands on the auction house as ilovetesters' perpetual lot
    at tools/ah_materials_20261114.py's price (100 tunar common, 500 rare, raised to the item's value).

A report by default (the chart against ours, row by row); --apply writes in one transaction, a gm_audit line a row,
logged in master_craft_1114_log; a second --apply does nothing; --undo puts back every row still as written (the
AUTOINCREMENT counters too). Recipes and items are read as they are used (craftrecipes within 30 s), the lots per
search, the loot per corpse; the Master Craftsmen's merchant flag (npcs) is read at the server's start: RESTART for
the shops (the recipes, drops and lots work without one).

    python3 -E -s tools/master_craft_20261114.py --db eqoa.db [--apply | --undo]
"""
import json
import os
import statistics
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402
import ah_materials_20261114 as ah  # noqa: E402

LOG = "master_craft_1114_log"
DETAIL = "Master crafting as the 2010 Master Craft Guide Chart: tools/master_craft_20261114.py"
SOURCE = "master-craft-1114"
ARMOR, SPECIAL = 1, 10                       # Armor Crafting; its component list's Special tab
LEVELS = (30, 40, 50, 60)
TRADE_LEVEL = {30: 7, 40: 8, 50: 9, 60: 10}
RARITY = {30: 1, 40: 1, 50: 2, 60: 2}
MOD_PRICE = [20, 100, 400, 1500]             # tools/seed_tradeskills.py's, by rarity
ANY = 32767
CLASS_ID = {"Warrior": 0, "Ranger": 1, "Paladin": 2, "Shadowknight": 3, "Monk": 4, "Bard": 5, "Rogue": 6, "Druid": 7,
            "Shaman": 8, "Cleric": 9, "Magician": 10, "Necromancer": 11, "Enchanter": 12, "Wizard": 13, "Alchemist": 14}

# The chart, as its table has it (spellings kept, but CHART_FIX's).
CHART = {
    "plate": (["Bronzed", "Sultar Plate", "Complex Plate", "Elaborate Plate"],
              ["Cobalt Spring", "Nickel Ring", "Vellium Ore", "Platinum Ore Chunck"],
              {"Paladin": ["Gilded Rocks", "Elemental Rune", "Tempest Rune", "Nickel Rod"],
               "Warrior": ["Kyanite Rock", "Rock of Might", "Shattered Lavarock", "Bronze Alloy"],
               "Shadowknight": ["Phyllite Stud", "Midnight Black Stud", "Darkstone Stud", "Magnesium Alloy"],
               "Cleric": ["Accurate Rune", "Water Worn Rock", "Earth Shattered Rock", "Brazen Alloy"]}),
    "chain": (["Leaf Chain", "Raven Chain", "Silver Fitted", "Intricate Chain"],
              ["Cobalt Spring", "Nickel Ring", "Vellium Ore", "Platinum Ore Chunck"],
              {"Bard": ["Scoured Rune", "Beaten Rune", "Rythmic Scrolled Rune", "Brass Ring"],
               "Ranger": ["Fire Branded Gem", "Eagle Eye Gem", "Hawkeye Gem", "Copper Nugget"],
               "Rogue": ["No Gem", "Twisted Metal Spring", "Heartsblood Ruby", "Slippery Sprocket"],
               "Shaman": ["Acidic Silver Stud", "Rune of Lore", "Rune of Fate", "Titanium Spring"]}),
    "leather": (["Pigskin", "Shaded Leather", "Trimmed Leather", "Superior"],
                ["Intact Fur Hide", "Double Layered Hide", "Flakey Hide", "Brown Split Hide"],
                {"Druid": ["Leaf Encrusted Gem", "Black Star Gem", "Creeping Beryl", "Dual Slot Studs"],
                 "Alchemist": ["Transformed Rune", "Blood Dried Rune", "Dual Bonded Rune", "Gold Melded Studs"]}),
    "cloth": (["Tweed", "Vineweb Silk", "Red Lined", "Embroidered"],
              ["Silky Thread", "Vineweb Silk Thread", "Blood Drenched Thread", "Silklined Swatch"],
              {"Monk": ["Black Wooden Stud", "Rage Fist Stud", "Shadowfist Stud", "Lana Thread"],
               "Necromancer": ["Stellar Green Gem", "Wind Driven Gem", "Gale Force Rock", "Red Ribbon Thread"],
               "Magician": ["Conjured Charm", "Skull Embossed Charm", "Bonewoven Charm", "Naro Thread"],
               "Wizard": ["Golden Ember Rock", "Fire Engulfed Gem", "Frozen Ice Gem", "Crystalline Thread"],
               "Enchanter": ["Starlit Gem", "Lily Encrusted Gem", "Starfire Gem", "Rowan Thread"]}),
}
CHART_FIX = {"Platinum Ore Chunck": "Platinum Ore Chunk", "Rythmic Scrolled Rune": "Rhythmic Scrolled Rune"}
NO_GEM = "No Gem"

# The sellers (the chart's "Master Craft Merchants"): place -> level -> (plate and chain, cloth and leather).
SELLERS = {"Arcadin": {30: ("Farns", "Rhodes"), 40: ("Truen", "Prat"), 50: ("Sams", "Dunn"), 60: ("Gibbs", "Kline")},
           "SoM": {30: ("Yarney", "Glarn"), 40: ("Yeatts", "Zeller"), 50: ("Johns", "Watts"), 60: ("Fritts", "Higgy")}}
SELLER_WORLD = 2

# The sets: QuestMasters id527 / EQOA-Data's names; None: neither names it (SET_OURS).
SETS = {
    "Paladin": ["Gilded", "Scarlet", "Valorium", "Champion's"],
    "Warrior": ["Kyanite Polished", "Mighty Plate", "Indicolite", "Soldier's"],
    "Shadowknight": ["Phyllite", "Coal Blackened", "Umbral", "Soulripper's"],
    "Cleric": ["Zealous", "Water Worn", "Ethereal Mist", "Fervored"],
    "Bard": ["Lambent", "Beaten Chain", "Imbrued", "{piece} of Melody"],
    "Ranger": ["Fire Branded", "Magnified", "Thorny Vine", "Silverwood"],
    "Rogue": [None, "Crimped Chain", "Woven", "Mischievous"],
    "Shaman": ["Acidic", "Dust Filled", "Rune Etched", "Souldivine"],
    "Druid": ["Leaf Encrusted", "Blackened Leather", None, "Harmonic"],
    "Alchemist": ["Alacrium", "Blood Dried", "Blood Tanned", "Transmuted"],
    "Monk": ["Wu's Fighting", "Fisted Rage", "Obsidian", "Solitary"],
    "Necromancer": ["Shadowbound", "Wind Worn", None, "Bonecrusher"],
    "Magician": ["Earthen", "Embossed", "Apothic", "Elementally Charged"],
    "Wizard": ["Gold Threaded", "Fire Etched", "Carmine", "Charmed"],
    "Enchanter": ["Incandescent", "Crimson", None, "Illusionary"],
}
SET_OURS = {("Rogue", 30): "Leaf Chain", ("Druid", 50): "Creeping Beryl", ("Necromancer", 50): "Gale Force",
            ("Enchanter", 50): "Starfire"}

# The pieces: (word, wear slot, value at level 60: the level 60 records' own).
PIECES = [("tunic", 5, 6553), ("helm", 1, 3662), ("leggings", 10, 5393), ("gloves", 19, 3662), ("boots", 11, 4292),
          ("belt", 9, 5804), ("bracers", 7, 4375)]
MODEL = {"plate": 4, "chain": 3, "leather": 2, "cloth": 1}   # CLIENT's armor sets; a belt has none (0)

# Icons: this server's commonest records of the kind (a modifier's by its last word).
ICON_PATTERN = -607649328                     # the recipe scrolls'
ICON_RAW = {"plate": 1577500738, "chain": 1577500738, "leather": -1063213915, "cloth": 441928322}
ICON_MOD = {"rune": 1810179422, "rock": -649479524, "rocks": -649479524, "lavarock": -649479524,
            "stud": -1836445075, "studs": -1836445075, "gem": 140265582, "ruby": 140265582, "beryl": 140265582,
            "charm": -1949681806, "alloy": 1246639590, "thread": 441928322, "ring": 1577500738,
            "spring": 1577500738, "nugget": 1577500738, "rod": 1577500738, "sprocket": 1577500738}
STAND_IN_ICONS = (848266163, -570739083)
COLS = {"STR": "str", "STA": "sta", "AGI": "agi", "WIS": "wis", "DEX": "dex", "CHA": "cha", "INT": "intelligence",
        "HP": "HPMAX", "POW": "POWMAX", "AC": "AC", "PR": "PR", "DR": "DR", "FR": "FR", "CR": "CR", "LR": "LR", "AR": "AR"}
STAT_COLS = ["str", "sta", "agi", "wis", "dex", "cha", "intelligence", "HPMAX", "POWMAX", "PoT", "HoT", "AC", "PR",
             "DR", "FR", "CR", "LR", "AR"]
AC_FLOOR = 0.3   # a known piece's AC (not a belt's) under this share of its estimate is a slip: the estimate's is used
LOOT_POOL, LOOT_WEIGHT, LOOT_UNDER = "modifiers", 1, 5

# EQOA-Data Information/mastercraft.json's stats (the pieces it has), by class, level and piece.
STATS = {
    "Alchemist": {
        30: {"tunic": "WIS10 INT20 AC10 FR18", "helm": "WIS14 INT16 AC29 PR9", "belt": "WIS14 INT16 AC7 CR9"},
        40: {"tunic": "WIS15 INT15 AC188 PR6 DR6", "helm": "WIS16 INT22 HP60 AC37", "leggings": "WIS18 INT18 AC92 PR6 DR6", "gloves": "WIS15 INT15 AC119 FR6 LR6", "belt": "WIS16 INT22 POW60 AC9"},
        50: {"tunic": "WIS20 INT16 AC233 LR20", "belt": "WIS24 INT22 POW50 AC11"},
        60: {"tunic": "WIS15 INT15 POW165 AC198", "helm": "WIS24 INT24 HP40 AC53 AR28", "leggings": "WIS16 INT16 POW80 AC291", "gloves": "WIS15 INT15 POW165 AC99", "boots": "WIS19 INT19 HP118 AC66 AR26", "belt": "STA15 WIS40 HP100 AC33 PR15", "bracers": "WIS27 INT27 AC99 DR15"},
    },
    "Bard": {
        30: {"tunic": "STA10 AGI12 HP30 AC62 FR10 LR10"},
        40: {"tunic": "STA16 AGI16 POW50 AC207 AR18", "helm": "STA16 AGI16 POW50 AC55 AR18", "leggings": "STA16 AGI24 AC158 CR6 AR6", "gloves": "STA24 AGI16 AC124 PR6 DR6", "boots": "STA16 AGI20 POW60 AC69 CR10", "belt": "STA20 AGI16 POW60 AC14 CR10"},
        50: {"tunic": "STA18 AGI22 AC252 FR13 LR16", "helm": "STA22 AGI18 AC67 AR16 PR13"},
        60: {"tunic": "STA20 AGI20 HP105 AC297 AR28", "helm": "STA22 AGI22 HP55 AC79 PR15 DR15", "leggings": "STA19 AGI19 POW20 AC198 FR20 LR20", "gloves": "STA20 AGI20 HP135 AC149 AR10", "boots": "STA19 AGI19 HP20 AC99 PR20 DR20", "belt": "STA19 AGI19 POW20 CR20 AR20", "bracers": "STA25 AGI25 HP85 AC149"},
    },
    "Cleric": {
        30: {"tunic": "WIS10 CHA10 AC216 LR10 PR13", "helm": "WIS13 CHA12 AC101", "leggings": "WIS13 CHA10 AC144 CR13 PR10"},
        40: {"tunic": "WIS22 CHA15 POW55 AC306", "helm": "WIS22 CHA15 POW55 AC104", "boots": "WIS16 CHA20 POW65 AC92"},
        50: {"tunic": "WIS20 CHA20 AC396", "leggings": "WIS26 CHA20 AC270"},
        60: {"tunic": "WIS20 CHA20 HP100 AC396 DR30", "helm": "WIS20 CHA20 HP100 AC106 FR30", "leggings": "WIS26 CHA26 AC264 CR10 AR10", "gloves": "WIS26 CHA26 AC198 CR20", "boots": "WIS26 CHA26 AC132 PR20", "belt": "WIS20 CHA20 POW100 AC26 AR30", "bracers": "WIS26 CHA26 AC198 AR20"},
    },
    "Druid": {
        30: {"tunic": "DEX14 WIS11 AC153", "helm": "DEX11 WIS14 AC74", "leggings": "DEX12 WIS10 POW30 AC72 FR10 AR10", "gloves": "DEX13 WIS10 POW30 AC54 AR13 DR10", "boots": "DEX10 WIS13 AC36 FR10 DR13", "belt": "DEX13 WIS10 AC7 CR13 PR10", "bracers": "DEX14 WIS11 AC99"},
        40: {"tunic": "DEX18 WIS18 AC138 CR6 AR8", "leggings": "DEX18 WIS18 AC92 AR8 PR6", "gloves": "DEX19 WIS20 AC69", "boots": "DEX20 WIS19 AC46", "belt": "DEX15 WIS15 AC59 PR6 DR6"},
        50: {"tunic": "DEX16 WIS22 AC226 FR10 CR10", "gloves": "DEX12 WIS24 POW60 AC84 FR35", "belt": "DEX18 WIS20 POW20 AC11 CR14 AR15"},
        60: {"tunic": "DEX24 WIS24 POW100 AC198", "helm": "DEX24 WIS24 POW40 AC53 LR28", "leggings": "DEX24 WIS24 HP40 AC132 DR28", "gloves": "DEX24 WIS24 POW40 AC99 PR28", "boots": "DEX19 WIS19 HP118 AC66 AR26", "belt": "DEX19 WIS19 POW118 AC13 LR26", "bracers": "DEX19 WIS19 POW118 AC99 CR26"},
    },
    "Enchanter": {
        30: {"tunic": "INT12 CHA13 AC97", "gloves": "INT12 CHA18 POW50 AC27", "belt": "INT18 CHA12 POW50 AC4"},
        40: {"tunic": "INT20 CHA16 POW65 AC69", "leggings": "INT16 CHA20 POW65 AC46", "belt": "INT18 CHA18 AC8 PR6 DR6"},
        50: {"tunic": "INT22 CHA22 HP15 POW15 AC119", "leggings": "INT26 CHA20 POW70 AC56", "boots": "INT20 CHA26 POW70 AC28"},
        60: {"tunic": "INT27 CHA27 AC99 FR30 AR15", "helm": "INT15 CHA15 HP100 AC104 PR10 DR10", "leggings": "INT15 CHA15 POW95 AC144 CR10 AR10", "gloves": "INT15 CHA15 POW95 AC128 FR10 LR10", "boots": "INT20 CHA20 HP95 AC33 LR30", "belt": "INT20 CHA20 POW95 AC7 FR30", "bracers": "INT20 CHA20 HP95 AC50 AR30"},
    },
    "Magician": {
        30: {"tunic": "AGI12 INT15 HP55 AC53", "helm": "AGI12 INT13 AC57", "leggings": "AGI12 INT15 POW55 AC35", "belt": "AGI10 INT14 AC4 LR9 CR15"},
        40: {"tunic": "AGI15 INT15 AC99 CR10 AR10", "leggings": "AGI15 INT15 AC76 FR10 LR10", "boots": "AGI20 INT16 POW60 AC23 AR10"},
        50: {"tunic": "AGI24 INT16 HP65 POW35 AC84 DR16"},
        60: {"tunic": "AGI27 INT27 AC99 AR15", "helm": "AGI16 INT14 POW110 AC116", "leggings": "AGI15 INT15 POW95 AC144 AR10 PR10", "gloves": "AGI18 INT18 HP150 AC50", "boots": "AGI14 INT14 POW15 AC146", "belt": "AGI18 INT18 POW150 AC7", "bracers": "AGI14 INT14 HP15 AC163"},
    },
    "Monk": {
        30: {"tunic": "STA13 DEX12 AC97", "leggings": "STA12 DEX13 AC79", "belt": "STA13 DEX10 AC4 LR13 CR10"},
        40: {"tunic": "STA16 DEX16 POW50 AC119", "helm": "STA16 DEX16 POW50 AC18 AR18", "leggings": "STA17 DEX16 HP50 AC96", "boots": "STA16 DEX18 AC23 AR7 PR7 DR6", "belt": "STA18 DEX16 AC5 FR7 LR7 CR6"},
        50: {"tunic": "STA26 DEX23 AC84", "leggings": "STA25 DEX20 HP40 AC54 FR20", "belt": "STA30 DEX20 AC6 PR16"},
        60: {"tunic": "STA20 DEX20 AC99 PR20 DR19", "helm": "STA23 DEX25 HP35 AC91", "leggings": "STA18 DEX18 POW105 AC66 CR15 AR15", "gloves": "STA20 DEX20 AC99 CR19 PR20", "boots": "STA27 DEX28 AC58", "belt": "STA28 DEX27 AC32", "bracers": "STA25 DEX23 AC115"},
    },
    "Necromancer": {
        30: {"tunic": "STA12 INT13 AC97", "belt": "STA14 INT12 POW60 AC4"},
        40: {"tunic": "STA16 INT17 AC119", "leggings": "STA14 INT14 AC46 FR30", "gloves": "STA17 INT16 AC85", "boots": "STA14 INT14 AC23 PR30", "belt": "STA14 INT14 AC5 AR30"},
        50: {"tunic": "STA16 INT16 AC166", "leggings": "STA16 INT16 AC138"},
        60: {"tunic": "STA27 INT27 AC129", "helm": "STA18 INT18 HP105 FR15", "leggings": "STA18 INT18 HP105 AC66 CR15 PR15", "gloves": "STA18 INT18 POW40 AC115 PR15 DR15", "boots": "STA18 INT18 POW105 AC33 AR15 DR15", "belt": "STA18 INT18 HP40 AC72 FR15 LR15", "bracers": "STA18 INT18 HP40 AC115 CR15 AR15"},
    },
    "Paladin": {
        30: {"tunic": "STR20 STA10 AC216 AR18", "helm": "STR20 STA10 AC58 CR18", "leggings": "STR10 STA20 AC144 LR18"},
        40: {"tunic": "STR15 STA22 POW55 AC306", "helm": "STR22 STA15 POW55 AC104", "leggings": "STR16 STA24 AC204 FR6 LR6", "gloves": "STR14 STA14 AC138 FR30"},
        50: {"tunic": "STR20 STA24 AC336 CR14 PR10", "leggings": "STR20 STA24 AC224 AR14 DR10", "gloves": "STR26 STA23 AC168", "boots": "STR20 STA18 POW50 AC112 AR28"},
        60: {"tunic": "STR20 STA20 HP40 AC396 FR18 AR18", "helm": "STR22 STA22 HP105 AC106 FR18", "leggings": "STR26 STA26 AC264 LR10 AR10", "gloves": "STR26 STA26 HP40 AC198 DR18", "boots": "STR26 STA26 HP40 AC132 CR18", "belt": "STR22 STA22 HP105 AC26 AR18", "bracers": "STR26 STA26 POW40 AC198 PR18"},
    },
    "Ranger": {
        30: {"tunic": "AGI12 DEX14 HP60 AC162", "helm": "AGI18 DEX12 POW50 AC43", "leggings": "AGI10 DEX13 AC108 PR13 DR10", "belt": "AGI13 DEX10 AC11 FR13 LR10"},
        40: {"tunic": "STA16 AGI16 POW50 AC207 AR18", "leggings": "AGI14 DEX14 AC138 DR30", "gloves": "AGI14 DEX14 AC104 AR30", "bracers": "AGI20 DEX16 POW60 AC104 AR10"},
        50: {"tunic": "AGI28 DEX20 HP30 AC290", "helm": "AGI20 DEX28 POW30 AC105", "leggings": "AGI30 DEX15 AC168 AR16 PR13", "bracers": "AGI20 DEX30 POW55 AC126"},
        60: {"tunic": "AGI27 DEX27 HP36 AC297", "helm": "AGI27 DEX27 HP36 AC79", "leggings": "AGI24 DEX26 HP40 AC198 FR26", "gloves": "AGI26 DEX24 HP40 AC149 AR26", "boots": "AGI26 DEX24 HP98 AC99 CR22 AR9", "belt": "AGI24 DEX26 HP65 AC20 FR22", "bracers": "AGI26 DEX24 HP15 AC149 PR28 DR9"},
    },
    "Rogue": {
        30: {},
        40: {"tunic": "AGI16 DEX20 HP65 AC207", "helm": "AGI16 DEX24 AC75 CR8 AR6", "leggings": "AGI20 DEX16 POW65 AC138", "gloves": "AGI16 DEX24 AC126 CR6 AR6", "boots": "AGI24 DEX18 AC89 FR6 LR6 PR8 DR6", "belt": "AGI18 DEX18 AC14 PR8 DR6", "bracers": "AGI16 DEX24 AC124 CR6 AR6"},
        50: {"tunic": "AGI20 DEX24 AC252 FR12 AR7 PR7 DR6"},
        60: {"tunic": "AGI15 DEX15 POW90 AC335 LR18 AR18", "helm": "AGI15 DEX15 POW90 AC117 AR18 PR18", "leggings": "AGI15 DEX15 POW90 AC243 PR18 DR18", "gloves": "AGI26 HP90 AC209 CR18 AR18", "boots": "AGI18 DEX18 HP75 AC99 CR18 AR18", "belt": "AGI18 DEX18 HP75 AC20 FR18 LR18", "bracers": "AGI18 DEX18 POW75 AC149 PR18 DR18"},
    },
    "Shadowknight": {
        30: {"tunic": "STR14 STA10 AC261 AR5 DR5", "helm": "STR18 STA14 AC74", "belt": "STR18 STA14 AC30"},
        40: {"tunic": "STR15 STA15 AC306 AR10 DR10", "helm": "STR16 STA18 AC74 AR6 PR6", "leggings": "STR15 STA15 AC214 PR10 DR10", "gloves": "STR16 STA16 POW50 AC138 DR18", "boots": "STR15 STA15 AC122 CR10 AR10", "belt": "STR18 STA16 AC18 FR7 CR6", "bracers": "STR16 STA16 POW50 AC138 AR18"},
        50: {"tunic": "STR24 STA28 AC336", "helm": "STR26 STA18 AC125 CR20 AR10", "leggings": "STR24 STA20 POW30 AC224 FR10 LR10", "boots": "STR20 STA20 AC112 AR7 PR8 DR8", "bracers": "STR18 STA26 AC203 FR20 LR10"},
        60: {"tunic": "STR20 STA20 POW85 AC396 CR15 AR15", "helm": "STR19 STA19 POW110 AC106 FR29", "leggings": "STR22 STA22 HP85 AC264 AR26", "gloves": "STR19 STA19 POW85 AC199 FR15 LR18", "boots": "STR19 STA19 HP110 AC132 LR29", "belt": "STR19 STA19 HP110 AC29 AR29", "bracers": "STR22 STA22 HP85 AC198 PR15 DR15"},
    },
    "Shaman": {
        30: {"gloves": "STA13 WIS10 AC81 AR10 DR13", "belt": "STA18 WIS14 AC27"},
        40: {"tunic": "STA16 WIS20 POW65 AC207", "leggings": "STA14 WIS14 AC138 LR30", "boots": "STA14 WIS14 AC69 AR30", "belt": "STA16 WIS17 AC64"},
        50: {"tunic": "STA24 WIS20 AC287 AR8 DR8", "belt": "STA20 WIS24 POW80 AC17"},
        60: {"tunic": "STA20 WIS20 HP135 AC297 LR10", "helm": "STA22 WIS22 POW55 AC79 FR15 LR15", "leggings": "STA22 WIS22 HP55 AC198 PR15 DR15", "gloves": "STA20 WIS20 HP132 AC149 FR10", "boots": "STA22 WIS22 POW55 AC99 CR15 AR15", "belt": "STA20 WIS20 POW135 AC20 AR10", "bracers": "STA25 WIS25 AC153 AR26"},
    },
    "Warrior": {
        30: {"tunic": "STR15 STA12 POW55 AC210", "helm": "STR10 STA14 AC103 AR5 PR5", "leggings": "STR15 STA12 POW55 AC140"},
        40: {"tunic": "STR18 STA23 AC276", "helm": "STR20 STA16 POW65 AC74", "leggings": "STR16 STA20 HP65 AC184", "gloves": "STR23 STA18 AC138"},
        50: {"tunic": "STR21 AC336 CR14 AR14 PR12", "helm": "STR21 AC90 FR14 LR14 CR12", "boots": "STR20 STA22 AC112 AR7 PR8 DR8", "belt": "STR20 STA20 AC22 FR8 LR8 CR7"},
        60: {"tunic": "STR28 STA26 HP55 AC396 PR9", "helm": "STR25 STA30 HP65 AC106 AR8", "leggings": "STR26 STA26 HP40 AC265 LR18", "gloves": "STR26 STA26 HP40 AC198 AR18", "boots": "STR26 STA26 HP40 AC132 FR18", "belt": "STR28 STA26 HP55 AC26 LR9", "bracers": "STR30 STA25 HP65 AC198 CR8"},
    },
    "Wizard": {
        30: {"tunic": "DEX14 INT12 POW60 AC54"},
        40: {"tunic": "DEX15 INT15 AC99 CR10 AR10", "helm": "DEX18 INT6 AC18 AR7 PR7 DR6", "leggings": "DEX18 INT6 AC46 CR7 AR7 PR6", "gloves": "DEX22 INT15 POW55 AC65", "boots": "DEX22 INT15 POW55 AC53", "belt": "DEX6 INT18 AC5 FR7 LR7 CR6"},
        50: {"tunic": "DEX15 INT20 AC158 FR16", "leggings": "INT35 HP70 AC56 CR16 AR14", "gloves": "DEX18 INT18 POW65 AC42 FR12 LR12", "boots": "INT35 POW70 AC28 CR14 AR16", "belt": "DEX18 INT18 POW65 AC6 PR12 DR12"},
        60: {"tunic": "DEX15 INT15 POW175 AC99", "helm": "DEX16 INT14 POW110 AC116", "leggings": "DEX20 INT20 HP80 AC141", "gloves": "DEX18 INT18 POW150 AC50", "boots": "DEX27 INT27 AC63", "belt": "DEX18 INT18 HP150 AC7", "bracers": "DEX20 INT20 POW80 AC125"},
    },
}


def fix(name):
    return CHART_FIX.get(name, name)


def norm(s):
    return " ".join(s.lower().replace("’", "'").replace("`", "'").split())


def set_name(cls, level):
    s = SETS[cls][LEVELS.index(level)]
    return s if s else SET_OURS[(cls, level)]


def set_word(cls, level):
    return set_name(cls, level).replace("{piece} of ", "")


def product_name(cls, level, piece):
    s = set_name(cls, level)
    word = piece.capitalize()
    return s.replace("{piece}", word) if "{piece}" in s else f"{s} {word}"


def pattern_name(pattern, piece):
    return f"{pattern} {piece.capitalize()} Pattern"


def parse(text):
    out = {}
    for tok in text.split():
        k = tok.rstrip("0123456789")
        out[COLS[k]] = int(tok[len(k):])
    return out


def estimates():
    """{(class, level, piece): (stats, how)} for every piece: EQOA-Data's, or the class's level 60 piece scaled."""
    ratio, out = {}, {}
    for cls in CLASS_ID:
        top = {p: parse(t) for p, t in STATS[cls][60].items()}
        for level in LEVELS:
            known = {p: parse(t) for p, t in STATS[cls].get(level, {}).items()}
            rs = [sum(st.values()) / sum(top[p].values()) for p, st in known.items() if p in top]
            if rs:
                ratio[(cls, level)] = statistics.median(rs)
    by_level = {lv: statistics.median([r for (_, lv2), r in ratio.items() if lv2 == lv]) for lv in LEVELS}
    for cls in CLASS_ID:
        top = {p: parse(t) for p, t in STATS[cls][60].items()}
        for level in LEVELS:
            known = {p: parse(t) for p, t in STATS[cls].get(level, {}).items()}
            r = ratio.get((cls, level), by_level[level])
            for piece, _, _ in PIECES:
                est = {k: max(1, round(v * r)) for k, v in top[piece].items()}
                if piece in known:
                    st, how = dict(known[piece]), "EQOA-Data"
                    if piece != "belt" and st.get("AC", 0) < AC_FLOOR * est.get("AC", 0):
                        how = f"EQOA-Data, its AC {st.get('AC', 0)} read as {est['AC']}"
                        st["AC"] = est["AC"]
                else:
                    st, how = est, f"estimate x{r:.2f}"
                out[(cls, level, piece)] = (st, how)
    return out


def blank_item(name, icon, price, level, slot, maxstack, craft, classes, desc, model=0, maxhp=0, duration=0):
    # rent 1: an ordinary item (0 is NO RENT, which the bank and the auction refuse)
    r = {"itemcost": price, "unk1": 0, "itemicon": icon, "unk2": 0, "equipslot": slot, "unk3": 0, "trade": 0, "rent": 1,
         "unk4": 0, "attacktype": 0, "weapondamage": 0, "unk5": 0, "levelreq": level, "maxstack": maxstack,
         "maxhp": maxhp, "duration": duration, "classuse": classes, "raceuse": ANY, "procanim": 0, "lore": 0, "unk6": 0,
         "craft": craft, "itemname": name, "itemdesc": desc, "model": model, "color": 4294967295}
    for c in STAT_COLS:
        r[c] = 0
    r["weaponproc"] = 0
    r["fish"] = 0
    return r


def the_chart():
    """[(kind, level, class, pattern, raw, modifier or None)] in the chart's order."""
    out = []
    for kind, (patterns, raws, classes) in CHART.items():
        for cls, mods in classes.items():
            for i, level in enumerate(LEVELS):
                mod = mods[i]
                out.append((kind, level, cls, patterns[i], fix(raws[i]), None if mod == NO_GEM else fix(mod)))
    return out


class Ids:
    """New keys of a table, after what it holds and its AUTOINCREMENT counter."""

    def __init__(self, db, tbl, col, seq=True):
        self.tbl, self.seq = tbl, seq
        top = db.execute(f"SELECT COALESCE(MAX({col}), 0) FROM {tbl}").fetchone()[0]
        if seq:
            r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = ?", (tbl,)).fetchone()
            top = max(top, r[0] if r else 0)
        self.next, self.used = top + 1, 0

    def take(self):
        self.used += 1
        self.next += 1
        return self.next - 1

    def seq_change(self):
        """The counter's row as it is after the writes: first in the list, so the undo puts it back last."""
        if not self.seq or not self.used:
            return None
        return ("sqlite_sequence", {"name": self.tbl}, {"name": self.tbl, "seq": self.next - 1})


def logged_keys(db, tbl):
    """The keys of the rows this tool made (inserted) in that table."""
    if not tr.table(db, LOG):
        return set()
    return {json.dumps(json.loads(k), sort_keys=True) for (k,) in
            db.execute(f"SELECT rowkey FROM {LOG} WHERE tbl = ? AND before IS NULL", (tbl,))}


def key_of(pid):
    return json.dumps({"patternid": pid}, sort_keys=True)


_names = {}


def find(db, name, level=None, slot=None):
    """This server's records of the name (any case), in id order; of that level and wear slot if given."""
    if db not in _names:
        idx = {}
        for pid, n in db.execute("SELECT patternid, itemname FROM itempattern ORDER BY patternid"):
            idx.setdefault(norm(n or ""), []).append(pid)
        _names.clear()
        _names[db] = idx
    out = []
    for pid in _names[db].get(norm(name), []):
        r = tr.get(db, "itempattern", {"patternid": pid})
        if level is None or (r["levelreq"] == level and r["equipslot"] == slot):
            out.append(r)
    return out


def changes(db):
    _names.clear()
    refusals, lines = [], []
    mine = logged_keys(db, "itempattern")
    items = Ids(db, "itempattern", "patternid")
    plan_items, plan_recipes, by_name, rarity_of = [], [], {}, {}
    reused_parts, same_name = [], []

    def part(name, row):
        """A part's id: this tool's record of the name; else this server's craft component of the name, kept as it
        is (the weaponsmiths' Magnesium Alloy); else a new record (a worn item of the name, the brass ring, is
        another thing)."""
        if name in by_name:
            return by_name[name]
        have = find(db, name)
        own = [h for h in have if key_of(h["patternid"]) in mine]
        comp = [h for h in have if h["craft"] == 1 and h["equipslot"] == -1]
        if own:
            pid = own[0]["patternid"]
        elif comp:
            pid = comp[0]["patternid"]
            reused_parts.append(f"{name} ({pid})")
            by_name[name] = pid
            return pid
        else:
            pid = items.take()
            if have:
                same_name.append(f"{name} (beside {', '.join(str(h['patternid']) for h in have)})")
        by_name[name] = pid
        plan_items.append(("itempattern", {"patternid": pid}, dict(row, patternid=pid)))
        return pid

    chart = the_chart()
    est = estimates()
    # the raw materials and the patterns, a level's each
    for kind, (patterns, raws, _) in CHART.items():
        for i, level in enumerate(LEVELS):
            raw = fix(raws[i])
            part(raw, blank_item(raw, ICON_RAW[kind], 10 * level, 0, -1, 99, 1, ANY,
                                 f"A master craftsman's raw material, worked with a {patterns[i].lower()} pattern "
                                 f"into level {level} armor."))
            for piece, _, _ in PIECES:
                n = pattern_name(patterns[i], piece)
                part(n, blank_item(n, ICON_PATTERN, 20 * level, 1, -1, 20, 1, ANY,
                                   f"A master craftsman's pattern for a {patterns[i].lower()} {piece}, cut with "
                                   f"{raw.lower()} and a class's modifier."))
    # the modifiers
    for kind, level, cls, pattern, raw, mod in chart:
        if mod is None:
            continue
        rarity_of[mod] = RARITY[level]
        word = mod.split()[-1].lower()
        part(mod, blank_item(mod, ICON_MOD.get(word, 140265582), MOD_PRICE[RARITY[level]], 0, -1, 50, 1, ANY,
                             f"A master craftsman's modifier: with a {pattern.lower()} pattern and {raw.lower()} it "
                             f"makes a {cls.lower()}'s {set_word(cls, level).lower()} armor.", duration=50))
    # the products and the recipes
    icons, table = {}, []
    recipes = Ids(db, "craftrecipes", "id", seq=False)
    rec_mine = {(r["name"], r["product"]): r for r in
                tr.rows(db, "SELECT id, name, product FROM craftrecipes WHERE source = ?", (SOURCE,))}
    made = reused = 0
    for kind, level, cls, pattern, raw, mod in chart:
        for piece, slot, value in PIECES:
            name = product_name(cls, level, piece)
            have = find(db, name, level, slot)
            if have and key_of(have[0]["patternid"]) not in mine:
                pid, how = have[0]["patternid"], "this server's record"
                reused += 1
            else:
                model = 0 if piece == "belt" else MODEL[kind]
                if (slot, model) not in icons:
                    r = db.execute("SELECT itemicon, COUNT(*) FROM itempattern WHERE equipslot = ? AND model = ? AND "
                                   f"levelreq >= 20 AND itemicon NOT IN (0, {', '.join(map(str, STAND_IN_ICONS))}) "
                                   "GROUP BY itemicon ORDER BY COUNT(*) DESC, itemicon LIMIT 1", (slot, model)).fetchone()
                    icons[(slot, model)] = r[0] if r else 0
                st, how = est[(cls, level, piece)]
                row = blank_item(name, icons[(slot, model)], round(value * level / 60), level, slot, 1, 0,
                                 1 << CLASS_ID[cls],
                                 f"A {kind} {piece} of master craftsmanship, of the {set_word(cls, level).lower()} "
                                 f"armor a {cls.lower()} wears.", model=model, maxhp=200 * level, duration=65)
                row.update(st)
                pid = have[0]["patternid"] if have else items.take()
                plan_items.append(("itempattern", {"patternid": pid}, dict(row, patternid=pid)))
                made += 1
            ings = [{"item": by_name[pattern_name(pattern, piece)], "count": 1, "keep": False},
                    {"item": by_name[raw], "count": 1, "keep": False}]
            if mod:
                ings.append({"item": by_name[mod], "count": 1, "keep": True})
            tl, rar = TRADE_LEVEL[level], (RARITY[level] if mod else 0)
            rname = name.lower()
            old = rec_mine.get((rname, pid))
            rid = old["id"] if old else recipes.take()
            plan_recipes.append(("craftrecipes", {"id": rid}, {
                "id": rid, "skill": ARMOR, "comp": SPECIAL, "level": tl, "rarity": rar, "name": rname, "product": pid,
                "yield": 1, "ingredients": json.dumps(ings), "difficulty": 10 * tl + 2 * rar,
                "xp": 25 * tl * (1 + rar), "discover": 1, "scroll": 0,
                "description": f"Master craft: a {cls.lower()}'s level {level} {piece}.", "source": SOURCE}))
            table.append((kind, level, cls, pattern, raw, mod, piece, name, pid, how))
    # the sellers
    npc_rows, ni_rows, seller_notes = [], [], []
    npcitem = Ids(db, "npcitem", "itemid")
    sold = {}
    for place, levels in SELLERS.items():
        for level, (metal_man, cloth_man) in levels.items():
            i = LEVELS.index(level)
            plate_chain = [pattern_name(CHART[k][0][i], p) for k in ("plate", "chain") for p, _, _ in PIECES] + \
                [fix(CHART["plate"][1][i])]
            cloth_leather = [pattern_name(CHART[k][0][i], p) for k in ("cloth", "leather") for p, _, _ in PIECES] + \
                [fix(CHART["cloth"][1][i]), fix(CHART["leather"][1][i])]
            for who, wares in ((metal_man, plate_chain), (cloth_man, cloth_leather)):
                full = f"Master Craftsman {who}"
                npcs = tr.rows(db, "SELECT id, npc_name, npc_type, npc_id FROM npcs WHERE npc_name = ? AND world = ? "
                                   "ORDER BY id", (full, SELLER_WORLD))
                if not npcs and who == cloth_man:
                    seller_notes.append(f"{full} ({place} {level}) is no NPC here: Master Craftsman {metal_man} sells "
                                        "his wares")
                    npcs = tr.rows(db, "SELECT id, npc_name, npc_type, npc_id FROM npcs WHERE npc_name = ? AND "
                                       "world = ? ORDER BY id", (f"Master Craftsman {metal_man}", SELLER_WORLD))
                if len(npcs) != 1:
                    refusals.append(f"{full} ({place}): {len(npcs)} NPCs in world {SELLER_WORLD}, one expected")
                    continue
                n = npcs[0]
                after = {"id": n["id"], "npc_type": (n["npc_type"] or 0) | 0x01, "npc_id": n["npc_id"] or n["id"]}
                if all(key["id"] != n["id"] for _, key, _ in npc_rows):
                    npc_rows.append(("npcs", {"id": n["id"]}, after))
                cat = after["npc_id"]
                if cat not in sold:
                    have = {r["patternid"]: r for r in tr.rows(db, "SELECT * FROM npcitem WHERE npcid = ?", (cat,))}
                    sold[cat] = {"have": have, "planned": set(),
                                 "next": max([r["listnumber"] or 0 for r in have.values()] + [-1]) + 1}
                s = sold[cat]
                for w in wares:
                    pid = by_name[w]
                    if pid in s["planned"]:
                        continue
                    s["planned"].add(pid)
                    if pid in s["have"]:
                        r = s["have"][pid]
                        ni_rows.append(("npcitem", {"itemid": r["itemid"]}, {"npcid": cat, "patternid": pid}))
                        continue
                    iid = npcitem.take()
                    ni_rows.append(("npcitem", {"itemid": iid}, {
                        "itemid": iid, "npcid": cat, "stackleft": 1, "remainHP": 0, "remaincharge": 0,
                        "patternid": pid, "equiploc": -1, "location": 1, "listnumber": s["next"]}))
                    s["next"] += 1
    # the drops
    pool_rows = []
    pool = Ids(db, "lootpool", "id")
    for kind, level, cls, pattern, raw, mod in chart:
        if not mod:
            continue
        pid = by_name[mod]
        r = db.execute("SELECT id, source FROM lootpool WHERE category = ? AND region = '*' AND npc_name = '' AND "
                       "patternid = ? ORDER BY id", (LOOT_POOL, pid)).fetchone()
        if r and r[1] != SOURCE:
            continue  # in the pool already (another craft's modifier): its row stays
        lid = r[0] if r else pool.take()
        pool_rows.append(("lootpool", {"id": lid}, {"id": lid, "category": LOOT_POOL, "region": "*", "npc_name": "",
                                                    "patternid": pid, "weight": LOOT_WEIGHT,
                                                    "minlevel": level - LOOT_UNDER, "maxlevel": 100,
                                                    "source": SOURCE}))
    # the standing auction lots (tools/ah_materials_20261114.py's rows and prices)
    lot_rows, lots = [], None
    if tr.column(db, "auction_lots", "perpetual"):
        lots = Ids(db, "auction_lots", "lot")
        for _, _, row in plan_items:
            if row["craft"] != 1:
                continue
            base = ah.RARE if rarity_of.get(row["itemname"], 0) >= 2 else ah.COMMON
            have = tr.rows(db, "SELECT lot FROM auction_lots WHERE seller = 0 AND perpetual = 1 AND patternid = ? "
                               "ORDER BY lot", (row["patternid"],))
            lot = have[0]["lot"] if have else lots.take()
            lot_rows.append(("auction_lots", {"lot": lot}, dict(ah.row(row, max(base, row["itemcost"])), lot=lot)))
    else:
        lines.append("auction_lots has no perpetual column here: no standing lots (run this again under a build with "
                     "them)")
    seqs = [c for c in (items.seq_change(), npcitem.seq_change(), pool.seq_change(),
                        lots.seq_change() if lots else None) if c]
    out = seqs + plan_items + plan_recipes + npc_rows + ni_rows + pool_rows + lot_rows
    pending = [(t, k, a) for t, k, a in out if not same(db, t, k, a)]
    if reused_parts:
        seller_notes.append("parts this server has (kept, with any standing lot they have): " + ", ".join(reused_parts))
    if same_name:
        seller_notes.append("parts made beside a worn item of the name: " + ", ".join(same_name))
    lines += report(db, table, pending, seller_notes, made, reused, est, len(lot_rows), len(pool_rows), sold)
    return pending, lines, refusals, not pending and not refusals


def same(db, tbl, key, after):
    now = tr.get(db, tbl, key)
    return now is not None and all(now.get(c) == v for c, v in after.items())


def report(db, table, pending, seller_notes, made, reused, est, n_lots, n_pool, sold):
    lines = []
    before = db.execute("SELECT COUNT(*) FROM craftrecipes WHERE source = ?", (SOURCE,)).fetchone()[0]
    cells = {(lv, c) for _, lv, c, *_ in table}
    lines.append(f"The chart: {len(cells)} class cells (15 classes x 4 levels), {len(table)} recipes (7 pieces each); "
                 f"{before} here already")
    lines.append(f"  products: {reused} this server's records (kept as they are), {made} this tool's")
    lines.append(f"  sold by {len(sold)} Master Craftsmen; {n_pool} modifiers in the loot pool; {n_lots} standing lots")
    for n in seller_notes:
        lines.append("  " + n)
    by_t = {}
    for t, _, _ in pending:
        by_t[t] = by_t.get(t, 0) + 1
    lines.append("rows to write: " + (", ".join(f"{t} {n}" for t, n in sorted(by_t.items())) or "none"))
    lines.append(f"{'class':<12} {'lvl':>3} {'TL':>2} {'pattern':<16} {'raw':<21} {'modifier':<22} set: pieces "
                 "(* an estimate, + this server's record)")
    seen = set()
    for kind, level, cls, pattern, raw, mod, piece, name, pid, how in table:
        if (cls, level) in seen:
            continue
        seen.add((cls, level))
        marks = " ".join(p + ("*" if h.startswith("estimate") else "+" if h.startswith("this") else "")
                         for _, lv2, c2, _, _, _, p, _, _, h in table if c2 == cls and lv2 == level)
        sname = set_word(cls, level) + (" (ours)" if (cls, level) in SET_OURS else "")
        lines.append(f"{cls:<12} {level:>3} {TRADE_LEVEL[level]:>2} {pattern:<16} {raw:<21} {(mod or '(no gem)'):<22} "
                     f"{sname}: {marks}")
    slips = [f"{c} {lv} {p}: {h}" for (c, lv, p), (_, h) in sorted(est.items()) if "read as" in h]
    if slips:
        lines.append("AC read as the estimate's (a slip in EQOA-Data): " + "; ".join(slips))
    return lines


SPEC = {"log": LOG, "detail": DETAIL,
        "needs": ("itempattern", "craftrecipes", "npcs", "npcitem", "lootpool", "auction_lots"),
        "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
