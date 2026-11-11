#!/usr/bin/env python3
"""The Testing Heroes Vendor's second stock (handlers/heroes.go, entity/wornshield.go), the GM Jibrish's petitions of
2026-11-10, on a server that has tools/seed_testing_heroes.py's rows:

  1516 + 1517: the Seraphic Planar Breastplate in three versions, "weakened -50% stats including proc", regular and
    Ascended, at 1250 / 1500 / 2000, then "upgrade the costs on seraphic, all 3 of them, by 750 each": 2000 / 2250 /
    2750. The breastplate is record 8527 (the Plane of Sky's drop; 990013 is its import's damped copy, itemimported,
    and so no base). Its weakened one is a record of its own, 965004 "Weakened Seraphic Planar Breastplate"; its
    Ascended one is the server's where itemascended has one, else made here by the Ascended records' rule
    (tools/ascend_boss_loot.py's SQL, database/ascended.go's: each figure a tenth more) with its itemascended row.
  1520: "The weakened version of both breastplates should be converted to all/all for class and race. However, they
    need to be 50% as effective now on proc and stats. The weakened bpoa increase its cost by 100": the two weakened
    records' classuse 32767 and raceuse 1023 (every class, every race: database/item.go reads 1023 as every race,
    the custom ones too), and the figures an Ascended record scales (ascend_boss_loot.SCALED: the seven stats, HP and
    power and their ticks, AC, the six resists, a weapon's damage) half their base's, in whole numbers, half up
    (WEAKENED_PERCENT, entity.WeakenedPercent; a figure of 0 or below as it is). Their hit effect is the server's: a
    record named "Weakened <name>" has <name>'s at half (entity/wornshield.go, WornShieldOf). The weakened Breastplate
    of the Anointed's stock row 250 -> 350, and both rows' and records' words say so.
  1524: "Add a CV Robe, ceremonial vestment, to Testing Heroes Vendor. It should costs 4000 faction, be all/all and give
    10 to all stats including resists": a copy of the Ceremonial Vestments (5192, the drop; 990091 its import's copy),
    965005 "Heroic Ceremonial Vestments", its look, slot, level and the rest its base's, every class and race, and 10
    more of each of the seven stats and the six resists than its base has (STR, STA, AGI, WIS, DEX, CHA, INT, PR, DR,
    FR, CR, LR, AR; its HP, power and AC as its base's). 4000 is a standing of +2000, the most a standing holds
    (entity.FactionMax): affordable only at the top.

Refused, nothing written, where a row is not as tools/seed_testing_heroes.py wrote it (the weakened Breastplate's
record not a copy of 1076's figures, its stock row not at 250), where 8527 or 5192 is not the record named, is itself a
copy, or where an id or a name it makes is taken. Run again after an apply, it finds every row as it wrote it and
writes nothing.

WHEN IT TAKES EFFECT: heroes_stock and the records are read at every talk and every give: the wares are on sale once
it is applied. The weakened Seraphic's hit effect and the weakened Breastplate's heal at half are the server's
(build with entity.WeakenedPercent 50 and the "Weakened " name rule): under an older binary the weakened Seraphic has
NO hit effect and the weakened Breastplate heals at 70%. Run it as the before.sh of the keep restart that brings that
code (tools/swap_keep.sh). A new Ascended record is known to the server within a minute (entity.LoadAscendedItems) and
made its base again by the rule at each start (database.SyncAscendedCopies).

Report only by default; --apply writes it in one transaction, every row with its gm_audit line and its line in
heroes_more_1111_log; --undo puts every row back as it was (the made ones taken away, the changed ones restored, the
ids handed out given back when no row took one since), each with its gm_audit line, and drops the log: all or nothing.
An undo is refused while a row is not as written (a GM's edit since: each said), while a character holds, lists or
can buy back a record made here, or once one of these wares has been sold since the apply.

    python3 -E -s tools/heroes_more_20261111.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import ascend_boss_loot as asc  # noqa: E402  the Ascended records' rule

WHO, VIA = "dab's sessions", "tool"
DETAIL = ("petitions 1516, 1517, 1520, 1524 (Jibrish): the Seraphic Planar Breastplate three ways, the weakened ones "
          "all/all at half, the Heroic Ceremonial Vestments; tools/heroes_more_20261111.py")
SRC = "testing-heroes"      # heroes_stock.source, as the seed's rows
LOG = "heroes_more_1111_log"
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,      -- the table, or sqlite_sequence / heroes_purchases for the marks kept
    rowkey  TEXT NOT NULL,      -- the row's key (JSON)
    before  TEXT,               -- the row before (JSON); NULL: made here
    after   TEXT,               -- the row as written (JSON)
    audit   INTEGER             -- the gm_audit line written
)"""
KEYS = {"itempattern": ("patternid",), "itemascended": ("patternid",), "heroes_stock": ("patternid",)}
SEQS = ("heroes_stock",)

