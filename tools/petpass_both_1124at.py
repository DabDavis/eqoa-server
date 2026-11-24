#!/usr/bin/env python3
"""Heirloom petition 2888 (Waldorf, 2026-11-24, Spell or ability): "magicians are missing the store bought level 55
spell to summon badi. this should be the last pet store bought spell right now its the summon djinn spell".

The spell is there: Summon Badi (60) (900742, ability level 55, power 1943, a level 60 Badi, petstats 900742), and since
Live petition 5305 (petfix551, 2026-11-19) its scroll (716955, level 55) is sold by the six class vendors that sell
Summon Djinn (Thrum in Freeport, Keeper Inda, Renferl, Radcliff, Galsen, Blumeria). But it was still filed as a
mastery's: classspells (10, 900742) "cm (eqoaemu)", so the Spell Archivist's Grant basic spells skipped it, and its
book line read "CM: Summons a Badi to aid you."; and the Spell Archivists (Heirloom's level 50+ vendors, 24 of them,
whose stock predates the scroll) did not sell it, the shop a level 60 buys at.

The era: Prima's eGuide (Dec 2004), Magician Abilities p. 413 (PDF 414) prints Summon Badi at 49 (power 1281), 55
(power 1943) and 60 (1538); its Magician Masteries table p. 44 (PDF 45) has Geomancy 1-5 end at Summon Badi Lv49 and
Greater Geomancy (level 60) needing "Summon Badi Lv55" to give Lv60, so no mastery gives the level 55 Badi: it is the
level's own, bought. questmasters id534 says the same (Greater Geomancy 1: "Req. Abilities: Summon Badi (Lvl 55)",
removes it). The Djinn's L55 is a mastery's (Aeromancy 4, needing Summon Djinn L49), so the Badi at 55 is the last
store-bought pet, as the player says.

So: classspells (10, 900742) source "vendor (...)" (database.GotBy: "" -- a class grant gives it), its description
without "CM: ", and on a shard with Spell Archivists the scroll in each one's shop beside the other magician L55 scroll
(Summon: Strawberry Lemonade's row cloned, patternid 716955). Summon Djinn (701036, L49) is left as it is: EQOA-Data and
questmasters id469 call it CM. Greater Geomancy (cmcatalog 6016) still adds 900742 (a restart's data; not changed).

Report only by default; --apply in one transaction, a gm_audit line a row, log petpass_both_1124at_log; --undo exact.
Spell rows at the next login (the book's line); merchants' wares at each opening; the archivist's grant reads the
merchants' scrolls and classspells again within 10 minutes (archiveTTL).

    python3 -E -s tools/petpass_both_1124at.py --db eqoa.db [--apply | --undo]
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_both_1124at_log"
DETAIL = ("Heirloom petition 2888: Summon Badi (60) (900742), the magician's level 55 earth pet, a store-bought spell "
          "(no mastery's), its scroll 716955 at the Spell Archivists (tools/petpass_both_1124at.py)")

SPELL, SCROLL, NAME = 900742, 716955, "Summon Badi (60)"
SIBLING = 900746  # Summon: Strawberry Lemonade, the archivists' magician L55 scroll
OLD_SOURCE = "cm (eqoaemu)"
NEW_SOURCE = "vendor (eqoaemu; petition 2888: Prima p. 413/44, the L55 Badi is no mastery's)"
OLD_DESC, NEW_DESC = "CM: Summons a Badi to aid you.", "Summons a Badi to aid you."
MAGICIAN = 10


def changes(db):
    out, lines, refusals = [], [], []
    p = tr.get(db, "spellpattern", {"spellid": SPELL})
    if p is None or p["spellname"] != NAME or p["abilitylvl"] != 55:
        refusals.append(f"spell {SPELL}: not {NAME} at 55: {p and (p['spellname'], p['abilitylvl'])}")
    elif p["spelldesc"] not in (OLD_DESC, NEW_DESC):
        refusals.append(f"spell {SPELL}: description {p['spelldesc']!r}, not as imported")
    else:
        out.append(("spellpattern", {"spellid": SPELL}, {"spelldesc": NEW_DESC}))
        lines.append(f"spellpattern {SPELL}: desc {p['spelldesc']!r} -> {NEW_DESC!r}")
    ckey = {"class": MAGICIAN, "spellid": SPELL}
    c = tr.get(db, "classspells", ckey)
    if c is None or c["source"] not in (OLD_SOURCE, NEW_SOURCE):
        refusals.append(f"classspells {ckey}: {c and c['source']!r}, not {OLD_SOURCE!r}")
    else:
        out.append(("classspells", ckey, {"source": NEW_SOURCE}))
        lines.append(f"classspells {ckey}: source {c['source']!r} -> {NEW_SOURCE!r}")
    s = tr.get(db, "itempattern", {"patternid": SCROLL})
    if s is None or s["itemname"] != NAME or s["levelreq"] != 55 or s["classuse"] != 1 << MAGICIAN:
        refusals.append(f"scroll {SCROLL}: not the magician's L55 {NAME}: {s and (s['itemname'], s['levelreq'])}")
    archivists = [r["id"] for r in tr.rows(db, "SELECT id FROM npcs WHERE npc_name = 'Spell Archivist' ORDER BY id")]
    nxt = (db.execute("SELECT COALESCE(MAX(itemid), 0) FROM npcitem").fetchone()[0] or 0) + 1
    added = 0
    for a in archivists:
        have = tr.rows(db, "SELECT * FROM npcitem WHERE npcid = ? AND patternid = ?", (a, SCROLL))
        if len(have) == 1:
            out.append(("npcitem", {"itemid": have[0]["itemid"]}, have[0]))  # there (as written)
            continue
        if have:
            refusals.append(f"archivist {a}: {len(have)} rows of {SCROLL}")
            continue
        tmpl = tr.rows(db, "SELECT * FROM npcitem WHERE npcid = ? AND patternid = ?", (a, SIBLING))
        if len(tmpl) != 1:
            refusals.append(f"archivist {a}: {len(tmpl)} rows of {SIBLING}, want one to place it by")
            continue
        row = dict(tmpl[0], itemid=nxt, patternid=SCROLL)
        out.append(("npcitem", {"itemid": nxt}, row))
        nxt += 1
        added += 1
    lines.append(f"Spell Archivists: {len(archivists)}, the scroll added to {added} (listnumber as {SIBLING}'s)")
    if refusals:
        out = []
    applied = bool(out) and all(tr.get(db, t, k) is not None and
                                all(tr.get(db, t, k).get(col) == v for col, v in a.items()) for t, k, a in out)
    lines.insert(0, f"{len(out)} rows")
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("spellpattern", "classspells", "itempattern", "npcitem", "npcs"),
        "changes": changes, "basis": "era", "basis_ref": "Prima pp.44,413; questmasters id534"}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, argv=sys.argv[1:], doc=__doc__))
