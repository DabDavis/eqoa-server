#!/usr/bin/env python3
"""Gear's icons and armour styles put right (the user, 2026-11-02: "on Jedd, woolen helm is wrong model and
icon", then "also look for others with same issue").

An item's icon (itempattern.itemicon, an ITEMICON.ESF surface) is the picture the inventory shows, and an
armour piece's style (itempattern.model, wear slots 1 5 7 10 11 19) is the armour set the character is drawn
in (handlers/character.go wornLook: the state channel's armour bytes; a helm's style picks one of CLIENT's
eight helms, VICSpriteCust $Helm, and style 0 draws none). Three kinds of record are changed:

1. **The woolen helm and the woolen gloves** (the user's word for the helm; the gloves are the set's other such
   piece): the woolen set is the tailors' cloth of level 15, worn by every class, and its tunic, bracers, legs
   and boots are style 1 (cloth) with the cloth icons, as the burlap, padded, silk and gossamer sets are whole;
   the helm and the gloves were style 2 (leather) with the leather icons. They take style 1 and the icon the
   style 1 pieces of their slot carry most. (Sandstorm's records give these two the leather icons too.)
2. **An icon Sandstorm's record of the same item gives otherwise** (the harvests: a gear record of the same
   name, the only one of it): its icon. An armour piece of style 0 whose new icon is a style's (below) takes
   that style too, so it is drawn as its icon shows. An armour piece of a style whose icon is that style's,
   where Sandstorm's icon is another style's, is listed and left: the look and the icon would disagree, and
   neither record says which is right.
3. Nothing else: the armour pieces of style 0 on their slot's plain cloth icon (the old dump's records, about
   1,450 on eqoaclassic, of which Sandstorm gives a different icon to 13 of the 18 it holds) are counted, not
   changed: no record says what they are drawn as.

A style's icon: an icon at least three records of a slot carry, nine in ten of those with a style carrying
that one. A record a GM edited in the panel (gm_audit) is listed and left.

Every change is logged in item_look_fix_log with what it replaced; --undo puts back each record still as this
left it, then drops the log. Report only by default. The server reads a record's style at each equip and
login; CLIENT draws the icon off the record it got at its login: no restart, a player sees both at the next
login.

    tools/fix_item_looks.py [--db eqoa.db] --prices <harvest JSONs or folders> ... [--apply | --undo]
"""
import argparse
import collections
import json
import os
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from fix_dump_weapons import key  # noqa: E402
from fix_wear_slots import sandstorm  # noqa: E402

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")

ARMOUR = (1, 5, 7, 10, 11, 19)          # head, chest, bracers, legs, feet, hands: the drawn armour sets
GEAR = (1, 19)
STYLE_MIN, STYLE_SHARE = 3, 0.9
CLOTH = 1
# (name, wear slot, style): the user's word, 2026-11-02, and the same set's other such piece
DECIDED = [("woolen helm", 1, CLOTH, "the user's word: the woolen set is cloth"),
           ("woolen gloves", 19, CLOTH, "the woolen set is cloth (its other leather piece)")]

