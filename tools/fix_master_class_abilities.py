#!/usr/bin/env python3
"""A Master Class's abilities in cmcatalog: each needing only the master class
itself, and named for what it gives (F3 20261026-222613, jib: "Master class
abilities should be named what they are eg "Ogre Pill" and not "Transmuter
2" or "Transmuter 3" ... Tag *unlocks* the 2, but isn't a singular master
class purchase chain").

tools/seed_masteries.py wrote a master class's ranks as EQOA-Data's lists
write them, a chain: rank 1 the class itself (its "tag"), then "<Class> 2",
"<Class> 3" (a few a 4), each with the rank before it as its parent, so its
abilities could be bought only in the list's order. The seed writes them now
as this makes a database seeded before it:

- parent: the master class's first rank, so each is bought in any order once
  the class is; but one that improves another rank's ability keeps that rank
  (seed_masteries.MASTER_UPGRADES: Defender 3, Immortal Guard, free after
  Guard; and a rank needing or replacing an ability an earlier one adds, none
  in the lists today);
- name: what it gives (seed_masteries.ability_rank_name): the one ability it
  adds, as the server's spell is named (as the list names it when no spell was
  found), else seed_masteries.MASTER_ABILITY_NAMES (a rank of stats alone or
  of several abilities: the rank's notes', EQOA-Data's spell lists', or OURS).

Only a rank the seed wrote (cmcatalog_seed, source "eqoa-data cms") is
changed, and only a column still holding the seed's value (the rank before it;
the list's name) or the value an earlier run of this left; a row a GM has
changed in the panel (gm_audit) is left whole. Every row left is said, and so
is a row another tool's log names (a table with tbl and rowkey columns). Ids,
costs and the rest stay, so a character keeps every rank it owns (charcms is
never touched). Before writing, every rank is walked for every class and race
(tools/fix_mastery_catalog.py's walk): a plan that leaves a rank unreachable,
or makes a cycle, is refused.

Every change is logged in mc_ability_fix_log (the row, the column, the value
before and after, and why); --undo puts back each value still as this left it
(one changed since is left, and said) and drops the log. Both take a copy of
the database first (<db>.bak-<when>-pre-mcabilities). Report only by default.
The server reads the catalog at its start, and again when a GM edits a
cmcatalog row in the panel: stop it around --apply or --undo, or restart it
after.

    tools/fix_master_class_abilities.py --eqoa-data ../external/EQOA-Data [--db eqoa.db] [--apply | --undo]
"""
import argparse
import collections
import datetime
import json
import os
import re
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import fix_mastery_catalog as fx  # noqa: E402
import seed_masteries as seed  # noqa: E402

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
CLASS_NAMES = seed.CLASS_NAMES
COLUMNS = ("parent", "name")  # all this ever writes
WHY_PARENT = "a master class's ability needs only the master class (F3 20261026-222613)"

LOG_SCHEMA = """CREATE TABLE IF NOT EXISTS mc_ability_fix_log (
    id      INTEGER NOT NULL,
    col     TEXT NOT NULL,
    before,
    after,
    why     TEXT NOT NULL,
    applied TEXT NOT NULL,
    PRIMARY KEY (id, col)
)"""


def has_table(db, name):
    return fx.has_table(db, name)


def rows_of(db):
    """Every cmcatalog row by id, the columns this reads."""
    cols = ("id", "category", "classes", "mastery", "rank", "name", "parent", "adds", "add_spells", "source")
    return {r[0]: dict(zip(cols, r)) for r in db.execute(f"SELECT {', '.join(cols)} FROM cmcatalog")}


def spell_names(db):
    if not has_table(db, "spellpattern"):
        return {}
    return dict(db.execute("SELECT spellid, spellname FROM spellpattern"))


