#!/usr/bin/env python3
"""Bring the Heirloom Test Server's and the Live Server's content up to date with each other (the owner, 2026-11-15:
"make sure heirloom and live has the most up to date quests, items, etc.").

Each shard got fixes of its own: GM work on Heirloom (the GMs' accounts, the panel), Live-only petition fixes,
tools run on one database. This tool compares the two databases' CONTENT tables (items, quests, NPCs and their tables,
loot, merchants, spells, masteries, triggers, balance) by natural keys, decides which side is newer, and copies the
newer side over, one direction a run (--from -> --to).

How rows are matched (ids differ between the shards):
  - itempattern: a patternid below 960000 is the same record on both (renames kept the id); from 960000 up each shard
    minted its own, matched by lower(itemname)|equipslot|levelreq.
  - quests: a questid below 101471 is the same quest; from 101471 up (the imports interleaved) by source|name|giver.
  - npcs: first by the row's own id: an id below SHARED_NPC_IDS is a row of the database both shards were cloned
    from, the same NPC on both however far a GM moved it, whatever it was renamed to or remade as (petitions 2193,
    2194, 2198: the first run matched by position and copied every such row to the other shard, so it stood twice);
    an id made since with the same name and world on both is the same tool run. Then name|world|round(x)|round(z);
    else the nearest unpaired row of the name and world within 60; else a rename on the very spot; else the row the
    same audited insert made on the other shard (a GM's /clone or /place followed to the row it copies); else, for a
    named NPC, the one row of its name in its world on each shard. A paired NPC's name is compared as content.
    A row one shard has that the other deleted is never copied back: an audited delete of that id (whatever its
    name by then), of its twin (the same insert's row there), or a shared row gone with no audit (left, listed).
  - spellpattern: lower(spellname)|abilitylvl.
  - every reference (item, spell, quest and npcs id columns, '#<npcs id>' names, ids inside text and JSON) by what it
    points at; a row written to the other shard gets that shard's ids (new records: an id of its own sequence, or the
    same id when it is free and in the shared range).
  - a quest is compared and copied WHOLE (its row, steps and step targets: a step put in renumbers the rest);
    characters' progress (charquests) on a replaced quest is mapped onto the new steps by the steps' kind and target.

Which side is newer: each shard's OWN gm_audit edits for the row (an entry the other shard logged too, within 12 h
and with the same words, is the same tool run on both, and a mirror's "(from Heirloom" entry is a copy: neither
says which is newer). A row one shard has and the other's gm_audit shows deleted is deleted, not copied back.

Left as they are (shard-specific): Heirloom's config features (heirlooms, imports, familiars, Testing Heroes, Rock of
Freeport, custom races and all classes, worlds 10-15 and all of theirs), the Ascended copies, the Heirloom-only
merchants' stock (Outfitters, Spell Archivists, the homelands), Live's 60 s respawns of level 20 and under (board
1392), Live's own Silver and Fur (101391), auction lots, characters, accounts; rows that differ with no own edit on
either shard (listed for a person, never synced); and EXCLUDED, another lane's: quests 101301, 101304, 101306 and
Liztin Misseliania's stock.

The owner approved (2026-11-15) copying to Live these Heirloom-only changes as well: the melee balance, the camp
spread and wandering, a GM's one-shard calls (Lady Vox's 2 h, Bertoxxulous -30%, the Recruiter starts at L60, the
quests' race lists), the spell range pass, the GM-made items.

Report by default. --plan FILE lists proposed operations. Use the reviewed-ticket path in live_sync_20261122.py
for new releases. --undo restores the original November 15 log, checking each current row before restoring it.

    python3 -E -s tools/shard_sync_20261115.py --from <heirloom eqoa.db> --to <live eqoa.db> [--plan p.json | --undo]
    python3 -E -s tools/shard_sync_20261115.py --from <live eqoa.db> --to <heirloom eqoa.db> ...

Run Heirloom -> Live first, then Live -> Heirloom. Quests and templates load at the server's start (a keep restart);
NPCs: the panel's reload_npcs.
"""
import argparse
import collections
import datetime
import difflib
import json
import math
import re
import sqlite3
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parent)) if str(Path(__file__).resolve().parent) not in sys.path else None
import shard_refs as refs  # noqa: E402  the reference registry (2026-11-27)

TOOL = "tools/shard_sync_20261115.py"
LOG = "shard_sync_1115_log"
WHO, VIA = "dab", "tool"
NAMES = {"H": "Heirloom", "L": "Live"}

SHARED_ITEM_IDS = 960000
SHARED_QUEST_IDS = 101471
# npcs ids below this are the rows of the database both shards were cloned from (Live's first row of its own is 162560,
# 2026-11-08): such a row is the same NPC on both shards whatever a GM has done to it since (moved, renamed, a new body)
SHARED_NPC_IDS = 162560
FEATURE_WORLDS = {10, 11, 12, 13, 14, 15}   # Heirloom's server_worlds: homelands 10, CTF 11/12, homes 13, dungeons 14, siege 15
EXCLUDED_QUESTS = {101301, 101304, 101306}  # the Plane of Sky key quests: another lane's (2026-11-15)
EXCLUDED_MERCHANTS = {"liztin misseliania"}  # her shop: another lane's
EXCLUDED_WORK = re.compile(r"teamjosh|handg", re.I)  # the Hunt and Gather lane's quests, givers and items (2026-11-15), mid-flight
LIVE_OWN_QUESTS = {101391}                  # Silver and Fur: Live's own design
HEIRLOOM_ONLY_MERCHANTS = {"heirloom outfitter", "spell archivist", "testing heroes quartermaster"}
# NPCs of Heirloom-only features standing in the shared worlds (config: mercs, homes, dungeons, ctf, teleporter,
# testing_heroes; the Buffer and Dabrish, petitions 1390/1266, Heirloom only)
HEIRLOOM_ONLY_NPCS = HEIRLOOM_ONLY_MERCHANTS | {"mercenary recruitment", "mercenary liaison", "testing heroes vendor",
                                               "teleporter", "real estate listings", "heroic dungeons", "pvp events",
                                               "bruce buffer", "dabrish"}
IDENTITY_TABLES = ("npcs", "itempattern", "spellpattern", "quests")
RECRUITER_STARTS = {101032, 101033, 101041, 101042, 101043, 101054, 101055, 101056, 101064, 101065, 101066, 101074,
                    101075, 101085, 101093, 101094, 101095, 101105, 101106}
JIB_CALLS = {
    ("npcrespawn", "lady vox"): "board 1093: Lady Vox 2 h repop (jibrish; owner-approved for Live 2026-11-15)",
    ("npcrespawn", "enraged lady vox"): "board 1093: Enraged Lady Vox 2 h repop (jibrish; owner-approved for Live 2026-11-15)",
    ("classtemplates", "bertoxxulous"): "board 1299: Bertoxxulous -30% damage (jibrish; owner-approved for Live 2026-11-15)",
}
CUSTOM_RACE_WORDS = re.compile(r"wisp|jal'?raeth|mermaid|homelands|custom race|racehomes|races11\d\d|open.races|racial", re.I)
COSMETIC = {"source", "note", "made", "notes"}
D_GROUPS = ("heirloom-worlds", "heirloom-items", "heirloom-ascended", "heirloom-races", "live-own", "orphan",
            "used-only-by-shard-specific", "cosmetic", "excluded")
NPC_EXTRAS = (("npcrespawn", "npc_row"), ("npcwander", "npcid"), ("npcpaths", "npcid"), ("npcrare", "npc_row"),
              ("npctitle", "npcid"), ("signtext", "npcid"), ("campspawns", "npcid"), ("npcstands", "npc_row"),
              ("npcitem", "npcid"), ("questwalls", "npcid"))

