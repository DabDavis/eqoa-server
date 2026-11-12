#!/usr/bin/env python3
"""The Tester Pro (the GM Jibrish's petition 1725: "Create an item that has infinite charges and is no trade lore. Call it
"Tester Pro". When the player uses it, it gives them a run speed buff that is x2 SoW speed. However, also make it reduce
their max HP to 1, and set all of their stats to 1 while the buff is up"; internal/entity/hpcap.go).

What it writes (a report by default; --apply writes, in one transaction with a gm_audit line a row; --undo takes it back):
- **the record** (itempattern 965006, the next of the GM items' ids after Heroic Ceremonial Vestments 965005): "Tester
  Pro", worn nowhere, CLIENT's gem type (procanim 7, as the Ring of Speed and the Heirloom Jar of Breath: the item menu's
  Use and the HUD's gem quick slot), NO TRADE (trade 1), LORE (one each), worth 0, a stack of 1, level 1, every class and
  race, 80 DUR, on the Ring of Speed's icon (960149: -1505803670);
- **its use** (itemeffect 965006): "Self, Run Speed 2x Spirit of the Wolf, Set Maximum HP to 1, Set All Stats to 1,
  Unlimited Uses, 30min", on the effects list as "Tester Pro" with Spirit of the Wolf's icon (spellpattern 452:
  638503769): for 30 minutes its user runs at twice Spirit of the Wolf's speed, its maximum hit points are 1 (its hit
  points with them) and STR, STA, AGI, DEX, WIS, INT and CHA are each 1, whatever else would raise them; at the end all of
  it is as it was (the hit points where it left them, to regenerate). A second use renews the 30 minutes; kept over a
  logout as every effect; ended by death. "Unlimited Uses" is the never-spent rule: no charge is ever taken.
No one is given one here: a GM gives it (#giveitem 965006 <player>, the panel's give_item). The line's four clauses are
code (hpcap.go, spelltext.go, handlers/potion.go): apply this with the server that reads them (before it the line reads
none of them and the item is a potion used up at its first use). Data only; no restart (the record reaches a client at its
login; the row is read at each use). Either shard's database.

Refused, nothing written, unless 965006 is free and no record has the name (or both are exactly this tool's: applied
already), and the icons' sources are as named. Log tester_pro_1112_log; --undo deletes the two rows (each only as
written), every Tester Pro characters hold, the quick slots holding one and the kept effects of its use (charbuffs).

    tools/tester_pro_20261112.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "tester_pro_1112_log"
DETAIL = "The Tester Pro (petition 1725, the GM Jibrish): tools/tester_pro_20261112.py"
WHO, VIA = "dab", "tool"
ITEM_ID = 965006
NAME = "Tester Pro"
DESC = ("A tester's charm that never wears out. Use it and for 30 minutes you run twice as fast as Spirit of the Wolf, "
        "but your maximum hit points are 1 and every stat is 1. Use it again to renew it.")
ICON_FROM = (960149, "Ring of Speed", -1505803670)  # the item's icon: the Ring of Speed's
EFFECT_ICON_FROM = (452, "Spirit of the Wolf", 638503769)  # its effect's: Spirit of the Wolf's
GEM = 7
POTION_EFFECT_BASE = -1_000_000  # entity/potion.go: an item's effect id is this less its id
RECORD = dict(patternid=ITEM_ID, itemcost=0, unk1=0, itemicon=ICON_FROM[2], unk2=0, equipslot=-1, unk3=0, trade=1, rent=1,
              unk4=0, attacktype=0, weapondamage=0, unk5=0, levelreq=1, maxstack=1, maxhp=0, duration=80, classuse=32767,
              raceuse=32767, procanim=GEM, lore=1, unk6=0, craft=0, itemname=NAME, itemdesc=DESC, model=0,
              color=0xFFFFFFFF)
# the figures' columns, 0 where this database has them (as the jar's record; NULL otherwise)
STATS = ("str", "sta", "agi", "dex", "wis", "intelligence", "cha", "HPMAX", "POWMAX", "PoT", "HoT", "AC", "PR", "DR", "FR",
         "CR", "LR", "AR", "weaponproc", "fish")
LINE = "Self, Run Speed 2x Spirit of the Wolf, Set Maximum HP to 1, Set All Stats to 1, Unlimited Uses, 30min"
EFFECT = dict(patternid=ITEM_ID, line=LINE, name=NAME, icon=EFFECT_ICON_FROM[2],
              breaks="", wear=0, area=0.0, source="testerpro 2026-11-12 (petition 1725, the GM Jibrish)", body=0, target_line="",
              charges=0, cooldown=0.0)
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq   INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl   TEXT NOT NULL,      -- itempattern or itemeffect
    after TEXT NOT NULL,      -- the row as written (JSON)
    audit INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def row_of(db, tbl, id):
    cur = db.execute(f"SELECT * FROM {tbl} WHERE patternid = ?", (id,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, tbl, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json({"patternid": ITEM_ID}),
                      None if before is None else go_json(before), None if after is None else go_json(after), detail))
    return cur.lastrowid


def record_for(db):
    """RECORD with the figures' columns this itempattern has, at 0."""
    cols = {r[1] for r in db.execute("PRAGMA table_info(itempattern)")}
    return {**RECORD, **{c: 0 for c in STATS if c in cols}}


def same(now, want):
    return now is not None and all(now.get(k) == v for k, v in want.items())


