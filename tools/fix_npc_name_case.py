#!/usr/bin/env python3
"""NPC names in one casing (the user, 2026-10-29: "our mob names are mixed
between upper/lower case. lets make lower case for generic names like moss
snake, etc. but if title like Guard Rick etc."): a generic name is lowercase
but for its proper words ("moss snake", "Deathfist reaver", "Tae Ew crusader",
"child of Xrag"), an individual's keeps its capitals ("Guard Rick", "Captain
Scilond", "Lady Vox"), and every name loses its stray spaces ("Copperhead " is
"copperhead", "Guard  Bajie" is "Guard Bajie"). Every column holding an NPC's
name is renamed with it.

**The record.**
- SOE's own casing is what Return Home's import (prh_import_npcs, recorded off
  the live game) and two retail savestates show: in the EE memory of Corsten's
  (Freeport) and Lostologist's (near Qeynos) the creatures are "kappa drudge",
  "phantom", "elder spirit", "spirit", an object "sign post", and the towns'
  people and the pets "Angry Patron", "Recruiter", "Spectral Servant",
  "Mummified Guardian" (SOE_RETAIL, which wins over all below). Return Home's
  names are SOE's in kind: 12,431 of its 19,877 rows lowercase, a proper word
  capitalized inside them ("Tae Ew crusader").
- Sandstorm's recordings are no evidence of casing: of the 2,103 NPC names the
  harvester saw there (eqoa.live, --harvests) 2,094 are capitalized throughout
  ("Grass Viper"; "Sign Post" where the retail memory has "sign post"), and
  FrontiersForge's journal (npcs_ff_import), read off Sandstorm, is too. Their
  spellings count only where they are lowercase; --out lists every name whose
  Sandstorm spelling differs from the one chosen.
- So a name spelled with a lowercase word anywhere (our rows, Return Home's,
  the retail memory) takes that spelling: "Grass Viper" (26 rows) becomes
  "grass viper" as Return Home's 84 are. Of two such spellings the one whose
  words agree with their classes (below) wins ("Deathfist pawn", not "deathfist
  pawn"), then the one whose proper words most names spell so ("Teir'Dal").
- A name capitalized throughout everywhere is read by its words. A word is
  proper when the names that mix cases capitalize it more often than any name
  writes it lowercase ("Deathfist" 30 to 2, "Freezeblood" 29 to 8); common when
  a mixed name writes it lowercase, two all-lowercase names do ("hatebone",
  "kerran"), it is a creature's or a role's noun, or the dictionary (--dict) has
  it lowercase; else proper (a single all-lowercase name, "paineel recovery
  agent", says little of a word in it). A creature's or a role's noun is the
  head of a name written lowercase, or one of the server's own lists
  (tools/seed_npc_move.py's animals, creatures and people). The head is the word
  before " of ", else the last.
- The name is generic when its head is such a noun and every proper word in it
  names a kind: one a mixed name capitalizes ("Tae", "Freezeblood"), a
  possessive ("Goran's Beast"), or a word put before two creatures' nouns or
  more ("Solusek Champion", "Solusek Evoker"). It is then lowercased but for its
  proper words ("Solusek champion"). A name spawned three times or more is a
  kind whatever its words (KIND_ROWS: "Partisan of Paineel" 16 rows, "partisan
  of Paineel"), unless a title starts it.
- An individual's name keeps its case: "The ..." ("The Storyteller"), "<proper>
  the <epithet>" ("Sarek the Betrayer"), a proper head ("Lord Nagafen",
  "Training Dummy Lv50", "Frostborn II"), a title before a name ("Guard Rick",
  "Pole Merchant Longhand": a title is one or two words seen before a proper
  name, and more often so than before a creature's noun), and a proper word
  before a head that is no creature's or role's noun ("Amanda Johns").

**Left as they are, for the user's word** (the ambiguous list): a summoned
pet's name (spellpattern's summoning spells, seed_npc_aggro.py's pets and an
enchanter's "Animation"s: the retail memory spells pets capitalized, and a pet
is named as its spell anyway); a generic name all of whose rows are people of
a players' race, peaceful ones or fewer than three ("Angry Patron" is SOE's so,
"Old Man"); a portal or an announcer, a named place; a head that is no
creature's or role's noun on this server ("Frostbite", "Fangs", "John"); a
proper word naming no kind before a creature's noun ("Ingrid Adder"); a title
before a creature's noun ("Elder Elephant"); and a name spelled lowercase
elsewhere that a title says is someone's. A name the server's code keys as
written (handlers/waysender.go's routes, entity/world.go's warp labels and
#tele places: CODE_KEYED) or whose capitals the code reads (entity/dialog.go's
guard titles, the "Spiritmaster ", "Waysender ", "Coachman " prefixes: a rename
may not change them) keeps its case too.

**Every reference.** The renamed names are written in npcs and in what keys on
them by exact name: factionhits and npcsocial (entity/faction.go and
socialaggro.go look them up as spelled), charbind's spiritmaster, the quests'
giver, step targets, trips and spawn events and the GM triggers' steps (folded
by the server, but said in chat as written). The tables keyed by the folded
name (npcclass, npcundead, npcmove, npcloot, npcfaction, npctemplate,
npcaggro, npcdialog) change only where a name's folded key does, a doubled
space inside it. Two rows that become one (factionhits' "Breath Stealer" and
"breath stealer" on one faction) are merged when they agree, else nothing is
written. The other tools' undo logs follow their rows (quest_hit_seed,
npcpaths_seed's made NPCs, pod_boss_fix_log's NPCs, npcfaction_seed,
npcclass_fix_log), and import_quests.py's digests are made again for the
quests that matched theirs, so a re-import still tells a GM's change.

Every value changed is logged in npc_name_case_log; --undo puts back each one
still as written (a merged row only while the row it went into is there); a
second --apply is refused while the log is there. Report only by default; --out
writes the change list and the ambiguous list (TSV). --names applies a change
list written so, exactly, instead of reading the names again. The running
server reads NPC names at its start: restart it after --apply or --undo.

    tools/fix_npc_name_case.py [--db eqoa.db] [--harvests <dir or file> ...] [--dict FILE]
        [--out DIR] [--names FILE] [--apply | --undo]
"""
import argparse
import collections
import datetime
import json
import os
import re
import shutil
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import seed_npc_move as move  # noqa: E402  (the server's lists of animals, creatures and people)

