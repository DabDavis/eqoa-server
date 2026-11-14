#!/usr/bin/env python3
"""Dust's monk circlet raised to the temples, a little bigger and red (the owner, 2026-11-14: "the circlet is sitting
too low on dust", "make the circlet slightly bigger too", "also make the circlet red").

His helm slot's charaura row (slot 5, kind 8, source 0x1D875ECD, the Ascendant staff: written by
tools/dust_circlet_20261114.py) gets nodes 72 and colour 0xE01818FF. On a circlet the nodes byte is how high on the
head it sits, the share in percent of the head from the chin (database.ValidCharAuraNodes; vanilla's aura::circlet_of):
72 is the upper forehead and temples, above the brows; 0 was the client's 62%, across the eyes. The colour recolours
the ring's glow red at its own brightness (aura::glow8, as a held item's colour does). The size is the client's
(aura::CIRCLET_OUT 1.5, +15%: no byte of the record is free for it). The named look "monk circlet" carries 72.
--height N (1-100) and --color RRGGBBAA for others.

Checked as found, else REFUSED and nothing written: at most one character named Dust (none: nothing, exit 0); his
row there and the monk circlet (kind 8, source 0x1D875ECD). Report by default; --apply in one transaction, a gm_audit
line, logged in dust_circlet_red_1114_log; a second --apply finds nothing to do; --undo puts back exactly what was there.

Needs a server with circlet-height (an older one drops a circlet of nodes past 32 on read: his circlet would vanish
until it is deployed) and a client with it (an older one ignores the height, and draws the red at its 1.3 size).
After --apply: the panel's "Reload every NPC".

    python3 -E -s tools/dust_circlet_red_20261114.py --db eqoa.db [--height N] [--color RRGGBBAA] [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "dust_circlet_red_1114_log"
DETAIL = "Dust's monk circlet at the temples, bigger and red (the owner, 2026-11-14); tools/dust_circlet_red_20261114.py"
WHO, VIA = "dab", "tool"
NAME = "Dust"
STAFF = 0x1D875ECD
SLOTS = (5,)
ALONG = 72
COLOR = 0xE01818FF

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


def plan(db, along, color):
    """(charid, writes: [slot], lines, refusals)."""
    for t in ("characters", "charaura", "gm_audit"):
        if not table(db, t):
            return 0, [], [], [f"no {t} table here"]
    chars = db.execute("SELECT serverid FROM characters WHERE charName = ?", (NAME,)).fetchall()
    if not chars:
        return 0, [], [f"no {NAME} on this shard: nothing for him here"], []
    if len(chars) != 1:
        return 0, [], [], [f"{len(chars)} characters named {NAME}, want 1"]
    charid = chars[0][0]
    lines, writes, refusals = [f"{NAME}: serverid {charid}"], [], []
    for slot in SLOTS:
        r = row(db, charid, slot)
        if r is None or r["kind"] != 8 or (r["source"] & 0xFFFFFFFF) != STAFF:
            refusals.append(f"helm (slot {slot}): not the monk circlet ({r}): run tools/dust_circlet_20261114.py first")
            continue
        if r["nodes"] == along and (r["color"] & 0xFFFFFFFF) == color:
            lines.append(f"  helm: already {along}% up, 0x{color:08X}")
            continue
        lines.append(f"  helm: nodes {r['nodes']} -> {along} ({along}% up the head; 0 was the client's 62%, across the eyes), "
                     f"colour 0x{r['color'] & 0xFFFFFFFF:08X} -> 0x{color:08X}")
        writes.append(slot)
    return charid, writes, lines, refusals


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--height", type=int, default=ALONG)
    ap.add_argument("--color", default=f"{COLOR:08X}")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    if not 1 <= a.height <= 100:
        print("--height is 1-100")
        return 2
    color = int(a.color.removeprefix("0x"), 16) & 0xFFFFFFFF
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
                    db.execute("UPDATE charaura SET nodes = ?, color = ? WHERE charid = ? AND merc = 0 AND slot = ?", (b["nodes"], b["color"], charid, slot))
                    audit(db, charid, slot, now, row(db, charid, slot))
                    back += 1
                if not left:
                    db.execute(f"DROP TABLE {LOG}")
            db.execute("COMMIT")
            print(f"undone: {back} rows back" + "".join(f"\n  left: {w}" for w in left) +
                  ("; press the GM panel's Reload every NPC" if back else ""))
            return 0 if not left else 1
        charid, writes, lines, refusals = plan(db, a.height, color)
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
            db.execute("UPDATE charaura SET nodes = ?, color = ? WHERE charid = ? AND merc = 0 AND slot = ?", (a.height, color, charid, slot))
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
