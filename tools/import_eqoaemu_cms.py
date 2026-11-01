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
- A cell left empty (or "NO DESCRIPTION FOUND") is no value: ours stays. A
  group of cells is the workbook's whole when it fills any of it (the 29
  stats, the base stats, the abilities added, ...).
- **A rank we lack is NEW**, id 900000 + its hex id (the import's
  convention: the spells' too). A were or were-hunter rank (Lycanthropy, the
  Were and Were Hunter sub-categories, the Were-Hunter master classes) is
  written DISABLED (enabled 0) unless --weres on: the were lines are not built
  on this server (no lycanthropy, CLAUDE.md "Not built: the were lines"), and
  the user said of the were forms in F3 pass 17 "Remove them". A new rank the
  workbook marks "QUEST ID" takes the quest it names that is in the game
  (found by the abilities it adds or by name), else is written disabled: a
  quest not in the game bars no one (cmBlockLocked).
- Our ranks the workbook lacks stay as they are, but one clearly the same as
  a NEW workbook rank (its name, or its stats at its level, in its group) is
  disabled (never deleted), unless a character owns it.
- charcms is not touched: what a character paid stays its paid cost; a
  bought rank whose cost changes is counted.
- Rows a GM changed (gm_audit, tbl cmcatalog) are overwritten too, and listed.
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
        if has_table(db, "quests"):
            for qid, name, enabled in db.execute("SELECT questid, name, enabled FROM quests"):
                self.quests[qid] = (name, enabled)
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
        for (k,) in db.execute("SELECT rowkey FROM gm_audit WHERE tbl = 'cmcatalog' AND reverted_by IS NULL"):
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
        off = []
        if is_were(w) and opts.get("weres") != "on":
            off.append("a were or were-hunter rank (the were lines are not built here)")
        if w["quest"]:
            q = quest_for(ctx, w, d)
            if q and ctx.quests[q][1]:
                row["require_quests"] = str(q)
            elif q:
                row["require_quests"] = str(q)
                off.append(f"its quest {q} ({ctx.quests[q][0]}) is a draft")
            elif not is_were(w):
                off.append("the quest it needs is not found")
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

    # what the plan does to the characters
    newcost = {c["id"]: c["after"]["cost"] for c in changes if "cost" in c["after"]}
    cost_changed = [(oid, sid, paid, newcost[oid]) for oid in newcost for sid, paid in owned.get(oid, [])]
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
    return dict(changes=changes, inserts=inserts, disables=disables, kept=[(o["id"], o["name"]) for o in kept],
                taken=taken, cover={w: [o["id"] for o in v] for w, v in cover.items()}, notes=notes,
                cost_changed=cost_changed, two_mc=two_mc, adds_missing=adds_missing,
                gm_rows=sorted(gm_rows), unmapped=ctx.unmapped, missing_spells=ctx.missing_spells,
                lyc=[(w["hex"], w["name"], w["lyc"]) for w in W if w["lyc"]],
                quest_marked=[(w["hex"], w["name"]) for w in W if w["quest"]],
                O=byo, W=ctx.byw)


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
    return hits[0] if len(hits) == 1 else 0


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


def audit(db, action, cid, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() \
        if has_table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, 'cmcatalog', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, go_json({"id": cid}),
                      None if before is None else go_json(before), None if after is None else go_json(after), detail))
    return cur.lastrowid


def same(row, cols):
    return all(str(row.get(k)) == str(v) for k, v in cols.items())


def apply(db, p, allow_missing=False):
    """Every planned change whose row is still as planned, in one
    transaction: (done, changed since)."""
    entries = p["changes"] + p["inserts"] + p["disables"]
    if not allow_missing:
        have = {r[0] for r in db.execute("SELECT spellid FROM spellpattern")}
        need = set()
        for e in entries:
            cols = e["row"] if e["op"] == "insert" else e["after"]
            for k in ("add_spells", "require_spells", "remove_spells"):
                need |= {i for i in id_list(cols.get(k, "")) if i > 0}
        lack = sorted(need - have)
        if lack:
            sys.exit(f"{len(lack)} spells the plan gives, needs or takes are not in spellpattern ({lack[:8]} ...): "
                     f"run the spells import first, or --allow-missing-spells")
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
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return done, stale


def undo(db):
    """Every logged row still as written back as it was: (back, left, why)."""
    if not has_table(db, LOG):
        return 0, 0, []
    back, left, why = 0, 0, []
    owned = {r[0] for r in db.execute("SELECT DISTINCT cmid FROM charcms")} if has_table(db, "charcms") else set()
    db.execute("BEGIN IMMEDIATE")
    try:
        for cid, op, before, after in db.execute(f"SELECT id, op, before, after FROM {LOG} ORDER BY id").fetchall():
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
        if not db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0]:
            db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return back, left, why


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
            "changes": p["changes"], "inserts": p["inserts"], "disables": p["disables"]}


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--sheet", help="EQOAEmu's workbook (.ods), the operator's copy")
    ap.add_argument("--tsv-dir", help="its sheets as TSV (06-CMs.tsv, 02-Spells.tsv)")
    ap.add_argument("--spellmap", help="the import's spell map: the workbook's SpellID to our spellid (TSV)")
    ap.add_argument("--weres", choices=("off", "on"), default="off",
                    help="the were and were-hunter ranks written offered (on) or disabled (off, the default)")
    ap.add_argument("--racial-mc", choices=("master", "keep"), default="master",
                    help="the racial master classes in Master Class (4, the workbook's) or kept in Racial (1)")
    ap.add_argument("--save-plan", metavar="FILE", help="write the plan (JSON) and stop")
    ap.add_argument("--plan", metavar="FILE", help="a plan --save-plan wrote, instead of the workbook")
    ap.add_argument("--list", action="store_true", help="every change and new rank")
    ap.add_argument("--allow-missing-spells", action="store_true",
                    help="--apply though spells the plan names are not in spellpattern yet")
    ap.add_argument("--no-backup", action="store_true")
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
        back, left, why = undo(db)
        print(f"put back {back}; left {left}")
        for x in why:
            print(f"  {x}")
        return
    if a.plan:
        with open(a.plan, encoding="utf-8") as f:
            p = json.load(f)
        print(f"plan {a.plan}: {len(p['changes'])} changes, {len(p['inserts'])} new, {len(p['disables'])} disabled")
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
        p = plan(db, W, read_spellmap(a.spellmap), read_spell_levels(spells),
                 {"weres": a.weres, "racial_mc": "keep" if a.racial_mc == "keep" else "master"})
        report(p, a.list)
        if a.save_plan:
            with open(a.save_plan, "w", encoding="utf-8") as f:
                json.dump(plan_json(p), f, indent=1, ensure_ascii=False)
            print(f"plan: {a.save_plan}")
            return
    if a.apply:
        done, stale = apply(db, p, a.allow_missing_spells)
        print(f"written {done}; {stale} changed since the plan, left")
    else:
        print("(report only: --apply to write it)")


if __name__ == "__main__":
    main()
