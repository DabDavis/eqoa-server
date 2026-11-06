#!/usr/bin/env python3
"""Zaoran Plateau, peopled: EQOA-Data's guide (github.com/Jadiction/EQOA-Data:
Quests/Portal/zaoran-plateau.json: Rallos Zek, Tallon Zek, The Unnamed
Champion, the Wailing Ghost's two quests, Vallon Zek) put on the plateau, where
the server had only the quests' own NPCs (tools/quests/npcs/qe.json: Arios,
the Rallosian Arbiter, the Wailing Ghost, the Zek Sentry, Tallon and Vallon
Zek; the vampires' Sanctuary) and Return Home's ZP1 marker (the user's "check
our massive local EQOA DB for ZP data and work on a patch to populate it",
2026-11-06). The merged research database (merged-content 20261020, every
private server's NPC rows and the world research) holds no placement on the
plateau but ZP1 and one unnamed Return Home marker; FrontiersForge's sightings
and Sandstorm's census never went there. So every spot here is ours.

Where things are, read off the disc: the Secrets world (world 5, DATA3/
SECRETS.ESF), zone 5 (x 4000-6000, z 6000-8000), CLIENT's "Zaoran Plateau":
islands in a sea of lava (the zone BSP's lava leaves, internal/ground/env.go).
The main island's floor is at 147.6. Players arrive at ZP1 (4899, 7049), in the
courtyard of the castle "to the left of the port in spot" (walls and towers at
161.6 and 175.4). North-east, a walled town is cut by two lava trenches (their
floor 90.6) with a bridge across each at 146.5, (5530, 6746) and (5776, 6594):
the Akessan bridges, the way to Rallos Zek, and past its north wall an island
at 218 that no walk reaches, his arena. South, up the mountain, Tallon's
fortress: three keeps on the surface (216) and below them an entrance hall at
197.5, a bench room at 179.9 (ramp 1's bottom), a level of pillars and coffins
at 167.5 (ramp 3's captains: the 4643276E pillars, the A6C3FBEF coffins) and
the deepest room at 162.4, with tables ("the brown squares are table-like
objects"): Tallon's. Vallon's island lies west (243-253); the highest, the
south-east island at 336.2, is Razeal's ("summoned to the isle ... Port up").

What is ours: every spot and count, every level (the guide gives cons to a
level 60: "con red@60"), the names the guide leaves out (the Rallosian
initiates', captain's and commander's, the bridge guards', the fortress's
golems, widows, captains and archers, the mists'), every body (copied from a
row of this server: the Priest of Rallos Zek's orc for the Rallosians and
Zek, a Rustel minotaur for Mograthar, a granite golem, the giant black widow,
the Bolash Bats' bat, the toxic mist, Mayong Mistmoore's for Razeal), the
portals' names and that they need only a level (the Mark of Rallos, the
gravestones' ritual and Razeal's teleporter are not in: handlers/waysender.go
holds the trips), and the Rallosian loot table (the server's own Rallosian
items, levels 44-60). Not placed: the catapult (no body to wear), the
gravestones and posts that the encounters spawn, the Rage of Zek (a bug, the
guide says), the "old info" lieutenants and biddings.

    tools/seed_zp.py --ground worlds/ground [--db eqoa.db]            report
    tools/seed_zp.py --ground worlds/ground --apply                     write it
    tools/seed_zp.py --undo                                             take it all back

Rows go in in one transaction with npc_id 0, logged in zp_seed_npcs; the rules
the creatures need (npcaggro: the orc and vampire bodies are players' races'
here, so nothing on them attacks on sight without one; npcmove) and the loot
(loottables, npcloot) are written where no row of that name exists, logged in
zp_seed_rules and zp_seed_loot, and --undo deletes exactly what a run wrote (a
GM's own rows are never touched). The plan is refused when a ground file
differs from the one its anchors were read off. A copy of the database is
made first (--no-backup to skip). Stop the server to apply, start it after.
"""
import argparse
import collections
import datetime
import json
import os
import sqlite3
import sys

# the ground reader is seed_wildlife's, the zone BSPs' and the ring of spots
# seed_pod's (the server's own formats)
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from seed_wildlife import Ground  # noqa: E402
from seed_pod import Env, anchor_floor, has_table, spots  # noqa: E402

DEFAULT_DB = "eqoa.db"
DEFAULT_GROUND = "worlds/ground"
SOURCE = "zp-seed"
SPREAD, NEAR, SPACING = 30.0, 6.0, 5.0

