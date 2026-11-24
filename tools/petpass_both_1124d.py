#!/usr/bin/env python3
"""Heirloom petition 2824 (Darkhavictwo, 2026-11-24, Item): "rogue metal gem icon is wrong, its supposed to be the
orange metal icon". Both shards.

  The record is Rogue Metal (Heirloom itempattern 996092, Live 980209), the weaponsmith's rare modifier that lends a
  lingering poison to a blade (tools/seed_tradeskills.py). The seed gave every weapon modifier one stand-in icon,
  1246639590, the grey alloy rod. It now wears 1320989093, the orange metal bar that Gold Bar, Copper Bar, Bronze Bar
  and gold ingot already wear (the decimal copied from those records, rendered from ITEMICON.ESF with
  ~/.cache/iconsheet beside the other metal icons: the silver bar of the ores and bars, the grey rod, the dark lump;
  it is the only orange metal of the 381 icons). Live's record is the same with the same icon: mirrored.

  The other eight weapon modifiers still on the alloy rod are not changed (Ethereal Mists wears the blue bottle
  since tools/petpass_both_1124a.py).

Refused, nothing written, when a row is not as expected (a GM's edit since). Report only by default; --apply in one
transaction, a gm_audit line a row, log petpass_both_1124d_log; --undo exact. An item's icon shows at the player's
next login.

    python3 -E -s tools/petpass_both_1124d.py --db eqoa.db [--apply | --undo]
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_both_1124d_log"
DETAIL = "Heirloom petition 2824: Rogue Metal wears the orange metal bar icon (tools/petpass_both_1124d.py)"
ITEM_NAME = "Rogue Metal"
DESC_MARK = "a weaponsmith works into a blade"
ALLOY_ICON, ORANGE_BAR_ICON = 1246639590, 1320989093
BAR_WEARER = "Copper Bar"


def changes(db):
    out, lines, refusals = [], [], []

    wearers = tr.rows(db, "SELECT count(*) AS c FROM itempattern WHERE itemname = ? AND itemicon = ?",
                      (BAR_WEARER, ORANGE_BAR_ICON))[0]["c"]
    if not wearers:
        refusals.append(f"no {BAR_WEARER} wears {ORANGE_BAR_ICON}: the orange bar's decimal is not confirmed here")

    recs = tr.rows(db, "SELECT patternid FROM itempattern WHERE itemname = ? AND itemdesc LIKE ?",
                   (ITEM_NAME, f"%{DESC_MARK}%"))
    if len(recs) != 1:
        refusals.append(f"{len(recs)} {ITEM_NAME} weapon-modifier records, expected 1")
    else:
        key = {"patternid": recs[0]["patternid"]}
        it = tr.get(db, "itempattern", key)
        if it["itemicon"] not in (ALLOY_ICON, ORANGE_BAR_ICON):
            refusals.append(f"itempattern {key['patternid']}'s icon is {it['itemicon']}, not the alloy's (a GM's edit since?)")
        else:
            out.append(("itempattern", key, dict(it, itemicon=ORANGE_BAR_ICON)))
            lines.append(f"2824: itempattern {key['patternid']} {ITEM_NAME} itemicon {it['itemicon']} -> "
                         f"{ORANGE_BAR_ICON} (the orange metal bar; {wearers} {BAR_WEARER} records wear it)")

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
