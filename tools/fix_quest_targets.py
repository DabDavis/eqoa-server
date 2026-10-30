#!/usr/bin/env python3
"""Quest kill and loot steps given the creatures their journal names (the
user, 2026-10-30: "we need to look at drop rates for items, including quest
drops. for example dust was farming beaks for the weapon quest and it never
drops").

A kill or loot step counts the creatures named its target, as the server
matches a name (internal/handlers/quests.go's stepNPC: trimmed, any case),
and the steps tools/import_quests.py wrote name ONE: Neriak's shadow knight's
"New Gear" (EQOA-Data: "Chipped Raven Beak - Drop from ravens and magpies")
dropped its beak only off a "raven", its journal "Take the chipped raven beak
from the ravens and magpies outside the city.", while within 600 of its giver
stand magpies, crows and Bloodfeather Ravens and the two ravens 750 and 980
off. A player killing the magpies the journal names never saw a beak. The
engine now takes several creatures a step (table queststeptargets, the step's
own target first).

**The audit** (every kill and loot step, the report's TSV with --tsv):
- the spawns its names match, anywhere, and near the quest: within NEAR of
  any of its places (its giver, the NPCs its talk, show and give steps send
  players to, its reach steps' places), in the same world;
- their levels against the quest's lowest (FIT_ABOVE levels above it, at most,
  fit a player taking the quest);
- the creatures its journal line names: every creature's name (an npcs name
  without the NPC bit, as the line says it, plural or not), the step's items'
  names and the quest's people's taken out first;
- a step whose creature a spawn event of the quest brings (the step's start,
  the step before's done) is the event's: nothing about where its kind stands
  matters.
Flags: nomatch (no spawn of its names anywhere), npcbit (every one carries the
NPC bit: none can be attacked), far (none within REGION of the quest: another
region or world), high (every one more than HIGH_ABOVE levels above the
quest's lowest, and above its highest when it has one), journal (the line
names, as where its item drops, creatures near the quest at fitting levels the
step does not count).

**The fix** (a flagged step only): the step's creatures become those its
journal names that stand near the quest at fitting levels, its own target
among them when it does; else (none near and fitting) the nearest fitting
creatures of the same kind as its target or the journal's (the last word of
the name: "Bloodfeather Raven" for "raven"), within FAR_KIND. The step's own
target stays when any of its spawns is anywhere (another place's still count);
one with none is replaced by the first of the new names. Each name is written
as npcs spells it, trimmed. A step it cannot decide is said, for the user.

**Also reported** (no fix): give and show steps wanting an item nothing
provides: no step of the quest before it drops, gives or rewards it, no
merchant sells it, no creature's corpse or loot table carries it, no quest,
recipe, catch or starting kit gives it.

Every row written is logged in quest_targets_fix_log; --undo takes back each
one still as written (a name's case aside: tools that lower NPC names may have
run since). Report only by default. The server reads quests as it goes: no
restart (a running engine reads them at its next Reload: the GM panel's quest
save, or a restart).

    tools/fix_quest_targets.py [--db eqoa.db] [--tsv audit.tsv] [--apply | --undo]
"""
import argparse
import collections
import datetime
import json
import math
import os
import re
import shutil
import sqlite3
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")

NEAR = 1200  # a creature this near one of the quest's places stands where the quest is played (units, x and z)
REGION = 4000  # farther than this from every place of the quest is another region (a zone is 2000 a side)
FAR_KIND = 2500  # how far the nearest creatures of the target's kind are looked for, when none named is near
FIT_ABOVE = 3  # a creature fits a player taking a quest up to this many levels above the quest's lowest
FIT_BELOW = 10  # and down to this many below
HIGH_ABOVE = 5  # a step whose every creature is more than this many levels above the quest's lowest is flagged
LOW_QUEST = 20  # a quest up to this level whose creatures are far or above it takes the nearest fitting of their kind

LOG_DDL = """CREATE TABLE IF NOT EXISTS quest_targets_fix_log (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    questid INTEGER NOT NULL,
    step    INTEGER NOT NULL,
    action  TEXT NOT NULL,   -- added (a queststeptargets row) | target (queststeps.target changed)
    old     TEXT NOT NULL,   -- target: the step's target before; added: ''
    new     TEXT NOT NULL,   -- the name written
    why     TEXT NOT NULL,
    applied TEXT NOT NULL
)"""
TARGETS_DDL = """CREATE TABLE IF NOT EXISTS queststeptargets (
    questid INTEGER NOT NULL,
    step    INTEGER NOT NULL,
    target  TEXT NOT NULL,
    PRIMARY KEY (questid, step, target)
)"""


def fold(s):
    """A name as the server matches one: trimmed, any case (and the backtick
    the npcs table spells Teir'dal names with)."""
    return " ".join((s or "").replace("`", "'").replace("’", "'").lower().split())


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


# ---- words


IRREGULAR = {"wolf": "wolves", "elf": "elves", "dwarf": "dwarves", "thief": "thieves", "knife": "knives", "wife": "wives",
             "mouse": "mice", "louse": "lice", "goose": "geese", "ox": "oxen", "child": "children", "foot": "feet",
             "tooth": "teeth", "man": "men", "woman": "women", "fungus": "fungi", "cactus": "cacti", "larva": "larvae",
             "leaf": "leaves", "calf": "calves", "half": "halves", "loaf": "loaves", "scarf": "scarves"}
SAME_PLURAL = {"sheep", "deer", "fish", "moose", "bison", "elk", "swine", "trout", "salmon", "cattle"}

