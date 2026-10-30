#!/usr/bin/env python3
"""Quest content files (tools/quests/data/<city>.json, tools/quests/FORMAT.md)
into the quests and queststeps the server runs (internal/handlers/quests.go,
questevents.go): every name resolved against this database, a quest with
anything missing written as a draft (enabled 0, the misses in its notes).

    tools/import_quests.py [--db eqoa.db] [FILE ...]      report (every file of tools/quests/data by default)
    tools/import_quests.py --apply [FILE ...]             write them
    tools/import_quests.py --undo                         take back everything an import wrote

Names are matched as a player would read them: case, apostrophes and spaces
aside ("Nanik's Eye" is "naniks eye"), and an item also by its name less a
plural's s when that names one item alone. NPCs and creatures are npcs.npc_name
(written as this database spells them, which is what the engine matches),
items itempattern.itemname, spells spellpattern.spellname (the quest's class's
own when classspells has one of the name), factions factions.name, a visual
a DictID or a spell's spelleffect.fxdict. A file's quest items (its "items")
are made where no item has the name, as a copy of their "like" item with
their name and description (trade 1 when notrade: CLIENT's NO TRADE). A quest
whose required quest is a draft is one too.

The later guides' fields (FORMAT.md): a quest's factions_required (factions
by name: one this database lacks makes it a draft), repeatable and unlocks (a
travel NPC's trip, its NPC by name and its place matched to a route of
internal/handlers/waysender.go: one naming no route is said, and written all
the same, to open the trip once the route is there), a step's rewards by
class (byclass: items, the class's own spells, experience and tunar), a
talk, show or give step's choice (choices, its question choose: each
option's answer, words, rewards, and whether it stays).

A quest's requires are keys, each finished first; a list among them is a
group any one of which will do (a token each class gets from its own quest),
written "a, b|c" in the requires column (the engine's database.ParseRequires).

Quests are kept by their key: their ids are logged in quest_import_quests
(from 100001, past every quest there is), so a run again rewrites what an
earlier one wrote, unless a GM has changed it since (its rows' digest no
longer the logged one: left as it is, and said). --undo deletes every quest,
step and item an import wrote, the characters' progress in those quests and
the items made in their inventories, and the log; the item counter is put
back; the trips they unlock go with them. A copy of the database is made first (--no-backup to skip). Stop the
server to apply or undo, start it after: the quest engine reads the quests
at its start.
"""
import argparse
import datetime
import glob
import hashlib
import json
import os
import re
import sqlite3
import sys

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "quests"))
import routes as travel  # noqa: E402  (tools/quests/routes.py)

DEFAULT_DB = "eqoa.db"
DEFAULT_DATA = os.path.join(os.path.dirname(os.path.abspath(__file__)), "quests", "data")
FIRST_ID = 100001
JOURNAL_CHARS, ANSWER_CHARS = 127, 63

CLASSES = {
    "warrior": 0, "ranger": 1, "paladin": 2, "shadowknight": 3, "shadow knight": 3, "monk": 4, "bard": 5,
    "rogue": 6, "druid": 7, "shaman": 8, "cleric": 9, "magician": 10, "necromancer": 11, "enchanter": 12,
    "wizard": 13, "alchemist": 14,
}
KINDS = ("talk", "show", "give", "kill", "loot", "reach")
NPC_KINDS = ("talk", "show", "give")
QUEST_WHENS, STEP_WHENS = ("accept", "finish"), ("start", "done")
DOS = ("popup", "say", "give", "spawn", "visual", "teleport", "vampire")

