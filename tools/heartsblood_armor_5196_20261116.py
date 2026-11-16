#!/usr/bin/env python3
"""Heartsblood armour gives Stamina, Arcane Resist and Hit Points (petition 5196, Innoruuk, 2026-11-16: "Heartsblood
Alloy is supposed to give Stamina, AR and HP in stats for plate armor. Currently only giving HoT."; a player's claim
about an item's stats is taken as the truth).

The armorsmiths' products of Heartsblood Alloy (craftrecipes skill 1 whose modifier is the "heartsblood alloy" item:
85 records, trade levels 6-10, the six plate pieces, the six scale pieces and the five shields) were made off
eqoatrade's grid, whose Heartsblood row holds the AC and a HoT alone (4, 6, 8, 10, 10 at trade levels 6-10: Prima
p. 441, whose modifier table names Heartsblood Alloy "Hit Point Regen"). The live records this server holds of the
same pieces (Return Home's "heartsblood seralite breastplate" and kin: Sta 18, AR 18, HP 81; the scale set: 27, 27,
95) give Stamina, Arcane Resist and Hit Points, as the player says.

Each one is given, at least, what the same piece at the same trade level gives by the single-stat modifier:
- sta   as its Rhodizite twin's   (14, 18, 18, 20, 20 at trade levels 6-10),
- AR    as its Achroite twin's    (14, 18, 18, 20, 20),
- HPMAX as its Titan Alloy twin's (55, 60, 65, 70, 75),
- HoT   Prima's figure (4, 6, 8, 10, 10): Prima gives it, so it stays, and the live records that lacked it get it.
The twin is the recipe of the same trade level with the same ore and mold. A figure already higher is kept (nothing is
lowered); AC and every other column are left as they are. tools/seed_tradeskills.py makes the same figures for a
product it writes.

Checked as found, else REFUSED and nothing written: the modifiers' items by name, 85 Heartsblood products each with
its three twins, the grid's products' HoT Prima's. Report by default; --apply in one transaction, a gm_audit line a
row (as the panel's), logged in heartsblood_5196_log; a second --apply finds nothing to do; --undo puts back exactly
what was there (a row changed since is left and listed).

After --apply: no restart. A character's items are sent at its login, so a holder sees the new stats at the next
login (eject to apply now). Imported copies of these records (itemimported) are made their bases again at the next
server start (SyncImportedCopies).

    python3 -E -s tools/heartsblood_armor_5196_20261116.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "heartsblood_5196_log"
DETAIL = ("Heartsblood armour gives Stamina, Arcane Resist and Hit Points (its Rhodizite, Achroite and Titan Alloy "
          "twins' figures at least) beside Prima's HoT (petition 5196, 2026-11-16); tools/heartsblood_armor_5196_20261116.py")
WHO, VIA = "dab", "tool"

ARMOR = 1  # craftrecipes.skill: Armorcrafting
HEARTSBLOOD = "heartsblood alloy"
# the column -> the modifier whose twin sets its least figure
TWINS = {"sta": "rhodizite", "AR": "achroite", "HPMAX": "titan alloy"}
# Prima p. 441's HoT by trade level (eqoatrade's Heartsblood row agrees, every piece)
HOT = {6: 4, 7: 6, 8: 8, 9: 10, 10: 10}
COLS = ["sta", "AR", "HPMAX", "HoT"]
EXPECT = 85  # 17 pieces x trade levels 6-10

LOG_DDL = f"""CREATE TABLE {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    rowkey TEXT NOT NULL,      -- the key (JSON)
    before TEXT,               -- the row before (JSON)
    after  TEXT NOT NULL,      -- the row written (JSON)
    audit  INTEGER NOT NULL
)"""


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
    """tbl's row of key given the columns of want; (before, after, audit id)."""
    before = row(db, tbl, key)
    where = " AND ".join(f"{k} = ?" for k in key)
    sets = {k: v for k, v in want.items() if k not in key}
    db.execute(f"UPDATE {tbl} SET {', '.join(f'{k} = ?' for k in sets)} WHERE {where}", tuple(sets.values()) + tuple(key.values()))
    after = row(db, tbl, key)
    return before, after, audit(db, tbl, key, "update", before, after)


def item_id(db, name, refusals):
    found = db.execute("SELECT patternid FROM itempattern WHERE lower(itemname) = ? AND equipslot = -1", (name,)).fetchall()
    if len(found) != 1:
        refusals.append(f"{len(found)} component records named {name!r}, not one")
        return None
    return found[0][0]


def fig(v):
    return "-" if v is None else str(v)


