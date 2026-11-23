#!/usr/bin/env python3
"""Heirloom petitions 2716-2727 of 2026-11-23, as data, for either shard.

Two groups of changes. Every row is found by its NAME (and what else names it), never by an id alone, and must read
as it did when this was made; a row already as this writes it is left ("already"); anything else refuses the whole
run.

with-the-code (both shards; needs the server build with branch pet1123-heirloom's code, so run it in that build's
keep restart; before it the words would say more than the server does):
  2716  Forest's Might mitigates a creature's every hit, not its melee alone: its spelleffect's mitigation takes
        "all": true and its line and its spellpattern description say "all damage" (entity.SpellEffect.MitigateAll).
  2718  the River Saren tower's tomb rat loot steps (The Ghoul Bone Wand, The Smolder Mace, The Burnished Staff,
        The Indigo Glaive) and Further Training's decaying rat step get a place (queststeps x, z): their family names
        (queststeptargets: "large rat", "rat", ...) count only within 1,500 of it (handlers.stepKill), so Halas's
        large rats drop no sigils.

live-mirror (Live only: the Heirloom panel made these on 2026-11-23):
  2719  the Wandering Priestess's spawn where the player stood, and she wanders 25 from it (npcmove).
  2722  the Shadowhand Receipt's description sends its bearer to Nanibar Volsweath in Wyndhaven.
  2724  Grave Silk's journal says where the grave spiders stand, and the step has the tomb as its place (marks).
  2727  Deidre IV's Cog is the Perpetual Motion Device, and the quest's words with it.
  2717  the Gravemold Tunic's AGI 75 is AC 75 (the source's misread; the owner, 2026-11-23).
  2724  four more grave spiders in the tomb, copies of its four standing beside them on their floors (the owner).
  2729  the Infused Draught ticks a 4th time, at 18 s: 3,200 power for 800 health (the owner).

Every row written is a gm_audit line (via "tool") and a row of petpass_heirloom_1123_log; --undo puts each back
as it was (a row changed since is left and said) and drops the log. A report by default; --apply writes once (a
second finds the log and writes nothing); --apply and --undo copy the database first (<db>.bak-<when>-pre-pet1123)
unless --no-backup.

WHEN IT TAKES EFFECT: item records at each player's next login; quests, npcmove and npcs at once on the panel, else
(this tool) at the server's next start or a GM panel quest reload; spell effects at the next start.

    python3 -E -s tools/petpass_heirloom_1123_20261123.py --db eqoa.db --shard live            report
    python3 -E -s tools/petpass_heirloom_1123_20261123.py --db eqoa.db --shard live --apply    write it
    python3 -E -s tools/petpass_heirloom_1123_20261123.py --db eqoa.db --shard live --undo     take it back
"""
import argparse
import datetime
import json
import shutil
import sqlite3
import sys

LOG = "petpass_heirloom_1123_log"
WHO, VIA = "claude", "tool"
DETAIL = "Heirloom petitions 2716-2727 (2026-11-23): tools/petpass_heirloom_1123_20261123.py"

FM_LINE_OLD = "Self, Mitigate 75% of melee damage up to 764% of AGI, 60sec"
FM_LINE_NEW = "Self, Mitigate 75% of all damage up to 764% of AGI, 60sec"
FM_DESC_OLD = "CM: Self, Mitigate 75% of melee damage, 60sec"
FM_DESC_NEW = "CM: Self, Mitigate 75% of all damage up to 764% of AGI, 60sec"

TOWER = (23943.0, 8650.0)    # the River Saren tower's tomb rat, by Dragoon X`Talin's watchtower
DECAYING = (5385.0, 21300.0)  # the decaying rats' camp
RAT_STEPS = [  # quest name, giver, step, target, place
    ("Further Training", "Ulana Stillmind", 1, "decaying rat", DECAYING),
    ("The Ghoul Bone Wand", "Grear Hosrottle", 2, "tomb rat", TOWER),
    ("The Smolder Mace", "Theologist S`Tai", 2, "tomb rat", TOWER),
    ("The Burnished Staff", "Dominary K`Jartan", 2, "tomb rat", TOWER),
    ("The Indigo Glaive", "Scornmaster U`Dedne", 2, "tomb rat", TOWER),
]

PRIESTESS_OLD = (11155.9, 17228.2)
PRIESTESS_NEW = {"x_coord": 11129.4, "y_coord": 65.77, "z_coord": 17221.1}
RECEIPT_OLD = "A receipt carried by a Shadowhand bandit."
RECEIPT_NEW = ("A receipt carried by a Shadowhand bandit, made out to Nanibar Volsweath at the inn in Wyndhaven. "
               "Take it to him.")
SILK_OLD = "Take three grave silks from the grave spiders in the rat and spider tomb in south-east Hagley."
SILK_NEW = ("Take three grave silks from the grave spiders in the rat and spider tomb in south-east Hagley: two at the "
            "top of the tomb, two on its lowest floor below them.")
