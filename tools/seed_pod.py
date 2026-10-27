#!/usr/bin/env python3
"""The Plane of Disease, peopled, and the way there: EQOA-Data's guides
(github.com/Jadiction/EQOA-Data: Quests/Portal/plane-of-disease.json, the
portal quest, the spell quest's nine rare spawns, the weapon quest; Quests/60/
evil-healer.json and Quests/Misc/stat-boost-quests.json for the Dead Hills;
Quests/HuntAndGather/hunt-and-gather.json for the Patrons of Bertoxxulous)
put on this server, where no one's data had PoD at all (F3 pass 17, the
user's "unable to travel to PoD eqoaclassic"). The trips themselves are
handlers/waysender.go's (Necrologist Zizclank's, the portals').

Where PoD is, read off the disc: the Secrets world (world 5, DATA3/
SECRETS.ESF) has two zones with anything in them; CLIENT's ZoneName names
zone 0's cell "Last Home" and zone 5's "Zaoran Plateau" (the vampires'
Sanctuary). The guides port to PoD "from the Last Home" and put Yarfsnoogle
"in the Noroxus pools area in the Plane of Disease (Last Home E)"; zone 0
(x 4031-5985, z 2000-4000) is a ring of canyon round high hills that hold a
sunken valley in the middle (a crescent slime pool round an island with a
throne), slime pools strung along the ring (the zone BSP's slime leaves,
internal/ground/env.go), a pocket of seven pools to the north-east, a slime
lake to the north-west whose island holds a portal pad in a ring of twelve
standing stones and, south-east, a bowl no walk reaches. eqoa.live's wiki
map of the zone (its Plane of Disease page's interactive map, whose points
land on its drawing flipped top to bottom) has Noroxus in that north-west
lake's lobe (drawn on the map's east side, turned half round: the island
with its ring of stones in the pool), the Queen in the bowl, and the Twins
and Bertoxxulous in the middle valley (claude-notes/plane-of-disease-20261027.md).
The Dead Hills are the Rathe's cell (world 1, row 2, col 1): their NW ninth
holds a fenced yard of tombs at (4333, 6405), "the mausoleum in the middle".

What is ours: every spot (the guides say "across the first pond", "behind
tree next to banker", "end of valley just before nox pools"; the anchors
follow them over the ground), every level (the guides give cons to a level
60: "db and lb", "1/2 red"), every body (copied from a namesake's or kin's
row on this server: no model id is guessed; a name with several bodies is
pinned to one; the three bosses jib named theirs, F3 pass 21: Noroxus the
green three-headed hydra of eqoa.live's wiki picture, Bertoxxulous his own
body, the Twins the green dragon), every size, the banker's name, the
jellies', piranhas' and wasps' names, the portals' names and that they need
only a level (the live game's flag quest, its Queen and Noroxus quests and
Bert's scepter are not in). Not
placed: the sanitorium's staff (Fatima, Rebo, Medelonia, the Last House's
physician and pharmacist, the orderlies), who belong to the flag quest and
its building, and the rare spawns' placeholders and the quests' own spawns
(the bosses stand always up: this server spawns nothing by event).

    tools/seed_pod.py --ground worlds/ground [--db eqoa.db]            report
    tools/seed_pod.py --ground worlds/ground --apply                     write it
    tools/seed_pod.py --undo                                             take it all back

Rows go in in one transaction with npc_id 0 (no loot table: this server has
none, so the guides' "PoD Drop" spells drop nowhere yet), logged in
pod_seed_npcs; the rules the creatures need (npcundead, npcmove, npcaggro)
are written where no row of that name exists, logged in pod_seed_rules, and
--undo deletes exactly what a run wrote (a GM's own rows are never touched).
The plan is refused when a ground file differs from the one its anchors were
read off. A copy of the database is made first (--no-backup to skip). Stop
the server to apply, start it after. A database seeded before F3 pass 21
(Noroxus by the cauldron, the three bosses in borrowed bodies) is brought to
this plan by tools/fix_pod_bosses.py.
"""
import argparse
import collections
import datetime
import hashlib
import json
import math
import os
import sqlite3
import struct
import sys

# the ground reader is seed_wildlife's (the server's own format)
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from seed_wildlife import Ground  # noqa: E402

DEFAULT_DB = "eqoa.db"
DEFAULT_GROUND = "worlds/ground"
SOURCE = "pod-seed"
SPREAD, STEP, SPACING, NEAR = 30.0, 2.0, 5.0, 6.0

