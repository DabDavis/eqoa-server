#!/usr/bin/env python3
"""The races' homelands' starter quests, on the starting cities' template.

The owner, 2026-11-10: "the first quests should be a piece of equipment(not wep), 2nd quest spiritmaster and coachman.
3rd kill a mob, etc. follow the same template as the other main race quests." Every starting city's class chain opens
the same way (tools/quests/data/<city>.json, from EQOA-Data's guides): level 1 a piece of equipment fetched from a
merchant and shown, level 2 the spiritmaster and the coachman, level 3 a hunt for drops that teaches the class its
first quest ability, level 4 new gear made of a merchant's materials and a drop, level 5 a named foe (or a tougher
kind) for the second ability. A homeland's chain is that, once for every class, then its gatekeeper's flight.

This writes tools/quests/data/homelands.json (a quest file as tools/quests/FORMAT.md has them, for
tools/import_quests.py) from:

- tools/quests/homelands/<race>.json, ONE SPEC A RACE: the names and the words, nothing the template fixes
  (tools/quests/homelands/README.md has every field);
- tools/quests/homelands/class-rewards.json: what levels 3, 4 and 5 give each class, the same in every home;
- tools/quests/homelands/legacy.json: a race with no spec yet keeps the three quests it had, word for word (a race
  with a spec has no use for its entry there: the spec is taken, so adding a home touches no shared file).

What the template fixes, for every home alike: the keys (homelands/<race>/L1 .. L5, flight), whose they are (the
home's own race alone, by its racedefs id: a coach brings anyone into any home, and a home's chain is its people's),
the levels (1-5), each quest after the one before, the experience (430, 2,200, 6,900, 17,000, 36,500: the cities'),
the steps' kinds, the drop chances, the rewards by class, the flight's teleport to the mainland and its bind, and on
each of L1-L5 the event that finishes the character's own city class quest of that level too (classquest:
handlers/questevents.go), so its guildmaster on the mainland goes on at level 7.

    tools/quests/gen_homelands.py             write tools/quests/data/homelands.json
    tools/quests/gen_homelands.py --check     exit 1 when the file is not what the specs give (nothing is written)
    tools/quests/gen_homelands.py --list      each race: its spec, or its legacy three
    tools/quests/gen_homelands.py --stdout    print it

The file is generated: change a spec and run this, never the file (tools/quests/fix_names.py and the GM panel's
exports would be overwritten by the next run). tools/quests/gen_homelands_test.py holds it to the specs.
"""
import argparse
import copy
import json
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
SPECS = os.path.join(HERE, "homelands")
OUT = os.path.join(HERE, "data", "homelands.json")
CITY = "homelands"

# The homes, in the file's order (the order the first three quests of each were written in).
RACES = ["hillgiant", "kerran", "kappa", "kobold", "ratman", "satyr", "dryad", "gargoyle", "jalraeth", "aviak", "goblin",
         "lizardman", "brownie", "pixie", "fairy", "gnoll", "orc", "minotaur", "skeleton", "fungusman", "wisp"]

# Each home's race (racedefs.id): the zone of tools/worlds/homelands.json that is its home, and whose alone its
# quests are (a quest's races of 11 and up name a race of racedefs by its own id: handlers/quests.go).
RACE_IDS = {"hillgiant": 11, "kerran": 12, "goblin": 13, "aviak": 14, "lizardman": 15, "kappa": 16, "brownie": 17, "pixie": 18,
            "fairy": 19, "gnoll": 20, "orc": 21, "kobold": 22, "minotaur": 23, "ratman": 24, "satyr": 25, "dryad": 26,
            "skeleton": 27, "fungusman": 28, "gargoyle": 29, "jalraeth": 30, "wisp": 31}

# The cities' figures (EQOA-Data's guides; Sandstorm gave 430 and 2,200 for the first two, questrec16; levels 7-20 the
# figure 74 of the 75 city class quests of the level give on the Heirloom server).
XP = {1: 430, 2: 2200, 3: 6900, 4: 17000, 5: 36500, 7: 157474, 10: 556753, 13: 550698, 15: 883791, 20: 2814929}
LEVELS = (1, 2, 3, 4, 5)
# the chain past the gatekeeper's flight (the owner, 2026-11-11: "add up to lvl 20 path quests for them all"): the cities'
# levels, each optional in a spec but each after the one before
HIGHER = (7, 10, 13, 15, 20)
# CLIENT's class ids (root CLAUDE.md), as the quests' keys of level 20 name them: one quest a class, each asking that
# class's own path pick
CLASS_NAMES = ["warrior", "ranger", "paladin", "shadowknight", "monk", "bard", "rogue", "druid", "shaman", "cleric",
               "magician", "necromancer", "enchanter", "wizard", "alchemist"]
