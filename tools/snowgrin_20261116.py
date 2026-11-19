#!/usr/bin/env python3
"""The Snowgrin quest (101267) as the owner's walkthrough has it (2026-11-16).

The source: eqoa.forumotion.com/t179-snowgrin-quest (Ronson, 2010-07-26), sent by the owner, whose drops and
rewards count as his approval. Read against our quest, its creatures and Snowgrin's loot on both shards; what it
says that we had otherwise:

  - Warlord Kraven is a creature that spawns "about once a day", as Relk IceMapper and General FrostBeard are ("Takes
    awhile to get all three spawns, they spawn about once a day"): he spawns "right in front of this ramp". Ours had a
    reach step at his spot (step 3) whose being reached spawned a hostile COPY of him for the player (step 4's start):
    a key on demand, with no wait for his spawn. He stands in the world already, attackable (npc_type 8) and back
    every 86,400 s (npcrespawn, a GM's entry 668), so the reach step goes and step 4 loses its spawn; the steps
    after move down one (4-7 -> 3-6), and a character on one of them with it.
  - "First will spawn Grilmash Frostcackle ... who speaks rhyming incantation and then spawns Snowgrim": ours spawned
    both at the kill step's start, after a popup that narrated the rhyme. Now the kill step's start spawns Grilmesh,
    he says his rhyme (a `say` event, his name before it), then Snowgrin spawns (hostile, as before). The walkthrough
    gives no words: the rhyme is ours. The popup at the tapestry goes.
  - "the ice goblin thieves, wizards, and shaman all walk up and down hallway (which see invis, be warned)": none of
    the three names saw invisibility. Each gets an npcaggro row, aggressive -1 (no word: the world's rules) and
    see_invis 1. By name, as npcaggro is: every one of those names (all in Vox's castle, but for one level 20 ice
    goblin shaman at (12636, 7538) in world 0, and Heirloom's world 14 copies) sees it.
  - The journals of the two keys say where they spawn as the walkthrough does (Relk "in the middle of that large
    room"; Kraven's directions from McDonough's room, which the reach step's line had carried), each within the
    journal's 127 characters.

The rest matches the walkthrough and is left: the giver (Karis McDonogh, "in the long hallway" of the second level),
level 55+ for all classes, the three keys looted off the three (pre-looting takes them in any order), the keys
handed to McDonogh, the spot behind the throne room's tapestry, Snowgrin's 780 cold AoE as he appears and every 20 s,
no quest reward, and his drops: every one of the walkthrough's names drops on both shards (its "Glacier Fell Hammer
of Divine Grace" and "Storm's Edge Tobrin's eyepatch" are two items each, the list being alphabetical).

Refused, nothing written, when a row is not as expected (a GM's edit since). Report only by default; --apply in one
transaction, a gm_audit line a row, log snowgrin_1116_log; a second --apply does nothing; --undo exact (a row
changed since is left and said). Quests are read at the server's start: apply before a (re)start (before.sh).

    python3 -E -s tools/snowgrin_20261116.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "snowgrin_1116_log"
WHO, VIA = "dab", "tool"
DETAIL = ("the Snowgrin quest as the owner's walkthrough has it (eqoa.forumotion.com t179, the owner 2026-11-16): "
          "tools/snowgrin_20261116.py")
SOURCE = "snowgrin1116"
QUEST = 101267

# what each step is now (kind, target, items, events), refused otherwise
OLD = {
    1: ("loot", "Relk Icemapper", "701375", ""),
    2: ("loot", "General Frostbeard", "701376", ""),
    3: ("reach", "", "", ""),
    4: ("loot", "Warlord Kraven", "701377", '[{"on":"start","do":"spawn","npc":"Warlord Kraven","hostile":true}]'),
    5: ("give", "Karis McDonogh", "701375, 701376, 701377", ""),
    6: ("reach", "", "", '[{"on":"done","do":"popup","text":"Grilmesh Frostcackle cackles a rhyme, and the snow piles up into a twenty-foot snowman!"}]'),
    7: ("kill", "Snowgrin", "", '[{"on":"start","do":"spawn","npc":"Grilmesh Frostcackle"},{"on":"start","do":"spawn","npc":"Snowgrin","hostile":true}]'),
}
OLD_J1 = "Take Relk Icemapper's key: right from this room, right at the hall, second right, down the stairs."
NEW_J1 = "Take Relk Icemapper's key: right, right at the hall, the second right, down the stairs. He spawns mid-room, once a day."
OLD_J4 = "Kill Warlord Kraven down the ramp past the two ice giants in Vox's Castle and take his key."
NEW_J4 = "Take Warlord Kraven's key: left, left, the first left, up past two ice giants, left, down a level. He spawns by the big ramp."
RHYME = ("Snow on stone and frost on bone, the keys are turned, the way is shown!\n"
         "Rise from the drift with a frozen grin: wake for the warm ones, Snowgrin!")
NEW_EV7 = json.dumps([
    {"on": "start", "do": "spawn", "npc": "Grilmesh Frostcackle"},
    {"on": "start", "do": "say", "npc": "Grilmesh Frostcackle", "text": RHYME},
    {"on": "start", "do": "spawn", "npc": "Snowgrin", "hostile": True},
], separators=(",", ":"))
OLD_NOTE = ("Warlord Kraven stands as a townsperson (npc_type 0x80): the set's attackable list makes them attackable, "
            "and the kill step's start spawns a hostile copy.")
NEW_NOTE = ("Warlord Kraven is a standing creature, as Relk Icemapper and General Frostbeard are, back once a day "
            "(npcrespawn 86400): the owner's walkthrough (eqoa.forumotion.com t179, Ronson 2010, 2026-11-16) has no "
            "summoned copy of him, so his reach step and the kill step's spawn are gone. Grilmesh Frostcackle appears "
            "first and says his rhyme (the words are ours: the walkthrough gives none), then Snowgrin "
            "(tools/snowgrin_20261116.py).")
SEE_INVIS = ("ice goblin thief", "ice goblin wizard", "ice goblin shaman")
AGGRO_NOTE = ("the owner's walkthrough (eqoa.forumotion.com t179): the ice goblin thieves, wizards and shaman of Karis "
              "McDonogh's hallway see invis (no word on aggro: the world's rules)")

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,
    rowkey  TEXT NOT NULL,
    before  TEXT,
    after   TEXT,
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def compact(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False)


def rows(db, sql, args=()):
    cur = db.execute(sql, args)
    names = [d[0] for d in cur.description]
    return [dict(zip(names, r)) for r in cur.fetchall()]


def one(db, tbl, key):
    where = " AND ".join(f"{k} = ?" for k in key)
    r = rows(db, f"SELECT * FROM {tbl} WHERE {where}", tuple(key.values()))
    return r[0] if r else None


def audit(db, tbl, key, before, after):
    action = "insert" if before is None else ("delete" if after is None else "update")
    if before is not None and after is not None:  # an update: the columns that change
        cols = [c for c in after if before.get(c) != after[c]]
        before, after = {c: before[c] for c in cols}, {c: after[c] for c in cols}
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, compact(key),
                      compact(before or {}), compact(after or {}), DETAIL))
    return cur.lastrowid


def write(db, tbl, key, before, after):
    """One row from before to after (None: absent), only when it is as before now."""
    now = one(db, tbl, key)
    if now != before:
        return False
    where = " AND ".join(f"{k} = ?" for k in key)
    if after is None:
        return db.execute(f"DELETE FROM {tbl} WHERE {where}", tuple(key.values())).rowcount == 1
    if before is None:
        names = list(after)
        db.execute(f"INSERT INTO {tbl} ({', '.join(names)}) VALUES ({', '.join('?' * len(names))})",
                   [after[n] for n in names])
        return True
    cols = [c for c in after if before.get(c) != after[c]]
    if not cols:
        return True
    db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in cols)} WHERE {where}",
               [after[c] for c in cols] + list(key.values()))
    return True


def plan(db):
    changes, lines, refusals = [], [], []
    q = one(db, "quests", {"questid": QUEST})
    if q is None or q["name"] != "Snowgrin" or q["giver"] != "Karis McDonogh":
        return [], [], [f"quest {QUEST} is not Karis McDonogh's Snowgrin"]
    steps = {r["step"]: r for r in rows(db, "SELECT * FROM queststeps WHERE questid = ? ORDER BY step", (QUEST,))}
    if sorted(steps) != list(range(1, 8)):
        return [], [], [f"quest {QUEST} has steps {sorted(steps)}, want 1-7"]
    for n, (kind, target, items, events) in OLD.items():
        s = steps[n]
        if (s["kind"], s["target"], s["items"], s["events"]) != (kind, target, items, events):
            refusals.append(f"step {n} is ({s['kind']!r}, {s['target']!r}, {s['items']!r}, {s['events']!r}), "
                            f"want ({kind!r}, {target!r}, {items!r}, {events!r})")
    if (steps[3]["x"], steps[3]["z"]) != (4861.0, 4605.0):
        refusals.append(f"step 3 reaches ({steps[3]['x']}, {steps[3]['z']}), not Warlord Kraven's spot (4861, 4605)")
    if steps[1]["journal"] != OLD_J1:
        refusals.append(f"step 1's journal is {steps[1]['journal']!r}")
    if steps[4]["journal"] != OLD_J4:
        refusals.append(f"step 4's journal is {steps[4]['journal']!r}")
    if OLD_NOTE not in q["notes"]:
        refusals.append("the quest's notes lack the Kraven sentence")
    for t in ("queststeptargets", "questwalls", "questdoors"):
        if table(db, t) and db.execute(f"SELECT 1 FROM {t} WHERE questid = ? AND step >= 3", (QUEST,)).fetchone():
            refusals.append(f"{t} holds a row of quest {QUEST} at step 3 or later: not foreseen")
    kraven = rows(db, "SELECT n.id, n.npc_type, r.seconds FROM npcs n LEFT JOIN npcrespawn r ON r.npc_row = n.id "
                      "WHERE lower(n.npc_name) = 'warlord kraven' AND n.world = 0")
    if len(kraven) != 1 or kraven[0]["npc_type"] & 0x80 or kraven[0]["seconds"] != 86400.0:
        refusals.append(f"Warlord Kraven in world 0 is not one attackable spawn back every 86400 s: {kraven}")
    if table(db, "npcspawnonly") and db.execute("SELECT 1 FROM npcspawnonly WHERE name = 'warlord kraven'").fetchone():
        refusals.append("Warlord Kraven is spawn-only: with no copy his key would never drop")
    if refusals:
        return [], [], refusals

    def change(tbl, key, before, after, line):
        changes.append((tbl, key, before, after))
        lines.append(line)

    k = lambda n: {"questid": QUEST, "step": n}
    change("queststeps", k(1), steps[1], dict(steps[1], journal=NEW_J1), "step 1 (Relk Icemapper): journal says where he spawns")
    for n in range(3, 8):
        change("queststeps", k(n), steps[n], None, f"step {n} ({steps[n]['kind']} {steps[n]['target']}) taken out to be renumbered")
    news = {
        3: dict(steps[4], step=3, events="", journal=NEW_J4),
        4: dict(steps[5], step=4),
        5: dict(steps[6], step=5, events=""),
        6: dict(steps[7], step=6, events=NEW_EV7),
    }
    what = {3: "Warlord Kraven's key, his standing spawn's (no copy)", 4: "the keys to Karis McDonogh",
            5: "behind the tapestry (no popup)", 6: "Snowgrin: Grilmesh spawns, says his rhyme, then Snowgrin"}
    for n, r in news.items():
        change("queststeps", k(n), None, r, f"step {n}: {what[n]}")
    change("quests", {"questid": QUEST}, q, dict(q, notes=q["notes"].replace(OLD_NOTE, NEW_NOTE)), "quest notes: Kraven, Grilmesh")
    for c in rows(db, "SELECT * FROM charquests WHERE questid = ? AND step >= 4 ORDER BY serverid", (QUEST,)):
        change("charquests", {"serverid": c["serverid"], "questid": QUEST}, c, dict(c, step=c["step"] - 1),
               f"character {c['serverid']}: step {c['step']} -> {c['step'] - 1}")
    for c in rows(db, "SELECT serverid FROM charquests WHERE questid = ? AND step = 3", (QUEST,)):
        lines.append(f"character {c['serverid']}: on step 3 (the reach), now Kraven's key at the same number")
    for name in SEE_INVIS:
        r = one(db, "npcaggro", {"npc_name": name})
        if r is None:
            change("npcaggro", {"npc_name": name}, None,
                   {"npc_name": name, "aggressive": -1, "radius": 0.0, "note": AGGRO_NOTE, "source": SOURCE,
                    "leash": 0.0, "holding": 0, "see_invis": 1}, f"npcaggro {name}: sees invis (new row)")
        elif not r["see_invis"]:
            change("npcaggro", {"npc_name": name}, r, dict(r, see_invis=1), f"npcaggro {name}: sees invis")
        else:
            lines.append(f"npcaggro {name}: sees invis already")
    return changes, lines, []


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for tbl, rowkey, before, after in db.execute(f"SELECT tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key, before, after = json.loads(rowkey), json.loads(before), json.loads(after)
        if not write(db, tbl, key, after, before):
            print(f"left: {tbl} {rowkey} changed since")
            left += 1
            continue
        audit(db, tbl, key, after, before)
        back += 1
    if not left:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
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
            print(f"applied already ({LOG} is there): nothing to do (--undo first to redo)")
            return 0
        changes, lines, refusals = plan(db)
        for line in lines:
            print(line)
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not a.apply:
            print(f"report only: {len(changes)} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            db.execute(LOG_SCHEMA)
            for tbl, key, before, after in changes:
                if not write(db, tbl, key, before, after):
                    raise RuntimeError(f"{tbl} {key} changed under the update")
                a_id = audit(db, tbl, key, before, after)
                db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                           (tbl, compact(key), json.dumps(before), json.dumps(after), a_id))
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
