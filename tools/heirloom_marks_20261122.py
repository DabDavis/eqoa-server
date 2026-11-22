#!/usr/bin/env python3
"""Heirloom Marks' data on the Heirloom Test Server (work board 1927, a GM's spec, approved as submitted; the code is
internal/handlers/marks.go, keeper.go, hotzones.go, relive.go, heirloomrank.go, behind world.heirloom_marks).

WHAT IT WRITES (one transaction, a gm_audit line a row, kept in table heirloom_marks_1122_log so --undo is exact):

  - item 966020 "Ring of Guises" (else the next free 966xxx): worn nowhere (equipslot -1), procanim 2 (Use), NO TRADE
    (trade 1), LORE, worth 0, a ring's icon; and its itemeffect row "Self, Unlimited Uses, 30min", cooldown 60, as
    Santa's Elf (716912) has. Its Use is the server's (keeper.go): the guise the character picked, for 30 minutes.
  - item 966021 "Mastery Tome" (else the next free): equipslot -1, procanim 2, NO TRADE (not LORE), stacks to 20, a
    tome's icon.
    Its Use is the server's: 49,117,200 experience for a level 60 not reliving.
  - table heirloomrankac (made when missing, as the server makes it): the GM's seed, 15 classes x 6 ranks, the AC each
    worn heirloom armour piece gets at each Heirloom Rank.
  - NPCs (npcs, npcmove "stands", npcaggro "talks only"):
      * "Hot Zone Herald" in Freeport 6 east of Dabrish (world 0), on his floor, facing him, in the Teleporter's look;
      * three "Heirloom Keeper"s, 4 west of Rehabilitator Anja, 4 west of Rehabilitator Frejo and 6 west of Dabrish,
        on their floors, in an Heirloom Outfitter's look but talkers (npc_type 128), facing as their neighbour does.
    Every spot is found from its neighbour's row by NAME; one within ROOM of another NPC refuses the whole run.
  - quests.marks and charquests.taken: the columns made when missing (the server makes them too). No quest's marks are
    set: every quest's are computed (the panel's Quests page shows each figure; a GM sets one in the quests table).

Refused, nothing written, when a neighbour is missing, a spot is crowded, or a record of those names exists that this
tool did not write. Idempotent: what is there as this tool writes it is left alone.

WHEN IT TAKES EFFECT: items, the itemeffect row and heirloomrankac at once (the server reads them as it goes); the NPCs
at a start: run it in the keep restart's before.sh of the build with this code (or with the server stopped).
Heirloom only: Live runs none of it.

    python3 -E -s tools/heirloom_marks_20261122.py --db eqoa.db [--dry | --apply | --undo]
"""
import argparse
import datetime
import json
import math
import sqlite3
import sys

SRC = "heirloom-marks 20261122"
LOG = "heirloom_marks_1122_log"
DETAIL = "Heirloom Marks (work board 1927): tools/heirloom_marks_20261122.py"
WHO, VIA = "dab", "tool"
ROOM = 2.5  # the least room round a new NPC's spot

RING, TOME = "Ring of Guises", "Mastery Tome"
RING_ID, TOME_ID = 966020, 966021
FREE_FROM, FREE_TO = 966000, 966999
RING_DESC = ("A ring of many faces, from the Heirloom Keeper. Use it to take the guise you chose for 30 minutes; "
             "the Keeper unlocks more guises and changes yours.")
TOME_DESC = ("A Mastery Tome, from the Heirloom Keeper. A level 60 who reads it learns as much as from 150 even fights. "
             "Not while you relive.")
RING_ICON_FROM = "Santa's Elf"      # its effect's icon and cooldown
TOME_ICON_FROM = "Tome of Illusion"
HERALD, KEEPER = "Hot Zone Herald", "Heirloom Keeper"
HERALD_LOOK, KEEPER_LOOK = "Teleporter", "Heirloom Outfitter"

# heirloomrankac's seed: class: AC a piece at ranks 1-6 (work 1927 8b)
RANK_AC = {
    0: (50, 67, 101, 138, 194, 257), 1: (50, 54, 70, 78, 91, 103), 2: (50, 67, 101, 139, 196, 269),
    3: (50, 61, 78, 103, 140, 195), 4: (50, 64, 84, 115, 161, 238), 5: (50, 58, 79, 96, 121, 153),
    6: (50, 58, 79, 96, 121, 153), 7: (50, 60, 75, 97, 130, 179), 8: (50, 54, 68, 75, 87, 103),
    9: (50, 54, 68, 76, 87, 104), 10: (50, 63, 81, 109, 151, 213), 11: (50, 63, 81, 109, 151, 213),
    12: (50, 63, 81, 109, 151, 213), 13: (50, 62, 79, 105, 143, 200), 14: (50, 59, 73, 93, 123, 169),
}
RANK_AC_SCHEMA = """CREATE TABLE IF NOT EXISTS heirloomrankac (
	class INTEGER NOT NULL,
	rank  INTEGER NOT NULL,
	ac    INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (class, rank)
)"""

