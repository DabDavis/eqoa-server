#!/usr/bin/env python3
"""Bring the Live Server's content up to the Heirloom Test Server's (a GM's report, 2026-11-22: Live is behind Heirloom on
mob placements, loot, quests and items). One direction only, Heirloom -> Live; nothing is ever written to Heirloom.

Since the shards split (2026-11-07) and the last two-way sync (tools/shard_sync_20261115.py, 2026-11-15), many Heirloom
fixes (GM panel edits, camp reports, petition fixes, guide audits, loot) went to Heirloom alone. This tool finds them by
comparing the content tables of both databases with shard_sync_20261115.py's engine (rows matched by natural keys: item
and quest records by name where the shards minted their own ids, NPC rows by the row both were cloned from or by name and
place; every reference translated), sorts every difference into three groups, and copies group (a) only:

  (a) Heirloom content Live lacks: rows Heirloom changed, added or deleted by its own audited edits, with no own edit
      on Live (a GM's panel or game edit, a tool, a petition fix, a camp report), and the records they need.
  (b) different on purpose, never copied: Heirloom's own features (the Christmas season, Heirloom Marks, heirlooms and
      their Outfitters, the Ascended copies, imports, familiars, Testing Heroes, mercenaries, custom races and their
      homelands, waysenders and kits, the evil dwarf start, Heirloom's own worlds 10-15, the Clockwork Invasion), one
      player's own records, the Staff and what make_live_db.py strips; Live's own fixes Heirloom lacks (listed: they
      may need to go the other way, by hand); the same tool run on both with its own random spots (wildspread).
  (c) for the owner to decide, never copied: rows both shards edited since the split (conflicts), rows that differ
      with no own edit on either shard, rows one shard deleted without an audit, dangling rows, a GM's Heirloom
      petition that placed service NPCs by the Outfitters, the new damage formula's training dummies, and any row whose reference could
      not be resolved on Live.

Player tables (characters, inventories, quest progress, factions, accounts, petitions, gm_audit, logs, transfers, ...)
are not content: never compared, never written. A quest whose steps change shape while a Live character is part-way
through it is left whole (no character's progress is ever moved), and listed.

TWO STEPS, because the comparison of two full databases takes a minute or two and a keep restart holds players for
12-20 s:

  1. --plan PLAN.json (before the restart, on COPIES: never scan the live databases): reads both, writes the report
     (--report REPORT.json: every difference with its group and reason) and the plan: each Live row to insert, update
     or delete, with its expected value before and after, Live's ids already chosen.
         python3 -E -s tools/live_sync_20261122.py --heirloom H.db --live L.db --plan plan.json [--report r.json]
  2. --apply (in the keep restart's hold, on the Live database): replays the plan in one transaction, each row
     checked against what the plan expects. A row as the plan leaves it is there already (a second --apply writes
     nothing); a row changed on Live since the plan is left (a quest: the whole quest), listed; a record id the plan
     inserts that Live has given to something else since refuses the whole run (make a new plan). A gm_audit line a
     row; every row's before and after in live_sync_1122_log (sqlite_sequence's in live_sync_1122_seq).
         python3 -E -s tools/live_sync_20261122.py --live ~/eqoa-live/eqoa.db --plan plan.json --apply
  --undo puts back every row the log holds, newest first (a row changed since is left and listed), and the id
  sequences.
         python3 -E -s tools/live_sync_20261122.py --live ~/eqoa-live/eqoa.db --undo

TAKES EFFECT: items, loot, factions at once (read as the server goes); NPCs, quests, templates at the server's start (the
keep restart this runs in).
"""
import argparse
import collections
import copy
import datetime
import importlib.util
import json
import os
import re
import sqlite3
import sys
import tempfile
import time

HERE = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location("shard_sync_20261115", os.path.join(HERE, "shard_sync_20261115.py"))
ss = importlib.util.module_from_spec(_spec)
# One change to the engine's NPC pairing: a row made since the split (an id from SHARED_NPC_IDS up) is the same NPC on
# both shards under the same id only when it also stands within 60 of it. Each shard minted those ids on its own, so
# the same id can be another copy of the same creature elsewhere (the Plane of Disease's Blood Thirsty Minions: Live's
# 162611 is Heirloom's 162621): paired by id they left ten rows unpaired, each copied over as a second minion.
_SRC = open(os.path.join(HERE, "shard_sync_20261115.py")).read()
_OLD = "(i < SHARED_NPC_IDS or (h[1] == v[1] and h[0].strip().lower() == v[0].strip().lower()))"
_NEW = ("(i < SHARED_NPC_IDS or (h[1] == v[1] and h[0].strip().lower() == v[0].strip().lower() and "
        "math.hypot(h[2] - v[2], h[3] - v[3]) < 60))")
assert _SRC.count(_OLD) == 1, "shard_sync_20261115.py's NPC pairing changed: read it again before using this tool"
exec(compile(_SRC.replace(_OLD, _NEW), _spec.origin, "exec"), ss.__dict__)

TOOL = "tools/live_sync_20261122.py"
LOG = "live_sync_1122_log"
SEQ = "live_sync_1122_seq"
WHO, VIA = "dab", "tool"
DETAIL = "Live sync 2026-11-22 (from Heirloom)"
PLAN_VERSION = 1

