#!/usr/bin/env python3
"""Santa's hat and robe on three Christmas NPCs, HEIRLOOM ONLY (the owner, 2026-11-17: "make the toymaker tinsel, holiday
elf weapon enchanter wear santa hats and santas robe"; "also the enchanter is only by the tree").

The names: Toymaker Tinsel, the Holiday Elf, and the Weapon Enchanter by the Christmas tree. npctint and npcdyes key by
NAME, so the Weapon Enchanter is dressed by its name only where every other Weapon Enchanter is held out of the world
(npcspawnonly "#<id>" rows, Heirloom's source enchanter-tree-only-1117) and the one left stands by the tree: REFUSED
otherwise (the Live shard has its 74).

Everything written holds only while the Christmas season is on (a server with branch santa-outfits: npctint.season,
npcdyes.season and npcdyes.robe; the season's turn reloads them). Toymaker Tinsel and the Holiday Elf are of the season
themselves (npcseason); the Weapon Enchanter is not, and wears its own look again when the season is put away.

1. npcdyes, each name: Santa Claus's robe (slot 9: his colour, and his npcs.robe_style as the row's robe), his helm dye
   (5: the hat's red), his trims (11 chest: the robe's top, 14 legs: its skirt, 16 helm: the hat's band), read off his
   rows here, season Christmas, source santa-outfits. Their own gloves, boots and the rest are kept. A row there already
   (Tinsel's chest trim) is replaced, what it was logged and put back by --undo. A name in Santa's robe already (the
   Holiday Elf: the panel set its npcs robe_style 0 and its robe Santa's red, 2026-11-17) is left as it is.
2. npctint, the three: Santa's hat (his row's aura: kind 10, the gnome cap 0xA20590E3, nodes 1). The panel put all
   three hats in 2026-11-17 (source santa-hat, no season): those rows are left as they are, but for the Weapon
   Enchanter's, which (holding no tint or glow) is put in the Christmas season so the hat goes with the robe: the Elf
   and Tinsel are out of the world outside the season anyway. A name with no hat row gets one (no tint, season
   Christmas); a row without the hat has the hat's aura columns set, its tint, glow and season kept.

What a PS2 sees: the robe (every client draws npcdyes.robe) in its nearest disc tint, red; the helm slot's red where the
body wears a helm (the Elf's cloth cap); no hat, no trims.

Report by default; --apply writes it, each row audited (gm_audit) and logged in santa_outfits_1117_log; a second --apply
finds nothing to do; --undo puts back exactly what was there, all or nothing (refused if a row written has changed).
After --apply or --undo: the GM panel's "Reload every NPC" (tints and dyes), or the next start.

    python3 -E -s tools/santa_outfits_20261117.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import math
import sqlite3
import sys

LOG = "santa_outfits_1117_log"
DETAIL = ("Santa's hat and robe on Toymaker Tinsel, the Holiday Elf and the Weapon Enchanter by the tree, in the Christmas "
          "season (the owner, 2026-11-17); tools/santa_outfits_20261117.py")
WHO, VIA = "dab", "tool"
SEASON, SRC = "Christmas", "santa-outfits"
SANTA = "Santa Claus"
TINSEL, ELF, ENCHANTER = "Toymaker Tinsel", "Holiday Elf", "Weapon Enchanter"
NAMES = (TINSEL, ELF, ENCHANTER)
HATTED = (TINSEL, ELF, ENCHANTER)
ROBE, HELM, CHEST_TRIM, LEGS_TRIM, HELM_TRIM = 9, 5, 11, 14, 16
SLOTS = (ROBE, HELM, CHEST_TRIM, LEGS_TRIM, HELM_TRIM)
HAT = 10
HELD_SRC = "enchanter-tree-only-1117"
TREE, NEAR = (25270.0, 15754.7), 30.0

LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    rowkey TEXT NOT NULL,
    before TEXT,
    after  TEXT,
    audit  INTEGER NOT NULL
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def cols(db, tbl):
    return {r[1] for r in db.execute(f"PRAGMA table_info({tbl})")}


def row(db, tbl, key):
    where = " AND ".join(f"{k} = ?" for k in key)
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", tuple(key.values()))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return None if m is None else json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, tbl, key, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    return db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                      "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                      (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key), go_json(before), go_json(after), DETAIL)).lastrowid


def write(db, log, tbl, key, want):
    """Make tbl's row of key want (a whole row; None deletes it), audited (and logged when log)."""
    before = row(db, tbl, key)
    where = " AND ".join(f"{k} = ?" for k in key)
    if want is None:
        db.execute(f"DELETE FROM {tbl} WHERE {where}", tuple(key.values()))
        action = "delete"
    elif before is None:
        c = list(want)
        db.execute(f"INSERT INTO {tbl} ({', '.join(c)}) VALUES ({', '.join('?' for _ in c)})", tuple(want[k] for k in c))
        action = "insert"
    else:
        sets = [c for c in want if c not in key]
        db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in sets)} WHERE {where}", tuple(want[c] for c in sets) + tuple(key.values()))
        action = "update"
    after = row(db, tbl, key)
    a = audit(db, action, tbl, key, before, after)
    if log:
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                   (tbl, go_json(key), go_json(before), go_json(after), a))


