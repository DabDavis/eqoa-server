#!/usr/bin/env python3
"""The quests jib's quest walk of 2026-10-30 could not finish, put right in the data (the walk: every
quest in the game taken, walked and handed in by the headless client on a copy of eqoaclassic, 1,335
of 1,386 passing for some character).

**Hand-ins only another quest gives** (REQUIRES). A give or show step's item that neither the quest
itself gives before the step, nor anything else (a merchant, a corpse, a loot table, a craft, a fish,
a starting item, a spell's summoning), nor a quest the quest requires, comes from quests the player
need not have done: the 30-49 paths' runed tablets and writs, the 60 epics' recruiter tokens, the
totem chain, the Nagafen passage's token. Those quests become a requirement: one quest as one, several
as one group any of them meets ("a|b|c", database.ParseRequires): each class of a quest several
classes take gets the token from a quest of its own (Ruddish's token from the cleric's, the shaman's
or the druid's The Recruiter; Crikston's note from each city's A New Trainer in Forkwatch), so the
group holds the givers a character of the quest's classes and races may take, and none that requires
the quest itself.

**A give step that hands on an item a later step needs** (KEEP). A give step whose item the quest
itself gave, which a later step of it gives or shows again with nothing giving it back between, and
which nothing else provides ("take them to Vedilion", "only Nalzik can open it"): a show, the item
kept. So is a repeatable quest's give of an item only quests that are not repeatable give (one each:
the Nagafen passage), which its next run would lack.

**The city class quests to the races that start there** (RACES; the user's pick of 2026-10-30, "the
races that start there"). A quest of a city's class chain (import key 1-20/<city>/<class>/L<n>) with
no races gets the races that start in that city as that class: each of defaultcharacter's combos
(race, humType, class) in its home city as database.HomeCity names it (an elf's rangers, bards, rogues
and druids Tethelin's, its others Fayspire's; a western human's rangers and druids Surefall Glade's,
its others Qeynos'), and the froglok (race 10, a human of the east in all but its body: database's
startPlace) where an eastern human of the class starts. A human of either city is race 0 to a quest:
neither starts at -350 or below with the other city's guards (factionstart), so the race gate sends
no one into a city that attacks it.

**A quest's NPC a GM deleted** (NPCS). A step's or a giver's NPC no npcs row names, which the GM
panel's delete took (gm_audit, not reverted), is put back as it stood (its id, every column of the
audit's before) with an audit line of its own marking the delete reverted, as the user had funyun's
three Neriak quest NPCs restored on 2026-10-30: the Stranger (quest 101230's step 3, placed by
tools/seed_quest_npcs.py's set qd at the top of the ramp into Neriak).

Reported, not changed: a give or show step's item nothing provides at all, one a step of the quest
gives only after it, a requirement whose givers are all drafts, and a stackable item a step wants
more of than one stack holds (CLIENT keeps one record of a stackable item: VIItemList::AddItem).

Every value changed is logged in quest_chain_fix_log; --undo puts back each one still as written,
newest first, and a second --apply is refused while the log holds one. tools/import_quests.py's
digests are made again for the quests whose digest matched before (so a re-import still tells a GM's
change), and --write-data DIR makes the same changes in the quest files (their layout kept, each file
dumped as it was read), so a re-import writes what --apply did. The quests and NPCs load at the
server's start: apply with the restart that brings the code that reads a group ("a|b"), which an
older server reads as both.

    tools/fix_quest_chains.py [--db eqoa.db] [--apply | --undo] [--write-data tools/quests/data]
"""
import argparse
import collections
import datetime
import json
import os
import re
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import import_quests as iq  # noqa: E402  (its digest)

