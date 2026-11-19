#!/usr/bin/env python3
"""XP camps populated as the guides and the records have them, on both shards (the owner, 2026-11-18: "lets make sure
all XP camps are populated correctly with mob placements for heirloom and live"; owner-asked spawn work, every change
listed for the owner).

THE CAMPS (catalog(); every one with its source)
  guide   EQOA-Data's XP-camp guide (github.com/Jadiction/EQOA-Data, Map/zones.json: the camp points with a note), as
          tools/gen_camp_teles.py placed them on this server: internal/handlers/camps.go's campDests (the anchor, the
          creatures: gen_camp_teles.CAMPS' regex) and campNotes (the guide's level range).
  map     EQOA-Data's map's other camps (its 403 camp points without a note) and the FAQ's camps (Jarrod Garripoli's
          EQOA Frontiers FAQ, GameFAQs 48152, section XVI "Camps & Unique Places"), as gen_camp_teles.py placed them:
          internal/handlers/mapcamps.go's mapCamps (the anchor, the creatures by name, the map's level range).
  records the worlds the map does not draw (Rathe 1, Lavastorm 3, the Plane of Sky 4, the Plane of Disease 5): where a
          hostile kind's spawns cluster (RECORD_LINK, RECORD_MIN rows or more, its kinds the cluster's names with
          RECORD_KIND rows or more), as the records (npcs) have them; no level range but the cluster's own.
  The FAQ's level bands (1-20 ... 51-60) are matched to the camps by their creatures (faq_bands()) as a second guide.

A camp's creatures (members()): its kinds' rows in its world within CORE of its anchor, then any of its kinds within
LINK of one of those, out to REACH; never a friendly row (npc_type 0x80). Each shard is read on its own.

WHAT IS LOOKED AT, per camp and shard (survey())
  count by name and level, standing vs spawn-only (npcspawnonly), wander (npcwander's radius, npcmove's roam), respawn
  (npcrespawn, else the shard's respawn_after 15 s), the rows other tables hold (paths, quests, signs, rares, titles,
  camp spawns, doors, portals: protected), and the ground (cmd/campground: the ground package's Stand, Floor, Embedded,
  Free and the env, as the server and its movement check use them): a spawn in the ground (no floor under it, rock
  over it: groundnpc.go's inGroundLocked), in something placed (movecheck's ball: Embedded), on air (more than
  spawnedAbove 2.5 over the floor Stand finds, the terrain's or anything placed).

WHAT IS CHANGED (plan(); each change a unit of rows, its why written beside it)
  ground   a spawn in the ground goes on the floor over it (liftOutLocked's); one in something placed to Free's place
  doubles  rows of one name stacked on one spot (within DOUBLE xz, DOUBLE_DY y) are one spawn: the extra rows go (the
           lowest id, or a protected one, stays)
  levels   a row more than LEVEL_OFF levels from the median of its own name's rows in the camp, when the camp has
           LEVEL_KIN or more of them, takes that median (a grey or red stray in a band camp)
  shards   a camp's kind with fewer rows on one shard than on the other, where the other's extra rows are not doubles
           and the poorer shard's audit shows no deliberate removal there: copies on the poorer shard at the other's spots
  density  a guide camp (guide or map) with fewer than MIN_CAMP of its kinds after the above gets copies to MIN_CAMP,
           in proportion to its kinds, on its own open ground (cmd/campground's spot: a floor within reach of the
           copied creature's height, dry, clear of walls and placed things, no wall between them), each the farthest of
           CANDIDATES random places within SPREAD of the camp's creatures from every NPC, SPACING apart at least
  fork     the owner's 11-15 database ("the owner's database file of 11-15": "this has some updated camps but some is old
           from a fork", read-only, --fork): its own camp edits where it is the newer version (FORK_TAKE), each said
  owner    the owner's answers of 2026-11-18 (owner_units, propagate_units): the fork's frost giant camp by the Artic
           Spiritcaller on both shards, Heirloom's Xrag/bone hunter/Snowgrin/Quintashi rows at Live's levels, and the
           Tendora-Buki villagers, toxic pollutive slime and frost drake deleted on one shard deleted on the other
  A new row is a copy of the nearest row of its name in the camp (every npcs column but id and place: its body, gear,
  level, class, faction, template and loot, all by name or on the row), with that row's npcrespawn and npcwander rows.
  Never touched: a protected row (above), a spawn-only, seasonal (npcseason) or rare/named name (a proper noun, three
  rows or fewer in its world, a respawn over RARE_RESPAWN, an npcrare row), a row of a GM trigger.

Planning reads both shards' copies and the ground; applying needs neither (a before.sh holds the server seconds):

    python3 -E -s tools/xp_camps_20261118.py --heirloom H.db --live L.db --ground worlds/ground \\
        [--fork "OWNERS-DB.db"] [--eqoa-data ../external/EQOA-Data] --write-plan PLAN.json [--report]
    python3 -E -s tools/xp_camps_20261118.py --db eqoa.db --plan PLAN.json [--shard h|l] [--apply | --undo | --hash]

--apply in one transaction, a gm_audit line a row, log xp_camps_20261118_log (and _meta: the npcs sequence before); a
unit already in the log is done (a second --apply writes nothing); a unit whose rows are no longer as planned (a GM's
edit since) is left and said. --undo puts every logged row back newest first and the npcs sequence as it was (a row
changed since is left and said): npcs, npcrespawn and npcwander come back byte for byte (--hash prints their digests).
NPC rows are read at the server's start: apply in a keep restart's before.sh.
"""
import argparse
import collections
import datetime
import hashlib
import json
import math
import os
import random
import re
import sqlite3
import statistics
import subprocess
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import gen_camp_teles as gct  # noqa: E402
import tix4_rows as tr  # noqa: E402

TOOL = "tools/xp_camps_20261118.py"
LOG, META = "xp_camps_20261118_log", "xp_camps_20261118_meta"
DETAIL = "XP camps populated (the owner, 2026-11-18): " + TOOL
SOURCE = "xp-camps 20261118"
CAMPS_GO = os.path.join(HERE, "..", "internal", "handlers", "camps.go")
MAPCAMPS_GO = os.path.join(HERE, "..", "internal", "handlers", "mapcamps.go")
FAQ = os.path.expanduser("~/.cache/gamefaqs/faq48152.txt")

CORE, LINK, REACH = 150.0, 60.0, 300.0
RECORD_WORLDS = (1, 3, 4, 5)
RECORD_LINK, RECORD_MIN, RECORD_KIND = 45.0, 8, 3
DOUBLE, DOUBLE_DY = 1.0, 3.0
LEVEL_OFF, LEVEL_KIN = 5, 4
MIN_CAMP = 6
SPREAD, SPACING, CANDIDATES = 60.0, 12.0, 300
SPAWNED_ABOVE = 2.5          # groundnpc.go's spawnedAbove
FLOAT_FIX = 10.0             # on air by more than this is something the ground lacks: said, not moved
FLIER = re.compile(r"\b(bat|crow|eagle|hawk|raven|vulture|bird|wisp|gust|faerie|fairy|pixie|sprite|wasp|bee|fly|insect|cloud|spirit|ghost|phantom|wraith|shade|eye|imp|drake|wyvern|griffon|griffawn|harpy|aviak|gargoyle)s?\b", re.I)
RARE_RESPAWN = 600.0         # campfix's rare rule: back after more than 10 minutes
DEFAULT_RESPAWN = 15.0       # both shards' config respawn_after
SEED = 1118
FRIENDLY = 0x80


def low(s):
    return (s or "").strip().lower()


def proper(name):
    words = [w for w in re.split(r"\s+", (name or "").strip()) if w]
    big = [w for w in words if w.lower() not in {"of", "the", "a", "an"}]
    return bool(big) and all(w[:1].isupper() for w in big)


# ---- the camps -------------------------------------------------------------------------------------------------

