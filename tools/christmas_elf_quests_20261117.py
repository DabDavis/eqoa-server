#!/usr/bin/env python3
"""The Holiday Elf's quests rewritten (work entry 1597, jibrish, owner-approved): the lost fourteen years.

Applies christmas_elf_quests_20261117.json (the entry's rows as written):
- 102901 "Gifts for Freeport": its new words, once only now (repeatable 0), day group christmas-gifts; 102902 "Carols
  for the Guard": its new words, still daily; both with the entry's questions at the offer (quests.asks) and their
  steps' new words, journals naming where each NPC stands, the visits in any order;
- three new daily gift runs, 102904 "More Gifts for Freeport", 102905 "Still More Gifts", 102906 "The Bottomless
  Sack", each requiring 102901, in day group christmas-gifts (one a day with 102901, taken in turn);
- npcdialog 2982's new line and eight new lines (source christmas-2026, weight 2).
Needs the server with quests.asks and quests.daygroup (holiday-elf) for the asks and the day group to work; the columns
are made here when the server has not made them yet.

A report by default; --apply writes it with gm_audit lines and keeps every row as it was in --applied; --undo puts them
back. Live at a reload of the quests (a GM panel touch of a Quests row) or a restart; npcdialog at once.

    christmas_elf_quests_20261117.py --db eqoa.db --applied FILE [--apply | --undo] [--rows FILE.json]
"""
import argparse, json, os, sqlite3, sys, time

SRC = "christmas-2026"
ap = argparse.ArgumentParser()
ap.add_argument("--db", required=True)
ap.add_argument("--applied", required=True)
ap.add_argument("--rows", default=os.path.join(os.path.dirname(os.path.abspath(__file__)), "christmas_elf_quests_20261117.json"))
g = ap.add_mutually_exclusive_group()
g.add_argument("--apply", action="store_true")
g.add_argument("--undo", action="store_true")
a = ap.parse_args()
db = sqlite3.connect(a.db, timeout=30)
db.row_factory = sqlite3.Row
now = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def audit(action, tbl, key, before, after):
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) VALUES (?, 1, 'dab', 'tool', '', ?, ?, ?, ?, ?, ?)",
               (now, action, tbl, json.dumps(key), None if before is None else json.dumps(before), None if after is None else json.dumps(after),
                "work entry 1597: the Holiday Elf's quests rewritten; tools/christmas_elf_quests_20261117.py"))


def cols(tbl):
    return [r[1] for r in db.execute(f"PRAGMA table_info({tbl})")]


for tbl, col, decl in (("quests", "asks", "TEXT NOT NULL DEFAULT ''"), ("quests", "daygroup", "TEXT NOT NULL DEFAULT ''"),
                       ("queststeps", "anyorder", "INTEGER NOT NULL DEFAULT 0")):
    if col not in cols(tbl):
        if a.undo:
            continue
        db.execute(f"ALTER TABLE {tbl} ADD COLUMN {col} {decl}")


def rows(sql, args=()):
    return [dict(r) for r in db.execute(sql, args)]


def put(tbl, row):
    keys = [k for k in row if k in cols(tbl)]
    db.execute(f"INSERT INTO {tbl} ({', '.join(keys)}) VALUES ({', '.join('?' * len(keys))})", [row[k] for k in keys])


if a.undo:
    j = json.load(open(a.applied))
    for qid in j["inserted"]:
        db.execute("DELETE FROM queststeps WHERE questid = ?", (qid,))
        db.execute("DELETE FROM quests WHERE questid = ?", (qid,))
        audit("delete", "quests", {"questid": qid}, {"inserted": True}, None)
    for qid, was in j["updated"].items():
        db.execute("DELETE FROM queststeps WHERE questid = ?", (int(qid),))
        db.execute("DELETE FROM quests WHERE questid = ?", (int(qid),))
        put("quests", was["quest"])
        for st in was["steps"]:
            put("queststeps", st)
        audit("update", "quests", {"questid": int(qid)}, None, {"restored": True})
    for d in j["dialog_inserted"]:
        db.execute("DELETE FROM npcdialog WHERE id = ?", (d,))
    for d, line in j["dialog_updated"].items():
        db.execute("UPDATE npcdialog SET line = ? WHERE id = ?", (line, int(d)))
    audit("update", "npcdialog", {"ids": list(j["dialog_updated"]) + j["dialog_inserted"]}, None, {"restored": True})
    db.commit(); os.rename(a.applied, a.applied + ".undone"); print("undone"); sys.exit()