# the cities' level 20 question, and an option's words when it is picked
PATH_ASK, PATH_PICKED = "Which do you take?", "Then it is yours."
# a hunt's drop: one kill in two (the cities' files' own figure); a named foe always drops
CHANCE, NAMED_CHANCE = 0.5, 1.0
# CLIENT's room (handlers/quests.go): a box 217 characters with the speaker's name and ": " before the line, a question
# 127, an answer 63, a journal line 127
BOX, ASK, ANSWER, LINE = 217, 127, 63, 127
# what a first gift's set may give and how much (the cities' level 1 gifts: 3-5 armour, or 5 hit points or power)
PIECE_FIGURES = {"AC": 5, "HPMAX": 5, "POWMAX": 5}
# every class, every race of the game's (a server's own race is asked about as its wire race), level 1, free
PIECE_FIXED = {"classuse": 32767, "raceuse": 1023, "levelreq": 1, "itemcost": 0, "maxhp": 500}

WHAT = {
    1: "the cities' level 1 quest: a piece of equipment fetched from a merchant and shown to the giver",
    2: "the cities' level 2 quest: the spiritmaster's bind and the coachman's ledger",
    3: "the cities' level 3 quest: a hunt for drops; each class is taught what its own city's level 3 quest teaches it",
    4: "the cities' level 4 quest: new gear of a merchant's materials and a drop; each class is given what its own "
       "city's level 4 quest gives it (class-rewards.json has the two classes the cities give nothing)",
    5: "the cities' level 5 quest: a foe to bring down; each class is taught what its own city's level 5 quest teaches it",
    7: "the cities' level 7 quest: two tasks of the hunt, their spoils handed in; each class is taught what its own city's "
       "level 7 quest teaches it",
    10: "the cities' level 10 quest: a new weapon, of what a camp hands over and a creature's drop; each class is given what "
        "its own city's level 10 quest gives it (class-rewards.json has the four classes the cities give nothing)",
    13: "the cities' level 13 quest: a named foe brought down; each class is taught what its own city's level 13 quest "
        "teaches it",
    15: "the cities' level 15 quest: a named foe's spoil carried where it is wanted; each class is taught what its own "
        "city's level 15 quest teaches it",
    20: "the cities' level 20 quest: a road of errands ending at the giver, who asks which path the character takes, the "
        "options its class's city asks (\"Which do you take?\")",
}


class SpecError(Exception):
    pass


def prev_level(level):
    """The level of the chain's quest before level's."""
    chain = LEVELS + HIGHER
    return chain[chain.index(level) - 1]


def lines_of(v):
    """A text of a spec: a string (its lines) or a list of lines, empty ones left out."""
    if v is None:
        return []
    if isinstance(v, str):
        v = v.split("\n")
    if not isinstance(v, list) or not all(isinstance(x, str) for x in v):
        raise SpecError(f"{v!r} is not a text (a string, or a list of lines)")
    return [x.strip() for x in v if x.strip()]


