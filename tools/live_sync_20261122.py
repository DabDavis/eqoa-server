#!/usr/bin/env python3
"""Sync the Heirloom Test Server's and the Live Server's content both ways, and settle what the two disagree on (the
owner, 2026-11-22, after a GM's report that Live was behind Heirloom: "sync both ways, and rectify" the rows left for
a person). One plan carries both directions; each shard's half is applied to that shard alone.

Since the split (2026-11-07) and the last two-way sync (tools/shard_sync_20261115.py, 2026-11-15), many fixes went to one
shard only. This tool compares the content tables of both databases with shard_sync_20261115.py's engine (rows matched
by natural keys: item and quest records by name where the shards minted their own ids, NPC rows by the row both were
cloned from or by name and place; every reference translated; three fixes to the engine below) and sorts every
difference:

  HL   Heirloom content Live lacks (Heirloom's own audited edit, none on Live): copied to Live.
  LH   Live's own fixes Heirloom lacks: copied to Heirloom. Never Live's design (the 60 s low-level respawns, Silver and
       Fur) nor Live's stand-ins for a Heirloom feature (the Ascended loot held off Live, the Spell Archivists' scrolls
       sold at Live's guild merchants).
  rectified (the owner's rules for what the two disagree on):
       R-dangling  a row whose NPC/item its shard no longer has: deleted there; kept, under the new id, when this plan
                   brings that NPC back to the shard.
       R-noaudit   differs with no audit on either shard: Heirloom's value on both (it is the main shard), Live's where
                   a Live tool's own table (a log, backup or give list Heirloom has none of) names the row. Recipes:
                   Heirloom's.
       R-both      edited on both since the split: the later edit (gm_audit) on both.
       R-deletes   a row one shard deleted: deleted on the other too when deliberate (the other shard audited the delete
                   of its own row of the same insert, or a tool's log records it), else restored where it was lost.
       R-copy      the GM's NPCs by Dabrish (the Wage Cage, Rehabilitator Sela) and the damage formula's training dummies:
                   copied to Live, which lacks them.
  b    different on purpose, never copied: Heirloom's own features (the Christmas season, Heirloom Marks, heirlooms and
       their Outfitters, the Ascended copies, imports, familiars, Testing Heroes, mercenaries, custom races and their
       homelands, waysenders and kits, the evil dwarf start, Heirloom's own worlds 10-15, the Clockwork Invasion), one
       player's own records, what make_live_db.py strips; Live's design rows; each shard's own Dyers and Groomers; a
       value set per shard by the row's own note (Sarek: 23 h on Heirloom, 3 days on Live); the same spread tool run on
       both with its own random spots (wildspread).
  c    left for a person, listed: rows the writer could not resolve on the other shard (a quest that is several quests
       of one name and giver on a shard; a same-named duplicate record).
Plus Historian Tsali on the hill north of Freeport's north gate, on both shards (TSALI below).

Player tables (characters, inventories, quest progress, factions, accounts, petitions, gm_audit, logs, transfers, ...)
are not content: never compared, never written. A quest whose steps change shape while a character of the shard written
is part-way through it is left whole there (no character's progress is ever moved), and listed.

TWO STEPS, because the comparison of two full databases takes about a minute and a keep restart holds players for 12-20 s:

  1. the plan, on COPIES of both databases (never scan the live files): writes the report (--report: every difference
     with its group, direction and reason) and the plan: each shard's rows to insert, update or delete, with the value
     expected before and after, that shard's ids already chosen.
         python3 -E -s tools/live_sync_20261122.py --heirloom H.db --live L.db --plan plan.json [--report r.json]
  2. --apply, in the keep restart's hold, once per shard (each names ONE database; its half of the plan is replayed in
     one transaction, each row checked against the plan). A row as the plan leaves it is there already (a second
     --apply writes nothing); a row changed since the plan is left (a quest, or Tsali with his rows: whole), listed; a
     record id the plan inserts that the shard has given to something else since refuses that shard's whole run (make
     a new plan). A gm_audit line a row; every row's before and after in live_sync_1122_log (sqlite_sequence's in
     live_sync_1122_seq).
         python3 -E -s tools/live_sync_20261122.py --heirloom ~/eqoa-server/eqoa.db --plan plan.json --apply
         python3 -E -s tools/live_sync_20261122.py --live ~/eqoa-live/eqoa.db --plan plan.json --apply
  --undo puts back every row the log holds on the database named, newest first (a row changed since is left and
  listed), rowids and id sequences too.
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
# both shards under the same id only when it also stands on the same spot (within 2). Each shard minted those ids on its
# own, so the same id can be another copy of the same creature elsewhere (the Plane of Disease's Blood Thirsty Minions:
# Live's 162611 is Heirloom's 162621): paired by id they left ten rows unpaired, each copied over as a second minion;
# two plagued insect clouds 58 apart paired crosswise swapped places at every run. A row moved since is paired by the
# engine's later passes (its name and spot, else the nearest of its name within 60).
# And a delete one shard audited of an NPC's extra row (a respawn, a wander) under an id minted since the split is the
# other shard's delete of ITS row of that id only when that id is the same NPC on both (paired): Live's delete of its
# Harvest Scarecrow 162563's respawn is not Heirloom's Equipment Dyer 162563's.
_SRC = open(os.path.join(HERE, "shard_sync_20261115.py")).read()
_OLD = "(i < SHARED_NPC_IDS or (h[1] == v[1] and h[0].strip().lower() == v[0].strip().lower()))"
_NEW = ("(i < SHARED_NPC_IDS or (h[1] == v[1] and h[0].strip().lower() == v[0].strip().lower() and "
        "math.hypot(h[2] - v[2], h[3] - v[3]) < 2))")
# And one to its test of "the same tool run on both shards": a tool's words name the shard it ran on ("...py
# (heirloom)" against "...py (live)": owner_decisions_20261122.py's 1,850 and 1,275 lines), so the shard's name is
# dropped before the words are compared; else the later of the two runs reads as the newer edit of every row it touched.
_PATCHES = [
    (_OLD, _NEW),
    ('''                for e in self.DELETED[side].get((t, cnd), []):
                    if t == "npcs"''', '''                for e in self.DELETED[side].get((t, cnd), []):
                    if t != "npcs" and t in {x for x, _ in NPC_EXTRAS} and cnd and str(cnd[0]).isdigit() and \\
                            int(cnd[0]) >= SHARED_NPC_IDS and self.PAIR[rowside].get(int(cnd[0])) != int(cnd[0]):
                        continue
                    if t == "npcs"'''),
    ('det[(tbl, re.sub(r"\\d+", "N", detail[:300]))].append(at)', 'det[(tbl, _detnorm(detail[:300]))].append(at)'),
    ('self.DET[OTHER[side]].get((tbl, re.sub(r"\\d+", "N", ent[4])), [])', 'self.DET[OTHER[side]].get((tbl, _detnorm(ent[4])), [])'),
]
for _a, _b in _PATCHES:
    assert _SRC.count(_a) == 1, f"shard_sync_20261115.py changed ({_a[:40]}...): read it again before using this tool"
    _SRC = _SRC.replace(_a, _b)
_SRC += """

