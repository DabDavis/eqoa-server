#!/usr/bin/env python3
"""The EQOAEmu follow-ups' places (the user, 2026-11-01: "lets fix all
these"): the five teleports the workbook brought, where they land and what
they open, and the Plane of Sky's main isle where Sky Portal lands, on a
database the import (tools/import_eqoaemu_spells.py), its follow-ups and the
gates' seed (tools/seed_spell_gates.py) were applied to.

  1. Recasts (RECASTS, import_eqoaemu_spells.py's: a fresh import makes the
     same). Envaric Acension the hour EQOA-Data's Envaric Ascension guide
     gives ("Recast: 3600 (One Hour)"), the Circle of Dead Hills 600, its
     kin's (every druid circle on this server, Barren Coast's, the Rathe's,
     Stonebrunt's, Kithicor's, Jaggedpine's, Sycamore's, Toxxulia's: ours, by
     kin); Sky Portal, Stonewatcher's Portal and Odus Escape keep the
     workbook's 3, their kin's (every wizard gate, portal and escape here).
     A record whose recast is not the workbook's 3 any more is left, said.

  2. The Prophet of Envar's tower, the way up (EQOA-Data's Envaric Ascension
     guide: "now you must slay your way up to the prophet"). Envaric Acension
     lands its group in the first chamber, among the Jal'Raeth nobles, as
     the guide's "Ports to the first chamber of PoE's tower"; every
     character has Return Home (database/newchar.go), so the chamber is no
     trap. Three gems (GEMS: the Noble's and the Champion's Gem the guide's
     names, the Channeler's Gem ours), each dropped by its chamber's
     creatures (the nobles; the champions and knights; the channelers: a
     loot table each, bound in npcloot, GEM_CHANCE a kill, one roll: ours),
     single-charged, lore, no rent ("UC, NO RENT LORE"), each a potion
     whose use sends its user up one chamber (entity/potion.go's "Teleport
     to <place>", a self gate of its own in spellgates landed inside as
     entity/gates.go lands under a roof) and leaves its cracked version
     ("You will now have a cracked gem in your inventory. Do not destroy it!
     It is needed in your inventory to use the next stone"), which the next
     gem takes (its line's Reagent, handlers/potion.go): "Thus people may
     not simply coth into the tower". The gems trade (the guide gives the
     first chamber's to more than a group: "bloodgate the gems from the
     first chamber to your force"), the cracked ones do not (ours). The
     chambers' places (CHAMBERS) are their floors as the disc's terrain has
     them (91.38, 111.38, 131.38 round (5276, 9262)): the second chamber's
     the south-east corner of its floor, where the landing finds no way out
     through the chamber's walls (from most of that floor the ground's walls
     let the search out onto the ramparts at 82); the third's and the top's
     the floor's middle. Not built: the orb in the channelers' room that
     summons the Prophet ("speak to the orb (unnamed con)... this will summon
     the prophet to the top of his tower") and the Prophet himself: neither
     is in this server's data.

  3. The two quests that teach two of the five, Fendren the Forgotten's The
     Dead Hills Port and the altars' Envaric Ascension, drafts only because
     their reward names matched no record: tools/quests/data/portal.json now
     names the Circle of Dead Hills (the druid's: no record gives a wizard's
     port to the Dead Hills, so the quest is the druid's alone) and Envaric
     Acension, and they are written again by tools/import_quests.py's own
     plan and write (a quest a GM has changed since its import is left), in
     the game where nothing keeps them out, the GM panel's quest index's
     rules too (gmweb/questindex.go: a kill or loot step's creature
     attackable), else drafts with why.

  4. The Plane of Sky's main isle (world 4), where Sky Portal and the Endless
     Tower's portal land players: its elemental workers (level 60, the two
     clusters round the isle's merchants) do not attack on sight (an npcaggro
     row, aggressive 0: still attackable); the isle's people its quests talk
     to that carry no NPC bit get it (128, as the Island of Wonder's portal
     did, 2026-11-01): Mistress Sanchez (the 57 and 59 quests' contact, who
     attacked on sight), the Mark of the Majestic (the snow key's giver), every
     essence in the Island of Wonder's portal body (0x8143DC70: "Approach the
     Essence ... On using the portal", EQOA-Data's Plane of Sky guide), and by
     the same rule Sartheret (the Endless Tower quest's) and the aviak elder
     (the Desolation quest's): each was a creature to the client, peaceful
     only by an npcaggro row. A name a quest kills, loots or spawns hostile is
     left, said.

  5. Two lasting debuffs' times (DURATIONS, import_eqoaemu_spells.py's: a
     fresh import makes the same). The workbook gives the mastery Swamp
     Stench's AGI and AC and the rogue's epic Strategic Blow's AC Duration 0,
     so neither lasted any time nor landed (entity/cast.go's lasting): each
     takes 30 s, the user's figure (2026-11-01: "30 sec for both"), as
     import_eqoaemu_spells.py's with_duration writes it: duration_s in its
     effects and its line ending in the time, as the import's line writer
     ends a line with one (its line the import's, as its log has it). Only
     where its effects are still as the import wrote them (its log's last
     row for them); else listed, left.

  6. The enchanter's taunting Visages (MIRRORS, import_eqoaemu_spells.py's: a
     fresh import makes the same; the user, 2026-11-01: "yes make it a taunt
     proc", "yes make them taunt procs"): Alarming, Haunting and Ghastly
     Visage, whose EQOA-Data lines read to no effect, take Calming Visage's
     effects (a friendly target's offensive proc of kind hate, 5 min, its
     stack) with their procs raising the hate of what the bearer's weapon
     hits by 10, 20 and 40 (ours), at their own levels; their records target
     spells as Calming Visage is (scope, frame), their descriptions ours
     (import_eqoaemu_spells.py's mirror_changes on Calming Visage's effects
     as the import's log has them). Only a spell with no effects of its own
     yet is written; else listed, left.

  7. Two alchemist spells of ours the workbook lacks given effects of ours
     (MAKES, CURES, import_eqoaemu_spells.py's: a fresh import makes the
     same; the user, 2026-11-01: "yes make frosty potion summon a frosty
     potion, make it cure up to 3 effects"): Frosty Mixture makes one Frosty
     Potion (our item by the server's own lookup by name) in Icy Mixture's
     frame, Restorative cures up to three of the harmful effects on its
     bearer, the first come by first; their lines and descriptions ours
     (import_eqoaemu_spells.py's ours_changes, the maker as the import's log
     has it). Only a spell with no effects of its own yet is written; else
     listed, left.

Where two of the five land stays as tools/seed_spell_gates.py put them:
the Circle of Dead Hills at Fendren's spot (Fendren's guide: "Ports to Dead
Hills SE (where Fendren is)", its level 53 guide that his port goes "to the
same place" as the 53 quest's; the guides disagree only about the 53
quest's own port, which no record of ours has), and Stonewatcher's Portal on
the west bank at the dam (the stat-boost guide: "He sends you to Stone
Watchers (C) next to a dam (wizard's port takes you very close)", then
"speak to gnoll npc": the trial's reaver gnoll stands on the west shore).

Report by default. --apply writes it all in one transaction, every row
logged in eqoaemu_places_log under the run's time (the quests' rows as they
were and as written); a second --apply writes only what is missing (none, on
a database it was applied to). --undo takes the last run back: each row
still as it wrote it put back as it was (a row changed since is left, and
said), the item and loot counters too. Both take a copy of the database
first (--no-backup to skip). The server reads npcs, npcaggro, spellgates and
the quests at its start: restart it after; the items, their uses, the loot
and a spell's effects (at each cast) are read as they are used.

    tools/fix_eqoaemu_places.py [--db eqoa.db] [--apply | --undo] [--no-backup]
"""
import argparse
import collections
import datetime
import json
import os
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import import_quests as iq  # noqa: E402
from import_eqoaemu_spells import CURES, DURATIONS, MAKES, MIRRORS, RECASTS, dumps as im_dumps, with_duration  # noqa: E402
import import_eqoaemu_spells as im  # noqa: E402

