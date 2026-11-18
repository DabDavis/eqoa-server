#!/usr/bin/env python3
"""The Plane of Sky's Snow Chain as the owner's source has it: Ronson's walkthrough on the EQOA forum
(eqoa.forumotion.com/t212-plane-of-sky-snow-chain, "make it work as that post describes", the owner 2026-11-18; no replies
on the thread). By NAMES and places, never by row ids: the two shards' rows are read where they stand.

WHAT WAS THERE (quest 101307 "The Chain of Snow" by the Mark of the Majestic on both shards, with tools/
snow1_essence_20261114.py, sky_events_20261114.py and sky_events2_20261114.py): the snow key at the Mark (requires the
Night chain, 101306); the essence of snow on the main isle (skypass.go) to Snow 1; Ryini's walk down the mountain
(trigger "Snow 1: Ryini's Walk": her row hidden, a copy walks, the gargoyle pair, the Wind Mephit at the stone bridge,
the two popups, her death fails the run); the drop to the igloo village (step 4's teleport); Tamis, the Whirlwind
(Shard of Shivers, 20 Snow Key 2), Guenzi; the ice by the village (hazards "The ice", 6,000 every 10 s); Snow 3's
behemoth pair at the landing; the glacial champion (Shard of Glaciers, 20 Snow Key 3: the Snowfall essence); Snow 4's
drakes; the stew back at Guenzi and the teleport to Snow 4; Enraged Aeyinar at her rest (trigger "Snow 4: Enraged
Aeyinar"; Shard of Icy Depths, 20 Snow Key 4); the essence of the brood to the Isle of Wonder; the Mark again; the
ordinary Aeyinar every 8 hours; the Astral Light's three white shards (101433) to the Construct of Frost (trigger 104:
16 ported, a pet one, 5 minutes after its death).

WHAT DIFFERED FROM THE POST, AND WHAT THIS WRITES:
  - "If she dies, you fail, and must wait one hour for the npc to respawn": her row was hidden an hour from the
    walk's START, so she was back 30-60 minutes after a death. Now the walk's protect has minutes 60: hidden an hour
    from her death (branch snow-chain's code: protect's minutes).
  - A failed walk left the questor on "kill the Wind Mephit", which only the walk spawns: stuck for good. The server's
    encounter put-back (questencounter.go) did not see the walk's Mephit, spawned by its row key "#id"; with branch
    snow-chain's code it does (SpawnsNPC reads a row key as its row's name): the questor is put back to "speak with
    Ryini" and, her hour over, walks her again. Enraged Aeyinar's spawn is the same.
  - The raid: in the post the questor talks, the raid escorts, everyone goes on. Here only the one who talked was on
    the next step, and a member who talked during the walk was refused. The walk now does step 2 (speak with Ryini)
    for everyone near on it as it starts and step 3 (the Wind Mephit) for everyone near on it when the Mephit dies
    (queststep).
  - The herb: "herb (random drop off any mob)" was left out. Now a quest item "frostbloom herb" (a copy of the
    gargoyle eye's record) and a loot step before the way back to the village: off any creature of the four isles
    (icebound gargoyles, glacial behemoths, snow brutes, celestial drakes, the Whirlwind, the Wind Mephit, the glacial
    champion, Aeyinar), 15% a kill (ours), dropping ahead of its step too (questpreloot.go) as the post's "farm it on
    isle 1 prior". Guenzi's hand-in takes it with the heart, the foot and the eye. Steps 13-18 become 14-19
    (skypass.go's Snow Key 4 passage moves with them in the branch's code); a character on one of them is moved too.
  - "you only have 15 min in which to kill her": her spawn simply went at 15 minutes, nothing said, and the questor
    stood on "kill Enraged Aeyinar" for good. Now a fifteen-minute wait with a failure's words, and the questor is put
    back to the walk toward her rest (the encounter put-back above).
  - "If you are in the kill group, you will be flagged for Desolation Chain": only the questor, who alone brought the
    stew, could finish. Now her death sets everyone near who is on the chain past Ryini's walk (step 5 on) to the way
    back to the Isle of Wonder (questset, branch snow-chain's code), so the Mark flags them all. Not those short of
    step 5: the post's "bypass by rez" is not kept.
  - Enraged Aeyinar fought as a plain level 64 roamer (16,282 hit points, a 20% rampage). The post: "about five times
    the amount of hit points", "will also LH twice, not once like the normal one", "about 100% melee immune", "debuffs
    STA by 200 ... she'll also debuff agility", quads "about 3 times harder", "a 1k aoe". Now (OURS where the post gives
    no figure): hp_pct 500 (~81,000), melee_pct 250, melee of every kind 5% taken (spells and arts land), two Lay Hands
    (100% at 50% and at 25%, once a fight), Aeyinar's Frostbite every 45 s (a new spell: -200 STA, -100 AGI, 120 s,
    CR resisted), Frost Burst at 67% (~1,000 cold round her, radius 30) every 30 s. The ordinary Aeyinar ("Aeyinar has
    one heal", "a wicked AoE", the same debuff) had no template at all: now one of her own, the roamer's figures with
    one Lay Hands, the Frostbite and Frost Burst at 50%.
  - The Storyteller in Baga Village ("head to The Storyteller in Baga Village NE to obtain faction points"): no quest.
    Now "Tales of the Snow" (requires the chain): +250 Denizens of the Plane of Sky (ours: the Rain chain's figure).

LEFT AS IT IS: the Mark's requirement (the Night chain's quest: "in the kill group for King Rhutra"); the essence's key
by a talk (the post's popup on approach and its glitches are not kept); Ryini's template (she aggroes no one: the post's
heal aggro is not kept); the ice (7 patches, 6,000: no new damage zone); the ordinary Aeyinar's 8 hours and her spot
shared with the Enraged one; the shards, keys and the Construct of Frost; no loot added beyond the herb (a quest drop).

Report only by default; --apply in one transaction with gm_audit lines and the log snow_chain_1118_log (every row before
and after, a renumbered step by its old and new key); a second --apply does nothing; --applied says what it wrote;
--undo puts back exactly what it wrote (a row changed since is left and said) and drops the log. Refused whole if a row
is not as this tool expects.

WHEN IT TAKES EFFECT: with the restart that brings branch snow-chain's code (protect's minutes, questset, the row key's
put-back, the passage's step 18): an older server reads the walk's and Aeyinar's triggers as problems and would send a
Snow Key 4 holder's step 17 (now the kill) by the passage. Apply it in that restart's before.sh.

    python3 -E -s tools/snow_chain_20261118.py --db eqoa.db [--apply | --applied | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "snow_chain_1118_log"
WHO, VIA = "dab", "tool"
SRC = "snowchain1118"
POST = "Ronson's walkthrough (eqoa.forumotion.com/t212, the owner's source 2026-11-18)"
DETAIL = f"the Snow Chain as {POST}: tools/snow_chain_20261118.py"

QUEST_NAME, GIVER = "The Chain of Snow", "Mark of the Majestic"
KINDS = {1: ("loot", "icebound gargoyle"), 2: ("talk", "Ryini"), 3: ("kill", "Wind Mephit"), 4: ("talk", "Ryini"),
         5: ("talk", "Tamis"), 6: ("kill", "Whirlwind"), 7: ("talk", "Guenzi"), 8: ("talk", "essence of snow"),
         9: ("loot", "glacial behemoths"), 10: ("kill", "glacial champion"), 11: ("talk", "essence of snow"),
         12: ("loot", "celestial drake"), 13: ("talk", "essence of snow"), 14: ("give", "Guenzi"), 15: ("reach", ""),
         16: ("kill", "Enraged Aeyinar"), 17: ("talk", "essence of snow"), 18: ("talk", "Mark of the Majestic")}
HERB_STEP = 13  # the new step; the old 13-18 become 14-19
SHIFT = range(18, HERB_STEP - 1, -1)  # renumbered from the last down
GIVE, REACH, KILL, BACK = 15, 16, 17, 18  # after the shift
FLAG_FROM = 5  # questset: from Tamis on (past Ryini's walk)

WALK, ENRAGED = "Snow 1: Ryini's Walk", "Snow 4: Enraged Aeyinar"
HIDE_MINUTES = 60
LIMIT = 900  # seconds: the post's 15 minutes
FAIL = ("Enraged Aeyinar's fury is spent: she is gone into the snow. Walk toward her rest again with the stew to wake "
        "her.")
WAKE = "Enraged Aeyinar wakes. She will not stay past fifteen minutes."

STEW_ITEMS = ["ape's heart", "drake's foot", "gargoyle eye"]
STEW = "Guenzi's stew"
HERB_ID, HERB_NAME = 716980, "frostbloom herb"
HERB_DESC = "A pale herb that grows where the sky's snow lies. Guenzi wants one for the stew."
HERB_COPY = "gargoyle eye"
HERB_CHANCE = 0.15
HERB_TARGET = "icebound gargoyle"
HERB_ALSO = ["glacial behemoths", "snow brute", "celestial drake", "Whirlwind", "Wind Mephit", "glacial champion",
             "Aeyinar"]

SPELL_ID, SPELL_NAME = 905590, "Aeyinar's Frostbite"
SPELL_COPY = "Razeal's Weakening"
SPELL_LINE = "Target, Decrease STA by 200, Decrease AGI by 100, 120s"
SPELL_EFFECTS = {"v": 1, "scope": "target", "target_type": "Enemy", "harmful": True, "duration_s": 120, "stack": 0,
                 "level": 60, "min_level": 1, "req_equipment": "Any", "sheet_type": "Statistic Change",
                 "effects": [{"type": "Statistic Change", "kind": "stat", "stat": "sta", "amount": -200, "resist": "cr"},
                             {"type": "Statistic Change", "kind": "stat", "stat": "agi", "amount": -100, "resist": "cr"}]}
FROST_BURST = "Frost Burst"
LAY_HANDS = "Lay Hands"

EA, AE = "enraged aeyinar", "aeyinar"
EA_OLD = {"hp_pct": 100, "taken_pct": 100, "melee_pct": 100, "abilities": [{"kind": "rampage", "chance": 20}]}

STORY, STORY_QUEST = "The Storyteller", "Tales of the Snow"
BAGA = (14000.0, 16000.0, 10000.0, 12000.0)  # world 0's Baga Village zone (claude-notes/tunaria-map.md)
STORY_FACTION = "Denizens of the Plane of Sky:+250"


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


def item_id(db, name):
    r = rows(db, "itempattern", "lower(itemname) = lower(?) ORDER BY patternid", (name,))
    return r[0]["patternid"] if len(r) == 1 else None


def spell_id(db, name):
    r = rows(db, "spellpattern", "spellname = ? ORDER BY spellid", (name,))
    return r[0]["spellid"] if len(r) == 1 else None


def trigger(db, name):
    r = rows(db, "gmtriggers", "name = ?", (name,))
    return r[0] if len(r) == 1 else None


class Plan:
    """What this database holds that the tool reads, found by names and places."""

    def __init__(self, db):
        self.bad = []
        bad = self.bad.append
        q = rows(db, "quests", "name = ? AND giver = ?", (QUEST_NAME, GIVER))
        if len(q) != 1:
            bad(f"{len(q)} quests named {QUEST_NAME!r} by {GIVER!r}, want one")
            return
        self.q = q[0]
        self.quest = self.q["questid"]
        self.steps = {r["step"]: r for r in rows(db, "queststeps", "questid = ?", (self.quest,))}
        got = {s: (r["kind"], r["target"]) for s, r in self.steps.items()}
        if got != KINDS:
            bad(f"quest {self.quest}'s steps are {got}, want {KINDS}")
            return
        if db.execute("SELECT count(*) FROM queststeptargets WHERE questid = ?", (self.quest,)).fetchone()[0]:
            bad(f"quest {self.quest} has queststeptargets rows: this tool expects none")
        self.stew = [item_id(db, n) for n in STEW_ITEMS]
        self.stew_out = item_id(db, STEW)
        if None in self.stew or self.stew_out is None:
            bad(f"items {dict(zip(STEW_ITEMS + [STEW], self.stew + [self.stew_out]))}: want one record each")
            return
        give = self.steps[14]
        if sorted(int(x) for x in give["items"].split(",")) != sorted(self.stew):
            bad(f"step 14 takes {give['items']!r}, want {self.stew}")
        if str(self.stew_out) not in (give["rewarditems"] or ""):
            bad(f"step 14 gives {give['rewarditems']!r}, want {STEW} {self.stew_out}")
        if HERB_ID and row(db, "itempattern", "patternid = ?", (HERB_ID,)):
            bad(f"itempattern {HERB_ID} is there already")
        if item_id(db, HERB_NAME) is not None or rows(db, "itempattern", "lower(itemname) = lower(?)", (HERB_NAME,)):
            bad(f"an item named {HERB_NAME!r} is there already")
        self.herb_copy = item_id(db, HERB_COPY)
        if self.herb_copy is None:
            bad(f"no single {HERB_COPY!r} record to copy")
        for n in [HERB_TARGET] + HERB_ALSO:
            if not db.execute("SELECT 1 FROM npcs WHERE lower(trim(npc_name)) = lower(?) AND world = 4", (n,)).fetchone():
                bad(f"no {n!r} in world 4 (the herb's creatures)")

        # the triggers
        self.walk, self.enraged = trigger(db, WALK), trigger(db, ENRAGED)
        if self.walk is None or self.enraged is None:
            bad(f"triggers {WALK!r} {bool(self.walk)}, {ENRAGED!r} {bool(self.enraged)}: want one each")
            return
        ws = json.loads(self.walk["steps"])
        prot = [i for i, s in enumerate(ws) if s.get("do") == "protect"]
        meph = [i for i, s in enumerate(ws) if s.get("do") == "awaitdead" and s.get("npc") == "Wind Mephit"]
        if len(prot) != 1 or ws[prot[0]].get("npc") != "Ryini" or ws[prot[0]].get("minutes"):
            bad(f"{WALK!r}: want one protect of Ryini with no minutes, got {[ws[i] for i in prot]}")
        if len(meph) != 1 or any(s.get("do") in ("queststep", "questset") for s in ws):
            bad(f"{WALK!r}: want one awaitdead of the Wind Mephit and no quest step yet")
        self.walk_steps = ws
        es = json.loads(self.enraged["steps"])
        shape = [(s.get("do"), s.get("lock") or s.get("npc")) for s in es]
        if len(es) != 3 or shape[0] != ("lock", "enraged aeyinar") or es[1].get("do") != "spawn" or \
                es[1].get("minutes") != 15 or es[2] != {"do": "awaitdead"}:
            bad(f"{ENRAGED!r} is {es}, not the 11-14 lock, spawn (15 minutes), awaitdead")
        self.enraged_steps = es
        ev = json.loads(self.steps[15]["events"] or "[]")
        if not any(e.get("do") == "trigger" and e.get("trigger") == ENRAGED for e in ev):
            bad(f"step 15 does not fire {ENRAGED!r}")
        ev = json.loads(self.steps[2]["events"] or "[]")
        if not any(e.get("do") == "trigger" and e.get("trigger") == WALK for e in ev):
            bad(f"step 2 does not fire {WALK!r}")

        # Aeyinar's two
        for n, want in ((EA, 1), (AE, 1)):
            c = db.execute("SELECT count(*) FROM npcs WHERE lower(trim(npc_name)) = ? AND world = 4", (n,)).fetchone()[0]
            if c != want:
                bad(f"{c} rows named {n!r} in world 4, want {want}")
        t = row(db, "classtemplates", "name = ?", (EA,))
        nt = row(db, "npctemplate", "name = ?", (EA,))
        if t is None or nt is None or nt["template"] != EA:
            bad(f"no classtemplates/npctemplate {EA!r}")
        else:
            now = {k: t[k] for k in ("hp_pct", "taken_pct", "melee_pct")}
            now["abilities"] = json.loads(t["abilities"])
            if now != EA_OLD:
                bad(f"classtemplates {EA!r} is {now}, not the 11-12 roamer copy {EA_OLD}")
        self.ea_template = t
        if row(db, "classtemplates", "name = ?", (AE,)) or row(db, "npctemplate", "name = ?", (AE,)):
            bad(f"a template {AE!r} is there already")
        if row(db, "spellpattern", "spellid = ?", (SPELL_ID,)) or spell_id(db, SPELL_NAME) is not None:
            bad(f"spell {SPELL_ID} or {SPELL_NAME!r} is there already")
        self.spell_copy = spell_id(db, SPELL_COPY)
        self.frost_burst = spell_id(db, FROST_BURST)
        self.lay_hands = spell_id(db, LAY_HANDS)
        if None in (self.spell_copy, self.frost_burst, self.lay_hands):
            bad(f"spells {SPELL_COPY!r} {self.spell_copy}, {FROST_BURST!r} {self.frost_burst}, {LAY_HANDS!r} "
                f"{self.lay_hands}: want one each")

        # the Storyteller
        lo_x, hi_x, lo_z, hi_z = BAGA
        st = [r for r in rows(db, "npcs", "npc_name = ? AND world = 0", (STORY,))
              if lo_x <= r["x_coord"] <= hi_x and lo_z <= r["z_coord"] <= hi_z]
        if len(st) != 1:
            bad(f"{len(st)} rows of {STORY!r} in Baga Village, want one")
        if rows(db, "quests", "name = ?", (STORY_QUEST,)):
            bad(f"a quest {STORY_QUEST!r} is there already")
        self.story_quest = db.execute("SELECT max(questid) FROM quests").fetchone()[0] + 1
        self.moved = rows(db, "charquests", "questid = ? AND step >= ?", (self.quest, HERB_STEP))

    # what the triggers become
    def walk_new(self):
        out = []
        for s in self.walk_steps:
            if s.get("do") == "protect":
                out.append(dict(s, minutes=HIDE_MINUTES))
                out.append({"do": "queststep", "quest": self.quest, "step": 2})
                continue
            out.append(s)
            if s.get("do") == "awaitdead" and s.get("npc") == "Wind Mephit":
                out.append({"do": "queststep", "quest": self.quest, "step": 3})
        return out

    def enraged_new(self):
        lock, spawn, _ = self.enraged_steps
        return [lock, {"do": "broadcast", "text": WAKE}, dict(spawn, minutes=LIMIT // 60 + 1),
                {"do": "awaitdead", "seconds": LIMIT, "fail": FAIL},
                {"do": "questset", "quest": self.quest, "step": BACK, "from": FLAG_FROM}]


WALK_NOTE = (" Branch snow-chain (tools/snow_chain_20261118.py): her row out of view an hour from her death (protect's "
             "minutes: 'If she dies, you fail, and must wait one hour'); the walk does step 2 for everyone near on it as "
             "it starts and step 3 for everyone near on it when the Mephit dies (the raid escorts her, the post's), and "
             "a failed walk puts the questor back to step 2 (the encounter put-back reads her row-keyed Mephit).")
ENRAGED_NOTE = (" Branch snow-chain (tools/snow_chain_20261118.py): a broadcast as she wakes; 15 minutes to kill her "
                "(awaitdead 900 s with a failure's words; her spawn 16 so the failure comes first), then the questor is "
                "put back to the walk toward her rest; her death sets everyone near on the chain from step 5 to the way "
                "back to the Isle of Wonder (questset: 'If you are in the kill group, you will be flagged for "
                "Desolation Chain'), not those short of Tamis (the post's bypass by rez left out).")

QUEST_NOTE = ("From " + POST + " (= EQOA-Data Quests/Portal/plane-of-sky.json's ## Plane of Sky Snow Chain, "
              "github.com/Jadiction/EQOA-Data, MIT). Isles 1 to 4 and Enraged Aeyinar: the Mark of the Majestic's snow "
              "key opens the essence of snow on the main isle to Snow 1; finishing flags the player for Snow 1 from the "
              "Mark (unlocks) and opens the Desolation chain. Ryini walks down the mountain with the raid (trigger "
              "'Snow 1: Ryini's Walk': gargoyles, the Wind Mephit at the stone bridge, the popups; her death fails it "
              "and she is gone an hour; the walk does steps 2 and 3 for everyone near). The drop down the waterfall "
              "and the guide's back-and-forth between isles are teleports. The stew takes the herb, off any creature "
              "of the isles (15%, ours). Enraged Aeyinar: 15 minutes, then the questor walks toward her rest again; "
              "everyone near on the chain from Tamis on is set past her kill (the kill group's flag). The essences' "
              "names in the guide (Icicles, Snowfall, the Brood) are the game's essence of snow. No reward but the "
              "flag; the Storyteller in Baga Village tells it for faction (quest 'Tales of the Snow'). "
              "tools/snow_chain_20261118.py.")


def template_rows(p):
    """The two Aeyinars' templates as they will be."""
    rampage = {"kind": "rampage", "chance": 20}
    frostbite = {"kind": "debuff", "spell": SPELL_NAME, "every": 45, "first": 10}

    def lay_hands(below):
        return {"kind": "heal", "spell_id": p.lay_hands, "heal_pct": 100, "below": below, "every": 3600,
                "cooldown": 3600}

    def burst(pct):
        return {"kind": "aoe", "spell_id": p.frost_burst, "damage_pct": pct, "every": 30, "first": 20}

    melee = [{"kind": "meleekind", "melee_kind": k, "taken_pct": 5} for k in ("slash", "pierce", "blunt")]
    ea = {"hp_pct": 500, "melee_pct": 250,
          "abilities": [rampage] + melee + [lay_hands(50), lay_hands(25), frostbite, burst(67)]}
    ae = {"hp_pct": 100, "melee_pct": 100, "abilities": [rampage, lay_hands(50), frostbite, burst(50)]}
    return ea, ae


