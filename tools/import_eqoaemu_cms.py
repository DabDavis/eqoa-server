#!/usr/bin/env python3
"""Class masteries from EQOAEmu's spell workbook (the user, 2026-11-01: "lean
on them", "Workbook wins, re-fit", "add what we lack: the spells, the
masteries ...", and for what a GM set: "change gm set also").

EQOAEmu's EQOA_Spells_and_CM workbook (2020; the operator's copy, an .ods, or
its sheets as TSV: neither is in this repository) lists every mastery rank in
its CMs sheet: an id (hex), the rank's name and text, its Classification and
Sub-Category, the races and classes it is for, level, points, the points spent
it needs, the ranks it needs (CM Req 1-4, by id), the abilities (Ability Req
1-6), the base stats, the abilities it adds and takes (+Spell / -Spell, by the
Spells sheet's id) and the 29 stats it gives. This tool puts them into
cmcatalog (entity/cmbuy.go), the workbook winning wherever it has a value:

- **A rank we have keeps OUR id** (charcms, the characters' bought ranks, keys
  it) and takes the workbook's name, text, cost, level, spent, minstats,
  prerequisites, abilities, stats, classes, races and category. Ours is found
  by its name (apostrophes, a plural s and case aside: "Soldier's Grace 1" is
  our "Soldier Grace 1"), by its line and rank ("Deputy" is our "Deputy" rank
  1), by the ability it adds (a master class's: "Protect the Vale" is our
  "Deputy 2"), by the same stats at the same level and cost ("Avoid Danger 1"
  is our "AGI 1"), or by a close spelling with the same rank ("Bulging
  Muscles" our "Buldging Muscles"). One workbook rank may be several of ours
  (our master classes shared by an archetype are a row a class: each keeps its
  class), several workbook ranks one of ours (the two Focus lines, one for the
  casters and melee, one for the tanks and priests, are our one line for all).
- **The workbook's own slips are put right** (`fix_slips`): an ability a rank
  needs, adds or takes away named by the id of another class's spell, where
  the Spells sheet has exactly one spell of that name for the rank's class,
  is that spell (the warrior's Tranquility named the enchanter's), each said.
- The Transmuter's second and third ranks take each other's descriptions
  (DESC_SWAPS: the workbook's guesses are each other's; the user, 2026-11-01:
  "swap them").
- A cell left empty (or "NO DESCRIPTION FOUND") is no value: ours stays. A
  group of cells is the workbook's whole when it fills any of it (the 29
  stats, the base stats, the abilities added, ...).
- **A rank we lack is NEW**, id 900000 + its hex id (the import's
  convention: the spells' too). A were or were-hunter rank (Lycanthropy, the
  Were and Were Hunter sub-categories, the Were-Hunter master classes) is
  offered behind the quest that makes a character its kind (the user,
  2026-11-01: "lets add weres and werehunters back"; WERE_KINDS: a kind's p3,
  which gives its form; a werebeast's any of the five, "a|b|c", which the
  server reads as a group any one quest meets; a were-hunter's the brand),
  found through quest_import_quests; --weres off writes them disabled. A new
  rank the workbook marks "QUEST ID" takes the quest that rewards an ability
  it adds, or is named as it or one, or whose notes name it as opened for a
  mastery point (the tank exemplars'), else is written disabled. A quest not
  in the game (a draft) bars no one (cmBlockLocked): such gates are listed,
  as the drafts go live with the spells they lack.
- Our ranks the workbook lacks stay as they are, but one clearly the same as
  a NEW workbook rank (its name, or its stats at its level, in its group) is
  disabled (never deleted), unless a character owns it.
- charcms is not touched: what a character paid stays its paid cost; a
  bought rank whose cost changes is counted.
- Rows a GM changed (gm_audit, tbl cmcatalog, through the panel or an agent:
  not a tool's line) are overwritten too, and listed.
- A rank's prerequisites the sheet names by a master class other than its
  own are the sheet's slip and left out (Death Knight's abilities name
  Slayer: one master class at a time would bar them for good).
- The workbook's racial master classes (Deputy, Juggernaut, ...: our seed put
  them under Racial, 1) are Master Class, 4, as CLIENT's window and the one
  master class rule (OneMasterClass) read them: --racial-mc keep leaves 1.

The ability ids go through the import's spell map (--spellmap: the
workbook's SpellID to ours; a spell we lack is 900000 + its hex id, made by
the spells import), so run that first: --apply refuses a plan whose added or
needed spells are not all in spellpattern, unless --allow-missing-spells.

Each change is a row edit as the GM panel writes one: the row before and
after in gm_audit (as dab, via "tool"), so the panel lists it with the rank and
reverts it alone; all are logged in cmcatalog_eqoaemu_log for --undo, which
puts back every row still as it wrote it (one changed since is left and said;
a new rank a character bought since is left too). The world reads the catalog
at its start and when a GM edits a row: restart after --apply (or touch a row
in the panel).

    tools/import_eqoaemu_cms.py --tsv-dir <sheets> --spellmap spellmap.tsv --db <copy> --save-plan plan.json
    tools/import_eqoaemu_cms.py --sheet <workbook.ods> --spellmap spellmap.tsv --db <copy> [--list]
    tools/import_eqoaemu_cms.py --db eqoa.db --plan plan.json --apply      # each change only if the row is still as planned
    tools/import_eqoaemu_cms.py --db eqoa.db --undo
"""
import argparse
import collections
import csv
import datetime
import difflib
import json
import os
import re
import sqlite3
import sys
import xml.etree.ElementTree as ET
import zipfile

HERE = os.path.dirname(os.path.abspath(__file__))
DEFAULT_DB = os.path.join(HERE, "..", "eqoa.db")
SOURCE = "eqoaemu workbook"
NEW_BASE = 900000
LOG = "cmcatalog_eqoaemu_log"
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    id      INTEGER PRIMARY KEY,   -- the cmcatalog row
    op      TEXT NOT NULL,         -- update, insert or disable
    before  TEXT,                  -- the columns it changed, as they were (JSON; NULL: the row was made here)
    after   TEXT NOT NULL,         -- the same columns as written
    audit   INTEGER NOT NULL,      -- the gm_audit line written
    applied TEXT NOT NULL
)"""
# the characters' rows the import changed (their books, a moved purchase):
# each row as it was and as written, for --undo
CHARS_LOG = "cmcatalog_eqoaemu_chars_log"
CHARS_LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {CHARS_LOG} (
    n       INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,         -- spells or charcms
    key     TEXT NOT NULL,         -- the row's key as the GM panel writes it
    before  TEXT,                  -- the row as it was (JSON; NULL: made here)
    after   TEXT,                  -- the row as written (NULL: taken out here)
    audit   INTEGER NOT NULL,
    applied TEXT NOT NULL
)"""
# database.GMAuditSchema, as the server makes it
AUDIT_SCHEMA = """CREATE TABLE IF NOT EXISTS gm_audit (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	at TEXT NOT NULL, accountid INTEGER NOT NULL, username TEXT NOT NULL,
	via TEXT NOT NULL, ip TEXT NOT NULL DEFAULT '', action TEXT NOT NULL,
	tbl TEXT NOT NULL DEFAULT '', rowkey TEXT NOT NULL DEFAULT '',
	before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER
)"""
WHO, VIA = "dab", "tool"
DETAIL = "EQOAEmu's spell workbook (2020), its CMs: tools/import_eqoaemu_cms.py"

CLASS_NAMES = "WAR RAN PAL SK MNK BRD RGE DRD SHA CL MAG NEC ENC WIZ ALC".split()
RACE_BITS = {"HUM": 0, "ELF": 1, "DELF": 2, "GNO": 3, "GNM": 3, "DWF": 4, "TRL": 5, "BAR": 6, "HLF": 7, "ERU": 8,
             "OGR": 9}
ALL_CLASSES = (1 << 15) - 1
ALL_RACES = (1 << 11) - 1  # the froglok (10) too
TEN_RACES = (1 << 10) - 1  # the workbook's "every race": the froglok is ours, and keeps its bit
CATEGORIES = {"general": 0, "racial": 1, "archetype": 2, "class": 3, "master class": 4}
CATEGORY_NAMES = ("general", "racial", "archetype", "class", "master class")
MASTER = 4
NO_TEXT = {"", "no description found"}

# The window's 29 stats (entity/cmbuy.go's slots) as cmcatalog writes them, and
# the server's own reading of a key (cmStatSlots: lower case, no spaces).
SLOT_KEYS = ["STR", "STA", "AGI", "DEX", "WIS", "INT", "CHA",
             "STRMax", "STAMax", "AGIMax", "DEXMax", "WISMax", "INTMax", "CHAMax",
             "HP", "Pow", "HoT", "PoT", "AC", "Off Mod", "Def Mod", "HP Factor",
             "FR", "LR", "CR", "AR", "PR", "DR", "MovRate"]
SERVER_KEYS = {"str": 0, "sta": 1, "agi": 2, "dex": 3, "wis": 4, "int": 5, "cha": 6,
               "strmax": 7, "stamax": 8, "agimax": 9, "dexmax": 10, "wismax": 11, "intmax": 12, "chamax": 13,
               "hp": 14, "maxhp": 14, "hpmax": 14, "pow": 15, "pwrmax": 15, "power": 15, "maxpower": 15,
               "hot": 16, "pot": 17, "ac": 18, "offmod": 19, "defmod": 20, "hpfactor": 21,
               "fr": 22, "lr": 23, "cr": 24, "ar": 25, "pr": 26, "dr": 27, "movrate": 28, "moverate": 28}
# the workbook's stat columns: the server's slot of each ("Speed" is the
# movement rate, MovRate: Northman's Gait's 5 is ours too)
SHEET_STATS = {"AC": 18, "STR": 0, "STA": 1, "AGI": 2, "DEX": 3, "WIS": 4, "INT": 5, "CHA": 6, "HP Max": 14,
               "PWR Max": 15, "HoT": 16, "PoT": 17, "AR": 25, "CR": 24, "LR": 23, "FR": 22, "PR": 26, "DR": 27,
               "STR Max": 7, "STA Max": 8, "AGI Max": 9, "DEX Max": 10, "WIS Max": 11, "INT Max": 12,
               "CHA Max": 13, "HP Factor": 21, "OFF Mod": 19, "DEF Mod": 20, "Speed": 28}