# npc_type: 0 hostile by nature; 0x80 not attackable; 0x100 travel
HOSTILE, FRIEND, TRAVEL = 0, 0x80, 0x180

ORC, MINOTAUR, GOLEM = "priest of rallos zek", "rustel minotaur", "granite golem"
WIDOW, BAT, MIST, VAMPIRE, PORTAL = "giant black widow", "bolash bats", "toxic mist", "mayong mistmoore", "dark portal"

# Each entry: an anchor (x, z) and the height of the floor meant there (y: of
# the floors at the anchor, the nearest; every spot of a group within NEAR of
# it, at most SPREAD off), checked against the ground files when the plan is
# made. body: the row (by name, lower case) every look column is copied from;
# its size times scale. spacing: how far each one stands from every NPC placed
# before it (SPACING when not given). aggro: an npcaggro row (the body
# is a players' race's, entity/aggro.go). loot: (chance, rolls) of the
# Rallosian table. Levels,
# counts and spots are ours. New entries go at the end: a row's default facing
# counts the rows before it, so one put in the middle turns every row after.
PLAN = [
    # ---- the main island: "farm yourself the blood vial from the bolash bats"
    # (no blood vial on this server: the bats drop nothing yet), round the
    # cemetery north of the castle, the east and the south-east rise
    dict(name="Bolash Bats", body=BAT, level=55, spacing=10, kind=HOSTILE, scale=2.0,
         at=(5060, 6880), y=147.6, count=5, role="the cemetery's (the blood vial is not in)"),
    dict(name="Bolash Bats", body=BAT, level=56, spacing=10, kind=HOSTILE, scale=2.0,
         at=(5320, 6980), y=147.6, count=5, role="the east of the main island"),
    dict(name="Bolash Bats", body=BAT, level=57, spacing=10, kind=HOSTILE, scale=2.0,
         at=(5250, 7160), y=160.8, count=5, role="the south-east rise"),

    # ---- Rallos Zek: "approach the first Akessan bridge ... On the other side
    # of the bridge you'll see Initiates. Kill these until a Captain spawns.
    # Kill the Captain and a Commander will spawn ... head to the next bridge
    # and cross it. You'll see more Initiates and a Zek Trainer along with his
    # unattackable pet (Mograthar?) ... a portal over in the back corner".
    # The bridges across the town's two lava trenches; the captain and the
    # commander stand (this server spawns nothing on a kill), Mograthar can be
    # fought from the start
    dict(name="Rallosian bridge guard", body=ORC, level=61, kind=HOSTILE, aggro=True, loot=(20, 1),
         at=(5530, 6746), y=146.5, count=2, role="the first Akessan bridge"),
    dict(name="Rallosian initiate", body=ORC, level=58, spacing=12, kind=HOSTILE, aggro=True, loot=(15, 1),
         at=(5560, 6670), y=147.6, count=6, role="past the first bridge"),
    dict(name="Rallosian captain", body=ORC, level=61, kind=HOSTILE, aggro=True, loot=(50, 1), scale=1.15,
         at=(5610, 6650), y=147.6, role="past the first bridge (live: spawns when the initiates die)"),
    dict(name="Rallosian commander", body=ORC, level=62, kind=HOSTILE, aggro=True, loot=(100, 1), scale=1.25,
         at=(5620, 6705), y=147.6, role="past the first bridge (live: drops the Mark of Rallos, not in)"),
    dict(name="Rallosian bridge guard", body=ORC, level=61, kind=HOSTILE, aggro=True, loot=(20, 1),
         at=(5776, 6594), y=146.5, count=2, role="the second Akessan bridge"),
    dict(name="Rallosian initiate", body=ORC, level=58, spacing=12, kind=HOSTILE, aggro=True, loot=(15, 1),
         at=(5760, 6480), y=147.6, count=5, role="past the second bridge"),
    dict(name="Zek trainer", body=ORC, level=62, kind=HOSTILE, aggro=True, loot=(50, 1), scale=1.15,
         at=(5600, 6460), y=147.6, role="Mograthar's trainer"),
    dict(name="Mograthar", body=MINOTAUR, level=63, kind=HOSTILE, scale=2.5,
         at=(5615, 6462), y=147.6, role="\"a giant minotaur-looking pet\" (live: unattackable until the trainer dies)"),
    dict(name="Portal of Rallos", body=PORTAL, level=60, kind=TRAVEL,
         at=(5525, 6410), y=152.4, facing=0, role="to Rallos Zek's arena, level 60 (live: the Mark of Rallos)"),
    # his arena, the island past the town's north wall; his body is the
    # orc priests' at twice their size (no row of him)
    dict(name="Portal to the Akessan Bridges", body=PORTAL, level=60, kind=TRAVEL,
         at=(5670, 6295), y=218.2, facing=96, role="back to the Portal of Rallos"),
    dict(name="Rallos Zek", body=ORC, level=66, kind=HOSTILE, aggro=True, loot=(100, 3), scale=2.0,
         at=(5720, 6230), y=218.3, facing=160, role="boss (\"a hard mob there is no denying that\")"),

    # ---- Tallon's fortress: "The first floor is suppose to be filled with
    # Tallon Rage's ... huge black widow spiders to your left and right (they
    # seem to be a very common mob throughout the fortress) ... Once you reach
    # the bottom of ramp #1 you will spawn golems (6-8) ... At the bottom of
    # this ramp are more golems ... 4 pillars to the ceiling, and there are 6
    # or 8 things in the room that look like coffins ... you will find
    # captains, about 10-15, roaming all over ... the Archers that pop when you
    # spawn TZ himself". All stand (a wander indoors would walk through walls).
    # Tallon himself is the quest set's, on the pillar level; the archers
    # stand in the deepest room, the tables'
    dict(name="Tallon's Rage", body=ORC, level=64, spacing=8, kind=HOSTILE, aggro=True, loot=(25, 1), scale=1.4,
         at=(4733, 7640), y=197.5, count=4, role="the entrance hall (live: die with the Zek Sentry)"),
    dict(name="huge black widow", body=WIDOW, level=58, spacing=8, kind=HOSTILE, scale=2.5, roam=0,
         at=(4690, 7600), y=197.5, count=2, role="the entrance hall's left"),
    dict(name="huge black widow", body=WIDOW, level=58, spacing=8, kind=HOSTILE, scale=2.5, roam=0,
         at=(4770, 7625), y=197.5, role="the entrance hall's right"),
    dict(name="fortress golem", body=GOLEM, level=59, spacing=7, kind=HOSTILE,
         at=(4710, 7612), y=179.9, count=6, role="the bottom of ramp 1"),
    dict(name="fortress golem", body=GOLEM, level=59, spacing=7, kind=HOSTILE,
         at=(4780, 7720), y=167.5, count=6, role="the bottom of ramp 2"),
    dict(name="Zek captain", body=ORC, level=60, spacing=10, kind=HOSTILE, aggro=True, loot=(20, 1), scale=1.15,
         at=(4683, 7687), y=167.5, count=10, role="the room of pillars and coffins (ramp 3)"),
    dict(name="huge black widow", body=WIDOW, level=58, spacing=8, kind=HOSTILE, scale=2.5, roam=0,
         at=(4640, 7745), y=167.5, count=2, role="a dead-end hall (\"Black Widow Spiders which means Dead End\")"),
    dict(name="Zek archer", body=ORC, level=60, spacing=6, kind=HOSTILE, aggro=True, loot=(20, 1),
         at=(4728, 7641), y=162.4, count=4, role="Tallon's room, the deepest (live: spawn with him)"),

    # ---- Razeal Moloch: "After this is done, they all disappear and a post
    # spawns in the center. Speak w/it and he is summoned to the isle ... Port
    # up and own ... The con-able 'Mist of Razeal' are supposed to be up during
    # the encounter". The post by the cemetery's Wailing Ghost (the gravestones'
    # ritual is not in), the isle the highest, south-east; he stands always up
    dict(name="Razeal's Summoning Post", body=PORTAL, level=60, kind=TRAVEL,
         at=(5025, 6800), y=147.6, facing=128, role="to Razeal's isle, level 60 (live: the gravestones' ritual)"),
    dict(name="Portal to the Plateau", body=PORTAL, level=60, kind=TRAVEL,
         at=(5295, 7570), y=336.2, facing=64, role="back to the Summoning Post"),
    dict(name="Razeal Moloch", body=VAMPIRE, level=65, kind=HOSTILE, aggro=True, scale=1.4,
         at=(5340, 7600), y=336.2, facing=192, role="boss (\"Tankable with 14 toons\"; the Pendant of Razeal is not in)"),
    dict(name="Mist of Razeal", body=MIST, level=58, spacing=10, kind=HOSTILE,
         at=(5320, 7630), y=336.2, count=6, role="\"supposed to be up during the encounter\""),
]

