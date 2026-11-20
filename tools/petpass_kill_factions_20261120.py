#!/usr/bin/env python3
"""Kill factions for the creature kinds that gave none (petition pass 2026-11-20, the owner: "yes lets fix"; both
shards).

A creature's kill moves its killer's standings by the factionhits rows of its name, or by factionhitareas rows for
where it spawned; a name with neither prints no faction line at its kill. Measured on the 2026-11-20 copies, whole
kinds had none (the Deathfist, kobolds, centaurs, lizardmen, ogres) and others only some names (goblins, orcs, gnolls,
aviaks, frogloks, trolls, bandits). The source is Prima's Frontiers guide (2003), pp. 383-391
(claude-notes/factions-prima-20261119.md): Table 1's faction and its kill penalty, with the place its note names,
and Table 2's associates (a kill costs an associate its care rating% of the penalty, truncated: an enemy gains). So a
spawn's faction is read off its kind and where it stands (world 0's 2,000-unit zone squares, claude-notes/
tunaria-map.md), as RULES says rule by rule; its kill is that faction's Prima set (PRIMA). Where Prima gives no
faction for a kind at a place, nothing is written and HELD says why.

Also an NPC's own faction (npcfaction) decides its kill where one is placed on it (a town seed's: Grobb's troll
bashers are Citizens of Grobb, Honjour's troll crusaders Citizens of Honjour), and the Rathe Mountains' hobgoblins and
Ankextet get their on-sight faction (npcfaction, world 1: Hobgoblins of Rathe Mtns., Clan Ankextet; every character
starts at -500 with both, so they stay KOS to all).

Written:
  - a name whose every spawn here takes one faction: factionhits rows (by name, as every other kill row);
  - a name split over factions or partly held: factionhitareas rows, a circle round each camp of it (refused if a
    circle would hold a spawn of the name of another faction or held).
Left: a name with any factionhits or factionhitareas row already (a GM's, a film's, a seed's); the other petition
lane's names (tools/petpass_hobgoblin_factions_20261120.py: the Rathe hobgoblins' and Ankextet's kill rows;
tools/petpass_npc_factions_20261120.py: Thedruk's orcs, the Slithtar Hive troll camp of petition 5343; the Qeynos
prison's prisoners and felons are no kind here). A faction missing on a shard drops that row (said).

Report only by default; --apply in one transaction, a gm_audit line a row, log petpass_kill_factions_log; --undo
exact. factionhits, factionhitareas and npcfaction are read at the server's start (after the next server update).

    python3 -E -s tools/petpass_kill_factions_20261120.py --db eqoa.db [--apply | --undo]
"""
import collections
import math
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_kill_factions_log"
DETAIL = "Kill factions for the creature kinds that gave none, Prima's: tools/petpass_kill_factions_20261120.py"
SOURCE = "prima-2003 petpass-1120 kinds"

