#!/usr/bin/env python3
"""The alchemists' potions (F3 20261025-223151, -223841, -223959): every
potion an alchemist's spell summons, as an item, and what using it does, as a
row of itemeffect (entity/potion.go, handlers/potion.go).

The potions are the ones EQOA-Data's alchemist list (Information/databases/
spells/alchemist.json) says the spells summon, named exactly as it names
them, so a spell's words find their item: "SELF Summon Watery Draught",
"Summons Unstable Potion X3". A spell summons three of a potion (jib: "stacks
of 3", F3 20261026-014336) unless its words say how many.

- Thrown potions (the Mixtures, the cocktails): the list's own "Mix Potion"
  and "Fire Potion" lines are their hits; a ranged weapon (F3 20261026-014336,
  jib: "equips in the range attack slot, like a bow, and fires as a range auto
  attack"): worn where a bow is (wear 16, as the rusty throwing knife; attack
  type 10, range 20) and no potion on the client's quick slots (item type 0),
  stacking to 999, an alchemist's (class bit 14), usable from the spell's
  level. The cocktails splash everything within 10 of their target (ours).
- Draughts (Crude Refinement and the rest of the Refinement line, jib: "convert
  HP into Mana, at a high amount of mana gain"): 4 health a level taken at once
  and 15 power a level given over 18 s (ours: no record gives a draught's
  numbers), stacking to 3, usable from the spell's level.
- Speed (the Quickening line) and invisibility (the Sublimation line), jib's
  design: Spirit of the Wolf's own line ("sow, 30min"), on the effects list by
  the spell's name, ended by the drinker's attack; invisibility 4 minutes, a 1%
  chance each tick to wear off, ended by an attack or a hit. The crude ones
  (Crude Quickening, Crude Sublimation) last a minute (F3 20261026-014527).
- Tonics (the Infusion line): an arcane resist, 15 and 30 the list's own (Minor
  and Lesser Tonic), 45, 60 and 75 past them and the 30 minutes ours.
- Correctives and restoratives (Distillation, Purification): one or three
  harmful effects taken off ("Dispel Effect x3", the list's Restorative).
- The returning potion (Summon Returning Potion): back to the bind point.

A "crude" or "faulty" spell's potions are NO TRADE (itempattern.trade 1, the
client's m_noTrade), the rest tradeable (jib: "The non crude versions ... give
tradeable versions"). An item of a potion's name already there that is no
spell scroll (the scroll's art, worn nowhere: the Klik'Anon Cocktail of
itempattern is its spell's scroll, as are tools/import_eqoadata_spells.py's
Lesser Tonic and Restorative) is that potion: its row is kept and given its
use. An itemeffect row already there (a GM's) is left alone. New items take
ids from 50001, a block no item uses; a spell's words find the potion before a
scroll of its name (the server prefers an item with a use).

The report ends with every spell whose words summon an item and the item they
will summon once applied (the words' name, else the spell's own), or none.

Every change is logged in potion_seed and --undo puts it all back. Both take a
copy of the database first (<db>.bak-<when>-pre-potions). Report only by
default. A potion's use is read at each use, but the server loads which items
are thrown at its start: restart it after --apply or --undo.

    tools/seed_potions.py [--db eqoa.db] [--apply | --undo]
"""
import argparse
import collections
import datetime
import os
import re
import sqlite3

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")

FIRST_ID = 50001
ALCHEMIST = 1 << 14
EVERY_CLASS = 32767
EVERY_RACE = 1023

# Item icons (ITEMICON.ESF surfaces) of bottles already in itempattern: arcadin
# brandy's, the wines' (port, merlot), Miraculous Draught's, Alacrity
# Potion's, Major Tonic's. Never the scroll's art (-607649328): an unworn
# item with it is scribed (handlers/scribe.go).
ICON_THROWN = -506671519
ICON_COCKTAIL = 1116835215
ICON_DRAUGHT = -649479524
ICON_FLASK = 209763529
ICON_TONIC = 824019198
SCROLL_ART = -607649328

# Effect icons (an ability's front icon, UI.CSF): Breeze's power over time
# (the drinks'), Spirit of the Wolf's, Invisibility's, Elemental Rhythms'
# resists.
FX_POWER = 0xA2774C47
FX_SOW = 0x260ECB59
FX_INVIS = 0x9CD122E0
FX_RESIST = 0x10CF1729

THROW_RANGE = 20

