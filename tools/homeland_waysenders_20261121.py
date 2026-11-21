#!/usr/bin/env python3
"""Waysenders home: one in each custom race's mainland city who sends a player of that race back to its homeland
(the owner, 2026-11-21: "Build waysenders"; claude-notes/race-homelands-20261108.md, the owner's decisions, item 4).

For each race of racedefs with a home in world 10 (tools/worlds/homelands.json) and its gatekeeper's flight
(homelands/<race>/flight, tools/quests/gen_homelands.py):

- an NPC, "Waysender <name>", stands in the city the flight lands it in (its wire race's start row: the warrior's,
  where the flight sets down most classes; in Neriak, Tethelin and Klick'Anon another class's, SPOTS below), a few
  steps off that landing, facing it, on the floor. A copy of the
  home's gatekeeper's row (its body, size, gear and level), npc_type 128 (talked to, never fought, stays put); its
  zone columns its nearest neighbour's. Each spot was picked on the server's baked ground (cmd/campground: the floor
  within 1.5 of the landing's height, not embedded, no water, nothing walled between it and the landing, roofed as
  the landing is) and 5 or more from every NPC and every other waysender: SPOTS below, its answers beside it.
- a quest of its, repeatable, for that race alone (`races` its racedefs id: handlers/quests.go questRaceTakes asks a
  race of racedefs of the character's own race, never its wire race), needing the flight finished (`requires`), one
  talk step done on the answer (`dostep`) whose done teleports the player to the home's start (its race's start
  rows in world 10: where a new character of the race starts, in front of its elder), WITHOUT bind: the spirit stays
  where it was bound. No fee, no reward (the flight has none).

Nothing else is changed. A race whose racedefs row, flight quest, homeland start or gatekeeper this database lacks
is left out, and said (the Live shard has no homelands). Refused for a race (nothing written for it) when its city's
landing is not where the spot was picked round, the spot is crowded since, or its name is someone's already.

A report by default; --apply writes in one transaction, a gm_audit row a row, the rows written logged in
homeland_waysenders_log (each with its row as written); a second run finds them and writes nothing. --undo takes back
what the log holds (and the characters' marks of the quests), refused for a row a GM has changed since.

WHEN IT TAKES EFFECT: npcs and quests are read at the server's start: a keep restart's before.sh.

    tools/homeland_waysenders_20261121.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import math
import sqlite3
import sys

LOG = "homeland_waysenders_log"
DETAIL = "the owner, 2026-11-21: \"Build waysenders\": a way home from each custom race's city: tools/homeland_waysenders_20261121.py"
WHO, VIA = "claude", "tool"
SOURCE = "ours: tools/homeland_waysenders_20261121.py"
TALKER = 128
HOMELANDS = 10
MONK = 4
LANDING_SLACK = 0.5

# Per race (racedefs id): its quests' key, its waysender, the city, the landing the spot was picked round (world, x, y,
# z of the wire race's warrior start row), the spot (world, x, y, z), and the quest's words. The ground's answers
# (campground, the box's ground files of 2026-11-21) are in GROUND.
RACES = {
    11: dict(slug="hillgiant", waysender="Waysender Hrumgar", city="Oggok", home="West Plateau", elder="Elder Hrolmog",
             quest="The Stone Way Back",
             offer="The ogres keep you well, but the plateau remembers you. I can open the stone way back to Elder Hrolmog.",
             ask="Shall I send you home to the West Plateau?", declined="The plateau will keep. Stone does.",
             started="Hold your breath, little giant!"),
    12: dict(slug="kerran", waysender="Waysender Mrrazha", city="Qeynos", home="Kerra Isle", elder="Elder Ssharu",
             quest="Back Across the Water",
             offer="You smell of Qeynos now, cub, but the isle still knows you. I can send you back across the water to Elder Ssharu.",
             ask="Shall I send you home to Kerra Isle?", declined="The tide will wait.",
             started="Keep your fur dry!"),
    13: dict(slug="goblin", waysender="Waysender Grizzik", city="Neriak", home="Moss Mouth Cavern", elder="Elder Snikkit",
             quest="Back to Moss Mouth",
             offer="Tired of dark elves looking down their noses? I can send you back to the cavern, to Elder Snikkit. No charge. Don't tell anyone.",
             ask="Shall I send you home to Moss Mouth Cavern?", declined="Suit yourself. The tunnels aren't going anywhere.",
             started="Mind your purse on the way!"),
    14: dict(slug="aviak", waysender="Waysender Skyquill", city="Halas", home="Aviak Village", elder="Elder Kraawk",
             quest="The Long Flight Home",
             offer="The winds blow south as well as north, fledgling. I can send you back to the Roost, to Elder Kraawk.",
             ask="Shall I send you home to Aviak Village?", declined="The winds will wait for you.",
             started="Fly well! Greet the Roost for me."),
    15: dict(slug="lizardman", waysender="Waysender Sseskal", city="Freeport", home="Cazic-Thule", elder="Elder Hsslarn",
             quest="Back to the Mire",
             offer="Freeport's stone is too dry for our scales. I can send you back to the mire, to Elder Hsslarn.",
             ask="Shall I send you home to Cazic-Thule?", declined="The mire will keep.",
             started="Ssswim well!"),
    16: dict(slug="kappa", waysender="Waysender Tobbu", city="Rivervale", home="Sylhilthis' Dwell", elder="Elder Hoshuk",
             quest="Up the River",
             offer="Every river runs both ways, young one. I can send you back up it to the Dwell, to Elder Hoshuk.",
             ask="Shall I send you home to Sylhilthis' Dwell?", declined="The river will wait for you.",
             started="Keep your shell clean!"),
    17: dict(slug="brownie", waysender="Waysender Fernkettle", city="Rivervale", home="Mayfly Glade", elder="Elder Thimbleroot",
             quest="The Road Back to the Glade",
             offer="The halflings feed you well, but the glade misses you. I can send you back to Elder Thimbleroot in a blink.",
             ask="Shall I send you home to Mayfly Glade?", declined="The glade will keep. So will the pies.",
             started="Off you go! Bring back some honey."),
    18: dict(slug="pixie", waysender="Waysender Dewglimmer", city="Fayspire", home="Misty Thicket", elder="Elder Brightwing",
             quest="Back to the Thicket",
             offer="The elves' wood is grand, little spark, but the thicket is home. I can send you back to Elder Brightwing.",
             ask="Shall I send you home to Misty Thicket?", declined="The thicket will wait for you.",
             started="Fly well, little spark!"),
    19: dict(slug="fairy", waysender="Waysender Bellflower", city="Qeynos", home="Twisted Tower", elder="Elder Moonpetal",
             quest="The Road Back to the Tower",
             offer="Qeynos is so very slow, isn't it? I can send you back to the Tower, to Elder Moonpetal, quicker than a human can blink.",
             ask="Shall I send you home to the Twisted Tower?", declined="The tower will wait. It is good at that.",
             started="Fly true, little friend!"),
    20: dict(slug="gnoll", waysender="Waysender Rakkar", city="Neriak", home="Blackburrow", elder="Elder Grawl",
             quest="Back to the Burrow",
             offer="This city stinks of dark elf. Your nose wants the burrow, pup. I can send you back to Elder Grawl.",
             ask="Shall I send you home to Blackburrow?", declined="The burrow will keep.",
             started="Nose to the wind!"),
    21: dict(slug="orc", waysender="Waysender Gruzzak", city="Halas", home="Thedruk", elder="Elder Gorruk",
             quest="The Road Back to Thedruk",
             offer="Cold here, whelp. The camp's fires are warmer. I can send you back to Thedruk, to Elder Gorruk.",
             ask="Shall I send you home to Thedruk?", declined="The camp will still be there.",
             started="Fight well, and come back strong!"),
    22: dict(slug="kobold", waysender="Waysender Yipkrag", city="Klick'Anon", home="The Warrens", elder="Elder Grikkis",
             quest="Back Down the Deep Ways",
             offer="Too many gnomes, too much ticking. I can send you back down to the Warrens, to Elder Grikkis.",
             ask="Shall I send you home to the Warrens?", declined="The deep ways will wait for you.",
             started="Mind your tail!"),
    23: dict(slug="minotaur", waysender="Waysender Ironhorn", city="Grobb", home="Gerntar Mines", elder="Elder Hornbrand",
             quest="The Road Back to the Mines",
             offer="The trolls hit hard, but the mines are home. I can send you back to Gerntar, to Elder Hornbrand.",
             ask="Shall I send you home to Gerntar Mines?", declined="The mines will still be there.",
             started="Keep your horns sharp!"),
    24: dict(slug="ratman", waysender="Waysender Skrikk", city="Moradhim", home="The Vastly Deep", elder="Elder Skritch",
             quest="Out of the Deep Halls",
             offer="Dwarf halls, dwarf ale, dwarf boots. I can send you back to the shore, to Elder Skritch.",
             ask="Shall I send you home to the Vastly Deep?", declined="The shore will wait for you.",
             started="Keep your nose to the stone!"),
    25: dict(slug="satyr", waysender="Waysender Pellos", city="Fayspire", home="East Toxxulia", elder="Elder Pannoch",
             quest="The Glade Road Home",
             offer="The elves dance too slowly. I can send you back to the glades of Toxxulia, to Elder Pannoch.",
             ask="Shall I send you home to East Toxxulia?", declined="The road will wait for you.",
             started="Go with a light step!"),
    26: dict(slug="dryad", waysender="Waysender Willowmere", city="Qeynos", home="West Toxxulia", elder="Elder Silverbough",
             quest="Roots Back in the West",
             offer="Your roots reach a long way, sapling, all the way back to the grove. I can send you home to Elder Silverbough.",
             ask="Shall I send you home to West Toxxulia?", declined="The grove will wait for you.",
             started="Grow tall and strong!"),
    27: dict(slug="skeleton", waysender="Waysender Ashgrave", city="Freeport", home="Tomb of Kings", elder="Elder Ossric",
             quest="The Road Back to the Tomb",
             offer="The living crowd these streets. The tomb is quieter. I can send you back to Elder Ossric.",
             ask="Shall I send you home to the Tomb of Kings?", declined="The tomb is patient. So am I.",
             started="Keep your bones together!"),
    28: dict(slug="fungusman", waysender="Waysender Puffcap", city="Fayspire", home="Guk", elder="Elder Moldcap",
             quest="Down from the Light",
             offer="Too much light up here for a cap like yours. I can send you back down to the damp, to Elder Moldcap.",
             ask="Shall I send you home to Guk?", declined="There is no hurry. The damp will keep.",
             started="Grow well, down in the dark!"),
    29: dict(slug="gargoyle", waysender="Waysender Granitewing", city="Oggok", home="South Barren Coast", elder="Elder Gorrak",
             quest="Wings Back to the Crag",
             offer="Oggok stands, and you have guarded it well. I can send you back to the crag, to Elder Gorrak.",
             ask="Shall I send you home to the South Barren Coast?", declined="The crag will wait for you.",
             started="Stand firm!"),
    30: dict(slug="jalraeth", waysender="Waysender Saelvar", city="Highbourne", home="Envar", elder="Elder Ithrael",
             quest="The Way Back to Envar",
             offer="The scholars' city is loud with minds. Envar is quieter. I can send you back to Elder Ithrael.",
             ask="Shall I send you home to Envar?", declined="The door will wait.",
             started="Drift well, and keep your mind still!"),
    31: dict(slug="wisp", waysender="Waysender Glimmerlight", city="Qeynos", home="West Feerrott", elder="Elder Faintglow",
             quest="A Light Back Home",
             offer="Your light burns far from the swamp, little one. I can send you back to the Feerrott, to Elder Faintglow.",
             ask="Shall I send you home to West Feerrott?", declined="The swamp will hold your light a while longer.",
             started="Burn bright!"),
    32: dict(slug="mermaid", waysender="Waysender Nerissa", city="Highbourne", home="Erud's Crossing", elder="Elder Coralyn",
             quest="The Tide Home",
             offer="The tide turns, and so may you. I can send you back to the strand, to Elder Coralyn.",
             ask="Shall I send you home to Erud's Crossing?", declined="The tide will wait for you.",
             started="Swim well, and sing for us!"),
}
ACCEPT, DECLINE = "Send me home.", "Not now."
# CLIENT's room (tools/quests/gen_homelands.py): a box 217 with the speaker's name and ": ", a question 127, an answer
# 63, a journal line 127
BOX, ASK, ANSWER, LINE = 217, 127, 63, 127

# Where each stands (picked 2026-11-21 on the Heirloom database's NPCs and the server's baked ground, cmd/campground):
# `landing` is where the flight sets down a character of the race of class `cls` (database.MainlandPlace: its wire
# race's start row for the class), `spot` the waysender's (world, x, y, z) on the floor there, `room` how far every
# other NPC stands at least. A ring round the landing was searched, nearest first, in front of it first: the floor
# within 1.5 of the landing's height, not embedded (the move check's ball), no water, unwalled from the landing,
# roofed as the landing is, 3 or more from any start row. Most stand by the warrior's landing (where the flight sets
# down most classes); where that guild's floor is full (the Heirloom Outfitter, Equipment Dyer, Pet Groomer and Weapon
# Enchanter stand 2-4 apart round each landing) the room is 3.5, the herald's (tools/place_ctf_herald.py), and where
# even that found nothing (Neriak's, Tethelin's and Klick'Anon's warrior halls: raised floors full to their walls) the
# waysender stands at another class's landing in the city: Neriak's shadowknights', Fayspire's paladins',
# Klick'Anon's rogues'. `ground` is what the ground answered.
SPOTS = {
    11: dict(cls=0, landing=(1, 9377.67, 53.19, 7385.81), spot=(1, 9378.51, 53.25, 7391.75), room=5,
            ground='6 from the landing at +90 deg of its facing; check {"stand": 53.25, "floor": 53.25, "env": 0}; spot {"stand": 53.25, "env": 0}; nearest NPC Heirloom Outfitter 5.5'),
    12: dict(cls=0, landing=(0, 4243.7, 58.13, 17169.14), spot=(0, 4252.66, 58.13, 17168.32), room=5,
            ground='9 from the landing at -170 deg of its facing; check {"stand": 58.12754, "placed": true, "floor": 57.71875, "env": 0}; spot {"stand": 58.12754, "placed": true, "env": 0}; nearest NPC Heirloom Outfitter 5.6'),
    13: dict(cls=3, landing=(0, 24659.84, 5.77, 8836.48), spot=(0, 24637.05, 5.77, 8839.6), room=5,
            ground='23 from the landing at -150 deg of its facing; check {"stand": 5.765625, "placed": true, "floor": -0.21875, "env": 0}; spot {"stand": 5.765625, "placed": true, "env": 0}; nearest NPC ghoul servant 5.1'),
    14: dict(cls=0, landing=(0, 13059.32, 54.4, 4289.59), spot=(0, 13060.35, 54.4, 4280.65), room=5,
            ground='9 from the landing at +100 deg of its facing; check {"stand": 54.398438, "placed": true, "floor": 54.15625, "env": 0}; spot {"stand": 57.866695, "placed": true, "env": 0}; nearest NPC Merchant McAllistar 9.0'),
    15: dict(cls=0, landing=(0, 25486.54, 61.37, 15546.98), spot=(0, 25493.45, 61.36, 15542.95), room=3.5,
            ground='8 from the landing at -150 deg of its facing; check {"stand": 61.364063, "placed": true, "floor": 54.09375, "env": 0}; spot {"stand": 61.364063, "placed": true, "env": 0}; nearest NPC Equipment Dyer 4.7'),
    16: dict(cls=0, landing=(0, 18776.33, 62.38, 10959.55), spot=(0, 18773.33, 62.34, 10964.75), room=5,
            ground='6 from the landing at -30 deg of its facing; check {"stand": 62.34375, "floor": 62.34375, "env": 0}; spot {"stand": 62.34375, "env": 0}; nearest NPC Smithy Laryn 5.1'),
    17: dict(cls=0, landing=(0, 18776.33, 62.38, 10959.55), spot=(0, 18770.42, 62.34, 10960.59), room=5,
            ground='6 from the landing at -80 deg of its facing; check {"stand": 62.34375, "floor": 62.34375, "env": 0}; spot {"stand": 62.34375, "env": 0}; nearest NPC Marshall Drake 7.2'),
    18: dict(cls=2, landing=(0, 18985.54, 63.4, 6356.56), spot=(0, 18988.32, 63.4, 6351.24), room=5,
            ground='6 from the landing at -70 deg of its facing; check {"stand": 63.398445, "placed": true, "floor": 52.84375, "env": 0}; spot {"stand": 63.398445, "placed": true, "env": 0}; nearest NPC Weapon Enchanter 5.5'),
    19: dict(cls=0, landing=(0, 4243.7, 58.13, 17169.14), spot=(0, 4259.65, 58.52, 17170.47), room=5,
            ground='16 from the landing at +180 deg of its facing; check {"stand": 58.523438, "placed": true, "floor": 57.71875, "env": 0}; spot {"stand": 58.523438, "placed": true, "env": 0}; nearest NPC Lancet Carroll 5.2'),
    20: dict(cls=3, landing=(0, 24659.84, 5.77, 8836.48), spot=(0, 24646.51, 5.77, 8840.76), room=3.5,
            ground='14 from the landing at -140 deg of its facing; check {"stand": 5.765625, "placed": true, "floor": -0.21875, "env": 0}; spot {"stand": 5.765625, "placed": true, "env": 0}; nearest NPC Pet Groomer 4.4'),
    21: dict(cls=0, landing=(0, 13059.32, 54.4, 4289.59), spot=(0, 13060.41, 54.16, 4307.56), room=5,
            ground='18 from the landing at -70 deg of its facing; check {"stand": 54.156246, "floor": 54.156246, "env": 0}; spot {"stand": 54.156246, "env": 0}; nearest NPC Merchant McNeil 9.8'),
    22: dict(cls=6, landing=(0, 23656.97, 3.3, 6473.16), spot=(0, 23646.12, 3.16, 6468.03), room=5,
            ground='12 from the landing at +160 deg of its facing; check {"stand": 3.15625, "floor": 3.15625, "env": 0}; spot {"stand": 3.15625, "env": 0}; nearest NPC Equipment Dyer 5.3'),
    23: dict(cls=0, landing=(0, 25407.72, 70.01, 31410.26), spot=(0, 25411.47, 69, 31414.95), room=5,
            ground='6 from the landing at +150 deg of its facing; check {"stand": 69, "floor": 69, "env": 0}; spot {"stand": 69, "env": 0}; nearest NPC Equipment Dyer 5.1'),
    24: dict(cls=0, landing=(0, 15190.8, 99.92, 8767.33), spot=(0, 15204.16, 98.97, 8752.45), room=5,
            ground='20 from the landing at -110 deg of its facing; check {"stand": 98.96874, "floor": 98.96874, "env": 0}; spot {"stand": 98.96874, "env": 0}; nearest NPC Merchant Dora 5.5'),
    25: dict(cls=2, landing=(0, 18985.54, 63.4, 6356.56), spot=(0, 18979.63, 63.4, 6361.96), room=5,
            ground='8 from the landing at +90 deg of its facing; check {"stand": 63.398445, "placed": true, "floor": 52.84375, "env": 0}; spot {"stand": 63.398445, "placed": true, "env": 0}; nearest NPC Equipment Dyer 5.0'),
    26: dict(cls=0, landing=(0, 4243.7, 58.13, 17169.14), spot=(0, 4263.62, 58.52, 17167.31), room=5,
            ground='20 from the landing at -170 deg of its facing; check {"stand": 58.523438, "placed": true, "floor": 57.71875, "env": 0}; spot {"stand": 58.523438, "placed": true, "env": 0}; nearest NPC Lancet Carroll 6.2'),
    27: dict(cls=0, landing=(0, 25486.54, 61.37, 15546.98), spot=(0, 25491.57, 61.36, 15555.62), room=3.5,
            ground='10 from the landing at +120 deg of its facing; check {"stand": 61.364063, "placed": true, "floor": 54.093746, "env": 0}; spot {"stand": 61.364063, "placed": true, "env": 0}; nearest NPC Heirloom Outfitter 4.2'),
    28: dict(cls=2, landing=(0, 18985.54, 63.4, 6356.56), spot=(0, 18984.48, 63.4, 6364.49), room=5,
            ground='8 from the landing at +130 deg of its facing; check {"stand": 63.398445, "placed": true, "floor": 52.84375, "env": 0}; spot {"stand": 63.398445, "placed": true, "env": 0}; nearest NPC Equipment Dyer 5.0'),
    29: dict(cls=0, landing=(1, 9377.67, 53.19, 7385.81), spot=(1, 9380.85, 53.25, 7380.72), room=5,
            ground='6 from the landing at -130 deg of its facing; check {"stand": 53.25, "floor": 53.25, "env": 0}; spot {"stand": 53.25, "env": 0}; nearest NPC Weapon Enchanter 5.1'),
    30: dict(cls=0, landing=(0, 4727.37, 54.13, 21654.9), spot=(0, 4733.38, 54.61, 21651.3), room=5,
            ground='7 from the landing at +50 deg of its facing; check {"stand": 54.61435, "placed": true, "floor": 54.15625, "env": 0}; spot {"stand": 54.61435, "placed": true, "env": 0}; nearest NPC Equipment Dyer 5.2'),
    31: dict(cls=0, landing=(0, 4243.7, 58.13, 17169.14), spot=(0, 4269.59, 58.52, 17166.76), room=5,
            ground='26 from the landing at -170 deg of its facing; check {"stand": 58.523438, "placed": true, "floor": 57.71875, "env": 0}; spot {"stand": 58.523438, "placed": true, "env": 0}; nearest NPC Lancet Carroll 11.9'),
    32: dict(cls=0, landing=(0, 4727.37, 54.13, 21654.9), spot=(0, 4719.37, 54.16, 21655.02), room=5,
            ground='8 from the landing at -160 deg of its facing; check {"stand": 54.15625, "floor": 54.15625, "env": 0}; spot {"stand": 65.90625, "env": 0}; nearest NPC Heirloom Outfitter 5.5'),
}


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def row(db, tbl, where, args):
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", args)
    cols = [c[0] for c in cur.description]
    r = cur.fetchone()
    return None if r is None else dict(zip(cols, r))


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def audit(db, action, tbl, key, before, after, detail=DETAIL):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key),
                None if before is None else go_json(before), None if after is None else go_json(after), detail))


def heading_to(x, z, tx, tz):
    """CLIENT's heading from (x, z) toward (tx, tz): the direction (sin h, cos h)."""
    return math.atan2(tx - x, tz - z) % (2 * math.pi)


