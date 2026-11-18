#!/usr/bin/env python3
"""The Vallon Zek raid as Ronson's post has it (the owner, 2026-11-18: eqoa.forumotion.com/t222-vallon-zek, "make
the Vallon Zek raid work as that post describes"). Run on each shard's database; every row is found by NAME and
place (the shards' ids differ), each new id taken on the shard. It builds on tools/seed_guide_gaps.py's set
"vallon" (Vallon's template, his guards, triggers "Vallon Zek" and "Guards of Vallon", his loot) and on quest
"Vallon Zek" (tools/quests/data/portal.json, imported), and refuses if they are not as those wrote them.

THE POST, step by step, and what this makes of it:

1. "a camp of Zek Scouts walking around a campfire ... little goblins, con white@60, and are white-faced": the six
   Zek scout rows round the campfire east of Siloth Mines (world 0, about 15300, 18900) go from level 58 to 60, and
   never attack on sight (npcaggro 0: the white face; claude-notes/aggro-20261025.md). Their melee is the roamer
   template's at 60, as before ("quadding my alch for 547ish": not measured here).
2. "Farm the Zek Scouts until one drops a Portal Rune, No Rent. I had one drop after I'd killed about 10": a loot
   table "zek portal rune" (the Portal Rune of Zek) on "zek scout" at 10% x 1, for anyone (until now the quest's
   loot step gave it, at 20%, only to a character on the quest). The rune becomes NO RENT (rent 0, and a
   norentvanish row: gone five minutes after its holder logs out, as conjured items).
3. "As soon as a Zek Scout drops a Portal Rune, the portal will spawn by the campfire. Speak with the portal ...
   Touch the portal ... it 'breaks' your Portal Rune of Zek and replaces it with 20 Portal Shards (no rent)":
   - the Portal of Zek's row is held out of the world (npcspawnonly), and a corpse that rolls the rune fires the
     new trigger "Portal of Zek" where it lies (table lootdroptrigger: this branch's server code), which raises the
     portal at its row's place for 60 minutes (ours), untouchable (talked to only), and says so to the players
     near; while it stands another rune raises no second one (its lock);
   - quest "Vallon Zek" becomes the rune's hand-in alone: one give step at the Portal of Zek, the rune taken, 20
     Portal Shards of Zek given (its old loot, teleport, talk and kill steps go: anyone may now speak to Vallon,
     below). A character on the quest past its first step is set done (one on Heirloom), so it can be taken again.
4. "Hand out the portal shards to those in your raid party, con yourself to use the portal. You are ported to an
   island in Zaoran Plateau (w)": Portal Shard of Zek (made from the rune's record: stacks of 20, tradeable, NO
   RENT, all classes and levels), whose Use (table itemspell, the valk-loot server code) casts spell 760380
   "Zek's Portal" (no cast time, no power): a self gate (spellgates) to the island's near side, where quest step 2
   used to land (4377.8, 253.5, 7052.8), the far side Vallon's. Each Use takes one shard off the stack.
5. "speak with Vallon ... he will equip it ... the orc graphic that Tallon was, the fat short Orc. He still wears
   plate armor but he is no longer a deathfist and no longer has that plate helm ... Vallon will tell you that
   you've found his training grounds and you are not welcome there ... in the middle of it he respawns kos along
   with some friends": trigger "Vallon Zek" starts on ANY player's talk to him (awaittalk start; before, only a
   character at the quest's talk step could), his answer, four seconds, a second line, then as before: his own
   row hidden, a hostile copy and three Guards of Vallon. His row keeps Tallon Zek's body (the ogre body
   0x84807490 at 4.5, which both rows have: a body that wears armour) and no helm; his armour goes from chain (3)
   to plate (4) on all five pieces. Sapphyra's Sword is NOT put in his hand: held drops (handlers/helddrop.go)
   would add a sword of its look to his corpse, and the sword itself is NO TRADE, which held drops never give, so
   another record of the look would drop: the owner's call (the report says which).
6. "you have 90 minutes from spawn time": the run's clock, 5,400 s (10 minutes with no one on the isle ends it).
   Won: "You have won the battle, but not the war!", fifteen minutes to loot, the isle cleared, the raid sent to
   the plateau's port-in. THE COOLDOWN (ours): his death fires the new trigger "Vallon Zek's Rest", which hides
   his row for two hours once the run is over (he can be spoken to again 2 h 16 min after the kill); a timeout or
   a wipe rests the trigger fifteen minutes after its end (and a new rune is needed: the shards are spent).
7. "Rage of Zek's appear to spawn mid-fight, and quad for 150k ... kill people who res back over and over" (the bug
   Kekoo owned): a fair add instead, ours: once a fight, below half his hit points, two Rages of Zek rise beside
   him ("Rage of Zek, to my side!") and join his fight, each at 15% of him (the template summon's share: 150,000
   hit points, his melee rolled at 15%, about 530 a hit where he lands 3,520, his AoE and AC debuff at 15%); they
   die with him and go when his fight ends any other way (entity/summon.go), so none is left on the isle to camp
   corpses. A spawn-only row "Rage of Zek" (Tallon's Rage's look: the ogre body in plate, size 1.4, level 64) by
   his row is what they copy.

Nothing of his loot is changed (the report lists the proposals).

Report only by default; --apply in one transaction, a gm_audit line a row and the log vallon_zek_1118_log (every row
before and after); a second --apply does nothing; --undo puts back exactly what it wrote (a row changed since is
left and said; a made record held by anyone is left) and drops the log. Refused whole if a row is not as this tool
expects. Copies the database first (<db>.bak-<when>-pre-vallonzek) unless --no-backup. --dump prints the triggers,
deaths and spawn-only names as JSON for the Go test (internal/handlers/vallon_zek_test.go).

WHEN IT TAKES EFFECT: NPC rows, spawn-only rows, aggro, templates, spell gates and quests at a start: apply in the
keep restart's before.sh, with this branch's build (lootdroptrigger; an older server ignores the table, and the
portal would never rise).

    python3 -E -s tools/vallon_zek_20261118.py --db eqoa.db [--apply | --undo | --dump] [--no-backup]
"""
import argparse
import copy
import datetime
import json
import math
import os
import sqlite3
import sys

