#!/usr/bin/env python3
"""Blackvein Deep: the evil dwarves' own start, for dwarf shadow knights and necromancers (the owner, 2026-11-21: "add a
dwarf evil starting area near moradhim, a new cave complex for SKs and Necros heirloom only"). Heirloom alone: a
database with no Homelands (world 10, the Live shard's) is refused.

The deep is the cave under the Hatebone orcs' hills east of Moradhim, Tunaria zone 42 (Thedruk), as world 10 holds it
whole (tools/worlds/homelands.json composes the zone for the orcs' home, whose camp is on the surface 1,000 east): its
tunnels at (18500-19080, 8890-9310), floors -103 to -23 under the rock, empty there. No world is built or baked: the
world, its ground and its clients' copies are the Homelands'. Everything is data, in tools/evil_dwarf_start_20261121.json
(each spot as the baked ground answers it, cmd/campground).

Steps, all written in one transaction:

starts     defaultcharacter: a row for each class (3, 11) in the deep's hall, where a new one starts and is bound
           (database.CreateNew), and one at the camp by Moradhim's coach (world 0), where a login whose client lacks
           world 10 stands (a PS2: database.MainlandPlace, which takes a class's disc row; this branch's newchar.go).
           Stats: the dwarf warrior's row plus the eastern humans' class over their warrior (as every row is 60 plus
           the race's and the class's bonuses).
factions   factionstart (4, class, -1): the erudite's of the class (an evil class of a good race that starts at home),
           but the erudites' own (Highbourne's, Qeynos's tolerance) the gnome necromancer's, and Moradhim's own 0
           (neutral: Moradhim's guards never attack them, nor welcome them). A row only where that differs from what a
           dwarf of the class starts with now.
existing   charfactions of the dwarf shadow knights and necromancers there are, once: a row a character holds for one
           of those factions moved by the start's change (its earned part kept), within -2000..2000. A faction it holds
           no row for reads the new start by itself.
people     npcs: the deep's elder, the two guildmasters, an outfitter and a grocer (Moradhim's wares), a spiritmaster,
           the Spell Archivist (where the database has one), the tunnelwarden, a coachman (Blackvein Deep's stable,
           whose coach runs to Moradhim and Moradhim's back, for a client that holds world 10: entity/coachstables.go;
           of no faction, so the coach-faction gate never refuses him), a sentry; its creatures for levels 1-14
           (grubs and spiders that attack no one, Moradhim's trackers and the Brellsworn who attack on sight: npcaggro);
           at the camp a deepwarden and a spiritmaster. Their words (npcdialog by name).
quests     the deep's quests (104900-104912): the storyline "The Oathbreaker's Road" (The Broken Oath L1, Bound in the
           Deep L2, Trackers from Moradhim L5, The Brellsworn L8, What Was Sworn L13) and each class's guildmaster's
           L3 and L4; the level 1-5 ones carry classquest events, so they stand for Freeport's shadow knight and
           necromancer chains' level 1-5 and Malethai Crimsonhand and Corious Slaerin go on from level 7
           (handlers/questevents.go). Four trips (repeatable, a teleport each): the deep up to the camp, the deep or
           the camp to Freeport's guilds (the smugglers' road), the camp down into the deep (a PS2 is refused it: the
           engine sends no one into a world its client lacks). Race 4 added to Freeport's two chains, group-6-evil
           (levels 30-49) and the necromancers' evil caster epic; the off-race Moradhim starters of a dwarf shadow
           knight and necromancer (103569-103571, 103584-103586) disabled; the Freeport necromancers' level 1 and
           7 gear made wearable by a dwarf.

Every row written is a gm_audit line (via "tool") and a row of evil_dwarf_start_20261121_log; --undo puts each row back
as it was (one a GM or the game has changed since is left, and said), takes the characters' marks of the deep's quests
with it, and drops the log. A report by default; --apply writes, once (a second finds the log and writes nothing);
--apply and --undo copy the database first (<db>.bak-<when>-pre-evildwarf) unless --no-backup.

WHEN IT TAKES EFFECT: factionstart, npcs, npcaggro and quests are read at the server's start, a character's
charfactions at its login (and written over from memory at its logout): stop the server for --apply and --undo, a
keep restart's before.sh. The start rows need the server of this branch (database.placeOf): an older one may start a
new dwarf shadow knight at the camp instead of the deep (the rows' order), and sends a PS2's to Freeport.

    python3 -E -s tools/evil_dwarf_start_20261121.py --db eqoa.db                  report
    python3 -E -s tools/evil_dwarf_start_20261121.py --db eqoa.db --apply          write it
    python3 -E -s tools/evil_dwarf_start_20261121.py --db eqoa.db --undo           take it back
"""
import argparse
import datetime
import json
import math
import os
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import monk_start as ms  # noqa: E402  (the server's faction start rules: combo_of, resolve)