# Prima's set for a kill of a member: (PDF page, ((faction, delta), ...)), Table 1's penalty and Table 2's shares.
PRIMA = {
    'Deathfist Hordes': (385, (('Deathfist Hordes', -10), ('Deathfist Mercenaries', -5), ('Citizens of Freeport', 1),
                              ('Citizens of Hazinak', 1), ('Monks of the Endless Tower', 1), ("Muniel's Tea Garden", 1),
                              ('The Freeport Militia', 1))),
    'Deathfist Mercenaries': (385, (('Deathfist Hordes', -5), ('Deathfist Mercenaries', -10))),
    'Clan Kolbok Kobolds': (385, (('Clan Kolbok Kobolds', -10), ('Kejek Village', 1))),
    'Kikarnta Kobolds': (386, (('Clan Kolbok Kobolds', -1), ('Kikarnta Kobolds', -10), ('The Merka Huntsmen', 1),
                              ('The Merka Tribe', 1), ('Welahi-Nai Tribe', 1))),
    'Fire Pit Kobolds': (385, (('Fire Pit Kobolds', -1),)),
    'Lizardmen of Tunaria': (386, (('Lizardmen of Tunaria', -10), ('Citizens of Grobb', 1), ('Citizens of Oggok', 1),
                                  ('Craknek Warriors', 1), ('Da Bashers', 1))),
    'Centaurs of the Plains': (384, (('Centaurs of the Plains', -10), ('Citizens of Caer Sloth', 1))),
    'Mog Thugs': (386, (('Mog Thugs', -5),)),
    'Clan Runnyeye': (385, (('Clan Runnyeye', -10), ('Citizens of Highpass', 5), ('Citizens of Loreth Kith', 1),
                           ('Citizens of Merry-by-water', 1), ('Citizens of Rivervale', 1), ('Saerk', 5),
                           ('Temple of Bristlebane', 1), ('The Deep Pockets', 1), ('The Guardians of the Vale', 1),
                           ('The Highpass Guard', 5), ('The Stormreapers', 1))),
    'Mossmouth Goblins': (386, (('Mossmouth Goblins', -10), ('Citizens of Bastable', 5), ('Citizens of Highpass', 5),
                               ('Saerk', 5), ('The Highpass Guard', 5))),
    'Darkwood Goblins': (385, (('Darkwood Goblins', -10), ('Black Swan Inn', 10), ('Chiasta Citizens', 1),
                              ('Citizens of Blakedown', 5), ('Citizens of Caer Sloth', 1), ('Citizens of Forkwatch', 1),
                              ("Jared's Tower", 1))),
    'The Freezeblood Goblins': (387, (('The Freezeblood Goblins', -10), ('Citizens of Halas', 1),
                                     ('Citizens of Moradhim', 1), ('Eyes of the Tribunal', 1), ('Miners Guild 231', 1),
                                     ('Seers of the Tribunal', 1), ('The Church of Below', 1),
                                     ('The Doomseekers of Moradhim', 1), ('The Stoneguard', 1),
                                     ('The Wolves of the North', 1))),
    'Solusek Goblins': (387, (('Solusek Goblins', -5),)),
    'Frosteye Cohort': (385, (('Frosteye Cohort', -10), ('Citizens of Halas', 1), ('Citizens of Moradhim', 1),
                             ('Eyes of the Tribunal', 1), ('Miners Guild 231', 1), ('Seers of the Tribunal', 1),
                             ('The Church of Below', 1), ('The Doomseekers of Moradhim', 1), ('The Stoneguard', 1),
                             ('The Wolves of the North', 1))),
    'Blacktongue Orcs': (384, (('Blacktongue Orcs', -10), ('Citizens of Bastable', 5))),
    'Hatebone Orcs': (385, (('Hatebone Orcs', -10), ('Academy of Arcane Science', 1), ('Citizens of Fayspires', 1),
                           ('Citizens of Freeport', 1), ('Citizens of Tethelin', 1), ('College of High Magic', 1),
                           ('Keepers of the Glade', 1), ('Merchant House Slaerin', 1), ('Scouts of Tunare', 1),
                           ('Silken Gauntlet', 1), ('Songweavers of Tunare', 1), ('The Emerald Guardians', 1),
                           ('The Freeport Militia', 1), ('The Paladins of Tunare', 1), ('The Spiteful Shield', 1))),
    'Orcs of Tunaria': (386, (('Orcs of Tunaria', -10), ('Academy of Arcane Science', 1), ('Citizens of Fayspires', 1),
                             ('Citizens of Freeport', 1), ('Citizens of Tethelin', 1), ('College of High Magic', 1),
                             ('Keepers of the Glade', 1), ('Merchant House Slaerin', 1), ('Scouts of Tunare', 1),
                             ('Silken Gauntlet', 1), ('Songweavers of Tunare', 1), ('The Emerald Guardians', 1),
                             ('The Freeport Militia', 1), ('The Paladins of Tunare', 1), ('The Spiteful Shield', 1))),
    'Coldpaw': (385, (('Coldpaw', -1),)),
    'Blackburrow Gnolls': (384, (('Blackburrow Gnolls', -10), ('Black Swan Inn', 10), ('Citizens of Blakedown', 1),
                                ('Citizens of Forkwatch', 1), ('Citizens of Merry-by-water', 1),
                                ('Citizens of Qeynos', 1), ('Citizens of Surefall Glade', 1),
                                ('Citizens of Wymondham', 1), ('Crusaders of Mithaniel Marr', 1),
                                ('Defenders of Erollisi Marr', 1), ('Order of the Silent Fist', 1),
                                ('Protectors of the Pine', 1), ('Qeynos Troupe', 1), ('The Anagogical Order', 1),
                                ('The Heavy Purse', 1), ('The Qeynos Guard', 1))),
    'Red Ear Gnolls': (386, (('Red Ear Gnolls', -10), ('Citizens of Jethro Pike', 1), ('Citizens of Surefall Glade', 1),
                            ('Citizens of Wyndhaven', 10), ('Protectors of the Pine', 1))),
    'Slesher Gnolls': (386, (('Slesher Gnolls', -10), ('Blackburrow Gnolls', 1), ('Citizens of Surefall Glade', 1),
                            ('Protectors of the Pine', 1))),
    'Gnoll Raiders': (385, (('Gnoll Raiders', -10),)),
    'Gnolls of Tunaria': (385, (('Gnolls of Tunaria', -10), ('Citizens of Qeynos', 1), ('Citizens of Rivervale', 1),
                               ('Citizens of Surefall Glade', 1), ('Protectors of the Pine', 1),
                               ('The Guardians of the Vale', 1), ('The Qeynos Guard', 1))),
    'Ragel Aviaks': (386, (('Ragel Aviaks', -10), ('Citizens of Highbourne', 1), ('The Peacekeepers', 1))),
    'Goldfeather Aviaks': (385, (('Goldfeather Aviaks', -10),)),
    "Bluefeather's Brood": (384, (("Bluefeather's Brood", -10),)),
    'Cawtou Aviaks': (384, (('Cawtou Aviaks', -10),)),
    'Aviaks of Tunaria': (384, (('Aviaks of Tunaria', -10),)),
    'The Frogloks of Guk': (387, (('Fungusmen of Guk', -1), ('The Frogloks of Guk', -5), ('Citizens of Grobb', 1),
                                 ('Da Bashers', 1), ('Shadowknights of Nightkeep', 1), ('The Dark Ones', 1),
                                 ('Undead Frogloks', 1))),
    'Da Bashers': (385, (('Citizens of Grobb', -10), ('Da Bashers', -10))),
    'Citizens of Grobb': (384, (('Citizens of Grobb', -10), ('Da Bashers', -10))),
    'Citizens of Honjour': (384, (('Citizens of Grobb', -5), ('Citizens of Honjour', -10), ('Da Bashers', -5))),
    'Razgut': (386, (('Razgut', -10),)),
    'Brokenskull Rock': (384, (('Brokenskull Rock', -10),)),
    "Tarlin's Bandits": (387, (("Tarlin's Bandits", -10),)),
    'Taereg of Tunaria': (387, (('Taereg of Tunaria', -10), ('Citizens of Klick`Anon', 1), ('Klick`Anon Clockworks', 1),
                               ('Klick`Anon Watch', 1), ('Mechanamagical College', 1), ('Necrological Society', 1),
                               ('Temple of Brell', 1), ('The Junk Mongers', 1))),
    'The Taksehir Nation': (387, (('The Taksehir Nation', -10), ('Nasehir Cutthroats', 1))),
    "Brontar's Bandits": (384, (("Brontar's Bandits", -10),)),
    'Bandits of Tunaria': (384, (('Bandits of Tunaria', -10), ('Citizens of Freeport', 1), ('Citizens of Qeynos', 1),
                                ('Citizens of Wyndhaven', 10), ('The Freeport Militia', 1), ('The Qeynos Guard', 1))),
}

