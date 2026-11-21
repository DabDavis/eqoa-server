#!/usr/bin/env python3
"""The werehunters' Mistmoore encounter as MasterVG782's GameFAQs walkthrough tells it (the owner approved the fix,
2026-11-21; ~/.cache/gfaqs-20261121/guide.txt lines 5255-5310; the audit's findings maestro-mistmoore-strength and
mistmoore-spawn-mechanics, ~/.cache/wereaudit-20261121/gaps.json). Run on each shard's database: every row is found
by NAME and place (quests by their import keys, NPCs by name in world 0 by their spots, items by the quests' own
steps), each new id taken on the shard, and nothing written unless each is as the import and the audit found it.

THE GUIDE, and what this makes of it (quests 101383-101386, "Mistmoore", one an archetype; all four alike):

1. "have the person who is doing the quest step on the red carpet in front of the organ to get a popup, which will
   spawn the Maestro of Rancor after around 5 seconds ... Spawning the Maestro of Rancor will remove the songbook
   from your inventory": step 4, the reach at the organ (14481, -60, 18966, radius 25; now within 20 of its height
   too, the drachnid level alone), wants the tome of Vedel's songs (701545) held and takes it (a reach step's items:
   handlers/quests.go reachTakes); its popup, then the new trigger "Maestro of Rancor": five seconds, then one
   hostile copy of his row (npcs 106853, "#106853", held out of the world: npcspawnonly by row, so the Salt Mines
   dungeon's world-14 copy is untouched) where his row stands; one at a time (its lock: another questor's step waits,
   the tome kept, and is told why); 30 minutes at most, ten with no one by him ending it.
2. "His only move that you have to watch out for is an AoE lifetap of around 860 every 30-45 seconds ... cannot be
   soloed": his own template "maestro of rancor" (npctemplate by name, was 'roamer'; the roamer's Critical Hit kept):
   hp_pct 380 (about 40,000 at level 51: Pomus Magemell's and Relk Icemapper's size, a group's named of the 50s),
   melee 150%, and Rancor's Requiem (a new spell made from Valkanith's Bloodtap: a self-centred area of radius 30
   that drains, as Lord Valkanith's) cast as a fixed 860 AR life tap on everyone round him every 37 s (the middle of
   the guide's 30-45), from 10 s in, all that lands healing him.
3. "Once he is dead, have the person doing the quest loot 'vedel's song of awakening'": step 5 is as it was (his
   corpse drops the page to a player on the step); the page (701546, "song page") is named "vedel's song of
   awakening", the guide's.
4. '"mistmoore coffin" must be up in the basement of the mines ... have the person with the song of awakening talk
   to mistmoore's coffin to spawn the ancient one ... Spawning Mistmoore will remove the song of awakening from your
   inventory': the coffin that stands at his spot (npcs 106879 "coffin of Mayong", a talked-to prop, type 0x180) is
   named "mistmoore coffin"; step 6 is a give of the song at it, whose doing fires the new trigger "Mayong Mistmoore":
   the coffin out of view (opened: 75 minutes, past the run), three seconds, one hostile copy of his row (npcs 106904,
   "#106904", spawn-only by row); an hour at most, ten minutes with no one by him ending it; at the run's end (a
   kill, a timeout, an empty cave) the coffin stays down 30 minutes more (ours: the guide's "must be up" with no
   time given), so it is not up again at once. One at a time (its lock).
5. "he has an AoE DoT of around 650 or so, plus a melee AoE of around 2,000 or so. Also, he has a large lifetap of
   around 2.6K ... (the AoE DoT and the lifetap are AR-resisted) ... Gather a party of 8 60s": his own template
   "mayong mistmoore" (was 'roamer'; its Critical Hit kept): hp_pct 1485 (about 175,000 at level 54: between
   Snowgrin's 150k and Kratas's 200k, the raids of a group and a half to two), melee 1300% (about 690 a hit on a
   tank: Snowgrin's 944% lands ~500 at the same level), Mistmoore's Blight (a sethp of no floor, radius 30, players
   only: 650 a tick for 18 s, 80 at the AR cap, every 36 s from 8 s: the dreadnaught's measured tick), Mistmoore's
   Fury (an area rampage, radius 15, every 30 s from 15 s, 300% of a melee hit: about 2,000 on a tank, more on cloth),
   and Mistmoore's Embrace (a fixed 2,600 AR life tap on what he fights, every 30 s from 12 s, Spirit Tap's look).
6. Step 7 is the kill of Mayong Mistmoore (the old step 6's), step 8 the return to Djivan (the old step 7, its
   rewards as they were). A character on the old step 7 goes to 8.
7. "If you fail to kill the Maestro [Mistmoore], then you will have to start the ability quest from the very
   beginning, meaning you'll have to farm another Writ of the Wild": as written. Steps 4 and 6's trigger events carry
   putback 2 (database.QuestEvent.PutBack): a fight that ends without its kill puts the questor back to step 2, the
   Writ's loot, the tome or song not handed back (the steps give them again).
8. One song wakes Mistmoore for the group: at his death every player near on the quest at steps 4-7 is set to step 8
   (questset), so a group's members need not each give a song (the owner, 2026-11-21).

Report only by default; --apply in one transaction, a gm_audit line a row (account 1 'dab', via 'tool') and the log
mistmoore_1121_log (every row before and after); a second --apply does nothing; --undo puts back exactly what it
wrote (a row changed since is left and said) and drops the log. Refused whole if a row is not as this tool expects.
Copies the database first (<db>.bak-<when>-pre-mistmoore) unless --no-backup. --dump prints the triggers, the quest's
steps 4-8, the spawn-only keys and the templates as JSON for the Go test (internal/handlers/mistmoore_test.go).

WHEN IT TAKES EFFECT: quests, charquests, NPC rows (the coffin's name), spawn-only rows, templates and triggers are
read at a start: run it in a keep restart's before.sh, the server stopped (a running server keeps its characters'
quest rows in memory). Items' rows at once. tools/quests/data/weres.json still has the import's steps: a re-import of
that file would put them back (undo this first).

    python3 -E -s tools/mistmoore_20261121.py --db eqoa.db [--apply | --undo | --dump] [--no-backup]
"""
import argparse
import copy
import datetime
import json
import math
import os
import sqlite3
import sys