# npc_type: 0 hostile by nature; 0x80 not attackable; 0x02 banker; 0x100 travel
HOSTILE, FRIEND, BANKER, TRAVEL = 0, 0x80, 0x82, 0x180

# Each entry: an anchor (x, z) and the height of the floor meant there (y: of
# the floors at the anchor, the nearest; every spot of a group within NEAR of
# it, dry or in a pool as asked, at most SPREAD off), checked against the
# ground files when the plan is made. The anchors were read off the baked
# ground and the zone's actors (claude-notes/plane-of-disease-20261027.md):
# every one is on the canyon's floor (or a pool's), and reachable on foot from
# the arrival at the walk's 60-degree slope, but for the Queen's bowl, which
# nothing reaches but its portal (Noroxus's lobe, the north-west lake's, is
# joined to the ring only over ridges steeper than 31 degrees).
#
# body: the row (by name, lower case; model: of that name, the one with this
# CHAR.ESF body) every look column is copied from; its size times scale.
# wear: a body no row of this server has (a CHAR.ESF DictID), put in place of
# the copied row's own. Levels, counts and all the friendly NPCs' and
# portals' spots are ours. New entries go at the end: a row's default facing
# counts the rows before it, so one put in the middle turns every row after.
PLAN = [
    # ---- the Dead Hills (NW), the Rathe (world 1): the necrologists, outside
    # the tomb yard's fence on its west, where the tombs' doors face ("Log the
    # quest by speaking to Necrologist Zizclank and his assistant Necroloigst
    # Xeptip. Then you want to enter the mausoleum in the middle"); a gnome's
    # body, as this server's Necrologist Ipstok has
    dict(name="Necrologist Zizclank", body="necrologist ipstok", level=55, kind=TRAVEL, world=1,
         at=(4280, 6412), y=97.8, facing=64, role="sends a level 50 to PoD (the flag quest is not in)"),
    dict(name="Necrologist Xeptip", body="necrologist ipstok", level=52, kind=FRIEND, world=1,
         at=(4280, 6396), y=97.2, facing=64, role="Zizclank's assistant"),

    # ---- PoD, the Secrets world's "Last Home" (world 5, zone 0): the
    # arrival, on the zone's largest portal (actors 67DA0534, AE9501BC at 3.41,
    # E3B08E91) in the south lobe; facing north, the way in
    dict(name="Portal to the Dead Hills", body="dark portal", level=55, kind=TRAVEL, world=5,
         at=(5063, 3881), y=-25.3, facing=128, role="the way back to the Dead Hills"),
    # "Near the portal where you come to PoD, across the first pond, is a
    # series of tents. In one, you'll find Tuskmaster Helgley"; "go to the
    # north side of the lake and talk to Bonefisher Chklurgl inside the tent"
    # (a camp's props north-east of the pond)
    dict(name="Tuskmaster Helgley", body="angry patron", level=52, kind=FRIEND, world=5,
         at=(5140, 3405), y=-25.3, facing=0, role="plague swine tusks (hunt and gather, not in)"),
    dict(name="Bonefisher Chklurgl", body="goblin witchdoctor", level=52, kind=FRIEND, world=5,
         at=(5168, 3425), y=-25.3, facing=0, role="the Queen's quest (not in)"),
    # the Queen "inside whirling portal within the lake in entrance area"; it
    # stands on the pond's south bank here, its arrivals south of it, dry
    dict(name="Whirling Portal", body="dark portal", level=60, kind=TRAVEL, world=5,
         at=(5040, 3662), y=-26.8, facing=0, role="to the Queen's bowl, level 60 (live: the stinger quest)"),
    dict(name="Witchdoctor Muglchlur", body="goblin witchdoctor", level=55, kind=FRIEND, world=5,
         at=(5085, 3662), y=-29.6, facing=192, role="\"an npc near the Queen's portal\" (the scepter, not in)"),
    # "roaming scourge rats on both sides of the tunnel in the entrance area
    # (even deep into Fleshripper's valley)"
    dict(name="scourge rat", body="bile rat", level=55, kind=HOSTILE, world=5,
         at=(4800, 3480), y=-28.8, count=4, role="the entrance side of the first tunnel"),
    dict(name="scourge rat", body="bile rat", level=55, kind=HOSTILE, world=5,
         at=(4700, 3420), y=-21.7, count=3, role="the entrance side of the first tunnel"),

    # ---- "go north and take the first tunnel on your left": the west arc,
    # the banker inside the very first tunnel, Lich Overlord "behind tree next
    # to banker", Pluxklez "in the same room" (the banker's name is ours)
    dict(name="Plague Banker", body="banker ligs", level=50, kind=BANKER, world=5,
         at=(4300, 3150), y=-23.5, facing=64, role="the banker in the first tunnel (its name is ours)"),
    dict(name="Pluxklez", body="enthralled goblin", level=52, kind=FRIEND, world=5,
         at=(4312, 3170), y=-28.6, facing=64, role="scuzz toad eyes (not in)"),
    dict(name="Lich Overlord", body="blade swarm", level=60, kind=HOSTILE, world=5, scale=1.6,
         at=(4320, 3200), y=-26.4, roam=0, undead=True,
         role="rare (\"looks like huge swarming blades\"): fetid rotting organ"),

    # ---- Fleshripper's valley, up the slime river: scourge rats, Rallius
    # ("anywhere within the two valleys connected by the very first tunnel"),
    # plague swine, Fleshripper, Honcho Oozlgurgle "@ the end of the valley"
    dict(name="scourge rat", body="bile rat", level=55, kind=HOSTILE, world=5,
         at=(4340, 2860), y=-20.0, count=4, role="deep into Fleshripper's valley"),
    dict(name="Rallius Rattican", body="ratman channeler", level=58, kind=HOSTILE, world=5, scale=1.3,
         at=(4340, 2800), y=-19.9, roam=0, role="rare (\"big rat dressed in robe\"): fetid rotting femur"),
    dict(name="plague swine", body="dread rot", level=56, kind=HOSTILE, world=5,
         at=(4360, 2720), y=-18.9, count=6, role="Fleshripper's valley (a rotting boar's body)"),
    dict(name="Fleshripper", body="buzzard", level=60, kind=HOSTILE, world=5, scale=3.0,
         at=(4380, 2600), y=-19.4, roam=0, role="rare (\"huge flying vulture\"): fetid rotting skin"),
    dict(name="Honcho Oozlgurgle", body="enthralled goblin", level=55, kind=FRIEND, world=5,
         at=(4520, 2460), y=-18.9, facing=192, role="Fleshripper's quest (not in)"),

    # ---- "after the second tunnel, you head east, the first lake you come
    # to at the end of the river is where he spawns, Location1": the north
    # pool; Gluboxious "huge scuzz toad" (no toad on this server: a froglok's
    # body), jellies and piranhas (their names ours); Location2 east of it
    dict(name="Gluboxious the Fetid", body="froglok warrior", level=61, kind=HOSTILE, world=5, scale=2.5,
         at=(5040, 2340), y=-49.9, liquid=True, roam=0, water="water",
         role="rare: fetid rotting tibia (a froglok's body: no toad on this server)"),
    dict(name="plague jelly", body="green jelly", level=55, kind=HOSTILE, world=5, water="water",
         at=(5040, 2340), y=-49.9, liquid=True, count=4, role="the north pool"),
    dict(name="plague piranha", body="piranha", level=55, kind=HOSTILE, world=5, water="water",
         at=(5040, 2340), y=-49.9, liquid=True, count=4, role="the north pool"),
    dict(name="plague jelly", body="green jelly", level=55, kind=HOSTILE, world=5, water="water",
         at=(5420, 2660), y=-49.9, liquid=True, count=3, role="the second lake, further east"),

    # ---- the nox pools ("Last Home E"): Rotwood "end of valley just before
    # nox pools" among the deadwood arborians ("farm yellow/red"), Rawgl and
    # the viscera scavengers "surrounding nox pools", Honcho Ulgurglch
    # "almost all the way around the circle clockwise", Yarfsnoogle "across
    # the valley, straight south of Honcho" (each at a camp's props).
    # Noroxus is not at the pools: "Take the combined item to the big cauldron
    # in the middle of all 3 pools to open fuming portal ... This leads to
    # Noroxus" (the Fuming Portal, at the end of the plan), into the north-west
    # lake's lobe (the wiki map's; the island's ring of stones is its pad),
    # on its floor's dry north end, clear of its rocks and trees by 13 all
    # along his body, the lake to his east and the raid's landing by it to
    # his south-east ("All necs stack down by the rocks near the water (to
    # the S/E of nox)"), the lobe his and his raid's alone, facing the way
    # the portal brings the raid in. His body is
    # the green three-headed hydra of eqoa.live's wiki picture of him (red
    # crests; CHAR.ESF's other three hydras are tan, grey and red), at 1.1:
    # 14 tall, twice the dead tree (8.5) beside him in that picture, eight
    # men; the cave hydra's row gives the rest of his look, as hydras have it
    dict(name="deadwood arborian", body="deadwood", level=60, kind=HOSTILE, world=5,
         at=(5500, 2860), y=-25.4, count=5, role="before the nox pools"),
    dict(name="Rotwood the Hollow", body="deadwood", level=61, kind=HOSTILE, world=5, scale=1.6,
         at=(5480, 2830), y=-28.8, roam=0, role="rare: fetid rotting skull"),
    dict(name="viscera scavenger", body="chaos hound", level=58, kind=HOSTILE, world=5,
         at=(5640, 3100), y=-31.8, count=5, role="surrounding the nox pools"),
    dict(name="Rawgl the Carnage Hound", body="chaos hound", level=61, kind=HOSTILE, world=5, scale=1.6,
         at=(5540, 3080), y=-30.7, roam=0, role="rare: fetid rotting skull x2"),
    dict(name="Noroxus", body="cave hydra", wear=0xA18F3F22, level=65, kind=HOSTILE, world=5, scale=1.1,
         at=(4173, 2072), y=-21.4, facing=46, roam=0,
         role="boss (\"Guild needed\"): fetid rotting gland; in his arena, through the Fuming Portal"),
    dict(name="Honcho Ulgurglch", body="enthralled goblin", level=55, kind=FRIEND, world=5,
         at=(5585, 2905), y=-25.3, facing=0, role="Noroxus's quest (not in)"),
    dict(name="Yarfsnoogle", body="necrologist ipstok", level=55, kind=FRIEND, world=5,
         at=(5690, 3140), y=-21.7, facing=128, role="the PoD spell quest (not in)"),

    # ---- the Twins and Bertoxxulous, "through the same portal", in the
    # middle valley, as eqoa.live's wiki map has them (its central oval: Bert
    # and his throne at the top, past the pool, the Twins left and right in
    # the middle, a portal pad at the bottom). The valley, sunken in the
    # hills, is a crescent slime pool (the "moat") round the north island
    # with the throne (15681FC3, facing south), a floor south of it and a
    # portal pad at its south end, one of the zone's four (EC4AB525: the
    # arrival's, this one, the north-west lake's island and that lobe's
    # south-west pocket, each in one of two ground fogs, E3B08E91 or
    # FFC8B09E). The Bloody Portal is the guide's Closed Portal: "head all
    # the way around the circle past Honcho Ulgurglch to the Closed Portal",
    # clockwise from the arrival, so on the ring's south-east arc, where the
    # wiki map draws a structure and the zone has its slime fall (a slime pool
    # on a raised rock pouring down between two pillars, the wiki's PoD
    # screenshot 7): in front of it, facing north, a level 60's way to the
    # valley's pad, where the Portal of the Twins (moved from the north-west
    # pocket, whose pad is left empty) sends players back. Bert stands
    # between the two standing stones before the throne, facing the moat
    # ("You get hit with a disease spell just swimming across the lake
    # towards him"); the Twins "spawn to the left and the right, above the
    # rocks, near the moat", Uzrathixius west and Poxathixius east as the map
    # has them, facing the pad, 130 apart and 103 from it; the patrons (on no
    # map) on Bert's island, three each side of him, 59-63 off, past his
    # reach on sight (30) and their wander (20). Always up (no spawn by event
    # here). Bert wears his own body (jib: "He has his own dedicated unique
    # player model"; hooded, red-faced, a round belly in a tattered black
    # vest; no row has it), at 1.3 (3.85 tall, a little over twice a man, as
    # a screenshot of him in a fight shows him); the ghoul reaver's row gives
    # the rest of his look. Not a players' race's body, he attacks on sight
    # by nature (entity/aggro.go): no npcaggro rule. The Twins wear the green
    # dragon (Cyrcalis's row; eqoa.live's wiki picture of Uzrathixius), at 1.5
    dict(name="Bloody Portal", body="dark portal", level=60, kind=TRAVEL, world=5,
         at=(5243, 3175), y=-25.1, facing=128, role="to the Twins' valley, level 60 (live: Noroxus's blood)"),
    dict(name="Portal of the Twins", body="dark portal", level=60, kind=TRAVEL, world=5,
         at=(4872, 2987), y=-19.9, facing=128, role="back to the Bloody Portal"),
    dict(name="Bertoxxulous the Plaguebringer", body="ghoul reaver", wear=0xA6320E74, level=66,
         kind=HOSTILE, world=5, scale=1.3, at=(4842, 2750), y=-23.6, facing=0, roam=0, undead=True,
         role="boss (\"Hard, full guild needed\"): soul dust x2"),
    dict(name="Poxathixius", body="cyrcalis", level=65, kind=HOSTILE, world=5,
         scale=1.5, at=(4937, 2904), y=-22.1, facing=229, roam=0, role="one of the Twins"),
    dict(name="Uzrathixius", body="cyrcalis", level=65, kind=HOSTILE, world=5,
         scale=1.5, at=(4807, 2904), y=-23.6, facing=27, roam=0, role="one of the Twins"),
    dict(name="Patron of Bertoxxulous", body="decaying zombie", level=59, kind=HOSTILE, world=5, undead=True,
         at=(4783, 2728), y=-25.7, count=3, radius=20.0, role="hunt and gather: zombie patron skins"),
    dict(name="Patron of Bertoxxulous", body="decaying zombie", level=59, kind=HOSTILE, world=5, undead=True,
         at=(4888, 2713), y=-24.0, count=3, radius=20.0, role="hunt and gather: zombie patron skins"),

    # ---- the weapon quest's pocket of seven pools, north-east: "After exiting
    # this tunnel, right below is the NPC Witchdoctor Yurplag"; larvae ("con db
    # and lb at 60"), their monarchs and matriarchs (the mid-level spawns
    # between are left out: no names)
    dict(name="Witchdoctor Yurplag", body="goblin witchdoctor", level=55, kind=FRIEND, world=5,
         at=(5680, 2380), y=5.5, facing=32, role="the PoD weapon quest (not in)"),
    dict(name="moth larva", body="flesh maggot", level=53, kind=HOSTILE, world=5,
         at=(5760, 2300), y=-19.9, count=3, role="the weapon quest's larvae"),
    dict(name="locust larva", body="flesh maggot", level=53, kind=HOSTILE, world=5,
         at=(5820, 2320), y=-22.8, count=3, role="the weapon quest's larvae"),
    dict(name="mantis larva", body="flesh maggot", level=53, kind=HOSTILE, world=5,
         at=(5700, 2300), y=-21.7, count=3, role="the weapon quest's larvae"),
    dict(name="millipede larva", body="flesh maggot", level=53, kind=HOSTILE, world=5,
         at=(5800, 2400), y=-35.2, count=3, role="the weapon quest's larvae"),
    dict(name="Moth Monarch", body="great wasp", model=0xC27D1633, level=62, kind=HOSTILE, world=5,
         scale=1.8, at=(5766, 2186), y=-35.5, roam=0, role="wax dust"),
    dict(name="Locust Monarch", body="firefly swarm", level=62, kind=HOSTILE, world=5, scale=2.0,
         at=(5860, 2300), y=-23.5, roam=0, role="acidic blood bile (a swarm's body)"),
    dict(name="Mantis Matriarch", body="death beetle", level=62, kind=HOSTILE, world=5, scale=1.8,
         at=(5720, 2340), y=-24.1, roam=0, role="front legs"),
    dict(name="Millipede Matriarch", body="deadly centipede", level=62, kind=HOSTILE, world=5, scale=2.0,
         at=(5820, 2440), y=-27.9, roam=0, role="mandibles"),

    # ---- the south-east bowl, which no walk reaches: the Queen's ("inside
    # whirling portal"; "be ported to the wasp hive")
    dict(name="Portal to the Lake", body="dark portal", level=60, kind=TRAVEL, world=5,
         at=(5640, 3760), y=-20.6, facing=64, role="back to the Whirling Portal"),
    dict(name="Queen Qy'tha", body="giant wasp", level=63, kind=HOSTILE, world=5, scale=2.5,
         at=(5725, 3745), y=-21.1, roam=0, role="boss: fetid rotting eyeball, the stinger"),
    dict(name="plague wasp", body="giant wasp", level=58, kind=HOSTILE, world=5,
         at=(5770, 3770), y=-19.9, count=4, roam=0, role="the Queen's (the name is ours)"),

    # ---- Noroxus's way in (F3 pass 21, jib: "Noroxus is currently spawning at
    # his portal entrance, not his arena"): the Fuming Portal where he stood,
    # at the cauldron's east side, facing away from it ("the big cauldron in
    # the middle of all 3 pools ... Once the portal is open, all people with a
    # skin can go through"; live: the hearts' item opens it, here level 60, as
    # the other bosses' portals), and the way back on his lobe's dry shore,
    # 59 from him, its arrivals landing on its west side, 56 from him (the
    # island's pad stands in the middle of the lake)
    dict(name="Fuming Portal", body="dark portal", level=60, kind=TRAVEL, world=5,
         at=(5620, 3060), y=-30.5, facing=54, role="to Noroxus's lair, level 60 (live: the slime hearts' item)"),
    dict(name="Portal to the Cauldron", body="dark portal", level=60, kind=TRAVEL, world=5,
         at=(4226, 2097), y=-24.2, facing=174, role="back to the Fuming Portal"),
]


