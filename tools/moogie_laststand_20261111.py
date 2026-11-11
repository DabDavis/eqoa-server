#!/usr/bin/env python3
"""Mr. Moogie's last stand (2026-11-11; the GM Jibrish's petition 1522: "Add a new ability to Mr. Moogie. If he reaches
2% hp he completely heals to full and doubles his damage if a non player character is hitting him"), on the server's
code of the same branch (entity/laststand.go: the "laststand" ability kind, and a ninth ability: an older server reads
this template as unreadable).

  classtemplates "mr moogie": his eight abilities as they are, and after them a last stand: once a fight, at 2% of his
    hit points or under while an NPC alive on his hate list is hitting him (a pet, a mercenary, a creature a trigger set
    on him), he heals to full and his damage is 200% (doubled) for the rest of that fight, and says
    "You bring beasts to fight your battles? The king of this jungle answers twice as hard!"

The row is found by its name and left alone, nothing written, unless its abilities are the eight the tools of
2026-11-10 wrote (fix_moogie_tunare_20261110.py's seven and fix_innoruuk_visuals_20261110.py's killsay); no Mr. Moogie
template: another shard, nothing done. Report only by default; --apply in one transaction, a gm_audit line, log
moogie_laststand_1111_log; --undo exact (a row changed since left and said). The templates are read at the server's
start or a GM panel template edit (reload_npcs): run it as the before.sh of the restart bringing this code.

    tools/moogie_laststand_20261111.py --db eqoa.db [--apply | --undo]
"""
import argparse
import os
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import fix_innoruuk_visuals_20261110 as fv  # noqa: E402  his eighth ability as it wrote it
import fix_moogie_tunare_20261110 as fm  # noqa: E402  his first seven, and the writer

LOG = "moogie_laststand_1111_log"
DETAIL = "Mr. Moogie's last stand (petition 1522, Jibrish, 2026-11-11): tools/moogie_laststand_20261111.py"
MOOGIE_NOW = fm.MOOGIE_NEW + [fv.KILLSAY]
LASTSTAND = {"kind": "laststand", "below": 2, "damage_pct": 200,
             "say": "You bring beasts to fight your battles? The king of this jungle answers twice as hard!"}


def plan(db):
    """(changes, lines, refusals, here): the template row to write."""
    moogie = fm.row_of(db, "classtemplates", {"name": fm.MOOGIE_T})
    if moogie is None:
        return [], [f"no class template {fm.MOOGIE_T!r} here: not his shard"], [], False
    if not fm.same_abilities(moogie["abilities"], MOOGIE_NOW):
        return [], [], [f"{fm.MOOGIE_T!r}'s abilities are not the eight of 2026-11-10's tools: {moogie['abilities']}"], True
    after = dict(moogie, abilities=fm.abilities_json(MOOGIE_NOW + [LASTSTAND]))
    lines = [f"classtemplates {fm.MOOGIE_T!r}: a last stand after his eight abilities: at {LASTSTAND['below']}% or under with a "
             f"non-player hitting him, healed whole and his damage {LASTSTAND['damage_pct']}% for that fight, once a fight; "
             f"\"{LASTSTAND['say']}\""]
    return [("classtemplates", moogie, after)], lines, [], True


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    fm.LOG, fm.DETAIL = LOG, DETAIL  # its log and its audit's words, through the raid gods' tool's writer
    fm.LOG_SCHEMA = fm.LOG_SCHEMA.replace("moogie_tunare_1110_log", LOG)
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
