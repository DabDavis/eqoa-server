#!/usr/bin/env python3
"""The Plane of Disease's flag quest as the wiki has it (the owner, 2026-11-13: "yes match it to the wiki", sending
https://wiki.eqoa.live/index.php?title=Plane_of_Disease:_Portal_Quest; and "also where dust is standing is where something
should block players until they possess the suit").

The quests are tools/quests/data/portal.json's, as now edited, written again through tools/import_quests.py for their keys
alone (refused when a GM changed one since its import, or one would be a draft):

  - flag-1 "The Death Knights' Hearts" and flag-2 "The Last House" (Necrologist Zizclank): level 51 (was 50). Xeptip no
    longer hands out the zombie flesh suit; he says how it is made. The mausoleum's portal lets the quester in without
    the suit (the wiki's first entry: the suit is made inside); the last step, at the Oozing Portal, shows the suit.
    The wiki's names: Deathknight Bileheart (the game's "Blieheart"), Deathknight Halik, Fatima Ubel, Rebo Chilins,
    Medelonia Virel, the Oozing Portal (was the Portal of the Last House), every npcs row and every name-keyed row with
    them. The Oozing Portal's trip on to the Plane of Disease is flag-2's to unlock (no suit asked of the flagged).
  - suit-swatch and suit-sew, two repeatable hand-ins at Fatima Ubel (ours: tailoring is not live): three Stitched
    Zombie Skin for a Zombie Swatch; two Zombie Swatches and a Sinew Cord for a zombie flesh suit. The three items are
    made (tradeable, craft components, stack 20), and the wiki's two tailoring recipes beside them (craftrecipes,
    trade level 10), for when tailoring is.

Beside the quests, as data:
  - the upper halls (the wiki's second floor, beyond the orderly's wall: the step-13 landing), each spot measured on
    the world's ground (worlds/ground/5.*: a floor there, reached from the landing): a Last House Surgeon (the
    physician's body) whose first hurt calls three surgeon's zombies (a class template's summon, below 100%); two
    scourge rats by the first room (they do not see invisibility: no npcaggro row); a Master Surgeon (the physician's
    body, larger) who drops the Sinew Cord every kill; four last house zombies (the Dead Hills' derelict zombies'
    body) who drop a Stitched Zombie Skin 75% of kills. The surgeons attack on sight (npcaggro: their bodies are a
    human's, which would not); the last house orderlies see invisibility (npcaggro see_invis, a row of no aggro word).
    Respawns as the house's: the surgeons 90 s (its named), the rest the server's.
  - the Last House Physician respawns every 10 minutes (Skywalker's ticket 1906; was 90 s).
  - the ward: an invisible wall (CHAR.ESF 0x6F412342, size 2: a box 10 a side) across the lower hall where Dust stood,
    the one way from the Last House's lower halls up to the Oozing Portal; a questwalls row of the suit (item 1012):
    passed by whoever carries or wears it, by whoever finished The Last House (the owner, 2026-11-13: "let flagged
    players through": the row's quest, past its last step), by a GM, and by anyone on its far side (its inner boxes:
    the upper halls and the lower hall's west arm up to the wall's middle), so no one is shut in; one stopped is told
    its line once.

Every row it changes is found by comparing the tables before and after, logged with its old and new values (table
pod_wiki_1113_log), a gm_audit line a row; --undo puts each back and refuses, writing nothing, when a row it would put
back has changed since (a GM's edit, a player's progress in a quest it made). Report only by default; --apply in one
transaction; a second --apply does nothing. Quests, NPCs, templates, walls and recipes are read at the server's start:
run it with the server stopped or as a keep restart's before.sh, on a server built from the branch with the item wall
(it adds questwalls' item, inner and text columns when the table lacks them, as that server's start would).

    tools/pod_wiki_20261113.py --db eqoa.db [--apply | --undo]
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

LOG = "pod_wiki_1113_log"
WORLD = 5
WHO, VIA = "dab", "tool"
SRC = "pod-wiki-1113"
DETAIL = "The Plane of Disease's flag quest as the wiki has it (the owner, 2026-11-13): tools/pod_wiki_20261113.py"
PORTAL = os.path.join(HERE, "quests", "data", "portal.json")

TABLES = tuple(dict.fromkeys(tuple(sgg.TABLES) + ("questwalls", "craftrecipes", "npcrespawn", "classtemplates", "campspawns",
                                                  "npctint", "lootpool", "charquests")))

# key -> (giver, name) as imported; the new keys
QUESTS = {"Portal/plane-of-disease/flag-1": ("Necrologist Zizclank", "The Death Knights' Hearts"),
          "Portal/plane-of-disease/flag-2": ("Necrologist Zizclank", "The Last House")}
NEW_QUESTS = ("Portal/plane-of-disease/suit-swatch", "Portal/plane-of-disease/suit-sew")
NEW_ITEMS = ("Stitched Zombie Skin", "Zombie Swatch", "Sinew Cord")
SUIT, SUIT_NAME = 1012, "zombie flesh suit"

# (old, new, world): every npcs row of the name in that world, and the rows keyed by the name
NPC_RENAMES = [("Fatima", "Fatima Ubel", 5), ("Rebo", "Rebo Chilins", 5), ("Medelonia", "Medelonia Virel", 5),
               ("Portal of the Last House", "Oozing Portal", 5), ("Deathknight Blieheart", "Deathknight Bileheart", 0),
               ("deathknight Halik", "Deathknight Halik", 0)]
NAME_KEYED = [("npcclass", "name"), ("npctemplate", "name"), ("npcmove", "name"), ("npcundead", "name"),
                ("npccharm", "name"), ("lootpool", "npc_name"), ("lootcreature", "npc_name"), ("npcloot", "npc_name"),
                ("npcaggro", "npc_name"), ("factionhits", "npc_name"), ("npcfaction", "name"), ("npcdialog", "name"),
                ("npcsocial", "npc_name"), ("npcspawnonly", "name"), ("npcdeath", "name"), ("npctint", "name"),
                ("queststeptargets", "target")]

# The upper halls, measured on worlds/ground/5.ground and 5.walls (cmd probes, the note): each (x, z) a floor at y, and
# reached on foot from the step-13 landing (4545, -1.3, 3676) and from the Oozing Portal (4540, 3612).
SURGEON = ("Last House Surgeon", 57, (4564.0, -0.94, 3648.0), 1.0)       # the N-S hall between the landing and the portal
MASTER = ("Master Surgeon", 58, (4590.0, -0.94, 3632.0), 1.2)           # the E-W hall from the ramp, by the junction
RATS = [(4562.0, -2.69, 3694.0), (4566.0, -2.69, 3700.0)]               # the passage south of the landing's room
ZOMBIES = [(4540.0, -1.31, 3718.0), (4555.0, -1.29, 3730.0), (4575.0, -0.95, 3718.0), (4590.0, -0.94, 3718.0)]  # the south hall
ZOMBIE, ADD, ZOMBIE_LEVEL, ADD_LEVEL = "last house zombie", "surgeon's zombie", 56, 55
ZOMBIE_BODY_OF = "derelict zombie"          # the Dead Hills' Sanitorium's zombies (world 1): their body, 465275276
SURGEON_BODY_OF = "Last House Physician"    # the house's own staff's body
RAT_OF = "scourge rat"
TEMPLATE_OF = "scourge rat"                # the house's neighbours' template (Heirloom's pod roamer, Live's roamer): the new ones'
SUMMON = {"kind": "summon", "below": 100, "count": 3, "npc": ADD, "share": 0, "say": "Patients! To me!"}
NAMED_RESPAWN = 90.0
PHYSICIAN, PHYSICIAN_RESPAWN, PHYSICIAN_WAS = "Last House Physician", 600.0, 90.0
SKIN_CHANCE, CORD_CHANCE = 75, 100

# The ward, where Dust stood (4627.3, -11.9, 3604.5): the lower hall runs east-west there, 7.3 wide (walls at z 3600.48 and
# 3607.78 by rays from that spot), the only way between the east arm (the lower halls) and the west arm's ramp up (a
# flood over the ground from Fatima reaches the Oozing Portal without the box and not with it). The box's middle on the
# hall's middle, 2.5 over the floor (-11.94): 10 a side, so -14.4 to -4.4, the hall's whole width.
# The body: CHAR.ESF's 0x6F412342, a SimpleSprite of a collision box 5 a side and nothing drawn (claude-notes/invisible-
# blockers-20261029.md; entity.BlockerBody), on a copy of the Oozing Portal's row as the Cazic-Thule walls are on a Kithgrin's.
WALL_AT, WALL_SIZE, WALL_NAME, BLOCKER_BODY = (4627.3, -9.44, 3604.13), 2.0, "invisible wall", 0x6F412342
# the far side: the upper halls (every floor above -7 in the house: the lower halls' floors are -12.6 to -11.9) and the
# lower hall's west arm to the wall's middle, the ramp's foot
WALL_INNER = [[4510.0, -7.0, 3595.0, 4680.0, 10.0, 3840.0], [4598.0, -16.0, 3596.0, 4627.3, -7.0, 3624.0]]
WALL_TEXT = "The ward's stench turns you back: you need a Zombie Flesh Suit to go on."

# The wiki's tailoring recipes (skill 2, trade level 10), by the engine's rules for the rest (difficulty 10 a level and 2 a
# rarity, xp 25 a level by rarity + 1: tradeskills-armor-20261029.md)
RECIPES = [("zombie swatch", "Zombie Swatch", [("Stitched Zombie Skin", 3)]),
           ("zombie flesh suit", SUIT_NAME, [("Zombie Swatch", 2), ("Sinew Cord", 1)])]
TAILORING, RECIPE_LEVEL, RECIPE_RARITY = 2, 10, 2


def key(n):
    return (n or "").strip().lower()


def has(db, t):
    return sgg.has(db, t)


def one(db, sql, args=()):
    rows = db.execute(sql, args).fetchall()
    return rows[0] if len(rows) == 1 else None


def item_ids(db, name):
    return [r[0] for r in db.execute("SELECT patternid FROM itempattern WHERE lower(trim(itemname)) = ?", (key(name),))]


def npc_rows(db, name, world=None):
    if world is None:
        return [r[0] for r in db.execute("SELECT id FROM npcs WHERE lower(trim(npc_name)) = ?", (key(name),))]
    return [r[0] for r in db.execute("SELECT id FROM npcs WHERE lower(trim(npc_name)) = ? AND world = ?", (key(name), world))]


def load_portal():
    with open(PORTAL) as f:
        d = json.load(f)
    keys = set(QUESTS) | set(NEW_QUESTS)
    return {"city": d["city"], "quests": [q for q in d["quests"] if q["key"] in keys],
            "items": [it for it in d["items"] if it["name"] in NEW_ITEMS]}


def check(db):
    """Refusals: what is not as this tool expects (Heirloom's and Live's of 2026-11-13)."""
    bad = []
    logged = {k: (q, dg) for k, q, dg in db.execute("SELECT key, questid, digest FROM quest_import_quests")} if has(db, "quest_import_quests") else {}
    for k, (giver, name) in QUESTS.items():
        if k not in logged:
            bad.append(f"quest {k} was never imported")
            continue
        qid, dg = logged[k]
        if one(db, "SELECT giver, name FROM quests WHERE questid = ?", (qid,)) != (giver, name):
            bad.append(f"quest {qid} ({k}) is not {giver}'s {name!r}")
        elif import_quests.digest(db, qid) != dg:
            bad.append(f"quest {qid} ({k}) was changed since its import (a GM's): this tool would not rewrite it")
        mid = db.execute("SELECT count(*) FROM charquests WHERE questid = ? AND done = 0 AND step > 0", (qid,)).fetchone()[0] \
            if has(db, "charquests") else 0
        if mid:
            bad.append(f"{mid} characters are part way through quest {qid} ({k}): its steps' numbers stay, but look first")
    for k in NEW_QUESTS:
        if k in logged:
            bad.append(f"quest {k} is in already")
    for old, new, world in NPC_RENAMES:
        if len(npc_rows(db, old, world)) != 1:
            bad.append(f"{len(npc_rows(db, old, world))} {old!r} in world {world}, want 1")
        if key(old) != key(new) and npc_rows(db, new):
            bad.append(f"an NPC is named {new!r} already")
        if len(npc_rows(db, old)) != len(npc_rows(db, old, world)):
            bad.append(f"{old!r} stands outside world {world} too: its name-keyed rows are not this tool's alone")
        if db.execute("SELECT 1 FROM gmtriggers WHERE steps LIKE ?", (f'%"{old}"%',)).fetchone():
            bad.append(f"a trigger names {old!r}: this tool does not rewrite triggers")
    for n in (SURGEON[0], MASTER[0], ZOMBIE, ADD):
        if npc_rows(db, n):
            bad.append(f"an NPC is named {n!r} already")
        for t, c in NAME_KEYED + [("classtemplates", "name")]:
            if has(db, t) and db.execute(f'SELECT 1 FROM "{t}" WHERE lower(trim("{c}")) = ?', (key(n),)).fetchone():
                bad.append(f"{t} has a row for {n!r} already")
    for n in NEW_ITEMS:
        if item_ids(db, n):
            bad.append(f"an item is named {n!r} already")
    suit = one(db, "SELECT itemname, trade, lore FROM itempattern WHERE patternid = ?", (SUIT,))
    if suit is None or key(suit[0]) != SUIT_NAME or suit[1] != 0:
        bad.append(f"item {SUIT} is {suit}, want the tradeable {SUIT_NAME!r}")
    for like in ("Zombie Patron Skin", "zombie sinew"):
        if len(item_ids(db, like)) != 1:
            bad.append(f"{len(item_ids(db, like))} items named {like!r} (an item's like), want 1")
    if not npc_rows(db, ZOMBIE_BODY_OF, 1):
        bad.append(f"no {ZOMBIE_BODY_OF!r} in world 1 (the zombies' body)")
    if len(npc_rows(db, SURGEON_BODY_OF, WORLD)) != 1:
        bad.append(f"{len(npc_rows(db, SURGEON_BODY_OF, WORLD))} {SURGEON_BODY_OF!r} in world {WORLD}, want 1")
    else:
        r = one(db, "SELECT seconds FROM npcrespawn WHERE npc_row = ?", (npc_rows(db, PHYSICIAN, WORLD)[0],))
        if r is None or r[0] != PHYSICIAN_WAS:
            bad.append(f"the {PHYSICIAN}'s respawn is {r}, want {PHYSICIAN_WAS} s")
    if not npc_rows(db, RAT_OF, WORLD):
        bad.append(f"no {RAT_OF!r} in world {WORLD}")
    if db.execute("SELECT 1 FROM npcaggro WHERE npc_name = ?", (key(RAT_OF),)).fetchone():
        bad.append(f"{RAT_OF} has an npcaggro row: the wiki's rats do not see invisibility, look first")
    if db.execute("SELECT 1 FROM npcaggro WHERE npc_name = 'last house orderly'").fetchone():
        bad.append("last house orderly has an npcaggro row already")
    base = one(db, "SELECT template FROM npctemplate WHERE name = ?", (key(TEMPLATE_OF),))
    if base is None or not one(db, "SELECT 1 FROM classtemplates WHERE name = ?", (base[0],)):
        bad.append(f"no class template for {TEMPLATE_OF!r} (npctemplate) to give the new creatures")
    if not has(db, "questwalls"):
        bad.append("no table questwalls: the server is older than the Cazic-Thule trials' walls")
    elif has(db, "questwalls") and db.execute("SELECT 1 FROM questwalls WHERE source = ?", (SRC,)).fetchone():
        bad.append("a questwalls row of this tool is there already")
    if not has(db, "craftrecipes"):
        bad.append("no table craftrecipes")
    return bad


def copy_npc(db, src_id, changes):
    cols = [r[1] for r in db.execute("PRAGMA table_info(npcs)")]
    row = dict(zip(cols, db.execute("SELECT * FROM npcs WHERE id = ?", (src_id,)).fetchone()))
    row.pop("id")
    row.update(changes)
    names = list(row)
    cur = db.execute(f'INSERT INTO npcs ({",".join(names)}) VALUES ({",".join("?" * len(names))})', [row[c] for c in names])
    db.execute("UPDATE npcs SET npc_id = id WHERE id = ?", (cur.lastrowid,))
    return cur.lastrowid


def rename_npcs(db, say):
    for old, new, world in NPC_RENAMES:
        n = db.execute("UPDATE npcs SET npc_name = ? WHERE lower(trim(npc_name)) = ? AND world = ?", (new, key(old), world)).rowcount
        keyed = 0
        if key(old) != key(new):
            for t, c in NAME_KEYED:
                if has(db, t):
                    keyed += db.execute(f'UPDATE "{t}" SET "{c}" = ? WHERE lower(trim("{c}")) = ?', (key(new), key(old))).rowcount
            if has(db, "campspawns"):
                for cid, kinds in db.execute("SELECT npcid, kinds FROM campspawns").fetchall():
                    parts = [p.strip() for p in kinds.split(",")]
                    if any(key(p) == key(old) for p in parts):
                        db.execute("UPDATE campspawns SET kinds = ? WHERE npcid = ?", (", ".join(new if key(p) == key(old) else p for p in parts), cid))
                        keyed += 1
        say(f"renamed {old!r} -> {new!r} (world {world}): {n} npcs row, {keyed} name-keyed rows")


def import_now(db, say):
    p = import_quests.plan(db, [("portal.json", load_portal())])
    for w in p["warnings"]:
        say("import warning: " + w)
    drafts = [f"{x['key']}: {x['why']}" for x in p["quests"] if x["draft"]]
    if drafts:
        raise SystemExit("refused: a quest would be a draft: " + "; ".join(drafts))
    written, kept, made = import_quests.write(db, p, own=False)
    if kept:
        raise SystemExit(f"refused: the import left {kept} (changed by a GM)")
    say(f"quests: wrote {written}, made {made} items")
    for n in NEW_ITEMS:
        pid = item_ids(db, n)[0]
        db.execute("UPDATE itempattern SET craft = 1, maxstack = 20, trade = 0 WHERE patternid = ?", (pid,))
        say(f"item {pid} {n!r}: tradeable, a craft component, stack 20")


def recipes(db, say):
    for name, product, ings in RECIPES:
        rid = db.execute("SELECT COALESCE(max(id), 0) + 1 FROM craftrecipes").fetchone()[0]
        ing = [{"item": item_ids(db, n)[0], "count": c, "keep": False} for n, c in ings]
        db.execute("""INSERT INTO craftrecipes (id, skill, comp, level, rarity, name, product, yield, ingredients, difficulty, xp,
            discover, scroll, description, source) VALUES (?, ?, 0, ?, ?, ?, ?, 1, ?, ?, ?, 1, 0, ?, ?)""",
                   (rid, TAILORING, RECIPE_LEVEL, RECIPE_RARITY, name, item_ids(db, product)[0], json.dumps(ing),
                    10 * RECIPE_LEVEL + 2 * RECIPE_RARITY, 25 * RECIPE_LEVEL * (1 + RECIPE_RARITY),
                    "The wiki's Plane of Disease: Portal Quest.", SRC))
        say(f"tailoring recipe {rid} {name!r} (trade level {RECIPE_LEVEL}): {', '.join(f'{c} {n}' for n, c in ings)}")


def place(db, say):
    phys = npc_rows(db, SURGEON_BODY_OF, WORLD)[0]
    zbody = db.execute("SELECT modelid FROM npcs WHERE lower(npc_name) = ? AND world = 1 GROUP BY modelid ORDER BY count(*) DESC LIMIT 1",
                       (key(ZOMBIE_BODY_OF),)).fetchone()[0]
    zsrc = npc_rows(db, ZOMBIE_BODY_OF, 1)[0]
    rat = npc_rows(db, RAT_OF, WORLD)[0]
    made = {}

    def at(name, src, level, xyz, size, **more):
        rid = copy_npc(db, src, {"npc_name": name, "npc_level": level, "x_coord": xyz[0], "y_coord": xyz[1], "z_coord": xyz[2],
                                 "world": WORLD, "zone": 0, "size": size, "facing": 0, "npc_type": 0, **more})
        made.setdefault(name, []).append(rid)
        say(f"{name}: row {rid} at ({xyz[0]}, {xyz[1]}, {xyz[2]}), level {level}")
        return rid

    for name, level, xyz, size in (SURGEON, MASTER):
        rid = at(name, phys, level, xyz, size)
        db.execute("INSERT INTO npcrespawn (npc_row, seconds, note, source) VALUES (?, ?, ?, ?)",
                   (rid, NAMED_RESPAWN, "named: the Last House's named keep 90 s (the wiki's second floor)", SRC))
    for xyz in RATS:
        at(RAT_OF, rat, 55, xyz, 1.0)
    for xyz in ZOMBIES:
        at(ZOMBIE, zsrc, ZOMBIE_LEVEL, xyz, 1.0, modelid=zbody)
    at(ADD, zsrc, ADD_LEVEL, SURGEON[2], 1.0, modelid=zbody)
    db.execute("INSERT INTO npcspawnonly (name, note, source) VALUES (?, ?, ?)",
               (key(ADD), "called three at a time by the Last House Surgeon (its template's summon): the wiki's 'aggroing it summons three Zombies'", SRC))
    # the surgeon's template: the house's neighbours' (the scourge rats'), with its summon
    base = db.execute("SELECT template FROM npctemplate WHERE name = ?", (key(TEMPLATE_OF),)).fetchone()[0]
    cols = [r[1] for r in db.execute("PRAGMA table_info(classtemplates)")]
    row = dict(zip(cols, db.execute("SELECT * FROM classtemplates WHERE name = ?", (base,)).fetchone()))
    row.update(name=key(SURGEON[0]), abilities=json.dumps([SUMMON], separators=(",", ":")), source=SRC,
               note=f"The {base} with the wiki's Last House Surgeon's call: three zombies at its first hurt (2026-11-13).")
    db.execute(f'INSERT INTO classtemplates ({",".join(chr(34) + c + chr(34) for c in cols)}) VALUES ({",".join("?" * len(cols))})',
               [row[c] for c in cols])
    for n, tpl in ((SURGEON[0], key(SURGEON[0])), (MASTER[0], base), (ZOMBIE, base), (ADD, base)):
        db.execute("INSERT INTO npctemplate (name, template, note, source) VALUES (?, ?, ?, ?)",
                   (key(n), tpl, "the wiki's Last House second floor (2026-11-13), as the house's creatures", SRC))
    for n in (SURGEON[0], MASTER[0]):
        db.execute("INSERT INTO npcaggro (npc_name, aggressive, radius, note, source) VALUES (?, 1, 0, ?, ?)",
                   (key(n), "attacks on sight: the wiki's second floor; its body is a human's, which would not", SRC))
    db.execute("INSERT INTO npcaggro (npc_name, aggressive, radius, note, source, see_invis) VALUES ('last house orderly', -1, 0, ?, ?, 1)",
               ("the wiki: 'Rats don't see invisibility, but Orderlies do' (no word on aggro: the world's rules)", SRC))
    # loot
    for tname, item, npc, chance in (("pod stitched zombie skin", "Stitched Zombie Skin", ZOMBIE, SKIN_CHANCE),
                                     ("pod sinew cord", "Sinew Cord", MASTER[0], CORD_CHANCE)):
        db.execute("INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel, source) VALUES (?, ?, 1, 1, 100, ?)",
                   (tname, item_ids(db, item)[0], SRC))
        db.execute("INSERT INTO npcloot (npc_name, tablename, chance, rolls, note, source) VALUES (?, ?, ?, 1, ?, ?)",
                   (key(npc), tname, chance, "the wiki's Plane of Disease: Portal Quest", SRC))
        say(f"loot: {npc} drops {item} {chance}% a kill")
    # the physician
    prow = npc_rows(db, PHYSICIAN, WORLD)[0]
    db.execute("UPDATE npcrespawn SET seconds = ?, note = ?, source = ? WHERE npc_row = ?",
               (PHYSICIAN_RESPAWN, "every 10 minutes (Skywalker's ticket 1906, 2026-11-13)", SRC, prow))
    say(f"{PHYSICIAN} (row {prow}): respawn {PHYSICIAN_RESPAWN:.0f} s")
    return made


def ward(db, say):
    # the columns this branch's server makes at its start (internal/database/questwalls.go), made here if missing
    have = {r[1] for r in db.execute("PRAGMA table_info(questwalls)")}
    for col, ddl in (("item", "INTEGER NOT NULL DEFAULT 0"), ("inner", "TEXT NOT NULL DEFAULT ''"), ("text", "TEXT NOT NULL DEFAULT ''")):
        if col not in have:
            db.execute(f"ALTER TABLE questwalls ADD COLUMN {col} {ddl}")
            say(f"questwalls: column {col} added")
    src = npc_rows(db, "Oozing Portal", WORLD)[0]
    rid = copy_npc(db, src, {"npc_name": WALL_NAME, "world": WORLD, "zone": 0, "x_coord": WALL_AT[0], "y_coord": WALL_AT[1],
                             "z_coord": WALL_AT[2], "facing": 0, "size": WALL_SIZE, "npc_level": 1, "npc_type": 128,
                             "modelid": BLOCKER_BODY})
    # passed by whoever finished The Last House (past its last step: the owner's "let flagged players through")
    flag2 = db.execute("SELECT questid FROM quest_import_quests WHERE key = 'Portal/plane-of-disease/flag-2'").fetchone()[0]
    last = db.execute("SELECT max(step) FROM queststeps WHERE questid = ?", (flag2,)).fetchone()[0]
    db.execute("INSERT INTO questwalls (npcid, questid, step, note, source, item, inner, text) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
               (rid, flag2, last, "the Last House's ward where Dust stood (the owner, 2026-11-13): the zombie flesh suit passes it, "
                "and having finished The Last House (\"let flagged players through\")", SRC, SUIT, json.dumps(WALL_INNER), WALL_TEXT))
    say(f"the ward: invisible wall row {rid} at {WALL_AT}, size {WALL_SIZE}; questwalls: item {SUIT} or quest {flag2} finished "
        f"(past step {last}), far side {WALL_INNER}")


def run(db, say):
    rename_npcs(db, say)
    import_now(db, say)
    recipes(db, say)
    place(db, say)
    ward(db, say)


# ---- the log, the audit, the undo

def snapshot(db):
    snap = {}
    for t in TABLES:
        if has(db, t):
            cols = [r[1] for r in db.execute(f'PRAGMA table_info("{t}")')]
            snap[t] = (cols, {r[0]: tuple(r[1:]) for r in db.execute(f'SELECT rowid, * FROM "{t}"')})
    return snap


def changes(before, after):
    """(table, op, rowid, old JSON, new JSON): a row added, changed or gone; a column added changes no row's values."""
    out = []
    for t, (cols, rows) in after.items():
        bcols, old = before.get(t, (cols, {}))
        for rid, row in rows.items():
            new = dict(zip(cols, row))
            if rid not in old:
                out.append((t, "added", rid, "", json.dumps(new)))
                continue
            was = dict(zip(bcols, old[rid]))
            if any(was.get(c) != new[c] for c in bcols):
                out.append((t, "changed", rid, json.dumps(was), json.dumps(new)))
        for rid, row in old.items():
            if rid not in rows:
                out.append((t, "gone", rid, json.dumps(dict(zip(bcols, row))), ""))
    return out