# The quest tables as internal/database/quests.go makes them, and the columns
# it adds to an older one.
QUEST_DDL = """
CREATE TABLE IF NOT EXISTS quests (
	questid INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 0,
	giver TEXT NOT NULL DEFAULT '', minlevel INTEGER NOT NULL DEFAULT 1, maxlevel INTEGER NOT NULL DEFAULT 60,
	classes TEXT NOT NULL DEFAULT '', races TEXT NOT NULL DEFAULT '', requires TEXT NOT NULL DEFAULT '',
	repeatable INTEGER NOT NULL DEFAULT 0, offer TEXT NOT NULL DEFAULT '', accept TEXT NOT NULL DEFAULT '',
	decline TEXT NOT NULL DEFAULT '', declined TEXT NOT NULL DEFAULT '', started TEXT NOT NULL DEFAULT '',
	source TEXT NOT NULL DEFAULT '', notes TEXT NOT NULL DEFAULT '', events TEXT NOT NULL DEFAULT '',
	factionreq TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS queststeps (
	questid INTEGER NOT NULL, step INTEGER NOT NULL, kind TEXT NOT NULL DEFAULT 'talk',
	target TEXT NOT NULL DEFAULT '', npcid INTEGER NOT NULL DEFAULT 0, items TEXT NOT NULL DEFAULT '',
	tunar INTEGER NOT NULL DEFAULT 0, count INTEGER NOT NULL DEFAULT 1, world INTEGER NOT NULL DEFAULT 0,
	x REAL NOT NULL DEFAULT 0, y REAL NOT NULL DEFAULT 0, z REAL NOT NULL DEFAULT 0, radius REAL NOT NULL DEFAULT 15,
	journal TEXT NOT NULL DEFAULT '', answer TEXT NOT NULL DEFAULT '', says TEXT NOT NULL DEFAULT '',
	waiting TEXT NOT NULL DEFAULT '', notyet TEXT NOT NULL DEFAULT '', reminder TEXT NOT NULL DEFAULT '',
	xp INTEGER NOT NULL DEFAULT 0, rewardtunar INTEGER NOT NULL DEFAULT 0, rewarditems TEXT NOT NULL DEFAULT '',
	factions TEXT NOT NULL DEFAULT '', spells TEXT NOT NULL DEFAULT '', chance REAL NOT NULL DEFAULT 0,
	events TEXT NOT NULL DEFAULT '', byclass TEXT NOT NULL DEFAULT '', choices TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (questid, step));
CREATE TABLE IF NOT EXISTS questunlocks (
	questid INTEGER NOT NULL, npc TEXT NOT NULL, dest TEXT NOT NULL, PRIMARY KEY (questid, npc, dest));
CREATE TABLE IF NOT EXISTS charquests (
	serverid INTEGER NOT NULL, questid INTEGER NOT NULL, step INTEGER NOT NULL DEFAULT 1,
	count INTEGER NOT NULL DEFAULT 0, done INTEGER NOT NULL DEFAULT 0, slot INTEGER NOT NULL DEFAULT 0,
	updated TEXT NOT NULL DEFAULT '', PRIMARY KEY (serverid, questid));
"""
ADDED_COLUMNS = [("quests", "declined", "TEXT NOT NULL DEFAULT ''"), ("queststeps", "notyet", "TEXT NOT NULL DEFAULT ''"),
                 ("queststeps", "reminder", "TEXT NOT NULL DEFAULT ''"), ("quests", "events", "TEXT NOT NULL DEFAULT ''"),
                 ("queststeps", "events", "TEXT NOT NULL DEFAULT ''"), ("queststeps", "chance", "REAL NOT NULL DEFAULT 0"),
                 ("quests", "factionreq", "TEXT NOT NULL DEFAULT ''"), ("queststeps", "byclass", "TEXT NOT NULL DEFAULT ''"),
                 ("queststeps", "choices", "TEXT NOT NULL DEFAULT ''")]

LOG_DDL = [
    "CREATE TABLE IF NOT EXISTS quest_import_quests (key TEXT PRIMARY KEY, questid INTEGER NOT NULL, file TEXT NOT NULL, "
    "digest TEXT NOT NULL, enabled INTEGER NOT NULL)",
    "CREATE TABLE IF NOT EXISTS quest_import_items (patternid INTEGER PRIMARY KEY, name TEXT NOT NULL, file TEXT NOT NULL)",
    "CREATE TABLE IF NOT EXISTS quest_import_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)",
]

QUEST_COLS = ["questid", "name", "enabled", "giver", "minlevel", "maxlevel", "classes", "races", "requires", "repeatable",
              "offer", "accept", "decline", "declined", "started", "source", "notes", "events", "factionreq"]
STEP_COLS = ["questid", "step", "kind", "target", "npcid", "items", "tunar", "count", "world", "x", "y", "z", "radius",
             "journal", "answer", "says", "waiting", "notyet", "reminder", "xp", "rewardtunar", "rewarditems", "factions",
             "spells", "chance", "events", "byclass", "choices"]
# the columns added after imports had logged their digests: a quest that
# leaves them empty digests as it did before them (digest), so a run again
# still knows an earlier import's quest as its own
LATER_COLS = {"factionreq", "byclass", "choices"}
FACTION_MIN, FACTION_MAX = -2000, 2000   # entity/faction.go's FactionMin, FactionMax


def fold(s):
    """A name as it is matched: lower case, apostrophes gone, one space."""
    s = str(s or "").lower().replace("’", "'").replace("`", "'").replace("'", "")
    return " ".join(re.sub(r"[^\w\s-]", " ", s).split())


def has_table(db, t):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone()[0] > 0


def columns(db, t):
    return [r[1] for r in db.execute(f"PRAGMA table_info({t})")]


def ensure_quest_tables(db):
    # statement by statement: executescript would commit the caller's transaction
    for stmt in QUEST_DDL.split(";"):
        if stmt.strip():
            db.execute(stmt)
    for t, c, decl in ADDED_COLUMNS:
        if c not in columns(db, t):
            db.execute(f"ALTER TABLE {t} ADD COLUMN {c} {decl}")