# ---- the engine, for this run ---------------------------------------------------------------------------------------
# 2026-11-15's exclusions were lanes then in flight (the Plane of Sky key quests, Liztin's shop, Hunt and Gather): done
# since, so compared like any content now.
ss.EXCLUDED_QUESTS = set()
ss.EXCLUDED_MERCHANTS = set()
ss.EXCLUDED_WORK = re.compile(r"(?!)")

# content tables made since 2026-11-15 (or left out then): natural key, references
EXTRA = {
    "gemsetting": dict(key=["patternid"], trans={"patternid": "item"}, lid="patternid"),
    "heldbarred": dict(key=["patternid"], trans={"patternid": "item"}, lid="patternid"),
    "norentvanish": dict(key=["patternid"], trans={"patternid": "item"}, lid="patternid"),
    "itemspeed": dict(key=["patternid"], trans={"patternid": "item"}, lid="patternid"),
    "itemhide": dict(key=["patternid"], trans={"patternid": "item", "questid": "quest"}, lid="patternid"),
    "itemspell": dict(key=["patternid"], trans={"patternid": "item", "spellid": "spell"}, lid="patternid"),
    "lootdroptrigger": dict(key=["patternid"], trans={"patternid": "item"}, lid="patternid"),
    "itemoutfit": dict(key=["patternid", "slot"], trans={"patternid": "item"}),
    "merchantprice": dict(key=["merchant", "patternid"], trans={"patternid": "item"}),
    "tokenshop": dict(key=["shop", "patternid"], trans={"patternid": "item"}, ignore=["id"], lid="id"),
    "npcheld": dict(key=["npc_name", "hand"], trans={"patternid": "item"}),
    "npcdyes": dict(key=["name", "slot"]),
    "npcseason": dict(key=["name"]),
    "npctours": dict(key=["name"]),
    "npcwalkon": dict(key=["name"]),
    "npccoins": dict(key=["npc_name"]),
    "npctalk": dict(key=["npc", "node"]),
    "npctime": dict(key=["name"]),
    "npcrarecycle": dict(key=["npc_row"], trans={"npc_row": "npc"}, lid="npc_row"),
    "questdoors": dict(key=["npcid"], trans={"npcid": "npc", "questid": "quest", "item": "item"}, lid="npcid"),
    "petdyeareas": dict(key=["look"]),
    "lighttemplates": dict(key=["name"]),
    "lightstrings": dict(key=["template", "npc", "world", "shape", "params"], ignore=["id"], lid="id"),
    "lightspheres": dict(key=["world", "x", "z"], ignore=["id"], lid="id"),
    "placetexts": dict(key=["world", "x1", "z1", "x2", "z2"], ignore=["id"], lid="id"),
    "cityintro": dict(key=["world", "x", "z", "race", "class"], ignore=["id"], lid="id"),
    "decorart": dict(key=["name"]),
    "decorassets": dict(key=["name"]),
    "fxland": dict(key=["fxdict"]),
    "seasonrewards": dict(key=["season"]),
    "weatherzones": dict(key=["world", "col", "row"]),
    "weatherprofiles": dict(key=["profile"]),
    "weatherclimates": dict(key=["climate"]),
}
for _t, _s in EXTRA.items():
    if _t not in ss.SPECS:
        ss.SPECS[_t] = _s
        ss.ORDER.append(_t)
for _e in (("npcrarecycle", "npc_row"), ("questdoors", "npcid")):
    if _e not in ss.NPC_EXTRAS:
        ss.NPC_EXTRAS = ss.NPC_EXTRAS + (_e,)

IDENTITY = {"npcs": "id", "itempattern": "patternid", "spellpattern": "spellid", "quests": "questid"}
H2L = {"copy_H_to_L", "update_L_from_H", "quest_L_from_H", "delete_on_L"}
L2H = {"copy_L_to_H", "update_H_from_L", "quest_H_from_L", "delete_on_H"}

TOPICS = [
    ("placements", "npcs npcrespawn npcwander npcpaths npcrare npcrarecycle npcstands campspawns npcspawnonly noroam "
                   "npctitle signtext gmportals"),
    ("npc behaviour and looks", "npctemplate classtemplates npcmove npcaggro npccharm npcclass npcsocial npcundead npctint "
                                "npcdeath npcoverseer npcdialog npcdyes npcseason npctours npcwalkon npccoins npctalk npctime "
                                "npcheld"),
    ("loot", "npcloot loottables lootpool lootcreature lootrate lootpreset lootdroptrigger"),
    ("merchants", "npcitem merchantprice tokenshop"),
    ("quests", "quests queststeps queststeptargets questunlocks questwalls questmarks questnotyet questdoors"),
    ("items", "itempattern itemeffect itemaura itemfactions foodeffect itemascended itemimported heirlooms familiars "
              "heroes_stock gemsetting heldbarred norentvanish itemspeed itemhide itemspell itemoutfit craftrecipes"),
    ("spells", "spellpattern spelleffect spellarea spellascended spellrecastgroup classspells petstats pettemplate "
               "spellgates cmcatalog"),
    ("factions", "factions npcfaction factionhits factionhitareas"),
]
TOPIC = {t: name for name, ts in TOPICS for t in ts.split()}


def topic(t):
    return TOPIC.get(t, "other")


