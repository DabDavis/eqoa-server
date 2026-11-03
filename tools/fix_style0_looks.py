#!/usr/bin/env python3
"""Gear that draws nothing when worn given a look (the user, 2026-11-03, on a troll warrior: "i equipped basher
shirt and no graphic when equipped. the icon is the cloth tunic, so lets make it that. also look for others").

An armour piece's style (itempattern.model, wear slots 1 5 7 10 11 19) is the armour set the character is drawn
in (handlers/character.go wornLook; style 0 draws none, on the head no helm); a shield's, weapon's or held item's
model (wear slots 12-18 and 20) is the ITEM.ESF sprite in the hand, and 0 draws nothing. The old dump's records
came with neither: on eqoaclassic 1,437 armour pieces of style 0, 1,392 of them on their slot's plain cloth icon
(the Basher Shirt, a troll warrior's first chest, among them), and 300-odd shields, weapons and held items of
model 0. tools/fix_item_looks.py (2026-11-02) only counted them: no record says how they were drawn. The user's
rule is the icon's. Changed:

1. **Armour on a cloth icon**: its slot's plain icon (the icon the slot's style 1 records carry most), an icon
   every styled record of the slot carrying it has style 1 (the cloth caps'), or an icon robes (wear 2) carry most
   (a tunic on a robe's picture): cloth (style 1), as its icon shows, unless its name says another material
   (rule 3).
2. **Armour on an icon of a style** (at least 3 records of its slot carry it with a style, 9 in 10 of those the
   same, fix_item_looks.py's rule): that style (the Gust set's scale icons, the Gladestrider bracers').
3. **Its name's material**, for armour of AC above 0 on a cloth icon or on another kind's icon (one no armour
   record of its slot carries with a style, carried most by records of another wear slot: the Brazen Chain
   Bracers on the earrings' picture): a word of MATERIAL_WORDS that this database's own styled armour gives one
   style (at least 8 records, 9 in 10 of them the same; the report prints each word's count) gives that style,
   and the icon the slot's records of that style carry most, so the picture and the look agree (the Etheric
   Plate Helm, on the cloth cap's icon, becomes a plate helm with the plate helm's icon). A name with words of
   two styles says nothing.
4. **A shield, weapon or held item of model 0** on an icon that the other records of its kind (shields;
   one-handed, either-hand and two-handed weapons; bows; thrown; held items, with the weapons; fishing poles)
   carry with one model (at least 3, 9 in 10 of them): that model, which those records are already drawn with.
   The held items' stand-in picture is a wand's, drawn as the wand it shows (136 books, orbs, totems and horns
   on eqoaclassic): the icon decides, as for the armour.

Left, and listed: armour on an icon no rule reads (crowns, circlets and headbands on icons of their own, which
no styled record shares; a necklace's picture with no material in the name), and anything a GM edited
(gm_audit). Held items on an icon no record draws are counted (books, gems, the alchemist's thrown potions).

Every change is logged in style0_look_fix_log with what it replaced; --undo puts back each record still as this
left it, then drops the log. Report only by default; --only limits a run to the records named. The server
reads a record's look at each equip and login, CLIENT its icon at its login: no restart. A player sees the look
at the next equip (take it off and put it back on) or login, the new icon at the next login.

    tools/fix_style0_looks.py [--db eqoa.db] [--only PATTERNID ...] [--apply | --undo]
"""
import argparse
import collections
import json
import os
import re
import sqlite3
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")

ARMOUR = (1, 5, 7, 10, 11, 19)          # head, chest, bracers, legs, feet, hands: the drawn armour sets
ROBE = 2
CLOTH = 1
STYLE_MIN, STYLE_SHARE = 3, 0.9
WORD_MIN, WORD_SHARE = 8, 0.9
MODEL_MIN, MODEL_SHARE = 3, 0.9
# words for what armour is made of; each counts only where the database's styled armour gives it one style
MATERIAL_WORDS = ("cloth", "silk", "silken", "wool", "woolen", "woollen", "burlap", "linen", "cotton", "velvet",
                  "satin", "leather", "skin", "hide", "pelt", "fur", "studded", "chain", "chainmail", "ringmail",
                  "mail", "scale", "scaled", "scalemail", "plate", "plates", "platemail", "banded", "bone")
# the hands' kinds (wear slots): a model is taken only off records of the same kind, a held item's off held
# items and weapons too (a wand is either: the held items' stand-in icon is the wands' picture)
HELD_KINDS = {13: "shield", 12: "weapon", 14: "weapon", 15: "weapon", 16: "bow", 17: "thrown", 18: "held",
              20: "fishing"}
MODEL_FROM = {"held": ("held", "weapon")}

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


def words(name):
    return set(re.findall(r"[a-z]+", (name or "").lower()))


def commonest(counter, least, share):
    """The counter's top key when it holds at least `least` and `share` of all, else None."""
    if not counter:
        return None
    k, n = counter.most_common(1)[0]
    return k if n >= least and n >= share * sum(counter.values()) else None