LOG = "mistmoore_1121_log"
WHO, VIA = "dab", "tool"
SRC = "mistmoore1121"
DETAIL = ("The werehunters' Mistmoore encounter as MasterVG782's GameFAQs walkthrough (the owner, 2026-11-21): "
          "tools/mistmoore_20261121.py")

QUEST_KEYS = ("Weres/werehunter/ability-tank", "Weres/werehunter/ability-melee", "Weres/werehunter/ability-caster",
              "Weres/werehunter/ability-healer")
QUEST_NAME, GIVER = "Mistmoore", "Djivan Mertshak"

MAESTRO, MISTMOORE = "Maestro of Rancor", "Mayong Mistmoore"
MAESTRO_AT, MISTMOORE_AT, REACH = (0, 14481.0, 18966.0), (0, 14542.0, 18980.0), 60.0
COFFIN_FROM, COFFIN = "coffin of Mayong", "mistmoore coffin"
COFFIN_REACH = 15.0
PAGE_FROM, PAGE = "song page", "vedel's song of awakening"
PAGE_DESC = "Vedel's song of awakening, taken from the Maestro of Rancor: sung over the mistmoore coffin, it wakes Mayong Mistmoore."
TOME = "tome of Vedel's songs"

T_MAESTRO, T_MISTMOORE = "Maestro of Rancor", "Mayong Mistmoore"
L_MAESTRO, L_MISTMOORE = "maestro of rancor", "mayong mistmoore"
TPL_MAESTRO, TPL_MISTMOORE = "maestro of rancor", "mayong mistmoore"
TPL_FROM = "roamer"

SPELL_FROM, SPELL_NAME = "Valkanith's Bloodtap", "Rancor's Requiem"
SPELL_RANGE = (760000, 769999)
TAP_RADIUS, MAESTRO_TAP = 30, 860
SPELL_LINE = f"Self, aoe Drain HP by {MAESTRO_TAP} give self, Radius {TAP_RADIUS}"

MAESTRO_HP_PCT, MAESTRO_MELEE = 380, 150
MISTMOORE_HP_PCT, MISTMOORE_MELEE = 1485, 1300
SPIRIT_TAP_FX = 2943531156


def maestro_abilities(spell_id):
    return [{"kind": "lifetap", "spell_id": spell_id, "damage": MAESTRO_TAP, "resist": "AR", "every": 37, "first": 10}]


def mistmoore_abilities():
    return [
        {"kind": "sethp", "name": "Mistmoore's Blight", "set_hp": 1000000, "radius": 30, "players_only": True,
         "tick": 650, "tick_at_cap": 80, "tick_for": 18, "resist": "AR", "every": 36, "first": 8},
        {"kind": "rampage", "every": 30, "first": 15, "damage_pct": 300, "radius": 15, "name": "Mistmoore's Fury"},
        {"kind": "lifetap", "name": "Mistmoore's Embrace", "fx": SPIRIT_TAP_FX, "damage": 2600, "resist": "AR",
         "every": 30, "first": 12},
    ]


MAESTRO_NOTE = ("MasterVG782's GameFAQs walkthrough (the owner, 2026-11-21): 'cannot be soloed', 'His only move that you "
                "have to watch out for is an AoE lifetap of around 860 every 30-45 seconds'. hp_pct 380 (~40k at L51, "
                "a group's named: Pomus Magemell's, Relk Icemapper's size; ours), melee 150% (ours), Rancor's Requiem "
                "860 AR on everyone within 30 every 37 s, healing him. The roamer's Critical Hit kept. "
                "tools/mistmoore_20261121.py")