class Names:
    """What the database holds, by folded name."""

    def __init__(self, db):
        self.npcs = {}
        spell_counts = {}
        for (name,) in db.execute("SELECT npc_name FROM npcs WHERE npc_name IS NOT NULL AND npc_name != ''"):
            k = fold(name)
            spell_counts.setdefault(k, {}).setdefault(name, 0)
            spell_counts[k][name] += 1
        for k, spellings in spell_counts.items():
            self.npcs[k] = max(sorted(spellings), key=lambda n: spellings[n])
        self.items = {}
        for pid, name in db.execute("SELECT patternid, itemname FROM itempattern ORDER BY patternid"):
            self.items.setdefault(fold(name), []).append((pid, name))
        self.spells = {}
        for sid, name in db.execute("SELECT spellid, spellname FROM spellpattern ORDER BY spellid"):
            self.spells.setdefault(fold(name), []).append(sid)
        self.class_spells = set()
        if has_table(db, "classspells"):
            self.class_spells = {(c, s) for c, s in db.execute("SELECT class, spellid FROM classspells")}
        self.factions = {}
        if has_table(db, "factions"):
            self.factions = {fold(n): n for (n,) in db.execute("SELECT name FROM factions")}
        self.fx = {}
        if has_table(db, "spelleffect"):
            self.fx = {s: fx for s, fx in db.execute("SELECT spellid, fxdict FROM spelleffect WHERE fxdict != 0")}

    def npc(self, name):
        return self.npcs.get(fold(name))

    def item(self, name):
        k = fold(name)
        if k in self.items:
            return self.items[k][0]
        if k.endswith("s") and k[:-1] in self.items:
            return self.items[k[:-1]][0]
        if k + "s" in self.items:
            return self.items[k + "s"][0]
        return None

    def spell(self, name, classes):
        ids = self.spells.get(fold(name)) or []
        for c in classes:
            for sid in ids:
                if (c, sid) in self.class_spells:
                    return sid
        return ids[0] if ids else None

    def faction(self, name):
        return self.factions.get(fold(name))

    def visual(self, fx, classes):
        """A visual's DictID as the events column keeps it: hex as given, or a spell's."""
        s = str(fx or "").strip()
        if re.fullmatch(r"(0[xX][0-9a-fA-F]{1,8}|[0-9a-fA-F]{8})", s):
            v = int(s, 16)
            return f"0x{v:08X}" if v else None
        sid = self.spell(s, classes) if s else None
        if sid is not None and self.fx.get(sid):
            return f"0x{self.fx[sid] & 0xFFFFFFFF:08X}"
        return None


def item_list(v):
    """A file's items: [{"item", "count"}] (or names) as [(name, count)]."""
    out = []
    for it in v or []:
        if isinstance(it, str):
            out.append((it, 1))
        elif isinstance(it, dict):
            out.append((it.get("item") or it.get("name") or "", max(int(it.get("count") or 1), 1)))
    return out


def requires_of(q):
    """A quest's requires: each a key, or a group of keys any one of which
    will do (a list), as the requires column writes them ("a, b|c")."""
    out = []
    for r in q.get("requires") or []:
        if isinstance(r, list):
            g = [str(k).strip() for k in r if str(k).strip()]
            if len(g) > 1:
                out.append(g)
            elif g:
                out.append(g[0])
        elif str(r).strip():
            out.append(str(r).strip())
    return out


def class_ids(v):
    out = []
    for c in v or []:
        if isinstance(c, int):
            out.append(c)
        elif str(c).strip().isdigit():
            out.append(int(c))
        elif fold(c) in CLASSES:
            out.append(CLASSES[fold(c)])
    return out


def counts(pairs):
    return ", ".join(str(p) if n == 1 else f"{p}*{n}" for p, n in pairs)


def load_files(paths):
    out = []
    for p in paths:
        with open(p) as f:
            d = json.load(f)
        out.append((os.path.basename(p), d))
    return out


