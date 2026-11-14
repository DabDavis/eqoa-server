#!/usr/bin/env python3
"""Zerkziee's weapons the druid's epic staff glow, and his scimitar the druid's epic scimitar's look (the owner,
2026-11-14: "give zerkziee hachet and ranger level 60 epics the level 49 druid staff glow (also make a template like
other auras) and make his scimitar the epic druid scimitar with same aura").

1. The glow: the hands' charaura slots (9 the main hand, 10 the off hand) kind 7 (database.AuraEpic) with source
   0xCBF4135A, the Oak Staff of Eilerawdl (the druid's level 49 epic, ITEM.ESF Druid_Staff): the held item takes the
   staff's amber emissive (120, 80, 30), its gold additive sheen and its sparkles (its particle sprite); colour 0, the
   staff's own colours. Per hand and per character, whatever the hand holds: his Ascended Axes of Nature's Innocence
   and Purity (the ranger's level 60 epics, model 0x11666A69), his hatchets, the scimitar below. The same look is the
   named aura "druid staff glow" (/aura druid staff glow).
2. The scimitar: his Heroic Ykesha (996023, model 0x47D981E0, held in his main hand on Heirloom) becomes his alone in
   the look of the Triad Scimitar, the druid's level 49 epic scimitar (model 0xD03942DE, ITEM.ESF Druid_Scimitar,
   its icon 2051918763): a new itempattern row copied from 996023 whole (name, description and its proc, every stat)
   with the Triad's model and icon, at the next free id after 714384 in the heirloom range; his charinventory row of
   it moved onto the copy, and his gear sets that name that row. Every other Heroic Ykesha is as it was (the precedent:
   the Heirloom Sword of Twisted Virtue 714382, a copy of its own). Where he has no Heroic Ykesha (Live: Zerkziee is
   level 7 with an Apprentice Sword) the glow alone.

Checked as found, else REFUSED and nothing written: exactly one character named Zerkziee; the staff and the Triad
Scimitar as itempattern rows of those models; tables charaura and gm_audit. Report by default; --apply in one
transaction, a gm_audit line a row (as the panel's), logged in zerkziee_glow_1114_log; a second --apply finds nothing
to do; --undo puts back exactly what was there (the copy deleted when nothing holds it any more), each a gm_audit line,
a row changed since left and listed (and the rest still put back).

After --apply: the panel's "Reload every NPC" shows the glow at the next tick (a server with weapon-taint, build 214
on); the scimitar's new look reaches him at his next login (eject him to apply now).

    python3 -E -s tools/zerkziee_glow_20261114.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "zerkziee_glow_1114_log"
DETAIL = "Zerkziee's weapons the druid staff glow, his scimitar the Triad Scimitar's look (the owner, 2026-11-14); tools/zerkziee_glow_20261114.py"
WHO, VIA = "dab", "tool"
NAME = "Zerkziee"
STAFF, STAFF_NAME = 0xCBF4135A, "Oak Staff of Eilerawdl"
TRIAD, TRIAD_NAME = 0xD03942DE, "Triad Scimitar"
SCIMITAR = 996023  # Heroic Ykesha
COPY_FROM = 714385  # the first id the copy may take (714382-714384 are GM-made heirlooms)
COPY_TO = 714999
SLOTS = (9, 10)  # database.CharAuraMainHand, CharAuraOffHand
NOTE = "Zerkziee's weapons: the druid staff glow (Oak Staff of Eilerawdl, epic look), the owner 2026-11-14"

LOG_DDL = f"""CREATE TABLE {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    rowkey TEXT NOT NULL,      -- the key (JSON)
    before TEXT,               -- the row before (JSON), NULL none
    after  TEXT NOT NULL,      -- the row written (JSON)
    audit  INTEGER NOT NULL
)"""

# each table's key columns
KEYS = {"charaura": ("charid", "merc", "slot"), "itempattern": ("patternid",), "charinventory": ("itemid",),
        "gearsets": ("serverid", "slot")}


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


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
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key), go_json(before), go_json(after), DETAIL))
    return cur.lastrowid


def write(db, tbl, key, want):
    """Make tbl's row of key what want says (want a whole row; None deletes it), audited and logged."""
    before = row(db, tbl, key)
    if want is None:
        where = " AND ".join(f"{k} = ?" for k in key)
        db.execute(f"DELETE FROM {tbl} WHERE {where}", tuple(key.values()))
        action = "delete"
    elif before is None:
        cols = list(want)
        db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' for _ in cols)})", tuple(want[c] for c in cols))
        action = "insert"
    else:
        sets = [c for c in want if c not in key]
        where = " AND ".join(f"{k} = ?" for k in key)
        db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in sets)} WHERE {where}", tuple(want[c] for c in sets) + tuple(key.values()))
        action = "update"
    after = row(db, tbl, key)
    return before, after, audit(db, action, tbl, key, before, after)