EVERY_CLASS, EVERY_RACE = 32767, 1023
WEAKENED_PERCENT = 50        # entity.WeakenedPercent
WEAKENED_PREFIX = "Weakened "  # entity.WeakenedPrefix
STATS = ("str", "sta", "agi", "wis", "dex", "cha", "intelligence")
RESISTS = ("pr", "dr", "fr", "cr", "lr", "ar")
CV_MORE = 10

BPOA, BPOA_NAME, WEAK_BPOA = 1076, "Breastplate of the Anointed", 965003
SERAPHIC, SERAPHIC_NAME, WEAK_SERAPHIC = 8527, "Seraphic Planar Breastplate", 965004
CV, CV_NAME, HERO_CV = 5192, "Ceremonial Vestments", 965005
WEAK_BPOA_NAME = WEAKENED_PREFIX + BPOA_NAME
WEAK_SERAPHIC_NAME = WEAKENED_PREFIX + SERAPHIC_NAME
HERO_CV_NAME = "Heroic " + CV_NAME
OLD_WEAK_BPOA_COST, WEAK_BPOA_COST = 250, 350
COSTS = {"weak": 2000, "regular": 2250, "ascended": 2750, "cv": 4000}

# A box draws no percent sign (CLIENT shows "(?)"): the words say percent.
DESC = {
    WEAK_BPOA: "A Breastplate of the Anointed whose blessing has thinned: any class or race may wear it, at half its stats and half its heal at each hit.",
    WEAK_SERAPHIC: "A Seraphic Planar Breastplate whose power has thinned: any class or race may wear it, at half its stats and half its answer to each hit.",
    HERO_CV: "Ceremonial vestments blessed by the Testing Heroes: any class or race may wear them, and they give 10 more of every stat and every resist.",
}
SAYS = {
    WEAK_BPOA: "The Breastplate of the Anointed, weakened: anyone may wear it, at half its stats and half its heal at each hit you take.",
    WEAK_SERAPHIC: "The Seraphic Planar Breastplate, weakened: anyone may wear it, at half its stats and half its damage and heal at each hit you take.",
    SERAPHIC: "The Seraphic Planar Breastplate, whole: a shadow knight's. It hurts what hits you and heals you, more with your intelligence.",
    "ascended": "The Seraphic Planar Breastplate, Ascended: a shadow knight's, its stats and its answer to each hit a tenth more.",
    HERO_CV: "Ceremonial Vestments for anyone: 10 more of every stat and every resist than the robe has. Yours at the very top of your standing.",
}


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def columns(db, tbl):
    return [r[1] for r in db.execute(f"PRAGMA table_info({tbl})")]


def row_of(db, tbl, key):
    where = " AND ".join(f"{c} = ?" for c in KEYS[tbl])
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", [key[c] for c in KEYS[tbl]])
    cols = [d[0] for d in cur.description]
    r = cur.fetchone()
    return None if r is None else dict(zip(cols, r))


def go_json(m):
    return json.dumps(m, sort_keys=True, separators=(",", ":"), default=str)


def js(m):
    return json.dumps(m, sort_keys=True, default=str)


def col(rec, low):
    """The record's column of that lower-case name (itempattern spells some in capitals)."""
    return next(c for c in rec if c.lower() == low)


def half(v):
    """entity.WornShield.weakened's figure: above 0 at WEAKENED_PERCENT, half up in whole numbers; 0 or below as it is."""
    if v is None or v <= 0:
        return v
    return (v * WEAKENED_PERCENT + 50) // 100


