#!/usr/bin/env python3
"""The broken item records (the user, 2026-11-01: "lets fix all these"):
level 60 records of one junk template, every stat a figure no item of the
game's has (PoT 987, LR 927; DMG 999 on the weapons), the old server's test
items (EQOA-Data holds the same template under the same names: "Rediculously
Rare Robe, most would kill to have it"), copied by tools/import_quests.py
into the three priests' epic staves (60.json's items are "like" one of
them). Players hold copies (every holder a GM's or an admin's test
character, from the old server's default inventory), #template handed them
to GMs and one loot table dropped one. A record of the template is one whose
PoT or a resist is past 500 (entity/balancekit.go's simBroken, which keeps
the balance kits off them and stays). Each takes, the first of these that
has it:

  1. A record of ours of its name (as a player reads it, case, apostrophes
     and a word's plural s aside) that is none of the template's: its
     figures (FIGURES: level, class, race, the stats, AC, damage, item HP,
     durability, trade, rent, lore, and its wear slot). Ceremonial Vestments
     takes the level 29 robe's of its name; Wind Golem Gloves and Boots the
     Wind Golem set's gloves and boots (level 58), whose names theirs
     misspell (the boots worn on the shield's arm until then: the feet).
  2. EQOA-Data's record of its name (Information/databases/items/*.json)
     that is none of the template's: the priests' epic staves (Staff of
     Divinity, of Nature's Blessing, of Spiritual Energies): its stats, its
     damage, its class (the cleric's, the druid's, the shaman's), lore and no
     trade, as it gives them; a figure one lacks is its kin's, the records
     taken here of its attack type and level (Divinity's damage the others'
     928, ours, the user's decision of 2026-11-01; a durability the others'),
     and item HP none of them gives the weapons' rule
     (tools/fill_weapon_durability.py: its level's commonest among the
     server's real weapons; ours).
  3. Else its name and wear slot kept, the template lost (ours, the user's
     decision of 2026-11-01): no stats; a weapon the median damage of the
     server's real level 60 weapons of its attack type, armour the median AC
     of its slot's real level 60 items (the median of an even count the mean
     of the middle two, a half up); item HP its slot's rule
     (tools/fill_weapon_durability.py, fill_wear_durability.py: the level's
     commonest where three records carry one, else 200 a level) and
     durability 50. Khal Sword, Sapphire's Sword, Khal Healer Robe, Staff of
     the Jade Forest, Healer gloves, Healer boots: no record of any of them
     anywhere but the template's.

A held copy (charinventory) at more hit points than its record's new item HP
is made whole at it: a weapon's condition is its hit points over its item HP
(handlers/character.go's WeaponCond, uncapped), so a sword held at the
template's 18500 would hit the harder. A loot table's row of a record left
with no real record (3.) is taken out: the Khal Warlord's Khal Sword
(EQOA-Data's junk record's "dropped by the Khal Warlord's"), which
tools/seed_camp_loot.py no longer seeds. The old server's default inventory
(table defaultcharinventory, read by no code) is left.

The plan needs EQOA-Data, which the server's box lacks: --save-plan FILE
here, --plan FILE --apply there (each record changed only where it is still
as planned; the held copies and the loot rows read where the database is).
--apply writes it all in one transaction, every row logged in
broken_records_fix_log; --undo puts back each row still as it wrote it (one
changed since is left, and said). Both take a copy of the database first
(--no-backup to skip). The server reads item records as they are used: no
restart (a player sees a record's new figures at the next login).

    tools/fix_broken_records.py --eqoa-data ../external/EQOA-Data [--db eqoa.db] [--save-plan FILE]
    tools/fix_broken_records.py --db eqoa.db --plan FILE --apply
    tools/fix_broken_records.py --db eqoa.db --undo
"""
import argparse
import collections
import datetime
import glob
import json
import os
import re
import sqlite3
import statistics
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
DEFAULT_DB = os.path.join(HERE, "..", "eqoa.db")
LOG = "broken_records_fix_log"

BROKEN_OVER = 500          # simBroken's: a PoT or a resist past it
BROKEN_COLS = ("PoT", "PR", "DR", "FR", "CR", "LR", "AR")
STATS = ("str", "sta", "agi", "wis", "dex", "cha", "intelligence", "HPMAX", "POWMAX", "PoT", "HoT", "AC",
         "PR", "DR", "FR", "CR", "LR", "AR")
# what a record of ours of the name gives (1.)
FIGURES = ("levelreq", "classuse", "raceuse", *STATS, "weapondamage", "attacktype", "maxhp", "duration", "trade",
           "rent", "lore", "equipslot")
