#!/usr/bin/env python3
"""The Plane of Sky's bosses' loot on Live, where the shard sync left it empty (Live petition 5511, 2026-11-23: "PoS
rares and bosses are not dropping items").

Live keeps Ascended loot off on purpose (tools/live_sync_20261122.py kept Live's held-drop bars and the loot rows that
keep it off), and three of the Plane's bosses had nothing else:

  - Ahriaana the Ultimate: her table "epic ahriaana" holds sixteen Ascended pieces on Heirloom (the epic audit of
    2026-11-30's set, tools/ascended_epics_*), none on Live: she dropped nothing of her own. Live's table is given the
    sixteen pieces they were made from (itemascended.base on Heirloom; the same ids and names on Live), weight 1, as
    Heirloom's.
  - Veeshan's Talon and Veeshan's Bulwark (the sky twins, work 1343): on Heirloom their gear is the lootcreature
    profile "sky twins ascended"'s category of that name (the owner-approved 30% one piece, 70% two, a pool of 32
    Ascended pieces) beside its trash, gems, modifiers and sky parts. Live has the profile but none of its rates and
    none of its pool: the Talon rolled nothing but its scales and gem, the Bulwark only its older "sky twins" table
    (three of 25, every kill). Live is given the profile's rates as Heirloom's, its gear category as "sky twins" (the
    32 pieces the Ascended ones were made from, region "plane of sky"), and the Bulwark's old table set to 0 as on
    Heirloom. No rate is new: each is Heirloom's.

Every row is found by name and checked; anything else REFUSES, nothing written. Loot is read at each corpse: no
restart is needed, but Live has no panel token, so this runs in the before.sh of the next keep restart.

Dry run by default. --apply writes in one transaction, a gm_audit line a row (account 1, dab, via tool), each row's
before and after in sky_loot_live_1123_log; a second --apply writes nothing. --undo puts back what the log holds.

    python3 -E -s tools/sky_loot_live_20261123.py --db eqoa.db --shard live [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "sky_loot_live_1123_log"
WHO, ACCOUNT, VIA = "dab", 1, "tool"
TOOL = "tools/sky_loot_live_20261123.py"
SRC = "Live petition 5511 (sky_loot_live_20261123)"
DETAIL = f"Live petition 5511: the Plane of Sky's bosses' non-Ascended loot on Live: {TOOL}"

# Ahriaana's sixteen: the pieces Heirloom's Ascended ones were made from (id, name)
AHRIAANA = "ahriaana the ultimate"
AHRIAANA_TABLE = "epic ahriaana"
AHRIAANA_ITEMS = [(1031, "bulwark greaves"), (4968, "fever robe"), (4985, "laboratory coat"),
                  (719127, "Thaumaturgical Bracers"), (719128, "Virtuoso Bracers"), (719129, "Palladia Gloves"),
                  (719130, "Ensorceling Leggings"), (719131, "Maven's Bracers"), (719132, "Guru Bracers"),
                  (719133, "Stygian Gloves"), (719134, "Bastion Tunic"), (719135, "Runic Mithril Boots"),
                  (719136, "Silent Chain Tunic"), (719137, "Wretched Boots"), (719138, "Medium Tunic"),
                  (719139, "Theurgist's Boots")]
# the twins' 32: the pieces of Heirloom's "sky twins ascended" pool
TWINS = ("veeshan's talon", "veeshan's bulwark")
PROFILE = "sky twins ascended"
GEAR = "sky twins"
REGION = "plane of sky"
TWIN_ITEMS = [(716016, "vaporous legwrap"), (716017, "aeriform shield"), (1047, "sublime belt"), (716018, "astral sai"),
              (960030, "Pneumatic Gloves"), (960062, "Pneumatic Staff"), (6465, "pneumatic tunic"),
              (716019, "hallowed boots"), (716020, "belt of untold miracles"), (960048, "Sublime Gloves"),
              (960049, "Sublime Armguard"), (716021, "hallowed helm"), (4913, "aeriform shroud"),
              (716022, "Seraphic Planar Slippers"), (716023, "Seraphic Planar Leggings"), (716024, "empyreal orb"),
              (716025, "Seraphic Planar Sabatons"), (716026, "Seraphic Planar Shield"),
              (716027, "Seraphic Planar Trousers"), (716028, "Empyreal Hat"), (5088, "seraphic planar shroud"),
              (716029, "Seraphic Planar Girdle"), (960010, "Seraphic Planar Bow"), (716030, "Seraphic Planar Dagger"),
              (716031, "Seraphic Planar Earring"), (5087, "seraphic planar robe"), (8527, "Seraphic Planar Chestplate"),
              (6088, "transcendental wall"), (716032, "Seraphic Planar Orb"), (716033, "Astral Chain Leggings"),
              (716034, "Transcendental Boots"), (716035, "Vaporous Shoes")]
# Heirloom's rates for the profile, preset by preset: (category, rate, counts, minstack, maxstack, position, note)
APPROVED = "User-approved 30% one gear item, 70% two, 0% three. Independent equal-weight draws. Quest and junk separate."
RATES = {
    "jib": [("trash", 0.0, "25,31,25,12,5,1.5,0.5", 1, 1, 0, ""), ("gems", 0.2, "", 1, 1, 1, ""),
            ("modifiers", 0.03, "", 1, 1, 2, ""), ("sky parts", 0.1, "", 1, 1, 9, "the Plane of Sky's (pos_armor.py)"),
            (GEAR, 0.0, "0,30,70", 1, 1, 20, APPROVED)],
    "user": [("trash", 0.5, "", 1, 3, 0, ""), ("gems", 0.05, "", 1, 1, 1, ""), ("modifiers", 0.03, "", 1, 1, 2, ""),
             ("sky parts", 0.1, "", 1, 1, 9, "the Plane of Sky's (pos_armor.py)"),
             (GEAR, 0.0, "0,30,70", 1, 1, 20, APPROVED)],
}

LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    rid    INTEGER NOT NULL,
    rowkey TEXT NOT NULL,
    before TEXT,
    after  TEXT,
    audit  INTEGER NOT NULL,
    at     TEXT NOT NULL
)"""