def weakened_of(base, pid, name, desc):
    """A weakened record of base: every column its base's but the id, name, words, classes and races, and the scaled
    figures at half."""
    rec = dict(base)
    for c in rec:
        low = c.lower()
        if low in asc.SCALED:
            rec[c] = half(base[c])
    rec[col(rec, "patternid")] = pid
    rec[col(rec, "itemname")] = name
    rec[col(rec, "itemdesc")] = desc
    rec[col(rec, "classuse")] = EVERY_CLASS
    rec[col(rec, "raceuse")] = EVERY_RACE
    return rec


def cv_of(base):
    rec = dict(base)
    for c in rec:
        if c.lower() in STATS + RESISTS:
            rec[c] = (base[c] or 0) + CV_MORE
    rec[col(rec, "patternid")] = HERO_CV
    rec[col(rec, "itemname")] = HERO_CV_NAME
    rec[col(rec, "itemdesc")] = DESC[HERO_CV]
    rec[col(rec, "classuse")] = EVERY_CLASS
    rec[col(rec, "raceuse")] = EVERY_RACE
    return rec


def is_copy(db, pid):
    return [t for t in ("itemascended", "itemimported")
            if table(db, t) and db.execute(f"SELECT 1 FROM {t} WHERE patternid = ?", (pid,)).fetchone()]


def base_record(db, pid, name, refusals):
    rec = row_of(db, "itempattern", {"patternid": pid})
    if rec is None or (rec[col(rec, "itemname")] or "").strip() != name:
        refusals.append(f"itempattern {pid} is {rec and rec[col(rec, 'itemname')]!r}, not {name!r}")
        return None
    for t in is_copy(db, pid):
        refusals.append(f"itempattern {pid} is a copy ({t})")
        return None
    return rec


def same(a, b):
    return a is not None and js(a) == js(b)


