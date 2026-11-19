#!/usr/bin/env python3
"""Enstatite armour gives Strength and Stamina, Takish Ivory armour Dexterity and Agility (a player's petitions,
2026-11-16: 5198 "Enstatite gem should give strength and stamina to armor when crafted, not strength and
intelligence."; 5199 "Takish ivory should give dex and agility when crafted, not charisma."; a player's claim about an
item's stats is taken as the truth).

The armorsmiths' products of the two (craftrecipes skill 1 whose modifier is the "enstatite" or "takish ivory" item:
115 records each, trade levels 4-10, the six plate pieces, the six chain/scale pieces and the shields) were made off
eqoatrade's grid, whose rows give Str and Int, and Dex and Cha, alike (6, 7, 8, 9, 9, 10, 10 at trade levels 4-10), as
Prima p. 441 names them ("Strength and Intelligence", "Dexterity and Charisma"). Each one's Int goes to Sta (Enstatite)
and its Cha to Agi (Takish Ivory), the same figure; the other stat stays; nothing else changes.
tools/seed_tradeskills.py makes the same figures for a product it writes. No other craft's recipe takes either gem
(else REFUSED: not looked at).

Checked as found, else REFUSED and nothing written: each gem's item by name, 115 products of each, each with its kept
stat equal to the moved one and nothing yet in the new one (or already moved). Report by default; --apply in one
transaction, a gm_audit line a row (as the panel's), logged in two_stat_5198_log; a second --apply finds nothing to
do; --undo puts back exactly what was there (a row changed since is left and listed).

After --apply: no restart. A character's items are sent at its login, so a holder sees the new stats at the next
login (eject to apply now). Imported copies of these records (itemimported) are made their bases again at the next
server start (SyncImportedCopies).

    python3 -E -s tools/two_stat_mods_5198_20261116.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "two_stat_5198_log"
DETAIL = ("Enstatite armour gives Strength and Stamina (its Intelligence moved to Stamina), Takish Ivory armour Dexterity "
          "and Agility (its Charisma moved to Agility) (petitions 5198, 5199, 2026-11-16); tools/two_stat_mods_5198_20261116.py")
WHO, VIA = "dab", "tool"

ARMOR = 1  # craftrecipes.skill: Armorcrafting
# the gem -> (the stat it keeps, the stat moved, where it goes)
MOVED = {"enstatite": ("str", "intelligence", "sta"), "takish ivory": ("dex", "cha", "agi")}
COLS = ["str", "sta", "dex", "agi", "intelligence", "cha"]
EXPECT = 115  # a gem's

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


def fig(v):
    return "-" if v is None else str(v)


def plan(db):
    """(writes [(key, want)], lines, refusals)."""
    writes, lines, refusals = [], [], []
    for t in ("itempattern", "craftrecipes", "gm_audit"):
        if not table(db, t):
            refusals.append(f"no {t} table here")
    if refusals:
        return writes, lines, refusals
    recipes = [(skill, level, json.loads(ings), product) for skill, level, ings, product in
               db.execute("SELECT skill, level, ingredients, product FROM craftrecipes ORDER BY level, product")]
    ids = []
    for gem, (keep, old, new) in MOVED.items():
        found = db.execute("SELECT patternid FROM itempattern WHERE lower(itemname) = ? AND equipslot = -1", (gem,)).fetchall()
        if len(found) != 1:
            refusals.append(f"{len(found)} component records named {gem!r}, not one")
            continue
        gid = found[0][0]
        targets, other = [], []
        for skill, level, ings, product in recipes:
            if any(i.get("item") == gid for i in ings):
                (targets if skill == ARMOR else other).append((level, product))
        if other:
            refusals.append(f"{len(other)} recipes of another craft take {gem}: not looked at")
        if len(targets) != EXPECT:
            refusals.append(f"{len(targets)} {gem} armour products here, not {EXPECT}")
        lines.append(f"{gem}: {len(targets)} products, {old} -> {new}")
        for lvl, pid in targets:
            ids.append(pid)
            cur = row(db, "itempattern", {"patternid": pid})
            if cur is None:
                refusals.append(f"recipe product {pid} (trade level {lvl}) has no itempattern row")
                continue
            k, o, n = cur.get(keep) or 0, cur.get(old) or 0, cur.get(new) or 0
            if o == 0 and n > 0 and k == n:
                want, after = None, cur
            elif k > 0 and k == o and n == 0:
                want = {old: None, new: o}
                after = dict(cur, **want)
            else:
                refusals.append(f"{pid} {cur['itemname']!r}: {keep} {fig(cur.get(keep))} {old} {fig(cur.get(old))} "
                                f"{new} {fig(cur.get(new))}, not the grid's pair")
                continue
            lines.append(f"  {pid:>7} TL{lvl:<2} {cur['itemname']:<40} AC {cur['AC']:>3}  "
                         + "  ".join(f"{c} {fig(cur.get(c))}->{fig(after.get(c))}" for c in (keep, old, new)))
            if want:
                writes.append(({"patternid": pid}, want))
    if table(db, "charinventory") and ids:
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
