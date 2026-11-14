#!/usr/bin/env python3
"""Ascended Severupidus' Fire Potion makes potions (Fullmetal's petition 2059, 2026-11-14: "Alc 60 epic ability gave
original plus ascended. Ascended needs to be fixed to create a potion, currently it is a nuke with pot making
animation").

The alchemist's 60 epic reward option "Ascended Severupidus' Cocktail of Torment" (quests 101121 and 101133, step 1's
choices) gives two spells: Ascended Severupidus' Cocktail of Torment, which summons three Ascended Severupidus'
Cocktails, and Ascended Severupidus' Fire Potion, the Ascended copy (tools/ascended_epics_20261111.py) of the castable
throw made for petition 1181: a nuke of 1,980 fire on the target and all within 10, cast with the alchemists' potion
visual. The petition asks for the second to make a potion. There is no stronger potion than the Ascended Severupidus'
Cocktail (1,980, the base's 1,800 a tenth more), so the Fire Potion is made to summon it as the Cocktail of Torment
does: its spellpattern takes the summon's casting (scope, range, cast time, power, recast, icon, equipment) and a
description, its spelleffect the summon's line, visual and effects (create_item: three Ascended Severupidus'
Cocktails). Its name and level stay. The base pair (Severupidus' Cocktail of Torment and Severupidus' Fire Potion, the
non-Ascended option) is left as it is.

Both found by name, never by id (Heirloom 906808 from 906789, Live 906689 from 906670); refused when a name is not
exactly one record or the Fire Potion is not the nuke this was written against. A report by default; --apply writes, in
one transaction with a gm_audit line a row, logged in alc_fire_potion_1114_log; --undo puts back each row still as
written. Spells are read at the server's start: RESTART after --apply; a character's book shows the change from its
next login.

    python3 -E -s tools/alc_fire_potion_summon_20261114.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "alc_fire_potion_1114_log"
DETAIL = ("petition 2059 (Fullmetal): Ascended Severupidus' Fire Potion summons Ascended Severupidus' Cocktails as the "
          "Cocktail of Torment does; tools/alc_fire_potion_summon_20261114.py")
WHO, VIA = "dab", "tool"
FIX = "Ascended Severupidus' Fire Potion"
FROM = "Ascended Severupidus' Cocktail of Torment"
POTION = "Ascended Severupidus' Cocktail"
PATTERN_COPY = ("unk2", "unk3", "range", "casttime", "power", "iconColor", "icon", "scope", "recast", "eqprequire")
EFFECT_COPY = ("line", "fxdict", "effects")
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


def one(db, name, refusals):
    ids = [r[0] for r in db.execute("SELECT spellid FROM spellpattern WHERE spellname = ?", (name,))]
    if len(ids) != 1:
        refusals.append(f"spellpattern has {len(ids)} records named {name!r}, want one")
        return None
    return ids[0]


def kinds(effects):
    try:
        return [e.get("kind") for e in json.loads(effects or "{}").get("effects", [])]
    except ValueError:
        return []


def plan(db):
    """(changes [(tbl, sid, before, after)], lines, refusals, applied)."""
    for t in ("spellpattern", "spelleffect", "gm_audit"):
        if not table(db, t):
            return [], [], [f"no {t} table here"], False
    refusals = []
    fix, src = one(db, FIX, refusals), one(db, FROM, refusals)
    if refusals:
        return [], [], refusals, False
    sp_fix, sp_src = row(db, "spellpattern", fix), row(db, "spellpattern", src)
    se_fix, se_src = row(db, "spelleffect", fix), row(db, "spelleffect", src)
    if se_fix is None or se_src is None:
        return [], [], [f"no spelleffect row for {FIX!r} ({fix}) or {FROM!r} ({src})"], False
    if kinds(se_src["effects"]) != ["create_item"] or POTION not in se_src["effects"]:
        return [], [], [f"{FROM!r} ({src}) does not summon {POTION!r}: {se_src['effects']}"], False
    desc = "Ascended (+10%): " + se_src["line"]
    want_sp = dict(sp_fix, **{k: sp_src[k] for k in PATTERN_COPY}, spelldesc=desc)
    want_se = dict(se_fix, **{k: se_src[k] for k in EFFECT_COPY})
    if sp_fix == want_sp and se_fix == want_se:
        return [], [], [], True
    if kinds(se_fix["effects"]) != ["hp2"]:
        return [], [], [f"{FIX!r} ({fix}) is not the nuke this was written against: {se_fix['effects']}"], False
    changes, lines = [], []
    if sp_fix != want_sp:
        changes.append(("spellpattern", fix, sp_fix, want_sp))
        diff = ", ".join(f"{k} {sp_fix[k]!r} -> {want_sp[k]!r}" for k in sp_fix if sp_fix[k] != want_sp[k])
        lines.append(f"spellpattern {fix} {FIX!r} (from {src}): {diff}")
    if se_fix != want_se:
        changes.append(("spelleffect", fix, se_fix, want_se))
        lines.append(f"spelleffect {fix} {FIX!r}: line {se_fix['line']!r} -> {want_se['line']!r}; effects -> {want_se['effects']}")
    return changes, lines, [], False


def write(db, tbl, sid, values):
    cols = [k for k in values if k != "spellid"]
    db.execute(f"UPDATE {tbl} SET " + ", ".join(f"{c} = ?" for c in cols) + " WHERE spellid = ?",
               [values[c] for c in cols] + [sid])


def apply(db, changes):
    db.execute(LOG_SCHEMA)
    for tbl, sid, before, after in changes:
        write(db, tbl, sid, after)
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
        write(db, tbl, sid, was)
        audit(db, tbl, sid, now, row(db, tbl, sid))
        back += 1
    if not left:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
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
        changes, lines, refusals, applied = plan(db)
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
