#!/usr/bin/env python3
"""Two of the owner's asks (2026-11-14), both shards:

canyon -- "box canyon needs wildlife spread out". Box Canyons (Tunaria's zone 121, x and z 20000-22000) is a maze of
  mesas (their tops ~146 high, no way up) with the canyon floor (~5 high) winding between them, open to the desert
  (~55) at its edges. Its wildlife stood as the captures had it: in heaps of 3-6, and more than half of it on the
  mesa tops and cliffs, where no one walks. The 126 rows the server wanders as wildlife (its verdict: an animal by its
  name: inferno, torrid drakes, plague, pox griffons, crag spiders, calescent ants, singetail scorpions), on the
  canyon floor or on a mesa or a cliff no one reaches, inside the mesas' span (x, z 20200-21900), are moved to places
  spread over the canyon floor (data_1114c_20261114.json, from cmd/canyonspread's survey: the floor reachable from the
  canyon floor, 20 in from the cliffs' feet, 45 from every NPC that stays: the Majorkahn giants', the Taksehir's,
  the undead's camps, the quest NPCs, the named; 40 from a quest's reach place; each place 58 or more from the next).
  Only x_coord, y_coord, z_coord change: kind, level, body, respawn, wander and facing stay. Left as they are: a row
  a path, a sign, a quest step or a camp spawn names, one in a cave under the floor, the desert's wildlife round the
  canyon. Each row is refused unless it is the name and level the plan says, at one of its places on the shards.
  npcs are read at the server's start: restart (a keep restart carries the creatures to their new spawns: keepnpc.go,
  a moved row).

wounds -- "rogue wounds aren't strong enough they are supposed to be stronger they are modded by strength, zaeire is
  testing them. make wounds 75% stronger also they apply snare". The rogue's wound line (classspells, class 6, found
  by name: the ascended ones' ids differ by shard): each hit's base amount x 1.75 (its STR scale kept: every one has
  it, 30% of STR, 33% the ascended), the Mortal Wounds' damage a tic x 1.75 too, and the slow each already had
  (speed -30 for the spell's 18 or 24 s; its line said "Root", the effects a 30% snare) by tier: 30% at 20 and 30, 35%
  at 40 and Grievous, 40% for Mortal, Sothimus and the ascended. spelleffect.effects and its line, and the spell's
  description (spellpattern.spelldesc, its "Ascended (+10%): " kept), say the new figures. Refused unless each record
  is as it was. A cast reads its spell's row (handlers/cast.go): no restart; the book's descriptions are sent at login.

Report only by default; --apply in one transaction per part, a gm_audit line a row, logs data_1114c_canyon_log and
data_1114c_wounds_log; --undo exact; --only canyon,wounds (default both).

    python3 -E -s tools/data_1114c_20261114.py --db eqoa.db [--apply | --undo] [--only canyon,wounds]
"""
import json
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

HERE = os.path.dirname(os.path.abspath(__file__))
PLAN = os.path.join(HERE, "data_1114c_20261114.json")
NEAR = 0.01

# --- canyon ---------------------------------------------------------------------------------------------------------