def plan(db, files, routes=False):
    """What an import would write: each quest's rows, enabled or a draft and why,
    the items to make, and warnings. routes are the travel NPCs' routes the
    unlocks are matched to (tools/quests/routes.py; by default waysender.go's,
    None: not checked)."""
    names = Names(db)
    made, made_by_fold = [], {}  # items to make
    warnings = []
    if routes is False:
        routes = travel.travel_routes()
        if routes is None:
            warnings.append(f"no {travel.WAYSENDER}: the unlocks' routes are not checked")
    for fname, d in files:
        for it in d.get("items") or []:
            name = (it.get("name") or "").strip()
            if not name or names.item(name) or fold(name) in made_by_fold:
                continue
            like = names.item(it.get("like") or "")
            if like is None:
                warnings.append(f"{fname}: item {name!r} cannot be made: its like {it.get('like')!r} is not here")
                continue
            m = dict(name=name, description=it.get("description") or "", notrade=bool(it.get("notrade")),
                     like=like[0], file=fname)
            made.append(m)
            made_by_fold[fold(name)] = m

    def item_ref(name):
        """(patternid or a made item's name, or None)."""
        got = names.item(name)
        if got:
            return got[0]
        m = made_by_fold.get(fold(name))
        return ("made", m["name"]) if m else None

    quests = []
    for fname, d in files:
        for q in d.get("quests") or []:
            key = (q.get("key") or "").strip()
            if not key:
                warnings.append(f"{fname}: a quest has no key ({q.get('name')!r}): left out")
                continue
            miss = []
            classes = class_ids(q.get("classes"))
            giver = names.npc(q.get("giver"))
            if not giver:
                miss.append(f"NPC {q.get('giver')!r}")
            # the standings it asks for: a faction this database lacks keeps it out
            reqs = []
            for fr in q.get("factions_required") or []:
                shaped = isinstance(fr, (list, tuple)) and len(fr) in (2, 3) and isinstance(fr[0], str) and \
                    all(isinstance(v, int) and not isinstance(v, bool) for v in fr[1:])
                if not shaped or (len(fr) == 3 and fr[2] < fr[1]):
                    miss.append(f"faction requirement {fr!r} (not [faction, min] or [faction, min, max])")
                    continue
                fn = names.faction(fr[0])
                if fn is None:
                    miss.append(f"faction {fr[0]!r} (required)")
                    continue
                reqs.append(f"{fn}:{fr[1]}" + (f"..{fr[2]}" if len(fr) == 3 else ""))
            # the trips it unlocks: said when the NPC or the route is not here, written all the same
            unlocks = []
            for u in q.get("unlocks") or []:
                npc = (u.get("npc") or "").strip() if isinstance(u, dict) else ""
                to = (u.get("to") or "").strip() if isinstance(u, dict) else ""
                if not npc or not to:
                    warnings.append(f"{key}: an unlock without its npc and to ({u!r}): left out")
                    continue
                n = names.npc(npc)
                if not n:
                    warnings.append(f"{key}: it unlocks {npc!r}'s trip to {to!r}, and no NPC is named {npc!r}")
                r = travel.route_of(n or npc, to, routes) if routes is not None else None
                if r is None and routes is not None:
                    warnings.append(f"{key}: it unlocks {npc!r}'s trip to {to!r}, which is no route of waysender.go "
                                    "yet (written: it opens the trip once the route is there)")
                unlock = (r[0], r[1]) if r else (n or npc, to)
                if unlock not in unlocks:
                    unlocks.append(unlock)

            def events(evs, whens, where):
                out = []
                for e in evs or []:
                    on, do = (e.get("on") or "").strip().lower(), (e.get("do") or "").strip().lower()
                    if on not in whens or do not in DOS:
                        miss.append(f"{where} event {on}/{do} (not one the engine has)")
                        continue
                    ev = {"on": on, "do": do}
                    if do in ("popup", "say"):
                        ev["text"] = e.get("text") or ""
                        if do == "popup" and e.get("answer"):
                            ev["answer"] = e["answer"]
                        if do == "say" and e.get("npc"):
                            n = names.npc(e["npc"])
                            ev["npc"] = n or e["npc"]
                            if not n:
                                miss.append(f"NPC {e['npc']!r} ({where} say)")
                    elif do == "give":
                        refs = []
                        for nm, n in item_list(e.get("items")):
                            r = item_ref(nm)
                            if r is None:
                                miss.append(f"item {nm!r} ({where} give)")
                            else:
                                refs.append((r, n))
                        ev["items"] = refs
                    elif do == "spawn":
                        n = names.npc(e.get("npc"))
                        ev["npc"] = n or (e.get("npc") or "")
                        if not n:
                            miss.append(f"NPC {e.get('npc')!r} ({where} spawn)")
                        if e.get("hostile"):
                            ev["hostile"] = True
                    elif do == "visual":
                        fx = names.visual(e.get("fx"), classes)
                        if not fx:
                            miss.append(f"visual {e.get('fx')!r} ({where})")
                        ev["fx"] = fx or str(e.get("fx") or "")
                        if e.get("seconds"):
                            ev["seconds"] = float(e["seconds"])
                    elif do == "teleport":
                        for k in ("world", "x", "y", "z"):
                            if k in e:
                                ev[k] = e[k]
                    out.append(ev)
                return out

            qev = events(q.get("events"), QUEST_WHENS, "the quest's")
            steps = []
            for i, st in enumerate(q.get("steps") or [], 1):
                kind = (st.get("kind") or "").strip().lower()
                where = f"step {i}"
                if kind not in KINDS:
                    miss.append(f"{where} kind {kind!r}")
                    continue
                row = dict(step=i, kind=kind, target="", npcid=0, items=[], tunar=int(st.get("tunar") or 0),
                           count=int(st.get("count") or (1 if kind != "loot" else 0)), world=int(st.get("world") or 0),
                           x=float(st.get("x") or 0), y=float(st.get("y") or 0), z=float(st.get("z") or 0),
                           radius=float(st.get("radius") or 15), journal=st.get("journal") or "",
                           answer=st.get("answer") or "", says=st.get("says") or "", waiting=st.get("waiting") or "",
                           notyet=st.get("notyet") or "", reminder=st.get("reminder") or "",
                           chance=float(st.get("chance") or 0), xp=0, rewardtunar=0, rewarditems=[], factions="", spells="")
                if kind != "reach":
                    t = names.npc(st.get("target"))
                    row["target"] = t or (st.get("target") or "")
                    if not t:
                        miss.append(f"{'NPC' if kind in NPC_KINDS else 'creature'} {st.get('target')!r} ({where})")
                for nm, n in item_list(st.get("items")):
                    r = item_ref(nm)
                    if r is None:
                        miss.append(f"item {nm!r} ({where})")
                    else:
                        row["items"].append((r, n))
                if kind == "loot" and not row["items"]:
                    miss.append(f"{where}: a loot step with no item")
                rw = st.get("rewards") or {}
                row["xp"], row["rewardtunar"] = int(rw.get("xp") or 0), int(rw.get("tunar") or 0)
                for nm, n in item_list(rw.get("items")):
                    r = item_ref(nm)
                    if r is None:
                        miss.append(f"item {nm!r} ({where} reward)")
                    else:
                        row["rewarditems"].append((r, n))
                spells = []
                for nm in rw.get("spells") or []:
                    sid = names.spell(nm, classes)
                    if sid is None:
                        miss.append(f"spell {nm!r} ({where} reward)")
                    else:
                        spells.append(sid)
                row["spells"] = ", ".join(map(str, spells))
                facs = []
                for fct in rw.get("factions") or []:
                    nm, n = (fct[0], fct[1]) if isinstance(fct, (list, tuple)) else (fct.get("faction"), fct.get("delta"))
                    fn = names.faction(nm)
                    if fn is None:
                        miss.append(f"faction {nm!r} ({where} reward)")
                    elif int(n or 0):
                        facs.append(f"{fn}:{int(n):+d}")
                row["factions"] = ", ".join(facs)
                # the rewards for a player of one class, its spells that class's own
                byclass = {}
                for ck, cr in (rw.get("byclass") or {}).items():
                    cids = class_ids([ck])
                    if len(cids) != 1 or not 0 <= cids[0] <= max(CLASSES.values()) or not isinstance(cr, dict):
                        miss.append(f"{where} rewards for class {ck!r} (no such class)")
                        continue
                    cid = cids[0]
                    ent = dict(xp=int(cr.get("xp") or 0), tunar=int(cr.get("tunar") or 0), items=[], spells=[])
                    for nm, n in item_list(cr.get("items")):
                        r = item_ref(nm)
                        if r is None:
                            miss.append(f"item {nm!r} ({where} reward for class {cid})")
                        else:
                            ent["items"].append((r, n))
                    for nm in cr.get("spells") or []:
                        sid = names.spell(nm, [cid])
                        if sid is None:
                            miss.append(f"spell {nm!r} ({where} reward for class {cid})")
                        else:
                            ent["spells"].append(sid)
                    byclass[cid] = ent
                row["byclass"] = byclass
                # a choice asked when the step is done at its NPC: each option's rewards resolved as the step's are
                opts = []
                for k, o in enumerate(st.get("choices") or [], 1):
                    if not isinstance(o, dict) or not str(o.get("answer") or "").strip():
                        miss.append(f"{where} option {k} (no answer)")
                        continue
                    orw = o.get("rewards") or {}
                    ent = dict(answer=str(o["answer"]).strip(), says=o.get("says") or "", stay=bool(o.get("stay")),
                               xp=int(orw.get("xp") or 0), tunar=int(orw.get("tunar") or 0), items=[], spells=[])
                    for nm, n in item_list(orw.get("items")):
                        r = item_ref(nm)
                        if r is None:
                            miss.append(f"item {nm!r} ({where} option {k})")
                        else:
                            ent["items"].append((r, n))
                    for nm in orw.get("spells") or []:
                        sid = names.spell(nm, classes)
                        if sid is None:
                            miss.append(f"spell {nm!r} ({where} option {k})")
                        else:
                            ent["spells"].append(sid)
                    opts.append(ent)
                if opts and kind not in NPC_KINDS:
                    miss.append(f"{where}: a {kind} step asks no choice (talk, show and give steps do)")
                row["choices"] = dict(ask=st.get("choose") or "", options=opts) if opts else None
                row["events"] = events(st.get("events"), STEP_WHENS, where)
                if len(row["journal"]) > JOURNAL_CHARS:
                    warnings.append(f"{key} {where}: the journal line is {len(row['journal'])} characters (the client shows {JOURNAL_CHARS})")
                steps.append(row)
            if not steps:
                miss.append("no steps")
            for a in (q.get("accept") or "").splitlines() + [q.get("decline") or ""]:
                if len(a) > ANSWER_CHARS:
                    warnings.append(f"{key}: an answer is {len(a)} characters (the client shows {ANSWER_CHARS})")
            told = [str(u) for u in (q.get("unresolved") or []) if str(u).strip()]
            quests.append(dict(key=key, file=fname, q=q, classes=classes, giver=giver or (q.get("giver") or ""),
                               steps=steps, events=qev, miss=miss, told=told, factionreq=", ".join(reqs),
                               unlocks=unlocks, requires=requires_of(q)))
    # a quest needing one that is missing or a draft is a draft too; a quest a
    # GM has changed since its import (left as it is) counts as the database has it
    by_key = {x["key"]: x for x in quests}
    logged, gm_kept = {}, {}
    if has_table(db, "quest_import_quests"):
        for k, qid, en, dg in db.execute("SELECT key, questid, enabled, digest FROM quest_import_quests").fetchall():
            logged[k] = (qid, en)
            row = db.execute("SELECT enabled FROM quests WHERE questid = ?", (qid,)).fetchone()
            if row is not None and digest(db, qid) != dg:
                gm_kept[k] = bool(row[0])

    def lacks(r):
        """Why the quest of key r keeps one that requires it out, or None."""
        dep = by_key.get(r)
        if r in gm_kept:
            return None if gm_kept[r] else f"the quest {r!r} it requires, a draft (a GM's)"
        if dep is None and r not in logged:
            return f"the quest {r!r} it requires, which no file has"
        if dep is not None and dep.get("draft"):
            return f"the quest {r!r} it requires, a draft"
        return None

    changed = True
    while changed:
        changed = False
        for x in quests:
            if x.get("draft") or x["key"] in gm_kept:
                continue
            why = list(x["miss"]) + [f"the file's unresolved {u!r}" for u in x["told"]]
            for r in x["requires"]:
                # a group keeps it out only when every one of the group does
                group = r if isinstance(r, list) else [r]
                ws = [lacks(k) for k in group]
                if all(ws):
                    why.append(ws[0] if len(ws) == 1 else "every quest of a group it requires (one would do): " + "; ".join(ws))
            if why:
                x["draft"], x["why"] = True, why
                changed = True
    for x in quests:
        x.setdefault("draft", False)
        x.setdefault("why", [])
    return dict(quests=quests, made=made, warnings=warnings)


