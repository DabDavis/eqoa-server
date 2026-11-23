#!/usr/bin/env python3
"""Wktaan's 4th Talon's orc camps populated as the guides have them (the owner, 2026-11-23: "Wktaan orc camps are not
populated correctly"; owner-asked spawn work).

THE SOURCES
  EQOA-Data (github.com/Jadiction/EQOA-Data, Map/zones.json) marks SIX camps named "Wktaan Orcs" in the zone (map
  points, no note, no level range), along its south from west to east. Through the map's fit to the game
  (tools/gen_camp_teles.py's fits(): game x = 2.1316 * map x - 3640.1, z = 2.1310 * map y + 3789.7, a median of 8.4
  units off over 594 points) they are W1-W6 below.
  The GameFAQs FAQ (Jarrod Garripoli's EQOA Frontiers FAQ, 48152, "Camps", LEVELS 31-40) and MasterVG782's guide
  (GameFAQs) list one camp, "WKTAANS: Warchief Wktaan, Wktaan Crafter, Wktaan Prophet, Wktaan Reaver -- Wktaan's 4th
  Talon (SW)"; no count, no loot. The zone is CLIENT's Tunaria cell c8 r10 (x 16000-18000, z 20000-22000,
  claude-notes/tunaria-map.md); W6 lies just over its south edge, where EQOA-Data draws it. The weres' and
  Harbingers' guides name the zone's SW (35-40 ratweres) and NE (Khalimet's portal) and no orcs.
  packets.db (33 Freeport townsfolk) holds nothing of the zone.

WHAT THE SERVER HAD (both shards alike, rows of the content import, ids 107153-107222)
  W1 (16440, 21697) the Warchief's camp: 22 Wktaan rows (Warchief Wktaan 42, Wktaan crafter 40, prophets, raiders,
     reavers 38-40) on the hilltop at y 112 and the slopes west and south of it.
  W2 (16653, 21857) 3 Wktaan rows (two prophets, a raider).
  W3 (16981, 21825), W4 (17365, 21716), W5 (17906, 21690), W6 (17889, 22174): 4, 9, 4 and 13 rows named "Deathfist
     prophet" and "Deathfist reaver", L38-40. Every column of theirs but the name, place, facing, level and zone is the
     Wktaan prophet's and reaver's (bodies 832107953 and 939332601, gear, flags), they are the import's own block
     among the Wktaan rows, and they stand on EQOA-Data's Wktaan points (W4's were spread round its point by
     cmd/wildspread -ranks on 2026-11-15 from a rank at y 75, each shard its own way:
     LIVE_PLACES); the Deathfist kinds of those levels are the Deathfist
     Citadel's (the FAQ's DFC camp: champion, cleric, prophet, reaver, slayer) and the Deathfist Horde's, a zone east.
     So five of the six camps were Wktaan camps under the Citadel's names: their loot (the desert's trash), template
     (the Citadel's priest for a prophet) and faction hits (the Deathfist Hordes -10) were the Citadel's.

WHAT THIS DOES
  rename  the 30 rows at W3-W6: "Deathfist prophet" -> "Wktaan prophet", "Deathfist reaver" -> "Wktaan reaver" (the
          FAQ's kinds; the name alone changes, so each takes the Wktaan kind's class, template, loot and faction rules,
          all by name). Their place, level, respawn and wander rows stay.
  add     the thin camps (W2, W3, W5: 3-4 orcs) topped up to MIN_CAMP (6, xp_camps_20261118's rule for a guide camp):
          7 rows, each a copy of a row of its kind in the camp, or the nearest Wktaan row of its kind (every npcs column
          but id, place and facing: body, gear, level, flags), with that row's npcrespawn row, standing (no npcwander
          row), facing the camp's middle. Their spots (ADDS) were found with cmd/campground (--find-spots, below): on
          the terrain (as every row of these camps) within REACH_DY of the height of the camp's row nearest its middle,
          not roofed, not embedded, dry, no wall between it and that row, SPACING from every NPC of either shard, the
          nearest such to the camp's middle (the same spots on both shards).
  Left: the five "Deathfist" rows at (16985, 21195) (107179-107183, y 104), in the zone's middle where no source
  marks a camp; the Warchief stays one; no paths or wanderers (no source gives any).

Each shard's rows are checked as read before anything is written (every row's id, name, level and place); a row not so
is a refusal and nothing is written. --apply in one transaction, a gm_audit line a row, log wktaan_camps_20261123_log
(and _meta: the npcs sequence before); a second --apply writes nothing. --undo puts every logged row back newest
first and the npcs sequence as it was (a row changed since is left and said, and the log kept): npcs, npcrespawn
and npcwander come back byte for byte (--hash prints their digests). NPC rows are read at the server's start: run it
in a keep restart's before.sh, once per shard.

    python3 -E -s tools/wktaan_camps_20261123.py --db eqoa.db --shard heirloom|live [--apply | --undo | --hash]
    python3 -E -s tools/wktaan_camps_20261123.py --find-spots --db H.db --db L.db --ground worlds/ground
"""
import argparse
import contextlib
import hashlib
import json
import math
import os
import sqlite3
import subprocess
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