# ---- the rules that keep a Heirloom change off Live, read in a row's names, source/note and Heirloom's own audit -----
RULES = [
    ("b", "christmas season (Heirloom only, owner 2026-11-17)",
     r"christmas|holiday elf|\bsanta\b|santa's|grinch|toymaker|tinsel|holly token|snowball|reindeer|\bblitzen\b|\bcomet\b|"
     r"\bcupid\b|\bdonner\b|\bdasher\b|\bprancer\b|\bvixen\b|\brudolph\b|gingerbread|candy cane|pegasus plushie|"
     r"string of lights|puppet strings|\bsleigh\b|toy shoppe|toy-shoppe|seasongift"),
    ("b", "Heirloom Marks (Heirloom only)",
     r"ring of guises|mastery tome|hot zone herald|heirloom keeper|heirloom marks|heirloom_marks|heirloomrank"),
    ("b", "heirlooms and Heirloom's own features",
     r"heirloom outfitter|heirloom item|heirloom epic|heirloom stat|\bheirlooms\b|legacy splendor|legacy shield of piety|"
     r"legacy ring of toxic breath|legacy rule|genie's lamp|summoning portal|guenhwyvar|staff of faykind|familiar|"
     r"testing heroes|mercenar|rock of freeport|dabrish|moogie|quintashi|hot zone|account bank|healbot"),
    ("b", "custom races, their homelands and kits (Heirloom's open_races/all_classes)",
     r"waysender|custom race|race home|racehome|homeland|jal'?raeth|mermaid|wisp kit|wisp_kit|race_home|evil dwarf|"
     r"dwarf evil|blackvein|thedruk|\bquarry\b|shaftwarden|deepwarden|tunnelwarden|haldra coalvein|evil start"),
    ("b", "Heirloom's sandbox events and worlds (Live stays close to vanilla, owner 2026-11-17/20)",
     r"clockwork invasion|clockwork112|klick clockworks|elder mysidia|\bcorroded |\brecon (recon|lecon|eacon|wecon)\b|"
     r"klockizar|capture the flag|\bctf\b|\bsiege\b|heroic dungeon|"
     r"heroic_depth|heroic_loot|real estate|player home"),
    ("b", "the Ascended copies and their loot (Heirloom only)", r"\bascended\b"),
    ("b", "one player's own records",
     r"his alone|her alone|batplate|gotham|caped crusader|barrel of monkeys|automatic snowball"),
    ("c", "the new damage formula's training dummies (Live runs that formula in shadow only)", r"training dummy jib"),
    ("c", "an Heirloom petition's service NPCs by the Outfitters (Live has its own Dyers/Groomers/Enchanters)",
     r"equipment dyer, pet groomer|pet groomer"),
    ("c", "a GM's NPCs by Dabrish in Freeport, Heirloom's hub (the Wage Cage, Rehabilitator Sela)",
     r"wage cage|rehabilitator sela"),
]
RULES = [(c, lab, re.compile(rx, re.I)) for c, lab, rx in RULES]
ENGINE_WORDS = re.compile(r"Heirloom's own edit|Heirloom newer|Heirloom deleted|vs Heirloom|from Heirloom|on Heirloom|"
                          r"vs Live|Live's own edit|Live newer|Live deleted", re.I)
PER_SHARD = re.compile(r"\bon live\b|\blive's\b|\blive only\b|\blive-only\b|\(live\)|\bfor live\b", re.I)
NAMECOLS = ("npc_name", "name", "itemname", "spellname", "tablename", "npc", "merchant", "shop", "template", "season",
            "source", "note", "notes", "giver", "overseer", "boss", "category", "region", "itemdesc", "spelldesc")
NAME_TABLES = {"npctemplate", "classtemplates", "npcspawnonly", "npcmove", "npcaggro", "npccharm", "npcclass",
               "npcsocial", "npcundead", "npctint", "npcdeath", "npcloot", "lootcreature", "factionhits", "npcfaction",
               "factionhitareas", "npcdyes", "npcseason", "npctours", "npcwalkon", "npccoins", "npctalk", "npctime",
               "npcheld", "lootpool", "merchantprice", "tokenshop"}


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def jn(v):
    """a value as JSON compares (the plan's rows went through JSON)"""
    return json.dumps(v, sort_keys=True, separators=(",", ":"), default=str)


def plain_name(x):
    if isinstance(x, str) and x[:2] in ("N<", "I<", "S<", "Q<"):
        x = x[2:-1]
        if x.startswith(("#", "n:", "s:")):
            x = x[2:] if x.startswith(("n:", "s:")) else x
        return x.split("|")[0]
    return x