# EQOA-Data's stats by their names there
DATA_STATS = {"str": "str", "sta": "sta", "agi": "agi", "wis": "wis", "dex": "dex", "cha": "cha", "int": "intelligence",
              "hp": "HPMAX", "pow": "POWMAX", "pot": "PoT", "hot": "HoT", "ac": "AC", "pr": "PR", "dr": "DR", "fr": "FR",
              "cr": "CR", "lr": "LR", "ar": "AR"}
CLASSES = {"WAR": 0, "RAN": 1, "PAL": 2, "SK": 3, "MNK": 4, "BRD": 5, "ROG": 6, "DRU": 7, "SHM": 8, "CLR": 9,
           "MAG": 10, "NEC": 11, "ENC": 12, "WIZ": 13, "ALC": 14}
ALL_CLASSES = (1 << len(CLASSES)) - 1
WEAPON_SLOTS = (12, 13, 14, 15, 16)    # fill_weapon_durability.py's
DURATION, MIN_SAMPLES = 50, 3          # the durability tools' rule


def has_table(db, t):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone()[0] > 0


def columns(db, t):
    return [r[1] for r in db.execute(f"PRAGMA table_info({t})")]


def dumps(o):
    return json.dumps(o, sort_keys=True, separators=(",", ":"))


def fold(name):
    """A name as a player reads it: case and apostrophes aside, and a word's
    plural s ("Wind Golems Gloves", "Wind Golem's boots")."""
    n = str(name or "").lower().replace("'", "").replace("`", "").replace("’", "")
    return " ".join(w[:-1] if len(w) > 3 and w.endswith("s") else w for w in re.findall(r"[a-z0-9]+", n))


def broken(r):
    return any((r.get(c) or 0) > BROKEN_OVER for c in BROKEN_COLS)


def data_broken(it):
    st = it.get("stats") or {}
    return any((st.get(k) or 0) > BROKEN_OVER for k in ("pot", "pr", "dr", "fr", "cr", "lr", "ar")) or (it.get("damage") or 0) >= 999


def is_weapon(r):
    return r["equipslot"] in WEAPON_SLOTS and (r["weapondamage"] or 0) > 0 and not (r["equipslot"] == 13 and not r["attacktype"])


def records(db):
    cols = columns(db, "itempattern")
    return [dict(zip(cols, r)) for r in db.execute("SELECT * FROM itempattern ORDER BY patternid")]


def median(vals):
    """The median, an even count's the mean of the middle two, a half up."""
    m = statistics.median(vals)
    return int(m + 0.5)


def data_items(root):
    """EQOA-Data's item records by folded name, the template's left out."""
    out = collections.defaultdict(list)
    for path in sorted(glob.glob(os.path.join(root, "Information", "databases", "items", "*.json"))):
        with open(path, encoding="utf-8") as f:
            d = json.load(f)
        for it in d["items"] if isinstance(d, dict) else d:
            if isinstance(it, dict) and it.get("name") and not data_broken(it):
                out[fold(it["name"])].append(dict(it, _file=os.path.basename(path)))
    return out


def max_hp_rules(rows, bad):
    """The durability tools' item HP rules over the real records: weapons by
    level, the rest by slot and level, the commonest where three carry one."""
    weapons, worn = collections.defaultdict(collections.Counter), collections.defaultdict(collections.Counter)
    for r in rows:
        if r["patternid"] in bad or not r["maxhp"] or r["maxhp"] <= 0:
            continue
        if is_weapon(r):
            weapons[r["levelreq"]][r["maxhp"]] += 1
        elif r["equipslot"] is not None and r["equipslot"] >= 0:
            worn[(r["equipslot"], r["levelreq"])][r["maxhp"]] += 1

    def pick(c):
        if not c:
            return None
        hp, n = c.most_common(1)[0]
        return hp if n >= MIN_SAMPLES else None
    return (lambda level: pick(weapons.get(level)) or 200 * max(level or 0, 1),
            lambda slot, level: pick(worn.get((slot, level))) or 200 * max(level or 0, 1))


