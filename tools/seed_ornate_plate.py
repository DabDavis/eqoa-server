#!/usr/bin/env python3
"""Ornate plate at level 60 (the owner, 2026-11-13: "lets also add armor plate type 9 the sandstorm patch, but we dont
need the patch the armor is on the ISO"; internal/entity/livelook.go, this server's armour).

Set 9 is the ornate gold-and-steel plate CHAR.ESF holds under the plain plate's DictIDs (claude-notes/
charesf-shadowed-armor.md). An item record of model 9 on an armour slot wears it: a vanilla client that draws this
server's armour draws it (and its gold trim in a trim dye: the Armor Dyers' "Armor trim"), a PS2 and every other
client is sent set 4, the plain plate, in its place. The database already holds Sandstorm's level 48 set (patternid
34278-34282: Ornate Tunic, Bracers, Gloves, Leggings, Boots, classuse 879); this adds a level 60 set beside it.

What it writes (a report by default; --apply writes, --undo takes back exactly what the last --apply wrote):
- **Five records** (itempattern), at FIRST_ID on: chest, bracers, gloves, legs and boots of model 9, colour 0 (the
  art's own), each a copy of SOURCE's figures (the Valiant set, level 60 plate: its level, AC, stats, resists, hit
  points, wear, value, class and race masks, trade and rent) under the name, description and icon given here (the
  level 48 Ornate set's icons, Sandstorm's own for this art). Nothing sells or drops them: a GM gives them (#giveitem,
  the GM panel) or a later step stocks them.

A record is read at each request: no restart. Logged in ornate_plate_log; --undo deletes the records it made unless
a character holds one (then it says so and deletes nothing).

    tools/seed_ornate_plate.py [--db eqoa.db] [--apply | --undo] [--first-id 711000]
"""

import argparse
import datetime
import os
import shutil
import sqlite3

LOG = "ornate_plate_log"
FIRST_ID = 711000  # an empty thousand on both shards (2026-11-13): 717000-717024 hold the capes, 718000 on others
MODEL_ORNATE = 9  # the armour set: entity.OrnateSet

# (wear slot, the source record's name (level 60 plate, the Valiant set), the new name, the level 48 Ornate record whose
# icon it takes, its description)
PIECES = [
    (5, "Valiant Chestplate", "Ornate Breastplate", "Ornate Tunic",
     "A breastplate of polished steel chased in gold, the ornate plate of old. It holds a greater health enchantment."),
    (7, "Valiant Bracers", "Ornate Vambraces", "Ornate Bracers",
     "Vambraces of steel banded in gold, the ornate plate of old. They hold a greater health enchantment."),
    (19, "Valiant Gloves", "Ornate Gauntlets", "Ornate Gloves",
     "Gauntlets of polished steel, the ornate plate of old. They hold a potent health enchantment."),
    (10, "Valiant Leggings", "Ornate Greaves", "Ornate Leggings",
     "Greaves of steel chased in gold, the ornate plate of old. A health enchantment has been laid upon them."),
    (11, "Valiant Boots", "Ornate Sabatons", "Ornate Boots",
     "Sabatons of steel with gold-banded cuffs, the ornate plate of old. They hold a greater health enchantment."),
]

# the columns copied from the source record (everything but the id, the name, the description, the icon, the model
# and the colour)
OWN = {"patternid", "itemname", "itemdesc", "itemicon", "model", "color"}


def rows_of(db, sql, *args):
    cur = db.execute(sql, args)
    cols = [d[0] for d in cur.description]
    return [dict(zip(cols, r)) for r in cur.fetchall()]


def one(db, sql, *args):
    r = rows_of(db, sql, *args)
    return r[0] if r else None


