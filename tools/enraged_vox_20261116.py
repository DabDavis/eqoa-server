#!/usr/bin/env python3
"""The enraged Lady Vox a quest spawn only (the owner, 2026-11-16: "enraged lady vox is a quest spawn only but she
spawned normal like lady vox").

WHAT DIFFERED (quest 101028 "The Scepter of the Ritualists"; Enraged Lady Vox npcs 122824, beside Lady Vox 103173):
  - She stood in Permafrost for anyone, on a 2 h respawn (npcrespawn 122824, a GM's board #1093). Now spawn-only
    (npcspawnonly "#122824", her row alone: Lady Vox 103173 stands as she did). The respawn row is left: moot while
    she is spawn-only (and it comes back into force if this is undone).
  - Step 13 (Mistress Sanchez) gives the primal stew (701090) and step 14's journal says "Set the primal stew down in
    Permafrost, then defeat the enraged Lady Vox", but no step set it down and nothing raised her. Now a reach step
    14 at her lair (her row's spot, radius 30) wants the stew held and takes it (set down: handlers/quests.go
    checkReach, reachTakes, branch enraged-vox); its doing fires the new trigger "Permafrost: Enraged Lady Vox",
    which raises one hostile copy of her row (by her name: her row is the only one) there, one at a time (its lock: the step waits, the stew kept, while
    another's Vox lives), for an hour at most, ten minutes with no one by her ending it (Kratas's trigger's shape,
    tools/kratas_20261116.py). The pattern of quest 101307 step 15 (the stew and Enraged Aeyinar).
  - The old steps 14-31 are 15-32 (the loot step "loot Enraged Lady Vox 701091" unchanged but its journal line, now
    "Defeat the enraged Lady Vox ..."). A character on old step 14 goes to the new step 14 (she rises only there now;
    its count 0); one on 15-31 to the step one higher, its count kept.

Report only by default; --apply in one transaction with gm_audit lines and the log enraged_vox_1116_log (every row
before and after); a second --apply does nothing; --undo puts back exactly what it wrote (a row changed since is left
and said) and drops the log. Refused whole if a row is not as this tool expects.

WHEN IT TAKES EFFECT: quests and charquests at a start (a keep-restart's before.sh, the server stopped: a running
server keeps its characters' quest rows in memory and saves over them); npcspawnonly at the GM panel's reload_npcs or
a start; the trigger at a start. The reach step's taking of the stew needs the server of branch enraged-vox (an older
server does the step on arrival, taking nothing). Run it in the before.sh.

    python3 -E -s tools/enraged_vox_20261116.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "enraged_vox_1116_log"
WHO, VIA = "dab", "tool"
SRC = "enragedvox1116"
DETAIL = ("the enraged Lady Vox a quest spawn only (the owner, 2026-11-16): the stew set down at her lair raises her: "
          "tools/enraged_vox_20261116.py")

QUEST = 101028
QUEST_NAME = "The Scepter of the Ritualists"
VOX_ROW = 122824
VOX = "Enraged Lady Vox"
SPAWNONLY = f"#{VOX_ROW}"
STEW, SCALE = 701090, 701091
AT = 14            # the new step's number; the old 14.. move up one
LAST = 31          # the quest's last step today
RADIUS = 30.0

TRIGGER = "Permafrost: Enraged Lady Vox"
LOCK = "enraged lady vox"
BUSY = "The enraged Lady Vox is awake already: wait until she falls or sleeps again."
SETDOWN = "You set the primal stew down. Its smell fills the lair, and Lady Vox wakes, enraged!"
REMINDER = "Only the smell of the primal stew from Mistress Sanchez will wake the enraged Lady Vox."
JOURNAL = "Set the primal stew down in Lady Vox's lair in Permafrost."

OLD13 = {"kind": "give", "target": "Mistress Sanchez", "items": "701088, 701089",
         "events": '[{"on":"done","do":"give","items":"701090"}]'}
OLD14 = {"kind": "loot", "target": VOX, "items": str(SCALE),
         "journal": "Set the primal stew down in Permafrost, then defeat the enraged Lady Vox."}
NEW_LOOT_JOURNAL = "Defeat the enraged Lady Vox in her lair in Permafrost."
OLD15 = {"kind": "talk", "target": "Mistress Sanchez"}


def has(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone() is not None


def row(db, tbl, where, args):
    cur = db.execute(f'SELECT * FROM "{tbl}" WHERE {where}', args)
    r = cur.fetchone()
    return dict(zip([d[0] for d in cur.description], r)) if r else None


def js(v):
    return json.dumps(v, separators=(",", ":")) if v is not None else None


def vox_place(db):
    r = row(db, "npcs", "id = ?", (VOX_ROW,))
    return r["world"], round(r["x_coord"], 1), round(r["y_coord"], 1), round(r["z_coord"], 1)


def trigger_steps(db):
    world, x, y, z = vox_place(db)
    return [
        {"do": "lock", "lock": LOCK},
        {"do": "clock", "seconds": 3600, "empty": 600},
        {"do": "spawn", "npc": VOX, "hostile": True, "minutes": 60, "spread": 1, "world": world, "x": x, "y": y,
         "z": z},
        {"do": "awaitdead"},
        {"do": "end"},
        {"do": "despawn"},
    ]


def step_events():
    return json.dumps([{"on": "done", "do": "trigger", "trigger": TRIGGER, "text": BUSY},
                       {"on": "done", "do": "popup", "text": SETDOWN}], separators=(",", ":"))


def on_quest(db):
    """The characters on the quest now (not finished, or on it again): (serverid, name, step, count, done)."""
    names = has(db, "characters")
    out = []
    for sid, step, count, done in db.execute(
            "SELECT serverid, step, count, done FROM charquests WHERE questid = ? AND step > 0 ORDER BY serverid", (QUEST,)):
        n = db.execute("SELECT charname FROM characters WHERE serverid = ?", (sid,)).fetchone() if names else None
        out.append((sid, n[0] if n else "?", step, count, done))
    return out


def moved_to(step):
    """The step a character on an old step goes to."""
    if step == AT:
        return AT  # the new reach step: she rises only there now
    if step > AT:
        return step + 1
    return step


def check(db):
    """What is wrong with the database for this tool, [] for nothing."""
    bad = []
    q = row(db, "quests", "questid = ?", (QUEST,))
    if q is None or q["name"] != QUEST_NAME:
        bad.append(f"quests {QUEST}: {q and q['name']!r}, want {QUEST_NAME!r}")
    steps = [r[0] for r in db.execute("SELECT step FROM queststeps WHERE questid = ? ORDER BY step", (QUEST,))]
    if steps != list(range(1, LAST + 1)):
        bad.append(f"queststeps {QUEST}: steps {steps}, want 1-{LAST}")
    else:
        for step, want in ((13, OLD13), (14, OLD14), (15, OLD15)):
            r = row(db, "queststeps", "questid = ? AND step = ?", (QUEST, step))
            for c, v in want.items():
                if r[c] != v:
                    bad.append(f"queststeps {QUEST} step {step} {c}: {r[c]!r}, want {v!r}")
    v = row(db, "npcs", "id = ?", (VOX_ROW,))
    if v is None or v["npc_name"] != VOX or v["world"] != 0:
        bad.append(f"npcs {VOX_ROW}: {v and (v['npc_name'], v['world'])}, want {VOX} in world 0")
    if db.execute("SELECT count(*) FROM npcs WHERE lower(trim(npc_name)) = ?", (VOX.lower(),)).fetchone()[0] != 1:
        bad.append(f"not exactly one npcs row named {VOX} (the trigger copies her by name)")
    if row(db, "npcspawnonly", "name = ?", (SPAWNONLY,)) or row(db, "npcspawnonly", "lower(name) = ?", (VOX.lower(),)):
        bad.append(f"npcspawnonly {SPAWNONLY} (or her name) is there already")
    for pid in (STEW, SCALE):
        if row(db, "itempattern", "patternid = ?", (pid,)) is None:
            bad.append(f"no itempattern {pid}")
    if row(db, "gmtriggers", "name = ?", (TRIGGER,)):
        bad.append(f"a trigger {TRIGGER!r} is there already")
    for tbl in ("questdoors", "questwalls", "itemhide", "queststeptargets"):
        if has(db, tbl) and db.execute(f"SELECT count(*) FROM {tbl} WHERE questid = ?", (QUEST,)).fetchone()[0]:
            bad.append(f"{tbl} has rows of quest {QUEST}: their steps would need moving too")
    return bad


def report(db):
    q = row(db, "quests", "questid = ?", (QUEST,))
    print(f"  quest {QUEST}: {q and q['name']!r}")
    for r in db.execute("SELECT step, kind, target, items, world, x, z, radius, events FROM queststeps "
                        "WHERE questid = ? AND step BETWEEN 13 AND 16 ORDER BY step", (QUEST,)):
        print(f"    step {r[0]} {r[1]} {r[2]!r} items {r[3]!r} place {r[4]} ({r[5]:.0f}, {r[6]:.0f}) r {r[7]:.0f} events {r[8]!r}")
    print(f"  {VOX} (npcs {VOX_ROW}): spawn-only {bool(row(db, 'npcspawnonly', 'name = ?', (SPAWNONLY,)))}, "
          f"trigger {bool(row(db, 'gmtriggers', 'name = ?', (TRIGGER,)))}")
    on = on_quest(db)
    print(f"  on the quest now: {len(on)}" + "".join(f"\n    {sid} {n} step {s} count {c} done {d}" for sid, n, s, c, d in on))
    print(f"  finished it: {db.execute('SELECT count(*) FROM charquests WHERE questid = ? AND done > 0', (QUEST,)).fetchone()[0]}")


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

    # the characters on the quest, read before the steps move
    on = on_quest(db)

    # the steps from 14 up one, the last first (each a delete at its old number and an insert at its new, in the log)
    for step in range(LAST, AT - 1, -1):
        before = row(db, "queststeps", "questid = ? AND step = ?", (QUEST, step))
        db.execute("UPDATE queststeps SET step = ? WHERE questid = ? AND step = ?", (step + 1, QUEST, step))
        done("queststeps", {"questid": QUEST, "step": step}, before)
        done("queststeps", {"questid": QUEST, "step": step + 1}, None)
    update("queststeps", {"questid": QUEST, "step": AT + 1}, {"journal": NEW_LOOT_JOURNAL})

    # the new reach step at her lair: the stew held, taken; the trigger raises her
    world, x, y, z = vox_place(db)
    insert("queststeps", {"questid": QUEST, "step": AT, "kind": "reach", "items": str(STEW), "world": world, "x": x,
                          "y": y, "z": z, "radius": RADIUS, "journal": JOURNAL, "reminder": REMINDER,
                          "events": step_events()}, {"questid": QUEST, "step": AT})

    # the characters on the moved steps
    for sid, _name, step, _count, _done in on:
        new = moved_to(step)
        if step < AT:
            continue
        values = {"step": new} if new != AT else {"step": new, "count": 0}
        update("charquests", {"serverid": sid, "questid": QUEST}, values)

    # her standing spawn held: she comes only by the trigger
    insert("npcspawnonly", {"name": SPAWNONLY, "note": f"{VOX}: a quest spawn only (the owner, 2026-11-16), raised by "
                            f"quest {QUEST} step {AT}'s stew at her lair (trigger {TRIGGER!r}); her 2 h respawn "
                            "(npcrespawn, board #1093) moot while this stands: tools/enraged_vox_20261116.py",
                            "source": SRC}, {"name": SPAWNONLY})

    # the trigger the step fires
    slot = next_slot(db)
    insert("gmtriggers", {"slot": slot, "name": TRIGGER, "enabled": 1, "scope": "near", "radius": 120, "world": world,
                          "x": x, "y": y, "z": z, "steps": js(trigger_steps(db)),
                          "notes": f"Fired by quest {QUEST} ({QUEST_NAME}) step {AT}, the primal stew set down at her "
                                   f"lair: one hostile {VOX} (npcs {VOX_ROW}, spawn-only) raised there, one at a time, "
                                   "for an hour at most; ten minutes with no one by her end it. The owner, 2026-11-16: "
                                   "tools/enraged_vox_20261116.py"}, {"slot": slot})


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
        on = on_quest(db)
        moves = [(sid, n, s, moved_to(s)) for sid, n, s, _c, _d in on if s >= AT]
        print(f"  characters to move: {len(moves)}" + "".join(f"\n    {sid} {n}: step {s} -> {t}" for sid, n, s, t in moves))
        if not a.apply:
            print("report only: --apply to write it")
            return 0
        db.execute("BEGIN IMMEDIATE")
        apply(db)
        db.execute("COMMIT")
        print("applied; takes effect at a restart (the quests are read at a start)")
        report(db)
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