# A creature a line names is where an item drops (a "source") when, read back
# from it, a verb of killing or "from" comes before any word of place or
# company: "from the ravens and magpies outside the city", "Kill crows or
# ravens", but not "near the bandit fort", "with the gnoll champions", "among
# the tavern rats", "north-west of the werewolves".
CUES = {"kill", "kills", "slay", "slays", "defeat", "hunt", "from", "clear", "destroy", "fight", "battle", "cull",
        "exterminate", "attack", "stun", "loot", "collect", "farm", "take"}
STOPS = {"for", "near", "north", "south", "east", "west", "north-east", "north-west", "south-east", "south-west",
         "northeast", "northwest", "southeast", "southwest", "of", "by", "beside", "past", "beyond", "round", "around",
         "among", "amongst", "with", "at", "in", "until", "where", "as", "not", "outside", "inside", "on", "behind",
         "under", "above", "below", "between", "towards", "toward", "to", "into", "while", "when", "who", "that",
         "which", "guarding", "guarded", "led", "leads", "beneath", "across", "along", "through", "than", "like",
         "after", "before", "about", "if", "once", "then", "is", "are", "was", "be", "has", "have", "his", "her",
         "its", "their", "lure", "out", "find", "speak", "talk", "bring", "give", "show", "return", "go", "see",
         "ask", "tell", "visit", "meet", "use", "wait", "watch", "follow", "escort", "save", "free", "rescue", "help"}
DETERMINERS = {"the", "a", "an", "any", "some", "all", "every", "each", "more", "other", "few", "several", "many"}
GENERIC = {"a", "an", "any", "some", "all", "every", "each", "more", "other", "several", "many"}
# a creature's word before one of these is a place or a person, not the creature: "the poacher camp", "the poacher boss"
AFTER = {"camp", "camps", "fort", "fortress", "tower", "towers", "cave", "caves", "village", "hill", "hills", "lair",
         "den", "nest", "nests", "mine", "mines", "citadel", "keep", "outpost", "encampment", "territory", "island",
         "isle", "tomb", "crypt", "temple", "castle", "hideout", "ruins", "field", "fields", "grounds", "boss", "king",
         "queen", "lord", "leader", "chief", "chieftain", "warlord", "captain", "matron", "patriarch", "master",
         "mother", "father", "elder", "shaman", "priest", "pit", "pits", "burrow", "burrows", "hive", "colony",
         "spawn", "spawns", "comes", "appears", "statue", "statues", "skin", "skins", "hide", "hides", "pelt",
         "pelts", "head", "heads", "meat", "fur", "furs", "leg", "legs", "wing", "wings", "eye", "eyes", "tooth",
         "teeth", "fang", "fangs", "claw", "claws", "tail", "tails", "egg", "eggs", "feather", "feathers", "scale",
         "scales", "bone", "bones", "blood", "heart", "hearts", "ear", "ears", "beak", "beaks", "silk", "venom",
         "carapace", "shell", "shells", "horn", "horns", "tusk", "tusks", "gland", "glands", "skull", "skulls"}
NUMBERS = {"one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten", "eleven", "twelve", "twenty"}


def plurals(word):
    """A word's plural forms (the word itself too)."""
    out = {word}
    if word in SAME_PLURAL:
        return out
    if word in IRREGULAR:
        out.add(IRREGULAR[word])
    if word.endswith("man") and len(word) > 3:
        out.add(word[:-3] + "men")
    if re.search(r"(s|x|z|ch|sh|o)$", word):
        out.add(word + "es")
    if re.search(r"[^aeiou]y$", word):
        out.add(word[:-1] + "ies")
    if word.endswith("f"):
        out.add(word[:-1] + "ves")
    if word.endswith("fe"):
        out.add(word[:-2] + "ves")
    out.add(word + "s")
    return out


def forms(name):
    """A creature's name as a line may say it: itself and its last word's
    plurals ("orc pawn", "orc pawns")."""
    words = name.split()
    if not words:
        return set()
    return {" ".join(words[:-1] + [p]) for p in plurals(words[-1])}


def head(name):
    """A name's kind: its last word ("bloodfeather raven" a raven)."""
    w = name.split()
    return w[-1] if w else ""


def tokens(text):
    """A line's words with their case (apostrophes and hyphens kept inside a
    word), and the punctuation that ends a clause as words of its own."""
    t = (text or "").replace("`", "'").replace("’", "'")
    return re.findall(r"[A-Za-z0-9]+(?:['-][A-Za-z0-9]+)*|[,;:.!?()]", t)


class Mention:
    """A creature a line names: the name (or kind) it says, where, as said."""

    def __init__(self, key, kind, start, end, said, plural):
        self.key, self.kind, self.start, self.end, self.said, self.plural = key, kind, start, end, said, plural
        self.source = False  # where the item drops: read back, a verb of killing or "from" first
        self.generic = False  # any of its kind (a plural, "a"/"any"), not one known creature ("the red-eyed rat")
        self.proper = False  # said with a capital inside the line: a name, a place
        # said with a word before it that is no determiner, number or list's
        # ("blood flies", "elite ice giants", "the red-eyed rat"): a kind of its
        # own, not every creature of the name
        self.qualified = False
        self.place = []  # the words after it to the clause's end ("of the unkempt village")