DESC = {
    "hit": "Thrown at an enemy, this potion bursts on impact.",
    "dot": "Thrown at an enemy, this potion's contents eat away at it.",
    "cocktail": "Thrown at an enemy, this cocktail bursts into flame, catching everything near it.",
    "draught": "A draught that turns some of its drinker's health into power.",
    "speed": "The liquid within the bottle seems to be agitated. Drinking it increases your movement speed.",
    "invis": "Drinking this potion makes you invisible for a time.",
    "tonic": "The potion is filled with a deep red liquid; it gives some resistance to arcane magic.",
    "corrective": "A potion that takes one harmful effect off its drinker.",
    "restorative": "A potion that takes up to three harmful effects off its drinker.",
    "return": "Drinking this potion returns you to your bind point.",
}


def hit(n):
    return f"Target, Decrease Hitpoints by {n} ; Increase 15% based on WIS"


def dot(n, pr, secs=None):
    line = f"Target, Decrease HP per tic by {n}, Decrease PR by {pr}"
    return line + (f", {secs}sec" if secs else "")


def draught(level):
    return f"Self, Decrease Hitpoints by {4 * level}, Increase Power per tic by {5 * level}, 18sec"


# name, the summoning spell, its level, kind, line, effect name, area
POTIONS = [
    # the Mixture line: EQOA-Data's "<X> Mix Potion" lines
    ("Unstable Potion", "Unstable Mixture", 1, "hit", "Target, Decrease Hitpoints by 54 ; Increase 5% based on WIS", "", 0),
    ("Acidic Potion", "Acidic Mixture", 4, "dot", "Target, Decrease HP per tic by 9, Decrease DR by 5, Decrease PR by 5", "", 0),
    ("Volatile Potion", "Volatile Mixture", 8, "hit", hit(100), "", 0),
    ("Caustic Potion", "Caustic Mixture", 12, "dot", dot(14, 10, 60), "", 0),
    ("Explosive Potion", "Explosive Mixture", 16, "hit", hit(134), "", 0),
    ("Corroding Potion", "Corroding Mixture", 20, "dot", dot(40, 15), "", 0),
    ("Fulminant Potion", "Fulminant Mixture", 24, "hit", hit(346), "", 0),
    ("Corrosive Potion", "Corrosive Mixture", 29, "dot", dot(58, 20), "", 0),
    ("Detonating Potion", "Detonating Mixture", 34, "hit", hit(530), "", 0),
    ("Frosty Potion", "Frosty Mixture", 37, "hit", hit(756), "", 0),
    ("Acerbic Potion", "Acerbic Mixture", 39, "dot", dot(109, 25), "", 0),
    ("Meteoric Potion", "Meteoric Mixture", 44, "hit", hit(882), "", 0),
    ("Icy Potion", "Icy Mixture", 46, "hit", hit(1030), "", 0),
    ("Virulent Potion", "Virulent Mixture", 49, "dot", dot(158, 25), "", 0),
    ("Kinetic Potion", "Kinetic Mixture", 55, "hit", hit(1256), "", 0),
    ("Freezing Potion", "Freezing Mixture", 57, "hit", hit(1302), "", 0),
    ("Septic Potion", "Septic Mixture", 60, "dot", dot(193, 30), "", 0),
    # the cocktails: the "<town> Fire Potion" lines, a splash of 10 (ours).
    # Neriak Cocktail summons a Grobb Cocktail, as its line says and the
    # game's own record has it (Allakhazam's archived AK 3902, "Summon Item:
    # Grobb Cocktail"; jib's bug report of 2026-10-27, ALC-3)
    ("Klik'Anon Cocktail", "Klik'Anon Cocktail", 20, "cocktail", "Target, Decrease Hitpoints by 214", "", 10),
    ("Moradhim Cocktail", "Moradhim Cocktail", 30, "cocktail", "Target, Decrease Hitpoints by 338", "", 10),
    ("Grobb Cocktail", "Neriak Cocktail", 40, "cocktail", "Target, Decrease Hitpoints by 626", "", 10),
    ("Freeport Cocktail", "Freeport Cocktail", 49, "cocktail", "Target, Decrease Hitpoints by 1122", "", 10),
    ("Qeynos Cocktail", "Qeynos Cocktail", 49, "cocktail", "Target, Decrease Hitpoints by 842", "", 10),
    # the Refinement line
    ("Watery Draught", "Crude Refinement", 5, "draught", draught(5), "", 0),
    ("Weak Draught", "Faulty Refinement", 15, "draught", draught(15), "", 0),
    ("Draught", "Refinement", 25, "draught", draught(25), "", 0),
    ("Bitter Draught", "Greater Refinement", 35, "draught", draught(35), "", 0),
    ("Stout Draught", "Superior Refinement", 45, "draught", draught(45), "", 0),
    ("Infused Draught", "Pure Refinement", 54, "draught", draught(54), "", 0),
    # the Quickening line: Spirit of the Wolf's own line
    ("Faulty Alacrity Potion", "Crude Quickening", 13, "speed", "Self, Sow, 1min", "Crude Quickening", 0),
    ("Self Alacrity Potion", "Quickening", 33, "speed", "Self, Sow, 30min", "Quickening", 0),
    # the Sublimation line
    ("Faulty Ethereal Potion", "Crude Sublimation", 7, "invis", "Self, Invis, 1min", "Crude Sublimation", 0),
    ("Ethereal Potion", "Sublimation", 27, "invis", "Self, Invis, 4min", "Sublimation", 0),
    ("Aether Potion", "Volatile Sublimation", 47, "invis", "Self, Invis, 4min", "Volatile Sublimation", 0),
    # the Infusion line
    ("Minor Tonic", "Minor Infusion", 4, "tonic", "Self, Increase AR by 15, 30min", "", 0),
    ("Lesser Tonic", "Lesser Infusion", 16, "tonic", "Self, Increase AR by 30, 30min", "", 0),
    ("Tonic", "Infusion", 29, "tonic", "Self, Increase AR by 45, 30min", "", 0),
    ("Greater Tonic", "Greater Infusion", 44, "tonic", "Self, Increase AR by 60, 30min", "", 0),
    ("Major Tonic", "Major Infusion", 55, "tonic", "Self, Increase AR by 75, 30min", "", 0),
    # the Distillation and Purification lines
    ("Minor Corrective", "Minor Distillation", 1, "corrective", "Self, Dispel Effect x1", "", 0),
    ("Lesser Corrective", "Lesser Distillation", 12, "corrective", "Self, Dispel Effect x1", "", 0),
    ("Corrective", "Distillation", 24, "corrective", "Self, Dispel Effect x1", "", 0),
    ("Greater Corrective", "Greater Distillation", 39, "corrective", "Self, Dispel Effect x1", "", 0),
    ("Minor Restorative", "Minor Purification", 8, "restorative", "Self, Dispel Effect x3", "", 0),
    ("Lesser Restorative", "Lesser Purification", 20, "restorative", "Self, Dispel Effect x3", "", 0),
    ("Restorative", "Purification", 34, "restorative", "Self, Dispel Effect x3", "", 0),
    ("Greater Restorative", "Greater Purification", 49, "restorative", "Self, Dispel Effect x3", "", 0),
    ("Returning Potion", "Summon Returning Potion", 50, "return", "Self, Return to bind point", "", 0),
]