# ---- the zone BSPs' flags (internal/ground/env.go's format) ---------------------

LEAF_BIT, INDEX_MASK = 0x40000000, 0x0FFFFFFF


class Env:
    """A world's environment file: "EQEV", version 1, the world tree (node and
    leaf counts, each node nx ny nz d as float32 and front, back as uint32, a
    leaf with LeafBit set; each leaf a zone index), the zone count, and each
    zone's tree the same with flags for leaves (1 water, 2 slime, 4 lava, 8 mud)."""

    def __init__(self, path):
        data = open(path, "rb").read()
        if data[:4] != b"EQEV":
            raise ValueError(f"{path}: not an environment file")
        (version,) = struct.unpack_from("<I", data, 4)
        if version != 1:
            raise ValueError(f"{path}: version {version}, this tool reads 1")
        self.p = 8
        self.data = data
        self.tree = self._tree()
        (zones,) = self._u32s(1)
        self.zones = [self._tree() for _ in range(zones)]

    def _u32s(self, n):
        v = struct.unpack_from(f"<{n}I", self.data, self.p)
        self.p += 4 * n
        return v

    def _tree(self):
        nodes, leaves = self._u32s(2)
        ns = [struct.unpack_from("<4f2I", self.data, self.p + 24 * i) for i in range(nodes)]
        self.p += 24 * nodes
        ls = list(self._u32s(leaves)) if leaves else []
        return ns, ls

    @staticmethod
    def _leaf(tree, x, y, z):
        nodes, leaves = tree
        at = 0
        if nodes:
            for _ in range(len(nodes) + 1):
                i = at & INDEX_MASK
                if i >= len(nodes):
                    return None
                nx, ny, nz, d, front, back = nodes[i]
                at = front if x * nx + y * ny + z * nz + d >= 0 else back
                if at & LEAF_BIT:
                    break
            else:
                return None
            at &= INDEX_MASK
        return leaves[at] if at < len(leaves) else None

    def flags(self, x, y, z):
        zi = self._leaf(self.tree, x, y, z)
        if zi is None or zi >= len(self.zones):
            return 0
        f = self._leaf(self.zones[zi], x, y, z)
        return f or 0


