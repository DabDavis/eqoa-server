#!/usr/bin/env python3
"""Heirloom petitions 2852 and 2853 (Wulfric, 2026-11-24).

2852 (Creature or NPC): "Scornblade Vo'Dleth is floating in air on a ramp in the Dark Elf Warrior guild."

Scornblade Vo`Dleth (world 0, Neriak's warrior guild; Heirloom npcs 196275, Live 196306, the same row on both: the
server view's target, object 102105) spawned at (25238, 7.4, 8630). The ground under it (cmd/campground, the server's
own floors): the guild's ramp at 2.56 (placed, rising north to south about 0.5 a unit, 1.7 at z 8628.3 to 3.7 at
8632.2), the terrain at 0.64 under the ramp; no floor between 2.56 and 9.4. The player stood 4.8 away at 7.34, on the
landing the ramp climbs to, which ends at z ~8627.8: the spawn's 7.4 was the landing's height carried out over the
ramp. So y 7.4 -> 2.56, the ramp under it (not embedded), keeping x, z and facing 129.

It does not wander: NPC bit 0x80 (npc_type 128) and a player race's body; no npcwander row on either shard.

2853 (Item, loot or merchant): "Delf Warrior lv 3 spell scroll Kick is a sword icon in the inventory instead of a
scroll." The Kick scroll (itempattern 8563, both shards; Further Training's reward from Scornblade Vo`Dleth and the
other warrior trainers) has the scroll icon, -607649328, as its itemicon, but its second icon (unk2, the one CLIENT
shows on Frontiers: VIClient::ItemIcon, craftitems.go craftItemIcon) was 506409893, the scimitars' (Rusty Scimitar,
Dueling Sabre and the rest wear it). Every other record with the scroll icon (1,252 on Heirloom, 1,213 on Live) has
unk2 0, its siblings Stomp 8567 and Taunt 707000 too: unk2 506409893 -> 0. By the same rule, the one other spell
scroll (a "scribe" description, worn nowhere) that does not show the scroll: Debilitating Hex (8819, the shaman's
level 60) wore -430139392, the cloth leggings' icon (Acolyte's Leggings 991728): itemicon -> -607649328, copied
from Stomp's. Item records reach a client at its next login.

The npcs row is found by name, world and its old place, and must read as it did when this was made (or as this writes
it); else refused.

Report only by default; --apply in one transaction, a gm_audit line a row, log petpass_both_1124x_log; --undo
exact. NPC rows are read at the server's next start, or at once by the GM panel's reload_npcs.

    python3 -E -s tools/petpass_both_1124x.py --db eqoa.db [--apply | --undo]
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_both_1124x_log"
DETAIL = ("Heirloom petitions 2852-2853: Scornblade Vo`Dleth down from the air onto the Neriak warrior guild's ramp; "
          "the Kick and Debilitating Hex scrolls show the scroll icon (tools/petpass_both_1124x.py)")
NAME, WORLD = "Scornblade Vo`Dleth", 0
OLD = {"x_coord": 25238.0, "y_coord": 7.4, "z_coord": 8630.0, "facing": 129}
NEW = {"x_coord": 25238.0, "y_coord": 2.56, "z_coord": 8630.0, "facing": 129}
SCROLL = -607649328  # the spell scrolls' icon (Stomp 8567, Taunt 707000, 1,252 more)
# (patternid, name, column, old, new)
ITEMS = [
    (8563, "Kick", "unk2", 506409893, 0),
    (8819, "Debilitating Hex", "itemicon", -430139392, SCROLL),
]


def near(r, place):
    return all(abs(r[c] - v) < 0.01 for c, v in place.items())


def changes(db):
    out, lines, refusals = [], [], []
    found = [r for r in tr.rows(db, "SELECT * FROM npcs WHERE npc_name = ? AND world = ?", (NAME, WORLD))
             if near(r, OLD) or near(r, NEW)]
    if len(found) != 1:
        refusals.append(f"2852: {len(found)} {NAME} rows at ({OLD['x_coord']}, {OLD['z_coord']}) at height "
                        f"{OLD['y_coord']} or {NEW['y_coord']}, expected 1 (a GM's edit since?)")
    else:
        r = found[0]
        out.append(("npcs", {"id": r["id"]}, dict(r, **NEW)))
        lines.append(f"2852: npcs {r['id']} {NAME} ({r['x_coord']}, {r['y_coord']}, {r['z_coord']}) -> height "
                     f"{NEW['y_coord']}, the ramp under it")
    for pid, name, col, old, new in ITEMS:
        r = tr.get(db, "itempattern", {"patternid": pid})
        if r is None or r["itemname"] != name:
            refusals.append(f"2853: itempattern {pid} is not {name}")
        elif r[col] not in (old, new):
            refusals.append(f"2853: itempattern {pid} {name} {col} {r[col]}, expected {old} (changed since?)")
        else:
            out.append(("itempattern", {"patternid": pid}, dict(r, **{col: new})))
            lines.append(f"2853: itempattern {pid} {name} {col} {r[col]} -> {new}")
    if refusals:
        out = []
    applied = bool(out) and all(tr.get(db, t, k) == a for t, k, a in out)
    lines.insert(0, f"{len(out)} rows")
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("npcs", "itempattern"), "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, argv=sys.argv[1:], doc=__doc__))
