#!/usr/bin/env python3
"""The Plane of Sky's Sun, Snow and Desolation chains finished against their guides (the owner, 2026-11-14: "lets add the
sun chain scripted events", "... snow chain ...", "... desolation chain ...": t213, t212, t208), on top of set sky-events
(tools/sky_events_20261114.py, which must be in). One guide-gaps set, `sky-events-2`, written by tools/seed_guide_gaps.py's
machinery (the box needs the whole tools folder), every row it changes kept in guide_set_log, so --undo is exact (undo it
before sky-events'):

  Sun 2   Master Fiharaz's invisible wall ("there is a small ramp there that goes no where - that is the invisible wall"):
          an invisible blocker (CHAR.ESF 0x6F412342 at size 3, a box 15 a side) at the top of the ramp down from the
          courtyard to Imerzan's halls, a quest wall of quest 101305 step 2 (a character past the talisman's hand-in, or
          done, walks through it), and trigger 221 (quest 101305 step 2's trigger event): the wall out of every view
          thirty minutes, so the questor's raid goes through with it ("Only one person is required to log the quest").
          The spot: the one cell of the ground's heightfield whose loss cuts the landing's level from Imerzan's and
          leaves Fiharaz's tower reached (a heightfield search, ~/.cache/skyevents/scripts/nav.py), where sycophant
          120174 stands. The bridges' edges are rails2051's hazards already.
  Sun 4   the rapacious drakes the rancorous drake's placeholders (npcrare, 10%: jib's held figure), its row held out of
          the world, so its death (sky-events' trigger 211) brings Randarin only after the farm.
  Snow 2  the Whirlwind "whirls around on the outskirts of the little igloo village": a loop of nine points round the
          village on its floor (each leg joined by the ground); "Beware being near the ice, as it can nuke you for 6k":
          the seven ice slabs on the village's frozen pond (the disc's zone actors 0x803E497D-F, cmd/hazardscan, each
          slab's radius and 2 more), 6,000 every 10 s (the interval ours).
  Snow 3  "the apes (glacial behemoths) will spawn almost directly on top of your party as you port in ... they tend to
          spawn in pairs": trigger 222, a player at the landing brings two, then rests ten minutes (ours).
  Snow 4  the ordinary Aeyinar ("a random spawn ... every 8 hours she is up"; jib's held placement): a row of her own at
          the spot the Enraged one comes to, Enraged Aeyinar's body, back eight hours after her death, the plane's named
          loot. Her template is jib's held one, by her name, when it lands.
  Deso 1  a patroller comes "in the sw quad of the druid ring" for a placeholder of the sw quad: trigger 212's spawn at
          the patroller row nearest the death (four rows, five at most standing); the Arbiter's first two lives a corpse
          of nothing, the keys and shard on the last ("the last spawn having the Shard"); "she is unconnable" until
          then: trigger 220 keeps an untouchable copy of her in the ring while a player is on the isle, hidden by the
          ring's controller (213) while she fights.
  Deso 2  "if you fail, you must wait one hour for the Elder to respawn": the elder's row back an hour after a death.
  Deso 4  "Be careful around some of the rocks. If you get too close you'll get a warning about them being unstable. 3
          warnings=death": the ten mid-sized boulders round the Quintessence's mountain (the disc's 0x6630AE67, LOD
          sprites 10 across; the big boulders, 0x4B0A2400, are the raid's cover and left), hazards of three strikes
          (the engine's new strikes: a warning a hit, the third a death), a hit every 3 s while one stays (ours).

Refused, nothing written, unless sky-events is in and its triggers 212 and 213 are as it wrote them, slots 220-222 are
free, each row is the creature it should be, the new names are not taken and quest 101305 step 2 is as imported.
Report only by default (but for hazards' column strikes, made as the code's start makes it when it is missing: an undo leaves
it, which an older binary ignores); --apply in one transaction with a gm_audit line; an --apply when the set is in says so and does
nothing; --undo exact. Needs branch sky-events-2's code (hazards' strikes, a spawn's spots, untouchable and noloot):
an older server leaves the rocks out and reads triggers 212, 213 and 220 as problems. Read at the server's start: a
restart after the code and this.

    tools/sky_events2_20261114.py --db eqoa.db [--apply | --undo]
"""
import argparse
import copy
import datetime
import json
import math
import os
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import seed_guide_gaps as sgg  # noqa: E402
import sky_events_20261114 as one  # noqa: E402

