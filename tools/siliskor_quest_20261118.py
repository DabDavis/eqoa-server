#!/usr/bin/env python3
"""The Siliskor quest as the owner's source has it: Ronson's walkthrough on the EQOA forum (eqoa.forumotion.com/t177,
"lets add this", the owner 2026-11-18; the same text as EQOA-Data's Misc/siliskor guide the quest was imported from),
with Siliskor's fight as the GM Jibrish decided it (work board 708, "quest-raised, L57, ~250k, sets his tank's HP to
700, a 600 poison DoT AoE, no crit"; his asks are the owner's approved). By NAMES and places, never by ids: the two
shards' rows are read where they stand.

WHAT DIFFERED (quest "Siliskor" by Zaryn Boome, 101266 on both shards):
  - Siliskor stood in the world (two rows: the Brog Fens' by the cultists' cave and the Graveyard's by the crooked
    statue). The walkthrough: he rises when the player reaches the statue. The lead made him spawn-only (npcspawnonly
    "Siliskor", 2026-11-18, the owner's "he should only spawn on the quest"), and step 9 spawned Pomus and two
    worshippers but NOT him: the kill could not be done. Now step 9's start raises him in his pit, at the Graveyard
    row's place (board 708: "at row 111881's place"), with Pomus Magemell and two dragon worshippers at theirs.
  - Every spawn of the quest is a GROUP's (the server's code of branch siliskor: a spawn event's "group"): one set
    for a group, the kill and the scepter's drop shared as any group kill's; another group raises its own, so two
    groups never block each other (the walkthrough's known glitch: "the second group may not get the statue popup").
    A spawn lives 120 minutes (the engine's 10 would have taken a 250k raid boss away mid-fight), the soul 30.
  - Step 2 and 5 ("the dragon altar, down the second ramp") had no height: a player on the hill above (54) did them.
    The altar is where the Mirrored Soul's row stands (17, the cave's floor at the bottom of its second ramp: the
    server's ground files); radius 30, height 10.
  - Step 3 ("rush out of the cave by its main entrance") was a point INSIDE the cave (23, under 34 and 54 of rock).
    The mouth is its tunnel's north end on the hill's face at 14306, 56, 23577 (the ground files: a roof of 79-98
    over the tunnel to 23,610, open ground north of it); radius 25, height 10. Its done shows the dragon "watching
    from afar": a copy of the Graveyard's Siliskor (his 2.5 size), untouchable (the NPC bit: no fight), where the
    Brog Fens row stands, 120 off and in sight of the mouth (the ground's line of sight), for 15 seconds.
  - The Mirrored Soul stood at the altar for anyone, level 49. The walkthrough: "a level 50 mob called Mirrored Soul
    spawns". Spawn-only, level 50, raised by step 6 at its place.
  - Pamus Magemell (FrontiersForge's sighting of the walkthrough's Pomus) stood at the statue for anyone, and the two
    dragon worshippers there stood on Siliskor's 12 hours (petition 2214). Now out of the world (Pamus by name, the
    two by their rows, "#id": the cave's seven worshippers stand as ever), the step's copies made from them.
  - Zaryn Boome stood always. The walkthrough: "he spawns rarely". On a clock (npcrarecycle, branch siliskor): 30
    minutes there, then gone 60 to 180 (ours: the walkthrough gives no times).
  - Siliskor's template (petition 1240's placeholder, then sethp_bosses_20261113's AoE): 150,060 hit points
    (hp_pct 1229 at 55), a 1,000 Critical Hit every 10 s, Siliskor's Blight on everyone within 25 to 700 with a DR
    tick. Board 708: level 57, hp_pct 1914 (~250k), no crit; his tank's hit points to 700 every 30 s ("only targeted
    the person tanking"); poison 600 a tick every 6 s for 30 s round him (radius 40), recast every 30 s, PR resisted
    (forum t281). Here the tank's is Siliskor's Blight (a sethp of radius 0, every 30 s from 6 s) and the poison
    Siliskor's Venom (a sethp that sets no one's hit points, radius 40, players_only so Pomus and the worshippers
    are spared, 600 a tick for 30 s, every 30 s from 12 s).
  - Pomus Magemell: level 55 (from 50), his own template "pomus magemell", the roamer's at hp_pct 328 (~40k).
  - Loot (board 708): the shield of king jorat NO TRADE (it was tradeable; the Plane of Sky's 101299 step 9 takes
    it); his own drops, which no table held: one roll among the dragonhide shield, Lythrial's Robe, Rycoin's
    Scimitar and Siliskor's Horn, every kill (table "boss siliskor drops"). Their Ascended versions are NOT made
    here (board 708's code ask 3 first; tools/ascend_boss_loot.py --boss Siliskor after it).
  Left as they were: the offer (a GM's words), steps 1, 4, 7 and 10 (Corliss Magemell: the cursed one is out of the
  world since petition 2187, the steps sent to the plain one), no reward (petition 2215: the prize is his drop),
  the Sphere of Annihilation's roamers, quest 101028's own Siliskor (not ruled on, board 708).

Report only by default; --apply in one transaction with gm_audit lines and the log siliskor_1118_log (every row
before and after); a second --apply does nothing; --applied says whether it is applied and what it wrote; --undo puts
back exactly what it wrote (a row changed since is left and said) and drops the log. Refused whole if a row is not as
this tool expects. Rows already there that it would write (the lead's npcspawnonly "Siliskor", "Pomus Magemell") are
left and NOT logged, so --undo never takes them.

WHEN IT TAKES EFFECT: with the restart that brings branch siliskor's code (group, placed and untouchable spawns,
players_only, npcrarecycle): an older server reads this quest's events and Siliskor's template as unreadable. The
quests, templates, npcspawnonly, levels and the rare clock at that start; items and loot as they are read.

    python3 -E -s tools/siliskor_quest_20261118.py --db eqoa.db [--apply | --applied | --undo]
"""
import argparse
import datetime
import json
import math
import sqlite3
import sys

