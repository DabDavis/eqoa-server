#!/usr/bin/env python3
"""teamjosh.com's Hunt and Gather quests that tools/handg_quests_20261115.py could not make, made (the owner,
2026-11-15: "lets add these", new NPCs allowed for them). Each shard's own rows: run it on each database, after
handg_quests_20261115.py.

  1. Heirloom's givers of its eight teamjosh quests (board entry 1081) on a shard that lacks them: Jeweler Yilt,
     Counciler Dreda, Jenya Gooten, Tinker Z., Katrina Finney, Terison Eydillo, Viltharus Battleprayer (and Big Basher
     Biggum, item 4's giver), their npcs rows as Heirloom has them (tools/handg/givers_20261115.json: place, world,
     look, level, npc_type) with their rows by name (npcclass; Biggum's npcfaction and lootcreature);
     then those eight quests and Preserved Ghast Eyes (handg_quests_20261115.py's, as it makes them).
  2. The givers not in the game, made beside the NPCs of the place the source names (each spot on the world's ground
     file's floor, measured: no water, nothing placed over it), each a copy of a townsperson's look with the NPC bit
     (128), and their quests: the Fist of Marr Recruiter at Marr's Fist by the Temple of Light (keys 8, 10, 15, 16,
     32), Handyman Hilty on Stone of Morthalis's east docks (rat whiskers), Abrion Oleken in Stormhaven (tattered
     shark skins, for the page's Sharkskin Shield), the Escaping Slave by the Salt Mines (a siloth slaver key taken
     to an enslaved miner in the mines, two made there: the page's "do not return keys to the escaped slave"),
     Apothecary Nadira in the Oasis (the page's giver is unnamed; poison stingers).
  3. The two pages whose giver is the game's under another name, put to the page's figures, not made twice:
     Tuskcarver is Tuskmaster Helgley (Plagueswine Tusks, the plague swine north of the Last House), the "NPC near
     banker in PoD" Pluxklez (Scuzz Toad Eyes, for the stew: the page's "Soup?").
  4. The two pages that name no item, given one named from their creature (flagged in the notes): Big Basher
     Biggum's froglok scalps (Guk's frogloks, the page's 14-34) and Sprexy Lionhunter's lioness pelts (the page's
     pie as the reward).

Stand-ins (no new creatures): the page's sandsharks are the game's mako, bloodfin and white-tipped sharks (none
swims off Stormhaven: Freeport's coast); the "Plainsraider Orcs" are the game's orc plainsraiders. A figure the page
lacks is ours, said in the quest's notes.

Every row it changes is found by comparing the tables before and after (tools/seed_guide_gaps.py's machinery, set
"handg-givers-20261115"), so --undo is exact; a gm_audit line a row, one for an undo. Report only by default;
--apply in one transaction; a second --apply does nothing; --undo, again nothing. NPCs are read at the server's
start or by the GM panel's reload_npcs; quests at the start.

    tools/handg_givers_20261115.py --db eqoa.db [--apply | --undo]
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
import handg_quests_20261115 as h  # noqa: E402

SET = "handg-givers-20261115"
WHO, VIA = "dab", "tool"
DETAIL = "teamjosh.com's Hunt and Gather givers and quests (the owner, 2026-11-15): tools/handg_givers_20261115.py"
GIVERS = os.path.join(HERE, "handg", "givers_20261115.json")
sgg.TABLES = ("itempattern", "quests", "queststeps", "queststeptargets", "npcs", "npcclass", "npcfaction", "lootcreature",
              "npcdialog", "npctemplate", "npcspawnonly", "npcaggro", "npcmove", "sqlite_sequence")

# name, the townsperson whose look it takes, world, x, y (the ground file's floor there), z, facing (the neighbour's),
# level, the neighbour it stands by (its zone), why there
NPCS = [
    ("Fist of Marr Recruiter", "Kipcah", 0, 23184.6, 61.00, 15500.8, 158, 40, "Kipcah",
     "the page: outside the entrance to the Temple of Light; Marr's Fist, by the Temple of Light west of Freeport's north "
     "gate, 5 west of Kipcah (Sir Roger Rodgers' temple door 55 south)"),
    ("Handyman Hilty", "Werp Juglre", 2, 7350.3, 53.69, 11128.6, 177, 50, "Dockmaster Bibbin",
     "the page: Stone of Morthalis, by the east exit of town on the docks; 5 north of Dockmaster Bibbin"),
    ("Abrion Oleken", "Smithy Vorgel", 0, 4650.0, 57.72, 19479.6, 59, 30, "Smithy Vorgel",
     "the page: Stormhaven; 7 south of Smithy Vorgel, Stormhaven's smith"),
    ("Escaping Slave", "slave", 0, 14793.4, 54.19, 19180.8, 85, 20, "Briggs",
     "the page: by the entrance to the Salt Mines; 5 east of Briggs at the mines' gate (Slavemaster Hern 230 west)"),
    ("enslaved miner", "slave", 0, 14559.1, -28.06, 19034.1, 0, 20, "Lionell Redrick",
     "the page: the enslaved miners in the Salt Mines; the mines' lowest floor, among the siloth slavers"),
    ("enslaved miner", "slave", 0, 14573.4, 6.19, 19067.9, 0, 20, "Lionell Redrick",
     "the page: the enslaved miners in the Salt Mines; the slavers' middle floor"),
    ("Apothecary Nadira", "Gypsy Leigh", 0, 21024.7, 54.19, 25267.1, 8, 40, "Gypsy Leigh",
     "the page: '- Oasis', its giver unnamed; a name of ours, 5 south of Gypsy Leigh among the Oasis's quest givers"),
]

FIST = "Fist of Marr Recruiter"
NT99 = {"trade": 1, "maxstack": 99}


def fist(key, name, levels, xp, item, like, target, also, chance, offer, loot, notes, factions=""):
    return dict(key=key, name=name, giver=FIST, levels=levels, xp=xp, tunar=0, factions=factions,
                item=(item, like, f"Proof off the {loot}. The Fist of Marr Recruiter pays for them.", NT99),
                target=target, also=also, chance=chance, offer=offer,
                loot=f"Kill {loot} for four {item.lower()}s.", give=f"Bring four {item.lower()}s to the Fist of Marr Recruiter.",
                says="The Fist thanks you.", notes=notes)


QUESTS = [
    fist(8, "Blackened Earrings", (13, 18), 56000, "Blackened Earring", "Gnoll Ear Cuff", "orc plainsraider", [], 0.25,
         "The plainsraider orcs harry the plains around Freeport. Bring me four of their blackened earrings.",
         "orc plainsraiders on the plains", "teamjosh H&G key 8 (the owner's source, 2026-11-15): 13-18, the Fist of Marr "
         "Recruiter, blackened earrings off the Plainsraider Orcs (the game's orc plainsraiders, 18-21). The page gives no "
         "experience: 56,000 is ours (Recruiter Deryn's 14-18 turn-in); 'Unusually rare drop': 0.25, ours."),
    fist(10, "Dark Silver Rings", (19, 23), 105984, "Dark Silver Ring", "Black Gnoll Ring", "Deathfist legionnaire", [], 0.5,
         "The Deathfist legionnaires wear dark silver rings. Bring me four.",
         "Deathfist legionnaires", "teamjosh H&G key 10 (the owner's source, 2026-11-15): 19-23, 105,984, dark silver rings "
         "off the Deathfist Legionnaires. Count 4 and chance 0.5 ours."),
    fist(15, "Deathfist Insignia Rings", (30, 34), 135000, "Insignia Ring", "Siloth Insignia Ring", "Deathfist champion",
         ["Deathfist cleric"], 0.5, "The Deathfist champions and clerics of the citadel wear insignia rings. Bring me four.",
         "Deathfist champions and clerics", "teamjosh H&G key 15 (the owner's source, 2026-11-15): 30-34, '135,000ish' "
         "(135,000), insignia rings off the Deathfist Champions and Clerics. Count 4 and chance 0.5 ours."),
    fist(16, "Platinum Insignias", (35, 38), 395136, "Platinum Insignia", "Siloth Insignia Ring", "Deathfist reaver",
         ["Deathfist slayer"], 0.5, "The Deathfist reavers and slayers carry platinum insignias. Bring me four.",
         "Deathfist reavers and slayers", "teamjosh H&G key 16 (the owner's source, 2026-11-15): 35-38, 395,136, platinum "
         "insignias off the Deathfist Reavers and Slayers. Count 4 and chance 0.5 ours."),
    fist(32, "Deathfist Seals", (24, 29), 177600, "Deathfist Seal", "Blood Paw Collar", "Deathfist myrmidon", [], 0.25,
         "The Deathfist myrmidons west of the tea garden carry the citadel's seals. Bring me four.",
         "Deathfist myrmidons", "teamjosh H&G key 32 (the owner's source, 2026-11-15): 24-29, 177,600, deathfist seals off the "
         "Deathfist Myrmidons, the page's factions; 'possibly every 1 out of 4 will drop': 0.25. Count 4 ours.",
         factions="Crusaders of Marr's Fist:+4, Stormhaven Guard:+5, Citizens of Stormhaven:+10, Citizens of Wymondham:+10, "
                  "Citizens of Hagley:+10"),
    dict(key=28, name="Rat Whiskers", giver="Handyman Hilty", levels=(53, 56), xp=1200000, tunar=0, factions="",
         item=("Rat Whisker", "Broken Leopard Claw", "A ratman's whisker from Cape Dreg. Handyman Hilty of Stone of Morthalis pays for them.", NT99),
         target="ratman warrior", also=["ratman fighter", "ratman scavenger", "ratman spiritist", "elite ratman", "ratman augur",
                                        "ratman channeler", "ratman plaguespreader", "ratman wizard", "ratman scout",
                                        "ratman diviner", "ratman slayer", "ratman sorcerer", "ratman trickster",
                                        "fishmaster ratman"],
         chance=0.5, offer="The ratmen of Cape Dreg have whiskers stiff enough for my brushes. Bring me four.",
         loot="Kill the ratmen of Cape Dreg for four rat whiskers.", give="Bring four rat whiskers to Handyman Hilty.",
         says="Fine whiskers!", notes="teamjosh H&G key 28 (the owner's source, 2026-11-15): 53-56, 1,200,000, Handyman Hilty "
         "(on Stone of Morthalis's docks by the east exit of town), rat whiskers off Cape Dreg's ratmen (all the game's ratman "
         "kinds). Count 4 and chance 0.5 ours."),
    dict(key=50, name="Tattered Shark Skins", giver="Abrion Oleken", levels=(15, 20), xp=3200, tunar=0, factions="",
         item=("Tattered Shark Skin", "Matted Bear Hide", "A tattered shark skin. Abrion Oleken of Stormhaven makes shields of them.", NT99),
         target="mako shark", also=["bloodfin shark", "white-tipped shark"], chance=0.5, rewards=["Sharkskin Shield"],
         offer="A wooden frame and tattered shark skins make a fine shield. Bring me four skins and it is yours.",
         loot="Kill sharks for four tattered shark skins.", give="Bring four tattered shark skins to Abrion Oleken.",
         says="A fine shield it will make. Here.", notes="teamjosh H&G key 50 (the owner's source, 2026-11-15): 15-20, 3,200 "
         "experience and the Sharkskin Shield (the page's figures), Abrion Oleken of Stormhaven, tattered shark skins off the "
         "sandsharks along Stormhaven's coast. No shark swims there: the game's mako, bloodfin and white-tipped sharks "
         "(13-20, off Freeport's coast) stand for them. Count 4 and chance 0.5 ours."),
    dict(key=19, name="The Siloth Slaver Keys", giver="Escaping Slave", give_to="enslaved miner", levels=(38, 38), xp=13000,
         tunar=0, factions="", count=1,
         item=("Siloth Slaver Key", "Broken Totem", "A key off a Siloth slaver. It opens an enslaved miner's chains.",
               {"trade": 1, "maxstack": 5}),
         target="Siloth slaver", also=["Siloth guard", "Siloth knight", "Siloth ritualist"], chance=0.5,
         offer="The Siloth keep my people chained in the Salt Mines. Take a key off them and free a miner. Do not bring it to me.",
         loot="Take a siloth slaver key off the Siloth of the Salt Mines.", give="Free an enslaved miner in the Salt Mines with the key.",
         says="Free! The Siloth will not hold me again. Thank you.",
         notes="teamjosh H&G key 19 (the owner's source, 2026-11-15): 38, '13,000ish' (13,000), the Escaping Slave by the "
         "Salt Mines' entrance; siloth slaver keys off the Siloth Humans (slavers, guards, knights, ritualists), stacking to "
         "five, taken to the enslaved miners in the mines (two made, the mines' lowest and middle floors), never back to the "
         "slave. The page's five kinds of key, each freeing its own miner, are one key and either miner here (the engine's "
         "steps name a creature, not one spawn of it). Chance 0.5 ours."),
    dict(key=23, name="Poison Stingers", giver="Apothecary Nadira", levels=(43, 43), xp=730944, tunar=0, factions="",
         item=("Poison Stinger", "Heavy Venom Sac", "A singetail scorpion's stinger. Apothecary Nadira of the Oasis brews with them.", NT99),
         target="singetail scorpion", also=["singetail scropion", "singletail scorpion"], chance=0.25,
         offer="The singetail scorpions round the Oasis carry a poison I brew into a cure. Bring me four stingers.",
         loot="Kill singetail scorpions for four poison stingers.", give="Bring four poison stingers to Apothecary Nadira.",
         says="Careful with those. Thank you.", notes="teamjosh H&G key 23 (the owner's source, 2026-11-15): 43, 730,944, its "
         "giver unnamed ('- Oasis'): Apothecary Nadira, ours. Poison stingers off the Singetail Scorpions; 'Rare drop': 0.25, "
         "ours. Count 4 ours."),
    dict(key=11, name="Froglok Scalps", giver="Big Basher Biggum", levels=(14, 34), xp=105984, tunar=0, factions="",
         item=("Froglok Scalp", "Zombie Patron Skin", "A froglok's scalp from Guk. Big Basher Biggum outside Grobb pays for them.", NT99),
         target="froglok tal shaman", also=["froglok questor", "froglok scout", "froglok warrior", "froglok villager",
                                            "froglok guk sage", "froglok tal wizard", "froglok tsu shaman",
                                            "froglok guk forager", "froglok templar", "froglok vis knight"],
         chance=0.5, offer="Biggum hate da frogloks of Guk. Bring Biggum four froglok scalps.",
         loot="Kill the frogloks of Guk for four froglok scalps.", give="Bring four froglok scalps to Big Basher Biggum.",
         says="Hah! Good scalps.", notes="teamjosh H&G key 11 (the owner's source, 2026-11-15): Big Basher Biggum just outside "
         "Grobb, the frogloks of Guk, loggable 14-34. The page names no item and gives no experience: the froglok scalp "
         "(FLAGGED: named from the page's creature) and 105,984 (a 19-25 turn-in's) are ours; Guk's frogloks of 19-34 count. "
         "Count 4 and chance 0.5 ours."),
    dict(key=2, name="Lioness Pelts", giver="Sprexy Lionhunter", levels=(6, 8), xp=1960, tunar=0, factions="",
         item=("Lioness Pelt", "Matted Bear Hide", "A lioness's pelt from round Highbourne. Sprexy Lionhunter of the arena pays for them.", NT99),
         target="rogue lion", also=["lioness cub"], chance=0.5, rewards=["pie"],
         offer="The lionesses round Highbourne prowl too near my arena. Bring me four pelts and I will bake you a pie.",
         loot="Kill the lions round Highbourne for four lioness pelts.", give="Bring four lioness pelts to Sprexy Lionhunter.",
         says="Fine pelts. Here, a pie for you.", notes="teamjosh H&G key 2 (the owner's source, 2026-11-15): 6-8, Sprexy the "
         "Lionhunter (the game's Sprexy Lionhunter) at Highbourne's arena, the lionesses near Highbourne, a pie (the page's "
         "reward). The page names no item and gives no experience: the lioness pelt (FLAGGED: named from the page's "
         "creature) and 1,960 (the 6-8 turn-ins') are ours; the rogue lions and the lioness cub by Highbourne (6-8) count. "
         "Count 4 and chance 0.5 ours."),
]

# the Sharkskin Shield, the page's: "SHIELD HP: 1900/1900 DUR: 50 Race: ALL Class: WAR RAN PAL SK BRD RGE DRD SHA CL
# Level: 10 HPMax: 15 AC: 35 STA: 5" (no monk: classuse 1007, the Wooden Shield's own; DUR 50 has no column)
SHIELD = ("Sharkskin Shield", "Wooden Shield",
          "A shield crafted from a wooden frame and tattered shark skins by Abrion Oleken of Stormhaven.",
          {"AC": 35, "sta": 5, "HPMAX": 15, "maxhp": 1900, "levelreq": 10, "classuse": 1007, "trade": 0})

# the pages whose giver the game has under another name: (key, giver, item, levels, xp)
SAME = [(25, "Tuskmaster Helgley", "plagueswine tusk", (46, 50), 251680,
         "teamjosh H&G key 25 (the owner's source, 2026-11-15): Tuskcarver, N of the 'prot in' in the Plane of Disease, is this "
         "quest's Tuskmaster Helgley: its levels and experience (its '800ish' tunar left as the 956 here)."),
        (30, "Pluxklez", "bulging toad eye", (58, 58), 1200000,
         "teamjosh H&G key 30 (the owner's source, 2026-11-15): its 'NPC - near banker in PoD' with toad eyes for a soup is "
         "this quest's Pluxklez (the stew): its level and experience.")]


def cols(db, t):
    return [r[1] for r in db.execute(f'PRAGMA table_info("{t}")')]


def has(db, t):
    return sgg.has(db, t)


def put_npc(db, row, extra, say, label):
    """An npcs row, and its rows by name in other tables ({table: [rows]}: npcclass, npcfaction, lootcreature, ...),
    each where this database has the table and no row of the name."""
    c = cols(db, "npcs")
    row = {k: v for k, v in row.items() if k in c and k != "id"}
    cur = db.execute(f'INSERT INTO npcs ({",".join(row)}) VALUES ({",".join("?" * len(row))})', list(row.values()))
    nid = cur.lastrowid
    if "npc_id" in c:
        db.execute("UPDATE npcs SET npc_id = ? WHERE id = ?", (nid, nid))
    for t, rows in extra.items():
        if not has(db, t):
            continue
        tc = cols(db, t)
        namecol = "npc_name" if "npc_name" in tc else "name"
        if db.execute(f'SELECT 1 FROM "{t}" WHERE lower(trim({namecol})) = ?', (h.key(row["npc_name"]),)).fetchone():
            continue
        for r in rows:
            r = {k: v for k, v in r.items() if k in tc and k != "id"}
            db.execute(f'INSERT OR IGNORE INTO "{t}" ({",".join(r)}) VALUES ({",".join("?" * len(r))})', list(r.values()))
    h.npc_name(db, "")
    h._NAMES[id(db)].setdefault(h.key(row["npc_name"]), row["npc_name"])
    say(f"  NPC {row['npc_name']} ({nid}) w{row['world']} ({row['x_coord']}, {row['y_coord']}, {row['z_coord']}): {label}")
    return nid


def heirloom_givers(db, say):
    with open(GIVERS) as f:
        d = json.load(f)
    n = 0
    for g in d["givers"]:
        if h.npc_name(db, g["npc"]["npc_name"]):
            continue
        put_npc(db, g["npc"], g["rows"], say, "Heirloom's row")
        n += 1
    return n


def new_npcs(db, say):
    made = 0
    for name, like, w, x, y, z, facing, level, by, why in NPCS:
        here = [r for r in db.execute("SELECT x_coord, z_coord FROM npcs WHERE lower(trim(npc_name)) = ? AND world = ?", (h.key(name), w))
                if abs(r[0] - x) < 1 and abs(r[1] - z) < 1]
        if here:
            continue
        src = db.execute("SELECT * FROM npcs WHERE lower(trim(npc_name)) = ? ORDER BY id LIMIT 1", (h.key(like),)).fetchone()
        anchor = db.execute("SELECT zone FROM npcs WHERE lower(trim(npc_name)) = ? AND world = ? ORDER BY id LIMIT 1", (h.key(by), w)).fetchone()
        if not src or not anchor:
            say(f"  NPC {name}: not made, no {like!r} to copy or no {by!r} to stand by")
            continue
        row = dict(zip(cols(db, "npcs"), src))
        row.update(npc_name=name, world=w, x_coord=x, y_coord=y, z_coord=z, facing=facing, npc_level=level, npc_type=128,
                   zone=anchor[0])
        cls = [dict(name=h.key(name), **{"class": r[0]}, source=SET) for r in
               db.execute("SELECT class FROM npcclass WHERE name = ?", (h.key(like),))] if has(db, "npcclass") else []
        put_npc(db, row, {"npcclass": cls[:1]}, say, why)
        made += 1
    return made


def add_quest(db, q, say):
    giver, to = h.npc_name(db, q["giver"]), h.npc_name(db, q.get("give_to", q["giver"]))
    target = h.npc_name(db, q["target"])
    if not (giver and to and target):
        say(f"  key {q['key']} {q['name']}: not made, " + ", ".join(n for n, v in ((q["giver"], giver), (q.get("give_to", ""), to),
                                                                                   (q["target"], target)) if n and not v) + " not here")
        return False
    name, like, desc, sets = q["item"]
    pid = h.item_for(db, name, like, desc, sets, say)
    rewards = []
    for r in q.get("rewards", []):
        if r == SHIELD[0]:
            rewards.append(h.item_for(db, *SHIELD, say))
        else:
            ids = h.item_ids(db, r)
            if not ids:
                raise SystemExit(f"no item {r!r} for {q['name']}")
            rewards.append(ids[0])
    n = q.get("count", 4)
    qid = h.next_quest(db)
    h.put_row(db, "quests", dict(questid=qid, name=q["name"], enabled=1, giver=giver, minlevel=q["levels"][0], maxlevel=q["levels"][1],
                                 repeatable=1, offer=q["offer"], accept="I will do it.", decline="Not now.",
                                 declined="Then go. Come back when you are ready.", started="Off you go. I will be here.",
                                 source=h.QSOURCE.replace("handg_quests", "handg_givers"), notes=q["notes"]))
    items = f"{pid}*{n}" if n > 1 else str(pid)
    h.put_row(db, "queststeps", dict(questid=qid, step=1, kind="loot", target=target, items=items, count=n, chance=q["chance"],
                                     journal=q["loot"]))
    h.put_row(db, "queststeps", dict(questid=qid, step=2, kind="give", target=to, items=items, count=1, journal=q["give"],
                                     says=q["says"], xp=q["xp"], rewardtunar=q["tunar"], factions=q["factions"],
                                     rewarditems=h.counts((r, 1) for r in rewards)))
    gone = []
    for a in q["also"]:
        if h.npc_name(db, a):
            db.execute("INSERT OR IGNORE INTO queststeptargets (questid, step, target) VALUES (?, 1, ?)", (qid, h.npc_name(db, a)))
        else:
            gone.append(a)
    for j in (q["loot"], q["give"]):
        if len(j) > 127:
            raise SystemExit(f"journal too long: {j}")
    say(f"  quest {qid} {q['name']} (key {q['key']}) made{'; not here: ' + ', '.join(gone) if gone else ''}")
    return True


def quest_of(db, giver, item):
    """The giver's quests handing the item in (to anyone)."""
    ids = set(h.item_ids(db, item))
    out = []
    for (q,) in db.execute("SELECT questid FROM quests WHERE lower(trim(giver)) = ?", (h.key(giver),)):
        if any(s["kind"] == "give" and any(p in ids for p, _ in h.parse_items(s["items"])) for s in h.steps(db, q)):
            out.append(q)
    return out


