#!/usr/bin/env python3
"""The Mermaid, race 32 of the Heirloom Test Server (petition 1620, the GM Jibrish, 2026-11-11: "Add mermaid as a
playable race to the game, all classes, using the new races starting area. It works functionally the same as the
others. ... Name the race Mermaid though, not siren."), as the Jal'Raeth and the Wisp were made
(claude-notes/races-30-31-20261109.md):

  racedefs        32 Mermaid, MER, passing as an Erudite (8: Highbourne, the Erudites' city on the coast across the sea from Odus, which the
                  merfolk of Erud's Crossing swim to; the Erudite's camera, 1.94, as tall as a mermaid drawn at 0.75);
                  STR 0 STA 5 AGI 15 DEX 0 WIS 5 INT 0 CHA 10 (35, as every race's); body_male the merman's
                  0xA316EB78, body_female the mermaid's 0x74BA7C3D (CHAR.ESF, by sight; the two sirens of Odus,
                  0xBF21FEF5 and 0x1740427C, are her options: go-eqoa-server entity/raceoptions.go)
  racesize        32 at 0.75: the merfolk float upright with their tails under them, the mermaid's head at 2.6 at her own
                  scale (the merman's at 3.15, which his option step brings to hers); at 0.75 she stands 1.97
  defaultcharacter 15 rows, one a class: the Erudite's start spot of the class, else its first, with the race's and the
                  class's stats (tools/seed_race_homes.py moves them to the Mermaid's home in world 10 after this)
  startitems      the Erudite's combo of the class (else its items but the weapon, plus the class's weapon), plus the
                  heirloom Genie's Lamp, Fang and Summoning Portal
  factionstart    the Erudite's rows

Refused, nothing written: a database with no heirloom records (the Live Server's), a body of the race's in
charactermodel (every creature of that body would go friendly: hostileNatureLocked), a racedefs, racesize or
defaultcharacter row of race 32 that is not this tool's. Report only by default; --apply in one transaction, a
gm_audit line a row, log mermaid_race_1111_log; --undo exact (a row changed since is left and said). Every table here
is read at the server's start: a keep restart's before.sh, with the binary that knows the Mermaid's options and swims
its body (go-eqoa-server branch mermaid1620). Then tools/seed_race_homes.py and the homelands quests (the README).

    tools/mermaid_race_20261111.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "mermaid_race_1111_log"
DETAIL = "The Mermaid, race 32 (petition 1620, Jibrish): tools/mermaid_race_20261111.py"
WHO, VIA = "dab", "tool"
SOURCE = "mermaid1620"

RACE, NAME, SHORT, WIRE, CITY = 32, "Mermaid", "MER", 8, 0
BONUS = (0, 5, 15, 0, 5, 0, 10)  # STR STA AGI DEX WIS INT CHA
MALE, FEMALE = 0xA316EB78, 0x74BA7C3D
SIZE = 0.75
DESCRIPTION = ("Merfolk are the sea's own people, who swim the warm waters of Erud's Crossing in the Homelands; Highbourne's scholars take them in.")
NOTE = ("the merman's and the mermaid's bodies (CHAR.ESF, by sight; the sirens of Odus her options); as an Erudite: "
        "Highbourne, across Erud's Crossing from the merfolk's waters off Odus; bonuses ours, 35 as every race's")
SIZE_NOTE = "the merfolk float upright, tails under them: the mermaid 2.63 tall at her scale, 1.97 at 0.75 (petition 1620)"
KEPT = (715000, 715010, 715020)  # Genie's Lamp, Heirloom Fang of the Wolf, Summoning Portal
# each class's bonuses (entity/stats.go classBonus): a start row's default stats are 60 + the race's + the class's
CLASS_BONUS = [(10, 15, 0, 0, -5, -10, -10), (0, 0, 10, 15, -5, -10, -10), (10, 15, 0, 0, -5, -10, -10), (10, 15, 0, 0, -10, -5, -10),
               (0, 10, 0, 15, -5, -10, -10), (-5, 10, 15, 0, -10, -10, 0), (0, 0, 10, 15, -10, -10, -5), (0, -10, -10, 10, 15, -5, 0),
               (-10, 10, 0, -5, 15, -10, 0), (0, 0, -5, -10, 15, -10, 10), (-10, 0, 10, -5, 0, 15, -10), (0, 10, -10, 0, -5, 15, -10),
               (-10, -10, 0, 0, -5, 15, 10), (0, -5, 0, 10, -10, 15, -10), (-10, 0, -5, 0, 10, 15, -10)]
STAT_COLS = ("defaultStrength", "defaultStamina", "defaultAgility", "defaultDexterity", "defaultWisdom", "defaultIntelligence",
             "defaultCharisma")
KEYS = {"racedefs": "id", "racesize": "race", "defaultcharacter": "defaultCharacterID", "startitems": "id", "factionstart": "id"}
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,      -- racedefs, racesize, defaultcharacter, startitems or factionstart
    rowkey  INTEGER NOT NULL,   -- its key (KEYS)
    before  TEXT,               -- the row before (JSON), NULL when written new
    after   TEXT NOT NULL,      -- the row as written (JSON)
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def s32(v):
    """A DictID as characters.modelid and racedefs hold it: signed 32 bits."""
    return v - (1 << 32) if v >= 1 << 31 else v


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def row_of(db, tbl, key):
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {KEYS[tbl]} = ?", (key,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def rowdict(cur):
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, tbl, key, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json({KEYS[tbl]: key}),
                      None if before is None else go_json(before), None if after is None else go_json(after), detail))
    return cur.lastrowid


def racedef_row():
    return {"id": RACE, "name": NAME, "short_name": SHORT, "wire_race": WIRE, "bonus_str": BONUS[0], "bonus_sta": BONUS[1],
            "bonus_agi": BONUS[2], "bonus_dex": BONUS[3], "bonus_wis": BONUS[4], "bonus_int": BONUS[5], "bonus_cha": BONUS[6],
            "body_male": s32(MALE), "body_female": s32(FEMALE), "note": NOTE, "source": SOURCE, "description": DESCRIPTION,
            "class_mask": 0, "city": CITY}


def start_row(db, cls):
    """The start spot the server takes for a race passing as WIRE (database.startPlace): its row of the class, else its
    first (the lowest class)."""
    return rowdict(db.execute("""SELECT * FROM defaultcharacter WHERE race = ?
        ORDER BY humType = ? DESC, tclass = ? DESC, humType, tclass LIMIT 1""", (WIRE, CITY, cls)))


def start_items(db, cls):
    """database.StartItems for the wire race's combo: its rows, else its items but their weapon and the class's weapon
    (the one most combos of the class carry; a monk none)."""
    rows = db.execute("SELECT patternid, count FROM startitems WHERE race = ? AND class = ? AND humtype = 0 ORDER BY id", (WIRE, cls)).fetchall()
    if rows:
        return [tuple(r) for r in rows]
    first = db.execute("SELECT class, humtype FROM startitems WHERE race = ? ORDER BY humtype, class LIMIT 1", (WIRE,)).fetchone()
    if not first:
        return []
    out = [tuple(r) for r in db.execute("""SELECT s.patternid, s.count FROM startitems s JOIN itempattern p ON p.patternid = s.patternid
        WHERE s.race = ? AND s.class = ? AND s.humtype = ? AND p.attacktype = 0 ORDER BY s.id""", (WIRE, first[0], first[1]))]
    w = db.execute("""SELECT s.patternid FROM startitems s JOIN itempattern p ON p.patternid = s.patternid
        WHERE s.class = ? AND p.attacktype != 0 GROUP BY s.patternid ORDER BY COUNT(*) DESC, s.patternid LIMIT 1""", (cls,)).fetchone()
    if w:
        out.append((w[0], 1))
    return out


def rows_to_write(db):
    """{table: [row, ...]} as --apply writes them (no keys for the added rows of defaultcharacter, startitems,
    factionstart)."""
    out = {"racedefs": [racedef_row()], "racesize": [{"race": RACE, "size": SIZE, "note": SIZE_NOTE, "source": SOURCE}],
           "defaultcharacter": [], "startitems": [], "factionstart": []}
    for cls in range(15):
        src = start_row(db, cls)
        if src is None:
            return None
        src = dict(src)
        src.pop("defaultCharacterID")
        src.update(race=RACE, tclass=cls)
        for i, c in enumerate(STAT_COLS):
            src[c] = 60 + BONUS[i] + CLASS_BONUS[cls][i]
        out["defaultcharacter"].append(src)
        items = start_items(db, cls)
        if not any(p in KEPT for p, _ in items):
            items += [(p, 1) for p in KEPT]
        for p, n in items:
            out["startitems"].append({"race": RACE, "class": cls, "humtype": 0, "patternid": p, "count": n,
                                      "note": f"{NAME}: its wire race's ({WIRE}) for the class", "source": SOURCE})
    for cls, faction, standing in db.execute("SELECT class, faction, standing FROM factionstart WHERE race = ? ORDER BY id", (WIRE,)):
        out["factionstart"].append({"race": RACE, "class": cls, "humtype": -1, "faction": faction, "standing": standing,
                                    "note": f"{NAME}: its wire race's ({WIRE}) start", "source": SOURCE})
    return out


def logged(db):
    return table(db, LOG) and db.execute(f"SELECT COUNT(*) FROM {LOG}").fetchone()[0] > 0


def applied_as_written(db):
    """Every row the log holds is as it was written."""
    for tbl, key, after in db.execute(f"SELECT tbl, rowkey, after FROM {LOG}").fetchall():
        now = row_of(db, tbl, key)
        if now is None or json.dumps(now, sort_keys=True) != after:
            return False
    return True


def plan(db):
    """(lines, refusals, applied)."""
    for t in ("racedefs", "racesize", "defaultcharacter", "startitems", "factionstart", "itempattern", "gm_audit", "charactermodel"):
        if not table(db, t):
            return [], [f"no {t} table here"], False
    if logged(db):
        return [], [] if applied_as_written(db) else [f"{LOG} is there but the rows are not as written: --undo first"], applied_as_written(db)
    refusals = []
    if not table(db, "heirlooms") or db.execute("SELECT COUNT(*) FROM heirlooms").fetchone()[0] == 0:
        refusals.append("no heirloom records: not the Heirloom Test Server's database")
    bodies = (s32(MALE), s32(FEMALE), MALE, FEMALE)
    if inmodel := db.execute("SELECT modelid FROM charactermodel WHERE modelid IN (?, ?, ?, ?)", bodies).fetchall():
        refusals.append(f"bodies in charactermodel ({inmodel}): every creature of them would go friendly")
    for tbl, where in (("racedefs", "id"), ("racesize", "race"), ("defaultcharacter", "race"), ("startitems", "race"),
                       ("factionstart", "race")):
        if n := db.execute(f"SELECT COUNT(*) FROM {tbl} WHERE {where} = ?", (RACE,)).fetchone()[0]:
            refusals.append(f"{tbl} holds {n} row(s) of race {RACE} already (not this tool's)")
    if db.execute("SELECT COUNT(*) FROM racedefs WHERE name = ? COLLATE NOCASE", (NAME,)).fetchone()[0]:
        refusals.append(f"another race is named {NAME}")
    rows = rows_to_write(db)
    if rows is None:
        return [], refusals + [f"race {WIRE} has no start row to copy"], False
    lines = [f"racedefs {RACE} {NAME} ({SHORT}): wire race {WIRE}, bonuses {BONUS}, bodies {MALE:08X} / {FEMALE:08X}",
             f"racesize {RACE}: {SIZE}"]
    for t in ("defaultcharacter", "startitems", "factionstart"):
        lines.append(f"{t}: {len(rows[t])} rows new")
    return lines, refusals, False


def log(db, tbl, key, before, after, action):
    a = audit(db, action, tbl, key, before, after, DETAIL)
    db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
               (tbl, key, None if before is None else json.dumps(before, sort_keys=True), json.dumps(after, sort_keys=True), a))


def insert(db, tbl, row):
    cols = list(row)
    cur = db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [row[c] for c in cols])
    return cur.lastrowid


def apply(db):
    db.execute(LOG_SCHEMA)
    n = 0
    for tbl, rows in rows_to_write(db).items():
        for row in rows:
            rid = insert(db, tbl, row)
            key = row.get(KEYS[tbl], rid)
            log(db, tbl, key, None, row_of(db, tbl, key), "insert")
            n += 1
    return n


def undo(db):
    """(back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, tbl, key, before, after in db.execute(f"SELECT seq, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        now = row_of(db, tbl, key)
        if now is None or json.dumps(now, sort_keys=True) != after:
            left += 1
            print(f"  left: {tbl} {key}, changed since it was written: {now}")
            continue
        if before is None:
            db.execute(f"DELETE FROM {tbl} WHERE {KEYS[tbl]} = ?", (key,))
            audit(db, "delete", tbl, key, now, None, "undo: " + DETAIL)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if left == 0:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
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
            print(f"put back {back}, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        lines, refusals, applied = plan(db)
        for line in lines:
            print(line)
        if applied:
            print(f"applied already (race {RACE} is this tool's): nothing done")
            return 0
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not a.apply:
            print("report only (--apply writes it)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            n = apply(db)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {n} rows; in gm_audit and {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
