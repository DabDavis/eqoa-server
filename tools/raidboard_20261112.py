#!/usr/bin/env python3
"""The Instanced Raids board against Freeport's well, and the moth larvae that see invisibility (2026-11-12).

1. The owner: "for the instanced raids add them to a bulletin board similar to PvP Events, against the well". Freeport's
   Dungeon Warden (npcs 166499, world 0, beside Dabrish) becomes the papered bulletin board "Instanced Raids" on the
   well's SOUTH side, (25296.7, 54.13, 15756.3) facing 0 (toward +z, away from the well; the well's other boards: Real
   Estate Listings west, PvP Events east, at the same 6.2 off its centre). The north side was not clear: the Mercenary
   Recruitment board already stands there, 2.0 from the spot. The south's nearest standing NPC is the Testing Heroes
   Vendor, 3.6 off (the GM triggers' Moogie fights spawn Megaladon or Mithaniel Marr 1.9 off it for their hour).
   Its look is the PvP Events board's (modelid 638497839 size 1, its hair/face/robe/race columns); npcmove and
   npcaggro rows for the new name, as the other boards have (it stands; it talks only). The server (IsDungeonWarden)
   opens the same dungeon menu from it and shows its name plate (boardplate.go); the entrance twins in world 14
   (166500-166502) stay Dungeon Wardens, rows untouched, and so do the "dungeon warden" npcmove/npcaggro rows they use.
2. Petition 1814 (Skywalker): "make all moth larva npc in the game see through invisibility". npcaggro row 'moth larva'
   (aggressive -1: no word on aggro, the world's rules decide) with see_invis 1 (the column added when missing, as the
   server's start does): the Plane of Disease's larvae (world 5, 3 rows) notice an invisible player.

A report by default; --apply writes it all in one transaction with a gm_audit line a change; --undo takes back each
change still as this tool left it (the npcs row to its exact before, the inserted rows deleted, the see_invis column
dropped when this tool added it and no row uses it). Refused, nothing written, unless row 166499 is the Dungeon Warden
in world 0 within 1 of (25301.144, 15737.166), nothing is named Instanced Raids yet, the moth larvae are there and
their npcaggro row (if any) says nothing of see_invis. Log raidboard_1112_log.

Needs the server with raidboard's code (IsDungeonWarden takes "Instanced Raids"; npcaggro.see_invis read). A RESTART:
npcs rows are read at the start (the npcaggro rows a GM panel edit of npcaggro would reload, but the board needs it).

    tools/raidboard_20261112.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "raidboard_1112_log"
DETAIL = "Instanced Raids board + moth larvae see invisibility (the owner 2026-11-12; petition 1814): tools/raidboard_20261112.py"
WHO, VIA = "dab", "tool"
WARDEN_ID, WARDEN, BOARD = 166499, "Dungeon Warden", "Instanced Raids"
WAS_X, WAS_Z = 25301.144, 15737.166
SPOT = {"x_coord": 25296.7, "y_coord": 54.13, "z_coord": 15756.3, "facing": 0}
LOOK_FROM = 162976  # PvP Events
LOOK = ["modelid", "size", "hair_color", "hair_length", "hair_style", "face", "robe_style", "race", "unk_i", "unk_n"]
LARVA = "moth larva"
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,      -- npcs, npcmove, npcaggro, or 'made' for the see_invis column this tool added
    rowkey TEXT,
    before TEXT,               -- the row before (JSON), NULL for an insert
    after  TEXT,               -- the row as this tool left it
    audit  INTEGER NOT NULL DEFAULT 0
)"""
KEYS = {"npcs": "id", "npcmove": "name", "npcaggro": "npc_name"}


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def has_col(db, t, c):
    return any(r[1] == c for r in db.execute(f"PRAGMA table_info({t})"))


def row(db, t, key):
    cur = db.execute(f"SELECT * FROM {t} WHERE {KEYS[t]} = ?", (key,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, t, key, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, t, go_json({KEYS[t]: key}),
                      go_json(before) if before is not None else "", go_json(after) if after is not None else "", DETAIL))
    return cur.lastrowid


def log(db, t, key, before, after, a):
    db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
               (t, json.dumps(key), None if before is None else json.dumps(before), None if after is None else json.dumps(after), a))