class Race:
    """One race's spec, read and checked into quests."""

    def __init__(self, race, spec, rewards):
        self.race, self.spec, self.rewards = race, spec, rewards
        self.items = []   # the quest file's Items this home makes, in order
        self.problems = []

    def bad(self, where, msg):
        self.problems.append(f"{self.race} {where}: {msg}")

    # --- the words

    def need(self, where, d, key, kind=str):
        v = d.get(key) if isinstance(d, dict) else None
        if not isinstance(v, kind) or (kind is str and not v.strip()):
            self.bad(where, f"no {key}")
            return kind()
        return v.strip() if kind is str else v

    def words(self, where, d, key):
        """A text a spec must have (a string, or a list of lines), as given."""
        v = d.get(key) if isinstance(d, dict) else None
        if not v or not isinstance(v, (str, list)):
            self.bad(where, f"no {key}")
            return ""
        return v

    def said(self, where, speaker, v):
        """An NPC's words: a box a line, the speaker's name before each."""
        try:
            ls = lines_of(v)
        except SpecError as e:
            self.bad(where, str(e))
            return ""
        for l in ls:
            n = len(l) + (0 if l.lower().startswith(speaker.lower() + ":") else len(speaker) + 2)
            if n > BOX:
                self.bad(where, f"a line is {n} characters with {speaker!r} before it (a box shows {BOX}): {l[:50]!r}...")
        return "\n".join(ls)

    def offer(self, where, speaker, v):
        """A giver's offer: its lines a box each, the last one the question the answers answer."""
        text = self.said(where, speaker, v)
        ls = text.split("\n") if text else []
        if len(ls) < 2:
            self.bad(where, "an offer is at least two lines: what the giver says, then its question")
        elif len(ls[-1]) > ASK:
            self.bad(where, f"the question (the offer's last line) is {len(ls[-1])} characters (a question shows {ASK})")
        elif not ls[-1].endswith("?"):
            self.bad(where, f"the offer's last line is the question the answers answer: {ls[-1]!r} asks nothing")
        return text

    def answer(self, where, d, key):
        v = self.need(where, d, key)
        if len(v) > ANSWER:
            self.bad(where, f"{key} is {len(v)} characters (an answer shows {ANSWER})")
        return v

    def journal(self, where, d):
        v = self.need(where, d, "journal")
        if len(v) > LINE:
            self.bad(where, f"the journal line is {len(v)} characters (the journal shows {LINE})")
        return v

    # --- the items

    def make(self, where, it, piece=False):
        """An item the home's quests make (FORMAT.md, Item); returns its name."""
        name = self.need(where, it, "name")
        like = self.need(where, it, "like")
        out = {"name": name, "description": (it.get("description") or "").strip() if isinstance(it, dict) else "",
               "notrade": True, "like": like}
        if not out["description"]:
            self.bad(where, f"{name!r} has no description")
        sets = it.get("set") if isinstance(it, dict) else None
        if piece:
            figures = sets if isinstance(sets, dict) else {}
            wrong = [k for k, v in figures.items() if k not in PIECE_FIGURES or not isinstance(v, int) or isinstance(v, bool)
                     or not 0 < v <= PIECE_FIGURES[k]]
            if wrong or not figures:
                self.bad(where, f"the piece's set is its figures, as the cities' level 1 gifts': "
                                f"{', '.join(f'{k} up to {v}' for k, v in PIECE_FIGURES.items())} (it gives {sets!r})")
            out["set"] = {**{k: figures[k] for k in sorted(figures)}, **PIECE_FIXED}
        elif sets is not None:
            self.bad(where, f"{name!r}: only the level 1 piece has a set")
        for have in self.items:
            if have["name"].lower() == name.lower():
                if have != out:
                    self.bad(where, f"{name!r} is made twice, differently")
                return name
        self.items.append(out)
        return name

    def item(self, where, it):
        """A material: an existing record's name, or an item to make."""
        if isinstance(it, str) and it.strip():
            return it.strip()
        if isinstance(it, dict):
            return self.make(where, it)
        self.bad(where, f"{it!r} is no item (a record's name, or one to make: name, description, like)")
        return ""

    # --- the quests

    def base(self, level, q, events=True):
        where = f"L{level}"
        giver = self.need(where, q, "giver")
        out = {
            "key": f"homelands/{self.race}/L{level}",
            "name": self.need(where, q, "name"),
            "giver": giver,
            "classes": [], "races": [RACE_IDS[self.race]],
            "minlevel": level, "maxlevel": 60,
            "requires": [f"homelands/{self.race}/L{prev_level(level)}"] if level > 1 else [],
            "offer": self.offer(f"{where} offer", giver, q.get("offer") if isinstance(q, dict) else None),
            "accept": self.answer(where, q, "accept"),
            "decline": self.answer(where, q, "decline"),
            "declined": self.said(f"{where} declined", giver, self.words(where, q, "declined")),
            "started": self.said(f"{where} started", giver, self.words(where, q, "started")),
            "steps": [],
            "events": [{"on": "finish", "do": "classquest", "level": level}],
            "unresolved": [],
            "notes": f"Ours (the Homelands' starter quests): the {self.race}'s level {level}, at {self.spec.get('home', 'its home')}, "
                     f"on {WHAT[level]}. Finishing it finishes the character's own city class quest of level {level} too "
                     f"(the classquest event), so its guildmaster on the mainland goes on from where the homeland left off. "
                     f"Written by tools/quests/gen_homelands.py from tools/quests/homelands/{self.race}.json: change that "
                     f"file, never this quest's rows by hand.",
        }
        return out, giver

    def step(self, where, kind, target, d, **more):
        """A step's dict: the journal line, the target's words when it is done (a talk, show or give step's)."""
        st = {"kind": kind, "target": target}
        st.update(more)
        st["journal"] = self.journal(where, d)
        if kind in ("talk", "show", "give"):
            st["says"] = self.said(f"{where} says", target, self.words(where, d, "says"))
            for opt in ("waiting", "notyet", "reminder"):
                if isinstance(d, dict) and d.get(opt):
                    st[opt] = self.said(f"{where} {opt}", target, d[opt])
        return st

    def reward(self, level):
        rw = {"xp": XP[level]}
        if level >= 3 and level != 20:
            by = {c: copy.deepcopy(self.rewards[c][f"L{level}"]) for c in sorted(self.rewards, key=int)}
            # a class its cities give only experience at the level has no entry
            by = {c: v for c, v in by.items() if v}
            if by:
                rw["byclass"] = by
        return rw

    def l1(self, q):
        out, giver = self.base(1, q)
        merchant = self.need("L1", q, "merchant")
        piece = self.make("L1 piece", q.get("piece") or {}, piece=True)
        fetch = self.step("L1 fetch", "talk", merchant, q.get("fetch") or {})
        fetch["events"] = [{"on": "done", "do": "give", "items": [{"item": piece, "count": 1}]}]
        show = self.step("L1 show", "show", giver, q.get("show") or {}, items=[{"item": piece, "count": 1}])
        show["rewards"] = self.reward(1)
        out["steps"] = [fetch, show]
        return out

    def l2(self, q):
        out, giver = self.base(2, q)
        sm, coach = self.need("L2", q, "spiritmaster"), self.need("L2", q, "coachman")
        if sm and not sm.startswith("Spiritmaster "):
            self.bad("L2", f"{sm!r} is no spiritmaster (the server binds at a \"Spiritmaster ...\")")
        if coach and not coach.startswith(("Coachman ", "Coachwoman ")):
            self.bad("L2", f"{coach!r} is no coachman (the server knows a \"Coachman ...\" or a \"Coachwoman ...\")")
        back = self.step("L2 back", "talk", giver, q.get("back") or {})
        back["rewards"] = self.reward(2)
        out["steps"] = [self.step("L2 bind", "talk", sm, q.get("bind") or {}),
                        self.step("L2 ledger", "talk", coach, q.get("ledger") or {}), back]
        return out

    def hunt(self, level, q, lo, hi, chance, where=None):
        """A loot step: (the step, its drop's name, how many)."""
        where = where or f"L{level}"
        target = self.need(where, q, "target")
        count = q.get("count") if isinstance(q, dict) else None
        if not isinstance(count, int) or isinstance(count, bool) or not lo <= count <= hi:
            self.bad(where, f"count {count!r} (the template takes {lo} to {hi})")
            count = lo
        drop = self.make(f"{where} drop", q.get("drop") or {})
        st = self.step(f"{where} hunt", "loot", target, q.get("hunt") or {},
                       items=[{"item": drop, "count": count}], count=count, chance=chance)
        return st, drop, count

    def l3(self, q):
        out, giver = self.base(3, q)
        hunt, drop, n = self.hunt(3, q, 2, 3, CHANCE)
        give = self.step("L3 give", "give", giver, q.get("give") or {}, items=[{"item": drop, "count": n}])
        give["rewards"] = self.reward(3)
        out["steps"] = [hunt, give]
        return out

    def l4(self, q):
        out, giver = self.base(4, q)
        mats = q.get("materials") if isinstance(q, dict) else None
        if not isinstance(mats, list) or not 1 <= len(mats) <= 2:
            self.bad("L4", "materials: one or two merchants' hand-overs, as the cities' level 4 quests have")
            mats = []
        steps, handed = [], []
        for i, m in enumerate(mats, 1):
            where = f"L4 materials {i}"
            merchant = self.need(where, m, "merchant")
            its = [self.item(where, it) for it in (m.get("items") if isinstance(m, dict) and isinstance(m.get("items"), list) else [])]
            if not 1 <= len(its) <= 2:
                self.bad(where, "one or two items a merchant")
            st = self.step(where, "talk", merchant, m)
            st["events"] = [{"on": "done", "do": "give", "items": [{"item": it, "count": 1} for it in its]}]
            steps.append(st)
            handed += its
        hunt, drop, n = self.hunt(4, q, 1, 2, CHANCE)
        give = self.step("L4 give", "give", giver, q.get("give") or {},
                         items=[{"item": it, "count": 1} for it in handed] + [{"item": drop, "count": n}])
        give["rewards"] = self.reward(4)
        out["steps"] = steps + [hunt, give]
        return out

    def l5(self, q):
        out, giver = self.base(5, q)
        kind = q.get("kind", "loot") if isinstance(q, dict) else "loot"
        if kind == "loot":
            chance = q.get("chance", NAMED_CHANCE)
            if not isinstance(chance, (int, float)) or isinstance(chance, bool) or not 0 < chance <= 1:
                self.bad("L5", f"chance {chance!r}")
                chance = NAMED_CHANCE
            hunt, drop, n = self.hunt(5, q, 1, 3, float(chance))
            last = self.step("L5 give", "give", giver, q.get("give") or {}, items=[{"item": drop, "count": n}])
            out["steps"] = [hunt, last]
        elif kind == "kill":
            target = self.need("L5", q, "target")
            count = q.get("count")
            if not isinstance(count, int) or isinstance(count, bool) or not 1 <= count <= 5:
                self.bad("L5", f"count {count!r} (one named foe, or up to five of a tougher kind)")
                count = 3
            hunt = self.step("L5 hunt", "kill", target, q.get("hunt") or {}, count=count)
            last = self.step("L5 back", "talk", giver, q.get("back") or {})
            out["steps"] = [hunt, last]
        else:
            self.bad("L5", f"kind {kind!r} (loot: a foe's drop brought back; kill: a count of a kind)")
            return out
        last["rewards"] = self.reward(5)
        return out

    # --- past the flight: levels 7-20, on the cities' shapes (their objectives in the Homelands' shared ground)

    def talks(self, where, q, key, lo, hi):
        """A list of talk steps (a road's stops): each {target, journal, says}."""
        stops = q.get(key) if isinstance(q, dict) else None
        if stops is None and lo == 0:
            return []
        if not isinstance(stops, list) or not lo <= len(stops) <= hi:
            self.bad(where, f"{key}: {lo} to {hi} stops, each {{target, journal, says}}")
            return []
        return [self.step(f"{where} {key} {i}", "talk", self.need(f"{where} {key} {i}", st, "target"), st)
                for i, st in enumerate(stops, 1)]

    def l7(self, q):
        """Two tasks of the hunt (Halas's Three Tasks): one or two kinds' drops, handed in together."""
        out, giver = self.base(7, q)
        hunts = q.get("hunts")
        if not isinstance(hunts, list) or not 1 <= len(hunts) <= 2:
            self.bad("L7", "hunts: one or two {target, count, drop, hunt}, as the cities' level 7 tasks")
            hunts = []
        steps, handed = [], []
        for i, h in enumerate(hunts, 1):
            st, drop, n = self.hunt(7, h if isinstance(h, dict) else {}, 1, 3, CHANCE, where=f"L7 hunt {i}")
            steps.append(st)
            handed.append({"item": drop, "count": n})
        give = self.step("L7 give", "give", giver, q.get("give") or {}, items=handed)
        give["rewards"] = self.reward(7)
        out["steps"] = steps + [give]
        return out

    def l10(self, q):
        """A new weapon (the cities' level 10): what a camp or a merchant hands over, and a creature's drop."""
        return self.gear(10, q, 0, 2)

    def l13(self, q):
        """A named foe brought down (Gunkat, Battle Plans): a kill, or a foe's drop brought back."""
        return self.foe(13, q, kill=(1, 8), loot=(1, 3))

    def l15(self, q):
        """A named foe's spoil (Trukat, Numla, Kurnor): taken off it (it always drops), carried to whoever wants it when
        the spec says (carry), then back to the giver."""
        out, giver = self.base(15, q)
        chance = q.get("chance", NAMED_CHANCE)
        if not isinstance(chance, (int, float)) or isinstance(chance, bool) or not 0 < chance <= 1:
            self.bad("L15", f"chance {chance!r}")
            chance = NAMED_CHANCE
        hunt, drop, n = self.hunt(15, q, 1, 3, float(chance))
        carry = q.get("carry")
        if carry is None:
            last = self.step("L15 give", "give", giver, q.get("give") or {}, items=[{"item": drop, "count": n}])
            out["steps"] = [hunt, last]
        else:
            to = self.need("L15 carry", carry, "target")
            st = self.step("L15 carry", "give", to, carry, items=[{"item": drop, "count": n}])
            last = self.step("L15 back", "talk", giver, q.get("back") or {})
            out["steps"] = [hunt, st, last]
        last["rewards"] = self.reward(15)
        return out

    def l20(self, q):
        """The road (Halas's River Road): stops along the way, a hunt whose drop someone on it wants, more stops, and the
        last word at the giver (final), who asks which path the character takes: one quest a class, each its city's pick.
        """
        out, giver = self.base(20, q)
        road = self.talks("L20", q, "road", 1, 3)
        hunt, drop, n = self.hunt(20, q, 1, 3, CHANCE)
        hd = q.get("handin") if isinstance(q.get("handin"), dict) else {}
        to = hd.get("target") or (road[-1]["target"] if road else giver)
        handin = self.step("L20 handin", "give", to, hd, items=[{"item": drop, "count": n}])
        after = self.talks("L20", q, "after", 0, 2)
        fin = q.get("final") if isinstance(q.get("final"), dict) else {}
        final = self.step("L20 final", "talk", fin.get("target") or giver, fin)
        picked = self.said("L20 picked", final["target"], q.get("picked") or PATH_PICKED)
        steps = road + [hunt, handin] + after + [final]
        outs = []
        for c in sorted(self.rewards, key=int):
            opts = self.rewards[c]["L20"]
            qc = copy.deepcopy(out)
            qc["key"] = f"homelands/{self.race}/L20/{CLASS_NAMES[int(c)]}"
            qc["classes"] = [int(c)]
            st = copy.deepcopy(steps)
            st[-1]["rewards"] = self.reward(20)
            st[-1]["choose"] = PATH_ASK
            st[-1]["choices"] = [{"answer": o["answer"], "says": picked, "rewards": {k: list(o[k]) for k in ("items", "spells") if o.get(k)}}
                                 for o in opts]
            qc["steps"] = st
            qc["notes"] = qc["notes"].replace("level 20,", f"level 20 for the {CLASS_NAMES[int(c)]},", 1)
            outs.append(qc)
        return outs

    def gear(self, level, q, lo, hi):
        """Materials one to two merchants hand over (lo may be 0) and a creature's drop, handed in for the class's gear."""
        where = f"L{level}"
        out, giver = self.base(level, q)
        mats = q.get("materials", [] if lo == 0 else None) if isinstance(q, dict) else None
        if not isinstance(mats, list) or not lo <= len(mats) <= hi:
            self.bad(where, f"materials: {lo} to {hi} hand-overs ({{merchant, items, journal, says}})")
            mats = []
        steps, handed = [], []
        for i, m in enumerate(mats, 1):
            w = f"{where} materials {i}"
            merchant = self.need(w, m, "merchant")
            its = [self.item(w, it) for it in (m.get("items") if isinstance(m, dict) and isinstance(m.get("items"), list) else [])]
            if not 1 <= len(its) <= 2:
                self.bad(w, "one or two items a merchant")
            st = self.step(w, "talk", merchant, m)
            st["events"] = [{"on": "done", "do": "give", "items": [{"item": it, "count": 1} for it in its]}]
            steps.append(st)
            handed += its
        hunt, drop, n = self.hunt(level, q, 1, 3 if level > 4 else 2, CHANCE)
        give = self.step(f"{where} give", "give", giver, q.get("give") or {},
                         items=[{"item": it, "count": 1} for it in handed] + [{"item": drop, "count": n}])
        give["rewards"] = self.reward(level)
        out["steps"] = steps + [hunt, give]
        return out

    def foe(self, level, q, kill, loot):
        """A foe to bring down: kind "loot" (its drop brought back; a named foe always drops) or "kill" (a count, then
        back to the giver)."""
        where = f"L{level}"
        out, giver = self.base(level, q)
        kind = q.get("kind", "loot" if level == 5 else "kill") if isinstance(q, dict) else "loot"
        if kind == "loot":
            chance = q.get("chance", NAMED_CHANCE)
            if not isinstance(chance, (int, float)) or isinstance(chance, bool) or not 0 < chance <= 1:
                self.bad(where, f"chance {chance!r}")
                chance = NAMED_CHANCE
            hunt, drop, n = self.hunt(level, q, loot[0], loot[1], float(chance))
            last = self.step(f"{where} give", "give", giver, q.get("give") or {}, items=[{"item": drop, "count": n}])
            out["steps"] = [hunt, last]
        elif kind == "kill":
            target = self.need(where, q, "target")
            count = q.get("count")
            if not isinstance(count, int) or isinstance(count, bool) or not kill[0] <= count <= kill[1]:
                self.bad(where, f"count {count!r} (one named foe, or up to {kill[1]} of a tougher kind)")
                count = kill[0]
            hunt = self.step(f"{where} hunt", "kill", target, q.get("hunt") or {}, count=count)
            last = self.step(f"{where} back", "talk", giver, q.get("back") or {})
            out["steps"] = [hunt, last]
        else:
            self.bad(where, f"kind {kind!r} (loot: a foe's drop brought back; kill: a count of a kind)")
            return out
        last["rewards"] = self.reward(level)
        return out

    def flight(self, q):
        where = "flight"
        giver = self.need(where, q, "giver")
        if giver and not giver.startswith(("Gatekeeper ", "Harbormaster ")):
            self.bad(where, f"{giver!r} is no gatekeeper")
        return {
            "key": f"homelands/{self.race}/flight",
            "name": self.need(where, q, "name"),
            "giver": giver,
            "classes": [], "races": [RACE_IDS[self.race]],
            "minlevel": 1, "maxlevel": 60,
            "requires": [f"homelands/{self.race}/L{k}" for k in LEVELS],
            "repeatable": True,
            "offer": self.offer(f"{where} offer", giver, q.get("offer") if isinstance(q, dict) else None),
            "accept": self.answer(where, q, "accept"),
            "decline": self.answer(where, q, "decline"),
            "declined": self.said(f"{where} declined", giver, self.words(where, q, "declined")),
            "started": self.said(f"{where} started", giver, self.words(where, q, "started")),
            "steps": [{"kind": "talk", "target": giver, "journal": self.journal(where, q),
                       "events": [{"on": "start", "do": "dostep"},
                                  {"on": "done", "do": "teleport", "bind": True, "to": "mainland"}]}],
            "unresolved": [],
            "notes": f"Ours: the gatekeeper's port to {self.spec.get('city', 'the mainland')}, the {self.race}'s wire race's city, each "
                     "class to its own guild's spot there (teleport to mainland: the wire race's start row for the class), bound "
                     "there: the passage the five starter quests earn (a coach runs between the home and Freeport for anyone). "
                     "Repeatable, so one back in the homeland is sent again; no reward, so none is had twice. Written by "
                     f"tools/quests/gen_homelands.py from tools/quests/homelands/{self.race}.json.",
        }

    def quests(self):
        s = self.spec
        if s.get("race") != self.race:
            self.bad("race", f"the file is {self.race}.json and says race {s.get('race')!r}")
        for k in ("home", "city"):
            self.need("spec", s, k)
        known = {"race", "home", "city", "flight", "note"} | {f"L{k}" for k in LEVELS + HIGHER}
        unknown = set(s) - known
        if unknown:
            self.bad("spec", f"has {sorted(unknown)} (race, home, city, L1-L5, flight, L7, L10, L13, L15, L20 and note are read)")
        build = {1: self.l1, 2: self.l2, 3: self.l3, 4: self.l4, 5: self.l5}
        out = []
        for k in LEVELS:
            q = s.get(f"L{k}")
            if not isinstance(q, dict):
                self.bad(f"L{k}", "missing")
                continue
            out.append(build[k](q))
        if not isinstance(s.get("flight"), dict):
            self.bad("flight", "missing")
        else:
            out.append(self.flight(s["flight"]))
        # past the flight: each level optional, but none without the one before it (a content lane adds them in order)
        higher = {7: self.l7, 10: self.l10, 13: self.l13, 15: self.l15, 20: self.l20}
        for i, k in enumerate(HIGHER):
            q = s.get(f"L{k}")
            if q is None:
                for later in HIGHER[i + 1:]:
                    if s.get(f"L{later}") is not None:
                        self.bad(f"L{later}", f"comes after L{k}, which the spec lacks")
                break
            if not isinstance(q, dict):
                self.bad(f"L{k}", "is not a quest")
                break
            got = higher[k](q)
            out += got if isinstance(got, list) else [got]
        return out


