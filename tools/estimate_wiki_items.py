#!/usr/bin/env python3
"""The items the EQOA wiki names in a loot list alone, made with ESTIMATED figures (the user, 2026-11-02: "add the 58
boss items with estimated stats"). tools/import_wiki_items.py made the wiki's items that come with a level and a slot
and listed these: Cazic Thule's doubt and horror pieces, Lord Nagafen's, Bertoxxulous's, Noroxus's, Queen Cizkik's,
the gnolls', frogloks' and undead's own drops, a few named creatures'. Nothing on record gives their figures, so every
figure here is OURS: each a real item's of ours, its DONOR's. The description a player reads is left empty; the log
says each is an estimate and from what.

For each such name (the import's own matching: an item of ours under its spelling or the wiki's other wording is not
made; the wiki's "Packmaster helmet" is our Packmaster Helm), and each item page that gives no level (Boneduke's Boots,
the Fire Chitin Shield):
- **Its slot** from its name's piece word (a ring 8, an amulet or a charm 4, a circlet 1, legwraps 10, a buckler or a
  guard 13, a trinket 3, a rope a belt), else a weapon's words (a longbow 7, a cleaver or a -render 1, a halberd 2, a
  dagger or a claw 5, a lance 6, a cudgel 3), else a held item (Swampstench, a lantern, a peach); a page's category
  first.
- **Its level**: a page's AC (the level of ours with that AC in its slot), else the creature of ours the wiki says
  drops it (at most 60), else the wiki's range for its creature type, else its siblings' median (items sharing a word
  of its set or clan: "Doglord" for Doglord Longclaw), else the median of its creature type's spawns.
- **Its classes**: the loot list's "(Bard only)", else its page's, else its set's (siblings that drop where it drops),
  else its donor's (a weapon's from a donor of another type: the commonest of ours of its type).
- **Its figures**: its donor's, one item of ours of its slot (a weapon: of its type; a two-hander any two-hander's when
  three of its type are not there) within 3 levels (6, 10, 20), usable by every class it is for, a drop (no merchant
  sells it, no recipe makes it, not NO RENT, none of the wiki's), no freak record (past twice its fellows' median
  power); where three remain, one of its drawn style (an armour piece's AC is its material's: searched first within 6
  levels), one whose own name says its slot or type, an armour piece with AC and a weapon with stats, of few classes
  for a one-class item, of most for an item of none; at the rank between the 60th and 90th percentile of power (a
  weapon's damage first) its name picks, one no other estimate took where the band holds one: boss loot above the
  middle, no two alike. A page's own figures are kept (the Fire Chitin Shield's FR 25 and AC 189).
- **Its look, icon and value** as tools/import_wiki_items.py makes them (style from the classes, the commonest icon of
  its kind, the hardened rule's value); hit points the donor's, durability 50, tradeable.
Where it comes from (the rates OURS): a creature of ours the wiki says drops it, that creature's own table ("wiki
<name>", the import's where it made one): every kill for a creature of level 50 and up (as tools/seed_boss_loot.py's
bosses), a quarter of them below; a creature type's table (gnolls, frogloks, undead) for the wiki's levels or 3 round
its own, 3% a kill, bound to that type's creatures of those levels; else (Champion Skrak and Scorchsting are not in
this world) the rare pool of every region for 5 levels round it. An item with no source of its own takes its siblings'.

Ids follow tools/import_wiki_items.py's in 713000-713999. Logged in wiki_item_estimate_log; --undo deletes every row it
made (a record a character holds is kept, and said) and puts the counters back: undo it before the import (their
creature tables are shared). Report only by default; no restart (the game reads item records and loot tables as it goes).

    tools/estimate_wiki_items.py [--db eqoa.db] --wiki <folder> --prices <harvest JSONs or folders> ... [--list] [--apply | --undo]
"""
import argparse
import collections
import hashlib
import json
import os
import re
import sqlite3
import statistics
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import import_wiki_items as w  # noqa: E402
from fill_item_values import robust_rule  # noqa: E402
from stock_merchants import STAT_COLS, Measured, stat_points  # noqa: E402