class Refused(Exception):
    pass


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def dumps(v):
    return json.dumps(v, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def rows(db, tbl, where):
    w = " AND ".join(f'"{c}" = ?' for c in where) or "1"
    cur = db.execute(f'SELECT rowid AS _rowid_, * FROM "{tbl}" WHERE {w} ORDER BY rowid', tuple(where.values()))
    names = [d[0] for d in cur.description]
    return [dict(zip(names, r)) for r in cur.fetchall()]


def plain(r):
    return None if r is None else {k: v for k, v in r.items() if k != "_rowid_"}


def same(a, b):
    if isinstance(a, (int, float)) and isinstance(b, (int, float)) and not isinstance(a, bool):
        return abs(float(a) - float(b)) < 1e-6
    return a == b


def num(v):
    return int(v) if float(v).is_integer() else v


class Plan:
    def __init__(self, db):
        self.db = db
        self.ops = []  # (tbl, rid or None, before or None, after values)
        self.lines = []

    def say(self, s):
        self.lines.append(f"  {s}")

    def add(self, tbl, values, match, what):
        have = rows(self.db, tbl, {c: values[c] for c in match})
        if have:
            if len(have) == 1 and all(same(have[0].get(c), v) for c, v in values.items()):
                self.say(f"{what}: already there")
                return
            raise Refused(f"{what}: {tbl} has {dumps([plain(h) for h in have])[:300]}, not {dumps(values)[:300]}")
        self.ops.append((tbl, None, None, dict(values)))
        self.say(f"{what}: insert {tbl}")

    def change(self, tbl, row, was, now_, what):
        """Set now_'s columns on row, which must read was (or now_ already)."""
        if all(same(row.get(c), v) for c, v in now_.items()):
            self.say(f"{what}: already so")
            return
        for c, v in was.items():
            if not same(row.get(c), v):
                raise Refused(f"{what}: {c} reads {row.get(c)!r}, not {v!r}")
        self.ops.append((tbl, row["_rowid_"], plain(row), dict(now_)))
        self.say(f"{what}: {dumps({c: row.get(c) for c in now_})[:160]} -> {dumps(now_)[:160]}")



def build(p, shard):
    db = p.db
    if shard != "live":
        raise Refused("Live only: Heirloom's bosses drop their Ascended loot (nothing to do there)")
    for t in ("itempattern", "loottables", "npcloot", "lootpool", "lootrate", "lootpreset", "lootcreature", "gm_audit"):
        if not table(db, t):
            raise Refused(f"no table {t}")

    def item(pid, name):
        r = db.execute("SELECT itemname FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
        if not r or r[0] != name:
            raise Refused(f"item {pid} reads {r[0] if r else None!r}, not {name!r}")
        if name.lower().startswith("ascended"):
            raise Refused(f"item {pid} is Ascended")

    # Ahriaana
    if not rows(db, "npcloot", {"npc_name": AHRIAANA, "tablename": AHRIAANA_TABLE}):
        raise Refused(f"{AHRIAANA} has no table {AHRIAANA_TABLE!r}")
    have = {r["patternid"] for r in rows(db, "loottables", {"tablename": AHRIAANA_TABLE})}
    if have - {pid for pid, _ in AHRIAANA_ITEMS}:
        raise Refused(f"{AHRIAANA_TABLE!r} holds other items: {sorted(have)}")
    for pid, name in AHRIAANA_ITEMS:
        item(pid, name)
        p.add("loottables", {"tablename": AHRIAANA_TABLE, "patternid": pid, "weight": 1, "minlevel": 1,
                             "maxlevel": 100, "source": SRC}, ("tablename", "patternid"),
              f"{AHRIAANA_TABLE!r}: {name}")

    # the twins
    for n in TWINS:
        lc = rows(db, "lootcreature", {"npc_name": n})
        if len(lc) != 1 or lc[0]["profile"] != PROFILE or lc[0]["region"] != REGION:
            raise Refused(f"{n}: lootcreature {[plain(r) for r in lc]}, not profile {PROFILE!r} of {REGION!r}")
    for pid, name in TWIN_ITEMS:
        item(pid, name)
        p.add("lootpool", {"category": GEAR, "region": REGION, "npc_name": "", "patternid": pid, "weight": 1,
                           "minlevel": 1, "maxlevel": 100, "source": SRC}, ("category", "region", "patternid"),
              f"pool {GEAR!r}: {name}")
    presets = {r["name"] for r in rows(db, "lootpreset", {})}
    for preset, rates in RATES.items():
        if preset not in presets:
            raise Refused(f"no loot preset {preset!r}")
        for cat, rate, counts, mn, mx, pos, note in rates:
            p.add("lootrate", {"preset": preset, "profile": PROFILE, "category": cat, "rate": rate, "counts": counts,
                               "minstack": mn, "maxstack": mx, "position": pos, "note": note, "source": SRC},
                  ("preset", "profile", "category"), f"rate {preset}/{PROFILE}/{cat}")
    bw = rows(db, "npcloot", {"npc_name": "veeshan's bulwark", "tablename": GEAR})
    if len(bw) != 1:
        raise Refused(f"veeshan's bulwark's {GEAR!r} table: {len(bw)} rows")
    p.change("npcloot", bw[0], {"chance": 100}, {"chance": 0},
             f"veeshan's bulwark's old {GEAR!r} table (its gear the profile's now, as Heirloom's)")


def keyof(db, tbl, row, rid):
    pk = sorted((r[5], r[1]) for r in db.execute(f'PRAGMA table_info("{tbl}")') if r[5])
    return {c: row[c] for _, c in pk} if pk else {"rowid": rid}


def audit(db, action, tbl, key, before, after):
    return db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                      "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                      (now(), ACCOUNT, WHO, VIA, action, tbl, dumps(key), None if before is None else dumps(before),
                       None if after is None else dumps(after), DETAIL)).lastrowid