# The kinds this tool looks at, by name (as npcclass keys it).
KINDS = {
    "deathfist": r"deathfist",
    "kobold": r"kobold|kolbok",
    "lizardman": r"lizardman",
    "centaur": r"centaur",
    "ogre": r"\bogres?\b|\borge\b|mog thug",
    "goblin": r"goblin|pickclaw|^darkwood (drunk|elite guard|grunt|guard|oracle|prison guard|scout|seer|taskmaster|"
              r"temptress)$",
    "orc": r"\borcs?\b|orcish|hatebone|frosteye|blacktongue|wktaan",
    "gnoll": r"gnoll|coldpaw",
    "aviak": r"aviak|cawtou|^r[ae]gal (high )?guard|^regal high priest",
    "froglok": r"froglok|frogluk|forglok",
    "troll": r"\btrolls?\b|razgut|skullbreaker|skullseer",
    "bandit": r"bandit",
}

# World 0's zones as tunaria-map.md grids them: a zone is the 2,000-unit square at its (X, Z).
ZONES0 = {
    "Runnyeye": (16000, 12000), "Misty Thicket": (16000, 10000), "Rivervale": (18000, 10000),
    "Moss Mouth Cavern": (18000, 12000), "Saerk Towers": (20000, 12000), "Alseops Wall": (12000, 16000),
    "Freezeblood Village": (12000, 6000), "Blackburrow": (12000, 12000), "Merry-by-Water": (14000, 12000),
    "Jethros Cast": (6000, 12000), "Aviak Village": (8000, 20000), "Goldfeather Eyrie": (14000, 6000),
    "Widows Peak": (12000, 22000), "Kelinar": (12000, 24000), "Bashers Enclave": (22000, 30000),
    "Brokenskull Rock": (20000, 30000), "Fort Alliance": (14000, 24000), "Kara Village": (20000, 6000),
    "Snowfist": (14000, 4000), "Greyvaxs Caves": (16000, 4000),
}