def _detnorm(d):
    d = re.sub(r"\\b(heirloom|live)('s)?\\b", "", (d or "").lower())
    return re.sub(r"\\s+", " ", re.sub(r"\\d+", "N", re.sub(r"[()]", "", d))).strip()
"""
exec(compile(_SRC, _spec.origin, "exec"), ss.__dict__)

TOOL = "tools/live_sync_20261122.py"
LOG = "live_sync_1122_log"
SEQ = "live_sync_1122_seq"
WHO, VIA = "dab", "tool"
DETAIL = "Shard sync 2026-11-22"
PLAN_VERSION = 2

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
# the comparison and the groups
# =====================================================================================================================
# what each group means in the report, and which way it writes
CATS = collections.OrderedDict([
    ("HL", "Heirloom content Live lacks: copied to Live"),
    ("LH", "Live's own fixes Heirloom lacks: copied to Heirloom"),
    ("R-dangling", "rectified: a dangling row deleted on the shard whose NPC/item is gone"),
    ("R-noaudit", "rectified: differs with no audit: Heirloom's value on both, Live's where a Live tool's log names it"),
    ("R-both", "rectified: edited on both shards: the later edit on both"),
    ("R-deletes", "rectified: a one-side delete: deleted on the other too when deliberate, else restored"),
    ("R-copy", "rectified: copied to the shard that lacks it (the NPCs by Dabrish, the formula dummies)"),
    ("b", "different on purpose: kept per shard, never copied"),
    ("c", "left for a person (listed)"),
    ("same", "no real difference (cosmetic)"),
])
WRITES = {"HL", "LH", "R-dangling", "R-noaudit", "R-both", "R-deletes", "R-copy"}
TO_L = {"copy": "copy_H_to_L", "update": "update_L_from_H", "quest": "quest_L_from_H", "delete": "delete_on_L"}
TO_H = {"copy": "copy_L_to_H", "update": "update_H_from_L", "quest": "quest_H_from_L", "delete": "delete_on_H"}
# Live rows that stand in for a Heirloom feature (the Ascended loot, the Spell Archivists' scrolls): never to Heirloom
LIVE_STANDS_IN = re.compile(r"no ascended|ascended record|unsold level \d+\+ scrolls|unsold \d+-\d+ scrolls|"
                            r"live's level \d+/\d+ and \d+ spell drops|scrolls at live's", re.I)


class Sorter:
    def __init__(self, sh):
        self.sh = sh
        self.names = {s: {(n or "").strip().lower() for (n,) in sh.db[s].execute("SELECT npc_name FROM npcs")} for s in "HL"}
        self.spread = self._spread()
        self.live_ints, self.live_names = self._live_logs()
        self.rules_used = collections.Counter()

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

    def _live_logs(self):
        """the ids and names Live's own tools' tables hold (the tables Heirloom has none of: a Live tool's log, backup
        or give list): a Live value one of them names is a deliberate Live fix"""
        H, L = self.sh.H, self.sh.L
        th = {r[0] for r in H.execute("SELECT name FROM sqlite_master WHERE type = 'table'")}
        ints, names = set(), set()
        for (t,) in L.execute("SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name"):
            if t in th or t.startswith("sqlite_"):
                continue
            for row in L.execute(f'SELECT * FROM "{t}"'):
                for v in row:
                    if isinstance(v, int) and v >= 100:
                        ints.add(v)
                    elif isinstance(v, str):
                        if len(v) <= 64:
                            names.add(v.strip().lower())
                        ints.update(int(x) for x in re.findall(r"(?<![\d.])(\d{3,7})(?![\d.])", v))
                        names.update(x.lower() for x in re.findall(r'"(?:npc_name|name|itemname|spellname|tablename)":\s*"([^"]{1,64})"', v))
        return ints, names

    def detail(self, side, t, r):
        """a shard's latest audit words for the row (any edit, a tool's run on both included)"""
        probes = []
        for i in r.get(side + "id") or []:
            probes += [("id", i), i] + [(f, i) for f in ("patternid", "questid", "npc_row", "npcid", "spellid", "slot")]
        kk = r["key"]
        if t in ss.QUEST_TABLES:
            for q in self.sh.QREV[side].get(kk[0], []):
                probes.append(("questid", q))
        elif kk and isinstance(kk[0], str) and kk[0][:2] not in ("N<", "I<", "S<", "Q<"):
            probes += [("name", kk[0]), ("npc_name", kk[0]), str(kk[0]), tuple(str(x) for x in kk)]
        tabs = (t, "npcs") if t in ("npcrespawn", "npcwander", "npcpaths", "npctitle", "signtext") else \
            (("quests", "queststeps") if t in ss.QUEST_TABLES else (t,))
        best = None
        for tb in tabs:
            e = self.sh.last(side, tb, *probes)
            if e and (best is None or e[0] > best[0]):
                best = e
        return best[4] if best else ""

    def blob(self, t, r, side):
        parts = [str(plain_name(x)) for x in r["key"]]
        first, second = ("Hraw", "Lraw") if side == "H" else ("Lraw", "Hraw")
        for raw in (r.get(first) or [])[:3] + (r.get(second) or [])[:1]:
            for c in NAMECOLS:
                if raw.get(c):
                    parts.append(str(raw[c]))
        parts.append(ENGINE_WORDS.sub(" ", r.get("reason") or ""))
        parts.append(self.detail(side, t, r))
        return " | ".join(parts)

    def rule(self, t, r, side):
        b = self.blob(t, r, side)
        for cat, lab, rx in RULES:
            if rx.search(b):
                return cat, lab, b
        return None, None, b

    @staticmethod
    def nullish(r):
        """an update whose every difference is NULL against 0 or ''"""
        ch = r.get("changed")
        if r.get("multi") or not ch:
            return False
        return all((a in (None, 0, "") and b in (None, 0, "")) for a, b in ch.values())

    @staticmethod
    def per_shard(r):
        """both shards have the row and its own note names each shard's value (Sarek: 23 h on Heirloom, 3 days on Live)"""
        if not (r.get("Hraw") and r.get("Lraw")):
            return False
        own = " ".join(str(raw.get(c) or "") for raw in r["Hraw"] + r["Lraw"] for c in ("note", "notes", "source"))
        return bool(PER_SHARD.search(own))

    def live_named(self, t, r):
        """a Live tool's own table names this row (its Live id, or its name for a row keyed by name)"""
        ids = list(r.get("Lid") or [])
        if t in ss.QUEST_TABLES:
            ids += self.sh.QREV["L"].get(r["key"][0], [])
        if any(i in self.live_ints for i in ids if isinstance(i, int) and i >= 100):
            return True
        k0 = r["key"][0] if r["key"] else None
        if isinstance(k0, str) and k0[:2] not in ("N<", "I<", "S<", "Q<") and k0.strip().lower() in self.live_names:
            return True
        return False

    @staticmethod
    def side_of(r):
        return "H" if r.get("Hraw") and not r.get("Lraw") else ("L" if r.get("Lraw") and not r.get("Hraw") else None)

    def one(self, t, r):
        """(group, why, the engine action to run or 'leave')"""
        act, grp = r["action"], r["group"]
        if act == "leave":
            if grp == "cosmetic":
                return "same", "only source/note text, npc_id or same-named duplicates differ", "leave"
            if grp in ("heirloom-items", "heirloom-ascended", "heirloom-races", "heirloom-worlds", "used-only-by-shard-specific"):
                return "b", {"heirloom-items": "heirlooms and Heirloom's own features",
                             "heirloom-ascended": "the Ascended copies and their loot (Heirloom only)",
                             "heirloom-races": "custom races, their homelands and kits (Heirloom's open_races/all_classes)",
                             "heirloom-worlds": "Heirloom's own worlds 10-15 (homelands, CTF, homes, dungeons, siege)",
                             "used-only-by-shard-specific": "used only by rows that stay on their shard"}[grp], "leave"
            if grp == "live-own":
                return "b", "Live's own design (60 s low-level respawns, Silver and Fur)", "leave"
            if grp == "orphan":
                s = self.side_of(r)
                return "R-dangling", f"its NPC/item is gone on {ss.NAMES[s]}: deleted there", (TO_H if s == "H" else TO_L)["delete"]
            if grp == "deleted-on-other":
                s = self.side_of(r)
                name = str(plain_name(r["key"][0])).split("|")[0].strip().lower()
                if name in ("pet groomer", "equipment dyer"):
                    return "b", ("Live's GM deleted Heirloom's extra service NPCs on Live: each shard keeps its own "
                                 "Dyers/Groomers"), "leave"
                return "R-deletes", (f"deliberate: {ss.NAMES[ss.OTHER[s]]} deleted its own row of the same insert "
                                     f"(audited): deleted on {ss.NAMES[s]} too"), (TO_H if s == "H" else TO_L)["delete"]
            if grp == "deleted-unaudited":
                s = self.side_of(r)   # the shard that still has it
                if self.logged_delete(ss.OTHER[s], r):
                    return "R-deletes", f"a tool's log on {ss.NAMES[ss.OTHER[s]]} records the delete: deleted on {ss.NAMES[s]} too", \
                        (TO_H if s == "H" else TO_L)["delete"]
                return "R-deletes", f"lost on {ss.NAMES[ss.OTHER[s]]} with no audit and no tool's log: restored there", \
                    (TO_L if s == "H" else TO_H)["copy"]
            return "c", f"left by the engine ({grp})", "leave"
        if self.nullish(r):
            return "same", "NULL against 0 only", "leave"
        if act == "review":
            if t == "npcs" and not r.get("multi") and set(r.get("changed") or {}) <= {"x_coord", "y_coord", "z_coord", "facing"} \
                    and any(i in self.spread["H"] for i in r.get("Hid") or []) and any(i in self.spread["L"] for i in r.get("Lid") or []):
                return "b", "the same spread run on both shards, each with its own random spots (wildspread)", "leave"
            cat, lab, _ = self.rule(t, r, "H")
            if cat == "b":
                return cat, lab, "leave"
            if self.per_shard(r):
                return "b", "a value set per shard by the row's own note", "leave"
            kind = "quest" if t in ss.QUEST_TABLES else "update"
            if t != "craftrecipes" and self.live_named(t, r):
                return "R-noaudit", "no audit; a Live tool's own table names Live's value: Live's on both", TO_H[kind]
            return "R-noaudit", "no audit: Heirloom's value on both" + (" (recipes: Heirloom's)" if t == "craftrecipes" else ""), TO_L[kind]
        if act in H2L:
            if self.per_shard(r):
                return "b", "a value set per shard by the row's own note", "leave"
            cat, lab, _ = self.rule(t, r, "H")
            if cat == "b":
                return cat, lab, "leave"
            if lab and "dummies" in lab:
                return "R-copy", lab + ": copied to Live (harmless while Live's formula is shadow)", act
            if lab and "Dabrish" in lab:
                return "R-copy", lab + ": copied to Live, which lacks them", act
            if lab and "service NPCs" in lab:
                return "b", lab + ": each shard keeps its own", "leave"
            if t == "quests" and not r.get("multi") and set(r.get("changed") or {}) <= {"races", "notes", "source"} \
                    and "races" in (r.get("changed") or {}):
                a, b = (r["changed"]["races"] + [None, None])[:2]
                sa = {int(x) for x in re.findall(r"\d+", a or "")}
                sb = {int(x) for x in re.findall(r"\d+", b or "")}
                if all(x >= 11 for x in sa ^ sb):
                    return "b", "a race list that differs only in custom races (Live has none)", "leave"
            if grp == "conflict":
                return "R-both", "edited on both; Heirloom's edit is the later: " + r["reason"][:200], act
            return "HL", r["reason"], act
        if act in L2H:
            if self.per_shard(r):
                return "b", "a value set per shard by the row's own note", "leave"
            cat, lab, b = self.rule(t, r, "L")
            if cat == "b":
                return cat, lab, "leave"
            if LIVE_STANDS_IN.search(b):
                return "b", "Live's stand-in for a Heirloom feature (the Ascended loot, the Spell Archivists' scrolls)", "leave"
            if grp == "conflict":
                return "R-both", "edited on both; Live's edit is the later: " + r["reason"][:200], act
            return "LH", r["reason"], act
        return "c", f"left by the engine ({act})", "leave"

    def logged_delete(self, side, r):
        """a tool's log table on this shard records deleting the row (its id with 'delete'/'removed' beside it)"""
        db = self.sh.db[side]
        ids = [i for s in "HL" for i in (r.get(s + "id") or [])]
        for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' AND (name LIKE '%log%' OR name LIKE '%bak%')"):
            cols = [c[1] for c in db.execute(f'PRAGMA table_info("{t}")')]
            for i in ids:
                for c in cols:
                    try:
                        rows = db.execute(f'SELECT * FROM "{t}" WHERE "{c}" = ? LIMIT 5', (i,)).fetchall()
                    except sqlite3.Error:
                        continue
                    for row in rows:
                        txt = " ".join(str(v) for v in row).lower()
                        if re.search(r"\bdelete|\bremov|\bgone\b|\bdrop", txt) and "before" not in txt:
                            return True
        return False

    def run(self):
        out = self.sh.out
        for t, rows in out.items():
            for r in rows:
                r["cat"], r["why"], r["do"] = self.one(t, r)
        self.consistency()
        return out

    @staticmethod
    def copyable(r, src, copy):
        """a row only src has, that the engine would copy or kept for want of a user"""
        only = bool(r.get(src + "raw")) and not r.get(ss.OTHER[src] + "raw")
        return r["action"] == copy or (r["action"] == "leave" and r.get("group") == "used-only-by-shard-specific" and only)

    def consistency(self):
        out, sh = self.sh.out, self.sh
        def keep(r, cat, why):
            r["cat"], r["why"], r["do"] = cat, why, "leave"
        # NPCs of a kept-off feature take the rows keyed by their name with them (when the other shard has no such NPC)
        for src, copy in (("H", "copy_H_to_L"), ("L", "copy_L_to_H")):
            dst = ss.OTHER[src]
            bnames = {}
            for r in out.get("npcs", []):
                if r["action"] == copy and r["do"] == "leave" and r["cat"] in ("b", "c"):
                    n = plain_name(r["key"][0]).strip().lower()
                    if n not in self.names[dst]:
                        bnames.setdefault(n, (r["cat"], r["why"]))
            for t in NAME_TABLES:
                for r in out.get(t, []):
                    if r["do"] != copy:
                        continue
                    raw = ((r.get(src + "raw") or [{}]))[0]
                    n = str(raw.get("npc_name") or raw.get("name") or raw.get("npc") or raw.get("merchant") or
                            raw.get("shop") or plain_name(r["key"][0]) or "").strip().lower()
                    if n in bnames:
                        keep(r, bnames[n][0], bnames[n][1] + " (an NPC of it)")
        # an NPC deleted on a shard takes its extras there with it (the delete removes them); restored, they come along
        npc_do = {r["key"][0]: r["do"] for r in out.get("npcs", [])}
        for t, _c in ss.NPC_EXTRAS:
            for r in out.get(t, []):
                n0 = r["key"][0]
                if not (isinstance(n0, str) and n0.startswith("N<")):
                    continue
                d = npc_do.get(n0[2:-1])
                if d in ("delete_on_L", "delete_on_H") and r["do"] != "leave":
                    keep(r, "R-deletes", "goes with its NPC's delete")
                elif d in ("copy_L_to_H", "copy_H_to_L") and r["cat"] in ("c", "R-deletes") and r["do"] == "leave":
                    src = "H" if d == "copy_H_to_L" else "L"
                    if r.get(src + "raw") and not r.get(ss.OTHER[src] + "raw"):
                        r["cat"], r["why"], r["do"] = "R-deletes", "comes with its NPC, restored", d
        # a quest goes whole, one way: the quests row's decision (else its first row's), for every row of it
        qdo = {}
        rank = {"b": 3, "c": 2}
        for t in ss.QUEST_TABLES:
            for r in out.get(t, []):
                q = r["key"][0]
                cur = qdo.get(q)
                if r["cat"] in rank:
                    if cur is None or rank.get(cur[0], 0) < rank[r["cat"]]:
                        qdo[q] = (r["cat"], r["why"], "leave")
                elif cur is None or (t == "quests" and cur[0] not in rank):
                    if r["do"] != "leave":
                        qdo[q] = (r["cat"], r["why"], r["do"])
        for t in ss.QUEST_TABLES:
            for r in out.get(t, []):
                q = r["key"][0]
                if q not in qdo or r["cat"] == "same" and qdo[q][2] == "leave":
                    continue
                cat, why, d = qdo[q]
                if d == "leave":
                    if r["do"] != "leave" or r["cat"] not in ("b", "c"):
                        r["cat"], r["why"], r["do"] = cat, "its quest: " + why, "leave"
                    continue
                on = {s: bool(sh.QREV[s].get(q)) for s in "HL"}
                to = "L" if d in H2L else "H"
                frm = ss.OTHER[to]
                if d in ("delete_on_L", "delete_on_H"):
                    nd = d
                elif on[to]:
                    nd = "quest_L_from_H" if to == "L" else "quest_H_from_L"
                else:
                    nd = "copy_H_to_L" if to == "L" else "copy_L_to_H"
                r["cat"], r["why"], r["do"] = cat, why if r["key"][0] == q and t == "quests" else "its quest: " + why, nd
        # loot tables named only by creatures kept off
        for src, copy in (("H", "copy_H_to_L"), ("L", "copy_L_to_H")):
            dst = ss.OTHER[src]
            have = {x[0] for x in sh.db[dst].execute("SELECT tablename FROM npcloot")}
            moving = {r["key"][1] for r in out.get("npcloot", []) if r["do"] == copy}
            for r in out.get("loottables", []):
                if r["do"] == copy and r["key"][0] not in have | moving:
                    keep(r, "b", "a loot table only creatures kept off use")
        # records (items, spells) a written row needs: copied with it, though the engine kept them for want of a user
        # (a record kept for its own sake, an Ascended copy or a feature's, stays: the row that needs it is listed)
        soft = ("used only by rows that stay on their shard", "needed only by rows kept on their shard",
                "its record is kept on its shard")
        for src, acts in (("H", H2L), ("L", L2H)):
            copy = "copy_H_to_L" if src == "H" else "copy_L_to_H"
            need = set()
            for t, rows in out.items():
                if t in ("itempattern", "spellpattern"):
                    continue
                for r in rows:
                    if r["do"] in acts:
                        blob = json.dumps({x: r.get(x) for x in ("key", src, "changed", "Honly" if src == "H" else "Lonly")}, default=str)
                        need.update(re.findall(r"([IS]<[^<>]*>)", blob))
            # a quest is written whole: every record any of its rows names, changed or not
            qs = {q for t in ss.QUEST_TABLES for r in out.get(t, []) if r["do"] in acts for q in sh.QREV[src].get(r["key"][0], [])}
            db = sh.db[src]
            for q in qs:
                for t in ("quests", "queststeps"):
                    for row in db.execute(f"SELECT * FROM {t} WHERE questid = ?", (q,)):
                        for v in row:
                            if isinstance(v, str):
                                for m in re.findall(r"(?<![\w.])(\d{4,7})(?![\w.])", v):
                                    if int(m) in sh.ITEM[src]:
                                        need.add(f"I<{sh.ITEM[src][int(m)]}>")
                                    if int(m) in sh.SPELL[src]:
                                        need.add(f"S<{sh.SPELL[src][int(m)]}>")
            for tbl, tag in (("itempattern", "I"), ("spellpattern", "S")):
                for r in out.get(tbl, []):
                    if self.copyable(r, src, copy) and r["do"] == "leave" and r["why"] in soft and r["key"][0] in need:
                        r["cat"], r["why"], r["do"] = ("HL" if src == "H" else "LH"), "needed by a row this plan writes", copy
            got = {r["key"][0] for t in ("itempattern", "spellpattern") for r in out.get(t, []) if r["do"] == copy}
            for t in ("itemeffect", "itemaura", "itemfactions", "foodeffect", "gemsetting", "norentvanish", "itemspeed",
                      "itemhide", "itemspell", "lootdroptrigger", "itemoutfit", "spelleffect", "spellarea", "petstats",
                      "spellrecastgroup"):
                for r in out.get(t, []):
                    if self.copyable(r, src, copy) and r["do"] == "leave" and r["why"] in soft and r["key"][0] in got:
                        r["cat"], r["why"], r["do"] = ("HL" if src == "H" else "LH"), "comes with its record", copy
        # records (items, spells) copied only for rows kept off: kept off with them; what is attached to a kept record stays
        for src, acts in (("H", H2L), ("L", L2H)):
            copy = "copy_H_to_L" if src == "H" else "copy_L_to_H"
            for tbl, tag in (("itempattern", "I"), ("spellpattern", "S")):
                pat = re.compile(re.escape(tag) + r"<([^<>]*)>")
                users = collections.defaultdict(set)
                for t, rows in out.items():
                    if t == tbl:
                        continue
                    for r in rows:
                        if r["action"] not in acts:
                            continue
                        blob = json.dumps({x: r.get(x) for x in ("key", "H", "L", "changed", "Honly", "Lonly")}, default=str)
                        for m in set(pat.findall(blob)):
                            users[m].add(r["do"] != "leave")
                for r in out.get(tbl, []):
                    if r["do"] != copy:
                        continue
                    u = users.get(r["key"][0][len(tag) + 1:-1])
                    if u and True not in u:
                        keep(r, "b", "needed only by rows kept on their shard")
            kept = {r["key"][0] for t in ("itempattern", "spellpattern") for r in out.get(t, [])
                    if r["do"] == "leave" and r["action"] == copy}
            for t in ("itemeffect", "itemaura", "itemfactions", "foodeffect", "gemsetting", "heldbarred", "norentvanish",
                      "itemspeed", "itemhide", "itemspell", "lootdroptrigger", "itemoutfit", "spelleffect", "spellarea",
                      "petstats", "spellrecastgroup"):
                for r in out.get(t, []):
                    if r["do"] == copy and r["key"][0] in kept:
                        keep(r, "b", "its record is kept on its shard")


