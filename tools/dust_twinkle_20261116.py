#!/usr/bin/env python3
"""Dust's sword keeps its red aura and gains Drizzt's Twinkle's twinkle at the blade's end (the owner, 2026-11-16: "the
twinkle at the end of his sword like drizzt twinkle", "it will keep the red aura it has and stack the twinkle").

The twinkle is a SECOND aura on Dust's own main-hand row (charaura slot 9, database.CharAuraMainHand): charaura's color2,
kind2, source2, nodes2 (go-eqoa-server branch charaura-two), set as Drizzt's Twinkle's (itemaura 715040) second aura is:
kind 3 (particles), source 0x1D875ECD (the monk's Ascendant staff's star flare), along 100 (the blade's tip), colour 0
(its own colours). His row's first aura (colour 0x000000FF, kind 3, source 0, nodes 0 as found) is left exactly as it
is. Character-only: the Heirloom Sword of Twisted Virtue (714382) is held by others, so no itemaura.

Checked as found, else REFUSED and nothing written: exactly one character named Dust, of serverid 197; table charaura
WITH the second-aura columns (a server of charaura-two adds them at its start: run this after it has started once);
gm_audit there; Dust's slot 9 row there. A second already there (a GM's) is changed and its old values kept in the log.
Report by default; --apply in one transaction, a gm_audit line (as the panel's: update, the whole row before and
after), logged in dust_twinkle_1116_log; a second --apply finds nothing to do; --undo puts back exactly what was there,
a gm_audit line, a row changed since left and listed.

The server reads charaura at its start and on the GM panel's "Reload every NPC" (reload_npcs): after --apply (or --undo)
press that; Dust's channel carries it at the next tick, to a client that asked for second auras (vanilla's).

    python3 -E -s tools/dust_twinkle_20261116.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "dust_twinkle_1116_log"
DETAIL = "Dust's sword: its red aura kept, Drizzt's Twinkle's twinkle stacked (the owner, 2026-11-16); tools/dust_twinkle_20261116.py"
WHO, VIA = "dab", "tool"
NAME, SERVERID = "Dust", 197
SLOT = 9  # database.CharAuraMainHand
COLOR2, KIND2, SOURCE2, NODES2 = 0, 3, 0x1D875ECD, 100  # its own colours, particles, the star flare, the blade's tip
SECOND = ("color2", "kind2", "source2", "nodes2")
WANT = dict(zip(SECOND, (COLOR2, KIND2, SOURCE2, NODES2)))

LOG_DDL = f"""CREATE TABLE {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    charid  INTEGER NOT NULL,
    slot    INTEGER NOT NULL,
    before  TEXT NOT NULL,     -- the row before (JSON)
    after   TEXT NOT NULL,     -- the row written (JSON)
    audit   INTEGER NOT NULL
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def columns(db, name):
    return {r[1] for r in db.execute(f"PRAGMA table_info({name})")}


def row(db, charid, slot):
    cur = db.execute("SELECT * FROM charaura WHERE charid = ? AND merc = 0 AND slot = ?", (charid, slot))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return None if m is None else json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, charid, slot, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, 'charaura', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, go_json({"charid": charid, "merc": 0, "slot": slot}),
                      go_json(before), go_json(after), DETAIL))
    return cur.lastrowid


