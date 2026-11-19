#!/usr/bin/env python3
"""The Automatic Snowball Launcher: one record, Zerkziee's alone, given into his pack (the owner, 2026-11-16: "lets give
zerkziee an automatic snowball launcher, infinite ammo").

1. The record: itempattern LAUNCHER (715090, beside the Barrel of Monkeys 715080), "Automatic Snowball Launcher": worn
   nowhere (equipslot -1), CLIENT's usable type (procanim 2, as the barrel: the item menu offers Use and Prepare onto a
   potion quick slot), LORE (one), NO TRADE (trade 1), kept over logout (rent 1), worth nothing, never spent (infinite
   ammo: the server's Use never takes a charge), any class, race and level, the Machinist's Crossbow's icon (ICON: a
   crossbow, a tinker's ranged piece; ITEMICON.ESF has no launcher). Its description says what it does, at most DESC_MAX
   plain characters with no '|'.
2. Only he may use it: table itemowner (database/itemowner.go) binds the record to his serverid; the server
   (handlers/snowlauncher.go, branch snowball-launcher) refuses anyone else's Use of a copy ("The launcher's gears lock
   up: it was built for someone else."). A server WITHOUT that branch does nothing with it (an unknown usable item);
   NO TRADE still holds.
3. --give: one into his pack (location 0, unequipped, at the end of the list, as the GM panel's give does offline),
   unless he holds one already. Run it only while he is OFFLINE (the row reaches him at his next login); while he is
   online give it by the GM panel's give (patternid 715090), which sends it at once.

The character is found by name (Zerkziee) and must be account 220, else REFUSED and nothing written (Heirloom: serverid
747, L60 gnome ranger; Live has a Zerkziee of the same account, serverid 93, L7, should the owner want it there too).
Checked too: the id free (or already this record), the binding free (or already his), tables itempattern, characters,
charinventory and gm_audit, and with --give his pack's room. Report by default; --apply in one transaction, a gm_audit
line a row (as the panel's), logged in snowball_launcher_1116_log; a second --apply finds nothing to do; --undo puts
back exactly what was there (a row changed since is left and listed; the record and its binding are left while anyone
still holds a copy).

    python3 -E -s tools/snowball_launcher_20261116.py --db eqoa.db [--give] [--apply | --undo] [--pack-slots 100]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "snowball_launcher_1116_log"
DETAIL = "Zerkziee's Automatic Snowball Launcher, his alone (the owner, 2026-11-16); tools/snowball_launcher_20261116.py"
WHO, VIA = "dab", "tool"
NAME, ACCOUNT = "Zerkziee", 220
LAUNCHER, ITEM_NAME = 715090, "Automatic Snowball Launcher"
ICON = 544383080  # the Machinist's Crossbow's (Adorned, Eagle, Stonewood, ... crossbows): a tinker's ranged piece
USABLE = 2  # CLIENT's item type of the potions and food (database.ItemTypeUsable)
DESC_MAX = 200
DESC = ("A gnomish contraption of springs, cogs and a bottomless hopper of snow. Use it to switch automatic fire on "
        "or off: it pelts your target. Never runs dry. Built for Zerkziee alone.")

OWNER_DDL = """CREATE TABLE IF NOT EXISTS itemowner (
		patternid INTEGER PRIMARY KEY,
		serverid  INTEGER NOT NULL,
		note      TEXT NOT NULL DEFAULT ''
	)"""  # database/itemowner.go's schema

LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    rowkey TEXT NOT NULL,      -- the key (JSON)
    before TEXT,               -- the row before (JSON), NULL none
    after  TEXT NOT NULL,      -- the row written (JSON)
    audit  INTEGER NOT NULL
)"""


def record():
    return {"itemcost": 0, "unk1": 0, "itemicon": ICON, "unk2": 0, "equipslot": -1, "unk3": 0, "trade": 1, "rent": 1,
            "unk4": 0, "attacktype": 0, "weapondamage": 0, "unk5": 0, "levelreq": 1, "maxstack": 1, "maxhp": 0,
            "duration": 80, "classuse": 32767, "raceuse": 32767, "procanim": USABLE, "lore": 1, "unk6": 0, "craft": 0,
            "itemname": ITEM_NAME, "itemdesc": DESC, "model": 0, "color": 0xFFFFFFFF,
            "str": 0, "sta": 0, "agi": 0, "wis": 0, "dex": 0, "cha": 0, "intelligence": 0, "HPMAX": 0, "POWMAX": 0,
            "PoT": 0, "HoT": 0, "AC": 0, "PR": 0, "DR": 0, "FR": 0, "CR": 0, "LR": 0, "AR": 0, "weaponproc": 0, "fish": 0}


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def row(db, tbl, key):
    where = " AND ".join(f"{k} = ?" for k in key)
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", tuple(key.values()))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return None if m is None else json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, tbl, key, action, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key), go_json(before), go_json(after), DETAIL))
    return cur.lastrowid


