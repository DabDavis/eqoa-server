#!/usr/bin/env python3
"""The rat were quest's two fights as its guide has them (Heirloom petition 2487, 2026-11-20: "When I went to Highpass to
speak with the shady trafficker there were no guards that spawned for me to fight. And cogswhisker didnt ask me to bribe
him or fight him and his two henchmen. I just had to kill him").

Debts and Favors (101361) was imported from EQOA-Data's Quests/Weres/ratwere.json with both left out (its notes: "the
engine has one path"). The guide: at Highpass "Two HP guards will spawn ... kill the two triggered HP guards ... THEN talk
to the trafficker again", and Cogwhisker "You have two options: You can either pay him off with tunar or fight him ...
Rumour has it, if you kill him, you get 200 faction with Topi di Ombra ... his henchmen will con dark blue". A choice that
costs tunar and passes over a step is the engine's since this petition (database.StepOption Cost, Skip).

The steps, old -> new:
  1 talk the shady trafficker (new): two Highpass guards come at the player
  2 kill Guard Wexler and Guard Wode (new): hostile copies of the two, spawned at the step's start (a group's one set)
  3 give the purse (old 1)
  4-9 (old 2-7)
  10 talk Cogwhisker (new): pay 1,000 tunar (ours; the guide names no sum) and pass over the fight, or fight
  11 kill Cogwhisker (old 8): he turns on the player, two bandit henchmen with him; +200 Topi Di Ombra
  12 the Swashbuckler (old 9)
Cogwhisker attacks no one on sight (npcaggro), so he can be talked to. A character on the quest is moved with its step
(old 1 -> 1, 2-7 -> +2, 8 -> 10, 9 -> 12).

Both shards (the quest is the same on each). Refused, writing nothing, unless the quest's steps are the import's nine as
read here and nothing else keys its steps. Report only by default; --apply in one transaction, a gm_audit line a row,
logged in rat_were_2487_log; a second --apply finds it done; --undo exact (refused when a row changed since).

WHEN IT TAKES EFFECT: quests and npcaggro are read at the server's start, and the choice's cost needs that server's
binary: run it as the keep restart's before.sh that brings the code.

    tools/rat_were_2487_20261120.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "rat_were_2487_log"
DETAIL = "petition 2487: the rat were quest's Highpass ambush and Cogwhisker's bribe or fight: tools/rat_were_2487_20261120.py"
WHO, VIA = "claude", "tool"
QUEST, QUEST_NAME = 101361, "Debts and Favors"
COST = 1000

# the import's nine, (kind, target), as both shards hold them on 2026-11-20
OLD_STEPS = [("give", "shady trafficker"), ("talk", "Shady Swashbuckler"), ("talk", "Rathei Slaerin"), ("talk", "Yindrius"),
             ("talk", "Shady Swashbuckler"), ("talk", "Rolby Rumblebelly"), ("talk", "Shady Swashbuckler"),
             ("kill", "Cogwhisker"), ("talk", "Shady Swashbuckler")]
MOVE = {1: 1, 2: 4, 3: 5, 4: 6, 5: 7, 6: 8, 7: 9, 8: 10, 9: 12}  # a character's old step -> its new one
FROM_OLD = {3: 1, 4: 2, 5: 3, 6: 4, 7: 5, 8: 6, 9: 7, 11: 8, 12: 9}  # a new step that is an old one

GUARDS = ("Guard Wexler", "Guard Wode")
NEW_STEPS = {
    1: dict(kind="talk", target="shady trafficker",
            journal="Take the Swashbuckler's coin purse to the shady trafficker in Highpass castle.",
            says="You were followed! Those are Highpass guards. Deal with them, or we both hang!"),
    2: dict(kind="kill", target=GUARDS[0], count=2,
            journal="Kill the two Highpass guards who followed you to the shady trafficker.",
            events=json.dumps([{"on": "start", "do": "spawn", "npc": g, "hostile": True, "minutes": 15, "group": True}
                               for g in GUARDS], separators=(",", ":"))),
    10: dict(kind="talk", target="Cogwhisker",
             journal="Settle Cogwhisker's debt at the bandit camp by the lake in northern Bandit Hills.",
             choices=json.dumps({
                 "ask": "The Topi sent you? I can pay what I owe them... or you can try to take it out of my hide.",
                 "options": [
                     {"answer": f"Pay the Topi's debt ({COST:,} tunar)", "cost": COST, "skip": 1,
                      "says": "Fine, fine. Tell the Swashbuckler we are square."},
                     {"answer": "Settle it in blood", "says": "Then come and take it! Boys, to me!"}]},
                 separators=(",", ":"))),
}
KILL_COG = dict(journal="Kill Cogwhisker.", factions="Topi Di Ombra:+200",
                events=json.dumps([{"on": "start", "do": "attack", "npc": "Cogwhisker"},
                                   {"on": "start", "do": "spawn", "npc": "bandit henchman", "hostile": True, "count": 2,
                                    "minutes": 15, "group": True}], separators=(",", ":")))
GIVE_PURSE_JOURNAL = "Now hand the shady trafficker the Swashbuckler's coin purse."
NOTES_ADD = (" Since petition 2487 (2026-11-20, tools/rat_were_2487_20261120.py): the Highpass ambush (two guards; the dealer "
             "to keep alive is left out) and Cogwhisker paid off (1,000 tunar, ours) or fought with two henchmen (+200 "
             "Topi Di Ombra, the guide's rumour).")
AGGRO = {"npc_name": "Cogwhisker", "aggressive": 0, "radius": 0,
         "note": "petition 2487: talked to first (the rat were quest's bribe or fight)", "source": "gm", "leash": 0}
KEYED = ("queststeptargets", "questwalls", "questdoors")


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def audit(db, action, tbl, key, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key), go_json(before), go_json(after), detail))


def rows(db, sql, args=()):
    cur = db.execute(sql, args)
    cols = [d[0] for d in cur.description]
    return [dict(zip(cols, r)) for r in cur.fetchall()]


def steps(db):
    return rows(db, "SELECT * FROM queststeps WHERE questid = ? ORDER BY step", (QUEST,))


def new_steps(old):
    """The twelve rows, whole, from the nine."""
    blank = {k: v for k, v in old[0].items()}
    for k in blank:
        if k not in ("questid",):
            blank[k] = "" if isinstance(old[0][k], str) else 0
    out = []
    for n in range(1, 13):
        if n in FROM_OLD:
            r = dict(old[FROM_OLD[n] - 1])
        else:
            r = dict(blank)
            r.update(count=1, radius=15.0, world=0, x=0.0, y=0.0, z=0.0)
            r.update(NEW_STEPS[n])
        r["step"] = n
        if n == 3:
            r["journal"] = GIVE_PURSE_JOURNAL
        if n == 11:
            r.update(KILL_COG)
        out.append(r)
    return out


def plan(db):
    for t in ("quests", "queststeps", "charquests", "npcaggro", "gm_audit"):
        if not table(db, t):
            return None, [f"no {t} table here"], []
    q = rows(db, "SELECT * FROM quests WHERE questid = ?", (QUEST,))
    if not q or q[0]["name"] != QUEST_NAME:
        return None, [f"quest {QUEST} is {q and q[0]['name']!r}, not {QUEST_NAME!r}"], []
    if table(db, LOG) and db.execute(f"SELECT COUNT(*) FROM {LOG}").fetchone()[0]:
        return "done", [], ["applied already (rat_were_2487_log holds it)"]
    have = [(r["kind"], r["target"]) for r in steps(db)]
    refusals = []
    if have != OLD_STEPS:
        refusals.append(f"{QUEST_NAME}'s steps are {have}, not the import's nine (changed since: a GM's?)")
    for t in KEYED:
        if table(db, t) and db.execute(f"SELECT COUNT(*) FROM {t} WHERE questid = ?", (QUEST,)).fetchone()[0]:
            refusals.append(f"{t} has rows of quest {QUEST}: their steps would need moving too")
    if db.execute("SELECT COUNT(*) FROM npcaggro WHERE npc_name = ? COLLATE NOCASE", (AGGRO["npc_name"],)).fetchone()[0]:
        refusals.append("npcaggro has a Cogwhisker row already")
    for g in GUARDS + ("Cogwhisker", "bandit henchman"):
        if not db.execute("SELECT COUNT(*) FROM npcs WHERE npc_name = ?", (g,)).fetchone()[0]:
            refusals.append(f"no NPC {g!r} to copy")
    if refusals:
        return None, refusals, []
    on = rows(db, "SELECT serverid, step FROM charquests WHERE questid = ? AND step > 0", (QUEST,))
    return "todo", [], [f"{QUEST_NAME}: 9 steps -> 12 (the Highpass ambush, Cogwhisker's bribe or fight); "
                        f"{len(on)} character(s) on it moved: " + ", ".join(f"{r['serverid']} {r['step']}->{MOVE[r['step']]}" for r in on)]


def log(db, tbl, key, before, after):
    db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES (?, ?, ?, ?)",
               (tbl, json.dumps(key, sort_keys=True), json.dumps(before, sort_keys=True), json.dumps(after, sort_keys=True)))
    audit(db, "insert" if before is None else "delete" if after is None else "update", tbl, key, before, after, DETAIL)


def insert(db, tbl, r):
    db.execute(f"INSERT INTO {tbl} ({', '.join(r)}) VALUES ({', '.join('?' for _ in r)})", list(r.values()))


def apply(db):
    db.execute(f"""CREATE TABLE IF NOT EXISTS {LOG} (
        seq INTEGER PRIMARY KEY AUTOINCREMENT, tbl TEXT NOT NULL, rowkey TEXT NOT NULL,
        before TEXT, after TEXT)""")
    old = steps(db)
    for r in old:
        db.execute("DELETE FROM queststeps WHERE questid = ? AND step = ?", (QUEST, r["step"]))
        log(db, "queststeps", {"questid": QUEST, "step": r["step"]}, r, None)
    for r in new_steps(old):
        insert(db, "queststeps", r)
        log(db, "queststeps", {"questid": QUEST, "step": r["step"]}, None, r)
    tr = {"questid": QUEST, "step": 2, "target": GUARDS[1]}
    insert(db, "queststeptargets", tr)
    log(db, "queststeptargets", tr, None, tr)
    for r in rows(db, "SELECT * FROM charquests WHERE questid = ? AND step > 0", (QUEST,)):
        a = dict(r, step=MOVE[r["step"]], count=0)
        db.execute("UPDATE charquests SET step = ?, count = 0 WHERE serverid = ? AND questid = ?", (a["step"], r["serverid"], QUEST))
        log(db, "charquests", {"serverid": r["serverid"], "questid": QUEST}, r, a)
    q = rows(db, "SELECT * FROM quests WHERE questid = ?", (QUEST,))[0]
    db.execute("UPDATE quests SET notes = ? WHERE questid = ?", (q["notes"] + NOTES_ADD, QUEST))
    log(db, "quests", {"questid": QUEST}, q, dict(q, notes=q["notes"] + NOTES_ADD))
    cols = [c[1] for c in db.execute("PRAGMA table_info(npcaggro)")]
    ag = {k: v for k, v in AGGRO.items() if k in cols}
    insert(db, "npcaggro", ag)
    log(db, "npcaggro", {"npc_name": AGGRO["npc_name"]}, None, rows(db, "SELECT * FROM npcaggro WHERE npc_name = ?", (AGGRO["npc_name"],))[0])


def now_row(db, tbl, key):
    w = " AND ".join(f"{k} = ?" for k in key)
    r = rows(db, f"SELECT * FROM {tbl} WHERE {w}", list(key.values()))
    return r[0] if r else None


def undo(db):
    if not table(db, LOG):
        return 0
    entries = rows(db, f"SELECT * FROM {LOG} ORDER BY seq DESC")
    changed = []
    for e in entries:
        key, after = json.loads(e["rowkey"]), json.loads(e["after"]) if e["after"] else None
        if after is not None and now_row(db, e["tbl"], key) != after:
            changed.append(f"{e['tbl']} {key}")
    if changed:
        raise SystemExit("REFUSED: changed since it was written: " + "; ".join(changed))
    for e in entries:
        tbl, key = e["tbl"], json.loads(e["rowkey"])
        before = json.loads(e["before"]) if e["before"] else None
        after = json.loads(e["after"]) if e["after"] else None
        w = " AND ".join(f"{k} = ?" for k in key)
        if after is not None:
            db.execute(f"DELETE FROM {tbl} WHERE {w}", list(key.values()))
        if before is not None:
            insert(db, tbl, before)
        audit(db, "undo", tbl, key, after, before, "undo: " + DETAIL)
    db.execute(f"DROP TABLE {LOG}")
    return len(entries)


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            n = undo(db)
            db.execute("COMMIT")
            print(f"put back {n} row changes" if n else "nothing to undo")
            return 0
        state, refusals, notes = plan(db)
        for r in refusals:
            print("REFUSED:", r)
        for n in notes:
            print(n)
        if state != "todo" or not a.apply:
            db.execute("ROLLBACK")
            print({None: "", "done": "nothing to do"}.get(state, "report only (--apply to write)"))
            return 1 if state is None else 0
        apply(db)
        db.execute("COMMIT")
        print("applied")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
