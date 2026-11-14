#!/usr/bin/env python3
"""The Plane of Sky's scripted events (the owner, 2026-11-14: "we are missing scripted events in PoS"), from the
players' 2010 walkthroughs of the chains (Ronson's copies on eqoa.forumotion.com: the Rain, Sun, Night, Snow and
Desolation chains; CS's and Yado's notes in them). The creatures stood in the world all at once; here each isle's
event brings them as the guides tell it. One guide-gaps set, `sky-events`, written by tools/seed_guide_gaps.py's
machinery (every row it changes kept in guide_set_log, so --undo is exact), triggers 201-219:

  201 Night 1, the Voice of the Ancients (a new NPC at the isle's north end): a talk wakes the waves, four ancient
      peasants, five minutes after their deaths three vassals, three minutes after three denizens, five minutes
      after the squire by the exit portal, then the Ancient Jester (his Night Key 1 opens the isle's essence).
  202 Night 3, save the maiden: a player at the courtyard's mouth starts it; the maiden comes out on her pedestal,
      two dukes of the night at the mouth, and Arch Bishop Millian and Arch Bishop Lancel must die within five
      minutes (awaitkill: the bishops of the towers, not copies), else the dukes carry her off (the run fails);
      saved, she stays fifteen minutes for the quest's talk. Her row stands out of the world between events.
  203 Rain 2, Kaglanor: a talk to Sterie Glitterlight (her quest answers as ever) raises Kaglanor by the maze's
      ruined wall, one at a time.
  204 Rain 4, the necrologist: a player within 25 of his spot (the quest's popup spot) brings Narciliisisin; at his
      death his spirit rises where he fell. 205 Rain 5: his death (npcdeath) wakes Chimareon the Tyrant on his isle.
  206 Snow 1, Ryini's walk (quest 101307 step 2's trigger event): her row hidden an hour, a copy of her walks the
      mountain corridor (a route off the ground file), two icebound gargoyles set on her halfway, a Wind Mephit at
      the stone bridge, a talk to go on, then left at the road's end over the village; her death fails it.
  207 Snow 4, Enraged Aeyinar (quest 101307 step 15's trigger event): fifteen minutes.
  208-210 Sun 3: a whispering wind's death 30% a whistling wind where it fell, a whistling's 30% a wailing, a
      wailing's the Torrential Wind (one at a time).
  211 Sun 4: the rancorous drake's death raises Randarin the Relentless.
  212-213 Desolation 1: a scout's or scavenger's death 5% an erych patroller where it fell (five at most stand);
      the ring's controller (any scout's or scavenger's death starts it, four hours) counts five patroller kills,
      raises the five guardians on their rows, then the Erych Arbiter three times on her death spot.
  214 Desolation 2, the aviak village: a talk to the elder (his quest answers as ever) sends six waves of four,
      scouts then warriors, five minutes apart, clockwise round the village (N, E, S, W), each set on the elder; his
      death fails it; all dead, the Warlord of Narcillian on his row.
  215-219 Desolation 3: a dark harpy's death a light harpy where it fell (30% a second), a light's a dark, eight
      at most of a kind near; either's 5% the elite harpy queen on the spot (219), one at a time.

Prototypes held out of the world by row key (npcspawnonly "#id", the rows' ids the same on both shards, checked):
the Night 1 waves, the Night Maiden, Kaglanor, the necrologist, his spirit, Chimareon, the Wind Mephit, Enraged
Aeyinar, the whistling, wailing and torrential winds, Randarin, the patrollers, guardians and Arbiter, Narcillian's
scouts, warriors and Warlord, the harpy queen. New: the Voice of the Ancients (the ghost of Magi'Kot's look, NPC
bit), a night duke (Ruthra's squire's look, NPC bit, spawn-only), classtemplate "sky escort" (30x a level's hit
points, 40% of a hit taken: ours) for Ryini and the aviak elder, and the two quest step events.

Every spot is a row's own (the creature's), or one read off ~/.cache/movecheck/ground/4.ground (the Voice, the
squire by the portal, the courtyard's mouth, the four wave spots, Ryini's route by a heightfield search from her
row to the Wind Mephit's and on to the floor over the village); every count, chance and time not in the guides is
ours. Jib's package (#1371) sets these creatures' templates by name: its rows apply to the copies as they are, and
its held Sun 4 rare (rapacious -> rancorous) and Enraged Aeyinar profile slot in beside these without a change.

Refused, nothing written, unless trigger slots 201-219 are free, no trigger has one of the names, each row is the
creature it should be in world 4, the new names are not taken and the two quest steps are as imported. Report
only by default; --apply in one transaction with a gm_audit line; an --apply when the set is in says so and does
nothing; --undo exact, with its own line. Needs the code of branch sky-events (start, awaitkill, fail, walk
points/pace): an older server reads triggers 201, 202, 203, 204, 206, 212, 214 as problems. Triggers, NPCs,
templates, npcdeath and quests are read at the server's start, so a restart after the code and this.

    tools/sky_events_20261114.py --db eqoa.db [--apply | --undo]
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

SET = "sky-events"
WHO, VIA = "dab", "tool"
DETAIL = "The Plane of Sky's scripted events (the owner, 2026-11-14): tools/sky_events_20261114.py"
W = 4

# the rows held out of the world, and copied by the events: id -> name as the row has it
ROWS = {
    119680: "ancient peasant", 119681: "ancient peasant", 119682: "ancient peasant", 119683: "ancient peasant",
    119684: "ancient vassal", 119685: "ancient vassal", 119686: "ancient vassal",
    119687: "ancient denizen", 119688: "ancient denizen", 119689: "ancient denizen",
    119690: "ancient squire", 119691: "Ancient Jester", 119727: "Night Maiden",
    120092: "Kaglanor", 120149: "Necrologist", 120150: "Spirit of Narciliisisin", 120151: "Chimareon the Tyrant",
    119634: "Wind Mephit", 119679: "Enraged Aeyinar",
    120193: "whistling wind", 120205: "whistling wind", 120206: "whistling wind", 120207: "whistling wind", 120208: "whistling wind",
    120194: "wailing wind", 120209: "wailing wind", 120210: "wailing wind", 120195: "torrential wind",
    120219: "Randarin the Relentless",
    119543: "erych patroller", 119550: "erych patroller", 119551: "erych patroller", 119558: "erych patroller",
    119529: "erych guardian", 119530: "erych guardian", 119531: "erych guardian", 119532: "erych guardian", 119533: "erych guardian",
    119534: "Erych Arbiter",
    119578: "scout of Narcillian", 119579: "scout of Narcillian", 119580: "scout of Narcillian", 119581: "scout of Narcillian",
    119582: "warrior of Narcillian", 119583: "warrior of Narcillian", 119584: "warrior of Narcillian", 119585: "warrior of Narcillian",
    119586: "warlord of Narcillian", 119595: "elite harpy queen",
}
# rows the events stand by (checked, never changed)
KEEP = {119600: "Ryini", 119574: "aviak elder", 120091: "Sterie Glitterlight", 120218: "rancorous drake"}
NEW_NAMES = ("voice of the ancients", "night duke")


def at(x, y, z):
    return {"world": W, "x": x, "y": y, "z": z}


def spawn(npc, x, y, z, count=1, spread=1, minutes=60, **kw):
    s = {"do": "spawn", "npc": npc, "hostile": True, "spread": spread, "minutes": minutes, **at(x, y, z)}
    if count > 1:
        s["count"] = count
    s.update(kw)
    return s


def trig(slot, name, radius, x, y, z, steps, notes, world=W):
    t = {"slot": slot, "name": name, "scope": "near", "radius": radius, "world": world, "steps": steps, "notes": notes}
    if world >= 0:
        t.update({"x": x, "y": y, "z": z})
    return t


# Ryini's route off the ground file (a heightfield search, cells of 2, a step of at most 1.6 a cell): her row to
# the Wind Mephit's at the stone bridge, 437 long, then on down to the floor over the village, 295
RYINI_TO_BRIDGE = [[5647.0, 558.6, 6333.0], [5619.0, 559.8, 6327.0], [5591.0, 556.4, 6333.0], [5569.0, 551.2, 6355.0],
                   [5553.0, 538.9, 6375.0], [5553.0, 540.0, 6405.0], [5553.0, 534.8, 6435.0], [5553.0, 524.0, 6463.0],
                   [5555.0, 507.9, 6489.0], [5575.0, 506.8, 6511.0], [5595.0, 498.1, 6531.0], [5621.0, 490.6, 6537.0],
                   [5651.0, 488.7, 6537.0]]
BRIDGE_TO_ROAD_END = [[5689.0, 484.7, 6571.0], [5689.0, 485.0, 6597.0], [5689.0, 485.2, 6623.0], [5687.0, 483.2, 6647.0],
                      [5681.0, 475.8, 6649.0], [5659.0, 473.9, 6655.0], [5651.0, 460.8, 6661.0], [5631.0, 453.7, 6663.0],
                      [5611.0, 445.2, 6671.0], [5589.0, 439.0, 6679.0]]
RYINI_PACE = 4.0  # walked, an escort's

DESO2 = (4933.34, 615.75, 6704.0)
WAVES = [(4800.0, 615.75, 6605.0), (5050.0, 620.69, 6718.0), (4930.0, 615.75, 6795.0), (4763.0, 616.94, 6694.0)]  # N E S W


def deso2_steps():
    steps = [{"do": "lock", "lock": "aviak village", "text": "The elder is watching the sky already: Narcillian's warriors are coming."},
             {"do": "awaittalk", "npc": "aviak elder", "start": True},
             {"do": "protect", "npc": "aviak elder", "radius": 10,
              "text": "The elder has fallen, and Narcillian's warriors turn away from the burning village."},
             {"do": "clock", "seconds": 5400, "empty": 900},
             {"do": "broadcast", "text": "The aviak elder cries out: Narcillian's scouts are on the wind! Defend the village!"}]
    for i in range(6):
        x, y, z = WAVES[i % 4]
        who, row = ("scout of Narcillian", "#119578") if i % 2 == 0 else ("warrior of Narcillian", "#119582")
        steps += [spawn(row, x, y, z, count=4, spread=6, minutes=60),
                  {"do": "sic", "npc": who, "target": "aviak elder", "radius": 320}]
        if i < 5:
            steps.append({"do": "wait", "seconds": 300})
    steps += [{"do": "awaitdead"},
              {"do": "broadcast", "text": "The last of the waves is broken. The Warlord of Narcillian comes for the village himself!"},
              spawn("#119586", 4656.63, 630.69, 6645.69, minutes=60),
              {"do": "awaitdead"},
              {"do": "broadcast", "text": "The warlord is dead. The orb to the third isle of desolation shines on the portal pad."},
              {"do": "end"}, {"do": "despawn"}, {"do": "wait", "seconds": 600}]
    return steps


def harpy(slot, name, npc, death, chance_note):
    return trig(slot, name, 30, 0, 0, 0, [spawn(npc, 0, 0, 0, spread=3, minutes=30, max=8) | {"world": None}],
                f"Desolation 3: 'when you kill the dark version of the harpy a light harpy will spawn and vica-versa (often TWO "
                f"spawn)'. Fired where the {death} fell (npcdeath, {chance_note}); eight at most of a kind within 300 (ours).", world=-1)


def spec():
    for_world = lambda st: [{k: v for k, v in s.items() if not (k == "world" and v is None)} for s in st]
    t = []
    t.append(trig(201, "Night 1: Voice of the Ancients", 220, 4160.0, 612.38, 6235.0, [
        {"do": "lock", "lock": "night 1 voice", "text": "The Voice of the Ancients is silent: the ancients already walk the isle."},
        {"do": "awaittalk", "npc": "Voice of the Ancients", "start": True, "radius": 220,
         "text": "You would wake those who sleep beneath the night? Then see them rise."},
        {"do": "clock", "seconds": 3600, "empty": 600},
        {"do": "broadcast", "text": "The ground of the isle stirs. Ancient peasants rise from it."},
        spawn("#119680", 4164.5, 612.38, 6123.3, count=4, spread=8),
        {"do": "awaitdead"},
        {"do": "wait", "seconds": 300},
        {"do": "broadcast", "text": "Ancient vassals come to avenge their peasants."},
        spawn("#119684", 4164.35, 612.38, 6149.5, count=3, spread=7),
        {"do": "awaitdead"},
        {"do": "wait", "seconds": 180},
        {"do": "broadcast", "text": "The ancient denizens of the isle rise."},
        spawn("#119687", 4167.65, 612.38, 6175.7, count=3, spread=7),
        {"do": "awaitdead"},
        {"do": "wait", "seconds": 300},
        {"do": "broadcast", "text": "An ancient squire stands by the exit portal."},
        spawn("#119690", 4161.0, 612.38, 6390.0, spread=2),
        {"do": "awaitdead"},
        {"do": "broadcast", "text": "Laughter rings across the isle: the Ancient Jester has come."},
        spawn("#119691", 4174.66, 612.38, 6215.7),
        {"do": "awaitdead"},
        {"do": "broadcast", "text": "The Jester falls silent, and the essence of night at the isle's end glows: his keys open it."},
        {"do": "end"}, {"do": "despawn"}, {"do": "wait", "seconds": 300}],
        "Night 1 (t209): speak to the Voice of the Ancients; peasants x4, 5 min, vassals x3, a few minutes (180 s, ours), "
        "denizens x3, 5 min, the squire by the exit portal, then the Ancient Jester (shard 1, the keys). Waves at their old "
        "rows; the Voice at the isle's north end and the squire's spot by the essence read off the ground file; an hour, "
        "10 empty minutes end it, 5 minutes' rest after (ours)."))
    t.append(trig(202, "Night 3: Save the Maiden", 300, 4250.0, 569.38, 6580.0, [
        {"do": "lock", "lock": "night 3 maiden"},
        {"do": "awaitnear", "start": True, "radius": 15},
        {"do": "popup", "text": "The Night Maiden is brought out onto her pedestal, and the dukes of the night start toward her. "
                               "Slay Arch Bishop Millian and Arch Bishop Lancel in their towers before the dukes reach her!"},
        {"do": "spawn", "npc": "#119727", "spread": 1, "minutes": 30, **at(4250.09, 660.62, 6743.46)},
        {"do": "spawn", "npc": "night duke", "count": 2, "spread": 3, "minutes": 10, **at(4250.0, 571.2, 6590.0)},
        {"do": "awaitkill", "npc": "Arch Bishop Millian, Arch Bishop Lancel", "seconds": 300,
         "fail": "The dukes of the night reach the maiden and carry her off into the dark. When the bishops stand again, try again."},
        {"do": "despawn", "npc": "night duke"},
        {"do": "broadcast", "text": "The bishops are dead, and the dukes fall back into the night. The maiden is safe: speak with her."},
        {"do": "wait", "seconds": 900}],
        "Night 3 (t209, Yado and CS): the popup at the entrance, the bishops Millian and Lancel dead before the dukes reach "
        "the maiden, then she gives the shard. The courtyard's mouth (south of the entry plaza, the landing 130 off) off the "
        "ground file; her pedestal her row's; five minutes and two dukes (their name and look ours, untouchable). She stands "
        "out of the world between events; saved, 15 minutes."))
    t.append(trig(203, "Rain 2: Kaglanor", 60, 4194.98, 180.0, 5147.39, [
        {"do": "lock", "lock": "kaglanor"},
        {"do": "awaittalk", "npc": "Sterie Glitterlight", "start": True},
        {"do": "broadcast", "text": "A cold wind moves by the maze's ruined wall: the spirit Kaglanor has come."},
        spawn("#120092", 4158.34, 180.0, 5158.97, minutes=30),
        {"do": "awaitdead"}, {"do": "wait", "seconds": 120}],
        "Rain 2 (t211): 'the questor must speak with her. At this point a white con spirit (kaglanor) spawns behind you near "
        "the maze wall ruins.' His row's spot; one at a time; Sterie's quest answers the talk as ever."))
    t.append(trig(204, "Rain 4: The Necrologist", 120, 4231.7, 85.76, 4214.52, [
        {"do": "lock", "lock": "narciliisisin"},
        {"do": "awaitnear", "start": True, "radius": 25},
        {"do": "broadcast", "text": "Among the demions the necrologist Narciliisisin shows himself."},
        {"do": "spawn", "npc": "#120149", "hostile": True, "spread": 1, "minutes": 30},
        {"do": "awaitdead"},
        {"do": "place", "dead": True},
        {"do": "broadcast", "text": "The necrologist falls, and his spirit tears free of him."},
        {"do": "spawn", "npc": "#120150", "hostile": True, "spread": 1, "minutes": 30},
        {"do": "awaitdead"}, {"do": "wait", "seconds": 300}],
        "Rain 4 (t211): 'roam ... until you acquire a pop-up which will spawn ... Narciliisisin'; 'Kill the Necrologist ... "
        "a Spirit of Narciliisisin has spawned'. Started within 25 of his row (quest 101303 step 6's spot), for anyone "
        "(Chimareon's farmers need it too); the spirit where he fell; 5 minutes' rest (ours)."))
    t.append(trig(205, "Rain 5: Chimareon", 150, 4555.66, 30.53, 4276.34, [
        {"do": "lock", "lock": "chimareon"},
        {"do": "broadcast", "text": "Far across the last isle of rain, Chimareon the Tyrant wakes."},
        {"do": "spawn", "npc": "#120151", "hostile": True, "spread": 1, "minutes": 60},
        {"do": "awaitdead"}],
        "Rain 5 (t211): 'Chimera only spawns on Rain 5, when someone on Rain 4 gets the popup and kills the necrologist'. "
        "Fired by the necrologist's death (npcdeath); his row's spot; an hour (ours)."))
    t.append(trig(206, "Snow 1: Ryini's Walk", 400, 5667.63, 552.84, 6355.57, [
        {"do": "lock", "lock": "ryini"},
        {"do": "clock", "seconds": 1800, "empty": 600},
        {"do": "hide", "npc": "Ryini", "radius": 10, "minutes": 60},
        {"do": "spawn", "npc": "#119600", "spread": 1, "minutes": 30, "hostile": False, **at(5667.63, 552.84, 6355.57)},
        {"do": "protect", "npc": "Ryini", "radius": 10,
         "text": "Ryini has fallen to the gargoyles. In an hour she will be back where you found her."},
        {"do": "say", "npc": "Ryini", "text": "Take me down the mountain, %p. I would see Tamis again."},
        {"do": "walk", "npc": "Ryini", "points": RYINI_TO_BRIDGE, "pace": RYINI_PACE, "x": 5674.45, "y": 488.09, "z": 6554.08},
        {"do": "wait", "seconds": 45},
        {"do": "spawn", "npc": "icebound gargoyle", "count": 2, "spread": 4, "minutes": 20, "hostile": True, **at(5553.0, 531.5, 6445.0)},
        {"do": "sic", "npc": "icebound gargoyle", "target": "Ryini", "radius": 100},
        {"do": "wait", "seconds": 70},
        {"do": "popup", "text": "Ryini reaches the stone bridge, and a Wind Mephit tears out of the air before her!"},
        spawn("#119634", 5674.45, 488.09, 6554.08, spread=4, minutes=20),
        {"do": "awaitdead", "npc": "Wind Mephit"},
        {"do": "popup", "text": "Ryini waits by the bridge. Speak with her to go on."},
        {"do": "awaittalk", "npc": "Ryini", "text": "Thank you. It is not far now."},
        {"do": "walk", "npc": "Ryini", "points": BRIDGE_TO_ROAD_END, "pace": RYINI_PACE, "x": 5571.0, "y": 439.0, "z": 6697.0},
        {"do": "wait", "seconds": 75},
        {"do": "popup", "text": "Ryini stands at the end of the road over her village. She thanks you: speak with her, then with Tamis below."},
        {"do": "wait", "seconds": 300},
        {"do": "end"}, {"do": "despawn"}, {"do": "hide", "npc": "Ryini", "radius": 10, "minutes": 0}],
        "Snow 1 (t212): escort Ryini down the mountain corridor; at the stone bridge a Wind Mephit, a popup, talk to her "
        "again; where it opens a sharp left to the end of the road, a popup; if she dies you fail and wait an hour. "
        "Fired by quest 101307 step 2. Her route off the ground file (a heightfield search from her row to the Mephit's "
        "and on to the floor over the village; it turns left there as the guide says); her row hidden, a copy walks "
        "(her template 'sky escort'); the gargoyle pair, the times and the pace ours."))
    t.append(trig(207, "Snow 4: Enraged Aeyinar", 150, 5508.73, 106.56, 7597.06, [
        {"do": "lock", "lock": "enraged aeyinar"},
        {"do": "spawn", "npc": "#119679", "hostile": True, "spread": 1, "minutes": 15},
        {"do": "awaitdead"}],
        "Snow 4 (t212): 'walk towards the Aeyinar spawn spot with the stew ... They will get a popup and Enraged Aeyinar will "
        "spawn ... you only have 15 min in which to kill her'. Fired by quest 101307 step 15; her row's spot."))
    t.append(trig(208, "Sun 3: Whistling Wind", 30, 0, 0, 0, for_world([spawn("#120193", 0, 0, 0, spread=3, minutes=30, max=5) | {"world": None}]),
                  "Sun 3 (t213): 'Kill Whispering Winds to spawn Whistling Winds'. Where it fell (npcdeath 30%, ours); five at most.", world=-1))
    t.append(trig(209, "Sun 3: Wailing Wind", 30, 0, 0, 0, for_world([spawn("#120194", 0, 0, 0, spread=3, minutes=30, max=3) | {"world": None}]),
                  "Sun 3 (t213): 'Kill Whistling Winds to spawn Wailing Winds'. Where it fell (npcdeath 30%, ours); three at most.", world=-1))
    t.append(trig(210, "Sun 3: Torrential Wind", 30, 0, 0, 0, for_world([{"do": "lock", "lock": "torrential wind"},
                                                                             spawn("#120195", 0, 0, 0, spread=3, minutes=60) | {"world": None},
                                                                             {"do": "awaitdead"}]),
                  "Sun 3 (t213): 'Torrential Wind will spawn after Wailing Wind is killed'. Where it fell; one at a time.", world=-1))
    t.append(trig(211, "Sun 4: Randarin the Relentless", 150, 5073.07, 430.88, 4244.48, [
        {"do": "lock", "lock": "randarin"},
        {"do": "broadcast", "text": "The rancorous drake's death cry is answered: Randarin the Relentless has come."},
        spawn("#120219", 5073.07, 430.88, 4244.48),
        {"do": "awaitdead"}],
        "Sun 4 (t213): 'Randarin the Relentless will spawn after you kill Rancorous Drake'. npcdeath; his row's spot."))
    t.append(trig(212, "Deso 1: Erych Patroller", 30, 0, 0, 0, for_world([spawn("#119543", 0, 0, 0, spread=2, minutes=240, max=5) | {"world": None}]),
                  "Desolation 1 (t208): 'Each time the placeholder is killed there is a % chance that the corresponding patroller will "
                  "spawn'. Where the scout or scavenger fell (5%, jib's regional figure); five at most; four hours.", world=-1))
    t.append(trig(213, "Deso 1: The Erych Ring", 300, 4760.07, 569.03, 6248.97, [
        {"do": "lock", "lock": "erych ring"},
        {"do": "clock", "seconds": 14400, "empty": 1800},
        {"do": "awaitkill", "npc": "erych patroller", "count": 5},
        {"do": "broadcast", "text": "The fifth patroller falls. Five Erych Guardians rise in the druid ring."},
        spawn("#119529", 4768.53, 568.0, 6232.2, minutes=120), spawn("#119530", 4776.91, 568.0, 6243.02, minutes=120),
        spawn("#119531", 4776.92, 568.0, 6255.98, minutes=120), spawn("#119532", 4742.62, 568.0, 6242.12, minutes=120),
        spawn("#119533", 4752.75, 568.0, 6232.82, minutes=120),
        {"do": "awaitdead"},
        {"do": "broadcast", "text": "The guardians are dead. In the druid ring the Erych Arbiter wakes."},
        spawn("#119534", 4760.07, 569.03, 6248.97),
        {"do": "awaitdead"}, {"do": "place", "dead": True}, {"do": "wait", "seconds": 10},
        {"do": "broadcast", "text": "The Erych Arbiter rises again where she fell."},
        {"do": "spawn", "npc": "#119534", "hostile": True, "spread": 1, "minutes": 60},
        {"do": "awaitdead"}, {"do": "place", "dead": True}, {"do": "wait", "seconds": 10},
        {"do": "broadcast", "text": "Once more the Erych Arbiter rises."},
        {"do": "spawn", "npc": "#119534", "hostile": True, "spread": 1, "minutes": 60},
        {"do": "awaitdead"},
        {"do": "broadcast", "text": "The Erych Arbiter falls for the last time. The portal at the isle's north end opens."},
        {"do": "end"}, {"do": "despawn"}, {"do": "wait", "seconds": 600}],
        "Desolation 1 (t208): five patrollers, then five guardians in their place, then the Arbiter, who 'will respawn on her "
        "deathspot 3 times' (three lives, 10 s apart: jib's from the 2006 tape). Started by any scout's or scavenger's death "
        "(npcdeath, the lock keeps one); the guardians on their rows; four hours, 30 empty minutes end it (ours). Each life "
        "carries the row's keys and shard (no second prototype: a new row of her name would break #1371's guards)."))
    t.append(trig(214, "Deso 2: The Aviak Village", 320, *DESO2, deso2_steps(),
                  "Desolation 2 (t208): 'talk to the Elder ... Scouts and Warriors of Narcillian will spawn and head towards the "
                  "village every 5minutes ... about four ... six waves ... in a clockwise motion'; the elder must live; then the "
                  "Warlord. Wave spots N, E, S, W of the village off the ground file (W the scouts' old rows), each joined to "
                  "the elder by the ground; the Warlord on his row; the elder's template 'sky escort' (ours)."))
    t.append(harpy(215, "Deso 3: Light Harpy", "light harpy", "dark harpy", "100%"))
    t.append(harpy(216, "Deso 3: Second Light Harpy", "light harpy", "dark harpy", "30%, ours"))
    t.append(harpy(217, "Deso 3: Dark Harpy", "dark harpy", "light harpy", "100%"))
    t.append(harpy(218, "Deso 3: Second Dark Harpy", "dark harpy", "light harpy", "30%, ours"))
    t.append(trig(219, "Deso 3: The Harpy Queen", 30, 0, 0, 0, for_world([{"do": "lock", "lock": "harpy queen"},
                                                                              spawn("#119595", 0, 0, 0, spread=2, minutes=60) | {"world": None},
                                                                              {"do": "awaitdead"}]),
                  "Desolation 3 (t208): 'If the Harpy Queen is going to spawn, she'll spawn on top of you'. A harpy's death 5% "
                  "(jib's figure) where it fell; one at a time.", world=-1))
    for x in t:
        clean = []
        for st in x["steps"]:
            if st.get("world", 0) is None:  # at the run's place (where a death fell): no place of its own
                st = {k: v for k, v in st.items() if k not in ("world", "x", "y", "z")}
            clean.append(st)
        x["steps"] = clean
    deaths = [{"name": "necrologist", "trigger": "Rain 5: Chimareon", "chance": 100, "note": "Rain 5 wakes at his death (t211)."},
              {"name": "whispering wind", "trigger": "Sun 3: Whistling Wind", "chance": 30, "note": "Sun 3 (t213); chance ours."},
              {"name": "whistling wind", "trigger": "Sun 3: Wailing Wind", "chance": 30, "note": "Sun 3 (t213); chance ours."},
              {"name": "wailing wind", "trigger": "Sun 3: Torrential Wind", "chance": 100, "note": "Sun 3 (t213)."},
              {"name": "rancorous drake", "trigger": "Sun 4: Randarin the Relentless", "chance": 100, "note": "Sun 4 (t213)."},
              {"name": "erych scout", "trigger": "Deso 1: Erych Patroller", "chance": 5, "note": "Deso 1 (t208): a placeholder."},
              {"name": "erych scavenger", "trigger": "Deso 1: Erych Patroller", "chance": 5, "note": "Deso 1 (t208): a placeholder."},
              {"name": "erych scout", "trigger": "Deso 1: The Erych Ring", "chance": 100, "note": "Starts the ring's controller (its lock keeps one)."},
              {"name": "erych scavenger", "trigger": "Deso 1: The Erych Ring", "chance": 100, "note": "Starts the ring's controller (its lock keeps one)."},
              {"name": "dark harpy", "trigger": "Deso 3: Light Harpy", "chance": 100, "note": "Deso 3 (t208)."},
              {"name": "dark harpy", "trigger": "Deso 3: Second Light Harpy", "chance": 30, "note": "Deso 3 (t208): 'often TWO'; chance ours."},
              {"name": "light harpy", "trigger": "Deso 3: Dark Harpy", "chance": 100, "note": "Deso 3 (t208)."},
              {"name": "light harpy", "trigger": "Deso 3: Second Dark Harpy", "chance": 30, "note": "Deso 3 (t208): 'often TWO'; chance ours."},
              {"name": "dark harpy", "trigger": "Deso 3: The Harpy Queen", "chance": 5, "note": "Deso 3 (t208); jib's 5%."},
              {"name": "light harpy", "trigger": "Deso 3: The Harpy Queen", "chance": 5, "note": "Deso 3 (t208); jib's 5%."}]
    npcs = [{"name": "Voice of the Ancients", "like": "ghost of Magi'Kot", "x": 4164.0, "y": 612.38, "z": 6080.0, "facing": 0, "world": W,
             "level": 60, "type": 128, "look": {"npc_id": 0},
             "note": "Night 1 (t209): speaking with it starts the waves (trigger 201). The ghost of Magi'Kot's look, ours."},
            {"name": "night duke", "like": "Ruthra's squire", "x": 4250.0, "y": 571.2, "z": 6590.0, "facing": 0, "world": W,
             "level": 64, "type": 128, "spawnonly": True, "look": {"npc_id": 0},
             "note": "Night 3 (t209): the dukes who must not reach the maiden (trigger 202). Name and look ours; untouchable."}]
    src = f"guide gaps {SET}"
    sql = [["INSERT INTO npcspawnonly (name, note, source) VALUES (?, ?, ?)",
            [f"#{rid}", f"{name}: brought by the Plane of Sky's scripted events (tools/sky_events_20261114.py)", src]]
           for rid, name in sorted(ROWS.items())]
    sql += [["INSERT OR REPLACE INTO npctemplate (name, template, note, source) VALUES (?, 'sky escort', ?, ?)", [n, note, src]]
            for n, note in (("ryini", "Snow 1's escort (trigger 206)"), ("aviak elder", "Deso 2's defended elder (trigger 214)"))]
    return {
        "set": SET,
        "templates": [{"name": "sky escort", "class": 0, "hp_pct": 3000, "level": 60, "taken": 40,
                       "note": "Ryini and the aviak elder, set on by an event's creatures: 30x a level's hit points and 40% of a "
                               "hit taken (ours), so a wave left alone takes minutes to kill them, not seconds."}],
        "npcs": npcs, "triggers": t, "deaths": deaths, "sql": sql,
        "step_events": [
            {"quest": "Portal/plane-of-sky/snow", "step": 2, "target": "Ryini",
             "events": [{"on": "done", "do": "trigger", "trigger": "Snow 1: Ryini's Walk",
                         "text": "Ryini is already on her way down the mountain with someone. Wait for her."}]},
            {"quest": "Portal/plane-of-sky/snow", "step": 15, "target": "",
             "events": [{"on": "done", "do": "trigger", "trigger": "Snow 4: Enraged Aeyinar",
                         "text": "Aeyinar is awake already: wait until she falls or sleeps again."}]}],
    }


def audit(db, action, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if sgg.has(db, "accountinfo") else None
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', ?, 'guide_set_log', ?, NULL, ?, ?)",
               (when, acct[0] if acct else 0, WHO, VIA, action, json.dumps({"set": SET}), json.dumps(after, sort_keys=True), DETAIL))


def refusals(db, s):
    out = []
    for t in s["triggers"]:
        r = db.execute("SELECT name FROM gmtriggers WHERE slot = ?", (t["slot"],)).fetchone()
        if r:
            out.append(f"trigger slot {t['slot']} is taken: {r[0]!r}")
        if db.execute("SELECT 1 FROM gmtriggers WHERE lower(name) = lower(?)", (t["name"],)).fetchone():
            out.append(f"a trigger is named {t['name']!r} already")
    for rid, name in {**ROWS, **KEEP}.items():
        r = db.execute("SELECT npc_name, world FROM npcs WHERE id = ?", (rid,)).fetchone()
        if not r or r[0].strip().lower() != name.lower() or r[1] != W:
            out.append(f"npcs row {rid} is {r}, not {name!r} of world {W}")
    for rid in ROWS:
        if db.execute("SELECT 1 FROM npcspawnonly WHERE name = ?", (f"#{rid}",)).fetchone():
            out.append(f"#{rid} is held out of the world already")
    for n in NEW_NAMES:
        for tbl, col in (("npcs", "npc_name"), ("npcspawnonly", "name"), ("npctemplate", "name")):
            if db.execute(f"SELECT 1 FROM {tbl} WHERE lower(trim({col})) = ?", (n,)).fetchone():
                out.append(f"{tbl} names {n!r} already")
    for n in ("ghost of magi'kot", "ruthra's squire", "icebound gargoyle", "light harpy", "dark harpy"):
        if not db.execute("SELECT 1 FROM npcs WHERE world = ? AND lower(trim(npc_name)) = ?", (W, n)).fetchone():
            out.append(f"no {n!r} in world {W}")
    if db.execute("SELECT 1 FROM classtemplates WHERE name = 'sky escort'").fetchone():
        out.append("classtemplate 'sky escort' is there already")
    for step, kind, target in ((2, "talk", "Ryini"), (15, "reach", "")):
        r = db.execute("SELECT kind, target, events FROM queststeps WHERE questid = 101307 AND step = ?", (step,)).fetchone()
        if not r or r[0] != kind or r[1] != target or "trigger" in r[2]:
            out.append(f"quest 101307 step {step} is not as imported: {r}")
    return out


def check(db):
    """After the set's rows: every trigger without a problem the Go side's checks would find in its JSON shape."""
    for slot, steps in db.execute("SELECT slot, steps FROM gmtriggers WHERE slot BETWEEN 201 AND 219"):
        st = json.loads(steps)
        if not st or any("do" not in x for x in st):
            raise SystemExit(f"trigger {slot}'s steps are not a list of steps")
    n = db.execute("SELECT count(*) FROM npcspawnonly WHERE source = ?", (f"guide gaps {SET}",)).fetchone()[0]
    if n != len(ROWS) + 1:
        raise SystemExit(f"{n} rows held out of the world, want {len(ROWS) + 1}")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--undo", action="store_true")
    ap.add_argument("--dump", action="store_true", help="print the set's rows as JSON and stop")
    a = ap.parse_args()
    s = spec()
    if a.dump:
        print(json.dumps(s, indent=1))
        return
    db = sqlite3.connect(a.db, timeout=5, isolation_level=None)
    if a.undo:
        sgg.undo(db, SET, print, extra=lambda d, n: audit(d, "undo", {"rows": n}))
        return
    if sgg.has(db, sgg.LOG) and db.execute(f"SELECT 1 FROM {sgg.LOG} WHERE setname = ?", (SET,)).fetchone():
        print(f"set {SET} is in already: nothing to do (--undo takes it out)")
        return
    bad = refusals(db, s)
    if bad:
        for b in bad:
            print("refused:", b)
        sys.exit(1)

    def extra(d, counts):
        check(d)
        audit(d, "apply", counts)
    sgg.apply(db, s, a.apply, print, extra=extra if a.apply else None)


if __name__ == "__main__":
    main()
