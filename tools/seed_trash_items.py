#!/usr/bin/env python3
"""Two vendor-trash items for every creature that has none, made by its body
(the user, 2026-10-30: "Make them by body"; jib: each creature's own two
stackable trash items, its trash roll picking between them).

tools/seed_loot_categories.py gave only 148 creature names trash of their own:
the game's trophies for the rest were never recorded. This makes them:

  - **Its kind** from its name's creature word (KEYWORDS: "dire wolf" a wolf,
    "orc pawn" an orc, "skeleton warrior" a skeleton), else from its BODY:
    a player race's body a humanoid of that race, any other the kind most of
    the creatures sharing that body have ("Old Red" on a wolf's body is a
    wolf), else a creature of no known kind. A role alone ("guard telamber",
    "prophet olvadr") is a humanoid of its body's people ("human ear"). What
    is no creature (a chest, a signpost, an announcer, a name with a number)
    gets none.
  - **Its two items** by kind (KINDS): an animal a pelt and a fang, a bird a
    feather and a talon, a reptile a scale and a fang, a fish a scale and a
    fin, an insect a carapace and a leg, a crustacean a shell and a claw, the
    undead a bone and dust, a humanoid a belt and an ear, a construct a shard
    and dust, a plant bark and a root, an ooze slime and residue, a spirit
    essence and dust, a dragon a scale and a claw, a worm chitin and a tooth,
    a fungus spores and a root, a demon a horn and ichor, the fey a wing and
    dust, a squid or kraken a tentacle and ink, anything else a hide and a
    tooth. Named for the creature where its name holds one ("wolf pelt",
    "skeleton bone"), else for its kind ("beast pelt", "human ear").
  - **Its grade** by the creature's level (GRADES: levels 1-15, 16-30, 31-45,
    46 up: "torn wolf pelt", "wolf pelt", "fine wolf pelt", "pristine wolf
    pelt"), so that the price rises with the level.
  - **Its price**: the median level of the creatures that drop it times
    PRICE_PER_LEVEL (2.40 tunar: what Sandstorm's 17 priced trophies fetch a
    level of their creatures, tools/seed_loot_categories.py's fit), 1 at the
    least; stacking to 99; for vendors only (in no quest, recipe or shop).
  - **Its record**: an existing one where one already is that item (the same
    name, stackable, tradeable, worn by no one, in no quest, recipe, catch or
    shop), its price kept if it has one; else made, its icon, colour, rent and
    class and race masks those of an existing trophy of that part (the commonest
    icon of the trophies named for it). A name an existing record of another
    use holds is not reused or repeated: the item takes another grade word
    (SPARE: "worn snake fang" where "chipped snake fang" is a quest's).

Each creature's pool gets its two (lootpool, category trash, its own name),
weight 1 each. On the live data of 2026-10-30 02:05 UTC with the four tools
before it applied: 4,100 creature names (their kind by their name 1,159, by a
player race's body 1,997, by the creatures sharing their body 669, of no known
kind 130), 906 items made and 1 reused, 8,200 pool rows. Every item made, price set and pool row written is logged in
trash_seed_log; --undo takes back those still as written (an item someone
already holds, or that any other row names, is left, and named). Report only by
default. Run after tools/seed_loot_categories.py. The server reads the items
and pools at every corpse: no restart.

    python3 -E -s tools/seed_trash_items.py --db eqoa.db [--report trash.md] [--apply | --undo]
"""
import argparse
import collections
import datetime
import json
import os
import re
import shutil
import sqlite3
import statistics
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import seed_camp_loot as campseed  # noqa: E402
import seed_loot_categories as cats  # noqa: E402

SOURCE = "trash-seed"
TRASH = cats.TRASH
PRICE_PER_LEVEL = 2.40
MAX_STACK = 99
BANDS = ((1, 15), (16, 30), (31, 45), (46, 100))
SCROLL_ART = cats.SCROLL_ART
RACES = {0: "human", 1: "elf", 2: "dark elf", 3: "gnome", 4: "dwarf", 5: "troll", 6: "barbarian", 7: "halfling",
         8: "erudite", 9: "ogre"}