MISTMOORE_NOTE = ("MasterVG782's GameFAQs walkthrough (the owner, 2026-11-21): '8 60s to be safe', 'an AoE DoT of around "
                  "650 or so, plus a melee AoE of around 2,000 or so ... a large lifetap of around 2.6K ... (the AoE "
                  "DoT and the lifetap are AR-resisted)'. hp_pct 1485 (~175k at L54, between Snowgrin's 150k and "
                  "Kratas's 200k; ours), melee 1300% (~690 a hit on a tank; ours), Mistmoore's Blight 650 a tick "
                  "(80 at the AR cap) for 18 s round him every 36 s, Mistmoore's Fury an area rampage of 300% (~2,000) "
                  "every 30 s, Mistmoore's Embrace a 2,600 AR tap every 30 s (timers ours). The roamer's Critical Hit "
                  "kept. tools/mistmoore_20261121.py")

MAESTRO_BUSY = "The Maestro of Rancor is awake already: wait until he falls or sleeps again."
MISTMOORE_BUSY = "Mayong Mistmoore is awake already: wait until he falls or sleeps again."


def maestro_steps(row_key, x, y, z):
    return [
        {"do": "lock", "lock": L_MAESTRO},
        {"do": "clock", "seconds": 1800, "empty": 600},
        {"do": "wait", "seconds": 5},
        {"do": "spawn", "npc": row_key, "hostile": True, "spread": 0.5, "minutes": 35, "world": 0, "x": x, "y": y,
         "z": z},
        {"do": "broadcast", "text": "The Maestro of Rancor rises before the organ: \"Who plays my song? Then hear how it ends!\""},
        {"do": "awaitdead"},
        {"do": "end"},
        {"do": "despawn"},
    ]


def mistmoore_steps(row_key, x, y, z, qids):
    return [
        {"do": "lock", "lock": L_MISTMOORE},
        {"do": "hide", "npc": COFFIN, "radius": COFFIN_REACH, "minutes": 75},
        {"do": "clock", "seconds": 3600, "empty": 600},
        {"do": "broadcast", "text": "The lid of the mistmoore coffin grinds aside as the song's last note dies away."},
        {"do": "wait", "seconds": 3},
        {"do": "spawn", "npc": row_key, "hostile": True, "spread": 0.5, "minutes": 65, "world": 0, "x": x, "y": y,
         "z": z},
        {"do": "broadcast", "text": "Mayong Mistmoore: \"Vedel's song, sung by cattle. You will be my new choir.\""},
        {"do": "awaitdead"},
        # the group shares the kill (the owner, 2026-11-21: one song wakes him for everyone): each player near
        # on the quest past the Writ (steps 4-7) is set to its last step, the return to Djivan
        *[{"do": "questset", "quest": q, "step": 8, "from": 4} for q in qids],
        {"do": "broadcast", "text": "Mayong Mistmoore is destroyed. His coffin stands empty."},
        {"do": "end"},
        {"do": "despawn"},
        {"do": "hide", "npc": COFFIN, "radius": COFFIN_REACH, "minutes": 30},
    ]


MAESTRO_TRIG_NOTE = ("Fired by quests 101383-101386 (Mistmoore) step 4, Vedel's songbook played on the carpet before the "
                     "organ: five seconds, then one hostile Maestro of Rancor (his row, spawn-only) for 30 minutes at most; "
                     "ten with no one by him end it (MasterVG782's GameFAQs walkthrough, the owner 2026-11-21). "
                     "tools/mistmoore_20261121.py")
MISTMOORE_TRIG_NOTE = ("Fired by quests 101383-101386 (Mistmoore) step 6, vedel's song of awakening given to the mistmoore "
                       "coffin: the coffin down (out of view) for the run and 30 minutes after it (ours), one hostile "
                       "Mayong Mistmoore (his row, spawn-only) for an hour at most; ten minutes with no one by him end it "
                       "(MasterVG782's GameFAQs walkthrough, the owner 2026-11-21). tools/mistmoore_20261121.py")


def step_changes(tome, page):
    """What steps 4-8 of each quest become: {step: columns} (8 is the old 7 moved, 7 new)."""
    return {
        4: {"items": str(tome), "height": 20.0,
            "journal": "Step onto the red carpet before the organ on the drachnid level of the Salt Mine (the second "
                       "doorway right of the room with the spider mosaic) with Vedel's songbook. Clear the glyphed "
                       "familiars and bring a group: the Maestro cannot be soloed.",
            "reminder": "Only Vedel's own songbook, the tome of Vedel's songs from Enrique Revnold, can call the Maestro of Rancor.",
            "events": jsc([{"on": "done", "do": "trigger", "trigger": T_MAESTRO, "text": MAESTRO_BUSY, "putback": 2},
                           {"on": "done", "do": "popup",
                            "text": "You step onto the red carpet and play Vedel's song on the organ. The songbook crumbles "
                                    "to dust in your hands, and something stirs in the dark..."}])},
        5: {"journal": "Defeat the Maestro of Rancor before the organ and take vedel's song of awakening from him."},
        6: {"kind": "give", "target": COFFIN, "items": str(page), "count": 1,
            "journal": "Bring vedel's song of awakening to the mistmoore coffin in the cavern below the brick hall, near "
                       "the organ, and wake Mayong Mistmoore. Gather eight of level 60, the tank's arcane resist maxed.",
            "answer": "I sing vedel's song of awakening.",
            "says": "The page crumbles as the last note fades. Something stirs inside the coffin...",
            "waiting": "The coffin is sealed fast. Only vedel's song of awakening will wake what sleeps inside.",
            "events": jsc([{"on": "done", "do": "trigger", "trigger": T_MISTMOORE, "text": MISTMOORE_BUSY, "putback": 2}])},
    }