def put(db, tbl, key, want):
    """tbl's row of key made want (None: deleted); (key, before, after, audit id). A key of None inserts a new row (its
    autoincrement itemid the key)."""
    if key is None:
        cur = db.execute(f"INSERT INTO {tbl} ({', '.join(want)}) VALUES ({', '.join('?' * len(want))})", tuple(want.values()))
        key = {"itemid": cur.lastrowid}
        before, action = None, "insert"
    else:
        before = row(db, tbl, key)
        where = " AND ".join(f"{k} = ?" for k in key)
        if want is None:
            db.execute(f"DELETE FROM {tbl} WHERE {where}", tuple(key.values()))
            action = "delete"
        elif before is None:
            cols = dict(key, **want)
            db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", tuple(cols.values()))
            action = "insert"
        else:
            sets = {k: v for k, v in want.items() if k not in key}
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{k} = ?' for k in sets)} WHERE {where}",
                       tuple(sets.values()) + tuple(key.values()))
            action = "update"
    after = row(db, tbl, key)
    return key, before, after, audit(db, tbl, key, action, before, after)


def plan(db, give, pack_slots):
    """(writes [(tbl, key, want)], lines, refusals)."""
    writes, lines, refusals = [], [], []
    for t in ("itempattern", "characters", "charinventory", "gm_audit"):
        if not table(db, t):
            refusals.append(f"no {t} table here")
    if refusals:
        return writes, lines, refusals
    if len(DESC) > DESC_MAX or "|" in DESC or not DESC.isascii():
        refusals.append(f"the description is {len(DESC)} characters (at most {DESC_MAX}, ASCII, no '|')")
    found = db.execute("SELECT serverid, accountid, race, tclass, level FROM characters WHERE charName = ?", (NAME,)).fetchall()
    if len(found) != 1:
        return writes, lines, [f"{len(found)} characters named {NAME}, not one"]
    sid, acct, race, cls, level = found[0]
    if acct != ACCOUNT:
        return writes, lines, [f"{NAME} (serverid {sid}) is account {acct}'s, not {ACCOUNT}'s"]
    lines.append(f"  {NAME}: serverid {sid}, account {acct}, race {race}, class {cls}, level {level}")

    want = record()
    cur = row(db, "itempattern", {"patternid": LAUNCHER})
    if cur is not None and cur["itemname"] != ITEM_NAME:
        refusals.append(f"itempattern {LAUNCHER} is {cur['itemname']!r}, not free")
    else:
        others = db.execute("SELECT patternid FROM itempattern WHERE lower(trim(itemname)) = lower(?) AND patternid != ?",
                            (ITEM_NAME, LAUNCHER)).fetchall()
        if others:
            refusals.append(f"other records named {ITEM_NAME!r}: {[o[0] for o in others]} (the server knows the launcher by its name)")
        if cur is None or {k: cur[k] for k in want} != want:
            writes.append(("itempattern", {"patternid": LAUNCHER}, want))
            lines.append(f"  itempattern {LAUNCHER} {ITEM_NAME}: " + ("new" if cur is None else "made as below") +
                         f"; usable (procanim {USABLE}), worn nowhere, LORE, NO TRADE, worth 0, icon {ICON}")
        else:
            lines.append(f"  itempattern {LAUNCHER} {ITEM_NAME}: as wanted already")

    owner_rows = table(db, "itemowner")
    if not owner_rows:
        writes.append(("ddl", None, OWNER_DDL))
        lines.append("  table itemowner made (database/itemowner.go's schema)")
    b = row(db, "itemowner", {"patternid": LAUNCHER}) if owner_rows else None
    if b is not None and b["serverid"] != sid:
        refusals.append(f"itemowner {LAUNCHER} binds it to serverid {b['serverid']}, not {sid}")
    elif b is None:
        writes.append(("itemowner", {"patternid": LAUNCHER}, {"serverid": sid, "note": f"{ITEM_NAME}: {NAME}'s alone, the owner 2026-11-16"}))
        lines.append(f"  itemowner {LAUNCHER}: used by serverid {sid} ({NAME}) alone")
    else:
        lines.append(f"  itemowner {LAUNCHER}: his already")

    held = db.execute("SELECT c.charName, i.serverid, i.location FROM charinventory i LEFT JOIN characters c USING (serverid) "
                      "WHERE i.patternid = ?", (LAUNCHER,)).fetchall()
    lines.append(f"  held by: " + (", ".join(f"{n or '?'} ({s}, location {l})" for n, s, l in held) or "no one"))
    if give:
        if any(s == sid for _, s, _ in held):
            lines.append(f"  --give: {NAME} holds one already")
        else:
            n = db.execute("SELECT COUNT(*) FROM charinventory WHERE serverid = ? AND location = 0", (sid,)).fetchone()[0]
            if n + 1 > pack_slots:
                refusals.append(f"his pack holds {n} of {pack_slots}: no room")
            nxt = db.execute("SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory WHERE serverid = ? AND location = 0",
                             (sid,)).fetchone()[0]
            writes.append(("charinventory", None, {"serverid": sid, "stackleft": 1, "remainHP": 0, "remaincharge": 0,
                                                    "patternid": LAUNCHER, "equiploc": -1, "location": 0, "listnumber": nxt}))
            lines.append(f"  charinventory: {ITEM_NAME} into his pack, list number {nxt} ({n} -> {n + 1} of {pack_slots})")
    return writes, lines, refusals