def plan(db, eqoa_data):
    """What --apply would write: each broken record's new figures, the rows
    it reads them from, and the report's lists."""
    rows = records(db)
    bad = {r["patternid"] for r in rows if broken(r)}
    real = [r for r in rows if r["patternid"] not in bad]
    ours = collections.defaultdict(list)
    for r in real:
        ours[fold(r["itemname"])].append(r)
    data = data_items(eqoa_data) if eqoa_data else {}
    weapon_hp, worn_hp = max_hp_rules(rows, bad)
    lists = collections.defaultdict(list)
    out = []
    # the EQOA-Data records taken, for a figure one lacks (its kin's)
    taken = {}
    for r in rows:
        if r["patternid"] in bad and not ours.get(fold(r["itemname"])) and data.get(fold(r["itemname"])):
            taken[r["patternid"]] = data[fold(r["itemname"])][0]
    for r in rows:
        if r["patternid"] not in bad:
            continue
        pid, name = r["patternid"], r["itemname"]
        kin = sorted(ours.get(fold(name), []), key=lambda k: (k["equipslot"] != r["equipslot"], k["patternid"]))
        if kin:
            k = kin[0]
            want = {c: k[c] for c in FIGURES}
            how = f"ours: {k['patternid']} {k['itemname']} (level {k['levelreq']})"
            if k["equipslot"] != r["equipslot"]:
                lists["a record taking its kin's wear slot"].append(f"{pid} {name}: {r['equipslot']} -> {k['equipslot']}")
        elif pid in taken:
            it = taken[pid]
            want, how = from_data(r, it, taken, weapon_hp, lists)
        else:
            want, how = stripped(r, real, weapon_hp, worn_hp, lists)
        before = {c: r[c] for c in want}
        set_ = {c: v for c, v in want.items() if before[c] != v}
        out.append(dict(patternid=pid, name=name, how=how, before={c: before[c] for c in set_}, set=set_,
                        strip=how.startswith("no record")))
        lists["the records"].append(f"{pid} {name}: {how}")
    # what the report says of who holds them and what drops them
    for pid in sorted(bad):
        n = db.execute("SELECT count(*), count(DISTINCT serverid) FROM charinventory WHERE patternid = ?", (pid,)).fetchone() \
            if has_table(db, "charinventory") else (0, 0)
        if n[0]:
            lists["held copies (made whole at the new item HP where above it)"].append(f"{pid}: {n[0]} by {n[1]} characters")
        if has_table(db, "defaultcharinventory"):
            m = db.execute("SELECT count(*) FROM defaultcharinventory WHERE patternid = ?", (pid,)).fetchone()[0]
            if m:
                lists["the old default inventory's rows (read by no code: left)"].append(f"{pid}: {m}")
        if has_table(db, "loottables"):
            for lid, t in db.execute("SELECT id, tablename FROM loottables WHERE patternid = ?", (pid,)):
                strip = any(c["patternid"] == pid and c["strip"] for c in out)
                lists["loot rows of a record with no real record (taken out)" if strip else "loot rows of a fixed record (kept)"].append(
                    f"{lid} {t}: {pid}")
    return dict(changes=out, lists={k: v for k, v in lists.items()})


def from_data(r, it, taken, weapon_hp, lists):
    """A record's figures out of EQOA-Data's record of its name (2.)."""
    st = it.get("stats") or {}
    want = {col: int(st.get(k) or 0) for k, col in DATA_STATS.items()}
    req = it.get("requirements") or {}
    if isinstance(req.get("level"), int):
        want["levelreq"] = req["level"]
    cls = [c for c in req.get("class") or [] if c != "ALL"]
    want["classuse"] = sum(1 << CLASSES[c] for c in cls if c in CLASSES) if cls else ALL_CLASSES
    want["lore"] = 1 if it.get("lore") else 0
    want["trade"] = 1 if it.get("noTrade") else 0
    # its kin: the other records taken here of its level and attack type (a
    # record that gives none is of any)
    def kin_of(k):
        a, b = k.get("attackType"), it.get("attackType")
        return (k.get("requirements") or {}).get("level") == req.get("level") and (a is None or b is None or a == b)
    kin = [k for p, k in taken.items() if p != r["patternid"] and kin_of(k)]
    lacks = []
    for key, col in (("damage", "weapondamage"), ("durability", "duration")):
        if isinstance(it.get(key), int):
            want[col] = it[key]
            continue
        vals = [k[key] for k in kin if isinstance(k.get(key), int)]
        if vals:
            want[col] = median(vals)
            lacks.append(f"{col} its kin's {want[col]}")
        elif col == "duration":
            want[col] = DURATION
            lacks.append(f"durability {DURATION} (the durability tools')")
    if isinstance(it.get("itemHp"), int):
        want["maxhp"] = it["itemHp"]
    else:
        want["maxhp"] = weapon_hp(want.get("levelreq", r["levelreq"]))
        lacks.append(f"item HP {want['maxhp']} (the weapons' rule)")
    if lacks:
        lists["figures EQOA-Data's record lacks (ours)"].append(f"{r['patternid']} {r['itemname']}: {'; '.join(lacks)}")
    return want, f"EQOA-Data's {it['_file']} {it['name']!r}"


