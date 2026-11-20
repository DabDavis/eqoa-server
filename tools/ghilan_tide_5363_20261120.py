#!/usr/bin/env python3
"""The Ghilan Noble's healing tide at the pace of her other heals (Live petition 5363, 2026-11-20: "Ghilan Noble,
lvl 60 Hydromancer pet, spam casts its AOE heal every 5 ish secs").

Ghilan's Healing Tide (905495, 150 on everyone of her side in reach: entity/petgroupheal.go) was given her by
tools/fix_pets_1110.py (petition 1495) as an ability every 6 s while anyone of her side is below full, the 6 s ours
(the petition gave the figure, not how often). In any fight someone is scratched, so she cast it every 6 s. It is
made one of her heals as her others are: every 30 s (her Mending's and the Water Heals' recast), and only when one of
her side is below 90% (not for a scratch). The spell's own recast goes to 30 with it.

Both shards: the spell and the pet have the same ids there (905495, 701088). Refused, writing nothing, unless her
kit holds the tide as fix_pets_1110.py left it (every 6, below 100, first 2, target group). Report only by default;
--apply in one transaction, a gm_audit line a row, logged in ghilan_5363_log; a second --apply finds it done;
--undo exact (a row changed since is left and said).

WHEN IT TAKES EFFECT: petstats and spells are read at the server's start; a Ghilan Noble summoned after the next
restart has it (one out then keeps the old pace until summoned again).

    tools/ghilan_tide_5363_20261120.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "ghilan_5363_log"
DETAIL = "petition 5363: the Ghilan Noble's healing tide every 30 s below 90%: tools/ghilan_tide_5363_20261120.py"
WHO, VIA = "claude", "tool"

GHILAN, GHILAN_NAME = 701088, "Ghilan Noble"
TIDE, TIDE_NAME = 905495, "Ghilan's Healing Tide"
OLD = {"every": 6.0, "first": 2.0, "kind": "heal", "spell_id": TIDE, "target": "group", "below": 100}
NEW = {"every": 30.0, "first": 2.0, "kind": "heal", "spell_id": TIDE, "target": "group", "below": 90}
OLD_RECAST, NEW_RECAST = 6, 30

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,      -- spellpattern | petstats
    rowkey  TEXT NOT NULL,      -- the row's key (JSON)
    before  TEXT NOT NULL,      -- the whole row as it was (JSON)
    after   TEXT NOT NULL       -- the whole row as written (JSON)
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
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key), go_json(before), go_json(after), detail))


def row(db, tbl, key):
    w = " AND ".join(f"{k} = ?" for k in key)
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {w}", list(key.values()))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def new_kit(kit):
    return [NEW if a == OLD else a for a in kit]


def plan(db):
    """(state, refusals, notes): state "todo", "done" or None (refused)."""
    for t in ("spellpattern", "petstats", "gm_audit"):
        if not table(db, t):
            return None, [f"no {t} table here"], []
    pet = row(db, "petstats", {"spellid": GHILAN})
    if pet is None or pet["name"] != GHILAN_NAME:
        return None, [f"petstats {GHILAN} is {pet and pet['name']!r}, not {GHILAN_NAME!r}"], []
    spell = row(db, "spellpattern", {"spellid": TIDE})
    if spell is None or spell["spellname"] != TIDE_NAME:
        return None, [f"spell {TIDE} is {spell and spell['spellname']!r}, not {TIDE_NAME!r}"], []
    try:
        kit = json.loads(pet["abilities"] or "[]")
    except ValueError:
        return None, [f"{GHILAN_NAME}'s abilities are not JSON: {pet['abilities']!r}"], []
    olds, news = kit.count(OLD), kit.count(NEW)
    if olds == 0 and news == 1 and spell["recast"] == NEW_RECAST:
        return "done", [], [f"{GHILAN_NAME}'s tide is every {NEW['every']:g} s below {NEW['below']}% already"]
    refusals = []
    if olds != 1 or news != 0:
        refusals.append(f"{GHILAN_NAME}'s abilities hold the tide as {[a for a in kit if a.get('spell_id') == TIDE]}, "
                        f"not once as {OLD} (changed since: a GM's?)")
    if spell["recast"] not in (OLD_RECAST, NEW_RECAST):
        refusals.append(f"{TIDE_NAME}'s recast is {spell['recast']}, not {OLD_RECAST}")
    if refusals:
        return None, refusals, []
    return "todo", [], [f"{GHILAN_NAME} ({GHILAN}): {TIDE_NAME} every {OLD['every']:g} s below {OLD['below']}% -> "
                        f"every {NEW['every']:g} s below {NEW['below']}%; its recast {spell['recast']} -> {NEW_RECAST}"]


def write(db, tbl, key, sets):
    before = row(db, tbl, key)
    w = " AND ".join(f"{k} = ?" for k in key)
    db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in sets)} WHERE {w}",
               list(sets.values()) + list(key.values()))
    after = row(db, tbl, key)
    if after == before:
        return
    db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES (?, ?, ?, ?)",
               (tbl, json.dumps(key, sort_keys=True), json.dumps(before, sort_keys=True), json.dumps(after, sort_keys=True)))
    audit(db, "update", tbl, key, before, after, DETAIL)


def apply(db):
    db.execute(LOG_SCHEMA)
    pet = row(db, "petstats", {"spellid": GHILAN})
    write(db, "petstats", {"spellid": GHILAN}, {"abilities": compact(new_kit(json.loads(pet["abilities"])))})
    write(db, "spellpattern", {"spellid": TIDE}, {"recast": NEW_RECAST})


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for s, tbl, key, before, after in db.execute(f"SELECT seq, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        if tbl not in ("spellpattern", "petstats"):
            raise SystemExit(f"{LOG} names table {tbl!r}, which this tool never writes")
        key, before, after = json.loads(key), json.loads(before), json.loads(after)
        now = row(db, tbl, key)
        if now != after:
            left += 1
            print(f"  left: {tbl} {key}, {'gone' if now is None else 'changed since it was written'}")
            continue
        w = " AND ".join(f"{k} = ?" for k in key)
        db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in before)} WHERE {w}",
                   list(before.values()) + list(key.values()))
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
        for n in notes:
            print(n)
        if state != "todo" or not a.apply:
            db.execute("ROLLBACK")
            print({None: "", "done": "nothing to do"}.get(state, "report only (--apply to write)"))
            return 1 if state is None else 0
        apply(db)
        db.execute("COMMIT")
        print("applied")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
