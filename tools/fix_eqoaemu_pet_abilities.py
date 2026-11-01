#!/usr/bin/env python3
"""The workbook pets' abilities as tools/import_eqoaemu_pets.py makes them now,
on a server the pet import already filled (2026-11-01, the user's asks of the
day, after the import of 10:20):

  - a drain of the enemy's stats a life tap of them (the necromancer's pets'
    Undead Bane, from the level 30 pet): the import left it out, a creature's
    stats counting in no fight then ("necromancers mummy and higher pets have
    a drain"); its records of no time given the first rank's (the sheet gives
    one to the level 30 rank alone, 60 s; ours: a drain of no time takes
    nothing that lasts);
  - a root at the enemy a root (the Djinni Vizier's Air Root), left out for
    want of a pet's kind ("fix them all");
  - the magician's pets' buffs on themselves, not their owner as the sheet's
    "Owner" has them ("the pets buff themselves not the player, the water
    pets will heal the player though"; the alchemist's and wizard's pets
    still buff their owner);
  - a pet spell the sheet names in error put right by the operator's file of
    fixes (--pet-fixes, as the importer reads it: the Ghilan Noble's water
    nuke of her level, "use her water nuke 55").

Each workbook pet's petstats row whose abilities are still as the import wrote
them (its log, eqoaemu_pets_log) takes the importer's abilities now; a row a
GM changed since is left and said. Every row changed has its gm_audit line, as
the import's, so the GM panel lists and reverts it, and its own line in this
tool's log for --undo.

The sheets and the fixes are the operator's and not on the box: --save-plan
FILE here, --plan FILE --apply there.

    tools/fix_eqoaemu_pet_abilities.py --db copy.db --sheets <dir> --spellmap <map.tsv> --pet-fixes <fixes.tsv>   # report
    tools/fix_eqoaemu_pet_abilities.py ... --save-plan plan.json
    tools/fix_eqoaemu_pet_abilities.py --db eqoa.db --plan plan.json --apply      # one transaction, logged
    tools/fix_eqoaemu_pet_abilities.py --db eqoa.db --undo                        # each row still as written back
"""
import argparse
import json
import os
import sqlite3
import sys
import types

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import import_eqoaemu_pets as pets  # noqa: E402  the importer's rules, one place

LOG = "eqoaemu_pet_abilities_log"
DEFAULT_DB = "eqoa.db"
DETAIL = "the workbook pets' abilities as the import makes them now: tools/fix_eqoaemu_pet_abilities.py"


def stat_drain(sheet_spell, effects):
    """Whether a pet's sheet spell is a drain of the enemy's stats (no hit)."""
    effs = pets.effect_rows(sheet_spell, effects)
    if not effs:
        return False
    e = effs[0]
    return pets.fold(e.get("TYPE")) == "drain" and (pets.num(e.get("HP1")) or 0) == 0


def written_abilities(db):
    """Each petstats row's abilities as the pet import last wrote them, by spellid."""
    out = {}
    if not pets.table(db, pets.LOG):
        return out
    for rk, after in db.execute(f"SELECT rowkey, after FROM {pets.LOG} WHERE tbl = 'petstats' ORDER BY id"):
        out[json.loads(rk)["spellid"]] = json.loads(after).get("abilities")
    return out


def make_plan(db, sheets, smap, fixes):
    """The rows to change (spellid, the abilities now, the importer's), the drains to time, and notes."""
    rows, _, _, _ = pets.plan(db, sheets, smap, fixes=fixes)
    written = written_abilities(db)
    out, notes = [], []
    for r in rows:
        sid = r["spellid"]
        cur = db.execute("SELECT abilities, name FROM petstats WHERE spellid = ?", (sid,)).fetchone()
        if cur is None or cur[0] == r["abilities"]:
            continue
        if sid not in written or written[sid] != cur[0]:
            notes.append(f"{cur[1]!r} ({sid}): its abilities changed since the import (a GM's): left")
            continue
        out.append(dict(spellid=sid, name=cur[1], before=cur[0], after=r["abilities"]))
    # each drain of stats of no time: the first rank's (the sheet's one time)
    spells = {s["SpellID"].strip().upper(): s for s in sheets["Spells"] if s.get("SpellID", "").strip()}
    effects = {e["EffectID"].strip().upper(): e for e in sheets["Effects"] if e.get("EffectID", "").strip()}
    drains = {}
    for p in sheets["Pets"]:
        for k in ("Spell1", "Spell2", "Spell3", "Spell4"):
            ref = (p.get(k) or "").strip().upper()
            s = spells.get(ref)
            if s is not None and ref in smap and stat_drain(s, effects):
                drains.setdefault(smap[ref], s)
    line_time = max(((pets.num(s.get("Duration")) or 0) for s in drains.values()), default=0)
    timed = []
    for sid in sorted(drains):
        if (pets.num(drains[sid].get("Duration")) or 0) > 0 or line_time <= 0:
            continue
        r = db.execute("SELECT effects FROM spelleffect WHERE spellid = ?", (sid,)).fetchone()
        if r is None or not r[0]:
            notes.append(f"{drains[sid].get('Name')!r} ({sid}): no record in spelleffect")
            continue
        if (json.loads(r[0]).get("duration_s") or 0) > 0:
            continue
        timed.append(dict(spellid=sid, spell=drains[sid].get("Name"), duration_s=int(line_time)))
    return {"rows": out, "times": timed}, notes


LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,      -- petstats or spelleffect
    spellid INTEGER NOT NULL,
    before  TEXT NOT NULL,      -- the column before: petstats.abilities, spelleffect.effects
    after   TEXT NOT NULL,      -- as written
    audit   INTEGER NOT NULL DEFAULT 0,  -- its gm_audit line (petstats)
    note    TEXT NOT NULL DEFAULT ''
)"""


def apply(db, plan):
    """The plan written (in the caller's transaction), each change logged; returns how many."""
    db.execute(LOG_SCHEMA)
    db.execute(pets.AUDIT_SCHEMA)
    written = 0
    for t in plan.get("times") or []:
        cur = db.execute("SELECT effects FROM spelleffect WHERE spellid = ?", (t["spellid"],)).fetchone()
        if cur is None or not cur[0]:
            continue
        j = json.loads(cur[0])
        if (j.get("duration_s") or 0) > 0:
            continue  # timed already: a second run writes nothing
        j["duration_s"] = t["duration_s"]
        after = json.dumps(j, separators=(",", ":"))
        db.execute("UPDATE spelleffect SET effects = ? WHERE spellid = ?", (after, t["spellid"]))
        db.execute(f"INSERT INTO {LOG} (tbl, spellid, before, after, note) VALUES ('spelleffect', ?, ?, ?, ?)",
                   (t["spellid"], cur[0], after, f"{t['spell']}: the first rank's {t['duration_s']} s"))
        written += 1
    for row in plan.get("rows") or []:
        k = {"spellid": row["spellid"]}
        before = pets.row_of(db, "petstats", k)
        if before is None or before.get("abilities") != row["before"]:
            continue  # changed since the plan (or done): left
        db.execute("UPDATE petstats SET abilities = ? WHERE spellid = ?", (row["after"], row["spellid"]))
        a = pets.audit(db, "update", "petstats", k, before, pets.row_of(db, "petstats", k), DETAIL)
        db.execute(f"INSERT INTO {LOG} (tbl, spellid, before, after, audit, note) VALUES ('petstats', ?, ?, ?, ?, ?)",
                   (row["spellid"], row["before"], row["after"], a, row["name"]))
        written += 1
    return written


COLUMN = {"petstats": "abilities", "spelleffect": "effects"}


def undo(db):
    """Each logged change still as written back as it was, the last first; returns (back, left)."""
    if not pets.table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, tbl, sid, before, after in db.execute(f"SELECT seq, tbl, spellid, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        col = COLUMN[tbl]
        k = {"spellid": sid}
        now = pets.row_of(db, tbl, k) if tbl == "petstats" else None
        cur = db.execute(f"SELECT {col} FROM {tbl} WHERE spellid = ?", (sid,)).fetchone()
        if cur is None or cur[0] != after:
            left += 1
            print(f"  left: {tbl} {sid}, changed since it was written")
            continue
        db.execute(f"UPDATE {tbl} SET {col} = ? WHERE spellid = ?", (before, sid))
        if tbl == "petstats":
            pets.audit(db, "update", tbl, k, now, pets.row_of(db, tbl, k), "undo: " + DETAIL)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if not db.execute(f"SELECT 1 FROM {LOG} LIMIT 1").fetchone():
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--sheets", help="EQOAEmu's sheets as .tsv (07-Pets.tsv, 02-Spells.tsv, 03-Effects.tsv)")
    ap.add_argument("--spellmap", help="the workbook's SpellID -> spellid map (.tsv)")
    ap.add_argument("--pet-fixes", help="the operator's file of pet spells the sheet names in error (.tsv)")
    ap.add_argument("--save-plan", metavar="FILE")
    ap.add_argument("--plan", metavar="FILE")
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    try:
        if a.undo:
            db.execute("BEGIN IMMEDIATE")
            try:
                back, left = undo(db)
                db.execute("COMMIT")
            except BaseException:
                db.execute("ROLLBACK")
                raise
            print(f"put back {back}, {left} left (changed since)")
            return
        if a.plan:
            with open(a.plan, encoding="utf-8") as f:
                plan = json.load(f)
            notes = []
        else:
            if not (a.sheets and a.spellmap):
                sys.exit("--sheets and --spellmap (or --plan)")
            sheets = pets.read_sheets(types.SimpleNamespace(sheets=a.sheets, sheet=None))
            plan, notes = make_plan(db, sheets, pets.read_spellmap(a.spellmap), pets.read_pet_fixes(a.pet_fixes))
        for t in plan.get("times") or []:
            print(f"{t['spell']} ({t['spellid']}): the first rank's {t['duration_s']} s")
        for row in plan.get("rows") or []:
            b, n = json.loads(row["before"] or "[]"), json.loads(row["after"])
            print(f"{row['name']} ({row['spellid']}): {len(b)} abilities -> {len(n)}: "
                  + "; ".join(f"{x.get('kind')}{'(' + x['target'] + ')' if x.get('target') else ''} {x.get('spell_id', '')}" for x in n))
        for n in notes:
            print("note:", n)
        print(f"{len(plan.get('times') or [])} drains timed, {len(plan.get('rows') or [])} pets' abilities to write")
        if a.save_plan:
            with open(a.save_plan, "w", encoding="utf-8") as f:
                json.dump(plan, f, indent=1)
            print("plan written:", a.save_plan)
            return
        if a.apply:
            db.execute("BEGIN IMMEDIATE")
            try:
                n = apply(db, plan)
                db.execute("COMMIT")
            except BaseException:
                db.execute("ROLLBACK")
                raise
            print(f"written {n}")
    finally:
        db.close()


if __name__ == "__main__":
    main()