# =====================================================================================================================
# the plan: the engine's writer run on a scratch copy of each shard, every row change captured
# =====================================================================================================================
class Capture(ss.Applier):
    def __init__(self, *a, **k):
        super().__init__(*a, **k)
        self.cap = []
        self.unit = None
        self.sigchange = set()

    def insert(self, t, values, detail):
        try:
            return super().insert(t, values, detail)
        except sqlite3.IntegrityError as e:   # a same-named duplicate's second row, say: that row is left, listed
            raise ss.Unresolved(f"{t}: {e}")

    def log(self, t, pk, before, after, detail):
        self.cap.append({"t": t, "pk": pk, "before": before, "after": after, "why": detail, "unit": self.unit})

    def detail(self, r):
        return f"{CATS_SHORT.get(r.get('cat'), '')}: {r.get('why') or r['reason']}"[:240]

    def map_progress(self, q, old_steps, new_steps, det):
        # no character's progress is moved: the quest's steps changed shape; --apply leaves the quest whole while a
        # character is part-way
        self.sigchange.add(q)

    def op_quest(self, qkey, rs):
        self.unit = "quest:" + str(qkey)
        try:
            super().op_quest(qkey, rs)
        finally:
            self.unit = None


CATS_SHORT = {"HL": "from Heirloom", "LH": "from Live", "R-dangling": "dangling row", "R-noaudit": "no audit",
              "R-both": "edited on both", "R-deletes": "one-side delete", "R-copy": "copied over"}