KIND_NOUNS = frozenset(move.NOT_ANIMALS | move.PEOPLE)

DEFAULT_DB = os.path.join(HERE, "..", "eqoa.db")
DEFAULT_DICT = "/usr/share/dict/american-english"
LOG = "npc_name_case_log"
SANDSTORM = {"eqoa.live", "5.161.217.188"}

# Names in the EE memory of two retail savestates, SOE's servers' own
# (savestates/Corsten_Final.zip in Freeport, Lostologist_Final.zip near Qeynos):
# every NUL-ended string there that is an NPC name of this server, as spelled.
SOE_RETAIL = (
    "Aeric Sparelli", "Aloj Tilsteran", "Angry Patron", "Blixin Kaboom", "Bowyer Koll", "Captain Scilond",
    "Coachman Ronks", "Dockmaster Yarnning", "Dr. Killian", "Eridus", "Freeport Announcements", "Grimefod Philch",
    "Guard Jahn", "Guard Perinen", "Guard Serenda", "Guard Telan", "Guard Telsto", "Harbormaster Thand",
    "Iulias Maphesti", "Jack Holson", "Marona Jofranka", "Merchant Ahkham", "Merchant Kari", "Mummified Guardian",
    "Nascien Vladya", "Pa'Trysha Oz'Baern", "Piker Yeovalech", "Recruiter", "Renee Hallsong", "Rianba Oschba",
    "Royce Tilsteran", "Setham", "sign post", "Smithy Polinae", "Tailor Bariston", "Tailor Zixar", "Tchuy",
    "Tresca Goodmorrow",
    "elder spirit", "kappa drudge", "phantom", "Spectral Servant", "spirit",
)

# Names the server's code keys exactly, as written there: a rename would lose
# what the code gives them (fix_npc_name_case_test.py reads the maps out of
# the Go files and fails on one missing here).
CODE_KEYED = (
    # handlers/waysender.go wayRoutes
    "Waysender Zapgit", "Waysender Glowbap", "Waysender Windfoot", "Waysender Carowyn", "Waysender Naelarie",
    "Waysender Brindel", "Necrologist Zizclank", "Portal to the Dead Hills", "Whirling Portal",
    "Portal to the Lake", "Bloody Portal", "Portal of the Twins", "Fuming Portal", "Portal to the Cauldron",
    "Sky Portal", "Norach", "portal guardian", "Subdued Magus", "essence of rain", "essence of sun",
    "essence of night", "Mark of the Majestic",
    # handlers/chat.go teleDests (entity/world.go SpawnOfNPC)
    "Coachman Ronks", "Archivist Vaal", "Harbormaster Machado",
    # entity/world.go warpLocationNames
    "Harbormaster Thand",
)

FUNCTION_WORDS = frozenset("of the a an and to in on at for from with by".split())
ALPHA = re.compile(r"[A-Za-z]")
ROMAN = re.compile(r"[IVXLC]+")
# heads naming a place or a thing for a place: kept as they are written
PLACE_HEADS = frozenset(["portal", "announcements"])
# an enchanter's pets ("Lesser Animation", "Pendril's Animation"): named as their spells
PET_HEADS = frozenset(["animation"])
# a name spawned this many times is a kind, whoever its words seem to name
KIND_ROWS = 3
# the rule's reasons that outweigh a lowercase spelling elsewhere: a title
# before a name, "The ...", "<proper> the ..." (a proper head alone does not:
# its word may be lowercase only in that very spelling, "caiman")
STRONG = ("the title", "starts with The", "a proper name before 'the'")
# what the server's code reads off a name's capitals: entity/dialog.go's
# guardTitle (a guard of no town speaks only so named), and the roles a
# name's first word gives (handlers/spiritmaster.go, waysender.go,
# entity/world.go's coaches and warps)
GUARD_TITLE = re.compile(r"^(?:Guard|Guardsman|Deputy|Captain|Sergeant|Lieutenant|Sentry|Sentinel|Watchman|"
                         r"Warden|Marshal|Marshall|Commander|Corporal|Trooper|Footman|Protector|Defender|Soldier)\s+[A-Z]")
ROLE_PREFIXES = ("Spiritmaster ", "Waysender ", "Coachman ", "Coachwoman ", "Gatekeeper ", "Harbormaster ")

LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,     -- the table written
    rid    INTEGER NOT NULL,  -- the row's rowid
    col    TEXT NOT NULL,     -- the column changed; '*' a row merged into another and deleted
    old    TEXT,              -- the value before ('*': the row, JSON)
    new    TEXT,              -- the value written ('*': the rowid of the row it went into)
    why    TEXT NOT NULL DEFAULT ''
)"""


# ---- names and words


def clean(s):
    """A name with its stray spaces gone: trimmed, a run of spaces one."""
    return " ".join((s or "").split())


def fold(s):
    return clean(s).lower()


def class_key(s):
    """database.NPCClassKey: how npcclass and the tables like it key a name."""
    n = (s or "").strip().lower()
    return n[:-1].strip() if n.endswith("$") else n


def is_lower(w):
    return bool(ALPHA.search(w)) and w == w.lower()


def is_cap(w):
    return w[:1].isupper()


def content_words(ws):
    return [w for w in ws if ALPHA.search(w) and w.lower() not in FUNCTION_WORDS]


def shape(name):
    """'lower' every word lowercase, 'mixed' some, 'title' none (function
    words aside)."""
    ws = content_words(name.split(" "))
    low = [w for w in ws if is_lower(w)]
    if low and len(low) < len(ws):
        return "mixed"
    return "lower" if low else "title"


def head_index(ws):
    low = [w.lower() for w in ws]
    if "of" in low[1:]:
        return max(low.index("of", 1) - 1, 0)
    return len(ws) - 1


def code_reads_case(name):
    """What the server's code reads off a name's capitals (GUARD_TITLE,
    ROLE_PREFIXES): a rename must not change it."""
    return bool(GUARD_TITLE.match(name)), tuple(name.startswith(p) for p in ROLE_PREFIXES)


def lowered(name, proper):
    return " ".join(w if proper(w) else w.lower() for w in name.split(" "))


# ---- the record


class Evidence:
    """What says how a name is cased: every spelling of it (our rows, Return
    Home's, Sandstorm's, the retail memory), the words those spellings case,
    the dictionary's words and the pets' names."""

    def __init__(self, sources, dict_words=(), pets=(), soe=SOE_RETAIL):
        self.sources = {}
        for k, v in sources.items():
            names = collections.Counter()
            for n, c in v.items():
                n = clean(n).rstrip("$").rstrip()  # Return Home marks some names with a '$'
                if n:
                    names[n] += c
            self.sources[k] = names
        self.soe = {fold(n): clean(n) for n in soe}
        self.sources["soe"] = collections.Counter({clean(n): 1 for n in soe})
        self.dict_words = frozenset(dict_words)
        self._proper, self._noun = {}, {}  # each word's class, worked out once
        self.pets = frozenset(fold(n) for n in pets)
        self.variants = collections.defaultdict(collections.Counter)
        for src, names in self.sources.items():
            for n, c in names.items():
                self.variants[n.lower()][n] += c
        # a word's casing: capitalized inside a mixed name, lowercase inside
        # one, lowercase in a name all lowercase (the weakest: a whole name
        # typed lowercase says little of a word in it)
        self.cap_mixed, self.low_mixed, self.low_all = collections.Counter(), collections.Counter(), collections.Counter()
        corpus = {n for names in self.sources.values() for n in names}
        for n in corpus:
            sh = shape(n)
            for w in content_words(n.split(" ")):
                if sh == "mixed" and is_cap(w):
                    self.cap_mixed[w.lower()] += 1
                elif sh == "mixed" and is_lower(w):
                    self.low_mixed[w] += 1
                elif sh == "lower":
                    self.low_all[w] += 1
        # how often each spelling of a word is written, over every spelling of every name
        self.word_forms = collections.Counter()
        for names in self.sources.values():
            for n, c in names.items():
                for w in n.split(" "):
                    self.word_forms[w] += c
        # the nouns a name written lowercase has as its head
        self.headlow = collections.Counter()
        for n in corpus:
            if shape(n) != "title":
                ws = n.split(" ")
                h = ws[head_index(ws)]
                if is_lower(h):
                    self.headlow[h] += 1
        # titles: one or two capitalized words before a proper name, more often
        # so than before a creature's noun ("Guard Jahn", "Pole Merchant
        # Alterheim", not "Froglok Knight"); a kind's word is no title
        # ("Freezeblood Hoarder")
        before_proper, before_noun = collections.Counter(), collections.Counter()
        kinds = collections.defaultdict(set)  # a proper word -> the creatures' nouns it is put before
        for n in corpus:
            ws = n.split(" ")
            if len(ws) in (2, 3) and all(is_cap(w) for w in ws):
                t = " ".join(ws[:-1]).lower()
                if self.proper(ws[-1]):
                    before_proper[t] += 1
                elif self.creature_noun(ws[-1]):
                    before_noun[t] += 1
            hi = head_index(ws)
            if hi >= 1 and self.creature_noun(ws[hi]):
                for w in ws[:hi]:
                    if is_cap(w) and self.proper(w):
                        kinds[w.lower()].add(ws[hi].lower())
        self.kind_words = frozenset(w for w, heads in kinds.items() if len(heads) >= 2)
        self.titles = frozenset(t for t, c in before_proper.items()
                                if c > before_noun[t] and not any(self.kind_word(w) for w in t.split(" ")))

    def proper(self, w):
        if w not in self._proper:
            self._proper[w] = self._proper_word(w)
        return self._proper[w]

    def _proper_word(self, w):
        k = w.lower()
        if k in FUNCTION_WORDS:
            return False
        if not ALPHA.search(k) or ROMAN.fullmatch(w):
            return True
        c, lm, la = self.cap_mixed[k], self.low_mixed[k], self.low_all[k]
        if c or lm:
            return c > lm + la
        if la >= 2 or self.creature_noun(k):
            return False
        return k not in self.dict_words

    def creature_noun(self, w):
        """A noun a creature or a role is named by: the head of a name written
        lowercase, or one of the server's own lists of animals, creatures and
        people (tools/seed_npc_move.py's, entity/wildlife.go's)."""
        k = w.lower()
        if k not in self._noun:
            self._noun[k] = self.headlow[k] > 0 or move.wildlife_noun(k) or move.in_set(k, KIND_NOUNS)
        return self._noun[k]

    def kind_word(self, w):
        """A proper word that names a kind, not a person: capitalized in a name
        that mixes cases ("Deathfist reaver"), a possessive ("Goran's Beast"),
        or put before two creatures' nouns or more ("Solusek Champion",
        "Solusek Evoker")."""
        k = w.lower()
        return self.cap_mixed[k] > 0 or k.endswith(("'s", "'")) or k in self.kind_words

    def title_of(self, ws):
        """The title a name starts with (one word or two, then a capitalized
        word), or None."""
        for n in (2, 1):
            if len(ws) > n and " ".join(ws[:n]).lower() in self.titles and is_cap(ws[n]):
                return n
        return None

    def rule(self, name, rows=1):
        """(kind, why) for a name capitalized throughout: generic, individual,
        place, pet or ambiguous. rows: how many npcs rows spell it, a kind
        spawned three times or more being no one person."""
        ws = name.split(" ")
        if ws[0] == "The":
            return "individual", "starts with The"
        low = [w.lower() for w in ws]
        if "the" in low[1:]:
            i = low.index("the", 1)
            if any(self.proper(w) for w in ws[:i]):
                return "individual", "a proper name before 'the'"
        hi = head_index(ws)
        head = ws[hi]
        if head.lower() in PLACE_HEADS or low[0] in PLACE_HEADS:
            return "place", f"a {('portal' if 'portal' in low else head.lower())}, named as a place"
        if head.lower() in PET_HEADS:
            return "pet", f"an enchanter's pet ({head.lower()})"
        if self.proper(head):
            return "individual", f"its head {head!r} is a proper word"
        noun = self.creature_noun(head)
        many = rows >= KIND_ROWS
        t = self.title_of(ws)
        if t and hi >= t:
            title = " ".join(ws[:t])
            if self.proper(ws[t]):
                return "individual", f"the title {title!r} before a proper name"
            if not noun:
                return "individual", f"the title {title!r} before a name"
            if not many:
                return "ambiguous", f"the title {title!r} before a creature's noun {head.lower()!r}"
        stray = [w for w in ws if self.proper(w) and not self.kind_word(w)]
        if stray and not many:
            if not noun:
                return "individual", f"{stray[0]!r}, a proper word, before {head.lower()!r}"
            return "ambiguous", f"{stray[0]!r}, in no other generic name, before a creature's noun {head.lower()!r}"
        if not noun and not many:
            return "ambiguous", f"{head.lower()!r} is no creature's or role's noun on this server"
        return "generic", "" if not many or (noun and not stray) else f"spawned {rows} times: a kind"

    def agrees(self, name):
        """Each word of the name cased as its class: a proper word capitalized,
        a common one lowercase."""
        return all(is_cap(w) == self.proper(w) for w in content_words(name.split(" ")))

    def pick(self, spellings):
        """The one of a name's lowercase-bearing spellings its words agree
        with ("Deathfist pawn", not "deathfist pawn"), of those the one whose
        proper words are spelled as most names spell them ("Teir'Dal knight"
        as "Teir'Dal" is 60 times to "Teir'dal"'s 16), then the one most rows
        have."""
        forms = self.word_forms

        def popular(n):
            return sum(forms[w] for w in n.split(" ") if is_cap(w))
        return sorted(spellings, key=lambda n: (not self.agrees(n), -popular(n), -spellings[n], n))[0]


class Decision:
    __slots__ = ("key", "kind", "target", "why")

    def __init__(self, key, kind, target, why):
        self.key, self.kind, self.target, self.why = key, kind, target, why


def decide(ev, key, town_rows=None):
    """How the name of this fold key is cased: a Decision whose target is the
    spelling every spelling of it takes, or None (each keeps its own case).
    town_rows: town_rows()'s for the name."""
    v = ev.variants[key]
    if not v:
        return Decision(key, "individual", None, "no spelling of it is known")
    rows = town_rows[0] if town_rows else 1
    if key in ev.soe:
        return Decision(key, "retail", ev.soe[key], "spelled so in a retail savestate's memory")
    if key in ev.pets:
        return Decision(key, "pet", None, "a summoned pet's name (a pet is named as its spell, capitalized)")
    lowered_spellings = collections.Counter({n: c for n, c in v.items() if shape(n) != "title"})
    titled = sorted((n for n in v if shape(n) == "title"), key=lambda n: (-v[n], n))
    kind, why = ev.rule(titled[0] if titled else max(v, key=lambda n: (v[n], n)), rows)
    if lowered_spellings:
        best = ev.pick(lowered_spellings)
        if kind == "individual" and shape(best) == "lower" and why.startswith(STRONG):
            return Decision(key, "ambiguous", None, f"spelled lowercase elsewhere ({best!r}), but {why}")
        return Decision(key, "variant", best, f"spelled so elsewhere ({len(lowered_spellings)} lowercase-bearing spelling"
                        f"{'s' if len(lowered_spellings) > 1 else ''})")
    if kind == "generic" and town_rows and town_rows[0] and town_rows[2] == town_rows[0]:
        if town_rows[1] == town_rows[0]:
            return Decision(key, "town", None, "every row a town's peaceful person (the NPC bit, a players' race body)")
        if town_rows[0] < KIND_ROWS:
            return Decision(key, "town", None, f"a person of a players' race, {town_rows[0]} of them: a role or a name")
    if kind == "generic":
        return Decision(key, "generic", lowered(titled[0], ev.proper), why or "a creature's or a role's noun, its words common")
    return Decision(key, kind, None, why)


# ---- the database


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def read_dict(path):
    """A word list's lowercase words."""
    if not path or not os.path.exists(path):
        return []
    with open(path, encoding="utf-8", errors="replace") as f:
        return [w for w in (ln.strip() for ln in f) if w and w == w.lower()]


def harvest_names(paths):
    """NPC names as Sandstorm's own entity records spell them, by distinct
    entity: the harvester's recordings whose "server" is Sandstorm's (their
    "seen" and "merchants", and the patrol tracks beside them), players (ids of
    100 million and more) aside."""
    files = []
    for p in paths:
        if os.path.isdir(p):
            for d, _, fs in os.walk(p):
                files += [os.path.join(d, f) for f in fs if f.endswith((".json", ".track.jsonl"))]
        else:
            files.append(p)

    def server_of(path):
        try:
            with open(path, encoding="utf-8", errors="replace") as f:
                m = re.search(r'"server":\s*"([^"]*)"', f.read(400))
        except OSError:
            return None
        return m.group(1) if m else None

    seen = collections.defaultdict(set)
    for p in sorted(set(files)):
        if p.endswith(".track.jsonl"):
            if server_of(p[:-len(".track.jsonl")]) not in SANDSTORM:
                continue
            with open(p, encoding="utf-8", errors="replace") as f:
                for line in f:
                    if '"name"' not in line:
                        continue
                    try:
                        e = json.loads(line)
                    except ValueError:
                        continue
                    if isinstance(e.get("id"), int) and e.get("name") and e["id"] < 100_000_000:
                        seen[e["name"]].add(e["id"])
            continue
        if server_of(p) not in SANDSTORM:
            continue
        try:
            with open(p, encoding="utf-8", errors="replace") as f:
                d = json.load(f)
        except ValueError:
            continue
        for k in ("seen", "merchants"):
            for e in (d.get(k) or []) if isinstance(d, dict) else []:
                if isinstance(e.get("id"), int) and e.get("name") and e["id"] < 100_000_000:
                    seen[e["name"]].add(e["id"])
    return collections.Counter({n: len(ids) for n, ids in seen.items()})


def pet_names(db):
    """The summoned pets' names: spellpattern's summoning spells (a pet is
    named as its spell) and the pets tools/seed_npc_aggro.py found."""
    out = set()
    if has_table(db, "spellpattern"):
        for name, desc in db.execute("SELECT spellname, spelldesc FROM spellpattern"):
            if name and "summons" in (desc or "").lower():
                out.add(name)
    if has_table(db, "npcaggro"):
        for (name,) in db.execute("SELECT npc_name FROM npcaggro WHERE source = 'seed' AND note LIKE 'a summoned pet''s name%'"):
            out.add(name)
    return out


def load_evidence(db, harvests=(), dict_path=DEFAULT_DICT):
    ours = collections.Counter()
    for n, c in db.execute("SELECT npc_name, count(*) FROM npcs WHERE npc_name IS NOT NULL GROUP BY npc_name"):
        ours[clean(n)] += c
    prh = collections.Counter()
    if has_table(db, "prh_import_npcs"):
        for n, c in db.execute("SELECT npc_name, count(*) FROM prh_import_npcs WHERE npc_name IS NOT NULL GROUP BY npc_name"):
            prh[clean(n)] += c
    sources = {"ours": ours, "returnhome": prh}
    if harvests:
        sources["sandstorm"] = harvest_names(harvests)
    return Evidence(sources, read_dict(dict_path), pet_names(db))


def town_rows(db):
    """Per fold key: [rows, rows with the NPC bit on a players' race body,
    rows on a players' race body]."""
    bodies = set()
    if has_table(db, "charactermodel"):
        bodies = {m & 0xFFFFFFFF for (m,) in db.execute("SELECT modelid FROM charactermodel") if m is not None}
    out = collections.defaultdict(lambda: [0, 0, 0])
    for n, t, m in db.execute("SELECT npc_name, npc_type, modelid FROM npcs WHERE npc_name IS NOT NULL"):
        r = out[fold(n)]
        r[0] += 1
        body = (m or 0) & 0xFFFFFFFF in bodies
        r[1] += bool((t or 0) & 0x80 and body)
        r[2] += body
    return out


class Plan:
    """The renames: renames maps each spelling in npcs to its new one (only
    those that change), targets each fold key with a case decision to its
    spelling, decisions every fold key's Decision."""

    def __init__(self, renames, decisions, spellings):
        self.renames = dict(renames)
        self.decisions = decisions
        self.spellings = spellings
        self.targets = {k: d.target for k, d in decisions.items() if d.target is not None}

    def new_name(self, value):
        """A reference's new value (a name as any column holds it), or None."""
        if value is None:
            return None
        t = self.targets.get(fold(value))
        if t is not None:
            return t if t != value else None
        n = self.renames.get(value)
        return n if n is not None and n != value else None


def plan(db, ev):
    rows = town_rows(db)
    spellings = collections.defaultdict(collections.Counter)
    for n, c in db.execute("SELECT npc_name, count(*) FROM npcs WHERE npc_name IS NOT NULL GROUP BY npc_name"):
        if fold(n):
            spellings[fold(n)][n] += c
    decisions, renames = {}, {}
    for key in sorted(spellings):
        if any(clean(n) in CODE_KEYED for n in spellings[key]):
            d = Decision(key, "code", None, "the server's code keys it as written")
        else:
            d = decide(ev, key, rows.get(key))
        if d.target is not None and any(code_reads_case(clean(n)) != code_reads_case(d.target) for n in spellings[key]):
            d = Decision(key, "code", None, f"the server's code reads its capitals ({d.target!r} would lose them)")
        decisions[key] = d
        for n in spellings[key]:
            new = d.target if d.target is not None else clean(n)
            if new != n:
                renames[n] = new
    return Plan(renames, decisions, spellings)


def plan_from_tsv(db, path):
    """A change list as --out wrote it (old, new, kind, why), applied as it is:
    a name no npcs row spells so any more is passed over."""
    have = {n for (n,) in db.execute("SELECT DISTINCT npc_name FROM npcs WHERE npc_name IS NOT NULL")}
    renames, decisions, spellings = {}, {}, collections.defaultdict(collections.Counter)
    with open(path, encoding="utf-8") as f:
        for ln in f:
            if ln.startswith("#") or not ln.strip():
                continue
            old, new, kind, why = (json.loads(x) for x in ln.rstrip("\n").split("\t")[:4])
            if old not in have:
                continue
            if new != old:
                renames[old] = new
            spellings[fold(old)][old] += 1
            target = new if kind in ("retail", "variant", "generic") else None
            decisions[fold(old)] = Decision(fold(old), kind, target, why)
    return Plan(renames, decisions, spellings)


# ---- what --apply writes


def log_set(db, tbl, rid, col, old, new, why=""):
    db.execute(f"INSERT INTO {LOG} (tbl, rid, col, old, new, why) VALUES (?, ?, ?, ?, ?, ?)", (tbl, rid, col, old, new, why))


def set_value(db, tbl, rid, col, old, new, why=""):
    db.execute(f"UPDATE {tbl} SET {col} = ? WHERE _rowid_ = ?", (new, rid))
    log_set(db, tbl, rid, col, old, new, why)


def row_dict(db, tbl, rid):
    cur = db.execute(f"SELECT _rowid_ AS rowid_, * FROM {tbl} WHERE _rowid_ = ?", (rid,))
    r = cur.fetchone()
    if r is None:
        return None
    names = [d[0] for d in cur.description]
    return dict(zip(names, r))


def merge_or_set(db, tbl, rid, col, old, new, unique, why, problems):
    """Rename one row's name, or, when a row holding the new name and the same
    values of the other unique columns is there already, delete this one into
    it when the rest agrees. unique: the other columns of its unique key."""
    row = row_dict(db, tbl, rid)
    where = " AND ".join([f"{col} = ?"] + [f"{c} = ?" for c in unique])
    other = db.execute(f"SELECT _rowid_ FROM {tbl} WHERE {where} AND _rowid_ != ?",
                       [new] + [row[c] for c in unique] + [rid]).fetchone()
    if other is None:
        set_value(db, tbl, rid, col, old, new, why)
        return "renamed"
    keep = row_dict(db, tbl, other[0])
    rest = [c for c in row if c not in ("rowid_", col, "id") and c not in unique]
    if any(row[c] != keep[c] for c in rest if c not in ("source", "note")):
        problems.append(f"{tbl} row {rid} ({old!r}) and row {other[0]} ({new!r}) differ: "
                        f"{ {c: row[c] for c in rest} } and { {c: keep[c] for c in rest} }")
        return "conflict"
    whole = {k: v for k, v in row.items() if k != "rowid_"}
    whole["_rowid_"] = rid
    db.execute(f"DELETE FROM {tbl} WHERE _rowid_ = ?", (rid,))
    log_set(db, tbl, rid, "*", json.dumps(whole, sort_keys=True), str(other[0]), why)
    return "merged"


# the JSON formats the columns are kept in, to write one back as it was written
JSON_FORMATS = ({"separators": (",", ":")}, {}, {"sort_keys": True}, {"separators": (",", ":"), "sort_keys": True})


def json_format(text, value):
    for kw in JSON_FORMATS:
        if json.dumps(value, **kw) == text:
            return kw
    return None


def rename_json(text, rename, keys=("npc",)):
    """A JSON column's text with the names under keys renamed (rename: value ->
    new value or None), written as it was; None when nothing changes or its
    format is not one of JSON_FORMATS (reported)."""
    try:
        value = json.loads(text)
    except (TypeError, ValueError):
        return None
    kw = json_format(text, value)
    changed = [False]

    def walk(v):
        if isinstance(v, dict):
            for k, x in v.items():
                if k in keys and isinstance(x, str):
                    n = rename(x)
                    if n is not None:
                        v[k] = n
                        changed[0] = True
                else:
                    walk(x)
        elif isinstance(v, list):
            for x in v:
                walk(x)
    walk(value)
    if not changed[0]:
        return None
    if kw is None:
        raise ValueError(f"a JSON value not in a known format: {text[:80]!r}")
    return json.dumps(value, **kw)


# columns that name an NPC by exact name or as said, and the unique columns
# beside the name (None: no unique key with the name)
NAME_COLUMNS = (
    ("factionhits", "npc_name", ("faction",)),
    ("npcsocial", "npc_name", ()),
    ("quests", "giver", None),
    ("queststeps", "target", None),
    ("questunlocks", "npc", ("questid", "dest")),
)
# columns renamed only where they spell a name as an npcs row does (charbind's
# spiritmaster is a town's name too)
EXACT_COLUMNS = (("charbind", "spiritmaster"),)
JSON_COLUMNS = (("queststeps", "events"), ("quests", "events"), ("gmtriggers", "steps"))
# tables keyed by the folded name (database.NPCClassKey), and their unique columns
FOLDED_COLUMNS = (
    ("npcclass", "name", ()),
    ("npcundead", "name", ()),
    ("npcmove", "name", ()),
    ("npcloot", "npc_name", ("tablename",)),
    ("npcfaction", "name", ("world", "x", "z", "radius")),
    ("npctemplate", "name", ()),
    ("npcaggro", "npc_name", ()),
    ("npcdialog", "name", None),
)
# the undo logs of other tools that key a folded table's row by its name, and
# the row's rowid column there (None: by name alone)
FOLDED_LOGS = {
    "npcfaction": (("npcfaction_seed", "name", "rowid"),),
    "npcclass": (("npcclass_fix_log", "name", None),),
    "npcmove": (("npcmove_seed", "name", None),),
    "npcaggro": (("quest_npc_seed_aggro", "npc_name", None), ("pod_seed_rules", "name", None)),
    "npcundead": (("pod_seed_rules", "name", None),),
}


def references(db, p):
    """Every value --apply would rename: (table, rowid, column, old, new, kind),
    kind 'name' (by exact name), 'json' or 'folded' (a folded key's
    doubled space)."""
    out = []
    for tbl, col, unique in NAME_COLUMNS:
        if not has_table(db, tbl):
            continue
        for rid, v in db.execute(f"SELECT _rowid_, {col} FROM {tbl} WHERE {col} IS NOT NULL AND {col} != ''"):
            n = p.new_name(v)
            if n is not None:
                out.append((tbl, rid, col, v, n, "name"))
    for tbl, col in EXACT_COLUMNS:
        if not has_table(db, tbl):
            continue
        for rid, v in db.execute(f"SELECT _rowid_, {col} FROM {tbl} WHERE {col} IS NOT NULL AND {col} != ''"):
            if v in p.renames:
                out.append((tbl, rid, col, v, p.renames[v], "name"))
    for tbl, col in JSON_COLUMNS:
        if not has_table(db, tbl):
            continue
        for rid, v in db.execute(f"SELECT _rowid_, {col} FROM {tbl} WHERE {col} LIKE '%\"npc\"%'"):
            n = rename_json(v, p.new_name)
            if n is not None:
                out.append((tbl, rid, col, v, n, "json"))
    renamed_keys = {fold(n) for n in p.renames}
    for tbl, col, unique in FOLDED_COLUMNS:
        if not has_table(db, tbl):
            continue
        for rid, v in db.execute(f"SELECT _rowid_, {col} FROM {tbl} WHERE {col} LIKE '%  %'"):
            n = clean(v)
            if fold(v) in renamed_keys and class_key(n) != class_key(v):
                out.append((tbl, rid, col, v, n, "folded"))
    return out


def apply(db, p, skip=()):
    """Write the plan in one transaction. skip: tables left out (the tests'
    control). Returns (counts, problems); with problems nothing is written."""
    problems, done = [], collections.Counter()
    quest_digest = None
    if has_table(db, "quest_import_quests"):
        from import_quests import digest as quest_digest  # noqa: E402  (tools/import_quests.py)
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_DDL)
        if db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0]:
            raise SystemExit(f"{LOG} holds a rename: applied already (--undo first)")
        clean_quests = []
        if quest_digest is not None:
            clean_quests = [(rid, qid, dg) for rid, qid, dg in db.execute(
                "SELECT _rowid_, questid, digest FROM quest_import_quests") if quest_digest(db, qid) == dg]
        # npcs
        if "npcs" not in skip:
            for old, new in sorted(p.renames.items()):
                for (rid,) in db.execute("SELECT id FROM npcs WHERE npc_name = ?", (old,)).fetchall():
                    set_value(db, "npcs", rid, "npc_name", old, new, p.decisions[fold(old)].kind)
                    done["npcs"] += 1
        refs = references(db, p)
        renamed_rows = collections.defaultdict(dict)  # table -> rowid -> (old, new)
        for tbl, rid, col, old, new, kind in refs:
            if tbl in skip:
                continue
            why = p.decisions[fold(old)].kind if kind == "name" and fold(old) in p.decisions else kind
            if kind == "json":
                set_value(db, tbl, rid, col, old, new, "json")
                done[tbl] += 1
                continue
            unique = dict((t, u) for t, c, u in NAME_COLUMNS + FOLDED_COLUMNS).get(tbl)
            if unique is None:
                set_value(db, tbl, rid, col, old, new, why)
                how = "renamed"
            else:
                how = merge_or_set(db, tbl, rid, col, old, new, unique, why, problems)
            done[f"{tbl} {how}"] += 1
            if how == "renamed":
                renamed_rows[tbl][rid] = (old, new)
        # the other tools' undo logs, following their rows
        logs = [("quest_hit_seed", "npc_name", "rowid", "factionhits")]
        for tbl, ls in FOLDED_LOGS.items():
            logs += [(lt, lc, rc, tbl) for lt, lc, rc in ls]
        for lt, lc, rc, tbl in logs:
            if lt in skip or not has_table(db, lt) or not renamed_rows.get(tbl):
                continue
            pairs = renamed_rows[tbl]
            if rc is None:
                by_name = {o: n for o, n in pairs.values()}
                for rid, v in db.execute(f"SELECT _rowid_, {lc} FROM {lt}").fetchall():
                    if v in by_name:
                        set_value(db, lt, rid, lc, v, by_name[v], f"follows {tbl}")
                        done[lt] += 1
                continue
            for rid, ref, v in db.execute(f"SELECT _rowid_, {rc}, {lc} FROM {lt}").fetchall():
                if ref in pairs and pairs[ref][0] == v:
                    set_value(db, lt, rid, lc, v, pairs[ref][1], f"follows {tbl}")
                    done[lt] += 1
        # NPCs another tool made, logged as written
        npc_new = {}
        for (rid,) in [] if "npcs" in skip else db.execute(
                f"SELECT rid FROM {LOG} WHERE tbl = 'npcs' AND col = 'npc_name'").fetchall():
            npc_new[rid] = db.execute("SELECT npc_name FROM npcs WHERE id = ?", (rid,)).fetchone()[0]
        if npc_new and has_table(db, "npcpaths_seed") and "npcpaths_seed" not in skip:
            for rid, made in db.execute("SELECT _rowid_, made FROM npcpaths_seed WHERE made IS NOT NULL AND made != ''").fetchall():
                m = json.loads(made)
                npc = m.get("npc") or {}
                if rid in npc_new and npc.get("npc_name") is not None and npc["npc_name"] != npc_new[rid]:
                    kw = json_format(made, m)
                    npc["npc_name"] = npc_new[rid]
                    set_value(db, "npcpaths_seed", rid, "made", made, json.dumps(m, **(kw or {})), "follows npcs")
                    done["npcpaths_seed"] += 1
        if npc_new and has_table(db, "pod_boss_fix_log") and "pod_boss_fix_log" not in skip:
            for rid, row, after in db.execute("SELECT _rowid_, row, after FROM pod_boss_fix_log WHERE tbl = 'npcs' "
                                              "AND kind IN ('insert', 'update') AND after LIKE '%npc_name%'").fetchall():
                a = json.loads(after)
                if row in npc_new and a.get("npc_name") is not None and a["npc_name"] != npc_new[row]:
                    kw = json_format(after, a)
                    a["npc_name"] = npc_new[row]
                    set_value(db, "pod_boss_fix_log", rid, "after", after, json.dumps(a, **(kw or {})), "follows npcs")
                    done["pod_boss_fix_log"] += 1
        # import_quests.py's digests of the quests that matched theirs
        if "quest_import_quests" not in skip:
            for rid, qid, dg in clean_quests:
                now = quest_digest(db, qid)
                if now != dg:
                    set_value(db, "quest_import_quests", rid, "digest", dg, now, "the quest's names renamed")
                    done["quest_import_quests"] += 1
        if problems:
            raise RuntimeError("problems")
        db.execute("COMMIT")
    except RuntimeError:
        db.execute("ROLLBACK")
        return done, problems
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return done, problems


