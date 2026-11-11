#!/usr/bin/env python3
"""tools/quests/gen_homelands.py: the homelands' starter quests held to the cities' template.

- tools/quests/data/homelands.json is what the specs give (a spec changed and the file not written again fails here);
- a race with a spec has exactly L1-L5 and its flight, each on the template: the levels, the chain, the cities'
  experience, the steps' kinds, the drop chances, every class's reward at 3, 4 and 5 as class-rewards.json has it,
  the classquest event of its level, the flight after all five with its teleport to the mainland and its bind;
- a race without one keeps its legacy three (one with a spec has no use for an entry there);
- every NPC and creature a spec names stands in its home of tools/worlds/homelands.json, in its role (a giver, a
  merchant, the spiritmaster, the coachman and the gatekeeper talkers, a hunt's target a foe with a rule), and a
  merchant that hands things over sells something (a window: CLIENT asks a merchant for nothing else);
- a journal line's north, south, east and west agree with where its step's NPC or creatures stand from the home's
  start in the recipe (+x is east, +z SOUTH: a word turned round is caught, as the worked example's were);
- what the client cannot show is refused (a box's line with its speaker's name, a question, an answer, a journal
  line), and so is a spec that leaves the template (made-up specs: the controls);
- with EQOA_REAL_DB (a copy of a database the homes are seeded in): tools/quests/check_quests.py finds no error in
  the file, and every class's reward is its own (a spell of its classspells, an item it may use) and what that
  database's city class quests of the level give it most.

    python3 tools/quests/gen_homelands_test.py
    EQOA_REAL_DB=/path/to/seeded-copy.db python3 tools/quests/gen_homelands_test.py
"""
import collections
import copy
import json
import os
import re
import shutil
import sqlite3
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import check_quests as cq  # noqa: E402
import gen_homelands as gh  # noqa: E402

RECIPE = os.path.join(HERE, "..", "worlds", "homelands.json")
REAL_DB = os.environ.get("EQOA_REAL_DB", "")


def load(path):
    with open(path) as f:
        return json.load(f)


# The game's compass: +x is east, +z is SOUTH (north is toward smaller z: Halas at z 4,289, Grobb at 31,410; the
# client's map draws -z up). A direction word is wrong when the thing lies clearly the other way.
COMPASS = re.compile(r"\b(north|south)?[- ]?(east|west)?\b", re.I)
CLEARLY = 20.0


def direction_problems(text, dx, dz):
    """What a text's compass words get wrong of something dx east and dz south of where the text is read from."""
    out = []
    for m in COMPASS.finditer(text):
        ns, ew = (m.group(1) or "").lower(), (m.group(2) or "").lower()
        if ns == "north" and dz > CLEARLY:
            out.append(f"north, and it is {dz:.0f} to the south")
        if ns == "south" and dz < -CLEARLY:
            out.append(f"south, and it is {-dz:.0f} to the north")
        if ew == "east" and dx < -CLEARLY:
            out.append(f"east, and it is {-dx:.0f} to the west")
        if ew == "west" and dx > CLEARLY:
            out.append(f"west, and it is {dx:.0f} to the east")
    return out


