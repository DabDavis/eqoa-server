#!/usr/bin/env python3
"""Rallosian Studs give Stamina and Dexterity, Marr Blessed Alloy Stamina and Wisdom, Lazulite Agility and Stamina
(Innoruuk's petitions, 2026-11-16: 5200 "Rallosian Studs not in the game (gem/crafting item) Stamina and Dexterity I
believe should be the stats."; 5201 "Marr Blessed Alloy should give Stamina and Wisdom, not Stamina and Charisma";
5202 "Lazulite is supposed to give crafted armor Agility and Stamina."; a player's claim about an item's stats is
taken as the truth).

Rallosian Studs: Tailoring's seed (tools/seed_tradeskills.py --craft tailor) made the studs (an uncommon modifier,
Prima p. 444: "Strength and Intelligence", the "Rallosian*" products) and their 92 products (craftrecipes skill 2: the
wool robe at trade level 3, then the 13 cloth and leather pieces at 4-10), each giving Str and Int alike (Prima p.
445's two attributes: 5, 6, 7, 8, 8, 9, 10, 10 at trade levels 3-10), and put the studs in the "modifiers" loot pool
as every uncommon tailoring modifier (weight 9, levels 10-60). So the item is in the game on both shards; what it
gives is not what the player says. Each product's Str goes to Sta and its Int to Dex, the same figures, and the
studs' description says what they lend ("... lending stamina and dexterity.").

Marr Blessed Alloy: the armorsmiths' 115 products of it (craftrecipes skill 1, trade levels 4-10: eqoatrade's grid and
Prima p. 441's "Stamina and Charisma", 6, 7, 8, 9, 9, 10, 10) have their Cha moved to Wis, the same figure; their Sta
stays. Tailoring's Marr Blessed Studs (another item, Sta and Cha) are not touched.

Lazulite: the armorsmiths' 115 products of it (skill 1, trade levels 4-10: the grid and Prima's "Agility and Wisdom",
6, 7, 8, 9, 9, 10, 10) have their Wis moved to Sta, the same figure; their Agi stays. Tailoring's Tunare Hair Thread
(Agi and Wis, Prima's tailoring counterpart) is not touched.

Nothing else changes. --with-drop (a separate step: drops need the owner's approval): the Rallosian Studs' row in the
"modifiers" pool (category 'modifiers', region '*', at the weight and levels of Marr Blessed Studs' row, the other
uncommon tailoring studs), made only if it is not there. On both shards it is there already, so it adds nothing.

Checked as found, else REFUSED and nothing written: one component record of each name; the products' counts (92, 115
and 115), each with its moved stats equal (to the kept one too) and nothing yet where they go (or already moved); no other
craft's recipe takes either. Report by default; --apply in one transaction, a gm_audit line a row (as the panel's),
logged in rallosian_5200_log; a second --apply finds nothing to do; --undo puts back exactly what was there (a row
changed since is left and listed).

After --apply: no restart. A character's items are sent at its login, so a holder sees the new stats at the next
login (eject to apply now). The loot pool is read as creatures die.

    python3 -E -s tools/rallosian_marr_5200_20261116.py --db eqoa.db [--with-drop] [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "rallosian_5200_log"
DETAIL = ("Rallosian Studs give Stamina and Dexterity (their products' Str and Int moved to Sta and Dex), Marr Blessed Alloy "
          "Stamina and Wisdom (its products' Cha moved to Wis), Lazulite Agility and Stamina (its products' Wis moved to Sta) "
          "(petitions 5200-5202, 2026-11-16); "
          "tools/rallosian_marr_5200_20261116.py")
WHO, VIA = "dab", "tool"

STUDS = "Rallosian Studs"
# a modifier: its craftrecipes skill, its products' count, the stat it keeps (None: none), the moves, its new description
MODS = {
    STUDS: {"skill": 2, "expect": 92, "keep": None, "moves": {"str": "sta", "intelligence": "dex"},
            "desc": "An uncommon rallosian studs a tailor works into cloth or leather, lending stamina and dexterity."},
    "Marr Blessed Alloy": {"skill": 1, "expect": 115, "keep": "sta", "moves": {"cha": "wis"}, "desc": None},
    "Lazulite": {"skill": 1, "expect": 115, "keep": "agi", "moves": {"wis": "sta"}, "desc": None},
}
POOL, LIKE = "modifiers", "Marr Blessed Studs"  # the pool, and the uncommon studs whose row the new one copies

LOG_DDL = f"""CREATE TABLE {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    rowkey TEXT NOT NULL,      -- the key (JSON)
    before TEXT,               -- the row before (JSON), NULL none
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
    """tbl's row of key made want (the columns to set; None: deleted; a row not there: inserted); (before, after, audit)."""
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
    return before, after, audit(db, tbl, key, action, before, after)


def fig(v):
    return "-" if v is None else str(v)


def plan(db, with_drop):
    """(writes [(tbl, key, want)], lines, refusals)."""
    writes, lines, refusals = [], [], []
    for t in ("itempattern", "craftrecipes", "gm_audit") + (("lootpool",) if with_drop else ()):
        if not table(db, t):
            refusals.append(f"no {t} table here")
    if refusals:
        return writes, lines, refusals
    recipes = [(skill, level, json.loads(ings), product) for skill, level, ings, product in
               db.execute("SELECT skill, level, ingredients, product FROM craftrecipes ORDER BY level, product")]
    ids, sid = [], None
    for name, m in MODS.items():
        found = db.execute("SELECT patternid, itemdesc FROM itempattern WHERE itemname = ? AND equipslot = -1", (name,)).fetchall()
        if len(found) != 1:
            refusals.append(f"{len(found)} component records named {name!r}, not one")
            continue
        mid, desc = found[0]
        ids.append(mid)
        if name == STUDS:
            sid = mid
        lines.append(f"{name}: item {mid}")
        if m["desc"] and desc != m["desc"]:
            writes.append(("itempattern", {"patternid": mid}, {"itemdesc": m["desc"]}))
            lines.append(f"  description: {desc!r}\n            -> {m['desc']!r}")
        targets, other = [], []
        for skill, level, ings, product in recipes:
            if any(i.get("item") == mid for i in ings):
                (targets if skill == m["skill"] else other).append((level, product))
        if other:
            refusals.append(f"{len(other)} recipes of another craft take {name}: not looked at")
        if len(targets) != m["expect"]:
            refusals.append(f"{len(targets)} {name} products here, not {m['expect']}")
        moves = m["moves"]
        shown = ([m["keep"]] if m["keep"] else []) + list(moves) + list(moves.values())
        lines.append(f"  products: {len(targets)} (" + ", ".join(f"{o} -> {n}" for o, n in moves.items()) + ")")
        for lvl, pid in targets:
            ids.append(pid)
            cur = row(db, "itempattern", {"patternid": pid})
            if cur is None:
                refusals.append(f"recipe product {pid} (trade level {lvl}) has no itempattern row")
                continue
            v = lambda c: cur.get(c) or 0  # noqa: E731
            olds, news = [v(c) for c in moves], [v(c) for c in moves.values()]
            kept = [v(m["keep"])] if m["keep"] else []
            if all(x == 0 for x in olds) and news[0] > 0 and len(set(news + kept)) == 1:
                want, after = None, cur
            elif olds[0] > 0 and len(set(olds + kept)) == 1 and all(x == 0 for x in news):
                want = dict({o: None for o in moves}, **{n: v(o) for o, n in moves.items()})
                after = dict(cur, **want)
            else:
                refusals.append(f"{pid} {cur['itemname']!r}: " + " ".join(f"{c} {v(c)}" for c in shown) + ", not the grid's pair")
                continue
            lines.append(f"  {pid:>7} TL{lvl:<2} {cur['itemname']:<34} AC {cur['AC']:>3}  "
                         + "  ".join(f"{c} {fig(cur.get(c))}->{fig(after.get(c))}" for c in shown))
            if want:
                writes.append(("itempattern", {"patternid": pid}, want))
    if sid is None:
        return writes, lines, refusals
    if table(db, "lootpool"):
        have = db.execute("SELECT id, region, weight, minlevel, maxlevel FROM lootpool WHERE category = ? AND patternid = ?",
                          (POOL, sid)).fetchall()
        if have:
            lines.append(f"{STUDS} drop: in the {POOL!r} pool already ({', '.join(f'row {r[0]}: region {r[1]}, weight {r[2]}, levels {r[3]}-{r[4]}' for r in have)})"
                         + ("; --with-drop adds nothing" if with_drop else ""))
        else:
            like = db.execute("SELECT l.region, l.weight, l.minlevel, l.maxlevel FROM lootpool l JOIN itempattern p ON "
                              "p.patternid = l.patternid WHERE l.category = ? AND p.itemname = ? ORDER BY l.id", (POOL, LIKE)).fetchone()
            if like is None:
                (refusals if with_drop else lines).append(f"{STUDS} drop: not in the {POOL!r} pool, nor {LIKE} to copy")
            else:
                region, weight, lo, hi = like
                nxt = db.execute("SELECT COALESCE(MAX(id), 0) + 1 FROM lootpool").fetchone()[0]
                lines.append(f"{STUDS} drop: not in the {POOL!r} pool; --with-drop adds row {nxt}: region {region}, weight {weight}, "
                             f"levels {lo}-{hi} ({LIKE}'s)" + ("" if with_drop else " (not asked)"))
                if with_drop:
                    writes.append(("lootpool", {"id": nxt}, {"category": POOL, "region": region, "npc_name": "", "patternid": sid,
                                                             "weight": weight, "minlevel": lo, "maxlevel": hi,
                                                             "source": "petition-5200"}))
    if table(db, "charinventory") and ids:
        q = ",".join("?" * len(ids))
        n, chars = db.execute(f"SELECT count(*), count(DISTINCT serverid) FROM charinventory WHERE patternid IN ({q})", ids).fetchone()
        lines.append(f"  held (the modifiers and their products): {n} items, {chars} characters")
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
    ap.add_argument("--with-drop", action="store_true", help="also put the Rallosian Studs in the modifiers loot pool if missing (owner's approval)")
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
        writes, lines, refusals = plan(db, a.with_drop)
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