# =====================================================================================================================
# the comparison and the three groups
# =====================================================================================================================
class Sorter:
    def __init__(self, sh):
        self.sh = sh
        self.L_names = {(n or "").strip().lower() for (n,) in sh.L.execute("SELECT npc_name FROM npcs")}
        self.spread = self._spread()

    def _spread(self):
        """NPC rows wildspread's zones1115 run moved on BOTH shards (its own random spot on each)"""
        out = {}
        for s in "HL":
            db = self.sh.db[s]
            ids = set()
            for t in ("wildspread_moves_zones1115", "wildspread_moves_kelinar1115", "wildspread_moves"):
                if ss.has(db, t):
                    ids |= {r[0] for r in db.execute(f"SELECT npc_row FROM {t}")}
            out[s] = ids
        return out

    def h_detail(self, t, r):
        """Heirloom's latest audit words for the row (any edit, a tool's run on both included)"""
        probes = []
        for i in r.get("Hid") or []:
            probes += [("id", i), i] + [(f, i) for f in ("patternid", "questid", "npc_row", "npcid", "spellid", "slot")]
        kk = r["key"]
        if t in ss.QUEST_TABLES:
            for q in self.sh.QREV["H"].get(kk[0], []):
                probes.append(("questid", q))
        elif kk and isinstance(kk[0], str) and kk[0][:2] not in ("N<", "I<", "S<", "Q<"):
            probes += [("name", kk[0]), ("npc_name", kk[0]), str(kk[0]), tuple(str(x) for x in kk)]
        tabs = (t, "npcs") if t in ("npcrespawn", "npcwander", "npcpaths", "npctitle", "signtext") else \
            (("quests", "queststeps") if t in ss.QUEST_TABLES else (t,))
        best = None
        for tb in tabs:
            e = self.sh.last("H", tb, *probes)
            if e and (best is None or e[0] > best[0]):
                best = e
        return best[4] if best else ""

    def blob(self, t, r):
        parts = [str(plain_name(x)) for x in r["key"]]
        for raw in (r.get("Hraw") or [])[:3] + (r.get("Lraw") or [])[:1]:
            for c in NAMECOLS:
                if raw.get(c):
                    parts.append(str(raw[c]))
        parts.append(ENGINE_WORDS.sub(" ", r.get("reason") or ""))
        parts.append(self.h_detail(t, r))
        return " | ".join(parts)

    def rule(self, t, r):
        b = self.blob(t, r)
        for cat, lab, rx in RULES:
            if rx.search(b):
                return cat, lab
        return None

    @staticmethod
    def nullish(r):
        """an update whose every difference is NULL against 0 or ''"""
        ch = r.get("changed")
        if r.get("multi") or not ch:
            return False
        return all((a in (None, 0, "") and b in (None, 0, "")) for a, b in ch.values())

    def one(self, t, r):
        act, grp = r["action"], r["group"]
        if act == "leave":
            if grp == "cosmetic":
                return "same", "only source/note text, npc_id or same-named duplicates differ"
            if grp in ("heirloom-items", "heirloom-ascended", "heirloom-races", "heirloom-worlds", "used-only-by-shard-specific"):
                return "b", {"heirloom-items": "heirlooms and Heirloom's own features",
                             "heirloom-ascended": "the Ascended copies and their loot (Heirloom only)",
                             "heirloom-races": "custom races, their homelands and kits (Heirloom's open_races/all_classes)",
                             "heirloom-worlds": "Heirloom's own worlds 10-15 (homelands, CTF, homes, dungeons, siege)",
                             "used-only-by-shard-specific": "used only by rows that stay on their shard"}[grp]
            if grp == "live-own":
                return "b", "Live's own design (60 s low-level respawns, Silver and Fur)"
            if grp == "orphan":
                return "c", "dangling: a row its own shard no longer has a record or NPC for"
            if grp in ("deleted-on-other", "deleted-unaudited"):
                return "c", "one shard deleted the row (the twin of an insert, or with no audit)"
            return "c", f"left by the engine ({grp})"
        if act in L2H:
            if self.nullish(r):
                return "same", "NULL against 0 only"
            return "b", "Live's own fix Heirloom lacks (the other way: not done here)"
        if act == "review":
            if t == "npcs" and not r.get("multi") and set(r.get("changed") or {}) <= {"x_coord", "y_coord", "z_coord", "facing"} \
                    and any(i in self.spread["H"] for i in r.get("Hid") or []) and any(i in self.spread["L"] for i in r.get("Lid") or []):
                return "b", "the same spread run on both shards, each with its own random spots (wildspread)"
            if self.nullish(r):
                return "same", "NULL against 0 only"
            ru = self.rule(t, r)
            if ru and ru[0] == "b":
                return ru
            return "c", "differs with no own edit on either shard (unaudited, or the same tool run on both)"
        # Heirloom -> Live
        if self.nullish(r):
            return "same", "NULL against 0 only"
        own = " ".join(str(raw.get(c) or "") for raw in (r.get("Hraw") or []) + (r.get("Lraw") or [])
                       for c in ("note", "notes", "source"))
        if PER_SHARD.search(own):
            return "c", "the row's own note sets a value per shard (it names Live's): " + own[:160]
        ru = self.rule(t, r)
        if ru:
            return ru
        if t == "quests" and not r.get("multi") and set(r.get("changed") or {}) <= {"races", "notes", "source"} \
                and "races" in (r.get("changed") or {}):
            a, b = (r["changed"]["races"] + [None, None])[:2]
            sa = {int(x) for x in re.findall(r"\d+", a or "")}
            sb = {int(x) for x in re.findall(r"\d+", b or "")}
            if all(x >= 11 for x in sa ^ sb):
                return "b", "a race list that differs only in custom races (Live has none)"
        if grp == "conflict":
            return "c", "both shards edited it since the split: " + r["reason"][:200]
        return "a", r["reason"]

    def run(self):
        out = self.sh.out
        for t, rows in out.items():
            for r in rows:
                r["cat"], r["why"] = self.one(t, r)
        self.consistency()
        return out

    def consistency(self):
        out, sh = self.sh.out, self.sh
        # NPCs of a kept-off feature take the rows keyed by their name with them (when Live has no such NPC at all)
        bnames = {}
        for r in out.get("npcs", []):
            if r["action"] == "copy_H_to_L" and r["cat"] in ("b", "c"):
                n = plain_name(r["key"][0]).strip().lower()
                if n not in self.L_names:
                    bnames.setdefault(n, (r["cat"], r["why"]))
        for t in NAME_TABLES:
            for r in out.get(t, []):
                if r["cat"] != "a" or r["action"] != "copy_H_to_L":
                    continue
                raw = (r.get("Hraw") or [{}])[0]
                n = str(raw.get("npc_name") or raw.get("name") or raw.get("npc") or raw.get("merchant") or
                        raw.get("shop") or plain_name(r["key"][0]) or "").strip().lower()
                if n in bnames:
                    r["cat"], r["why"] = bnames[n][0], bnames[n][1] + " (an NPC of it)"
        # a quest goes whole: one row kept off keeps the quest off
        worst = {}
        rank = {"same": 0, "a": 1, "b": 3, "c": 2}
        for t in ss.QUEST_TABLES:
            for r in out.get(t, []):
                q = r["key"][0]
                if r["action"] in H2L | {"review"} | L2H and (q not in worst or rank[r["cat"]] > rank[worst[q][0]]):
                    worst[q] = (r["cat"], r["why"])
        for t in ss.QUEST_TABLES:
            for r in out.get(t, []):
                w = worst.get(r["key"][0])
                if w and r["cat"] == "a" and w[0] in ("b", "c"):
                    r["cat"], r["why"] = w[0], "its quest: " + w[1]
        # loot tables named only by creatures kept off
        live_tables = {x[0] for x in sh.L.execute("SELECT tablename FROM npcloot")}
        moving = {r["key"][1] for r in out.get("npcloot", []) if r["cat"] == "a"}
        for r in out.get("loottables", []):
            if r["cat"] == "a" and r["action"] == "copy_H_to_L" and r["key"][0] not in live_tables | moving:
                r["cat"], r["why"] = "b", "a loot table only creatures kept off Live use"
        # records (items, spells) copied only for rows kept off: kept off with them
        for tbl, tag in (("itempattern", "I"), ("spellpattern", "S")):
            pat = re.compile(re.escape(tag) + r"<([^<>]*)>")
            users = collections.defaultdict(set)
            for t, rows in out.items():
                if t == tbl:
                    continue
                for r in rows:
                    if r["action"] not in H2L:
                        continue
                    blob = json.dumps({x: r.get(x) for x in ("key", "H", "changed", "Honly")}, default=str)
                    for m in set(pat.findall(blob)):
                        users[m].add(r["cat"])
            for r in out.get(tbl, []):
                if r["cat"] != "a" or r["action"] != "copy_H_to_L":
                    continue
                u = users.get(r["key"][0][len(tag) + 1:-1])
                if u and "a" not in u:
                    r["cat"], r["why"] = sorted(u)[0], "needed only by rows kept off Live"
        # what is attached to a record kept off stays off
        kept = {r["key"][0] for t in ("itempattern", "spellpattern") for r in out.get(t, []) if r["cat"] != "a" and r["action"] == "copy_H_to_L"}
        for t in ("itemeffect", "itemaura", "itemfactions", "foodeffect", "gemsetting", "heldbarred", "norentvanish",
                  "itemspeed", "itemhide", "itemspell", "lootdroptrigger", "itemoutfit", "spelleffect", "spellarea",
                  "petstats", "spellrecastgroup"):
            for r in out.get(t, []):
                if r["cat"] == "a" and r["action"] == "copy_H_to_L" and r["key"][0] in kept:
                    r["cat"], r["why"] = "b", "its record is kept off Live"