def report(p, verbose=False):
    by_file = {}
    for x in p["quests"]:
        f = by_file.setdefault(x["file"], [0, 0])
        f[0 if not x["draft"] else 1] += 1
    for fname, (whole, drafts) in sorted(by_file.items()):
        print(f"{fname}: {whole + drafts} quests, {whole} whole, {drafts} drafts")
    trips = sum(len(x["unlocks"]) for x in p["quests"])
    if trips:
        print(f"trips unlocked: {trips} (by {sum(1 for x in p['quests'] if x['unlocks'])} quests)")
    if p["made"]:
        print(f"items to make: {len(p['made'])} ({', '.join(m['name'] for m in p['made'][:12])}{' ...' if len(p['made']) > 12 else ''})")
    for w in p["warnings"]:
        print("warning:", w)
    for x in p["quests"]:
        if x["draft"] and verbose:
            print(f"  draft {x['key']}: {'; '.join(x['why'])}")


def digest(db, qid):
    """A quest's rows as one value, to tell a GM's change by: its row, its
    steps, the trips it unlocks and its kill and loot steps' other creatures
    (queststeptargets: a GM's or tools/fix_quest_targets.py's, which this
    import never writes). The LATER_COLS, the trips and the creatures count
    only when the quest has them, so a quest an import logged before they were
    there digests as it did."""
    h = hashlib.md5()
    for table, cols, order in (("quests", QUEST_COLS, ""), ("queststeps", STEP_COLS, " ORDER BY step")):
        have = set(columns(db, table))
        early = [c for c in cols if c not in LATER_COLS]
        later = [c for c in cols if c in LATER_COLS and c in have]
        for r in db.execute(f"SELECT {', '.join(early + later)} FROM {table} WHERE questid = ?{order}", (qid,)):
            h.update(repr(tuple(r[:len(early)])).encode())
            if any(v not in ("", None, 0) for v in r[len(early):]):
                h.update(repr(("later", tuple(r[len(early):]))).encode())
    if has_table(db, "questunlocks"):
        trips = db.execute("SELECT npc, dest FROM questunlocks WHERE questid = ? ORDER BY npc, dest", (qid,)).fetchall()
        if trips:
            h.update(repr(("unlocks", trips)).encode())
    if has_table(db, "queststeptargets"):
        others = db.execute("SELECT step, target FROM queststeptargets WHERE questid = ? ORDER BY step, target", (qid,)).fetchall()
        if others:
            h.update(repr(("targets", others)).encode())
    return h.hexdigest()


