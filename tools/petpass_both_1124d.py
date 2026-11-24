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

Step 2 (--step 2, the coordinator after 2825: "a stud looks like a stud"): the seed's three other tailor studs, Marr
Blessed Studs, Rallosian Studs and Takish Ivory Studs (Heirloom 998464/998466/998467, Live 982582/982584/982585),
wear the same studs icon -1836445075. Its own log, petpass_both_1124d_studs_log, so its --undo stands alone.

    python3 -E -s tools/petpass_both_1124d.py --step 2 --db eqoa.db [--apply | --undo]

Step 3 (Heirloom petition 2831, the same player: "clarification on the rogue metal, it is the copper coins looking
icon, sorry"): Rogue Metal (996092, Live 980209) wears 2086103783, the stack of copper coins, the copper sibling of
the silver coins (-2114812357, Defaced Coin's) and the gold coins (1170433018). No record wears it on either shard, so
the decimal is ITEMICON.ESF's own: its dumped surface 2086103783.png, which ~/.cache/iconsheet finds and renders by
that decimal. Its own log, petpass_both_1124d_coins_log. Step 1's --undo leaves Rogue Metal alone after this (its row
no longer reads as step 1 wrote it); undo step 3 first to take Rogue Metal back to the orange bar.

    python3 -E -s tools/petpass_both_1124d.py --step 3 --db eqoa.db [--apply | --undo]
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

LOG2 = "petpass_both_1124d_studs_log"
DETAIL2 = ("After Heirloom petition 2825: the seed's other tailor studs wear the studs icon "
           "(tools/petpass_both_1124d.py --step 2)")
STUDS_ICON, STUDS_WEARER = -1836445075, "Heartsblood Alloy Studs"
STUDS = ("Marr Blessed Studs", "Rallosian Studs", "Takish Ivory Studs")
STUDS_MARK = "a tailor works into cloth or leather"


def changes2(db):
    out, lines, refusals = [], [], []

    wearers = tr.rows(db, "SELECT count(*) AS c FROM itempattern WHERE itemname = ? AND itemicon = ?",
                      (STUDS_WEARER, STUDS_ICON))[0]["c"]
    if not wearers:
        refusals.append(f"no {STUDS_WEARER} wears {STUDS_ICON}: the studs decimal is not confirmed here")

    for name in STUDS:
        recs = tr.rows(db, "SELECT patternid FROM itempattern WHERE itemname = ? AND itemdesc LIKE ?",
                       (name, f"%{STUDS_MARK}%"))
        if len(recs) != 1:
            refusals.append(f"{len(recs)} {name} modifier records, expected 1")
            continue
        key = {"patternid": recs[0]["patternid"]}
        it = tr.get(db, "itempattern", key)
        if it["itemicon"] not in (ALLOY_ICON, STUDS_ICON):
            refusals.append(f"itempattern {key['patternid']}'s icon is {it['itemicon']}, not the alloy's "
                            "(a GM's edit since?)")
            continue
        out.append(("itempattern", key, dict(it, itemicon=STUDS_ICON)))
        lines.append(f"studs: itempattern {key['patternid']} {name} itemicon {it['itemicon']} -> {STUDS_ICON} "
                     f"(the studs; {wearers} {STUDS_WEARER} records wear it)")

    if refusals:
        out = []
    applied = bool(out) and all(tr.get(db, t, k) == a for t, k, a in out)
    lines.insert(0, f"{len(out)} rows")
    return out, lines, refusals, applied


SPEC2 = {"log": LOG2, "detail": DETAIL2, "needs": ("itempattern",), "changes": changes2}

LOG3 = "petpass_both_1124d_coins_log"
DETAIL3 = "Heirloom petition 2831: Rogue Metal wears the copper coins icon (tools/petpass_both_1124d.py --step 3)"
ORANGE_BAR_ICON, COPPER_COINS_ICON = 1320989093, 2086103783


def changes3(db):
    out, lines, refusals = [], [], []
    name, mark = FIXES[0][1], FIXES[0][2]
    recs = tr.rows(db, "SELECT patternid FROM itempattern WHERE itemname = ? AND itemdesc LIKE ?", (name, f"%{mark}%"))
    if len(recs) != 1:
        refusals.append(f"2831: {len(recs)} {name} modifier records, expected 1")
    else:
        key = {"patternid": recs[0]["patternid"]}
        it = tr.get(db, "itempattern", key)
        if it["itemicon"] not in (ORANGE_BAR_ICON, ALLOY_ICON, COPPER_COINS_ICON):
            refusals.append(f"2831: itempattern {key['patternid']}'s icon is {it['itemicon']}, not the orange bar's "
                            "or the alloy's (a GM's edit since?)")
        else:
            out.append(("itempattern", key, dict(it, itemicon=COPPER_COINS_ICON)))
            lines.append(f"2831: itempattern {key['patternid']} {name} itemicon {it['itemicon']} -> "
                         f"{COPPER_COINS_ICON} (the copper coins, ITEMICON.ESF's surface; no record wore it)")
    if refusals:
        out = []
    applied = bool(out) and all(tr.get(db, t, k) == a for t, k, a in out)
    lines.insert(0, f"{len(out)} rows")
    return out, lines, refusals, applied


SPEC3 = {"log": LOG3, "detail": DETAIL3, "needs": ("itempattern",), "changes": changes3}

if __name__ == "__main__":
    argv = sys.argv[1:]
    spec = SPEC
    if "--step" in argv:
        i = argv.index("--step")
        step = argv[i + 1] if i + 1 < len(argv) else ""
        if step not in ("1", "2", "3"):
            sys.exit("--step takes 1, 2 or 3")
        spec = {"1": SPEC, "2": SPEC2, "3": SPEC3}[step]
        del argv[i:i + 2]
    sys.exit(tr.main(spec, argv=argv, doc=__doc__))
