#!/usr/bin/env python3
"""Heirloom petitions 2855 and 2856 (Aeris, 2026-11-24).

2855 (Creature or NPC): "the Gal'saris npcs in the green rift are not hostile and some are un-attackable as a good
character. I'm not sure if they should be or not."

The Gal`Saris camp of the Green Rift (world 0, x 22,223-22,687, z 10,647-11,120): dragoons, knights, priestesses,
necromancers, ghouls, a magician, an enchanter, a scout, Ziliq, the necromancers' four Decaying Minions, the magician's
Fire Elementalkin. Dark elf bodies (a players' race: peaceful with no faction rule), and no npcfaction, factionhits
or npcsocial row on any of them, so none attacked anyone on sight (the ghouls' undead body aside) and a kill cost
nothing. The camp is a hunting camp of House Gal`Saris (Prima's Frontiers guide p. 385: the house digging the Combine
ruins in the Greenrift; the guides fight Ziliq's people there, light blue to a 30; quest 100899 kills Ziliq). Its
talkers stay as they are: Devik Gal`Saris and Animator Jen`Tylis (quest givers, 384), Tsekar Kelriss (the diamond
seller, board #1014), Miasma Malignus and the Blasphemous box. The knights (npc_type 128) stay talkers: one takes the
Aurabound Shield (quest 101226 step 7, by name at any knight), and whether the others were fought is jibrish's open
question on board #1004 (held there).

This is board #1004 (jibrish's, "For dab's sessions", sections 1-4), as submitted:
  1. Levels [Allakhazam q542/q639/q657: light blue to a 30]: every Gal`Saris row and Ziliq down 3 (30->27, 29->26,
     28->25), the Fire Elementalkin 108615 27->22. 43 npcs rows by id.
  2. npcfaction, 11 names, each scoped world 0, (22455, 10883), radius 340, House Gal`Saris (keeps out the pets' rows
     elsewhere, the nearest 1,966 off). They then attack on sight, within 21 and in sight, a player at -350 or lower
     with the house who does not con them green; the knights (talkers) and the Fire Elementalkin (npcaggro 0) only
     show the red face.
  3. factionhits, the eight Gal`Saris kinds and Ziliq: House Gal`Saris -10, The Indigo Brotherhood -5, Citizens of
     Neriak -5 (Prima Table 1 penalty 10, Table 2 associates at 50%; petfix582's PRIMA_T1/T2; filmed in 2010 as
     exactly these three lines, #1004's note).
  4. npcsocial, the same nine names: social 1, faction House Gal`Saris, radius 30 (q657: a pull brings more).

2856 (Item): "The ring of Gal'saris quest item is using ore art and not ring art". Ring of Gal'Saris (itempattern
700886 on both shards, Ziliq's quest drop) wore 1577500738, the ore's icon (Iron Ore, Bronze Ore...). It now wears
-1505803670, the gold ring 568 ring records wear (Ring of Garthis, ring of destiny...; rendered from ITEMICON.ESF with
~/.cache/iconsheet). The twelve other records on the ore icon are not touched.

Refused, nothing written, when a row is not as expected. Report only by default; --apply in one transaction, a
gm_audit line a row, log petpass_both_1124y_log; --undo exact. npcs levels reach the world with reload_npcs;
npcfaction, factionhits and npcsocial at the next restart; the icon at the player's next login.

    python3 -E -s tools/petpass_both_1124y.py --db eqoa.db [--apply | --undo]
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_both_1124y_log"
DETAIL = ("Heirloom petitions 2855/2856 (board #1004): the Gal`Saris camp bound to House Gal`Saris, levels, Prima's "
          "kill hits, social help; Ring of Gal'Saris a ring icon (tools/petpass_both_1124y.py)")
SOURCE = "board-1004 prima-2003 petpass-1124y"
HOUSE, INDIGO, NERIAK = "House Gal`Saris", "The Indigo Brotherhood", "Citizens of Neriak"
SCOPE = {"world": 0, "x": 22455.0, "z": 10883.0, "radius": 340.0}
NOTE = "the Green Rift's Gal`Saris camp (board #1004, Prima p. 385); Heirloom petition 2855"

KINDS = ("Gal`Saris dragoon", "Gal`Saris enchanter", "Gal`Saris ghoul", "Gal`Saris knight", "Gal`Saris magician",
         "Gal`Saris necromancer", "Gal`Saris priestess", "Gal`Saris scout", "Ziliq")
BOUND = KINDS + ("Decaying Minion", "Fire Elementalkin")

# Section 1: id -> (now, then)
LEVELS = {}
for ids, frm, to in (
        ((108585, 108593, 108598, 108600, 108607, 108610, 108574, 108584, 108592, 108599, 108602, 108573, 108582,
          108587, 108616, 108617, 108576, 108577, 108579, 108589, 108603, 108606, 108620, 108618, 108619), 30, 27),
        ((108611, 108612, 108570, 108583, 108590, 108594, 108597, 108605, 108608, 108609, 108601, 122601), 29, 26),
        ((108572, 108591, 108595, 108581, 108604), 28, 25),
        ((108615,), 27, 22)):
    for i in ids:
        LEVELS[i] = (frm, to)

# Prima Table 1 / Table 2 for the house (as petfix582's PRIMA_T1 / PRIMA_T2)
PRIMA_T1 = {HOUSE: 10}
PRIMA_T2 = {HOUSE: [[INDIGO, 50], [NERIAK, 50]]}

RING, ORE_ICON, RING_ICON, RING_WEARER = 700886, 1577500738, -1505803670, "Ring of Garthis"


def prima_hits(f):
    """petfix582's prima_hits, unchanged."""
    p = PRIMA_T1[f]
    h = {f: -p}
    for a, r in PRIMA_T2.get(f, []):
        s = p * r / 100
        if abs(s) < 1:
            continue
        h[a] = h.get(a, 0) - int(s)
    return h