def other_logs(db):
    """{cmcatalog id: [log table, ...]}: the rows another tool's log names (a
    table with tbl and rowkey columns, gm_audit aside)."""
    out = collections.defaultdict(list)
    for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name"):
        if t in ("gm_audit", "mc_ability_fix_log"):
            continue
        cols = {c[1] for c in db.execute(f'PRAGMA table_info("{t}")')}
        if not {"tbl", "rowkey"} <= cols:
            continue
        for (k,) in db.execute(f'SELECT rowkey FROM "{t}" WHERE tbl = ?', ("cmcatalog",)):
            try:
                i = int(json.loads(k)["id"])
            except (ValueError, KeyError, TypeError):
                m = re.search(r"\d+", k or "")
                if not m:
                    continue
                i = int(m.group())
            if t not in out[i]:
                out[i].append(t)
    return out


def logged(db):
    """{(id, col): after} of an earlier run of this."""
    if not has_table(db, "mc_ability_fix_log"):
        return {}
    return {(i, c): a for i, c, a in db.execute("SELECT id, col, after FROM mc_ability_fix_log")}


def plan(db, data):
    """(changes, left, notes): changes [(id, col, before, after, why)]; left
    [(id, name, why)] for a value the plan would change but must not; notes
    [(id, name, what)] for a row another tool's log names (changed all the
    same where its column still holds the seed's value)."""
    want = {r["id"]: r for r in seed.plan_rows(data, collections.Counter())
            if r["category"] == seed.MASTER and r["rank"] > 1}
    cat = rows_of(db)
    mine = fx.seeded_ids(db)
    gm = fx.gm_changed(db)
    others = other_logs(db)
    names = spell_names(db)
    earlier = logged(db)
    changes, left, notes = [], [], []
    for rid in sorted(cat):
        r = cat[rid]
        if r["category"] != seed.MASTER or r["rank"] <= 1:
            continue
        s = want.get(rid)
        if s is None or (s["classes"], s["mastery"].lower(), s["rank"]) != \
                (r["classes"], str(r["mastery"]).strip().lower(), r["rank"]):
            left.append((rid, r["name"], "not a rank of the lists where tools/seed_masteries.py numbers it"))
            continue
        if rid not in mine or r["source"] != seed.SOURCE:
            left.append((rid, r["name"], f"not a row the seed wrote (source {r['source']!r})"))
            continue
        if rid in gm:
            left.append((rid, r["name"], "a GM has changed the row (gm_audit)"))
            continue
        if rid in others:
            notes.append((rid, r["name"], f"also named in {', '.join(others[rid])}: the columns it changed kept"))
        name, why_name = seed.ability_rank_name(r, names)
        why_parent = f"kept after rank {s['parent'] - rid + r['rank']}: {s['kept']}" if s["kept"] else WHY_PARENT
        for col, seeded, after, why in (("parent", rid - 1, s["parent"], why_parent),
                                        ("name", s["list_name"], name, why_name)):
            now = r[col]
            if str(now) == str(after):
                continue
            if str(now) != str(seeded) and earlier.get((rid, col)) != now:
                left.append((rid, r["name"], f"{col} is {now!r}, not the seed's {seeded!r}: changed since, left"))
                continue
            changes.append((rid, col, now, after, why))
    return changes, left, notes