# Historian Tsali on the hill north of Freeport's north gate (the guide: "Go out the North gate, and run North by
# Northeast for approximately 300yds ... He is on top of a large hill"; a player's map mark at (25492, 15291)). The
# floor is cmd/campground's (the ground package's Stand over world 0's ground): 77.88 on the hilltop, nothing placed
# over it, dry. He faces south, toward the gate. A copy of row 1681 (Neriak's Tsali: body, level, type, gear); his
# respawn as 1681's; the Citizens of Freeport as his faction where he stands (npcfaction by name and place, as the
# city's people: Neriak's row covers 1681's spot alone). Quest 101251 is given by name (handlers/quests.go: foldName
# of the giver), so he starts it as the Neriak one does.
TSALI = {"name": "Historian Tsali", "world": 0, "x": 25492.0, "y": 77.88, "z": 15291.0, "facing": 0, "from_row": 1681,
         "faction": "Citizens of Freeport"}
TSALI_WHY = ("Historian Tsali on the hill north of Freeport's north gate (Live petition 5493; the guide's place, a player's "
             "map mark): a copy of row 1681")


def add_tsali(ap):
    db, T = ap.db, TSALI
    near = db.execute("SELECT id FROM npcs WHERE lower(npc_name) = lower(?) AND world = ? AND "
                      "abs(x_coord - ?) < 30 AND abs(z_coord - ?) < 30", (T["name"], T["world"], T["x"], T["z"])).fetchall()
    if near:
        return "there"
    cur = db.execute("SELECT * FROM npcs WHERE id = ? AND npc_name = ?", (T["from_row"], T["name"]))
    src = cur.fetchone()
    if src is None:
        return "no row 1681 named Historian Tsali here"
    src = dict(zip([d[0] for d in cur.description], src))
    vals = {c: v for c, v in src.items() if c != "id"}
    vals.update(x_coord=T["x"], y_coord=T["y"], z_coord=T["z"], facing=T["facing"], world=T["world"], dt_kill=None)
    ap.unit = "tsali"
    try:
        pk = ap.insert("npcs", vals, TSALI_WHY)
        rs = db.execute("SELECT * FROM npcrespawn WHERE npc_row = ?", (T["from_row"],))
        r = rs.fetchone()
        if r is not None:
            r = dict(zip([d[0] for d in rs.description], r))
            r["npc_row"] = pk["id"]
            ap.insert("npcrespawn", r, TSALI_WHY + ": his respawn as 1681's")
        if ss.has(db, "npcfaction"):
            ap.insert("npcfaction", {"name": T["name"].lower(), "world": T["world"], "x": T["x"], "z": T["z"], "radius": 5.0,
                                     "faction": T["faction"], "note": "Freeport's Historian Tsali (petition 5493)",
                                     "source": "live-sync-1122"}, TSALI_WHY + ": the Citizens of Freeport where he stands")
    finally:
        ap.unit = None
    return "added"


