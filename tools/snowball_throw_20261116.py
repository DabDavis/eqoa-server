#!/usr/bin/env python3
"""Petition 2204 (L60 rogue, Heirloom, 2026-11-16): "Moogie snowballs showing as stackable trash item and
cant be equipped to throw at other players". The Metal Moogie chain's three used items (tools/
moogie_chain_688_20261115.py) copied the Moogie Tribute's record, item type (procanim) 0: with no wear slot and no
charge CLIENT's VIInfoText::IsMisc files such an item with the loot, and no menu offers Use (IsUsable is types 2, 5, 7
and 8), so CLIENT never sent one (no "Potion:" or "Snowball:" line of the snowball in the log). The two potions are
the same: never drinkable. This makes, by each record's name (the shards' ids differ):

  Snowball: procanim 2 (CLIENT's usable type: listed with Use, Prepare onto quick slot 1 or 2), equipslot 16 (the
    ranged slot, the throwing knives' wear), attacktype 10 (Thrown), unk5 30 (its reach, SnowballRange), model the
    Hill Giant's Throwing Stone's (a stone in the hand: the record had none, and no hand item of this server is drawn
    from model 0; left 0, and said, when no one record has that name). The code of the same branch throws it at
    another player from either list or a quick slot and wears it from the Equipment list (entity/moogieitems.go).
  Potion of Moogie's Insight, Deputy's Draught: procanim 2.

Each column is written only while it is still what the chain wrote (a GM's change since is left and said), each record
with its gm_audit line and its own line in snowball_throw_20261116_log. A shard without the records says so and
writes nothing. It is idempotent: a second run writes nothing.

WHEN IT TAKES EFFECT: item records are read at each use and sent at each login, so a player who holds them sees the
change at the next login. RUN IT WITH THE CODE (the before.sh of the swap_keep.sh restart that brings snowball-throw):
under an older server a snowball of wear 16 is a worn thrown potion only, never thrown by Use.

--dry (the default) prints what it would write; --apply writes it in one transaction; --undo puts back each column
still as this wrote it.

    tools/snowball_throw_20261116.py [--db ~/eqoa-server/eqoa.db] [--dry | --apply | --undo]
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

LOG = "snowball_throw_20261116_log"
WHO, VIA = "dab", "tool"
DETAIL = "petition 2204 (Hoofbite): snowballs thrown and worn, the Moogie potions usable, tools/snowball_throw_20261116.py"
DEFAULT_DB = os.path.expanduser("~/eqoa-server/eqoa.db")  # Heirloom's; Live's is another --db
STONE = "Hill Giant's Throwing Stone"
USABLE = 2  # CLIENT's item type of the potions and food (database.ItemTypeUsable)

# name: {column: (as the chain wrote it, as it is to be)}; "model" of the snowball is the stone's, read at the run
CHANGES = {
    "Snowball": {"procanim": (0, USABLE), "equipslot": (-1, 16), "attacktype": (0, 10), "unk5": (0, 30), "model": (0, None)},
    "Potion of Moogie's Insight": {"procanim": (0, USABLE)},
    "Deputy's Draught": {"procanim": (0, USABLE)},
}

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq       INTEGER PRIMARY KEY AUTOINCREMENT,
    patternid INTEGER NOT NULL,
    col       TEXT NOT NULL,
    before    INTEGER,
    after     INTEGER,
    audit     INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def rows(db, q, args=()):
    cur = db.execute(q, args)
    cols = [d[0] for d in cur.description]
    return [dict(zip(cols, r)) for r in cur.fetchall()]


def item_named(db, name):
    return rows(db, "SELECT * FROM itempattern WHERE lower(trim(itemname)) = ? ORDER BY patternid", (name.lower(),))


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def plan(db):
    """(steps, lines, refusals, here): a step is (patternid, name, {col: (before, after)})."""
    steps, lines, refusals = [], [], []
    found = {name: item_named(db, name) for name in CHANGES}
    if not any(found.values()):
        lines.append("no Snowball or Moogie potion records here: not a shard of the Metal Moogie chain; nothing to do")
        return [], lines, [], False
    stone = item_named(db, STONE)
    stone_model = stone[0]["model"] if len(stone) == 1 and stone[0]["model"] not in (0, -1) else None
    for name, cols in CHANGES.items():
        recs = found[name]
        if len(recs) != 1:
            refusals.append(f"{len(recs)} item records named {name!r}, want one")
            continue
        rec = recs[0]
        change = {}
        for col, (was, want) in cols.items():
            if col == "model" and want is None:
                if stone_model is None:
                    lines.append(f"{name} ({rec['patternid']}): no one {STONE!r} with a model; model left {rec['model']}")
                    continue
                want = stone_model
            if rec[col] == want:
                continue
            if rec[col] != was:
                lines.append(f"{name} ({rec['patternid']}): {col} is {rec[col]}, not the chain's {was}: LEFT (a GM's?)")
                continue
            change[col] = (was, want)
        if change:
            steps.append((rec["patternid"], name, change))
            lines.append(f"{name} ({rec['patternid']}): " + ", ".join(f"{c} {b} -> {a}" for c, (b, a) in change.items()))
        else:
            lines.append(f"{name} ({rec['patternid']}): as wanted already")
    return steps, lines, refusals, True


def audit(db, tbl, key, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', 'update', ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, tbl, go_json(key), go_json(before), go_json(after), DETAIL))
    return cur.lastrowid


def apply(db, steps):
    db.execute(LOG_SCHEMA)
    for pid, _, change in steps:
        sets = ", ".join(f"{c} = ?" for c in change)
        db.execute(f"UPDATE itempattern SET {sets} WHERE patternid = ?", [a for _, a in change.values()] + [pid])
        a = audit(db, "itempattern", {"patternid": pid}, {c: b for c, (b, _) in change.items()},
                  {c: x for c, (_, x) in change.items()})
        for c, (b, x) in change.items():
            db.execute(f"INSERT INTO {LOG} (patternid, col, before, after, audit) VALUES (?, ?, ?, ?, ?)", (pid, c, b, x, a))


def undo(db):
    if not table(db, LOG):
        return ["nothing logged: nothing to undo"]
    out = []
    by_item = {}
    for seq, pid, col, before, after in db.execute(f"SELECT seq, patternid, col, before, after FROM {LOG} ORDER BY seq DESC"):
        by_item.setdefault(pid, []).append((seq, col, before, after))
    for pid, logged in by_item.items():
        rec = rows(db, "SELECT * FROM itempattern WHERE patternid = ?", (pid,))
        back, done = {}, []
        for seq, col, before, after in logged:
            if not rec:
                out.append(f"item {pid}: gone, {col} not put back")
            elif rec[0][col] != after:
                out.append(f"item {pid} ({rec[0]['itemname']}): {col} is {rec[0][col]}, changed since: LEFT")
                continue
            else:
                back[col] = (after, before)
            done.append(seq)
        if back:
            sets = ", ".join(f"{c} = ?" for c in back)
            db.execute(f"UPDATE itempattern SET {sets} WHERE patternid = ?", [b for _, b in back.values()] + [pid])
            audit(db, "itempattern", {"patternid": pid}, {c: a for c, (a, _) in back.items()}, {c: b for c, (_, b) in back.items()})
            out.append(f"item {pid} ({rec[0]['itemname']}): " + ", ".join(f"{c} {a} -> {b}" for c, (a, b) in back.items()))
        for seq in done:
            db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB, help=f"the server's database (default Heirloom's {DEFAULT_DB})")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--dry", action="store_true", help="print what it would write (the default)")
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    args = ap.parse_args()
    if not os.path.exists(args.db):
        sys.exit(f"no database at {args.db}")
    db = sqlite3.connect(args.db, timeout=5)
    if args.undo:
        with db:
            for line in undo(db):
                print(line)
        return
    steps, lines, refusals, here = plan(db)
    for line in lines:
        print(line)
    if refusals:
        for r in refusals:
            print("REFUSED:", r)
        sys.exit(2)
    if not here or not steps:
        print("nothing to write")
        return
    if not args.apply:
        print(f"--dry: {len(steps)} records would be changed; --apply writes them")
        return
    with db:
        apply(db, steps)
    print(f"applied: {len(steps)} records, logged in {LOG}")


if __name__ == "__main__":
    main()