NEW7 = {"kind": "kill", "target": MISTMOORE, "items": "", "count": 1,
        "journal": "Destroy Mayong Mistmoore before his coffin. Be in the group that gets the experience for his kill."}


def jsc(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False)


def js(v):
    return None if v is None else json.dumps(v, separators=(",", ":"), ensure_ascii=False, sort_keys=True)


class Refused(Exception):
    pass


def has(db, t):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone()[0] > 0


def row(db, tbl, key):
    where = " AND ".join(f'"{k}" = ?' for k in key)
    cur = db.execute(f'SELECT * FROM "{tbl}" WHERE {where}', list(key.values()))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def near(db, name, place, reach):
    w, x, z = place
    out = []
    for (rid, rx, rz) in db.execute("SELECT id, x_coord, z_coord FROM npcs WHERE lower(trim(npc_name)) = lower(?) AND "
                                    "world = ? ORDER BY id", (name, w)).fetchall():
        if math.hypot(rx - x, rz - z) <= reach:
            out.append(rid)
    return out


def between(rows, level):
    """balance.go's Level: the row of the level, the last past the end, the first before it, else the line between."""
    rows = sorted(rows)
    if not rows:
        return 100.0
    for i, (lv, v) in enumerate(rows):
        if lv >= level:
            if lv == level or i == 0:
                return v
            plv, pv = rows[i - 1]
            return pv + (v - pv) * (level - plv) / (lv - plv)
    return rows[-1][1]


def max_hp(db, level, hp_pct):
    base = max(level * 60, 10)
    bal = between(list(db.execute("SELECT level, hp_pct FROM balancelevel")) if has(db, "balancelevel") else [], level)
    return round(base * bal / 100 * hp_pct / 100)


def old_shape(tome, page):
    """Steps 1-7 as the import (tools/import_quests.py, weres.json) wrote them: kind, target, items, events."""
    return [(1, "talk", "Enrique Revnold", "", ""), (2, "loot", "unkempt druid", "701544", ""),
            (3, "give", "Enrique Revnold", "701544", jsc([{"on": "done", "do": "give", "items": str(tome)}])),
            (4, "reach", "", "", jsc([{"on": "done", "do": "popup",
                                       "text": "You play Vedel's song on the organ. The Maestro of Rancor rises on the carpet!"}])),
            (5, "loot", MAESTRO, str(page), ""), (6, "kill", MISTMOORE, "", ""), (7, "talk", GIVER, "", "")]