def byte_heading(h):
    return int(round(h / (2 * math.pi) * 256)) % 256


def landing(db, race, cls):
    """Where the flight sets a character of the race and class down (database.MainlandPlace: its wire race's start row
    for the class, ORDER BY as placeOf's; the city a human's racedefs.city names)."""
    wire, city = db.execute("SELECT wire_race, city FROM racedefs WHERE id = ?", (race,)).fetchone()
    hum = 0 if wire != 0 else city
    return db.execute("SELECT world, x, y, COALESCE(z, 0) FROM defaultcharacter WHERE race = ? "
                      "ORDER BY humType = ? DESC, tclass = ? DESC, humType, tclass LIMIT 1", (wire, hum, cls)).fetchone()


def words(race):
    s = RACES[race]
    offer = "\n".join([s["offer"], "Your spirit stays bound where it is.", s["ask"]])
    journal = f"Tell {s['waysender']} when you are ready to go home to {s['home']}."
    return offer, journal


def check_words():
    bad = []
    for race, s in RACES.items():
        offer, journal = words(race)
        lead = len(s["waysender"]) + 2
        for i, part in enumerate(offer.split("\n")):
            if lead + len(part) > BOX:
                bad.append(f"{s['waysender']}: offer line {i + 1} is {lead + len(part)} with the name (a box shows {BOX})")
        if len(s["ask"]) > ASK or not s["ask"].endswith("?"):
            bad.append(f"{s['waysender']}: the question is {len(s['ask'])} or asks nothing")
        for k in ("declined", "started"):
            if lead + len(s[k]) > BOX:
                bad.append(f"{s['waysender']}: {k} is {lead + len(s[k])}")
        if len(journal) > LINE:
            bad.append(f"{s['waysender']}: the journal line is {len(journal)} (the journal shows {LINE})")
        if max(len(ACCEPT), len(DECLINE)) > ANSWER:
            bad.append("an answer is too long")
    if bad:
        raise SystemExit("\n".join(bad))