# kind -> (two parts, the noun for a creature its name names none of)
KINDS = {
    "animal": (("pelt", "fang"), "beast"),
    "bird": (("feather", "talon"), "bird"),
    "reptile": (("scale", "fang"), "reptile"),
    "fish": (("scale", "fin"), "fish"),
    "insect": (("carapace", "leg"), "insect"),
    "crustacean": (("shell", "claw"), "crab"),
    "undead": (("bone", "dust"), "skeletal"),
    "humanoid": (("belt", "ear"), "humanoid"),
    "construct": (("shard", "dust"), "stone"),
    "plant": (("bark", "root"), "forest"),
    "ooze": (("slime", "residue"), "ooze"),
    "spirit": (("essence", "dust"), "spirit"),
    "dragon": (("scale", "claw"), "drake"),
    "worm": (("chitin", "tooth"), "worm"),
    "fungus": (("spore", "root"), "fungal"),
    "demon": (("horn", "ichor"), "demon"),
    "fey": (("wing", "dust"), "fey"),
    "cephalopod": (("tentacle", "ink"), "squid"),
    "creature": (("hide", "tooth"), "creature"),
}
# a part's name in each band: "{grade} {noun} {part}"; a quantity word makes "{grade} of {noun} {part}"
GRADES = {
    "pelt": ("torn", "", "fine", "pristine"), "fang": ("chipped", "", "sharp", "flawless"),
    "feather": ("broken", "", "fine", "pristine"), "talon": ("chipped", "", "sharp", "flawless"),
    "scale": ("chipped", "", "fine", "flawless"), "fin": ("torn", "", "fine", "pristine"),
    "carapace": ("cracked", "", "hardened", "pristine"), "leg": ("broken", "", "intact", "pristine"),
    "shell": ("cracked", "", "polished", "pristine"), "claw": ("chipped", "", "sharp", "flawless"),
    "bone": ("brittle", "", "polished", "pristine"), "belt": ("frayed", "", "sturdy", "ornate"),
    "ear": ("shriveled", "", "intact", "pristine"), "shard": ("cracked", "", "polished", "flawless"),
    "bark": ("rotting", "", "fine", "pristine"), "root": ("withered", "", "fresh", "pristine"),
    "essence": ("faint", "", "vibrant", "pure"), "hide": ("torn", "", "fine", "pristine"),
    "tooth": ("cracked", "", "sharp", "flawless"), "chitin": ("cracked", "", "fine", "pristine"),
    "horn": ("chipped", "", "sharp", "flawless"), "wing": ("torn", "", "intact", "pristine"),
    "tentacle": ("torn", "", "fine", "pristine"),
}
QUANTITY = {"dust": ("pinch", "pile", "heap", "mound"), "slime": ("dab", "glob", "jar", "vial"),
            "residue": ("smear", "lump", "clot", "cake"), "spore": ("pinch", "puff", "pouch", "sack"),
            "ichor": ("drop", "vial", "flask", "jar"), "ink": ("drop", "vial", "flask", "jar")}
SPARE = ("worn", "dull", "plain", "common", "old")  # a grade word for a name another record of other use holds
# where a part's icon is found: the words of the trophies named for it
ICON_WORDS = {
    "pelt": ("pelt", "fur"), "fang": ("fang", "tooth"), "feather": ("feather", "feathers"), "talon": ("talon",),
    "scale": ("scale", "scales"), "fin": ("scale", "scales"), "carapace": ("carapace",), "leg": ("leg", "legs"),
    "shell": ("shell", "carapace"), "claw": ("claw",), "bone": ("bone", "skull"), "dust": ("dust",),
    "belt": ("belt",), "ear": ("ear", "eye"), "shard": ("shard",), "bark": ("bark", "root", "roots"),
    "root": ("root", "roots"), "slime": ("slime",), "residue": ("slime", "dust"), "essence": ("essence", "shard"),
    "hide": ("hide",), "tooth": ("tooth", "fang"), "chitin": ("chitin", "carapace"), "spore": ("spores", "dust"),
    "horn": ("horn",), "ichor": ("ink", "slime"), "wing": ("wing", "wings"), "tentacle": ("leg", "legs"),
    "ink": ("ink", "slime"),
}

