#!/usr/bin/env python3
"""Every race's monks start at the western humans' monastery, get along with Qeynos, and are welcomed there.

The owner, 2026-11-17: "start all monk races chars at human west monastery and fix their factions so they can get a
long with qeynos", then "fix the existing monks factions too" and "also flavor text towards the other races monks in a
welcoming manner, like you have embraced the ways of peace, etc.". Three steps, each named on the command line:

--starts  (defaultcharacter, factionstart)
    The monastery is the western human monk's own start row (defaultcharacter race 0, humType 2, class 4: Master
    T`an Chen's hall east of Qeynos, its people Citizens of Qeynos). Every other race's monk row (the game's races,
    the humans of the east, whose row the froglok and a human of no city take too, and every race of racedefs that has
    rows of its own: their homelands' monks) is put there, world, place and facing; a race with no monk row gets one,
    its stats its own row's of a class the western humans have too plus the western monk's over that class's
    (the rows are 60 plus the race's and the class's bonuses: a race keeps its bonuses), its level, money and
    training points that row's. A new character is bound where it starts (database.CreateNew), so its bind is the
    monastery too; its spiritmaster's name stays its race's city (database.HomeCity: only logs and the bind line read
    it). Its stats and starting items are untouched (startitems is by race and class, not place).
    Each of those races' monks starts at least at FLOOR with the Qeynos factions (QEYNOS): the western human monk's
    own start (the most specific factionstart row, else the faction's own), never under 0 (neutral), or the race's own
    where that is higher: a row (race, 4, -1) (a human of the east: (0, 4, 1)) is written where the race's start is
    lower. The race's other standings are its own. Then no monk is KOS (below -350) to any faction of an NPC in Qeynos,
    the monastery or by Qeynos's coach (AREA), which is checked and said.

--existing  (charfactions)
    Every monk character but the western humans' (they started there) gets its standing with each QEYNOS faction
    raised to the floor where it is below it: its own row (charfactions) raised, or one written where it has none and
    its start (factionstart as it was, the rows --starts writes left out) is below. Only raised, never lowered; no
    other faction and no place touched.

--dialog  (npcdialog)
    LINES: Qeynos's people and guards (town Qeynos, which takes the monastery's: they are Citizens of Qeynos) welcome
    a monk of any race but the humans (npcdialog.class 4, not_race 0: the server's monk-start branch reads them; an
    older server reads neither column and would say the lines to everyone, so --apply --dialog wants --server-binary,
    a binary that reads them).

Every row written is a row edit as the GM panel writes one (gm_audit, via "tool": listed and revertible there) and is
logged in monk_start_log by run and step; --undo [--step S ...] puts each row back exactly as it was (one changed since
is left, and said). A report by default; --apply and --undo copy the database first (<db>.bak-<when>-pre-monkstart)
unless --no-backup.

The server reads factionstart and npcfaction at its start, a character's charfactions at its login and writes them over
at its logout (from memory), and npcdialog within 30 s: so stop the server for --apply and --undo (a keep restart's
before.sh), all three steps; defaultcharacter alone is read at each creation.

    python3 -E -s tools/monk_start.py --db eqoa.db                                         report
    python3 -E -s tools/monk_start.py --db eqoa.db --apply --starts --existing --dialog --server-binary eqoa-server
    python3 -E -s tools/monk_start.py --db eqoa.db --undo [--step starts|existing|dialog ...]
"""
import argparse
import datetime
import json
import math
import os
import shutil
import sqlite3
import sys