TOOL = "tools/wktaan_camps_20261123.py"
LOG, META = "wktaan_camps_20261123_log", "wktaan_camps_20261123_meta"
DETAIL = "Wktaan's 4th Talon's orc camps as EQOA-Data's map and the FAQ have them (the owner, 2026-11-23): " + TOOL
WORLD = 0
PLACE = 1.0          # a row read is where this tool read it within this (x and z)
MIN_CAMP = 6
SPACING = 7.0
RADIUS = 30.0        # a new spawn within this of its camp's middle
REACH_DY = 4.0
GRID = 1.5

PROPHET, REAVER, RAIDER = "Wktaan prophet", "Wktaan reaver", "Wktaan raider"
RENAMES = {"Deathfist prophet": PROPHET, "Deathfist reaver": REAVER}

# EQOA-Data's six "Wktaan Orcs" points, west to east: (map x, map y), the game's (x, z) by the map's fit
CAMPS = {
    "W1": dict(map=(9420, 8403), at=(16440, 21697), what="the Warchief's camp"),
    "W2": dict(map=(9520, 8478), at=(16653, 21857), what="below the Warchief's hill, southeast"),
    "W3": dict(map=(9674, 8463), at=(16981, 21825), what="the south slope"),
    "W4": dict(map=(9854, 8412), at=(17365, 21716), what="the ridge"),
    "W5": dict(map=(10108, 8400), at=(17906, 21690), what="the east"),
    "W6": dict(map=(10100, 8627), at=(17889, 22174), what="the southeast, over the zone's edge"),
}