# a name's creature word -> (kind, the noun its items carry)
KEYWORDS = {}


def _kw(kind, words, noun=None):
    for w in words.split():
        KEYWORDS[w] = (kind, noun or w)


_kw("animal", "wolf worg bear boar sawtusk warthog rhinoceros rhino hound dog cat lion lioness tiger panther puma "
              "cougar leopard jaguar badger fox jackal coyote elephant mammoth gorilla ape monkey yak bison buffalo ox "
              "bull cow horse pony elk deer stag moose mole weasel wolverine otter beaver skunk rabbit hare squirrel "
              "kodiak grizzly lynx bobcat hyena raccoon wildcat sabertooth rat bat mastiff boarling beast manticore "
              "catamount swine")
_kw("animal", "wererat", "rat")
_kw("animal", "greatbear", "bear")
_kw("animal", "werebear", "bear")
_kw("animal", "lionwere werelion", "lion")
_kw("animal", "werewolf", "wolf")
_kw("animal", "gatorwere", "gator")
_kw("bird", "aviak griffon griffin crow raven rook magpie eagle hawk falcon owl vulture buzzard harpy roc kestrel "
            "condor heron crane gull parrot owlbear")
_kw("bird", "griffenne griffawn", "griffon")
_kw("reptile", "snake viper rattler copperhead cobra kingsnake python asp adder serpent lizard iguana gecko basilisk "
               "crocodile gharial alligator gator tortoise turtle saurian skink salamander newt toad")
_kw("fish", "piranha eel trout minnow pike shark fish barracuda catfish bass carp perch sunfish swordfish stingray "
            "moray zander rudd gambusa megaladon ray")
_kw("fish", "bloodfin", "shark")
_kw("insect", "scarab scorpion ant wasp hornet bee beetle spider tarantula centipede millipede crawler mosquito tick "
              "locust mantis cricket roach cockroach termite firefly dragonfly sparkfly emberfly skipper burrower")
_kw("worm", "worm grub larva maggot")
_kw("insect", "scropion", "scorpion")
_kw("insect", "spiderling widow widowmaker", "spider")
_kw("insect", "fireflies", "firefly")
_kw("insect", "sparkflies", "sparkfly")
_kw("insect", "emberflies", "emberfly")
_kw("insect", "dragonflies", "dragonfly")
_kw("insect", "saurianflies", "saurianfly")
_kw("crustacean", "crab lobster crayfish")
_kw("undead", "skeleton zombie mummy ghoul ghost wraith phantom spectre specter shade banshee lich revenant wight "
              "ghast apparition hexbone hatebone corpse deathknight")
_kw("undead", "skeletal", "skeleton")
_kw("undead", "undead", "skeletal")
_kw("spirit", "spirit soul wisp shadowwisp will")
_kw("humanoid", "goblin hobgoblin gnoll froglok orc kobold troll ogre giant kerran kappa ratman centaur minotaur "
                "cyclops lizardman sahuagin merkan welahi pickclaw barbarian dwarf gnome halfling erudite elf human "
                "dryad siren mermaid merman bandit pirate brigand thug ruffian cutthroat raider reaver gladiator outlaw "
                "cultist zealot berserker deckhand sailor poacher")
_kw("humanoid", "forglok", "froglok")
_kw("humanoid", "tae", "tae ew")
# a role names no creature: its kind a humanoid, its noun its body's
ROLES = set("""assassin knight soldier warrior guard scout shaman wizard mage necromancer cleric druid ranger hunter
slayer villager farmer fisherman servant slave prisoner drudge mystic crusader templar champion oracle seer prophet augur
channeler ritualist spiritist gypsy forager artisan henchman minion brute basher brawler myrmidon defender warder stalker
rider matriarch alarmist captain monk paladin rogue bard enchanter magician alchemist sorcerer warlock acolyte priest
priestess apprentice guildmaster merchant banker trader smith smithy tailor jeweler baker grocer innkeeper bouncer
citizen peasant noble lord lady king queen prince princess duke chief chieftain warlord commander lieutenant sergeant
general sentry watchman deputy sheriff rebel fanatic heretic pilgrim hermit nomad tribesman madman squire corporal
inspector overseer inquisitor bishop elder sapper worker herbalist harbinger destroyer partisan guardian""".split())
# what no creature is: no trash
NOT_CREATURES = re.compile(r"\d|\b(chest|signpost|announcements)\b|^(freeport|qeynos|highpass|toxxulia)$")
_kw("construct", "golem gargoyle elemental marionette animation statue construct sentinel automaton clockwork armor "
                 "elementaling elementalkin")