# ---- the plan ---------------------------------------------------------------------

def body_key(e):
    return (e["body"], e.get("model"))


def bodies(db):
    """The first row of each body (a name, lower case, and its model where
    the entry names one)."""
    out = {}
    for name, model in sorted({body_key(e) for e in PLAN}, key=lambda k: (k[0], k[1] or 0)):
        if model is None:
            r = db.execute("SELECT * FROM npcs WHERE lower(trim(npc_name)) = ? ORDER BY id LIMIT 1", (name,))
        else:
            r = db.execute("SELECT * FROM npcs WHERE lower(trim(npc_name)) = ? AND modelid = ? ORDER BY id LIMIT 1",
                           (name, model))
        r = r.fetchone()
        if r is not None:
            out[(name, model)] = dict(r)
    return out


def anchor_floor(g, at, y):
    """Of the floors at the anchor, the one nearest y (None: no floor)."""
    fs = g.floors(*at)
    return min(fs, key=lambda h: abs(h - y)) if fs else None


def spots(g, env, at, base, liquid, spread):
    """The anchor's ring of candidate spots, nearest first: (x, y, z) on a
    floor within NEAR of base whose zone leaf is liquid (or not) as asked."""
    ax, az = at
    seen = set()
    r = 0.0
    while r <= spread:
        n = max(1, int(2 * math.pi * r / STEP))
        for k in range(n):
            a = 2 * math.pi * k / n
            x, z = round(ax + r * math.cos(a), 1), round(az + r * math.sin(a), 1)
            if (x, z) in seen:
                continue
            seen.add((x, z))
            for h in g.floors(x, z):
                if abs(h - base) > NEAR:
                    continue
                wet = env is not None and env.flags(x, h + 0.5, z) & 3 != 0
                if wet == liquid:
                    yield x, h, z
                    break
        r += STEP