LOG = "quest_chain_fix_log"
LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl TEXT NOT NULL,           -- quests, queststeps, npcs, gm_audit, quest_import_quests
    rid TEXT NOT NULL,           -- questid, questid/step, the npcs id, the gm_audit id, the import key
    col TEXT NOT NULL,           -- the column, * for a row made
    old TEXT,                    -- JSON, what it was (a row made: none)
    new TEXT,                    -- JSON, what it became
    why TEXT NOT NULL DEFAULT ''
)"""
STAMP = "jib's quest walk of 2026-10-30, tools/fix_quest_chains.py"
CITY_KEY = re.compile(r"^1-20/([^/]+)/([^/]+)/L\d+[a-z]?$")
CLASSES = ["warrior", "ranger", "paladin", "shadowknight", "monk", "bard", "rogue", "druid", "shaman", "cleric",
           "magician", "necromancer", "enchanter", "wizard", "alchemist"]
RACE_NAMES = {0: "human", 1: "elf", 2: "dark elf", 3: "gnome", 4: "dwarf", 5: "troll", 6: "barbarian", 7: "halfling",
              8: "erudite", 9: "ogre", 10: "froglok"}
FROGLOK = 10  # database.raceFroglok: a human of the east (humType 1) in all but its body
# database.HomeCity's cities, by the quest files' city keys
RACE_CITY = {2: "neriak-nektulos", 3: "klick-anon", 4: "moradhim", 5: "grobb", 6: "halas", 7: "rivervale",
             8: "highbourne", 9: "oggok"}
SUMMON = re.compile(r"^summ[ou]ns?\s+(?:item\s*:?\s*)?(?:an?\s+|the\s+)?(.+?)(?:\s+x\s*(\d+))?$", re.I)
NPC_KINDS = ("talk", "show", "give")


def home_city(race, hum, cls):
    """database.HomeCity, as the quest files key the city."""
    if race == 0:
        if hum == 2:
            return "surefall-glade" if cls in (1, 7) else "qeynos"
        return "freeport"
    if race == 1:
        return "tethelin" if cls in (1, 5, 6, 7) else "fayspire"
    return RACE_CITY.get(race)


def fold(s):
    return " ".join(str(s or "").lower().split())


def has_table(db, t):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone()[0] > 0


def items(s):
    """"4866*2, 8373" as [(4866, 2), (8373, 1)] (database.ParseItemCounts)."""
    out = []
    for part in str(s or "").split(","):
        part = part.strip()
        if not part:
            continue
        pid, _, n = part.partition("*")
        try:
            out.append((int(pid), max(int(n), 1) if n.strip() else 1))
        except ValueError:
            pass
    return out


def event_items(e):
    v = e.get("items")
    if isinstance(v, list):  # a file's form
        return [(it, 1) if isinstance(it, int) else (it.get("item"), int(it.get("count") or 1)) for it in v]
    return items(v)


def evs(s):
    try:
        v = json.loads(s) if s else []
        return v if isinstance(v, list) else []
    except ValueError:
        return []


def ints(s):
    return [int(x) for x in re.findall(r"-?\d+", str(s or ""))]


def groups(s):
    """The requires column as database.ParseRequires reads it."""
    out = []
    for part in str(s or "").split(","):
        if "|" not in part:
            out += [[n] for n in ints(part)]
        else:
            g = [n for alt in part.split("|") for n in ints(alt)]
            if g:
                out.append(g)
    return out


def format_groups(gs):
    return ", ".join("|".join(map(str, g)) for g in gs if g)


class Data:
    """What the database holds of the quests and of every other way to an item."""

    def __init__(self, db):
        self.db = db
        self.keys = {}  # questid -> (key, file)
        if has_table(db, "quest_import_quests"):
            for k, qid, f in db.execute("SELECT key, questid, file FROM quest_import_quests"):
                self.keys[qid] = (k, f)
        self.quests = {}
        for qid, name, en, giver, cls, races, req, rep, qev in db.execute(
                "SELECT questid, name, enabled, giver, classes, races, requires, repeatable, events FROM quests"):
            self.quests[qid] = dict(qid=qid, name=name, enabled=en, giver=giver, classes=ints(cls), races=ints(races),
                                    requires=req or "", groups=groups(req), repeatable=bool(rep), events=evs(qev), steps=[])
        for r in db.execute("SELECT questid, step, kind, target, items, rewarditems, events, byclass, choices, count "
                            "FROM queststeps ORDER BY questid, step"):
            q = self.quests.get(r[0])
            if q is None:
                continue
            byclass, choices = {}, {}
            try:
                byclass = json.loads(r[7]) if r[7] else {}
                choices = json.loads(r[8]) if r[8] else {}
            except ValueError:
                pass
            q["steps"].append(dict(step=r[1], kind=r[2], target=r[3], items=items(r[4]), rewards=items(r[5]),
                                   events=evs(r[6]), byclass=byclass or {}, choices=choices or {}, count=r[9]))
        self.item = {pid: dict(name=n, maxstack=ms or 1) for pid, n, ms in
                     db.execute("SELECT patternid, itemname, maxstack FROM itempattern")}
        by_name = collections.defaultdict(list)
        for pid in sorted(self.item):
            by_name[fold(self.item[pid]["name"])].append(pid)
        # every way to an item but a quest's
        self.other = collections.defaultdict(set)
        if has_table(db, "npcitem"):
            merchants = {r[0] for r in db.execute("SELECT id FROM npcs WHERE npc_type & 1")}
            for npc, pid in db.execute("SELECT npcid, patternid FROM npcitem"):
                self.other[pid].add("a merchant" if npc in merchants else "a corpse")
        if has_table(db, "loottables") and has_table(db, "npcloot"):
            bound = {r[0] for r in db.execute("SELECT DISTINCT tablename FROM npcloot")}
            for pid, t in db.execute("SELECT patternid, tablename FROM loottables WHERE weight > 0"):
                if t in bound:
                    self.other[pid].add("a loot table")
        if has_table(db, "lootpool"):
            for (pid,) in db.execute("SELECT DISTINCT patternid FROM lootpool WHERE weight > 0"):
                self.other[pid].add("a loot pool")
        for t, col, what in (("craftrecipes", "product", "a craft"), ("fishcatch", "itemid", "fishing"),
                             ("startitems", "patternid", "a new character's start")):
            if has_table(db, t):
                for (pid,) in db.execute(f"SELECT DISTINCT {col} FROM {t}"):
                    self.other[pid].add(what)
        if has_table(db, "spelleffect"):
            for (line,) in db.execute("SELECT line FROM spelleffect WHERE line LIKE '%ummon%'"):
                for part in line.split(","):
                    m = SUMMON.match(re.sub(r"^\s*(self|target|group)\s+", "", part.strip(), flags=re.I))
                    if m:
                        for pid in by_name.get(fold(re.sub(r"\s*\(.*?\)\s*", " ", m.group(1))), []):
                            self.other[pid].add("a spell's summoning")
        # each quest's gives, any step or event (a byclass or a choice too)
        self.gives = collections.defaultdict(set)  # questid -> patternids
        for q in self.quests.values():
            for at, pid, n, sure in self.own(q):
                self.gives[q["qid"]].add(pid)
        self.givers = collections.defaultdict(set)  # patternid -> questids
        for qid, pids in self.gives.items():
            for pid in pids:
                self.givers[pid].add(qid)
        self._sure = {}
        self._closure = {}

    def own(self, q):
        """The quest's own gives: (the first step that can use it, pattern, count, given whatever the
        class or choice). A start event's is its step's; a step's rewards, done events and loot the
        next step's; the quest's accept events the first step's; its finish events none's."""
        out = []
        first = q["steps"][0]["step"] if q["steps"] else 1
        for e in q["events"]:
            if (e.get("do") or "").lower() == "give" and (e.get("on") or "").lower() == "accept":
                out += [(first, pid, n, True) for pid, n in event_items(e)]
            elif (e.get("do") or "").lower() == "give":
                out += [(10 ** 6, pid, n, True) for pid, n in event_items(e)]
        for st in q["steps"]:
            s = st["step"]
            for e in st["events"]:
                if (e.get("do") or "").lower() == "give":
                    at = s if (e.get("on") or "").lower() == "start" else s + 1
                    out += [(at, pid, n, True) for pid, n in event_items(e)]
            if st["kind"] == "loot" and st["items"]:
                pid, n = st["items"][0]
                out.append((s + 1, pid, max(st["count"] or 0, n, 1), True))
            out += [(s + 1, pid, n, True) for pid, n in st["rewards"]]
            sure_class = len(q["classes"]) == 1
            for cid, v in (st["byclass"] or {}).items():
                for pid, n in items(v.get("items") if isinstance(v, dict) else ""):
                    out.append((s + 1, pid, n, sure_class and str(q["classes"][0]) == str(cid)))
            for o in (st["choices"] or {}).get("options") or []:
                for pid, n in items(o.get("items")):
                    out.append((s + 1, pid, n, False))
        return out

    def held_at(self, q, pid, step, kept=()):
        """(given, held) of an item when step is done: what the quest gave it by then, and that less
        what its earlier give steps took (a step in kept is a show)."""
        given = sum(n for at, p, n, sure in self.own(q) if p == pid and sure and at <= step)
        took = sum(n for st in q["steps"] if st["step"] < step and st["kind"] == "give"
                   and (q["qid"], st["step"]) not in kept for p, n in st["items"] if p == pid)
        return given, given - took

    def closure(self, qid, seen=()):
        """Every quest a quest requires, through its requirements, each of a group's too."""
        if qid in self._closure:
            return self._closure[qid]
        out = set()
        for g in self.quests.get(qid, {}).get("groups", []):
            for r in g:
                if r not in out and r not in seen:
                    out.add(r)
                    out |= self.closure(r, seen + (qid,))
        self._closure[qid] = out
        return out

    def sure(self, qid, seen=()):
        """The items a player who may take the quest has had from the quests it requires: of each
        group, those every one of its quests (or those it requires in turn) gives."""
        if qid in self._sure:
            return self._sure[qid]
        out = set()
        for g in self.quests.get(qid, {}).get("groups", []):
            each = None
            for r in g:
                got = set() if r in seen or r not in self.quests else self.gives[r] | self.sure(r, seen + (qid,))
                each = got if each is None else each & got
            out |= each or set()
        self._sure[qid] = out
        return out

    def may_share(self, a, b):
        """Whether a character could take both quests: a class and a race of both (none: any)."""
        qa, qb = self.quests[a], self.quests[b]
        cl = not qa["classes"] or not qb["classes"] or set(qa["classes"]) & set(qb["classes"])
        ra = not qa["races"] or not qb["races"] or set(qa["races"]) & set(qb["races"])
        return bool(cl) and bool(ra)

    def name(self, pid):
        return self.item.get(pid, {}).get("name", f"item {pid}")


