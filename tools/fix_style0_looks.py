#!/usr/bin/env python3
"""Gear that draws nothing when worn given a look (the user, 2026-11-03, on a troll warrior: "i equipped basher
shirt and no graphic when equipped. the icon is the cloth tunic, so lets make it that. also look for others";
then, of the pieces that went cloth by their icon though their AC is a heavier armour's: "by AC is fine").

An armour piece's style (itempattern.model, wear slots 1 5 7 10 11 19) is the armour set the character is drawn
in (handlers/character.go wornLook; style 0 draws none, on the head no helm); a shield's, weapon's or held item's
model (wear slots 12-18 and 20) is the ITEM.ESF sprite in the hand, and 0 draws nothing. The old dump's records
came with neither: on eqoaclassic 1,437 armour pieces of style 0, 1,392 of them on their slot's plain cloth icon
(the Basher Shirt, a troll warrior's first chest, among them) and 1,272 of those with no class mask, and 300-odd
shields, weapons and held items of model 0. No record says how they were drawn. Changed:

1. **Armour on a cloth icon**: its slot's plain icon (the icon the slot's style 1 records carry most), an icon
   every styled record of the slot carrying it has style 1 (the cloth caps'), or an icon robes (wear 2) carry most
   (a tunic on a robe's picture). Its look, in this order:
   a. DECIDED (the user's word: the Basher Shirt is cloth);
   b. its name's material: a word of MATERIAL_WORDS that this database's styled armour gives one style (at least
      8 records, 9 in 10 of them; the report prints each word's count): that style; a compound ending in scale, plate or chain
      ("Icescale", "Glacierplate", "Nightchain"; not a piece's own noun, "breastplate") counts as the word;
   c. its name's kind: a word of KIND_WORDS that the styled drops give one tier (cloth, leather, chain: styles 3
      and 7, plate: 4 and 5; at least 8 distinct drops, 9 in 10): that tier ("breastplate", "coif", "jerkin");
   d. its AC: the tier whose styled drops of its slot near its level (within 5 levels, wider until 3) have the
      median AC nearest its own (on a log scale), no heavier than its class mask allows (a mask with a magician,
      necromancer, enchanter or wizard cloth; with a monk, druid or alchemist leather; with a ranger, bard, rogue
      or shaman chain: CLIENT's records hold that rule; a mask of 0, the dump's, allows any);
   e. AC 0: cloth, as its icon shows.
   A tier's style is its commonest among the slot's styled drops near the level (chain at 18, scale from the
   20s). A look other than cloth takes the icon the slot's records of that style carry most, so the picture and
   the look agree. "Styled drops": armour records with a style, AC and level, neither crafted ("Armor forged
   of ...") nor changed by this tool, each name once. Measured leaving each out (1,286 on eqoaclassic): a name's
   word names the tier of 226 of the 229 it decides, AC alone 63% (86% for cloth or not, its misses the next tier
   up or down), the two together 70%.
2. **Armour on an icon of a style** (at least 3 records of its slot carry it with a style, 9 in 10 of those the
   same, fix_item_looks.py's rule): that style (the Gust set's scale icons, the Gladestrider bracers').
3. **Armour on another kind's icon** (one no armour record of its slot carries with a style, carried most by
   records of another wear slot: the Brazen Chain Bracers on the earrings' picture): its name's material (1b),
   with that style's icon.
4. **A shield, weapon or held item of model 0** on an icon that the other records of its kind (shields;
   one-handed, either-hand and two-handed weapons; bows; thrown; held items, with the weapons; fishing poles)
   carry with one model (at least 3, 9 in 10 of them): that model, which those records are already drawn with.
   The held items' stand-in picture is a wand's, drawn as the wand it shows (136 books, orbs, totems and horns
   on eqoaclassic): the icon decides, as for the armour.

Left, and listed: armour on an icon no rule reads (crowns, circlets and headbands on icons of their own, which
no styled record shares; a necklace's picture with no material in the name), and anything a GM edited
(gm_audit). Held items on an icon no record draws are counted (books, gems, the alchemist's thrown potions).

Every change is logged in style0_look_fix_log with what it replaced; --undo puts back each record still as this
left it, then drops the log. Report only by default; --only limits a run to the records named; --replan reports
what the rules make of a database this tool already changed (its changes put back first, in one transaction
that is then rolled back), --reapply writes that (one transaction: undone, planned, applied). The server reads a
record's look at each equip and login, CLIENT its icon at its login: no restart. A player sees the look at the
next equip (take it off and put it back on) or login, the new icon at the next login.

    tools/fix_style0_looks.py [--db eqoa.db] [--only PATTERNID ...] [--apply | --undo | --replan | --reapply]
"""
import argparse
import collections
import json
import math
import os
import re
import sqlite3
import statistics
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")