BASE_STATS = {"Base STR Req": 0, "Base STA Req": 1, "Base AGI Req": 2, "Base DEX Req": 3, "Base WIS Req": 4,
              "Base INT Req": 5, "Base CHA Req": 6}

COLS = ("id", "category", "classes", "races", "mastery", "rank", "name", "parent", "cost", "level", "spent",
        "minstats", "requires", "require_spells", "require_ranks", "require_quests", "effects", "adds",
        "add_spells", "removes", "remove_spells", "description", "enabled", "source")


# ---- reading the workbook --------------------------------------------------------------

_T = "urn:oasis:names:tc:opendocument:xmlns:table:1.0"
_X = "urn:oasis:names:tc:opendocument:xmlns:text:1.0"
_O = "urn:oasis:names:tc:opendocument:xmlns:office:1.0"


def ods_table(path, sheet):
    """The named sheet's rows, each a list of cells (a cell's lines joined by
    " / ", as the TSVs write them)."""
    z = zipfile.ZipFile(path)
    for _, el in ET.iterparse(z.open("content.xml"), events=("end",)):
        if el.tag != f"{{{_T}}}table":
            continue
        if el.get(f"{{{_T}}}name") != sheet:
            el.clear()
            continue
        rows = []
        for r in el.iter(f"{{{_T}}}table-row"):
            cells = []
            for c in r:
                if c.tag not in (f"{{{_T}}}table-cell", f"{{{_T}}}covered-table-cell"):
                    continue
                v = " / ".join("".join(p.itertext()) for p in c.iter(f"{{{_X}}}p")) or c.get(f"{{{_O}}}value") or ""
                cells.extend([v] * min(int(c.get(f"{{{_T}}}number-columns-repeated", "1")), 200))
            while cells and cells[-1] == "":
                cells.pop()
            if cells:
                rows.append(cells)
        return rows
    sys.exit(f"{path}: no sheet {sheet!r}")


def tsv_table(path):
    with open(path, newline="", encoding="utf-8") as f:
        return [r for r in csv.reader(f, delimiter="\t", quoting=csv.QUOTE_NONE) if any(c.strip() for c in r)]


class Sheet:
    """A sheet's rows by its header, a column named twice ("CM Req 1": the
    id, then the name) by its occurrence."""

    def __init__(self, table):
        self.head = [h.strip() for h in table[0]]
        self.rows = [r + [""] * (len(self.head) - len(r)) for r in table[1:]]
        self.at = collections.defaultdict(list)
        for i, h in enumerate(self.head):
            self.at[h].append(i)

    def has(self, name):
        return name in self.at

    def get(self, row, name, n=0):
        idx = self.at.get(name)
        if not idx or n >= len(idx):
            return ""
        return (row[idx[n]] or "").strip()


def hexid(s):
    s = (s or "").strip()
    if s[:2].lower() == "0x":
        s = s[2:]
    return int(s, 16) if re.fullmatch(r"[0-9a-fA-F]{1,6}", s) else None


def number(s):
    s = (s or "").strip()
    try:
        return int(float(s)) if s else None
    except ValueError:
        return None


def fold(s):
    """A name as the matches compare it: case, apostrophes, a possessive or
    plural s and punctuation aside ("Soldier's Grace" is "Soldier Grace")."""
    s = (s or "").lower().replace("’", "'").replace("`", "'")
    s = re.sub(r"'s\b", "", s).replace("'", "")
    words = re.sub(r"[^a-z0-9]+", " ", s).split()
    return " ".join(w[:-1] if len(w) > 3 and w.endswith("s") else w for w in words)


def split_rank(name):
    """("Nimbleness", 1) for "Nimbleness 1"; (name, None) with no number."""
    m = re.match(r"^(.*?)\s*(\d+)$", (name or "").strip())
    return (m.group(1).strip(), int(m.group(2))) if m else ((name or "").strip(), None)


def mask_of(text, bits, every, all_mask):
    m = 0
    unknown = []
    for w in (text or "").replace(",", " ").split():
        w = w.strip().upper()
        if w in bits:
            m |= 1 << bits[w]
        elif w:
            unknown.append(w)
    if m == every:
        m = all_mask
    return m, unknown


def read_ranks(sheet):
    """The CMs sheet as dicts."""
    need = ("CM ID", "Mastery", "Classification", "Race", "Class", "Level", "Points")
    missing = [n for n in need if not sheet.has(n)]
    if missing:
        sys.exit(f"the CMs sheet has no column {missing}")
    out = []
    for r in sheet.rows:
        wid = hexid(sheet.get(r, "CM ID"))
        if wid is None:
            continue
        name = sheet.get(r, "Mastery")
        classes, uc = mask_of(sheet.get(r, "Class"), {c: i for i, c in enumerate(CLASS_NAMES)}, ALL_CLASSES,
                              ALL_CLASSES)
        races, ur = mask_of(sheet.get(r, "Race"), RACE_BITS, TEN_RACES, ALL_RACES)
        cat = CATEGORIES.get(sheet.get(r, "Classification").lower())
        base, num = split_rank(name)

        def ids(prefix, suffix, n):
            got = []
            for i in range(1, n + 1):
                x = hexid(sheet.get(r, f"{prefix}{i}{suffix}", 0))
                if x is not None:
                    nm = sheet.get(r, f"{prefix}{i}", 1 if not suffix else 0) if not suffix else sheet.get(r, f"{prefix}{i}")
                    got.append((x, nm))
            return got

        reqs = []
        for i in range(1, 5):
            x = hexid(sheet.get(r, f"CM Req {i}", 0))
            if x is not None:
                reqs.append((x, sheet.get(r, f"CM Req {i}", 1)))
        stats = {}
        for col, slot in SHEET_STATS.items():
            v = number(sheet.get(r, col))
            if v:
                stats[slot] = stats.get(slot, 0) + v
        base_stats = {}
        for col, slot in BASE_STATS.items():
            v = number(sheet.get(r, col))
            if v:
                base_stats[slot] = v
        out.append(dict(
            wid=wid, hex=sheet.get(r, "CM ID").upper(), name=name, base=base, num=num,
            desc=sheet.get(r, "Description"), classification=sheet.get(r, "Classification"),
            sub=sheet.get(r, "Sub-Category"), cat=cat, classes=classes, races=races,
            unknown=uc + ur, arch=sheet.get(r, "Archetype"), lyc=sheet.get(r, "Lycanthropy"),
            quest=sheet.get(r, "Quest"), level=number(sheet.get(r, "Level")), cost=number(sheet.get(r, "Points")),
            spent=number(sheet.get(r, "CM Spent Req")), reqs=reqs,
            abil=[(x, sheet.get(r, f"Ability Req {i}")) for i in range(1, 7)
                  for x in [hexid(sheet.get(r, f"Ability Req {i} ID"))] if x is not None],
            adds=[(x, sheet.get(r, f"=+Spell {i}")) for i in range(1, 7)
                  for x in [hexid(sheet.get(r, f"=+Spell {i} ID"))] if x is not None],
            removes=[(x, sheet.get(r, f"-Spell {i}")) for i in range(1, 7)
                     for x in [hexid(sheet.get(r, f"-Spell {i} ID"))] if x is not None],
            stats=stats, base_stats=base_stats))
    return out


def read_spell_levels(sheet):
    """The Spells sheet's level of each SpellID (a new spell's, for the
    window's "Name@level")."""
    out = {}
    if sheet is None:
        return out
    for r in sheet.rows:
        x, lvl = hexid(sheet.get(r, "SpellID")), number(sheet.get(r, "Level"))
        if x is not None and lvl:
            out.setdefault(x, lvl)
    return out


def slip_name(s):
    """A spell's name as fix_slips compares it: case and apostrophes aside, a
    "+" kept ("Serenity +" is another spell than "Serenity")."""
    s = (s or "").lower().replace("’", "'").replace("`", "'").replace("'", "")
    return " ".join(re.sub(r"[^a-z0-9+]+", " ", s).split())


def read_spell_owners(sheet):
    """The Spells sheet's name and classes (a mask, 0 for none) of each
    SpellID, for fix_slips."""
    out = {}
    if sheet is None or not sheet.has("SpellID"):
        return out
    bits = {c: i for i, c in enumerate(CLASS_NAMES)}
    for r in sheet.rows:
        x = hexid(sheet.get(r, "SpellID"))
        if x is not None:
            out.setdefault(x, (slip_name(sheet.get(r, "Name")), mask_of(sheet.get(r, "CLASS"), bits, ALL_CLASSES,
                                                                        ALL_CLASSES)[0]))
    return out


def fix_slips(W, owners):
    """The workbook's own slips put right: an ability a rank requires, adds or
    takes away, named by the id of a spell of none of the rank's classes, where
    the Spells sheet has exactly one spell of that name of the rank's classes,
    is that spell (2026-11-01's workbook: the warrior's Tranquility names the
    enchanter's Tranquility, 0276, and no mastery names the warrior's own,
    0635; the shadow knight's Parasitic Bond 1 requires the necromancer's
    Siphon Strength, which no shadow knight holds, so it could never be
    bought; eight such ids in six ranks). A rank of every class, or a spell of
    no class, is never one. Returns each id put right: (rank hex, kind, the
    sheet's id, the one taken, the name)."""
    byname = collections.defaultdict(list)
    for x, (nm, cls) in owners.items():
        if nm and cls:
            byname[nm].append((x, cls))
    fixed = []
    for w in W:
        for kind in ("abil", "adds", "removes"):
            out = []
            for x, nm in w[kind]:
                own = owners.get(x)
                if own and own[1] and not own[1] & w["classes"]:
                    alt = sorted(y for y, cls in byname.get(own[0], []) if y != x and cls & w["classes"])
                    if len(alt) == 1:
                        fixed.append((w["hex"], kind, x, alt[0], nm))
                        x = alt[0]
                out.append((x, nm))
            w[kind] = out
    return fixed


# Ranks whose workbook descriptions are each other's (the user, 2026-11-01:
# "swap them"): the Transmuter's second and third ranks, whose descriptions
# guess "Rat?" for the one whose potion is an ogre's body (the workbook's
# own Created Item sheets: the Creative Mix Potion) and "Ogre Pill?" for the
# one whose thrown potion turns its target into an albino rat (the
# Transmutative Mix Potion). By the ranks' names, folded.
DESC_SWAPS = [("creative transmutation", "transmutative mixture")]