# An item already in itempattern that only needs its use: Alacrity Potion's
# own description says what it does (its row is Return Home's).
USES_ONLY = [
    ("Alacrity Potion", "Self, Sow, 30min", "", FX_SOW, "", 0),
]

THROWN_KINDS = {"hit", "dot", "cocktail"}


def item_of(name, spell, level, kind):
    """The itempattern columns of a new potion."""
    no_trade = 1 if spell.split()[0].lower() in ("crude", "faulty") else 0
    thrown = kind in THROWN_KINDS
    return {
        "itemcost": 0, "unk1": 0, "unk2": 0, "unk3": 0, "unk4": 0, "unk6": 0, "craft": 0,
        "itemicon": {"draught": ICON_DRAUGHT, "corrective": ICON_DRAUGHT, "restorative": ICON_DRAUGHT,
                     "tonic": ICON_TONIC, "cocktail": ICON_COCKTAIL}.get(kind, ICON_THROWN if thrown else ICON_FLASK),
        "equipslot": 16 if thrown else -1,  # a bow's slot: the ranged weapon
        "trade": no_trade, "rent": 1,
        "attacktype": 10 if thrown else 0, "weapondamage": 0, "unk5": THROW_RANGE if thrown else 0,
        "levelreq": level if kind in THROWN_KINDS | {"draught"} else 1,
        "maxstack": 999 if thrown else 3, "maxhp": 0, "duration": 50,
        "classuse": ALCHEMIST if thrown else EVERY_CLASS, "raceuse": EVERY_RACE,
        # the client's item type: 2 a potion (quick slots 0-1), 0 a thrown one
        "procanim": 0 if thrown else 2,
        "lore": 0, "itemname": name, "itemdesc": DESC[kind], "model": 0, "color": 4294967295,
    }