# the server's own Rallosian items (itempattern), each for creatures of
# levels 55-70 (ours: the guide names no drop of theirs)
LOOT_TABLE = "zp rallosian"
LOOT_ITEMS = ["Rallosian War Shield", "Rallosian Battle Shield", "Rallosian Longsword", "Rallosian Shadow Dirk",
              "Rallosian Totem", "Rallosian War Totem", "Rallosian Imperial Orb", "Rallosian Orb of Power",
              "Rallosian Imperial Shrouds", "Rallosian War Blade", "Rallosian Imperial Glaive",
              "Rallosian Imperial Blade", "Rallosian Beheader", "Rallosian Symbol", "Arcane Rallosian Dagger",
              "golden rallosian runed charm", "necklace of Rallosian runes"]
LOOT_LEVELS = (55, 70)


# ---- the plan ---------------------------------------------------------------------

def bodies(db):
    """The first row of each body (a name, lower case)."""
    out = {}
    for name in sorted({e["body"] for e in PLAN}):
        r = db.execute("SELECT * FROM npcs WHERE lower(trim(npc_name)) = ? ORDER BY id LIMIT 1", (name,)).fetchone()
        if r is not None:
            out[name] = dict(r)
    return out


def loot_items(db):
    """The loot table's items by name ((name, patternid) found, names missing)."""
    found, missing = [], []
    for name in LOOT_ITEMS:
        r = db.execute("SELECT patternid FROM itempattern WHERE lower(trim(itemname)) = ? ORDER BY patternid LIMIT 1",
                       (name.lower(),)).fetchone() if has_table(db, "itempattern") else None
        if r is None:
            missing.append(name)
        else:
            found.append((name, r[0]))
    return found, missing


