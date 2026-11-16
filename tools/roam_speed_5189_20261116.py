#!/usr/bin/env python3
"""Crocodilians' wandering walk (petition 5189, Innoruuk, 2026-11-16: "Gharial conned runs too fast when roaming").

Every wandering creature walked at the server's 5.0 units a second (entity.NPCWalkSpeed, a man's walk), and CLIENT
plays the walk (state channel byte 58 = 1) at the body's own rate whatever the velocity (VIClient::UpdateCreatures ->
VICSprite::SetLocomotion; the frames a second are the animation's play rate times its playlist slot's speed,
VIHSprite::PrecalcPlayback). The four crocodilian bodies of CHAR.ESF share one walk (0xCAC3A9F6, 8 frames at 10 a
second) whose feet sweep 0.46 (front) and 0.84 (rear) units back a cycle, against a man's 1.20 (0x70D898C6's walk
0xF1797FA4): at 5.0 they slid along the ground at about twice a man's slide, legs paddling, which reads as a sprint.

Each body's walk measured off CHAR.ESF (stride by the body's DefaultScale, cycle by its slot speed), as a man's
5.0 is to his walk, mean of two measures (foot sweep a cycle, and the planted foot's own speed):
    0x015C345E  scale 1.2, slot 0.8 (1.00 s cycle)   3.0     the gharial
    0x0263929E  scale 1.0, slot 1.0 (0.80 s)         3.2     the crocodile
    0x3E0EBFE1  scale 0.8, slot 0.9 (0.89 s)         2.3     the alligator
    0x88806543  scale 0.5, slot 1.2 (0.67 s)         1.9     the caiman
A name's speed is its body's times its rows' size (most of its rows'; a name on two bodies weighted by rows).

Writes npcmove.speed (the server's new column: 0 is the server's 5.0): an update of a name's row where it has one
(its other columns kept), an insert of a row as its kind otherwise (roam -1, radius 0, water '': as no row). The
column is added when missing, as the build with it does at its start. Report only by default; --apply in one
transaction, a gm_audit line a row, log roam_speed_5189_log; --undo exact. Live at once on a server with the
column's build (a GM panel edit reloads it; else at its next start); an older build ignores the column.

    python3 -E -s tools/roam_speed_5189_20261116.py --db ~/eqoa-server/eqoa.db [--apply | --undo]
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "roam_speed_5189_log"
DETAIL = ("Petition 5189 (Innoruuk): crocodilians wander at their walk's own pace, not a man's 5.0 "
          "(CHAR.ESF's walk cycles): tools/roam_speed_5189_20261116.py")
SOURCE = "roam-speed-5189"

# name -> (speed, why)
SPEEDS = {
    "gharial": (3.0, "0x015C345E"),
    "shoreline gharial": (3.3, "0x015C345E at size 1.1"),
    "ancient gharial": (3.3, "0x015C345E, most at size 1.125 (6 of 53 on 0x3E0EBFE1)"),
    "mudscale": (4.5, "0x015C345E at size 1.5"),
    "crocodile": (3.2, "0x0263929E (1 of 8 on 0x015C345E)"),
    "ancient crocodile": (3.2, "0x0263929E"),
    "swampwater crocodile": (3.2, "0x0263929E"),
    "tame crocodile": (3.2, "0x0263929E"),
    "slitnose alligator": (3.2, "0x0263929E (2 of 58 on 0x3E0EBFE1)"),
    "saltwater crocodile": (2.8, "0x0263929E x3, 0x3E0EBFE1 x2"),
    "young crocodile": (2.5, "0x0263929E most at size 0.812, 0x3E0EBFE1"),
    "alligator": (2.0, "0x88806543 (10 of 62 on 0x3E0EBFE1)"),
    "raging alligator": (2.3, "0x3E0EBFE1"),
    "crocosaurus": (2.3, "0x3E0EBFE1"),
    "subterranean gator": (2.3, "0x3E0EBFE1"),
    "ancient ghariel": (2.3, "0x3E0EBFE1"),
    "alleza": (2.3, "0x3E0EBFE1"),
    "bogscraper": (2.3, "0x3E0EBFE1"),
    "da snapper": (2.3, "0x3E0EBFE1"),
    "gnawtooth": (2.3, "0x3E0EBFE1"),
    "man hunter": (2.3, "0x3E0EBFE1"),
    "thulian maneater": (2.3, "0x3E0EBFE1"),
    "trapjaw": (2.3, "0x3E0EBFE1"),
    "caiman": (1.9, "0x88806543"),
    "young caiman": (1.9, "0x88806543"),
    "cave alligator": (1.9, "0x88806543"),
}


def changes(db):
    has_speed = tr.column(db, "npcmove", "speed")
    out, lines = [], []
    for name, (speed, why) in sorted(SPEEDS.items()):
        key = {"name": name}
        have = tr.get(db, "npcmove", key)
        if have is None:
            out.append(("npcmove", key, {"name": name, "speed": speed,
                                         "note": f"walks {speed} (petition 5189: {why}'s walk)", "source": SOURCE}))
            lines.append(f"  {name:<22} {speed:>4} new row (as its kind)   {why}")
        elif not has_speed or have.get("speed") != speed:
            out.append(("npcmove", key, {"name": name, "speed": speed}))
            lines.append(f"  {name:<22} {speed:>4} its row ({have.get('speed', 0) if has_speed else 0} -> {speed})  {why}")
    lines.insert(0, f"npcmove speed for {len(SPEEDS)} names, {len(out)} to write"
                 + ("" if has_speed else " (the speed column added first)"))
    return out, lines, [], not out


def prepare(db):
    if tr.column(db, "npcmove", "speed"):
        return []
    db.execute("ALTER TABLE npcmove ADD COLUMN speed REAL NOT NULL DEFAULT 0")
    return ["added npcmove.speed (as the server's start does)"]


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("npcmove",), "changes": changes, "prepare": prepare}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