def step_choices(ch, pid):
    """A step's choice as the choices column keeps it (database.
    ParseStepChoices's form), "" for none."""
    if not ch:
        return ""
    opts = []
    for o in ch["options"]:
        ent = dict(answer=o["answer"])
        if o["says"]:
            ent["says"] = o["says"]
        if o["stay"]:
            ent["stay"] = True
        if o["xp"]:
            ent["xp"] = o["xp"]
        if o["tunar"]:
            ent["tunar"] = o["tunar"]
        if o["items"]:
            ent["items"] = counts((pid(r), n) for r, n in o["items"])
        if o["spells"]:
            ent["spells"] = ", ".join(map(str, o["spells"]))
        opts.append(ent)
    out = dict(ask=ch["ask"], options=opts) if ch["ask"] else dict(options=opts)
    return json.dumps(out, separators=(",", ":"))


def class_rewards(byclass, pid):
    """A step's rewards by class as the byclass column keeps them (database.
    FormatClassRewards's form: class ids as keys, items and spells written as
    the step's own columns have them), "" for none."""
    out = {}
    for cid in sorted(byclass, key=str):
        e = byclass[cid]
        ent = {}
        if e["xp"]:
            ent["xp"] = e["xp"]
        if e["tunar"]:
            ent["tunar"] = e["tunar"]
        if e["items"]:
            ent["items"] = counts((pid(r), n) for r, n in e["items"])
        if e["spells"]:
            ent["spells"] = ", ".join(map(str, e["spells"]))
        out[str(cid)] = ent
    return json.dumps(out, separators=(",", ":")) if out else ""