def load_rewards(path=None):
    """class-rewards.json: {class id: {"L3": {...}, "L4": {...}, "L5": {...}, "L7" .. "L15": {...}, "L20": [options]}},
    every class."""
    doc = json.load(open(path or os.path.join(SPECS, "class-rewards.json")))
    out = {}
    for c in map(str, range(15)):
        ent = doc.get("classes", {}).get(c)
        if not isinstance(ent, dict):
            raise SpecError(f"class-rewards.json has no class {c}")
        out[c] = {}
        for lv in ("L3", "L4", "L5"):
            rw = ent.get(lv)
            if not isinstance(rw, dict) or set(rw) - {"items", "spells"} or not (rw.get("items") or rw.get("spells")) or \
                    not all(isinstance(n, str) and n.strip() for k in rw for n in rw[k]):
                raise SpecError(f"class-rewards.json class {c} {lv}: {rw!r} is not {{items: [names]}} or {{spells: [names]}}")
            out[c][lv] = {k: list(rw[k]) for k in ("items", "spells") if rw.get(k)}
        for lv in ("L7", "L10", "L13", "L15"):
            # levels 7-20 (the homes' chain past the flight): a class its cities give only experience at a level has {}
            rw = ent.get(lv)
            if not isinstance(rw, dict) or set(rw) - {"items", "spells"} or \
                    not all(isinstance(rw[k], list) and all(isinstance(n, str) and n.strip() for n in rw[k]) for k in rw):
                raise SpecError(f"class-rewards.json class {c} {lv}: {rw!r} is not {{items: [names], spells: [names]}} (or {{}})")
            out[c][lv] = {k: list(rw[k]) for k in ("items", "spells") if rw.get(k)}
        opts = (ent.get("L20") or {}).get("choices") if isinstance(ent.get("L20"), dict) else None
        if not isinstance(opts, list) or not 2 <= len(opts) <= 6 or not all(
                isinstance(o, dict) and isinstance(o.get("answer"), str) and 0 < len(o["answer"].strip()) <= ANSWER
                and not set(o) - {"answer", "items", "spells"} and (o.get("items") or o.get("spells"))
                and all(isinstance(n, str) and n.strip() for k in ("items", "spells") for n in o.get(k, []))
                for o in opts):
            raise SpecError(f"class-rewards.json class {c} L20: the path pick is {{choices: [{{answer, items or spells}}, ...]}}, "
                            f"2 to 6 options (the client shows six), each answer {ANSWER} characters at most")
        out[c]["L20"] = [{"answer": o["answer"].strip(), **{k: list(o[k]) for k in ("items", "spells") if o.get(k)}} for o in opts]
    return out