# the NPCs: (name, neighbour, offset east, offset south, distance asked, face the neighbour or as it does)
SPOTS = [
    (HERALD, "Dabrish", 6.0, 0.0, "faces"),
    (KEEPER, "Rehabilitator Anja", -4.0, 0.0, "as"),
    (KEEPER, "Rehabilitator Frejo", -4.0, 0.0, "as"),
    (KEEPER, "Dabrish", -6.0, 0.0, "as"),
]
MOVE = {HERALD: f"stands (the {HERALD}, work 1927)", KEEPER: f"stands (the {KEEPER}, work 1927)"}
AGGRO = {HERALD: f"the {HERALD}: talks only", KEEPER: f"the {KEEPER}: talks only"}

KEYS = {"itempattern": ("patternid",), "itemeffect": ("patternid",), "heirloomrankac": ("class", "rank"),
        "npcs": ("id",), "npcmove": ("name",), "npcaggro": ("npc_name",)}
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    rowkey TEXT NOT NULL,      -- the row's key (JSON, by column)
    after  TEXT NOT NULL,      -- the row as this tool left it (JSON); every change is an insert
    audit  INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def cols(db, t):
    return [r[1] for r in db.execute(f"PRAGMA table_info({t})")]


def row(db, t, key):
    where = " AND ".join(f"{c} = ?" for c in KEYS[t])
    cur = db.execute(f"SELECT * FROM {t} WHERE {where}", [key[c] for c in KEYS[t]])
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, t, key, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, t, go_json(key),
                      go_json(before) if before is not None else "", go_json(after) if after is not None else "", DETAIL))
    return cur.lastrowid


def log(db, t, key, after, a):
    db.execute(f"INSERT INTO {LOG} (tbl, rowkey, after, audit) VALUES (?, ?, ?, ?)", (t, go_json(key), go_json(after), a))


def insert(db, t, vals):
    names = list(vals)
    cur = db.execute(f"INSERT INTO {t} ({', '.join(names)}) VALUES ({', '.join('?' * len(names))})", [vals[n] for n in names])
    return cur.lastrowid


def logged(db, t):
    """The keys this tool wrote in t."""
    if not table(db, LOG):
        return set()
    return {r[0] for r in db.execute(f"SELECT rowkey FROM {LOG} WHERE tbl = ?", (t,))}


def heading(x, z, tx, tz):
    """CLIENT's heading byte from (x, z) toward (tx, tz): 0 is +z, 64 +x."""
    a = math.atan2(tx - x, tz - z)
    if a < 0:
        a += 2 * math.pi
    return int(round(a / (2 * math.pi) * 256)) % 256


def free_id(db, want):
    if not db.execute("SELECT 1 FROM itempattern WHERE patternid = ?", (want,)).fetchone():
        return want
    for i in range(FREE_FROM, FREE_TO + 1):
        if not db.execute("SELECT 1 FROM itempattern WHERE patternid = ?", (i,)).fetchone():
            return i
    return None


def by_name(db, t, col, name):
    return db.execute(f"SELECT * FROM {t} WHERE {col} = ? ORDER BY 1 LIMIT 1", (name,)).fetchone()


def item_row(db, pid, name, desc, icon, maxstack):
    r = {c: 0 for c in cols(db, "itempattern")}
    r.update(patternid=pid, itemname=name, itemdesc=desc, itemicon=icon, equipslot=-1, trade=1, rent=1, lore=1 if maxstack == 1 else 0,
             procanim=2, maxstack=maxstack, levelreq=1, duration=80, classuse=32767, raceuse=32767, color=4294967295,
             itemcost=0, model=0)
    for c in list(r):
        if c not in cols(db, "itempattern"):
            del r[c]
    return r


