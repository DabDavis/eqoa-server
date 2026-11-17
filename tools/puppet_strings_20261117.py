#!/usr/bin/env python3
"""The Toy Shoppe's Puppet Strings (the owner, 2026-11-17: "charms the target if light blue or lower", then "the charm
has 10 charges then disappears"; for Zaeire, and sold by Toymaker Tinsel).

NOT the Cazic Thule drop of the same name (713085, petition 1426: a charm of levels 41-52, 50 charges, rechargeable at
Master Lizzlen), which this leaves alone: the server knows this one by its itemeffect line, never by its name.

What it does is the server's (branch puppet-strings, entity/charmcon.go): Use on the target; a creature that cons light
blue or green to the user becomes the user's charmed pet for 10 minutes, or until let go by the charm's usual rules;
refused in red, nothing spent, with any pet out, at what no charm takes (players, pets, uncharmable npccharm names, raid
bosses, rares, quest and trigger spawns, dummies) or at one that cons dark blue or above ("This creature is too strong
for the strings."). Each charm spends a charge; the tenth takes the item ("Your Puppet Strings snap."). 60 s cooldown,
kept over logout (potioncooldowns). Lizzlen never recharges it (the line's "No Recharge").

On one database (Heirloom: the owner's ask) --apply writes, each row audited (gm_audit, as the panel's) and logged in
puppet_strings_1117_log:

1. itempattern "Puppet Strings" at the next free id from 716914 (the Christmas items' range, after the Santa Hat
   716913): the Cazic Thule strings' record (713085) copied for its icon and CLIENT's item type (procanim 7: the item
   menu's Use, a gem quick slot), with its own description, level 1, every class and race (32767), TRADEABLE (trade 0),
   not lore (each set holds its own charges, so a player may carry spares), no stats, worth nothing (as the toys),
   one to a stack.
2. itemeffect on it: LINE (a Target charm "up to light blue", "No Recharge", 10 minutes), charges 10, cooldown 60:
   every copy made from it (Tinsel's sale, a GM's give, --give) holds 10.
3. tokenshop: Toymaker Tinsel sells it for 108 Holly Tokens (the Barrel of Monkeys' price), sort 5.
4. --give: one into Zaeire's pack (serverid 193, account 107 on Heirloom; refused if the name or account differ),
   10 charges, at the end of the list, unless he holds one already. Only while he is OFFLINE (the row reaches him at
   his next login); while he is online, give it by the GM panel's give (the new patternid), which sends it at once.

Checked as found, else REFUSED and nothing written: 713085 the Cazic Thule strings (procanim 7, worn nowhere);
Tinsel's shop there; tables itempattern, itemeffect, tokenshop, characters, charinventory, gm_audit. Report by default;
a second --apply finds nothing to do; --undo puts back exactly what was there, all or nothing: refused while anyone
other than the --give's copy holds one (packs, banks, mercs, auctions) or a row written has changed since.

Needs a server with puppet-strings (an older one reads the line's "up to light blue" as a charm of its level rule, which
refuses everything, and spends nothing). Takes effect at once: the item use and the shop read the database at each use.

    python3 -E -s tools/puppet_strings_20261117.py --db eqoa.db [--give] [--apply | --undo] [--pack-slots 100]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "puppet_strings_1117_log"
DETAIL = ("The Toy Shoppe's Puppet Strings: charms a light blue or lower creature, 10 charges (the owner, 2026-11-17); "
          "tools/puppet_strings_20261117.py")
WHO, VIA = "dab", "tool"
NAME = "Puppet Strings"
BASE = 713085  # the Cazic Thule strings: their icon and item type
IDS = (716914, 716999)
DESC_MAX = 200
DESC = ("A toymaker's marionette strings on a little wooden cross. Use them on a creature that cons light blue or "
        "green and it dances to your tune as your pet for 10 minutes. 10 charges.")
LINE = "Target, Charm up to light blue, No Recharge, 10min"
CHARGES, COOLDOWN = 10, 60.0
SHOP, PRICE, SORT = "Toymaker Tinsel", 108, 5
SAYS = ("Marionette strings, my own make! Use them on any creature that cons light blue or green and it dances for you "
        "for ten minutes. Ten charms to a set.")
GIVE_NAME, GIVE_ACCOUNT = "Zaeire", 107
STATS = ("str", "sta", "agi", "wis", "dex", "cha", "intelligence", "HPMAX", "POWMAX", "PoT", "HoT", "AC", "PR", "DR",
         "FR", "CR", "LR", "AR")

LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    rowkey TEXT NOT NULL,
    before TEXT,
    after  TEXT,
    audit  INTEGER NOT NULL
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def cols(db, tbl):
    return {r[1] for r in db.execute(f"PRAGMA table_info({tbl})")}


def row(db, tbl, key):
    where = " AND ".join(f"{k} = ?" for k in key)
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", tuple(key.values()))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return None if m is None else json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, tbl, key, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    return db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                      "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                      (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key), go_json(before), go_json(after), DETAIL)).lastrowid


def write(db, log, tbl, key, want):
    """Make tbl's row of key want (a whole row; None deletes it), audited (and logged when log). A key of None inserts
    a new row (charinventory: its autoincrement itemid the key). Returns the key."""
    if key is None:
        cur = db.execute(f"INSERT INTO {tbl} ({', '.join(want)}) VALUES ({', '.join('?' for _ in want)})", tuple(want.values()))
        key, before, action = {"itemid": cur.lastrowid}, None, "insert"
    else:
        before = row(db, tbl, key)
        where = " AND ".join(f"{k} = ?" for k in key)
        if want is None:
            db.execute(f"DELETE FROM {tbl} WHERE {where}", tuple(key.values()))
            action = "delete"
        elif before is None:
            c = dict(key, **want)
            db.execute(f"INSERT INTO {tbl} ({', '.join(c)}) VALUES ({', '.join('?' for _ in c)})", tuple(c.values()))
            action = "insert"
        else:
            sets = [c for c in want if c not in key]
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in sets)} WHERE {where}",
                       tuple(want[c] for c in sets) + tuple(key.values()))
            action = "update"
    after = row(db, tbl, key)
    a = audit(db, action, tbl, key, before, after)
    if log:
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                   (tbl, go_json(key), go_json(before), go_json(after), a))
    return key


def strings_id(db):
    """The Toy Shoppe's record: named so in the range with this tool's line (never the Cazic Thule one)."""
    for (pid,) in db.execute("SELECT patternid FROM itempattern WHERE itemname = ? AND patternid BETWEEN ? AND ? ORDER BY 1",
                             (NAME, *IDS)):
        e = row(db, "itemeffect", {"patternid": pid})
        if e is None or e["line"] == LINE:
            return pid
    return None


def record(db, base, pid):
    want = dict(base, patternid=pid, itemname=NAME, itemdesc=DESC, itemcost=0, trade=0, lore=0, rent=1, levelreq=1,
                maxstack=1, maxhp=0, classuse=32767, raceuse=32767, equipslot=-1, attacktype=0, weapondamage=0, unk5=0,
                craft=0)
    for s in STATS:
        if s in want:
            want[s] = 0
    for c in ("weaponproc", "fish"):
        if c in want and want[c] in ("", None):
            want[c] = 0
    return want


def plan(db, give, pack_slots):
    """(writes [(tbl, key, want)], lines, refusals)."""
    for t in ("itempattern", "itemeffect", "tokenshop", "characters", "charinventory", "gm_audit"):
        if not table(db, t):
            return [], [], [f"no {t} table here"]
    if len(DESC) > DESC_MAX or "|" in DESC or not DESC.isascii():
        return [], [], [f"the description is {len(DESC)} characters (at most {DESC_MAX}, ASCII, no '|')"]
    if not {"charges", "cooldown"} <= cols(db, "itemeffect"):
        return [], [], ["itemeffect has no charges/cooldown columns: run a newer server once first"]
    base = row(db, "itempattern", {"patternid": BASE})
    if base is None or base["itemname"] != NAME or base["procanim"] != 7 or base["equipslot"] != -1:
        return [], [], [f"itempattern {BASE} is not the Cazic Thule Puppet Strings ({base and base['itemname']})"]
    if not db.execute("SELECT 1 FROM tokenshop WHERE shop = ?", (SHOP,)).fetchone():
        return [], [], [f"no tokenshop rows for {SHOP} here: the Christmas data first"]
    lines, writes, refusals = [], [], []
    pid = strings_id(db)
    if pid is None:
        taken = {r[0] for r in db.execute("SELECT patternid FROM itempattern WHERE patternid BETWEEN ? AND ?", IDS)}
        pid = next((p for p in range(IDS[0], IDS[1] + 1) if p not in taken), None)
        if pid is None:
            return [], [], [f"no free itempattern id in {IDS[0]}-{IDS[1]}"]
        writes.append(("itempattern", {"patternid": pid}, record(db, base, pid)))
        lines.append(f"itempattern {pid}: {NAME}, {BASE}'s icon and type (procanim 7), level 1, every class and race, "
                     "tradeable, not lore, worth 0")
    else:
        have = row(db, "itempattern", {"patternid": pid})
        want = record(db, base, pid)
        if any(have.get(k) != v for k, v in want.items()):
            writes.append(("itempattern", {"patternid": pid}, want))
            lines.append(f"itempattern {pid}: {NAME} made as below")
        else:
            lines.append(f"itempattern {pid}: {NAME} there already")
    want = {"patternid": pid, "line": LINE, "name": NAME, "icon": 0, "breaks": "", "wear": 0, "area": 0.0,
            "source": "puppet-strings", "body": 0, "target_line": "", "charges": CHARGES, "cooldown": COOLDOWN}
    want = {k: v for k, v in want.items() if k in cols(db, "itemeffect")}
    have = row(db, "itemeffect", {"patternid": pid})
    if have is None or any(have.get(k) != v for k, v in want.items()):
        writes.append(("itemeffect", {"patternid": pid}, want))
        lines.append(f"itemeffect {pid}: {LINE!r}, {CHARGES} charges, {COOLDOWN:g} s cooldown")
    if row(db, "tokenshop", {"shop": SHOP, "patternid": pid}) is None:
        writes.append(("tokenshop", {"shop": SHOP, "patternid": pid},
                       {"price": PRICE, "token": "", "says": SAYS, "sort": SORT, "source": "puppet-strings"}))
        lines.append(f"tokenshop: {SHOP} sells it for {PRICE} Holly Tokens (sort {SORT})")
    if give:
        found = db.execute("SELECT serverid, accountid, level FROM characters WHERE charName = ?", (GIVE_NAME,)).fetchall()
        if len(found) != 1:
            return [], lines, [f"{len(found)} characters named {GIVE_NAME}, not one"]
        sid, acct, level = found[0]
        if acct != GIVE_ACCOUNT:
            return [], lines, [f"{GIVE_NAME} (serverid {sid}) is account {acct}'s, not {GIVE_ACCOUNT}'s"]
        lines.append(f"--give: {GIVE_NAME}, serverid {sid}, account {acct}, level {level}")
        if db.execute("SELECT 1 FROM charinventory WHERE serverid = ? AND patternid = ?", (sid, pid)).fetchone():
            lines.append(f"--give: {GIVE_NAME} holds one already")
        else:
            n = db.execute("SELECT COUNT(*) FROM charinventory WHERE serverid = ? AND location = 0", (sid,)).fetchone()[0]
            if n + 1 > pack_slots:
                refusals.append(f"{GIVE_NAME}'s pack holds {n} of {pack_slots}: no room")
            nxt = db.execute("SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory WHERE serverid = ? AND location = 0",
                             (sid,)).fetchone()[0]
            writes.append(("charinventory", None, {"serverid": sid, "stackleft": 1, "remainHP": 0, "remaincharge": CHARGES,
                                                    "patternid": pid, "equiploc": -1, "location": 0, "listnumber": nxt}))
            lines.append(f"charinventory: {NAME} into {GIVE_NAME}'s pack, {CHARGES} charges, list number {nxt} "
                         f"({n} -> {n + 1} of {pack_slots})")
    return writes, lines, refusals


def held(db, pid, but=()):
    n = 0
    for tbl in ("charinventory", "accountbank", "charmercitem", "auction_lots"):
        if table(db, tbl) and "patternid" in cols(db, tbl):
            if tbl == "charinventory" and but:
                n += db.execute(f"SELECT count(*) FROM charinventory WHERE patternid = ? AND itemid NOT IN "
                                f"({', '.join('?' for _ in but)})", (pid, *but)).fetchone()[0]
            else:
                n += db.execute(f"SELECT count(*) FROM {tbl} WHERE patternid = ?", (pid,)).fetchone()[0]
    return n


def undo(db):
    if not table(db, LOG):
        return 0, []
    logged = db.execute(f"SELECT seq, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall()
    given = tuple(json.loads(k)["itemid"] for _, t, k, _, _ in logged if t == "charinventory")
    left = []
    for _, tbl, rowkey, before, after in logged:
        if go_json(row(db, tbl, json.loads(rowkey))) != after:
            left.append(f"{tbl} {rowkey}: changed since, nothing undone")
        if tbl == "itempattern" and before is None:
            n = held(db, json.loads(rowkey)["patternid"], given)
            if n:
                left.append(f"{n} Puppet Strings are held (packs, banks, mercs, auctions) beside the --give's: nothing undone")
    if left:
        return 0, left
    for _, tbl, rowkey, before, _ in logged:
        write(db, False, tbl, json.loads(rowkey), json.loads(before) if before else None)
    db.execute(f"DROP TABLE {LOG}")
    return len(logged), []


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--give", action="store_true", help=f"one into {GIVE_NAME}'s pack (only while he is offline)")
    ap.add_argument("--pack-slots", type=int, default=100, help="world.pack_slots (eqoaclassic: 100)")
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
        for line in lines:
            print(line)
        if refusals:
            db.execute("ROLLBACK")
            print("REFUSED, nothing written:\n  " + "\n  ".join(refusals))
            return 1
        if not writes:
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"(report only: {len(writes)} rows; --apply to write)")
            return 0
        db.execute(LOG_DDL)
        for tbl, key, want in writes:
            write(db, True, tbl, key, want)
        db.execute("COMMIT")
        print(f"applied: {len(writes)} rows, in gm_audit and {LOG}; the shop and the item's use read them at once" +
              ("; the pack row reaches him at his next login" if a.give else ""))
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
