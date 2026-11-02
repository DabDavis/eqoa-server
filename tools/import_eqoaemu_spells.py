#!/usr/bin/env python3
"""The spells of EQOAEmu's spell workbook in eqoaclassic's tables (the user,
2026-11-01: "lean on them", and of its combat numbers, "Workbook wins,
re-fit"): every ability record the workbook gives (spellpattern), what each
does as the workbook's effects say it (spelleffect.effects, plus a line in
EQOA-Data's words), its area (spellarea), the classes that have a new one
(classspells), and a new vendor spell's scroll at the class's spell vendors
(itempattern, npcitem).

The workbook is the operator's (EQOAEmu's EQOA_Spells_and_CM, 2020, an .ods;
or its sheets as TSV files): it is not in this repository, and neither is the
map from its SpellIDs to ours (a TSV the import's coordinator made: sheet_id,
spellid, how; existing spells matched by name, name and class where several
share one, a new one numbered 900000 + int(SpellID, 16)). Every workbook
spell is named through that map, here and in the other lanes' tools.

The workbook's errors are put right as it is read (the user, 2026-11-01: "yes
fix it"): a Spells row that carries a second spell's effects beside its own is
two rows, the second on the record of ours of its name (TWO_SPELLS: Flesh
Mastery's row gives its "+" rank's 40 a stat for 30 minutes beside its own 20
for 5, so Flesh Mastery takes its own and Flesh Mastery + the other, as each
of the necromancer's other pet buffs and its "+" are; the mastery rank that
swaps them, import_eqoaemu_cms.py's RANK_ADDS).

What it writes, and from what:

  spellpattern, a spell the map says is ours: abilitylvl (Level), range,
      casttime, recast (two teleports' are not the workbook's 3 s: RECASTS),
      power, scope, where the workbook has a value, and
      (the user, 2026-11-01: "workbook wins on all") its icon, frame and
      spelling as a new spell's below; a rename only where every lookup by
      name reads the two alike (rename_safe), else listed; spelldesc and
      eqprequire stay ours (the workbook has no description), but for a record
      DESCRIPTIONS names, whose own says other than its row does (Flesh
      Mastery +'s names charisma; Agonize Death's, EQOA-Data's, is cut off
      mid-word: the user's "change it"). Several workbook rows
      mapped onto one record (one name, two classes' different spells) set it
      from the row that is the record's own (a class the record is linked to,
      its level, a class of Sandstorm's, its scope, the lowest SpellID); the
      others, split off in the map (--split-map: how "split", their own id
      900000 + SpellID, split_from the record), are made new records, and what
      was their classes' on the shared one moves over (plan_splits): their
      classspells rows, the books of characters of those classes, their kept
      buffs (charbuffs), the masteries of those classes alone (cmcatalog's
      add/require/remove_spells), the quest rewards of those classes alone
      (queststeps' spells and choices) or for them (byclass).
    a new spell: the same, and icon = IconID byte-reversed (the client's
      DictID, a signed 32-bit int as the column holds it; with --iso, the way
      round that is an icon in the disc's UI.CSF: icon_of), iconColor by
      IconBGDColor as every record of ours has its frame (Red 486870879 the
      target frame, Yellow -1809115846 self, Purple 593297002 group, Gray
      439871130 pet, Green -1580294943 the storms' area frame on Sandstorm's
      records), eqprequire from ReqEQuipment as VIWndSpellBook::AddEquipReq
      reads the mask (bit 0 1HS, 1 2HS, 2 1HB, 3 2HB, 4 1HP, 5 2HP, 6 RANGED, 7
      SHIELD; "None" 0, "Any" -1, as ours), spelldesc a sentence or two of
      what its effects do (ours: the workbook has none), unk2 and unk3 0.
    scope (VIAbility's): Self 0, Target 1, Group 2, Pet 3; "TB AoE" (round
      its target) 1 and "PB AoE" (round its caster, every player in it) 0,
      each with its spellarea row; "Owner" (a pet's spell on its owner) 1.
      A resurrection keeps its record's scope (1 for a new one): the server's
      rez is cast at a player (resurrect.go), and a group scope would pick
      only the living.
    range: the workbook's when it is over 1. Its 1 (and 0 or blank) is its
      word for self or touch ("Self are set to 1"; its melee skills too),
      which the records hold as 4.5 (CLIENT's TargetInRange reach; all of
      Sandstorm's melee skills): a self or target spell takes 4.5, unless
      ours is one of Sandstorm's own records (its glamours' 5.0 stays); a
      group or pet spell keeps ours (a new one 30.0, the records' convention
      for them). The column holds the float's high half (writeAbilityRecord).

  spelleffect: `effects`, JSON v1 as the import's brief defines it (the
      column is added when the database lacks it), built from the spell's
      EffectID1-4 rows of the Effects sheet, procs resolved (ApplyProc1/2 ->
      those rows, "on" offense for an Offensive Proc, defense for a
      Defensive Proc); the numbers the workbook's, untouched, but for the
      times of two lasting debuffs the workbook gives none (DURATIONS: its
      Duration 0 on the spell's row and its effects', so the debuff lasted
      no time and never landed). Where the shape
      has no word for what a row says, the effect carries a key of its own
      (see EXTRA_KEYS). Two readings the shape leaves open, made here:
        - a Dmg/Heal row with a positive HP1 and a duration of a minute or
          less (HEAL_TICKS_UP_TO: every such row of the workbook's lasts 18
          or 24 s) is a heal each tick ("hp_tick"): the workbook's legend has
          a heal tick as a HoT does, twice at its landing and every 6 seconds
          after, and Sandstorm's Minor Healing ticked its line's figure
          (casting-sandstorm-20261021.md). One that lasts longer is a heal at
          its landing ("hp"): the legend's HP1 is the "Damage (or heal) base
          number", a heal over time its HoT, and the user's Avatar (2026-11-01)
          "is a one time max hp buff and massive stat buff lasts until next
          avatar recast": its time the buff's beside it. A negative HP1 with a
          duration stays a hit ("hp", its duration kept): the workbook's own
          known-issues list calls some of those durations errors;
        - a DoT figure is damage whatever its sign ("hp_tick" negative): the
          column is "Damage over time".
      `line`: the same effects in EQOA-Data's words, as entity/spelltext.go
      reads them, so the GM panel and the logs say what the spell does, and
      a server reading no `effects` reads the same figures; what those words
      cannot say is written as plain words the parser leaves alone. A spell
      the workbook gives no effect for keeps its line. fxdict stays as it is
      (tools/fill_spell_visuals_eqoaemu.py filled it); a row made here takes
      the workbook's Graphic when the disc has that visual (--iso), else 0
      (the server's stand-in).

  spellarea: a spell whose effects give an AoERadius, or whose scope is an
      area one, is an area spell (area.go): centre 1 (round its target) for
      Target and TB AoE, 2 (round its caster) for Self, PB AoE and Group;
      radius its AoERadius (0: the server's own); maxtargets 0 (the server's
      8). A proc's radius is its proc's, not the spell's.

  classspells: a new spell gets a row for each class the workbook gives it,
      source "eqoaemu" (got by level: a scroll from a vendor, a quest or a
      drop, so #template and start_all_spells give it, as eqoa-data's) or
      "cm (eqoaemu)" for a mastery's (database.GotBy reads "cm": no class
      grant hands it out; the masteries' own tool gives it through cmcatalog
      add_spells). A spell we have keeps its rows; --link-existing adds the
      classes the workbook gives it that hold no spell of its name.

  itempattern, foodeffect, itemeffect: the workbook's food and drink
      (plan_foods: its Created Item sheets' items that give only power or
      health over time, or health): ours take its HoT and PoT, one we lack is
      made as a sibling of ours (910000 + its ItemID), each its foodeffect row
      (its time, DisspellOnCast, DisspellOnContact: entity/consumable.go);
      one that heals at once as well is a potion, an itemeffect line.
    and every other item a spell makes that is none of ours (MadeItems: the
      server summons an item by its name, so a spell whose item we lack
      makes nothing): one the Created Item sheets give (by its name, or for
      a Create Item that names none, the sheets' item of no ItemID named for
      its spell: the Surefall Potion) is a potion, as tools/seed_potions.py
      made the alchemist's: its row its sibling's (an item with a use that
      another spell of its class makes, drunk or thrown as it is, of its kind
      (a cure, a teleport, a splash), the nearest its level; a level 1
      sibling's stays 1), and its use an itemeffect line in the server's own
      words (entity/potion.go reads it as spelltext.go reads a line): a cure
      the sibling's "Dispel Effect xN", a teleport the place of the spell
      gate within 15 of its Location ("Teleport to <place>", potion.go), a
      thrown one's splash its AoERadius with the area's figure (HP2: the
      splash lands the line on every creature in it, its target among them)
      and its target's own (HP1) its target_line, its body its Alter
      Appearance's sprite the other way round (itemeffect.body, as the
      server reads a spell's sprite: the workbook writes every SpriteID
      byte-reversed; the invisible body none, its Invisible said by the
      line; with --bodies, cmd/modelbake's models.txt, one the disc has no
      CSprite of listed and not written), its reach (unk5) its Range; what
      no line says (a resist) listed. It is named as our records
      name such items (MADE_NAMES: "<X> Corrective" for the sheets' "<X>
      Cure Potion", "<X> Potion" for a "<X> Mix Potion", no " Effect"),
      910000 + its ItemID (950000 + its spell's SpellID with none). One no
      source gives is made by its spell's words (OUR_KINDS: a conjured
      staff, a rogue's daggers, the racial masteries' food and drink) as our
      item of its kind nearest its level is, OURS: a weapon NO RENT (rent 0,
      as every conjured weapon of ours), its damage and item HP the median of
      our weapons of its type within 2 levels when its model is further off
      (tools/fix_dump_weapons.py's rule), a food its model's figures and
      foodeffect row. An item of ours the sheets give a use for that has
      none (no itemeffect, foodeffect or meal's or drink's words) takes the
      workbook's, its row made a thrown one's when the use is thrown. A
      record of ours the map spells otherwise (one spell under two
      spellings) makes the item its line names, the 2004 records' (the
      Transformative Pill, not the Creative Potion the workbook's name finds:
      that one is listed, `folds`, and tools/fix_eqoaemu_followup2.py folds
      a made one into it), and that item takes the workbook's use over its
      own as the record takes the workbook's (its effect's name kept, its
      icon a made potion's), and the workbook's item's level (the Pill's 60,
      the user's "yes make it level 60", 2026-11-01). A use of ours the
      workbook gives as invisibility (the Ethereal potions', the Phase
      Ring's) takes the workbook's time in its line (the user's "use
      workbook durations"; its words otherwise ours). A use the import wrote
      takes, on a plan made again, the body and target's own share it says
      now (a GM's value since left, and said).

  spellgates: a new teleport whose Location names a world and a point gets
      its row (kind group or self by its scope, its radius the spell's, the
      place as a row at that spot names it); a Location differently named
      teleports share is the workbook's stand-in, listed.

  How a player gets a new class spell (its Learn Method and ScrollSource):
    Scroll + Vendor: a scroll as ours are (the scroll's art, worn nowhere:
      handlers/scribe.go scribes the first spell of its name at its level),
      id = its spell's, the classes' and races' masks the workbook's, priced
      on Sandstorm's scroll curve (stock_merchants.py's Rule); sold by every
      merchant that sells three or more of a class's scrolls in a band
      holding its level (else the nearest band), as
      import_eqoadata_spells.py stocks one, never past the 40 records a
      client's merchant list holds. A spell a scroll of its name already
      teaches its class (handlers/scribe.go's rule: the exact name before
      spellKey's, the class's record first), or would scribe another record
      of its name, gets none: listed. A scroll it finds keeps its level, so
      one whose spell's level this sets asks for the old one:
      tools/fix_scrolls.py, run after this import, brings each scroll to its
      spell's level (and folds or renames a scroll that teaches nothing).
    Scroll + Quest: listed, unless a quest's reward already scribes it.
    Scroll + Drop: listed (no loot is changed).
    CM: nothing here (the masteries lane's).
  A spell of no class (a pet's or a creature's) is a record and its effects.

Report only by default. --save-plan FILE writes what --apply would do and
stops; --plan FILE applies such a plan where the database is (each change
only where the row is still as planned). --apply writes it all in one
transaction, every row logged in eqoaemu_spell_import_log (its key, the
columns before and after; a deleted row's rowid); --undo puts each logged
row back as it was (one changed since is left, and said), the counters
too, and drops the effects column, a spellarea or spellgates table it made
when they are empty again (--apply adds itemeffect's body and target_line
to a database made before them when it writes them, and --undo drops them
again when no row holds anything in them); refused while a character holds a made spell
(a book a split moved aside) or scroll, unless --force. Both take a copy of the database first (SQLite's backup API). A
cast reads its spell at the cast, a scroll at its use and a catalog at the
window, so no restart is needed; the class templates and pet taps read
classspells at the start.

    tools/import_eqoaemu_spells.py --sheets <dir of TSVs> | --ods <workbook.ods>
        --map <spellmap.tsv> [--iso <disc.iso>] [--bodies <worlds/models.txt>] [--db eqoa.db] [--save-plan FILE]
    tools/import_eqoaemu_spells.py --db eqoa.db --plan FILE --apply
    tools/import_eqoaemu_spells.py --db eqoa.db --undo [--force]
"""
import argparse
import collections
import csv
import datetime
import json
import os
import re
import sqlite3
import struct
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
DEFAULT_DB = os.path.join(HERE, "..", "eqoa.db")

LOG = "eqoaemu_spell_import_log"
META = "eqoaemu_spell_import_meta"
def log_ddl(log=LOG, meta=META):
    """A log table and a facts table, the import's or another tool's (the
    follow-up's: tools/fix_eqoaemu_followup.py)."""
    return f"""
CREATE TABLE IF NOT EXISTS {meta} (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
-- Every row written, in order: --undo takes them back last first.
CREATE TABLE IF NOT EXISTS {log} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,     -- one of KEYS' tables
    key     TEXT NOT NULL,     -- the row's key columns (JSON)
    action  TEXT NOT NULL,     -- insert | update | delete
    before  TEXT,              -- the columns written, as they were (JSON); an insert: none
    after   TEXT,              -- the columns written, as written (JSON); a delete: none
    note    TEXT NOT NULL DEFAULT ''
);
"""


LOG_DDL = log_ddl()
# each table written, by the columns that key a row
KEYS = {"spellpattern": ["spellid"], "spelleffect": ["spellid"], "spellarea": ["spellid"],
        "classspells": ["class", "spellid"], "itempattern": ["patternid"], "npcitem": ["itemid"],
        "spells": ["id"], "charbuffs": ["serverid", "seq"], "cmcatalog": ["id"], "queststeps": ["questid", "step"],
        "spellgates": ["spell"], "foodeffect": ["patternid"], "itemeffect": ["patternid"]}
# a food, drink or potion made from the Created Item sheets: 910000 + the workbook's ItemID
ITEM_BASE = 910000
# a made item that has no ItemID (the sheets' one of none, OUR_KINDS'): 950000 + its spell's SpellID
MADE_BASE = 950000
# Recasts that are not the workbook's, by the spell's name (lower case): of
# the five teleports its Location left with no place (tools/seed_spell_gates.py),
# Envaric Acension's is the hour EQOA-Data's Envaric Ascension guide gives
# ("Recast: 3600"), and the Circle of Dead Hills' its kin's, every druid
# circle's on this server (ours, by kin); the other three keep the
# workbook's 3, every wizard gate's, portal's and escape's here (2026-11-01,
# the EQOAEmu follow-ups' places).
RECASTS = {"envaric acension": 3600, "circle of dead hills": 600}
# Times that are not the workbook's, by the spell's name (lower case): two
# lasting debuffs it gives Duration 0, on the spell's row and on the effect
# rows (02-Spells, 03-Effects), so the debuff lasted no time and never landed
# (entity/cast.go's lasting): the mastery Swamp Stench's AGI and AC and the
# rogue's epic Strategic Blow's AC take 30 s, the user's figure (2026-11-01:
# "30 sec for both"; EQOA-Data's line gave Strategic Blow's AC 45 seconds).
# The spell's duration_s, where its row has none (spell_json, with_duration).
DURATIONS = {"swamp stench": 30, "strategic blow": 30}
# Spells of ours the workbook lacks, made the mirror of one it has (the user,
# 2026-11-01: "yes make it a taunt proc", "yes make them taunt procs"): the
# enchanter's taunting Visages, whose EQOA-Data lines ("Group, Enchant
# warriors weapon with taunt, 5min", "Group, Weapon proc, 5min") read to no
# effect, each Calming Visage's effects (a friendly target's offensive proc
# of kind hate, 5 min, its stack) with its proc raising by the figure here
# (ours) the hate of what its bearer's weapon hits; at its own record's
# level, its lowest receiver's as far below it as the original's (10); its
# record a target spell as the original is (scope, frame), its description
# MIRROR_DESC (ours). mirror_json, mirror_changes; spell name -> (the
# original's, the hate a proc gives).
MIRRORS = {"alarming visage": ("calming visage", 10), "haunting visage": ("calming visage", 20),
           "ghastly visage": ("calming visage", 40)}
MIRROR_DESC = "Enchants the target's weapon with taunt: what its blows strike comes to hate it more."
MIRROR_NOTE = "effects ours: the mirror of the workbook's {}"
# Spells of ours the workbook lacks that make an item of ours (the user,
# 2026-11-01: "yes make frosty potion summon a frosty potion"): EQOA-Data's
# line ("SELF Summon Frosty Potion") reads to nothing; each takes the frame of
# the workbook's spell named (Icy Mixture's: a self spell's create_item),
# making the count here of the item of ours by its name (the server's own
# lookup), at its own level (makes_json). Spell name -> (the workbook's
# maker, the item, the count).
MAKES = {"frosty mixture": ("icy mixture", "Frosty Potion", 1)}
# Spells of ours the workbook lacks that cure (the user, 2026-11-01: "make it
# cure up to 3 effects"): EQOA-Data's line ("Dispel Effect x3") reads to
# nothing; each a cure of up to the count here of the harmful effects on its
# bearer, the first come by first (the reader's cure_max), self or a
# friendly target as its record's scope has it, in the workbook's cure frame
# (Minor Curative's: "Remove Ability"), at its own level (cures_json).
CURES = {"restorative": 3}
# their descriptions (ours)
OURS_DESC = {"frosty mixture": "Makes a Frosty Potion.",
             "restorative": "Removes up to three harmful effects from you, the first you came by first."}
# The workbook's errors, put right (the user, 2026-11-01: "yes fix it", "change
# it"). TWO_SPELLS: a Spells-sheet row that carries a second spell's effects
# beside its own, by its name (folded) -> the second spell's name and the
# places (EffectID1-4) of its effects in the row. Flesh Mastery's row gives its
# "+" rank's 40 STR, STA and DEX for 30 minutes beside its own 20 for 5, where
# each of the necromancer's other pet buffs has its "+" a row of its own, the
# one its mastery rank swaps it for (import_eqoaemu_cms.py's RANK_ADDS). The
# second spell is a row of its own (two_spells), on the record of ours of its
# name (the map's rule for a spell we have; none of ours: listed).
TWO_SPELLS = {"flesh mastery": ("Flesh Mastery +", (2,))}
# DESCRIPTIONS: a record of ours whose own description says other than what
# the workbook's row it takes does, by its name (folded) -> ours (None: made
# from its effects, as a new record's is: desc_of). Flesh Mastery +'s names
# charisma; Agonize Death's is EQOA-Data's cut off mid-word (the record takes
# the workbook's Agonize Undead: its hit and its wound over 24 seconds).
DESCRIPTIONS = {"flesh mastery +": None,
                "agonize death": "Level 60 Epic third spell option. An arcane strike at an undead foe. Its wound "
                                 "damages it over 24 seconds. Resisted by AR."}
# MADE_ERRATA: a spell whose row makes another spell's item, by its name
# (folded) -> the workbook's name of the item it makes, its own name's and
# level's: the Spells sheet gives the ranger's Forage Grub (L25) the created
# item of Foraged Fruit (L35) and Forage Fruit the item of Foraged Grub, so
# each made the other's meal (the user, 2026-11-01: "yes swap them").
MADE_ERRATA = {"forage grub": "Foraged Grub", "forage fruit": "Foraged Fruit"}
# MEAL_RULE: the workbook's meals that end as a meal of ours does (entity/
# consumable.go: refused in a fight, ended by an aggressive action or a hit:
# a foodeffect row's ends_on_contact, not its ends_on_cast) and last the
# workbook's time (its Created Item Effects' Duration, 120 s: the user,
# 2026-11-01, "yes use 2 minutes"), by the workbook's item name (folded): the
# ranger's five foraged meals, which the workbook ends on any cast instead.
MEAL_RULE = {"foraged berries", "foraged grub", "foraged fruit", "foraged herbs", "foraged tuber"}
# the key on a Spells row two_spells made, naming the row it was cut from
ERRATUM_OF = "erratum of"
# a Dmg/Heal row with a positive HP1 lasting at most this many seconds heals
# each tick; one lasting longer heals once, at its landing (the doc)
HEAL_TICKS_UP_TO = 60
# the worlds a Location names (gmweb's world numbers)
WORLDS = {"tunaria": 0, "rathe": 1, "odus": 2, "lavastorm": 3, "plane of sky": 4, "secrets": 5}
NOTE = "EQOAEmu's workbook (2020)"
NEW_BASE = 900000
CLASSES = {"WAR": 0, "RAN": 1, "PAL": 2, "SK": 3, "MNK": 4, "BRD": 5, "RGE": 6, "DRD": 7, "SHA": 8, "CL": 9,
           "MAG": 10, "NEC": 11, "ENC": 12, "WIZ": 13, "ALC": 14}
RACES = {"HUM": 0, "ELF": 1, "DELF": 2, "GNO": 3, "DWF": 4, "TRL": 5, "BAR": 6, "HLF": 7, "ERU": 8, "OGR": 9,
         "GNM": 3}  # a gnome, as one row spells it
ALL_CLASSES, ALL_RACES = 32767, 1023
SCROLL_ICON = -607649328
LIST_CAP = 40          # the records a client's merchant list holds (handlers' invListCap)
TOUCH, FAR = 4.5, 30.0

SCOPE = {"Self": 0, "Target": 1, "Group": 2, "Pet": 3, "TB AoE": 1, "PB AoE": 0, "Owner": 1}
JSON_SCOPE = {"Self": "self", "Target": "target", "Group": "group", "Pet": "pet", "TB AoE": "target_aoe",
              "PB AoE": "player_aoe", "Owner": "target"}
# where an area spell's area lies (spellarea.centre), by its scope
AREA_CENTRE = {"Target": 1, "TB AoE": 1, "Self": 2, "PB AoE": 2, "Group": 2}
FRAMES = {"Red": 486870879, "Yellow": -1809115846, "Purple": 593297002, "Gray": 439871130, "Green": -1580294943}
WEAPON_BITS = {"1HS": 1, "2HS": 2, "1HB": 4, "2HB": 8, "1HP": 16, "2HP": 32, "RANGE": 64, "RANGED": 64,
               "SHIELD": 128}
RESIST = {"AR": "ar", "FR": "fr", "CR": "cr", "LR": "lr", "PR": "pr", "DR": "dr", "Blunt": "blunt",
          "Pierce": "pierce", "Slash": "slash"}
