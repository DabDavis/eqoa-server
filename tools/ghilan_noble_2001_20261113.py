#!/usr/bin/env python3
"""The Ghilan Noble's blessing on herself too, and her debuff on what she fights (ticket 2001, the magician Waldorf,
2026-11-13: "Ghilan Noble doesn't seem to cast her blessing on herself in addition to the caster. Efreeti Lord does
cast Efreeti's fire on both so one must be incorrect. Also can you confirm that Ghilan Noble also casts a debuff on
the mob. That was one of the first spells she would use which was great for pulling mobs on live").

The blessing: the workbook gives each magician pet's buff to its owner (Ghilan's Blessing and Efreeti's Fire alike:
sheet scope Owner, a "Pet Buff"). The Efreeti Lord's went on itself by the owner's word of 2026-11-01 and on its
owner too by petition 1478 (tools/fix_pets_1110.py: two abilities, target self and target owner); the Ghilan Noble
kept hers on her owner alone. She is given the second the Efreeti Lord's way: Ghilan's Blessing, target self, 5 s
after her summoning and whenever she lacks it (30 minutes), after her owner's at 3 s. The Hydromancer's notes of
the operator's Known Issues sheet say the same ("Ghilan Noble buffs caster and herself with 50 int/wis 250 hp & 130
hot").

The debuff: she had none (her kit: two heals, the blessing, Ghilan's Chill, Ghilan's Frost Wave, Purifying Tide, her
healing tide). The operator's "Known Issues Spell_CM sheet" (Magician rows 45-46) names it: "She Debuffs mobs cr"
and "-35 debuff ability for each *mancer pet of their element": a cold resist debuff of 35. Nothing names its spell,
time or look, so those are ours: "Ghilan's Frost Curse", "Target, Decrease CR by 35, 1min" (the minute of the
magician's own resist debuffs, Synder's Burning Hatred's 35 FR among them), a 2 s cast as her Chill's, its look and
icon, range and level; free (a summoned pet's casts cost nothing). Her first ability: the first thing she casts at a
creature, 1 s into the fight, kept up (cast again once it is about to run out, looked at every 30 s): the server's
pets cast a "debuff" at what they fight from this branch's code (entity/pettemplate.go's petDebuffLocked; an older
binary skips the ability). Taken off a creature's cold resist it lowers its chance to resist the magician's cold
spells and lets more of them land on one strong in cold (workbook.go's npcResistKindPct), and it angers the
creature at her.

Ids differ between shards: the spell is made at the next id of each (spellpattern is AUTOINCREMENT; the id counter
put back by --undo), the pet found by its id and name (701088, "Ghilan Noble", the same on both). Refused, writing
nothing, unless the pet's abilities are as tools/fix_pets_1110.py left them (seven: heal 905457, heal 905456, buff
902167 on its owner, nukes 905454, 905455, 905130, the group heal 905495) and no spell of the new name is there.
Report only by default; --apply in one transaction, a gm_audit line a row, logged in ghilan_2001_log; a second
--apply finds it done and writes nothing; --undo exact (a row changed since is left and said).

WHEN IT TAKES EFFECT: petstats and spells are read at the server's start (or a GM panel edit of the table), a pet
made at its summoning: run it as the before.sh of the keep restart that brings this branch's binary, on each shard;
a Ghilan Noble out then is summoned again to have it.

    tools/ghilan_noble_2001_20261113.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "ghilan_2001_log"
DETAIL = ("ticket 2001 (Waldorf): the Ghilan Noble's blessing on herself too, and her cold resist debuff: "
          "tools/ghilan_noble_2001_20261113.py")
WHO, VIA = "claude", "tool"

GHILAN, GHILAN_NAME = 701088, "Ghilan Noble"
BLESSING, BLESSING_NAME = 902167, "Ghilan's Blessing"
LOOK, LOOK_NAME = 905454, "Ghilan's Chill"  # whose look, icon, range and cast the curse takes
CURSE_NAME = "Ghilan's Frost Curse"
CURSE_CR, CURSE_SECONDS, CURSE_EVERY = 35, 60, 30
CURSE_LINE = f"Target, Decrease CR by {CURSE_CR}, 1min"
CURSE_DESC = (f"The Ghilan Noble's frost curse: {CURSE_CR} off her target's cold resist for a minute, the first "
              "thing she casts at it.")
CURSE_EFFECTS = {"v": 1, "scope": "target", "target_type": "Enemy", "harmful": True, "duration_s": CURSE_SECONDS,
                 "stack": 0, "level": 60, "min_level": 1, "req_equipment": "Any", "sheet_type": "Statistic Change",
                 "effects": [{"type": "Statistic Change", "kind": "stat", "stat": "cr", "amount": -CURSE_CR,
                              "resist": "cr"}]}
CURSE_SOURCE = ("ticket 2001 (Waldorf); the Known Issues sheet's -35 CR, Hydromancer rows; Ghilan's Chill's look "
                "(tools/ghilan_noble_2001_20261113.py)")

# her abilities as tools/fix_pets_1110.py left them (both shards)
BEFORE = [
    {"every": 60.0, "first": 1.0, "kind": "heal", "spell_id": 905457, "target": "either", "below": 50},
    {"every": 30.0, "first": 1.0, "kind": "heal", "spell_id": 905456, "target": "either", "below": 60},
    {"every": 1800.0, "first": 3.0, "kind": "buff", "spell_id": BLESSING, "target": "owner"},
    {"every": 30.0, "first": 5.0, "kind": "nuke", "spell_id": 905454},
    {"every": 30.0, "first": 8.0, "kind": "nuke", "spell_id": 905455},
    {"every": 30.0, "first": 8.0, "kind": "nuke", "spell_id": 905130},
    {"every": 6.0, "first": 2.0, "kind": "heal", "spell_id": 905495, "target": "group", "below": 100},
]
SELF_BLESSING = {"every": 1800.0, "first": 5.0, "kind": "buff", "spell_id": BLESSING, "target": "self"}
NOTE = (f" Ticket 2001 (Waldorf, 2026-11-13): {BLESSING_NAME} on herself too (the Efreeti Lord's way), and "
        f"{CURSE_NAME} ({CURSE_CR} CR off her target for a minute) her first cast at a creature "
        "(tools/ghilan_noble_2001_20261113.py).")


def after_kit(curse_id):
    """Her abilities with the curse first and her blessing on herself after her owner's."""
    curse = {"every": float(CURSE_EVERY), "first": 1.0, "kind": "debuff", "spell_id": curse_id}
    return [curse] + BEFORE[:3] + [SELF_BLESSING] + BEFORE[3:]


LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,      -- spellpattern | spelleffect | petstats | sqlite_sequence
    rowkey  TEXT NOT NULL,      -- the row's key (JSON)
    before  TEXT,               -- the whole row as it was (JSON); NULL: the row was added
    after   TEXT NOT NULL,      -- the whole row as written (JSON)
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def compact(v):
    return json.dumps(v, separators=(",", ":"))


def audit(db, action, tbl, key, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key), go_json(before or {}), go_json(after), detail))
    return cur.lastrowid


def row(db, tbl, key):
    w = " AND ".join(f"{k} = ?" for k in key)
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {w}", list(key.values()))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def kit_of(text):
    try:
        return json.loads(text or "[]")
    except ValueError:
        return None


def plan(db):
    """(state, refusals, notes): state "todo" with the rows to write, or "done"."""
    refusals, notes = [], []
    for t in ("spellpattern", "spelleffect", "petstats", "gm_audit"):
        if not table(db, t):
            return None, [f"no {t} table here"], notes
    pet = row(db, "petstats", {"spellid": GHILAN})
    if pet is None or pet["name"] != GHILAN_NAME:
        return None, [f"petstats {GHILAN} is {pet and pet['name']!r}, not {GHILAN_NAME!r}"], notes
    for sid, name in ((BLESSING, BLESSING_NAME), (LOOK, LOOK_NAME)):
        r = row(db, "spellpattern", {"spellid": sid})
        if r is None or r["spellname"] != name:
            refusals.append(f"spell {sid} is {r and r['spellname']!r}, not {name!r}")
    if refusals:
        return None, refusals, notes
    named = db.execute("SELECT spellid FROM spellpattern WHERE lower(trim(spellname)) = ?", (CURSE_NAME.lower(),)).fetchall()
    kit = kit_of(pet["abilities"])
    if len(named) == 1 and kit == after_kit(named[0][0]):
        notes.append(f"{GHILAN_NAME} has her self blessing and {CURSE_NAME} ({named[0][0]}) already")
        return "done", refusals, notes
    if named:
        refusals.append(f"a spell named {CURSE_NAME!r} is there already ({[r[0] for r in named]}), her row not this "
                        "tool's: look first")
    if kit != BEFORE:
        refusals.append(f"{GHILAN_NAME}'s abilities are {pet['abilities']}, not what tools/fix_pets_1110.py left "
                        "(changed since: a GM's?)")
    if not refusals:
        notes.append(f"{GHILAN_NAME} ({GHILAN}): {BLESSING_NAME} on herself too; {CURSE_NAME} made "
                     f"({CURSE_LINE}) and cast first at what she fights")
    return "todo", refusals, notes


