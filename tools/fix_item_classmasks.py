#!/usr/bin/env python3
"""The wearable items with no class mask given one (the user, 2026-11-05: "lets fix the 2001 items").

itempattern.classuse is the classes that may use an item, a bit a class (1 << class: WAR 0 .. ALC 14; all fifteen
32767). CLIENT's VIPlayer::CanUse (0x00BCA8F0) refuses an item whose mask lacks the player's bit, so a mask of 0 is an
item no class may use: its item menu marks it, CLIENT sends the equip all the same, and the server puts it on
(handlers/equip.go cannotWear does not check a mask of 0). A mask also decides what the balance simulator's kits and
#template wear (database.WearablePatterns never picks a mask of 0) and, once it is not 0, whom the server lets put
the item on. On eqoaclassic's copy of 2026-11-05 1,996 wearable records had one (five more had been set by hand),
nearly all Return Home's dump's (tools/import_prh_content.py, patternids 5000-8999).

A record counts when it is worn (equipslot above 0) and its mask is 0; one a GM edited in the panel (gm_audit) is
listed and left. Each takes the first of these that gives it a mask:
1. **Sandstorm's record** (--harvests: eqoa harvest JSON, its merchants' wares and its characters' items, SOE's
   records): one of the same name (fix_dump_weapons.key) and wear slot; where its records of the name differ, the
   one of the record's level; where they still differ, none.
2. **Allakhazam's item page** (--pages: tools/fetch_item_pages.py's folder, the live game's items as its players
   submitted them, 2003-2007): a page of the same name whose slot is the record's ("Arms" bracers or a bracelet,
   "Torso" a chest, "Primary" any weapon), by its "Classes" line ("All Classes" or class names); where pages of the
   name differ, the one of the record's level, else none. A page of armour naming exactly the ten classes before the
   alchemist (1023, the leather wearers of their day) gives the leather wearers of the live game, 17407, as
   Sandstorm's records of such pieces have them.
3. **The same item here**: records of the same name and wear slot with a mask (a trailing " (copy)" aside), all one.
4. **The rule** (OURS):
   a. an armour piece's set: the other pieces of the set its name says (the name without the piece's noun,
      "Cavernous Bracers", "Gloves of the Howler"; a possessive or a plural s aside) within 3 levels of it that
      Sandstorm's records or Allakhazam's pages give a mask, all one;
   b. else (--no-rule leaves these at 0) jewellery and belts every class; armour by its material (itempattern.model:
      cloth every class, leather 17407, chain and scale 879, plate and banded 525; style 0, 6 or 8 none), but a piece
      whose look tools/fix_style0_looks.py chose by its AC (its log says so) the next lighter material's wearers
      (--ac-armour: lighter, material, every or none); a robe the priests and casters; a shield, a held item and a
      weapon by its kind (RULE).
A record whose race mask is 0 (CLIENT refuses every race) takes every race, 1023, when it is given a class mask. The
other race masks are left as they are, also where the class mask's source names fewer races (reported).

Measured on that copy, one a name and slot (1,025 Allakhazam pages read, 688 of them fetched for it):
- Allakhazam's pages against Sandstorm's records of the same items: 58 of 78 equal, 7 differ by the alchemist's bit
  alone, 6 by the warrior's, 7 otherwise. Against this database's masks: 158 of 281 equal, 110 by the warrior's bit
  alone (weapons whose masks here lack it), 5 by the alchemist's, 8 otherwise. Sandstorm's against this database's:
  296 of 352 equal, 49 otherwise (most of them every class here and a class group there).
- Held out: the 688 records with no mask that sources 1-3 give one, which the rule never looks at. The set names
  126 of the 146 it guesses exactly and leaves out a class the record allows for 11. Armour by the rule: 72 of 442
  exact, 23 narrower, the rest wider (one no guess); of the 347 whose look was chosen by AC, their material's wearers
  would leave out a class for 88 (62 exact), the next lighter material's for 15 (40 exact), every class for none (25
  exact). Jewellery, every class: 124 of 232 exact, never narrower. On Sandstorm's own records (their styles not
  chosen by AC) the rule's armour is exact for 92 of 127 and narrower for one; the set for 95 of the 97 it guesses.
- The balance simulator's kits move (more is wearable): of the real-database checks (balancesim_real_test.go), the
  unfixed copy fails 2 cells, the fixed one 7 (a warrior of 20 and paladins of 20, 40 and 55 lose yellows; a
  druid's and a shaman's reds at 40 and a magician's pet's at 30 are won alone), one with --no-rule 6. No balance row
  changes.

The report counts each source and its picks by slot and by where the game hands them out, prints a sample of each
with its evidence (--list every one, --tsv FILE all of it), says what is left, the masks before and after, the held
copies a new mask refuses to their holder's class, and every measure above (each source against this database's
masks and the others, one a name and slot; the rule against what sources 1-3 give the records with no mask, which
it never looks at, under every --ac-armour, and against Sandstorm's records and this database's, crafted families,
a description 8 records share, left out).

--apply writes it in one transaction and logs every change in item_classmask_fix_log (the record, the column, before
and after, the source, its evidence, when); --undo puts back each value still as this left it, then drops the log.
Report only by default. No restart: the server reads an item record at each use, so a new mask holds at once; a
client reads item records at its login, so a player sees it at the next one.

The harvests and the pages are the operator's own (nothing of them is in this repository), so a server's box
has neither: --save-plan FILE here, on a copy of its database, then --plan FILE --apply there (each change made
only where the record is still as planned; one a GM has edited since is left).

    tools/fix_item_classmasks.py [--db eqoa.db] --harvests <harvest JSONs or folders> ... --pages <folder>
                                 [--no-rule] [--ac-armour lighter|material|every|none] [--list] [--tsv FILE]
                                 [--save-plan FILE] [--apply]
    tools/fix_item_classmasks.py --db eqoa.db --plan FILE [--apply]
    tools/fix_item_classmasks.py --db eqoa.db --undo
"""
import argparse
import collections
import csv
import glob
import json
import os
import re
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import fetch_item_pages  # noqa: E402
from fix_dump_weapons import key  # noqa: E402

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
LOG = "item_classmask_fix_log"
LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq       INTEGER PRIMARY KEY AUTOINCREMENT,
    patternid INTEGER NOT NULL,
    col       TEXT NOT NULL,      -- classuse | raceuse
    before    INTEGER NOT NULL,
    after     INTEGER NOT NULL,
    source    TEXT NOT NULL,      -- sandstorm | allakhazam | same item | rule | race
    evidence  TEXT NOT NULL,
    at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
)"""

ABBR = ["WAR", "RAN", "PAL", "SK", "MNK", "BRD", "RGE", "DRD", "SHA", "CL", "MAG", "NEC", "ENC", "WIZ", "ALC"]
CLASS_WORDS = {"warrior": 0, "ranger": 1, "paladin": 2, "shadowknight": 3, "shadow knight": 3, "monk": 4, "bard": 5,
               "rogue": 6, "druid": 7, "shaman": 8, "cleric": 9, "magician": 10, "necromancer": 11, "enchanter": 12,
               "wizard": 13, "alchemist": 14}
ALL = (1 << 15) - 1
EVERY_RACE = 1023                # the ten races SOE shipped (handlers/character.go allShippedRacesMask)
BEFORE_ALC = 1023                # the ten classes before the alchemist: also the leather wearers
LEATHER_NOW = 17407              # the leather wearers with the alchemist, as the live game's leather is
ARMOUR = (1, 5, 7, 10, 11, 19)   # head, chest, bracers, legs, feet, hands
JEWELLERY = (3, 4, 6, 8, 9)      # earring, neck, bracelet, ring, belt
SLOT_NAMES = {1: "head", 2: "robe", 3: "ear", 4: "neck", 5: "chest", 6: "bracelet", 7: "bracers", 8: "ring",
              9: "belt", 10: "legs", 11: "feet", 12: "primary", 13: "shield", 14: "either hand", 15: "two hands",
              16: "ranged", 17: "thrown", 18: "held", 19: "hands"}
# Allakhazam's slot words, as this server numbers its wear slots
PAGE_SLOTS = {"Head": {1}, "Robe": {2}, "Ear": {3}, "Neck": {4}, "Chest": {5}, "Torso": {5}, "Arms": {6, 7},
              "Finger": {8}, "Waist": {9}, "Legs": {10}, "Feet": {11}, "Held": {18}, "Hands": {19},
              "Primary": {12, 14, 15, 16, 17}, "Secondary": {13, 14}}

# 4. The rule (OURS). An armour piece's material is its style (itempattern.model; handlers/character.go
# wornLook draws it): the classes that wear the material, as Sandstorm's records of styled armour carry them.
# (fix_style0_looks.py's tiers; Sandstorm's and Allakhazam's records of styles 6 and 8 name no one material's)
MATERIAL = {1: ("cloth", ALL), 2: ("leather", LEATHER_NOW), 3: ("chain", 879), 7: ("scale", 879), 4: ("plate", 525),
            5: ("banded", 525)}
# a piece whose look tools/fix_style0_looks.py chose by its AC (its log says "its AC"): an AC names the tier 63% of
# the time and misses by a tier, so by default the next lighter material's wearers (AC_ARMOUR, --ac-armour)
LIGHTER = {525: ("chain", 879), 879: ("leather", LEATHER_NOW), LEATHER_NOW: ("cloth", ALL), ALL: ("cloth", ALL)}
AC_ARMOUR = ("lighter", "material", "every", "none")
# the rest by kind: (wear slots, attack types or None, mask, what); a kind's commonest mask in Sandstorm's records
# and this database's, or where those differ the one that leaves out fewest of the classes they name (a
# piercer's every class but the monk, where the commonest left out the casters of half of them; a robe's every
# priest and caster, where the casters' alone left out half)
RULE = [(JEWELLERY, None, ALL, "jewellery and belts"),
        ((2,), None, 32640, "a robe"),
        ((13,), None, 1007, "a shield"),
        ((18,), None, ALL, "a held item"),
        ((12, 14), (1,), 111, "a one-handed slashing weapon"),
        ((12, 14), (3,), 1023, "a one-handed blunt weapon"),
        ((12, 14), (5,), 32751, "a one-handed piercing weapon"),
        ((15,), (2, 6), 15, "a two-handed slashing or piercing weapon"),
        ((15,), (4,), ALL, "a two-handed blunt weapon"),
        ((16, 17), (7, 9), 111, "a bow or crossbow")]
CRAFTED_SHARE = 8                # a description this many records share is a made family's (the crafted sets)
# an armour set's name is a piece's name without the piece's own noun ("Cavernous Boots", "Gloves of the Howler")
PIECES = ("helm|helmet|coif|cap|circlet|crown|headband|turban|hat|cowl|hood|skullcap|coronet|mask|barbute|headwrap|"
          "headgear|headpiece|galea|tunic|breastplate|chestguard|chestplate|jerkin|vest|mail|coat|garb|hauberk|gi|"
          "shirt|doublet|scalemail|chainmail|tabard|armor|armour|bracers|bracer|wraps|wristguards|wristguard|"
          "wristwraps|armwraps|vambraces|armguards|armbands|sleeves|leggings|greaves|trousers|pants|pantaloons|"
          "legplates|legwraps|breeches|kilt|skirt|legguards|boots|boot|slippers|sandals|shoes|moccasins|footwraps|"
          "booties|footpads|gloves|glove|gauntlets|gauntlet|guantlets|handwraps|mittens|mitts")
SET_LEVELS = 3                   # a set's pieces this many levels apart at most
SOURCES = ("sandstorm", "allakhazam", "same item", "set", "rule")


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def mask_names(mask):
    if mask & ALL == ALL:
        return "every class"
    return "/".join(ABBR[c] for c in range(15) if mask >> c & 1) or "no class"


def base_key(name):
    """A name as key reads it, a trailing "(copy)" aside (a GM's copy of a record)."""
    return re.sub(r"\s*\(copy\)$", "", key(name))


def gm_patterns(db):
    out = set()
    if not has_table(db, "gm_audit"):
        return out
    for (k,) in db.execute("SELECT rowkey FROM gm_audit WHERE tbl = 'itempattern'"):
        try:
            out.add(int(json.loads(k)["patternid"]))
        except (ValueError, KeyError, TypeError):
            pass
    return out


def classes_mask(text):
    """The mask an Allakhazam "Classes" line says: "All Classes", or class names with commas; None for an
    empty line or one naming anything else."""
    t = " ".join((text or "").split()).strip(" ,")
    if not t:
        return None
    if t.lower() in ("all classes", "all"):
        return ALL
    mask = 0
    for w in t.split(","):
        w = w.strip().lower()
        if not w:
            continue
        if w not in CLASS_WORDS:
            return None
        mask |= 1 << CLASS_WORDS[w]
    return mask or None


def set_key(name):
    """The set an armour piece's name says it is of: its name without the piece's noun, or None; a possessive
    or a plural s of its last word aside ("Wind Golem's helm", "Wind Golems Gloves", "Wind Golem Boots")."""
    k = key(name).replace("'s ", " ").replace("'", "")
    m = re.fullmatch(rf"(?:{PIECES})\s+(of\s+.+)", k) or re.fullmatch(rf"(.+?)\s+(?:{PIECES})", k)
    if not m:
        return None
    return re.sub(r"(?<=\S\S[^\ssiu])s$", "", m.group(1))


def set_index(ss, ak):
    """{set: [(name, wear slot, level, mask, whose)]}: every armour piece Sandstorm's records or Allakhazam's
    pages give a mask, by the set its name says (Sandstorm's record of a name and slot before a page)."""
    out = collections.defaultdict(list)
    seen = set()
    for (k, slot), cands in sorted(ss.items()):
        masks = {c[1] & ALL for c in cands}
        sk = set_key(k)
        if slot in ARMOUR and sk and len(masks) == 1 and masks != {0}:
            out[sk].append((k, slot, cands[0][0], masks.pop(), "Sandstorm's record"))
            seen.add((k, slot))
    for k, pages in sorted(ak.items()):
        sk = set_key(k)
        for p in pages:
            slots = PAGE_SLOTS.get(p["slot"], set()) & set(ARMOUR)
            if not sk or len(slots) != 1 or p["mask"] is None or (k, min(slots)) in seen:
                continue
            m = LEATHER_NOW if p["mask"] == BEFORE_ALC else p["mask"]
            out[sk].append((k, min(slots), p["level"], m, f"Allakhazam item {p['id']}"))
    return out


def sandstorm(paths):
    """Sandstorm's gear records: {(name, wear slot): [record]}, each record (level, class mask, race mask, id)
    once."""
    files = []
    for p in paths:
        files += sorted(glob.glob(os.path.join(p, "**", "*.json"), recursive=True)) if os.path.isdir(p) else [p]
    out = collections.defaultdict(set)

    def walk(o):
        if isinstance(o, dict):
            if o.get("name") and isinstance(o.get("wear_slot"), int) and isinstance(o.get("class_mask"), int):
                yield o
            for v in o.values():
                yield from walk(v)
        elif isinstance(o, list):
            for v in o:
                yield from walk(v)

    for f in files:
        try:
            with open(f) as fh:
                d = json.load(fh)
        except (OSError, ValueError):
            continue
        for w in walk(d):
            if w["wear_slot"] > 0:
                out[(key(w["name"]), w["wear_slot"])].add(
                    (w.get("level_req"), w["class_mask"], w.get("race_mask"), w.get("id")))
    return {k: sorted(v, key=lambda r: tuple(-1 if x is None else x for x in r)) for k, v in out.items()}


def page_classes(text):
    """A page's "Classes" line when read_page missed it (the first field after the item's name, which its
    pattern drops): the line up to "Races"."""
    m = re.search(r"\|\s*Classes\s*\|\s*:(.*?)\|\s*Races\s*\|", fetch_item_pages.text_of(text))
    if not m:
        return None
    return re.sub(r"\s*,\s*", ", ", " ".join(m.group(1).replace("|", " ").split()).strip(" ,"))


def allakhazam(folder):
    """Allakhazam's item pages in the folder: {name: [page]}, a page {id, slot, level, classes, mask}."""
    out = collections.defaultdict(list)
    if not folder:
        return out
    for p in sorted(glob.glob(os.path.join(folder, "pages", "*.html"))):
        iid = os.path.basename(p)[:-len(".html")]
        if not iid.isdigit():
            continue
        text = fetch_item_pages.read_file(p)
        rec = fetch_item_pages.read_page(text)
        if not rec or not rec.get("name"):
            continue
        classes = rec.get("Classes")
        if classes is None:
            classes = page_classes(text)
        lv = rec.get("Item Level", "")
        out[key(rec["name"])].append(dict(id=int(iid), slot=(rec.get("Slot") or "").strip(),
                                          level=int(lv) if str(lv).strip().isdigit() else None,
                                          classes=classes or "", mask=classes_mask(classes),
                                          races=(rec.get("Races") or "").strip()))
    return out


def pick(cands, level, mask_of):
    """The one mask candidates give: all alike, else those of the level all alike; (mask, the ones it came
    from) or (None, why)."""
    if not cands:
        return None, ""
    masks = {mask_of(c) for c in cands}
    if len(masks) == 1:
        return masks.pop(), cands
    at = [c for c in cands if c_level(c) == level]
    masks = {mask_of(c) for c in at}
    if len(masks) == 1:
        return masks.pop(), at
    return None, f"its {len(cands)} records differ ({', '.join(str(m) for m in sorted({mask_of(c) for c in cands}))})"


def c_level(c):
    return c["level"] if isinstance(c, dict) else c[0]


def from_sandstorm(rec, ss):
    cands = ss.get((key(rec["name"]), rec["slot"]), [])
    mask, got = pick(cands, rec["level"], lambda r: r[1])
    if mask is None or mask & ALL == 0:
        return None, got if mask is None else "its record's mask is 0"
    lv, _m, race, sid = got[0]
    same = " (the same id)" if sid == rec["pid"] else f" (#{sid})"
    return mask, f"Sandstorm's record{same}, level {lv}: {mask} {mask_names(mask)}"


def from_allakhazam(rec, ak):
    pages = [p for p in ak.get(key(rec["name"]), []) if rec["slot"] in PAGE_SLOTS.get(p["slot"], ())]
    pages = [p for p in pages if p["mask"] is not None]
    mask, got = pick(pages, rec["level"], lambda p: p["mask"])
    if mask is None:
        return None, got
    p = got[0]
    ev = f"Allakhazam item {p['id']} ({p['slot']}, level {p['level']}): {p['classes']!r}"
    if mask == BEFORE_ALC and rec["slot"] in ARMOUR:
        return LEATHER_NOW, ev + " (the leather wearers before the alchemist): 17407 with it"
    return mask, ev


def from_same(rec, same):
    got = same.get((base_key(rec["name"]), rec["slot"]), [])
    masks = {m for _pid, _lv, m in got}
    if len(masks) == 1:
        m = masks.pop()
        pid, lv, _ = got[0]
        return m, f"record {pid} of the same name and slot (level {lv}): {m} {mask_names(m)}"
    if masks:
        return None, f"its records here differ ({', '.join(map(str, sorted(masks)))})"
    return None, ""


def from_set(rec, sets):
    """4a. An armour piece's set: the other pieces of the set its name says, within SET_LEVELS of its level,
    that Sandstorm's records or Allakhazam's pages give a mask, all one."""
    if rec["slot"] not in ARMOUR:
        return None, ""
    sk = set_key(rec["name"])
    if not sk:
        return None, ""
    k = key(rec["name"])
    mates = [e for e in sets.get(sk, []) if e[0] != k and
             (e[2] is None or rec["level"] is None or abs(e[2] - rec["level"]) <= SET_LEVELS)]
    masks = {e[3] for e in mates}
    if len(masks) > 1:
        return None, f"the {sk!r} pieces differ ({', '.join(map(str, sorted(masks)))})"
    if not masks:
        return None, ""
    m = masks.pop()
    named = "; ".join(f"{e[0]} ({e[4]}, level {e[2]})" for e in mates[:3]) + (" ..." if len(mates) > 3 else "")
    return m, f"the {sk!r} set's other pieces, {named}: {m} {mask_names(m)}"


def from_rule(rec, ac_armour="lighter"):
    """4b. By slot and kind: jewellery every class, armour by its material, the rest by RULE. A piece whose
    look its AC chose (rec["by_ac"]) as ac_armour says: the next lighter material's wearers, its material's,
    every class, or none."""
    slot, model, at = rec["slot"], rec["model"], rec["attacktype"]
    if slot in ARMOUR:
        if model not in MATERIAL:
            return None, f"armour of style {model}: no one material"
        what, mask = MATERIAL[model]
        if not rec.get("by_ac") or ac_armour == "material" or (mask == ALL and ac_armour == "lighter"):
            how = " (its look by its AC)" if rec.get("by_ac") else ""
            return mask, f"{what} armour (style {model}){how}: {mask_names(mask)}"
        if ac_armour == "none":
            return None, f"{what} armour by its AC (style {model}): left (--ac-armour none)"
        if ac_armour == "every":
            return ALL, f"{what} armour by its AC (style {model}): every class (--ac-armour every)"
        light, mask = LIGHTER[mask]
        return mask, f"{what} armour by its AC (style {model}): the next lighter, {light}: {mask_names(mask)}"
    for slots, ats, mask, what in RULE:
        if slot in slots and (ats is None or at in ats):
            return mask, f"{what}: {mask_names(mask)}"
    return None, f"no rule for wear slot {slot}, attack type {at}"


def by_ac(db):
    """The armour pieces whose look tools/fix_style0_looks.py chose by their AC and that still wear it."""
    if not has_table(db, "style0_look_fix_log"):
        return set()
    return {pid for pid, in db.execute(
        "SELECT l.patternid FROM style0_look_fix_log l JOIN itempattern p ON p.patternid = l.patternid "
        "WHERE l.source LIKE 'its AC (%' AND p.model = l.new_model")}


def records(db):
    cols = ["patternid", "itemname", "equipslot", "levelreq", "classuse", "raceuse", "model", "attacktype", "itemdesc"]
    ac = by_ac(db)
    out = []
    for r in db.execute(f"SELECT {', '.join(cols)} FROM itempattern WHERE equipslot > 0 ORDER BY patternid"):
        d = dict(zip(cols, r))
        out.append(dict(pid=d["patternid"], name=d["itemname"], slot=d["equipslot"], level=d["levelreq"],
                        mask=d["classuse"], race=d["raceuse"], model=d["model"], attacktype=d["attacktype"],
                        desc=d["itemdesc"], by_ac=d["patternid"] in ac))
    return out


def plan(db, ss, ak, use_rule=True, ac_armour="lighter"):
    """(changes, left, measured): changes [{pid, name, slot, level, col, before, after, source, evidence}],
    left [{pid, name, slot, level, why}], and the sources measured against each other."""
    recs = records(db)
    gm = gm_patterns(db)
    same = collections.defaultdict(list)
    for r in recs:
        if r["mask"]:
            same[(base_key(r["name"]), r["slot"])].append((r["pid"], r["level"], r["mask"]))
    sets = set_index(ss, ak)
    changes, left = [], []
    for r in recs:
        if r["mask"] != 0:
            continue
        item = dict(pid=r["pid"], name=r["name"], slot=r["slot"], level=r["level"])
        if r["pid"] in gm:
            left.append(dict(item, why="a GM changed it"))
            continue
        why = []
        new = None
        sources = [("sandstorm", lambda: from_sandstorm(r, ss)), ("allakhazam", lambda: from_allakhazam(r, ak)),
                   ("same item", lambda: from_same(r, same)), ("set", lambda: from_set(r, sets))]
        if use_rule:
            sources.append(("rule", lambda: from_rule(r, ac_armour)))
        for source, fn in sources:
            m, ev = fn()
            if m is not None:
                new = (source, m, ev)
                break
            if ev:
                why.append(f"{source}: {ev}")
        if new is None:
            left.append(dict(item, why="; ".join(why) or "nothing names its classes"))
            continue
        source, m, ev = new
        changes.append(dict(item, col="classuse", before=0, after=m, source=source, evidence=ev))
        if r["race"] == 0:
            changes.append(dict(item, col="raceuse", before=0, after=EVERY_RACE, source="race",
                                evidence="its race mask was 0, which CLIENT's CanUse refuses every race: every race"))
    measured = measure(recs, ss, ak, same, sets, ac_armour)
    for what, c in check_held_out(changes, {r["pid"]: r for r in recs}, sets, ac_armour).items():
        measured[f"held out: {what} against sources 1-3 on the records with no mask"] = c
    return changes, left, measured


def compare(have, got):
    """How a mask another source gives compares with one a record has: equal, the warrior's bit, the
    alchemist's, or other."""
    have, got = have & ALL, got & ALL
    if have == got:
        return "equal"
    if have ^ got == 1:
        return "differ by the warrior's bit"
    if have ^ got == 1 << 14:
        return "differ by the alchemist's bit"
    return "differ otherwise"


def judge(truth, guess):
    """The rule's guess against a mask: exact, wider (every class the mask allows, and more), or narrower
    (refuses a class the mask allows)."""
    truth, guess = truth & ALL, guess & ALL
    if truth == guess:
        return "exact"
    if truth & ~guess == 0:
        return "wider"
    return "refuses a class it allows"


def group_of(slot):
    return "armour" if slot in ARMOUR else "jewellery" if slot in JEWELLERY else "other"


def guesses(r, sets, ac_armour, variants=False):
    """What 4a and 4b would give a record, and every class beside them, for the measures; with variants, a
    piece whose look its AC chose under every --ac-armour too."""
    out = {"the rule": from_rule(r, ac_armour)[0], "every class": ALL}
    if r["slot"] in ARMOUR:
        out["the set"] = from_set(r, sets)[0]
        if variants and r.get("by_ac"):
            for v in AC_ARMOUR:
                out[f"the rule, its look by its AC, --ac-armour {v}"] = from_rule(r, v)[0]
    return out


def measure(recs, ss, ak, same, sets, ac_armour):
    """Each source against the masks it can be checked on, one a name and slot: Sandstorm's records and
    Allakhazam's pages against this database's masks and each other, and the set and the rule against this
    database's masks (records of ours, made families left out) and Sandstorm's."""
    out = collections.defaultdict(collections.Counter)
    descs = collections.Counter(r["desc"] for r in recs)
    seen = set()
    for r in recs:
        k = (key(r["name"]), r["slot"])
        if k in seen:
            continue
        seen.add(k)
        s, _ = from_sandstorm(r, ss)
        a, _ = from_allakhazam(r, ak)
        if s is not None and a is not None:
            out["Allakhazam's page against Sandstorm's record"][compare(s, a)] += 1
        if s is not None:
            for what, g in guesses(r, sets, ac_armour).items():
                out[f"{what} against Sandstorm's records ({group_of(r['slot'])})"][
                    "no guess" if g is None else judge(s, g)] += 1
        if not r["mask"] or r["mask"] & ALL == 0:
            continue
        if s is not None:
            out["Sandstorm's record against this database's"][compare(r["mask"], s)] += 1
        if a is not None:
            out["Allakhazam's page against this database's"][compare(r["mask"], a)] += 1
        if r["pid"] < 700000 and descs[r["desc"]] < CRAFTED_SHARE:
            for what, g in guesses(r, sets, ac_armour).items():
                out[f"{what} against this database's masks ({group_of(r['slot'])})"][
                    "no guess" if g is None else judge(r["mask"], g)] += 1
    return out


def check_held_out(changes, recs_by_pid, sets, ac_armour):
    """The set, the rule and every class against what sources 1-3 gave the records with no mask: none of
    them looks at those masks, so these are the measures that count."""
    out = collections.defaultdict(collections.Counter)
    for c in changes:
        if c["col"] != "classuse" or c["source"] not in ("sandstorm", "allakhazam", "same item"):
            continue
        r = recs_by_pid[c["pid"]]
        for what, g in guesses(r, sets, ac_armour, variants=True).items():
            out[f"{what} ({group_of(r['slot'])})"]["no guess" if g is None else judge(c["after"], g)] += 1
    return out


def held_refused(db, changes):
    """The held copies whose new mask leaves out their holder's class: [(pid, name, serverid, class, worn,
    the account's level or None)]."""
    new = {c["pid"]: c["after"] for c in changes if c["col"] == "classuse"}
    if not new or not has_table(db, "charinventory") or not has_table(db, "characters"):
        return []
    level = "a.acctlevel" if has_table(db, "accountinfo") else "NULL"
    join = "LEFT JOIN accountinfo a ON a.accountid = c.accountid " if has_table(db, "accountinfo") else ""
    marks = ",".join("?" * len(new))
    out = []
    for pid, name, sid, cls, loc, lv in db.execute(
            f"SELECT i.patternid, p.itemname, i.serverid, c.tclass, i.equiploc, {level} FROM charinventory i "
            f"JOIN characters c ON c.serverid = i.serverid JOIN itempattern p ON p.patternid = i.patternid {join}"
            f"WHERE i.patternid IN ({marks}) ORDER BY i.patternid, i.serverid", list(new)):
        if isinstance(cls, int) and 0 <= cls < 15 and not new[pid] >> cls & 1:
            out.append((pid, name, sid, ABBR[cls], loc != -1, lv))
    return out


def uses(db):
    """{patternid: [what]}: where the game hands an item out (loot tables, a corpse's or a merchant's npcitem,
    a quest step's rewards, a character's inventory)."""
    out = collections.defaultdict(set)
    for what, table, sql in (("a loot table", "loottables", "SELECT patternid FROM loottables"),
                             ("a loot pool", "lootpool", "SELECT patternid FROM lootpool"),
                             ("a merchant's or a corpse's", "npcitem", "SELECT patternid FROM npcitem"),
                             ("a character's", "charinventory", "SELECT patternid FROM charinventory"),
                             ("the start items", "startitems", "SELECT patternid FROM startitems")):
        if has_table(db, table):
            for (pid,) in db.execute(sql):
                out[pid].add(what)
    if has_table(db, "queststeps"):
        def ids(v):
            return [int(m.group(1)) for m in re.finditer(r"(?<![\d*])(\d+)(?:\*\d+)?", str(v or ""))]
        for rewards, *rest in db.execute("SELECT rewarditems, byclass, choices, events FROM queststeps"):
            for pid in ids(rewards) + [p for v in rest for m in re.finditer(r'"items"\s*:\s*"([^"]*)"', v or "")
                                       for p in ids(m.group(1))]:
                out[pid].add("a quest's reward")
    return out


def apply(db, changes):
    db.execute(LOG_DDL)
    done = 0
    for c in changes:
        cur = db.execute(f"UPDATE itempattern SET {c['col']} = ? WHERE patternid = ? AND {c['col']} = ?",
                         (c["after"], c["pid"], c["before"]))
        if cur.rowcount:
            db.execute(f"INSERT INTO {LOG} (patternid, col, before, after, source, evidence) VALUES (?, ?, ?, ?, ?, ?)",
                       (c["pid"], c["col"], c["before"], c["after"], c["source"], c["evidence"]))
            done += 1
    return done


def undo(db):
    if not has_table(db, LOG):
        print("nothing logged: nothing to undo")
        return
    back = kept = 0
    rows = db.execute(f"SELECT patternid, col, before, after FROM {LOG} ORDER BY seq DESC").fetchall()
    for pid, col, before, after in rows:
        if col not in ("classuse", "raceuse"):
            raise SystemExit(f"{LOG}: unknown column {col!r}")
        cur = db.execute(f"UPDATE itempattern SET {col} = ? WHERE patternid = ? AND {col} = ?", (before, pid, after))
        back, kept = (back + 1, kept) if cur.rowcount else (back, kept + 1)
    db.execute(f"DROP TABLE {LOG}")
    print(f"put back {back}; left {kept} changed since")


def report(db, changes, left, measured, a):
    recs = {r["pid"]: r for r in records(db)}
    use = uses(db)
    mask_changes = [c for c in changes if c["col"] == "classuse"]
    zero = len(mask_changes) + len(left)
    print(f"{zero} wearable records with no class mask: {len(mask_changes)} given one, {len(left)} left")
    by = collections.Counter(c["source"] for c in mask_changes)
    print("  by source: " + ", ".join(f"{s} {by[s]}" for s in SOURCES if by[s]))
    slots = collections.defaultdict(collections.Counter)
    for c in mask_changes:
        slots[c["slot"]][c["source"]] += 1
    for c in left:
        slots[c["slot"]]["left"] += 1
    for s in sorted(slots):
        print(f"  slot {s:2} {SLOT_NAMES.get(s, '?'):11} " + ", ".join(f"{k} {v}" for k, v in slots[s].most_common()))
    in_use = collections.Counter()
    for c in mask_changes:
        for u in use.get(c["pid"], ()):
            in_use[(u, c["source"])] += 1
    for c in left:
        for u in use.get(c["pid"], ()):
            in_use[(u, "left")] += 1
    for u in ("a loot table", "a loot pool", "a merchant's or a corpse's", "a quest's reward", "a character's",
              "the start items"):
        row = {s: in_use[(u, s)] for s in SOURCES + ("left",) if in_use[(u, s)]}
        if row:
            print(f"  in {u}: " + ", ".join(f"{k} {v}" for k, v in row.items()))
    sample = collections.defaultdict(list)
    for c in changes:
        sample[c["source"]].append(c)
    for s in SOURCES + ("race",):
        rows = sample.get(s, [])
        if not rows:
            continue
        print(f"{s}: {len(rows)}" + ("" if a.list or len(rows) <= 12 else ", a sample of 12 (--list: every one)"))
        step = 1 if a.list or len(rows) <= 12 else len(rows) / 12
        for i in range(len(rows) if a.list or len(rows) <= 12 else 12):
            c = rows[int(i * step)]
            print(f"  {c['pid']} {c['name']!r} (slot {c['slot']}, level {c['level']}): {c['col']} {c['before']} -> "
                  f"{c['after']} | {c['evidence']}")
    whys = collections.Counter()
    for x in left:
        whys["a GM changed it" if x["why"] == "a GM changed it" else x["why"].split(": ")[0] + ": " +
             re.sub(r"\d+", "N", x["why"].split("; ")[-1].split(": ", 1)[-1])[:70]] += 1
    if left:
        print(f"left: {len(left)}")
        for w, n in whys.most_common(8):
            print(f"  {n} {w}")
        for x in left[:20] if not a.list else left:
            print(f"  {x['pid']} {x['name']!r} (slot {x['slot']}, level {x['level']}): {x['why']}")
    print("each source measured, one a name:")
    for what in sorted(measured):
        c = measured[what]
        print(f"  {what}: " + ", ".join(f"{k} {v}" for k, v in c.most_common()) + f" (of {sum(c.values())})")
    refused = held_refused(db, changes)
    if refused:
        gms = sum(1 for *_x, lv in refused if lv)
        print(f"held copies the new masks refuse to their holder's class (kept, not put on afresh): {len(refused)}, "
              f"{gms} of them a GM's character (acctlevel 1 or more)")
        for pid, name, sid, cls, worn, lv in refused[:20] if not a.list else refused:
            print(f"  {pid} {name!r}: character {sid}, {cls}{', worn' if worn else ''}{', a GM' if lv else ''}")
    narrower = collections.Counter()
    for c in mask_changes:
        r = recs[c["pid"]]
        if c["source"] == "sandstorm":
            got = a.ss.get((key(r["name"]), r["slot"]), [])
            if got and all(x[2] is not None and x[2] & EVERY_RACE != EVERY_RACE for x in got):
                narrower["Sandstorm's record"] += 1
        elif c["source"] == "allakhazam":
            pages = [p for p in a.ak.get(key(r["name"]), []) if r["slot"] in PAGE_SLOTS.get(p["slot"], ())]
            if pages and all(p["races"] and p["races"].lower() != "all races" for p in pages):
                narrower["Allakhazam's page"] += 1
    if narrower:
        print("race masks left as they are (every race) where the class mask's source names fewer races: " +
              ", ".join(f"{k} {v}" for k, v in narrower.items()))
    races = db.execute("SELECT count(*), sum(raceuse = 0), sum(raceuse & 1024 = 0), sum(raceuse & 1023 = 1023 AND "
                       "raceuse & 1024 = 0), sum(raceuse <> 0 AND raceuse & 1023 <> 1023 AND raceuse & 1024 = 0) "
                       "FROM itempattern WHERE equipslot > 0").fetchone()
    print(f"race masks of the {races[0]} wearable records: {races[1]} are 0 (no race can use one: given every race "
          f"above when its class mask is), {races[2]} lack the froglok's bit (1024): {races[3]} of them name all ten "
          f"shipped races, which the wire widens to the froglok (raceUseForWire), {races[4]} leave out a shipped race "
          f"and stay closed to it")
    after = collections.Counter(dict(db.execute("SELECT classuse, count(*) FROM itempattern WHERE equipslot > 0 "
                                                "GROUP BY classuse").fetchall()))
    before = after.copy()
    for c in mask_changes:
        after[0] -= 1
        after[c["after"]] += 1
    print("the masks of wearable records, before: " + ", ".join(f"{m} {n}" for m, n in before.most_common(12)))
    print("                                after: " + ", ".join(f"{m} {n}" for m, n in (+after).most_common(12)))
    if a.tsv:
        with open(a.tsv, "w", newline="") as out:
            w = csv.writer(out, delimiter="\t", lineterminator="\n")
            w.writerow(["patternid", "item", "slot", "level", "column", "before", "after", "classes", "source",
                        "evidence", "uses"])
            for c in changes:
                w.writerow([c["pid"], c["name"], c["slot"], c["level"], c["col"], c["before"], c["after"],
                            mask_names(c["after"]) if c["col"] == "classuse" else "", c["source"], c["evidence"],
                            ", ".join(sorted(use.get(c["pid"], ())))])
            for x in left:
                w.writerow([x["pid"], x["name"], x["slot"], x["level"], "classuse", 0, "", "", "left", x["why"],
                            ", ".join(sorted(use.get(x["pid"], ())))])


def load_plan(db, path):
    """A plan --save-plan wrote, as changes, each still as planned: its record worn, its value as it was then,
    and no GM's change of the record since (a GM's edit is left)."""
    with open(path) as fh:
        planned = json.load(fh)["changes"]
    gm = gm_patterns(db)
    now = {pid: (slot, cu, ru) for pid, slot, cu, ru in
           db.execute("SELECT patternid, equipslot, classuse, raceuse FROM itempattern")}
    changes, stale = [], []
    for c in planned:
        if c["col"] not in ("classuse", "raceuse"):
            raise SystemExit(f"{path}: unknown column {c['col']!r}")
        slot, cu, ru = now.get(c["pid"], (None, None, None))
        here = cu if c["col"] == "classuse" else ru
        if c["pid"] in gm or slot is None or slot <= 0 or here != c["before"]:
            stale.append(c)
        else:
            changes.append(c)
    return changes, stale


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--harvests", "--prices", nargs="*", default=[],
                    help="eqoa harvest JSON files or folders of them (Sandstorm's records)")
    ap.add_argument("--pages", help="tools/fetch_item_pages.py's folder (Allakhazam's item pages)")
    ap.add_argument("--no-rule", action="store_true",
                    help="leave at 0 what no record of the item or of its set gives a mask (the rule's 4b)")
    ap.add_argument("--ac-armour", choices=AC_ARMOUR, default="lighter",
                    help="an armour piece whose look its AC chose: the next lighter material's wearers (default), its "
                         "material's, every class, or none")
    ap.add_argument("--list", action="store_true", help="print every change and every record left")
    ap.add_argument("--tsv", help="write every change and every record left here")
    ap.add_argument("--save-plan",
                    help="write the changes here, for --plan on a machine without the harvests and pages")
    ap.add_argument("--plan", help="the changes --save-plan wrote, in place of planning (each only if still as then)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the masks (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply wrote")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"--db {a.db}: no such database")
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 10000")
    if a.undo:
        db.execute("BEGIN IMMEDIATE")
        try:
            undo(db)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        return
    if a.plan:
        changes, stale = load_plan(db, a.plan)
        by = collections.Counter((c["col"], c["source"]) for c in changes)
        print(f"{a.plan}: {len(changes)} changes still as planned (" + ", ".join(
            f"{col} by {src} {n}" for (col, src), n in sorted(by.items())) + f"), {len(stale)} left: changed since "
              "or a GM's")
        for c in stale[:20]:
            print(f"  left: {c['pid']} {c['name']!r} {c['col']}")
    else:
        if not a.harvests or not a.pages:
            sys.exit("--harvests and --pages: Sandstorm's records and Allakhazam's pages are what the masks are taken "
                     "from first (--plan applies a plan made with them)")
        if not os.path.isdir(os.path.join(a.pages, "pages")):
            sys.exit(f"--pages {a.pages}: no pages/ folder in it (tools/fetch_item_pages.py --out makes one)")
        ss = sandstorm(a.harvests)
        ak = allakhazam(a.pages)
        print(f"Sandstorm's gear records: {sum(len(v) for v in ss.values())} under {len(ss)} names and slots; "
              f"Allakhazam's pages: {sum(len(v) for v in ak.values())} under {len(ak)} names")
        changes, left, measured = plan(db, ss, ak, use_rule=not a.no_rule, ac_armour=a.ac_armour)
        a.ss, a.ak = ss, ak
        report(db, changes, left, measured, a)
        if a.save_plan:
            with open(a.save_plan, "w") as fh:
                json.dump({"db": os.path.abspath(a.db), "no_rule": a.no_rule, "ac_armour": a.ac_armour,
                           "changes": changes}, fh, indent=1)
            print(f"plan written to {a.save_plan}: {len(changes)} changes")
    if not a.apply:
        print("report only: --apply to write")
        return
    db.execute("BEGIN IMMEDIATE")
    try:
        done = apply(db, changes)
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"wrote {done} values (logged in {LOG}); no restart: a player sees them at the next login")


if __name__ == "__main__":
    main()