class Plan:
    def __init__(self, db):
        self.db = db
        self.d = Data(db)
        self.requires = []  # dict(qid, add, item, why)
        self.keeps = []     # dict(qid, step, item, why)
        self.races = []     # dict(qid, city, cls, races)
        self.npcs = []      # dict(name, audit, row, quests)
        self.reports = []   # what is left for a person, said
        self.table = {}     # (city, class) -> races

    def build(self):
        self.plan_keeps()
        self.plan_requires()
        self.plan_races()
        self.plan_npcs()
        self.plan_stacks()
        return self

    def in_files(self, qid):
        return qid in self.d.keys

    # -- KEEP

    def plan_keeps(self):
        d = self.d
        kept = set()
        for qid in sorted(d.quests):
            q = d.quests[qid]
            steps = q["steps"]
            for i, st in enumerate(steps):
                if st["kind"] != "give":
                    continue
                for pid, n in st["items"]:
                    if d.other.get(pid):
                        continue  # another can be had: two hand-ins are two
                    later = [t for t in steps[i + 1:] if t["kind"] in ("give", "show") and pid in dict(t["items"])]
                    own = [x for x in d.own(q) if x[1] == pid]
                    if later and own:
                        t = later[0]
                        need = dict(t["items"])[pid]
                        _, held = d.held_at(q, pid, t["step"], kept)
                        _, held_kept = d.held_at(q, pid, t["step"], kept | {(qid, st["step"])})
                        if held < need <= held_kept:
                            kept.add((qid, st["step"]))
                            self.keeps.append(dict(qid=qid, step=st["step"], item=pid, why=(
                                f"step {t['step']} {t['kind']}s the {d.name(pid)} to {t['target']} again, and nothing "
                                f"gives it back between: shown at step {st['step']}, not given")))
                            break
                    if not own and q["repeatable"] and d.givers.get(pid) and \
                            not any(d.quests[g]["repeatable"] for g in d.givers[pid] if g != qid):
                        kept.add((qid, st["step"]))
                        self.keeps.append(dict(qid=qid, step=st["step"], item=pid, why=(
                            f"a repeatable quest taking the {d.name(pid)}, which only quests done once give "
                            f"({', '.join(map(str, sorted(d.givers[pid])))}): its next run would lack it")))
                        break
        self.kept = kept

    # -- REQUIRES

    def plan_requires(self):
        d = self.d
        for qid in sorted(d.quests):
            add = self.needs(qid)
            if add:
                self.requires.append(dict(qid=qid, add=add))
                self.require(qid, [a["group"] for a in add])
        # a requirement its others make good (a quest required gives the item now) is dropped:
        # the three tablets' note makes Three Tablets for Crikston's own needless
        for r in self.requires:
            for a in list(r["add"]):
                rest = [x["group"] for x in r["add"] if x is not a]
                self.require(r["qid"], rest, replace=True)
                if a["item"] in d.sure(r["qid"]):
                    r["add"].remove(a)
                else:
                    self.require(r["qid"], rest + [a["group"]], replace=True)
        self.requires = [r for r in self.requires if r["add"]]

    def require(self, qid, gs, replace=False):
        """The plan's requirements of a quest in the data it looks at (added, or in place of
        what the plan added before), the derived sets read again."""
        q = self.d.quests[qid]
        q.setdefault("own_groups", list(q["groups"]))
        q["groups"] = (list(q["own_groups"]) if replace else q["groups"]) + [list(g) for g in gs]
        self.d._sure, self.d._closure = {}, {}

    def needs(self, qid):
        """What a quest's give and show steps want that only quests it does not require give:
        [dict(group, item, step)]."""
        d = self.d
        q = d.quests[qid]
        add = []
        for st in q["steps"]:
            if st["kind"] not in ("give", "show"):
                continue
            for pid, n in st["items"]:
                given, held = d.held_at(q, pid, st["step"], self.kept)
                if held >= n or d.other.get(pid) or pid in d.sure(qid):
                    continue
                srcs = sorted(g for g in d.givers.get(pid, ()) if g != qid)
                if given >= n:
                    continue  # a give before took it: KEEP's, or said there
                if not srcs:
                    later = [x for x in d.own(q) if x[1] == pid]
                    self.reports.append(f"quest {qid} {q['name']!r} step {st['step']} ({st['kind']} at "
                                        f"{st['target']!r}) wants {d.name(pid)!r} ({pid}): "
                                        + ("only a later step of its own gives it" if later else "nothing gives it"))
                    continue
                alts = [g for g in srcs if d.may_share(qid, g)]
                cycle = [g for g in alts if qid in d.closure(g) or g in d.closure(qid)]
                alts = [g for g in alts if g not in cycle]
                if not alts:
                    self.reports.append(f"quest {qid} {q['name']!r} step {st['step']} wants {d.name(pid)!r}, which "
                                        f"quests {srcs} give, none of them one its classes and races may take first")
                    continue
                if any(set(alts) <= set(g) or set(g) <= set(alts) for g in q["groups"] + [x["group"] for x in add]):
                    continue
                drafts = [g for g in alts if not d.quests[g]["enabled"]]
                if len(drafts) == len(alts):
                    self.reports.append(f"quest {qid} {q['name']!r}: every quest giving its {d.name(pid)!r} "
                                        f"({alts}) is a draft (required all the same)")
                add.append(dict(group=alts, item=pid, step=st["step"]))
        return add

    # -- RACES

    def plan_races(self):
        d = self.d
        table = collections.defaultdict(set)
        if has_table(self.db, "defaultcharacter"):
            for c, r, h in self.db.execute("SELECT tclass, race, humType FROM defaultcharacter"):
                city = home_city(r, h, c)
                if city:
                    table[(city, c)].add(r)
                    if r == 0 and h == 1:
                        table[(city, c)].add(FROGLOK)
        self.table = {k: sorted(v) for k, v in table.items()}
        for qid in sorted(d.quests):
            if qid not in d.keys:
                continue
            m = CITY_KEY.match(d.keys[qid][0])
            if not m or m.group(2) not in CLASSES:
                continue
            q = d.quests[qid]
            if q["races"]:
                continue
            c = CLASSES.index(m.group(2))
            races = self.table.get((m.group(1), c))
            if not races:
                self.reports.append(f"quest {qid} {q['name']!r} ({d.keys[qid][0]}): no race starts in "
                                    f"{m.group(1)} as a {m.group(2)}: left open to every race")
                continue
            self.races.append(dict(qid=qid, city=m.group(1), cls=c, races=races))

    # -- NPCS

    def plan_npcs(self):
        db, d = self.db, self.d
        if not has_table(db, "gm_audit"):
            return
        have = {fold(r[0]) for r in db.execute("SELECT npc_name FROM npcs")}
        wanted = collections.defaultdict(set)
        for q in d.quests.values():
            if q["enabled"] and fold(q["giver"]) not in have:
                wanted[fold(q["giver"])].add(q["qid"])
            for st in q["steps"]:
                if st["kind"] in NPC_KINDS + ("kill", "loot") and st["target"] and fold(st["target"]) not in have \
                        and q["enabled"]:
                    wanted[fold(st["target"])].add(q["qid"])
        cols = [r[1] for r in db.execute("PRAGMA table_info(npcs)")]
        for aid, before in db.execute("SELECT id, before FROM gm_audit WHERE tbl = 'npcs' AND action = 'delete' "
                                      "AND reverted_by IS NULL ORDER BY id"):
            try:
                row = json.loads(before or "")
            except ValueError:
                continue
            if not isinstance(row, dict) or fold(row.get("npc_name")) not in wanted:
                continue
            if db.execute("SELECT count(*) FROM npcs WHERE id = ?", (row.get("id"),)).fetchone()[0]:
                self.reports.append(f"{row.get('npc_name')!r} (audit {aid}): its id {row.get('id')} is another NPC's now")
                continue
            if set(row) - set(cols):
                self.reports.append(f"{row.get('npc_name')!r} (audit {aid}): columns npcs lacks {sorted(set(row) - set(cols))}")
                continue
            name = fold(row["npc_name"])
            if any(fold(n["row"]["npc_name"]) == name for n in self.npcs):
                continue  # one spawn of the name is enough
            self.npcs.append(dict(name=row["npc_name"], audit=aid, row=row, quests=sorted(wanted[name])))

    # -- reported

    def plan_stacks(self):
        d = self.d
        for qid in sorted(d.quests):
            q = d.quests[qid]
            for st in q["steps"]:
                want = list(st["items"][:1]) if st["kind"] == "loot" else list(st["items"]) if st["kind"] in ("give", "show") else []
                for pid, n in want:
                    if st["kind"] == "loot":
                        n = max(st["count"] or 0, n)
                    ms = d.item.get(pid, {}).get("maxstack", 1)
                    if ms >= 2 and n > ms:
                        self.reports.append(f"quest {qid} {q['name']!r} step {st['step']} wants {n} of {d.name(pid)!r}, "
                                            f"which stacks to {ms}: CLIENT keeps one record of it")