def effect_of(kind, line, name, area):
    icon = {"draught": FX_POWER, "speed": FX_SOW, "invis": FX_INVIS, "tonic": FX_RESIST}.get(kind, 0)
    breaks = {"speed": "attack", "invis": "attack,damage"}.get(kind, "")
    wear = 1 if kind == "invis" else 0
    return {"line": line, "name": name, "icon": icon, "breaks": breaks, "wear": wear, "area": area}



def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-potions"
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def ensure_tables(db):
    db.execute("""CREATE TABLE IF NOT EXISTS itemeffect (
        patternid INTEGER PRIMARY KEY,
        line      TEXT NOT NULL,
        name      TEXT NOT NULL DEFAULT '',
        icon      INTEGER NOT NULL DEFAULT 0,
        breaks    TEXT NOT NULL DEFAULT '',
        wear      INTEGER NOT NULL DEFAULT 0,
        area      REAL NOT NULL DEFAULT 0,
        source    TEXT NOT NULL DEFAULT 'gm'
    )""")
    db.execute("""CREATE TABLE IF NOT EXISTS potion_seed (
        what      TEXT NOT NULL,     -- 'item' | 'effect': a row this run inserted
        patternid INTEGER NOT NULL
    )""")


def plan(db):
    """What a run would write: (new items, uses, potions already used)."""
    by_name = {}
    for pid, name in db.execute("SELECT patternid, itemname FROM itempattern WHERE NOT (itemicon = ? AND equipslot = -1) "
                                "ORDER BY patternid DESC", (SCROLL_ART,)):
        by_name[name.lower()] = pid  # the lowest id last, so it wins
    have_effect = set()
    if db.execute("SELECT name FROM sqlite_master WHERE name = 'itemeffect'").fetchone():
        have_effect = {pid for (pid,) in db.execute("SELECT patternid FROM itemeffect")}
    used = {pid for (pid,) in db.execute("SELECT patternid FROM itempattern WHERE patternid >= ?", (FIRST_ID,))}
    next_id = FIRST_ID
    new, effects, kept = [], [], []
    for name, spell, level, kind, line, ename, area in POTIONS:
        pid = by_name.get(name.lower())
        if pid is None:
            while next_id in used:
                next_id += 1
            pid = next_id
            used.add(pid)
            new.append((pid, item_of(name, spell, level, kind)))
        if pid in have_effect:
            kept.append(name)
        else:
            effects.append((pid, name, effect_of(kind, line, ename, area)))
    for name, line, ename, icon, breaks, wear in USES_ONLY:
        pid = by_name.get(name.lower())
        if pid is None:
            continue
        if pid in have_effect:
            kept.append(name)
        else:
            effects.append((pid, name, {"line": line, "name": ename, "icon": icon, "breaks": breaks, "wear": wear, "area": 0}))
    return new, effects, kept


SUMMON = re.compile(r"^summ[ou]ns?\s+(?:an?\s+|the\s+)?(.+?)(?:\s+x\s*(\d+))?$")
PARENS = re.compile(r"\s*\([^)]*\)")
CREATURE = re.compile(r"\b(?:to your aid|to aid|creature|familiar|familar)\b")


def summons(db, new, effects):
    """Every spell whose words summon an item (as entity/potion.go reads them,
    a pet's summoning aside) with the item it will summon once applied: by the
    words' name, else the spell's own, a potion (an item with a use) first,
    then the lowest id; and the count."""
    if not db.execute("SELECT name FROM sqlite_master WHERE name = 'spelleffect'").fetchone():
        return []
    used = {pid for (pid,) in db.execute("SELECT patternid FROM itemeffect")} if db.execute(
        "SELECT name FROM sqlite_master WHERE name = 'itemeffect'").fetchone() else set()
    used |= {pid for pid, _, _ in effects}
    items = collections.defaultdict(list)
    for pid, name, stack in db.execute("SELECT patternid, itemname, maxstack FROM itempattern "
                                       "WHERE NOT (itemicon = ? AND equipslot = -1)", (SCROLL_ART,)):
        items[name.lower()].append((pid not in used, pid, name, stack))
    for pid, it in new:
        items[it["itemname"].lower()].append((pid not in used, pid, it["itemname"], it["maxstack"]))
    out = []
    for sid, sname, line in db.execute("SELECT sp.spellid, sp.spellname, se.line FROM spellpattern sp "
                                       "JOIN spelleffect se ON se.spellid = sp.spellid ORDER BY sp.spellid"):
        text = (line or "").lower().strip()
        if not re.search(r"\bsumm[ou]ns?\b", text) or re.search(r"\bpet\b|to aid you|to your defense", text):
            continue
        for part in re.split(r"[,;]", text):
            part = PARENS.sub("", part).strip()
            for w in ("self ", "target "):
                if part.startswith(w):
                    part = part[len(w):]
            m = SUMMON.match(part)
            if not m or CREATURE.search(part):
                continue
            found = sorted(items.get(m.group(1), [])) or sorted(items.get(sname.lower(), []))
            item = found[0] if found else None
            count = int(m.group(2)) if m.group(2) else (3 if item and item[3] == 3 else 1)
            out.append((sid, sname, m.group(1), item and item[1], item and item[2], count))
    return out


