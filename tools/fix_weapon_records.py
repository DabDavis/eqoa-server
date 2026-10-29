#!/usr/bin/env python3
"""Melee weapons as Sandstorm's records have them (the user, 2026-10-29: "dust
has a worn club equipped, but it wont atk and no model").

A weapon's attack type (itempattern.attacktype: 1-6 the melee kinds) is what
CLIENT's TargetInRange and the server's reach (entity/attack.go SwingFor)
read: any other type is never in reach, so a weapon of type 0 cannot strike.
Its model (itempattern.model) is what the character's state channel holds in
its hand (handlers/character.go applyEquipmentFromDB): 0 is no weapon drawn.
The Worn Club (1825) had both 0, as Return Home's record has them, where
Sandstorm's own record (its smiths' wares in the harvests, --prices) is a
blunt weapon (type 3) of damage 20 at level 2, and 14 of the melee weapons
Sandstorm sells differ so.

Each melee weapon Sandstorm sells, by its name, takes Sandstorm's record
(slot, attack type, damage, level) where ours differs; one of no model takes
the model most of this server's weapons of its kind wear (the name's last
word and the attack type: an axe an axe's), else Sandstorm's starting weapon's
of its attack type (the Apprentice Club's, Sword's, Dagger's or Staff's):
OURS, as no ware carries a model. A potion's (itemeffect) is left, and a value set since by a GM
(gm_audit) too.

Every change is logged in weapon_records_fix_log (the pattern, its values
before and after, why); --undo puts back each one still as this left it and
drops the log. Report only by default. The server reads the record at each
swing and the model when a weapon is put on; a client sees it at its next
login, or when it puts the weapon on again.

    tools/fix_weapon_records.py [--db eqoa.db] --prices <harvest JSON or folder> ... [--apply | --undo]
"""
import argparse
import datetime
import glob
import json
import os
import sqlite3
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
MELEE = range(1, 7)
FIELDS = {"wear_slot": "equipslot", "attack_type": "attacktype", "damage": "weapondamage", "level_req": "levelreq"}
# the starting weapons (every new character's on Sandstorm), whose models stand in by attack type
STARTERS = {1: "apprentice sword", 3: "apprentice club", 4: "apprentice staff", 5: "apprentice dagger"}

LOG_SCHEMA = """CREATE TABLE weapon_records_fix_log (
    patternid INTEGER PRIMARY KEY,
    before    TEXT NOT NULL,
    after     TEXT NOT NULL,
    why       TEXT NOT NULL,
    applied   TEXT NOT NULL
)"""


def key(name):
    return " ".join((name or "").replace("`", "'").lower().split())


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def records(paths):
    """Sandstorm's melee weapons' records by name: {name: {column: value}}, a
    name of two records left out."""
    files = []
    for p in paths:
        files += glob.glob(os.path.join(p, "**", "*.json"), recursive=True) if os.path.isdir(p) else [p]
    seen = {}

    def walk(o):
        if isinstance(o, dict):
            if o.get("attack_type") in MELEE and o.get("damage", 0) > 0 and o.get("name"):
                yield o
            for v in o.values():
                yield from walk(v)
        elif isinstance(o, list):
            for v in o:
                yield from walk(v)

    for f in files:
        try:
            with open(f) as fh:
                d = json.load(fh)
        except (OSError, ValueError):
            continue
        for w in walk(d):
            seen.setdefault(key(w["name"]), set()).add(tuple((col, w.get(k)) for k, col in FIELDS.items()))
    return {k: dict(next(iter(v))) for k, v in seen.items() if len(v) == 1}