EA_WORDS = (" (2026-11-18, tools/snow_chain_20261118.py, " + POST + ": 'about five times the amount of hit points', "
            "'will also LH twice', 'about 100% melee immune', 'debuffs STA by 200 ... also debuff agility', quads "
            "'about 3 times harder', 'a 1k aoe'. Ours where it gives no figure: hp_pct 500 (5x the ordinary Aeyinar "
            "at 64), melee_pct 250, every melee kind 5% taken (spells and arts land), Lay Hands at 50% and at 25% "
            "once a fight, Aeyinar's Frostbite every 45 s from 10 s, Frost Burst at 67% every 30 s from 20 s)")
AE_NOTE = ("The ordinary Aeyinar of Snow 4 (8 hours), " + POST + ": 'Aeyinar has one heal', 'a wicked AoE', 'will "
           "also debuff MT's stamina', 'she'll also debuff agility'. The roamer's figures as Enraged Aeyinar had them "
           "(CR and AR at 50), with one Lay Hands at 50% (once a fight), Aeyinar's Frostbite every 45 s from 10 s, "
           "Frost Burst at 50% (~750) every 30 s from 20 s: ours. tools/snow_chain_20261118.py")


def report(db):
    q = rows(db, "quests", "name = ? AND giver = ?", (QUEST_NAME, GIVER))
    if not q:
        print("  no Snow Chain quest")
        return
    qid = q[0]["questid"]
    print(f"  quest {qid} {QUEST_NAME!r} by {GIVER!r}, requires {q[0]['requires']!r}")
    for r in db.execute("SELECT step, kind, target, items, chance, events FROM queststeps WHERE questid = ? ORDER BY step",
                        (qid,)):
        print(f"    step {r[0]} {r[1]} {r[2]!r} items {r[3]!r} chance {r[4]} events {(r[5] or '')[:90]!r}")
    also = [r[0] for r in db.execute("SELECT target FROM queststeptargets WHERE questid = ?", (qid,))]
    print(f"    other creatures: {also}")
    for n in (WALK, ENRAGED):
        t = trigger(db, n)
        print(f"  trigger {n!r}: {[s.get('do') for s in json.loads(t['steps'])] if t else None}")
    for n in (EA, AE):
        t = row(db, "classtemplates", "name = ?", (n,))
        print(f"  template {n!r}: hp_pct {t and t['hp_pct']} melee_pct {t and t['melee_pct']} "
              f"abilities {t and [a.get('kind') for a in json.loads(t['abilities'])]}")
    print(f"  herb {item_id(db, HERB_NAME)}; spell {spell_id(db, SPELL_NAME)}; "
          f"{STORY_QUEST!r} {[r['questid'] for r in rows(db, 'quests', 'name = ?', (STORY_QUEST,))]}")
    print(f"  characters on the chain: {db.execute('SELECT step, count(*) FROM charquests WHERE questid = ? AND step > 0 GROUP BY step', (qid,)).fetchall()}")