DEFAULT_DB = os.path.join(HERE, "..", "eqoa.db")
LOG = "eqoaemu_places_log"
SOURCE = "eqoaemu places"
WORKBOOK_RECAST = 3   # the five teleports' recast in the workbook
IMPORT_LOG = "eqoaemu_spell_import_log"   # import_eqoaemu_spells.py's: what it wrote (5.)

# ---- 2. the tower --------------------------------------------------------------

RATHE = 1
GEM_BASE = 960000         # the gems' item ids: 960000 + n (ours; no other tool's range)
GEM_ICON = 140265582      # the quest gems' icon (the Khal gems', the green soulstone gem's)
GEM_CHANCE = 33           # percent a kill, one roll (ours: no record of the live game's)
GEM_LEVELS = (50, 70)     # the creature levels a gem's table drops for (the chambers' are 55-57)

# Each chamber a gem sends its user to: (its place as a player reads it,
# (x, y, z) its floor's spot), the second's the south-east corner, the
# others' the middle (the doc).
CHAMBERS = {
    "second": ("Prophet's Second Chamber", (5288.0, 91.38, 9279.0)),
    "third": ("Prophet's Third Chamber", (5276.4, 111.38, 9261.2)),
    "top": ("Top of the Prophet's Tower", (5276.4, 131.38, 9261.2)),
}

# The gems, lowest first: its name, its cracked version's, the cracked gem it
# needs (the gem before's), the creatures whose kills drop it (npc names as
# npcclass keys them), its loot table, the chamber it sends to, its id.
GEMS = [
    dict(name="Noble's Gem", cracked="Cracked Noble's Gem", needs=None, drops=["jal'raeth noble"],
         table="envar tower nobles", to="second", n=1, of="the Jal'Raeth nobles"),
    dict(name="Champion's Gem", cracked="Cracked Champion's Gem", needs="Cracked Noble's Gem",
         drops=["jal'raeth champion", "jal'raeth knight"], table="envar tower champions", to="third", n=3,
         of="the Jal'Raeth champions and knights"),
    dict(name="Channeler's Gem", cracked="Cracked Channeler's Gem", needs="Cracked Champion's Gem",
         drops=["jal'raeth channeler"], table="envar tower channelers", to="top", n=5, of="the Jal'Raeth channelers"),
]
WHERE = {"second": "the second chamber of the Prophet's tower", "third": "the third chamber of the Prophet's tower",
         "top": "the top of the Prophet's tower"}


def gem_line(g, chambers=CHAMBERS):
    """A gem's use in the potion reader's words (entity/potion.go): its
    reagent, its place, what it leaves."""
    needs = f"Reagent {g['needs']} x1, " if g["needs"] else ""
    return f"Self, {needs}Teleport to the {chambers[g['to']][0]}, Summon {g['cracked']} x1"


def item_row(pid, name, desc, use):
    """An item's row: a gem used as a potion is (equipslot -1, procanim 2,
    durability 50, any class and race, level 1), its cracked version a quest
    gem's; single, lore, no rent; a gem trades, a cracked one does not."""
    return dict(patternid=pid, itemcost=0, unk1=0, itemicon=GEM_ICON, unk2=0, equipslot=-1, unk3=0,
                trade=0 if use else 1, rent=0, unk4=0, attacktype=0, weapondamage=0, unk5=0,
                levelreq=1 if use else 0, maxstack=1, maxhp=0, duration=50 if use else 0, classuse=32767,
                raceuse=1023, procanim=2 if use else 0, lore=1, unk6=0, craft=0, itemname=name, itemdesc=desc,
                model=0, color=4294967295)