ARMOUR = (1, 5, 7, 10, 11, 19)          # head, chest, bracers, legs, feet, hands: the drawn armour sets
ROBE = 2
CLOTH = 1
STYLE_MIN, STYLE_SHARE = 3, 0.9
WORD_MIN, WORD_SHARE = 8, 0.9
MODEL_MIN, MODEL_SHARE = 3, 0.9
NEAR_LEVELS, NEAR_LEAST, NEAR_MOST = 5, 3, 30
# the armour styles by how heavy they are; 6 and 8 (a few dozen pieces, 8 a monk's) take no part
TIER_OF = {1: "cloth", 2: "leather", 3: "chain", 7: "chain", 4: "plate", 5: "plate"}
TIERS = ("cloth", "leather", "chain", "plate")
# the classes (bits of classuse) that cap a mask's armour: CLIENT's records wear no heavier
CAPS = (("cloth", (10, 11, 12, 13)), ("leather", (4, 7, 14)), ("chain", (1, 5, 6, 8)))
# the user's word
DECIDED = {("basher shirt", 5): (CLOTH, "the user's word: the Basher Shirt is cloth, as its icon")}
# words for what armour is made of; each counts only where the database's styled armour gives it one style
MATERIAL_WORDS = ("cloth", "silk", "silken", "wool", "woolen", "woollen", "burlap", "linen", "cotton", "velvet",
                  "satin", "leather", "skin", "hide", "pelt", "fur", "studded", "chain", "chainmail", "ringmail",
                  "mail", "scale", "scaled", "scalemail", "plate", "plates", "platemail", "banded", "bone")
# words for what a piece is, which may name a tier; each counts only where the styled drops give it one
KIND_WORDS = MATERIAL_WORDS + ("breastplate", "chestplate", "chestguard", "coif", "jerkin", "slippers", "sandals",
                               "shoes", "gauntlets", "greaves", "hauberk", "legplates", "vambraces", "robe", "gown",
                               "turban", "cowl", "hood", "cap", "hat", "mitts", "mittens", "pants", "trousers",
                               "vest", "shirt", "kilt", "skirt", "sleeves", "wraps", "wristguards", "helm", "helmet",
                               "tunic", "leggings", "boots", "gloves", "bracers", "bracer", "coat", "cuirass")
# the hands' kinds (wear slots): a model is taken only off records of the same kind, a held item's off held
# items and weapons too (a wand is either: the held items' stand-in icon is the wands' picture)
HELD_KINDS = {13: "shield", 12: "weapon", 14: "weapon", 15: "weapon", 16: "bow", 17: "thrown", 18: "held",
              20: "fishing"}
MODEL_FROM = {"held": ("held", "weapon")}
CRAFTED = re.compile(r"^\s*(armor|a shield) forged of", re.I)

LOG_DDL = """CREATE TABLE IF NOT EXISTS style0_look_fix_log (
    seq       INTEGER PRIMARY KEY AUTOINCREMENT,
    patternid INTEGER NOT NULL,
    old_icon  INTEGER NOT NULL,
    old_model INTEGER NOT NULL,
    new_icon  INTEGER NOT NULL,
    new_model INTEGER NOT NULL,
    source    TEXT NOT NULL,
    applied   TEXT NOT NULL DEFAULT (datetime('now'))
)"""


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


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


def name_key(name):
    return " ".join((name or "").replace("`", "'").lower().split())


# a material at the end of a compound word ("Icescale", "Glacierplate", "Nightchain") counts as the word; not
# "silk" or "skin", whose compounds are any material (Eelskin and Pineskin pieces are cloth, Sandsilk mixed)
COMPOUND_ENDS = ("scale", "plate", "chain")


def words(name):
    got = set(re.findall(r"[a-z]+", (name or "").lower()))
    for w in list(got):
        if w in KIND_WORDS:
            continue
        for m in COMPOUND_ENDS:
            if w.endswith(m) and len(w) >= len(m) + 3:
                got.add(m)
    return got


def commonest(counter, least, share):
    """The counter's top key when it holds at least `least` and `share` of all, else None."""
    if not counter:
        return None
    k, n = sorted(counter.items(), key=lambda kv: (-kv[1], str(kv[0])))[0]
    return k if n >= least and n >= share * sum(counter.values()) else None