def mentions(text, names, kinds, skip=()):
    """The creatures a line names, longest first and not overlapping: names
    maps a form ("ravens") to a creature's folded name, kinds a form to a kind
    (the last words of creatures' names: "wolf spiders"). A mention inside
    one of the skip phrases (an item's name: "chipped raven beak" names no
    raven) is left out; one that runs past it is kept ("the bloodfeather
    ravens" beside the item "bloodfeather")."""
    tok = tokens(text)
    low = [t.lower() for t in tok]
    inside = [False] * len(tok)
    for ph in skip:
        pw = ph.split()
        n = len(pw)
        for i in range(len(low) - n + 1):
            if n and low[i:i + n] == pw:
                for j in range(i, i + n):
                    inside[j] = True
    longest = max([len(f.split()) for f in names] + [len(f.split()) for f in kinds] + [1])
    out = []
    i = 0
    while i < len(tok):
        hit = None
        for n in range(min(longest, len(tok) - i), 0, -1):
            ph = " ".join(low[i:i + n])
            if ph in names:
                base = re.sub(r"^(the|a|an) ", "", names[ph])
                hit = Mention(names[ph], False, i, i + n, ph, ph != base or base in SAME_PLURAL)
            elif ph in kinds:
                hit = Mention(kinds[ph], True, i, i + n, ph, ph != kinds[ph])
            if hit:
                break
        if hit is None:
            i += 1
            continue
        if all(inside[hit.start:hit.end]):
            i = hit.end
            continue
        hit.proper = any(tok[j][:1].isupper() for j in range(hit.start, hit.end)) and hit.start > 0 and low[hit.start - 1] not in ".!?"
        nxt = low[hit.end] if hit.end < len(low) else ""
        # its own determiner (back to the list's "and", "or" or comma), then
        # back to the verb or the word of place
        j = hit.start - 1
        det = None
        phrase = True
        while j >= 0:
            w = low[j]
            if w in ("and", "or", ","):
                phrase = False
            elif phrase and w in DETERMINERS and det is None:
                det = w
            if w in CUES:
                hit.source = True
                break
            if w in STOPS or w in ".;:!?()":
                break
            j -= 1
        # a name that is a plural itself ("sparkflies") is said as one
        if not hit.plural and re.search(r"[^s]s$", hit.key) and hit.said == hit.key:
            hit.plural = True
        if nxt in AFTER or (hit.start > 0 and low[hit.start - 1] == "not") or (hit.start > 1 and low[hit.start - 2] == "not"):
            hit.source = False
        prev = low[hit.start - 1] if hit.start > 0 else ""
        hit.qualified = bool(prev) and prev not in DETERMINERS | NUMBERS | CUES | STOPS | {"and", "or", ","} \
            and not prev.isdigit() and prev not in ".;:!?()"
        if hit.plural:
            hit.generic = True
        elif not hit.proper and (det in GENERIC or det is None):
            hit.generic = True
        k = hit.end
        while k < len(low) and low[k] not in ".;:!?()":
            hit.place.append(low[k])
            k += 1
        # two creatures' words side by side are one thing ("a lion guard"): neither is meant alone
        if out and out[-1].end == hit.start:
            out[-1].qualified = hit.qualified = True
        out.append(hit)
        i = hit.end
    return out


# ---- the database


class World:
    """What the audit reads, once."""

    def __init__(self, db):
        self.db = db
        self.spawns = collections.defaultdict(list)  # folded name -> [(id, world, x, z, level, attackable)]
        self.spelling = collections.defaultdict(collections.Counter)  # folded name -> its trimmed spellings
        for nid, name, world, x, z, lvl, typ in db.execute(
                "SELECT id, npc_name, world, x_coord, z_coord, npc_level, npc_type FROM npcs WHERE npc_name IS NOT NULL"):
            k = fold(name)
            if not k:
                continue
            self.spawns[k].append((nid, world or 0, x or 0.0, z or 0.0, lvl or 0, not (typ or 0) & 0x80))
            self.spelling[k][" ".join(name.split())] += 1
        # a line's words for creatures: every creature's name (one some spawn of
        # which can be attacked) as said, and every kind (its last words: "wolf
        # spider" of "large wolf spider") as said
        self.names, self.kinds, self.of_kind = {}, {}, collections.defaultdict(set)
        plain = CUES | STOPS | DETERMINERS | AFTER | NUMBERS | {"and", "or"}
        for k, sp in self.spawns.items():
            # a name with a digit is a stand-in or a test's ("RE PC Fire 2")
            if not any(a for *_, a in sp) or len(k) < 3 or re.search(r"\d", k):
                continue
            # "The Lost" is said "the lost thoughts": a leading article is the line's
            bare = re.sub(r"^(the|a|an) ", "", k)
            if len(bare) < 3:
                continue
            for f in forms(bare):
                self.names.setdefault(f, k)
            w = k.split()
            for n in range(1, len(w)):
                kind = " ".join(w[n:])
                if len(kind) < 3 or kind in plain:
                    continue
                self.of_kind[kind].add(k)
                for f in forms(kind):
                    self.kinds.setdefault(f, kind)
        self.items = {pid: name for pid, name in db.execute("SELECT patternid, itemname FROM itempattern")}

    def creatures(self, m, after=()):
        """The creatures (folded names) a mention means: its name, or every
        creature of its kind; of those, the ones a word the line says after it
        names when some do ("the druids and rangers of the Unkempt village":
        the unkempt druids)."""
        if not m.kind:
            return [m.key]
        out = sorted(self.of_kind.get(m.key, ()))
        words = set(after) - set(m.key.split())
        named = [k for k in out if words & set(k.split())]
        return named or out

    def spell(self, k):
        """A folded name as npcs spells it, trimmed: its commonest spelling (a
        lowered one on a tie)."""
        c = self.spelling.get(k)
        if not c:
            return k
        best = max(c.values())
        tied = sorted(s for s, n in c.items() if n == best)
        lower = [s for s in tied if s == s.lower()]
        return (lower or tied)[0]


def parse_items(s):
    out = []
    for f in (s or "").split(","):
        f = f.strip()
        if not f:
            continue
        m = re.match(r"^(-?\d+)\s*(?:[*x×]\s*(\d+))?$", f)
        if m:
            out.append((int(m.group(1)), int(m.group(2) or 1)))
    return out