if a.apply and os.path.exists(a.applied):
    sys.exit("applied already")
data = json.load(open(a.rows))
asks = json.dumps(data["asks"], ensure_ascii=False, separators=(",", ":"))
done = {"inserted": [], "updated": {}, "dialog_inserted": [], "dialog_updated": {}}
for q in data["quests"]:
    qid, change = q["questid"], q["change"]
    have = rows("SELECT * FROM quests WHERE questid = ?", (qid,))
    if change == "insert":
        if have:
            sys.exit(f"quest {qid} is here already ({have[0]['name']!r}): nothing done")
        row = dict(q["quest"], questid=qid)
        row.setdefault("source", SRC)
        put("quests", row)
        for st in q["steps"]:
            put("queststeps", dict(st, questid=qid))
        done["inserted"].append(qid)
        audit("insert", "quests", {"questid": qid}, None, {"name": row["name"], "steps": len(q["steps"])})
        print(f"quest {qid} {row['name']!r}: new, {len(q['steps'])} steps")
        continue
    if not have:
        sys.exit(f"no quest {qid} to update: nothing done")
    was_steps = rows("SELECT * FROM queststeps WHERE questid = ? ORDER BY step", (qid,))
    done["updated"][str(qid)] = {"quest": have[0], "steps": was_steps}
    fields = dict(q["quest"], asks=asks)
    keys = [k for k in fields if k in cols("quests")]
    db.execute(f"UPDATE quests SET {', '.join(k + ' = ?' for k in keys)} WHERE questid = ?", [fields[k] for k in keys] + [qid])
    for st in q["steps"]:
        cur = rows("SELECT * FROM queststeps WHERE questid = ? AND step = ?", (qid, st["step"]))
        if cur:
            keys = [k for k in st if k in cols("queststeps") and k != "step"]
            db.execute(f"UPDATE queststeps SET {', '.join(k + ' = ?' for k in keys)} WHERE questid = ? AND step = ?",
                       [st[k] for k in keys] + [qid, st["step"]])
        else:
            put("queststeps", dict(st, questid=qid))
    audit("update", "quests", {"questid": qid}, {"name": have[0]["name"], "repeatable": have[0]["repeatable"]}, {"fields": sorted(q["quest"]), "steps": len(q["steps"])})
    print(f"quest {qid} {have[0]['name']!r}: new words, {len(q['steps'])} steps")
# the asks on the inserted quests too? The entry gives them to 102901 and 102902 alone.
for d in data["dialog_update"]:
    old = db.execute("SELECT line FROM npcdialog WHERE id = ?", (d["id"],)).fetchone()
    if old is None:
        sys.exit(f"no npcdialog {d['id']}: nothing done")
    done["dialog_updated"][str(d["id"])] = old[0]
    db.execute("UPDATE npcdialog SET line = ? WHERE id = ?", (d["line"], d["id"]))
for d in data["dialog_insert"]:
    nid = db.execute("INSERT INTO npcdialog (town, role, name, line, weight, source) VALUES (?, ?, ?, ?, 2, ?)",
                     (d["town"], d["role"], d["name"], d["line"], SRC)).lastrowid
    done["dialog_inserted"].append(nid)
audit("update", "npcdialog", {"updated": list(done["dialog_updated"]), "inserted": done["dialog_inserted"]}, None, None)
print(f"npcdialog: {len(done['dialog_updated'])} line changed, {len(done['dialog_inserted'])} added")
if a.apply:
    json.dump(done, open(a.applied, "w")); db.commit(); print("applied")
else:
    db.rollback()
