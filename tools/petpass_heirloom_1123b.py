#!/usr/bin/env python3
"""Heirloom petitions 2745-2765 of 2026-11-23 (the second batch of the day), as data.

Every row is found by its name (and what else names it), never by an id alone, and must read as it did when this
was made; a row already as this writes it is left ("already"); anything else refuses the whole run.

  2745  Abominus (Castle Felstart) is undead: an npcundead row for its name (its name says nothing undead).
  2746  Wolf's Eye Agate wears the green gem's icon (Emerald's, Jade's), not the red one (Star Rose Quartz's).
  2749  Skarr's Paw wears the furry paw's icon (rat foot's, drake's foot's), not the raw meat's; Blue Paw and
        Trapmaw's Paw, the other two paws on the meat, with it.
  2755  Clear Quartz wears the white diamond's icon (Diamond's), not the dark cut stone's (Onyx's, Schorl's).
  2765  the Dimunitive Shortblade (and its two namesakes) is a short sword: the short sword's icon and hand
        model (militia short sword's, steel shortsword's), not a dagger's.
  2747  Highpass Hold's six level 35 quests ask Citizens of Highpass 150 (the faction audit, owner approved,
        2026-11-21): their givers now say how it is earned (quests.untrusted), the goblins of the pass.
  2750  The Clockwork Invasion: Klockizar Riddik: he and his guardian appear on the hill east of Recon Wecon
  2751  (the reach step's place and its spawn events), where the player stood; Recon Wecon stands where the
        player found him, below it, and his words send the player to the hill to the east.
  2753  the Pirates of Klick (the nine names of the camp on the Klick`Anon coast) are of Plankwalker's Pirates
        (Prima's gnomish pirates, start -500): kill on sight (npcfaction, the camp's place only).
  2757  Feria Huckert is of her own faction, Feria Hurkert (the one her cracktooth kills raise and her quests
        ask 150 of), not Citizens of Hodstock: her face follows the standing the player earned.
  2760  the camp's five named pirates spawned inside their tents (walled on every side) and could not get out:
        each spawn moved just outside its tent, on open ground of the camp.
  2761  the red-haired arctic bandit (The Redheaded Bandit's step, row 107270) spawns nearer the hills, where
        the player stood, and wanders there (npcwander 60).

Every row written is a gm_audit line (via "tool") and a row of petpass_heirloom_1123b_log; --undo puts each back
as it was (a row changed since is left and said) and drops the log. A report by default; --apply writes once (a
second finds the log and writes nothing); --apply and --undo copy the database first (<db>.bak-<when>-pre-pet1123b)
unless --no-backup.

WHEN IT TAKES EFFECT: item records at each player's next login; quests, npcs, npcwander, npcfaction and npcundead
at the server's next start (or a GM panel reload of each).

    python3 -E -s tools/petpass_heirloom_1123b.py --db eqoa.db            report
    python3 -E -s tools/petpass_heirloom_1123b.py --db eqoa.db --apply    write it
    python3 -E -s tools/petpass_heirloom_1123b.py --db eqoa.db --undo     take it back
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "petpass_heirloom_1123b_log"
WHO, VIA = "claude", "tool"
DETAIL = "Heirloom petitions 2745-2765 (2026-11-23): tools/petpass_heirloom_1123b.py"
SOURCE = "petpass-1123b"

# (petition, item name, column, old, new): icons and hand models, copied from records that wear them
GREEN_GEM, RED_GEM = 140265582, -1731215164          # Emerald, Jade, Malachite / Star Rose Quartz, Rubellite
MEAT, FURRY_PAW = -84908632, -879281303              # / rat foot, drake's foot, harpy foot
DARK_STONE, WHITE_DIAMOND = -1071168822, 596177162   # Onyx, Schorl, Topaz / Diamond
DAGGER_ICON, THIN_BLADE_ICON = -541133199, 1681985143
SHORT_SWORD_ICON = 445518418                         # militia short sword, steel shortsword, Recruit's Shortsword
DAGGER_MODEL, DIRK_MODEL, SHORT_SWORD_MODEL = -653878684, -2084666022, -751439115
ITEMS = [
    ("2746", "Wolf's Eye Agate", {"itemicon": (RED_GEM, GREEN_GEM)}),
    ("2749", "Skarr's Paw", {"itemicon": (MEAT, FURRY_PAW)}),
    ("2749", "Blue Paw", {"itemicon": (MEAT, FURRY_PAW)}),
    ("2749", "Trapmaw's Paw", {"itemicon": (MEAT, FURRY_PAW)}),
    ("2755", "Clear Quartz", {"itemicon": (DARK_STONE, WHITE_DIAMOND)}),
    ("2765", "Dimunitive Shortblade", {"itemicon": (THIN_BLADE_ICON, SHORT_SWORD_ICON),
                                       "model": (DIRK_MODEL, SHORT_SWORD_MODEL)}),
    ("2765", "Diminutive Shortblade", {"itemicon": (DAGGER_ICON, SHORT_SWORD_ICON),
                                       "model": (DAGGER_MODEL, SHORT_SWORD_MODEL)}),
    ("2765", "Dimunutive Shortblade", {"itemicon": (DAGGER_ICON, SHORT_SWORD_ICON),
                                       "model": (DAGGER_MODEL, SHORT_SWORD_MODEL)}),
]
LOOK_REFS = {GREEN_GEM: "Emerald", FURRY_PAW: "rat foot", WHITE_DIAMOND: "Diamond", SHORT_SWORD_ICON: "steel shortsword"}
MODEL_REFS = {SHORT_SWORD_MODEL: "steel shortsword"}

HIGHPASS_GIVERS = ["Dametrios Velspane", "Ambrose Castor", "Dunn McCabe", "Lord Voriander", "Fender Crow", "Rin Tintoggle"]
HIGHPASS_UNTRUSTED = ("The goblins in and around Highpass Hold are its enemy: every one you put down earns you the "
                      "trust of its citizens.")

# 2750/2751: where the player stood
RECON_OLD = (24420.0, 6600.0)
RECON_NEW = {"x_coord": 23930.9, "y_coord": 54.13, "z_coord": 6763.8}
HILL = (24136.2, 94.72, 6717.8)
RIDDIK_S1_OLD = ("Sizzick's lieutenant, Klockizar Riddik, holds the very top of the mountain to the south, with a "
                 "mummified guardian at his side. Bring me his plans.")
RIDDIK_S1_NEW = ("Sizzick's lieutenant, Klockizar Riddik, holds the hill just east of here, with a mummified guardian "
                 "at his side. Bring me his plans.")
RIDDIK_S2_JOURNAL_OLD = "Climb to the very top of the mountain south of Klick`Anon's east gate."
RIDDIK_S2_JOURNAL_NEW = "Climb the hill just east of Recon Wecon, outside Klick`Anon's east gate."
RIDDIK_S3_JOURNAL_OLD = "Kill Klockizar Riddik on the mountain top and take the Stage 4 Operations."
RIDDIK_S3_JOURNAL_NEW = "Kill Klockizar Riddik on the hill east of Recon Wecon and take the Stage 4 Operations."
MOUNTAIN = (24612.8, 153.5, 6901.7)

# 2753: the camp of the Pirates of Klick
PIRATES = ["deckhand", "pirate scout", "pirate stonemason", "glim plankwalker", "frithar brittlebeard",
           "garlor broadsides", "jikor highseas", "salzar tidemaster", "scoot plunderhands"]
CAMP = (25435.0, 6560.0, 150.0)
PIRATE_FACTION = "Plankwalker's Pirates"

# 2760: each named pirate's spawn (in its tent) and the open ground just outside it (the server's walls, probed)
TENTS = [
    ("Glim Plankwalker", (25464.4, 6598.91), (25470.4, 6592.9)),
    ("Garlor Broadsides", (25398.5, 6519.5), (25404.5, 6521.5)),
    ("Jikor Highseas", (25443.2, 6600.95), (25443.2, 6590.9)),
    ("Salzar Tidemaster", (25427.5, 6510.91), (25435.5, 6508.9)),
    ("Scoot Plunderhands", (25473.0, 6572.27), (25463.0, 6580.3)),
]

# 2761: the red-haired arctic bandit
BANDIT_ID, BANDIT_OLD = 107270, (17653.2, 5593.34)
BANDIT_NEW = {"x_coord": 17654.1, "y_coord": 55.11, "z_coord": 5488.0}

KEYS = {"itempattern": ("patternid",), "quests": ("questid",), "queststeps": ("questid", "step"), "npcs": ("id",),
        "npcfaction": ("id",), "npcundead": ("name",), "npcwander": ("npcid",)}
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

    def build(self):
        db = self.db
        # looks, each id checked against a record already wearing it
        for icon, ref in LOOK_REFS.items():
            one(db, f"{ref}'s icon", "SELECT patternid FROM itempattern WHERE itemname = ? AND itemicon = ? LIMIT 1",
                (ref, icon))
        for model, ref in MODEL_REFS.items():
            one(db, f"{ref}'s model", "SELECT patternid FROM itempattern WHERE itemname = ? AND model = ? LIMIT 1",
                (ref, model))
        for pet, name, cols in ITEMS:
            for r in rows(db, "SELECT * FROM itempattern WHERE itemname = ? ORDER BY patternid", (name,)) or \
                    [None]:
                if r is None:
                    raise Refused(f"{pet}: no item record named {name!r}")
                self.set("itempattern", r, {c: n for c, (_, n) in cols.items()}, {c: o for c, (o, _) in cols.items()},
                         f"{pet} {name} ({r['patternid']}) look")

        # 2745
        ab = rows(db, "SELECT id FROM npcs WHERE npc_name = 'Abominus'")
        if not ab:
            raise Refused("2745: no NPC named Abominus")
        und = rows(db, "SELECT * FROM npcundead WHERE name = 'abominus'")
        self.insert("npcundead", {"name": "abominus"},
                    {"name": "abominus", "undead": 1, "note": "Castle Felstart's Abominus (Heirloom petition 2745)",
                     "source": SOURCE}, "2745 npcundead abominus", und[0] if und else None)

        # 2747
        for giver in HIGHPASS_GIVERS:
            q = one(db, f"{giver}'s level 35 quest", """SELECT * FROM quests WHERE giver = ? AND minlevel = 35
                AND factionreq = 'Citizens of Highpass:150'""", (giver,))
            self.set("quests", q, {"untrusted": HIGHPASS_UNTRUSTED}, {"untrusted": ""},
                     f"2747 {q['name']}'s untrusted words")

        # 2750 / 2751
        recon = one(db, "Recon Wecon", "SELECT * FROM npcs WHERE npc_name = 'Recon Wecon' AND world = 0")
        self.set("npcs", recon, dict(RECON_NEW), {"x_coord": RECON_OLD[0], "z_coord": RECON_OLD[1]},
                 "2751 Recon Wecon's spawn")
        q = one(db, "the quest Klockizar Riddik", """SELECT * FROM quests WHERE name =
            'The Clockwork Invasion: Klockizar Riddik' AND giver = 'Elder Mysidia'""")
        s1 = one(db, "Riddik step 1", "SELECT * FROM queststeps WHERE questid = ? AND step = 1 AND kind = 'talk' "
                 "AND target = 'Recon Wecon'", (q["questid"],))
        self.set("queststeps", s1, {"says": RIDDIK_S1_NEW}, {"says": RIDDIK_S1_OLD}, "2751 Recon Wecon's words")
        s2 = one(db, "Riddik step 2", "SELECT * FROM queststeps WHERE questid = ? AND step = 2 AND kind = 'reach'",
                 (q["questid"],))
        ev = json.loads(s2["events"]) if s2["events"] else []
        spawns = [e for e in ev if e.get("do") == "spawn"]
        if len(spawns) != 2:
            raise Refused(f"2750 Riddik step 2: {len(spawns)} spawn events, want two")
        for e in spawns:
            if not (near(e.get("x", 0), HILL[0]) and near(e.get("z", 0), HILL[2])):
                if not (near(e.get("x", 0), MOUNTAIN[0]) and near(e.get("z", 0), MOUNTAIN[2])):
                    raise Refused(f"2750 Riddik step 2: a spawn event at {e.get('x')}, {e.get('z')}")
                e["x"], e["y"], e["z"] = HILL
        new_events = json.dumps(ev, separators=(",", ":"), ensure_ascii=False)
        if new_events == s2["events"] or all(near(e["x"], HILL[0]) for e in json.loads(s2["events"])
                                             if e.get("do") == "spawn"):
            new_events = s2["events"]
        self.set("queststeps", s2, {"target": "the hill east of Recon Wecon", "x": HILL[0], "y": HILL[1], "z": HILL[2],
                                    "journal": RIDDIK_S2_JOURNAL_NEW, "events": new_events},
                 {"target": "the mountain top", "x": MOUNTAIN[0], "z": MOUNTAIN[2], "journal": RIDDIK_S2_JOURNAL_OLD},
                 "2750 Riddik's hill (step 2, its spawns)")
        s3 = one(db, "Riddik step 3", "SELECT * FROM queststeps WHERE questid = ? AND step = 3 AND kind = 'loot'",
                 (q["questid"],))
        self.set("queststeps", s3, {"journal": RIDDIK_S3_JOURNAL_NEW}, {"journal": RIDDIK_S3_JOURNAL_OLD},
                 "2750 Riddik step 3's journal")

        # 2753
        one(db, "the faction Plankwalker's Pirates", "SELECT * FROM factions WHERE name = ?", (PIRATE_FACTION,))
        cx, cz, cr = CAMP
        for name in PIRATES:
            n = rows(db, """SELECT id FROM npcs WHERE lower(npc_name) = ? AND world = 0 AND
                (x_coord - ?) * (x_coord - ?) + (z_coord - ?) * (z_coord - ?) <= ? * ?""",
                     (name, cx, cx, cz, cz, cr, cr))
            if not n:
                raise Refused(f"2753: no {name} in the camp")
            have = rows(db, "SELECT * FROM npcfaction WHERE name = ?", (name,))
            if have and not (len(have) == 1 and have[0]["faction"] == PIRATE_FACTION and near(have[0]["x"], cx)):
                raise Refused(f"2753: {name} has npcfaction rows already: {have}")
            self.insert("npcfaction", None,
                        {"name": name, "world": 0, "x": cx, "z": cz, "radius": cr, "faction": PIRATE_FACTION,
                         "note": "the Pirates of Klick's camp, Prima's gnomish pirates (Heirloom petition 2753)",
                         "source": SOURCE}, f"2753 npcfaction {name}: {PIRATE_FACTION}", have[0] if have else None)

        # 2757
        fr = one(db, "Feria Huckert's npcfaction row", "SELECT * FROM npcfaction WHERE name = 'feria huckert'")
        one(db, "the faction Feria Hurkert", "SELECT * FROM factions WHERE name = 'Feria Hurkert'")
        self.set("npcfaction", fr, {"faction": "Feria Hurkert"}, {"faction": "Citizens of Hodstock"},
                 "2757 Feria Huckert's faction")

        # 2760
        for name, (ox, oz), (nx, nz) in TENTS:
            r = one(db, name, "SELECT * FROM npcs WHERE npc_name = ? AND world = 0", (name,))
            self.set("npcs", r, {"x_coord": nx, "y_coord": 54.125, "z_coord": nz}, {"x_coord": ox, "z_coord": oz},
                     f"2760 {name}'s spawn out of its tent")

        # 2761
        b = one(db, "the red-haired arctic bandit", "SELECT * FROM npcs WHERE id = ? AND npc_name = 'arctic bandit'",
                (BANDIT_ID,))
        one(db, "The Redheaded Bandit's step 2", """SELECT s.* FROM queststeps s JOIN quests q ON q.questid = s.questid
            WHERE q.name = 'The Redheaded Bandit' AND s.step = 2 AND s.npcid = ?""", (BANDIT_ID,))
        self.set("npcs", b, dict(BANDIT_NEW), {"x_coord": BANDIT_OLD[0], "z_coord": BANDIT_OLD[1]},
                 "2761 the red-haired bandit's spawn")
        w = rows(db, "SELECT * FROM npcwander WHERE npcid = ?", (BANDIT_ID,))
        self.insert("npcwander", {"npcid": BANDIT_ID},
                    {"npcid": BANDIT_ID, "radius": 60.0,
                     "note": "the red-haired bandit wanders near the hills (Heirloom petition 2761)", "source": SOURCE},
                    "2761 npcwander the red-haired bandit, 60", w[0] if w else None)


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
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-pet1123b"
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