MONK = 4
RACE_FROGLOK = 10
QEYNOS = ["Citizens of Qeynos", "The Qeynos Guard", "Qeynos Troupe", "Order of the Silent Fist"]
FLOOR_MIN = 0           # never under neutral
KOS_BELOW = -350        # entity.FactionKOSBelow
# Qeynos (its centre), the monastery, its coach: an NPC within these is one a new monk meets
AREA = [(4400.0, 17200.0, 1000.0), (5593.7, 17626.6, 300.0), (4813.0, 17144.0, 100.0)]
WORLD = 0
SOURCE = "monk-start"
WHO, VIA = "dab's sessions", "tool"
DETAIL = "the owner 2026-11-17: every race's monks start at the western monastery, get along with Qeynos; tools/monk_start.py"
LOG = "monk_start_log"
LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    run     INTEGER NOT NULL,
    step    TEXT NOT NULL,      -- starts | existing | dialog
    tbl     TEXT NOT NULL,      -- defaultcharacter, factionstart, charfactions, npcdialog
    ref     TEXT NOT NULL,      -- the row's key (JSON)
    before  TEXT,               -- the row before (JSON); NULL: made here
    after   TEXT,               -- the row as written (JSON)
    audit   INTEGER             -- the gm_audit line written
)"""
# database.GMAuditSchema, as the server makes it
AUDIT_SCHEMA = """CREATE TABLE IF NOT EXISTS gm_audit (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	at TEXT NOT NULL, accountid INTEGER NOT NULL, username TEXT NOT NULL,
	via TEXT NOT NULL, ip TEXT NOT NULL DEFAULT '', action TEXT NOT NULL,
	tbl TEXT NOT NULL DEFAULT '', rowkey TEXT NOT NULL DEFAULT '',
	before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER
)"""
KEYS = {"defaultcharacter": ("defaultCharacterID",), "factionstart": ("id",), "charfactions": ("serverid", "faction"),
        "npcdialog": ("id",)}
# (role, line, weight): %b brother or sister, %r the listener's race, %p its name (entity.DialogSay)
LINES = [
    ("", "Welcome, %b. You have set aside the old ways of your people and embraced the ways of peace. "
         "The Silent Fist knows no race.", 3),
    ("", "Few of your kind walk the path of the open hand, %p. Whatever road brought you here, the monastery is your "
         "home now.", 3),
    ("", "A %r who seeks stillness of mind and body. Train hard, %b, and Qeynos will be glad to call you its own.", 3),
    ("", "Blood and birth matter little on the path, %p. The discipline of the fist asks the same of every soul who "
         "walks it.", 3),
    ("guard", "Walk in peace, %r. A student of the monastery is a friend of the Qeynos Guard.", 3),
    ("guard", "The monks of the Silent Fist vouch for you, %p. You'll have no trouble from the Guard while you keep "
              "the peace.", 3),
]
DIALOG_COLUMNS = [("class", "INTEGER NOT NULL DEFAULT -1"), ("not_race", "INTEGER NOT NULL DEFAULT -1")]
BINARY_MARK = b"away, class, not_race FROM npcdialog"  # database.NPCDialog's query in a server that reads them
STATS = ["defaultStrength", "defaultStamina", "defaultAgility", "defaultDexterity", "defaultWisdom",
         "defaultIntelligence", "defaultCharisma"]
STEPS = ("starts", "existing", "dialog")


def go_json(m):
    """A row as Go's json.Marshal writes a map (keys sorted, <>& escaped), as the panel's audit lines are."""
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def rows(db, sql, args=()):
    cur = db.execute(sql, args)
    names = [c[0] for c in cur.description]
    return [dict(zip(names, r)) for r in cur.fetchall()]


def row_of(db, tbl, ref):
    r = rows(db, f"SELECT * FROM {tbl} WHERE " + " AND ".join(f"{c} = ?" for c in KEYS[tbl]),
             tuple(ref[c] for c in KEYS[tbl]))
    return r[0] if r else None


def has_column(db, tbl, col):
    return any(r[1] == col for r in db.execute(f"PRAGMA table_info({tbl})"))


# ---- the server's rules ------------------------------------------------------------------------------------------

def combo_of(race, cls, hum):
    """entity.comboOf: a froglok and a human of no west city as the east's, any other race's city 0."""
    if race == RACE_FROGLOK or (race == 0 and hum != 2):
        return (0, cls, 1)
    if race != 0:
        hum = 0
    return (race, cls, hum)


def specificity(r):
    named = [v >= 0 for v in r[:3]]
    return (sum(named), named[0], named[1], named[2])


def resolve(start_rows, base, combo):
    """entity.startsFor: the most specific row (race, class, humtype, standing) holding the combo, else base."""
    best = None
    for r in start_rows:
        if all(v < 0 or v == c for v, c in zip(r[:3], combo)) and (best is None or specificity(r) > specificity(best)):
            best = r
    return best[3] if best else base


def place_of(dc, race, hum, cls):
    """database.placeOf's query on defaultcharacter rows (own rows; a froglok as an eastern human)."""
    if race == RACE_FROGLOK:
        race, hum = 0, 1
    mine = [r for r in dc if r["race"] == race]
    if not mine:
        return None
    mine.sort(key=lambda r: (not (r["humType"] == hum), not (r["tclass"] == cls), r["humType"], r["tclass"]))
    return mine[0]


