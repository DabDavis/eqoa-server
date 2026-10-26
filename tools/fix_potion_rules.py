#!/usr/bin/env python3
"""The potions' rules of jib's second round (F3 20261026), on a database
tools/seed_potions.py has already filled:

- 20261026-014336 ("unstable mixture is a consumable that equips in the range
  attack slot, like a bow, and fires as a range auto attack. Currently its a
  useable potion"): a thrown potion (the Mixtures', the cocktails': an item
  whose itemeffect line is thrown, "Target, ...", and whose attack type is 10)
  goes from wear 17 (Thrown) to 16, a bow's slot (as the rusty throwing knife
  has it), so wearing it takes the hands off as a bow does and the auto-attack
  is the ranged swing (CLIENT's GetSwing: a ranged item with a weapon in hand
  swings melee), and from the client's item type 2 (procanim: a potion, which
  the client puts on quick slots 0-1) to 0, so it is no potion on the client's
  quick slots. Potion quick slots (pos 0-1) holding one are emptied. The
  server wears it at every Use from then on (handlers/potion.go usesPotion).
- 20261026-014527 ("Crude sublimation, and crude quickening both need a brief
  item cast time, about 1 minute"; the user: "he means duration"): the crude
  potions' effects last a minute. The itemeffect lines named Crude Quickening
  and Crude Sublimation (their effect names, as seed_potions writes them) have
  their time made "1min"; Quickening, Sublimation and Volatile Sublimation keep
  theirs.

Every change is logged in potion_rules_log and --undo puts each back as it
was. Both take a copy of the database first (<db>.bak-<when>-pre-potionrules).
Report only by default. The server reads an item's use at each use but which
items are thrown at its start, and a client takes an item's wear slot and type
at its login: restart the server after --apply or --undo.

    tools/fix_potion_rules.py [--db eqoa.db] [--apply | --undo]
"""
import argparse
import datetime
import os
import re
import sqlite3

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")

WEAR_THROWN, WEAR_BOW = 17, 16
TYPE_POTION, TYPE_NONE = 2, 0
CRUDE = ("crude quickening", "crude sublimation")
CRUDE_TIME = "1min"

LOG_SCHEMA = """CREATE TABLE IF NOT EXISTS potion_rules_log (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,
    rowkey  TEXT NOT NULL,
    col     TEXT NOT NULL,
    before  TEXT,
    after   TEXT,
    applied TEXT NOT NULL
)"""

# a line's time: "30min", "4min", "18sec", "2hr" (the last one counts)
RE_TIME = re.compile(r"(\d+)\s*(sec|min|hr)s?\b", re.I)


def with_minute(line):
    """The line with its time made a minute (added when it has none)."""
    found = list(RE_TIME.finditer(line))
    if not found:
        return line.rstrip(" ,") + ", " + CRUDE_TIME
    m = found[-1]
    return line[:m.start()] + CRUDE_TIME + line[m.end():]


def thrown_items(db):
    """The thrown potions as itempattern has them: (patternid, name, equipslot, procanim)."""
    return db.execute(
        "SELECT p.patternid, p.itemname, p.equipslot, p.procanim FROM itempattern p "
        "JOIN itemeffect e ON e.patternid = p.patternid "
        "WHERE lower(trim(e.line)) LIKE 'target%' AND p.attacktype = 10 ORDER BY p.patternid").fetchall()


def plan(db):
    """Every change to make: (table, key, column, before, after, label)."""
    out = []
    thrown = thrown_items(db)
    for pid, name, wear, kind in thrown:
        if wear == WEAR_THROWN:
            out.append(("itempattern", str(pid), "equipslot", wear, WEAR_BOW, f"{name}: worn as a bow ({wear} -> {WEAR_BOW})"))
        if kind == TYPE_POTION:
            out.append(("itempattern", str(pid), "procanim", kind, TYPE_NONE, f"{name}: no potion on the quick slots (type {kind} -> {TYPE_NONE})"))
    ids = [pid for pid, *_ in thrown]
    if ids and has_table(db, "quickslots"):
        marks = ",".join("?" * len(ids))
        for sid, pos, item, name in db.execute(
                f"SELECT q.serverid, q.pos, q.itemid, p.itemname FROM quickslots q "
                f"JOIN charinventory c ON c.itemid = q.itemid JOIN itempattern p ON p.patternid = c.patternid "
                f"WHERE q.pos IN (0, 1) AND c.patternid IN ({marks}) ORDER BY q.serverid, q.pos", ids):
            out.append(("quickslots", f"{sid},{pos}", "itemid", item, None, f"character {sid}'s potion slot {pos}: {name} taken off"))
    for pid, line, ename in db.execute("SELECT patternid, line, name FROM itemeffect ORDER BY patternid"):
        if (ename or "").strip().lower() in CRUDE:
            new = with_minute(line)
            if new != line:
                out.append(("itemeffect", str(pid), "line", line, new, f"{ename}: {line!r} -> {new!r}"))
    return out


def has_table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-potionrules"
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def apply(db, changes):
    now = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute(LOG_SCHEMA)
    for tbl, key, col, before, after, _ in changes:
        if tbl == "itempattern":
            db.execute(f"UPDATE itempattern SET {col} = ? WHERE patternid = ?", (after, int(key)))
        elif tbl == "itemeffect":
            db.execute("UPDATE itemeffect SET line = ? WHERE patternid = ?", (after, int(key)))
        elif tbl == "quickslots":
            sid, pos = (int(v) for v in key.split(","))
            db.execute("DELETE FROM quickslots WHERE serverid = ? AND pos = ?", (sid, pos))
        db.execute("INSERT INTO potion_rules_log (tbl, rowkey, col, before, after, applied) VALUES (?, ?, ?, ?, ?, ?)",
                   (tbl, key, col, None if before is None else str(before), None if after is None else str(after), now))


def undo(db):
    if not has_table(db, "potion_rules_log"):
        return 0
    rows = db.execute("SELECT id, tbl, rowkey, col, before FROM potion_rules_log ORDER BY id DESC").fetchall()
    for _, tbl, key, col, before in rows:
        if tbl == "itempattern":
            db.execute(f"UPDATE itempattern SET {col} = ? WHERE patternid = ?", (int(before), int(key)))
        elif tbl == "itemeffect":
            db.execute("UPDATE itemeffect SET line = ? WHERE patternid = ?", (before, int(key)))
        elif tbl == "quickslots":
            sid, pos = (int(v) for v in key.split(","))
            db.execute("INSERT OR IGNORE INTO quickslots (serverid, pos, itemid) VALUES (?, ?, ?)", (sid, pos, int(before)))
    db.execute("DROP TABLE potion_rules_log")
    return len(rows)


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="make the changes (logged in potion_rules_log)")
    g.add_argument("--undo", action="store_true", help="put every logged change back")
    args = ap.parse_args()
    db = sqlite3.connect(args.db)
    if args.undo:
        print("backup:", backup(db, args.db))
        with db:
            n = undo(db)
        print(f"put back {n} changes; restart the server")
        return
    changes = plan(db)
    for *_, label in changes:
        print("  " + label)
    print(f"{len(changes)} changes")
    if not args.apply:
        print("report only: --apply to make them")
        return
    if not changes:
        return
    print("backup:", backup(db, args.db))
    with db:
        apply(db, changes)
    print(f"made {len(changes)} changes; restart the server")


if __name__ == "__main__":
    main()
