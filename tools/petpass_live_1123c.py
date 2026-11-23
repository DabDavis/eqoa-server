#!/usr/bin/env python3
"""Live petitions 5543-5546 of 2026-11-23, as data, for the Live shard: the Shadowknight exemplar's camp of the
paladins of Rodcet Nife (quest 101340 "Screaming Souls", EQOA-Data Quests/TankExemplar/shadowknight.json, AoE Strike
Path: "slay Healers and Paladins of Rodcet Nife at a campfire north of Sunnyvale"). The owner approved each, 2026-11-23.

  5544  the paladins wear a cleric's robe (robe 0) and no armour: they take plate on all six slots (armour set 4,
        CHARCUST's plate, the plate helm too; npcs torso..head) and no robe (robe_style 255).
  5545  the paladins are a little tougher: template "rodcet nife paladin" (classtemplates; their own class, so their
        melee is as before) at 130% of their level's hit points (13,502 at 58 -> 17,553).
  5543  clerics of Rodcet Nife beside them, half as many: four npcs rows, each a copy of the paladin row nearest its
        spot (level 58, the human body, the same faction: none, as the paladins have) in the robe the paladins wore
        (robe 0), one by each group of paladins, at the ground's height there (TUNARIA's baked ground, read for this
        tool). Template "rodcet nife cleric": a cleric (class 9) that heals the worst off of its kin within 30 by 15%
        of its own hit points every 15 s (as the kin healers', petition 5322), in a fight or standing by while one of
        its kin fights, and casts a cleric's nuke of its level every 12 s at 35% (as the guide camps' clerics). Its
        kin is a social row's faction: both names get an npcsocial row of faction "Followers of Rodcet Nife" with
        social 0, so neither comes to anyone's help in a fight (as before), but a cleric's heal finds the paladins.
        npcclass names the cleric a cleric for the stats line.
  5546  the Screaming Soul drops at 35% (it was 75%): quest 101340 step 1's chance, the only source of the item (no
        loot table gives it; the step is its one drop). The clerics drop it too (queststeptargets: the guide's
        "Healers and Paladins"), the journal and the item's description say so, and the quest's notes.

Not here: 5542 (the camp's place and its campfires) waits for the player's own reports from the spots.

Every row is found by its name and place and must read as it did when this was made; a row already as this writes
it is left ("already"); anything else refuses the whole run. Every row written is a gm_audit line (via "tool") and a
row of petpass_live_1123c_log; --undo puts each back as it was (an inserted row is taken out, said so when it changed
since) and the npcs sequence back, and drops the log. A report by default; --apply writes once (a second finds the
log and writes nothing).

WHEN IT TAKES EFFECT: the item's description at each player's next login; everything else (npcs rows, templates,
npcclass, npcsocial, quests) at the server's next start.

    python3 -E -s tools/petpass_live_1123c.py --db eqoa.db            report
    python3 -E -s tools/petpass_live_1123c.py --db eqoa.db --apply    write it
    python3 -E -s tools/petpass_live_1123c.py --db eqoa.db --undo     take it back
"""
import argparse
import datetime
import json
import math
import sqlite3
import sys

LOG = "petpass_live_1123c_log"
WHO, VIA = "claude", "tool"
DETAIL = "Live petitions 5543-5546 (2026-11-23): the paladins of Rodcet Nife's camp: tools/petpass_live_1123c.py"

WORLD = 0
CAMP = (12460.0, 15240.0)  # the paladins' camp north of Sunnyvale
CAMP_R = 150.0
PALADIN = "paladin of Rodcet Nife"
CLERIC = "cleric of Rodcet Nife"
PALADINS = 8
LOOK = ("torso", "forearm", "gloves", "legs", "feet", "head")
PLATE = 4  # $Armor set 4, plate
NO_ROBE = "255"
ROBE = "0"  # the robe the paladins wore, the clerics' now
FACTION = "Followers of Rodcet Nife"

# the clerics' spots: one by each group of paladins, the height TUNARIA's baked ground's there (floor_near)
CLERIC_SPOTS = [(12439.5, 54.15625, 15203.0),   # the three at the fire
                (12454.5, 56.68084, 15226.5),   # the two on the rise
                (12441.5, 58.19254, 15238.0),   # the one west
                (12470.0, 64.27319, 15265.5)]   # the one up the slope
SPACING = 2.0  # no other NPC within this of a spot