def plan(db):
    """(lines, refusals, writes): writes is a list of (table, row) to insert, in order."""
    for t in ("itempattern", "itemeffect", "npcs", "npcmove", "npcaggro", "gm_audit"):
        if not table(db, t):
            return [], [f"no {t} table here"], []
    lines, refusals, writes = [], [], []
    mine = {t: logged(db, t) for t in KEYS}

    # the records
    for name, want, desc, icon_from, stack in ((RING, RING_ID, RING_DESC, None, 1), (TOME, TOME_ID, TOME_DESC, TOME_ICON_FROM, 20)):
        have = db.execute("SELECT patternid FROM itempattern WHERE itemname = ?", (name,)).fetchall()
        if have:
            if all(go_json({"patternid": h[0]}) in mine["itempattern"] for h in have):
                lines.append(f"itempattern {have[0][0]} {name!r}: there already (this tool's)")
            else:
                refusals.append(f"a record named {name!r} is there already ({[h[0] for h in have]}), not this tool's")
            continue
        pid = free_id(db, want)
        if pid is None:
            refusals.append(f"no free id in {FREE_FROM}-{FREE_TO} for {name!r}")
            continue
        if icon_from:
            ic = db.execute("SELECT itemicon FROM itempattern WHERE itemname = ? LIMIT 1", (icon_from,)).fetchone()
        else:  # the commonest ring icon
            ic = db.execute("SELECT itemicon FROM itempattern WHERE equipslot = 8 GROUP BY itemicon ORDER BY count(*) DESC LIMIT 1").fetchone()
        icon = ic[0] if ic else 0
        writes.append(("itempattern", item_row(db, pid, name, desc, icon, stack)))
        lines.append(f"itempattern {pid} {name!r}: equipslot -1, procanim 2, NO TRADE{', LORE' if stack == 1 else ''}, maxstack {stack}, icon {icon}")
        if name == RING:
            elf = db.execute("SELECT icon, cooldown FROM itemeffect WHERE name = ? LIMIT 1", (RING_ICON_FROM,)).fetchone()
            eff = {"patternid": pid, "line": "Self, Unlimited Uses, 30min", "name": RING, "icon": elf[0] if elf else 0,
                   "breaks": "", "wear": 0, "area": 0.0, "source": SRC, "body": 0, "target_line": "", "charges": 0,
                   "cooldown": 60.0}
            eff = {k: v for k, v in eff.items() if k in cols(db, "itemeffect")}
            if db.execute("SELECT 1 FROM itemeffect WHERE patternid = ?", (pid,)).fetchone():
                refusals.append(f"itemeffect has a row for {pid} already")
            writes.append(("itemeffect", eff))
            lines.append(f"itemeffect {pid}: \"Self, Unlimited Uses, 30min\", cooldown 60 (icon {eff.get('icon')})")

    # heirloomrankac
    n = 0
    for cls, acs in RANK_AC.items():
        for rank, ac in enumerate(acs, 1):
            if table(db, "heirloomrankac") and db.execute("SELECT 1 FROM heirloomrankac WHERE class = ? AND rank = ?", (cls, rank)).fetchone():
                continue
            writes.append(("heirloomrankac", {"class": cls, "rank": rank, "ac": ac}))
            n += 1
    lines.append(f"heirloomrankac: {n} rows of {len(RANK_AC) * 6} to write")

    # the NPCs
    names_there = {r[0] for r in db.execute("SELECT DISTINCT npc_name FROM npcs WHERE npc_name IN (?, ?)", (HERALD, KEEPER))}
    if names_there:
        rows = db.execute("SELECT id FROM npcs WHERE npc_name IN (?, ?)", (HERALD, KEEPER)).fetchall()
        if all(go_json({"id": r[0]}) in mine["npcs"] for r in rows):
            lines.append(f"npcs: {len(rows)} herald and keepers there already (this tool's)")
        else:
            refusals.append(f"npcs named {sorted(names_there)} are there already, not this tool's")
    else:
        looks = {}
        for look in (HERALD_LOOK, KEEPER_LOOK):
            cur = db.execute("SELECT * FROM npcs WHERE npc_name = ? ORDER BY id LIMIT 1", (look,))
            r = cur.fetchone()
            if r is None:
                refusals.append(f"no NPC named {look!r} to take a look from")
                continue
            looks[look] = dict(zip([d[0] for d in cur.description], r))
        placed = []
        for name, nb, dx, dz, face in SPOTS:
            cur = db.execute("SELECT * FROM npcs WHERE npc_name = ? AND world = 0 ORDER BY id LIMIT 1", (nb,))
            r = cur.fetchone()
            if r is None:
                refusals.append(f"no {nb!r} in world 0 to stand beside")
                continue
            n_ = dict(zip([d[0] for d in cur.description], r))
            x, z = round(n_["x_coord"] + dx, 3), round(n_["z_coord"] + dz, 3)
            y = n_["y_coord"]
            facing = heading(x, z, n_["x_coord"], n_["z_coord"]) if face == "faces" else n_["facing"]
            near = db.execute("SELECT id, npc_name, x_coord, z_coord FROM npcs WHERE world = 0 AND x_coord BETWEEN ? AND ? "
                              "AND z_coord BETWEEN ? AND ?", (x - 10, x + 10, z - 10, z + 10)).fetchall()
            near = sorted((math.hypot(nx - x, nz - z), i, nm) for i, nm, nx, nz in near if nx is not None and nz is not None)
            near += [(math.hypot(px - x, pz - z), 0, pn) for pn, px, pz in placed]
            near.sort()
            for d, i, nm in near:
                if d < ROOM:
                    refusals.append(f"{name} beside {nb}: {nm!r} ({i}) stands {d:.1f} from ({x}, {z})")
            placed.append((name, x, z))
            look = looks.get(HERALD_LOOK if name == HERALD else KEEPER_LOOK)
            if look is None:
                continue
            new = dict(look)
            new.pop("id", None)
            new.update(npc_name=name, world=0, x_coord=x, y_coord=y, z_coord=z, facing=facing, npc_type=128,
                       npc_level=60, dt_kill=None, npc_id=0)
            writes.append(("npcs", new))
            lines.append(f"npcs (new): {name!r} beside {nb} at ({x}, {y}, {z}) facing {facing}; nearest: " +
                         ", ".join(f"{nm} {d:.1f}" for d, _, nm in near[:3]))
        for name in (HERALD, KEEPER):
            if db.execute("SELECT 1 FROM npcmove WHERE lower(name) = lower(?)", (name,)).fetchone():
                refusals.append(f"npcmove has a row for {name} already")
            else:
                vals = {"name": name.lower(), "roam": 0, "radius": 0.0, "water": "", "note": MOVE[name], "source": SRC}
                writes.append(("npcmove", {k: v for k, v in vals.items() if k in cols(db, "npcmove")}))
            if db.execute("SELECT 1 FROM npcaggro WHERE lower(npc_name) = lower(?)", (name,)).fetchone():
                refusals.append(f"npcaggro has a row for {name} already")
            else:
                vals = {"npc_name": name, "aggressive": 0, "radius": 0.0, "note": AGGRO[name], "source": SRC}
                writes.append(("npcaggro", {k: v for k, v in vals.items() if k in cols(db, "npcaggro")}))
        lines.append(f"npcmove/npcaggro: {HERALD!r} and {KEEPER!r} stand and talk only")

    # the columns
    for t, c in (("quests", "marks"), ("charquests", "taken")):
        if table(db, t) and c not in cols(db, t):
            lines.append(f"{t}.{c}: made (INTEGER, NULL)")
    return lines, refusals, writes


