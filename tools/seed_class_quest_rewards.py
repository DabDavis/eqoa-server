#!/usr/bin/env python3
"""The records the city class quests' rewards lack, made, and those quests filled (the user,
2026-10-30, of tools/fill_class_quest_rewards.py's 100 quests not filled: all three).

1. **Scrolls.** A quest spell whose class has no scroll handlers/scribe.go can scribe (warrior
   Taunt, rogue Sneak and Acrobatics, Tethelin's ranger and druid spells, Fayspire's paladin spells,
   the level-20 paths of rangers, paladins, monks, druids and the bard's Sweeping Combat) gets one,
   made as the Return Home quest scrolls are (Malice's record, 8553: no trade, lore, no cost): the
   spell's name (with the class's tag, "Sneak (RGE)", where another item has the name: the quest files
   name items by name, and scribe.go reads the tag off), "This allows you to scribe X into your spell
   book.", the class's mask, the spell's level (the quest's where that is lower: the wizard's Arcane
   Bindings record is of level 4, its quest of 3). Each is checked as scribe.go reads it: it scribes a
   record of its spell's name at the class's level, the class's own or its twin. Return Home's "Sneak
   Scroll" (8539), "Acrobatics Scroll" (8543) and "swift blade scroll" (8463) name no spell and stay
   as they are (nothing here sells, holds or names them). The quests are filled as
   tools/fill_class_quest_rewards.py fills them: the scroll on the last step, a level-20 quest the
   choice of its two path scrolls, and a level-20 quest that tool gave one path (the bard's) the
   choice of both, its item kept.
2. **The items the guides name**, 26 quests: a record made for each as tools/seed_newgear_rewards.py
   made New Gear's, a copy of a fitting record (NAMED: the same slot, about the quest's level, a
   record the class may wear; where none is, the nearest of the kind with the quest's class: said
   for each) under the guide's name with a line saying who made it of what. Hit points the copy
   lacks are tools/fill_weapon_durability.py's for its level (200 a level: 800 at 4-5, 2000 at 10),
   durability 50. The guide's "Small Stiletto" is the database's "Small Stilletto" (5831), given as is.
3. **The item ties and misfits** (29 quests) are written for the user to pick (--item-review FILE,
   REVIEW): the candidates with their numbers and a suggestion. None is applied.

A character of the quest's class who finished it (charquests done) is given its scroll or item as a
quest gives one, unless it holds it or, for a scroll, knows the spell; a level-20 choice is the
player's to pick (reported); a full inventory (40) is reported. Every row made or changed is logged
in class_quest_seed_log; --undo takes back only what is still as this left it, and keeps a record
someone holds. One transaction. --write-data DIR names the same rewards in the quest files there
(text, their layout). The records are SCROLLS' and NAMED's, at fixed ids: on a database the quest files
are imported into afresh, --apply --records-only first makes them, so the import finds every reward
the files name (else it writes those quests as drafts). The quests load at the server's start: a
restart (or a GM panel quest save, which reloads them) after --apply; grants go straight to
charinventory: apply with the receivers offline.

    tools/seed_class_quest_rewards.py [--db eqoa.db] [--apply | --undo] [--records-only] [--tsv FILE]
                                      [--write-data tools/quests/data] [--item-review FILE]
"""

import argparse
import collections
import csv
import json
import os
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import fill_class_quest_rewards as fc  # noqa: E402
import fill_weapon_durability as fwd  # noqa: E402

ABBR = ["WAR", "RAN", "PAL", "SK", "MNK", "BRD", "RGE", "DRD", "SHA", "CL", "MAG", "NEC", "ENC", "WIZ", "ALC"]
SCROLL_TEMPLATE = 8553  # Malice: a Return Home quest scroll
SCROLL_LINE = "This allows you to scribe {} into your spell book."
STAMP = "tools/seed_class_quest_rewards.py, 2026-10-30"

SCROLLS = [  # (id, class, its spell, the scroll's name, its level): the scrolls this makes
    (707000, 0, 2295, "Taunt", 5),                  # scribe.go scribes 183, Return Home's number of the same spell
    (707001, 1, 700022, "Swift Blade", 3),
    (707002, 1, 700045, "Forage Berries", 5),
    (707003, 1, 700056, "Tangling Weeds", 7),
    (707004, 1, 700120, "Flametongue", 13),
    (707005, 1, 700134, "Tangling Roots", 15),
    (707006, 1, 700216, "Sparrow Eye", 20),
    (707007, 1, 700222, "Strikeblade", 20),
    (707008, 2, 700018, "Hero Guard", 3),
    (707009, 2, 700046, "Lay Hands", 5),
    (707010, 2, 700055, "Protectorate", 7),
    (707011, 2, 700123, "Towering Wall", 13),
    (707012, 2, 700131, "Knight's Bash", 15),
    (707013, 2, 700188, "Fortify", 20),
    (707014, 2, 700213, "Shining Bastion", 20),
    (707015, 4, 700221, "Stone Stance", 20),
    (707016, 4, 700219, "Stasis Strike", 20),
    (707017, 5, 160, "Anthem of Light", 13),        # the spell is "Anthem of Light +" (a bard's song)
    (707018, 6, 700021, "Sneak (RGE)", 3),          # "Sneak" is the ranger's scroll 700002
    (707019, 6, 700051, "Acrobatics", 7),
    (707020, 7, 343, "Fiery Burst", 3),
    (707021, 7, 346, "Snare", 5),
    (707022, 7, 347, "Nature's Blessing", 7),
    (707023, 7, 354, "Regenerative Aura", 13),
    (707024, 7, 362, "Enkindle", 20),
    (707025, 7, 700178, "Creeping Health", 20),
    (707026, 8, 700130, "Endure Poison", 15),
    (707027, 11, 333, "Ward Death (NEC)", 13),      # "Ward Death" is the cleric's scroll 8408
    (707028, 13, 767, "Arcane Bindings", 3),        # the record is of level 4, the quest of 3
    (707029, 5, 700225, "Sweeping Combat", 20),
]