# =====================================================================================================================
# the plan: the engine's writer run on a scratch copy of Live, every row change captured
# =====================================================================================================================
class Capture(ss.Applier):
    def __init__(self, *a, **k):
        super().__init__(*a, **k)
        self.cap = []
        self.unit = None
        self.sigchange = set()

    def log(self, t, pk, before, after, detail):
        self.cap.append({"t": t, "pk": pk, "before": before, "after": after, "why": detail, "unit": self.unit})

    def detail(self, r):
        return f"{topic_of_row(r)}: {r.get('why') or r['reason']}"[:240]

    def map_progress(self, q, old_steps, new_steps, det):
        # no character's progress is moved: the quest's steps changed shape; --apply leaves the quest whole while a
        # Live character is part-way
        self.sigchange.add(q)

    def op_quest(self, qkey, rs):
        self.unit = "quest:" + str(qkey)
        try:
            super().op_quest(qkey, rs)
        finally:
            self.unit = None


def topic_of_row(r):
    return r.get("_t", "")


def make_plan(hpath, lpath, plan_path, report_path):
    t0 = time.time()
    if ss.role_of(hpath) != "H" or ss.role_of(lpath) != "L":
        sys.exit("REFUSED: --heirloom must be Heirloom's database and --live Live's (heirlooms rows decide)")
    sh = ss.Shards(hpath, lpath)
    sh.compare()
    sh.classify_all()
    print(f"compared in {time.time() - t0:.0f} s", flush=True)
    Sorter(sh).run()
    # what the writer sees: (a) only
    for t, rows in sh.out.items():
        for r in rows:
            r["_t"] = t
            r["orig_action"] = r["action"]
            if r["cat"] != "a" or r["action"] not in H2L:
                r["action"] = "leave"
    w = ss.Writer(sh, "H")
    ops = w.ops()
    w.allocate(ops)
    fd, scratch = tempfile.mkstemp(prefix="livesync-", suffix=".db", dir=os.path.dirname(os.path.abspath(plan_path)))
    os.close(fd)
    try:
        src = sqlite3.connect(f"file:{lpath}?mode=ro", uri=True)
        dst = sqlite3.connect(scratch)
        src.backup(dst)   # page for page: rowids as Live has them
        src.close()
        dst.close()
        sh.db["H"].close()
        sh.db["H"] = sqlite3.connect(f"file:{hpath}?mode=ro", uri=True)
        sh.db["H"].row_factory = sqlite3.Row
        w.sdb = sh.db["H"]
        ap = Capture(sh, w, scratch, "plan")
        ap.db.execute("BEGIN IMMEDIATE")
        ap.run_ops(ops)
        ap.db.execute("ROLLBACK")
        ap.db.close()
    finally:
        for p in (scratch, scratch + "-journal", scratch + "-wal", scratch + "-shm"):
            if os.path.exists(p):
                os.remove(p)
    # rows the writer could not resolve on Live: kept off, for the owner
    skipped = collections.defaultdict(list)
    for s in ap.skipped:
        skipped[(s["table"], json.dumps(s["key"], default=str))].append(s["why"])
    for t, rows in sh.out.items():
        for r in rows:
            k = (t, json.dumps(r["key"], default=str))
            if r["cat"] == "a" and k in skipped:
                r["cat"], r["why"] = "c", "could not be written on Live: " + skipped[k][0][:160]
    report = build_report(sh)
    plan = {"version": PLAN_VERSION, "made": now(), "heirloom": os.path.abspath(hpath), "live": os.path.abspath(lpath),
            "sigchange": sorted("quest:" + str(k) for k in sh_quest_keys(sh, ap.sigchange)),
            "ops": ap.cap, "summary": report["summary"]}
    with open(plan_path, "w") as f:
        json.dump(plan, f, separators=(",", ":"), default=str)
    if report_path:
        with open(report_path, "w") as f:
            json.dump(report, f, indent=0, default=str)
    print_summary(report["summary"], len(ap.cap))
    print(f"plan: {len(ap.cap)} row changes on Live -> {plan_path} ({time.time() - t0:.0f} s)")
    if report_path:
        print(f"report: {report_path}")


