#!/usr/bin/env python3
"""Live petitions 5571 and 5572 (Warsloth, a level 17 ogre warrior, 2026-11-23): "madmen camp is whiteface" and
"decaying skeleton is whiteface at madmen aswell", at the desert madmen's camp on the coast south of Freeport (world
0, about (25020, 16720)).

Why white: the camp's twelve desert madmen wear a human body (0x70D898C6, a players' race's), so by their nature they
attack no one (entity/aggro.go, hostileNatureLocked), and no npcfaction row named them. Twelve such bodies in a chain
also make the camp a "town" (entity/towns.go: at least 10 of a players' race within 60 of the next), and a creature
spawned within 40 of a town's people is not hostile by nature either: so four of the camp's five decaying skeletons
(105035-105037, 105045; a skeleton's body, 0x6AF98A81, hostile anywhere else) showed the white face too.

  - desert madman (world 0: all twelve rows are this camp) are of Madman, a faction of the live game's own list (the
    eqoa.live capture's standings, internal/database/factions.go's DefaultFactions), every character's start -500 and
    no factionstart row raising it: below -350, the red face, attacked on sight within the aggro radius. No row had
    used the faction before.
  - decaying skeleton: no era source puts them in a faction (Prima's faction tables and EQOA-Data name none for
    them), so none is invented: an npcaggro row (aggressive, the radius and leash their own) says they are creatures
    whatever stands beside them, as jib's rows did for Castle Mistmoore's creatures in its human "town". The other
    decaying skeletons of world 0 are hostile already; the row changes only the ones in a town.

The same on Heirloom (no row for either name there either). Report only by default; --apply in one transaction, a
gm_audit line a row, log petpass_live_1123u_log; --undo exact. npcfaction and npcaggro are read at the server's start.

    python3 -E -s tools/petpass_live_1123u.py --db eqoa.db [--apply | --undo]
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_live_1123u_log"
DETAIL = "Live petitions 5571/5572: the desert madmen's camp red-faced (tools/petpass_live_1123u.py)"
SOURCE = "petpass-1123u"
FACTION = "Madman"
CAMP = (25020.0, 16720.0, 200.0)


def changes(db):
    out, lines, refusals = [], [], []
    f = tr.rows(db, "SELECT standing FROM factions WHERE name = ?", (FACTION,))
    if not f:
        refusals.append(f"no faction {FACTION!r} here")
    elif f[0]["standing"] > -350:
        refusals.append(f"{FACTION} starts at {f[0]['standing']}, not KOS")
    starts = tr.rows(db, "SELECT count(*) AS c FROM factionstart WHERE faction = ? AND standing > -350", (FACTION,))
    if starts[0]["c"]:
        lines.append(f"note: {starts[0]['c']} factionstart rows start someone above -350 with {FACTION}")
    others = tr.rows(db, "SELECT name, world, faction FROM npcfaction WHERE lower(name) = 'desert madman'")
    if others:
        refusals.append(f"npcfaction already names desert madman: {others}")
    rule = tr.get(db, "npcaggro", {"npc_name": "decaying skeleton"})
    if rule is not None:
        refusals.append(f"npcaggro already has decaying skeleton: {rule}")

    cx, cz, r = CAMP
    n = tr.rows(db, "SELECT count(*) AS c FROM npcs WHERE lower(npc_name) = 'desert madman' AND world = 0")[0]["c"]
    near = tr.rows(db, "SELECT count(*) AS c FROM npcs WHERE lower(npc_name) = 'desert madman' AND world = 0 AND "
                       "abs(x_coord - ?) <= ? AND abs(z_coord - ?) <= ?", (cx, r, cz, r))[0]["c"]
    key = {"name": "desert madman", "world": 0, "x": 0.0, "z": 0.0, "radius": 0.0}
    out.append(("npcfaction", key, dict(key, faction=FACTION, note="Live petition 5571: the madmen's camp KOS (red), "
                                       "the live game's Madman faction", source=SOURCE)))
    lines.append(f"5571: desert madman, world 0 ({n} rows, {near} at the camp) -> npcfaction {FACTION}")

    ds = tr.rows(db, "SELECT count(*) AS c FROM npcs WHERE lower(npc_name) = 'decaying skeleton' AND world = 0 AND "
                     "abs(x_coord - ?) <= ? AND abs(z_coord - ?) <= ?", (cx, r, cz, r))[0]["c"]
    k2 = {"npc_name": "decaying skeleton"}
    out.append(("npcaggro", k2, dict(k2, aggressive=1, radius=0.0, leash=0.0, holding=0, see_invis=0, holds=0,
                                     note="Live petition 5572: aggressive; the desert madmen's human bodies make their "
                                          "camp a town (towns.go), which left the skeletons there peaceful",
                                     source=SOURCE)))
    lines.append(f"5572: decaying skeleton ({ds} at the camp) -> npcaggro aggressive (no faction: none in the sources)")
    applied = all(tr.get(db, t, k) is not None and all(tr.get(db, t, k).get(c) == v for c, v in a.items())
                  for t, k, a in out)
    lines.insert(0, f"{len(out)} rows")
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("npcfaction", "npcaggro", "factions", "factionstart", "npcs"),
        "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