def logged(db):
    if not table(db, LOG):
        return {}
    return {r[0]: dict(npcid=r[1], questid=r[2], npcrow=json.loads(r[3]), questrow=json.loads(r[4]), steprow=json.loads(r[5]))
            for r in db.execute(f"SELECT race, npcid, questid, npcrow, questrow, steprow FROM {LOG}")}


def same(a, b):
    return go_json(a) == go_json(b)


def plan(db):
    """[(race, what, detail)]: what is 'write' (detail: the rows to write), 'done', 'skip' or 'refuse' (detail: why)."""
    have = logged(db)
    out = []
    keys = dict(db.execute("SELECT key, questid FROM quest_import_quests WHERE key LIKE 'homelands/%/flight'")) \
        if table(db, "quest_import_quests") else {}
    for race, s in sorted(RACES.items()):
        tag = f"{race} {s['slug']}"
        if race in have:
            h = have[race]
            n, q = row(db, "npcs", "id = ?", (h["npcid"],)), row(db, "quests", "questid = ?", (h["questid"],))
            st = row(db, "queststeps", "questid = ? AND step = 1", (h["questid"],))
            if same(n, h["npcrow"]) and same(q, h["questrow"]) and same(st, h["steprow"]):
                out.append((race, "done", f"{tag}: {s['waysender']} (npcs {h['npcid']}) and quest {h['questid']} stand as written"))
            else:
                out.append((race, "done", f"{tag}: written before (npcs {h['npcid']}, quest {h['questid']}), CHANGED since (a GM's "
                                           "edit?): left as it is"))
            continue
        rd = row(db, "racedefs", "id = ?", (race,))
        if rd is None:
            out.append((race, "skip", f"{tag}: no racedefs row: no such race here"))
            continue
        flight = keys.get(f"homelands/{s['slug']}/flight")
        fq = flight and row(db, "quests", "questid = ?", (flight,))
        if not fq:
            out.append((race, "skip", f"{tag}: no flight quest (homelands/{s['slug']}/flight): no homeland here"))
            continue
        homes = db.execute("SELECT DISTINCT world, x, y, COALESCE(z, 0), facing FROM defaultcharacter WHERE race = ? AND world = ? "
                           "AND tclass != ?", (race, HOMELANDS, MONK)).fetchall()
        if len(homes) != 1:
            out.append((race, "skip" if not homes else "refuse",
                        f"{tag}: {len(homes)} homeland start spots in world {HOMELANDS} (one wanted)"))
            continue
        hw, hx, hy, hz, hf = homes[0]
        gks = db.execute("SELECT id FROM npcs WHERE npc_name = ? AND world = ? ORDER BY id", (fq["giver"], HOMELANDS)).fetchall()
        if len(gks) != 1:
            out.append((race, "refuse", f"{tag}: {len(gks)} rows of its gatekeeper {fq['giver']!r} in world {HOMELANDS}"))
            continue
        gk = row(db, "npcs", "id = ?", (gks[0][0],))
        if race not in SPOTS:
            out.append((race, "refuse", f"{tag}: no spot picked"))
            continue
        sp = SPOTS[race]
        (lw, lx, ly, lz), (w, x, y, z), room = sp["landing"], sp["spot"], sp["room"]
        land = landing(db, race, sp["cls"])
        if land is None or land[0] != lw or math.dist((land[1], land[2], land[3]), (lx, ly, lz)) > LANDING_SLACK:
            out.append((race, "refuse", f"{tag}: the flight's landing (class {sp['cls']}) in {s['city']} is {land}, not ({lw}, {lx}, {ly}, {lz}) "
                                        "where the spot was picked round: pick it again"))
            continue
        taken = db.execute("SELECT count(*) FROM npcs WHERE npc_name = ? COLLATE NOCASE", (s["waysender"],)).fetchone()[0] + \
            db.execute("SELECT count(*) FROM quests WHERE giver = ? COLLATE NOCASE OR name = ?", (s["waysender"], s["quest"])).fetchone()[0]
        if taken:
            out.append((race, "refuse", f"{tag}: the name {s['waysender']!r} or the quest {s['quest']!r} is someone's already"))
            continue
        near = db.execute("SELECT id, npc_name, x_coord, y_coord, z_coord FROM npcs WHERE world = ? AND abs(x_coord - ?) < 60 "
                          "AND abs(z_coord - ?) < 60", (w, x, z)).fetchall()
        crowd = [(n, math.hypot(nx - x, nz - z)) for _, n, nx, ny, nz in near if math.hypot(nx - x, nz - z) < room and abs(ny - y) < 15]
        if crowd:
            out.append((race, "refuse", f"{tag}: {crowd[0][0]} stands {crowd[0][1]:.1f} from the spot (wanted {room:g})"))
            continue
        nb = min(near, key=lambda r: math.hypot(r[2] - x, r[4] - z)) if near else None
        zone = row(db, "npcs", "id = ?", (nb[0],)) if nb else {"zone": 0, "unk_a": 0, "unk_b": 0}
        npc = {k: v for k, v in gk.items() if k != "id"}
        npc.update(npc_name=s["waysender"], world=w, x_coord=x, y_coord=y, z_coord=z,
                   facing=byte_heading(heading_to(x, z, lx, lz)), npc_type=TALKER, dt_kill=None,
                   zone=zone["zone"], unk_a=zone["unk_a"], unk_b=zone["unk_b"])
        offer, journal = words(race)
        quest = dict(name=s["quest"], enabled=1, giver=s["waysender"], minlevel=1, maxlevel=60, classes="", races=str(race),
                     requires=str(flight), repeatable=1, offer=offer, accept=ACCEPT, decline=DECLINE, declined=s["declined"],
                     started=s["started"], source=SOURCE,
                     notes=f"Ours (the owner, 2026-11-21: \"Build waysenders\"): the way home for a {rd['name']} who has flown out "
                           f"of {s['home']} on {fq['name']} (quest {flight}): {s['waysender']} in {s['city']} sends it back to "
                           f"its home's start, in front of {s['elder']}, without a bind. Repeatable, no fee, no reward. "
                           "Written by tools/homeland_waysenders_20261121.py.",
                     events="", factionreq="", excludes="")
        tele = {"on": "done", "do": "teleport", "world": hw, "x": round(hx, 2), "y": round(hy, 2), "z": round(hz, 2)}
        if hf:
            tele["facing"] = round(hf, 6)
        step = dict(step=1, kind="talk", target=s["waysender"], journal=journal,
                    events=json.dumps([{"on": "start", "do": "dostep"}, tele], separators=(",", ":")))
        out.append((race, "write", dict(tag=tag, npc=npc, quest=quest, step=step, flight=(flight, fq["name"], bool(fq["enabled"])),
                                         home=(hw, hx, hy, hz, hf), landing=land, gatekeeper=(gk["id"], gk["npc_name"]),
                                         neighbour=nb and (nb[0], nb[1], round(math.hypot(nb[2] - x, nb[4] - z), 1)))))
    return out


