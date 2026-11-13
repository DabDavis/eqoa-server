#!/usr/bin/env python3
"""Barks and roots that are vendor trash listed with the consumables (petition 1991, Zerkziee, 2026-11-13: "is kolbok
root and kolbok bark supposed to stack in consumables? Its not consumable this is from the kobold instance").

What lists an item with the usable items is its record's item type (itempattern.procanim, CLIENT's VIItem +0x4C):
VIInfoText::IsUsable is type 2, 5, 7 or 8, and IsMisc (the misc items) is any other type with no count and no wear
slot (decomp modules/client/src/viinfotext.cpp). tools/seed_trash_items.py made each creature's two trophies on a
template record of their part, copied whole, and the bark's and the root's template was "weak roots" (1014), a
potion of the usable type 2 for every class and race; so its 23 barks and roots (the kolbok bark and root the Clan
Kolbok of Warren's Den drop among them) are type 2, while its other 884 trophies are type 0 with no class or race
mask. This gives the 23 the type and masks of a trophy of the same tool, the pristine kobold ear (the kobold
instance's other trash): procanim 0, classuse 0, raceuse 0. Nothing else of them changes (name, price, icon, stack).

Rows are looked up by name: refused, nothing written, unless every one of the 23 is there once, as the seed made it
(a trophy: not worn, no count, stacking to 99, "A merchant will pay for it."), of type 2 with every class and race
(or as this tool writes it: applied already), and the ear is type 0 with no masks; and no other trophy of that
description is of a usable type. Report only by default; --apply in one transaction, a gm_audit line a row, log
bark_root_type_1113_log; --undo exact (a row changed since is left and said). An item's record is read at each
inventory send, so a held one moves to the misc items at its owner's next login; no restart.

    tools/bark_root_type_20261113.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "bark_root_type_1113_log"
DETAIL = "Trophy barks and roots of item type 0, not usable (petition 1991): tools/bark_root_type_20261113.py"
WHO, VIA = "dab", "tool"
TROPHY = "%. A merchant will pay for it."
MODEL = "pristine kobold ear"
FIELDS = ("procanim", "classuse", "raceuse")
WAS = {"procanim": 2, "classuse": 32767, "raceuse": 1023}
USABLE = (2, 5, 7, 8)
NAMES = (
    "fine arborian bark", "fine deadwood bark", "fine treant bark", "fresh arborian root", "fresh deadwood root",
    "fresh fungal root", "fresh fungusman root", "fresh treant root", "fungal root", "pristine arborian bark",
    "pristine arborian root", "pristine fungal root", "pristine fungusman root", "pristine kolbok bark",
    "pristine kolbok root", "pristine rotwood bark", "pristine rotwood root", "pristine treant bark",
    "pristine treant root", "rotting treant bark", "sapling bark", "sapling root", "withered treant root",
)

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    patternid INTEGER NOT NULL,
    before  TEXT NOT NULL,      -- the row before (JSON)
    after   TEXT NOT NULL,      -- the row as written (JSON)
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def rows(db, sql, args=()):
    cur = db.execute(sql, args)
    cols = [d[0] for d in cur.description]
    return [dict(zip(cols, r)) for r in cur.fetchall()]


def get(db, pid):
    r = rows(db, "SELECT * FROM itempattern WHERE patternid = ?", (pid,))
    return r[0] if r else None


def go_json(m):
    return json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, pid, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() \
        if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, 'itempattern', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, go_json({"patternid": pid}),
                      go_json(before), go_json(after), detail))
    return cur.lastrowid


def trophy(r):
    return r["equipslot"] == -1 and r["craft"] == 0 and r["maxstack"] == 99 and r["itemdesc"].endswith(
        ". A merchant will pay for it.")


def changes(db):
    """(changes, lines, refusals, applied): changes a list of (patternid, after)."""
    lines, refusals = [], []
    for t in ("itempattern", "gm_audit"):
        if not table(db, t):
            return [], lines, [f"no {t} table here"], False
    model = rows(db, "SELECT * FROM itempattern WHERE itemname = ?", (MODEL,))
    if len(model) != 1 or not trophy(model[0]) or any(model[0][f] != 0 for f in FIELDS):
        return [], lines, [f"itempattern {MODEL!r}: {[{f: m[f] for f in FIELDS} for m in model]}, not one trophy "
                           "of type 0 with no masks"], False
    want = {f: model[0][f] for f in FIELDS}
    out, now = [], 0
    for name in NAMES:
        got = rows(db, "SELECT * FROM itempattern WHERE itemname = ?", (name,))
        if len(got) != 1 or not trophy(got[0]):
            refusals.append(f"itempattern {name!r}: {len(got)} rows, not one trophy")
            continue
        r = got[0]
        cur = {f: r[f] for f in FIELDS}
        if cur == want:
            now += 1
        elif cur == WAS:
            out.append((r["patternid"], {**r, **want}))
            held = db.execute("SELECT COUNT(*), COALESCE(SUM(stackleft), 0) FROM charinventory WHERE patternid = ?",
                              (r["patternid"],)).fetchone() if table(db, "charinventory") else (0, 0)
            lines.append(f"itempattern {r['patternid']} {name!r}: type {r['procanim']} (usable), classes "
                         f"{r['classuse']}, races {r['raceuse']} -> type 0, 0, 0 as {MODEL!r} "
                         f"({held[0]} stack(s) held, {held[1]} in all)")
        else:
            refusals.append(f"itempattern {r['patternid']} {name!r}: {cur}, neither as seeded nor as this tool writes")
    # any other trophy of a usable type is one this tool does not know
    for r in rows(db, f"SELECT patternid, itemname, procanim FROM itempattern WHERE itemdesc LIKE ? "
                      f"AND procanim IN ({', '.join('?' * len(USABLE))})", (TROPHY, *USABLE)):
        if r["itemname"] not in NAMES:
            refusals.append(f"itempattern {r['patternid']} {r['itemname']!r}: a trophy of usable type "
                            f"{r['procanim']} this tool does not know: looked at by hand")
    return out, lines, refusals, not out and not refusals and now == len(NAMES)


def apply(db, out):
    db.execute(LOG_SCHEMA)
    n = 0
    for pid, after in out:
        before = get(db, pid)
        sets = [c for c in FIELDS if before[c] != after[c]]
        if not sets:
            continue
        db.execute(f"UPDATE itempattern SET {', '.join(f'{c} = ?' for c in sets)} WHERE patternid = ?",
                   [after[c] for c in sets] + [pid])
        now = get(db, pid)
        a = audit(db, "update", pid, before, now, DETAIL)
        db.execute(f"INSERT INTO {LOG} (patternid, before, after, audit) VALUES (?, ?, ?, ?)",
                   (pid, json.dumps(before, sort_keys=True), json.dumps(now, sort_keys=True), a))
        n += 1
    return n


def undo(db):
    """(back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, pid, before, after in db.execute(
            f"SELECT seq, patternid, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        now = get(db, pid)
        if now is None or json.dumps(now, sort_keys=True) != after:
            left += 1
            print(f"  left: itempattern {pid}, changed since it was written")
            continue
        was = json.loads(before)
        db.execute(f"UPDATE itempattern SET {', '.join(f'{c} = ?' for c in FIELDS)} WHERE patternid = ?",
                   [was[c] for c in FIELDS] + [pid])
        audit(db, "undo", pid, now, get(db, pid), "undo: " + DETAIL)
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