class Facts:
    """What the database's own styled records say about icons, words and models."""

    def __init__(self, rows):
        styled = collections.defaultdict(collections.Counter)      # (slot, icon) -> {style: n}
        by_style = collections.defaultdict(collections.Counter)    # (slot, style) -> {icon: n}
        slots_of = collections.defaultdict(collections.Counter)    # icon -> {wear slot: n}
        word_styles = collections.defaultdict(collections.Counter)
        models = collections.defaultdict(collections.Counter)      # (kind, icon) -> {model: n}
        for _pid, name, slot, model, icon, _ac in rows:
            slots_of[icon][slot] += 1
            if slot in ARMOUR and model:
                styled[(slot, icon)][model] += 1
                by_style[(slot, model)][icon] += 1
                for w in words(name) & set(MATERIAL_WORDS):
                    word_styles[w][model] += 1
            if slot in HELD_KINDS and model:
                for kind, srcs in [(k, MODEL_FROM.get(k, (k,))) for k in set(HELD_KINDS.values())]:
                    if HELD_KINDS[slot] in srcs:
                        models[(kind, icon)][model] += 1
        self.styled, self.slots_of = styled, slots_of
        self.icon_of = {k: c.most_common(1)[0][0] for k, c in by_style.items()}
        self.style_of = {k: s for k, c in styled.items() if (s := commonest(c, STYLE_MIN, STYLE_SHARE))}
        self.plain = {slot: self.icon_of[(slot, CLOTH)] for slot in ARMOUR if (slot, CLOTH) in self.icon_of}
        self.word_style = {w: s for w, c in word_styles.items() if (s := commonest(c, WORD_MIN, WORD_SHARE))}
        self.word_counts = word_styles
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


def plan(db, only=None):
    """[(patternid, name, (old icon, old model), (new icon, new model), source)], [(name, why)] left, and counts
    of held items left with nothing to draw, by kind."""
    rows = db.execute("SELECT patternid, itemname, equipslot, model, itemicon, coalesce(AC, 0) FROM itempattern "
                      "ORDER BY patternid").fetchall()
    f = Facts(rows)
    gm = gm_patterns(db)
    todo, left = [], []
    bare = collections.Counter()
    for pid, name, slot, model, icon, ac in rows:
        if model != 0 or (only and pid not in only):
            continue
        new_icon, new_model, src = icon, model, None
        if slot in ARMOUR:
            cloth = f.cloth_icon(slot, icon)
            by_name = f.name_style(name) if ac > 0 else None
            if cloth and by_name and by_name != CLOTH and (slot, by_name) in f.icon_of:
                new_icon, new_model = f.icon_of[(slot, by_name)], by_name
                src = f"its name's material (style {by_name}), on {cloth}"
            elif cloth:
                new_model, src = CLOTH, f"cloth: {cloth}"
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


def undo(db):
    if not has_table(db, "style0_look_fix_log"):
        print("nothing logged: nothing to undo")
        return
    back = kept = 0
    for pid, oi, om, ni, nm in db.execute("SELECT patternid, old_icon, old_model, new_icon, new_model "
                                          "FROM style0_look_fix_log ORDER BY seq DESC").fetchall():
        cur = db.execute("UPDATE itempattern SET itemicon = ?, model = ? WHERE patternid = ? AND itemicon = ? AND model = ?",
                         (oi, om, pid, ni, nm))
        back, kept = (back + 1, kept) if cur.rowcount else (back, kept + 1)
    db.execute("DROP TABLE style0_look_fix_log")
    print(f"put back {back}; left {kept} changed since")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--only", type=int, nargs="+", help="only these patternids")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the looks (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply wrote")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 10000")
    if a.undo:
        db.execute("BEGIN IMMEDIATE")
        undo(db)
        db.execute("COMMIT")
        return
    todo, left, bare, f = plan(db, set(a.only) if a.only else None)
    print("plain cloth icons: " + ", ".join(f"slot {s}: {i}" for s, i in sorted(f.plain.items())))
    print("material words: " + ", ".join(
        f"{w} style {s} ({f.word_counts[w][s]}/{sum(f.word_counts[w].values())})" for w, s in sorted(f.word_style.items())))
    by = collections.Counter(src.split(":")[0].split(" (")[0].split(",")[0] for *_x, src in todo)
    styles = collections.Counter(nm for _p, _n, _o, (_ni, nm), src in todo if "model its icon" not in src)
    hands = sum(1 for *_x, src in todo if "model its icon" in src)
    print(f"{len(todo)} records change: " + ", ".join(f"{s}: {n}" for s, n in by.most_common()))
    print("  armour styles: " + ", ".join(f"style {k}: {n}" for k, n in sorted(styles.items())) +
          f"; hand models: {hands}")
    for pid, name, (oi, om), (ni, nm), src in todo:
        icon = f", icon {oi} -> {ni}" if oi != ni else ""
        print(f"  {pid} {name!r}: model {om} -> {nm}{icon} ({src})")
    for pid, name, why in left:
        print(f"  left: {pid} {name!r}: {why}")
    if bare:
        print("held items of model 0 on an icon no record of their kind draws, left: " +
              ", ".join(f"{k} {n}" for k, n in sorted(bare.items())))
    if not a.apply:
        print("report only: --apply to write")
        return
    db.execute("BEGIN IMMEDIATE")
    done = apply(db, todo)
    db.execute("COMMIT")
    print(f"wrote {done} records")


if __name__ == "__main__":
    main()
