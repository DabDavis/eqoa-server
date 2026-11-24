#!/usr/bin/env python3
"""Live petition 5578 (Clore, a level 60 elf ranger, 2026-11-24, at Merchant Glacia in Tethelin): "lvl 55-60 abils
missing from this npc for ranger".

What a ranger of 50-60 buys, and where, on Live:
  - Merchant Glacia (Tethelin, 104664) and Merchant Laurel (Surefall Glade, 105672) are the ranger's two scroll
    merchants. Both sell the class scrolls to 49, as the live game's class vendors did; Laurel also sells Bladecoat,
    Poison Arrow, Predacious Instinct (55), Shimmering Blades, Panther Form and Panther's Alacrity (60), the era's
    stock on both shards (Glacia's list ends at 49 on Heirloom too).
  - Drops by design on Live (tools/live_spell_sweep_20261120.py): Entanglement 46 (Uwu's spirits and phantoms),
    Greater Camouflage 51 (Uwu's Himlar and the Ancient One; the t303 51-54 creatures), Forage Tuber 54 (t303 51-54),
    Fairy's Grace 57 (t303 57: Xrag's), Lightning Arrow 60 (Plane of Disease), Arcanic, Divine and Warmonger's
    Assault 60 (Zaoran Plateau).
  - Lycanthropy (50) had no source on Live: its scroll record (716201, made for Heirloom's Spell Archivists on
    2026-11-08, petition 1273) was used only by Archivist rows, so the 11-23 shard sync left it Heirloom's, and the
    11-20 sweep skipped it as a record Live lacks. No loot table or quest gives it on either shard.

So, as the sweep stocks an unsold 50+ scroll on Live: Heirloom's record copied under its own id, and the scroll put
last in the stock of the ranger's two guild merchants (Glacia 12 items, Laurel 19; the client's list holds 40).
Live only. Report only by default; --apply in one transaction, a gm_audit line a row, log petpass_live_1124p_log;
--undo exact. A merchant's stock is read at each window; the record is read where it is used.

    python3 -E -s tools/petpass_live_1124p.py --db eqoa.db [--apply | --undo]
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_live_1124p_log"
DETAIL = "Live petition 5578: the ranger's Lycanthropy scroll at the ranger merchants (tools/petpass_live_1124p.py)"
SCROLL = 716201
SPELL = (700667, "Lycanthropy", 50)
LIST_MOST = 40
VENDORS = {104664: "Merchant Glacia", 105672: "Merchant Laurel"}
# Heirloom's record, as it stands (dab 2026-11-08, ~/charmigrate/spell_archivist_20261108.py)
RECORD = {"patternid": SCROLL, "itemcost": 2500, "unk1": 0, "itemicon": -607649328, "unk2": 0, "equipslot": -1,
          "unk3": 0, "trade": 0, "rent": 1, "unk4": 0, "attacktype": 0, "weapondamage": 0, "unk5": 0, "levelreq": 50,
          "maxstack": 1, "maxhp": 0, "duration": 0, "classuse": 2, "raceuse": 1023, "procanim": 2, "lore": 0, "unk6": 0,
          "craft": 0, "itemname": "Lycanthropy",
          "itemdesc": "Self, Increase STR/DEX by 300, Increase Hitpoints by 5000, 15min", "model": 0,
          "color": 4294967295, "str": None, "sta": None, "agi": None, "wis": None, "dex": None, "cha": None,
          "intelligence": None, "HPMAX": None, "POWMAX": None, "PoT": None, "HoT": None, "AC": None, "PR": None,
          "DR": None, "FR": None, "CR": None, "LR": None, "AR": None, "weaponproc": 0, "fish": 0}


def changes(db):
    out, lines, refusals = [], [], []
    if tr.rows(db, "SELECT 1 FROM npcs WHERE npc_name = 'Spell Archivist' LIMIT 1"):
        refusals.append("this database has Spell Archivists (Heirloom): Live only")
    sp = tr.rows(db, "SELECT spellid, spellname, abilitylvl FROM spellpattern WHERE spellid = ?", (SPELL[0],))
    if not sp or (sp[0]["spellname"], sp[0]["abilitylvl"]) != SPELL[1:]:
        refusals.append(f"spell {SPELL[0]} is not {SPELL[1]} of level {SPELL[2]}: {sp}")
    if not tr.rows(db, "SELECT 1 FROM classspells WHERE class = 1 AND spellid = ?", (SPELL[0],)):
        refusals.append("Lycanthropy is not a ranger's spell (classspells) here")
    rec = tr.get(db, "itempattern", {"patternid": SCROLL})
    if rec is not None and rec.get("itemname") != RECORD["itemname"]:
        refusals.append(f"pattern {SCROLL} is another record here: {rec.get('itemname')!r}")
    other = tr.rows(db, "SELECT patternid FROM itempattern WHERE itemname = 'Lycanthropy' AND patternid != ?", (SCROLL,))
    if other:
        refusals.append(f"another Lycanthropy record here: {other}")
    out.append(("itempattern", {"patternid": SCROLL}, dict(RECORD)))
    lines.append(f"itempattern {SCROLL} Lycanthropy (50, ranger, 2,500): copied from Heirloom's")
    for npcid, name in VENDORS.items():
        n = tr.rows(db, "SELECT npc_name FROM npcs WHERE id = ?", (npcid,))
        if not n or n[0]["npc_name"] != name:
            refusals.append(f"npc {npcid} is not {name}: {n}")
            continue
        if not tr.rows(db, "SELECT 1 FROM npcitem WHERE npcid = ? AND patternid = 21670", (npcid,)):
            refusals.append(f"{name} does not sell Predatorial Instinct (49): not the ranger's merchant")
        st = tr.rows(db, "SELECT count(*) AS c, COALESCE(MAX(listnumber), -1) AS m FROM npcitem WHERE npcid = ?",
                     (npcid,))[0]
        if st["c"] + 1 > LIST_MOST:
            refusals.append(f"{name} would list {st['c'] + 1} (the client's list holds {LIST_MOST})")
        key = {"npcid": npcid, "patternid": SCROLL}
        have = tr.get(db, "npcitem", key)
        listno = have["listnumber"] if have else st["m"] + 1
        out.append(("npcitem", key, dict(key, stackleft=1, remainHP=0, remaincharge=0, equiploc=-1, location=1,
                                         listnumber=listno)))
        lines.append(f"{name} ({npcid}): Lycanthropy at list {listno} ({st['c']} items before)")
    applied = all(tr.get(db, t, k) is not None and all(tr.get(db, t, k).get(c) == v for c, v in a.items())
                  for t, k, a in out)
    lines.insert(0, f"{len(out)} rows")
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("itempattern", "npcitem", "npcs", "spellpattern", "classspells"),
        "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