def heaviest(mask):
    """The heaviest tier a class mask allows (a mask of 0, the dump's, allows any)."""
    mask &= 0x7FFF
    if mask:
        for tier, classes in CAPS:
            if any(mask >> c & 1 for c in classes):
                return tier
    return "plate"


def near(points, level):
    """The points (level first) within NEAR_LEVELS of the level, wider until NEAR_LEAST are in."""
    w = NEAR_LEVELS
    while True:
        got = [p for p in points if abs(p[0] - level) <= w]
        if len(got) >= NEAR_LEAST or w >= NEAR_MOST:
            return got
        w += 5


class Facts:
    """What the database's own styled records say about icons, words, levels, AC and models."""

    def __init__(self, rows, ours):
        styled = collections.defaultdict(collections.Counter)      # (slot, icon) -> {style: n}
        by_style = collections.defaultdict(collections.Counter)    # (slot, style) -> {icon: n}
        slots_of = collections.defaultdict(collections.Counter)    # icon -> {wear slot: n}
        word_styles = collections.defaultdict(collections.Counter)
        models = collections.defaultdict(collections.Counter)      # (kind, icon) -> {model: n}
        drops = {}                                                 # name -> (slot, style, level, ac)
        for pid, name, slot, model, icon, ac, level, _mask, desc, craft in rows:
            slots_of[icon][slot] += 1
            if slot in ARMOUR and model:
                styled[(slot, icon)][model] += 1
                by_style[(slot, model)][icon] += 1
                for w in words(name) & set(MATERIAL_WORDS):
                    word_styles[w][model] += 1
                if (model in TIER_OF and ac > 0 and level >= 1 and not craft and not CRAFTED.match(desc or "")
                        and pid not in ours):
                    drops.setdefault(name_key(name), (slot, model, level, ac, name))
            if slot in HELD_KINDS and model:
                for kind, srcs in [(k, MODEL_FROM.get(k, (k,))) for k in set(HELD_KINDS.values())]:
                    if HELD_KINDS[slot] in srcs:
                        models[(kind, icon)][model] += 1
        self.styled, self.slots_of = styled, slots_of
        self.icon_of = {k: sorted(c.items(), key=lambda kv: (-kv[1], kv[0]))[0][0] for k, c in by_style.items()}
        self.style_of = {k: s for k, c in styled.items() if (s := commonest(c, STYLE_MIN, STYLE_SHARE))}
        self.plain = {slot: self.icon_of[(slot, CLOTH)] for slot in ARMOUR if (slot, CLOTH) in self.icon_of}
        self.word_style = {w: s for w, c in word_styles.items() if (s := commonest(c, WORD_MIN, WORD_SHARE))}
        self.word_counts = word_styles
        kind_tiers = collections.defaultdict(collections.Counter)
        self.drops = collections.defaultdict(list)                 # (slot, tier) -> [(level, ac, style)]
        for slot, style, level, ac, name in drops.values():
            self.drops[(slot, TIER_OF[style])].append((level, ac, style))
            for w in words(name) & set(KIND_WORDS):
                kind_tiers[w][TIER_OF[style]] += 1
        self.kind_tier = {w: t for w, c in kind_tiers.items() if (t := commonest(c, WORD_MIN, WORD_SHARE))}
        self.kind_counts = kind_tiers
        self.n_drops = len(drops)
        self.model_of = {k: m for k, c in models.items() if (m := commonest(c, MODEL_MIN, MODEL_SHARE))}

    def cloth_icon(self, slot, icon):
        """Why the icon is cloth's in the slot, or None."""
        if self.plain.get(slot) == icon:
            return "the slot's plain cloth icon"
        c = self.styled.get((slot, icon))
        if c and set(c) == {CLOTH}:
            return "an icon only cloth pieces carry"
        if not c:
            s = self.slots_of.get(icon)
            if s and s.most_common(1)[0][0] == ROBE and s[ROBE] >= STYLE_MIN:
                return "a robe's icon"
        return None

    def other_kind(self, slot, icon):
        """Whether the icon is another kind of item's (no styled record of the slot, most records elsewhere)."""
        if self.styled.get((slot, icon)):
            return False
        s = self.slots_of.get(icon)
        return bool(s) and s.most_common(1)[0][0] != slot

    def name_style(self, name):
        got = {self.word_style[w] for w in words(name) if w in self.word_style}
        return next(iter(got)) if len(got) == 1 else None

    def name_tier(self, name):
        got = {self.kind_tier[w] for w in words(name) if w in self.kind_tier}
        return next(iter(got)) if len(got) == 1 else None

    def tier_by_ac(self, slot, level, ac, most):
        best = None
        for tier in TIERS[:TIERS.index(most) + 1]:
            got = near(self.drops.get((slot, tier), []), level)
            if len(got) < NEAR_LEAST:
                continue
            d = abs(math.log(ac) - math.log(statistics.median(a for _l, a, _s in got)))
            if best is None or d < best[0]:
                best = (d, tier)
        return best[1] if best else None

    def style_in(self, slot, tier, level):
        if tier == "cloth":
            return CLOTH
        c = collections.Counter(s for _l, _a, s in near(self.drops.get((slot, tier), []), level))
        if not c:
            return min(s for s, t in TIER_OF.items() if t == tier)
        return sorted(c.items(), key=lambda kv: (-kv[1], kv[0]))[0][0]


