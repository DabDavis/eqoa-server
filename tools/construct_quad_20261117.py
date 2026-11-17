#!/usr/bin/env python3
"""The Tinker's Construct: quad auto attack, Tinker Kick whenever its cooldown is up, arcane resist at the cap (the
owner, 2026-11-17: "the construct is supposed to quad auto attack, kick when cooldown is up, max ar").

What it was (petstats 901835 Construct, and its two forms, 901836 Construct (Drachnid) and 901837 Construct (Rat), all
EQOAEmu's workbook rows; no pettemplate row for any of them):
  - its swing pet.go's: a warrior of its level (51, the row's), Attack4 (four hits a round, 6 s) for an owner of any class
    but an enchanter, whose pets swing once a round (petSwing: Attack1, 3.2 s); a gnome enchanter Tinker's construct hit
    once a round;
  - Construct's abilities: a taunt (hate alone) 1 s into a fight, then every 12 s; Tinker Kick (902052, a blunt strike,
    200 + 15% of its STR, recast 12 s) 3 s into a fight, then every 12 s. The forms had their own strikes, no kick;
  - AR 95 (Construct), 91 (the forms): ResistShare's line takes ~27% of an arcane spell there.

What it is:
  1. classtemplates "pet construct" (new): class 0 (a warrior), 100% hit points, taken and melee, no abilities (the
     petstats rows' own are what it casts), and pettemplate rows "construct", "construct (drachnid)", "construct (rat)"
     (weapon 0) on it with swing 5 (CLIENT's Attack4: four hits at 0.5, 1, 1.7 and 2.4 s of a 6 s round, SwingDelay(5),
     its normal timer), round_ms 0: four swings a round, each rolled as ever (sheetMeleeHit: hit, miss, damage), for an
     owner of any class (the enchanter's single swing gone) at any level it can be summoned at.
  2. petstats abilities: Tinker Kick (spell_id 902052) at "first" 0 and "every" 12, its recast (spellpattern.recast 12):
     the kick opens each fight and comes again each time its cooldown is up while it fights (pettemplate.go's clock:
     due from then, cast at its first turn in reach). Construct's kick moved from first 3 to 0; the forms given it as
     a third ability, their own strikes kept.
  3. petstats ar 315 on all three: ResistCap / playerResistMax, the most of a resist that counts on the player's line
     the workbook pets' resists are read on (petResistShare -> playerResistShareNow). No pet had more than 110.

The construct is level 51 at every owner level it can have (rank 1542 Construct needs level 60; petLevel caps a pet at
its owner's): one tier. The PetLook ranks (900917 Jarek's Creation, any later look) change ModelID and size only
(cmpetlook.go), so a construct in any look is this one. --base-only: Construct (901835) alone, not the forms.

Checked as found, else REFUSED and nothing written: the three petstats rows as the 2026-11-13 import left them (ar,
abilities), spell 902052 Tinker Kick, no pettemplate row for them and no classtemplates "pet construct" (or this tool's,
then nothing to do). Report by default; --apply in one transaction, a gm_audit line a row, log construct_quad_1117_log;
--undo exact (a row changed since is left and listed).

petstats, classtemplates and pettemplate are read at the server's start and at the panel's template edits
(LoadClassTemplates); a construct out at the time keeps its old self until summoned again.

    python3 -E -s tools/construct_quad_20261117.py --db eqoa.db [--base-only] [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "construct_quad_1117_log"
DETAIL = "the owner (2026-11-17): the construct quads, kicks on its cooldown, max AR: tools/construct_quad_20261117.py"
WHO, VIA = "claude", "tool"
SRC = "constructquad1117"

KICK = 902052  # Tinker Kick
KICK_RECAST = 12
AR_CAP = 315  # entity ResistCap / playerResistMax
TEMPLATE = "pet construct"
SWING = 5  # CLIENT's Attack4
NOTE = ("The Tinker's Construct (the owner, 2026-11-17: \"the construct is supposed to quad auto attack, kick when cooldown "
        "is up, max ar\"): a warrior's Attack4 at any owner class, four hits a 6 s round; its petstats row's spells "
        "(Tinker Kick on its 12 s recast); tools/construct_quad_20261117.py")
TEMPLATE_ROW = {"name": TEMPLATE, "class": 0, "hp_pct": 100, "taken_pct": 100, "melee_pct": 100, "abilities": "[]",
                "note": NOTE, "source": SRC}

# spellid: (pettemplate spell key, ar as found, abilities as found)
OLD_KICK = {"every": 12.0, "first": 3.0, "kind": "nuke", "spell_id": KICK}
NEW_KICK = {"every": float(KICK_RECAST), "first": 0.0, "kind": "nuke", "spell_id": KICK}
ROWS = {
    901835: ("construct", 95, [{"every": 12.0, "first": 1.0, "kind": "taunt", "hits": [[1, 0, 0]]}, OLD_KICK]),
    901836: ("construct (drachnid)", 91, [{"every": 15.0, "first": 1.0, "kind": "nuke", "spell_id": 902053},
                                          {"every": 6.0, "first": 3.0, "kind": "nuke", "spell_id": 902054}]),
    901837: ("construct (rat)", 91, [{"every": 6.0, "first": 1.0, "kind": "nuke", "spell_id": 902055},
                                     {"every": 12.0, "first": 3.0, "kind": "nuke", "spell_id": 902056}]),
}

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    rowkey TEXT NOT NULL,
    before TEXT,
    after  TEXT,
    audit  INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def compact(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False, sort_keys=True)


def abilities_text(v):
    return json.dumps(v, separators=(",", ":"))


def new_abilities(spell, old):
    if spell == 901835:
        return [a if a != OLD_KICK else NEW_KICK for a in old]
    return old + [NEW_KICK]


def audit(db, action, tbl, key, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, compact(key), compact(before or {}), compact(after or {}), DETAIL))
    return cur.lastrowid


def plan(db, spells):
    """changes: (tbl, key, before, after) with before None an insert; lines; refusals."""
    changes, lines, refusals = [], [], []
    k = db.execute("SELECT spellname, recast FROM spellpattern WHERE spellid = ?", (KICK,)).fetchone()
    if k is None or k[0] != "Tinker Kick" or k[1] != KICK_RECAST:
        refusals.append(f"spellpattern {KICK}: {k}, want Tinker Kick, recast {KICK_RECAST}")
    t = db.execute("SELECT class, abilities, source FROM classtemplates WHERE name = ?", (TEMPLATE,)).fetchone()
    if t is None:
        changes.append(("classtemplates", {"name": TEMPLATE}, None, TEMPLATE_ROW))
        lines.append(f"classtemplates {TEMPLATE!r}: new, class 0 (a warrior), 100/100/100, no abilities")
    elif t[2] != SRC:
        refusals.append(f"classtemplates {TEMPLATE!r} is there already, not this tool's: {t}")
    for spell in spells:
        key, ar, abilities = ROWS[spell]
        p = db.execute("SELECT template, swing_class, swing, round_ms, source FROM pettemplate WHERE spell = ? AND weapon = 0",
                       (key,)).fetchone()
        prow = {"spell": key, "weapon": 0, "template": TEMPLATE, "swing_class": -1, "swing": SWING, "round_ms": 0,
                "note": "the Tinker's construct: four hits a round (tools/construct_quad_20261117.py)", "source": SRC}
        if p is None:
            changes.append(("pettemplate", {"spell": key, "weapon": 0}, None, prow))
            lines.append(f"pettemplate {key!r}: new, template {TEMPLATE!r}, swing {SWING} (Attack4), its 6 s round")
        elif p[4] != SRC:
            refusals.append(f"pettemplate {key!r} is there already, not this tool's: {p}")
        r = db.execute("SELECT name, ar, abilities FROM petstats WHERE spellid = ?", (spell,)).fetchone()
        if r is None or r[0] != "Construct":
            refusals.append(f"petstats {spell}: {r}, want a Construct")
            continue
        have = json.loads(r[2])
        want = new_abilities(spell, abilities)
        if r[1] == AR_CAP and have == want:
            lines.append(f"petstats {spell} ({key}): AR {AR_CAP} and the kick already")
            continue
        if r[1] != ar or have != abilities:
            refusals.append(f"petstats {spell} ({key}): ar {r[1]}, abilities {r[2]}; want ar {ar}, {abilities_text(abilities)}")
            continue
        changes.append(("petstats", {"spellid": spell}, {"ar": r[1], "abilities": r[2]},
                        {"ar": AR_CAP, "abilities": abilities_text(want)}))
        lines.append(f"petstats {spell} ({key}): ar {r[1]} -> {AR_CAP}; abilities {r[2]} -> {abilities_text(want)}")
    return changes, lines, refusals


def apply(db, changes):
    db.execute(LOG_SCHEMA)
    for tbl, key, before, after in changes:
        if tbl == "classtemplates":
            db.execute("INSERT INTO classtemplates (name, class, hp_pct, taken_pct, melee_pct, abilities, note, source) "
                       "VALUES (:name, :class, :hp_pct, :taken_pct, :melee_pct, :abilities, :note, :source)", after)
            a = audit(db, "insert", tbl, key, None, after)
        elif tbl == "pettemplate":
            db.execute("INSERT INTO pettemplate (spell, weapon, template, swing_class, swing, round_ms, note, source) "
                       "VALUES (:spell, :weapon, :template, :swing_class, :swing, :round_ms, :note, :source)", after)
            a = audit(db, "insert", tbl, key, None, after)
        else:
            if db.execute("UPDATE petstats SET ar = ?, abilities = ? WHERE spellid = ? AND ar = ? AND abilities = ?",
                          (after["ar"], after["abilities"], key["spellid"], before["ar"], before["abilities"])).rowcount != 1:
                raise RuntimeError(f"petstats {key} changed under the update")
            a = audit(db, "update", tbl, key, before, after)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                   (tbl, compact(key), None if before is None else compact(before), compact(after), a))


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, tbl, key, before, after in db.execute(f"SELECT seq, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key, after = json.loads(key), json.loads(after)
        before = None if before is None else json.loads(before)
        if tbl == "classtemplates":
            ok = db.execute("DELETE FROM classtemplates WHERE name = ? AND source = ? AND class = ? AND abilities = ?",
                            (key["name"], SRC, after["class"], after["abilities"])).rowcount == 1
        elif tbl == "pettemplate":
            ok = db.execute("DELETE FROM pettemplate WHERE spell = ? AND weapon = ? AND source = ? AND template = ? AND swing = ?",
                            (key["spell"], key["weapon"], SRC, after["template"], after["swing"])).rowcount == 1
        else:
            ok = db.execute("UPDATE petstats SET ar = ?, abilities = ? WHERE spellid = ? AND ar = ? AND abilities = ?",
                            (before["ar"], before["abilities"], key["spellid"], after["ar"], after["abilities"])).rowcount == 1
        if not ok:
            print(f"  left: {tbl} {key} changed since")
            left += 1
            continue
        audit(db, "update" if before is not None else "delete", tbl, key, after, before)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if left == 0:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    ap.add_argument("--base-only", action="store_true", help="Construct (901835) alone, not the drachnid and rat forms")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    spells = [901835] if a.base_only else sorted(ROWS)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        db.execute("BEGIN IMMEDIATE")
        if a.undo:
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"put back {back}, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        if table(db, LOG):
            db.execute("ROLLBACK")
            print(f"applied already ({LOG} is there): --undo first")
            return 0
        changes, lines, refusals = plan(db, spells)
        for line in lines:
            print(line)
        if refusals:
            db.execute("ROLLBACK")
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not changes:
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"report only: {len(changes)} rows would be written (--apply writes them)")
            return 0
        apply(db, changes)
        db.execute("COMMIT")
        print(f"applied: {len(changes)} rows, log {LOG}")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