def parse_go_map(path, var):
    text = open(path).read()
    body = text[text.index(f"var {var} = "):]
    body = body[:body.index("\n}\n")]
    out = {}
    for m in re.finditer(r'^\t"(\w+)":\s*\{(.*)\},?$', body, re.M):
        key, inner = m.group(1), m.group(2)
        d = {}
        for fm in re.finditer(r'(\w+): ("(?:[^"\\]|\\.)*"|[-\d.]+|true|false|\[\]string\{[^}]*\})', inner):
            k, v = fm.group(1), fm.group(2)
            if v.startswith('"'):
                v = json.loads(v)
            elif v.startswith("[]string"):
                v = [json.loads(s) for s in re.findall(r'"(?:[^"\\]|\\.)*"', v)]
            elif v in ("true", "false"):
                v = v == "true"
            else:
                v = float(v)
            d.setdefault(k, v)
        d["rares"] = re.findall(r'campRare\{Name: "([^"]*)"', inner)
        out[key] = d
    return out


def band_of(text):
    nums = [int(v) for v in re.findall(r"\d+", text or "")]
    if not nums or nums == [1, 1]:
        return None
    return (min(nums), max(nums))


def faq_camps():
    """The FAQ's camps: (band, name, [mobs], location)."""
    if not os.path.exists(FAQ):
        return []
    lines = open(FAQ, encoding="utf-8", errors="replace").read().split("\n")
    a = next(i for i, l in enumerate(lines) if "XVI. CAMPS & UNIQUE PLACES" in l)
    b = next(i for i, l in enumerate(lines) if "XXVI. RANDOM INFORMATION" in l and i > a)
    out, band, name = [], None, None
    i = a
    while i < b:
        line = lines[i]
        m = re.match(r"\s*\(\s+LEVELS ([0-9-]+)\s+\)", line)
        if m:
            band = band_of(m.group(1))
        m = re.match(r"^\(\s+(.+?)\s+\)\s*$", line)
        if m:
            name = m.group(1)
        if line.startswith("MOBS --"):
            mobs, j = line[7:].strip(), i + 1
            while lines[j].startswith("        "):
                mobs += " " + lines[j].strip()
                j += 1
            loc = lines[j][11:].strip() if lines[j].startswith("LOCATION") else ""
            out.append((band, name, [low(x) for x in re.split(r"[,;]", mobs) if x.strip()], loc))
        i += 1
    return out


def catalog(rows_h):
    """Every camp: dict(key, name, world, x, z, kinds (a test on a lower-case name), guide band, sources, rares)."""
    camps = []
    tele = parse_go_map(CAMPS_GO, "campDests")
    notes = parse_go_map(CAMPS_GO, "campNotes")
    spec = {s[0]: s for s in gct.CAMPS}
    for key, d in tele.items():
        s = spec.get(key)
        rx = re.compile(s[4], re.I) if s else re.compile("^" + re.escape(low(d["NPC"])) + "$")
        n = notes.get(key, {})
        camps.append(dict(key=key, name=d.get("Label", key), world=int(d["World"]), x=d["X"], z=d["Z"],
                          test=(lambda nm, rx=rx: bool(rx.search(nm))), kinds_text=s[4] if s else d["NPC"],
                          guide=band_of(n.get("Guide")), source="EQOA-Data XP-camp guide (camps.go)",
                          rares={low(r) for r in n.get("rares", [])}, note=n.get("Note", "")))
    for key, d in parse_go_map(MAPCAMPS_GO, "mapCamps").items():
        names = {low(c) for c in d.get("Creatures", [])}
        src = "the FAQ (GameFAQs 48152)" if key in {f[0] for f in gct.FAQ_CAMPS} else "EQOA-Data map camp"
        camps.append(dict(key=key, name=d.get("Label", key), world=int(d["World"]), x=d["X"], z=d["Z"],
                          test=(lambda nm, names=names: nm in names), kinds_text=", ".join(sorted(names)),
                          guide=band_of(d.get("Guide")), source=src + " (mapcamps.go)",
                          rares={low(r) for r in d.get("rares", [])}, note=""))
    camps += record_camps(rows_h, camps)
    # the FAQ's bands by creatures
    for band, fname, mobs, loc in faq_camps():
        if not band or not mobs:
            continue
        for c in camps:
            if c["world"] in RECORD_WORLDS and c["source"].startswith("records"):
                continue
            if any(c["test"](m) for m in mobs):
                c.setdefault("faq", []).append(f"{fname} ({band[0]}-{band[1]})")
                c.setdefault("faq_band", band)
    return camps