_kw("plant", "treant shrub vine sapling thornbush briar bloom kolbok deadwood arborian rotwood")
_kw("fungus", "mushroom fungus sporeling fungusman mycanoid")
_kw("fungus", "fungal funagl", "fungal")
_kw("demon", "imp daemon demon efreet ifrit demion")
_kw("fey", "pixie bixie nixie sprite brownie sylvan faun nymph")
_kw("cephalopod", "kraken squid octopus")
_kw("construct", "dervish model overdrone")
_kw("dragon", "hydra")
_kw("ooze", "goo jelly ooze slime cube pudding gel blob")
_kw("ooze", "slith tar", "slith")
_kw("dragon", "drake dragon wyvern wyrm wurm")

LOG_DDL = """CREATE TABLE IF NOT EXISTS trash_seed_log (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    kind    TEXT NOT NULL,   -- item (a record made) | price (a record's itemcost set) | pool (a lootpool row written)
    ref     TEXT NOT NULL,   -- the patternid, or the lootpool id
    old     TEXT NOT NULL,   -- price: the itemcost before
    new     TEXT NOT NULL,   -- item: [name, itemcost]; price: the itemcost set; pool: [npc_name, patternid]
    applied TEXT NOT NULL
)"""


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def band_of(level):
    for i, (lo, hi) in enumerate(BANDS):
        if lo <= level <= hi:
            return i
    return len(BANDS) - 1 if level > BANDS[-1][1] else 0


def name_kind(name):
    """(kind, noun) by the last creature word of a name; a role alone ("guard telamber") a humanoid of no noun
    (its body's); (None, None) when it holds neither."""
    ws = re.findall(r"[a-z']+", name.lower())
    for i in range(len(ws) - 1, -1, -1):
        w = ws[i]
        if w in KEYWORDS:
            return KEYWORDS[w]
        if len(w) > 3 and w.endswith("s") and w[:-1] in KEYWORDS:
            return KEYWORDS[w[:-1]]
    if any(w in ROLES or (w.endswith("s") and w[:-1] in ROLES) for w in ws):
        return "humanoid", None
    return None, None


def item_name(part, grade, noun, word=None):
    """The item's name: its grade's word (or word), the noun (none when it is the part: "glob of slime"), the part."""
    noun = "" if noun == part else noun
    if part in QUANTITY:
        return " ".join(x for x in (word or QUANTITY[part][grade], "of", noun, part) if x)
    return " ".join(x for x in (word if word is not None else GRADES[part][grade], noun, part) if x)


def article(s):
    return ("an " if s[:1] in "aeiou" else "a ") + s


