#!/usr/bin/env python3
"""Live petitions 5549, 5555, 5556/5557 and 5565 of 2026-11-23 and the Black Sapphire's icon, as data, for the Live shard
(the owner's yes to all, 2026-11-23).

  5549  a test robe for a tank asked by name, of which no record exists: a level 60 robe a warrior, paladin or shadow
        knight wears, NEW_ROBE, made on the strongest robe those three wear (Khal Robe of War, 1027: AC 100, HP 150,
        STA/WIS/INT/CHA 20 at level 55) at level 60 (AC 120, HP 180, those stats 24) and with its look (model 0,
        colour 65535) and icon: an estimate, no figure of the live game's being known. LORE, NO TRADE as 1027. The
        give is a GM grant of its own (an approved transfer), not this tool.
  5555  the enchanter's animations and golems (the 13 summonings of the pet templates "pet animation", "pet gymon's
        animation", "pet earthen golem" and "pet ferrous golem", all of them workbook pets) hit at 200%: petstats'
        melee_pct, a column the server adds at its next start (database/petstats.go) and this tool adds when it is
        not there yet. A workbook pet takes none of its template's melee_pct, so the templates are left. Simulated
        (cmd/balancesim -pets -petalone, an even con, 40 fights): the time to kill at owner levels 30/50/60 from
        142/164/243 s to 73/82/121 s, the target about 85 s at level 50.
  5556  the paladin level 49 epic's camp: The Unnamed King to 250,000 hit points and Tolo Grimsteel and Ganron
  5557  Lorebane to 150,000 each, Parathior to 300,000 and 150% melee, each by a class template of its own made from
        the roamer template they shared (a level 52 roamer has 10,982: 52 x 60 x balancelevel's 352%), so no other
        roamer changes: hp_pct 2276 (249,950), 1366 (150,014) and 2732 (300,028).
  5565  a creature below its foe's level hits it for less: the con table's rows -1 to -5 at 90, 80, 70, 60 and 50%
        of the even row's damage, -5 for every blue and green below it (Balance.Con: below the lowest row, the
        lowest), their other columns as the even row's.
  ---   the Black Sapphire (704208) takes Onyx's dark gem icon (25640's), as on Heirloom.

Every row must read as Live's did when this was made, or as this writes it ("already"); anything else refuses the
whole run. Every row written is a gm_audit line (via "tool") and a row of petpass_live_1123e_log; --undo puts each
back as it was (an inserted row taken out) and drops the log; the petstats column it may have added stays (the
server adds it anyway). A report by default; --apply writes once (a second finds the log and writes nothing).

WHEN IT TAKES EFFECT: the robe and the icon at each player's next login; the templates, the con rows and the pets'
melee at the server's next start (the pets' melee needs the code of the same change too).

    python3 -E -s tools/petpass_live_1123e.py --db eqoa.db            report
    python3 -E -s tools/petpass_live_1123e.py --db eqoa.db --apply    write it
    python3 -E -s tools/petpass_live_1123e.py --db eqoa.db --undo     take it back
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "petpass_live_1123e_log"
WHO, VIA = "claude", "tool"
DETAIL = "Live petitions 5549/5555/5556/5557/5565 + Black Sapphire icon (2026-11-23): tools/petpass_live_1123e.py"

# 5549
ROBE_BASE, ROBE_BASE_NAME = 1027, "Khal Robe of War"
NEW_ROBE, NEW_ROBE_NAME = 1000274, "Nightmare Tank Robe"
ROBE = {"itemname": NEW_ROBE_NAME, "levelreq": 60, "classuse": 13, "AC": 120, "HPMAX": 180, "sta": 24, "wis": 24,
        "intelligence": 24, "cha": 24, "itemcost": 5150, "lore": 1, "trade": 1,
        "itemdesc": "Woven from the coils of the Nightmare, it turns aside blows meant for others."}
ROBE_BASE_READS = {"itemname": ROBE_BASE_NAME, "equipslot": 2, "classuse": 13, "levelreq": 55, "AC": 100,
                   "HPMAX": 150, "model": 0, "color": 65535, "itemicon": 1188988948, "lore": 1, "trade": 1}

# 5555
ENC_PETS = {498: "Pendril's Animation", 510: "Mircyl's Animation", 526: "Shalee's Animation",
            700891: "Gymon's Animation", 542: "Clay Golem", 700929: "Earthen Golem", 566: "Stone Golem",
            700973: "Ferrous Golem", 586: "Iron Golem", 591: "Blade Swarm", 700732: "Steel Tempest",
            700819: "Storm of Blades", 701263: "Grand Animation"}
PET_MELEE = 200

# 5556/5557
ROAMER = "roamer"
EPIC_TEMPLATES = [  # (template, hp_pct, melee_pct, the NPC names it is given)
    ("epic the unnamed king", 2276, 100, ["the unnamed king"]),
    ("epic the unnamed king's companions", 1366, 100, ["tolo grimsteel", "ganron lorebane"]),
    ("epic parathior", 2732, 150, ["parathior"]),
]
EPIC_NOTE = ("The paladin level 49 epic's {who} (Live petitions 5556/5557, the owner's yes 2026-11-23: 'very under "
             "tuned and can be solod'): the roamer template at hp_pct {hp} (about {total:,} hit points at level 52){melee}")

# 5565
CON_ROWS = {-1: 90.0, -2: 80.0, -3: 70.0, -4: 60.0, -5: 50.0}
CON_NOTE = "a creature below its foe hits it for less (Live petition 5565, the owner's yes 2026-11-23){more}"

# the Black Sapphire
SAPPHIRE, SAPPHIRE_NAME, GREEN_GEM = 704208, "Black Sapphire", 140265582
ONYX, ONYX_NAME, DARK_GEM = 25640, "Onyx", -1071168822

KEYS = {"itempattern": ("patternid",), "classtemplates": ("name",), "npctemplate": ("name",),
        "balancecon": ("diff",), "petstats": ("spellid",)}
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


def where(key):
    return " AND ".join(f"{k} = ?" for k in key), list(key.values())


def has_table(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone() is not None


def same(a, b):
    if isinstance(a, float) or isinstance(b, float):
        try:
            return abs(float(a) - float(b)) < 1e-4
        except (TypeError, ValueError):
            return False
    return a == b


class Plan:
    def __init__(self, db):
        self.db = db
        self.changes = []  # (op, tbl, key, before row, values)
        self.lines = []

    def get(self, tbl, key):
        w, args = where(key)
        got = rows(self.db, f"SELECT * FROM {tbl} WHERE {w}", args)
        if len(got) > 1:
            raise Refused(f"{tbl} {key}: {len(got)} rows")
        return got[0] if got else None

    def set(self, tbl, key, old, new, what):
        """new over the row of key, when it reads old; already as new: left."""
        row = self.get(tbl, key)
        if row is None:
            raise Refused(f"{what}: no {tbl} row {key}")
        if all(same(row[c], v) for c, v in new.items()):
            self.lines.append(f"already  {what}")
            return
        for c, v in old.items():
            if not same(row[c], v):
                raise Refused(f"{what}: {c} reads {row[c]!r}, not {v!r} as read when this was made")
        vals = {c: v for c, v in new.items() if not same(row[c], v)}
        self.changes.append(("update", tbl, key, row, vals))
        self.lines.append(f"change   {what}: " + ", ".join(vals))

    def insert(self, tbl, key, values, what):
        row = self.get(tbl, key)
        if row is not None:
            if all(same(row[c], v) for c, v in values.items()):
                self.lines.append(f"already  {what}")
                return
            raise Refused(f"{what}: {tbl} already has {row}")
        self.changes.append(("insert", tbl, key, None, {**key, **values}))
        self.lines.append(f"insert   {what}")

    def delete(self, tbl, key, old, what):
        row = self.get(tbl, key)
        if row is None:
            self.lines.append(f"already  {what} (gone)")
            return
        for c, v in old.items():
            if not same(row[c], v):
                raise Refused(f"{what}: {c} reads {row[c]!r}, not {v!r}")
        self.changes.append(("delete", tbl, key, row, None))
        self.lines.append(f"delete   {what}")

    def build(self):
        # 5549: the robe, made on the strongest robe a tank wears
        base = self.get("itempattern", {"patternid": ROBE_BASE})
        if base is None:
            raise Refused(f"no itempattern {ROBE_BASE}")
        for c, v in ROBE_BASE_READS.items():
            if not same(base[c], v):
                raise Refused(f"{ROBE_BASE_NAME} ({ROBE_BASE}): {c} reads {base[c]!r}, not {v!r}")
        robe = {c: v for c, v in base.items() if c != "patternid"}
        robe.update(ROBE)
        self.insert("itempattern", {"patternid": NEW_ROBE}, robe,
                    f"5549 {NEW_ROBE_NAME} ({NEW_ROBE}): level 60, AC 120, HP 180, STA/WIS/INT/CHA 24, "
                    f"{ROBE_BASE_NAME}'s look")
        # 5555: the enchanter's pets' melee
        for sid, name in ENC_PETS.items():
            self.set("petstats", {"spellid": sid}, {"name": name, "melee_pct": 100}, {"melee_pct": PET_MELEE},
                     f"5555 {name} ({sid}): melee {PET_MELEE}%")
        # 5556/5557: the epic's camp, templates of their own
        roamer = self.get("classtemplates", {"name": ROAMER})
        if roamer is None or roamer["hp_pct"] != 100 or roamer["melee_pct"] != 100 or roamer["special"] != "Critical Hit":
            raise Refused(f"the roamer template reads {roamer}")
        for tmpl, hp, melee, names in EPIC_TEMPLATES:
            who = " and ".join(n.title() for n in names)
            row = {c: v for c, v in roamer.items() if c != "name"}
            row.update({"hp_pct": hp, "melee_pct": melee, "source": "gm",
                        "note": EPIC_NOTE.format(who=who, hp=hp, total=round(10982 * hp / 100),
                                                 melee=f", melee_pct {melee}" if melee != 100 else "")})
            self.insert("classtemplates", {"name": tmpl}, row, f"5556/5557 template {tmpl!r}: hp_pct {hp}, melee {melee}")
            for n in names:
                self.set("npctemplate", {"name": n}, {"template": ROAMER}, {"template": tmpl, "source": "gm",
                         "note": f"named, level 52; the paladin 49 epic (Live petitions 5556/5557)"},
                         f"5556/5557 {n!r} fights by {tmpl!r}")
        # 5565: the con table below even
        even = self.get("balancecon", {"diff": 0})
        if even is None or even["damage_pct"] != 100:
            raise Refused(f"balancecon's even row reads {even}")
        for diff, dmg in CON_ROWS.items():
            more = "; and every blue and green below it" if diff == min(CON_ROWS) else ""
            self.insert("balancecon", {"diff": diff}, {"damage_pct": dmg, "taken_pct": even["taken_pct"],
                        "root_resist_pct": even["root_resist_pct"], "root_break_pct": even["root_break_pct"],
                        "note": CON_NOTE.format(more=more), "source": "gm"},
                        f"5565 con {diff}: a creature's melee {dmg:g}%")
        # the Black Sapphire's icon, Onyx's
        onyx = self.get("itempattern", {"patternid": ONYX})
        if onyx is None or onyx["itemname"] != ONYX_NAME or onyx["itemicon"] != DARK_GEM:
            raise Refused(f"{ONYX_NAME} ({ONYX}) reads {onyx and (onyx['itemname'], onyx['itemicon'])}")
        self.set("itempattern", {"patternid": SAPPHIRE}, {"itemname": SAPPHIRE_NAME, "itemicon": GREEN_GEM},
                 {"itemicon": DARK_GEM}, f"{SAPPHIRE_NAME} ({SAPPHIRE}): {ONYX_NAME}'s icon")


def audit(db, action, tbl, key, before, after):
    db.execute(AUDIT_DDL)
    cur = db.execute("""INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after,
        detail) VALUES (?, 0, ?, ?, '', ?, ?, ?, ?, ?, ?)""",
                     (now(), WHO, VIA, action, tbl, go_json(key), go_json(before) if before else None,
                      go_json(after) if after else None, DETAIL))
    return cur.lastrowid


def apply(db, plan):
    db.execute(LOG_DDL)
    for op, tbl, key, before, values in plan.changes:
        w, args = where(key)
        if op == "insert":
            cols = list(values)
            db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                       [values[c] for c in cols])
            after = rows(db, f"SELECT * FROM {tbl} WHERE {w}", args)[0]
        elif op == "update":
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in values)} WHERE {w}",
                       list(values.values()) + args)
            after = rows(db, f"SELECT * FROM {tbl} WHERE {w}", args)[0]
        else:
            db.execute(f"DELETE FROM {tbl} WHERE {w}", args)
            after = None
        a = audit(db, op, tbl, key, before, after)
        db.execute(f"INSERT INTO {LOG} (tbl, ref, before, after, audit, at) VALUES (?, ?, ?, ?, ?, ?)",
                   (tbl, json.dumps(key), json.dumps(before) if before else None,
                    json.dumps(after) if after else None, a, now()))


def undo(db):
    out = []
    for r in rows(db, f"SELECT * FROM {LOG} ORDER BY seq DESC"):
        tbl, key = r["tbl"], json.loads(r["ref"])
        w, args = where(key)
        cur = rows(db, f"SELECT * FROM {tbl} WHERE {w}", args)
        before = json.loads(r["before"]) if r["before"] else None
        after = json.loads(r["after"]) if r["after"] else None
        if before is None:  # an insert: take it out
            if not cur:
                out.append(f"gone     {tbl} {key}: deleted since")
                continue
            db.execute(f"DELETE FROM {tbl} WHERE {w}", args)
            audit(db, "delete", tbl, key, cur[0], None)
            out.append(f"deleted  {tbl} {key}" + ("" if cur[0] == after else " (it had changed since)"))
        elif after is None:  # a delete: put it back
            if cur:
                out.append(f"left     {tbl} {key}: a row is there again")
                continue
            cols = list(before)
            db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                       [before[c] for c in cols])
            audit(db, "insert", tbl, key, None, before)
            out.append(f"restored {tbl} {key} (put back)")
        else:
            if len(cur) != 1 or cur[0] != after:
                out.append(f"left     {tbl} {key}: changed since")
                continue
            cols = [c for c in before if c not in key]
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in cols)} WHERE {w}",
                       [before[c] for c in cols] + args)
            audit(db, "update", tbl, key, after, before)
            out.append(f"restored {tbl} {key}")
    db.execute(f"DROP TABLE {LOG}")
    return out


def ensure_melee_column(db):
    """petstats' melee_pct as the server adds it (database/petstats.go); reports whether it was added."""
    cols = [r[1] for r in db.execute("PRAGMA table_info(petstats)")]
    if "melee_pct" in cols:
        return False
    db.execute("ALTER TABLE petstats ADD COLUMN melee_pct INTEGER NOT NULL DEFAULT 100")
    return True


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
        if ensure_melee_column(db):
            print("column   petstats.melee_pct added (default 100)" + ("" if a.apply else ", rolled back with the dry run"))
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
        print(f"applied: {len(plan.changes)} rows, a gm_audit line each, logged in {LOG}; the robe and the icon at each "
              "player's next login, the rest at the next start")
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