def plan(db, ground_dir):
    rows, missing, unplaced, off = [], [], [], []
    body = bodies(db)
    grounds, envs = {}, {}
    taken = collections.defaultdict(list)
    for e in PLAN:
        b = body.get(body_key(e))
        if b is None:
            missing.append(e["body"] + ("" if e.get("model") is None else f" ({e['model']:08X})"))
            continue
        w = e["world"]
        if w not in grounds:
            grounds[w] = Ground(os.path.join(ground_dir, f"{w}.ground"))
            ep = os.path.join(ground_dir, f"{w}.env")
            envs[w] = Env(ep) if os.path.exists(ep) else None
        base = anchor_floor(grounds[w], e["at"], e["y"])
        if base is None or abs(base - e["y"]) > 1.0:
            # the ground is not the one the anchor was read off
            off.append(f"{e['name']} at {e['at']}: floor {base}, planned {e['y']}")
            continue
        for i in range(e.get("count", 1)):
            placed = None
            spread = SPREAD if e.get("count", 1) > 1 else NEAR
            for x, y, z in spots(grounds[w], envs[w], e["at"], base, e.get("liquid", False), spread):
                if all((x - px) ** 2 + (z - pz) ** 2 >= SPACING ** 2 for px, pz in taken[w]):
                    placed = (x, y, z)
                    break
            if placed is None:
                unplaced.append(e["name"])
                continue
            x, y, z = placed
            taken[w].append((x, z))
            row = dict(b)
            row.pop("id", None)
            row.update(npc_name=e["name"], npc_level=e["level"], npc_type=e["kind"], world=w,
                       x_coord=x, y_coord=round(y, 3), z_coord=z,
                       facing=e.get("facing", (37 * (len(rows) + 1)) % 256),
                       size=round(float(b.get("size") or 1.0) * e.get("scale", 1.0), 3),
                       npc_id=0, dt_kill=None, zone=0)
            if e.get("wear") is not None:
                row["modelid"] = e["wear"]
            look = e["body"] if e.get("wear") is None else f"{e['body']}, wearing {e['wear']:08X}"
            rows.append(dict(row=row, body=look, body_row=b["id"], role=e["role"]))
    rules = []
    seen = set()
    for e in PLAN:
        key = e["name"].strip().lower()
        if key in seen:
            continue
        seen.add(key)
        if e.get("undead"):
            rules.append(("npcundead", "name", key, dict(name=key, undead=1, note="PoD (guides)", source=SOURCE)))
        if "roam" in e or "water" in e or "radius" in e:
            rules.append(("npcmove", "name", key, dict(name=key, roam=e.get("roam", 1), radius=e.get("radius", 0.0),
                                                       water=e.get("water", ""), note="PoD", source=SOURCE)))
        if e.get("aggro"):
            # npcaggro is by the NPC's own name (entity/aggro.go)
            rules.append(("npcaggro", "npc_name", e["name"], dict(npc_name=e["name"], aggressive=1, radius=0.0,
                                                                   note="PoD: a human's body", source=SOURCE)))
    cols = [r[1] for r in db.execute("PRAGMA table_info(npcs)")]
    return dict(rows=rows, rules=rules, missing=sorted(set(missing)), unplaced=unplaced, off=off, cols=cols)


