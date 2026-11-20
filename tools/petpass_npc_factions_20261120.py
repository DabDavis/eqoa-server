#!/usr/bin/env python3
"""Who attacks on sight and what a kill does, as a GM asked in four petitions (2026-11-20; Live, the GM's requests
approved as submitted):

  5341  "Deathfist mercenary's should be KoS to players"
  5343  "Troll crusader's and troll raider's in this camp I'm standing at should be KoS to players" (the GM at
        (24140.5, 26589.4), world 0: the camp of ten crusaders and five raiders within 60 of (24158, 26585))
  5324  "Prisoners, felons in qeynos prison should be KoS. When killed they should give positive qeynos faction for
        guard, city, etc. They should give negative qeynos prisoner faction"
  5330  "Orcs in thedruk also gave freeport faction (guards, citizens) and the same for fayspire and tethelin"

An NPC of a faction (npcfaction) attacks a player on sight whose standing with it is below -350, at any level it is
not green to (entity/aggro.go); a body of a players' race (the ogre and troll mercenaries, the human prisoners)
attacks no one by its nature. So:

  - deathfist mercenary (world 0, every one: all are in the Deathfist's country) and the camp's troll crusader and
    troll raider (that camp alone: the name's other camps and Honjour's crusaders are left as they are) are of
    Deathfist Mercenaries, which every character starts at -500 (factions.standing; no factionstart row raises it):
    KOS to everyone. The camp stands beside the Deathfist's myrmidons and oracles.
  - prisoner and felon inside the Qeynos prison (world 0, within 150 of (5465, 19035): 14 prisoners and 10 felons;
    the 2428 row for one prisoner stays) are of Qeynos Prisoners, -500 to all but bards and rogues (factionstart's
    0: Prima, "KOS to all except bards and rogues", left as it is). Their kill there (factionhitareas): Qeynos
    Prisoners -10 (Prima's penalty), The Qeynos Guard +5, Citizens of Qeynos +5 (ours: Prima gives the prisoners no
    associates).
  - every orc killed in Thedruk (world 0, the zone x 18000-20000, z 8000-10000; the orcs by their bodies: orc,
    orcish, hatebone and the named orcs there, 21 names on Live): +1 each with The Freeport Militia, Citizens of
    Freeport, The Paladins of Tunare (Fayspires' guards, Prima), Citizens of Fayspires, The Emerald Guardians
    (Tethelin's guards), Citizens of Tethelin, by area rows: within 1000 of (19000, 9000), the Hatebone camp's middle,
    and within 120 of (18380, 8220), the zone's south-west corner camp (thugs, a hatebone warrior and mystic). The
    camps on the Fayspires-Tethelin road just north of the zone (lockpickers, chemists, two spiritists, z 7770-7955)
    are left out. Refused if the circles hold a spawn of one of the names outside the zone, or miss one inside it.
    (The video audit's hatebone rows gave +1 to the Freeport and Tethelin four already: an area row of the same
    faction takes its place, the same +1.)

Report only by default; --apply in one transaction, a gm_audit line a row, log petpass_npc_factions_log; --undo exact.
npcfaction and factionhitareas are read at the server's start (after the next server update).

    python3 -E -s tools/petpass_npc_factions_20261120.py --db eqoa.db [--apply | --undo]
"""
import math
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_npc_factions_log"
DETAIL = "KOS and kill factions (a GM's petitions 5341, 5343, 5324, 5330): tools/petpass_npc_factions_20261120.py"
SOURCE = "petpass-1120"

MERCS = "Deathfist Mercenaries"
TROLL_CAMP = (24158.0, 26585.0, 60.0)
PRISON = (5465.0, 19035.0, 150.0)
THEDRUK_BOX = (18000.0, 20000.0, 8000.0, 10000.0)
THEDRUK = ((19000.0, 9000.0, 1000.0), (18380.0, 8220.0, 120.0))
ORC_BODIES = (0x0A03EB92, 0x656DD4C5, 0x8214A32D, 0xB2FBD7EF, 0xDA515962)
THEDRUK_HITS = ("The Freeport Militia", "Citizens of Freeport", "The Paladins of Tunare", "Citizens of Fayspires",
                "The Emerald Guardians", "Citizens of Tethelin")
PRISON_HITS = (("Qeynos Prisoners", -10), ("The Qeynos Guard", 5), ("Citizens of Qeynos", 5))


def member(name, world, x, z, radius, faction, note):
    key = {"name": name, "world": world, "x": x, "z": z, "radius": radius}
    return ("npcfaction", key, dict(key, faction=faction, note=note, source=SOURCE))