def cloth_look(f, slot, name, ac, level, mask, cloth):
    """(style, why) for armour on a cloth icon (rule 1)."""
    if (name_key(name), slot) in DECIDED:
        return DECIDED[(name_key(name), slot)]
    if ac <= 0:
        return CLOTH, f"cloth: AC 0, on {cloth}"
    by_name = f.name_style(name)
    if by_name:
        return by_name, f"its name's material (style {by_name}), on {cloth}"
    tier = f.name_tier(name)
    if tier:
        style = f.style_in(slot, tier, level)
        return style, f"its name's kind ({tier}, style {style} at its level), on {cloth}"
    tier = f.tier_by_ac(slot, level, ac, heaviest(mask))
    if tier:
        style = f.style_in(slot, tier, level)
        return style, f"its AC ({tier} at level {level}, style {style}), on {cloth}"
    return CLOTH, f"cloth: no AC to compare, on {cloth}"


def plan(db, only=None):
    """[(patternid, name, (old icon, old model), (new icon, new model), source)], [(patternid, name, why)] left,
    counts of held items left with nothing to draw by kind, and the Facts."""
    rows = db.execute("SELECT patternid, itemname, equipslot, model, itemicon, coalesce(AC, 0), coalesce(levelreq, 0), "
                      "coalesce(classuse, 0), itemdesc, coalesce(craft, 0) FROM itempattern ORDER BY patternid").fetchall()
    ours = set()
    if has_table(db, "style0_look_fix_log"):
        ours = {p for (p,) in db.execute("SELECT patternid FROM style0_look_fix_log")}
    f = Facts(rows, ours)
    gm = gm_patterns(db)
    todo, left = [], []
    bare = collections.Counter()
    for pid, name, slot, model, icon, ac, level, mask, _desc, _craft in rows:
        if model != 0 or (only and pid not in only):
            continue
        new_icon, new_model, src = icon, model, None
        if slot in ARMOUR:
            cloth = f.cloth_icon(slot, icon)
            by_name = f.name_style(name) if ac > 0 else None
            if cloth:
                new_model, src = cloth_look(f, slot, name, ac, level, mask, cloth)
                if new_model != CLOTH and (slot, new_model) in f.icon_of:
                    new_icon = f.icon_of[(slot, new_model)]
            elif (slot, icon) in f.style_of:
                new_model, src = f.style_of[(slot, icon)], f"its icon's style ({f.style_of[(slot, icon)]})"
            elif f.other_kind(slot, icon) and by_name and (slot, by_name) in f.icon_of:
                new_icon, new_model = f.icon_of[(slot, by_name)], by_name
                src = f"its name's material (style {by_name}), on another kind's icon"
            else:
                left.append((pid, name, "no rule reads its icon" + (" (another kind's)" if f.other_kind(slot, icon) else "")))
                continue
        elif slot in HELD_KINDS:
            m = f.model_of.get((HELD_KINDS[slot], icon))
            if not m:
                bare[HELD_KINDS[slot]] += 1
                continue
            new_model, src = m, f"the model its icon's {HELD_KINDS[slot]}s carry"
        else:
            continue
        if pid in gm:
            left.append((pid, name, "a GM changed it"))
            continue
        todo.append((pid, name, (icon, model), (new_icon, new_model), src))
    return todo, left, bare, f


def apply(db, todo):
    db.execute(LOG_DDL)
    done = 0
    for pid, _name, (oi, om), (ni, nm), src in todo:
        cur = db.execute("UPDATE itempattern SET itemicon = ?, model = ? WHERE patternid = ? AND itemicon = ? AND model = ?",
                         (ni, nm, pid, oi, om))
        if cur.rowcount:
            db.execute("INSERT INTO style0_look_fix_log (patternid, old_icon, old_model, new_icon, new_model, source) "
                       "VALUES (?, ?, ?, ?, ?, ?)", (pid, oi, om, ni, nm, src))
            done += 1
    return done


