#!/usr/bin/env python3
"""Quest stacks without Lore (the owner, 2026-11-21: "remove lore from quest stacks like madmen blood so players can xp
without being slowed down").

A quest item is an item a quest step names to be had or handed in: a loot, give or show step's items (queststeps.items,
"4866*2, 8373"; any quest, enabled or not). A reward alone is not one (rewarditems, choices). Of those, a STACK
(itempattern.maxstack > 1) that is Lore (lore != 0) and is not gear (equipslot < 1) has its lore set to 0, so a player
may hold and loot more than one. Left as they are: gear, a single item that does not stack, and a key an event reads
in the pack (handlers/raidportal.go's Tallon's Key, whose one bearer closes the portal behind a raid; skypass.go's
isles' keys), Lore on purpose.

The rest of the owner's ask is the server's (handlers/questpreloot.go: a quest's item drops for anyone, once a corpse;
the old "holds as many as the step wants" stop is gone): "blood of the desert madman" (8341) is lore 0 and maxstack 1
on both shards, so this tool does not touch it; what held it back was that code.

Report only by default; --apply writes, a gm_audit line a row, the old values kept in quest_items_unlore_20261121_log;
--undo puts back each row still as this tool left it. A second --apply finds nothing to do.

WHEN IT TAKES EFFECT: an item's record is read when it is next sent or looted; a pack's items already held carry the
record the client was sent at login (a relog shows the new one).

    tools/quest_items_unlore_20261121.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import re
import sqlite3
import sys

LOG = "quest_items_unlore_20261121_log"
DETAIL = "the owner 2026-11-21: quest stacks without lore: tools/quest_items_unlore_20261121.py"
ACCOUNT, WHO, VIA = 1, "dab", "tool"
KINDS = ("loot", "give", "show")
# keys an event reads in the pack (handlers/raidportal.go raidPortals, handlers/skypass.go skyPasses): Lore on purpose
EVENT_KEYS = {k.lower() for k in (
    "Tallon's Key", "key of the ancients", "rain key", "sun key", "snow key",
    "Day Key 1", "Day Key 2", "Day Key 3", "Day Key 4",
    "Desolation Key 1", "Desolation Key 2", "Desolation Key 3", "Desolation Key 4",
    "Night Key 1", "Night Key 2", "Night Key 3", "Night Key 4", "Night Key 5",
    "Rain Key 1", "Rain Key 2", "Rain Key 3", "Rain Key 4", "Rain Key 5",
    "Snow Key 2", "Snow Key 3", "Snow Key 4")}


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def audit(db, pid, before, after, detail):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', 'itempattern', ?, ?, ?, ?)",
               (at, ACCOUNT, WHO, VIA, go_json({"patternid": pid}), go_json(before), go_json(after), detail))


def item_ids(s):
    """handlers' database.ParseItemCounts: "4866*2, 8373" (x and × too), the pattern ids."""
    out = []
    for f in (s or "").split(","):
        f = f.strip()
        if not f:
            continue
        m = re.match(r"^\s*(\d+)\s*(?:[*x×]\s*(\d+))?\s*$", f)
        if m and (m.group(2) is None or int(m.group(2)) >= 1):
            out.append(int(m.group(1)))
    return out


def quest_items(db):
    """Every item a loot, give or show step names, with the quests that name it."""
    named = {}
    q = "SELECT questid, items FROM queststeps WHERE kind IN (%s) AND items != ''" % ",".join("?" * len(KINDS))
    for qid, items in db.execute(q, KINDS):
        for pid in item_ids(items):
            named.setdefault(pid, set()).add(qid)
    return named


def candidates(db):
    named = quest_items(db)
    out = []
    for pid in sorted(named):
        r = db.execute("SELECT itemname, lore, maxstack, equipslot FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
        if r is None:
            continue
        name, lore, maxstack, equipslot = r
        if lore != 0 and maxstack > 1 and equipslot < 1 and name.strip().lower() not in EVENT_KEYS:
            out.append((pid, name, lore, maxstack, len(named[pid])))
    return out, named


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            if not table(db, LOG):
                db.execute("ROLLBACK")
                print("nothing to undo")
                return 0
            back, skipped = 0, 0
            for pid, lore in db.execute(f"SELECT patternid, lore FROM {LOG} ORDER BY patternid").fetchall():
                r = db.execute("SELECT itemname, lore FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
                if r is None or r[1] != 0:
                    print(f"  {pid} {r and r[0]!r}: changed since (lore {r and r[1]}), left as it is")
                    skipped += 1
                    continue
                db.execute("UPDATE itempattern SET lore = ? WHERE patternid = ?", (lore, pid))
                audit(db, pid, {"lore": 0}, {"lore": lore}, "undo: " + DETAIL)
                print(f"  {pid} {r[0]!r}: lore {lore} again")
                back += 1
            db.execute(f"DROP TABLE {LOG}")
            db.execute("COMMIT")
            print(f"put back {back}, {skipped} changed since and left")
            return 0
        rows, named = candidates(db)
        print(f"{len(named)} items named by quest steps ({', '.join(KINDS)}); {len(rows)} Lore stacks, not gear, not an event's key:")
        for pid, name, lore, maxstack, nq in rows:
            print(f"  {pid:>7} {name!r}: lore {lore} -> 0 (stacks to {maxstack}; {nq} quest(s))")
        if not rows:
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if not a.apply:
            db.execute("ROLLBACK")
            print("report only (--apply to write)")
            return 0
        db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (patternid INTEGER PRIMARY KEY, lore INTEGER NOT NULL)")
        for pid, name, lore, maxstack, nq in rows:
            db.execute("UPDATE itempattern SET lore = 0 WHERE patternid = ?", (pid,))
            db.execute(f"INSERT OR IGNORE INTO {LOG} (patternid, lore) VALUES (?, ?)", (pid, lore))
            audit(db, pid, {"lore": lore}, {"lore": 0}, DETAIL)
        db.execute("COMMIT")
        print(f"applied: {len(rows)} rows")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