SET = "sky-events-2"
WHO, VIA = "dab", "tool"
DETAIL = "The Plane of Sky's Sun, Snow and Desolation chains finished (the owner, 2026-11-14): tools/sky_events2_20261114.py"
W = 4
SRC = f"guide gaps {SET}"
# the tables this set touches beyond seed_guide_gaps' own, so its snapshot (and the undo) has them
EXTRA_TABLES = ("hazards", "questwalls", "npcrare", "npcpaths")

ROWS = {120212: "rapacious drake", 120213: "rapacious drake", 120214: "rapacious drake", 120215: "rapacious drake",
        120216: "rapacious drake", 120217: "rapacious drake", 120218: "rancorous drake", 119637: "Whirlwind",
        119574: "aviak elder", 119534: "Erych Arbiter", 119679: "Enraged Aeyinar", 120174: "imerzan sycophant",
        119543: "erych patroller", 119550: "erych patroller", 119551: "erych patroller", 119558: "erych patroller"}
NEW_NAMES = ("an unseen wall", "aeyinar")

WALL = (5569.0, 370.5, 4664.0)
WHIRL = [[5610.0, 241.62, 6950.0], [5510.0, 240.85, 6870.0], [5490.0, 237.38, 6750.0], [5580.0, 234.55, 6650.0],
         [5700.0, 235.0, 6655.0], [5815.0, 237.38, 6705.0], [5840.0, 237.38, 6800.0], [5810.0, 237.38, 6895.0],
         [5722.98, 237.41, 6927.8]]
# cmd/hazardscan over PLANESKY.ESF's sprites 0x803e497d,e,f (-min-radius 3 -max-radius 14), in the village
ICE = [(5662.4, 6713.9, 5666.14, 6713.1, 9.2), (5706.45, 6692.21, 5712.26, 6693.92, 7.17), (5627.88, 6675.81, 5628.68, 6672.46, 3.0),
       (5641.6, 6694.69, 5642.49, 6691.98, 3.0), (5628.16, 6711.29, 5629.16, 6715.11, 3.0), (5648.08, 6724.62, 5648.55, 6728.03, 3.0),
       (5625.7, 6695.24, 5626.6, 6693.62, 4.47)]
ICE_Y = 234.97
# the zone actors of sprite 0x6630ae67 within 170 of the Quintessence (x, y, z, scale); its LOD box reaches 5.63 off its middle
ROCKS = [(4589.0, 838.3, 7635.6, 0.77), (4594.0, 827.4, 7542.4, 1.05), (4662.4, 864.0, 7720.1, 0.93), (4684.4, 854.9, 7759.7, 0.76),
         (4719.5, 838.6, 7570.2, 0.58), (4723.3, 883.5, 7676.7, 1.0), (4738.8, 883.5, 7731.7, 1.0), (4781.7, 815.5, 7464.8, 0.75),
         (4813.2, 860.3, 7633.8, 0.81), (4837.3, 827.5, 7538.3, 0.82)]
ROCK_REACH = 5.63
PATROLLER_SPOTS = [[4694.72, 568.76, 6224.49], [4816.53, 578.52, 6252.7], [4727.83, 597.02, 6155.03], [4759.66, 568.0, 6356.56]]
SNOW3_LANDING = (5832.8, 176.62, 7079.7)


