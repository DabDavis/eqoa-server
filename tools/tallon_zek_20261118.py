#!/usr/bin/env python3
"""Tallon Zek's raid as eqoa.forumotion.com t218 tells it (the owner, 2026-11-18: "make the Tallon Zek raid work as
that post describes"), on top of the guide-gaps set `tallon` (tools/guidegaps/tallon.json, applied on both shards
2026-11-08) and quest 101327 "Tallon Zek". Every row found by name and place, each shard's own ids.

  The camp      "Kill the Zek Troops (con red@60) until you spawn a Zek Sergeant. Kill the Sergeant, and he drops one
                'Tallon's Key' No Trade No Rent LORE. At this point the portal spawns, it cons blank but you can talk
                to it." The four Zek troops 60 -> 63 (red to a 60), the Zek Sergeant 62 -> 64 and held out of the world
                (npcspawnonly) with the Portal of Tallon: a troop's death (npcdeath, 100) fires trigger "Zek Sergeant"
                (a free slot 290-299) where it fell: three more troop kills there (30 minutes, else "The Zek troops
                regroup."), the sergeant hostile for 30 minutes, his death standing the portal at its row's spot for
                15 minutes ("The energy of the portal is waning, use it quickly"), the run (and its lock: one sergeant
                at a time) over when the portal is gone. One key on his corpse (loot table "zek sergeant key", 100%,
                one roll), not one each: quest step 2 is a kill of him now, not a loot step (whose drop went to every
                player of the kill). The keys No Trade, No Rent (the key) and LORE, the key icon (-441710123, Heirloom's
                since the panel edit of 2026-11-18).
  The portal    internal/handlers/raidportal.go (this branch): anyone of 60 "use portal", then "Yes, I am the last" /
                "No, there are more that need to use it", sent to the plateau's port-in on either; the key's bearer
                alone closes it, on "Yes" (its "No" sends it nowhere: it goes last). Step 3 is a show of the key at the
                portal (the key kept; no teleport event, no reward: the portal sends, the ramp swaps the key), done by
                the bearer's "Yes"; a bearer whose steps 1-2 were not counted is put on step 3 there.
  The ramp      step 4 (reach): (4715, 185, 7500) radius 110, any height: the whole foot of the slope from x 4616 to
                4814 (worlds/ground/5.ground: the slope rises from the basin's 147 at z 7444 to the plateau's 218 at
                z 7572, x 4610-4820), the port-in (4750, 147.6, 7380) 125 off. The old point (4750, 160, 7460, 25) was
                missed by a player who ran up 96 to its side (Dust, 2026-11-18). It takes Tallon's Key and gives
                Tallon's Bent Key: "the person with the key will now get a popup that states you have given away the
                key, and received 'Tallon's Bent Key' in exchange, and that the fortress is now attackable"; its events
                as before (the fortress's and the post's triggers).
  The sentry    npcaggro: see_invis 1 ("He's sees through invis"), holds 1 (this branch: "cannot be pulled out of
                position"); its template a second breath, out of reach (range 80, in its sight: "he will not move nor
                cast if you are out of sight"). The fortress's run (trigger "Tallon's Fortress") stands two "Tallon's
                ward" blockers (CHAR.ESF 0x6F412342, size 3: a box 15 a side, collision only) on the fortress's one way
                in, the narrow ramp up from the plateau (x 4730-4736, z 7660-7690, 218 -> 232; a walk of the ground
                from the port-in reaches the first floor through it alone) with the sentry, and takes them away at its
                death ("the zek sentry step should place a bbox in front of the castle door until defeated", the owner).
                Its death puts every player of the kill on step 6 (raidportal.go's raidKillFlags).
  Tallon        his spawn stays the run (120 minutes) with an engage window of 20 (this branch's spawn `engage`): not
                fought for 20 minutes, or 20 after a fight ends alive, he leaves; never in a fight (the post's five).

Report only by default; --apply in one transaction, a gm_audit line a row, log tallon_zek_1118_log; --undo exact (the
npcaggro holds column it may add stays, as the server's start makes it). Characters on step 3 or 4 of 101327 at the
apply are refused (step 3's give and step 4's key change under them): finish or move them first.

    python3 -E -s tools/tallon_zek_20261118.py --db eqoa.db [--apply | --undo]
"""
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "tallon_zek_1118_log"
DETAIL = "Tallon Zek as t218 tells it (the owner, 2026-11-18): tools/tallon_zek_20261118.py"
SRC = "tallon zek 1118"

