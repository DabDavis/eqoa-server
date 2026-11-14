#!/usr/bin/env python3
"""Zerkziee's druid staff glow lower down his weapons' blades (the owner, 2026-11-14: "lets move the druid epic aura
down zerkziee scimitar down the blade more").

His hands' charaura rows (slots 9 and 10, kind 7, source 0xCBF4135A, the Oak Staff of Eilerawdl: written by
tools/zerkziee_glow_20261114.py) get nodes 45: on a hand that byte is where along the held weapon the borrowed sparkles
sit, the share in percent of its length from the hand (database.AuraAlongMost; vanilla's aura::along), mid-blade on
his Triad-look scimitar. 0 was the staff's own share, 78%, near the tip. --along N for another share (1-100). The
named look "druid staff glow" carries 45 too, so /aura druid staff glow gives the same.

Checked as found, else REFUSED and nothing written: exactly one character named Zerkziee; both his hand rows there and
of the druid staff glow (kind 7, source 0xCBF4135A). Report by default; --apply in one transaction, a gm_audit line a
row, logged in zerkziee_glow_lower_1114_log; a second --apply finds nothing to do; --undo puts back exactly what was
there, a row changed since left and listed. Run it on each shard.

Needs a server that knows a hand's along (aura-blade-lower: an older one drops a hand row of nodes past 32 on read, and
45 is past it) and a client that reads it (vanilla aura-blade-lower; an older one puts the sparkles where it did).
After --apply: the panel's "Reload every NPC".

    python3 -E -s tools/zerkziee_glow_lower_20261114.py --db eqoa.db [--along N] [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "zerkziee_glow_lower_1114_log"
DETAIL = "Zerkziee's druid staff glow lower down the blade (the owner, 2026-11-14); tools/zerkziee_glow_lower_20261114.py"
WHO, VIA = "dab", "tool"
NAME = "Zerkziee"
STAFF = 0xCBF4135A
SLOTS = (9, 10)
ALONG = 45

LOG_DDL = f"""CREATE TABLE {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    charid INTEGER NOT NULL,
    slot   INTEGER NOT NULL,
    before TEXT NOT NULL,
    after  TEXT NOT NULL,
    audit  INTEGER NOT NULL
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def row(db, charid, slot):
    cur = db.execute("SELECT * FROM charaura WHERE charid = ? AND merc = 0 AND slot = ?", (charid, slot))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return None if m is None else json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, charid, slot, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', 'update', 'charaura', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, go_json({"charid": charid, "merc": 0, "slot": slot}),
                      go_json(before), go_json(after), DETAIL))
    return cur.lastrowid


def plan(db, along):
    """(charid, writes: [slot], lines, refusals)."""
    for t in ("characters", "charaura", "gm_audit"):
        if not table(db, t):
            return 0, [], [], [f"no {t} table here"]
    chars = db.execute("SELECT serverid FROM characters WHERE charName = ?", (NAME,)).fetchall()
    if len(chars) != 1:
        return 0, [], [], [f"{len(chars)} characters named {NAME}, want 1"]
    charid = chars[0][0]
    lines, writes, refusals = [f"{NAME}: serverid {charid}"], [], []
    for slot in SLOTS:
        r = row(db, charid, slot)
        hand = "main hand" if slot == 9 else "off hand"
        if r is None or r["kind"] != 7 or (r["source"] & 0xFFFFFFFF) != STAFF:
            refusals.append(f"{hand} (slot {slot}): not the druid staff glow ({r}): run tools/zerkziee_glow_20261114.py first")
            continue
        if r["nodes"] == along:
            lines.append(f"  {hand}: already {along}% along")
            continue
        lines.append(f"  {hand}: nodes {r['nodes']} -> {along} ({along}% along the weapon; 0 was the staff's own 78%)")
        writes.append(slot)
    return charid, writes, lines, refusals


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--along", type=int, default=ALONG)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    if not 1 <= a.along <= 100:
        print("--along is 1-100")
        return 2
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = 0, []
            if table(db, LOG):
                for charid, slot, before, after in db.execute(f"SELECT charid, slot, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
                    now = row(db, charid, slot)
                    if go_json(now) != after:
                        left.append(f"charaura {charid} slot {slot}: changed since, left as it is")
                        continue
                    b = json.loads(before)
                    db.execute("UPDATE charaura SET nodes = ? WHERE charid = ? AND merc = 0 AND slot = ?", (b["nodes"], charid, slot))
                    audit(db, charid, slot, now, row(db, charid, slot))
                    back += 1
                if not left:
                    db.execute(f"DROP TABLE {LOG}")
            db.execute("COMMIT")
            print(f"undone: {back} rows back" + "".join(f"\n  left: {w}" for w in left) +
                  ("; press the GM panel's Reload every NPC" if back else ""))
            return 0 if not left else 1
        charid, writes, lines, refusals = plan(db, a.along)
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
            print(f"(report only: {len(writes)} rows; --apply to write)")
            return 0
        db.execute(LOG_DDL)
        for slot in writes:
            before = row(db, charid, slot)
            db.execute("UPDATE charaura SET nodes = ? WHERE charid = ? AND merc = 0 AND slot = ?", (a.along, charid, slot))
            after = row(db, charid, slot)
            n = audit(db, charid, slot, before, after)
            db.execute(f"INSERT INTO {LOG} (charid, slot, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                       (charid, slot, go_json(before), go_json(after), n))
        db.execute("COMMIT")
        print(f"applied: {len(writes)} rows, in gm_audit and {LOG}; press the GM panel's Reload every NPC")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
