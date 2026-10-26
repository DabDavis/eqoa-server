#!/usr/bin/env python3
"""The seed rows of npcmove: how the creatures of a name move (entity/movement.go,
wildlife.go, water.go; F3 20261026-021730, -020316, -042712, jib's asks).

No record of the live game says which creatures stood, roamed or swam, so the seed
goes by whole words of the creatures' names, the bodies they wear and EQOA-Data's
camp notes, and only for creatures (NPCs without the NPC bit 0x80 whose bodies are
not the players' races, charactermodel: those are the townsfolk, who never wander),
but for the camps below:

- roam 0, standing on their spawns until pulled: the Hexbones (jib: "hexbones in
  zentaurs keep do NOT roam, they are static spawn until agrod"; they stand in
  camps of four to eight, Zentar's Keep among them), every creature named a guard,
  guardian, bodyguard, lookout, watchman, sentry, sentinel, warden or gatekeeper
  (a camp's or a keep's guard: "Lionguard brigand" is not one), and the chests.
  Since F3 20261026-042712 anything that is not wildlife stands without a row too
  (jib: "Not all mobs are wild life. Fire Giants are roaming currently when they
  should not be unless otherwise specified / designated as a roamer with a path"):
  these rows say so to GMs.
- roam 1, wandering though the server's words would stand them:
  - animals the words miss (wildlife.go's lists, kept here as WILDLIFE_NOUNS,
    WILDLIFE_TITLES and WILDLIFE_GUARDS; TestSeedToolKeepsTheWildlifeLists holds
    the two the same): a name of two spawns or more, not undead, no guard's or
    person's word in it, on a body that at least 60% of the creature names wearing
    it (two at least) are wildlife's by their words: spineback on the boars' body,
    widowmaker on the black widows', razorgill on the bonefish's;
  - with --eqoa-data, the camps EQOA-Data's map (Map/zones.json) notes roaming
    ("Outdoor camp with roaming aviaks", "Outdoor pulls with roaming patrols"):
    every name within 300 of the camp (its map point laid over Tunaria by the fit
    entity/coachtowns.go's towns give, each within 7 units), not the NPC bit, not a
    guard's, and with no more spawns elsewhere than there (a name is a key: the
    row moves every one of its name).
- water "both", in and out of the water: caiman, crocodiles, alligators, crabs,
  turtles, tortoises, frogs, toads, water rats and water moccasins (jib: "Caiman,
  crocodiles, crabs can roam both").
- water "water", keeping to it: sharks, eels, piranhas, fish, anglers, barracudas,
  squid, octopi, jellyfish, minnows, rays, krakens, sea snakes, water wolves and
  water lions. A creature that spawned in the water keeps to it with no row; these
  rows only make it so for one recorded at the surface, and show it to GMs. A water
  row leaves the wandering to its kind (roam -1).

Rows are written with source "seed" and never over a row already there (a GM's,
or an earlier seed's); --undo deletes every seed row. Both take a copy of the
database first (<db>.bak-<when>-pre-moveseed). Report only by default. The table
is the server's (database/npcmove.go); --apply makes it if the server has not yet.
Restart the server after --apply or --undo, or edit any npcmove row in the GM
panel, for the running world to read them.

    tools/seed_npc_move.py [--db eqoa.db] [--eqoa-data ../external/EQOA-Data] [--apply | --undo] [--list]
"""
import argparse
import collections
import datetime
import json
import math
import os
import re
import sqlite3

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
SOURCE = "seed"

# the table as database/npcmove.go makes it
SCHEMA = """CREATE TABLE IF NOT EXISTS npcmove (
    name   TEXT PRIMARY KEY,
    roam   INTEGER NOT NULL DEFAULT -1,
    radius REAL NOT NULL DEFAULT 0,
    water  TEXT NOT NULL DEFAULT '',
    note   TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm'
)"""

STAND = re.compile(
    r"\b(?:hexbones?|guards?|guardians?|bodyguards?|lookouts?|watchm[ae]n|sentr(?:y|ies)|sentinels?|"
    r"wardens?|gatekeepers?|chests?)\b")
BOTH = re.compile(
    r"\b(?:caimans?|crocodiles?|alligators?|crabs?|turtles?|tortoises?|frogs?|toads?|water rats?|"
    r"water moccasins?)\b")
WATER = re.compile(
    r"\b(?:sharks?|eels?|piranhas?|fish|fishes|stonefish|sawfish|bonefish|swordfish|anglers?|barracudas?|"
    r"squids?|octopus(?:es)?|octopi|jellyfish|minnows?|stingrays?|rays?|krakens?|sea snakes?|"
    r"water wol(?:f|ves)|water lions?)\b")