LOG = "vallon_zek_1118_log"
WHO, VIA = "dab", "tool"
SRC = "vallonzek1118"
DETAIL = "The Vallon Zek raid as Ronson's t222 (the owner, 2026-11-18): tools/vallon_zek_20261118.py"

# places (world, x, z): the scouts' campfire and Vallon's isle
CAMP = (0, 15300.0, 18900.0)
CAMP_REACH = 300.0
ISLE = (5, 4372.0, 7105.0)
ISLE_REACH = 60.0

SCOUT = "Zek scout"
SCOUT_LEVEL_FROM, SCOUT_LEVEL = 58, 60
PORTAL = "Portal of Zek"
VALLON = "Vallon Zek"
GUARD = "Guard of Vallon"
RAGE = "Rage of Zek"
RAGE_FROM = "Tallon's Rage"
RAGE_LEVEL = 64
RAGE_AT = (4373.0, 253.5, 7120.0)
RUNE = "Portal Rune of Zek"
SHARD = "Portal Shard of Zek"
SHARD_DESC = ("A shard of a broken Portal Rune of Zek. Use it to step through to Vallon Zek's training grounds; "
              "the portal by the Zek scouts' campfire gives twenty for a rune.")
SHARDS = 20
GATE_FROM = "Nerius Gate"
GATE_SPELL = 760380
GATE_NAME = "Zek's Portal"
GATE_PLACE = "Vallon Zek's training grounds"
GATE_AT = (5, 4377.8, 253.5, 7052.8)
GATE_LINE = "Self, Teleports the caster to Vallon Zek's training grounds"
RUNE_TABLE = "zek portal rune"
RUNE_CHANCE, RUNE_ROLLS = 10, 1
PLATE = 4
ARMOR_COLS = ("torso", "forearm", "gloves", "legs", "feet")

QUEST_KEY = "Portal/zaoran-plateau/vallon"
TEMPLATE = "vallon zek"
SUMMON = {"kind": "summon", "below": 50, "count": 2, "npc": RAGE, "share": 15, "say": "Rage of Zek, to my side!"}

TRIG_VALLON, TRIG_GUARDS = "Vallon Zek", "Guards of Vallon"
TRIG_PORTAL, TRIG_REST = "Portal of Zek", "Vallon Zek's Rest"
PORTAL_MINUTES = 60
REST_MINUTES = 120

# trigger "Vallon Zek" as tools/guidegaps/vallon.json wrote it (refused if it is otherwise)
OLD_VALLON_STEPS = [
    {"do": "lock", "lock": "vallon zek"},
    {"do": "clock", "seconds": 5400, "empty": 600},
    {"do": "hide", "npc": "Vallon Zek", "radius": 60, "minutes": 95.0},
    {"do": "spawn", "npc": "Vallon Zek", "hostile": True, "spread": 0.5, "minutes": 95.0, "world": 5, "x": 4373.0,
     "y": 253.5, "z": 7105.0},
    {"do": "spawn", "npc": "Guard of Vallon", "hostile": True, "count": 3, "spread": 20.0, "minutes": 95.0, "world": 5,
     "x": 4373.0, "y": 253.5, "z": 7105.0},
    {"do": "broadcast", "text": "Vallon Zek: You have found my training grounds. Let me show you how Zek trains! (Ninety minutes.)"},
    {"do": "awaitdead", "npc": "Vallon Zek", "any": True},
    {"do": "stop", "slot": 133},
    {"do": "despawn", "npc": "Guard of Vallon"},
    {"do": "broadcast", "text": "Vallon Zek: You have won the battle, but not the war!"},
    {"do": "broadcast", "text": "Vallon Zek has fallen. His training grounds will not hold you long."},
    {"do": "wait", "seconds": 900},
    {"do": "end"},
    {"do": "stop", "slot": 133},
    {"do": "despawn"},
    {"do": "hide", "npc": "Vallon Zek", "radius": 60, "minutes": 0},
    {"do": "teleport", "world": 5, "x": 4899.0, "y": 148.0, "z": 7049.0},
]