DATA = os.path.join(HERE, "evil_dwarf_start_20261121.json")
LOG = "evil_dwarf_start_20261121_log"
DETAIL = ("the owner, 2026-11-21: \"add a dwarf evil starting area near moradhim, a new cave complex for SKs and Necros "
          "heirloom only\": tools/evil_dwarf_start_20261121.py")
WHO, VIA = "claude", "tool"
SOURCE = "ours: tools/evil_dwarf_start_20261121.py"
DIALOG_SOURCE = "evil-dwarf-start"
TALKER, FOE, MERCHANT = 128, 0, 0x01
DWARF, WARRIOR = 4, 0
STATS = ms.STATS
KOS_BELOW = ms.KOS_BELOW
COACH_FACTION_MIN = -99  # entity.CoachFactionMin: a coachman of a faction serves no one below it
KEYS = {"defaultcharacter": ("defaultCharacterID",), "factionstart": ("id",), "charfactions": ("serverid", "faction"),
        "npcs": ("id",), "npcitem": ("itemid",), "npcaggro": ("npc_name",), "npcdialog": ("id",),
        "quests": ("questid",), "queststeps": ("questid", "step"), "itempattern": ("patternid",)}
# npcs rows the game writes to (a kill's time): a row is ours while these are as written
NPC_SAME = ("npc_name", "world", "x_coord", "y_coord", "z_coord")
LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    step   TEXT NOT NULL,
    tbl    TEXT NOT NULL,
    ref    TEXT NOT NULL,
    before TEXT,
    after  TEXT,
    audit  INTEGER,
    at     TEXT NOT NULL
)"""
AUDIT_DDL = """CREATE TABLE IF NOT EXISTS gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER,
    username TEXT, via TEXT, ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT)"""
STEPS = ("starts", "factions", "existing", "people", "quests")

go_json = ms.go_json


def rows(db, sql, args=()):
    return ms.rows(db, sql, args)


def row_of(db, tbl, ref):
    r = rows(db, f"SELECT * FROM {tbl} WHERE " + " AND ".join(f"{c} = ?" for c in KEYS[tbl]),
             tuple(ref[c] for c in KEYS[tbl]))
    return r[0] if r else None


def has_table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def heading_to(x, z, tx, tz):
    """CLIENT's heading from (x, z) toward (tx, tz), radians (0 facing +z)."""
    return math.atan2(tx - x, tz - z) % (2 * math.pi)


def byte_heading(h):
    return int(round(h / (2 * math.pi) * 256)) % 256


def races_with(races, race):
    """A races column with race in it, in order ("0, 4, 10"); None when it is there already or the column is empty
    (empty is every race)."""
    have = [int(x) for x in str(races or "").replace(" ", "").split(",") if x != ""]
    if not have or race in have:
        return None
    return ", ".join(str(r) for r in sorted(have + [race]))