def undo(db):
    ensure_tables(db)
    rows = list(db.execute("SELECT what, patternid FROM potion_seed ORDER BY rowid DESC"))
    db.execute("BEGIN")
    n = collections.Counter()
    for what, pid in rows:
        if what == "item":
            db.execute("DELETE FROM itempattern WHERE patternid = ?", (pid,))
        elif what == "effect":
            db.execute("DELETE FROM itemeffect WHERE patternid = ?", (pid,))
        n[what] += 1
    db.execute("DROP TABLE potion_seed")
    db.execute("COMMIT")
    return n


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--apply", action="store_true", help="write the potions and their uses")
    ap.add_argument("--undo", action="store_true", help="put back everything a run wrote")
    ap.add_argument("--no-backup", action="store_true", help="write without a copy first")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 5000")
    if a.undo:
        if not a.no_backup:
            print("backup:", backup(db, a.db))
        n = undo(db)
        print("put back:", dict(n) or "nothing")
        return
    if a.apply and db.execute("SELECT name FROM sqlite_master WHERE name = 'potion_seed'").fetchone():
        ap.error("potion_seed is there: this database was seeded already (--undo first)")
    new, effects, kept = plan(db)
    for pid, it in new:
        print(f"  new item {pid}: {it['itemname']} (level {it['levelreq']}, stacks to {it['maxstack']}"
              f"{', no trade' if it['trade'] else ''}{', thrown' if it['equipslot'] == 17 else ''})")
    for pid, name, e in effects:
        extra = ", ".join(x for x in (e["name"] and f"named {e['name']!r}", e["breaks"] and f"ends on {e['breaks']}",
                                      e["wear"] and f"{e['wear']}% a tick", e["area"] and f"splash {e['area']}") if x)
        print(f"  use of {name} ({pid}): {e['line']}" + (f" [{extra}]" if extra else ""))
    if kept:
        print(f"{len(kept)} have a use already, left alone: {', '.join(kept)}")
    print("spells whose words summon an item, and the item once applied:")
    none = 0
    for sid, sname, named, pid, iname, count in summons(db, new, effects):
        if pid is None:
            none += 1
            print(f"  spell {sid} {sname}: {named!r}, no item of that name")
        else:
            via = "" if iname.lower() == named else f" (by the spell's name; its words say {named!r})"
            print(f"  spell {sid} {sname}: {count} x item {pid} {iname}{via}")
    print(f"{len(new)} new items, {len(effects)} uses; {none} summoning spells name no item")
    if not a.apply:
        print("(report only: --apply writes them)")
        return
    if not a.no_backup:
        print("backup:", backup(db, a.db))
    ensure_tables(db)
    db.execute("BEGIN")
    log = []
    for pid, it in new:
        cols = ["patternid"] + list(it)
        db.execute(f"INSERT INTO itempattern ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                   [pid] + list(it.values()))
        log.append(("item", pid))
    for pid, _, e in effects:
        db.execute("INSERT INTO itemeffect (patternid, line, name, icon, breaks, wear, area, source) "
                   "VALUES (?, ?, ?, ?, ?, ?, ?, 'potions')",
                   (pid, e["line"], e["name"], e["icon"], e["breaks"], e["wear"], e["area"]))
        log.append(("effect", pid))
    db.executemany("INSERT INTO potion_seed (what, patternid) VALUES (?, ?)", log)
    db.execute("COMMIT")
    print(f"wrote {len(new)} items and {len(effects)} uses; restart the server")


if __name__ == "__main__":
    main()