LOG = "wiki_item_estimate_log"
BOSS_LEVEL, BOSS_CHANCE = 50, 100
DONOR_SPANS, DONOR_RANKS = (3, 6, 10, 20), (0.6, 0.9)   # each item its own rank in this band, by its name
OUTLIER = 2.0               # a candidate of more than twice the candidates' median power is a freak record, no donor
NARROW, WIDE = 8, 12        # a donor for one class's item names at most 8 classes; for an item of no classes, at least 12
STYLE_SPAN = 6              # an armour piece's donor of its own style first, within this many levels
TYPE_SPAN, POOL_SPAN = 3, 5
MAX_LEVEL = 60
# a name's piece word -> wear slot (the last that names one decides, before any " of ")
SLOT_WORDS = [
    (r"rings?|bands?|loops?", 8), (r"earrings?|trinkets?|hoops?", 3), (r"amulets?|necklaces?|chokers?|collars?|pendants?|medallions?|charms?", 4),
    (r"bracelets?|bangles?|armlets?|cuffs?", 6), (r"bracers?|vambraces?", 7), (r"belts?|sash(?:es)?|girdles?|cords?|ropes?", 9),
    (r"boots?|shoes?|sandals?|slippers?", 11), (r"circlets?|helms?|helmets?|crowns?|caps?|coifs?|hoods?", 1),
    (r"leggings?|legwraps?|legguards?|trousers|pants|greaves|kilts?", 10), (r"tunics?|chestguards?|breastplates?|chestplates?|hauberks?|vests?|robes?|coats?", 5),
    (r"gloves?|gauntlets?", 19), (r"shields?|bucklers?|targes?|guards?", 13), (r"lanterns?|orbs?|totems?|strings|stones?|peach(?:es)?|icons?", 18),
]
CLASS_NAMES = {"warrior": 0, "ranger": 1, "paladin": 2, "shadowknight": 3, "shadow knight": 3, "monk": 4, "bard": 5, "rogue": 6,
               "druid": 7, "shaman": 8, "cleric": 9, "magician": 10, "necromancer": 11, "enchanter": 12, "wizard": 13, "alchemist": 14}
TYPES = {"gnolls": r"\bgnolls?\b", "frogloks": r"\bfrogloks?\b", "undead": None}
COMMON_WORDS = {"of", "the", "a", "an", "ancient", "cracked", "restored", "heavy", "hewn", "shield", "ring", "boots", "gloves",
                "helm", "helmet", "tunic", "leggings", "legguards", "dagger", "bow", "longbow", "crossbow", "necklace", "bracelet",
                "earring", "amulet", "belt", "sash", "lance", "halberd", "cleaver", "longsword", "trinket", "buckler", "guard",
                "chestguard", "circlet", "trousers", "legwraps", "doubt", "horror", "dead", "dragon"}


def slot_of(name):
    """(wear slot, attack type, why) a name says; a held item when it says nothing."""
    k = w.key(name)
    head = k.split(" of ")[0]
    for word in reversed(re.findall(r"[a-z']+", head)):
        for pat, slot in SLOT_WORDS:
            if re.fullmatch(pat, word):
                return slot, 0, f"its word {word!r}"
    hit = w.by_words(name)
    if hit:
        at, two = hit
        slot = 16 if at in (7, 9, 10) else 15 if at in (2, 4, 6) else 14
        return slot, at, "a weapon's words"
    at = w.weapon_inside(name, 14, None)
    if at:
        return 14, at, "a weapon's word inside its name"
    return 18, 0, "its name says no slot: a held item"


def distinctive(k):
    return {x for x in re.findall(r"[a-z]+", k.replace("'s", "")) if len(x) >= 4 and x not in COMMON_WORDS}