class Plan:
    def __init__(self, db, data):
        self.db, self.d = db, data
        self.changes = []  # (step, tbl, ref, before, after, note)
        self.lines = {s: [] for s in STEPS}
        self.problems = []

    def add(self, step, tbl, ref, before, after, note=""):
        self.changes.append((step, tbl, ref, before, after, note))

    # ---- starts -------------------------------------------------------------------------------------------------
    def starts(self):
        d, db = self.d, self.db
        war = rows(db, "SELECT * FROM defaultcharacter WHERE race = ? AND tclass = ? ORDER BY defaultCharacterID", (DWARF, WARRIOR))
        hwar = rows(db, "SELECT * FROM defaultcharacter WHERE race = 0 AND humType = 1 AND tclass = ?", (WARRIOR,))
        if not war or not hwar:
            self.problems.append("no dwarf warrior or eastern human warrior start row to take the stats from")
            return
        war, hwar = war[0], hwar[0]
        places = [("deep", d["start"]), ("camp", d["camp"])]
        for cls in d["factions"]["classes"]:
            hcls = rows(db, "SELECT * FROM defaultcharacter WHERE race = 0 AND humType = 1 AND tclass = ?", (cls,))
            if not hcls:
                self.problems.append(f"no eastern human start row of class {cls}")
                continue
            for what, p in places:
                had = rows(db, "SELECT defaultCharacterID FROM defaultcharacter WHERE race = ? AND tclass = ? AND COALESCE(world, 0) = ?",
                           (DWARF, cls, p["world"]))
                if had:
                    self.problems.append(f"a dwarf of class {cls} has a start row in world {p['world']} already "
                                         f"(defaultcharacter {had[0]['defaultCharacterID']})")
                    continue
                row = {k: v for k, v in war.items() if k != "defaultCharacterID"}
                facing = p["facing"] if "facing" in p else heading_to(p["x"], p["z"], *p["toward"])
                row.update(tclass=cls, humType=0, x=p["x"], y=p["y"], z=p["z"], facing=round(facing, 6), world=p["world"])
                for s in STATS:
                    row[s] = war[s] + hcls[0][s] - hwar[s]
                self.add("starts", "defaultcharacter", None, None, row, f"class {cls}: {what}")
                self.lines["starts"].append(f"  class {cls:2} {what}: world {p['world']} ({p['x']}, {p['y']}, {p['z']}) facing {row['facing']}; "
                                            + " ".join(str(row[s]) for s in STATS))

    # ---- factions -----------------------------------------------------------------------------------------------
    def start_rows(self):
        out = {}
        for r in self.db.execute("SELECT race, class, humtype, faction, standing FROM factionstart"):
            out.setdefault(r[3], []).append((r[0], r[1], r[2], r[4]))
        return out

    def targets(self):
        """{class: {faction: (now, target)}} for every faction whose target differs from a dwarf of the class's
        start now."""
        f = self.d["factions"]
        st = self.start_rows()
        base = dict(self.db.execute("SELECT name, standing FROM factions"))
        out = {}
        for cls in f["classes"]:
            mine = ms.combo_of(DWARF, cls, 0)
            like = ms.combo_of(f["like_race"], cls, 0)
            other = ms.combo_of(*f["others_like"][:2], 0)
            per = {}
            for name, b in base.items():
                now = ms.resolve(st.get(name, []), b, mine)
                if name in f["home"]:
                    want = f["home"][name]
                elif name in f["others"]:
                    want = ms.resolve(st.get(name, []), b, other)
                else:
                    want = ms.resolve(st.get(name, []), b, like)
                if want != now:
                    per[name] = (now, want)
            out[cls] = per
        return out

    def factions(self):
        for cls, per in self.targets().items():
            up = sum(1 for n, w in per.values() if w > n)
            self.lines["factions"].append(f"  class {cls}: {len(per)} rows (raised {up}, lowered {len(per) - up})")
            for name, (now, want) in sorted(per.items()):
                had = rows(self.db, "SELECT * FROM factionstart WHERE race = ? AND class = ? AND humtype = -1 AND faction = ?",
                           (DWARF, cls, name))
                if had:
                    self.add("factions", "factionstart", {"id": had[0]["id"]}, had[0], dict(had[0], standing=want, source="evil-dwarf-start"))
                else:
                    self.add("factions", "factionstart", None, None,
                             {"race": DWARF, "class": cls, "humtype": -1, "faction": name, "standing": want,
                              "note": "the owner 2026-11-21: dwarf shadow knights and necromancers are evil, as the erudite's "
                                      "evil classes start (tools/evil_dwarf_start_20261121.py)", "source": "evil-dwarf-start"})
                self.lines["factions"].append(f"    {name}: {now} -> {want}")

    # ---- existing -----------------------------------------------------------------------------------------------
    def existing(self):
        t = self.targets()
        chars = rows(self.db, "SELECT serverid, tclass, level FROM characters WHERE race = ? AND tclass IN (%s) ORDER BY serverid"
                     % ",".join("?" * len(t)), (DWARF, *t))
        self.lines["existing"].append(f"  dwarf shadow knights and necromancers: {len(chars)}")
        for c in chars:
            per = t[c["tclass"]]
            moved = []
            for r in rows(self.db, "SELECT * FROM charfactions WHERE serverid = ?", (c["serverid"],)):
                if r["faction"] not in per:
                    continue
                now, want = per[r["faction"]]
                v = max(-2000, min(2000, r["standing"] + want - now))
                if v != r["standing"]:
                    ref = {"serverid": c["serverid"], "faction": r["faction"]}
                    self.add("existing", "charfactions", ref, r, dict(r, standing=v))
                    moved.append(f"{r['faction']} {r['standing']} -> {v}")
            self.lines["existing"].append(f"    character {c['serverid']} (class {c['tclass']}, level {c['level']}): "
                                          + ("; ".join(moved) if moved else "no row of the changed factions (it reads the new starts)"))

    # ---- people -------------------------------------------------------------------------------------------------
    def template(self, spec):
        like = spec["like"]
        if isinstance(like, int):
            r = rows(self.db, "SELECT * FROM npcs WHERE id = ?", (like,))
        else:
            r = rows(self.db, "SELECT * FROM npcs WHERE npc_name = ? ORDER BY id LIMIT 1", (like,))
        return r[0] if r else None

    def people(self):
        d, db = self.d, self.db
        names = {p["name"] for p in d["people"]}
        for name in sorted(names):
            n = db.execute("SELECT count(*) FROM npcs WHERE lower(npc_name) = lower(?)", (name,)).fetchone()[0]
            if n and name != "Spell Archivist":
                self.problems.append(f"{n} NPCs are named {name!r} already: the deep's people have names of their own "
                                     "(a quest's kill, npcaggro and the words go by name)")
        for p in d["people"]:
            t = self.template(p)
            if t is None:
                if p.get("optional"):
                    self.lines["people"].append(f"  left out: {p['name']} (no npcs row like {p['like']!r})")
                    continue
                self.problems.append(f"{p['name']}: no npcs row like {p['like']!r}")
                continue
            world = p.get("world", d["cave_world"])
            x, y, z = p["at"]
            face = byte_heading(heading_to(x, z, *p["toward"])) if "toward" in p else int(p.get("facing", 0))
            row = {k: v for k, v in t.items() if k != "id"}
            row.update(npc_name=p["name"], x_coord=x, y_coord=y, z_coord=z, facing=face, world=world,
                       npc_level=int(p["level"]), npc_type=TALKER if p["role"] == "talker" else FOE, dt_kill=None)
            stock = []
            if p.get("stock"):
                src = rows(db, "SELECT id FROM npcs WHERE npc_name = ? AND world != ? ORDER BY id LIMIT 1", (p["stock"], world))
                if src:
                    cur = db.execute("SELECT * FROM npcitem WHERE npcid = ? ORDER BY listnumber, itemid", (src[0]["id"],))
                    cols = [c[0] for c in cur.description]
                    stock = [dict(zip(cols, r)) for r in cur.fetchall()]
                if not stock:
                    if p.get("optional"):
                        self.lines["people"].append(f"  left out: {p['name']} (no wares of {p['stock']!r})")
                        continue
                    self.problems.append(f"{p['name']}: {p['stock']!r} sells nothing")
                    continue
                row["npc_type"] |= MERCHANT
            self.add("people", "npcs", None, None, {"row": row, "stock": stock}, p["name"])
            self.lines["people"].append(f"  {p['name']}: world {world} ({x}, {y}, {z}) level {row['npc_level']} type {row['npc_type']}"
                                        + (f", {len(stock)} wares" if stock else ""))
        for key, aggressive in (("peaceful", 0), ("hostile", 1)):
            for name in d[key]:
                if name not in names:
                    self.problems.append(f"{key} names {name!r}, who is none of the deep's people")
                old = rows(db, "SELECT * FROM npcaggro WHERE npc_name = ?", (name,))
                if old:
                    self.lines["people"].append(f"  npcaggro {name}: has a row already (aggressive {old[0]['aggressive']}): kept")
                    continue
                self.add("people", "npcaggro", {"npc_name": name}, None,
                         {"npc_name": name, "aggressive": aggressive, "radius": 0,
                          "note": "Blackvein Deep's " + ("critter" if not aggressive else "foe") + ": tools/evil_dwarf_start_20261121.py",
                          "source": DIALOG_SOURCE})
                self.lines["people"].append(f"  npcaggro {name}: aggressive {aggressive}")
        for name, lines in d["lines"].items():
            if name not in names:
                self.problems.append(f"lines for {name!r}, who is none of the deep's people")
            for line in lines:
                self.add("people", "npcdialog", None, None, {"name": name, "line": line, "weight": 1, "source": DIALOG_SOURCE})
        self.lines["people"].append(f"  words: {sum(len(v) for v in d['lines'].values())} lines")

    # ---- quests -------------------------------------------------------------------------------------------------
    def place(self, which):
        d = self.d
        p = {"camp": d["camp"], "freeport": d["freeport"], "start": d["start"]}[which]
        facing = p["facing"] if "facing" in p else heading_to(p["x"], p["z"], *p["toward"])
        ev = {"on": "done", "do": "teleport", "x": p["x"], "y": p["y"], "z": p["z"], "facing": round(facing, 4)}
        if p["world"]:
            ev["world"] = p["world"]
        return ev

    def quest_row(self, q, classes="3, 11", repeatable=0, events=""):
        return {"questid": q["id"], "name": q["name"], "enabled": 1, "giver": q["giver"], "minlevel": q.get("minlevel", 1),
                "maxlevel": 60, "classes": q.get("classes", classes), "races": str(DWARF), "requires": q.get("requires", ""),
                "repeatable": repeatable, "offer": q["offer"], "accept": q["accept"], "decline": "Not now.",
                "declined": q["declined"], "started": q["started"], "source": f"{SOURCE} ({q['key']})", "notes": "",
                "events": events, "factionreq": "", "excludes": "", "asks": "", "daygroup": ""}

    def step_row(self, qid, n, s):
        return {"questid": qid, "step": n, "kind": s["kind"], "target": s.get("target", ""), "npcid": 0, "items": "",
                "tunar": 0, "count": s.get("count", 1), "world": 0, "x": 0.0, "y": 0.0, "z": 0.0, "radius": 15.0,
                "journal": s["journal"], "answer": "", "says": s.get("says", ""), "waiting": "", "notyet": "",
                "reminder": "", "xp": s.get("xp", 0), "rewardtunar": s.get("rewardtunar", 0), "rewarditems": "",
                "factions": s.get("factions", ""), "spells": "", "events": s.get("events", ""), "chance": 0.0,
                "byclass": json.dumps(s["byclass"], sort_keys=True, separators=(",", ":")) if s.get("byclass") else "",
                "choices": "", "height": 0.0, "anyorder": 0}

    def quests(self):
        d, db = self.d, self.db
        people = {p["name"] for p in d["people"]}
        for q in d["quests"] + d["trips"]:
            if row_of(db, "quests", {"questid": q["id"]}) is not None:
                self.problems.append(f"quest {q['id']} is there already")
            if q["giver"] not in people:
                self.problems.append(f"quest {q['id']}: its giver {q['giver']!r} is none of the deep's people")
        for q in d["quests"]:
            ev = [{"on": "finish", "do": "classquest", "level": lv} for lv in q.get("classquest", [])]
            self.add("quests", "quests", {"questid": q["id"]}, None,
                     self.quest_row(q, events=json.dumps(ev, separators=(",", ":")) if ev else ""))
            for n, s in enumerate(q["steps"], 1):
                if s["kind"] in ("talk", "kill") and s.get("target") not in people:
                    self.problems.append(f"quest {q['id']} step {n}: {s.get('target')!r} is none of the deep's people")
                self.add("quests", "queststeps", {"questid": q["id"], "step": n}, None, self.step_row(q["id"], n, s))
            self.lines["quests"].append(f"  {q['id']} L{q['minlevel']} {q['name']} ({q['giver']}; classes {q.get('classes', '3, 11')}"
                                        + (f"; classquest {q['classquest']}" if q.get("classquest") else "") + ")")
        for t in d["trips"]:
            self.add("quests", "quests", {"questid": t["id"]}, None, self.quest_row(t, repeatable=1))
            st = {"kind": "talk", "target": t["giver"], "journal": f"Tell {t['giver']} when you are ready.",
                  "events": json.dumps([{"on": "start", "do": "dostep"}, self.place(t["to"])], separators=(",", ":"))}
            self.add("quests", "queststeps", {"questid": t["id"], "step": 1}, None, self.step_row(t["id"], 1, st))
            self.lines["quests"].append(f"  {t['id']} trip {t['name']} ({t['giver']} -> {t['to']})")
        widened = 0
        for qid in d["widen"]["quests"]:
            r = row_of(db, "quests", {"questid": qid})
            if r is None:
                self.problems.append(f"quest {qid} (to take dwarves) is not in this database")
                continue
            new = races_with(r["races"], DWARF)
            if new is not None:
                self.add("quests", "quests", {"questid": qid}, r, dict(r, races=new))
                widened += 1
        self.lines["quests"].append(f"  race 4 added to {widened} of {len(d['widen']['quests'])} quests (the rest have it or take every race)")
        for qid in d["disable"]["quests"]:
            r = row_of(db, "quests", {"questid": qid})
            if r is not None and r["enabled"]:
                self.add("quests", "quests", {"questid": qid}, r, dict(r, enabled=0))
                self.lines["quests"].append(f"  disabled {qid} {r['name']} ({r['giver']})")
        bit = d["items"]["raceuse_add"]
        for pid in d["items"]["patterns"]:
            r = row_of(db, "itempattern", {"patternid": pid})
            if r is not None and not (r["raceuse"] & bit):
                self.add("quests", "itempattern", {"patternid": pid}, r, dict(r, raceuse=r["raceuse"] | bit))
                self.lines["quests"].append(f"  item {pid} {r['itemname']}: raceuse {r['raceuse']} -> {r['raceuse'] | bit}")

    def build(self):
        if not rows(self.db, "SELECT id FROM npcs WHERE world = ? LIMIT 1", (self.d["cave_world"],)):
            self.problems.append(f"this database has no people in world {self.d['cave_world']} (the Homelands): the deep is "
                                 "the Heirloom server's alone")
            return self
        for s in STEPS:
            getattr(self, s)()
        return self


