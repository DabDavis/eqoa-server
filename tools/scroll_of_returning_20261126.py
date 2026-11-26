#!/usr/bin/env python3
"""The Scroll of Returning, made and sold (Heirloom petition 3199: "Heroic Dungeon scrolls: scrolls that return you to
town, and back to your exact position in the dungeon before consumed on use").

This:

  - makes the scroll's record (itempattern), a clone of a spell scroll's (TEMPLATE: its icon, the scroll's look, its
    use animation, its duration and rent): named "Scroll of Returning" (handlers/dungeonreturn.go knows it by that
    name, worn nowhere), its own words, every class and race, level 1, tradeable, not LORE, stacking to MAX_STACK,
    valued PRICE (a merchant sells at the value: handlers/merchant.go), at the first free id from FROM_ID;
  - stocks it in the shop of Freeport's Spell Archivist (the one in world 0, beside the Heroic Dungeons board and
    the Announcements board), last in his list. His list is the buyer's class's alone (buyback.go's classShop):
    every class may use the scroll, so every class sees it.

What the scroll does is the server's code (entity/dungeonreturn.go): read in a heroic dungeon's instance it marks the
place and sends its reader home; read again within 30 minutes, the instance still open and the reader still of its
party (and not locked out), it sends it back to the mark and is spent. A Hardcore character, and a session the
quality-of-life additions are off for (world.vanilla_ui, an [OG] character), are refused: run this on the Heirloom
Test Server's database. Without that code the scroll is a spell scroll naming no spell.

Each row written is logged in scroll_of_returning_1126_log and gets its own gm_audit line (tbl, rowkey, before,
after), so the GM panel shows each. Refused, nothing written, unless the Spell Archivist is one NPC in world 0, the
template is one record, and no record is named "Scroll of Returning" yet. Report only by default (the change run and
rolled back); --apply in one transaction; a second --apply refuses (the log holds rows); --undo deletes the stock
row and the record (refused while a character holds or a merchant buys back a copy), each with its own gm_audit line,
and drops the log.

WHEN IT TAKES EFFECT: at once (itempattern and npcitem are read at each window and sale; a client gets the record at
its next login or at the shop's window).

    python3 -E -s tools/scroll_of_returning_20261126.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "scroll_of_returning_1126_log"
WHO, VIA = "tool", "tool"
DETAIL = "The Scroll of Returning, made and sold by Freeport's Spell Archivist: tools/scroll_of_returning_20261126.py"
NAME = "Scroll of Returning"
DESC = ("Read it within a heroic dungeon to mark your place and return home. Read it again within 30 minutes to return "
        "to that very spot, and it crumbles to dust.")
SELLER = "Spell Archivist"
TEMPLATE = "Commons Gate"
SCROLL_ICON = -607649328  # handlers/scribe.go's scrollIcon
PRICE = 1000
MAX_STACK = 20
FROM_ID = 716970


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def has(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone() is not None


def row(db, sql, params):
    cur = db.execute(sql, params)
    r = cur.fetchone()
    if r is None:
        return None
    return {d[0]: v for d, v in zip(cur.description, r)}


class Writer:
    def __init__(self, db):
        self.db = db
        self.n = 0

    def log(self, tbl, key, before, after, action="apply"):
        b = json.dumps(before, sort_keys=True) if before is not None else None
        a = json.dumps(after, sort_keys=True) if after is not None else None
        k = json.dumps(key, sort_keys=True)
        cur = self.db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                              "VALUES (?, 0, ?, ?, '', ?, ?, ?, ?, ?, ?)", (now(), WHO, VIA, action, tbl, k, b, a, DETAIL))
        if action == "apply":
            self.db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)", (tbl, k, b, a, cur.lastrowid))
        self.n += 1
        return cur.lastrowid


def find(db):
    """The seller and the template, or the refusals."""
    bad = []
    sellers = db.execute("SELECT id FROM npcs WHERE npc_name = ? AND world = 0", (SELLER,)).fetchall()
    if len(sellers) != 1:
        bad.append(f"{len(sellers)} NPCs named {SELLER} in world 0, want 1")
    tmpl = db.execute("SELECT patternid FROM itempattern WHERE itemname = ? AND itemicon = ? AND equipslot = -1",
                      (TEMPLATE, SCROLL_ICON)).fetchall()
    if len(tmpl) != 1:
        bad.append(f"{len(tmpl)} scroll records named {TEMPLATE!r}, want 1")
    if db.execute("SELECT 1 FROM itempattern WHERE lower(trim(itemname)) = lower(?)", (NAME,)).fetchone():
        bad.append(f"a record named {NAME!r} is there already")
    if bad:
        return None, bad
    return (sellers[0][0], tmpl[0][0]), bad


def apply(db, seller, tmpl, say):
    w = Writer(db)
    t = row(db, "SELECT * FROM itempattern WHERE patternid = ?", (tmpl,))
    pid = FROM_ID
    while db.execute("SELECT 1 FROM itempattern WHERE patternid = ?", (pid,)).fetchone():
        pid += 1
    rec = dict(t, patternid=pid, itemname=NAME, itemdesc=DESC, itemcost=PRICE, classuse=32767, raceuse=32767, levelreq=1,
               maxstack=MAX_STACK, trade=0, lore=0, craft=0)
    cols = list(rec)
    db.execute(f"INSERT INTO itempattern ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", tuple(rec[c] for c in cols))
    w.log("itempattern", {"patternid": pid}, None, row(db, "SELECT * FROM itempattern WHERE patternid = ?", (pid,)))
    say(f"record: {NAME} ({pid}), cloned from {TEMPLATE} ({tmpl}), valued {PRICE}, stacks to {MAX_STACK}")
    last = db.execute("SELECT COALESCE(MAX(listnumber), -1) FROM npcitem WHERE npcid = ?", (seller,)).fetchone()[0]
    cur = db.execute("INSERT INTO npcitem (npcid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber) "
                     "VALUES (?, 1, 0, 0, ?, -1, 1, ?)", (seller, pid, last + 1))
    w.log("npcitem", {"itemid": cur.lastrowid}, None, row(db, "SELECT * FROM npcitem WHERE itemid = ?", (cur.lastrowid,)))
    say(f"stock: {SELLER} ({seller}) sells {NAME} at list {last + 1}, npcitem {cur.lastrowid}")
    return w.n


def undo(db, say):
    if not has(db, LOG):
        say("nothing to undo")
        return
    db.execute("BEGIN IMMEDIATE")
    try:
        w = Writer(db)
        for tbl, k, b, a in db.execute(f"SELECT tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
            key = json.loads(k)
            if tbl == "itempattern":
                pid = key["patternid"]
                for t in ("charinventory", "charbuyback"):
                    if has(db, t) and db.execute(f"SELECT 1 FROM {t} WHERE patternid = ?", (pid,)).fetchone():
                        raise SystemExit(f"refused: {t} holds a {NAME} ({pid}); nothing undone")
            where = " AND ".join(f"{c} = ?" for c in key)
            now_row = row(db, f"SELECT * FROM {tbl} WHERE {where}", tuple(key.values()))
            db.execute(f"DELETE FROM {tbl} WHERE {where}", tuple(key.values()))
            w.log(tbl, key, now_row, None, action="undo")
            say(f"removed: {tbl} {key}")
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    say(f"undone: {w.n} rows")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, timeout=5, isolation_level=None)
    if a.undo:
        undo(db, print)
        return
    if has(db, LOG) and db.execute(f"SELECT 1 FROM {LOG}").fetchone():
        print(f"refused: {LOG} holds rows: the change is in already")
        sys.exit(1)
    found, bad = find(db)
    if bad:
        for b in bad:
            print("refused:", b)
        sys.exit(1)
    seller, tmpl = found
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (seq INTEGER PRIMARY KEY AUTOINCREMENT, tbl TEXT NOT NULL, rowkey TEXT NOT NULL, "
                   "before TEXT, after TEXT, audit INTEGER NOT NULL DEFAULT 0, applied TEXT NOT NULL DEFAULT (datetime('now')))")
        n = apply(db, seller, tmpl, print)
        if a.apply:
            db.execute("COMMIT")
            print(f"applied: {n} rows")
        else:
            db.execute("ROLLBACK")
            print(f"report only: {n} rows would be written (--apply to write)")
    except Exception:
        db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    main()