def load_legacy(path=None):
    p = path or os.path.join(SPECS, "legacy.json")
    if not os.path.exists(p):
        return {}
    return json.load(open(p)).get("races", {})


def build(specs=None):
    """The quest file: (document, {race: "spec" or "legacy"}, problems). specs: the directory (the tree's by default)."""
    specs = specs or SPECS
    rewards = load_rewards(os.path.join(specs, "class-rewards.json"))
    legacy = load_legacy(os.path.join(specs, "legacy.json"))
    quests, items, how, problems = [], [], {}, []
    made = {}
    for race in RACES:
        path = os.path.join(specs, race + ".json")
        if os.path.exists(path):
            try:
                spec = json.load(open(path))
            except ValueError as e:
                problems.append(f"{race}.json: {e}")
                continue
            r = Race(race, spec, rewards)
            quests += r.quests()
            problems += r.problems
            for it in r.items:
                k = it["name"].lower()
                if k in made and made[k] != it:
                    problems.append(f"{race}: the item {it['name']!r} is another home's too, made differently")
                elif k not in made:
                    made[k] = it
                    items.append(it)
            how[race] = "spec"
        elif race in legacy:
            quests += copy.deepcopy(legacy[race])
            how[race] = "legacy"
        else:
            problems.append(f"{race}: no spec ({race}.json) and no legacy quests")
    for f in sorted(os.listdir(specs)):
        stem, ext = os.path.splitext(f)
        if ext == ".json" and stem not in RACES and stem not in ("legacy", "class-rewards"):
            problems.append(f"{f}: no home is named {stem!r} (the homes: {', '.join(RACES)})")
    keys = [q["key"] for q in quests]
    for k in sorted({k for k in keys if keys.count(k) > 1}):
        problems.append(f"the key {k!r} is used twice")
    return {"city": CITY, "quests": quests, "items": items}, how, problems