class Pool:
    """This server's items as donors and siblings."""

    def __init__(self, db):
        cols = ", ".join(STAT_COLS)
        self.rows = [dict(zip(["id", "name", "slot", "at", "dmg", "level", "classes", "rent", "maxhp", "model", "AC", *STAT_COLS], r))
                     for r in db.execute(f"SELECT patternid, itemname, equipslot, attacktype, weapondamage, levelreq, classuse, rent, "
                                         f"maxhp, model, coalesce(AC, 0), {cols} FROM itempattern")]
        sold = {p for (p,) in db.execute("SELECT DISTINCT i.patternid FROM npcitem i JOIN npcs n ON n.npc_id = i.npcid "
                                         "WHERE n.npc_id != 0 AND (n.npc_type & 1) = 1")}
        crafted = {p for (p,) in db.execute("SELECT product FROM craftrecipes")} if w.has_table(db, "craftrecipes") else set()
        self.donors = [r for r in self.rows if r["id"] not in sold and r["id"] not in crafted and r["rent"] != 0
                       and not (w.ID_FIRST <= r["id"] <= w.ID_LAST) and r["level"] >= 1]
        for r in self.rows:
            r["stats"] = {c: r.get(c) or 0 for c in STAT_COLS}
            r["power"] = stat_points(r) + (r["AC"] or 0)
        self.taken = set()      # donors an estimate took: another takes the next in rank where it can

    def siblings(self, k):
        words = distinctive(k)
        if not words:
            return []
        return [r for r in self.rows if words & distinctive(w.key(r["name"])) and r["level"] >= 1 and w.key(r["name"]) != k]

    def candidates(self, slot, at, level, classes, span, style, strict=False):
        """Items of ours within span levels that may give an item its figures, ranked by power (a weapon's by its
        damage first), or None when fewer than three: a weapon's of its attack type in the slots that type is
        held in (a one-hander's either hand's; a two-hander's any two-hander's when three of its type are not
        there), usable by every class it is for, drawn as it will be when strict (an armour piece's AC is its
        style's), no freak record (OUTLIER); where three remain, one drawn as it will be, one whose own name
        says its slot or type (a misfiled record is no donor), an armour
        piece with AC and a weapon with stats, and for a one-class item one of few classes, for an item of no
        classes one most classes may use."""
        slots = {1: (12, 14), 3: (12, 14), 5: (12, 14), 2: (15,), 4: (15,), 6: (15,), 7: (16,), 9: (16,), 10: (16,)}.get(at, (slot,))
        two = {2, 4, 6}

        def bits(m):
            return bin(m or 0).count("1")
        for same_type in (True, False):
            if not same_type and at not in two:
                continue
            c = [r for r in self.donors if r["slot"] in slots and abs(r["level"] - level) <= span
                 and (not at or (r["at"] == at if same_type else r["at"] in two))
                 and (not classes or ((r["classes"] or 0) & classes) == classes)
                 and (r["power"] > 0 or (at and r["dmg"] > 0))
                 and not (strict and r["model"] != style)]
            if len(c) < 3:
                continue
            med = statistics.median(r["power"] for r in c) or 1
            c = [r for r in c if r["power"] <= OUTLIER * med]
            for keep in (lambda r: style is None or r["model"] == style,
                         lambda r: (slot_of(r["name"])[1] == at) if at else (slot_of(r["name"])[0] == slot),
                         lambda r: (r["power"] > 0) if at else (r["AC"] > 0 if slot in w.ARMOUR | {13} else True),
                         lambda r: (bits(r["classes"]) <= NARROW if classes and bits(classes) <= 2 else
                                    bits(r["classes"]) >= WIDE if not classes and not at else True)):
                narrower = [r for r in c if keep(r)]
                if len(narrower) >= 3:
                    c = narrower
            if len(c) >= 3:
                return sorted(c, key=lambda r: ((r["dmg"] if at else 0), r["power"], r["id"]))
        return None

    def donor(self, slot, at, level, classes, name="", style=None, take=True):
        """(the donor, its span): the candidates of the narrowest span holding three, an armour piece's of its
        style within STYLE_SPAN levels before any other's, at the item's own rank in the band (by its name); one
        another estimate took gives way to the nearest in rank in the band no estimate took, in that search
        or the next, else it is taken again."""
        lo, hi = DONOR_RANKS
        rank = lo + (hi - lo) * (int(hashlib.md5(name.encode()).hexdigest()[:4], 16) / 0xFFFF)
        first = None
        order = ([(span, True) for span in DONOR_SPANS if span <= STYLE_SPAN] if style is not None else []) + \
            [(span, False) for span in DONOR_SPANS]
        for span, strict in order:
            c = self.candidates(slot, at, level, classes, span, style, strict)
            if not c:
                continue
            n = len(c) - 1
            i = min(n, int(rank * n + 0.5))
            if not take:
                return c[i], span
            band = range(min(n, int(lo * n + 0.5)), min(n, int(hi * n + 0.5)) + 1)
            free = [j for j in sorted(band, key=lambda j: (abs(j - i), j)) if c[j]["id"] not in self.taken]
            if free:
                self.taken.add(c[free[0]]["id"])
                return c[free[0]], span
            if first:
                break
            first = (c[i], span)
        return first or (None, None)


