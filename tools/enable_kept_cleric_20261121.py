#!/usr/bin/env python3
"""Petition 2556 (the owner: switch the homelands on): the 22 homes' L20 cleric quests the importer leaves as a GM
changed them (Jib's agent's step edits, 2026-11-18) switched on, the edits kept. Idempotent; a gm_audit line each."""
import json, sqlite3, sys, time
db = sqlite3.connect(sys.argv[1], timeout=30)
now = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()); n = 0
with db:
    for (qid,) in db.execute("SELECT questid FROM quests WHERE source LIKE '%homelands/%/L20/cleric%' AND enabled=0").fetchall():
        db.execute("UPDATE quests SET enabled=1 WHERE questid=?", (qid,))
        db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) VALUES (?,1,'dab','tool','','update','quests',?,?,?,?)",
                   (now, json.dumps({"questid": qid}), json.dumps({"enabled": 0}), json.dumps({"enabled": 1}),
                    "petition 2556 (the owner: switch the homelands on): a home's L20 cleric quest, kept as Jib's agent edited it, switched on"))
        n += 1
print("enabled", n, "kept L20 cleric quests")