def seq(db):
    r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'spellpattern'").fetchone()
    return None if r is None else r[0]


def log(db, tbl, key, before, after, a):
    db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
               (tbl, json.dumps(key, sort_keys=True), None if before is None else json.dumps(before, sort_keys=True),
                json.dumps(after, sort_keys=True), a))


def apply(db):
    """Writes the rows; returns the curse's id."""
    db.execute(LOG_SCHEMA)
    look = row(db, "spellpattern", {"spellid": LOOK})
    look_fx = row(db, "spelleffect", {"spellid": LOOK})["fxdict"]
    seq0 = seq(db)
    pattern = {"abilitylvl": look["abilitylvl"], "unk2": 0, "unk3": 0, "range": look["range"],
               "casttime": look["casttime"], "power": 0, "iconColor": look["iconColor"], "icon": look["icon"],
               "scope": look["scope"], "recast": CURSE_EVERY, "eqprequire": -1, "spellname": CURSE_NAME,
               "spelldesc": CURSE_DESC}
    cols = list(pattern)
    cid = db.execute(f"INSERT INTO spellpattern ({', '.join(cols)}) VALUES ({', '.join('?' for _ in cols)})",
                     [pattern[c] for c in cols]).lastrowid
    written = row(db, "spellpattern", {"spellid": cid})
    log(db, "spellpattern", {"spellid": cid}, None, written,
        audit(db, "insert", "spellpattern", {"spellid": cid}, None, written, DETAIL))
    effect = {"spellid": cid, "line": CURSE_LINE, "fxdict": look_fx, "source": CURSE_SOURCE,
              "effects": compact(CURSE_EFFECTS)}
    cols = list(effect)
    db.execute(f"INSERT INTO spelleffect ({', '.join(cols)}) VALUES ({', '.join('?' for _ in cols)})",
               [effect[c] for c in cols])
    written = row(db, "spelleffect", {"spellid": cid})
    log(db, "spelleffect", {"spellid": cid}, None, written,
        audit(db, "insert", "spelleffect", {"spellid": cid}, None, written, DETAIL))
    pet = row(db, "petstats", {"spellid": GHILAN})
    db.execute("UPDATE petstats SET abilities = ?, note = ? WHERE spellid = ?",
               (compact(after_kit(cid)), (pet["note"] or "") + NOTE, GHILAN))
    written = row(db, "petstats", {"spellid": GHILAN})
    log(db, "petstats", {"spellid": GHILAN}, pet, written,
        audit(db, "update", "petstats", {"spellid": GHILAN}, pet, written, DETAIL))
    # the spells' id counter, put back by --undo as the rows are (no gm_audit line: not a row of the game's)
    log(db, "sqlite_sequence", {"name": "spellpattern"}, None if seq0 is None else {"name": "spellpattern", "seq": seq0},
        {"name": "spellpattern", "seq": seq(db)}, 0)
    return cid


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for s, tbl, key, before, after in db.execute(f"SELECT seq, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        if tbl not in ("spellpattern", "spelleffect", "petstats", "sqlite_sequence"):
            raise SystemExit(f"{LOG} names table {tbl!r}, which this tool never writes")
        key, after = json.loads(key), json.loads(after)
        now = row(db, tbl, key)
        if now != after:
            left += 1
            print(f"  left: {tbl} {key}, {'gone' if now is None else 'changed since it was written'}")
            continue
        w = " AND ".join(f"{k} = ?" for k in key)
        if before is None:
            db.execute(f"DELETE FROM {tbl} WHERE {w}", list(key.values()))
            if tbl != "sqlite_sequence":
                audit(db, "delete", tbl, key, after, {}, "undo: " + DETAIL)
        else:
            before = json.loads(before)
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in before)} WHERE {w}",
                       list(before.values()) + list(key.values()))
            if tbl != "sqlite_sequence":
                audit(db, "update", tbl, key, after, before, "undo: " + DETAIL)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (s,))
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
        state, refusals, notes = plan(db)
        for r in refusals:
            print("REFUSED:", r)
        if refusals:
            db.execute("ROLLBACK")
            return 1
        for n in notes:
            print(n)
        if state == "done":
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if not a.apply:
            db.execute("ROLLBACK")
            print("report only: 3 rows to write (--apply to write)")
            return 0
        cid = apply(db)
        db.execute("COMMIT")
        print(f"applied: {CURSE_NAME} is spell {cid}; {GHILAN_NAME}'s abilities: {compact(after_kit(cid))}")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