def swap_descriptions(W):
    """DESC_SWAPS' ranks given each other's workbook descriptions (both of a
    pair found, in one mastery). Returns the names swapped."""
    done = []
    for a, b in DESC_SWAPS:
        wa = [w for w in W if fold(w["name"]) == a]
        wb = [w for w in W if fold(w["name"]) == b]
        if len(wa) != 1 or len(wb) != 1 or wa[0].get("sub") != wb[0].get("sub"):
            continue
        wa[0]["desc"], wb[0]["desc"] = wb[0]["desc"], wa[0]["desc"]
        done.append((wa[0]["name"], wb[0]["name"]))
    return done


def read_spellmap(path):
    """The import's spell map: the workbook's SpellID to our spellid."""
    out = {}
    with open(path, encoding="utf-8") as f:
        for line in f:
            if line.startswith("#"):
                continue
            p = line.rstrip("\n").split("\t")
            if len(p) < 2 or p[0] == "sheet_id":
                continue
            x, sid = hexid(p[0]), number(p[1])
            if x is not None and sid:
                out[x] = (sid, p[2] if len(p) > 2 else "")
    return out


# ---- our side ---------------------------------------------------------------------------

def slots_of(effects):
    """A cmcatalog stat list as the server reads it (ParseCMRank)."""
    out = [0] * 29
    for f in (effects or "").split(","):
        k, sep, v = f.partition(":")
        if not sep:
            continue
        try:
            n = int(v.strip())
        except ValueError:
            continue
        s = SERVER_KEYS.get(k.strip().lower().replace(" ", ""))
        if s is not None:
            out[s] += n
    return out


def stat_text(slots):
    return ",".join(f"{SLOT_KEYS[i]}:{v}" for i, v in enumerate(slots) if v)


def min_text(stats):
    return ",".join(f"{SLOT_KEYS[i]}:{stats[i]}" for i in range(7) if stats.get(i))


def ability_names(text):
    return [a.split("@")[0].strip() for a in (text or "").split(";") if a.strip()]


def id_list(text):
    out = []
    for f in (text or "").split(","):
        try:
            out.append(int(f.strip()))
        except ValueError:
            pass
    return out


def ours(db):
    cur = db.execute("SELECT * FROM cmcatalog ORDER BY id")
    names = [d[0] for d in cur.description]
    rows = []
    for x in cur.fetchall():
        o = dict(zip(names, x))
        o["fname"], o["fline"] = fold(o["name"]), fold(o["mastery"])
        o["slots"] = slots_of(o["effects"])
        o["add_ids"] = {i for i in id_list(o["add_spells"]) if i > 0}
        o["add_names"] = {fold(n) for n in ability_names(o["adds"])}
        rows.append(o)
    return rows


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


# ---- matching ---------------------------------------------------------------------------

def is_were(w):
    return bool(w["lyc"]) or "were" in (w["sub"] or "").lower()


def cat_ok(w, o):
    """Our category for the workbook's, or our racial master class (1) for its
    race's Master Class (4)."""
    if w["cat"] == o["category"]:
        return True
    return w["cat"] == MASTER and o["category"] == 1 and w["races"] != ALL_RACES


def compat(w, o):
    return bool(w["classes"] & o["classes"]) and bool(w["races"] & o["races"]) and cat_ok(w, o)


def wb_slots(w):
    s = [0] * 29
    for k, v in w["stats"].items():
        s[k] += v
    return s


def ability_fold(name):
    """An ability's name as the matches compare it: a level or note in
    brackets, a trailing * or + aside ("Howling Death (44)" is our "Howling
    Death@44", "Ghoulish Death*" the workbook's "Ghoulish Death")."""
    name = re.sub(r"\([^)]*\)", " ", (name or "").split("@")[0])
    return fold(name.replace("*", " ").replace("+", " "))


def abilities_of(o, kind):
    """Our row's abilities of a kind (adds, requires, removes): folded names
    and spell ids."""
    col = {"adds": "add_spells", "requires": "require_spells", "removes": "remove_spells"}[kind]
    return ({ability_fold(n) for n in ability_names(o[kind])} - {""},
            {i for i in id_list(o[col]) if i > 0})


def match(W, O, spellmap):
    """Each of our rows to the workbook rank it is (at most one), each
    workbook rank to the rows of ours it is (none, one, or several of disjoint
    classes or races): ({our id: (workbook id, how)}, {workbook id: [our rows]})."""
    for w in W:
        w["fname"], w["fbase"], w["fsub"] = fold(w["name"]), fold(w["base"]), fold(w["sub"])
        w["slots"] = wb_slots(w)
        for kind, key in (("adds", "adds"), ("requires", "abil"), ("removes", "removes")):
            w[kind + "_names"] = {ability_fold(n) for _, n in w[key]} - {""}
            w[kind + "_ids"] = {spellmap[x][0] for x, _ in w[key] if x in spellmap}
    for o in O:
        # a master class's ability as the seed named some: "Quick Hands Burglar"
        o["fshort"] = o["fname"]
        if o["category"] in (1, MASTER) and o["fline"] and o["fname"].endswith(" " + o["fline"]):
            o["fshort"] = o["fname"][:-len(o["fline"]) - 1]
        for kind in ("adds", "requires", "removes"):
            o[kind + "_names"], o[kind + "_ids"] = abilities_of(o, kind)
    taken = {}  # our id -> (wid, how)
    cover = collections.defaultdict(list)  # wid -> our rows

    def free(w, o):
        # disjoint from the rows of ours this rank already is
        return all(not (p["classes"] & o["classes"] and p["races"] & o["races"]) for p in cover[w["wid"]])

    def take(w, o, how):
        taken[o["id"]] = (w["wid"], how)
        cover[w["wid"]].append(o)

    def tier(how, ok, only_unmatched=False):
        for w in W:
            if only_unmatched and cover[w["wid"]]:
                continue
            for o in O:
                if o["id"] not in taken and compat(w, o) and free(w, o) and ok(w, o):
                    take(w, o, how)

    def scored(how, score):
        """Pairs by the best score first, each side once (a workbook rank may
        still take several of ours of other classes)."""
        pairs = []
        for w in W:
            if cover[w["wid"]]:
                continue
            for o in O:
                if o["id"] not in taken and compat(w, o):
                    s = score(w, o)
                    if s > 0:
                        pairs.append((-s, w["wid"], o["id"], w, o))
        pairs.sort(key=lambda x: x[:3])
        done_w = set()
        for _, wid, oid, w, o in pairs:
            if oid in taken or (wid in done_w and not free(w, o)):
                continue
            if wid in done_w and any(p["classes"] == o["classes"] and p["races"] == o["races"] for p in cover[wid]):
                continue
            take(w, o, how)
            done_w.add(wid)

    def group(w, o):
        """Whether both are of one master class (its name), or one line."""
        if w["cat"] == MASTER:
            return w["fsub"] == o["fline"]
        return True

    tier("name", lambda w, o: w["fname"] in (o["fname"], o["fshort"]))
    tier("line and rank", lambda w, o: w["fbase"] == o["fline"] and (w["num"] or 1) == o["rank"])

    def by_ability(w, o):
        if not group(w, o):
            return 0
        s = 0
        for kind, weight in (("adds", 3), ("requires", 2), ("removes", 1)):
            s += weight * (len(w[kind + "_names"] & o[kind + "_names"]) + len(w[kind + "_ids"] & o[kind + "_ids"]))
        if not s:
            return 0
        return s + 2 * (w["cost"] == o["cost"]) + (w["level"] == o["level"]) + ((w["num"] or 1) == o["rank"])

    scored("the abilities it adds or needs", by_ability)
    tier("its stats, level and cost", lambda w, o: any(w["slots"]) and w["slots"] == o["slots"]
         and w["level"] == o["level"] and w["cost"] == o["cost"] and (w["num"] or 1) == o["rank"]
         and w["cat"] == o["category"] and w["classes"] == o["classes"] and w["races"] == o["races"],
         only_unmatched=True)
    tier("its stats, in its master class", lambda w, o: w["cat"] == MASTER and w["reqs"] and o["parent"]
         and group(w, o) and any(w["slots"]) and w["slots"] == o["slots"], only_unmatched=True)

    def close(w, o):
        return (w["num"] or 1) == o["rank"] and difflib.SequenceMatcher(None, w["fname"], o["fname"]).ratio() >= 0.85

    tier("a close spelling", close, only_unmatched=True)
    # a master class's last ability left on both sides (Blade Dance, our
    # Bravo's Dance; Sacrifice, our Doomseeker 2's "Sacrafice")
    for o in O:
        if o["id"] in taken or o["category"] not in (MASTER, 1) or not o["parent"]:
            continue
        cands = [w for w in W if not cover[w["wid"]] and w["cat"] == MASTER and w["reqs"] and group(w, o)
                 and compat(w, o)]
        mine = [p for p in O if p["id"] not in taken and p["fline"] == o["fline"] and p["parent"]
                and p["classes"] & o["classes"] and p["races"] & o["races"]]
        if len(cands) == 1 and len(mine) == 1:
            take(cands[0], o, "the last ability of its master class")
    # the rank after the same rank, one left on each side (Greater Geomancy,
    # our Geomancy 6: both after Geomancy 5)
    for o in O:
        if o["id"] in taken or not o["parent"]:
            continue
        after = [w for w in W if not cover[w["wid"]] and compat(w, o) and w["cat"] == o["category"]
                 and any(o["parent"] in [p["id"] for p in cover.get(x, [])] for x, _ in w["reqs"])]
        mine = [p for p in O if p["id"] not in taken and p["parent"] == o["parent"] and compat(after[0], p)] if after else []
        if len(after) == 1 and len(mine) == 1:
            take(after[0], o, "the rank after the same rank")
    # the same rank for our other classes (a master class an archetype shares,
    # our row a class): its other rows of ours by the name it now matched
    for w in W:
        if not cover[w["wid"]]:
            continue
        for o in O:
            if o["id"] in taken or not compat(w, o) or not free(w, o):
                continue
            if any(o["fname"] == p["fname"] and o["fline"] == p["fline"] for p in cover[w["wid"]]):
                take(w, o, cover[w["wid"]][0] and taken[cover[w["wid"]][0]["id"]][1])
    return taken, cover


# ---- the plan ----------------------------------------------------------------------------

