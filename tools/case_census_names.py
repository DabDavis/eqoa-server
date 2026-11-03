#!/usr/bin/env python3
"""The names Sandstorm's census brought in, cased as ours are (the user,
2026-10-31: "also fix the uppercase/lowercase match it to our style").

Sandstorm capitalizes every word of every name ("Rabid Timberwolf", "Tendora-
Buki Spahi", "Toxic Mist"), and tools/fix_npc_name_case.py, the rules our
names were cased by on 2026-10-30, reads a word its dictionary lacks as a
proper word: so a census kind whose last word is no dictionary word
("Timberwolf", "Seaturtle", "Spahi", "Tunneler") was read as someone's name
and kept its capitals, and "Blackhide", "Leatherback" or "Pestiferous" stayed
capitalized in front of a lowercase noun.

Our style (that tool's): a generic name lowercase but for its proper words
("moss snake", "Deathfist reaver", "Tae Ew crusader", "partisan of Paineel");
an individual's, a pet's, a town's people's and a place's as written. For a
name the census made three or more creatures of, here:

- It is a kind's, whatever its last word: no one person stands in three
  places. Its head (the word before " of ", else the last) is the kind's
  noun, lowercase ("rabid timberwolf", "Tendora-Buki spahi"), unless it
  carries a capital inside it ("Gorvak One-Eye": a name).
- Any other word is proper when our names other than the census's say so (a
  capital inside a name that mixes cases, more often than it is written
  lowercase: "Tae", "Paineel"), when it carries a capital inside it
  ("Atesh-Ah", "Ti'Rizeth", "Jal'Raeth"), or when it is none of: a
  dictionary word, two of them run together ("Blackhide", "Murkwood",
  "Ironhorn"), one with an ending added ("Corruptor"), a creature's noun,
  or one of COMMON (English words the dictionary lacks: "pestiferous",
  "leatherback"). Else lowercase.
- Left as written: a summoned pet's name (a spell's: "Feral Spirit"), a
  town's peaceful people (the NPC bit on a players' race body), a name the
  server's code keys or whose capitals it reads (fix_npc_name_case.py's
  CODE_KEYED, GUARD_TITLE, ROLE_PREFIXES), and a name seen fewer than three
  times, or twice with a last word that is no noun we know or a title before
  it (fix_npc_name_case.py's rules had their say at the import), and anyone's
  by "the" ("Neskar the Deep", "The Storyteller"). A title before a kind seen
  three times or more is no title: "acolyte satyr", 160 of them.

Every row of such a name is renamed, in every world (a kept row of ours too:
one name, one spelling), and every reference with it, by
fix_npc_name_case.py's own apply (quests' givers and targets, faction hits,
the tools' logs), logged in npc_name_case_census_log, not that tool's log:
--undo puts back this pass alone. --tag NAME is a tagged census's pass
(tools/seed_sandstorm_census.py --tag NAME, a city's census of 2026-11-03):
only the names that census made are renamed (every census's rows still
counted), logged in npc_name_case_census_log_<NAME>, applied and undone
apart from the others. Report only by default; --out writes the
change list. The running server reads names at its start: apply with it
stopped (a swap's before.sh), before cmd/wildspread copies the names.

    tools/case_census_names.py [--db eqoa.db] [--dict FILE] [--tag NAME] [--out DIR] [--apply | --undo]
"""
import argparse
import collections
import json
import os
import re
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import fix_npc_name_case as nc  # noqa: E402

LOG = "npc_name_case_census_log"
# fix_npc_name_case.py's own log and its table's statement, as that tool has
# them: use_log points its apply and undo at this pass's log
NC_LOG, NC_LOG_DDL = nc.LOG, nc.LOG_DDL
KIND_ROWS = nc.KIND_ROWS
# a name the census made twice is a kind's too when its last word is a noun we
# know ("Elder Ratwere", "Fetid Hydra"), not when it is no word ("Bouncer Trod")
PAIR_ROWS = 2
# English words the dictionary lacks, seen in the census's names
COMMON = frozenset("""
pestiferous leatherback timberwolf seaturtle rockcrusher necrapede weregator gatorwere tunneler windstrider corruptor
sacrificer weaponmaker spahi ispusar puzhal ghulam pasdar mamluk mujahed ademzada dreg amir ghazi
""".split())
# endings a word takes on a dictionary word ("corrupt" + "or")
ENDINGS = ("er", "or", "r", "ers", "ors", "s", "es", "ed", "ing", "ous", "ly", "ling")
APOSTROPHE = re.compile(r"[-'][A-Z]")


def log_name(tag=""):
    """This pass's log; a tagged census's pass its own."""
    return LOG + (f"_{tag}" if tag else "")