def dumps(doc):
    return json.dumps(doc, indent=2, ensure_ascii=False) + "\n"


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--specs", default=SPECS, help="the specs' directory (tools/quests/homelands)")
    ap.add_argument("--out", default=OUT, help="the quest file written (tools/quests/data/homelands.json)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--check", action="store_true", help="exit 1 when the file is not what the specs give")
    g.add_argument("--list", action="store_true", help="each race: its spec, or its legacy three")
    g.add_argument("--stdout", action="store_true", help="print the file")
    a = ap.parse_args(argv)
    try:
        doc, how, problems = build(a.specs)
    except (SpecError, OSError, ValueError) as e:
        print(f"ERROR: {e}")
        return 1
    for p in problems:
        print("ERROR:", p)
    if problems:
        print(f"{len(problems)} errors: nothing written")
        return 1
    text = dumps(doc)
    if a.list:
        for race in RACES:
            n = sum(1 for q in doc["quests"] if q["key"].split("/")[1] == race)
            print(f"{race:10} {how[race]:6} {n} quests")
        print(f"{sum(1 for h in how.values() if h == 'spec')} of {len(RACES)} homes on the cities' template; "
              f"{len(doc['quests'])} quests, {len(doc['items'])} items made")
        return 0
    if a.stdout:
        sys.stdout.write(text)
        return 0
    have = open(a.out).read() if os.path.exists(a.out) else None
    if a.check:
        if have != text:
            print(f"{a.out} is not what the specs give: run tools/quests/gen_homelands.py")
            return 1
        print(f"{a.out}: as the specs give it ({len(doc['quests'])} quests)")
        return 0
    if have == text:
        print(f"{a.out}: unchanged ({len(doc['quests'])} quests)")
        return 0
    with open(a.out, "w") as f:
        f.write(text)
    print(f"{a.out}: written, {len(doc['quests'])} quests ({sum(1 for h in how.values() if h == 'spec')} homes on the "
          f"template, {sum(1 for h in how.values() if h == 'legacy')} on their legacy three), {len(doc['items'])} items made")
    return 0


if __name__ == "__main__":
    sys.exit(main())