# ---- the content tables: natural key, translations, columns not compared, the row's own id ---------------------
# trans: item | spell | quest | npc (an id column) ; itext | stext | qtext (ids inside text) ; jtext (a JSON column read
# by key, shard_refs.py: "#<npcs id>" strings, quest/spell/item/slot keys, items/spells id lists; 2026-11-27: with itext
# the "#<id>" spawns of gmtriggers.steps and queststeps.events were copied verbatim, so they named the other shard's rows)
SPECS = {
    "itempattern": dict(key=["patternid"], trans={"patternid": "item"}, lid="patternid"),
    "itemeffect": dict(key=["patternid"], trans={"patternid": "item"}, lid="patternid"),
    "itemaura": dict(key=["patternid"], trans={"patternid": "item"}, lid="patternid"),
    "itemfactions": dict(key=["patternid", "faction"], trans={"patternid": "item"}, lid="patternid"),
    "itemascended": dict(key=["patternid"], trans={"patternid": "item", "base": "item"}, ignore=["made", "source"], lid="patternid"),
    "itemimported": dict(key=["patternid"], trans={"patternid": "item", "base": "item"}, ignore=["made"], lid="patternid"),
    "heirlooms": dict(key=["patternid"], trans={"patternid": "item"}, lid="patternid"),
    "familiars": dict(key=["patternid"], trans={"patternid": "item"}, lid="patternid"),
    "heroes_stock": dict(key=["patternid"], trans={"patternid": "item"}, ignore=["id"], lid="patternid"),
    "foodeffect": dict(key=["patternid"], trans={"patternid": "item"}),
    "spellpattern": dict(key=["spellid"], trans={"spellid": "spell"}, lid="spellid"),
    "spelleffect": dict(key=["spellid"], trans={"spellid": "spell"}, lid="spellid"),
    "spellarea": dict(key=["spellid"], trans={"spellid": "spell"}, lid="spellid"),
    "spellgates": dict(key=["spell"]),
    "spellascended": dict(key=["spellid"], trans={"spellid": "spell", "base": "spell"}, ignore=["made", "source"]),
    "spellrecastgroup": dict(key=["spellid"], trans={"spellid": "spell"}),
    "classspells": dict(key=["class", "spellid"], trans={"spellid": "spell"}),
    "petstats": dict(key=["spellid"], trans={"spellid": "spell", "abilities": "stext"}, lid="spellid"),
    "pettemplate": dict(key=["spell", "weapon"]),
    "cmcatalog": dict(key=["id"], trans={"add_spells": "stext", "remove_spells": "stext", "require_spells": "stext",
                                         "require_quests": "qtext"}, lid="id"),
    "craftrecipes": dict(key=["id"], trans={"product": "item", "ingredients": "itext", "scroll": "item"}),
    "npcs": dict(key=["npc_name"], round_key=True, ignore=["id", "dt_kill", "hp_percent"], lid="id"),
    "npcrespawn": dict(key=["npc_row"], trans={"npc_row": "npc"}, lid="npc_row"),
    "npcwander": dict(key=["npcid"], trans={"npcid": "npc"}, lid="npcid"),
    "npcpaths": dict(key=["npcid"], trans={"npcid": "npc"}, lid="npcid"),
    "npcrare": dict(key=["npc_row"], trans={"npc_row": "npc"}, lid="npc_row"),
    "npctitle": dict(key=["npcid"], trans={"npcid": "npc"}, lid="npcid"),
    "npcstands": dict(key=["npc_row"], trans={"npc_row": "npc"}),
    "campspawns": dict(key=["npcid"], trans={"npcid": "npc"}),
    "signtext": dict(key=["npcid"], trans={"npcid": "npc"}, lid="npcid"),
    "gmportals": dict(key=["npc_id"], trans={"npc_id": "npc", "exit_npc_id": "npc"}, ignore=["id"]),
    "npctemplate": dict(key=["name"]),
    "classtemplates": dict(key=["name"], trans={"abilities": "stext"}),
    "npcspawnonly": dict(key=["name"]),
    "npcmove": dict(key=["name"]),
    "npcaggro": dict(key=["npc_name"]),
    "npccharm": dict(key=["name"]),
    "npcclass": dict(key=["name"]),
    "npcsocial": dict(key=["npc_name"]),
    "npcundead": dict(key=["name"]),
    "npctint": dict(key=["name"]),
    "npcdeath": dict(key=["name", "trigger"]),
    "npcoverseer": dict(key=["overseer", "boss", "world"], ignore=["id"]),
    "npcfaction": dict(key=["name", "world", "x", "z", "radius"], ignore=["id"]),
    "factionhits": dict(key=["npc_name", "faction"], ignore=["id"]),
    "factionhitareas": dict(key=["npc_name", "faction", "world", "x", "z"], ignore=["id"]),
    "factions": dict(key=["name"]),
    "npcloot": dict(key=["npc_name", "tablename"]),
    "loottables": dict(key=["tablename", "patternid"], trans={"patternid": "item"}, ignore=["id"], lid="id"),
    "lootpool": dict(key=["category", "region", "npc_name", "patternid"], trans={"patternid": "item"}, ignore=["id"], lid="id"),
    "lootcreature": dict(key=["npc_name"]),
    "lootrate": dict(key=["preset", "profile", "category"]),
    "lootpreset": dict(key=["name"]),
    "npcitem": dict(key=["npcid", "patternid", "listnumber"], trans={"npcid": "npc", "patternid": "item"}, ignore=["itemid"], lid="itemid"),
    "quests": dict(key=["questid"], trans={"questid": "quest", "events": "jtext", "requires": "qtext"}, lid="questid"),
    "queststeps": dict(key=["questid", "step"], trans={"questid": "quest", "spells": "stext", "items": "itext",
                                                       "rewarditems": "itext", "choices": "jtext", "byclass": "jtext",
                                                       "events": "jtext", "npcid": "npc"}),
    "queststeptargets": dict(key=["questid", "step", "target"], trans={"questid": "quest"}),
    "questunlocks": dict(key=["questid", "npc", "dest"], trans={"questid": "quest"}),
    "questwalls": dict(key=["npcid"], trans={"npcid": "npc", "item": "item", "questid": "quest"}, lid="npcid"),
    "questmarks": dict(key=["questid"], trans={"questid": "quest"}),
    "questnotyet": dict(key=["city", "line"], ignore=["id"], lid="id"),
    "npcdialog": dict(key=["town", "role", "name", "mood", "race", "sex", "speaker_race", "away", "line"], ignore=["id"], lid="id"),
    "gmtriggers": dict(key=["name"], ignore=["slot"], trans={"steps": "jtext"}, lid="slot"),
    "hazards": dict(key=["name", "world"], ignore=["id"]),
    "orbs": dict(key=["name", "world"], ignore=["id"]),
    "noroam": dict(key=["name", "world"], ignore=["id"]),
    "racedefs": dict(key=["id"]),
    "racesize": dict(key=["race"]),
    "racearmorbody": dict(key=["race", "armor"]),
    "startitems": dict(key=["race", "class", "humtype", "patternid"], trans={"patternid": "item"}, ignore=["id"]),
    "defaultcharacter": dict(key=["tclass", "race", "humType"], ignore=["defaultCharacterID"]),
    "defaultcharinventory": dict(key=["tclass", "patternid", "equiploc", "listnumber"], trans={"patternid": "item"}, ignore=["itemid"]),
    "defaultspells": dict(key=["tclass", "spellid"], trans={"spellid": "spell"}, ignore=["def_spell_id"]),
    "balanceclass": dict(key=["class"]), "balanceclasslevel": dict(key=["class", "level"]),
    "balancecon": dict(key=["diff"]), "balancelevel": dict(key=["level"]),
    "fishcatch": dict(key=["itemid", "world", "minx", "minz"], trans={"itemid": "item"}, ignore=["id"]),
    "rehabsettings": dict(key=["setting"]),
    "mapmarks": dict(key=["world", "name"], ignore=["id", "created", "updated", "suggestion_id"]),
    "charactermodel": dict(key=["modelid"]),
}
for _s in SPECS.values():
    if not _s.get("lid"):
        for _c in ("id", "slot", "itemid", "defaultCharacterID", "spellid", "npcid", "npc_row"):
            if _c in (_s.get("ignore") or []) or _c in _s["key"]:
                _s["lid"] = _c
                break
# the order operations run in: what others point at first
ORDER = ["itempattern", "itemeffect", "itemaura", "itemfactions", "foodeffect", "itemascended", "itemimported",
         "heirlooms", "familiars", "heroes_stock", "spellpattern",
         "spelleffect", "spellarea", "spellascended", "spellrecastgroup", "petstats", "pettemplate", "classspells",
         "spellgates", "cmcatalog", "craftrecipes", "npcs", "npcrespawn", "npcwander", "npcpaths", "npcrare", "npctitle",
         "npcstands", "campspawns", "signtext", "gmportals", "npctemplate", "classtemplates", "npcspawnonly", "npcmove",
         "npcaggro", "npccharm", "npcclass", "npcsocial", "npcundead", "npctint", "npcdeath", "npcoverseer", "factions",
         "npcfaction", "factionhits", "factionhitareas", "lootpreset", "lootrate", "lootcreature", "npcloot",
         "loottables", "lootpool", "npcitem", "quests", "queststeps", "queststeptargets", "questunlocks", "questwalls",
         "questmarks", "questnotyet", "npcdialog", "gmtriggers", "hazards", "orbs", "noroam", "racedefs", "racesize",
         "racearmorbody", "startitems", "defaultcharacter", "defaultcharinventory", "defaultspells", "balanceclass",
         "balanceclasslevel", "balancecon", "balancelevel", "fishcatch", "rehabsettings", "mapmarks", "charactermodel"]
assert set(ORDER) == set(SPECS), set(SPECS) ^ set(ORDER)
QUEST_TABLES = ("quests", "queststeps", "queststeptargets")

NUM = re.compile(r"(?<![\w.])(\d{4,7})(?![\w.])")
ROWREF = re.compile(r"^#(\d+)$")
CLONE_OF = re.compile(r"^/(?:clone|place) in the game: .*\(row (\d+)\)")
OTHER = {"H": "L", "L": "H"}