COG_OLD = "Deidre IV's Cog"
COG_NEW = "Perpetual Motion Device"
COG_DESC_NEW = "The perpetual motion device that drove the clockwork Deidre IV."
DEIDRE_OFFER_OLD = ("Deidre IV has gone wild. Out the east gate, south-east over the mountain's shoulder, to the "
                    "clockwork village. She roams the flats there. Bring me a cog of hers.")
DEIDRE_OFFER_NEW = ("Deidre IV has gone wild. Out the east gate, south-east over the mountain's shoulder, to the "
                    "clockwork village. She roams the flats there. Bring me her perpetual motion device.")
DEIDRE_J3_OLD = ("Bring Deidre IV's cog and the intense grease to Ludvick Undogeem in the enchanters' guild in "
                 "Klik'Anon.")
DRAUGHT_OLD = "Self, Decrease Hitpoints by 800, Increase Power per tic by 800, 18sec"
DRAUGHT_NEW = "Self, Decrease Hitpoints by 800, Increase Power per tic by 800, 4 ticks, 18sec"
# each standing grave spider (its place) and its copy's place, on the same floor (the ground's floor at it)
SPIDERS = [
    ((7783.77, 17450.9), (7787.0, 74.51, 17451.0)),
    ((7759.92, 17403.4), (7763.0, 76.14, 17408.0)),
    ((7761.88, 17450.0), (7766.0, 24.67, 17452.0)),
    ((7757.02, 17458.8), (7760.0, 24.82, 17462.0)),
]
DEIDRE_J3_NEW = ("Bring Deidre IV's perpetual motion device and the intense grease to Ludvick Undogeem in the "
                 "enchanters' guild in Klik'Anon.")

KEYS = {"itemeffect": ("patternid",), "spelleffect": ("spellid",), "spellpattern": ("spellid",), "queststeps": ("questid", "step"),
        "quests": ("questid",), "itempattern": ("patternid",), "npcs": ("id",), "npcmove": ("name",)}
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