def gem_rows(g, chambers=CHAMBERS):
    """The gem's and its cracked version's item rows."""
    where = WHERE.get(g["to"], chambers[g["to"]][0])
    took = f", taking the {g['needs'][len('Cracked '):]} its bearer used" if g["needs"] else ""
    gem = item_row(GEM_BASE + g["n"], g["name"],
                   f"A single-charged gem of {g['of']}. Used, it sends its bearer up to {where}{took}, and cracks.", True)
    cracked = item_row(GEM_BASE + g["n"] + 1, g["cracked"],
                       f"The {g['name']}, used. The gem of the chamber above will not take its bearer up without it.",
                       False)
    return gem, cracked


# ---- 3. the quests --------------------------------------------------------------

PORTAL = os.path.join(HERE, "quests", "data", "portal.json")
QUEST_KEYS = ("Portal/fendren-the-forgotten/port", "Portal/envaric-ascension-quest/ascension")
QUEST_ROWS = ("queststeps", "questunlocks", "queststeptargets")

# ---- 4. the isle ----------------------------------------------------------------

SKY = 4
PORTAL_BODY = 2168708208   # 0x8143DC70: the Island of Wonder's portal body, the isle's essences'
WORKERS = "elemental worker"
TALKED_TO = ("Mistress Sanchez", "Mark of the Majestic", "Sartheret", "aviak elder")
NPC_BIT = 0x80

KEYS = {"spellpattern": ["spellid"], "itempattern": ["patternid"], "itemeffect": ["patternid"],
        "spellgates": ["spell"], "loottables": ["id"], "npcloot": ["npc_name", "tablename"],
        "npcaggro": ["npc_name"], "npcs": ["id"]}
COUNTERS = {"itempattern": "patternid", "loottables": "id"}

LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY,
    run    TEXT NOT NULL,     -- the run's time (UTC): --undo takes the last run back
    tbl    TEXT NOT NULL,
    ref    TEXT NOT NULL,     -- the row's key columns (JSON); a quest's import key
    action TEXT NOT NULL,     -- insert | update | quest | meta | counter
    before TEXT,              -- the columns as they were (JSON); an insert: none
    after  TEXT,              -- the columns as written (JSON)
    note   TEXT NOT NULL DEFAULT ''
)"""


def has_table(db, t):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone()[0] > 0


def columns(db, t):
    return [r[1] for r in db.execute(f"PRAGMA table_info({t})")]


def npc_key(name):
    """database.NPCClassKey: lower case, trimmed, no trailing $."""
    n = name.strip().lower()
    return n[:-1].strip() if n.endswith("$") else n


def dumps(o):
    return json.dumps(o, sort_keys=True, separators=(",", ":"))


def item_named(db, name):
    """The item a player reads by the name (database.ItemPatternByName's
    rule, a scroll aside): its patternid, or None."""
    r = db.execute("SELECT p.patternid FROM itempattern p LEFT JOIN itemeffect e ON e.patternid = p.patternid "
                   "WHERE lower(p.itemname) = ? AND NOT (p.itemicon = -607649328 AND p.equipslot = -1) "
                   "ORDER BY e.patternid IS NULL, p.patternid LIMIT 1", (name.strip().lower(),)).fetchone()
    return None if r is None else r[0]


# ---- the plan -------------------------------------------------------------------

def plan_recasts(db, recasts, ch, said):
    for name, want in sorted(recasts.items()):
        rows = db.execute("SELECT spellid, spellname, recast FROM spellpattern WHERE lower(trim(spellname)) = ? ORDER BY spellid",
                          (name,)).fetchall()
        if not rows:
            said["a recast's spell is not here"].append(name)
        for sid, sname, recast in rows:
            if recast == want:
                said["recasts as they are to be"].append(f"{sid} {sname} {recast}")
            elif recast != WORKBOOK_RECAST:
                said["a recast not the workbook's 3 (left: a GM's figure?)"].append(f"{sid} {sname} {recast}")
            else:
                ch.append(dict(tbl="spellpattern", action="update", ref={"spellid": sid}, before={"recast": recast},
                               set={"recast": want}, note=f"{sname}: recast {recast} -> {want}"))


def import_wrote(db, sid):
    """A spell's spelleffect row as import_eqoaemu_spells.py last wrote it:
    (its line, None when it kept the line it found; its effects), or None
    when its log has no row for it."""
    if not has_table(db, IMPORT_LOG):
        return None
    r = db.execute(f"SELECT after FROM {IMPORT_LOG} WHERE tbl = 'spelleffect' AND key = ? AND after IS NOT NULL "
                   "ORDER BY seq DESC LIMIT 1", (im_dumps({"spellid": sid}),)).fetchone()
    if r is None:
        return None
    a = json.loads(r[0])
    return a.get("line"), a.get("effects")


def plan_durations(db, durations, ch, said):
    for name, want in sorted(durations.items()):
        rows = db.execute("SELECT spellid, spellname FROM spellpattern WHERE lower(trim(spellname)) = ? ORDER BY spellid",
                          (name,)).fetchall()
        if not rows:
            said["a duration's spell is not here"].append(name)
        for sid, sname in rows:
            r = db.execute("SELECT line, effects FROM spelleffect WHERE spellid = ?", (sid,)).fetchone() \
                if has_table(db, "spelleffect") and "effects" in columns(db, "spelleffect") else None
            if r is None or not (r[1] or "").strip():
                said["a duration's spell with no workbook effects (left)"].append(f"{sid} {sname}")
                continue
            line, fx = r
            try:
                j = json.loads(fx)
            except ValueError:
                j = {}
            if isinstance(j, dict) and j.get("duration_s") == want:
                said["durations as they are to be"].append(f"{sid} {sname} {want} s")
                continue
            wrote = import_wrote(db, sid)
            new = with_duration(fx, line if line == wrote[0] else None, want) if wrote and wrote[1] == fx else None
            if new is None:
                said["a duration's effects not as the import wrote them (left: a GM's?)"].append(f"{sid} {sname}: {fx}")
                continue
            before, set_ = {"effects": fx}, {"effects": new[0]}
            if new[1] is None:
                said["a duration's line not the import's, kept (a look: does it say a time?)"].append(f"{sid} {sname}: {line}")
            elif new[1] != line:
                before["line"], set_["line"] = line, new[1]
            ch.append(dict(tbl="spelleffect", action="update", ref={"spellid": sid}, before=before, set=set_,
                           note=f"{sname}: what lasts of it {want} s (DURATIONS: the workbook's 0)"))


def plan_mirrors(db, mirrors, ch, said):
    cols = [c for c in ("spellid", "spellname", "abilitylvl", "scope", "iconColor", "spelldesc") if c in columns(db, "spellpattern")]
    recs = [dict(zip(cols, r)) for r in db.execute(f"SELECT {', '.join(cols)} FROM spellpattern")]
    has_fx = has_table(db, "spelleffect") and "effects" in columns(db, "spelleffect")
    for name, (orig, hate) in sorted(mirrors.items()):
        mine = [r for r in recs if im.exact_key(r["spellname"]) == im.exact_key(name)]
        theirs = [r for r in recs if im.exact_key(r["spellname"]) == im.exact_key(orig)]
        if len(mine) != 1 or len(theirs) != 1 or not has_fx:
            said["a mirror whose spell or original is not one record here (left)"].append(
                f"{name}: {len(mine)} records, {orig}: {len(theirs)}")
            continue
        rec, o = mine[0], theirs[0]
        orow = db.execute("SELECT effects FROM spelleffect WHERE spellid = ?", (o["spellid"],)).fetchone()
        wrote = import_wrote(db, o["spellid"])
        if orow is None or not (orow[0] or "").strip() or wrote is None or wrote[1] != orow[0]:
            said["a mirror's original not as the import wrote it (left)"].append(f"{o['spellid']} {o['spellname']}")
            continue
        r = db.execute("SELECT line, effects, source FROM spelleffect WHERE spellid = ?", (rec["spellid"],)).fetchone()
        now = dict(zip(("line", "effects", "source"), r)) if r else None
        record, eff = im.mirror_changes(rec, now, o["scope"], o["iconColor"], o["spellname"], json.loads(orow[0]), hate)
        if not record and not eff:
            said["mirrors as they are to be"].append(f"{rec['spellid']} {rec['spellname']}")
            continue
        if now is None or (eff and (now.get("effects") or "").strip()):
            said["a mirror's spell with effects of its own or no effects row (left: a GM's?)"].append(
                f"{rec['spellid']} {rec['spellname']}")
            continue
        tag = f"{rec['spellname']}: {o['spellname']}'s mirror, a taunt of {hate} (MIRRORS)"
        if record:
            ch.append(dict(tbl="spellpattern", action="update", ref={"spellid": rec["spellid"]}, before=record["before"],
                           set=record["set"], note=tag))
        if eff:
            ch.append(dict(tbl="spelleffect", action="update", ref={"spellid": rec["spellid"]}, before=eff["before"],
                           set=eff["set"], note=tag))


def our_item_by_name(db, name):
    """(patternid, name) of the item the server finds by name: the lowest
    numbered of the name that is no spell's scroll (import_eqoaemu_spells.py's
    items_by_name), or None."""
    for pid, n, icon, slot in db.execute("SELECT patternid, itemname, itemicon, equipslot FROM itempattern ORDER BY patternid"):
        if not (icon == im.SCROLL_ICON and slot == -1) and im.fold(n) == im.fold(name):
            return pid, n
    return None


def plan_ours(db, makes, cures, ch, said):
    cols = [c for c in ("spellid", "spellname", "abilitylvl", "scope", "iconColor", "spelldesc") if c in columns(db, "spellpattern")]
    recs = [dict(zip(cols, r)) for r in db.execute(f"SELECT {', '.join(cols)} FROM spellpattern")]
    has_fx = has_table(db, "spelleffect") and "effects" in columns(db, "spelleffect")

    def one(name):
        return [r for r in recs if im.exact_key(r["spellname"]) == im.exact_key(name)]
    todo = []
    for name, (maker, item, count) in sorted(makes.items()):
        mine, theirs, it = one(name), one(maker), our_item_by_name(db, item)
        if len(mine) != 1 or len(theirs) != 1 or it is None or not has_fx:
            said["a spell of ours to make an item whose record, maker or item is not one here (left)"].append(
                f"{name}: {len(mine)} records, {maker}: {len(theirs)}, {item}: {it}")
            continue
        o = theirs[0]
        orow = db.execute("SELECT effects FROM spelleffect WHERE spellid = ?", (o["spellid"],)).fetchone()
        wrote = import_wrote(db, o["spellid"])
        if orow is None or not (orow[0] or "").strip() or wrote is None or wrote[1] != orow[0]:
            said["a maker not as the import wrote it (left)"].append(f"{o['spellid']} {o['spellname']}")
            continue
        j = im.makes_json(json.loads(orow[0]), mine[0]["abilitylvl"], it[1], it[0], count)
        todo.append((mine[0], name, j, f"effects ours: {o['spellname']}'s, making our {it[1]}", {it[0]: it[1]},
                     f"a {it[1]}"))
    for name, n in sorted(cures.items()):
        mine = one(name)
        if len(mine) != 1 or not has_fx:
            said["a spell of ours to cure whose record is not one here (left)"].append(f"{name}: {len(mine)} records")
            continue
        j = im.cures_json(mine[0]["scope"], mine[0]["abilitylvl"], n)
        todo.append((mine[0], name, j, f"effects ours: a cure of up to {n}", None, f"a cure of up to {n}"))
    for rec, name, j, note, item_names, what in todo:
        r = db.execute("SELECT line, effects, source FROM spelleffect WHERE spellid = ?", (rec["spellid"],)).fetchone()
        now = dict(zip(("line", "effects", "source"), r)) if r else None
        record, eff = im.ours_changes(rec, now, {"spelldesc": im.OURS_DESC.get(name)}, j, note, item_names)
        if not record and not eff:
            said["spells of ours given their effects already"].append(f"{rec['spellid']} {rec['spellname']}")
            continue
        if now is None or (eff and (now.get("effects") or "").strip()):
            said["a spell of ours with effects of its own or no effects row (left: a GM's?)"].append(
                f"{rec['spellid']} {rec['spellname']}")
            continue
        tag = f"{rec['spellname']}: {what} (MAKES, CURES)"
        if record:
            ch.append(dict(tbl="spellpattern", action="update", ref={"spellid": rec["spellid"]}, before=record["before"],
                           set=record["set"], note=tag))
        if eff:
            ch.append(dict(tbl="spelleffect", action="update", ref={"spellid": rec["spellid"]}, before=eff["before"],
                           set=eff["set"], note=tag))


def plan_tower(db, gems, chambers, ch, said):
    for g in gems:
        # its two items: one of the name there is used; one to make whose id
        # another item holds leaves the gem out whole (its use, gate and drop)
        make, blocked = [], []
        for row in gem_rows(g, chambers):
            name, pid = row["itemname"], row["patternid"]
            have = item_named(db, name)
            taken = db.execute("SELECT itemname FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
            if have is not None:
                said["a gem's item there already (left as it is)"].append(f"{have} {name}")
            elif taken is not None:
                blocked.append(f"{pid} {taken[0]} for {name}")
            else:
                make.append(dict(tbl="itempattern", action="insert", ref={"patternid": pid}, row=row, note=name))
        if blocked:
            said["a gem's id taken by another item (the gem left out: its items, use, gate and drop)"] += blocked
            continue
        ch.extend(make)
        # its use, its place, its drop
        line = gem_line(g, chambers)
        pid = item_named(db, g["name"]) or GEM_BASE + g["n"]
        cur = db.execute("SELECT line FROM itemeffect WHERE patternid = ?", (pid,)).fetchone() if has_table(db, "itemeffect") else None
        if cur is not None:
            said["a gem's use there already (left as it is)"].append(f"{g['name']}: {cur[0]}" + ("" if cur[0] == line else f" (planned: {line})"))
        else:
            ch.append(dict(tbl="itemeffect", action="insert", ref={"patternid": pid}, note=g["name"],
                           row=dict(patternid=pid, line=line, name="", icon=0, breaks="", wear=0, area=0.0, source=SOURCE)))
        place, (x, y, z) = chambers[g["to"]]
        key = g["name"].strip().lower()
        gate = dict(spell=key, kind="self", place=place, world=RATHE, x=x, y=y, z=z, radius=0.0, source=SOURCE,
                    note=f"{g['name']} (tools/fix_eqoaemu_places.py): {WHERE.get(g['to'], place)}, its floor as the disc's "
                         "terrain has it, landed under the roof (entity/gates.go); the spot ours")
        cur = db.execute("SELECT place, world, x, y, z FROM spellgates WHERE spell = ?", (key,)).fetchone() if has_table(db, "spellgates") else None
        if cur is not None:
            said["a gem's gate there already (left as it is)"].append(f"{key}: {cur}")
        else:
            ch.append(dict(tbl="spellgates", action="insert", ref={"spell": key}, row=gate, note=g["name"]))
        have = db.execute("SELECT count(*) FROM loottables WHERE tablename = ? AND patternid = ?", (g["table"], pid)).fetchone()[0]
        if have:
            said["a gem's loot row there already (left as it is)"].append(f"{g['table']}: {g['name']}")
        else:
            ch.append(dict(tbl="loottables", action="insert", ref={"id": None}, note=g["name"],
                           row=dict(tablename=g["table"], patternid=pid, weight=1, minlevel=GEM_LEVELS[0],
                                    maxlevel=GEM_LEVELS[1], source=SOURCE)))
        for npc in g["drops"]:
            k = npc_key(npc)
            n = db.execute("SELECT count(*) FROM npcs WHERE lower(trim(npc_name)) = ?", (k,)).fetchone()[0]
            if not n:
                said["a gem's creature not in npcs (bound all the same)"].append(f"{npc} ({g['name']})")
            if db.execute("SELECT count(*) FROM npcloot WHERE npc_name = ? AND tablename = ?", (k, g["table"])).fetchone()[0]:
                said["a gem's binding there already (left as it is)"].append(f"{k} -> {g['table']}")
                continue
            ch.append(dict(tbl="npcloot", action="insert", ref={"npc_name": k, "tablename": g["table"]}, note=g["name"],
                           row=dict(npc_name=k, tablename=g["table"], chance=GEM_CHANCE, rolls=1, source=SOURCE,
                                    note=f"the {g['name']} (tools/fix_eqoaemu_places.py: Envar's tower); the chance ours")))


def attack_misses(db, x):
    """The quest index's rule the importer does not hold a quest to
    (gmweb/questindex.go): every kill or loot step's creature has one spawn
    without the NPC bit."""
    out = []
    for st in x["steps"]:
        if st["kind"] not in ("kill", "loot") or not st["target"]:
            continue
        free = db.execute("SELECT count(*) FROM npcs WHERE lower(npc_name) = lower(?) AND (COALESCE(npc_type, 0) & 128) = 0",
                          (st["target"],)).fetchone()[0]
        if not free:
            out.append(f"every {st['target']!r} carries the NPC bit, so none can be attacked (step {st['step']})")
    return out


def plan_quests(db, portal, keys, said):
    """The quests' plan (import_quests.py's, these keys alone), or None."""
    if not os.path.exists(portal):
        said["the quests' content file is not here (left)"].append(portal)
        return None
    if not has_table(db, "quests"):
        said["no quests table here (left)"].append("quests")
        return None
    p = iq.plan(db, iq.load_files([portal]), routes=iq.travel.travel_routes())
    p["quests"] = [x for x in p["quests"] if x["key"] in keys]
    for k in keys:
        if not any(x["key"] == k for x in p["quests"]):
            said["a quest the content file lacks (left)"].append(k)
    named = set()

    def refs(o):  # the items the importer makes that these quests name: ("made", name)
        if isinstance(o, tuple) and len(o) == 2 and o[0] == "made":
            named.add(iq.fold(o[1]))
        elif isinstance(o, (list, tuple)):
            for v in o:
                refs(v)
        elif isinstance(o, dict):
            for v in o.values():
                refs(v)
    refs(p["quests"])
    p["made"] = [m for m in p["made"] if iq.fold(m["name"]) in named]
    logged = dict(db.execute("SELECT key, enabled FROM quest_import_quests").fetchall()) if has_table(db, "quest_import_quests") else {}
    for x in p["quests"]:
        miss = attack_misses(db, x)
        if miss:
            x["draft"] = True
            x["why"] = list(x["why"]) + miss
        now = {1: "in the game", 0: "a draft"}.get(logged.get(x["key"]), "not imported here")
        to = "a draft: " + "; ".join(x["why"]) if x["draft"] else "in the game"
        said["the quests (written again by import_quests.py's plan and write)"].append(f"{x['key']}: {now} -> {to}")
    return p if p["quests"] else None


def isle_targets(db, talked_to, said):
    """The isle's rows to give the NPC bit: (id, name, npc_type)."""
    names = [n.lower() for n in talked_to]
    marks = ",".join("?" * len(names))
    rows = db.execute(f"SELECT id, npc_name, COALESCE(npc_type, 0) FROM npcs WHERE world = ? AND (lower(npc_name) IN ({marks}) "
                      "OR (modelid = ? AND lower(npc_name) LIKE '%essence%')) ORDER BY id",
                      [SKY, *names, PORTAL_BODY]).fetchall()
    found = {r[1].lower() for r in rows}
    for n in talked_to:
        if n.lower() not in found:
            said["an isle NPC not here (left)"].append(n)
    return [r for r in rows if not r[2] & NPC_BIT]