MODS = {"str", "sta", "agi", "dex", "wis", "int", "cha"}
# the Effects sheet's figure columns that are a stat (the brief's names)
STAT_COLS = [("STR", "str"), ("STA", "sta"), ("AGI", "agi"), ("DEX", "dex"), ("WIS", "wis"), ("INT", "int"),
             ("CHA", "cha"), ("AC", "ac"), ("HPMAX", "maxhp"), ("POWMAX", "maxpower"), ("HoT", "hot"),
             ("PoT", "pot"), ("FR", "fr"), ("CR", "cr"), ("LR", "lr"), ("PR", "pr"), ("DR", "dr"), ("AR", "ar"),
             ("Pierce Debuff", "pierce"), ("Blunt Debuff", "blunt"), ("Slash Debuff", "slash"),
             ("Ranged Debuff", "ranged")]
# the TYPEs whose row is the one effect of its kind, its figures beside it
TYPE_KIND = {"Summon Pet": "summon", "Alter Appearance": "illusion", "Create Item": "create_item",
             "Teleport": "teleport", "Teleport to Bind Point": "gate", "Remove Ability": "cure",
             "Faction Change": "faction", "Charm": "charm", "Invisible": "invis", "Lull": "lull",
             "Enduring Breath": "breath", "Hide": "hide", "Pickpocket": "pickpocket",
             "Ressurect Player": "resurrect", "Invulnerable": "invulnerable", "Offensive Proc": "proc",
             "Defensive Proc": "def_proc", "Summon Player": "summon_player", "Increase Speed": "speed"}
# database/were.go's IsWereAbility: a were form or a were's ability, by its
# name or description. No class grant gives one (#template, the new
# characters'), so none gets a classspells row here, whatever CLASS says.
WERE_ABILITY = re.compile(r"(?i)\bwere(rat|wolf|bear|lion|gator)|(rat|lion|wolf|bear|gator)were\b|power of the beast|"
                          r"^assume the form of|^cm: illusion\.?$")
WERE_QUESTS = os.path.join(HERE, "quests", "data", "weres.json")
# the were quests' kinds (their keys' second part) as a description words them
WERE_KIND = {"wolfwere": "Werewolf", "bearwere": "Werebear", "ratwere": "Wererat", "lionwere": "Werelion",
             "gatorwere": "Weregator", "werehunter": "Werehunter", "vampire": "Vampire"}


def is_were_ability(name, desc):
    return bool(WERE_ABILITY.search(name or "") or WERE_ABILITY.search(desc or ""))


def quest_fold(s):
    """A name as tools/import_quests.py matches a reward by it."""
    s = str(s or "").lower().replace("\u2019", "'").replace("`", "'").replace("'", "")
    return " ".join(re.sub(r"[^\w\s-]", " ", s).split())


def were_rewards(path):
    """The spells the were quests give, by quest_fold: (the name as the file
    spells it, the quest's key)."""
    out = {}
    if not path or not os.path.exists(path):
        return out

    def walk(o, key):
        if isinstance(o, dict):
            for k, v in o.items():
                if k == "spells" and isinstance(v, list):
                    for n in v:
                        out.setdefault(quest_fold(n), (n, key))
                else:
                    walk(v, key)
        elif isinstance(o, list):
            for x in o:
                walk(x, key)
    with open(path, encoding="utf-8") as f:
        data = json.load(f)
    for q in data.get("quests", []):
        walk(q, q.get("key", ""))
    return out


# the kinds whose own code picks whom they land on (pet.go, charm.go,
# illusion.go, the gates, resurrect.go): no spellarea row for them, as
# area.go's reading leaves pets, charms and illusions out
OWN_TARGETS = {"summon", "charm", "illusion", "teleport", "gate", "resurrect", "summon_player"}
# what the brief's shape has no word for, kept under keys of their own
EXTRA_KEYS = {
    "absolute": "a Statistic Absolute row's speed, power or hit points (its stats are stat_set)",
    "target_type": "an effect's Target Type where it is not its spell's (a proc's always)",
    "min_level": "an effect's MinLevelEffect where it is not its spell's",
    "radius": "an effect's AoERadius where it is not its spell's (a proc's)",
    "level": "an effect's Level where it is not its spell's",
    "sheet_item_id": "a created or consumed item no item of ours is named as",
    "sheet_dot": "a DoT figure written positive, as written",
    "summon_player": "kind summon_player: the workbook's Summon Player TYPE (no kind in the shape)",
    "stat power": "a Drain/Drain Group row's POW: stat \"power\" (the shape's stats have none)",
    "sheet_scope": "the spell's Scope where the shape has none (Owner: a pet's spell on its owner)",
    "arch_type": "the Spells sheet's ArchType, as written",
    "sheet_type": "the Spells sheet's TYPE, as written",
}

SHEETS = {"Spells": "02-Spells.tsv", "Effects": "03-Effects.tsv", "Created Item Info": "04-Created_Item_Info.tsv",
          "Created Item Effects": "05-Created_Item_Effects.tsv", "Pets": "07-Pets.tsv"}


# ---- reading -------------------------------------------------------------------

def read_tsv(path):
    """A sheet's rows as dicts by its header (a header named twice: "X#2")."""
    with open(path, encoding="utf-8", newline="") as f:
        rows = list(csv.reader(f, delimiter="\t", quoting=csv.QUOTE_NONE))
    names, seen = [], collections.Counter()
    for h in rows[0]:
        seen[h] += 1
        names.append(h if seen[h] == 1 else f"{h}#{seen[h]}")
    return [{names[i]: (r[i] if i < len(r) else "") for i in range(len(names))}
            for r in rows[1:] if any(c.strip() for c in r)]


def read_workbook(sheets=None, ods=None):
    if ods:
        import fill_spell_visuals_eqoaemu as fsv
        return {name: fsv.ods_rows(ods, name) for name in SHEETS}
    return {name: read_tsv(os.path.join(sheets, f)) for name, f in SHEETS.items()}


def read_map(path):
    """sheet SpellID (upper case hex) -> (spellid, how, split_from): a row
    split from a record another row keeps (how "split") names that record."""
    out = {}
    with open(path, encoding="utf-8", newline="") as f:
        for r in csv.reader(f, delimiter="\t", quoting=csv.QUOTE_NONE):
            if not r or r[0].startswith("#") or r[0] == "sheet_id" or len(r) < 3:
                continue
            frm = r[7].strip() if len(r) > 7 else ""
            out[r[0].strip().upper()] = (int(r[1]), r[2].strip(), int(frm) if frm.isdigit() else None)
    return out


def map_entry(smap, hx):
    """A map row as (spellid, how, split_from), whatever its length."""
    m = smap.get(hx)
    if m is None:
        return None
    return (m[0], m[1], m[2] if len(m) > 2 else None)


def cell(r, k):
    """A cell's text, its line breaks (" / " in the TSVs) as spaces."""
    v = r.get(k)
    if v is None:
        return ""
    return " ".join(p.strip() for p in re.split(r"\n| / ", str(v)) if p.strip())


def num(r, k):
    """A cell's number (int when whole), None when blank or not a number."""
    v = cell(r, k)
    if v == "":
        return None
    try:
        f = float(v)
    except ValueError:
        return None
    return int(f) if f == int(f) else f


def hexid(v):
    v = (v or "").strip().upper()
    return v if re.fullmatch(r"[0-9A-F]{1,4}", v) else ""


def spell_key(name):
    """The name a scroll and its spell share: handlers/scribe.go's spellKey."""
    n = (name or "").strip()
    if n.endswith("$"):
        n = n[:-1].rstrip()
    n = re.sub(r"\s*\((?:[A-Za-z]{2,3}(?:\s*[,/]\s*)?)+\)\s*$", "", n)
    return n.replace("+", "").replace("`", "'").strip().lower()


def fold(s):
    return re.sub(r"\s+", " ", (s or "").replace("`", "'").strip().lower())


def columns(db, table):
    return [r[1] for r in db.execute(f"PRAGMA main.table_info({table})")]


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


# ---- the record's columns ------------------------------------------------------

def range_bits(r):
    """A range as the column holds it: the float's high half (exact)."""
    b = struct.unpack(">I", struct.pack(">f", float(r)))[0]
    if b & 0xFFFF:
        raise ValueError(f"range {r} has no exact high half")
    return b >> 16


def range_of(col):
    """The range a column holds (a stray small int read as itself)."""
    if 0 <= col <= 255:
        return float(col)
    return struct.unpack(">f", struct.pack(">I", (col & 0xFFFF) << 16))[0]


def icon_of(text, icons=None):
    """IconID as the signed int the icon column holds: byte-reversed, as the
    workbook mostly writes it, unless that is no icon on the disc and the id as
    written is one (icons: disc_icons' set; 2026-11-01's workbook writes 12 of
    its spells' ids our way round, Lightning Arrow's 0x7C84475E among them, and
    reversed they were no icon at all); None if not 8 hex digits, or with the
    disc read when neither way round is an icon."""
    t = (text or "").strip()
    t = t[2:] if t[:2].lower() == "0x" else t
    if not re.fullmatch(r"[0-9a-fA-F]{8}", t):
        return None
    v = int(t, 16)
    r = struct.unpack("<I", struct.pack(">I", v))[0]
    if icons is not None and r not in icons:
        if v not in icons:
            return None
        r = v
    return struct.unpack("<i", struct.pack("<I", r))[0]


def equip_mask(text):
    """ReqEQuipment as eqprequire: -1 any, 0 none, else VIWndSpellBook::AddEquipReq's bits."""
    t = (text or "").strip()
    if t == "" or t.lower() == "any":
        return -1
    if t.lower() == "none":
        return 0
    mask = 0
    for w in re.split(r"[,\s]+", t.upper()):
        if w:
            if w not in WEAPON_BITS:
                return None
            mask |= WEAPON_BITS[w]
    return mask


def class_mask(text):
    return [CLASSES[c] for c in (text or "").split() if c in CLASSES]


def race_mask(text):
    m = 0
    for r in (text or "").split():
        if r in RACES:
            m |= 1 << RACES[r]
    return m or ALL_RACES


# ---- the effects, as the brief's JSON v1 ----------------------------------------

def two_spells(spells, effects):
    """The Spells sheet's rows, each TWO_SPELLS row made two: the row with its
    own effects alone, then the second spell's row (its name, the effects at
    its places, its time theirs, a mastery's as the other "+" rows are; no
    SpellID of its own, ERRATUM_OF naming the row's). A row whose places hold
    no effect stays as it is (the workbook put right). Returns the rows and
    what was cut, as (the row's SpellID, its name, the second's, its effects)."""
    out, cut = [], []
    for s in spells:
        spec = TWO_SPELLS.get(fold(cell(s, "Name")))
        if spec is None:
            out.append(s)
            continue
        name, places = spec
        ids = [hexid(s.get(f"EffectID{i}")) for i in range(1, 5)]
        mine = [x for i, x in enumerate(ids, 1) if x and i not in places]
        theirs = [x for i, x in enumerate(ids, 1) if x and i in places]
        if not theirs:
            out.append(s)
            continue
        first, second = dict(s), dict(s)
        for i in range(1, 5):
            first[f"EffectID{i}"] = mine[i - 1] if i <= len(mine) else ""
            second[f"EffectID{i}"] = theirs[i - 1] if i <= len(theirs) else ""
        second.update({"SpellID": "", "Name": name, "Learn Method": "CM", "ScrollSource": "",
                       ERRATUM_OF: hexid(s.get("SpellID"))})
        times = {num(effects[x], "Duration") for x in theirs if x in effects} - {None, 0}
        if len(times) == 1:
            second["Duration"] = str(times.pop())
        out += [first, second]
        cut.append((hexid(s.get("SpellID")), cell(s, "Name"), name, theirs))
    return out, cut


class Workbook:
    """The sheets, indexed."""

    def __init__(self, sheets):
        self.effects, self.effect_clash = {}, []
        for e in sheets["Effects"]:
            i = hexid(e.get("EffectID"))
            if not i:
                continue
            if i in self.effects:
                if any(cell(e, k) != cell(self.effects[i], k) for k in e):
                    self.effect_clash.append(i)   # the first kept
                continue
            self.effects[i] = e
        self.spells, self.errata = two_spells(sheets["Spells"], self.effects)
        self.items = {}
        for r in sheets["Created Item Info"]:
            n = num(r, "ItemID")
            if n is not None and cell(r, "Name"):
                self.items[n] = cell(r, "Name")
        self.pets = {hexid(r.get("PetID")): r for r in sheets["Pets"] if hexid(r.get("PetID"))}
        self.item_info = sheets.get("Created Item Info") or []
        self.item_effects = {hexid(e.get("EffectID")): e for e in sheets.get("Created Item Effects") or []
                             if hexid(e.get("EffectID"))}


def spell_ctx(s, wb):
    """What a spell's effects are compared with: its scope, target type, duration, level, min level, radius."""
    rows = [wb.effects[i] for i in effect_ids(s) if i in wb.effects]
    radius = max([num(e, "AoERadius") or 0 for e in rows] + [0])
    return dict(scope=cell(s, "Scope"), target_type=cell(s, "Target Type"), duration=num(s, "Duration") or 0,
                level=num(s, "Level"), min_level=num(s, "MinLevelEffect"), radius=radius)


def effect_ids(s):
    out = []
    for i in range(1, 5):
        e = hexid(s.get(f"EffectID{i}"))
        if e:
            out.append(e)
    return out


# the workbook's item names against ours: as written, then as our records
# word the same item: no trailing " Effect" (an item's effect, named for it),
# " Mix Potion" as " Potion", no trailing " Potion", " Fire Potion" as
# " Cocktail" (the potions seed_potions.py made are named so)
ITEM_NAME_VARIANTS = [(r"\s+effect$", ""), (r"\s+mix\s+potion$", " potion"), (r"\s+potion$", ""),
                      (r"\s+fire\s+potion$", " cocktail"), (r"s$", "")]


def our_item(items_by_name, name):
    n = fold(name)
    if n in items_by_name:
        return items_by_name[n]
    for pat, to in ITEM_NAME_VARIANTS:
        v = re.sub(pat, to, n)
        if v != n and v in items_by_name:
            return items_by_name[v]
    return None


def item_ref(wb, items_by_name, e, id_col, count):
    """A created or consumed item: the workbook's name for it (its own ids are
    not ours: the Created Item Info sheet names them), and ours by that name."""
    out = {"count": count}
    sheet_id = num(e, id_col)
    name = wb.items.get(sheet_id) if isinstance(sheet_id, int) else None
    if name:
        out["name"] = name
        pid = our_item(items_by_name, name)
        if pid is not None:
            out["patternid"] = pid
    else:
        out["sheet_item_id"] = cell(e, id_col)
    return out


# a line's clause naming an item it makes or takes, as the server's readers
# take them (entity/potion.go's reSummonItem, spelltext.go's Reagent): "SELF
# Summon Draught", "Self, Summon Item: foraged herb", "Summons Unstable Potion
# X3", "Summon Unit of Blood x1" (Feed's); "Reagent Unit of Blood x2" (the
# vampires' abilities). "Summon Player" is a kind of its own, and a creature's
# summoning ("to your aid", a familiar) no item
LINE_TARGET = re.compile(r"(?i)^(?:self|target(?:\s+undead)?|groups?|pet)\b[\s,]*")
LINE_ITEM_WORDS = {
    "item": re.compile(r"(?i)^summ[ou]ns?\s+(?!player\b)(?:item\s*:?\s*)?(?:an?\s+|the\s+)?(.+?)(?:\s+x\s*\d+)?$"),
    "consume": re.compile(r"(?i)^reagent\s+(.+?)(?:\s+x\s*\d+)?$"),
}
LINE_CREATURE = re.compile(r"(?i)\b(?:to your aid|to aid|creature|familiar|familar)\b")
# a line for undead alone, and a line for its target that does not say so
UNDEAD_LINE = re.compile(r"(?i)^target\s+undead\b")


def undead_name(name):
    """entity/undead.go's readUndead: a harmful spell named "... Undead" is
    for undead alone."""
    return (name or "").strip().lower().endswith(" undead")


TARGET_LINE = re.compile(r"(?i)^target\b(?!\s+undead)")


def line_items(old, key):
    """The items a line names in clauses of that kind ("item" made, "consume" taken)."""
    out = []
    for c in old.split(","):
        c = LINE_TARGET.sub("", re.sub(r"\s*\([^)]*\)", "", c).strip()).strip()
        m = LINE_ITEM_WORDS[key].match(c)
        if m and not LINE_CREATURE.search(c):
            out.append(m.group(1).strip())
    return out


def settle_item_names(j, old, items_by_name, item_names, line_first=False, folds=None):
    """Each made ("item") or taken ("consume") item under the name the server
    finds it by (handlers' summonItem and the reagent check read an item by
    its name; the effects are all the server reads, the brief's contract):
    one that is an item of ours by the workbook's name (our_item: its
    "<X> Mix Potion" is our "<X> Potion", a food's "<X> effect" the food made
    for it) takes our item's own name; one the workbook names none of ours
    of, or names not at all (Neriak Cocktail's, Feed's Unit of Blood, the
    vampires' reagents), takes the one item
    of ours of that kind a spell of ours names in its line ("SELF Summon
    Grobb Cocktail", "Reagent Unit of Blood x2"). The workbook's name stays as
    sheet_name, its count as it is. line_first: the spell is a record of ours
    the map spells otherwise (one spell under two spellings, 2026-11-01), and
    makes the item its line names (the 2004 records' and EQOA-Data's) though
    the workbook's name finds another of ours (an item the import made for
    the workbook's record: the Transformative Pill, not the Creative Potion);
    that other is put in folds (the follow-up folds it into the line's,
    tools/fix_eqoaemu_followup2.py). Returns (renamed, by_line): what it
    did, for the report; an item left none of ours makes nothing or takes
    nothing."""
    renamed, by_line = [], []

    def walk(objs):
        for o in objs:
            yield o
            yield from walk(o.get("procs") or [])

    for key in LINE_ITEM_WORDS:
        unresolved, found = [], []
        for o in walk(j["effects"]):
            it = o.get(key)
            if it is None:
                continue
            nm = (it.get("name") or "").strip()
            pid = it.get("patternid")
            if pid is None and nm:
                pid = our_item(items_by_name, nm)
            if pid in item_names:
                found.append((it, nm, pid))
            else:
                unresolved.append(it)
        ours = [n for n in line_items(old, key) if our_item(items_by_name, n) in item_names]
        if line_first and key == "item" and len(ours) == 1 and len(found) == 1 and not unresolved:
            it, nm, pid = found[0]
            lid = our_item(items_by_name, ours[0])
            if pid != lid:
                # the workbook's name found another item of ours than the line's
                if folds is not None:
                    folds.append(dict(name=it.get("sheet_name") or nm, found=pid, line=lid))
                it.pop("patternid", None)
                found, unresolved = [], [it]
        for it, nm, pid in found:
            if it.get("name") != item_names[pid]:
                it["sheet_name"], it["name"], it["patternid"] = nm, item_names[pid], pid
                renamed.append(f"{nm!r} -> {item_names[pid]!r}")
        if len(unresolved) != 1 or len(ours) != 1:
            continue
        it, pid = unresolved[0], our_item(items_by_name, ours[0])
        nm = (it.get("sheet_name") or it.get("name") or "").strip()
        if nm:
            it["sheet_name"] = nm
        it["name"], it["patternid"] = item_names[pid], pid
        if not str(it.get("sheet_item_id") or "").strip():
            it.pop("sheet_item_id", None)
        by_line.append(f"{'makes' if key == 'item' else 'takes'} {item_names[pid]} x{it.get('count', 1)}"
                       + (f" (the workbook's {nm!r}, none of ours)" if nm else ""))
    return renamed, by_line


def effect_objects(e, ctx, wb, items_by_name, proc_of=None):
    """One Effects row as the brief's effect objects (one a figure; a row of
    one TYPE_KIND kind its object first). proc_of: "offense" or "defense"
    for a proc's own row."""
    t = cell(e, "TYPE")
    common = {"id": hexid(e.get("EffectID")), "type": t}
    absolute = t == "Statistic Absolute"
    objs = []

    def add(kind, **kw):
        o = dict(common)
        o["kind"] = kind
        o.update({k: v for k, v in kw.items() if v is not None})
        objs.append(o)
        return o

    kind = TYPE_KIND.get(t)
    if kind == "summon":
        pet = hexid(e.get("PetID"))
        p = {"sheet_pet_id": pet}
        if pet in wb.pets and cell(wb.pets[pet], "PetName"):
            p["name"] = cell(wb.pets[pet], "PetName")
        add("summon", pet=p)
    elif kind == "illusion":
        add("illusion", sprite=("0x" + cell(e, "SpriteID").upper().replace("0X", "")) if cell(e, "SpriteID") else None)
    elif kind == "create_item":
        add("create_item", item=item_ref(wb, items_by_name, e, "CreatedItemID", num(e, "QTY") or 1))
    elif kind == "teleport":
        add("teleport", location=cell(e, "Location") or None)
    elif kind == "cure":
        add("cure", removes=cell(e, "RemovesEffects") or None)
    elif kind == "resurrect":
        add("resurrect", rez_pct=num(e, "RezDebtRemoval"))
    elif kind == "invulnerable":
        add("invulnerable", mitigation=mitigation(e))
    elif kind in ("proc", "def_proc"):
        procs = []
        for k in ("ApplyProc1", "ApplyProc2"):
            pid = hexid(e.get(k))
            if pid and pid in wb.effects:
                pctx = dict(scope="", target_type="", duration=0, level=None, min_level=None, radius=0)
                for o in effect_objects(wb.effects[pid], pctx, wb, items_by_name,
                                        proc_of="offense" if kind == "proc" else "defense"):
                    procs.append(o)
        add(kind, procs=procs or None)
    elif kind == "speed":
        pass  # its figure below; a row with none is still a speed
    elif kind:
        add(kind)

    # the figures
    hp1, hp2, dot, pow_ = num(e, "HP1"), num(e, "HP2"), num(e, "DoT"), num(e, "POW")
    dur = num(e, "Duration") or 0
    drain = {"Drain": "drain", "Drain Group": "drain_group"}.get(t)
    if hp1:
        if drain:
            add(drain, amount=hp1)
        elif t == "Backstab":
            add("backstab", amount=hp1)
        elif t == "Dmg/Heal" and hp1 > 0 and 0 < dur <= HEAL_TICKS_UP_TO:
            add("hp_tick", amount=hp1)
        else:
            add("hp", amount=hp1, absolute=True if absolute else None)
    if hp2:
        add("hp2", amount=hp2)
    if dot:
        add("hp_tick", amount=-abs(dot), sheet_dot=dot if dot > 0 else None)
    if pow_:
        if drain:
            add(drain, stat="power", amount=pow_)
        else:
            add("power", amount=pow_, absolute=True if absolute else None)
    for col, stat in STAT_COLS:
        v = num(e, col)
        if v:
            if drain:
                add(drain, stat=stat, amount=v)
            else:
                add("stat_set" if absolute else "stat", stat=stat, amount=v)
    for col, k in (("HP Factors", "hp_factor"), ("OFF Mods", "off_mod"), ("DEF Mods", "def_mod")):
        v = num(e, col)
        if v:
            add(k, amount=v)
    speed = num(e, "SPEED")
    if speed:
        add("speed", amount=speed, absolute=True if absolute else None)
    elif kind == "speed":
        add("speed")
    if kind != "invulnerable" and (num(e, "Mitigation%") or num(e, "MitigationMax")):
        add("mitigation", mitigation=mitigation(e))
    hate = num(e, "Hate")
    if t == "Modify Hate" and hate:
        add("hate", amount=hate)
        hate = None
    if not objs:
        return []

    # what goes with the row's figures
    mod, pct = cell(e, "Modifier").lower(), num(e, "Percentage")
    lvl = num(e, "%") if cell(e, "Level Scaling?").lower() == "yes" else None
    resist = RESIST.get(cell(e, "Resist Basis"))
    for o in objs:
        if o["kind"] != "speed":
            if pct and mod in MODS:
                o["scale"], o["scale_pct"] = mod, pct
            if lvl:
                o["level_pct"] = lvl
        if resist:
            o["resist"] = resist
    if hate:
        objs[0]["hate"] = hate
    scope, ttype = cell(e, "Scope"), cell(e, "Target Type")
    radius, minlv, level = num(e, "AoERadius") or 0, num(e, "MinLevelEffect"), num(e, "Level")
    consume = num(e, "ConsumeItemQTY")
    for o in objs:
        if proc_of:
            o["on"] = proc_of
            if scope:
                o["scope"] = JSON_SCOPE.get(scope, scope.lower())
            if ttype:
                o["target_type"] = ttype
            if dur:
                o["duration_s"] = dur
            if radius:
                o["radius"] = radius
        else:
            # a row's 0 is its spell's time (a buff's stat rows mostly say 0)
            if dur and dur != ctx["duration"]:
                o["duration_s"] = dur
            if scope and scope != ctx["scope"]:
                o["scope"] = JSON_SCOPE.get(scope, scope.lower())
            if ttype and ttype != ctx["target_type"]:
                o["target_type"] = ttype
            if radius and radius != ctx["radius"]:
                o["radius"] = radius
            if minlv is not None and minlv != ctx["min_level"]:
                o["min_level"] = minlv
            if level is not None and ctx["level"] is not None and level != ctx["level"]:
                o["level"] = level
        if cell(e, "DisspellOnCast") == "1":
            o["dispel_on_cast"] = True
        if cell(e, "DisspellOnContact") == "1":
            o["dispel_on_contact"] = True
    if consume:
        objs[0]["consume"] = item_ref(wb, items_by_name, e, "ConsumeItemID", consume)
    return objs


