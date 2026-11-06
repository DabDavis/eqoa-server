#!/usr/bin/env python3
"""The Heirloom Fang of the Wolf (the user, 2026-11-06: "lets add a speed heir loom item like the lamp they save it to the
item toolbar"; internal/handlers/wolffang.go, internal/entity/boardwolf.go).

What it writes (a report by default; --apply writes, --undo takes back what the last run wrote):
- **the fang's record** (itempattern 715010): "Heirloom Fang of the Wolf", worn nowhere, CLIENT's gem type (procanim 7:
  the item menu's Use, and the HUD's third quick slot, so the two potion slots stay the potions' and the lamp's), NO
  TRADE (trade 1), worth 0, a stack of 1, level 1, every class and race, 80 DUR, on the Forger Wolf Fang's icon (700137,
  -720619736: a wolf's fang); the server never spends it. Its Use puts the towns' boards' Spirit of the Wolf on its holder
  (Spirit of the Wolf's run speed for an hour of play; the holder's own hostile act ends it);
- **startitems**: one fang for every race, class and city combo (a new character starts with it).
A character already made gets its fang at its next login (the server's giveWolfFang, in the pack, else the bank): not
here, since a character in the world reads its items only at its login. Its Use and that gift are code: apply this with
the server that has wolffang.go, or after it (before it the record does nothing when used, and only new characters get
one). Data only; no restart (the record reaches a client at its login). Logged in wolf_fang_log; --undo deletes the
record, its startitems rows (and their AUTOINCREMENT mark), every fang a character holds (the server gave those; with
the record gone they would name nothing) and the quick slots holding one.

    tools/seed_wolf_fang.py [--db eqoa.db] [--list] [--apply | --undo]
"""

import argparse
import datetime
import os
import sqlite3

LOG = "wolf_fang_log"
FANG_ID = 715010
NAME = "Heirloom Fang of the Wolf"
DESC = ("Use it and the spirit of the wolf quickens your step for an hour of play. A hostile act ends it; use the fang "
        "again. It never wears out.")
LOOK = 700137  # the Forger Wolf Fang: its icon (-720619736), a wolf's fang
GEM = 7  # CLIENT's gem type: IsUsable, the HUD's third quick slot (Msg000A)
INV_CAP = 40


def ensure(db):
    db.execute(f"""CREATE TABLE IF NOT EXISTS {LOG} (seq INTEGER PRIMARY KEY, run TEXT NOT NULL, tbl TEXT NOT NULL,
                   rowkey INTEGER NOT NULL, action TEXT NOT NULL)""")


def plan(db):
    if db.execute(f"SELECT COUNT(*) FROM {LOG}").fetchone()[0]:
        raise SystemExit("the fang is in this database already: --undo first")
    if db.execute("SELECT COUNT(*) FROM itempattern WHERE patternid = ? OR itemname = ?", (FANG_ID, NAME)).fetchone()[0]:
        raise SystemExit(f"a record {FANG_ID} or named {NAME!r} exists")
    cols = [r[1] for r in db.execute("PRAGMA table_info(itempattern)")]
    look = db.execute(f"SELECT {', '.join(cols)} FROM itempattern WHERE patternid = ?", (LOOK,)).fetchone()
    if look is None:
        raise SystemExit(f"no record {LOOK} for the fang's look")
    r = dict(zip(cols, look))
    for c in ("str", "sta", "agi", "dex", "wis", "intelligence", "cha", "HPMAX", "POWMAX", "PoT", "HoT", "AC",
              "PR", "DR", "FR", "CR", "LR", "AR", "weaponproc", "fish"):
        if c in r:
            r[c] = 0
    r.update(patternid=FANG_ID, itemname=NAME, itemdesc=DESC, itemcost=0, equipslot=-1, trade=1, rent=1, levelreq=1,
             maxstack=1, maxhp=0, duration=80, classuse=32767, raceuse=32767, procanim=GEM, lore=0, craft=0, attacktype=0,
             weapondamage=0, unk5=0, model=0)
    combos = db.execute("SELECT DISTINCT race, class, humtype FROM startitems ORDER BY race, class, humtype").fetchall()
    chars = db.execute("SELECT COUNT(*) FROM characters").fetchone()[0]
    full = db.execute(f"SELECT COUNT(*) FROM (SELECT serverid FROM charinventory WHERE location = 0 GROUP BY serverid "
                      f"HAVING COUNT(*) >= {INV_CAP})").fetchone()[0]
    return dict(record={c: r.get(c) for c in cols}, combos=combos, chars=chars, full=full)


