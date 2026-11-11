#!/usr/bin/env python3
"""The Froglok Herbalist's protection event (petition 1617, Lickinfrogs, barbarian warrior 60: "60 good war epic, ep1,
collected items for froglok herbalist, turned in and no mobs spawned"). EQOA-Data's Good Tank, Episode 1, and
questmasters1.com's t238: "Talking to the Herbalist with your ingredients will initiate an event in which you must
protect the herbalist. Every five minutes, 2 mobs will spawn - this will occur for 3 rounds ... Two Watchmen, Two
Guardsmen, A Lookout and a Scout ... Watchmen: dark blue, Guardsmen: white, Lookout/Scout: yellow/red ... 4th Round:
Bouncers will spawn - they con red, and are very difficult ... If the herbman dies while you are making potions, you
will have to wait until someone else triggers the event. Since the popups have an Area of Effect, your quest will
still be advanced if you have the necessary items in your inventory."

One guide-gaps set, `guk-herbalist` (tools/guidegaps/guk-herbalist.json, made by its notes' figures), written by
tools/seed_guide_gaps.py's machinery, every row it changes logged in guide_set_log so the undo is exact:

  - trigger slot 161, The Froglok Herbalist (world 0 at the herbalist, near 80): lock "guk herbalist"; protect the
    herbalist (his death fails the run: a window, the spawns taken away, nothing taken from anyone); a 90 minute clock
    (10 empty minutes end it); three rounds of three waves 5 minutes apart from a spot on his floor 26 off (two
    froglok watchmen L53, two froglok guardsmen L60, a froglok lookout L61 and a froglok scout L63, each set on the
    herbalist), then two froglok bouncers L64 (hostile, template "guk bouncer"); when all are dead, quest 101035's and
    101097's step 4 done for every player near on it who holds the three ingredients (queststep): taken, and the
    potion (701141) given by the step's own done event;
  - quests 101035 and 101097 (the herbalist's two epic lines) step 4: its trigger event HELD (talking to him with the
    ingredients fires the event and takes nothing; a talk while it runs says so), its words and journal; step 5 and
    everyone already past step 4 (Lickinfrogs, potion in hand) untouched;
  - five spawn-only rows (looks copied off Guk's froglok guk warrior, zol knight, yun warrior, assassin, executioner;
    the bouncer at 1.3), held out of the world by row key ("#id": the world's froglok scouts stay where they are);
    npctemplate roamer for the new names as Guk's frogloks have; classtemplate "guk bouncer" (ours);
  - the herbalist (row 107883) given the NPC bit (no longer red to a player nor hit by a player's area spell) and
    classtemplate "guk herbalist" (8x a L40's hit points, 40% of a hit taken: ours; on the rig two guardsmen killed
    the plain L40 in 18 s), so a wave left alone takes minutes to kill him, not seconds.

Refused, nothing written, unless trigger slot 161 is free, the herbalist row and both steps are as imported, the copied
rows and items are there and none of the new names is taken. Report only by default (the set run and rolled back);
--apply in one transaction with a gm_audit line; a second --apply refuses (the set is in); --undo exact, with its own
line. The code (handlers/questhold.go: hold, protect, queststep) and this go together: an older server cannot run the
trigger, and quests, NPCs, templates and triggers are read at the server's start, so a restart after both.

    tools/guk_herbalist_20261111.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import seed_guide_gaps as sgg  # noqa: E402

SET = "guk-herbalist"
WHO, VIA = "dab", "tool"
DETAIL = "The Froglok Herbalist's protection event (petition 1617, Lickinfrogs): tools/guk_herbalist_20261111.py"
SLOT = 161
HERBALIST = 107883
NEW_NAMES = ("froglok watchman", "froglok guardsman", "froglok lookout", "froglok bouncer")


def spec():
    s = json.load(open(os.path.join(HERE, "guidegaps", SET + ".json")))
    s["set"] = SET
    return s


def audit(db, action, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if sgg.has(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', ?, 'guide_set_log', ?, NULL, ?, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, action, json.dumps({"set": SET}), json.dumps(after, sort_keys=True), DETAIL))


def refusals(db, s):
    out = []
    if sgg.has(db, sgg.LOG) and db.execute(f"SELECT 1 FROM {sgg.LOG} WHERE setname = ?", (SET,)).fetchone():
        return [f"set {SET} is in already"]
    r = db.execute("SELECT name FROM gmtriggers WHERE slot = ?", (SLOT,)).fetchone()
    if r:
        out.append(f"trigger slot {SLOT} is taken: {r[0]!r}")
    if db.execute("SELECT 1 FROM gmtriggers WHERE lower(name) = 'the froglok herbalist'").fetchone():
        out.append("a trigger is named The Froglok Herbalist already")
    r = db.execute("SELECT npc_name, world, npc_type FROM npcs WHERE id = ?", (HERBALIST,)).fetchone()
    if r != ("froglok herbalist", 0, 0):
        out.append(f"npcs row {HERBALIST} is {r}, not the froglok herbalist of world 0 with npc_type 0")
    for stmt, params in s["sql"]:
        if stmt.startswith("UPDATE queststeps"):
            q, old_ev, old_says = params[3], params[4], params[5]
            r = db.execute("SELECT target, events, says FROM queststeps WHERE questid = ? AND step = 4", (q,)).fetchone()
            if r != ("froglok herbalist", old_ev, old_says):
                out.append(f"quest {q} step 4 is not as imported: {r}")
    for n in s["npcs"]:
        if not db.execute("SELECT 1 FROM npcs WHERE lower(trim(npc_name)) = ?", (n["like"],)).fetchone():
            out.append(f"no {n['like']!r} to copy")
    for name in NEW_NAMES:
        for t, c in (("npcs", "npc_name"), ("npctemplate", "name"), ("npcspawnonly", "name"), ("npcaggro", "npc_name")):
            if sgg.has(db, t) and db.execute(f"SELECT 1 FROM {t} WHERE lower(trim({c})) = ?", (name,)).fetchone():
                out.append(f"{t} names {name!r} already")
    for t in ("guk bouncer", "guk herbalist"):
        if db.execute("SELECT 1 FROM classtemplates WHERE name = ?", (t,)).fetchone():
            out.append(f"classtemplate {t!r} is there already")
    r = db.execute("SELECT template FROM npctemplate WHERE name = 'froglok herbalist'").fetchone()
    if r != ("roamer",):
        out.append(f"npctemplate 'froglok herbalist' is {r}, not roamer")
    if not db.execute("SELECT 1 FROM classtemplates WHERE name = 'roamer'").fetchone():
        out.append("no classtemplate 'roamer'")
    for pid in (701138, 701139, 701140, 701141):
        if not db.execute("SELECT 1 FROM itempattern WHERE patternid = ?", (pid,)).fetchone():
            out.append(f"no item {pid}")
    return out


def check(db):
    """After the set's rows: every placeholder replaced, the five rows held out of the world."""
    steps = db.execute("SELECT steps FROM gmtriggers WHERE slot = ?", (SLOT,)).fetchone()[0]
    for tok in ("#WATCHMAN", "#GUARDSMAN", "#LOOKOUT", "#SCOUT", "#BOUNCER"):
        if tok in steps:
            raise SystemExit(f"trigger {SLOT} still names {tok}")
    n = db.execute("SELECT count(*) FROM npcspawnonly WHERE source = 'guide gaps guk-herbalist'").fetchone()[0]
    if n != 5:
        raise SystemExit(f"{n} rows held out of the world, want 5")
    for q in (101035, 101097):
        ev = json.loads(db.execute("SELECT events FROM queststeps WHERE questid = ? AND step = 4", (q,)).fetchone()[0])
        if not ev or not ev[0].get("hold"):
            raise SystemExit(f"quest {q} step 4 holds no trigger")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, timeout=5, isolation_level=None)
    say = print
    if a.undo:
        sgg.undo(db, SET, say, extra=lambda d, n: audit(d, "undo", {"rows": n}))
        return
    s = spec()
    bad = refusals(db, s)
    if bad:
        for b in bad:
            print("refused:", b)
        sys.exit(1)

    def extra(d, counts):
        check(d)
        audit(d, "apply", counts)
    sgg.apply(db, s, a.apply, say, extra=extra)


if __name__ == "__main__":
    main()