def vallon_steps(guards_slot):
    """Trigger "Vallon Zek" as this tool writes it (the docstring's 5 and 6)."""
    spawn_v, spawn_g = copy.deepcopy(OLD_VALLON_STEPS[3]), copy.deepcopy(OLD_VALLON_STEPS[4])
    return [
        {"do": "lock", "lock": "vallon zek"},
        {"do": "awaittalk", "npc": VALLON, "start": True,
         "text": "So. You have found my training grounds. You are not welcome here."},
        {"do": "wait", "seconds": 4},
        {"do": "say", "npc": VALLON,
         "text": "My brother trains his armies in his fortress; I train mine here, in blood. You will learn as they did."},
        {"do": "clock", "seconds": 5400, "empty": 600},
        {"do": "hide", "npc": VALLON, "radius": 60, "minutes": 95.0},
        spawn_v,
        spawn_g,
        {"do": "broadcast", "text": "Vallon Zek: Let me show you how Zek trains! (Ninety minutes.)"},
        {"do": "awaitdead", "npc": VALLON, "any": True},
        {"do": "stop", "slot": guards_slot},
        {"do": "despawn", "npc": GUARD},
        {"do": "broadcast", "text": "Vallon Zek: You have won the battle, but not the war!"},
        {"do": "clock", "seconds": 3600},
        {"do": "broadcast", "text": "Vallon Zek has fallen. His training grounds will not hold you long."},
        {"do": "wait", "seconds": 900},
        {"do": "end"},
        {"do": "stop", "slot": guards_slot},
        {"do": "despawn"},
        {"do": "teleport", "world": 5, "x": 4899.0, "y": 148.0, "z": 7049.0},
        {"do": "hide", "npc": VALLON, "radius": 60, "minutes": 0},
        {"do": "wait", "seconds": 900},
    ]


def portal_steps(px, py, pz):
    return [
        {"do": "lock", "lock": "portal of zek"},
        {"do": "spawn", "npc": PORTAL, "untouchable": True, "spread": 0.5, "minutes": PORTAL_MINUTES, "world": 0,
         "x": px, "y": py, "z": pz},
        {"do": "broadcast", "text": "A dark portal tears open beside the Zek scouts' campfire."},
    ]


def rest_steps(vx, vy, vz):
    return [
        {"do": "lock", "lock": "vallon zek rest"},
        {"do": "place", "world": 5, "x": vx, "y": vy, "z": vz},
        {"do": "wait", "seconds": 960},
        {"do": "hide", "npc": VALLON, "radius": 60, "minutes": REST_MINUTES},
    ]


VALLON_NOTE = ("Ronson's t222 (the owner, 2026-11-18): spoken to by anyone, Vallon answers and respawns hostile with "
               "three guards; 90 minutes from his spawn; won: 15 minutes to loot, then the isle cleared (his two hours' "
               "rest is trigger \"Vallon Zek's Rest\"); lost or timed out: the trigger rests 15 minutes. "
               "tools/vallon_zek_20261118.py")
PORTAL_NOTE = ("t222: 'As soon as a Zek Scout drops a Portal Rune, the portal will spawn by the campfire': fired by a "
               "corpse that rolls the rune (lootdroptrigger); it stands 60 minutes (ours). tools/vallon_zek_20261118.py")
REST_NOTE = ("Vallon's death hides his row two hours once his run's fifteen minutes of looting are over: one raid "
             "cannot chain him (ours). tools/vallon_zek_20261118.py")


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


def item_by_name(db, name):
    r = db.execute("SELECT patternid FROM itempattern WHERE itemname = ? ORDER BY patternid", (name,)).fetchall()
    return [x[0] for x in r]


def trigger_named(db, name):
    r = db.execute("SELECT slot FROM gmtriggers WHERE name = ?", (name,)).fetchall()
    return [x[0] for x in r]


