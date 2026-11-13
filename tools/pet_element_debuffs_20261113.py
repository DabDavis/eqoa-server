#!/usr/bin/env python3
"""Each master class's elemental pet curses what it fights with a resist debuff of its element (the owner's spell
sheet, "Known Issues Spell_CM sheet", Magician rows 46-47: "Ghilan Noble ... Debuffs mobs cr" and "-35 debuff ability
for each *mancer pet of their element"; the owner approved them 2026-11-13, a follow-up to the magician Waldorf's
ticket 2001, whose fix gave the Ghilan Noble Ghilan's Frost Curse: tools/ghilan_noble_2001_20261113.py).

The "*mancer pets": the four magician master classes' alliance pets, each the pet of its master class's Alliance
ability (EQOAEmu's workbook, CMs sheet: Aeromancer's Djinni Alliance, Summon Djinni Vizier (60); Geomancer's Badi
Alliance, Summon Badi Elder (60); Hydromancer's Ghilani Alliance, Summon Ghilan Noble (60); Pyromancer's Efreeti
Alliance, Summon Efreeti Lord (60); the Pyromancer's text: "They are often accompanied by an Efreeti lord"). The
Ghilan Noble has hers (2001's tool); this one gives the other three theirs. The magician's other elementals (the
summoned elementals by level, Badi/Djinn/Efreet/Ghilan of 49 and 55, the Greater Efreet) are no master class's pet
and get none.

Their elements in this game's resists (fire, cold, lightning, arcane, poison, disease), by each pet's own kit and
row: the Efreeti Lord's fire is FR (its Efreeti's Inferno resisted by fire, its own fire resist 96, its blessing
Efreeti's Fire raising FR); the Djinni Vizier's air is LR, lightning (all three of its nukes, Vizier's Bolt,
Thunderclap and Tempest, resisted by lightning, its own lightning resist 96, its Djinni's Grace raising LR; the
Aeromancer's own Djinn Bolt resisted by lightning too); the Badi Elder's earth is DR, disease (every Earth Strike of the Badi line, its own
Earth Strike Level 55 among them, and the Geomancer's Badi Crush are resisted by disease in the workbook; its row's
PR and DR are its two high resists and Badi's Durability raises both, so the nukes decide). The sheet names none of
the three, so their names are ours, in the Ghilan's frame: Efreeti's Flame Curse, Djinni's Storm Curse, Badi's
Blight Curse.

Each as Ghilan's Frost Curse: "Target, Decrease XR by 35, 1min", the stat line of the resist -35 for 60 s (harmful,
an enemy's, level 60), a 2 s cast, free (a summoned pet's casts cost nothing), recast 30, range 16880, level 60,
the look (fxdict) and icon of the pet's own nuke (Efreeti's Inferno; Vizier's Bolt; the Badi Elder's own workbook
nuke Earth Strike Level 55, off its kit since petition 1450 but its spell still there), first in its kit: cast 1 s
into a fight, kept up (again once it is about to run out, looked at every 30 s; entity/pettemplate.go's
petDebuffLocked, live since build 192). The rest of each kit stays as it is, in order.

Ids differ between shards: each spell is made at the next id of each (spellpattern is AUTOINCREMENT; the id counter
put back by --undo), the pets found by their summoning spell id and name (the same on both). Refused, writing
nothing, unless each pet's abilities are exactly as the live shards hold them (2026-11-13: tools/fix_pets_1110.py
and tools/fix_mage_pets_1450.py), each look spell is there by its id and name, and no spell of a new name is there;
when all three are done it says so and writes nothing; a part done is refused. Report only by default; --apply in
one transaction, a gm_audit line a row, logged in pet_element_debuffs_log; --undo exact (a row changed since is left
and said).

WHEN IT TAKES EFFECT: petstats and the spells its abilities name are read at the server's start (or a GM panel edit
of the table), a pet made at its summoning: run it as the before.sh of a keep restart (with
tools/ghilan_noble_2001_20261113.py if that has not run), on each shard; a pet out then is summoned again to have it.

    tools/pet_element_debuffs_20261113.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "pet_element_debuffs_log"
DETAIL = ("the owner's sheet, Magician rows 46-47 (a follow-up to ticket 2001): each master class pet's -35 resist "
          "debuff of its element: tools/pet_element_debuffs_20261113.py")
WHO, VIA = "claude", "tool"
RESIST, SECONDS, EVERY, CAST, RANGE, LEVEL = 35, 60, 30, 2, 16880, 60
WORDS = {"fr": "fire", "lr": "lightning", "dr": "disease", "cr": "cold"}

# each pet: (summoning spell id, name), its kit as the shards hold it, its nuke (whose look and icon), its curse
PETS = [
    {"pet": 701087, "name": "Efreeti Lord", "stat": "fr", "curse": "Efreeti's Flame Curse",
     "look": (905453, "Efreeti's Inferno"),
     "kit": [{"every": 30.0, "first": 1.0, "kind": "nuke", "spell_id": 905453},
             {"every": 1800.0, "first": 5.0, "kind": "buff", "spell_id": 902164, "target": "self"},
             {"every": 1800.0, "first": 7.0, "kind": "buff", "spell_id": 902164, "target": "owner"}]},
    {"pet": 900761, "name": "Djinni Vizier", "stat": "lr", "curse": "Djinni's Storm Curse",
     "look": (905450, "Vizier's Bolt"),
     "kit": [{"every": 12.0, "first": 1.0, "kind": "nuke", "spell_id": 905450},
             {"every": 12.0, "first": 3.0, "kind": "nuke", "spell_id": 905451},
             {"every": 12.0, "first": 5.0, "kind": "aoe", "spell_id": 905452},
             {"every": 1800.0, "first": 7.0, "kind": "buff", "spell_id": 902161, "target": "self"}]},
    {"pet": 701086, "name": "Badi Elder", "stat": "dr", "curse": "Badi's Blight Curse",
     "look": (902156, "Earth Strike Level 55"),
     "kit": [{"every": 15.0, "first": 1.0, "kind": "taunt", "hits": [[1, 0, 0]]},
             {"every": 1800.0, "first": 5.0, "kind": "buff", "spell_id": 902157, "target": "self"}]},
]


def line(p):
    return f"Target, Decrease {p['stat'].upper()} by {RESIST}, 1min"


def desc(p):
    return (f"The {p['name']}'s curse: {RESIST} off its target's {WORDS[p['stat']]} resist for a minute, the first "
            "thing it casts at it.")


def effects(p):
    return {"v": 1, "scope": "target", "target_type": "Enemy", "harmful": True, "duration_s": SECONDS, "stack": 0,
            "level": LEVEL, "min_level": 1, "req_equipment": "Any", "sheet_type": "Statistic Change",
            "effects": [{"type": "Statistic Change", "kind": "stat", "stat": p["stat"], "amount": -RESIST,
                         "resist": p["stat"]}]}


def source(p):
    return (f"the owner's Known Issues sheet, Magician rows 46-47 (-35 for each *mancer pet of its element); "
            f"{p['look'][1]}'s look (tools/pet_element_debuffs_20261113.py)")


def after_kit(p, curse_id):
    return [{"every": float(EVERY), "first": 1.0, "kind": "debuff", "spell_id": curse_id}] + p["kit"]


def note(p):
    return (f" The owner's sheet (2026-11-13, a follow-up to ticket 2001): {p['curse']} ({RESIST} {p['stat'].upper()} "
            "off its target for a minute) its first cast at a creature (tools/pet_element_debuffs_20261113.py).")


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
    """(state, refusals, notes): state "todo" or "done"."""
    refusals, notes = [], []
    for t in ("spellpattern", "spelleffect", "petstats", "gm_audit"):
        if not table(db, t):
            return None, [f"no {t} table here"], notes
    states = []
    for p in PETS:
        pet = row(db, "petstats", {"spellid": p["pet"]})
        if pet is None or pet["name"] != p["name"]:
            refusals.append(f"petstats {p['pet']} is {pet and pet['name']!r}, not {p['name']!r}")
            continue
        sid, sname = p["look"]
        look = row(db, "spellpattern", {"spellid": sid})
        if look is None or look["spellname"] != sname or row(db, "spelleffect", {"spellid": sid}) is None:
            refusals.append(f"spell {sid} is {look and look['spellname']!r}, not {sname!r} with its effects row")
            continue
        named = db.execute("SELECT spellid FROM spellpattern WHERE lower(trim(spellname)) = ?", (p["curse"].lower(),)).fetchall()
        kit = kit_of(pet["abilities"])
        if len(named) == 1 and kit == after_kit(p, named[0][0]):
            states.append("done")
            notes.append(f"{p['name']} has {p['curse']} ({named[0][0]}) already")
            continue
        if named:
            refusals.append(f"a spell named {p['curse']!r} is there already ({[r[0] for r in named]}), its pet's row "
                            "not this tool's: look first")
        if kit != p["kit"]:
            refusals.append(f"{p['name']}'s abilities are {pet['abilities']}, not as the shards held them on "
                            "2026-11-13 (changed since: a GM's?)")
        states.append("todo")
        notes.append(f"{p['name']} ({p['pet']}): {p['curse']} ({line(p)}, {sname}'s look) first in its kit")
    if refusals:
        return None, refusals, notes
    if all(s == "done" for s in states):
        return "done", refusals, notes
    if "done" in states:
        return None, ["some of the three pets have their curse and some not: look first"], notes
    return "todo", refusals, notes


def seq(db):
    r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'spellpattern'").fetchone()
    return None if r is None else r[0]


def log(db, tbl, key, before, after, a):
    db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
               (tbl, json.dumps(key, sort_keys=True), None if before is None else json.dumps(before, sort_keys=True),
                json.dumps(after, sort_keys=True), a))


def apply(db):
    """Writes the rows; returns each curse's id by pet name."""
    db.execute(LOG_SCHEMA)
    seq0 = seq(db)
    made = {}
    for p in PETS:
        look = row(db, "spellpattern", {"spellid": p["look"][0]})
        look_fx = row(db, "spelleffect", {"spellid": p["look"][0]})["fxdict"]
        pattern = {"abilitylvl": LEVEL, "unk2": 0, "unk3": 0, "range": RANGE, "casttime": CAST, "power": 0,
                   "iconColor": look["iconColor"], "icon": look["icon"], "scope": look["scope"], "recast": EVERY,
                   "eqprequire": -1, "spellname": p["curse"], "spelldesc": desc(p)}
        cols = list(pattern)
        cid = db.execute(f"INSERT INTO spellpattern ({', '.join(cols)}) VALUES ({', '.join('?' for _ in cols)})",
                         [pattern[c] for c in cols]).lastrowid
        written = row(db, "spellpattern", {"spellid": cid})
        log(db, "spellpattern", {"spellid": cid}, None, written,
            audit(db, "insert", "spellpattern", {"spellid": cid}, None, written, DETAIL))
        effect = {"spellid": cid, "line": line(p), "fxdict": look_fx, "source": source(p),
                  "effects": compact(effects(p))}
        cols = list(effect)
        db.execute(f"INSERT INTO spelleffect ({', '.join(cols)}) VALUES ({', '.join('?' for _ in cols)})",
                   [effect[c] for c in cols])
        written = row(db, "spelleffect", {"spellid": cid})
        log(db, "spelleffect", {"spellid": cid}, None, written,
            audit(db, "insert", "spelleffect", {"spellid": cid}, None, written, DETAIL))
        pet = row(db, "petstats", {"spellid": p["pet"]})
        db.execute("UPDATE petstats SET abilities = ?, note = ? WHERE spellid = ?",
                   (compact(after_kit(p, cid)), (pet["note"] or "") + note(p), p["pet"]))
        written = row(db, "petstats", {"spellid": p["pet"]})
        log(db, "petstats", {"spellid": p["pet"]}, pet, written,
            audit(db, "update", "petstats", {"spellid": p["pet"]}, pet, written, DETAIL))
        made[p["name"]] = cid
    # the spells' id counter, put back by --undo as the rows are (no gm_audit line: not a row of the game's)
    log(db, "sqlite_sequence", {"name": "spellpattern"}, None if seq0 is None else {"name": "spellpattern", "seq": seq0},
        {"name": "spellpattern", "seq": seq(db)}, 0)
    return made


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
            print(f"report only: {3 * len(PETS)} rows to write (--apply to write)")
            return 0
        made = apply(db)
        db.execute("COMMIT")
        print("applied: " + "; ".join(f"{n}'s curse is spell {c}" for n, c in made.items()))
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