def apply(db, writes):
    db.execute(LOG_SCHEMA)
    db.execute(RANK_AC_SCHEMA)
    for t, c in (("quests", "marks"), ("charquests", "taken")):
        if table(db, t) and c not in cols(db, t):
            db.execute(f"ALTER TABLE {t} ADD COLUMN {c} INTEGER")
    n = 0
    for t, vals in writes:
        rid = insert(db, t, vals)
        if t == "npcs":
            db.execute("UPDATE npcs SET npc_id = ? WHERE id = ?", (rid, rid))
            key = {"id": rid}
        else:
            key = {c: vals[c] for c in KEYS[t]}
        after = row(db, t, key)
        log(db, t, key, after, audit(db, "insert", t, key, None, after))
        n += 1
    return n


def undo(db):
    """(back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, tbl, key, after in db.execute(f"SELECT seq, tbl, rowkey, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key = json.loads(key)
        now = row(db, tbl, key)
        if now is None:
            db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
            continue
        if json.loads(go_json(now)) != json.loads(after):
            left += 1
            print(f"left: {tbl} {key} changed since")
            continue
        where = " AND ".join(f"{c} = ?" for c in KEYS[tbl])
        db.execute(f"DELETE FROM {tbl} WHERE {where}", [key[c] for c in KEYS[tbl]])
        audit(db, "delete", tbl, key, now, None)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--dry", action="store_true", help="report only (the default)")
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 5000")
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = undo(db)
            print(f"undo: {back} rows taken back, {left} left as changed since")
            db.execute("COMMIT")
            return
        lines, refusals, writes = plan(db)
        for l in lines:
            print(l)
        if refusals:
            for r in refusals:
                print("REFUSED:", r)
            db.execute("ROLLBACK")
            sys.exit(1)
        if not writes:
            print("nothing to do")
            db.execute("ROLLBACK")
            return
        if not a.apply:
            print(f"(report only: --apply writes {len(writes)} rows)")
            db.execute("ROLLBACK")
            return
        n = apply(db, writes)
        db.execute("COMMIT")
        print(f"applied: {n} rows")
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    main()
