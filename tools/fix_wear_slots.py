#!/usr/bin/env python3
"""Items in wear slot 0 given the slot they are worn in (the user,
2026-10-29: "fix all 511").

Slot 0 is no slot:
- CLIENT's VIItem calls it "empty".
- Sandstorm's records use none: -1 is not worn, 1 helm, 2 robe, 3 earring,
  4 neck, 5 chest, 6 bracelet, 7 bracers, 8 ring, 9 belt, 10 legs, 11 boots,
  12-16 the hands (13 the shield), 18 held, 19 gloves.
- The server's wear table (handlers/equip.go itemSlotToEquipSlot) has no
  entry for it, so it refuses to put one on.

The old dump left 511 items there (eqoaclassic), all on one stand-in icon
(-570739083, which no Sandstorm item carries). The weapons among them go to
their hand slots by tools/fix_dump_weapons.py. This takes the rest (no attack
type and no damage, or named as armour: tools/fix_dump_weapons.py's
ARMOUR_WORDS). Each takes the first of these that has its slot:
1. **Sandstorm's record** (--prices: the harvests' wares), by name.
2. **The Allakhazam list it is on** (--pages: tools/fetch_item_pages.py's
   lists by slot, its players' submissions). Head 1, Robe 2, Ear 3, Neck 4,
   Chest 5, Finger 8, Waist 9, Legs 10, Feet 11, Held 18, Hands 19,
   Inventory -1 (not worn). Arms is 7 (bracers), or 6 when the name says one
   wrist (a bracelet, bangle, armlet or cuff). A shield on any list is 13.
3. **What its name says** (OURS): a shield, targe or buckler 13; bracers,
   vambraces, armwraps, shackles, chains 7; a bracelet, bangle, armlet, cuff
   6 (an armband too); a ring, band, signet 8; greaves, leggings 10;
   wristguards 7.
Anything else is left and listed.

Its icon, when it is the stand-in (the icon most slot 0 items carry, when no
Sandstorm item has it), becomes its own Sandstorm record's, else the commonest
icon of Sandstorm's items of its new slot, else this server's (OURS: a
stand-in of the right kind).

Every change is logged in wear_slots_fix_log (the pattern, its values before
and after, the source); --undo puts back each one still as this left it and
drops the log. Report only by default. The server reads the record at each
equip, so an item can be put on at once; a client files it and draws its icon
from the item records it gets at its login.

    tools/fix_wear_slots.py [--db eqoa.db] --pages <folder> --prices <harvests> ... [--apply | --undo]
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
from fix_dump_weapons import ARMOUR_WORDS, key  # noqa: E402

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
# Allakhazam's slot lists, as this server numbers its wear slots
LIST_SLOTS = {"Head": 1, "Robe": 2, "Ear": 3, "Neck": 4, "Chest": 5, "Arms": 7, "Finger": 8, "Waist": 9, "Legs": 10,
              "Feet": 11, "Held": 18, "Hands": 19, "Inventory": -1}
SHIELD = r"\b(?:shields?|targes?|bucklers?)\b"
ONE_WRIST = r"\b(?:bracelets?|braclets?|bangles?|armlets?|cuffs?|wristbands?)\b"
# OURS: what a name says
NAME_SLOTS = [(SHIELD, 13), (r"\b(?:bracers?|vambraces?|armwraps?|wraps|shackles?|chains|wristguards?)\b", 7),
              (ONE_WRIST, 6), (r"\barmbands?\b", 6),
              (r"\b(?:rings?|bands?|signets?)\b", 8), (r"\b(?:greaves|leggings)\b", 10)]

LOG_SCHEMA = """CREATE TABLE wear_slots_fix_log (
    patternid INTEGER PRIMARY KEY,
    before    TEXT NOT NULL,
    after     TEXT NOT NULL,
    source    TEXT NOT NULL,   -- sandstorm | allakhazam <list> | name, and the icon's
    applied   TEXT NOT NULL
)"""


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def sandstorm(paths):
    """Sandstorm's items by name: {name: (wear slot, icon)}, a name of two
    records left out; and every icon its items carry."""
    files = []
    for p in paths:
        files += glob.glob(os.path.join(p, "**", "*.json"), recursive=True) if os.path.isdir(p) else [p]
    seen = collections.defaultdict(set)
    icons = set()

    def walk(o):
        if isinstance(o, dict):
            if o.get("name") and "wear_slot" in o and "icon" in o:
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
            icons.add(w["icon"])
            seen[key(w["name"])].add((w["wear_slot"], w["icon"]))
    return {k: next(iter(v)) for k, v in seen.items() if len(v) == 1}, icons


def allakhazam(folder):
    """{name: slot word} off Allakhazam's lists by slot (the fetched pages
    themselves, lists/slot-<word>.html); a name on two lists left out."""
    if not folder:
        return {}
    seen = collections.defaultdict(set)
    for path in glob.glob(os.path.join(folder, "lists", "slot-*.html")):
        word = os.path.basename(path)[len("slot-"):-len(".html")]
        for _iid, name in fetch_item_pages.read_list(fetch_item_pages.read_file(path)):
            seen[key(name)].add(word)
    return {k: v.pop() for k, v in seen.items() if len(v) == 1}


def slot_of(k, ss, ak):
    """(slot, source) of a slot 0 item by its name, or (None, "")."""
    if k in ss and ss[k][0] not in (0, None):
        return ss[k][0], "sandstorm"
    if k in ak:
        word = ak[k]
        if re.search(SHIELD, k):
            return 13, f"allakhazam {word}: a shield"
        if word == "Arms" and re.search(ONE_WRIST, k):
            return 6, "allakhazam Arms: one wrist"
        if word in LIST_SLOTS:
            return LIST_SLOTS[word], f"allakhazam {word}"
    for pat, slot in NAME_SLOTS:
        if re.search(pat, k):
            return slot, "name"
    return None, ""


def plan(db, ss, ss_icons, ak):
    skip = set()
    if has_table(db, "gm_audit"):
        for (k,) in db.execute("SELECT rowkey FROM gm_audit WHERE tbl = 'itempattern'"):
            try:
                skip.add(int(json.loads(k)["patternid"]))
            except (ValueError, KeyError, TypeError):
                pass
    top = db.execute("SELECT itemicon, count(*) FROM itempattern WHERE equipslot = 0 GROUP BY itemicon "
                     "ORDER BY 2 DESC LIMIT 1").fetchone()
    total = db.execute("SELECT count(*) FROM itempattern WHERE equipslot = 0").fetchone()[0]
    stand_in = top[0] if top and top[1] * 2 > total and top[0] not in ss_icons else None
    ss_slot_icon = collections.defaultdict(collections.Counter)
    for slot, icon in ss.values():
        if icon and icon != stand_in:
            ss_slot_icon[slot][icon] += 1
    our_slot_icon = collections.defaultdict(collections.Counter)
    for slot, icon in db.execute("SELECT equipslot, itemicon FROM itempattern WHERE equipslot NOT IN (0, -1)"):
        if icon and icon != stand_in:
            our_slot_icon[slot][icon] += 1

    out, left = [], []
    for pid, name, icon, at, dmg in db.execute("SELECT patternid, itemname, itemicon, attacktype, weapondamage "
                                               "FROM itempattern WHERE equipslot = 0 ORDER BY patternid").fetchall():
        k = key(name)
        if (at or dmg) and not re.search(ARMOUR_WORDS, k):
            continue  # a weapon: tools/fix_dump_weapons.py's
        if pid in skip:
            left.append((name, "a GM changed it"))
            continue
        slot, source = slot_of(k, ss, ak)
        if slot is None:
            left.append((name, "nothing says where it is worn"))
            continue
        before, after = {"equipslot": 0}, {"equipslot": slot}
        if stand_in is not None and icon == stand_in and slot != -1:
            if k in ss and ss[k][1] and ss[k][1] != stand_in:
                new, why = ss[k][1], "its Sandstorm icon"
            elif ss_slot_icon.get(slot):
                new, why = ss_slot_icon[slot].most_common(1)[0][0], f"the icon of Sandstorm's slot {slot} items"
            elif our_slot_icon.get(slot):
                new, why = our_slot_icon[slot].most_common(1)[0][0], f"the icon of this server's slot {slot} items"
            else:
                new, why = None, ""
            if new:
                before["itemicon"], after["itemicon"] = icon, new
                source += "; " + why
        out.append((pid, name, before, after, source))
    return out, left, stand_in


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--pages", help="tools/fetch_item_pages.py's folder (Allakhazam's lists by slot)")
    ap.add_argument("--prices", nargs="*", default=[], help="eqoa harvest JSON files or folders of them")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the slots (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply changed")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        if not has_table(db, "wear_slots_fix_log"):
            print("nothing logged: nothing to undo")
            return
        db.execute("BEGIN IMMEDIATE")
        back = left = 0
        for pid, before, after in db.execute("SELECT patternid, before, after FROM wear_slots_fix_log").fetchall():
            before, after = json.loads(before), json.loads(after)
            c = list(after)
            now = db.execute(f"SELECT {', '.join(c)} FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
            if now is None or dict(zip(c, now)) != after:
                left += 1
                continue
            db.execute(f"UPDATE itempattern SET {', '.join(x + ' = ?' for x in c)} WHERE patternid = ?",
                       [before[x] for x in c] + [pid])
            back += 1
        db.execute("DROP TABLE wear_slots_fix_log")
        db.execute("COMMIT")
        print(f"put back {back} items, {left} changed since left as they are")
        return
    if a.apply and has_table(db, "wear_slots_fix_log"):
        sys.exit("wear_slots_fix_log is there: applied already (--undo first)")
    ss, ss_icons = sandstorm(a.prices)
    if not ss:
        sys.exit("no Sandstorm item records: give --prices (eqoa harvest JSON)")
    ak = allakhazam(a.pages)
    changes, left, stand_in = plan(db, ss, ss_icons, ak)
    print(f"slot 0's stand-in icon: {stand_in}" if stand_in is not None else "no stand-in icon in slot 0")
    for pid, name, before, after, source in changes:
        print(f"{pid:7d} {name}: " + ", ".join(f"{c} {before[c]} -> {after[c]}" for c in after) + f" ({source})")
    for name, why in left:
        print(f"left as it is: {name} ({why})")
    by = collections.Counter(c[4].split(";")[0].split(":")[0].split()[0] for c in changes)
    slots = collections.Counter(c[3]["equipslot"] for c in changes)
    print(f"{len(changes)} items given a slot (" + ", ".join(f"{n} by {s}" for s, n in by.most_common())
          + "; to slots " + ", ".join(f"{s}: {n}" for s, n in sorted(slots.items())) + f"), {len(left)} left")
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
                db.execute("INSERT INTO wear_slots_fix_log VALUES (?, ?, ?, ?, ?)",
                           (pid, json.dumps(before, sort_keys=True), json.dumps(after, sort_keys=True), source, when))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"changed {len(changes)} items; they can be put on at once, filed and drawn at a player's next login")


if __name__ == "__main__":
    main()