def stripped(r, real, weapon_hp, worn_hp, lists):
    """A record of no real record: the template lost (3.)."""
    want = {c: 0 for c in STATS}
    lvl = r["levelreq"]
    if is_weapon(r):
        dmg = [k["weapondamage"] for k in real if k["levelreq"] == lvl and is_weapon(k) and k["attacktype"] == r["attacktype"]]
        if not dmg:
            lists["a weapon with no real weapon of its attack type and level (damage left)"].append(f"{r['patternid']} {r['itemname']}")
        else:
            want["weapondamage"] = median(dmg)
        want["maxhp"] = weapon_hp(lvl)
        what = f"damage {want.get('weapondamage', r['weapondamage'])}, the median of {len(dmg)} real level {lvl} weapons of attack type {r['attacktype']}"
    else:
        acs = [(k["AC"] or 0) for k in real if k["levelreq"] == lvl and k["equipslot"] == r["equipslot"]]
        if acs:
            want["AC"] = median(acs)
        want["maxhp"] = worn_hp(r["equipslot"], lvl)
        what = f"AC {want['AC']}, the median of {len(acs)} real level {lvl} items of wear slot {r['equipslot']}"
    want["duration"] = DURATION
    return want, f"no record but the template's: no stats, {what}, item HP {want['maxhp']}, durability {DURATION} (ours)"


# ---- apply and undo -----------------------------------------------------------------

LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY,
    run    TEXT NOT NULL,     -- the run's time (UTC): --undo takes the last run back
    tbl    TEXT NOT NULL,
    ref    TEXT NOT NULL,     -- the row's key columns (JSON)
    action TEXT NOT NULL,     -- update | delete
    before TEXT,              -- the columns as they were (JSON; a deleted row whole, its rowid too)
    after  TEXT,              -- the columns as written (JSON); a delete: none
    note   TEXT NOT NULL DEFAULT ''
)"""


def log(db, run, tbl, ref, action, before, after, note=""):
    db.execute(f"INSERT INTO {LOG} (run, tbl, ref, action, before, after, note) VALUES (?, ?, ?, ?, ?, ?, ?)",
               (run, tbl, dumps(ref), action, None if before is None else dumps(before), None if after is None else dumps(after),
                note or ""))


def row_now(db, tbl, ref, cols):
    w = " AND ".join(f"{c} = ?" for c in ref)
    r = db.execute(f"SELECT {', '.join(cols)} FROM {tbl} WHERE {w}", list(ref.values())).fetchone()
    return None if r is None else dict(zip(cols, r))


def apply(db, p, when=None):
    """The plan's records still as planned, their held copies and loot rows
    as the database has them now, in one transaction, each row logged;
    returns (records, held copies, loot rows, stale)."""
    when = when or datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")
    made_log = not has_table(db, LOG)
    recs = held = loot = stale = 0
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_DDL)
        last = db.execute(f"SELECT max(run) FROM {LOG}").fetchone()[0]
        if last is not None and when <= last:
            raise SystemExit(f"the last run was applied at {last}, this one at {when}: run it again in a second")
        for c in p["changes"]:
            ref = {"patternid": c["patternid"]}
            name = row_now(db, "itempattern", ref, ["itemname"])
            if name is None or name["itemname"] != c["name"]:
                stale += 1
                continue
            if c["set"] and row_now(db, "itempattern", ref, list(c["set"])) == c["set"]:
                pass  # as planned already (a run again)
            elif c["set"]:
                if row_now(db, "itempattern", ref, list(c["before"])) != c["before"]:
                    stale += 1
                    continue
                db.execute(f"UPDATE itempattern SET {', '.join(f'{k} = ?' for k in c['set'])} WHERE patternid = ?",
                           [*c["set"].values(), c["patternid"]])
                log(db, when, "itempattern", ref, "update", c["before"], c["set"], f"{c['name']}: {c['how']}")
                recs += 1
            # its held copies made whole at its item HP where above it
            hp = db.execute("SELECT maxhp FROM itempattern WHERE patternid = ?", (c["patternid"],)).fetchone()[0]
            if has_table(db, "charinventory") and hp and hp > 0:
                for iid, now in db.execute("SELECT itemid, remainHP FROM charinventory WHERE patternid = ? AND remainHP > ? "
                                           "ORDER BY itemid", (c["patternid"], hp)).fetchall():
                    db.execute("UPDATE charinventory SET remainHP = ? WHERE itemid = ?", (hp, iid))
                    log(db, when, "charinventory", {"itemid": iid}, "update", {"remainHP": now}, {"remainHP": hp},
                        f"a held {c['name']} made whole at its item HP")
                    held += 1
            # a record of no real record drops off no one's corpse
            if c["strip"] and has_table(db, "loottables"):
                cols = columns(db, "loottables")
                for r in db.execute(f"SELECT rowid, {', '.join(cols)} FROM loottables WHERE patternid = ? ORDER BY rowid",
                                    (c["patternid"],)).fetchall():
                    row = dict(zip(cols, r[1:]))
                    db.execute("DELETE FROM loottables WHERE rowid = ?", (r[0],))
                    log(db, when, "loottables", {"id": row["id"]}, "delete", dict(row, rowid_=r[0]), None,
                        f"{c['name']}: a record of no real record drops off no one")
                    loot += 1
        if not (recs or held or loot) and made_log:
            db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return recs, held, loot, stale


def undo(db):
    """Takes the last run back: (put back, left changed since, the run)."""
    if not has_table(db, LOG):
        sys.exit("no run to undo here")
    back, left = 0, []
    db.execute("BEGIN IMMEDIATE")
    try:
        run = db.execute(f"SELECT max(run) FROM {LOG}").fetchone()[0]
        for tbl, ref, act, before, after in db.execute(f"SELECT tbl, ref, action, before, after FROM {LOG} WHERE run = ? "
                                                       "ORDER BY seq DESC", (run,)).fetchall():
            ref, before = json.loads(ref), json.loads(before)
            if act == "update":
                after = json.loads(after)
                if row_now(db, tbl, ref, list(after)) != after:
                    left.append(f"{tbl} {dumps(ref)}")
                    continue
                db.execute(f"UPDATE {tbl} SET {', '.join(f'{k} = ?' for k in before)} WHERE "
                           f"{' AND '.join(f'{k} = ?' for k in ref)}", [*before.values(), *ref.values()])
            else:
                row = dict(before)
                rid = row.pop("rowid_")
                if row_now(db, tbl, ref, list(ref)) is not None or db.execute(f"SELECT 1 FROM {tbl} WHERE rowid = ?", (rid,)).fetchone():
                    left.append(f"{tbl} {dumps(ref)} (a row there again)")
                    continue
                db.execute(f"INSERT INTO {tbl} (rowid, {', '.join(row)}) VALUES (?, {', '.join('?' * len(row))})", [rid, *row.values()])
            back += 1
        db.execute(f"DELETE FROM {LOG} WHERE run = ?", (run,))
        if db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0] == 0:
            db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return back, left, run


def report(p, full=True):
    print(f"records: {len(p['changes'])}, {sum(1 for c in p['changes'] if c['set'])} to change")
    for c in p["changes"]:
        print(f"  {c['patternid']} {c['name']}: {c['how']}")
        if c["set"]:
            print(f"      {dumps(c['before'])}")
            print(f"   -> {dumps(c['set'])}")
    for k, v in p["lists"].items():
        if k == "the records":
            continue
        print(f"  {k}: {len(v)}")
        for x in v:
            print(f"      {x}")


def backup(path):
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-broken-records"
    src, out = sqlite3.connect(path, timeout=30), sqlite3.connect(dst)
    try:
        src.backup(out)
    finally:
        out.close()
        src.close()
    return dst


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--eqoa-data", help="EQOA-Data's checkout (Information/databases/items/*.json)")
    ap.add_argument("--save-plan", metavar="FILE")
    ap.add_argument("--plan", metavar="FILE")
    ap.add_argument("--no-backup", action="store_true")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    if (a.apply or a.undo) and not a.no_backup:
        print(f"copy: {backup(a.db)}")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        back, left, run = undo(db)
        print(f"the run of {run}: put back {back}" + (f"; left, changed since: {', '.join(left)}" if left else ""))
        return
    if a.plan:
        with open(a.plan, encoding="utf-8") as f:
            p = json.load(f)
    else:
        if not a.eqoa_data:
            sys.exit("--eqoa-data (or --plan FILE)")
        p = plan(db, a.eqoa_data)
    report(p)
    if a.save_plan:
        p["made"] = datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")
        with open(a.save_plan, "w", encoding="utf-8") as f:
            json.dump(p, f, indent=1, ensure_ascii=False)
        print(f"plan: {a.save_plan}")
        return
    if a.apply:
        recs, held, loot, stale = apply(db, p)
        print(f"written: {recs} records, {held} held copies made whole, {loot} loot rows taken out; {stale} changed since the plan, left")
    else:
        print("(report only: --apply to write it)")


if __name__ == "__main__":
    main()