def npc_key(name):
    return (name or "").strip().lower().rstrip("$").strip()


class State:
    """What the database holds now."""

    def __init__(self, db):
        self.db = db
        self.dc = rows(db, "SELECT * FROM defaultcharacter ORDER BY race, humType, tclass")
        self.factions = dict(db.execute("SELECT name, standing FROM factions"))
        self.starts = {}
        self.starts_old = {}  # without the rows --starts wrote (factionstart as it was)
        ours = set()
        if db.execute(f"SELECT 1 FROM sqlite_master WHERE name = '{LOG}'").fetchone():
            for (ref,) in db.execute(f"SELECT ref FROM {LOG} WHERE tbl = 'factionstart'"):
                ours.add(json.loads(ref)["id"])
        for i, race, cls, hum, f, v in db.execute("SELECT id, race, class, humtype, faction, standing FROM factionstart"):
            self.starts.setdefault(f, []).append((race, cls, hum, v, i))
            if i not in ours:
                self.starts_old.setdefault(f, []).append((race, cls, hum, v, i))
        self.racedefs = {}
        if db.execute("SELECT 1 FROM sqlite_master WHERE name = 'racedefs'").fetchone():
            for i, wire, city, name in db.execute("SELECT id, wire_race, city, name FROM racedefs"):
                self.racedefs[i] = (wire, city, name)

    def start(self, f, combo, old=False):
        src = self.starts_old if old else self.starts
        return resolve(src.get(f, []), self.factions.get(f, 0), combo)

    def monastery(self):
        m = [r for r in self.dc if r["race"] == 0 and r["humType"] == 2 and r["tclass"] == MONK]
        if not m:
            raise SystemExit("no western human monk row in defaultcharacter: no monastery to start at")
        return m[0]

    def floors(self):
        west = (0, MONK, 2)
        return {f: max(FLOOR_MIN, self.start(f, west)) for f in QEYNOS}

    def monk_races(self):
        """(race, humType) of every monk start but the western humans': the eastern humans (and so the froglok and a
        human of no city), and every other race defaultcharacter has rows of."""
        out = [(0, 1)]
        for race in sorted({r["race"] for r in self.dc if r["race"] != 0}):
            hums = sorted({r["humType"] for r in self.dc if r["race"] == race})
            out.append((race, hums[0]))
        return out

    def race_name(self, race, hum=0):
        names = ["Human", "Elf", "Dark Elf", "Gnome", "Dwarf", "Troll", "Barbarian", "Halfling", "Erudite", "Ogre",
                 "Froglok"]
        if race == 0:
            return {1: "Human (east)", 2: "Human (west)"}.get(hum, "Human")
        if race < len(names):
            return names[race]
        return self.racedefs.get(race, (0, 0, f"race {race}"))[2]


# ---- the plans ----------------------------------------------------------------------------------------------------

