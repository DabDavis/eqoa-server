#!/usr/bin/env python3
"""Drizzt's scale mail: six records he alone may wear, given into his pack (the owner, 2026-11-16: "give drizzt a set
of the plate 9 armor that he alone can wear drizzt only", then "lets replace drizzt armoe with scale mail").

1. The records, itempattern 715043-715048 (beside his Twinkle, Icingdeath and Taulmaril, 715040-715042): a coif,
   tunic, bracers, gloves, leggings and boots, model 7 (CHAR.ESF's scale mail, every slot, the helm too: helm set 7)
   in the disc art's own colours (colour 0xFFFFFFFF, untinted; a dyer still dyes them), the crafted scale sets' icons
   (the Adamantite ... Seralite Scale Coif/Tunic/...). Ranger only (classuse 2), level 60, NO TRADE (trade 1, the
   server's trade refuses it), kept over logout (rent 1), LORE (one each), worth nothing (no sale for profit).
   Their figures beat, field by field, what he wears in each slot now (the report prints both): the Gravemold Tunic
   (STR 15, STA 15, AGI 75, DEX 10, INT 80) and his heirloom bracers, gauntlets, greaves and boots (as a level 60
   ranger sees them, capped at 49: +10 every stat, 40 HP and power, the bracers AC 232); no helm is worn (his bank's
   Glacierplate Helm: AC 100, HP 50, STA 15, WIS 15, FR 63, CR 15).
2. Only he may wear them: table itemowner (database/itemowner.go, server branch drizzt-plate) binds each record to his
   serverid; the server refuses anyone else's equip of a copy ("This item can only be worn by the one it was made
   for."), a merc's and another raid seat's Need. His account (98) has another level 60 ranger (Destiny) who shares
   the account bank, so the class mask and NO TRADE alone would not do. A server WITHOUT that branch reads no
   itemowner: the class mask and NO TRADE still hold.
3. --give: one of each into his pack (location 0, unequipped, at the end of the list, as the GM panel's give does
   offline), only those he does not hold already; nothing taken away. Run it only while he is OFFLINE (the rows reach
   him at his next login); while he is online give the six by the GM panel's give (by patternid), which sends them
   at once.

The character is found by name (Drizzt) and must be a level 60 dark elf ranger, else REFUSED and nothing written (the
Live shard's Drizzt is a level 1 barbarian rogue: refused there). Checked too: the ids free (or already exactly
these records), tables itempattern, characters, charinventory and gm_audit, and with --give his pack's room. Report by
default; --apply in one transaction, a gm_audit line a row (as the panel's), logged in drizzt_armor_1116_log; a second
--apply finds nothing to do; --undo puts back exactly what was there (a row changed since is left and listed; a
record still held by anyone is left, and its binding with it). --apply may run again with --give after one without.

    python3 -E -s tools/drizzt_armor_20261116.py --db eqoa.db [--give] [--apply | --undo] [--pack-slots 100]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "drizzt_armor_1116_log"
DETAIL = "Drizzt's scale mail, his alone (the owner, 2026-11-16); tools/drizzt_armor_20261116.py"
WHO, VIA = "dab", "tool"
NAME, CLASS, RACE, LEVEL = "Drizzt", 1, 2, 60  # ranger, dark elf
SCALE, PLAIN = 7, 0xFFFFFFFF
DESC = "Scale mail made for Drizzt Do'Urden alone; it will sit on no other shoulders."

OWNER_DDL = """CREATE TABLE IF NOT EXISTS itemowner (
		patternid INTEGER PRIMARY KEY,
		serverid  INTEGER NOT NULL,
		note      TEXT NOT NULL DEFAULT ''
	)"""  # database/itemowner.go's schema


def stats(ac, hp, pw, s=15, st=15, ag=15, dx=15, wi=15, it=15, ch=15, pr=15, dr=15, fr=15, cr=15, lr=15, ar=15):
    return {"AC": ac, "HPMAX": hp, "POWMAX": pw, "str": s, "sta": st, "agi": ag, "dex": dx, "wis": wi,
            "intelligence": it, "cha": ch, "PR": pr, "DR": dr, "FR": fr, "CR": cr, "LR": lr, "AR": ar}


# patternid -> (name, wear slot, icon, figures)
PIECES = {
    715043: ("Drizzt's Scale Mail Coif", 1, 1087965268, stats(148, 125, 50, fr=63)),
    715044: ("Drizzt's Scale Mail Tunic", 5, -427326871, stats(474, 200, 50, s=20, st=20, ag=75, dx=20, it=80)),
    715045: ("Drizzt's Scale Mail Bracers", 7, -393670558, stats(252, 150, 50)),
    715046: ("Drizzt's Scale Mail Gloves", 19, -1591038372, stats(252, 150, 50)),
    715047: ("Drizzt's Scale Mail Leggings", 10, 1189600244, stats(326, 150, 50)),
    715048: ("Drizzt's Scale Mail Boots", 11, -215262289, stats(178, 150, 50)),
}
DURABILITY = 12000


def record(pid):
    name, wear, icon, figs = PIECES[pid]
    r = {"itemcost": 0, "unk1": 0, "itemicon": icon, "unk2": 0, "equipslot": wear, "unk3": 0, "trade": 1, "rent": 1,
         "unk4": 0, "attacktype": 0, "weapondamage": 0, "unk5": 0, "levelreq": LEVEL, "maxstack": 1,
         "maxhp": DURABILITY, "duration": 65, "classuse": 1 << CLASS, "raceuse": 1023, "procanim": 0, "lore": 1,
         "unk6": 0, "craft": 0, "itemname": name, "itemdesc": DESC, "model": SCALE, "color": PLAIN,
         "PoT": 0, "HoT": 0, "weaponproc": 0, "fish": 0}
    r.update(figs)
    return r


FIG = ["AC", "HPMAX", "POWMAX", "str", "sta", "agi", "dex", "wis", "intelligence", "cha", "PR", "DR", "FR", "CR", "LR", "AR"]
# a heirloom's figures as a level 60 wearer sees them (handlers/heirloom.go: capped at 49, +10 every stat, 40 HP and
# power; the bracers' AC its role's best's): what the table's own columns leave blank
HEIRLOOM_VIEW = {714016: dict(stats(232, 40, 40, *([10] * 7), *([0] * 6))),
                 714017: dict(stats(0, 40, 40, *([10] * 7), *([0] * 6))),
                 714018: dict(stats(0, 40, 40, *([10] * 7), *([0] * 6))),
                 714019: dict(stats(0, 40, 40, *([10] * 7), *([0] * 6)))}

LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    rowkey TEXT NOT NULL,      -- the key (JSON)
    before TEXT,               -- the row before (JSON), NULL none
    after  TEXT NOT NULL,      -- the row written (JSON)
    audit  INTEGER NOT NULL
)"""


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
    """tbl's row of key made want (None: deleted); (before, after, audit id). A key of None inserts a new row (its
    autoincrement id the key)."""
    if key is None:
        cur = db.execute(f"INSERT INTO {tbl} ({', '.join(want)}) VALUES ({', '.join('?' * len(want))})", tuple(want.values()))
        key = {"itemid": cur.lastrowid}
        before, action = None, "insert"
    else:
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
    return key, before, after, audit(db, tbl, key, action, before, after)