def plan(db):
    """(steps, lines, refusals). A step is (tbl, key, before, after): before None a row made, after the row as it is to
    be; ("ascend", None, None, base) the Ascended record made by the server's own SQL. A row already as it is to be is
    no step (said in lines)."""
    steps, lines, refusals = [], [], []
    need = ("itempattern", "heroes_stock", "factions", "gm_audit")
    missing = [t for t in need if not table(db, t)]
    if missing:
        return [], [], [f"no table {', '.join(missing)}: not a server with tools/seed_testing_heroes.py's rows"]
    if not db.execute("SELECT 1 FROM factions WHERE name = 'Testing Heroes'").fetchone():
        refusals.append("factions has no 'Testing Heroes': tools/seed_testing_heroes.py first")

    def want(tbl, key, after, what):
        now = row_of(db, tbl, key)
        if same(now, after):
            lines.append(f"{what}: as it is to be already")
            return
        steps.append((tbl, key, now, after))
        lines.append(f"{what}: {'made' if now is None else 'changed'}")

    # 1520: the weakened Breastplate of the Anointed, all/all at half
    bpoa = base_record(db, BPOA, BPOA_NAME, refusals)
    weak = row_of(db, "itempattern", {"patternid": WEAK_BPOA})
    if bpoa is not None:
        new = weakened_of(bpoa, WEAK_BPOA, WEAK_BPOA_NAME, DESC[WEAK_BPOA])
        seeded = dict(bpoa)
        seeded[col(seeded, "patternid")], seeded[col(seeded, "itemname")] = WEAK_BPOA, WEAK_BPOA_NAME
        if weak is None:
            refusals.append(f"itempattern {WEAK_BPOA} is not there: tools/seed_testing_heroes.py first")
        elif not same(weak, new):
            # as the seed wrote it: its base's every column but the name and the words
            w = {k: v for k, v in weak.items() if k.lower() != "itemdesc"}
            s = {k: v for k, v in seeded.items() if k.lower() != "itemdesc"}
            if js(w) != js(s):
                diff = sorted(k for k in w if w.get(k) != s.get(k))
                refusals.append(f"itempattern {WEAK_BPOA} is not the seed's copy of {BPOA} (differs in {', '.join(diff)})")
            else:
                want("itempattern", {"patternid": WEAK_BPOA}, new, f"itempattern {WEAK_BPOA} {WEAK_BPOA_NAME}: all/all, "
                     + figures(bpoa, new))
        else:
            lines.append(f"itempattern {WEAK_BPOA}: as it is to be already")
    stock_bpoa = row_of(db, "heroes_stock", {"patternid": WEAK_BPOA})
    if stock_bpoa is None:
        refusals.append(f"heroes_stock does not sell {WEAK_BPOA}")
    else:
        new = dict(stock_bpoa, cost=WEAK_BPOA_COST, says=SAYS[WEAK_BPOA])
        if not same(stock_bpoa, new) and stock_bpoa["cost"] != OLD_WEAK_BPOA_COST:
            refusals.append(f"heroes_stock sells {WEAK_BPOA} at {stock_bpoa['cost']}, not {OLD_WEAK_BPOA_COST}")
        else:
            want("heroes_stock", {"patternid": WEAK_BPOA}, new, f"heroes_stock {WEAK_BPOA}: {OLD_WEAK_BPOA_COST} -> {WEAK_BPOA_COST}")

    # 1516/1517: the Seraphic Planar Breastplate three ways
    ser = base_record(db, SERAPHIC, SERAPHIC_NAME, refusals)
    made = {WEAK_SERAPHIC: WEAK_SERAPHIC_NAME, HERO_CV: HERO_CV_NAME}
    logged = set(made) if table(db, LOG) else set()
    for pid, name in made.items():
        other = db.execute("SELECT patternid FROM itempattern WHERE lower(trim(itemname)) = lower(?) AND patternid != ?", (name, pid)).fetchone()
        if other:
            refusals.append(f"a record named {name!r} is there already ({other[0]})")
        if pid not in logged and db.execute("SELECT 1 FROM itempattern WHERE patternid = ?", (pid,)).fetchone():
            refusals.append(f"itempattern {pid} is taken")
    if ser is not None:
        new = weakened_of(ser, WEAK_SERAPHIC, WEAK_SERAPHIC_NAME, DESC[WEAK_SERAPHIC])
        want("itempattern", {"patternid": WEAK_SERAPHIC}, new, f"itempattern {WEAK_SERAPHIC} {WEAK_SERAPHIC_NAME}: a copy of "
             f"{SERAPHIC}, all/all, " + figures(ser, new))
    asc_id = None
    if table(db, "itemascended"):
        r = db.execute("SELECT patternid FROM itemascended WHERE base = ?", (SERAPHIC,)).fetchone()
        asc_id = r[0] if r else None
    if asc_id is not None:
        lines.append(f"the Ascended {SERAPHIC_NAME} is record {asc_id}: sold as it is")
    elif ser is not None:
        if not asc.is_gear({k.lower(): v for k, v in ser.items()}):
            refusals.append(f"itempattern {SERAPHIC} is not gear an Ascended record is made of")
        other = db.execute("SELECT patternid FROM itempattern WHERE lower(trim(itemname)) = lower(?)", (asc.ascended_name(SERAPHIC_NAME),)).fetchone()
        if other:
            refusals.append(f"a record named {asc.ascended_name(SERAPHIC_NAME)!r} is there already ({other[0]}) and no itemascended row")
        steps.append(("ascend", None, None, SERAPHIC))
        lines.append(f"an Ascended record of {SERAPHIC} is made (the Ascended records' rule), with its itemascended row")

    # 1524: the Heroic Ceremonial Vestments
    cv = base_record(db, CV, CV_NAME, refusals)
    if cv is not None:
        new = cv_of(cv)
        want("itempattern", {"patternid": HERO_CV}, new, f"itempattern {HERO_CV} {HERO_CV_NAME}: a copy of {CV}, all/all, "
             + figures(cv, new))

    # the stock
    wares = [(WEAK_SERAPHIC, COSTS["weak"], SAYS[WEAK_SERAPHIC]), (SERAPHIC, COSTS["regular"], SAYS[SERAPHIC]),
             (asc_id if asc_id is not None else "ascended", COSTS["ascended"], SAYS["ascended"]), (HERO_CV, COSTS["cv"], SAYS[HERO_CV])]
    for pid, cost, says in wares:
        if pid == "ascended":
            steps.append(("heroes_stock", {"patternid": pid}, None, {"patternid": pid, "cost": cost, "says": says, "source": SRC}))
            lines.append(f"heroes_stock: the new Ascended record at {cost}")
            continue
        now = row_of(db, "heroes_stock", {"patternid": pid})
        if now is not None:
            if now["cost"] == cost and now["says"] == says and now["source"] == SRC:
                lines.append(f"heroes_stock {pid}: at {cost} already")
            else:
                refusals.append(f"heroes_stock sells {pid} already, at {now['cost']}")
            continue
        steps.append(("heroes_stock", {"patternid": pid}, None, {"patternid": pid, "cost": cost, "says": says, "source": SRC}))
        lines.append(f"heroes_stock {pid}: at {cost}")
    return steps, lines, refusals


