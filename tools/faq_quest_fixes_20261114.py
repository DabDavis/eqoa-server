#!/usr/bin/env python3
"""The quest problems the GameFAQs 17-60 review listed and left (the owner, 2026-11-14: "fix the other listed quest
problems too"; the source is Jarrod Garripoli's EQOA Frontiers FAQ, GameFAQs 48152, cited "faq L<n>"; the review's
reports are the quest-guides-17 lane's high, mid, misc, path49, raid and weres-hg). Each shard's own rows: run it on
each database.

  1. Quest 101026 (The Chamberlain's Errands) step 11: Shrixiv stands only by the mud river; the FAQ (L2284) has a
     popup "as you approach the room with the wooden spiral ramp leading to the very top floor" of the kappa fortress
     spawn an attackable Shrixiv. A reach step is put in before the kill: the fortress's fourth floor (y 84, the
     nagas' room under Syilhilthis's top floor) within 30, on that floor alone (queststeps.height 5, the engine's new
     column: the fortress's floors stand over one another), and the kill step's start spawns her hostile beside the
     player. Steps 11-24 become 12-25.
  2. Missing steps:
     - the Undead Drake Lord chain (faq L3167): 101260 the Smitka trade (Ikkl is shown the four sandstone gems; two
       and 5,000 tunar to Smitka for a whirlycog; the whirlycog and the other two to Ikkl for the Ward of Desert);
       101262 the gem crafting (32 rubilent gems; jewelcrafting recipes, any kit: 4 gems a refined, 4 refined a
       polished, 2 polished the pristine gem, discovered by an Experiment; shown to Amani, cooled in the well, given
       to Amani for the Ward of Fire); 101263 Gedwin's reward (the living Magifent leaves four coloured gems;
       Magifent the Unliving sends the player to Gedwin, new, by the watchmen of Klick'Anon's east gate: the gems for
       a ring of the player's pick, the FAQ's four: the Crimson Ward of Anger and the Mystical Ward of Fortitude made,
       EQOA-Data's names, the FAQ's figures). The drake gems were Lore and one to a stack, so no player could hold the
       steps' five, four, six or 32: Lore off and ten to a stack (the FAQ's "rubilent gems only stack to 10"). The
       blue and green rings' 63s (a negative read as 63) are the FAQ's -75 strength and -20 PoT.
     - the Desolation chain (101308; faq L3090): a stone from each Mark of the Isle of Wonder (the Majestic, the
       Frigid) given to the Mark of Desolation, who then sends the player (the accept's teleport moved to that
       step). Steps 1-15 become 4-18.
     - Live's Avatar of Fear: the stone guardian's passage (Heirloom's quest 102388, Jibrish's work 1335) made on a
       database that lacks it.
     Players mid-chain keep their place: each step they stand on moves with its step (charquests.step).
  3. Faction gates (quests.factionreq): 100763 Black Swan Inn 0 (the FAQ's "some faction": its start is -200),
     100764 Citizens of Wyndhaven 150, 100766 Saerk 150, 100782 and 100783 Tunaria Gypsies 150 (the FAQ's "Tunarian
     Gypsies", L1268; Heirloom's name since Jibrish's 2026-11-11 rename). A gate needs a way to earn it: the FAQ's
     Blackburrow gnolls (Black Swan Inn +5), Bandit Hills' bandits and Jethro's Cast's gnolls (Wyndhaven +5) get
     factionhits; Saerk's goblins have theirs. On a database whose faction is still "Tunarian Gypsies" (Live) it is
     renamed and given Heirloom's Nasehir kill hits, as Heirloom has it.
  4. Too few creatures: three more pickclaw soldiers by the one (101185's four sashes) and three more Hawkblade
     mercenaries on the hill (100774 step 5; the FAQ's "a few", L1089), each a copy of its own row on the ground
     file's floor (the spots measured off worlds/ground, written here); Jezia's locket drops from every froglok
     slayer kill (101047, was 0.3) and the four froglok steps count the FAQ's Froglok Inquisitors too
     (queststeptargets; the faq-camps lane places them).
  5. Slick Svenson (Live: Slick Stevenson) and Bucktooth at level 38 (were 63; the FAQ L4372: red to a 30, green to a
     55); their template (roamer, 100%) makes their figures the level's.
  6. Turn-ins the FAQ lists that we lacked (L4555, L4520): the pickclaw tiers' own sashes (101182-101186 take the
     ripped, torn, soiled, plain and pristine sash, made; a character on a loot step of one starts its count again)
     and twelve repeatable hand-ins at Udrik Lazow for the brawlers', warriors', soldiers' swords and axes and the
     shamans', mystics', elders' totems and orbs (dropping off their tier's creature, 20%), the FAQ's experience and
     tunar (tiers 5 and 6 ours, as our sash quests scale); fifteen repeatable stack hand-ins at Djivan Mertshak, four
     tails, four high quality meats or four pelts of one were (395,136 / 1,140,480 / 950,400 experience and 2 / 8 / 4
     Werehunters of Tunaria, the FAQ's), for a werehunter as the set (101392) asks (101389 done, Gypsies of Tunaria
     1851), the parts dropping off the weres (a third of kills; ours).

Every row it changes is found by comparing the tables before and after (tools/seed_guide_gaps.py's machinery, set
"faq-quest-fixes-1114" in guide_set_log), so --undo is exact; a gm_audit line a row. Report only by default; --apply
in one transaction; a second --apply does nothing; --undo. Quests, NPCs, factions and loot are read at the server's
start: run it with the server stopped or as a keep restart's before.sh, with the build that reads queststeps.height
(the column is made here when it is missing, before the set, and stays after an --undo: the server makes it too).

    tools/faq_quest_fixes_20261114.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import seed_guide_gaps as sgg  # noqa: E402

SET = "faq-quest-fixes-1114"
WHO, VIA = "dab", "tool"
DETAIL = "The GameFAQs FAQ 48152's listed quest problems (the owner, 2026-11-14): tools/faq_quest_fixes_20261114.py"
SOURCE = "faq-quest-fixes-1114"
QSOURCE = "GameFAQs FAQ 48152 (tools/faq_quest_fixes_20261114.py)"

sgg.TABLES = tuple(sgg.TABLES) + tuple(t for t in ("charquests", "craftrecipes", "npcrespawn", "quest_guides17_log",
                                                     "quest_guides_log", "questwalls") if t not in sgg.TABLES)
# the tables whose (quest, step) is unique: moved by taking the rows out and in again
UNIQUE_STEP = ("queststeps", "queststeptargets")
# the tables a quest's step number keys, moved with an inserted step
STEP_KEYED = (("queststeptargets", "questid", "step"), ("questwalls", "questid", "step"),
              ("quest_guides17_log", "questid", "step"), ("quest_guides_log", "questid", "step"))

# ---------------------------------------------------------------------------- 1. Shrixiv in the fortress
Q_SHRIXIV = 101026
FORT = dict(world=2, x=4870.0, y=84.0, z=2335.0, radius=30.0, height=5.0)

# ---------------------------------------------------------------------------- 2. the drake lord, desolation, the passage
Q_DESERT, Q_FIRE, Q_UDL, Q_DESO, Q_PASSAGE = 101260, 101262, 101263, 101308, 102388
GEMS = ("Emeraldine Gem", "Sandstone Gem", "Pearlescent Gem", "Rubilent Gem")  # Lore off, ten to a stack
SANDSTONE, RUBILENT, PRISTINE = "Sandstone Gem", "Rubilent Gem", "Pristine Rubilent Gem"
WARD_DESERT, WARD_FIRE = "Ward of Desert", "Ward of Fire"
RING_POWER, RING_RESIST = 8145, 8146  # the blue and the green: their 63s are negatives
GEDWIN = dict(name="Gedwin", like="Watchman Bopple", x=23795.0, y=54.16, z=6745.0, facing=38, level=45, npc_type=128)
STONES = (("Mark of the Majestic", "Stone of the Majestic", "A stone the Mark of the Majestic gave, for the Mark of Desolation."),
          ("Mark of the Frigid", "Stone of the Frigid", "A stone the Mark of the Frigid gave, for the Mark of Desolation."))
COLOURS = (("crimson", "Magifent's Crimson Gem"), ("blue", "Magifent's Blue Gem"), ("green", "Magifent's Green Gem"),
           ("gold", "Magifent's Gold Gem"))

# (name, like (an item id or name), description, columns set over the like's)
ITEMS = [
    ("Whirlycog", "Deidre IV's Cog", "A gnomish whirlycog Smitka traded for two sandstone gems.", {"trade": 1, "maxstack": 1}),
    ("Refined Rubilent Gem", PRISTINE, "Four rubilent gems, refined with a jewelcrafting kit.", {"maxstack": 10, "lore": 0}),
    ("Polished Rubilent Gem", PRISTINE, "Four refined rubilent gems, polished with a jewelcrafting kit.", {"maxstack": 10, "lore": 0}),
] + [(n, "Khal Green Gem", f"A {c} gem the living Magifent carried. Gedwin of Klick'Anon sets such gems.",
      {"trade": 1, "lore": 1, "maxstack": 1, "itemcost": 0}) for c, n in COLOURS] + [
    ("Crimson Ward of Anger", RING_POWER, "A ring Gedwin set with Magifent's crimson gem.",
     {"str": 50, "dex": 25, "POWMAX": 175, "wis": -50, "intelligence": -50, "cha": -50, "sta": -10, "agi": -10,
      "PoT": 0, "HPMAX": 0, "trade": 0}),
    ("Mystical Ward of Fortitude", RING_POWER, "A ring Gedwin set with Magifent's gold gem.",
     {"sta": 75, "dex": -75, "HPMAX": 125, "str": 0, "POWMAX": 0, "PoT": 0, "trade": 0}),
] + [(n, "Size 3 Whirlygig", d, {"trade": 1, "lore": 1, "maxstack": 1, "itemicon": 140265582}) for _, n, d in STONES]

# the jewelcrafting recipes (craftrecipes; skill 3 Jewelry, the book's first tab, level 1: any kit)
RECIPES = [("refined rubilent gem", "Refined Rubilent Gem", RUBILENT, 4, "Four rubilent gems refined into one."),
           ("polished rubilent gem", "Polished Rubilent Gem", "Refined Rubilent Gem", 4, "Four refined rubilent gems polished into one."),
           ("pristine rubilent gem", PRISTINE, "Polished Rubilent Gem", 2, "Two polished rubilent gems made one pristine gem.")]

# ---------------------------------------------------------------------------- 3. faction gates
GATES = {100763: "Black Swan Inn:0", 100764: "Citizens of Wyndhaven:150", 100766: "Saerk:150",
         100782: "Tunaria Gypsies:150", 100783: "Tunaria Gypsies:150"}
GATE_HITS = [("Black Swan Inn", 5, ["gnoll soldier", "gnoll ruffian", "gnoll whitepaw", "gnoll spiritist", "gnoll redpaw",
                                    "gnoll blackpaw", "gnoll enforcer", "gnoll forager"]),
             ("Citizens of Wyndhaven", 5, ["bandit", "gnoll hierophant", "gnoll ovate", "gnoll ranger", "gnoll warder",
                                           "gnoll spear fisher"])]
GYPSIES, GYPSIES_OLD = "Tunaria Gypsies", "Tunarian Gypsies"
# Heirloom's hits on the gypsies (Jibrish, 2026-11-11, source video-audit)
GYPSY_HITS = [(n, 5) for n in ("nasehir bandit", "nasehir bard", "nasehir cleric", "nasehir hunter", "nasehir kresni",
                               "nasehir lookout", "nasehir poisonkeeper", "nasehir ranger", "nasehir rogue",
                               "nasehir sandcaster", "nasehir scout", "nasehir warrior", "nasehir wizard", "nasehir yven",
                               "fire elementaling")] + [("highwayman", 1)]

# ---------------------------------------------------------------------------- 4. spawns, 5. levels
SPAWNS = [("pickclaw soldier", 0, [(17208.3, 94.75, 15213.4), (17194.6, 94.75, 15271.1), (17204.9, 94.79, 15194.8)]),
          ("Hawkblade mercenary", 0, [(18644.0, 161.07, 17157.0), (18596.3, 159.72, 17140.9), (18608.7, 161.21, 17161.8)])]
LOCKET = (101047, 1, 0.3, 1.0)
FROG_STEPS = [(101047, 1), (101067, 1), (101076, 1), (101107, 1)]
INQUISITOR = "Froglok Inquisitor"
LEVELS = (("Slick Svenson", "Slick Stevenson"), ("Bucktooth",))
LEVEL_FROM, LEVEL_TO = 63, 38

# ---------------------------------------------------------------------------- 6. turn-ins
SASH = "destroyed pickclaw sash"
# tier quest -> (its sash, made)
TIER_SASHES = {101182: "ripped pickclaw sash", 101183: "torn pickclaw sash", 101184: "soiled pickclaw sash",
               101185: "pickclaw sash", 101186: "pristine pickclaw sash"}
# tier: levels, (melee creature, its piece, xp, tunar), (caster creature, its piece, xp)
PICKCLAW = [((1, 10), ("pickclaw brawler", "pickclaw brawler's sword", 20000, 200), ("pickclaw shaman", "pickclaw shawman's totem", 20000)),
            ((11, 20), ("adept pickclaw brawler", "pickclaw brawler's axe", 50000, 300), ("adept pickclaw shaman", "pickclaw shaman orb", 50000)),
            ((21, 30), ("pickclaw warrior", "pickclaw warrior sword", 120000, 400), ("pickclaw mystic", "pickclaw mystic totem", 120000)),
            ((31, 40), ("adept pickclaw warrior", "pickclaw warrior's axe", 240000, 500), ("adept pickclaw mystic", "pickclaw mystic's orb", 240000)),
            ((41, 50), ("pickclaw soldier", "pickclaw soldier sword", 400000, 600), ("pickclaw elder", "pickclaw elder totem", 400000)),
            ((51, 60), ("adept pickclaw soldier", "pickclaw soldier axe", 700000, 700), ("adept pickclaw elder", "pickclaw elder orb", 700000))]
PICKCLAW_CHANCE = 20
UDRIK = "Udrik Lazow"
ICON = {"sword": "pickclaw brawler's sword", "totem": "pickclaw shawman's totem", "axe": -1545510330, "orb": -611086336}
# the weres: kind, its creatures, its tail (there), its meat and pelt (made)
WERES = [("wolfwere", ["wolfwere", "werewolf"], "wolfwere tail"), ("bearwere", ["bearwere", "werebear"], "bearwere tail"),
         ("ratwere", ["wererat"], "ratwere tail"), ("lionwere", ["lionwere"], "lionwere tail"),
         ("gatorwere", ["gatorwere"], "gatorwere tail")]
WERE_PARTS = [("tail", "tails", 395136, 2), ("meat", "high quality meats", 1140480, 8), ("pelt", "pelts", 950400, 4)]
WERE_CHANCE = 33
DJIVAN, WEREHUNTERS, SET_QUEST, TOKEN_QUEST = "Djivan Mertshak", "Werehunters of Tunaria", 101392, 101389
MEAT_ICON, PELT_ICON = -84908632, -1063213915


def key(n):
    return (n or "").strip().lower()


def has(db, t):
    return sgg.has(db, t)


def cols(db, t):
    return [r[1] for r in db.execute(f'PRAGMA table_info("{t}")')]


def item_ids(db, name):
    return [r[0] for r in db.execute("SELECT patternid FROM itempattern WHERE lower(trim(itemname)) = ?", (key(name),))]


def item_id(db, name):
    ids = item_ids(db, name)
    if len(ids) != 1:
        raise SystemExit(f"{len(ids)} items named {name!r}, want 1")
    return ids[0]


def npc_rows(db, name, world=None):
    sql, args = "SELECT id FROM npcs WHERE lower(trim(npc_name)) = ?", [key(name)]
    if world is not None:
        sql, args = sql + " AND world = ?", args + [world]
    return [r[0] for r in db.execute(sql, args)]


def steps(db, q):
    c = cols(db, "queststeps")
    return [dict(zip(c, r)) for r in db.execute("SELECT * FROM queststeps WHERE questid = ? ORDER BY step", (q,))]


def quest(db, q):
    r = db.execute("SELECT * FROM quests WHERE questid = ?", (q,)).fetchone()
    return dict(zip(cols(db, "quests"), r)) if r else None


def shape(db, q):
    return [(s["step"], s["kind"], key(s["target"])) for s in steps(db, q)]


def counts(pairs):
    return ", ".join(str(p) if n == 1 else f"{p}*{n}" for p, n in pairs)


def which(db, *names):
    """The first of the names an NPC has (a shard's own spelling)."""
    for n in names:
        if npc_rows(db, n):
            return n
    return None


# the chains' shapes as this tool found them (both shards' on 2026-11-14)
SHAPES = {
    Q_SHRIXIV: 24, Q_DESERT: [(1, "loot", "sandstone drake"), (2, "give", "keeper ikkl")],
    Q_FIRE: [(1, "loot", "rubilent drake"), (2, "give", "keeper amani"), (3, "reach", "")],
    Q_UDL: 10, Q_DESO: 15,
}


def check(db):
    """Refusals: what is not as this tool expects."""
    bad = []
    for t in ("quests", "queststeps", "charquests", "npcs", "itempattern", "factions", "factionhits", "craftrecipes",
              "loottables", "npcloot", "queststeptargets", "gm_audit"):
        if not has(db, t):
            bad.append(f"no table {t}")
    if bad:
        return bad
    for q, want in SHAPES.items():
        got = shape(db, q)
        if (isinstance(want, int) and len(got) != want) or (isinstance(want, list) and got != want):
            bad.append(f"quest {q}'s steps are {got[:4]}..., not as this tool knows them ({want if isinstance(want, int) else want})")
    s = {x["step"]: x for x in steps(db, Q_SHRIXIV)}
    if s.get(11, {}).get("kind") != "loot" or key(s.get(11, {}).get("target")) != "shrixiv" or key(s.get(10, {}).get("target")) != "shrixiv":
        bad.append("quest 101026 steps 10-11 are not Shrixiv's give and loot")
    s = {x["step"]: x for x in steps(db, Q_UDL)}
    if key(s.get(9, {}).get("target")) != "magifent" or not s.get(10, {}).get("choices"):
        bad.append("quest 101263 steps 9-10 are not the living Magifent's kill and the ring's choice")
    d = quest(db, Q_DESO)
    if not d or '"teleport"' not in d["events"]:
        bad.append("quest 101308 has no accept teleport to move")
    if key(steps(db, Q_DESO)[0]["target"]) != "erych patroller":
        bad.append("quest 101308's first step is not the erych patrollers'")
    for n in GEMS + (PRISTINE, WARD_DESERT, WARD_FIRE, SANDSTONE):
        if len(item_ids(db, n)) != 1:
            bad.append(f"{len(item_ids(db, n))} items named {n!r}, want 1")
    for pid, col in ((RING_POWER, "str"), (RING_RESIST, "PoT")):
        r = db.execute(f'SELECT "{col}" FROM itempattern WHERE patternid = ?', (pid,)).fetchone()
        if not r or r[0] != 63:
            bad.append(f"item {pid}'s {col} is {r}, want 63")
    for n, like, _, _ in ITEMS:
        if item_ids(db, n):
            bad.append(f"an item is named {n!r} already")
        if isinstance(like, str) and len(item_ids(db, like)) != 1:
            bad.append(f"no one item {like!r} to make {n!r} like")
    for n in list(TIER_SASHES.values()) + [p[1] for _, m, c in PICKCLAW for p in (m, c)][2:] + \
            [f"high quality {k} meat" for k, _, _ in WERES] + [f"{k} pelt" for k, _, _ in WERES]:
        if item_ids(db, n):
            bad.append(f"an item is named {n!r} already")
    for n in (SASH, PICKCLAW[0][1][1], PICKCLAW[0][2][1]) + tuple(t for _, _, t in WERES):
        if len(item_ids(db, n)) != 1:
            bad.append(f"no one item {n!r}")
    for n in ("Smitka", "Keeper Ikkl", "Keeper Amani", "Magifent the Unliving", "Shrixiv", "Mark of Desolation", UDRIK,
              DJIVAN, GEDWIN["like"]) + tuple(m for m, _, _ in STONES):
        if not npc_rows(db, n):
            bad.append(f"no NPC {n!r}")
    if npc_rows(db, GEDWIN["name"]):
        bad.append("an NPC is named Gedwin already")
    for n, world, _ in SPAWNS:
        if len(npc_rows(db, n, world)) != 1:
            bad.append(f"{len(npc_rows(db, n, world))} {n!r} in world {world}, want 1")
    for names in LEVELS:
        n = which(db, *names)
        if n is None:
            bad.append(f"no NPC {names}")
        elif {r[0] for r in db.execute("SELECT npc_level FROM npcs WHERE lower(trim(npc_name)) = ?", (key(n),))} != {LEVEL_FROM}:
            bad.append(f"{n} is not level {LEVEL_FROM}")
    r = db.execute("SELECT chance FROM queststeps WHERE questid = ? AND step = ?", LOCKET[:2]).fetchone()
    if not r or abs(r[0] - LOCKET[2]) > 1e-9:
        bad.append(f"quest {LOCKET[0]} step {LOCKET[1]}'s chance is {r}, want {LOCKET[2]}")
    for q, st in FROG_STEPS:
        if db.execute("SELECT 1 FROM queststeps WHERE questid = ? AND step = ? AND lower(target) = 'froglok slayer'", (q, st)).fetchone() is None:
            bad.append(f"quest {q} step {st} is not a froglok slayer step")
    for q, gate in GATES.items():
        r = db.execute("SELECT factionreq FROM quests WHERE questid = ?", (q,)).fetchone()
        if not r or r[0] != "":
            bad.append(f"quest {q}'s factionreq is {r}, want none")
    fac = {r[0] for r in db.execute("SELECT name FROM factions")}
    for f in ("Black Swan Inn", "Citizens of Wyndhaven", "Saerk", WEREHUNTERS, "Gypsies of Tunaria"):
        if f not in fac:
            bad.append(f"no faction {f!r}")
    if GYPSIES not in fac and GYPSIES_OLD not in fac:
        bad.append(f"no faction {GYPSIES!r} nor {GYPSIES_OLD!r}")
    if GYPSIES in fac and GYPSIES_OLD in fac:
        bad.append(f"both {GYPSIES!r} and {GYPSIES_OLD!r}")
    if GYPSIES_OLD in fac:
        for t, c in (("factionhits", "faction"), ("charfactions", "faction"), ("npcfaction", "faction")):
            if has(db, t) and db.execute(f"SELECT 1 FROM {t} WHERE {c} = ?", (GYPSIES_OLD,)).fetchone():
                bad.append(f"{t} names {GYPSIES_OLD!r}: this tool renames only an unused one")
    for f, _, names in GATE_HITS:
        for n in names:
            if db.execute("SELECT 1 FROM factionhits WHERE npc_name = ? AND faction = ?", (n, f)).fetchone():
                bad.append(f"{n} gives {f} already")
    for q, st in TIER_SASHES.items():
        r = db.execute("SELECT items FROM queststeps WHERE questid = ? AND step = 1", (q,)).fetchone()
        if not r or r[0] != f"{item_ids(db, SASH)[0] if item_ids(db, SASH) else '?'}*4":
            bad.append(f"quest {q} step 1 takes {r}, want 4 destroyed sashes")
    for _, (m, _, _, _), (c, _, _) in PICKCLAW:
        for n in (m, c):
            if not npc_rows(db, n):
                bad.append(f"no {n!r}")
    for _, names, _ in WERES:
        if not any(npc_rows(db, n) for n in names):
            bad.append(f"no {names}")
    if not quest(db, SET_QUEST) or not quest(db, TOKEN_QUEST):
        bad.append(f"no quest {SET_QUEST} or {TOKEN_QUEST}")
    # characters a new step would strand (none on 2026-11-14): the old hand-in for the pristine gem (it needs 32 gems
    # now) and the ring's choice after Magifent's kill (his gems come at the kill)
    for q, st in ((Q_FIRE, 2), (Q_UDL, 10)):
        n = db.execute("SELECT count(*) FROM charquests WHERE questid = ? AND step = ?", (q, st)).fetchone()[0]
        if n:
            bad.append(f"{n} characters stand on quest {q} step {st}: map them by hand first")
    if db.execute("SELECT 1 FROM quests WHERE source LIKE ?", (QSOURCE + "%",)).fetchone():
        bad.append("quests of this tool are in already")
    if not quest(db, Q_PASSAGE):
        if not npc_rows(db, "stone guardian") or not item_ids(db, "Sapphire Talisman"):
            bad.append("no stone guardian or Sapphire Talisman for the passage")
    elif quest(db, Q_PASSAGE)["giver"] != "stone guardian":
        bad.append(f"quest {Q_PASSAGE} is {quest(db, Q_PASSAGE)['name']!r}, not the passage")
    for t in ("loottables",):
        if db.execute(f"SELECT 1 FROM {t} WHERE source = ?", (SOURCE,)).fetchone():
            bad.append(f"{t} has this tool's rows already")
    return bad


# ---------------------------------------------------------------------------- writers


def shift_steps(db, q, at, n):
    """Steps at and after `at` of quest q move n on, with everything their number keys and the characters on them."""
    for t, qc, sc in (("queststeps", "questid", "step"),) + STEP_KEYED + (("charquests", "questid", "step"),):
        if not has(db, t):
            continue
        if t in UNIQUE_STEP:
            # taken out and written again (new rowids), so an undo puts the old rows back whole, never one over another
            c = cols(db, t)
            rows = db.execute(f'SELECT * FROM "{t}" WHERE "{qc}" = ? AND "{sc}" >= ? ORDER BY "{sc}"', (q, at)).fetchall()
            db.execute(f'DELETE FROM "{t}" WHERE "{qc}" = ? AND "{sc}" >= ?', (q, at))
            for r in rows:
                v = dict(zip(c, r))
                v[sc] += n
                db.execute(f'INSERT INTO "{t}" ({",".join(c)}) VALUES ({",".join("?" * len(c))})', [v[k] for k in c])
            continue
        db.execute(f'UPDATE "{t}" SET "{sc}" = "{sc}" + ? WHERE "{qc}" = ? AND "{sc}" >= ?', (n, q, at))


def put_step(db, q, step, **v):
    c = cols(db, "queststeps")
    row = {k: d for k, d in zip(c, [None] * len(c))}
    row.update(questid=q, step=step, kind="talk", target="", npcid=0, items="", tunar=0, count=1, world=0, x=0.0, y=0.0,
               z=0.0, radius=15.0, journal="", answer="", says="", waiting="", notyet="", reminder="", xp=0, rewardtunar=0,
               rewarditems="", factions="", spells="", events="", chance=0.0, byclass="", choices="")
    if "height" in c:
        row["height"] = 0.0
    row.update(v)
    row = {k: x for k, x in row.items() if k in c}
    db.execute(f'INSERT INTO queststeps ({",".join(row)}) VALUES ({",".join("?" * len(row))})', list(row.values()))


def set_step(db, q, step, **v):
    db.execute("UPDATE queststeps SET " + ", ".join(f'"{k}" = ?' for k in v) + " WHERE questid = ? AND step = ?",
               list(v.values()) + [q, step])


def evjson(evs):
    return json.dumps(evs, separators=(",", ":")) if evs else ""


def make_item(db, name, like, desc, sets):
    lid = like if isinstance(like, int) else item_id(db, like)
    c = cols(db, "itempattern")
    row = dict(zip(c, db.execute("SELECT * FROM itempattern WHERE patternid = ?", (lid,)).fetchone()))
    row.pop("patternid")
    row.update(itemname=name, itemdesc=desc)
    for k, v in sets.items():
        if k not in row:
            raise SystemExit(f"item {name!r}: no column {k!r}")
        row[k] = v
    cur = db.execute(f'INSERT INTO itempattern ({",".join(row)}) VALUES ({",".join("?" * len(row))})', list(row.values()))
    return cur.lastrowid


def copy_npc(db, src, changes):
    c = cols(db, "npcs")
    row = dict(zip(c, db.execute("SELECT * FROM npcs WHERE id = ?", (src,)).fetchone()))
    row.pop("id")
    row.update(changes)
    cur = db.execute(f'INSERT INTO npcs ({",".join(row)}) VALUES ({",".join("?" * len(row))})', list(row.values()))
    if "npc_id" in c:
        db.execute("UPDATE npcs SET npc_id = id WHERE id = ?", (cur.lastrowid,))
    return cur.lastrowid


def next_quest(db):
    return db.execute("SELECT COALESCE(max(questid), 0) + 1 FROM quests").fetchone()[0]


def put_quest(db, **v):
    c = cols(db, "quests")
    row = dict(questid=next_quest(db), name="", enabled=1, giver="", minlevel=1, maxlevel=60, classes="", races="",
               requires="", repeatable=0, offer="", accept="", decline="", declined="", started="", source=QSOURCE,
               notes="", events="", factionreq="")
    row.update(v)
    row = {k: x for k, x in row.items() if k in c}
    db.execute(f'INSERT INTO quests ({",".join(row)}) VALUES ({",".join("?" * len(row))})', list(row.values()))
    return row["questid"]


def fix_shrixiv(db, say):
    s10 = {x["step"]: x for x in steps(db, Q_SHRIXIV)}
    shift_steps(db, Q_SHRIXIV, 11, 1)
    put_step(db, Q_SHRIXIV, 11, kind="reach", world=FORT["world"], x=FORT["x"], y=FORT["y"], z=FORT["z"],
             radius=FORT["radius"], height=FORT["height"],
             journal="Climb the kappa fortress in Sylhilthis' Dwell (NW) to its fourth floor, the room of the wooden spiral ramp.",
             events=evjson([{"on": "done", "do": "popup",
                             "text": "By the wooden spiral ramp a voice hisses: \"Arios's errand-runner. I said we would meet again.\""}]))
    old = json.loads(s10[11]["events"] or "[]")
    set_step(db, Q_SHRIXIV, 12,
             journal="Defeat Shrixiv by the wooden spiral ramp on the kappa fortress's fourth floor, and look about where she falls.",
             events=evjson([{"on": "start", "do": "spawn", "npc": "Shrixiv", "hostile": True}] + old))
    set_step(db, Q_SHRIXIV, 10, says="Thank you, fool. We will meet again, on the kappa fortress's fourth floor, by the wooden spiral ramp.")
    # a character on the kill goes to the fortress first: the kill's start (the spawn) comes after the reach
    db.execute("UPDATE charquests SET step = 11, count = 0 WHERE questid = ? AND step = 12", (Q_SHRIXIV,))
    say("101026: a reach step 11 at the fortress's fourth floor (y 84 within 5), Shrixiv spawned hostile at step 12's start; 11-24 -> 12-25")


def fix_gems(db, say):
    for n in GEMS:
        db.execute("UPDATE itempattern SET lore = 0, maxstack = 10 WHERE patternid = ?", (item_id(db, n),))
    db.execute("UPDATE itempattern SET str = -75 WHERE patternid = ?", (RING_POWER,))
    db.execute("UPDATE itempattern SET PoT = -20 WHERE patternid = ?", (RING_RESIST,))
    say(f"the drake gems: Lore off, ten to a stack; rings {RING_POWER} str -75, {RING_RESIST} PoT -20")


def fix_desert(db, say):
    sand, cog = item_id(db, SANDSTONE), item_id(db, "Whirlycog")
    old = {x["step"]: x for x in steps(db, Q_DESERT)}
    shift_steps(db, Q_DESERT, 2, 2)  # the old hand-in is step 4
    put_step(db, Q_DESERT, 2, kind="show", target="Keeper Ikkl", items=f"{sand}*4",
             journal="Show the four sandstone gems to Keeper Ikkl, by the blacksmith in Highpass.",
             says="Good. I have arranged a trade: Smitka, behind the coach, gives a whirlycog for two of them and 5,000 tunar. Bring it to me.")
    smitka = which(db, "Smitka")
    put_step(db, Q_DESERT, 3, kind="give", target=smitka, items=f"{sand}*2", tunar=5000,
             journal="Trade Smitka, behind the Highpass coach, two sandstone gems and 5,000 tunar for a whirlycog.",
             says="Ikkl's gems? A fair trade. Here is your whirlycog.",
             events=evjson([{"on": "done", "do": "give", "items": str(cog)}]))
    set_step(db, Q_DESERT, 4, items=f"{cog}, {sand}*2",
             journal="Bring the whirlycog and the other two sandstone gems to Keeper Ikkl in Highpass.")
    assert old[2]["kind"] == "give"
    # a character at the old hand-in (holding the four gems) shows them first
    db.execute("UPDATE charquests SET step = 2 WHERE questid = ? AND step = 4", (Q_DESERT,))
    say("101260: Ikkl shown the gems (2), Smitka's trade (3), the whirlycog and two gems to Ikkl (4, was 2)")


def fix_fire(db, say):
    gem, pristine, ward = item_id(db, RUBILENT), item_id(db, PRISTINE), item_id(db, WARD_FIRE)
    amani = "Keeper Amani"
    db.execute("UPDATE quests SET offer = ?, accept = ? WHERE questid = ?",
               ("Fire is the hardest. The rubilent drakes of Lavastorm, on the long path behind the goblin general's fort, "
                "carry rubilent gems. Take a jewelcrafting kit: 32 gems make one pristine gem, and a pristine gem I can make a ward.",
                "Thirty-two gems.", Q_FIRE))
    set_step(db, Q_FIRE, 1, items=f"{gem}*32", count=32, chance=1.0,
             journal="Take 32 rubilent gems from the rubilent drakes past Blacksmith Kadeshi in Lavastorm; they stack to ten: refine as you go.")
    set_step(db, Q_FIRE, 2, kind="show", items=str(pristine), events="",
             journal="With a jewelcrafting kit make 4 gems a refined, 4 refined a polished, 2 polished a pristine; show Keeper Amani.",
             says="Pristine, and still hot. Cool it in the well behind Tradesman Ralis, in Blackwater, then bring it back to me.")
    set_step(db, Q_FIRE, 3, journal="Cool the pristine rubilent gem in the well behind Tradesman Ralis in Blackwater.",
             events=evjson([{"on": "done", "do": "popup", "text": "The gem hisses in the well's water. It is the right temperature now."}]))
    put_step(db, Q_FIRE, 4, kind="give", target=amani, items=str(pristine),
             journal="Bring the cooled pristine rubilent gem to Keeper Amani by Arcadin's north wall.",
             says="The Ward of Fire. Take your wards back to Magifent the Unliving.",
             events=evjson([{"on": "done", "do": "give", "items": str(ward)}]))
    rid = db.execute("SELECT COALESCE(max(id), 0) FROM craftrecipes").fetchone()[0]
    for name, product, ingredient, n, desc in RECIPES:
        rid += 1
        db.execute("INSERT INTO craftrecipes (id, skill, comp, level, rarity, name, product, yield, ingredients, difficulty, xp, "
                   "discover, scroll, description, source) VALUES (?, 3, 0, 1, 0, ?, ?, 1, ?, 10, 25, 1, 0, ?, ?)",
                   (rid, name, item_id(db, product), json.dumps([{"item": item_id(db, ingredient), "count": n, "keep": True}]),
                    desc, SOURCE))
    say("101262: 32 gems, the pristine gem shown (2), cooled (3), given for the Ward of Fire (4, new); three jewelcrafting recipes")


def fix_udl(db, say):
    gems = [item_id(db, n) for _, n in COLOURS]
    s = {x["step"]: x for x in steps(db, Q_UDL)}
    ev = json.loads(s[9]["events"] or "[]") + [
        {"on": "done", "do": "popup", "text": "The living Magifent falls. Among his remains are four coloured gems: crimson, blue, green and gold."},
        {"on": "done", "do": "give", "items": counts((g, 1) for g in gems)}]
    set_step(db, Q_UDL, 9, events=evjson(ev))
    set_step(db, Q_UDL, 10, choices="", journal="Return to Magifent the Unliving in the tomb with the coloured gems.",
             says="Free of him, and of myself. Gedwin, by the watchmen of Klick'Anon's east gate, sets such gems. Take him the four.")
    rings = {"crimson": item_id(db, "Crimson Ward of Anger"), "blue": RING_POWER, "green": RING_RESIST,
             "gold": item_id(db, "Mystical Ward of Fortitude")}
    opts = [{"answer": f"The {c} gem.", "items": str(rings[c])} for c, _ in COLOURS]
    put_step(db, Q_UDL, 11, kind="give", target=GEDWIN["name"], items=counts((g, 1) for g in gems),
             journal="Take Magifent's four coloured gems to Gedwin, by the watchmen of Klick'Anon's east gate, and pick a ring.",
             says="Magifent's gems, and the drake lord is dead? Then I will set one for you. Wear it well.",
             choices=json.dumps({"ask": "Which gem shall I set?", "options": opts}, separators=(",", ":")))
    src = npc_rows(db, GEDWIN["like"])[0]
    rid = copy_npc(db, src, {"npc_name": GEDWIN["name"], "x_coord": GEDWIN["x"], "y_coord": GEDWIN["y"],
                             "z_coord": GEDWIN["z"], "facing": GEDWIN["facing"], "npc_level": GEDWIN["level"],
                             "npc_type": GEDWIN["npc_type"], "dt_kill": None})
    say(f"101263: Magifent's four gems (9), Gedwin's ring of four (11, new); Gedwin row {rid}")


def fix_deso(db, say):
    q = quest(db, Q_DESO)
    evs = json.loads(q["events"])
    tele = [e for e in evs if e.get("do") == "teleport" and e.get("on") == "accept"]
    rest = [e for e in evs if e not in tele]
    db.execute("UPDATE quests SET events = ?, offer = ? WHERE questid = ?",
               (evjson(rest), "Aeyinar has fallen to you. Desolation is the last chain, and the hardest: four isles, and the "
                "Quintessence of Veeshan at their end. Bring me a stone from each Mark of this isle, the Majestic and the "
                "Frigid, and I will send you to the first.", Q_DESO))
    shift_steps(db, Q_DESO, 1, 3)
    ids = []
    for i, (mark, stone, _) in enumerate(STONES):
        sid = item_id(db, stone)
        ids.append(sid)
        put_step(db, Q_DESO, i + 1, kind="talk", target=mark,
                 journal=f"Ask the {mark}, beside the Mark of Desolation on the Isle of Wonder, for its stone.",
                 says="You would walk desolation? Take my stone, and may it hold." if i == 0 else "Cold and sure as the snow. Here is my stone.",
                 events=evjson([{"on": "done", "do": "give", "items": str(sid)}]))
    done = [dict(e, on="done") for e in tele]
    put_step(db, Q_DESO, 3, kind="give", target="Mark of Desolation", items=counts((s, 1) for s in ids),
             journal="Bring the stones of the Majestic and the Frigid to the Mark of Desolation on the Isle of Wonder.",
             says="The Marks are agreed. Go, then, to the first isle.", events=evjson(done))
    say("101308: the Majestic's and the Frigid's stones (1, 2), given to the Mark of Desolation, who sends you (3); 1-15 -> 4-18")


def fix_passage(db, say):
    if quest(db, Q_PASSAGE):
        say(f"quest {Q_PASSAGE} (the passage) is here already")
        return
    put_quest(db, questid=Q_PASSAGE, name="Passage to the Inner Temple", giver="stone guardian", minlevel=45, maxlevel=60,
              repeatable=1, offer="Show me the Sapphire Talisman and I will send you to the inner temple.",
              accept="Enter the inner temple.", decline="Stay here.", declined="The guardian waits.",
              started="The guardian examines your talisman.", source="gm work1335 Avatar of Fear summoning (" + QSOURCE + ")",
              notes="Heirloom's quest 102388 (Jibrish's work 1335, 2026-11-13), made on this shard by tools/faq_quest_fixes_20261114.py: "
                    "the golem passage of the Avatar of Fear quest 101282, a Sapphire Talisman shown (kept for the ritual).")
    guardian = db.execute("SELECT id FROM npcs WHERE npc_name = 'stone guardian' ORDER BY id LIMIT 1").fetchone()[0]
    put_step(db, Q_PASSAGE, 1, kind="show", target="stone guardian", npcid=guardian, items=str(item_id(db, "Sapphire Talisman")),
             journal="Show the stone guardian your Sapphire Talisman to enter the inner temple.",
             says="The stone guardian opens the passage.", waiting="Bring a Sapphire Talisman from the maneaters' pool.",
             notyet="I need a Sapphire Talisman.",
             events=evjson([{"on": "start", "do": "dostep"},
                            {"on": "done", "do": "teleport", "world": 0, "x": 14828, "y": 52.7766, "z": 30553}]))
    say(f"quest {Q_PASSAGE}: the stone guardian's passage, as Heirloom's")


def fix_factions(db, say):
    fac = {r[0] for r in db.execute("SELECT name FROM factions")}
    if GYPSIES_OLD in fac:
        db.execute("UPDATE factions SET name = ? WHERE name = ?", (GYPSIES, GYPSIES_OLD))
        for n, d in GYPSY_HITS:
            db.execute("INSERT OR IGNORE INTO factionhits (npc_name, faction, delta, source) VALUES (?, ?, ?, ?)", (n, GYPSIES, d, SOURCE))
        say(f"faction {GYPSIES_OLD!r} renamed {GYPSIES!r}, with Heirloom's {len(GYPSY_HITS)} kill hits")
    for q, gate in GATES.items():
        db.execute("UPDATE quests SET factionreq = ? WHERE questid = ?", (gate, q))
    for f, d, names in GATE_HITS:
        for n in names:
            db.execute("INSERT INTO factionhits (npc_name, faction, delta, source) VALUES (?, ?, ?, ?)", (n, f, d, SOURCE))
    say("gates: " + ", ".join(f"{q} {g}" for q, g in GATES.items()) + "; kill hits for Black Swan Inn and Wyndhaven")


def fix_spawns(db, say):
    for name, world, spots in SPAWNS:
        src = npc_rows(db, name, world)[0]
        resp = db.execute("SELECT seconds, note FROM npcrespawn WHERE npc_row = ?", (src,)).fetchone() if has(db, "npcrespawn") else None
        made = []
        for i, (x, y, z) in enumerate(spots):
            rid = copy_npc(db, src, {"x_coord": x, "y_coord": y, "z_coord": z, "facing": (i * 85) % 256, "dt_kill": None})
            if resp:
                db.execute("INSERT INTO npcrespawn (npc_row, seconds, note, source) VALUES (?, ?, ?, ?)",
                           (rid, resp[0], resp[1] + " (a copy of row %d)" % src, SOURCE))
            made.append(rid)
        say(f"{name}: rows {made}, copies of {src}")
    set_step(db, LOCKET[0], LOCKET[1], chance=LOCKET[3])
    for q, st in FROG_STEPS:
        db.execute("INSERT INTO queststeptargets (questid, step, target) VALUES (?, ?, ?)", (q, st, INQUISITOR))
    for names in LEVELS:
        n = which(db, *names)
        c = db.execute("UPDATE npcs SET npc_level = ? WHERE lower(trim(npc_name)) = ?", (LEVEL_TO, key(n))).rowcount
        if has(db, "npctemplate"):
            db.execute("UPDATE npctemplate SET note = replace(note, 'level 63', 'level 38') WHERE name = ?", (key(n),))
        say(f"{n}: level {LEVEL_TO} ({c} rows)")


def loot(db, table, items, npcs, chance):
    for pid, weight in items:
        db.execute("INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel, source) VALUES (?, ?, ?, 1, 100, ?)",
                   (table, pid, weight, SOURCE))
    for n in npcs:
        db.execute("INSERT INTO npcloot (npc_name, tablename, chance, rolls, note, source) VALUES (?, ?, ?, 1, ?, ?)",
                   (key(n), table, chance, "the FAQ's hunt and gather turn-ins (faq L4555/L4520)", SOURCE))


def hand_in(db, giver, name, levels, item, n, xp, tunar, offer, says, journal, requires="", factionreq="", factions=""):
    q = put_quest(db, name=name, giver=giver, minlevel=levels[0], maxlevel=levels[1], repeatable=1, requires=requires,
                  factionreq=factionreq, offer=offer, accept="Here they are." if n > 1 else "Here it is.", decline="Not now.",
                  declined="Bring them when you like." if n > 1 else "Bring it when you like.",
                  notes="A hunt and gather turn-in of the GameFAQs FAQ 48152 (tools/faq_quest_fixes_20261114.py).")
    put_step(db, q, 1, kind="give", target=giver, items=f"{item}*{n}" if n > 1 else str(item), xp=xp, rewardtunar=tunar,
             factions=factions, journal=journal,
             says=says, events=evjson([{"on": "start", "do": "dostep"}]))
    return q


def fix_pickclaw(db, say):
    sash = item_id(db, SASH)
    for q, n in TIER_SASHES.items():
        pid = make_item(db, n, SASH, f"A {n.replace('pickclaw sash', '').strip() or 'plain'} sash off a Pickclaw goblin. "
                        "Udrik Lazow of Highpass pays for them.", {})
        for st in (1, 2):
            db.execute("UPDATE queststeps SET items = ?, journal = replace(journal, 'destroyed pickclaw sashes', ?) "
                       "WHERE questid = ? AND step = ?", (f"{pid}*4", n.replace("sash", "sashes"), q, st))
        db.execute("UPDATE charquests SET count = 0 WHERE questid = ? AND step = 1 AND count > 0", (q,))
    made = []
    for levels, (mc, mi, mxp, mt), (cc, ci, cxp) in PICKCLAW:
        for creature, piece, xp, tunar in ((mc, mi, mxp, mt), (cc, ci, cxp, 0)):
            ids = item_ids(db, piece)
            if ids:
                pid = ids[0]
            else:
                kind = next(k for k in ("sword", "axe", "totem", "orb") if k in piece)
                icon = ICON[kind] if isinstance(ICON[kind], int) else db.execute(
                    "SELECT itemicon FROM itempattern WHERE patternid = ?", (item_id(db, ICON[kind]),)).fetchone()[0]
                pid = make_item(db, piece, SASH, f"A {kind} off a Pickclaw goblin. Udrik Lazow of Highpass pays for them.",
                                {"itemicon": icon})
                made.append(piece)
            loot(db, f"pickclaw turn-in {creature}", [(pid, 1)], [creature], PICKCLAW_CHANCE)
            hand_in(db, UDRIK, f"Pickclaw Turn-in: {piece[0].upper() + piece[1:]}", levels, pid, 1, xp, tunar,
                    f"A {piece}? The Pickclaw raid us, and I pay for proof of every one put down. Hand it over.",
                    "Good. Keep them coming.", f"Give {UDRIK} in Highpass the {piece}.")
    say(f"pickclaw: tiers 2-6 take their own sash; 12 turn-ins at {UDRIK}; made {len(TIER_SASHES) + len(made)} items")


def fix_weres(db, say):
    tail_like = item_id(db, WERES[0][2])
    gate = quest(db, SET_QUEST)
    levels = (gate["minlevel"], gate["maxlevel"])
    for kind, creatures, tail in WERES:
        tid = item_id(db, tail)
        meat = make_item(db, f"high quality {kind} meat", tail_like, f"High quality meat of a {kind}, for the werehunters' studies.",
                         {"itemicon": MEAT_ICON})
        pelt = make_item(db, f"{kind} pelt", tail_like, f"The pelt of a {kind}, for the werehunters' studies.", {"itemicon": PELT_ICON})
        present = [c for c in creatures if npc_rows(db, c)]
        loot(db, f"were parts {kind}", [(tid, 2), (meat, 1), (pelt, 1)], present, WERE_CHANCE)
        for part, plural, xp, fac in WERE_PARTS:
            pid = {"tail": tid, "meat": meat, "pelt": pelt}[part]
            label = {"tail": f"{kind} tails", "meat": f"high quality {kind} meats", "pelt": f"{kind} pelts"}[part]
            hand_in(db, DJIVAN, f"The Hunt: Four {label[0].upper() + label[1:]}", levels, pid, 4, xp, 0,
                    f"Four {label}, from one kind of were: the cure for lycanthropy needs the strongest. Will you give them?",
                    "The hunters thank you. Bring more when you have them.", f"Give {DJIVAN} in the Oasis four {label}.",
                    requires=str(TOKEN_QUEST),
                    factionreq="Gypsies of Tunaria:1851", factions=f"{WEREHUNTERS}:+{fac}")
    say(f"weres: 15 stack turn-ins at {DJIVAN}; 10 items made; loot on the weres")


def run(db, spec, say):
    for n, like, desc, sets in ITEMS:
        make_item(db, n, like, desc, sets)
    fix_gems(db, say)
    fix_shrixiv(db, say)
    fix_desert(db, say)
    fix_fire(db, say)
    fix_udl(db, say)
    fix_deso(db, say)
    fix_passage(db, say)
    fix_factions(db, say)
    fix_spawns(db, say)
    fix_pickclaw(db, say)
    fix_weres(db, say)
    lengths(db, say)


def lengths(db, say):
    """The client cuts a journal line at 127 and a box at 217 less the speaker's name: refused past them."""
    long = []
    for q, st, j, says, target in db.execute(
            "SELECT questid, step, journal, says, target FROM queststeps WHERE questid IN (?, ?, ?, ?, ?, ?) OR questid IN "
            "(SELECT questid FROM quests WHERE source LIKE ?)", (Q_SHRIXIV, Q_DESERT, Q_FIRE, Q_UDL, Q_DESO, Q_PASSAGE, QSOURCE + "%")):
        if len(j) > 127:
            long.append(f"{q}/{st} journal {len(j)}")
        for line in (says or "").split("\n"):
            if len(line) > 217 - len(target) - 2:
                long.append(f"{q}/{st} says {len(line)}")
    if long:
        raise SystemExit("too long: " + "; ".join(long))


def audit_rows(db, counts_):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    for tbl, op, rid, old in db.execute(f"SELECT tbl, op, rid, old FROM {sgg.LOG} WHERE setname = ? ORDER BY seq", (SET,)).fetchall():
        if tbl in ("sqlite_sequence", "guide_set_log") or op == "made":
            continue
        new = db.execute(f'SELECT * FROM "{tbl}" WHERE rowid = ?', (rid,)).fetchone()
        after = json.dumps(dict(zip(cols(db, tbl), new)), separators=(",", ":")) if new else None
        db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                   "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                   (at, acct[0] if acct else 0, WHO, VIA, {"added": "insert", "changed": "update", "gone": "delete"}[op], tbl,
                    json.dumps({"rowid": rid}), old or None, after, DETAIL))


def audit_undo(db, n):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', 'guide_set_log', ?, NULL, NULL, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, json.dumps({"setname": SET}), f"undone, {n} rows put back: " + DETAIL))


def ensure_height(db):
    """queststeps.height, the server's (database/quests.go), made before the set so its rows compare whole."""
    if has(db, "queststeps") and "height" not in cols(db, "queststeps"):
        db.execute("ALTER TABLE queststeps ADD COLUMN height REAL NOT NULL DEFAULT 0")
        print("queststeps.height made (the server's column)")
        return True
    return False


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
        made = ensure_height(db)
        sgg.run = run  # the set's rows are this tool's, through seed_guide_gaps' snapshot, log and undo
        try:
            sgg.apply(db, {"set": SET}, a.apply, say, extra=audit_rows)
        finally:
            if made and not a.apply:  # a report leaves the database as it found it
                db.execute("ALTER TABLE queststeps DROP COLUMN height")
        if a.apply:
            print("restart the server (quests, NPCs, factions, loot and recipes' reads; a keep restart's before.sh)")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
