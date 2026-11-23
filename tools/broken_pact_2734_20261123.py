#!/usr/bin/env python3
"""The Broken Pact's goblin messenger comes when the quester reaches the crossroads, not standing there for anyone
(Heirloom petition 2734, 2026-11-23: "Goblin Messenger shouldn't appear until you receive a pop-up at my location;
you shouldn't be able to see him from afar"; the same ask as Live petition 5271).

Quest 100940 "The Broken Pact" (Count Luvincius's group-6 evil level 30 path: Talknar outside Dark Solace sends you
NE over the hill to the crossroads). The guides (QuestMasters id158, EQOA-Data Quests/30,40,49/group-6-evil.json):
"This goblin spawns (triggered) without warning near a tree where all the roads meet". Here the messenger was an
ordinary npcs row standing at the crossroads (13114.4, 54.5, 20592.9, world 0) for anyone to kill, and step 2 a loot
step on him from the start.

What this writes (the server's own quest-spawn machinery, handlers/questevents.go + entity/spawnonly.go), as quest
100770 step 1 and 100775 step 9 do:
  - npcspawnonly 'Goblin Messenger': the standing row stays loaded (its place is where the quest marks point and
    where respawnLost makes a lost spawn again) but is in no one's world;
  - the quest's steps 2-3 become 3-4 and a new step 2 is a reach step at the crossroads where the player stood
    (13114.1, 54.6, 20585.7; radius 30), whose done events are a popup and a hostile spawn of the messenger for
    10 minutes (a done's spawn is the next step's: the loot step 3). A spawn lost (its minutes, the player leaving,
    a corpse gone unlooted) is made again when the player comes within 21 of where the messenger stands
    (respawnLost);
  - characters on the quest at step 2 or later (not done) move one step on, so no one's place in the quest changes
    meaning. Run --apply in a restart's hold (before.sh) or just before a cold start: quests and npcs load at the
    start, and the running server holds each character's step in memory and writes it on progress.

The messenger's row is found by name and place (world 0, within 50 of the crossroads), so each shard's own id is
used; the quest is checked to be as written (talk Talknar, loot Goblin Messenger, give Talknar), else REFUSED and
nothing written. Report by default; --apply in one transaction, a gm_audit line a row, logged in
broken_pact_2734_log; a second --apply finds nothing to do; --undo puts back exactly what was there (the steps, the
npcspawnonly row, each logged character's step if it is still the moved one; a character moved on since is left and
listed).

    python3 -E -s tools/broken_pact_2734_20261123.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import math
import sqlite3
import sys

QUEST = 100940
NAME = "Goblin Messenger"
SPOT = (0, 13114.4, 54.5, 20592.9)
LOG = "broken_pact_2734_log"
DETAIL = ("The Broken Pact's goblin messenger is a triggered spawn at the crossroads (Heirloom petition 2734, Live "
          "petition 5271; QuestMasters id158, EQOA-Data group-6-evil): tools/broken_pact_2734_20261123.py")
WHO, VIA = "claude", "tool"
REACH = {
    "kind": "reach", "target": "", "world": 0, "x": 13114.1, "y": 54.6, "z": 20585.7, "radius": 30.0,
    "journal": "Go to the crossroads north-east of Talknar, over the hill, by the tree where all roads meet.",
    "events": json.dumps([
        {"on": "done", "do": "popup", "text": "A goblin messenger comes hurrying down the road to the crossroads!"},
        {"on": "done", "do": "spawn", "npc": NAME, "hostile": True, "minutes": 10},
    ], separators=(",", ":")),
}


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def rows(db, sql, *a):
    cur = db.execute(sql, a)
    cols = [d[0] for d in cur.description]
    return [dict(zip(cols, r)) for r in cur.fetchall()]


def audit(db, action, tbl, key, before, after):
    db.execute("INSERT INTO gm_audit (at,accountid,username,via,ip,action,tbl,rowkey,before,after,detail) "
               "VALUES (?,1,?,?,'',?,?,?,?,?,?)",
               (now(), WHO, VIA, action, tbl, json.dumps(key, sort_keys=True),
                None if before is None else json.dumps(before, sort_keys=True, default=str),
                None if after is None else json.dumps(after, sort_keys=True, default=str), DETAIL))
    return db.execute("SELECT last_insert_rowid()").fetchone()[0]


def log(db, tbl, key, before, after, aid):
    db.execute(f"INSERT INTO {LOG} (tbl,rowkey,before,after,audit) VALUES (?,?,?,?,?)",
               (tbl, json.dumps(key, sort_keys=True), None if before is None else json.dumps(before, sort_keys=True),
                None if after is None else json.dumps(after, sort_keys=True), aid))


def has_table(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type='table' AND name=?", (t,)).fetchone() is not None


def messenger(db):
    w, x, _, z = SPOT
    found = [r for r in rows(db, "SELECT id,npc_name,world,x_coord,z_coord FROM npcs WHERE npc_name=?", NAME)
             if r["world"] == w and math.hypot(r["x_coord"] - x, r["z_coord"] - z) <= 50]
    return found


def plan(db):
    problems = []
    steps = rows(db, "SELECT * FROM queststeps WHERE questid=? ORDER BY step", QUEST)
    shape = [(s["step"], s["kind"], s["target"]) for s in steps]
    applied = shape == [(1, "talk", "Talknar"), (2, "reach", ""), (3, "loot", NAME), (4, "give", "Talknar")]
    if not applied and shape != [(1, "talk", "Talknar"), (2, "loot", NAME), (3, "give", "Talknar")]:
        problems.append(f"quest {QUEST}'s steps are not as written: {shape}")
    m = messenger(db)
    if len(m) != 1:
        problems.append(f"{len(m)} '{NAME}' rows at the crossroads (want 1): {[r['id'] for r in m]}")
    others = [r["id"] for r in rows(db, "SELECT id FROM npcs WHERE lower(npc_name)=lower(?)", NAME)]
    so = db.execute("SELECT name FROM npcspawnonly WHERE lower(name)=lower(?)", (NAME,)).fetchone()
    moving = rows(db, "SELECT serverid,step,done FROM charquests WHERE questid=? AND done=0 AND step>=2", QUEST)
    return dict(problems=problems, steps=steps, applied=applied, row=m[0]["id"] if len(m) == 1 else None,
                all_rows=others, spawnonly=so is not None, moving=moving)


def report(p):
    print(f"quest {QUEST}: {len(p['steps'])} steps, {'already applied' if p['applied'] else 'as written'}")
    print(f"'{NAME}' row at the crossroads: {p['row']} (all rows of the name: {p['all_rows']}); "
          f"npcspawnonly row: {'yes' if p['spawnonly'] else 'no'}")
    print(f"characters on the quest at step >= 2, not done: {[(r['serverid'], r['step']) for r in p['moving']]}")
    for x in p["problems"]:
        print("REFUSED:", x)


def apply(db):
    p = plan(db)
    report(p)
    if p["problems"]:
        return 1
    if p["applied"]:
        print("nothing to do: already applied")
        return 0
    db.execute("BEGIN IMMEDIATE")
    if not has_table(db, LOG):
        db.execute(f"CREATE TABLE {LOG} (seq INTEGER PRIMARY KEY AUTOINCREMENT, tbl TEXT NOT NULL, rowkey TEXT NOT NULL, "
                   "before TEXT, after TEXT, audit INTEGER NOT NULL)")
    # characters first (their steps as the old numbering reads them)
    for r in p["moving"]:
        key = {"serverid": r["serverid"], "questid": QUEST}
        aid = audit(db, "update", "charquests", key, {"step": r["step"]}, {"step": r["step"] + 1})
        log(db, "charquests", key, {"step": r["step"]}, {"step": r["step"] + 1}, aid)
        db.execute("UPDATE charquests SET step=step+1 WHERE serverid=? AND questid=? AND done=0", (r["serverid"], QUEST))
    # steps 2..n -> 3..n+1, highest first (the key is (questid, step))
    for s in sorted(p["steps"], key=lambda s: -s["step"]):
        if s["step"] < 2:
            continue
        key = {"questid": QUEST, "step": s["step"]}
        aid = audit(db, "update", "queststeps", key, {"step": s["step"]}, {"step": s["step"] + 1})
        log(db, "queststeps", key, s, {"step": s["step"] + 1}, aid)
        db.execute("UPDATE queststeps SET step=? WHERE questid=? AND step=?", (s["step"] + 1, QUEST, s["step"]))
    new = dict(REACH, questid=QUEST, step=2)
    cols = ", ".join(new)
    db.execute(f"INSERT INTO queststeps ({cols}) VALUES ({', '.join('?' * len(new))})", tuple(new.values()))
    aid = audit(db, "insert", "queststeps", {"questid": QUEST, "step": 2}, None, new)
    log(db, "queststeps", {"questid": QUEST, "step": 2}, None, new, aid)
    if not p["spawnonly"]:
        so = {"name": NAME, "note": f"the spawn of quest {QUEST} 'The Broken Pact' step 3, made at the crossroads reach "
                                    f"(petition 2734); it stood there as row {p['row']}", "source": "petition 2734"}
        db.execute("INSERT INTO npcspawnonly (name,note,source) VALUES (?,?,?)", (so["name"], so["note"], so["source"]))
        aid = audit(db, "insert", "npcspawnonly", {"name": NAME}, None, so)
        log(db, "npcspawnonly", {"name": NAME}, None, so, aid)
    db.execute("COMMIT")
    print("applied: quest steps renumbered + reach step 2, npcspawnonly, "
          f"{len(p['moving'])} character(s) moved one step on; in force at the next restart")
    return 0


def undo(db):
    if not has_table(db, LOG):
        print("nothing to undo: no log")
        return 0
    entries = rows(db, f"SELECT * FROM {LOG} ORDER BY seq DESC")
    if not entries:
        print("nothing to undo")
        return 0
    db.execute("BEGIN IMMEDIATE")
    left = []
    for e in entries:
        key, before, after = json.loads(e["rowkey"]), e["before"] and json.loads(e["before"]), e["after"] and json.loads(e["after"])
        if e["tbl"] == "npcspawnonly":
            db.execute("DELETE FROM npcspawnonly WHERE name=? AND source='petition 2734'", (key["name"],))
            audit(db, "undo", "npcspawnonly", key, after, None)
        elif e["tbl"] == "queststeps" and before is None:
            db.execute("DELETE FROM queststeps WHERE questid=? AND step=2 AND kind='reach'", (QUEST,))
            audit(db, "undo", "queststeps", key, after, None)
        elif e["tbl"] == "queststeps":
            db.execute("UPDATE queststeps SET step=? WHERE questid=? AND step=?", (before["step"], QUEST, after["step"]))
            audit(db, "undo", "queststeps", key, after, {"step": before["step"]})
        elif e["tbl"] == "charquests":
            cur = db.execute("UPDATE charquests SET step=? WHERE serverid=? AND questid=? AND done=0 AND step=?",
                             (before["step"], key["serverid"], QUEST, after["step"]))
            if cur.rowcount:
                audit(db, "undo", "charquests", key, after, before)
            else:
                left.append(key["serverid"])
    db.execute(f"DROP TABLE {LOG}")
    db.execute("COMMIT")
    print("undone" + (f"; characters moved on since, left as they are: {left}" if left else ""))
    return 0


def main(argv=None):
    ap = argparse.ArgumentParser()
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=20)
    if a.apply:
        return apply(db)
    if a.undo:
        return undo(db)
    report(plan(db))
    return 0


if __name__ == "__main__":
    sys.exit(main())