class Ctx:
    def __init__(self, db, W, spellmap, levels, opts):
        self.db, self.W, self.spellmap, self.levels, self.opts = db, W, spellmap, levels, opts
        self.byw = {w["wid"]: w for w in W}
        self.spells = {sid: (name, lvl) for sid, name, lvl in
                       db.execute("SELECT spellid, spellname, abilitylvl FROM spellpattern")}
        self.quests = {}
        self.quest_notes = {}
        if has_table(db, "quests"):
            notes = "notes" in [r[1] for r in db.execute("PRAGMA table_info(quests)")]
            for qid, name, enabled, text in db.execute(
                    f"SELECT questid, name, enabled, {'notes' if notes else chr(39) * 2} FROM quests"):
                self.quests[qid] = (name, enabled)
                self.quest_notes[qid] = text or ""
        self.import_keys = {}  # tools/import_quests.py's key -> questid
        if has_table(db, "quest_import_quests"):
            self.import_keys = {k: q for k, q in db.execute("SELECT key, questid FROM quest_import_quests")}
        self.were_gates = {}
        self.quest_spells = collections.defaultdict(set)  # spellid -> quests rewarding it
        if has_table(db, "queststeps"):
            for qid, sp in db.execute("SELECT questid, spells FROM queststeps WHERE spells != ''"):
                for sid in id_list(sp.replace(";", ",").replace(" ", ",")):
                    self.quest_spells[sid].add(qid)
        self.unmapped = collections.Counter()
        self.missing_spells = collections.Counter()

    def spell(self, x, name):
        """A workbook spell as our id and the window's "Name@level" (0 when
        the map lacks it)."""
        hit = self.spellmap.get(x)
        if not hit:
            self.unmapped[f"{x:04X} {name}"] += 1
            return 0, name
        sid = hit[0]
        have = self.spells.get(sid)
        if have:
            shown, lvl = (have[0] or "").strip().rstrip("$").strip() or name, have[1]
        else:
            self.missing_spells[sid] += 1
            shown, lvl = name, self.levels.get(x)
        return sid, f"{shown}@{lvl}" if lvl else shown

    def abilities(self, pairs):
        ids, names = [], []
        for x, name in pairs:
            sid, shown = self.spell(x, name)
            ids.append(str(sid))
            names.append(shown)
        return "; ".join(names), (",".join(ids) if any(i != "0" for i in ids) else "")


def target_ids(ctx, wid, row_classes, row_races, ids_of):
    """Our ids of a workbook rank another names (CM Req), those of the row's
    classes and races."""
    got = ids_of.get(wid, [])
    fit = [i for i, c, r in got if c & row_classes and r & row_races]
    return fit or [i for i, _, _ in got]


def plan(db, W, spellmap, levels, opts):
    ctx = Ctx(db, W, spellmap, levels, opts)
    O = ours(db)
    byo = {o["id"]: o for o in O}
    taken, cover = match(W, O, spellmap)
    owned = collections.defaultdict(list)  # cmid -> [(serverid, paid)]
    for sid, cmid, paid in db.execute("SELECT serverid, cmid, cost FROM charcms"):
        owned[cmid].append((sid, paid))
    gm_rows = set()
    if has_table(db, "gm_audit"):
        # a GM's edit in the panel or by an agent, not a tool's (this one's
        # own lines, an --undo's, say via "tool")
        for (k,) in db.execute("SELECT rowkey FROM gm_audit WHERE tbl = 'cmcatalog' AND reverted_by IS NULL "
                               "AND via != ?", (VIA,)):
            try:
                gm_rows.add(int(json.loads(k)["id"]))
            except (ValueError, KeyError, TypeError):
                pass

    # every workbook rank's rows: ours matched, else (or for classes or races
    # no row of ours covers) a new one
    notes = collections.defaultdict(list)
    rows_of = {}  # wid -> [(target id, classes, races)]
    new = []      # (w, classes, races, why)
    merged = {}   # wid -> our id: a workbook rank another workbook rank's row of ours is too
    for w in W:
        if cover[w["wid"]]:
            continue
        for oid, (wid0, _) in taken.items():
            o, w0 = byo[oid], ctx.byw[wid0]
            if w0["fname"] == w["fname"] and compat(w, o) and o["classes"] & w["classes"] == w["classes"] \
                    and o["races"] & w["races"] == w["races"]:
                merged[w["wid"]] = oid
                diff = [k for k in ("level", "cost", "spent", "slots", "base_stats", "adds", "removes", "abil")
                        if w[k] != w0[k]]
                notes["workbook ranks one row of ours is, by their classes (merged)"].append(
                    f"{w['hex']} {w['name']} ({w['classes']}) with {w0['hex']} ({w0['classes']}) in {oid}"
                    + (f": they differ in {', '.join(diff)} ({w0['hex']}'s taken)" if diff else ""))
                break
    for w in W:
        mine = cover[w["wid"]]
        if w["wid"] in merged:
            o = byo[merged[w["wid"]]]
            rows_of[w["wid"]] = [(o["id"], o["classes"], o["races"])]
            continue
        rows_of[w["wid"]] = [(o["id"], o["classes"], o["races"]) for o in mine]
        left_c = w["classes"] & ~sum_mask(o["classes"] for o in mine if o["races"] & w["races"] == w["races"])
        if not mine:
            new.append((w, w["classes"], w["races"], "new"))
            rows_of[w["wid"]].append((NEW_BASE + w["wid"], w["classes"], w["races"]))
        elif left_c and not any(o["classes"] == ALL_CLASSES for o in mine):
            new.append((w, left_c, w["races"], "classes no rank of ours is for"))
            rows_of[w["wid"]].append((NEW_BASE + w["wid"], left_c, w["races"]))
    ids_of = rows_of
    changes, inserts = [], []

    def desired(w, row_classes, row_races, cur, oid):
        """The row the workbook makes of a rank (cur: ours now, or None)."""
        d = {}
        cat = w["cat"]
        if cat == MASTER and opts.get("racial_mc") == "keep" and cur is not None and cur["category"] == 1:
            cat = 1
        if cat is not None:
            d["category"] = cat
        d["classes"], d["races"] = row_classes, row_races
        if w["name"]:
            d["name"] = w["name"][:63]
        if w["cost"] is not None:
            d["cost"] = w["cost"]
        if w["level"] is not None:
            d["level"] = w["level"]
        if w["spent"] is not None:
            d["spent"] = w["spent"]
        if w["desc"].strip().lower() not in NO_TEXT:
            d["description"] = w["desc"]
        # a row's groups are the workbook's whole, empty ones too: it and
        # EQOA-Data put a master class's stats on different ranks (Vanish's
        # are its Manticore Tail's, Touch of Nife's its Enlightened Body's)
        if cur is None or slots_of(cur["minstats"])[:7] != [w["base_stats"].get(i, 0) for i in range(7)]:
            d["minstats"] = min_text(w["base_stats"])
        if cur is None or slots_of(cur["effects"]) != w["slots"]:
            d["effects"] = stat_text(w["slots"])
        d["requires"], d["require_spells"] = ctx.abilities(w["abil"])
        d["adds"], d["add_spells"] = ctx.abilities(w["adds"])
        d["removes"], d["remove_spells"] = ctx.abilities(w["removes"])
        if w["reqs"]:
            targets = []
            for x, nm in w["reqs"]:
                if x not in ids_of:
                    notes["a rank it needs is not in the sheet"].append(f"{w['hex']} {w['name']}: {x:04X} {nm}")
                    continue
                other = ctx.byw[x]
                if w["cat"] == MASTER and other["cat"] == MASTER and other["fsub"] != w["fsub"]:
                    # the sheet's slip: Death Knight's abilities name Slayer, a
                    # master class of their own would then bar them
                    notes["a master class's rank needing another master class (the sheet's slip: left out)"].append(
                        f"{w['hex']} {w['name']} ({w['sub']}): {x:04X} {nm} ({other['sub']})")
                    continue
                for t in target_ids(ctx, x, row_classes, row_races, ids_of):
                    if t != oid and t not in targets:
                        targets.append(t)
            parent, others = pick_parent(ctx, w, targets, cur, byo, rows_of)
            # a rank the parent needs already is said once, by the parent
            # (Immortal Guard needs Defender's Guard, which needs Defender)
            up, seen = parent, set()
            while up and up not in seen:
                seen.add(up)
                up = byo[up]["parent"] if up in byo else 0
            others = [t for t in others if t not in seen]
            d["parent"] = parent
            mine = id_list(cur["require_ranks"]) if cur is not None else []
            d["require_ranks"] = ",".join(str(t) for t in (mine if sorted(mine) == sorted(others) else others))
        return d

    # ours matched
    for oid, (wid, how) in sorted(taken.items()):
        o = byo[oid]
        w = ctx.byw[wid]
        d = desired(w, o["classes"], o["races"], o, oid)
        if w["quest"] and not o["require_quests"]:
            q = quest_for(ctx, w, d)
            if q:
                d["require_quests"] = str(q)
            else:
                notes["a QUEST ID rank of ours with no quest found (left as it is)"].append(f"{oid} {o['name']}")
        diff = {k: v for k, v in d.items() if str(o[k]) != str(v)}
        if not diff:
            continue
        if "source" not in diff:
            src = o["source"] if SOURCE in (o["source"] or "") else f"{o['source']}; {SOURCE}".lstrip("; ")
            if src != o["source"]:
                diff["source"] = src
        changes.append({"op": "update", "id": oid, "wb": w["hex"], "how": how,
                        "before": {k: o[k] for k in diff}, "after": diff,
                        "gm": oid in gm_rows, "owners": len(owned.get(oid, []))})

    # new ranks
    used_ids = set(byo)
    for w, cls, rcs, why in new:
        nid = NEW_BASE + w["wid"]
        if nid in used_ids:
            notes["a new rank's id is taken (not written)"].append(f"{w['hex']} {w['name']} -> {nid}")
            continue
        used_ids.add(nid)
        d = desired(w, cls, rcs, None, nid)
        line, rank = new_line(w, d, byo, rows_of)
        row = {c: "" for c in COLS}
        row.update(id=nid, category=d.get("category", 0), classes=cls, races=rcs, mastery=line, rank=rank,
                   name=d.get("name", ""), parent=d.get("parent", 0), cost=d.get("cost", 0) or 0,
                   level=d.get("level", 1) or 1, spent=d.get("spent", 0) or 0, enabled=1, source=SOURCE)
        for k in ("minstats", "requires", "require_spells", "require_ranks", "effects", "adds", "add_spells",
                  "removes", "remove_spells", "description"):
            row[k] = d.get(k, "")
        row["require_quests"] = ""
        off, gate = [], ""
        if is_were(w):
            gate, kind = were_gate(ctx, w)
            if opts.get("weres") == "off":
                off.append("a were or were-hunter rank (--weres off)")
            elif not gate:
                off.append(f"no quest of its kind ({kind}) in quest_import_quests")
            else:
                ctx.were_gates[kind] = gate
        elif w["quest"]:
            q = quest_for(ctx, w, d)
            if q:
                gate = str(q)
            else:
                off.append("the quest it needs is not found")
        row["require_quests"] = gate
        # a group whose quests are all drafts bars no one until one is in the game
        open_groups = [g for g in parse_groups(gate) if not any(ctx.quests.get(q, ("", 0))[1] for q in g)]
        if open_groups and not off:
            notes["new ranks gated on a draft quest (it bars no one until it is in the game)"].append(
                f"{nid} {row['name']}: " + ", ".join(f"{q} {ctx.quests.get(q, ('?',))[0]}"
                                                    for g in open_groups for q in g))
        if off:
            row["enabled"] = 0
        inserts.append({"op": "insert", "id": nid, "wb": w["hex"], "how": why, "row": row, "off": off})

    # ours the workbook lacks: one the same as a new rank is disabled
    disables, kept = [], []
    new_by = collections.defaultdict(list)
    for e in inserts:
        new_by[(e["row"]["category"], fold(e["row"]["name"]))].append(e)
    for o in O:
        if o["id"] in taken:
            continue
        same = [e for e in inserts if e["row"]["classes"] & o["classes"] and e["row"]["races"] & o["races"]
                and cat_ok(ctx.byw[int(e["wb"], 16)], o)
                and (fold(e["row"]["name"]) == o["fname"]
                     or (any(o["slots"]) and slots_of(e["row"]["effects"]) == o["slots"]
                         and e["row"]["level"] == o["level"] and e["row"]["cost"] == o["cost"]))]
        if not same:
            kept.append(o)
            continue
        if owned.get(o["id"]):
            notes["ours the same as a new rank but owned: left enabled"].append(
                f"{o['id']} {o['name']} = {same[0]['id']} {same[0]['row']['name']} ({len(owned[o['id']])} own it)")
            kept.append(o)
            continue
        if not o["enabled"]:
            continue
        disables.append({"op": "disable", "id": o["id"], "before": {"enabled": o["enabled"]}, "after": {"enabled": 0},
                         "same": f"{same[0]['id']} {same[0]['row']['name']}", "name": o["name"],
                         "gm": o["id"] in gm_rows, "owners": 0})

    # the spell map's new spells where ours names one of ours of that name
    # (the spells import's to settle: "Decaying Flesh (24)" is new there, our
    # Death Sacrament 1 gives our Decaying Flesh, 918)
    for c in changes:
        a = c["after"]
        for names_col, ids_col in (("adds", "add_spells"), ("removes", "remove_spells"), ("requires", "require_spells")):
            if ids_col not in a:
                continue
            old_ids = dict(zip([ability_fold(n) for n in ability_names(c["before"].get(names_col, byo[c["id"]][names_col]))],
                               id_list(c["before"].get(ids_col, byo[c["id"]][ids_col]))))
            for n, i in zip([ability_fold(x) for x in ability_names(a.get(names_col, ""))], id_list(a[ids_col])):
                was = old_ids.get(n)
                if i >= NEW_BASE and i not in ctx.spells and was and was in ctx.spells and was != i:
                    notes["the spell map's new spell where ours names our spell of that name"].append(
                        f"{c['id']} {a.get('name', byo[c['id']]['name'])} {names_col}: {n}: ours {was}, the map's {i}")

    # what the plan does to the characters: a purchase whose stats move to a
    # new rank moves with them, and the owners' books follow their ranks
    charcms = {}
    for sid, cmid, cost, bought in db.execute("SELECT serverid, cmid, cost, bought FROM charcms"):
        charcms[(sid, cmid)] = {"serverid": sid, "cmid": cmid, "cost": cost, "bought": bought}
    moves = plan_moves(changes, inserts, byo, charcms, notes)
    books = plan_books(db, changes, inserts, moves, byo, charcms, ctx, notes)
    moved = {(m["serverid"], m["from"]) for m in moves}
    newcost = {c["id"]: c["after"]["cost"] for c in changes if "cost" in c["after"]}
    cost_changed = [(oid, sid, paid, newcost[oid]) for oid in newcost for sid, paid in owned.get(oid, [])
                    if (sid, oid) not in moved]
    cats = {o["id"]: o["category"] for o in O}
    for c in changes:
        if "category" in c["after"]:
            cats[c["id"]] = c["after"]["category"]
    two_mc = []
    by_char = collections.defaultdict(set)
    for cmid, lst in owned.items():
        for sid, _ in lst:
            by_char[sid].add(cmid)
    lines = {o["id"]: o["mastery"] for o in O}
    for sid, ids in by_char.items():
        mcs = {lines.get(i) for i in ids if cats.get(i) == MASTER}
        if len(mcs) > 1:
            two_mc.append((sid, sorted(m for m in mcs if m)))
    adds_missing = []
    for c in changes:
        if "add_spells" in c["after"] and owned.get(c["id"]):
            new_ids = set(id_list(c["after"]["add_spells"])) - set(id_list(c["before"].get("add_spells", ""))) - {0}
            if new_ids:
                adds_missing.append((c["id"], c["after"].get("name", byo[c["id"]]["name"]), len(owned[c["id"]]),
                                     sorted(new_ids)))
    return dict(changes=changes, inserts=inserts, disables=disables, moves=moves, books=books,
                kept=[(o["id"], o["name"]) for o in kept],
                taken=taken, cover={w: [o["id"] for o in v] for w, v in cover.items()}, notes=notes,
                cost_changed=cost_changed, two_mc=two_mc, adds_missing=adds_missing,
                gm_rows=sorted(gm_rows), unmapped=ctx.unmapped, missing_spells=ctx.missing_spells,
                lyc=[(w["hex"], w["name"], w["lyc"]) for w in W if w["lyc"]], were_gates=ctx.were_gates,
                quests=ctx.quests,
                quest_marked=[(w["hex"], w["name"]) for w in W if w["quest"]],
                O=byo, W=ctx.byw)