# wildlife.go's three lists, the same words (TestSeedToolKeepsTheWildlifeLists)
WILDLIFE_NOUNS = set("""
rat mouse mice mole badger wolverine weasel ferret raccoon skunk squirrel rabbit hare beaver otter seal walrus
wolf warg worg jackal hyena fox coyote dog hound mastiff
bear grizzly kodiak cub greatbear blackbear
lion lioness lionness tiger panther puma cougar catamount lynx jaguar leopard sabretooth sabertooth cat kitten
boar pig hog sawtusk warthog
deer elk stag doe fawn moose caribou bison buffalo ox oxen yak cow bull horse pony mule goat ram sheep mammoth rhino rhinoceros elephant hippo camel
gorilla ape monkey baboon
bat bird hawk eagle falcon vulture crow raven owl parrot gull seagull pelican heron crane stork flamingo ostrich rooster chicken turkey duck goose geese swan kestrel buzzard condor magpie rook
snake asp viper cobra python adder rattler rattlesnake moccasin boa serpent serpant copperhead cooperhead kingsnake anaconda sidewinder mamba
lizard gecko iguana skink crocodile alligator caiman gharial turtle tortoise terrapin frog toad salamander newt
basilisk cockatrice
ant antlion beetle firebeetle scarab spider spiderling tarantula widow recluse scorpion scropion wasp hornet bee
fly firefly sparkfly emberfly drakefly saurianfly dragonfly mosquito gnat moth butterfly locust grasshopper cricket mantis centipede millipede crawler lavacrawler burrower
worm grub larva larvae maggot slug snail leech tick roach cockroach termite bug
crab lobster shrimp crayfish
fish eel shark hammerhead piranha pike trout salmon bass carp catfish minnow angler ray squid octopus octopi jellyfish jelly kraken whale dolphin barracuda bonefish stonefish sawfish
goo ooze slime
griffon griffin gryphon griffenne griffawn hippogriff drake wyvern manticore
hatchling mindwhipper bixie dreadwolf filthbat snowbat sandcrawler warboar hornfly razorback ghariel
""".split())
WILDLIFE_TITLES = set("""
mother father matriarch patriarch queen king alpha leader pack brood broodmother young elder runt pup calf kit chick
stalker worker soldier drone carpenter leafcutter harvester scout warrior hunter lurker breeder monarch swarm
""".split())
WILDLIFE_GUARDS = set("""
guard guardian sentry sentinel warden warder watcher watchman watchmen keeper protector defender gatekeeper bodyguard lookout
""".split())

# undead.go's reUndeadName (seed_npc_undead.py keeps the same list): an undead
# creature is not wildlife
UNDEAD = re.compile(
    r"\b(?:skeletons?|skeletal|zombies?|ghouls?|ghasts?|mumm(?:y|ies|ified)|"
    r"spectres?|specters?|spectral|wraiths?|liches?|undead|revenants?|ghosts?|ghostly|phantoms?|phantasms?|"
    r"apparitions?|banshees?|wights?|unliving|unresting|reanimated|risen|raised|cadavers?|rotting|decaying|"
    r"bones|eidolons?|souls?|haunts?|dead|shades?|restless)\b")

# for the bodies alone: a name with one of these words is not an animal however
# its body looks (a "Deathfist legionnaire" on the scorpions' body, the "Kindred
# Spirit" pets on the wolves'), and a head noun of these is a person's
NOT_ANIMALS = set("""
spirit familiar elemental elementalkin golem animation construct clockwork automaton model gargoyle imp wisp
fairy faerie pixie nixie sprite dryad sylvan treant fungusman merman mermaid siren centaur minotaur cyclops giant troll ogre orc orcish
gnoll kobold goblin hobgoblin ratman ratmen lizardman lizardmen froglok aviak kerran kappa drachnid harpy experiment scarecrow behemoth
dragon wyrm worshipper
""".split())
PEOPLE = set("""
legionnaire champion prophet slayer reaver cleric priest priestess shaman mystic oracle seer sage wizard mage magus evoker
conjurer warlock necromancer knight templar crusader paladin ranger rogue bandit brigand thief assassin cutthroat pirate sailor deckhand
merchant trader villager citizen commoner peasant slave servant minion thrall cultist disciple apprentice initiate acolyte zealot fanatic
berserker brute brawler grunt captain lieutenant sergeant general commander chief chieftain lord lady prince princess baron duke emperor
overlord master mistress fisherman gladiator myrmidon devastator hierophant drudge artisan ruffian flayer augur channeler ritualist
scavenger forager raider marauder thug burglar lockpicker chemist arsonist spiritist adept novice rider archer cavalier noble bureaucrat
centurion dragoon defrauder runecaster flameflinger soothsayer observer alarmist sycophant huntress henchman matron druid
""".split())