def plan_starts(st):
    """[(tbl, ref or None, before or None, after)] for --starts, and the report's lines."""
    m = st.monastery()
    place = {"world": m["world"], "x": m["x"], "y": m["y"], "z": m["z"], "facing": m["facing"]}
    west = {r["tclass"]: r for r in st.dc if r["race"] == 0 and r["humType"] == 2}
    changes, lines = [], []
    lines.append(f"The monastery: world {m['world']} ({m['x']:.2f}, {m['y']:.2f}, {m['z']:.2f}) facing {m['facing']:.4f}"
                 f" (defaultcharacter {m['defaultCharacterID']}, the western human monk's row)")
    for race, hum in st.monk_races():
        own = [r for r in st.dc if r["race"] == race and r["humType"] == hum]
        monk = [r for r in own if r["tclass"] == MONK]
        name = st.race_name(race, hum)
        if monk:
            for r in monk:
                if all(r[k] == v for k, v in place.items()):
                    continue
                after = dict(r, **place)
                changes.append(("defaultcharacter", {"defaultCharacterID": r["defaultCharacterID"]}, r, after))
                lines.append(f"  {name} monk: moved from world {r['world']} ({r['x']:.1f}, {r['z']:.1f})")
            continue
        base = next((r for r in sorted(own, key=lambda r: r["tclass"]) if r["tclass"] in west), None)
        if base is None:
            lines.append(f"  {name}: no row of a class the western humans have; no monk row made (PROBLEM)")
            continue
        new = {k: v for k, v in base.items() if k != "defaultCharacterID"}
        new.update(place)
        new["tclass"] = MONK
        for s in STATS:
            new[s] = base[s] + west[MONK][s] - west[base["tclass"]][s]
        changes.append(("defaultcharacter", None, None, new))
        lines.append(f"  {name} monk: a row made (it started on its class {base['tclass']} row, world {base['world']} "
                     f"({base['x']:.1f}, {base['z']:.1f})); stats {[new[s] for s in STATS]}")
    floors = st.floors()
    lines.append("Qeynos floors (the western human monk's start, at least 0): " +
                 ", ".join(f"{f} {v}" for f, v in floors.items()))
    for race, hum in st.monk_races():
        combo = combo_of(race, MONK, hum)
        key = (race, MONK, -1) if race != 0 else (0, MONK, 1)
        raised = []
        for f in QEYNOS:
            cur = st.start(f, combo)
            if cur >= floors[f]:
                continue
            exact = [r for r in st.starts.get(f, []) if r[:3] == key]
            if exact:
                r = exact[0]
                before = row_of(st.db, "factionstart", {"id": r[4]})
                changes.append(("factionstart", {"id": r[4]}, before, dict(before, standing=floors[f])))
            else:
                changes.append(("factionstart", None, None, {"race": key[0], "class": key[1], "humtype": key[2],
                                "faction": f, "standing": floors[f], "note": DETAIL, "source": SOURCE}))
            raised.append(f"{f} {cur} -> {floors[f]}")
        if raised:
            lines.append(f"  {st.race_name(race, hum)} monk: " + "; ".join(raised))
    return changes, lines


def monk_chars(db):
    return rows(db, "SELECT serverid, charName AS charname, race, humType, level FROM characters WHERE tclass = ? "
                    "AND NOT (race = 0 AND humType = 2) ORDER BY race, charname", (MONK,))


def plan_existing(st):
    floors = st.floors()
    own = {}
    for sid, f, v in st.db.execute("SELECT serverid, faction, standing FROM charfactions WHERE faction IN (%s)"
                                   % ",".join("?" * len(QEYNOS)), QEYNOS):
        own[(sid, f)] = v
    changes, lines, chars = [], [], set()
    every = monk_chars(st.db)
    lines.append(f"Monks of other races than the western humans: {len(every)}")
    for c in every:
        combo = combo_of(c["race"], MONK, c["humType"])
        now, after = [], []
        for f in QEYNOS:
            if (c["serverid"], f) in own:
                v, kind = own[(c["serverid"], f)], "own"
            else:
                v, kind = st.start(f, combo, old=True), "start"
            now.append(f"{f} {v}{'' if kind == 'start' else '*'}")
            if v >= floors[f]:
                continue
            ref = {"serverid": c["serverid"], "faction": f}
            if kind == "own":
                before = row_of(st.db, "charfactions", ref)
                changes.append(("charfactions", ref, before, dict(before, standing=floors[f])))
            else:
                changes.append(("charfactions", ref, None, {"serverid": c["serverid"], "faction": f,
                                                            "standing": floors[f]}))
            after.append(f)
            chars.add(c["serverid"])
        lines.append(f"  {c['charname']} ({st.race_name(c['race'], c['humType'])}, level {c['level']}): " +
                     ", ".join(now) + (f"  -> raised: {', '.join(after)}" if after else ""))
    lines.append(f"  (* its own charfactions row; else its start)  {len(chars)} characters, {len(changes)} rows to write")
    return changes, lines


