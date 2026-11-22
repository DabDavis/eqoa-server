#!/usr/bin/env python3
"""The quarry shaft: a way into Blackvein Deep from Moradhim's own world, at the old quarry by Coachman Doba (the owner,
2026-11-22, approving a player's idea: "using the quarry in mord as a evil starter point. raise the earth over the two
pillars northeast of coach where the mine carts tracks stop add a cave and have it turn left under the mountain lots of
room there for a sub city"). Heirloom alone, over tools/evil_dwarf_start_20261121.py (refused without it).

The spot: Tunaria zone 40's quarry pit (floor y 39-42, x 15380-15550, z 9110-9200), north-east of Coachman Doba (15352,
9267). Its mine-cart rails (0x27DB8A66, z 9161.7) run east from the coach camp down into the pit and stop at a bumper
(0x25D902F4, 15535.7, 42.0, 9161.8) before two stone pillars with a striped banner between them (x 15553-15595; the
north one z 9135-9155, the south one z 9170-9190); between them a dirt ramp climbs east out of the pit (y 40 at x 15553,
50 at 15575). Read off `eqoa actormesh TUNARIA.ESF`, `eqoa shot TUNARIA.ESF zone 40` and cmd/campground on world 0's
baked ground.

What it is NOT: the ground of world 0 is the disc's (TUNARIA.ESF, every client's own copy). No earth is raised and no
cave is cut; that would be a new world file (vanilla) or a zone overlay every PS2 takes (claude-notes/
evil-dwarf-start-20261121.md's sequel, quarry-shaft-20261122.md). The shaft is a story's: the shaftwarden at the rails'
end takes a dwarf shadow knight or necromancer down to Blackvein Deep (world 10: a client that holds it, vanilla), and
the deep's tunnelwarden brings one up here. A PS2 (no world 10) is told the way is closed by the giver (this branch's
handlers/questevents.go) and has the smugglers' road to Freeport here as at the camp.

Steps, all written in one transaction:

people   npcs: Shaftwarden Orrik Slagfoot at the rails' end, between the pillars (a copy of the camp's deepwarden: a
         dwarf, friendly, no faction). His words (npcdialog by name), and a line for the camp's deepwarden and the
         deep's tunnelwarden that names the shaft.
quests   three trips (repeatable, a teleport each, classes 3 and 11, race 4): the quarry down into the deep (the deep's
         hall, its start), the quarry to Freeport's guilds (the smugglers' road), the deep up to the quarry (from the
         tunnelwarden, beside his way up to the camp).

Every row written is a gm_audit line (via "tool") and a row of moradhim_quarry_20261122_log; --undo puts each row back
(one changed since is left, and said), takes the characters' marks of the three trips with it, and drops the log. A
report by default; --apply writes, once; --apply and --undo copy the database first unless --no-backup.

WHEN IT TAKES EFFECT: npcs, npcdialog and quests are read at the server's start: --apply and --undo in a keep restart's
before.sh. The PS2's "closed" line needs the server of this branch; an older one moves no one and says nothing.

    python3 -E -s tools/moradhim_quarry_20261122.py --db eqoa.db                  report
    python3 -E -s tools/moradhim_quarry_20261122.py --db eqoa.db --apply          write it
    python3 -E -s tools/moradhim_quarry_20261122.py --db eqoa.db --undo           take it back
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
import monk_start as ms  # noqa: E402  (go_json, rows, combo_of, resolve, npc_key)
import evil_dwarf_start_20261121 as ed  # noqa: E402  (the deep's places, its KOS rule)

LOG = "moradhim_quarry_20261122_log"
DETAIL = ("the owner, 2026-11-22, a player's idea approved: \"using the quarry in mord as a evil starter point\": "
          "tools/moradhim_quarry_20261122.py")
WHO, VIA = "claude", "tool"
SOURCE = "ours: tools/moradhim_quarry_20261122.py"
DIALOG_SOURCE = "moradhim-quarry"
TALKER = 128
DWARF = 4
CLASSES = "3, 11"
KEYS = {"npcs": ("id",), "npcdialog": ("id",), "quests": ("questid",), "queststeps": ("questid", "step")}
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
STEPS = ("people", "quests")

# Where things stand (world 0, cmd/campground's stand on the baked ground)
QUARRY = {"world": 0, "x": 15528.0, "y": 41.72, "z": 9172.0, "toward": [15553.0, 9158.0],
          "note": "the landing from the deep: beside the rails, before their bumper, the pillars ahead (east)"}
WARDEN = {"name": "Shaftwarden Orrik Slagfoot", "like": "Deepwarden Korga Ashvein", "level": 40,
          "at": [15553.0, 40.05, 9158.0], "toward": [15528.0, 9172.0],
          "note": "at the rails' end, in the gap between the two pillars under the banner, facing back along the rails"}
DEEP_WARDEN = "Tunnelwarden Brokk Deepdelve"   # the deep's (evil_dwarf_start_20261121.json)
CAMP_WARDEN = "Deepwarden Korga Ashvein"       # the camp's
LINES = {
    WARDEN["name"]: [
        "The Stoneguard say this quarry is worked out. Let them go on saying it.",
        "The carts stop here. The rest of the way you walk: between the pillars, then east, then north under the mountain.",
        "There is room enough under that mountain for a city. Thrain says one day. I say one day, too, and dig.",
        "Mind the ramp. And do not look back at the coach: Doba counts who goes in.",
        "Every shift some fool from Moradhim asks why the rails run into a wall. I tell him the stone was better here.",
    ],
    CAMP_WARDEN: ["The quarry shaft is quicker than mine. Orrik keeps it, at the end of the mine-cart rails."],
    DEEP_WARDEN: ["The north tunnel climbs all the way to the old quarry by Moradhim's coach. Orrik keeps that end."],
}
TRIPS = [
    {"id": 105301, "key": "quarry/down", "name": "Down the Quarry Shaft", "giver": WARDEN["name"], "to": "start",
     "offer": "The shaft between these pillars runs east, then turns north under the mountain, and down. It comes out in "
              "Blackvein Deep.\nShall I take you down?",
     "accept": "Take me down.", "declined": "The shaft will keep.", "started": "Keep your head low where it turns."},
    {"id": 105302, "key": "quarry/freeport", "name": "The Smugglers' Road", "giver": WARDEN["name"], "to": "freeport",
     "offer": "A side drift off the shaft meets the smugglers' road. It comes out under Freeport, near the guilds that "
              "train our kind.\nShall I take you to Freeport?",
     "accept": "Take me to Freeport.", "declined": "The road will keep.",
     "started": "Keep quiet. Freeport's militia do not like surprises."},
    {"id": 105303, "key": "quarry/up", "name": "Up to the Old Quarry", "giver": DEEP_WARDEN, "to": "quarry",
     "offer": "The north tunnel climbs to the old quarry by Moradhim's coach, where the mine carts stop. Orrik keeps "
              "that end.\nShall I take you up?",
     "accept": "Take me up.", "declined": "The tunnel will be here.", "started": "Follow the draught. It smells of the coach."},
]


def rows(db, sql, args=()):
    return ms.rows(db, sql, args)


def row_of(db, tbl, ref):
    r = rows(db, f"SELECT * FROM {tbl} WHERE " + " AND ".join(f"{c} = ?" for c in KEYS[tbl]),
             tuple(ref[c] for c in KEYS[tbl]))
    return r[0] if r else None


def has_table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def places(deep):
    """The trips' places: the deep's start and Freeport's landing (evil_dwarf_start_20261121.json) and the quarry."""
    return {"start": deep["start"], "freeport": deep["freeport"], "quarry": QUARRY}