def logged(db):
    return has_table(db, LOG) and db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0] > 0


def undo(db):
    """Put back every value still as --apply wrote it, newest first; a merged
    row while the row it went into is there and its rowid free."""
    if not logged(db):
        return 0, []
    back, kept = 0, []
    db.execute("BEGIN IMMEDIATE")
    try:
        for seq, tbl, rid, col, old, new in db.execute(
                f"SELECT seq, tbl, rid, col, old, new FROM {LOG} ORDER BY seq DESC").fetchall():
            if not has_table(db, tbl):
                kept.append(f"{tbl} row {rid}: the table is gone")
                continue
            if col == "*":
                row = json.loads(old)
                rowid = row.pop("_rowid_")
                here = db.execute(f"SELECT count(*) FROM {tbl} WHERE _rowid_ = ?", (rowid,)).fetchone()[0]
                into = db.execute(f"SELECT count(*) FROM {tbl} WHERE _rowid_ = ?", (int(new),)).fetchone()[0]
                if here or not into:
                    kept.append(f"{tbl} row {rowid}: merged into row {new}, which is gone, or its rowid taken")
                    continue
                cols = ["_rowid_"] + list(row)
                db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                           [rowid] + [row[c] for c in row])
                back += 1
                continue
            r = db.execute(f"SELECT {col} FROM {tbl} WHERE _rowid_ = ?", (rid,)).fetchone()
            if r is None or r[0] != new:
                kept.append(f"{tbl} row {rid} {col}: {None if r is None else r[0]!r} now, written {new!r}")
                continue
            db.execute(f"UPDATE {tbl} SET {col} = ? WHERE _rowid_ = ?", (old, rid))
            back += 1
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return back, kept


