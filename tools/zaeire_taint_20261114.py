#!/usr/bin/env python3
"""Zaeire's held weapons the necromancer's tainted aura of the epic sickle (the owner, 2026-11-14: "give zaeires
equipped weapons the necro's tainted aura from the epic sickle").

The aura is the hands' charaura slots (entity/pieceaura.go: 9 the main hand, 10 the off hand), kind 7 (database.AuraEpic):
the item held in that hand drawn with the Sickle of Avhi Escron's own look, its hand model 0x5FD4AE7C lending it its
materials' olive emissive glow (60, 90, 0) and its green mist particle sprite; colour 0, the sickle's own colours.
Per hand and per character, whatever the hand holds: on Heirloom Zaeire (serverid 193) holds two Heirloom Daggers of
Bloodletting (714100) in equiploc 14 and 15; on Live (serverid 20) nothing, and the aura shows when it holds something.
A client before vanilla's weapon-taint (it never answers the live look's bit 6) and a PS2 draw nothing of it.

Checked as found, else REFUSED and nothing written: exactly one character named Zaeire; an itempattern row named "Sickle
of Avhi Escron" of model 1607773820 (0x5FD4AE7C); table charaura and gm_audit there. A row on a hand already (a GM's) is
changed and its old values kept in the log. Report by default; --apply in one transaction, a gm_audit line a row (as the
panel's: insert or update, the whole row before and after), logged in zaeire_taint_1114_log; a second --apply finds
nothing to do; --undo puts back exactly what was there (a row this made deleted, a row it changed given its values
back), each a gm_audit line, a row changed since left and listed. Run it on each shard.

The server reads charaura at its start and on the GM panel's "Reload every NPC" (reload_npcs): after --apply (or --undo)
on a server that has weapon-taint, press that; Zaeire's channel carries it at the next tick. A server before it skips
these rows (slot past 8), so applying first is harmless.

    python3 -E -s tools/zaeire_taint_20261114.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "zaeire_taint_1114_log"
DETAIL = "Zaeire's weapons the epic sickle's tainted aura (the owner, 2026-11-14); tools/zaeire_taint_20261114.py"
WHO, VIA = "dab", "tool"
NAME = "Zaeire"
SICKLE, SICKLE_NAME = 0x5FD4AE7C, "Sickle of Avhi Escron"
SLOTS = (9, 10)  # database.CharAuraMainHand, CharAuraOffHand
COLOR, KIND, NODES = 0, 7, 0  # no colour (the sickle's own), database.AuraEpic, no nodes
NOTE = "Zaeire's weapons: the Sickle of Avhi Escron's tainted aura (epic look), the owner 2026-11-14"
COLS = ("charid", "merc", "slot", "color", "kind", "source", "nodes", "note")

LOG_DDL = f"""CREATE TABLE {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    charid  INTEGER NOT NULL,
    slot    INTEGER NOT NULL,
    before  TEXT,              -- the row before (JSON), NULL none
    after   TEXT NOT NULL,     -- the row written (JSON)
    audit   INTEGER NOT NULL
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


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


def wanted(charid, slot):
    """The hand's row as this writes it."""
    return {"charid": charid, "merc": 0, "slot": slot, "color": COLOR, "kind": KIND, "source": SICKLE, "nodes": NODES, "note": NOTE}


def plan(db):
    """(charid, writes: [(slot, before or None)], lines, refusals)."""
    refusals, lines, writes = [], [], []
    for t in ("characters", "itempattern", "charaura", "gm_audit"):
        if not table(db, t):
            refusals.append(f"no {t} table here")
    if refusals:
        return 0, writes, lines, refusals
    chars = db.execute("SELECT serverid, level FROM characters WHERE charName = ?", (NAME,)).fetchall()
    if len(chars) != 1:
        refusals.append(f"{len(chars)} characters named {NAME}, want 1")
        return 0, writes, lines, refusals
    charid, level = chars[0]
    sickles = db.execute("SELECT patternid FROM itempattern WHERE itemname = ? AND model = ?", (SICKLE_NAME, SICKLE)).fetchall()
    if not sickles:
        refusals.append(f"no itempattern {SICKLE_NAME!r} of model {SICKLE} (0x{SICKLE:08X})")
        return charid, writes, lines, refusals
    held = db.execute("SELECT i.equiploc, p.itemname, p.model FROM charinventory i JOIN itempattern p ON p.patternid = i.patternid "
                      "WHERE i.serverid = ? AND i.equiploc IN (14, 15, 16) ORDER BY i.equiploc", (charid,)).fetchall() \
        if table(db, "charinventory") else []
    what = ", ".join(f"equiploc {e} {n} (0x{m & 0xFFFFFFFF:08X})" for e, n, m in held) or "nothing in hand"
    lines.append(f"{NAME}: serverid {charid}, level {level}, holds {what}; the sickle is itempattern "
                 f"{', '.join(str(s[0]) for s in sickles)}")
    for slot in SLOTS:
        hand = "main hand" if slot == 9 else "off hand"
        before = row(db, charid, slot)
        if before == wanted(charid, slot):
            lines.append(f"  {hand} (charaura slot {slot}): already the sickle's taint")
            continue
        was = "none" if before is None else \
            f"0x{before['color'] & 0xFFFFFFFF:08X} kind {before['kind']} source 0x{before['source'] & 0xFFFFFFFF:08X} (kept in the log)"
        lines.append(f"  {hand} (charaura slot {slot}): {was} -> kind {KIND} (epic) source 0x{SICKLE:08X}, its own colours")
        writes.append((slot, before))
    return charid, writes, lines, refusals


def apply(db, charid, writes):
    db.execute(LOG_DDL)
    for slot, before in writes:
        w = wanted(charid, slot)
        if before is None:
            db.execute(f"INSERT INTO charaura ({', '.join(COLS)}) VALUES ({', '.join('?' for _ in COLS)})", tuple(w[c] for c in COLS))
            action = "insert"
        else:
            db.execute("UPDATE charaura SET color = ?, kind = ?, source = ?, nodes = ?, note = ? WHERE charid = ? AND merc = 0 AND slot = ?",
                       (COLOR, KIND, SICKLE, NODES, NOTE, charid, slot))
            action = "update"
        after = row(db, charid, slot)
        a = audit(db, action, charid, slot, before, after)
        db.execute(f"INSERT INTO {LOG} (charid, slot, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                   (charid, slot, go_json(before), go_json(after), a))
    return len(writes)


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
        before = json.loads(before) if before else None
        if before is None:
            db.execute("DELETE FROM charaura WHERE charid = ? AND merc = 0 AND slot = ?", (charid, slot))
            audit(db, "delete", charid, slot, now, None)
        else:
            db.execute("UPDATE charaura SET color = ?, kind = ?, source = ?, nodes = ?, note = ? WHERE charid = ? AND merc = 0 AND slot = ?",
                       (before["color"], before["kind"], before["source"], before["nodes"], before["note"], charid, slot))
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
        charid, writes, lines, refusals = plan(db)
        for line in lines:
            print(line)
        if refusals:
            db.execute("ROLLBACK")
            print("REFUSED, nothing written:\n  " + "\n  ".join(refusals))
            return 1
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
            print(f"(report only: {len(writes)} rows; --apply to write, then the GM panel's Reload every NPC)")
            return 0
        n = apply(db, charid, writes)
        db.execute("COMMIT")
        print(f"applied: {n} rows, in gm_audit and {LOG}; press the GM panel's Reload every NPC (or restart)")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