def check(db):
    """What the shard holds, refused when it is not as this tool expects: a dict of the ids it works on."""
    for t in ("npcs", "itempattern", "itemeffect", "loottables", "npcloot", "spellpattern", "spelleffect", "spellgates",
              "gmtriggers", "npcdeath", "npcspawnonly", "npcaggro", "classtemplates", "quests", "queststeps",
              "quest_import_quests", "charquests", "norentvanish", "gm_audit"):
        if not has(db, t):
            raise Refused(f"no table {t}")
    ids = {}
    scouts = near(db, SCOUT, CAMP, CAMP_REACH)
    if not scouts:
        raise Refused(f"no {SCOUT} within {CAMP_REACH} of the campfire")
    levels = {db.execute("SELECT npc_level FROM npcs WHERE id = ?", (i,)).fetchone()[0] for i in scouts}
    if levels != {SCOUT_LEVEL_FROM}:
        raise Refused(f"the scouts' levels are {sorted(levels)}, not {SCOUT_LEVEL_FROM}")
    ids["scouts"] = scouts
    for name, place, reach, key in ((PORTAL, CAMP, CAMP_REACH, "portal"), (VALLON, ISLE, ISLE_REACH, "vallon")):
        r = near(db, name, place, reach)
        if len(r) != 1:
            raise Refused(f"{len(r)} rows of {name!r} at its place, not one")
        ids[key] = r[0]
    v = row(db, "npcs", {"id": ids["vallon"]})
    if v["npc_type"] & 0x80 == 0:
        raise Refused(f"{VALLON}'s row does not carry the NPC bit (white-faced until spoken to)")
    if [v[c] for c in ARMOR_COLS] != [3] * 5 or v["head"] != 0:
        raise Refused(f"{VALLON}'s armour is {[v[c] for c in ARMOR_COLS]} head {v['head']}, not chain and no helm")
    rage = db.execute("SELECT id FROM npcs WHERE npc_name = ? AND world = 5 ORDER BY id", (RAGE_FROM,)).fetchall()
    if not rage:
        raise Refused(f"no {RAGE_FROM!r} row to copy")
    ids["rage_from"] = rage[0][0]
    if db.execute("SELECT 1 FROM npcs WHERE lower(npc_name) = lower(?)", (RAGE,)).fetchone():
        raise Refused(f"an NPC {RAGE!r} exists already")
    for name in (SCOUT, RAGE, PORTAL):
        if name != PORTAL and row(db, "npcaggro", {"npc_name": name.lower()}):
            raise Refused(f"an npcaggro row for {name!r} exists already")
        if db.execute("SELECT 1 FROM npcspawnonly WHERE lower(name) = lower(?)", (name,)).fetchone():
            raise Refused(f"{name!r} is spawn-only already")
    rune = item_by_name(db, RUNE)
    if len(rune) != 1:
        raise Refused(f"{len(rune)} records {RUNE!r}, not one")
    ids["rune"] = rune[0]
    r = row(db, "itempattern", {"patternid": rune[0]})
    if r["rent"] != 1:
        raise Refused(f"the rune's rent is {r['rent']}, not 1")
    if item_by_name(db, SHARD):
        raise Refused(f"a record {SHARD!r} exists already")
    if row(db, "norentvanish", {"patternid": rune[0]}):
        raise Refused("the rune has a norentvanish row already")
    for sid in (GATE_SPELL,):
        if row(db, "spellpattern", {"spellid": sid}) or row(db, "spelleffect", {"spellid": sid}):
            raise Refused(f"spell id {sid} is taken")
    g = db.execute("SELECT spellid FROM spellpattern WHERE lower(spellname) = lower(?) ORDER BY spellid", (GATE_FROM,)).fetchall()
    if not g or not row(db, "spelleffect", {"spellid": g[0][0]}):
        raise Refused(f"no spell {GATE_FROM!r} to copy")
    ids["gate_from"] = g[0][0]
    if row(db, "spellgates", {"spell": GATE_NAME.lower()}):
        raise Refused(f"a spell gate {GATE_NAME!r} exists already")
    if db.execute("SELECT 1 FROM loottables WHERE tablename = ?", (RUNE_TABLE,)).fetchone() or \
            db.execute("SELECT 1 FROM npcloot WHERE tablename = ?", (RUNE_TABLE,)).fetchone():
        raise Refused(f"table {RUNE_TABLE!r} exists already")
    if has(db, "lootdroptrigger") and db.execute("SELECT 1 FROM lootdroptrigger WHERE patternid = ?", (rune[0],)).fetchone():
        raise Refused("the rune fires a trigger already")
    if has(db, "itemspell") and db.execute("SELECT count(*) FROM itemspell").fetchone() is None:
        raise Refused("itemspell unreadable")
    # the guide-gaps set's triggers and template, as it wrote them
    for name in (TRIG_VALLON, TRIG_GUARDS):
        s = trigger_named(db, name)
        if len(s) != 1:
            raise Refused(f"{len(s)} triggers named {name!r}, not one")
    ids["trig_vallon"], ids["trig_guards"] = trigger_named(db, TRIG_VALLON)[0], trigger_named(db, TRIG_GUARDS)[0]
    old = json.loads(row(db, "gmtriggers", {"slot": ids["trig_vallon"]})["steps"])
    want = copy.deepcopy(OLD_VALLON_STEPS)
    for s in want:
        if s["do"] == "stop":
            s["slot"] = ids["trig_guards"]
    if js(old) != js(want):
        raise Refused(f"trigger {TRIG_VALLON!r}'s steps are not as tools/guidegaps/vallon.json wrote them")
    for name in (TRIG_PORTAL, TRIG_REST):
        if trigger_named(db, name):
            raise Refused(f"a trigger {name!r} exists already")
    if db.execute("SELECT 1 FROM npcdeath WHERE name = ?", (VALLON.lower(),)).fetchone():
        raise Refused(f"{VALLON!r}'s death fires a trigger already")
    t = row(db, "classtemplates", {"name": TEMPLATE})
    if t is None:
        raise Refused(f"no template {TEMPLATE!r}")
    if any(a.get("kind") == "summon" for a in json.loads(t["abilities"] or "[]")):
        raise Refused(f"template {TEMPLATE!r} summons already")
    # the quest
    q = db.execute("SELECT questid FROM quest_import_quests WHERE key = ?", (QUEST_KEY,)).fetchone()
    if not q:
        raise Refused(f"no quest {QUEST_KEY!r}")
    ids["quest"] = q[0]
    steps = db.execute("SELECT step, kind, target, items FROM queststeps WHERE questid = ? ORDER BY step", (q[0],)).fetchall()
    want_steps = [(1, "loot", SCOUT, str(rune[0])), (2, "give", PORTAL, str(rune[0])), (3, "talk", VALLON, ""),
                  (4, "kill", VALLON, "")]
    if [tuple(s) for s in steps] != want_steps:
        raise Refused(f"quest {q[0]}'s steps are {steps}, not the import's")
    return ids


