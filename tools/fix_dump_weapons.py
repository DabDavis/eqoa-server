#!/usr/bin/env python3
"""The old dump's weapons as weapons (the user, 2026-10-29: "fix the 426
weapons with no attack type too", and "some weapons are using wrong icon, it
looks like an epic weapon 2h sword for crafted composite longbow, that same
icon is on many").

This server's old dump carries 423 weapons with damage and no attack type,
none of which Sandstorm sells. tools/fix_weapon_records.py gave the ones it
does sell Sandstorm's records.

- **The attack type** (itempattern.attacktype) is what reaches. 1-6 are the
  melee kinds (one- and two-hand slash, blunt and pierce); 7, 9 and 10 are
  bow, crossbow and thrown, which reach by the range. 0 never reaches
  (CLIENT's TargetInRange, entity/attack.go SwingFor).
- **Nearly all sit in slot 12**, the primary hand, zweihanders, staves and
  poleaxes too; Sandstorm's two-handers are slot 15.
- **Nearly all have no model**, so nothing is drawn in the hand.
- **Nearly all carry the dump's stand-in icon** (848266163 on eqoaclassic:
  455 items, 433 of them of no attack type). None of Sandstorm's 1,316 items
  uses it, and it draws an epic two-hand sword for a bow. The fix of the day
  gave the bows Sandstorm's records but kept this icon.

**A weapon of no attack type** takes the first of these that has it:
1. Sandstorm's record (--prices: its merchants' wares in the harvests): its
   attack type and wear slot.
2. Allakhazam's page (--pages: tools/fetch_item_pages.py's folder, its
   players' submissions of the live game's items), by the item's name: its
   "Weapon Type" (One Hand Slashing 1, Two Hand Slashing 2, One Hand Blunt
   3, Two Hand Blunt 4, One Hand Piercing 5, Two Hand Piercing 6, Bow 7,
   Crossbow 9, Thrown 10) and its slot (a two-hander 15, a ranged weapon 16,
   "Secondary" 14, else the slot it has). Pages of one name that disagree
   are left to the rules below.
3. The weapon its name says (OURS: KIND below, the name, then its
   description: "Goblin Stave" a staff, "This tiny dagger packs a big punch"
   a dagger), its hands by the words (TWO_HANDED), a two-hander in slot 15.
4. Its icon's commonest attack type among this server's typed weapons
   (OURS), never the stand-in's.
5. One-hand blunt (OURS), listed: nothing says what it is.

**A weapon in wear slot 0** (284 on eqoaclassic: no slot, CLIENT's "empty";
Sandstorm's records use none, and the server's wear table refuses it) goes
to its hand slot by its attack type, Allakhazam's "Secondary" to 14; one of
no attack type takes one first, as above. One named as armour (a shield,
bracers, a ring) is left to tools/fix_wear_slots.py.

**A weapon with the stand-in icon** (slot 12, 14, 15 or 16: 13 is a shield's; of any type) takes its own
Sandstorm record's icon, else the commonest icon of its kind (the name's last
word and attack type) among Sandstorm's weapons, then this server's, else
the commonest of its attack type among Sandstorm's (OURS: a stand-in of its
kind). A stand-in is an icon on 20 weapons or more of three kinds or more that no
Sandstorm item carries (eqoaclassic has two: 848266163, the dump's weapons',
and -570739083, every slot 0 item's), or --placeholder's.

A weapon with no model takes the model most of this server's weapons of its
kind wear, else the commonest of its attack type (ours, as
fix_weapon_records.py). A ranged one with no range takes its description's
"Range: N", else 25 (fix_ranged_reach.py's). A potion's (itemeffect) is
left, and one a GM changed (gm_audit) too.

The report measures each source against Sandstorm's records where both have
a weapon: Allakhazam's pages, the rule and the icons.

Every change is logged in dump_weapons_fix_log (the pattern, its values
before and after, the source); --undo puts back each one still as this left
it and drops the log. Report only by default. The server reads the record
at each swing, so a weapon strikes at once; a client draws its model and
icon and files its slot from the item records it gets at its login.

    tools/fix_dump_weapons.py [--db eqoa.db] --pages <folder> --prices <harvests> ... [--placeholder ICON ...] [--apply | --undo]
"""
import argparse
import collections
import datetime
import glob
import json
import os
import re
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import fetch_item_pages  # noqa: E402

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
DEFAULT_RANGE = 25
WEAPON_TYPES = {
    "one hand slashing": 1, "two hand slashing": 2, "one hand blunt": 3, "two hand blunt": 4,
    "one hand piercing": 5, "two hand piercing": 6, "bow": 7, "crossbow": 9, "thrown": 10, "throwing": 10,
}
TWO_HAND = {2, 4, 6}
RANGED = {7, 9, 10}
SLOT_TWO_HAND, SLOT_RANGED, SLOT_EITHER = 15, 16, 14
# OURS: the weapon words, and the words of a two-hander. EQOA's own names
# (Sandstorm's records) make a battleaxe and a spear one-handed, a greataxe,
# a beheader, a warblade, a walking stick and a glaive (a slashing weapon)
# two-handed. The rule is tuned on those records; the report measures it on
# Allakhazam's pages of weapons Sandstorm does not sell, which it was not.
RANGED_WORDS = [(r"\bcross ?bows?\b", 9), (r"\b(?:great|long|short|composite|recurve)?bows?\b", 7),
                (r"\bthrow(?:ing)? (?:knife|knives|dagger|axe|star)s?\b|\bshuriken\b|\bchakram\b", 10)]