def plan(db):
    """(writes [(key, want, line)], lines, refusals)."""
    writes, lines, refusals = [], [], []
    for t in ("itempattern", "craftrecipes", "gm_audit"):
        if not table(db, t):
            refusals.append(f"no {t} table here")
    if refusals:
        return writes, lines, refusals
    hb = item_id(db, HEARTSBLOOD, refusals)
    twin_ids = {col: item_id(db, name, refusals) for col, name in TWINS.items()}
    if refusals:
        return writes, lines, refusals
    # every armorsmith recipe by (trade level, ore, mold) -> {modifier item: product}
    by_base = {}
    for level, ings, product in db.execute("SELECT level, ingredients, product FROM craftrecipes WHERE skill = ?", (ARMOR,)):
        ings = json.loads(ings)
        mods = [i["item"] for i in ings if i.get("keep")]
        base = tuple(i["item"] for i in ings if not i.get("keep"))
        if len(mods) == 1:
            by_base.setdefault((level, base), {})[mods[0]] = product
    targets = sorted(((lvl, base), m[hb]) for (lvl, base), m in by_base.items() if hb in m)
    if len(targets) != EXPECT:
        refusals.append(f"{len(targets)} Heartsblood armour products here, not {EXPECT}")
    for (lvl, base), pid in targets:
        cur = row(db, "itempattern", {"patternid": pid})
        if cur is None:
            refusals.append(f"recipe product {pid} (trade level {lvl}) has no itempattern row")
            continue
        if lvl not in HOT:
            refusals.append(f"{pid} {cur['itemname']!r}: trade level {lvl}, Heartsblood is 6-10")
            continue
        want = {}
        twins = {}
        for col, mid in twin_ids.items():
            tp = by_base[(lvl, base)].get(mid)
            t = row(db, "itempattern", {"patternid": tp}) if tp is not None else None
            if t is None or not t.get(col):
                refusals.append(f"{pid} {cur['itemname']!r}: no {TWINS[col]} twin with {col} (trade level {lvl})")
                continue
            twins[col] = t[col]
        if len(twins) != len(twin_ids):
            continue
        if cur.get("HoT") and cur["HoT"] != HOT[lvl]:
            refusals.append(f"{pid} {cur['itemname']!r}: HoT {cur['HoT']}, Prima's is {HOT[lvl]} at trade level {lvl}")
            continue
        for col, least in list(twins.items()) + [("HoT", HOT[lvl])]:
            have = cur.get(col) or 0
            if have < least:
                want[col] = least
        new = {c: want.get(c, cur.get(c)) for c in COLS}
        line = (f"  {pid:>7} TL{lvl} {cur['itemname']:<40} AC {cur['AC']:>3}  "
                + "  ".join(f"{c} {fig(cur.get(c))}->{fig(new[c])}" for c in COLS))
        lines.append(line)
        if want:
            writes.append(({"patternid": pid}, want))
    if table(db, "charinventory"):
        ids = [pid for _, pid in targets]
        if ids:
            q = ",".join("?" * len(ids))
            n, chars = db.execute(f"SELECT count(*), count(DISTINCT serverid) FROM charinventory WHERE patternid IN ({q})", ids).fetchone()
            lines.append(f"  held: {n} items, {chars} characters")
    return writes, lines, refusals


def apply(db, writes):
    db.execute(LOG_DDL)
    for key, want in writes:
        before, after, n = put(db, "itempattern", key, want)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                   ("itempattern", go_json(key), go_json(before), go_json(after), n))
    return len(writes)


def undo(db):
    back, left = 0, []
    if not table(db, LOG):
        return back, left
    for seq, tbl, rowkey, before, after in db.execute(f"SELECT seq, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key = json.loads(rowkey)
        if go_json(row(db, tbl, key)) != after:
            left.append(f"{tbl} {rowkey}: changed since, left as it is")
            continue
        b = json.loads(before)
        put(db, tbl, key, {c: b[c] for c in COLS})
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if not left:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
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
        writes, lines, refusals = plan(db)
        if refusals:
            db.execute("ROLLBACK")
            print("REFUSED, nothing written:" + "".join(f"\n  {r}" for r in refusals))
            return 1
        print("\n".join(lines))
        if not writes:
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if table(db, LOG):
            db.execute("ROLLBACK")
            print(f"applied before and changed since ({LOG} is there): --undo first")
            return 1
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"(report only: {len(writes)} records; --apply to write)")
            return 0
        n = apply(db, writes)
        db.execute("COMMIT")
        print(f"applied: {n} records, in gm_audit and {LOG}; holders see it at their next login, imported copies at the next start")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
