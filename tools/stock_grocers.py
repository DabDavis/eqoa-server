#!/usr/bin/env python3
"""Grocers' food and drink (F3 20261025-172627): every grocer sells the food
line or the drink line, the meals and drinks have a price, and each its HoT
or PoT (what a Use of one gives for a minute: handlers/consume.go).

What a grocer sells is Return Home's (jib's export, --prh): there every
grocer and some barkeeps sell either the whole food line (13 meals, levels 2
to 49) or the whole drink line (14 drinks, 2 to 54), and grocers stand in
pairs, one of each. A meal is an unworn item described as "A hearty meal that
will increase health regeneration.", a drink as "A refreshing beverage that
increases (aids in) power regeneration."; the lines are what Return Home's
food and drink grocers sell.

- An NPC whose Return Home catalog is one line gets that line.
- A grocer (merchant flag, a name "Grocer ...") without one takes it from the
  grocer standing nearest it within --pair: the other line when that one's is
  known, else the food the first by name and the drink the other. A grocer
  with none near sells both lines.
- Only an NPC with no catalog yet is stocked: a GM's catalog is left alone.
- An NPC stocked whose npc_id is 0 (the key into npcitem: NPC.NPCID) is given
  its own row id, as the Return Home import gave our merchants theirs.

No source gives a meal's price or HoT, so both are this server's own: the
price --price-per-level times the item's level, and the HoT (a meal) or PoT
(a drink) the item's level; each only where the item has none (0 or NULL),
so a GM's value stays. A GM may change any of it in the panel.

Every change is recorded in grocer_stock (what it was) and --undo puts it all
back. Both take a copy of the database first (<db>.bak-<when>-pre-grocers).
Report only by default. Restart the server after --apply or --undo: catalogs
and items are read as they are needed, but the NPCs' npc_id at its start.

    tools/stock_grocers.py --prh prh-export.db [--db eqoa.db] [--apply | --undo]
        [--pair 25] [--price-per-level 10]
"""
import argparse
import collections
import datetime
import math
import os
import re
import sqlite3

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
MEAL = re.compile(r"(?i)\bmeal\b.*\bhealth regeneration\b")
DRINK = re.compile(r"(?i)\bbeverage\b.*\bpower regeneration\b")
GROCER = re.compile(r"(?i)^grocer\s")


def kind(desc, equipslot):
    """'food', 'drink' or None for an item pattern."""
    if equipslot != -1:
        return None
    if MEAL.search(desc or ""):
        return "food"
    if DRINK.search(desc or ""):
        return "drink"
    return None


def prh_catalogs(prh, ours):
    """Return Home's food and drink lines (pattern ids in its order) and the
    line each of OUR NPC rows sells there ('food', 'drink' or 'other'), by the
    Return Home import's own map of its rows to ours (prh_import_npcs)."""
    pats = {pid: kind(desc, slot) for pid, desc, slot in
            prh.execute("SELECT patternid, itemdesc, equipslot FROM itempattern")}
    cats = collections.defaultdict(list)
    for npcid, pid in prh.execute("SELECT npcid, patternid FROM npcitem ORDER BY itemid"):
        cats[npcid].append(pid)
    to_ours = dict(ours.execute("SELECT src_id, dst_id FROM prh_import_npcs WHERE dst_id IS NOT NULL"))
    lines = {"food": [], "drink": []}
    sells = {}
    for src, pids in cats.items():
        ks = {pats.get(p) for p in pids}
        k = ks.pop() if len(ks) == 1 and None not in ks else "other"
        if k in lines:
            for p in pids:
                if p not in lines[k]:
                    lines[k].append(p)
        if src in to_ours:
            sells[to_ours[src]] = k
    return lines, sells


def plan(ours, prh, pair):
    lines, sells = prh_catalogs(prh, ours)
    # the lines as OUR patterns: the same id, a meal or drink here too
    here = {pid: (name, level, kind(desc, slot)) for pid, name, level, desc, slot in
            ours.execute("SELECT patternid, itemname, levelreq, itemdesc, equipslot FROM itempattern")}
    for k in lines:
        lines[k] = sorted((p for p in lines[k] if p in here and here[p][2] == k), key=lambda p: (here[p][1], p))
    stocked = {n for (n,) in ours.execute("SELECT DISTINCT npcid FROM npcitem")}
    npcs = [dict(id=i, name=n, type=t or 0, npc_id=k or 0, world=w, x=x, z=z) for i, n, t, k, w, x, z in
            ours.execute("SELECT id, npc_name, npc_type, npc_id, world, x_coord, z_coord FROM npcs")]
    merchant = [n for n in npcs if n["type"] & 1]
    grocers = [n for n in merchant if GROCER.match(n["name"] or "")]
    out = {}  # row id -> (lines, why)
    for n in merchant:
        k = sells.get(n["id"])
        if k in ("food", "drink"):
            out[n["id"]] = ([k], "Return Home's catalog")
    for n in sorted(grocers, key=lambda n: n["name"]):
        if n["id"] in out:
            continue
        near = [m for m in grocers if m is not n and m["world"] == n["world"]
                and math.hypot(m["x"] - n["x"], m["z"] - n["z"]) <= pair]
        if not near:
            out[n["id"]] = (["food", "drink"], "no grocer near: both")
            continue
        m = min(near, key=lambda m: math.hypot(m["x"] - n["x"], m["z"] - n["z"]))
        if m["id"] in out and len(out[m["id"]][0]) == 1:
            other = "drink" if out[m["id"]][0][0] == "food" else "food"
            out[n["id"]] = ([other], f"beside {m['name']} ({out[m['id']][0][0]})")
        else:
            first = n["name"] < m["name"]
            out[n["id"]] = (["food" if first else "drink"], f"paired with {m['name']} by name")
    by_id = {n["id"]: n for n in npcs}
    todo = {i: v for i, v in out.items() if (by_id[i]["npc_id"] or i) not in stocked}
    kept = sorted(by_id[i]["name"] for i in out if i not in todo)
    unflagged = sorted(n["name"] for n in npcs if GROCER.match(n["name"] or "") and not n["type"] & 1)
    return lines, here, todo, by_id, kept, unflagged


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-grocers"
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def ensure_log(db):
    db.execute("""CREATE TABLE IF NOT EXISTS grocer_stock (
        what  TEXT NOT NULL,     -- 'catalog' | 'npc_id' | 'itemcost' | 'HoT' | 'PoT' | 'seq'
        key   INTEGER NOT NULL,  -- the npcitem row, the npcs row or the pattern (0 for seq)
        old   INTEGER            -- what it was (NULL for a catalog row: none)
    )""")