LOG_DDL = """CREATE TABLE IF NOT EXISTS item_look_fix_log (
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


def style_icons(rows):
    """{(slot, icon): style} for the icons of a style, and {(slot, style): its commonest icon}."""
    seen = collections.defaultdict(collections.Counter)
    for _pid, _name, slot, model, icon in rows:
        if slot in ARMOUR and model:
            seen[(slot, icon)][model] += 1
    style_of = {}
    for (slot, icon), c in seen.items():
        m, n = c.most_common(1)[0]
        if n >= STYLE_MIN and n >= STYLE_SHARE * sum(c.values()):
            style_of[(slot, icon)] = m
    icon_of = {}
    by = collections.defaultdict(collections.Counter)
    for _pid, _name, slot, model, icon in rows:
        if slot in ARMOUR and model:
            by[(slot, model)][icon] += 1
    for k, c in by.items():
        icon_of[k] = c.most_common(1)[0][0]
    return style_of, icon_of


def plan(db, ss):
    """[(patternid, name, (old icon, old style), (new icon, new style), source)] and [(name, why)] left, and
    the style 0 armour counted by slot."""
    rows = db.execute("SELECT patternid, itemname, equipslot, model, itemicon FROM itempattern ORDER BY patternid").fetchall()
    style_of, icon_of = style_icons(rows)
    gm = gm_patterns(db)
    decided = {(n, slot): (style, why) for n, slot, style, why in DECIDED}
    todo, left = [], []
    plain = collections.defaultdict(lambda: [0, 0])     # slot -> [style 0 on a style 1 icon, style 0 otherwise]
    for pid, name, slot, model, icon in rows:
        if not (GEAR[0] <= slot <= GEAR[1]):
            continue
        k = key(name)
        new_icon, new_model, src = icon, model, None
        if (k, slot) in decided:
            style, why = decided[(k, slot)]
            if model != style or icon_of.get((slot, style), icon) != icon:
                new_icon, new_model, src = icon_of.get((slot, style), icon), style, why
        elif k in ss and GEAR[0] <= ss[k][0] <= GEAR[1] and ss[k][1] and ss[k][1] != icon:
            s_icon = ss[k][1]
            s_style = style_of.get((slot, s_icon))
            if slot in ARMOUR and model and style_of.get((slot, icon)) == model and s_style not in (None, model):
                left.append((name, f"Sandstorm's icon is style {s_style}'s, its style {model} and its icon agree"))
                continue
            new_icon, src = s_icon, "Sandstorm's icon"
            if slot in ARMOUR and model == 0 and s_style:
                new_model, src = s_style, f"Sandstorm's icon, which is style {s_style}'s"
        if src is None:
            if slot in ARMOUR and model == 0:
                plain[slot][0 if style_of.get((slot, icon)) == CLOTH else 1] += 1
            continue
        if pid in gm:
            left.append((name, "a GM changed it"))
            continue
        todo.append((pid, name, (icon, model), (new_icon, new_model), src))
    return todo, left, dict(plain)


def apply(db, todo):
    db.execute(LOG_DDL)
    done = 0
    for pid, _name, (oi, om), (ni, nm), src in todo:
        cur = db.execute("UPDATE itempattern SET itemicon = ?, model = ? WHERE patternid = ? AND itemicon = ? AND model = ?",
                         (ni, nm, pid, oi, om))
        if cur.rowcount:
            db.execute("INSERT INTO item_look_fix_log (patternid, old_icon, old_model, new_icon, new_model, source) "
                       "VALUES (?, ?, ?, ?, ?, ?)", (pid, oi, om, ni, nm, src))
            done += 1
    return done


def undo(db):
    if not has_table(db, "item_look_fix_log"):
        print("nothing logged: nothing to undo")
        return
    back = kept = 0
    for pid, oi, om, ni, nm in db.execute("SELECT patternid, old_icon, old_model, new_icon, new_model FROM item_look_fix_log "
                                          "ORDER BY seq DESC").fetchall():
        cur = db.execute("UPDATE itempattern SET itemicon = ?, model = ? WHERE patternid = ? AND itemicon = ? AND model = ?",
                         (oi, om, pid, ni, nm))
        back, kept = (back + 1, kept) if cur.rowcount else (back, kept + 1)
    db.execute("DROP TABLE item_look_fix_log")
    print(f"put back {back}; left {kept} changed since")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--prices", nargs="*", default=[], help="eqoa harvest JSON files or folders of them (Sandstorm's records)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the icons and styles (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply wrote")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 10000")
    if a.undo:
        db.execute("BEGIN IMMEDIATE")
        undo(db)
        db.execute("COMMIT")
        return
    if not a.prices:
        sys.exit("--prices: Sandstorm's harvests are what the icons are taken from")
    ss = sandstorm(a.prices)[0]
    todo, left, plain = plan(db, ss)
    by = collections.Counter(src.split(",")[0] for *_x, src in todo)
    print(f"{len(todo)} records change: " + ", ".join(f"{s}: {n}" for s, n in by.most_common()))
    for pid, name, (oi, om), (ni, nm), src in todo:
        style = f", style {om} -> {nm}" if om != nm else ""
        print(f"  {pid} {name!r}: icon {oi} -> {ni}{style} ({src})")
    for name, why in left:
        print(f"  left: {name!r}: {why}")
    print("armour of style 0 (draws nothing), left: " + ", ".join(
        f"slot {s}: {a0 + b0} ({a0} on the cloth icon)" for s, (a0, b0) in sorted(plain.items())))
    if not a.apply:
        print("report only: --apply to write")
        return
    db.execute("BEGIN IMMEDIATE")
    done = apply(db, todo)
    db.execute("COMMIT")
    print(f"wrote {done} records")


if __name__ == "__main__":
    main()