def in_zone(s, zone):
    x0, z0 = ZONES0[zone]
    return s["w"] == 0 and x0 <= s["x"] < x0 + 2000 and z0 <= s["z"] < z0 + 2000


def near(s, w, x, z, r):
    return s["w"] == w and math.hypot(s["x"] - x, s["z"] - z) <= r


def has(n, *words):
    return any(re.search(w, n) for w in words)


# Undead and lone oddities Prima gives no faction for, by name.
UNDEAD = r"zombie|skeleton|ghoul|mummif|reanimated|^the lost froglok$"

# (name of the rule, faction, test(kind, name, spawn), basis). The first that holds wins. Held rules (faction None)
# say why nothing is written.
RULES = [
    ("undead", None, lambda k, n, s: has(n, UNDEAD) and not (k == "gnoll" and near(s, 0, 9621.5, 13734.5, 300)),
     "an undead or lone creature: Prima gives no faction for it"),
    ("deathfist mercenary", "Deathfist Mercenaries", lambda k, n, s: s["w"] == 0 and n in ("deathfist mercenary",
                                                                                        "troll mercenary"),
     "Prima: 'Ogres and Trolls that have signed on with the Horde'"),
    ("deathfist", "Deathfist Hordes", lambda k, n, s: k == "deathfist" and s["w"] == 0,
     "Prima: the Deathfist horde of orcs, the Desert of Ro (every Deathfist name)"),
    # kobolds
    ("kolbok outcast", None, lambda k, n, s: n == "clan kolbok outcast", "an outcast of the clan: no faction in Prima"),
    ("kobolds of the Warrens", "Clan Kolbok Kobolds", lambda k, n, s: k == "kobold" and s["w"] == 2 and s["x"] >= 7000,
     "Prima: Clan Kolbok, 'Kobolds of Odus'; EQOA-Data's map names the Warrens' camps Clan Kolbok"),
    ("kobolds of the Hunt", "Kikarnta Kobolds", lambda k, n, s: k == "kobold" and s["w"] == 2 and s["x"] < 7000,
     "Prima: Kikarnta, 'a tribe of kobolds found on Odus', the Merka and Welahi-Nai Kerrans' enemy (Table 2): the "
     "Hunt's and the Abyssmal Sea's kobolds, beside the Merkans (INFERRED: Prima places no camp)"),
    ("kobolds of Lavastorm", "Fire Pit Kobolds", lambda k, n, s: k == "kobold" and s["w"] == 3,
     "Prima: Fire Pit Kobolds, 'kobold tribe within Solusek's Eye' (Lavastorm's only kobold faction)"),
    # lizardmen, centaurs, ogres
    ("lizardmen", "Lizardmen of Tunaria", lambda k, n, s: k == "lizardman" and s["w"] in (0, 1, 2),
     "Prima: 'Standard lizardmen faction used by swamp standard brown lizardmen' (Odus's too: no other)"),
    ("centaurs", "Centaurs of the Plains", lambda k, n, s: k == "centaur" and s["w"] == 0 and s["z"] >= 20000,
     "Prima: 'A society of Centaurs that live on Southern Plains of Karana' (every centaur camp is south of z 20000)"),
    ("ogre thugs", "Mog Thugs", lambda k, n, s: k == "ogre" and s["w"] == 1 and has(n, r"thug"),
     "Prima: Mog Thugs, 'Faction for Ogre bandits in Rathe Mountains'"),
    # goblins
    ("pickclaw", None, lambda k, n, s: has(n, r"pickclaw"),
     "Highpass Hold's pickclaw goblins (the Pickclaw Sashes quests' targets): no Pickclaw faction in Prima"),
    ("Runnyeye", "Clan Runnyeye", lambda k, n, s: k == "goblin" and (
        in_zone(s, "Runnyeye") or in_zone(s, "Misty Thicket") or in_zone(s, "Rivervale") or s["w"] == 14),
     "Prima: Clan Runnyeye, 'Runnyeye Citadel ... found on the edge of Misty Thicket' (and the Vale's woods; world 14 "
     "is Runnyeye's copy); their named neighbours there have it already"),
    ("Moss Mouth", "Mossmouth Goblins", lambda k, n, s: k == "goblin" and (
        in_zone(s, "Moss Mouth Cavern") or in_zone(s, "Saerk Towers")),
     "Prima: 'Goblins of Moss Mouth Cavern' (Saerk Towers' goblins beside it: Saerk is their enemy in Table 2); "
     "EQOA-Data's map: Mossmouth Goblins at the gladiators"),
    ("Darkwood", "Darkwood Goblins", lambda k, n, s: k == "goblin" and in_zone(s, "Alseops Wall"),
     "Prima: 'A clan of murderous goblins living within Darkwood Forest' (the darkwood camp's goblins)"),
    ("Freezeblood", "The Freezeblood Goblins", lambda k, n, s: k == "goblin" and in_zone(s, "Freezeblood Village"),
     "Prima: 'Arctic goblins of the North' (Freezeblood Village's)"),
    ("Lavastorm goblins", "Solusek Goblins", lambda k, n, s: k == "goblin" and s["w"] == 3,
     "Prima: 'The main goblin faction for Lavastorm Mountains'"),
    # orcs
    ("Frosteye", "Frosteye Cohort", lambda k, n, s: k == "orc" and has(n, r"frosteye"),
     "Prima: 'Frosteye orc faction located south of Halas and west of Moradhim'"),
    ("Blacktongue", "Blacktongue Orcs", lambda k, n, s: k == "orc" and has(n, r"blacktongue"),
     "Prima: Blacktongue Orcs (Ferran's Hope's camp, EQOA-Data's map)"),
    ("Wktaan", None, lambda k, n, s: has(n, r"wktaan"), "Wktaan's orcs: no faction in Prima"),
    ("Freeport's siege orcs", None, lambda k, n, s: k == "orc" and n in (
        "elder orc sapper", "orc sapper", "orc seigemaster", "orc strategist", "orc marauder", "orc gatherer"),
     "orcs at Freeport's walls (L48-50) and in Northwestern Ro, the Deathfist's country: Hatebone or Deathfist, "
     "Prima does not say"),
    ("Hatebone", "Hatebone Orcs", lambda k, n, s: k == "orc" and s["w"] == 0 and s["x"] >= 14000,
     "Prima: 'Black orcs of the Hatebone can be found throughout most of Eastern Tunaria' (their camps' film rows "
     "give its associates already)"),
    ("orcs of the west", "Orcs of Tunaria", lambda k, n, s: k == "orc" and s["w"] == 0 and s["x"] < 14000,
     "Prima: 'This is a basic orc faction' (west of x 14000: Druid's Watch, Mt. Hatespike, Blackburrow)"),
    # gnolls
    ("Coldpaw", "Coldpaw", lambda k, n, s: k == "gnoll" and has(n, r"coldpaw"),
     "Prima: Coldpaw, 'a gnoll village just outside of Klick`Anon' (penalty 1)"),
    ("Slesher camp", "Slesher Gnolls", lambda k, n, s: k == "gnoll" and near(s, 0, 9621.5, 13734.5, 300),
     "Prima: 'Outcast gnoll necromancers and shadowknights from Blackburrow'; EQOA-Data's map: Surefall Glade's "
     "Slesher gnoll camp (reanimated gnoll, gnoll coward, runt, runner)"),
    ("Blackburrow", "Blackburrow Gnolls", lambda k, n, s: k == "gnoll" and (
        in_zone(s, "Blackburrow") or in_zone(s, "Merry-by-Water") or s["w"] == 14),
     "Prima: 'A tribe of vicious gnolls that inhabit the dungeon of Blackburrow' (Merry-by-Water's ruffian camp, "
     "Blackburrow's names; world 14 is its copy)"),
    ("Red Ear", "Red Ear Gnolls", lambda k, n, s: k == "gnoll" and in_zone(s, "Jethros Cast"),
     "Prima: 'in conflict with the humans of Jethro's Pike'"),
    ("Gnoll Raiders", "Gnoll Raiders", lambda k, n, s: k == "gnoll" and s["w"] == 0 and s["x"] < 14000 and (
        n == "gnoll raider" or near(s, 0, 13589, 14661, 120)),
     "Prima: 'A ragtag band of desperate gnolls found in the Western regions of Tunaria' (the gnoll raiders, "
     "Jared's Blight's raider camp)"),
    ("gnolls", "Gnolls of Tunaria", lambda k, n, s: k == "gnoll" and s["w"] == 0,
     "Prima: 'Any gnolls that do not belong to a specific tribe on Tunaria'"),
    # aviaks
    ("Ragel", "Ragel Aviaks", lambda k, n, s: k == "aviak" and in_zone(s, "Aviak Village"),
     "Prima: 'The aviaks who live in and around Ragel Village in Aviak Village'"),
    ("Goldfeather", "Goldfeather Aviaks", lambda k, n, s: k == "aviak" and in_zone(s, "Goldfeather Eyrie"),
     "Prima: 'Aviaks of Goldfeather Eyrie'"),
    ("Bluefeather", "Bluefeather's Brood", lambda k, n, s: k == "aviak" and (
        in_zone(s, "Widows Peak") or in_zone(s, "Kelinar")),
     "Prima: 'Aviaks from Widow's Peak and Kelinar'"),
    ("Cawtou", "Cawtou Aviaks", lambda k, n, s: k == "aviak" and s["w"] == 1 and has(n, r"cawtou"),
     "Prima: 'aviaks living in the Rathe Mountains ... Cawtou village'"),
    ("aviaks", "Aviaks of Tunaria", lambda k, n, s: k == "aviak" and s["w"] == 0,
     "Prima: 'Tunarian Aviaks who do not belong to any tribe' (the outpost near Halas)"),
    # frogloks
    ("froglok prisoner", None, lambda k, n, s: n == "froglok prisoner", "a prisoner in Grobb: no faction in Prima"),
    ("Guk", "The Frogloks of Guk", lambda k, n, s: k == "froglok" and (
        (s["w"] == 0 and s["x"] >= 18000 and s["z"] >= 28000) or s["w"] == 14),
     "Prima: 'Main froglok faction in and around Guk' (Guk, Grobb, Sslathis, the Bashers' Enclave, the Ant "
     "Colonies, the Burial Mounds; world 14 is Guk's copy)"),
    # trolls
    ("Razgut", "Razgut", lambda k, n, s: k == "troll" and has(n, r"razgut"),
     "Prima: 'Camp of Trolls in Fort Alliance'"),
    ("Brokenskull", "Brokenskull Rock", lambda k, n, s: k == "troll" and has(n, r"skullbreaker|skullseer"),
     "Prima: 'Faction for the Brokenskull Troll clan' (Brokenskull Rock's skullbreakers)"),
    ("Bashers' Enclave", "Da Bashers", lambda k, n, s: n == "troll basher" and in_zone(s, "Bashers Enclave"),
     "Prima: Da Bashers, 'Grobb guard faction' (the enclave's troll bashers; Grobb's own are Citizens of Grobb)"),
    # bandits
    ("Tarlin", "Tarlin's Bandits", lambda k, n, s: k == "bandit" and has(n, r"tarlin"),
     "Prima: 'A small group of bandits led by Tarlin'"),
    ("Taereg", "Taereg of Tunaria", lambda k, n, s: k == "bandit" and has(n, r"taereg"),
     "Prima: 'A band of rogues and burglars camped to the West of Klick`Anon'"),
    ("Taksehir", "The Taksehir Nation", lambda k, n, s: k == "bandit" and has(n, r"taksehir"),
     "Prima: The Taksehir Nation (the desert bandits)"),
    ("arctic bandits", "Brontar's Bandits", lambda k, n, s: n == "arctic bandit",
     "Prima: 'A group of bandits that wander the northern tundras of Everfrost' (Greyvax's Caves)"),
    ("bandits", "Bandits of Tunaria", lambda k, n, s: k == "bandit" and s["w"] == 0,
     "Prima: 'An underground society of bandits who operate all over Tunaria'"),
]

