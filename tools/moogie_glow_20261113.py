#!/usr/bin/env python3
"""Mr. Moogie's soft red glow (2026-11-13; the GM Jibrish's petition 1893: "Use the new glow mechanic to make mr. moogies
whole body glow a soft red"), on the server's code of the same branch (database/npctint.go's glow column, sent in
OpcodeNPCTints layout 2: entity/npctint.go).

  npctint "Mr. Moogie": color 0xFFFFFFFF (no tint: his body's own colours) and glow 0xFF606099, a soft red (255, 96, 96)
    at 60%: the held items' glow (vanilla's aura.rs) laid on his whole body by a vanilla that reads the tints' layout 2.
    A vanilla before it (layout 1) and a PS2 draw him as ever.

The row is keyed by name (database.NPCClassKey: "mr. moogie"); one already there for him (a GM's or another's) is left
as it is and the tool refuses. A table without the glow column (a server older than this code has not yet started on it)
is given it first, as the server's own start would (ALTER TABLE npctint ADD COLUMN glow INTEGER NOT NULL DEFAULT 0:
--undo leaves the column, which an older server never reads). Report only by default; --apply in one transaction, a
gm_audit line, log moogie_glow_1113_log; --undo exact (a row changed since left and said).

WHEN IT TAKES EFFECT: the server reads npctint at its start, a GM panel edit of NPC body tints, or the panel's Reload
every NPC (reload_npcs): run it as the before.sh of the restart bringing this code, or after it and then Reload.

    tools/moogie_glow_20261113.py --db eqoa.db [--apply | --undo]
"""
import argparse
import os
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import fix_moogie_tunare_20261110 as fm  # noqa: E402  the writer

LOG = "moogie_glow_1113_log"
DETAIL = "Mr. Moogie's soft red glow (petition 1893, Jibrish, 2026-11-13): tools/moogie_glow_20261113.py"
NAME = "Mr. Moogie"
COLOR, GLOW = 0xFFFFFFFF, 0xFF606099


def has_glow(db):
    return any(r[1].lower() == "glow" for r in db.execute("PRAGMA table_info(npctint)"))


def plan(db):
    """(changes, lines, refusals, here)."""
    if not fm.table(db, "npctint"):
        return [], ["no table npctint here: a server older than the NPC tints"], [], False
    if db.execute("SELECT count(*) FROM npcs WHERE lower(trim(npc_name)) = lower(?)", (NAME,)).fetchone()[0] == 0:
        return [], [f"no NPC {NAME!r} here: not his shard"], [], False
    key = fm.npc_key(NAME)
    for (name,) in db.execute("SELECT name FROM npctint").fetchall():
        if fm.npc_key(name) == key:
            return [], [], [f"npctint has a row for him already ({name!r}): left as it is"], True
    row = {"name": NAME, "color": COLOR, "glow": GLOW, "note": "Petition 1893: his whole body glows a soft red (layout 2 clients).",
           "source": "moogie1113"}
    return [("npctint", None, row)], [f"npctint {NAME!r}: color {COLOR:08X} (no tint), glow {GLOW:08X} (a soft red at 60%)"], [], True


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    fm.LOG, fm.DETAIL = LOG, DETAIL
    fm.LOG_SCHEMA = fm.LOG_SCHEMA.replace("moogie_tunare_1110_log", LOG)
    fm.KEYS.update({"npctint": ("name",)})
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
        glow_col = has_glow(db)
        if not glow_col:
            print("npctint has no glow column: --apply adds it, as the server's start would")
        if not a.apply:
            print(f"report only: {len(changes)} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            if not glow_col:
                db.execute("ALTER TABLE npctint ADD COLUMN glow INTEGER NOT NULL DEFAULT 0")
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