# every row of the camps as read on both shards' databases of 2026-11-23 01:53/01:55 UTC: id: (camp, name, level, x, z);
# Heirloom's places (Live's differ for W4 alone: LIVE_PLACES)
ROWS = {
    107184: ("W1", "Wktaan raider", 39, 16488.9, 21683.8), 107185: ("W1", "Wktaan reaver", 38, 16451.7, 21680.0),
    107186: ("W1", "Wktaan crafter", 40, 16443.5, 21697.6), 107187: ("W1", "Warchief Wktaan", 42, 16411.5, 21719.7),
    107188: ("W1", "Wktaan prophet", 39, 16438.6, 21675.1), 107205: ("W1", "Wktaan raider", 38, 16477.9, 21669.9),
    107206: ("W1", "Wktaan raider", 40, 16414.7, 21721.1), 107207: ("W1", "Wktaan raider", 39, 16410.1, 21714.7),
    107208: ("W1", "Wktaan prophet", 38, 16472.7, 21706.8), 107209: ("W1", "Wktaan prophet", 39, 16475.3, 21700.0),
    107210: ("W1", "Wktaan reaver", 40, 16410.8, 21680.2), 107212: ("W1", "Wktaan prophet", 39, 16418.8, 21677.9),
    107213: ("W1", "Wktaan raider", 38, 16410.3, 21703.1), 107214: ("W1", "Wktaan raider", 39, 16426.3, 21720.7),
    107215: ("W1", "Wktaan prophet", 40, 16447.2, 21731.9), 107216: ("W1", "Wktaan reaver", 39, 16438.3, 21732.1),
    107217: ("W1", "Wktaan prophet", 39, 16260.1, 21755.0), 107218: ("W1", "Wktaan prophet", 40, 16254.7, 21741.0),
    107219: ("W1", "Wktaan reaver", 38, 16267.1, 21747.4), 107220: ("W1", "Wktaan prophet", 39, 16314.3, 21512.9),
    107221: ("W1", "Wktaan reaver", 40, 16328.4, 21515.2), 107222: ("W1", "Wktaan raider", 39, 16494.4, 21578.0),
    107202: ("W2", "Wktaan prophet", 40, 16650.0, 21839.6), 107203: ("W2", "Wktaan prophet", 38, 16651.5, 21855.2),
    107204: ("W2", "Wktaan raider", 38, 16640.2, 21776.0),
    107189: ("W3", "Deathfist prophet", 39, 16964.0, 21827.9), 107190: ("W3", "Deathfist prophet", 38, 16957.1, 21824.0),
    107191: ("W3", "Deathfist reaver", 39, 16972.2, 21833.6), 107192: ("W3", "Deathfist reaver", 40, 16972.0, 21816.9),
    107166: ("W4", "Deathfist reaver", 40, 17228.1, 21746.0), 107167: ("W4", "Deathfist reaver", 38, 17473.5, 21659.3),
    107168: ("W4", "Deathfist reaver", 40, 17370.4, 21841.9), 107169: ("W4", "Deathfist prophet", 39, 17322.5, 21553.6),
    107170: ("W4", "Deathfist prophet", 39, 17400.3, 21602.6), 107171: ("W4", "Deathfist prophet", 38, 17443.8, 21775.6),
    107172: ("W4", "Deathfist reaver", 39, 17259.7, 21654.8), 107173: ("W4", "Deathfist reaver", 40, 17280.6, 21848.0),
    107174: ("W4", "Deathfist reaver", 39, 17352.8, 21732.3),
    107175: ("W5", "Deathfist reaver", 40, 17908.6, 21702.8), 107176: ("W5", "Deathfist prophet", 39, 17898.9, 21689.5),
    107177: ("W5", "Deathfist prophet", 38, 17912.5, 21688.8), 107178: ("W5", "Deathfist reaver", 40, 17905.4, 21688.1),
    107153: ("W6", "Deathfist reaver", 40, 17825.1, 22275.2), 107154: ("W6", "Deathfist prophet", 39, 17804.5, 22297.4),
    107155: ("W6", "Deathfist reaver", 40, 17812.8, 22288.3), 107156: ("W6", "Deathfist prophet", 39, 17917.1, 22179.4),
    107157: ("W6", "Deathfist prophet", 38, 17922.9, 22180.8), 107158: ("W6", "Deathfist reaver", 39, 17847.8, 22182.7),
    107159: ("W6", "Deathfist prophet", 39, 17899.4, 22144.4), 107160: ("W6", "Deathfist reaver", 38, 17895.1, 22140.0),
    107161: ("W6", "Deathfist reaver", 40, 17855.2, 22144.9), 107162: ("W6", "Deathfist prophet", 39, 17860.8, 22148.6),
    107163: ("W6", "Deathfist reaver", 40, 17926.0, 22156.7), 107164: ("W6", "Deathfist reaver", 39, 17876.8, 22217.2),
    107165: ("W6", "Deathfist prophet", 40, 17872.6, 22125.8),
}

# Live's own places for W4's rows (each shard's wildspread run spread that rank on its own): id: (x, z)
LIVE_PLACES = {
    107166: (17222.1, 21742.0), 107167: (17469.2, 21644.1), 107168: (17377.6, 21841.2), 107169: (17328.7, 21543.3),
    107170: (17392.9, 21604.0), 107171: (17445.8, 21783.0), 107172: (17240.7, 21658.5), 107173: (17282.7, 21843.0),
    107174: (17353.0, 21724.8),
}

# the adds: (camp, kind, the row copied, x, y, z, facing), found by --find-spots on both shards' copies and the ground
ADDS = [
    ('W2', 'Wktaan reaver', 107216, 16645.7, 65.44, 21825.1, 96),
    ('W2', 'Wktaan reaver', 107216, 16642.7, 65.44, 21817.6, 26),
    ('W2', 'Wktaan raider', 107204, 16638.2, 65.44, 21823.6, 64),
    ('W3', 'Wktaan prophet', 107189, 16970.8, 58.06, 21825.6, 192),
    ('W3', 'Wktaan reaver', 107191, 16964.8, 58.06, 21819.6, 10),
    ('W5', 'Wktaan prophet', 107177, 17906.3, 66.06, 21695.3, 127),
    ('W5', 'Wktaan reaver', 107178, 17913.8, 66.06, 21696.8, 170),
]