# The guides' items: (quest, the new record's id, the quest's class, the guide's name, the record copied,
# the record whose look it takes (None: its own), the class set (the quest's) when the record is not the
# class's, the line, why).
NAMED = [
    (100360, 707100, 0, 'Guard Sword', 8571, None, False,
     'Forged by Sergeant Grem from an oilstone, a gold spring, a cracked rat tooth and a fractured firefly thorax.',
     "the warrior's level-4 sword, Freeport's quest's (Neriak's Firefly Blade and Oggok's Eagle Feather Blade "
     'copy it)'),
    (100550, 707101, 0, "Guard's Sword", 8571, None, False,
     'Forged by Lancet Carroll from iron ore and a leather strip, for a defaced coin won back.',
     "the warrior's level-4 sword, Freeport's quest's"),
    (100280, 707102, 3, 'Wicked Longsword', 5703, 5859, False,
     'Forged by Desh the Harvester from iron ore, a leather strip and a flattened tarantula eye.',
     "the shadow knight's level-4 weapon, Freeport's Iron Hatchet, with a sword's look (the Stone Sword's) as "
     "Neriak's Black Raven Sword"),
    (100613, 707103, 5, "Messenger's Blade", 6042, None, False,
     "Given by Jenny Sweethy for Vithara Swiftpaw's boots.",
     "the bard's level-10 sword, Freeport's and Tethelin's quests'"),
    (100350, 707104, 6, "Footpad's Shank", 8540, None, False,
     'Forged by Junker Plim from iron ore, a copper spring, bent rat whiskers and a cracked ant pincer.',
     "the rogue's level-4 dagger, Freeport's quest's (Neriak's Tarantula Dirk copies it)"),
    (100600, 707105, 6, 'Ivory Dirk', 8540, None, False,
     'Forged by Crow Darkstride from iron ore, ivory and a poison sac.',
     "the rogue's level-4 dagger, Freeport's quest's"),
    (100207, 707106, 6, "The Slicer's Special", 8544, None, False,
     'Forged by Juno Felligan with the silk of the ice spider queen.',
     "the rogue's level-10 dagger, six cities' quests'"),
    (100603, 707107, 6, "Crow's Parrying Dagger", 8544, None, False,
     "Made by Crow Darkstride from an etched dagger's blade and hilt.",
     "the rogue's level-10 dagger, six cities' quests'"),
    (100234, 707108, 9, 'Ironsun Mace', 6055, None, False,
     'Forged by Bishop Felkahn from a leather strip, iron ore and a cracked rat tooth.',
     "the cleric's level-4 hammer, five cities' quests'"),
    (100310, 707109, 9, "Initiate's Mallet", 6055, None, False,
     'Made by Master Micha from lubricating oil, dusting feathers and a cracked ant pincer.',
     "the cleric's level-4 hammer, five cities' quests'"),
    (100237, 707110, 9, 'Ironsun Morningstar', 5863, None, False,
     "Given by Bishop Felkahn for Rory Redface's ring.",
     "the cleric's level-10 mace, five cities' quests'"),
    (100313, 707111, 9, 'Dogtooth Mace', 5863, None, False,
     "Made by Renard of the skeletal dogs' chewed-up bones.",
     "the cleric's level-10 mace, five cities' quests'"),
    (100563, 707112, 9, "Battered Defender's Club", 5863, None, False,
     'Given by Vedilion Brithstar for the small idol.',
     "the cleric's level-10 mace, five cities' quests'"),
    (100633, 707113, 10, 'Knotted Pine Staff', 8449, None, False,
     "Given by Ammathor Lithkin for the darkpaw brutes' fangs.",
     "the magician's level-10 staff, five cities' quests'"),
    (100260, 707114, 11, 'Staff of Necromancy', 8585, None, True,
     'Carved by Retainer Alishal from a leather strip, a wooden shaft and a cracked rat claw.',
     "no staff of level 3-6 a necromancer may wear: Highbourne's wizard's level-4 staff of evocation (the same "
     "city's caster staff of the level), for the necromancer"),
    (100263, 707115, 11, 'Whitebone Dagger', 8528, None, False,
     "Given by Ammathor Lithkin for the Stoneclaw necromancer's necklace.",
     "the necromancer's level-10 dagger, four cities' quests'"),
    (100244, 707116, 12, "Keeper's Staff", 5740, None, False,
     "Given by Master Delar for the traitor Ganron's head.",
     "the enchanter's level-10 staff, Qeynos's quest's"),
    (100293, 707117, 13, 'Crimson Staff', 8584, None, False,
     "Given by Ammathor Lithkin for the Stoneclaw necromancer's pendant.",
     "the wizard's level-10 staff, four cities' quests'"),
    (100004, 707118, 14, 'Venomguard Bracers', 1256, None, False,
     'Treated by Venathorn with a drained poison sac, from burlap bracers.',
     "no alchemist's bracers of level 3-6: Cloth Bracers, level 5, which any class may wear"),
    (100224, 707119, 14, "Alchemist's Log", 5334, 8403, True,
     'Given by Truborne Jaegir for a cracked beetle horn and a hundred tunar.',
     'no held item below level 8 an alchemist may hold: the Iron Holy Symbol of Brell (level 1, AC 1), for the '
     "alchemist, with the book of alchemical symbols' look"),
    (100300, 707120, 14, 'Soft Iron Shaft', 6055, None, True,
     'Made by Chemist Ambrakma for a glass beaker, a wooden tube and shattered ant chitin.',
     "no weapon of level 4-5 an alchemist may wear: the clerics' Acolyte's Hammer (level 5, 50 damage, the "
     "alchemists' Alchemy Hammer's look), for the alchemist"),
    (100640, 707121, 14, "Alchemist's Gloves", 6854, None, True,
     'Treated by Kelin from tailored gloves and a cracked rat claw.',
     "the guide's item's own record, the old dump's Alchemist Gloves (level 4, AC 5, fire 5, cold 2), its "
     'class mask blank: for the alchemist'),
    (100007, 707122, 14, 'Alchemist Apprentice Staff', 1293, None, False,
     'Given by Venathorn for the wings Amelie wanted.',
     "the casters' level-9 staff of learning, which an alchemist may wear (no alchemist's staff of level 10)"),
    (100303, 707123, 14, 'Brown Gnarled Staff', 1833, None, False,
     "Given by Lanlin Ogledmaggen for Lyriam's note.",
     "a gnarled staff of level 9 any class may wear (no alchemist's staff of level 10)"),
    (100643, 707124, 14, 'Small Dagger', 8355, None, False,
     'Given by Jergish Anaebraum for the jar of ooze.',
     "the alchemist's level-10 dagger, Freeport's quest's"),
]
# a guide's item the database has under another spelling: given as it is
EXISTING = {100573: (5831, "Small Stiletto", "the database's Small Stilletto, the wizard's level-10 dagger")}