def plan(db, recs):
    skip = set()
    if has_table(db, "itemeffect"):
        skip |= {p for (p,) in db.execute("SELECT patternid FROM itemeffect")}
    if has_table(db, "gm_audit"):
        for (k,) in db.execute("SELECT rowkey FROM gm_audit WHERE tbl = 'itempattern'"):
            try:
                skip.add(int(json.loads(k)["patternid"]))
            except (ValueError, KeyError, TypeError):
                pass
    # the commonest model of each kind (the name's last word, the attack type)
    kinds = {}
    for name, at, model in db.execute("SELECT itemname, attacktype, model FROM itempattern WHERE model != 0 AND attacktype BETWEEN 1 AND 6"):
        words = key(name).split()
        if words:
            c = kinds.setdefault((words[-1], at), {})
            c[model] = c.get(model, 0) + 1
    models = {}
    for at, name in STARTERS.items():
        r = db.execute("SELECT model FROM itempattern WHERE lower(itemname) = ? AND model != 0 ORDER BY patternid LIMIT 1",
                       (name,)).fetchone()
        if r:
            models[at] = r[0]
    out = []
    cols = ", ".join(FIELDS.values())
    for row in db.execute(f"SELECT patternid, itemname, model, {cols} FROM itempattern ORDER BY patternid").fetchall():
        pid, name, model, now = row[0], row[1], row[2], dict(zip(FIELDS.values(), row[3:]))
        want = recs.get(key(name))
        if not want or pid in skip:
            continue
        diff = {c: v for c, v in want.items() if v is not None and now[c] != v}
        before = {c: now[c] for c in diff}
        why = "Sandstorm's record"
        if model == 0:
            words = key(name).split()
            kind = kinds.get((words[-1], want["attacktype"])) if words else None
            if kind:
                m = max(kind.items(), key=lambda kv: (kv[1], -abs(kv[0])))[0]
                diff["model"], before["model"] = m, 0
                why += f"; its model most of this server's {words[-1]}s' (ours)"
            elif want["attacktype"] in models:
                diff["model"], before["model"] = models[want["attacktype"]], 0
                why += f"; its model the {STARTERS[want['attacktype']]}'s (ours)"
        if diff:
            out.append((pid, name, before, diff, why))
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--prices", nargs="*", default=[], help="eqoa harvest JSON files or folders of them")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the records (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply changed")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        if not has_table(db, "weapon_records_fix_log"):
            print("nothing logged: nothing to undo")
            return
        db.execute("BEGIN IMMEDIATE")
        back = left = 0
        for pid, before, after in db.execute("SELECT patternid, before, after FROM weapon_records_fix_log").fetchall():
            before, after = json.loads(before), json.loads(after)
            c = list(after)
            now = db.execute(f"SELECT {', '.join(c)} FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
            if now is None or dict(zip(c, now)) != after:
                left += 1
                continue
            db.execute(f"UPDATE itempattern SET {', '.join(x + ' = ?' for x in c)} WHERE patternid = ?",
                       [before[x] for x in c] + [pid])
            back += 1
        db.execute("DROP TABLE weapon_records_fix_log")
        db.execute("COMMIT")
        print(f"put back {back} weapons, {left} changed since left as they are")
        return
    if a.apply and has_table(db, "weapon_records_fix_log"):
        sys.exit("weapon_records_fix_log is there: applied already (--undo first)")
    recs = records(a.prices)
    if not recs:
        sys.exit("no Sandstorm weapon records: give --prices (eqoa harvest JSON)")
    changes = plan(db, recs)
    for pid, name, before, after, why in changes:
        print(f"{pid:7d} {name}: " + ", ".join(f"{c} {before[c]} -> {after[c]}" for c in after) + f" ({why})")
    print(f"{len(changes)} weapons to change; {len(recs)} of Sandstorm's melee weapons' records read")
    if not a.apply or not changes:
        if not a.apply:
            print("report only: --apply to write")
        return
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_SCHEMA)
        for pid, name, before, after, why in changes:
            db.execute("INSERT INTO weapon_records_fix_log VALUES (?, ?, ?, ?, ?)",
                       (pid, json.dumps(before, sort_keys=True), json.dumps(after, sort_keys=True), why, when))
            c = list(after)
            db.execute(f"UPDATE itempattern SET {', '.join(x + ' = ?' for x in c)} WHERE patternid = ?",
                       [after[x] for x in c] + [pid])
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"changed {len(changes)} weapons; a client sees them at its next login")


if __name__ == "__main__":
    main()
