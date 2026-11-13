#!/usr/bin/env python3
"""Mr. Moogie's GM tickets of 2026-11-13 (the GM Jibrish, playing Jibwiz: petitions 1892, 1894 and 1896), on the server's
code of the same branch (entity/ward.go and entity/deathhold.go: the "ward" and "deathhold" ability kinds; an older
server reads this template as unreadable):

  classtemplates "mr moogie": his nine abilities as they are (the tools of 2026-11-10 and 11-11), and after them
    1892, a ward at 15%: once a fight, below 15% of his hit points, every hit that lands on him heals him 500 more
      (beside his hit heal of 150) and is answered with 5,000 pure damage (no resist) on whoever landed it, a
      player, a pet, a mercenary or an NPC; named "Royal Ward" in its lines (ours), no line of his (none asked);
    1894, a hold at 1 hit point: once a fight, the hit that would kill him leaves him at 1, frozen and taking nothing
      for 30 s, and he says "You thought you had me, didn't you? I got 30 seconds left in me, can you survive?";
      then his fight goes on at 1 and the next hit kills him;
  gmtriggers, a new slot after the last, "Mr. Moogie Defeated": scope all, one broadcast step, the GM BROADCAST's
    line in the GMs' yellow to every player in the world:
      "You have won for now mortals. But I do not forgive and I do not forget." - Mr. Moogie
  npcdeath ("mr. moogie", that trigger, 100%): his death fires it (1896).

The template is found by its name and left alone, nothing written, unless its abilities are the nine the tools of
2026-11-10/11 wrote (fix_moogie_tunare_20261110.py's seven, fix_innoruuk_visuals_20261110.py's killsay,
moogie_laststand_20261111.py's last stand); a trigger already named so, or a death row for him to it: refused. No Mr.
Moogie template: another shard, nothing done. Report only by default; --apply in one transaction, a gm_audit line a
row, log moogie_gm_1113_log; --undo exact (a row changed since left and said).

WHEN IT TAKES EFFECT: the templates are read at the server's start or a GM panel template edit (reload_npcs): run it
as the before.sh of the keep restart bringing this code, never under an older server. The trigger and its death row
are read when the trigger engine next reads its tables (a trigger saved in the panel, or a restart).

    tools/moogie_gm_20261113.py --db eqoa.db [--apply | --undo]
"""
import argparse
import os
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import fix_moogie_tunare_20261110 as fm  # noqa: E402  the writer
import moogie_laststand_20261111 as ml  # noqa: E402  his nine abilities as they stand

LOG = "moogie_gm_1113_log"
DETAIL = "Mr. Moogie's ward, hold and death line (petitions 1892, 1894, 1896, Jibrish, 2026-11-13): tools/moogie_gm_20261113.py"
MOOGIE_NINE = ml.MOOGIE_NOW + [ml.LASTSTAND]
HOLD_SAY = "You thought you had me, didn't you? I got 30 seconds left in me, can you survive?"
WARD = {"kind": "ward", "below": 15, "heal": 500, "damage": 5000, "name": "Royal Ward"}
HOLD = {"kind": "deathhold", "hold": 30, "say": HOLD_SAY}
TRIGGER = "Mr. Moogie Defeated"
ANNOUNCE = '"You have won for now mortals. But I do not forgive and I do not forget." - Mr. Moogie'
STEPS = [{"do": "broadcast", "text": ANNOUNCE}]
DEATH_KEY = fm.npc_key(fm.MOOGIE_NAME)  # "mr. moogie", as database.NPCClassKey keys a creature's death


def plan(db):
    """(changes, lines, refusals, here)."""
    moogie = fm.row_of(db, "classtemplates", {"name": fm.MOOGIE_T})
    if moogie is None:
        return [], [f"no class template {fm.MOOGIE_T!r} here: not his shard"], [], False
    refusals = []
    if not fm.same_abilities(moogie["abilities"], MOOGIE_NINE):
        refusals.append(f"{fm.MOOGIE_T!r}'s abilities are not the nine of the 2026-11-10/11 tools: {moogie['abilities']}")
    if db.execute("SELECT count(*) FROM gmtriggers WHERE lower(trim(name)) = lower(?)", (TRIGGER,)).fetchone()[0]:
        refusals.append(f"a trigger named {TRIGGER!r} is there already")
    if db.execute("SELECT count(*) FROM npcdeath WHERE name = ? AND lower(trigger) = lower(?)", (DEATH_KEY, TRIGGER)).fetchone()[0]:
        refusals.append(f"npcdeath ({DEATH_KEY!r}, {TRIGGER!r}) is there already")
    if refusals:
        return [], [], refusals, True
    slot = (db.execute("SELECT max(slot) FROM gmtriggers").fetchone()[0] or 8) + 1
    slot = max(slot, 9)  # past the TEST menu's eight lines
    after = dict(moogie, abilities=fm.abilities_json(MOOGIE_NINE + [WARD, HOLD]))
    trig = {"slot": slot, "name": TRIGGER, "enabled": 1, "scope": "all", "radius": 0.0, "world": -1, "x": 0.0, "y": 0.0, "z": 0.0,
            "steps": fm.abilities_json(STEPS),
            "notes": "Petition 1896 (Jibrish, 2026-11-13): Mr. Moogie's death fires it (npcdeath), the GM broadcast to everyone."}
    death = {"name": DEATH_KEY, "trigger": TRIGGER, "chance": 100.0, "note": "Petition 1896: the server-wide line at his defeat.",
             "source": "moogie1113"}
    lines = [
        f"classtemplates {fm.MOOGIE_T!r}: after his nine abilities, 1892's ward below {WARD['below']}% (+{WARD['heal']} heal and "
        f"{WARD['damage']} pure damage on the hitter at every hit, \"{WARD['name']}\") and 1894's hold at 1 hit point for "
        f"{HOLD['hold']} s, saying \"{HOLD_SAY}\"",
        f"gmtriggers slot {slot}: {TRIGGER!r}, scope all, a broadcast: {ANNOUNCE}",
        f"npcdeath ({DEATH_KEY!r}, {TRIGGER!r}, 100%): his death fires it",
    ]
    return [("classtemplates", moogie, after), ("gmtriggers", None, trig), ("npcdeath", None, death)], lines, [], True


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    fm.LOG, fm.DETAIL = LOG, DETAIL  # its log and its audit's words, through the raid gods' tool's writer
    fm.LOG_SCHEMA = fm.LOG_SCHEMA.replace("moogie_tunare_1110_log", LOG)
    fm.KEYS.update({"gmtriggers": ("slot",), "npcdeath": ("name", "trigger")})
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        if a.undo:
            db.execute("BEGIN IMMEDIATE")
            try:
                back, left = fm.undo(db)
                db.execute("COMMIT")
            except BaseException:
                db.execute("ROLLBACK")
                raise
            print(f"put back {back} rows, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        if fm.table(db, LOG):
            print(f"applied already ({LOG} is there): --undo first")
            return 1
        changes, lines, refusals, here = plan(db)
        for line in lines:
            print(line)
        if not here:
            print("nothing done")
            return 0
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
            n = fm.apply(db, changes)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {n} rows, each in gm_audit and {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
