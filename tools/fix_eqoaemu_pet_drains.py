#!/usr/bin/env python3
"""The workbook pets' drains of their enemy's stats (Undead Bane), on a server
the pet import already filled.

tools/import_eqoaemu_pets.py made each workbook pet's spells its abilities, but
left out a drain of the enemy's stats (a necromancer's pets' Undead Bane, from
the level 30 pet: STR and AC taken and given to the pet for a minute): a
creature's stats counted in no fight then. Since 2026-11-01 they do (a
creature's effects, entity/pet.go's creatureMods) and a pet's life tap lands a
drain of stats (entity/pettemplate.go's petDrainStatsLocked); the importer now
makes such a spell a life tap (its ability_of). This tool adds to each petstats
row of a workbook pet the abilities the importer makes now and the row lacks
that are such drains, appended, its others left as they are; and gives each
such drain's record with no time of its own (the sheet gives one to the first
rank alone, Undead Bane Level 30's 60 s) that first rank's time, ours: a drain
of no time takes nothing that lasts.

The sheets are the operator's (EQOAEmu's workbook as .tsv files, as the pet
import reads them) and not on the box: --save-plan FILE here, --plan FILE
--apply there.

    tools/fix_eqoaemu_pet_drains.py --db copy.db --sheets <folder> --spellmap <map.tsv>      # report
    tools/fix_eqoaemu_pet_drains.py --db copy.db --sheets <folder> --spellmap <map.tsv> --save-plan plan.json
    tools/fix_eqoaemu_pet_drains.py --db eqoa.db --plan plan.json --apply                    # one transaction, logged
    tools/fix_eqoaemu_pet_drains.py --db eqoa.db --undo                                      # each row still as written back
"""
import argparse
import json
import os
import sqlite3
import sys
import types

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import import_eqoaemu_pets as pets  # noqa: E402  the importer's rule, one place

LOG = "eqoaemu_pet_drains_log"
DEFAULT_DB = "eqoa.db"


def stat_drain(sheet_spell, effects):
    """Whether a pet's sheet spell is a drain of the enemy's stats (no hit)."""
    effs = pets.effect_rows(sheet_spell, effects)
    if not effs:
        return False
    e = effs[0]
    return pets.fold(e.get("TYPE")) == "drain" and (pets.num(e.get("HP1")) or 0) == 0


def make_plan(db, sheets, smap):
    """Each workbook pet's row: the stat drains the importer makes now that the
    row lacks."""
    spells = {s["SpellID"].strip().upper(): s for s in sheets["Spells"] if s.get("SpellID", "").strip()}
    effects = {e["EffectID"].strip().upper(): e for e in sheets["Effects"] if e.get("EffectID", "").strip()}
    rows = {r[1].strip().upper(): (r[0], r[2], r[3]) for r in
            db.execute("SELECT spellid, sheet_pet_id, name, abilities FROM petstats WHERE sheet_pet_id <> ''")}
    out, notes, timed = [], [], []
    drains = {}  # spellid -> the sheet's spell, each pet's drain of stats
    for p in sheets["Pets"]:
        pid = (p.get("PetID") or "").strip().upper()
        if pid not in rows:
            continue
        spellid, name, abilities = rows[pid]
        have = json.loads(abilities or "[]")
        pinned = {a.get("spell_id") for a in have if a.get("spell_id")}
        made = []
        for k in ("Spell1", "Spell2", "Spell3", "Spell4"):
            ref = (p.get(k) or "").strip().upper()
            s = spells.get(ref)
            if not ref or s is None:
                continue
            psid = smap.get(ref)
            if psid is None:
                notes.append(f"{name!r}: {s.get('Name')!r} not in the spell map")
                continue
            a, _ = pets.ability_of(s, effects, psid, len(made))
            if a is None:
                continue
            made.append(a)
            if a.get("kind") == "lifetap" and stat_drain(s, effects):
                drains.setdefault(psid, s)
                if psid not in pinned:
                    out.append(dict(spellid=spellid, sheet_pet_id=pid, name=name, add=a, spell=s.get("Name")))
    # each drain's record with no time: its line's first rank's (the sheet's one time)
    times = {sid: (pets.num(s.get("Duration")) or 0) for sid, s in drains.items()}
    line_time = max(times.values(), default=0)
    for sid in sorted(drains):
        if times[sid] > 0 or line_time <= 0:
            continue
        r = db.execute("SELECT effects FROM spelleffect WHERE spellid = ?", (sid,)).fetchone()
        if r is None or not r[0]:
            notes.append(f"{drains[sid].get('Name')!r} ({sid}): no record in spelleffect")
            continue
        j = json.loads(r[0])
        if (j.get("duration_s") or 0) > 0:
            continue
        timed.append(dict(spellid=sid, spell=drains[sid].get("Name"), duration_s=int(line_time)))
    return {"rows": out, "times": timed}, notes


