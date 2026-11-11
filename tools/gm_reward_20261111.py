#!/usr/bin/env python3
"""GM's Reward, the record #reward and /reward give (petition 1501, the GM Jibrish, 2026-11-10: "create a GM only
command /reward — this puts a buff on the target that gives them a 25 heal damage shield aka they heal for 25 whenever
hit, and it deals 25 to who hits them. They also gain 100ac, 50 str, 50 dex, 50 wis, 50 int, 50 cha, 50 sta. Lasts 1
hour."), on the server's code of the same branch (handlers/reward.go reads the record by its id and name at each use).

  spellpattern 905121 "GM's Reward" (beside Buffer's GM Armor, 905120, whose fields it takes): level 60, scope 1, no
    cast time, power or recast, Avatar's icon (-213673665, colour 486870879), so it shows in its bearer's effects;
  spelleffect 905121: its workbook effects (spelljson.go) +100 AC, +50 STR, STA, DEX, WIS, INT and CHA, and a
    defensive proc of 25 damage back at every creature's melee hit on its bearer (a damage shield: dshield.go) and 25
    hit points back to the bearer at each (workbook.go's defensive proc heal, the workbook's defensive procs at every
    hit), for 3600 s of play, on a stack type of its own (its id: it stacks with every other buff), GM Armor's visual.

No class, merchant or book has it. Nothing written unless 905121 is free (or already exactly this, then nothing to do).
Report only by default; --apply in one transaction, a gm_audit line a row, log gm_reward_1111_log; --undo exact (a row
changed since is left and said). The record is read at each #reward: no restart needed for the data, only for the code.

    tools/gm_reward_20261111.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "gm_reward_1111_log"
WHO, VIA = "claude", "tool"
DETAIL = "petition 1501 (Jibrish): GM's Reward, the buff #reward gives: tools/gm_reward_20261111.py"
SPELL_ID = 905121
NAME = "GM's Reward"
GM_ARMOR = 905120
AVATAR_ICON, ICON_COLOR = -213673665, 486870879
GM_ARMOR_FX = 497989046

# the record's effects, as spelljson.go reads them (internal/entity/gmreward_test.go reads this line)
EFFECTS = '{"v":1,"scope":"target","target_type":"Friendly","harmful":false,"duration_s":3600,"stack":905121,"level":60,"min_level":1,"req_equipment":"Any","sheet_type":"Statistic Change","arch_type":"GM Reward","effects":[{"id":"GRAC","type":"Statistic Change","kind":"stat","stat":"ac","amount":100},{"id":"GRST","type":"Statistic Change","kind":"stat","stat":"str","amount":50},{"id":"GRSA","type":"Statistic Change","kind":"stat","stat":"sta","amount":50},{"id":"GRDX","type":"Statistic Change","kind":"stat","stat":"dex","amount":50},{"id":"GRWS","type":"Statistic Change","kind":"stat","stat":"wis","amount":50},{"id":"GRIN","type":"Statistic Change","kind":"stat","stat":"int","amount":50},{"id":"GRCH","type":"Statistic Change","kind":"stat","stat":"cha","amount":50},{"id":"GRDP","type":"Defensive Proc","kind":"def_proc","procs":[{"id":"GRDS","type":"Dmg/Heal","kind":"hp","amount":-25,"on":"defense","scope":"target","target_type":"Enemy"},{"id":"GRHL","type":"Dmg/Heal","kind":"hp","amount":25,"on":"defense","scope":"self","target_type":"Self"}]}]}'

PATTERN = {"spellid": SPELL_ID, "abilitylvl": 60, "unk2": 0, "unk3": 0, "range": 16880, "casttime": 0, "power": 0,
           "iconColor": ICON_COLOR, "icon": AVATAR_ICON, "scope": 1, "recast": 0, "eqprequire": -1, "spellname": NAME,
           "spelldesc": "A GM's reward: a damage shield of 25 and a heal of 25 at every hit taken, 100 more AC and 50 more "
                        "STR, STA, DEX, WIS, INT and CHA, for an hour of play."}
EFFECT = {"spellid": SPELL_ID,
          "line": "Target, Damage Shield 25, Heal 25 when hit, Increase AC by 100, Increase STR STA DEX WIS INT CHA by 50, 1hr",
          "fxdict": GM_ARMOR_FX, "source": "petition 1501 (Jibrish): GM's Reward, #reward's buff; made (gm_reward_20261111)",
          "effects": EFFECTS}

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,
    rowkey  TEXT NOT NULL,      -- the row's key (JSON)
    after   TEXT NOT NULL,      -- the row as written (JSON)
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def go_json(m):
    """A row as Go's json.Marshal writes a map (keys sorted, <>& escaped), as the panel's audit has it."""
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def row_of(db, tbl):
    cur = db.execute(f"SELECT * FROM {tbl} WHERE spellid = ?", (SPELL_ID,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def same(row, want):
    return row is not None and all(row.get(k) == v for k, v in want.items())


def audit(db, action, tbl, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json({"spellid": SPELL_ID}),
                      None if before is None else go_json(before), None if after is None else go_json(after), detail))
    return cur.lastrowid


def plan(db):
    """(rows to insert, lines, refusals)."""
    lines, refusals, rows = [], [], []
    armor = db.execute("SELECT spellname FROM spellpattern WHERE spellid = ?", (GM_ARMOR,)).fetchone()
    lines.append(f"spell {GM_ARMOR} is {armor[0]!r}" if armor else f"no spell {GM_ARMOR} (GM Armor) here: GM's Reward is made all the same")
    if not table(db, "gm_audit"):
        refusals.append("no gm_audit table: not a server database")
    for tbl, want in (("spellpattern", PATTERN), ("spelleffect", EFFECT)):
        now = row_of(db, tbl)
        if now is None:
            rows.append((tbl, want))
            lines.append(f"{tbl} {SPELL_ID}: to be written ({NAME})")
        elif same(now, want):
            lines.append(f"{tbl} {SPELL_ID}: already as this tool writes it")
        else:
            refusals.append(f"{tbl} {SPELL_ID} is taken: {json.dumps(now, default=str)[:300]}")
    return rows, lines, refusals


def apply(db, rows):
    db.execute(LOG_SCHEMA)
    for tbl, want in rows:
        cols = list(want)
        db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [want[c] for c in cols])
        row = row_of(db, tbl)
        a = audit(db, "insert", tbl, None, row, DETAIL)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, after, audit) VALUES (?, ?, ?, ?)",
                   (tbl, json.dumps({"spellid": SPELL_ID}), json.dumps(row, sort_keys=True, default=str), a))
    return len(rows)


def undo(db):
    """Each logged row still as written deleted, the last first, its gm_audit line. Returns (back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, tbl, after in db.execute(f"SELECT seq, tbl, after FROM {LOG} ORDER BY seq DESC").fetchall():
        now = row_of(db, tbl)
        if now is None or json.dumps(now, sort_keys=True, default=str) != after:
            left += 1
            print(f"  left: {tbl} {SPELL_ID}, changed since it was written")
            continue
        db.execute(f"DELETE FROM {tbl} WHERE spellid = ?", (SPELL_ID,))
        audit(db, "delete", tbl, now, None, "undo: " + DETAIL)
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
    try:
        if a.undo:
            db.execute("BEGIN IMMEDIATE")
            try:
                back, left = undo(db)
                db.execute("COMMIT")
            except BaseException:
                db.execute("ROLLBACK")
                raise
            print(f"put back {back} rows, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        rows, lines, refusals = plan(db)
        for l in lines:
            print(l)
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not rows:
            print("nothing to do")
            return 0
        if not a.apply:
            print(f"report only: {len(rows)} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            n = apply(db, rows)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {n} rows, each in gm_audit and {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
