#!/usr/bin/env python3
"""The stat-boost masteries behind their quests (the user, 2026-10-29: "gate
the stat boost masteries behind the quests").

EQOA-Data's Misc guide (Quests/Misc/stat-boost-quests.json) opens seven of
the General masteries that add to a stat (not to its cap) by four trials
each, a trial a rank: "Congratulations, Bulging Muscles 1(5cm cost, 10str)
is now available for purchase." The trials are tools/quests/data/misc.json's
quests Misc/stat-boost-quests/<stat><n> (quest_import_quests names their ids:
tools/import_quests.py first), and the ranks the General list's as
tools/seed_masteries.py wrote them: Buldging Muscles (STR), Intestinal
Fortitude (STA), AGI (the guide's Avoid Danger), Unrivaled Grace (DEX), Sages
Wit (WIS), Superior Intellect (INT) and Razzle Dazzle (CHA), ranks 1 to 4,
each 10 a rank of its stat (checked: a rank whose effects say otherwise is
left, and said). The stat caps' masteries (Brawn, Vigor, ...) are not
touched.

Each rank's require_quests (entity/cmbuy.go: the quests to finish first; one
not in the game bars no one) is set to its trial's quest where it is empty;
one a GM has set is left, and said. The column is the server's
(database/cmcatalog.go adds it at its start); a database the new server has
not opened yet gets it here, as the server would add it. Nothing a character
owns (charcms) is taken: the report counts those who own a rank without its
quest finished.

Every change is logged in stat_mastery_gate_log (the rank, its value before
and after, the quest's key); --undo puts back each value still as this left
it (one changed since is left, and said) and drops the log, and a second
--apply is refused while the log is there. Both take a copy of the database
first (<db>.bak-<when>-pre-statgate). Report only by default. The server
reads the catalogue at its start: run this with the server stopped, or
restart it after.

    tools/gate_stat_masteries.py [--db eqoa.db] [--apply | --undo]
"""
import argparse
import datetime
import os
import sqlite3
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
GENERAL = 0
RANKS = 4
# (the quest key's stat, the mastery as the General list names it, the stat
# its effects name)
LINES = [
    ("str", "Buldging Muscles", "STR"),
    ("sta", "Intestinal Fortitude", "STA"),
    ("agi", "AGI", "AGI"),
    ("dex", "Unrivaled Grace", "DEX"),
    ("wis", "Sages Wit", "WIS"),
    ("int", "Superior Intellect", "INT"),
    ("cha", "Razzle Dazzle", "CHA"),
]
KEY = "Misc/stat-boost-quests/{stat}{n}"

LOG_SCHEMA = """CREATE TABLE stat_mastery_gate_log (
    id      INTEGER PRIMARY KEY,
    before  TEXT NOT NULL,
    after   TEXT NOT NULL,
    quest   TEXT NOT NULL,
    applied TEXT NOT NULL
)"""


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def has_column(db, table, col):
    return any(r[1] == col for r in db.execute(f"PRAGMA table_info({table})"))


def stats(text):
    """An effects list, "STR:10,STRMax:15", as {stat: n}, the keys lower case."""
    out = {}
    for f in str(text or "").split(","):
        k, _, v = f.partition(":")
        try:
            out[k.strip().lower()] = int(v)
        except ValueError:
            pass
    return out


def plan(db):
    """(changes, left, notes): each change (rank id, name, before, after, quest
    key); each rank left (name or what was looked for, why); notes on what
    is written all the same."""
    changes, left, notes = [], [], []
    has_col = has_column(db, "cmcatalog", "require_quests")
    for stat, mastery, statname in LINES:
        for n in range(1, RANKS + 1):
            key = KEY.format(stat=stat, n=n)
            want = f"{mastery} rank {n}"
            ranks = db.execute(
                "SELECT id, name, effects, level, " + ("require_quests" if has_col else "''") +
                " FROM cmcatalog WHERE category = ? AND lower(trim(mastery)) = lower(?) AND rank = ?",
                (GENERAL, mastery, n)).fetchall()
            if len(ranks) != 1:
                left.append((want, f"{len(ranks)} General ranks of that mastery and rank, not one"))
                continue
            rid, name, effects, level, now = ranks[0]
            if stats(effects) != {statname.lower(): 10 * n}:
                left.append((name, f"its effects are {effects!r}, not {statname}:{10 * n}"))
                continue
            row = None
            if has_table(db, "quest_import_quests"):
                row = db.execute("SELECT q.questid, q.name, q.enabled, q.minlevel FROM quest_import_quests i "
                                 "JOIN quests q ON q.questid = i.questid WHERE i.key = ?", (key,)).fetchone()
            if row is None:
                left.append((name, f"no quest {key} (tools/import_quests.py first)"))
                continue
            qid, qname, enabled, minlevel = row
            if now == str(qid):
                left.append((name, f"already needs {qname} ({qid})"))
                continue
            if now.strip():
                left.append((name, f"require_quests is {now!r}, a GM's: left"))
                continue
            changes.append((rid, name, now, str(qid), key))
            if not enabled:
                notes.append(f"{name}: {qname} ({qid}) is a draft: it bars no one until it is in the game")
            if minlevel > level:
                notes.append(f"{name}: level {level}, its quest {qname} from level {minlevel}")
    return changes, left, notes