def unify_items(sh):
    """A record each shard minted on its own (an id from 960000 up) is matched by name, slot and level; when a GM changed
    one shard's slot (the weaponsmithing blades: Heirloom's 'Volatile Brass Short Blade' a slot-12 weapon, Live's 14),
    the two never matched and every recipe naming it pointed at a record the other shard lacked. A name that is ONE
    such record on each shard, the same level, neither identity on the other shard, is one record."""
    def mine(s):
        out = collections.defaultdict(list)
        for pid, ident in sh.ITEM[s].items():
            if pid >= ss.SHARED_ITEM_IDS and ident.startswith("n:"):
                name, slot, lvl = ident[2:].rsplit("|", 2)
                out[(name, lvl)].append((pid, ident))
        return out
    mh, ml = mine("H"), mine("L")
    have = {s: set(sh.ITEM[s].values()) for s in "HL"}
    n = 0
    for k, hs in mh.items():
        ls_ = ml.get(k)
        if len(hs) != 1 or not ls_ or len(ls_) != 1:
            continue
        (hp, hi), (lp, li) = hs[0], ls_[0]
        if hi == li or hi in have["L"] or li in have["H"]:
            continue
        sh.ITEM["L"][lp] = hi
        n += 1
    if n:
        for s in "HL":
            m = collections.defaultdict(list)
            for i, ident in sorted(sh.ITEM[s].items()):
                m[ident].append(i)
            sh.IDS[("item", s)] = m
    return n


