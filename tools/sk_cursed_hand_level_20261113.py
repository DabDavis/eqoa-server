#!/usr/bin/env python3
"""The shadow knight's Cursed Hand at level 4 (Smoke's Live petition 5005, 2026-11-13: "ok so Veil of death is level 4 and
cursed hand is level 12!").

Every source puts the two at 4 and 12: EQOA-Data's shadowknight.json (Cursed Hand 4, Veil of Death 12), questmasters'
Shadowknight Spells page (id471: "LEVEL 4 Cursed Hand", "LEVEL 12 ... Veil of Death") and Allakhazam's Shadowknight list
(Cursed Hand 4, its proc "Cursed Hand Effect" 3; Veil of Death 12, its effect 9). Both shards have Veil of Death (901430)
and Veil of Death + (1431) at 12, as the sources do, and Cursed Hand (1422, the old dump's record) at 3: its EFFECT's level,
not the ability's. This puts Cursed Hand at 4, where its scroll is sold and scribed too:

- spellpattern "Cursed Hand" (class 3's): abilitylvl 3 -> 4;
- its spelleffect: the effects' "level" 3 -> 4 (its stack level);
- its scroll, itempattern "Cursed Hand" (shadow knights', classuse 8): levelreq 3 -> 4 and its description's
  "Level 3 - Offensive Proc" -> "Level 4 - Offensive Proc".

Veil of Death is left at 12: no source has it at 4 (the petition's words read the two the other way round from every
source; the open question is said in the report). Everything found by name and checked as it was found, else refused,
nothing written. Report by default; --apply in one transaction, a gm_audit line a row, logged in sk_cursed_hand_1113_log;
--undo puts back each row still as written. Spells and items are read at the server's start: RESTART after --apply. Run
it on each shard.

    python3 -E -s tools/sk_cursed_hand_level_20261113.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "sk_cursed_hand_1113_log"
DETAIL = "Live petition 5005 (Smoke): Cursed Hand at level 4, as every source has it; tools/sk_cursed_hand_level_20261113.py"
WHO, VIA = "dab", "tool"
KEYS = {"spellpattern": "spellid", "spelleffect": "spellid", "itempattern": "patternid"}


# ---- a small writer: rows by key, each change a gm_audit line and a log row, undone only where still as written ----

def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def row(db, tbl, key):
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {KEYS[tbl]} = ?", (key,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, detail, tbl, key, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', 'update', ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, tbl, go_json({KEYS[tbl]: key}), go_json(before), go_json(after), detail))
    return cur.lastrowid


def log_schema(log):
    return f"""CREATE TABLE IF NOT EXISTS {log} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    rowkey INTEGER NOT NULL,
    cols   TEXT NOT NULL,     -- the columns changed, {{col: [before, after]}} (JSON)
    audit  INTEGER NOT NULL DEFAULT 0
)"""


def write(db, log, detail, changes):
    """changes: [(tbl, key, {col: new})]. Each a gm_audit line and a log row."""
    db.execute(log_schema(log))
    for tbl, key, cols in changes:
        before = row(db, tbl, key)
        sets = ", ".join(f"{c} = ?" for c in cols)
        db.execute(f"UPDATE {tbl} SET {sets} WHERE {KEYS[tbl]} = ?", [*cols.values(), key])
        after = row(db, tbl, key)
        a = audit(db, detail, tbl, key, before, after)
        db.execute(f"INSERT INTO {log} (tbl, rowkey, cols, audit) VALUES (?, ?, ?, ?)",
                   (tbl, key, json.dumps({c: [before[c], after[c]] for c in cols}), a))
    return len(changes)


def unwrite(db, log, detail):
    """(back, left: [why])."""
    if not table(db, log):
        return 0, []
    back, left = 0, []
    for tbl, key, cols in db.execute(f"SELECT tbl, rowkey, cols FROM {log} ORDER BY seq DESC").fetchall():
        cols = json.loads(cols)
        now = row(db, tbl, key)
        if now is None or any(now[c] != ba[1] for c, ba in cols.items()):
            left.append(f"{tbl} {key}: changed since")
            continue
        db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in cols)} WHERE {KEYS[tbl]} = ?",
                   [*(ba[0] for ba in cols.values()), key])
        audit(db, detail, tbl, key, now, row(db, tbl, key))
        back += 1
    if not left:
        db.execute(f"DROP TABLE {log}")
    return back, left


def run(plan, log, detail, doc):
    """The command line every tool of this file's kind has: report, --apply, --undo."""
    ap = argparse.ArgumentParser(description=doc.split("\n")[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = unwrite(db, log, detail)
            db.execute("COMMIT")
            print(f"undone: {back} rows back" + "".join(f"\n  left: {w}" for w in left))
            return 0 if not left else 1
        if table(db, log):
            db.execute("ROLLBACK")
            print(f"applied already ({log} is there): --undo first")
            return 0
        changes, lines, refusals = plan(db)
        for line in lines:
            print(line)
        if refusals:
            db.execute("ROLLBACK")
            print("REFUSED, nothing written:\n  " + "\n  ".join(refusals))
            return 1
        if not changes:
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"(report only: {len(changes)} rows; --apply to write, then RESTART)")
            return 0
        n = write(db, log, detail, changes)
        db.execute("COMMIT")
        print(f"applied: {n} rows, in gm_audit and {log}; RESTART the server")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


# ---- Cursed Hand -----------------------------------------------------------------------------------------------------

FROM, TO = 3, 4
DESC_FROM, DESC_TO = "Level 3 - Offensive Proc", "Level 4 - Offensive Proc"


def plan(db):
    """(changes, lines, refusals)."""
    for t in ("spellpattern", "spelleffect", "itempattern", "classspells", "gm_audit"):
        if not table(db, t):
            return [], [], [f"no {t} table here"]
    changes, lines, refusals = [], [], []
    sp = db.execute("SELECT p.spellid, p.abilitylvl FROM spellpattern p JOIN classspells c ON c.spellid = p.spellid AND c.class = 3 "
                    "WHERE p.spellname = 'Cursed Hand'").fetchall()
    if len(sp) != 1:
        return [], [], [f"{len(sp)} shadow knight spells named 'Cursed Hand', want one"]
    sid, lvl = sp[0]
    if lvl != FROM:
        refusals.append(f"spellpattern {sid} 'Cursed Hand': level {lvl}, not {FROM}")
    else:
        changes.append(("spellpattern", sid, {"abilitylvl": TO}))
        lines.append(f"spellpattern {sid} 'Cursed Hand': abilitylvl {FROM} -> {TO}")
    se = row(db, "spelleffect", sid)
    if se is not None and (se.get("effects") or "").strip():
        j = json.loads(se["effects"])
        if j.get("level") == FROM:
            j["level"] = TO
            changes.append(("spelleffect", sid, {"effects": json.dumps(j, separators=(",", ":"))}))
            lines.append(f"spelleffect {sid}: the effects' level {FROM} -> {TO}")
        elif j.get("level") != TO:
            refusals.append(f"spelleffect {sid}: the effects' level {j.get('level')}, not {FROM}")
    items = db.execute("SELECT patternid, levelreq, itemdesc FROM itempattern WHERE itemname = 'Cursed Hand'").fetchall()
    for pid, req, desc in items:
        if req != FROM or DESC_FROM not in (desc or ""):
            refusals.append(f"itempattern {pid} 'Cursed Hand': level {req}, description {desc!r}: not as found")
            continue
        changes.append(("itempattern", pid, {"levelreq": TO, "itemdesc": desc.replace(DESC_FROM, DESC_TO)}))
        lines.append(f"itempattern {pid} 'Cursed Hand' (its scroll): levelreq {FROM} -> {TO}, {DESC_FROM!r} -> {DESC_TO!r}")
    veil = db.execute("SELECT spellid, spellname, abilitylvl FROM spellpattern WHERE spellname LIKE 'Veil of Death%'").fetchall()
    lines.append("left: " + ", ".join(f"{n} ({s}) level {l}" for s, n, l in veil) + " (every source: 12)")
    return changes, lines, refusals


if __name__ == "__main__":
    sys.exit(run(plan, LOG, DETAIL, __doc__))