def check(db):
    """What the shard holds, refused when it is not as this tool expects: a dict of the ids it works on."""
    for t in ("npcs", "itempattern", "spellpattern", "spelleffect", "gmtriggers", "npcspawnonly", "classtemplates",
              "npctemplate", "quests", "queststeps", "quest_import_quests", "charquests", "gm_audit", "balancelevel"):
        if not has(db, t):
            raise Refused(f"no table {t}")
    ids = {"quests": []}
    for key in QUEST_KEYS:
        r = db.execute("SELECT questid FROM quest_import_quests WHERE key = ?", (key,)).fetchone()
        if not r:
            raise Refused(f"no quest {key!r}")
        q = row(db, "quests", {"questid": r[0]})
        if q is None or q["name"] != QUEST_NAME or q["giver"] != GIVER:
            raise Refused(f"quest {r[0]} ({key}) is {q and (q['name'], q['giver'])}, not {QUEST_NAME} of {GIVER}")
        ids["quests"].append(r[0])
    # the items, by the first quest's own steps (each quest is then held to them)
    q0 = ids["quests"][0]
    s3 = row(db, "queststeps", {"questid": q0, "step": 3})
    s5 = row(db, "queststeps", {"questid": q0, "step": 5})
    try:
        tome = int(json.loads(s3["events"])[0]["items"])
        page = int(s5["items"])
    except (TypeError, ValueError, KeyError, IndexError):
        raise Refused(f"quest {q0}'s steps 3 and 5 name no tome and page")
    for pid, name in ((tome, TOME), (page, PAGE_FROM)):
        r = row(db, "itempattern", {"patternid": pid})
        if r is None or r["itemname"] != name:
            raise Refused(f"item {pid} is {r and r['itemname']!r}, not {name!r}")
    if db.execute("SELECT 1 FROM itempattern WHERE lower(itemname) = lower(?)", (PAGE,)).fetchone():
        raise Refused(f"an item {PAGE!r} exists already")
    ids["tome"], ids["page"] = tome, page
    for qid in ids["quests"]:
        steps = [tuple(s) for s in db.execute("SELECT step, kind, target, items, events FROM queststeps WHERE questid = ? "
                                              "ORDER BY step", (qid,)).fetchall()]
        if steps != old_shape(tome, page):
            raise Refused(f"quest {qid}'s steps are not as the import wrote them: {steps}")
        s4 = row(db, "queststeps", {"questid": qid, "step": 4})
        if (s4["world"], s4["x"], s4["z"]) != (0, 14481.0, 18966.0):
            raise Refused(f"quest {qid} step 4 is at {(s4['world'], s4['x'], s4['z'])}, not the organ")
        mid = db.execute("SELECT serverid, step FROM charquests WHERE questid = ? AND done = 0 AND step = 6",
                         (qid,)).fetchall()
        for sid, _ in mid:
            if not db.execute("SELECT 1 FROM charinventory WHERE serverid = ? AND patternid = ?", (sid, page)).fetchone() \
                    if has(db, "charinventory") else False:
                print(f"  note: character {sid} is on quest {qid} step 6 (now the coffin's) without the page")
    # the bosses and the coffin
    for name, place, key in ((MAESTRO, MAESTRO_AT, "maestro"), (MISTMOORE, MISTMOORE_AT, "mistmoore")):
        r = near(db, name, place, REACH)
        if len(r) != 1:
            raise Refused(f"{len(r)} rows of {name!r} at its place in world 0, not one")
        ids[key] = r[0]
        if row(db, "npcspawnonly", {"name": f"#{r[0]}"}) or \
                db.execute("SELECT 1 FROM npcspawnonly WHERE lower(name) = lower(?)", (name,)).fetchone():
            raise Refused(f"{name} (row {r[0]}) is spawn-only already")
    m = row(db, "npcs", {"id": ids["mistmoore"]})
    c = near(db, COFFIN_FROM, (0, m["x_coord"], m["z_coord"]), COFFIN_REACH)
    if len(c) != 1:
        raise Refused(f"{len(c)} rows of {COFFIN_FROM!r} by Mistmoore's row, not one")
    ids["coffin"] = c[0]
    if row(db, "npcs", {"id": c[0]})["npc_type"] & 0x80 == 0:
        raise Refused("the coffin does not carry the NPC bit (talked to)")
    if db.execute("SELECT 1 FROM npcs WHERE lower(npc_name) = lower(?)", (COFFIN,)).fetchone():
        raise Refused(f"an NPC {COFFIN!r} exists already")
    # templates
    for name in (TPL_MAESTRO, TPL_MISTMOORE):
        u = row(db, "npctemplate", {"name": name})
        if u is None or u["template"] != TPL_FROM:
            raise Refused(f"npctemplate {name!r} is {u and u['template']!r}, not {TPL_FROM!r}")
        if row(db, "classtemplates", {"name": name}):
            raise Refused(f"a template {name!r} exists already")
    if row(db, "classtemplates", {"name": TPL_FROM}) is None:
        raise Refused(f"no template {TPL_FROM!r} to copy")
    # the spell
    if db.execute("SELECT 1 FROM spellpattern WHERE lower(spellname) = lower(?)", (SPELL_NAME,)).fetchone():
        raise Refused(f"a spell {SPELL_NAME!r} exists already")
    g = db.execute("SELECT spellid FROM spellpattern WHERE spellname = ? ORDER BY spellid", (SPELL_FROM,)).fetchall()
    if len(g) != 1 or not row(db, "spelleffect", {"spellid": g[0][0]}):
        raise Refused(f"not one spell {SPELL_FROM!r} with its effects to copy")
    ids["spell_from"] = g[0][0]
    top = max(db.execute("SELECT coalesce(max(spellid), 0) FROM spellpattern WHERE spellid BETWEEN ? AND ?", SPELL_RANGE).fetchone()[0],
              db.execute("SELECT coalesce(max(spellid), 0) FROM spelleffect WHERE spellid BETWEEN ? AND ?", SPELL_RANGE).fetchone()[0],
              SPELL_RANGE[0])
    if top + 1 > SPELL_RANGE[1]:
        raise Refused("no spell id left in the range")
    ids["spell"] = top + 1
    # triggers
    for name in (T_MAESTRO, T_MISTMOORE):
        if db.execute("SELECT 1 FROM gmtriggers WHERE name = ?", (name,)).fetchone():
            raise Refused(f"a trigger {name!r} exists already")
    return ids