def write(db, p):
    """Write the plan: returns (written, left as a GM changed them, items made)."""
    db.execute("BEGIN IMMEDIATE")
    try:
        ensure_quest_tables(db)
        for stmt in LOG_DDL:
            db.execute(stmt)
        meta = dict(db.execute("SELECT key, value FROM quest_import_meta"))
        if "item_seq_before" not in meta:
            r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'itempattern'").fetchone()
            meta["item_seq_before"] = "" if r is None else str(r[0])
            meta["first"] = datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")
        meta["last"] = datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")
        # the items
        made_ids = {fold(n): pid for pid, n in db.execute("SELECT patternid, name FROM quest_import_items")}
        icols = [c for c in columns(db, "itempattern") if c != "patternid"]
        made = 0
        for m in p["made"]:
            if fold(m["name"]) in made_ids:
                continue
            row = dict(zip(columns(db, "itempattern"), db.execute("SELECT * FROM itempattern WHERE patternid = ?", (m["like"],)).fetchone()))
            row.update(itemname=m["name"], itemdesc=m["description"] or row["itemdesc"])
            if m["notrade"]:
                row["trade"] = 1
            cur = db.execute(f"INSERT INTO itempattern ({', '.join(icols)}) VALUES ({', '.join('?' * len(icols))})",
                             [row[c] for c in icols])
            db.execute("INSERT INTO quest_import_items VALUES (?, ?, ?)", (cur.lastrowid, m["name"], m["file"]))
            made_ids[fold(m["name"])] = cur.lastrowid
            made += 1

        def pid(ref):
            return made_ids[fold(ref[1])] if isinstance(ref, tuple) else ref

        def fix_events(evs):
            out = []
            for e in evs:
                e = dict(e)
                if e["do"] == "give":
                    e["items"] = counts((pid(r), n) for r, n in e["items"])
                out.append(e)
            return json.dumps(out, separators=(",", ":")) if out else ""

        # ids by key
        logged = {k: (qid, dg) for k, qid, dg in db.execute("SELECT key, questid, digest FROM quest_import_quests")}
        top = max(FIRST_ID - 1, db.execute("SELECT COALESCE(max(questid), 0) FROM quests").fetchone()[0],
                  max([q for q, _ in logged.values()] or [0]))
        ids = {}
        for x in p["quests"]:
            if x["key"] in logged:
                ids[x["key"]] = logged[x["key"]][0]
            else:
                top += 1
                ids[x["key"]] = top
        written, kept = 0, []
        for x in p["quests"]:
            qid, q = ids[x["key"]], x["q"]
            if x["key"] in logged:
                have = db.execute("SELECT count(*) FROM quests WHERE questid = ?", (qid,)).fetchone()[0]
                if have and digest(db, qid) != logged[x["key"]][1]:
                    kept.append(x["key"])
                    continue
                db.execute("DELETE FROM quests WHERE questid = ?", (qid,))
                db.execute("DELETE FROM queststeps WHERE questid = ?", (qid,))
                db.execute("DELETE FROM questunlocks WHERE questid = ?", (qid,))
                if has_table(db, "queststeptargets"):
                    db.execute("DELETE FROM queststeptargets WHERE questid = ?", (qid,))
            notes = q.get("notes") or ""
            if x["draft"]:
                notes = (notes + "\n\n" if notes else "") + "A draft until these are here: " + "; ".join(x["why"]) + "."
            requires = []
            for r in x["requires"]:
                group = [ids[k] if k in ids else logged[k][0] for k in (r if isinstance(r, list) else [r]) if k in ids or k in logged]
                if group:
                    requires.append("|".join(map(str, group)))
            vals = dict(questid=qid, name=q.get("name") or x["key"], enabled=0 if x["draft"] else 1, giver=x["giver"],
                        minlevel=int(q.get("minlevel") or 1), maxlevel=int(q.get("maxlevel") or 60),
                        classes=", ".join(map(str, x["classes"])), races=", ".join(map(str, class_ids(q.get("races")))),
                        requires=", ".join(requires), repeatable=1 if q.get("repeatable") else 0,
                        offer=q.get("offer") or "", accept=q.get("accept") or "", decline=q.get("decline") or "",
                        declined=q.get("declined") or "", started=q.get("started") or "",
                        source=f"EQOA-Data {x['key']} (tools/import_quests.py)", notes=notes, events=fix_events(x["events"]),
                        factionreq=x["factionreq"])
            db.execute(f"INSERT INTO quests ({', '.join(QUEST_COLS)}) VALUES ({', '.join('?' * len(QUEST_COLS))})",
                       [vals[c] for c in QUEST_COLS])
            for st in x["steps"]:
                row = dict(st, questid=qid, items=counts((pid(r), n) for r, n in st["items"]),
                           rewarditems=counts((pid(r), n) for r, n in st["rewarditems"]), events=fix_events(st["events"]),
                           byclass=class_rewards(st["byclass"], pid), choices=step_choices(st["choices"], pid))
                db.execute(f"INSERT INTO queststeps ({', '.join(STEP_COLS)}) VALUES ({', '.join('?' * len(STEP_COLS))})",
                           [row[c] for c in STEP_COLS])
            db.executemany("INSERT OR IGNORE INTO questunlocks (questid, npc, dest) VALUES (?, ?, ?)",
                           [(qid, npc, to) for npc, to in x["unlocks"]])
            db.execute("INSERT OR REPLACE INTO quest_import_quests VALUES (?, ?, ?, ?, ?)",
                       (x["key"], qid, x["file"], digest(db, qid), 0 if x["draft"] else 1))
            written += 1
        db.executemany("INSERT OR REPLACE INTO quest_import_meta VALUES (?, ?)", sorted(meta.items()))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return written, kept, made