def owners(db, cat):
    """(characters, ranks): who owns a master class's ability now."""
    if not has_table(db, "charcms"):
        return 0, 0
    ids = [i for i, r in cat.items() if r["category"] == seed.MASTER and r["rank"] > 1]
    if not ids:
        return 0, 0
    q = ",".join("?" * len(ids))
    return db.execute(f"SELECT count(DISTINCT serverid), count(*) FROM charcms WHERE cmid IN ({q})", ids).fetchone()


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-mcabilities"
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def report(db, data):
    """The plan, said, and the walk over it: (changes, refusal or "")."""
    changes, left, notes = plan(db, data)
    cat = rows_of(db)
    by_id = collections.defaultdict(dict)
    for rid, col, before, after, why in changes:
        by_id[rid][col] = (before, after, why)
    for rid in sorted(by_id):
        r, c = cat[rid], by_id[rid]
        who = CLASS_NAMES[r["classes"].bit_length() - 1] if r["classes"] else "?"
        parts = []
        if "name" in c:
            parts.append(f"{c['name'][0]!r} -> {c['name'][1]!r} ({c['name'][2]})")
        if "parent" in c:
            parts.append(f"parent {c['parent'][0]} -> {c['parent'][1]}")
        print(f"{rid:6d} {who:3s} " + "; ".join(parts))
    for rid, name, why in left:
        print(f"{rid:6d} {name}: left as it is ({why})")
    for rid, name, what in notes:
        print(f"{rid:6d} {name}: {what}")
    print(f"{len(changes)} values to change ({sum(1 for c in changes if c[1] == 'name')} names, "
          f"{sum(1 for c in changes if c[1] == 'parent')} parents), {len(left)} left")
    full, spells_of = fx.catalogue(db), fx.class_spells(db)
    fixed = fx.applied(full, [c for c in changes if c[1] == "parent"])
    before, after = fx.walk(full, spells_of), fx.walk(fixed, spells_of)
    loops = fx.cycles(fixed) - fx.cycles(full)
    worse = {i: v for i, v in after.items() if set(v) - set(before.get(i, []))}
    print(f"walk: {len(full)} ranks, every class and race: {len(after)} unreachable after ({len(before)} before), "
          f"{len(fx.cycles(fixed))} on a cycle")
    for line in fx.describe(after, fixed):
        print(line)
    n, k = owners(db, cat)
    print(f"characters owning a master class's ability: {n} ({k} ranks); every rank kept, by its id")
    if worse:
        return changes, "refused: the plan would leave the ranks above unreachable"
    if loops:
        return changes, "refused: ranks on a cycle of parent and require_ranks"
    return changes, ""


def apply(db, changes):
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_SCHEMA)
        for rid, col, before, after, why in changes:
            if col not in COLUMNS:
                raise SystemExit(f"{col}: not a column this writes")
            # a later run keeps the first value before, so --undo goes back to it
            db.execute("INSERT OR IGNORE INTO mc_ability_fix_log (id, col, before, after, why, applied) "
                       "VALUES (?, ?, ?, ?, ?, ?)", (rid, col, before, after, why, when))
            db.execute("UPDATE mc_ability_fix_log SET after = ?, why = ?, applied = ? WHERE id = ? AND col = ?",
                       (after, why, when, rid, col))
            db.execute(f"UPDATE cmcatalog SET {col} = ? WHERE id = ?", (after, rid))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db, path):
    if not has_table(db, "mc_ability_fix_log"):
        print("nothing logged: nothing to undo")
        return
    entries = db.execute("SELECT id, col, before, after FROM mc_ability_fix_log ORDER BY id, col").fetchall()
    print(f"copy: {backup(db, path)}")
    db.execute("BEGIN IMMEDIATE")
    try:
        back = 0
        for rid, col, before, after in entries:
            if col not in COLUMNS:
                raise SystemExit(f"mc_ability_fix_log names column {col!r}, which this tool never writes")
            now = db.execute(f"SELECT {col} FROM cmcatalog WHERE id = ?", (rid,)).fetchone()
            if now is None or now[0] != after:
                print(f"  {rid} {col}: changed since ({now[0] if now else 'no row'!r}): left as it is")
                continue
            db.execute(f"UPDATE cmcatalog SET {col} = ? WHERE id = ?", (before, rid))
            back += 1
        db.execute("DROP TABLE mc_ability_fix_log")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"put back {back} of {len(entries)} values; restart the server to read them")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--eqoa-data", help="EQOA-Data's checkout, or its Information/databases/cms directory")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the changes (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply changed")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    if not has_table(db, "cmcatalog"):
        sys.exit(f"{a.db}: no cmcatalog (tools/seed_masteries.py fills it)")
    if a.undo:
        undo(db, a.db)
        return
    if not a.eqoa_data:
        sys.exit("--eqoa-data is required")
    changes, refusal = report(db, seed.load(a.eqoa_data))
    if refusal:
        sys.exit(refusal)
    if not a.apply or not changes:
        if not a.apply:
            print("report only: --apply to write")
        return
    print(f"copy: {backup(db, a.db)}")
    apply(db, changes)
    print(f"changed {len(changes)} values; restart the server to read them")


if __name__ == "__main__":
    main()
