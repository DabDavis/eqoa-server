#!/usr/bin/env python3
"""The Kratas quest and Kratas made as the owner's source has them: Ronson's walkthrough on the EQOA forum
(eqoa.forumotion.com/t169-kratas-quest, 2010-07-26, the same text as EQOA-Data's Misc/kratas guide), sent by the owner
on 2026-11-16 ("its drops and rewards count as owner-approved"); QuestMasters' pages of 2006 (external/questmasters:
id434 the quest, id257 the tactics, id232/id163/id650 the Plane of Sky's use of the harpoon) where Ronson says nothing.

WHAT DIFFERED (quest 101265 "Kratas", giver Julaech; Kratas npcs 123007):
  - Step 1 looted "Ebony Recluse Silk" (701372), an item of our importer's. The walkthrough: "First, you need an Ebony
    Recluse ... Work his health down and loot it"; QuestMasters (id434): "Kill Ebony Recluse (Kelinar) to get jar". The
    item is the disc era's own record 1186 "ebony recluse" ("This jar contains an ebony recluse."), on both shards.
  - Steps 5 and 6 (kill Kratas, talk back to Julaech) were ours. The walkthrough: "Wander near the whale bones
    underwater near Old Man Antiok. Get close enough, your quest will be completed, and Kratas will spawn." Here the
    reach step at the bones is the last, and its doing raises Kratas (the trigger below).
  - Kratas stood at the bones for anyone (FrontiersForge's sighting placed him): the walkthrough's Kratas is raised
    by the quest alone. Now spawn-only (npcspawnonly "kratas"): the trigger "Kratas Rises", fired by the reach step,
    raises him there, one at a time (its lock; the step waits while another's Kratas lives: the server's code of
    branch kratas, handlers/quests.go checkReach).
  - Kratas fought as a level 53 roamer (hp_pct 100: about 11,700 hit points, a crit, no ability). The walkthrough:
    "a major mob with insane HPs", "a massive poison AoE/DoT", "Kratas has a healing DS", "main tank do not hit X, just
    taunt", the group hidden "behind the whale bones"; QuestMasters (id257): "a TON of HP", "a pretty wicked damage
    shield", "the un-resistible Area Effect DoT", casters "just outside the range of the AE", "you can use your special
    melee abilities on Kratas without incurring any damage". Now:
      level 57 (the raid list's peers: Siliskor 55, Lady Vox 57; his harpoon a level 53 item, id232) and hp_pct 1530,
        about 200,000 hit points (the ebony recluse's 980 at 50 is 100,000: "killing him will take an eternity");
      melee 250%; the roamer's Critical Hit kept (id232: "he will repeatedly CH");
      Abyssal Venom (new spell 760351): a poison burst round himself, 2,000 PR, radius 25, every 18 s from 6 s;
      Ink of the Deep: a sethp moment that sets no one's hit points (set_hp 1,000,000) and carries the damage over
        time, 500 a tick for 24 s, 300 at the PR cap ("un-resistible" mostly), radius 25, every 24 s from 3 s;
        both reach only those in his sight (line of sight: the bones hide), and a nuke's 30 reaches from outside
        the 25;
      a healing damage shield: every hit that lands heals him 300 (hitheal), and each melee hit is answered with
        600 (shield_damage, resisted by PR); arts are spells and are not answered;
      his resists kept (PR, DR, AR at 50: id576).
  - Kratas had no loot of his own. The walkthrough's drops: the Harpoon of Telinan, the Shimmering Seaweed (four L47
    pieces, Master Craft Jeweller) and his Tentacle (the Natatorial Earring). The harpoon is the Plane of Sky's
    quest loot already (101298 step 6, chance 1.0, and dropped before the step: questpreloot.go), so every portal
    quester of the raid takes one each kill ("drops with every kill", id257): not doubled here. New: tables
    "boss kratas" (Shimmering Seaweed, 100% x 2) and "boss kratas tentacle" (Kratas Tentacle, 50% x 1); a "boss "
    table makes him a raid boss (need before greed).
  - None of the jewelry existed. New items (L47, 9,400 durability as the jewelers' 200 a level, slot icons of the
    adamantite pieces): Seaweed Ring (AGI 30, HP 50, POW 75), Seaweed Bracelet (STA 30, HP 75, POW 50), Seaweed
    Necklace (STR 30, HP 75, POW 50; a pendant), Seaweed Earring (DEX 30, HP 75, POW 50), Natatorial Earring (INT 30,
    HP 50, POW 75); five Jewelcrafting recipes under Special at trade level 9 (Master): the seaweed or the tentacle
    (kept on a failure, as a gem is), the piece's mold and an adamantite bar (ours: the walkthrough names no metal),
    found by experiment.
  - The bream's Hazinak box (fishcatch 32: x 24000-26000) stopped 490 short of EQOA-Data's "oceanic watering hole"
    mark (its map's 13901, 11195: about 26,490, 27,290 here): its east edge moved to 27,000. Odus (row 33) is whole.
  - Julaech's lines name the jar, the Box Canyons and the warning ("DO NOT TRY TO SPAWN KRATAS UNTIL YOUR GUILD IS
    RAID READY").
  Left as they were: Julaech standing (no sometimes-spawn), the ants, the recluse, the lure (701373), 701372 (no one
    holds it; no step names it now).

Report only by default; --apply in one transaction with gm_audit lines and the log kratas_1116_log (every row before
and after); a second --apply does nothing; --undo puts back exactly what it wrote (a row changed since is left and
said) and drops the log. Refused whole if a row is not as this tool expects.

WHEN IT TAKES EFFECT: quests at a start (a keep-restart's before.sh); templates, npctemplate, npcspawnonly and the
spells at the GM panel's reload_npcs or a start; loot, recipes, items, fishcatch and the trigger at a start (the
recipes and items too). Run it in the before.sh, the server stopped.

    python3 -E -s tools/kratas_20261116.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "kratas_1116_log"
WHO, VIA = "dab", "tool"
SRC = "kratas1116"
DETAIL = ("the Kratas quest and Kratas as Ronson's walkthrough (the owner's source, 2026-11-16): "
          "tools/kratas_20261116.py")
WALK = "Ronson's walkthrough (eqoa.forumotion.com/t169, the owner's source 2026-11-16)"

QUEST = 101265
KRATAS_ROW = 123007
KRATAS = "kratas"  # NPCClassKey
LEVEL_OLD, LEVEL_NEW = 53, 57

# the records the quest uses, the same ids on both shards (checked by name)
JAR, FRUIT, BREAM, LURE, SILK = 1186, 1182, 700703, 701373, 701372
MOLDS = {"ring": 27506, "earring": 27507, "pendant": 27508, "bracelet": 27509}
BAR = 27579  # Adamantite Bar
KNOWN = {JAR: "ebony recluse", FRUIT: "Ambroise Fruit", BREAM: "Longfellow bream", LURE: "Kratas Lure",
         SILK: "Ebony Recluse Silk", 27506: "Ring Mold", 27507: "Earring Mold", 27508: "Pendant Mold",
         27509: "Bracelet Mold", BAR: "Adamantite Bar"}
# slot icons and equip slots: the adamantite wolf's eye agate pieces' (705112-705115)
SLOT = {"ring": (-1505803670, 8), "earring": (-520458434, 3), "pendant": (756343796, 4), "bracelet": (-1977361665, 6)}

# new items: id -> (name, desc, kind, stats); kind "comp" a component, else the slot
SEAWEED, TENTACLE = 985001, 985002
ITEMS = {
    SEAWEED: ("Shimmering Seaweed", "Seaweed from the deep that shimmers in the light. A master jeweler can work it.",
              "comp", {}, 806013905),
    TENTACLE: ("Kratas Tentacle", "A tentacle of the sea beast Kratas. A master jeweler can work it.",
               "comp", {}, 681434711),
    985003: ("Seaweed Ring", "A ring of shimmering seaweed.", "ring", {"agi": 30, "HPMAX": 50, "POWMAX": 75}, None),
    985004: ("Seaweed Bracelet", "A bracelet of shimmering seaweed.", "bracelet", {"sta": 30, "HPMAX": 75, "POWMAX": 50}, None),
    985005: ("Seaweed Necklace", "A necklace of shimmering seaweed.", "pendant", {"str": 30, "HPMAX": 75, "POWMAX": 50}, None),
    985006: ("Seaweed Earring", "An earring of shimmering seaweed.", "earring", {"dex": 30, "HPMAX": 75, "POWMAX": 50}, None),
    985007: ("Natatorial Earring", "An earring worked from a tentacle of Kratas.", "earring",
             {"intelligence": 30, "HPMAX": 50, "POWMAX": 75}, None),
}
PIECE_LEVEL = 47
STATS = ("str", "sta", "agi", "wis", "dex", "cha", "intelligence", "HPMAX", "POWMAX", "PoT", "HoT", "AC",
         "PR", "DR", "FR", "CR", "LR", "AR")

# recipes: id -> (product, component)
RECIPES = {8001: (985003, SEAWEED), 8002: (985004, SEAWEED), 8003: (985005, SEAWEED), 8004: (985006, SEAWEED),
           8005: (985007, TENTACLE)}
JEWEL, SPECIAL, MASTER = 3, 10, 9  # skill, the Special tab, trade level 9 (the Master Jewelers Kit's)

LOOT = {  # table: (item, chance, rolls)
    "boss kratas": (SEAWEED, 100, 2),
    "boss kratas tentacle": (TENTACLE, 50, 1),
}

VENOM = 760351
VENOM_FROM = 905140  # Frost Breath: a self-centred area (Lady Vox's)
VENOM_FX = 905023    # Pestilent Breath's look
VENOM_DMG, VENOM_RADIUS = 2000, 25

ABILITIES = [
    {"kind": "aoe", "spell_id": VENOM, "every": 18, "first": 6},
    {"kind": "sethp", "name": "Ink of the Deep", "set_hp": 1000000, "radius": VENOM_RADIUS, "every": 24, "first": 3,
     "tick": 500, "tick_at_cap": 300, "tick_for": 24, "resist": "PR"},
    {"kind": "hitheal", "heal": 300},
]
TEMPLATE_OLD = {"hp_pct": 100, "melee_pct": 100, "abilities": [], "shield_damage": 0, "shield_resist": ""}
TEMPLATE_NEW = {"hp_pct": 1530, "melee_pct": 250, "abilities": ABILITIES, "shield_damage": 600, "shield_resist": "PR"}
TEMPLATE_WORDS = (" (2026-11-16, tools/kratas_20261116.py, " + WALK + ": 'insane HPs', 'a massive poison AoE/DoT', "
                  "'a healing DS', hide behind the whale bones; QuestMasters id257: 'a wicked damage shield', the AE DoT "
                  "outside its range: level 57, hp_pct 1530 (~200k), melee 250%, Abyssal Venom 2,000 PR radius 25 every "
                  "18 s, Ink of the Deep 500 a tick (300 at the PR cap) for 24 s radius 25 every 24 s, a 300 heal at "
                  "each hit landed, a 600 PR damage shield on melee; sizes ours)")

TRIGGER = "Kratas Rises"
BUSY = "Kratas already stirs among the bones. Come back when he is gone."

# the quest's rows: (column, old, new)
QUEST_EDITS = [
    ("offer",
     "A sea monster, Kratas, ate my crew. I want revenge, but first he must be lured out. Bring me silk from an ebony "
     "recluse in the hills near Kelinar, an ambroise fruit from a calescent ant, and a longfellow bream.",
     "A sea monster, Kratas, ate my crew. I want revenge, but first he must be lured out. Bring me an ebony recluse "
     "from the hills near Kelinar, an ambroise fruit from a calescent ant of the Box Canyons, and a longfellow bream "
     "from the open sea."),
    ("notes", None,
     "From " + WALK + " (= EQOA-Data Quests/Misc/kratas.json, # Kratas Quest, github.com/Jadiction/EQOA-Data, MIT): "
     "the quest ends at the whale bones, and its doing raises Kratas (trigger 'Kratas Rises', one at a time; he is "
     "spawn-only). The recluse's jar is the disc's record 1186 (QuestMasters id434: 'to get jar'). Kratas's drops are "
     "loot: Shimmering Seaweed and his Tentacle (boss kratas tables), the Harpoon of Telinan the Plane of Sky's quest "
     "loot (101298). Julaech stands always (the guide's is a sometimes spawn). tools/kratas_20261116.py."),
]
STEP_EDITS = {  # step: {column: (old, new)}
    1: {"items": (str(SILK), str(JAR)),
        "journal": ("Take silk from the ebony recluse in the hills near Kelinar.",
                    "Catch the ebony recluse in the hills near Kelinar: wear him down and loot him in his jar.")},
    2: {"journal": ("Take an ambroise fruit from a calescent ant in Box Canyon.",
                    "Take an ambroise fruit from a calescent ant in the Box Canyons (many in the valley of the dragon "
                    "Skayhir).")},
    3: {"items": (f"{SILK}, {FRUIT}, {BREAM}", f"{JAR}, {FRUIT}, {BREAM}"),
        "journal": ("Fish a longfellow bream from an ocean watering hole, and bring it all to Julaech.",
                    "Fish a longfellow bream from an oceanic watering hole (Hazinak, or Odus) with a good pole and "
                    "bait, and bring it all to Julaech."),
        "says": ("Strange bait, but he loves it. Here, all bundled. Wander near the whale bones by Old Man Antiok, "
                 "under the water.",
                 "Strange bait, but he loves it. Here, all bundled. Wander near the whale bones by Old Man Antiok, "
                 "under the water, and he will come. Do not wake him until your guild is ready to fight him.")},
    4: {"journal": ("Swim to the whale bones under the water near Old Man Antiok.",
                    "Swim to the whale bones under the water near Old Man Antiok, your guild with you: Kratas comes."),
        "events": ('[{"on":"done","do":"popup","text":"Something huge stirs among the bones. Kratas!"}]',
                   json.dumps([{"on": "done", "do": "trigger", "trigger": TRIGGER, "text": BUSY},
                               {"on": "done", "do": "popup", "text": "Something huge stirs among the bones. Kratas!"}],
                              separators=(",", ":")))},
}
DROP_STEPS = (5, 6)
STEP5 = {"kind": "kill", "target": "Kratas", "events": '[{"on":"start","do":"spawn","npc":"Kratas","hostile":true}]'}
STEP6 = {"kind": "talk", "target": "Julaech", "says": "My crew is avenged. Thank you."}

FISH_ROW = 32
FISH_OLD = (700703, 0, 24000.0, 26000.0, 26000.0, 28000.0)  # itemid, world, minx, minz, maxx, maxz
FISH_MAXX = 27000.0


def has(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone() is not None


def row(db, tbl, where, args):
    cur = db.execute(f'SELECT * FROM "{tbl}" WHERE {where}', args)
    r = cur.fetchone()
    return dict(zip([d[0] for d in cur.description], r)) if r else None


def js(v):
    return json.dumps(v, separators=(",", ":")) if v is not None else None


def kratas_place(db):
    r = row(db, "npcs", "id = ?", (KRATAS_ROW,))
    return r["world"], round(r["x_coord"], 1), round(r["y_coord"], 1), round(r["z_coord"], 1)


def trigger_steps(db):
    world, x, y, z = kratas_place(db)
    return [
        {"do": "lock", "lock": "kratas"},
        {"do": "clock", "seconds": 3600, "empty": 600},
        {"do": "spawn", "npc": "Kratas", "hostile": True, "minutes": 60, "spread": 1, "world": world, "x": x, "y": y, "z": z},
        {"do": "broadcast", "text": "Kratas rises from among the whale bones!"},
        {"do": "awaitdead", "npc": "Kratas"},
        {"do": "end"},
        {"do": "despawn"},
    ]


def check(db):
    """What is wrong with the database for this tool, [] for nothing."""
    bad = []
    q = row(db, "quests", "questid = ?", (QUEST,))
    if q is None or q["name"] != "Kratas" or q["giver"] != "Julaech":
        bad.append(f"quests {QUEST}: {q and (q['name'], q['giver'])}, want Kratas by Julaech")
    elif q["offer"] != QUEST_EDITS[0][1]:
        bad.append(f"quests {QUEST}: its offer is not the importer's")
    steps = {r[0]: r for r in db.execute("SELECT step FROM queststeps WHERE questid = ?", (QUEST,))}
    if sorted(steps) != [1, 2, 3, 4, 5, 6]:
        bad.append(f"queststeps {QUEST}: steps {sorted(steps)}, want 1-6")
    else:
        for step, cols in STEP_EDITS.items():
            r = row(db, "queststeps", "questid = ? AND step = ?", (QUEST, step))
            for c, (old, _) in cols.items():
                if r[c] != old:
                    bad.append(f"queststeps {QUEST} step {step} {c}: {r[c]!r}, want {old!r}")
        for step, want in ((5, STEP5), (6, STEP6)):
            r = row(db, "queststeps", "questid = ? AND step = ?", (QUEST, step))
            for c, v in want.items():
                if r[c] != v:
                    bad.append(f"queststeps {QUEST} step {step} {c}: {r[c]!r}, want {v!r}")
    n = db.execute("SELECT count(*) FROM charquests WHERE questid = ? AND done = 0 AND step IN (5, 6)", (QUEST,)).fetchone()[0]
    if n:
        bad.append(f"{n} characters stand on step 5 or 6 of the quest")
    k = row(db, "npcs", "id = ?", (KRATAS_ROW,))
    if k is None or k["npc_name"] != "Kratas" or k["npc_level"] != LEVEL_OLD:
        bad.append(f"npcs {KRATAS_ROW}: {k and (k['npc_name'], k['npc_level'])}, want Kratas at {LEVEL_OLD}")
    if db.execute("SELECT count(*) FROM npcs WHERE lower(trim(npc_name)) = ?", (KRATAS,)).fetchone()[0] != 1:
        bad.append("not exactly one npcs row named Kratas")
    if row(db, "npcspawnonly", "name = ?", (KRATAS,)):
        bad.append("npcspawnonly kratas is there already")
    nt = row(db, "npctemplate", "name = ?", (KRATAS,))
    if nt is None or nt["template"] != KRATAS:
        bad.append(f"npctemplate kratas: {nt}, want the template kratas")
    t = row(db, "classtemplates", "name = ?", (KRATAS,))
    if t is None:
        bad.append("no classtemplates kratas")
    else:
        now = {c: (json.loads(t[c]) if c == "abilities" else t[c]) for c in TEMPLATE_OLD}
        if now != TEMPLATE_OLD:
            bad.append(f"classtemplates kratas: {now}, want {TEMPLATE_OLD}")
    for pid, name in KNOWN.items():
        r = row(db, "itempattern", "patternid = ?", (pid,))
        if r is None or r["itemname"] != name:
            bad.append(f"itempattern {pid}: {r and r['itemname']!r}, want {name!r}")
    for pid, (name, *_rest) in ITEMS.items():
        if row(db, "itempattern", "patternid = ?", (pid,)):
            bad.append(f"itempattern {pid} is taken")
        if db.execute("SELECT 1 FROM itempattern WHERE lower(itemname) = lower(?)", (name,)).fetchone():
            bad.append(f"an item {name!r} is there already")
    for rid in RECIPES:
        if row(db, "craftrecipes", "id = ?", (rid,)):
            bad.append(f"craftrecipes {rid} is taken")
    for tbl in LOOT:
        if db.execute("SELECT 1 FROM loottables WHERE tablename = ?", (tbl,)).fetchone() or row(
                db, "npcloot", "npc_name = ? AND tablename = ?", (KRATAS, tbl)):
            bad.append(f"loot table {tbl!r} is there already")
    for sid in (VENOM,):
        if row(db, "spellpattern", "spellid = ?", (sid,)) or row(db, "spelleffect", "spellid = ?", (sid,)):
            bad.append(f"spell {sid} is taken")
    for sid in (VENOM_FROM, VENOM_FX):
        if row(db, "spellpattern", "spellid = ?", (sid,)) is None or row(db, "spelleffect", "spellid = ?", (sid,)) is None:
            bad.append(f"no spell {sid}")
    if row(db, "gmtriggers", "name = ?", (TRIGGER,)):
        bad.append(f"a trigger {TRIGGER!r} is there already")
    f = row(db, "fishcatch", "id = ?", (FISH_ROW,))
    if f is None or (f["itemid"], f["world"], f["minx"], f["minz"], f["maxx"], f["maxz"]) != FISH_OLD:
        bad.append(f"fishcatch {FISH_ROW}: {f}, want the bream's Hazinak box {FISH_OLD}")
    return bad


def report(db):
    q = row(db, "quests", "questid = ?", (QUEST,))
    print(f"  quest {QUEST}: {q and q['name']!r} by {q and q['giver']!r}")
    for r in db.execute("SELECT step, kind, target, items, events FROM queststeps WHERE questid = ? ORDER BY step", (QUEST,)):
        print(f"    step {r[0]} {r[1]} {r[2]!r} items {r[3]!r} events {r[4]!r}")
    k = row(db, "npcs", "id = ?", (KRATAS_ROW,))
    t = row(db, "classtemplates", "name = ?", (KRATAS,))
    print(f"  Kratas: npcs {KRATAS_ROW} level {k and k['npc_level']}, spawn-only "
          f"{bool(row(db, 'npcspawnonly', 'name = ?', (KRATAS,)))}, template hp_pct {t and t['hp_pct']} "
          f"abilities {t and t['abilities']}")
    print(f"  his loot: {[r[0] for r in db.execute('SELECT tablename FROM npcloot WHERE npc_name = ?', (KRATAS,))]}")


def next_slot(db):
    return (db.execute("SELECT max(slot) FROM gmtriggers").fetchone()[0] or 8) + 1


def apply(db):
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

    # the quest
    update("quests", {"questid": QUEST}, {c: new for c, _, new in QUEST_EDITS})
    for step, cols in STEP_EDITS.items():
        update("queststeps", {"questid": QUEST, "step": step}, {c: new for c, (_, new) in cols.items()})
    for step in DROP_STEPS:
        key = {"questid": QUEST, "step": step}
        before = row(db, "queststeps", "questid = ? AND step = ?", (QUEST, step))
        db.execute("DELETE FROM queststeps WHERE questid = ? AND step = ?", (QUEST, step))
        done("queststeps", key, before)

    # Kratas: spawn-only, his level, his template
    insert("npcspawnonly", {"name": KRATAS, "note": "raised by the Kratas quest's last step at the whale bones (trigger "
                            "'Kratas Rises'), " + WALK + ": tools/kratas_20261116.py", "source": SRC}, {"name": KRATAS})
    update("npcs", {"id": KRATAS_ROW}, {"npc_level": LEVEL_NEW})
    t = row(db, "classtemplates", "name = ?", (KRATAS,))
    update("classtemplates", {"name": KRATAS}, dict(TEMPLATE_NEW, abilities=js(ABILITIES), note=t["note"] + TEMPLATE_WORDS,
                                                    source=SRC))

    # his poison burst
    p = row(db, "spellpattern", "spellid = ?", (VENOM_FROM,))
    line = f"Self, aoe Decrease Hitpoints by {VENOM_DMG}, Radius {VENOM_RADIUS}"
    insert("spellpattern", dict(p, spellid=VENOM, spellname="Abyssal Venom", spelldesc=line), {"spellid": VENOM})
    eff = json.loads(row(db, "spelleffect", "spellid = ?", (VENOM_FROM,))["effects"])
    eff["radius"] = VENOM_RADIUS
    eff["effects"] = [{"type": "Dmg/Heal", "kind": "hp2", "amount": -VENOM_DMG, "resist": "pr"}]
    fx = row(db, "spelleffect", "spellid = ?", (VENOM_FX,))["fxdict"]
    insert("spelleffect", {"spellid": VENOM, "line": line, "fxdict": fx, "source": f"Kratas's poison AoE, {WALK}: {SRC}",
                           "effects": js(eff)}, {"spellid": VENOM})

    # the items
    cols = [r[1] for r in db.execute("PRAGMA table_info(itempattern)")]
    comp_like = row(db, "itempattern", "patternid = ?", (704196,)) or row(db, "itempattern", "patternid = ?", (BAR,))
    for pid, (name, desc, kind, stats, icon) in ITEMS.items():
        if kind == "comp":
            r = {c: comp_like[c] for c in cols}
            r.update(patternid=pid, itemname=name, itemdesc=desc, itemicon=icon, itemcost=200, maxstack=50,
                     levelreq=0, lore=0, trade=0, rent=1, craft=1, equipslot=-1)
        else:
            sicon, slot = SLOT[kind]
            r = {c: None for c in cols}
            r.update(patternid=pid, itemcost=2400, unk1=0, itemicon=sicon, unk2=0, equipslot=slot, unk3=0, trade=0, rent=1,
                     unk4=0, attacktype=0, weapondamage=0, unk5=0, levelreq=PIECE_LEVEL, maxstack=1,
                     maxhp=200 * PIECE_LEVEL, duration=70, classuse=32767, raceuse=32767, procanim=0, lore=0, unk6=0,
                     craft=0, itemname=name, itemdesc=desc, model=0, color=0)
            r.update(stats)
        insert("itempattern", r, {"patternid": pid})

    # the recipes
    for rid, (product, comp) in RECIPES.items():
        name, _, kind, _, _ = ITEMS[product]
        ingredients = [{"item": comp, "count": 1, "keep": True}, {"item": MOLDS[kind], "count": 1, "keep": False},
                       {"item": BAR, "count": 1, "keep": False}]
        insert("craftrecipes", {"id": rid, "skill": JEWEL, "comp": SPECIAL, "level": MASTER, "rarity": 2, "name": name,
                                "product": product, "yield": 1, "ingredients": js(ingredients), "difficulty": 90, "xp": 225,
                                "discover": 1, "scroll": 0,
                                "description": f"{ITEMS[comp][0]}, a {kind} mold and an adamantite bar: a master jeweler's "
                                               "work.",
                                "source": SRC}, {"id": rid})

    # his loot
    for tbl, (item, chance, rolls) in LOOT.items():
        db.execute("INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel, source) VALUES (?, ?, 1, 1, 70, ?)",
                   (tbl, item, SRC))
        done("loottables", {"id": db.execute("SELECT last_insert_rowid()").fetchone()[0]}, None)
        insert("npcloot", {"npc_name": KRATAS, "tablename": tbl, "chance": chance, "rolls": rolls,
                           "note": f"{WALK}: his drops", "source": SRC}, {"npc_name": KRATAS, "tablename": tbl})

    # the bream's Hazinak box
    update("fishcatch", {"id": FISH_ROW}, {"maxx": FISH_MAXX})

    # the trigger the last step fires
    world, x, y, z = kratas_place(db)
    slot = next_slot(db)
    insert("gmtriggers", {"slot": slot, "name": TRIGGER, "enabled": 1, "scope": "near", "radius": 120, "world": world,
                          "x": x, "y": y, "z": z, "steps": js(trigger_steps(db)),
                          "notes": "Fired by the Kratas quest's last step (101265 step 4) at the whale bones by Old Man "
                                   "Antiok: Kratas raised there, one at a time, for an hour at most; ten minutes with no "
                                   "one by him end it. " + WALK + ": tools/kratas_20261116.py"}, {"slot": slot})


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
        bad = check(db)
        if bad:
            print("refused, nothing done:\n  " + "\n  ".join(bad))
            return 1
        if not a.apply:
            print("report only: --apply to write it")
            return 0
        db.execute("BEGIN IMMEDIATE")
        apply(db)
        db.execute("COMMIT")
        print("applied; takes effect at a restart (the quests are read at a start)")
        report(db)
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
