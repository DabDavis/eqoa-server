#!/usr/bin/env python3
"""The Jal'Raeth's Sacred Scripture and Path of the Devout as the owner asked on 2026-11-13 ("Sacred scripture should
be +10 Wis +20wis max for 25 CMs ... Path of the devoted is a 1 min buff 30 min recast. It should be just raw stats
not a buff"), over tools/jalraeth_masteries_20261111.py's rows:

  cmcatalog 2164 Sacred Scripture   one rank now: cost 25 (was 10), WIS +10 and WIS cap +20 (was WIS +10), named
                                    "Sacred Scripture" (was "Sacred Scripture 1")
  cmcatalog 2165 Sacred Scripture 2 deleted (cost 15, WIS +10 more: the old line's second rank)
  cmcatalog 2167 Path of the Devout its stats for good, as the rank's own effects (the window's slots: HP, AC, OFF
                                    and DEF MOD, STA, WIS): max HP +500, AC +250, OFF MOD +2, DEF MOD +2, STA +25,
                                    WIS +25, the buff's six parts at their figures; it adds no ability any more
                                    (adds and add_spells emptied)

What was bought keeps what was paid for:

  - a character owning 2165 (and so 2164, its parent) paid 10 + 15 = 25, the new rank's cost: its 2165 row goes
    and its 2164 row's cost becomes what it paid for both (charcms.cost is what a reset gives back: respec.go);
  - a character owning 2164 alone paid 10 and keeps the rank, now the whole line, at what it paid;
  - the ability Path of the Devout (760170) leaves every book that holds it (spells), and a kept buff of it
    (charbuffs: what a logout saved) goes, so nobody has the buff's minute on top of the stats. The catalog's
    remove_spells is NOT used: it acts at a purchase only (cmbuy.go), never on a rank owned already, and a
    reset (respec.go) undoes a rank's book change, so a removal there would put the spell back in the book.
    The spell's records (spellpattern, spelleffect 760170) stay, unused.

Refused, nothing written, unless the three rows are as the 2026-11-11 tool wrote them (or as this tool writes them:
applied already), racedefs 30 is the Jal'Raeth, and every 2165 owner owns 2164. Report only by default; --apply in
one transaction, a gm_audit line a row, log devoted_fix_1113_log; --undo exact (a row changed since is left and
said). cmcatalog is read at the start (and at a GM panel edit of it); a book and a kept buff at the next login.

    tools/devoted_fix_20261113.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "devoted_fix_1113_log"
DETAIL = "The Jal'Raeth's Sacred Scripture one rank, Path of the Devout raw stats (the owner, 2026-11-13): " \
         "tools/devoted_fix_20261113.py"
WHO, VIA = "dab", "tool"
JALRAETH = 30
SCRIPTURE, SCRIPTURE2, PATH_RANK, PATH_SPELL = 2164, 2165, 2167, 760170

# the rows as tools/jalraeth_masteries_20261111.py wrote them (the fields this tool reads or changes)
WAS = {
    SCRIPTURE: {"name": "Sacred Scripture 1", "parent": 0, "cost": 10, "level": 6, "effects": "WIS:10",
                "adds": "", "add_spells": ""},
    SCRIPTURE2: {"name": "Sacred Scripture 2", "parent": SCRIPTURE, "cost": 15, "level": 20, "effects": "WIS:10"},
    PATH_RANK: {"name": "Path of the Devout", "parent": 2166, "cost": 75, "level": 60, "effects": "",
                "adds": "Path of the Devout@60", "add_spells": str(PATH_SPELL), "removes": "", "remove_spells": ""},
}

SCRIPTURE_DESC = "The scriptures of Envar's temple, learned by heart, deepen a Jal'Raeth's wisdom. WIS +10, WIS cap +20."
PATH_DESC = "The devout walk their path untouched. Max HP +500, AC +250, Off Mod +2, Def Mod +2, STA +25, WIS +25."

# what this tool makes of them
NOW = {
    SCRIPTURE: {"name": "Sacred Scripture", "cost": 25, "effects": "WIS:10,WISMax:20", "description": SCRIPTURE_DESC},
    PATH_RANK: {"effects": "HP:500,AC:250,Off Mod:2,Def Mod:2,STA:25,WIS:25", "adds": "", "add_spells": "",
                "description": PATH_DESC},
}

KEYS = {"cmcatalog": ("id",), "charcms": ("serverid", "cmid"), "spells": ("id",), "charbuffs": ("serverid", "seq")}

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,      -- cmcatalog, charcms, spells or charbuffs
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


def matches(row, want):
    return row is not None and all(row.get(k) == v for k, v in want.items())


def changes(db):
    """(changes, lines, refusals, applied): changes a list of (tbl, key, after) with after None for a delete."""
    lines, refusals = [], []
    for t in ("cmcatalog", "charcms", "spells", "charbuffs", "racedefs", "gm_audit"):
        if not table(db, t):
            return [], lines, [f"no {t} table here"], False
    rd = db.execute("SELECT name FROM racedefs WHERE id = ?", (JALRAETH,)).fetchone()
    if rd is None or rd[0] != "Jal'Raeth":
        return [], lines, [f"racedefs {JALRAETH} is {rd!r}, not the Jal'Raeth"], False
    s1, s2, path = (get(db, "cmcatalog", {"id": i}) for i in (SCRIPTURE, SCRIPTURE2, PATH_RANK))
    books = rows(db, "SELECT * FROM spells WHERE spellid = ? ORDER BY id", (PATH_SPELL,))
    buffs = rows(db, "SELECT * FROM charbuffs WHERE spellid = ? ORDER BY serverid, seq", (PATH_SPELL,))
    owners2 = rows(db, "SELECT * FROM charcms WHERE cmid = ? ORDER BY serverid", (SCRIPTURE2,))
    if s2 is None and matches(s1, NOW[SCRIPTURE]) and matches(path, NOW[PATH_RANK]) and not books and not buffs \
            and not owners2:
        return [], lines, refusals, True
    for i, row in ((SCRIPTURE, s1), (SCRIPTURE2, s2), (PATH_RANK, path)):
        if not matches(row, WAS[i]):
            refusals.append(f"cmcatalog {i} is not as tools/jalraeth_masteries_20261111.py wrote it: "
                            f"{ {k: (row or {}).get(k) for k in WAS[i]} }")
    out = []
    # Sacred Scripture: 2164 the one rank, 2165 gone
    out.append(("cmcatalog", {"id": SCRIPTURE}, {**(s1 or {}), **NOW[SCRIPTURE]}))
    out.append(("cmcatalog", {"id": SCRIPTURE2}, None))
    lines.append(f"cmcatalog {SCRIPTURE}: {WAS[SCRIPTURE]['name']!r} cost 10, WIS:10 -> {NOW[SCRIPTURE]['name']!r} "
                 f"cost 25, {NOW[SCRIPTURE]['effects']}")
    lines.append(f"cmcatalog {SCRIPTURE2}: {WAS[SCRIPTURE2]['name']!r} (cost 15, WIS:10) deleted")
    for o in owners2:
        own1 = get(db, "charcms", {"serverid": o["serverid"], "cmid": SCRIPTURE})
        if own1 is None:
            refusals.append(f"charcms: character {o['serverid']} owns {SCRIPTURE2} but not {SCRIPTURE}")
            continue
        paid = own1["cost"] + o["cost"]
        out.append(("charcms", key_of("charcms", own1), {**own1, "cost": paid}))
        out.append(("charcms", key_of("charcms", o), None))
        lines.append(f"charcms: character {o['serverid']} owned both ranks ({own1['cost']} + {o['cost']}): "
                     f"{SCRIPTURE} at {paid}, its {SCRIPTURE2} row gone")
    only1 = db.execute("SELECT serverid, cost FROM charcms WHERE cmid = ? AND serverid NOT IN "
                       "(SELECT serverid FROM charcms WHERE cmid = ?) ORDER BY serverid", (SCRIPTURE, SCRIPTURE2)).fetchall()
    for sid, cost in only1:
        lines.append(f"charcms: character {sid} owns {SCRIPTURE} alone (paid {cost}): kept as it is, now the whole line")
    # Path of the Devout: the stats for good, the ability out of every book
    out.append(("cmcatalog", {"id": PATH_RANK}, {**(path or {}), **NOW[PATH_RANK]}))
    lines.append(f"cmcatalog {PATH_RANK}: adds {WAS[PATH_RANK]['adds']!r} ({PATH_SPELL}) -> effects "
                 f"{NOW[PATH_RANK]['effects']}, adds nothing")
    for b in books:
        out.append(("spells", key_of("spells", b), None))
        lines.append(f"spells {b['id']}: character {b['serverid']}'s Path of the Devout ({PATH_SPELL}, book place "
                     f"{b['addedorder']}, hot bar {b['whereonBar'] if b['onHotBar'] else '-'}) out of the book")
    for b in buffs:
        out.append(("charbuffs", key_of("charbuffs", b), None))
        lines.append(f"charbuffs: character {b['serverid']}'s kept Path of the Devout buff ({b['left_ms']} ms left) gone")
    owners = [r[0] for r in db.execute("SELECT serverid FROM charcms WHERE cmid = ? ORDER BY serverid", (PATH_RANK,))]
    lines.append(f"cmcatalog {PATH_RANK} is owned by {len(owners)} character(s) {owners}: their stats at the next "
                 "catalog read (a start, or a GM panel edit of cmcatalog)")
    return out, lines, refusals, False


def write(db, tbl, key, after):
    before = get(db, tbl, key)
    if after is None:
        where = " AND ".join(f"{k} = ?" for k in key)
        db.execute(f"DELETE FROM {tbl} WHERE {where}", list(key.values()))
        action = "delete"
    elif before is None:
        cols = list(after)
        db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                   [after[c] for c in cols])
        action = "insert"
    else:
        sets = [c for c in after if before.get(c) != after[c]]
        if not sets:
            return
        where = " AND ".join(f"{k} = ?" for k in key)
        db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in sets)} WHERE {where}",
                   [after[c] for c in sets] + list(key.values()))
        action = "update"
    now = get(db, tbl, key)
    return action, before, now


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
                   (tbl, json.dumps(key, sort_keys=True), None if before is None else json.dumps(before, sort_keys=True),
                    None if now is None else json.dumps(now, sort_keys=True), a))
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
        if (now is None) != (after is None) or (now is not None and json.dumps(now, sort_keys=True) != after):
            left += 1
            print(f"  left: {tbl} {key}, changed since it was written: {now}")
            continue
        was = None if before is None else json.loads(before)
        if was is None:
            where = " AND ".join(f"{k} = ?" for k in key)
            db.execute(f"DELETE FROM {tbl} WHERE {where}", list(key.values()))
        elif now is None:
            cols = list(was)
            db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                       [was[c] for c in cols])
        else:
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
        if applied:
            print("applied already (the rows are as this tool writes them): nothing done")
            return 0
        if table(db, LOG):
            print(f"{LOG} is there but the rows are not as written: --undo first")
            return 1
        for line in lines:
            print(line)
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
