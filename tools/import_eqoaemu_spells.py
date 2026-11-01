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

What it writes, and from what:

  spellpattern, a spell the map says is ours: abilitylvl (Level), range,
      casttime, recast, power, scope, where the workbook has a value, and
      (the user, 2026-11-01: "workbook wins on all") its icon, frame and
      spelling as a new spell's below; a rename only where every lookup by
      name reads the two alike (rename_safe), else listed; spelldesc and
      eqprequire stay ours (the workbook has no description). Several workbook rows
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
      Defensive Proc); the numbers the workbook's, untouched. Where the shape
      has no word for what a row says, the effect carries a key of its own
      (see EXTRA_KEYS). Two readings the shape leaves open, made here:
        - a Dmg/Heal row with a positive HP1 and a duration is a heal each
          tick ("hp_tick"): the workbook's legend has a heal tick as a HoT
          does, twice at its landing and every 6 seconds after, and
          Sandstorm's Minor Healing ticked its line's figure
          (casting-sandstorm-20261021.md). A negative HP1 with a duration
          stays a hit ("hp", its duration kept): the workbook's own
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
      of its name, gets none: listed.
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
when they are empty again; refused while a character holds a made spell
(a book a split moved aside) or scroll, unless --force. Both take a copy of the database first (SQLite's backup API). A
cast reads its spell at the cast, a scroll at its use and a catalog at the
window, so no restart is needed; the class templates and pet taps read
classspells at the start.

    tools/import_eqoaemu_spells.py --sheets <dir of TSVs> | --ods <workbook.ods>
        --map <spellmap.tsv> [--iso <disc.iso>] [--db eqoa.db] [--save-plan FILE]
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
LOG_DDL = f"""
CREATE TABLE IF NOT EXISTS {META} (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
-- Every row written, in order: --undo takes them back last first.
CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,     -- one of KEYS' tables
    key     TEXT NOT NULL,     -- the row's key columns (JSON)
    action  TEXT NOT NULL,     -- insert | update | delete
    before  TEXT,              -- the columns written, as they were (JSON); an insert: none
    after   TEXT,              -- the columns written, as written (JSON); a delete: none
    note    TEXT NOT NULL DEFAULT ''
);
"""
# each table written, by the columns that key a row
KEYS = {"spellpattern": ["spellid"], "spelleffect": ["spellid"], "spellarea": ["spellid"],
        "classspells": ["class", "spellid"], "itempattern": ["patternid"], "npcitem": ["itemid"],
        "spells": ["id"], "charbuffs": ["serverid", "seq"], "cmcatalog": ["id"], "queststeps": ["questid", "step"],
        "spellgates": ["spell"], "foodeffect": ["patternid"], "itemeffect": ["patternid"]}
# a food, drink or potion made from the Created Item sheets: 910000 + the workbook's ItemID
ITEM_BASE = 910000
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

class Workbook:
    """The sheets, indexed."""

    def __init__(self, sheets):
        self.spells = sheets["Spells"]
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


def line_items(old, key):
    """The items a line names in clauses of that kind ("item" made, "consume" taken)."""
    out = []
    for c in old.split(","):
        c = LINE_TARGET.sub("", re.sub(r"\s*\([^)]*\)", "", c).strip()).strip()
        m = LINE_ITEM_WORDS[key].match(c)
        if m and not LINE_CREATURE.search(c):
            out.append(m.group(1).strip())
    return out


def settle_item_names(j, old, items_by_name, item_names):
    """Each made ("item") or taken ("consume") item under the name the server
    finds it by (handlers' summonItem and the reagent check read an item by
    its name; the effects are all the server reads, the brief's contract):
    one that is an item of ours by the workbook's name (our_item: its
    "Acidic Mix Potion" is our "Acidic Potion", its "Sparkling Cider effect"
    the food made for it) takes our item's own name; one the workbook names
    none of ours of, or names not at all (Neriak Cocktail's "Neriak Fire
    Potion", Feed's Unit of Blood, the vampires' reagents), takes the one item
    of ours of that kind a spell of ours names in its line ("SELF Summon
    Grobb Cocktail", "Reagent Unit of Blood x2"). The workbook's name stays as
    sheet_name, its count as it is. Returns (renamed, by_line): what it did,
    for the report; an item left none of ours makes nothing or takes
    nothing."""
    renamed, by_line = [], []

    def walk(objs):
        for o in objs:
            yield o
            yield from walk(o.get("procs") or [])

    for key in LINE_ITEM_WORDS:
        unresolved = []
        for o in walk(j["effects"]):
            it = o.get(key)
            if it is None:
                continue
            nm = (it.get("name") or "").strip()
            pid = it.get("patternid")
            if pid is None and nm:
                pid = our_item(items_by_name, nm)
            if pid in item_names:
                if it.get("name") != item_names[pid]:
                    it["sheet_name"], it["name"], it["patternid"] = nm, item_names[pid], pid
                    renamed.append(f"{nm!r} -> {item_names[pid]!r}")
            else:
                unresolved.append(it)
        ours = [n for n in line_items(old, key) if our_item(items_by_name, n) in item_names]
        if len(unresolved) != 1 or len(ours) != 1:
            continue
        it, pid = unresolved[0], our_item(items_by_name, ours[0])
        nm = (it.get("name") or "").strip()
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
        elif t == "Dmg/Heal" and hp1 > 0 and dur > 0:
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
    if num(s, "Duration"):
        out["duration_s"] = num(s, "Duration")
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
    return out, missing


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


def plan(db, wb, smap, disc=None, link_existing=False, weres=None, icons=None):
    """Everything --apply would write, and the report's lists. weres: the
    were quests' reward spells (were_rewards); icons: the disc's (disc_icons),
    which IconID's byte order is read by (icon_of)."""
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
    for pid, name in db.execute("SELECT patternid, itemname FROM main.itempattern ORDER BY patternid"):
        items_by_name.setdefault(fold(name), pid)
        item_names[pid] = name
    lists = collections.defaultdict(list)
    counts = collections.Counter()
    foods = plan_foods(db, wb, items_by_name, item_names, lists, counts)

    by_sid = collections.defaultdict(list)
    split_of = {}   # a split row's new spellid -> the record it leaves
    for s in wb.spells:
        sid_hex = hexid(s.get("SpellID"))
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
    new_rows = {}   # spellid -> (sheet row, json, a were's) of a new spell
    for sid, rows in sorted(by_sid.items()):
        rec = records.get(sid)
        s = rows[0] if len(rows) == 1 or rec is None else pick_owner(rows, rec, linked[sid])
        for o in rows:
            if o is not s:
                lists["several rows on one record: not applied (the map's to settle)"].append(
                    f"{hexid(o.get('SpellID'))} {cell(o, 'Name')} {cell(o, 'CLASS')} L{cell(o, 'Level')} -> {sid} "
                    f"(set from {hexid(s.get('SpellID'))} {cell(s, 'CLASS')} L{cell(s, 'Level')})")
        j, missing = spell_json(s, wb, items_by_name)
        for m in missing:
            lists["an effect the Effects sheet lacks"].append(f"{hexid(s.get('SpellID'))} {cell(s, 'Name')}: {m}")
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
            old_line = ((erows.get(sid) or {}).get("line") or "") if rec is not None else ""
            renamed, by_line = settle_item_names(j, old_line, items_by_name, item_names)
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
    moves, migrations = plan_splits(db, wb, smap, splits, records, links, lists, counts)
    gate_rows = plan_gates(db, gates, lists, counts)
    counts["class rows"] = len(links)

    scrolls, stock = plan_scrolls(db, new_rows, records, lists, counts, links, moves, migrations)
    p = dict(spells=spells, effects=effects, areas=area_rows, links=links, scrolls=scrolls, stock=stock,
             moves=moves, migrations=migrations, gates=gate_rows, foods=foods,
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
                    figures["hot" if (num(e, "Duration") or 0) > 0 else "heal"] += v
                else:
                    ok = False
        name = re.sub(r"(?i)\s+effect$", "", cell(rs[0], "Name")).strip()
        if not ok or not figures or POTION_NAME.search(name):
            continue  # not a food or a drink (an alchemist's potion is not this part's)
        potion = figures["heal"] > 0
        foods.append(dict(iid=iid, name=name, level=num(rs[0], "Level") or 1, figures=figures, potion=potion,
                          kind="potion" if potion else "meal" if figures["hot"] else "drink",
                          dur=max(num(e, "Duration") or 0 for e in effs),
                          cast=any(cell(e, "DisspellOnCast") == "1" for e in effs),
                          contact=any(cell(e, "DisspellOnContact") == "1" for e in effs)))
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
    taken out or changed ("kithacor circle", "kithicor circle")."""
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
            # letter otherwise and with no place of its own (Kithacor Circle,
            # Kithicor Circle's level 60 twin for every race): its twin's place
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
    return out


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


def log(db, tbl, key, action, before, after, note=""):
    db.execute(f"INSERT INTO {LOG} (tbl, key, action, before, after, note) VALUES (?, ?, ?, ?, ?, ?)",
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


def apply(db, p):
    """Every planned change whose row is still as planned, in one
    transaction, each logged; returns (done, changed since)."""
    db.executescript(LOG_DDL)
    if db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0]:
        sys.exit("this database already holds an EQOAEmu spell import; --undo it first")
    changes = p.get("changes") or changes_of(p)
    done = stale = 0
    db.execute("BEGIN IMMEDIATE")
    try:
        seqs = dict(db.execute("SELECT name, seq FROM sqlite_sequence WHERE name IN "
                               "('spellpattern', 'itempattern', 'npcitem')").fetchall())
        added = "effects" not in columns(db, "spelleffect")
        if added:
            db.execute("ALTER TABLE main.spelleffect ADD COLUMN effects TEXT NOT NULL DEFAULT ''")
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
                log(db, tbl, {"itemid": cur.lastrowid}, "insert", None, row, c.get("note"))
            elif act == "insert":
                if row_now(db, tbl, c["key"], KEYS[tbl]) is not None:
                    stale += 1
                    continue
                insert(db, tbl, c["row"])
                log(db, tbl, c["key"], "insert", None, c["row"], c.get("note"))
                if tbl == "itempattern":
                    scrolls.add(c["key"]["patternid"])
            elif act == "update":
                if row_now(db, tbl, c["key"], list(c["before"])) != c["before"]:
                    stale += 1
                    continue
                w, args = where_of(tbl, c["key"])
                db.execute(f"UPDATE main.{tbl} SET {', '.join(f'{k} = ?' for k in c['set'])} WHERE {w}",
                           [*c["set"].values(), *args])
                log(db, tbl, c["key"], "update", c["before"], c["set"], c.get("note"))
            elif act == "delete":
                if row_now(db, tbl, c["key"], list(c["before"])) != c["before"]:
                    stale += 1
                    continue
                w, args = where_of(tbl, c["key"])
                # its rowid too, so --undo puts it back where it was (a dump's order)
                rid = db.execute(f"SELECT rowid FROM main.{tbl} WHERE {w}", args).fetchone()[0]
                db.execute(f"DELETE FROM main.{tbl} WHERE {w}", args)
                log(db, tbl, c["key"], "delete", dict(c["before"], rowid=rid), None, c.get("note"))
            done += 1
        now = datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")
        for k, v in {"applied_at": now, "sqlite_sequence": dumps(seqs), "added_effects_column": str(int(added)),
                     "made_spellarea": str(int(made["spellarea"])), "made_spellgates": str(int(made["spellgates"])),
                     "made_foodeffect": str(int(made["foodeffect"])),
                     "counts": dumps(p["counts"])}.items():
            db.execute(f"INSERT OR REPLACE INTO {META} VALUES (?, ?)", (k, v))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return done, stale


def undo(db, force=False):
    """Every logged row still as written put back, last first; returns
    (back, left, what was left)."""
    if not has_table(db, LOG):
        sys.exit("no EQOAEmu spell import to undo here")
    entries = [(t, json.loads(k), a, None if b is None else json.loads(b), None if f is None else json.loads(f))
               for t, k, a, b, f in db.execute(f"SELECT tbl, key, action, before, after FROM {LOG} ORDER BY seq DESC")]
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
        meta = dict(db.execute(f"SELECT key, value FROM {META}").fetchall())
        seqs = json.loads(meta.get("sqlite_sequence", "{}"))
        for t, col in (("spellpattern", "spellid"), ("itempattern", "patternid"), ("npcitem", "itemid")):
            top = db.execute(f"SELECT coalesce(max({col}), 0) FROM main.{t}").fetchone()[0]
            if t in seqs:
                db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = ?", (max(seqs[t], top), t))
            elif not top:
                db.execute("DELETE FROM sqlite_sequence WHERE name = ?", (t,))
        db.execute(f"DROP TABLE {LOG}")
        db.execute(f"DROP TABLE {META}")
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
                 were_rewards(a.were_quests), icons)
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