def apply(db, writes):
    db.execute(LOG_DDL)
    for tbl, key, want in writes:
        if tbl == "ddl":
            db.execute(want)
            n = audit(db, "itemowner", {}, "create", None, {"sql": want})
            db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES ('ddl', '{{}}', NULL, ?, ?)", (want, n))
            continue
        key, before, after, n = put(db, tbl, key, want)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                   (tbl, go_json(key), go_json(before), go_json(after), n))
    return len(writes)


def undo(db):
    """Every logged write put back, last first. The record and its binding are left while anyone holds a copy, as is
    a row changed since."""
    back, left = 0, []
    if not table(db, LOG):
        return back, left
    for seq, tbl, rowkey, before, after in db.execute(f"SELECT seq, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        if tbl == "ddl":
            if db.execute("SELECT COUNT(*) FROM itemowner").fetchone()[0]:
                left.append("table itemowner: holds rows, left")
                continue
            db.execute("DROP TABLE itemowner")
            audit(db, "itemowner", {}, "drop", {"sql": after}, None)
            db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
            back += 1
            continue
        key = json.loads(rowkey)
        if tbl in ("itempattern", "itemowner") and before is None:
            held = db.execute("SELECT COUNT(*) FROM charinventory WHERE patternid = ?", (key["patternid"],)).fetchone()[0]
            if held:
                left.append(f"{tbl} {rowkey}: {held} copies still held, left")
                continue
        if go_json(row(db, tbl, key)) != after:
            left.append(f"{tbl} {rowkey}: changed since, left as it is")
            continue
        b = json.loads(before) if before else None
        put(db, tbl, key, None if b is None else {k: v for k, v in b.items() if k not in key})
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if not left:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--give", action="store_true", help="one into his pack (only while he is offline)")
    ap.add_argument("--pack-slots", type=int, default=100, help="world.pack_slots (the production servers: 100)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"undone: {back} rows back" + "".join(f"\n  left: {w}" for w in left))
            return 0 if not left else 1
        writes, lines, refusals = plan(db, a.give, a.pack_slots)
        print("\n".join(lines))
        if refusals:
            db.execute("ROLLBACK")
            print("REFUSED, nothing written:" + "".join(f"\n  {r}" for r in refusals))
            return 1
        if not writes:
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"(report only: {len(writes)} rows; --apply to write)")
            return 0
        n = apply(db, writes)
        db.execute("COMMIT")
        print(f"applied: {n} rows, in gm_audit and {LOG}" +
              ("; the pack row reaches him at his next login" if a.give else
               f"; give it by the GM panel's give (patternid {LAUNCHER})"))
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
