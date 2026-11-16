#!/usr/bin/env python3
"""Drizzt's weapons, Twinkle and Icingdeath, in the ranger heirloom scimitar's look, each with its own aura (the owner,
2026-11-16: "lets give make drizzt special weapons twinkle and icingdeath look like ranger heirloom scimitar but icing
death freezing aura and blue. twinkle the aura from druid epic.").

1. The look: the ranger's heirloom scimitar is the Heirloom Scimitar of Fury (714027, heirlooms class 1 = ranger, role
   main, epic 701006 Scimitar of Fury): ITEM.ESF model 0xAB6B7D29, icon 1086674104, colour 0xFFFFFFFF (plain). Both
   records take its model and icon (an heirloom keeps its record's own look: handlers/heirloom.go; there is no size
   column, the size is the model's). Twinkle takes its plain colour too (it was 0x4A7BFFFF, blue: the owner's "but
   icingdeath ... blue" keeps the blue for Icingdeath alone); Icingdeath is blue, 0x3060FFFF (/aura's "blue"), where
   it was 0xD6F2FFFF: vanilla draws a held item in its record's colour (aura::hand: the Scimitar of Fury's glow in
   its hue, the rest lightly tinted).
2. Icingdeath's aura: itemaura kind 3 (particles) of the spell visual 0x6C151A60, the ice dripping off a caster's
   hands (the named aura "freezing"), at the blade's tip (nodes 0: vanilla's aura::tip), in blue 0x3060FFFF.
3. Twinkle's aura: the druid's epic glow, itemaura kind 7 (epic look) of 0xCBF4135A, the Oak Staff of Eilerawdl (the
   druid's level 49 epic; the named aura "druid staff glow", the one the owner called "the druid epic aura" on
   Zerkziee's scimitar): the staff's amber emissive, its gold sheen and its green sparkles at 45% along the blade
   (nodes 45, as the named look), in the staff's own colours (colour 0). The druid's level 60 epic scimitar, the Triad
   Scimitar (0xD03942DE), carries an emissive glow alone: no sheen, no particles.

An itemaura row on a held item's record shows on whichever hand holds it (entity.pieceAuras, kinds 3/7/9: a server
with shield-energy d5d3f9a5 and freezing-aura f4e59632 on, a vanilla with freezing-tip 49ad319 on); a PS2 and an
older client draw the plain scimitar.

The records are found by name (exactly one Twinkle and one Icingdeath, one-handed slashing), not by id: the ids may
differ between shards. Checked as found, else REFUSED and nothing written: those two records; a record of the Scimitar
of Fury's model and icon on the shard; tables itempattern, itemaura and gm_audit. Report by default; --apply in one
transaction, a gm_audit line a row (as the panel's), logged in drizzt_weapons_1116_log; a second --apply finds nothing
to do; --undo puts back exactly what was there (a row changed since left and listed, the rest still put back).

After --apply: the panel's "Reload every NPC" (or an Item auras edit) shows the auras at the next tick; the new model,
icon and colour reach a wielder at the next login (eject to apply now), since a character's items are sent then.

    python3 -E -s tools/drizzt_weapons_20261116.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "drizzt_weapons_1116_log"
DETAIL = "Twinkle and Icingdeath in the ranger heirloom scimitar's look, Icingdeath blue and freezing, Twinkle the druid epic glow (the owner, 2026-11-16); tools/drizzt_weapons_20261116.py"
WHO, VIA = "dab", "tool"

FURY_MODEL, FURY_ICON = 0xAB6B7D29, 1086674104  # Heirloom Scimitar of Fury 714027 / Scimitar of Fury 701006
PLAIN, BLUE = 0xFFFFFFFF, 0x3060FFFF
STAFF = 0xCBF4135A  # Oak Staff of Eilerawdl, ITEM.ESF Druid_Staff
FREEZING = 0x6C151A60  # the spell visual: ice dripping off a caster's hands

# name -> (the record's colour, its itemaura row)
WANT = {
    "Twinkle": (PLAIN, {"color": 0, "kind": 7, "source": STAFF, "nodes": 45,
                        "note": "Twinkle: the druid's epic glow (Oak Staff of Eilerawdl, epic look) on the ranger heirloom scimitar, the owner 2026-11-16"}),
    "Icingdeath": (BLUE, {"color": BLUE, "kind": 3, "source": FREEZING, "nodes": 0,
                          "note": "Icingdeath: the freezing aura (spell visual 6C151A60's ice drip at the tip), blue, the owner 2026-11-16"}),
}

KEYS = {"itempattern": ("patternid",), "itemaura": ("patternid",)}

LOG_DDL = f"""CREATE TABLE {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    rowkey TEXT NOT NULL,      -- the key (JSON)
    before TEXT,               -- the row before (JSON), NULL none
    after  TEXT NOT NULL,      -- the row written (JSON)
    audit  INTEGER NOT NULL
)"""


def signed(v):
    return v - (1 << 32) if v >= 1 << 31 else v


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


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
    """tbl's row of key made want (a dict of the columns to set; None: deleted; on itempattern only the named columns
    change); (before, after, audit id)."""
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


def plan(db):
    """(writes [(tbl, key, want)], lines, refusals)."""
    writes, lines, refusals = [], [], []
    for t in ("itempattern", "itemaura", "gm_audit"):
        if not table(db, t):
            refusals.append(f"no {t} table here")
    if refusals:
        return writes, lines, refusals
    if not db.execute("SELECT 1 FROM itempattern WHERE (model & 0xFFFFFFFF) = ? AND itemicon = ?", (FURY_MODEL, FURY_ICON)).fetchone():
        refusals.append(f"no record of the Scimitar of Fury's look (model 0x{FURY_MODEL:08X}, icon {FURY_ICON}) on this shard")
    for name, (color, aura) in WANT.items():
        found = db.execute("SELECT patternid, equipslot, attacktype, model, itemicon, color FROM itempattern WHERE itemname = ?", (name,)).fetchall()
        if len(found) != 1:
            refusals.append(f"{len(found)} itempattern rows named {name!r}, not one")
            continue
        pid, slot, attack, model, icon, was = found[0]
        if slot not in (12, 14) or attack != 1:
            refusals.append(f"itempattern {pid} {name!r} is wear slot {slot}, attack type {attack}: not a one-handed slashing weapon")
            continue
        look = {"itemicon": FURY_ICON, "model": signed(FURY_MODEL), "color": color}
        if (icon, model & 0xFFFFFFFF, was & 0xFFFFFFFF) != (FURY_ICON, FURY_MODEL, color):
            writes.append(("itempattern", {"patternid": pid}, look))
            lines.append(f"  itempattern {pid} {name}: model 0x{model & 0xFFFFFFFF:08X} icon {icon} colour 0x{was & 0xFFFFFFFF:08X}"
                         f" -> model 0x{FURY_MODEL:08X} icon {FURY_ICON} colour 0x{color:08X}")
        cur = row(db, "itemaura", {"patternid": pid})
        if cur is None or {k: cur[k] for k in aura} != aura:
            writes.append(("itemaura", {"patternid": pid}, dict(aura)))
            had = "none" if cur is None else f"kind {cur['kind']} 0x{cur['color'] & 0xFFFFFFFF:08X} of 0x{cur['source'] & 0xFFFFFFFF:08X} (kept in the log)"
            lines.append(f"  itemaura {pid} {name}: {had} -> kind {aura['kind']} colour 0x{aura['color']:08X} of 0x{aura['source']:08X}, nodes {aura['nodes']}")
        held = db.execute("SELECT c.charName, i.equiploc FROM charinventory i JOIN characters c ON c.serverid = i.serverid "
                          "WHERE i.patternid = ?", (pid,)).fetchall() if table(db, "charinventory") and table(db, "characters") else []
        lines.append(f"  {name} ({pid}) held by " + (", ".join(f"{n} (equiploc {e})" for n, e in held) or "no one"))
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
                  ("; press the GM panel's Reload every NPC; the looks at the wielder's next login" if back else ""))
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
        print(f"applied: {n} rows, in gm_audit and {LOG}; press the GM panel's Reload every NPC for the auras; "
              "the new model, icon and colour at the wielder's next login (eject to apply now)")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