class Plan:
    def __init__(self, db):
        self.db = db
        self.bad = cats.excluded_items(db)
        have = {n for (n,) in db.execute("SELECT DISTINCT npc_name FROM lootpool WHERE category = ? AND npc_name <> ''", (TRASH,))}
        creatures = {n: r for n, r in db.execute("SELECT npc_name, region FROM lootcreature")}
        races = {m: r for m, r in db.execute("SELECT modelid, race FROM charactermodel")} if has_table(db, "charactermodel") else {}
        levels = collections.defaultdict(list)
        models = collections.defaultdict(collections.Counter)
        for name, typ, lvl, model in db.execute("SELECT npc_name, npc_type, npc_level, modelid FROM npcs WHERE npc_name IS NOT NULL"):
            if (typ or 0) & 0x80:
                continue
            k = campseed.npc_key(name)
            if k in creatures:
                levels[k].append(lvl or 0)
                models[k][model or 0] += 1
        # the kind each body's named creatures mostly are
        body = collections.defaultdict(collections.Counter)
        for k, ms in models.items():
            kind, noun = name_kind(k)
            if kind and noun:
                for m, n in ms.items():
                    body[m][(kind, noun)] += n
        self.kinds = {}
        self.how = {}
        for k in sorted(creatures):
            if creatures[k] == "-" or k in have or not levels.get(k) or NOT_CREATURES.search(k):
                continue
            kind, noun = name_kind(k)
            how = "its name"
            if not kind or not noun:
                model = models[k].most_common(1)[0][0]
                humanoid = [kn for kn in body.get(model, {}) if kn[0] == "humanoid"]
                if model in races:
                    kind, noun, how = "humanoid", RACES.get(races[model], "humanoid"), "a player race's body"
                elif kind == "humanoid" and humanoid:  # a role: its body's people
                    noun = max(humanoid, key=lambda kn: body[model][kn])[1]
                    how = "its body (the creatures sharing it)"
                elif kind == "humanoid":
                    noun, how = KINDS["humanoid"][1], "its name (a role alone)"
                elif body.get(model):
                    (kind, noun), n = body[model].most_common(1)[0]
                    how = "its body (the creatures sharing it)"
                else:
                    kind, noun, how = "creature", KINDS["creature"][1], "no known kind"
            self.kinds[k] = (kind, noun)
            self.how[k] = how
        self.levels = {k: statistics.median([l for l in levels[k] if l > 0] or [1]) for k in self.kinds}
        # every existing record by name; the icon, colour and masks a part's trophies wear
        self.records = {}
        for pid, name, cost, ms, trade, slot, icon in db.execute(
                "SELECT patternid, itemname, itemcost, maxstack, trade, equipslot, itemicon FROM itempattern ORDER BY patternid"):
            key = " ".join((name or "").split()).lower()
            self.records.setdefault(key, (pid, cost or 0, ms or 0, trade or 0, slot, icon))
        self.templates = {}
        for part, words in ICON_WORDS.items():
            got = collections.Counter()
            first = {}
            for pid, name, icon in db.execute(
                    "SELECT patternid, itemname, itemicon FROM itempattern WHERE equipslot < 1 AND itemicon <> ?", (SCROLL_ART,)):
                ws = set(re.findall(r"[a-z]+", (name or "").lower()))
                if ws & set(words):
                    got[icon] += 1
                    first.setdefault(icon, pid)
            if got:
                self.templates[part] = first[got.most_common(1)[0][0]]
        fallback = self.templates.get("hide") or self.templates.get("pelt")
        for part in ICON_WORDS:
            self.templates.setdefault(part, fallback)

    def usable(self, key):
        """An existing record that already is this trash: (patternid, its price) or None."""
        r = self.records.get(key)
        if not r:
            return None
        pid, cost, ms, trade, slot, icon = r
        if ms > 1 and not trade and (slot is None or slot < 1) and icon != SCROLL_ART and pid not in self.bad:
            return pid, cost
        return None

    def build(self):
        """(items {name: {...}}, pool rows [(creature, name)])."""
        items = {}
        pool = []
        for k, (kind, noun) in sorted(self.kinds.items()):
            grade = band_of(int(self.levels[k]))
            for part in KINDS[kind][0]:
                name = item_name(part, grade, noun)
                key = name.lower()
                for word in SPARE:  # a name a record of other use holds: the next free grade word
                    if not (key in self.records and not self.usable(key)):
                        break
                    name = item_name(part, grade, noun, word)
                    key = name.lower()
                it = items.setdefault(key, {"name": name, "part": part, "kind": kind, "levels": [], "creatures": []})
                it["levels"].append(self.levels[k])
                it["creatures"].append(k)
                pool.append((k, key))
        for key, it in items.items():
            use = self.usable(key)
            it["price"] = max(1, round(PRICE_PER_LEVEL * statistics.median(it["levels"])))
            if use:
                it["pid"], it["old"] = use
                if it["old"] > 0:
                    it["price"] = it["old"]
            else:
                it["pid"], it["old"] = None, None
        return items, pool


