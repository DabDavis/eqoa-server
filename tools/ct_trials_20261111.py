#!/usr/bin/env python3
"""The Cazic-Thule trials on the Isle of Dread, end to end (2026-11-11; the owner's pointer to the forumotion t196
walkthrough, the GM Jibrish's work board entries 430, 616, 627, 637, 640, 656), on the server's code of the same branch:
quest walls (questwalls), aggro on an item held (npcaggro.holding), an item that hides an NPC (itemhide), a survive
step, spawn events with count/minutes/keep, NPC row keys ("#118877"), copies that walk their row's path, and the
boss moments (selfdot, raidheal, powerzero). It makes what that server's start would make if it is not there (tables questwalls and itemhide, column
npcaggro.holding, as internal/database/questwalls.go does), so it runs as the before.sh of the restart that first
brings the code, and --undo drops what it made. An older server cannot read this data: apply it with the restart that
brings the code (NPCs, quests, templates, triggers and walls are read at the start).

What it writes (every row in gm_audit and its log, ct_trials_1111_log; --undo exact):

  The Faceless Generals (101311): its last step pinned to the first Kithgrin (118858). The Guardian KoS only to a
    holder of Cazic's Scent (npcaggro "The Guardian", holding 1231: t196 "will become KoS and attack you if you have
    Cazic's Scent on you"); the scent, used within 25 of the door by one past the heart's step, takes the Guardian
    out of the world 10 minutes (itemhide 1231: "uses Cazic's Scent to remove the Guardian blocking the doorway").
    The second Guardian row (118860) held out of the world (656's "one at the door": its question 1 for jibrish).
  The Trials of Cazic Thule (101312), its steps rewritten in the walkthrough's order:
     1 reach the long hallway (a popup)            2 survive 30 minutes: 6 thoughtbleeders on the questor, each made
     3 talk to the second Kithgrin (new, at the     again as its corpse goes (one lying holds its place), all killed,
       head of the catacombs' ramp)                 the questor alive in the hall (death does not fail it: t196)
     4 talk to the Trial of Celerity (new, the pool room): trigger "The Trial of Celerity": the red copies at 63
       (Contagion, Festering Jack, Zin, three Bloodletters and an Impale: 640) and Qual'Tin, not KoS, for an hour
     5 kill Qual'Tin (a popup)                      6 talk to the third Kithgrin (new, the pool room): trigger "The
     7 kill the Watchman: Kithgrin's talisman         Trial of Contention": Kithgrin gone, 4 s later the Watchman
       (a self port of 500 charges)                   (118868, renamed from "The Watchmen") on his round of the halls
     8 enter the main chamber: trigger "The Enraged Phantasms": 7 come unconnable, 8 s later attack; each killed
       brings 2 (half the time 3: npcdeath), gone in 6-10 minutes      9 a gem off one (only the questor's drop)
    10 reach the statue: trigger "Cazic Thule Awakens" (once a day: the lock held 24 h; CT gone in 45 minutes)
    11 kill CazicThule                                               12 return to the third Kithgrin
  Kithgrin's Talisman (101950, new, repeatable, after 101312): give the talisman to the third Kithgrin, a full one back.
  Walls (questwalls, invisible blockers 0x6F412342, size 2): the catacombs' ramp (past step 2 of 101312), the west
    hall into the main chamber (past step 7, the Watchman), the way into the statue's room (past step 9, the gem).
  Out of the world (npcspawnonly), there for the triggers' and steps' copies: the phantasms, the thoughtbleeders,
    Qual'Tin, the Watchman and the second Watchmen row (616's "one mob", its question 4), the catacomb copies
    (Contagion, Jack, Zin at level 63; the Reaver of Graumaud, which t196's list leaves out: 654's question 2), the
    new Bloodletter and Impale copies, CazicThule (the statue's: a "Statue of Cazic Thule" stands in his place).
  Templates: the Watchman by 616 (melee 785, kick 1,600 every 18 s from 9, proc 2,000 at 10%, no regen; a 3,000
    self dot every 6 s, a full reheal once below 20%, 10,000 on 3 raiders every 30 s, his target's power to 0 at the
    pull and every 60 s); Qual'Tin by 640 (AoE 3,500, a 6,500 death-touch stand-in at 20 s then every 120 s).
  The talisman (701480): "Self, Teleport to Cazic Thule's Estate", 500 charges (itemeffect), its landing a spellgates
    row on the Estate's top floor by the main entrance; level 50, every race.

Rows are found by key and their present values checked; anything not as the 2026-11-11 Heirloom data had it refuses
the whole run. Report only by default; --apply in one transaction; --undo exact (a row changed since is left, said).

    tools/ct_trials_20261111.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "ct_trials_1111_log"
WHO = "ct-trials"
VIA = "tool"
SRC = "cttrials1111"
DETAIL = "the Cazic-Thule trials (t196 walkthrough; board 430/616/640/656), 2026-11-11: tools/ct_trials_20261111.py"

KEYS = {"npcs": ("id",), "npcspawnonly": ("name",), "npctemplate": ("name",), "classtemplates": ("name",),
        "npcaggro": ("npc_name",), "questwalls": ("npcid",), "itemhide": ("patternid",), "itemeffect": ("patternid",),
        "spellgates": ("spell",), "quests": ("questid",), "queststeps": ("questid", "step"), "gmtriggers": ("slot",),
        "npcdeath": ("name", "trigger"), "npcpaths": ("npcid",), "itempattern": ("patternid",)}

W = 0  # Tunaria
SCENT, HEART, GEM, TALISMAN = 1231, 701478, 701479, 701480
KITH1, QUALTIN, WATCHMAN, WATCHMEN2, ZIN_C, REAVER_C, JACK_C, CONTAGION_C = 118858, 118867, 118868, 118880, 118869, 118876, 118875, 118877
CT, GUARDIAN2, BLOODLETTER, IMPALE = 118882, 118860, 118883, 118884
BLOCKER = 0x6F412342
SIGNPOST = 0x6A82352C

# places, measured off the box's ground file (worlds/ground/0.ground: the floors' heights there)
HALL = (25709.0, 37.0, 3207.0)        # the long hallway (the thoughtbleeders' rows)
K2_AT = (25636.0, 29.7, 3200.0)       # the head of the ramp down into the catacombs
POOL = (25690.0, 17.9, 3310.0)        # the catacombs' room with the pool
CHAMBER = (25497.0, 16.0, 3097.0)     # the main chamber with the great lava pool
STATUE = (25421.0, 18.0, 3096.0)      # CazicThule's row
STATUE_REACH = (25435.0, 17.7, 3097.0)
CENTER = (25620.0, 29.0, 3150.0)      # the halls' centre room
LANDING = (25712.0, 54.4, 3232.0)     # the top floor by the main entrance
WALLS = [  # (name in the plan, place, quest step past which it opens)
    ("wall catacombs", (25636.0, 17.9, 3236.0), 2),
    ("wall chamber", (25570.0, 19.9, 3097.0), 7),
    ("wall statue", (25467.0, 17.9, 3097.0), 9),
]

NEW_NPCS = {  # token: (clone of, name, place, facing, overrides)
    "K2": (KITH1, "Kithgrin", K2_AT, 0, {}),
    "K3": (KITH1, "Kithgrin", (25694.0, 17.9, 3310.0), 128, {}),
    "TC": (KITH1, "Trial of Celerity", (25686.0, 17.9, 3310.0), 128, {"modelid": SIGNPOST, "size": 1.0, "npc_level": 1}),
    "ST": (CT, "Statue of Cazic Thule", STATUE, None, {"npc_type": 128}),
    "B1": (BLOODLETTER, "Bloodletter", (25672.0, 17.9, 3312.0), 64, {"npc_level": 63}),
    "B2": (BLOODLETTER, "Bloodletter", (25678.0, 17.9, 3312.0), 64, {"npc_level": 63}),
    "B3": (BLOODLETTER, "Bloodletter", (25684.0, 17.9, 3314.0), 64, {"npc_level": 63}),
    "I1": (IMPALE, "Impale", (25690.0, 17.9, 3316.0), 64, {"npc_level": 63}),
}
for i, (name, at, _) in enumerate(WALLS):
    NEW_NPCS[f"W{i + 1}"] = (KITH1, "invisible wall", at, 0, {"modelid": BLOCKER, "size": 2.0, "npc_level": 1, "npc_type": 128})

WATCHMAN_ABILITIES = [
    {"kind": "selfdot", "damage": 3000, "every": 6, "name": "Massive Rot"},
    {"kind": "heal", "heal_pct": 100, "below": 20, "every": 3600, "name": "Full Reheal"},
    {"kind": "raidheal", "damage": 10000, "count": 3, "every": 30, "first": 30, "name": "Gift of Contention"},
    {"kind": "powerzero", "every": 60, "name": "Mind Drain"},
]
QUALTIN_ABILITIES_NOW = [{"kind": "aoe", "spell_id": 905002, "damage_pct": 167, "every": 30, "first": 10}]
QUALTIN_ABILITIES = [{"kind": "aoe", "spell_id": 905002, "damage_pct": 233, "every": 30, "first": 10}]


def ev(*evs):
    return json.dumps(list(evs), separators=(",", ":"), ensure_ascii=False)


def step(n, kind, journal, **kw):
    r = {"questid": 101312, "step": n, "kind": kind, "target": "", "npcid": 0, "items": "", "tunar": 0, "count": 1, "world": 0,
         "x": 0.0, "y": 0.0, "z": 0.0, "radius": 15.0, "journal": journal, "answer": "", "says": "", "waiting": "", "notyet": "",
         "reminder": "", "xp": 0, "rewardtunar": 0, "rewarditems": "", "factions": "", "spells": "", "events": "", "chance": 0.0,
         "byclass": "", "choices": ""}
    r.update(kw)
    return r


def steps_101312():
    hx, hy, hz = HALL
    return [
        step(1, "reach", "Go down the long hallway alone.", x=hx, y=hy, z=hz, radius=12.0,
             events=ev({"on": "done", "do": "popup", "text": "The air thickens. Thoughtbleeders swarm out of the walls around you."})),
        step(2, "survive", "Survive the trial of endurance: thirty minutes of thoughtbleeders in the long hallway, and kill them all.",
             count=1800, x=hx, y=hy, z=hz, radius=30.0,
             events=ev({"on": "start", "do": "spawn", "npc": "thoughtbleeder", "hostile": True, "count": 6, "keep": True, "minutes": 40},
                       {"on": "done", "do": "popup", "text": "The last thoughtbleeder falls still. You have endured. Go down to the second Kithgrin."})),
        step(3, "talk", "Go down to the second Kithgrin, at the head of the catacombs.", target="Kithgrin", npcid="#K2",
             says="You endured. The trial of celerity waits in the catacombs, in the room with the pool. Bring your friends; "
                  "speak to the Trial when they are ready."),
        step(4, "talk", "Speak to the Trial of Celerity in the catacombs' room with the pool when your raid is ready.",
             target="Trial of Celerity", npcid="#TC", says="The trial of celerity begins. The dead of the isle rise red around you.",
             events=ev({"on": "done", "do": "trigger", "trigger": "The Trial of Celerity",
                        "text": "Others face the trial of celerity now. Come back when they are done."})),
        step(5, "kill", "Kill Qual'Tin in the catacombs.", target="Qual'Tin",
             events=ev({"on": "start", "do": "popup", "text": "Qual'Tin regards your raid with contempt. \"Strike first, if you dare.\""})),
        step(6, "talk", "Speak to the third Kithgrin, in the room with the pool.", target="Kithgrin", npcid="#K3",
             says="If you pass you will have been deemed worthy to face the Faceless.\nYou must defeat the Watchman.",
             events=ev({"on": "done", "do": "trigger", "trigger": "The Trial of Contention",
                        "text": "The Watchman walks for another now. Come back when that trial is done."})),
        step(7, "kill", "Defeat the Watchman as he walks the Estate's halls.", target="The Watchman", rewarditems=str(TALISMAN),
             events=ev({"on": "done", "do": "popup", "text": "You have passed the trial of contention. Kithgrin's talisman is yours: "
                        "use it to come back to the Estate. The main chamber with the great lava pool is open to you."})),
        step(8, "reach", "Enter the main chamber with the great lava pool.", x=CHAMBER[0], y=CHAMBER[1], z=CHAMBER[2], radius=15.0,
             events=ev({"on": "done", "do": "trigger", "trigger": "The Enraged Phantasms",
                        "text": "The phantasms are already roused. Wait until they are gone."})),
        step(9, "loot", "Take a gem from the enraged phantasms.", target="enraged phantasm", items=str(GEM), chance=0.34),
        step(10, "reach", "Carry the gem to the Statue of Cazic Thule.", x=STATUE_REACH[0], y=STATUE_REACH[1], z=STATUE_REACH[2],
             radius=15.0,
             events=ev({"on": "done", "do": "popup", "text": "You set the phantasm's gem into the statue. Its stone face cracks, "
                        "and CazicThule wakes.", "answer": "Awaken him"},
                       {"on": "done", "do": "trigger", "trigger": "Cazic Thule Awakens",
                        "text": "The statue is still. It has not gathered its strength again since it last woke."})),
        step(11, "kill", "Kill CazicThule, the god of fear.", target="CazicThule"),
        step(12, "talk", "Return to the third Kithgrin.", target="Kithgrin", npcid="#K3",
             says="You have passed every trial and faced the Faceless. Keep the talisman; come to me when it runs low."),
    ]


def trig(slot, name, steps, world=W, at=(0.0, 0.0, 0.0), radius=150.0, scope="near", notes=""):
    return {"slot": slot, "name": name, "enabled": 1, "scope": scope, "radius": radius, "world": world,
            "x": at[0], "y": at[1], "z": at[2], "steps": json.dumps(steps, separators=(",", ":"), ensure_ascii=False),
            "notes": notes + " (tools/ct_trials_20261111.py)"}


def place(at, r):
    return {"do": "place", "world": W, "x": at[0], "y": at[1], "z": at[2], "radius": r}


def spawn(npc, at, **kw):
    s = {"do": "spawn", "npc": npc, "world": W, "x": at[0], "y": at[1], "z": at[2], "spread": 0.5}
    s.update(kw)
    return s


def triggers(ids):
    def at(tok):
        return NEW_NPCS[tok][2]
    reds = [("#%d" % CONTAGION_C, (25664.0, 17.9, 3307.0)), ("#%d" % JACK_C, (25688.0, 17.9, 3306.0)),
            ("#%d" % ZIN_C, (25695.0, 17.9, 3307.0))] + [(f"#{ids[t]}", at(t)) for t in ("B1", "B2", "B3", "I1")]
    celerity = [{"do": "lock", "lock": "ct celerity"}, place(POOL, 80), {"do": "clock", "seconds": 3600, "empty": 900}]
    celerity += [spawn(n, p, hostile=True, minutes=60) for n, p in reds]
    celerity += [spawn("#%d" % QUALTIN, (25679.0, 17.9, 3329.0), attackable=True, minutes=60),
                 {"do": "awaitdead", "npc": "Qual'Tin"},
                 {"do": "broadcast", "text": "Qual'Tin falls. The trial of celerity is passed; the third Kithgrin waits by the pool."},
                 {"do": "wait", "seconds": 60}, {"do": "end"}, {"do": "despawn"}]
    contention = [{"do": "lock", "lock": "ct contention"}, place(at("K3"), 20),
                  {"do": "hide", "npc": "Kithgrin", "minutes": 30},
                  place(CENTER, 150), {"do": "clock", "seconds": 1800, "empty": 900},
                  {"do": "wait", "seconds": 4},
                  spawn("#%d" % WATCHMAN, (25600.0, 29.1, 3150.0), hostile=True, minutes=30),
                  {"do": "awaitdead", "npc": "The Watchman"},
                  {"do": "broadcast", "text": "The Watchman falls. The main chamber is open to the one who faced him."},
                  {"do": "end"}, {"do": "despawn"}, place(at("K3"), 20), {"do": "hide", "npc": "Kithgrin", "minutes": 0}]
    phantasms = [{"do": "lock", "lock": "ct phantasms"}, place(CHAMBER, 60), {"do": "clock", "seconds": 600, "empty": 300},
                 spawn("enraged phantasm", CHAMBER, count=7, spread=15.0, minutes=10),
                 {"do": "popup", "text": "Enraged phantasms rise out of the lava pool. Their stare holds you where you stand."},
                 {"do": "wait", "seconds": 8}, {"do": "rouse", "npc": "enraged phantasm"},
                 {"do": "awaitdead"}, {"do": "end"}, {"do": "despawn"}]
    split = [{"do": "spawn", "npc": "enraged phantasm", "hostile": True, "count": 2, "spread": 4.0, "minutes": 6, "max": 14}]
    third = [{"do": "spawn", "npc": "enraged phantasm", "hostile": True, "count": 1, "spread": 4.0, "minutes": 6, "max": 14}]
    statue = [{"do": "lock", "lock": "cazic statue"}, place(STATUE, 80),
              {"do": "hide", "npc": "Statue of Cazic Thule", "minutes": 1440},
              spawn("#%d" % CT, STATUE, hostile=True, minutes=45),
              {"do": "broadcast", "text": "CazicThule wakes in his estate."},
              {"do": "clock", "seconds": 2700}, {"do": "awaitdead", "npc": "CazicThule"},
              {"do": "broadcast", "text": "CazicThule has fallen."}, {"do": "end"}, {"do": "despawn"},
              ] + [{"do": "wait", "seconds": 14400}] * 6  # the lock held 24 h (a wait is 4 h at most)
    return [
        trig(150, "The Trial of Celerity", celerity, at=POOL, radius=80, notes="t196's Trial Two; 640's red copies at 63"),
        trig(151, "The Trial of Contention", contention, at=CENTER, radius=150, notes="t196's Trial Three; 616's Watchman"),
        trig(152, "The Enraged Phantasms", phantasms, at=CHAMBER, radius=60, notes="t196: they root (not modelled), 2-3 replace each, 5-10 min"),
        trig(153, "Phantasm Splits", split, world=-1, radius=60, notes="an enraged phantasm's death: two more"),
        trig(154, "Phantasm Third", third, world=-1, radius=60, notes="half the time a third"),
        trig(155, "Cazic Thule Awakens", statue, at=STATUE, radius=80, notes="the statue once a day; he despawns in 45 min (t196)"),
    ]


def table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def row_of(db, tbl, key):
    where = " AND ".join(f"{c} = ?" for c in KEYS[tbl])
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", tuple(key[c] for c in KEYS[tbl]))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def key_of(tbl, row):
    return {c: row[c] for c in KEYS[tbl]}


def same(text, want):
    try:
        return json.loads(text or "[]") == want
    except ValueError:
        return False


def plan(db):
    """(changes, refusals): changes as (table, before or None, after or None); "#TOKEN" in a value names a new NPC's id."""
    ch, refuse = [], []

    def need(tbl, key, **want):
        r = row_of(db, tbl, key)
        if r is None:
            refuse.append(f"{tbl} {key}: missing")
            return None
        for c, v in want.items():
            if r.get(c) != v:
                refuse.append(f"{tbl} {key}: {c} is {r.get(c)!r}, not {v!r}")
        return r

    def absent(tbl, key):
        if table(db, tbl) and row_of(db, tbl, key) is not None:
            refuse.append(f"{tbl} {key}: there already")

    if not table(db, "npcaggro"):
        refuse.append("no table npcaggro")
        return [], refuse

    # the NPCs: new rows (clones), renames, levels
    for tok, (src, name, at, facing, over) in NEW_NPCS.items():
        s = need("npcs", {"id": src})
        if s is None:
            continue
        r = dict(s)
        del r["id"]
        r.update({"npc_name": name, "x_coord": at[0], "y_coord": at[1], "z_coord": at[2], "world": W, "npc_id": 0})
        if facing is not None:
            r["facing"] = facing
        r.update(over)
        r["_token"] = tok
        ch.append(("npcs", None, r))
    w = need("npcs", {"id": WATCHMAN}, npc_name="The Watchmen")
    if w:
        ch.append(("npcs", w, dict(w, npc_name="The Watchman")))
    for rid, lvl in ((ZIN_C, 62), (JACK_C, 62), (CONTAGION_C, 62)):
        r = need("npcs", {"id": rid}, npc_level=lvl)
        if r:
            ch.append(("npcs", r, dict(r, npc_level=63)))
    for rid, name in ((QUALTIN, "Qual'Tin"), (WATCHMEN2, "The Watchmen"), (REAVER_C, "Reaver of Graumaud"), (CT, "CazicThule"),
                      (GUARDIAN2, "The Guardian"), (KITH1, "Kithgrin")):
        need("npcs", {"id": rid}, npc_name=name)

    # out of the world, there to be copied
    held = ["enraged phantasm", "thoughtbleeder"] + ["#%d" % i for i in (QUALTIN, WATCHMAN, WATCHMEN2, ZIN_C, REAVER_C, JACK_C,
                                                                           CONTAGION_C, CT, GUARDIAN2)] + ["#B1", "#B2", "#B3", "#I1"]
    for n in held:
        if not n.startswith("#") or n[1:].isdigit():
            absent("npcspawnonly", {"name": n})
        ch.append(("npcspawnonly", None, {"name": n, "note": "the Cazic-Thule trials: copied by their triggers and steps", "source": SRC}))

    # templates
    absent("npctemplate", {"name": "the watchman"})
    ch.append(("npctemplate", None, {"name": "the watchman", "template": "iod watchman", "note": "board 616 (renamed row 118868)", "source": SRC}))
    t = need("classtemplates", {"name": "iod watchman"}, melee_pct=1047, kick_damage=1600, kick_every=12.0, kick_first=6.0,
             proc_damage=2000, proc_chance=10, fight_regen=2000)
    if t:
        if not same(t["abilities"], []):
            refuse.append(f"classtemplates 'iod watchman': abilities {t['abilities']}")
        ch.append(("classtemplates", t, dict(t, melee_pct=785, kick_every=18.0, kick_first=9.0, fight_regen=0,
                                             abilities=json.dumps(WATCHMAN_ABILITIES, separators=(",", ":")))))
    t = need("classtemplates", {"name": "iod qual'tin"}, kick_damage=0)
    if t:
        if not same(t["abilities"], QUALTIN_ABILITIES_NOW):
            refuse.append(f"classtemplates \"iod qual'tin\": abilities {t['abilities']}")
        ch.append(("classtemplates", t, dict(t, kick_damage=6500, kick_every=120.0, kick_first=20.0,
                                             abilities=json.dumps(QUALTIN_ABILITIES, separators=(",", ":")))))

    # the Guardian and the scent
    absent("npcaggro", {"npc_name": "The Guardian"})
    ch.append(("npcaggro", None, {"npc_name": "The Guardian", "aggressive": 1, "radius": 0, "holding": SCENT,
                                  "note": "KoS to a holder of Cazic's Scent alone (t196, board 656)", "source": SRC}))
    need("itempattern", {"patternid": SCENT}, itemname="Cazic's Scent")
    absent("itemhide", {"patternid": SCENT})
    ch.append(("itemhide", None, {"patternid": SCENT, "npc": "The Guardian", "radius": 25, "minutes": 10, "questid": 101311, "step": 5,
                                  "text": "Cazic's Scent fills the doorway. The Guardian steps aside.",
                                  "refuse": "The Guardian does not know you yet. Earn its heart first.",
                                  "note": "t196: the scent removes the Guardian blocking the doorway, each time", "source": SRC}))

    # the walls
    for i, (name, at, past) in enumerate(WALLS):
        ch.append(("questwalls", None, {"npcid": f"#W{i + 1}", "questid": 101312, "step": past, "note": name, "source": SRC}))

    # the talisman
    t = need("itempattern", {"patternid": TALISMAN}, itemname="Kithgrin's talisman", levelreq=59)
    if t:
        ch.append(("itempattern", t, dict(t, levelreq=50, raceuse=32767,
                                          itemdesc="Kithgrin's talisman. Use it and you stand on the top floor of Cazic Thule's Estate. "
                                                   "The third Kithgrin fills it again.")))
    absent("itemeffect", {"patternid": TALISMAN})
    ch.append(("itemeffect", None, {"patternid": TALISMAN, "line": "Self, Teleport to Cazic Thule's Estate", "name": "", "icon": 0,
                                    "breaks": "", "wear": 0, "area": 0, "source": SRC, "body": 0, "target_line": "", "charges": 500,
                                    "cooldown": 0}))
    absent("spellgates", {"spell": "kithgrin's talisman"})
    ch.append(("spellgates", None, {"spell": "kithgrin's talisman", "kind": "self", "place": "Cazic Thule's Estate", "world": W,
                                    "x": LANDING[0], "y": LANDING[1], "z": LANDING[2], "radius": 0,
                                    "note": "t196: a self port of 500 charges, landing on the top floor near the main entrance", "source": SRC}))

    # the quests
    s = need("queststeps", {"questid": 101311, "step": 6}, target="Kithgrin", npcid=0)
    if s:
        ch.append(("queststeps", s, dict(s, npcid=KITH1)))
    q = need("quests", {"questid": 101312}, name="The Trials of Cazic Thule")
    if q:
        ch.append(("quests", q, dict(q, notes="From EQOA-Data Quests/Portal/isle-of-dread.json and the forumotion t196 walkthrough: "
                                              "rebuilt 2026-11-11 (tools/ct_trials_20261111.py) in the walkthrough's order, its "
                                              "trials scripted (survive step, triggers 150-155, walls).")))
    old = db.execute("SELECT * FROM queststeps WHERE questid = 101312 ORDER BY step").fetchall()
    cols = [d[1] for d in db.execute("PRAGMA table_info(queststeps)")]
    old = [dict(zip(cols, r)) for r in old]
    if [(r["step"], r["kind"], r["target"]) for r in old] != [
            (1, "reach", ""), (2, "kill", "thoughtbleeder"), (3, "talk", "Kithgrin"), (4, "kill", "Qual'Tin"), (5, "talk", "Kithgrin"),
            (6, "kill", "The Watchmen"), (7, "loot", "enraged phantasm"), (8, "reach", ""), (9, "kill", "CazicThule"), (10, "talk", "Kithgrin")]:
        refuse.append("queststeps 101312 are not the ten the import wrote")
    olds = {r["step"]: r for r in old}
    for new in steps_101312():
        ch.append(("queststeps", olds.get(new["step"]), new))
    active = db.execute("SELECT count(*) FROM charquests WHERE questid = 101312 AND step > 0").fetchone()[0]
    if active:
        refuse.append(f"{active} characters are part way through 101312: their steps would change meaning")
    absent("quests", {"questid": 101950})
    ch.append(("quests", None, {"questid": 101950, "name": "Kithgrin's Talisman", "enabled": 1, "giver": "Kithgrin", "minlevel": 50,
                                "maxlevel": 60, "classes": "", "races": "", "requires": "101312", "repeatable": 1,
                                "offer": "Your talisman grows dim? Give it to me by the pool and I will fill it again.",
                                "accept": "Fill it.", "decline": "Not now.", "declined": "Then use it sparingly.", "started": "",
                                "source": SRC, "notes": "t196: the third Kithgrin refills the self port (tools/ct_trials_20261111.py)",
                                "events": "", "factionreq": ""}))
    ch.append(("queststeps", None, dict(step(1, "give", "Give Kithgrin's talisman to the third Kithgrin, by the pool.", questid=101950,
                                              target="Kithgrin", npcid="#K3", items=str(TALISMAN), rewarditems=str(TALISMAN),
                                              says="There. Five hundred times more."))))

    # triggers, deaths, the Watchman's round
    for t in triggers({t: t for t in NEW_NPCS}):  # "#B1": resolved to the new row's id as it is written
        absent("gmtriggers", {"slot": t["slot"]})
        if db.execute("SELECT count(*) FROM gmtriggers WHERE lower(name) = lower(?)", (t["name"],)).fetchone()[0]:
            refuse.append(f"a trigger is named {t['name']!r} already")
        ch.append(("gmtriggers", None, t))
    for trig_name, chance in (("Phantasm Splits", 100), ("Phantasm Third", 50)):
        absent("npcdeath", {"name": "enraged phantasm", "trigger": trig_name})
        ch.append(("npcdeath", None, {"name": "enraged phantasm", "trigger": trig_name, "chance": chance,
                                      "note": "t196: 'When you kill one 2-3 will take its place'", "source": SRC}))
    absent("npcpaths", {"npcid": WATCHMAN})
    cx, cy, cz = CENTER
    pts = [{"x": 25650.0, "y": 29.0, "z": 3150.0}, {"x": cx, "y": cy, "z": cz},
           {"x": 25603.0, "y": 29.9, "z": 3100.0, "pause": 3}, {"x": cx, "y": cy, "z": cz},
           {"x": 25600.0, "y": 29.1, "z": 3150.0, "pause": 2}, {"x": cx, "y": cy, "z": cz},
           {"x": cx, "y": cy, "z": 3162.0, "pause": 2}]
    ch.append(("npcpaths", None, {"npcid": WATCHMAN, "world": W, "mode": "loop", "pause": 0, "speed": "walk",
                                  "points": json.dumps(pts, separators=(",", ":"))}))
    return ch, refuse


