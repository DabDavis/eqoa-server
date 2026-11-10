#!/usr/bin/env python3
"""The lore lines (tools/dialog/lore_lines.json, the lore set of tools/seed_town_dialog.py; the user,
2026-11-10: "Let's add more lines, the NPCs talk about random eqoa lore (remember this is before EQ1
lorewise)") held to their rules:

  - every line carries a note naming its facts, and every fact named is in tools/dialog/LORE-FACTS.md
    (lore is not invented: a line stands on a source, and the sheet says which and where);
  - every line fits the client: one dialogue box of 217 characters with the longest speaker's name and
    the longest words it may be filled with, only characters a PS2's text box and font take (no '|',
    which its TextBox reads as a code: claude-notes/ps2-textbox-bar-crash-20261107.md), written as the
    town lines are (no contractions);
  - no line names a thing of EverQuest's own day (EQOA is five hundred years before it): BANNED below;
  - no two lines are alike, and none is like a line of the town, race or NPC files;
  - every one of the 31 towns has lore of its own, for its people and for its guards; every race has
    its voice, and is spoken to; no line is one NPC's (a name's lines would silence its town lines).

    python3 tools/seed_town_dialog_lore_test.py
"""
import collections
import difflib
import json
import os
import re
import sys
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import seed_town_dialog as std  # noqa: E402

LORE = std.SETS["lore"][0]
FACTS = os.path.join(HERE, "dialog", "LORE-FACTS.md")
OTHERS = [std.SETS["town"][0], os.path.join(HERE, "dialog", "race_lines.json"), os.path.join(HERE, "dialog", "npc_lines.json")]

# The names of EverQuest's own day, five hundred years on, that no one in EQOA's Tunaria can know: the
# continents' later names and the lands not yet found, the cities not yet built (the elves are in
# Fayspires and Tethelin, the gnomes in Klick'Anon, the dwarves in Moradhim, the Erudites in Highbourne),
# its rulers and its orders. A name EQOA's own sources use in an earlier form (Paineel, a city the
# Faceless's Erudites are only now building) is kept out too: the lines say what is, not its later name.
BANNED = [
    "Antonica", "Faydwer", "Faydark", "Kelethin", "Felwithe", "Kaladim", "Ak'Anon", "Akanon", "Erudin", "Paineel",
    "Kunark", "Velious", "Luclin", "Plane of Knowledge", "Lucan", "D'Lere", "Firiona", "Iksar", "Cabilis", "Sebilis",
    "Vah Shir", "Shar Vahl", "Thurgadin", "Kael", "Skyshrine", "Crushbone", "Butcherblock", "Steamfont", "High Keep",
    "Highkeep", "Kedge Keep", "Gukta", "Splitpaw", "Najena", "Befallen", "Knights of Truth", "Steel Warriors",
    "Qeynos Hills", "Kane Bayle", "Bayle III", "Bayle IV", "Bayle the Third", "Bayle the Fourth", "Kerafyrm",
    "Tserrina", "Nagafen's Lair", "Bloodsaber", "Circle of Unseen Hands", "Shadow Haven", "Taelosia",
    "Age of Turmoil", "EverQuest",
]
# what a line's words are filled with, at their longest: a character's name is 12 at most (CLIENT's
# name box), "barbarian", "shadowknight", "Muniel's Tea Garden", and a speaker's name (25 covers the
# longest on eqoaclassic, 23)
FILLS = {"%p": "P" * 12, "%r": "R" * 9, "%c": "C" * 12, "%t": "T" * 19, "%n": "N" * 25}
BOX, NAME = 217, 25
# the characters the town lines are written in: letters, digits, the space and these marks
CHARS = re.compile(r"[A-Za-z0-9 !',.:;?-]+")
CONTRACTION = re.compile(r"\b\w+n't\b|\b\w+'(?:re|ve|ll|d|m)\b|\b(?:it|that|there|he|she|what|who|let|here|where)'s\b", re.I)
TOWNS = ["Blackwater", "Bobble-by-Water", "Castle Lightwolf", "Dark Solace", "Darvar Manor", "Fayspires", "Forkwatch",
         "Fort Seriak", "Freeport", "Gerntar", "Graumord", "Grobb", "Halas", "Hazinak", "Highbourne", "Highpass", "Honjour",
         "Kerplunk Outpost", "Klick`Anon", "Moradhim", "Muniels Tea Garden", "Murnf", "Neriak", "Oasis", "Oggok", "Qeynos",
         "Rivervale", "Surefall Glade", "Tethelin", "Wyndhaven", "Zentar's Keep"]
# the towns whose people do not speak the common tongue's lines (entity.BrokenSpeech's races are theirs)
BROKEN_TOWNS = {"Grobb", "Honjour", "Oggok", "Kerplunk Outpost"}


def lore():
    """The lore lines as (row, note) pairs."""
    pairs, problems = std.blocks_of(LORE)
    assert not problems, problems
    return pairs


def plain(line):
    """A line as it is compared: its words alone."""
    for k in FILLS:
        line = line.replace(k, " ")
    return " ".join(re.findall(r"[a-z0-9']+", line.lower()))


def alike(a, b):
    """Whether two lines (as plain) are near enough to be one said twice."""
    wa, wb = set(a.split()), set(b.split())
    if not wa or not wb or len(wa & wb) < 0.5 * min(len(wa), len(wb)):
        return False
    return difflib.SequenceMatcher(None, a, b).ratio() >= 0.8