QUEST = 101327
KEY, BENT = "Tallon's Key", "Tallon's Bent Key"
KEY_ICON = -441710123
WARD = "Tallon's ward"
WARD_SPOTS = [(4733.0, 221.0, 7663.0), (4733.0, 228.0, 7678.0)]
WARD_SIZE = 3.0
BLOCKER = 0x6F412342
SERGEANT_SLOTS = range(290, 300)
SERGEANT_TRIGGER = "Zek Sergeant"
LOOT_TABLE = "zek sergeant key"
FORT = "Tallon's Fortress"

REACH = {"world": 5, "x": 4715.0, "y": 185.0, "z": 7500.0, "radius": 110.0, "height": 0.0}
POPUP = ("You have given away Tallon's Key, and received Tallon's Bent Key in exchange. "
         "The fortress is now attackable.")
BREATH = 905041
FAR_BREATH = {"kind": "aoe", "spell_id": BREATH, "every": 18, "first": 4, "out_of_reach": True, "range": 80}


def cj(v):
    return json.dumps(v, separators=(",", ":"))


def one(db, sql, args, what, refusals):
    r = tr.rows(db, sql, args)
    if len(r) != 1:
        refusals.append(f"{what}: {len(r)} rows, not one")
        return None
    return r[0]


def fortress_steps(steps, ward_ok):
    """Trigger 130's steps with the wards and Tallon's window, from the guide-gaps set's steps; None if not those."""
    out, seen = [], {"sentry": 0, "dead": 0, "tallon": 0}
    for s in steps:
        if s.get("do") == "spawn" and s.get("npc") == WARD:
            return None  # ours already: the caller compares
        out.append(dict(s))
        if s.get("do") == "spawn" and s.get("npc") == "Zek sentry":
            seen["sentry"] += 1
            for x, y, z in WARD_SPOTS:
                out.append({"do": "spawn", "npc": WARD, "spread": 0.01, "minutes": 120, "world": 5,
                            "x": x, "y": y, "z": z, "face": "north"})
        elif s.get("do") == "awaitdead" and s.get("npc") == "Zek sentry":
            seen["dead"] += 1
            out.append({"do": "despawn", "npc": WARD})
        elif s.get("do") == "spawn" and s.get("npc") == "Tallon Zek":
            seen["tallon"] += 1
            out[-1]["minutes"] = 120
            out[-1]["engage"] = 20
    if seen != {"sentry": 1, "dead": 1, "tallon": 1}:
        return None
    return out


