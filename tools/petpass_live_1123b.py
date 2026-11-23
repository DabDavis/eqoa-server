#!/usr/bin/env python3
"""Live petitions 5526-5541 of 2026-11-23 (the second pass), as data, for the Live shard.

Every row is found by its NAME (and what else names it), never by an id alone, and must read as it did when this
was made; a row already as this writes it is left ("already"); anything else refuses the whole run.

  5526  Ianant's Chalice drew a gem: it takes the chalice the other chalices draw (brass chalice, stone chalice,
        U'Dedne's Chalice, chalice of the fallen: icon -1742570203).
  5531  the alchemist's thrown potions: those that strike at once draw the blue flask (icon 209763529, as Fulminant
        Potion and the Freeport Cocktail already did), those that burn over time the red one (824019198), each read
        off its itemeffect line ("Decrease HP per tic": over time).
  5541  bark, roots and branches drew the red flask (824019198, the vial the tonics and blood draw): a bark or a
        branch takes Xannasu's branch's strip of bark (339127726), a root the bundle the savory treant root and
        nightworm roots draw (1277509455). Murnf's treants drop gnarled treant bark and the Treant barks and roots.
  5530  Specialization's goblin alchemist step (Master Torlin, the alchemists' level 20) marked the Goblin Alchemist
        in Moradhim's town; the step's place is the one by the goblin camp at Saerk's Tower its journal names
        (queststeps world, x, z), so its mark goes there (handlers/questmarkspots.go: camps within 1,500 of a
        step's place first). Kills count anywhere as before: the target's own name is not held to the place.
  5540  Hobgoblins' vandal step (Oligan, the ogre shamans' level 13) marked the three vandals by Oggok's west wall,
        south of the player; its place is set by the vandal north of them (8268, 5950), and 1,500 from the three,
        so the mark goes north.
  5532  the Glinaldal rangers and druids of Mayfly Glade kept the 90 s respawn of a named creature (petfix259 took a
        capital for a name): their npcrespawn rows go, so they come back as the world's other creatures do.
  5535  the lost froglok (Guk, the level 43 quest's) is level 45, not 40: yellow to a level 43.
  5537  the small stone of elves and the Guise of the Believer (Illusion: Elf) showed a blank buff icon (itemeffect
        icon 0): they take the Illusion: Elf spell's icon (spellpattern 700035's -1569239993, unsigned 2725727303).

Every row written is a gm_audit line (via "tool") and a row of petpass_live_1123b_log; --undo puts each back as it
was (a row changed since is left and said) and drops the log. A report by default; --apply writes once (a second
finds the log and writes nothing).

WHEN IT TAKES EFFECT: item records and item effects at each player's next login or use; quests, npcs and npcrespawn
at the server's next start.

    python3 -E -s tools/petpass_live_1123b.py --db eqoa.db            report
    python3 -E -s tools/petpass_live_1123b.py --db eqoa.db --apply    write it
    python3 -E -s tools/petpass_live_1123b.py --db eqoa.db --undo     take it back
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "petpass_live_1123b_log"
WHO, VIA = "claude", "tool"
DETAIL = "Live petitions 5526-5541 (2026-11-23): tools/petpass_live_1123b.py"

VIAL = 824019198
JUG = -506671519
BLUE = 209763529
CHALICE = -1742570203
GEM = -1731215164
BARK = 339127726
ROOTS = 1277509455
ILLUSION_ICON = 2725727303  # spellpattern "Illusion: Elf" icon -1569239993, as itemeffect keeps it (unsigned)

STRIKE = ["Unstable Potion", "Volatile Potion", "Explosive Potion", "Klik'Anon Cocktail", "Detonating Potion",
          "Frosty Potion", "Grobb Cocktail", "Meteoric Potion", "Icy Potion", "Qeynos Cocktail", "Kinetic Potion",
          "Severupidus' Cocktail", "Ascended Severupidus' Cocktail"]
OVER_TIME = ["Acidic Potion", "Caustic Potion", "Corroding Potion", "Corrosive Potion", "Acerbic Potion",
             "Virulent Potion", "Septic Potion", "Cocktail of Death Potion", "Ascended Cocktail of Death Potion"]
BARKS = ["Fine Arborian Bark", "Fine Deadwood Bark", "Fine Treant Bark", "Growthor's Bark", "Pristine Arborian Bark",
         "Pristine Kolbok Bark", "Pristine Rotwood Bark", "Pristine Treant Bark", "Rotting Treant Bark", "Sapling Bark",
         "gnarled treant bark", "gnarled treant branch"]
ROOT_NAMES = ["Fresh Arborian Root", "Fresh Deadwood Root", "Fresh Fungal Root", "Fresh Fungusman Root",
              "Fresh Treant Root", "Fungal Root", "Pristine Arborian Root", "Pristine Fungal Root",
              "Pristine Fungusman Root", "Pristine Kolbok Root", "Pristine Rotwood Root", "Pristine Treant Root",
              "Sapling Root", "Tilmiraen's Root", "Wailing Root", "Wailing Tree Root", "Withered Treant Root",
              "bitter roots", "weak roots"]
# quest name, giver, step, kind, target, (world, x, z)
PLACES = [("5530", "Specialization", "Master Torlin", 1, "loot", "Goblin Alchemist", (0, 20240.0, 13531.0)),
          ("5540", "Hobgoblins", "Oligan", 2, "loot", "hobgoblin vandal", (1, 8250.0, 5850.0))]
GLINALDAL = ("Glinaldal ranger", "Glinaldal druid")
FROGLOK = ("the lost froglok", 40, 45)
ILLUSION_ITEMS = ["small stone of elves", "Guise of the Believer"]

KEYS = {"itempattern": ("patternid",), "itemeffect": ("patternid",), "queststeps": ("questid", "step"),
        "npcs": ("id",), "npcrespawn": ("npc_row",)}
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
        self.changes = []  # (tbl, key dict, before row or None, values or None: a delete)
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

    def delete(self, tbl, row, what):
        key = {k: row[k] for k in KEYS[tbl]}
        self.changes.append((tbl, key, dict(row), None))
        self.lines.append(f"delete   {what}")

    def item(self, name):
        return one(self.db, f"the record {name!r}", "SELECT * FROM itempattern WHERE itemname = ?", (name,))

    def icon(self, pet, name, old, new, why):
        r = self.item(name)
        self.set("itempattern", r, {"itemicon": new}, {"itemicon": old}, f"{pet} {name!r}'s icon ({why})")

    def build(self):
        db = self.db
        self.icon("5526", "Ianant's Chalice", GEM, CHALICE, "a chalice")
        for names, icon, why in ((STRIKE, BLUE, "strikes at once: the blue flask"),
                                 (OVER_TIME, VIAL, "burns over time: the red flask")):
            for name in names:
                r = one(db, f"the thrown potion {name!r}", "SELECT * FROM itempattern WHERE itemname = ? AND "
                        "equipslot = 16", (name,))
                eff = one(db, f"{name!r}'s effect", "SELECT line FROM itemeffect WHERE patternid = ?", (r["patternid"],))
                if ("HP per tic" in eff["line"]) != (icon == VIAL):
                    raise Refused(f"5531 {name!r}: its line {eff['line']!r} does not read as {why}")
                if r["equipslot"] != 16 or not r["classuse"] & (1 << 14):
                    raise Refused(f"5531 {name!r} is not an alchemist's thrown potion")
                self.set("itempattern", r, {"itemicon": icon}, {"itemicon": JUG}, f"5531 {name!r}'s icon ({why})")
        for name in BARKS:
            self.icon("5541", name, VIAL, BARK, "bark")
        for name in ROOT_NAMES:
            self.icon("5541", name, VIAL, ROOTS, "roots")
        for pet, qname, giver, step, kind, target, (world, x, z) in PLACES:
            st = one(db, f"{qname}'s step {step}", """SELECT s.* FROM queststeps s JOIN quests q ON q.questid = s.questid
                WHERE q.name = ? AND q.giver = ? AND s.step = ? AND s.kind = ? AND s.target = ?""",
                     (qname, giver, step, kind, target))
            self.set("queststeps", st, {"world": world, "x": x, "z": z}, {"world": 0, "x": 0.0, "z": 0.0},
                     f"{pet} {qname!r} step {step} ({target}) placed")
        for name in GLINALDAL:
            got = rows(db, """SELECT r.* FROM npcrespawn r JOIN npcs n ON n.id = r.npc_row WHERE n.npc_name = ?
                ORDER BY r.npc_row""", (name,))
            if not got:
                self.lines.append(f"already  5532 {name}: no respawn rows")
            for r in got:
                if not near(r["seconds"], 90.0) or r["source"] != "petfix259":
                    raise Refused(f"5532 {name} row {r['npc_row']}: {r['seconds']} s from {r['source']!r}")
                self.delete("npcrespawn", r, f"5532 {name} row {r['npc_row']}'s 90 s respawn")
        name, old, new = FROGLOK
        fr = one(db, name, "SELECT * FROM npcs WHERE npc_name = ? AND world = 0", (name,))
        self.set("npcs", fr, {"npc_level": new}, {"npc_level": old}, f"5535 {name}'s level")
        for name in ILLUSION_ITEMS:
            e = one(db, f"{name!r}'s effect", """SELECT e.* FROM itemeffect e JOIN itempattern p ON
                p.patternid = e.patternid WHERE p.itemname = ? AND e.name = 'Illusion: Elf'""", (name,))
            self.set("itemeffect", e, {"icon": ILLUSION_ICON}, {"icon": 0}, f"5537 {name!r}'s buff icon")


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
        w, args = where(key)
        if values is None:
            db.execute(f"DELETE FROM {tbl} WHERE {w}", args)
            a = audit(db, "delete", tbl, key, before, None)
            after = None
        else:
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in values)} WHERE {w}",
                       list(values.values()) + args)
            after = rows(db, f"SELECT * FROM {tbl} WHERE {w}", args)[0]
            a = audit(db, "update", tbl, key, before, after)
        db.execute(f"INSERT INTO {LOG} (tbl, ref, before, after, audit, at) VALUES (?, ?, ?, ?, ?, ?)",
                   (tbl, json.dumps(key), json.dumps(before), json.dumps(after) if after else None, a, now()))


def undo(db):
    out = []
    for r in rows(db, f"SELECT * FROM {LOG} ORDER BY seq DESC"):
        tbl, key = r["tbl"], json.loads(r["ref"])
        w, args = where(key)
        now_rows = rows(db, f"SELECT * FROM {tbl} WHERE {w}", args)
        before = json.loads(r["before"])
        if r["after"] is None:  # a delete: put the row back
            if now_rows:
                out.append(f"left     {tbl} {key}: a row is there again")
                continue
            cols = list(before)
            db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                       [before[c] for c in cols])
            audit(db, "insert", tbl, key, None, before)
        else:
            after = json.loads(r["after"])
            if len(now_rows) != 1 or now_rows[0] != after:
                out.append(f"left     {tbl} {key}: changed since")
                continue
            cols = [c for c in before if c not in key]
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in cols)} WHERE {w}",
                       [before[c] for c in cols] + args)
            audit(db, "update", tbl, key, after, before)
        out.append(f"restored {tbl} {key}")
    db.execute(f"DROP TABLE {LOG}")
    return out


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, timeout=5, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 5000")
    db.execute("BEGIN IMMEDIATE")
    try:
        logged = db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (LOG,)).fetchone()
        if a.undo:
            if not logged:
                print("nothing to undo: no log")
                db.execute("ROLLBACK")
                return 0
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
        apply(db, plan)
        db.execute("COMMIT")
        print(f"applied: {len(plan.changes)} rows, a gm_audit line each, logged in {LOG}")
        return 0
    except Refused as e:
        db.execute("ROLLBACK")
        print("refused:", e, file=sys.stderr)
        return 2
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