def teleport(p):
    facing = p["facing"] if "facing" in p else ed.heading_to(p["x"], p["z"], *p["toward"])
    ev = {"on": "done", "do": "teleport", "x": p["x"], "y": p["y"], "z": p["z"], "facing": round(facing, 4)}
    if p["world"]:
        ev["world"] = p["world"]
    return ev


def quest_row(t):
    return {"questid": t["id"], "name": t["name"], "enabled": 1, "giver": t["giver"], "minlevel": 1, "maxlevel": 60,
            "classes": CLASSES, "races": str(DWARF), "requires": "", "repeatable": 1, "offer": t["offer"],
            "accept": t["accept"], "decline": "Not now.", "declined": t["declined"], "started": t["started"],
            "source": f"{SOURCE} ({t['key']})", "notes": "", "events": "", "factionreq": "", "excludes": "", "asks": "",
            "daygroup": ""}


def step_row(t, where):
    return {"questid": t["id"], "step": 1, "kind": "talk", "target": t["giver"], "npcid": 0, "items": "", "tunar": 0,
            "count": 1, "world": 0, "x": 0.0, "y": 0.0, "z": 0.0, "radius": 15.0,
            "journal": f"Tell {t['giver']} when you are ready.", "answer": "", "says": "", "waiting": "", "notyet": "",
            "reminder": "", "xp": 0, "rewardtunar": 0, "rewarditems": "", "factions": "", "spells": "",
            "events": json.dumps([{"on": "start", "do": "dostep"}, teleport(where)], separators=(",", ":")),
            "chance": 0.0, "byclass": "", "choices": "", "height": 0.0, "anyorder": 0}


