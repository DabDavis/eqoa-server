#!/usr/bin/env python3
"""petpass_bossloot_20261120.py: ordinary creatures dropping a boss's loot by way of a COPY of its record (the
2026-11-20 petition pass, bossloot lane; the owner, 2026-11-20: "some mobs are dropping boss loot still"; loot and
drop changes approved for the pass).

A creature killed holding a weapon or a shield drops the catalog's record of that look for its level
(database/helddrops.go, HeldDrops.Pick). A boss's or a raid's own record is kept off that by its patternid: table
heldbarred (the guides' boss and raid loot), the rare pools' items, and a named creature's own table or pool items
(its alone). A COPY of such a record (another patternid, the same look and level) was none of those, so where the
record itself was barred the creature dropped the copy instead. Two kinds of copy:

  copies    (both shards) every import copy (table itemimported: a character transfer's copy of a player's item, or
            the import damper's) in a hand or shield slot. On the Heirloom shard on 2026-11-20 these dropped from
            ordinary creatures: Pneumatic Staff (the copy of the Sky constructs' and the twins' staff, a Plane of Sky
            super rare) off Agra, Rubbip and the Necrologist; Two Horned Ripper (the copy of the Darkwood named's
            own table item) off Overlord Frank; Glacialsunder (the copy of an arctic rare) could off the ice goblin
            shamans. A copy is a player's record: no creature drops one as what it holds.
  ascended  (Heirloom only: on Live the loot lane's tools/petpass_loot_20261120.py part "ascended" bars every
            Ascended record already, petition 5315) every Ascended record (table itemascended: a tuned raid boss's
            drop made a tenth better) in a hand or shield slot. None drops as held today (each is in its boss's own
            table, or the Sky twins' rare pool), but only by that: a table pointed back at its base, as Live's are
            being, would let the copy drop from anything holding its look, as Live petition 5315 found.

Each a heldbarred row (note naming the base, source "petpass1120-bossloot"). Nothing a boss drops changes: heldbarred
is read only by the held drops, never by a loot table, a pool or npcheld. Not touched: what the loot tables and
category pools give (every such path to an ordinary creature found is a GM's or a guide's deliberate drop: the
lane's report lists them).

Report only by default; --apply writes in one transaction, a gm_audit line a row and the log petpass_bossloot_1120_log;
a second --apply does nothing; --undo deletes exactly the rows it made (a row changed since is left and said) and drops
the log. Copies the database first (<db>.bak-<when>-pre-petpassbossloot) unless --no-backup.

WHEN IT TAKES EFFECT: heldbarred is read at the server's start: the next (keep) restart.

    python3 -E -s tools/petpass_bossloot_20261120.py --db eqoa.db --shard live|heirloom [--parts a,b] [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "petpass_bossloot_1120_log"
WHO, VIA = "dab", "tool"
SRC = "petpass1120-bossloot"
DETAIL = "petition pass 2026-11-20, bossloot lane: tools/petpass_bossloot_20261120.py"
PARTS = {"copies": {"live", "heirloom"}, "ascended": {"heirloom"}}
# database.HeldHandSlots and HeldShieldSlot: the records a held drop may be
HELD_SLOTS = (12, 13, 14, 15, 16, 18)


class Refused(Exception):
    pass


def js(v):
    return None if v is None else json.dumps(v, separators=(",", ":"), ensure_ascii=False, sort_keys=True)


def has(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone() is not None


def row(db, tbl, key):
    where = " AND ".join(f'"{k}" = ?' for k in key)
    cur = db.execute(f'SELECT * FROM "{tbl}" WHERE {where}', list(key.values()))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def account(db):
    a = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() \
        if has(db, "accountinfo") else None
    return a[0] if a else 0


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


class Writer:
    def __init__(self, db, apply):
        self.db, self.apply, self.n = db, apply, 0
        self.at, self.acct = now(), account(db)
        if apply and not has(db, LOG):
            db.execute(f"CREATE TABLE {LOG} (seq INTEGER PRIMARY KEY, part TEXT NOT NULL, tbl TEXT NOT NULL, "
                       f"rowkey TEXT NOT NULL, before TEXT, after TEXT)")

    def bar(self, part, pid, note, why):
        key = {"patternid": pid}
        if row(self.db, "heldbarred", key) is not None:
            return False
        self.n += 1
        print(f"  {part}: heldbarred {pid}: {note}")
        if self.apply:
            self.db.execute("INSERT INTO heldbarred (patternid, note, source) VALUES (?, ?, ?)", (pid, note, SRC))
            after = row(self.db, "heldbarred", key)
            self.db.execute(f"INSERT INTO {LOG} (part, tbl, rowkey, before, after) VALUES (?, 'heldbarred', ?, NULL, ?)",
                            (part, js(key), js(after)))
            self.db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, "
                            "after, detail) VALUES (?, ?, ?, ?, '', 'insert', 'heldbarred', ?, NULL, ?, ?)",
                            (self.at, self.acct, WHO, VIA, js(key), js(after), f"{why}; {DETAIL}"))
        return True


def copies_of(db, table):
    """(copy, base, copy's name, base's name) of table's copies in a held slot, by copy."""
    slots = ",".join(str(s) for s in HELD_SLOTS)
    return db.execute(f"""SELECT c.patternid, c.base, p.itemname, b.itemname FROM {table} c
        JOIN itempattern p ON p.patternid = c.patternid LEFT JOIN itempattern b ON b.patternid = c.base
        WHERE p.equipslot IN ({slots}) ORDER BY c.patternid""").fetchall()


