#!/usr/bin/env python3
"""Wisp's "mana" path spells (petition 1586, Wisp, 2026-11-11: "Requested Path spell mana spells towering mind,
aktrans will and fortress but they werent added to spell book").

The three are the wizard's self-power line: Towering Mind (700653, the level 49 path reward's option, the Spell
Archivist's "path of Mental Focus": Mental Focus, Mental Surge, Mental Boost, Heightened Mind, Towering Mind) and
Aktrana's Tower of Will and Aktrana's Cerebral Fortress (701099, 701098: the level 60 epic's Power path, EQOA-Data
Information/epics_60.json; here the options of the wizard's epic reward, quests 101120 and 101132, Ediopolus). Why they
were not written: the Archivist's paths are the 20/30/40/49 path quests' (handlers/archivistpaths.go: pathRewards
reads the 30,40,49 reward quests and the level 20 class quests only, the rule the GMs answered petition 1487 with), so
his Mind path ends at Towering Mind, which Wisp's book has; the 60 epic's spells come only from the epic quest's one
pick, and Wisp has done neither epic quest (no charquests row for 101120 or 101132). No fault in data or code: this
writes what was asked.

Into Wisp's book (by name and serverid: "Wisp", 676; refused when 676 is another character, skipped with a line where
there is none, as on the Live shard) each of the three spells it lacks (by name; refused when a name is not exactly
one record), off the bar at its next display index, as the scribe and the Archivist write one. Report only by default;
--apply in one transaction, a gm_audit line a row, log wisp_power_path_1111_log; --undo exact (a row moved since, put
on the bar, is left and said). The book is read at login: from Wisp's next login; no restart.

    tools/petfix_wisp_power_path_20261111.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "wisp_power_path_1111_log"
DETAIL = ("petition 1586 (Wisp): Towering Mind, Aktrana's Tower of Will and Aktrana's Cerebral Fortress into Wisp's "
          "book: tools/petfix_wisp_power_path_20261111.py")
WHO, VIA = "claude", "tool"
WIZARD = 13
CHARACTER = ("Wisp", 676)
SPELLS = ["Towering Mind", "Aktrana's Tower of Will", "Aktrana's Cerebral Fortress"]
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


def plan(db, _unused=False):
    """(grants [(serverid, name, [(spellid, spellname)])], refusals, notes)."""
    grants, refusals, notes = [], [], []
    for t in ("characters", "spells", "spellpattern"):
        if not table(db, t):
            return grants, [f"no {t} table here"], notes
    want = [(n, spell_id(db, n, refusals)) for n in SPELLS]
    if refusals:
        return grants, refusals, notes
    name, sid = CHARACTER
    r = db.execute("SELECT charName, tclass FROM characters WHERE serverid = ?", (sid,)).fetchone()
    if r is None:
        notes.append(f"{name} ({sid}): no such character here, skipped")
        return grants, refusals, notes
    if r[0] != name or r[1] != WIZARD:
        return grants, [f"serverid {sid} is {r[0]!r} of class {r[1]}, not the wizard {name}"], notes
    have = {x[0] for x in db.execute("SELECT spellid FROM spells WHERE serverid = ?", (sid,))}
    for sn, i in want:
        if i in have:
            notes.append(f"{name} ({sid}) has {sn} ({i}) already")
    lack = [(i, sn) for sn, i in want if i not in have]
    if lack:
        grants.append((sid, name, lack))
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
        grants, refusals, notes = plan(db)
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
            print("applied already (Wisp has all three): nothing done")
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
