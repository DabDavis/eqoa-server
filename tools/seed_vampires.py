#!/usr/bin/env python3
"""The vampires (entity/vampire.go): their abilities, the blood they cost,
their Sanctuary and its coffins, and their quests made playable.

The user, 2026-10-30: "Build the vampire line", "Move them under rock". The
sources, best first:

- SANDSTORM'S OWN RECORDS of Feed (spell 1807) and Sanctuary (1823), the
  0x00B3 records its login sent Afkqq, a vampire of the user's (2026-10-30):
  level 30, range 4.5, no power; Feed cast at a target, no cast time, recast
  5 s (the guide's 3 and 3 are not the game's); Sanctuary on its caster, 3 s,
  recast 3600 s (the guide's "usable once per day", a game day); their icons
  and words. Sanctuary's landing is Sandstorm's (claude-notes/sanctuary-
  memcard-patch-20261025.md: S2's world 7 at 4322.19, 274.134, 6179.78, the
  Frontiers disc's world 5 file), on the floor there (273.62), under 48 of
  rock: a chamber in the Zaoran Plateau (world 5's zone 5), the guide's
  "hidden room in ZP".
- EQOA-DATA'S VAMPIRE GUIDE (Quests/Weres/vampire.json) for the rest:
  Vampiric Strike (L40: cast 1, recast 12, ~1200-1500 damage and ~400 heal,
  7 blood), Vampire Form (L40: cast 3, recast 15, the body alone, 2 blood),
  Bat Form (L50: cast 3, recast 30, 30 s, STR set to 500, held to the stat
  cap as any STR, AC -963, power 1/1, 8 blood), Vitality (L50: cast 1, recast 60, 60 s, +1000 hit points
  and +300 AC, 5 blood), Frenzy (L60: cast 1, recast 60, 60 s, 500 more
  damage a hit "but reduces defense", 5 blood), Mist Form (L60: cast 1,
  recast 30, 30 s, +2000 AC, STR 0, power 1/1, 8 blood), Vampiric Gaze
  (L60: no cast time, "Sets aggro with target to 0", 10 blood), Sanctuary's
  2 blood; Feed "Primary method for gaining Units of Blood", "Blood stacks:
  30 (No Trade / No Rent)"; the nine coffin ports.
- OURS, where neither says: Feed's drain (40 to 60, healing the vampire for
  what it takes) and its one Unit of Blood a feeding; Frenzy's AC -300; the
  gaze's recast (30 s) and reach (20); the other records' icons (Feed's and
  Sanctuary's own, by whether they are cast at a target); the item's icon
  (Randerith's blood's, "the blood of a vampire"); the descriptions of the
  abilities Sandstorm's records do not give; the five NPCs' places in the
  chamber round the landing (each on its floor, under the rock, clear of the
  landing's walls: internal/ground's Roof and Wall on a bake), facing it;
  the ids 760001-760007 of the seven abilities with no record.

What it writes, each change logged in vampire_seed_log with what was there
before, in one transaction:
  - the nine ability records (spellpattern) and their lines (spelleffect,
    source "seed_vampires"), which entity/spelltext.go reads ("Reagent Unit
    of Blood x2", "Set STR to 500", "Increase Damage per Hit by 500", "Set
    Aggro to 0" are the vampires' own words for it); a record or a line a
    spell has already is left alone and said;
  - the item Unit of Blood (itempattern, stacking to 30, no trade, no rent);
  - the Sanctuary spell's place (spellgates, self);
  - Hinder, Creed, Weaver and Simmons (world 5) moved from the open ground
    north of the building to the chamber where Sandstorm's spell lands;
  - the Sanctuary's coffin (an NPC named "Sanctuary coffin", the coffins'
    body), whose trips internal/handlers/waysender.go keeps, as it keeps
    those of the three coffins Return Home placed (Freeport, Collonridge
    Cemetery, the Lavastorm's Coffin1);
and then, in the import's own, the four vampire quests (Weres/vampire/p1,
L40, L50, L60) imported again from tools/quests/data/weres.json by
tools/import_quests.py's plan and write (their spells now here, so they are
drafts no more; the kill step's vampire event; the teleport to the
landing), each quest's rows as they were logged first. A quest a GM changed
since its import is left as the GM left it, and said.

    python3 -E -s tools/seed_vampires.py --db eqoa.db             report
    python3 -E -s tools/seed_vampires.py --db eqoa.db --apply     write it
    python3 -E -s tools/seed_vampires.py --db eqoa.db --undo      take it back

--undo puts back what a run changed that no one changed since (a changed
row is said and kept): the quests' rows as they were, the NPCs where they
stood, and deletes what it made, but an ability in a character's book or an
item someone holds (both said); characters' progress in the quests and the
vampires the quests made (charvampire) stay. A copy of the database is made
first (--no-backup to skip). Stop the server to apply or undo, start it
after: the quests, the gates and the NPCs are read at its start.
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)

DEFAULT_DB = "eqoa.db"
LOG = "vampire_seed_log"
SOURCE = "seed_vampires"
WERES = os.path.join(HERE, "quests", "data", "weres.json")
QUEST_KEYS = ("Weres/vampire/p1", "Weres/vampire/L40", "Weres/vampire/L50", "Weres/vampire/L60")

# Sandstorm's icons: every vampire ability's front, and the backs of a
# target's (Feed's) and a self (Sanctuary's) ability, as spellpattern keeps
# them (signed 32 bits: Sanctuary's back is 0x942B153A, every self spell's)
FRONT = 2036326070
BACK_TARGET = 486870879
BACK_SELF = -1809115846

# the range column holds a float's high 16 bits (4.5 is 0x4090, 20 0x41A0)
RANGE_4_5 = 0x4090
RANGE_20 = 0x41A0

ITEM = "Unit of Blood"
ITEM_ICON = 1778128471  # Randerith's blood's, "the blood of a vampire"
ITEM_DESC = "A unit of blood, drawn by a vampire's feeding. Every vampire ability costs blood."

# (spellid, name, level, cast s, recast s, scope, range, back icon, description, line)
ABILITIES = [
    (1807, "Feed", 30, 0, 5, 1, RANGE_4_5, BACK_TARGET,
     "Primary method for gaining Units of Blood, which is used to power all Vampire abilities.",
     "Target, Drain HP by 40 to 60 give self, Summon Unit of Blood x1"),
    (1823, "Sanctuary", 30, 3, 3600, 0, RANGE_4_5, BACK_SELF,
     "Vampire Teleport to The Sanctuary.",
     "Self, Teleport to The Sanctuary, Reagent Unit of Blood x2"),
    (760001, "Vampiric Strike", 40, 1, 12, 1, RANGE_4_5, BACK_TARGET,
     "Strike, and drink of what you spill.",
     "Target, Decrease Hitpoints by 1200 to 1500, Increase Caster HP by 400, Reagent Unit of Blood x7"),
    (760002, "Vampire Form", 40, 3, 15, 0, RANGE_4_5, BACK_SELF,
     "Assume the form of a vampire.",
     "Self, Reagent Unit of Blood x2"),
    (760003, "Bat Form", 50, 3, 30, 0, RANGE_4_5, BACK_SELF,
     "Assume the form of a bat: great strength at the cost of defense.",
     "Self, Set STR to 500, Decrease AC by 963, Set Maximum Power to 1, 30sec, Reagent Unit of Blood x8"),
    (760004, "Vitality", 50, 1, 60, 0, RANGE_4_5, BACK_SELF,
     "Call on the blood to heal and harden you.",
     "Self, Increase HP by 1000, Increase AC by 300, 60sec, Reagent Unit of Blood x5"),
    (760005, "Frenzy", 60, 1, 60, 0, RANGE_4_5, BACK_SELF,
     "Every blow lands harder, at the cost of defense.",
     "Self, Increase Damage per Hit by 500, Decrease AC by 300, 60sec, Reagent Unit of Blood x5"),
    (760006, "Mist Form", 60, 1, 30, 0, RANGE_4_5, BACK_SELF,
     "Assume the form of a red mist: hard to harm, and harmless.",
     "Self, Increase AC by 2000, Set STR to 0, Set Maximum Power to 1, 30sec, Reagent Unit of Blood x8"),
    (760007, "Vampiric Gaze", 60, 0, 30, 1, RANGE_20, BACK_TARGET,
     "Turns a creature's eyes and its anger from you.",
     "Target, Set Aggro to 0, Reagent Unit of Blood x10"),
]

# where Sandstorm's Sanctuary spell lands, on the floor (world 5)
LANDING = (5, 4322.19, 273.62, 6179.78)

# the Sanctuary's people round the landing (x, y, z, facing byte toward it)
NPC_SPOTS = {
    "Hinder": (4327.73, 273.62, 6182.08, 176),
    "Creed": (4326.43, 273.62, 6184.02, 160),
    "Weaver": (4326.43, 273.62, 6175.54, 224),
    "Simmons": (4327.73, 273.62, 6177.48, 208),
}
COFFIN = "Sanctuary coffin"
COFFIN_SPOT = (4323.72, 273.62, 6183.48, 144)
# spellgates as the server makes it (internal/database/spellgates.go)
GATES_DDL = """CREATE TABLE IF NOT EXISTS spellgates (
    spell  TEXT PRIMARY KEY,
    kind   TEXT NOT NULL DEFAULT 'self',
    place  TEXT NOT NULL DEFAULT '',
    world  INTEGER NOT NULL DEFAULT 0,
    x      REAL NOT NULL DEFAULT 0,
    y      REAL NOT NULL DEFAULT 0,
    z      REAL NOT NULL DEFAULT 0,
    radius REAL NOT NULL DEFAULT 0,
    note   TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm'
)"""

# the tables whose AUTOINCREMENT counters a run's new rows move, by key
COUNTERS = {"npcs": "id", "itempattern": "patternid", "spellpattern": "spellid"}

# a coffin's body where the database has no coffin to copy
COFFIN_BODY = dict(modelid=990449726, size=1.4, npc_type=384, npc_level=1, hp=100, hp_percent=255)


def has_table(db, t):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone()[0] > 0


def columns(db, t):
    return [r[1] for r in db.execute(f"PRAGMA table_info({t})")]


def row_of(db, table, where, args):
    cur = db.execute(f"SELECT * FROM {table} WHERE {where}", args)
    r = cur.fetchone()
    if r is None:
        return None
    return dict(zip([d[0] for d in cur.description], r))


def quest_ids(db):
    """The vampire quests' ids by key, as the import logged them."""
    if not has_table(db, "quest_import_quests"):
        return {}
    marks = ",".join("?" * len(QUEST_KEYS))
    return dict(db.execute(f"SELECT key, questid FROM quest_import_quests WHERE key IN ({marks})", QUEST_KEYS).fetchall())