def report(db, ids):
    print(f"  {SCOUT}: rows {ids['scouts']} level {SCOUT_LEVEL_FROM} -> {SCOUT_LEVEL}, never attack on sight")
    print(f"  {RUNE} ({ids['rune']}): NO RENT, {RUNE_CHANCE}% x {RUNE_ROLLS} off every {SCOUT} (table {RUNE_TABLE!r}); "
          f"its roll fires trigger {TRIG_PORTAL!r}")
    print(f"  {PORTAL} (row {ids['portal']}): spawn-only; raised {PORTAL_MINUTES} minutes by the rune")
    print(f"  quest {ids['quest']}: one give step: the rune for {SHARDS} {SHARD} (made, NO RENT, tradeable, stacks of {SHARDS})")
    stuck = db.execute("SELECT serverid, step FROM charquests WHERE questid = ? AND done = 0 AND step > 1",
                       (ids["quest"],)).fetchall()
    print(f"  characters on it past step 1, set done: {stuck}")
    print(f"  {SHARD}: Use casts spell {GATE_SPELL} {GATE_NAME!r} (copy of {GATE_FROM} {ids['gate_from']}), a self gate "
          f"to {GATE_AT}")
    print(f"  {VALLON} (row {ids['vallon']}): armour chain -> plate, no helm, Tallon's body kept")
    print(f"  {RAGE}: a spawn-only row from {RAGE_FROM} ({ids['rage_from']}) at {RAGE_AT}, level {RAGE_LEVEL}; template "
          f"{TEMPLATE!r} gains {js(SUMMON)}")
    print(f"  trigger {TRIG_VALLON!r} ({ids['trig_vallon']}): starts on a talk, 90 minutes, rests 15 after a loss; "
          f"new triggers {TRIG_PORTAL!r} and {TRIG_REST!r} (his death: hidden {REST_MINUTES} minutes after the loot)")
    hd = held_drop_note(db, ids)
    if hd:
        print(f"  not done: Sapphyra's Sword in his hand would also drop {hd} (held drops)")