def parse_events(s):
    try:
        v = json.loads(s) if (s or "").strip() else []
        return v if isinstance(v, list) else []
    except ValueError:
        return []


def load_quests(db):
    qcols = ["questid", "name", "enabled", "giver", "minlevel", "maxlevel", "classes", "events", "requires"]
    quests = {}
    for r in db.execute(f"SELECT {', '.join(qcols)} FROM quests ORDER BY questid"):
        q = dict(zip(qcols, r))
        q["steps"] = []
        quests[q["questid"]] = q
    scols = [c[1] for c in db.execute("PRAGMA table_info(queststeps)")]
    for r in db.execute("SELECT * FROM queststeps ORDER BY questid, step"):
        st = dict(zip(scols, r))
        st["kind"] = (st["kind"] or "").strip().lower()
        st["also"] = []
        if st["questid"] in quests:
            quests[st["questid"]]["steps"].append(st)
    if has_table(db, "queststeptargets"):
        for qid, n, t in db.execute("SELECT questid, step, target FROM queststeptargets ORDER BY rowid"):
            for st in quests.get(qid, {}).get("steps", []):
                if st["step"] == n:
                    st["also"].append(t)
    return quests


def step_names(st):
    """A kill or loot step's creatures, folded: its target, then its others."""
    out = []
    for t in [st["target"]] + list(st["also"]):
        k = fold(t)
        if k and k not in out:
            out.append(k)
    return out


def places(w, q):
    """The quest's places: (world, x, z) of its giver's spawns, of the NPCs its
    talk, show and give steps name, and of its reach steps."""
    out = []
    names = [fold(q["giver"])] + [fold(st["target"]) for st in q["steps"] if st["kind"] in ("talk", "show", "give")]
    for k in names:
        for _, world, x, z, _, _ in w.spawns.get(k, []):
            out.append((world, x, z))
    for st in q["steps"]:
        if st["kind"] == "reach" and (st["x"] or st["z"]):
            out.append((st["world"] or 0, st["x"], st["z"]))
    return out


def nearest(sp, where):
    """The distance from a spawn to the nearest of the places (inf: none in its world)."""
    _, world, x, z, _, _ = sp
    best = math.inf
    for pw, px, pz in where:
        if pw == world:
            best = min(best, math.hypot(x - px, z - pz))
    return best


def spawned_by_events(q, st):
    """The creatures (folded) a spawn event brings for this step: its own start
    events' and the step before's done events', with whether each is hostile."""
    out = {}
    prev = None
    for s in q["steps"]:
        if s["step"] < st["step"] and (prev is None or s["step"] > prev["step"]):
            prev = s
    evs = [e for e in parse_events(st["events"]) if (e.get("on") or "").strip().lower() == "start"]
    if prev is not None:
        evs += [e for e in parse_events(prev["events"]) if (e.get("on") or "").strip().lower() == "done"]
    for e in evs:
        if (e.get("do") or "").strip().lower() == "spawn" and fold(e.get("npc")):
            out[fold(e.get("npc"))] = out.get(fold(e.get("npc")), False) or bool(e.get("hostile"))
    return out


def people(q):
    """The quest's people (folded): its giver and its talk, show and give
    steps' NPCs; a line naming them names no creature to kill."""
    return {fold(q["giver"])} | {fold(st["target"]) for st in q["steps"] if st["kind"] in ("talk", "show", "give")}


def item_phrases(w, st):
    """The step's items' names as a line may say them (folded, plural too)."""
    out = set()
    for pid, _ in parse_items(st["items"]):
        n = fold(w.items.get(pid, ""))
        if n:
            out |= forms(n)
    return out


def near_any(sp, where):
    """Whether a spawn stands within NEAR of one of the places."""
    return nearest(sp, where) <= NEAR


def describe(w, k, where):
    """A name's spawns: anywhere, near the quest, the nearest's distance and
    the levels of those near (or of all when none is near)."""
    sp = w.spawns.get(k, [])
    att = [s for s in sp if s[5]]
    near = [s for s in att if near_any(s, where)]
    d = min((nearest(s, where) for s in att), default=math.inf)
    lv = [s[4] for s in (near or att or sp)]
    return {"name": k, "spawns": len(sp), "near": len(near), "dist": d, "attackable": bool(att),
            "lo": min(lv) if lv else None, "hi": max(lv) if lv else None,
            "low": min((s[4] for s in att), default=None)}


def levels(q):
    """The creature levels that fit a player taking a quest: from FIT_BELOW
    below its lowest to FIT_ABOVE above it, or to its highest when it has one
    below 60 (a level 11-20 quest's creatures may be 19)."""
    lo = q["minlevel"] or 1
    hi = q["maxlevel"] or 60
    return max(1, lo - FIT_BELOW), max(lo + FIT_ABOVE, hi if hi < 60 else 0)


def fitting(w, k, q, where):
    """The spawns of a name a player taking the quest hunts: those that can be
    attacked, near one of the places, of a fitting level."""
    lo, hi = levels(q)
    return [s for s in w.spawns.get(k, []) if s[5] and lo <= s[4] <= hi and near_any(s, where)]


def triggered(journal):
    """Whether a line's creature comes only when others are killed ("until the
    fire imp protector comes"): its drop is that one's."""
    return bool(re.search(r"\buntil\b[^.]*\b(comes?|spawns?|appears?|shows? up|arrives?)\b", (journal or "").lower()))