def sh_quest_keys(sh, live_qids):
    """Live quest ids -> the quest keys the plan's units carry"""
    out = set()
    for q in live_qids:
        v = sh.QUEST["L"].get(q)
        if v is not None:
            out.add(f"Q<{v}>")
    return out


def build_report(sh):
    rows, summary = [], collections.OrderedDict()
    cnt = collections.Counter()
    why = collections.Counter()
    for t in ss.ORDER:
        for r in sh.out.get(t, []):
            cat = r["cat"]
            if cat == "same":
                cnt[("same", topic(t), t)] += 1
                continue
            cnt[(cat, topic(t), t)] += 1
            why[(cat, t, r["why"][:150])] += 1
            raw = ((r.get("Hraw") or []) + (r.get("Lraw") or []) + [{}])[0]
            name = next((raw[c] for c in ("itemname", "npc_name", "name", "spellname", "tablename", "npc") if raw.get(c)), "")
            rows.append({"cat": cat, "topic": topic(t), "table": t, "key": r["key"], "name": name, "engine": r.get("orig_action", r["action"]),
                         "why": r["why"], "changed": r.get("changed"), "Hid": r.get("Hid"), "Lid": r.get("Lid")})
    for cat in ("a", "b", "c", "same"):
        per = collections.OrderedDict()
        for (c, tp, t), n in sorted(cnt.items()):
            if c == cat:
                per.setdefault(tp, {})[t] = n
        summary[cat] = per
    summary["reasons"] = [{"cat": c, "table": t, "why": w, "rows": n} for (c, t, w), n in why.most_common()]
    return {"summary": summary, "rows": rows}


def print_summary(summary, nops):
    names = {"a": "(a) Heirloom content Live lacks: COPIED", "b": "(b) different on purpose: never copied",
             "c": "(c) for the owner: never copied", "same": "no real difference (cosmetic)"}
    for cat in ("a", "b", "c", "same"):
        per = summary[cat]
        tot = sum(sum(v.values()) for v in per.values())
        print(f"{names[cat]}: {tot} rows")
        for tp, ts in per.items():
            print(f"    {tp:26} {sum(ts.values()):6}  " + ", ".join(f"{t} {n}" for t, n in sorted(ts.items(), key=lambda x: -x[1])))


