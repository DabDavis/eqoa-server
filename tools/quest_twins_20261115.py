#!/usr/bin/env python3
"""Two tellings of one errand exclude each other (petition 2199, Heirloom, a player: "argot bittersteel is asking for
the runed tablet. i already retrieved it but hes sending me back to get it again").

The same errand came in from two guides: 100868 "The Runed Clay Tablet" (Crikston Gundlefop's class and race quest,
tools/quests/data/30-40-49.json) and 100770 "The Al'Karad Tablet" (Argot Bittersteel's own, every class,
tools/quests/data/23-47.json): the same ruins, the same savage bear, the same hand-in at Argot and the same reward
(item 8086). A character who finished one was offered the other at once. The quests' excludes column
(database.Quest.Excludes, handlers/quests.go's mayTakeAtLocked: a quest is neither offered nor taken by one who has
finished a quest it excludes, or has one under way) now keeps them apart; the files carry it for a re-import.

What --apply writes, in one transaction, each change a gm_audit line and a row of quest_twins_1115_log:
  - quests.excludes: 100770 excludes 100868 and 100868 excludes 100770 (added to what the column holds); the column
    is made when the database lacks it (the server makes it at its start too);
  - quest_import_quests.digest of the two (tools/import_quests.py's digest), when it was the quest's own before the
    change, so a later import still knows them as its own (a GM's changed quest is left and said);
  - charquests: a character who finished one of the two and holds the other unfinished (done 0: under way, or a row
    left at step 0) loses the unfinished one's row (a player holds 100770 at step 1); the row is logged whole.
A character with both under way, or both finished, is reported only.

Report by default; --apply; --undo (the excludes as they were, the rows back where the character has none of the
quest now, the digests as they were; then the log dropped). A second --apply finds its log and does nothing.
Run --apply in a keep restart's before.sh (tools/swap_keep.sh: the server stopped and everyone saved): quests load at
the server's start, and the quest engine reads charquests at a character's first use after it, so no one need log
out. Run while the server runs, a character online could keep the row in memory and write it back.

--pairs reports other candidate twins across the two guides' files (the same giver or last step's NPC and the same
reward item), report only, for a GM's decision.

    python3 -E -s tools/quest_twins_20261115.py --db eqoa.db [--apply | --undo] [--pairs]
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import import_quests as iq  # noqa: E402  (its digest, the import's own)

LOG = "quest_twins_1115_log"
WHO, VIA = "dab", "tool"
DETAIL = "two tellings of one errand exclude each other (petition 2199, Cardile): tools/quest_twins_20261115.py"
# questid -> (its name, its import key)
TWINS = {
    100868: ("The Runed Clay Tablet", "30,40,49/group-1-good/L30-a"),
    100770: ("The Al'Karad Tablet", "23-47/30/blakedown"),
}
PAIRS = [(100868, 100770)]
GUIDE_FILES = ("23-47.json", "30-40-49.json")


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def column(db, tbl, col):
    return any(r[1] == col for r in db.execute(f"PRAGMA table_info({tbl})"))


def go_json(m):
    return None if m is None else json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def now_text():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def audit(db, action, tbl, key, before, after):
    row = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
               (now_text(), row[0] if row else 0, WHO, VIA, action, tbl, go_json(key), go_json(before), go_json(after), DETAIL))


def ids(s):
    out = []
    for part in (s or "").replace("|", ",").split(","):
        part = part.strip()
        if part.lstrip("-").isdigit() and int(part) not in out:
            out.append(int(part))
    return out


def check(db):
    """The two quests as this tool knows them, or why not."""
    why = []
    for qid, (name, key) in TWINS.items():
        r = db.execute("SELECT name FROM quests WHERE questid = ?", (qid,)).fetchone()
        if r is None:
            why.append(f"no quest {qid}")
        elif r[0] != name:
            why.append(f"quest {qid} is {r[0]!r}, not {name!r}")
        if table(db, "quest_import_quests"):
            k = db.execute("SELECT questid FROM quest_import_quests WHERE key = ?", (key,)).fetchone()
            if k is not None and k[0] != qid:
                why.append(f"the import's {key!r} is quest {k[0]}, not {qid}")
    return why


def plan(db):
    """(excludes changes [(qid, before, after)], rows to take [(row dict, finished twin)], reported lines)."""
    has_col = column(db, "quests", "excludes")
    ex = []
    for a, b in PAIRS:
        for q, other in ((a, b), (b, a)):
            before = db.execute("SELECT excludes FROM quests WHERE questid = ?", (q,)).fetchone()[0] if has_col else ""
            after = ids(before)
            if other not in after:
                after.append(other)
                ex.append((q, before, ", ".join(map(str, after))))
    take, said = [], []
    cols = [r[1] for r in db.execute("PRAGMA table_info(charquests)")]
    for a, b in PAIRS:
        rows = {}
        for r in db.execute(f"SELECT {', '.join(cols)} FROM charquests WHERE questid IN (?, ?) ORDER BY serverid, questid", (a, b)):
            d = dict(zip(cols, r))
            rows.setdefault(d["serverid"], {})[d["questid"]] = d
        for sid, held in sorted(rows.items()):
            if len(held) < 2:
                continue
            ra, rb = held[a], held[b]
            fa, fb = ra["done"] > 0, rb["done"] > 0
            if fa and not fb:
                take.append((rb, a))
            elif fb and not fa:
                take.append((ra, b))
            elif fa and fb:
                said.append(f"character {sid} finished both {a} and {b}: left")
            else:
                said.append(f"character {sid} has both {a} (step {ra['step']}) and {b} (step {rb['step']}) under way: left")
    return ex, take, said


def name_of(db, sid):
    r = db.execute("SELECT charname FROM characters WHERE serverid = ?", (sid,)).fetchone() if table(db, "characters") else None
    return r[0] if r else "?"


def ensure_log(db):
    db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, kind TEXT NOT NULL, "
               "questid INTEGER NOT NULL, serverid INTEGER NOT NULL DEFAULT 0, before TEXT, after TEXT)")


def apply(db):
    if table(db, LOG) and db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0] > 0:
        print(f"already applied ({LOG} has rows): nothing done")
        return 0
    why = check(db)
    if why:
        print("refused: " + "; ".join(why))
        return 2
    db.execute("BEGIN IMMEDIATE")
    try:
        ensure_log(db)
        if not column(db, "quests", "excludes"):
            db.execute("ALTER TABLE quests ADD COLUMN excludes TEXT NOT NULL DEFAULT ''")
            print("made quests.excludes")
        # the digests before, to tell an import's own quest from a GM's changed one
        own = {}
        if table(db, "quest_import_quests"):
            for qid, (_, key) in TWINS.items():
                r = db.execute("SELECT digest FROM quest_import_quests WHERE key = ?", (key,)).fetchone()
                if r is not None:
                    own[qid] = (key, r[0], r[0] == iq.digest(db, qid))
        ex, take, said = plan(db)
        at = now_text()
        for qid, before, after in ex:
            db.execute("UPDATE quests SET excludes = ? WHERE questid = ?", (after, qid))
            db.execute(f"INSERT INTO {LOG} (at, kind, questid, before, after) VALUES (?, 'excludes', ?, ?, ?)", (at, qid, before, after))
            audit(db, "update", "quests", {"questid": qid}, {"excludes": before}, {"excludes": after})
            print(f"quest {qid} {TWINS[qid][0]!r}: excludes {before!r} -> {after!r}")
        for qid, (key, was, mine) in sorted(own.items()):
            if not mine:
                print(f"quest {qid}: its import digest was not its own before (a GM's change): left")
                continue
            new = iq.digest(db, qid)
            if new != was:
                db.execute("UPDATE quest_import_quests SET digest = ? WHERE key = ?", (new, key))
                db.execute(f"INSERT INTO {LOG} (at, kind, questid, before, after) VALUES (?, 'digest', ?, ?, ?)", (at, qid, was, new))
        for row, finished in take:
            db.execute("DELETE FROM charquests WHERE serverid = ? AND questid = ?", (row["serverid"], row["questid"]))
            db.execute(f"INSERT INTO {LOG} (at, kind, questid, serverid, before) VALUES (?, 'charquest', ?, ?, ?)",
                       (at, row["questid"], row["serverid"], go_json(row)))
            audit(db, "delete", "charquests", {"serverid": row["serverid"], "questid": row["questid"]}, row, None)
            print(f"character {row['serverid']} {name_of(db, row['serverid'])}: finished {finished}; its {row['questid']} "
                  f"(step {row['step']}) taken")
        for s in said:
            print(s)
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"applied: {len(ex)} excludes, {len(take)} rows taken")
    return 0


def undo(db):
    if not table(db, LOG):
        print("nothing to undo (no log)")
        return 0
    db.execute("BEGIN IMMEDIATE")
    try:
        rows = db.execute(f"SELECT id, kind, questid, serverid, before, after FROM {LOG} ORDER BY id DESC").fetchall()
        for _, kind, qid, sid, before, after in rows:
            if kind == "excludes":
                cur = db.execute("SELECT excludes FROM quests WHERE questid = ?", (qid,)).fetchone()
                if cur is None or cur[0] != after:
                    print(f"quest {qid}: excludes changed since ({cur[0] if cur else 'gone'!r}): left")
                    continue
                db.execute("UPDATE quests SET excludes = ? WHERE questid = ?", (before or "", qid))
                audit(db, "update", "quests", {"questid": qid}, {"excludes": after}, {"excludes": before or ""})
                print(f"quest {qid}: excludes back to {before!r}")
            elif kind == "digest":
                key = TWINS[qid][1]
                db.execute("UPDATE quest_import_quests SET digest = ? WHERE key = ? AND digest = ?", (before, key, after))
            elif kind == "charquest":
                row = json.loads(before)
                if db.execute("SELECT 1 FROM charquests WHERE serverid = ? AND questid = ?", (sid, qid)).fetchone():
                    print(f"character {sid}: has quest {qid} again: its old row left out")
                    continue
                cols = [r[1] for r in db.execute("PRAGMA table_info(charquests)") if r[1] in row]
                db.execute(f"INSERT INTO charquests ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [row[c] for c in cols])
                audit(db, "insert", "charquests", {"serverid": sid, "questid": qid}, None, row)
                print(f"character {sid}: quest {qid} back at step {row.get('step')}")
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return 0


def report(db):
    why = check(db)
    if why:
        print("would refuse: " + "; ".join(why))
        return
    if table(db, LOG) and db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0] > 0:
        print(f"applied already ({LOG})")
    ex, take, said = plan(db)
    for qid, before, after in ex:
        print(f"would set quest {qid} {TWINS[qid][0]!r}: excludes {before!r} -> {after!r}")
    for row, finished in take:
        print(f"would take character {row['serverid']} {name_of(db, row['serverid'])}'s {row['questid']} (step {row['step']}): "
              f"it finished {finished}")
    for s in said:
        print(s)
    print(f"report: {len(ex)} excludes, {len(take)} rows to take")


def pairs(db):
    """Candidate twins across the two guides: the same giver or last step's NPC, the same reward item."""
    if not table(db, "quest_import_quests"):
        print("no quest_import_quests: no guides' quests to pair")
        return
    by_file = {f: [] for f in GUIDE_FILES}
    for key, qid, fname in db.execute("SELECT key, questid, file FROM quest_import_quests"):
        if fname in by_file:
            q = db.execute("SELECT name, giver FROM quests WHERE questid = ?", (qid,)).fetchone()
            if q is None:
                continue
            steps = db.execute("SELECT target, rewarditems, byclass FROM queststeps WHERE questid = ? ORDER BY step", (qid,)).fetchall()
            last = (steps[-1][0] if steps else "") or ""
            rewards = set()
            for _, ri, bc in steps:
                for part in (ri or "").split(","):
                    pid = part.split("*")[0].strip()
                    if pid.isdigit():
                        rewards.add(int(pid))
                try:
                    for v in (json.loads(bc) if bc else {}).values():
                        for part in str(v.get("items", "") if isinstance(v, dict) else "").split(","):
                            pid = part.split("*")[0].strip()
                            if pid.isdigit():
                                rewards.add(int(pid))
                except (ValueError, AttributeError):
                    pass
            by_file[fname].append(dict(qid=qid, key=key, name=q[0], npcs={(q[1] or "").lower(), last.lower()} - {""}, rewards=rewards))
    a, b = (by_file[f] for f in GUIDE_FILES)
    found = 0
    for x in a:
        for y in b:
            if x["npcs"] & y["npcs"] and x["rewards"] & y["rewards"]:
                found += 1
                print(f"pair: {x['qid']} {x['name']!r} ({x['key']}) ~ {y['qid']} {y['name']!r} ({y['key']}): "
                      f"NPC {sorted(x['npcs'] & y['npcs'])}, reward {sorted(x['rewards'] & y['rewards'])}")
    print(f"pairs: {found} (report only)")


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", default="eqoa.db")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    ap.add_argument("--pairs", action="store_true", help="report other candidate twins of the two guides")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=5)
    try:
        if a.pairs:
            pairs(db)
            return 0
        if a.apply:
            return apply(db)
        if a.undo:
            return undo(db)
        report(db)
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
