#!/usr/bin/env python3
"""The Plane of Disease's quests as the wiki has them (the owner, 2026-11-12, sending wiki.eqoa.live's
"Plane of Disease: Weapon Quest", "Plane of Disease: Ability Quest" and "Honcho Ulgurglch" (its "Plane of Disease:
Noroxus Quest"), with Skywalker's petitions of the day; petition 1678: "rename this npc Rawlg the Carnage Hound").

The quests are tools/quests/data/portal.json's, as now edited, written again through tools/import_quests.py for
their keys alone (found by key in quest_import_quests and checked by giver and name; refused when a GM has changed
one since its import, or one would be a draft):

  - Portal/plane-of-disease/weapon, "The Plague Weapons" (Witchdoctor Yurplag): level 60, The Pusling 1000; ONE give
    step (wax dust, acidic blood bile, matriarch's front legs, matriarch's mandible), so the pieces are taken in any
    order and before the quest is logged (a hand-in, done on accepting: dostep); rewards by archetype, The Bloodsabre
    (rangers, bards, rogues) and the Thorn of Deatharn (monks) made, who had nothing.
  - Portal/plane-of-disease/spell, "Yarfsnoogle's Anatomy" (Yarfsnoogle): any level, The Pusling 2000, no quest
    before it; ONE give step of the eight pieces, then Grum; the cleric's Purifying Winds of Nife (had nothing), the
    rogue's Virulent Blade (was Virulent Blade Assassin); Grum gives each class a piece of his (the tunic, helm and
    boots made: rangers, bards and rogues had nothing).
  - two new repeatable hand-ins at Yarfsnoogle's Pet (placed beside Yarfsnoogle, wherever he stands): the seven
    non-dust pieces for Yarfsnoogle's bag, and the bag back for them.
  - Portal/plane-of-disease/noroxus-1 and -2 (Honcho Ulgurglch): level 51, The Pusling 1000; the jellyfish goo of
    the cyanea virullatas and the Last House toxicologists, the wiki's names.
  - Portal/plane-of-disease/bert (Witchdoctor Muglchlur; the wiki's "Plane of Disease: Bertoxxulous Quest"): The
    Pusling 1000 (was 1); the scepter is the wiki's Bonescepter; Bertoxxulous spawns only at his throne: the step at
    the throne fires trigger "Bertoxxulous Rises" (he stays 60 minutes; the throne answers no one again for 3 days
    16 hours, killed or not; a lock, which a restart lets go), and the Bloody Portal Opens trigger's shut time is the
    wiki's Closed Portal respawn, 60 minutes to 3 hours (was 45).
  The Pusling standings asked are PUSLING_GATES below, one edit away.

Beside the quests, as data:
  - renames, every row keyed by the name (npcs in world 5, npcclass, npctemplate, npcmove, npcundead, npccharm,
    lootpool, lootcreature, npcloot, npcaggro, factionhits, npcfaction, npcdialog, npcsocial, npcspawnonly,
    npcdeath, npctint, queststeptargets, queststeps' targets, camps' kinds, triggers' steps): Rawgl the Carnage
    Hound -> Rawlg (the wiki's and the game's own items' spelling: Rawlg's mace, shortsword, daggers; item 5366
    "Rawgl's shortsword" is a misspelt twin of 5364 "Rawlg's shortsword" and is left), cillieane viturille -> cyanea virullata, plague toxicologist -> last house toxicologist;
    the items viturille skin -> jellyfish goo, front legs of the Mantis Matriarch -> matriarch's front legs,
    mandibles of the Millipede Matriarch -> matriarch's mandible (quest_import_items with them);
  - the pieces drop from the corpses (loot tables "pod quest <creature>"): each anatomy piece from its giver at
    100% a kill, primary only (Rotwood the Hollow the skull too); each insect piece from its boss, two rolls at
    75% (the wiki's 150%); putrid carapace 75% and fetid meat 15% (made) off the larvae, the diseased insects and
    the bosses;
  - each insect area's cycle: a larva's death raises a diseased insect of its kind 25% (three at most), a diseased
    insect's death its monarch or matriarch 15% (one at a time, gone after 30 minutes unkilled): npcdeath and
    gmtriggers rows; the four diseased insects made (each its boss's body at size 1, level 61, the 'pod roamer'
    template, +2 The Pusling a kill: the wiki's Diseased Moth), the four bosses spawn-only;
  - the reward records as the wiki gives them: NO TRADE and LORE, durability 14,100 (the gear HP), the totems'
    and the cleaver's 70; the Totem of Avhi worn by alchemists too, the robe by monks, druids and shamans, the
    shield by warriors; the shield's power -75 and the robe's and shield's fire and cold -10 (were 63);
  - The Pusling on a kill (factionhits, to the killer): Bertoxxulous, Noroxus, Uzrathixius and Poxathixius +100,
    Queen Qy'tha +75 (their wiki pages); the Plane of Disease's spells table rolled 99% a Bertoxxulous kill, 35% a
    Noroxus, 75% a twin (their pages' "Chance to Drop"; were 25, 25, 50, 50); Bertoxxulous's soul dust two a kill;
  - the givers' levels 65 (Honcho Ulgurglch, Yarfsnoogle, Witchdoctor Yurplag);
  - characters on the two rewritten quests: a step before the hand-in moved to the hand-in (1), Grum's to 2.

Every row it changes is found by comparing the tables before and after (tools/seed_guide_gaps.py's machinery,
set "pod-quests-1112" in guide_set_log), so --undo is exact; a gm_audit line a row. Report only by default;
--apply in one transaction; a second --apply does nothing; --undo. Quests, NPCs, triggers, deaths, templates and
factions are read at the server's start: run it with the server stopped or as a keep restart's before.sh.

    tools/pod_quests_20261112.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import import_quests  # noqa: E402
import seed_guide_gaps as sgg  # noqa: E402

SET = "pod-quests-1112"
WORLD = 5
WHO, VIA = "dab", "tool"
DETAIL = "The Plane of Disease's quests as the wiki has them (the owner, 2026-11-12): tools/pod_quests_20261112.py"
SOURCE = "pod-quests-1112"
PORTAL = os.path.join(HERE, "quests", "data", "portal.json")

sgg.TABLES = tuple(sgg.TABLES) + tuple(t for t in ("charquests", "npctint", "campspawns", "lootpool") if t not in sgg.TABLES)

# THE PUSLING STANDING EACH QUEST ASKS (the wiki's; the owner is deciding whether to keep them: one edit here, then the
# tool run again after an --undo). A key left out asks none. These override the file's factions_required.
PUSLING_GATES = {
    "Portal/plane-of-disease/weapon": 1000,
    "Portal/plane-of-disease/spell": 2000,
    "Portal/plane-of-disease/noroxus-1": 1000,
    "Portal/plane-of-disease/noroxus-2": 1000,
    "Portal/plane-of-disease/bert": 1000,
}
# The Pusling on a kill (factionhits: the killer's standing, the engine's way; the wiki's is everyone near): the NPC
# pages' "Opposing: The Pusling (+N)"; the diseased insects' +2 are made in make_npcs
PUSLING_KILLS = {"Bertoxxulous": 100, "Noroxus": 100, "Uzrathixius": 100, "Poxathixius": 100, "Queen Qy'tha": 75}
# the Plane of Disease's spell table's chance a kill, the NPC pages' "Chance to Drop"
SPELL_TABLE, SPELL_CHANCES = "pod spells", {"bertoxxulous": 99, "noroxus": 35, "uzrathixius": 75, "poxathixius": 75}
# the throne: Bertoxxulous an hour, then none for 3 days 16 hours (22 waits of the engine's longest, 4 h)
BERT, THRONE, BERT_TRIGGER, BERT_MINUTES, BERT_WAITS = "Bertoxxulous", "Throne of Bertoxxulous", "Bertoxxulous Rises", 60, 22
# the Closed Portal's respawn, the wiki's 60 minutes to 3 hours (the Bloody Portal Opens trigger's last wait: 45 min to 3 h)
PORTAL_TRIGGER, PORTAL_WAIT = "Bloody Portal Opens", ({"seconds": 2700, "upto": 10800}, {"seconds": 3600, "upto": 10800})

# key -> (giver, name) as imported
QUESTS = {
    "Portal/plane-of-disease/spell": ("Yarfsnoogle", "Yarfsnoogle's Anatomy"),
    "Portal/plane-of-disease/weapon": ("Witchdoctor Yurplag", "The Plague Weapons"),
    "Portal/plane-of-disease/noroxus-1": ("Honcho Ulgurglch", "The Fuming Portal"),
    "Portal/plane-of-disease/noroxus-2": ("Honcho Ulgurglch", "Blood of Noroxus"),
    "Portal/plane-of-disease/bert": ("Witchdoctor Muglchlur", "Bertoxxulous and the Throne of Plague"),
}
# the rows of a quest whose logged digest is stale, as they stood on 2026-11-11 (heirloom-20261111g): the same rows the
# file wrote (written again from it, nothing differs), so no GM's change
ROWS_AS_FILE = {"Portal/plane-of-disease/spell": "34d05e9247d236efebebe29d78606683",
                "Portal/plane-of-disease/bert": "0cb8bc618b5508f6868c996a80a48af5"}
NEW_QUESTS = ("Portal/plane-of-disease/spell-bag", "Portal/plane-of-disease/spell-unbag")
# the steps a character on the old quest moves to: old step -> new step
STEP_MAP = {"Portal/plane-of-disease/spell": {**{n: 1 for n in range(1, 10)}, 10: 2},
            "Portal/plane-of-disease/weapon": {n: 1 for n in range(1, 6)}}
NEW_ITEMS = ("The Bloodsabre", "Thorn of Deatharn", "Fetid Scaled Flesh Tunic", "Fetid Scaled Flesh Helm", "Fetid Flesh Boots",
             "Yarfsnoogle's bag")

NPC_RENAMES = [("Rawgl the Carnage Hound", "Rawlg the Carnage Hound"),
               ("cillieane viturille", "cyanea virullata"),
               ("plague toxicologist", "last house toxicologist")]
# the tables keyed by an NPC's name (NPCClassKey: lowercase), and their name column
NAME_KEYED = [("npcclass", "name"), ("npctemplate", "name"), ("npcmove", "name"), ("npcundead", "name"), ("npccharm", "name"),
              ("lootpool", "npc_name"), ("lootcreature", "npc_name"), ("npcloot", "npc_name"), ("npcaggro", "npc_name"),
              ("factionhits", "npc_name"), ("npcfaction", "name"), ("npcdialog", "name"), ("npcsocial", "npc_name"),
              ("npcspawnonly", "name"), ("npcdeath", "name"), ("npctint", "name"), ("queststeptargets", "target")]
ITEM_RENAMES = [("viturille skin", "jellyfish goo", "Goo of a cyanea virullata from the nox pools. It lets you through the fuming portal."),
                ("front legs of the Mantis Matriarch", "matriarch's front legs", "The front legs of the Mantis Matriarch."),
                ("mandibles of the Millipede Matriarch", "matriarch's mandible", "The mandible of the Millipede Matriarch."),
                ("scepter of plague", "Bonescepter", "Made by Witchdoctor Muglchlur from the Queen's stinger. The Throne of Bertoxxulous answers it.")]

NOTRADE_LORE = {"trade": 1, "lore": 1, "maxhp": 14100}
# name -> {column: (before, after)}; before None: whatever it is
ITEM_FIXES = {
    "gory cleaver of Mavis": {**{k: (None, v) for k, v in NOTRADE_LORE.items()}, "duration": (65, 70)},
    "Totem of Avhi": {**{k: (None, v) for k, v in NOTRADE_LORE.items()}, "duration": (65, 70), "classuse": (15360, 31744)},
    "Totem of Toluwon": {**{k: (None, v) for k, v in NOTRADE_LORE.items()}, "duration": (65, 70)},
    "Fetid Flesh Shield": {**{k: (None, v) for k, v in NOTRADE_LORE.items()}, "duration": (65, 75), "classuse": (12, 13),
                           "POWMAX": (63, -75), "FR": (63, -10), "CR": (63, -10)},
    "fetid flesh robe": {**{k: (None, v) for k, v in NOTRADE_LORE.items()}, "classuse": (30976, 31376), "FR": (63, -10), "CR": (63, -10)},
}
# the made rewards' figures the import's "set" cannot give
# (the wiki's figures whole: every stat it does not give 0, not its like record's)
STATS = ("str", "sta", "agi", "wis", "dex", "cha", "intelligence", "HPMAX", "POWMAX", "PoT", "HoT", "AC", "PR", "DR", "FR", "CR", "LR", "AR")


def wiki_stats(**given):
    return {**{c: 0 for c in STATS}, **given}


MADE_FIXES = {
    "The Bloodsabre": {"weapondamage": 720, "duration": 70, **wiki_stats(HPMAX=120, POWMAX=80)},
    "Thorn of Deatharn": {"weapondamage": 720, "duration": 70, "equipslot": 14, **wiki_stats(HPMAX=120, AC=80)},
    "Fetid Scaled Flesh Tunic": {"duration": 65, **wiki_stats(AC=310, str=10, sta=10, dex=10, cha=10, intelligence=10, HPMAX=150,
                                                              DR=30, FR=-10, CR=-10)},
    "Fetid Scaled Flesh Helm": {"duration": 60, **wiki_stats(AC=82, str=10, agi=10, dex=10, HPMAX=150, DR=45, FR=-10, CR=-10)},
    "Fetid Flesh Boots": {"duration": 60, **wiki_stats(AC=70, agi=10, wis=10, dex=10, HPMAX=150, DR=30, FR=-10, CR=-10)},
}
GIVER_LEVELS = {"Honcho Ulgurglch": 65, "Yarfsnoogle": 65, "Witchdoctor Yurplag": 65}

# insect kind -> its boss, its quest piece
INSECTS = [("moth", "moth monarch", "wax dust"), ("locust", "locust monarch", "acidic blood bile"),
           ("mantis", "mantis matriarch", "matriarch's front legs"), ("millipede", "millipede matriarch", "matriarch's mandible")]
LARVA_TO_DISEASED, DISEASED_TO_BOSS, DISEASED_MAX, BOSS_MINUTES, DISEASED_MINUTES = 25, 15, 3, 30, 30
DISEASED_LEVEL = 61
# the anatomy pieces: creature -> piece
ANATOMY = [("Rallius Rattican", "fetid rotting femur"), ("Lich Overlord", "fetid rotting organ"), ("Fleshripper", "fetid rotting skin"),
           ("Gluboxious the Fetid", "fetid rotting tibia"), ("Rawlg the Carnage Hound", "fetid rotting skull"),
           ("Rotwood the Hollow", "fetid rotting skull"), ("Noroxus", "fetid rotting gland"), ("Queen Qy'tha", "fetid rotting eyeball"),
           ("Bertoxxulous", "soul dust")]
TRASH = [("putrid carapace", "beetle carapace fragment", "A putrid carapace from an insect of the Plane of Disease.", 75),
         ("fetid meat", "huge chunk of meat", "A chunk of fetid meat from an insect of the Plane of Disease.", 15)]
PET, PET_LIKE, PET_OFFSET, PET_LEVEL, PET_SIZE = "Yarfsnoogle's Pet", "Yarfsnoogle", 6.0, 65, 0.8
PUSLING = "The Pusling"


def key(n):
    return (n or "").strip().lower()


def has(db, t):
    return sgg.has(db, t)


def one(db, sql, args=()):
    rows = db.execute(sql, args).fetchall()
    return rows[0] if len(rows) == 1 else None


def item_ids(db, name):
    return [r[0] for r in db.execute("SELECT patternid FROM itempattern WHERE lower(trim(itemname)) = ?", (key(name),))]


def quest_item_id(db, name):
    """The record import_quests resolves the name to (the one the quests name)."""
    r = import_quests.Names(db).item(name)
    return r[0] if r else None


def world_npcs(db, name):
    return db.execute("SELECT id FROM npcs WHERE lower(trim(npc_name)) = ? AND world = ?", (key(name), WORLD)).fetchall()


def load_portal():
    with open(PORTAL) as f:
        d = json.load(f)
    keys = set(QUESTS) | set(NEW_QUESTS)
    quests = [q for q in d["quests"] if q["key"] in keys]
    for q in quests:
        others = [f for f in q.get("factions_required") or [] if f[0] != PUSLING]
        gate = PUSLING_GATES.get(q["key"])
        q["factions_required"] = others + ([[PUSLING, gate]] if gate else [])
    items = [it for it in d["items"] if it["name"] in NEW_ITEMS]
    return {"city": d["city"], "quests": quests, "items": items}


def check(db):
    """Refusals: what is not as this tool expects."""
    bad = []
    logged = {k: (q, dg) for k, q, dg in db.execute("SELECT key, questid, digest FROM quest_import_quests")} if has(db, "quest_import_quests") else {}
    for k, (giver, name) in QUESTS.items():
        if k not in logged:
            bad.append(f"quest {k} was never imported")
            continue
        qid, dg = logged[k]
        r = one(db, "SELECT giver, name FROM quests WHERE questid = ?", (qid,))
        if r != (giver, name):
            bad.append(f"quest {qid} ({k}) is {r}, want {(giver, name)}")
        elif import_quests.digest(db, qid) not in (dg, ROWS_AS_FILE.get(k)):
            bad.append(f"quest {qid} ({k}) was changed since its import (a GM's): this tool would not rewrite it")
    for k in NEW_QUESTS:
        if k in logged:
            bad.append(f"quest {k} is in already")
    for old, new in NPC_RENAMES:
        if len(world_npcs(db, old)) == 0:
            bad.append(f"no {old!r} in world {WORLD}")
        if db.execute("SELECT 1 FROM npcs WHERE lower(trim(npc_name)) = ?", (key(new),)).fetchone():
            bad.append(f"an NPC is named {new!r} already")
        if db.execute("SELECT 1 FROM npcs WHERE lower(trim(npc_name)) = ? AND world != ?", (key(old), WORLD)).fetchone():
            bad.append(f"{old!r} stands outside world {WORLD} too: its name-keyed rows are not this tool's alone")
    for old, new, _ in ITEM_RENAMES:
        if len(item_ids(db, old)) != 1:
            bad.append(f"{len(item_ids(db, old))} items named {old!r}, want 1")
        if item_ids(db, new):
            bad.append(f"an item is named {new!r} already")
    for name, cols in ITEM_FIXES.items():
        pid = quest_item_id(db, name)
        if pid is None:
            bad.append(f"no item {name!r}")
            continue
        row = dict(zip([c[1] for c in db.execute("PRAGMA table_info(itempattern)")], db.execute("SELECT * FROM itempattern WHERE patternid = ?", (pid,)).fetchone()))
        for col, (before, _) in cols.items():
            if before is not None and row[col] != before:
                bad.append(f"item {pid} {name!r} {col} is {row[col]!r}, want {before!r}")
    for n in NEW_ITEMS + tuple(t[0] for t in TRASH) + ("Yarfsnoogle's bag",):
        if item_ids(db, n):
            bad.append(f"an item is named {n!r} already")
    for n, like, _, _ in TRASH:
        if not item_ids(db, like):
            bad.append(f"no item {like!r} to make {n!r} like")
    for name in GIVER_LEVELS:
        if not world_npcs(db, name):
            bad.append(f"no {name!r} in world {WORLD}")
    if len(world_npcs(db, PET_LIKE)) != 1:
        bad.append(f"{len(world_npcs(db, PET_LIKE))} {PET_LIKE!r} in world {WORLD}, want 1 (the pet stands beside him)")
    for n in [PET] + [f"diseased {k}" for k, _, _ in INSECTS]:
        if db.execute("SELECT 1 FROM npcs WHERE lower(trim(npc_name)) = ?", (key(n),)).fetchone():
            bad.append(f"an NPC is named {n!r} already")
    for k, boss, piece in INSECTS:
        if not world_npcs(db, boss):
            bad.append(f"no {boss!r} in world {WORLD}")
        if not world_npcs(db, f"{k} larva"):
            bad.append(f"no {k} larva in world {WORLD}")
        if not item_ids(db, piece) and piece not in [o for _, o, _ in ITEM_RENAMES] and piece not in [n for _, n, _ in ITEM_RENAMES]:
            bad.append(f"no item {piece!r}")
    for creature, piece in ANATOMY:
        c = creature if creature != "Rawlg the Carnage Hound" else "Rawgl the Carnage Hound"
        if not world_npcs(db, c):
            bad.append(f"no {c!r} in world {WORLD}")
        if not item_ids(db, piece):
            bad.append(f"no item {piece!r}")
    if len(world_npcs(db, THRONE)) != 1:
        bad.append(f"{len(world_npcs(db, THRONE))} {THRONE!r} in world {WORLD}, want 1")
    if not world_npcs(db, BERT):
        bad.append(f"no {BERT!r} in world {WORLD}")
    for t in (BERT_TRIGGER,):
        if db.execute("SELECT 1 FROM gmtriggers WHERE name = ?", (t,)).fetchone():
            bad.append(f"a trigger is named {t!r} already")
    if not db.execute("SELECT 1 FROM gmtriggers WHERE name = ?", (PORTAL_TRIGGER,)).fetchone():
        bad.append(f"no trigger {PORTAL_TRIGGER!r}")
    for npc in PUSLING_KILLS:
        if db.execute("SELECT 1 FROM factionhits WHERE lower(npc_name) = ? AND faction = ?", (key(npc), PUSLING)).fetchone():
            bad.append(f"{npc} gives {PUSLING} on a kill already")
    if not world_npcs(db, "Grum"):
        bad.append("no Grum in world 5")
    if not db.execute("SELECT 1 FROM factions WHERE name = ?", (PUSLING,)).fetchone():
        bad.append(f"no faction {PUSLING!r}")
    if not db.execute("SELECT 1 FROM classtemplates WHERE name = 'pod roamer'").fetchone():
        bad.append("no class template 'pod roamer'")
    names = [r[0] for r in db.execute("SELECT name FROM gmtriggers")]
    for t in trigger_names():
        if t in names:
            bad.append(f"a trigger is named {t!r} already")
    return bad


def trigger_names():
    out = []
    for k, boss, _ in INSECTS:
        out += [f"PoD {k.title()} Metamorphosis", f"PoD {boss.title()} Rises"]
    return out


def rename_npcs(db, say):
    for old, new in NPC_RENAMES:
        n = db.execute("UPDATE npcs SET npc_name = ? WHERE lower(trim(npc_name)) = ? AND world = ?", (new, key(old), WORLD)).rowcount
        keyed = 0
        for t, c in NAME_KEYED:
            if has(db, t):
                keyed += db.execute(f'UPDATE "{t}" SET "{c}" = ? WHERE lower(trim("{c}")) = ?', (key(new), key(old))).rowcount
        if has(db, "campspawns"):
            for cid, kinds in db.execute("SELECT npcid, kinds FROM campspawns").fetchall():
                parts = [p.strip() for p in kinds.split(",")]
                if any(key(p) == key(old) for p in parts):
                    db.execute("UPDATE campspawns SET kinds = ? WHERE npcid = ?", (", ".join(new if key(p) == key(old) else p for p in parts), cid))
                    keyed += 1
        for qid, step, target in db.execute("SELECT questid, step, target FROM queststeps WHERE lower(trim(target)) = ?", (key(old),)).fetchall():
            db.execute("UPDATE queststeps SET target = ? WHERE questid = ? AND step = ?", (new if target.strip() != key(old) else key(new), qid, step))
            keyed += 1
        for slot, steps in db.execute("SELECT slot, steps FROM gmtriggers").fetchall():
            if old in steps or key(old) in steps:
                db.execute("UPDATE gmtriggers SET steps = ? WHERE slot = ?", (steps.replace(old, new).replace(key(old), key(new)), slot))
                keyed += 1
        say(f"renamed {old!r} -> {new!r}: {n} npcs rows, {keyed} keyed rows")


def rename_items(db, say):
    for old, new, desc in ITEM_RENAMES:
        pid = item_ids(db, old)[0]
        d = db.execute("SELECT itemdesc FROM itempattern WHERE patternid = ?", (pid,)).fetchone()[0]
        if desc is None:
            desc = d.replace("Rawgl", "Rawlg")
        db.execute("UPDATE itempattern SET itemname = ?, itemdesc = ? WHERE patternid = ?", (new, desc, pid))
        if has(db, "quest_import_items"):
            db.execute("UPDATE quest_import_items SET name = ? WHERE patternid = ?", (new, pid))
        say(f"item {pid} {old!r} -> {new!r}")


def fix_items(db, say):
    for name, cols in ITEM_FIXES.items():
        pid = quest_item_id(db, name)
        sets = ", ".join(f'"{c}" = ?' for c in cols)
        db.execute(f"UPDATE itempattern SET {sets} WHERE patternid = ?", [v for _, v in cols.values()] + [pid])
        say(f"item {pid} {name!r}: " + ", ".join(f"{c} {v}" for c, (_, v) in cols.items()))


def copy_npc(db, src_id, changes):
    cols = [r[1] for r in db.execute("PRAGMA table_info(npcs)")]
    row = dict(zip(cols, db.execute("SELECT * FROM npcs WHERE id = ?", (src_id,)).fetchone()))
    row.pop("id")
    row.update(changes)
    names = list(row)
    cur = db.execute(f'INSERT INTO npcs ({",".join(names)}) VALUES ({",".join("?" * len(names))})', [row[c] for c in names])
    db.execute("UPDATE npcs SET npc_id = id WHERE id = ?", (cur.lastrowid,))
    return cur.lastrowid


def make_npcs(db, say):
    for name, level in GIVER_LEVELS.items():
        n = db.execute("UPDATE npcs SET npc_level = ? WHERE lower(trim(npc_name)) = ? AND world = ?", (level, key(name), WORLD)).rowcount
        say(f"{name}: level {level} ({n} rows)")
    src, x = db.execute("SELECT id, x_coord FROM npcs WHERE lower(trim(npc_name)) = ? AND world = ?", (key(PET_LIKE), WORLD)).fetchone()
    rid = copy_npc(db, src, {"npc_name": PET, "x_coord": x + PET_OFFSET, "npc_level": PET_LEVEL, "size": PET_SIZE})
    say(f"{PET}: row {rid}, beside {PET_LIKE} (row {src})")
    for k, boss, _ in INSECTS:
        bid = world_npcs(db, boss)[0][0]
        name = f"diseased {k}"
        rid = copy_npc(db, bid, {"npc_name": name, "npc_level": DISEASED_LEVEL, "size": 1.0})
        db.execute("INSERT OR REPLACE INTO npcspawnonly (name, note, source) VALUES (?, ?, ?)",
                   (name, f"raised by a {k} larva's death (trigger PoD {k.title()} Metamorphosis): the wiki's spawn cycle", SOURCE))
        db.execute("INSERT OR REPLACE INTO npcspawnonly (name, note, source) VALUES (?, ?, ?)",
                   (boss, f"raised by a diseased {k}'s death (trigger PoD {boss.title()} Rises): the wiki's spawn cycle", SOURCE))
        db.execute("INSERT OR REPLACE INTO npctemplate (name, template, note, source) VALUES (?, 'pod roamer', ?, ?)",
                   (name, f"the wiki's Diseased {k.title()}, level 60-63", SOURCE))
        db.execute("INSERT OR REPLACE INTO factionhits (npc_name, faction, delta, source) VALUES (?, ?, 2, ?)", (name, PUSLING, SOURCE))
        say(f"{name}: row {rid} ({boss}'s body, level {DISEASED_LEVEL}, spawn only, +2 {PUSLING}); {boss} spawn only")


def make_triggers(db, say):
    slot = db.execute("SELECT COALESCE(max(slot), 0) FROM gmtriggers").fetchone()[0]
    for k, boss, _ in INSECTS:
        met, rises = f"PoD {k.title()} Metamorphosis", f"PoD {boss.title()} Rises"
        rows = [
            (met, [{"do": "spawn", "npc": f"diseased {k}", "hostile": True, "count": 1, "max": DISEASED_MAX, "spread": 3.0,
                    "minutes": DISEASED_MINUTES}],
             f"The wiki's Weapon Quest: kill {k} larvae until diseased {k}s come. A larva's death (npcdeath, {LARVA_TO_DISEASED}%) "
             f"raises one where it fell, {DISEASED_MAX} at most within 300, gone after {DISEASED_MINUTES} minutes (ours)."),
            (rises, [{"do": "lock", "lock": f"pod {boss}"},
                     {"do": "spawn", "npc": boss, "hostile": True, "spread": 2.0, "minutes": BOSS_MINUTES},
                     {"do": "broadcast", "text": f"The {boss.title()} rises from the pool!"},
                     {"do": "awaitdead"}],
             f"The wiki's Weapon Quest: kill diseased {k}s until the {boss.title()} spawns ('bosses run on a timer and may "
             f"despawn'). A diseased {k}'s death (npcdeath, {DISEASED_TO_BOSS}%) raises it where it fell, one at a time, gone "
             f"after {BOSS_MINUTES} minutes unkilled (ours)."),
        ]
        for name, steps, notes in rows:
            slot += 1
            db.execute("""INSERT INTO gmtriggers (slot, name, enabled, scope, radius, world, x, y, z, steps, notes)
                VALUES (?, ?, 1, 'near', 200, -1, 0, 0, 0, ?, ?)""", (slot, name, json.dumps(steps, separators=(",", ":")), notes))
            say(f"trigger {slot} {name}")
        db.execute(sgg.NPCDEATH_DDL)
        db.execute("INSERT OR REPLACE INTO npcdeath (name, trigger, chance, note, source) VALUES (?, ?, ?, ?, ?)",
                   (f"{k} larva", met, LARVA_TO_DISEASED, "the wiki's spawn cycle: larvae, then the diseased", SOURCE))
        db.execute("INSERT OR REPLACE INTO npcdeath (name, trigger, chance, note, source) VALUES (?, ?, ?, ?, ?)",
                   (f"diseased {k}", rises, DISEASED_TO_BOSS, "the wiki's spawn cycle: the diseased, then the boss", SOURCE))


def make_loot(db, say):
    cols = [r[1] for r in db.execute("PRAGMA table_info(itempattern)")]
    trash = {}
    for name, like, desc, chance in TRASH:
        row = dict(zip(cols, db.execute("SELECT * FROM itempattern WHERE patternid = ?", (item_ids(db, like)[0],)).fetchone()))
        row.pop("patternid")
        row.update(itemname=name, itemdesc=desc)
        names = list(row)
        cur = db.execute(f'INSERT INTO itempattern ({",".join(names)}) VALUES ({",".join("?" * len(names))})', [row[c] for c in names])
        trash[name] = (cur.lastrowid, chance)
        say(f"item {cur.lastrowid} {name!r} (like {like!r})")

    def table(tname, pid, npcs, chance, rolls):
        db.execute("INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel, source) VALUES (?, ?, 1, 1, 100, ?)", (tname, pid, SOURCE))
        for n in npcs:
            db.execute("INSERT OR REPLACE INTO npcloot (npc_name, tablename, chance, rolls, note, source) VALUES (?, ?, ?, ?, ?, ?)",
                       (key(n), tname, chance, rolls, "the wiki's Plane of Disease quests", SOURCE))

    for creature, piece in ANATOMY:
        # Bertoxxulous's soul dust 200% (his page; EQOA-Data's "Soul Dust x 2"): two sure rolls
        table(f"pod quest {key(creature)}", quest_item_id(db, piece), [creature], 100, 2 if creature == BERT else 1)
    for npc, chance in SPELL_CHANCES.items():
        n = db.execute("UPDATE npcloot SET chance = ? WHERE npc_name = ? AND tablename = ?", (chance, npc, SPELL_TABLE)).rowcount
        if n != 1:
            raise SystemExit(f"refused: {npc} does not roll {SPELL_TABLE!r} (npcloot)")
    for npc, delta in PUSLING_KILLS.items():
        db.execute("INSERT OR REPLACE INTO factionhits (npc_name, faction, delta, source) VALUES (?, ?, ?, ?)", (npc, PUSLING, delta, SOURCE))
    for k, boss, piece in INSECTS:
        table(f"pod quest {boss}", quest_item_id(db, piece), [boss], 75, 2)
    insects = [n for k, boss, _ in INSECTS for n in (f"{k} larva", f"diseased {k}", boss)]
    for name, (pid, chance) in trash.items():
        table(f"pod {name}", pid, insects, chance, 1)
    say(f"loot: {len(ANATOMY)} anatomy tables, {len(INSECTS)} insect tables, {len(trash)} trash tables")


def move_progress(db, ids, say):
    for k, m in STEP_MAP.items():
        qid = ids[k]
        for sid, step in db.execute("SELECT serverid, step FROM charquests WHERE questid = ? AND done = 0", (qid,)).fetchall():
            new = m.get(step, step)
            if new != step:
                db.execute("UPDATE charquests SET step = ?, count = 0 WHERE serverid = ? AND questid = ?", (new, sid, qid))
                say(f"character {sid}: quest {qid} step {step} -> {new}")


def import_quests_now(db, say):
    for k in QUESTS:
        qid = db.execute("SELECT questid FROM quest_import_quests WHERE key = ?", (k,)).fetchone()[0]
        db.execute("UPDATE quest_import_quests SET digest = ? WHERE key = ?", (import_quests.digest(db, qid), k))
    files = [("portal.json", load_portal())]
    p = import_quests.plan(db, files)
    for w in p["warnings"]:
        say("import warning: " + w)
    drafts = [f"{x['key']}: {x['why']}" for x in p["quests"] if x["draft"]]
    if drafts:
        raise SystemExit("refused: a quest would be a draft: " + "; ".join(drafts))
    written, kept, made = import_quests.write(db, p, own=False)
    if kept:
        raise SystemExit(f"refused: the import left {kept} (changed by a GM)")
    say(f"quests: wrote {written}, made {made} items")
    for name, cols in MADE_FIXES.items():
        pid = item_ids(db, name)[0]
        db.execute(f"UPDATE itempattern SET {', '.join(f'"{c}" = ?' for c in cols)} WHERE patternid = ?", list(cols.values()) + [pid])


def bert_throne(db, say):
    """Bertoxxulous only at the throne, as the wiki's quest has him: spawn-only, the quest's show step fires the trigger."""
    tx, ty, tz = db.execute("SELECT x_coord, y_coord, z_coord FROM npcs WHERE lower(trim(npc_name)) = ? AND world = ?",
                            (key(THRONE), WORLD)).fetchone()
    db.execute("INSERT OR REPLACE INTO npcspawnonly (name, note, source) VALUES (?, ?, ?)",
               (key(BERT), f"raised at his throne by the bonescepter (trigger {BERT_TRIGGER}): the wiki's Bertoxxulous Quest", SOURCE))
    steps = [{"do": "lock", "lock": "throne of bertoxxulous"},
             {"do": "spawn", "npc": BERT, "hostile": True, "spread": 8.0, "minutes": BERT_MINUTES},
             {"do": "broadcast", "text": "Bertoxxulous the Plaguebringer rises from his throne!"},
             {"do": "awaitdead"}] + [{"do": "wait", "seconds": 14400}] * BERT_WAITS
    slot = db.execute("SELECT COALESCE(max(slot), 0) FROM gmtriggers").fetchone()[0] + 1
    db.execute("""INSERT INTO gmtriggers (slot, name, enabled, scope, radius, world, x, y, z, steps, notes)
        VALUES (?, ?, 1, 'near', 200, ?, ?, ?, ?, ?, ?)""",
               (slot, BERT_TRIGGER, WORLD, tx, ty, tz, json.dumps(steps, separators=(",", ":")),
                "The wiki's Plane of Disease: Bertoxxulous Quest: the bonescepter at the throne spawns him; he stays 60 minutes, "
                "and the throne answers again 3 days 16 hours on (its respawn), killed or not. Fired by quest 'Bertoxxulous and the "
                "Throne of Plague' step 2. A restart lets the lock go."))
    say(f"trigger {slot} {BERT_TRIGGER}; {BERT} spawn only")
    row = db.execute("SELECT slot, steps FROM gmtriggers WHERE name = ?", (PORTAL_TRIGGER,)).fetchone()
    old, new = PORTAL_WAIT
    st = json.loads(row[1])
    if st[-1] != {"do": "wait", **old}:
        raise SystemExit(f"refused: trigger {PORTAL_TRIGGER!r}'s last step is {st[-1]}, want the wait {old}")
    st[-1] = {"do": "wait", **new}
    db.execute("UPDATE gmtriggers SET steps = ? WHERE slot = ?", (json.dumps(st, separators=(",", ":")), row[0]))
    say(f"trigger {row[0]} {PORTAL_TRIGGER}: shut {new['seconds'] // 60} minutes to {new['upto'] // 3600} hours")


def run(db, spec, say):
    ids = {k: q for k, q, in db.execute("SELECT key, questid FROM quest_import_quests")}
    rename_npcs(db, say)
    rename_items(db, say)
    fix_items(db, say)
    make_npcs(db, say)
    make_triggers(db, say)
    bert_throne(db, say)
    move_progress(db, ids, say)
    import_quests_now(db, say)
    make_loot(db, say)


def audit_rows(db, counts):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    rows = db.execute(f"SELECT tbl, op, rid, old FROM {sgg.LOG} WHERE setname = ? ORDER BY seq", (SET,)).fetchall()
    for tbl, op, rid, old in rows:
        if tbl == "sqlite_sequence" or op == "made":
            continue
        new = db.execute(f'SELECT * FROM "{tbl}" WHERE rowid = ?', (rid,)).fetchone()
        cols = [r[1] for r in db.execute(f'PRAGMA table_info("{tbl}")')]
        after = json.dumps(dict(zip(cols, new)), separators=(",", ":")) if new else None
        action = {"added": "insert", "changed": "update", "gone": "delete"}[op]
        db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                   "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                   (at, acct[0] if acct else 0, WHO, VIA, action, tbl, json.dumps({"rowid": rid}), old or None, after, DETAIL))


def audit_undo(db, n):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', 'guide_set_log', ?, NULL, NULL, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, json.dumps({"setname": SET}), f"undone, {n} rows put back: " + DETAIL))


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    say = print
    try:
        if a.undo:
            if not has(db, sgg.LOG) or not db.execute(f"SELECT 1 FROM {sgg.LOG} WHERE setname = ?", (SET,)).fetchone():
                print("nothing to undo")
                return 0
            sgg.undo(db, SET, say, extra=audit_undo)
            return 0
        if has(db, sgg.LOG) and db.execute(f"SELECT 1 FROM {sgg.LOG} WHERE setname = ?", (SET,)).fetchone():
            print(f"applied already ({SET} is in {sgg.LOG}): --undo first")
            return 0
        bad = check(db)
        if bad:
            print("refused, nothing done:")
            for b in bad:
                print("  " + b)
            return 1
        sgg.run = run  # the set's rows are this tool's, through seed_guide_gaps' snapshot, log and undo
        sgg.apply(db, {"set": SET}, a.apply, say, extra=audit_rows)
        if a.apply:
            print("restart the server (quests, NPCs, triggers, deaths, templates and factions are read at its start)")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