# =====================================================================================================================
# --apply: the plan replayed on Live, each row checked
# =====================================================================================================================
LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    run    TEXT NOT NULL,
    tbl    TEXT NOT NULL,
    pk     TEXT NOT NULL,      -- the row's key (JSON)
    before TEXT,               -- the whole row before (JSON; NULL: there was none)
    after  TEXT,               -- the whole row after (JSON; NULL: deleted)
    audit  INTEGER NOT NULL DEFAULT 0,
    undone INTEGER NOT NULL DEFAULT 0
)"""
SEQ_DDL = f"""CREATE TABLE IF NOT EXISTS {SEQ} (
    run    TEXT NOT NULL,
    tbl    TEXT NOT NULL,
    before INTEGER,            -- sqlite_sequence's seq before the run (NULL: no row)
    after  INTEGER,
    PRIMARY KEY (run, tbl)
)"""


class Refused(Exception):
    pass


def pkcols(db, t, cache={}):
    if t not in cache:
        cache[t] = ss.pkcols(db, t)
    return cache[t]


def fetch(db, t, pk):
    where = " AND ".join(f'"{c}" = ?' for c in pk)
    cur = db.execute(f'SELECT * FROM "{t}" WHERE {where}', tuple(pk.values()))
    r = cur.fetchone()
    if r is None:
        return None
    return json.loads(jn(dict(zip([d[0] for d in cur.description], r))))


def has_seq(db, t):
    sql = db.execute("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone()
    return bool(sql and "AUTOINCREMENT" in (sql[0] or "").upper())


def seq_of(db, t):
    r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = ?", (t,)).fetchone()
    return r[0] if r else None


def realloc_ok(t):
    """a table whose own id nothing points at: a row the plan inserts takes the next id when its planned one is taken"""
    if t in IDENTITY:
        return False
    s = ss.SPECS.get(t) or {}
    return any(c in (s.get("ignore") or []) for c in ("id", "itemid", "slot", "defaultCharacterID", "def_spell_id"))


def state(db, op):
    cur = fetch(db, op["t"], op["pk"])
    b, a = op["before"], op["after"]
    if b is None:
        if cur is None:
            return "do", cur
        if jn(cur) == jn(a):
            return "there", cur
        if realloc_ok(op["t"]):
            return "realloc", cur
        return "taken", cur
    if a is None:
        if cur is None:
            return "there", cur
        return ("do", cur) if jn(cur) == jn(b) else ("changed", cur)
    if cur is not None and jn(cur) == jn(a):
        return "there", cur
    if cur is not None and jn(cur) == jn(b):
        return "do", cur
    return "changed", cur


def apply_plan(lpath, plan_path):
    with open(plan_path) as f:
        plan = json.load(f)
    if plan.get("version") != PLAN_VERSION:
        sys.exit("REFUSED: not a plan of this tool's version")
    if ss.role_of(lpath) != "L":
        sys.exit("REFUSED: --live is not Live's database (it has heirlooms rows)")
    t0 = time.time()
    db = sqlite3.connect(lpath, timeout=60, isolation_level=None)
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if ss.has(db, "accountinfo") else None
    acct = acct[0] if acct else 0
    run = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    ops = plan["ops"]
    sig = set(plan.get("sigchange") or [])
    done, there, left = collections.Counter(), 0, []
    seqs = {}
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_DDL)
        db.execute(SEQ_DDL)
        # units: a quest goes whole or not at all
        units = collections.defaultdict(list)
        for i, op in enumerate(ops):
            if op.get("unit"):
                units[op["unit"]].append(i)
        skip = set()
        for u, idx in units.items():
            bad = [(ops[i]["t"], ops[i]["pk"], st) for i in idx for st in [state(db, ops[i])[0]] if st in ("changed", "taken")]
            if bad:
                left.append({"unit": u, "why": "changed on Live since the plan", "rows": bad[:5]})
                skip.update(idx)
                continue
            if u in sig:
                qids = {ops[i]["pk"].get("questid") for i in idx if ops[i]["t"] in ss.QUEST_TABLES}
                qids.discard(None)
                if qids and ss.has(db, "charquests"):
                    n = db.execute(f"SELECT COUNT(*) FROM charquests WHERE done = 0 AND questid IN ({','.join('?' * len(qids))})",
                                   tuple(qids)).fetchone()[0]
                    if n:
                        left.append({"unit": u, "why": f"its steps change shape and {n} Live character(s) are part-way: "
                                                       "left whole (no progress is moved)", "rows": []})
                        skip.update(idx)
        for i, op in enumerate(ops):
            if i in skip:
                continue
            t, pk = op["t"], dict(op["pk"])
            st, cur = state(db, op)
            if st == "there":
                there += 1
                continue
            if st == "taken":
                raise Refused(f"{t} {pk} is another row on Live now (the plan inserts it): make a new plan")
            if st == "changed":
                left.append({"unit": None, "why": "changed on Live since the plan", "rows": [(t, pk)]})
                continue
            if t not in seqs and has_seq(db, t):
                seqs[t] = seq_of(db, t)
            if op["before"] is None:
                vals = dict(op["after"])
                if st == "realloc":
                    for c in pk:
                        vals.pop(c, None)
                cs = list(vals)
                c2 = db.execute(f'INSERT INTO "{t}" ({", ".join(chr(34) + c + chr(34) for c in cs)}) VALUES ({", ".join("?" for _ in cs)})',
                                tuple(vals[c] for c in cs))
                if st == "realloc":
                    pk = {c: c2.lastrowid for c in pk}
                after, before, action = fetch(db, t, pk), None, "insert"
            elif op["after"] is None:
                rid = db.execute(f'SELECT rowid FROM "{t}" WHERE ' + " AND ".join(f'"{c}" = ?' for c in pk), tuple(pk.values())).fetchone()
                db.execute(f'DELETE FROM "{t}" WHERE ' + " AND ".join(f'"{c}" = ?' for c in pk), tuple(pk.values()))
                after, before, action = None, dict(cur, _rowid_=rid[0] if rid else None), "delete"
            else:
                sets = {c: v for c, v in op["after"].items() if c not in pk and cur.get(c) != v}
                db.execute(f'UPDATE "{t}" SET ' + ", ".join(f'"{c}" = ?' for c in sets) + " WHERE " +
                           " AND ".join(f'"{c}" = ?' for c in pk), tuple(sets.values()) + tuple(pk.values()))
                after, before, action = fetch(db, t, pk), cur, "update"
            a = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                           "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                           (now(), acct, WHO, VIA, action, t, jn(pk), jn({c: v for c, v in before.items() if c != "_rowid_"}) if before is not None else "",
                            jn(after) if after is not None else "", f"{DETAIL}: {op.get('why', '')}; {TOOL}"[:900])).lastrowid
            db.execute(f"INSERT INTO {LOG} (run, tbl, pk, before, after, audit) VALUES (?, ?, ?, ?, ?, ?)",
                       (run, t, jn(pk), jn(before) if before is not None else None, jn(after) if after is not None else None, a))
            done[(t, action)] += 1
        for t, b in seqs.items():
            db.execute(f"INSERT INTO {SEQ} (run, tbl, before, after) VALUES (?, ?, ?, ?)", (run, t, b, seq_of(db, t)))
        db.execute("COMMIT")
    except Exception:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()
    print(f"APPLIED run {run}: {sum(done.values())} row changes on Live, {there} already as the plan leaves them, "
          f"{len(left)} left; {time.time() - t0:.2f} s")
    for (t, k), n in sorted(done.items()):
        print(f"  {t:22} {k:7} {n}")
    for x in left[:80]:
        print("  LEFT", x["unit"] or "", x["why"], x["rows"][:3])
    return done, there, left


# =====================================================================================================================
# --undo
# =====================================================================================================================
def undo(lpath):
    db = sqlite3.connect(lpath, timeout=60, isolation_level=None)
    if not ss.has(db, LOG):
        print("nothing to undo: no", LOG)
        return 0, []
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if ss.has(db, "accountinfo") else None
    acct = acct[0] if acct else 0
    n, left = collections.Counter(), []
    db.execute("BEGIN IMMEDIATE")
    try:
        rows = db.execute(f"SELECT seq, run, tbl, pk, before, after FROM {LOG} WHERE undone = 0 ORDER BY seq DESC").fetchall()
        runs = set()
        for seq, run, t, pk, before, after in rows:
            pk = json.loads(pk)
            before = json.loads(before) if before else None
            after = json.loads(after) if after else None
            rowid = before.pop("_rowid_", None) if before else None
            cur = fetch(db, t, pk)
            if (after is None and cur is not None) or (after is not None and (cur is None or jn(cur) != jn(after))):
                left.append((seq, t, pk))
                continue
            where = " AND ".join(f'"{c}" = ?' for c in pk)
            if before is None:
                db.execute(f'DELETE FROM "{t}" WHERE {where}', tuple(pk.values()))
                act = "delete"
            elif after is None:
                cs = list(before)
                if rowid is not None and not db.execute(f'SELECT 1 FROM "{t}" WHERE rowid = ?', (rowid,)).fetchone():
                    before = dict(before, rowid=rowid)   # back under its own rowid (a row of a table keyed otherwise)
                    cs = list(before)
                db.execute(f'INSERT INTO "{t}" ({", ".join(chr(34) + c + chr(34) for c in cs)}) VALUES ({", ".join("?" for _ in cs)})',
                           tuple(before[c] for c in cs))
                act = "insert"
            else:
                sets = [c for c in before if before[c] != after.get(c)]
                if sets:
                    db.execute(f'UPDATE "{t}" SET ' + ", ".join(f'"{c}" = ?' for c in sets) + f" WHERE {where}",
                               tuple(before[c] for c in sets) + tuple(pk.values()))
                act = "update"
            db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                       "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                       (now(), acct, WHO, VIA, act, t, jn(pk), jn(cur) if cur is not None else "",
                        jn({c: v for c, v in before.items() if c != "rowid"}) if before is not None else "", f"{DETAIL} undone (log {seq}, run {run}); {TOOL} --undo"))
            db.execute(f"UPDATE {LOG} SET undone = 1 WHERE seq = ?", (seq,))
            n[(t, act)] += 1
            runs.add(run)
        if ss.has(db, SEQ) and not left:
            for run, t, b, a in db.execute(f"SELECT run, tbl, before, after FROM {SEQ}").fetchall():
                if run in runs and seq_of(db, t) == a:
                    if b is None:
                        db.execute("DELETE FROM sqlite_sequence WHERE name = ?", (t,))
                    else:
                        db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = ?", (b, t))
            db.execute(f"DELETE FROM {SEQ} WHERE run IN ({','.join('?' * len(runs))})", tuple(runs))
        db.execute("COMMIT")
    except Exception:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()
    print(f"undone: {sum(n.values())} rows")
    for (t, a), c in sorted(n.items()):
        print(f"  {t:22} {a:7} {c}")
    if left:
        print(f"LEFT (changed since the sync; not put back, nor the id sequences): {len(left)}")
        for x in left[:50]:
            print("  ", x)
    return sum(n.values()), left


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--heirloom", help="Heirloom's database (a copy), read only: --plan")
    ap.add_argument("--live", required=True, help="Live's database: a copy for --plan; the one to write for --apply/--undo")
    ap.add_argument("--plan", help="the plan file: written without --apply, replayed with it")
    ap.add_argument("--report", help="with a new plan: every difference, its group and reason (JSON)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    if a.undo:
        undo(a.live)
        return
    if not a.plan:
        ap.error("--plan is needed (written by a report run, replayed by --apply)")
    if a.apply:
        apply_plan(a.live, a.plan)
        return
    if not a.heirloom:
        ap.error("--heirloom is needed to make a plan")
    make_plan(a.heirloom, a.live, a.plan, a.report)


if __name__ == "__main__":
    try:
        main()
    except Refused as e:
        sys.exit(f"REFUSED (nothing written): {e}")