def apply(db, plan, items, pool):
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    log = "INSERT INTO trash_seed_log (kind, ref, old, new, applied) VALUES (?, ?, ?, ?, ?)"
    cols = [r[1] for r in db.execute("PRAGMA table_info(itempattern)") if r[1] != "patternid"]
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_DDL)
        rows = {}  # a part's template record, read once
        for key, it in sorted(items.items()):
            if it["pid"] is None:
                t = plan.templates[it["part"]]
                if t not in rows:
                    rows[t] = db.execute(f"SELECT {', '.join(cols)} FROM itempattern WHERE patternid = ?", (t,)).fetchone()
                tpl = dict(zip(cols, rows[t]))
                tpl.update(itemname=it["name"], itemcost=it["price"], maxstack=MAX_STACK, equipslot=-1, trade=0, lore=0,
                           craft=0, levelreq=0, maxhp=0, duration=0,
                           itemdesc=f"{article(it['name']).capitalize()}. A merchant will pay for it.")
                cur = db.execute(f"INSERT INTO itempattern ({', '.join(tpl)}) VALUES ({', '.join('?' * len(tpl))})",
                                 list(tpl.values()))
                it["pid"] = cur.lastrowid
                db.execute(log, ("item", str(it["pid"]), "", json.dumps([it["name"], it["price"]]), when))
            elif it["old"] == 0:
                db.execute("UPDATE itempattern SET itemcost = ? WHERE patternid = ?", (it["price"], it["pid"]))
                db.execute(log, ("price", str(it["pid"]), "0", str(it["price"]), when))
        (top,) = db.execute("SELECT COALESCE(MAX(id), 0) FROM lootpool").fetchone()
        db.executemany("INSERT INTO lootpool (category, region, npc_name, patternid, weight, minlevel, maxlevel, source) "
                       "VALUES (?, '', ?, ?, 1, 1, 100, ?)", [(TRASH, k, items[key]["pid"], SOURCE) for k, key in pool])
        db.executemany(log, [("pool", str(rid), "", json.dumps([k, pid]), when) for rid, k, pid in db.execute(
            "SELECT id, npc_name, patternid FROM lootpool WHERE id > ? AND source = ? ORDER BY id", (top, SOURCE)).fetchall()])
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db):
    if not has_table(db, "trash_seed_log"):
        print("nothing logged: nothing to undo")
        return
    db.execute("BEGIN IMMEDIATE")
    try:
        back, kept = 0, []
        for kind, ref, old, new in db.execute("SELECT kind, ref, old, new FROM trash_seed_log ORDER BY seq DESC").fetchall():
            if kind == "pool":
                k, pid = json.loads(new)
                if db.execute("SELECT category, npc_name, patternid, source FROM lootpool WHERE id = ?", (int(ref),)).fetchone() == \
                        (TRASH, k, pid, SOURCE):
                    db.execute("DELETE FROM lootpool WHERE id = ?", (int(ref),))
                    back += 1
                else:
                    kept.append(f"lootpool {ref}")
            elif kind == "price":
                if db.execute("SELECT itemcost FROM itempattern WHERE patternid = ?", (int(ref),)).fetchone() == (int(new),):
                    db.execute("UPDATE itempattern SET itemcost = ? WHERE patternid = ?", (int(old), int(ref)))
                    back += 1
                else:
                    kept.append(f"itempattern {ref}")
            elif kind == "item":
                pid = int(ref)
                name, cost = json.loads(new)
                held = sum(db.execute(f"SELECT count(*) FROM {t} WHERE {c} = ?", (pid,)).fetchone()[0]
                           for t, c in (("charinventory", "patternid"), ("npcitem", "patternid"), ("lootpool", "patternid"),
                                        ("loottables", "patternid"), ("auction_lots", "patternid"))
                           if has_table(db, t))
                if held:
                    kept.append(f"{name!r} ({pid}: held or named by {held} rows)")
                elif db.execute("SELECT itemname, itemcost FROM itempattern WHERE patternid = ?", (pid,)).fetchone() == (name, cost):
                    db.execute("DELETE FROM itempattern WHERE patternid = ?", (pid,))
                    back += 1
                else:
                    kept.append(f"itempattern {pid}")
        db.execute("DROP TABLE trash_seed_log")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"took back {back} changes" + (f"; {len(kept)} left as they are: " + ", ".join(kept[:20]) if kept else ""))


