#!/usr/bin/env python3
"""Mr. Moogie strikes those he cannot reach (the GM Jibwiz's petition 1916, 2026-11-13: "Give mr moogie an ability that if
someone is on his hate list for more than 60 seconds, and he has not been able to hit any target on his hate list for more
than 60 seconds, he nukes them unresistable for 20,000 every 6 seconds. Essentially if he's fighting 10 people, hitting
one, then this does not go off"), on the server's code of the same branch (entity/unreached.go: the "unreached" boss
moment; an older server reads this template as unreadable):

  classtemplates "mr moogie": his eleven abilities as they are (the tools of 2026-11-10/11 and moogie_gm_20261113.py),
    and after them an unreached moment: every 6 s of a fight (the first 6 s in), once 60 s have gone with no melee hit of
    his landed on anyone (a player, a pet, a mercenary, a creature set on him), 20,000 pure damage (no resist, armour or
    mitigation) on every player who has been on his hate list 60 s or more; one hit landed puts it off for 60 s again.
    Named "Royal Wrath" in its lines (ours: "<Mr. Moogie>'s Royal Wrath hits you for 20000.").

The template is found by its name and left alone, nothing written, unless its abilities are the eleven (moogie_gm's
nine and its ward and hold); no Mr. Moogie template: another shard (Live has none), nothing done. Report only by
default; --apply in one transaction, a gm_audit line, log moogie_unreached_1113_log; --undo exact (a row changed since
left and said). Twelve abilities is a template's most (database.MaxTemplateAbilities): this is his last free one.

WHEN IT TAKES EFFECT: templates are read at the server's start or a GM panel template edit: run it as the before.sh of
the keep restart bringing this code, never under an older server.

    tools/moogie_unreached_20261113.py --db eqoa.db [--apply | --undo]
"""
import argparse
import os
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import fix_moogie_tunare_20261110 as fm  # noqa: E402  the writer
import moogie_gm_20261113 as mg  # noqa: E402  his eleven abilities as they stand

LOG = "moogie_unreached_1113_log"
DETAIL = "Mr. Moogie's Royal Wrath at those he cannot reach (petition 1916, Jibwiz, 2026-11-13): tools/moogie_unreached_20261113.py"
MOOGIE_ELEVEN = mg.MOOGIE_NINE + [mg.WARD, mg.HOLD]
UNREACHED = {"kind": "unreached", "every": 6, "first": 6, "damage": 20000, "unhit": 60, "name": "Royal Wrath"}


def plan(db):
    """(changes, lines, refusals, here)."""
    moogie = fm.row_of(db, "classtemplates", {"name": fm.MOOGIE_T})
    if moogie is None:
        return [], [f"no class template {fm.MOOGIE_T!r} here: not his shard"], [], False
    if not fm.same_abilities(moogie["abilities"], MOOGIE_ELEVEN):
        return [], [], [f"{fm.MOOGIE_T!r}'s abilities are not the eleven of the tools up to moogie_gm_20261113.py: "
                         f"{moogie['abilities']}"], True
    after = dict(moogie, abilities=fm.abilities_json(MOOGIE_ELEVEN + [UNREACHED]))
    lines = [f"classtemplates {fm.MOOGIE_T!r}: after his eleven abilities, 1916's unreached moment: every {UNREACHED['every']} s, "
             f"no hit landed for {UNREACHED['unhit']} s, {UNREACHED['damage']} pure on each player {UNREACHED['unhit']} s or "
             f"more on his hate list (\"{UNREACHED['name']}\")"]
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