# ---- the joins the server makes on a name


# (table, column, how the server matches it): 'exact' as spelled, 'folded' by
# NPCClassKey, 'said' by quests.go's foldName (trimmed, any case)
JOINS = (
    ("factionhits", "npc_name", "exact"),
    ("npcsocial", "npc_name", "exact"),
    ("charbind", "spiritmaster", "exact"),
    ("npcclass", "name", "folded"),
    ("npcundead", "name", "folded"),
    ("npcmove", "name", "folded"),
    ("npcloot", "npc_name", "folded"),
    ("npcfaction", "name", "folded"),
    ("npctemplate", "name", "folded"),
    ("npcaggro", "npc_name", "folded"),
    ("npcdialog", "name", "folded"),
    ("quests", "giver", "said"),
    ("queststeps", "target", "said"),
    ("questunlocks", "npc", "said"),
)


def name_joins(db):
    """{(table, column): set of (npcs row, table rowid)} the server's matching
    reaches: an exact column by the name as spelled, a folded one by
    NPCClassKey, a said one by quests.go's foldName (trimmed, any case)."""
    npcs = collections.defaultdict(list)
    keyed = collections.defaultdict(list)
    for rid, n in db.execute("SELECT id, npc_name FROM npcs WHERE npc_name IS NOT NULL"):
        npcs[n].append(rid)
        keyed[class_key(n)].append(rid)
    out = {}
    for tbl, col, how in JOINS:
        if not has_table(db, tbl):
            continue
        pairs = set()
        for rid, v in db.execute(f"SELECT _rowid_, {col} FROM {tbl} WHERE {col} IS NOT NULL AND {col} != ''"):
            for n in (npcs.get(v, ()) if how == "exact" else keyed.get(class_key(v), ())):
                pairs.add((n, rid))
        out[(tbl, col)] = pairs
    return out


