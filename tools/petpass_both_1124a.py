#!/usr/bin/env python3
"""Heirloom petition 2820 (Darkhavictwo, 2026-11-24, Item): "ethril mist, the gem, currently looks like an alloy, it
should look like the blue bottle icon". Both shards.

  The record is Ethereal Mists (Heirloom itempattern 996094, Live 980211), the weaponsmith's rare modifier that lends
  a lingering arcane to a blade (tools/seed_tradeskills.py, EQOA-Data's gems.json "ethereal non dmg proc"). The seed
  gave every weapon modifier one stand-in icon, 1246639590, the alloy bar (Magnesium Alloy's, Titan Alloy's). It now
  wears 209763529, the blue bottle that 29 potions already wear (Ethereal Potion, Aether Potion, Freezing Potion:
  the decimal copied from those records, rendered from ITEMICON.ESF with ~/.cache/iconsheet: a corked flask of blue
  liquid). Live's record is the same with the same icon: mirrored.

  No other "mist" gem exists. The other nine weapon modifiers keep the alloy bar: three are alloys or metal (Magnesium
  Alloy, Malign Alloy, Rogue Metal), and for the rest (the crystals, spores, stone and blood) neither the sources nor a
  sibling says which icon is right.

Refused, nothing written, when a row is not as expected (a GM's edit since). Report only by default; --apply in one
transaction, a gm_audit line a row, log petpass_both_1124a_log; --undo exact. An item's icon shows at the player's
next login.

    python3 -E -s tools/petpass_both_1124a.py --db eqoa.db [--apply | --undo]
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_both_1124a_log"
DETAIL = "Heirloom petition 2820: Ethereal Mists wears the blue bottle icon (tools/petpass_both_1124a.py)"
ITEM_NAME = "Ethereal Mists"
DESC_MARK = "a weaponsmith works into a blade"
ALLOY_ICON, BLUE_BOTTLE_ICON = 1246639590, 209763529
BOTTLE_WEARER = "Ethereal Potion"


def changes(db):
    out, lines, refusals = [], [], []

    wearers = tr.rows(db, "SELECT count(*) AS c FROM itempattern WHERE itemname = ? AND itemicon = ?",
                      (BOTTLE_WEARER, BLUE_BOTTLE_ICON))[0]["c"]
    if not wearers:
        refusals.append(f"no {BOTTLE_WEARER} wears {BLUE_BOTTLE_ICON}: the blue bottle's decimal is not confirmed here")

    recs = tr.rows(db, "SELECT patternid FROM itempattern WHERE itemname = ? AND itemdesc LIKE ?",
                   (ITEM_NAME, f"%{DESC_MARK}%"))
    if len(recs) != 1:
        refusals.append(f"{len(recs)} {ITEM_NAME} weapon-modifier records, expected 1")
    else:
        key = {"patternid": recs[0]["patternid"]}
        it = tr.get(db, "itempattern", key)
        if it["itemicon"] not in (ALLOY_ICON, BLUE_BOTTLE_ICON):
            refusals.append(f"itempattern {key['patternid']}'s icon is {it['itemicon']}, not the alloy's (a GM's edit since?)")
        else:
            out.append(("itempattern", key, dict(it, itemicon=BLUE_BOTTLE_ICON)))
            lines.append(f"2820: itempattern {key['patternid']} {ITEM_NAME} itemicon {it['itemicon']} -> "
                         f"{BLUE_BOTTLE_ICON} (the blue bottle; {wearers} {BOTTLE_WEARER} records wear it)")

    others = tr.rows(db, "SELECT count(*) AS c FROM itempattern WHERE itemdesc LIKE ? AND itemname != ? AND itemicon = ?",
                     (f"%{DESC_MARK}%", ITEM_NAME, ALLOY_ICON))[0]["c"]
    lines.append(f"note: {others} other weapon modifiers keep the alloy icon, not changed")

    if refusals:
        out = []
    applied = bool(out) and all(tr.get(db, t, k) == a for t, k, a in out)
    lines.insert(0, f"{len(out)} rows")
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("itempattern",), "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