# ---- apply and undo ---------------------------------------------------------------

LOG_DDL = [
    "CREATE TABLE pod_seed_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)",
    "CREATE TABLE pod_seed_npcs (npc_row INTEGER PRIMARY KEY, name TEXT NOT NULL, body_row INTEGER NOT NULL, "
    "role TEXT NOT NULL)",
    "CREATE TABLE pod_seed_rules (tbl TEXT NOT NULL, keycol TEXT NOT NULL, name TEXT NOT NULL, "
    "PRIMARY KEY (tbl, name))",
]


def seeded(db):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE name = 'pod_seed_npcs'").fetchone()[0] > 0


def has_table(db, t):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone()[0] > 0


def npc_seq(db):
    r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'npcs'").fetchone()
    return None if r is None else r[0]


def rows_digest(db, table="npcs"):
    h = hashlib.md5()
    for r in db.execute(f"SELECT * FROM {table} ORDER BY 1"):
        h.update(repr(tuple(r)).encode())
    return h.hexdigest()


def apply(db, p):
    if seeded(db):
        sys.exit("this database already holds the PoD seed (pod_seed_npcs): --undo it first")
    if p["missing"] or p["unplaced"] or p["off"]:
        sys.exit(f"not written: bodies missing {p['missing']}, unplaced {p['unplaced']}, off {p['off']}")
    cols = [c for c in p["cols"] if c != "id"]
    db.execute("BEGIN IMMEDIATE")
    try:
        for stmt in LOG_DDL:
            db.execute(stmt)
        meta = dict(when=datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds"),
                    seq_before="" if npc_seq(db) is None else str(npc_seq(db)), rows=str(len(p["rows"])))
        for r in p["rows"]:
            cur = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                             [r["row"].get(c) for c in cols])
            db.execute("INSERT INTO pod_seed_npcs VALUES (?, ?, ?, ?)",
                       (cur.lastrowid, r["row"]["npc_name"], r["body_row"], r["role"]))
        wrote = 0
        for tbl, keycol, key, vals in p["rules"]:
            if not has_table(db, tbl):
                continue
            if db.execute(f"SELECT count(*) FROM {tbl} WHERE {keycol} = ?", (key,)).fetchone()[0]:
                continue  # a GM's (or another seed's) row stands
            names = list(vals)
            db.execute(f"INSERT INTO {tbl} ({', '.join(names)}) VALUES ({', '.join('?' * len(names))})",
                       [vals[n] for n in names])
            db.execute("INSERT INTO pod_seed_rules VALUES (?, ?, ?)", (tbl, keycol, key))
            wrote += 1
        meta["rules"] = str(wrote)
        db.executemany("INSERT INTO pod_seed_meta VALUES (?, ?)", sorted(meta.items()))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return wrote