def plan(db):
    """What a run would do, as (what, detail) lines."""
    out = []
    for sid, name, *_ in ABILITIES:
        have = row_of(db, "spellpattern", "spellid = ?", (sid,))
        same = db.execute("SELECT spellid FROM spellpattern WHERE lower(spellname) = lower(?) AND spellid != ?", (name, sid)).fetchall()
        if have:
            out.append(("ability", f"{name} ({sid}): a record there already ({have['spellname']!r}): left"))
        else:
            out.append(("ability", f"{name} ({sid}): made" + (f"; other records of the name: {[s for (s,) in same]}" if same else "")))
    item = db.execute("SELECT patternid FROM itempattern WHERE lower(itemname) = lower(?)", (ITEM,)).fetchone()
    out.append(("item", f"{ITEM}: " + (f"there already ({item[0]}): left" if item else "made, stacking to 30, no trade, no rent")))
    if has_table(db, "spellgates") and db.execute("SELECT count(*) FROM spellgates WHERE spell = 'sanctuary'").fetchone()[0]:
        out.append(("gate", "sanctuary: a row there already: left"))
    else:
        out.append(("gate", f"sanctuary: self, The Sanctuary, world {LANDING[0]} ({LANDING[1]}, {LANDING[2]}, {LANDING[3]})"))
    for name, (x, y, z, f) in NPC_SPOTS.items():
        rows = db.execute("SELECT id, x_coord, y_coord, z_coord FROM npcs WHERE npc_name = ? AND world = 5", (name,)).fetchall()
        if not rows:
            out.append(("npc", f"{name}: not in world 5 here: nothing to move"))
        for rid, ox, oy, oz in rows:
            out.append(("npc", f"{name} ({rid}): ({ox}, {oy}, {oz}) to ({x}, {y}, {z}) facing {f}"))
    coffin = db.execute("SELECT id FROM npcs WHERE npc_name = ? AND world = 5", (COFFIN,)).fetchone()
    out.append(("coffin", f"{COFFIN}: " + (f"there already ({coffin[0]}): left" if coffin else f"made at {COFFIN_SPOT[:3]}")))
    ids = quest_ids(db)
    for k in QUEST_KEYS:
        out.append(("quest", f"{k}: " + (f"quest {ids[k]} imported again from weres.json" if k in ids else "not imported here: imported new")))
    return out


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-vampires"
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def log(db, tbl, ref, did, before, after, when):
    db.execute(f"INSERT INTO {LOG} (tbl, ref, did, before, after, at) VALUES (?, ?, ?, ?, ?, ?)",
               (tbl, str(ref), did, None if before is None else json.dumps(before), None if after is None else json.dumps(after), when))


