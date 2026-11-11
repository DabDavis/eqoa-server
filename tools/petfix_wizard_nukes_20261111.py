#!/usr/bin/env python3
"""The wizard's single-target nukes: a 1 second cast, and ready again 1 second after they land (petition 1584, Wisp,
2026-11-11: "all the wizzy single target spells have wrong recast they should be 1 sec cast 1 second recast, recast
starts after spell is cast so you have to chain 2 spells back and forth"; petition 1590, the GM Jibmonk: "They were 1
second cast time, and would come off cool down 1 second after they fired. ... Don't change our recast calculation at
all, but verify the non mca wizard single target nukes only (not AoE), have that same behavior").

The server's recast (entity/cast.go, not changed): a cast locks its ability out for cast time + recast + 300 ms from
the moment it begins (Sandstorm's lockout, recorded 2026-10-21). With casttime 1 and recast 1 a nuke goes off 1 s after
it begins and may begin again 2.3 s after it began: 1 s (and Sandstorm's 0.3 s) after it went off, the behaviour asked.
Before, most had casttime 2 (EQOAEmu's workbook and the import: 2 s cast, recast 1 or 3), so they went off at 2 s and
were ready again at 3.3 s (recast 1) or 5.3 s (recast 3).

Which spells: every wizard spell (a classspells row of class 13) of these names, the wizard's direct damage on one
target from a merchant's, a quest's or an epic's scroll. Left out: the masteries' and the master classes' (the Fury
line, Clash +, the Icy/Static/Fiery Explode-to-Supernova records, Mana Blast, Arcane Shock, Icy Fury, ...: "outside of
MCA abilities"), the area spells (storms, maelstroms, Pillar of Flame, Lightning Column, the Fingers), the pure hate
spells (Clash, Shock, Stupefy, Concussion, Impact) and roots, and the big-cooldown nukes of 180 s (Explode, Detonate,
Annihilate, Nova, Freezing Nova, Lightning Nova, Supernova, Manaburn): a nuke of 2,100-4,300 every 2.3 s is not what
either petition describes, so they are named in the report for the GMs and not changed.

Records found by name among the wizard's classspells rows, so the Live shard's are found as Heirloom's; a name with no
wizard record there is reported and skipped. Refused, nothing written, if a record found is not a single-target harmful
spell (spellpattern.scope 1 and, where it has workbook effects, scope "target"). Report only by default (each spell,
before and after); --apply in one transaction, a gm_audit line a record, log wizard_nukes_1111_log; --undo exact (a
record changed since is left and said). A cast reads its record at the cast: the server needs no restart; a client
shows the new cast time from its next login (the book is sent then).

    tools/petfix_wizard_nukes_20261111.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "wizard_nukes_1111_log"
DETAIL = ("petitions 1584 (Wisp) and 1590 (Jibmonk): the wizard's single-target nukes cast in 1 s and ready 1 s after "
          "they land: tools/petfix_wizard_nukes_20261111.py")
WHO, VIA = "claude", "tool"
WIZARD = 13
CAST, RECAST = 1, 1
NUKES = [
    # the bolts, shocks, gazes and strikes (merchants' and quests' scrolls)
    "Static Bolt", "Fire Bolt", "Shock of Frost", "Static Shock", "Chilling Gaze", "Shocking Gaze", "Burning Gaze",
    "Shocking Strike", "Freezing Strike", "Burning Strike", "Lightning Blast", "Incineration", "Electrocution",
    "Ice Comet", "Thunderbolt", "Sunstrike",
    # the clash path's nukes (damage and hate shed on one target; quests' scrolls)
    "Blazing Clash", "Static Clash", "Freezing Clash", "Burning Clash", "Shocking Clash", "Chilling Clash",
    # the level 60 ones (the Archivist's scrolls and the epic's options)
    "Vylinnuza's Merciless Inferno", "Vylinnuza's Merciless Blizzard", "Vylinnuza's Tidal Destruction",
    "Vylinnuza's Merciless Undertow",
]
LEFT_OUT = ["Explode", "Detonate", "Annihilate", "Nova", "Freezing Nova", "Lightning Nova", "Supernova", "Manaburn"]
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    spellid INTEGER NOT NULL,
    before  TEXT NOT NULL,      -- {{"casttime":..,"recast":..}} before
    after   TEXT NOT NULL,      -- as written
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def row_of(db, id):
    cur = db.execute("SELECT * FROM spellpattern WHERE spellid = ?", (id,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def audit(db, id, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', 'update', 'spellpattern', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, go_json({"spellid": id}), go_json(before), go_json(after), detail))
    return cur.lastrowid


def wizard_records(db, name):
    return [r[0] for r in db.execute(
        "SELECT p.spellid FROM spellpattern p JOIN classspells c ON c.spellid = p.spellid AND c.class = ? "
        "WHERE p.spellname = ? ORDER BY p.spellid", (WIZARD, name))]


def plan(db):
    """(changes [(spellid, name, level, (ct, rc))], refusals, notes, done)."""
    changes, refusals, notes, done = [], [], [], 0
    for t in ("spellpattern", "classspells"):
        if not table(db, t):
            return changes, [f"no {t} table here"], notes, 0
    effects = table(db, "spelleffect")
    for name in NUKES:
        ids = wizard_records(db, name)
        if not ids:
            notes.append(f"{name}: no wizard record here, skipped")
            continue
        for id in ids:
            r = row_of(db, id)
            if r["scope"] != 1:
                refusals.append(f"{name} ({id}): scope {r['scope']}, not a single target's")
                continue
            if effects:
                e = db.execute("SELECT effects FROM spelleffect WHERE spellid = ?", (id,)).fetchone()
                if e and e[0]:
                    ej = json.loads(e[0])
                    if ej.get("scope") != "target" or not ej.get("harmful"):
                        refusals.append(f"{name} ({id}): effects scope {ej.get('scope')!r}, harmful {ej.get('harmful')!r}")
                        continue
            if (r["casttime"], r["recast"]) == (CAST, RECAST):
                done += 1
            else:
                changes.append((id, name, r["abilitylvl"], (r["casttime"], r["recast"])))
    for name in LEFT_OUT:
        for id in wizard_records(db, name):
            r = row_of(db, id)
            notes.append(f"left out: {name} ({id}), cast {r['casttime']} s, recast {r['recast']} s (a big-cooldown nuke)")
    return changes, refusals, notes, done


def apply(db, changes):
    db.execute(LOG_SCHEMA)
    for id, _, _, (ct, rc) in changes:
        b = row_of(db, id)
        db.execute("UPDATE spellpattern SET casttime = ?, recast = ? WHERE spellid = ?", (CAST, RECAST, id))
        a = audit(db, id, b, row_of(db, id), DETAIL)
        db.execute(f"INSERT INTO {LOG} (spellid, before, after, audit) VALUES (?, ?, ?, ?)",
                   (id, json.dumps({"casttime": ct, "recast": rc}), json.dumps({"casttime": CAST, "recast": RECAST}), a))


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, id, before, after in db.execute(f"SELECT seq, spellid, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        now, b, w = row_of(db, id), json.loads(before), json.loads(after)
        if now is None or (now["casttime"], now["recast"]) != (w["casttime"], w["recast"]):
            left += 1
            print(f"  left: spellpattern {id}, changed since: {None if now is None else (now['casttime'], now['recast'])}")
            continue
        db.execute("UPDATE spellpattern SET casttime = ?, recast = ? WHERE spellid = ?", (b["casttime"], b["recast"], id))
        audit(db, id, now, row_of(db, id), "undo: " + DETAIL)
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
        changes, refusals, notes, done = plan(db)
        for r in refusals:
            print("REFUSED:", r)
        if refusals:
            db.execute("ROLLBACK")
            return 1
        for id, name, lv, (ct, rc) in changes:
            print(f"spellpattern {id} {name} (level {lv}): cast {ct} s, recast {rc} s -> cast {CAST} s, recast {RECAST} s "
                  f"(goes off at {CAST} s, ready again {CAST + RECAST + 0.3:.1f} s after it began; before at {ct} s and "
                  f"{ct + rc + 0.3:.1f} s)")
        for n in notes:
            print(n)
        if not changes:
            db.execute("ROLLBACK")
            print(f"applied already: {done} records cast in {CAST} s with recast {RECAST} s; nothing done")
            return 0
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"report only: {len(changes)} records would change, {done} are so already (--apply to write)")
            return 0
        apply(db, changes)
        db.execute("COMMIT")
        print(f"applied: {len(changes)} records")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