def mitigation(e):
    m = {}
    if num(e, "Mitigation%") is not None:
        m["pct"] = num(e, "Mitigation%")
    if num(e, "MitigationMax") is not None:
        m["max"] = num(e, "MitigationMax")
    return m or None


def spell_json(s, wb, items_by_name):
    """A Spells-sheet row as the brief's JSON v1 (a dict), and the ids of its effect rows missing."""
    ctx = spell_ctx(s, wb)
    effects, missing = [], []
    for i in effect_ids(s):
        if i not in wb.effects:
            missing.append(i)
            continue
        effects += effect_objects(wb.effects[i], ctx, wb, items_by_name)
    scope = cell(s, "Scope")
    ttype = cell(s, "Target Type")
    out = {"v": 1, "scope": JSON_SCOPE.get(scope, scope.lower())}
    if ttype:
        out["target_type"] = ttype
    if scope not in JSON_SCOPE or scope == "Owner":
        out["sheet_scope"] = scope
    out["harmful"] = ttype.lower().startswith("enemy")
    dur = num(s, "Duration") or DURATIONS.get(cell(s, "Name").strip().lower())
    if dur:
        out["duration_s"] = dur
    out["stack"] = num(s, "StackType") or 0
    if num(s, "Level") is not None:
        out["level"] = num(s, "Level")
    if num(s, "MinLevelEffect") is not None:
        out["min_level"] = num(s, "MinLevelEffect")
    if ctx["radius"]:
        out["radius"] = ctx["radius"]
    if cell(s, "ReqEQuipment"):
        out["req_equipment"] = cell(s, "ReqEQuipment")
    if cell(s, "TYPE"):
        out["sheet_type"] = cell(s, "TYPE")
    if cell(s, "ArchType"):
        out["arch_type"] = cell(s, "ArchType")
    out["effects"] = effects
    fix = MADE_ERRATA.get(fold(cell(s, "Name")))
    if fix:
        for o in effects:
            if o.get("kind") == "create_item":
                o["item"] = made_erratum(o.get("item") or {}, fix, items_by_name)
    return out, missing


def made_erratum(item, name, items_by_name):
    """A create_item's item (as item_ref writes it) for the item a spell's row
    should name (MADE_ERRATA): the workbook's name for it, ours by that name."""
    out = {"count": item.get("count", 1), "name": name}
    pid = our_item(items_by_name, name)
    if pid is not None:
        out["patternid"] = pid
    return out


REAGENT = re.compile(r"(?i)\breagents?\s*:?\s+[^,;]+?\s+x\s*\d+")


def keep_line(j, old):
    """Why a spell we have keeps its line beside the workbook's effects: the
    line names what the server reads off it by name (a pet's template, an
    item made that the workbook names otherwise) or a place in prose (the
    gates' places are spellgates' rows); "" to write the new one."""
    if not old.strip():
        return ""
    kinds = {o["kind"] for o in j["effects"]}
    if "summon" in kinds:
        return "a summoning (the pet's name is the pets lane's)"
    if "teleport" in kinds:
        return "a teleport (its place is spellgates')"
    if any(o["kind"] == "create_item" and "patternid" not in o.get("item", {}) for o in j["effects"]):
        return "an item made that is none of ours by the workbook's name"
    return ""


def dumps(o):
    return json.dumps(o, separators=(",", ":"), ensure_ascii=False)


def mirror_json(j, level, hate):
    """An original's effects (JSON v1, a dict) mirrored for a spell of ours
    the workbook lacks (MIRRORS): its own level, its lowest receiver's as far
    below it as the original's, every proc of kind hate raising by hate, the
    original's effect ids and its rows' own levels (the workbook's rows of
    it) left out."""
    out = json.loads(json.dumps(j))
    if "level" in out:
        out["level"] = level
    if "min_level" in out and j.get("level") is not None:
        out["min_level"] = max(1, level - (j["level"] - j["min_level"]))
    for e in out.get("effects") or []:
        e.pop("id", None)
        e.pop("level", None)   # the original's row's own level
        for p in e.get("procs") or []:
            p.pop("id", None)
            p.pop("level", None)
            if p.get("kind") == "hate":
                p["amount"] = hate
    return out


def ours_changes(rec, now, want, j, note, item_names=None):
    """A spell of ours given effects of ours (MIRRORS, MAKES, CURES): its
    record's update (want: the columns it takes) and its effects row's (the
    effects j, their line, its source with note), each None where it is so
    already. rec its record, now its effects row."""
    want = {k: v for k, v in want.items() if v is not None}
    changed = {k: v for k, v in want.items() if rec.get(k) != v}
    record = (dict(before={k: rec.get(k) for k in changed}, set=changed) if changed else None)
    src = (now or {}).get("source") or "eqoa-data"
    after = dict(line=line_of(j, item_names=item_names), effects=dumps(j), source=src if note in src else f"{src}; {note}")
    before = {k: (now or {}).get(k) or "" for k in after}
    return record, (dict(before=before, set=after) if before != after else None)


def mirror_changes(rec, now, scope, frame, orig_name, orig_json, hate):
    """A spell of ours made the mirror of an original (MIRRORS): its record's
    update (scope and frame the original's, its description MIRROR_DESC) and
    its effects row's (the mirrored effects, their line, its source), each
    None where it is so already. rec its record, now its effects row; scope
    and frame the original's as written; orig_json its effects (a dict)."""
    return ours_changes(rec, now, {"scope": scope, "iconColor": frame, "spelldesc": MIRROR_DESC},
                        mirror_json(orig_json, rec["abilitylvl"], hate), MIRROR_NOTE.format(orig_name))


def makes_json(j, level, name, pid, count):
    """A maker's effects (JSON v1, a dict: the workbook's spell named in
    MAKES) for a spell of ours making count of our item pid: the maker's
    frame (a self spell's: its lowest receiver's level too), its own level,
    the original's effect ids, its rows' own levels and its item out."""
    out = json.loads(json.dumps(j))
    if "level" in out:
        out["level"] = level
    for e in out.get("effects") or []:
        e.pop("id", None)
        e.pop("level", None)   # the original's row's own level
        if e.get("kind") == "create_item":
            e["item"] = {"count": count, "name": name, "patternid": pid}
    return out


def cures_json(scope, level, n):
    """A cure of up to n harmful effects (CURES), in the workbook's cure
    frame (Minor Curative's), self or a friendly target as scope (the
    record's: 0 self) has it."""
    s, t = ("self", "Self") if scope == 0 else ("target", "Friendly")
    return {"v": 1, "scope": s, "target_type": t, "harmful": False, "stack": 0, "level": level, "min_level": 1,
            "req_equipment": "Any", "sheet_type": "Remove Ability",
            "effects": [{"type": "Remove Ability", "kind": "cure", "removes": "", "cure_max": n}]}


def with_duration(effects, line, seconds):
    """A spell's effects (JSON text) and line as this import wrote them with
    no time, as it writes them with one (DURATIONS): duration_s after
    "harmful", where spell_json puts it, and the line ending in its time, as
    line_of ends it; None for effects it did not write so (a GM's or another
    tool's). The live change (tools/fix_eqoaemu_places.py) writes these, so
    a database the import ran on before DURATIONS matches a fresh import.
    line None keeps the line (one this import did not write)."""
    try:
        j = json.loads(effects)
    except ValueError:
        return None
    if not isinstance(j, dict) or dumps(j) != effects or "harmful" not in j or "duration_s" in j or \
            any(o.get("duration_s") for o in j.get("effects") or []):
        return None
    out = {}
    for k, v in j.items():
        out[k] = v
        if k == "harmful":
            out["duration_s"] = seconds
    if line is not None and line.strip():
        line += ", " + fmt_dur(seconds)
    return dumps(out), line


# ---- the line, in EQOA-Data's words ----------------------------------------------

STAT_WORD = {"str": "STR", "sta": "STA", "agi": "AGI", "dex": "DEX", "wis": "WIS", "int": "INT", "cha": "CHA",
             "ac": "AC", "maxhp": "Maximum HP", "maxpower": "Maximum Power", "fr": "FR", "cr": "CR", "lr": "LR",
             "pr": "PR", "dr": "DR", "ar": "AR", "pierce": "Pierce Resist", "blunt": "Blunt Resist",
             "slash": "Slash Resist", "ranged": "Ranged Resist", "hot": "HP per tic", "pot": "Power per tic",
             "power": "Power"}
ELEMENT = {"fr": "fire", "cr": "cold", "lr": "lightning", "pr": "poison", "dr": "disease", "ar": "arcane",
           "blunt": "blunt", "pierce": "piercing", "slash": "slashing"}


def fmt_dur(s):
    if not s:
        return ""
    if s >= 60 and s % 60 == 0:
        return f"{s // 60}min"
    return f"{s}sec"


def by(verb_up, verb_down, what, n):
    return f"{verb_up if n >= 0 else verb_down} {what} by {abs(n)}"


def scale_words(o):
    w = ""
    if o.get("scale"):
        w += f" ; Increase {o['scale_pct']}% based on {o['scale'].upper()}"
    return w


def clause(o, spell):
    """An effect object's words (a list of clauses), as spelltext.go reads them."""
    k, n = o["kind"], o.get("amount")
    own = o.get("scope") == "self" and spell["scope"] != "self"
    tail = " to Self" if own else ""
    if k == "hp":
        return [by("Increase", "Decrease", "Hitpoints", n) + tail + scale_words(o)]
    if k == "hp2":
        return [by("Increase", "Decrease", "Hitpoints", n) + scale_words(o)]
    if k == "hp_tick":
        return [by("Increase", "Decrease", "HP per tic", n) + tail + scale_words(o)]
    if k == "power":
        if o.get("absolute"):
            return [f"Set Power to {n}"]
        return [by("Increase", "Decrease", "Power", n) + tail + scale_words(o)]
    if k == "stat":
        return [by("Increase", "Decrease", STAT_WORD[o["stat"]], n) + tail + scale_words(o)]
    if k == "stat_set":
        return [f"Set {STAT_WORD[o['stat']]} to {n}"]
    if k == "speed":
        if n is None:
            return ["Sow"]
        if n < 0 and o.get("absolute"):
            return ["Root"]
        return [by("Increase", "Decrease", "Movement Speed", n)]
    if k == "hate":
        return [by("Increase", "Decrease", "Hate", n) + scale_words(o)]
    if k in ("drain", "drain_group"):
        group = k == "drain_group"
        st = o.get("stat")
        if not st:
            return [f"Drain {'Group ' if group else ''}HP by {abs(n)}" + ("" if group else " give self") + scale_words(o)]
        if st == "power":
            return [f"Drain {'Group ' if group else ''}Power by {abs(n)}" + scale_words(o)]
        return [f"Decrease {STAT_WORD[st]} by {abs(n)}" + scale_words(o), "give to group" if group else "give to self"]
    if k == "backstab":
        return [by("Increase", "Decrease", "HP via Backstab", n) + scale_words(o)]
    if k == "summon":
        name = (o.get("pet") or {}).get("name")
        return [f"Summons {article(name)} to aid you" if name else "Summons a pet"]
    if k == "create_item":
        # our item's own name, which the summon reads (potion.go); one that
        # is none of ours in words no summon takes
        it = o.get("item") or {}
        ours = (spell.get("item_names") or {}).get(it.get("patternid"))
        times = f" x{it['count']}" if it.get("count", 1) > 1 else ""
        if ours:
            return [f"Summon {ours}{times}"]
        return [f"Creates {it.get('name') or 'an item'}{times}"]
    if k == "illusion":
        return ["Gives Form"]
    if k == "teleport":
        return [f"Teleport to {o.get('location') or 'a place'}".replace(",", " ")]
    if k == "gate":
        return ["Return to bind point"]
    if k == "cure":
        if o.get("cure_max"):
            return [f"Cure up to {o['cure_max']} {o.get('removes') or 'harmful effects'}".replace(",", " ")]
        return [f"Cure {o.get('removes') or 'effects'}".replace(",", " ")]
    if k == "resurrect":
        return [f"Ressurect Player: restore {abs(o.get('rez_pct') or 0)}% experience debt"]
    if k == "charm":
        return ["Charm" + scale_words(o)]
    if k == "invis":
        return ["Invis"]
    if k == "lull":
        return ["Lull"]
    if k == "breath":
        return ["Enduring Breath"]
    if k == "hide":
        return ["Invis"]  # the server's nearest: unseen (the old lines' word for it)
    if k == "pickpocket":
        return ["Pickpocket"]
    if k == "faction":
        return ["Faction Change"]
    if k == "summon_player":
        return ["Summon Player"]
    if k in ("invulnerable", "mitigation"):
        m = o.get("mitigation") or {}
        return [f"Mitigate {m.get('pct', 0)}% of melee damage up to {abs(m.get('max', 0))}"]
    if k in ("hp_factor", "off_mod", "def_mod"):
        return [by("Increase", "Decrease", {"hp_factor": "HP Factor", "off_mod": "Offensive Mod",
                                            "def_mod": "Defensive Mod"}[k], n)]
    if k in ("proc", "def_proc"):
        return proc_words(o, spell)
    return []


def proc_words(o, spell):
    """A proc's words: a defensive proc that hurts its attacker is a damage
    shield (the workbook's defensive procs land at every hit, as dshield.go's
    shields do); an offensive one on a friend whose description says proc is
    its hit (ParseSpell makes it a proc); anything else in words no reader
    takes for a hit."""
    hits = [p for p in o.get("procs") or [] if p["kind"] == "hp" and (p.get("amount") or 0) < 0]
    if o["kind"] == "def_proc" and hits:
        p = hits[0]
        out = [f"Damage Shield Decrease Hitpoints by {abs(p['amount'])}" + scale_words(p)]
        for q in (o.get("procs") or []):
            if q is not p:
                words = clause(q, {"scope": "", "harmful": True})
                if words:
                    out.append("Add Defensive Proc: " + " ".join(words).replace(";", "") + " when hit")
        return out
    if o["kind"] == "proc" and spell.get("proc_desc") and hits and not spell["harmful"]:
        p = hits[0]
        return [f"Decrease Hitpoints by {abs(p['amount'])}" + scale_words(p)]
    out = []
    for p in o.get("procs") or []:
        words = clause(p, {"scope": "", "harmful": True})
        if words:
            out.append(("Add Offensive Proc: " if o["kind"] == "proc" else "Add Defensive Proc: ") +
                        " ".join(words).replace(";", "") + (" when it hits" if o["kind"] == "proc" else " when hit"))
    return out or [("Add Offensive Proc" if o["kind"] == "proc" else "Add Defensive Proc")]


def article(name):
    n = name.strip()
    if re.match(r"^(a|an|the)\s", n, re.I):
        return n
    return ("an " if n[:1].lower() in "aeiou" else "a ") + n


def line_of(j, proc_desc=False, record_scope=None, item_names=None):
    """The spell's line in EQOA-Data's words (see the doc): its first word the
    record's scope, whom the server lands it on."""
    spell = dict(scope=j["scope"], harmful=j["harmful"], proc_desc=proc_desc, item_names=item_names or {})
    effects = j["effects"]
    if not effects:
        return ""
    head = {"self": "Self", "target": "Target", "group": "Group", "pet": "Pet", "target_aoe": "Target",
            "player_aoe": "Self"}.get(j["scope"], "Target")
    if record_scope is not None:
        head = {0: "Self", 1: "Target", 2: "Group", 3: "Pet"}.get(record_scope, head)
    if j.get("target_type", "").lower().endswith("undead"):
        head = "Target undead"
    if len(effects) == 1 and effects[0]["kind"] == "summon":
        return clause(effects[0], spell)[0]
    parts = []
    hp_at = None
    for o in effects:
        words = clause(o, spell)
        if o["kind"] == "hp2" and j["scope"] == "target_aoe" and hp_at is not None:
            parts[hp_at] += " aoe " + words[0]
            continue
        if o["kind"] == "hp" and hp_at is None:
            hp_at = len(parts)
        parts += words
    # its area, in the words area.go reads (spellarea's row says it too)
    if {o["kind"] for o in effects} & OWN_TARGETS:
        pass
    elif j["scope"] == "group" and j.get("radius"):
        parts.append(f"Radius {j['radius']}")
    elif j["scope"] == "player_aoe" or (j["scope"] == "self" and j.get("radius")):
        parts.append("PB AE")
    elif (j["scope"] == "target_aoe" or (j["scope"] == "target" and j.get("radius"))) and \
            not any(" aoe " in p for p in parts):
        parts.append("AOE")
    # a reagent the cast takes (vampire.go reads "Reagent <item> xN")
    for o in effects:
        c = o.get("consume") or {}
        if c.get("patternid") in (item_names or {}):
            parts.append(f"Reagent {item_names[c['patternid']]} x{c.get('count', 1)}")
    d = j.get("duration_s") or max([o.get("duration_s") or 0 for o in effects] + [0])
    line = head + ", " + ", ".join(p for p in parts if p)
    if d:
        line += ", " + fmt_dur(d)
    return line


# ---- a new spell's description ----------------------------------------------------

def desc_of(j, cm, name="", were_quest="", were_form=False):
    """A sentence or two of what a new spell does, in the style of ours (the
    workbook has none): never the words the line's reader takes from a
    description ("proc" but for a weapon proc, "undead" but for an undead
    spell, "the form of" but for an illusion). A were form is "Assume the
    form of the <name>." as ours are; a were quest's reward names its were
    and archetype first ("Werewolf caster ability."), as ours do ("Wererat
    Melee Ability - Debuff AC"), which is what IsWereAbility reads."""
    if were_form or re.fullmatch(r"(?i)(rat|lion|wolf|bear|gator)were", (name or "").strip()):
        return f"Assume the form of the {re.sub(r'(?i)\s+form$', '', name.strip()).lower()}."
    lead = ""
    if were_quest:
        parts = were_quest.split("/")
        kind = WERE_KIND.get(parts[1] if len(parts) > 1 else "", "")
        arch = re.sub(r"^(?:L\d+|trials|ability|p\d+)-?", "", parts[-1]) if len(parts) > 2 else ""
        if kind:
            lead = f"{kind} {arch + ' ' if arch else ''}ability. "
    whom = {"self": "you", "target": "the target", "group": "your group", "pet": "your pet",
            "target_aoe": "the target and those near it", "player_aoe": "you and those near you"}.get(j["scope"], "the target")
    whose = {"self": "your", "target": "the target's", "group": "your group's", "pet": "your pet's",
             "target_aoe": "the target's and its neighbours'", "player_aoe": "your and your neighbours'"}.get(j["scope"], "the target's")
    if j.get("target_type") == "Owner" or j.get("target_type") == "Owner/Self":
        whom, whose = "its owner", "its owner's"
    said, scaled, resist = [], set(), None
    ups = [o for o in j["effects"] if o["kind"] in ("stat", "stat_set") and (o.get("amount") or 0) > 0]
    downs = [o for o in j["effects"] if o["kind"] in ("stat", "stat_set") and (o.get("amount") or 0) < 0]

    def stats(objs):
        names = []
        for o in objs:
            w = {"maxhp": "maximum hit points", "maxpower": "maximum power", "hot": "health regeneration",
                 "pot": "power regeneration", "ac": "armor"}.get(o["stat"], STAT_WORD.get(o["stat"], o["stat"]))
            if w not in names:
                names.append(w)
        return ", ".join(names[:-1]) + (" and " if len(names) > 1 else "") + names[-1]

    for o in j["effects"]:
        k, n = o["kind"], o.get("amount") or 0
        if o.get("scale"):
            scaled.add(o["scale"].upper())
        if o.get("resist") and j["harmful"]:
            resist = resist or o["resist"].upper()
        if k in ("hp", "hp2") and n < 0:
            el = ELEMENT.get(o.get("resist", ""), "")
            said.append(f"{'An' if el[:1] in ('a', 'e', 'i', 'o', 'u') else 'A'} {el + ' ' if el else ''}strike at {whom}."
                        if j["harmful"] else f"Costs {whom} hit points.")
        elif k in ("hp", "hp2") and n > 0:
            said.append(f"Heals {whom}.")
        elif k == "hp_tick":
            said.append(f"Damages {whom} over time." if n < 0 else f"Heals {whom} over time.")
        elif k == "power":
            said.append(f"Restores {whose} power." if n > 0 else f"Drains {whose} power.")
        elif k in ("drain", "drain_group"):
            said.append("Drains the target's life force, healing the caster's group." if k == "drain_group" and not o.get("stat")
                        else "Drains the target's life force, dealing damage and restoring health to the caster."
                        if not o.get("stat") else f"Drains the target's {STAT_WORD[o['stat']]} and gives it to the caster.")
        elif k == "hate":
            said.append("Allows you to gain the attention of your enemy." if n > 0 else "Lowers your enemy's hate for you.")
        elif k == "speed":
            said.append("Roots the target in place." if n < 0 and o.get("absolute") else
                        f"Slows {whom}." if n < 0 else f"Increases {whose} movement speed.")
        elif k == "summon":
            nm = (o.get("pet") or {}).get("name")
            said.append(f"Summons {article(nm)} to aid you." if nm else "Summons a pet to aid you.")
        elif k == "create_item":
            it = o.get("item") or {}
            said.append(f"Creates {it.get('count', 1)} {it.get('name') or 'items'}.")
        elif k == "illusion":
            said.append(f"Changes {whose} appearance.")
        elif k == "teleport":
            said.append(f"Teleports {whom} away.")
        elif k == "gate":
            said.append("Returns you to your bind point.")
        elif k == "cure":
            said.append(f"Removes harmful effects from {whom}.")
        elif k == "resurrect":
            said.append(f"Revives a fallen ally, restoring {abs(o.get('rez_pct') or 0)}% of the experience lost.")
        elif k == "charm":
            said.append("Charms an enemy to aid you.")
        elif k == "lull":
            said.append("Calms the target.")
        elif k == "invis":
            said.append(f"Makes {whom} invisible.")
        elif k == "breath":
            said.append(f"Lets {whom} breathe underwater.")
        elif k == "hide":
            said.append("Hides you from sight.")
        elif k == "pickpocket":
            said.append("Steals from the target.")
        elif k == "backstab":
            said.append("A strike from behind.")
        elif k == "faction":
            said.append("Changes how others regard you.")
        elif k == "summon_player":
            said.append("Calls a group member to you.")
        elif k in ("invulnerable", "mitigation"):
            said.append(f"Shields {whom} from melee damage.")
        elif k == "proc":
            said.append(f"Gives {whose} attacks a chance to deal additional damage." if not j["harmful"]
                        else f"Strikes {whom} each time it hits.")
        elif k == "def_proc":
            said.append(f"Damages those who strike {whom}." if any((p.get("amount") or 0) < 0 for p in o.get("procs") or [])
                        else f"Aids {whom} when struck.")
        elif k in ("off_mod", "def_mod", "hp_factor"):
            said.append({"off_mod": "Raises offense.", "def_mod": "Raises defense.", "hp_factor": "Raises hit points."}[k]
                        if n > 0 else {"off_mod": "Lowers offense.", "def_mod": "Lowers defense.", "hp_factor": "Lowers hit points."}[k])
    if ups:
        said.insert(0, f"A buff that increases {whose} {stats(ups)}.")
    if downs:
        said.insert(0, f"Lowers {whose} {stats(downs)}.")
    seen, out = set(), []
    for x in said:
        if x not in seen:
            seen.add(x)
            out.append(x)
    text = " ".join(out[:2]) or "A spell."
    if scaled:
        text += " Modified by " + ", ".join(sorted(scaled)) + "."
    if resist:
        text += f" Resisted by {resist}."
    if cm:
        text = "CM: " + text
    if j.get("target_type", "").lower().endswith("undead") and "undead" not in text.lower():
        text += " Works on undead alone."
    text = re.sub(r"\bproc\b", "", text) if "chance to deal additional" not in text else text
    return (lead + text)[:127]