def undo(db):
    if not seeded(db):
        sys.exit("no PoD seed to undo here")
    meta = dict(db.execute("SELECT key, value FROM pod_seed_meta"))
    db.execute("BEGIN IMMEDIATE")
    try:
        n = db.execute("DELETE FROM npcs WHERE id IN (SELECT npc_row FROM pod_seed_npcs)").rowcount
        for tbl, keycol, key in db.execute("SELECT tbl, keycol, name FROM pod_seed_rules").fetchall():
            if has_table(db, tbl):
                db.execute(f"DELETE FROM {tbl} WHERE {keycol} = ? AND source = ?", (key, SOURCE))
        before = meta.get("seq_before", "")
        top = db.execute("SELECT max(id) FROM npcs").fetchone()[0] or 0
        if before == "":
            if top == 0:
                db.execute("DELETE FROM sqlite_sequence WHERE name = 'npcs'")
        elif top <= int(before):
            db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = 'npcs'", (int(before),))
        for t in ("pod_seed_rules", "pod_seed_npcs", "pod_seed_meta"):
            db.execute(f"DROP TABLE {t}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return n


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-podseed"
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def report(p, verbose=False):
    by = collections.Counter(r["row"]["npc_name"] for r in p["rows"])
    worlds = collections.Counter(r["row"]["world"] for r in p["rows"])
    print(f"{len(p['rows'])} NPCs ({len(by)} names): {dict(sorted(worlds.items()))} by world; "
          f"{len(p['rules'])} rules (npcundead, npcmove, npcaggro)")
    if p["missing"]:
        print("bodies missing (no row of that name):", ", ".join(p["missing"]))
    if p["unplaced"]:
        print("no spot found for:", ", ".join(p["unplaced"]))
    for o in p["off"]:
        print("the ground differs:", o)
    for r in p["rows"] if verbose else []:
        row = r["row"]
        print(f"  {row['npc_name']:32} L{row['npc_level']:<3} w{row['world']} ({row['x_coord']:.1f}, "
              f"{row['y_coord']:.1f}, {row['z_coord']:.1f}) f{row['facing']:<3} type {row['npc_type']:<3} "
              f"size {row['size']:.2f} body {r['body']}")
    if not verbose:
        for name, n in sorted(by.items()):
            print(f"  {name} x{n}" if n > 1 else f"  {name}")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--ground", default=DEFAULT_GROUND, help="the ground files' folder (worlds/ground)")
    ap.add_argument("--apply", action="store_true", help="write the NPCs and their rules")
    ap.add_argument("--undo", action="store_true", help="delete everything a seed wrote")
    ap.add_argument("--no-backup", action="store_true", help="write without a copy first")
    ap.add_argument("--verbose", action="store_true", help="list every NPC and its spot")
    ap.add_argument("--json", help="write the plan to this file")
    args = ap.parse_args()
    db = sqlite3.connect(args.db)
    db.row_factory = sqlite3.Row
    if args.undo:
        if not args.no_backup:
            print("backup:", backup(db, args.db))
        print(f"deleted {undo(db)} NPCs and the rules the seed wrote")
        return
    p = plan(db, args.ground)
    report(p, args.verbose)
    if args.json:
        with open(args.json, "w") as f:
            json.dump([dict(r["row"], body=r["body"], role=r["role"]) for r in p["rows"]], f, indent=1, default=str)
    if args.apply:
        if not args.no_backup:
            print("backup:", backup(db, args.db))
        print(f"wrote {len(p['rows'])} NPCs and {apply(db, p)} rules; restart the server")
    else:
        print("(report only: --apply to write it)")


if __name__ == "__main__":
    main()
