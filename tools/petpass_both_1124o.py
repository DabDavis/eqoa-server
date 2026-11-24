#!/usr/bin/env python3
"""Heirloom petition 2848 (Vlad, 2026-11-24, Faction): "Tavern rats in the tavern in Neriak are taking away rep with
Neriak. They should give rep for Neriak; I had to grind them for rep back in the day."

The tavern rat (7 spawns, level 2) and the tavern rat breeder (1, level 10) live in the Double Daggers Inn's cellar in
Neriak (world 0, about (25325, 8755), beside the inn's owner Forgun E`Phet, whose quest in Prima's Frontiers guide is
"go downstairs and kill the rats ... kill the tavern rats until the tavern rat breeder spawns"). The town seed bound
them to Citizens of Neriak by place (npcfaction 2504, 2505, source seed), and petfix582 (2026-11-19, "townsfolk who
had none") then gave them Neriak's own kill hits: Citizens of Neriak -10, The Indigo Brotherhood -10, -5 to the
Spurned, Church of Innoruuk, Lodge of the Dead, Ebon Mask and Fort Seriak, +1 Last Defenders of Feydwell.

Prima's faction tables (pp. 383-392) have the inn's own vermin faction: Double Dagger Rats, kill penalty 1 (Table 1),
one associate, Citizens of Neriak at -100 care (Table 2): an enemy, so a kill raises Neriak. petfix582's rule
(prima_hits: the faction's own penalty, each associate rating% of it, a share under 1 point no row, a fraction cut
toward 0): Double Dagger Rats -1; Citizens of Neriak -(1 x -100%) = +1, a whole point, so it stays.

So both names: npcfaction -> Double Dagger Rats (the server's default standing for it is -500, as before this pass;
they now attack on sight those to whom they are not green, as every vermin faction's creatures do), and their kill
hits -> Double Dagger Rats -1, Citizens of Neriak +1 (the other seven rows removed).

Not changed (reported): petfix582 bound other place-bound generic names to a city's citizens, among them sunbeetle and
scruffy (Qeynos) and ghoul servant (Neriak); Prima gives none of them a faction of its own (a sunbeetle is a
bestiary creature), so they keep their rows.

Live's rows are the same: mirrored. Report only by default; --apply in one transaction, a gm_audit line a row, log
petpass_both_1124o_log; --undo exact. factionhits and npcfaction are read at the start.

    python3 -E -s tools/petpass_both_1124o.py --db eqoa.db [--apply | --undo]
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_both_1124o_log"
DETAIL = ("Heirloom petition 2848: Neriak's tavern rats bound to Double Dagger Rats, Prima's kill hits (Double Dagger "
          "Rats -1, Citizens of Neriak +1) (tools/petpass_both_1124o.py)")
SOURCE = "prima-2003 petpass-1124o"
WAS_SOURCE = "prima-2003 petfix582"
FACTION, CITY = "Double Dagger Rats", "Citizens of Neriak"
NAMES = {"tavern rat": 2504, "tavern rat breeder": 2505}  # npcfaction row ids

# Prima Table 1 / Table 2 for the faction (as petfix582's PRIMA_T1 / PRIMA_T2)
PRIMA_T1 = {FACTION: 1}
PRIMA_T2 = {FACTION: [[CITY, -100]]}
# petfix582's rows for each name (faction -> delta), the only rows a name may have before this pass
WAS = {CITY: -10, "The Indigo Brotherhood": -10, "The Spurned": -5, "The Church of Innoruuk": -5,
       "Lodge of the Dead": -5, "The Ebon Mask": -5, "Fort Seriak": -5, "Last Defenders of Feydwell": 1}


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


def changes(db):
    out, lines, refusals = [], [], []
    want = prima_hits(FACTION)
    if want != {FACTION: -1, CITY: 1}:
        refusals.append(f"Prima's hits computed as {want}")
    if not tr.rows(db, "SELECT 1 FROM npcs WHERE npc_name = ? LIMIT 1", ("tavern rat",)):
        refusals.append("no tavern rat in npcs")
    for name, nid in NAMES.items():
        k = {"id": nid}
        r = tr.get(db, "npcfaction", k)
        others = tr.rows(db, "SELECT id FROM npcfaction WHERE lower(name) = lower(?) AND id != ?", (name, nid))
        if others:
            refusals.append(f"npcfaction has other rows for {name}: {[o['id'] for o in others]}")
        if r is None or r["name"] != name or r["world"] != -1:
            refusals.append(f"npcfaction {nid} is not {name} (world -1): {r}")
        elif r["faction"] not in (CITY, FACTION):
            refusals.append(f"npcfaction {nid} {name} is bound to {r['faction']}")
        else:
            out.append(("npcfaction", k, dict(r, faction=FACTION, source="prima-2003 petpass-1124o",
                                             note="Double Daggers Inn's cellar, Neriak (Prima: Double Dagger Rats); "
                                                  "was " + r["note"] if r["faction"] == CITY else r["note"])))
            lines.append(f"npcfaction {nid} {name}: {r['faction']} -> {FACTION}")
        have = {h["faction"]: h for h in tr.rows(db, "SELECT * FROM factionhits WHERE npc_name = ?", (name,))}
        odd = [f for f, h in have.items() if not ((f in WAS and h["delta"] == WAS[f] and h["source"] == WAS_SOURCE)
                                                   or (f in want and h["delta"] == want[f] and h["source"] == SOURCE))]
        if odd:
            refusals.append(f"factionhits {name}: rows not petfix582's nor this tool's: {[have[f] for f in odd]}")
        for f in sorted(set(have) | set(want)):
            hk = {"npc_name": name, "faction": f}
            if f in want:
                after = {"npc_name": name, "faction": f, "delta": want[f], "source": SOURCE}
                if f in have:
                    after = dict(have[f], delta=want[f], source=SOURCE)
                out.append(("factionhits", hk, after))
                lines.append(f"factionhits {name} / {f}: {have[f]['delta'] if f in have else '(none)'} -> {want[f]:+d}")
            else:
                out.append(("factionhits", hk, None))
                lines.append(f"factionhits {name} / {f}: {have[f]['delta']:+d} -> removed")
    if refusals:
        out = []
    applied = bool(out) and all(
        (tr.get(db, t, k) is None) if a is None else
        (tr.get(db, t, k) is not None and all(tr.get(db, t, k).get(c) == v for c, v in a.items()))
        for t, k, a in out)
    lines.insert(0, f"{len(out)} rows")
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("npcs", "npcfaction", "factionhits"), "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