def part_copies(db, w):
    if not has(db, "itemimported"):
        print("  copies: no import copies")
        return
    why = "the owner 2026-11-20 (boss loot still dropping): an import copy is a player's record, never a held drop"
    for pid, base, name, bname in copies_of(db, "itemimported"):
        w.bar("copies", pid, f"import copy of {base} ({bname or '?'}): never a held drop (bossloot 2026-11-20)", why)


def part_ascended(db, w):
    if not has(db, "itemascended"):
        print("  ascended: no Ascended records")
        return
    why = "the owner 2026-11-20 (boss loot still dropping): an Ascended record is a tuned raid boss's, never a held drop"
    for pid, base, name, bname in copies_of(db, "itemascended"):
        w.bar("ascended", pid, f"Ascended copy of {base} ({bname or '?'}): a raid boss's loot, never a held drop "
              f"(bossloot 2026-11-20)", why)


def undo(db):
    n, left = 0, 0
    for tbl, rowkey, after in db.execute(f"SELECT tbl, rowkey, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key = json.loads(rowkey)
        if js(row(db, tbl, key)) != after:
            print(f"  {tbl} {rowkey}: changed since this tool wrote it, left as it is")
            left += 1
            continue
        where = " AND ".join(f'"{k}" = ?' for k in key)
        db.execute(f'DELETE FROM "{tbl}" WHERE {where}', list(key.values()))
        n += 1
    db.execute(f"DROP TABLE {LOG}")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'delete', 'heldbarred', '{}', NULL, NULL, ?)",
               (now(), account(db), WHO, VIA, f"undone, {n} rows taken back, {left} left: " + DETAIL))
    print(f"  {n} rows taken back, {left} left")


def backup(path):
    dst = f"{path}.bak-{datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%d-%H%M%S')}-pre-petpassbossloot"
    src, out = sqlite3.connect(path), sqlite3.connect(dst)
    with out:
        src.backup(out)
    src.close()
    out.close()
    print(f"  backup: {dst}")


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    ap.add_argument("--shard", required=True, choices=("live", "heirloom"))
    ap.add_argument("--parts", default="", help="comma list (default: every part of the shard)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    ap.add_argument("--no-backup", action="store_true")
    a = ap.parse_args(argv)
    parts = [p for p in a.parts.split(",") if p] or [p for p, s in PARTS.items() if a.shard in s]
    for p in parts:
        if p not in PARTS or a.shard not in PARTS[p]:
            print(f"refused: part {p!r} is not for the {a.shard} shard")
            return 2
    db = sqlite3.connect(a.db, isolation_level=None)
    if not has(db, "heldbarred"):
        print("refused: no table heldbarred (a server older than held drops' barring)")
        return 1
    if (a.apply or a.undo) and not a.no_backup:
        backup(a.db)
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            if not has(db, LOG):
                print("nothing to undo")
                db.execute("ROLLBACK")
                return 0
            undo(db)
        else:
            w = Writer(db, a.apply)
            for p in parts:
                {"copies": part_copies, "ascended": part_ascended}[p](db, w)
            print(f"{'applied' if a.apply else 'report only'}: {w.n} rows {'written' if a.apply else 'to write'}")
        db.execute("COMMIT" if (a.apply or a.undo) else "ROLLBACK")
    except Refused as e:
        db.execute("ROLLBACK")
        print(f"refused: {e}")
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
