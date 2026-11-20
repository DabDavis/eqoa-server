#!/usr/bin/env python3
"""The Clockwork Invasion of Klick`Anon (ticket 2520, Waldorf: "Clockwork Invasion quest isn't in"): Elder Mysidia's
"Klick Clockworks Quest" (Klockizar Sizzick's plot, "Started March 6, 2006"; the Lor Guild's December 2006 newsletter:
"The Klick`Anon Clockwork Invasion: ... a surprise uprising and subsequent quick capture of Klockizar Sizzick"), built
as work board 1028 wrote it for dab's sessions and entry 1055 section 2 cleared it (jibrish, 2026-11-11: "leave it
in": the six quests, the spawn-only named, the five plates, the Stage 2-6 Operations, the Corroded Giant's table).

SOURCE: Cryptic Souls' "Klick Clockworks Quest" by Morynn (external/questmasters/text/id357.txt; [CS] below): six
stages, five from Elder Mysidia and the sixth Recon Recon's, which can be raised "as many times as you like". Names,
the order, who sends you where and what drops are [CS] (its "Corrorded" mended); levels, counts, XP, tunar and the
places' numbers are ours: [CS] gives directions, read here off the ground files and EQOA-Data's map (to be moved in
game by a GM where the windmill or the hut stand otherwise).

  1. Mysidia -> the windmill on the left as you come up the ramp out of the city (s/e of the entrance): reaching it
     brings the Corroded Saboteur and three deadly corrosions; three corrosions killed bring the Corroded Militant,
     who drops Stage 2 Operations; hand it to Mysidia. Reward level 10.
  2. Mysidia -> Waysender Gumwicket ([CS]'s "Bummicket, beside waysender Brindle"): three Corroded Rat Clones, then
     the Corroded Rat Original, Stage 3 Operations. Level 20.
  3. Mysidia -> Recon Wecon outside the east gate -> the top of the mountain to the south: Klockizar Riddik and his
     Mummified Guardian; Stage 4 Operations. Level 30.
  4. Mysidia -> Recon Eacon at the volcano's southern foot (Rogue Clockworks, S) -> a Corroded Drachnid east by the
     water; Stage 5 Operations. Level 40.
  5. Mysidia -> Recon Lecon on the small island east of the clockwork town -> the camp on the shore west of it: three
     corroded clockworks, then the Corroded Warlord; Stage 6 Operations. Level 50.
  6. Recon Recon at the empty campfire south-east of Recon Wecon: talk, and the Corroded Giant comes from the
     north-east; kill him. Repeatable. His table: strongest corroded plate, Strongest Corroded Garb, Strongest
     Corroded Robe, one a kill ([CS]: "He drops illusions, robes and breastplates"). The Ascended copies (entries 580,
     623): tools/ascend_boss_loot.py --boss "Corroded Giant" after this.

Each Mysidia stage's reward [CS]: the plate of its level to warriors, paladins and shadow knights; ours: the garb of the
level to rangers, monks, bards and rogues, the habit to the priests, the robe to the casters (the records there are).
The illusions [CS] are not made (board 1028: no stats). Every named is a quest spawn, out of the world (npcspawnonly),
copied from an anchor row at its stage's place; every spawn is a group's. The four Recons are standing talkers.

Records: five plates (clones of strongest corroded plate 1101, its line [CS]); Stage 2-6 Operations (clones of
tactician's orders 700209). 1101 itself to [CS]'s line (board 1028 section 1: STR 0, STA 40, HPMax 110, AC 466). The
L10-50 corroded garbs' classes 32640 (the priests and casters, a slid mask) -> 114 (RNG MNK BRD ROG, as the L60 garb
960053): none is held.

Report only by default; --apply writes it in one transaction, every row with a gm_audit line (via "tool") and in the
log clockwork_1120_log; --undo puts back exactly what it wrote (a row changed since is left and said) and drops the
log; --applied lists it. Refused whole if a row is not as this tool expects. By NAMES, never ids: each shard's rows and
new ids are its own.

WHEN IT TAKES EFFECT: quests, npcs, npcspawnonly and loot at the next restart; the items at a login.

    python3 -E -s tools/clockwork_invasion_20261120.py --db eqoa.db [--apply | --applied | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "clockwork_1120_log"
WHO, VIA = "dab", "tool"
SRC = "clockwork1120"
CS = "Cryptic Souls' Klick Clockworks Quest (questmasters id357)"
DETAIL = f"the Clockwork Invasion (ticket 2520) as {CS}, board 1028/1055: tools/clockwork_invasion_20261120.py"

GIVER = "Elder Mysidia"
SIZZICK = "Klockizar Sizzick"
GUMWICKET = "Waysender Gumwicket"
W = 0  # Tunaria

# places (ours, from [CS]'s directions: the ground files' floors)
WINDMILL = (23580.0, 54.2, 6580.0)   # up the ramp by the coach, on the left, s/e of the entrance
HUT = (24327.0, 54.8, 6567.0)        # Gumwicket's spot: the rat clones' anchor
WECON = (24420.0, 57.7, 6600.0)      # outside the east gate
TOP = (24725.0, 207.4, 6950.0)       # the mountain's top to the south (the ridge's highest, 207)
EACON = (24900.0, 54.2, 7960.0)      # the volcano's southern foot (its crater at 24880, 7720)
WATER = (25250.0, 43.4, 7850.0)      # east of it by the water
LECON = (26520.0, 34.1, 7150.0)      # the small island east of the clockwork town (Clockworks camp 24919, 7284)
SHORE = (25700.0, 54.2, 7150.0)      # the shore camp west of the island
RECON = (24480.0, 68.6, 6680.0)      # the empty campfire s/e of Recon Wecon
GIANT = (24560.0, 135.7, 6600.0)     # "back behind you (to the n/e)"

FOE, TALKER = 0, 128
# name: (base row's name, level, npc_type, place, size or None, spawn-only)
NEW_NPCS = {
    "Corroded Saboteur": (SIZZICK, 12, TALKER, WINDMILL, None, True),
    "Corroded Militant": ("corroded clockwork", 12, FOE, WINDMILL, None, True),
    "Corroded Rat Clone": ("giant rat", 18, FOE, HUT, None, True),
    "Corroded Rat Original": ("giant rat", 22, FOE, HUT, 1.5, True),
    "Klockizar Riddik": (SIZZICK, 32, FOE, TOP, None, True),
    "Corroded Drachnid": ("#drachnid", 42, FOE, WATER, None, True),
    "Corroded Warlord": ("corroded clockwork", 52, FOE, SHORE, 1.5, True),
    "Corroded Giant": ("corroded clockwork", 60, FOE, GIANT, 3.0, True),
    "Recon Wecon": (SIZZICK, 50, TALKER, WECON, None, False),
    "Recon Eacon": (SIZZICK, 50, TALKER, EACON, None, False),
    "Recon Lecon": (SIZZICK, 50, TALKER, LECON, None, False),
    "Recon Recon": (SIZZICK, 50, TALKER, RECON, None, False),
}
GUARDIAN = ("Mummified Guardian", 30)  # a new row of his, spawn-only by "#<id>" (the 26 others stand as ever)
DRACHNID_BODY = 588351722

# the plates [CS]: (name, level, STA, HPMax, PR, AC); durability and price the same level's garb's (ours)
PLATES = [("weakest corroded plate", 10, 5, 25, 5, 66), ("weaker corroded plate", 20, 15, 40, 10, 144),
          ("weak corroded plate", 30, 25, 50, 15, 222), ("strong corroded plate", 40, 30, 65, 20, 276),
          ("stronger corroded plate", 50, 35, 75, 30, 342)]
TIER = ["weakest", "weaker", "weak", "strong", "stronger"]
STRONGEST = "strongest corroded plate"
P1101_OLD = {"str": 20, "sta": 20, "HPMAX": 100, "AC": 440}
P1101_NEW = {"str": 0, "sta": 40, "HPMAX": 110, "AC": 466}
GARB_MASK_OLD, GARB_MASK_NEW = 32640, 114
ORDERS = "tactician's orders"
OPS = [f"Stage {n} Operations" for n in range(2, 7)]
OPS_DESC = "Klockizar Sizzick's corroded plans: stage {n} of his operations against Klick`Anon."
GIANT_TABLE = "corroded giant"
GIANT_DROPS = [STRONGEST, "Strongest Corroded Garb", "Strongest Corroded Robe"]

XP = [556753, 2814929, 7890859, 14919719, 25000000]  # the imported quests' at those levels; L50 ours
TUNAR = [200, 1000, 3000, 6000, 10000]  # ours
MELEE, PRIEST, CASTER, TANK = (1, 4, 5, 6), (7, 8, 9), (10, 11, 12, 13, 14), (0, 2, 3)


def has(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone() is not None


def rows(db, tbl, where, args=()):
    cur = db.execute(f'SELECT * FROM "{tbl}" WHERE {where}', args)
    names = [d[0] for d in cur.description]
    return [dict(zip(names, r)) for r in cur.fetchall()]


def row(db, tbl, where, args=()):
    r = rows(db, tbl, where, args)
    return r[0] if r else None


def js(v):
    return json.dumps(v, separators=(",", ":")) if v is not None else None


def item(db, name):
    """The records of a name, the 990000-999999 copies (the held copies' range) left out."""
    return rows(db, "itempattern", "lower(trim(itemname)) = lower(?) AND patternid NOT BETWEEN 990000 AND 999999 "
                "ORDER BY patternid", (name,))


def spawn(npc, at=None, hostile=True, count=0, minutes=30, on="done"):
    e = {"on": on, "do": "spawn", "npc": npc}
    if hostile:
        e["hostile"] = True
    if count:
        e["count"] = count
    e.update({"group": True, "minutes": minutes})
    if at:
        e.update({"world": W, "x": at[0], "y": at[1], "z": at[2]})
    return e


def step(kind, target, journal, **kw):
    s = {"kind": kind, "target": target, "journal": journal}
    s.update(kw)
    return s


def quests(ids, guardian):
    """The six quests as they are written: (name, giver, minlevel, repeatable, offer, accept, decline, declined,
    started, steps). ids: the new items' ids by name; guardian: the new guardian row's id."""
    def ops(n):
        return str(ids[OPS[n - 2]])

    def reward(stage):
        lv = stage * 10
        t = TIER[stage - 1]
        by = {}
        for c in TANK:
            by[str(c)] = {"items": str(ids[f"{t} corroded plate"])}
        for c, what in [(c, "garb") for c in MELEE] + [(c, "habit") for c in PRIEST] + [(c, "robe") for c in CASTER]:
            by[str(c)] = {"items": str(ids[f"{t} corroded {what}"])}
        return {"xp": XP[stage - 1], "rewardtunar": TUNAR[stage - 1], "byclass": js(dict(sorted(by.items(), key=lambda kv: int(kv[0])))),
                "_level": lv}

    def hand_in(n, stage, line):
        return step("give", GIVER, f"Bring the Stage {n} Operations to Elder Mysidia in Klick`Anon.", items=ops(n),
                    says=line, **{k: v for k, v in reward(stage).items() if not k.startswith("_")})

    w = {"world": W}
    return [
        ("The Clockwork Invasion: The Windmill", GIVER, 8, 0,
         "Klockizar Sizzick has evil plans for Klick`Anon, and we are readying our defences. Check our windmills for "
         "me: the one on your left as you come up the ramp out of the city. Will you help?",
         "I will help.", "Not now.", "Then Klick`Anon waits on braver gnomes.", "Go up the ramp and look to your left.",
         [step("reach", "the windmill", "Go up the ramp out of Klick`Anon to the windmill on your left, south-east of the "
               "entrance.", x=WINDMILL[0], y=WINDMILL[1], z=WINDMILL[2], radius=25.0, height=15.0,
               events=js([{"on": "done", "do": "popup", "text": "A Corroded Saboteur at the windmill opens a portal "
                           "at its back: deadly corrosions pour out!"},
                          spawn("Corroded Saboteur", WINDMILL, hostile=False, minutes=10),
                          spawn("deadly corrosion", WINDMILL, count=3, minutes=20)]), **w),
          step("kill", "deadly corrosion", "Kill the deadly corrosions at the windmill until their leader shows himself.",
               count=3, events=js([spawn("Corroded Militant", WINDMILL)])),
          step("loot", "Corroded Militant", "Kill the Corroded Militant and take his Stage 2 Operations.",
               items=ops(2), chance=1.0),
          hand_in(2, 1, "Stage two of Sizzick's plan! You have done Klick`Anon a great service. Take this, and come "
                        "back to me: there is more to do.")]),
        ("The Clockwork Invasion: The Rat Clones", GIVER, 18, 0,
         "Sizzick's corrosion spreads. Waysender Gumwicket, by Waysender Brindel, has seen strange rats about his hut. "
         "Speak with him and find what Sizzick is breeding there.",
         "I will go.", "Not now.", "Then the rats will go on breeding.", "Speak with Waysender Gumwicket.",
         [step("talk", GUMWICKET, "Speak with Waysender Gumwicket, outside Klick`Anon by Waysender Brindel.",
               says="Rats! Clockwork rats, all over my hut! There, on the roof, here they come!",
               events=js([spawn("Corroded Rat Clone", HUT, count=3, minutes=20)])),
          step("kill", "Corroded Rat Clone", "Kill the Corroded Rat Clones at Waysender Gumwicket's hut until the "
               "original shows itself.", count=3, events=js([spawn("Corroded Rat Original", HUT)])),
          step("loot", "Corroded Rat Original", "Kill the Corroded Rat Original and take the Stage 3 Operations.",
               items=ops(3), chance=1.0),
          hand_in(3, 2, "The third stage. Sizzick builds his army of copies. Take this for your trouble, and come back.")]),
        ("The Clockwork Invasion: Klockizar Riddik", GIVER, 28, 0,
         "We need intelligence. Report to Recon Wecon, out the east gate of Klick`Anon. He watches the hills for us.",
         "I will report.", "Not now.", "Then we stay blind.", "Find Recon Wecon outside the east gate.",
         [step("talk", "Recon Wecon", "Find Recon Wecon outside the east gate of Klick`Anon.",
               says="Sizzick's lieutenant, Klockizar Riddik, holds the very top of the mountain to the south, with a "
                    "mummified guardian at his side. Bring me his plans."),
          step("reach", "the mountain top", "Climb to the very top of the mountain south of Klick`Anon's east gate.",
               x=TOP[0], y=TOP[1], z=TOP[2], radius=30.0, height=20.0,
               events=js([spawn("Klockizar Riddik", TOP), spawn(f"#{guardian}", TOP)]), **w),
          step("loot", "Klockizar Riddik", "Kill Klockizar Riddik on the mountain top and take the Stage 4 Operations.",
               items=ops(4), chance=1.0),
          hand_in(4, 3, "Excellent work. Riddik will plot no more. Here, and come back to me.")]),
        ("The Clockwork Invasion: The Drachnid", GIVER, 38, 0,
         "Sizzick's corruption has reached the Rogue Clockworks. Recon Eacon waits at the southern foot of the volcano "
         "south across the valley. Speak with him.",
         "I will find him.", "Not now.", "Then the corruption spreads.", "Find Recon Eacon at the volcano's foot.",
         [step("talk", "Recon Eacon", "Find Recon Eacon at the southern foot of the volcano in the Rogue Clockworks.",
               says="A corroded drachnid hunts to the east, near the water. It carries the next stage of Sizzick's plans."),
          step("reach", "the water", "Go east from Recon Eacon to the water's edge.", x=WATER[0], y=WATER[1],
               z=WATER[2], radius=40.0, height=20.0, events=js([spawn("Corroded Drachnid", WATER)]), **w),
          step("loot", "Corroded Drachnid", "Kill the Corroded Drachnid and take the Stage 5 Operations.", items=ops(5),
               chance=1.0),
          hand_in(5, 4, "The fifth stage. We are close to the end of his plan. Take this, and come back.")]),
        ("The Clockwork Invasion: The Warlord", GIVER, 48, 0,
         "Recon Lecon watches from a small island south-east of the clockwork town. Sizzick's warlord gathers his "
         "army on the shore. Find Lecon.",
         "I will find him.", "Not now.", "Then the warlord gathers his army.", "Find Recon Lecon on the island.",
         [step("talk", "Recon Lecon", "Find Recon Lecon on the small island east of the clockwork town.",
               says="Their camp is on the shore due west of here. Kill them until their warlord comes out."),
          step("reach", "the shore camp", "Go to the corroded camp on the shore west of Recon Lecon's island.",
               x=SHORE[0], y=SHORE[1], z=SHORE[2], radius=40.0, height=20.0,
               events=js([spawn("corroded clockwork", SHORE, count=3, minutes=20)]), **w),
          step("kill", "corroded clockwork", "Kill the corroded clockworks at the shore camp until the Corroded "
               "Warlord comes.", count=3, events=js([spawn("Corroded Warlord", SHORE)])),
          step("loot", "Corroded Warlord", "Kill the Corroded Warlord and take the Stage 6 Operations.", items=ops(6),
               chance=1.0),
          hand_in(6, 5, "The last stage. Sizzick's giant is all that is left. Find Recon Recon at the empty campfire "
                        "south-east of Recon Wecon.")]),
        ("The Clockwork Invasion: The Corroded Giant", "Recon Recon", 58, 1,
         "Sizzick's last creation, the Corroded Giant, walks these hills. Tell me when you are ready and I will draw "
         "him out.",
         "I am ready.", "Not now.", "Then rest, and come back.", "Behind you! To the north-east!",
         [step("talk", "Recon Recon", "Speak with Recon Recon at the empty campfire south-east of Recon Wecon.",
               says="Behind you! To the north-east!", events=js([spawn("Corroded Giant", GIANT, minutes=60)])),
          step("kill", "Corroded Giant", "Kill the Corroded Giant, north-east of Recon Recon's campfire.")]),
    ]


class Plan:
    def __init__(self, db):
        self.bad = []
        g = rows(db, "npcs", "npc_name = ?", (GIVER,))
        if len(g) != 1:
            self.bad.append(f"{len(g)} rows of {GIVER}, want one")
        for n in (SIZZICK, GUMWICKET):
            if len(rows(db, "npcs", "npc_name = ?", (n,))) != 1:
                self.bad.append(f"want one row of {n}")
        self.base = {}
        for name, (b, *_rest) in NEW_NPCS.items():
            if rows(db, "npcs", "lower(trim(npc_name)) = lower(?)", (name,)):
                self.bad.append(f"a row named {name} is there already")
            if b in self.base:
                continue
            if b == "#drachnid":
                r = rows(db, "npcs", "modelid = ? ORDER BY id LIMIT 1", (DRACHNID_BODY,))
            else:
                r = rows(db, "npcs", "lower(trim(npc_name)) = lower(?) ORDER BY id LIMIT 1", (b,))
            if not r:
                self.bad.append(f"no row of {b} to copy")
            else:
                self.base[b] = r[0]
        gd = rows(db, "npcs", "npc_name = ? ORDER BY npc_level, id LIMIT 1", (GUARDIAN[0],))
        if not gd:
            self.bad.append("no Mummified Guardian to copy")
        else:
            self.guardian_base = gd[0]
        for q in quests({k: 0 for k in self.item_names()}, 0):
            if rows(db, "quests", "name = ?", (q[0],)):
                self.bad.append(f"a quest {q[0]!r} is there already")
        p = item(db, STRONGEST)
        if len(p) != 1:
            self.bad.append(f"{len(p)} records of {STRONGEST}")
        else:
            self.p1101 = p[0]
            if {k: p[0][k] for k in P1101_OLD} not in (P1101_OLD, P1101_NEW):
                self.bad.append(f"{STRONGEST} is {({k: p[0][k] for k in P1101_OLD})}, not board 1028's before")
        o = item(db, ORDERS)
        if len(o) != 1:
            self.bad.append(f"{len(o)} records of {ORDERS}")
        else:
            self.orders = o[0]
        self.have = {}
        for t in TIER:
            for what in ("garb", "habit", "robe"):
                n = f"{t} corroded {what}"
                r = item(db, n)
                if len(r) != 1:
                    self.bad.append(f"{len(r)} records of {n}")
                else:
                    self.have[n] = r[0]
                    if what == "garb" and r[0]["classuse"] not in (GARB_MASK_OLD, GARB_MASK_NEW):
                        self.bad.append(f"{n}'s classes are {r[0]['classuse']}")
                    if what == "garb" and r[0]["classuse"] == GARB_MASK_OLD and db.execute(
                            "SELECT 1 FROM charinventory WHERE patternid = ?", (r[0]["patternid"],)).fetchone():
                        self.bad.append(f"{n} is held: its classes are not changed here")
        for n in [p[0] for p in PLATES] + OPS:
            if item(db, n):
                self.bad.append(f"a record {n!r} is there already")
        self.drops = {}
        for n in GIANT_DROPS:
            r = item(db, n)
            if len(r) != 1:
                self.bad.append(f"{len(r)} records of {n}")
            else:
                self.drops[n] = r[0]["patternid"]
        if db.execute("SELECT 1 FROM loottables WHERE tablename = ?", (GIANT_TABLE,)).fetchone() or \
                db.execute("SELECT 1 FROM npcloot WHERE tablename = ? OR lower(npc_name) = ?", (GIANT_TABLE, GIANT_TABLE)).fetchone():
            self.bad.append(f"a loot table or npcloot row {GIANT_TABLE!r} is there already")

    @staticmethod
    def item_names():
        return [p[0] for p in PLATES] + OPS + [f"{t} corroded {w}" for t in TIER for w in ("garb", "habit", "robe")]


def report(db):
    for r in db.execute("SELECT questid, name, giver, minlevel, requires, repeatable FROM quests WHERE source = ? "
                        "ORDER BY questid", (SRC,)):
        print(f"  quest {r}")
    for r in db.execute("SELECT id, npc_name, npc_level, npc_type, round(x_coord), round(y_coord), round(z_coord) FROM "
                        "npcs WHERE id IN (SELECT CAST(json_extract(rowkey, '$.id') AS INTEGER) FROM "
                        f"{LOG} WHERE tbl = 'npcs')" if has(db, LOG) else "SELECT 1 WHERE 0"):
        print(f"  npc {r}")


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

    def insert(tbl, values, keycol=None):
        cur = db.execute(f'INSERT INTO "{tbl}" ({", ".join(values)}) VALUES ({", ".join("?" * len(values))})',
                         list(values.values()))
        key = {keycol: values[keycol]} if keycol and keycol in values else {keycol or "id": cur.lastrowid}
        done(tbl, key, None)
        return cur.lastrowid

    def update(tbl, key, values):
        where = " AND ".join(f"{c} = ?" for c in key)
        before = row(db, tbl, where, tuple(key.values()))
        if all(before[c] == v for c, v in values.items()):
            return
        db.execute(f'UPDATE "{tbl}" SET {", ".join(f"{c} = ?" for c in values)} WHERE {where}',
                   list(values.values()) + list(key.values()))
        done(tbl, key, before)

    # records
    ids = {n: r["patternid"] for n, r in p.have.items()}
    update("itempattern", {"patternid": p.p1101["patternid"]}, P1101_NEW)
    for t in TIER:
        update("itempattern", {"patternid": p.have[f"{t} corroded garb"]["patternid"]}, {"classuse": GARB_MASK_NEW})
    for (name, lv, sta, hp, pr, ac), t in zip(PLATES, TIER):
        garb = p.have[f"{t} corroded garb"]
        r = dict(p.p1101)
        r.pop("patternid")
        r.update(P1101_NEW)
        r.update(itemname=name, levelreq=lv, str=0, sta=sta, HPMAX=hp, PR=pr, AC=ac, maxhp=garb["maxhp"],
                 itemcost=garb["itemcost"])
        ids[name] = insert("itempattern", r, "patternid")
    for k, name in enumerate(OPS):
        r = dict(p.orders)
        r.pop("patternid")
        r.update(itemname=name, itemdesc=OPS_DESC.format(n=k + 2))
        ids[name] = insert("itempattern", r, "patternid")

    # the anchors and talkers
    made = {}
    for name, (b, lv, typ, place, size, only) in NEW_NPCS.items():
        r = dict(p.base[b])
        r.pop("id")
        r.update(npc_name=name, npc_level=lv, npc_type=typ, world=W, x_coord=place[0], y_coord=place[1],
                 z_coord=place[2], npc_id=0, dt_kill=None, zone=0)
        if size:
            r["size"] = size
        made[name] = insert("npcs", r)
        if only:
            insert("npcspawnonly", {"name": name, "note": f"the Clockwork Invasion raises it ({CS}): {SRC}",
                                    "source": SRC}, "name")
    r = dict(p.guardian_base)
    r.pop("id")
    r.update(npc_level=GUARDIAN[1], world=W, x_coord=TOP[0] + 6, y_coord=TOP[1], z_coord=TOP[2], npc_id=0,
             dt_kill=None, zone=0, npc_type=FOE)
    guardian = insert("npcs", r)
    insert("npcspawnonly", {"name": f"#{guardian}", "note": f"Klockizar Riddik's Mummified Guardian ({CS}): {SRC}",
                            "source": SRC}, "name")

    # the quests
    prev = 0
    qid = db.execute("SELECT max(questid) FROM quests").fetchone()[0] or 0
    note = (f"From {CS} ('By: Morynn', started 2006-03-06); work board 1028, cleared by 1055 (jibrish 2026-11-11). "
            "Places, levels, counts, XP and tunar ours; the illusions not made. tools/clockwork_invasion_20261120.py.")
    for name, giver, minlv, rep, offer, accept, decline, declined, started, steps in quests(ids, guardian):
        qid += 1
        insert("quests", {"questid": qid, "name": name, "enabled": 1, "giver": giver, "minlevel": minlv, "maxlevel": 60,
                          "requires": str(prev) if prev else "", "repeatable": rep, "offer": offer, "accept": accept,
                          "decline": decline, "declined": declined, "started": started, "source": SRC, "notes": note},
               "questid")
        for i, s in enumerate(steps, 1):
            v = {"questid": qid, "step": i}
            v.update(s)
            cur = db.execute(f'INSERT INTO queststeps ({", ".join(v)}) VALUES ({", ".join("?" * len(v))})', list(v.values()))
            done("queststeps", {"questid": qid, "step": i}, None)
        prev = qid

    # the giant's table
    for pid in p.drops.values():
        db.execute("INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel, source) VALUES (?, ?, 1, 1, 70, ?)",
                   (GIANT_TABLE, pid, SRC))
        done("loottables", {"id": db.execute("SELECT last_insert_rowid()").fetchone()[0]}, None)
    db.execute("INSERT INTO npcloot (npc_name, tablename, chance, rolls, note, source) VALUES (?, ?, 100, 1, ?, ?)",
               (GIANT_TABLE, GIANT_TABLE, f"{CS}: 'He drops illusions, robes and breastplates': one a kill", SRC))
    done("npcloot", {"npc_name": GIANT_TABLE, "tablename": GIANT_TABLE}, None)
    return n[0]


def undo(db):
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
        else:
            b = json.loads(before)
            db.execute(f'UPDATE "{tbl}" SET {", ".join(f"{c} = ?" for c in b)} WHERE {where}',
                       list(b.values()) + list(key.values()))
        n += 1
    db.execute(f"DROP TABLE {LOG}")
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', ?, '{}', NULL, NULL, ?)",
               (datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"), acct[0] if acct else 0,
                WHO, VIA, LOG, f"undone, {n} rows put back: " + DETAIL))
    print(f"  {n} rows put back")


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--applied", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        if a.applied:
            if not has(db, LOG):
                print("not applied")
                return 1
            for tbl, key, before, after in db.execute(f"SELECT tbl, rowkey, before, after FROM {LOG} ORDER BY seq"):
                print(f"  {tbl} {key}: {'insert' if before is None else 'update'}")
            report(db)
            return 0
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
            print(f"applied already ({LOG} is there): --undo to take it back")
            return 0
        p = Plan(db)
        if p.bad:
            print("refused, nothing done:\n  " + "\n  ".join(p.bad))
            return 1
        print(f"  would write: 6 quests by {GIVER} and Recon Recon, {len(NEW_NPCS) + 1} npcs rows "
              f"({sum(1 for v in NEW_NPCS.values() if v[5]) + 1} spawn-only), {len(PLATES)} plates, {len(OPS)} "
              f"Operations, {STRONGEST} to [CS]'s line, the garbs' classes, the giant's table {list(p.drops.values())}")
        if not a.apply:
            print("report only: --apply to write it")
            return 0
        db.execute("BEGIN IMMEDIATE")
        n = apply(db, p)
        db.execute("COMMIT")
        print(f"applied, {n} rows; takes effect at the next restart (items at a login)")
        report(db)
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