# EQOA-Data's map over Tunaria, fitted to entity/coachtowns.go's 30 towns (each
# within 7 units): x = AX * map x + BX, z = AZ * map y + BZ
AX, BX, AZ, BZ = 2.12999, -3625.6, 2.13043, 3799.7
CAMP_RADIUS = 300
CAMP_ROAMS = re.compile(r"\broaming\b", re.I)


def key(name):
    """The name as database.NPCClassKey keys it: lower case, trimmed, no
    trailing '$'."""
    n = name.strip().lower()
    if n.endswith("$"):
        n = n[:-1]
    return n.strip()


def words(name):
    return re.findall(r"[a-z]+", name.lower().replace("$", ""))


def singulars(w):
    out = [w]
    if w.endswith("ies") and len(w) > 4:
        out.append(w[:-3] + "y")
    if w.endswith("ves") and len(w) > 4:
        out.append(w[:-3] + "f")
    if w.endswith("es") and len(w) > 3:
        out.append(w[:-2])
    if w.endswith("s") and len(w) > 3:
        out.append(w[:-1])
    return out


def in_set(w, s):
    return any(x in s for x in singulars(w))


def wildlife_noun(w):
    return any(x in WILDLIFE_NOUNS or (len(x) >= 6 and x.endswith("fly")) for x in singulars(w))


def head_noun(ws):
    while len(ws) > 1 and in_set(ws[-1], WILDLIFE_TITLES):
        ws = ws[:-1]
    return ws[-1] if ws else ""


def wildlife_word(name):
    """wildlife.go's WildlifeWord: the head noun that makes a name an animal's,
    "" for none."""
    ws = words(name)
    if not ws or any(in_set(w, WILDLIFE_GUARDS) for w in ws):
        return ""
    h = head_noun(ws)
    return h if wildlife_noun(h) else ""


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-moveseed"
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def read_npcs(db):
    """name key -> spawns, whether every spawn is a creature, the creature
    bodies with their spawns; the undead tags; the spawns (key, world, x, z,
    npc_type) for the camps."""
    try:
        players = {(m or 0) & 0xFFFFFFFF for (m,) in db.execute("SELECT modelid FROM charactermodel")}
    except sqlite3.OperationalError:
        players = set()
    try:
        undead = {key(n): bool(u) for n, u in db.execute("SELECT name, undead FROM npcundead")}
    except sqlite3.OperationalError:
        undead = {}
    spawns = collections.Counter()
    creature = collections.defaultdict(bool)
    bodies = collections.defaultdict(collections.Counter)
    places = []
    for name, model, typ, world, x, z in db.execute("SELECT npc_name, modelid, npc_type, world, x_coord, z_coord FROM npcs"):
        if not name or not key(name):
            continue
        k = key(name)
        spawns[k] += 1
        places.append((k, world, x or 0, z or 0, typ or 0))
        if not ((typ or 0) & 0x80) and model and (model & 0xFFFFFFFF) not in players:
            creature[k] = True
            bodies[model & 0xFFFFFFFF][k] += 1
    return spawns, creature, bodies, undead, places


def is_undead(k, undead):
    return undead.get(k, bool(UNDEAD.search(k)))


def word_rows(spawns, creature):
    """The rows by the names' words: (roam, water, note)."""
    rows = {}
    for k in sorted(spawns, key=lambda n: (-spawns[n], n)):
        if not creature[k]:
            continue
        roam, water, why = None, "", []
        m = STAND.search(k)
        if m:
            roam = 0
            why.append(f"stands ({m.group(0)})")
        m = WATER.search(k)
        if m:
            water = "water"
            why.append(f"keeps to the water ({m.group(0)})")
        else:
            m = BOTH.search(k)
            if m:
                water = "both"
                why.append(f"in and out of the water ({m.group(0)})")
        if why:
            rows[k] = (-1 if roam is None else roam, water, "; ".join(why))
    return rows


def body_rows(spawns, creature, bodies, undead, raid):
    """roam 1 for the animals the words miss, by the bodies they wear."""
    rows = {}
    for model, names in bodies.items():
        wild = {k for k in names if wildlife_word(k) and not is_undead(k, undead)}
        if len(wild) < 2 or len(wild) / len(names) < 0.6:
            continue
        like = ", ".join(sorted(wild, key=lambda n: (-names[n], n))[:3])
        for k in names:
            if k in wild or not creature[k] or wildlife_word(k) or is_undead(k, undead):
                continue
            ws = words(k)
            if any(in_set(w, WILDLIFE_GUARDS) or in_set(w, NOT_ANIMALS) for w in ws) or in_set(head_noun(ws), PEOPLE):
                continue
            if spawns[k] < 2 or k in raid:
                continue
            rows.setdefault(k, (1, "", f"wanders: an animal by its body (as {like})"))
    return rows