def class_note(bullets):
    for _page, text in bullets:
        m = re.search(r"\((\w+(?: \w+)?) only\)", text, re.I)
        if m and m.group(1).lower() in CLASS_NAMES:
            return 1 << CLASS_NAMES[m.group(1).lower()]
    return None


def plan(db, pages, harvest, ours, pool):
    """[(item dict for import_wiki_items.build, why)] and [(name, why)] left."""
    _made, theirs = w.plan(db, pages, harvest, ours)
    names = {k for k, why in theirs if "name alone" in why}
    left = []
    # item pages that give no level (Boneduke's Boots): their page's classes and figures
    paged = {}
    for title, p in pages.items():
        if set(w.ITEM_CATS) & set(p["cats"]) and "Category:Item Sets" not in p["cats"] and not w.item_page(title, p):
            k = w.key(title)
            if not ours.have(k):
                names.add(k)
                t = p["text"] or ""
                cl = re.search(r"Classes:\s*([^\n]+)", t)
                stats = {w.STAT_NAMES[n.lower()]: int(v) for n, v in re.findall(r"^\*\s*([A-Za-z]+):\s*(-?\d+)", t, re.M)
                         if w.STAT_NAMES.get(n.lower())}
                slot = None
                for c in p["cats"]:
                    slot = slot or w.ITEM_CATS.get(c)
                paged[k] = dict(title=title, classes=w.mask(cl.group(1) if cl else "", w.CLASS_ABBR, w.ALL_CLASSES), stats=stats, slot=slot)
    bullets = collections.defaultdict(list)
    for t, p in pages.items():
        for sec in re.finditer(r"^==+\s*([^=]*?)\s*==+\s*$(.*?)(?=^==|\Z)", p["text"] or "", re.M | re.S):
            if "loot" in sec.group(1).lower():
                for b in re.finditer(r"^\*+\s*(.+)$", sec.group(2), re.M):
                    bullets[w.key(re.sub(r"\s*\([^)]*\)", "", b.group(1)))].append((t, b.group(1)))
    src = w.sources(pages, names | {w.key(r["name"]) for r in pool.rows if w.ID_FIRST <= r["id"] <= w.ID_LAST})
    kinds = type_names(ours)
    out = []
    for k in sorted(names):
        if ours.have(k):
            continue
        page = paged.get(k, {})
        title = page.get("title") or next((re.sub(r"\[\[(?:[^|\]]*\|)?([^\]]*)\]\]", r"\1", re.sub(r"\[\[File:[^\]]*\]\]|\s*\([^)]*\)", "", b))
                                           for _t, b in bullets.get(k, [])), k)
        name = w.titled(title.strip())
        if name[:1].islower():
            name = name[:1].upper() + name[1:]
        slot, at, slot_why = slot_of(name)
        if page.get("slot"):
            slot, slot_why = page["slot"], "its page's category"
        sib = pool.siblings(k)
        sources = list(src.get(k, []))
        if not sources:     # its set's or clan's: the siblings' sources
            for r in sib:
                if src.get(w.key(r["name"])):
                    sources = list(src[w.key(r["name"])])
                    break
        level, lwhy = None, ""
        if page.get("stats", {}).get("AC") and slot:
            near = sorted((abs((r["AC"] or 0) - page["stats"]["AC"]), r["level"]) for r in pool.donors if r["slot"] == slot and r["AC"])
            if near:
                level, lwhy = int(statistics.median(lv for _d, lv in near[:5])), "the level of ours of its AC"
        for kind, who, a, b, _ultra in sources:
            if level:
                break
            c = w.SOURCE_ALIASES.get(w.key(who), w.key(who))
            if kind == "named" and c in ours.creatures and max(ours.creatures[c]) > 0:
                level, lwhy = min(MAX_LEVEL, max(ours.creatures[c])), f"{who}'s level"
            elif kind == "type" and a:
                level, lwhy = (a + b) // 2, f"its type's range {a}-{b}"
        if not level and sib:
            level, lwhy = int(statistics.median(r["level"] for r in sib)), "its siblings' (" + ", ".join(sorted({r["name"] for r in sib})[:3]) + ")"
        if not level:
            for kind, who, _a, _b, _u in sources:
                tk = w.key(who)
                if tk in kinds and kinds[tk]:
                    level, lwhy = int(statistics.median(lv for n in kinds[tk] for lv in ours.creatures[n] if lv)), f"the median of the {tk}' spawns"
                    break
        if not level:
            left.append((k, "no level: no sibling, no creature of ours, no type"))
            continue
        level = max(1, min(MAX_LEVEL, level))
        classes = class_note(bullets.get(k, [])) or page.get("classes")
        cwhy = "its loot list's or page's" if classes else ""
        if not classes and sib:
            mine = {(kind, w.key(who)) for kind, who, *_x in sources}
            kin = [r for r in sib if mine & {(kind, w.key(who)) for kind, who, *_x in src.get(w.key(r["name"]), [])}]
            c = collections.Counter(r["classes"] for r in kin if r["classes"])
            if c:
                classes, cwhy = c.most_common(1)[0][0], "its set's (" + ", ".join(sorted({r["name"] for r in kin})[:2]) + ")"
        style = w.style_of(dict(name=name, classes=classes, pics=""))[0] if slot in w.ARMOUR and classes else None
        take = not page.get("stats")
        donor, span = pool.donor(slot, at, level, classes, k, style, take)
        if not donor:
            left.append((k, f"no item of ours to take figures from (slot {slot}, type {at}, level {level})"))
            continue
        if not classes:     # a real item's line, its classes too; a weapon's only from a weapon of its own type
            m = ours.masks.get((slot, at)) if at and donor["at"] != at else None
            classes, cwhy = ((m.most_common(1)[0][0], f"the commonest of this server's type {at} weapons'") if m else
                             (donor["classes"] or w.ALL_CLASSES, "its donor's"))
        stats = dict(donor["stats"])
        if page.get("stats"):
            stats = {c: page["stats"].get(c, 0) for c in STAT_COLS}
        it = dict(name=name, key=k, level=level, slot=slot, attack=at or donor["at"] if slot in (12, 14, 15, 16) else 0,
                  dmg=donor["dmg"] if slot in (12, 14, 15, 16) else 0,
                  rng=25 if slot in (12, 14, 15, 16) and (at or donor["at"]) in (7, 9, 10) else 0,
                  stats={c: v for c, v in stats.items() if v}, hp=donor["maxhp"] or 200 * level, dur=50, races=w.ALL_RACES,
                  classes=classes, notrade=False, desc="", pics="", src="an estimate (the wiki names it alone)",
                  found="", sources=sources)
        why = (f"slot {slot} ({slot_why}); level {level} ({lwhy}); classes {classes} ({cwhy}); figures "
               f"{'its page' if page.get('stats') else 'of ' + repr(donor['name'])} (within {span})")
        out.append((it, why))
    return out, left