def owners_short(db, changes):
    """The characters owning a rank to be gated whose quest they have not
    finished: {serverid: [rank name]}."""
    out = {}
    if not has_table(db, "charcms"):
        return out
    quests = has_table(db, "charquests")
    for rid, name, _, after, _ in changes:
        for (sid,) in db.execute("SELECT serverid FROM charcms WHERE cmid = ?", (rid,)):
            done = quests and db.execute("SELECT done FROM charquests WHERE serverid = ? AND questid = ?",
                                         (sid, int(after))).fetchone()
            if not done or done[0] <= 0:
                out.setdefault(sid, []).append(name)
    return out


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-statgate"
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def report(db):
    changes, left, notes = plan(db)
    for rid, name, before, after, key in changes:
        print(f"{rid:6d} {name}: require_quests {before!r} -> {after!r}  ({key})")
    for name, why in left:
        print(f"       {name}: left as it is ({why})")
    for note in notes:
        print(f"  note: {note}")
    short = owners_short(db, changes)
    print(f"{len(changes)} ranks to gate, {len(left)} left; characters owning a rank to be gated without its quest "
          f"finished: {len(short)} (nothing is taken from them)")
    for sid, names in sorted(short.items()):
        print(f"  character {sid}: {', '.join(names)}")
    return changes


def apply(db, changes):
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        if not has_column(db, "cmcatalog", "require_quests"):
            # as the server's start adds it (database/cmcatalog.go, ensureCMCatalog)
            db.execute("ALTER TABLE cmcatalog ADD COLUMN require_quests TEXT NOT NULL DEFAULT ''")
        db.execute(LOG_SCHEMA)
        for rid, _, before, after, key in changes:
            db.execute("INSERT INTO stat_mastery_gate_log (id, before, after, quest, applied) VALUES (?, ?, ?, ?, ?)",
                       (rid, before, after, key, when))
            db.execute("UPDATE cmcatalog SET require_quests = ? WHERE id = ?", (after, rid))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db, path, keep_copy):
    if not has_table(db, "stat_mastery_gate_log"):
        print("nothing logged: nothing to undo")
        return
    logged = db.execute("SELECT id, before, after FROM stat_mastery_gate_log ORDER BY id").fetchall()
    if keep_copy:
        print(f"copy: {backup(db, path)}")
    db.execute("BEGIN IMMEDIATE")
    try:
        back = 0
        for rid, before, after in logged:
            now = db.execute("SELECT require_quests FROM cmcatalog WHERE id = ?", (rid,)).fetchone()
            if now is None or now[0] != after:
                print(f"  {rid}: changed since ({now[0] if now else 'no row'!r}): left as it is")
                continue
            db.execute("UPDATE cmcatalog SET require_quests = ? WHERE id = ?", (before, rid))
            back += 1
        db.execute("DROP TABLE stat_mastery_gate_log")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"put back {back} of {len(logged)} values; restart the server to read them")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the changes (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply changed")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    if not has_table(db, "cmcatalog"):
        sys.exit(f"{a.db}: no cmcatalog (tools/seed_masteries.py fills it)")
    if a.undo:
        undo(db, a.db, not a.no_backup)
        return
    if has_table(db, "stat_mastery_gate_log"):
        n = db.execute("SELECT count(*) FROM stat_mastery_gate_log").fetchone()[0]
        print(f"already applied: {n} ranks logged in stat_mastery_gate_log")
        if a.apply:
            sys.exit("refused: --undo first")
        return
    changes = report(db)
    if not a.apply or not changes:
        if not a.apply:
            print("report only: --apply to write")
        return
    if not a.no_backup:
        print(f"copy: {backup(db, a.db)}")
    apply(db, changes)
    print(f"gated {len(changes)} ranks; restart the server to read them")


if __name__ == "__main__":
    main()