# What this branch's server makes at its start (internal/database/questwalls.go's ensureQuestWalls), made here
# first when it is not there, since the tool runs before that server first starts; --undo drops what it made.
QUESTWALLS_SCHEMA = """CREATE TABLE IF NOT EXISTS questwalls (
		npcid   INTEGER PRIMARY KEY,
		questid INTEGER NOT NULL,
		step    INTEGER NOT NULL DEFAULT 0,
		note    TEXT NOT NULL DEFAULT '',
		source  TEXT NOT NULL DEFAULT 'gm'
	)"""
ITEMHIDE_SCHEMA = """CREATE TABLE IF NOT EXISTS itemhide (
		patternid INTEGER PRIMARY KEY,
		npc       TEXT NOT NULL,
		radius    REAL NOT NULL DEFAULT 20,
		minutes   REAL NOT NULL DEFAULT 10,
		questid   INTEGER NOT NULL DEFAULT 0,
		step      INTEGER NOT NULL DEFAULT 0,
		text      TEXT NOT NULL DEFAULT '',
		refuse    TEXT NOT NULL DEFAULT '',
		note      TEXT NOT NULL DEFAULT '',
		source    TEXT NOT NULL DEFAULT 'gm'
	)"""
HOLDING_ALTER = "ALTER TABLE npcaggro ADD COLUMN holding INTEGER NOT NULL DEFAULT 0"