def audit(db, action, tbl, rid, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, action, tbl, json.dumps({"rowid": rid}), before or None, after or None, detail))


def apply(db, commit, say):
    db.execute("BEGIN IMMEDIATE")
    try:
        before = snapshot(db)
        run(db, say)
        ch = changes(before, snapshot(db))
        db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (seq INTEGER PRIMARY KEY AUTOINCREMENT, tbl TEXT NOT NULL, op TEXT NOT NULL, "
                   "rid INTEGER NOT NULL, old TEXT NOT NULL, new TEXT NOT NULL)")
        counts = {}
        for t, op, rid, old, new in ch:
            db.execute(f"INSERT INTO {LOG} (tbl, op, rid, old, new) VALUES (?, ?, ?, ?, ?)", (t, op, rid, old, new))
            counts[f"{t} {op}"] = counts.get(f"{t} {op}", 0) + 1
            if t != "sqlite_sequence":
                audit(db, {"added": "insert", "changed": "update", "gone": "delete"}[op], t, rid, old, new, DETAIL)
        say("rows: " + ", ".join(f"{k} {v}" for k, v in sorted(counts.items())))
        if commit:
            db.execute("COMMIT")
            say(f"applied: {len(ch)} rows, logged in {LOG}")
        else:
            db.execute("ROLLBACK")
            say("(a report: nothing written)")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def row_now(db, t, rid):
    cols = [r[1] for r in db.execute(f'PRAGMA table_info("{t}")')]
    r = db.execute(f'SELECT * FROM "{t}" WHERE rowid = ?', (rid,)).fetchone()
    return None if r is None else dict(zip(cols, r))


