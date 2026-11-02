#!/usr/bin/env python3
"""Belts out of the robe slot and robes out of the belt slot (the user, 2026-11-02: "bowline belt is saying it
is Equip: Robe", then "also look for others with same issue").

Return Home's import (tools/import_prh_content.py, 2026-10-21) copied its wear slots as they stood, and among
its records are belts, sashes and girdles in wear slot 2 (robe) and robes in wear slot 9 (belt): on
eqoaclassic 173 and 51, the Bowline Belt among the first. The item window says "Equip: Robe" for such a belt
(CLIENT reads the wear slot off the record), the server's equip puts it in the robe slot (handlers/equip.go:
wear 2 goes in equip slot 21) and draws it as a robe of style 0 (handlers/character.go wornLook: wear 2 is a
robe), and a robe in slot 9 goes on as a belt and draws nothing. Sandstorm's records (the harvests) and
Allakhazam's item lists (tools/fetch_item_pages.py) hold 19 and 109 of these items, and every one of them is
in the slot its icon says.

The rule:
- In wear slots 2 and 9 an item's ICON says which it is. The icons there fall into two families, found from
  the names in those two slots: an icon whose items are named as belts (the head noun, before any " of ",
  a belt, sash, girdle, cord, cincture, strap or buckle) three times as often as robes is a belt's, and one
  named as robes (robe, gown, shroud, vestment, raiment, drape, cloak, mantle, smock, rags, tabard, uniform,
  garb, shawl) three times as often a robe's. On eqoaclassic 9 icons are belts', 11 robes', none both.
- An item whose icon is of neither family goes by what its description calls it ("This belt has an ingot
  ...": the Ingot of Vitality), else it is listed and left.
- With --prices and --pages, an item a Sandstorm record or an Allakhazam list places otherwise than its icon
  is listed and left; and an item outside the two slots goes to the belt or the robe slot when its Sandstorm
  record, or every Allakhazam list naming it, says so and its icon is of that family (the Healthy Belt, in the
  chest slot: Allakhazam's Waist).
- A record a GM edited in the panel (gm_audit) is listed and left.
- A worn copy goes with its record: out of the robe slot (equip slot 21) into the belt slot (11), or the other
  way, when its owner wears nothing there after the move, else into the pack (equiploc -1). Its owner sees it
  at the next login.

Every change is logged in belt_robe_slot_fix_log with what it replaced; --undo puts back each row still as
this left it, then drops the log. Report only by default. The server reads item records as it goes: no
restart (a player sees the slot at the next login, when the client gets its item records).

    tools/fix_belt_robe_slots.py [--db eqoa.db] [--prices <harvest JSONs or folders> ...] [--pages <folder>]
                                 [--apply | --undo]
"""
import argparse
import collections
import json
import os
import re
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from fix_dump_weapons import key  # noqa: E402
from fix_wear_slots import allakhazam, sandstorm  # noqa: E402

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")

ROBE, BELT = 2, 9                       # wear slots (itempattern.equipslot)
EQUIP_OF = {ROBE: 21, BELT: 11}         # the equip slot each goes in (handlers/equip.go itemSlotToEquipSlot)
BELT_WORDS = r"belts?|sash(?:es)?|girdles?|cords?|cinctures?|straps?|buckles?"
ROBE_WORDS = (r"robes?|gowns?|shrouds?|vestments?|raiments?|drapes?|cloaks?|mantles?|smocks?|rags|tabb?ards?|"
              r"uniforms?|garb|shawls?")
DOMINANCE = 3                           # an icon's names this many to one make it a family's
LIST_SLOT = {"Robe": ROBE, "Waist": BELT}

LOG_DDL = """CREATE TABLE IF NOT EXISTS belt_robe_slot_fix_log (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,     -- itempattern (equipslot) | charinventory (equiploc)
    rid     INTEGER NOT NULL,  -- the pattern or the inventory row
    old     INTEGER NOT NULL,
    new     INTEGER NOT NULL,
    source  TEXT NOT NULL,
    applied TEXT NOT NULL DEFAULT (datetime('now'))
)"""


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def kind_of_words(text):
    """BELT or ROBE by the head noun of a name: its last piece word before any " of " (a "Belt of the
    Harpies" is a belt, a "Robe of the Belt" a robe), or None."""
    head = (text or "").lower().split(" of ")[0]
    for w in reversed(re.findall(r"[a-z']+", head)):
        if re.fullmatch(ROBE_WORDS, w):
            return ROBE
        if re.fullmatch(BELT_WORDS, w):
            return BELT
    return None