def changes(db):
    out, lines, refusals = [], [], []
    q = one(db, "SELECT questid, name FROM quests WHERE questid = ?", (QUEST,), f"quest {QUEST}", refusals)
    if q and q["name"] != "Tallon Zek":
        refusals.append(f"quest {QUEST} is {q['name']!r}, not Tallon Zek")
    steps = {s["step"]: s for s in tr.rows(db, "SELECT * FROM queststeps WHERE questid = ?", (QUEST,))}
    key = one(db, "SELECT * FROM itempattern WHERE itemname = ?", (KEY,), KEY, refusals)
    bent = one(db, "SELECT * FROM itempattern WHERE itemname = ?", (BENT,), BENT, refusals)
    troops = tr.rows(db, "SELECT * FROM npcs WHERE npc_name = 'Zek troop' AND world = 0 "
                         "AND x_coord BETWEEN 20100 AND 20400 AND z_coord BETWEEN 18000 AND 18400")
    if len(troops) != 4:
        refusals.append(f"Zek troops at the camp: {len(troops)}, not 4")
    sgt = one(db, "SELECT * FROM npcs WHERE npc_name = 'Zek Sergeant' AND world = 0", (), "the Zek Sergeant", refusals)
    portal = one(db, "SELECT * FROM npcs WHERE npc_name = 'Portal of Tallon' AND world = 0", (), "the Portal of Tallon", refusals)
    sentry = one(db, "SELECT * FROM npcs WHERE npc_name = 'Zek sentry' AND world = 5", (), "the Zek sentry", refusals)
    fort = one(db, "SELECT * FROM gmtriggers WHERE name = ?", (FORT,), f"trigger {FORT!r}", refusals)
    tmpl = one(db, "SELECT name, abilities FROM classtemplates WHERE name = 'zek sentry'", (), "template zek sentry", refusals)
    aggro = one(db, "SELECT * FROM npcaggro WHERE npc_name = 'zek sentry'", (), "npcaggro zek sentry", refusals)
    wall = tr.rows(db, "SELECT * FROM npcs WHERE modelid = ? AND npc_name = 'invisible wall' ORDER BY id LIMIT 1", (BLOCKER,))
    if not wall:
        refusals.append("no 'invisible wall' row to copy a blocker from")
    if sorted(steps) != [1, 2, 3, 4, 5, 6]:
        refusals.append(f"quest {QUEST}'s steps are {sorted(steps)}, not 1-6")
    if refusals:
        return out, lines, refusals, False

    # characters a step's change would catch mid-way (the steps as found, not ours yet)
    mine = steps[4]["items"] == str(key["patternid"])
    if not mine:
        for r in tr.rows(db, "SELECT serverid, step FROM charquests WHERE questid = ? AND step IN (3, 4)", (QUEST,)):
            refusals.append(f"character {r['serverid']} is on step {r['step']} of {QUEST}: finish or move it first")

    # the items
    out.append(("itempattern", {"patternid": key["patternid"]}, {"trade": 1, "rent": 0, "lore": 1, "itemicon": KEY_ICON}))
    out.append(("itempattern", {"patternid": bent["patternid"]}, {"trade": 1, "lore": 1, "itemicon": KEY_ICON}))
    # the camp's levels
    for t in troops:
        out.append(("npcs", {"id": t["id"]}, {"npc_level": 63}))
    out.append(("npcs", {"id": sgt["id"]}, {"npc_level": 64}))
    # held out: the sergeant, the portal, the ward
    for name, note in (("zek sergeant", "Spawned by trigger 'Zek Sergeant' at the fourth troop's death (t218)."),
                       ("portal of tallon", "Spawned by trigger 'Zek Sergeant' at the sergeant's death, 15 minutes (t218)."),
                       ("tallon's ward", "The fortress door's blocker, stood by trigger 'Tallon's Fortress' while the sentry lives.")):
        out.append(("npcspawnonly", {"name": name}, {"name": name, "note": note, "source": SRC}))
    # the ward's row: an invisible wall's, on the fortress's ramp
    ward = tr.rows(db, "SELECT * FROM npcs WHERE npc_name = ? AND world = 5", (WARD,))
    if len(ward) > 1:
        refusals.append(f"{len(ward)} rows of {WARD!r}")
    wid = ward[0]["id"] if ward else db.execute("SELECT max(id) FROM npcs").fetchone()[0] + 1
    w = dict(wall[0])
    w.update({"id": wid, "npc_name": WARD, "world": 5, "zone": sentry["zone"], "x_coord": WARD_SPOTS[0][0],
              "y_coord": WARD_SPOTS[0][1], "z_coord": WARD_SPOTS[0][2], "facing": 0, "size": WARD_SIZE,
              "npc_level": 1, "npc_type": 128, "npc_id": 0})
    out.append(("npcs", {"id": wid}, w))
    # the sergeant's one key
    lt = tr.rows(db, "SELECT * FROM loottables WHERE tablename = ?", (LOOT_TABLE,))
    if len(lt) > 1:
        refusals.append(f"{len(lt)} rows of loot table {LOOT_TABLE!r}")
    ltid = lt[0]["id"] if lt else db.execute("SELECT coalesce(max(id), 0) FROM loottables").fetchone()[0] + 1
    out.append(("loottables", {"id": ltid}, {"id": ltid, "tablename": LOOT_TABLE, "patternid": key["patternid"],
                                             "weight": 1, "minlevel": 1, "maxlevel": 100, "source": SRC}))
    out.append(("npcloot", {"npc_name": "zek sergeant", "tablename": LOOT_TABLE},
                {"npc_name": "zek sergeant", "tablename": LOOT_TABLE, "chance": 100, "rolls": 1,
                 "note": "Tallon's Key: 'he drops one' (t218)", "source": SRC}))
    # the sergeant's trigger
    trig = tr.rows(db, "SELECT * FROM gmtriggers WHERE name = ?", (SERGEANT_TRIGGER,))
    if len(trig) > 1:
        refusals.append(f"{len(trig)} triggers named {SERGEANT_TRIGGER!r}")
    if trig:
        slot = trig[0]["slot"]
    else:
        used = {r[0] for r in db.execute("SELECT slot FROM gmtriggers")}
        free = [s for s in SERGEANT_SLOTS if s not in used]
        if not free:
            refusals.append(f"no free trigger slot in {SERGEANT_SLOTS}")
            return out, lines, refusals, False
        slot = free[0]
    psteps = [
        {"do": "lock", "lock": "zek sergeant"},
        {"do": "awaitkill", "npc": "Zek troop", "count": 3, "any": True, "seconds": 1800, "fail": "The Zek troops regroup."},
        {"do": "spawn", "npc": "Zek Sergeant", "hostile": True, "spread": 2, "minutes": 30},
        {"do": "broadcast", "text": "A Zek Sergeant storms out to rally his troops!"},
        {"do": "awaitdead", "npc": "Zek Sergeant", "any": True, "seconds": 1860, "fail": "The Zek Sergeant withdraws."},
        {"do": "spawn", "npc": "Portal of Tallon", "spread": 0.01, "minutes": 15, "world": 0,
         "x": portal["x_coord"], "y": portal["y_coord"], "z": portal["z_coord"], "face": "west"},
        {"do": "broadcast", "text": "A portal shimmers open by the Zek camp. Its energy is waning: use it quickly."},
        {"do": "awaitdead", "npc": "Portal of Tallon"},
    ]
    out.append(("gmtriggers", {"slot": slot}, {
        "slot": slot, "name": SERGEANT_TRIGGER, "enabled": 1, "scope": "near", "radius": 150.0, "world": -1,
        "x": 0.0, "y": 0.0, "z": 0.0, "steps": cj(psteps),
        "notes": "t218: 'Kill the Zek Troops until you spawn a Zek Sergeant. Kill the Sergeant, and he drops one Tallon's "
                 "Key ... At this point the portal spawns'. Fired where a troop falls (npcdeath); four kills, the "
                 "sergeant, then the Portal of Tallon for 15 minutes (raidportal.go: its key's bearer closes it). "
                 "tools/tallon_zek_20261118.py"}))
    out.append(("npcdeath", {"name": "zek troop", "trigger": SERGEANT_TRIGGER},
                {"name": "zek troop", "trigger": SERGEANT_TRIGGER, "chance": 100.0,
                 "note": "each troop's death; the run counts four and holds its lock (t218)", "source": SRC}))
    # the fortress's run
    fsteps = json.loads(fort["steps"])
    if any(s.get("npc") == WARD for s in fsteps):
        new = fsteps
    else:
        new = fortress_steps(fsteps, True)
        if new is None:
            refusals.append(f"trigger {FORT!r}'s steps are not the guide-gaps set's (one sentry spawn, its awaitdead, "
                            "one Tallon spawn)")
            new = fsteps
    out.append(("gmtriggers", {"slot": fort["slot"]}, {"steps": cj(new)}))
    # the sentry
    abil = json.loads(tmpl["abilities"] or "[]")
    if not any(a.get("out_of_reach") for a in abil):
        abil = abil + [FAR_BREATH]
    out.append(("classtemplates", {"name": "zek sentry"}, {"abilities": cj(abil)}))
    note = ("'He's sees through invis', 'cannot be pulled out of position', 'he will not move nor cast if you are out "
            "of sight' (t218)")
    out.append(("npcaggro", {"npc_name": "zek sentry"}, {"see_invis": 1, "holds": 1, "note": note}))
    # the quest's steps 2-4
    out.append(("queststeps", {"questid": QUEST, "step": 2}, {
        "kind": "kill", "items": "", "chance": 0.0, "count": 1,
        "journal": "Kill the Zek Sergeant; he carries Tallon's Key."}))
    out.append(("queststeps", {"questid": QUEST, "step": 3}, {
        "kind": "show", "items": str(key["patternid"]), "rewarditems": "", "choices": "", "events": "",
        "waiting": "The energy of the portal is waning, use it quickly.",
        "says": "The portal flares as Tallon's Key passes through, and closes behind you.",
        "journal": "Let the raid through the Portal of Tallon first, then go last with Tallon's Key: 'Yes, I am the last'."}))
    out.append(("queststeps", {"questid": QUEST, "step": 4}, dict(REACH, **{
        "items": str(key["patternid"]), "rewarditems": str(bent["patternid"]),
        "reminder": "You need Tallon's Key to wake the fortress.",
        "journal": "Run south up the mountain to the Zek Sentry's ramp with Tallon's Key.",
        "events": cj(step4_events(steps[4]["events"]))})))
    return out, lines, refusals, False