def quest_uses(db, name):
    """How the quests use an NPC of the name: (kills, talks), each a list."""
    kills, talks = [], []
    if not has_table(db, "queststeps"):
        return kills, talks
    for qid, step, kind, ev in db.execute("SELECT questid, step, kind, events FROM queststeps WHERE lower(target) = lower(?)", (name,)):
        (kills if kind in ("kill", "loot") else talks).append(f"quest {qid} step {step} {kind}")
    if has_table(db, "queststeptargets"):
        for qid, step in db.execute("SELECT questid, step FROM queststeptargets WHERE lower(target) = lower(?)", (name,)):
            kills.append(f"quest {qid} step {step} (another creature of its)")
    for qid, step, ev in db.execute("SELECT questid, step, events FROM queststeps WHERE events LIKE ?", (f"%{name}%",)):
        try:
            evs = json.loads(ev) if ev else []
        except ValueError:
            continue
        for e in evs:
            if str(e.get("npc", "")).lower() == name.lower() and (e.get("do") == "attack" or (e.get("do") == "spawn" and e.get("hostile"))):
                kills.append(f"quest {qid} step {step} {e.get('do')}")
    for (qid,) in db.execute("SELECT questid FROM quests WHERE lower(giver) = lower(?)", (name,)):
        talks.append(f"quest {qid} giver")
    return kills, talks


