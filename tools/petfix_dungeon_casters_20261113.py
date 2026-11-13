#!/usr/bin/env python3
"""The instanced dungeons' casters cast (petition 1900, Brother, 2026-11-13: "doing The Guk instance none of the NPC
like wiz, knights, sha..etc... do not do any spells").

A creature casts only by its class template's abilities (entity/classtemplate.go), and every creature of world 14
fights by one of the dungeons' three tiers (tools/dungeons_20261112.py, bound by npctemplate "#n"): "dungeon trash"
and "dungeon named" had no ability, "dungeon boss" one area spell pinned to Pestilent Breath. The tiers are class -1,
the creature's own class (npcclass: the frogloks' wizards 13, shamans 8, necromancers 11, ...), so a wizard of Guk's
fought with its melee alone. (Its open world self casts nothing either: no template of its name has an ability.)

Each tier is given three abilities that pick their spell from the creature's own class's book (classspells, as
jib's class templates do: the "wizard", "necromancer" and "shadowknight" rows), at the highest level at or below its
own, each cast only by the classes it names (the abilities' "classes", the server's code of the same branch: a
warrior's book has a "nuke", Hero's Blow, and the tiers' melee classes stay melee):

  a nuke (a harmful hit on one target) every 12 s from 4 s into the fight, at 35% of a player's figure, no spell of
    a recast past 10 s or a cast past 5 s (jib's wizard's: no 180 s nova, no 15 s cast), by the druids, shamans,
    clerics, magicians, necromancers, enchanters, wizards and alchemists;
  a life tap every 15 s from 6 s, at 50%, no spell of a recast past 90 s (jib's shadow knight's and necromancer's),
    by the shadow knights and necromancers;
  a heal on itself below half its hit points, every 20 s, no cast past 5 s, by the paladins, druids, shamans and
    clerics.

An older server reads a template with "classes" as unreadable (and leaves it out): run it as the before.sh of the keep
restart that brings this code, never under an older server.

The boss tier keeps its Pestilent Breath first. The figures are ours (no record of a creature casting exists), as
the tiers' are: tune freely in the three rows.

And the knights (petition 1900's "knights"): world 14's "froglok vis knight", "froglok wan knight", "froglok zol
knight", "ghoul tal knight" and "ghoul urd knight" are class 0 (a warrior: Return Home's npcclass rows) and so would
cast nothing. They fight as shadow knights (class 3: their life tap, the dark knights of Guk; ghouls are undead) by
"dungeon trash shadowknight", a copy of "dungeon trash" of class 3 with the abilities its class casts (the life
tap), their "#n" rows pointed at it. "ghoul tal
necromaner" (a misspelt necromancer, class 0) and "froglok bonecaster" fight as necromancers (11) by "dungeon trash
necromancer" the same way. Their open world rows and npcclass are left as they are. Only the trash tier has them.

Refused, writing nothing, when a tier's abilities are not as the dungeons tool wrote them (trash and named none, the
boss the one area spell), a copy's name is taken, or a "#n" row of those names is not on "dungeon trash". With no
dungeon tiers (a shard without the dungeons) it says so and writes nothing. Report only by default; --apply in one
transaction, a gm_audit line a row, log dungeon_casters_1113_log; --undo exact (a row changed since is left and said).

WHEN IT TAKES EFFECT: templates and npctemplate are read at the server's start, or at once by the GM panel's
reload_npcs; an instance already open keeps its creatures' templates until reloaded, a new one has them.

    tools/petfix_dungeon_casters_20261113.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import re
import sqlite3
import sys

LOG = "dungeon_casters_1113_log"
DETAIL = ("petition 1900 (Brother): the instanced dungeons' casters cast their class's spells, the knights fight as "
          "shadow knights: tools/petfix_dungeon_casters_20261113.py")
WHO, VIA = "claude", "tool"
SRC = "dungeoncasters1113"
WORLD = 14
TIERS = ("dungeon trash", "dungeon named", "dungeon boss")
BOSS_BEFORE = [{"kind": "aoe", "spell_id": 905023, "every": 30, "first": 15, "damage_pct": 25}]
CASTERS = [7, 8, 9, 10, 11, 12, 13, 14]  # druid, shaman, cleric, magician, necromancer, enchanter, wizard, alchemist
TAPPERS = [3, 11]                        # shadow knight, necromancer
HEALERS = [2, 7, 8, 9]                   # paladin, druid, shaman, cleric
ADD = [
    {"kind": "nuke", "every": 12, "first": 4, "damage_pct": 35, "max_recast": 10, "max_cast": 5, "classes": CASTERS},
    {"kind": "lifetap", "every": 15, "first": 6, "damage_pct": 50, "max_recast": 90, "classes": TAPPERS},
    {"kind": "heal", "every": 20, "first": 5, "below": 50, "max_cast": 5, "classes": HEALERS},
]
# a trash creature's class by its name where npcclass's is no caster's: (pattern, class, the copy's suffix)
OVERRIDES = [
    (re.compile(r"\bknight\b"), 3, "shadowknight"),
    (re.compile(r"\bnecromaner\b|\bbonecaster\b"), 11, "necromancer"),
]
OVERRIDE_TIER = "dungeon trash"
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
    tiers = {n: row(db, "classtemplates", {"name": n}) for n in TIERS}
    if all(v is None for v in tiers.values()):
        notes.append("no dungeon tiers here (a shard without the dungeons): nothing to do")
        return changes, refusals, notes
    new_abilities = {}
    for name, t in tiers.items():
        if t is None:
            refusals.append(f"classtemplates {name!r} is not here, the others are")
            continue
        try:
            have = json.loads(t["abilities"] or "[]")
        except ValueError:
            refusals.append(f"{name!r}'s abilities are not JSON: {t['abilities']}")
            continue
        want = BOSS_BEFORE if name == "dungeon boss" else []
        if have == want + ADD:
            notes.append(f"{name!r} has its casts already")
            new_abilities[name] = have
            continue
        if have != want:
            refusals.append(f"{name!r}'s abilities are {t['abilities']}, not the dungeons tool's {abilities_json(want)}")
            continue
        new_abilities[name] = want + ADD
        changes.append(("classtemplates", {"name": name}, {"abilities": t["abilities"]}, {"abilities": abilities_json(want + ADD)}))
    if refusals:
        return changes, refusals, notes
    # the copies of the trash tier for the classes its casters' names say
    base = tiers[OVERRIDE_TIER]
    rows = db.execute("SELECT n.id, n.npc_name, t.template FROM npcs n JOIN npctemplate t ON t.name = '#' || n.id "
                      "WHERE n.world = ? ORDER BY n.id", (WORLD,)).fetchall()
    for pattern, cls, suffix in OVERRIDES:
        copy = f"{OVERRIDE_TIER} {suffix}"
        mine = [(i, n, tmpl) for i, n, tmpl in rows if pattern.search((n or "").lower())]
        if not mine:
            continue
        have = row(db, "classtemplates", {"name": copy})
        theirs = [a for a in new_abilities[OVERRIDE_TIER] if cls in a.get("classes", [cls])]  # its class's alone
        want = dict(base, name=copy, **{"class": cls}, abilities=abilities_json(theirs), source=SRC,
                    note=(f"{OVERRIDE_TIER!r} as class {cls} for the dungeons' {suffix}s that npcclass makes no caster "
                          f"(petition 1900: tools/petfix_dungeon_casters_20261113.py). Keep its figures with "
                          f"{OVERRIDE_TIER!r}'s when that is tuned. {OVERRIDE_TIER!r}'s note: " + (base["note"] or "")))
        if have is not None and have.get("source") != SRC:
            refusals.append(f"classtemplates {copy!r} is there already, not this tool's: look first")
            continue
        if have is None:
            changes.append(("classtemplates", {"name": copy}, None, want))
        for i, n, tmpl in mine:
            if tmpl == copy:
                continue
            if tmpl != OVERRIDE_TIER:
                refusals.append(f"{n!r} (row {i}) fights by {tmpl!r}, not {OVERRIDE_TIER!r}: left for a look")
                continue
            changes.append(("npctemplate", {"name": f"#{i}"}, {"template": tmpl}, {"template": copy}))
        notes.append(f"{copy!r} (class {cls}): {len(mine)} rows of {sorted({n for _, n, _ in mine})}")
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
