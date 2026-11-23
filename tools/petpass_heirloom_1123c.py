#!/usr/bin/env python3
"""Heirloom petitions 2759 and 2766-2770 of 2026-11-23 (the third batch of the day), as data.

Every row is found by its name (and what else names it), never by an id alone, and must read as it did when this
was made; a row already as this writes it is left ("already"); anything else refuses the whole run. Every new place
was asked of the server's own ground files (cmd/campground: the floor a spawn is set down on, not in the rock).

  2759  Razuam's Signet (The Temby Debt's drop, the owner's yes): a necklace worn at level 25 by any class or race,
        +10 STA, DEX and WIS, a necklace's icon (Icebone Necklace's); its hand-in to Dristan Blane becomes a show
        (the signet is shown and kept), so the step still counts it and the player keeps what it wears.
  2766  Commander Dursley stands on the ground floor of Saerk's tower, where the player stood, not on the roof.
  2767  the goblin fanatic of The Goblin Poison's first step (the row the quest's first kill is) spawns on the
        tower's inner ramp, where the player stood (and where it stood until 2026-11-21).
  2768  the two orbs of light of Saerk's tower answer a talk (lightspheres): "Touch the light." sends the player as
        a step onto the pad does (orbs 9 and 10), landing where those orbs land.
  2769  the young eagle out Oggok's north gate stood inside the hill: its spawn moved where the player stood; the
        other young eagle there in the rock (by the same ground check) set on the hill's surface over it.
  2770  the lizardman outcasts' tower: of the six rows (five on the roof, one in the tower's ground under the ramp)
        one at the foot of the ramp, one midway up, one near the top, one on the roof, and two roam the ground
        round the tower (npcwander 30).

Every row written is a gm_audit line (via "tool") and a row of petpass_heirloom_1123c_log; --undo puts each back
as it was (a row changed since is left and said) and drops the log. A report by default; --apply writes once (a
second finds the log and writes nothing); --apply and --undo copy the database first (<db>.bak-<when>-pre-pet1123c)
unless --no-backup.

WHEN IT TAKES EFFECT: the item record at each player's next login; lightspheres within 30 s (a talk reads them at
most 30 s old); npcs, npcwander and queststeps at the server's next start (or a GM panel reload of each).

    python3 -E -s tools/petpass_heirloom_1123c.py --db eqoa.db            report
    python3 -E -s tools/petpass_heirloom_1123c.py --db eqoa.db --apply    write it
    python3 -E -s tools/petpass_heirloom_1123c.py --db eqoa.db --undo     take it back
"""
import argparse
import datetime
import json
import math
import sqlite3
import sys

LOG = "petpass_heirloom_1123c_log"
WHO, VIA = "claude", "tool"
DETAIL = "Heirloom petitions 2759, 2766-2770 (2026-11-23): tools/petpass_heirloom_1123c.py"
SOURCE = "petpass-1123c"

# 2759: the signet, a necklace (columns: old, new)
SIGNET = "Razuam's Signet"
NECK_ICON, NECK_ICON_REF = 756343796, "Icebone Necklace"
SIGNET_COLS = {
    "equipslot": (8, 4),                 # a ring's slot -> the neck's
    "itemicon": (1429772174, NECK_ICON),
    "levelreq": (1, 25),
    "classuse": (1024, 32767),
    "raceuse": (1, 32767),
    "maxhp": (200, 4400),                # a level 22-25 necklace's durability
    "sta": (0, 10), "dex": (0, 10), "wis": (0, 10),
    "itemdesc": ("The signet ring of Razuam Marus.", "The signet ring of Razuam Marus, worn on a chain about the neck."),
}
TEMBY = ("The Temby Debt", "Dristan Blane")

# 2766-2768: Saerk's tower (world 0)
DURSLEY_OLD, DURSLEY_NEW = (21216.2, 270.0, 13508.2), (21233.8, 243.25, 13499.8)
FANATIC_ID, FANATIC_OLD, FANATIC_NEW = 122448, (21232.0, 240.8, 13532.0), (21209.0, 253.0, 13515.4)
GOBLIN_POISON = ("The Goblin Poison", "Commander Dursley")
# each orb's NPC spawn, the orbs row it stands in, the line; the landing (to_x/z) is put so that
# handlers/lightsphere.go's inFrontOf (bindReach 3 along to_facing) lands where the orbs row does
ORB_LOW, ORB_HIGH = (21250.0, 245.0, 13515.0), (21253.7, 305.188, 13511.2)
SPHERES = [
    (ORB_LOW, 9, "This orb of light carries you up the tower to Saerk."),
    (ORB_HIGH, 10, "This orb of light carries you down to the ground floor of the tower."),
]
BIND_REACH = 3.0

