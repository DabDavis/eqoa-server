#!/usr/bin/env python3
"""Add Quartermaster Giblet's marks stock to an installed Heirloom Thanksgiving event."""

import argparse
import json
import sqlite3
from pathlib import Path


PLAN = json.loads(Path(__file__).with_name("mark-rewards.json").read_text(encoding="utf-8"))
VIA = "tool install_mark_rewards.py"
DETAIL = "[basis:owner] Quartermaster Giblet's Thanksgiving marks rewards"


def as_json(value):
    return None if value is None else json.dumps(value, sort_keys=True)


def where(key):
    return " AND ".join(f'"{column}" = ?' for column in key)


def get(db, table, key):
    row = db.execute(f"SELECT * FROM {table} WHERE {where(key)}", tuple(key.values())).fetchone()
    return dict(row) if row is not None else None


def audit(db, action, table, key, before, after):
    return db.execute(
        """INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail)
           VALUES (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), 0, 'tool', ?, '', ?, ?, ?, ?, ?, ?)""",
        (VIA, action, table, as_json(key), as_json(before), as_json(after), DETAIL),
    ).lastrowid


def bind_event(db):
    installed = db.execute(
        """SELECT 1 FROM npcs n JOIN npcseason ns ON ns.name = lower(n.npc_name)
           JOIN gmseasons g ON g.name = ns.season JOIN quests q ON q.questid = 109910
           WHERE n.id = ? AND n.npc_name = ? AND n.world = 0 AND g.name = ?
           AND ns.source = 'thanksgiving-2026' AND q.source = ns.source""",
        (PLAN["spawn"], PLAN["shop"], PLAN["season"]),
    ).fetchone()
    if installed is None:
        raise ValueError("Expected the installed Heirloom Thanksgiving event and its Blackwater quartermaster")


def put(db, table, key, value):
    current = get(db, table, key)
    if current is not None:
        if any(current[column] != expected for column, expected in value.items()):
            raise ValueError(f"Existing {table} row differs: {key}")
        return None
    columns = ", ".join(f'"{column}"' for column in value)
    marks = ", ".join("?" for _ in value)
    cursor = db.execute(f"INSERT INTO {table} ({columns}) VALUES ({marks})", tuple(value.values()))
    if table == "tokenshop":
        key = {"id": cursor.lastrowid}
    return audit(db, "insert", table, key, None, get(db, table, key))


def install(db):
    receipts = []
    for number, reward in enumerate(PLAN["rewards"], 1):
        pattern = reward["patternid"]
        item_key = {"patternid": pattern}
        other = db.execute(
            "SELECT patternid FROM itempattern WHERE itemname = ? COLLATE NOCASE AND patternid <> ? LIMIT 1",
            (reward["name"], pattern),
        ).fetchone()
        if other is not None:
            raise ValueError(f"{reward['name']} already exists at {other[0]}")
        donor = get(db, "itempattern", {"patternid": reward["donor"]})
        effect = get(db, "itemeffect", {"patternid": reward["donor"]})
        if donor is None or donor["itemname"] != reward["donor_name"] or effect is None:
            raise ValueError(f"Missing donor for {reward['name']}")
        donor.update(patternid=pattern, itemname=reward["name"], itemdesc=reward["description"],
                     itemcost=0, equipslot=-1, attacktype=0, weaponproc=0, levelreq=1,
                     trade=0, rent=1, lore=0, maxstack=1, raceuse=32767, classuse=32767)
        receipts.append(put(db, "itempattern", item_key, donor))
        receipts.append(put(db, "itemeffect", item_key, dict(
            item_key, line=reward["line"], name=reward["name"], icon=effect["icon"],
            breaks="", wear=0, area=0, body=0, target_line="", charges=reward["charges"],
            cooldown=0, source=PLAN["source"],
        )))
        stock_key = dict(item_key, shop=PLAN["shop"])
        receipts.append(put(db, "tokenshop", stock_key, dict(
            stock_key, price=PLAN["price"], token=PLAN["token"], says=reward["says"],
            sort=number, source=PLAN["source"],
        )))
    return [receipt for receipt in receipts if receipt is not None]


def undo(db):
    receipts = []
    for reward in PLAN["rewards"]:
        item_key = {"patternid": reward["patternid"]}
        acquired = db.execute(
            """SELECT 1 FROM tokenshop_purchases WHERE patternid = ?
               UNION ALL SELECT 1 FROM charinventory WHERE patternid = ? LIMIT 1""",
            (reward["patternid"], reward["patternid"]),
        ).fetchone()
        if acquired:
            raise ValueError(f"{reward['name']} was acquired. Keep its item definition and withdraw the stock separately.")
        stock = get(db, "tokenshop", dict(item_key, shop=PLAN["shop"]))
        rows = [("itemeffect", item_key), ("itempattern", item_key)]
        if stock:
            rows.insert(0, ("tokenshop", {"id": stock["id"]}))
        for table, key in rows:
            current = get(db, table, key)
            if current is None:
                continue
            initial = db.execute(
                """SELECT after FROM gm_audit WHERE via = ? AND detail = ? AND action = 'insert' AND tbl = ? AND rowkey = ?
                   ORDER BY id DESC LIMIT 1""",
                (VIA, DETAIL, table, as_json(key)),
            ).fetchone()
            if initial is None or json.loads(initial[0]) != current:
                raise ValueError(f"Cannot undo {table} {key}: the installed row differs or lacks its audit entry")
            db.execute(f"DELETE FROM {table} WHERE {where(key)}", tuple(key.values()))
            receipts.append(audit(db, "delete", table, key, current, None))
    return receipts


def run(db, apply=False, revert=False):
    db.row_factory = sqlite3.Row
    db.execute("PRAGMA foreign_keys=ON")
    db.execute("BEGIN IMMEDIATE")
    try:
        bind_event(db)
        receipts = undo(db) if revert else install(db)
        if apply or revert:
            db.commit()
        else:
            db.rollback()
        return receipts
    except (ValueError, sqlite3.Error):
        db.rollback()
        raise


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--db", required=True, type=Path)
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--apply", action="store_true")
    mode.add_argument("--undo", action="store_true")
    args = parser.parse_args()
    try:
        db = sqlite3.connect(args.db.resolve().as_uri() + "?mode=rw", uri=True)
        try:
            receipts = run(db, apply=args.apply, revert=args.undo)
        finally:
            db.close()
    except (ValueError, sqlite3.Error) as error:
        parser.exit(1, f"{error}\n")
    if args.apply or args.undo:
        print(f"{'Removed' if args.undo else 'Installed'} {len(receipts)} rows. Audit IDs: {receipts}")
    else:
        print(f"Dry run rolled back. {len(receipts)} rows to install.")