def plan_moves(changes, inserts, byo, charcms, notes):
    """A bought rank whose stats the plan moves to a new rank of its own
    master class or line (the workbook's Enlightened Body takes our Touch of
    Nife's): each owner holds the new rank at what it paid, and keeps the old
    one, its ability, at 0, so its points spent and its stats stay as they
    were (a #cmreset gives back the same). The coordinator, 2026-11-01."""
    out = []
    owners = collections.defaultdict(list)
    for (sid, cmid), row in charcms.items():
        owners[cmid].append(row)
    for c in changes:
        if "effects" not in c["after"] or not owners.get(c["id"]):
            continue
        was = slots_of(c["before"]["effects"])
        if not any(was) or slots_of(c["after"]["effects"]) == was:
            continue
        o = byo[c["id"]]
        to = [e["row"] for e in inserts if e["row"]["enabled"] and slots_of(e["row"]["effects"]) == was
              and e["row"]["mastery"] == o["mastery"] and e["row"]["classes"] & o["classes"]
              and e["row"]["races"] & o["races"]]
        if len(to) != 1:
            notes["a bought rank's stats change, none moved (no one new rank has them)"].append(
                f"{o['id']} {o['name']}: {c['before']['effects']!r} -> {c['after']['effects']!r}, "
                f"{len(owners[c['id']])} owners")
            continue
        for row in owners[c["id"]]:
            if (row["serverid"], to[0]["id"]) in charcms:
                continue
            out.append({"serverid": row["serverid"], "from": o["id"], "to": to[0]["id"], "paid": row["cost"],
                        "bought": row["bought"], "from_name": o["name"], "to_name": to[0]["name"]})
    return out


def book_change(row):
    """CMRank.BookChange of a cmcatalog row: (gives, takes) as [(name, spellid)],
    a spell both added and taken (improved in place) in neither."""
    def pairs(names, ids):
        ns, xs = ability_names(names), id_list(ids)
        return [(n, xs[i] if i < len(xs) else 0) for i, n in enumerate(ns)]
    adds, removes = pairs(row["adds"], row["add_spells"]), pairs(row["removes"], row["remove_spells"])
    both = {i for _, i in adds if i > 0} & {i for _, i in removes if i > 0}
    return ([a for a in adds if a[1] > 0 and a[1] not in both],
            [r for r in removes if r[1] > 0 and r[1] not in both])


BOOK_COLS = ("id", "serverid", "spellid", "addedorder", "onHotBar", "whereonBar", "unk1", "showhide")


def book_digest(rows):
    return json.dumps(sorted([[r[c] for c in BOOK_COLS] for r in rows]))