DANGLING_NPC_COLS = {"npcwander": "npcid", "npcrespawn": "npc_row", "npcpaths": "npcid", "npcstands": "npc_row",
                     "npcrare": "npc_row", "npctitle": "npcid"}


def keep_mapped(sh, w, ap, dst):
    """A dangling row whose NPC (a row of the shared database) this plan brings back to its shard is kept, under the NPC's
    new id there (the delete the plan made of it is followed by the same row, mapped)"""
    n = 0
    other = ss.OTHER[dst]
    back = {i for r in sh.out.get("npcs", []) if r.get("action") == f"copy_{other}_to_{dst}" for i in (r.get(other + "id") or [])}
    there = {i for r in sh.out.get("npcs", []) if r.get("action") == f"copy_{dst}_to_{other}" for i in (r.get(dst + "id") or [])}
    for t, col in DANGLING_NPC_COLS.items():
        for r in sh.out.get(t, []):
            if r.get("cat") != "R-dangling":
                continue
            m = re.search(r"\?missing (\d+)", json.dumps(r["key"]))
            ref = int(m.group(1)) if m else None
            if ref is None or ref >= ss.SHARED_NPC_IDS:
                continue
            if r.get("action") == "delete_on_" + other and ref in there:
                # the other shard's row, kept there under the restored NPC's new id: the NPC's own row here has it too
                for raw in r.get(other + "raw") or []:
                    vals = {k: v for k, v in raw.items() if k != "__rowid"}
                    if ap.fetch(t, {col: ref}) is None and ref in sh.NPCROW[dst]:
                        ap.insert(t, vals, f"dangling row kept on {ss.NAMES[other]} (its NPC {ref} restored there): its NPC here has it too")
                        n += 1
                continue
            if r.get("action") != "delete_on_" + dst or ref not in back:
                continue
            new = w.new_ids["npc"].get(ref)
            if not new:
                continue
            for raw in r.get(dst + "raw") or []:
                vals = {k: v for k, v in raw.items() if k != "__rowid"}
                vals[col] = new
                if ap.fetch(t, {col: new}) is None:
                    ap.insert(t, vals, f"dangling row kept: its NPC ({ref}) is restored here as {new}")
                    n += 1
                    r["why"] = f"its NPC ({ref}) is restored on {ss.NAMES[dst]} by this plan: kept, mapped to {new}"
    return n