def plan_isle(db, talked_to, ch, said):
    k = npc_key(WORKERS)
    if not db.execute("SELECT count(*) FROM npcs WHERE world = ? AND lower(trim(npc_name)) = ?", (SKY, k)).fetchone()[0]:
        said["no elemental workers on the isle here"].append(WORKERS)
    elif has_table(db, "npcaggro") and db.execute("SELECT count(*) FROM npcaggro WHERE lower(trim(npc_name)) = ?", (k,)).fetchone()[0]:
        said["an npcaggro row for the workers there already (left as it is)"].append(WORKERS)
    else:
        ch.append(dict(tbl="npcaggro", action="insert", ref={"npc_name": WORKERS}, note=WORKERS,
                       row=dict(npc_name=WORKERS, aggressive=0, radius=0.0, source=SOURCE,
                                note="the Plane of Sky's main isle, round the merchants players come to "
                                     "(tools/fix_eqoaemu_places.py): does not attack on sight; still attackable")))
    rows = isle_targets(db, talked_to, said)
    by_name = collections.defaultdict(list)
    for rid, name, t in rows:
        by_name[name].append((rid, t))
    for name, rs in sorted(by_name.items()):
        kills, talks = quest_uses(db, name)
        if kills:
            said["an isle NPC a quest kills (left without the NPC bit)"].append(f"{name}: {', '.join(kills)}")
            continue
        said["the isle's NPCs given the NPC bit, and what the quests do with them"].append(
            f"{name} x{len(rs)}: " + (", ".join(talks) if talks else "no quest names it"))
        for rid, t in rs:
            ch.append(dict(tbl="npcs", action="update", ref={"id": rid}, before={"npc_type": t}, set={"npc_type": t | NPC_BIT},
                           note=f"{name}: the NPC bit"))


def plan(db, recasts=None, gems=None, chambers=None, portal=PORTAL, keys=QUEST_KEYS, talked_to=TALKED_TO, durations=None,
         mirrors=None, makes=None, cures=None):
    """Every change --apply would make, in order, and the report's lines."""
    ch, said = [], collections.defaultdict(list)
    plan_recasts(db, RECASTS if recasts is None else recasts, ch, said)
    plan_tower(db, GEMS if gems is None else gems, CHAMBERS if chambers is None else chambers, ch, said)
    q = plan_quests(db, portal, keys, said)
    plan_isle(db, talked_to, ch, said)
    plan_durations(db, DURATIONS if durations is None else durations, ch, said)
    plan_mirrors(db, MIRRORS if mirrors is None else mirrors, ch, said)
    plan_ours(db, MAKES if makes is None else makes, CURES if cures is None else cures, ch, said)
    return dict(changes=ch, quests=q, said=dict(said))