def low(s):
    return (s or "").strip().lower()


def facing_to(x, z, tx, tz):
    """The facing byte of a look from (x, z) toward (tx, tz) (CLIENT's heading atan2(dx, dz), entity.FacingByte)."""
    h = math.atan2(tx - x, tz - z)
    if h < 0:
        h += 2 * math.pi
    return int(round(h / (2 * math.pi) * 256)) % 256


def middle(camp):
    """A camp's middle: its rows' mean (W1's the Warchief's hilltop, its outliers on the slopes left out)."""
    ids = [i for i, r in ROWS.items() if r[0] == camp and (camp != "W1" or math.hypot(r[3] - 16440, r[4] - 21700) < 80)]
    return (sum(ROWS[i][3] for i in ids) / len(ids), sum(ROWS[i][4] for i in ids) / len(ids))


def kind_after(name):
    return RENAMES.get(name, name)


def wanted_kinds(camp):
    """The kinds a thin camp gets, to MIN_CAMP: each the FAQ's kind (prophet, reaver; a raider where the camp has one)
    the camp has fewest of, prophets first on a tie."""
    have = {}
    for r in ROWS.values():
        if r[0] == camp:
            k = kind_after(r[1])
            have[k] = have.get(k, 0) + 1
    kinds = [PROPHET, REAVER] + ([RAIDER] if have.get(RAIDER) else [])
    out = []
    for _ in range(MIN_CAMP - sum(have.values())):
        k = min(kinds, key=lambda k: (have.get(k, 0), kinds.index(k)))
        have[k] = have.get(k, 0) + 1
        out.append(k)
    return out


def template_for(camp, kind, x, z):
    """The row a new one copies: the nearest of its kind (after the renames) in its camp, else the nearest Wktaan row
    of the kind."""
    mine = [i for i, r in ROWS.items() if r[0] == camp and kind_after(r[1]) == kind]
    pool = mine or [i for i, r in ROWS.items() if kind_after(r[1]) == kind]
    return min(pool, key=lambda i: (math.hypot(ROWS[i][3] - x, ROWS[i][4] - z), i))


# ---- reading and checking ---------------------------------------------------------------------------------------

def connect(path, ro=False):
    db = sqlite3.connect(f"file:{path}?mode=ro" if ro else path, uri=ro, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 5000")
    return db


def applied(db):
    return tr.table(db, LOG) and db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0] > 0


def check(db, shard):
    """Refusals: every row of ROWS as read on the shard (a renamed one may be its new name only when this tool's log
    has it)."""
    done = applied(db)
    out = []
    for i, (camp, name, level, x, z) in sorted(ROWS.items()):
        if shard == "live" and i in LIVE_PLACES:
            x, z = LIVE_PLACES[i]
        r = tr.get(db, "npcs", {"id": i})
        if r is None:
            out.append(f"{camp} row {i} ({name}) is not there")
            continue
        names = {name} | ({kind_after(name)} if done else set())
        if r["npc_name"] not in names or r["npc_level"] != level or r["world"] != WORLD \
                or abs(r["x_coord"] - x) > PLACE or abs(r["z_coord"] - z) > PLACE:
            out.append(f"{camp} row {i} is {r['npc_name']} L{r['npc_level']} at ({r['x_coord']:.1f}, {r['z_coord']:.1f}) "
                       f"in world {r['world']}, read as {name} L{level} at ({x}, {z})")
    return out


def changes(db):
    """[(tbl, key, after)]: the renames, then each add's npcs row (key id NEW) and its npcrespawn row."""
    out = []
    for i, (camp, name, level, x, z) in sorted(ROWS.items()):
        if name in RENAMES:
            r = tr.get(db, "npcs", {"id": i})
            out.append(("npcs", {"id": i}, dict(r, npc_name=RENAMES[name])))
    for camp, kind, tid, x, y, z, facing in ADDS:
        t = tr.get(db, "npcs", {"id": tid})
        new = {k: v for k, v in t.items() if k != "id"}
        new.update(npc_name=kind, x_coord=x, y_coord=y, z_coord=z, facing=facing, dt_kill=None)
        out.append(("npcs", {"id": "NEW"}, new))
        rs = tr.get(db, "npcrespawn", {"npc_row": tid})
        if rs:
            out.append(("npcrespawn", {"npc_row": "NEW"}, dict(rs, npc_row="NEW")))
    return out