def insert(db, tbl, values):
    cols = list(values)
    return db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [values[c] for c in cols]).lastrowid


def apply(db, writes):
    db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (race INTEGER PRIMARY KEY, npcid INTEGER NOT NULL, questid INTEGER NOT NULL, "
               "npcrow TEXT NOT NULL, questrow TEXT NOT NULL, steprow TEXT NOT NULL, at TEXT NOT NULL)")
    for race, _, d in writes:
        nid = insert(db, "npcs", d["npc"])
        db.execute("UPDATE npcs SET npc_id = ? WHERE id = ?", (nid, nid))
        n = row(db, "npcs", "id = ?", (nid,))
        audit(db, "insert", "npcs", {"id": nid}, None, n)
        qid = insert(db, "quests", d["quest"])
        q = row(db, "quests", "questid = ?", (qid,))
        audit(db, "insert", "quests", {"questid": qid}, None, q)
        insert(db, "queststeps", dict(d["step"], questid=qid))
        st = row(db, "queststeps", "questid = ? AND step = 1", (qid,))
        audit(db, "insert", "queststeps", {"questid": qid, "step": 1}, None, st)
        db.execute(f"INSERT INTO {LOG} VALUES (?, ?, ?, ?, ?, ?, ?)",
                   (race, nid, qid, go_json(n), go_json(q), go_json(st), datetime.datetime.now(datetime.timezone.utc).isoformat()))
        print(f"  wrote {d['tag']}: npcs {nid}, quest {qid}")