def audit(w, quests):
    """One row a kill or loot step: what its names match, its flags and the
    fix (or why there is none)."""
    rows = []
    for q in quests.values():
        where = places(w, q)
        lvl = q["minlevel"] or 1
        folks = people(q)
        for st in q["steps"]:
            if st["kind"] not in ("kill", "loot"):
                continue
            names = step_names(st)
            evs = spawned_by_events(q, st)
            descs = [describe(w, k, where) for k in names]
            skip = item_phrases(w, st) | {f for p in folks for f in forms(p)}
            ms = [m for m in mentions(st["journal"], w.names, w.kinds, skip) if m.key not in folks]
            # the step's own creatures' spawns: a creature beside them stands where the step is done
            around = [(s[1], s[2], s[3]) for k in names for s in w.spawns.get(k, []) if s[5]]
            sources = []
            for m in ms:
                if not (m.source and m.generic) or m.qualified:
                    continue
                for k in w.creatures(m, after=m.place):
                    # a name said is looked for near the quest or the step's own creatures; a kind's other creatures
                    # beside the step's own only (the line's "snakes" are those where its snakes are)
                    near = (around or where) if m.kind else where + around
                    if k not in names and k not in sources and k not in folks and fitting(w, k, q, near):
                        sources.append(k)
            flags = []
            by_event = [k for k in names if k in evs]
            if not by_event:
                att = [d for d in descs if d["attackable"]]
                if all(d["spawns"] == 0 for d in descs):
                    flags.append("nomatch")
                elif not att:
                    flags.append("npcbit")
                else:
                    if min(d["dist"] for d in att) > REGION:
                        flags.append("far")
                    if min(d["low"] for d in att) > max(lvl + HIGH_ABOVE, levels(q)[1]):
                        flags.append("high")
                if sources:
                    flags.append("journal")
            one = (sum(d["spawns"] for d in descs) == 1 and not any(m.plural and m.key in names for m in ms)) or \
                any(m.key in names and not m.kind and not m.generic for m in ms)
            rows.append({"q": q, "st": st, "names": names, "descs": descs, "mentions": ms, "sources": sources,
                         "events": evs, "by_event": by_event, "flags": flags, "where": where, "around": around,
                         "named": one, "triggered": triggered(st["journal"])})
    return rows


def decide(w, row):
    """The fix of a flagged row: (the step's new target or None, names to add,
    why), or (None, [], why not) when it cannot be decided."""
    q, st, names = row["q"], row["st"], row["names"]
    lvl = q["minlevel"] or 1
    where = row["where"]
    flags = set(row["flags"]) - {"spawn"}
    if not flags:
        return None, [], ""
    if row["named"]:
        return None, [], "its creature is one known one (a single spawn): its journal's others are its company or its place"
    if row["triggered"]:
        return None, [], "its creature comes only when others are killed: the drop is that one's"
    if not where:
        return None, [], "the quest has no place: its giver and its NPCs stand nowhere"
    gone = all(d["spawns"] == 0 for d in row["descs"])
    if row["sources"]:
        add = list(row["sources"])
        if gone:
            return add[0], add[1:], "named by its journal as where the item drops, near the quest at fitting levels"
        return None, add, "named by its journal as where the item drops, near the quest at fitting levels"
    if lvl > LOW_QUEST:
        return None, [], f"a level {lvl} quest: its creature's place and level may be the guide's own"
    if any(m.key in names for m in row["mentions"]):
        return None, [], "its journal names its own creature: where that stands and its level are the guide's"
    if flags & {"nomatch", "high", "far"}:
        # the nearest fitting creatures of the kind its names and its journal's sources are
        kinds = {head(k) for k in names} | {head(m.key) for m in row["mentions"] if m.source and m.generic}
        cands = []
        for kind in kinds:
            for k in set(w.of_kind.get(kind, ())) | ({kind} if kind in w.spawns else set()):
                if k in names:
                    continue
                lo, hi = levels(q)
                sp = [s for s in w.spawns[k] if s[5] and lo <= s[4] <= hi]
                d = min((nearest(s, where) for s in sp), default=math.inf)
                if d <= FAR_KIND:
                    cands.append((d, k))
        cands.sort()
        lo, hi = levels(q)
        if not cands:
            return None, [], f"no creature of its kind ({', '.join(sorted(kinds))}) within {FAR_KIND} of the quest at levels {lo}-{hi}"
        best = cands[0][0]
        add = [k for d, k in cands if d <= max(best * 1.5, NEAR)]
        why = f"the nearest of its kind at levels {lo}-{hi} ({round(best)} off)"
        if gone:
            return add[0], add[1:], why
        return None, add, why
    return None, [], "nothing near the quest at fitting levels is named for it"


# ---- what else a quest needs: the items its give and show steps want


def item_sources(db, w, quests):
    """Every item something provides: {patternid: [what]}."""
    src = collections.defaultdict(set)
    if has_table(db, "npcitem"):
        for (pid,) in db.execute("SELECT DISTINCT patternid FROM npcitem"):
            src[pid].add("merchant")
    if has_table(db, "loottables") and has_table(db, "npcloot"):
        bound = {r[0] for r in db.execute("SELECT DISTINCT tablename FROM npcloot")}
        for pid, t in db.execute("SELECT patternid, tablename FROM loottables WHERE weight > 0"):
            if t in bound:
                src[pid].add("loot table")
    for t, col in (("startitems", "patternid"), ("fishcatch", "itemid"), ("craftrecipes", "product")):
        if has_table(db, t):
            for (pid,) in db.execute(f"SELECT DISTINCT {col} FROM {t}"):
                src[pid].add(t)
    for q in quests.values():
        for e in parse_events(q["events"]):
            if (e.get("do") or "").strip().lower() == "give":
                for pid, _ in parse_items(e.get("items")):
                    src[pid].add(f"quest {q['questid']}")
        for st in q["steps"]:
            for pid, _ in parse_items(st["rewarditems"]):
                src[pid].add(f"quest {q['questid']}")
            if st["kind"] == "loot":
                for pid, _ in parse_items(st["items"])[:1]:
                    src[pid].add(f"quest {q['questid']}")
            for e in parse_events(st["events"]):
                if (e.get("do") or "").strip().lower() == "give":
                    for pid, _ in parse_items(e.get("items")):
                        src[pid].add(f"quest {q['questid']}")
            try:
                by = json.loads(st.get("byclass") or "{}") or {}
                for v in by.values():
                    for pid, _ in parse_items(v.get("items")):
                        src[pid].add(f"quest {q['questid']}")
            except (ValueError, AttributeError):
                pass
            try:
                ch = json.loads(st.get("choices") or "{}") or {}
                for o in ch.get("options") or []:
                    for pid, _ in parse_items(o.get("items")):
                        src[pid].add(f"quest {q['questid']}")
            except (ValueError, AttributeError):
                pass
    return src


