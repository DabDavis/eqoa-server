#!/usr/bin/env python3
"""The raid bosses' rampage (2026-11-17; the owner: "lets introduce rampage the area of effect melee damage ability with
10m", then of the proposed list, "use the rampage list as is"), on the server's code of branch rampage
(entity/arearampage.go: the template ability kind "rampage": one full melee hit, rolled on each, on the one it fights and
every player, pet and mercenary within 10 in its sight).

Each boss's template gets one rampage, added after its abilities as they are:

  20% of its melee rounds, x1.0: lord nagafen, lady vox, enraged lady vox, enraged aeyinar, avatar of hate,
    overlord xrag, cyrcalis
  every 15 s (from 15 s into the fight), x1.0: king daran
  every 12 s (from 12 s), x1.0: iod cazicthule
  15%, x0.8: bertoxxulous, tallon zek
  25%, x1.0: rallos zek, rallos zek the warlord, rallos zek unbound, vallon zek
  15%, x1.0: innoruuk, avatar of fear, iod megaladon, razeal moloch
  every 18 s (from 18 s), x1.0: talon's veeshan, veeshan's bulwark

Matched by the template's name on each shard. Passed by and listed, the others still written: a template not on this
shard; one already at the 12 abilities the server reads (refused); one holding a rampage already; one whose
npctemplate rows give it to names other than its boss's (the Live shard's bosses on a shared template such as
"roamer" are not touched: their template is not the boss's alone).

WHEN IT TAKES EFFECT: the server reads the templates at its start, and again at a GM panel's template edit or its
"Reload every NPC" (reload_npcs). An older server reads a template holding a "rampage" as unreadable and leaves the
WHOLE template out (the boss fights as a plain creature): apply it only under a server running branch rampage's code
(as the before.sh of the keep restart that brings it, or after it and then reload_npcs).

Report only by default; --apply writes it in one transaction, every row with its gm_audit line and its line in
rampage_bosses_1117_log; --undo puts back each row still as written (one changed since is left and said).

    tools/rampage_bosses_20261117.py --db eqoa.db [--apply | --undo]
"""
import argparse
import os
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import fix_moogie_tunare_20261110 as fm  # noqa: E402  the writer (gm_audit, log, exact undo)

LOG = "rampage_bosses_1117_log"
DETAIL = "the raid bosses' rampage (the owner, 2026-11-17: \"use the rampage list as is\"): tools/rampage_bosses_20261117.py"
MOST = 12


def chance(pct, dmg=100):
    a = {"kind": "rampage", "chance": pct}
    if dmg != 100:
        a["damage_pct"] = dmg
    return a


def clock(every):
    return {"kind": "rampage", "every": every, "first": every}


# (template, its boss's npctemplate names, the rampage)
BOSSES = [
    ("lord nagafen", ["lord nagafen"], chance(20)),
    ("lady vox", ["lady vox"], chance(20)),
    ("enraged lady vox", ["enraged lady vox"], chance(20)),
    ("enraged aeyinar", ["enraged aeyinar"], chance(20)),
    ("avatar of hate", ["avatar of hate"], chance(20)),
    ("overlord xrag", ["overlord xrag"], chance(20)),
    ("cyrcalis", ["cyrcalis"], chance(20)),
    ("king daran", ["king daran"], clock(15)),
    ("iod cazicthule", ["cazicthule"], clock(12)),
    ("bertoxxulous", ["bertoxxulous"], chance(15, 80)),
    ("tallon zek", ["tallon zek"], chance(15, 80)),
    ("rallos zek", ["rallos zek"], chance(25)),
    ("rallos zek the warlord", ["rallos zek the warlord"], chance(25)),
    ("rallos zek unbound", ["rallos zek unbound"], chance(25)),
    ("vallon zek", ["vallon zek"], chance(25)),
    ("innoruuk", ["innoruuk"], chance(15)),
    ("avatar of fear", ["avatar of fear"], chance(15)),
    ("iod megaladon", ["megaladon"], chance(15)),
    ("razeal moloch", ["razeal moloch"], chance(15)),
    ("talon's veeshan", ["talon's veeshan"], clock(18)),
    ("veeshan's bulwark", ["veeshan's bulwark"], clock(18)),
]


def describe(a):
    s = f"every {a['every']} s from {a['first']} s" if a.get("every") else f"{a['chance']}% of its rounds"
    return f"rampage within 10, {s}, x{a.get('damage_pct', 100) / 100:.1f}"


def plan(db):
    """(changes, written, passed): the rows to write, the lines of what is written and of what is passed by."""
    changes, written, passed = [], [], []
    for tname, names, ability in BOSSES:
        tmpl = fm.row_of(db, "classtemplates", {"name": tname})
        if tmpl is None:
            passed.append(f"{tname!r}: no such template on this shard")
            continue
        users = [r[0] for r in db.execute("SELECT name FROM npctemplate WHERE template = ? ORDER BY name", (tname,))]
        others = [u for u in users if u not in names]
        if others:
            passed.append(f"{tname!r}: shared with {len(others)} other names ({', '.join(others[:5])}{'...' if len(others) > 5 else ''})")
            continue
        try:
            abilities = fm.json.loads(tmpl["abilities"] or "[]")
        except ValueError:
            passed.append(f"{tname!r}: its abilities are not JSON: {tmpl['abilities']}")
            continue
        if any(a.get("kind") == "rampage" for a in abilities):
            passed.append(f"{tname!r}: holds a rampage already")
            continue
        if len(abilities) >= MOST:
            passed.append(f"{tname!r}: REFUSED, {len(abilities)} abilities already, the {MOST} the server reads")
            continue
        on = "" if users else " (no npctemplate row gives it to a name here)"
        changes.append(("classtemplates", tmpl, dict(tmpl, abilities=fm.abilities_json(abilities + [ability]))))
        written.append(f"{tname!r} ({len(abilities)} abilities before): {describe(ability)}{on}")
    return changes, written, passed


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    fm.LOG, fm.DETAIL = LOG, DETAIL  # its log and its audit's words, through the raid gods' tool's writer
    fm.LOG_SCHEMA = fm.LOG_SCHEMA.replace("moogie_tunare_1110_log", LOG)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        if a.undo:
            db.execute("BEGIN IMMEDIATE")
            try:
                back, left = fm.undo(db)
                db.execute("COMMIT")
            except BaseException:
                db.execute("ROLLBACK")
                raise
            print(f"put back {back} rows, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        if fm.table(db, LOG):
            print(f"applied already ({LOG} is there): --undo first")
            return 1
        changes, written, passed = plan(db)
        for line in written:
            print(line)
        if passed:
            print(f"passed by ({len(passed)}):")
            for line in passed:
                print("  " + line)
        if not changes:
            print("nothing to do")
            return 0
        if not a.apply:
            print(f"report only: {len(changes)} templates would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            n = fm.apply(db, changes)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {n} templates, each in gm_audit and {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