def plan_dialog(st):
    have = set()
    if has_column(st.db, "npcdialog", "class"):
        for (line,) in st.db.execute("SELECT line FROM npcdialog WHERE town = 'Qeynos' AND class = ? AND not_race = 0",
                                     (MONK,)):
            have.add(line)
    changes, lines = [], []
    for role, line, w in LINES:
        if line in have:
            continue
        changes.append(("npcdialog", None, None, {"town": "Qeynos", "role": role, "name": "", "mood": "", "race": -1,
                        "sex": "", "line": line, "weight": w, "source": SOURCE, "speaker_race": -1, "away": 0,
                        "class": MONK, "not_race": 0}))
        lines.append(f"  [{role or 'anyone'}] {line}")
    lines.insert(0, f"Qeynos's welcome to a monk of another race than the humans: {len(changes)} lines to add")
    return changes, lines


# ---- checks -------------------------------------------------------------------------------------------------------

def area_factions(db):
    """The factions of the NPCs a new monk meets (AREA), each with one of its members (npcfaction as the server
    matches it: a place's row, the smallest first, then a world's, then anywhere's)."""
    nf = {}
    for name, w, x, z, r, f in db.execute("SELECT name, world, x, z, radius, faction FROM npcfaction"):
        nf.setdefault(npc_key(name), []).append((w, x, z, r, f))
    out = {}
    for n, w, x, z in db.execute("SELECT npc_name, world, x_coord, z_coord FROM npcs WHERE world = ?", (WORLD,)):
        if not any(math.hypot(x - cx, z - cz) <= cr for cx, cz, cr in AREA):
            continue
        best = None
        for rw, rx, rz, rr, f in nf.get(npc_key(n), []):
            if rw >= 0 and rw != w or rr > 0 and math.hypot(x - rx, z - rz) > rr:
                continue
            rank = rr if rr > 0 else (1e30 if rw >= 0 else 1e38)
            if best is None or rank < best[0]:
                best = (rank, f)
        if best and best[1]:
            out.setdefault(best[1], n)
    return out


def check(st, starts, existing):
    """Problems: a new monk who would not start at the monastery, or below a floor or KOS with an area faction
    (starts); a monk character below a floor or KOS with one (existing)."""
    probs = []
    m = st.monastery()
    area = area_factions(st.db)
    floors = st.floors()
    for race, hum in (st.monk_races() + [(RACE_FROGLOK, 1), (0, 0)]) if starts else []:
        p = place_of(st.dc, race, hum, MONK)
        if p is None or (p["world"], p["x"], p["z"]) != (m["world"], m["x"], m["z"]):
            probs.append(f"{st.race_name(race, hum)} monk starts at {None if p is None else (p['world'], p['x'], p['z'])}")
    for rid, (wire, city, name) in st.racedefs.items() if starts else []:
        if not any(r["race"] == rid for r in st.dc):
            p = place_of(st.dc, wire, city if wire == 0 else 0, MONK)
            if p is None or (p["world"], p["x"], p["z"]) != (m["world"], m["x"], m["z"]):
                probs.append(f"{name} monk (no rows: its wire race's) starts at {p and (p['world'], p['x'], p['z'])}")
    for race, hum in st.monk_races() if starts else []:
        combo = combo_of(race, MONK, hum)
        for f in QEYNOS:
            if st.start(f, combo) < floors[f]:
                probs.append(f"{st.race_name(race, hum)} monk starts at {st.start(f, combo)} with {f}")
        for f in area:
            if st.start(f, combo) < KOS_BELOW:
                probs.append(f"{st.race_name(race, hum)} monk is KOS to {f} ({area[f]})")
    if existing:
        own = {(s, f): v for s, f, v in st.db.execute("SELECT serverid, faction, standing FROM charfactions")}
        for c in monk_chars(st.db):
            combo = combo_of(c["race"], MONK, c["humType"])
            for f in set(QEYNOS) | set(area):
                v = own.get((c["serverid"], f), st.start(f, combo))
                if f in floors and v < floors[f] or v < KOS_BELOW:
                    probs.append(f"{c['charname']} stands at {v} with {f}")
    return probs, area