def record_camps(rows, guided):
    """Clusters of a hostile kind's spawns in the worlds the map does not draw."""
    out = []
    by_world = collections.defaultdict(list)
    count = collections.Counter((r["world"], r["key"]) for r in rows)
    for r in rows:
        if r["world"] in RECORD_WORLDS and r["hostile"] and count[(r["world"], r["key"])] >= RECORD_KIND \
                and not proper(r["name"]):
            by_world[r["world"]].append(r)
    for w, rs in sorted(by_world.items()):
        parent = list(range(len(rs)))

        def root(i):
            while parent[i] != i:
                parent[i] = parent[parent[i]]
                i = parent[i]
            return i
        grid = collections.defaultdict(list)
        cell = RECORD_LINK
        for i, r in enumerate(rs):
            grid[(int(r["x"] // cell), int(r["z"] // cell))].append(i)
        for i, r in enumerate(rs):
            cx, cz = int(r["x"] // cell), int(r["z"] // cell)
            for dx in (-1, 0, 1):
                for dz in (-1, 0, 1):
                    for j in grid.get((cx + dx, cz + dz), ()):
                        if j > i and math.hypot(rs[j]["x"] - r["x"], rs[j]["z"] - r["z"]) <= RECORD_LINK \
                                and abs(rs[j]["y"] - r["y"]) <= 40:
                            parent[root(i)] = root(j)
        sets = collections.defaultdict(list)
        for i in range(len(rs)):
            sets[root(i)].append(rs[i])
        for members in sets.values():
            if len(members) < RECORD_MIN:
                continue
            names = collections.Counter(m["key"] for m in members)
            kinds = {k for k, n in names.items() if n >= 2}
            cx = statistics.median(m["x"] for m in members)
            cz = statistics.median(m["z"] for m in members)
            anchor = min(members, key=lambda m: math.hypot(m["x"] - cx, m["z"] - cz))
            top = ", ".join(k for k, _ in names.most_common(3))
            key = f"rec{w}_{int(anchor['x'])}_{int(anchor['z'])}"
            out.append(dict(key=key, name=f"{top} (world {w}, {anchor['x']:.0f}, {anchor['z']:.0f})", world=w,
                            x=anchor["x"], z=anchor["z"], test=(lambda nm, kinds=kinds: nm in kinds),
                            kinds_text=", ".join(sorted(kinds)), guide=None, source="records (npcs clusters)",
                            rares=set(), note="", record=True))
    return out


# ---- reading a shard -------------------------------------------------------------------------------------------

def load(db):
    db.row_factory = sqlite3.Row
    cols = [r[1] for r in db.execute("PRAGMA table_info(npcs)")]
    pc = {r[0] & 0xFFFFFFFF for r in db.execute("SELECT modelid FROM charactermodel")} \
        if tr.table(db, "charactermodel") else set()
    rows = []
    for r in db.execute("SELECT * FROM npcs ORDER BY id"):
        d = dict(r)
        if d["x_coord"] is None or d["z_coord"] is None:
            continue
        model = (d["modelid"] or 0) & 0xFFFFFFFF
        rows.append(dict(row=d, id=d["id"], name=(d["npc_name"] or "").strip(), key=low(d["npc_name"]),
                         world=d["world"] or 0, x=d["x_coord"], y=d["y_coord"] or 0.0, z=d["z_coord"],
                         level=d["npc_level"] or 0, model=model, type=d["npc_type"] or 0,
                         hostile=not ((d["npc_type"] or 0) & FRIENDLY) and model not in pc and bool(model)))
    db.row_factory = None

    def ids(sql):
        try:
            return {r[0] for r in db.execute(sql)}
        except sqlite3.OperationalError:
            return set()

    def names(sql):
        return {low(x) for x in ids(sql)}
    protected = set()
    for sql in ("SELECT npcid FROM npcpaths", "SELECT npcid FROM queststeps WHERE npcid != 0",
                "SELECT npcid FROM signtext", "SELECT npcid FROM campspawns", "SELECT npcid FROM questwalls",
                "SELECT npc_id FROM gmportals", "SELECT npcid FROM npctitle", "SELECT npc_row FROM npcrare",
                "SELECT npc_row FROM npcrarecycle", "SELECT npcid FROM questdoors", "SELECT npcid FROM npcitem",
                "SELECT npc_row FROM npc_mirror_inbox"):
        protected |= ids(sql)
    trig = set()
    for (steps,) in (db.execute("SELECT steps FROM gmtriggers") if tr.table(db, "gmtriggers") else []):
        trig |= {low(m) for m in re.findall(r'"(?:npc|name)"\s*:\s*"([^"]+)"', steps or "")}
    respawn = {r[0]: r[1] for r in db.execute("SELECT npc_row, seconds FROM npcrespawn")}
    info = dict(cols=cols, rows=rows, paths=ids("SELECT npcid FROM npcpaths"), byid={r["id"]: r for r in rows}, protected=protected,
                spawnonly=names("SELECT name FROM npcspawnonly"), season=names("SELECT name FROM npcseason"),
                trigger=trig, respawn=respawn,
                wander={r[0]: r[1] for r in db.execute("SELECT npcid, radius FROM npcwander")},
                move={low(r[0]): (r[1], r[2], r[3]) for r in db.execute("SELECT name, roam, radius, water FROM npcmove")},
                count=collections.Counter((r["world"], r["key"]) for r in rows),
                rarenames={low(r[0]) for r in db.execute(
                    "SELECT n.npc_name FROM npcrare r JOIN npcs n ON n.id = r.npc_row")} | names("SELECT rare FROM npcrare"),
                heirloom=tr.table(db, "heirloom_seed_log"))
    info["deleted"] = deleted_spots(db)
    return info


def deleted_spots(db):
    """The npcs rows a GM or a tool deleted on this shard (gm_audit): (world, name, x, z, at, detail)."""
    out = []
    for before, at, detail in db.execute("SELECT before, at, detail FROM gm_audit WHERE tbl = 'npcs' AND action = 'delete'"
                                         " AND reverted_by IS NULL"):
        try:
            b = json.loads(before or "{}")
        except ValueError:
            continue
        if b.get("x_coord") is None:
            continue
        out.append((b.get("world") or 0, low(b.get("npc_name")), b["x_coord"], b["z_coord"], at, detail))
    return out


def rare_name(info, c, key, world):
    return (proper(key_spelling(info, key, world)) or info["count"][(world, key)] <= 3 or key in c["rares"]
            or key in info["rarenames"] or key in info["spawnonly"] or key in info["season"] or key in info["trigger"])


def key_spelling(info, key, world):
    if "spell" not in info:
        sp = collections.defaultdict(collections.Counter)
        for r in info["rows"]:
            sp[(r["world"], r["key"])][r["name"]] += 1
        info["spell"] = {k: c.most_common(1)[0][0] for k, c in sp.items()}
    return info["spell"].get((world, key), key)


def members(info, c):
    rs = [r for r in info["rows"] if r["world"] == c["world"] and not (r["type"] & FRIENDLY) and c["test"](r["key"])
          and math.hypot(r["x"] - c["x"], r["z"] - c["z"]) <= REACH]
    got = {r["id"] for r in rs if math.hypot(r["x"] - c["x"], r["z"] - c["z"]) <= CORE}
    grew = True
    while grew:
        grew = False
        for r in rs:
            if r["id"] in got:
                continue
            if any(math.hypot(r["x"] - o["x"], r["z"] - o["z"]) <= LINK for o in rs if o["id"] in got):
                got.add(r["id"])
                grew = True
    return [r for r in rs if r["id"] in got]


# ---- the ground (cmd/campground) -------------------------------------------------------------------------------

def campground(ground, questions):
    if not questions:
        return {}
    exe = os.environ.get("CAMPGROUND")
    cmd = [exe] if exe else ["go", "run", os.path.join(HERE, "..", "cmd", "campground")]
    p = subprocess.run(cmd + ["-ground", ground], input="\n".join(json.dumps(q) for q in questions) + "\n",
                       capture_output=True, text=True, check=True)
    out = {}
    for line in p.stdout.splitlines():
        a = json.loads(line)
        out[a["k"]] = a
    return out


def ground_verdict(r, a):
    """(what, new (x, y, z) or None) of a spawn's ground answer."""
    if not a or not a.get("knows"):
        return None, None
    if a.get("inground"):
        if a.get("lift") is not None:
            return f"in the ground ({a['lift'] - r['y']:.1f} under the floor over it)", (r["x"], a["lift"], r["z"])
        return "in the ground (no floor over it found)", None
    if a.get("embedded"):
        f = a.get("free")
        if f:
            return f"in something placed (free {math.hypot(f[0] - r['x'], f[2] - r['z']):.1f} away)", tuple(f)
        return "in something placed (no free place near)", None
    st = a.get("stand")
    if st is not None and r["y"] - st > SPAWNED_ABOVE and not (a.get("env", 0) & 3) and not FLIER.search(r.get("key", "")):
        up = r["y"] - st
        if up <= FLOAT_FIX:
            return f"on air ({up:.1f} over the floor)", (r["x"], st, r["z"])
        return f"on air ({up:.1f} over the floor: something the ground lacks, left)", None
    if st is None and a.get("floor") is None and not a.get("inground"):
        return None, None
    return None, None


# ---- the survey and the plan -----------------------------------------------------------------------------------

def survey(info, camps, ground):
    """Per camp: its members and what is wrong with them on this shard."""
    rows_of = {c["key"]: members(info, c) for c in camps}
    seen = {}
    for rs in rows_of.values():
        for r in rs:
            seen[r["id"]] = r
    answers = campground(ground, [dict(op="check", k=r["id"], w=r["world"], x=r["x"], y=r["y"], z=r["z"])
                                  for r in seen.values()]) if ground else {}
    out = {}
    for c in camps:
        rs = rows_of[c["key"]]
        s = dict(members=rs, ground=[], doubles=[], levels=[], respawn=[], notes=[])
        for r in rs:
            what, to = ground_verdict(r, answers.get(r["id"]))
            if what:
                s["ground"].append((r, what, to))
        # doubles: one name stacked on one spot
        by = collections.defaultdict(list)
        for r in rs:
            by[r["key"]].append(r)
        for key, kin in by.items():
            kin = sorted(kin, key=lambda r: (r["id"] not in info["protected"], r["id"]))
            kept = []
            for r in kin:
                twin = next((k for k in kept if math.hypot(k["x"] - r["x"], k["z"] - r["z"]) <= DOUBLE
                             and abs(k["y"] - r["y"]) <= DOUBLE_DY), None)
                if twin is None:
                    kept.append(r)
                else:
                    s["doubles"].append((r, twin))
            # levels: a stray of its own name's band
            if len(kin) >= LEVEL_KIN and not rare_name(info, c, key, c["world"]):
                med = int(statistics.median(r["level"] for r in kin))
                band = c.get("guide")
                for r in kin:
                    near = sum(1 for o in kin if abs(o["level"] - r["level"]) <= 3)
                    in_band = band and band[0] - 3 <= r["level"] <= band[1] + 3
                    if abs(r["level"] - med) > LEVEL_OFF and near < 3 and not in_band:
                        s["levels"].append((r, med))
        for r in rs:
            sec = info["respawn"].get(r["id"], DEFAULT_RESPAWN)
            if sec > RARE_RESPAWN and not rare_name(info, c, r["key"], c["world"]):
                s["respawn"].append((r, sec))
        out[c["key"]] = s
    return out


def touchable(info, c, r):
    return r["id"] not in info["protected"] and not rare_name(info, c, r["key"], r["world"])


def jrow(d):
    return {k: d[k] for k in sorted(d)}


def unit(camp, what, why, ops):
    return dict(camp=camp, what=what, why=why, ops=ops)


def plan_shard(name, info, other, camps, sv, sv_other, ground, fork_units, rnd):
    units, notes = [], collections.defaultdict(list)
    taken = set()  # ids a unit already changes
    parity_done = set()  # the other shard's rows copied already (camps overlap)

    def row_op(r, after):
        before = jrow(r["row"])
        return ["npcs", {"id": r["id"]}, before, None if after is None else jrow(after)]
    adds = collections.defaultdict(list)   # camp -> [(template row, x, y, z, facing, level, why)]
    for c in camps:
        s = sv[c["key"]]
        k = c["key"]
        for r, twin in s["doubles"]:
            if r["id"] in taken:
                continue
            if not touchable(info, c, r):
                notes[k].append(f"double of #{twin['id']} {r['name']} at ({r['x']:.0f}, {r['z']:.0f}): protected, left")
                continue
            ops = [row_op(r, None)]
            for tbl, col in (("npcrespawn", "npc_row"), ("npcwander", "npcid"), ("npcstands", "npc_row")):
                cur = tr.get(DBS[name], tbl, {col: r["id"]})
                if cur is not None:
                    ops.append([tbl, {col: r["id"]}, cur, None])
            units.append(unit(k, "remove", f"#{r['id']} {r['name']} L{r['level']} stacked on #{twin['id']} "
                              f"({math.hypot(r['x'] - twin['x'], r['z'] - twin['z']):.2f} apart): one spawn", ops))
            taken.add(r["id"])
        for r, what, to in s["ground"]:
            if r["id"] in taken:
                continue
            if to is None:
                notes[k].append(f"#{r['id']} {r['name']} ({r['x']:.0f}, {r['y']:.1f}, {r['z']:.0f}) {what}")
                continue
            if r["id"] in info["protected"]:
                notes[k].append(f"#{r['id']} {r['name']} {what}: a protected row, left")
                continue
            after = dict(r["row"], x_coord=round(to[0], 3), y_coord=round(to[1], 4), z_coord=round(to[2], 3))
            units.append(unit(k, "move", f"#{r['id']} {r['name']} {what}: to ({to[0]:.1f}, {to[1]:.2f}, {to[2]:.1f})",
                              [row_op(r, after)]))
            taken.add(r["id"])
        for r, med in s["levels"]:
            if r["id"] in taken or not touchable(info, c, r):
                continue
            after = dict(r["row"], npc_level=med)
            units.append(unit(k, "level", f"#{r['id']} {r['name']} L{r['level']} among its kind's L{med} "
                              f"(median of {sum(1 for m in s['members'] if m['key'] == r['key'])} here): L{med}",
                              [row_op(r, after)]))
            taken.add(r["id"])
        for r, sec in s["respawn"]:
            notes[k].append(f"#{r['id']} {r['name']} comes back after {sec:.0f} s (left: a long timer is a GM's call)")
        # shards: the other shard's camp has more of a kind
        mine = collections.Counter(r["key"] for r in s["members"])
        removed_here = {r["id"] for u in units if u["camp"] == k and u["what"] == "remove" for r in [info["byid"][u["ops"][0][1]["id"]]]}
        theirs_doubles = {r["id"] for r, _ in sv_other[k]["doubles"]}
        theirs = [r for r in sv_other[k]["members"] if r["id"] not in theirs_doubles]
        their_count = collections.Counter(r["key"] for r in theirs)
        mine_live = collections.Counter(r["key"] for r in s["members"] if r["id"] not in removed_here)
        for key, n in their_count.items():
            if n <= mine_live[key] or rare_name(info, c, key, c["world"]):
                continue
            mine_rows = [r for r in s["members"] if r["key"] == key and r["id"] not in removed_here]
            extra = [r for r in theirs if r["key"] == key
                     and not any(math.hypot(m["x"] - r["x"], m["z"] - r["z"]) <= 5 for m in mine_rows)]
            need = n - mine_live[key]
            extra = [r for r in extra if r["id"] not in parity_done]
            for r in extra[:need]:
                parity_done.add(r["id"])
                gone = [d for d in info["deleted"] if d[0] == c["world"] and d[1] == key
                        and math.hypot(d[2] - r["x"], d[3] - r["z"]) <= 30]
                if gone and key in OWNER_PROPAGATE:
                    PROPAGATE.append((other_name(name), k, gone[0], r))
                    continue
                if gone:
                    notes[k].append(f"{r['name']} at ({r['x']:.0f}, {r['z']:.0f}): the other shard has it, this one's "
                                    f"was deleted {gone[0][4][:16]} ({gone[0][5][:80]}): left for the owner")
                    continue
                tmpl = nearest_template(info, c, key, r["x"], r["z"], s["members"])
                if tmpl is None:
                    notes[k].append(f"{r['name']}: no row of it on this shard to copy")
                    continue
                adds[k].append((tmpl, r["x"], r["y"], r["z"], r["row"]["facing"], r["level"],
                                f"the other shard's #{r['id']} {r['name']} L{r['level']} at ({r['x']:.0f}, {r['z']:.0f}) "
                                f"(this shard had {mine_live[key]} of {n})"))
        # density
        have = len(s["members"]) - len(removed_here) + len(adds[k])
        if not c.get("record") and have < MIN_CAMP and have > 0:
            kinds = collections.Counter(r["key"] for r in s["members"] if touchable(info, c, r))
            if not kinds:
                notes[k].append(f"{have} creatures, all named/rare/protected: no common kind to copy")
            else:
                want = MIN_CAMP - have
                pick = share(kinds, want)
                spots = find_spots(info, c, s["members"], want, ground, rnd,
                                   [(a[1], a[3]) for lst in adds.values() for a in lst])
                for key, cnt in pick.items():
                    for _ in range(cnt):
                        if not spots:
                            notes[k].append(f"no open ground left for one more {key}")
                            break
                        (x, y, z, near) = spots.pop(0)
                        tmpl = nearest_template(info, c, key, x, z, s["members"])
                        lv = tmpl["level"]
                        adds[k].append((tmpl, x, y, z, rnd.randrange(256), lv,
                                        f"the camp had {have} creatures of its kinds (fewer than {MIN_CAMP})"))
        elif have == 0:
            notes[k].append("none of its creatures on this shard")
    for k, lst in adds.items():
        for tmpl, x, y, z, facing, lv, why in lst:
            row = dict(tmpl["row"])
            row.pop("id", None)
            row.update(x_coord=round(x, 3), y_coord=round(y, 4), z_coord=round(z, 3), facing=int(facing) % 256,
                       dt_kill=None, npc_level=lv)
            ops = [["npcs", {"id": "NEW"}, None, jrow(row)]]
            sec = info["respawn"].get(tmpl["id"])
            if sec is not None:
                cur = tr.get(DBS[name], "npcrespawn", {"npc_row": tmpl["id"]})
                ops.append(["npcrespawn", {"npc_row": "NEW"}, None,
                            dict(npc_row="NEW", seconds=cur["seconds"], note=f"as row {tmpl['id']}'s; {SOURCE}",
                                 source=cur["source"] or SOURCE)])
            wr = info["wander"].get(tmpl["id"])
            if wr is not None:
                ops.append(["npcwander", {"npcid": "NEW"}, None,
                            dict(npcid="NEW", radius=wr, note=f"as row {tmpl['id']}'s; {SOURCE}", source=SOURCE)])
            units.append(unit(k, "add", f"{tmpl['name']} L{lv} at ({x:.1f}, {y:.2f}, {z:.1f}), a copy of #{tmpl['id']}: {why}", ops))
    for u in fork_units:
        rid = u["ops"][0][1]["id"]
        if rid in taken:
            notes[u["camp"]].append(f"fork: #{rid}: this tool's own change to it stands over the fork's ({u['why'][:120]})")
            continue
        units.append(u)
    return units, notes


def share(kinds, want):
    total = sum(kinds.values())
    raw = {k: want * n / total for k, n in kinds.items()}
    got = {k: int(v) for k, v in raw.items()}
    for k in sorted(raw, key=lambda k: (-(raw[k] - got[k]), k))[:want - sum(got.values())]:
        got[k] += 1
    return {k: v for k, v in got.items() if v}


def nearest_template(info, c, key, x, z, mem):
    rs = [r for r in mem if r["key"] == key and touchable(info, c, r)]
    if not rs:
        rs = [r for r in info["rows"] if r["world"] == c["world"] and r["key"] == key and touchable(info, c, r)]
    if not rs:
        return None
    return min(rs, key=lambda r: (math.hypot(r["x"] - x, r["z"] - z), r["id"]))


def find_spots(info, c, mem, want, ground, rnd, extra):
    """want places on the camp's own ground, each the farthest of CANDIDATES from every NPC (and the places before)."""
    if not mem or not ground:
        return []
    others = [(r["x"], r["z"]) for r in info["rows"] if r["world"] == c["world"]
              and abs(r["x"] - c["x"]) < REACH + 2 * SPREAD and abs(r["z"] - c["z"]) < REACH + 2 * SPREAD] + list(extra)
    qs = []
    for i in range(CANDIDATES):
        m = rnd.choice(mem)
        a, d = rnd.random() * 2 * math.pi, SPREAD * math.sqrt(rnd.random())
        x, z = m["x"] + d * math.sin(a), m["z"] + d * math.cos(a)
        qs.append(dict(op="spot", k=i, w=c["world"], x=round(x, 2), z=round(z, 2), hint=m["y"], fx=m["x"], fy=m["y"], fz=m["z"]))
    ans = campground(ground, qs)
    ok = []
    for q in qs:
        a = ans.get(q["k"])
        if not a or a.get("stand") is None or a.get("roofed") or a.get("embedded") or a.get("walled") or a.get("env", 0) & 15:
            continue
        ok.append((q["x"], a["stand"], q["z"]))
    out = []
    for _ in range(want):
        best = None
        for x, y, z in ok:
            d = min((math.hypot(x - ox, z - oz) for ox, oz in others), default=1e9)
            if d >= SPACING and (best is None or d > best[0]):
                best = (d, x, y, z)
        if best is None:
            break
        out.append((best[1], best[2], best[3], best[0]))
        others.append((best[1], best[3]))
    return out


DBS = {}
PROPAGATE = []  # (shard, camp, the deletion, the row there): the owner's answer 4


def other_name(sh):
    return "l" if sh == "h" else "h"


# ---- the owner's answers (2026-11-18) ---------------------------------------------------------------------------
#
# 1. A guide camp's minimum stays MIN_CAMP (6).
# 2. The tenacious frost giant camp round the Artic Spiritcaller (Snowblind Plains, about 7008, 5853): the fork's
#    version on both shards: its 11 giants' spots at size 2.3 and the Spiritcaller at its spot at size 3.0, made from
#    our own rows (names, bodies, levels, gear, loot, templates ours; the fork's spellings not taken): the 11 of our
#    giants nearest the fork's spots move there, the rest go. FORK_TFG: the fork's rows (id there, x, y, z, facing).
# 3. Heirloom's child/pariah of Xrag, bone hunters, Snowgrin and Quintashi at Live's levels (the same row: id, name
#    and spot on both).
# 4. A row deleted on one shard (gm_audit) that the other still has, for OWNER_PROPAGATE's names: deleted there too.
TFG_CENTER, TFG_REACH = (7008.0, 5853.0), 80.0
FORK_TFG = [(101490, 6969.7, 54.1, 5856.2, 191), (162682, 6976.0, 54.1, 5833.2, 174), (162683, 6992.2, 54.1, 5816.0, 151),
            (162684, 7017.4, 54.0, 5811.4, 117), (162685, 7047.4, 54.1, 5845.5, 65), (162686, 7037.6, 53.7, 5820.6, 94),
            (162687, 7043.4, 54.1, 5871.1, 55), (162688, 7029.2, 54.1, 5884.9, 19), (162689, 7008.0, 54.1, 5892.7, 4),
            (162690, 6986.0, 54.1, 5887.8, 241), (162691, 6973.1, 54.1, 5873.3, 215)]
TFG_SIZE = 2.3
FORK_SPIRITCALLER = (101488, 7015.1, 54.1, 5869.6, 21, 3.0)
LIVE_LEVEL_NAMES = {"child of xrag", "pariah of xrag", "bone hunter", "snowgrin", "quintashi"}
OWNER_PROPAGATE = {"tendora-buki villager", "toxic pollutive slime", "frost drake"}
OWNER = "the owner's answer, 2026-11-18"


def side_rows(db, rid):
    out = []
    for tbl, col in (("npcrespawn", "npc_row"), ("npcwander", "npcid"), ("npcstands", "npc_row")):
        cur = tr.get(db, tbl, {col: rid}) if tr.table(db, tbl) else None
        if cur is not None:
            out.append([tbl, {col: rid}, cur, None])
    return out


def owner_units(sh, db, info, other, camps):
    units, notes = [], []
    camp = min(camps, key=lambda c: (c["world"] != 0, math.hypot(c["x"] - TFG_CENTER[0], c["z"] - TFG_CENTER[1])))["key"]
    giants = [r for r in info["rows"] if r["world"] == 0 and r["key"] == "tenacious frost giant"
              and math.hypot(r["x"] - TFG_CENTER[0], r["z"] - TFG_CENTER[1]) <= TFG_REACH]
    pairs = sorted(((math.hypot(r["x"] - f[1], r["z"] - f[3]), r["id"] in info["protected"], r["id"], f[0])
                    for r in giants for f in FORK_TFG), key=lambda t: (not t[1], t[0], t[2], t[3]))
    got, used = {}, set()
    for d, _, rid, fid in pairs:
        if rid in got or fid in used:
            continue
        got[rid], used = fid, used | {fid}
    spot = {f[0]: f for f in FORK_TFG}
    for r in sorted(giants, key=lambda r: r["id"]):
        if r["id"] in got:
            f = spot[got[r["id"]]]
            after = dict(r["row"], x_coord=f[1], y_coord=f[2], z_coord=f[3], facing=f[4], size=TFG_SIZE)
            units.append(unit(camp, "owner", f"#{r['id']} tenacious frost giant L{r['level']}: to the fork's #{f[0]} spot "
                              f"({f[1]}, {f[2]}, {f[3]}) at size {TFG_SIZE} ({OWNER}: the fork's frost giant camp)",
                              [["npcs", {"id": r["id"]}, jrow(r["row"]), jrow(after)]]))
        elif r["id"] in info["protected"]:
            notes.append((camp, f"#{r['id']} tenacious frost giant: protected, left (the fork's camp has 11)"))
        else:
            units.append(unit(camp, "owner", f"#{r['id']} tenacious frost giant L{r['level']} at ({r['x']:.0f}, {r['z']:.0f}): "
                              f"removed ({OWNER}: the fork's camp has 11 giants, ours had {len(giants)})",
                              [["npcs", {"id": r["id"]}, jrow(r["row"]), None]] + side_rows(db, r["id"])))
    sc = info["byid"].get(FORK_SPIRITCALLER[0])
    if sc and sc["key"] == "artic spiritcaller":
        _, x, y, z, fa, size = FORK_SPIRITCALLER
        after = dict(sc["row"], x_coord=x, y_coord=y, z_coord=z, facing=fa, size=size)
        units.append(unit(camp, "owner", f"#{sc['id']} {sc['name']}: to the fork's spot ({x}, {y}, {z}) at size {size} ({OWNER})",
                          [["npcs", {"id": sc["id"]}, jrow(sc["row"]), jrow(after)]]))
    else:
        notes.append((camp, f"#{FORK_SPIRITCALLER[0]} is no Artic Spiritcaller here: left"))
    if sh == "h":
        for r in info["rows"]:
            if r["key"] not in LIVE_LEVEL_NAMES:
                continue
            o = other["byid"].get(r["id"])
            if o is None or o["key"] != r["key"] or math.hypot(o["x"] - r["x"], o["z"] - r["z"]) > 1 or o["level"] == r["level"]:
                continue
            c = min((c for c in camps if c["world"] == r["world"]), key=lambda c: math.hypot(c["x"] - r["x"], c["z"] - r["z"]))
            after = dict(r["row"], npc_level=o["level"])
            units.append(unit(c["key"], "owner", f"#{r['id']} {r['name']} L{r['level']}: Live's L{o['level']} ({OWNER})",
                              [["npcs", {"id": r["id"]}, jrow(r["row"]), jrow(after)]]))
    return units, notes


def propagate_units(db, info, asks):
    """asks: (camp, the other shard's deletion, this shard's row): delete the row here too."""
    units, notes, seen = [], [], set()
    for camp, gone, r in asks:
        if r["id"] in seen:
            continue
        seen.add(r["id"])
        if r["id"] in info["protected"]:
            notes.append((camp, f"#{r['id']} {r['name']}: deleted on the other shard, protected here: left"))
            continue
        units.append(unit(camp, "owner", f"#{r['id']} {r['name']} at ({r['x']:.0f}, {r['z']:.0f}): deleted, as the other shard's "
                          f"was {gone[4][:16]} ({gone[5][:60]}) ({OWNER})",
                          [["npcs", {"id": r["id"]}, jrow(r["row"]), None]] + side_rows(db, r["id"])))
    return units, notes


# ---- the owner's 11-15 database (a fork) ----------------------------------------------------------------------

def touched_ids(db):
    """The npcs ids this shard's own history accounts for: gm_audit's npcs lines and every tool log naming a row."""
    out = set()
    for (rk,) in db.execute("SELECT rowkey FROM gm_audit WHERE tbl = 'npcs'"):
        try:
            i = json.loads(rk).get("id")
        except (ValueError, AttributeError):
            continue
        if isinstance(i, int) or (isinstance(i, str) and i.isdigit()):
            out.add(int(i))
    live = {"npcs", "npcitem", "npcrespawn", "npcwander", "npcpaths", "queststeps", "npcstands", "npcrare", "npctitle",
            "signtext", "campspawns", "questwalls", "gmportals", "questdoors", "npcrarecycle", "npc_mirror_inbox",
            "npcpaths_seed", "signtext_seed", LOG}
    for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table'"):
        if t in live:
            continue
        cs = [r[1] for r in db.execute(f"PRAGMA table_info('{t}')")]
        for col in ("npc_row", "npcid", "npc_id"):
            if col in cs:
                out |= {i for (i,) in db.execute(f"SELECT {col} FROM '{t}'") if isinstance(i, int)}
                break
    return out


FORK_COLS = ("npc_name", "x_coord", "y_coord", "z_coord", "npc_level", "modelid", "size")


def fork_edits(fork, infos, dbs):
    """The fork's rows that differ from a shard's where neither shard's history accounts for the difference: its own
    edits (or ours made without a trace: either way, the GM's to judge, unless the ground or the kin decide)."""
    touched = set()
    for db in dbs.values():
        touched |= touched_ids(db)
    fr = {}
    for r in fork.execute(f"SELECT id, world, {', '.join(FORK_COLS)} FROM npcs"):
        fr[r[0]] = dict(zip(("id", "world") + FORK_COLS, r))
    edits = []
    for i, f in fr.items():
        if i in touched or f["x_coord"] is None:
            continue
        for sh, info in infos.items():
            r = info["byid"].get(i)
            if r is None:
                edits.append(dict(shard=sh, id=i, kind="fork-only row", fork=f, ours=None))
                continue
            o = r["row"]
            what = []
            if low(o["npc_name"]) != low(f["npc_name"]):
                what.append("name")
            if math.hypot(o["x_coord"] - f["x_coord"], o["z_coord"] - f["z_coord"]) > 1 or abs((o["y_coord"] or 0) - f["y_coord"]) > 1:
                what.append("place")
            if o["npc_level"] != f["npc_level"]:
                what.append("level")
            if o["modelid"] != f["modelid"]:
                what.append("body")
            if (o["size"] or 0) != (f["size"] or 0):
                what.append("size")
            if what:
                edits.append(dict(shard=sh, id=i, kind="+".join(what), fork=f, ours=o))
    for sh, info in infos.items():
        for r in info["rows"]:
            if r["id"] <= max(fr) and r["id"] not in fr and r["id"] not in touched:
                edits.append(dict(shard=sh, id=r["id"], kind="not in the fork", fork=None, ours=r["row"]))
    return edits


def fork_review(edits, camps, infos, ground):
    """Per camp and shard: (which source won, why) lines, and the units that take the fork's version."""
    by_camp = collections.defaultdict(list)
    ckeys = {}
    for e in edits:
        ref = e["ours"] or e["fork"]
        nm = low(ref["npc_name"])
        for c in camps:
            if c["world"] == ref["world"] and c["test"](nm) and \
                    math.hypot(ref["x_coord"] - c["x"], ref["z_coord"] - c["z"]) <= REACH:
                by_camp[c["key"]].append(e)
                ckeys[c["key"]] = c
                break
    qs = []
    for k, es in by_camp.items():
        for n, e in enumerate(es):
            if e["kind"].startswith("place") or "+place" in e["kind"]:
                for side in ("fork", "ours"):
                    v = e[side]
                    qs.append(dict(op="check", k=f"{e['shard']}:{e['id']}:{side}", w=v["world"], x=v["x_coord"],
                                   y=v["y_coord"], z=v["z_coord"]))
    ans = campground(ground, qs) if ground else {}
    lines = collections.defaultdict(list)
    units = collections.defaultdict(list)
    for k, es in by_camp.items():
        c = ckeys[k]
        for e in es:
            sh, info = e["shard"], infos[e["shard"]]
            o, f = e["ours"], e["fork"]
            nm = (o or f)["npc_name"]
            tag = f"[{sh}] #{e['id']} {nm}"
            if e["kind"] == "fork-only row":
                lines[k].append(f"{tag}: only the fork has it at ({f['x_coord']:.0f}, {f['z_coord']:.0f}) L{f['npc_level']} "
                                f"size {f['size']}: AMBIGUOUS (the fork's own add; ids collide with ours past its fork point)")
                continue
            if e["kind"] == "not in the fork":
                lines[k].append(f"{tag}: ours, at ({o['x_coord']:.0f}, {o['z_coord']:.0f}) L{o['npc_level']}, the fork removed "
                                f"it (no trace on our side): AMBIGUOUS")
                continue
            parts = e["kind"].split("+")
            if "place" in parts:
                fa, oa = ans.get(f"{sh}:{e['id']}:fork"), ans.get(f"{sh}:{e['id']}:ours")
                r_f = dict(x=f["x_coord"], y=f["y_coord"], z=f["z_coord"])
                r_o = dict(x=o["x_coord"], y=o["y_coord"], z=o["z_coord"])
                vf, vo = ground_verdict(r_f, fa)[0], ground_verdict(r_o, oa)[0]
                d = math.hypot(o["x_coord"] - f["x_coord"], o["z_coord"] - f["z_coord"])
                if vo and not vf and d <= REACH and e["id"] not in info["protected"]:
                    after = dict(info["byid"][e["id"]]["row"], x_coord=f["x_coord"], y_coord=f["y_coord"], z_coord=f["z_coord"])
                    units[sh].append(unit(k, "fork", f"#{e['id']} {nm}: ours {vo}, the fork's spot "
                                          f"({f['x_coord']:.1f}, {f['y_coord']:.1f}, {f['z_coord']:.1f}) {d:.0f} away stands on "
                                          f"the floor: the fork's", [["npcs", {"id": e["id"]}, jrow(info["byid"][e["id"]]["row"]), jrow(after)]]))
                    lines[k].append(f"{tag}: place: FORK wins (ours {vo}; its spot on the floor, {d:.0f} away)")
                elif vf and not vo:
                    lines[k].append(f"{tag}: place: OURS wins (the fork's spot {vf}; ours on the floor, {d:.0f} away)")
                elif d > REACH:
                    lines[k].append(f"{tag}: place: OURS wins (the fork's spot is {d:.0f} away, out of the camp)")
                else:
                    lines[k].append(f"{tag}: place: AMBIGUOUS (both on the floor, {d:.0f} apart: ours "
                                    f"({o['x_coord']:.0f}, {o['y_coord']:.1f}, {o['z_coord']:.0f}), the fork's "
                                    f"({f['x_coord']:.0f}, {f['y_coord']:.1f}, {f['z_coord']:.0f}))")
            if "size" in parts:
                kin = collections.Counter(r["row"]["size"] for r in info["rows"] if r["key"] == low(nm) and r["world"] == o["world"])
                mode = kin.most_common(1)[0][0]
                fk = sum(1 for e2 in by_camp[k] if e2["shard"] == sh and e2["fork"] and low(e2["fork"]["npc_name"]) == low(nm)
                         and e2["fork"]["size"] == f["size"])
                if o["size"] == mode and f["size"] != mode:
                    lines[k].append(f"{tag}: size: OURS wins ({o['size']} as {kin[mode]} of its name; the fork's {f['size']} on "
                                    f"{fk} of them)")
                else:
                    lines[k].append(f"{tag}: size: AMBIGUOUS (ours {o['size']}, the fork's {f['size']})")
            if "level" in parts:
                band = c.get("guide")
                if band and (band[0] - 3 <= o["npc_level"] <= band[1] + 3) != (band[0] - 3 <= f["npc_level"] <= band[1] + 3):
                    win = "OURS" if band[0] - 3 <= o["npc_level"] <= band[1] + 3 else "FORK"
                    lines[k].append(f"{tag}: level: {win} wins (ours L{o['npc_level']}, the fork's L{f['npc_level']}, "
                                    f"the guide's {band[0]}-{band[1]})")
                    if win == "FORK" and e["id"] not in info["protected"]:
                        after = dict(info["byid"][e["id"]]["row"], npc_level=f["npc_level"])
                        units[sh].append(unit(k, "fork", f"#{e['id']} {nm} L{o['npc_level']}: the fork's L{f['npc_level']} is in "
                                              f"the guide's band {band[0]}-{band[1]}",
                                              [["npcs", {"id": e["id"]}, jrow(info["byid"][e["id"]]["row"]), jrow(after)]]))
                else:
                    other = [s for s in infos if s != sh]
                    same = other and infos[other[0]]["byid"].get(e["id"]) and infos[other[0]]["byid"][e["id"]]["row"]["npc_level"] == f["npc_level"]
                    lines[k].append(f"{tag}: level: AMBIGUOUS (ours L{o['npc_level']}, the fork's L{f['npc_level']}"
                                    + (f"; the other shard has the fork's" if same else "") + ")")
            if "body" in parts:
                lines[k].append(f"{tag}: body: OURS wins (ours {o['modelid']}, the fork's {f['modelid']}: a body is never "
                                f"changed by a guess)")
            if "name" in parts:
                lines[k].append(f"{tag}: name: OURS wins ({o['npc_name']!r} over the fork's {f['npc_name']!r})")
    return lines, units


# ---- applying, undoing ---------------------------------------------------------------------------------------

def unit_id(u):
    return hashlib.sha1(json.dumps([u["camp"], u["what"], u["why"]], sort_keys=True).encode()).hexdigest()[:16]


def log_ddl():
    return (f"CREATE TABLE IF NOT EXISTS {LOG} (seq INTEGER PRIMARY KEY AUTOINCREMENT, unit TEXT NOT NULL, camp TEXT NOT NULL,"
            f" what TEXT NOT NULL, tbl TEXT NOT NULL, rowkey TEXT NOT NULL, before TEXT, after TEXT, audit INTEGER NOT NULL DEFAULT 0)",
            f"CREATE TABLE IF NOT EXISTS {META} (key TEXT PRIMARY KEY, value TEXT)")


def same(now, want):
    return now is not None and all(now.get(c) == v for c, v in want.items())


def unit_state(db, u, done_units):
    if unit_id(u) in done_units:
        return "done", ""
    for tbl, key, before, after in u["ops"]:
        if "NEW" in key.values():
            continue
        now = tr.get(db, tbl, key)
        if after is not None and same(now, after) or after is None and now is None:
            continue
        if before is None and now is not None:
            return "conflict", f"{tbl} {key} is there already"
        if before is not None and not same(now, before):
            return "conflict", f"{tbl} {key} is not as planned (changed since)"
    if all(("NEW" not in key.values()) and ((after is None and tr.get(db, tbl, key) is None) or
                                             (after is not None and same(tr.get(db, tbl, key), after)))
           for tbl, key, before, after in u["ops"]):
        return "as-planned", ""
    return "todo", ""


def apply_units(db, units):
    for ddl in log_ddl():
        db.execute(ddl)
    if db.execute(f"SELECT 1 FROM {META} WHERE key = 'seq_before'").fetchone() is None:
        r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'npcs'").fetchone()
        db.execute(f"INSERT INTO {META} VALUES ('seq_before', ?)", (json.dumps(None if r is None else r[0]),))
    done_units = {r[0] for r in db.execute(f"SELECT DISTINCT unit FROM {LOG}")}
    count = collections.Counter()
    left = []
    for u in units:
        st, why = unit_state(db, u, done_units)
        count[st] += 1
        if st == "conflict":
            left.append(f"  left: {u['camp']} {u['what']}: {u['why']} ({why})")
        if st != "todo":
            continue
        new_id = None
        uid = unit_id(u)
        for tbl, key, before, after in u["ops"]:
            key = {k: (new_id if v == "NEW" else v) for k, v in key.items()}
            if after is not None:
                after = {k: (new_id if v == "NEW" else v) for k, v in after.items()}
            if tbl == "npcs" and before is None:
                cols = [c for c in after if c != "id"]
                cur = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                                 [after[c] for c in cols])
                new_id = cur.lastrowid
                key = {"id": new_id}
                now = tr.get(db, "npcs", key)
                res = ("insert", None, now)
            else:
                res = tr.write(db, tbl, key, after)
            if res is None:
                continue
            action, b, now = res
            a = tr.audit(db, action, tbl, key, b, now, DETAIL + f" ({u['camp']}: {u['why'][:300]})")
            db.execute(f"INSERT INTO {LOG} (unit, camp, what, tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
                       (uid, u["camp"], u["what"], tbl, json.dumps(key, sort_keys=True),
                        None if b is None else json.dumps(b, sort_keys=True), None if now is None else json.dumps(now, sort_keys=True), a))
            count["rows"] += 1
    return count, left


def undo(db):
    if not tr.table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, tbl, rowkey, before, after in db.execute(f"SELECT seq, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key = json.loads(rowkey)
        now = tr.get(db, tbl, key)
        if (now is None) != (after is None) or (now is not None and json.dumps(now, sort_keys=True) != after):
            left += 1
            print(f"  left: {tbl} {key}, changed since it was written")
            continue
        was = None if before is None else json.loads(before)
        where = " AND ".join(f"{k} = ?" for k in key)
        if was is None:
            db.execute(f"DELETE FROM {tbl} WHERE {where}", list(key.values()))
        elif now is None:
            cols = list(was)
            db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [was[c] for c in cols])
        else:
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in was)} WHERE {where}", list(was.values()) + list(key.values()))
        tr.audit(db, "undo", tbl, key, now, tr.get(db, tbl, key), "undo: " + DETAIL)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if left == 0:
        seq = json.loads(db.execute(f"SELECT value FROM {META} WHERE key = 'seq_before'").fetchone()[0])
        top = db.execute("SELECT max(id) FROM npcs").fetchone()[0] or 0
        if seq is None:
            db.execute("DELETE FROM sqlite_sequence WHERE name = 'npcs'")
        elif top <= seq:
            db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = 'npcs'", (seq,))
        db.execute(f"DROP TABLE {LOG}")
        db.execute(f"DROP TABLE {META}")
    return back, left


def digest(db, table):
    h = hashlib.md5()
    if tr.table(db, table):
        for r in db.execute(f"SELECT * FROM {table} ORDER BY 1"):
            h.update(repr(tuple(r)).encode())
    return h.hexdigest()


# ---- the plan and its report ------------------------------------------------------------------------------------

def counts(rs):
    c = collections.Counter(r["name"] for r in rs)
    lv = collections.defaultdict(list)
    for r in rs:
        lv[r["name"]].append(r["level"])
    return ", ".join(f"{n} x{c[n]} L{min(lv[n])}" + (f"-{max(lv[n])}" if max(lv[n]) != min(lv[n]) else "")
                     for n in sorted(c, key=lambda n: (-c[n], n)))


def behaviour(info, rs):
    stand = sum(1 for r in rs if info["wander"].get(r["id"], 1) <= 0 and r["id"] in info["wander"])
    walk = collections.Counter()
    for r in rs:
        m = info["move"].get(r["key"])
        walk["path" if r["id"] in info["paths"] else
             ("roam" if m and m[0] == 1 else "stands" if m and m[0] == 0 else "default")] += 1
    resp = collections.Counter(info["respawn"].get(r["id"], DEFAULT_RESPAWN) for r in rs)
    rtxt = ", ".join(f"{s:.0f}s x{n}" for s, n in sorted(resp.items()))
    return f"respawn {rtxt}; npcwander stand/fixed {stand}; npcmove {dict(walk)}"


def make_plan(a):
    dbs = {"h": sqlite3.connect(f"file:{a.heirloom}?mode=ro", uri=True), "l": sqlite3.connect(f"file:{a.live}?mode=ro", uri=True)}
    DBS.update(dbs)
    infos = {k: load(db) for k, db in dbs.items()}
    camps = catalog(infos["h"]["rows"])
    svs = {k: survey(infos[k], camps, a.ground) for k in infos}
    fork_lines, fork_units = collections.defaultdict(list), collections.defaultdict(list)
    if a.fork:
        fork = sqlite3.connect(f"file:{a.fork}?mode=ro", uri=True)
        edits = fork_edits(fork, infos, dbs)
        fork_lines, fork_units = fork_review(edits, camps, infos, a.ground)
    rnd = random.Random(SEED)
    doc = dict(about=f"{TOOL}: per shard, units of [table, key, before, after]; NEW is the npcs row a unit adds",
               made=datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
               inputs=dict(heirloom=a.heirloom, live=a.live, ground=a.ground, fork=a.fork),
               camps=[], shards={})
    plans = {}
    for sh in ("h", "l"):
        other = "l" if sh == "h" else "h"
        units, notes = plan_shard(sh, infos[sh], infos[other], camps, svs[sh], svs[other], a.ground, fork_units[sh], rnd)
        plans[sh] = (units, notes)
    for sh in ("h", "l"):
        other = other_name(sh)
        ou, on = owner_units(sh, dbs[sh], infos[sh], infos[other], camps)
        pu, pn = propagate_units(dbs[sh], infos[sh], [(k, g, r) for s2, k, g, r in PROPAGATE if s2 == sh])
        units, notes = plans[sh]
        owned = {op[1].get("id") for u in ou + pu for op in u["ops"] if op[0] == "npcs"}
        units[:] = [u for u in units if not any(op[0] == "npcs" and op[1].get("id") in owned for op in u["ops"])]
        units += ou + pu
        for k, n in on + pn:
            notes[k].append(n)
        doc["shards"][sh] = units
    for c in camps:
        e = dict(key=c["key"], name=c["name"], world=c["world"], x=round(c["x"], 1), z=round(c["z"], 1),
                 kinds=c["kinds_text"], guide=c["guide"], faq=c.get("faq", []), source=c["source"], per_shard={})
        for sh in ("h", "l"):
            s, info = svs[sh][c["key"]], infos[sh]
            units = [u for u in plans[sh][0] if u["camp"] == c["key"]]
            ids = {r["id"] for r in s["members"]}
            gone = {u["ops"][0][1]["id"] for u in units if u["ops"][0][0] == "npcs" and u["ops"][0][3] is None
                    and u["ops"][0][1]["id"] in ids}
            after = len(s["members"]) - len(gone) + sum(1 for u in units if u["what"] == "add")
            e["per_shard"][sh] = dict(before=len(s["members"]), after=after, now=counts(s["members"]),
                                      behaviour=behaviour(info, s["members"]),
                                      changes=[f"{u['what']}: {u['why']}" for u in units],
                                      notes=plans[sh][1].get(c["key"], []))
        e["fork"] = fork_lines.get(c["key"], [])
        doc["camps"].append(e)
    return doc


def report(doc, out=sys.stdout):
    tot = {sh: collections.Counter(u["what"] for u in units) for sh, units in doc["shards"].items()}
    names = {"h": "Heirloom", "l": "Live"}
    for e in doc["camps"]:
        band = f", guide {e['guide'][0]}-{e['guide'][1]}" if e["guide"] else ""
        print(f"\n== {e['key']}: {e['name']} (world {e['world']}, {e['x']:.0f}, {e['z']:.0f}{band}) [{e['source']}]", file=out)
        if e["faq"]:
            print(f"   FAQ: {'; '.join(e['faq'])}", file=out)
        print(f"   kinds: {e['kinds']}", file=out)
        for sh, p in e["per_shard"].items():
            print(f"   {names[sh]}: {p['before']} -> {p['after']}: {p['now']}", file=out)
            print(f"      {p['behaviour']}", file=out)
            for ch in p["changes"]:
                print(f"      * {ch}", file=out)
            for n in p["notes"]:
                print(f"      - {n}", file=out)
        for f in e["fork"]:
            print(f"   fork: {f}", file=out)
    print("\nTOTALS", file=out)
    for sh, c in tot.items():
        print(f"  {names[sh]}: " + ", ".join(f"{k} {v}" for k, v in sorted(c.items())), file=out)


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--heirloom")
    ap.add_argument("--live")
    ap.add_argument("--ground")
    ap.add_argument("--fork")
    ap.add_argument("--write-plan")
    ap.add_argument("--report", action="store_true")
    ap.add_argument("--db")
    ap.add_argument("--plan")
    ap.add_argument("--shard", choices=("h", "l"))
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    g.add_argument("--hash", action="store_true")
    a = ap.parse_args(argv)
    if a.write_plan or (a.report and a.heirloom):
        doc = make_plan(a)
        if a.write_plan:
            json.dump(doc, open(a.write_plan, "w"), indent=1, default=list)
        if a.report:
            report(doc)
        return 0
    if a.report and a.plan:
        report(json.load(open(a.plan)))
        return 0
    if not a.db:
        ap.error("--db (with --plan, --undo or --hash), or --heirloom/--live/--ground with --write-plan")
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    if a.hash:
        for t in ("npcs", "npcrespawn", "npcwander"):
            print(t, digest(db, t))
        return 0
    if a.undo:
        db.execute("BEGIN IMMEDIATE")
        try:
            back, left = undo(db)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"put back {back} rows, {left} left (changed since)" if back or left else "nothing to undo")
        return 0 if left == 0 else 1
    if not a.plan:
        ap.error("--plan")
    doc = json.load(open(a.plan))
    sh = a.shard or ("h" if tr.table(db, "heirloom_seed_log") else "l")
    units = doc["shards"][sh]
    print(f"shard {sh} ({'Heirloom' if sh == 'h' else 'Live'}): {len(units)} units planned "
          f"({', '.join(f'{k} {v}' for k, v in sorted(collections.Counter(u['what'] for u in units).items()))})")
    db.execute("BEGIN IMMEDIATE")
    try:
        count, left = apply_units(db, units)
        if a.apply:
            db.execute("COMMIT")
        else:
            db.execute("ROLLBACK")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    for line in left:
        print(line)
    verb = "applied" if a.apply else "report only (--apply writes)"
    print(f"{verb}: {count['todo']} units written ({count['rows']} rows), {count['done']} done already, "
          f"{count['as-planned']} as planned already, {count['conflict']} left (changed since)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