LOG = "siliskor_1118_log"
WHO, VIA = "dab", "tool"
SRC = "siliskor1118"
POST = "Ronson's walkthrough (eqoa.forumotion.com/t177, the owner's source 2026-11-18)"
DETAIL = f"the Siliskor quest as {POST} and Siliskor as board 708: tools/siliskor_quest_20261118.py"

QUEST_NAME, GIVER = "Siliskor", "Zaryn Boome"
KINDS = {1: "talk", 2: "reach", 3: "reach", 4: "talk", 5: "reach", 6: "loot", 7: "give", 8: "reach", 9: "kill",
         10: "talk"}
STATUE = (14549.0, 26778.0)  # step 8's reach point, the crooked elephant statue
CAVE_MOUTH = (14306.0, 56.0, 23577.0)  # the ground files: the tunnel's north end on the hill's face
LIFE = 120  # minutes a step 9 spawn lives (the engine's limit)
SOUL_LIFE = 30
WATCH_MINUTES = 0.25  # the dragon watching from afar: 15 s

SIL, POMUS, PAMUS, SOUL, WORSHIPPER = "siliskor", "pomus magemell", "pamus magemell", "mirrored soul", "dragon worshipper"
LEVELS = {SIL: (55, 57), POMUS: (50, 55), SOUL: (49, 50)}

DROPS = ["dragonhide shield", "Lythrial's Robe", "Rycoin's Scimitar", "Siliskor's Horn"]
DROP_TABLE = "boss siliskor drops"
SHIELD = "shield of king jorat"

RARE_UP, RARE_GONE = 30, (60, 180)
RARE_SCHEMA = """CREATE TABLE IF NOT EXISTS npcrarecycle (
		npc_row    INTEGER PRIMARY KEY,
		up_minutes REAL NOT NULL DEFAULT 30,
		gone_min   REAL NOT NULL DEFAULT 60,
		gone_max   REAL NOT NULL DEFAULT 180,
		note       TEXT NOT NULL DEFAULT '',
		source     TEXT NOT NULL DEFAULT 'gm'
	)"""