def apply(db, p):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    db.execute(f"CREATE TABLE {LOG} (seq INTEGER PRIMARY KEY, tbl TEXT NOT NULL, rowkey TEXT NOT NULL, before TEXT, "
               "after TEXT, newkey TEXT)")
    n = [0]

    def logged(tbl, key, before, after, newkey=None):
        action = "update" if before and after else "insert" if after else "delete"
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, newkey) VALUES (?, ?, ?, ?, ?)",
                   (tbl, js(key), js(before), js(after), js(newkey)))
        db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                   "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                   (at, acct[0] if acct else 0, WHO, VIA, action, tbl, js(newkey or key), js(before), js(after), DETAIL))
        n[0] += 1

    def where(key):
        return " AND ".join(f"{c} = ?" for c in key), tuple(key.values())

    def insert(tbl, values, key):
        db.execute(f'INSERT INTO "{tbl}" ({", ".join(values)}) VALUES ({", ".join("?" * len(values))})',
                   list(values.values()))
        w, a = where(key)
        logged(tbl, key, None, row(db, tbl, w, a))

    def update(tbl, key, values):
        w, a = where(key)
        before = row(db, tbl, w, a)
        db.execute(f'UPDATE "{tbl}" SET {", ".join(f"{c} = ?" for c in values)} WHERE {w}',
                   list(values.values()) + list(a))
        logged(tbl, key, before, row(db, tbl, w, a))

    def rekey(tbl, key, newkey):
        """A row's key changed in place (its rowid kept): logged by both keys."""
        w, a = where(key)
        before = row(db, tbl, w, a)
        db.execute(f'UPDATE "{tbl}" SET {", ".join(f"{c} = ?" for c in newkey)} WHERE {w}', list(newkey.values()) + list(a))
        w2, a2 = where(newkey)
        logged(tbl, key, before, row(db, tbl, w2, a2), newkey)

    qid = p.quest
    # the herb's record, a copy of the gargoyle eye's
    icols = [r[1] for r in db.execute("PRAGMA table_info(itempattern)")]
    herb = row(db, "itempattern", "patternid = ?", (p.herb_copy,))
    herb.update(patternid=HERB_ID, itemname=HERB_NAME, itemdesc=HERB_DESC)
    insert("itempattern", {c: herb[c] for c in icols}, {"patternid": HERB_ID})

    # steps 13-18 to 14-19 (from the last down: the key is unique), the characters on them with them
    for s in SHIFT:
        rekey("queststeps", {"questid": qid, "step": s}, {"questid": qid, "step": s + 1})
    for c in p.moved:
        update("charquests", {"serverid": c["serverid"], "questid": qid}, {"step": c["step"] + 1})
    # the herb's step
    insert("queststeps", {"questid": qid, "step": HERB_STEP, "kind": "loot", "target": HERB_TARGET,
                          "items": str(HERB_ID), "count": 1, "chance": HERB_CHANCE,
                          "journal": "Take a frostbloom herb off any creature of the isles of snow: they carry one now "
                                     "and then."}, {"questid": qid, "step": HERB_STEP})
    for t in HERB_ALSO:
        insert("queststeptargets", {"questid": qid, "step": HERB_STEP, "target": t},
               {"questid": qid, "step": HERB_STEP, "target": t})
    update("queststeps", {"questid": qid, "step": 7},
           {"says": "A stew for the drake mother: an ape's heart, a drake's foot, a gargoyle eye, and a herb. Any "
                    "creature of the isles may carry a frostbloom herb.",
            "journal": "Speak with Guenzi in the igloos about a stew."})
    update("queststeps", {"questid": qid, "step": GIVE},
           {"items": ", ".join(str(i) for i in p.stew + [HERB_ID]),
            "journal": "Bring the ape's heart, drake's foot, gargoyle eye and frostbloom herb to Guenzi."})
    update("queststeps", {"questid": qid, "step": KILL},
           {"journal": "Kill Enraged Aeyinar: you have fifteen minutes from her waking."})
    update("quests", {"questid": qid}, {"notes": QUEST_NOTE})

    # the triggers
    update("gmtriggers", {"slot": p.walk["slot"]}, {"steps": js(p.walk_new()), "notes": p.walk["notes"] + WALK_NOTE})
    update("gmtriggers", {"slot": p.enraged["slot"]},
           {"steps": js(p.enraged_new()), "notes": p.enraged["notes"] + ENRAGED_NOTE})

    # Aeyinar's Frostbite, Razeal's Weakening's record with the post's figures
    sp = row(db, "spellpattern", "spellid = ?", (p.spell_copy,))
    sp.update(spellid=SPELL_ID, spellname=SPELL_NAME, spelldesc=SPELL_LINE)
    insert("spellpattern", sp, {"spellid": SPELL_ID})
    se = row(db, "spelleffect", "spellid = ?", (p.spell_copy,))
    se.update(spellid=SPELL_ID, line=SPELL_LINE, source=SRC, effects=js(SPELL_EFFECTS))
    insert("spelleffect", se, {"spellid": SPELL_ID})

    # the two Aeyinars
    ea, ae = template_rows(p)
    update("classtemplates", {"name": EA}, {"hp_pct": ea["hp_pct"], "melee_pct": ea["melee_pct"],
                                            "abilities": js(ea["abilities"]), "note": p.ea_template["note"] + EA_WORDS,
                                            "source": SRC})
    base = dict(p.ea_template)
    base.update(name=AE, hp_pct=ae["hp_pct"], melee_pct=ae["melee_pct"], abilities=js(ae["abilities"]), note=AE_NOTE,
                source=SRC)
    insert("classtemplates", base, {"name": AE})
    insert("npctemplate", {"name": AE, "template": AE, "note": f"{POST}: {SRC}", "source": SRC}, {"name": AE})

    # the Storyteller's tale
    sq = p.story_quest
    insert("quests", {"questid": sq, "name": STORY_QUEST, "enabled": 1, "giver": STORY, "minlevel": 50, "maxlevel": 60,
                      "requires": str(qid), "repeatable": 0,
                      "offer": "You have walked the isles of snow in the Plane of Sky? Sit, and tell me of Ryini, of "
                               "Guenzi's stew and of the drake mother's rage. The folk of the sky will hear of it.",
                      "accept": "I will tell it.", "decline": "Not now.", "declined": "Another time, then.",
                      "source": f"{POST}: 'head to The Storyteller in Baga Village NE to obtain faction points' ({SRC})",
                      "notes": "The Snow chain told to the Storyteller in Baga Village for faction (" + POST + "). "
                               "+250 Denizens of the Plane of Sky: ours (the Rain chain's figure); the post's 'and/or "
                               "being in the kill group for the Construct' is not kept (no record of that kill group). "
                               "tools/snow_chain_20261118.py."},
           {"questid": sq})
    insert("queststeps", {"questid": sq, "step": 1, "kind": "talk", "target": STORY,
                          "journal": "Tell the Storyteller in Baga Village of the isles of snow.",
                          "says": "So the drake mother fell, and Ryini is home. The denizens of the sky will know your "
                                  "name.", "factions": STORY_FACTION}, {"questid": sq, "step": 1})
    return n[0]