def plan(db):
    """(lines, refusals, applied)."""
    refusals = []
    for t in ("itempattern", "itemeffect", "spellpattern", "gm_audit"):
        if not table(db, t):
            return [], [f"no {t} table here"], False
    rec, eff = row_of(db, "itempattern", ITEM_ID), row_of(db, "itemeffect", ITEM_ID)
    if same(rec, record_for(db)) and same(eff, EFFECT):
        return [], [], True
    if missing := set(RECORD) - {r[1] for r in db.execute("PRAGMA table_info(itempattern)")}:
        refusals.append(f"itempattern lacks {sorted(missing)}")
    if missing := set(EFFECT) - {r[1] for r in db.execute("PRAGMA table_info(itemeffect)")}:
        refusals.append(f"itemeffect lacks {sorted(missing)} (a server older than the itemeffect columns)")
    if rec is not None:
        refusals.append(f"itempattern {ITEM_ID} is taken: {rec['itemname']!r}")
    if eff is not None:
        refusals.append(f"itemeffect {ITEM_ID} is taken: {eff['line']!r}")
    for (pid, name) in db.execute("SELECT patternid, itemname FROM itempattern WHERE itemname = ? COLLATE NOCASE", (NAME,)):
        refusals.append(f"itempattern {pid} is named {name!r} already")
    got = db.execute("SELECT itemname, itemicon FROM itempattern WHERE patternid = ?", (ICON_FROM[0],)).fetchone()
    if got != (ICON_FROM[1], ICON_FROM[2]):
        refusals.append(f"itempattern {ICON_FROM[0]} is {got!r}, not {ICON_FROM[1:]!r} (the item's icon)")
    got = db.execute("SELECT spellname, icon FROM spellpattern WHERE spellid = ?", (EFFECT_ICON_FROM[0],)).fetchone()
    if got != (EFFECT_ICON_FROM[1], EFFECT_ICON_FROM[2]):
        refusals.append(f"spellpattern {EFFECT_ICON_FROM[0]} is {got!r}, not {EFFECT_ICON_FROM[1:]!r} (its effect's icon)")
    chars = db.execute("SELECT COUNT(*) FROM characters").fetchone()[0] if table(db, "characters") else 0
    lines = [f"itempattern {ITEM_ID} {NAME!r}: gem type {GEM}, NO TRADE, LORE, worth 0, icon {RECORD['itemicon']} "
             f"({ICON_FROM[1]}'s)",
             f"itemeffect {ITEM_ID}: {EFFECT['line']!r} as {EFFECT['name']!r}, icon {EFFECT['icon']} ({EFFECT_ICON_FROM[1]}'s), never spent",
             f"no one is given one ({chars} characters): a GM gives it, #giveitem {ITEM_ID} <player>"]
    return lines, refusals, False


def insert(db, tbl, row):
    cols = list(row)
    db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [row[c] for c in cols])
    now = row_of(db, tbl, ITEM_ID)
    a = audit(db, "insert", tbl, None, now, DETAIL)
    db.execute(f"INSERT INTO {LOG} (tbl, after, audit) VALUES (?, ?, ?)", (tbl, json.dumps(now, sort_keys=True), a))


def apply(db):
    db.execute(LOG_SCHEMA)
    insert(db, "itempattern", record_for(db))
    insert(db, "itemeffect", EFFECT)


def undo(db):
    """(back, left, held, quick, buffs)."""
    if not table(db, LOG):
        return 0, 0, 0, 0, 0
    back = left = 0
    quick = buffs = 0
    if table(db, "quickslots"):
        quick = db.execute("DELETE FROM quickslots WHERE itemid IN (SELECT itemid FROM charinventory WHERE patternid = ?)",
                           (ITEM_ID,)).rowcount
    held = db.execute("DELETE FROM charinventory WHERE patternid = ?", (ITEM_ID,)).rowcount
    if table(db, "charbuffs"):
        buffs = db.execute("DELETE FROM charbuffs WHERE spellid = ?", (POTION_EFFECT_BASE - ITEM_ID,)).rowcount
    for seq, tbl, after in db.execute(f"SELECT seq, tbl, after FROM {LOG} ORDER BY seq DESC").fetchall():
        now = row_of(db, tbl, ITEM_ID)
        if now is None or json.dumps(now, sort_keys=True) != after:
            left += 1
            print(f"  left: {tbl} {ITEM_ID}, changed since it was written: {now}")
            continue
        db.execute(f"DELETE FROM {tbl} WHERE patternid = ?", (ITEM_ID,))
        audit(db, "delete", tbl, now, None, "undo: " + DETAIL)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if left == 0:
        db.execute(f"DROP TABLE {LOG}")
    return back, left, held, quick, buffs


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        if a.undo:
            db.execute("BEGIN IMMEDIATE")
            try:
                back, left, held, quick, buffs = undo(db)
                db.execute("COMMIT")
            except BaseException:
                db.execute("ROLLBACK")
                raise
            if back or left:
                print(f"put back {back} rows, {left} left (changed since); took {held} Tester Pros characters held, {quick} quick "
                      f"slots holding one, {buffs} kept effects of its use")
            else:
                print("nothing to undo")
            return 0 if left == 0 else 1
        lines, refusals, applied = plan(db)
        for line in lines:
            print(line)
        if applied:
            print("applied already (the record and its use are this tool's): nothing done")
            return 0
        if table(db, LOG):
            print(f"{LOG} is there but the rows are not as written: --undo first")
            return 1
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not a.apply:
            print("report only: 2 rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            apply(db)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: the record and its use, in gm_audit and {LOG}; a GM gives it: #giveitem {ITEM_ID} <player>")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