def plan(db, ground_dir):
    rows, missing, unplaced, off = [], [], [], []
    body = bodies(db)
    g = Ground(os.path.join(ground_dir, "5.ground"))
    ep = os.path.join(ground_dir, "5.env")
    env = Env(ep) if os.path.exists(ep) else None
    taken = []
    for e in PLAN:
        b = body.get(e["body"])
        if b is None:
            missing.append(e["body"])
            continue
        base = anchor_floor(g, e["at"], e["y"])
        if base is None or abs(base - e["y"]) > 1.0:
            # the ground is not the one the anchor was read off
            off.append(f"{e['name']} at {e['at']}: floor {base}, planned {e['y']}")
            continue
        for i in range(e.get("count", 1)):
            placed = None
            spread = SPREAD if e.get("count", 1) > 1 else NEAR
            for x, y, z in spots(g, env, e["at"], base, False, spread):
                if env is not None and env.flags(x, y + 0.5, z) & 4:
                    continue  # never on the lava
                gap = e.get("spacing", SPACING)
                if all((x - px) ** 2 + (z - pz) ** 2 >= gap ** 2 for px, pz in taken):
                    placed = (x, y, z)
                    break
            if placed is None:
                unplaced.append(e["name"])
                continue
            x, y, z = placed
            taken.append((x, z))
            row = dict(b)
            row.pop("id", None)
            row.update(npc_name=e["name"], npc_level=e["level"], npc_type=e["kind"], world=5,
                       x_coord=x, y_coord=round(y, 3), z_coord=z,
                       facing=e.get("facing", (37 * (len(rows) + 1)) % 256),
                       size=round(float(b.get("size") or 1.0) * e.get("scale", 1.0), 3),
                       npc_id=0, dt_kill=None, zone=0)
            rows.append(dict(row=row, body=e["body"], body_row=b["id"], role=e["role"]))
    rules, loot = [], []
    seen = set()
    for e in PLAN:
        key = e["name"].strip().lower()
        if key in seen:
            continue
        seen.add(key)
        if "roam" in e:
            rules.append(("npcmove", "name", key, dict(name=key, roam=e["roam"], radius=0.0, water="",
                                                       note="Zaoran Plateau", source=SOURCE)))
        if e.get("aggro"):
            # npcaggro is by the NPC's own name (entity/aggro.go)
            rules.append(("npcaggro", "npc_name", e["name"], dict(npc_name=e["name"], aggressive=1, radius=0.0,
                                                                   note="Zaoran Plateau: a players' race's body",
                                                                   source=SOURCE)))
        if e.get("loot"):
            chance, rolls = e["loot"]
            loot.append(dict(npc_name=key, tablename=LOOT_TABLE, chance=chance, rolls=rolls,
                             note="Zaoran Plateau (ours)", source=SOURCE))
    items, items_missing = loot_items(db)
    cols = [r[1] for r in db.execute("PRAGMA table_info(npcs)")]
    return dict(rows=rows, rules=rules, loot=loot, items=items, items_missing=items_missing,
                missing=sorted(set(missing)), unplaced=unplaced, off=off, cols=cols)