class Plan:
    def __init__(self, db, deep):
        self.db, self.deep = db, deep
        self.changes = []  # (step, tbl, ref, before, after, note)
        self.lines = {s: [] for s in STEPS}
        self.problems = []

    def add(self, step, tbl, ref, before, after, note=""):
        self.changes.append((step, tbl, ref, before, after, note))

    def people(self):
        db, w = self.db, WARDEN
        if db.execute("SELECT count(*) FROM npcs WHERE lower(npc_name) = lower(?)", (w["name"],)).fetchone()[0]:
            self.problems.append(f"an NPC is named {w['name']!r} already")
        t = rows(db, "SELECT * FROM npcs WHERE npc_name = ? ORDER BY id LIMIT 1", (w["like"],))
        if not t:
            self.problems.append(f"no {w['like']!r} to copy (the evil dwarf start's camp)")
            return
        x, y, z = w["at"]
        row = {k: v for k, v in t[0].items() if k != "id"}
        row.update(npc_name=w["name"], x_coord=x, y_coord=y, z_coord=z, world=0, npc_level=w["level"], npc_type=TALKER,
                   facing=ed.byte_heading(ed.heading_to(x, z, *w["toward"])), dt_kill=None)
        self.add("people", "npcs", None, None, row, w["name"])
        self.lines["people"].append(f"  {w['name']}: world 0 ({x}, {y}, {z}) level {w['level']} facing {row['facing']} "
                                    f"(a copy of {w['like']}, npcs {t[0]['id']})")
        for name, lines in LINES.items():
            if name != w["name"] and not rows(db, "SELECT id FROM npcs WHERE npc_name = ? LIMIT 1", (name,)):
                self.problems.append(f"no NPC {name!r} for its line")
            for line in lines:
                self.add("people", "npcdialog", None, None, {"name": name, "line": line, "weight": 1, "source": DIALOG_SOURCE})
            self.lines["people"].append(f"  words of {name}: {len(lines)}")

    def quests(self):
        db, at = self.db, places(self.deep)
        if not rows(db, "SELECT id FROM npcs WHERE npc_name = ? LIMIT 1", (DEEP_WARDEN,)):
            self.problems.append(f"no {DEEP_WARDEN!r} (the deep's tunnelwarden)")
        for t in TRIPS:
            if row_of(db, "quests", {"questid": t["id"]}) is not None:
                self.problems.append(f"quest {t['id']} is there already")
                continue
            self.add("quests", "quests", {"questid": t["id"]}, None, quest_row(t))
            self.add("quests", "queststeps", {"questid": t["id"], "step": 1}, None, step_row(t, at[t["to"]]))
            p = at[t["to"]]
            self.lines["quests"].append(f"  {t['id']} {t['name']}: {t['giver']} -> {t['to']} (world {p['world']} "
                                        f"{p['x']}, {p['y']}, {p['z']})")

    def build(self):
        if not has_table(self.db, ed.LOG) or not self.db.execute(f"SELECT 1 FROM {ed.LOG}").fetchone():
            self.problems.append("the evil dwarf start (tools/evil_dwarf_start_20261121.py) is not applied here: "
                                 "the quarry shaft is its way in, Heirloom's alone")
            return self
        for s in STEPS:
            getattr(self, s)()
        return self


def check(db):
    """After the write: no NPC within 150 of the quarry is KOS to a new dwarf shadow knight or necromancer (the
    server's faction rules, as the evil dwarf start's check), and the trips' givers stand."""
    probs = []
    st = {}
    for r in db.execute("SELECT race, class, humtype, faction, standing FROM factionstart"):
        st.setdefault(r[3], []).append((r[0], r[1], r[2], r[4]))
    base = dict(db.execute("SELECT name, standing FROM factions"))
    rules = {}
    for name, w, rx, rz, radius, fac in db.execute("SELECT name, world, x, z, radius, faction FROM npcfaction"):
        rules.setdefault(ms.npc_key(name), []).append((w, rx, rz, radius, fac))
    near = db.execute("SELECT npc_name, x_coord, z_coord FROM npcs WHERE world = 0 AND (x_coord - ?) * (x_coord - ?) + "
                      "(z_coord - ?) * (z_coord - ?) < 22500",
                      (QUARRY["x"], QUARRY["x"], QUARRY["z"], QUARRY["z"])).fetchall()
    for cls in (3, 11):
        combo = ms.combo_of(DWARF, cls, 0)
        for name, x, z in near:
            fac = ed.npc_faction(rules, name, 0, x, z)
            if fac and ms.resolve(st.get(fac, []), base.get(fac, 0), combo) < ed.KOS_BELOW:
                probs.append(f"class {cls}: {name} ({fac}) by the quarry is KOS to it")
    for t in TRIPS:
        if not rows(db, "SELECT id FROM npcs WHERE npc_name = ? LIMIT 1", (t["giver"],)):
            probs.append(f"trip {t['id']}: no {t['giver']!r}")
    return probs