def figures(base, new):
    out = [f"{asc.FIGURE_NAMES.get(c.lower(), c)} {base[c]}->{new[c]}" for c in base
           if base[c] != new[c] and c.lower() not in ("patternid", "itemname", "itemdesc", "classuse", "raceuse")]
    return ", ".join(out) or "no figure changed"


def audit(db, action, tbl, key, before, after, detail):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, 0, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, WHO, VIA, action, tbl, go_json(key), None if before is None else go_json(before),
                      None if after is None else go_json(after), detail))
    return cur.lastrowid


def seqs(db):
    if not table(db, "sqlite_sequence"):
        return {}
    return {t: (lambda r: None if r is None else r[0])(db.execute("SELECT seq FROM sqlite_sequence WHERE name = ?", (t,)).fetchone())
            for t in SEQS}


def logged(db, tbl, key, before):
    row = row_of(db, tbl, key)
    a = audit(db, "insert" if before is None else "update", tbl, key, before, row, DETAIL)
    db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
               (tbl, json.dumps(key), None if before is None else js(before), js(row), a))


def last_purchase(db):
    if not table(db, "heroes_purchases"):
        return 0
    return db.execute("SELECT COALESCE(MAX(id), 0) FROM heroes_purchases").fetchone()[0]


def apply(db, steps):
    """The plan written, in the caller's transaction. Returns how many rows."""
    db.execute(LOG_SCHEMA)
    before_seq = seqs(db)
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    asc_id, n = None, 0
    for tbl, key, before, row in steps:
        if tbl == "ascend":
            db.execute(asc.SCHEMA)
            sets, froms, _ = asc.copy_columns(columns(db, "itempattern"))
            asc_id = db.execute("SELECT COALESCE(MAX(patternid), ?) + 1 FROM itempattern WHERE patternid BETWEEN ? AND ?",
                                (asc.FIRST_ID - 1, asc.FIRST_ID, asc.LAST_ID)).fetchone()[0]
            if asc_id > asc.LAST_ID:
                raise SystemExit(f"no id left for an Ascended record ({asc.FIRST_ID}-{asc.LAST_ID})")
            db.execute(f'INSERT INTO itempattern ("patternid", {sets}) SELECT ?, {froms} FROM itempattern WHERE patternid = ?', (asc_id, row))
            logged(db, "itempattern", {"patternid": asc_id}, None)
            db.execute("INSERT INTO itemascended (patternid, base, made, source) VALUES (?, ?, ?, ?)",
                       (asc_id, row, at, "tools/heroes_more_20261111.py"))
            logged(db, "itemascended", {"patternid": asc_id}, None)
            n += 2
            continue
        row = dict(row)
        if tbl == "heroes_stock" and row["patternid"] == "ascended":
            row["patternid"] = asc_id
            key = {"patternid": asc_id}
        if before is None:
            cols = list(row)
            db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [row[c] for c in cols])
        else:
            sets = [c for c in row if c not in KEYS[tbl] and row[c] != before.get(c)]
            where = " AND ".join(f"{c} = ?" for c in KEYS[tbl])
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{chr(34)}{c}{chr(34)} = ?' for c in sets)} WHERE {where}",
                       [row[c] for c in sets] + [key[c] for c in KEYS[tbl]])
        logged(db, tbl, key, before)
        n += 1
    db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES ('sqlite_sequence', '{{}}', ?, ?)",
               (json.dumps(before_seq), json.dumps(seqs(db))))
    db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES ('heroes_purchases', '{{}}', ?, NULL)",
               (json.dumps(last_purchase(db)),))
    return n


def made_ids(db):
    return [json.loads(k)["patternid"] for (k,) in db.execute(f"SELECT rowkey FROM {LOG} WHERE tbl = 'itempattern' AND before IS NULL")]