def apply(db, plan):
    """The plan written in one transaction (the caller's), each row logged."""
    db.execute(f"""CREATE TABLE IF NOT EXISTS {LOG} (
        seq INTEGER PRIMARY KEY AUTOINCREMENT, tbl TEXT NOT NULL, spellid INTEGER NOT NULL,
        before TEXT NOT NULL, after TEXT NOT NULL, note TEXT NOT NULL DEFAULT '')""")
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
                   (t["spellid"], cur[0], after, f"{t['spell']}: its line's {t['duration_s']} s"))
        written += 1
    for row in plan.get("rows") or []:
        cur = db.execute("SELECT abilities FROM petstats WHERE spellid = ?", (row["spellid"],)).fetchone()
        if cur is None:
            continue
        have = json.loads(cur[0] or "[]")
        if any(a.get("spell_id") == row["add"].get("spell_id") for a in have):
            continue  # there already: a second run writes nothing
        after = json.dumps(have + [row["add"]], separators=(",", ":"))
        db.execute("UPDATE petstats SET abilities = ? WHERE spellid = ?", (after, row["spellid"]))
        db.execute(f"INSERT INTO {LOG} (tbl, spellid, before, after, note) VALUES ('petstats', ?, ?, ?, ?)",
                   (row["spellid"], cur[0], after, f"{row['name']}: {row['spell']}"))
        written += 1
    return written


COLUMN = {"petstats": "abilities", "spelleffect": "effects"}


def undo(db):
    """Each logged row still as written back as it was, the last first."""
    if not db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (LOG,)).fetchone():
        return 0, 0
    back = left = 0
    for seq, tbl, spellid, before, after in db.execute(
            f"SELECT seq, tbl, spellid, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        col = COLUMN[tbl]
        cur = db.execute(f"SELECT {col} FROM {tbl} WHERE spellid = ?", (spellid,)).fetchone()
        if cur is None or cur[0] != after:
            left += 1
            continue
        db.execute(f"UPDATE {tbl} SET {col} = ? WHERE spellid = ?", (before, spellid))
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
    ap.add_argument("--save-plan", metavar="FILE")
    ap.add_argument("--plan", metavar="FILE")
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db)
    try:
        if a.undo:
            with db:
                back, left = undo(db)
            print(f"put back {back} rows, {left} left (changed since)")
            return
        if a.plan:
            with open(a.plan, encoding="utf-8") as f:
                plan = json.load(f)
            notes = []
        else:
            if not (a.sheets and a.spellmap):
                sys.exit("--sheets and --spellmap (or --plan)")
            sheets = pets.read_sheets(types.SimpleNamespace(sheets=a.sheets, sheet=None))
            plan, notes = make_plan(db, sheets, pets.read_spellmap(a.spellmap))
        for t in plan.get("times") or []:
            print(f"{t['spell']} ({t['spellid']}): its line's {t['duration_s']} s")
        for row in plan.get("rows") or []:
            print(f"{row['name']} ({row['spellid']}): adds {row['spell']} as a life tap of its stats, spell {row['add']['spell_id']}, every {row['add']['every']} s")
        for n in notes:
            print("note:", n)
        print(f"{len(plan.get('times') or [])} drains timed, {len(plan.get('rows') or [])} abilities to add")
        if a.save_plan:
            with open(a.save_plan, "w", encoding="utf-8") as f:
                json.dump(plan, f, indent=1)
            print("plan written:", a.save_plan)
            return
        if a.apply:
            with db:
                n = apply(db, plan)
            print(f"written {n} rows")
    finally:
        db.close()


if __name__ == "__main__":
    main()