def plan(db):
    """(writes: [(tbl, key, want row)], lines, refusals)."""
    refusals, lines, writes = [], [], []
    for t in ("characters", "itempattern", "charinventory", "charaura", "gm_audit"):
        if not table(db, t):
            refusals.append(f"no {t} table here")
    if refusals:
        return writes, lines, refusals
    chars = db.execute("SELECT serverid, level, tclass FROM characters WHERE charName = ?", (NAME,)).fetchall()
    if len(chars) != 1:
        return writes, lines, [f"{len(chars)} characters named {NAME}, want 1"]
    charid, level, tclass = chars[0]
    for model, name in ((STAFF, STAFF_NAME), (TRIAD, TRIAD_NAME)):
        if not db.execute("SELECT 1 FROM itempattern WHERE itemname = ? AND (model & 0xFFFFFFFF) = ?", (name, model)).fetchone():
            refusals.append(f"no itempattern {name!r} of model 0x{model:08X}")
    if refusals:
        return writes, lines, refusals
    held = db.execute("SELECT i.equiploc, p.itemname, p.model FROM charinventory i JOIN itempattern p ON p.patternid = i.patternid "
                      "WHERE i.serverid = ? AND i.equiploc IN (14, 15, 16) ORDER BY i.equiploc", (charid,)).fetchall()
    what = ", ".join(f"equiploc {e} {n} (0x{m & 0xFFFFFFFF:08X})" for e, n, m in held) or "nothing in hand"
    lines.append(f"{NAME}: serverid {charid}, level {level}, class {tclass}; holds {what}")

    # 1. the glow on both hands
    for slot in SLOTS:
        key = {"charid": charid, "merc": 0, "slot": slot}
        want = {"charid": charid, "merc": 0, "slot": slot, "color": 0, "kind": 7, "source": STAFF, "nodes": 0, "note": NOTE}
        before = row(db, "charaura", key)
        hand = "main hand" if slot == 9 else "off hand"
        if before == want:
            lines.append(f"  {hand} (charaura slot {slot}): already the druid staff glow")
            continue
        was = "none" if before is None else f"kind {before['kind']} source 0x{before['source'] & 0xFFFFFFFF:08X} (kept in the log)"
        lines.append(f"  {hand} (charaura slot {slot}): {was} -> kind 7 (epic) source 0x{STAFF:08X}, its own colours")
        writes.append(("charaura", key, want))

    # 2. his scimitar, a copy of its own in the Triad Scimitar's look
    mine = db.execute("SELECT itemid, equiploc, location FROM charinventory WHERE serverid = ? AND patternid = ? ORDER BY (equiploc < 0), itemid",
                      (charid, SCIMITAR)).fetchall()
    done = db.execute("SELECT i.itemid, p.patternid FROM charinventory i JOIN itempattern p ON p.patternid = i.patternid "
                      "WHERE i.serverid = ? AND p.patternid BETWEEN ? AND ? AND (p.model & 0xFFFFFFFF) = ? AND p.itemname = "
                      "(SELECT itemname FROM itempattern WHERE patternid = ?)", (charid, COPY_FROM, COPY_TO, TRIAD, SCIMITAR)).fetchall()
    if done and not mine:
        lines.append(f"  scimitar: already his own copy {done[0][1]} in the Triad Scimitar's look (itemid {done[0][0]})")
        return writes, lines, refusals
    if not mine:
        lines.append("  scimitar: he has no Heroic Ykesha here: the glow alone")
        return writes, lines, refusals
    itemid, equiploc, location = mine[0]
    if len(mine) > 1:
        lines.append(f"  scimitar: {len(mine)} Heroic Ykesha; the {'held' if equiploc >= 0 else 'first'} one, itemid {itemid}, alone")
    orig = row(db, "itempattern", {"patternid": SCIMITAR})
    triad = db.execute("SELECT itemicon, model FROM itempattern WHERE itemname = ? AND (model & 0xFFFFFFFF) = ? ORDER BY patternid LIMIT 1",
                       (TRIAD_NAME, TRIAD)).fetchone()
    taken = {r[0] for r in db.execute("SELECT patternid FROM itempattern WHERE patternid BETWEEN ? AND ?", (COPY_FROM, COPY_TO))}
    free = next((p for p in range(COPY_FROM, COPY_TO + 1) if p not in taken), None)
    if free is None:
        return writes, lines, refusals + [f"no free itempattern id in {COPY_FROM}-{COPY_TO}"]
    copy = dict(orig, patternid=free, model=triad[1], itemicon=triad[0])  # the model as the Triad's row stores it
    lines.append(f"  scimitar: itemid {itemid} (equiploc {equiploc}, location {location}) Heroic Ykesha {SCIMITAR} -> his own copy {free}: "
                 f"model 0x{orig['model'] & 0xFFFFFFFF:08X} -> 0x{TRIAD:08X} (Triad Scimitar), icon {orig['itemicon']} -> {triad[0]}, the rest as 996023")
    writes.append(("itempattern", {"patternid": free}, copy))
    inv = row(db, "charinventory", {"itemid": itemid})
    writes.append(("charinventory", {"itemid": itemid}, dict(inv, patternid=free)))
    if table(db, "gearsets"):
        for g in db.execute("SELECT slot FROM gearsets WHERE serverid = ? AND (mainitem = ? OR offitem = ?)", (charid, itemid, itemid)).fetchall():
            key = {"serverid": charid, "slot": g[0]}
            gs = row(db, "gearsets", key)
            want = dict(gs)
            if gs["mainitem"] == itemid:
                want["mainpattern"] = free
            if gs["offitem"] == itemid:
                want["offpattern"] = free
            lines.append(f"  gear set {g[0]} ({gs['name'] or 'unnamed'}): names it, its pattern {free}")
            writes.append(("gearsets", key, want))
    return writes, lines, refusals


def apply(db, writes):
    db.execute(LOG_DDL)
    for tbl, key, want in writes:
        before, after, a = write(db, tbl, key, want)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                   (tbl, go_json(key), go_json(before), go_json(after), a))
    return len(writes)


def undo(db):
    """(back, left: [why])."""
    if not table(db, LOG):
        return 0, []
    back, left = 0, []
    for tbl, rowkey, before, after in db.execute(f"SELECT tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key = json.loads(rowkey)
        if go_json(row(db, tbl, key)) != after:
            left.append(f"{tbl} {rowkey}: changed since, left as it is")
            continue
        if tbl == "itempattern" and before is None and \
                db.execute("SELECT count(*) FROM charinventory WHERE patternid = ?", (key["patternid"],)).fetchone()[0]:
            left.append(f"itempattern {key['patternid']}: still held, left")
            continue
        write(db, tbl, key, json.loads(before) if before else None)
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
                  ("; press the GM panel's Reload every NPC (and eject Zerkziee for his scimitar)" if back else ""))
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
        print(f"applied: {n} rows, in gm_audit and {LOG}; press the GM panel's Reload every NPC for the glow; "
              "his scimitar's look at his next login (eject him to apply now)")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