def near(r):
    return r["world"] == 0 and (r["x_coord"] - SCOPE["x"]) ** 2 + (r["z_coord"] - SCOPE["z"]) ** 2 < SCOPE["radius"] ** 2


def changes(db):
    out, lines, refusals = [], [], []
    want = prima_hits(HOUSE)
    if want != {HOUSE: -10, INDIGO: -5, NERIAK: -5}:
        refusals.append(f"Prima's hits computed as {want}")
    for f in want:
        if not tr.rows(db, "SELECT 1 FROM factions WHERE name = ?", (f,)):
            refusals.append(f"no faction {f!r} in factions")

    # 1. levels
    for i, (frm, to) in sorted(LEVELS.items()):
        r = tr.get(db, "npcs", {"id": i})
        if r is None or not near(r) or r["npc_name"].lower() not in {b.lower() for b in BOUND}:
            refusals.append(f"npcs {i}: not a camp row as expected: {None if r is None else r['npc_name']}")
            continue
        if r["npc_level"] not in (frm, to):
            refusals.append(f"npcs {i} {r['npc_name']}: level {r['npc_level']}, expected {frm}")
            continue
        out.append(("npcs", {"id": i}, {"npc_level": to}))
        lines.append(f"npcs {i} {r['npc_name']}: level {r['npc_level']} -> {to}")
    # every camp row of the bound names: the type stays (the knights' 128 held), the count as #1004 saw it
    camp = [r for n in BOUND for r in tr.rows(db, "SELECT * FROM npcs WHERE lower(npc_name) = ? AND world = 0",
                                              (n.lower(),)) if near(r)]
    if len(camp) != 47:
        refusals.append(f"{len(camp)} camp rows of the bound names, expected 47 (41 + Ziliq + 4 minions + the kin)")
    # nothing of these names elsewhere inside the scope's reach is bound by mistake: the scope holds the camp only
    for n in ("Decaying Minion", "Fire Elementalkin", "Lesser Animation"):
        far = [r for r in tr.rows(db, "SELECT * FROM npcs WHERE lower(npc_name) = ?", (n.lower(),)) if not near(r)]
        lines.append(f"note: {len(far)} {n} rows outside the scope keep their own rules")

    # 2. npcfaction, 3. factionhits, 4. npcsocial
    for name in BOUND:
        low = name.lower()
        k = dict({"name": low}, **SCOPE)
        nf = tr.rows(db, "SELECT * FROM npcfaction WHERE lower(trim(name)) = ?", (low,))
        inside = [r for r in nf if r["world"] in (0, -1) and (r["world"] == -1 or r["radius"] == 0 or
                  (r["x"] - SCOPE["x"]) ** 2 + (r["z"] - SCOPE["z"]) ** 2 < (r["radius"] + SCOPE["radius"]) ** 2)]
        odd = [r for r in inside if not (r["faction"] == HOUSE and r["source"] == SOURCE)]
        if odd:
            refusals.append(f"npcfaction rows for {name} reach the camp: {odd}")
        out.append(("npcfaction", k, dict(k, faction=HOUSE, note=NOTE, source=SOURCE)))
        lines.append(f"npcfaction {low} (world 0, radius 340 of the camp) -> {HOUSE}")
    for name in KINDS:
        low = name.lower()
        if tr.rows(db, "SELECT 1 FROM factionhitareas WHERE lower(trim(npc_name)) = ?", (low,)):
            refusals.append(f"factionhitareas has rows for {name}")
        have = {h["faction"]: h for h in tr.rows(db, "SELECT * FROM factionhits WHERE lower(trim(npc_name)) = ?",
                                                 (low,))}
        odd = [h for h in have.values() if h["source"] != SOURCE]
        if odd:
            refusals.append(f"factionhits {name}: rows already: {odd}")
        for f, d in want.items():
            out.append(("factionhits", {"npc_name": low, "faction": f},
                        {"npc_name": low, "faction": f, "delta": d, "source": SOURCE}))
            lines.append(f"factionhits {low} / {f}: {d:+d}")
        soc = tr.rows(db, "SELECT * FROM npcsocial WHERE lower(trim(npc_name)) = ?", (low,))
        if [s for s in soc if s["source"] != SOURCE]:
            refusals.append(f"npcsocial {name}: a row already: {soc}")
        out.append(("npcsocial", {"npc_name": name},
                    {"npc_name": name, "social": 1, "faction": HOUSE, "radius": 30.0, "note": NOTE, "source": SOURCE}))
        lines.append(f"npcsocial {name}: helps {HOUSE} within 30")

    # 2856: the ring's icon
    wearers = tr.rows(db, "SELECT count(*) AS c FROM itempattern WHERE itemicon = ? AND itemname = ?",
                      (RING_ICON, RING_WEARER))[0]["c"]
    if not wearers:
        refusals.append(f"no {RING_WEARER} wears {RING_ICON}: the ring icon's decimal is not confirmed here")
    it = tr.get(db, "itempattern", {"patternid": RING})
    if it is None or it["itemname"] != "Ring of Gal'Saris" or it["itemicon"] not in (ORE_ICON, RING_ICON):
        refusals.append(f"itempattern {RING} not as expected: "
                        f"{None if it is None else (it['itemname'], it['itemicon'])}")
    else:
        out.append(("itempattern", {"patternid": RING}, {"itemicon": RING_ICON}))
        lines.append(f"2856: itempattern {RING} Ring of Gal'Saris itemicon {it['itemicon']} -> {RING_ICON} (gold ring)")

    if refusals:
        out = []
    applied = bool(out) and all(
        tr.get(db, t, k) is not None and all(tr.get(db, t, k).get(c) == v for c, v in a.items()) for t, k, a in out)
    lines.insert(0, f"{len(out)} rows")
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL,
        "needs": ("npcs", "npcfaction", "factionhits", "factionhitareas", "factions", "npcsocial", "itempattern"),
        "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