# ---- writing


def log(db, tbl, rid, col, old, new, why=""):
    db.execute(f"INSERT INTO {LOG} (tbl, rid, col, old, new, why) VALUES (?, ?, ?, ?, ?, ?)",
               (tbl, str(rid), col, None if old is None else json.dumps(old), None if new is None else json.dumps(new), why))


def apply(db, p, now=None):
    """Write the plan (the caller's transaction): counts."""
    db.execute(LOG_DDL)
    n = collections.Counter()
    clean = []
    if has_table(db, "quest_import_quests"):
        clean = [(k, qid, dg) for k, qid, dg in db.execute("SELECT key, questid, digest FROM quest_import_quests")
                 if iq.digest(db, qid) == dg]
    for r in p.requires:
        old = db.execute("SELECT requires FROM quests WHERE questid = ?", (r["qid"],)).fetchone()[0]
        gs = groups(old) + [a["group"] for a in r["add"]]
        new = format_groups(gs)
        db.execute("UPDATE quests SET requires = ? WHERE questid = ?", (new, r["qid"]))
        log(db, "quests", r["qid"], "requires", old, new,
            "; ".join(f"its {p.d.name(a['item'])} (step {a['step']}) only these give" for a in r["add"]))
        n["requires"] += 1
    for k in p.keeps:
        cur = db.execute("SELECT kind FROM queststeps WHERE questid = ? AND step = ?", (k["qid"], k["step"])).fetchone()
        if cur is None or cur[0] != "give":
            raise SystemExit(f"quest {k['qid']} step {k['step']} changed under the plan")
        db.execute("UPDATE queststeps SET kind = 'show' WHERE questid = ? AND step = ?", (k["qid"], k["step"]))
        log(db, "queststeps", f"{k['qid']}/{k['step']}", "kind", "give", "show", k["why"])
        n["keeps"] += 1
    for r in p.races:
        old = db.execute("SELECT races FROM quests WHERE questid = ?", (r["qid"],)).fetchone()[0]
        if ints(old):
            raise SystemExit(f"quest {r['qid']} has races now: changed under the plan")
        new = ", ".join(map(str, r["races"]))
        db.execute("UPDATE quests SET races = ? WHERE questid = ?", (new, r["qid"]))
        log(db, "quests", r["qid"], "races", old, new, f"the races that start in {r['city']} as a {CLASSES[r['cls']]}")
        n["races"] += 1
    when = now or datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    def counter(t):
        r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = ?", (t,)).fetchone()
        return None if r is None else r[0]
    counters = {t: counter(t) for t in ("npcs", "gm_audit")} if p.npcs else {}
    for x in p.npcs:
        row = x["row"]
        cols = list(row)
        db.execute(f"INSERT INTO npcs ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [row[c] for c in cols])
        log(db, "npcs", row["id"], "*", None, row, f"quest {', '.join(map(str, x['quests']))}'s NPC")
        detail = (f"restored {x['name']} (audit {x['audit']}, a GM's delete): quest "
                  f"{', '.join(map(str, x['quests']))} talks to it ({STAMP})")
        cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                         "VALUES (?, 0, 'dab''s sessions', 'tool', '', 'revert', 'npcs', ?, NULL, ?, ?)",
                         (when, json.dumps({"id": row["id"]}, separators=(",", ":")), json.dumps(row, separators=(",", ":")),
                          detail))
        log(db, "gm_audit", cur.lastrowid, "*", None, {"detail": detail}, "the restore's audit line")
        db.execute("UPDATE gm_audit SET reverted_by = ? WHERE id = ?", (cur.lastrowid, x["audit"]))
        log(db, "gm_audit", x["audit"], "reverted_by", None, cur.lastrowid, "the delete, reverted")
        n["npcs"] += 1
    for t, was in counters.items():
        # the tables' counters, put back at --undo so the database is as it was
        if counter(t) != was:
            log(db, "sqlite_sequence", t, "seq", was, counter(t), "the rows' ids")
    for k, qid, dg in clean:
        now_dg = iq.digest(db, qid)
        if now_dg != dg:
            db.execute("UPDATE quest_import_quests SET digest = ? WHERE key = ?", (now_dg, k))
            log(db, "quest_import_quests", k, "digest", dg, now_dg, "the quest changed here and in its file")
            n["digests"] += 1
    return n