# ---- checks ---------------------------------------------------------------------------------------------------------

def npc_faction(rules, name, world, x, z):
    """An NPC's faction as the server reads npcfaction (entity/faction.go): of its name, the row of a place it spawned
    within the radius of first, then the world's, then anywhere's; "" none."""
    best, rank = "", -1
    for w, rx, rz, radius, fac in rules.get(ms.npc_key(name), []):
        if w >= 0 and w != world:
            continue
        if radius > 0:
            if (x - rx) ** 2 + (z - rz) ** 2 > radius * radius:
                continue
            r = 2
        else:
            r = 1 if w >= 0 else 0
        if r > rank:
            best, rank = fac, r
    return best


def check(db, data, before=None):
    """After the write: a new dwarf of each class starts in the deep, its disc row at the camp, and no NPC standing
    within 300 of the deep's start, the camp or Freeport's landing turns KOS to it (one it was KOS to before, a
    creature hostile to every new character, is no change), nor is a coachman there of a faction it stands below
    entity.CoachFactionMin with (he would not sign his ledger: Moradhim's Doba by the camp, Freeport's Ronks); before:
    the start rows as they were (factionstart)."""
    probs = []
    def starts(rowsrc):
        st = {}
        for r in rowsrc:
            st.setdefault(r[3], []).append((r[0], r[1], r[2], r[4]))
        return st
    st = starts(db.execute("SELECT race, class, humtype, faction, standing FROM factionstart"))
    old = starts(before) if before is not None else st
    base = dict(db.execute("SELECT name, standing FROM factions"))
    rules = {}
    for name, w, rx, rz, radius, fac in db.execute("SELECT name, world, x, z, radius, faction FROM npcfaction"):
        rules.setdefault(ms.npc_key(name), []).append((w, rx, rz, radius, fac))
    dc = rows(db, "SELECT * FROM defaultcharacter WHERE race = ?", (DWARF,))
    for cls in data["factions"]["classes"]:
        mine = [r for r in dc if r["tclass"] == cls]
        deep = [r for r in mine if (r["world"] or 0) == data["start"]["world"]]
        disc = [r for r in mine if (r["world"] or 0) < 6]
        if not deep or not disc:
            probs.append(f"class {cls}: start rows deep {len(deep)}, disc {len(disc)}")
        combo = ms.combo_of(DWARF, cls, 0)
        for where in ("start", "camp", "freeport"):
            p = data[where]
            for name, x, z in db.execute("SELECT npc_name, x_coord, z_coord FROM npcs WHERE world = ? AND (x_coord - ?) * (x_coord - ?) + "
                                         "(z_coord - ?) * (z_coord - ?) < 90000", (p["world"], p["x"], p["x"], p["z"], p["z"])):
                fac = npc_faction(rules, name, p["world"], x, z)
                if not fac:
                    continue
                v = ms.resolve(st.get(fac, []), base.get(fac, 0), combo)
                was = ms.resolve(old.get(fac, []), base.get(fac, 0), combo)
                if v < KOS_BELOW <= was:
                    probs.append(f"class {cls}: {name} ({fac}) by the {where} turns KOS to it ({was} -> {v})")
                if name.startswith(("Coachman ", "Coachwoman ")) and v < COACH_FACTION_MIN:
                    probs.append(f"class {cls}: {name} ({fac}) by the {where} will not sign his ledger for it ({v} < {COACH_FACTION_MIN})")
    return probs