# ---- writing ------------------------------------------------------------------------------------------------------

def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-monkstart"
    src = sqlite3.connect(path)
    out = sqlite3.connect(dst)
    src.backup(out)
    out.close()
    src.close()
    print(f"backup: {dst}")


class Writer:
    def __init__(self, db):
        self.db = db
        db.execute(LOG_DDL)
        db.execute(AUDIT_SCHEMA)
        self.run = (db.execute(f"SELECT COALESCE(MAX(run), 0) FROM {LOG}").fetchone()[0]) + 1
        self.at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
        self.n = 0

    def log(self, step, tbl, ref, before, after, action, detail):
        cur = self.db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, "
                              "after, detail) VALUES (?, 0, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                              (self.at, WHO, VIA, action, tbl, go_json(ref), None if before is None else go_json(before),
                               None if after is None else go_json(after), detail))
        self.db.execute(f"INSERT INTO {LOG} (run, step, tbl, ref, before, after, audit) VALUES (?, ?, ?, ?, ?, ?, ?)",
                        (self.run, step, tbl, go_json(ref), None if before is None else go_json(before),
                         None if after is None else go_json(after), cur.lastrowid))
        self.n += 1

    def write(self, step, changes):
        for tbl, ref, before, after in changes:
            if ref is None:  # a row made
                cols = list(after)
                cur = self.db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                                      [after[c] for c in cols])
                ref = {KEYS[tbl][0]: cur.lastrowid} if len(KEYS[tbl]) == 1 else {c: after[c] for c in KEYS[tbl]}
                self.log(step, tbl, ref, None, row_of(self.db, tbl, ref), "insert", f"{DETAIL} (run {self.run})")
            elif before is None:  # a keyed row made (charfactions)
                cols = list(after)
                self.db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                                [after[c] for c in cols])
                self.log(step, tbl, ref, None, row_of(self.db, tbl, ref), "insert", f"{DETAIL} (run {self.run})")
            else:
                diff = [c for c in after if after[c] != before[c]]
                self.db.execute(f"UPDATE {tbl} SET " + ", ".join(f"{c} = ?" for c in diff) + " WHERE " +
                                " AND ".join(f"{c} = ?" for c in KEYS[tbl]),
                                [after[c] for c in diff] + [ref[c] for c in KEYS[tbl]])
                self.log(step, tbl, ref, before, row_of(self.db, tbl, ref), "update", f"{DETAIL} (run {self.run})")