# ---- the quests' rows --------------------------------------------------------------

def rows_at(db, table, where, args):
    """The rows, each with its rowid (rowid_), in rowid order."""
    cur = db.execute(f"SELECT rowid AS rowid_, * FROM {table} WHERE {where} ORDER BY rowid", args)
    cols = [d[0] for d in cur.description]
    return [dict(zip(cols, r)) for r in cur.fetchall()]


def put_rows(db, table, rows):
    for r in rows:
        r = dict(r)
        rid = r.pop("rowid_")
        db.execute(f"INSERT INTO {table} (rowid, {', '.join(r)}) VALUES (?, {', '.join('?' * len(r))})", [rid, *r.values()])


def quest_state(db, key):
    """The quest the import keeps under key: its mark, its quests row and its
    rows of QUEST_ROWS, each with its rowid; None when none is."""
    if not has_table(db, "quest_import_quests"):
        return None
    mark = rows_at(db, "quest_import_quests", "key = ?", (key,))
    if not mark:
        return None
    qid = mark[0]["questid"]
    out = dict(mark=mark[0], quest=(rows_at(db, "quests", "questid = ?", (qid,)) or [None])[0])
    for t in QUEST_ROWS:
        out[t] = rows_at(db, t, "questid = ?", (qid,)) if has_table(db, t) else []
    return out


def bare(state):
    """A quest's state without its rowids: what an import writes again alike."""
    if state is None:
        return None

    def strip(r):
        return None if r is None else {k: v for k, v in r.items() if k != "rowid_"}
    return {k: ([strip(r) for r in v] if isinstance(v, list) else strip(v)) for k, v in state.items()}


def put_quest_back(db, key, now, was):
    """The quest under key as was: now's rows taken out, was's put back at
    their own rowids (was None: the quest taken out alone)."""
    if now is not None:
        qid = now["mark"]["questid"]
        db.execute("DELETE FROM quests WHERE questid = ?", (qid,))
        for t in QUEST_ROWS:
            if has_table(db, t):
                db.execute(f"DELETE FROM {t} WHERE questid = ?", (qid,))
        db.execute("DELETE FROM quest_import_quests WHERE key = ?", (key,))
    if was is None:
        return
    if was["quest"] is not None:
        put_rows(db, "quests", [was["quest"]])
    for t in QUEST_ROWS:
        if was.get(t):
            put_rows(db, t, was[t])
    put_rows(db, "quest_import_quests", [was["mark"]])


# ---- apply and undo ----------------------------------------------------------------

def row_now(db, tbl, ref, cols):
    w = " AND ".join(f"{c} = ?" for c in ref)
    r = db.execute(f"SELECT {', '.join(cols)} FROM {tbl} WHERE {w}", list(ref.values())).fetchone()
    return None if r is None else dict(zip(cols, r))


def log(db, run, tbl, ref, action, before, after, note=""):
    db.execute(f"INSERT INTO {LOG} (run, tbl, ref, action, before, after, note) VALUES (?, ?, ?, ?, ?, ?, ?)",
               (run, tbl, ref if isinstance(ref, str) else dumps(ref), action,
                None if before is None else dumps(before), None if after is None else dumps(after), note or ""))


def counters(db):
    return {t: (lambda r: None if r is None else r[0])(db.execute("SELECT seq FROM sqlite_sequence WHERE name = ?", (t,)).fetchone())
            for t in COUNTERS}


def apply(db, p, when=None):
    """Every planned change whose row is still as planned, in one
    transaction, each logged under this run's time; returns (written, stale,
    what the quests' write said)."""
    when = when or datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")
    made_log = not has_table(db, LOG)
    wrote = stale = 0
    quests = None
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_DDL)
        last = db.execute(f"SELECT max(run) FROM {LOG}").fetchone()[0]
        if last is not None and when <= last:
            raise SystemExit(f"the last run was applied at {last}, this one at {when}: run it again in a second")
        seqs = counters(db)
        for c in p["changes"]:
            tbl, act = c["tbl"], c["action"]
            if not has_table(db, tbl):
                stale += 1
                continue
            if act == "insert":
                row = dict(c["row"])
                ref = {k: row.get(k) for k in KEYS[tbl]}
                if None not in ref.values() and row_now(db, tbl, ref, list(ref)) is not None:
                    stale += 1
                    continue
                cols = [k for k in row if k in columns(db, tbl)]
                cur = db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                                 [row[k] for k in cols])
                if tbl == "loottables":
                    row["id"] = cur.lastrowid
                    ref = {"id": cur.lastrowid}
                log(db, when, tbl, ref, "insert", None, {k: row[k] for k in cols + (["id"] if tbl == "loottables" else [])},
                    c.get("note"))
            elif act == "update":
                if row_now(db, tbl, c["ref"], list(c["before"])) != c["before"]:
                    stale += 1
                    continue
                w = " AND ".join(f"{k} = ?" for k in c["ref"])
                db.execute(f"UPDATE {tbl} SET {', '.join(f'{k} = ?' for k in c['set'])} WHERE {w}",
                           [*c["set"].values(), *c["ref"].values()])
                log(db, when, tbl, c["ref"], "update", c["before"], c["set"], c.get("note"))
            wrote += 1
        if p.get("quests"):
            n, quests = write_quests(db, p["quests"], when)
            wrote += n
        if wrote:
            for t, s in seqs.items():
                log(db, when, "sqlite_sequence", t, "counter", {"seq": s}, None)
        elif made_log:
            db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return wrote, stale, quests


