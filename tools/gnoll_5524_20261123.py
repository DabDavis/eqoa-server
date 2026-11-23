#!/usr/bin/env python3
"""The blue-pawed gnoll comes when the quester reaches its tunnel in Blackburrow, not standing there for anyone (Live
petition 5524, 2026-11-23: "blue-pawed gnoll should be a trigger spawn"; the owner: "yes do the gnoll like phorriun").

Quest 100780 "Rin's Clockworks" (Rin Tintoggle in Highpass, level 35, Quest 6 of EQOA-Data Quests/23-47/35.json:
"Kill Blue-pawed Gnoll in Blackburrow, room above dwarf temple, bottom left-hand tunnels near Black/Redpaw spawns,
left-hand tunnel popup"). Here the gnoll was an ordinary npcs row standing in that tunnel (12949.2, -24.0, 12537.0,
world 0) for anyone to kill, and step 5 a loot step on it.

What this writes, as tools/phorriun_5518_20261123.py does for Phorriun:
  - npcspawnonly 'blue pawed gnoll': the standing row stays loaded (its place is where the quest marks point and
    where respawnLost makes a lost spawn again) but is in no one's world;
  - the quest's steps 5-9 become 6-10 and a new step 5 is a reach step at the gnoll's place (radius 30), whose done
    events are a popup and a hostile spawn of the gnoll for 10 minutes (a done's spawn is the next step's: the loot
    step 6);
  - characters on the quest past step 5 (not done) move one step on, so no one's place in the quest changes meaning;
    one on step 5 stays on it, now the walk into the tunnel, whose popup brings the gnoll.
Yjuri the Vicious, who stands beside it, is another quest's (100775 step 6) and is left as it is.

RUN --apply IN A RESTART'S HOLD (before.sh) or just before a cold start: quests and npcs load at the start, and the
running server holds each character's step in memory and writes it on progress, so a character reaching step 5 or
past it between the apply and the restart would be read under the other numbering.

The gnoll's row is found by name and place (world 0, within 50 of the tunnel), so each shard's own id is used; the
quest is checked to be as written (its nine steps' kinds and targets), else REFUSED and nothing written. Report by
default; --apply in one transaction, a gm_audit line a row, logged in gnoll_5524_log; a second --apply finds nothing
to do; --undo puts back exactly what was there (a character moved on since is left and listed).

    python3 -E -s tools/gnoll_5524_20261123.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import math
import sqlite3
import sys

QUEST = 100780
NAME = "blue pawed gnoll"
SPOT = (0, 12949.2, -24.0, 12537.0)
AT = 5  # the new reach step's number
LOG = "gnoll_5524_log"
AS_READ = [(1, "loot", "Model LXI"), (2, "give", "Rin Tintoggle"), (3, "give", "Merchant Samwe"),
           (4, "give", "Rin Tintoggle"), (5, "loot", NAME), (6, "give", "Rin Tintoggle"), (7, "reach", ""),
           (8, "loot", "Model LXIX"), (9, "give", "Rin Tintoggle")]
APPLIED = AS_READ[:AT - 1] + [(AT, "reach", "")] + [(s + 1, k, t) for s, k, t in AS_READ[AT - 1:]]
DETAIL = ("Rin's Clockworks's blue-pawed gnoll is a triggered spawn in Blackburrow's bottom left-hand tunnel (Live "
          "petition 5524; EQOA-Data Quests/23-47/35.json): tools/gnoll_5524_20261123.py")
WHO, VIA = "claude", "tool"
REACH = {
    "kind": "reach", "target": "", "world": 0, "x": 12949.2, "y": -24.0, "z": 12537.0, "radius": 30.0,
    "journal": "Go down Blackburrow's bottom left-hand tunnels, past the redpaws, until the blue-pawed gnoll shows itself.",
    "events": json.dumps([
        {"on": "done", "do": "popup", "text": "Something stirs in the dark of the tunnel. The blue-pawed gnoll leaps out!"},
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
    applied = shape == APPLIED
    if not applied and shape != AS_READ:
        problems.append(f"quest {QUEST}'s steps are not as written: {shape}")
    m = messenger(db)
    if len(m) != 1:
        problems.append(f"{len(m)} '{NAME}' rows in the tunnel (want 1): {[r['id'] for r in m]}")
    others = [r["id"] for r in rows(db, "SELECT id FROM npcs WHERE lower(npc_name)=lower(?)", NAME)]
    so = db.execute("SELECT name FROM npcspawnonly WHERE lower(name)=lower(?)", (NAME,)).fetchone()
    moving = rows(db, "SELECT serverid,step,done FROM charquests WHERE questid=? AND done=0 AND step>?", QUEST, AT)
    return dict(problems=problems, steps=steps, applied=applied, row=m[0]["id"] if len(m) == 1 else None,
                all_rows=others, spawnonly=so is not None, moving=moving)


def report(p):
    print(f"quest {QUEST}: {len(p['steps'])} steps, {'already applied' if p['applied'] else 'as written'}")
    print(f"'{NAME}' row in the tunnel: {p['row']} (all rows of the name: {p['all_rows']}); "
          f"npcspawnonly row: {'yes' if p['spawnonly'] else 'no'}")
    print(f"characters on the quest past step {AT}, not done: {[(r['serverid'], r['step']) for r in p['moving']]}")
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
    # steps AT..n -> AT+1..n+1, highest first (the key is (questid, step))
    for s in sorted(p["steps"], key=lambda s: -s["step"]):
        if s["step"] < AT:
            continue
        key = {"questid": QUEST, "step": s["step"]}
        aid = audit(db, "update", "queststeps", key, {"step": s["step"]}, {"step": s["step"] + 1})
        log(db, "queststeps", key, s, {"step": s["step"] + 1}, aid)
        db.execute("UPDATE queststeps SET step=? WHERE questid=? AND step=?", (s["step"] + 1, QUEST, s["step"]))
    new = dict(REACH, questid=QUEST, step=AT)
    cols = ", ".join(new)
    db.execute(f"INSERT INTO queststeps ({cols}) VALUES ({', '.join('?' * len(new))})", tuple(new.values()))
    aid = audit(db, "insert", "queststeps", {"questid": QUEST, "step": AT}, None, new)
    log(db, "queststeps", {"questid": QUEST, "step": AT}, None, new, aid)
    if not p["spawnonly"]:
        so = {"name": NAME, "note": f"the spawn of quest {QUEST} 'Rin\'s Clockworks' step {AT + 1}, made at the tunnel's reach "
                                    f"(petition 5524); it stood there as row {p['row']}", "source": "petition 5524"}
        db.execute("INSERT INTO npcspawnonly (name,note,source) VALUES (?,?,?)", (so["name"], so["note"], so["source"]))
        aid = audit(db, "insert", "npcspawnonly", {"name": NAME}, None, so)
        log(db, "npcspawnonly", {"name": NAME}, None, so, aid)
    db.execute("COMMIT")
    print(f"applied: quest steps renumbered + reach step {AT}, npcspawnonly, "
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
            db.execute("DELETE FROM npcspawnonly WHERE name=? AND source='petition 5524'", (key["name"],))
            audit(db, "undo", "npcspawnonly", key, after, None)
        elif e["tbl"] == "queststeps" and before is None:
            db.execute("DELETE FROM queststeps WHERE questid=? AND step=? AND kind='reach'", (QUEST, AT))
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
