#!/usr/bin/env python3
"""Tailoring's modifiers as the forum's Trade Skills section gives them (eqoa.forumotion.com f72, "Gems, Gems, Gems",
t394, 31 July 2010; the section sent by the owner 2026-11-21 to check our tradeskills against).

The section's four threads were read whole (t392 Master Craft Guide Chart, t393 Level 60 Gem Drops, t394 Gems, Gems,
Gems, t395 Trade Skills General Questions) and set against both shards' recipes, components, sellers and rules. Its
Master Craft chart was already ours (tools/master_craft_20261114.py: 420 recipes, every cell matches); its jewel gems
match ours stat for stat (32 of 32); its armorsmiths' gems match the 2026-11-16 petition fixes; its weapon modifiers
match but for what two procs do. What differs on existing rows, and is data, is four of Tailoring's modifiers, whose
products give other stats than t394's "Tailoring: Magical Studs, Tannins, and Gems" table says (Prima p. 444's, which
the seed followed):

  Marr Blessed Studs   ours Sta + Cha        t394 "Mana Blessed Studs | wis + cha"     each product's Sta -> Wis
  Takish Ivory Studs   ours Dex + Cha        t394 "Takish Ivory Studs | int + agi"     Dex -> Agi, Cha -> Int
  Tunare Hair Thread   ours Agi + Wis        t394 "Tunare Hair Thread | dex + wis"     Agi -> Dex
  Mindflow Thread      ours PoT              t394 "Mindflow thread | PWR major"         + Max Power (PoT kept)

A move keeps the figure (the two stats of a two-stat modifier are equal at each trade level), as
tools/rallosian_marr_5200_20261116.py moved the armorsmiths' Marr Blessed Alloy, Lazulite and Rallosian Studs to what
the same table says. Mindflow Thread's power is ours: t394 gives no figure, so each product takes the Max Power of its
Mana Infused Thread twin (the same pattern and textile: the "PWR minor" thread), as tools/seed_tradeskills.py's
ULTRA_PLUS gave the armorsmiths' Mindflow Crystals the Mana Infused Alloy's; its Power over Time stays (the armorsmiths'
kept theirs too). Each modifier's description says what it now lends. t394's spellings "Mana Blessed" and "Tital Alloy
Studs" are not taken (this server's records, Prima's and the armorsmiths' twins say Marr Blessed, Titan Alloy).

Nothing else is written: no item is made, no drop, rarity, price, merchant or code touched (the audit's other
differences are listed in its report, for the owner).

Checked as read, else REFUSED and nothing written: one component record of each name; every product of each is a
Tailoring recipe's (no other craft takes them), the counts as read (92, 92, 92 and 65), each product's stats the
table's old pair (equal figures) with nothing yet where they go, or already moved; each Mindflow product with a Mana
Infused twin whose power is above 0. Report by default; --apply in one transaction, a gm_audit line a row (as the
panel's), logged in tradeskills_f72_1121_log; a second --apply finds nothing to do; --undo puts back exactly what was
there (a row changed since is left and listed). --skip NAME leaves a modifier as it is (repeatable).

WHEN IT TAKES EFFECT: no restart. Item records are read at each request; a character's items are sent at its login,
so a holder sees the new stats at the next login (eject to apply now).

    python3 -E -s tools/tradeskills_f72_20261121.py --db eqoa.db [--skip NAME ...] [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "tradeskills_f72_1121_log"
DETAIL = ("Tailoring's modifiers as the forum's Gems, Gems, Gems (t394): Marr Blessed Studs Wis+Cha, Takish Ivory Studs "
          "Int+Agi, Tunare Hair Thread Dex+Wis, Mindflow Thread + Max Power; tools/tradeskills_f72_20261121.py")
WHO, VIA = "dab", "tool"
TAILOR = 2  # craftrecipes.skill

# a modifier: its products' count, the stat it keeps (None: none), the moves (old column -> new), its new description
MOVES = {
    "Marr Blessed Studs": {"expect": 92, "keep": "cha", "moves": {"sta": "wis"},
                           "desc": "An uncommon marr blessed studs a tailor works into cloth or leather, lending wisdom and charisma."},
    "Takish Ivory Studs": {"expect": 92, "keep": None, "moves": {"dex": "agi", "cha": "intelligence"},
                           "desc": "An uncommon takish ivory studs a tailor works into cloth or leather, lending intelligence and agility."},
    "Tunare Hair Thread": {"expect": 92, "keep": "wis", "moves": {"agi": "dex"},
                           "desc": "An uncommon tunare hair thread a tailor works into cloth or leather, lending dexterity and wisdom."},
}
# a modifier given a stat: its products' count, the column, the twin modifier whose figure it takes, the column kept
ADDS = {
    "Mindflow Thread": {"expect": 65, "col": "POWMAX", "twin": "Mana Infused Thread", "keep": "PoT",
                        "desc": "An ultra rare mindflow thread a tailor works into cloth or leather, lending power and power regeneration."},
}

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


def audit(db, tbl, key, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', 'update', ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, tbl, go_json(key), go_json(before), go_json(after), DETAIL))
    return cur.lastrowid


def put(db, tbl, key, want):
    """tbl's existing row of key given want's columns; (before, after, audit id)."""
    before = row(db, tbl, key)
    where = " AND ".join(f"{k} = ?" for k in key)
    db.execute(f"UPDATE {tbl} SET {', '.join(f'{k} = ?' for k in want)} WHERE {where}",
               tuple(want.values()) + tuple(key.values()))
    after = row(db, tbl, key)
    return before, after, audit(db, tbl, key, before, after)


def fig(v):
    return "-" if v is None else str(v)


def component(db, name, refusals):
    found = db.execute("SELECT patternid, itemdesc FROM itempattern WHERE itemname = ? AND equipslot = -1", (name,)).fetchall()
    if len(found) != 1:
        refusals.append(f"{len(found)} component records named {name!r}, not one")
        return None, None
    return found[0]


def base_of(ings):
    """A recipe's consumed ingredients, the key its twins share."""
    return tuple(sorted((i.get("item"), i.get("count")) for i in ings if not i.get("keep")))