# 2769: young eagles (world 1): row, old place, new place
EAGLES = [
    (126858, (8468.48, 80.82, 6841.35), (8476.6, 80.36, 6842.4)),   # the player's spot
    (126701, (8449.225, 71.44, 6660.65), (8449.225, 74.05, 6660.65)),  # the hill's surface over it
]

# 2770: the outcasts' tower (world 1): row, old place, new place
OUTCASTS = [
    (126349, (8320.99, 111.21, 7640.05), (8325.5, 77.15, 7625.0)),    # the foot of the ramp
    (126350, (8323.005, 111.3, 7637.9), (8334.0, 92.0, 7635.0)),      # midway up (the east side)
    (126367, (8328.48, 111.3, 7629.225), (8318.0, 102.0, 7634.0)),    # near the top (the west side)
    (126351, (8326.145, 111.3, 7630.1), (8302.0, 76.68, 7636.0)),     # roams the ground, west
    (126352, (8323.1, 99.7, 7632.415), (8348.0, 76.81, 7640.0)),      # roams the ground, east
]
OUTCAST_ROOF = (126366, (8320.64, 111.3, 7638.13))                  # stays on the roof
ROAMERS, ROAM_RADIUS = (126351, 126352), 30.0

KEYS = {"itempattern": ("patternid",), "queststeps": ("questid", "step"), "npcs": ("id",), "npcwander": ("npcid",),
        "lightspheres": ("id",)}
LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    ref    TEXT NOT NULL,
    before TEXT,
    after  TEXT,
    audit  INTEGER,
    at     TEXT NOT NULL
)"""
AUDIT_DDL = """CREATE TABLE IF NOT EXISTS gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER,
    username TEXT, via TEXT, ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT)"""


class Refused(Exception):
    pass


def go_json(m):
    """A row as Go's json.Marshal writes a map (keys sorted, <>& escaped), as the panel's audit lines are."""
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def rows(db, sql, args=()):
    cur = db.execute(sql, args)
    names = [c[0] for c in cur.description]
    return [dict(zip(names, r)) for r in cur.fetchall()]


def one(db, what, sql, args=()):
    got = rows(db, sql, args)
    if len(got) != 1:
        raise Refused(f"{what}: {len(got)} rows, want one")
    return got[0]


def near(a, b):
    return abs(float(a) - float(b)) < 0.05


def same(a, b):
    return near(a, b) if isinstance(b, float) else a == b


class Plan:
    def __init__(self, db):
        self.db = db
        self.changes = []  # (tbl, key dict or None, before row or None, values)
        self.lines = []

    def set(self, tbl, row, values, olds, what):
        """values over row, when each column of olds reads its old; already as values: left."""
        if all(same(row[c], v) for c, v in values.items()):
            self.lines.append(f"already  {what}")
            return
        for c, old in olds.items():
            if not same(row[c], old):
                raise Refused(f"{what}: {c} reads {row[c]!r}, not {old!r} as read when this was made")
        key = {k: row[k] for k in KEYS[tbl]}
        self.changes.append((tbl, key, dict(row), dict(values)))
        self.lines.append(f"change   {what}: " + ", ".join(f"{c} {row[c]!r} -> {v!r}" for c, v in values.items()))

    def insert(self, tbl, key, values, what, present):
        """a new row, unless present (a row already there as wanted: left; another: refused)."""
        if present is not None:
            if all(same(present[c], v) for c, v in values.items()):
                self.lines.append(f"already  {what}")
                return
            raise Refused(f"{what}: a row is there already: {present}")
        self.changes.append((tbl, key, None, dict(values)))
        self.lines.append(f"insert   {what}")

    def move(self, pet, world, npc_id, name, old, new, what):
        r = one(self.db, f"{pet} {what}", "SELECT * FROM npcs WHERE id = ? AND lower(npc_name) = ? AND world = ?",
                (npc_id, name.lower(), world))
        self.set("npcs", r, {"x_coord": new[0], "y_coord": new[1], "z_coord": new[2]},
                 {"x_coord": old[0], "y_coord": old[1], "z_coord": old[2]}, f"{pet} {what} ({npc_id})")

    def build(self):
        db = self.db
        # 2759: the signet
        one(db, f"{NECK_ICON_REF}'s icon", "SELECT patternid FROM itempattern WHERE itemname = ? AND itemicon = ? "
            "AND equipslot = 4 LIMIT 1", (NECK_ICON_REF, NECK_ICON))
        sig = one(db, SIGNET, "SELECT * FROM itempattern WHERE itemname = ?", (SIGNET,))
        self.set("itempattern", sig, {c: n for c, (_, n) in SIGNET_COLS.items()},
                 {c: o for c, (o, _) in SIGNET_COLS.items()}, f"2759 {SIGNET} ({sig['patternid']}) a necklace")
        q = one(db, TEMBY[0], "SELECT * FROM quests WHERE name = ? AND giver = ?", TEMBY)
        s3 = one(db, "The Temby Debt's hand-in", "SELECT * FROM queststeps WHERE questid = ? AND step = 3 AND target = ? "
                 "AND items = ?", (q["questid"], TEMBY[1], str(sig["patternid"])))
        self.set("queststeps", s3, {"kind": "show"}, {"kind": "give"}, "2759 The Temby Debt step 3: shown, kept")

        # 2766, 2767
        one(db, "The Goblin Poison", "SELECT * FROM quests WHERE name = ? AND giver = ?", GOBLIN_POISON)
        d = one(db, "Commander Dursley", "SELECT * FROM npcs WHERE npc_name = 'Commander Dursley' AND world = 0")
        self.move("2766", 0, d["id"], "Commander Dursley", DURSLEY_OLD, DURSLEY_NEW, "Commander Dursley's spawn")
        self.move("2767", 0, FANATIC_ID, "goblin fanatic", FANATIC_OLD, FANATIC_NEW,
                  "the goblin fanatic's spawn (the first kill)")

        # 2768: the orbs answer a talk
        for (ox, oy, oz), orb_id, line in SPHERES:
            one(db, f"the orb of light at ({ox}, {oz})", """SELECT id FROM npcs WHERE npc_name = 'orb of light' AND
                world = 0 AND abs(x_coord - ?) < 0.05 AND abs(z_coord - ?) < 0.05""", (ox, oz))
            orb = one(db, f"orbs {orb_id}", "SELECT * FROM orbs WHERE id = ? AND name LIKE 'Saerk''s Tower:%' AND world = 0",
                      (orb_id,))
            pair = SPHERES[1][0] if orb_id == SPHERES[0][1] else SPHERES[0][0]
            dx, dz = orb["to_x"] - pair[0], orb["to_z"] - pair[2]
            h = round((math.atan2(dx, dz) % (2 * math.pi)) * 256 / (2 * math.pi)) % 256
            a = h / 256 * 2 * math.pi
            tx, tz = round(orb["to_x"] - BIND_REACH * math.sin(a), 3), round(orb["to_z"] - BIND_REACH * math.cos(a), 3)
            have = rows(db, """SELECT * FROM lightspheres WHERE world = 0 AND (x - ?) * (x - ?) + (z - ?) * (z - ?) <= 9""",
                        (ox, ox, oz, oz))
            if len(have) > 1:
                raise Refused(f"2768: {len(have)} lightspheres at ({ox}, {oz})")
            self.insert("lightspheres", None,
                        {"world": 0, "x": ox, "z": oz, "line": line, "answer": "Touch the light.", "to_world": 0,
                         "to_x": tx, "to_y": orb["to_y"], "to_z": tz, "to_facing": h, "source": SOURCE,
                         "note": f"Saerk's tower: a talk sends as orbs {orb_id} does, landing on its spot "
                                 f"({orb['to_x']}, {orb['to_y']}, {orb['to_z']}) (Heirloom petition 2768)"},
                        f"2768 lightsphere at ({ox}, {oz}) -> lands ({orb['to_x']}, {orb['to_z']}), facing {h}",
                        have[0] if have else None)

        # 2769
        for npc_id, old, new in EAGLES:
            self.move("2769", 1, npc_id, "young eagle", old, new, "a young eagle's spawn out of the hill")

        # 2770
        rid, rold = OUTCAST_ROOF
        r = one(db, "the roof's outcast", "SELECT * FROM npcs WHERE id = ? AND npc_name = 'lizardman outcast'", (rid,))
        if not (near(r["x_coord"], rold[0]) and near(r["z_coord"], rold[2])):
            raise Refused(f"2770: the roof's outcast {rid} is not where it was read")
        one(db, "The Lizardman Outcast", "SELECT * FROM quests WHERE name = 'The Lizardman Outcast' AND giver = 'Oligan'")
        for npc_id, old, new in OUTCASTS:
            self.move("2770", 1, npc_id, "lizardman outcast", old, new, "a lizardman outcast's spawn")
        for npc_id in ROAMERS:
            w = rows(db, "SELECT * FROM npcwander WHERE npcid = ?", (npc_id,))
            self.insert("npcwander", {"npcid": npc_id},
                        {"npcid": npc_id, "radius": ROAM_RADIUS,
                         "note": "an outcast roams the ground round its tower (Heirloom petition 2770)", "source": SOURCE},
                        f"2770 npcwander {npc_id}, {ROAM_RADIUS:g}", w[0] if w else None)


def audit(db, action, tbl, key, before, after):
    db.execute(AUDIT_DDL)
    cur = db.execute("""INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after,
        detail) VALUES (?, 0, ?, ?, '', ?, ?, ?, ?, ?, ?)""",
                     (now(), WHO, VIA, action, tbl, go_json(key), go_json(before) if before else None,
                      go_json(after) if after else None, DETAIL))
    return cur.lastrowid


def where(key):
    return " AND ".join(f"{k} = ?" for k in key), list(key.values())


def apply(db, plan):
    db.execute(LOG_DDL)
    for tbl, key, before, values in plan.changes:
        if before is None:
            cols = list(values)
            cur = db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                             [values[c] for c in cols])
            if key is None:  # a new row's id is the one made
                key = {"id": cur.lastrowid}
            after = rows(db, f"SELECT * FROM {tbl} WHERE {where(key)[0]}", where(key)[1])[0]
            a = audit(db, "insert", tbl, key, None, after)
        else:
            w, args = where(key)
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in values)} WHERE {w}",
                       list(values.values()) + args)
            after = rows(db, f"SELECT * FROM {tbl} WHERE {w}", args)[0]
            a = audit(db, "update", tbl, key, before, after)
        db.execute(f"INSERT INTO {LOG} (tbl, ref, before, after, audit, at) VALUES (?, ?, ?, ?, ?, ?)",
                   (tbl, json.dumps(key), json.dumps(before) if before else None, json.dumps(after), a, now()))


def undo(db):
    out = []
    for r in rows(db, f"SELECT * FROM {LOG} ORDER BY seq DESC"):
        tbl, key = r["tbl"], json.loads(r["ref"])
        w, args = where(key)
        now_rows = rows(db, f"SELECT * FROM {tbl} WHERE {w}", args)
        after = json.loads(r["after"])
        if len(now_rows) != 1 or now_rows[0] != after:
            out.append(f"left     {tbl} {key}: changed since")
            continue
        if r["before"] is None:
            db.execute(f"DELETE FROM {tbl} WHERE {w}", args)
            audit(db, "delete", tbl, key, after, None)
        else:
            before = json.loads(r["before"])
            cols = [c for c in before if c not in key]
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in cols)} WHERE {w}",
                       [before[c] for c in cols] + args)
            audit(db, "update", tbl, key, after, before)
        out.append(f"restored {tbl} {key}")
    db.execute(f"DROP TABLE {LOG}")
    return out


def backup(path):
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-pet1123c"
    src = sqlite3.connect(path)
    out = sqlite3.connect(dst)
    src.backup(out)
    out.close()
    src.close()
    return dst


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--undo", action="store_true")
    ap.add_argument("--no-backup", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, timeout=5, isolation_level=None)
    db.execute("BEGIN IMMEDIATE")
    try:
        logged = db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (LOG,)).fetchone()
        if a.undo:
            if not logged:
                print("nothing to undo: no log")
                db.execute("ROLLBACK")
                return 0
            if not a.no_backup:
                print("backup", backup(a.db))
            for line in undo(db):
                print(line)
            db.execute("COMMIT")
            return 0
        if logged and a.apply:
            print(f"applied already ({LOG} exists): nothing written")
            db.execute("ROLLBACK")
            return 0
        plan = Plan(db)
        plan.build()
        for line in plan.lines:
            print(line)
        if not a.apply:
            print(f"dry run: {len(plan.changes)} rows would change")
            db.execute("ROLLBACK")
            return 0
        if not a.no_backup:
            print("backup", backup(a.db))
        apply(db, plan)
        db.execute("COMMIT")
        print(f"applied: {len(plan.changes)} rows")
        return 0
    except Refused as e:
        db.execute("ROLLBACK")
        print("refused:", e, file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