KIND = [
    (5, r"\b(?:spears?|shortspears?|longspears?|tridents?|pikes?|lances?|ahlsp[ie]+ss|partizans?|partisans?|"
        r"ranseurs?|spetums?|harpoons?|pitchforks?|javelins?|daggers?|knives|knife|dirks?|stil+et+os?|"
        r"kris+|rapiers?|shanks?|stingers?|stabbers?|stinging|fangs?|picks?|needles?|spikes?|awls?|epees?|"
        r"foils?|tantos?|sai|claws?|talons?|sliver|tooth|pricks?|skewers?)\b"),
    (3, r"\b(?:staffs?|staves?|stave|quarterstaffs?|liegestaffs?|battlestaffs?|mechanastaff|mauls?|frostmaul|"
        r"hammers?|warhammers?|battlehammers?|doomhammer|forgehammer|clubs?|warclubs?|greatclubs?|maces?|"
        r"morning ?stars?|flails?|scepters?|sceptres?|jitte|knuckles?|nuckles|dusters|fists?|warfists?|"
        r"cudgels?|batons?|rods?|bludgeons?|blackjacks?|bashers?|knocka|gavels?|mallets?|truncheons?|stumps?|"
        r"bones?|totems?|wands?|branch|crooks?|crosiers?|canes?|sticks?|mattocks?|oars?|relic|bark|thicket)\b"),
    (1, r"\b(?:swords?|swrod|blades?|warblades?|longblades?|shortblades?|grimblade|iceblade|sabres?|sabers?|"
        r"scimitars?|shamshirs?|shortswords?|longswords?|broadswords?|bastardswords?|bastard swords?|"
        r"greatswords?|claymores?|zweihanders?|two handers?|katanas?|cutlass(?:es)?|falchions?|gladius|axes?|"
        r"handaxes?|waraxes?|battleaxes?|doubleaxes?|greataxes?|hatchets?|cleavers?|edge|slicers?|tulwars?|"
        r"khopesh|machetes?|naginatas?|glaives?|halberds?|halbreds?|halberts?|poleaxes?|scythes?|sickles?|choppa|"
        r"choppers?|flameberge|flamberge|reavers?|foereaver|beheaders?|splitters?|gorgesplitter|grimsplitter|"
        r"carvers?|gravecarver|rippers?|reapers?|slayers?|slashers?|cutters?|severers?|gutwrenchers?)\b"),
]
TWO_HANDED = (r"\b(?:great ?swords?|greataxes?|great axes?|great ?clubs?|great ?hammers?|claymores?|"
              r"zweihanders?|two.?hand(?:ed|er)?s?|bastard ?swords?|flameberge|flamberge|halberds?|halbreds?|"
              r"halberts?|poleaxes?|scythes?|glaives?|naginatas?|staffs?|staves?|stave|quarterstaffs?|"
              r"liegestaffs?|battlestaffs?|mechanastaff|mauls?|frostmaul|warhammers?|battlehammers?|mattocks?|"
              r"pitchforks?|partizans?|partisans?|ranseurs?|spetums?|ahlsp[ie]+ss|lances?|pikes?|tridents?|"
              r"harpoons?|crooks?|crosiers?|oars?|doubleaxes?|beheaders?|warblades?|walking sticks?)\b")

