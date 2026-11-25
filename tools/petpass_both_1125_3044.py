#!/usr/bin/env python3
"""Heirloom petition 3044 (Deathknell, 2026-11-25, Item, loot or merchant): "Warrior 60 epic which currently gives 2
of the Vanquisher's Blade (ID 718024) is supposed to give 1 Vanquisher's Blade and 1 World Splitter (ID 5371). The
option for the two handed sword should be Sword of the Challenger (id 5373)."

The evil warrior's level 60 reward (quest 101127 "The Warrior's Epic Reward", EQOA-Data 60/evil-tank/reward-warrior)
step 2 offered "The Ascended Warrior's Epic Blades" (two of 718024's Ascended record) and "The Ascended Warrior's Epic
Greatsword" (718023's). The era agrees with the player: the forum armory's "best tank gear" thread (t283) lists
Vanquisher's Blade and World Splitter as two different "SECONDARY (Warrior only, epic)" one-handers, 801 each; EQOA-Data
has Sword of the Challenger, 928 two-handed.

This tool writes the records the quest step then names (the step itself goes by the GM panel, one row):

  - an Ascended record of World Splitter (5371) and of Sword of the Challenger (5373), by the server's rule
    (database/ascended.go AscendedCopyTx, the SQL of tools/ascend_boss_loot.py): "Ascended <name>", figures a tenth
    more, its use effects copied, listed in itemascended (the server's start keeps each its base again);
  - on a shard where 718024 is still "Warrior's Epic Blade" (Heirloom), its name "Vanquisher's Blade" and its Ascended
    record's "Ascended Vanquisher's Blade", as Live has them (GM Garrak, 2026-11-25) and the player calls it.

Every record is found by id and checked by name; nothing of the other shard's ids is assumed. Report only by default;
--apply in one transaction, a gm_audit line a row, log petpass_both_1125_3044_log; --undo exact. Item records at the
next login; the quest's options at the next restart.

    python3 -E -s tools/petpass_both_1125_3044.py --db eqoa.db [--apply | --undo]
"""
import datetime
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import ascend_boss_loot as abl  # noqa: E402  the server's Ascended item rule
import tix4_rows as tr  # noqa: E402

LOG = "petpass_both_1125_3044_log"
DETAIL = ("Heirloom petition 3044 (Deathknell): the warrior's level 60 epic gives Vanquisher's Blade + World Splitter, "
          "or Sword of the Challenger; their Ascended records (tools/petpass_both_1125_3044.py)")
SOURCE = "tools/petpass_both_1125_3044.py (petition 3044)"

BLADE, OLD_BLADE_NAME, BLADE_NAME = 718024, "Warrior's Epic Blade", "Vanquisher's Blade"
BASES = {5371: "World Splitter", 5373: "Sword of the Challenger"}


def ascended_row(db, base, nid):
    """base's Ascended record as the server's SQL makes it, with id nid."""
    cols = [r[1] for r in db.execute("PRAGMA table_info(itempattern)")]
    _, froms, names = abl.copy_columns(cols)
    vals = db.execute(f"SELECT {froms} FROM itempattern WHERE patternid = ?", (base,)).fetchone()
    return dict({"patternid": nid}, **dict(zip(names, vals)))


def changes(db):
    out, lines, refusals = [], [], []
    asc = {b: c for c, b in db.execute("SELECT patternid, base FROM itemascended")}
    imported = {r[0] for r in db.execute("SELECT patternid FROM itemimported")} if tr.table(db, "itemimported") else set()
    # the blade: 718024 and its Ascended record
    b = tr.get(db, "itempattern", {"patternid": BLADE})
    if b is None or b["itemname"] not in (OLD_BLADE_NAME, BLADE_NAME) or b["equipslot"] != 14 or b["levelreq"] != 60:
        refusals.append(f"{BLADE}: not the warrior's level 60 one-hander: {b and (b['itemname'], b['equipslot'])}")
    elif BLADE not in asc:
        refusals.append(f"{BLADE} has no Ascended record")
    else:
        bc = tr.get(db, "itempattern", {"patternid": asc[BLADE]})
        if b["itemname"] != BLADE_NAME:
            out.append(("itempattern", {"patternid": BLADE}, {"itemname": BLADE_NAME}))
            lines.append(f"itempattern {BLADE}: {b['itemname']!r} -> {BLADE_NAME!r}")
        if bc["itemname"] != abl.ascended_name(BLADE_NAME):
            out.append(("itempattern", {"patternid": asc[BLADE]}, {"itemname": abl.ascended_name(BLADE_NAME)}))
            lines.append(f"itempattern {asc[BLADE]}: {bc['itemname']!r} -> {abl.ascended_name(BLADE_NAME)!r}")
        lines.append(f"blade: {BLADE} {BLADE_NAME}, Ascended {asc[BLADE]}")
    # the two Ascended records
    nid = db.execute("SELECT COALESCE(MAX(patternid), ?) + 1 FROM itempattern WHERE patternid BETWEEN ? AND ?",
                     (abl.FIRST_ID - 1, abl.FIRST_ID, abl.LAST_ID)).fetchone()[0]
    made_log = tr.table(db, LOG)
    for base, name in BASES.items():
        r = tr.get(db, "itempattern", {"patternid": base})
        if r is None or r["itemname"] != name or r["levelreq"] != 60 or base in imported or not abl.is_gear(r):
            refusals.append(f"{base}: not the level 60 gear {name!r}: {r and (r['itemname'], r['levelreq'])}")
            continue
        if base in asc and not made_log:
            refusals.append(f"{base} {name} has an Ascended record already ({asc[base]}): point at it instead")
            continue
        if base in asc:  # this tool's, written
            c = asc[base]
        else:
            c, nid = nid, nid + 1
        if c > abl.LAST_ID:
            refusals.append("no Ascended id left")
            continue
        clash = tr.rows(db, "SELECT patternid FROM itempattern WHERE itemname = ? AND patternid != ?",
                        (abl.ascended_name(name), c))
        if clash:
            refusals.append(f"{abl.ascended_name(name)!r} is a record already: {clash}")
            continue
        out.append(("itempattern", {"patternid": c}, ascended_row(db, base, c)))
        for e in tr.rows(db, "SELECT * FROM itemeffect WHERE patternid = ?", (base,)) if tr.table(db, "itemeffect") else []:
            e = dict(e, patternid=c)
            for col in ("line", "target_line"):
                if col in e:
                    e[col] = abl.scaled_line(e[col])
            out.append(("itemeffect", {"patternid": c}, e))
        made = tr.get(db, "itemascended", {"patternid": c})
        out.append(("itemascended", {"patternid": c}, made or {
            "patternid": c, "base": base, "source": SOURCE,
            "made": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")}))
        row = ascended_row(db, base, c)
        lines.append(f"Ascended {name}: {c} (base {base}; damage {row['weapondamage']}, slot {row['equipslot']}, "
                     f"classes {row['classuse']})")
    if refusals:
        out = []
    applied = bool(out) and made_log and all(
        tr.get(db, t, k) is not None and all(tr.get(db, t, k).get(col) == v for col, v in a.items()) for t, k, a in out)
    lines.insert(0, f"{len(out)} rows")
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("itempattern", "itemascended"),
        "changes": changes, "basis": "shared", "basis_ref": "petition 3044; forum armory t283"}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, argv=sys.argv[1:], doc=__doc__))