def held_drop_note(db, ids):
    """Which records a held Sapphyra's Sword's look would drop of (helddrops.go's rule, roughly: the look's
    tradeable, rentable, not LORE records in a hand's slot at or below level 68, the top level's)."""
    r = db.execute("SELECT model FROM itempattern WHERE itemname = ? ORDER BY patternid LIMIT 1", ("Sapphyra's Sword",)).fetchone()
    if not r:
        return ""
    rows = db.execute("SELECT itemname, levelreq FROM itempattern WHERE model = ? AND equipslot IN (12, 14, 15, 16, 18) "
                      "AND trade = 0 AND rent != 0 AND lore = 0 AND levelreq <= 68 ORDER BY levelreq", (r[0],)).fetchall()
    bar = ("epic", "artifact", "heirloom", "test", "ascended")
    rows = [(n, l) for n, l in rows if not any(b in n.lower() for b in bar)]
    if not rows:
        return "nothing"
    top = rows[-1][1]
    return " or ".join(sorted({n for n, l in rows if l == top}))


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
                   (at, acct[0] if acct else 0, WHO, VIA, action, tbl, js(key), js(before), js(after), DETAIL))

    def insert(tbl, r, key):
        db.execute(f'INSERT INTO "{tbl}" ({", ".join(chr(34) + c + chr(34) for c in r)}) VALUES '
                   f'({", ".join("?" * len(r))})', list(r.values()))
        done(tbl, key, None)

    def insert_auto(tbl, r, keycol):
        cur = db.execute(f'INSERT INTO "{tbl}" ({", ".join(chr(34) + c + chr(34) for c in r)}) VALUES '
                         f'({", ".join("?" * len(r))})', list(r.values()))
        done(tbl, {keycol: cur.lastrowid}, None)
        return cur.lastrowid

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

    seqs = {t: row(db, "sqlite_sequence", {"name": t}) for t in ("itempattern", "loottables", "npcs", "gmtriggers")}

    # 1. the scouts: level 60, white-faced
    for rid in ids["scouts"]:
        update("npcs", {"id": rid}, {"npc_level": SCOUT_LEVEL})
    insert("npcaggro", {"npc_name": SCOUT.lower(), "aggressive": 0, "radius": 0,
                        "note": "t222: 'con white@60, and are white-faced': never attack on sight", "source": SRC},
           {"npc_name": SCOUT.lower()})

    # 2. the rune: NO RENT, off every scout
    rune = ids["rune"]
    update("itempattern", {"patternid": rune}, {"rent": 0})
    insert("norentvanish", {"patternid": rune, "note": "t222: 'a Portal Rune, No Rent'", "source": SRC},
           {"patternid": rune})
    lid = insert_auto("loottables", {"tablename": RUNE_TABLE, "patternid": rune, "weight": 1, "minlevel": 1,
                                     "maxlevel": 70, "source": SRC}, "id")
    insert("npcloot", {"npc_name": SCOUT.lower(), "tablename": RUNE_TABLE, "chance": RUNE_CHANCE, "rolls": RUNE_ROLLS,
                       "note": "t222: 'I had one drop after I'd killed about 10 scouts'", "source": SRC},
           {"npc_name": SCOUT.lower(), "tablename": RUNE_TABLE})
    made["loottables " + RUNE_TABLE] = lid

    # 3. the portal: spawn-only, raised by the rune
    insert("npcspawnonly", {"name": PORTAL.lower(), "note": PORTAL_NOTE, "source": SRC}, {"name": PORTAL.lower()})
    db.execute("""CREATE TABLE IF NOT EXISTS lootdroptrigger (
		patternid INTEGER PRIMARY KEY,
		trigger   TEXT NOT NULL,
		note      TEXT NOT NULL DEFAULT '',
		source    TEXT NOT NULL DEFAULT 'gm'
	)""")
    insert("lootdroptrigger", {"patternid": rune, "trigger": TRIG_PORTAL, "note": PORTAL_NOTE, "source": SRC},
           {"patternid": rune})
    p = row(db, "npcs", {"id": ids["portal"]})
    next_slot = (db.execute("SELECT max(slot) FROM gmtriggers").fetchone()[0] or 8) + 1
    insert("gmtriggers", {"slot": next_slot, "name": TRIG_PORTAL, "enabled": 1, "scope": "near", "radius": 150,
                          "world": 0, "x": p["x_coord"], "y": p["y_coord"], "z": p["z_coord"],
                          "steps": js_steps(portal_steps(p["x_coord"], p["y_coord"], p["z_coord"])),
                          "notes": PORTAL_NOTE}, {"slot": next_slot})
    made["trigger " + TRIG_PORTAL] = next_slot

    # 4. the shards and their gate
    r = dict(row(db, "itempattern", {"patternid": rune}))
    del r["patternid"]
    r.update({"itemname": SHARD, "itemdesc": SHARD_DESC, "maxstack": SHARDS, "lore": 0, "trade": 0, "rent": 0,
              "levelreq": 1, "classuse": 32767})
    shard = insert_auto("itempattern", r, "patternid")
    made[SHARD] = shard
    insert("norentvanish", {"patternid": shard, "note": "t222: '20 Portal Shards (no rent)'", "source": SRC},
           {"patternid": shard})
    insert("itemeffect", {"patternid": shard, "line": GATE_LINE, "name": GATE_NAME, "charges": 1, "source": SRC},
           {"patternid": shard})
    src = row(db, "spellpattern", {"spellid": ids["gate_from"]})
    insert("spellpattern", dict(src, spellid=GATE_SPELL, spellname=GATE_NAME, abilitylvl=1, casttime=0, power=0, recast=0,
                                spelldesc=GATE_LINE), {"spellid": GATE_SPELL})
    fx = row(db, "spelleffect", {"spellid": ids["gate_from"]})
    eff = json.loads(fx["effects"]) if fx["effects"] else {}
    if eff:
        eff["level"] = 1
        for e in eff.get("effects", []):
            if e.get("kind") == "teleport":
                e["location"] = f"Zaoran Plateau,{GATE_AT[1]},{GATE_AT[2]},{GATE_AT[3]}"
    insert("spelleffect", {"spellid": GATE_SPELL, "line": GATE_LINE, "fxdict": fx["fxdict"],
                           "source": f"{SRC}: {DETAIL} (visual {GATE_FROM}'s)", "effects": js(eff) if eff else ""},
           {"spellid": GATE_SPELL})
    insert("spellgates", {"spell": GATE_NAME.lower(), "kind": "self", "place": GATE_PLACE, "world": GATE_AT[0],
                          "x": GATE_AT[1], "y": GATE_AT[2], "z": GATE_AT[3], "radius": 0,
                          "note": "t222: 'con yourself to use the portal. You are ported to an island in Zaoran Plateau "
                                  "(w)': the isle's near side, where the quest's teleport landed", "source": SRC},
           {"spell": GATE_NAME.lower()})
    db.execute("""CREATE TABLE IF NOT EXISTS itemspell (
		patternid INTEGER PRIMARY KEY,
		spellid   INTEGER NOT NULL,
		rest_s    REAL NOT NULL DEFAULT 0,
		note      TEXT NOT NULL DEFAULT '',
		source    TEXT NOT NULL DEFAULT 'gm'
	)""")
    insert("itemspell", {"patternid": shard, "spellid": GATE_SPELL, "rest_s": 0,
                         "note": "t222: a Portal Shard of Zek sends its user to Vallon Zek's isle", "source": SRC},
           {"patternid": shard})

    # the quest: the rune's hand-in alone
    qid = ids["quest"]
    old2 = row(db, "queststeps", {"questid": qid, "step": 2})
    for st in (1, 2, 3, 4):
        delete("queststeps", {"questid": qid, "step": st})
    new1 = dict(old2, step=1, journal="Bring a Portal Rune of Zek to the portal by the Zek scouts' campfire.",
                says="The portal takes your Portal Rune of Zek and breaks it into twenty Portal Shards. Give one to "
                     "each of your raid: each of you, use your shard to step through to Vallon Zek's island.",
                rewarditems=f"{shard}*{SHARDS}", events="")
    insert("queststeps", new1, {"questid": qid, "step": 1})
    update("quests", {"questid": qid}, {
        "offer": "A dark portal, opened by the goblins' rune. Bring me a Portal Rune of Zek and I will break it into "
                 "shards: each shard opens my way to Vallon Zek's training grounds for one of your raid.",
        "accept": "I have a rune.", "started": "Show me the rune.",
        "notes": "Ronson's t222 (eqoa.forumotion.com/t222-vallon-zek), the owner's 2026-11-18 ask: the rune drops "
                 "off any Zek scout (table 'zek portal rune', 10%), its roll raises this portal (lootdroptrigger), the "
                 "rune is broken into 20 Portal Shards of Zek, each shard's Use a self gate to Vallon's isle; Vallon "
                 "is spoken to by anyone (trigger 'Vallon Zek'). tools/vallon_zek_20261118.py"})
    for (sid,) in db.execute("SELECT serverid FROM charquests WHERE questid = ? AND done = 0 AND step > 1 ORDER BY serverid",
                             (qid,)).fetchall():
        update("charquests", {"serverid": sid, "questid": qid}, {"step": 0, "done": 1})

    # 5. Vallon: plate, no helm; his trigger starts on a talk; his rest
    update("npcs", {"id": ids["vallon"]}, {c: PLATE for c in ARMOR_COLS})
    v = row(db, "npcs", {"id": ids["vallon"]})
    update("gmtriggers", {"slot": ids["trig_vallon"]}, {"steps": js_steps(vallon_steps(ids["trig_guards"])),
                                                        "notes": VALLON_NOTE})
    rest_slot = (db.execute("SELECT max(slot) FROM gmtriggers").fetchone()[0] or 8) + 1
    insert("gmtriggers", {"slot": rest_slot, "name": TRIG_REST, "enabled": 1, "scope": "near", "radius": 220,
                          "world": 5, "x": v["x_coord"], "y": v["y_coord"], "z": v["z_coord"],
                          "steps": js_steps(rest_steps(v["x_coord"], v["y_coord"], v["z_coord"])), "notes": REST_NOTE},
           {"slot": rest_slot})
    made["trigger " + TRIG_REST] = rest_slot
    insert("npcdeath", {"name": VALLON.lower(), "trigger": TRIG_REST, "chance": 100, "note": REST_NOTE, "source": SRC},
           {"name": VALLON.lower(), "trigger": TRIG_REST})

    # 7. the Rage of Zek, a fair add
    r = dict(row(db, "npcs", {"id": ids["rage_from"]}))
    del r["id"]
    r.update({"npc_name": RAGE, "x_coord": RAGE_AT[0], "y_coord": RAGE_AT[1], "z_coord": RAGE_AT[2],
              "npc_level": RAGE_LEVEL, "npc_id": 0})
    made[RAGE] = insert_auto("npcs", r, "id")
    insert("npcspawnonly", {"name": RAGE.lower(), "note": "Vallon Zek's summon below half (template 'vallon zek'): "
                            "t222's Rage of Zek as a fair add, ours", "source": SRC}, {"name": RAGE.lower()})
    t = row(db, "classtemplates", {"name": TEMPLATE})
    ab = json.loads(t["abilities"] or "[]") + [SUMMON]
    update("classtemplates", {"name": TEMPLATE}, {"abilities": json.dumps(ab, separators=(",", ":"))})

    for tb, before in seqs.items():
        if before is not None:
            done("sqlite_sequence", {"name": tb}, before)
    return made