def first_set():
    """sky-events' triggers 212 and 213 as it wrote them, and as this set has them."""
    t = {x["slot"]: x for x in one.spec()["triggers"]}
    patroller, ring = copy.deepcopy(t[212]), copy.deepcopy(t[213])
    old = (json.dumps(t[212]["steps"], separators=(",", ":")), json.dumps(t[213]["steps"], separators=(",", ":")))
    patroller["steps"][0]["spots"] = PATROLLER_SPOTS
    patroller["notes"] += " Since sky-events-2: at the patroller row nearest where it fell (the guide's quarters)."
    steps = ring["steps"]
    i = next(k for k, s in enumerate(steps) if s.get("npc") == "#119534")  # her first life
    lives = [k for k, s in enumerate(steps) if s.get("npc") == "#119534"]
    for k in lives[:2]:
        steps[k]["noloot"] = True
    steps.insert(i, {"do": "hide", "npc": "Erych Arbiter", "radius": 30, "minutes": 240})
    e = next(k for k, s in enumerate(steps) if s["do"] == "end")
    steps.insert(e + 2, {"do": "hide", "npc": "Erych Arbiter", "radius": 250, "minutes": 0})  # after end and despawn
    ring["notes"] += (" Since sky-events-2: her first two lives a corpse of nothing (the shard and keys on the last), the sleeping"
                      " one (trigger 220) hidden while she fights.")
    return patroller, ring, old