PAL_TEMPLATE = {
    "name": "rodcet nife paladin", "class": -1, "hp_pct": 130, "taken_pct": 100, "melee_pct": 100,
    "abilities": "[]",
    "note": "Live petition 5545 (the owner, 2026-11-23): the paladins of Rodcet Nife a little tougher: 130% of their "
            "level's hit points, their own class and melee: tools/petpass_live_1123c.py",
    "source": "petpass-1123c"}
CLERIC_ABILITIES = [{"kind": "heal", "every": 15, "first": 2, "heal_pct": 15, "below": 85, "target": "kin"},
                    {"kind": "nuke", "every": 12, "first": 4, "damage_pct": 35, "max_recast": 10, "max_cast": 5}]
CLERIC_TEMPLATE = {
    "name": "rodcet nife cleric", "class": 9, "hp_pct": 100, "taken_pct": 100, "melee_pct": 100,
    "abilities": json.dumps(CLERIC_ABILITIES, separators=(",", ":")),
    "note": "Live petition 5543 (the owner, 2026-11-23): the clerics of Rodcet Nife heal their kin (the paladins of "
            "the camp, by their npcsocial faction) 15% of their own hit points every 15 s, as the kin healers, and "
            "nuke as the guide camps' clerics: tools/petpass_live_1123c.py",
    "source": "petpass-1123c"}

QUEST, STEP, ITEM = 101340, 1, 701530
CHANCE_OLD, CHANCE_NEW = 0.75, 0.35
JOURNAL_OLD = "Kill the paladins of Rodcet Nife at the campfire north of Sunnyvale for ten screaming souls."
JOURNAL_NEW = ("Kill the paladins and clerics of Rodcet Nife at the campfire north of Sunnyvale for ten screaming "
               "souls.")
DESC_OLD = "A skull that screams, from a paladin of Rodcet Nife."
DESC_NEW = "A skull that screams, from a paladin or cleric of Rodcet Nife."
NOTES_OLD = "only the paladins of the camp are in (the guide's healers too would be a second quest)"
NOTES_NEW = ("the camp's paladins and its clerics (the guide's healers, Live petition 5543) both drop the souls, at "
             "35% a kill (Live petition 5546: a chore, as the guide has it)")

KEYS = {"npcs": ("id",), "classtemplates": ("name",), "npctemplate": ("name",), "npcclass": ("name",),
        "npcsocial": ("npc_name",), "queststeps": ("questid", "step"), "queststeptargets": ("questid", "step", "target"),
        "itempattern": ("patternid",), "quests": ("questid",)}
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


def same(a, b):
    if isinstance(b, float) or isinstance(a, float):
        try:
            return abs(float(a) - float(b)) < 1e-4
        except (TypeError, ValueError):
            return False
    return a == b