def has_column(db, tbl, col):
    return any(r[1] == col for r in db.execute(f"PRAGMA table_info({tbl})"))


def ensure_schema(db):
    """What the server's start would make, made now: the names made, for the log (and --undo)."""
    made = []
    for name, ddl in (("questwalls", QUESTWALLS_SCHEMA), ("itemhide", ITEMHIDE_SCHEMA)):
        if not table(db, name):
            db.execute(ddl)
            made.append("table " + name)
    if not has_column(db, "npcaggro", "holding"):
        db.execute(HOLDING_ALTER)
        made.append("column npcaggro.holding")
    return made


LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq INTEGER PRIMARY KEY AUTOINCREMENT, tbl TEXT NOT NULL, rowkey TEXT NOT NULL,
    before TEXT, after TEXT, audit INTEGER NOT NULL DEFAULT 0)"""


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def audit(db, action, tbl, key, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key),
                      None if before is None else go_json(before), None if after is None else go_json(after), detail))
    return cur.lastrowid


def resolve(v, ids):
    """A "#TOKEN" (a new NPC) in a value: the id it took; in a JSON text, each "#TOKEN" in it."""
    if isinstance(v, str):
        if v.startswith("#") and v[1:] in ids:
            return ids[v[1:]]
        for tok, nid in ids.items():
            v = v.replace(f'"#{tok}"', f'"#{nid}"')
        return v
    return v


def apply(db, changes):
    made = ensure_schema(db)
    db.execute(LOG_SCHEMA)
    db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES ('schema', '{{}}', NULL, ?)", (json.dumps(made),))
    seq_before = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'npcs'").fetchone()
    ids = {}
    n = 0
    order = [c for c in changes if c[0] == "npcs" and c[1] is None] + [c for c in changes if not (c[0] == "npcs" and c[1] is None)]
    for tbl, before, after in order:
        after = dict(after)
        tok = after.pop("_token", None)
        after = {c: resolve(v, ids) for c, v in after.items()}
        if tbl == "npcspawnonly" and isinstance(after["name"], int):
            after["name"] = "#%d" % after["name"]
        if before is None:
            cols = list(after)
            cur = db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [after[c] for c in cols])
            if tbl == "npcs":
                db.execute("UPDATE npcs SET npc_id = ? WHERE id = ?", (cur.lastrowid, cur.lastrowid))
                ids[tok] = cur.lastrowid
                key = {"id": cur.lastrowid}
            else:
                key = key_of(tbl, after)
            row = row_of(db, tbl, key)
            a = audit(db, "insert", tbl, key, None, row, DETAIL)
        else:
            key = key_of(tbl, before)
            sets = [c for c in after if after[c] != before.get(c)]
            if not sets:
                continue
            db.execute(f"UPDATE {tbl} SET {', '.join(c + ' = ?' for c in sets)} WHERE " + " AND ".join(f"{c} = ?" for c in KEYS[tbl]),
                       [after[c] for c in sets] + [key[c] for c in KEYS[tbl]])
            row = row_of(db, tbl, key)
            a = audit(db, "update", tbl, key, before, row, DETAIL)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                   (tbl, json.dumps(key), None if before is None else json.dumps(before, sort_keys=True, default=str),
                    json.dumps(row, sort_keys=True, default=str), a))
        n += 1
    db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES ('sqlite_sequence', '{{}}', ?, ?)",
               (json.dumps(None if seq_before is None else seq_before[0]),
                json.dumps(db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'npcs'").fetchone()[0])))
    return n, ids


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    seq_row = db.execute(f"SELECT before, after FROM {LOG} WHERE tbl = 'sqlite_sequence'").fetchone()
    for seq, tbl, key, before, after in db.execute(
            f"SELECT seq, tbl, rowkey, before, after FROM {LOG} WHERE tbl NOT IN ('sqlite_sequence', 'schema') ORDER BY seq DESC").fetchall():
        key = json.loads(key)
        now = row_of(db, tbl, key)
        if now is None or json.dumps(now, sort_keys=True, default=str) != after:
            left += 1
            print(f"  left: {tbl} {key}, changed since it was written")
            continue
        where = " AND ".join(f"{c} = ?" for c in KEYS[tbl])
        if before is None:
            db.execute(f"DELETE FROM {tbl} WHERE {where}", [key[c] for c in KEYS[tbl]])
            audit(db, "delete", tbl, key, now, None, "undo: " + DETAIL)
        else:
            b = json.loads(before)
            db.execute(f"UPDATE {tbl} SET {', '.join(c + ' = ?' for c in b)} WHERE {where}", [b[c] for c in b] + [key[c] for c in KEYS[tbl]])
            audit(db, "update", tbl, key, now, row_of(db, tbl, key), "undo: " + DETAIL)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if left == 0:
        if seq_row is not None:
            was, written = json.loads(seq_row[0]), json.loads(seq_row[1])
            cur = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'npcs'").fetchone()
            if was is not None and cur is not None and cur[0] == written:
                db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = 'npcs'", (was,))
        schema = db.execute(f"SELECT after FROM {LOG} WHERE tbl = 'schema'").fetchone()
        for what in reversed(json.loads(schema[0]) if schema else []):
            if what.startswith("table "):
                name = what[6:]
                if db.execute(f"SELECT count(*) FROM {name}").fetchone()[0]:
                    print(f"  {what} kept: it has rows of another's now")
                else:
                    db.execute(f"DROP TABLE {name}")
            elif what == "column npcaggro.holding":
                if db.execute("SELECT count(*) FROM npcaggro WHERE holding != 0").fetchone()[0]:
                    print("  npcaggro.holding kept: another row uses it now")
                else:
                    db.execute("ALTER TABLE npcaggro DROP COLUMN holding")
        db.execute(f"DROP TABLE {LOG}")
    return back, left


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
            db.execute("BEGIN IMMEDIATE")
            try:
                back, left = undo(db)
                db.execute("COMMIT")
            except BaseException:
                db.execute("ROLLBACK")
                raise
            print(f"put back {back} rows, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        if table(db, LOG):
            print(f"applied already ({LOG} is there): --undo first")
            return 0
        changes, refusals = plan(db)
        by = {}
        for tbl, before, after in changes:
            k = (tbl, "update" if before is not None else "insert")
            by[k] = by.get(k, 0) + 1
        for (tbl, what), c in sorted(by.items()):
            print(f"  {tbl}: {c} {what}")
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not a.apply:
            print(f"report only: {len(changes)} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            n, ids = apply(db, changes)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {n} rows, each in gm_audit and {LOG}; new NPCs: " + ", ".join(f"{t} {i}" for t, i in ids.items()))
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