class LoreLines(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.pairs = lore()
        with open(FACTS, encoding="utf-8") as f:
            cls.facts = set(re.findall(r"^- \*\*([A-Z][A-Z0-9]*-[0-9]+)\*\* ", f.read(), re.M))

    def test_how_many(self):
        self.assertGreaterEqual(len(self.pairs), 400)
        self.assertLessEqual(len(self.pairs), 620)

    def test_every_line_names_its_facts(self):
        self.assertGreater(len(self.facts), 100, "the facts sheet read empty")
        for r, note in self.pairs:
            ids, sep, words = note.partition(": ")
            self.assertTrue(sep and words.strip(), f"no note on {r['line']!r}")
            named = [i.strip() for i in ids.split(",")]
            self.assertTrue(named and all(named), f"no fact named for {r['line']!r}")
            for i in named:
                self.assertIn(i, self.facts, f"{r['line']!r} names a fact the sheet lacks")

    def test_every_fact_of_the_sheet_has_a_source(self):
        with open(FACTS, encoding="utf-8") as f:
            text = f.read()
        blocks = re.split(r"^- \*\*", text, flags=re.M)[1:]
        self.assertEqual(len(blocks), len(self.facts))
        for b in blocks:
            self.assertRegex(b, r"\n  - \S", f"no source under {b[:40]!r}")

    def test_every_line_fits_the_client(self):
        for r, _ in self.pairs:
            line = r["line"]
            self.assertTrue(CHARS.fullmatch(std.FILLS.sub("", line)), f"a character the town lines do not use: {line!r}")
            self.assertEqual(line, line.strip())
            self.assertNotIn("  ", line)
            filled = line
            for k, v in FILLS.items():
                filled = filled.replace(k, v)
            self.assertLessEqual(NAME + 2 + len(filled), BOX, f"past one box: {line!r}")
            self.assertRegex(line, r"[.!?]$", f"no end to {line!r}")
            self.assertIsNone(CONTRACTION.search(line), f"a contraction (the town lines have none): {line!r}")
            if not r["town"] and "%t" in line and r["speaker_race"] < 0:
                self.fail(f"a common line names %t (a guard on a road has no town): {line!r}")

    def test_no_line_names_everquests_day(self):
        for r, _ in self.pairs:
            for word in BANNED:
                self.assertIsNone(re.search(r"(?<![A-Za-z])" + re.escape(word) + r"(?![a-z])", r["line"], re.I),
                                  f"{word!r} in {r['line']!r}")
        self.assertIsNone(re.search(r"(?<![A-Za-z])Antonica(?![a-z])", "the continent is Tunaria"))
        self.assertIsNotNone(re.search(r"(?<![A-Za-z])" + re.escape("Faydwer") + r"(?![a-z])", "They sailed to Faydwer.", re.I))

    def test_no_two_lines_are_alike(self):
        mine = [(plain(r["line"]), r["line"]) for r, _ in self.pairs]
        self.assertEqual(len({p for p, _ in mine}), len(mine), "the same words twice")
        others = []
        for path in OTHERS:
            with open(path, encoding="utf-8") as f:
                for b in json.load(f)["blocks"]:
                    for said in b["say"]:
                        line = said["line"] if isinstance(said, dict) else said
                        others.append((plain(line), line))
        for i, (a, la) in enumerate(mine):
            for b, lb in mine[i + 1:]:
                self.assertFalse(alike(a, b), f"alike: {la!r} and {lb!r}")
            for b, lb in others:
                self.assertFalse(alike(a, b), f"{la!r} is like the earlier line {lb!r}")
        self.assertTrue(alike(plain("The Marr twins made the Northmen."), plain("The Marr twins made the Northmen, %p!")))
        self.assertFalse(alike(plain("Brell gave us the stone."), plain("Tunare gave the elves the green world.")))

    def test_every_town_and_every_people(self):
        by_town, guards = collections.Counter(), collections.Counter()
        voices, spoken_to = collections.Counter(), collections.Counter()
        for r, _ in self.pairs:
            self.assertEqual(r["name"], "", f"a line of one NPC's own: {r['line']!r}")
            if r["town"]:
                self.assertIn(r["town"], TOWNS, r["line"])
                by_town[r["town"]] += 1
                if r["role"] == "guard":
                    guards[r["town"]] += 1
            if r["speaker_race"] >= 0:
                voices[r["speaker_race"]] += 1
            if r["race"] >= 0:
                self.assertIn(r["race"], range(10))
                spoken_to[r["race"]] += 1
            self.assertGreaterEqual(r["weight"], 1)
            self.assertLessEqual(r["weight"], 3)
        for t in TOWNS:
            self.assertGreaterEqual(by_town[t], 6, f"{t} has too little lore of its own")
            self.assertGreaterEqual(guards[t], 1, f"{t}'s guards have no lore of their own")
        for race in range(10):
            self.assertGreaterEqual(voices[race], 4, f"race {race} has no voice of its own")
            self.assertGreaterEqual(spoken_to[race], 1, f"no line is said to race {race}")

    def test_trolls_and_ogres_keep_their_speech(self):
        """A troll's or an ogre's line is in their speech: none says "I", "am", "is", "are", "the" or "a"
        as the common tongue does (the town and race files' lines of theirs say "me", "troll", "ogre")."""
        common = re.compile(r"\b(I|am|is|are|was|were|the|an?|my|have|has|does|did)\b")
        for r, _ in self.pairs:
            if r["town"] in BROKEN_TOWNS or r["speaker_race"] in (5, 9):
                self.assertIsNone(common.search(r["line"]), f"not a troll's or an ogre's speech: {r['line']!r}")
            if r["town"] in BROKEN_TOWNS:
                self.assertLess(r["speaker_race"], 0)


if __name__ == "__main__":
    unittest.main()
