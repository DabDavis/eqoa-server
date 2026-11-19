#!/usr/bin/env python3
"""The Lord Valkanith quest made as the owner's source has it: Ronson's "Envar Island Quest-Lord Valkanith" on the EQOA
forum (eqoa.forumotion.com/t171-lord-valkanith-quest, 2010-07-26), sent by the owner on 2026-11-17; a GM's work
board entry 737 (forum t171, t281, t299; his rules and estimates) where Ronson gives no figure.

THE POST: level 50+, all classes. The Spirit of Tellainain, on a small island off the south-west corner of Envar, asks
three gems: the emerald off the Tal'Thex inside the city, the ruby off Geomancer's Citadel's rallosian spectres and
warlords, the diamond from Sycamore Joy's Rest. Talking to him with all three brings an ambush of Tal'thex Knights,
Soldiers and Sorcerers (dark blue to a 60, yellow to a 55); the undead keep coming for about 30 minutes; "a dying
Tal'Thex Sorcerer announces Lord Valkanith"; he despawns after 30 minutes and the gems must be farmed again. Pet-tank
him away from the adds; melee keep off, his bloodtap AoE heals him; hide behind rocks; a resist cleric. Three
necromancers and a resist cleric took him in about 8 minutes. He can drop the Bone Wing Sentinel (a rune, 100 charges,
not rechargeable, level 55, all/all): it summons a thexian bonewing familiar, a lifetap damage shield (40 damage, 20
heal) and +40 PR and DR.

WHAT DIFFERED (quest 101269 "Envar Island: Lord Valkanith", giver Spirit of Tellainain):
  - Step 5 asked three Tal'thex sorcerer kills of the step's own spawns, a knight, a soldier and a sorcerer before
    each player (entry 737's broken step: one sorcerer a wave), and step 6 raised Lord Valkanith before the player for
    the engine's ten minutes. The post's event is one for everyone there: now step 4 (the gems handed in) fires the
    trigger "Lord Valkanith" (one at a time: its lock; the gems are kept while another's runs), which brings seven
    waves of a knight, a soldier and a sorcerer four minutes apart (28 minutes: the post's "about 30"; the wave and its
    size ours), then waits on a sorcerer's death (up to 15 minutes, a fresh one raised with it), which says "Lord
    Valkanith... comes..." and raises him for 30 minutes (the post), fought or not. Step 5 is his kill; step 6 is gone.
    A run that ends without his death puts the quest back to step 4 with the gems handed back (questencounter.go, the
    owner's rule of 2026-11-13) where the era made the player farm them again.
  - The quest was not repeatable: the post's era redid him by farming the gems again (and "delete the quest from your
    log before he dies, or you can't log it again", a bug of the era's). Repeatable now (entry 737 too).
  - Two Spirits of Tellainain stood: row 127000 (level 60, on the island off the south-west corner, water all round
    it in the server's ground, worlds/ground 1.env) and row 123015 (level 55, on the mainland at 4900, 9400 among the
    Envaric dead: tools/seed_quest_npcs.py's "the place is ours"). 123015 and its npcrespawn row go; the event's place
    is 127000's.
  - Lord Valkanith (123022) fought as a level 60 roamer (hp_pct 100), with no loot. Now his own template: hp_pct 3993
    (~575,000 hit points at 144 a point: entry 737's for "maybe 8 minutes" under three necromancers' pets), melee 473%
    (~250 a hit, entry 737), the roamer's Critical Hit kept, and Valkanith's Bloodtap (new spell 760352): a life tap of
    500 on every player and pet within 30 of him he can see (line of sight: the post's rocks hide), disease (DR, the
    post's resist cleric; t281's "DR or PR"), every 12 s from 8 s, all that lands healing him (the post's "his AoE
    bloodtap will take your life and heal him"; 500 a GM's rule, radius, timer and school entry 737's).
  - His drop: table "boss lord valkanith" (a boss table: need before greed), 1 roll at 100%: the Bone Wing Sentinel
    (the shard's record of that name: Heirloom 991104, Live 1000036). With --t299 the table also holds the phylactery
    of valkanith (7982) and the positronian energy shield (6067), entry 737's t299 records that exist (the shield's
    classuse 32766 to 32767 on Live too, as a GM set it on Heirloom on 2026-11-12). No new item.
  - The Bone Wing Sentinel had no use. Now (itemeffect) 100 charges, the post's familiar as a buff on its bearer,
    "Thexian Bonewing": a damage shield of 40 and +40 PR and DR, for 30 minutes (the time ours). Not carried: the
    shield's 20 heal (an item use's line has no words for a heal when hit: ParseSpellText leaves "Heal 20 when hit"
    unread; the workbook's def_proc is a spell's alone) and the familiar's body (no pet).
  - The Tal'thex waves: knights 56 to 58, soldiers and sorcerers 55 to 57 (the post's con; entry 737's levels), their
    rows' HP as they are. Undead (npcundead) as the post's "undead": the three kinds, and Lord Valkanith on Live (on
    Heirloom since a GM's 2026-11-12 row).
  Left as they were: the gem steps (1-3), their drop chance (0.25) and the creatures they name; the offer; Lord
    Valkanith's level (60); the seeded wave and boss rows' places (spawn-only; the trigger places their copies).

Report only by default; --apply in one transaction with gm_audit lines and the log valkanith_1117_log (every row
before and after); a second --apply does nothing; --undo puts back exactly what it wrote (a row changed since is left
and said) and drops the log. Refused whole if a row is not as this tool expects.

WHEN IT TAKES EFFECT: quests at a start (a keep-restart's before.sh); npcs, templates, npctemplate, npcundead and the
spell at the GM panel's reload_npcs or a start; loot, itemeffect, items and the trigger at a start. Run it in the
before.sh, the server stopped.

    python3 -E -s tools/valkanith_20261117.py --db eqoa.db [--t299] [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "valkanith_1117_log"
WHO, VIA = "dab", "tool"
SRC = "valkanith1117"
DETAIL = ("the Lord Valkanith quest as Ronson's post (the owner's source, 2026-11-17) and work board entry 737: "
          "tools/valkanith_20261117.py")
POST = "Ronson's post (eqoa.forumotion.com/t171, the owner's source 2026-11-17)"

QUEST = 101269
GIVER = "Spirit of Tellainain"
GIVER_ROW, GIVER_DUP = 127000, 123015  # the island's; the seeder's on the mainland
GIVER_DUP_PLACE = (1, 4900.0, 9400.0)
GIVER_ROW_PLACE = (1, 4160.0, 9809.0)
LORD_ROW = 123022
LORD = "lord valkanith"  # NPCClassKey
LEVELS = {123016: ("Tal'thex knight", 56, 58), 123017: ("Tal'thex knight", 56, 58),
          123018: ("Tal'thex soldier", 55, 57), 123019: ("Tal'thex soldier", 55, 57),
          123020: ("Tal'thex sorcerer", 55, 57), 123021: ("Tal'thex sorcerer", 55, 57)}
WAVE = ("Tal'thex knight", "Tal'thex soldier", "Tal'thex sorcerer")
UNDEAD = ("tal'thex knight", "tal'thex soldier", "tal'thex sorcerer", LORD)

RUNE = "Bone Wing Sentinel"
PHYLACTERY, SHIELD = 7982, 6067
T299 = {PHYLACTERY: "phylactery of valkanith", SHIELD: "positronian energy shield"}
LOOT_TABLE = "boss lord valkanith"
LOOT_CHANCE, LOOT_ROLLS = 100, 1

TAP = 760352
TAP_NAME = "Valkanith's Bloodtap"
TAP_FROM = 905140  # Frost Breath: a self-centred area (Lady Vox's); its record's cast and range
TAP_FX = 1436      # Bloodwasp's look (a necromancer's drain)
TAP_DMG, TAP_RADIUS = 500, 30
TAP_LINE = f"Self, aoe Drain HP by {TAP_DMG} give self, Radius {TAP_RADIUS}"
TAP_EFFECTS = {"v": 1, "scope": "player_aoe", "target_type": "Enemy", "harmful": True, "stack": 0, "level": 60,
               "min_level": 1, "radius": TAP_RADIUS, "req_equipment": "Any", "sheet_type": "Drain",
               "effects": [{"type": "Drain", "kind": "drain", "amount": -TAP_DMG, "resist": "dr"}]}

ABILITIES = [{"kind": "lifetap", "spell_id": TAP, "damage": TAP_DMG, "resist": "DR", "every": 12, "first": 8}]
TEMPLATE_FROM = "roamer"
TEMPLATE = {"hp_pct": 3993, "melee_pct": 473}
TEMPLATE_WORDS = (" (2026-11-17, tools/valkanith_20261117.py, " + POST + ": 'his AoE bloodtap will take your life and "
                  "heal him', 'maybe 8 minutes' under three necromancers and a resist cleric; work board 737: level 60, "
                  "hp_pct 3993 (~575k), melee 473% (~250 a hit), Valkanith's Bloodtap a 500 DR life tap on every player "
                  "and pet within 30 he sees, every 12 s from 8 s, healing him by what lands)")

RUNE_LINE = "Self, Damage Shield Decrease Hitpoints by 40, Increase PR by 40, Increase DR by 40, 30min"
RUNE_EFFECT = "Thexian Bonewing"
RUNE_CHARGES = 100

TRIGGER = "Lord Valkanith"
LOCK = "lord valkanith"
BUSY = "The Tal'thex dead are awake already for others. Come back when it is over."
WAVES, WAVE_EVERY = 7, 240
SPREAD = 15.0


def trigger_steps(world, x, y, z):
    steps = [{"do": "lock", "lock": LOCK},
             {"do": "clock", "seconds": 4800, "empty": 600},
             {"do": "broadcast", "text": "The gems wake the Tal'thex! Their dead rise on Envar's island."}]
    for i in range(WAVES):
        for n in WAVE:
            steps.append({"do": "spawn", "npc": n, "hostile": True, "minutes": 30, "spread": SPREAD})
        steps.append({"do": "wait", "seconds": WAVE_EVERY})
    steps += [
        {"do": "spawn", "npc": "Tal'thex sorcerer", "hostile": True, "minutes": 30, "spread": SPREAD},
        {"do": "awaitdead", "npc": "Tal'thex sorcerer", "any": True, "seconds": 900,
         "fail": "The Tal'thex dead sink back into the island. Lord Valkanith will not come."},
        {"do": "say", "npc": "Tal'thex sorcerer", "text": "Lord Valkanith... comes..."},
        {"do": "spawn", "npc": "Lord Valkanith", "hostile": True, "minutes": 30, "spread": 5},
        {"do": "broadcast", "text": "Lord Valkanith has come to Envar's island. He will not stay long."},
        {"do": "awaitdead", "npc": "Lord Valkanith", "seconds": 1800},
        {"do": "end"},
        {"do": "despawn"},
    ]
    return steps


QUEST_OLD = {"repeatable": 0, "notes": (
    "From EQOA-Data Quests/Misc/lord-valkanith.json, # Lord Valkanith Quest (github.com/Jadiction/EQOA-Data, MIT). The "
    "gems drop off the creatures the guide names: the city of Takish'Hiz's (its star knights here), Geomancer's "
    "Citadel's rallosian spectres and warlords (the spectral rallosians), Sycamore Joy's Rest's dark elves (its "
    "Teir'dal knights). The guide's half-hour of undead before Lord Valkanith is three sorcerers here. His 30-minute "
    "despawn and the advice to delete the quest before he dies are left out; his Bone Wing Sentinel rune is loot.")}
QUEST_NEW = {"repeatable": 1, "notes": (
    "From " + POST + " (= EQOA-Data Quests/Misc/lord-valkanith.json, github.com/Jadiction/EQOA-Data, MIT) and work "
    "board entry 737: the gems handed in fire the trigger 'Lord Valkanith' (one at a time) on the island: seven waves "
    "of Tal'thex knights, soldiers and sorcerers four minutes apart, then a dying sorcerer calls Lord Valkanith, who "
    "stays 30 minutes. A run ended without his death gives the gems back at step 4. Repeatable: the era farmed the gems "
    "again. His Bone Wing Sentinel is loot (boss lord valkanith). tools/valkanith_20261117.py.")}

STEP4_OLD = {"journal": "Bring the three gems to the Spirit of Tellainain on Envar's island.", "events": ""}
STEP4_NEW = {"journal": "Bring the three gems to the Spirit of Tellainain on the small island off the south-west "
                        "corner of Envar. Bring friends: the dead will come.",
             "events": json.dumps([{"on": "done", "do": "trigger", "trigger": TRIGGER, "text": BUSY}],
                                  separators=(",", ":"))}
STEP5_OLD = {"kind": "kill", "target": "Tal'thex sorcerer", "count": 3,
             "journal": "Fight off the Tal'thex undead: kill their sorcerers.",
             "events": '[{"on":"start","do":"spawn","npc":"Tal\'thex knight","hostile":true},{"on":"start","do":"spawn",'
                       '"npc":"Tal\'thex soldier","hostile":true},{"on":"start","do":"spawn","npc":"Tal\'thex sorcerer",'
                       '"hostile":true}]'}
STEP5_NEW = {"kind": "kill", "target": "Lord Valkanith", "count": 1,
             "journal": "Hold the island against the Tal'thex dead for half an hour, until a dying sorcerer calls Lord "
                        "Valkanith. Kill him within half an hour of his coming; keep away from him, his bloodtap heals "
                        "him.",
             "events": ""}
STEP6_OLD = {"kind": "kill", "target": "Lord Valkanith", "count": 1,
             "events": '[{"on":"start","do":"say","text":"Lord Valkanith... comes...","npc":"Tal\'thex sorcerer"},'
                       '{"on":"start","do":"spawn","npc":"Lord Valkanith","hostile":true}]'}


def has(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone() is not None


def row(db, tbl, where, args):
    cur = db.execute(f'SELECT * FROM "{tbl}" WHERE {where}', args)
    r = cur.fetchone()
    return dict(zip([d[0] for d in cur.description], r)) if r else None


def js(v):
    return json.dumps(v, separators=(",", ":")) if v is not None else None


def rune_id(db):
    ids = [r[0] for r in db.execute("SELECT patternid FROM itempattern WHERE itemname = ?", (RUNE,))]
    return ids[0] if len(ids) == 1 else None


def place(db):
    r = row(db, "npcs", "id = ?", (GIVER_ROW,))
    return r["world"], round(r["x_coord"], 1), round(r["y_coord"], 1), round(r["z_coord"], 1)


def near(r, want):
    return r is not None and r["world"] == want[0] and abs(r["x_coord"] - want[1]) < 1 and abs(r["z_coord"] - want[2]) < 1


def check(db, t299):
    """What is wrong with the database for this tool, [] for nothing."""
    bad = []
    q = row(db, "quests", "questid = ?", (QUEST,))
    if q is None or q["giver"] != GIVER:
        bad.append(f"quests {QUEST}: {q and q['giver']!r}, want {GIVER!r}")
    else:
        for c, v in QUEST_OLD.items():
            if q[c] != v:
                bad.append(f"quests {QUEST} {c}: not the importer's")
    steps = [r[0] for r in db.execute("SELECT step FROM queststeps WHERE questid = ? ORDER BY step", (QUEST,))]
    if steps != [1, 2, 3, 4, 5, 6]:
        bad.append(f"queststeps {QUEST}: steps {steps}, want 1-6")
    else:
        for step, want in ((4, STEP4_OLD), (5, STEP5_OLD), (6, STEP6_OLD)):
            r = row(db, "queststeps", "questid = ? AND step = ?", (QUEST, step))
            for c, v in want.items():
                if r[c] != v:
                    bad.append(f"queststeps {QUEST} step {step} {c}: {r[c]!r}, want {v!r}")
        r = row(db, "queststeps", "questid = ? AND step = 4", (QUEST,))
        if r["kind"] != "give" or r["target"] != GIVER:
            bad.append(f"queststeps {QUEST} step 4: {r['kind']} {r['target']!r}, want give {GIVER!r}")
    n = db.execute("SELECT count(*) FROM charquests WHERE questid = ? AND done = 0 AND step IN (5, 6)", (QUEST,)).fetchone()[0]
    if n:
        bad.append(f"{n} characters stand on step 5 or 6 of the quest")
    givers = sorted(r[0] for r in db.execute("SELECT id FROM npcs WHERE npc_name = ?", (GIVER,)))
    if givers != sorted((GIVER_ROW, GIVER_DUP)):
        bad.append(f"npcs {GIVER}: rows {givers}, want {GIVER_DUP} and {GIVER_ROW}")
    elif not near(row(db, "npcs", "id = ?", (GIVER_ROW,)), GIVER_ROW_PLACE) or \
            not near(row(db, "npcs", "id = ?", (GIVER_DUP,)), GIVER_DUP_PLACE):
        bad.append(f"npcs {GIVER}: not where this tool expects them")
    for rid, (name, old, _) in LEVELS.items():
        r = row(db, "npcs", "id = ?", (rid,))
        if r is None or r["npc_name"] != name or r["npc_level"] != old:
            bad.append(f"npcs {rid}: {r and (r['npc_name'], r['npc_level'])}, want {name} at {old}")
    for name in WAVE:
        if db.execute("SELECT count(*) FROM npcs WHERE npc_name = ?", (name,)).fetchone()[0] != 2:
            bad.append(f"not two npcs rows named {name}")
        if not row(db, "npcspawnonly", "name = ?", (name,)):
            bad.append(f"{name} is not spawn-only")
    r = row(db, "npcs", "id = ?", (LORD_ROW,))
    if r is None or r["npc_name"] != "Lord Valkanith" or r["npc_level"] != 60:
        bad.append(f"npcs {LORD_ROW}: {r and (r['npc_name'], r['npc_level'])}, want Lord Valkanith at 60")
    if db.execute("SELECT count(*) FROM npcs WHERE lower(trim(npc_name)) = ?", (LORD,)).fetchone()[0] != 1:
        bad.append("not exactly one npcs row named Lord Valkanith")
    if not row(db, "npcspawnonly", "name = ?", ("Lord Valkanith",)):
        bad.append("Lord Valkanith is not spawn-only")
    nt = row(db, "npctemplate", "name = ?", (LORD,))
    if nt is None or nt["template"] != TEMPLATE_FROM:
        bad.append(f"npctemplate {LORD}: {nt}, want the template {TEMPLATE_FROM}")
    if row(db, "classtemplates", "name = ?", (LORD,)):
        bad.append(f"classtemplates {LORD} is there already")
    if not row(db, "classtemplates", "name = ?", (TEMPLATE_FROM,)):
        bad.append(f"no classtemplates {TEMPLATE_FROM}")
    for name in UNDEAD:
        u = row(db, "npcundead", "name = ?", (name,))
        if u is not None and not (name == LORD and u["undead"] == 1):
            bad.append(f"npcundead {name}: {u}")
    rid = rune_id(db)
    if rid is None:
        bad.append(f"not exactly one item {RUNE!r}")
    else:
        r = row(db, "itempattern", "patternid = ?", (rid,))
        if r["maxstack"] != 1 or r["levelreq"] != 55:
            bad.append(f"itempattern {rid}: maxstack {r['maxstack']} level {r['levelreq']}, want 1 and 55")
        if row(db, "itemeffect", "patternid = ?", (rid,)):
            bad.append(f"itemeffect {rid} is there already")
    if t299:
        for pid, name in T299.items():
            r = row(db, "itempattern", "patternid = ?", (pid,))
            if r is None or r["itemname"] != name:
                bad.append(f"itempattern {pid}: {r and r['itemname']!r}, want {name!r}")
            elif pid == SHIELD and r["classuse"] not in (32766, 32767):
                bad.append(f"itempattern {SHIELD} classuse {r['classuse']}")
    if db.execute("SELECT 1 FROM loottables WHERE tablename = ?", (LOOT_TABLE,)).fetchone() or \
            db.execute("SELECT 1 FROM npcloot WHERE npc_name = ?", (LORD,)).fetchone():
        bad.append(f"loot of {LORD} is there already")
    if row(db, "spellpattern", "spellid = ?", (TAP,)) or row(db, "spelleffect", "spellid = ?", (TAP,)):
        bad.append(f"spell {TAP} is taken")
    for sid in (TAP_FROM, TAP_FX):
        if row(db, "spellpattern", "spellid = ?", (sid,)) is None or row(db, "spelleffect", "spellid = ?", (sid,)) is None:
            bad.append(f"no spell {sid}")
    if row(db, "gmtriggers", "name = ?", (TRIGGER,)):
        bad.append(f"a trigger {TRIGGER!r} is there already")
    return bad


def report(db):
    q = row(db, "quests", "questid = ?", (QUEST,))
    print(f"  quest {QUEST}: {q and q['name']!r} by {q and q['giver']!r}, repeatable {q and q['repeatable']}")
    for r in db.execute("SELECT step, kind, target, items, count, events FROM queststeps WHERE questid = ? ORDER BY step",
                        (QUEST,)):
        print(f"    step {r[0]} {r[1]} {r[2]!r} items {r[3]!r} count {r[4]} events {r[5]!r}")
    print(f"  givers: {[r for r in db.execute('SELECT id, round(x_coord), round(z_coord) FROM npcs WHERE npc_name = ?', (GIVER,))]}")
    nt = row(db, "npctemplate", "name = ?", (LORD,))
    t = row(db, "classtemplates", "name = ?", (nt["template"],)) if nt else None
    print(f"  Lord Valkanith: template {nt and nt['template']!r} hp_pct {t and t['hp_pct']} abilities {t and t['abilities']}")
    print(f"  his loot: {[r for r in db.execute('SELECT tablename, chance, rolls FROM npcloot WHERE npc_name = ?', (LORD,))]}")
    print(f"  waves: {[r for r in db.execute('SELECT id, npc_name, npc_level FROM npcs WHERE id BETWEEN 123016 AND 123021')]}")


def next_slot(db):
    return (db.execute("SELECT max(slot) FROM gmtriggers").fetchone()[0] or 8) + 1


def apply(db, t299):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    db.execute(f"CREATE TABLE {LOG} (seq INTEGER PRIMARY KEY, tbl TEXT NOT NULL, rowkey TEXT NOT NULL, before TEXT, after TEXT)")

    def done(tbl, key, before):
        where = " AND ".join(f"{c} = ?" for c in key)
        after = row(db, tbl, where, tuple(key.values()))
        action = "update" if before and after else "insert" if after else "delete"
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES (?, ?, ?, ?)", (tbl, js(key), js(before), js(after)))
        db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                   "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                   (at, acct[0] if acct else 0, WHO, VIA, action, tbl, js(key), js(before), js(after), DETAIL))

    def insert(tbl, values, key):
        db.execute(f'INSERT INTO "{tbl}" ({", ".join(values)}) VALUES ({", ".join("?" * len(values))})', list(values.values()))
        done(tbl, key, None)

    def update(tbl, key, values):
        where = " AND ".join(f"{c} = ?" for c in key)
        before = row(db, tbl, where, tuple(key.values()))
        db.execute(f'UPDATE "{tbl}" SET {", ".join(f"{c} = ?" for c in values)} WHERE {where}',
                   list(values.values()) + list(key.values()))
        done(tbl, key, before)

    def delete(tbl, key):
        where = " AND ".join(f"{c} = ?" for c in key)
        before = row(db, tbl, where, tuple(key.values()))
        if before is None:
            return
        db.execute(f'DELETE FROM "{tbl}" WHERE {where}', tuple(key.values()))
        done(tbl, key, before)

    # the quest
    update("quests", {"questid": QUEST}, QUEST_NEW)
    update("queststeps", {"questid": QUEST, "step": 4}, STEP4_NEW)
    update("queststeps", {"questid": QUEST, "step": 5}, STEP5_NEW)
    delete("queststeps", {"questid": QUEST, "step": 6})

    # the giver: the island's row stays, the seeder's mainland copy goes
    delete("npcrespawn", {"npc_row": GIVER_DUP})
    delete("npcs", {"id": GIVER_DUP})

    # the waves' levels
    for rid, (_, _, new) in LEVELS.items():
        update("npcs", {"id": rid}, {"npc_level": new})

    # undead
    for name in UNDEAD:
        if not row(db, "npcundead", "name = ?", (name,)):
            insert("npcundead", {"name": name, "undead": 1, "note": f"{POST}: the Tal'thex undead; tools/valkanith_20261117.py",
                                 "source": SRC}, {"name": name})

    # his bloodtap
    p = row(db, "spellpattern", "spellid = ?", (TAP_FROM,))
    insert("spellpattern", dict(p, spellid=TAP, spellname=TAP_NAME, spelldesc=TAP_LINE), {"spellid": TAP})
    fx = row(db, "spelleffect", "spellid = ?", (TAP_FX,))["fxdict"]
    insert("spelleffect", {"spellid": TAP, "line": TAP_LINE, "fxdict": fx,
                           "source": f"Lord Valkanith's AoE bloodtap, {POST}, work board 737: {SRC}",
                           "effects": js(TAP_EFFECTS)}, {"spellid": TAP})

    # his template
    t = row(db, "classtemplates", "name = ?", (TEMPLATE_FROM,))
    insert("classtemplates", dict(t, name=LORD, abilities=js(ABILITIES), source=SRC,
                                  note=f"'{TEMPLATE_FROM}' as it was on 2026-11-17. '{TEMPLATE_FROM}''s note: " + t["note"] +
                                  TEMPLATE_WORDS, **TEMPLATE), {"name": LORD})
    update("npctemplate", {"name": LORD}, {"template": LORD, "note": "named, level 60: his own (tools/valkanith_20261117.py)",
                                           "source": SRC})

    # his loot
    items = [rune_id(db)] + ([PHYLACTERY, SHIELD] if t299 else [])
    for pid in items:
        db.execute("INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel, source) VALUES (?, ?, 1, 1, 70, ?)",
                   (LOOT_TABLE, pid, SRC))
        done("loottables", {"id": db.execute("SELECT last_insert_rowid()").fetchone()[0]}, None)
    insert("npcloot", {"npc_name": LORD, "tablename": LOOT_TABLE, "chance": LOOT_CHANCE, "rolls": LOOT_ROLLS,
                       "note": f"{POST}: 'can drop' the Bone Wing Sentinel" + ("; work board 737's t299 records" if t299 else ""),
                       "source": SRC}, {"npc_name": LORD, "tablename": LOOT_TABLE})
    if t299 and row(db, "itempattern", "patternid = ?", (SHIELD,))["classuse"] == 32766:
        update("itempattern", {"patternid": SHIELD}, {"classuse": 32767})

    # the rune's use
    insert("itemeffect", {"patternid": rune_id(db), "line": RUNE_LINE, "name": RUNE_EFFECT, "icon": 0, "breaks": "",
                          "wear": 0, "area": 0, "source": SRC, "body": 0, "target_line": "", "charges": RUNE_CHARGES,
                          "cooldown": 0}, {"patternid": rune_id(db)})

    # the trigger step 4 fires
    world, x, y, z = place(db)
    slot = next_slot(db)
    insert("gmtriggers", {"slot": slot, "name": TRIGGER, "enabled": 1, "scope": "near", "radius": 400, "world": world,
                          "x": x, "y": y, "z": z, "steps": js(trigger_steps(world, x, y, z)),
                          "notes": "Fired by the Lord Valkanith quest's gems handed in (101269 step 4) at the Spirit of "
                                   "Tellainain's island: seven waves of Tal'thex four minutes apart, a dying sorcerer "
                                   "calls Lord Valkanith, who stays 30 minutes; one at a time; ten minutes with no one "
                                   "near end it. " + POST + ", work board 737: tools/valkanith_20261117.py"},
           {"slot": slot})


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


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    ap.add_argument("--t299", action="store_true", help="his table also holds entry 737's t299 records that exist "
                                                        "(phylactery of valkanith 7982, positronian energy shield 6067)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
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
        bad = check(db, a.t299)
        if bad:
            print("refused, nothing done:\n  " + "\n  ".join(bad))
            return 1
        if not a.apply:
            print("report only: --apply to write it")
            return 0
        db.execute("BEGIN IMMEDIATE")
        apply(db, a.t299)
        db.execute("COMMIT")
        print("applied; takes effect at a restart (the quests are read at a start)")
        report(db)
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