# ---- the plan -------------------------------------------------------------------

def exact_key(name):
    """handlers/scribe.go's exactKey: spell_key with its "+" kept."""
    n = (name or "").strip()
    if n.endswith("$"):
        n = n[:-1].rstrip()
    n = re.sub(r"\s*\((?:[A-Za-z]{2,3}(?:\s*[,/]\s*)?)+\)\s*$", "", n)
    return " ".join(n.replace("`", "'").lower().split())


def scroll_spell(spells, scroll, level, has=None):
    """handlers/scribe.go's scrollSpellFor over (spellid, name, level) in id
    order: its exact name's records, else spell_key's; the first its class
    has (has(spellid)) at the level, else the first its class has, else the
    first at the level, else the first. has None counts no class."""
    for same in (lambda n: exact_key(n) == exact_key(scroll), lambda n: spell_key(n) == spell_key(scroll)):
        best, rank = None, 0
        for sid, name, lv in spells:
            if not same(name):
                continue
            r = 1 + (lv == level) + (2 if has is not None and has(sid) else 0)
            if r > rank:
                best, rank = sid, r
        if best is not None:
            return best
    return None


def item_ids(text):
    """The item ids of a quest's items column ("5*2, 7")."""
    return [int(m) for m in re.findall(r"(?:^|,)\s*(\d+)", text or "")]


def quest_rewards(db):
    """(item ids, spell ids) every quest step, choice and class reward gives."""
    items, spells = set(), set()
    if not has_table(db, "queststeps"):
        return items, spells
    cols = columns(db, "queststeps")
    want = [c for c in ("rewarditems", "spells", "byclass", "choices") if c in cols]
    for row in db.execute(f"SELECT {', '.join(want)} FROM queststeps"):
        r = dict(zip(want, row))
        items.update(item_ids(r.get("rewarditems")))
        spells.update(int(x) for x in re.findall(r"\d+", r.get("spells") or ""))
        for k in ("byclass", "choices"):
            try:
                v = json.loads(r.get(k) or "null")
            except ValueError:
                continue
            opts = list(v.values()) if k == "byclass" and isinstance(v, dict) else (v or {}).get("options", []) if isinstance(v, dict) else []
            for o in opts:
                if isinstance(o, dict):
                    items.update(item_ids(str(o.get("items") or "")))
                    spells.update(int(x) for x in re.findall(r"\d+", str(o.get("spells") or "")))
    return items, spells


def scroll_curve(db):
    """Sandstorm's scroll prices by level (the scrolls import_sandstorm_harvest.py
    wrote, else every scroll a merchant sells), as stock_merchants.py's Rule."""
    from stock_merchants import Rule
    q = ("SELECT levelreq, itemcost FROM itempattern WHERE itemicon = ? AND equipslot = -1 AND itemcost > 0 "
         "AND levelreq > 0")
    rows = []
    if has_table(db, "sandstorm_import_rows"):
        rows = db.execute(q + " AND patternid IN (SELECT key FROM sandstorm_import_rows WHERE tbl = 'itempattern')",
                          (SCROLL_ICON,)).fetchall()
    if not rows:
        rows = db.execute(q + " AND patternid IN (SELECT patternid FROM npcitem)", (SCROLL_ICON,)).fetchall()
    curve = Rule.scroll_curve([{"level": lv, "cost": c} for lv, c in rows])
    return lambda level: max(1, round(Rule.on_curve(curve, level) or 1)) if curve else 1


def rename_safe(old, new, db=None):
    """Whether a record may take the workbook's spelling: every lookup of a
    spell by its name reads both alike. scribe.go's exactKey and spellKey and
    the quest importer's fold are blind to case, spacing and a backtick for an
    apostrophe; database.SpellGateKey, pettemplate's and the class templates'
    keys and the Go code's names (area.go, illusion.go, spellnotes.go...) to
    case. A rename that changes more than case and a backtick would need
    every one of those carried; none of the workbook's does (2026-11-01)."""
    if exact_key(old) != exact_key(new):
        return False
    if old.strip().lower() == new.strip().lower() or db is None:
        return True
    # a backtick read as an apostrophe is not case alone: the keys blind only
    # to case must hold no row of the old name
    lo = old.strip().lower()
    for table, col in (("spellgates", "spell"), ("pettemplate", "spell")):
        if has_table(db, table) and db.execute(f"SELECT 1 FROM main.{table} WHERE lower(trim({col})) = ?", (lo,)).fetchone():
            return False
    if has_table(db, "classtemplates") and "abilities" in columns(db, "classtemplates"):
        for (a,) in db.execute("SELECT abilities FROM main.classtemplates"):
            if lo in (a or "").lower():
                return False
    return True


def pick_owner(rows, rec, linked, sandstorm_classes=()):
    """Of several workbook rows mapped onto one record, the record's own: a
    row of a class the record is linked to today (classspells), else its
    level, a class Sandstorm's dumps or vendors gave it (classspells dump or
    vendor), its scope, the lowest SpellID. The others are split off
    (split_map)."""
    def score(s):
        cls = set(class_mask(cell(s, "CLASS")))
        return (bool(cls & linked), num(s, "Level") == rec["abilitylvl"], bool(cls & set(sandstorm_classes)),
                SCOPE.get(cell(s, "Scope")) == rec["scope"], -int(hexid(s.get("SpellID")) or "0", 16))
    return max(rows, key=score)


def split_map(db, wb, map_path, out_path):
    """The map with each row that shares a record with the record's own row
    given its own: spellid 900000 + its SpellID, how "split", split_from the
    record (the coordinator, 2026-11-01: "workbook wins, so split them").
    Returns the rows split, as (SpellID, name, classes, from, to)."""
    smap = read_map(map_path)
    records = {r[0]: (r[1], r[2]) for r in db.execute("SELECT spellid, abilitylvl, scope FROM main.spellpattern")}
    linked, ss = collections.defaultdict(set), collections.defaultdict(set)
    for c, sid, src in db.execute("SELECT class, spellid, source FROM main.classspells"):
        linked[sid].add(c)
        if src in ("dump", "vendor"):
            ss[sid].add(c)
    by_sid = collections.defaultdict(list)
    for s in wb.spells:
        m = map_entry(smap, hexid(s.get("SpellID")))
        if m and m[1].split(" ")[0] not in ("new", "split") and m[0] in records:
            by_sid[m[0]].append(s)
    split = {}
    for sid, rows in by_sid.items():
        if len(rows) < 2:
            continue
        lvl, scope = records[sid]
        own = pick_owner(rows, {"abilitylvl": lvl, "scope": scope}, linked[sid], ss[sid])
        for r in rows:
            if r is not own:
                hx = hexid(r.get("SpellID"))
                split[hx] = (NEW_BASE + int(hx, 16), sid)
    out, done = [], []
    with open(map_path, encoding="utf-8", newline="") as f:
        for r in csv.reader(f, delimiter="\t", quoting=csv.QUOTE_NONE):
            if r and r[0] == "sheet_id":
                r = r[:7] + ["split_from"]
            elif r and not r[0].startswith("#") and len(r) >= 3:
                hx = r[0].strip().upper()
                r = (r + [""] * 8)[:8]
                if hx in split:
                    to, frm = split[hx]
                    done.append((hx, r[3], r[4], frm, to))
                    r[1], r[2], r[7] = str(to), "split", str(frm)
            out.append(r)
    with open(out_path, "w", encoding="utf-8", newline="") as f:
        w = csv.writer(f, delimiter="\t", quoting=csv.QUOTE_NONE, lineterminator="\n", escapechar="\\")
        for r in out:
            w.writerow(r)
    return done


def plan(db, wb, smap, disc=None, link_existing=False, weres=None, icons=None, bodies=None):
    """Everything --apply would write, and the report's lists. weres: the
    were quests' reward spells (were_rewards); icons: the disc's (disc_icons),
    which IconID's byte order is read by (icon_of); bodies: the disc's
    CSprites (read_bodies), a use's body no CSprite has listed and not
    written."""
    weres = weres or {}
    rcols = columns(db, "spellpattern")
    records = {r[0]: dict(zip(rcols, r)) for r in db.execute(f"SELECT {', '.join(rcols)} FROM main.spellpattern")}
    ecols = columns(db, "spelleffect")
    erows = {r[0]: dict(zip(ecols, r)) for r in db.execute(f"SELECT {', '.join(ecols)} FROM main.spelleffect")}
    areas = {}
    if has_table(db, "spellarea"):
        acols = columns(db, "spellarea")
        areas = {r[0]: dict(zip(acols, r)) for r in db.execute(f"SELECT {', '.join(acols)} FROM main.spellarea")}
    linked = collections.defaultdict(set)    # spellid -> classes
    class_keys = collections.defaultdict(set)  # class -> spell keys it has
    for c, sid in db.execute("SELECT class, spellid FROM main.classspells"):
        linked[sid].add(c)
        if sid in records:
            class_keys[c].add(spell_key(records[sid]["spellname"]))
    sandstorm = set()
    if has_table(db, "sandstorm_import_rows"):
        sandstorm = {r[0] for r in db.execute("SELECT key FROM sandstorm_import_rows WHERE tbl = 'spellpattern'")}
    items_by_name, item_names = {}, {}
    for pid, name, icon, slot in db.execute("SELECT patternid, itemname, itemicon, equipslot FROM main.itempattern "
                                            "ORDER BY patternid"):
        if not (icon == SCROLL_ICON and slot == -1):  # never a spell's scroll, as the server's lookup
            items_by_name.setdefault(fold(name), pid)
        item_names[pid] = name
    lists = collections.defaultdict(list)
    counts = collections.Counter()
    foods = plan_foods(db, wb, items_by_name, item_names, lists, counts)
    made = MadeItems(db, wb, items_by_name, item_names, lists, counts, icons, foods, bodies)

    by_sid = collections.defaultdict(list)
    split_of = {}   # a split row's new spellid -> the record it leaves
    # the records the workbook's errata touch: a row two_spells cut (first), its
    # second spell's (second), a description put right (description)
    errata = {"first": set(), "second": set(), "description": set()}
    for cut in getattr(wb, "errata", []):
        lists["a workbook row carrying a second spell's effects, made two (TWO_SPELLS)"].append(
            f"{cut[0]} {cut[1]}: {', '.join(cut[3])} -> {cut[2]}")
    for s in wb.spells:
        sid_hex = hexid(s.get("SpellID"))
        if not sid_hex and s.get(ERRATUM_OF):
            # the second spell of a row two_spells cut: the record of ours of its name
            mine = [sid for sid, r in records.items() if exact_key(r["spellname"]) == exact_key(cell(s, "Name"))]
            if len(mine) != 1:
                lists["a workbook erratum's second spell: none of ours, or several, of its name (left)"].append(
                    f"{s[ERRATUM_OF]} {cell(s, 'Name')}: {mine or 'none'}")
                continue
            by_sid[mine[0]].append(s)
            errata["second"].add(mine[0])
            m = map_entry(smap, s[ERRATUM_OF])
            if m is not None:
                errata["first"].add(m[0])
            continue
        if not sid_hex:
            continue
        if sid_hex not in smap:
            lists["not in the map"].append(f"{sid_hex} {cell(s, 'Name')}")
            continue
        sid, how, frm = map_entry(smap, sid_hex)
        # the map's words: "new" (a lane's "new (pets lane)" too), "split", else a record of ours
        how = how.split(" ")[0] if how.split(" ")[0] in ("new", "split") else how
        if how == "split" and sid in records:
            how = "made"  # a plan made again after its own --apply: the record is ours now
        if how not in ("new", "split") and sid not in records:
            lists["the map names a record we lack"].append(f"{sid_hex} {cell(s, 'Name')} -> {sid}")
            continue
        by_sid[sid].append(s)
        if how == "split":
            split_of[sid] = frm

    spells, effects, area_rows, links = [], [], [], []
    splits, gates = [], []
    folds = []      # an item the workbook's name found where a record the map spells otherwise makes another
    new_rows = {}   # spellid -> (sheet row, json, a were's) of a new spell
    written = {}    # spellid -> (sheet row, json) of every record a row writes (the mirrors' originals)
    for sid, rows in sorted(by_sid.items()):
        rec = records.get(sid)
        s = rows[0] if len(rows) == 1 or rec is None else pick_owner(rows, rec, linked[sid])
        for o in rows:
            if o is not s:
                lists["several rows on one record: not applied (the map's to settle)"].append(
                    f"{hexid(o.get('SpellID'))} {cell(o, 'Name')} {cell(o, 'CLASS')} L{cell(o, 'Level')} -> {sid} "
                    f"(set from {hexid(s.get('SpellID'))} {cell(s, 'CLASS')} L{cell(s, 'Level')})")
        j, missing = spell_json(s, wb, items_by_name)
        written[sid] = (s, j)
        for m in missing:
            lists["an effect the Effects sheet lacks"].append(f"{hexid(s.get('SpellID'))} {cell(s, 'Name')}: {m}")
        # the items it makes under the names the server finds them by, ours
        # or made (MadeItems), before its description and its line name them
        renamed, by_line = [], []
        # one spell under two spellings, the map's row on ours (how "spelled
        # otherwise"): its item is the one its line names, which takes the
        # workbook's use, as the record takes the workbook's record
        otherwise = rec is not None and (map_entry(smap, hexid(s.get("SpellID"))) or (0, ""))[1].startswith(
            "spelled otherwise")
        if j["effects"]:
            old_line = ((erows.get(sid) or {}).get("line") or "") if rec is not None else ""
            n = len(folds)
            renamed, by_line = settle_item_names(j, old_line, items_by_name, item_names, otherwise, folds)
            for f in folds[n:]:
                f["spellid"] = sid
                lists["an item the workbook names that a spell of ours makes as its line names another (the line's "
                      "kept; tools/fix_eqoaemu_followup2.py folds a made one into it)"].append(
                    f"{sid} {rec['spellname']}: the workbook's {f['name']!r} ({f['found']} "
                    f"{item_names.get(f['found'])}) is {f['line']} {item_names.get(f['line'])}")
            made.settle(s, j, sid, replace=otherwise)
        sheet_scope = cell(s, "Scope")
        scope = SCOPE.get(sheet_scope)
        rez = any(o["kind"] == "resurrect" for o in j["effects"])
        cm = cell(s, "Learn Method") == "CM"
        classes = class_mask(cell(s, "CLASS"))
        quest = weres.get(quest_fold(cell(s, "Name")))
        # a body every class may take is a were's form: the were quests are
        # any class's, and the forms past the first their masteries (were.go)
        were_form = any(o["kind"] == "illusion" for o in j["effects"]) and len(set(classes)) == len(CLASSES)

        # the record
        want = {}
        if num(s, "Level") is not None:
            want["abilitylvl"] = num(s, "Level")
        for col, k in (("casttime", "CastTime"), ("recast", "Recast"), ("power", "PowerCost")):
            if num(s, k) is not None:
                want[col] = int(num(s, k))
        if cell(s, "Name").strip().lower() in RECASTS:
            want["recast"] = RECASTS[cell(s, "Name").strip().lower()]
        summon = any(o["kind"] == "summon" for o in j["effects"])
        if scope is not None and not (rez and rec is not None):
            # a rez is cast at a player, a pet made beside its caster (the doc)
            want["scope"] = 1 if rez else 0 if summon else scope
        new_scope = want.get("scope", rec["scope"] if rec else 1)
        r = num(s, "Range")
        ours = range_of(rec["range"]) if rec else None
        if new_scope == 0:
            # a self spell's is 4.5, the records' convention (Sandstorm's own kept)
            if rec is None or (sid not in sandstorm and ours != TOUCH):
                want["range"] = range_bits(TOUCH)
        elif r is not None and r > 1:
            want["range"] = range_bits(r)
        elif r is not None:
            # the workbook's 1 (or 0): touch
            if new_scope == 1 and (rec is None or (sid not in sandstorm and ours != TOUCH)):
                want["range"] = range_bits(TOUCH)
            elif rec is None:
                want["range"] = range_bits(FAR)
        elif rec is None:
            # no range given: the records' convention for its kind
            want["range"] = range_bits(FAR if rez or new_scope in (2, 3) else TOUCH)
        if rec is None:
            icon = icon_of(cell(s, "IconID"), icons)
            frame = FRAMES.get(cell(s, "IconBGDColor"))
            eq = equip_mask(cell(s, "ReqEQuipment"))
            if icon is None:
                lists["a new spell with no icon (0)"].append(f"{hexid(s.get('SpellID'))} {cell(s, 'Name')}")
            if eq is None:
                lists["ReqEQuipment not read (-1)"].append(f"{hexid(s.get('SpellID'))} {cell(s, 'ReqEQuipment')}")
            row = dict(spellid=sid, abilitylvl=want.get("abilitylvl", 1), unk2=0, unk3=0, range=want["range"],
                       casttime=want.get("casttime", 0), power=want.get("power", 0),
                       iconColor=frame if frame is not None else FRAMES["Red"], icon=icon or 0, scope=new_scope,
                       recast=want.get("recast", 0), eqprequire=-1 if eq is None else eq,
                       spellname=cell(s, "Name"), spelldesc="")
            proc_desc = any(o["kind"] == "proc" for o in j["effects"]) and not j["harmful"]
            row["spelldesc"] = desc_of(j, cm, row["spellname"], quest[1] if quest else "", were_form)
            spells.append(dict(spellid=sid, action="insert", sheet_id=hexid(s.get("SpellID")), name=row["spellname"],
                               row=row))
            new_rows[sid] = (s, j, False)
            counts["records made"] += 1
        else:
            # what makes the record's line a proc today: its description's
            # words, or (enchanter.go) its name, which an old line of a plain
            # hit on a friend shows
            old = (erows.get(sid) or {}).get("line") or ""
            proc_desc = bool(re.search(r"\bproc\b|chance to deal additional", (rec["spelldesc"] or "").lower())) or \
                (bool(re.search(r"\bdecrease (?:hitpoints|hp) by \d+", old.lower())) and "proc" not in old.lower())
            # workbook wins on all (the user, 2026-11-01): its icon, frame and
            # spelling too; a rename is one every lookup by name reads alike
            # (rename_safe) or it is kept and listed
            icon, frame = icon_of(cell(s, "IconID"), icons), FRAMES.get(cell(s, "IconBGDColor"))
            if icon is not None:
                want["icon"] = icon
            elif icons is not None and icon_of(cell(s, "IconID")) is not None:
                lists["the workbook's icon is no icon on the disc either way round (ours kept)"].append(
                    f"{sid} {rec['spellname']}: {cell(s, 'IconID')}")
            if frame is not None:
                want["iconColor"] = frame
            nm = cell(s, "Name")
            if nm and nm != rec["spellname"]:
                if rename_safe(rec["spellname"], nm, db):
                    want["spellname"] = nm
                    lists["a spell renamed as the workbook spells it (every lookup by name reads both alike)"].append(
                        f"{sid} {rec['spellname']!r} -> {nm!r}")
                else:
                    lists["a spell the workbook spells otherwise, its name kept (a lookup by name reads it otherwise)"].append(
                        f"{sid} {rec['spellname']!r}: the workbook's {nm!r}")
            # a description of ours that says other than this row does (DESCRIPTIONS)
            if fold(rec["spellname"]) in DESCRIPTIONS:
                text = DESCRIPTIONS[fold(rec["spellname"])]
                want["spelldesc"] = text if text is not None else \
                    desc_of(j, cm, rec["spellname"], quest[1] if quest else "", were_form)
                errata["description"].add(sid)
                if want["spelldesc"] != rec["spelldesc"]:
                    lists["a record of ours given the description its row says (DESCRIPTIONS)"].append(
                        f"{sid} {rec['spellname']}: {want['spelldesc']!r}")
            changed = {k: v for k, v in want.items() if rec.get(k) != v}
            if changed:
                spells.append(dict(spellid=sid, action="update", sheet_id=hexid(s.get("SpellID")),
                                   name=rec["spellname"], before={k: rec[k] for k in changed}, set=changed))
                counts["records updated"] += 1
                for k in changed:
                    counts[f"  {k} changed"] += 1
            else:
                counts["records as the workbook has them already"] += 1

        # what it does
        if j["effects"]:
            for what in renamed:
                lists["a made or taken item of ours by the workbook's name, under our item's own"].append(
                    f"{sid} {cell(s, 'Name')}: {what}")
            for what in by_line:
                lists["a made or taken item the workbook names none of ours of, named as our line names it"].append(
                    f"{sid} {cell(s, 'Name')}: {what}")
            for key in ("item", "consume"):
                left = [o[key].get("name") or "no name" for o in j["effects"] if key in o and "patternid" not in o[key]]
                if left:
                    lists[f"an item none of ours {'made' if key == 'item' else 'taken'} (it "
                          f"{'makes' if key == 'item' else 'takes'} nothing)"].append(
                        f"{sid} {cell(s, 'Name')}: {', '.join(left)}")
            line = line_of(j, proc_desc, new_scope, item_names)
            text = dumps(j)
            now = erows.get(sid)
            if now is None:
                fx = 0
                if disc:
                    import fill_spell_visuals_eqoaemu as fsv
                    fx = fsv.dictid(cell(s, "Graphic"), disc) or 0
                effects.append(dict(spellid=sid, action="insert", row=dict(spellid=sid, line=line, fxdict=fx,
                                                                           source=f"{NOTE}; made", effects=text)))
                counts["effects rows made"] += 1
            else:
                src = now["source"] or ""
                src = src if "effects from EQOAEmu's workbook" in src else f"{src}; effects from EQOAEmu's workbook".lstrip("; ")
                keep = keep_line(j, now.get("line") or "")
                if keep:
                    counts[f"lines kept: {keep}"] += 1
                    line = now.get("line") or ""
                elif "reagent" not in line.lower():
                    # the reagent its line names, where the workbook names none of ours
                    m = REAGENT.search(now.get("line") or "")
                    if m:
                        line += ", " + m.group(0).strip(" ,")
                        counts["lines given their old reagent"] += 1
                # the undead check its line gives it (EQOA-Data's "Target undead"),
                # which the server keeps on a harmful spell read from the workbook
                # (entity/spelljson.go's undeadFromLine, reading this line): the new
                # line says it too (Punish Death, Exorcise: the workbook's "Enemy")
                if j["harmful"] and UNDEAD_LINE.match((now.get("line") or "").strip()) and TARGET_LINE.match(line):
                    line = TARGET_LINE.sub("Target undead", line, count=1)
                    counts["lines keeping their undead target"] += 1
                # and the one the workbook's name gives it (entity/undead.go's
                # readUndead: a harmful spell named "... Undead" is for undead
                # alone) where our record keeps a name of its own: Agonize Death,
                # the workbook's Agonize Undead (the user, 2026-11-01: "yes", for
                # undead alone as the workbook's record is)
                elif j["harmful"] and undead_name(cell(s, "Name")) and \
                        not undead_name(want.get("spellname") or (rec or {}).get("spellname")) and TARGET_LINE.match(line):
                    line = TARGET_LINE.sub("Target undead", line, count=1)
                    counts["lines given the undead target the workbook's name gives"] += 1
                after = dict(line=line, effects=text, source=src)
                before = {k: now.get(k, "") if k != "effects" else (now.get("effects") or "") for k in after}
                if before != after:
                    effects.append(dict(spellid=sid, action="update", before=before, set=after))
                    counts["effects rows written"] += 1
        else:
            counts["no effect in the workbook: line kept"] += 1

        # its area
        centre = AREA_CENTRE.get(sheet_scope)
        own = {o["kind"] for o in j["effects"]} & OWN_TARGETS
        if centre is not None and (j.get("radius") or sheet_scope in ("TB AoE", "PB AoE")) and own:
            counts["areas left out (" + ", ".join(sorted(own)) + ": its own code picks whom)"] += 1
        elif centre is not None and (j.get("radius") or sheet_scope in ("TB AoE", "PB AoE")):
            want_a = dict(centre=centre, radius=float(j.get("radius") or 0), maxtargets=0,
                          note=f"{NOTE}: AoERadius {j.get('radius') or 0} ({sheet_scope})")
            now = areas.get(sid)
            if now is None:
                area_rows.append(dict(spellid=sid, action="insert", row=dict(spellid=sid, **want_a)))
                counts["areas made"] += 1
            elif any(now.get(k) != v for k, v in want_a.items() if k != "note"):
                area_rows.append(dict(spellid=sid, action="update", before={k: now.get(k) for k in want_a}, set=want_a))
                counts["areas changed"] += 1

        # a new teleport's place (gates.go reads spellgates by the name)
        if rec is None:
            for o in j["effects"]:
                if o["kind"] == "teleport":
                    gates.append(dict(spellid=sid, name=cell(s, "Name"), location=o.get("location") or "",
                                      group=j["scope"] != "self", radius=j.get("radius") or 0))

        # whose it is: a were's is no class's (were.go)
        src = "cm (eqoaemu)" if cm else "eqoaemu"
        were = quest is not None or were_form or is_were_ability(cell(s, "Name"),
                                                     spells[-1]["row"]["spelldesc"] if rec is None else rec["spelldesc"])
        if sid in split_of:
            splits.append(dict(row=s, new=sid, shared=split_of[sid], classes=classes, source=src, were=were))
            if were:
                new_rows[sid] = (s, j, True)
        elif were:
            if classes:
                counts["were forms and abilities: no class rows"] += 1
                lists["a were's (were.go, or a were quest's reward): record and effects, no class rows, no scroll"].append(
                    f"{sid} {cell(s, 'Name')}" + (f" (the quest file: {quest[0]!r}, {quest[1]})" if quest else ""))
            if rec is None:
                new_rows[sid] = (s, j, True)
        elif rec is None:
            for c in classes:
                links.append(dict(cls=c, spellid=sid, source=src))
        elif link_existing:
            key = spell_key(rec["spellname"])
            for c in classes:
                if c not in linked[sid] and key not in class_keys[c]:
                    links.append(dict(cls=c, spellid=sid, source=src))
        elif classes:
            key = spell_key(rec["spellname"])
            miss = [n for n, c in CLASSES.items() if c in classes and c not in linked[sid] and key not in class_keys[c]]
            if miss:
                lists["a class the workbook gives a spell we have, with no spell of its name (--link-existing)"].append(
                    f"{sid} {rec['spellname']}: {' '.join(miss)}")
    plan_mirrors(records, erows, written, spells, effects, lists, counts)
    plan_ours(records, erows, written, items_by_name, item_names, spells, effects, lists, counts)
    moves, migrations = plan_splits(db, wb, smap, splits, records, links, lists, counts)
    gate_rows = plan_gates(db, gates, lists, counts)
    counts["class rows"] = len(links)

    scrolls, stock = plan_scrolls(db, new_rows, records, lists, counts, links, moves, migrations)
    p = dict(spells=spells, effects=effects, areas=area_rows, links=links, scrolls=scrolls, stock=stock,
             moves=moves, migrations=migrations, gates=gate_rows, foods=foods, made=made.out,
             made_spells=sorted(made.settled), folds=folds,
             errata={k: sorted(v) for k, v in errata.items()},
             lists={k: v for k, v in lists.items()}, counts=dict(counts), effect_clash=wb.effect_clash)
    p["changes"] = changes_of(p)
    return p