# ---- apply and undo ---------------------------------------------------------------

LOG_DDL = [
    "CREATE TABLE zp_seed_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)",
    "CREATE TABLE zp_seed_npcs (npc_row INTEGER PRIMARY KEY, name TEXT NOT NULL, body_row INTEGER NOT NULL, "
    "role TEXT NOT NULL)",
    "CREATE TABLE zp_seed_rules (tbl TEXT NOT NULL, keycol TEXT NOT NULL, name TEXT NOT NULL, "
    "PRIMARY KEY (tbl, name))",
    "CREATE TABLE zp_seed_loot (tbl TEXT NOT NULL, row_key TEXT NOT NULL, PRIMARY KEY (tbl, row_key))",
]


def seeded(db):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE name = 'zp_seed_npcs'").fetchone()[0] > 0


def seq_of(db, table):
    """A table's AUTOINCREMENT counter as text ("" when it has none yet)."""
    r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = ?", (table,)).fetchone() \
        if has_table(db, "sqlite_sequence") else None
    return "" if r is None else str(r[0])


def restore_seq(db, table, before):
    """Puts a table's counter back where the seed found it, when nothing has
    taken an id above it since."""
    top = db.execute(f"SELECT max(id) FROM {table}").fetchone()[0] or 0
    if before == "":
        if top == 0:
            db.execute("DELETE FROM sqlite_sequence WHERE name = ?", (table,))
    elif top <= int(before):
        db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = ?", (int(before), table))


def apply(db, p):
    if seeded(db):
        sys.exit("this database already holds the Zaoran Plateau seed (zp_seed_npcs): --undo it first")
    if p["missing"] or p["unplaced"] or p["off"]:
        sys.exit(f"not written: bodies missing {p['missing']}, unplaced {p['unplaced']}, off {p['off']}")
    cols = [c for c in p["cols"] if c != "id"]
    db.execute("BEGIN IMMEDIATE")
    try:
        for stmt in LOG_DDL:
            db.execute(stmt)
        meta = dict(when=datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds"),
                    seq_before=seq_of(db, "npcs"), loot_seq_before=seq_of(db, "loottables"),
                    rows=str(len(p["rows"])))
        for r in p["rows"]:
            cur = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                             [r["row"].get(c) for c in cols])
            db.execute("INSERT INTO zp_seed_npcs VALUES (?, ?, ?, ?)",
                       (cur.lastrowid, r["row"]["npc_name"], r["body_row"], r["role"]))
        wrote = 0
        for tbl, keycol, key, vals in p["rules"]:
            if not has_table(db, tbl):
                continue
            if db.execute(f"SELECT count(*) FROM {tbl} WHERE {keycol} = ?", (key,)).fetchone()[0]:
                continue  # a GM's (or another seed's) row stands
            names = list(vals)
            db.execute(f"INSERT INTO {tbl} ({', '.join(names)}) VALUES ({', '.join('?' * len(names))})",
                       [vals[n] for n in names])
            db.execute("INSERT INTO zp_seed_rules VALUES (?, ?, ?)", (tbl, keycol, key))
            wrote += 1
        tied = 0
        if has_table(db, "loottables") and has_table(db, "npcloot") and p["items"]:
            if not db.execute("SELECT count(*) FROM loottables WHERE tablename = ?", (LOOT_TABLE,)).fetchone()[0]:
                for _, pid in p["items"]:
                    cur = db.execute("INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel, source) "
                                     "VALUES (?, ?, 1, ?, ?, ?)", (LOOT_TABLE, pid, *LOOT_LEVELS, SOURCE))
                    db.execute("INSERT INTO zp_seed_loot VALUES ('loottables', ?)", (str(cur.lastrowid),))
            for b in p["loot"]:
                if db.execute("SELECT count(*) FROM npcloot WHERE npc_name = ? AND tablename = ?",
                              (b["npc_name"], b["tablename"])).fetchone()[0]:
                    continue
                db.execute("INSERT INTO npcloot (npc_name, tablename, chance, rolls, note, source) "
                           "VALUES (?, ?, ?, ?, ?, ?)",
                           (b["npc_name"], b["tablename"], b["chance"], b["rolls"], b["note"], b["source"]))
                db.execute("INSERT INTO zp_seed_loot VALUES ('npcloot', ?)", (b["npc_name"],))
                tied += 1
        meta["rules"] = str(wrote)
        meta["loot"] = str(tied)
        db.executemany("INSERT INTO zp_seed_meta VALUES (?, ?)", sorted(meta.items()))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return wrote, tied