# ---- writing --------------------------------------------------------------------------------------------------------

class Writer:
    def __init__(self, db):
        self.db = db
        db.execute(LOG_DDL)
        db.execute(AUDIT_DDL)
        self.at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
        self.n = 0

    def audit(self, action, tbl, ref, before, after, detail):
        cur = self.db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                              "VALUES (?, 0, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                              (self.at, WHO, VIA, action, tbl, go_json(ref), None if before is None else go_json(before),
                               None if after is None else go_json(after), detail))
        return cur.lastrowid

    def log(self, step, tbl, ref, before, after, action):
        a = self.audit(action, tbl, ref, before, after, DETAIL)
        self.db.execute(f"INSERT INTO {LOG} (step, tbl, ref, before, after, audit, at) VALUES (?, ?, ?, ?, ?, ?, ?)",
                        (step, tbl, go_json(ref), None if before is None else go_json(before),
                         None if after is None else go_json(after), a, self.at))
        self.n += 1

    def insert(self, tbl, row):
        cols = list(row)
        return self.db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                               [row[c] for c in cols])

    def write(self, changes):
        for step, tbl, ref, before, after, note in changes:
            if tbl == "npcs":
                cur = self.insert("npcs", after["row"])
                nid = cur.lastrowid
                self.db.execute("UPDATE npcs SET npc_id = ? WHERE id = ?", (nid, nid))
                self.log(step, "npcs", {"id": nid}, None, row_of(self.db, "npcs", {"id": nid}), "insert")
                for it in after["stock"]:
                    it = {k: v for k, v in it.items() if k != "itemid"}
                    it["npcid"] = nid
                    c2 = self.insert("npcitem", it)
                    self.log(step, "npcitem", {"itemid": c2.lastrowid}, None, row_of(self.db, "npcitem", {"itemid": c2.lastrowid}), "insert")
            elif before is None:
                cur = self.insert(tbl, after)
                if ref is None:
                    ref = {KEYS[tbl][0]: cur.lastrowid}
                self.log(step, tbl, ref, None, row_of(self.db, tbl, ref), "insert")
            else:
                diff = [c for c in after if after[c] != before[c]]
                self.db.execute(f"UPDATE {tbl} SET " + ", ".join(f"{c} = ?" for c in diff) + " WHERE " +
                                " AND ".join(f"{c} = ?" for c in KEYS[tbl]), [after[c] for c in diff] + [ref[c] for c in KEYS[tbl]])
                self.log(step, tbl, ref, before, row_of(self.db, tbl, ref), "update")