# The other lane's rows (petition 5343): its camp's trolls.
TROLL_CAMP = (24158.0, 26585.0, 60.0)
# On-sight factions (npcfaction) for the Rathe Mountains (world 1).
ON_SIGHT = (("hobgoblin", "Hobgoblins of Rathe Mtns."), ("ankextet", "Clan Ankextet"))
OTHER_LANE = r"hobgoblin|ankextet"
OTHER_LANE_SOURCE = "petpass-1120"
CAMP_GAP = 150.0


def kind_of(n):
    for k, p in KINDS.items():
        if re.search(p, n):
            return k
    return None


def placed_faction(rows, s):
    """The npcfaction row's faction placed on this spawn (the smallest circle holding it, else its world's, else
    anywhere's), or None."""
    best = None
    for r in rows:
        if r["world"] not in (-1, s["w"]):
            continue
        if r["radius"] > 0 and math.hypot(s["x"] - r["x"], s["z"] - r["z"]) > r["radius"]:
            continue
        rank = (0 if r["radius"] > 0 else 1 if r["world"] >= 0 else 2, r["radius"])
        if best is None or rank < best[0]:
            best = (rank, r["faction"])
    return best[1] if best else None


def camps(spawns):
    """The spawns in groups, each within CAMP_GAP of another of its group (one world)."""
    left, out = list(spawns), []
    while left:
        group = [left.pop()]
        grew = True
        while grew:
            grew = False
            for s in list(left):
                if any(s["w"] == g["w"] and math.hypot(s["x"] - g["x"], s["z"] - g["z"]) <= CAMP_GAP for g in group):
                    group.append(s)
                    left.remove(s)
                    grew = True
        out.append(group)
    return out