def import_vampire_quests(db):
    """The four quests imported again from weres.json by import_quests.py's
    own plan and write: (written, kept as a GM changed them, still drafts)."""
    import import_quests as iq
    p = iq.plan(db, iq.load_files([WERES]), routes=iq.travel.travel_routes())
    p["quests"] = [x for x in p["quests"] if x["key"] in QUEST_KEYS]
    named = set()

    def refs(o):  # the items the importer makes that these quests name: ("made", name)
        if isinstance(o, tuple) and len(o) == 2 and o[0] == "made":
            named.add(iq.fold(o[1]))
        elif isinstance(o, (list, tuple)):
            for v in o:
                refs(v)
        elif isinstance(o, dict):
            for v in o.values():
                refs(v)
    refs(p["quests"])
    p["made"] = [m for m in p["made"] if iq.fold(m["name"]) in named]
    drafts = [f"{x['key']}: {'; '.join(x['why'])}" for x in p["quests"] if x["draft"]]
    written, kept, _ = iq.write(db, p)
    return written, kept, drafts


def apply(db, import_quests=import_vampire_quests):
    if has_table(db, LOG):
        sys.exit(f"this database holds a run already ({LOG}): --undo it first")
    when = datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")
    said = []
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(f"CREATE TABLE {LOG} (seq INTEGER PRIMARY KEY, tbl TEXT NOT NULL, ref TEXT NOT NULL, "
                   "did TEXT NOT NULL, before TEXT, after TEXT, at TEXT NOT NULL)")
        # the counters the new rows move, and the import's own record, as they were
        for t in COUNTERS:
            r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = ?", (t,)).fetchone() \
                if has_table(db, "sqlite_sequence") else None
            log(db, "sqlite_sequence", t, "before", {"seq": None if r is None else r[0]}, None, when)
        if has_table(db, "quest_import_meta"):
            log(db, "quest_import_meta", "*", "before", db.execute("SELECT key, value FROM quest_import_meta").fetchall(), None, when)
        # the abilities and their lines
        for sid, name, level, cast, recast, scope, rng, back, desc, line in ABILITIES:
            if row_of(db, "spellpattern", "spellid = ?", (sid,)) is None:
                db.execute("INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, "
                           "icon, scope, recast, eqprequire, spellname, spelldesc) VALUES (?, ?, 0, 0, ?, ?, 0, ?, ?, ?, ?, -1, ?, ?)",
                           (sid, level, rng, cast, back, FRONT, scope, recast, name, desc))
                log(db, "spellpattern", sid, "insert", None, row_of(db, "spellpattern", "spellid = ?", (sid,)), when)
            else:
                said.append(f"ability {sid} ({name}): a record there already, left")
            if row_of(db, "spelleffect", "spellid = ?", (sid,)) is None:
                db.execute("INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (?, ?, 0, ?)", (sid, line, SOURCE))
                log(db, "spelleffect", sid, "insert", None, row_of(db, "spelleffect", "spellid = ?", (sid,)), when)
            else:
                said.append(f"ability {sid} ({name}): a line there already, left")
        # the blood
        if db.execute("SELECT count(*) FROM itempattern WHERE lower(itemname) = lower(?)", (ITEM,)).fetchone()[0] == 0:
            cols = [c for c in columns(db, "itempattern") if c != "patternid"]
            like = row_of(db, "itempattern", "lower(itemname) = lower(?)", ("Randerith's blood",)) or {}
            row = {c: like.get(c, 0) for c in cols}
            for c in cols:
                if row[c] is None:
                    row[c] = 0
            # every class and race, as the summoned foraged berries
            row.update(itemcost=0, itemicon=ITEM_ICON, equipslot=-1, trade=1, rent=0, attacktype=0, weapondamage=0,
                       levelreq=1, maxstack=30, maxhp=0, classuse=32767, raceuse=1023, procanim=0, lore=0,
                       craft=0, itemname=ITEM, itemdesc=ITEM_DESC, model=0)
            # the next id the table gives (AUTOINCREMENT: never one a deleted item had)
            pid = db.execute(f"INSERT INTO itempattern ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                             [row[c] for c in cols]).lastrowid
            log(db, "itempattern", pid, "insert", None, row_of(db, "itempattern", "patternid = ?", (pid,)), when)
        else:
            said.append(f"{ITEM}: there already, left")
        # the Sanctuary's place (the table as the server makes it, where it has not yet)
        if not has_table(db, "spellgates"):
            db.execute(GATES_DDL)
            log(db, "spellgates", "*", "made", None, None, when)
        if row_of(db, "spellgates", "spell = 'sanctuary'", ()) is None:
            w, x, y, z = LANDING
            db.execute("INSERT INTO spellgates (spell, kind, place, world, x, y, z, radius, note, source) VALUES "
                       "('sanctuary', 'self', 'The Sanctuary', ?, ?, ?, ?, 0, ?, 'seed')",
                       (w, x, y, z, "where Sandstorm's Sanctuary spell lands, under the rock (tools/seed_vampires.py)"))
            log(db, "spellgates", "sanctuary", "insert", None, row_of(db, "spellgates", "spell = 'sanctuary'", ()), when)
        else:
            said.append("spellgates sanctuary: a row there already, left")
        # the Sanctuary's people, under the rock
        for name, (x, y, z, f) in NPC_SPOTS.items():
            for (rid,) in db.execute("SELECT id FROM npcs WHERE npc_name = ? AND world = 5", (name,)).fetchall():
                before = row_of(db, "npcs", "id = ?", (rid,))
                db.execute("UPDATE npcs SET x_coord = ?, y_coord = ?, z_coord = ?, facing = ? WHERE id = ?", (x, y, z, f, rid))
                log(db, "npcs", rid, "update", before, row_of(db, "npcs", "id = ?", (rid,)), when)
        # the Sanctuary's coffin
        if db.execute("SELECT count(*) FROM npcs WHERE npc_name = ? AND world = 5", (COFFIN,)).fetchone()[0] == 0:
            like = row_of(db, "npcs", "npc_name = 'coffin' ORDER BY id", ())
            cols = [c for c in columns(db, "npcs") if c != "id"]
            row = {c: 0 for c in cols}
            row.update(dt_kill=None, robe_style="255", unk_i="", **COFFIN_BODY)
            if like:  # Return Home's coffins, body and all
                row.update({c: like[c] for c in cols})
            x, y, z, f = COFFIN_SPOT
            row.update(npc_name=COFFIN, world=LANDING[0], x_coord=x, y_coord=y, z_coord=z, facing=f, npc_id=0)
            cur = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [row[c] for c in cols])
            log(db, "npcs", cur.lastrowid, "insert", None, row_of(db, "npcs", "id = ?", (cur.lastrowid,)), when)
        else:
            said.append(f"{COFFIN}: there already, left")
        # the quests as they were, before the import below rewrites them
        for k, qid in quest_ids(db).items():
            q = row_of(db, "quests", "questid = ?", (qid,))
            cur = db.execute("SELECT * FROM queststeps WHERE questid = ? ORDER BY step", (qid,))
            steps = [dict(zip([d[0] for d in cur.description], r)) for r in cur.fetchall()]
            mark = row_of(db, "quest_import_quests", "key = ?", (k,))
            unlocks = db.execute("SELECT npc, dest FROM questunlocks WHERE questid = ?", (qid,)).fetchall() \
                if has_table(db, "questunlocks") else []
            log(db, "quests", k, "before", dict(quest=q, steps=steps, mark=mark, unlocks=unlocks), None, when)
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    written, kept, drafts = import_quests(db)
    db.execute(f"INSERT INTO {LOG} (tbl, ref, did, before, after, at) VALUES ('quests', '*', 'imported', NULL, ?, ?)",
               (json.dumps(dict(written=written, kept=kept, drafts=drafts)), when))
    return said, written, kept, drafts


