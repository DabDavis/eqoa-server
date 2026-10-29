#!/usr/bin/env python3
"""The bows', crossbows' and thrown weapons' range (the user, 2026-10-29: "im
on funyun and cant throw throwing knife").

An item's range is the item record's field after its weapon damage
(itempattern.unk5), which CLIENT's TargetInRange (0x00BDC4B8) and the
server's reach (entity/attack.go SwingFor) read for attack types 7 (bow), 9
(crossbow) and 10 (thrown): the swing reaches that far, and never with 0.
Every one of this server's (60 bows, 10 crossbows, 2 throwing knives) had 0,
so none could be fired; only the thrown potions, which tools/seed_potions.py
gave 20, could.

Sandstorm's own records give each one (every `eqoa harvest` JSON with
merchants' wares: --prices, as tools/stock_merchants.py reads them): every
thrown weapon and crossbow 25, every bow 25 but the crafted and weak
composite longbows, 30. And they show more of its ranged weapons wrong here:
six of its bows and crossbows were no ranged weapon at all (slot 12, attack
type 0: Simple Light, Medium and War Crossbow, Crafted Shortbow and
Composite Longbow, Weak Longbow), the crossbows' damage below Sandstorm's,
the throwing knives stacking to 99 and 1 where Sandstorm's stack to 2000, the
steel one worn in slot 17.

So: a ranged weapon Sandstorm sells, by its name, takes Sandstorm's record
(slot, stack, attack type, damage, level and range) where ours differs; an
item with a potion's row (itemeffect: tools/seed_potions.py's, ours by
design) is left as it is. Any other item of a ranged attack type with no
range takes 25 (OURS: Sandstorm's every thrown weapon's and crossbow's, and
155 of its 184 bows'). A range already set is never changed.

Every change is logged in ranged_reach_fix_log (the pattern, its values before
and after, why); --undo puts back each one still as this left it and drops
the log. Report only by default. The server reads the record at each swing;
a client reads it with its item records, at its next login.

    tools/fix_ranged_reach.py [--db eqoa.db] [--prices <harvest JSON or folder> ...] [--apply | --undo]
"""
import argparse
import datetime
import glob
import json
import os
import sqlite3
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
RANGED = (7, 9, 10)
DEFAULT_RANGE = 25

# Sandstorm's record's fields, as itempattern names them
FIELDS = {"wear_slot": "equipslot", "max_stack": "maxstack", "attack_type": "attacktype", "damage": "weapondamage",
          "level_req": "levelreq", "range": "unk5"}
LOG_SCHEMA = """CREATE TABLE ranged_reach_fix_log (
    patternid INTEGER PRIMARY KEY,
    before    TEXT NOT NULL,   -- the changed columns before, JSON
    after     TEXT NOT NULL,   -- and after
    why       TEXT NOT NULL,
    applied   TEXT NOT NULL
)"""


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def key(name):
    return " ".join((name or "").replace("`", "'").lower().split())


def measured(paths):
    """Sandstorm's ranged weapons' records by item name: {name: {column:
    value}}, and the names it gives two records (none so far)."""
    files = []
    for p in paths:
        files += glob.glob(os.path.join(p, "**", "*.json"), recursive=True) if os.path.isdir(p) else [p]
    seen = {}

    def walk(o):
        if isinstance(o, dict):
            if o.get("attack_type") in RANGED and isinstance(o.get("range"), int) and o.get("name"):
                yield o
            for v in o.values():
                yield from walk(v)
        elif isinstance(o, list):
            for v in o:
                yield from walk(v)

    for f in files:
        try:
            with open(f) as fh:
                d = json.load(fh)
        except (OSError, ValueError):
            continue
        for w in walk(d):
            rec = tuple((col, w.get(f)) for f, col in FIELDS.items())
            seen.setdefault(key(w["name"]), set()).add(rec)
    two = {k for k, v in seen.items() if len(v) > 1}
    return {k: dict(next(iter(v))) for k, v in seen.items() if k not in two}, two


def plan(db, records):
    """[(patternid, name, {column: before}, {column: after}, why)]."""
    potions = set()
    if has_table(db, "itemeffect"):
        potions = {p for (p,) in db.execute("SELECT patternid FROM itemeffect")}
    cols = ", ".join(FIELDS.values())
    out = []
    for row in db.execute(f"SELECT patternid, itemname, {cols} FROM itempattern ORDER BY patternid").fetchall():
        pid, name, now = row[0], row[1], dict(zip(FIELDS.values(), row[2:]))
        if pid in potions:
            continue
        want = records.get(key(name))
        if want:
            diff = {c: v for c, v in want.items() if v is not None and now[c] != v and not (c == "unk5" and now[c] != 0)}
            if diff:
                out.append((pid, name, {c: now[c] for c in diff}, diff, "Sandstorm's record"))
        elif now["attacktype"] in RANGED and now["unk5"] == 0:
            out.append((pid, name, {"unk5": 0}, {"unk5": DEFAULT_RANGE},
                        "ours: Sandstorm's every thrown weapon's and crossbow's range, most bows'"))
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--prices", nargs="*", default=[], help="eqoa harvest JSON files or folders of them")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the ranges (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply changed")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        if not has_table(db, "ranged_reach_fix_log"):
            print("nothing logged: nothing to undo")
            return
        db.execute("BEGIN IMMEDIATE")
        back = left = 0
        for pid, before, after in db.execute("SELECT patternid, before, after FROM ranged_reach_fix_log").fetchall():
            before, after = json.loads(before), json.loads(after)
            cols = list(after)
            now = db.execute(f"SELECT {', '.join(cols)} FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
            if now is None or dict(zip(cols, now)) != after:
                left += 1
                continue
            db.execute(f"UPDATE itempattern SET {', '.join(c + ' = ?' for c in cols)} WHERE patternid = ?",
                       [before[c] for c in cols] + [pid])
            back += 1
        db.execute("DROP TABLE ranged_reach_fix_log")
        db.execute("COMMIT")
        print(f"put back {back} ranges, {left} changed since left as they are")
        return
    if a.apply and has_table(db, "ranged_reach_fix_log"):
        sys.exit("ranged_reach_fix_log is there: applied already (--undo first)")
    records, two = measured(a.prices)
    changes = plan(db, records)
    for pid, name, before, after, why in changes:
        print(f"{pid:7d} {name}: " + ", ".join(f"{c} {before[c]} -> {after[c]}" for c in after) + f" ({why})")
    if two:
        print(f"Sandstorm gives two records to: {', '.join(sorted(two))} (left to the range rule)")
    print(f"{len(changes)} items to change ({sum(c[4] == 'Sandstorm' + chr(39) + 's record' for c in changes)} by "
          f"Sandstorm's record); {len(records)} of its ranged weapons' records read")
    if not a.apply or not changes:
        if not a.apply:
            print("report only: --apply to write")
        return
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_SCHEMA)
        for pid, name, before, after, why in changes:
            db.execute("INSERT INTO ranged_reach_fix_log VALUES (?, ?, ?, ?, ?)",
                       (pid, json.dumps(before, sort_keys=True), json.dumps(after, sort_keys=True), why, when))
            cols = list(after)
            db.execute(f"UPDATE itempattern SET {', '.join(c + ' = ?' for c in cols)} WHERE patternid = ? AND "
                       + " AND ".join(f"{c} = ?" for c in cols),
                       [after[c] for c in cols] + [pid] + [before[c] for c in cols])
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"changed {len(changes)} items; log out and back in to see them")


if __name__ == "__main__":
    main()
