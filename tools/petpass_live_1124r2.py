#!/usr/bin/env python3
"""Live petition 5575, the reply of 03:48 UTC 2026-11-24 (Xylith): "the storyteller merchants are not selling items
right now from storyteller faction, only the luxury vendor is working".

Why: the four Storyteller Merchants (114462, 114463, 114493, 114494) carry npc_type 129, the merchant flag (0x01) on,
so CLIENT's interaction with them is the merchant window's request (0x004C, InteractWithTarget's SendOpU32(76)), never
a talk: the log shows "Merchant: Xylith opened "Storyteller Merchant" (npc 114463): 0 items for sale" each time, and
the token shop (handlers/tokenshop.go) is reached by a talk alone. The Luxury Storyteller Merc (114466) is 128, no
merchant flag, so his click is a talk and his faction shop opened. None of the four has a tunar stock (npcitem): the
merchant window had nothing to sell.

So: npc_type 129 -> 128 for the four, as the Luxury Merc's. Live only. Report only by default; --apply in one
transaction, a gm_audit line a row, log petpass_live_1124r2_log; --undo exact. WHEN: the NPC rows are read at a
start or the GM panel's reload_npcs; a client sees the flag when the NPC comes into view after that.

    python3 -E -s tools/petpass_live_1124r2.py --db eqoa.db [--apply | --undo]
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_live_1124r2_log"
DETAIL = "Live petition 5575: the Storyteller Merchants talk (no merchant flag) so their faction shop opens (tools/petpass_live_1124r2.py)"
IDS = (114462, 114463, 114493, 114494)
NAME, WAS, NOW = "Storyteller Merchant", 129, 128


def changes(db):
    out, lines, refusals = [], [], []
    if tr.rows(db, "SELECT 1 FROM npcs WHERE npc_name = 'Spell Archivist' LIMIT 1"):
        refusals.append("this database has Spell Archivists (Heirloom): Live only")
    for i in IDS:
        r = tr.get(db, "npcs", {"id": i})
        if not r or r["npc_name"] != NAME or r["npc_type"] not in (WAS, NOW):
            refusals.append(f"npc {i} is not a {NAME} of npc_type {WAS}: {r and (r['npc_name'], r['npc_type'])}")
            continue
        if tr.rows(db, "SELECT 1 FROM npcitem WHERE npcid = ? LIMIT 1", (i,)):
            refusals.append(f"npc {i} has a tunar stock (npcitem): the merchant window would lose it")
        out.append(("npcs", {"id": i}, {"npc_type": NOW}))
        lines.append(f"npcs {i} {NAME}: npc_type {r['npc_type']} -> {NOW}")
    if not tr.rows(db, "SELECT 1 FROM tokenshop WHERE shop = ? AND price > 0", (NAME,)):
        refusals.append(f"no tokenshop rows for {NAME}: run petpass_live_1124r.py --shop first")
    applied = all(tr.get(db, t, k) is not None and all(tr.get(db, t, k).get(c) == v for c, v in a.items())
                  for t, k, a in out)
    lines.insert(0, f"{len(out)} rows")
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("npcs", "npcitem", "tokenshop"), "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