def fix_same(db, say):
    n = 0
    for k, giver, item, (lo, hi), xp, note in SAME:
        for g in h.find(db, h.Q(giver, item)):
            q = g["questid"]
            if (g["minlevel"], g["maxlevel"]) != (lo, hi):
                db.execute("UPDATE quests SET minlevel = ?, maxlevel = ? WHERE questid = ?", (lo, hi, q))
                say(f"  key {k} quest {q}: levels {g['minlevel']}-{g['maxlevel']} -> {lo}-{hi}")
                n += 1
            if g["give"]["xp"] != xp:
                db.execute("UPDATE queststeps SET xp = ? WHERE questid = ? AND step = ?", (xp, q, g["give"]["step"]))
                say(f"  key {k} quest {q}: xp {g['give']['xp']:,} -> {xp:,}")
                n += 1
            db.execute("UPDATE quests SET notes = notes || ? WHERE questid = ? AND instr(notes, ?) = 0", (" " + note, q, note))
    return n


def report(db, say):
    src, copies, _ = h.source()
    say("Heirloom's givers and their quests:")
    with open(GIVERS) as f:
        givers = json.load(f)["givers"]
    for g in givers:
        say(f"  {g['npc']['npc_name']}: {'here' if h.npc_name(db, g['npc']['npc_name']) else 'to copy'}")
    for k in list(h.COPIES) + [82]:
        got = [m for m in h.PRESENT[k] if h.find(db, m)]
        say(f"  key {k:>2} {src[k]['item']}: {'here' if got else 'to make'}")
    say("new givers:")
    for name, like, w, x, y, z, *_ in NPCS:
        here = [r for r in db.execute("SELECT x_coord, z_coord FROM npcs WHERE lower(trim(npc_name)) = ? AND world = ?", (h.key(name), w))
                if abs(r[0] - x) < 1 and abs(r[1] - z) < 1]
        say(f"  {name} w{w} ({x}, {y}, {z}) like {like}: {'here' if here else 'to make'}")
    say("their quests:")
    for q in QUESTS:
        got = quest_of(db, q["giver"], q["item"][0])
        say(f"  key {q['key']:>2} {q['name']} ({q['giver']}, {q['levels'][0]}-{q['levels'][1]}, {q['xp']:,}): {'here ' + str(got) if got else 'to make'}")
    say("the game's under another name:")
    for k, giver, item, (lo, hi), xp, _ in SAME:
        for g in h.find(db, h.Q(giver, item)):
            ok = (g["minlevel"], g["maxlevel"]) == (lo, hi) and g["give"]["xp"] == xp
            say(f"  key {k} quest {g['questid']} {giver}: {g['minlevel']}-{g['maxlevel']}, {g['give']['xp']:,}"
                f"{'' if ok else f' -> {lo}-{hi}, {xp:,}'}")