class Writer:
    def __init__(self, db):
        self.db = db
        db.execute(LOG_DDL)
        db.execute(ed.AUDIT_DDL)
        self.at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
        self.n = 0

    def audit(self, action, tbl, ref, before, after, detail):
        cur = self.db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                              "VALUES (?, 0, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                              (self.at, WHO, VIA, action, tbl, ms.go_json(ref), None if before is None else ms.go_json(before),
                               None if after is None else ms.go_json(after), detail))
        return cur.lastrowid

    def log(self, step, tbl, ref, before, after, action):
        a = self.audit(action, tbl, ref, before, after, DETAIL)
        self.db.execute(f"INSERT INTO {LOG} (step, tbl, ref, before, after, audit, at) VALUES (?, ?, ?, ?, ?, ?, ?)",
                        (step, tbl, ms.go_json(ref), None if before is None else ms.go_json(before),
                         None if after is None else ms.go_json(after), a, self.at))
        self.n += 1

    def write(self, changes):
        for step, tbl, ref, before, after, note in changes:
            cols = list(after)
            cur = self.db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                                  [after[c] for c in cols])
            if tbl == "npcs":
                self.db.execute("UPDATE npcs SET npc_id = ? WHERE id = ?", (cur.lastrowid, cur.lastrowid))
            if ref is None:
                ref = {KEYS[tbl][0]: cur.lastrowid}
            self.log(step, tbl, ref, None, row_of(self.db, tbl, ref), "insert")


def same(tbl, now, after):
    if now is None:
        return False
    if tbl == "npcs":
        return all(now.get(c) == after.get(c) for c in NPC_SAME)
    return ms.go_json(now) == ms.go_json(after)


def undo(db):
    if not has_table(db, LOG):
        print("nothing to undo: no log")
        return 0, 0
    wr = Writer(db)
    back = left = 0
    for seq, tbl, ref, after in db.execute(f"SELECT seq, tbl, ref, after FROM {LOG} ORDER BY seq DESC").fetchall():
        ref, after = json.loads(ref), json.loads(after)
        now = row_of(db, tbl, ref)
        if not same(tbl, now, after):
            print(f"  left: {tbl} {ref}: changed since it was written")
            left += 1
            continue
        db.execute(f"DELETE FROM {tbl} WHERE " + " AND ".join(f"{c} = ?" for c in KEYS[tbl]), [ref[c] for c in KEYS[tbl]])
        wr.audit("delete", tbl, ref, now, None, f"undo: {DETAIL}")
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    ids = [t["id"] for t in TRIPS]
    if has_table(db, "charquests"):
        for r in rows(db, "SELECT * FROM charquests WHERE questid IN (%s)" % ",".join("?" * len(ids)), ids):
            if row_of(db, "quests", {"questid": r["questid"]}) is None:
                db.execute("DELETE FROM charquests WHERE serverid = ? AND questid = ?", (r["serverid"], r["questid"]))
                wr.audit("delete", "charquests", {"serverid": r["serverid"], "questid": r["questid"]}, r, None, f"undo: {DETAIL}")
    if not db.execute(f"SELECT 1 FROM {LOG}").fetchone():
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def backup(path):
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-quarry"
    src, out = sqlite3.connect(path), sqlite3.connect(dst)
    src.backup(out)
    out.close()
    src.close()
    print(f"backup: {dst}")


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--deep", default=ed.DATA, help="the evil dwarf start's data (its places)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    ap.add_argument("--no-backup", action="store_true")
    a = ap.parse_args(argv)
    if not os.path.exists(a.db):
        ap.error(f"no database {a.db}")
    deep = json.load(open(a.deep))
    if (a.apply or a.undo) and not a.no_backup:
        backup(a.db)
    db = sqlite3.connect(a.db, timeout=5)
    try:
        if a.undo:
            with db:
                back, left = undo(db)
            print(f"undo: {back} rows put back, {left} left as they are now")
            return 0
        if has_table(db, LOG) and db.execute(f"SELECT 1 FROM {LOG}").fetchone():
            n = db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0]
            print(f"applied already ({n} rows in {LOG}): nothing to do; --undo takes it back")
            return 0
        p = Plan(db, deep).build()
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
            mem = sqlite3.connect(":memory:")
            db.backup(mem)
            Writer(mem).write(Plan(mem, deep).build().changes)
            probs = check(mem)
            mem.close()
            for pr in probs:
                print(f"CHECK: {pr}")
            if not probs:
                print("check: no one by the quarry is KOS to a dwarf shadow knight or necromancer; every trip's giver stands")
            return 1 if probs else 0
        with db:
            wr = Writer(db)
            wr.write(p.changes)
            probs = check(db)
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