def figures(db, pid):
    if pid in HEIRLOOM_VIEW:
        return HEIRLOOM_VIEW[pid]
    r = row(db, "itempattern", {"patternid": pid})
    return {k: r[k] or 0 for k in FIG}


def fig_line(f):
    return " ".join(f"{k} {f[k]}" for k in FIG if f[k])


def plan(db, give, pack_slots):
    """(writes [(tbl, key, want)], lines, refusals)."""
    writes, lines, refusals = [], [], []
    for t in ("itempattern", "characters", "charinventory", "gm_audit"):
        if not table(db, t):
            refusals.append(f"no {t} table here")
    if refusals:
        return writes, lines, refusals
    found = db.execute("SELECT serverid, tclass, race, level FROM characters WHERE charName = ?", (NAME,)).fetchall()
    if len(found) != 1:
        return writes, lines, [f"{len(found)} characters named {NAME}, not one"]
    sid, cls, race, level = found[0]
    if (cls, race, level) != (CLASS, RACE, LEVEL):
        return writes, lines, [f"{NAME} (serverid {sid}) is class {cls}, race {race}, level {level}: not the level 60 dark elf ranger"]
    lines.append(f"  {NAME}: serverid {sid}, a level {level} dark elf ranger")

    worn = {r[0]: (r[1], r[2]) for r in db.execute(
        "SELECT p.equipslot, p.patternid, p.itemname FROM charinventory i JOIN itempattern p USING (patternid) "
        "WHERE i.serverid = ? AND i.location = 0 AND i.equiploc >= 0", (sid,))}
    owner_rows = table(db, "itemowner")
    if not owner_rows:
        writes.append(("ddl", None, OWNER_DDL))
        lines.append("  table itemowner made (database/itemowner.go's schema; the drizzt-plate server reads it)")
    for pid, (name, wear, _, figs) in PIECES.items():
        want = record(pid)
        cur = row(db, "itempattern", {"patternid": pid})
        if cur is not None and cur["itemname"] != name:
            refusals.append(f"itempattern {pid} is {cur['itemname']!r}, not free")
            continue
        if cur is None or {k: cur[k] for k in want} != want:
            writes.append(("itempattern", {"patternid": pid}, want))
            lines.append(f"  itempattern {pid} {name}: " + ("new" if cur is None else "made as below") +
                         f"; wear {wear}, model {SCALE} scale, ranger only, level {LEVEL}, NO TRADE, LORE")
        lines.append(f"      new: {fig_line(figs)}")
        if wear in worn:
            wpid, wname = worn[wear]
            have = figures(db, wpid)
            worse = [k for k in FIG if figs[k] < have[k]]
            lines.append(f"      now: {wname} ({wpid}): {fig_line(have)}" + (f"  WORSE IN {worse}" if worse else ""))
            if worse:
                refusals.append(f"{name} is worse than his {wname} in {worse}")
        else:
            lines.append("      now: nothing worn there")
        bind = {"serverid": sid, "note": f"{name}: {NAME}'s alone, the owner 2026-11-16"}
        b = row(db, "itemowner", {"patternid": pid}) if owner_rows else None
        if b is None or b["serverid"] != sid:
            if b is not None:
                refusals.append(f"itemowner {pid} binds it to serverid {b['serverid']}, not {sid}")
                continue
            writes.append(("itemowner", {"patternid": pid}, bind))
            lines.append(f"  itemowner {pid}: worn by serverid {sid} ({NAME}) alone")

    if give:
        held = {r[0] for r in db.execute("SELECT patternid FROM charinventory WHERE serverid = ?", (sid,))}
        new = [pid for pid in PIECES if pid not in held]
        n = db.execute("SELECT COUNT(*) FROM charinventory WHERE serverid = ? AND location = 0", (sid,)).fetchone()[0]
        if n + len(new) > pack_slots:
            refusals.append(f"his pack holds {n} of {pack_slots}: no room for {len(new)} more")
        nxt = db.execute("SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory WHERE serverid = ? AND location = 0",
                         (sid,)).fetchone()[0]
        for pid in new:
            writes.append(("charinventory", None, {"serverid": sid, "stackleft": 1, "remainHP": DURABILITY,
                                                    "remaincharge": 0, "patternid": pid, "equiploc": -1,
                                                    "location": 0, "listnumber": nxt}))
            lines.append(f"  charinventory: {PIECES[pid][0]} into his pack, list number {nxt}")
            nxt += 1
        lines.append(f"  his pack: {n} -> {n + len(new)} of {pack_slots}" +
                     (f"; held already: {sorted(set(PIECES) - set(new))}" if len(new) < len(PIECES) else ""))
    return writes, lines, refusals


