#!/usr/bin/env python3
"""Heirloom petitions 2824 and 2825 (Darkhavictwo, 2026-11-24, Item), both shards:
  2824 "rogue metal gem icon is wrong, its supposed to be the orange metal icon"
  2825 "titan allow studs icon is wrong, its supposed to be the studs icon. it is currently alloy icon"

  Rogue Metal (Heirloom itempattern 996092, Live 980209) is the weaponsmith's rare modifier that lends a lingering
  poison to a blade; Titan Alloy Studs (Heirloom 998462, Live 982580) the tailor's modifier that lends vitality. Both
  are tools/seed_tradeskills.py records, which gave the modifiers one stand-in icon, 1246639590, the grey alloy rod.

  - Rogue Metal now wears 1320989093, the orange metal bar that Gold Bar, Copper Bar, Bronze Bar and gold ingot
    already wear (rendered from ITEMICON.ESF with ~/.cache/iconsheet beside the other metal icons: the silver bar of
    the ores and bars, the grey rod, the dark lump; it is the only orange metal of the 381 icons).
  - Titan Alloy Studs now wears -1836445075, the heap of studs that Heartsblood Alloy Studs, Marr's Blessed Studs and
    nine master craftsman's studs already wear (rendered beside the earring studs, which are jewellery).

  Each decimal is copied from those records and the tool refuses unless a wearer still has it. Live's records are the
  same with the same icon: mirrored. The other seed modifiers still on the alloy rod are not changed (Ethereal Mists
  wears the blue bottle since tools/petpass_both_1124a.py).

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
DETAIL = ("Heirloom petitions 2824/2825: Rogue Metal wears the orange metal bar icon, Titan Alloy Studs the studs "
          "icon (tools/petpass_both_1124d.py)")
ALLOY_ICON = 1246639590
# (petition, item name, description mark, new icon, what it is, a record that already wears it)
FIXES = [
    (2824, "Rogue Metal", "a weaponsmith works into a blade", 1320989093, "the orange metal bar", "Copper Bar"),
    (2825, "Titan Alloy Studs", "a tailor works into cloth or leather", -1836445075, "the studs",
     "Heartsblood Alloy Studs"),
]


def changes(db):
    out, lines, refusals = [], [], []

    for pet, name, mark, icon, what, wearer in FIXES:
        wearers = tr.rows(db, "SELECT count(*) AS c FROM itempattern WHERE itemname = ? AND itemicon = ?",
                          (wearer, icon))[0]["c"]
        if not wearers:
            refusals.append(f"{pet}: no {wearer} wears {icon}: {what} decimal is not confirmed here")
        recs = tr.rows(db, "SELECT patternid FROM itempattern WHERE itemname = ? AND itemdesc LIKE ?",
                       (name, f"%{mark}%"))
        if len(recs) != 1:
            refusals.append(f"{pet}: {len(recs)} {name} modifier records, expected 1")
            continue
        key = {"patternid": recs[0]["patternid"]}
        it = tr.get(db, "itempattern", key)
        if it["itemicon"] not in (ALLOY_ICON, icon):
            refusals.append(f"{pet}: itempattern {key['patternid']}'s icon is {it['itemicon']}, not the alloy's "
                            "(a GM's edit since?)")
            continue
        out.append(("itempattern", key, dict(it, itemicon=icon)))
        lines.append(f"{pet}: itempattern {key['patternid']} {name} itemicon {it['itemicon']} -> {icon} "
                     f"({what}; {wearers} {wearer} records wear it)")

    others = tr.rows(db, "SELECT count(*) AS c FROM itempattern WHERE itemicon = ? AND itemname NOT IN (?, ?)",
                     (ALLOY_ICON, FIXES[0][1], FIXES[1][1]))[0]["c"]
    lines.append(f"note: {others} other records keep the alloy icon, not changed")

    if refusals:
        out = []
    applied = bool(out) and all(tr.get(db, t, k) == a for t, k, a in out)
    lines.insert(0, f"{len(out)} rows")
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("itempattern",), "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