def run_on(db, say):
    src, copies, items = h.source()
    say("Heirloom's givers:")
    ng = heirloom_givers(db, say)
    na = 0
    for k in list(h.COPIES) + [82]:
        if any(h.find(db, m) for m in h.PRESENT[k]):
            continue
        if k == 82:
            n = h.NEW[k]
            if h.npc_name(db, n["giver"]) and h.npc_name(db, n["target"]):
                h.add_new(db, k, n, say)
                na += 1
        else:
            c = copies[k]
            if h.npc_name(db, c["quest"]["giver"]) and h.npc_name(db, c["steps"][0]["target"]):
                h.add_copy(db, k, c, items, say)
                na += 1
    say("new givers:")
    nn = new_npcs(db, say)
    say("their quests:")
    nq = 0
    for q in QUESTS:
        if quest_of(db, q["giver"], q["item"][0]):
            continue
        nq += add_quest(db, q, say)
    say("the game's under another name:")
    nf = fix_same(db, say)
    say(f"{ng} givers copied, {na} of their quests; {nn} NPCs made, {nq} quests; {nf} figures put right")


def audit_rows(db, counts_):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    for tbl, op, rid, old in db.execute(f"SELECT tbl, op, rid, old FROM {sgg.LOG} WHERE setname = ? ORDER BY seq", (SET,)).fetchall():
        if tbl in ("sqlite_sequence", sgg.LOG) or op == "made":
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
               "VALUES (?, ?, ?, ?, '', 'update', ?, ?, NULL, NULL, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, sgg.LOG, json.dumps({"setname": SET}), f"undone, {n} rows put back: " + DETAIL))