class Plan:
    def __init__(self, db):
        self.db = db
        self.changes = []  # (tbl, key dict, before row or None, values)
        self.lines = []

    def set(self, tbl, row, values, olds, what):
        """values over row, when each column of olds reads its old (a float column near it); already as values: left."""
        if all(near(row[c], v) if isinstance(v, float) else row[c] == v for c, v in values.items()):
            self.lines.append(f"already  {what}")
            return
        for c, old in olds.items():
            ok = near(row[c], old) if isinstance(old, float) else row[c] == old
            if not ok:
                raise Refused(f"{what}: {c} reads {row[c]!r}, not {old!r} as read when this was made")
        key = {k: row[k] for k in KEYS[tbl]}
        self.changes.append((tbl, key, dict(row), dict(values)))
        self.lines.append(f"change   {what}: " + ", ".join(f"{c} {row[c]!r} -> {v!r}" for c, v in values.items()))

    def with_code(self):
        db = self.db
        sp = one(db, "Forest's Might's spellpattern", "SELECT * FROM spellpattern WHERE spellname = ?", ("Forest's Might",))
        se = one(db, "Forest's Might's spelleffect", "SELECT * FROM spelleffect WHERE spellid = ?", (sp["spellid"],))
        eff = json.loads(se["effects"])
        mits = [e for e in eff.get("effects", []) if e.get("mitigation")]
        if len(mits) != 1 or mits[0]["mitigation"].get("pct") != 75:
            raise Refused(f"Forest's Might's effects: {len(mits)} mitigations, want one of 75%")
        if mits[0]["mitigation"].get("all") is True:
            new_effects = se["effects"]
        else:
            mits[0]["mitigation"]["all"] = True
            new_effects = json.dumps(eff, separators=(",", ":"), ensure_ascii=False)
        self.set("spelleffect", se, {"effects": new_effects, "line": FM_LINE_NEW}, {"line": FM_LINE_OLD},
                 "2716 Forest's Might's effect (mitigation all)")
        self.set("spellpattern", sp, {"spelldesc": FM_DESC_NEW}, {"spelldesc": FM_DESC_OLD},
                 "2716 Forest's Might's description")
        for name, giver, step, target, (x, z) in RAT_STEPS:
            st = one(db, f"{name}'s step {step}", """SELECT s.* FROM queststeps s JOIN quests q ON q.questid = s.questid
                WHERE q.name = ? AND q.giver = ? AND s.step = ? AND s.kind = 'loot' AND s.target = ?""",
                     (name, giver, step, target))
            self.set("queststeps", st, {"world": 0, "x": x, "z": z}, {"world": 0, "x": 0.0, "z": 0.0},
                     f"2718 {name} step {step} ({target}) placed")

    def live_mirror(self):
        db = self.db
        npc = one(db, "the Wandering Priestess", "SELECT * FROM npcs WHERE npc_name = 'Wandering Priestess' AND world = 0")
        self.set("npcs", npc, dict(PRIESTESS_NEW), {"x_coord": PRIESTESS_OLD[0], "z_coord": PRIESTESS_OLD[1]},
                 "2719 the Wandering Priestess's spawn")
        mv = rows(db, "SELECT * FROM npcmove WHERE name = 'wandering priestess'")
        want = {"name": "wandering priestess", "roam": 1, "radius": 25.0,
                "note": "wanders near her spot (Heirloom petition 2719)", "source": "gm"}
        if not mv:
            self.changes.append(("npcmove", {"name": want["name"]}, None, want))
            self.lines.append("insert   2719 npcmove wandering priestess: roam 1, radius 25")
        elif mv[0]["roam"] == 1 and near(mv[0]["radius"], 25):
            self.lines.append("already  2719 npcmove wandering priestess")
        else:
            raise Refused(f"npcmove wandering priestess reads {mv[0]}")
        rc = one(db, "the Shadowhand Receipt", "SELECT * FROM itempattern WHERE itemname = 'Shadowhand Receipt'")
        self.set("itempattern", rc, {"itemdesc": RECEIPT_NEW}, {"itemdesc": RECEIPT_OLD},
                 "2722 the Shadowhand Receipt's description")
        silk = one(db, "Grave Silk's step 1", """SELECT s.* FROM queststeps s JOIN quests q ON q.questid = s.questid
            WHERE q.name = 'Grave Silk' AND s.step = 1 AND s.target = 'grave spider'""")
        self.set("queststeps", silk, {"journal": SILK_NEW, "x": 7770.0, "z": 17440.0},
                 {"journal": SILK_OLD, "x": 0.0, "z": 0.0}, "2724 Grave Silk's step 1")
        q = one(db, "the quest Deidre IV", "SELECT * FROM quests WHERE name = 'Deidre IV' AND giver = 'Ludvick Undogeem'")
        st1 = one(db, "Deidre IV's step 1", "SELECT * FROM queststeps WHERE questid = ? AND step = 1 AND kind = 'loot'",
                  (q["questid"],))
        cogs = rows(db, "SELECT * FROM itempattern WHERE patternid = ?", (int(str(st1["items"]).split("*")[0]),))
        if len(cogs) != 1:
            raise Refused("Deidre IV's step 1 names no item record")
        self.set("itempattern", cogs[0], {"itemname": COG_NEW, "itemdesc": COG_DESC_NEW}, {"itemname": COG_OLD},
                 "2727 Deidre IV's item")
        self.set("quests", q, {"offer": DEIDRE_OFFER_NEW}, {"offer": DEIDRE_OFFER_OLD}, "2727 Deidre IV's offer")
        st3 = one(db, "Deidre IV's step 3", "SELECT * FROM queststeps WHERE questid = ? AND step = 3", (q["questid"],))
        self.set("queststeps", st3, {"journal": DEIDRE_J3_NEW}, {"journal": DEIDRE_J3_OLD}, "2727 Deidre IV's step 3")
        tunic = one(db, "the Gravemold Tunic", "SELECT * FROM itempattern WHERE itemname = 'Gravemold Tunic'")
        self.set("itempattern", tunic, {"agi": 0, "AC": 75}, {"agi": 75, "AC": 0}, "2717 the Gravemold Tunic's AGI 75 as AC")
        dr = one(db, "the Infused Draught's effect", """SELECT e.* FROM itemeffect e JOIN itempattern p ON
            p.patternid = e.patternid WHERE p.itemname = 'Infused Draught'""")
        self.set("itemeffect", dr, {"line": DRAUGHT_NEW}, {"line": DRAUGHT_OLD}, "2729 the Infused Draught's 4th tick")
        spiders = rows(db, "SELECT * FROM npcs WHERE npc_name = 'grave spider' AND world = 0")
        for (ox, oz), (x, y, z) in SPIDERS:
            if any(abs(r["x_coord"] - x) < 0.5 and abs(r["z_coord"] - z) < 0.5 for r in spiders):
                self.lines.append(f"already  2724 a grave spider at ({x}, {z})")
                continue
            src = [r for r in spiders if abs(r["x_coord"] - ox) < 0.5 and abs(r["z_coord"] - oz) < 0.5]
            if len(src) != 1:
                raise Refused(f"2724: {len(src)} grave spiders at ({ox}, {oz}), want one")
            new = {k: v for k, v in src[0].items() if k != "id"}
            new.update({"x_coord": x, "y_coord": y, "z_coord": z})
            self.changes.append(("npcs", None, None, new))
            self.lines.append(f"insert   2724 a grave spider at ({x}, {y}, {z}), a copy of row {src[0]['id']}")


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
            if key is None:  # a new npcs row: its id is the one made
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
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-pet1123"
    src = sqlite3.connect(path)
    out = sqlite3.connect(dst)
    src.backup(out)
    out.close()
    src.close()
    return dst


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--shard", required=True, choices=("heirloom", "live"))
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
        plan.with_code()
        if a.shard == "live":
            plan.live_mirror()
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