def undo(db):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    n, left = 0, 0
    for tbl, rowkey, before, after, newkey in db.execute(
            f"SELECT tbl, rowkey, before, after, newkey FROM {LOG} ORDER BY seq DESC").fetchall():
        key = json.loads(rowkey)
        at_key = json.loads(newkey) if newkey else key
        w = " AND ".join(f"{c} = ?" for c in at_key)
        now = row(db, tbl, w, tuple(at_key.values()))
        if js(now) != after:
            print(f"  {tbl} {js(at_key)}: changed since this tool wrote it, left as it is")
            left += 1
            continue
        if before is None:
            db.execute(f'DELETE FROM "{tbl}" WHERE {w}', tuple(at_key.values()))
        elif after is None:
            b = json.loads(before)
            db.execute(f'INSERT INTO "{tbl}" ({", ".join(b)}) VALUES ({", ".join("?" * len(b))})', list(b.values()))
        else:
            b = json.loads(before)
            db.execute(f'UPDATE "{tbl}" SET {", ".join(f"{c} = ?" for c in b)} WHERE {w}',
                       list(b.values()) + list(at_key.values()))
        n += 1
    db.execute(f"DROP TABLE {LOG}")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', ?, '{}', NULL, NULL, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, LOG, f"undone, {n} rows put back: " + DETAIL))
    print(f"  {n} rows put back, {left} left")
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
            for tbl, key, before, after, newkey in db.execute(
                    f"SELECT tbl, rowkey, before, after, newkey FROM {LOG} ORDER BY seq"):
                what = "renumber -> " + newkey if newkey else "insert" if before is None else "update" if after else "delete"
                print(f"  {tbl} {key}: {what}")
            return 0
        report(db)
        if a.undo:
            if not has(db, LOG):
                print("nothing to undo")
                return 0
            db.execute("BEGIN IMMEDIATE")
            undo(db)
            db.execute("COMMIT")
            print("undone; takes effect at a restart (with a server that reads the old step numbers)")
            return 0
        if has(db, LOG):
            print(f"applied already ({LOG} is there): nothing to do (--undo to take it back)")
            return 0
        p = Plan(db)
        if p.bad:
            print("refused, nothing done:\n  " + "\n  ".join(p.bad))
            return 1
        print(f"  would write: quest {p.quest} steps 13-18 -> 14-19 and the herb's step 13 ({len(p.moved)} characters "
              f"moved); triggers {p.walk['slot']} and {p.enraged['slot']}; templates {EA!r} and {AE!r}; herb "
              f"{HERB_ID} (a copy of {p.herb_copy}); spell {SPELL_ID} (a copy of {p.spell_copy}); quest "
              f"{p.story_quest} {STORY_QUEST!r}")
        if not a.apply:
            print("report only: --apply to write it")
            return 0
        db.execute("BEGIN IMMEDIATE")
        n = apply(db, p)
        db.execute("COMMIT")
        print(f"applied, {n} rows; takes effect at the restart that brings branch snow-chain's code")
        report(db)
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