def use_log(tag=""):
    """fix_npc_name_case.py's apply and undo write and read the module's LOG:
    this pass's log instead (a tagged census's its own)."""
    nc.LOG = log_name(tag)
    nc.LOG_DDL = NC_LOG_DDL.replace(NC_LOG, nc.LOG)


def census_names(db, tag=None):
    """npc_name -> how many rows Sandstorm's census added of it, every world;
    with a tag, that census's alone (sandstorm_census_seed_w<world>_<tag>)."""
    out = collections.Counter()
    mine = re.compile(rf"sandstorm_census_seed_w\d+_{re.escape(tag)}") if tag else None
    for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' AND name LIKE 'sandstorm_census_seed_w%'"):
        if mine is not None and not mine.fullmatch(t):
            continue
        for (n,) in db.execute(f"SELECT n.npc_name FROM {t} s JOIN npcs n ON n.id = s.rid "
                               f"WHERE s.tbl = 'npcs' AND s.old IS NULL"):
            if n:
                out[n] += 1
    return out


def evidence_without_census(db, dict_path):
    """fix_npc_name_case.py's evidence, from our names alone: the census's own
    spellings (decided by these very rules) prove nothing of a word."""
    census_rows = set()
    for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' AND name LIKE 'sandstorm_census_seed_w%'"):
        census_rows |= {r for (r,) in db.execute(f"SELECT rid FROM {t} WHERE tbl = 'npcs' AND old IS NULL")}
    # cmd/wildspread's fills, a tagged run's (wildspread_adds_<tag>) too: copies of the census's names
    for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' AND (name = 'wildspread_adds' "
                           "OR name LIKE 'wildspread\\_adds\\_%' ESCAPE '\\')").fetchall():
        census_rows |= {r for (r,) in db.execute(f"SELECT npc_row FROM {t}")}
    ours = collections.Counter()
    for rid, n in db.execute("SELECT id, npc_name FROM npcs WHERE npc_name IS NOT NULL"):
        if rid not in census_rows:
            ours[nc.clean(n)] += 1
    prh = collections.Counter()
    if nc.has_table(db, "prh_import_npcs"):
        for n, c in db.execute("SELECT npc_name, count(*) FROM prh_import_npcs WHERE npc_name IS NOT NULL GROUP BY npc_name"):
            prh[nc.clean(n)] += c
    return nc.Evidence({"ours": ours, "returnhome": prh}, nc.read_dict(dict_path), nc.pet_names(db))


class Words:
    """Which words of a census kind's name are proper. names: every census
    kind's name, so a word a name puts before its noun as a proper word
    ("Ankexfen knight") is proper as another's noun ("animated Ankexfen")."""

    def __init__(self, ev, names=()):
        self.ev = ev
        self.dict = ev.dict_words
        self.proper_before = set()
        for n in names:
            ws = nc.clean(n).split(" ")
            hi = nc.head_index(ws)
            for i, w in enumerate(ws):
                if i != hi and self.proper(w, False):
                    self.proper_before.add(w.lower())

    def known(self, w):
        """A common word: the dictionary's, COMMON's, a creature's noun, or one
        with an ending added to one of those."""
        if w in self.dict or w in COMMON or self.ev.creature_noun(w):
            return True
        for e in ENDINGS:
            if w.endswith(e) and len(w) - len(e) >= 3:
                stem = w[: -len(e)]
                if stem in self.dict or stem + "e" in self.dict or (len(stem) > 3 and stem[-1] == stem[-2] and stem[:-1] in self.dict):
                    return True
        return False

    def compound(self, w):
        """Two common words run together ("blackhide", "murkwood")."""
        return any(len(w[:i]) >= 3 and len(w[i:]) >= 3 and self.known(w[:i]) and self.known(w[i:]) for i in range(3, len(w) - 2))

    def proper(self, word, head):
        if APOSTROPHE.search(word):
            return True  # Atesh-Ah, Ti'Rizeth, One-Eye
        k = word.lower()
        if k in nc.FUNCTION_WORDS or not nc.ALPHA.search(k):
            return False
        c, lm, la = self.ev.cap_mixed[k], self.ev.low_mixed[k], self.ev.low_all[k]
        if c or lm:
            return c > lm + la
        if head:
            return k in getattr(self, "proper_before", ())  # a kind's noun, but a tribe's name ("animated Ankexfen")
        bare = re.sub(r"[^a-z]", "", k)
        return not (self.known(bare) or self.compound(bare))