# ---- the report


def report_lines(db, p, ev):
    kinds = collections.Counter()
    changed = collections.Counter()
    for k, d in p.decisions.items():
        kinds[d.kind] += 1
        if any(n in p.renames for n in p.spellings[k]):
            changed[d.kind] += 1
    lines = [f"names: {len(p.decisions)} (folded), {sum(len(s) for s in p.spellings.values())} spellings in npcs; "
             f"renamed spellings {len(p.renames)}"]
    for k in sorted(kinds, key=lambda k: -kinds[k]):
        lines.append(f"  {k}: {kinds[k]} names, {changed[k]} with a spelling renamed")
    rows = sum(c for k in p.spellings for n, c in p.spellings[k].items() if n in p.renames)
    lines.append(f"npcs rows renamed: {rows}")
    return lines


def write_lists(p, ev, out_dir):
    os.makedirs(out_dir, exist_ok=True)
    changes = os.path.join(out_dir, "npc-name-case.tsv")
    with open(changes, "w", encoding="utf-8") as f:
        f.write("# old\tnew\tkind\twhy (each a JSON string; tools/fix_npc_name_case.py --names reads it)\n")
        for old, new in sorted(p.renames.items(), key=lambda kv: (fold(kv[0]), kv[0])):
            d = p.decisions[fold(old)]
            f.write("\t".join(json.dumps(x) for x in (old, new, d.kind, d.why)) + "\n")
        # the names whose spellings are as decided already: --names needs them,
        # a reference spelled otherwise (a quest's giver) taking their spelling
        f.write("# spelled as decided already\n")
        for k, d in sorted(p.decisions.items()):
            for n in sorted(p.spellings[k]):
                if d.target is not None and n == d.target:
                    f.write("\t".join(json.dumps(x) for x in (n, n, d.kind, d.why)) + "\n")
    ambiguous = os.path.join(out_dir, "npc-name-case-ambiguous.tsv")
    with open(ambiguous, "w", encoding="utf-8") as f:
        f.write("# name (its spellings in npcs, rows)\tkind\twhy\n")
        for k, d in sorted(p.decisions.items(), key=lambda kv: (kv[1].kind, kv[0])):
            if d.kind in ("ambiguous", "pet", "town", "place"):
                sp = ", ".join(f"{n!r} {c}" for n, c in sorted(p.spellings[k].items()))
                f.write(f"{sp}\t{d.kind}\t{d.why}\n")
    sandstorm = os.path.join(out_dir, "npc-name-case-sandstorm.tsv")
    with open(sandstorm, "w", encoding="utf-8") as f:
        f.write("# name as planned\tSandstorm's spellings\n")
        ss = ev.sources.get("sandstorm") or {}
        by_key = collections.defaultdict(list)
        for n in ss:
            by_key[n.lower()].append(n)
        for k in sorted(p.decisions):
            if k not in by_key:
                continue
            planned = {p.renames.get(n, n) for n in p.spellings[k]}
            if any(s not in planned for s in by_key[k]):
                f.write(f"{' | '.join(sorted(planned))}\t{' | '.join(sorted(by_key[k]))}\n")
    return changes, ambiguous, sandstorm


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-namecase"
    shutil.copy2(path, dst)
    return dst


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--harvests", nargs="*", default=[], help="eqoa harvest recordings, or folders of them")
    ap.add_argument("--dict", default=DEFAULT_DICT, help="a word list; its lowercase words are common ('' none)")
    ap.add_argument("--out", help="write the change list and the ambiguous list here")
    ap.add_argument("--names", help="apply this change list (as --out writes it) instead of reading the names")
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        if not logged(db):
            print(f"{LOG} holds nothing: nothing to undo")
            return
        if not a.no_backup:
            print(f"copy: {backup(a.db)}")
        back, kept = undo(db)
        for k in kept:
            print(f"left: {k}")
        print(f"put back {back} values; {len(kept)} changed since, left as they are")
        return
    if a.apply and logged(db):
        sys.exit(f"{LOG} holds a rename: applied already (--undo first)")
    ev = load_evidence(db, a.harvests, a.dict)
    print(f"evidence: {', '.join(f'{k} {len(v)} names' for k, v in ev.sources.items())}; "
          f"dictionary {len(ev.dict_words)} lowercase words ({a.dict or 'none'}); {len(ev.pets)} pets; {len(ev.titles)} titles")
    p = plan_from_tsv(db, a.names) if a.names else plan(db, ev)
    for ln in report_lines(db, p, ev):
        print(ln)
    refs = collections.Counter((t, k) for t, _, _, _, _, k in references(db, p))
    for (t, k), n in sorted(refs.items()):
        print(f"  references: {t} {n} ({k})")
    if a.out:
        for path in write_lists(p, ev, a.out):
            print(f"wrote {path}")
    if not a.apply:
        print("report only: --apply to write")
        return
    if not a.no_backup:
        print(f"copy: {backup(a.db)}")
    done, problems = apply(db, p)
    for ln in problems:
        print(f"problem: {ln}")
    if problems:
        sys.exit("problems: nothing written")
    print("wrote " + ", ".join(f"{v} {k}" for k, v in sorted(done.items())))
    print("restart the server for the world to read the names")


if __name__ == "__main__":
    main()