def undo(db):
    """Put back every value still as --apply wrote it, newest first: (put back, left)."""
    if not has_table(db, LOG):
        return 0, []
    back, left = 0, []
    for seq, tbl, rid, col, old, new in db.execute(f"SELECT seq, tbl, rid, col, old, new FROM {LOG} ORDER BY seq DESC").fetchall():
        old = json.loads(old) if old is not None else None
        new = json.loads(new) if new is not None else None
        if tbl == "quests":
            cur = db.execute(f"UPDATE quests SET {col} = ? WHERE questid = ? AND {col} = ?", (old, int(rid), new))
        elif tbl == "queststeps":
            q, st = rid.split("/")
            cur = db.execute("UPDATE queststeps SET kind = ? WHERE questid = ? AND step = ? AND kind = ?", (old, int(q), int(st), new))
        elif tbl == "quest_import_quests":
            cur = db.execute("UPDATE quest_import_quests SET digest = ? WHERE key = ? AND digest = ?", (old, rid, new))
        elif tbl == "npcs":
            have = db.execute("SELECT * FROM npcs WHERE id = ?", (int(rid),)).fetchone()
            cols = [r[1] for r in db.execute("PRAGMA table_info(npcs)")]
            now = dict(zip(cols, have)) if have else None
            if now is not None and all(now.get(c) == v for c, v in new.items()):
                cur = db.execute("DELETE FROM npcs WHERE id = ?", (int(rid),))
            else:
                left.append(f"npcs {rid}: {'gone' if now is None else 'changed since'}")
                continue
        elif tbl == "gm_audit" and col == "*":
            cur = db.execute("DELETE FROM gm_audit WHERE id = ? AND detail = ?", (int(rid), new["detail"]))
        elif tbl == "gm_audit" and col == "reverted_by":
            cur = db.execute("UPDATE gm_audit SET reverted_by = NULL WHERE id = ? AND reverted_by = ?", (int(rid), new))
        elif tbl == "sqlite_sequence":
            if old is None:
                cur = db.execute("DELETE FROM sqlite_sequence WHERE name = ? AND seq = ?", (rid, new))
            else:
                cur = db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = ? AND seq = ?", (old, rid, new))
        else:
            left.append(f"{tbl} {rid}: not one of this tool's")
            continue
        if cur.rowcount:
            back += 1
        else:
            left.append(f"{tbl} {rid} {col}: changed since")
    db.execute(f"DROP TABLE {LOG}")
    return back, left