MEAL_DESC = re.compile(r"(?i)\bmeal\b.*\bhealth regeneration\b")      # handlers/consume.go's
DRINK_DESC = re.compile(r"(?i)\bbeverage\b.*\bpower regeneration\b")
DRINK_TEXT = "A refreshing beverage that increases power regeneration."   # every drink of ours
POTION_NAME = re.compile(r"(?i)\b(potion|draught|vaccine|elixir|tonic)\b")


def plan_foods(db, wb, items_by_name, item_names, lists, counts):
    """The workbook's food and drink (its Created Item sheets: an item whose
    effects give its eater power over time, health over time or health, and
    nothing else; the user, 2026-11-01: "yes add the food too"): ours take
    its HoT and PoT (they held their level, tools/stock_grocers.py's stand-in);
    one we lack is made as its nearest sibling of ours is (a drink a drink's
    row, a meal a meal's: art, stack, usable type 2, trade), 910000 + its
    ItemID, described as ours are; each gets its foodeffect row (its time,
    DisspellOnCast, DisspellOnContact). One that heals at once as well as over
    time is a potion (itemeffect, entity/potion.go: drunk, its line's heal now
    and its ticks), as ours of that kind are. items_by_name and item_names
    take the made ones, so the summoning spells name them."""
    out = dict(updates=[], items=[], foodeffects=[], itemeffects=[])
    if not wb.item_info:
        return out
    cols = columns(db, "itempattern")
    rows = {r[0]: dict(zip(cols, r)) for r in db.execute(f"SELECT {', '.join(cols)} FROM main.itempattern")}
    have_fe = {r[0] for r in db.execute("SELECT patternid FROM main.foodeffect")} if has_table(db, "foodeffect") else set()
    ie = {}
    if has_table(db, "itemeffect"):
        ie = {r[0]: r[1] for r in db.execute("SELECT patternid, line FROM main.itemeffect")}
    ie_src = {}
    if has_table(db, "itemeffect"):
        ie_src = {r[0]: r[1] for r in db.execute("SELECT patternid, source FROM main.itemeffect")}
    # the potions an item could be modelled on: tools/seed_potions.py's drunk ones
    drunk = [r for pid, r in rows.items() if (ie.get(pid) or "").lower().startswith("self") and ie_src.get(pid) == "potions"]
    found = []   # (kind, our row) of the workbook's foods we have: a made one's siblings
    groups = collections.OrderedDict()
    for r in wb.item_info:
        iid = num(r, "ItemID")
        if isinstance(iid, int):
            groups.setdefault(iid, []).append(r)
    foods = []
    for iid, rs in groups.items():
        effs = [wb.item_effects[e] for r in rs for e in effect_ids(r) if e in wb.item_effects]
        if not effs:
            continue
        figures = collections.Counter()
        ok = True
        for e in effs:
            if cell(e, "Scope").split(" ")[0] not in ("Self", "") or cell(e, "TYPE") not in ("Statistic Change", "Dmg/Heal"):
                ok = False
            for col in [c for c, _ in STAT_COLS] + ["HP1", "HP2", "DoT", "POW", "SPEED", "HP Factors", "OFF Mods", "DEF Mods"]:
                v = num(e, col)
                if not v:
                    continue
                if col == "PoT" and v > 0:
                    figures["pot"] += v
                elif col == "HoT" and v > 0:
                    figures["hot"] += v
                elif col == "HP1" and v > 0:
                    # a meal's health over time (its row a Statistic Change of
                    # 2-3 minutes); a Dmg/Heal row as effect_objects reads it
                    d = num(e, "Duration") or 0
                    figures["hot" if 0 < d and (cell(e, "TYPE") != "Dmg/Heal" or d <= HEAL_TICKS_UP_TO) else "heal"] += v
                else:
                    ok = False
        name = re.sub(r"(?i)\s+effect$", "", cell(rs[0], "Name")).strip()
        if not ok or not figures or POTION_NAME.search(name):
            continue  # not a food or a drink (an alchemist's potion is not this part's)
        potion = figures["heal"] > 0
        meal_rule = fold(name) in MEAL_RULE and not potion and figures["hot"]
        foods.append(dict(iid=iid, name=name, level=num(rs[0], "Level") or 1, figures=figures, potion=potion,
                          kind="potion" if potion else "meal" if figures["hot"] else "drink",
                          dur=max(num(e, "Duration") or 0 for e in effs),
                          cast=False if meal_rule else any(cell(e, "DisspellOnCast") == "1" for e in effs),
                          contact=True if meal_rule else any(cell(e, "DisspellOnContact") == "1" for e in effs)))
    # ours first (a made one's siblings are the workbook's foods we have, of its kind)
    for f in foods:
        f["pid"] = our_item(items_by_name, f["name"])
        if f["pid"] not in rows:
            f["pid"] = None
    siblings = collections.defaultdict(list)
    for f in foods:
        if f["pid"] is not None and not f["potion"]:
            siblings["meal" if f["figures"]["hot"] else "drink"].append(rows[f["pid"]])
    for f in foods:
        figures, name, level, potion = f["figures"], f["name"], f["level"], f["potion"]
        tag = f"{name} (L{level}, {f['kind']})"
        pid = f["pid"]
        if pid is not None:
            row = rows[pid]
            if potion:
                lists["a potion of the workbook's we have (its itemeffect row left)"].append(f"{pid} {tag}")
                continue
            want = {}
            if figures["hot"]:
                want["HoT"] = figures["hot"]
            if figures["pot"]:
                want["PoT"] = figures["pot"]
            changed = {k: v for k, v in want.items() if row.get(k) != v}
            if changed:
                out["updates"].append(dict(patternid=pid, name=row["itemname"], before={k: row.get(k) for k in changed},
                                           set=changed))
                counts["foods and drinks given the workbook's figures"] += 1
        else:
            pool = drunk if potion else siblings["meal" if figures["hot"] else "drink"]
            if not pool:
                lists["a food of the workbook's with no sibling of ours to make it as"].append(tag)
                continue
            sib = min(pool, key=lambda r: (abs((r["levelreq"] or 0) - level), r["patternid"]))
            pid = ITEM_BASE + f["iid"]
            if pid in rows:
                lists["a made food's id taken"].append(f"{pid} {tag}")
                continue
            a = article(name.lower())
            if potion:
                desc = f"{a[0].upper()}{a[1:]}, a draught that heals at once and over time."
            elif figures["hot"]:
                desc = (f"{a[0].upper()}{a[1:]}, a meal that will increase health regeneration"
                        f"{' and power regeneration' if figures['pot'] else ''}.")
            else:
                desc = DRINK_TEXT
            row = dict(sib)
            row.update(patternid=pid, itemname=name, levelreq=level, itemdesc=desc)
            if not potion:
                row.update(HoT=figures["hot"] or None, PoT=figures["pot"] or None)
            out["items"].append(dict(patternid=pid, row=row, sibling=sib["patternid"]))
            rows[pid] = row
            items_by_name.setdefault(fold(name), pid)
            item_names[pid] = name
            counts["foods and drinks made"] += 1
            lists["a food or drink made (as its sibling)"].append(f"{pid} {tag}, as {sib['patternid']} {sib['itemname']}")
        if potion:
            parts = []
            if figures["heal"]:
                parts.append(f"Increase Hitpoints by {figures['heal']}")
            if figures["hot"]:
                parts.append(f"Increase HP per tic by {figures['hot']}")
            if figures["pot"]:
                parts.append(f"Increase Power per tic by {figures['pot']}")
            line = "Self, " + ", ".join(parts) + (f", {fmt_dur(f['dur'])}" if f["dur"] else "")
            if pid not in ie:
                out["itemeffects"].append(dict(patternid=pid, line=line, name="", icon=0, breaks="", wear=0, area=0.0,
                                               source="eqoaemu"))
                counts["potions given their itemeffect row"] += 1
            continue
        if pid not in have_fe:
            out["foodeffects"].append(dict(patternid=pid, duration_s=int(f["dur"]), ends_on_cast=int(f["cast"]),
                                           ends_on_contact=int(f["contact"]), source="eqoaemu"))
            counts["foodeffect rows"] += 1
    return out


# ---- the items the spells make (the user, 2026-11-01: "lets fix these") ----------

# The Created Item sheets name an item's EFFECT ("<X> Effect", "<X> Mix
# Potion"); a made item is named as our records name such items: no " Effect"
# (the magician's conjured stones), EQOA-Data's "<X> Potion" for a Mixture's
# "<X> Mix Potion", our "<X> Cocktail" for a "<X> Fire Potion", and the
# Distillation and Purification lines' "<X> Corrective" and "<X> Restorative"
# (EQOA-Data's and Allakhazam's 2004 records' words) for the sheets' "<X> Cure
# Potion" and "<X> Restorative Potion"
MADE_NAMES = [(r"\s+effect$", ""), (r"\s+mix\s+potion$", " Potion"), (r"\s+fire\s+potion$", " Cocktail"),
              (r"\s+cure\s+potion$", " Corrective"), (r"\s+restorative\s+potion$", " Restorative")]
# What a spell makes that no source names (no CreatedItemID, or one no sheet
# row has; EQOA-Data and Allakhazam's archived pages name none either), by its
# name's words: (its words, kind, the items of ours it is modelled on (a
# food's: our meals or drinks a spell makes), its name (None: the spell's,
# singular)). A conjured staff named as its spell is, as the magician's
# Phase Staff; a rogue's daggers ("Creates 6", EQOA-Data's Rogue's Daggers)
# its conjured Shadow Knife's kind, thrown; the racial masteries' food and
# drink (EQOA-Data's Bake, "Summon food", and Brew, "summon drink"). OURS.
OUR_KINDS = [
    (re.compile(r"(?i)\bstaff$"), "weapon", ("phase staff",), None),
    (re.compile(r"(?i)\bdaggers?$"), "weapon", ("shadow knife",), None),
    (re.compile(r"(?i)^bake$"), "meal", None, "Halfling Pie"),
    (re.compile(r"(?i)^brew$"), "drink", None, "Dwarven Brew"),
]
# who conjures a weapon, by the making spell's class, in our conjured items'
# words ("Conjured by the magician's Phase Mallet.")
CLASS_WORD = {10: "magician", 6: "rogue", 14: "alchemist", 12: "enchanter", 11: "necromancer", 13: "wizard"}
NUMBER_WORD = {2: "two", 3: "three", 4: "four", 5: "five"}


def made_name(name):
    """A made item's name as our records name such items (MADE_NAMES)."""
    n = " ".join((name or "").split())
    for pat, to in MADE_NAMES:
        m = re.search(pat, n, re.I)
        if m:
            return n[:m.start()] + to
    return n


def walk_effects(objs):
    """Every effect object, a proc's own too."""
    for o in objs:
        yield o
        yield from walk_effects(o.get("procs") or [])


def item_makers(db):
    """The classes whose spells make each item, by its name (fold): every
    class spell (classspells) whose line or effects summon it."""
    out = collections.defaultdict(set)
    if not has_table(db, "spelleffect"):
        return out
    has_fx = "effects" in columns(db, "spelleffect")
    q = ("SELECT c.class, e.line, " + ("e.effects" if has_fx else "''") +
         " FROM main.classspells c JOIN main.spelleffect e ON e.spellid = c.spellid")
    for c, line, eff in db.execute(q):
        for n in line_items(line or "", "item"):
            out[fold(n)].add(c)
        try:
            j = json.loads(eff) if (eff or "").strip() else {}
        except ValueError:
            j = {}
        for o in walk_effects(j.get("effects") or [] if isinstance(j, dict) else []):
            n = ((o.get("item") or {}).get("name") or "") if o.get("kind") == "create_item" else ""
            if n:
                out[fold(n)].add(c)
    return out


def potion_line(objs, thrown, area=0, place=None, cures=1):
    """A potion's use in the words the potion reader reads (entity/potion.go
    reads its itemeffect line as spelltext.go reads a spell's), and what of
    the workbook's those words cannot say."""
    spell = {"scope": "target" if thrown else "self", "harmful": thrown, "item_names": {}}
    parts, unsaid = [], []
    hits = [o for o in objs if o["kind"] in ("hp", "hp2") and (o.get("amount") or 0) < 0]
    splash = None
    if thrown and area and hits:
        # the splash lands the line on every creature in it, its target
        # among them (potion.go throwLocked): the area's figure for all, its
        # target's own beside it its target_line (potion_target_line)
        splash = next((o for o in hits if o["kind"] == "hp2"), hits[0])
    dur, lasting, said_splash = 0, False, False
    for o in objs:
        k, n = o["kind"], o.get("amount")
        dur = max(dur, o.get("duration_s") or 0)
        if splash is not None and o in hits:
            if not said_splash:
                parts.append(f"Decrease Hitpoints by {abs(splash['amount'])}" + scale_words(splash))
                said_splash = True
            continue
        if k == "cure":
            parts.append(f"Dispel Effect x{cures}")
        elif k == "teleport":
            if place:
                parts.append(f"Teleport to {place}")
            else:
                unsaid.append(f"a teleport to {o.get('location') or 'nowhere'} (no spell gate is that place)")
        elif k == "illusion":
            if not o.get("sprite"):
                unsaid.append("a body with no sprite")  # else its row's body (potion_body)
        elif k in ("power", "stat_set", "speed") and o.get("absolute"):
            unsaid.append(f"a {k} set to {n} (no potion's words for it)")
        elif k in ("hp", "hp2", "hp_tick", "power", "power_tick", "stat", "speed", "invis", "gate"):
            parts += clause(o, spell)
            lasting = lasting or k in ("hp_tick", "power_tick", "stat", "speed", "invis")
        else:
            unsaid.append(f"{k} ({o.get('type')})")
    if not parts:
        return "", unsaid
    line = ("Target" if thrown else "Self") + ", " + ", ".join(parts)
    if dur and lasting:
        line += ", " + fmt_dur(dur)
    return line, unsaid


# a line's time, as spelltext.go's reDuration reads one ("4min", "18sec")
LINE_TIME = re.compile(r"(?i)^(?:for\s+)?\d+(?:\.\d+)?\s*(?:sec|secs|second|seconds|s|min|mins|minute|minutes|hr|hrs|hour|"
                       r"hours)$")
# the invisible body, either way round: an illusion of it is invisibility
# (entity/spelljson.go's invisibleSprite), which the use's own Invisible
# effect says in its line
INVISIBLE_SPRITES = {0x435EF663, 0x63F65E43}


def sprite_body(sprite):
    """The body a workbook SpriteID shows: the CHAR.ESF CSprite's DictID, as
    npcs.modelid holds it, which the workbook writes byte-reversed (every one
    of its sprites, measured against the disc 2026-11-01; the server reads a
    spell's sprite so: entity/spelljson.go's spriteBody); 0 for none and for
    the invisible body."""
    try:
        v = int(str(sprite or "").strip(), 16)
    except ValueError:
        return 0
    if not 0 < v <= 0xFFFFFFFF or v in INVISIBLE_SPRITES:
        return 0
    return int.from_bytes(v.to_bytes(4, "big")[::-1], "big")


def potion_body(objs):
    """The body a potion's use shows (itemeffect.body: its drinker's, or a
    thrown one's creature's while it lasts): its Alter Appearance sprite's,
    0 for none."""
    for o in objs:
        if o["kind"] == "illusion":
            b = sprite_body(o.get("sprite"))
            if b:
                return b
    return 0


def potion_target_line(objs, thrown, area):
    """What a thrown potion's target alone takes beside its line
    (itemeffect.target_line), in the line's words: a target-based area's
    first figure (HP1) where it has a second (HP2, the line's, which lands on
    every creature in the splash, the target among them: the workbook's
    legend, "AoE hits target and also the radius from the target, including
    target a second time"); "" for none."""
    hits = [o for o in objs if o["kind"] in ("hp", "hp2") and (o.get("amount") or 0) < 0]
    if not (thrown and area) or {o["kind"] for o in hits} != {"hp", "hp2"}:
        return ""
    return "Target, " + ", ".join(f"Decrease Hitpoints by {abs(o['amount'])}" + scale_words(o)
                                  for o in hits if o["kind"] == "hp")


def potion_extras(objs, thrown, area):
    """A use's columns past its line (body, target_line), each only when it
    has one: a database made before them reads none."""
    out = {}
    b = potion_body(objs)
    if b:
        out["body"] = b
    t = potion_target_line(objs, thrown, area)
    if t:
        out["target_line"] = t
    return out


