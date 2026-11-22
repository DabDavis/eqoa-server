#!/usr/bin/env python3
"""Work board lane 3's data (2026-11-22; a GM's approved asks, the owner's standing rule): the rows the code of entries
693, 1258, 1312 and 1714 needs, in two parts that go in at different moments.

  --part before   1714's one-time migration, ONCE A SHARD, with the server STOPPED (a cold swap's before step), BEFORE the
                  first start of the binary that makes every Ascended record NO TRADE: otherwise that start's sync makes
                  every copy players hold NO TRADE. T is every itempattern record of trade 0 that is Ascended (in
                  itemascended, an itemimported copy of one, or named "Ascended ..."); H the records of T a player holds
                  (charinventory, an open or undelivered auction lot, charbuyback, charmercitem, mail_attachments). Each
                  record of H gets a Legacy record (ids from 967001; database/legacy.go's rule: the record but its name,
                  "Legacy " + the record's, and trade 0; its itemeffect rows copied) and its itemlegacy row, and those
                  holdings are pointed at it. Then every record of T not in itemascended is made trade 1 (the server
                  does the listed ones at its start). Loot tables, pools, shops and quests keep the Ascended records.
  --part after    with or after the new binary (an after.sh, or at once once it runs; nothing here needs a restart):
                  - 693: itemeffect 990988 (Super SoW) "Target, Increase Movement Speed by 260, 30min", where the row
                    reads "Self, Sow, 30min" (an older binary reads a Target line at players as refused);
                  - 1258: the four heroic Guk specials, renumbered off the asked 996055/58/59/62 (taken on the Heirloom
                    shard) to 968055/968058/968059/968062: Guise of the Believer, Powerstone, Bag of Open Kind Eye,
                    Flowing Black Satin Sash, their itemeffect rows and the sash's itemspeed 7 (an older binary would
                    read the bag's "Procs 100 power" as a 100 hit: after the binary). A record whose look record is
                    missing on a shard is left out and said;
                  - 1312: the Fungal Spore Tunic's own table at the asked rates with 1697's x10/9 below heroic Guk's
                    first floor (spawn y under 33): the Froglok King's binding 2.222 -> 2.778, the 20 named 0.5 / 0.556,
                    the trash bound to "dungeon guk named" under 2% (whatever its chance) 0.05 / 0.056. A shard
                    without the King's binding (no heroic Guk) is left alone.

Report only by default; --apply writes each row with a gm_audit line and its before/after in jib_lane3_1122_log;
--undo puts back every row of that part still as this tool left it (refused, nothing done, when one changed since).
A second --apply finds nothing to do.

    tools/jib_lane3_20261122.py --db eqoa.db --part before|after [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "jib_lane3_1122_log"
ACCOUNT, WHO, VIA = 1, "dab", "tool"
DETAIL = "work board %s (a GM's approved ask): tools/jib_lane3_20261122.py"

LEGACY_FIRST, LEGACY_LAST = 967001, 967999
LEGACY_PREFIX = "Legacy "
LEGACY_SOURCE = "a GM's ruling 2026-11-18 (work board 1714): Ascended NO TRADE"
LEGACY_SCHEMA = """CREATE TABLE IF NOT EXISTS itemlegacy (
	patternid INTEGER PRIMARY KEY,
	record    INTEGER NOT NULL UNIQUE,
	made      TEXT NOT NULL DEFAULT '',
	source    TEXT NOT NULL DEFAULT ''
)"""
ITEMSPEED_SCHEMA = """
	CREATE TABLE IF NOT EXISTS itemspeed (
		patternid INTEGER PRIMARY KEY,
		pct       REAL NOT NULL,
		note      TEXT NOT NULL DEFAULT '',
		source    TEXT NOT NULL DEFAULT 'gm'
	)"""

SOW_ID, SOW_OLD, SOW_NEW = 990988, "Self, Sow, 30min", "Target, Increase Movement Speed by 260, 30min"

STATS = ["str", "sta", "agi", "wis", "dex", "cha", "intelligence", "HPMAX", "POWMAX", "PoT", "HoT", "AC",
         "PR", "DR", "FR", "CR", "LR", "AR"]
COMMON = {"levelreq": 60, "rent": 1, "maxstack": 1, "duration": 50, "attacktype": 0, "weapondamage": 0, "unk5": 0,
          "procanim": 0, "craft": 0, "weaponproc": 0, "fish": 0, "raceuse": 1023}
# (id, the asked id, look record, fields, its itemeffect row or None, its itemspeed or None)
GUK_ITEMS = [
    (968055, 996055, 8852, {"itemname": "Guise of the Believer", "equipslot": -1, "classuse": 32767, "trade": 1, "lore": 1,
                            "itemcost": 4235, "maxhp": 12000,
                            "itemdesc": "A guise that shows its bearer to all of Norrath as an Elf."},
     {"name": "Illusion: Elf", "line": "Self, Illusion: Elf, Race Faction, Unlimited Uses, 30min", "body": 0, "charges": 0,
      "cooldown": 0}, None),
    (968058, 996058, 705311, {"itemname": "Powerstone", "equipslot": -1, "classuse": 32672, "trade": 0, "lore": 0,
                              "itemcost": 4235, "maxhp": 12000, "itemdesc": "Trades its user's life for power, without end."},
     {"name": "Powerstone", "line": "Self, Decrease Hitpoints by 300, Increase Power by 100, Unlimited Uses", "body": 0,
      "charges": 0, "cooldown": 6}, None),
    (968059, 996059, 8737, {"itemname": "Bag of Open Kind Eye", "equipslot": 16, "attacktype": 10, "weapondamage": 761,
                            "unk5": 25, "classuse": 32767, "HPMAX": 40, "POWMAX": 40, "dex": 20, "wis": 20,
                            "intelligence": 20, "cha": 20, "trade": 0, "lore": 1, "itemcost": 4792, "maxhp": 10578,
                            "itemdesc": "An open, kindly eye sewn to a bag. Procs 100 power to its wielder, a 10% chance each "
                                        "hit, plus 0.15% for each point of its wielder's highest DEX, INT or WIS (100% at 600)."},
     None, None),
    (968062, 996062, 960012, {"itemname": "Flowing Black Satin Sash", "equipslot": 9, "classuse": 32767, "AC": 30,
                              "str": 25, "sta": 20, "dex": 25, "trade": 0, "lore": 0, "itemcost": 3614, "maxhp": 12000,
                              "itemdesc": "Black satin that hurries its wearer: 7% faster, beside any other speed."},
     None, 7),
]

TUNIC_TABLE = "heroic guk fungal spore tunic"
KING = 166246
TUNIC_NAMED = [165977, 165988, 165992, 165995, 166014, 166029, 166042, 166051, 166057, 166083, 166167, 166170, 166186,
               166188, 166193, 166200, 166215, 166216, 166217, 166239]
FLOOR1_Y = 33  # spawn y at or above: heroic Guk's first floor (entry 1697's cut)
RATES = {"king": (2.5, 2.778), "named": (0.5, 0.556), "trash": (0.05, 0.056)}  # (first floor, below it)


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def go_json(v):
    return json.dumps(v, sort_keys=True, separators=(",", ":")) if v is not None else None


def has_table(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type='table' AND name=?", (t,)).fetchone() is not None


def cols(db, t):
    return [r[1] for r in db.execute(f"PRAGMA table_info({t})")]


def get(db, tbl, key):
    if not has_table(db, tbl):
        return None
    db.row_factory = sqlite3.Row
    r = db.execute(f"SELECT * FROM {tbl} WHERE " + " AND ".join(f'"{k}" = ?' for k in key), list(key.values())).fetchone()
    db.row_factory = None
    return dict(r) if r else None


def ensure_log(db):
    db.execute(f"""CREATE TABLE IF NOT EXISTS {LOG} (seq INTEGER PRIMARY KEY AUTOINCREMENT, part TEXT NOT NULL,
                   entry TEXT NOT NULL, tbl TEXT NOT NULL, pk TEXT NOT NULL, before TEXT, after TEXT,
                   at TEXT NOT NULL, undone TEXT NOT NULL DEFAULT '')""")


class Writer:
    """Every row change with its gm_audit line and its log row."""

    def __init__(self, db, part):
        self.db, self.part, self.n = db, part, 0

    def put(self, entry, tbl, key, after):
        """Make the row of key after (a dict of every column; None deletes it)."""
        before = get(self.db, tbl, key)
        if before == after:
            return False
        if before is not None:
            self.db.execute(f"DELETE FROM {tbl} WHERE " + " AND ".join(f'"{k}" = ?' for k in key), list(key.values()))
        if after is not None:
            names = list(after)
            self.db.execute(f"INSERT INTO {tbl} ({', '.join(chr(34) + c + chr(34) for c in names)}) VALUES "
                            f"({', '.join('?' for _ in names)})", [after[c] for c in names])
            after = get(self.db, tbl, key)  # as stored (a REAL column's 6 reads 6.0): what --undo compares
            if before == after:
                return False
        action = "insert" if before is None else "delete" if after is None else "update"
        at = now()
        self.db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                        "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                        (at, ACCOUNT, WHO, VIA, action, tbl, go_json(key), go_json(before), go_json(after), DETAIL % entry))
        self.db.execute(f"INSERT INTO {LOG} (part, entry, tbl, pk, before, after, at) VALUES (?, ?, ?, ?, ?, ?, ?)",
                        (self.part, entry, tbl, go_json(key), go_json(before), go_json(after), at))
        self.n += 1
        return True

    def update(self, entry, tbl, key, **fields):
        row = get(self.db, tbl, key)
        if row is None:
            return False
        new = dict(row)
        new.update(fields)
        return self.put(entry, tbl, key, new)


# ---- part before: 1714 ------------------------------------------------------------------------------------------------

HOLDINGS = [  # (table, its key column, the rows that hold an item)
    ("charinventory", "itemid", "1"),
    ("auction_lots", "lot", "(state = 'open' OR delivered = 0)"),
    ("charbuyback", "id", "1"),
    ("charmercitem", "id", "1"),
    ("mail_attachments", "id", "1"),
]


def ascended_t(db):
    """T: every record of trade 0 that is Ascended."""
    imported = "OR patternid IN (SELECT patternid FROM itemimported WHERE base IN (SELECT patternid FROM itemascended))" \
        if has_table(db, "itemimported") else ""
    return [r[0] for r in db.execute(f"""SELECT patternid FROM itempattern WHERE trade = 0 AND (
        patternid IN (SELECT patternid FROM itemascended) {imported} OR itemname LIKE 'Ascended %') ORDER BY patternid""")]


def part_before(db, w, out):
    if not has_table(db, "itemascended"):
        out.append("1714: no itemascended table: nothing to do")
        return
    db.execute(LEGACY_SCHEMA)
    done = {r[0]: r[1] for r in db.execute("SELECT record, patternid FROM itemlegacy")}
    t = ascended_t(db)
    tset = set(t)
    held = {}
    for tbl, key, where in HOLDINGS:
        if not has_table(db, tbl):
            continue
        for k, pid in db.execute(f"SELECT {key}, patternid FROM {tbl} WHERE {where} AND patternid IN "
                                 f"(SELECT patternid FROM itempattern WHERE trade = 0)"):
            if pid in tset:
                held.setdefault(pid, []).append((tbl, key, k))
    listed = {r[0] for r in db.execute("SELECT patternid FROM itemascended")}
    icols = cols(db, "itempattern")
    ecols = cols(db, "itemeffect") if has_table(db, "itemeffect") else []
    nid = max([LEGACY_FIRST - 1] + [r[0] for r in db.execute("SELECT patternid FROM itempattern WHERE patternid BETWEEN ? AND ?",
                                                             (LEGACY_FIRST, LEGACY_LAST))]) + 1
    made = moved = 0
    for pid in sorted(held):
        lid = done.get(pid)
        if lid is None:
            if nid > LEGACY_LAST:
                sys.exit(f"no Legacy id left ({LEGACY_FIRST}-{LEGACY_LAST}): nothing done")
            lid, nid = nid, nid + 1
        rec = get(db, "itempattern", {"patternid": pid})
        leg = {c: rec[c] for c in icols}
        leg["patternid"], leg["itemname"], leg["trade"] = lid, LEGACY_PREFIX + rec["itemname"].strip(), 0
        if len(leg["itemname"]) > 63:
            sys.exit(f"record {pid}: {leg['itemname']!r} is longer than the client shows: nothing done")
        w.put("1714", "itempattern", {"patternid": lid}, leg)
        if ecols:
            db.row_factory = sqlite3.Row
            effects = [dict(r) for r in db.execute("SELECT * FROM itemeffect WHERE patternid = ?", (pid,))]
            db.row_factory = None
            for e in effects:  # itemeffect is keyed by patternid (one row an item)
                e["patternid"] = lid
                w.put("1714", "itemeffect", {"patternid": lid}, e)
        w.put("1714", "itemlegacy", {"patternid": lid},
              {"patternid": lid, "record": pid, "made": now(), "source": LEGACY_SOURCE})
        made += 1
        for tbl, key, k in held[pid]:
            fields = {"patternid": lid}
            if tbl == "mail_attachments":
                fields["itemname"] = leg["itemname"]
            w.update("1714", tbl, {key: k}, **fields)
            moved += 1
    traded = 0
    for pid in t:
        if pid not in listed and w.update("1714", "itempattern", {"patternid": pid}, trade=1):
            traded += 1
    by = {}
    for pid in held:
        for tbl, _, _ in held[pid]:
            by[tbl] = by.get(tbl, 0) + 1
    out.append(f"1714: T {len(t)} records ({len(tset & listed)} in itemascended); H {len(held)} held: {made} Legacy records "
               f"(ids {LEGACY_FIRST}-), {moved} holdings moved ({by}); {traded} unlisted records of T made NO TRADE")


# ---- part after: 693, 1258, 1312 --------------------------------------------------------------------------------------

def part_after(db, w, out):
    # 693
    row = get(db, "itemeffect", {"patternid": SOW_ID})
    if row is None:
        out.append(f"693: no itemeffect {SOW_ID} on this shard: left alone")
    elif row["line"] == SOW_NEW:
        out.append("693: in place already")
    elif row["line"] != SOW_OLD:
        out.append(f"693: itemeffect {SOW_ID} reads {row['line']!r}, not {SOW_OLD!r} (changed since?): left alone")
    else:
        w.update("693", "itemeffect", {"patternid": SOW_ID}, line=SOW_NEW)
        out.append(f"693: itemeffect {SOW_ID} line -> {SOW_NEW!r}")

    # 1258
    icols = cols(db, "itempattern")
    ecols = cols(db, "itemeffect")
    db.execute(ITEMSPEED_SCHEMA)  # in the transaction (executescript would commit it)
    for pid, asked, look, fields, effect, speed in GUK_ITEMS:
        src = get(db, "itempattern", {"patternid": look})
        there = get(db, "itempattern", {"patternid": pid})
        if src is None:
            out.append(f"1258: {fields['itemname']} ({pid}): look record {look} missing on this shard: left out")
            continue
        if there is not None and there["itemname"] != fields["itemname"]:
            sys.exit(f"1258: itempattern {pid} is {there['itemname']!r}: nothing done")
        rec = {c: src[c] for c in icols}
        for c in STATS:
            if c in rec:
                rec[c] = 0
        rec.update(COMMON)
        rec.update(fields)
        rec["patternid"] = pid
        w.put("1258", "itempattern", {"patternid": pid}, rec)
        if effect is not None:
            srce = get(db, "itemeffect", {"patternid": look})
            e = {c: None for c in ecols}
            e.update({"patternid": pid, "icon": srce["icon"] if srce else 0, "breaks": "", "wear": 0, "area": 0.0,
                      "source": "work board 1258", "target_line": ""})
            e.update(effect)
            w.put("1258", "itemeffect", {"patternid": pid}, {c: e[c] for c in ecols})
        if speed is not None:
            w.put("1258", "itemspeed", {"patternid": pid},
                  {"patternid": pid, "pct": float(speed), "note": fields["itemname"], "source": "work board 1258"})
        out.append(f"1258: {fields['itemname']} {pid} (asked {asked}), look of {look}" +
                   (f", itemspeed {speed}" if speed else "") + (", its use" if effect else ""))

    # 1312
    king = get(db, "npcloot", {"npc_name": f"#{KING}", "tablename": TUNIC_TABLE})
    if king is None:
        out.append(f"1312: no {TUNIC_TABLE!r} binding on the King #{KING}: left alone (no heroic Guk)")
        return

    def rate(kind, row_id):
        y = db.execute("SELECT y_coord FROM npcs WHERE id = ?", (row_id,)).fetchone()
        if y is None:
            return None
        lo, hi = RATES[kind]
        return lo if y[0] >= FLOOR1_Y else hi

    def bind(row_id, kind, note):
        r = rate(kind, row_id)
        if r is None:
            return 0
        key = {"npc_name": f"#{row_id}", "tablename": TUNIC_TABLE}
        cur = get(db, "npcloot", key)
        if cur is None:
            w.put("1312", "npcloot", key, {"npc_name": f"#{row_id}", "tablename": TUNIC_TABLE, "chance": r, "rolls": 1,
                                          "note": note, "source": "work board 1312"})
        else:
            w.update("1312", "npcloot", key, chance=r)
        return 1

    n_king = bind(KING, "king", "the Froglok King")
    n_named = sum(bind(i, "named", "a heroic Guk named") for i in TUNIC_NAMED)
    skip = {KING} | set(TUNIC_NAMED)
    trash = [int(r[0][1:]) for r in db.execute("""SELECT l.npc_name FROM npcloot l JOIN npcs n ON '#' || n.id = l.npc_name
        WHERE l.tablename = 'dungeon guk named' AND l.chance < 2 AND n.world = 14 ORDER BY n.id""")]
    trash = [i for i in trash if i not in skip]
    n_trash = sum(bind(i, "trash", "heroic Guk trash") for i in trash)
    out.append(f"1312: {TUNIC_TABLE!r}: the King {n_king}, named {n_named} of {len(TUNIC_NAMED)}, trash {n_trash} "
               f"(rates {RATES}; below y {FLOOR1_Y} the x10/9)")


# ---- undo -------------------------------------------------------------------------------------------------------------

def undo(db, part):
    rows = db.execute(f"SELECT seq, tbl, pk, before, after FROM {LOG} WHERE part = ? AND undone = '' ORDER BY seq DESC",
                      (part,)).fetchall()
    if not rows:
        sys.exit("nothing applied: nothing done")
    for seq, tbl, pk, before, after in rows:
        cur = get(db, tbl, json.loads(pk))
        if go_json(cur) != after:
            sys.exit(f"{tbl} {pk} changed since this tool wrote it: nothing done")
    w = Writer(db, part + "-undo")
    at = now()
    for seq, tbl, pk, before, after in rows:
        w.put("undo", tbl, json.loads(pk), json.loads(before) if before else None)
        db.execute(f"UPDATE {LOG} SET undone = ? WHERE seq = ?", (at, seq))
    return len(rows)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--db", required=True)
    ap.add_argument("--part", required=True, choices=["before", "after"])
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, timeout=5, isolation_level=None)
    db.execute("BEGIN IMMEDIATE")
    try:
        ensure_log(db)
        if a.undo:
            n = undo(db, a.part)
            db.execute("COMMIT")
            print(f"undone: {n} rows")
            return
        w = Writer(db, a.part)
        out = []
        (part_before if a.part == "before" else part_after)(db, w, out)
        print("\n".join(out))
        print(f"{w.n} rows")
        if a.apply:
            db.execute("COMMIT")
            print("applied")
        else:
            db.execute("ROLLBACK")
            print("(report only: --apply writes it)")
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    main()