def undo(db):
    if not has_table(db, LOG):
        sys.exit("no run to undo here")
    done, kept = [], []
    rows = db.execute(f"SELECT tbl, ref, did, before, after FROM {LOG} ORDER BY seq DESC").fetchall()
    db.execute("BEGIN IMMEDIATE")
    try:
        for tbl, ref, did, before, after in rows:
            before = json.loads(before) if before else None
            after = json.loads(after) if after else None
            if tbl == "quests" and did == "before":
                put_quest_back(db, ref, before)
                done.append(f"quest {ref} as it was")
            elif tbl == "spellgates" and did == "made":
                if db.execute("SELECT count(*) FROM spellgates").fetchone()[0] == 0:
                    db.execute("DROP TABLE spellgates")  # this run made it
            elif tbl == "quest_import_meta":
                db.execute("DELETE FROM quest_import_meta")
                db.executemany("INSERT INTO quest_import_meta VALUES (?, ?)", [tuple(r) for r in before])
            elif tbl == "sqlite_sequence":
                key = COUNTERS[ref]
                top = db.execute(f"SELECT COALESCE(max({key}), 0) FROM {ref}").fetchone()[0]
                if before["seq"] is None:
                    if top == 0:
                        db.execute("DELETE FROM sqlite_sequence WHERE name = ?", (ref,))
                elif top <= before["seq"]:
                    db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = ?", (before["seq"], ref))
            elif tbl == "npcs" and did == "update":
                now = row_of(db, "npcs", "id = ?", (int(ref),))
                if now != after:
                    kept.append(f"NPC {ref} ({after['npc_name']}): changed since, left")
                    continue
                db.execute("UPDATE npcs SET x_coord = ?, y_coord = ?, z_coord = ?, facing = ? WHERE id = ?",
                           (before["x_coord"], before["y_coord"], before["z_coord"], before["facing"], int(ref)))
                done.append(f"NPC {ref} ({before['npc_name']}) back")
            elif did == "insert":
                key = {"spellpattern": "spellid", "spelleffect": "spellid", "itempattern": "patternid", "npcs": "id",
                       "spellgates": "spell"}[tbl]
                arg = ref if tbl == "spellgates" else int(ref)
                now = row_of(db, tbl, f"{key} = ?", (arg,))
                if now is None:
                    continue
                if now != after:
                    kept.append(f"{tbl} {ref}: changed since, left")
                    continue
                if tbl in ("spellpattern", "spelleffect") and has_table(db, "spells") and \
                        db.execute("SELECT count(*) FROM spells WHERE spellid = ?", (arg,)).fetchone()[0]:
                    kept.append(f"{tbl} {ref}: in a character's book, left")
                    continue
                if tbl == "itempattern" and held(db, arg):
                    kept.append(f"item {ref}: someone holds it, left")
                    continue
                db.execute(f"DELETE FROM {tbl} WHERE {key} = ?", (arg,))
                done.append(f"{tbl} {ref} deleted")
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return done, kept