def is_in(db):
    return has(db, sgg.LOG) and db.execute(f"SELECT 1 FROM {sgg.LOG} WHERE setname = ?", (SET,)).fetchone() is not None


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
        for t in ("quests", "queststeps", "queststeptargets", "itempattern", "npcs", "factions", "gm_audit"):
            if not has(db, t):
                print(f"refused, nothing done: no table {t}")
                return 1
        if a.undo:
            if not is_in(db):
                print("nothing to undo")
                return 0
            sgg.undo(db, SET, say, extra=audit_undo)
            print("NPCs: the GM panel's reload_npcs (or a restart); quests: a restart")
            return 0
        report(db, say)
        if is_in(db):
            print(f"applied already ({SET} is in {sgg.LOG}): nothing to do (--undo to take it back)")
            return 0
        bad = [f for q in QUESTS for f in [x.split(":")[0] for x in q["factions"].split(", ") if x]
               if not db.execute("SELECT 1 FROM factions WHERE name = ?", (f,)).fetchone()]
        if bad:
            print(f"refused, nothing done: no faction {bad}")
            return 1
        print()
        sgg.run = lambda db_, spec, say_: run_on(db_, say_)
        sgg.apply(db, {"set": SET}, a.apply, say, extra=audit_rows)
        if a.apply:
            print("NPCs: the GM panel's reload_npcs (or a restart); quests are read at the server's start: the next restart")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