def kind_of_desc(desc):
    """BELT or ROBE by what a description calls the item, when it names only one of the two."""
    d = (desc or "").lower()
    belt = re.search(r"\b(?:" + BELT_WORDS + r")\b", d)
    robe = re.search(r"\b(?:" + ROBE_WORDS + r")\b", d)
    if belt and not robe:
        return BELT
    if robe and not belt:
        return ROBE
    return None


def families(rows):
    """{icon: BELT or ROBE} from the names of the items in the two slots."""
    votes = collections.defaultdict(collections.Counter)
    for _pid, name, slot, icon, _desc in rows:
        if slot in (ROBE, BELT):
            k = kind_of_words(name)
            if k:
                votes[icon][k] += 1
    out = {}
    for icon, c in votes.items():
        if c[BELT] >= DOMINANCE * max(c[ROBE], 1) or (c[ROBE] == 0 and c[BELT] > 0):
            out[icon] = BELT
        elif c[ROBE] >= DOMINANCE * max(c[BELT], 1) or (c[BELT] == 0 and c[ROBE] > 0):
            out[icon] = ROBE
    return out


def gm_patterns(db):
    """The patterns a GM edited in the panel."""
    out = set()
    if not has_table(db, "gm_audit"):
        return out
    for (k,) in db.execute("SELECT rowkey FROM gm_audit WHERE tbl = 'itempattern'"):
        try:
            out.add(int(json.loads(k)["patternid"]))
        except (ValueError, KeyError, TypeError):
            pass
    return out


def measured(k, ss, ak):
    """(slot, source) a Sandstorm record or Allakhazam's lists give a name in the two slots' terms, or
    (None, '') when neither places it as a belt or a robe."""
    if k in ss and ss[k][0] in (ROBE, BELT):
        return ss[k][0], "Sandstorm's record"
    if k in ak and ak[k] in LIST_SLOT:
        return LIST_SLOT[ak[k]], f"Allakhazam's {ak[k]} list"
    return None, ""


def plan(db, ss=None, ak=None):
    """The pattern moves [(patternid, name, old slot, new slot, source)], the items left [(name, why)], the
    families, and how the measured records agree {source: [agree, disagree]}."""
    ss, ak = ss or {}, ak or {}
    rows = db.execute("SELECT patternid, itemname, equipslot, itemicon, itemdesc FROM itempattern "
                      "ORDER BY patternid").fetchall()
    fam = families(rows)
    skip = gm_patterns(db)
    moves, left = [], []
    agree = collections.defaultdict(lambda: [0, 0])
    for pid, name, slot, icon, desc in rows:
        k = key(name)
        m_slot, m_src = measured(k, ss, ak)
        if slot in (ROBE, BELT):
            want, src = fam.get(icon), "its icon's family"
            if want is None:
                want, src = kind_of_desc(desc), "its description"
            if m_slot is not None and want is not None:
                agree[m_src.split("'")[0]][0 if m_slot == want else 1] += 1
            if want is None:
                left.append((name, "neither its icon nor its description says belt or robe"))
                continue
            if m_slot is not None and m_slot != want:
                left.append((name, f"{m_src} says slot {m_slot}, its icon {want}"))
                continue
        elif m_slot is not None and fam.get(icon) == m_slot and slot not in (-1, 0):
            want, src = m_slot, f"{m_src}, its icon's family"
        else:
            continue
        if want == slot:
            continue
        if pid in skip:
            left.append((name, "a GM changed it"))
            continue
        moves.append((pid, name, slot, want, src))
    return moves, left, fam, dict(agree)


def worn_moves(db, moves):
    """Each worn copy of a moved pattern: [(itemid, serverid, name, old equiploc, new equiploc)], the new
    one its new wear slot's equip slot when its owner wears nothing there after every move, else -1."""
    new_slot = {pid: new for pid, _name, _old, new, _src in moves}
    if not new_slot:
        return []
    marks = ",".join("?" * len(new_slot))
    worn = db.execute(f"SELECT itemid, serverid, patternid, equiploc FROM charinventory WHERE patternid IN ({marks}) "
                      "AND equiploc != -1 ORDER BY itemid", list(new_slot)).fetchall()
    if not worn:
        return []
    moving = {itemid for itemid, *_ in worn}
    owners = {s for _i, s, _p, _e in worn}
    taken = collections.defaultdict(set)   # serverid -> equip slots held by what stays
    for itemid, serverid, equiploc in db.execute(
            f"SELECT itemid, serverid, equiploc FROM charinventory WHERE serverid IN ({','.join('?' * len(owners))}) "
            "AND equiploc != -1", list(owners)):
        if itemid not in moving:
            taken[serverid].add(equiploc)
    names = {pid: name for pid, name, *_ in moves}
    out = []
    for itemid, serverid, pid, equiploc in worn:
        to = EQUIP_OF[new_slot[pid]]
        if to == equiploc:
            continue
        if to in taken[serverid]:
            to = -1
        else:
            taken[serverid].add(to)
        out.append((itemid, serverid, names[pid], equiploc, to))
    return out