def board_values(db):
    look = row(db, "npcs", LOOK_FROM)
    v = {"npc_name": BOARD, **SPOT}
    for c in LOOK:
        v[c] = look[c]
    return v


def plan(db):
    """(lines, refusals, applied)."""
    for t in ("npcs", "npcmove", "npcaggro", "gm_audit"):
        if not table(db, t):
            return [], [f"no {t} table here"], False
    refusals, lines = [], []
    w = row(db, "npcs", WARDEN_ID)
    look = row(db, "npcs", LOOK_FROM)
    if look is None or look["npc_name"] != "PvP Events" or look["modelid"] != 638497839:
        refusals.append(f"npcs {LOOK_FROM} is not the PvP Events board: {look and (look['npc_name'], look['modelid'])}")
    see = has_col(db, "npcaggro", "see_invis")
    larva = row(db, "npcaggro", LARVA)
    if w is not None and w["npc_name"] == BOARD and look is not None and \
            all(w[k] == v for k, v in board_values(db).items()) and see and larva and larva["see_invis"] == 1:
        return [], [], True
    if w is None or w["npc_name"] != WARDEN or w["world"] != 0 or \
            abs(w["x_coord"] - WAS_X) > 1 or abs(w["z_coord"] - WAS_Z) > 1:
        refusals.append(f"npcs {WARDEN_ID} is not the Dungeon Warden at Freeport's ({WAS_X}, {WAS_Z}): "
                        f"{w and (w['npc_name'], w['world'], w['x_coord'], w['z_coord'])}")
    n = db.execute("SELECT count(*) FROM npcs WHERE lower(trim(npc_name)) = lower(?)", (BOARD,)).fetchone()[0]
    if n:
        refusals.append(f"{n} npcs rows named {BOARD} already")
    for t, k in (("npcmove", BOARD.lower()), ("npcaggro", BOARD)):
        if db.execute(f"SELECT 1 FROM {t} WHERE lower({KEYS[t]}) = lower(?)", (k,)).fetchone():
            refusals.append(f"{t} has a row for {BOARD} already")
    nl = db.execute("SELECT count(*) FROM npcs WHERE lower(trim(npc_name)) = ?", (LARVA,)).fetchone()[0]
    if nl == 0:
        refusals.append("no moth larva in npcs")
    if db.execute("SELECT count(*) FROM npcaggro WHERE lower(npc_name) = ? AND npc_name != ?", (LARVA, LARVA)).fetchone()[0]:
        refusals.append("npcaggro has another spelling of moth larva: look first")
    if larva is not None and see and larva["see_invis"] not in (0, None):
        refusals.append(f"npcaggro {LARVA}: see_invis {larva['see_invis']} already (a GM's?)")
    if refusals:
        return lines, refusals, False
    v = board_values(db)
    lines.append(f"npcs {WARDEN_ID}: {WARDEN!r} at ({w['x_coord']}, {w['y_coord']}, {w['z_coord']}) facing {w['facing']} "
                 f"body {w['modelid']} -> {BOARD!r} at ({v['x_coord']}, {v['y_coord']}, {v['z_coord']}) facing {v['facing']}, "
                 f"the PvP Events board's look (body {v['modelid']}, size {v['size']})")
    lines.append(f"npcmove {BOARD.lower()!r}: roam 0 (stands, as the other boards)")
    lines.append(f"npcaggro {BOARD!r}: aggressive 0 (talks only, as the other boards)")
    if not see:
        lines.append("npcaggro: add column see_invis INTEGER NOT NULL DEFAULT 0 (as the server's own start does)")
    lines.append(f"npcaggro {LARVA!r}: " + ("insert, aggressive -1 (no word), see_invis 1" if larva is None else
                                            f"see_invis 0 -> 1 (aggressive {larva['aggressive']} kept)") +
                 f" ({nl} npcs rows; petition 1814)")
    return lines, refusals, False