def plan_books(db, changes, inserts, moves, byo, charcms, ctx, notes):
    """Each owner's book brought in line with what buying its changed ranks
    gives now, as the buy path does it (handlers/classmastery.go buy, then
    giveAbilitiesTo and takeAbilitiesFrom): a rank's spells taught, those it
    replaces taken out, nothing else touched; a rank before the ranks after
    it, its gives before its takes. A spell taught where the rank takes out
    the one it improves (its name, or the one of each) takes that one's
    place in the book and on the bar (addedorder, onHotBar, whereonBar),
    as a player moved or hotkeyed it; any other at the next place below 252,
    off the bar (AddSpell's record). A spell spellpattern lacks is not taught
    (the client's book joins it). [{serverid, digest, ops}]."""
    after = {c["id"]: c["after"] for c in changes}
    newrows = {e["id"]: e["row"] for e in inserts}

    def row(cid):
        if cid in newrows:
            return newrows[cid]
        r = dict(byo[cid])
        r.update(after.get(cid, {}))
        return r

    touched = {c["id"] for c in changes if {"add_spells", "remove_spells"} & set(c["after"])}
    per = collections.defaultdict(set)
    for (sid, cmid) in charcms:
        if cmid in touched:
            per[sid].add(cmid)
    for m in moves:
        per[m["serverid"]].add(m["to"])
    known = set(ctx.spells)

    def depth(cid):
        d, seen = 0, set()
        while cid and cid not in seen and (cid in byo or cid in newrows):
            seen.add(cid)
            cid = row(cid)["parent"]
            d += 1
        return d

    out = []
    for sid in sorted(per):
        cur = db.execute(f"SELECT {', '.join(BOOK_COLS)} FROM spells WHERE serverid = ?", (sid,))
        orig = [dict(zip(BOOK_COLS, r)) for r in cur.fetchall()]
        book = [dict(r) for r in orig]
        for cid in sorted(per[sid], key=lambda c: (depth(c), c)):
            r = row(cid)
            gives, takes = book_change(r)
            held = {}
            for b in book:
                held.setdefault(b["spellid"], b)
            place = {}  # a given spell -> the row it takes the place of
            for tn, tid in takes:
                if tid not in held:
                    continue
                same = [g for g in gives if ability_fold(g[0]) == ability_fold(tn)]
                if not same and len(gives) == 1 and len(takes) == 1:
                    same = gives
                if same and same[0][1] not in place:
                    place[same[0][1]] = held[tid]
            for gn, gid in gives:
                if any(b["spellid"] == gid and gid in known for b in book):
                    continue
                if gid not in known:
                    notes["a spell a bought rank gives is not in spellpattern (not taught)"].append(
                        f"character {sid}, {cid} {r['name']}: {gid} {gn}")
                    continue
                if gid in place:
                    at = place[gid]
                    book.append({"id": None, "serverid": sid, "spellid": gid, "addedorder": at["addedorder"],
                                 "onHotBar": at["onHotBar"], "whereonBar": at["whereonBar"], "unk1": 0,
                                 "showhide": -1})
                    continue
                nxt = 0
                for b in book:
                    if b["spellid"] in known and nxt <= b["addedorder"] < 252:
                        nxt = b["addedorder"] + 1
                if nxt >= 252:
                    notes["a book full (252): not taught"].append(f"character {sid}, {cid} {r['name']}: {gid} {gn}")
                    continue
                book.append({"id": None, "serverid": sid, "spellid": gid, "addedorder": nxt, "onHotBar": 0,
                             "whereonBar": -1, "unk1": 0, "showhide": -1})
            gone = {tid for _, tid in takes}
            book = [b for b in book if b["spellid"] not in gone]
        ids = {b["id"] for b in book if b["id"] is not None}
        ops = [{"op": "remove", "row": b} for b in orig if b["id"] not in ids]
        ops += [{"op": "add", "row": b} for b in book if b["id"] is None]
        if ops:
            out.append({"serverid": sid, "digest": book_digest(orig), "ops": ops})
    return out


def sum_mask(masks):
    m = 0
    for x in masks:
        m |= x
    return m


def pick_parent(ctx, w, targets, cur, byo, rows_of):
    """The rank owned first (parent) among the ranks the workbook says it
    needs, the rest require_ranks: ours when it is one of them; else the one
    of its own line (the rank before it: its name's base with the number one
    less, or a master class's rank, the most specific: an ability it
    improves before the master class itself)."""
    if not targets:
        return 0, []
    if cur is not None and cur["parent"] in targets:
        p = cur["parent"]
        return p, [t for t in targets if t != p]
    names = {}
    for t in targets:
        o = byo.get(t)
        if o is not None:
            names[t] = (o["name"], o["mastery"], o["category"], o["parent"])
        else:
            nw = ctx.byw.get(t - NEW_BASE)
            names[t] = (nw["name"], nw["base"] if nw["cat"] != MASTER else nw["sub"], nw["cat"], None) if nw else ("", "", None, None)
    if w["num"] and w["num"] > 1:
        for t in targets:
            b, n = split_rank(names[t][0])
            if fold(b) == w["fbase"] and n == w["num"] - 1:
                return t, [x for x in targets if x != t]
    if w["cat"] == MASTER:
        mine = [t for t in targets if fold(names[t][1]) == fold(w["sub"]) or
                (byo.get(t) and byo[t]["category"] in (1, MASTER) and fold(byo[t]["mastery"]) == fold(w["sub"]))]
        if mine:
            # the ability it improves (itself needing the master class) before the master class
            deep = [t for t in mine if names[t][3] not in (0, None) or (t >= NEW_BASE and ctx.byw.get(t - NEW_BASE) and ctx.byw[t - NEW_BASE]["reqs"])]
            p = deep[-1] if deep else mine[0]
            return p, [x for x in targets if x != p]
        return 0, targets
    if len(targets) == 1:
        t = targets[0]
        tw_cat = names[t][2]
        if tw_cat == w["cat"]:
            return t, []
    return 0, targets


def new_line(w, d, byo, rows_of):
    """A new rank's mastery (its line's name) and rank: a master class's ranks
    share the master class's name (ours, when the master class is ours), a
    numbered rank its name's base and number."""
    if w["cat"] == MASTER:
        p = d.get("parent") or 0
        o = byo.get(p)
        if o is not None:
            line = o["mastery"]
            ranks = [x["rank"] for x in byo.values() if x["mastery"] == line and x["classes"] & d["classes"]]
            return line, max(ranks or [0]) + 1
        return w["sub"] or w["base"], (w["num"] or (1 if not w["reqs"] else 2))
    return w["base"], w["num"] or 1


# A were rank's kind (the sheet's Lycanthropy column) is the quest that makes
# a character one, as our quest data has it (tools/quests/data/weres.json,
# imported under these keys, quest_import_quests): each kind's chain p1, p2,
# p3, and p3 gives the form (Weres/bearwere/p3's last step the spell
# Bearwere); a werebeast's rank (any were) any of the five; a were-hunter's
# (the Were Hunter ranks and the Were-Hunter master classes) the brand, the
# quest that joins one to them ("Will you join the Were-Hunters? I will brand
# you with silver. No lycan's bite will ever take you after that": +1000 with
# the Werehunters of Tunaria; the trials after it are the level 50-60 path).
WERE_KINDS = {"werewolf": "wolfwere", "werebear": "bearwere", "wererat": "ratwere", "werelion": "lionwere",
              "weregator": "gatorwere"}
WEREHUNTER_QUEST = "Weres/werehunter/brand"


def were_gate(ctx, w):
    """A were or were-hunter rank's require_quests ("" when its quest is not
    imported) and its kind."""
    lyc, sub = fold(w["lyc"]), fold(w["sub"])
    if lyc == "were hunter" or "were hunter" in sub:
        kind, keys = "were-hunter", [WEREHUNTER_QUEST]
    elif lyc in WERE_KINDS:
        kind, keys = lyc, [f"Weres/{WERE_KINDS[lyc]}/p3"]
    else:  # Werebeast, and a were rank naming no kind: any were
        kind, keys = "werebeast (any were)", [f"Weres/{k}/p3" for k in WERE_KINDS.values()]
    ids = [ctx.import_keys[k] for k in keys if k in ctx.import_keys]
    if len(ids) != len(keys):
        return "", kind
    return "|".join(str(i) for i in ids), kind


def parse_groups(text):
    """require_quests as the server reads it (database.ParseRequires): every
    group, a group met by any of its quests."""
    out = []
    for part in (text or "").split(","):
        g = [int(x) for x in re.findall(r"-?\d+", part)]
        if "|" in part:
            if g:
                out.append(g)
        else:
            out += [[x] for x in g]
    return out


def quest_for(ctx, w, d):
    """The quest a "QUEST ID" rank needs: one rewarding a spell it adds, else
    one named as the rank or a spell it adds, else 0."""
    found = set()
    for sid in id_list(d.get("add_spells", "")):
        found |= ctx.quest_spells.get(sid, set())
    if len(found) == 1:
        return found.pop()
    names = {fold(w["name"])} | {fold(n) for _, n in w["adds"]}
    names |= {n.replace(" ", "") for n in names}
    hits = [q for q, (nm, _) in ctx.quests.items() if fold(nm) in names or fold(nm).replace(" ", "") in names]
    if len(hits) == 1:
        return hits[0]
    rank = fold(w["name"])
    told = [q for q, notes in ctx.quest_notes.items() if rank and f" {rank} " in f" {fold(notes)} "
            and "mastery point" in notes]
    return told[0] if len(told) == 1 else 0


# ---- the report ---------------------------------------------------------------------------

