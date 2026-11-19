#!/usr/bin/env python3
"""a player's weapons with two auras each, and Guenhwyvar with a dark blue aura (the owner, 2026-11-16: "also give icing
death the sword of virtue aura blue and also the freezing so we can have 2 auras now. for twinkle sword of virtue aura
with monk staff twinkle at end." and "give [a player]'s cat guen an aura like moogie but darker blue").

1. Icingdeath (found by name): its itemaura row's first aura the Sword of Virtue's look (kind 7 epic, source 0x88EC092D,
   the paladin's level 60 epic: its glow, its sheen and its sparkles, the named look "sword of virtue") in blue
   0x3060FFFF; its second aura (color2 ... nodes2) the freezing it had (kind 3 particles of the spell visual 0x6C151A60,
   the ice dripping at the tip) in the same blue.
2. Twinkle (found by name): its first aura the Sword of Virtue's look in its own bluish 0x6EA8FFFF (it was the druid
   staff glow: replaced, kept in the log); its second the monk staff's twinkle, the Ascendant staff 0x1D875ECD's
   particle sprite (the star flare at its yin-yang end) at the blade's end (kind 3, nodes2 100: all the way along).
3. Guenhwyvar (npctint, by the pet's name, which handlers/guenhwyvar.go's stone summons from petstats 760113): Mr.
   Moogie's aura (tint and glow, aura kind 0: the shell while the glow lights) in a darker blue: tint 0x6070FFFF,
   glow 0x0020C0FF (Moogie's: 0xFF6060FF, 0xFF0000FF).

Needs a server with the second auras (branch two-auras: itemaura's color2, kind2, source2, nodes2, which it adds at its
start), else REFUSED: an older server would show the first auras alone. Checked as found, else REFUSED and nothing
written: one Twinkle and one Icingdeath; tables itempattern, itemaura (with the second columns), npctint, gm_audit.
Report by default; --apply in one transaction, a gm_audit line a row (as the panel's), logged in
two_auras_1116_log; a second --apply finds nothing to do; --undo puts back exactly what was there (a row changed since
left and listed, the rest still put back).

After --apply: the GM panel's "Reload every NPC" (or an Item auras edit) shows the weapons' auras at the next tick and
sends Guenhwyvar's tint to every client that draws tints. A vanilla before two-auras draws each weapon's first aura
alone; a PS2 neither.

    python3 -E -s tools/two_auras_drizzt_20261116.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "two_auras_1116_log"
DETAIL = "Two auras on Drizzt's Twinkle and Icingdeath (the Sword of Virtue's look + the monk staff's twinkle / the freezing), Guenhwyvar Mr. Moogie's aura in a darker blue (the owner, 2026-11-16); tools/two_auras_drizzt_20261116.py"
WHO, VIA = "dab", "tool"

SWORD_OF_VIRTUE = 0x88EC092D  # the paladin's level 60 epic, ITEM.ESF
ASCENDANT_STAFF = 0x1D875ECD  # the monk's level 49 epic: its particle sprite, a star flare at its yin-yang end
FREEZING = 0x6C151A60  # the spell visual: ice dripping off a caster's hands
BLUE, TWINKLE_BLUE = 0x3060FFFF, 0x6EA8FFFF

# name -> its itemaura row (the first aura, then the second)
WANT = {
    "Icingdeath": {"color": BLUE, "kind": 7, "source": SWORD_OF_VIRTUE, "nodes": 0,
                   "color2": BLUE, "kind2": 3, "source2": FREEZING, "nodes2": 0,
                   "note": "Icingdeath: the Sword of Virtue's look in blue, and the freezing (6C151A60's ice drip at the tip) in blue, the owner 2026-11-16"},
    "Twinkle": {"color": TWINKLE_BLUE, "kind": 7, "source": SWORD_OF_VIRTUE, "nodes": 0,
                "color2": 0, "kind2": 3, "source2": ASCENDANT_STAFF, "nodes2": 100,
                "note": "Twinkle: the Sword of Virtue's look in its bluish, and the monk staff's twinkle (1D875ECD's star flare) at the blade's end, the owner 2026-11-16"},
}

PET = "Guenhwyvar"
PET_SPELL = 760113
PET_TINT = {"color": 0x6070FFFF, "glow": 0x0020C0FF, "aura_kind": 0, "aura_source": 0, "aura_nodes": 0,
            "note": "Drizzt's cat: Mr. Moogie's aura (his glow's shell) in a darker blue, the owner 2026-11-16", "source": "twoauras1116"}

LOG_DDL = f"""CREATE TABLE {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    rowkey TEXT NOT NULL,      -- the key (JSON)
    before TEXT,               -- the row before (JSON), NULL none
    after  TEXT NOT NULL,      -- the row written (JSON)
    audit  INTEGER NOT NULL
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def columns(db, name):
    return {r[1] for r in db.execute(f"PRAGMA table_info({name})")}


def row(db, tbl, key):
    where = " AND ".join(f"{k} = ?" for k in key)
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", tuple(key.values()))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return None if m is None else json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, tbl, key, action, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key), go_json(before), go_json(after), DETAIL))
    return cur.lastrowid


def put(db, tbl, key, want):
    """tbl's row of key made want (a dict of the columns to set; None: deleted); (before, after, audit id)."""
    before = row(db, tbl, key)
    where = " AND ".join(f"{k} = ?" for k in key)
    if want is None:
        db.execute(f"DELETE FROM {tbl} WHERE {where}", tuple(key.values()))
        action = "delete"
    elif before is None:
        cols = dict(key, **want)
        db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", tuple(cols.values()))
        action = "insert"
    else:
        sets = {k: v for k, v in want.items() if k not in key}
        db.execute(f"UPDATE {tbl} SET {', '.join(f'{k} = ?' for k in sets)} WHERE {where}",
                   tuple(sets.values()) + tuple(key.values()))
        action = "update"
    after = row(db, tbl, key)
    return before, after, audit(db, tbl, key, action, before, after)


def hexes(r, cols):
    return " ".join(f"{c} {'0x%08X' % (r[c] & 0xFFFFFFFF) if c.startswith(('color', 'source', 'glow', 'aura_source')) else r[c]}" for c in cols)


def plan(db):
    """(writes [(tbl, key, want)], lines, refusals)."""
    writes, lines, refusals = [], [], []
    for t in ("itempattern", "itemaura", "npctint", "gm_audit"):
        if not table(db, t):
            refusals.append(f"no {t} table here")
    if refusals:
        return writes, lines, refusals
    missing = {"color2", "kind2", "source2", "nodes2"} - columns(db, "itemaura")
    if missing:
        refusals.append(f"itemaura has no {', '.join(sorted(missing))}: deploy the server with the second auras (two-auras) first; it adds them at its start")
        return writes, lines, refusals
    for name, aura in WANT.items():
        found = db.execute("SELECT patternid, equipslot FROM itempattern WHERE itemname = ?", (name,)).fetchall()
        if len(found) != 1:
            refusals.append(f"{len(found)} itempattern rows named {name!r}, not one")
            continue
        pid, slot = found[0]
        if slot not in (12, 14):
            refusals.append(f"itempattern {pid} {name!r} is wear slot {slot}: not a held weapon")
            continue
        cur = row(db, "itemaura", {"patternid": pid})
        cols = [c for c in aura if c != "note"]
        if cur is None or any((cur[c] & 0xFFFFFFFF if isinstance(cur[c], int) else cur[c]) != aura[c] for c in cols):
            writes.append(("itemaura", {"patternid": pid}, dict(aura)))
            had = "none" if cur is None else hexes(cur, cols)
            lines.append(f"  itemaura {pid} {name}:\n    was {had}\n    now {hexes(aura, cols)}")
        held = db.execute("SELECT c.charName, i.equiploc FROM charinventory i JOIN characters c ON c.serverid = i.serverid "
                          "WHERE i.patternid = ?", (pid,)).fetchall() if table(db, "charinventory") and table(db, "characters") else []
        lines.append(f"  {name} ({pid}) held by " + (", ".join(f"{n} (equiploc {e})" for n, e in held) or "no one"))
    if table(db, "petstats"):
        pet = db.execute("SELECT name FROM petstats WHERE spellid = ?", (PET_SPELL,)).fetchone()
        if pet is None or pet[0].strip().lower() != PET.lower():
            refusals.append(f"petstats {PET_SPELL} is {pet!r}, not {PET}: the stone's pet is named otherwise here")
    names = [r[0] for r in db.execute("SELECT name FROM npctint") if r[0].strip().lower().rstrip("$ ").strip() == PET.lower()]
    if len(names) > 1:
        refusals.append(f"{len(names)} npctint rows for {PET}: {names}")
    key = {"name": names[0] if names else PET}
    cur = row(db, "npctint", key)
    cols = ["color", "glow", "aura_kind", "aura_source", "aura_nodes"]
    if cur is None or any((cur[c] & 0xFFFFFFFF) != PET_TINT[c] for c in cols):
        writes.append(("npctint", key, dict(PET_TINT)))
        had = "none" if cur is None else hexes(cur, cols)
        lines.append(f"  npctint {key['name']}:\n    was {had}\n    now {hexes(PET_TINT, cols)}")
    return writes, lines, refusals


def apply(db, writes):
    db.execute(LOG_DDL)
    for tbl, key, want in writes:
        before, after, n = put(db, tbl, key, want)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                   (tbl, go_json(key), go_json(before), go_json(after), n))
    return len(writes)


def undo(db):
    back, left = 0, []
    if not table(db, LOG):
        return back, left
    for seq, tbl, rowkey, before, after in db.execute(f"SELECT seq, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key = json.loads(rowkey)
        if go_json(row(db, tbl, key)) != after:
            left.append(f"{tbl} {rowkey}: changed since, left as it is")
            continue
        b = json.loads(before) if before else None
        put(db, tbl, key, None if b is None else {k: v for k, v in b.items() if k not in key})
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
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
        writes, lines, refusals = plan(db)
        if refusals:
            db.execute("ROLLBACK")
            print("REFUSED, nothing written:" + "".join(f"\n  {r}" for r in refusals))
            return 1
        print("\n".join(lines))
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
        n = apply(db, writes)
        db.execute("COMMIT")
        print(f"applied: {n} rows, in gm_audit and {LOG}; press the GM panel's Reload every NPC (the weapons' auras at "
              "the next tick, Guenhwyvar's tint sent to every client that draws tints)")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