def target_of(name, words):
    """A census kind's spelling in our style, or None to leave it."""
    ws = nc.clean(name).split(" ")
    hi = nc.head_index(ws)
    if APOSTROPHE.search(ws[hi]) or (words.ev.cap_mixed[ws[hi].lower()] > words.ev.low_mixed[ws[hi].lower()] + words.ev.low_all[ws[hi].lower()]):
        return None  # a name's head ("Gorvak One-Eye"): someone's
    return " ".join(w if words.proper(w, i == hi) else w.lower() for i, w in enumerate(ws))


def plan(db, dict_path=nc.DEFAULT_DICT, tag=""):
    """A fix_npc_name_case.Plan of the census kinds' renames; with a tag,
    of the names that census made (counted with every census's rows)."""
    ev = evidence_without_census(db, dict_path)
    made = census_names(db)
    only = {nc.fold(n) for n in census_names(db, tag)} if tag else None
    words = Words(ev, [n for n, c in made.items() if c >= KIND_ROWS])
    town = nc.town_rows(db)
    spellings = collections.defaultdict(collections.Counter)
    for n, c in db.execute("SELECT npc_name, count(*) FROM npcs WHERE npc_name IS NOT NULL GROUP BY npc_name"):
        spellings[nc.fold(n)][n] += c
    count = collections.Counter()
    for n, c in made.items():
        count[nc.fold(n)] += c
    decisions, renames = {}, {}
    for key, c in sorted(count.items()):
        names = spellings[key]
        if c < PAIR_ROWS or (only is not None and key not in only):
            continue
        spelled = max(names, key=lambda n: (names[n], n))
        ws = nc.clean(spelled).split(" ")
        if c < KIND_ROWS and not (words.known(ws[nc.head_index(ws)].lower()) or words.compound(ws[nc.head_index(ws)].lower())):
            continue  # twice, and its last word no noun we know: maybe someone's
        kind, why = ev.rule(nc.clean(spelled), c)
        if kind == "individual" and (why.startswith(("starts with The", "a proper name before 'the'")) or
                                     (c < KIND_ROWS and why.startswith(nc.STRONG))):
            continue  # "The ...", "<name> the ...", or twice and a title before a name: someone's
        if any(nc.clean(n) in nc.CODE_KEYED for n in names):
            continue
        if key in ev.pets or nc.class_key(key) in {nc.class_key(p) for p in ev.pets}:
            continue
        t = town.get(key)
        if t and t[0] and t[1] == t[0]:
            continue  # a town's peaceful people
        new = target_of(spelled, words)
        if new is None or any(nc.code_reads_case(nc.clean(n)) != nc.code_reads_case(new) for n in names):
            continue
        decisions[key] = nc.Decision(key, "generic", new, f"Sandstorm's census made {c}: a kind")
        for n in names:
            if n != new:
                renames[n] = new
    return nc.Plan(renames, decisions, spellings)


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=nc.DEFAULT_DB)
    ap.add_argument("--dict", default=nc.DEFAULT_DICT)
    ap.add_argument("--tag", default="", help="a tagged census's pass (seed_sandstorm_census.py --tag): its names, its own log")
    ap.add_argument("--out", help="write the change list here (TSV)")
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    if a.tag and not all(ch.isalnum() or ch == "_" for ch in a.tag):
        sys.exit(f"--tag {a.tag!r}: letters, digits and _ only")
    use_log(a.tag)  # this pass's own log, not fix_npc_name_case.py's
    db = sqlite3.connect(a.db, timeout=5, isolation_level=None)
    if a.undo:
        back, kept = nc.undo(db)
        print(f"undone: {back} values put back ({nc.LOG})" + (f"; kept {len(kept)}: " + "; ".join(kept[:20]) if kept else ""))
        return
    if a.apply and nc.logged(db):
        sys.exit(f"{nc.LOG} holds a rename: applied already (--undo first)")
    p = plan(db, a.dict, a.tag)
    rows = sum(c for n, c in ((n, sum(s[n] for s in p.spellings.values() if n in s)) for n in p.renames))
    print(f"census kinds renamed: {len(p.decisions)} names, {rows} rows")
    for n, new in sorted(p.renames.items(), key=lambda x: x[0].lower()):
        print(f"  {n!r} -> {new!r}")
    if a.out:
        os.makedirs(a.out, exist_ok=True)
        with open(os.path.join(a.out, "census-name-case.tsv"), "w") as f:
            f.write("# old\tnew\twhy (each a JSON string)\n")
            for n, new in sorted(p.renames.items()):
                f.write(f"{json.dumps(n)}\t{json.dumps(new)}\t{json.dumps(p.decisions[nc.fold(n)].why)}\n")
    if a.apply:
        done, problems = nc.apply(db, p)
        if problems:
            sys.exit("nothing written: " + "; ".join(problems[:20]))
        print("applied:", dict(done), f"({nc.LOG})")


if __name__ == "__main__":
    main()