def by_rowid(db, tbl, rid):
    r = rows(db, tbl, {"rowid": rid})
    return plain(r[0]) if r else None


def apply(db, p):
    db.execute(LOG_DDL)
    at = now()
    for tbl, rid, before, new in p.ops:
        if rid is None:
            cs = list(new)
            rid = db.execute(f'INSERT INTO "{tbl}" ({", ".join(chr(34) + c + chr(34) for c in cs)}) '
                             f'VALUES ({", ".join("?" * len(cs))})', [new[c] for c in cs]).lastrowid
            action = "insert"
        else:
            db.execute(f'UPDATE "{tbl}" SET {", ".join(chr(34) + c + chr(34) + " = ?" for c in new)} WHERE rowid = ?',
                       [*new.values(), rid])
            action = "update"
        after = by_rowid(db, tbl, rid)
        key = keyof(db, tbl, after, rid)
        a = audit(db, action, tbl, key, before, after)
        db.execute(f"INSERT INTO {LOG} (tbl, rid, rowkey, before, after, audit, at) VALUES (?, ?, ?, ?, ?, ?, ?)",
                   (tbl, rid, dumps(key), None if before is None else dumps(before), dumps(after), a, at))
    return len(p.ops)


def undo(db):
    if not table(db, LOG):
        return 0, []
    back, left = 0, []
    for seq, tbl, rid, rowkey, before, after in db.execute(
            f"SELECT seq, tbl, rid, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        have = by_rowid(db, tbl, rid)
        if have is None or dumps(have) != after:
            left.append(f"{tbl} {rowkey}: changed since, left as it is")
            continue
        b = None if before is None else json.loads(before)
        if b is None:
            db.execute(f'DELETE FROM "{tbl}" WHERE rowid = ?', (rid,))
            action = "delete"
        else:
            db.execute(f'UPDATE "{tbl}" SET {", ".join(chr(34) + c + chr(34) + " = ?" for c in b)} WHERE rowid = ?',
                       [*b.values(), rid])
            action = "update"
        audit(db, action, tbl, json.loads(rowkey), have, b)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if not db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0]:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--shard", required=True, choices=("heirloom", "live"))
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("PRAGMA busy_timeout = 5000")
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"undone: {back} rows put back" + "".join(f"\n  left: {w}" for w in left))
            return 0
        p = Plan(db)
        try:
            build(p, a.shard)
        except Refused as e:
            db.execute("ROLLBACK")
            print("\n".join(p.lines))
            print(f"REFUSED, nothing written: {e}")
            return 1
        print(f"{a.shard}:")
        print("\n".join(p.lines))
        print(f"totals: {len(p.ops)} rows to write")
        if not a.apply or not p.ops:
            db.execute("ROLLBACK")
            print("dry run: nothing written (--apply to write)" if not a.apply else "nothing to write: all in place")
            return 0
        n = apply(db, p)
        db.execute("COMMIT")
        print(f"applied: {n} rows, a gm_audit line each, logged in {LOG}")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