def make_plan(hpath, lpath, plan_path, report_path):
    t0 = time.time()
    if ss.role_of(hpath) != "H" or ss.role_of(lpath) != "L":
        sys.exit("REFUSED: --heirloom must be Heirloom's database and --live Live's (heirlooms rows decide)")
    sh = ss.Shards(hpath, lpath)
    n = unify_items(sh)
    print(f"{n} records each shard minted apart matched by name and level (a slot changed on one)", flush=True)
    sh.compare()
    sh.classify_all()
    print(f"compared in {time.time() - t0:.0f} s", flush=True)
    srt = Sorter(sh)
    srt.run()
    for t, rows in sh.out.items():
        for r in rows:
            r["_t"] = t
            r["orig_action"] = r["action"]
            r["action"] = r["do"] if r["cat"] in WRITES else "leave"
    path = {"H": hpath, "L": lpath}
    plan = {"version": PLAN_VERSION, "made": now(), "heirloom": os.path.abspath(hpath), "live": os.path.abspath(lpath)}
    skipped = collections.defaultdict(list)
    tsali = {}
    mapped = 0
    for src in ("H", "L"):
        dst = ss.OTHER[src]
        for s in "HL":   # fresh read-only connections for each direction
            try:
                sh.db[s].close()
            except Exception:
                pass
            sh.db[s] = sqlite3.connect(f"file:{path[s]}?mode=ro", uri=True, timeout=30)
            sh.db[s].row_factory = sqlite3.Row
        sh.H, sh.L = sh.db["H"], sh.db["L"]
        w = ss.Writer(sh, src)
        ops = w.ops()
        w.allocate(ops)
        fd, scratch = tempfile.mkstemp(prefix="livesync-", suffix=".db", dir=os.path.dirname(os.path.abspath(plan_path)))
        os.close(fd)
        try:
            a = sqlite3.connect(f"file:{path[dst]}?mode=ro", uri=True)
            b = sqlite3.connect(scratch)
            a.backup(b)   # page for page: rowids as the shard has them
            a.close()
            b.close()
            ap = Capture(sh, w, scratch, "plan")
            ap.db.execute("BEGIN IMMEDIATE")
            ap.run_ops(ops)
            mapped += keep_mapped(sh, w, ap, dst)
            tsali[dst] = add_tsali(ap)
            ap.db.execute("ROLLBACK")
            ap.db.close()
        finally:
            for p in (scratch, scratch + "-journal", scratch + "-wal", scratch + "-shm"):
                if os.path.exists(p):
                    os.remove(p)
        for s in ap.skipped:
            skipped[(s["table"], json.dumps(s["key"], default=str))].append(f"to {ss.NAMES[dst]}: " + s["why"])
        sig = set()
        for q in ap.sigchange:
            v = sh.QUEST[dst].get(q)
            if v is not None:
                sig.add(f"quest:Q<{v}>")
        plan[dst] = {"ops": ap.cap, "sigchange": sorted(sig)}
        print(f"to {ss.NAMES[dst]}: {len(ap.cap)} row changes; Tsali: {tsali[dst]}", flush=True)
    for t, rows in sh.out.items():
        for r in rows:
            k = (t, json.dumps(r["key"], default=str))
            if r["cat"] in WRITES and k in skipped:
                r["cat"], r["why"] = "c", "could not be written: " + skipped[k][0][:180]
    report = build_report(sh)
    report["tsali"] = tsali
    report["dangling_kept_and_mapped"] = mapped
    plan["summary"] = report["summary"]
    with open(plan_path, "w") as f:
        json.dump(plan, f, separators=(",", ":"), default=str)
    if report_path:
        with open(report_path, "w") as f:
            json.dump(report, f, indent=0, default=str)
    print_summary(report["summary"])
    print(f"plan -> {plan_path} ({time.time() - t0:.0f} s)")
    if report_path:
        print(f"report: {report_path}")
    return plan, report


def direction(r):
    d = r.get("action") if r.get("cat") in WRITES else "leave"
    return "to Live" if d in H2L else ("to Heirloom" if d in L2H else "-")


