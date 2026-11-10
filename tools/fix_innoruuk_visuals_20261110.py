#!/usr/bin/env python3
"""Innoruuk's buffs his own, his procs' looks, and Mr. Moogie's line at Innoruuk's death (2026-11-10, after
tools/fix_moogie_tunare_20261110.py; the owner, watching the fight: "innoruuk is casting veil of death on moogie and
the buffs, should be on innoruuk", "innoruuk keeps casting veil of death also no reason since it is a longer cooldown",
"largesse of plague should have a proc spellfx"; the GM Jibrish's petition 1458: "whenever he kills Innoruuk he says
"Dab got dunked again""), on the server's code of the same branch (selfbuff.go: a proc or a veil of a pinned spell is a
buff its creature casts on itself; a killsay; an older server reads these templates as unreadable).

  classtemplates "innoruuk": his Largesse of Plague proc (700794) takes the disease proc hit's look at what it hits,
    SPELLFX.CSF 0x0CACDEE1 (no caster animation; the target's instant and four emitters, among them the olive spore
    particle 0x122CD977 of Disease Cloud's and Shock of Spores' own visuals), where it showed the creature special's;
    his veil pins Veil of Death + (1431, the 30 minute one), so he casts it on himself, its own visual, as his fight
    begins and at its time, and its tap at every hit shows nothing (it had shown Veil of Death's cast visual at what
    he hit); Largesse of Plague is cast on himself the same way (its 5 minutes);
  classtemplates "mr moogie": "Dab got dunked again" in /say when his hit kills Innoruuk (killsay).

Every row found by its name and left alone, nothing written, unless each is as fix_moogie_tunare_20261110.py wrote it
(Innoruuk's last two abilities its proc and veil, Mr. Moogie's its seven) and Veil of Death + is 1431's name; no
Innoruuk template: another shard, nothing done. Report only by default; --apply in one transaction, gm_audit lines,
log innoruuk_visuals_1110_log; --undo exact (a row changed since left and said). The templates are read at the
server's start or a GM panel template edit (reload_npcs): run it as the before.sh of the restart bringing this code.

    tools/fix_innoruuk_visuals_20261110.py --db eqoa.db [--apply | --undo]
"""
import argparse
import json
import os
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import fix_moogie_tunare_20261110 as fm  # noqa: E402  its rows as it wrote them, its helpers

LOG = "innoruuk_visuals_1110_log"
DETAIL = ("Innoruuk's buffs his own and his procs' looks, Mr. Moogie's line at his death (the owner and petition 1458, "
          "2026-11-10): tools/fix_innoruuk_visuals_20261110.py")
FX_DISEASE_HIT = 0x0CACDEE1
VEIL_PLUS, VEIL_PLUS_NAME = 1431, "Veil of Death +"
LARGESSE_NOW = {"kind": "proc", "spell_id": fm.LARGESSE}
LARGESSE_NEW = {"kind": "proc", "spell_id": fm.LARGESSE, "fx": FX_DISEASE_HIT}
VEIL_NOW = {"kind": "veil", "tap_pct": 1, "resist": "AR", "name": "Veil of Death"}  # and the fx the tool pinned, if any
VEIL_NEW = {"kind": "veil", "spell_id": VEIL_PLUS, "tap_pct": 1, "resist": "AR"}
KILLSAY = {"kind": "killsay", "foes": ["Innoruuk"], "say": "Dab got dunked again"}


def plan(db):
    """(changes, lines, refusals, here): the two template rows to write."""
    lines, refusals, changes = [], [], []
    inno = fm.row_of(db, "classtemplates", {"name": fm.INNO_T})
    if inno is None:
        return [], [f"no class template {fm.INNO_T!r} here: not his shard"], [], False
    try:
        kit = json.loads(inno["abilities"] or "[]")
    except ValueError:
        kit = None
    veil = dict(kit[-1]) if kit and len(kit) >= 2 else {}
    veil.pop("fx", None)
    if kit is None or len(kit) != 7 or fm.norm(kit[-2]) != fm.norm(LARGESSE_NOW) or fm.norm(veil) != fm.norm(VEIL_NOW):
        refusals.append(f"{fm.INNO_T!r}'s abilities do not end in the Largesse of Plague proc and Veil of Death the raid gods' tool "
                        f"wrote: {inno['abilities']}")
    else:
        was = kit[-1].get("fx")
        changes.append(("classtemplates", inno, dict(inno, abilities=fm.abilities_json(kit[:-2] + [LARGESSE_NEW, VEIL_NEW]))))
        lines.append(f"classtemplates {fm.INNO_T!r}: Largesse of Plague's procs show 0x{FX_DISEASE_HIT:08X} (the disease proc hit) at what "
                     f"they hit; his veil pins {VEIL_PLUS_NAME} ({VEIL_PLUS}), its tap silent "
                     f"(it showed {'0x%08X' % (was & 0xFFFFFFFF) if was else 'the creature special'} at every hit); both buffs "
                     f"cast on himself as his fight begins and at their times")
    vp = db.execute("SELECT p.spellname, COALESCE(e.line, ''), COALESCE(e.fxdict, 0) FROM spellpattern p LEFT JOIN spelleffect e "
                    "ON e.spellid = p.spellid WHERE p.spellid = ?", (VEIL_PLUS,)).fetchone()
    if vp is None or (vp[0] or "").strip() != VEIL_PLUS_NAME:
        refusals.append(f"spell {VEIL_PLUS} is {vp and vp[0]!r}, not {VEIL_PLUS_NAME}")
    else:
        lines.append(f"{VEIL_PLUS_NAME} ({VEIL_PLUS}): its line {vp[1]!r}, its visual 0x{vp[2] & 0xFFFFFFFF:08X}: cast on himself again at "
                     f"the line's time")
    lp = db.execute("SELECT COALESCE(e.line, ''), COALESCE(e.fxdict, 0) FROM spelleffect e WHERE e.spellid = ?", (fm.LARGESSE,)).fetchone()
    if lp is not None:
        lines.append(f"{fm.LARGESSE_NAME} ({fm.LARGESSE}): its visual 0x{lp[1] & 0xFFFFFFFF:08X} on himself, again at its record's time")
    moogie = fm.row_of(db, "classtemplates", {"name": fm.MOOGIE_T})
    if moogie is None:
        refusals.append(f"no class template {fm.MOOGIE_T!r}")
    elif not fm.same_abilities(moogie["abilities"], fm.MOOGIE_NEW):
        refusals.append(f"{fm.MOOGIE_T!r}'s abilities are not the raid gods' tool's seven: {moogie['abilities']}")
    else:
        changes.append(("classtemplates", moogie, dict(moogie, abilities=fm.abilities_json(fm.MOOGIE_NEW + [KILLSAY]))))
        lines.append(f"classtemplates {fm.MOOGIE_T!r}: \"{KILLSAY['say']}\" in /say when his hit kills Innoruuk")
    return changes, lines, refusals, True


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
        changes, lines, refusals, here = plan(db)
        for l in lines:
            print(l)
        if not here:
            print("nothing done")
            return 0
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not a.apply:
            print(f"report only: {len(changes)} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            n = fm.apply(db, changes)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {n} rows, each in gm_audit and {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