LOG_SCHEMA = """CREATE TABLE dump_weapons_fix_log (
    patternid INTEGER PRIMARY KEY,
    before    TEXT NOT NULL,
    after     TEXT NOT NULL,
    source    TEXT NOT NULL,   -- sandstorm | allakhazam <ids> | name | description | icon | default, and the icon's
    applied   TEXT NOT NULL
)"""


def key(name):
    return " ".join((name or "").replace("`", "'").replace("’", "'").lower().split())


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def by_words(text):
    """(attack type, two-handed) the words say, or None (OURS)."""
    t = key(text)
    for pat, at in RANGED_WORDS:
        if re.search(pat, t):
            return at, False
    two = re.search(TWO_HANDED, t) is not None
    for at, pat in KIND:
        if re.search(pat, t):
            return at + (1 if two else 0), two
    return None


def sandstorm(paths):
    """Sandstorm's weapons by name: {name: (attack type, wear slot, icon)}, a
    name of two records left out; and every icon its items carry."""
    files = []
    for p in paths:
        files += glob.glob(os.path.join(p, "**", "*.json"), recursive=True) if os.path.isdir(p) else [p]
    seen = collections.defaultdict(set)
    icons = set()

    def walk(o):
        if isinstance(o, dict):
            if o.get("name") and "icon" in o and "wear_slot" in o:
                icons.add(o["icon"])
            if o.get("name") and isinstance(o.get("attack_type"), int) and 1 <= o["attack_type"] <= 10 \
                    and o.get("damage", 0) > 0:
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
            seen[key(w["name"])].add((w["attack_type"], w.get("wear_slot"), w.get("icon")))
    return {k: next(iter(v)) for k, v in seen.items() if len(v) == 1}, icons


def placeholder_icons(db, ss_icons):
    """The dump's stand-in icons: each on 20 weapons or more, of three kinds or
    more (a real icon draws one kind), that no Sandstorm item carries."""
    kinds = collections.defaultdict(collections.Counter)
    for icon, at, name in db.execute("SELECT itemicon, attacktype, itemname FROM itempattern WHERE "
                                     "(weapondamage > 0 OR attacktype > 0) AND equipslot IN (0, 12, 14, 15, 16)"):
        g = by_words(name)
        kinds[icon][at or (g[0] if g else 0)] += 1
    return {icon for icon, c in kinds.items()
            if icon not in ss_icons and sum(c.values()) >= 20 and len([k for k in c if k]) >= 3}


def allakhazam(folder):
    """Allakhazam's weapon types by name: {name: (attack type, slot word,
    ids, damage or None)}, a name whose pages disagree left out (and
    counted)."""
    if not folder:
        return {}, 0
    with open(os.path.join(folder, "lists.json")) as f:
        lists = json.load(f)
    pages = {}
    if os.path.exists(os.path.join(folder, "pages.json")):
        with open(os.path.join(folder, "pages.json")) as f:
            pages = json.load(f)
    # a page fetched but not yet read into pages.json (a fetch still going)
    for path in glob.glob(os.path.join(folder, "pages", "*.html")):
        iid = os.path.basename(path)[:-5]
        if iid.isdigit() and iid not in pages:
            p = fetch_item_pages.read_page(fetch_item_pages.read_file(path))
            if p:
                pages[iid] = p
    out, split = {}, 0
    for name, entries in lists.items():
        got = set()
        ids = []
        for iid, _slot in entries:
            p = pages.get(str(iid))
            if not p or not p.get("Weapon Type"):
                continue
            at = WEAPON_TYPES.get(key(p["Weapon Type"]))
            if at is None:
                continue
            dmg = p.get("DAMAGE", "")
            got.add((at, p.get("Slot", ""), int(dmg) if dmg.isdigit() and int(dmg) > 0 else None))
            ids.append(iid)
        kinds = {g[0] for g in got}
        if len(kinds) == 1:
            slots = {g[1] for g in got}
            dmgs = {g[2] for g in got if g[2]}
            out[name] = (kinds.pop(), slots.pop() if len(slots) == 1 else "", sorted(ids),
                         dmgs.pop() if len(dmgs) == 1 else None)
        elif len(kinds) > 1:
            split += 1
    return out, split


def slot_for(at, slot, page_slot=""):
    if at in TWO_HAND:
        return SLOT_TWO_HAND
    if at in RANGED:
        return SLOT_RANGED
    if page_slot == "Secondary":
        return SLOT_EITHER
    return slot if slot in (12, 14) else 12