# The ties and misfits for the user (none applied): quest -> (suggested record or None, why)
REVIEW = {
    100184: (8571, "Freeport's militia short sword (Neriak's and Oggok's New Gear blades are copies of it); or a "
                   "copy named for the quest, as New Gear's"),
    100194: (8571, "the same; Halas's guide: Derry makes a weapon of an oilstone, iron ore and a bat wing"),
    100410: (8571, "the same"),
    100680: (8571, "the same"),
    100187: (5864, "Freeport's Sturdy Two-Handed Sword (the other cities' level-10 fills are Freeport's). All three "
                   "candidates carry no class mask (the old dump's: the server lets any class wear them, the client "
                   "marks them unusable): Freeport's, Klick'Anon's and Qeynos's own rewards included"),
    100197: (5864, "the same"),
    100413: (5864, "the same"),
    100493: (5864, "the same"),
    100543: (5864, "the same"),
    100683: (5864, "the same"),
    100164: (5703, "Freeport's Iron Hatchet (Neriak's Black Raven Sword and Oggok's Centipede Hatchet copy it); or a "
                   "copy named for the quest"),
    100710: (5897, "Traveler's Rapier: a rapier, as the bard's L20 path weapons (Troupe Foil is a blunt weapon)"),
    100400: (8540, "Freeport's tiger dirk (Neriak's Tarantula Dirk copies it)"),
    100670: (8540, "the same"),
    100740: (8540, "the same"),
    100174: (5575, "Corse Spiked Club, the shaman's level-5 club Oggok's Eagle Talon Club copies (its line names "
                   "Oggok's smith)"),
    100214: (5575, "the same"),
    100034: (5843, "the title is a staff: Staff of Conjuring, the magician's level-5 staff (the others give robes)"),
    100330: (5843, "the same"),
    100340: (None, "the title is a staff: the Staff of Necromancy's record made for Highbourne (a caster staff of "
                   "level 4 for the necromancer); or Freeport's Black Robe of the Order"),
    100024: (None, "the title is a staff and no enchanter's staff of level 3-6 exists: the wizard's staff of "
                   "evocation for the enchanter (as Highbourne's Staff of Necromancy); or Freeport's yellow "
                   "enchanter's robe"),
    100320: (None, "the same"),
    100027: (8425, "the title is a mallet: Freeport's rod of eyes (a one-handed blunt weapon)"),
    100323: (8425, "Freeport's rod of eyes (Qeynos's Manawood Staff is the other)"),
    100443: (5740, "the title is a staff: Qeynos's Manawood Staff"),
    100054: (8585, "the title is a staff: Highbourne's staff of evocation (level 4)"),
    100056: (8583, "Freeport's fancy slippers (Qeynos's Arcane Apprentice's Boots carry no class mask)"),
    100502: (8583, "the same"),
    100423: (8355, "Freeport's small dirk (the base of Qeynos's Small Dagger)"),
}