def report(db, ids):
    mae, mis = row(db, "npcs", {"id": ids["maestro"]}), row(db, "npcs", {"id": ids["mistmoore"]})
    print(f"  quests {ids['quests']}: step 4 takes the tome ({ids['tome']}) and fires {T_MAESTRO!r}; step 6 the coffin's "
          f"give of the page ({ids['page']}), firing {T_MISTMOORE!r}; 7 the kill; 8 the return")
    on7 = db.execute(f"SELECT questid, serverid FROM charquests WHERE questid IN ({','.join('?' * len(ids['quests']))}) "
                     "AND done = 0 AND step = 7", ids["quests"]).fetchall()
    print(f"  characters on the old step 7, to 8: {on7}")
    print(f"  {MAESTRO} (row {ids['maestro']}, L{mae['npc_level']}): spawn-only by row; template {TPL_MAESTRO!r} "
          f"hp_pct {MAESTRO_HP_PCT} = {max_hp(db, mae['npc_level'], MAESTRO_HP_PCT):,} HP, melee {MAESTRO_MELEE}%, "
          f"{SPELL_NAME} ({ids['spell']}, from {SPELL_FROM} {ids['spell_from']}) {MAESTRO_TAP} AR every 37 s")
    print(f"  {MISTMOORE} (row {ids['mistmoore']}, L{mis['npc_level']}): spawn-only by row; template {TPL_MISTMOORE!r} "
          f"hp_pct {MISTMOORE_HP_PCT} = {max_hp(db, mis['npc_level'], MISTMOORE_HP_PCT):,} HP, melee {MISTMOORE_MELEE}%, "
          f"{js(mistmoore_abilities())}")
    print(f"  {COFFIN_FROM!r} (row {ids['coffin']}) -> {COFFIN!r}; item {ids['page']} {PAGE_FROM!r} -> {PAGE!r}")