def js_steps(steps):
    return json.dumps(steps, separators=(",", ":"), ensure_ascii=False)


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
            if tbl == "itempattern" and has(db, "charinventory"):
                held = db.execute("SELECT COUNT(*) FROM charinventory WHERE patternid = ?", (key["patternid"],)).fetchone()[0]
                if held:
                    print(f"  itempattern {key['patternid']}: held by {held}, left")
                    left += 1
                    continue
            db.execute(f'DELETE FROM "{tbl}" WHERE {where}', list(key.values()))
        elif after is None:
            b = json.loads(before)
            db.execute(f'INSERT INTO "{tbl}" ({", ".join(chr(34) + c + chr(34) for c in b)}) VALUES '
                       f'({", ".join("?" * len(b))})', list(b.values()))
        else:
            b = json.loads(before)
            db.execute(f'UPDATE "{tbl}" SET {", ".join(chr(34) + c + chr(34) + " = ?" for c in b)} WHERE {where}',
                       list(b.values()) + list(key.values()))
        n += 1
    db.execute(f"DROP TABLE {LOG}")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', ?, '{}', NULL, NULL, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, LOG, f"undone, {n} rows put back, {left} left: " + DETAIL))
    print(f"  {n} rows put back, {left} left")


def dump():
    """The triggers, deaths and spawn-only names as the Go test reads them (zpSetFile), at the shards' places
    (Vallon's row 4372, 253.5, 7105; the portal's 15299.4, 58.6, 18920.0); slots 132 and 133 the guide's, 901 and
    902 the new ones'."""
    vx, vy, vz = 4372.0, 253.5, 7105.0
    px, py, pz = 15299.4, 58.6, 18920.0
    guards = [{"do": "wait", "seconds": 15},
              {"do": "spawn", "npc": GUARD, "hostile": True, "count": 1, "max": 3, "spread": 20.0, "minutes": 30,
               "world": 5, "x": 4373.0, "y": 253.5, "z": 7105.0},
              {"do": "awaitdead"}]
    out = {
        "triggers": [
            {"slot": 132, "name": TRIG_VALLON, "scope": "near", "radius": 220, "world": 5, "x": 4372.0, "y": 253.5,
             "z": 7140.0, "steps": vallon_steps(133)},
            {"slot": 133, "name": TRIG_GUARDS, "scope": "near", "radius": 220, "world": 5, "x": 4372.0, "y": 253.5,
             "z": 7140.0, "steps": guards},
            {"slot": 901, "name": TRIG_PORTAL, "scope": "near", "radius": 150, "world": 0, "x": px, "y": py, "z": pz,
             "steps": portal_steps(px, py, pz)},
            {"slot": 902, "name": TRIG_REST, "scope": "near", "radius": 220, "world": 5, "x": vx, "y": vy, "z": vz,
             "steps": rest_steps(vx, vy, vz)},
        ],
        "npcs": [{"name": GUARD, "spawnonly": True}, {"name": PORTAL, "spawnonly": True},
                 {"name": RAGE, "spawnonly": True}],
        "deaths": [{"name": GUARD, "trigger": TRIG_GUARDS, "chance": 100},
                   {"name": VALLON, "trigger": TRIG_REST, "chance": 100}],
        "summon": SUMMON,
        "drop": {"item": RUNE, "trigger": TRIG_PORTAL},
    }
    print(json.dumps(out, indent=1))


def backup(path):
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-vallonzek"
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
    print("applied; loot, items and quests' rows at once; NPC rows, spawn-only rows, aggro, the template, spell gates, "
          "triggers and quests are read at the next start (this branch's build)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
