#!/usr/bin/env python3
"""Install the GM-grant-only Scroll of Returning (Test) on a Heirloom database.

Run after deploying the code that recognizes item 716971. The command reports
and rolls back by default. Use --apply to install and --undo to remove an
unmodified row when no character, buyback, or configured source references it.
"""
import argparse
import json
from pathlib import Path
import sqlite3

from scroll_of_returning_20261126 import Writer, has, row


ORIGINAL_ID = 716970
TEST_ID = 716971
NAME = "Scroll of Returning (Test)"
DESCRIPTION = (
    "GM-issued test scroll. Read it to mark your place and return home. "
    "Read again within 30 minutes to return to that spot and spend one scroll. "
    "Wait 5 seconds between uses."
)
LOG = "test_scroll_of_returning_log"
DETAIL = "GM-issued Scroll of Returning (Test): tools/gm_return_scroll_20261127.py"
REFERENCES = {
    "npcitem": "patternid",
    "loottables": "patternid",
    "lootpool": "patternid",
    "startitems": "patternid",
    "itemeffect": "patternid",
    "craftrecipes": "product",
    "charinventory": "patternid",
    "charbuyback": "patternid",
}


def check_references(db):
    for table, column in REFERENCES.items():
        if has(db, table) and db.execute(
            f"SELECT 1 FROM {table} WHERE {column} = ? LIMIT 1", (TEST_ID,)
        ).fetchone():
            raise ValueError(f"{table} references item {TEST_ID}")


def apply(db, writer):
    original = row(db, "SELECT * FROM itempattern WHERE patternid = ?", (ORIGINAL_ID,))
    if original is None or original["itemname"] != "Scroll of Returning" or original["equipslot"] != -1:
        raise ValueError(f"expected the original Scroll of Returning at item {ORIGINAL_ID}")
    if db.execute(
        "SELECT 1 FROM itempattern WHERE patternid = ? OR lower(trim(itemname)) = lower(?)", (TEST_ID, NAME)
    ).fetchone():
        raise ValueError(f"item {TEST_ID} or the name {NAME!r} already exists")
    check_references(db)
    clone = dict(original, patternid=TEST_ID, itemname=NAME, itemdesc=DESCRIPTION, trade=1, itemcost=0)
    columns = ", ".join('"' + column.replace('"', '""') + '"' for column in clone)
    placeholders = ", ".join("?" for _ in clone)
    db.execute(f"INSERT INTO itempattern ({columns}) VALUES ({placeholders})", tuple(clone.values()))
    db.execute(f"CREATE TABLE {LOG} (tbl TEXT NOT NULL, rowkey TEXT NOT NULL, before TEXT, after TEXT, audit INTEGER NOT NULL)")
    installed = row(db, "SELECT * FROM itempattern WHERE patternid = ?", (TEST_ID,))
    return writer.log("itempattern", {"patternid": TEST_ID}, None, installed)


def undo(db, writer):
    if not has(db, LOG):
        raise ValueError("no install record")
    check_references(db)
    saved = db.execute(f"SELECT after FROM {LOG}").fetchone()
    current = row(db, "SELECT * FROM itempattern WHERE patternid = ?", (TEST_ID,))
    if not saved or current != json.loads(saved[0]):
        raise ValueError(f"item {TEST_ID} changed since installation")
    db.execute("DELETE FROM itempattern WHERE patternid = ?", (TEST_ID,))
    audit = writer.log("itempattern", {"patternid": TEST_ID}, current, None, action="undo")
    db.execute(f"DROP TABLE {LOG}")
    return audit


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--db", required=True, help="the Heirloom database")
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--apply", action="store_true")
    mode.add_argument("--undo", action="store_true")
    args = parser.parse_args()
    db = sqlite3.connect(Path(args.db).resolve().as_uri() + "?mode=rw", uri=True, timeout=5, isolation_level=None)
    try:
        db.execute("BEGIN IMMEDIATE")
        writer = Writer(db, log_table=LOG, detail=DETAIL)
        audit = undo(db, writer) if args.undo else apply(db, writer)
        if args.apply or args.undo:
            db.commit()
            print(f"{'Removed' if args.undo else 'Installed'} {NAME} ({TEST_ID}), audit {audit}")
        else:
            db.rollback()
            print(f"Dry run passed for {NAME} ({TEST_ID})")
    except (ValueError, sqlite3.Error) as error:
        db.rollback()
        parser.exit(1, f"Refused: {error}\n")
    finally:
        db.close()


if __name__ == "__main__":
    main()
