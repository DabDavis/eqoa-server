#!/usr/bin/env python3
"""Ascended Natureblade at level 60 (Zerkziee's petition 1926, 2026-11-13: "Ranger 60 epic reward ability Ascended
natures blad is lvl 49").

Natureblade (700623) is the ranger's level 60 epic Broken path ability, which hands out the level 49 epic's spell
("Level 60 Epic Broken path ... specialized 49 epic: 250 proc and 150 Int buff", EQOA-Data's ranger.json; questmasters'
id477: "Self, Decrease Hitpoints by 250, Increase INT by 150, 5min"), so its own record is level 49.
tools/ascended_epics_20261111.py made its Ascended copy with its base's level, 49, where every other Ascended reward of a
60 epic's specialized paths is level 60. This gives the copy level 60, the base left as it is:

- spellpattern.abilitylvl 49 -> 60 (what CLIENT's spell book shows; CastSpell.Level, its rank in its line);
- spelleffect.effects' "level" 49 -> 60 (the workbook's stack level, SpellDef.StackLevel: the Ascended copy now
  outranks Natureblade in their line rather than tying it).

Its figures are not touched: the proc is the record's 275 (questmasters' 250 a tenth more) plus 11% of its caster's STR
(10% a tenth more), the INT 165: Zerkziee's procs land 321 every time in gameserver.log (275 + 11% of about 420 STR).

Found by name ("Ascended Natureblade", listed in spellascended with its base "Natureblade"), never by id: Heirloom's is
906732, Live's 906613. --also-broken does the same for the other Ascended copies of a 60 epic reward at level 49 (the
other classes' Broken path abilities, the dry report lists them); left out by default, as the petition names
Natureblade alone.

A report by default; --apply writes, in one transaction with a gm_audit line a row, logged in ascended_lvl60_1113_log;
--undo puts back each row still as written. Spells are read at the server's start: RESTART after --apply.

    python3 -E -s tools/ascended_natureblade_20261113.py --db eqoa.db [--also-broken] [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "ascended_lvl60_1113_log"
DETAIL = "petition 1926 (Zerkziee): the 60 epic reward's Ascended ability at level 60; tools/ascended_natureblade_20261113.py"
WHO, VIA = "dab", "tool"
NAME = "Ascended Natureblade"
FROM, TO = 49, 60
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,     -- spellpattern or spelleffect
    spellid INTEGER NOT NULL,
    before TEXT NOT NULL,     -- the row before (JSON)
    after  TEXT NOT NULL,     -- the row as written (JSON)
    audit  INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def row(db, tbl, sid):
    cur = db.execute(f"SELECT * FROM {tbl} WHERE spellid = ?", (sid,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, tbl, sid, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', 'update', ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, tbl, go_json({"spellid": sid}), go_json(before), go_json(after), DETAIL))
    return cur.lastrowid


def candidates(db):
    """Every Ascended copy at level FROM: (spellid, name, base name)."""
    return db.execute("SELECT p.spellid, p.spellname, b.spellname FROM spellascended a "
                      "JOIN spellpattern p ON p.spellid = a.spellid JOIN spellpattern b ON b.spellid = a.base "
                      "WHERE p.abilitylvl = ? ORDER BY p.spellname", (FROM,)).fetchall()


def effects_at(text, level):
    """The effects JSON with its "level" set, its keys in order; None when it has none or no level of FROM/level."""
    if not (text or "").strip():
        return None
    j = json.loads(text)
    if j.get("level") not in (FROM, level):
        return None
    j["level"] = level
    return json.dumps(j, separators=(",", ":"))


def plan(db, also):
    """(changes [(tbl, sid, before, after)], lines, refusals, applied)."""
    for t in ("spellpattern", "spelleffect", "spellascended", "gm_audit"):
        if not table(db, t):
            return [], [], [f"no {t} table here"], False
    rows = candidates(db)
    lines, refusals, changes = [], [], []
    mine = [r for r in rows if r[1] == NAME]
    others = [r for r in rows if r[1] != NAME]
    done = db.execute("SELECT p.spellid FROM spellpattern p JOIN spellascended a USING (spellid) WHERE p.spellname = ? "
                      "AND p.abilitylvl = ?", (NAME, TO)).fetchone()
    if not mine and done:
        if not also or not others:
            return [], [], [], True
    elif not mine:
        refusals.append(f"no {NAME!r} at level {FROM} listed in spellascended")
    pick = mine + (others if also else [])
    for sid, name, base in pick:
        sp = row(db, "spellpattern", sid)
        after = dict(sp, abilitylvl=TO)
        changes.append(("spellpattern", sid, sp, after))
        lines.append(f"spellpattern {sid} {name!r} (base {base!r}): abilitylvl {FROM} -> {TO}")
        se = row(db, "spelleffect", sid)
        if se is not None:
            eff = effects_at(se.get("effects"), TO)
            if eff is not None and eff != se["effects"]:
                changes.append(("spelleffect", sid, se, dict(se, effects=eff)))
                lines.append(f"spelleffect {sid} {name!r}: effects' level {FROM} -> {TO}")
    if not also and others:
        lines.append(f"left (--also-broken does them): {len(others)} other Ascended copies at level {FROM}: " +
                     ", ".join(f"{n} ({s})" for s, n, _ in others))
    return changes, lines, refusals, False


def apply(db, changes):
    db.execute(LOG_SCHEMA)
    for tbl, sid, before, after in changes:
        if tbl == "spellpattern":
            db.execute("UPDATE spellpattern SET abilitylvl = ? WHERE spellid = ?", (after["abilitylvl"], sid))
        else:
            db.execute("UPDATE spelleffect SET effects = ? WHERE spellid = ?", (after["effects"], sid))
        now = row(db, tbl, sid)
        a = audit(db, tbl, sid, before, now)
        db.execute(f"INSERT INTO {LOG} (tbl, spellid, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                   (tbl, sid, go_json(before), go_json(now), a))
    return len(changes)


def undo(db):
    """(back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for tbl, sid, before, after in db.execute(f"SELECT tbl, spellid, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        was, wrote, now = json.loads(before), json.loads(after), row(db, tbl, sid)
        if now is None or go_json(now) != go_json(wrote):
            left += 1
            continue
        if tbl == "spellpattern":
            db.execute("UPDATE spellpattern SET abilitylvl = ? WHERE spellid = ?", (was["abilitylvl"], sid))
        else:
            db.execute("UPDATE spelleffect SET effects = ? WHERE spellid = ?", (was["effects"], sid))
        audit(db, tbl, sid, now, row(db, tbl, sid))
        back += 1
    if not left:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--also-broken", action="store_true", help="the other Ascended copies at level 49 too")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"undone: {back} rows back" + (f", {left} changed since (left, the log kept)" if left else ""))
            return 0
        changes, lines, refusals, applied = plan(db, a.also_broken)
        if applied:
            db.execute("ROLLBACK")
            print("applied already: nothing to do")
            return 0
        for l in lines:
            print(l)
        if refusals:
            db.execute("ROLLBACK")
            print("REFUSED:\n  " + "\n  ".join(refusals))
            return 1
        if not a.apply:
            db.execute("ROLLBACK")
            print("(report only: --apply to write; RESTART after)")
            return 0
        n = apply(db, changes)
        db.execute("COMMIT")
        print(f"applied: {n} rows, in gm_audit and {LOG}; RESTART the server")
        return 0
    except Exception:
        db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