def has_table(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone() is not None


class Plan:
    def __init__(self, db):
        self.db = db
        self.changes = []  # ("update", tbl, key, before, values) or ("insert", tbl, None, None, values)
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
        self.changes.append(("update", tbl, key, dict(row), dict(values)))
        self.lines.append(f"change   {what}: " + ", ".join(f"{c} {row[c]!r} -> {v!r}" for c, v in values.items()))

    def insert(self, tbl, values, what, check=None):
        """a new row, unless one of its key is there: then as values (check: the columns that must agree) is
        already, anything else refuses."""
        if tbl != "npcs":
            key = {k: values[k] for k in KEYS[tbl]}
            w, args = where(key, lower=True)
            got = rows(self.db, f"SELECT * FROM {tbl} WHERE {w}", args)
            if got:
                cols = check or [c for c in values if c not in ("note", "source")]
                if len(got) == 1 and all(same(got[0][c], values[c]) for c in cols):
                    self.lines.append(f"already  {what}")
                    return
                raise Refused(f"{what}: {tbl} already has {got}")
        self.changes.append(("insert", tbl, None, None, dict(values)))
        self.lines.append(f"insert   {what}")

    def build(self):
        db = self.db
        pals = rows(db, "SELECT * FROM npcs WHERE npc_name = ? AND world = ? ORDER BY id", (PALADIN, WORLD))
        if len(pals) != PALADINS:
            raise Refused(f"{len(pals)} {PALADIN} rows in world {WORLD}, want {PALADINS}")
        for p in pals:
            if math.hypot(p["x_coord"] - CAMP[0], p["z_coord"] - CAMP[1]) > CAMP_R:
                raise Refused(f"{PALADIN} row {p['id']} at ({p['x_coord']:.0f}, {p['z_coord']:.0f}): not in the camp")
        # 5543: the clerics, copies of the paladin row nearest each spot as the paladins are now (robe 0, no armour)
        clerics = rows(db, "SELECT id FROM npcs WHERE npc_name = ?", (CLERIC,))
        if clerics:
            raise Refused(f"{CLERIC} rows already: {[r['id'] for r in clerics]}")
        others = rows(db, "SELECT id, npc_name, x_coord, y_coord, z_coord FROM npcs WHERE world = ? AND "
                      "x_coord BETWEEN ? AND ? AND z_coord BETWEEN ? AND ?",
                      (WORLD, CAMP[0] - 2 * CAMP_R, CAMP[0] + 2 * CAMP_R, CAMP[1] - 2 * CAMP_R, CAMP[1] + 2 * CAMP_R))
        for i, (x, y, z) in enumerate(CLERIC_SPOTS):
            for o in others:
                if math.hypot(o["x_coord"] - x, o["z_coord"] - z) < SPACING:
                    raise Refused(f"cleric spot {i + 1} ({x}, {z}): {o['npc_name']} row {o['id']} stands there")
            src = min(pals, key=lambda p: math.hypot(p["x_coord"] - x, p["z_coord"] - z))
            if any(src[c] != 0 for c in LOOK) or str(src["robe_style"]) != ROBE:
                raise Refused(f"{PALADIN} row {src['id']}: its look is not as read (robe 0, no armour)")
            new = {c: v for c, v in src.items() if c != "id"}
            new.update(npc_name=CLERIC, x_coord=x, y_coord=y, z_coord=z, robe_style=ROBE, **{c: 0 for c in LOOK})
            self.insert("npcs", new, f"5543 {CLERIC} at ({x}, {y}, {z}), a copy of row {src['id']}, facing {src['facing']}")
        # 5544: the paladins' plate, no robe
        for p in pals:
            self.set("npcs", p, {**{c: PLATE for c in LOOK}, "robe_style": NO_ROBE},
                     {**{c: 0 for c in LOOK}, "robe_style": ROBE}, f"5544 {PALADIN} row {p['id']}: plate, no robe")
        # 5545 + 5543: the templates and who fights by them
        self.insert("classtemplates", PAL_TEMPLATE, f"5545 template {PAL_TEMPLATE['name']!r} (hp_pct 130)")
        self.insert("classtemplates", CLERIC_TEMPLATE, f"5543 template {CLERIC_TEMPLATE['name']!r} (a cleric, kin heal)")
        for name, t in ((PALADIN, PAL_TEMPLATE), (CLERIC, CLERIC_TEMPLATE)):
            self.insert("npctemplate", {"name": name.lower(), "template": t["name"],
                                        "note": f"Live petitions 5543/5545 (2026-11-23): tools/petpass_live_1123c.py",
                                        "source": "petpass-1123c"}, f"npctemplate {name} -> {t['name']!r}", ["template"])
            self.insert("npcsocial", {"npc_name": name.lower(), "social": 0, "faction": FACTION, "radius": 0.0,
                                      "note": "Live petition 5543: of the camp's kin for the clerics' heal, helps no "
                                              "one: tools/petpass_live_1123c.py", "source": "petpass-1123c"},
                        f"npcsocial {name}: of {FACTION!r}, social 0", ["social", "faction"])
        self.insert("npcclass", {"name": CLERIC.lower(), "class": 9, "source": "petpass-1123c"},
                    f"npcclass {CLERIC}: a cleric", ["class"])
        # 5546: the drop
        st = one(db, f"quest {QUEST} step {STEP}", """SELECT s.* FROM queststeps s JOIN quests q ON q.questid = s.questid
            WHERE s.questid = ? AND s.step = ? AND q.name = 'Screaming Souls' AND s.kind = 'loot' AND s.target = ?""",
                 (QUEST, STEP, PALADIN))
        if st["items"] != f"{ITEM}*10":
            raise Refused(f"quest {QUEST} step {STEP}'s items read {st['items']!r}")
        self.set("queststeps", st, {"chance": CHANCE_NEW, "journal": JOURNAL_NEW},
                 {"chance": CHANCE_OLD, "journal": JOURNAL_OLD}, f"5546 quest {QUEST} step {STEP}: 35%, the clerics")
        self.insert("queststeptargets", {"questid": QUEST, "step": STEP, "target": CLERIC},
                    f"5543 quest {QUEST} step {STEP} counts the {CLERIC}", [])
        it = one(db, f"item {ITEM}", "SELECT * FROM itempattern WHERE patternid = ? AND itemname = 'Screaming Soul'",
                 (ITEM,))
        self.set("itempattern", it, {"itemdesc": DESC_NEW}, {"itemdesc": DESC_OLD}, "5543 the Screaming Soul's text")
        q = one(db, f"quest {QUEST}", "SELECT * FROM quests WHERE questid = ?", (QUEST,))
        if NOTES_NEW in q["notes"]:
            self.lines.append(f"already  quest {QUEST}'s notes")
        elif NOTES_OLD not in q["notes"]:
            raise Refused(f"quest {QUEST}'s notes do not read as when this was made")
        else:
            self.set("quests", q, {"notes": q["notes"].replace(NOTES_OLD, NOTES_NEW)}, {}, f"quest {QUEST}'s notes")
        sources = rows(db, "SELECT tablename FROM loottables WHERE patternid = ?", (ITEM,))
        others = rows(db, "SELECT questid, step FROM queststeps WHERE (items LIKE ? OR rewarditems LIKE ?) AND "
                      "NOT (questid = ? AND step IN (1, 2))", (f"%{ITEM}%", f"%{ITEM}%", QUEST))
        if sources or others:
            raise Refused(f"item {ITEM} has other sources now: loot tables {sources}, quest steps {others}")


def audit(db, action, tbl, key, before, after):
    db.execute(AUDIT_DDL)
    cur = db.execute("""INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after,
        detail) VALUES (?, 0, ?, ?, '', ?, ?, ?, ?, ?, ?)""",
                     (now(), WHO, VIA, action, tbl, go_json(key), go_json(before) if before else None,
                      go_json(after) if after else None, DETAIL))
    return cur.lastrowid


def where(key, lower=False):
    parts, args = [], []
    for k, v in key.items():
        if lower and isinstance(v, str):
            parts.append(f"lower({k}) = lower(?)")
        else:
            parts.append(f"{k} = ?")
        args.append(v)
    return " AND ".join(parts), args


def npcs_seq(db):
    r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'npcs'").fetchone()
    return r[0] if r else None


def apply(db, plan):
    db.execute(LOG_DDL)
    seq = npcs_seq(db)
    db.execute(f"INSERT INTO {LOG} (tbl, ref, before, after, audit, at) VALUES ('sqlite_sequence', ?, ?, NULL, NULL, ?)",
               (json.dumps({"name": "npcs"}), json.dumps({"seq": seq}), now()))
    for op, tbl, key, before, values in plan.changes:
        if op == "insert":
            cols = list(values)
            cur = db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                             [values[c] for c in cols])
            key = {"id": cur.lastrowid} if tbl == "npcs" else {k: values[k] for k in KEYS[tbl]}
            w, args = where(key)
            after = rows(db, f"SELECT * FROM {tbl} WHERE {w}", args)[0]
            a = audit(db, "insert", tbl, key, None, after)
            before = None
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
    seq_row = None
    for r in rows(db, f"SELECT * FROM {LOG} ORDER BY seq DESC"):
        tbl, key = r["tbl"], json.loads(r["ref"])
        if tbl == "sqlite_sequence":
            seq_row = json.loads(r["before"])["seq"]
            continue
        w, args = where(key)
        now_rows = rows(db, f"SELECT * FROM {tbl} WHERE {w}", args)
        after = json.loads(r["after"])
        if r["before"] is None:  # an insert: take it out
            if not now_rows:
                out.append(f"gone     {tbl} {key}: deleted since")
                continue
            db.execute(f"DELETE FROM {tbl} WHERE {w}", args)
            audit(db, "delete", tbl, key, now_rows[0], None)
            out.append(f"deleted  {tbl} {key}" + ("" if now_rows[0] == after else " (it had changed since)"))
            continue
        before = json.loads(r["before"])
        if len(now_rows) != 1 or now_rows[0] != after:
            out.append(f"left     {tbl} {key}: changed since")
            continue
        cols = [c for c in before if c not in key]
        db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in cols)} WHERE {w}",
                   [before[c] for c in cols] + args)
        audit(db, "update", tbl, key, after, before)
        out.append(f"restored {tbl} {key}")
    if seq_row is not None:
        top = db.execute("SELECT max(id) FROM npcs").fetchone()[0] or 0
        if top <= seq_row:
            db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = 'npcs'", (seq_row,))
            out.append(f"npcs sequence back to {seq_row}")
        else:
            out.append(f"npcs sequence left: a row {top} was made since")
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
        logged = has_table(db, LOG)
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
        print(f"applied: {len(plan.changes)} rows, a gm_audit line each, logged in {LOG}; in force at the next "
              "start (the item's text at each player's next login)")
        return 0
    except Refused as e:
        db.execute("ROLLBACK")
        print("refused:", e, file=sys.stderr)
        return 2
    except BaseException:
        db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