# ---- the quest files


def load_file(path):
    """A quest file's text and data, and the indent it was dumped with; None when the file is not
    exactly what json.dump writes of it (its layout would not be kept)."""
    with open(path, encoding="utf-8") as f:
        text = f.read()
    doc = json.loads(text)
    for ind in (1, 2, 4):
        if json.dumps(doc, indent=ind, ensure_ascii=False) + "\n" == text:
            return text, doc, ind
    return text, doc, None


def write_data(datadir, p):
    """The plan's changes in the quest files: changed quests counted by file; problems said."""
    d = p.d
    edits = collections.defaultdict(lambda: collections.defaultdict(list))  # file -> key -> [(what, ...)]
    problems = []
    for r in p.requires:
        if not p.in_files(r["qid"]):
            problems.append(f"quest {r['qid']}: in no quest file (its requires written here alone)")
            continue
        key, f = d.keys[r["qid"]]
        for a in r["add"]:
            ks = [d.keys[g][0] for g in a["group"] if g in d.keys]
            if len(ks) != len(a["group"]):
                problems.append(f"quest {r['qid']}: a quest it now requires is in no file ({a['group']})")
                continue
            edits[f][key].append(("requires", ks[0] if len(ks) == 1 else ks))
    for k in p.keeps:
        if p.in_files(k["qid"]):
            key, f = d.keys[k["qid"]]
            edits[f][key].append(("show", k["step"], d.name(k["item"])))
    for r in p.races:
        key, f = d.keys[r["qid"]]
        edits[f][key].append(("races", r["races"]))
    changed = collections.Counter()
    for fname in sorted(edits):
        path = os.path.join(datadir, fname)
        text, doc, ind = load_file(path)
        if ind is None:
            problems.append(f"{fname}: not laid out as json.dump writes it: left")
            continue
        by_key = {q.get("key"): q for q in doc.get("quests", [])}
        for key, es in edits[fname].items():
            q = by_key.get(key)
            if q is None:
                problems.append(f"{fname}: no quest {key!r}")
                continue
            for e in es:
                if e[0] == "requires":
                    q.setdefault("requires", [])
                    if e[1] not in q["requires"]:
                        q["requires"].append(e[1])
                elif e[0] == "show":
                    _, step, item = e
                    steps = q.get("steps") or []
                    st = steps[step - 1] if 0 < step <= len(steps) else None
                    names = [fold(it.get("item")) for it in (st or {}).get("items") or [] if isinstance(it, dict)]
                    if st is None or st.get("kind") not in ("give", "show") or fold(item) not in names:
                        problems.append(f"{fname} {key}: step {step} is not the give of {item!r} the database has")
                        continue
                    st["kind"] = "show"
                elif e[0] == "races":
                    if q.get("races"):
                        problems.append(f"{fname} {key}: has races {q['races']} in the file")
                        continue
                    q["races"] = e[1]
            changed[fname] += 1
        out = json.dumps(doc, indent=ind, ensure_ascii=False) + "\n"
        with open(path, "w", encoding="utf-8") as fh:
            fh.write(out)
    return changed, problems