def plan(db, first_id):
    """The records it would write: a list of dicts (column: value), and the problems that stop it."""
    out, problems = [], []
    for i, (wear, src_name, name, icon_of, desc) in enumerate(PIECES):
        src = one(db, "SELECT * FROM itempattern WHERE itemname = ? AND equipslot = ? AND model = 4 AND levelreq = 60 "
                      "ORDER BY patternid LIMIT 1", src_name, wear)
        icon = one(db, "SELECT itemicon FROM itempattern WHERE itemname = ? AND equipslot = ? AND model = ? "
                       "ORDER BY patternid LIMIT 1", icon_of, wear, MODEL_ORNATE)
        if src is None:
            problems.append(f"no level 60 plate record {src_name!r} of wear slot {wear} to copy")
            continue
        if icon is None:
            icon = {"itemicon": src["itemicon"]}
        rec = {k: v for k, v in src.items() if k not in OWN}
        rec.update(patternid=first_id + i, itemname=name, itemdesc=desc, itemicon=icon["itemicon"],
                   model=MODEL_ORNATE, color=0)
        taken = one(db, "SELECT patternid, itemname FROM itempattern WHERE patternid = ?", rec["patternid"])
        if taken is not None and taken["itemname"] != name:
            problems.append(f"patternid {rec['patternid']} is {taken['itemname']!r}")
        named = one(db, "SELECT patternid FROM itempattern WHERE itemname = ? AND patternid != ?", name, rec["patternid"])
        if named is not None:
            problems.append(f"{name!r} is already record {named['patternid']}")
        rec["_source"] = src["patternid"]
        rec["_exists"] = taken is not None
        out.append(rec)
    return out, problems


def ensure_log(db):
    db.execute(f"""CREATE TABLE IF NOT EXISTS {LOG} (
        run TEXT NOT NULL, patternid INTEGER NOT NULL, source INTEGER NOT NULL, itemname TEXT NOT NULL)""")


def report(recs):
    for r in recs:
        stats = ", ".join(f"{k} {r[k]}" for k in ("str", "sta", "agi", "wis", "dex", "cha", "intelligence", "HPMAX",
                                                    "POWMAX", "PR", "DR", "FR", "CR", "LR", "AR") if r.get(k))
        state = "kept as it is" if r["_exists"] else "new"
        print(f"  {r['patternid']} {r['itemname']!r}: wear {r['equipslot']}, model {r['model']}, level {r['levelreq']}, "
              f"AC {r['AC']}, classes {r['classuse']}, value {r['itemcost']}, icon {r['itemicon']}; {stats} "
              f"(copied from record {r['_source']}; {state})")


def apply(db, recs):
    run = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    ensure_log(db)
    n = 0
    for r in recs:
        if r["_exists"]:
            continue
        cols = [k for k in r if not k.startswith("_")]
        db.execute(f"INSERT INTO itempattern ({', '.join(cols)}) VALUES ({', '.join('?' for _ in cols)})",
                   [r[k] for k in cols])
        db.execute(f"INSERT INTO {LOG} (run, patternid, source, itemname) VALUES (?, ?, ?, ?)",
                   (run, r["patternid"], r["_source"], r["itemname"]))
        n += 1
    return run, n


def undo(db):
    ensure_log(db)
    last = one(db, f"SELECT max(run) AS run FROM {LOG}")
    if not last or not last["run"]:
        return None, 0, []
    ids = [r["patternid"] for r in rows_of(db, f"SELECT patternid FROM {LOG} WHERE run = ?", last["run"])]
    held = rows_of(db, f"SELECT patternid, count(*) AS n FROM charinventory WHERE patternid IN "
                       f"({', '.join('?' for _ in ids)}) GROUP BY patternid", *ids) if ids else []
    if held:
        return last["run"], 0, held
    for i in ids:
        db.execute("DELETE FROM itempattern WHERE patternid = ?", (i,))
    db.execute(f"DELETE FROM {LOG} WHERE run = ?", (last["run"],))
    return last["run"], len(ids), []


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--first-id", type=int, default=FIRST_ID)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        raise SystemExit(f"{a.db}: no database")
    db = sqlite3.connect(a.db, timeout=5)
    if a.undo:
        run, n, held = undo(db)
        if held:
            raise SystemExit(f"run {run}: characters hold {held}; nothing deleted")
        db.commit()
        print(f"undid run {run}: {n} records deleted" if run else "nothing to undo")
        return
    recs, problems = plan(db, a.first_id)
    print(f"{len(recs)} ornate plate records (model {MODEL_ORNATE}):")
    report(recs)
    if problems:
        raise SystemExit("refused:\n  " + "\n  ".join(problems))
    if not a.apply:
        print("a report: --apply writes them")
        return
    backup = f"{a.db}.before-ornate-{datetime.datetime.now():%Y%m%d-%H%M%S}"
    db.close()
    shutil.copy2(a.db, backup)
    db = sqlite3.connect(a.db, timeout=5)
    run, n = apply(db, recs)
    db.commit()
    print(f"run {run}: {n} records written (backup {backup}; --undo takes them back)")


if __name__ == "__main__":
    main()
