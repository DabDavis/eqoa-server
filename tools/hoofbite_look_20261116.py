#!/usr/bin/env python3
"""A player's look (the owner, 2026-11-16): "for [a player] give his weapons sword of virtue aura matching his weapon
color. for his monk hat lets use the kappas built in hat that is wide brimmed for his model", then, of the weapons:
"for weapons with aura they can be marked for an aura though they are the same weapon as other players, it will have a
tag 'Aura: Virtuous' for example. so it doesnt just stick to all his weps".

A player is Heirloom's serverid 213 (a halfling rogue, level 60). Two steps, each checked and written on its own:

1. THE HAT: his own helm aura, charaura (213, merc 0, slot 5 database.CharAuraHelm): kind 10 (database.AuraHat, server
   branch hoofbite-look), source 0xF0FAB933 (the kappa's body, whose wide-brimmed straw hat vanilla cuts out and seats on
   his head in place of the helm: vanilla's hat.rs), colour 0 (the straw's own colours), nodes 0. Shown whatever he
   wears on his head (the Crown of the Jaguar, which has no look of its own). A server before hoofbite-look drops a kind
   10 row on read (nothing shown, nothing broken): apply after that server is live. A PS2, and a vanilla before the hat,
   shows his head as before.

2. THE WEAPONS: an aura on each of his two swords' COPIES (instanceaura, by charinventory itemid; server branch
   instance-aura), tagged "Virtuous" (shown "Aura: Virtuous" on the copy), the Sword of Virtue's look (kind 7 epic,
   source 0x88EC092D) in each sword's colour as the world draws it:
     - itemid 14767 Sword of Power (patternid 960260, main hand, equiploc 14): its record's colour is 0x00000008 (a disc
       tint index, which no client draws on an item: entity.ArmorColor), so the hand shows his main-hand dye,
       chardyes slot 6 = 0x00008CFF (navy blue): the aura 0x00008CFF.
     - itemid 14768 Sword of Protection (patternid 960261, off hand, equiploc 15): its record's colour 0x4B0082FF
       (indigo) wins over his off-hand dye (0x0000C0FF): the aura 0x4B0082FF.
   No other character holds either record. Nothing goes in charaura's hand slots or itemaura. When table instanceaura
   is not there (the instance-aura server has not started on this database yet), this step is SKIPPED and said so; the
   hat step goes on.

Checked as found, else that step is REFUSED and nothing of it written: exactly one character named a player, serverid
213; for the hat, his charaura slot 5 empty (a row there is a GM's: refused, listed); for the weapons, each itemid his
(serverid 213) and of its record, and no instanceaura row on it (a row there: refused, listed). Report by default;
--apply writes both in one transaction, a gm_audit line per row (as the panel's insert), logged in
hoofbite_look_1116_log; a second --apply writes only what is not there yet (the weapons, once instanceaura is), else
finds nothing to do; --undo deletes exactly what was written, a gm_audit line
each, a row changed since left and listed.

The server reads charaura and instanceaura at its start and on the GM panel's "Reload every NPC" (reload_npcs): after
--apply (or --undo) press that; a player's channel carries both at the next tick.

    python3 -E -s tools/hoofbite_look_20261116.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "hoofbite_look_1116_log"
DETAIL = "Hoofbite's look: the kappa's straw hat, the Sword of Virtue's aura on his swords (the owner, 2026-11-16); tools/hoofbite_look_20261116.py"
WHO, VIA = "dab", "tool"
NAME, SERVERID = "Hoofbite", 213

HAT_SLOT, HAT_KIND, KAPPA = 5, 10, 0xF0FAB933  # database.CharAuraHelm, AuraHat, KappaHat
TAG = "Virtuous"
EPIC, SWORD_OF_VIRTUE = 7, 0x88EC092D  # database.AuraEpic, the paladin's level 60 epic
# (itemid, patternid, name, aura colour, why)
SWORDS = [
    (14767, 960260, "Sword of Power", 0x00008CFF, "his main-hand dye (the record's colour 0x00000008 is a tint index: none)"),
    (14768, 960261, "Sword of Protection", 0x4B0082FF, "the record's own colour (wins over his off-hand dye)"),
]

LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,     -- charaura or instanceaura
    rowkey  TEXT NOT NULL,     -- the row's key (JSON)
    after   TEXT NOT NULL,     -- the row written (JSON)
    audit   INTEGER NOT NULL
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def go_json(m):
    return None if m is None else json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def fetch(db, tbl, key):
    where = " AND ".join(f"{k} = ?" for k in key)
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", tuple(key.values()))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def audit(db, action, tbl, key, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key), go_json(before), go_json(after), DETAIL))
    return cur.lastrowid


def plan(db):
    """(writes [(tbl, key, values)], lines, refusals)."""
    writes, lines, refusals = [], [], []
    for t in ("characters", "charaura", "charinventory", "gm_audit"):
        if not table(db, t):
            refusals.append(f"no {t} table here")
    if refusals:
        return writes, lines, refusals
    chars = db.execute("SELECT serverid, level, race, tclass FROM characters WHERE charName = ?", (NAME,)).fetchall()
    if len(chars) != 1 or chars[0][0] != SERVERID:
        refusals.append(f"characters named {NAME}: {[c[0] for c in chars]}, want exactly serverid {SERVERID}")
        return writes, lines, refusals
    lines.append(f"{NAME}: serverid {SERVERID}, level {chars[0][1]}, race {chars[0][2]}, class {chars[0][3]}")

    # 1. the hat
    key = {"charid": SERVERID, "merc": 0, "slot": HAT_SLOT}
    have = fetch(db, "charaura", key)
    want = dict(key, color=0, kind=HAT_KIND, source=KAPPA, nodes=0, note="the kappa's wide-brimmed straw hat (owner 2026-11-16)")
    if have is None:
        lines.append(f"  hat: charaura slot {HAT_SLOT} kind {HAT_KIND} (hat) source 0x{KAPPA:08X}, colour 0 (its own)")
        writes.append(("charaura", key, want))
    elif have["kind"] == HAT_KIND and (have["source"] & 0xFFFFFFFF) == KAPPA:
        lines.append("  hat: already there")
    else:
        refusals.append(f"hat: his charaura slot {HAT_SLOT} has a row already (kind {have['kind']} source "
                        f"0x{have['source'] & 0xFFFFFFFF:08X} colour 0x{have['color'] & 0xFFFFFFFF:08X}): left; ask the owner")

    # 2. the weapons, on their copies
    if not table(db, "instanceaura"):
        lines.append("  weapons: SKIPPED: no instanceaura table here (start the instance-aura server on this database "
                     "first, then run --apply again); the values:")
        for itemid, pid, name, color, why in SWORDS:
            lines.append(f"    itemid {itemid} {name}: tag {TAG!r}, kind {EPIC} source 0x{SWORD_OF_VIRTUE:08X} colour 0x{color:08X} ({why})")
        return writes, lines, refusals
    for itemid, pid, name, color, why in SWORDS:
        inv = db.execute("SELECT serverid, patternid, equiploc FROM charinventory WHERE itemid = ?", (itemid,)).fetchone()
        if inv is None or inv[0] != SERVERID or inv[1] != pid:
            refusals.append(f"weapons: itemid {itemid} is {inv}, want serverid {SERVERID} patternid {pid} ({name})")
            continue
        others = db.execute("SELECT COUNT(*) FROM charinventory WHERE patternid = ? AND serverid <> ?", (pid, SERVERID)).fetchone()[0]
        key = {"itemid": itemid}
        have = fetch(db, "instanceaura", key)
        want = dict(key, tag=TAG, color=color, kind=EPIC, source=SWORD_OF_VIRTUE, nodes=0,
                    color2=0, kind2=0, source2=0, nodes2=0, note=f"Hoofbite's {name}, the Sword of Virtue's aura in its colour (owner 2026-11-16)")
        if "source_tool" in columns(db, "instanceaura"):
            want["source_tool"] = "tools/hoofbite_look_20261116.py"
        label = f"  weapons: itemid {itemid} {name} (equiploc {inv[2]}; {others} other copies of the record): "
        if have is None:
            lines.append(label + f"tag {TAG!r}, Sword of Virtue (kind {EPIC}, 0x{SWORD_OF_VIRTUE:08X}) in 0x{color:08X}, {why}")
            writes.append(("instanceaura", key, want))
        elif all(have.get(k) == v for k, v in want.items() if k not in ("note", "source_tool")):
            lines.append(label + "already there")
        else:
            refusals.append(f"weapons: itemid {itemid} {name} has an instanceaura row already: {go_json(have)}; left")
    return writes, lines, refusals


def columns(db, name):
    return {r[1] for r in db.execute(f"PRAGMA table_info({name})")}


def apply(db, writes):
    db.execute(LOG_DDL)
    for tbl, key, values in writes:
        cols = [c for c in values if c in columns(db, tbl)]
        db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' for _ in cols)})", tuple(values[c] for c in cols))
        after = fetch(db, tbl, key)
        a = audit(db, "insert", tbl, key, None, after)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, after, audit) VALUES (?, ?, ?, ?)", (tbl, go_json(key), go_json(after), a))


def undo(db):
    """(back, left: [why])."""
    if not table(db, LOG):
        return 0, []
    back, left = 0, []
    for tbl, rowkey, after in db.execute(f"SELECT tbl, rowkey, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key = json.loads(rowkey)
        now = fetch(db, tbl, key) if table(db, tbl) else None
        if go_json(now) != after:
            left.append(f"{tbl} {rowkey}: changed since, left as it is")
            continue
        where = " AND ".join(f"{k} = ?" for k in key)
        db.execute(f"DELETE FROM {tbl} WHERE {where}", tuple(key.values()))
        audit(db, "delete", tbl, key, now, None)
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
            print(f"undone: {back} rows deleted" + "".join(f"\n  left: {w}" for w in left) +
                  ("; press the GM panel's Reload every NPC" if back else ""))
            return 0 if not left else 1
        writes, lines, refusals = plan(db)
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
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"(report only: {len(writes)} rows; --apply to write, then the GM panel's Reload every NPC)")
            return 0
        apply(db, writes)
        db.execute("COMMIT")
        print(f"applied: {len(writes)} rows, in gm_audit and {LOG}; press the GM panel's Reload every NPC (or restart)")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