# a use's columns as a row with none holds them (database/itemeffect.go)
USE_DEFAULTS = {"line": "", "name": "", "icon": 0, "breaks": "", "wear": 0, "area": 0.0, "source": "gm", "body": 0,
                "target_line": ""}
# the columns itemeffect gained after it was made, which --apply adds to a
# database made before them (the server adds them at its start)
ITEMEFFECT_ADDED = [("body", "INTEGER NOT NULL DEFAULT 0"), ("target_line", "TEXT NOT NULL DEFAULT ''")]


def read_bodies(path):
    """The disc's CSprites, as cmd/modelbake writes them (<worlds>/models.txt,
    which the server reads: a line a body, its DictID in hex first; # a
    comment)."""
    out = set()
    with open(path, encoding="utf-8") as f:
        for line in f:
            w = line.split("#", 1)[0].split()
            if w:
                out.add(int(w[0], 16))
    return out


def potion_desc(name, objs, thrown, area, place, cures):
    """A made potion's description, in the words of ours (seed_potions.py's)."""
    kinds = {o["kind"] for o in objs}
    if thrown:
        it = "cocktail" if "cocktail" in name.lower() else "potion"
        if kinds & {"hp", "hp2"}:
            return f"Thrown at an enemy, this {it} bursts on impact" + (", catching everything near it." if area else ".")
        if "hp_tick" in kinds:
            return f"Thrown at an enemy, this {it}'s contents eat away at it."
        return f"Thrown at an enemy, this {it} weakens it."
    if "cure" in kinds:
        return ("A potion that takes one harmful effect off its drinker." if cures == 1 else
                f"A potion that takes up to {NUMBER_WORD.get(cures, cures)} harmful effects off its drinker.")
    if "teleport" in kinds and place:
        return f"Drinking this potion takes you to {place}."
    if "gate" in kinds:
        return "Drinking this potion returns you to your bind point."
    bits = []
    if any(o["kind"] == "hp" and (o.get("amount") or 0) > 0 for o in objs):
        bits.append("heals its drinker")
    if any(o["kind"] == "hp_tick" and (o.get("amount") or 0) > 0 for o in objs):
        bits.append("heals its drinker over time")
    if any(o["kind"] == "power" and (o.get("amount") or 0) > 0 for o in objs):
        bits.append("restores its power")
    ups = []
    for o in objs:
        if o["kind"] == "stat" and (o.get("amount") or 0) > 0:
            w = {"maxhp": "maximum hit points", "maxpower": "maximum power", "ac": "armor"}.get(o["stat"], STAT_WORD.get(o["stat"], o["stat"]))
            if w not in ups:
                ups.append(w)
    if ups:
        bits.append("raises its " + (", ".join(ups[:-1]) + " and " if len(ups) > 1 else "") + ups[-1] + " for a time")
    if not bits:
        return "A potion of an alchemist's making."
    return "A potion that " + (", ".join(bits[:-1]) + " and " if len(bits) > 1 else "") + bits[-1] + "."