def held(db):
    """Why an undo may not be: who holds, lists or may buy back a record made here, what was sold since."""
    why = []
    made = made_ids(db)
    q = ", ".join("?" * len(made))
    for t, extra in (("charinventory", ""), ("auction_lots", " AND state = 'open'"), ("charbuyback", "")):
        if made and table(db, t) and "patternid" in columns(db, t):
            for pid, n in db.execute(f"SELECT patternid, count(*) FROM {t} WHERE patternid IN ({q}){extra} GROUP BY patternid", made):
                why.append(f"{n} of record {pid} in {t}")
    mark = db.execute(f"SELECT before FROM {LOG} WHERE tbl = 'heroes_purchases'").fetchone()
    stock = [json.loads(k)["patternid"] for (k,) in db.execute(f"SELECT rowkey FROM {LOG} WHERE tbl = 'heroes_stock'")]
    if mark is not None and stock and table(db, "heroes_purchases"):
        s = ", ".join("?" * len(stock))
        n = db.execute(f"SELECT count(*) FROM heroes_purchases WHERE id > ? AND patternid IN ({s})", [json.loads(mark[0])] + stock).fetchone()[0]
        if n:
            why.append(f"{n} purchases of these wares since the apply (heroes_purchases)")
    return why


def changed(db):
    out = []
    for tbl, key, after in db.execute(f"SELECT tbl, rowkey, after FROM {LOG} WHERE tbl IN ({', '.join(repr(t) for t in KEYS)}) ORDER BY seq").fetchall():
        now = row_of(db, tbl, json.loads(key)) if table(db, tbl) else None
        if now is None or js(now) != after:
            out.append(f"{tbl} {key} changed since it was written")
    return out


def undo(db):
    back = 0
    for tbl, key, before in db.execute(f"SELECT tbl, rowkey, before FROM {LOG} WHERE tbl IN ({', '.join(repr(t) for t in KEYS)}) ORDER BY seq DESC").fetchall():
        key = json.loads(key)
        now = row_of(db, tbl, key)
        where = " AND ".join(f"{c} = ?" for c in KEYS[tbl])
        if before is None:
            db.execute(f"DELETE FROM {tbl} WHERE {where}", [key[c] for c in KEYS[tbl]])
            audit(db, "delete", tbl, key, now, None, "undo: " + DETAIL)
        else:
            was = json.loads(before)
            sets = [c for c in was if c not in KEYS[tbl] and was[c] != now.get(c)]
            if sets:
                db.execute(f"UPDATE {tbl} SET {', '.join(f'{chr(34)}{c}{chr(34)} = ?' for c in sets)} WHERE {where}",
                           [was[c] for c in sets] + [key[c] for c in KEYS[tbl]])
            audit(db, "update", tbl, key, now, was, "undo: " + DETAIL)
        back += 1
    seq_row = db.execute(f"SELECT before, after FROM {LOG} WHERE tbl = 'sqlite_sequence'").fetchone()
    if seq_row is not None:
        was, written = json.loads(seq_row[0]), json.loads(seq_row[1])
        cur = seqs(db)
        for t in SEQS:
            if cur.get(t) is None or cur.get(t) != written.get(t):
                continue
            if was.get(t) is None:
                db.execute("DELETE FROM sqlite_sequence WHERE name = ?", (t,))
            else:
                db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = ?", (was[t], t))
    db.execute(f"DROP TABLE {LOG}")
    return back


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"no database at {a.db}")
    db = sqlite3.connect(a.db, isolation_level=None)
    try:
        if a.undo:
            if not table(db, LOG):
                sys.exit(f"nothing to undo: no {LOG}")
            why = changed(db) + held(db)
            if why:
                for w in why:
                    print("refused:", w)
                sys.exit("undo refused: nothing done")
            db.execute("BEGIN IMMEDIATE")
            n = undo(db)
            db.execute("COMMIT")
            print(f"undone: {n} rows put back")
            return
        steps, lines, refusals = plan(db)
        for line in lines:
            print(line)
        if refusals:
            for r in refusals:
                print("refused:", r)
            sys.exit("refused: nothing written")
        if not steps:
            print("nothing to do: every row is as it is to be")
            return
        if not a.apply:
            print(f"report only: {len(steps)} steps; --apply writes them")
            return
        if table(db, LOG):
            sys.exit(f"{LOG} is there already and rows are not as it wrote them: --undo first, or look")
        db.execute("BEGIN IMMEDIATE")
        try:
            n = apply(db, steps)
        except BaseException:
            db.execute("ROLLBACK")
            raise
        db.execute("COMMIT")
        print(f"applied: {n} rows")
    finally:
        db.close()


if __name__ == "__main__":
    main()
