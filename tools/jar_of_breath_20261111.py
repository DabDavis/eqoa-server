#!/usr/bin/env python3
"""The Heirloom Jar of Breath (the owner, 2026-11-11: "lets also add a heirloom jar of breath that buffs underwater breath
for 4 hours"; internal/handlers/jarofbreath.go).

What it writes (a report by default; --apply writes, in one transaction with a gm_audit line a row; --undo takes it back):
- **the jar's record** (itempattern 715070, the next of the kept items' ids after Healbot's Cog 715060): "Heirloom Jar of
  Breath", worn nowhere, CLIENT's gem type (procanim 7, as the Heirloom Fang of the Wolf and the Rock of Freeport: the
  item menu's Use and the HUD's gem quick slot), NO TRADE (trade 1), LORE (one each), worth 0, a stack of 1, level 1,
  every class and race, 80 DUR, on Breath Potion's icon (701393: 1342617881, a jar, the Ooze Jar's too); the server never
  spends it (jarofbreath.go);
- **its use** (itemeffect 715070): "Self, Enduring Breath, 4hr", on the effects list as "Enduring Breath" with Hold
  Breath's ability icon (spellpattern 901781: 1533256884): under water no breath is lost for four hours, kept over a
  logout as every effect (its time runs only in the world), ended by death; a second use renews the four hours.
No startitems rows: every character, new or old, is given its jar at its login (the server's giveJarOfBreath, with the
lamp, the fang and the portal: world.heirlooms; in the pack, else the bank). The gift and the never-spent Use are code:
apply this with the server that has jarofbreath.go (before it the record is a potion used up at its first drink, and no
one is given one). **The Heirloom Test Server's database only**: the Live Server has no heirloom items. Data only; no
restart (the record reaches a client at its login; the row is read at each use).

Refused, nothing written, unless 715070 is free and no record has the jar's name (or both are exactly this tool's:
applied already), and the icon's sources are as named. Log jar_of_breath_1111_log; --undo deletes the two rows (each
only as written), every jar characters hold (the server gave them; with the record gone they would name nothing), the
quick slots holding one, and the kept effects of its use (charbuffs).

    tools/jar_of_breath_20261111.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "jar_of_breath_1111_log"
DETAIL = "The Heirloom Jar of Breath (the owner, 2026-11-11): tools/jar_of_breath_20261111.py"
WHO, VIA = "dab", "tool"
JAR_ID = 715070
NAME = "Heirloom Jar of Breath"
DESC = ("A stoppered jar of cool air that never runs out. Use it and you lose no breath under water for four hours. Death "
        "ends it; use the jar again. It never wears out.")
ICON_FROM = (701393, "Breath Potion", 1342617881)  # the jar's icon: Breath Potion's (and the Ooze Jar's)
EFFECT_ICON_FROM = (901781, "Hold Breath 1", 1533256884)  # its effect's: Hold Breath's ability icon
GEM = 7
POTION_EFFECT_BASE = -1_000_000  # entity/potion.go: an item's effect id is this less its id
RECORD = dict(patternid=JAR_ID, itemcost=0, unk1=0, itemicon=ICON_FROM[2], unk2=0, equipslot=-1, unk3=0, trade=1, rent=1,
              unk4=0, attacktype=0, weapondamage=0, unk5=0, levelreq=1, maxstack=1, maxhp=0, duration=80, classuse=32767,
              raceuse=32767, procanim=GEM, lore=1, unk6=0, craft=0, itemname=NAME, itemdesc=DESC, model=0,
              color=0xFFFFFFFF)
# the figures' columns, 0 where this database has them (as the fang's record; NULL otherwise)
STATS = ("str", "sta", "agi", "dex", "wis", "intelligence", "cha", "HPMAX", "POWMAX", "PoT", "HoT", "AC", "PR", "DR", "FR",
         "CR", "LR", "AR", "weaponproc", "fish")
EFFECT = dict(patternid=JAR_ID, line="Self, Enduring Breath, 4hr", name="Enduring Breath", icon=EFFECT_ICON_FROM[2],
              breaks="", wear=0, area=0.0, source="jarofbreath 2026-11-11 (the owner's word)", body=0, target_line="",
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
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json({"patternid": JAR_ID}),
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
    rec, eff = row_of(db, "itempattern", JAR_ID), row_of(db, "itemeffect", JAR_ID)
    if same(rec, record_for(db)) and same(eff, EFFECT):
        return [], [], True
    if missing := set(RECORD) - {r[1] for r in db.execute("PRAGMA table_info(itempattern)")}:
        refusals.append(f"itempattern lacks {sorted(missing)}")
    if missing := set(EFFECT) - {r[1] for r in db.execute("PRAGMA table_info(itemeffect)")}:
        refusals.append(f"itemeffect lacks {sorted(missing)} (a server older than the itemeffect columns)")
    if rec is not None:
        refusals.append(f"itempattern {JAR_ID} is taken: {rec['itemname']!r}")
    if eff is not None:
        refusals.append(f"itemeffect {JAR_ID} is taken: {eff['line']!r}")
    for (pid, name) in db.execute("SELECT patternid, itemname FROM itempattern WHERE itemname = ? COLLATE NOCASE", (NAME,)):
        refusals.append(f"itempattern {pid} is named {name!r} already")
    got = db.execute("SELECT itemname, itemicon FROM itempattern WHERE patternid = ?", (ICON_FROM[0],)).fetchone()
    if got != (ICON_FROM[1], ICON_FROM[2]):
        refusals.append(f"itempattern {ICON_FROM[0]} is {got!r}, not {ICON_FROM[1:]!r} (the jar's icon)")
    got = db.execute("SELECT spellname, icon FROM spellpattern WHERE spellid = ?", (EFFECT_ICON_FROM[0],)).fetchone()
    if got != (EFFECT_ICON_FROM[1], EFFECT_ICON_FROM[2]):
        refusals.append(f"spellpattern {EFFECT_ICON_FROM[0]} is {got!r}, not {EFFECT_ICON_FROM[1:]!r} (its effect's icon)")
    chars = db.execute("SELECT COUNT(*) FROM characters").fetchone()[0] if table(db, "characters") else 0
    lines = [f"itempattern {JAR_ID} {NAME!r}: gem type {GEM}, NO TRADE, LORE, worth 0, icon {RECORD['itemicon']} "
             f"({ICON_FROM[1]}'s)",
             f"itemeffect {JAR_ID}: {EFFECT['line']!r} as {EFFECT['name']!r}, icon {EFFECT['icon']} ({EFFECT_ICON_FROM[1]}'s), never spent",
             f"{chars} characters are given theirs at their next login (world.heirlooms)"]
    return lines, refusals, False


def insert(db, tbl, row):
    cols = list(row)
    db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [row[c] for c in cols])
    now = row_of(db, tbl, JAR_ID)
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
                           (JAR_ID,)).rowcount
    held = db.execute("DELETE FROM charinventory WHERE patternid = ?", (JAR_ID,)).rowcount
    if table(db, "charbuffs"):
        buffs = db.execute("DELETE FROM charbuffs WHERE spellid = ?", (POTION_EFFECT_BASE - JAR_ID,)).rowcount
    for seq, tbl, after in db.execute(f"SELECT seq, tbl, after FROM {LOG} ORDER BY seq DESC").fetchall():
        now = row_of(db, tbl, JAR_ID)
        if now is None or json.dumps(now, sort_keys=True) != after:
            left += 1
            print(f"  left: {tbl} {JAR_ID}, changed since it was written: {now}")
            continue
        db.execute(f"DELETE FROM {tbl} WHERE patternid = ?", (JAR_ID,))
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
                print(f"put back {back} rows, {left} left (changed since); took {held} jars characters held, {quick} quick "
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
        print(f"applied: the record and its use, in gm_audit and {LOG}; each character gets its jar at its next login")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
