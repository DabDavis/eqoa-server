#!/usr/bin/env python3
"""The wizard's Sorcerer master class: Flame Mastery gives Flaming Fury and Icy Mastery Icy Fury (petition 1990,
Kyros, 2026-11-13: "Sorc MCA - purchased Fire and Thunder but got Ice and Thunder MCA AOE's").

The cause is the catalog's rows, not cmbuy: entity/cmbuy.go grants a rank's own add_spells. The import
(tools/seed_masteries.py, "eqoa-data cms; eqoaemu workbook") took the abilities from EQOA-Data's ranks in their order
(Sorcerer 2 "Icy Fury AoE", Sorcerer 3 "Flaming Fury AoE", Sorcerer 4 "Thunderous Fury AoE") and the names from the
workbook (Flame, Icy, Lightning Mastery), so the two first were crossed:

  cmcatalog "Flame Mastery" (7112)  adds Icy Fury (701159)      -> Flaming Fury (701141)
  cmcatalog "Icy Mastery"   (7113)  adds Flaming Fury (701141)  -> Icy Fury (701159)

Lightning Mastery gives Thunderous Fury, as named, and is left. A character that owns both ranks holds both spells
and is left. A character that owns ONE of them has the other's spell: its book row (spells) takes the spell it paid
for in place, its book place and hot bar spot kept, so a later reset (respec.go: it takes back a rank's add_spells)
finds it. A book not as expected (the paid-for spell there already, or the crossed one missing) is left and said, as
are the rows a deleted character left behind (no characters row: Kyros, serverid 819, the petition's, was deleted
on the Heirloom Test Server between his petition at 19:01 and the 19:46 UTC snapshot; his account holds Kytank).

Rows are looked up by name (the ranks under the Sorcerer master class of the wizard's mask, the spells by
spellpattern.spellname): refused, nothing written, unless there is one of each and the ranks are as the import wrote
them (or as this tool writes them: applied already). Report only by default; --apply in one transaction, a gm_audit
line a row, log sorcerer_fury_1113_log; --undo exact (a row changed since is left and said). cmcatalog is read at
the start (and at a GM panel edit of it: Masteries' reload); a book at the next login (apply with the characters
logged out, or they log in again).

    tools/sorcerer_fury_20261113.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "sorcerer_fury_1113_log"
DETAIL = "Sorcerer's Flame Mastery gives Flaming Fury and Icy Mastery Icy Fury (petition 1990): " \
         "tools/sorcerer_fury_20261113.py"
WHO, VIA = "dab", "tool"
WIZARD = 1 << 13
MASTER = "Sorcerer"
FLAME, ICY = "Flame Mastery", "Icy Mastery"
FLAMING_FURY, ICY_FURY = "Flaming Fury", "Icy Fury"

KEYS = {"cmcatalog": ("id",), "spells": ("id",)}

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,      -- cmcatalog or spells
    rowkey  TEXT NOT NULL,      -- the row's key (JSON)
    before  TEXT,               -- the row before (JSON), NULL for none
    after   TEXT,               -- the row as written (JSON), NULL for deleted
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def rows(db, sql, args=()):
    cur = db.execute(sql, args)
    cols = [d[0] for d in cur.description]
    return [dict(zip(cols, r)) for r in cur.fetchall()]


def key_of(tbl, row):
    return {k: row[k] for k in KEYS[tbl]}


def get(db, tbl, key):
    where = " AND ".join(f"{k} = ?" for k in key)
    r = rows(db, f"SELECT * FROM {tbl} WHERE {where}", list(key.values()))
    return r[0] if r else None


def go_json(m):
    return json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, tbl, key, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() \
        if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key),
                      None if before is None else go_json(before), None if after is None else go_json(after), detail))
    return cur.lastrowid


def spell_id(db, name):
    """The one spellpattern row of the name: (id, refusal)."""
    got = [r[0] for r in db.execute("SELECT spellid FROM spellpattern WHERE spellname = ? ORDER BY spellid", (name,))]
    if len(got) != 1:
        return 0, f"spellpattern {name!r}: {len(got)} rows {got}, not one"
    return got[0], None


def changes(db):
    """(changes, lines, refusals, applied): changes a list of (tbl, key, after)."""
    lines, refusals = [], []
    for t in ("cmcatalog", "charcms", "spells", "spellpattern", "gm_audit"):
        if not table(db, t):
            return [], lines, [f"no {t} table here"], False
    flaming, r1 = spell_id(db, FLAMING_FURY)
    icy, r2 = spell_id(db, ICY_FURY)
    if r1 or r2:
        return [], lines, [r for r in (r1, r2) if r], False
    masters = rows(db, "SELECT * FROM cmcatalog WHERE category = 4 AND mastery = ? AND rank = 1 AND name = ? "
                       "AND classes & ? ORDER BY id", (MASTER, MASTER, WIZARD))
    if len(masters) != 1:
        return [], lines, [f"cmcatalog: {len(masters)} Sorcerer master class rows of the wizard's, not one"], False
    top = masters[0]["id"]
    ranks = {}
    for name in (FLAME, ICY):
        got = rows(db, "SELECT * FROM cmcatalog WHERE parent = ? AND name = ? ORDER BY id", (top, name))
        if len(got) != 1:
            refusals.append(f"cmcatalog: {len(got)} rows {name!r} under {top}, not one")
        else:
            ranks[name] = got[0]
    if refusals:
        return [], lines, refusals, False
    # what each rank gave (as imported) and gives (as named)
    want = {FLAME: (FLAMING_FURY, flaming), ICY: (ICY_FURY, icy)}
    crossed = {FLAME: (ICY_FURY, icy), ICY: (FLAMING_FURY, flaming)}
    out, state = [], {}
    for name, row in ranks.items():
        now = (row["adds"], row["add_spells"])
        if now == (f"{want[name][0]}@60", str(want[name][1])):
            state[name] = "now"
        elif now == (f"{crossed[name][0]}@60", str(crossed[name][1])):
            state[name] = "was"
            out.append(("cmcatalog", key_of("cmcatalog", row),
                        {**row, "adds": f"{want[name][0]}@60", "add_spells": str(want[name][1])}))
            lines.append(f"cmcatalog {row['id']} {name!r}: adds {row['adds']!r} ({row['add_spells']}) -> "
                         f"'{want[name][0]}@60' ({want[name][1]})")
        else:
            refusals.append(f"cmcatalog {row['id']} {name!r} adds {now}: neither as the import wrote it nor as this "
                            "tool writes it")
    if refusals:
        return [], lines, refusals, False
    if len(set(state.values())) != 1:
        refusals.append(f"cmcatalog: one rank crossed and the other not ({state}): looked at by hand first")
        return [], lines, refusals, False
    # the books of the owners of one rank alone
    owners = {}
    for name, row in ranks.items():
        for (sid,) in db.execute("SELECT serverid FROM charcms WHERE cmid = ?", (row["id"],)):
            owners.setdefault(sid, set()).add(name)
    for sid in sorted(owners):
        held = owners[sid]
        if len(held) != 1:
            continue
        (name,) = held
        good, bad = want[name][1], crossed[name][1]
        book = {r["spellid"]: r for r in rows(db, "SELECT * FROM spells WHERE serverid = ? AND spellid IN (?, ?)",
                                                (sid, good, bad))}
        who = db.execute("SELECT charName FROM characters WHERE serverid = ?", (sid,)).fetchone()
        if who is None:
            lines.append(f"character {sid} owns {name} alone but is deleted (no characters row; its charcms and "
                         f"spells rows stayed): left")
            continue
        who = who[0]
        if bad in book and good not in book:
            b = book[bad]
            out.append(("spells", key_of("spells", b), {**b, "spellid": good}))
            lines.append(f"spells {b['id']}: {who} ({sid}) owns {name} alone: {crossed[name][0]} ({bad}) -> "
                         f"{want[name][0]} ({good}), book place {b['addedorder']}, hot bar "
                         f"{b['whereonBar'] if b['onHotBar'] else '-'} kept")
        elif good in book and bad not in book:
            lines.append(f"{who} ({sid}) owns {name} alone and holds {want[name][0]} already: left")
        else:
            lines.append(f"{who} ({sid}) owns {name} alone; book has {sorted(book)}: left, looked at by hand")
    both = sorted(s for s, h in owners.items() if len(h) == 2)
    lines.append(f"{len(both)} character(s) own both ranks and hold both spells: left {both}")
    applied = not out and state[FLAME] == "now"
    return out, lines, refusals, applied


def write(db, tbl, key, after):
    before = get(db, tbl, key)
    sets = [c for c in after if before.get(c) != after[c]]
    if not sets:
        return None
    where = " AND ".join(f"{k} = ?" for k in key)
    db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in sets)} WHERE {where}",
               [after[c] for c in sets] + list(key.values()))
    return "update", before, get(db, tbl, key)


def apply(db, out):
    db.execute(LOG_SCHEMA)
    n = 0
    for tbl, key, after in out:
        r = write(db, tbl, key, after)
        if r is None:
            continue
        action, before, now = r
        a = audit(db, action, tbl, key, before, now, DETAIL)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                   (tbl, json.dumps(key, sort_keys=True), json.dumps(before, sort_keys=True),
                    json.dumps(now, sort_keys=True), a))
        n += 1
    return n


def undo(db):
    """(back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, tbl, rowkey, before, after in db.execute(
            f"SELECT seq, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key = json.loads(rowkey)
        now = get(db, tbl, key)
        if now is None or json.dumps(now, sort_keys=True) != after:
            left += 1
            print(f"  left: {tbl} {key}, changed since it was written: {now}")
            continue
        was = json.loads(before)
        where = " AND ".join(f"{k} = ?" for k in key)
        db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in was)} WHERE {where}",
                   list(was.values()) + list(key.values()))
        audit(db, "undo", tbl, key, now, get(db, tbl, key), "undo: " + DETAIL)
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
            print(f"put back {back}, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        out, lines, refusals, applied = changes(db)
        for line in lines:
            print(line)
        if applied:
            print("applied already (the rows are as this tool writes them): nothing done")
            return 0
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not a.apply:
            print(f"report only: {len(out)} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            n = apply(db, out)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {n} rows, in gm_audit and {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
