#!/usr/bin/env python3
"""The dark elf quests of levels 1-20 walked against the players' guides (2026-11-14, batch tix5, lane crabs).

The owner: "dark elf quests are messed up the live player is saying", with the fandom wiki's All Race/Class Starting
Quests (Level 1-15) (Singz's walkthroughs); this week's Live dark elves (Innoruuk, Lanys, Valizrae, Trulyblind,
Groceries) met The Hazinak Report's duplicate R`Noxt, the necromancer's D`Unarr / J`Narus handoff, vague starter
text, The Ceremony's reward and the spikeshell crabs (each fixed on its own). Every quest of the nine classes (Neriak,
1-20, and The Ceremony at 17) walked step by step against the wiki and eqoa.forumotion.com's t53-t61 and t101, on both
shards. Givers, chains (requires), merchants' wares and creatures agree with the guides; what did not:
  - the fire bats of the level 5 quests are not "north" of Neriak: ours stand on the hill north-west of the city and
    by the rock spires south of it, and the journals and words say so (every class);
  - the crabs of the level 7 Grobb quests ring Grobb's shores, not "north-west of Grobb" (journals, Chef Blignart);
  - four level 20 quests said "Choose your path." and gave one spell: now the guides' pick of two (shadowknight
    Bloodwasp or Shadow Tower, necromancer Blood Gale or Power Gale, rogue Avoidance or Minor Wound, scrolls as the
    other cities' quests give; enchanter Spacious Mind or Power Boon);
  - the alchemist's Zezilbe is south of Klik'Anon, not south-east of Neriak; the cleric's level 7 offer still named
    the deleted Denouncer R'Noxt; tomb rats, Blakedown and the waysender named where the text was vague;
  - Leelak Hk'Arr (npcs 102374), a second, attackable Leelak beside the merchant Leelak Hk'Aarr with no wares: deleted
    (both shards), with its respawn row.
Each change and why: tools/darkelf_quests_20261114.json, each shard's value before.

Refused, nothing written, when a row is not as either shard had it (a GM's edit since). Report only by default; --apply
in one transaction, a gm_audit line a row, log darkelf_quests_log; a second --apply does nothing; --undo exact (a row
changed since is left and said; a deleted row put back). --city limits it to classes (the plan's keys: alchemist, ...,
ceremony, neriak for the NPC). Quests are read at the server's start and a GM's reload, NPCs at the start: a restart
(a keep restart).

    python3 -E -s tools/darkelf_quests_20261114.py --db eqoa.db [--apply | --undo] [--city necromancer,rogue]
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

LOG = "darkelf_quests_log"
WHO, VIA = "dab", "tool"
PLAN = os.path.join(os.path.dirname(os.path.abspath(__file__)), "darkelf_quests_20261114.json")
DETAIL = "the dark elf quests 1-20 walked against the fandom wiki and the forum guides: tools/darkelf_quests_20261114.py"
QCOLS = {"offer", "accept", "decline", "declined", "started", "giver", "requires", "minlevel", "maxlevel", "name",
         "classes", "races", "enabled", "events"}
SCOLS = {"journal", "answer", "says", "waiting", "notyet", "reminder", "target", "items", "count", "chance", "kind",
         "npcid", "tunar", "world", "x", "y", "z", "radius", "xp", "rewardtunar", "rewarditems", "factions", "spells",
         "events", "byclass", "choices"}

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    city    TEXT NOT NULL,
    questid INTEGER NOT NULL,
    step    INTEGER NOT NULL,
    col     TEXT NOT NULL,
    before  TEXT,
    after   TEXT,
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def compact(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False)


ROW = "*row*"  # a whole row deleted (after None): an NPC duplicate; --undo puts it back
ROWTABLES = {-1: ("npcs", "id"), -2: ("npcrespawn", "npc_row")}


def where(step):
    if step in ROWTABLES:
        tbl, key = ROWTABLES[step]
        return tbl, f"{key} = ?"
    return ("quests", "questid = ?") if step == 0 else ("queststeps", "questid = ? AND step = ?")


def keys(qid, step):
    return (qid, step) if step > 0 else (qid,)


def rowdict(db, sql, args):
    cur = db.execute(sql, args)
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def current(db, qid, step, col):
    tbl, w = where(step)
    if col == ROW:
        r = rowdict(db, f"SELECT * FROM {tbl} WHERE {w}", keys(qid, step))
        return (r,)
    r = db.execute(f"SELECT {col} FROM {tbl} WHERE {w}", keys(qid, step)).fetchone()
    return None if r is None else (r[0],)


def audit(db, qid, step, col, before, after):
    tbl, _ = where(step)
    if step in ROWTABLES:
        rowkey = {ROWTABLES[step][1]: qid}
        action = "delete" if after is None else "insert"
        b, a = before or {}, after or {}
    else:
        rowkey = {"questid": qid} if step == 0 else {"questid": qid, "step": step}
        action, b, a = "update", {col: before}, {col: after}
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, compact(rowkey), compact(b), compact(a), DETAIL))
    return cur.lastrowid


def load(cities, path=PLAN):
    with open(path, encoding="utf-8") as f:
        plan = json.load(f)
    out = []
    for e in plan["changes"]:
        if cities and e["city"] not in cities:
            continue
        col, step = e["col"], e["step"]
        if step in ROWTABLES:
            if col != ROW or any(v["after"] is not None for v in e["variants"]):
                raise SystemExit(f"plan: {e}: a row table's change is a whole row's delete")
        elif (step == 0 and col not in QCOLS) or (step > 0 and col not in SCOLS):
            raise SystemExit(f"plan: column {col!r} of quest {e['questid']} step {step} is not one this tool writes")
        out.append(e)
    return plan, out


def plan_changes(db, entries):
    changes, already, refusals, missing = [], 0, [], 0
    for e in entries:
        qid, step, col = e["questid"], e["step"], e["col"]
        now = current(db, qid, step, col)
        if col == ROW and now == (None,):
            already += 1  # deleted already (or never on this shard)
            continue
        if now is None:
            missing += 1  # the other shard's quest or step
            continue
        v = now[0]
        afters = [x["after"] for x in e["variants"]]
        befores = [x["before"] for x in e["variants"]]
        if v in befores:
            after = e["variants"][befores.index(v)]["after"]
            if after != v:
                changes.append((e["city"], qid, step, col, v, after))
        elif v in afters:
            already += 1
        else:
            refusals.append(f"quest {qid} step {step} {col} = {v!r}: not as either shard had it")
    return changes, already, refusals, missing


def write(db, qid, step, col, before, after):
    tbl, w = where(step)
    if col == ROW:
        now = rowdict(db, f"SELECT * FROM {tbl} WHERE {w}", keys(qid, step))
        if after is None:
            if now != before:
                return False
            return db.execute(f"DELETE FROM {tbl} WHERE {w}", keys(qid, step)).rowcount == 1
        if now is not None:
            return False
        names = list(after)
        db.execute(f"INSERT INTO {tbl} ({', '.join(names)}) VALUES ({', '.join('?' * len(names))})", [after[n] for n in names])
        return True
    cur = db.execute(f"UPDATE {tbl} SET {col} = ? WHERE {w} AND {col} IS ?", (after, *keys(qid, step), before))
    return cur.rowcount == 1


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for _, city, qid, step, col, before, after, _a in db.execute(f"SELECT * FROM {LOG} ORDER BY seq DESC").fetchall():
        before, after = json.loads(before), json.loads(after)
        if not write(db, qid, step, col, after, before):
            print(f"left: {city} quest {qid} step {step} {col} changed since")
            left += 1
            continue
        audit(db, qid, step, col, after, before)
        back += 1
    if not left:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    ap.add_argument("--city", default="", help="comma separated cities (the plan's city keys); default all")
    ap.add_argument("--verbose", action="store_true", help="print every change")
    ap.add_argument("--plan", default=PLAN, help="the plan (default tools/darkelf_quests_20261114.json)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    cities = {c.strip() for c in a.city.split(",") if c.strip()}
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        if a.undo:
            db.execute("BEGIN IMMEDIATE")
            try:
                back, left = undo(db)
                db.execute("COMMIT")
            except BaseException:
                db.execute("ROLLBACK")
                raise
            print(f"put back {back} rows, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        if table(db, LOG):
            print(f"applied already ({LOG} is there): --undo first")
            return 0
        plan, entries = load(cities, a.plan)
        known = {e["city"] for e in plan["changes"]}
        if cities - known:
            sys.exit(f"--city: unknown {sorted(cities - known)} (the plan has {sorted(known)})")
        changes, already, refusals, missing = plan_changes(db, entries)
        by = {}
        for c in changes:
            by.setdefault(c[0], [0, 0])[0 if c[3] == "journal" else 1] += 1
        for city in sorted(by):
            print(f"{city}: {by[city][0]} journal lines, {by[city][1]} other columns")
        if a.verbose:
            for city, qid, step, col, before, after in changes:
                print(f"  {city} {qid}/{step} {col}: {before!r} -> {after!r}")
        print(f"{len(changes)} to write, {already} as the plan has them already, {missing} not on this shard")
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not changes:
            print("nothing to do")
            return 0
        if not a.apply:
            print(f"report only: {len(changes)} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            db.execute(LOG_SCHEMA)
            for city, qid, step, col, before, after in changes:
                if not write(db, qid, step, col, before, after):
                    raise RuntimeError(f"quest {qid} step {step} {col} changed under the update")
                a_id = audit(db, qid, step, col, before, after)
                db.execute(f"INSERT INTO {LOG} (city, questid, step, col, before, after, audit) VALUES (?, ?, ?, ?, ?, ?, ?)",
                           (city, qid, step, col, json.dumps(before), json.dumps(after), a_id))
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {len(changes)} rows, log {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