def unprovided(db, w, quests):
    """Give and show steps' items nothing provides: [(quest, step, patternid, name, what else)]."""
    src = item_sources(db, w, quests)
    out = []
    for q in quests.values():
        for st in q["steps"]:
            if st["kind"] not in ("give", "show"):
                continue
            for pid, n in parse_items(st["items"]):
                s = src.get(pid, set())
                own = f"quest {q['questid']}"
                if not s:
                    out.append((q, st, pid, w.items.get(pid, f"item {pid}"), "nothing"))
                elif s == {own}:
                    # only its own quest: taking it, or a step before this one, must give it
                    got = []
                    for e in parse_events(q["events"]):
                        if (e.get("do") or "").strip().lower() == "give" and (e.get("on") or "").strip().lower() == "accept":
                            got += [p for p, _ in parse_items(e.get("items"))]
                    for b in q["steps"]:
                        if b["step"] >= st["step"]:
                            continue
                        got += [p for p, _ in parse_items(b["rewarditems"])]
                        if b["kind"] == "loot":
                            got += [p for p, _ in parse_items(b["items"])[:1]]
                        for e in parse_events(b["events"]):
                            if (e.get("do") or "").strip().lower() == "give":
                                got += [p for p, _ in parse_items(e.get("items"))]
                    if pid not in got:
                        out.append((q, st, pid, w.items.get(pid, f"item {pid}"), "only a later step of its own quest"))
    return out


# The items quests want that nothing provides, given as the guides give them:
# each found by its quest's import key (quest_import_quests) and written only
# while the item is still provided by nothing.
ITEM_FIXES = [
    {"key": "Misc/zentars-keep-pet/p1", "accept_give": ["A Useless Sword", "Note from Sundro"],
     "why": "its offer hands them over (\"Take this useless one and my note to him\"), and nothing gave them: "
            "its first step could never be done"},
    {"key": "Portal/zaoran-plateau/growth", "loot_before": "bitter roots", "count": 3, "chance": 0.5,
     "target": "orc sapper", "journal": "Take three bitter roots from the orc sappers outside Freeport.",
     "why": "EQOA-Data's guide: \"3 bitter herbs or roots (from white con orcs in FP)\", and nothing dropped "
            "or sold bitter roots; the orc sappers (48) are the highest orcs outside Freeport"},
]


def item_id(db, name):
    """The one item of that name (any case), or None."""
    ids = [r[0] for r in db.execute("SELECT patternid FROM itempattern WHERE LOWER(TRIM(itemname)) = LOWER(TRIM(?))", (name,))]
    return ids[0] if len(ids) == 1 else None


def item_fixes(db, quests, gaps):
    """The ITEM_FIXES that apply here: [(fix, quest, what)], and [(fix, why
    not)] for the rest."""
    keyed = {}
    if has_table(db, "quest_import_quests"):
        keyed = {k: qid for k, qid in db.execute("SELECT key, questid FROM quest_import_quests")}
    wanting = {(q["questid"], pid) for q, st, pid, name, what in gaps}
    out, left = [], []
    for fx in ITEM_FIXES:
        q = quests.get(keyed.get(fx["key"]))
        if q is None:
            left.append((fx, "no quest of that key here"))
            continue
        if "accept_give" in fx:
            ids = [item_id(db, n) for n in fx["accept_give"]]
            if None in ids:
                left.append((fx, "an item of its is not here, or not one of the name"))
                continue
            need = [pid for pid in ids if (q["questid"], pid) in wanting]
            if not need:
                left.append((fx, "something provides its items now"))
                continue
            evs = parse_events(q["events"])
            evs.append({"on": "accept", "do": "give", "items": ", ".join(str(pid) for pid in need)})
            out.append((fx, q, {"events": json.dumps(evs, separators=(",", ":"))}))
        else:
            pid = item_id(db, fx["loot_before"])
            if pid is None or (q["questid"], pid) not in wanting:
                left.append((fx, "its item is not here, or something provides it now"))
                continue
            at = next((st["step"] for st in q["steps"] if st["kind"] in ("give", "show")
                       and pid in [p for p, _ in parse_items(st["items"])]), None)
            if at is None:
                left.append((fx, "no step of its wants the item"))
                continue
            out.append((fx, q, {"step": at, "kind": "loot", "target": fx["target"], "items": f"{pid}*{fx['count']}",
                                "count": fx["count"], "chance": fx["chance"], "journal": fx["journal"]}))
    return out, left


def shift_steps(db, qid, frm, by):
    """Moves a quest's steps from frm on by one (by +1 or -1), with their other
    creatures and the characters at them; one at a time, in the order that
    keeps the keys apart."""
    steps = [n for (n,) in db.execute("SELECT step FROM queststeps WHERE questid = ? AND step >= ?", (qid, frm))]
    for n in sorted(steps, reverse=by > 0):
        db.execute("UPDATE queststeps SET step = ? WHERE questid = ? AND step = ?", (n + by, qid, n))
        if has_table(db, "queststeptargets"):
            db.execute("UPDATE queststeptargets SET step = ? WHERE questid = ? AND step = ?", (n + by, qid, n))
        if has_table(db, "charquests"):
            db.execute("UPDATE charquests SET step = ? WHERE questid = ? AND step = ?", (n + by, qid, n))