def write_quests(db, qp, when):
    """The quests written again by import_quests.py's write in this
    transaction, each logged as it was and as written; a write that changes
    nothing (a run again) is taken back and logged not at all. Returns
    (the quests changed, what the write said)."""
    keys = [x["key"] for x in qp["quests"]]
    before = {k: quest_state(db, k) for k in keys}
    meta = rows_at(db, "quest_import_meta", "1", ()) if has_table(db, "quest_import_meta") else None
    db.execute("SAVEPOINT quests")
    written, kept, made = iq.write(db, qp, own=False)
    after = {k: quest_state(db, k) for k in keys}
    changed = [k for k in keys if bare(after[k]) != bare(before[k])]
    if not changed and not made:
        db.execute("ROLLBACK TO quests")
        db.execute("RELEASE quests")
        return 0, dict(written=0, kept=kept, made=0)
    db.execute("RELEASE quests")
    log(db, when, "quest_import_meta", "*", "meta", meta,
        rows_at(db, "quest_import_meta", "1", ()) if has_table(db, "quest_import_meta") else None)
    for k in changed:
        log(db, when, "quests", k, "quest", before[k], after[k])
    return len(changed), dict(written=written, kept=kept, made=made, changed=changed)


def undo(db):
    """Takes the last run back: (put back, left changed since, the run)."""
    if not has_table(db, LOG):
        sys.exit("no run to undo here")
    back, left, counted = 0, [], []
    db.execute("BEGIN IMMEDIATE")
    try:
        run = db.execute(f"SELECT max(run) FROM {LOG}").fetchone()[0]
        rows = db.execute(f"SELECT tbl, ref, action, before, after FROM {LOG} WHERE run = ? ORDER BY seq DESC", (run,)).fetchall()
        for tbl, ref, act, before, after in rows:
            before = None if before is None else json.loads(before)
            after = None if after is None else json.loads(after)
            if act == "insert":
                ref = json.loads(ref)
                now = row_now(db, tbl, ref, list(after))
                if now is None:
                    continue
                if now != after:
                    left.append(f"{tbl} {dumps(ref)}")
                    continue
                db.execute(f"DELETE FROM {tbl} WHERE {' AND '.join(f'{k} = ?' for k in ref)}", list(ref.values()))
            elif act == "update":
                ref = json.loads(ref)
                if row_now(db, tbl, ref, list(after)) != after:
                    left.append(f"{tbl} {dumps(ref)}")
                    continue
                db.execute(f"UPDATE {tbl} SET {', '.join(f'{k} = ?' for k in before)} WHERE "
                           f"{' AND '.join(f'{k} = ?' for k in ref)}", [*before.values(), *ref.values()])
            elif act == "quest":
                now = quest_state(db, ref)
                if bare(now) != bare(after):
                    left.append(f"quest {ref}")
                    continue
                put_quest_back(db, ref, now, before)
            elif act == "meta":
                now = rows_at(db, "quest_import_meta", "1", ()) if has_table(db, "quest_import_meta") else None
                if bare_rows(now) != bare_rows(after):
                    left.append("quest_import_meta (another import since)")
                    continue
                db.execute("DELETE FROM quest_import_meta")
                put_rows(db, "quest_import_meta", before or [])
            elif act == "counter":
                counted.append((ref, before["seq"]))  # after every row's taken back
                continue
            back += 1
        # the counters as they were, unless a row past them is still there
        for t, s in counted:
            top = db.execute(f"SELECT COALESCE(max({COUNTERS[t]}), 0) FROM {t}").fetchone()[0]
            if s is None:
                if top == 0:
                    db.execute("DELETE FROM sqlite_sequence WHERE name = ?", (t,))
            else:
                db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = ?", (max(s, top), t))
        db.execute(f"DELETE FROM {LOG} WHERE run = ?", (run,))
        if db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0] == 0:
            db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return back, left, run


def bare_rows(rows):
    return None if rows is None else sorted(tuple(sorted((k, v) for k, v in r.items() if k != "rowid_")) for r in rows)


# ---- the report ------------------------------------------------------------------

def report(p):
    by = collections.Counter(f"{c['tbl']} {c['action']}" for c in p["changes"])
    print(f"changes: {len(p['changes'])} ({', '.join(f'{k} {n}' for k, n in sorted(by.items()))})"
          + (f"; the quests: {len(p['quests']['quests'])} to write again" if p.get("quests") else ""))
    for c in p["changes"]:
        what = c.get("note") or ""
        if c["action"] == "update":
            print(f"  {c['tbl']} {dumps(c['ref'])}: {dumps(c['before'])} -> {dumps(c['set'])}  {what}")
        else:
            r = c["row"]
            short = {k: r[k] for k in r if k in ("patternid", "itemname", "line", "spell", "place", "x", "y", "z",
                                                   "tablename", "npc_name", "chance", "aggressive")}
            print(f"  {c['tbl']} insert {dumps(short)}")
    for k, v in p["said"].items():
        print(f"  {k}: {len(v)}")
        for x in v:
            print(f"      {x}")


def backup(path):
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-eqoaemu-places"
    src, out = sqlite3.connect(path, timeout=30), sqlite3.connect(dst)
    try:
        src.backup(out)
    finally:
        out.close()
        src.close()
    return dst


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--no-backup", action="store_true")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    if (a.apply or a.undo) and not a.no_backup:
        print(f"copy: {backup(a.db)}")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        back, left, run = undo(db)
        print(f"the run of {run}: put back {back}" + (f"; left, changed since: {', '.join(left)}" if left else "")
              + "; restart the server")
        return
    p = plan(db)
    report(p)
    if a.apply:
        wrote, stale, q = apply(db, p)
        print(f"written {wrote}; {stale} changed since the plan, left" + (f"; the quests: {q}" if q else "")
              + ("; restart the server" if wrote else ""))
    else:
        print("(report only: --apply to write it)")


if __name__ == "__main__":
    main()
