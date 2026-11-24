#!/usr/bin/env python3
"""Heirloom petition 2840 (Darkhavictwo, 2026-11-24, Quest): "I should not be able to attack, kill, and loot, the level
one chest for the quest im on, its at my current location".

The chest is the frosty chest (npcs 3068, world 0, (16262.7, 4324.4), Greyvax's cave beside the ice statue), the target
of step 5 of "The Remains of Manger MacGray" (100761): a give step, "Open the frosty chest beside the ice statue with the
ice key" (EQOA-Data Quests/23-47/23.json: "Receive ice key and open ice chest"). It is a thing to open, never a foe.
Its row has npc_type 0, so it was a creature: his pet killed it (01:36:38) and it dropped a level 1 creature's loot.
The ice statue beside it (122444) carries the NPC bit (npc_type 0x80): no one may attack it (entity/attack.go,
cast.go, area.go, aggro.go all refuse an NPC with the bit), and talk and give steps answer as ever. The chest gets
the same bit.

The same flaw, the same rule (npc_type |= 0x80), on every quest object that a talk/give step names and no kill or
loot step needs dead:
  - Parathoir Chest (3049; "Specialization" 100386/96/406/416 step 3: "Unlock the chest near the treasurer with the
    key"),
  - sand covered chest 3223 (by Mildred Morath; "Crow's Treasure" 100603 step 3: "the chest on the sea floor and open
    it"). The three on the skeleton pirates' island (162464-162466) are LEFT: "Specialization" 100130/140/150/160/476
    step 6 is a loot step on them ("Take the chiseled great axe of doom from the sand-covered chests"),
  - golem statue 114183, 114184 ("Thenon the Wise" 101242 step 3: "Give the statue ... The statue grinds aside"),
  - orb of light 108826 (a twin of 108825, which has the bit; "The Scepter of the Ritualists" 101028 step 20 a talk).
Left as they are: waterlogged chest 3213 (100130's loot step 8; the give steps' chests in world 2 have the bit
already), Anvil of Sarek 196278 (an event's hostile spawn of 101029, meant to be fought), and the chests that no quest
names (chest 3064, coin/luxury/magical/unkempt storage chest).

Step 2 is the coordinator's same-pass Heirloom petition 2841 (Deathknell): "Creepy man for quest 'Staff of Rainbow
magic' is friendly and unkillable". The log: his "Target is friendly." lines (01:37:07-01:38:17) were all on 0x18b91,
Baylor (101265, the captive, NPC bit, same body); Creepyman (101266, npcs 123027) took his spell at 01:39:26 and died
at 01:39:32, and the quest went on. So no flag: but Creepyman, Baylor's captor ("defeat Creepyman to free Baylor",
EQOA-Data Quests/Misc/acolyte-of-bristlebane.json), wears a players' race body and so attacks no one, looking just
like his prisoner. An npcaggro row (aggressive, as Ulria and Tesarin in petpass_both_1124f.py) makes him the foe.

Live's rows are the same: mirrored. Report only by default; --apply in one transaction, a gm_audit line a row, logs
petpass_both_1124k_log / petpass_both_1124k_creepyman_log; --undo exact. npcs and npcaggro are read at the start.

    python3 -E -s tools/petpass_both_1124k.py [--step 2] --db eqoa.db [--apply | --undo]
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_both_1124k_log"
DETAIL = ("Heirloom petition 2840: quest chests and objects (talk/give targets) given the NPC bit, not attackable "
          "(tools/petpass_both_1124k.py)")
SOURCE = "petpass-1124k"
NPC_BIT = 0x80

# row id -> (name, the step that names it)
OBJECTS = {
    3068: ("frosty chest", "100761/5 give"),
    3049: ("Parathoir Chest", "100386/3 give"),
    3223: ("sand covered chest", "100603/3 give"),
    114183: ("golem statue", "101242/3 give"),
    114184: ("golem statue", "101242/3 give"),
    108826: ("orb of light", "101028/20 talk"),
}


def changes(db):
    out, lines, refusals = [], [], []
    for rid, (name, why) in OBJECTS.items():
        key = {"id": rid}
        n = tr.get(db, "npcs", key)
        if n is None or n["npc_name"] != name:
            refusals.append(f"npcs {rid} is not {name}: {n and n['npc_name']}")
            continue
        t = n["npc_type"] or 0
        if t & ~NPC_BIT:
            refusals.append(f"npcs {rid} {name} npc_type {t} has other bits (a GM's edit?)")
            continue
        qid, step = why.split()[0].split("/")
        s = tr.rows(db, "SELECT kind, target FROM queststeps WHERE questid = ? AND step = ?", (int(qid), int(step)))
        if not s or s[0]["target"].lower() != name.lower() or s[0]["kind"] != why.split()[1]:
            refusals.append(f"quest step {why.split()[0]} does not {why.split()[1]} {name}: {s}")
            continue
        out.append(("npcs", key, dict(n, npc_type=t | NPC_BIT)))
        lines.append(f"npcs {rid} {name}: npc_type {t} -> {t | NPC_BIT} (the NPC bit; {why})")
    # a loot or kill step needs the object dead: none may name a row changed here, but for the island's chests
    for name in sorted({v[0] for v in OBJECTS.values()}):
        k = tr.rows(db, "SELECT questid, step, kind FROM queststeps WHERE kind IN ('kill', 'loot') AND "
                        "lower(target) = lower(?)", (name,))
        if k:
            lines.append(f"note: {name} is a {'/'.join(sorted({r['kind'] for r in k}))} target of "
                         f"{len(k)} steps ({', '.join(str(r['questid']) for r in k)}); its other rows stay attackable")
    if refusals:
        out = []
    applied = bool(out) and all(tr.get(db, t, k) is not None and
                                all(tr.get(db, t, k).get(c) == v for c, v in a.items()) for t, k, a in out)
    lines.insert(0, f"{len(out)} rows")
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("npcs", "queststeps"), "changes": changes}

LOG2 = "petpass_both_1124k_creepyman_log"
DETAIL2 = ("Heirloom petition 2841: Creepyman, Baylor's captor in The Staff of Rainbow Magic: The Green Gem, "
           "aggressive (tools/petpass_both_1124k.py --step 2)")
NAME2, ROW2 = "Creepyman", 123027


def changes2(db):
    out, lines, refusals = [], [], []
    n = tr.get(db, "npcs", {"id": ROW2})
    if n is None or n["npc_name"] != NAME2:
        refusals.append(f"npcs {ROW2} is not {NAME2}: {n and n['npc_name']}")
    elif (n["npc_type"] or 0) & NPC_BIT:
        refusals.append(f"npcs {ROW2} {NAME2} has the NPC bit (npc_type {n['npc_type']}): not attackable at all")
    others = tr.rows(db, "SELECT count(*) AS c FROM npcs WHERE npc_name = ? AND id != ?", (NAME2, ROW2))[0]["c"]
    if others:
        refusals.append(f"{others} other npcs rows are named {NAME2}")
    k = {"npc_name": NAME2}
    rule = tr.get(db, "npcaggro", k)
    if rule is not None and rule.get("source") != SOURCE:
        refusals.append(f"npcaggro already has {NAME2}: {rule}")
    else:
        out.append(("npcaggro", k, dict(k, aggressive=1, radius=0.0, leash=0.0, holding=0, see_invis=0, holds=0,
                                        note="Heirloom petition 2841: Baylor's captor, defeated in the guides; a "
                                             "players' race body left him peaceful, like his prisoner",
                                        source=SOURCE)))
        lines.append(f"2841: npcaggro {NAME2} -> aggressive")
    if refusals:
        out = []
    applied = bool(out) and all(tr.get(db, t, kk) is not None and
                                all(tr.get(db, t, kk).get(col) == v for col, v in a.items()) for t, kk, a in out)
    lines.insert(0, f"{len(out)} rows")
    return out, lines, refusals, applied


SPEC2 = {"log": LOG2, "detail": DETAIL2, "needs": ("npcs", "npcaggro"), "changes": changes2}

if __name__ == "__main__":
    argv = sys.argv[1:]
    spec = SPEC
    if "--step" in argv:
        i = argv.index("--step")
        step = argv[i + 1] if i + 1 < len(argv) else ""
        if step not in ("1", "2"):
            sys.exit("--step takes 1 or 2")
        spec = {"1": SPEC, "2": SPEC2}[step]
        del argv[i:i + 2]
    sys.exit(tr.main(spec, argv=argv, doc=__doc__))