def step4_events(raw):
    evs = json.loads(raw) if raw.strip() else []
    for e in evs:
        if e.get("do") == "popup" and e.get("on") == "done":
            e["text"] = POPUP
    return evs


def is_applied(db, out):
    for tbl, key, after in out:
        have = tr.get(db, tbl, key) if tr.table(db, tbl) else None
        if have is None:
            return False
        for c, v in after.items():
            if c not in have or have[c] != v:
                return False
    return True


def all_changes(db):
    out, lines, refusals, _ = changes(db)
    if refusals:
        return out, lines, refusals, False
    if is_applied(db, out):
        return out, lines, [], True
    for tbl, key, after in out:
        have = tr.get(db, tbl, key) if tr.table(db, tbl) else None
        what = "insert" if have is None else "update " + ", ".join(
            f"{c} {have.get(c)!r} -> {v!r}" for c, v in after.items() if have.get(c) != v and c not in ("steps",))
        lines.append(f"{tbl} {key}: {what[:300]}")
    return out, lines, refusals, False


def prepare(db):
    if tr.column(db, "npcaggro", "holds"):
        return []
    db.execute("ALTER TABLE npcaggro ADD COLUMN holds INTEGER NOT NULL DEFAULT 0")
    return ["added npcaggro.holds (as the server's start does)"]


SPEC = {"log": LOG, "detail": DETAIL,
        "needs": ("npcs", "quests", "queststeps", "itempattern", "gmtriggers", "npcdeath", "npcspawnonly",
                  "loottables", "npcloot", "classtemplates", "npcaggro", "charquests"),
        "changes": all_changes, "prepare": prepare}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