class Unresolved(Exception):
    pass


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def has(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone() is not None


def readonly(path):
    return sqlite3.connect(Path(path).resolve().as_uri() + "?mode=ro", uri=True, timeout=30)


def cols(db, t):
    return [r[1] for r in db.execute(f'PRAGMA table_info("{t}")')]


def pkcols(db, t):
    pk = sorted((r[5], r[1]) for r in db.execute(f'PRAGMA table_info("{t}")') if r[5])
    return [c for _, c in pk]


def go_json(m):
    return None if m is None else json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def _t(at):
    try:
        return datetime.datetime.fromisoformat(at.replace("Z", "+00:00")[:25])
    except Exception:
        return None


# =====================================================================================================================
# the comparison (both databases, read-only)
# =====================================================================================================================
class Shards:
    def __init__(self, hpath, lpath):
        self.path = {"H": hpath, "L": lpath}
        self.db = {}
        for s in "HL":
            c = readonly(self.path[s])
            c.row_factory = sqlite3.Row
            self.db[s] = c
        self.H, self.L = self.db["H"], self.db["L"]
        self._maps()
        self._audit()

    # ---- id -> natural key ---------------------------------------------------------------------------------------
    def _maps(self):
        self.ITEM, self.ITEMNAME, self.SPELL, self.NPCROW, self.QUEST = {}, {}, {}, {}, {}
        self.TRIG, self.NPCMODEL = {}, {}
        for s, db in self.db.items():
            self.ITEM[s] = {r[0]: (f"#{r[0]}" if r[0] < SHARED_ITEM_IDS else f"n:{(r[1] or '').lower()}|{r[2]}|{r[3]}")
                            for r in db.execute("SELECT patternid, itemname, equipslot, levelreq FROM itempattern")}
            self.ITEMNAME[s] = {r[0]: r[1] for r in db.execute("SELECT patternid, itemname FROM itempattern")}
            self.SPELL[s] = {r[0]: f"{(r[1] or '').lower()}|{r[2]}" for r in db.execute("SELECT spellid, spellname, abilitylvl FROM spellpattern")}

            self.NPCROW[s] = {r[0]: (r[1] or "", r[2], r[3] or 0, r[4] or 0) for r in
                              db.execute("SELECT id, npc_name, world, x_coord, z_coord FROM npcs")}
            self.QUEST[s] = {r[0]: (f"#{r[0]}" if r[0] < SHARED_QUEST_IDS else f"s:{r[1]}|{r[2]}|{r[3]}")
                             for r in db.execute("SELECT questid, source, name, giver FROM quests")}
            self.TRIG[s] = ({r[0]: (r[1] or "").strip().lower() for r in db.execute("SELECT slot, name FROM gmtriggers")}
                            if has(db, "gmtriggers") else {})
            self.NPCMODEL[s] = {r[0]: r[1] for r in db.execute("SELECT id, modelid FROM npcs")}
        # spells: an id both shards have is the same spell (renames kept it: 900429 Dead Hills Port, 700020 Mortal
        # Mask's level); one shard's own id (each minted the Ascended epic spells apart) by lower(name)|level
        both = set(self.SPELL["H"]) & set(self.SPELL["L"])
        for s in "HL":
            self.SPELL[s] = {i: (f"#{i}" if i in both else f"n:{v}") for i, v in self.SPELL[s].items()}
        # NPC pairing. First by the row's own id: a row of the shared database (below SHARED_NPC_IDS) is the same NPC
        # on both shards however far a GM moved it, whatever it was renamed to (petition 2193: a move of 119 was
        # copied back as a second NPC on each shard); a row made since under the same id, name and world is the same
        # tool run on both. Then the rest by name and position, then by the insert that made them (twins).
        def k(n, w, x, z):
            return f"{n}|w{w}|{round(x)}|{round(z)}"
        NH = {i: k(*v) for i, v in self.NPCROW["H"].items()}
        NL = {}
        pair = {}  # live id -> heirloom id
        byexact = collections.defaultdict(list)
        grp = collections.defaultdict(list)
        for i, v in self.NPCROW["H"].items():
            byexact[NH[i]].append(i)
            grp[(v[0].lower(), v[1])].append(i)
        used = set()
        for i, v in sorted(self.NPCROW["L"].items()):
            h = self.NPCROW["H"].get(i)
            if h is not None and (i < SHARED_NPC_IDS or (h[1] == v[1] and h[0].strip().lower() == v[0].strip().lower())):
                used.add(i); NL[i] = NH[i]; pair[i] = i
        pend = []
        for i, v in sorted(self.NPCROW["L"].items()):
            if i in pair:
                continue
            kk = k(*v)
            cand = [h for h in byexact.get(kk, []) if h not in used]
            if cand:
                used.add(cand[0]); NL[i] = kk; pair[i] = cand[0]
            else:
                pend.append(i)
        rest = []
        for i in pend:
            n, w, x, z = self.NPCROW["L"][i]
            best, bd = None, 60.0
            for h in grp.get((n.lower(), w), []):
                if h in used:
                    continue
                hn, hw, hx, hz = self.NPCROW["H"][h]
                d = math.hypot(hx - x, hz - z)
                if d < bd:
                    best, bd = h, d
            if best is not None:
                used.add(best); NL[i] = NH[best]; pair[i] = best
            else:
                rest.append(i)
        spot = collections.defaultdict(list)
        for h, v in self.NPCROW["H"].items():
            if h not in used:
                spot[(v[1], round(v[2]), round(v[3]))].append(h)
        made = self.MADE = {s: self._npc_inserts(db) for s, db in self.db.items()}
        twins = collections.defaultdict(list)   # (insert detail, name, world) -> unpaired Heirloom rows it made
        for h, (det, x0, z0, _) in made["H"].items():
            if h not in used and h in self.NPCROW["H"]:
                v = self.NPCROW["H"][h]
                twins[(det, v[0].strip().lower(), v[1])].append((h, x0, z0))
        for i in rest:
            v = self.NPCROW["L"][i]
            cand = [h for h in spot.get((v[1], round(v[2]), round(v[3])), []) if h not in used]
            if not cand and i in made["L"]:
                # the same audited insert (the same tool run) made it on each shard under its own id; nearest where
                # they were made, since a GM may have moved either
                det, x0, z0, _ = made["L"][i]
                tw = sorted((math.hypot(hx - x0, hz - z0), h) for h, hx, hz in
                            twins.get((det, v[0].strip().lower(), v[1]), []) if h not in used)
                if tw and tw[0][0] < 5:
                    cand = [tw[0][1]]
            if cand:
                used.add(cand[0]); NL[i] = NH[cand[0]]; pair[i] = cand[0]
            else:
                NL[i] = k(*v)
        # last: a named NPC (a capital letter), the one row of its name in its world on each shard and neither paired:
        # the same NPC, whatever a GM moved or remade (the Master Surgeon, made by an unaudited tool on each shard)
        one = {}
        for s in "HL":
            c = collections.Counter((v[0].strip().lower(), v[1]) for v in self.NPCROW[s].values())
            one[s] = {(v[0].strip().lower(), v[1]): i for i, v in self.NPCROW[s].items()
                      if v[0][:1].isupper() and c[(v[0].strip().lower(), v[1])] == 1}
        for nk, i in sorted(one["L"].items(), key=lambda x: x[1]):
            h = one["H"].get(nk)
            if h is not None and i not in pair and h not in used:
                used.add(h); NL[i] = NH[h]; pair[i] = h
        self.NPC = {"H": NH, "L": NL}
        self.PAIR = {"L": pair, "H": {h: l for l, h in pair.items()}}  # side -> {its id: the other's id}
        self.QREV = {s: collections.defaultdict(list) for s in "HL"}
        for s in "HL":
            for q, v in self.QUEST[s].items():
                self.QREV[s][f"Q<{v}>"].append(q)
        # identity -> ids on each side (for writing to the other)
        self.IDS = {}
        for kind, M in (("item", self.ITEM), ("spell", self.SPELL), ("quest", self.QUEST)):
            for s in "HL":
                m = collections.defaultdict(list)
                for i, ident in sorted(M[s].items()):
                    m[ident].append(i)
                self.IDS[(kind, s)] = m

    @staticmethod
    def _npc_inserts(db):
        """npcs id -> (the detail of the gm_audit insert that made it, x, z where it was made, its name then): a tool's or a GM's
        own words, not this tool's copies. A GM's /clone or /place of a row is followed to the row it copies, so a
        clone (and its clones) answers with the insert that made the first row (the Master Surgeon, petition 2193)."""
        own = {}
        if not has(db, "gm_audit"):
            return own
        for rk, after, detail in db.execute(
                "SELECT rowkey, after, detail FROM gm_audit WHERE tbl = 'npcs' AND action = 'insert' AND reverted_by IS NULL "
                "AND detail <> '' AND detail NOT LIKE 'shard sync%' ORDER BY id"):
            try:
                kk = json.loads(rk)
                i = int(kk["id"]) if isinstance(kk, dict) else int(kk)
                a = json.loads(after) if after else {}
                own[i] = (detail, float(a["x_coord"]), float(a["z_coord"]), str(a.get("npc_name") or "").strip().lower())
            except Exception:
                continue
        out = {}
        for i in own:
            j, seen = i, set()
            while j in own and j not in seen:
                seen.add(j)
                m = CLONE_OF.match(own[j][0])
                if not m or int(m.group(1)) == j:
                    break
                j = int(m.group(1))
            out[i] = own[j] if j in own else own[i]
        return out

    # ---- gm_audit ------------------------------------------------------------------------------------------------
    def _audit(self):
        self.IDX, self.DET, self.DELETED = {}, {}, {}
        for side, db in self.db.items():
            idx = collections.defaultdict(list)
            det = collections.defaultdict(list)
            dele = collections.defaultdict(list)
            for aid, at, action, user, tbl, rk, before, detail, rev in db.execute(
                    "SELECT id, at, action, username, tbl, rowkey, before, detail, reverted_by FROM gm_audit WHERE tbl <> ''"):
                detail = strip_basis(detail)
                det[(tbl, re.sub(r"\d+", "N", detail[:300]))].append(at)
                if not rk:
                    continue
                try:
                    kk = json.loads(rk)
                except Exception:
                    continue
                ent = (at, aid, action, user, detail[:300], rev)
                if isinstance(kk, dict):
                    vals = tuple(str(v) for _, v in sorted(kk.items()))
                    idx[(tbl, vals)].append(ent)
                    if len(vals) == 1:
                        idx[(tbl, vals[0])].append(ent)
                    for f, v in kk.items():
                        if f in ("name", "npc_name", "itemname", "spellname", "tablename", "questid", "patternid", "id",
                                 "npcid", "npc_row", "spellid", "slot"):
                            idx[(tbl, f, str(v))].append(ent)
                else:
                    vals = (str(kk),)
                    idx[(tbl, str(kk))].append(ent)
                if action == "delete" and rev is None:
                    nm = ""
                    try:
                        b = json.loads(before or "{}")
                        nm = str(b.get("npc_name") or b.get("name") or b.get("itemname") or "")
                    except Exception:
                        pass
                    dele[(tbl, vals)].append((at, aid, "delete", user, detail, nm))
            self.IDX[side], self.DET[side], self.DELETED[side] = idx, det, dele

    def deleted_twin(self, side, ids):
        """a row the same insert made on the other shard (its twin) that a GM deleted there, and no row of it left
        there: the NPC was deleted on the other shard under its own id (the Harvest Feast's NPCs, petition 2193)"""
        o = OTHER[side]
        for i in ids:
            mk = self.MADE[side].get(i)
            if not mk or i not in self.NPCROW[side]:
                continue
            twins = [j for j, (det, x, z, n) in self.MADE[o].items()
                     if det == mk[0] and n == mk[3] and math.hypot(x - mk[1], z - mk[2]) < 5]
            alive = [j for j in twins if j in self.NPCROW[o]]
            dead = [j for j in twins if j not in self.NPCROW[o] and any(
                    e[2] == "delete" and e[5] is None for e in self.IDX[o].get(("npcs", str(j)), []))]
            if dead and not alive:
                j = dead[0]
                e = [e for e in self.IDX[o][("npcs", str(j))] if e[2] == "delete"][-1]
                return (f"{NAMES[o]} deleted its own row of the same insert ({j}): audit {e[1]} {e[0][:16]} {e[3]}: "
                        f"{(e[4] or '(panel/game edit)')[:80]}; not copied back")
        return None

    def shared(self, side, tbl, ent):
        if not ent[4]:
            return False
        if re.search(r"mirror|\(from Heirloom|from Live|Heirloom's gm_audit|shard sync", ent[4], re.I):
            return True
        t0 = _t(ent[0])
        for at in self.DET[OTHER[side]].get((tbl, re.sub(r"\d+", "N", ent[4])), []):
            t1 = _t(at)
            if t0 and t1 and abs((t1 - t0).total_seconds()) < 12 * 3600:
                return True
        return False

    def last(self, side, tbl, *probes, unique=False):
        best = None
        for p in probes:
            if p is None:
                continue
            if isinstance(p, tuple) and len(p) == 2 and isinstance(p[0], str) and p[0] in (
                    "name", "npc_name", "itemname", "spellname", "tablename", "questid", "patternid", "id", "npcid",
                    "npc_row", "spellid", "slot"):
                kk = (tbl, p[0], str(p[1]))
            elif isinstance(p, tuple):
                kk = (tbl, tuple(str(x) for x in p))
            else:
                kk = (tbl, str(p))
            for e in self.IDX[side].get(kk, []):
                if e[5]:
                    continue
                if unique and self.shared(side, tbl, e):
                    continue
                if best is None or e[0] > best[0]:
                    best = e
        return best

    # ---- translation to natural keys (comparison) ---------------------------------------------------------------
    def tv(self, side, kind, v):
        if kind == "item":
            return v if v in (None, 0) else "I<" + self.ITEM[side].get(v, f"?missing {v}") + ">"
        if kind == "spell":
            return v if v in (None, 0) else "S<" + self.SPELL[side].get(v, f"?missing {v}") + ">"
        if kind == "quest":
            return v if v in (None, 0) else "Q<" + self.QUEST[side].get(v, f"?missing {v}") + ">"
        if kind == "npc":
            return v if v in (None, 0) else "N<" + self.NPC[side].get(v, f"?missing {v}") + ">"
        if kind == "slot":
            return v if v in (None, 0) else "T<" + self.TRIG[side].get(v, f"?missing {v}") + ">"
        M, tag = {"itext": (self.ITEM, "I"), "stext": (self.SPELL, "S"), "qtext": (self.QUEST, "Q")}[kind]
        if v is None or v == "":
            return v
        def tr(n):
            return f"{tag}<" + M[side][n] + ">" if n in M[side] else str(n)
        if isinstance(v, int):
            return tr(v)
        return NUM.sub(lambda m: tr(int(m.group(1))), str(v))

    def jtv(self, side, t, c, v):
        """a JSON column's value with its references by what they point at (shard_refs.normalize)"""
        if v is None or v == "":
            return v
        if refs.SITE.get((t, c)) != "json":
            raise AssertionError(f"{t}.{c} is jtext but not a JSON site of shard_refs.py")
        return refs.normalize(t, c, v, lambda k, i: self.tv(side, k, i))

    def norm(self, side, v):
        if isinstance(v, float):
            return round(v, 3)
        if isinstance(v, str):
            m = ROWREF.match(v)
            if m and int(m.group(1)) in self.NPC[side]:
                return "N<" + self.NPC[side][int(m.group(1))] + ">"
        return v

    def keyval(self, side, t, r):
        spec = SPECS[t]
        trans = spec.get("trans", {})
        def val(c):
            kind = trans.get(c)
            if kind == "jtext":
                return self.jtv(side, t, c, r[c])
            return self.tv(side, kind, r[c]) if kind else self.norm(side, r[c])
        allc = [c for c in r.keys() if c != "__rowid"]
        ign = set(spec.get("ignore", []))
        keyc = spec["key"]
        if spec.get("round_key"):
            k = (self.NPC[side][r["id"]],)
        else:
            k = tuple(val(c) for c in keyc)
        v = tuple((c, val(c)) for c in allc if c not in ign and (c not in keyc or spec.get("round_key")))
        return k, v

    def compare(self):
        diffs = {}
        for t in ORDER:
            if not (has(self.H, t) and has(self.L, t)):
                continue
            data = {}
            for s in "HL":
                d = collections.defaultdict(list)
                for r in self.db[s].execute(f'SELECT rowid AS __rowid, * FROM "{t}"'):
                    kk, v = self.keyval(s, t, r)
                    d[kk].append((v, dict(r)))
                data[s] = d
            rows = []
            for kk in set(data["H"]) | set(data["L"]):
                a, b = data["H"].get(kk, []), data["L"].get(kk, [])
                lid = SPECS[t].get("lid")
                base = {"key": list(kk), "Hraw": [x[1] for x in a], "Lraw": [x[1] for x in b],
                        "Hid": [x[1][lid] for x in a] if lid else [], "Lid": [x[1][lid] for x in b] if lid else []}
                if not b:
                    rows.append(dict(base, cls="a", H=[dict(x[0]) for x in a]))
                elif not a:
                    rows.append(dict(base, cls="b", L=[dict(x[0]) for x in b]))
                else:
                    ca, cb = collections.Counter(x[0] for x in a), collections.Counter(x[0] for x in b)
                    if ca == cb:
                        continue
                    if len(a) == 1 and len(b) == 1:
                        da, db_ = dict(a[0][0]), dict(b[0][0])
                        rows.append(dict(base, cls="c", changed={c: [da[c], db_.get(c)] for c in da if da[c] != db_.get(c)}))
                    else:
                        # the instances one side has and the other lacks, raw rows with them
                        honly, lonly = ca - cb, cb - ca
                        hr, lr = [], []
                        left = collections.Counter(honly)
                        for v, raw in a:
                            if left[v] > 0:
                                left[v] -= 1; hr.append(raw)
                        left = collections.Counter(lonly)
                        for v, raw in b:
                            if left[v] > 0:
                                left[v] -= 1; lr.append(raw)
                        rows.append(dict(base, cls="c", multi=True, Honly=[dict(x) for x in honly.elements()],
                                         Lonly=[dict(x) for x in lonly.elements()], Honly_raw=hr, Lonly_raw=lr))
            diffs[t] = rows
        self.diffs = diffs
        return diffs

    # ---- classification ------------------------------------------------------------------------------------------
    def classify_all(self):
        H, L = self.H, self.L
        diffs = self.diffs
        HW, LW = collections.defaultdict(set), collections.defaultdict(set)
        for M, db in ((HW, H), (LW, L)):
            for n, w in db.execute("SELECT lower(npc_name), world FROM npcs"):
                M[(n or "").rstrip("$ ")].add(w)

        def feature_only_name(n):
            n = (n or "").lower().rstrip("$ ")
            w = HW.get(n)
            return bool(w) and w <= FEATURE_WORLDS and not LW.get(n)

        def npc_world(kk):
            m = re.search(r"\|w(-?\d+)\|", kk or "")
            return int(m.group(1)) if m else None

        def npc_name(kk):
            kk = kk[2:-1] if kk.startswith("N<") else kk
            return kk.split("|")[0]
        self.npc_name = npc_name

        def ids(q):
            return {r[0] for r in H.execute(q)}
        feat = ids("SELECT patternid FROM heirlooms") | ids("SELECT patternid FROM itemimported") | \
            ids("SELECT patternid FROM familiars") | ids("SELECT patternid FROM heroes_stock")
        for pid, name in H.execute("SELECT patternid, itemname FROM itempattern"):
            ln = (name or "").lower()
            test_scroll = pid == 716971 and ln == "scroll of returning (test)"
            if test_scroll or ln.startswith("heirloom ") or "rock of freeport" in ln or ln.endswith("(imported)"):
                feat.add(pid)
        feat_ik = {self.ITEM["H"][p] for p in feat if p in self.ITEM["H"]}
        asc_only = set()
        for r in diffs.get("itemascended", []):
            if r["cls"] == "a":
                asc_only.update(r["Hid"])
        asc_ik = {self.ITEM["H"][p] for p in asc_only if p in self.ITEM["H"]}
        live_own_qk = {f"Q<{self.QUEST[s][q]}>" for s in "HL" for q in LIVE_OWN_QUESTS if q in self.QUEST[s]}
        excl_qk = {f"Q<{self.QUEST[s][q]}>" for s in "HL" for q in EXCLUDED_QUESTS if q in self.QUEST[s]}
        quest_feature = set()
        for qid, giver, src in H.execute("SELECT questid, giver, source FROM quests"):
            if feature_only_name(giver) or "homelands/" in (src or ""):
                quest_feature.add(f"Q<{self.QUEST['H'][qid]}>")

        def first_row(r):
            return (r.get("H") or r.get("L") or [{}])[0]

        def probe(t, r, side, unique=False):
            lid = r.get(side + "id") or []
            probes = []
            for i in lid:
                for f in ("id", "patternid", "questid", "npc_row", "npcid", "spellid", "slot"):
                    probes.append((f, i))
                probes.append(i)
            kk = r["key"]
            if t == "queststeps":
                for q in self.QREV[side].get(kk[0], []):
                    probes.append((q, kk[1])); probes.append(("questid", q))
            elif t == "quests":
                for q in self.QREV[side].get(kk[0], []):
                    probes.append(("questid", q))
            elif kk and not (isinstance(kk[0], str) and kk[0].startswith(("N<", "I<", "S<", "Q<"))):
                if isinstance(kk[0], str):
                    probes.append(("name", kk[0])); probes.append(("npc_name", kk[0]))
                probes.append(str(kk[0]))
                probes.append(tuple(str(x) for x in kk))
            e = None
            for tb in (t,) + (("npcs",) if t in ("npcrespawn", "npcwander", "npcpaths", "npctitle", "signtext") else ()):
                x = self.last(side, tb, *probes, unique=unique)
                if x and (e is None or x[0] > e[0]):
                    e = x
            return e

        def deleted_on(side, t, r, rowside):
            cands = [(str(i),) for i in (r.get(rowside + "id") or [])]
            kk = r["key"]
            if t == "queststeps":
                for q in self.QREV[rowside].get(kk[0], []):
                    cands.append((str(q), str(kk[1])))
            elif kk and all(not (isinstance(x, str) and x[:2] in ("N<", "I<", "S<", "Q<")) for x in kk):
                cands.append(tuple(str(x) for x in kk))
            for cnd in cands:
                for e in self.DELETED[side].get((t, cnd), []):
                    if t == "npcs" and e[5] and e[5].lower() != npc_name(kk[0]).lower() and \
                            not (cnd[0].isdigit() and int(cnd[0]) < SHARED_NPC_IDS):
                        continue
                    return e
            return None

        def decide(t, r, eh=None, el=None, do_probe=True):
            if do_probe:
                eh, el = probe(t, r, "H", unique=True), probe(t, r, "L", unique=True)
            def d(e):
                return f"{e[1]} {e[0]} {e[3]}: {(e[4] or '(panel/game edit)')[:80]}"
            if eh and not el:
                return "c", "update_L_from_H", "content", f"Heirloom's own edit ({d(eh)})"
            if el and not eh:
                return "c", "update_H_from_L", "content", f"Live's own edit ({d(el)})"
            if eh and el:
                if eh[0] >= el[0]:
                    return "c", "update_L_from_H", "conflict", f"both edited; Heirloom newer ({d(eh)} vs Live {d(el)})"
                return "c", "update_H_from_L", "conflict", f"both edited; Live newer ({d(el)} vs Heirloom {d(eh)})"
            return "c", "review", "content", "no own edit on either shard in gm_audit (same tool runs only, or unaudited)"

        def classify(t, r):
            cls = r["cls"]
            kk = r["key"]
            row = first_row(r)
            k0 = kk[0] if kk else ""
            src = str(row.get("source", "")) + " " + str(row.get("note", ""))
            qk = [x for x in kk if isinstance(x, str) and x.startswith("Q<")]
            if t == "quests":
                qk = [k0]
            if qk and any(q in excl_qk for q in qk):
                return "x", "leave", "excluded", "another lane's (the Plane of Sky key quests 101301/101304/101306)"
            if t == "npcitem" and isinstance(k0, str) and npc_name(k0).lower() in EXCLUDED_MERCHANTS:
                return "x", "leave", "excluded", "another lane's (Liztin Misseliania's shop)"
            if any(isinstance(x, str) and "?missing" in x for x in kk):
                return "d", "leave", "orphan", "points at an NPC or item its own shard no longer has"
            if cls == "c" and not r.get("multi"):
                ch = set(r["changed"])
                if ch <= COSMETIC:
                    return "c", "leave", "cosmetic", "only source/note text differs"
                if t == "npcs" and ch <= {"npc_id"}:
                    return "c", "leave", "cosmetic", "npc_id only"
            wk = [npc_world(x) for x in kk if isinstance(x, str) and x.startswith("N<")]
            if t == "npcs":
                wk = [npc_world(k0)]
            if wk and any(w is not None for w in wk) and all(w in FEATURE_WORLDS for w in wk if w is not None):
                return "d", "leave", "heirloom-worlds", "an NPC of Heirloom's own worlds (10-15)"
            if t in ("npctemplate", "classtemplates", "npcspawnonly", "npcmove", "npcaggro", "npccharm", "npcclass",
                     "npcsocial", "npcundead", "npctint", "npcdeath", "npcloot", "lootcreature", "factionhits",
                     "npcfaction", "npcdialog") and cls == "a":
                nm = k0 if t != "npcdialog" else (row.get("name") or kk[2])
                if t == "npcdialog" and ((kk[0] or "").lower() == "homelands" or "racehomes" in src):
                    return "d", "leave", "heirloom-worlds", "the homelands' dialog (world 10)"
                if t != "npcdialog" and isinstance(nm, str) and feature_only_name(nm):
                    return "d", "leave", "heirloom-worlds", "a creature only of Heirloom's own worlds"
                if re.search(r"dungeon|siege|homes board|ctf|merc ", src, re.I) and isinstance(nm, str) and not LW.get(nm.lower()):
                    return "d", "leave", "heirloom-worlds", f"Heirloom's dungeons/siege/homes/mercs ({src.strip()[:40]})"
            item_keys = [x[2:-1] for x in kk if isinstance(x, str) and x.startswith("I<")]
            if cls == "a" and item_keys and all(x in feat_ik for x in item_keys):
                return "d", "leave", "heirloom-items", "an item reserved for Heirloom"
            if t in ("heirlooms", "itemimported", "familiars", "heroes_stock"):
                return "d", "leave", "heirloom-items", f"{t}: a Heirloom-only feature"
            if isinstance(k0, str) and (k0.startswith("N<") or t == "npcs") and npc_name(k0).lower() in HEIRLOOM_ONLY_NPCS:
                return "d", "leave", "heirloom-items", f"{npc_name(k0)} and its rows (a Heirloom-only feature)"
            if isinstance(k0, str) and k0.lower().rstrip("$ ") in HEIRLOOM_ONLY_NPCS and cls == "a":
                return "d", "leave", "heirloom-items", f"{k0}: a Heirloom-only feature's NPC"
            if r.get("multi") and t in IDENTITY_TABLES and (not r.get("Honly") or not r.get("Lonly")):
                return "c", "leave", "cosmetic", "same-named duplicate records: one shard has one more copy"
            if cls in ("a", "b") and (t in ("itemascended", "spellascended") or
                                      (item_keys and all(x in asc_ik for x in item_keys))):
                return "d", "leave", "heirloom-ascended", "an Ascended copy (or its loot): left on its shard"
            if cls in ("a", "b") and any(x.startswith("n:ascended ") and not self.IDS[("item", OTHER["H" if cls == "a" else "L"])].get(x)
                                         for x in item_keys):
                return "d", "leave", "heirloom-ascended", "points at an Ascended copy the other shard lacks"
            if cls in ("a", "b") and t == "itempattern":
                nm = (row.get("itemname") or "")
                if nm.lower().startswith("ascended "):
                    return "d", "leave", "heirloom-ascended", "an Ascended copy: left on its shard"
            if cls == "a" and t in ("lootrate", "loottables", "lootpool") and re.search(r"ascend|sky twins|workboard 13(28|33)", str(kk) + src, re.I):
                return "d", "leave", "heirloom-ascended", "the tuned bosses' Ascended loot"
            if t in ("racedefs", "racesize", "defaultcharacter"):
                return "d", "leave", "heirloom-races", "open_races/all_classes are Heirloom only"
            if t == "startitems" and cls == "a" and (kk[0] >= 11 or re.search(r"races|genie|wolffang|summonportal|mermaid", src)):
                return "d", "leave", "heirloom-races", "start rows for custom races / the heirloom kit"
            if CUSTOM_RACE_WORDS.search(src) and cls in ("a", "c"):
                return "d", "leave", "heirloom-races", f"custom races' kit ({src.strip()[:50]})"
            if t == "cmcatalog" and cls == "a" and (row.get("races", 0) or 0) > 2047:
                return "d", "leave", "heirloom-races", "a mastery for custom races only"
            if qk and all(q in quest_feature for q in qk) and cls == "a":
                return "d", "leave", "heirloom-worlds", "a quest of the homelands (world 10)"
            if t == "questmarks":
                return "d", "leave", "heirloom-races", "petition 1813's first-quest marks (Heirloom)"
            if t == "npcrespawn" and cls in ("b", "c") and "live-lowlevel-1114" in json.dumps(r.get("L") or r.get("Lraw"), default=str):
                return "d", "leave", "live-own", "Live's 60 s low-level respawns (board 1392)"
            if qk and any(q in live_own_qk for q in qk):
                return "d", "leave", "live-own", "Silver and Fur (101391): Live's own design"
            if t == "spellpattern" and cls == "c" and not r.get("multi") and set(r["changed"]) == {"range"} \
                    and r["changed"]["range"] == [16544, 16528]:
                return "c", "update_L_from_H", "jib-spell-range", "jibrish's range pass 16528 -> 16544 (2026-11-11; owner-approved for Live)"
            jk = (t, npc_name(k0).lower() if isinstance(k0, str) else "")
            if jk in JIB_CALLS:
                return cls, "update_L_from_H" if cls == "c" else "copy_H_to_L", "owner-approved", JIB_CALLS[jk]
            if t == "quests" and cls == "c" and k0 in [f"Q<#{q}>" for q in RECRUITER_STARTS]:
                return cls, "quest_L_from_H", "owner-approved", "board 1125: Recruiter epic starts at level 60 (jibrish; owner-approved for Live)"
            if t == "quests" and cls == "c" and "races" in r.get("changed", {}) and set(r["changed"]) <= {"races", "notes", "source"}:
                return cls, "quest_L_from_H", "owner-approved", "board 1062/1125: race lists incl. the custom races (jibrish; owner-approved for Live)"
            if cls in ("a", "b"):
                side = "H" if cls == "a" else "L"
                dele = deleted_on(OTHER[side], t, r, side)
                if dele:
                    return cls, ("delete_on_H" if cls == "a" else "delete_on_L"), "deleted-on-other", \
                        f"{NAMES[OTHER[side]]} deleted it: audit {dele[1]} {dele[0][:16]} {dele[3]}: {(dele[4] or '(panel/game edit)')[:80]}"
                if t == "npcs":
                    tw = self.deleted_twin(side, r.get(side + "id") or [])
                    if tw:
                        return "d", "leave", "deleted-on-other", tw
                    gone = [i for i in (r.get(side + "id") or []) if i < SHARED_NPC_IDS and i not in self.NPCROW[OTHER[side]]]
                    if gone:
                        return "d", "leave", "deleted-unaudited", \
                            f"a row of the shared database {NAMES[OTHER[side]]} no longer has (deleted with no row audit): left, not copied back"
                e = probe(t, r, side)
                who = f"{e[3]} {e[0][:10]}: {(e[4] or '(panel/game edit)')[:90]}" if e else (
                    "source " + src.strip()[:60] if src.strip() else "no gm_audit")
                return cls, ("copy_H_to_L" if cls == "a" else "copy_L_to_H"), "content", who
            return decide(t, r)

        # a quest is decided whole
        def quest_level(qkey):
            res = {}
            for s in "HL":
                best = None
                for q in self.QREV[s].get(qkey, []):
                    for tb in ("quests", "queststeps"):
                        e = self.last(s, tb, ("questid", q), unique=True)
                        if e and (best is None or e[0] > best[0]):
                            best = e
                res[s] = best
            return res["H"], res["L"]

        qdec = {}
        for t in QUEST_TABLES:
            for r in diffs.get(t, []):
                qkey = r["key"][0]
                if qkey not in qdec:
                    eh, el = quest_level(qkey)
                    qdec[qkey] = decide("quests", r, eh, el, do_probe=False)
        # quests decided on the quests row (the owner-approved a GM calls) carry their steps with them
        out = {}
        for t in ORDER:
            res = []
            for r in diffs.get(t, []):
                cls, act, grp, why = classify(t, r)
                if t in QUEST_TABLES and grp not in D_GROUPS and grp != "owner-approved":
                    qkey = r["key"][0]
                    _, qact, qgrp, qwhy = qdec[qkey]
                    on_h, on_l = bool(self.QREV["H"].get(qkey)), bool(self.QREV["L"].get(qkey))
                    if qact == "review" and not (on_h and on_l):
                        qact = "copy_H_to_L" if on_h else "copy_L_to_H"
                        qwhy = "quest on one shard only"
                    act = {"update_L_from_H": "quest_L_from_H", "update_H_from_L": "quest_H_from_L"}.get(qact, qact)
                    if act in ("delete_on_L", "delete_on_H", "copy_H_to_L", "copy_L_to_H") and on_h and on_l:
                        act = "review"
                    grp, why = qgrp, "quest-level: " + qwhy
                rr = dict(r)
                rr.update(cls=cls, action=act, group=grp, reason=why)
                res.append(rr)
            out[t] = res
        # another lane's work, mid-flight: the Hunt and Gather quests, their givers and items
        for t, rows in out.items():
            for r in rows:
                if r["action"] != "leave" and EXCLUDED_WORK.search(
                        json.dumps([r["key"], r["reason"], (r.get("H") or r.get("L") or [{}])[0].get("source", "")], default=str)):
                    r.update(cls="x", action="leave", group="excluded", reason="another lane's (Hunt and Gather, handg): " + r["reason"])
        # an owner-approved quests row takes its whole quest with it
        approved_q = {r["key"][0] for r in out.get("quests", []) if r["group"] == "owner-approved"}
        for t in ("queststeps", "queststeptargets"):
            for r in out.get(t, []):
                if r["key"][0] in approved_q and r["group"] not in D_GROUPS:
                    r.update(action="quest_L_from_H", group="owner-approved", reason="its quest is owner-approved")
        # an NPC deleted on one shard takes its extras on the other with it
        npc_del = {s: {r["key"][0] for r in out["npcs"] if r["action"] == f"delete_on_{s}"} for s in "HL"}
        npc_gone = {r["key"][0] for r in out["npcs"] if r["group"] == "deleted-unaudited"}
        for t, _c in NPC_EXTRAS:
            for r in out.get(t, []):
                n0 = r["key"][0]
                if isinstance(n0, str) and n0.startswith("N<"):
                    if n0[2:-1] in npc_gone and r["action"] != "leave":
                        r.update(cls="d", action="leave", group="deleted-unaudited", reason="its NPC was deleted on the other shard (no row audit)")
                        continue
                    for s in "HL":
                        if n0[2:-1] in npc_del[s] and r["action"] not in ("leave",):
                            r.update(action=f"delete_on_{s}", group="deleted-on-other", reason="its NPC was deleted on the other shard")
        # loot tables: one that no Live row and no row to copy names stays with its users
        live_tables = {r[0] for r in L.execute("SELECT tablename FROM npcloot")}
        moving_tables = {r["key"][1] for r in out["npcloot"] if r["action"] in ("copy_H_to_L", "update_L_from_H")}
        for r in out["loottables"]:
            if r["action"] == "copy_H_to_L" and r["key"][0] not in live_tables | moving_tables:
                r.update(cls="d", action="leave", group="used-only-by-shard-specific",
                         reason="a loot table only Heirloom-only creatures use; " + r["reason"])
        # items and spells by what uses them
        attached = {"itempattern": ("itemeffect", "itemaura", "itemfactions", "foodeffect", "itemascended", "heirlooms",
                                    "familiars", "heroes_stock", "itemimported"),
                    "spellpattern": ("spelleffect", "spellarea", "petstats", "spellascended", "spellrecastgroup")}
        for tbl, tag in (("itempattern", "I"), ("spellpattern", "S")):
            users = collections.defaultdict(list)
            pat = re.compile(re.escape(tag) + r"<([^<>]*)>")
            for t, rows in out.items():
                if t == tbl or t in attached[tbl]:
                    continue
                for r in rows:
                    blob = json.dumps({x: r.get(x) for x in ("key", "H", "L", "changed", "Honly", "Lonly")}, default=str)
                    for m in set(pat.findall(blob)):
                        users[m].append((t, r["action"], r["group"]))
            for r in out[tbl]:
                if r["action"] not in ("copy_H_to_L", "copy_L_to_H"):
                    continue
                ident = r["key"][0][len(tag) + 1:-1]
                u = users.get(ident, [])
                moving = [x for x in u if x[1] not in ("leave", "review")]
                if u and not moving:
                    r.update(cls="d", action="leave", group="used-only-by-shard-specific",
                             reason="used only by rows that stay on their shard; " + r["reason"])
                elif moving:
                    r["reason"] = f"needed by {len(moving)} row(s) ({', '.join(sorted(set(x[0] for x in moving)))}); " + r["reason"]
                else:
                    r["reason"] = "used by no content row (GM-made; owner-approved); " + r["reason"]
                    r["group"] = "unreferenced"
            stays = {r["key"][0] for r in out[tbl] if r["action"] == "leave"}
            for t in attached[tbl]:
                for r in out.get(t, []):
                    if r["action"] in ("copy_H_to_L", "copy_L_to_H") and r["key"][0] in stays:
                        r.update(cls="d", action="leave", group="used-only-by-shard-specific",
                                 reason=f"its {tbl[:-7]} stays on its shard; " + r["reason"])
        self.out = out
        return out


# =====================================================================================================================
# writing one direction
# =====================================================================================================================
DIRECTION_ACTIONS = {
    "HL": {"copy_H_to_L": "insert", "update_L_from_H": "update", "quest_L_from_H": "quest", "delete_on_L": "delete"},
    "LH": {"copy_L_to_H": "insert", "update_H_from_L": "update", "quest_H_from_L": "quest", "delete_on_H": "delete"},
}


def op_id(t, action, key):
    return f"{t}|{action}|{json.dumps(key, sort_keys=True, default=str)}"


class Writer:
    """the operations of one direction, src -> dst, with the dst ids every reference gets"""

    def __init__(self, sh, src):
        self.sh, self.src, self.dst = sh, src, OTHER[src]
        self.acts = DIRECTION_ACTIONS[src + self.dst]
        self.sdb, self.ddb = sh.db[src], sh.db[self.dst]
        self.new_ids = {"item": {}, "spell": {}, "quest": {}, "npc": {}, "slot": {}}
        self.unresolved = []
        # every reference translated: {kind: {dst id: what the src row it came from is}}, for the applier to check that
        # the dst row is the same one (shard_refs / apply_sync, 2026-11-27)
        self.refs = {"npc": {}, "slot": {}}

    def ops(self):
        out = []
        for t in ORDER:
            for r in self.sh.out.get(t, []):
                if r["action"] in self.acts:
                    out.append((t, r))
        return out

    # ---- ids on dst --------------------------------------------------------------------------------------------
    def allocate(self, ops):
        d = self.ddb
        def nxt(table, col):
            m = d.execute(f'SELECT COALESCE(MAX("{col}"), 0) FROM "{table}"').fetchone()[0]
            s = d.execute("SELECT seq FROM sqlite_sequence WHERE name = ?", (table,)).fetchone() if has(d, "sqlite_sequence") else None
            return max(m, s[0] if s else 0) + 1
        nid = {"item": nxt("itempattern", "patternid"), "spell": nxt("spellpattern", "spellid"),
               "quest": nxt("quests", "questid"), "npc": nxt("npcs", "id")}
        exist = {"item": set(self.sh.ITEM[self.dst]), "spell": set(self.sh.SPELL[self.dst]),
                 "quest": set(self.sh.QUEST[self.dst]), "npc": set(self.sh.NPCROW[self.dst])}
        quests_new = set()
        for t, r in ops:
            act = self.acts[r["action"]]
            if t in QUEST_TABLES and act in ("insert", "quest"):
                for q in self.sh.QREV[self.src].get(r["key"][0], []):
                    if not self.sh.QREV[self.dst].get(r["key"][0]):
                        quests_new.add(q)
                continue
            if act != "insert":
                continue
            kind = {"itempattern": "item", "spellpattern": "spell", "npcs": "npc"}.get(t)
            if not kind:
                continue
            for sid in (r.get(self.src + "id") or []):
                if kind == "npc":
                    self.new_ids["npc"][sid] = nid["npc"]; nid["npc"] += 1
                    continue
                # a free id stays the same; an item minted from 960000 up takes the shard's own sequence
                keep = sid not in exist[kind] and not (kind == "item" and sid >= SHARED_ITEM_IDS)
                if keep:
                    self.new_ids[kind][sid] = sid; exist[kind].add(sid)
                else:
                    self.new_ids[kind][sid] = nid[kind]; exist[kind].add(nid[kind]); nid[kind] += 1
        for q in sorted(quests_new):
            if q < SHARED_QUEST_IDS and q not in exist["quest"]:
                self.new_ids["quest"][q] = q
            else:
                self.new_ids["quest"][q] = nid["quest"]; nid["quest"] += 1
            exist["quest"].add(self.new_ids["quest"][q])
        self.next_slot = nxt("gmtriggers", "slot") if has(d, "gmtriggers") else 1

    def to_dst(self, kind, v):
        """a src id -> the dst id of the same thing (Unresolved when there is none: the row is left, listed)"""
        out = self._to_dst(kind, v)
        if v not in (None, 0) and kind in self.refs:
            sh, s = self.sh, self.src
            if kind == "npc":
                n, w, x, z = sh.NPCROW[s][v]
                ident = {"src": v, "name": n, "world": w, "x": x, "z": z, "modelid": sh.NPCMODEL[s].get(v)}
            else:
                ident = {"src": v, "name": sh.TRIG[s][v]}
            self.refs[kind][str(out)] = ident
        return out

    def _to_dst(self, kind, v):
        if v in (None, 0):
            return v
        sh, s, d = self.sh, self.src, self.dst
        if kind == "slot":
            name = sh.TRIG[s].get(v)
            if name is None:
                raise Unresolved(f"trigger slot {v} (not on {NAMES[s]})")
            if v in self.new_ids["slot"]:
                return self.new_ids["slot"][v]
            cand = [x for x, n in sh.TRIG[d].items() if n == name]
            if len(cand) == 1:
                return cand[0]
            raise Unresolved(f"trigger slot {v} ({name!r}) is {'several triggers' if cand else 'not'} on {NAMES[d]}")
        if kind == "npc":
            if v in sh.PAIR[s]:
                return sh.PAIR[s][v]
            if v in self.new_ids["npc"]:
                return self.new_ids["npc"][v]
            raise Unresolved(f"npc {v} ({sh.NPCROW[s].get(v, ('?',))[0]})")
        M = {"item": sh.ITEM, "spell": sh.SPELL, "quest": sh.QUEST}[kind]
        ident = M[s].get(v)
        if ident is None:
            raise Unresolved(f"{kind} {v} (not on {NAMES[s]})")
        if M[d].get(v) == ident:
            return v
        cand = sh.IDS[(kind, d)].get(ident)
        if cand:
            return cand[0]
        if v in self.new_ids[kind]:
            return self.new_ids[kind][v]
        # a same-named record copied in this run under another src id
        for sid, nid in self.new_ids[kind].items():
            if M[s].get(sid) == ident:
                return nid
        raise Unresolved(f"{kind} {v} {(sh.ITEMNAME[s].get(v) if kind == 'item' else ident)!r} is not on {NAMES[d]}")

    def text_to_dst(self, kind, v):
        if v is None or v == "":
            return v
        M = {"itext": self.sh.ITEM, "stext": self.sh.SPELL, "qtext": self.sh.QUEST}[kind][self.src]
        k2 = {"itext": "item", "stext": "spell", "qtext": "quest"}[kind]
        if isinstance(v, int):
            return self.to_dst(k2, v) if v in M else v
        return NUM.sub(lambda m: str(self.to_dst(k2, int(m.group(1)))) if int(m.group(1)) in M else m.group(1), str(v))

    def row_to_dst(self, t, raw):
        """a src raw row -> the values dst gets (ids remapped); its own id columns as allocated"""
        spec = SPECS[t]
        trans = spec.get("trans", {})
        out = {}
        for c, v in raw.items():
            if c == "__rowid":
                continue
            kind = trans.get(c)
            if kind in ("item", "spell", "quest", "npc"):
                out[c] = self.to_dst(kind, v)
            elif kind == "jtext":
                if refs.SITE.get((t, c)) != "json":
                    raise AssertionError(f"{t}.{c} is jtext but not a JSON site of shard_refs.py")
                try:
                    out[c] = refs.rewrite(t, c, v, self.to_dst)
                except refs.Untranslatable as e:
                    raise Unresolved(str(e))
            elif kind:
                out[c] = self.text_to_dst(kind, v)
            elif isinstance(v, str) and ROWREF.match(v):
                # a "#<npcs id>" name: the dst row of the same NPC; one the src shard no longer has is unresolved (it
                # was copied verbatim before 2026-11-27, naming whatever row holds that id on dst)
                out[c] = "#" + str(self.to_dst("npc", int(v[1:])))
            else:
                out[c] = v
        if t == "npcs":
            out["id"] = self.new_ids["npc"].get(raw["id"]) or self.sh.PAIR[self.src].get(raw["id"])
        return out


class Applier:
    def __init__(self, sh, w, dbpath, run, allowed=None):
        self.sh, self.w = sh, w
        self.db = sqlite3.connect(dbpath, timeout=60, isolation_level=None)
        self.db.row_factory = sqlite3.Row
        self.run = run
        self.allowed = allowed
        self.done = collections.Counter()
        self.skipped = []
        self.charq = []
        acct = self.db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(self.db, "accountinfo") else None
        self.acct = acct[0] if acct else 0

    # ---- low level, each logged and audited ---------------------------------------------------------------------
    def pkdict(self, t, row, rowid=None):
        pk = pkcols(self.db, t)
        if pk:
            return {c: row[c] for c in pk}
        return {"rowid": rowid}

    def fetch(self, t, pk):
        if "rowid" in pk and len(pk) == 1:
            cur = self.db.execute(f'SELECT rowid AS __rowid, * FROM "{t}" WHERE rowid = ?', (pk["rowid"],))
        else:
            cur = self.db.execute(f'SELECT rowid AS __rowid, * FROM "{t}" WHERE ' + " AND ".join(f'"{c}" = ?' for c in pk),
                                  tuple(pk.values()))
        r = cur.fetchone()
        if r is None:
            return None
        d = dict(r)
        d.pop("__rowid", None)
        return d

    def log(self, t, pk, before, after, detail):
        action = "insert" if before is None else ("delete" if after is None else "update")
        cur = self.db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                              "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                              (now(), self.acct, WHO, VIA, action, t, go_json(pk), go_json(before), go_json(after), detail))
        self.db.execute(f"INSERT INTO {LOG} (run, tbl, pk, before, after, audit) VALUES (?, ?, ?, ?, ?, ?)",
                        (self.run, t, go_json(pk), go_json(before), go_json(after), cur.lastrowid))

    def insert(self, t, values, detail):
        have = set(cols(self.db, t))
        cs = [c for c in values if c in have]
        cur = self.db.execute(f'INSERT INTO "{t}" ({", ".join(chr(34) + c + chr(34) for c in cs)}) VALUES ({", ".join("?" for _ in cs)})',
                              tuple(values[c] for c in cs))
        pk = pkcols(self.db, t)
        pkd = {c: (values.get(c) if values.get(c) is not None else cur.lastrowid) for c in pk} if pk else {"rowid": cur.lastrowid}
        after = self.fetch(t, pkd)
        self.log(t, pkd, None, after, detail)
        return pkd

    def update(self, t, pkd, values, detail):
        before = self.fetch(t, pkd)
        sets = {c: v for c, v in values.items() if c in before and c not in pkd and before[c] != v}
        if not sets:
            return False
        where = "rowid = ?" if "rowid" in pkd else " AND ".join(f'"{c}" = ?' for c in pkd)
        self.db.execute(f'UPDATE "{t}" SET ' + ", ".join(f'"{c}" = ?' for c in sets) + " WHERE " + where,
                        tuple(sets.values()) + tuple(pkd.values()))
        self.log(t, pkd, before, self.fetch(t, pkd), detail)
        return True

    def delete(self, t, pkd, detail):
        before = self.fetch(t, pkd)
        if before is None:
            return False
        if "rowid" in pkd:
            self.db.execute(f'DELETE FROM "{t}" WHERE rowid = ?', (pkd["rowid"],))
        else:
            self.db.execute(f'DELETE FROM "{t}" WHERE ' + " AND ".join(f'"{c}" = ?' for c in pkd), tuple(pkd.values()))
        self.log(t, pkd, before, None, detail)
        return True

    def same_as_read(self, t, raw):
        """the dst row as the comparison read it: still so?"""
        pk = pkcols(self.db, t)
        pkd = {c: raw[c] for c in pk} if pk else {"rowid": raw["__rowid"]}
        cur = self.fetch(t, pkd)
        want = {c: v for c, v in raw.items() if c != "__rowid"}
        return pkd, (cur is not None and cur == want)

    # ---- the operations ------------------------------------------------------------------------------------------
    def detail(self, r):
        return f"shard sync 2026-11-15 (from {NAMES[self.w.src]}): {r['reason'][:160]}; {TOOL}"

    def skip(self, t, r, why):
        self.skipped.append({"table": t, "action": r["action"], "key": r["key"], "why": why})

    def run_ops(self, ops):
        quests = collections.OrderedDict()
        for t, r in ops:
            oid = op_id(t, r["action"], r["key"])
            if self.allowed is not None and oid not in self.allowed:
                self.skip(t, r, "not in the --plan file")
                continue
            kind = self.w.acts[r["action"]]
            if kind == "quest" or (t in QUEST_TABLES and kind in ("insert", "delete")):
                quests.setdefault(r["key"][0], []).append((t, r))
                continue
            try:
                getattr(self, "op_" + kind)(t, r)
            except Unresolved as e:
                self.skip(t, r, f"unresolved reference: {e}")
        for qkey, rs in quests.items():
            try:
                self.op_quest(qkey, rs)
            except Unresolved as e:
                for t, r in rs:
                    self.skip(t, r, f"unresolved reference: {e}")

    def op_insert(self, t, r):
        src_raws = r.get(self.w.src + "raw") or []
        det = self.detail(r)
        vals_list = [self.w.row_to_dst(t, raw) for raw in src_raws]  # all resolve before any write
        for raw, vals in zip(src_raws, vals_list):
            spec = SPECS[t]
            for c in ("id", "itemid", "defaultCharacterID", "def_spell_id"):
                if c in (spec.get("ignore") or []) and c in vals:
                    vals.pop(c)
            if t == "gmtriggers":
                vals["slot"] = self.w.next_slot; self.w.next_slot += 1
                self.w.new_ids["slot"][raw["slot"]] = vals["slot"]
            if t == "itempattern":
                vals["patternid"] = self.w.new_ids["item"][raw["patternid"]]
            if t == "spellpattern":
                vals["spellid"] = self.w.new_ids["spell"][raw["spellid"]]
            pk = pkcols(self.db, t)
            if pk and all(c in vals for c in pk) and self.fetch(t, {c: vals[c] for c in pk}) is not None:
                self.skip(t, r, f"{pk} {[vals[c] for c in pk]} is taken on {NAMES[self.w.dst]} now")
                continue
            self.insert(t, vals, det)
            self.done[(t, "insert")] += 1

    def op_update(self, t, r):
        det = self.detail(r)
        if r.get("multi"):
            sr = r["Honly_raw"] if self.w.src == "H" else r["Lonly_raw"]
            dr = r["Lonly_raw"] if self.w.src == "H" else r["Honly_raw"]
        else:
            sr, dr = r[self.w.src + "raw"], r[self.w.dst + "raw"]
        vals = [self.w.row_to_dst(t, x) for x in sr]
        ign = set(SPECS[t].get("ignore") or [])
        for i in range(max(len(sr), len(dr))):
            if i < len(sr) and i < len(dr):
                pkd, ok = self.same_as_read(t, dr[i])
                if not ok:
                    self.skip(t, r, f"the {NAMES[self.w.dst]} row changed since it was read")
                    continue
                v = {c: x for c, x in vals[i].items() if c not in ign and c != "id"}
                if self.update(t, pkd, v, det):
                    self.done[(t, "update")] += 1
            elif i < len(sr):
                v = {c: x for c, x in vals[i].items() if not (c in ign and c in ("id", "itemid", "defaultCharacterID", "def_spell_id", "slot"))}
                if t == "gmtriggers":
                    v["slot"] = self.w.next_slot; self.w.next_slot += 1
                    self.w.new_ids["slot"][sr[i]["slot"]] = v["slot"]
                if t in ("npcs", "itempattern", "spellpattern", "quests"):
                    continue  # a record of its own: never made or dropped from an update (same-named duplicates)
                self.insert(t, v, det)
                self.done[(t, "insert")] += 1
            else:
                if t in ("npcs", "itempattern", "spellpattern", "quests"):
                    continue
                pkd, ok = self.same_as_read(t, dr[i])
                if ok and self.delete(t, pkd, det):
                    self.done[(t, "delete")] += 1

    def op_delete(self, t, r):
        det = self.detail(r)
        for raw in r.get(self.w.dst + "raw") or []:
            pkd, ok = self.same_as_read(t, raw)
            if not ok:
                if self.fetch(t, pkd) is None and (t, "delete") in self.done:
                    continue  # gone already: its NPC's delete took it
                self.skip(t, r, f"the {NAMES[self.w.dst]} row changed since it was read")
                continue
            if t == "npcs":  # its extras go with it
                for et, ec in NPC_EXTRAS:
                    if not has(self.db, et):
                        continue
                    for er in self.db.execute(f'SELECT rowid AS __rowid, * FROM "{et}" WHERE "{ec}" = ?', (raw["id"],)).fetchall():
                        epk = self.pkdict(et, er, er["__rowid"])
                        if self.delete(et, epk, det):
                            self.done[(et, "delete")] += 1
            if self.delete(t, pkd, det):
                self.done[(t, "delete")] += 1

    def op_quest(self, qkey, rs):
        """a quest copied or replaced whole: its row, steps and step targets; characters' progress mapped"""
        sh, w = self.sh, self.w
        sq = sh.QREV[w.src].get(qkey, [])
        dq = sh.QREV[w.dst].get(qkey, [])
        det = self.detail(rs[0][1])
        acts = {w.acts[r["action"]] for _, r in rs}
        if acts == {"delete"}:
            for q in dq:
                self.drop_quest(q, det)
            return
        if len(sq) != 1 or len(dq) > 1:
            for t, r in rs:
                self.skip(t, r, f"quest {qkey} is {len(sq)} quests on {NAMES[w.src]}, {len(dq)} on {NAMES[w.dst]}")
            return
        s = sq[0]
        srow = dict(w.sdb.execute("SELECT * FROM quests WHERE questid = ?", (s,)).fetchone())
        ssteps = [dict(x) for x in w.sdb.execute("SELECT * FROM queststeps WHERE questid = ? ORDER BY step", (s,))]
        stargs = [dict(x) for x in w.sdb.execute("SELECT * FROM queststeptargets WHERE questid = ?", (s,))]
        if dq:
            d = dq[0]
        else:
            d = w.new_ids["quest"][s]
        # every reference resolved before a write
        qv = w.row_to_dst("quests", srow)
        stv = [w.row_to_dst("queststeps", x) for x in ssteps]
        tgv = [w.row_to_dst("queststeptargets", x) for x in stargs]
        for x in [qv] + stv + tgv:
            x["questid"] = d
        if dq:
            old_steps = [dict(x) for x in self.db.execute("SELECT * FROM queststeps WHERE questid = ? ORDER BY step", (d,))]
            cur = self.fetch("quests", {"questid": d})
            if self.update("quests", {"questid": d}, {c: v for c, v in qv.items() if c != "questid"}, det):
                self.done[("quests", "update")] += 1
            new_sig = [(x["step"], x["kind"], (x["target"] or "").lower(), x["items"]) for x in stv]
            old_sig = [(x["step"], x["kind"], (x["target"] or "").lower(), x["items"]) for x in old_steps]
            # steps: change only what differs (same step number: update; gone: delete; new: insert)
            oldby = {x["step"]: x for x in old_steps}
            newby = {x["step"]: x for x in stv}
            for st in sorted(set(oldby) - set(newby)):
                self.delete("queststeps", {"questid": d, "step": st}, det); self.done[("queststeps", "delete")] += 1
            for st in sorted(newby):
                if st in oldby:
                    if self.update("queststeps", {"questid": d, "step": st}, {c: v for c, v in newby[st].items() if c not in ("questid", "step")}, det):
                        self.done[("queststeps", "update")] += 1
                else:
                    self.insert("queststeps", newby[st], det); self.done[("queststeps", "insert")] += 1
            oldt = {(x["step"], x["target"]) for x in self.db.execute("SELECT step, target FROM queststeptargets WHERE questid = ?", (d,))}
            newt = {(x["step"], x["target"]) for x in tgv}
            for st, tg in sorted(oldt - newt):
                self.delete("queststeptargets", {"questid": d, "step": st, "target": tg}, det); self.done[("queststeptargets", "delete")] += 1
            for st, tg in sorted(newt - oldt):
                self.insert("queststeptargets", {"questid": d, "step": st, "target": tg}, det); self.done[("queststeptargets", "insert")] += 1
            if [x[1:] for x in old_sig] != [x[1:] for x in new_sig]:
                self.map_progress(d, old_steps, stv, det)
        else:
            if self.fetch("quests", {"questid": d}) is not None:
                for t, r in rs:
                    self.skip(t, r, f"questid {d} is taken on {NAMES[w.dst]} now")
                return
            self.insert("quests", qv, det); self.done[("quests", "insert")] += 1
            for x in stv:
                self.insert("queststeps", x, det); self.done[("queststeps", "insert")] += 1
            for x in tgv:
                self.insert("queststeptargets", x, det); self.done[("queststeptargets", "insert")] += 1

    def drop_quest(self, q, det):
        for t, keyc in (("queststeptargets", ("questid", "step", "target")), ("queststeps", ("questid", "step"))):
            for x in self.db.execute(f"SELECT * FROM {t} WHERE questid = ?", (q,)).fetchall():
                self.delete(t, {c: x[c] for c in keyc}, det); self.done[(t, "delete")] += 1
        if self.delete("quests", {"questid": q}, det):
            self.done[("quests", "delete")] += 1

    def map_progress(self, q, old_steps, new_steps, det):
        """characters part-way through a quest whose steps changed: each onto the step that is the same task"""
        rows = self.db.execute("SELECT * FROM charquests WHERE questid = ? AND done = 0", (q,)).fetchall()
        if not rows:
            return
        osig = [(x["kind"], (x["target"] or "").lower()) for x in old_steps]
        nsig = [(x["kind"], (x["target"] or "").lower()) for x in new_steps]
        onum = [x["step"] for x in old_steps]
        nnum = [x["step"] for x in new_steps]
        m = {}
        for a, b, size in difflib.SequenceMatcher(None, osig, nsig, autojunk=False).get_matching_blocks():
            for i in range(size):
                m[a + i] = b + i
        for cq in rows:
            cq = dict(cq)
            st = cq["step"]
            how = ""
            if st in onum:
                i = onum.index(st)
                if i in m:
                    ns, how = nnum[m[i]], "the same task"
                else:
                    prev = [m[j] for j in range(i) if j in m]
                    ni = (prev[-1] + 1) if prev else 0
                    ni = min(ni, len(nnum) - 1)
                    ns, how = nnum[ni], "no same task: the step after the last matched one"
            elif onum and st > max(onum):
                ns, how = (max(nnum) if nnum else st) + (st - max(onum)), "past the last step"
            else:
                ns, how = st, "unknown step left"
            if ns != st:
                vals = {"step": ns}
                if how != "the same task":
                    vals["count"] = 0
                self.update("charquests", {"serverid": cq["serverid"], "questid": q}, vals, det + f"; progress step {st} -> {ns} ({how})")
                self.done[("charquests", "update")] += 1
            self.charq.append({"serverid": cq["serverid"], "questid": q, "from": st, "to": ns, "how": how})


LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    run    TEXT NOT NULL,
    tbl    TEXT NOT NULL,
    pk     TEXT NOT NULL,
    before TEXT,
    after  TEXT,
    audit  INTEGER NOT NULL DEFAULT 0,
    undone INTEGER NOT NULL DEFAULT 0
)"""


def strip_basis(detail):
    return re.sub(r"^\[basis:[^\]\r\n]*\]\s*", "", detail or "", count=1, flags=re.I)


def role_of(path):
    import vanilla_tickets as tickets
    db = readonly(path)
    try:
        return {"heirloom": "H", "live": "L"}[tickets.bound_shard(db)]
    finally:
        db.close()


def summarize(sh, src):
    acts = DIRECTION_ACTIONS[src + OTHER[src]]
    per = collections.OrderedDict()
    for t in ORDER:
        c = collections.Counter(acts[r["action"]] for r in sh.out.get(t, []) if r["action"] in acts)
        if c:
            per[t] = dict(c)
    return per


def undo(path, run, shard, config=None):
    import vanilla_tickets as tickets
    tickets.require(tickets.nonblank(run) and shard in tickets.TARGETS, "--undo requires --run and --shard")
    db = sqlite3.connect(Path(path).resolve().as_uri() + "?mode=rw", uri=True, timeout=60, isolation_level=None)
    db.row_factory = sqlite3.Row
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone()
    acct = acct[0] if acct else 0
    left = []
    n = collections.Counter()
    db.execute("BEGIN IMMEDIATE")
    try:
        tickets.require(tickets.bound_shard(db, config) == shard, "destination database is the other shard")
        tickets.require(has(db, LOG) and db.execute(f"SELECT 1 FROM {LOG} WHERE run=? LIMIT 1", (run,)).fetchone(),
                        "unknown run on this destination")
        rows = db.execute(f"SELECT * FROM {LOG} WHERE run=? AND undone=0 ORDER BY seq DESC", (run,)).fetchall()
        for r in rows:
            t, pk = r["tbl"], json.loads(r["pk"])
            before = json.loads(r["before"]) if r["before"] else None
            after = json.loads(r["after"]) if r["after"] else None
            where = " AND ".join(f'"{c}" = ?' for c in pk) if "rowid" not in pk else "rowid = ?"
            cur = db.execute(f'SELECT * FROM "{t}" WHERE {where}', tuple(pk.values())).fetchone()
            cur = json.loads(go_json(dict(cur))) if cur else None
            if after is not None and cur != after:
                left.append((r["seq"], t, pk, "changed since the sync")); continue
            if after is None and cur is not None:
                left.append((r["seq"], t, pk, "a row is there again")); continue
            if before is None:
                db.execute(f'DELETE FROM "{t}" WHERE {where}', tuple(pk.values())); act = "delete"
            elif after is None:
                cs = list(before)
                db.execute(f'INSERT INTO "{t}" ({", ".join(chr(34) + c + chr(34) for c in cs)}) VALUES ({", ".join("?" for _ in cs)})',
                           tuple(before[c] for c in cs)); act = "insert"
            else:
                sets = [c for c in before if before[c] != after.get(c)]
                db.execute(f'UPDATE "{t}" SET ' + ", ".join(f'"{c}" = ?' for c in sets) + f" WHERE {where}",
                           tuple(before[c] for c in sets) + tuple(pk.values())); act = "update"
            db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                       "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                       (now(), acct, WHO, VIA, act, t, go_json(pk), go_json(cur), go_json(before),
                        f"[basis:test] undo {r['run']} log {r['seq']}"))
            db.execute(f"UPDATE {LOG} SET undone = 1 WHERE seq = ?", (r["seq"],))
            n[(t, act)] += 1
        db.execute("COMMIT")
    except Exception:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()
    print(f"undone: {sum(n.values())} rows")
    for (t, a), c in sorted(n.items()):
        print(f"  {t:20} {a:7} {c}")
    if left:
        print(f"LEFT (changed since; not put back): {len(left)}")
        for x in left[:50]:
            print("  ", x)


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--from", dest="src", help="source for a read-only comparison")
    ap.add_argument("--to", dest="dst", required=True, help="the database to bring up to date")
    ap.add_argument("--plan", help="write the proposed operations for review")
    ap.add_argument("--review", help="write the rows left for a person (no own edit on either shard) here")
    ap.add_argument("--shard", choices=["heirloom", "live"], help="explicit destination for rollback")
    ap.add_argument("--run", help="exact logged November 15 run")
    ap.add_argument("--config", help="destination server config for rollback")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help=argparse.SUPPRESS)
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    if a.apply:
        ap.error("bulk sync retired; use a reviewed ticket receipt with live_sync_20261122.py")
    if a.undo:
        if not (a.run and a.shard):
            ap.error("--undo requires --run and --shard")
        undo(a.dst, a.run, a.shard, a.config)
        return
    if not a.src or a.run or a.shard or a.config:
        ap.error("comparison requires --from and --to; --run, --shard and --config belong with --undo")
    rs, rd = role_of(a.src), role_of(a.dst)
    if rs == rd:
        sys.exit(f"REFUSED: both databases are bound to {NAMES[rs]}")
    hpath, lpath = (a.src, a.dst) if rs == "H" else (a.dst, a.src)
    print(f"{NAMES[rs]} {a.src} -> {NAMES[rd]} {a.dst}", flush=True)
    sh = Shards(hpath, lpath)
    sh.compare()
    sh.classify_all()
    w = Writer(sh, rs)
    ops = w.ops()
    w.allocate(ops)
    per = summarize(sh, rs)
    print(f"operations {NAMES[rs]} -> {NAMES[rd]}: {len(ops)} rows")
    for t, c in per.items():
        print(f"  {t:20} " + "  ".join(f"{k} {v}" for k, v in sorted(c.items())))
    review = [{"table": t, "key": r["key"], "changed": r.get("changed"), "Hid": r.get("Hid"), "Lid": r.get("Lid")}
              for t in ORDER for r in sh.out.get(t, []) if r["action"] == "review"]
    print(f"left for a person (review, not synced): {len(review)}")
    if a.review:
        json.dump(review, open(a.review, "w"), indent=1, default=str)
    if a.plan:
        plan = {"from": a.src, "to": a.dst, "direction": f"{NAMES[rs]} -> {NAMES[rd]}", "counts": per,
                "ops": [{"id": op_id(t, r["action"], r["key"]), "table": t, "action": r["action"], "group": r["group"],
                         "key": r["key"], "reason": r["reason"], "changed": r.get("changed")} for t, r in ops]}
        json.dump(plan, open(a.plan, "w"), indent=0, default=str)
        print("plan written:", a.plan)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError, sqlite3.Error) as exc:
        sys.exit(f"REFUSED: {exc}")