# what, in slot 0, is armour and no weapon (left to tools/fix_wear_slots.py)
ARMOUR_WORDS = (r"\b(?:shields?|targes?|bucklers?|bracers?|vambraces?|bracelets?|braclets?|bangles?|armlets?|armwraps?|"
                r"wraps|cuffs?|shackles?|chains|rings?|bands?|greaves|insignias?|charms?)\b")


def plan(db, ss, ak, stand_ins):
    """[(patternid, name, before, after, source)] and what was left."""
    skip = set()
    if has_table(db, "itemeffect"):
        skip |= {p for (p,) in db.execute("SELECT patternid FROM itemeffect")}
    if has_table(db, "gm_audit"):
        for (k,) in db.execute("SELECT rowkey FROM gm_audit WHERE tbl = 'itempattern'"):
            try:
                skip.add(int(json.loads(k)["patternid"]))
            except (ValueError, KeyError, TypeError):
                pass
    typed = db.execute("SELECT itemname, itemicon, model, attacktype FROM itempattern "
                       "WHERE weapondamage > 0 AND attacktype BETWEEN 1 AND 10").fetchall()
    icons = collections.defaultdict(collections.Counter)       # ours: icon -> types
    kinds = collections.defaultdict(collections.Counter)       # ours: (last word, type) -> models
    by_type = collections.defaultdict(collections.Counter)     # ours: type -> models
    our_kind_icon = collections.defaultdict(collections.Counter)
    our_type_icon = collections.defaultdict(collections.Counter)
    for name, icon, model, at in typed:
        words = key(name).split()
        if icon and icon not in stand_ins:
            icons[icon][at] += 1
            if words:
                our_kind_icon[(words[-1], at)][icon] += 1
            our_type_icon[at][icon] += 1
        if model:
            if words:
                kinds[(words[-1], at)][model] += 1
            by_type[at][model] += 1
    ss_kind_icon = collections.defaultdict(collections.Counter)
    ss_type_icon = collections.defaultdict(collections.Counter)
    for name, (at, _ws, icon) in ss.items():
        if icon and icon not in stand_ins:
            words = name.split()
            if words:
                ss_kind_icon[(words[-1], at)][icon] += 1
            ss_type_icon[at][icon] += 1

    levels = collections.defaultdict(list)   # ours: type -> [(level, damage)]
    for lv, d, at in db.execute("SELECT levelreq, weapondamage, attacktype FROM itempattern "
                                "WHERE attacktype BETWEEN 1 AND 10 AND weapondamage > 0 AND levelreq > 0"):
        levels[at].append((lv, d))

    def damage_for(at, level):
        """(damage, why) of a weapon of none: the median of this server's
        weapons of its type within 2 levels, else 5, else 10 (OURS: within 2,
        half come within 7% of the real figure, measured leaving each out)."""
        for span in (2, 5, 10):
            near = sorted(d for lv, d in levels.get(at, []) if abs(lv - level) <= span)
            if len(near) >= 3:
                return near[len(near) // 2], f"damage the median of this server's type {at} within {span} levels"
        return None, ""

    def commonest(c):
        return max(c.items(), key=lambda kv: (kv[1], -abs(kv[0])))[0]

    def icon_for(k, at):
        """(icon, why) of a weapon of the stand-in's."""
        if k in ss and ss[k][2] and ss[k][2] not in stand_ins:
            return ss[k][2], "its Sandstorm icon"
        words = k.split()
        for table, why in ((ss_kind_icon, "Sandstorm's"), (our_kind_icon, "this server's")):
            c = table.get((words[-1], at)) if words else None
            if c:
                return commonest(c), f"the icon of {why} {words[-1]}s"
        for table, why in ((ss_type_icon, "Sandstorm's"), (our_type_icon, "this server's")):
            if table.get(at):
                return commonest(table[at]), f"the icon of {why} weapons of type {at}"
        return None, ""

    out, left = [], []
    marks = ", ".join("?" * len(stand_ins)) or "NULL"
    rows = db.execute("SELECT patternid, itemname, itemdesc, itemicon, equipslot, model, unk5, attacktype, weapondamage, "
                      "levelreq FROM itempattern WHERE (equipslot IN (12, 14, 15, 16) AND "
                      f"(attacktype = 0 OR itemicon IN ({marks}))) OR "
                      "(equipslot = 0 AND (attacktype > 0 OR weapondamage > 0)) ORDER BY patternid",
                      sorted(stand_ins)).fetchall()
    for pid, name, desc, icon, slot, model, rng, at0, dmg, level in rows:
        if pid in skip:
            left.append((name, "a potion's or a GM's"))
            continue
        k = key(name)
        if slot == 0 and re.search(ARMOUR_WORDS, k):
            left.append((name, "in slot 0 and named as armour: tools/fix_wear_slots.py's"))
            continue
        before, after = {}, {}
        source = ""
        at = at0
        named = k in ss or k in ak or by_words(name) or by_words(desc or "")
        if at0 == 0 and dmg == 0 and not named:
            if icon in stand_ins:
                left.append((name, "no damage and nothing says it is a weapon"))
            continue
        if at0 == 0:
            if k in ss:
                at, ws, _ = ss[k]
                new_slot, source = ws if ws is not None else slot_for(at, slot), "sandstorm"
            elif k in ak:
                at, page_slot, ids, _ = ak[k]
                new_slot, source = slot_for(at, slot, page_slot), "allakhazam " + ",".join(map(str, ids))
            elif by_words(name):
                at, _ = by_words(name)
                new_slot, source = slot_for(at, slot), "name"
            elif by_words(desc or ""):
                at, _ = by_words(desc)
                new_slot, source = slot_for(at, slot), "description"
            elif icon not in stand_ins and icon in icons:
                at = icons[icon].most_common(1)[0][0]
                new_slot, source = slot_for(at, slot), "icon"
            else:
                at, new_slot, source = 3, slot_for(3, slot), "default"
                left.append((name, "nothing says what it is: one-hand blunt (ours)"))
            before["attacktype"], after["attacktype"] = 0, at
            if new_slot != slot:
                after["equipslot"], before["equipslot"] = new_slot, slot
            if dmg == 0:
                page_dmg = ak[k][3] if k in ak else None
                new_dmg, why = (page_dmg, "damage its page's") if page_dmg else damage_for(at, level)
                if not new_dmg:
                    left.append((name, "no damage, and none to take"))
                    continue
                before["weapondamage"], after["weapondamage"] = 0, new_dmg
                source += "; " + why
        elif slot == 0:
            # typed, but in no slot: its hand slot by its type
            page_slot = ak[k][1] if k in ak and ak[k][0] == at0 else ""
            after["equipslot"], before["equipslot"] = slot_for(at0, 0, page_slot), 0
            source = "its type's hand slot"
        if icon in stand_ins and at:
            new_icon, why = icon_for(k, at)
            if new_icon:
                before["itemicon"], after["itemicon"] = icon, new_icon
                source = (source + "; " if source else "") + why
        if not model and at:
            words = k.split()
            c = kinds.get((words[-1], at)) if words else None
            m = commonest(c) if c else (commonest(by_type[at]) if by_type[at] else 0)
            if m:
                after["model"], before["model"] = m, 0
        if at in RANGED and not rng:
            mm = re.search(r"\bRange:?\s*(\d+)", desc or "")
            after["unk5"], before["unk5"] = (int(mm.group(1)) if mm else DEFAULT_RANGE), 0
        if after:
            out.append((pid, name, before, after, source or "its kind"))
    return out, left


def measure(db, ss, ak, stand_ins):
    """How often each source agrees with Sandstorm's records where both have a
    weapon: [(source, agree, of)]."""
    icons = collections.defaultdict(collections.Counter)
    names = {}
    for name, icon, at in db.execute("SELECT itemname, itemicon, attacktype FROM itempattern "
                                     "WHERE weapondamage > 0 AND attacktype BETWEEN 1 AND 10"):
        names.setdefault(key(name), icon)
        if icon and icon not in stand_ins:
            icons[icon][at] += 1
    res = []
    both = [(k, ss[k][0], ak[k][0]) for k in ss if k in ak]
    res.append(("Allakhazam's pages", sum(1 for _, s_, a in both if s_ == a), len(both)))
    rule = [(k, v[0], by_words(k)) for k, v in ss.items()]
    rule = [(k, at, g[0]) for k, at, g in rule if g]
    res.append(("the name rule (tuned on these)", sum(1 for _, s_, g in rule if s_ == g), len(rule)))
    akr = [(k, v[0], by_words(k)) for k, v in ak.items() if k not in ss]
    akr = [(k, at, g[0]) for k, at, g in akr if g]
    res.append(("the name rule against Allakhazam's other pages", sum(1 for _, a, g in akr if a == g), len(akr)))
    ic = []
    for k, v in ss.items():
        icon = names.get(k)
        if icon in icons:
            c = icons[icon].copy()
            c[v[0]] -= 1  # the weapon itself left out
            c = +c
            if c:
                ic.append((v[0], c.most_common(1)[0][0]))
    res.append(("the icons", sum(1 for s_, g in ic if s_ == g), len(ic)))
    return res


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--pages", help="tools/fetch_item_pages.py's folder (Allakhazam's pages)")
    ap.add_argument("--prices", nargs="*", default=[], help="eqoa harvest JSON files or folders of them")
    ap.add_argument("--placeholder", type=int, action="append", help="a stand-in icon (default: found)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the weapons (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply changed")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        if not has_table(db, "dump_weapons_fix_log"):
            print("nothing logged: nothing to undo")
            return
        db.execute("BEGIN IMMEDIATE")
        back = left = 0
        for pid, before, after in db.execute("SELECT patternid, before, after FROM dump_weapons_fix_log").fetchall():
            before, after = json.loads(before), json.loads(after)
            c = list(after)
            now = db.execute(f"SELECT {', '.join(c)} FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
            if now is None or dict(zip(c, now)) != after:
                left += 1
                continue
            db.execute(f"UPDATE itempattern SET {', '.join(x + ' = ?' for x in c)} WHERE patternid = ?",
                       [before[x] for x in c] + [pid])
            back += 1
        db.execute("DROP TABLE dump_weapons_fix_log")
        db.execute("COMMIT")
        print(f"put back {back} weapons, {left} changed since left as they are")
        return
    if a.apply and has_table(db, "dump_weapons_fix_log"):
        sys.exit("dump_weapons_fix_log is there: applied already (--undo first)")
    ss, ss_icons = sandstorm(a.prices)
    if not ss:
        sys.exit("no Sandstorm weapon records: give --prices (eqoa harvest JSON)")
    ak, split = allakhazam(a.pages)
    stand_ins = set(a.placeholder) if a.placeholder else placeholder_icons(db, ss_icons)
    print("the dump's stand-in icons: " + (", ".join(map(str, sorted(stand_ins))) if stand_ins else "none found"))
    changes, left = plan(db, ss, ak, stand_ins)
    for pid, name, before, after, source in changes:
        print(f"{pid:7d} {name}: " + ", ".join(f"{c} {before[c]} -> {after[c]}" for c in after) + f" ({source})")
    for name, why in left:
        print(f"note: {name}: {why}")
    for what, agree, of in measure(db, ss, ak, stand_ins):
        print(f"measured: {what} agree with Sandstorm's records on {agree} of {of} weapons"
              + (f" ({100 * agree / of:.1f}%)" if of else "") if "other pages" not in what else
              f"measured: {what}: {agree} of {of}" + (f" ({100 * agree / of:.1f}%)" if of else ""))
    typed = collections.Counter(c[4].split(";")[0].split()[0] for c in changes if "attacktype" in c[3])
    iconed = sum(1 for c in changes if "itemicon" in c[3])
    unslotted = sum(1 for c in changes if c[2].get("equipslot") == 0)
    print(f"{len(changes)} weapons to change: {sum(typed.values())} given a type ("
          + ", ".join(f"{n} by {s_}" for s_, n in typed.most_common()) + f"), {iconed} a new icon, "
          f"{unslotted} out of slot 0; "
          f"{len(ak)} of Allakhazam's weapon names read, {split} whose pages disagree")
    if not a.apply or not changes:
        if not a.apply:
            print("report only: --apply to write")
        return
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_SCHEMA)
        for pid, name, before, after, source in changes:
            c = list(after)
            cur = db.execute(f"UPDATE itempattern SET {', '.join(x + ' = ?' for x in c)} WHERE patternid = ? AND "
                             + " AND ".join(f"{x} = ?" for x in c),
                             [after[x] for x in c] + [pid] + [before[x] for x in c])
            if cur.rowcount == 1:
                db.execute("INSERT INTO dump_weapons_fix_log VALUES (?, ?, ?, ?, ?)",
                           (pid, json.dumps(before, sort_keys=True), json.dumps(after, sort_keys=True), source, when))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"changed {len(changes)} weapons; they strike at once, their models, icons and slots at a player's next login")


if __name__ == "__main__":
    main()