def report(p, full):
    print(f"the record {FANG_ID} {NAME!r} (icon {p['record']['itemicon']}, type {p['record']['procanim']}); "
          f"{len(p['combos'])} startitems combos; {p['chars']} characters get theirs at their next login "
          f"({p['full']} with a full pack: the bank)")
    if full:
        for c in p["combos"]:
            print("  startitems race %d class %d humtype %d" % c)


def apply(db, p):
    run = datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")
    db.execute("BEGIN IMMEDIATE")
    try:
        log = lambda tbl, key, action: db.execute(f"INSERT INTO {LOG} (run, tbl, rowkey, action) VALUES (?, ?, ?, ?)",
                                                  (run, tbl, key, action))
        seq = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'startitems'").fetchone()
        if seq is not None:  # startitems' AUTOINCREMENT mark, put back by --undo
            log("sqlite_sequence", seq[0], "startitems")
        rec = p["record"]
        cols = list(rec)
        db.execute(f"INSERT INTO itempattern ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                   [rec[c] for c in cols])
        log("itempattern", FANG_ID, "insert")
        for race, cls, hum in p["combos"]:
            cur = db.execute("INSERT INTO startitems (race, class, humtype, patternid, count, note, source) "
                             "VALUES (?, ?, ?, ?, 1, ?, 'wolffang')",
                             (race, cls, hum, FANG_ID, "the Heirloom Fang of the Wolf (wolffang.go)"))
            log("startitems", cur.lastrowid, "insert")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"wrote it (run {run}): no restart needed; each character gets its fang at its next login")


def undo(db):
    rows = db.execute(f"SELECT tbl, rowkey FROM {LOG} ORDER BY seq DESC").fetchall()
    if not rows:
        raise SystemExit("nothing to undo")
    db.execute("BEGIN IMMEDIATE")
    try:
        quick = 0
        if db.execute("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'quickslots'").fetchone()[0]:
            quick = db.execute("DELETE FROM quickslots WHERE itemid IN (SELECT itemid FROM charinventory WHERE patternid = ?)",
                               (FANG_ID,)).rowcount
        held = db.execute("DELETE FROM charinventory WHERE patternid = ?", (FANG_ID,)).rowcount
        for tbl, key in rows:
            if tbl == "startitems":
                db.execute("DELETE FROM startitems WHERE id = ?", (key,))
            elif tbl == "itempattern":
                db.execute("DELETE FROM itempattern WHERE patternid = ?", (key,))
        for tbl, key in rows:
            if tbl == "sqlite_sequence":  # the mark back, unless rows came after the run's
                db.execute("UPDATE sqlite_sequence SET seq = MAX(?, (SELECT COALESCE(MAX(id), 0) FROM startitems)) "
                           "WHERE name = 'startitems'", (key,))
        db.execute(f"DELETE FROM {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"took back {len(rows)} rows, {held} fangs characters held and {quick} quick slots holding one")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--list", action="store_true")
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--undo", action="store_true")
    ap.add_argument("--no-backup", action="store_true")
    a = ap.parse_args()
    if not os.path.isfile(a.db):
        raise SystemExit(f"no database at {a.db}")
    db = sqlite3.connect(a.db, isolation_level=None, timeout=10)
    if (a.apply or a.undo) and not a.no_backup:
        stamp = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
        db.execute(f"VACUUM INTO '{a.db}.bak-{stamp}-pre-wolf-fang'")
    ensure(db)
    if a.undo:
        undo(db)
        return
    p = plan(db)
    report(p, a.list)
    if a.apply:
        apply(db, p)


if __name__ == "__main__":
    main()