class MadeItems:
    """The items the spells make (the doc's "every other item a spell
    makes"): settle() names each spell's created items as the server finds
    them, making those none of ours; out holds what --apply writes."""

    def __init__(self, db, wb, items_by_name, item_names, lists, counts, icons=None, foods=None, bodies=None):
        self.db, self.wb, self.lists, self.counts, self.icons = db, wb, lists, counts, icons
        self.bodies = bodies
        self.items_by_name, self.item_names = items_by_name, item_names
        self.cols = columns(db, "itempattern")
        self.rows = {r[0]: dict(zip(self.cols, r)) for r in
                     db.execute(f"SELECT {', '.join(self.cols)} FROM main.itempattern")}
        self.uses = {}
        if has_table(db, "itemeffect"):
            ecols = columns(db, "itemeffect")
            self.uses = {r[0]: dict(zip(ecols, r)) for r in db.execute(f"SELECT {', '.join(ecols)} FROM main.itemeffect")}
        self.foods = {}
        if has_table(db, "foodeffect"):
            fcols = columns(db, "foodeffect")
            self.foods = {r[0]: dict(zip(fcols, r)) for r in db.execute(f"SELECT {', '.join(fcols)} FROM main.foodeffect")}
        # what plan_foods plans beside: its made foods, their figures, their
        # foodeffect rows and the potions' uses
        for x in (foods or {}).get("items", []):
            self.rows[x["patternid"]] = dict(x["row"])
        for x in (foods or {}).get("updates", []):
            if x["patternid"] in self.rows:
                self.rows[x["patternid"]] = dict(self.rows[x["patternid"]], **x["set"])
        for x in (foods or {}).get("foodeffects", []):
            self.foods[x["patternid"]] = x
        for x in (foods or {}).get("itemeffects", []):
            self.uses[x["patternid"]] = x
        self.makers = item_makers(db)
        self.gates = []
        if has_table(db, "spellgates"):
            self.gates = db.execute("SELECT spell, place, world, x, z FROM main.spellgates WHERE place <> '' "
                                    "ORDER BY spell").fetchall()
        self.index = None  # fold(name) -> patternids, made at the first lookup
        self.made = {}     # an item's key -> its patternid, once found or made
        self.given = set()  # items of ours given a use
        self.settled = set()  # the spells whose items this named
        self.out = dict(items=[], updates=[], itemeffects=[], foodeffects=[], itemeffect_updates=[])

    # the server's lookup of an item by its name (database.ItemPatternByName):
    # no spell scroll, one with a use first, then the lowest id
    def by_name(self, name):
        if self.index is None:
            self.index = collections.defaultdict(list)
            for pid, r in self.rows.items():
                self.index[fold(r["itemname"])].append(pid)
        best = None
        for pid in self.index.get(fold(name), []):
            r = self.rows[pid]
            if r["itemicon"] == SCROLL_ICON and r["equipslot"] == -1:
                continue
            key = (pid not in self.uses, pid)
            if best is None or key < best[0]:
                best = (key, pid)
        return best[1] if best else None

    def sheet_rows(self, name, spell):
        """The Created Item Info rows of the item a create_item names, by its
        name; one that names none, the sheets' item of no ItemID named for
        its spell ("<spell> Potion")."""
        rows = self.wb.item_info
        if name:
            return [r for r in rows if fold(cell(r, "Name")) == fold(name)]
        lead = fold(spell) + " "
        return [r for r in rows if num(r, "ItemID") is None and fold(cell(r, "Name")).startswith(lead)]

    def effects_of(self, rows):
        """The item's effects as the brief's effect objects, each with its
        own row's time, and the largest AoERadius."""
        objs, radius = [], 0
        for r in rows:
            ctx = dict(scope=cell(r, "Scope"), target_type=cell(r, "Target Type"), duration=0, level=None,
                       min_level=None, radius=0)
            for i in effect_ids(r):
                e = self.wb.item_effects.get(i)
                if e is None:
                    continue
                radius = max(radius, num(e, "AoERadius") or 0)
                for o in effect_objects(e, ctx, self.wb, self.items_by_name):
                    o.pop("consume", None)  # the item itself
                    objs.append(o)
        return objs, radius

    def place_of(self, location):
        """The place of the spell gate within 15 of a Location (plan_gates'
        rule), None for none."""
        parts = [x.strip() for x in (location or "").split(",")]
        world = WORLDS.get(parts[0].lower()) if parts else None
        try:
            x, z = float(parts[1]), float(parts[3])
        except (ValueError, IndexError):
            return None
        near = [(abs(gx - x) + abs(gz - z), place) for _, place, gw, gx, gz in self.gates
                if gw == world and abs(gx - x) < 15 and abs(gz - z) < 15]
        return min(near)[1] if near else None

    def sibling(self, thrown, classes, level, want, last_word):
        """The potion of ours a made one is made as: an item with a use that
        another spell of its class makes (any class's when none), drunk or
        thrown as it is, of its kind (want: cure, teleport, area), a cure of
        its name's last word (a corrective's line, a restorative's), then
        the nearest its level."""
        if want != "cure":
            last_word = None
        def pool(any_class):
            out = []
            for pid, use in self.uses.items():
                r = self.rows.get(pid)
                if r is None or (use.get("line") or "").strip().lower().startswith("self") == thrown:
                    continue
                if not any_class and not (self.makers.get(fold(r["itemname"]), set()) & set(classes)):
                    continue
                out.append((pid, r, use))
            return out
        cands = pool(False) or pool(True)
        if not cands:
            return None, None

        def kind(use):
            line = (use.get("line") or "").lower()
            return {"cure": "dispel effect" in line, "teleport": "bind point" in line or "teleport" in line,
                    "area": (use.get("area") or 0) > 0}.get(want, True)
        pid, r, use = min(cands, key=lambda c: (not kind(c[2]), last_word is not None and
                                                fold(c[1]["itemname"]).split()[-1:] != [last_word],
                                                abs((c[1]["levelreq"] or 0) - level), c[0]))
        return r, use

    def new_id(self, it, s, rows):
        iid = num(rows[0], "ItemID") if rows else None
        if not isinstance(iid, int) and str(it.get("sheet_item_id") or "").strip().isdigit():
            iid = int(it["sheet_item_id"])
        pid = ITEM_BASE + iid if isinstance(iid, int) else MADE_BASE + int(hexid(s.get("SpellID")) or "0", 16)
        return None if pid in self.rows else pid

    def keep(self, pid, row):
        self.rows[pid] = row
        if self.index is not None:
            self.index[fold(row["itemname"])].append(pid)
        self.items_by_name.setdefault(fold(row["itemname"]), pid)
        self.item_names[pid] = row["itemname"]

    def settle(self, s, j, sid=None, replace=False):
        """Each item j's spell makes under the name the server finds it by:
        one of ours as settle_item_names left it, else one made here.
        replace: the spell is a record of ours the map spells otherwise, so
        its item of ours takes the workbook's use (give_use)."""
        for o in walk_effects(j["effects"]):
            it = o.get("item") if o.get("kind") == "create_item" else None
            if it is None:
                continue
            if it.get("patternid") is not None:
                pid = self.by_name(it.get("name") or "")
                if pid is not None:
                    self.give_use(pid, self.sheet_rows(it.get("sheet_name") or it.get("name") or "", ""), s, replace)
                continue
            pid = self.make(it, s)
            if pid is None:
                continue
            nm = (it.get("name") or "").strip()
            if nm and nm != self.item_names[pid]:
                it["sheet_name"] = nm
            it["name"], it["patternid"] = self.item_names[pid], pid
            if not str(it.get("sheet_item_id") or "").strip():
                it.pop("sheet_item_id", None)
            if sid is not None:
                self.settled.add(sid)

    def make(self, it, s):
        """The item a create_item names, found by its made name or made; its
        patternid, None when nothing is known of it (listed)."""
        spell = cell(s, "Name")
        rows = self.sheet_rows((it.get("name") or "").strip(), spell)
        our = None
        if rows:
            key, name = ("sheet", fold(cell(rows[0], "Name"))), made_name(cell(rows[0], "Name"))
        else:
            our = next((k for k in OUR_KINDS if k[0].search(spell.strip())), None)
            if our is None:
                return None
            name = our[3] or re.sub(r"(?i)(dagger|knife|staff|blade)s$", r"\1", spell.strip())
            key = ("ours", fold(spell))
        if key in self.made:
            return self.made[key]
        pid = self.by_name(name)
        if pid is not None:
            self.made[key] = pid
            if rows:
                self.give_use(pid, rows, s)
            return pid
        pid = self.new_id(it, s, rows)
        if pid is None:
            self.lists["a made item's id taken (nothing made)"].append(f"{name} ({spell})")
            return None
        made = self.make_potion(pid, name, rows, s) if rows else self.make_ours(pid, name, our, s)
        if made:
            self.made[key] = pid
            return pid
        return None

    def make_potion(self, pid, name, rows, s):
        info = rows[0]
        scope, ttype = cell(info, "Scope").split(" ")[0], cell(info, "Target Type").lower()
        thrown = scope not in ("Self", "")
        level = num(info, "Level") or num(s, "Level") or 1
        classes = class_mask(cell(info, "CLASS")) or class_mask(cell(s, "CLASS"))
        objs, radius = self.effects_of(rows)
        tag = f"{pid} {name} ({cell(s, 'Name')}'s, L{level})"
        if thrown and not ttype.startswith("enemy"):
            self.lists["a made item whose use is cast at a friend (the potion reader throws at enemies): not made"].append(tag)
            return False
        kinds = {o["kind"] for o in objs}
        want = "cure" if "cure" in kinds else "teleport" if kinds & {"teleport", "gate"} else "area" if thrown and radius else ""
        sib, use = self.sibling(thrown, classes, level, want, fold(name).split()[-1])
        if sib is None:
            self.lists["a made item with no potion of ours to make it as: not made"].append(tag)
            return False
        cures = 1
        m = re.search(r"(?i)dispel\s+effects?\s+x\s*(\d+)", use.get("line") or "")
        if want == "cure" and m:
            cures = int(m.group(1))
        place = None
        for o in objs:
            if o["kind"] == "teleport":
                place = self.place_of(o.get("location"))
        area = radius if thrown else 0
        line, unsaid = potion_line(objs, thrown, area, place, cures)
        if not line:
            self.lists["a made item whose use no potion's words say: not made"].append(f"{tag}: {', '.join(unsaid)}")
            return False
        row = dict(sib)
        # tradeable but a crude or faulty spell's (tools/seed_potions.py's rule)
        row.update(patternid=pid, itemname=name, levelreq=1 if sib["levelreq"] == 1 else level,
                   trade=1 if fold(cell(s, "Name")).split()[:1] in (["crude"], ["faulty"]) else 0,
                   itemdesc=self.desc(sib, s) or potion_desc(name, objs, thrown, area, place, cures))
        rng = num(info, "Range")
        if thrown and rng and rng > 1:
            row["unk5"] = int(rng)
        lasting = any(o.get("duration_s") for o in objs)
        icon = (icon_of(cell(info, "IconID"), self.icons) or 0) if lasting and not thrown else 0
        self.out["items"].append(dict(patternid=pid, row=row, sibling=sib["patternid"]))
        self.out["itemeffects"].append(dict(patternid=pid, line=line, name="", icon=icon, breaks="", wear=0,
                                            area=float(area), source="eqoaemu", **self.extras(objs, thrown, area, tag)))
        self.keep(pid, row)
        self.uses[pid] = self.out["itemeffects"][-1]
        self.counts["made items (potions)"] += 1
        self.lists["a potion made (as its sibling of ours): its use"].append(
            f"{tag}, as {sib['patternid']} {sib['itemname']}: {line}" + (f"; unsaid: {', '.join(unsaid)}" if unsaid else ""))
        return True

    def desc(self, sib, s):
        """A conjured item's words, as its sibling's ("Conjured by the
        magician's Lava Stone."), for its own spell."""
        m = re.match(r"(?i)^conjured by the (\w+)'s ", sib.get("itemdesc") or "")
        return f"Conjured by the {m.group(1)}'s {cell(s, 'Name')}." if m else ""

    def give_use(self, pid, rows, s, replace=False):
        """An item of ours the sheets give a use for: one with none (no
        itemeffect, foodeffect or a meal's or drink's words) takes the
        workbook's, as a potion's, its row made a thrown one's when the use
        is thrown; one a record the map spells otherwise makes (replace: the
        Transformative Pill, the 2004 records' item of the Transmuter's
        second rank) takes the workbook's use over its own, as the record
        takes the workbook's record (2026-11-01: "workbook wins"; its effect's
        name kept, its icon a made potion's); one whose use the import wrote
        (source eqoaemu) takes what a use's columns say now that they could
        not then (its body, its target's own share), a value a GM gave since
        left."""
        r = self.rows.get(pid)
        if not rows or r is None or pid in self.foods or pid in self.given:
            return
        old = self.uses.get(pid)
        if old is not None and not replace and (old.get("source") or "") != "eqoaemu":
            self.ours_time(pid, r, rows, old)
            return  # a use of ours
        if old is None and (MEAL_DESC.search(r["itemdesc"] or "") or DRINK_DESC.search(r["itemdesc"] or "")):
            return
        info = rows[0]
        thrown = cell(info, "Scope").split(" ")[0] not in ("Self", "")
        tag = f"{pid} {r['itemname']} (the workbook's {cell(info, 'Name')})"
        if thrown and not cell(info, "Target Type").lower().startswith("enemy"):
            self.lists["an item of ours with no use whose workbook use is cast at a friend: left"].append(tag)
            return
        level = num(info, "Level") or num(s, "Level") or 1
        objs, radius = self.effects_of(rows)
        area = radius if thrown else 0
        extras = self.extras(objs, thrown, area, tag)
        if old is not None and not replace:
            self.given.add(pid)
            self.use_columns(pid, r, tag, old, {"body": extras.get("body", 0),
                                                 "target_line": extras.get("target_line", "")}, False)
            return
        line, unsaid = potion_line(objs, thrown, area, None, 1)
        if not line:
            self.lists["an item of ours with no use whose workbook use no potion's words say: left"].append(tag)
            return
        self.given.add(pid)
        if thrown:
            sib, _ = self.sibling(True, class_mask(cell(info, "CLASS")) or class_mask(cell(s, "CLASS")), level,
                                  "area" if radius else "", "")
            if sib is not None:
                want = {k: sib[k] for k in ("equipslot", "attacktype", "unk5", "procanim", "maxstack", "classuse")}
                want["levelreq"] = 1 if sib["levelreq"] == 1 else level
                rng = num(info, "Range")
                if rng and rng > 1:
                    want["unk5"] = int(rng)
                changed = {k: v for k, v in want.items() if r.get(k) != v}
                if changed:
                    self.out["updates"].append(dict(patternid=pid, name=r["itemname"], before={k: r.get(k) for k in changed},
                                                    set=changed))
                    r.update(changed)
        if replace and r.get("levelreq") != level:
            # the workbook's item's level too (the user, 2026-11-01, of the
            # Transformative Pill: "yes make it level 60"), its other columns ours
            self.out["updates"].append(dict(patternid=pid, name=r["itemname"], before={"levelreq": r.get("levelreq")},
                                            set={"levelreq": level}))
            r["levelreq"] = level
            self.lists["an item of ours a merged spell makes: the workbook's level"].append(f"{tag}: L{level}")
        if old is None:
            self.out["itemeffects"].append(dict(patternid=pid, line=line, name="", icon=0, breaks="", wear=0,
                                                area=float(area), source="eqoaemu", **extras))
            self.uses[pid] = self.out["itemeffects"][-1]
            self.counts["items of ours given the workbook's use"] += 1
            self.lists["an item of ours with no use given the workbook's"].append(
                f"{tag}: {line}" + (f"; unsaid: {', '.join(unsaid)}" if unsaid else ""))
            return
        lasting = any(o.get("duration_s") for o in objs)
        self.use_columns(pid, r, tag, old, dict(
            line=line, icon=(icon_of(cell(info, "IconID"), self.icons) or 0) if lasting and not thrown else 0,
            area=float(area), body=extras.get("body", 0), target_line=extras.get("target_line", ""), source="eqoaemu"),
            True)

    def extras(self, objs, thrown, area, tag):
        """A use's columns past its line (potion_extras), its body only
        where the disc has that CSprite when the disc's bodies are known
        (--bodies): one it lacks listed, not written (the server shows no
        body the disc lacks either: entity/illusion.go)."""
        out = potion_extras(objs, thrown, area)
        if "body" in out and self.bodies is not None and out["body"] not in self.bodies:
            self.lists["a use's body no CSprite on the disc has (not written)"].append(f"{tag}: {out['body']:#010x}")
            del out["body"]
        return out

    def ours_time(self, pid, r, rows, old):
        """A use of ours the workbook gives as invisibility (the Ethereal
        potions', the Phase Ring's: its Invisible effect) takes the
        workbook's time, its line's own time put in its place (the user,
        2026-11-01: "use workbook durations"); its words otherwise ours (the
        Volatile Ethereal Potion's speed too: listed, not said)."""
        if pid in self.given:
            return
        objs, _ = self.effects_of(rows)
        if not any(o["kind"] == "invis" for o in objs):
            return
        self.given.add(pid)
        tag = f"{pid} {r['itemname']} (the workbook's {cell(rows[0], 'Name')})"
        dur = max([o.get("duration_s") or 0 for o in objs] + [0])
        line = old.get("line") or ""
        if not dur or not re.search(r"(?i)\binvis", line):
            return
        parts = [x.strip() for x in line.split(",")]
        at = next((i for i in range(len(parts) - 1, 0, -1) if LINE_TIME.match(parts[i])), None)
        if at is None:
            parts.append(fmt_dur(dur))
        else:
            parts[at] = fmt_dur(dur)
        want = ", ".join(parts)
        unsaid = sorted({o["kind"] for o in objs} - {"invis", "illusion"})
        if unsaid:
            self.lists["an invisibility of ours given the workbook's time: what else its use does, not said"].append(
                f"{tag}: {', '.join(unsaid)}")
        if want == line:
            return
        self.out["itemeffect_updates"].append(dict(patternid=pid, name=r["itemname"], before={"line": line},
                                                   set={"line": want}))
        self.uses[pid] = dict(old, line=want)
        self.counts["items of ours: an invisibility given the workbook's time"] += 1
        self.lists["an item of ours: an invisibility given the workbook's time"].append(f"{tag}: {line!r} -> {want!r}")

    def use_columns(self, pid, r, tag, old, want, replace):
        """An existing use's columns made want's: all of them (replace: the
        workbook's use over ours), else only those still at what a row with
        none holds (a GM's value since left, and said)."""
        before = {k: old.get(k, USE_DEFAULTS[k]) for k in want}
        changed = {k: v for k, v in want.items() if before[k] != v}
        if not replace:
            gm = sorted(k for k in changed if before[k] != USE_DEFAULTS[k])
            if gm:
                self.lists["a use the import wrote that a GM changed since: those columns left"].append(
                    f"{tag}: {', '.join(gm)}")
                changed = {k: v for k, v in changed.items() if k not in gm}
        if not changed:
            return
        self.out["itemeffect_updates"].append(dict(patternid=pid, name=r["itemname"],
                                                   before={k: before[k] for k in changed}, set=changed))
        self.uses[pid] = dict(old, **changed)
        what = "its use the workbook's (a record the map spells otherwise makes it)" if replace else \
            "its use given what its columns say now"
        self.counts[f"items of ours: {what}"] += 1
        self.lists[f"an item of ours: {what}"].append(
            f"{tag}: " + ", ".join(f"{k} {before[k]!r} -> {v!r}" for k, v in changed.items()))

    def make_ours(self, pid, name, our, s):
        """An item no source gives, as our item of its kind nearest its level
        (OUR_KINDS), OURS."""
        _, kind, models, _ = our
        level = num(s, "Level") or 1
        tag = f"{pid} {name} ({cell(s, 'Name')}'s, L{level})"
        if kind == "weapon":
            cands = [r for r in self.rows.values() if fold(r["itemname"]) in models and r["equipslot"] >= 0]
        else:
            words = MEAL_DESC if kind == "meal" else DRINK_DESC
            cands = [r for r in self.rows.values() if r["equipslot"] == -1 and words.search(r["itemdesc"] or "")
                     and self.makers.get(fold(r["itemname"]))]
        if not cands:
            self.lists["an item no source gives with none of ours of its kind: not made"].append(tag)
            return False
        sib = min(cands, key=lambda r: (abs((r["levelreq"] or 0) - level), r["patternid"]))
        row = dict(sib)
        row.update(patternid=pid, itemname=name, levelreq=level)
        why = f"as {sib['patternid']} {sib['itemname']}"
        if kind == "weapon":
            classes = class_mask(cell(s, "CLASS"))
            who = CLASS_WORD.get(classes[0]) if len(set(classes)) == 1 else None
            row["itemdesc"] = f"Conjured by the {who}'s {cell(s, 'Name')}." if who else f"Conjured by {cell(s, 'Name')}."
            row["rent"] = 0  # NO RENT, as every conjured weapon of ours
            if abs((sib["levelreq"] or 0) - level) > 2:
                fig = self.weapon_figures(sib["attacktype"], level)
                if fig:
                    row["weapondamage"], row["maxhp"] = fig[0], fig[1]
                    why += f", its damage and item HP the median of our type {sib['attacktype']} weapons within {fig[2]} levels"
        else:
            a = article(name.lower())
            row["itemdesc"] = (f"{a[0].upper()}{a[1:]}, a meal that will increase health regeneration." if kind == "meal"
                               else DRINK_TEXT)
            fe = self.foods.get(sib["patternid"])
            if fe:
                self.out["foodeffects"].append(dict(patternid=pid, duration_s=fe["duration_s"], ends_on_cast=fe["ends_on_cast"],
                                                    ends_on_contact=fe["ends_on_contact"], source="eqoaemu"))
                self.foods[pid] = self.out["foodeffects"][-1]
        self.out["items"].append(dict(patternid=pid, row=row, sibling=sib["patternid"]))
        self.keep(pid, row)
        self.counts["made items (ours: no source gives them)"] += 1
        self.lists["an item no source gives, made as ours of its kind (OURS)"].append(f"{tag}, {why}")
        return True

    def weapon_figures(self, attacktype, level):
        """(damage, item HP, span) of a weapon of the type at the level: the
        median of ours within 2 levels, else 5, else 10 (three at least);
        tools/fix_dump_weapons.py's rule."""
        for span in (2, 5, 10):
            near = [(r["weapondamage"], r["maxhp"]) for r in self.rows.values()
                    if r["attacktype"] == attacktype and (r["weapondamage"] or 0) > 0 and (r["levelreq"] or 0) > 0
                    and abs(r["levelreq"] - level) <= span]
            if len(near) >= 3:
                dmg = sorted(d for d, _ in near)
                hp = sorted(h for _, h in near if h)
                return dmg[len(dmg) // 2], (hp[len(hp) // 2] if hp else 0), span
        return None


def token_swap(text, old, new):
    """A spell id swapped for another in a list of ids ("1517, 700308")."""
    return re.sub(rf"(?<!\d){old}(?!\d)", str(new), text or "")


def plan_splits(db, wb, smap, splits, records, links, lists, counts):
    """Each split row's record gets the classes that were its row's on the
    shared one (the coordinator, 2026-11-01): their classspells rows moved
    over (the record's other classes keep theirs), and what a character of
    such a class holds of the shared record: its book's entry (the bar slot
    kept), its buffs kept over a logout (the caster's class), the masteries
    of those classes alone that give, need or take it (cmcatalog), and the
    quest rewards of those classes alone (queststeps' spells and choices) or
    for them (byclass). A row whose classes the record's own row has too
    adds its classes and moves nothing."""
    moves, migrations = [], []
    if not splits:
        return moves, migrations
    pending = {}   # (tbl, key) -> the migration of that row: one row, every split's swap on it

    def migrate(tbl, key, col, value_now, swap, tag, count):
        """The row's column swapped as its migration so far leaves it."""
        k = (tbl, tuple(sorted(key.items())))
        m = pending.get(k)
        if m is None:
            m = pending[k] = dict(tbl=tbl, key=key, before={}, set={}, note=tag)
            migrations.append(m)
            counts[count] += 1
        elif tag not in m["note"]:
            m["note"] += "; " + tag
        if col not in m["before"]:
            m["before"][col] = value_now
        new = swap(m["set"].get(col, value_now))
        if new != m["before"][col]:
            m["set"][col] = new
        else:
            m["set"].pop(col, None)
    owners = collections.defaultdict(set)
    for s in wb.spells:
        m = map_entry(smap, hexid(s.get("SpellID")))
        if m and m[1].split(" ")[0] != "split":
            owners[m[0]] |= set(class_mask(cell(s, "CLASS")))
    cs = {(c, sid): src for c, sid, src in db.execute("SELECT class, spellid, source FROM main.classspells")}
    tclass = dict(db.execute("SELECT serverid, tclass FROM main.characters")) if has_table(db, "characters") else {}
    qclasses = {}
    if has_table(db, "quests"):
        for qid, cl in db.execute("SELECT questid, classes FROM main.quests"):
            qclasses[qid] = {int(x) for x in re.findall(r"\d+", cl or "")}
    for x in splits:
        n, r = x["new"], x["shared"]
        if r is None:
            lists["a split row naming no record it leaves"].append(f"{n} {cell(x['row'], 'Name')}")
            continue
        moved = set(x["classes"]) - owners[r]
        mask = sum(1 << c for c in moved)
        tag = f"{n} {cell(x['row'], 'Name')} from {r}"
        counts["records split off a shared one"] += 1
        for c in x["classes"]:
            if x["were"]:
                continue
            if c in moved and (c, r) in cs:
                moves.append(dict(cls=c, spellid=r, source=cs[(c, r)]))
                links.append(dict(cls=c, spellid=n, source=cs[(c, r)]))
                counts["  class rows moved to the split record"] += 1
            else:
                links.append(dict(cls=c, spellid=n, source=x["source"]))
        if not moved:
            continue
        for rowid, sid in db.execute("SELECT id, serverid FROM main.spells WHERE spellid = ?", (r,)).fetchall():
            if tclass.get(sid) in moved:
                migrate("spells", {"id": rowid}, "spellid", r, lambda v: n, tag, "  books moved to the split record")
        if has_table(db, "charbuffs"):
            for sid, seq, caster in db.execute("SELECT serverid, seq, caster FROM main.charbuffs WHERE spellid = ?",
                                               (r,)).fetchall():
                if tclass.get(caster, tclass.get(sid)) in moved:
                    migrate("charbuffs", {"serverid": sid, "seq": seq}, "spellid", r, lambda v: n, tag,
                            "  kept buffs moved to the split record")
        if has_table(db, "cmcatalog"):
            cols = [c for c in ("add_spells", "require_spells", "remove_spells") if c in columns(db, "cmcatalog")]
            for row in db.execute(f"SELECT id, classes, name, rank, {', '.join(cols)} FROM main.cmcatalog").fetchall():
                cid, cm_mask, name, rank, vals = row[0], row[1], row[2], row[3], dict(zip(cols, row[4:]))
                hit = {c: v for c, v in vals.items() if token_swap(v, r, n) != (v or "")}
                if not hit:
                    continue
                if cm_mask and cm_mask & ~mask == 0:
                    for c, v in hit.items():
                        migrate("cmcatalog", {"id": cid}, c, v, lambda w, r=r, n=n: token_swap(w, r, n), tag,
                                "  masteries moved to the split record")
                elif cm_mask & mask:
                    lists["a mastery of a split record's classes and others: left naming the shared record"].append(
                        f"{cid} {name} {rank} ({', '.join(hit)}): {tag}")
        if has_table(db, "queststeps"):
            qcols = [c for c in ("spells", "byclass", "choices") if c in columns(db, "queststeps")]
            for row in db.execute(f"SELECT questid, step, {', '.join(qcols)} FROM main.queststeps").fetchall():
                qid, step, vals = row[0], row[1], dict(zip(qcols, row[2:]))
                if not any(token_swap(v, r, n) != (v or "") for v in vals.values()):
                    continue
                qc = qclasses.get(qid, set())
                theirs = bool(qc) and qc <= moved
                want = {}
                if theirs and "spells" in vals and token_swap(vals["spells"], r, n) != (vals["spells"] or ""):
                    want["spells"] = token_swap(vals["spells"], r, n)
                if theirs and vals.get("choices"):
                    want["choices"] = swap_json_spells(vals["choices"], r, n, None)
                if vals.get("byclass"):
                    want["byclass"] = swap_json_spells(vals["byclass"], r, n, moved)
                want = {k: v for k, v in want.items() if v != (vals.get(k) or "")}
                for k in want:
                    swap = (lambda w, r=r, n=n: token_swap(w, r, n)) if k == "spells" else \
                        (lambda w, r=r, n=n, moved=moved: swap_json_spells(w, r, n, moved)) if k == "byclass" else \
                        (lambda w, r=r, n=n: swap_json_spells(w, r, n, None))
                    migrate("queststeps", {"questid": qid, "step": step}, k, vals.get(k), swap, tag,
                            "  quest rewards moved to the split record")
                if not theirs and any(token_swap(vals.get(k), r, n) != (vals.get(k) or "") for k in ("spells", "choices")):
                    if qc & moved:
                        lists["a quest of a split record's classes and others: left giving the shared record"].append(
                            f"quest {qid} step {step}: {tag}")
    migrations[:] = [m for m in migrations if m["set"]]
    return moves, migrations


def swap_json_spells(text, old, new, classes):
    """A byclass or choices column with the id swapped in its "spells":
    byclass's entries of those classes alone (classes None: every option)."""
    try:
        o = json.loads(text)
    except ValueError:
        return text
    hit = False
    if isinstance(o, dict) and classes is not None:
        for k, v in o.items():
            if isinstance(v, dict) and k.strip().isdigit() and int(k) in classes and "spells" in v:
                w = token_swap(str(v["spells"]), old, new)
                if w != str(v["spells"]):
                    v["spells"], hit = w, True
    elif isinstance(o, dict) and classes is None:
        for opt in o.get("options") or []:
            if isinstance(opt, dict) and "spells" in opt:
                w = token_swap(str(opt["spells"]), old, new)
                if w != str(opt["spells"]):
                    opt["spells"], hit = w, True
    return json.dumps(o, separators=(",", ":"), ensure_ascii=False) if hit else text


def one_letter_apart(a, b):
    """Whether two names of ten letters or more differ by one letter put in,
    taken out or changed ("mossvale circle", "mossvole circle")."""
    if a == b or min(len(a), len(b)) < 10 or abs(len(a) - len(b)) > 1:
        return False
    if len(a) > len(b):
        a, b = b, a
    i = 0
    while i < len(a) and a[i] == b[i]:
        i += 1
    return a[i + (len(a) == len(b)):] == b[i + 1:]


def plan_gates(db, gates, lists, counts):
    """A new teleport's spellgates row where its Location is a place: the
    world it names and its point, a group's the spell's radius (gates.go);
    a Location several differently named teleports share is the workbook's
    stand-in (listed), and a name a row already has keeps that row."""
    out = []
    if not gates:
        return out
    have = set()
    if has_table(db, "spellgates"):
        have = {r[0] for r in db.execute("SELECT spell FROM main.spellgates")}
    def place_of(name):
        """A teleport's place as its name says it: the spell's words gone."""
        n = re.sub(r"(?i)\b(portal|escape|gate|circle|evacuation)\b", " ", name)
        return " ".join(n.split())
    places = collections.defaultdict(set)
    for g in gates:
        if g["location"]:
            places[g["location"]].add(place_of(g["name"]).lower())
    known, rows = [], {}
    if has_table(db, "spellgates"):
        known = db.execute("SELECT place, world, x, z FROM main.spellgates WHERE place <> ''").fetchall()
        cols = columns(db, "spellgates")
        rows = {r[0]: dict(zip(cols, r)) for r in db.execute(f"SELECT {', '.join(cols)} FROM main.spellgates WHERE place <> ''")}
    for g in gates:
        key = g["name"].strip().lower()
        tag = f"{g['spellid']} {g['name']}"
        parts = [x.strip() for x in g["location"].split(",")]
        world = WORLDS.get(parts[0].lower()) if parts else None
        try:
            x, y, z = (float(v) for v in parts[1:4])
        except (ValueError, IndexError):
            x = y = z = None
        twins = [k for k in rows if one_letter_apart(key, k)] if world is None or x is None else []
        if key in have:
            lists["a new teleport whose name a spellgates row has: that row kept"].append(tag)
        elif len(twins) == 1:
            # the workbook's second record of a circle we have, spelled one
            # letter otherwise and with no place of its own (a druid circle's
            # level 60 twin for every race): its twin's place
            row = dict(rows[twins[0]])
            row.update(spell=key, note=f"{NOTE}: {g['name']}, spelled as {twins[0]!r} is but for one letter, its "
                                       f"Location {g['location'] or 'none'}: that row's place", source="eqoaemu")
            out.append(row)
            have.add(key)
            lists["a new teleport with no place named as a spellgates row is but for one letter: that row's place"].append(
                f"{tag} = {twins[0]} ({row['place']})")
        elif world is None or x is None:
            lists["a new teleport with no place in its Location (no spellgates row)"].append(
                f"{tag}: {g['location'] or 'none'}")
        elif len(places[g["location"]]) > 1:
            lists["a new teleport whose Location differently named teleports share (the workbook's stand-in: no row)"].append(
                f"{tag}: {g['location']}")
        else:
            near = [k for k in known if k[1] == world and abs(k[2] - x) < 15 and abs(k[3] - z) < 15]
            place = near[0][0] if near else place_of(g["name"]) or g["name"].strip()
            out.append(dict(spell=key, kind="group" if g["group"] else "self", place=place, world=world,
                            x=round(x, 1), y=round(y, 1), z=round(z, 1), radius=float(g["radius"] or 0),
                            note=f"{NOTE}: Location {g['location']}", source="eqoaemu"))
            have.add(key)
    counts["spell gates made"] = len(out)
    return out


def changes_of(p):
    """Every planned write as one list, in the order --apply makes them."""
    out = []
    for tbl, items in (("spellpattern", p["spells"]), ("spelleffect", p["effects"]), ("spellarea", p["areas"])):
        for e in items:
            if e["action"] == "insert":
                out.append(dict(tbl=tbl, action="insert", key={"spellid": e["spellid"]}, row=e["row"]))
            else:
                out.append(dict(tbl=tbl, action="update", key={"spellid": e["spellid"]}, before=e["before"], set=e["set"]))
    for m in p["moves"]:
        out.append(dict(tbl="classspells", action="delete", key={"class": m["cls"], "spellid": m["spellid"]},
                        before={"class": m["cls"], "spellid": m["spellid"], "source": m["source"]}))
    for x in p["links"]:
        out.append(dict(tbl="classspells", action="insert", key={"class": x["cls"], "spellid": x["spellid"]},
                        row={"class": x["cls"], "spellid": x["spellid"], "source": x["source"]}))
    for x in p["scrolls"]:
        out.append(dict(tbl="itempattern", action="insert", key={"patternid": x["patternid"]}, row=x["row"]))
    for x in (p.get("foods") or {}).get("items", []):
        out.append(dict(tbl="itempattern", action="insert", key={"patternid": x["patternid"]}, row=x["row"]))
    for x in p["stock"]:
        out.append(dict(tbl="npcitem", action="insert", key=None, note=x["name"],
                        row={"npcid": x["npcid"], "stackleft": 1, "remainHP": 0, "remaincharge": 0,
                             "patternid": x["patternid"], "equiploc": -1, "location": 1}))
    for m in p["migrations"]:
        out.append(dict(tbl=m["tbl"], action="update", key=m["key"], before=m["before"], set=m["set"],
                        note=m.get("note", "")))
    for g in p["gates"]:
        out.append(dict(tbl="spellgates", action="insert", key={"spell": g["spell"]}, row=g))
    f = p.get("foods") or {}
    for x in f.get("updates", []):
        out.append(dict(tbl="itempattern", action="update", key={"patternid": x["patternid"]}, before=x["before"],
                        set=x["set"], note=x["name"]))
    for x in f.get("foodeffects", []):
        out.append(dict(tbl="foodeffect", action="insert", key={"patternid": x["patternid"]}, row=x))
    for x in f.get("itemeffects", []):
        out.append(dict(tbl="itemeffect", action="insert", key={"patternid": x["patternid"]}, row=x))
    m = p.get("made") or {}
    for x in m.get("items", []):
        out.append(dict(tbl="itempattern", action="insert", key={"patternid": x["patternid"]}, row=x["row"],
                        note=f"made as {x['sibling']}"))
    for x in m.get("updates", []):
        out.append(dict(tbl="itempattern", action="update", key={"patternid": x["patternid"]}, before=x["before"],
                        set=x["set"], note=x["name"]))
    for x in m.get("itemeffects", []):
        out.append(dict(tbl="itemeffect", action="insert", key={"patternid": x["patternid"]}, row=x))
    for x in m.get("itemeffect_updates", []):
        out.append(dict(tbl="itemeffect", action="update", key={"patternid": x["patternid"]}, before=x["before"],
                        set=x["set"], note=x["name"]))
    for x in m.get("foodeffects", []):
        out.append(dict(tbl="foodeffect", action="insert", key={"patternid": x["patternid"]}, row=x))
    return out


def names_of(records, written):
    """exact_key of a name -> the records of it: ours, and the ones this plan
    makes (by their sheet rows' names)."""
    by_name = collections.defaultdict(list)
    for sid, r in records.items():
        by_name[exact_key(r["spellname"])].append(sid)
    for sid, (s, _) in written.items():
        if sid not in records:
            by_name[exact_key(cell(s, "Name"))].append(sid)
    return by_name


def plan_mirrors(records, erows, written, spells, effects, lists, counts):
    """MIRRORS: each spell of ours the workbook lacks made the mirror of an
    original a workbook row writes, as this plan writes it."""
    by_name = names_of(records, written)
    for name, (orig, hate) in sorted(MIRRORS.items()):
        mine, theirs = by_name.get(exact_key(name), []), [x for x in by_name.get(exact_key(orig), []) if x in written]
        if len(mine) != 1 or len(theirs) != 1 or mine[0] in written:
            lists["a mirror whose spell or original is not one record here (left)"].append(
                f"{name} ({mine}) of {orig} ({theirs})")
            continue
        sid, osid = mine[0], theirs[0]
        s, j = written[osid]
        record, eff = mirror_changes(records[sid], erows.get(sid), SCOPE.get(cell(s, "Scope")),
                                     FRAMES.get(cell(s, "IconBGDColor")), cell(s, "Name"), j, hate)
        if record:
            spells.append(dict(spellid=sid, action="update", sheet_id="", name=records[sid]["spellname"], **record))
            counts["records made mirrors"] += 1
        if eff and sid in erows:
            effects.append(dict(spellid=sid, action="update", **eff))
            counts["effects rows made mirrors"] += 1
        elif eff:
            effects.append(dict(spellid=sid, action="insert", row=dict(spellid=sid, fxdict=0, **eff["set"])))
            counts["effects rows made mirrors"] += 1


def plan_ours(records, erows, written, items_by_name, item_names, spells, effects, lists, counts):
    """MAKES and CURES: each spell of ours the workbook lacks given the
    effects of ours its table says, as this plan writes its maker."""
    by_name = names_of(records, written)
    todo = []
    for name, (maker, item, count) in sorted(MAKES.items()):
        mine, theirs = by_name.get(exact_key(name), []), [x for x in by_name.get(exact_key(maker), []) if x in written]
        pid = items_by_name.get(fold(item))
        if len(mine) != 1 or len(theirs) != 1 or mine[0] in written or pid is None:
            lists["a spell of ours to make an item whose record, maker or item is not one here (left)"].append(
                f"{name} ({mine}) like {maker} ({theirs}): {item} ({pid})")
            continue
        s, j = written[theirs[0]]
        rec = records[mine[0]]
        todo.append((mine[0], name, makes_json(j, rec["abilitylvl"], item_names.get(pid, item), pid, count),
                     f"effects ours: {cell(s, 'Name')}'s, making our {item_names.get(pid, item)}"))
    for name, n in sorted(CURES.items()):
        mine = by_name.get(exact_key(name), [])
        if len(mine) != 1 or mine[0] in written:
            lists["a spell of ours to cure whose record is not one here (left)"].append(f"{name} ({mine})")
            continue
        rec = records[mine[0]]
        todo.append((mine[0], name, cures_json(rec["scope"], rec["abilitylvl"], n), f"effects ours: a cure of up to {n}"))
    for sid, name, j, note in todo:
        record, eff = ours_changes(records[sid], erows.get(sid), {"spelldesc": OURS_DESC.get(name)}, j, note, item_names)
        if record:
            spells.append(dict(spellid=sid, action="update", sheet_id="", name=records[sid]["spellname"], **record))
            counts["records given effects of ours"] += 1
        if eff and sid in erows:
            effects.append(dict(spellid=sid, action="update", **eff))
            counts["effects rows given effects of ours"] += 1
        elif eff:
            effects.append(dict(spellid=sid, action="insert", row=dict(spellid=sid, fxdict=0, **eff["set"])))
            counts["effects rows given effects of ours"] += 1


def plan_scrolls(db, new_rows, records, lists, counts, links=(), moves=(), migrations=()):
    """The new spells' ways in: a vendor spell's scroll and its merchants; the
    quest and drop spells listed. A scroll is judged as scribe.go will read
    it after the import: each class's spells as classspells will hold them."""
    names = sorted([(sid, r["spellname"], r["abilitylvl"]) for sid, r in records.items()] +
                   [(sid, cell(s, "Name"), num(s, "Level") or 1) for sid, (s, _, _) in new_rows.items()])
    after = collections.defaultdict(set)
    for c, sid in db.execute("SELECT class, spellid FROM main.classspells"):
        after[c].add(sid)
    for m in moves:
        after[m["cls"]].discard(m["spellid"])
    for x in links:
        after[x["cls"]].add(x["spellid"])

    def teaches_for(r, classes, sid):
        """Whether a scroll row teaches spell sid to a class of these it admits."""
        return any(r["classuse"] >> c & 1 and scroll_spell(names, r["itemname"], r["levelreq"], after[c].__contains__) == sid
                   for c in classes)
    icols = columns(db, "itempattern")
    scroll_rows = [dict(zip(icols, r)) for r in
                   db.execute("SELECT * FROM main.itempattern WHERE itemicon = ? AND equipslot = -1", (SCROLL_ICON,))]
    by_key = collections.defaultdict(list)
    for r in scroll_rows:
        by_key[spell_key(r["itemname"])].append(r)
    taken = {r[0] for r in db.execute("SELECT patternid FROM main.itempattern")}
    q_items, q_spells = quest_rewards(db)
    for m in migrations:   # the rewards a split moved to its record
        if m["tbl"] == "queststeps":
            for v in m["set"].values():
                q_spells.update(int(x) for x in re.findall(r"\d+", v or ""))
    price = scroll_curve(db)
    # vendor bands: npcid -> class -> levels of its scrolls of the class; and the size of each catalog
    bands = collections.defaultdict(lambda: collections.defaultdict(list))
    for npcid, cu, lv in db.execute(
            "SELECT i.npcid, p.classuse, p.levelreq FROM main.npcitem i JOIN main.itempattern p ON p.patternid = i.patternid "
            "WHERE p.itemicon = ? AND p.equipslot = -1 AND p.classuse <> ?", (SCROLL_ICON, ALL_CLASSES)):
        for c in range(len(CLASSES)):
            if cu >> c & 1:
                bands[npcid][c].append(lv)
    size = collections.Counter(dict(db.execute("SELECT npcid, count(*) FROM main.npcitem GROUP BY npcid").fetchall()))
    nextlist = dict(db.execute("SELECT npcid, coalesce(max(listnumber), -1) + 1 FROM main.npcitem GROUP BY npcid").fetchall())
    scrolls, stock = [], []
    for sid, (s, j, were) in sorted(new_rows.items()):
        classes = class_mask(cell(s, "CLASS"))
        if not classes:
            counts["new spells of no class (a pet's or a creature's)"] += 1
            continue
        if were:
            continue  # its quest or mastery gives it
        learn, how = cell(s, "Learn Method"), cell(s, "ScrollSource")
        name, level = cell(s, "Name"), num(s, "Level") or 1
        tag = f"{sid} {name} ({cell(s, 'CLASS')} L{level})"
        if learn == "CM":
            counts["new mastery spells (the masteries lane's)"] += 1
            continue
        if learn != "Scroll":
            lists["a new class spell with no Learn Method"].append(tag)
            continue
        existing = by_key.get(spell_key(name), [])
        teaches = [r for r in existing if teaches_for(r, classes, sid)]
        if how == "Quest":
            given = [r["patternid"] for r in teaches if r["patternid"] in q_items] + ([sid] if sid in q_spells else [])
            if given:
                counts["new quest spells a quest's reward already gives"] += 1
            else:
                lists["a new quest spell no quest gives (Scroll + Quest)"].append(tag + (
                    f"; a scroll of its name scribes {scroll_spell(names, existing[0]['itemname'], existing[0]['levelreq'])}"
                    if existing and not teaches else ""))
            continue
        if how == "Drop":
            lists["a new drop spell (Scroll + Drop; no loot changed)"].append(tag)
            continue
        if how != "Vendor":
            lists[f"a new scroll spell from {how or 'nowhere'}"].append(tag)
            continue
        # each class: a scroll of ours that admits it teaches it (covered),
        # one admits it and teaches another record (listed), or none admits it
        covered = {c for c in classes if any(r["classuse"] >> c & 1 and scroll_spell(
            names, r["itemname"], r["levelreq"], after[c].__contains__) == sid for r in existing)}
        clash = {c: [r for r in existing if r["classuse"] >> c & 1] for c in classes if c not in covered}
        clash = {c: rs for c, rs in clash.items() if rs}
        for c, rs in sorted(clash.items()):
            other = scroll_spell(names, rs[0]["itemname"], rs[0]["levelreq"], after[c].__contains__)
            lists["a new vendor spell whose name's scroll scribes another record for its class"].append(
                tag + f": {next(k for k, v in CLASSES.items() if v == c)}, scroll {rs[0]['patternid']} "
                f"{rs[0]['itemname']!r} scribes {other}")
        classes = [c for c in classes if c not in covered and c not in clash]
        if not classes:
            if covered:
                counts["new vendor spells a scroll of ours already teaches"] += 1
                lists["a new vendor spell a scroll of ours teaches (no scroll made)"].append(
                    tag + f": scroll {', '.join(str(r['patternid']) for r in teaches)}")
            continue
        wrong = {c: scroll_spell(names, name, level, after[c].__contains__) for c in classes}
        wrong = {c: x for c, x in wrong.items() if x != sid}
        if wrong:
            lists["a new vendor spell a scroll of its name would not scribe (scribe.go's key)"].append(
                tag + f": scribes {sorted(set(wrong.values()))}")
            continue
        pid = sid
        if pid in taken:
            lists["a scroll id taken"].append(tag)
            continue
        row = {c: None for c in icols}
        row.update(patternid=pid, itemcost=price(level), unk1=0, itemicon=SCROLL_ICON, unk2=0, equipslot=-1, unk3=0,
                   trade=0, rent=1, unk4=0, attacktype=0, weapondamage=0, unk5=0, levelreq=level, maxstack=1,
                   maxhp=0, duration=0, classuse=sum(1 << c for c in classes), raceuse=race_mask(cell(s, "RACE")),
                   procanim=2, lore=0, unk6=0, craft=0, itemname=name,
                   itemdesc=f"This scroll will allow you to learn {name}.", model=0, color=0)
        scrolls.append(dict(patternid=pid, row={k: v for k, v in row.items() if k in icols}))
        taken.add(pid)
        by_key[spell_key(name)].append(row)
        vendors = set()
        for c in classes:
            cands = []
            for npcid, per in bands.items():
                lv = per.get(c)
                if not lv or len(lv) < 3:
                    continue
                lo, hi = min(lv), max(lv)
                cands.append((0 if lo <= level <= hi else min(abs(level - lo), abs(level - hi)), npcid))
            if cands:
                best = min(d for d, _ in cands)
                vendors |= {n for d, n in cands if d == best}
        sold = 0
        for npcid in sorted(vendors):
            if size[npcid] >= LIST_CAP:
                lists["a merchant's list full (40): a scroll not stocked there"].append(f"{tag} at {npcid}")
                continue
            stock.append(dict(npcid=npcid, patternid=pid, listnumber=nextlist.get(npcid, 0), name=name))
            nextlist[npcid] = nextlist.get(npcid, 0) + 1
            size[npcid] += 1
            sold += 1
        if not sold:
            lists["a new vendor spell's scroll no merchant sells"].append(tag)
    counts["scrolls made"] = len(scrolls)
    counts["merchant rows"] = len(stock)
    counts["merchants stocked"] = len({x["npcid"] for x in stock})
    return scrolls, stock


REPORT_ORDER = ["records made", "records updated", "  abilitylvl changed", "  range changed", "  casttime changed",
                "  recast changed", "  power changed", "  scope changed", "  icon changed", "  iconColor changed",
                "  spellname changed", "records as the workbook has them already",
                "effects rows made", "effects rows written", "no effect in the workbook: line kept", "areas made",
                "areas changed", "class rows", "new spells of no class (a pet's or a creature's)",
                "new mastery spells (the masteries lane's)", "new quest spells a quest's reward already gives",
                "new vendor spells a scroll of ours already teaches", "scrolls made", "merchant rows",
                "merchants stocked"]


def were_quest_check(p, smap, wb, weres):
    """Each were quest's reward spell: found in the workbook (and so given a
    record here or ours already), found under another spelling the quest
    importer does not resolve, or not in the workbook at all."""
    sheet = collections.defaultdict(list)
    for s in wb.spells:
        sheet[quest_fold(cell(s, "Name"))].append(s)
    near = {re.sub(r"(.)\1", r"\1", k): k for k in sheet}  # one letter doubled: "Reptillian"
    out = {"found": [], "spelled differently": [], "missing": []}
    for k, (name, key) in sorted(weres.items(), key=lambda kv: kv[1][1]):
        rows = sheet.get(k)
        if rows:
            ids = sorted({smap.get(hexid(r.get("SpellID")), (None, ""))[0] for r in rows} - {None})
            out["found"].append(f"{name} ({key}): {', '.join(map(str, ids))}")
            continue
        other = near.get(re.sub(r"(.)\1", r"\1", k))
        if other:
            r = sheet[other][0]
            out["spelled differently"].append(f"{name} ({key}): the workbook's {cell(r, 'Name')!r}, "
                                              f"{smap.get(hexid(r.get('SpellID')), (None, ''))[0]}")
        else:
            out["missing"].append(f"{name} ({key})")
    return out


def report(p, full=False):
    c = p["counts"]
    for k in REPORT_ORDER + sorted(k for k in c if k not in REPORT_ORDER):
        if k in c:
            print(f"  {k}: {c[k]}")
    if p["effect_clash"]:
        print(f"  EffectIDs twice, differing (the first kept): {', '.join(p['effect_clash'])}")
    for k, v in p["lists"].items():
        print(f"  {k}: {len(v)}")
        for x in (v if full else v[:5]):
            print(f"      {x}")
        if not full and len(v) > 5:
            print("      ...")
    for k, v in (p.get("were_quest_spells") or {}).items():
        print(f"  the were quests' reward spells, {k}: {len(v)}")
        for x in (v if full or k != "found" else v[:5]):
            print(f"      {x}")


# ---- writing it -------------------------------------------------------------------

def where_of(tbl, key):
    cols = KEYS[tbl]
    return " AND ".join(f"{c} = ?" for c in cols), [key[c] for c in cols]


def row_now(db, tbl, key, cols):
    """The row's columns now, or None for no row."""
    w, args = where_of(tbl, key)
    r = db.execute(f"SELECT {', '.join(cols)} FROM main.{tbl} WHERE {w}", args).fetchone()
    return None if r is None else dict(zip(cols, r))


def log(db, tbl, key, action, before, after, note="", table=LOG):
    db.execute(f"INSERT INTO {table} (tbl, key, action, before, after, note) VALUES (?, ?, ?, ?, ?, ?)",
               (tbl, dumps(key), action, None if before is None else dumps(before),
                None if after is None else dumps(after), note or ""))


def insert(db, table, row):
    cols = [c for c in columns(db, table) if c in row]
    return db.execute(f"INSERT INTO main.{table} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                      [row[c] for c in cols])


# database/spellgates.go's table, for a database that lacks it
SPELLGATES_DDL = """CREATE TABLE spellgates (spell TEXT PRIMARY KEY, kind TEXT NOT NULL DEFAULT 'self',
    place TEXT NOT NULL DEFAULT '', world INTEGER NOT NULL DEFAULT 0, x REAL NOT NULL DEFAULT 0,
    y REAL NOT NULL DEFAULT 0, z REAL NOT NULL DEFAULT 0, radius REAL NOT NULL DEFAULT 0,
    note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm')"""


# database/foodeffect.go's table, for a database that lacks it
FOODEFFECT_DDL = """CREATE TABLE foodeffect (patternid INTEGER PRIMARY KEY, duration_s INTEGER NOT NULL DEFAULT 0,
    ends_on_cast INTEGER NOT NULL DEFAULT 0, ends_on_contact INTEGER NOT NULL DEFAULT 0,
    source TEXT NOT NULL DEFAULT 'gm')"""


def apply(db, p, log_table=LOG, meta_table=META, what="an EQOAEmu spell import"):
    """Every planned change whose row is still as planned, in one
    transaction, each logged (in log_table: the import's, or another tool's
    that plans as it does); returns (done, changed since)."""
    db.executescript(log_ddl(log_table, meta_table))
    if db.execute(f"SELECT count(*) FROM {log_table}").fetchone()[0]:
        sys.exit(f"this database already holds {what}; --undo it first")
    changes = p.get("changes") or changes_of(p)
    done = stale = 0
    db.execute("BEGIN IMMEDIATE")
    try:
        seqs = dict(db.execute("SELECT name, seq FROM sqlite_sequence WHERE name IN "
                               "('spellpattern', 'itempattern', 'npcitem')").fetchall())
        added = "effects" not in columns(db, "spelleffect")
        if added:
            db.execute("ALTER TABLE main.spelleffect ADD COLUMN effects TEXT NOT NULL DEFAULT ''")
        # the use's columns a change writes that a database made before them
        # lacks (the server adds them at its start: database/itemeffect.go)
        added_uses = []
        if has_table(db, "itemeffect"):
            want = set()
            for c in changes:
                if c["tbl"] == "itemeffect":
                    for part in ("row", "set", "before"):
                        want |= set(c.get(part) or {})
            have = set(columns(db, "itemeffect"))
            for col, decl in ITEMEFFECT_ADDED:
                if col in want and col not in have:
                    db.execute(f"ALTER TABLE main.itemeffect ADD COLUMN {col} {decl}")
                    added_uses.append(col)
        made = {}
        for tbl, ddl in (("spellarea", "CREATE TABLE spellarea (spellid INTEGER PRIMARY KEY, centre INTEGER NOT NULL "
                                       "DEFAULT 0, radius REAL NOT NULL DEFAULT 0, maxtargets INTEGER NOT NULL DEFAULT 0, "
                                       "note TEXT NOT NULL DEFAULT '')"),
                         ("spellgates", SPELLGATES_DDL), ("foodeffect", FOODEFFECT_DDL)):
            made[tbl] = not has_table(db, tbl) and any(c["tbl"] == tbl for c in changes)
            if made[tbl]:
                db.execute(ddl)
        scrolls = set()
        for c in changes:
            tbl, act = c["tbl"], c["action"]
            if not has_table(db, tbl):
                stale += 1
                continue
            if act == "insert" and tbl == "npcitem":
                row = dict(c["row"])
                n = db.execute("SELECT count(*), coalesce(max(listnumber), -1) + 1 FROM main.npcitem WHERE npcid = ?",
                               (row["npcid"],)).fetchone()
                if row["patternid"] not in scrolls or n[0] >= LIST_CAP:
                    stale += 1
                    continue
                row["listnumber"] = n[1]
                cur = insert(db, tbl, row)
                log(db, tbl, {"itemid": cur.lastrowid}, "insert", None, row, c.get("note"), log_table)
            elif act == "insert":
                if row_now(db, tbl, c["key"], KEYS[tbl]) is not None:
                    stale += 1
                    continue
                insert(db, tbl, c["row"])
                log(db, tbl, c["key"], "insert", None, c["row"], c.get("note"), log_table)
                if tbl == "itempattern":
                    scrolls.add(c["key"]["patternid"])
            elif act == "update":
                if row_now(db, tbl, c["key"], list(c["before"])) != c["before"]:
                    stale += 1
                    continue
                w, args = where_of(tbl, c["key"])
                db.execute(f"UPDATE main.{tbl} SET {', '.join(f'{k} = ?' for k in c['set'])} WHERE {w}",
                           [*c["set"].values(), *args])
                log(db, tbl, c["key"], "update", c["before"], c["set"], c.get("note"), log_table)
            elif act == "delete":
                if row_now(db, tbl, c["key"], list(c["before"])) != c["before"]:
                    stale += 1
                    continue
                w, args = where_of(tbl, c["key"])
                # its rowid too, so --undo puts it back where it was (a dump's order)
                rid = db.execute(f"SELECT rowid FROM main.{tbl} WHERE {w}", args).fetchone()[0]
                db.execute(f"DELETE FROM main.{tbl} WHERE {w}", args)
                log(db, tbl, c["key"], "delete", dict(c["before"], rowid=rid), None, c.get("note"), log_table)
            done += 1
        now = datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")
        for k, v in {"applied_at": now, "sqlite_sequence": dumps(seqs), "added_effects_column": str(int(added)),
                     "made_spellarea": str(int(made["spellarea"])), "made_spellgates": str(int(made["spellgates"])),
                     "made_foodeffect": str(int(made["foodeffect"])), "added_itemeffect_columns": ",".join(added_uses),
                     "counts": dumps(p["counts"])}.items():
            db.execute(f"INSERT OR REPLACE INTO {meta_table} VALUES (?, ?)", (k, v))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return done, stale


def undo(db, force=False, log_table=LOG, meta_table=META, what="EQOAEmu spell import"):
    """Every logged row still as written put back, last first; returns
    (back, left, what was left)."""
    if not has_table(db, log_table):
        sys.exit(f"no {what} to undo here")
    entries = [(t, json.loads(k), a, None if b is None else json.loads(b), None if f is None else json.loads(f))
               for t, k, a, b, f in db.execute(f"SELECT tbl, key, action, before, after FROM {log_table} ORDER BY seq DESC")]
    made_spells = [k["spellid"] for t, k, a, _, _ in entries if t == "spellpattern" and a == "insert"]
    made_items = [k["patternid"] for t, k, a, _, _ in entries if t == "itempattern" and a == "insert"]
    moved_books = [k["id"] for t, k, a, _, _ in entries if t == "spells"]

    def marks(n):
        return ",".join("?" * n)
    held_q = (f"FROM main.spells WHERE spellid IN ({marks(len(made_spells))}) AND id NOT IN ({marks(len(moved_books))})",
              made_spells + moved_books)
    held = {}
    if made_spells:
        held["spells"] = db.execute("SELECT count(*) " + held_q[0], held_q[1]).fetchone()[0]
    if made_items and has_table(db, "charinventory"):
        held["charinventory"] = db.execute(f"SELECT count(*) FROM main.charinventory WHERE patternid IN "
                                           f"({marks(len(made_items))})", made_items).fetchone()[0]
    if any(held.values()) and not force:
        sys.exit(f"characters hold made spells or scrolls ({held}); --force to take them out with the import")
    back = left = 0
    leftover = []
    db.execute("BEGIN IMMEDIATE")
    try:
        if force:
            if made_spells:
                db.execute("DELETE " + held_q[0], held_q[1])
            if made_items and has_table(db, "charinventory"):
                db.execute(f"DELETE FROM main.charinventory WHERE patternid IN ({marks(len(made_items))})", made_items)
        for tbl, key, act, before, after in entries:
            w, args = where_of(tbl, key)
            if act == "insert":
                if row_now(db, tbl, key, list(after)) != after:
                    left += 1
                    leftover.append(f"{tbl} {dumps(key)}")
                    continue
                db.execute(f"DELETE FROM main.{tbl} WHERE {w}", args)
            elif act == "update":
                if row_now(db, tbl, key, list(after)) != after:
                    left += 1
                    leftover.append(f"{tbl} {dumps(key)}")
                    continue
                db.execute(f"UPDATE main.{tbl} SET {', '.join(f'{k} = ?' for k in before)} WHERE {w}",
                           [*before.values(), *args])
            else:
                cols = [k for k in before if k != "rowid"]
                now = row_now(db, tbl, key, cols)
                if now is not None:
                    if now != {k: before[k] for k in cols}:
                        left += 1
                        leftover.append(f"{tbl} {dumps(key)}")
                        continue
                elif "rowid" in before and not db.execute(f"SELECT 1 FROM main.{tbl} WHERE rowid = ?",
                                                          (before["rowid"],)).fetchone():
                    db.execute(f"INSERT INTO main.{tbl} (rowid, {', '.join(cols)}) VALUES (?, {', '.join('?' * len(cols))})",
                               [before["rowid"], *(before[k] for k in cols)])
                else:
                    insert(db, tbl, {k: before[k] for k in cols})
            back += 1
        meta = dict(db.execute(f"SELECT key, value FROM {meta_table}").fetchall())
        seqs = json.loads(meta.get("sqlite_sequence", "{}"))
        for t, col in (("spellpattern", "spellid"), ("itempattern", "patternid"), ("npcitem", "itemid")):
            top = db.execute(f"SELECT coalesce(max({col}), 0) FROM main.{t}").fetchone()[0]
            if t in seqs:
                db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = ?", (max(seqs[t], top), t))
            elif not top:
                db.execute("DELETE FROM sqlite_sequence WHERE name = ?", (t,))
        db.execute(f"DROP TABLE {log_table}")
        db.execute(f"DROP TABLE {meta_table}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    if meta.get("added_effects_column") == "1" and not left and \
            not db.execute("SELECT count(*) FROM main.spelleffect WHERE effects <> ''").fetchone()[0]:
        db.execute("ALTER TABLE main.spelleffect DROP COLUMN effects")
    for t in ("spellarea", "spellgates", "foodeffect"):
        if meta.get(f"made_{t}") == "1" and not db.execute(f"SELECT count(*) FROM main.{t}").fetchone()[0]:
            db.execute(f"DROP TABLE main.{t}")
    # the use's columns it added, when no row holds anything in them again
    for col, _ in reversed(ITEMEFFECT_ADDED):
        if col in (meta.get("added_itemeffect_columns") or "").split(",") and not left and \
                not db.execute(f"SELECT count(*) FROM main.itemeffect WHERE {col} <> ?", (USE_DEFAULTS[col],)).fetchone()[0]:
            db.execute(f"ALTER TABLE main.itemeffect DROP COLUMN {col}")
    return back, left, leftover


def backup(path):
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-eqoaemu-spells"
    src, out = sqlite3.connect(path, timeout=30), sqlite3.connect(dst)
    try:
        src.backup(out)
    finally:
        out.close()
        src.close()
    return dst


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--sheets", help="a folder of the workbook's sheets as TSV (02-Spells.tsv, 03-Effects.tsv, ...)")
    ap.add_argument("--ods", help="the workbook itself (.ods)")
    ap.add_argument("--map", help="the SpellID map (sheet_id, spellid, how)")
    ap.add_argument("--iso", help="the Frontiers disc: a made spelleffect row takes the workbook's visual if the disc has it")
    ap.add_argument("--bodies", help="cmd/modelbake's models.txt (the disc's CSprites): a use's body the disc lacks "
                                     "listed and not written")
    ap.add_argument("--link-existing", action="store_true", help="give spells we have the classes the workbook adds")
    ap.add_argument("--were-quests", default=WERE_QUESTS,
                    help="the were quests' file, whose reward spells are no class's (default tools/quests/data/weres.json)")
    ap.add_argument("--split-map", metavar="OUT", help="write the map with each row that shares a record with "
                    "the record's own split off (how 'split', its own id, split_from the record) and stop")
    ap.add_argument("--save-plan", metavar="FILE")
    ap.add_argument("--plan", metavar="FILE")
    ap.add_argument("--list", action="store_true", help="every listed spell, not five a list")
    ap.add_argument("--no-backup", action="store_true")
    ap.add_argument("--force", action="store_true", help="--undo even when characters hold made spells or scrolls")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    if (a.apply or a.undo) and not a.no_backup:
        print(f"copy: {backup(a.db)}")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        back, left, leftover = undo(db, a.force)
        print(f"put back {back}; left {left} changed since" + (f": {', '.join(leftover[:20])}" if leftover else ""))
        return
    if a.split_map:
        if not (a.sheets or a.ods) or not a.map:
            sys.exit("--split-map needs --sheets or --ods, and --map")
        done = split_map(db, Workbook(read_workbook(a.sheets, a.ods)), a.map, a.split_map)
        for hx, name, classes, frm, to in done:
            print(f"  {hx} {name} ({classes}): {frm} -> {to}")
        print(f"split {len(done)}; map: {a.split_map}")
        return
    if a.plan:
        with open(a.plan, encoding="utf-8") as f:
            p = json.load(f)
    else:
        if not (a.sheets or a.ods) or not a.map:
            sys.exit("--sheets or --ods, and --map (or --plan)")
        disc = icons = None
        if a.iso:
            import fill_spell_visuals_eqoaemu as fsv
            disc, icons = fsv.disc_visuals(a.iso), fsv.disc_icons(a.iso)
        p = plan(db, Workbook(read_workbook(a.sheets, a.ods)), read_map(a.map), disc, a.link_existing,
                 were_rewards(a.were_quests), icons, read_bodies(a.bodies) if a.bodies else None)
        p["were_quest_spells"] = were_quest_check(p, read_map(a.map), Workbook(read_workbook(a.sheets, a.ods)),
                                                  were_rewards(a.were_quests))
    report(p, a.list)
    if a.save_plan:
        p["made"] = datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")
        with open(a.save_plan, "w", encoding="utf-8") as f:
            json.dump(p, f, indent=1, ensure_ascii=False)
        print(f"plan: {a.save_plan}")
        return
    if a.apply:
        done, stale = apply(db, p)
        print(f"written {done}; {stale} changed since the plan, left")
    else:
        print("(report only: --apply to write it)")


if __name__ == "__main__":
    main()
