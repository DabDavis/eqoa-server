#!/usr/bin/env python3
"""The Stone of Seventeen Banners, the Live shard only (petition 5089, Trulyblind, 2026-11-15: "would it be possible to
get some kind of item or something, so i can test the other 17s? just for testing"; the owner: "give them faction
stone that increases factions with all lvl 17 q givers").

The level 17 quests are the twelve cities' "17" quests (EQOA-Data 17/<city>, quests 100747-100758: The Necromancer
of Sslathis, The Scribe's Inks, Froglok Liver, Bloodmaw, Ulria Veshiue, Grave Silk, Courier Svex, Deidre IV,
Murien's Brew, The Ceremony, The Shadowhand Receipt, Tesht'uzu), whose last steps hand out each class's level 17
piece (the "of Malice" set among them). None asks a standing (quests.factionreq is empty on all twelve); what
stops a character of another city is its people: an NPC of a faction attacks below -350 and will not talk kindly
below -150 (entity/aggro.go, dialog.go). So the stone raises every faction an NPC of theirs is of (npcfaction, by
name: the givers and every NPC a step sends one to), and the guards' factions of the same cities (The Freeport
Militia, The Qeynos Guard, The Highpass Guard, Klick`Anon Watch: a guard of its own faction stands between a
stranger and the giver), to Allied (700), never down.

Writes: the item record (patternid 715100: a gem-type Use item like the Freeport Travel Stone, the blue rune stone's
icon, No Trade, Lore, worth 0, every class and race) and its itemfactions rows (handlers/itemfactions.go: the Use
raises each to its standing; the stone is not spent). itemfactions is the server's (the build with
handlers/itemfactions.go makes it at its start): without it this refuses, so nothing is written before that build
runs on the database. Report only by default (the dry run); --apply in one transaction, a gm_audit line a row, log
faction_stone_5089_log; --undo exact. Item records at the next login; the rows at the next Use.

    python3 -E -s tools/faction_stone_5089_20261115.py --db ~/eqoa-live/eqoa.db [--apply | --undo]
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "faction_stone_5089_log"
DETAIL = "Petition 5089 (Live, Trulyblind): the Stone of Seventeen Banners raises the level 17 quests' factions to Allied: tools/faction_stone_5089_20261115.py"
STONE = 715100
NAME = "Stone of Seventeen Banners"
DESC = ("A stone bound in twelve cities' colours. Use it and the people of every city that sets a level 17 task, and "
        "their guards, hold you an ally. It never wears out.")
ALLIED = 700
QUESTS = range(100747, 100759)
# a city's guards stand in a faction of their own
GUARDS = {
    "Citizens of Freeport": "The Freeport Militia",
    "Citizens of Qeynos": "The Qeynos Guard",
    "Citizens of Highpass": "The Highpass Guard",
    "Citizens of Klick`Anon": "Klick`Anon Watch",
}
ICON = 414194432  # ITEMICON.ESF's blue rune stone (the Freeport Travel Stone's, the Rock of Freeport's)


def record():
    return {"patternid": STONE, "itemcost": 0, "unk1": 0, "itemicon": ICON, "unk2": 0, "equipslot": -1, "unk3": 0,
            "trade": 1, "rent": 1, "unk4": 0, "attacktype": 0, "weapondamage": 0, "unk5": 0, "levelreq": 1,
            "maxstack": 1, "maxhp": 0, "duration": 0, "classuse": 32767, "raceuse": 32767, "procanim": 7, "lore": 1,
            "unk6": 0, "craft": 0, "itemname": NAME, "itemdesc": DESC, "model": 0, "color": 4294967295}


def factions(db):
    """[(faction, why)] in name order, and refusals."""
    qs = tr.rows(db, f"SELECT questid, name, giver FROM quests WHERE questid BETWEEN {QUESTS.start} AND {QUESTS.stop - 1}")
    refusals = []
    if len(qs) != len(QUESTS):
        refusals.append(f"{len(qs)} of the {len(QUESTS)} level 17 quests ({QUESTS.start}-{QUESTS.stop - 1}) here")
    names = {q["giver"] for q in qs}
    for r in db.execute(f"SELECT target FROM queststeps WHERE questid BETWEEN {QUESTS.start} AND {QUESTS.stop - 1} "
                        "AND kind IN ('talk', 'give')"):
        names.add(r[0])
    why = {}
    for n in sorted(names):
        for (f,) in db.execute("SELECT DISTINCT faction FROM npcfaction WHERE lower(name) = lower(?) AND faction != ''", (n,)):
            why.setdefault(f, set()).add(n)
    for city, guard in GUARDS.items():
        if city in why:
            why.setdefault(guard, set()).add("its city's guards")
    known = {r[0] for r in db.execute("SELECT name FROM factions")}
    for f in why:
        if f not in known:
            refusals.append(f"no faction {f!r} in the factions table")
    return sorted((f, ", ".join(sorted(w))) for f, w in why.items()), refusals


def changes(db):
    if not tr.table(db, "itemfactions"):
        return [], [], ["no itemfactions table: start the build with handlers/itemfactions.go on this database first; "
                        "it makes the table"], False
    out, lines = [], []
    held = tr.get(db, "itempattern", {"patternid": STONE})
    if held and held["itemname"] != NAME:
        return [], [], [f"patternid {STONE} is {held['itemname']!r} here"], False
    rec = record()
    if not held or any(held.get(c) != v for c, v in rec.items()):
        out.append(("itempattern", {"patternid": STONE}, rec))
    fs, refusals = factions(db)
    lines.append(f"{NAME} ({STONE}): {len(fs)} factions raised to {ALLIED} (Allied), never down")
    for f, why in fs:
        key = {"patternid": STONE, "faction": f}
        after = {"patternid": STONE, "faction": f, "standing": ALLIED, "note": "level 17 quests: " + why,
                 "source": "faction_stone_5089"}
        have = tr.get(db, "itemfactions", key)
        lines.append(f"  {f:<26} {why}")
        if have is None or any(have.get(c) != v for c, v in after.items()):
            out.append(("itemfactions", key, after))
    return out, lines, refusals, not out


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("itempattern", "quests", "queststeps", "npcfaction", "factions"),
        "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