def canyon_changes(db):
    moves = json.load(open(PLAN))["moves"]
    out, lines, refusals = [], [], []
    done = 0
    for m in moves:
        r = tr.get(db, "npcs", {"id": m["id"]})
        if r is None:
            refusals.append(f"npcs {m['id']} ({m['name']}): no row")
            continue
        if r["npc_name"] != m["name"] or r["npc_level"] != m["level"] or r["world"] != 0:
            refusals.append(f"npcs {m['id']}: {r['npc_name']!r} level {r['npc_level']} world {r['world']}, "
                            f"not {m['name']!r} level {m['level']} world 0")
            continue
        at = (r["x_coord"], r["y_coord"], r["z_coord"])
        if all(abs(a - b) <= NEAR for a, b in zip(at, m["to"])):
            done += 1
            continue
        if not any(all(abs(a - b) <= NEAR for a, b in zip(at, f)) for f in m["from"]):
            refusals.append(f"npcs {m['id']} ({m['name']}): at ({at[0]:.2f}, {at[1]:.2f}, {at[2]:.2f}), "
                            f"not at the plan's {m['from']}")
            continue
        x, y, z = m["to"]
        out.append(("npcs", {"id": m["id"]}, {**r, "x_coord": x, "y_coord": y, "z_coord": z}))
        lines.append(f"npcs {m['id']} {m['name']} (level {m['level']}): ({at[0]:.1f}, {at[1]:.1f}, {at[2]:.1f}) -> "
                     f"({x:.1f}, {y:.1f}, {z:.1f})")
    return out, lines, refusals, done == len(moves)


CANYON = {"log": "data_1114c_canyon_log", "keys": {"npcs": ("id",)}, "needs": ("npcs",), "changes": canyon_changes,
          "detail": "Box Canyon's wildlife spread over the canyon floor (the owner, 2026-11-14): tools/data_1114c_20261114.py"}

# --- wounds ---------------------------------------------------------------------------------------------------------

# name: (the hit's base amount, its STR scale %, its damage a tic or None, its slow, as they are; the new slow)
WOUNDS = {
    "Minor Wound": (-330, 30, None, -30, -30),
    "Lesser Wound": (-515, 30, None, -30, -30),
    "Wound": (-970, 30, None, -30, -35),
    "Grievous Wound": (-1300, 30, None, -30, -35),
    "Mortal Wound": (-1400, 30, -25, -30, -40),
    "Ascended Mortal Wound": (-1540, 33, -28, -30, -40),
    "Sothimus Critical Wounding": (-1600, 30, None, -30, -40),
    "Ascended Sothimus Critical Wounding": (-1760, 33, None, -30, -40),
}
STRONGER = 1.75


def stronger(n):
    """n x 1.75, halves away from 0."""
    v = abs(n) * STRONGER
    return -int(v + 0.5) if n < 0 else int(v + 0.5)


def wound_line(hp, scale, tick, slow, secs):
    s = f"Target, Snare {-slow}%, Decrease Hitpoints by {-hp} ; Increase {scale}% based on STR"
    if tick is not None:
        s += f", Decrease HP per tic by {-tick}"
    return s + f", {secs}sec"


def wound_desc(old, line, hp, scale, slow, secs):
    """The description: the line's words after its "Ascended (+10%): ", or, for an epic's own words ("Level 60 Epic
    Wound path. ... 1996 Damage, at 525 Str"), those words with the damage at that STR and the snare."""
    m = re.match(r"^(Ascended \(\+\d+%\): )?(.*)$", old, re.S)
    prefix, body = m.group(1) or "", m.group(2)
    if body.startswith("Target,") or not body:
        return prefix + line
    d = re.search(r"(\d+) Damage, at (\d+) Str", body)
    if d is None:
        return prefix + line
    at = int(d.group(2))
    body = body[:d.start()] + f"{int(-hp + scale * at / 100 + 0.5)} Damage, at {at} Str" + body[d.end():]
    return prefix + body.rstrip(". ") + f". Snare {-slow}%, {secs}sec."