def spec():
    patroller, ring, _ = first_set()
    trig = []
    trig.append({"slot": 220, "name": "Deso 1: The Arbiter Sleeps", "scope": "near", "radius": 250, "world": W,
                 "x": 4760.07, "y": 569.03, "z": 6248.97, "steps": [
                     {"do": "lock", "lock": "arbiter sleeps"},
                     {"do": "awaitnear", "start": True, "radius": 250},
                     {"do": "spawn", "npc": "#119534", "untouchable": True, "spread": 1, "minutes": 240,
                      "world": W, "x": 4760.07, "y": 569.03, "z": 6248.97}],
                 "notes": "Desolation 1 (t208): 'In the middle of the druid ring is a huge Erych, she is unconnable.' While a player "
                          "is on the isle, an untouchable copy of her row on its spot, four hours a run; the ring's controller (213) "
                          "hides it while she fights."})
    trig.append({"slot": 221, "name": "Sun 2: Fiharaz Opens the Wall", "scope": "near", "radius": 250, "world": W,
                 "x": WALL[0], "y": WALL[1], "z": WALL[2], "steps": [
                     {"do": "hide", "npc": "an unseen wall", "radius": 10, "minutes": 30},
                     {"do": "broadcast", "text": "Master Fiharaz speaks a word of fire, and the unseen wall at the courtyard's ramp "
                                                 "opens. It stays open thirty minutes."}],
                 "notes": "Sun 2 (t213): 'After speaking again with Master Fiharaz, he will open an invisible wall for you to pass "
                          "through ... Only one person is required to log the quest.' Fired by quest 101305 step 2 (the "
                          "talisman); everyone may pass for thirty minutes (ours); a questor past it always may (questwalls)."})
    trig.append({"slot": 222, "name": "Snow 3: The Behemoths", "scope": "near", "radius": 120, "world": W,
                 "x": SNOW3_LANDING[0], "y": SNOW3_LANDING[1], "z": SNOW3_LANDING[2], "steps": [
                     {"do": "lock", "lock": "snow 3 behemoths"},
                     {"do": "awaitnear", "start": True, "radius": 20},
                     {"do": "spawn", "npc": "glacial behemoths", "count": 2, "hostile": True, "spread": 12, "minutes": 20},
                     {"do": "wait", "seconds": 600}],
                 "notes": "Snow 3 (t212): 'the apes (glacial behemoths) will spawn almost directly on top of your party as you port "
                          "in and you need to get safe; they tend to spawn in pairs.' The landing's own spot; ten minutes' rest (ours)."})
    sql = []
    # Sun 2: the wall, and the quest wall on it
    sql.append(["INSERT INTO npcs (npc_name, zone, unk_a, unk_b, x_coord, y_coord, z_coord, facing, world, hp, hp_percent, modelid, size, "
                "npc_level, npc_type, npc_id) SELECT 'an unseen wall', zone, unk_a, unk_b, ?, ?, ?, 0, ?, hp, hp_percent, modelid, 3.0, 1, 128, 0 "
                "FROM npcs WHERE id = 162497", [WALL[0], WALL[1], WALL[2], W]])
    sql.append(["UPDATE npcs SET npc_id = id WHERE npc_name = 'an unseen wall' AND world = ? AND npc_id = 0", [W]])
    sql.append(["INSERT INTO questwalls (npcid, questid, step, note, source, item, inner, text) SELECT id, 101305, 2, ?, ?, 0, '', ? "
                "FROM npcs WHERE npc_name = 'an unseen wall' AND world = ?",
                ["Sun 2's invisible wall (t213): past the talisman's hand-in to Master Fiharaz, a character walks through.", SRC,
                 "An unseen wall turns you back. Master Fiharaz keeps this way shut.", W]])
    # Sun 4: the rancorous drake from its placeholders
    for rid in range(120212, 120218):
        sql.append(["INSERT INTO npcrare (npc_row, rare, chance, note, source) VALUES (?, 'rancorous drake', 10, ?, ?)",
                    [rid, "Sun 4 (t213): 'Rapacious Drakes serve as a place holder for the Rancorous drake.' 10% (jib's held figure).", SRC]])
    sql.append(["INSERT INTO npcspawnonly (name, note, source) VALUES ('#120218', ?, ?)",
                ["rancorous drake: the rapacious drakes' rare (npcrare), by sky-events-2", SRC]])
    # Snow 2: the Whirlwind's round, the ice
    sql.append(["INSERT INTO npcpaths (npcid, world, mode, pause, speed, points) VALUES (119637, ?, 'loop', 0, '5', ?)",
                [W, json.dumps([{"x": p[0], "y": p[1], "z": p[2], "pause": 0} for p in WHIRL])]])
    for x1, z1, x2, z2, r in ICE:
        sql.append(["INSERT INTO hazards (name, world, x1, y1, z1, x2, y2, z2, radius, below, above, damage, every_ms, words, note, source, "
                    "on_move) VALUES ('The ice', ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, 6000, 10000, ?, ?, ?, 0)",
                    [W, x1, ICE_Y, z1, x2, ICE_Y, z2, round(r + 2, 2), "The ice cracks under you, and its cold bites for %d.",
                     "Snow 2 (t212): 'Beware being near the ice, as it can nuke you for 6k.' An ice slab of the village's pond "
                     "(cmd/hazardscan, sprites 0x803E497D-F), its radius and 2.", SRC]])
    # Snow 4: the ordinary Aeyinar
    sql.append(["INSERT INTO npcs (npc_name, zone, unk_a, unk_b, x_coord, y_coord, z_coord, facing, world, hp, hp_percent, modelid, size, "
                "npc_level, npc_type, npc_id) SELECT 'Aeyinar', zone, unk_a, unk_b, x_coord, y_coord, z_coord, facing, world, hp, hp_percent, "
                "modelid, size, 64, 0, 0 FROM npcs WHERE id = 119679", []])
    sql.append(["UPDATE npcs SET npc_id = id WHERE npc_name = 'Aeyinar' AND world = ? AND npc_id = 0", [W]])
    sql.append(["INSERT INTO npcrespawn (npc_row, seconds, note, source) SELECT id, 28800, ?, ? FROM npcs WHERE npc_name = 'Aeyinar' AND world = ?",
                ["Snow 4 (t212): 'Aeyinar is an 8 hour spawn'.", SRC, W]])
    sql.append(["INSERT INTO lootcreature (npc_name, region, profile, source) VALUES ('aeyinar', 'plane of sky', 'named', ?)", [SRC]])
    # Deso 1: the patroller's quarter, the Arbiter's lives (triggers of sky-events)
    for t in (patroller, ring):
        sql.append(["UPDATE gmtriggers SET steps = ?, notes = ? WHERE slot = ?", [json.dumps(t["steps"], separators=(",", ":")), t["notes"], t["slot"]]])
    # Deso 2: the elder's hour
    sql.append(["INSERT INTO npcrespawn (npc_row, seconds, note, source) VALUES (119574, 3600, ?, ?)",
                ["Deso 2 (t208): 'if you fail, you must wait one hour for the Elder to respawn'.", SRC]])
    # Deso 4: the unstable rocks
    for x, y, z, s in ROCKS:
        sql.append(["INSERT INTO hazards (name, world, x1, y1, z1, x2, y2, z2, radius, below, above, damage, every_ms, words, note, source, "
                    "on_move, strikes) VALUES ('An unstable rock', ?, ?, ?, ?, ?, ?, ?, ?, 3, 8, 1000000, 3000, ?, ?, ?, 0, 3)",
                    [W, x, y, z, x, y, z, round(ROCK_REACH * s + 2, 2), "The rock shifts as you come near: it is unstable (warning %d of 3).",
                     f"Desolation 4 (t208): '3 warnings=death'. The zone actor of sprite 0x6630AE67 at ({x}, {y}, {z}), scale {s}: its "
                     f"box's reach and 2.", SRC]])
    return {"set": SET, "triggers": trig, "sql": sql,
            "step_events": [
                {"quest": quest_key_sun(), "step": 2, "target": "Master Fiharaz",
                 "events": [{"on": "done", "do": "trigger", "trigger": "Sun 2: Fiharaz Opens the Wall"}]}]}