def apply(db, writes):
    db.execute(LOG_DDL)
    for tbl, key, want in writes:
        if tbl == "ddl":
            db.execute(want)
            n = audit(db, "itemowner", {}, "create", None, {"sql": want})
            db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES ('ddl', '{{}}', NULL, ?, ?)", (want, n))
            continue
        key, before, after, n = put(db, tbl, key, want)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                   (tbl, go_json(key), go_json(before), go_json(after), n))
    return len(writes)


def undo(db):
    """Every logged write put back, last first. A record (and its binding) still held by anyone is left, as is a row
    changed since."""
    back, left = 0, []
    if not table(db, LOG):
        return back, left
    for seq, tbl, rowkey, before, after in db.execute(f"SELECT seq, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        if tbl == "ddl":
            if db.execute("SELECT COUNT(*) FROM itemowner").fetchone()[0]:
                left.append("table itemowner: holds rows, left")
                continue
            db.execute("DROP TABLE itemowner")
            audit(db, "itemowner", {}, "drop", {"sql": after}, None)
            db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
            back += 1
            continue
        key = json.loads(rowkey)
        if tbl in ("itempattern", "itemowner") and before is None:
            held = db.execute("SELECT COUNT(*) FROM charinventory WHERE patternid = ?", (key["patternid"],)).fetchone()[0]
            if held:
                left.append(f"{tbl} {rowkey}: {held} copies still held, left")
                continue
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
    ap.add_argument("--give", action="store_true", help="one of each into his pack (only while he is offline)")
    ap.add_argument("--pack-slots", type=int, default=100, help="world.pack_slots (eqoaclassic: 100)")
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
            print(f"undone: {back} rows back" + "".join(f"\n  left: {w}" for w in left))
            return 0 if not left else 1
        writes, lines, refusals = plan(db, a.give, a.pack_slots)
        if refusals:
            db.execute("ROLLBACK")
            print("\n".join(lines))
            print("REFUSED, nothing written:" + "".join(f"\n  {r}" for r in refusals))
            return 1
        print("\n".join(lines))
        if not writes:
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"(report only: {len(writes)} rows; --apply to write)")
            return 0
        n = apply(db, writes)
        db.execute("COMMIT")
        print(f"applied: {n} rows, in gm_audit and {LOG}" +
              ("; the pack rows reach him at his next login" if a.give else
               "; give the six by the GM panel's give (patternids 715043-715048)"))
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
