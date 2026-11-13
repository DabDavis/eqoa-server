#!/usr/bin/env python3
"""The Anvil of Sarek as the GM Jibwiz asked in petition 1956 (2026-11-13): "Anvil of sarek should be non agro (white
face), non attackable, permanently in that spot. If a player hits the distance my character is currently at from him he
should do a kick animation and hit them for 10,000 unresistable damage every second. That's all he does."

His distance: Jibwiz stood at (4738.6, 133.4, 4671.8), the Anvil's row 120968 at (4738.4, 132.0, 4651.9): 19.87 across
(the petition's own "19.9 away"). The zone is the new template ability kickzone (entity/kickzone.go):

  classtemplates 'anvil of sarek'  new: class -1 (his own), hp 100%, abilities
                                   [{"kind":"kickzone","radius":19.9,"damage":10000,"every":1,"name":"Kick"}]
  npctemplate 'anvil of sarek'     template 'roamer' (tools/seed_roamers.py's) -> 'anvil of sarek'

Non-aggro, unattackable and standing still are his rows', set by the batch of petitions 1955-1986 (board #1353): his
npcs row's NPC bit (npc_type 128: no one attacks him, he never wanders) and npcaggro 'Anvil of Sarek' aggressive 0.
This tool says whether they are so and changes neither. Refused, nothing written, when npctemplate's row is neither
'roamer' nor this tool's, or classtemplates already has another 'anvil of sarek'. Report only by default; --apply in one
transaction, a gm_audit line a row, log anvil_1956_log; --undo exact. The templates are read at the start and at a GM
panel edit of classtemplates or npctemplate. Needs the server with kickzone (an older one refuses the row as a whole:
the Anvil then fights by no template, as harmless as before).

    python3 -E -s tools/anvil_1956_20261113.py --db eqoa.db [--apply | --undo]
"""
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "anvil_1956_log"
DETAIL = "The Anvil of Sarek's kick zone (the GM Jibwiz's petition 1956): tools/anvil_1956_20261113.py"
NAME = "anvil of sarek"
ABILITIES = [{"kind": "kickzone", "radius": 19.9, "damage": 10000, "every": 1, "name": "Kick"}]
NOTE = ("petition 1956 (the GM Jibwiz): a player within 19.9 (his own distance) is kicked for 10,000, unresistable, "
        "every second; that is all he does; tools/anvil_1956_20261113.py")
TEMPLATE = {"name": NAME, "class": -1, "hp_pct": 100, "taken_pct": 100, "melee_pct": 100,
            "abilities": json.dumps(ABILITIES, separators=(",", ":")), "note": NOTE, "source": "tix4"}


def changes(db):
    out, lines, refusals = [], [], []
    ct = tr.get(db, "classtemplates", {"name": NAME})
    nt = tr.get(db, "npctemplate", {"name": NAME})
    mine_ct = ct is not None and all(ct.get(k) == v for k, v in TEMPLATE.items())
    mine_nt = nt is not None and nt["template"] == NAME
    for r in tr.rows(db, "SELECT id, npc_type, size, world, x_coord, z_coord FROM npcs WHERE npc_name = 'Anvil of Sarek'"):
        lines.append(f"npcs {r['id']}: npc_type {r['npc_type']} ({'the NPC bit: unattackable, standing' if r['npc_type'] & 0x80 else 'NO NPC bit yet: attackable, may wander'}), "
                     f"size {r['size']}, world {r['world']} at ({r['x_coord']:.1f}, {r['z_coord']:.1f})")
    ag = tr.rows(db, "SELECT * FROM npcaggro WHERE lower(npc_name) = ?", (NAME,)) if tr.table(db, "npcaggro") else []
    lines.append(f"npcaggro: {ag[0]['aggressive'] and 'aggressive' or 'not aggressive (white)'}" if ag else
                 "npcaggro: no row (his kind's rule)")
    if mine_ct and mine_nt:
        return [], lines, [], True
    if ct is not None and not mine_ct:
        refusals.append(f"classtemplates {NAME!r} is there already and not this tool's: {ct}")
    if nt is not None and nt["template"] not in ("roamer", NAME):
        refusals.append(f"npctemplate {NAME!r} is {nt['template']!r}, not 'roamer'")
    out.append(("classtemplates", {"name": NAME}, TEMPLATE))
    lines.append(f"classtemplates {NAME!r}: new, abilities {TEMPLATE['abilities']}")
    if nt is None:
        out.append(("npctemplate", {"name": NAME}, {"name": NAME, "template": NAME, "note": NOTE, "source": "tix4"}))
        lines.append(f"npctemplate {NAME!r}: new, template {NAME!r}")
    else:
        out.append(("npctemplate", {"name": NAME}, {**nt, "template": NAME, "note": NOTE}))
        lines.append(f"npctemplate {NAME!r}: {nt['template']!r} -> {NAME!r}")
    return out, lines, refusals, False


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("classtemplates", "npctemplate", "npcs"), "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