def plan(db):
    """(before or None when nothing to do, lines, refusals)."""
    refusals, lines = [], []
    for t in ("characters", "charaura", "gm_audit"):
        if not table(db, t):
            refusals.append(f"no {t} table here")
    if refusals:
        return None, lines, refusals
    missing = [c for c in SECOND if c not in columns(db, "charaura")]
    if missing:
        refusals.append(f"charaura has no {', '.join(missing)}: start a server of charaura-two on this database first")
        return None, lines, refusals
    chars = db.execute("SELECT serverid, level FROM characters WHERE charName = ?", (NAME,)).fetchall()
    if len(chars) != 1 or chars[0][0] != SERVERID:
        refusals.append(f"characters named {NAME}: {[c[0] for c in chars]}, want exactly serverid {SERVERID}")
        return None, lines, refusals
    held = db.execute("SELECT i.equiploc, p.patternid, p.itemname FROM charinventory i JOIN itempattern p ON p.patternid = i.patternid "
                      "WHERE i.serverid = ? AND i.equiploc = 14", (SERVERID,)).fetchall() \
        if table(db, "charinventory") and table(db, "itempattern") else []
    what = ", ".join(f"{n} ({p})" for _, p, n in held) or "nothing"
    before = row(db, SERVERID, SLOT)
    if before is None:
        refusals.append(f"{NAME} has no charaura slot {SLOT} row (his red aura): nothing to stack on")
        return None, lines, refusals
    lines.append(f"{NAME}: serverid {SERVERID}, level {chars[0][1]}, main hand {what}")
    lines.append(f"  first aura (kept): colour 0x{before['color'] & 0xFFFFFFFF:08X} kind {before['kind']} "
                 f"source 0x{before['source'] & 0xFFFFFFFF:08X} nodes {before['nodes']}")
    if all(before[c] == WANT[c] for c in SECOND):
        lines.append("  second aura: already the twinkle")
        return None, lines, refusals
    lines.append(f"  second aura: colour 0x{before['color2'] & 0xFFFFFFFF:08X} kind {before['kind2']} "
                 f"source 0x{before['source2'] & 0xFFFFFFFF:08X} along {before['nodes2']} -> "
                 f"colour 0 kind {KIND2} (particles) source 0x{SOURCE2:08X} along {NODES2}")
    return before, lines, refusals


def apply(db, before):
    db.execute(LOG_DDL)
    db.execute("UPDATE charaura SET color2 = ?, kind2 = ?, source2 = ?, nodes2 = ? WHERE charid = ? AND merc = 0 AND slot = ?",
               (COLOR2, KIND2, SOURCE2, NODES2, SERVERID, SLOT))
    after = row(db, SERVERID, SLOT)
    a = audit(db, "update", SERVERID, SLOT, before, after)
    db.execute(f"INSERT INTO {LOG} (charid, slot, before, after, audit) VALUES (?, ?, ?, ?, ?)",
               (SERVERID, SLOT, go_json(before), go_json(after), a))


def undo(db):
    """(back, left: [why])."""
    if not table(db, LOG):
        return 0, []
    back, left = 0, []
    for charid, slot, before, after in db.execute(f"SELECT charid, slot, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        now = row(db, charid, slot)
        if go_json(now) != after:
            left.append(f"charaura {charid} slot {slot}: changed since, left as it is")
            continue
        b = json.loads(before)
        db.execute("UPDATE charaura SET color2 = ?, kind2 = ?, source2 = ?, nodes2 = ? WHERE charid = ? AND merc = 0 AND slot = ?",
                   (b["color2"], b["kind2"], b["source2"], b["nodes2"], charid, slot))
        audit(db, "update", charid, slot, now, row(db, charid, slot))
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
            print(f"undone: {back} rows back" + "".join(f"\n  left: {w}" for w in left) +
                  ("; press the GM panel's Reload every NPC" if back else ""))
            return 0 if not left else 1
        before, lines, refusals = plan(db)
        for line in lines:
            print(line)
        if refusals:
            db.execute("ROLLBACK")
            print("REFUSED, nothing written:\n  " + "\n  ".join(refusals))
            return 1
        if before is None:
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if table(db, LOG):
            db.execute("ROLLBACK")
            print(f"applied before and changed since ({LOG} is there): --undo first")
            return 1
        if not a.apply:
            db.execute("ROLLBACK")
            print("(report only: 1 row; --apply to write, then the GM panel's Reload every NPC)")
            return 0
        apply(db, before)
        db.execute("COMMIT")
        print(f"applied: 1 row, in gm_audit and {LOG}; press the GM panel's Reload every NPC (or restart)")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