def undo(db):
    ensure_log(db)
    rows = list(db.execute("SELECT what, key, old FROM grocer_stock"))
    db.execute("BEGIN")
    n = collections.Counter()
    for what, key, old in rows:
        if what == "seq":
            if old is None:
                db.execute("DELETE FROM sqlite_sequence WHERE name = 'npcitem'")
            else:
                db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = 'npcitem'", (old,))
            continue
        if what == "catalog":
            db.execute("DELETE FROM npcitem WHERE itemid = ?", (key,))
        elif what == "npc_id":
            db.execute("UPDATE npcs SET npc_id = ? WHERE id = ?", (old, key))
        elif what in ("itemcost", "HoT", "PoT"):
            db.execute(f"UPDATE itempattern SET {what} = ? WHERE patternid = ?", (old, key))
        n[what] += 1
    db.execute("DROP TABLE grocer_stock")
    db.execute("COMMIT")
    return n


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--prh", help="jib's Return Home export (its npcitem and itempattern)")
    ap.add_argument("--apply", action="store_true", help="write the catalogs, prices and HoT/PoT")
    ap.add_argument("--undo", action="store_true", help="put back everything a run wrote")
    ap.add_argument("--no-backup", action="store_true", help="write without a copy first")
    ap.add_argument("--pair", type=float, default=25, help="a grocer this near another is its pair")
    ap.add_argument("--price-per-level", type=int, default=10, help="a meal's or drink's price a level")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 5000")
    if a.undo:
        if not a.no_backup:
            print("backup:", backup(db, a.db))
        n = undo(db)
        print("put back:", dict(n) or "nothing")
        return
    if not a.prh:
        ap.error("--prh is needed")
    if a.apply and db.execute("SELECT name FROM sqlite_master WHERE name = 'grocer_stock'").fetchone():
        ap.error("grocer_stock is there: this database was stocked already (--undo first)")
    prh = sqlite3.connect(f"file:{a.prh}?mode=ro", uri=True)
    lines, here, todo, by_id, kept, unflagged = plan(db, prh, a.pair)
    for k, ps in lines.items():
        print(f"{k} line: " + ", ".join(f"{here[p][0]} ({here[p][1]})" for p in ps))
    why = collections.Counter()
    for i, (ls, w) in sorted(todo.items(), key=lambda kv: by_id[kv[0]]["name"]):
        why["beside a grocer of the other line" if w.startswith("beside") else
            "paired by name" if w.startswith("paired") else w] += 1
        print(f"  {by_id[i]['name']} (row {i}): {' and '.join(ls)}, {w}")
    print(f"{len(todo)} to stock: {dict(why)}")
    if kept:
        print(f"{len(kept)} have a catalog already, left alone: {', '.join(kept)}")
    if unflagged:
        print(f"{len(unflagged)} grocers have no merchant flag (npc_type & 1) and open no window: {', '.join(unflagged)}")
    prices = [(p, a.price_per_level * here[p][1]) for k in lines for p in lines[k]]
    if not a.apply:
        print("(report only: --apply writes them)")
        return
    if not a.no_backup:
        print("backup:", backup(db, a.db))
    ensure_log(db)
    db.execute("BEGIN")
    seq = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'npcitem'").fetchone()
    log = [("seq", 0, seq[0] if seq else None)]
    for p, price in prices:
        cost, hot, pot = db.execute("SELECT itemcost, HoT, PoT FROM itempattern WHERE patternid = ?", (p,)).fetchone()
        if not cost:
            db.execute("UPDATE itempattern SET itemcost = ? WHERE patternid = ?", (price, p))
            log.append(("itemcost", p, cost))
        col, have = ("HoT", hot) if here[p][2] == "food" else ("PoT", pot)
        if not have:
            db.execute(f"UPDATE itempattern SET {col} = ? WHERE patternid = ?", (here[p][1], p))
            log.append((col, p, have))
    rows = 0
    for i, (ls, _) in todo.items():
        key = by_id[i]["npc_id"] or i
        if not by_id[i]["npc_id"]:
            db.execute("UPDATE npcs SET npc_id = ? WHERE id = ?", (i, i))
            log.append(("npc_id", i, by_id[i]["npc_id"]))
        for n, p in enumerate(p for k in ls for p in lines[k]):
            cur = db.execute("""INSERT INTO npcitem (npcid, stackleft, remainHP, remaincharge, patternid,
                equiploc, location, listnumber) VALUES (?, 1, 0, 0, ?, -1, 1, ?)""", (key, p, n))
            log.append(("catalog", cur.lastrowid, None))
            rows += 1
    db.executemany("INSERT INTO grocer_stock (what, key, old) VALUES (?, ?, ?)", log)
    db.execute("COMMIT")
    print(f"wrote {rows} catalog rows for {len(todo)} NPCs and {len(log) - rows - 1} other changes; restart the server")


if __name__ == "__main__":
    main()
