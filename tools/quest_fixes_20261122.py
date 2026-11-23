#!/usr/bin/env python3
"""Quest fixes from Heirloom's petitions of 2026-11-22, for a shard that did not get them through the GM panel (on
Heirloom they went in live by the panel's update_row; there this tool finds them in place and writes nothing):

  - 2675, The Necrologist of Rain: the Manifestation of Rain says "Take my key" and gave none; its step (the last,
    at the manifestation of rain) now gives the rain key (the guide: "The Manifestation of Rain will give you the NO
    TRADE key that allows you to return to Rain 1 for free at any time").
  - 2687, Captain Scilond's Scouts: the Klick'Anon step (deadly corrosions) alone had no spawn on start, as each
    other camp's has: five deadly corrosions, hostile.
  - 2693, Jarek's Creation: the fungus cockroach step's map mark went to the biggest cockroach camp, far off; the
    step is given the hut's camp across the river (x 24303, z 9580) for its mark (a named creature's kill counts
    anywhere still).
  - 2694 and 2690, the trigger Jarek's Creation: Jarek walked outside at a run ("lightning fast"): walked (pace 4);
    and after the creation tears him apart he was gone 5 minutes, so the next player found an empty hut: 1 minute.
  - 2705, Talya Montar (The Scribe's Inks): to the top of the Highpass keep, where the player found her place, and
    the step's journal says so (npcs rows are read at a start).
  - 2697, Waysender Hrumgar (where it is): its offer tells a hill giant that its next quests are at home.

Each change is found by the quest's id, its name checked, (and step) or the trigger's name, and must read as before (else refused,
nothing written). Quests are read as they go; a trigger at the next start or panel edit.

Dry run by default. --apply in one transaction, a gm_audit line a change (account 1, dab, via tool), each in
quest_fixes_1122_log; a second --apply writes nothing; --undo puts back what the log holds.

    python3 -E -s tools/quest_fixes_20261122.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "quest_fixes_1122_log"
TOOL = "tools/quest_fixes_20261122.py"
DETAIL = f"Heirloom petitions 2675, 2687, 2690, 2693, 2694, 2697 (2026-11-22): {TOOL}"

KEY_SAYS = "You have walked the whole chain of rain. Take my key: it brings you back any time."
CORROSIONS = '[{"on":"start","do":"spawn","npc":"deadly corrosion","hostile":true,"count":5}]'
HRUMGAR_OLD = ("The ogres keep you well, but the plateau remembers you. I can open the stone way back to Elder Hrolmog.\n"
               "Your spirit stays bound where it is.\nShall I send you home to the West Plateau?")
HRUMGAR_NEW = ("The ogres keep you well, but the plateau remembers you. Your own work waits there: Stonehand Borrik has more "
               "for a young giant, and Elder Hrolmog after him, up to the day Halvek asks for you. I can open the stone way "
               "back to Elder Hrolmog.\nYour spirit stays bound where it is.\nShall I send you home to the West Plateau?")

# (quest id, its name, step, column, before, after): the importer's ids, the same on every shard
STEPS = [
    (101303, "The Necrologist of Rain", 12, "rewarditems", "", "701464"),
    (101285, "Captain Scilond's Scouts", 22, "events", "", CORROSIONS),
    (100463, "Jarek's Creation", 3, "x", 0.0, 24303.0),
    (100463, "Jarek's Creation", 3, "z", 0.0, 9580.0),
    (100748, "The Scribe's Inks", 2, "journal",
     "Buy cured drake's blood from Talya Montar on the second floor of the Highpass keep (600 tunar).",
     "Buy cured drake's blood from Talya Montar at the top of the Highpass keep (600 tunar)."),
]
# (NPC name, world, its place as read (x, z), the new (x, y, z)): the one row of the name
MOVES = [("Talya Montar", 0, (16889.75, 15236.33), (16896.5, 225.25, 15238.97))]
STEP_CHECK = {("The Necrologist of Rain", 12): ("target", "manifestation of rain"),
              ("Captain Scilond's Scouts", 22): ("target", "deadly corrosion"),
              ("Jarek's Creation", 3): ("target", "fungus cockroach"),
              ("The Scribe's Inks", 2): ("target", "Talya Montar")}
TRIGGER = "Jarek's Creation"
JAREK = "Jarek N`Toxis"


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


class Refused(Exception):
    pass


def same(a, b):
    if isinstance(a, (int, float)) and isinstance(b, (int, float)):
        return abs(a - b) < 1e-6
    return a == b


def plan(db):
    ops, lines = [], []  # (table, where sql, where args, column, before, after, what)
    for qid, name, step, col, old, new in STEPS:
        have = db.execute("SELECT name FROM quests WHERE questid = ?", (qid,)).fetchone()
        if have is None or have[0] != name:
            raise Refused(f"quest {qid}: {have}, not {name!r}")
        r = db.execute(f'SELECT "{col}", {STEP_CHECK[(name, step)][0]} FROM queststeps WHERE questid = ? AND step = ?',
                       (qid, step)).fetchone()
        if r is None or r[1] != STEP_CHECK[(name, step)][1]:
            raise Refused(f"{name} step {step}: {r}, not at {STEP_CHECK[(name, step)][1]!r}")
        what = f"{name} ({qid}) step {step} {col}"
        if same(r[0], new):
            lines.append(f"  {what}: already {new!r}")
        elif same(r[0], old):
            ops.append(("queststeps", "questid = ? AND step = ?", (qid, step), col, r[0], new, what))
            lines.append(f"  {what}: {old!r} -> {new!r}")
        else:
            raise Refused(f"{what} reads {r[0]!r}, not {old!r}")
    for name, world, (ox, oz), (nx, ny, nz) in MOVES:
        rows = db.execute("SELECT id, world, x_coord, y_coord, z_coord FROM npcs WHERE npc_name = ?", (name,)).fetchall()
        if len(rows) != 1:
            raise Refused(f"{name}: {len(rows)} npcs rows")
        rid, w, x, y, z = rows[0]
        what = f"{name} (npcs {rid})"
        if w == world and same(x, nx) and same(y, ny) and same(z, nz):
            lines.append(f"  {what}: already at ({nx}, {ny}, {nz})")
            continue
        if w != world or abs(x - ox) > 1 or abs(z - oz) > 1:
            raise Refused(f"{what} stands at ({x}, {z}) world {w}, not as read")
        for col, old, new in (("x_coord", x, nx), ("y_coord", y, ny), ("z_coord", z, nz)):
            ops.append(("npcs", "id = ?", (rid,), col, old, new, f"{what} {col}"))
        lines.append(f"  {what}: moved to the keep's top ({nx}, {ny}, {nz}), petition 2705")
    rows = db.execute("SELECT slot, steps FROM gmtriggers WHERE name = ?", (TRIGGER,)).fetchall()
    if len(rows) != 1:
        raise Refused(f"trigger {TRIGGER!r}: {len(rows)} rows")
    slot, steps = rows[0]
    st = json.loads(steps)
    changed = False
    for s in st:
        if s.get("do") == "walk" and s.get("npc") == JAREK and s.get("pace") != 4.0:
            s["pace"], changed = 4.0, True
        if s.get("do") == "hide" and s.get("npc") == JAREK and s.get("minutes") != 1:
            s["minutes"], changed = 1, True
    if not any(s.get("do") == "walk" and s.get("npc") == JAREK for s in st):
        raise Refused(f"trigger {TRIGGER!r} walks no {JAREK}")
    what = f"trigger {TRIGGER!r} (slot {slot}) steps"
    if changed:
        new = json.dumps(st, separators=(",", ":"), ensure_ascii=False)
        ops.append(("gmtriggers", "slot = ?", (slot,), "steps", steps, new, what))
        lines.append(f"  {what}: Jarek walks (pace 4), gone 1 minute")
    else:
        lines.append(f"  {what}: already so")
    for qid, offer in db.execute("SELECT questid, offer FROM quests WHERE giver = 'Waysender Hrumgar'").fetchall():
        what = f"Waysender Hrumgar's quest {qid} offer"
        if offer == HRUMGAR_NEW:
            lines.append(f"  {what}: already so")
        elif offer == HRUMGAR_OLD:
            ops.append(("quests", "questid = ?", (qid,), "offer", offer, HRUMGAR_NEW, what))
            lines.append(f"  {what}: points to Stonehand Borrik")
        else:
            lines.append(f"  {what}: not as read, left")
    return ops, lines


def audit(db, tbl, key, col, before, after):
    return db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                      "VALUES (?, 1, 'dab', 'tool', '', 'update', ?, ?, ?, ?, ?)",
                      (now(), tbl, json.dumps(key), json.dumps({col: before}), json.dumps({col: after}), DETAIL)).lastrowid


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("PRAGMA busy_timeout = 5000")
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            if not db.execute("SELECT 1 FROM sqlite_master WHERE name = ?", (LOG,)).fetchone():
                db.execute("COMMIT")
                print("undone: nothing logged")
                return 0
            n, left = 0, []
            for seq, tbl, where, args, col, before, after in db.execute(
                    f"SELECT seq, tbl, wh, args, col, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
                args = json.loads(args)
                have = db.execute(f'SELECT "{col}" FROM "{tbl}" WHERE {where}', args).fetchone()
                if have is None or not same(have[0], json.loads(after)):
                    left.append(f"{tbl} {args} {col}: changed since, left")
                    continue
                db.execute(f'UPDATE "{tbl}" SET "{col}" = ? WHERE {where}', [json.loads(before), *args])
                audit(db, tbl, args, col, json.loads(after), json.loads(before))
                db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
                n += 1
            if not db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0]:
                db.execute(f"DROP TABLE {LOG}")
            db.execute("COMMIT")
            print(f"undone: {n} changes put back" + "".join(f"\n  left: {w}" for w in left))
            return 0
        try:
            ops, lines = plan(db)
        except Refused as e:
            db.execute("ROLLBACK")
            print(f"REFUSED, nothing written: {e}")
            return 1
        print("\n".join(lines))
        print(f"totals: {len(ops)} changes")
        if not a.apply or not ops:
            db.execute("ROLLBACK")
            print("dry run: nothing written (--apply to write)" if not a.apply else "nothing to write: all in place")
            return 0
        db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (seq INTEGER PRIMARY KEY AUTOINCREMENT, tbl TEXT, wh TEXT, args TEXT, "
                   "col TEXT, before TEXT, after TEXT, audit INTEGER, at TEXT)")
        for tbl, where, args, col, before, after, _ in ops:
            db.execute(f'UPDATE "{tbl}" SET "{col}" = ? WHERE {where}', [after, *args])
            au = audit(db, tbl, list(args), col, before, after)
            db.execute(f"INSERT INTO {LOG} (tbl, wh, args, col, before, after, audit, at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
                       (tbl, where, json.dumps(list(args)), col, json.dumps(before), json.dumps(after), au, now()))
        db.execute("COMMIT")
        print(f"applied: {len(ops)} changes, a gm_audit line each, logged in {LOG}")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