BLIGHT = {"kind": "sethp", "name": "Siliskor's Blight", "set_hp": 700, "every": 30, "first": 6}
VENOM = {"kind": "sethp", "name": "Siliskor's Venom", "set_hp": 1000000, "radius": 40, "players_only": True,
         "every": 30, "first": 12, "tick": 600, "resist": "PR", "tick_for": 30}
SIL_TEMPLATE = {"hp_pct": 1914, "special_every": 0, "special_chance": 0, "abilities": [BLIGHT, VENOM]}
SIL_WORDS = (" (2026-11-18, tools/siliskor_quest_20261118.py, board 708 as the GM Jibrish decided it: level 57, hp_pct "
             "1914 (~250k), no crit; Siliskor's Blight takes his tank to 700 every 30 s from 6 s; Siliskor's Venom, "
             "poison 600 a tick for 30 s on the players within 40 in his sight, PR resisted, every 30 s from 12 s; "
             "the timers ours)")
POMUS_HP = 328

OLD_BLIGHT = {"kind": "sethp", "name": "Siliskor's Blight", "set_hp": 700, "radius": 25, "every": 20, "first": 6,
              "tick": 150, "tick_at_cap": 16, "resist": "DR", "tick_for": 27}


def has(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone() is not None


def row(db, tbl, where, args):
    cur = db.execute(f'SELECT * FROM "{tbl}" WHERE {where}', args)
    r = cur.fetchone()
    return dict(zip([d[0] for d in cur.description], r)) if r else None


def rows(db, tbl, where, args):
    cur = db.execute(f'SELECT * FROM "{tbl}" WHERE {where}', args)
    names = [d[0] for d in cur.description]
    return [dict(zip(names, r)) for r in cur.fetchall()]


def js(v):
    return json.dumps(v, separators=(",", ":")) if v is not None else None


def named(db, name):
    return rows(db, "npcs", "lower(trim(npc_name)) = ? ORDER BY id", (name,))


def dist(r, x, z):
    return math.hypot(r["x_coord"] - x, r["z_coord"] - z)


def place(r):
    return {"world": r["world"], "x": round(r["x_coord"], 1), "y": round(r["y_coord"], 1), "z": round(r["z_coord"], 1)}


def item_id(db, name):
    r = rows(db, "itempattern", "lower(itemname) = lower(?) ORDER BY patternid", (name,))
    return r[0]["patternid"] if len(r) == 1 else None


class Plan:
    """What this database holds that the tool reads, found by names and places."""

    def __init__(self, db):
        self.bad = []
        q = rows(db, "quests", "name = ? AND giver = ?", (QUEST_NAME, GIVER))
        self.quest = q[0]["questid"] if len(q) == 1 else None
        if self.quest is None:
            self.bad.append(f"{len(q)} quests named {QUEST_NAME!r} by {GIVER!r}, want one")
            return
        self.steps = {r["step"]: r for r in rows(db, "queststeps", "questid = ?", (self.quest,))}
        got = {s: r["kind"] for s, r in self.steps.items()}
        if got != KINDS:
            self.bad.append(f"quest {self.quest}'s steps are {got}, want {KINDS}")
            return
        for s, t in ((6, SOUL), (9, SIL)):
            if self.steps[s]["target"].lower() != t:
                self.bad.append(f"step {s}'s target is {self.steps[s]['target']!r}, want {t!r}")
        sils = named(db, SIL)
        eg = [r for r in sils if dist(r, *STATUE) < 100]
        bf = [r for r in sils if dist(r, CAVE_MOUTH[0], CAVE_MOUTH[2]) < 300]
        if len(sils) != 2 or len(eg) != 1 or len(bf) != 1:
            self.bad.append(f"Siliskor's rows: {[(r['id'], round(r['x_coord']), round(r['z_coord'])) for r in sils]}, "
                            "want one by the statue and one by the cultists' cave")
            return
        self.sil_eg, self.sil_bf = eg[0], bf[0]
        soul = named(db, SOUL)
        pomus = named(db, POMUS)
        if len(soul) != 1 or len(pomus) != 1:
            self.bad.append(f"{len(soul)} mirrored soul rows and {len(pomus)} Pomus Magemell rows, want one each")
            return
        self.soul, self.pomus = soul[0], pomus[0]
        self.pamus = named(db, PAMUS)
        self.worshippers = [r for r in named(db, WORSHIPPER) if dist(r, *STATUE) < 60]
        if len(self.worshippers) != 2:
            self.bad.append(f"{len(self.worshippers)} dragon worshippers by the statue, want two")
            return
        z = named(db, GIVER.lower())
        if len(z) != 1:
            self.bad.append(f"{len(z)} rows named {GIVER}, want one")
            return
        self.zaryn = z[0]
        for r, (old, _new) in ((self.sil_eg, LEVELS[SIL]), (self.sil_bf, LEVELS[SIL]), (self.soul, LEVELS[SOUL]),
                               (self.pomus, LEVELS[POMUS])):
            if r["npc_level"] != old:
                self.bad.append(f"npcs {r['id']} {r['npc_name']!r} is level {r['npc_level']}, want {old}")
        t = row(db, "classtemplates", "name = ?", (SIL,))
        nt = row(db, "npctemplate", "name = ?", (SIL,))
        if t is None or nt is None or nt["template"] != SIL:
            self.bad.append("no classtemplates/npctemplate siliskor")
        else:
            now = {"hp_pct": t["hp_pct"], "special_every": t["special_every"], "abilities": json.loads(t["abilities"])}
            if now != {"hp_pct": 1229, "special_every": 10.0, "abilities": [OLD_BLIGHT]}:
                self.bad.append(f"classtemplates siliskor is {now}, not petition 1240's and the 11-13 Blight")
        if row(db, "classtemplates", "name = ?", (POMUS,)) or row(db, "npctemplate", "name = ?", (POMUS,)):
            self.bad.append("a pomus magemell template is there already")
        if row(db, "classtemplates", "name = ?", ("roamer",)) is None:
            self.bad.append("no classtemplates roamer")
        self.shield = item_id(db, SHIELD)
        self.drops = [item_id(db, n) for n in DROPS]
        if self.shield is None or None in self.drops:
            self.bad.append(f"items: {SHIELD} {self.shield}, drops {dict(zip(DROPS, self.drops))}: want one record each")
        elif row(db, "itempattern", "patternid = ?", (self.shield,))["trade"] != 0:
            self.bad.append(f"{SHIELD} is NO TRADE already")
        if db.execute("SELECT 1 FROM loottables WHERE tablename = ?", (DROP_TABLE,)).fetchone() or \
                db.execute("SELECT 1 FROM npcloot WHERE tablename = ?", (DROP_TABLE,)).fetchone():
            self.bad.append(f"a loot table {DROP_TABLE!r} is there already")
        if has(db, "npcrarecycle") and row(db, "npcrarecycle", "npc_row = ?", (self.zaryn["id"],)):
            self.bad.append(f"npcrarecycle {self.zaryn['id']} is there already")

    # the steps as they will be
    def step_edits(self):
        altar = place(self.soul)
        ax, ay, az = altar["x"], altar["y"], altar["z"]
        mx, my, mz = CAVE_MOUTH
        old3 = json.loads(self.steps[3]["events"] or "[]")
        popup3 = [e for e in old3 if e.get("do") == "popup"]
        old8 = json.loads(self.steps[8]["events"] or "[]")
        popup8 = [dict(e, text="The ground shakes. Siliskor rises from the pit, with Pomus Magemell and two dragon "
                                "worshippers!") for e in old8 if e.get("do") == "popup"]
        watch = place(self.sil_bf)
        sil = place(self.sil_eg)
        pomus_at = place(self.pamus[0]) if self.pamus else dict(sil, x=sil["x"] + 6)

        def spawn(npc, at, minutes, **kw):
            e = {"on": "start", "do": "spawn", "npc": npc}
            e.update(kw)
            e.update({"group": True, "minutes": minutes, "world": at["world"], "x": at["x"], "y": at["y"], "z": at["z"]})
            return e

        ev9 = [spawn("Siliskor", sil, LIFE, hostile=True), spawn("Pomus Magemell", pomus_at, LIFE)]
        ev9 += [spawn(WORSHIPPER, place(w), LIFE) for w in self.worshippers]
        dragon = dict(spawn(f"#{self.sil_eg['id']}", watch, WATCH_MINUTES, untouchable=True), on="done")
        return {
            2: {"x": ax, "y": ay, "z": az, "radius": 30.0, "height": 10.0,
                "journal": "Go into the Dragon Cultists' cave in the Brog Fens, south-west of the Blackwater coach past "
                           "the troll camp: down the second ramp to the dragon altar in its hub room."},
            3: {"x": mx, "y": my, "z": mz, "radius": 25.0, "height": 10.0,
                "journal": "Rush out of the cave by its main entrance, on the hill's north face.",
                "events": js(popup3 + [dragon])},
            5: {"x": ax, "y": ay, "z": az, "radius": 30.0, "height": 10.0,
                "journal": "Go back down to the dragon altar in the Dragon Cultists' cave for the scepter."},
            6: {"events": js([spawn("mirrored soul", altar, SOUL_LIFE, hostile=True)])},
            8: {"height": 15.0, "events": js(popup8),
                "journal": "Approach the crooked elephant statue in the depression in the west of the Elephant "
                           "Graveyard, your raid ready: Siliskor rises when you come."},
            9: {"events": js(ev9),
                "journal": "Kill Siliskor in his pit by the crooked elephant statue. Pomus Magemell and two dragon "
                           "worshippers rise with him: take Pomus down fast, and beware Siliskor's curse, which "
                           "leaves his tank at 700."},
        }


QUEST_NOTE = ("From " + POST + " (= EQOA-Data Quests/Misc/siliskor.json, github.com/Jadiction/EQOA-Data, MIT). The "
              "giver Zaryn Boome comes and goes (npcrarecycle). Step 3's done shows Siliskor watching from afar outside "
              "the cave (untouchable, 15 s); step 6 raises the Mirrored Soul at the altar; step 9 raises Siliskor in his "
              "pit with Pomus Magemell and two dragon worshippers. Every spawn is a group's: one set a group, another "
              "group raises its own (the walkthrough's second-group glitch left out). His prize is his drop (the "
              "shield of king jorat, the Plane of Sky's; board 708's four drops), no quest reward (petition 2215); "
              "Corliss Magemell plays the cursed one (petition 2187). tools/siliskor_quest_20261118.py.")


def report(db):
    q = rows(db, "quests", "name = ? AND giver = ?", (QUEST_NAME, GIVER))
    if not q:
        print("  no Siliskor quest")
        return
    qid = q[0]["questid"]
    print(f"  quest {qid} {QUEST_NAME!r} by {GIVER!r}")
    for r in db.execute("SELECT step, kind, target, round(x), round(y), round(z), radius, height, events FROM queststeps "
                        "WHERE questid = ? ORDER BY step", (qid,)):
        print(f"    step {r[0]} {r[1]} {r[2]!r} at ({r[3]}, {r[4]}, {r[5]}) r{r[6]} h{r[7]} events {r[8]!r}")
    t = row(db, "classtemplates", "name = ?", (SIL,))
    print(f"  Siliskor: levels {[r['npc_level'] for r in named(db, SIL)]}, template hp_pct {t and t['hp_pct']} crit "
          f"every {t and t['special_every']} abilities {t and t['abilities']}")
    so = [r[0] for r in db.execute("SELECT name FROM npcspawnonly WHERE lower(name) IN (?, ?, ?, ?) OR name LIKE '#%' AND "
                                   "source = ?", (SIL, POMUS, PAMUS, SOUL, SRC))]
    print(f"  spawn-only: {so}")
    print(f"  his loot: {[r[0] for r in db.execute('SELECT tablename FROM npcloot WHERE npc_name = ?', (SIL,))]}")


def apply(db, p):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    db.execute(f"CREATE TABLE {LOG} (seq INTEGER PRIMARY KEY, tbl TEXT NOT NULL, rowkey TEXT NOT NULL, before TEXT, after TEXT)")
    n = [0]

    def done(tbl, key, before):
        where = " AND ".join(f"{c} = ?" for c in key)
        after = row(db, tbl, where, tuple(key.values()))
        action = "update" if before and after else "insert" if after else "delete"
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES (?, ?, ?, ?)", (tbl, js(key), js(before), js(after)))
        db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                   "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                   (at, acct[0] if acct else 0, WHO, VIA, action, tbl, js(key), js(before), js(after), DETAIL))
        n[0] += 1

    def insert(tbl, values, key):
        db.execute(f'INSERT INTO "{tbl}" ({", ".join(values)}) VALUES ({", ".join("?" * len(values))})', list(values.values()))
        done(tbl, key, None)

    def update(tbl, key, values):
        where = " AND ".join(f"{c} = ?" for c in key)
        before = row(db, tbl, where, tuple(key.values()))
        db.execute(f'UPDATE "{tbl}" SET {", ".join(f"{c} = ?" for c in values)} WHERE {where}',
                   list(values.values()) + list(key.values()))
        done(tbl, key, before)

    # the quest
    update("quests", {"questid": p.quest}, {"notes": QUEST_NOTE})
    for step, cols in p.step_edits().items():
        update("queststeps", {"questid": p.quest, "step": step}, cols)

    # out of the world, raised by the quest: the lead's Siliskor and the Pomus row there already are left unlogged
    so_note = f"the Siliskor quest {p.quest} raises it ({POST}): {SRC}"
    want = [(SIL, "the owner 2026-11-18: Siliskor only on the quest"), (POMUS, "step 9 raises him"),
            (SOUL, "step 6 raises it at the dragon altar")]
    if p.pamus:
        want.append((PAMUS, "FrontiersForge's Pomus: step 9 raises Pomus Magemell"))
    want += [(f"#{w['id']}", "a dragon worshipper by the statue: step 9 raises two") for w in p.worshippers]
    for name, why in want:
        if row(db, "npcspawnonly", "lower(trim(name)) = lower(?)", (name,)) is None:  # the panel's case kept
            insert("npcspawnonly", {"name": name, "note": f"{why}; {so_note}", "source": SRC}, {"name": name})

    # levels
    for r, lv in ((p.sil_eg, LEVELS[SIL]), (p.sil_bf, LEVELS[SIL]), (p.soul, LEVELS[SOUL]), (p.pomus, LEVELS[POMUS])):
        update("npcs", {"id": r["id"]}, {"npc_level": lv[1]})

    # Siliskor's template, Pomus's own
    t = row(db, "classtemplates", "name = ?", (SIL,))
    update("classtemplates", {"name": SIL}, dict(SIL_TEMPLATE, abilities=js(SIL_TEMPLATE["abilities"]),
                                                 note=t["note"] + SIL_WORDS, source=SRC))
    roamer = row(db, "classtemplates", "name = ?", ("roamer",))
    insert("classtemplates", dict(roamer, name=POMUS, hp_pct=POMUS_HP, source=SRC,
                                  note=f"Pomus Magemell, Siliskor's companion (board 708: level 55, ~40,000 hit points, "
                                       f"'easy to take down'): the roamer's at hp_pct {POMUS_HP}; {SRC}"), {"name": POMUS})
    insert("npctemplate", {"name": POMUS, "template": POMUS, "note": f"board 708: {SRC}", "source": SRC}, {"name": POMUS})

    # loot
    update("itempattern", {"patternid": p.shield}, {"trade": 1})
    for pid in p.drops:
        db.execute("INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel, source) VALUES (?, ?, 1, 1, 70, ?)",
                   (DROP_TABLE, pid, SRC))
        done("loottables", {"id": db.execute("SELECT last_insert_rowid()").fetchone()[0]}, None)
    insert("npcloot", {"npc_name": SIL, "tablename": DROP_TABLE, "chance": 100, "rolls": 1,
                       "note": "board 708 (Allakhazam's live-era page: Dragonhide Shield, Lythrial's Robe, Rycoin's "
                               "Scimitar, Siliskor's Horn): one of the four every kill", "source": SRC},
           {"npc_name": SIL, "tablename": DROP_TABLE})

    # Zaryn Boome comes and goes
    db.execute(RARE_SCHEMA)
    insert("npcrarecycle", {"npc_row": p.zaryn["id"], "up_minutes": RARE_UP, "gone_min": RARE_GONE[0],
                            "gone_max": RARE_GONE[1], "note": f"{POST}: 'he spawns rarely' (the times ours); {SRC}",
                            "source": SRC}, {"npc_row": p.zaryn["id"]})
    return n[0]


def undo(db):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    n = 0
    for tbl, rowkey, before, after in db.execute(f"SELECT tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key = json.loads(rowkey)
        where = " AND ".join(f"{c} = ?" for c in key)
        now = row(db, tbl, where, tuple(key.values()))
        if js(now) != after:
            print(f"  {tbl} {rowkey}: changed since this tool wrote it, left as it is")
            continue
        if before is None:
            db.execute(f'DELETE FROM "{tbl}" WHERE {where}', tuple(key.values()))
        elif after is None:
            b = json.loads(before)
            db.execute(f'INSERT INTO "{tbl}" ({", ".join(b)}) VALUES ({", ".join("?" * len(b))})', list(b.values()))
        else:
            b = json.loads(before)
            db.execute(f'UPDATE "{tbl}" SET {", ".join(f"{c} = ?" for c in b)} WHERE {where}',
                       list(b.values()) + list(key.values()))
        n += 1
    db.execute(f"DROP TABLE {LOG}")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', ?, '{}', NULL, NULL, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, LOG, f"undone, {n} rows put back: " + DETAIL))
    print(f"  {n} rows put back")
    return n


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--applied", action="store_true", help="say whether it is applied, and what it wrote")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        if a.applied:
            if not has(db, LOG):
                print("not applied")
                return 1
            print("applied:")
            for tbl, key, before, after in db.execute(f"SELECT tbl, rowkey, before, after FROM {LOG} ORDER BY seq"):
                print(f"  {tbl} {key}: {'insert' if before is None else 'update' if after else 'delete'}")
            return 0
        report(db)
        if a.undo:
            if not has(db, LOG):
                print("nothing to undo")
                return 0
            db.execute("BEGIN IMMEDIATE")
            undo(db)
            db.execute("COMMIT")
            print("undone; takes effect at a restart")
            return 0
        if has(db, LOG):
            print(f"applied already ({LOG} is there): nothing to do (--undo to take it back)")
            return 0
        p = Plan(db)
        if p.bad:
            print("refused, nothing done:\n  " + "\n  ".join(p.bad))
            return 1
        print(f"  would write: quest {p.quest} steps {sorted(p.step_edits())}; Siliskor rows {p.sil_eg['id']} (the "
              f"statue's, copied) and {p.sil_bf['id']} (the watch's place); soul {p.soul['id']}; Pomus {p.pomus['id']}; "
              f"Pamus {[r['id'] for r in p.pamus]}; worshippers {[r['id'] for r in p.worshippers]}; Zaryn "
              f"{p.zaryn['id']}; shield {p.shield}; drops {p.drops}")
        if not a.apply:
            print("report only: --apply to write it")
            return 0
        db.execute("BEGIN IMMEDIATE")
        n = apply(db, p)
        db.execute("COMMIT")
        print(f"applied, {n} rows; takes effect at the restart that brings branch siliskor's code")
        report(db)
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
