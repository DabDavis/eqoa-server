#!/usr/bin/env python3
"""The shadow knight's Undead Servant, a familiar (petition 1805, Skywalker, 2026-11-12: "this spell does not work
this is what it should do: Summon Undead Servant (Familiar). Buying Undead servitude CM gives 65PR/PR/AR. The pet
Buffs 300 Hp and 300 Power along with a 80 lifetap proc that after the mob is debuffed, will often proc 160 instead").

Summon Servant (701236, the Death Knight's Undead Servitude, cmcatalog 4112) summoned EQOAEmu's workbook pet 0064, a
level 45 fighter of 3,203 HP with no abilities (petstats abilities []): it did nothing for its owner and died within
seconds of a fight. Now it is a familiar as the wizard's are (Arch/Gargoyle Familiar: a pet that buffs its owner):

  - spell 907236 "Undead Servitude" (new), the servant's buff on its owner: Max HP +300, Max Power +300, and an
    offensive proc on its owner's melee, a lifetap of 80 (80 off the creature, 80 healed), 160 (and 160 healed) on a
    creature that carries a lasting harmful spell of the owner's (his debuffs, snares and damage over time:
    debuffed_pct 100, entity/proc.go). Its chance is every proc's, by the owner's INT.
  - petstats 701236 abilities: that buff on its owner, every 30 s while it lacks it, from 1 s, while_out (it comes
    off when the servant goes, and is never kept over a logout: entity/pet.go petBuffsGoLocked).
  - spellpattern 701236's description says what it does.
  - cmcatalog 4112 Undead Servitude: AR, PR and DR 50 -> 65 (the petition's "65PR/PR/AR", read as PR/DR/AR); its HP
    250 and AC 200 stay.

Needs the server build with debuffed_pct and while_out (an older one reads the petstats row as bad and leaves the
pet as it was). Refused when a row is not as expected. Report only by default; --apply in one transaction, a gm_audit
line a row, log undeadservant_1112_log; --undo exact (a row changed since is left and said). cmcatalog, spellpattern,
spelleffect and petstats are read at the start: a restart.

    tools/undead_servant_20261112.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "undeadservant_1112_log"
DETAIL = ("petition 1805 (Skywalker): the Undead Servant a familiar (+300 HP, +300 power, an 80/160 lifetap proc), "
          "Undead Servitude's resists 65: tools/undead_servant_20261112.py")
WHO, VIA = "claude", "tool"
SUMMON = 701236
BUFF = 907236
CM = 4112


def compact(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False)


BUFF_EFFECTS = compact({
    "v": 1, "scope": "target", "target_type": "Owner", "harmful": False, "duration_s": 1800, "stack": 0, "level": 60,
    "min_level": 1, "req_equipment": "Any", "sheet_type": "Statistic Change", "arch_type": "Pet Buff",
    "effects": [
        {"id": "F805", "type": "Statistic Change", "kind": "stat", "stat": "maxhp", "amount": 300},
        {"id": "F806", "type": "Statistic Change", "kind": "stat", "stat": "maxpower", "amount": 300},
        {"id": "F807", "type": "Offensive Proc", "kind": "proc", "procs": [
            {"id": "F808", "type": "Dmg/Heal", "kind": "hp", "amount": -80, "debuffed_pct": 100, "on": "offense",
             "scope": "target", "target_type": "Enemy"},
            {"id": "F809", "type": "Dmg/Heal", "kind": "hp", "amount": 80, "debuffed_pct": 100, "on": "offense",
             "scope": "self", "target_type": "Self"}]}]})

ABILITIES = compact([{"every": 30.0, "first": 1.0, "kind": "buff", "spell_id": BUFF, "target": "owner",
                      "while_out": True}])

# new rows: (table, key column, row)
INSERTS = [
    ("spellpattern", "spellid", {
        "spellid": BUFF, "abilitylvl": 60, "unk2": 0, "unk3": 0, "range": 16880, "casttime": 2, "power": 0,
        "iconColor": -1809115846, "icon": 2036326070, "scope": 1, "recast": 30, "eqprequire": -1,
        "spellname": "Undead Servitude",
        "spelldesc": "The undead servant's gift to its master while it serves: maximum hit points and power, and a "
                     "lifetap on the master's blows, twice as strong on a foe the master has cursed."}),
    ("spelleffect", "spellid", {
        "spellid": BUFF, "fxdict": 4089483784, "source": "petition 1805 (Skywalker), tools/undead_servant_20261112.py",
        "line": "Target, Increase Maximum HP by 300, Increase Maximum Power by 300, Add Offensive Proc: Decrease "
                "Hitpoints by 80 (160 on a target you have debuffed), Increase Hitpoints by 80 to Self (160), while "
                "the servant is out",
        "effects": BUFF_EFFECTS}),
]

# changed columns: (table, key column, key, before, after)
UPDATES = [
    ("petstats", "spellid", SUMMON, {"abilities": "[]"}, {"abilities": ABILITIES}),
    ("spellpattern", "spellid", SUMMON, {"spelldesc": "Summons Pet"},
     {"spelldesc": "Summons an undead servant, a familiar: while it is out its master has 300 more hit points and "
                   "power, and an 80 point lifetap on his blows, 160 on a foe he has cursed."}),
    ("cmcatalog", "id", CM, {"effects": "HP:250,AC:200,AR:50,PR:50,DR:50"},
     {"effects": "HP:250,AC:200,AR:65,PR:65,DR:65"}),
]

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    action  TEXT NOT NULL,      -- insert | update
    tbl     TEXT NOT NULL,
    keycol  TEXT NOT NULL,
    id      INTEGER NOT NULL,
    before  TEXT,               -- update: the changed columns as they were (JSON)
    after   TEXT NOT NULL,      -- insert: the whole row as written; update: the columns as written (JSON)
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def row_of(db, tbl, keycol, id, cols=None):
    cur = db.execute(f"SELECT {', '.join(cols) if cols else '*'} FROM {tbl} WHERE {keycol} = ?", (id,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def audit(db, action, tbl, keycol, id, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json({keycol: id}),
                      None if before is None else go_json(before), None if after is None else go_json(after), detail))
    return cur.lastrowid


def plan(db):
    """(inserts, updates, refusals, notes): what is left to write."""
    ins, ups, refusals, notes = [], [], [], []
    for t in ("spellpattern", "spelleffect", "petstats", "cmcatalog", "gm_audit"):
        if not table(db, t):
            return ins, ups, [f"no {t} table here"], notes
    for tbl, keycol, row in INSERTS:
        cols = {r[1] for r in db.execute(f"PRAGMA table_info({tbl})")}
        if missing := set(row) - cols:
            return ins, ups, [f"{tbl} lacks {sorted(missing)}"], notes
        now = row_of(db, tbl, keycol, row[keycol], list(row))
        if now == row:
            notes.append(f"{tbl} {row[keycol]} is written already")
        elif now is None:
            ins.append((tbl, keycol, row))
        else:
            refusals.append(f"{tbl} {row[keycol]} is taken: {now.get('spellname') or now.get('line')!r}")
    for tbl, keycol, id, before, after in UPDATES:
        now = row_of(db, tbl, keycol, id, list(before))
        if now is None:
            refusals.append(f"no {tbl} {id}")
        elif now == after:
            notes.append(f"{tbl} {id} is as written already")
        elif now == before:
            ups.append((tbl, keycol, id, before, after))
        else:
            refusals.append(f"{tbl} {id} is {now}, neither {before} nor {after}")
    sp = row_of(db, "spellpattern", "spellid", SUMMON, ["spellname"])
    if sp is None or sp["spellname"] != "Summon Servant":
        refusals.append(f"spellpattern {SUMMON} is {sp}, not Summon Servant")
    pet = row_of(db, "petstats", "spellid", SUMMON, ["sheet_pet_id", "name"])
    if pet is None or pet != {"sheet_pet_id": "0064", "name": "Undead Servant"}:
        refusals.append(f"petstats {SUMMON} is {pet}, not workbook pet 0064 Undead Servant")
    cm = row_of(db, "cmcatalog", "id", CM, ["name", "add_spells"])
    if cm is None or cm != {"name": "Undead Servitude", "add_spells": str(SUMMON)}:
        refusals.append(f"cmcatalog {CM} is {cm}, not Undead Servitude adding {SUMMON}")
    return ins, ups, refusals, notes


def apply(db, ins, ups):
    db.execute(LOG_SCHEMA)
    for tbl, keycol, row in ins:
        cols = list(row)
        db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [row[c] for c in cols])
        whole = row_of(db, tbl, keycol, row[keycol])
        a = audit(db, "insert", tbl, keycol, row[keycol], None, whole, DETAIL)
        db.execute(f"INSERT INTO {LOG} (action, tbl, keycol, id, before, after, audit) VALUES ('insert', ?, ?, ?, NULL, ?, ?)",
                   (tbl, keycol, row[keycol], json.dumps(whole, sort_keys=True), a))
    for tbl, keycol, id, before, after in ups:
        db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in after)} WHERE {keycol} = ?", list(after.values()) + [id])
        a = audit(db, "update", tbl, keycol, id, before, after, DETAIL)
        db.execute(f"INSERT INTO {LOG} (action, tbl, keycol, id, before, after, audit) VALUES ('update', ?, ?, ?, ?, ?, ?)",
                   (tbl, keycol, id, json.dumps(before, sort_keys=True), json.dumps(after, sort_keys=True), a))


def undo(db):
    """(back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    rows = db.execute(f"SELECT seq, action, tbl, keycol, id, before, after FROM {LOG} ORDER BY seq DESC").fetchall()
    for seq, action, tbl, keycol, id, before, after in rows:
        after = json.loads(after)
        if action == "insert":
            now = row_of(db, tbl, keycol, id)
            if now != after:
                left += 1
                print(f"  left: {tbl} {id}, changed since it was written: {now}")
                continue
            db.execute(f"DELETE FROM {tbl} WHERE {keycol} = ?", (id,))
            audit(db, "delete", tbl, keycol, id, now, None, "undo: " + DETAIL)
        else:
            before = json.loads(before)
            now = row_of(db, tbl, keycol, id, list(after))
            if now != after:
                left += 1
                print(f"  left: {tbl} {id}, changed since it was written: {now}")
                continue
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in before)} WHERE {keycol} = ?",
                       list(before.values()) + [id])
            audit(db, "update", tbl, keycol, id, after, before, "undo: " + DETAIL)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if left == 0:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


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
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"put back {back}, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        ins, ups, refusals, notes = plan(db)
        for r in refusals:
            print("REFUSED:", r)
        if refusals:
            db.execute("ROLLBACK")
            return 1
        for n in notes:
            print(n)
        for tbl, keycol, row in ins:
            print(f"insert {tbl} {row[keycol]}: {row}")
        for tbl, keycol, id, before, after in ups:
            print(f"update {tbl} {id}: {before} -> {after}")
        n = len(ins) + len(ups)
        if n == 0:
            db.execute("ROLLBACK")
            print("applied already: nothing done")
            return 0
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"report only: {n} rows to write (--apply to write)")
            return 0
        apply(db, ins, ups)
        db.execute("COMMIT")
        print(f"applied: {n} rows, in gm_audit and {LOG}")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
