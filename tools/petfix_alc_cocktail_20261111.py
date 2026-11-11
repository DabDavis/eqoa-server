#!/usr/bin/env python3
"""Severupidus' Cocktail of Torment, the alchemist's level 60 AoE potion, beside the Knack 60 (petition 1577, Elas,
2026-11-11: "Missing Alc 60 epic for AoE pot, Severupidus' Cocktail of Torment. The Knack 60 spell is there, just
missing the AoE epic spell").

The alchemist's two 60s, as EQOAEmu's workbook has them (both "Scroll, Quest", ALC, level 60): Biddleby's Enduring
Knack (the Knack line's, +700 HP and 70 HoT) and Severupidus' Cocktail of Torment (the Cocktail line's: summons three
Severupidus' Cocktails, a thrown potion of 1,800 fire on the target and all within 10). Here the Knack 60 is in every
Spell Archivist's shop, so his "Grant basic spells" writes it into every level 60 alchemist's book; the Cocktail of
Torment is only the 60 epic reward's option (quests 101121 and 101133, Ediopolus: "spells": "701222, 760105", the
summon and Severupidus' Fire Potion, the castable throw of petition 1181), its classspells row "eqoa-data epic", which
no grant gives. So an alchemist with the Knack 60 and no epic pick of the Cocktail has no AoE 60.

This gives what the epic's Cocktail option gives, the two spells, to Elas and to every level 60 alchemist whose book
has the Knack 60 (Biddleby's Enduring Knack) and lacks either (--petitioner-only: Elas alone). Each spell goes into
the book off the bar at its next display index, as the scribe and the Archivist write one (spells row: onHotBar 0,
whereonBar -1, unk1 0, showhide -1). Characters by name and serverid (Elas: "Elas", 701; refused when that serverid is
another character, skipped with a line where there is none, as on the Live shard), spells by name (refused when a name
is not exactly one record). Report only by default; --apply in one transaction, a gm_audit line a row, log
alc_cocktail_1111_log; --undo exact (a row moved since, put on the bar, is left and said). The book is read at login:
a character online sees them from its next login; no restart.

    tools/petfix_alc_cocktail_20261111.py --db eqoa.db [--apply | --undo] [--petitioner-only]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "alc_cocktail_1111_log"
DETAIL = ("petition 1577 (Elas): Severupidus' Cocktail of Torment and Severupidus' Fire Potion, the 60 epic's Cocktail "
          "option, to the alchemists with the Knack 60: tools/petfix_alc_cocktail_20261111.py")
WHO, VIA = "claude", "tool"
ALCHEMIST = 14
PETITIONER = ("Elas", 701)
KNACK60 = "Biddleby's Enduring Knack"
SPELLS = ["Severupidus' Cocktail of Torment", "Severupidus' Fire Potion"]
BOOK_END = 252  # the book's display indices end at 252 (handlers/scribe.go nextDisplayIndex)
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    id      INTEGER NOT NULL,   -- the spells row written
    after   TEXT NOT NULL,      -- the row as written (JSON)
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def row_of(db, id):
    cur = db.execute("SELECT * FROM spells WHERE id = ?", (id,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def audit(db, action, id, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, 'spells', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, go_json({"id": id}),
                      None if before is None else go_json(before), None if after is None else go_json(after), detail))
    return cur.lastrowid


def spell_id(db, name, refusals):
    ids = [r[0] for r in db.execute("SELECT spellid FROM spellpattern WHERE spellname = ?", (name,))]
    if len(ids) != 1:
        refusals.append(f"spellpattern has {len(ids)} records named {name!r}, want one")
        return None
    return ids[0]


def plan(db, petitioner_only):
    """(grants [(serverid, name, [(spellid, spellname)])], refusals, notes)."""
    grants, refusals, notes = [], [], []
    for t in ("characters", "spells", "spellpattern"):
        if not table(db, t):
            return grants, [f"no {t} table here"], notes
    want = [(n, spell_id(db, n, refusals)) for n in SPELLS]
    knack = spell_id(db, KNACK60, refusals)
    if refusals:
        return grants, refusals, notes
    who = []
    name, sid = PETITIONER
    r = db.execute("SELECT charName, tclass FROM characters WHERE serverid = ?", (sid,)).fetchone()
    if r is None:
        notes.append(f"{name} ({sid}): no such character here, skipped")
    elif r[0] != name or r[1] != ALCHEMIST:
        refusals.append(f"serverid {sid} is {r[0]!r} of class {r[1]}, not the alchemist {name}")
    else:
        who.append((sid, name))
    if not petitioner_only:
        for s, n in db.execute("SELECT c.serverid, c.charName FROM characters c WHERE c.tclass = ? AND c.level >= 60 AND "
                               "EXISTS (SELECT 1 FROM spells b WHERE b.serverid = c.serverid AND b.spellid = ?) "
                               "ORDER BY c.serverid", (ALCHEMIST, knack)):
            if (s, n) not in who:
                who.append((s, n))
    for s, n in who:
        have = {r[0] for r in db.execute("SELECT spellid FROM spells WHERE serverid = ?", (s,))}
        lack = [(i, sn) for sn, i in want if i not in have]
        if lack:
            grants.append((s, n, lack))
    return grants, refusals, notes


def next_index(db, sid):
    r = db.execute("SELECT MAX(addedorder) FROM spells WHERE serverid = ? AND addedorder < ?", (sid, BOOK_END)).fetchone()
    return 0 if r[0] is None else r[0] + 1


def apply(db, grants):
    db.execute(LOG_SCHEMA)
    for sid, name, lack in grants:
        for spellid, _ in lack:
            at = next_index(db, sid)
            if at >= BOOK_END:
                raise SystemExit(f"{name}'s book is full: nothing written")
            cur = db.execute("INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide) "
                             "VALUES (?, ?, ?, 0, -1, 0, -1)", (sid, spellid, at))
            row = row_of(db, cur.lastrowid)
            a = audit(db, "insert", row["id"], None, row, DETAIL + f" ({name})")
            db.execute(f"INSERT INTO {LOG} (id, after, audit) VALUES (?, ?, ?)", (row["id"], json.dumps(row, sort_keys=True), a))


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, id, after in db.execute(f"SELECT seq, id, after FROM {LOG} ORDER BY seq DESC").fetchall():
        now = row_of(db, id)
        if now is None or json.dumps(now, sort_keys=True) != after:
            left += 1
            print(f"  left: spells {id}, changed since it was written: {now}")
            continue
        db.execute("DELETE FROM spells WHERE id = ?", (id,))
        audit(db, "delete", id, now, None, "undo: " + DETAIL)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if left == 0:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    ap.add_argument("--petitioner-only", action="store_true", help="Elas alone")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"put back {back}, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        grants, refusals, notes = plan(db, a.petitioner_only)
        for r in refusals:
            print("REFUSED:", r)
        if refusals:
            db.execute("ROLLBACK")
            return 1
        for n in notes:
            print(n)
        for sid, name, lack in grants:
            print(f"{name} ({sid}): {', '.join(f'{sn} ({i})' for i, sn in lack)}")
        if not grants:
            db.execute("ROLLBACK")
            print("applied already (every one has both): nothing done")
            return 0
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"report only: {sum(len(l) for _, _, l in grants)} spells to {len(grants)} characters (--apply to write)")
            return 0
        apply(db, grants)
        db.execute("COMMIT")
        print(f"applied: {sum(len(l) for _, _, l in grants)} spells to {len(grants)} characters")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