def undo(db, say=print):
    """Put back what the log holds; returns {patternid: (icon, model)} as this had left it, for a re-plan's diff."""
    if not has_table(db, "style0_look_fix_log"):
        say("nothing logged: nothing to undo")
        return {}
    back = kept = 0
    was = {}
    for pid, oi, om, ni, nm in db.execute("SELECT patternid, old_icon, old_model, new_icon, new_model "
                                          "FROM style0_look_fix_log ORDER BY seq DESC").fetchall():
        cur = db.execute("UPDATE itempattern SET itemicon = ?, model = ? WHERE patternid = ? AND itemicon = ? AND model = ?",
                         (oi, om, pid, ni, nm))
        if cur.rowcount:
            back += 1
            was.setdefault(pid, (ni, nm))
        else:
            kept += 1
    db.execute("DROP TABLE style0_look_fix_log")
    say(f"put back {back}; left {kept} changed since")
    return was


def report(todo, left, bare, f, was=None):
    print("plain cloth icons: " + ", ".join(f"slot {s}: {i}" for s, i in sorted(f.plain.items())))
    print("material words: " + ", ".join(
        f"{w} style {s} ({f.word_counts[w][s]}/{sum(f.word_counts[w].values())})" for w, s in sorted(f.word_style.items())))
    print(f"kind words (over {f.n_drops} styled drops): " + ", ".join(
        f"{w} {t} ({f.kind_counts[w][t]}/{sum(f.kind_counts[w].values())})" for w, t in sorted(f.kind_tier.items())))
    by = collections.Counter(re.split(r"[:(,]", src)[0].strip() for *_x, src in todo)
    styles = collections.Counter(nm for _p, _n, _o, (_ni, nm), src in todo if "model its icon" not in src)
    hands = sum(1 for *_x, src in todo if "model its icon" in src)
    print(f"{len(todo)} records change: " + ", ".join(f"{s}: {n}" for s, n in by.most_common()))
    print("  armour styles: " + ", ".join(f"style {k}: {n}" for k, n in sorted(styles.items())) +
          f"; hand models: {hands}")
    if was is not None:
        moved = [(pid, name, was.get(pid), new, src) for pid, name, _old, new, src in todo if was.get(pid) != new]
        print(f"  against what the log had: {len(moved)} records look otherwise, "
              f"{sum(1 for p, *_r in todo if p not in was)} new, {sum(1 for p in was if p not in {t[0] for t in todo})} gone")
        mv = collections.Counter((w[1] if w else None, n[1]) for _p, _n, w, n, _s in moved)
        print("  style moves (before -> after): " + ", ".join(f"{a}->{b}: {k}" for (a, b), k in sorted(mv.items(), key=lambda kv: -kv[1])[:20]))
    for pid, name, (oi, om), (ni, nm), src in todo:
        icon = f", icon {oi} -> {ni}" if oi != ni else ""
        print(f"  {pid} {name!r}: model {om} -> {nm}{icon} ({src})")
    for pid, name, why in left:
        print(f"  left: {pid} {name!r}: {why}")
    if bare:
        print("held items of model 0 on an icon no record of their kind draws, left: " +
              ", ".join(f"{k} {n}" for k, n in sorted(bare.items())))


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--only", type=int, nargs="+", help="only these patternids")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the looks (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply wrote")
    g.add_argument("--replan", action="store_true", help="report the rules' looks for a database this changed (rolled back)")
    g.add_argument("--reapply", action="store_true", help="put back what the log holds and apply the rules, in one transaction")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 10000")
    only = set(a.only) if a.only else None
    if a.undo:
        db.execute("BEGIN IMMEDIATE")
        undo(db)
        db.execute("COMMIT")
        return
    if a.replan or a.reapply:
        db.execute("BEGIN IMMEDIATE")
        was = undo(db)
        todo, left, bare, f = plan(db, only)
        report(todo, left, bare, f, was)
        if a.replan:
            db.execute("ROLLBACK")
            print("report only (rolled back): --reapply to write")
            return
        done = apply(db, todo)
        db.execute("COMMIT")
        print(f"wrote {done} records")
        return
    todo, left, bare, f = plan(db, only)
    report(todo, left, bare, f)
    if not a.apply:
        print("report only: --apply to write")
        return
    db.execute("BEGIN IMMEDIATE")
    done = apply(db, todo)
    db.execute("COMMIT")
    print(f"wrote {done} records")


if __name__ == "__main__":
    main()
