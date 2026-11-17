#!/usr/bin/env python3
"""The holiday quests' visits in any order (the owner, 2026-11-17: "for the xmas quests it has multiple quest npcs to
talk to for singing and giving gifts, lets make it completeable in any order, players can speak with bowyer koll or
tailor bariston in any order for example").

Sets queststeps.anyorder on the visit steps of 102901 "Gifts for Freeport" (Tailor Bariston, Bowyer Koll) and 102902
"Carols for the Guard" (Captain Scilond, Gylod Ungrim): the server (xmas-quests-anyorder) lets a player do them in any
order, the return to the Holiday Elf after both. A player part way through keeps what is done (a row on the second
visit reads as the first done). Every step talk at an NPC, or nothing is changed.

A report by default; --apply writes it with gm_audit lines and keeps what it did in --applied; --undo puts it back.
Needs the anyorder column (the server makes it at its start; this makes it too). Live at a reload of the quests (the
GM panel's touch of a Quest steps row) or a restart.

    xmas_quests_anyorder_20261117.py --db eqoa.db --applied FILE [--apply | --undo]
"""
import argparse, json, os, sqlite3, sys, time

STEPS = {102901: (1, 2), 102902: (1, 2)}
ap = argparse.ArgumentParser()
ap.add_argument("--db", required=True)
ap.add_argument("--applied", required=True)
g = ap.add_mutually_exclusive_group()
g.add_argument("--apply", action="store_true")
g.add_argument("--undo", action="store_true")
a = ap.parse_args()
db = sqlite3.connect(a.db, timeout=30)
now = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def audit(key, before, after):
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) VALUES (?, 1, 'dab', 'tool', '', 'update', 'queststeps', ?, ?, ?, ?)",
               (now, json.dumps(key), json.dumps(before), json.dumps(after), "the owner: the holiday quests' visits in any order; tools/xmas_quests_anyorder_20261117.py"))


if "anyorder" not in {r[1] for r in db.execute("PRAGMA table_info(queststeps)")}:
    if a.undo:
        sys.exit("no anyorder column: nothing to undo")
    db.execute("ALTER TABLE queststeps ADD COLUMN anyorder INTEGER NOT NULL DEFAULT 0")
if a.undo:
    j = json.load(open(a.applied))
    for k, was in j["steps"].items():
        q, s = map(int, k.split(":"))
        db.execute("UPDATE queststeps SET anyorder = ? WHERE questid = ? AND step = ?", (was, q, s))
        audit({"questid": q, "step": s}, {"anyorder": 1}, {"anyorder": was})
    db.commit(); os.rename(a.applied, a.applied + ".undone"); print("undone"); sys.exit()
if a.apply and os.path.exists(a.applied):
    sys.exit("applied already")
done = {"steps": {}}
for q, steps in STEPS.items():
    for s in steps:
        r = db.execute("SELECT kind, target, anyorder FROM queststeps WHERE questid = ? AND step = ?", (q, s)).fetchone()
        if r is None or r[0] != "talk":
            sys.exit(f"quest {q} step {s} is {r and r[0]}, not a talk step: nothing done")
        if r[2] == 1:
            sys.exit(f"quest {q} step {s} is any order already: nothing done")
        done["steps"][f"{q}:{s}"] = r[2]
        db.execute("UPDATE queststeps SET anyorder = 1 WHERE questid = ? AND step = ?", (q, s))
        audit({"questid": q, "step": s}, {"anyorder": r[2]}, {"anyorder": 1})
        print(f"quest {q} step {s} ({r[1]}): any order")
mid = db.execute("SELECT questid, step, count(*) FROM charquests WHERE questid IN (102901, 102902) AND done = 0 AND step > 0 GROUP BY 1, 2").fetchall()
print("players part way (quest, step, how many; each keeps what is done):", mid)
if a.apply:
    json.dump(done, open(a.applied, "w")); db.commit(); print("applied")
else:
    db.rollback()