def undo(db):
    if not seeded(db):
        sys.exit("no Zaoran Plateau seed to undo here")
    meta = dict(db.execute("SELECT key, value FROM zp_seed_meta"))
    db.execute("BEGIN IMMEDIATE")
    try:
        n = db.execute("DELETE FROM npcs WHERE id IN (SELECT npc_row FROM zp_seed_npcs)").rowcount
        for tbl, keycol, key in db.execute("SELECT tbl, keycol, name FROM zp_seed_rules").fetchall():
            if has_table(db, tbl):
                db.execute(f"DELETE FROM {tbl} WHERE {keycol} = ? AND source = ?", (key, SOURCE))
        for tbl, key in db.execute("SELECT tbl, row_key FROM zp_seed_loot").fetchall():
            if tbl == "loottables" and has_table(db, tbl):
                db.execute("DELETE FROM loottables WHERE id = ? AND source = ?", (int(key), SOURCE))
            elif tbl == "npcloot" and has_table(db, tbl):
                db.execute("DELETE FROM npcloot WHERE npc_name = ? AND tablename = ? AND source = ?",
                           (key, LOOT_TABLE, SOURCE))
        restore_seq(db, "npcs", meta.get("seq_before", ""))
        if has_table(db, "loottables"):
            restore_seq(db, "loottables", meta.get("loot_seq_before", ""))
        for t in ("zp_seed_loot", "zp_seed_rules", "zp_seed_npcs", "zp_seed_meta"):
            db.execute(f"DROP TABLE {t}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return n


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-zpseed"
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def report(p, verbose=False):
    by = collections.Counter(r["row"]["npc_name"] for r in p["rows"])
    print(f"{len(p['rows'])} NPCs ({len(by)} names) in world 5; {len(p['rules'])} rules (npcaggro, npcmove); "
          f"the {LOOT_TABLE!r} table of {len(p['items'])} items for {len(p['loot'])} names")
    if p["missing"]:
        print("bodies missing (no row of that name):", ", ".join(p["missing"]))
    if p["items_missing"]:
        print("loot items missing (left out):", ", ".join(p["items_missing"]))
    if p["unplaced"]:
        print("no spot found for:", ", ".join(p["unplaced"]))
    for o in p["off"]:
        print("the ground differs:", o)
    for r in p["rows"] if verbose else []:
        row = r["row"]
        print(f"  {row['npc_name']:30} L{row['npc_level']:<3} ({row['x_coord']:.1f}, {row['y_coord']:.1f}, "
              f"{row['z_coord']:.1f}) f{row['facing']:<3} type {row['npc_type']:<3} size {row['size']:.2f} "
              f"body {r['body']}")
    if not verbose:
        for name, n in sorted(by.items()):
            print(f"  {name} x{n}" if n > 1 else f"  {name}")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--ground", default=DEFAULT_GROUND, help="the ground files' folder (worlds/ground)")
    ap.add_argument("--apply", action="store_true", help="write the NPCs, their rules and the loot")
    ap.add_argument("--undo", action="store_true", help="delete everything a seed wrote")
    ap.add_argument("--no-backup", action="store_true", help="write without a copy first")
    ap.add_argument("--verbose", action="store_true", help="list every NPC and its spot")
    ap.add_argument("--json", help="write the plan to this file")
    args = ap.parse_args()
    db = sqlite3.connect(args.db)
    db.row_factory = sqlite3.Row
    if args.undo:
        if not args.no_backup:
            print("backup:", backup(db, args.db))
        print(f"deleted {undo(db)} NPCs and the rules and loot the seed wrote")
        return
    p = plan(db, args.ground)
    report(p, args.verbose)
    if args.json:
        with open(args.json, "w") as f:
            json.dump([dict(r["row"], body=r["body"], role=r["role"]) for r in p["rows"]], f, indent=1, default=str)
    if args.apply:
        if not args.no_backup:
            print("backup:", backup(db, args.db))
        wrote, tied = apply(db, p)
        print(f"wrote {len(p['rows'])} NPCs, {wrote} rules and {tied} loot bindings; restart the server")
    else:
        print("(report only: --apply to write it)")


if __name__ == "__main__":
    main()