def named(db, tbl, col, name):
    """The rows of tbl whose col is name as the server keys it (any case, trimmed)."""
    return [r[0] for r in db.execute(f"SELECT {col} FROM {tbl} WHERE lower(trim({col})) = lower(?)", (name,))]


def plan(db):
    """(writes [(tbl, key, want)], lines, refusals)."""
    for t in ("npcs", "npcdyes", "npctint", "npcseason", "npcspawnonly", "gmseasons", "gm_audit"):
        if not table(db, t):
            return [], [], [f"no {t} table here"]
    if not {"season", "robe"} <= cols(db, "npcdyes") or "season" not in cols(db, "npctint"):
        return [], [], ["npcdyes.season/robe or npctint.season missing: start a server with branch santa-outfits on this database once first "
                        "(an older server would wear these rows all year)"]
    lines, refusals = [], []
    # Santa: the look copied
    santa = db.execute("SELECT id, robe_style FROM npcs WHERE npc_name = ?", (SANTA,)).fetchall()
    if len(santa) != 1:
        return [], [], [f"{len(santa)} npcs rows of {SANTA}, not one"]
    try:
        robe = int(santa[0][1])
    except (TypeError, ValueError):
        robe = -1
    if not 0 <= robe <= 3:
        return [], [], [f"{SANTA}'s robe_style is {santa[0][1]!r}, no robe"]
    dyes = {}
    for slot in SLOTS:
        r = db.execute("SELECT color FROM npcdyes WHERE name = ? AND slot = ?", (SANTA, slot)).fetchone()
        if r is None or r[0] == 0:
            return [], [], [f"{SANTA} has no npcdyes slot {slot}: his suit first"]
        dyes[slot] = r[0] & 0xFFFFFFFF
    hat = db.execute("SELECT aura_kind, aura_source, aura_nodes FROM npctint WHERE name = ?", (SANTA,)).fetchone()
    if hat is None or hat[0] != HAT:
        return [], [], [f"{SANTA} wears no hat (npctint aura kind {HAT}): tools/santa_hat_20261117.py first"]
    hat = (HAT, hat[1] & 0xFFFFFFFF, hat[2])
    # the NPCs
    for n in NAMES:
        if not named(db, "npcs", "npc_name", n):
            refusals.append(f"no npcs row of {n}")
    for n in (TINSEL, ELF):
        s = db.execute("SELECT season FROM npcseason WHERE lower(trim(name)) = lower(?)", (n,)).fetchone()
        if s is None or s[0].strip().lower() != SEASON.lower():
            refusals.append(f"{n} is not of the {SEASON} season (npcseason): look first")
    # the Weapon Enchanter: only the tree's in the world (Heirloom), so its name means it alone
    held = {r[0] for r in db.execute("SELECT name FROM npcspawnonly")}
    if any(k.strip().lower() == ENCHANTER.lower() for k in held):
        refusals.append(f"every {ENCHANTER} is held out by name (npcspawnonly): none to dress")
    left = [(i, x, z) for i, x, z in db.execute("SELECT id, x_coord, z_coord FROM npcs WHERE npc_name = ? COLLATE NOCASE", (ENCHANTER,))
            if f"#{i}" not in held]
    if len(left) != 1:
        refusals.append(f"{len(left)} {ENCHANTER}s in the world, not one (the others held by npcspawnonly #id, source {HELD_SRC}): "
                        "this is Heirloom's change alone; the name would dress them all")
    elif math.hypot(left[0][1] - TREE[0], left[0][2] - TREE[1]) > NEAR:
        refusals.append(f"the one {ENCHANTER} in the world (npcs {left[0][0]}) is not by the tree")
    else:
        lines.append(f"{ENCHANTER}: npcs {left[0][0]} by the tree the only one in the world")
    if refusals:
        return [], lines, refusals
    writes = []
    note = "Santa's suit in the Christmas season (the owner 2026-11-17)"
    for n in NAMES:
        # a name dressed in Santa's robe already (the Holiday Elf, by the panel 2026-11-17: npcs robe_style 0 and its
        # robe row Santa's red): left as it is
        styles = {r[0] for r in db.execute("SELECT robe_style FROM npcs WHERE lower(trim(npc_name)) = lower(?)", (n,))}
        own = db.execute("SELECT color FROM npcdyes WHERE lower(trim(name)) = lower(?) AND slot = ?", (n, ROBE)).fetchone()
        if styles == {str(robe)} and own is not None and own[0] & 0xFFFFFFFF == dyes[ROBE]:
            lines.append(f"npcdyes {n}: wears Santa's robe already (npcs robe_style {robe}, robe {dyes[ROBE]:08X}): left as it is")
            continue
        keys = named(db, "npcdyes", "name", n)
        key_name = keys[0] if keys else n
        if len(set(keys)) > 1:
            return [], lines, [f"npcdyes names {n} as {sorted(set(keys))}: look first"]
        changed = []
        for slot in SLOTS:
            want = {"name": key_name, "slot": slot, "color": dyes[slot], "season": SEASON, "robe": robe if slot == ROBE else -1,
                    "note": note, "source": SRC}
            have = row(db, "npcdyes", {"name": key_name, "slot": slot})
            if have is None or any((have[k] & 0xFFFFFFFF if k == "color" else have[k]) != v for k, v in want.items() if k != "note"):
                writes.append(("npcdyes", {"name": key_name, "slot": slot}, want))
                changed.append(f"{slot}{' (was %08X)' % (have['color'] & 0xFFFFFFFF) if have else ''}")
        if changed:
            lines.append(f"npcdyes {n}: Santa's robe {robe} and colours, slots {', '.join(changed)}, season {SEASON}")
    for n in HATTED:
        t = named(db, "npctint", "name", n)
        if len(t) > 1:
            return [], lines, [f"{len(t)} npctint rows for {n}: look first"]
        if t:
            cur = row(db, "npctint", {"name": t[0]})
            if (cur["aura_kind"], cur["aura_source"] & 0xFFFFFFFF, cur["aura_nodes"]) == hat:
                # the panel's hat (2026-11-17): on a name not itself of the season (the Weapon Enchanter) the row is put
                # in the season, so the hat goes with the robe, when the row holds nothing else (no tint, no glow)
                plain = (cur["color"] & 0xFF) == 0 or (cur["color"] & 0xFFFFFFFF) >> 8 == 0xFFFFFF
                plain = plain and ((cur["glow"] & 0xFF) == 0 or (cur["glow"] & 0xFFFFFFFF) >> 8 == 0)
                if n == ENCHANTER and cur["season"] == "" and plain:
                    writes.append(("npctint", {"name": t[0]}, dict(cur, season=SEASON)))
                    lines.append(f"npctint {t[0]!r}: wears the hat already (source {cur['source']}): put in the {SEASON} season with the robe")
                else:
                    lines.append(f"npctint {t[0]!r}: wears the hat already (source {cur['source']}, season {cur['season']!r}): left as it is")
                continue
            writes.append(("npctint", {"name": t[0]}, dict(cur, aura_kind=hat[0], aura_source=hat[1], aura_nodes=hat[2])))
            lines.append(f"npctint {t[0]!r}: its tint, glow and season ({cur['season']!r}) kept, Santa's hat on its head")
        else:
            writes.append(("npctint", {"name": n}, {"name": n, "color": 0, "glow": 0, "aura_kind": hat[0], "aura_source": hat[1],
                                                     "aura_nodes": hat[2], "season": SEASON, "note": note, "source": SRC}))
            lines.append(f"npctint {n!r}: no tint, Santa's hat, season {SEASON}")
    return writes, lines, []


def undo(db):
    if not table(db, LOG):
        return 0, []
    logged = db.execute(f"SELECT seq, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall()
    left = [f"{tbl} {rowkey}: changed since, nothing undone" for _, tbl, rowkey, before, after in logged
            if go_json(row(db, tbl, json.loads(rowkey))) != after]
    if left:
        return 0, left
    for _, tbl, rowkey, before, _ in logged:
        write(db, False, tbl, json.loads(rowkey), json.loads(before) if before else None)
    db.execute(f"DROP TABLE {LOG}")
    return len(logged), []


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
            print(f"(report only: {len(writes)} rows; --apply to write)")
            return 0
        db.execute(LOG_DDL)
        for tbl, key, want in writes:
            write(db, True, tbl, key, want)
        db.execute("COMMIT")
        print(f"applied: {len(writes)} rows, in gm_audit and {LOG}; press the GM panel's Reload every NPC (tints and dyes)")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