class TheFile(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.doc, cls.how, cls.problems = gh.build()
        cls.rewards = gh.load_rewards()
        cls.by_race = collections.defaultdict(dict)
        for q in cls.doc["quests"]:
            _, race, *part = q["key"].split("/")
            cls.by_race[race]["/".join(part)] = q

    def test_the_file_is_what_the_specs_give(self):
        self.assertEqual(self.problems, [])
        with open(gh.OUT) as f:
            self.assertEqual(f.read(), gh.dumps(self.doc), "run tools/quests/gen_homelands.py")
        again, _, _ = gh.build()
        self.assertEqual(gh.dumps(again), gh.dumps(self.doc), "the same specs, the same file")
        self.assertEqual(gh.main(["--check"]), 0)

    def test_every_home_is_a_spec_or_its_legacy_three(self):
        self.assertEqual(sorted(self.how), sorted(gh.RACES))
        legacy = gh.load_legacy()
        for race in gh.RACES:
            spec = os.path.exists(os.path.join(gh.SPECS, race + ".json"))
            self.assertTrue(spec or race in legacy, f"{race}: a spec, or its legacy quests")
            self.assertEqual(self.how[race], "spec" if spec else "legacy", f"{race}: a spec is taken over its legacy three")
            parts = sorted(self.by_race[race])
            if spec:
                higher = [k for k in gh.HIGHER if f"L{k}" in load(os.path.join(gh.SPECS, race + ".json"))]
                self.assertEqual(higher, list(gh.HIGHER[:len(higher)]), f"{race}: levels past the flight in order, none skipped")
                want = ["L1", "L2", "L3", "L4", "L5", "flight"] + [f"L{k}" for k in higher if k != 20]
                if 20 in higher:
                    want += [f"L20/{c}" for c in gh.CLASS_NAMES]
                self.assertEqual(parts, sorted(want), race)
            else:
                self.assertEqual(len(parts), 3, race)
                self.assertEqual([q["key"] for q in legacy[race]], [q["key"] for q in self.doc["quests"] if q["key"].split("/")[1] == race])

    def test_a_spec_is_on_the_cities_template(self):
        spec_races = [r for r in gh.RACES if self.how[r] == "spec"]
        self.assertIn("aviak", spec_races, "the worked example")
        for race in spec_races:
            qs = self.by_race[race]
            for k in gh.LEVELS:
                q = qs[f"L{k}"]
                w = f"{race} L{k}"
                self.assertEqual((q["minlevel"], q["maxlevel"], q["classes"], q["races"]), (k, 60, [], [gh.RACE_IDS[race]]),
                                 f"{w}: every class of the home's own race, and no other race's")
                self.assertEqual(q["requires"], [f"homelands/{race}/L{k - 1}"] if k > 1 else [], w)
                self.assertEqual(q["events"], [{"on": "finish", "do": "classquest", "level": k}], w)
                self.assertNotIn("repeatable", q, w)
                last = q["steps"][-1]
                self.assertEqual(last["target"], q["giver"], f"{w}: handed in to its giver")
                self.assertEqual(last["rewards"]["xp"], gh.XP[k], w)
                self.assertEqual([st for st in q["steps"][:-1] if "rewards" in st], [], f"{w}: the reward is the last step's")
                if k >= 3:
                    self.assertEqual(last["rewards"]["byclass"], {c: self.rewards[c][f"L{k}"] for c in map(str, range(15))}, w)
                else:
                    self.assertNotIn("byclass", last["rewards"], w)
            kinds = {k: [st["kind"] for st in qs[f"L{k}"]["steps"]] for k in gh.LEVELS}
            self.assertEqual(kinds[1], ["talk", "show"], race)
            piece = qs["L1"]["steps"][1]["items"]
            self.assertEqual(qs["L1"]["steps"][0]["events"], [{"on": "done", "do": "give", "items": piece}], race)
            self.assertEqual(kinds[2], ["talk", "talk", "talk"], race)
            self.assertTrue(qs["L2"]["steps"][0]["target"].startswith("Spiritmaster "), race)
            self.assertTrue(qs["L2"]["steps"][1]["target"].startswith(("Coachman ", "Coachwoman ")), race)
            self.assertEqual(kinds[3], ["loot", "give"], race)
            hunt = qs["L3"]["steps"][0]
            self.assertEqual((hunt["chance"], hunt["items"]), (0.5, qs["L3"]["steps"][1]["items"]), race)
            self.assertIn(hunt["count"], (2, 3), race)
            self.assertIn(kinds[4], (["talk", "loot", "give"], ["talk", "talk", "loot", "give"]), race)
            handed = [it for st in qs["L4"]["steps"] if st["kind"] == "talk" for e in st["events"] for it in e["items"]]
            self.assertEqual(qs["L4"]["steps"][-1]["items"], handed + qs["L4"]["steps"][-2]["items"],
                             f"{race} L4: what the merchants hand over and the drop are what is handed in")
            self.assertIn(kinds[5], (["loot", "give"], ["kill", "talk"]), race)
            f = qs["flight"]
            self.assertEqual(f["requires"], [f"homelands/{race}/L{k}" for k in gh.LEVELS], race)
            self.assertTrue(f["repeatable"] and f["giver"].startswith("Gatekeeper "), race)
            self.assertEqual(f["races"], [gh.RACE_IDS[race]], f"{race}: the flight is its own race's")
            self.assertEqual(f["steps"][0]["events"], [{"on": "start", "do": "dostep"},
                                                       {"on": "done", "do": "teleport", "bind": True, "to": "mainland"}], race)
            self.assertNotIn("rewards", f["steps"][0], f"{race}: the flight gives nothing (it is repeatable)")
            self.higher_on_template(race, qs)

    def higher_on_template(self, race, qs):
        """Levels 7-20 (the chain past the flight): each the cities' level, after the one before, the cities'
        experience, its class rewards (L20: one quest a class, its city's path pick), the classquest event."""
        prev = 5
        for k in gh.HIGHER:
            keys = [f"L{k}"] if k != 20 else [f"L20/{c}" for c in gh.CLASS_NAMES]
            if keys[0] not in qs:
                break
            for i, key in enumerate(keys):
                q, w = qs[key], f"{race} {key}"
                self.assertEqual((q["minlevel"], q["maxlevel"], q["races"]), (k, 60, [gh.RACE_IDS[race]]), w)
                self.assertEqual(q["classes"], [] if k != 20 else [i], w)
                self.assertEqual(q["requires"], [f"homelands/{race}/L{prev}"], w)
                self.assertEqual(q["events"], [{"on": "finish", "do": "classquest", "level": k}], w)
                self.assertNotIn("repeatable", q, w)
                last = q["steps"][-1]
                self.assertEqual(last["target"], q["giver"], f"{w}: the last word is its giver's")
                self.assertEqual(last["rewards"]["xp"], gh.XP[k], w)
                self.assertEqual([st for st in q["steps"][:-1] if "rewards" in st], [], f"{w}: the reward is the last step's")
                if k == 20:
                    self.assertNotIn("byclass", last["rewards"], w)
                    self.assertEqual(last["choose"], gh.PATH_ASK, w)
                    self.assertEqual([{"answer": o["answer"], **o["rewards"]} for o in last["choices"]], self.rewards[str(i)]["L20"],
                                     f"{w}: the class's own city path pick")
                else:
                    want = {c: self.rewards[c][f"L{k}"] for c in map(str, range(15)) if self.rewards[c][f"L{k}"]}
                    self.assertEqual(last["rewards"].get("byclass"), want, w)
                kinds = [st["kind"] for st in q["steps"]]
                if k == 7:
                    self.assertIn(kinds, (["loot", "give"], ["loot", "loot", "give"]), w)
                    self.assertEqual(last["items"], [st["items"][0] for st in q["steps"][:-1]], f"{w}: the drops handed in")
                elif k == 10:
                    self.assertIn(kinds, (["loot", "give"], ["talk", "loot", "give"], ["talk", "talk", "loot", "give"]), w)
                elif k == 13:
                    self.assertIn(kinds, (["kill", "talk"], ["loot", "give"]), w)
                elif k == 15:
                    self.assertIn(kinds, (["loot", "give"], ["loot", "give", "talk"]), w)
                    self.assertEqual(q["steps"][0]["chance"], 1.0, f"{w}: a named foe always drops")
                else:
                    loot = kinds.index("loot")
                    self.assertTrue(1 <= loot <= 3 and kinds[:loot] == ["talk"] * loot, w)
                    self.assertEqual(kinds[loot + 1], "give", w)
                    self.assertTrue(all(kd == "talk" for kd in kinds[loot + 2:]), w)
            if k == 20:
                for key in keys[1:]:
                    self.assertEqual(qs[key]["steps"][:-1], qs[keys[0]]["steps"][:-1], f"{race}: the classes' roads are one")
            prev = k

    def test_the_made_items(self):
        names = [it["name"].lower() for it in self.doc["items"]]
        self.assertEqual(len(names), len(set(names)), "an item is made once")
        pieces = {q["steps"][1]["items"][0]["item"] for r in gh.RACES if self.how[r] == "spec" for q in [self.by_race[r]["L1"]]}
        for it in self.doc["items"]:
            self.assertTrue(it["notrade"] and it["description"] and it["like"], it["name"])
            if it["name"] in pieces:
                s = it["set"]
                self.assertEqual({k: s[k] for k in gh.PIECE_FIXED}, gh.PIECE_FIXED, f"{it['name']}: every class's, level 1, free")
                figures = {k: v for k, v in s.items() if k not in gh.PIECE_FIXED}
                self.assertTrue(figures and all(0 < v <= gh.PIECE_FIGURES[k] for k, v in figures.items()), it["name"])
            else:
                self.assertNotIn("set", it, it["name"])

    def test_the_words_fit_the_client(self):
        for race in gh.RACES:
            if self.how[race] != "spec":
                continue
            for q in self.by_race[race].values():
                w = q["key"]
                offer = q["offer"].split("\n")
                self.assertGreaterEqual(len(offer), 2, w)
                self.assertLessEqual(len(offer[-1]), gh.ASK, f"{w}: the question")
                self.assertTrue(offer[-1].endswith("?"), f"{w}: the offer ends on its question")
                for a in (q["accept"], q["decline"]):
                    self.assertLessEqual(len(a), gh.ANSWER, w)
                for line in offer[:-1] + q["declined"].split("\n") + q["started"].split("\n"):
                    self.assertLessEqual(len(line) + len(q["giver"]) + 2, gh.BOX, f"{w}: {line[:40]}")
                for st in q["steps"]:
                    self.assertLessEqual(len(st["journal"]), gh.LINE, w)
                    for line in (st.get("says") or "").split("\n"):
                        self.assertLessEqual(len(line) + len(st["target"]) + 2, gh.BOX, f"{w}: {line[:40]}")

    def test_the_specs_people_stand_in_their_homes(self):
        recipe = load(RECIPE)
        homes = {}
        shared = [z["shared"] for z in recipe["zones"] if z.get("shared")]
        self.assertEqual(len(shared), 1, "one shared ground")
        for z in recipe["zones"]:
            h = z.get("home")
            if h:
                homes[z["race"]] = (z["name"], h)
        ids = gh.RACE_IDS
        self.assertEqual(sorted(ids), sorted(gh.RACES))
        self.assertEqual(sorted(ids.values()), sorted(homes), "a home of the recipe for every race, and no other")
        for race in gh.RACES:
            if self.how[race] != "spec":
                continue
            zone, h = homes[ids[race]]
            people = collections.defaultdict(list)
            for n in h["npcs"] + shared[0]["npcs"]:   # past the flight, the shared ground's people are a home's too
                people[n["name"]].append(n)
            rules = h.get("peaceful", []) + h.get("hostile", []) + shared[0].get("peaceful", []) + shared[0].get("hostile", [])
            talkers, foes, hands = set(), set(), set()
            for q in self.by_race[race].values():
                talkers.add(q["giver"])
                for st in q["steps"]:
                    (foes if st["kind"] in ("kill", "loot") else talkers).add(st["target"])
                    if st["kind"] == "talk" and any(e["do"] == "give" for e in st.get("events", [])):
                        hands.add(st["target"])
            for name in sorted(talkers):
                self.assertIn(name, people, f"{race}: {name} is none of {zone}'s people (tools/worlds/homelands.json)")
                self.assertTrue(all(n.get("role", "talker") == "talker" for n in people[name]), f"{race}: {name} is talked to")
            for name in sorted(foes):
                self.assertIn(name, people, f"{race}: {name} is none of {zone}'s creatures (tools/worlds/homelands.json)")
                self.assertTrue(all(n.get("role") == "foe" for n in people[name]), f"{race}: {name} is hunted: a foe")
                self.assertIn(name, rules, f"{race}: {name} has a rule (peaceful or hostile)")
            hunted = [st["target"] for q in self.by_race[race].values() for st in q["steps"]
                      if st["kind"] == "loot" and st.get("chance") == gh.CHANCE]
            for name in hunted:
                self.assertGreaterEqual(len(people[name]), 6, f"{race}: {name}: six spawns or more for a hunt two players share")
            for name in sorted(hands):
                self.assertTrue(all(n.get("stock") for n in people[name]),
                                f"{race}: {name} hands things over and is a merchant of the home: give it a stock (a window that sells)")
            for kind, n in (("Coachman ", 1), ("Spiritmaster ", 1), ("Gatekeeper ", 1)):
                have = [n_["name"] for n_ in h["npcs"] if n_["name"].startswith(kind)]
                self.assertEqual(len(set(have)), n, f"{zone}: one {kind.strip()}")
        # the shared ground: a stable and a spiritmaster, its hunts' kinds six spawns or more, every creature with a rule
        sh = shared[0]
        names = collections.Counter(n["name"] for n in sh["npcs"])
        self.assertEqual(len([n for n in names if n.startswith(("Coachman ", "Coachwoman "))]), 1)
        self.assertEqual(len([n for n in names if n.startswith("Spiritmaster ")]), 1)
        for n in sh["npcs"]:
            if n["role"] == "foe":
                self.assertIn(n["name"], sh["peaceful"] + sh["hostile"], n["name"])


class TheCompass(unittest.TestCase):
    def test_a_journals_directions_are_where_its_step_goes(self):
        doc, how, _ = gh.build()
        recipe = load(RECIPE)["zones"]
        homes = {z["race"]: (z["name"], z["home"]) for z in recipe if z.get("home")}
        shared = [z for z in recipe if z.get("shared")][0]
        stable = [n["at"] for n in shared["shared"]["npcs"] if n["name"].startswith(("Coachman ", "Coachwoman "))][0]
        for q in doc["quests"]:
            _, race, part = q["key"].split("/", 2)
            if how[race] != "spec" or part == "flight":   # a flight's north is its city's
                continue
            zone, h = homes[gh.RACE_IDS[race]]
            for st in q["steps"]:
                # a home's people from its start; the shared ground's from its stable, where the coach sets a rider down
                sx, sz = h["start"]["x"], h["start"]["z"]
                at = [n["at"] for n in h["npcs"] if n["name"] == st["target"]]
                if not at:
                    zone, (sx, _, sz) = shared["name"], stable
                    at = [n["at"] for n in shared["shared"]["npcs"] if n["name"] == st["target"]]
                if not at:
                    continue
                dx = sum(a[0] for a in at) / len(at) - sx
                dz = sum(a[2] for a in at) / len(at) - sz
                wrong = direction_problems(st["journal"], dx, dz)
                self.assertEqual(wrong, [], f"{q['key']}: {st['journal']!r}: {st['target']} from {zone}'s start (+z is SOUTH)")

    def test_the_compass_itself(self):
        # a thing 30 west and 40 north (dx -30, dz -40): north-west of here
        self.assertEqual(direction_problems("Ask the weaver, north-west of the elder.", -30, -40), [])
        self.assertEqual(len(direction_problems("Ask the weaver, south-west of the elder.", -30, -40)), 1)
        self.assertEqual(len(direction_problems("Ask the weaver, south-east of the elder.", -30, -40)), 2)
        self.assertEqual(len(direction_problems("The rats on the grass South of the Roost.", 5, -300)), 1)
        self.assertEqual(direction_problems("The rats on the grass north of the Roost.", 5, -300), [])
        self.assertEqual(direction_problems("Up the east rise.", 390, 90), [])
        self.assertEqual(len(direction_problems("Up the West rise.", 390, 90)), 1)
        # beside the reader, any word passes; a word that is no compass word says nothing
        self.assertEqual(direction_problems("Just south of here, by the feast.", 3, -12), [])
        self.assertEqual(direction_problems("Bring the beast's nest to the western hall.", 500, 500), [])


class TheControls(unittest.TestCase):
    """Made-up specs: what the generator refuses."""

    def setUp(self):
        self.dir = tempfile.mkdtemp()
        for f in ("class-rewards.json", "legacy.json", "aviak.json"):
            shutil.copy(os.path.join(gh.SPECS, f), self.dir)
        # a race that came after the legacy three (the Mermaid, 32) has its spec alone
        legacy_races = load(os.path.join(gh.SPECS, "legacy.json"))["races"]
        for race in gh.RACES:
            if race not in legacy_races and race != "aviak":
                shutil.copy(os.path.join(gh.SPECS, race + ".json"), self.dir)
        self.spec = load(os.path.join(self.dir, "aviak.json"))
        # the aviak's legacy three back beside its spec would be both: the made-up legacy has none of it
        legacy = load(os.path.join(self.dir, "legacy.json"))
        legacy["races"].pop("aviak", None)
        with open(os.path.join(self.dir, "legacy.json"), "w") as f:
            json.dump(legacy, f)

    def tearDown(self):
        shutil.rmtree(self.dir)

    def problems(self, change):
        spec = copy.deepcopy(self.spec)
        change(spec)
        with open(os.path.join(self.dir, "aviak.json"), "w") as f:
            json.dump(spec, f)
        return "\n".join(gh.build(self.dir)[2])

    def test_the_spec_as_it_is_is_clean(self):
        self.assertEqual(self.problems(lambda s: None), "")

    def test_what_is_refused(self):
        def long_line(s):
            s["L1"]["fetch"]["says"] = "x" * 205   # 205 + "Weaver Tikkri: " is past a box
        self.assertIn("a box shows 217", self.problems(long_line))
        self.assertEqual(self.problems(lambda s: s["L1"]["fetch"].update(says="x" * 200)), "", "the control: it fits")

        def no_question(s):
            s["L3"]["offer"] = ["Hunt the rats.", "Bring me three tails."]
        self.assertIn("asks nothing", self.problems(no_question))
        self.assertIn("at least two lines", self.problems(lambda s: s["L3"].update(offer="Will you hunt them?")))
        self.assertIn("a question shows 127", self.problems(lambda s: s["L3"]["offer"].__setitem__(-1, "y" * 127 + "?")))
        self.assertIn("an answer shows 63", self.problems(lambda s: s["L2"].update(accept="a" * 64)))
        self.assertIn("the journal shows 127", self.problems(lambda s: s["L5"]["hunt"].update(journal="j" * 128)))
        self.assertIn("no merchant", self.problems(lambda s: s["L1"].pop("merchant")))
        self.assertIn("the piece's set is its figures", self.problems(lambda s: s["L1"]["piece"].update(set={"AC": 9})))
        self.assertIn("the piece's set is its figures", self.problems(lambda s: s["L1"]["piece"].update(set={"str": 2})))
        self.assertIn("the piece's set is its figures", self.problems(lambda s: s["L1"]["piece"].pop("set")))
        self.assertIn("is no spiritmaster", self.problems(lambda s: s["L2"].update(spiritmaster="Kreeal")))
        self.assertIn("is no coachman", self.problems(lambda s: s["L2"].update(coachman="Harrek")))
        self.assertIn("count 5", self.problems(lambda s: s["L3"].update(count=5)))
        self.assertIn("only the level 1 piece has a set", self.problems(lambda s: s["L3"]["drop"].update(set={"AC": 1})))
        self.assertIn("one or two merchants", self.problems(lambda s: s["L4"].update(materials=[])))
        self.assertIn("kind 'reach'", self.problems(lambda s: s["L5"].update(kind="reach")))
        self.assertIn("is no gatekeeper", self.problems(lambda s: s["flight"].update(giver="Elder Kraawk")))
        self.assertIn("L4: missing", self.problems(lambda s: s.pop("L4")))
        self.assertIn("says race 'eagle'", self.problems(lambda s: s.update(race="eagle")))
        self.assertIn("has ['L6']", self.problems(lambda s: s.update(L6={})))

    def test_the_chain_past_the_flight(self):
        # each level after the one before it; a home may stop at any level (a content lane adds them in order)
        self.assertIn("L10: comes after L7, which the spec lacks", self.problems(lambda s: s.pop("L7")))
        self.assertIn("L20: comes after L15, which the spec lacks", self.problems(lambda s: s.pop("L15")))
        self.assertEqual(self.problems(lambda s: [s.pop(f"L{k}") for k in (10, 13, 15, 20)]), "", "a home that stops at L7")
        self.assertEqual(self.problems(lambda s: [s.pop(f"L{k}") for k in (7, 10, 13, 15, 20)]), "", "one that stops at its flight")
        self.assertIn("hunts: one or two", self.problems(lambda s: s["L7"].update(hunts=[])))
        self.assertIn("count 4", self.problems(lambda s: s["L7"]["hunts"][0].update(count=4)))
        self.assertIn("materials: 0 to 2", self.problems(lambda s: s["L10"].update(materials=[{}, {}, {}])))
        self.assertIn("count 9", self.problems(lambda s: s["L13"].update(count=9)))
        self.assertIn("road: 1 to 3 stops", self.problems(lambda s: s["L20"].update(road=[])))
        self.assertIn("after: 0 to 2 stops", self.problems(lambda s: s["L20"].update(after=[{}, {}, {}])))
        self.assertIn("a box shows 217", self.problems(lambda s: s["L20"]["final"].update(says="x" * 205)))
        # a home of L10 with no materials, an L13 of a drop, an L15 handed to its giver, an L20 with a stop after
        def change(s):
            s["L10"]["materials"] = []
            s["L13"].update(kind="loot", drop={"name": "Frostjaw's fang", "description": "A fang.", "like": "Broken Leopard Claw"},
                            give={"journal": "Bring Frostjaw's fang to Huntmaster Arrok.", "says": "Good."})
            s["L15"].pop("carry")
            s["L15"]["give"] = {"journal": "Bring the shard to Huntmaster Arrok.", "says": "Good."}
            s["L20"]["after"] = [{"target": "Warden Halvek", "journal": "Tell Warden Halvek.", "says": "Go home."}]
        spec = copy.deepcopy(self.spec)
        change(spec)
        with open(os.path.join(self.dir, "aviak.json"), "w") as f:
            json.dump(spec, f)
        doc, _, problems = gh.build(self.dir)
        self.assertEqual(problems, [])
        q = {x["key"]: x for x in doc["quests"]}
        self.assertEqual([st["kind"] for st in q["homelands/aviak/L10"]["steps"]], ["loot", "give"])
        self.assertEqual([(st["kind"], st.get("chance")) for st in q["homelands/aviak/L13"]["steps"]], [("loot", 1.0), ("give", None)])
        self.assertEqual([st["kind"] for st in q["homelands/aviak/L15"]["steps"]], ["loot", "give"])
        self.assertEqual([st["target"] for st in q["homelands/aviak/L20/wizard"]["steps"]],
                         ["Warden Halvek", "Scout Feyla", "Goldfeather raider", "Scout Feyla", "Warden Halvek", "Elder Kraawk"])
        self.assertEqual(q["homelands/aviak/L20/wizard"]["classes"], [13])

    def test_a_kill_for_the_fifth_and_a_made_material(self):
        def change(s):
            s["L5"] = {**{k: s["L5"][k] for k in ("name", "giver", "offer", "accept", "decline", "declined", "started")},
                       "kind": "kill", "target": "savannah warthog", "count": 3,
                       "hunt": {"journal": "Kill three savannah warthogs."},
                       "back": {"journal": "Return to Huntmaster Arrok.", "says": "Three fewer."}}
            s["L4"]["materials"][0]["items"] = ["leather strip", {"name": "bundle of reeds", "description": "Reeds.", "like": "wooden shaft"}]
        spec = copy.deepcopy(self.spec)
        change(spec)
        with open(os.path.join(self.dir, "aviak.json"), "w") as f:
            json.dump(spec, f)
        doc, _, problems = gh.build(self.dir)
        self.assertEqual(problems, [])
        l5 = [q for q in doc["quests"] if q["key"] == "homelands/aviak/L5"][0]
        self.assertEqual([(st["kind"], st["target"], st.get("count")) for st in l5["steps"]],
                         [("kill", "savannah warthog", 3), ("talk", "Huntmaster Arrok", None)])
        self.assertEqual(l5["steps"][1]["rewards"]["xp"], 36500)
        self.assertIn("bundle of reeds", [it["name"] for it in doc["items"]])
        l4 = [q for q in doc["quests"] if q["key"] == "homelands/aviak/L4"][0]
        self.assertEqual([it["item"] for it in l4["steps"][-1]["items"]], ["leather strip", "bundle of reeds", "brush wasp stinger"])

    def test_a_home_with_neither_and_a_file_of_no_home(self):
        os.remove(os.path.join(self.dir, "aviak.json"))
        self.assertIn("aviak: no spec (aviak.json) and no legacy quests", "\n".join(gh.build(self.dir)[2]))
        with open(os.path.join(self.dir, "eagle.json"), "w") as f:
            json.dump({}, f)
        self.assertIn("eagle.json: no home is named 'eagle'", "\n".join(gh.build(self.dir)[2]))


@unittest.skipUnless(REAL_DB, "EQOA_REAL_DB: a copy of a database the homes are seeded in")
class OnARealDatabase(unittest.TestCase):
    def test_the_checker_finds_no_error(self):
        ck = cq.Check(cq.Names(REAL_DB), None)
        got = ck.file(gh.OUT, {})
        self.assertIsNotNone(got)
        doc, items = got
        for q in doc["quests"]:
            ck.quest("homelands.json", q, items)
        self.assertEqual(ck.errors, [])

    def test_each_class_is_given_its_own_as_the_cities_give_it(self):
        db = sqlite3.connect("file:" + REAL_DB + "?mode=ro", uri=True)
        rewards = gh.load_rewards()
        spell_ids = collections.defaultdict(list)
        for sid, name in db.execute("SELECT spellid, spellname FROM spellpattern"):
            spell_ids[name.strip().lower()].append(sid)
        own = {(c, s) for c, s in db.execute("SELECT class, spellid FROM classspells")}
        classes = ["warrior", "ranger", "paladin", "shadowknight", "monk", "bard", "rogue", "druid", "shaman", "cleric",
                   "magician", "necromancer", "enchanter", "wizard", "alchemist"]
        for c in range(15):
            for lv in (3, 5):
                (name,) = rewards[str(c)][f"L{lv}"]["spells"]
                mine = [s for s in spell_ids[name.lower()] if (c, s) in own]
                self.assertTrue(mine, f"{classes[c]} L{lv}: {name} is no spell of its own (classspells)")
                # what the cities' rows of that class and level give: the spell, or a scroll of its name
                given = collections.Counter()
                for (qid,) in db.execute("SELECT questid FROM quest_import_quests WHERE key GLOB ?", (f"1-20/*/{classes[c]}/L{lv}",)):
                    spells, items = db.execute("SELECT spells, rewarditems FROM queststeps WHERE questid = ? ORDER BY step DESC LIMIT 1",
                                               (qid,)).fetchone()
                    for s in [x for x in spells.replace(" ", "").split(",") if x]:
                        given[db.execute("SELECT spellname FROM spellpattern WHERE spellid = ?", (int(s),)).fetchone()[0].lower()] += 1
                    for it in [x for x in items.replace(" ", "").split(",") if x]:
                        nm = db.execute("SELECT itemname FROM itempattern WHERE patternid = ?", (int(it.split("*")[0]),)).fetchone()[0]
                        given[nm.lower().replace(" (rge)", "")] += 1
                self.assertTrue(given, f"{classes[c]} L{lv}: no city quest of the level here")
                self.assertEqual(given.most_common(1)[0][0], name.lower(), f"{classes[c]} L{lv}: the cities give {dict(given)}")
            # levels 7-20: each spell the class's own, each item one it may use (the path's scrolls too)
            for lv in ("L7", "L10", "L13", "L15", "L20"):
                rws = rewards[str(c)][lv] if lv != "L20" else rewards[str(c)]["L20"]
                for rw in (rws if isinstance(rws, list) else [rws]):
                    for name in rw.get("spells", []):
                        self.assertTrue([s for s in spell_ids[name.lower()] if (c, s) in own], f"{classes[c]} {lv}: {name} is no spell of its own")
                    for name in rw.get("items", []):
                        row = db.execute("SELECT classuse FROM itempattern WHERE lower(itemname) = lower(?) ORDER BY patternid LIMIT 1",
                                         (name,)).fetchone()
                        self.assertIsNotNone(row, f"{classes[c]} {lv}: no record {name!r}")
                        self.assertTrue(row[0] & (1 << c), f"{classes[c]} {lv}: {name} is not the class's to use")
            (item,) = rewards[str(c)]["L4"]["items"]
            row = db.execute("SELECT classuse, levelreq FROM itempattern WHERE lower(itemname) = lower(?) ORDER BY patternid LIMIT 1",
                             (item,)).fetchone()
            self.assertIsNotNone(row, f"{classes[c]} L4: no record {item!r}")
            self.assertTrue(row[0] & (1 << c), f"{classes[c]} L4: {item} is not the class's to use")
            self.assertLessEqual(row[1], 5, f"{classes[c]} L4: {item} is level {row[1]}")


if __name__ == "__main__":
    unittest.main()
