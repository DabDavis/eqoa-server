#!/usr/bin/env python3
"""Install the GM-grantable Glorious Experience Test potion in the Live database.

Run with the matching server build. Pass the database path to preview, or add
--apply to insert the item and its effect in one audited transaction.
"""

import argparse
import json
import re
import sqlite3
from pathlib import Path


NAME = "Glorious Experience Test"
TEMPLATE = "Potion of Moogie's Insight"
ITEM = {
    "itemname": NAME,
    "itemdesc": "Use on yourself or another player for 15x normal experience for one hour of play. Another dose refreshes the hour.",
    "itemcost": 0, "equipslot": -1, "attacktype": 0, "levelreq": 1,
    "trade": 1, "lore": 0, "maxstack": 99, "classuse": 32767, "raceuse": 32767,
}
EFFECT = {
    "line": "Target, Glorious Experience, 1hr", "name": NAME, "source": "gm-glorious-experience",
    "breaks": "", "wear": 0, "area": 0, "body": 0, "target_line": "", "cooldown": 0,
    "charges": 0,  # The item handler consumes one unit from the stack.
}


def named_item(db, name):
    rows = db.execute("SELECT * FROM itempattern WHERE itemname = ? COLLATE NOCASE", (name,)).fetchall()
    if len(rows) > 1:
        raise ValueError(f"multiple item records named {name!r}")
    return dict(rows[0]) if rows else None


def verify_sources(db, pattern_id):
    for table in ("npcitem", "loottables", "lootpool", "heroes_stock"):
        if db.execute(f"SELECT 1 FROM {table} WHERE patternid = ? LIMIT 1", (pattern_id,)).fetchone():
            raise ValueError(f"item {pattern_id} has an existing source in {table}")
    item_token = re.compile(rf"(?:^|,)\s*0*{pattern_id}\s*(?:[*x×]|,|$)")
    for row in db.execute("SELECT items FROM queststeps WHERE kind = 'loot' AND items LIKE ?", (f"%{pattern_id}%",)):
        if item_token.search(row[0]):
            raise ValueError(f"item {pattern_id} is already a quest drop")


def insert_audited(db, table, values):
    columns = ", ".join(f'"{key}"' for key in values)
    placeholders = ", ".join("?" for _ in values)
    cursor = db.execute(f"INSERT INTO {table} ({columns}) VALUES ({placeholders})", tuple(values.values()))
    pattern_id = values.get("patternid", cursor.lastrowid)
    row = db.execute(f"SELECT * FROM {table} WHERE patternid = ?", (pattern_id,)).fetchone()
    db.execute(
        """INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail)
           VALUES (strftime('%Y-%m-%dT%H:%M:%SZ','now'), 0, 'tool', 'tool glorious_experience.py', '',
                   'insert', ?, ?, NULL, ?, ?)""",
        (table, json.dumps({"patternid": pattern_id}), json.dumps(dict(row)), NAME),
    )
    return pattern_id


def install(db, apply):
    db.row_factory = sqlite3.Row
    with db:
        db.execute("BEGIN IMMEDIATE" if apply else "BEGIN")
        template = named_item(db, TEMPLATE)
        if template is None:
            raise ValueError(f"missing source potion {TEMPLATE!r}")
        icon = db.execute("SELECT icon FROM itemeffect WHERE patternid = ?", (template["patternid"],)).fetchone()
        if icon is None:
            raise ValueError("source potion has no effect icon")
        effect = dict(EFFECT, icon=icon[0])
        existing = named_item(db, NAME)
        if existing:
            pattern_id = existing["patternid"]
            current = db.execute("SELECT * FROM itemeffect WHERE patternid = ?", (pattern_id,)).fetchone()
            if (any(existing[k] != v for k, v in ITEM.items()) or current is None
                    or any(current[k] != v for k, v in effect.items())):
                raise ValueError(f"item {pattern_id} already has this name with a different definition")
            verify_sources(db, pattern_id)
            return f"Already installed: {NAME} ({pattern_id})"
        if not apply:
            return f"Would clone {TEMPLATE} and add the targeted XP effect for {NAME}."
        del template["patternid"]
        pattern_id = insert_audited(db, "itempattern", {**template, **ITEM})
        verify_sources(db, pattern_id)
        insert_audited(db, "itemeffect", dict(effect, patternid=pattern_id))
        return f"Installed {NAME} ({pattern_id}). GM grant: #giveitem {pattern_id} <player> [count]"


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("database", type=Path)
    parser.add_argument("--apply", action="store_true")
    args = parser.parse_args()
    mode = "rw" if args.apply else "ro"
    try:
        db = sqlite3.connect(f"{args.database.resolve().as_uri()}?mode={mode}", uri=True)
        try:
            print(install(db, args.apply))
        finally:
            db.close()
    except (ValueError, sqlite3.Error) as error:
        parser.exit(1, f"{error}\n")