HOLDERS = ("charinventory", "npcitem", "auction_lots", "loottables", "lootpool", "defaultcharinventory", "startitems")


def held(db, pid):
    """Whether a character, a merchant, an auction or a loot table holds the
    item."""
    for table in HOLDERS:
        if has_table(db, table) and "patternid" in columns(db, table) and \
                db.execute(f"SELECT count(*) FROM {table} WHERE patternid = ?", (pid,)).fetchone()[0]:
            return True
    return False


def put_quest_back(db, key, was):
    """A quest's rows and its import mark as they were logged."""
    q, steps, mark, unlocks = was["quest"], was["steps"], was["mark"], was["unlocks"]
    if q is None:
        return
    qid = q["questid"]
    db.execute("DELETE FROM quests WHERE questid = ?", (qid,))
    db.execute("DELETE FROM queststeps WHERE questid = ?", (qid,))
    if has_table(db, "questunlocks"):
        db.execute("DELETE FROM questunlocks WHERE questid = ?", (qid,))
    db.execute(f"INSERT INTO quests ({', '.join(q)}) VALUES ({', '.join('?' * len(q))})", list(q.values()))
    for st in steps:
        db.execute(f"INSERT INTO queststeps ({', '.join(st)}) VALUES ({', '.join('?' * len(st))})", list(st.values()))
    for npc, dest in unlocks:
        db.execute("INSERT OR IGNORE INTO questunlocks (questid, npc, dest) VALUES (?, ?, ?)", (qid, npc, dest))
    if mark is not None:
        db.execute(f"INSERT OR REPLACE INTO quest_import_quests ({', '.join(mark)}) VALUES ({', '.join('?' * len(mark))})",
                   list(mark.values()))


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--apply", action="store_true", help="write it")
    ap.add_argument("--undo", action="store_true", help="put back what a run changed that no one changed since")
    ap.add_argument("--no-backup", action="store_true", help="write without a copy first")
    args = ap.parse_args()
    db = sqlite3.connect(args.db, isolation_level=None)
    if args.undo:
        if not args.no_backup:
            print("backup:", backup(db, args.db))
        done, kept = undo(db)
        for d in done:
            print("  " + d)
        for k in kept:
            print("  kept: " + k)
        print(f"undone: {len(done)} changes, {len(kept)} kept; restart the server")
        return
    for what, line in plan(db):
        print(f"  {what:8} {line}")
    if not args.apply:
        print("(report only: --apply to write it)")
        return
    if not args.no_backup:
        print("backup:", backup(db, args.db))
    said, written, kept, drafts = apply(db)
    for s in said:
        print("  left: " + s)
    print(f"written; the vampire quests imported again: {written}" + (f", kept as a GM changed them: {kept}" if kept else ""))
    for d in drafts:
        print("  still a draft: " + d)
    print("restart the server")


if __name__ == "__main__":
    main()