# ---- writing


def plan(db):
    """The audit and what --apply writes: (world, quests, rows, changes,
    undecided, gaps, item fixes). A flagged step left as it is goes to
    undecided when the user should look at it: a quest up to LOW_QUEST, or a
    step whose creatures stand nowhere or can never be attacked."""
    w = World(db)
    quests = load_quests(db)
    rows = audit(w, quests)
    changes, undecided = [], []
    for r in rows:
        if not r["flags"]:
            continue
        target, add, why = decide(w, r)
        r["fix"] = (target, add, why)
        if target is None and not add:
            if (r["q"]["minlevel"] or 1) <= LOW_QUEST or set(r["flags"]) & {"nomatch", "npcbit"}:
                undecided.append((r, why))
            continue
        changes.append((r, target, add, why))
    gaps = unprovided(db, w, quests)
    fixes, left = item_fixes(db, quests, gaps)
    return w, quests, rows, changes, undecided, gaps, fixes, left


def fmt_desc(d):
    if d["spawns"] == 0:
        return f"{d['name']}: none"
    dist = "other world" if d["dist"] == math.inf else f"{round(d['dist'])}"
    return (f"{d['name']}: {d['spawns']} ({d['near']} near), nearest {dist}, L{d['lo']}-{d['hi']}"
            + ("" if d["attackable"] else ", NPC bit"))


def write_tsv(path, w, rows):
    cols = ["questid", "quest", "enabled", "giver", "minlevel", "step", "kind", "target", "others", "count", "chance",
            "flags", "names", "journal_names", "journal_sources", "one_creature", "by_event", "fix", "journal"]
    with open(path, "w") as f:
        f.write("\t".join(cols) + "\n")
        for r in rows:
            q, st = r["q"], r["st"]
            fix = r.get("fix")
            fixs = ""
            if fix:
                target, add, why = fix
                if target or add:
                    fixs = (f"target {w.spell(target)}; " if target else "") + ("add " + ", ".join(w.spell(k) for k in add) + "; " if add else "") + why
                else:
                    fixs = "undecided: " + why
            vals = [q["questid"], q["name"], q["enabled"], q["giver"], q["minlevel"], st["step"], st["kind"], st["target"],
                    ", ".join(st["also"]), st["count"], st["chance"], ",".join(r["flags"]),
                    " | ".join(fmt_desc(d) for d in r["descs"]),
                    ", ".join(m.said + ("" if m.source and m.generic else " (" + ("not where it drops" if not m.source else "one") + ")")
                              for m in r["mentions"]),
                    ", ".join(r["sources"]), "yes" if r["named"] else "", ", ".join(r["by_event"]), fixs,
                    st["journal"]]
            f.write("\t".join(str(v).replace("\t", " ").replace("\n", " ") for v in vals) + "\n")


def apply(db, w, changes, fixes=()):
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_DDL)
        db.execute(TARGETS_DDL)
        n = collections.Counter()
        for fx, q, what in fixes:
            if "events" in what:
                db.execute("UPDATE quests SET events = ? WHERE questid = ?", (what["events"], q["questid"]))
                db.execute("INSERT INTO quest_targets_fix_log (questid, step, action, old, new, why, applied) VALUES (?, 0, 'events', ?, ?, ?, ?)",
                           (q["questid"], q["events"] or "", what["events"], fx["why"], when))
                n["events"] += 1
            else:
                shift_steps(db, q["questid"], what["step"], 1)
                row = {c: what[c] for c in ("kind", "target", "items", "count", "chance", "journal")}
                db.execute(f"INSERT INTO queststeps (questid, step, {', '.join(row)}) VALUES (?, ?, {', '.join('?' * len(row))})",
                           [q["questid"], what["step"], *row.values()])
                db.execute("INSERT INTO quest_targets_fix_log (questid, step, action, old, new, why, applied) VALUES (?, ?, 'step', '', ?, ?, ?)",
                           (q["questid"], what["step"], json.dumps(row, sort_keys=True), fx["why"], when))
                n["steps"] += 1
        for r, target, add, why in changes:
            q, st = r["q"], r["st"]
            if target is not None:
                new = w.spell(target)
                db.execute("UPDATE queststeps SET target = ? WHERE questid = ? AND step = ?", (new, q["questid"], st["step"]))
                db.execute("INSERT INTO quest_targets_fix_log (questid, step, action, old, new, why, applied) VALUES (?, ?, 'target', ?, ?, ?, ?)",
                           (q["questid"], st["step"], st["target"], new, why, when))
                n["targets"] += 1
            for k in add:
                new = w.spell(k)
                cur = db.execute("INSERT OR IGNORE INTO queststeptargets (questid, step, target) VALUES (?, ?, ?)",
                                 (q["questid"], st["step"], new))
                if cur.rowcount:
                    db.execute("INSERT INTO quest_targets_fix_log (questid, step, action, old, new, why, applied) VALUES (?, ?, 'added', '', ?, ?, ?)",
                               (q["questid"], st["step"], new, why, when))
                    n["added"] += 1
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return n


