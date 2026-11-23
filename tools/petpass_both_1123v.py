#!/usr/bin/env python3
"""Live petition 5573 (Warsloth, 2026-11-23, Creature or NPC): "alot of tame crocodiles are in water of outskirts of
island", and Heirloom petition 2819 (Darkhavictwo, Item): "the 'broken arrowhead' at phantoms is supposed to look like
a red-orange color flint arrowhead. it currently looks like a wooden arrow". Both shards.

  5573: Croc Island, off the coast south of Freeport (world 0, about (25470, 17330)). "Tame crocodile" is the
        creatures' own name, not their manner: the petition's server view shows them hostile by nature (attacks on
        sight within 21, no faction, no town near: the island's one players'-race body is Croc), so no faction or
        npcaggro row is wanted. What was wrong is where they stood: of the eight in view, six swam together in the
        island's pond (x 25406-25490, z 17300-17370, the surface 62; seen at y 60.5) and two in the sea off its south
        shore (y 49.8), up to 66 from their spawns. npcmove's tame crocodile row (the seed's, as every crocodile's)
        says "both": the wander's legs end in the water as readily as on land, and the pond lies within the roam
        (70) of most of them. All twelve tame crocodile rows are this island's; eleven spawn on land (the ground's
        own check, cmd/campground: env 0, standing on the terrain at 63.4-68.6) and one, npcs 2536, in the pond (env
        1, the floor 54.09), put there by a GM (petition 2087, tools/crocs_staff_20261114.py, an npcwander of 20).
        The row's water word is cleared (keeps by spawn, entity/water.go's keepsToLocked): the eleven keep to the
        land round their own spawns, which are already spread over the island, and 2536 to its pond. Their roam
        (1) and pace (3.2) are kept; a fight still takes one where its enemy is. No spawn moves.
  2819: itempattern 720253, "broken arrowhead" (the elder spirits' drop), wore 2094679524, the arrow's icon (Arrow
        of Love's, bundle of arrowheads'). ITEMICON.ESF has a red-orange knapped flint arrowhead, 154320144 (the
        ESF's own DictID, rendered with ~/.cache/iconsheet by that decimal), which no record wears yet on either
        shard, so the decimal is the ESF's. Live's 720253 is the same record with the same icon: mirrored.

Refused, nothing written, when a row is not as expected (a GM's edit since). Report only by default; --apply in one
transaction, a gm_audit line a row, log petpass_both_1123v_log; --undo exact. npcmove is read at the server's start;
an item's icon at the player's next login.

    python3 -E -s tools/petpass_both_1123v.py --db eqoa.db [--apply | --undo]
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_both_1123v_log"
DETAIL = ("Live petition 5573 / Heirloom petition 2819: Croc Island's tame crocodiles keep to the land by their spawns; "
          "the broken arrowhead's flint icon (tools/petpass_both_1123v.py)")
SOURCE = "petpass-1123v"
CROC = "tame crocodile"
ISLAND = (25300.0, 25650.0, 17150.0, 17500.0)  # x0, x1, z0, z1
ITEM, ITEM_NAME = 720253, "broken arrowhead"
ARROW_ICON, FLINT_ICON = 2094679524, 154320144


def changes(db):
    out, lines, refusals = [], [], []

    r = tr.get(db, "npcmove", {"name": CROC})
    x0, x1, z0, z1 = ISLAND
    n = tr.rows(db, "SELECT count(*) AS c FROM npcs WHERE npc_name = ?", (CROC,))[0]["c"]
    on = tr.rows(db, "SELECT count(*) AS c FROM npcs WHERE npc_name = ? AND world = 0 AND x_coord BETWEEN ? AND ? "
                     "AND z_coord BETWEEN ? AND ?", (CROC, x0, x1, z0, z1))[0]["c"]
    after = {"water": "", "note": "Live petition 5573: keeps by its spawn (Croc Island's on land, its pond's one, "
                                  "npcs 2536, in the pond); was in and out of the water (crocodile)",
             "source": SOURCE}
    if r is None:
        refusals.append(f"no npcmove row for {CROC}")
    elif r["water"] != "both" and any(r[c] != v for c, v in after.items()):
        refusals.append(f"npcmove {CROC} is not as expected: {r}")
    elif n != on:
        refusals.append(f"{n} {CROC} rows, {on} on Croc Island: others elsewhere would change too")
    else:
        out.append(("npcmove", {"name": CROC}, dict(r, **after)))
        lines.append(f"5573: npcmove {CROC} water {r['water']!r} -> '' (by spawn), roam {r['roam']} speed {r['speed']} "
                     f"kept; {n} rows, all on Croc Island")

    it = tr.get(db, "itempattern", {"patternid": ITEM})
    if it is None or it["itemname"] != ITEM_NAME:
        refusals.append(f"itempattern {ITEM} is not the {ITEM_NAME}: {it and it['itemname']}")
    elif it["itemicon"] not in (ARROW_ICON, FLINT_ICON):
        refusals.append(f"itempattern {ITEM}'s icon is {it['itemicon']}, not the arrow's (a GM's edit since?)")
    else:
        out.append(("itempattern", {"patternid": ITEM}, dict(it, itemicon=FLINT_ICON)))
        lines.append(f"2819: itempattern {ITEM} {ITEM_NAME} itemicon {it['itemicon']} -> {FLINT_ICON} (the flint arrowhead)")
    others = tr.rows(db, "SELECT count(*) AS c FROM itempattern WHERE itemname = ? AND patternid != ?",
                     (ITEM_NAME, ITEM))[0]["c"]
    if others:
        lines.append(f"note: {others} other {ITEM_NAME} records, not changed")

    applied = bool(out) and all(tr.get(db, t, k) == a for t, k, a in out)
    lines.insert(0, f"{len(out)} rows")
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("npcmove", "npcs", "itempattern"), "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