def plan(db, skip=()):
    """(writes [(tbl, key, want)], lines, refusals)."""
    writes, lines, refusals = [], [], []
    for t in ("itempattern", "craftrecipes", "gm_audit"):
        if not table(db, t):
            refusals.append(f"no {t} table here")
    if refusals:
        return writes, lines, refusals
    unknown = [s for s in skip if s not in MOVES and s not in ADDS]
    if unknown:
        refusals.append(f"--skip names no modifier of this tool's: {', '.join(unknown)}")
        return writes, lines, refusals
    recipes = [(skill, level, json.loads(ings), product) for skill, level, ings, product in
               db.execute("SELECT skill, level, ingredients, product FROM craftrecipes ORDER BY level, product")]

    def products(name, mid, expect):
        targets, other = [], []
        for skill, level, ings, product in recipes:
            if any(i.get("item") == mid for i in ings):
                (targets if skill == TAILOR else other).append((level, product, ings))
        if other:
            refusals.append(f"{len(other)} recipes of another craft take {name}: not looked at")
        if len(targets) != expect:
            refusals.append(f"{len(targets)} {name} products here, not {expect}")
        return targets

    def describe(name, mid, desc, want):
        if desc != want:
            writes.append(("itempattern", {"patternid": mid}, {"itemdesc": want}))
            lines.append(f"  description: {desc!r}\n            -> {want!r}")

    for name, m in MOVES.items():
        if name in skip:
            lines.append(f"{name}: skipped (--skip)")
            continue
        mid, desc = component(db, name, refusals)
        if mid is None:
            continue
        lines.append(f"{name}: item {mid}")
        describe(name, mid, desc, m["desc"])
        moves = m["moves"]
        shown = ([m["keep"]] if m["keep"] else []) + list(moves) + list(moves.values())
        targets = products(name, mid, m["expect"])
        lines.append(f"  products: {len(targets)} (" + ", ".join(f"{o} -> {n}" for o, n in moves.items()) + ")")
        for lvl, pid, _ in targets:
            cur = row(db, "itempattern", {"patternid": pid})
            if cur is None:
                refusals.append(f"recipe product {pid} (trade level {lvl}) has no itempattern row")
                continue
            v = lambda c: cur.get(c) or 0  # noqa: E731
            olds, news = [v(c) for c in moves], [v(c) for c in moves.values()]
            kept = [v(m["keep"])] if m["keep"] else []
            if all(x == 0 for x in olds) and news[0] > 0 and len(set(news + kept)) == 1:
                want, after = None, cur  # moved already
            elif olds[0] > 0 and len(set(olds + kept)) == 1 and all(x == 0 for x in news):
                # a swap: the old column takes the new one's empty value (0 or NULL, as the row has it)
                want = dict({o: cur.get(n) for o, n in moves.items()}, **{n: cur.get(o) for o, n in moves.items()})
                after = dict(cur, **want)
            else:
                refusals.append(f"{pid} {cur['itemname']!r}: " + " ".join(f"{c} {v(c)}" for c in shown) + ", not the table's pair")
                continue
            lines.append(f"  {pid:>7} TL{lvl:<2} {cur['itemname']:<34} "
                         + "  ".join(f"{c} {fig(cur.get(c))}->{fig(after.get(c))}" for c in shown))
            if want:
                writes.append(("itempattern", {"patternid": pid}, want))

    for name, m in ADDS.items():
        if name in skip:
            lines.append(f"{name}: skipped (--skip)")
            continue
        mid, desc = component(db, name, refusals)
        tid, _ = component(db, m["twin"], refusals)
        if mid is None or tid is None:
            continue
        lines.append(f"{name}: item {mid}; {m['col']} from its {m['twin']} twin (item {tid})")
        describe(name, mid, desc, m["desc"])
        twins = {}
        for skill, level, ings, product in recipes:
            if skill == TAILOR and any(i.get("item") == tid and i.get("keep") for i in ings):
                twins[base_of(ings)] = product
        col, keep = m["col"], m["keep"]
        targets = products(name, mid, m["expect"])
        lines.append(f"  products: {len(targets)} (+ {col})")
        for lvl, pid, ings in targets:
            cur = row(db, "itempattern", {"patternid": pid})
            tw = twins.get(base_of(ings))
            twin = row(db, "itempattern", {"patternid": tw}) if tw is not None else None
            if cur is None or twin is None:
                refusals.append(f"recipe product {pid} (trade level {lvl}): " + ("no itempattern row" if cur is None else f"no {m['twin']} twin"))
                continue
            have, give = cur.get(col) or 0, twin.get(col) or 0
            if give <= 0 or (cur.get(keep) or 0) <= 0:
                refusals.append(f"{pid} {cur['itemname']!r}: {keep} {fig(cur.get(keep))}, twin {twin['itemname']!r} {col} {fig(twin.get(col))}")
                continue
            if have == give:
                want = None
            elif have == 0:
                want = {col: give}
            else:
                refusals.append(f"{pid} {cur['itemname']!r}: {col} {have}, neither 0 nor its twin's {give}")
                continue
            lines.append(f"  {pid:>7} TL{lvl:<2} {cur['itemname']:<34} {keep} {fig(cur.get(keep))}  {col} {fig(cur.get(col))}->{give}"
                         f"  ({twin['itemname']})")
            if want:
                writes.append(("itempattern", {"patternid": pid}, want))

    if table(db, "charinventory") and writes:
        ids = sorted({k["patternid"] for _, k, _ in writes})
        q = ",".join("?" * len(ids))
        n, chars = db.execute(f"SELECT count(*), count(DISTINCT serverid) FROM charinventory WHERE patternid IN ({q})", ids).fetchone()
        lines.append(f"held (the rows written): {n} items, {chars} characters")
    return writes, lines, refusals


def apply(db, writes):
    db.execute(LOG_DDL)
    for tbl, key, want in writes:
        before, after, n = put(db, tbl, key, want)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                   (tbl, go_json(key), go_json(before), go_json(after), n))
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
        a = json.loads(after)
        put(db, tbl, key, {k: v for k, v in b.items() if k not in key and a.get(k) != v})
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if not left:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--skip", action="append", default=[], metavar="NAME", help="leave this modifier as it is (repeatable)")
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
        writes, lines, refusals = plan(db, set(a.skip))
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
            print(f"(report only: {len(writes)} rows; --apply to write)")
            return 0
        n = apply(db, writes)
        db.execute("COMMIT")
        print(f"applied: {n} rows, in gm_audit and {LOG}; holders see it at their next login")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