def apply(db, ids):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() \
        if has(db, "accountinfo") else None
    db.execute(f"CREATE TABLE {LOG} (seq INTEGER PRIMARY KEY, tbl TEXT NOT NULL, rowkey TEXT NOT NULL, before TEXT, "
               f"after TEXT)")
    made = {}

    def done(tbl, key, before):
        after = row(db, tbl, key)
        action = "insert" if before is None else "delete" if after is None else "update"
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES (?, ?, ?, ?)",
                   (tbl, js(key), js(before), js(after)))
        if tbl == "sqlite_sequence":
            return
        db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                   "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                   (at, acct[0] if acct else 1, WHO, VIA, action, tbl, js(key), js(before), js(after), DETAIL))

    def insert(tbl, r, key):
        db.execute(f'INSERT INTO "{tbl}" ({", ".join(chr(34) + c + chr(34) for c in r)}) VALUES '
                   f'({", ".join("?" * len(r))})', list(r.values()))
        done(tbl, key, None)

    def update(tbl, key, sets):
        before = row(db, tbl, key)
        where = " AND ".join(f'"{k}" = ?' for k in key)
        db.execute(f'UPDATE "{tbl}" SET {", ".join(chr(34) + c + chr(34) + " = ?" for c in sets)} WHERE {where}',
                   list(sets.values()) + list(key.values()))
        done(tbl, key, before)

    def delete(tbl, key):
        before = row(db, tbl, key)
        where = " AND ".join(f'"{k}" = ?' for k in key)
        db.execute(f'DELETE FROM "{tbl}" WHERE {where}', list(key.values()))
        done(tbl, key, before)

    seqs = {t: row(db, "sqlite_sequence", {"name": t}) for t in ("spellpattern",)}

    # the spell: Valkanith's Bloodtap's record, Rancor's Requiem
    sid = ids["spell"]
    src = row(db, "spellpattern", {"spellid": ids["spell_from"]})
    insert("spellpattern", dict(src, spellid=sid, spellname=SPELL_NAME, spelldesc=SPELL_LINE), {"spellid": sid})
    fx = row(db, "spelleffect", {"spellid": ids["spell_from"]})
    eff = json.loads(fx["effects"])
    eff["radius"] = TAP_RADIUS
    for e in eff.get("effects", []):
        if e.get("kind") == "drain":
            e["amount"], e["resist"] = -MAESTRO_TAP, "ar"
    insert("spelleffect", {"spellid": sid, "line": SPELL_LINE, "fxdict": fx["fxdict"],
                           "source": f"The Maestro of Rancor's AoE lifetap, MasterVG782's GameFAQs walkthrough (the owner, "
                                     f"2026-11-21; visual {SPELL_FROM}'s): {SRC}",
                           "effects": jsc(eff)}, {"spellid": sid})
    made[SPELL_NAME] = sid

    # the templates
    base = row(db, "classtemplates", {"name": TPL_FROM})
    for name, hp, melee, ab, note in ((TPL_MAESTRO, MAESTRO_HP_PCT, MAESTRO_MELEE, maestro_abilities(sid), MAESTRO_NOTE),
                                      (TPL_MISTMOORE, MISTMOORE_HP_PCT, MISTMOORE_MELEE, mistmoore_abilities(), MISTMOORE_NOTE)):
        insert("classtemplates", dict(base, name=name, hp_pct=hp, melee_pct=melee, abilities=jsc(ab),
                                      note=f"'{TPL_FROM}' as it was on 2026-11-21. " + note, source=SRC), {"name": name})
        update("npctemplate", {"name": name}, {"template": name, "note": f"own template (was '{TPL_FROM}'): {SRC}",
                                               "source": SRC})

    # the bosses spawn-only by row, the coffin and the page named as the guide names them
    for key, name in (("maestro", MAESTRO), ("mistmoore", MISTMOORE)):
        insert("npcspawnonly", {"name": f"#{ids[key]}", "note": f"{name}: raised by quests 101383-101386 alone "
                                f"(trigger {name!r}), the guide's; tools/mistmoore_20261121.py", "source": SRC},
               {"name": f"#{ids[key]}"})
    update("npcs", {"id": ids["coffin"]}, {"npc_name": COFFIN})
    update("itempattern", {"patternid": ids["page"]}, {"itemname": PAGE, "itemdesc": PAGE_DESC})

    # the triggers
    mae, mis, cof = (row(db, "npcs", {"id": ids[k]}) for k in ("maestro", "mistmoore", "coffin"))
    slot = (db.execute("SELECT max(slot) FROM gmtriggers").fetchone()[0] or 8) + 1
    insert("gmtriggers", {"slot": slot, "name": T_MAESTRO, "enabled": 1, "scope": "near", "radius": 120, "world": 0,
                          "x": mae["x_coord"], "y": mae["y_coord"], "z": mae["z_coord"],
                          "steps": jsc(maestro_steps(f"#{ids['maestro']}", mae["x_coord"], mae["y_coord"], mae["z_coord"])),
                          "notes": MAESTRO_TRIG_NOTE}, {"slot": slot})
    made["trigger " + T_MAESTRO] = slot
    insert("gmtriggers", {"slot": slot + 1, "name": T_MISTMOORE, "enabled": 1, "scope": "near", "radius": 150,
                          "world": 0, "x": cof["x_coord"], "y": cof["y_coord"], "z": cof["z_coord"],
                          "steps": jsc(mistmoore_steps(f"#{ids['mistmoore']}", mis["x_coord"], mis["y_coord"], mis["z_coord"], ids["quests"])),
                          "notes": MISTMOORE_TRIG_NOTE}, {"slot": slot + 1})
    made["trigger " + T_MISTMOORE] = slot + 1

    # the quests
    ch = step_changes(ids["tome"], ids["page"])
    for qid in ids["quests"]:
        on7 = [r[0] for r in db.execute("SELECT serverid FROM charquests WHERE questid = ? AND done = 0 AND step = 7 "
                                        "ORDER BY serverid", (qid,)).fetchall()]
        old7 = row(db, "queststeps", {"questid": qid, "step": 7})
        old6 = row(db, "queststeps", {"questid": qid, "step": 6})
        delete("queststeps", {"questid": qid, "step": 7})
        insert("queststeps", dict(old7, step=8), {"questid": qid, "step": 8})
        insert("queststeps", dict(old6, step=7, **NEW7), {"questid": qid, "step": 7})
        for st, sets in ch.items():
            update("queststeps", {"questid": qid, "step": st}, sets)
        for sid_ in on7:
            update("charquests", {"serverid": sid_, "questid": qid}, {"step": 8, "count": 0})
        q = row(db, "quests", {"questid": qid})
        update("quests", {"questid": qid}, {"notes": q["notes"] + (
            " 2026-11-21, the owner (MasterVG782's GameFAQs walkthrough): Vedel's songbook played on the carpet raises "
            "the Maestro (trigger 'Maestro of Rancor', the book taken); vedel's song of awakening given to the mistmoore "
            "coffin raises Mayong Mistmoore (trigger 'Mayong Mistmoore', the song taken); both spawn-only, own "
            "templates. A fight lost puts the questor back to the step that raised it, its item handed back "
            "(questencounter.go), not to the Writ as the guide has it. tools/mistmoore_20261121.py")})

    for tb, before in seqs.items():
        if before is not None:
            done("sqlite_sequence", {"name": tb}, before)
    return made


def undo(db):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() \
        if has(db, "accountinfo") else None
    n, left = 0, 0
    for tbl, rowkey, before, after in db.execute(f"SELECT tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key = json.loads(rowkey)
        now = row(db, tbl, key)
        if js(now) != after:
            print(f"  {tbl} {rowkey}: changed since this tool wrote it, left as it is")
            left += 1
            continue
        where = " AND ".join(f'"{k}" = ?' for k in key)
        if before is None:
            db.execute(f'DELETE FROM "{tbl}" WHERE {where}', list(key.values()))
        elif after is None:
            b = json.loads(before)
            db.execute(f'INSERT INTO "{tbl}" ({", ".join(chr(34) + c + chr(34) for c in b)}) VALUES '
                       f'({", ".join("?" * len(b))})', list(b.values()))
        else:
            b = json.loads(before)
            db.execute(f'UPDATE "{tbl}" SET {", ".join(chr(34) + c + chr(34) + " = ?" for c in b)} WHERE {where}',
                       list(b.values()) + list(key.values()))
        if tbl != "sqlite_sequence":
            db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                       "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                       (at, acct[0] if acct else 1, WHO, VIA,
                        "delete" if before is None else "insert" if after is None else "update",
                        tbl, rowkey, after, before, "undo: " + DETAIL))
        n += 1
    db.execute(f"DROP TABLE {LOG}")
    print(f"  {n} rows put back, {left} left")