_SUN_KEY = None


def quest_key_sun():
    return _SUN_KEY or "Portal/plane-of-sky/sun"


def audit(db, action, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if sgg.has(db, "accountinfo") else None
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', ?, 'guide_set_log', ?, NULL, ?, ?)",
               (when, acct[0] if acct else 0, WHO, VIA, action, json.dumps({"set": SET}), json.dumps(after, sort_keys=True), DETAIL))


def refusals(db, s):
    out = []
    if not (sgg.has(db, sgg.LOG) and db.execute(f"SELECT 1 FROM {sgg.LOG} WHERE setname = 'sky-events'").fetchone()):
        out.append("set sky-events is not in: apply tools/sky_events_20261114.py first")
    _, _, old = first_set()
    for slot, steps in zip((212, 213), old):
        r = db.execute("SELECT steps FROM gmtriggers WHERE slot = ?", (slot,)).fetchone()
        if not r or json.loads(r[0]) != json.loads(steps):
            out.append(f"trigger {slot} is not as sky-events wrote it")
    for t in s["triggers"]:
        r = db.execute("SELECT name FROM gmtriggers WHERE slot = ?", (t["slot"],)).fetchone()
        if r:
            out.append(f"trigger slot {t['slot']} is taken: {r[0]!r}")
        if db.execute("SELECT 1 FROM gmtriggers WHERE lower(name) = lower(?)", (t["name"],)).fetchone():
            out.append(f"a trigger is named {t['name']!r} already")
    for rid, name in {**ROWS, 162497: "wind wall"}.items():
        r = db.execute("SELECT npc_name, world FROM npcs WHERE id = ?", (rid,)).fetchone()
        if not r or r[0].strip().lower() != name.lower() or r[1] != W:
            out.append(f"npcs row {rid} is {r}, not {name!r} of world {W}")
    if db.execute("SELECT modelid FROM npcs WHERE id = 162497").fetchone() != (0x6F412342,):
        out.append("row 162497 is not an invisible blocker (0x6F412342)")
    for n in NEW_NAMES:
        for tbl, col in (("npcs", "npc_name"), ("npctemplate", "name"), ("lootcreature", "npc_name"), ("npcspawnonly", "name")):
            if db.execute(f"SELECT 1 FROM {tbl} WHERE lower(trim({col})) = ?", (n,)).fetchone():
                out.append(f"{tbl} names {n!r} already")
    for rid in range(120212, 120218):
        if db.execute("SELECT 1 FROM npcrare WHERE npc_row = ?", (rid,)).fetchone():
            out.append(f"npcrare has row {rid} already")
    for k, what in (("SELECT 1 FROM npcspawnonly WHERE name = '#120218'", "#120218 is held out already"),
                    ("SELECT 1 FROM npcpaths WHERE npcid = 119637", "the Whirlwind has a path already"),
                    ("SELECT 1 FROM npcrespawn WHERE npc_row = 119574", "the aviak elder has a respawn row already"),
                    ("SELECT 1 FROM hazards WHERE name IN ('The ice', 'An unstable rock') AND world = 4", "the ice or rocks are hazards already")):
        if db.execute(k).fetchone():
            out.append(what)
    r = db.execute("SELECT kind, target, events FROM queststeps WHERE questid = 101305 AND step = 2").fetchone()
    if r != ("give", "Master Fiharaz", ""):
        out.append(f"quest 101305 step 2 is not as imported: {r}")
    return out


