#!/usr/bin/env python3
"""The Wandering Priestess casts (petition 5027, Trulyblind, Live, 2026-11-14: "the 17 quest Wandering Priestess Does
Not have right spells. She just Duel Hits, but doesnt do spells or AoE attack").

The Wandering Priestess (npcs 122506, Blakedown, level 21: FrontiersForge's sighting, tools/seed_quest_npcs) is the
kill of "The Ceremony" (quest 100756, Neriak's level 17 side quest, step 2: her vial of blood). A creature casts only
by its class template's abilities (entity/classtemplate.go), and no npctemplate row named one for her: she fought as a
warrior (class 0) with her melee alone. Quest Masters' guide (external/questmasters/text/id161.txt): "She is a level
20 caster and can heal herself"; EQOA-Data's: "CONs Red ... roaming spawn"; the player: spells and an area attack.

  classtemplates "quest wandering priestess" (new): class 9, a cleric (a priestess; her book is the cleric's), hit
    points, damage taken and melee as her own (100), three abilities, the dungeon casters' shape (petition 1900):
      a heal on herself below half her hit points, every 20 s from 5 s, no cast past 5 s (the cleric's book);
      a nuke every 12 s from 3 s, at 50% of a player's figure, no recast past 10 s, no cast past 5 s (the cleric's
        book at her level: Holy Bolt's 142 -> 71);
      an area spell every 20 s from 8 s: Fire Storm (700145, level 16, 82 on her target and 36 within 25 of it, the
        cleric's book has no area spell that hurts the living), at 75% (62 and 27).
    The figures are ours (no record of her casting exists): a red caster for a group of 17s. Tune in the row.
  npctemplate "#122506" (new) -> it: her own row's (the name's other rows, none, would not be touched).

The Live shard and Heirloom have her as the same row. Templates and npctemplate are read at the server's start, or at
once by the GM panel's reload_npcs: a restart (a keep restart's before.sh) or that reload. Refused, nothing written,
when she is not row 122506 of that name or either row is there not as this tool writes it. Report only by default;
--apply in one transaction, a gm_audit line a row, log wandering_priestess_1114_log; --undo exact.

    tools/wandering_priestess_20261114.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "wandering_priestess_1114_log"
DETAIL = ("petition 5027 (Trulyblind): the Wandering Priestess heals herself, casts a nuke and an area spell: "
          "tools/wandering_priestess_20261114.py")
WHO, VIA = "claude", "tool"
SRC = "priestess1114"
ROW, NAME = 122506, "Wandering Priestess"
TEMPLATE = "quest wandering priestess"
ABILITIES = [
    {"kind": "heal", "every": 20, "first": 5, "below": 50, "max_cast": 5},
    {"kind": "nuke", "every": 12, "first": 3, "damage_pct": 50, "max_recast": 10, "max_cast": 5},
    {"kind": "aoe", "spell_id": 700145, "every": 20, "first": 8, "damage_pct": 75},
]
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,      -- classtemplates | npctemplate
    rowkey  TEXT NOT NULL,      -- the row's key (JSON)
    before  TEXT,               -- the changed columns as they were (JSON); NULL: the row was added
    after   TEXT NOT NULL,      -- as written (JSON; an added row's every column)
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
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key), go_json(before or {}), go_json(after), detail))
    return cur.lastrowid


def abilities_json(a):
    return json.dumps(a, separators=(",", ":"))


def row(db, tbl, key):
    w = " AND ".join(f"{k} = ?" for k in key)
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {w}", list(key.values()))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def plan(db):
    """(changes [(tbl, key, before or None, after)], refusals, notes)."""
    changes, refusals, notes = [], [], []
    for t in ("classtemplates", "npctemplate", "npcs", "gm_audit"):
        if not table(db, t):
            return changes, [f"no {t} table here"], notes
    n = db.execute("SELECT npc_name FROM npcs WHERE id = ?", (ROW,)).fetchone()
    if n is None or n[0] != NAME:
        return changes, [f"npcs {ROW} is {n[0] if n else 'missing'!r}, not {NAME!r}"], notes
    want = {"name": TEMPLATE, "class": 9, "hp_pct": 100, "taken_pct": 100, "melee_pct": 100,
            "abilities": abilities_json(ABILITIES), "source": SRC,
            "note": "petition 5027: the Wandering Priestess (The Ceremony, level 17) heals, nukes and casts an area "
                    "spell: tools/wandering_priestess_20261114.py"}
    have = row(db, "classtemplates", {"name": TEMPLATE})
    if have is None:
        changes.append(("classtemplates", {"name": TEMPLATE}, None, want))
    elif any(have.get(k) != v for k, v in want.items()):
        refusals.append(f"classtemplates {TEMPLATE!r} is there, not as this tool writes it: look first")
    else:
        notes.append(f"classtemplates {TEMPLATE!r} there already")
    key = f"#{ROW}"
    have = row(db, "npctemplate", {"name": key})
    want = {"name": key, "template": TEMPLATE, "note": "petition 5027: tools/wandering_priestess_20261114.py", "source": SRC}
    if have is None:
        changes.append(("npctemplate", {"name": key}, None, want))
    elif have.get("template") != TEMPLATE:
        refusals.append(f"npctemplate {key!r} names {have.get('template')!r} already: look first")
    else:
        notes.append(f"npctemplate {key!r} there already")
    for tbl, k, _, after in changes:
        notes.append(f"{tbl} {k}: added: " + json.dumps(after)[:300])
    return changes, refusals, notes


def where(key):
    return " AND ".join(f"{k} = ?" for k in key), list(key.values())


def apply(db, changes):
    db.execute(LOG_SCHEMA)
    for tbl, key, before, after in changes:
        if before is None:
            cols = list(after)
            db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' for _ in cols)})", [after[c] for c in cols])
            a = audit(db, "insert", tbl, key, None, after, DETAIL)
        else:
            w, args = where(key)
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in after)} WHERE {w}", list(after.values()) + args)
            a = audit(db, "update", tbl, key, before, after, DETAIL)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                   (tbl, json.dumps(key, sort_keys=True), None if before is None else json.dumps(before, sort_keys=True),
                    json.dumps(after, sort_keys=True), a))


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, tbl, key, before, after in db.execute(f"SELECT seq, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key, after = json.loads(key), json.loads(after)
        now = row(db, tbl, key)
        if now is None or any(now.get(c) != v for c, v in after.items()):
            left += 1
            print(f"  left: {tbl} {key}, changed since it was written")
            continue
        w, args = where(key)
        if before is None:
            db.execute(f"DELETE FROM {tbl} WHERE {w}", args)
            audit(db, "delete", tbl, key, after, {}, "undo: " + DETAIL)
        else:
            before = json.loads(before)
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in before)} WHERE {w}", list(before.values()) + args)
            audit(db, "update", tbl, key, after, before, "undo: " + DETAIL)
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
        if table(db, LOG):
            db.execute("ROLLBACK")
            print(f"applied already ({LOG} is there): --undo first")
            return 1
        changes, refusals, notes = plan(db)
        for r in refusals:
            print("REFUSED:", r)
        if refusals:
            db.execute("ROLLBACK")
            return 1
        for n in notes:
            print(n)
        for tbl, key, before, after in changes:
            if before is None:
                print(f"{tbl} {key}: added (class {after.get('class')}, abilities {after.get('abilities')})")
            elif tbl == "npctemplate":
                print(f"{tbl} {key}: {before['template']} -> {after['template']}")
            else:
                print(f"{tbl} {key}: {before} -> {after}")
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