def undo(db, steps):
    if not db.execute(f"SELECT 1 FROM sqlite_master WHERE name = '{LOG}'").fetchone():
        print("nothing to undo: no log")
        return
    wr = Writer(db)
    wr.run -= 1  # undo lines carry the run they undo in their detail; the log's rows go
    marks = ",".join("?" * len(steps))
    todo = db.execute(f"SELECT seq, run, step, tbl, ref, before, after FROM {LOG} WHERE step IN ({marks}) ORDER BY seq DESC",
                      steps).fetchall()
    left = 0
    for seq, run, step, tbl, ref, before, after in todo:
        ref, before, after = json.loads(ref), before and json.loads(before), after and json.loads(after)
        now = row_of(db, tbl, ref)
        if now is None or go_json(now) != go_json(after):
            print(f"  left: {tbl} {ref} changed since it was written ({now})")
            left += 1
            continue
        where = " AND ".join(f"{c} = ?" for c in KEYS[tbl])
        if before is None:
            db.execute(f"DELETE FROM {tbl} WHERE {where}", [ref[c] for c in KEYS[tbl]])
            action = "delete"
        else:
            cols = list(before)
            db.execute(f"UPDATE {tbl} SET " + ", ".join(f"{c} = ?" for c in cols) + f" WHERE {where}",
                       [before[c] for c in cols] + [ref[c] for c in KEYS[tbl]])
            action = "update"
        cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, "
                         "detail) VALUES (?, 0, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                         (wr.at, WHO, VIA, action, tbl, go_json(ref), go_json(now),
                          None if before is None else go_json(row_of(db, tbl, ref)), f"undo: {DETAIL} (run {run})"))
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
    print(f"undone: {len(todo) - left} rows ({', '.join(steps)}); {left} left as they are now")
    if not db.execute(f"SELECT 1 FROM {LOG}").fetchone():
        db.execute(f"DROP TABLE {LOG}")


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--undo", action="store_true")
    ap.add_argument("--starts", action="store_true")
    ap.add_argument("--existing", action="store_true")
    ap.add_argument("--dialog", action="store_true")
    ap.add_argument("--step", action="append", choices=STEPS, help="--undo: this step alone (repeatable)")
    ap.add_argument("--server-binary", help="--apply --dialog: the server that will run on this database")
    ap.add_argument("--no-backup", action="store_true")
    a = ap.parse_args(argv)
    if a.apply and a.undo:
        ap.error("--apply or --undo, not both")
    chosen = [s for s in STEPS if getattr(a, s)]
    if a.apply and not chosen:
        ap.error("--apply wants the steps: --starts, --existing, --dialog")
    if not os.path.exists(a.db):
        ap.error(f"no database {a.db}")
    if a.apply and a.dialog:
        if not a.server_binary:
            ap.error("--dialog --apply wants --server-binary: a server that reads npcdialog.class and not_race "
                     "(an older one says the lines to everyone)")
        with open(a.server_binary, "rb") as fh:
            if BINARY_MARK not in fh.read():
                ap.error(f"{a.server_binary} does not read npcdialog.class and not_race (the monk-start branch's)")
    if (a.apply or a.undo) and not a.no_backup:
        backup(a.db)
    db = sqlite3.connect(a.db)
    try:
        if a.undo:
            with db:
                undo(db, a.step or list(STEPS))
            return 0
        st = State(db)
        plans = {"starts": plan_starts(st), "existing": plan_existing(st), "dialog": plan_dialog(st)}
        for s in STEPS:
            print(f"== {s}{'' if s in chosen or not a.apply else ' (not applied this run)'}")
            for line in plans[s][1]:
                print(line)
        if not a.apply:
            probs, area = check_after_plan(db, plans)
            print(f"== check, as it would be with every step (the area's factions: {', '.join(area)})")
            for p in probs:
                print(f"PROBLEM: {p}")
            print("OK: every monk at the monastery, at the floors, KOS to no one there" if not probs else "")
            return 1 if probs else 0
        with db:
            wr = Writer(db)
            if a.dialog:
                for col, ddl in DIALOG_COLUMNS:
                    if not has_column(db, "npcdialog", col):
                        db.execute(f"ALTER TABLE npcdialog ADD COLUMN {col} {ddl}")
            for s in chosen:
                wr.write(s, plans[s][0])
            probs, area = check(State(db), "starts" in chosen, "existing" in chosen)
            if probs:
                for p in probs:
                    print(f"PROBLEM: {p}")
                raise SystemExit("not written: the check after the write failed (rolled back)")
        print(f"run {wr.run}: wrote {wr.n} rows ({', '.join(chosen)}), each in gm_audit; stop the server for it "
              f"(a keep restart's before.sh); --undo [--step S] takes it back")
        return 0
    finally:
        db.close()


def check_after_plan(db, plans):
    """check() on a copy in memory with every step written."""
    mem = sqlite3.connect(":memory:")
    db.backup(mem)
    wr = Writer(mem)
    for col, ddl in DIALOG_COLUMNS:
        if not has_column(mem, "npcdialog", col):
            mem.execute(f"ALTER TABLE npcdialog ADD COLUMN {col} {ddl}")
    for s in STEPS:
        wr.write(s, plans[s][0])
    out = check(State(mem), True, True)
    mem.close()
    return out


if __name__ == "__main__":
    sys.exit(main())
