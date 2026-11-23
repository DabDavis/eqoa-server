#!/usr/bin/env python3
"""Heirloom petitions 2802 and 2803 of 2026-11-23 (the seventh batch of the day), as data: Hazinak (Good)'s camp of
Marius and Darius's faction, and An Ivory and Gold Statue's icon.

Every row is found by its name (and what else names it), never by an id alone, and must read as it did when this
was made; a row already as this writes it is left ("already"); anything else refuses the whole run.

  2802  "The 'Good' Hazinak coach is hostile to good characters as are some of the automaton guards." Hazinak has two
        coach towns (entity/coachtowns.go): Coachman Droogra's dark elf docks, "Hazinak (Evil)", and Coachman Zelik's
        dig camp to the south-west, "Hazinak (Good)", the camp of Marius Degintar and Darius Fingltotter and their
        clockwork excavators (l, ll, Xll, Xlll, XV). The 2026-10-29 town seed bound every NPC within reach of Hazinak
        to Citizens of Hazinak, the good camp's with the docks', so a good character (a character with no factionstart
        row starts at the factions table's -2000 with Citizens of Hazinak: a barbarian, a human of the west...) was
        refused by Zelik (handlers/factionrefusal.go, Refuses: at or below -350) and attacked on sight by the
        excavators. The game's own faction for the camp is "Marius and Darius" (DefaultFactions, the eqoa.live
        capture): its factionstart rows are the mirror of Citizens of Hazinak's (the evil races and classes -2000 or
        -500, a dark elf +100 with Hazinak, everyone else 1), and Prima's Table 2 makes it Hazinak's enemy (a Hazinak
        guard's kill gives Marius and Darius +1). The coachman, the two scholars and the five excavators take Marius
        and Darius (the four npcfaction rows' faction changed, excavator XV's row added: it had none), as Coachman
        Droogra and the docks keep Citizens of Hazinak: the good camp serves and ignores a good character and refuses
        an evil one, as the docks do the other way round. Haz explorer and Zsetikaowa, attackable and standing at the
        camp's west edge, keep Hazinak's (a quest's target, not the camp's people). The kill hits stay as they are.

  2803  "'An ivory and gold statue' the item you get as part of the reward for the hazinak quest you get from 'Taszin
        K'Ven' looks like an eyeball. probably wanna go with a statue icon". Item 701333 wore 0x62C1AD8B, the
        gargoyle eye's eyeball. ITEMICON.ESF's 381 surfaces hold no statue; the nearest is the grey winged gargoyle
        bust 0x40D90854 (rendered with ~/.cache/iconsheet), which specter costume (1334) and the Takish scale coifs
        wear; its decimal is copied from row 1334 and checked.

Every row written is a gm_audit line (via "tool") and a row of the log; --undo puts each back as it was (a row
changed since is left and said) and drops the log. A report by default; --apply writes once (a second finds the log
and writes nothing); --apply and --undo copy the database first (<db>.bak-<when>-pre-pet1123g) unless --no-backup.

WHEN IT TAKES EFFECT: npcfaction is read at the server's start: the camp from the next restart. The item record at the
player's next login.

    python3 -E -s tools/petpass_heirloom_1123g.py --db eqoa.db            report
    python3 -E -s tools/petpass_heirloom_1123g.py --db eqoa.db --apply    write it
    python3 -E -s tools/petpass_heirloom_1123g.py --db eqoa.db --undo     take it back
"""
import argparse
import datetime
import json
import math
import sqlite3
import sys

LOG = "petpass_heirloom_1123g_log"
WHO, VIA = "claude", "tool"
DETAIL = ("Heirloom petitions 2802-2803 (2026-11-23): Hazinak (Good)'s camp of Marius and Darius, the statue's icon: "
          "tools/petpass_heirloom_1123g.py")

# 2802
HAZINAK, MD = "Citizens of Hazinak", "Marius and Darius"
CAMP = ["coachman zelik", "marius degintar", "darius fingltotter",
        "excavator l", "excavator ll", "excavator xll", "excavator xlll"]  # npcfaction rows (seed, anywhere) to MD