def build_report(sh):
    rows = []
    cnt = collections.Counter()
    why = collections.Counter()
    for t in ss.ORDER:
        for r in sh.out.get(t, []):
            cat = r["cat"]
            dr = direction(r)
            cnt[(cat, dr, topic(t), t)] += 1
            if cat == "same":
                continue
            why[(cat, dr, t, r["why"][:150])] += 1
            raw = ((r.get("Hraw") or []) + (r.get("Lraw") or []) + [{}])[0]
            name = next((raw[c] for c in ("itemname", "npc_name", "name", "spellname", "tablename", "npc") if raw.get(c)), "")
            rows.append({"cat": cat, "dir": dr, "topic": topic(t), "table": t, "key": r["key"], "name": name,
                         "engine": r.get("orig_action", r["action"]), "do": r.get("action"), "why": r["why"],
                         "engine_reason": r.get("reason"), "changed": r.get("changed"), "Hid": r.get("Hid"), "Lid": r.get("Lid")})
    summary = collections.OrderedDict()
    for cat in CATS:
        per = collections.OrderedDict()
        for (c, dr, tp, t), n in sorted(cnt.items()):
            if c == cat:
                per.setdefault(f"{tp} ({dr})" if dr != "-" else tp, {})[t] = n
        summary[cat] = per
    summary["reasons"] = [{"cat": c, "dir": d, "table": t, "why": w, "rows": n} for (c, d, t, w), n in why.most_common()]
    return {"summary": summary, "rows": rows}


def print_summary(summary):
    for cat, label in CATS.items():
        per = summary.get(cat) or {}
        tot = sum(sum(v.values()) for v in per.values())
        print(f"[{cat}] {label}: {tot} rows")
        for tp, ts in per.items():
            print(f"    {tp:40} {sum(ts.values()):6}  " + ", ".join(f"{t} {n}" for t, n in sorted(ts.items(), key=lambda x: -x[1])))


# =====================================================================================================================
# --apply: one shard's half of the plan replayed on it, each row checked
# =====================================================================================================================
LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    run    TEXT NOT NULL,
    tbl    TEXT NOT NULL,
    pk     TEXT NOT NULL,      -- the row's key (JSON)
    before TEXT,               -- the whole row before (JSON, its rowid as _rowid_ when deleted; NULL: there was none)
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
    if t in IDENTITY or t == "npcrespawn":
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


def apply_plan(dbpath, plan_path):
    with open(plan_path) as f:
        plan = json.load(f)
    if plan.get("version") != PLAN_VERSION:
        sys.exit("REFUSED: not a plan of this tool's version (make a new plan)")
    side = ss.role_of(dbpath)
    name = ss.NAMES[side]
    half = plan.get(side) or {"ops": [], "sigchange": []}
    t0 = time.time()
    db = sqlite3.connect(dbpath, timeout=60, isolation_level=None)
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if ss.has(db, "accountinfo") else None
    acct = acct[0] if acct else 0
    run = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%SZ") + "-" + side
    detail = DETAIL + f" (on {name})"
    ops = half["ops"]
    sig = set(half.get("sigchange") or [])
    done, there, left = collections.Counter(), 0, []
    seqs = {}
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_DDL)
        db.execute(SEQ_DDL)
        # units: a quest (or Tsali with his rows) goes whole or not at all
        units = collections.defaultdict(list)
        for i, op in enumerate(ops):
            if op.get("unit"):
                units[op["unit"]].append(i)
        skip = set()
        for u, idx in units.items():
            sts = [state(db, ops[i])[0] for i in idx]
            if all(s == "there" for s in sts):
                continue
            bad = [(ops[i]["t"], ops[i]["pk"], s) for i, s in zip(idx, sts) if s in ("changed", "taken")]
            if bad:
                left.append({"unit": u, "why": f"changed on {name} since the plan", "rows": bad[:5]})
                skip.update(idx)
                continue
            if u in sig:
                qids = {ops[i]["pk"].get("questid") for i in idx if ops[i]["t"] in ss.QUEST_TABLES}
                qids.discard(None)
                if qids and ss.has(db, "charquests"):
                    n = db.execute(f"SELECT COUNT(*) FROM charquests WHERE done = 0 AND questid IN ({','.join('?' * len(qids))})",
                                   tuple(qids)).fetchone()[0]
                    if n:
                        left.append({"unit": u, "why": f"its steps change shape and {n} {name} character(s) are part-way: "
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
                raise Refused(f"{t} {pk} is another row on {name} now (the plan inserts it): make a new plan")
            if st == "changed":
                left.append({"unit": None, "why": f"changed on {name} since the plan", "rows": [(t, pk)]})
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
                            jn(after) if after is not None else "", f"{detail}: {op.get('why', '')}; {TOOL}"[:900])).lastrowid
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
    print(f"APPLIED run {run} on {name}: {sum(done.values())} row changes, {there} already as the plan leaves them, "
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
    ap.add_argument("--heirloom", help="Heirloom's database: a copy for --plan; the one to write for --apply/--undo")
    ap.add_argument("--live", help="Live's database: a copy for --plan; the one to write for --apply/--undo")
    ap.add_argument("--plan", help="the plan file: written without --apply, replayed with it")
    ap.add_argument("--report", help="with a new plan: every difference, its group and reason (JSON)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="replay the plan's half for the ONE database named")
    g.add_argument("--undo", action="store_true", help="undo this tool's runs on the ONE database named")
    a = ap.parse_args()
    if a.apply or a.undo:
        if bool(a.heirloom) == bool(a.live):
            ap.error("--apply and --undo write one database: name exactly one of --heirloom and --live")
        db = a.heirloom or a.live
        want = "H" if a.heirloom else "L"
        if ss.role_of(db) != want:
            sys.exit(f"REFUSED: {db} is not {ss.NAMES[want]}'s database (heirlooms rows decide)")
        if a.undo:
            undo(db)
            return
        if not a.plan:
            ap.error("--apply needs --plan")
        apply_plan(db, a.plan)
        return
    if not (a.heirloom and a.live and a.plan):
        ap.error("a plan needs --heirloom, --live (copies) and --plan")
    make_plan(a.heirloom, a.live, a.plan, a.report)


if __name__ == "__main__":
    try:
        main()
    except Refused as e:
        sys.exit(f"REFUSED (nothing written): {e}")