# ---- apply and undo ---------------------------------------------------------------------------------------------

def log_ddl():
    return (f"CREATE TABLE IF NOT EXISTS {LOG} (seq INTEGER PRIMARY KEY AUTOINCREMENT, tbl TEXT NOT NULL, rowkey TEXT NOT NULL,"
            f" before TEXT, after TEXT, audit INTEGER NOT NULL DEFAULT 0)",
            f"CREATE TABLE IF NOT EXISTS {META} (key TEXT PRIMARY KEY, value TEXT)")


def apply(db, shard):
    for ddl in log_ddl():
        db.execute(ddl)
    r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'npcs'").fetchone()
    db.execute(f"INSERT INTO {META} VALUES ('seq_before', ?)", (json.dumps(None if r is None else r[0]),))
    db.execute(f"INSERT INTO {META} VALUES ('shard', ?)", (shard,))
    n = 0
    new_id = None
    for tbl, key, after in changes(db):
        if "NEW" in key.values():
            if tbl == "npcs":
                cols = list(after)
                new_id = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                                    [after[c] for c in cols]).lastrowid
                key = {"id": new_id}
                res = ("insert", None, tr.get(db, "npcs", key))
            else:
                key = {k: new_id for k in key}
                res = tr.write(db, tbl, key, {k: (new_id if v == "NEW" else v) for k, v in after.items()})
        else:
            res = tr.write(db, tbl, key, after)
        if res is None:
            continue
        action, before, now = res
        a = tr.audit(db, action, tbl, key, before, now, DETAIL + f" ({shard})")
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                   (tbl, json.dumps(key, sort_keys=True), None if before is None else json.dumps(before, sort_keys=True),
                    None if now is None else json.dumps(now, sort_keys=True), a))
        n += 1
    return n


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
    seq = db.execute("SELECT seq FROM sqlite_sequence WHERE name = ?", (table,)).fetchone() \
        if tr.table(db, "sqlite_sequence") else None
    return h.hexdigest() + (f" seq {seq[0]}" if seq else "")


# ---- the report -------------------------------------------------------------------------------------------------

def report(db):
    for camp, c in CAMPS.items():
        rows = [(i, r) for i, r in ROWS.items() if r[0] == camp]
        now = {}
        for i, r in rows:
            k = kind_after(r[1])
            now[k] = now.get(k, 0) + 1
        adds = [a for a in ADDS if a[0] == camp]
        for a in adds:
            now[a[1]] = now.get(a[1], 0) + 1
        lv = [r[2] for _, r in rows]
        renamed = sum(1 for _, r in rows if r[1] in RENAMES)
        print(f"{camp} map {c['map']} -> game {c['at']}, {c['what']}: {len(rows)} rows (L{min(lv)}-{max(lv)})"
              + (f", {renamed} renamed" if renamed else "") + (f", {len(adds)} added" if adds else "")
              + f" -> {len(rows) + len(adds)}: " + ", ".join(f"{k} x{n}" for k, n in sorted(now.items())))
        for camp_, kind, tid, x, y, z, f in adds:
            print(f"    add {kind} (copy of {tid}, L{ROWS[tid][2]}) at ({x}, {y}, {z}) facing {f}")


# ---- finding the adds' spots (a dev step: its output is ADDS) ---------------------------------------------------

def campground(ground, qs):
    here = os.path.dirname(os.path.abspath(__file__))
    exe = os.environ.get("CAMPGROUND")
    cmd = [exe] if exe else ["go", "run", "./cmd/campground"]
    p = subprocess.run(cmd + ["-ground", ground], input="".join(json.dumps(q) + "\n" for q in qs),
                       capture_output=True, text=True, cwd=os.path.join(here, ".."), check=True)
    return {json.loads(l)["k"]: json.loads(l) for l in p.stdout.splitlines() if l.strip()}