def undo(db):
    if not has_table(db, "quest_import_quests"):
        sys.exit("no quest import to undo here")
    meta = dict(db.execute("SELECT key, value FROM quest_import_meta"))
    db.execute("BEGIN IMMEDIATE")
    try:
        qids = [r[0] for r in db.execute("SELECT questid FROM quest_import_quests")]
        pids = [r[0] for r in db.execute("SELECT patternid FROM quest_import_items")]
        n = dict(quests=0, steps=0, progress=0, items=0, held=0, trips=0, creatures=0)
        trips = has_table(db, "questunlocks")
        others = has_table(db, "queststeptargets")
        for qid in qids:
            n["quests"] += db.execute("DELETE FROM quests WHERE questid = ?", (qid,)).rowcount
            n["steps"] += db.execute("DELETE FROM queststeps WHERE questid = ?", (qid,)).rowcount
            n["progress"] += db.execute("DELETE FROM charquests WHERE questid = ?", (qid,)).rowcount
            if trips:
                n["trips"] += db.execute("DELETE FROM questunlocks WHERE questid = ?", (qid,)).rowcount
            if others:
                n["creatures"] += db.execute("DELETE FROM queststeptargets WHERE questid = ?", (qid,)).rowcount
        for p in pids:
            for t in ("charinventory", "npcitem"):
                if has_table(db, t):
                    n["held"] += db.execute(f"DELETE FROM {t} WHERE patternid = ?", (p,)).rowcount
            n["items"] += db.execute("DELETE FROM itempattern WHERE patternid = ?", (p,)).rowcount
        before = meta.get("item_seq_before", "")
        top = db.execute("SELECT COALESCE(max(patternid), 0) FROM itempattern").fetchone()[0]
        if before == "":
            if top == 0:
                db.execute("DELETE FROM sqlite_sequence WHERE name = 'itempattern'")
        elif top <= int(before):
            db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = 'itempattern'", (int(before),))
        for t in ("quest_import_quests", "quest_import_items", "quest_import_meta"):
            db.execute(f"DROP TABLE {t}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return n


def backup(db, path, what):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-{what}"
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("files", nargs="*", help="content files (default: every tools/quests/data/*.json)")
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--apply", action="store_true", help="write the quests")
    ap.add_argument("--undo", action="store_true", help="delete everything an import wrote")
    ap.add_argument("--no-backup", action="store_true", help="write without a copy first")
    ap.add_argument("--verbose", action="store_true", help="say why each draft is one")
    ap.add_argument("--routes", default=travel.WAYSENDER,
                    help="the server's waysender.go, whose routes the unlocks name (default: this tree's)")
    args = ap.parse_args()
    db = sqlite3.connect(args.db, isolation_level=None)
    if args.undo:
        if not args.no_backup:
            print("backup:", backup(db, args.db, "questundo"))
        n = undo(db)
        print(f"deleted {n['quests']} quests ({n['steps']} steps, {n['creatures']} other creatures of their steps, "
              f"{n['progress']} characters' progress, {n['trips']} trips they unlocked) and {n['items']} items made "
              f"({n['held']} of them held or sold); "
              "restart the server")
        return
    paths = args.files or sorted(glob.glob(os.path.join(DEFAULT_DATA, "*.json")))
    if not paths:
        sys.exit("no content files")
    rs = travel.travel_routes(args.routes)
    if rs is None:
        print(f"warning: no routes in {args.routes}: the unlocks' routes are not checked")
    p = plan(db, load_files(paths), routes=rs)
    report(p, args.verbose)
    if args.apply:
        if not args.no_backup:
            print("backup:", backup(db, args.db, "questimport"))
        written, kept, made = write(db, p)
        print(f"wrote {written} quests and made {made} items; restart the server")
        for k in kept:
            print("  left as a GM changed it:", k)
    else:
        print("(a report: --apply writes it)")


if __name__ == "__main__":
    main()
