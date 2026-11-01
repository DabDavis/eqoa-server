#!/usr/bin/env python3
"""EQOA-Data spells nothing should teach, out of their class's list, out of
every book and out of any quest choice that gives one (the user, 2026-11-01:
"remove them from both" of the druid's, "remove them" of the shaman's and the
necromancer's, "take it out" of the enchanter's epic option).

  - The druid's Nature Form (701064), Wild Nature (701258), Bixie Form
    (701107) and Sacred Spring (700670). Since the EQOAEmu workbook's import
    the Nature Form and Wild Nature masteries give the workbook's Spirit Form
    and Wild Nature: Faun and Satyr (each with its body), and nothing teaches
    the four.
  - The shaman's Savage Healing (700643), Tribal Healing (700654) and
    Spiritform (700672): EQOA-Data's lines, none in the workbook, taught by
    nothing (Spiritform's line reads to no effect at all).
  - The necromancer's Revert Lich (701214, "Removes Lich form") and Stone of
    Desecration (700673, a zombie form with resist and AC caps): EQOA-Data's
    lines, none in the workbook, taught by nothing, and each read to no
    effect (the user: "remove them").
  - The enchanter's Soothing Visage (700544, "Group, Weapon proc, 5min") and
    Adoration (700661): EQOA-Data's, taught by nothing ("remove them").
  - The alchemist's Surefall Potion (700224), Fire Field Potion (700600) and
    Acidic Mix Potion (700024), each with a working workbook twin (Surefall,
    Fire Field, Acidic Mixture), Icy Mixtures (701017), Freezing Mixtures
    (701090), Summon Returning Potion (701052), Serpent's Heart (700816) and
    Badger Form (700127): EQOA-Data's, taught by nothing ("remove them with
    quest check guarding").
  - The enchanter's Eldritch Rune (701132), one of the six spells its level
    60 epic reward quest offers: its line is only a description ("Level 60
    Epic third spell option. ..."), no source has its effects, and it did
    nothing. Its option goes from the quest's choice; the other options stay
    byte for byte.

Characters holding one (two to five a spell on live, 2026-11-01) had it from
the start-with-every-spell setting. Each spell's class row (classspells) and
every book's row of it (spells) go, logged whole; a quest choice's option
that gives it goes, the choice's text logged before and after. Their records
(spellpattern, spelleffect) stay, so an effect of one still on a character
runs out as it is. A spell whose record is no longer the one named here is
refused, not removed. --undo puts back every row still gone, at its own
rowid, and every choice still as this left it, and drops the log. Report
only by default; a copy of the database first (SQLite's backup API) unless
--no-backup. The server reads a book at a login, the class lists and the
quests at its start. After a fresh EQOA-Data import (tools/import_eqoadata_spells.py,
tools/import_quests.py) run it again.

    tools/remove_leftover_spells.py [--db eqoa.db] [--apply | --undo] [--no-backup]
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
# (class, spellid, the record's name)
LEFTOVERS = [
    (7, 701064, "Nature Form"), (7, 701258, "Wild Nature"), (7, 701107, "Bixie Form"), (7, 700670, "Sacred Spring"),
    (8, 700643, "Savage Healing"), (8, 700654, "Tribal Healing"), (8, 700672, "Spiritform"),
    (12, 701132, "Eldritch Rune"),
    (11, 701214, "Revert Lich"), (11, 700673, "Stone of Desecration"),
    (12, 700544, "Soothing Visage"), (12, 700661, "Adoration"),
    (14, 700224, "Surefall Potion"), (14, 700600, "Fire Field Potion"), (14, 700024, "Acidic Mix Potion"),
    (14, 701017, "Icy Mixtures"), (14, 701090, "Freezing Mixtures"), (14, 701052, "Summon Returning Potion"),
    (14, 700816, "Serpent's Heart"), (14, 700127, "Badger Form"),
]
# leftovers whose quest choices this takes out with them (the only quest that
# may still name one: any other quest giving a leftover refuses it)
CHOICES_TAKEN = {701132}
LOG = "leftover_spells_log"
LOG_SCHEMA = f"""CREATE TABLE {LOG} (
    seq     INTEGER PRIMARY KEY,
    op      TEXT NOT NULL,     -- delete (a row, whole) or choice (a quest step's choices, before and after)
    tbl     TEXT NOT NULL,
    rid     INTEGER NOT NULL,  -- the row's rowid
    data    TEXT NOT NULL,     -- delete: the row, JSON of its columns; choice: the choices before
    after   TEXT NOT NULL DEFAULT '',
    note    TEXT NOT NULL,
    applied TEXT NOT NULL
)"""


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def columns(db, table):
    return [r[1] for r in db.execute(f"PRAGMA table_info({table})")]


def compact(o):
    return json.dumps(o, separators=(",", ":"), ensure_ascii=False)


def gives(option, sid):
    return str(sid) in [s.strip() for s in str(option.get("spells", "")).split(",")]


def quest_gives(db):
    """{spellid: [where]}: every spell a quest step gives, by its spells, its
    choices' options or its rewards by class (byclass)."""
    out = {}
    if not has_table(db, "queststeps"):
        return out
    cols = columns(db, "queststeps")
    want = [c for c in ("spells", "choices", "byclass") if c in cols]
    for row in db.execute(f"SELECT questid, step, {', '.join(want)} FROM queststeps").fetchall():
        qid, step, vals = row[0], row[1], dict(zip(want, row[2:]))
        ids = []
        ids += [x.strip() for x in str(vals.get("spells") or "").split(",")]
        for key in ("choices", "byclass"):
            text = vals.get(key) or ""
            if not text:
                continue
            try:
                o = json.loads(text)
            except ValueError:
                continue
            opts = o.get("options", []) if key == "choices" and isinstance(o, dict) else \
                (list(o.values()) if isinstance(o, dict) else [])
            for opt in opts:
                if isinstance(opt, dict):
                    ids += [x.strip() for x in str(opt.get("spells", "")).split(",")]
        for x in ids:
            if x.isdigit():
                out.setdefault(int(x), []).append(f"quest {qid} step {step}")
    return out


def plan(db):
    """(rows, refused): rows [(op, table, rowid, data, after, note)], refused [why]."""
    rows, refused = [], []
    given = quest_gives(db)
    ccols, scols = columns(db, "classspells"), columns(db, "spells")
    names = {}
    for cls, sid, name in LEFTOVERS:
        rec = db.execute("SELECT spellname FROM spellpattern WHERE spellid = ?", (sid,)).fetchone()
        if rec is None:
            continue  # no record: nothing of it to take out
        if (rec[0] or "").strip() != name:
            refused.append(f"{sid}: its record is {rec[0] if rec else 'gone'!r}, not {name!r}")
            continue
        if sid not in CHOICES_TAKEN and given.get(sid):
            refused.append(f"{sid} {name}: a quest gives it ({', '.join(given[sid][:3])}); left")
            continue
        names[sid] = name
        for r in db.execute(f"SELECT rowid, {', '.join(ccols)} FROM classspells WHERE class = ? AND spellid = ?",
                            (cls, sid)).fetchall():
            rows.append(("delete", "classspells", r[0], dict(zip(ccols, r[1:])), None, f"{name}: class {cls}'s list"))
        for r in db.execute(f"SELECT s.rowid, {', '.join('s.' + c for c in scols)}, coalesce(c.charName, '?'), "
                            "coalesce(c.level, 0) FROM spells s LEFT JOIN characters c ON c.serverid = s.serverid "
                            "WHERE s.spellid = ? ORDER BY s.rowid", (sid,)).fetchall():
            data = dict(zip(scols, r[1:1 + len(scols)]))
            who, lvl = r[1 + len(scols):]
            rows.append(("delete", "spells", r[0], data, None, f"{name}: the book of {who} (level {lvl})"))
    if has_table(db, "queststeps") and names:
        for rid, qid, step, text in db.execute("SELECT rowid, questid, step, choices FROM queststeps "
                                               "WHERE choices <> '' ORDER BY rowid").fetchall():
            try:
                c = json.loads(text)
            except ValueError:
                continue
            opts = c.get("options") if isinstance(c, dict) else None
            if not isinstance(opts, list):
                continue
            gone = [o for o in opts if any(gives(o, sid) for sid in names)]
            if not gone:
                continue
            if compact(c) != text:
                refused.append(f"quest {qid} step {step}: its choices are not as the importer writes them; left")
                continue
            c["options"] = [o for o in opts if o not in gone]
            rows.append(("choice", "queststeps", rid, text, compact(c),
                         f"quest {qid} step {step}: the option{'s' if len(gone) > 1 else ''} "
                         f"{', '.join(repr(o.get('answer')) for o in gone)} out of its choice"))
    return rows, refused


def backup(path):
    stamp = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{stamp}-pre-leftover-spells"
    src = sqlite3.connect(path)
    out = sqlite3.connect(dst)
    with out:
        src.backup(out)
    out.close()
    src.close()
    return dst


def apply(db, rows):
    now = datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")
    db.execute("BEGIN IMMEDIATE")
    if not has_table(db, LOG):
        db.execute(LOG_SCHEMA)
    for op, tbl, rid, data, after, note in rows:
        if op == "delete":
            db.execute(f"INSERT INTO {LOG} (op, tbl, rid, data, note, applied) VALUES ('delete', ?, ?, ?, ?, ?)",
                       (tbl, rid, json.dumps(data, sort_keys=True), note, now))
            db.execute(f"DELETE FROM {tbl} WHERE rowid = ?", (rid,))
        else:
            db.execute(f"INSERT INTO {LOG} (op, tbl, rid, data, after, note, applied) VALUES ('choice', ?, ?, ?, ?, ?, ?)",
                       (tbl, rid, data, after, note, now))
            db.execute("UPDATE queststeps SET choices = ? WHERE rowid = ?", (after, rid))
    db.execute("COMMIT")


def undo(db):
    """(put back, left): a row goes back at its rowid unless that rowid or its
    key is taken again since; a choice unless it changed since."""
    db.execute("BEGIN IMMEDIATE")
    back = left = 0
    for op, tbl, rid, data, after in db.execute(f"SELECT op, tbl, rid, data, after FROM {LOG} ORDER BY seq DESC").fetchall():
        if op == "choice":
            now = db.execute("SELECT choices FROM queststeps WHERE rowid = ?", (rid,)).fetchone()
            if now is None or now[0] != after:
                left += 1
                continue
            db.execute("UPDATE queststeps SET choices = ? WHERE rowid = ?", (data, rid))
            back += 1
            continue
        data = json.loads(data)
        taken = db.execute(f"SELECT count(*) FROM {tbl} WHERE rowid = ?", (rid,)).fetchone()[0]
        if tbl == "classspells":
            taken += db.execute("SELECT count(*) FROM classspells WHERE class = ? AND spellid = ?",
                                (data["class"], data["spellid"])).fetchone()[0]
        else:
            taken += db.execute("SELECT count(*) FROM spells WHERE serverid = ? AND spellid = ?",
                                (data["serverid"], data["spellid"])).fetchone()[0]
        if taken:
            left += 1
            continue
        cols = list(data)
        if tbl == "spells":  # its id is the rowid itself
            db.execute(f"INSERT INTO spells ({', '.join(cols)}) VALUES ({', '.join('?' for _ in cols)})",
                       [data[c] for c in cols])
        else:
            db.execute(f"INSERT INTO {tbl} (rowid, {', '.join(cols)}) VALUES (?, {', '.join('?' for _ in cols)})",
                       [rid] + [data[c] for c in cols])
        back += 1
    db.execute(f"DROP TABLE {LOG}")
    db.execute("COMMIT")
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--no-backup", action="store_true")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="remove them (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply removed")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    if (a.apply or a.undo) and not a.no_backup:
        print("backup:", backup(a.db))
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        if not has_table(db, LOG):
            print("nothing logged: nothing to undo")
            return
        back, left = undo(db)
        print(f"put back {back} rows, {left} left (changed since)")
        return
    rows, refused = plan(db)
    for why in refused:
        print("refused:", why)
    for op, tbl, rid, data, after, note in rows:
        print(f"  {tbl:11} rowid {rid:>7}  {note}")
    n = lambda t: sum(1 for r in rows if r[1] == t)
    print(f"{len(rows)} rows to change ({n('classspells')} from class lists, {n('spells')} from books, "
          f"{n('queststeps')} quest choices), {len(refused)} refused")
    if a.apply:
        apply(db, rows)
        print(f"changed {len(rows)} rows, logged in {LOG}")


if __name__ == "__main__":
    main()