def apply(db):
    db.execute(LOG_SCHEMA)
    n = 0
    if not has_col(db, "npcaggro", "see_invis"):
        db.execute("ALTER TABLE npcaggro ADD COLUMN see_invis INTEGER NOT NULL DEFAULT 0")
        db.execute(f"INSERT INTO {LOG} (tbl) VALUES ('made')")
    before = row(db, "npcs", WARDEN_ID)
    v = board_values(db)
    db.execute("UPDATE npcs SET " + ", ".join(f"{k} = ?" for k in v) + " WHERE id = ?", (*v.values(), WARDEN_ID))
    after = row(db, "npcs", WARDEN_ID)
    log(db, "npcs", WARDEN_ID, before, after, audit(db, "update", "npcs", WARDEN_ID, before, after))
    n += 1
    db.execute("INSERT INTO npcmove (name, roam, radius, water, note, source) VALUES (?, 0, 0, '', ?, 'raidboard 20261112')",
               (BOARD.lower(), f"stands (the {BOARD} board)"))
    after = row(db, "npcmove", BOARD.lower())
    log(db, "npcmove", BOARD.lower(), None, after, audit(db, "insert", "npcmove", BOARD.lower(), None, after))
    db.execute("INSERT INTO npcaggro (npc_name, aggressive, radius, note, source) VALUES (?, 0, 0, ?, 'raidboard 20261112')",
               (BOARD, f"the {BOARD} board: talks only"))
    after = row(db, "npcaggro", BOARD)
    log(db, "npcaggro", BOARD, None, after, audit(db, "insert", "npcaggro", BOARD, None, after))
    n += 2
    before = row(db, "npcaggro", LARVA)
    if before is None:
        db.execute("INSERT INTO npcaggro (npc_name, aggressive, radius, note, source, see_invis) VALUES (?, -1, 0, ?, 'raidboard 20261112', 1)",
                   (LARVA, "petition 1814 (Skywalker): sees through invisibility"))
        after = row(db, "npcaggro", LARVA)
        log(db, "npcaggro", LARVA, None, after, audit(db, "insert", "npcaggro", LARVA, None, after))
    else:
        db.execute("UPDATE npcaggro SET see_invis = 1 WHERE npc_name = ?", (LARVA,))
        after = row(db, "npcaggro", LARVA)
        log(db, "npcaggro", LARVA, before, after, audit(db, "update", "npcaggro", LARVA, before, after))
    return n + 1


def undo(db):
    """(back, left, dropped)."""
    if not table(db, LOG):
        return 0, 0, False
    back = left = 0
    made = False
    for tbl, key, before, after in db.execute(f"SELECT tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        if tbl == "made":
            made = True
            continue
        key = json.loads(key)
        now = row(db, tbl, key)
        if now != json.loads(after):
            left += 1
            print(f"left: {tbl} {key!r} changed since")
            continue
        if before is None:
            db.execute(f"DELETE FROM {tbl} WHERE {KEYS[tbl]} = ?", (key,))
            audit(db, "delete", tbl, key, now, None)
        else:
            was = json.loads(before)
            db.execute(f"UPDATE {tbl} SET " + ", ".join(f"{k} = ?" for k in was) + f" WHERE {KEYS[tbl]} = ?", (*was.values(), key))
            audit(db, "update", tbl, key, now, row(db, tbl, key))
        back += 1
    dropped = False
    if made and not left and db.execute("SELECT count(*) FROM npcaggro WHERE see_invis != 0").fetchone()[0] == 0:
        db.execute("ALTER TABLE npcaggro DROP COLUMN see_invis")
        dropped = True
    db.execute(f"DROP TABLE {LOG}")
    return back, left, dropped


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left, dropped = undo(db)
            db.execute("COMMIT")
            print(f"undone: {back} changes back" + (f", {left} changed since (left)" if left else "") +
                  (", the see_invis column dropped" if dropped else ""))
            return 0
        lines, refusals, applied = plan(db)
        if applied:
            db.execute("ROLLBACK")
            print("applied already: nothing to do")
            return 0
        for l in lines:
            print(l)
        if refusals:
            db.execute("ROLLBACK")
            print("REFUSED:\n  " + "\n  ".join(refusals))
            return 1
        if not a.apply:
            db.execute("ROLLBACK")
            print("(report only: --apply to write)")
            return 0
        n = apply(db)
        db.execute("COMMIT")
        print(f"applied: {n} changes, in gm_audit and {LOG}")
        return 0
    except Exception:
        db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