def undo(db):
    have = logged(db)
    if not have:
        print("nothing to undo")
        return 0
    for race, h in have.items():
        for tbl, where, args, was in (("npcs", "id = ?", (h["npcid"],), h["npcrow"]),
                                      ("quests", "questid = ?", (h["questid"],), h["questrow"]),
                                      ("queststeps", "questid = ? AND step = 1", (h["questid"],), h["steprow"])):
            now = row(db, tbl, where, args)
            if now is not None and not same(now, was):
                print(f"REFUSED: {tbl} {args} (race {race}) changed since it was written: nothing undone")
                return 1
    marks = 0
    for race, h in sorted(have.items()):
        for tbl, where, args in (("charquests", "questid = ?", (h["questid"],)), ("queststeps", "questid = ?", (h["questid"],)),
                                 ("quests", "questid = ?", (h["questid"],)), ("npcs", "id = ?", (h["npcid"],))):
            cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", args)
            cols = [c[0] for c in cur.description]
            for r in cur.fetchall():
                r = dict(zip(cols, r))
                key = {"id": r["id"]} if tbl == "npcs" else {"questid": r["questid"]} if tbl == "quests" else \
                    {"questid": r["questid"], "step": r["step"]} if tbl == "queststeps" else {"serverid": r["serverid"], "questid": r["questid"]}
                audit(db, "delete", tbl, key, r, None, "undo: " + DETAIL)
                marks += tbl == "charquests"
            db.execute(f"DELETE FROM {tbl} WHERE {where}", args)
    db.execute(f"DROP TABLE {LOG}")
    print(f"took back {len(have)} waysenders and their quests ({marks} characters' marks of them)")
    return 0


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    check_words()
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            rc = undo(db)
            db.execute("COMMIT" if rc == 0 else "ROLLBACK")
            return rc
        steps = plan(db)
        writes = [st for st in steps if st[1] == "write"]
        for race, what, d in steps:
            if what != "write":
                print(f"  {what:6} {d}")
                continue
            n, q, (hw, hx, hy, hz, hf), (fid, fname, fen) = d["npc"], d["quest"], d["home"], d["flight"]
            print(f"  write  {d['tag']}: {n['npc_name']!r} at world {n['world']} ({n['x_coord']}, {n['y_coord']}, {n['z_coord']}) "
                  f"facing {n['facing']}, {RACES[race]['city']} (landing {tuple(round(v, 1) for v in d['landing'])}), "
                  f"the look of {d['gatekeeper'][1]} (npcs {d['gatekeeper'][0]}), nearest NPC {d['neighbour'][1]!r} "
                  f"{d['neighbour'][2]} off")
            print(f"         quest {q['name']!r}: race {q['races']}, requires {fid} {fname!r}"
                  f"{'' if fen else ' (a DRAFT here: only those who flew before can take this)'}, teleport to world {hw} "
                  f"({hx:.1f}, {hy:.1f}, {hz:.1f}) facing {hf:.3f}, no bind")
        refused = [st for st in steps if st[1] == "refuse"]
        if a.apply and writes:
            apply(db, writes)
            db.execute("COMMIT")
            print(f"{a.db}: {len(writes)} waysenders written, logged in {LOG}" + (f"; {len(refused)} refused" if refused else ""))
        else:
            db.execute("ROLLBACK")
            print(f"{len(writes)} to write" + (" (--apply writes them)" if writes else ": nothing to do") +
                  (f"; {len(refused)} refused" if refused else ""))
        return 1 if refused else 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