def apply(db, moves, worn):
    db.execute(LOG_DDL)
    done = 0
    for pid, _name, old, new, src in moves:
        cur = db.execute("UPDATE itempattern SET equipslot = ? WHERE patternid = ? AND equipslot = ?", (new, pid, old))
        if cur.rowcount:
            db.execute("INSERT INTO belt_robe_slot_fix_log (tbl, rid, old, new, source) VALUES ('itempattern', ?, ?, ?, ?)",
                       (pid, old, new, src))
            done += 1
    copies = 0
    for itemid, _serverid, _name, old, new in worn:
        cur = db.execute("UPDATE charinventory SET equiploc = ? WHERE itemid = ? AND equiploc = ?", (new, itemid, old))
        if cur.rowcount:
            db.execute("INSERT INTO belt_robe_slot_fix_log (tbl, rid, old, new, source) "
                       "VALUES ('charinventory', ?, ?, ?, 'its record moved')", (itemid, old, new))
            copies += 1
    return done, copies


def undo(db):
    if not has_table(db, "belt_robe_slot_fix_log"):
        print("nothing logged: nothing to undo")
        return
    back = kept = 0
    for tbl, rid, old, new in db.execute("SELECT tbl, rid, old, new FROM belt_robe_slot_fix_log ORDER BY seq DESC").fetchall():
        if tbl == "itempattern":
            cur = db.execute("UPDATE itempattern SET equipslot = ? WHERE patternid = ? AND equipslot = ?", (old, rid, new))
        else:
            cur = db.execute("UPDATE charinventory SET equiploc = ? WHERE itemid = ? AND equiploc = ?", (old, rid, new))
        back, kept = (back + 1, kept) if cur.rowcount else (back, kept + 1)
    db.execute("DROP TABLE belt_robe_slot_fix_log")
    print(f"put back {back}; left {kept} changed since")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--prices", nargs="*", default=[], help="eqoa harvest JSON files or folders of them (Sandstorm's records)")
    ap.add_argument("--pages", help="tools/fetch_item_pages.py's folder (Allakhazam's lists by slot)")
    ap.add_argument("--list", action="store_true", help="print every move")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the slots (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply wrote")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 10000")
    if a.undo:
        db.execute("BEGIN IMMEDIATE")
        undo(db)
        db.execute("COMMIT")
        return
    ss = sandstorm(a.prices)[0] if a.prices else {}
    ak = allakhazam(a.pages)
    moves, left, fam, agree = plan(db, ss, ak)
    worn = worn_moves(db, moves)
    print(f"icon families: {sum(1 for f in fam.values() if f == BELT)} belts', {sum(1 for f in fam.values() if f == ROBE)} robes'")
    for src, (yes, no) in sorted(agree.items()):
        print(f"{src}: {yes} agree with the rule, {no} disagree")
    by = collections.Counter((old, new) for _p, _n, old, new, _s in moves)
    print(f"{len(moves)} records move: " + (", ".join(f"slot {o} -> {n}: {c}" for (o, n), c in sorted(by.items())) or "none"))
    for pid, name, old, new, src in moves:
        if a.list or not (old in (ROBE, BELT) and src == "its icon's family"):
            print(f"  {pid} {name!r}: {old} -> {new} ({src})")
    for itemid, serverid, name, old, new in worn:
        print(f"  worn copy {itemid} of {name!r} (character {serverid}): equip slot {old} -> {new}")
    for name, why in left:
        print(f"  left: {name!r}: {why}")
    if not a.apply:
        print("report only: --apply to write")
        return
    db.execute("BEGIN IMMEDIATE")
    done, copies = apply(db, moves, worn)
    db.execute("COMMIT")
    print(f"wrote {done} records, {copies} worn copies moved")


if __name__ == "__main__":
    main()
