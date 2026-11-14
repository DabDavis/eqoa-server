#!/usr/bin/env python3
"""Virulent Blades on the Isle of Dread (2026-11-14; the owner: "lets add virulent blades to the database IoD mob"; the
wiki, https://wiki.eqoa.live/index.php?title=Virulent_Blade, saved in ~/.cache/iodaudit/wiki/: Warrior, Animated
family, level 53, 40,563 HP, respawn 15 Minutes, "roaming Isle of Dread", A, S(I), faction Denizens of Dread,
opposing Citizens of Graumord, no drops listed; no map point, no location anywhere: forumotion t198 does not name it,
FrontiersForge's journal row is the recorder's spot outside the Estate's door).

Six of them, ordinary IoD creatures as the island's other natives are (wicker man, breath stealer: whatever those have
on this database, nothing of the blade's own):

  npcs: six copies of a wicker man row (118919: race, zone 1125, level 53) named "virulent blade" (generic lowercase,
    as the island's natives), body 0x0021353C (CHAR.ESF 56: a ring of floating clean steel weapons, a sword, a
    scimitar, a dagger, a spear, a club, knives; Steel Tempest's body, one row elsewhere; by sight; the wiki's picture
    VirulentBlade.png was never uploaded, so nothing to match it against; the other floating-weapons body, 57
    0x95B15892, is the island's Bloodletter's, kept his), size 1.5 (the natives' middle). Six: the island's smallest
    ordinary kinds (devourer, ray of agony) have six rows; one to an area, a roaming presence, not a camp. Their spots,
    on the terrain's floor off the ground file, each dry and found with its 35 and 70 rings (the roam radius) dry, its
    ring within 20 of its height, 250+ from Graumord, 150+ from the Estate's door and every named, 40+ from any NPC,
    spread by farthest-point choice: N (24680, 54.125, 2310), NW (24700, 63.116, 2830), W (24300, 58.5, 3130),
    E (25220, 54.125, 2870), SE (25500, 54.125, 3310), S (25080, 54.125, 3610).
  npcmove "virulent blade": roam 1, radius 0 (the server's 70), water land: as the island's walkers (wicker man,
    breath stealer: roam 1, Sandstorm's census), kept out of the sea.
  npcclass 0 (Warrior); npctemplate "virulent blade" -> the wicker man's template here (Heirloom "iod native", the
    natives' board 430 fit; Live "roamer"); refused if the wicker man and the breath stealer disagree.
  npcaggro: aggressive, sees invisible (A, S(I)); npcfaction: Denizens of Dread; factionhits: Citizens of Graumord +5
    (the natives' quest-seed figure).
  npcrespawn: 900 s a row (the wiki's 15 minutes).
  lootcreature: the wicker man's region (forest): the island's ordinary loot, as the natives roll it.

Rows are found by key and checked; anything not as expected refuses the whole run. Report only by default; --apply in
one transaction (a second --apply finds its log and does nothing); --undo exact (a row changed since is left, said).

    tools/virulent_blade_20261114.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "virulent_blade_1114_log"
WHO = "virulent-blade"
VIA = "tool"
SRC = "virulentblade1114"
DETAIL = "Virulent Blades on the Isle of Dread (the owner, 2026-11-14; wiki.eqoa.live): tools/virulent_blade_20261114.py"

NAME = KEY = "virulent blade"
SOURCE_ROW = 118919  # a wicker man of the pit east of Graumord
LIKE = ("wicker man", "breath stealer")  # the island's natives whose template, region and roam the blades take
BODY = 0x0021353C
SIZE = 1.5
LEVEL = 53
RESPAWN = 900.0
FACTION, OPPOSING = "Denizens of Dread", "Citizens of Graumord"
SPOTS = [  # (x, y, z, facing)
    (24680.0, 54.125, 2310.0, 0),
    (24700.0, 63.116, 2830.0, 64),
    (24300.0, 58.5, 3130.0, 128),
    (25220.0, 54.125, 2870.0, 192),
    (25500.0, 54.125, 3310.0, 32),
    (25080.0, 54.125, 3610.0, 160),
]

KEYS = {"npcs": ("id",), "npcclass": ("name",), "npctemplate": ("name",), "npcaggro": ("npc_name",),
        "npcfaction": ("id",), "factionhits": ("id",), "npcrespawn": ("npc_row",), "npcmove": ("name",),
        "lootcreature": ("npc_name",)}
SEQS = ("npcs", "npcfaction", "factionhits")
AUTO = {"npcs": "id", "npcfaction": "id", "factionhits": "id"}


def table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def row_of(db, tbl, key):
    where = " AND ".join(f"{c} = ?" for c in KEYS[tbl])
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", tuple(key[c] for c in KEYS[tbl]))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def key_of(tbl, row):
    return {c: row[c] for c in KEYS[tbl]}


def go_json(m):
    """A row as Go's json.Marshal writes a map (keys sorted, <>& escaped), as the panel's audit has it."""
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def one(db, sql, args):
    r = db.execute(sql, args).fetchone()
    return None if r is None else r[0]


def plan(db):
    """(changes, lines, refusals). A change is (tbl, after row, token or None); a value ("$", tok) in a row is the id
    the row of that token took."""
    changes, lines, refusals = [], [], []
    if one(db, "SELECT count(*) FROM npcs WHERE lower(trim(npc_name)) = ?", (KEY,)):
        refusals.append(f"an NPC called {NAME!r} is there already")
    src = row_of(db, "npcs", {"id": SOURCE_ROW})
    if src is None or src["npc_name"].lower() != "wicker man" or src["world"] != 0 or src["npc_level"] != LEVEL:
        refusals.append(f"npcs {SOURCE_ROW} is not a level {LEVEL} wicker man of world 0: {src}")
    for f in (FACTION, OPPOSING):
        if not one(db, "SELECT count(*) FROM factions WHERE name = ?", (f,)):
            refusals.append(f"no faction {f!r}")
    tmpls = {n: one(db, "SELECT template FROM npctemplate WHERE name = ?", (n,)) for n in LIKE}
    regions = {n: one(db, "SELECT region FROM lootcreature WHERE npc_name = ?", (n,)) for n in LIKE}
    roams = {n: one(db, "SELECT roam FROM npcmove WHERE name = ?", (n,)) for n in LIKE}
    if len(set(tmpls.values())) != 1:
        refusals.append(f"the natives' templates disagree: {tmpls}")
    if len(set(regions.values())) != 1 or None in regions.values():
        refusals.append(f"the natives' loot regions disagree or are missing: {regions}")
    if set(roams.values()) != {1}:
        refusals.append(f"the natives' roam is not 1: {roams}")
    tmpl = tmpls[LIKE[0]]
    if tmpl is not None and not one(db, "SELECT count(*) FROM classtemplates WHERE name = ?", (tmpl,)):
        refusals.append(f"no classtemplates {tmpl!r}")
    for tbl, k in (("npcclass", {"name": KEY}), ("npctemplate", {"name": KEY}), ("npcaggro", {"npc_name": KEY}),
                   ("npcmove", {"name": KEY}), ("lootcreature", {"npc_name": KEY})):
        if row_of(db, tbl, k) is not None:
            refusals.append(f"{tbl} {k} is there already")
    if one(db, "SELECT count(*) FROM npcfaction WHERE lower(name) = ?", (KEY,)):
        refusals.append(f"npcfaction {KEY!r} has rows already")
    if one(db, "SELECT count(*) FROM factionhits WHERE lower(npc_name) = ?", (KEY,)):
        refusals.append(f"factionhits {KEY!r} has rows already")
    if refusals:
        return changes, lines, refusals

    for i, (x, y, z, facing) in enumerate(SPOTS):
        npc = {c: v for c, v in src.items() if c != "id"}
        npc.update(npc_name=NAME, x_coord=x, y_coord=y, z_coord=z, facing=facing, modelid=BODY, size=SIZE, npc_id=0)
        changes.append(("npcs", npc, f"B{i}"))
        changes.append(("npcrespawn", {"npc_row": ("$", f"B{i}"), "seconds": RESPAWN,
                                       "note": "the wiki's Virulent Blade: 15 Minutes", "source": SRC}, None))
        lines.append(f"npcs: {NAME!r} at ({x}, {y}, {z}) world 0 facing {facing}, body 0x{BODY:08X} size {SIZE}, level "
                     f"{LEVEL}; npcrespawn {RESPAWN:.0f} s")
    changes.append(("npcmove", {"name": KEY, "roam": 1, "radius": 0.0, "water": "land",
                                "note": "the wiki: 'roaming Isle of Dread'; as the island's walkers (wicker man, breath stealer)",
                                "source": SRC}, None))
    changes.append(("npcclass", {"name": KEY, "class": 0, "source": SRC}, None))
    if tmpl is not None:
        changes.append(("npctemplate", {"name": KEY, "template": tmpl,
                                        "note": f"the island's natives' ({LIKE[0]}, {LIKE[1]})", "source": SRC}, None))
    changes.append(("npcaggro", {"npc_name": KEY, "aggressive": 1, "radius": 0.0, "leash": 0.0, "holding": 0, "see_invis": 1,
                                 "note": "the wiki: A, S(I)", "source": SRC}, None))
    changes.append(("npcfaction", {"name": KEY, "world": -1, "x": 0.0, "z": 0.0, "radius": 0.0, "faction": FACTION,
                                   "note": "the wiki: Denizens of Dread", "source": SRC}, None))
    changes.append(("factionhits", {"npc_name": NAME, "faction": OPPOSING, "delta": 5, "source": SRC}, None))
    changes.append(("lootcreature", {"npc_name": KEY, "region": regions[LIKE[0]], "profile": "", "source": SRC}, None))
    lines.append(f"npcmove {KEY!r}: roam 1, the server's radius, land; npcclass 0; npctemplate -> {tmpl!r}; npcaggro: "
                 f"aggressive, sees invisible; npcfaction: {FACTION}; factionhits: {OPPOSING} +5; lootcreature region "
                 f"{regions[LIKE[0]]!r}")
    return changes, lines, refusals


LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,
    rowkey  TEXT NOT NULL,      -- the row's key (JSON)
    before  TEXT,               -- the row before (JSON), NULL: made here
    after   TEXT,               -- the row as written (JSON)
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def audit(db, action, tbl, key, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key),
                      None if before is None else go_json(before), None if after is None else go_json(after), detail))
    return cur.lastrowid


def seqs(db):
    return {t: one(db, "SELECT seq FROM sqlite_sequence WHERE name = ?", (t,)) for t in SEQS}


def apply(db, changes):
    """The plan written, in the caller's transaction: each row and its gm_audit line, its own line in the log."""
    db.execute(LOG_SCHEMA)
    before_seq = seqs(db)
    ids = {}
    for tbl, after, tok in changes:
        after = {c: (ids[v[1]] if isinstance(v, tuple) and v[0] == "$" else v) for c, v in after.items()}
        cols = list(after)
        cur = db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [after[c] for c in cols])
        if tbl in AUTO:
            key = {AUTO[tbl]: cur.lastrowid}
            if tbl == "npcs":
                db.execute("UPDATE npcs SET npc_id = ? WHERE id = ?", (cur.lastrowid, cur.lastrowid))
        else:
            key = key_of(tbl, after)
        if tok:
            ids[tok] = cur.lastrowid
        row = row_of(db, tbl, key)
        a = audit(db, "insert", tbl, key, None, row, DETAIL)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, NULL, ?, ?)",
                   (tbl, json.dumps(key), json.dumps(row, sort_keys=True, default=str), a))
    db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES ('sqlite_sequence', '{{}}', ?, ?)",
               (json.dumps(before_seq), json.dumps(seqs(db))))
    return len(changes), ids