def same(tbl, now, after):
    if now is None:
        return False
    if tbl == "npcs":
        return all(now.get(c) == after.get(c) for c in NPC_SAME)
    return go_json(now) == go_json(after)


def undo(db, data):
    if not has_table(db, LOG):
        print("nothing to undo: no log")
        return 0, 0
    wr = Writer(db)
    back = left = 0
    for seq, step, tbl, ref, before, after in db.execute(f"SELECT seq, step, tbl, ref, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        ref, before, after = json.loads(ref), before and json.loads(before), after and json.loads(after)
        now = row_of(db, tbl, ref)
        if not same(tbl, now, after):
            print(f"  left: {tbl} {ref}: changed since it was written")
            left += 1
            continue
        where = " AND ".join(f"{c} = ?" for c in KEYS[tbl])
        if before is None:
            db.execute(f"DELETE FROM {tbl} WHERE {where}", [ref[c] for c in KEYS[tbl]])
            wr.audit("delete", tbl, ref, now, None, f"undo: {DETAIL}")
        else:
            cols = list(before)
            db.execute(f"UPDATE {tbl} SET " + ", ".join(f"{c} = ?" for c in cols) + f" WHERE {where}",
                       [before[c] for c in cols] + [ref[c] for c in KEYS[tbl]])
            wr.audit("update", tbl, ref, now, row_of(db, tbl, ref), f"undo: {DETAIL}")
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    # the characters' marks of the deep's quests go with them
    ids = [q["id"] for q in data["quests"] + data["trips"]]
    if has_table(db, "charquests"):
        for r in rows(db, "SELECT * FROM charquests WHERE questid IN (%s)" % ",".join("?" * len(ids)), ids):
            if row_of(db, "quests", {"questid": r["questid"]}) is None:
                db.execute("DELETE FROM charquests WHERE serverid = ? AND questid = ?", (r["serverid"], r["questid"]))
                wr.audit("delete", "charquests", {"serverid": r["serverid"], "questid": r["questid"]}, r, None, f"undo: {DETAIL}")
    if not db.execute(f"SELECT 1 FROM {LOG}").fetchone():
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def backup(path):
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-evildwarf"
    src, out = sqlite3.connect(path), sqlite3.connect(dst)
    src.backup(out)
    out.close()
    src.close()
    print(f"backup: {dst}")


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--data", default=DATA)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    ap.add_argument("--no-backup", action="store_true")
    a = ap.parse_args(argv)
    if not os.path.exists(a.db):
        ap.error(f"no database {a.db}")
    data = json.load(open(a.data))
    if (a.apply or a.undo) and not a.no_backup:
        backup(a.db)
    db = sqlite3.connect(a.db, timeout=5)
    try:
        if a.undo:
            with db:
                back, left = undo(db, data)
            print(f"undo: {back} rows put back, {left} left as they are now")
            return 0
        if has_table(db, LOG) and db.execute(f"SELECT 1 FROM {LOG}").fetchone():
            n = db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0]
            print(f"applied already ({n} rows in {LOG}): nothing to do; --undo takes it back")
            return 0
        p = Plan(db, data).build()
        for s in STEPS:
            print(f"== {s}")
            for line in p.lines[s]:
                print(line)
        for pr in p.problems:
            print(f"PROBLEM: {pr}")
        print(f"{len(p.changes)} changes" + ("" if a.apply else " (--apply writes them)"))
        if p.problems:
            return 1
        if not a.apply:
            before = db.execute("SELECT race, class, humtype, faction, standing FROM factionstart").fetchall()
            mem = sqlite3.connect(":memory:")
            db.backup(mem)
            Writer(mem).write(Plan(mem, data).build().changes)
            probs = check(mem, data, before)
            mem.close()
            for pr in probs:
                print(f"CHECK: {pr}")
            print("check: every dwarf shadow knight and necromancer starts in the deep, its disc row at the camp, "
                  "KOS to no one by the deep, the camp or Freeport's landing, served by the coachmen there" if not probs else "")
            return 1 if probs else 0
        before = db.execute("SELECT race, class, humtype, faction, standing FROM factionstart").fetchall()
        with db:
            wr = Writer(db)
            wr.write(p.changes)
            probs = check(db, data, before)
            if probs:
                for pr in probs:
                    print(f"CHECK: {pr}")
                raise SystemExit("not written: the check after the write failed (rolled back)")
        print(f"applied: {wr.n} rows, each in gm_audit and {LOG}; the server reads them at its start (a keep restart's before.sh)")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
