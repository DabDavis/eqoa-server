#!/usr/bin/env python3
"""The instanced dungeons' shamans and druids cast their class's spells (ticket 2004, Brother, 2026-11-13: "Just did the
BB Dungeon and the NPC are not doing thier class spells, the same as Guk was, like sha not using spells etc...").

tools/petfix_dungeon_casters_20261113.py (petition 1900) gave the dungeons' three tiers a nuke, a life tap and a heal
of the creature's own class book. Blackburrow's casters are its gnoll shamans and spiritists (npcclass 8) and Bishop
Narn (9); Runnyeye's its goblin witchdoctor (8) and High Priest Goblin (9). The cleric casts Justice and heals with
Remedy; but a shaman's (and a druid's) heals are heals over time (Celestial Healing, "Increase HP per tic"), which the
tiers' heal (one at once) never takes, and their books' damage over time (a shaman's afflictions and Toxic Bolt) no
ability asked for: a gnoll shaman of level 61 cast one spell, Barbiedolls of Karana, every 12 s, and nothing else.

Each tier (and the necromancer copy of the trash tier, its class's alone) is given:

  a damage over time (kind "dot", creaturedot.go: kept up on what it fights) every 18 s from 8 s into the fight, at
    35% of a player's figure (the nuke's), no spell of a cast past 5 s, by the druids, shamans and necromancers
    (their books' Winged Doom, Toxic Bolt, Toxxulian Blight at 60);
  a heal of its share (no spell: heal_pct, abilitytimes.go's share heal, the heal's look) named "Celestial Healing"
    (the shaman's and druid's heal of level 60) below half its hit points, every 20 s, 10% of its hit points, by the
    druids and shamans; the tiers' heal of the book keeps the paladins and clerics (2, 9) alone, who have one.

The figures are ours, as the 1900 tool's are: tune freely in the rows. No creature's class is changed: every other
creature of Blackburrow and Runnyeye is a warrior by npcclass (Return Home's rows), and no name of theirs says a
casting class (Blackburrow's whitepaws, redpaws, Pack Masters, Runnyeye's goblin adept: left).

Refused, writing nothing, when a row's abilities are not as the 1900 tool wrote them. With no dungeon tiers (a shard
without the dungeons) it says so and writes nothing. Report only by default; --apply in one transaction, a gm_audit
line a row, log dungeon_casters_2004_log; --undo exact (a row changed since is left and said).

WHEN IT TAKES EFFECT: classtemplates are read at the server's start, or at once by the GM panel's reload_npcs (a
template edit in the panel reloads them too); an instance already open keeps its creatures' templates until reloaded,
a new one has them.

    tools/dungeon_casters_2004_20261113.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "dungeon_casters_2004_log"
DETAIL = ("ticket 2004 (Brother): the instanced dungeons' shamans and druids cast their damage over time and heal: "
          "tools/dungeon_casters_2004_20261113.py")
WHO, VIA = "claude", "tool"

# as tools/petfix_dungeon_casters_20261113.py wrote them
CASTERS = [7, 8, 9, 10, 11, 12, 13, 14]
NUKE = {"kind": "nuke", "every": 12, "first": 4, "damage_pct": 35, "max_recast": 10, "max_cast": 5, "classes": CASTERS}
TAP = {"kind": "lifetap", "every": 15, "first": 6, "damage_pct": 50, "max_recast": 90, "classes": [3, 11]}
HEAL = {"kind": "heal", "every": 20, "first": 5, "below": 50, "max_cast": 5, "classes": [2, 7, 8, 9]}
BOSS = {"kind": "aoe", "spell_id": 905023, "every": 30, "first": 15, "damage_pct": 25}
# this tool's
DOT = {"kind": "dot", "every": 18, "first": 8, "damage_pct": 35, "max_cast": 5, "classes": [7, 8, 11]}
HEAL_BOOK = dict(HEAL, classes=[2, 9])
HEAL_SHARE = {"kind": "heal", "every": 20, "first": 5, "below": 50, "heal_pct": 10, "name": "Celestial Healing",
              "classes": [7, 8]}

ROWS = {  # name: (as 1900 left it, as this tool leaves it)
    "dungeon trash": ([NUKE, TAP, HEAL], [NUKE, TAP, HEAL_BOOK, DOT, HEAL_SHARE]),
    "dungeon named": ([NUKE, TAP, HEAL], [NUKE, TAP, HEAL_BOOK, DOT, HEAL_SHARE]),
    "dungeon boss": ([BOSS, NUKE, TAP, HEAL], [BOSS, NUKE, TAP, HEAL_BOOK, DOT, HEAL_SHARE]),
    "dungeon trash necromancer": ([NUKE, TAP], [NUKE, TAP, DOT]),
}
TIERS = ("dungeon trash", "dungeon named", "dungeon boss")
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,      -- classtemplates
    rowkey  TEXT NOT NULL,      -- the row's key (JSON)
    before  TEXT NOT NULL,      -- the changed columns as they were (JSON)
    after   TEXT NOT NULL,      -- as written (JSON)
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def audit(db, action, tbl, key, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key), go_json(before), go_json(after), detail))
    return cur.lastrowid


def abilities_json(a):
    return json.dumps(a, separators=(",", ":"))


def plan(db):
    """(changes [(key, before, after)], refusals, notes)."""
    changes, refusals, notes = [], [], []
    for t in ("classtemplates", "gm_audit"):
        if not table(db, t):
            return changes, [f"no {t} table here"], notes
    have = {n: db.execute("SELECT abilities FROM classtemplates WHERE name = ?", (n,)).fetchone() for n in ROWS}
    if all(have[n] is None for n in TIERS):
        notes.append("no dungeon tiers here (a shard without the dungeons): nothing to do")
        return changes, refusals, notes
    for name, (before, after) in ROWS.items():
        r = have[name]
        if r is None:
            refusals.append(f"classtemplates {name!r} is not here (tools/petfix_dungeon_casters_20261113.py first)")
            continue
        try:
            now = json.loads(r[0] or "[]")
        except ValueError:
            refusals.append(f"{name!r}'s abilities are not JSON: {r[0]}")
            continue
        if now == after:
            notes.append(f"{name!r} has them already")
            continue
        if now != before:
            refusals.append(f"{name!r}'s abilities are {r[0]}, not the 1900 tool's {abilities_json(before)}")
            continue
        changes.append(({"name": name}, {"abilities": r[0]}, {"abilities": abilities_json(after)}))
    return changes, refusals, notes


def apply(db, changes):
    db.execute(LOG_SCHEMA)
    for key, before, after in changes:
        db.execute("UPDATE classtemplates SET abilities = ? WHERE name = ?", (after["abilities"], key["name"]))
        a = audit(db, "update", "classtemplates", key, before, after, DETAIL)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES ('classtemplates', ?, ?, ?, ?)",
                   (json.dumps(key, sort_keys=True), json.dumps(before, sort_keys=True), json.dumps(after, sort_keys=True), a))


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, key, before, after in db.execute(f"SELECT seq, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key, before, after = json.loads(key), json.loads(before), json.loads(after)
        now = db.execute("SELECT abilities FROM classtemplates WHERE name = ?", (key["name"],)).fetchone()
        if now is None or now[0] != after["abilities"]:
            left += 1
            print(f"  left: classtemplates {key}, changed since it was written")
            continue
        db.execute("UPDATE classtemplates SET abilities = ? WHERE name = ?", (before["abilities"], key["name"]))
        audit(db, "update", "classtemplates", key, after, before, "undo: " + DETAIL)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if left == 0:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"put back {back}, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        changes, refusals, notes = plan(db)
        for r in refusals:
            print("REFUSED:", r)
        if refusals:
            db.execute("ROLLBACK")
            return 1
        for n in notes:
            print(n)
        for key, before, after in changes:
            print(f"classtemplates {key}: {before['abilities']} -> {after['abilities']}")
        if not changes:
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"report only: {len(changes)} rows to write (--apply to write)")
            return 0
        apply(db, changes)
        db.execute("COMMIT")
        print(f"applied: {len(changes)} rows")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