def wounds_changes(db):
    out, lines, refusals = [], [], []
    done = 0
    found = tr.rows(db, "SELECT p.spellid, p.spellname FROM classspells c JOIN spellpattern p ON p.spellid = c.spellid "
                        "WHERE c.class = 6 AND p.spellname LIKE '%wound%' ORDER BY p.spellid")
    names = [f["spellname"] for f in found]
    for n in WOUNDS:
        if names.count(n) != 1:
            refusals.append(f"the rogue has {names.count(n)} spells named {n!r}, not 1")
    for extra in sorted(set(names) - set(WOUNDS)):
        refusals.append(f"the rogue's {extra!r} is not in this tool's line")
    for f in found:
        if f["spellname"] not in WOUNDS:
            continue
        sid, name = f["spellid"], f["spellname"]
        hp0, scale, tick0, slow0, slow1 = WOUNDS[name]
        hp1, tick1 = stronger(hp0), None if tick0 is None else stronger(tick0)
        se = tr.get(db, "spelleffect", {"spellid": sid})
        sp = tr.get(db, "spellpattern", {"spellid": sid})
        try:
            eff = json.loads(se["effects"]) if se else None
        except ValueError:
            eff = None
        if eff is None:
            refusals.append(f"spelleffect {sid} ({name}): no effects")
            continue
        es = eff.get("effects", [])
        hits = [e for e in es if e.get("kind") == "hp"]
        speeds = [e for e in es if e.get("kind") == "speed"]
        ticks = [e for e in es if e.get("kind") == "hp_tick"]
        shape = (len(hits), len(speeds), len(ticks), len(es))
        want_shape = (1, 1, 0 if tick0 is None else 1, 2 if tick0 is None else 3)
        if shape != want_shape or hits[0].get("scale") != "str" or hits[0].get("scale_pct") != scale or \
                any(e.get("absolute") for e in speeds):
            refusals.append(f"spelleffect {sid} ({name}): effects {es}, not the wound's shape")
            continue
        now = (hits[0].get("amount"), speeds[0].get("amount"), ticks[0].get("amount") if ticks else None)
        if now == (hp1, slow1, tick1):
            done += 1
            continue
        if now != (hp0, slow0, tick0):
            refusals.append(f"spelleffect {sid} ({name}): hit, slow, tic {now}, not {(hp0, slow0, tick0)}")
            continue
        hits[0]["amount"], speeds[0]["amount"] = hp1, slow1
        if ticks:
            ticks[0]["amount"] = tick1
        secs = int(eff.get("duration_s") or 0)
        line = wound_line(hp1, scale, tick1, slow1, secs)
        out.append(("spelleffect", {"spellid": sid},
                    {**se, "effects": json.dumps(eff, separators=(",", ":")), "line": line}))
        desc = wound_desc(sp["spelldesc"] or "", line, hp1, scale, slow1, secs)
        out.append(("spellpattern", {"spellid": sid}, {**sp, "spelldesc": desc}))
        lines.append(f"{sid} {name}: hit {hp0} -> {hp1} (+{scale}% STR kept)"
                     + (f", tic {tick0} -> {tick1}" if tick0 is not None else "")
                     + f", slow {-slow0}% -> {-slow1}% for {secs} s")
        lines.append(f"    line: {se['line']!r} -> {line!r}")
        lines.append(f"    desc: {sp['spelldesc']!r} -> {desc!r}")
    return out, lines, refusals, done == len(WOUNDS)


WOUNDSPEC = {"log": "data_1114c_wounds_log", "keys": {"spelleffect": ("spellid",), "spellpattern": ("spellid",)},
             "needs": ("spelleffect", "spellpattern", "classspells"), "changes": wounds_changes,
             "detail": "The rogue's wounds 75% stronger, their snare by tier (the owner, 2026-11-14): "
                       "tools/data_1114c_20261114.py"}

PARTS = {"canyon": CANYON, "wounds": WOUNDSPEC}

if __name__ == "__main__":
    argv = sys.argv[1:]
    only = list(PARTS)
    if "--only" in argv:
        i = argv.index("--only")
        only = [p.strip() for p in argv[i + 1].split(",") if p.strip()]
        del argv[i:i + 2]
        if not only or any(p not in PARTS for p in only):
            sys.exit(f"--only: {', '.join(PARTS)}")
    rc = 0
    for p in only:
        print(f"== {p}")
        rc |= tr.main(PARTS[p], argv=argv, doc=__doc__)
    sys.exit(rc)