def report(path, plan, items, pool):
    by = collections.defaultdict(list)
    for key, it in items.items():
        by[it["kind"]].append(it)
    hows = collections.Counter(plan.how.values())
    L = ["# Trash by body: the review", ""]
    L.append(f"Every creature name that had no trash of its own: {len(plan.kinds):,} (its kind by its name "
             f"{hows['its name']:,}, by a player race's body {hows['a player race' + chr(39) + 's body']:,}, by the "
             f"creatures sharing its body {hows['its body (the creatures sharing it)']:,}, of no known kind "
             f"{hows['no known kind']:,}). Items: {len(items):,} ({sum(1 for i in items.values() if i['pid'] is None):,} "
             f"to make, {sum(1 for i in items.values() if i['pid'] is not None):,} existing records reused). Prices: the "
             f"median level of the creatures that drop an item times {PRICE_PER_LEVEL:.2f} tunar.")
    L.append("")
    L.append("| kind | parts | creatures | items | price by band: 1-15 / 16-30 / 31-45 / 46+ (median, tunar) | for example |")
    L.append("|---|---|---:|---:|---|---|")
    for kind in KINDS:
        its = by.get(kind, [])
        if not its:
            continue
        ks = {k for i in its for k in i["creatures"]}
        prices = []
        for b in range(len(BANDS)):
            ps = [i["price"] for i in its if band_of(int(statistics.median(i["levels"]))) == b]
            prices.append(f"{statistics.median(ps):.0f}" if ps else "-")
        eg = sorted(its, key=lambda i: -len(i["creatures"]))[:4]
        L.append(f"| {kind} | {' and '.join(KINDS[kind][0])} | {len(ks):,} | {len(its):,} | {' / '.join(prices)} | " +
                 ", ".join(f"{i['name']} ({i['price']}t)" for i in eg) + " |")
    L.append("")
    L.append("## Trash an hour (jib's counts: 1.475 items a kill)")
    L.append("")
    L.append("| band | a creature's trash, median price | 40 kills an hour | 60 kills an hour |")
    L.append("|---|---:|---:|---:|")
    for b, (lo, hi) in enumerate(BANDS):
        ps = [statistics.mean(items[key]["price"] for k2, key in pool if k2 == k)
              for k in plan.kinds if band_of(int(plan.levels[k])) == b]
        if ps:
            m = statistics.median(ps)
            L.append(f"| {lo}-{hi if hi < 100 else 'up'} | {m:.0f} | {1.475 * 40 * m:,.0f} tunar | {1.475 * 60 * m:,.0f} tunar |")
    L.append("")
    with open(path, "w") as f:
        f.write("\n".join(L) + "\n")


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-trash"
    shutil.copy2(path, dst)
    return dst


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--report", help="write the review (Markdown) here")
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="make the items and fill the pools (logged)")
    g.add_argument("--undo", action="store_true", help="take back what --apply wrote")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        if not a.no_backup and has_table(db, "trash_seed_log"):
            print(f"copy: {backup(a.db)}")
        undo(db)
        return
    if a.apply and has_table(db, "trash_seed_log"):
        sys.exit("trash_seed_log is there: applied already (--undo first)")
    if not has_table(db, "lootpool") or not has_table(db, "lootcreature"):
        sys.exit("no lootpool or lootcreature: run tools/seed_loot_categories.py first")
    plan = Plan(db)
    items, pool = plan.build()
    made = sum(1 for i in items.values() if i["pid"] is None)
    print(f"creatures given trash: {len(plan.kinds):,} ({collections.Counter(k for k, _ in plan.kinds.values()).most_common()})")
    print(f"how their kinds were found: {dict(collections.Counter(plan.how.values()))}")
    print(f"items: {len(items):,} ({made:,} to make, {len(items) - made:,} existing records reused, "
          f"{sum(1 for i in items.values() if i['old'] == 0):,} of them priced); pool rows {len(pool):,}")
    if a.report:
        report(a.report, plan, items, pool)
        print(f"report: {a.report}")
    if not a.apply:
        print("report only: --apply to write")
        return
    if not a.no_backup:
        print(f"copy: {backup(a.db)}")
    apply(db, plan, items, pool)
    print(f"made {made:,} items, wrote {len(pool):,} pool rows")


if __name__ == "__main__":
    main()