def report(p, full=False):
    W, O = p["W"], p["O"]
    hows = collections.Counter(how for _, how in p["taken"].values())
    print(f"the workbook: {len(W)} ranks; ours: {len(O)}")
    print(f"ours matched: {len(p['taken'])} rows (" + ", ".join(f"{h} {n}" for h, n in hows.most_common()) + ")")
    multi = {w: ids for w, ids in p["cover"].items() if len(ids) > 1}
    merged = collections.Counter(wid for wid, _ in p["taken"].values())
    print(f"  workbook ranks that are several of ours (a row a class): {len(multi)}")
    many = collections.defaultdict(list)
    for oid, (wid, _) in p["taken"].items():
        many[oid].append(wid)
    renamed = [c for c in p["changes"] if "name" in c["after"]]
    print(f"rows changed: {len(p['changes'])}; renamed {len(renamed)}")
    cols = collections.Counter(k for c in p["changes"] for k in c["after"] if k != "source")
    print("  columns changed: " + ", ".join(f"{k} {n}" for k, n in cols.most_common()))
    ins = p["inserts"]
    off = [e for e in ins if e["off"]]
    print(f"new ranks: {len(ins)} ({len(ins) - len(off)} offered, {len(off)} written disabled)")
    why = collections.Counter(r for e in off for r in e["off"])
    for r, n in why.most_common():
        print(f"  disabled: {r}: {n}")
    by_cat = collections.Counter(CATEGORY_NAMES[e["row"]["category"]] for e in ins)
    print("  by category: " + ", ".join(f"{k} {n}" for k, n in by_cat.most_common()))
    print(f"ours disabled (the same as a new rank): {len(p['disables'])}")
    for d in p["disables"]:
        print(f"  {d['id']} {d['name']} = {d['same']}")
    print(f"ours the workbook lacks, kept as they are: {len(p['kept'])}")
    for oid, name in p["kept"]:
        print(f"  {oid} {name}")
    gm = [c for c in p["changes"] + p["disables"] if c.get("gm")]
    print(f"rows a GM changed in the panel, the workbook's now: {len(gm)} of {len(p['gm_rows'])}")
    for c in gm:
        print(f"  {c['id']}: " + ", ".join(sorted(k for k in c['after'] if k != 'source')))
    cc = p["cost_changed"]
    print(f"bought ranks whose cost changes (charcms keeps what was paid): {len(cc)} "
          f"({len({x[0] for x in cc})} ranks, {len({x[1] for x in cc})} characters)")
    if p["adds_missing"]:
        print(f"bought ranks that now add abilities their owners were not given ({len(p['adds_missing'])}):")
        for oid, name, n, ids in p["adds_missing"]:
            print(f"  {oid} {name}: {n} owners, spells {ids}")
    for m in p.get("moves", []):
        print(f"purchase moved: character {m['serverid']} holds {m['to']} {m['to_name']} at the {m['paid']} it paid for "
              f"{m['from']} {m['from_name']}, kept at 0")
    books = p.get("books", [])
    ops = collections.Counter(o["op"] for b in books for o in b["ops"])
    placed = sum(1 for b in books for o in b["ops"] if o["op"] == "add" and
                 any(r["op"] == "remove" and r["row"]["addedorder"] == o["row"]["addedorder"] for r in b["ops"]))
    print(f"books brought in line with their bought ranks: {len(books)} characters, {ops['add']} spells taught "
          f"({placed} in the place of the spell they replace), {ops['remove']} taken out")
    cat_moves = collections.Counter((CATEGORY_NAMES[c["before"]["category"]], CATEGORY_NAMES[c["after"]["category"]])
                                    for c in p["changes"] if "category" in c["after"])
    for (a, b), n in cat_moves.most_common():
        print(f"category {a} -> {b}: {n} ranks")
    if p["two_mc"]:
        print(f"characters holding two master classes after it: {len(p['two_mc'])}")
        for sid, mcs in p["two_mc"]:
            print(f"  {sid}: {', '.join(mcs)}")
    narrowed = [c for c in p["changes"] if "classes" in c["after"] or "races" in c["after"]]
    for c in narrowed:
        print(f"  {c['id']} classes/races {c['before'].get('classes', '')}/{c['before'].get('races', '')} -> "
              f"{c['after'].get('classes', '')}/{c['after'].get('races', '')}")
    if p["missing_spells"]:
        print(f"spells the plan names that spellpattern lacks (the spells import makes them): "
              f"{len(p['missing_spells'])}")
    if p["unmapped"]:
        print(f"workbook spells not in the spell map (written as 0, shown only): {len(p['unmapped'])}")
        for k in sorted(p["unmapped"]):
            print(f"  {k}")
    print(f"Lycanthropy marked: {len(p['lyc'])} ranks (" + ", ".join(
        f"{k} {n}" for k, n in collections.Counter(x[2] for x in p['lyc']).most_common()) + ")")
    print(f"Quest marked (\"QUEST ID\"): {len(p['quest_marked'])} ranks")
    for kind, gate in sorted(p["were_gates"].items()):
        qs = [f"{q} {p['quests'].get(q, ('?', 0))[0]}" + ("" if p["quests"].get(q, ("", 0))[1] else " (draft)")
              for g in parse_groups(gate) for q in g]
        print(f"  {kind} ranks need {'any of ' if '|' in gate else ''}{', '.join(qs)}")
    for k, v in p["notes"].items():
        print(f"{k}: {len(v)}")
        for x in v:
            print(f"  {x}")
    if full:
        for c in p["changes"]:
            o = O[c["id"]]
            print(f"~ {c['id']} {o['name']!r} <- {c['wb']} ({c['how']}): " +
                  "; ".join(f"{k}: {c['before'][k]!r} -> {v!r}" for k, v in c["after"].items() if k != "source"))
        for e in ins:
            r = e["row"]
            print(f"+ {r['id']} {CATEGORY_NAMES[r['category']]} {r['name']!r} ({e['how']}) enabled {r['enabled']} "
                  f"classes {r['classes']} races {r['races']} parent {r['parent']} needs {r['require_ranks']!r} "
                  f"cost {r['cost']} level {r['level']} | {r['effects']} | +{r['adds']} -{r['removes']}"
                  + (f" | off: {'; '.join(e['off'])}" if e["off"] else ""))


# ---- writing it ------------------------------------------------------------------------

def go_json(m):
    """A row as Go's json.Marshal writes a map (keys sorted, <>& escaped)."""
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def row_of(db, cid):
    cur = db.execute("SELECT * FROM cmcatalog WHERE id = ?", (cid,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def audit(db, action, cid, before, after, detail, tbl="cmcatalog", key=None):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() \
        if has_table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key if key is not None else {"id": cid}),
                      None if before is None else go_json(before), None if after is None else go_json(after), detail))
    return cur.lastrowid


CHAR_KEYS = {"spells": ("id",), "charcms": ("cmid", "serverid")}


def char_row(db, tbl, key):
    cols = BOOK_COLS if tbl == "spells" else ("serverid", "cmid", "cost", "bought")
    where = " AND ".join(f"{k} = ?" for k in CHAR_KEYS[tbl])
    r = db.execute(f"SELECT {', '.join(cols)} FROM {tbl} WHERE {where}", [key[k] for k in CHAR_KEYS[tbl]]).fetchone()
    return None if r is None else dict(zip(cols, r))


def char_write(db, tbl, key, before, after, when, detail=DETAIL):
    """One character row changed as the plan says (before None: made; after
    None: taken out), audited as the panel's edit and logged. The new row's
    key (a spells row's id is the table's)."""
    if before is None:
        cols = [c for c in after if not (tbl == "spells" and c == "id")]
        cur = db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                         [after[c] for c in cols])
        if tbl == "spells":
            after = dict(after, id=cur.lastrowid)
            key = {"id": cur.lastrowid}
        action = "insert"
    elif after is None:
        db.execute(f"DELETE FROM {tbl} WHERE {' AND '.join(f'{k} = ?' for k in CHAR_KEYS[tbl])}",
                   [key[k] for k in CHAR_KEYS[tbl]])
        action = "delete"
    else:
        sets = [c for c in after if c not in CHAR_KEYS[tbl]]
        db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in sets)} WHERE "
                   f"{' AND '.join(f'{k} = ?' for k in CHAR_KEYS[tbl])}",
                   [after[c] for c in sets] + [key[k] for k in CHAR_KEYS[tbl]])
        action = "update"
    a = audit(db, action, 0, before, after, detail, tbl=tbl, key=key)
    if when is not None:
        db.execute(f"INSERT INTO {CHARS_LOG} (tbl, key, before, after, audit, applied) VALUES (?, ?, ?, ?, ?, ?)",
                   (tbl, json.dumps(key, sort_keys=True), None if before is None else json.dumps(before),
                    None if after is None else json.dumps(after), a, when))
    return key


def apply_chars(db, p, when):
    """The plan's moved purchases and books, each only where the character's
    rows are still as planned: (done, changed since)."""
    db.execute(CHARS_LOG_SCHEMA)
    done = stale = 0
    for m in p.get("moves", []):
        frm = char_row(db, "charcms", {"serverid": m["serverid"], "cmid": m["from"]})
        to_key = {"cmid": m["to"], "serverid": m["serverid"]}
        if frm is None or frm["cost"] != m["paid"] or char_row(db, "charcms", to_key) is not None:
            stale += 1
            continue
        char_write(db, "charcms", to_key, None,
                   {"serverid": m["serverid"], "cmid": m["to"], "cost": m["paid"], "bought": m["bought"]}, when)
        char_write(db, "charcms", {"cmid": m["from"], "serverid": m["serverid"]}, frm, dict(frm, cost=0), when)
        done += 1
    have = {r[0] for r in db.execute("SELECT spellid FROM spellpattern")}
    for b in p.get("books", []):
        rows = [dict(zip(BOOK_COLS, r)) for r in
                db.execute(f"SELECT {', '.join(BOOK_COLS)} FROM spells WHERE serverid = ?", (b["serverid"],))]
        if book_digest(rows) != b["digest"] or any(o["op"] == "add" and o["row"]["spellid"] not in have for o in b["ops"]):
            stale += len(b["ops"])
            continue
        for o in b["ops"]:
            if o["op"] == "remove":
                char_write(db, "spells", {"id": o["row"]["id"]}, o["row"], None, when)
            else:
                char_write(db, "spells", None, None, o["row"], when)
            done += 1
    return done, stale


def undo_chars(db):
    """Every logged character row still as written put back, newest first:
    (back, left, why)."""
    back, left, why = 0, 0, []
    if not has_table(db, CHARS_LOG):
        return back, left, why
    for n, tbl, key, before, after in db.execute(
            f"SELECT n, tbl, key, before, after FROM {CHARS_LOG} ORDER BY n DESC").fetchall():
        key = json.loads(key)
        before = None if before is None else json.loads(before)
        after = None if after is None else json.loads(after)
        now = char_row(db, tbl, key)
        if (after is None and now is not None) or (after is not None and now != after):
            left += 1
            why.append(f"{tbl} {key}: changed since")
            continue
        if after is None:  # taken out: put back as it was, its own id
            cols = list(before)
            db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                       [before[c] for c in cols])
            audit(db, "insert", 0, None, before, "undo: " + DETAIL, tbl=tbl, key=key)
        else:
            char_write(db, tbl, key, after, before, None, "undo: " + DETAIL)
        db.execute(f"DELETE FROM {CHARS_LOG} WHERE n = ?", (n,))
        back += 1
    if not db.execute(f"SELECT count(*) FROM {CHARS_LOG}").fetchone()[0]:
        db.execute(f"DROP TABLE {CHARS_LOG}")
    return back, left, why


def same(row, cols):
    return all(str(row.get(k)) == str(v) for k, v in cols.items())