# ---- the report


def report(p):
    d = p.d
    out = []
    for r in p.requires:
        q = d.quests[r["qid"]]
        for a in r["add"]:
            names = [f"{g} {d.quests[g]['name']!r}" for g in a["group"]]
            out.append(f"REQUIRES {r['qid']} {q['name']!r}: step {a['step']} wants {d.name(a['item'])!r} ({a['item']}), "
                       f"which only {' or '.join(names)} give{'s' if len(names) == 1 else ''}")
    for k in p.keeps:
        q = d.quests[k["qid"]]
        out.append(f"KEEP {k['qid']} {q['name']!r} step {k['step']}: {k['why']}")
    if p.races:
        out.append(f"RACES {len(p.races)} city class quests: the races that start in their city as their class")
        cities = collections.defaultdict(list)
        for r in p.races:
            cities[(r["city"], r["cls"])].append(r["qid"])
        for (city, c), qids in sorted(cities.items()):
            out.append(f"  {city:16} {CLASSES[c]:13} {', '.join(RACE_NAMES[x] for x in p.table[(city, c)]):40} "
                       f"{len(qids)} quests")
    for x in p.npcs:
        row = x["row"]
        out.append(f"NPCS {x['name']!r}: put back (npcs {row['id']}, world {row.get('world')} at "
                   f"{row.get('x_coord')}, {row.get('y_coord')}, {row.get('z_coord')}), deleted in the GM panel "
                   f"(audit {x['audit']}); quest {', '.join(map(str, x['quests']))}")
    for r in p.reports:
        out.append("LEFT " + r)
    out.append(f"{sum(len(r['add']) for r in p.requires)} requirements on {len(p.requires)} quests, {len(p.keeps)} steps "
               f"kept, {len(p.races)} quests given races, {len(p.npcs)} NPCs put back, {len(p.reports)} left for a person")
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0], formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", default="eqoa.db")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write it")
    g.add_argument("--undo", action="store_true", help="put back what --apply wrote")
    ap.add_argument("--write-data", metavar="DIR", help="make the same changes in the quest files of DIR")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 10000")
    if a.undo:
        db.execute("BEGIN IMMEDIATE")
        try:
            back, left = undo(db)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        for x in left:
            print("left:", x)
        print(f"put back {back}; {len(left)} left as they are now; restart the server")
        return
    if a.apply and has_table(db, LOG) and db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0]:
        sys.exit(f"{LOG} holds an earlier apply: --undo it first")
    p = Plan(db).build()
    for line in report(p):
        print(line)
    if a.write_data:
        changed, problems = write_data(a.write_data, p)
        for x in problems:
            print("PROBLEM (files):", x)
        print("quest files: " + ", ".join(f"{f} {n}" for f, n in sorted(changed.items())))
    if not a.apply:
        print("report only: --apply to write")
        return
    db.execute("BEGIN IMMEDIATE")
    try:
        n = apply(db, p)
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"wrote {n['requires']} quests' requires, {n['keeps']} steps kept, {n['races']} quests' races, {n['npcs']} NPCs "
          f"put back, {n['digests']} import digests; restart the server (with the code that reads a requires group)")


if __name__ == "__main__":
    main()
