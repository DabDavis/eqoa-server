#!/usr/bin/env python3
"""Race-named NPCs in their race's body (the owner, 2026-11-16: "troll mercenary has the wrong model, should be a
troll").

"Troll Mercenary" (npcs row 3218, Tunaria near the Nasehir camps, level 25) is not a hired mercenary: it is a row of
the FrontiersForge import (npcs_ff_import 4898, promoted 3218) whose body was GUESSED from the last word of its name
(model_source "head:mercenary": the "mercenary" NPCs' human man, 0x70D898C6), with no race (NULL, so race 0 on the
wire). Hired mercenaries of race 5 already wear the troll's body (entity.mercModelLocked: charactermodel's row,
0x4C6FA532, as every hire in the log shows); their code and their rows are right and are not touched.

The same import guessed two more playable-race NPCs a body from the head noun "miner" (the goblin miners' body,
0x96371804): "Dwarven miner" (row 3224) and "dark elf miner" (row 3250). Every other NPC named for a playable race
(human, elf, dark elf, gnome, dwarf, troll, barbarian, halfling, erudite, ogre) already wears that race's body. Each of
the three gets its race's man's body (charactermodel sex 0; the names say no sex) and that race in npcs.race (text, as
the race's other NPCs have it: the state channel's byte 144). Their look columns (face, hair, armour) are left.

Each row is found by id, name, world and place, with the guessed body; a row not as found is REFUSED and listed, the
others still written; a row already in its race's body is "already". A report by default; --apply writes in one
transaction with a gm_audit line per row (as the panel's update), logged in race_named_bodies_1116_log; --undo puts
back each row still as written (a row changed since is left and listed, the log kept).

Takes effect without a restart: the GM panel's "Reload every NPC" (reload_npcs) remakes the changed rows in the running
world, else the next start reads them.

    python3 -E -s tools/race_named_bodies_20261116.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "race_named_bodies_1116_log"
DETAIL = ("the owner 2026-11-16, \"troll mercenary has the wrong model, should be a troll\": a race-named NPC's guessed "
          "body (FrontiersForge import, head noun) made its race's; tools/race_named_bodies_20261116.py")
WHO, VIA = "dab", "tool"
NEAR = 5.0
HUMAN_MAN, GOBLIN_MINER = 0x70D898C6, 0x96371804
TROLL_MAN, DWARF_MAN, DARK_ELF_MAN = 0x4C6FA532, 0xAA5B0447, 0x7ED89DAA

# (row id, name, world, x, z, guessed body, race's body, race)
ROWS = [
    (3218, "Troll Mercenary", 0, 24815.8, 17418.5, HUMAN_MAN, TROLL_MAN, 5),
    (3224, "Dwarven miner", 0, 9771.38, 18786.6, GOBLIN_MINER, DWARF_MAN, 4),
    (3250, "dark elf miner", 0, 14753.6, 30403.2, GOBLIN_MINER, DARK_ELF_MAN, 2),
]

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    npcid   INTEGER NOT NULL,
    before  TEXT NOT NULL,    -- {{"modelid", "race"}} before (JSON)
    after   TEXT NOT NULL,    -- as written (JSON)
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def compact(v):
    return json.dumps(v, separators=(",", ":"), sort_keys=True)


def look(db, npcid):
    r = db.execute("SELECT modelid, race FROM npcs WHERE id = ?", (npcid,)).fetchone()
    return None if r is None else {"modelid": r[0], "race": r[1]}


def audit(db, npcid, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', 'update', 'npcs', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, compact({"id": npcid}), compact(before), compact(after), DETAIL))
    return cur.lastrowid


def plan(db):
    """(writes [(npcid, name, before, after)], lines, refusals)."""
    for t in ("npcs", "gm_audit"):
        if not table(db, t):
            return [], [], [f"no {t} table here"]
    writes, lines, refusals = [], [], []
    for npcid, name, world, x, z, guessed, body, race in ROWS:
        r = db.execute("SELECT npc_name, world, x_coord, z_coord, modelid, race FROM npcs WHERE id = ?", (npcid,)).fetchone()
        if r is None:
            refusals.append(f"row {npcid} {name!r}: not here")
            continue
        if r[0] != name or r[1] != world or abs(r[2] - x) > NEAR or abs(r[3] - z) > NEAR:
            refusals.append(f"row {npcid}: {r[0]!r} world {r[1]} at ({r[2]}, {r[3]}), want {name!r} world {world} at ({x}, {z})")
            continue
        model = r[4] & 0xFFFFFFFF
        want = {"modelid": body, "race": str(race)}
        if model == body and r[5] == str(race):
            lines.append(f"{npcid} {name}: already {body:#010x}, race {race}")
            continue
        if model != guessed:
            refusals.append(f"row {npcid} {name!r}: body {model:#010x}, neither the guessed {guessed:#010x} nor its "
                            f"race's {body:#010x}: left (a GM's?)")
            continue
        before = {"modelid": r[4], "race": r[5]}
        lines.append(f"{npcid} {name}: modelid {model:#010x} -> {body:#010x}, race {r[5]!r} -> {race}")
        writes.append((npcid, name, before, want))
    return writes, lines, refusals


def undo(db):
    """(back, left [why])."""
    if not table(db, LOG):
        return 0, []
    back, left = 0, []
    for npcid, before, after in db.execute(f"SELECT npcid, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        now = look(db, npcid)
        if now is None or compact(now) != after:
            left.append(f"row {npcid}: changed since ({now}), left as it is")
            continue
        b = json.loads(before)
        db.execute("UPDATE npcs SET modelid = ?, race = ? WHERE id = ?", (b["modelid"], b["race"], npcid))
        audit(db, npcid, now, b)
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
        for line in lines:
            print(line)
        for r in refusals:
            print("REFUSED: " + r)
        if not writes:
            db.execute("ROLLBACK")
            print("nothing to write" + (" (refusals above)" if refusals else ""))
            return 1 if refusals else 0
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"(report only: --apply writes {len(writes)} rows; then the GM panel's Reload every NPC)")
            return 0
        db.execute(LOG_SCHEMA)
        for npcid, _, before, after in writes:
            db.execute("UPDATE npcs SET modelid = ?, race = ? WHERE id = ?", (after["modelid"], after["race"], npcid))
            aid = audit(db, npcid, before, after)
            db.execute(f"INSERT INTO {LOG} (npcid, before, after, audit) VALUES (?, ?, ?, ?)",
                       (npcid, compact(before), compact(look(db, npcid)), aid))
        db.execute("COMMIT")
        print(f"applied: {len(writes)} rows, in gm_audit and {LOG}; press the GM panel's Reload every NPC")
        return 0 if not refusals else 1
    except Exception:
        db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
