#!/usr/bin/env python3
"""Parathior raised by the level 49 epics' last ritual and tuned to the era's guides (Heirloom petition 2990,
Jibrish, 2026-11-25: "Parathior should be a pop spawn for the 49 epic quests, and he also should be tuned. Look up
in era sources and tune him and configure the quest"). It builds board entry 1056's section 3 (jibrish's research,
2026-11-11) with the five minutes its code ask A waited on, done here with a GM trigger's wait.

THE SOURCES
  [GF] MasterVG782's GameFAQs walkthrough (the owner's copy, 2026-11-21), "49 EPIC END -- PARATHIOR": "Once you
       speak with Oransan, a ball of light will appear at the top of the tower and you will have around 5 minutes
       to get ready for battle ... Once the ball of energy disappears, Parathior will show up walking down the
       mountain. Parathior is a bit harder than Chardith to kill, but he isn't that much harder; just make sure to
       avoid his 1,600ish AoE".
  [Prima] the Frontiers guide (2003) p.370 "summon forth Parathior", "After five minutes" (board 1056).
  [AZ] Allakhazam quest 694: speaking with Orinas "will also spawn" him; "Para has a nasty AE (approx 1200
       points)"; two groups killed him "readily"; forum t131 "It will take about five minutes for Parathior to
       spawn" (board 1149).

WHAT DIFFERED (quests 100880 / 100895 / 100938 "Parathior", the three class sets' last step of the 49 epic;
Parathior npcs 114825 by the broken tower in the north Snowblind Plains, world 0 (7035, 76, 4282)):
  - He stood by the tower for anyone, a level 52 roamer (hp_pct 100: 10,982 hit points, the roamer's crit, no
    AoE), respawning in 90 s. Now spawn-only (npcspawnonly "parathior"): only the ritual raises him.
  - Step 2 (talk to Orinas, beside Oransan) fires the new trigger "Parathior Descends" [AZ: Orinas; GF/Prima: the
    five minutes]: one at a time (its lock; Orinas answers "the light burns already" while another's lives), a
    window to those at the tower that the light kindles atop it, those near on step 2 of the same quest set on to
    step 3 (questset: a group fights one Parathior, the questor alone speaks to Orinas), five minutes' wait, then
    Parathior made on the mountainside straight north of the tower (7035, 195.5, 4130: the plateau's edge, 150
    above, the ground's floor walked down with no wall, checked on worlds/ground/0.*) and WALKED down to his old
    spot [GF "walking down the mountain"], hostile, for 35 minutes at most. The run's clock is 45 minutes, ended
    early by ten minutes with no one within 200 of the tower; his death ends it and frees the lock at once (his
    corpse stays to be looted). A run ended without his kill puts the questors back on step 2
    (handlers/questencounter.go), to speak to Orinas again.
  - His fight [GF "a bit harder than Chardith ... not that much harder"; Lord Chardith's own template: hp_pct 547,
    60,072 at L52, Chardith's Ruin 500 every 30 s from 10 s, roamer melee]: a template of his own "parathior",
    the roamer's columns with hp_pct 656 (about 72,000 at L52: Chardith's +20%), melee 100% and the roamer's
    Critical Hit kept, and Parathior's Wrath (new spell 760401, made from Chardith's Ruin): 1,400 flat on his
    target and all within 30 of it, no resist, every 30 s from 10 s [AZ "approx 1200", GF "1,600ish": the two
    era figures' middle; the rhythm and radius Chardith's]. Level 52 kept ([GF] of the 49 epic's mobs "all ...
    red to a level 49"; nothing sizes him otherwise).
  - The quests' notes no longer say he stands always.
  LIVE (a shard of its own tuning): Parathior there fights by template "epic parathior" (the owner's yes of
  2026-11-23, Live petitions 5556/5557, "very under tuned and can be solod": hp_pct 2732, about 300,000, melee 150%).
  That stays: on such a shard the tool adds Parathior's Wrath to that template's abilities and makes no template of
  its own (npctemplate untouched); everything else is as above.
  Left as they were: his loot (lootcreature arctic, the quest's Ice Chunk 700858 from step 3), his body, level,
  Orinas and Oransan standing (board 1056: a lost Orinas copy would strand step 2), npcrespawn 114825 (unused
  while he is spawn-only).

Report only by default; --apply in one transaction with gm_audit lines and the log parathior_1125_log (every row
before and after); a second --apply does nothing; --undo puts back exactly what it wrote (a row changed since is
left and said) and drops the log. Refused whole if a row is not as this tool expects. Rows are found by name and
place (the shards' ids differ: Chardith's Ruin is 908033 on Heirloom, 907423 on Live); the trigger takes the
shard's next slot.

WHEN IT TAKES EFFECT: quests, triggers, spells, templates and spawn-only names at a start: run it in the keep
restart's before.sh, the server stopped, on each shard's database.

    python3 -E -s tools/parathior_20261125.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "parathior_1125_log"
WHO, VIA = "dab", "tool"
SRC = "parathior1125"
DETAIL = ("Parathior raised by the L49 epics' ritual and tuned to the era's guides (Heirloom petition 2990, board "
          "1056): tools/parathior_20261125.py")

QUESTS = (100880, 100895, 100938)
FIRE_STEP, KILL_STEP = 2, 3
NAME = "Parathior"
KEY = "parathior"  # npcspawnonly, npctemplate, classtemplates
ROW = 114825
ROW_PLACE = (0, 7035.4, 76.2, 4281.6)  # world, x, y, z (rounded to 0.1)
LEVEL = 52

# the plateau's edge straight north of the tower (+z is south), on the ground's floor there
DESCENT_FROM = (7035.0, 195.5, 4130.0)
PACE = 4.0  # walked (under 7), an escort's pace
REACH = 200.0
WAIT_S = 300
CLOCK_S, EMPTY_S = 2700, 600
STAY_MIN = 35

TRIGGER = "Parathior Descends"
LOCK = "parathior"
BUSY = "The light burns atop the tower already: Parathior comes for others. Wait until they are done."
KINDLE = ("A ball of light kindles atop the broken tower. Make ready: in about five minutes Parathior will come "
          "down the mountain.")
COMES = "The light atop the broken tower goes out. Parathior comes walking down the mountain!"

CHARDITH_SPELL = "Chardith's Ruin"
CHARDITH_DMG = 500
WRATH = 760401
WRATH_NAME = "Parathior's Wrath"
WRATH_DMG, WRATH_RADIUS = 1400, 30
ABILITIES = [{"kind": "aoe", "spell_id": WRATH, "every": 30, "first": 10}]
HP_PCT = 656
# a shard whose Parathior was tuned already by the owner's word: its template kept, the AoE added
KEPT_TEMPLATE = "epic parathior"
KEPT_NOTE = (" | Petition 2990 (Heirloom, 2026-11-25), tools/parathior_20261125.py: Parathior's Wrath added, 1,400 flat on "
             "the target and within 30, every 30 s from 10 s (AZ 694 'approx 1200', MasterVG782's GameFAQs guide "
             "'1,600ish'); hit points and melee kept (the owner's yes, 2026-11-23)")
TEMPLATE_NOTE = ("Heirloom petition 2990 (Jibrish, 2026-11-25), board 1056: the roamer's columns plus Parathior's "
                 "Wrath, 1,400 flat on the target and within 30, no resist, every 30 s from 10 s (AZ 694 'approx "
                 "1200', MasterVG782's GameFAQs guide '1,600ish': their middle; Chardith's rhythm). Hit points "
                 "hp_pct 656, about 72,000 at L52: the guide's 'a bit harder than Chardith ... not that much "
                 "harder' (Lord Chardith hp_pct 547, 60,072), +20%. tools/parathior_20261125.py")

NOTE_OLD = "The ritual's five minutes are not in; Parathior stands by the tower (the guide's comes down from the sky); "
NOTE_NEW = ("Speaking to Orinas fires the trigger 'Parathior Descends': a light atop the tower, five minutes, then "
            "Parathior walks down the mountain (MasterVG782's GameFAQs guide; Prima p.370; AZ 694), one at a time; "
            "he is spawn-only (tools/parathior_20261125.py, petition 2990); ")


def has(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone() is not None


def row(db, tbl, where, args):
    cur = db.execute(f'SELECT * FROM "{tbl}" WHERE {where}', args)
    r = cur.fetchone()
    return dict(zip([d[0] for d in cur.description], r)) if r else None


def js(v):
    return json.dumps(v, separators=(",", ":")) if v is not None else None


def fire_events():
    return js([{"on": "done", "do": "trigger", "trigger": TRIGGER, "text": BUSY}])


def trigger_steps():
    world, x, y, z = ROW_PLACE
    fx, fy, fz = DESCENT_FROM
    steps = [{"do": "lock", "lock": LOCK},
             {"do": "clock", "seconds": CLOCK_S, "empty": EMPTY_S}]
    for q in QUESTS:
        steps.append({"do": "questset", "quest": q, "from": FIRE_STEP, "step": KILL_STEP})
    steps += [
        {"do": "popup", "text": KINDLE},
        {"do": "wait", "seconds": WAIT_S},
        {"do": "spawn", "npc": NAME, "hostile": True, "minutes": STAY_MIN, "spread": 1, "world": world,
         "x": fx, "y": fy, "z": fz, "face": "south"},
        {"do": "walk", "npc": NAME, "radius": REACH, "pace": PACE, "x": x, "y": y, "z": z},
        {"do": "broadcast", "text": COMES},
        {"do": "awaitdead", "npc": NAME},
        {"do": "end"},
        {"do": "despawn"},
    ]
    return steps


def chardith_spell(db):
    """The shard's Chardith's Ruin (500 flat, target AoE radius 30), or None."""
    for (sid,) in db.execute("SELECT spellid FROM spellpattern WHERE spellname = ? ORDER BY spellid", (CHARDITH_SPELL,)):
        e = row(db, "spelleffect", "spellid = ?", (sid,))
        if e is None:
            continue
        eff = json.loads(e["effects"])
        amounts = [x.get("amount") for x in eff.get("effects", [])]
        if amounts == [-CHARDITH_DMG] and eff.get("scope") == "target_aoe":
            return sid
    return None


def kept_template(db):
    """The shard's own tuning of Parathior to keep (Live's "epic parathior"), or None for the roamer."""
    nt = row(db, "npctemplate", "name = ?", (KEY,))
    return KEPT_TEMPLATE if nt and nt["template"] == KEPT_TEMPLATE else None


def check(db):
    """What is wrong with the database for this tool, [] for nothing."""
    bad = []
    for q in QUESTS:
        qu = row(db, "quests", "questid = ?", (q,))
        if qu is None or qu["name"] != NAME:
            bad.append(f"quests {q}: {qu and qu['name']!r}, want {NAME!r}")
        elif NOTE_OLD not in (qu["notes"] or ""):
            bad.append(f"quests {q}: its notes lack the importer's 'stands by the tower' line")
        s2 = row(db, "queststeps", "questid = ? AND step = ?", (q, FIRE_STEP))
        if s2 is None or (s2["kind"], s2["target"], s2["events"]) != ("talk", "Orinas", ""):
            bad.append(f"queststeps {q} step {FIRE_STEP}: {s2 and (s2['kind'], s2['target'], s2['events'])}, "
                       "want a talk to Orinas with no events")
        s3 = row(db, "queststeps", "questid = ? AND step = ?", (q, KILL_STEP))
        if s3 is None or (s3["kind"], s3["target"], s3["events"]) != ("loot", NAME, ""):
            bad.append(f"queststeps {q} step {KILL_STEP}: {s3 and (s3['kind'], s3['target'], s3['events'])}, "
                       f"want a loot of {NAME} with no events")
    others = db.execute("SELECT questid, step FROM queststeps WHERE (target = ? OR events LIKE ?) AND questid NOT IN "
                        f"({', '.join('?' * len(QUESTS))})", (NAME, f'%"{NAME}"%', *QUESTS)).fetchall()
    if others:
        bad.append(f"other steps name {NAME}: {others}")
    n = db.execute(f"SELECT count(*) FROM charquests WHERE done = 0 AND step = ? AND questid IN "
                   f"({', '.join('?' * len(QUESTS))})", (KILL_STEP, *QUESTS)).fetchone()[0]
    if n:
        bad.append(f"{n} characters stand on step {KILL_STEP} (they would be put back to step {FIRE_STEP})")
    r = row(db, "npcs", "id = ?", (ROW,))
    if r is None or r["npc_name"] != NAME or r["npc_level"] != LEVEL or r["world"] != ROW_PLACE[0] or \
            (round(r["x_coord"], 1), round(r["y_coord"], 1), round(r["z_coord"], 1)) != ROW_PLACE[1:]:
        bad.append(f"npcs {ROW}: {r and (r['npc_name'], r['npc_level'], r['world'], r['x_coord'], r['z_coord'])}, "
                   f"want {NAME} L{LEVEL} at {ROW_PLACE}")
    if db.execute("SELECT count(*) FROM npcs WHERE lower(trim(npc_name)) = ?", (KEY,)).fetchone()[0] != 1:
        bad.append(f"not exactly one npcs row named {NAME}")
    if row(db, "npcspawnonly", "name = ?", (KEY,)):
        bad.append(f"npcspawnonly {KEY} is there already")
    nt = row(db, "npctemplate", "name = ?", (KEY,))
    if nt is None or nt["template"] not in ("roamer", KEPT_TEMPLATE):
        bad.append(f"npctemplate {KEY}: {nt}, want the roamer or {KEPT_TEMPLATE!r}")
    elif nt["template"] == KEPT_TEMPLATE:
        kt = row(db, "classtemplates", "name = ?", (KEPT_TEMPLATE,))
        if kt is None or json.loads(kt["abilities"] or "[]") != []:
            bad.append(f"classtemplates {KEPT_TEMPLATE}: {kt and kt['abilities']}, want no abilities")
    if row(db, "classtemplates", "name = ?", (KEY,)):
        bad.append(f"classtemplates {KEY} is there already")
    if row(db, "classtemplates", "name = ?", ("roamer",)) is None:
        bad.append("no classtemplates roamer")
    if row(db, "spellpattern", "spellid = ?", (WRATH,)) or row(db, "spelleffect", "spellid = ?", (WRATH,)):
        bad.append(f"spell {WRATH} is taken")
    if db.execute("SELECT 1 FROM spellpattern WHERE spellname = ?", (WRATH_NAME,)).fetchone():
        bad.append(f"a spell {WRATH_NAME!r} is there already")
    if chardith_spell(db) is None:
        bad.append(f"no {CHARDITH_SPELL} of {CHARDITH_DMG} to make {WRATH_NAME} from")
    if row(db, "gmtriggers", "name = ?", (TRIGGER,)):
        bad.append(f"a trigger {TRIGGER!r} is there already")
    if db.execute("SELECT 1 FROM gmtriggers WHERE steps LIKE ?", (f'%"lock":"{LOCK}"%',)).fetchone():
        bad.append(f"a trigger holds the lock {LOCK!r} already")
    return bad


def report(db):
    r = row(db, "npcs", "id = ?", (ROW,))
    t = row(db, "npctemplate", "name = ?", (KEY,))
    ct = row(db, "classtemplates", "name = ?", (t["template"],)) if t else None
    print(f"  {NAME}: npcs {ROW} level {r and r['npc_level']}, spawn-only "
          f"{bool(row(db, 'npcspawnonly', 'name = ?', (KEY,)))}, template {t and t['template']!r} "
          f"(hp_pct {ct and ct['hp_pct']}, abilities {ct and ct['abilities']})")
    for q in QUESTS:
        for s in db.execute("SELECT step, kind, target, events FROM queststeps WHERE questid = ? AND step IN (?, ?) "
                            "ORDER BY step", (q, FIRE_STEP, KILL_STEP)):
            print(f"    quest {q} step {s[0]} {s[1]} {s[2]!r} events {s[3]!r}")
    g = row(db, "gmtriggers", "name = ?", (TRIGGER,))
    print(f"  trigger {TRIGGER!r}: {g and ('slot', g['slot'])}")


def next_slot(db):
    return (db.execute("SELECT max(slot) FROM gmtriggers").fetchone()[0] or 8) + 1


def apply(db):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    db.execute(f"CREATE TABLE {LOG} (seq INTEGER PRIMARY KEY, tbl TEXT NOT NULL, rowkey TEXT NOT NULL, before TEXT, after TEXT)")

    def done(tbl, key, before):
        where = " AND ".join(f"{c} = ?" for c in key)
        after = row(db, tbl, where, tuple(key.values()))
        action = "update" if before and after else "insert" if after else "delete"
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES (?, ?, ?, ?)", (tbl, js(key), js(before), js(after)))
        db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                   "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                   (at, acct[0] if acct else 0, WHO, VIA, action, tbl, js(key), js(before), js(after), DETAIL))

    def insert(tbl, values, key):
        db.execute(f'INSERT INTO "{tbl}" ({", ".join(values)}) VALUES ({", ".join("?" * len(values))})', list(values.values()))
        done(tbl, key, None)

    def update(tbl, key, values):
        where = " AND ".join(f"{c} = ?" for c in key)
        before = row(db, tbl, where, tuple(key.values()))
        db.execute(f'UPDATE "{tbl}" SET {", ".join(f"{c} = ?" for c in values)} WHERE {where}',
                   list(values.values()) + list(key.values()))
        done(tbl, key, before)

    # his AoE, made from the shard's Chardith's Ruin
    base = chardith_spell(db)
    line = f"Target, aoe Decrease Hitpoints by {WRATH_DMG}, Radius {WRATH_RADIUS}"
    p = row(db, "spellpattern", "spellid = ?", (base,))
    insert("spellpattern", dict(p, spellid=WRATH, spellname=WRATH_NAME, spelldesc=line), {"spellid": WRATH})
    e = row(db, "spelleffect", "spellid = ?", (base,))
    eff = json.loads(e["effects"])
    eff["radius"] = WRATH_RADIUS
    eff["effects"] = [{"type": "Dmg/Heal", "kind": "hp2", "amount": -WRATH_DMG}]
    insert("spelleffect", {"spellid": WRATH, "line": line, "fxdict": e["fxdict"],
                           "source": f"Parathior's AoE, petition 2990 / board 1056: {SRC}", "effects": js(eff)},
           {"spellid": WRATH})

    # his template: the roamer's columns, his hit points and AoE; or the shard's own tuning kept, the AoE added
    kept = kept_template(db)
    if kept:
        kt = row(db, "classtemplates", "name = ?", (kept,))
        update("classtemplates", {"name": kept}, {"abilities": js(ABILITIES), "note": kt["note"] + KEPT_NOTE})
    else:
        roamer = row(db, "classtemplates", "name = ?", ("roamer",))
        insert("classtemplates", dict(roamer, name=KEY, hp_pct=HP_PCT, abilities=js(ABILITIES), note=TEMPLATE_NOTE,
                                      source=SRC), {"name": KEY})
        update("npctemplate", {"name": KEY}, {"template": KEY, "note": "named, level 52; own template (petition 2990, "
                                              "tools/parathior_20261125.py)", "source": SRC})

    # spawn-only: the ritual alone raises him
    insert("npcspawnonly", {"name": KEY, "note": "raised by the L49 epics' ritual, quests 100880/100895/100938 step 2 "
                            "(talk to Orinas) firing trigger 'Parathior Descends' (petition 2990): "
                            "tools/parathior_20261125.py", "source": SRC}, {"name": KEY})

    # the trigger and the quests that fire it
    world, x, y, z = ROW_PLACE
    slot = next_slot(db)
    insert("gmtriggers", {"slot": slot, "name": TRIGGER, "enabled": 1, "scope": "near", "radius": REACH, "world": world,
                          "x": x, "y": y, "z": z, "steps": js(trigger_steps()),
                          "notes": "Fired by the L49 epic's 'Parathior' quests (100880/100895/100938) step 2, the talk "
                                   "to Orinas: a light atop the broken tower, those near on step 2 set to step 3, five "
                                   "minutes, Parathior made on the mountainside north of the tower and walked down to it "
                                   "(MasterVG782's GameFAQs guide; Prima p.370; AZ 694), one at a time, 45 minutes at "
                                   "most, ten with no one near end it. Petition 2990: tools/parathior_20261125.py"},
           {"slot": slot})
    for q in QUESTS:
        update("queststeps", {"questid": q, "step": FIRE_STEP}, {"events": fire_events()})
        qu = row(db, "quests", "questid = ?", (q,))
        update("quests", {"questid": q}, {"notes": qu["notes"].replace(NOTE_OLD, NOTE_NEW)})


def undo(db):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    n = 0
    for tbl, rowkey, before, after in db.execute(f"SELECT tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key = json.loads(rowkey)
        where = " AND ".join(f"{c} = ?" for c in key)
        now = row(db, tbl, where, tuple(key.values()))
        if js(now) != after:
            print(f"  {tbl} {rowkey}: changed since this tool wrote it, left as it is")
            continue
        if before is None:
            db.execute(f'DELETE FROM "{tbl}" WHERE {where}', tuple(key.values()))
        elif after is None:
            b = json.loads(before)
            db.execute(f'INSERT INTO "{tbl}" ({", ".join(b)}) VALUES ({", ".join("?" * len(b))})', list(b.values()))
        else:
            b = json.loads(before)
            db.execute(f'UPDATE "{tbl}" SET {", ".join(f"{c} = ?" for c in b)} WHERE {where}',
                       list(b.values()) + list(key.values()))
        n += 1
    db.execute(f"DROP TABLE {LOG}")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', ?, '{}', NULL, NULL, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, LOG, f"undone, {n} rows put back: " + DETAIL))
    print(f"  {n} rows put back")


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        report(db)
        if a.undo:
            if not has(db, LOG):
                print("nothing to undo")
                return 0
            db.execute("BEGIN IMMEDIATE")
            undo(db)
            db.execute("COMMIT")
            print("undone; takes effect at a restart")
            return 0
        if has(db, LOG):
            print(f"applied already ({LOG} is there): nothing to do (--undo to take it back)")
            return 0
        bad = check(db)
        if bad:
            print("refused, nothing done:\n  " + "\n  ".join(bad))
            return 1
        if not a.apply:
            print("report only: --apply to write it")
            return 0
        db.execute("BEGIN IMMEDIATE")
        apply(db)
        db.execute("COMMIT")
        print("applied; takes effect at a restart (quests, triggers, spells and templates are read at a start)")
        report(db)
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