def type_names(ours):
    out = {}
    for t, pat in TYPES.items():
        out[t] = sorted(ours.undead) if pat is None else [n for n in ours.creatures if re.search(pat, n)]
    return out


def placements(it, ours):
    """import_wiki_items' placement rows, with this tool's rates."""
    out, seen = [], set()
    lo, hi = max(1, it["level"] - POOL_SPAN), it["level"] + POOL_SPAN
    for kind, who, a, b, _ultra in it["sources"]:
        c = w.SOURCE_ALIASES.get(w.key(who), w.key(who))
        if kind == "named" and c in ours.creatures and ("npcloot", c) not in seen:
            seen.add(("npcloot", c))
            chance = BOSS_CHANCE if max(ours.creatures[c]) >= BOSS_LEVEL else w.NAMED_CHANCE
            out.append(("npcloot", c, chance, w.WEIGHT, 1, 100, f"the wiki: {who} drops it"))
        elif kind == "named" and w.key(who) in TYPES and ("type", w.key(who)) not in seen:
            seen.add(("type", w.key(who)))
            out.append(("type", w.key(who), w.TYPE_CHANCE, w.WEIGHT, max(1, it["level"] - TYPE_SPAN), it["level"] + TYPE_SPAN,
                        f"the wiki: {who}' own loot"))
        elif kind == "type" and ("type", who) not in seen:
            seen.add(("type", who))
            out.append(("type", who, w.TYPE_CHANCE, w.WEIGHT, a, b, f"the wiki: {who} of {a}-{b} drop it"))
    if not out:
        out.append(("pool", "*", 0, 1, lo, hi, "its source is not in this world: every region's rare pool"))
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", default=w.DEFAULT_DB)
    ap.add_argument("--wiki", help="tools/import_wiki_items.py --fetch's folder")
    ap.add_argument("--prices", nargs="*", default=[], help="eqoa harvest JSON files or folders of them")
    ap.add_argument("--list", action="store_true")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 10000")
    if a.undo:
        db.execute("BEGIN IMMEDIATE")
        w.undo(db, LOG)
        db.execute("COMMIT")
        return
    if not a.wiki or not a.prices:
        sys.exit("--wiki and --prices are both needed")
    if w.has_table(db, LOG):
        sys.exit(f"{LOG} is there: --undo first")
    pages = json.load(open(os.path.join(a.wiki, "allpages.json")))
    ours = w.Ours(db)
    pool = Pool(db)
    items, left = plan(db, pages, w.harvest_records(a.prices), ours, pool)
    measured = Measured()
    measured.load(a.prices)
    rule, _ = robust_rule(measured)
    used = {pid for (pid,) in db.execute("SELECT patternid FROM itempattern WHERE patternid BETWEEN ? AND ?", (w.ID_FIRST, w.ID_LAST))}
    recs, rows = [], []
    for it, why in items:
        r, _ = w.build([it], ours, rule, used)
        rec, bwhy = r[0]
        src, rest = bwhy.split("; ", 1)
        recs.append((rec, f"{src}: {why}; {rest}"))
        for kind, target, chance, weight, lo, hi, reason in placements(it, ours):
            rows.append(dict(kind=kind, target=target, chance=chance, weight=weight, low=lo, high=hi, pid=rec["patternid"],
                             name=it["name"], level=it["level"], why=reason, category="rare"))
    print(f"{len(recs)} items to make with estimated figures, {len(left)} left")
    print("  placed: " + ", ".join(f"{k} {n}" for k, n in collections.Counter(r["kind"] for r in rows).most_common()))
    for rec, why in recs:
        st = {c: rec[c] for c in STAT_COLS if rec.get(c)}
        print(f"  {rec['patternid']} {rec['itemname']!r} L{rec['levelreq']} slot {rec['equipslot']} type {rec['attacktype']} dmg "
              f"{rec['weapondamage']} classes {rec['classuse']} value {rec['itemcost']} {st}" + (f" | {why}" if a.list else ""))
    for r in rows if a.list else []:
        print(f"    -> {r['name']!r}: {r['kind']} {r['target']!r} chance {r['chance']} levels {r['low']}-{r['high']} ({r['why']})")
    for k, why in left:
        print(f"  left: {k!r}: {why}")
    if not a.apply:
        print("report only: --apply to write")
        return
    db.execute("BEGIN IMMEDIATE")
    placed = w.write(db, recs, rows, ours, LOG, type_names(ours))
    db.execute("COMMIT")
    print(f"made {len(recs)} items; placed: " + ", ".join(f"{k} {n}" for k, n in placed.most_common()))


if __name__ == "__main__":
    main()