def dump():
    """The triggers, the quest's steps 4-8, the spawn-only keys and the templates, as the Go test reads them, at the
    shards' places (both shards' rows: Maestro 106853 at 14480.7, -60, 18965.5; Mistmoore 106904 at 14541.8, -88,
    18979.9; the coffin 106879 at 14546.3, -88, 18978.0), the tome 701545 and the page 701546, slots 901 and 902, the
    new spell 760381 (each shard's own id at --apply)."""
    mx, my, mz = 14480.7, -60.0, 18965.5
    vx, vy, vz = 14541.8, -88.0, 18979.9
    cx, cy, cz = 14546.3, -88.0, 18978.0
    tome, page, spell = 701545, 701546, 760381
    ch = step_changes(tome, page)
    steps = {
        4: dict({"kind": "reach", "target": "", "world": 0, "x": 14481.0, "y": -60.0, "z": 18966.0, "radius": 25.0}, **ch[4]),
        5: dict({"kind": "loot", "target": MAESTRO, "items": str(page), "chance": 1.0}, **ch[5]),
        6: ch[6],
        7: NEW7,
        8: {"kind": "talk", "target": GIVER, "journal": "Return to Djivan Mertshak in the Oasis."},
    }
    out = {
        "triggers": [
            {"slot": 901, "name": T_MAESTRO, "scope": "near", "radius": 120, "world": 0, "x": mx, "y": my, "z": mz,
             "steps": maestro_steps("#106853", mx, my, mz)},
            {"slot": 902, "name": T_MISTMOORE, "scope": "near", "radius": 150, "world": 0, "x": cx, "y": cy, "z": cz,
             "steps": mistmoore_steps("#106904", vx, vy, vz, [101383, 101384, 101385, 101386])},
        ],
        "rows": {"maestro": {"id": 106853, "name": MAESTRO, "x": mx, "y": my, "z": mz, "level": 51},
                 "mistmoore": {"id": 106904, "name": MISTMOORE, "x": vx, "y": vy, "z": vz, "level": 54},
                 "coffin": {"id": 106879, "name": COFFIN, "x": cx, "y": cy, "z": cz}},
        "spawnonly": ["#106853", "#106904"],
        "items": {"tome": tome, "page": page},
        "steps": {str(k): v for k, v in steps.items()},
        "templates": {TPL_MAESTRO: {"hp_pct": MAESTRO_HP_PCT, "melee_pct": MAESTRO_MELEE, "abilities": maestro_abilities(spell)},
                      TPL_MISTMOORE: {"hp_pct": MISTMOORE_HP_PCT, "melee_pct": MISTMOORE_MELEE,
                                      "abilities": mistmoore_abilities()}},
    }
    print(json.dumps(out, indent=1))


def backup(path):
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-mistmoore"
    src = sqlite3.connect(path)
    out = sqlite3.connect(dst)
    with out:
        src.backup(out)
    src.close()
    out.close()
    print(f"  backup: {dst}")


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    g.add_argument("--dump", action="store_true")
    ap.add_argument("--no-backup", action="store_true")
    a = ap.parse_args(argv)
    if a.dump:
        dump()
        return 0
    if not a.db or not os.path.exists(a.db):
        print(f"no database {a.db}", file=sys.stderr)
        return 2
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 10000")
    logged = has(db, LOG)
    if a.undo:
        if not logged:
            print("nothing to undo: no log")
            return 0
        if not a.no_backup:
            backup(a.db)
        db.execute("BEGIN IMMEDIATE")
        try:
            undo(db)
            db.execute("COMMIT")
        except Exception:
            db.execute("ROLLBACK")
            raise
        return 0
    if logged:
        print(f"applied already (log {LOG}): nothing to do")
        return 0
    try:
        ids = check(db)
    except Refused as e:
        print(f"refused: {e}", file=sys.stderr)
        return 1
    report(db, ids)
    if not a.apply:
        print("report only: --apply to write")
        return 0
    if not a.no_backup:
        backup(a.db)
    db.execute("BEGIN IMMEDIATE")
    try:
        made = apply(db, ids)
        db.execute("COMMIT")
    except Exception:
        db.execute("ROLLBACK")
        raise
    for name, pid in made.items():
        print(f"  {pid:>8}  {name}")
    print("applied; items' rows at once; quests, charquests, NPC rows, spawn-only rows, templates and triggers are read "
          "at the next start (a keep restart's before.sh, the server stopped)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
