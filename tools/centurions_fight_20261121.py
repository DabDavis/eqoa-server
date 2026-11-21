#!/usr/bin/env python3
"""The owner 2026-11-21 (camp audit vs MasterVG782's guide): the frosteye centurions fight again: the 'attacks no
one' bit (npc_type 0x80) an earlier camp tool left on them cleared. Idempotent; a gm_audit line each."""
import json, sqlite3, sys, time
db = sqlite3.connect(sys.argv[1], timeout=30); now = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()); n = 0
with db:
    for i, t in db.execute("SELECT id,npc_type FROM npcs WHERE lower(npc_name)='frosteye centurion' AND (npc_type & 128)").fetchall():
        db.execute("UPDATE npcs SET npc_type=? WHERE id=?", (t & ~128, i))
        db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) VALUES (?,1,'dab','tool','','update','npcs',?,?,?,?)",
                   (now, json.dumps({"id": i}), json.dumps({"npc_type": t}), json.dumps({"npc_type": t & ~128}), "the owner 2026-11-21: the frosteye centurions fight (camp audit)"))
        n += 1
print("centurions made to fight:", n)