def apply(db, p, allow_missing=False):
    """Every planned change whose row is still as planned, in one
    transaction: (done, changed since)."""
    entries = p["changes"] + p["inserts"] + p["disables"]
    if not allow_missing and missing_spells(db, p):
        sys.exit("spells the plan names are not in spellpattern: run the spells import first")
    done = stale = 0
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(AUDIT_SCHEMA)
        db.execute(LOG_SCHEMA)
        for e in entries:
            cid = e["id"]
            now = row_of(db, cid)
            if db.execute(f"SELECT 1 FROM {LOG} WHERE id = ?", (cid,)).fetchone():
                stale += 1
                continue
            if e["op"] == "insert":
                if now is not None:
                    stale += 1
                    continue
                r = e["row"]
                db.execute(f"INSERT INTO cmcatalog ({', '.join(COLS)}) VALUES ({', '.join('?' * len(COLS))})",
                           [r[c] for c in COLS])
                a = audit(db, "insert", cid, None, row_of(db, cid), DETAIL)
                db.execute(f"INSERT INTO {LOG} VALUES (?, 'insert', NULL, ?, ?, ?)", (cid, json.dumps(r), a, when))
            else:
                if now is None or not same(now, e["before"]):
                    stale += 1
                    continue
                sets = e["after"]
                db.execute(f"UPDATE cmcatalog SET {', '.join(f'{k} = ?' for k in sets)} WHERE id = ?",
                           [sets[k] for k in sets] + [cid])
                a = audit(db, "update", cid, now, row_of(db, cid), DETAIL)
                db.execute(f"INSERT INTO {LOG} VALUES (?, ?, ?, ?, ?, ?)",
                           (cid, e["op"], json.dumps({k: now[k] for k in sets}), json.dumps(sets), a, when))
            done += 1
        cdone, cstale = apply_chars(db, p, when)
        done, stale = done + cdone, stale + cstale
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return done, stale


def missing_spells(db, p):
    """The spells a plan gives, needs or takes that spellpattern lacks."""
    have = {r[0] for r in db.execute("SELECT spellid FROM spellpattern")}
    need = set()
    for e in p["changes"] + p["inserts"] + p["disables"]:
        cols = e["row"] if e["op"] == "insert" else e["after"]
        for k in ("add_spells", "require_spells", "remove_spells"):
            need |= {i for i in id_list(cols.get(k, "")) if i > 0}
    return sorted(need - have)


def undo(db):
    """Every logged row still as written back as it was: (back, left, why)."""
    if not has_table(db, LOG) and not has_table(db, CHARS_LOG):
        return 0, 0, []
    db.execute("BEGIN IMMEDIATE")
    try:
        back, left, why = undo_chars(db)
        owned = {r[0] for r in db.execute("SELECT DISTINCT cmid FROM charcms")} if has_table(db, "charcms") else set()
        rows = db.execute(f"SELECT id, op, before, after FROM {LOG} ORDER BY id").fetchall() if has_table(db, LOG) else []
        for cid, op, before, after in rows:
            now = row_of(db, cid)
            after = json.loads(after)
            if now is None or not same(now, after):
                left += 1
                why.append(f"{cid}: changed since")
                continue
            if op == "insert":
                if cid in owned:
                    left += 1
                    why.append(f"{cid} {now['name']}: a character bought it since")
                    continue
                db.execute("DELETE FROM cmcatalog WHERE id = ?", (cid,))
                audit(db, "delete", cid, now, None, "undo: " + DETAIL)
            else:
                b = json.loads(before)
                db.execute(f"UPDATE cmcatalog SET {', '.join(f'{k} = ?' for k in b)} WHERE id = ?",
                           [b[k] for k in b] + [cid])
                audit(db, "update", cid, now, row_of(db, cid), "undo: " + DETAIL)
            db.execute(f"DELETE FROM {LOG} WHERE id = ?", (cid,))
            back += 1
        if has_table(db, LOG) and not db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0]:
            db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return back, left, why


def check(db):
    """What the catalog holds that the server would read wrong: a parent or a
    needed rank that is not there, a parent chain that loops, a stat key the
    server skips, a name or text CLIENT cuts, a bought rank not in it, two
    offered ranks one player would see twice. ([(what, row)], counts)."""
    rows = {r["id"]: r for r in ours(db)}
    bad = []
    for r in rows.values():
        if r["parent"] and r["parent"] not in rows:
            bad.append(("its parent is not in the catalog", r))
        for t in id_list(r["require_ranks"]):
            if t not in rows:
                bad.append((f"a rank it needs ({t}) is not in the catalog", r))
        seen, up = set(), r["id"]
        while up and up in rows and up not in seen:
            seen.add(up)
            up = rows[up]["parent"]
        if up and up in seen:
            bad.append(("its parent chain loops", r))
        for col in ("effects", "minstats"):
            for f in (r[col] or "").split(","):
                k = f.partition(":")[0].strip().lower().replace(" ", "")
                if k and k not in SERVER_KEYS:
                    bad.append((f"{col} key {k!r} the server skips", r))
        if len(r["name"]) > 63:
            bad.append(("a name past 63 characters", r))
        if len((r["description"] or r["mastery"]) + f" Cost: {r['cost']}.") > 255:
            bad.append(("a text past 255 characters with its cost", r))
        for names, ids in (("requires", "require_spells"), ("adds", "add_spells"), ("removes", "remove_spells")):
            if r[ids] and len(id_list(r[ids])) != len(ability_names(r[names])):
                bad.append((f"{names} and {ids} differ in length", r))
    quests = {q for (q,) in db.execute("SELECT questid FROM quests")} if has_table(db, "quests") else set()
    for r in rows.values():
        for g in parse_groups(r["require_quests"]):
            for q in g:
                if q not in quests:
                    bad.append((f"a quest it needs ({q}) is not in quests (it bars no one)", r))
    if has_table(db, "charcms"):
        for (cmid,) in db.execute("SELECT DISTINCT cmid FROM charcms"):
            if cmid not in rows:
                bad.append(("a bought rank not in the catalog", {"id": cmid, "name": "?"}))
    on = [r for r in rows.values() if r["enabled"]]
    by = collections.defaultdict(list)
    for r in on:
        by[(r["category"], r["fname"])].append(r)
    for group in by.values():
        for i, a in enumerate(group):
            for b in group[i + 1:]:
                if a["classes"] & b["classes"] and a["races"] & b["races"]:
                    bad.append((f"offered twice to one player (with {b['id']})", a))
    return bad, collections.Counter(r["category"] for r in on)


def backup(path):
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-eqoaemu-cms"
    src, out = sqlite3.connect(path, timeout=30), sqlite3.connect(dst)
    try:
        src.backup(out)
    finally:
        out.close()
        src.close()
    return dst


def plan_json(p):
    return {"made": datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds"),
            "changes": p["changes"], "inserts": p["inserts"], "disables": p["disables"],
            "moves": p["moves"], "books": p["books"]}


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--sheet", help="EQOAEmu's workbook (.ods), the operator's copy")
    ap.add_argument("--tsv-dir", help="its sheets as TSV (06-CMs.tsv, 02-Spells.tsv)")
    ap.add_argument("--spellmap", help="the import's spell map: the workbook's SpellID to our spellid (TSV)")
    ap.add_argument("--weres", choices=("off", "on"), default="on",
                    help="the were and were-hunter ranks offered, each behind its kind's quest (on, the default), "
                         "or written disabled (off)")
    ap.add_argument("--racial-mc", choices=("master", "keep"), default="master",
                    help="the racial master classes in Master Class (4, the workbook's) or kept in Racial (1)")
    ap.add_argument("--save-plan", metavar="FILE", help="write the plan (JSON) and stop")
    ap.add_argument("--plan", metavar="FILE", help="a plan --save-plan wrote, instead of the workbook")
    ap.add_argument("--list", action="store_true", help="every change and new rank")
    ap.add_argument("--allow-missing-spells", action="store_true",
                    help="--apply though spells the plan names are not in spellpattern yet")
    ap.add_argument("--no-backup", action="store_true")
    ap.add_argument("--check", action="store_true", help="say what in the catalog the server would read wrong")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo and not a.no_backup:
        print(f"copy: {backup(a.db)}")
    if a.check:
        bad, cats = check(db)
        print("offered: " + ", ".join(f"{CATEGORY_NAMES[c]} {n}" for c, n in sorted(cats.items())))
        for what, r in bad:
            print(f"  {r['id']} {r['name']}: {what}")
        print(f"{len(bad)} problems")
        return
    if a.undo:
        back, left, why = undo(db)
        print(f"put back {back}; left {left}")
        for x in why:
            print(f"  {x}")
        return
    if a.plan:
        with open(a.plan, encoding="utf-8") as f:
            p = json.load(f)
        print(f"plan {a.plan}: {len(p['changes'])} changes, {len(p['inserts'])} new, {len(p['disables'])} disabled, "
              f"{len(p.get('moves', []))} purchases moved, {sum(len(b['ops']) for b in p.get('books', []))} book "
              f"rows of {len(p.get('books', []))} characters")
    else:
        if not a.spellmap or not (a.sheet or a.tsv_dir):
            sys.exit("--sheet or --tsv-dir, and --spellmap (or --plan)")
        if a.sheet:
            cms, spells = Sheet(ods_table(a.sheet, "CMs")), Sheet(ods_table(a.sheet, "Spells"))
        else:
            cms = Sheet(tsv_table(os.path.join(a.tsv_dir, "06-CMs.tsv")))
            sp = os.path.join(a.tsv_dir, "02-Spells.tsv")
            spells = Sheet(tsv_table(sp)) if os.path.exists(sp) else None
        W = read_ranks(cms)
        slips = fix_slips(W, read_spell_owners(spells))
        for a, b in swap_descriptions(W):
            print(f"the workbook's descriptions of {a} and {b}, each the other's: swapped")
        print(f"the workbook's ability ids of another class's spell, put right: {len(slips)}")
        for hx, kind, was, now, nm in slips:
            what = {"abil": "requires", "adds": "adds", "removes": "takes away"}[kind]
            print(f"  {hx} {next(w['name'] for w in W if w['hex'] == hx)} {what} {nm}: {was:04X} -> {now:04X}")
        p = plan(db, W, read_spellmap(a.spellmap), read_spell_levels(spells),
                 {"weres": a.weres, "racial_mc": "keep" if a.racial_mc == "keep" else "master"})
        report(p, a.list)
        if a.save_plan:
            with open(a.save_plan, "w", encoding="utf-8") as f:
                json.dump(plan_json(p), f, indent=1, ensure_ascii=False)
            print(f"plan: {a.save_plan}")
            return
    if a.apply:
        if not a.allow_missing_spells:
            lack = missing_spells(db, p)
            if lack:
                sys.exit(f"{len(lack)} spells the plan gives, needs or takes are not in spellpattern ({lack[:8]} ...): "
                         f"run the spells import first, or --allow-missing-spells")
        if not a.no_backup:
            print(f"copy: {backup(a.db)}")
        done, stale = apply(db, p, a.allow_missing_spells)
        print(f"written {done}; {stale} changed since the plan, left")
    else:
        print("(report only: --apply to write it)")


if __name__ == "__main__":
    main()