LOG = "class_quest_seed_log"
LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl TEXT NOT NULL,
    rid TEXT NOT NULL,
    col TEXT NOT NULL,
    old TEXT,
    new TEXT
)"""


def row(db, sql, args=()):
    cur = db.execute(sql, args)
    r = cur.fetchone()
    return None if r is None else dict(zip([c[0] for c in cur.description], r))


def display(name):
    """A spell's name as its scroll is named: a bard's song without its '+'."""
    return name.rstrip("+ ").strip()


class Plan:
    def __init__(self, db):
        self.db = db
        self.rec = fc.Records(db)
        self.quests = fc.load(db, self.rec)
        self.rec.city_races(db, self.quests)
        self.made = []      # itempattern rows to insert
        self.fills = []     # quest fills
        self.problems = []
        self.scroll_of = {}  # (class, spell key) -> the scroll SCROLLS makes
        self.item_of = {}    # quest -> the record NAMED makes for it

    # -- records

    def place(self, r, why):
        """The record r at its id: made, or found made already (the same name); a problem when
        another record holds the id or another item has the name."""
        here = row(self.db, "SELECT itemname FROM itempattern WHERE patternid = ?", (r["patternid"],))
        if here is not None:
            if here["itemname"] != r["itemname"]:
                self.problems.append(f"id {r['patternid']} is {here['itemname']!r} here, not {r['itemname']!r}")
                return False
            return True  # made by an earlier run
        others = [x for x in self.rec.by_fold.get(fc.fold(r["itemname"]), []) if x != r["patternid"]]
        if others:
            self.problems.append(f"{r['itemname']!r}: item {others[0]} has the name: the quest files could not name it")
            return False
        self.made.append(dict(row=r, why=why))
        self.rec.item[r["patternid"]] = dict(name=r["itemname"], scroll=r["itemicon"] == fc.SCROLL_ICON and r["equipslot"] == -1,
                                             level=r["levelreq"], classes=r["classuse"], races=r["raceuse"],
                                             maxhp=r["maxhp"] or 0, slot=r["equipslot"], desc=r["itemdesc"])
        self.rec.by_fold[fc.fold(r["itemname"])].append(r["patternid"])
        self.rec.by_fold[fc.fold(r["itemname"])].sort()
        return True

    def make_records(self):
        db = self.db
        tmpl = row(db, "SELECT * FROM itempattern WHERE patternid = ?", (SCROLL_TEMPLATE,))
        if tmpl is None:
            self.problems.append(f"the scroll template {SCROLL_TEMPLATE} is not here")
            return
        for pid, c, sid, name, level in SCROLLS:
            if sid not in self.rec.spell:
                self.problems.append(f"scroll {pid} {name!r}: spell {sid} is not here")
                continue
            sname, slevel = self.rec.spell[sid]
            key = fc.spell_key(sname)
            r = dict(tmpl, patternid=pid, itemname=name, itemdesc=SCROLL_LINE.format(display(sname)), levelreq=level,
                     classuse=1 << c, raceuse=1023)
            if not self.place(r, f"the {ABBR[c]} scroll of {sname} (spell {sid}, level {slevel})"):
                continue
            # read back as scribe.go reads it: it scribes a record of its spell at the class's level
            good, bad = self.rec.scrolls_for(key, c, level)
            if pid not in good:
                self.problems.append(f"scroll {pid} {name!r} would not scribe {sname}: {dict(bad).get(pid)}")
                continue
            self.scroll_of[(c, key)] = pid
        hp = fwd.level_hp(db)
        for qid, pid, c, name, base, look, set_class, line, why in NAMED:
            b = row(db, "SELECT * FROM itempattern WHERE patternid = ?", (base,))
            if b is None:
                self.problems.append(f"quest {qid}: the record {base} to copy is not here")
                continue
            r = dict(b, patternid=pid, itemname=name, itemdesc=line)
            if set_class:
                r["classuse"] = 1 << c
            if look is not None:
                lk = row(db, "SELECT model, itemicon FROM itempattern WHERE patternid = ?", (look,))
                r.update(model=lk["model"], itemicon=lk["itemicon"])
            if not r["maxhp"]:
                r["maxhp"] = fwd.hp_for(r["levelreq"], hp)  # tools/fill_weapon_durability.py's
            if not r["duration"]:
                r["duration"] = fwd.DURATION
            if not r["classuse"] >> c & 1:
                self.problems.append(f"quest {qid}: {name!r} (a copy of {base}) is not the class's")
                continue
            if self.place(r, f"a copy of {b['itemname']} ({base}{', ' + str(look) + chr(39) + 's look' if look else ''}"
                             f"{', for the ' + fc.CLASSES[c] if set_class else ''}): {why}"):
                self.item_of[qid] = pid

    def make_scroll(self, c, key, quest_level):
        pid = self.scroll_of.get((c, key))
        if pid is None:
            self.problems.append(f"no scroll in SCROLLS for the {ABBR[c]} spell {key!r}: add one")
        elif self.rec.item[pid]["level"] > quest_level:
            self.problems.append(f"scroll {pid} is of level {self.rec.item[pid]['level']}, its quest of {quest_level}")
            return None
        return pid

    # -- the plan

    def build(self, records_only=False):
        db, rec = self.db, self.rec
        self.make_records()
        if records_only:
            return
        _, _, reports = fc.plan(db)
        filled_by_first = {}
        if fc.has_table(db, fc.LOG):
            for rid, new in db.execute(f"SELECT rid, new FROM {fc.LOG} WHERE tbl = 'queststeps' AND col = 'rewarditems'"):
                filled_by_first[rid] = json.loads(new)
        # 1. a quest spell with no scroll: the scroll made, the quest given it
        for r in sorted(reports, key=lambda r: (r["c"], r["level"], r["city"])):
            q = self.quests[r["qid"]]
            if r["what"] == "no scroll":
                key = fc.spell_key(r["spell"])
                pid = self.make_scroll(q["c"], key, q["level"])
                if pid:
                    self.add_fill(q, scroll=pid, spell=key, why=f"the {display(r['spell'])} scroll, made for it")
            elif r["what"] == "no scroll (level-20 path)":
                self.l20_choice(q, keep=None)
        # 1b. a level-20 quest the first tool gave one path (its scroll the step's whole spell reward)
        for q in sorted(self.quests.values(), key=lambda q: (q["c"], q["city"])):
            if q["level"] != 20 or q["c"] not in fc.L20_PATHS or q["last"]["choices"]:
                continue
            rid = f"{q['qid']}/{q['last']['step']}"
            if filled_by_first.get(rid) != q["last"]["rewarditems"]:
                continue
            keys = [k for k, *_ in q["spells"]]
            if len(keys) == 1 and keys[0] in fc.L20_PATHS[q["c"]]:
                self.l20_choice(q, keep=keys[0])
        # 2. the guides' items
        for qid, pid, c, name, base, look, set_class, line, why in NAMED:
            q = self.quests.get(qid)
            if q is None:
                self.problems.append(f"quest {qid} is not a city class quest here")
                continue
            if q["c"] != c:
                self.problems.append(f"quest {qid} is a {fc.CLASSES[q['c']]}'s, not a {fc.CLASSES[c]}'s")
                continue
            if pid in q["items"]:
                continue  # given already
            if q["items"]:
                self.problems.append(f"quest {qid}: its last step gives an item already: left")
                continue
            if self.item_of.get(qid) == pid:
                self.add_fill(q, item=pid, why=f"{name}, the item the guide names, made for this server ({line[:-1]})")
        for qid, (pid, guide, why) in EXISTING.items():
            q = self.quests.get(qid)
            if q is not None and pid in q["items"]:
                continue  # given already
            if q is None or q["items"]:
                self.problems.append(f"quest {qid}: not a city class quest here, or it gives an item already")
                continue
            if pid not in rec.item:
                self.problems.append(f"quest {qid}: record {pid} is not here")
                continue
            self.add_fill(q, item=pid, why=f"{rec.item[pid]['name']}, the guide's {guide} ({why})")
        seen = collections.Counter(f["qid"] for f in self.fills)
        for qid, n in seen.items():
            if n > 1:
                self.problems.append(f"quest {qid} is filled {n} times: merge them first")
        for f in self.fills:
            pids = [o["scroll"] for o in f["choice"]] if f["choice"] else [x for x in (f["scroll"], f["item"]) if x]
            for x in pids:
                if rec.file_name(x) is None:
                    self.problems.append(f"quest {f['qid']}: the quest file cannot name {x} {rec.item[x]['name']!r}")

    def l20_choice(self, q, keep):
        """The quest's last step asks the choice of the two level-20 path scrolls; keep: the path whose
        scroll the step gives today (moved into the choice, the step's item kept)."""
        c = q["c"]
        if q["last"]["kind"] not in fc.NPC_KINDS:
            self.problems.append(f"quest {q['qid']}: a {q['last']['kind']} step asks no choice")
            return
        opts = []
        for k in fc.L20_PATHS[c]:
            if not self.rec.is_own(k, c):
                self.problems.append(f"quest {q['qid']}: {k} is not a spell of the class's")
                return
            good, _ = self.rec.scrolls_for(k, c, 20, q["city"])
            pid = good[0] if good else self.make_scroll(c, k, 20)
            if not pid:
                return
            sid = self.rec.own[c][k][0][0]
            opts.append(dict(spell=k, spell_name=self.rec.spell[sid][0], scroll=pid, scroll_name=self.rec.item[pid]["name"]))
        rewards_after = q["last"]["rewarditems"]
        if keep:
            kept = [o for o in opts if o["spell"] == keep][0]
            rest = [p for p in fc.ids(rewards_after) if p != kept["scroll"]]
            if len(rest) == len(fc.ids(rewards_after)):
                self.problems.append(f"quest {q['qid']}: its reward does not hold the {keep} scroll")
                return
            rewards_after = ", ".join(map(str, rest))
        why = ("the choice of the " + " or the ".join(o["spell_name"] for o in opts) + " scroll, the two level-20 paths "
               "every city's guide offers (their weapons, each city's own, left out)")
        if keep:
            why += f"; the {self.rec.item[[o for o in opts if o['spell'] == keep][0]['scroll']]['name']} scroll it gave is one of them"
        self.add_fill(q, choice=opts, rewards_after=rewards_after, why=why, kept=keep)

    def add_fill(self, q, scroll=None, item=None, choice=None, spell=None, rewards_after=None, why="", kept=None):
        f = dict(qid=q["qid"], key=q["key"], file=q["file"], city=q["city"], c=q["c"], cls=fc.CLASSES[q["c"]],
                 level=q["level"], name=q["name"], last_step=q["last"]["step"], rewards_before=q["last"]["rewarditems"],
                 choices_before=q["last"]["choices"] or "", scroll=scroll, item=item, choice=choice, spell=spell,
                 kept=kept, note=f"The reward is ours: {why} ({STAMP}).")
        if rewards_after is None:
            add = [p for p in (scroll, item) if p]
            rewards_after = ", ".join([f["rewards_before"]] + [str(p) for p in add]) if f["rewards_before"] else \
                ", ".join(str(p) for p in add)
        f["rewards_after"] = rewards_after
        f["choices_after"] = fc.choices_column([dict(spell_name=o["spell_name"], scroll=o["scroll"]) for o in choice]) \
            if choice else f["choices_before"]
        f["owed"] = self.owed(q, f)
        self.fills.append(f)

    def owed(self, q, f):
        """[(serverid, name, [patternid], full)]: the quest's finishers of its class who lack its reward;
        for a choice, those who finished it (theirs to pick: not given)."""
        out = []
        for sid, name, cls in self.db.execute(
                "SELECT c.serverid, h.charName, h.tclass FROM charquests c JOIN characters h ON h.serverid = c.serverid "
                "WHERE c.questid = ? AND c.done = 1 ORDER BY c.serverid", (q["qid"],)):
            if cls != q["c"]:
                continue
            if f["choice"]:
                out.append((sid, name, [], False))
                continue
            mine = []
            for pid in [p for p in (f["scroll"], f["item"]) if p]:
                if self.db.execute("SELECT 1 FROM charinventory WHERE serverid = ? AND patternid = ?", (sid, pid)).fetchone():
                    continue
                if pid == f["scroll"]:
                    book = {fc.spell_key(n) for (n,) in self.db.execute(
                        "SELECT p.spellname FROM spells s JOIN spellpattern p ON p.spellid = s.spellid WHERE s.serverid = ?", (sid,))}
                    if f["spell"] in book:
                        continue
                mine.append(pid)
            if mine:
                held = self.db.execute("SELECT count(*) FROM charinventory WHERE serverid = ? AND location = 0", (sid,)).fetchone()[0]
                out.append((sid, name, mine, held + len(mine) > fc.INV_CAP))
        return out


# --- writing

def log(db, tbl, rid, col, old, new):
    db.execute(f"INSERT INTO {LOG} (tbl, rid, col, old, new) VALUES (?, ?, ?, ?, ?)",
               (tbl, str(rid), col, None if old is None else json.dumps(old), None if new is None else json.dumps(new)))


def apply(db, p):
    db.execute(LOG_DDL)
    n = collections.Counter()
    for m in p.made:
        r = m["row"]
        cols = list(r)
        db.execute(f"INSERT INTO itempattern ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [r[c] for c in cols])
        log(db, "itempattern", r["patternid"], "*", None, r)
        n["scrolls" if r["itemicon"] == fc.SCROLL_ICON and r["equipslot"] == -1 else "items"] += 1
    for f in p.fills:
        cur, cur_choices = db.execute("SELECT rewarditems, choices FROM queststeps WHERE questid = ? AND step = ?",
                                      (f["qid"], f["last_step"])).fetchone()
        if cur != f["rewards_before"] or (cur_choices or "") != f["choices_before"]:
            raise SystemExit(f"quest {f['qid']} step {f['last_step']} changed under the plan")
        if f["rewards_after"] != cur:
            db.execute("UPDATE queststeps SET rewarditems = ? WHERE questid = ? AND step = ?", (f["rewards_after"], f["qid"], f["last_step"]))
            log(db, "queststeps", f"{f['qid']}/{f['last_step']}", "rewarditems", cur, f["rewards_after"])
        if f["choices_after"] != (cur_choices or ""):
            db.execute("UPDATE queststeps SET choices = ? WHERE questid = ? AND step = ?", (f["choices_after"], f["qid"], f["last_step"]))
            log(db, "queststeps", f"{f['qid']}/{f['last_step']}", "choices", cur_choices or "", f["choices_after"])
        n["quests"] += 1
        old_notes = db.execute("SELECT notes FROM quests WHERE questid = ?", (f["qid"],)).fetchone()[0]
        notes = (old_notes + " " if old_notes else "") + f["note"]
        db.execute("UPDATE quests SET notes = ? WHERE questid = ?", (notes, f["qid"]))
        log(db, "quests", f["qid"], "notes", old_notes, notes)
        for sid, _, pids, full in f["owed"]:
            if f["choice"]:
                n["choice"] += 1
                continue
            if full:
                n["full"] += 1
                continue
            for pid in pids:
                cur = db.execute(
                    "INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, "
                    "listnumber) VALUES (?, 1, (SELECT COALESCE(maxhp, 0) FROM itempattern WHERE patternid = ?), 0, ?, -1, 0, "
                    "(SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory WHERE serverid = ? AND location = 0))",
                    (sid, pid, pid, sid))
                log(db, "charinventory", cur.lastrowid, "*", None, {"serverid": sid, "patternid": pid, "questid": f["qid"]})
                n["given"] += 1
    return n


def undo(db):
    if not fc.has_table(db, LOG):
        print("nothing logged: nothing to undo")
        return
    back = kept = 0
    for seq, tbl, rid, col, old, new in db.execute(f"SELECT seq, tbl, rid, col, old, new FROM {LOG} ORDER BY seq DESC").fetchall():
        old = json.loads(old) if old is not None else None
        new = json.loads(new) if new is not None else None
        if tbl == "charinventory":
            cur = db.execute("DELETE FROM charinventory WHERE itemid = ? AND serverid = ? AND patternid = ?",
                             (int(rid), new["serverid"], new["patternid"]))
        elif tbl == "queststeps" and col in ("rewarditems", "choices"):
            q, st = rid.split("/")
            cur = db.execute(f"UPDATE queststeps SET {col} = ? WHERE questid = ? AND step = ? AND {col} = ?", (old, int(q), int(st), new))
        elif tbl == "quests":
            cur = db.execute("UPDATE quests SET notes = ? WHERE questid = ? AND notes = ?", (old, int(rid), new))
        elif tbl == "itempattern":
            now = row(db, "SELECT * FROM itempattern WHERE patternid = ?", (int(rid),))
            held = db.execute("SELECT count(*) FROM charinventory WHERE patternid = ?", (int(rid),)).fetchone()[0]
            if now == new and not held:
                db.execute("DELETE FROM itempattern WHERE patternid = ?", (int(rid),))
                back += 1
            else:
                kept += 1
                print(f"kept item {rid} {new['itemname']!r}: {'held by ' + str(held) + ' records' if held else 'changed since'}")
            continue
        else:
            continue
        back, kept = (back + 1, kept) if cur.rowcount else (back, kept + 1)
    db.execute(f"DROP TABLE {LOG}")
    print(f"put back {back}; left {kept} changed since")


def write_data(datadir, p):
    """Name the same rewards in the quest files: each quest's own text rewritten in their layout."""
    by_file = collections.defaultdict(list)
    for f in p.fills:
        by_file[f["file"]].append(f)
    changed = 0
    for fname, fs in sorted(by_file.items()):
        path = os.path.join(datadir, fname)
        with open(path, encoding="utf-8") as fh:
            text = fh.read()
        for f in fs:
            sp = fc._span(text, f["key"])
            if sp is None:
                raise SystemExit(f"{fname}: no quest {f['key']!r}")
            s, e, ind = sp
            q = json.loads(text[s:e])
            if fc._dump(q, ind) != text[s:e]:
                raise SystemExit(f"{fname}: {f['key']!r} is not laid out as json.dump(indent=1) writes it: left")
            last = q["steps"][-1]
            if f["choice"]:
                if last.get("choices"):
                    raise SystemExit(f"{fname}: {f['key']!r}: its last step asks a choice already")
                names = [p.rec.file_name(o["scroll"]) for o in f["choice"]]
                if None in names:
                    raise SystemExit(f"{fname}: {f['key']!r}: a scroll the file cannot name")
                if f["kept"]:
                    items = (last.get("rewards") or {}).get("items") or []
                    kept_name = [n for o, n in zip(f["choice"], names) if o["spell"] == f["kept"]][0]
                    if kept_name not in items:
                        raise SystemExit(f"{fname}: {f['key']!r}: its reward does not name {kept_name!r}")
                    items.remove(kept_name)
                    if not items:
                        del last["rewards"]["items"]
                last["choose"] = fc.CHOICE_ASK
                last["choices"] = [dict(answer=o["spell_name"], says=fc.CHOICE_SAYS, rewards=dict(items=[n]))
                                   for o, n in zip(f["choice"], names)]
            else:
                items = last.setdefault("rewards", {}).setdefault("items", [])
                for pid in (f["scroll"], f["item"]):
                    if not pid:
                        continue
                    nm = p.rec.file_name(pid)
                    if nm is None:
                        raise SystemExit(f"{fname}: {f['key']!r}: item {pid} the file cannot name")
                    if nm not in items:
                        items.append(nm)
            if f["note"] not in (q.get("notes") or ""):
                q["notes"] = ((q.get("notes") or "") + " " + f["note"]).strip()
            text = text[:s] + fc._dump(q, ind) + text[e:]
            changed += 1
        json.loads(text)
        with open(path, "w", encoding="utf-8") as out:
            out.write(text)
    return changed


# --- the reports

SLOT = {1: "head", 2: "robe", 5: "chest", 6: "forearm", 7: "bracers", 10: "legs", 11: "feet", 12: "one hand", 13: "shield",
        14: "either hand", 15: "two hands", 18: "held", 19: "hands"}
ATTACK = {1: "slash/pierce", 2: "two-handed slash", 3: "blunt", 4: "staff", 5: "pierce", 6: "polearm"}
STATS = [("str", "STR"), ("sta", "STA"), ("agi", "AGI"), ("dex", "DEX"), ("wis", "WIS"), ("intelligence", "INT"),
         ("cha", "CHA"), ("HPMAX", "HP"), ("POWMAX", "POW"), ("AC", "AC"), ("PR", "PR"), ("DR", "DR"), ("FR", "FR"),
         ("CR", "CR"), ("LR", "LR"), ("AR", "AR")]


def describe(db, pid):
    r = row(db, "SELECT * FROM itempattern WHERE patternid = ?", (pid,))
    if r is None:
        return f"{pid} (not here)"
    cls = "/".join(ABBR[c] for c in range(15) if r["classuse"] >> c & 1) if r["classuse"] else "no class mask"
    if r["classuse"] & 0x7FFF == 0x7FFF:
        cls = "any class"
    parts = [f"level {r['levelreq']}", SLOT.get(r["equipslot"], f"slot {r['equipslot']}"), cls]
    if r["weapondamage"]:
        parts.append(f"{r['weapondamage']} damage ({ATTACK.get(r['attacktype'], r['attacktype'])})")
    parts += [f"{lbl} {r[col]}" for col, lbl in STATS if r.get(col)]
    parts.append(f"HP {r['maxhp']}, durability {r['duration']}")
    return f"**{r['itemname']}** ({pid}): " + ", ".join(parts)


def item_review(db, p, path):
    """The ties and misfits, for the user to pick: per quest its candidates and a suggestion."""
    rec = p.rec
    groups = collections.defaultdict(list)
    for q in p.quests.values():
        groups[(q["c"], q["level"])].append(q)
    out = ["# The class quests' item rewards to pick (2026-10-30)", "",
           "The other cities' quests of the same class and level give different items (a tie) or an item the quest's "
           "own story does not fit (a misfit: a robe for a quest titled for a staff, an item made for another city's "
           "quest). Nothing below is applied: name a pick per quest, or per group, and it is filled as the others "
           "were (on the last step; its finishers given it). Each quest's own spell scroll, if any, is already given.",
           ""]
    for qid in REVIEW:
        q = p.quests.get(qid)
        if q is None:
            continue
        sug, why = REVIEW[qid]
        last = row(db, "SELECT kind, target, items, rewarditems, choices FROM queststeps WHERE questid = ? ORDER BY step DESC LIMIT 1", (qid,))
        handed = ", ".join(rec.item[x]["name"] for x in fc.ids(last["items"]) if x in rec.item) or "nothing"
        now = ", ".join(rec.item[x]["name"] for x in fc.ids(last["rewarditems"]) if x in rec.item) or "no item"
        out.append(f"## {fc.CITY.get(q['city'], q['city'])}, {q['cls'] if 'cls' in q else fc.CLASSES[q['c']]} level {q['level']}: "
                   f"{q['name']} (quest {qid})")
        out.append("")
        out.append(f"- The last step: {last['kind']} {last['target']}; handed in: {handed}. It gives now: {now}.")
        cands = collections.OrderedDict()
        for o in groups[(q["c"], q["level"])]:
            for x in o["items"]:
                cands.setdefault(x, []).append(fc.CITY.get(o["city"], o["city"]))
        for x, cities in cands.items():
            out.append(f"- {', '.join(cities)}: {describe(db, x)}")
        out.append(f"- **Suggested:** {describe(db, sug) + '. ' if sug and sug not in cands else ''}{why}.")
        out.append("")
    with open(path, "w", encoding="utf-8") as fh:
        fh.write("\n".join(out))
    return len(REVIEW)


TSV_COLS = ["questid", "key", "city", "class", "level", "quest", "reward", "patternid", "made", "given_to"]


def tsv_rows(p):
    made = {m["row"]["patternid"]: m for m in p.made}
    out = []
    for f in p.fills:
        if f["choice"]:
            reward = "a choice: " + " or ".join(f"scroll {o['scroll_name']}" for o in f["choice"])
            pids = [o["scroll"] for o in f["choice"]]
            if f["kept"]:
                reward += " (one moved from the step's own reward)"
        else:
            pids = [x for x in (f["scroll"], f["item"]) if x]
            reward = "; ".join(("scroll " if x == f["scroll"] else "") + p.rec.item[x]["name"] for x in pids)
        out.append(dict(questid=f["qid"], key=f["key"], city=f["city"], **{"class": f["cls"]}, level=f["level"], quest=f["name"],
                        reward=reward, patternid=", ".join(map(str, pids)),
                        made="; ".join(f"{x}: {made[x]['why']}" for x in pids if x in made),
                        given_to=", ".join(f"{n} ({s}){' (a choice: theirs to pick)' if f['choice'] else ' INVENTORY FULL' if full else ''}"
                                           for s, n, _, full in f["owed"])))
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", default="eqoa.db")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    ap.add_argument("--tsv", help="write the review list here")
    ap.add_argument("--write-data", metavar="DIR", help="name the rewards in the quest files of DIR")
    ap.add_argument("--item-review", metavar="FILE", help="write the item ties and misfits for the user to pick")
    ap.add_argument("--records-only", action="store_true",
                    help="make the scrolls and items alone (a database the quest files are imported into after)")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 10000")
    if a.undo:
        db.execute("BEGIN IMMEDIATE")
        try:
            undo(db)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        return
    if a.apply and fc.has_table(db, LOG) and db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0]:
        sys.exit(f"{LOG} holds an earlier apply: --undo it first")
    p = Plan(db)
    p.build(records_only=a.records_only)
    for m in p.made:
        r = m["row"]
        print(f"make {r['patternid']} {r['itemname']!r} (level {r['levelreq']}, classes {r['classuse']}): {m['why']}")
    for f in p.fills:
        what = ("a choice of " + " or ".join(f"{o['scroll']} {o['scroll_name']!r}" for o in f["choice"])) if f["choice"] else \
            "; ".join(f"{x} {p.rec.item[x]['name']!r}" for x in (f["scroll"], f["item"]) if x)
        given = ", ".join(f"{n} ({s}){' FULL' if full else ''}" for s, n, _, full in f["owed"])
        print(f"{f['cls']} L{f['level']} {f['city']} {f['qid']} {f['name']!r}: {what}"
              + (f"{'' if not f['kept'] else ' (its ' + f['kept'] + ' scroll moved into the choice)'}")
              + (f"; {'owed a pick (not given)' if f['choice'] else 'given to'} {given}" if given else ""))
    for x in p.problems:
        print("PROBLEM:", x)
    print(f"{sum(1 for m in p.made if m['row']['equipslot'] == -1)} scrolls and "
          f"{sum(1 for m in p.made if m['row']['equipslot'] != -1)} items to make; {len(p.fills)} quests to fill; "
          f"{sum(len(f['owed']) for f in p.fills if not f['choice'])} characters owed")
    if a.tsv:
        with open(a.tsv, "w", newline="") as out:
            w = csv.DictWriter(out, fieldnames=TSV_COLS, delimiter="\t", lineterminator="\n")
            w.writeheader()
            w.writerows(tsv_rows(p))
    if a.item_review:
        print(f"item review: {item_review(db, p, a.item_review)} quests")
    if a.write_data:
        print(f"named in the quest files: {write_data(a.write_data, p)}")
    if not a.apply:
        print("report only: --apply to write")
        return
    if p.problems:
        sys.exit("refused: the problems above first")
    db.execute("BEGIN IMMEDIATE")
    try:
        n = apply(db, p)
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"made {n['scrolls']} scrolls and {n['items']} items, filled {n['quests']} quests, {n['given']} given "
          f"({n['full']} not: a full inventory; {n['choice']} owed a level-20 pick); restart the server (or save a quest "
          "in the GM panel) to load the quests")


if __name__ == "__main__":
    main()