ADD = ["excavator xv"]                                                       # no npcfaction row: one of MD
CAMP_X, CAMP_Z, CAMP_R = 24870.0, 27590.0, 60.0  # each name's one NPC row stands in Zelik's camp
CAMP_NOTE = "Hazinak (Good)'s camp, Marius and Darius's (petition 2802, 2026-11-23)"

# 2803
STATUE_ID, STATUE = 701333, "An Ivory and Gold Statue"
EYE = 1656860043                       # 0x62C1AD8B, the eyeball it wore
DONOR_ID, DONOR = 1334, "specter costume"  # wears the gargoyle bust, 0x40D90854
BUST = 1087965268

KEYS = {"npcfaction": ("id",), "itempattern": ("patternid",)}
LOG_DDL = """CREATE TABLE IF NOT EXISTS {log} (
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


def has_table(db, name):
    return bool(db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone())


class Plan:
    def __init__(self, db):
        self.db = db
        self.changes = []  # (tbl, key dict or None, before row or None, values or None: a delete)
        self.lines = []

    def set(self, tbl, row, values, olds, what):
        """values over row, when each column of olds reads its old; already as values: left."""
        if all(row[c] == v for c, v in values.items()):
            self.lines.append(f"already  {what}")
            return
        for c, old in olds.items():
            if row[c] != old:
                raise Refused(f"{what}: {c} reads {row[c]!r}, not {old!r} as read when this was made")
        key = {k: row[k] for k in KEYS[tbl]}
        self.changes.append((tbl, key, dict(row), dict(values)))
        self.lines.append(f"change   {what}: " + ", ".join(f"{c} {row[c]!r} -> {v!r}" for c, v in values.items()))

    def camp_npc(self, name):
        got = rows(self.db, "SELECT x_coord, z_coord, world FROM npcs WHERE lower(npc_name) = ?", (name,))
        if len(got) != 1:
            raise Refused(f"NPC {name}: {len(got)} rows, want one")
        r = got[0]
        if r["world"] != 0 or math.hypot(r["x_coord"] - CAMP_X, r["z_coord"] - CAMP_Z) > CAMP_R:
            raise Refused(f"NPC {name} stands at {r}, not in Zelik's camp")

    def build(self):
        db = self.db
        one(db, "faction Marius and Darius", "SELECT 1 FROM factions WHERE name = ?", (MD,))
        for name in CAMP:
            self.camp_npc(name)
            row = one(db, f"npcfaction {name}", "SELECT * FROM npcfaction WHERE name = ? AND world = -1 AND radius = 0",
                      (name,))
            if len(rows(db, "SELECT id FROM npcfaction WHERE lower(name) = ?", (name,))) != 1:
                raise Refused(f"npcfaction {name}: more than one row")
            self.set("npcfaction", row, {"faction": MD, "note": CAMP_NOTE}, {"faction": HAZINAK},
                     f"2802 {name} (npcfaction {row['id']}) {HAZINAK} -> {MD}")
        for name in ADD:
            self.camp_npc(name)
            got = rows(db, "SELECT * FROM npcfaction WHERE lower(name) = ?", (name,))
            if got:
                if len(got) == 1 and got[0]["faction"] == MD and got[0]["world"] == -1 and got[0]["radius"] == 0:
                    self.lines.append(f"already  2802 {name} of {MD}")
                    continue
                raise Refused(f"npcfaction {name}: {got}, want none")
            self.changes.append(("npcfaction", None, None,
                                 {"name": name, "world": -1, "x": 0.0, "z": 0.0, "radius": 0.0, "faction": MD,
                                  "note": CAMP_NOTE, "source": "petition-2802"}))
            self.lines.append(f"insert   2802 {name}: npcfaction of {MD} (it had none)")
        donor = one(db, f"item {DONOR}", "SELECT itemicon FROM itempattern WHERE patternid = ? AND itemname = ?",
                    (DONOR_ID, DONOR))
        if donor["itemicon"] != BUST:
            raise Refused(f"{DONOR} wears {donor['itemicon']}, not {BUST}")
        row = one(db, f"item {STATUE}", "SELECT * FROM itempattern WHERE patternid = ? AND itemname = ?",
                  (STATUE_ID, STATUE))
        self.set("itempattern", row, {"itemicon": donor["itemicon"]}, {"itemicon": EYE},
                 f"2803 {STATUE} ({STATUE_ID}) icon the gargoyle eye's {EYE:#x} -> {DONOR}'s bust {BUST:#x}")


def audit(db, action, tbl, key, before, after):
    db.execute(AUDIT_DDL)
    cur = db.execute("""INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after,
        detail) VALUES (?, 0, ?, ?, '', ?, ?, ?, ?, ?, ?)""",
                     (now(), WHO, VIA, action, tbl, go_json(key), go_json(before) if before else None,
                      go_json(after) if after else None, DETAIL))
    return cur.lastrowid


def where(key):
    return " AND ".join(f"{k} = ?" for k in key), list(key.values())


def keyof(tbl, row):
    return {k: row[k] for k in KEYS[tbl]}


def apply(db, plan, log):
    db.execute(LOG_DDL.format(log=log))
    for tbl, key, before, values in plan.changes:
        if values is None:  # a delete
            w, args = where(key)
            db.execute(f"DELETE FROM {tbl} WHERE {w}", args)
            a, after = audit(db, "delete", tbl, key, before, None), None
        elif before is None:
            cols = list(values)
            cur = db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                             [values[c] for c in cols])
            key = keyof(tbl, values) if all(k in values for k in KEYS[tbl]) else {KEYS[tbl][0]: cur.lastrowid}
            after = rows(db, f"SELECT * FROM {tbl} WHERE {where(key)[0]}", where(key)[1])[0]
            a = audit(db, "insert", tbl, key, None, after)
        else:
            w, args = where(key)
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in values)} WHERE {w}",
                       list(values.values()) + args)
            after = rows(db, f"SELECT * FROM {tbl} WHERE {w}", args)[0]
            a = audit(db, "update", tbl, key, before, after)
        db.execute(f"INSERT INTO {log} (tbl, ref, before, after, audit, at) VALUES (?, ?, ?, ?, ?, ?)",
                   (tbl, json.dumps(key), json.dumps(before) if before else None,
                    json.dumps(after) if after else None, a, now()))


def undo(db, log):
    out = []
    for r in rows(db, f"SELECT * FROM {log} ORDER BY seq DESC"):
        tbl, key = r["tbl"], json.loads(r["ref"])
        w, args = where(key)
        now_rows = rows(db, f"SELECT * FROM {tbl} WHERE {w}", args)
        if r["after"] is None:  # a delete: the row goes back unless one holds its key now
            if now_rows:
                out.append(f"left     {tbl} {key}: a row holds its key since")
                continue
            before = json.loads(r["before"])
            cols = list(before)
            db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                       [before[c] for c in cols])
            audit(db, "insert", tbl, key, None, before)
            out.append(f"restored {tbl} {key}")
            continue
        after = json.loads(r["after"])
        if len(now_rows) != 1 or now_rows[0] != after:
            out.append(f"left     {tbl} {key}: changed since")
            continue
        if r["before"] is None:
            db.execute(f"DELETE FROM {tbl} WHERE {w}", args)
            audit(db, "delete", tbl, key, after, None)
        else:
            before = json.loads(r["before"])
            cols = list(before)
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in cols)} WHERE {w}",
                       [before[c] for c in cols] + args)
            audit(db, "update", tbl, keyof(tbl, before), after, before)
        out.append(f"restored {tbl} {key}")
    db.execute(f"DROP TABLE {log}")
    return out


def backup(path):
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-pet1123g"
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
    log = LOG
    db = sqlite3.connect(a.db, timeout=5, isolation_level=None)
    db.execute("BEGIN IMMEDIATE")
    try:
        logged = has_table(db, log)
        if a.undo:
            if not logged:
                print("nothing to undo: no log")
                db.execute("ROLLBACK")
                return 0
            if not a.no_backup:
                print("backup", backup(a.db))
            for line in undo(db, log):
                print(line)
            db.execute("COMMIT")
            return 0
        if logged and a.apply:
            print(f"applied already ({log} exists): nothing written")
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
        apply(db, plan, log)
        db.execute("COMMIT")
        print(f"applied: {len(plan.changes)} rows")
        return 0
    except Refused as e:
        db.execute("ROLLBACK")
        print("refused:", e, file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