def changes(db):
    out, lines, refusals = [], [], []
    have = {r["name"] for r in tr.rows(db, "SELECT name FROM factions")}
    key = lambda n: (n or "").lower().strip().rstrip("$").strip()  # noqa: E731
    covered = {key(r["n"]) for r in tr.rows(db, "SELECT npc_name AS n FROM factionhits WHERE source != ?",
                                            (SOURCE,))}
    # an area row covers its name, except the other lane's (Thedruk, the prison): its spawns elsewhere are this one's
    covered |= {key(r["n"]) for r in tr.rows(db, "SELECT npc_name AS n FROM factionhitareas WHERE source NOT IN (?, ?)",
                                             (SOURCE, OTHER_LANE_SOURCE))}
    placed = collections.defaultdict(list)
    for r in tr.rows(db, "SELECT name, world, x, z, radius, faction FROM npcfaction WHERE source != ?", (SOURCE,)):
        placed[key(r["name"])].append(r)

    spawns = collections.defaultdict(list)
    for r in tr.rows(db, "SELECT npc_name, world, x_coord, z_coord FROM npcs WHERE (npc_type & 128) = 0"):
        n = key(r["npc_name"])
        k = kind_of(n)
        if k:
            spawns[n].append({"w": r["world"], "x": r["x_coord"], "z": r["z_coord"], "k": k})

    by_rule = collections.defaultdict(lambda: collections.Counter())
    held = collections.defaultdict(lambda: collections.Counter())
    left_names = collections.Counter()
    dropped = collections.Counter()
    for n in sorted(spawns):
        if n in covered:
            left_names[spawns[n][0]["k"]] += 1
            continue
        if re.search(OTHER_LANE, n) and all(s["w"] == 1 for s in spawns[n]):
            continue
        faction_of = []
        for s in spawns[n]:
            k = s["k"]
            if k == "orc" and s["w"] == 0 and 18000 <= s["x"] <= 20000 and 8000 <= s["z"] <= 10000:
                f, why = "-", "Thedruk (the other lane's, petition 5330)"
            elif n in ("troll crusader", "troll raider") and near(s, 0, *TROLL_CAMP):
                f, why = "-", "the Slithtar Hive camp (the other lane's, petition 5343)"
            else:
                f = placed_faction(placed.get(n, ()), s)
                why = "its own npcfaction"
                if f is not None and f not in PRIMA:
                    f, why = None, f"its npcfaction {f!r}, not one this tool carries"
                if f is None and why == "its own npcfaction":
                    for name, rf, test, basis in RULES:
                        if test(k, n, s):
                            f, why = rf, name
                            break
                    else:
                        f, why = None, "no rule: Prima gives no faction for it here"
            faction_of.append((s, f, why))
            if f and f != "-":
                by_rule[why][n] += 1
            elif f is None:
                held[why][n] += 1
        factions = {f for _, f, _ in faction_of if f and f != "-"}
        if not factions:
            continue
        whole = len(factions) == 1 and all(f and f != "-" for _, f, _ in faction_of)
        for f in sorted(factions):
            hits = [(hf, d) for hf, d in PRIMA[f][1]]
            for hf, _ in hits:
                if hf not in have:
                    dropped[hf] += 1
            hits = [(hf, d) for hf, d in hits if hf in have]
            if whole:
                for hf, d in hits:
                    k = {"npc_name": n, "faction": hf}
                    out.append(("factionhits", k, dict(k, delta=d, source=SOURCE)))
                continue
            mine = [s for s, sf, _ in faction_of if sf == f]
            others = [s for s, sf, _ in faction_of if sf != f]
            for g in camps(mine):
                cx = round(sum(s["x"] for s in g) / len(g), 1)
                cz = round(sum(s["z"] for s in g) / len(g), 1)
                r = round(max(math.hypot(s["x"] - cx, s["z"] - cz) for s in g) + 10.0, 1)
                bad = [o for o in others if near(o, g[0]["w"], cx, cz, r)]
                if bad:
                    refusals.append(f"{n}: a {f} circle ({cx}, {cz}) r {r} would hold {len(bad)} spawn(s) of "
                                    f"another faction or held")
                    continue
                for hf, d in hits:
                    k = {"npc_name": n, "faction": hf, "world": g[0]["w"], "x": cx, "z": cz, "radius": r}
                    out.append(("factionhitareas", k, dict(k, delta=d, note=f"{f}: a camp of {len(g)} "
                                                                             f"(Prima 2003, petpass-1120)",
                                                           source=SOURCE)))

    # On-sight: the Rathe Mountains' hobgoblins and Ankextet.
    for word, f in ON_SIGHT:
        if f not in have:
            refusals.append(f"no faction {f!r} here")
            continue
        names = sorted({key(r["n"]) for r in tr.rows(db, "SELECT npc_name AS n, world FROM npcs WHERE (npc_type & 128) = 0 "
                                                         "AND lower(npc_name) LIKE ?", (f"%{word}%",))})
        names = [n for n in names if not placed.get(n) and all(
            r["world"] == 1 for r in tr.rows(db, "SELECT DISTINCT world FROM npcs WHERE lower(trim(npc_name)) = ?", (n,)))]
        for n in names:
            k = {"name": n, "world": 1, "x": 0.0, "z": 0.0, "radius": 0.0}
            out.append(("npcfaction", k, dict(k, faction=f, note=f"{f} (Prima 2003, petpass-1120): on sight",
                                              source=SOURCE)))
        lines.append(f"on sight (npcfaction, world 1): {len(names)} {word} names -> {f}")

    for why in by_rule:
        f = next((rf for name, rf, _, _ in RULES if name == why), None) or "(its npcfaction)"
        names = by_rule[why]
        lines.append(f"{why} -> {f}: {len(names)} names, {sum(names.values())} spawns: " +
                     ", ".join(f"{n} {c}" for n, c in sorted(names.items())))
    for why in held:
        names = held[why]
        lines.append(f"HELD ({why}): {len(names)} names, {sum(names.values())} spawns: " +
                     ", ".join(f"{n} {c}" for n, c in sorted(names.items())))
    lines.append("left (a kill row already): " + ", ".join(f"{k} {c}" for k, c in sorted(left_names.items())))
    if dropped:
        lines.append("dropped (no such faction here): " + ", ".join(f"{f} ({c})" for f, c in sorted(dropped.items())))
    tables = collections.Counter(t for t, _, _ in out)
    lines.insert(0, f"{len(out)} rows: " + ", ".join(f"{t} {c}" for t, c in sorted(tables.items())))
    applied = bool(out) and all(tr.get(db, t, k) is not None and all(tr.get(db, t, k).get(c) == v
                                                                      for c, v in a.items()) for t, k, a in out)
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("factionhits", "factionhitareas", "npcfaction", "factions", "npcs"),
        "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