def camp_rows(places, camps):
    """roam 1 for the camps EQOA-Data's notes say roam."""
    rows = {}
    for camp in camps:
        gx, gz = AX * camp["x"] + BX, AZ * camp["y"] + BZ
        near, far = collections.Counter(), collections.Counter()
        for k, world, x, z, typ in places:
            if typ & 0x80:
                continue
            if world == 0 and math.hypot(x - gx, z - gz) <= CAMP_RADIUS:
                near[k] += 1
            else:
                far[k] += 1
        for k, n in near.items():
            if any(in_set(w, WILDLIFE_GUARDS) for w in words(k)) or STAND.search(k) or far[k] > n:
                continue
            note = camp.get("note", "").split(".")
            what = next((s.strip() for s in note if CAMP_ROAMS.search(s)), "roaming")
            rows.setdefault(k, (1, "", f"wanders: EQOA-Data's camp {camp['name']}: \"{what}\""))
    return rows


def eqoa_data(path):
    """The roaming camps and the raid creatures' names out of EQOA-Data's map."""
    if not path:
        return [], set()
    d = json.load(open(os.path.join(path, "Map", "zones.json")))
    camps = [p for p in d["pois"] if p.get("category") == "camp" and CAMP_ROAMS.search(p.get("note") or "")]
    raid = {re.sub(r"\s*\(.*\)$", "", p["name"]).strip().lower() for p in d["pois"] if p.get("category") == "raid-mob"}
    return camps, raid


def seed_rows(db, eqoa_data_path=None):
    """name key -> (roam, water, note, spawns): what the seed writes, and why."""
    spawns, creature, bodies, undead, places = read_npcs(db)
    camps, raid = eqoa_data(eqoa_data_path)
    rows = {}
    for part in (word_rows(spawns, creature), body_rows(spawns, creature, bodies, undead, raid), camp_rows(places, camps)):
        for k, v in part.items():
            rows.setdefault(k, v)
    return {k: v + (spawns[k],) for k, v in rows.items()}


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--eqoa-data", help="a checkout of EQOA-Data: the camps its map says roam, its raid creatures left out")
    ap.add_argument("--apply", action="store_true", help="write the seed rows")
    ap.add_argument("--undo", action="store_true", help="delete every seed row")
    ap.add_argument("--no-backup", action="store_true", help="write without a copy first")
    ap.add_argument("--list", action="store_true", help="list every name, not only the counts")
    args = ap.parse_args()

    db = sqlite3.connect(args.db)
    if args.undo:
        if not args.no_backup:
            print("copy:", backup(db, args.db))
        try:
            n = db.execute("DELETE FROM npcmove WHERE source = ?", (SOURCE,)).rowcount
        except sqlite3.OperationalError:
            n = 0
        db.commit()
        print(f"deleted {n} seed rows")
        return

    rows = seed_rows(db, args.eqoa_data)
    db.execute(SCHEMA)
    have = {n: s for n, s in db.execute("SELECT name, source FROM npcmove")}
    new = [(k, v) for k, v in rows.items() if k not in have]
    stand = [k for k, v in rows.items() if v[0] == 0]
    roam = [k for k, v in rows.items() if v[0] == 1]
    both = [k for k, v in rows.items() if v[1] == "both"]
    water = [k for k, v in rows.items() if v[1] == "water"]
    print(f"{len(rows)} creature names: {len(stand)} stand ({sum(rows[k][3] for k in stand)} spawns), "
          f"{len(roam)} wander ({sum(rows[k][3] for k in roam)}), "
          f"{len(both)} go in and out of the water ({sum(rows[k][3] for k in both)}), {len(water)} keep to it "
          f"({sum(rows[k][3] for k in water)}); {len(new)} not yet in npcmove "
          f"({sum(1 for k in rows if have.get(k) not in (None, SOURCE))} left as a GM set them)")
    for title, names in (("stand", stand), ("wander", roam), ("both", both), ("water", water)):
        shown = names if args.list else names[:25]
        print(f"  {title}: " + ", ".join(f"{k} x{rows[k][3]}" for k in shown) +
              ("" if len(shown) == len(names) else f", ... ({len(names) - len(shown)} more: --list)"))
    if args.list:
        for k in roam:
            print(f"    {k}: {rows[k][2]}")
    if not args.eqoa_data:
        print("(no --eqoa-data: the camps its map says roam are left out)")
    if not args.apply:
        print("(report only: --apply writes them, --list lists every name)")
        return
    if not args.no_backup:
        print("copy:", backup(db, args.db))
    db.executemany("INSERT OR IGNORE INTO npcmove (name, roam, radius, water, note, source) VALUES (?, ?, 0, ?, ?, ?)",
                   [(k, v[0], v[1], v[2], SOURCE) for k, v in new])
    db.commit()
    print(f"wrote {len(new)} rows")


if __name__ == "__main__":
    main()