def undo(db):
    if not has_table(db, "quest_targets_fix_log"):
        print("nothing logged: nothing to undo")
        return
    db.execute("BEGIN IMMEDIATE")
    try:
        back = kept = 0
        for seq, qid, n, action, old, new in db.execute(
                "SELECT seq, questid, step, action, old, new FROM quest_targets_fix_log ORDER BY seq DESC").fetchall():
            if action == "added":
                rows = [t for (t,) in db.execute("SELECT target FROM queststeptargets WHERE questid = ? AND step = ?", (qid, n))
                        if fold(t) == fold(new)]
                if rows:
                    for t in rows:
                        db.execute("DELETE FROM queststeptargets WHERE questid = ? AND step = ? AND target = ?", (qid, n, t))
                    back += 1
                else:
                    kept += 1
            elif action == "target":
                cur = db.execute("SELECT target FROM queststeps WHERE questid = ? AND step = ?", (qid, n)).fetchone()
                if cur is not None and fold(cur[0]) == fold(new):
                    db.execute("UPDATE queststeps SET target = ? WHERE questid = ? AND step = ?", (old, qid, n))
                    back += 1
                else:
                    kept += 1
            elif action == "events":
                cur = db.execute("SELECT events FROM quests WHERE questid = ?", (qid,)).fetchone()
                if cur is not None and cur[0] == new:
                    db.execute("UPDATE quests SET events = ? WHERE questid = ?", (old, qid))
                    back += 1
                else:
                    kept += 1
            elif action == "step":
                row = json.loads(new)
                cols = list(row)
                cur = db.execute(f"SELECT {', '.join(cols)} FROM queststeps WHERE questid = ? AND step = ?", (qid, n)).fetchone()
                if cur is not None and fold(cur[cols.index("target")]) == fold(row["target"]) and \
                        all(cur[i] == row[c] for i, c in enumerate(cols) if c != "target"):
                    db.execute("DELETE FROM queststeps WHERE questid = ? AND step = ?", (qid, n))
                    if has_table(db, "queststeptargets"):
                        db.execute("DELETE FROM queststeptargets WHERE questid = ? AND step = ?", (qid, n))
                    shift_steps(db, qid, n + 1, -1)
                    back += 1
                else:
                    kept += 1
        # queststeptargets stays, empty or not: the server makes it and reads it (a GM's rows may be in it)
        db.execute("DROP TABLE quest_targets_fix_log")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"took back {back} changes; {kept} changed since left as they are")


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-quest-targets"
    shutil.copy2(path, dst)
    return dst


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--tsv", help="write the audit, a row a kill or loot step, here")
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    ap.add_argument("--verbose", action="store_true", help="every flagged step, fixed or not")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the fixes (logged)")
    g.add_argument("--undo", action="store_true", help="take back what --apply wrote")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        if not a.no_backup and has_table(db, "quest_targets_fix_log"):
            print(f"copy: {backup(a.db)}")
        undo(db)
        return
    if a.apply and has_table(db, "quest_targets_fix_log"):
        sys.exit("quest_targets_fix_log is there: applied already (--undo first)")
    w, quests, rows, changes, undecided, gaps, fixes, left = plan(db)
    flags = collections.Counter(f for r in rows for f in r["flags"])
    on = collections.Counter(f for r in rows if r["q"]["enabled"] for f in r["flags"])
    low = collections.Counter(f for r in rows if (r["q"]["minlevel"] or 1) <= LOW_QUEST for f in r["flags"])
    print(f"kill and loot steps: {len(rows)} ({sum(1 for r in rows if r['q']['enabled'])} in quests in the game); "
          f"flagged {sum(1 for r in rows if r['flags'])}")
    for f, n in flags.most_common():
        print(f"  {f}: {n} ({on[f]} in the game, {low[f]} of quests up to level {LOW_QUEST})")
    for r, target, add, why in changes:
        q, st = r["q"], r["st"]
        what = []
        if target:
            what.append(f"target {st['target']!r} -> {w.spell(target)!r}")
        if add:
            what.append("add " + ", ".join(repr(w.spell(k)) for k in add))
        print(f"{q['questid']} step {st['step']} ({st['kind']} {st['target']!r}; {','.join(r['flags'])}): {'; '.join(what)} ({why})")
    for r, why in undecided:
        q, st = r["q"], r["st"]
        print(f"undecided: {q['questid']} step {st['step']} ({st['kind']} {st['target']!r}; {','.join(r['flags'])}) "
              f"{'in the game' if q['enabled'] else 'a draft'}: {why}; journal {st['journal']!r}")
    for q, st, pid, name, what in gaps:
        print(f"item nothing provides: {q['questid']} step {st['step']} ({st['kind']} at {st['target']!r}) wants {name!r} ({pid}): {what}")
    for fx, q, what in fixes:
        if "events" in what:
            print(f"{q['questid']} ({q['name']}): given on taking it: {', '.join(fx['accept_give'])} ({fx['why']})")
        else:
            print(f"{q['questid']} ({q['name']}): a loot step before step {what['step']}: {what['journal']!r} "
                  f"(off {what['target']}, {what['count']} at {what['chance']} a kill; {fx['why']})")
    for fx, why in left:
        print(f"item fix {fx['key']} left: {why}")
    print(f"{len(changes)} steps to fix ({sum(1 for c in changes if c[1])} targets replaced, "
          f"{sum(len(c[2]) for c in changes)} creatures added); {len(undecided)} undecided; "
          f"{len(gaps)} items wanted that nothing provides, {len(fixes)} quests to give them")
    if a.tsv:
        write_tsv(a.tsv, w, rows)
        print(f"audit: {a.tsv}")
    if not a.apply:
        print("report only: --apply to write")
        return
    if not changes and not fixes:
        return
    if not a.no_backup:
        print(f"copy: {backup(a.db)}")
    n = apply(db, w, changes, fixes)
    print(f"wrote {n['added']} creatures, {n['targets']} targets replaced, {n['events']} quests' events, {n['steps']} steps")


if __name__ == "__main__":
    main()