def area(name, faction, delta, x, z, radius, note):
    key = {"npc_name": name, "faction": faction, "world": 0, "x": x, "z": z, "radius": radius}
    return ("factionhitareas", key, dict(key, delta=delta, note=note, source=SOURCE))


def count(db, names, x, z, r):
    n = 0
    for row in tr.rows(db, "SELECT lower(npc_name) AS n, x_coord, z_coord FROM npcs WHERE world = 0"):
        if row["n"] in names and math.hypot(row["x_coord"] - x, row["z_coord"] - z) <= r:
            n += 1
    return n


def changes(db):
    out, lines, refusals = [], [], []
    have = {r["name"] for r in tr.rows(db, "SELECT name FROM factions")}
    for f in (MERCS,) + THEDRUK_HITS + tuple(f for f, _ in PRISON_HITS):
        if f not in have:
            refusals.append(f"no faction {f!r} here")

    n = tr.rows(db, "SELECT count(*) AS c FROM npcs WHERE lower(npc_name) = 'deathfist mercenary' AND world = 0")[0]["c"]
    out.append(member("deathfist mercenary", 0, 0.0, 0.0, 0.0, MERCS, "petition 5341 (a GM): KOS to players"))
    lines.append(f"5341: deathfist mercenary, world 0 ({n} rows) -> {MERCS}")

    x, z, r = TROLL_CAMP
    for name in ("troll crusader", "troll raider"):
        out.append(member(name, 0, x, z, r, MERCS, "petition 5343 (a GM): the camp's trolls KOS to players"))
        lines.append(f"5343: {name} within {r:.0f} of ({x:.0f}, {z:.0f}): {count(db, {name}, x, z, r)} rows -> {MERCS}")

    x, z, r = PRISON
    for name in ("prisoner", "felon"):
        out.append(member(name, 0, x, z, r, "Qeynos Prisoners", "petition 5324 (a GM): the Qeynos prison's KOS"))
        for f, d in PRISON_HITS:
            out.append(area(name, f, d, x, z, r, "petition 5324 (a GM): a kill in the Qeynos prison"))
        lines.append(f"5324: {name} within {r:.0f} of ({x:.0f}, {z:.0f}): {count(db, {name}, x, z, r)} rows -> "
                     f"Qeynos Prisoners; a kill there " + ", ".join(f"{f} {d:+d}" for f, d in PRISON_HITS))

    x0, x1, z0, z1 = THEDRUK_BOX
    body_sql = ",".join(str(b) for b in ORC_BODIES)
    spawns = tr.rows(db, f"""SELECT lower(npc_name) AS n, x_coord AS x, z_coord AS z FROM npcs WHERE world = 0
        AND (modelid & 4294967295) IN ({body_sql}) AND (npc_type & 128) = 0""")
    orcs = sorted({r["n"] for r in spawns if x0 <= r["x"] <= x1 and z0 <= r["z"] <= z1})
    inside = 0
    for row in tr.rows(db, "SELECT lower(npc_name) AS n, x_coord AS x, z_coord AS z FROM npcs WHERE world = 0"):
        if row["n"] not in orcs:
            continue
        in_box = x0 <= row["x"] <= x1 and z0 <= row["z"] <= z1
        in_circle = any(math.hypot(row["x"] - cx, row["z"] - cz) <= cr for cx, cz, cr in THEDRUK)
        if in_box != in_circle:
            refusals.append(f"5330: {row['n']} at ({row['x']:.0f}, {row['z']:.0f}): in Thedruk {in_box}, "
                            f"in its circles {in_circle}")
        inside += in_box
    for name in orcs:
        for cx, cz, cr in THEDRUK:
            if any(r["n"] == name and math.hypot(r["x"] - cx, r["z"] - cz) <= cr for r in spawns):
                for f in THEDRUK_HITS:
                    out.append(area(name, f, 1, cx, cz, cr, "petition 5330 (a GM): an orc of Thedruk"))
    lines.append(f"5330: {len(orcs)} orc names in Thedruk ({inside} rows): " + ", ".join(orcs))
    lines.append("      a kill there +1 each: " + ", ".join(THEDRUK_HITS))
    applied = all(tr.get(db, t, k) is not None and all(tr.get(db, t, k).get(c) == v for c, v in a.items())
                  for t, k, a in out)
    lines.insert(0, f"{len(out)} rows")
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("npcfaction", "factionhitareas", "factions", "npcs"),
        "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