def undo(db):
    """Each logged row still as written taken out, the last first, its gm_audit line; then the ids the new rows took,
    when no other row took one since. Returns (back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    seq_row = db.execute(f"SELECT before, after FROM {LOG} WHERE tbl = 'sqlite_sequence'").fetchone()
    for seq, tbl, key, after in db.execute(
            f"SELECT seq, tbl, rowkey, after FROM {LOG} WHERE tbl != 'sqlite_sequence' ORDER BY seq DESC").fetchall():
        key = json.loads(key)
        now = row_of(db, tbl, key)
        if now is None or json.dumps(now, sort_keys=True, default=str) != after:
            left += 1
            print(f"  left: {tbl} {key}, changed since it was written")
            continue
        db.execute(f"DELETE FROM {tbl} WHERE " + " AND ".join(f"{c} = ?" for c in KEYS[tbl]), [key[c] for c in KEYS[tbl]])
        audit(db, "delete", tbl, key, now, None, "undo: " + DETAIL)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if left == 0:
        if seq_row is not None:
            was, written = json.loads(seq_row[0]), json.loads(seq_row[1])
            cur = seqs(db)
            for t in SEQS:
                if was.get(t) is not None and cur.get(t) == written.get(t):
                    db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = ?", (was[t], t))
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
    try:
        if a.undo:
            db.execute("BEGIN IMMEDIATE")
            try:
                back, left = undo(db)
                db.execute("COMMIT")
            except BaseException:
                db.execute("ROLLBACK")
                raise
            print(f"put back {back} rows, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        if table(db, LOG):
            print(f"applied already ({LOG} is there): nothing to do (--undo first to write it again)")
            return 0
        changes, lines, refusals = plan(db)
        for line in lines:
            print(line)
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not a.apply:
            print(f"report only: {len(changes)} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            n, ids = apply(db, changes)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {n} rows, each in gm_audit and {LOG}; the blades' rows "
              f"{', '.join(str(ids[f'B{i}']) for i in range(len(SPOTS)))}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