def find_spots(paths, ground):
    others = []
    for p in paths:
        with contextlib.closing(connect(p, ro=True)) as db:
            others += db.execute("SELECT x_coord, z_coord FROM npcs WHERE world = ? AND x_coord BETWEEN 16000 AND 18200 "
                               "AND z_coord BETWEEN 21300 AND 22500", (WORLD,)).fetchall()
    out = []
    for camp in CAMPS:
        want = wanted_kinds(camp)
        if not want:
            continue
        mx, mz = middle(camp)
        for kind in want:
            tid = template_for(camp, kind, mx, mz)
            # the height and the wall test are the camp's own: its row nearest the middle (a template may stand elsewhere)
            near = min((i for i, r in ROWS.items() if r[0] == camp), key=lambda i: (math.hypot(ROWS[i][3] - mx, ROWS[i][4] - mz), i))
            with contextlib.closing(connect(paths[0], ro=True)) as db:
                t = db.execute("SELECT x_coord, y_coord, z_coord FROM npcs WHERE id = ?", (near,)).fetchone()
            qs = []
            n = int(RADIUS / GRID)
            for i in range(-n, n + 1):
                for j in range(-n, n + 1):
                    x, z = round(mx + i * GRID, 1), round(mz + j * GRID, 1)
                    if math.hypot(x - mx, z - mz) <= RADIUS:
                        qs.append(dict(op="spot", k=len(qs), w=WORLD, x=x, z=z, hint=t[1], fx=t[0], fy=t[1], fz=t[2]))
            ans = campground(ground, qs)
            best = None
            for q in qs:
                a = ans.get(q["k"])
                if not a or a.get("stand") is None or a.get("roofed") or a.get("embedded") or a.get("walled") \
                        or a.get("env", 0) & 15 or abs(a["stand"] - t[1]) > REACH_DY \
                        or a.get("placed"):
                    continue
                d = min(math.hypot(q["x"] - ox, q["z"] - oz) for ox, oz in others)
                if d < SPACING:
                    continue
                score = (math.hypot(q["x"] - mx, q["z"] - mz), q["x"], q["z"])
                if best is None or score < best[0]:
                    best = (score, q["x"], round(a["stand"], 2), q["z"])
            if best is None:
                print(f"# {camp}: no spot for a {kind}", file=sys.stderr)
                continue
            _, x, y, z = best
            others.append((x, z))
            out.append((camp, kind, tid, x, y, z, facing_to(x, z, mx, mz)))
    print("ADDS = [")
    for a in out:
        print(f"    {a!r},")
    print("]")
    # the adds checked as spawns, as the server sets them down
    ans = campground(ground, [dict(op="check", k=i, w=WORLD, x=a[3], y=a[4], z=a[5]) for i, a in enumerate(out)])
    for i, a in enumerate(out):
        c = ans[i]
        print(f"# {a[0]} {a[1]}: stand {c.get('stand')} floor {c.get('floor')} embedded {c.get('embedded', False)} "
              f"inground {c.get('inground', False)} env {c.get('env')}")


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", action="append", required=True)
    ap.add_argument("--shard", choices=("heirloom", "live"))
    ap.add_argument("--ground")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    g.add_argument("--hash", action="store_true")
    g.add_argument("--find-spots", action="store_true")
    a = ap.parse_args(argv)
    if a.find_spots:
        if not a.ground:
            ap.error("--find-spots needs --ground")
        find_spots(a.db, a.ground)
        return 0
    if len(a.db) != 1:
        ap.error("one --db")
    if not a.hash and not a.shard:
        ap.error("--shard heirloom|live")
    db = connect(a.db[0], ro=not (a.apply or a.undo))
    try:
        return act(a, db)
    finally:
        db.close()


def act(a, db):
    if a.hash:
        for t in ("npcs", "npcrespawn", "npcwander"):
            print(t, digest(db, t))
        return 0
    if a.undo:
        db.execute("BEGIN IMMEDIATE")
        back, left = undo(db)
        db.execute("COMMIT")
        print(f"undone: {back} rows put back" + (f", {left} left (changed since): the log is kept" if left else ""))
        return 0 if left == 0 else 1
    bad = check(db, a.shard)
    if bad:
        print("refused: the camps are not as this tool read them (nothing written):")
        for b in bad:
            print("  " + b)
        return 2
    report(db)
    if applied(db):
        print(f"already applied on this database ({LOG} has its rows): nothing written")
        return 0
    if not a.apply:
        print(f"dry run: {len(changes(db))} rows would be written; --apply to write them")
        return 0
    db.execute("BEGIN IMMEDIATE")
    try:
        bad = check(db, a.shard)
        if bad:
            raise SystemExit("refused in the transaction: " + "; ".join(bad))
        n = apply(db, a.shard)
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"applied on {a.shard}: {n} rows, in gm_audit and {LOG}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