def undo(db, say):
    db.execute("BEGIN IMMEDIATE")
    try:
        rows = db.execute(f"SELECT seq, tbl, op, rid, old, new FROM {LOG} ORDER BY seq DESC").fetchall()
        # refuse if anything it wrote has changed since (sqlite_sequence moves with anyone's insert: put back below others')
        moved = []
        added = {(t, rid) for _, t, op, rid, _, _ in rows if op == "added"}
        for _, t, op, rid, old, new in rows:
            if t == "sqlite_sequence":
                continue
            now = row_now(db, t, rid)
            if op in ("added", "changed"):
                want = json.loads(new)
                if now is None or any(now.get(c) != v for c, v in want.items()):
                    moved.append(f"{t} row {rid}")
            elif op == "gone" and now is not None and (t, rid) not in added:
                # a row it took whose rowid another's row has taken since (its own added rows reuse some: an INSERT OR REPLACE)
                moved.append(f"{t} row {rid} (taken since)")
        made_items = [json.loads(new)["patternid"] for _, t, op, _, _, new in rows if t == "itempattern" and op == "added"]
        made_quests = [json.loads(new)["questid"] for _, t, op, _, _, new in rows if t == "quests" and op == "added"]
        held = db.execute(f"SELECT count(*) FROM charinventory WHERE patternid IN ({','.join('?' * len(made_items))})", made_items).fetchone()[0] \
            if made_items and has(db, "charinventory") else 0
        if held:
            moved.append(f"{held} inventory rows hold an item it made")
        took = db.execute(f"SELECT count(*) FROM charquests WHERE questid IN ({','.join('?' * len(made_quests))})", made_quests).fetchone()[0] \
            if made_quests and has(db, "charquests") else 0
        if took:
            moved.append(f"{took} characters took a quest it made")
        if moved:
            raise SystemExit("refused, nothing undone: changed since the apply: " + "; ".join(moved[:20]) + (" ..." if len(moved) > 20 else ""))
        for _, t, op, rid, old, new in rows:
            if op == "added" and t != "sqlite_sequence":
                db.execute(f'DELETE FROM "{t}" WHERE rowid = ?', (rid,))
                audit(db, "delete", t, rid, new, "", "undone: " + DETAIL)
        for _, t, op, rid, old, new in rows:
            if op == "changed" and t != "sqlite_sequence":
                vals = json.loads(old)
                db.execute(f'UPDATE "{t}" SET ' + ",".join(f'"{c}" = ?' for c in vals) + " WHERE rowid = ?", list(vals.values()) + [rid])
                audit(db, "update", t, rid, new, old, "undone: " + DETAIL)
            elif op == "gone":
                vals = json.loads(old)
                db.execute(f'INSERT INTO "{t}" (rowid, ' + ",".join(f'"{c}"' for c in vals) + ") VALUES (?," + ",".join("?" * len(vals)) + ")",
                           [rid] + list(vals.values()))
                audit(db, "insert", t, rid, "", old, "undone: " + DETAIL)
        for _, t, op, rid, old, new in rows:
            if t == "sqlite_sequence" and op == "changed":
                v = json.loads(old)
                if has(db, v["name"]):
                    top = db.execute(f'SELECT max(rowid) FROM "{v["name"]}"').fetchone()[0] or 0
                    db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = ?", (max(v["seq"], top), v["name"]))
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
        say(f"undone: {len(rows)} rows put back (questwalls' item, inner and text columns stay: the server makes them)")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        if a.undo:
            if not has(db, LOG):
                print("nothing to undo")
                return 0
            undo(db, print)
            return 0
        if has(db, LOG):
            print(f"applied already ({LOG} is there): --undo first")
            return 0
        bad = check(db)
        if bad:
            print("refused, nothing done:")
            for b in bad:
                print("  " + b)
            return 1
        apply(db, a.apply, print)
        if a.apply:
            print("restart the server (quests, NPCs, templates, aggro, walls, respawns and recipes are read at its start)")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