def check(db):
    wall = db.execute("SELECT id FROM npcs WHERE npc_name = 'an unseen wall' AND world = 4").fetchall()
    if len(wall) != 1 or not db.execute("SELECT 1 FROM questwalls WHERE npcid = ? AND questid = 101305", wall[0]).fetchone():
        raise SystemExit("the wall and its quest wall are not one row each")
    if db.execute("SELECT count(*) FROM hazards WHERE source = ?", (SRC,)).fetchone()[0] != len(ICE) + len(ROCKS):
        raise SystemExit("the hazards are not all in")
    ev = json.loads(db.execute("SELECT events FROM queststeps WHERE questid = 101305 AND step = 2").fetchone()[0])
    if not ev or ev[0].get("trigger") != "Sun 2: Fiharaz Opens the Wall":
        raise SystemExit("quest 101305 step 2 fires no trigger")


def main():
    global _SUN_KEY
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--undo", action="store_true")
    ap.add_argument("--dump", action="store_true", help="print the set's rows as JSON and stop")
    a = ap.parse_args()
    sgg.TABLES = tuple(sgg.TABLES) + tuple(t for t in EXTRA_TABLES if t not in sgg.TABLES)
    if a.dump:
        print(json.dumps(spec(), indent=1))
        return
    db = sqlite3.connect(a.db, timeout=5, isolation_level=None)
    if a.undo:
        sgg.undo(db, SET, print, extra=lambda d, n: audit(d, "undo", {"rows": n}))
        return
    if sgg.has(db, sgg.LOG) and db.execute(f"SELECT 1 FROM {sgg.LOG} WHERE setname = ?", (SET,)).fetchone():
        print(f"set {SET} is in already: nothing to do (--undo takes it out)")
        return
    # the strikes column the code's start adds (database/hazards.go), made here as it makes it, for a database an older
    # binary last opened: left by an undo (the code reads it; an older binary's SELECT names its columns)
    if not db.execute("SELECT 1 FROM pragma_table_info('hazards') WHERE name = 'strikes'").fetchone():
        db.execute("ALTER TABLE hazards ADD COLUMN strikes INTEGER NOT NULL DEFAULT 0")
        print("hazards: made its column strikes (as the code's start does)")
    r = db.execute("SELECT key FROM quest_import_quests WHERE questid = 101305").fetchone()
    if r:
        _SUN_KEY = r[0]
    s = spec()
    bad = refusals(db, s)
    if bad:
        for b in bad:
            print("refused:", b)
        sys.exit(1)

    def extra(d, counts):
        check(d)
        audit(d, "apply", counts)
    sgg.apply(db, s, a.apply, print, extra=extra if a.apply else None)


if __name__ == "__main__":
    main()
