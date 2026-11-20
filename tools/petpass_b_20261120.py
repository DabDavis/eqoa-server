#!/usr/bin/env python3
"""The second lane of the 2026-11-20 petition pass: data fixes, each part with its petition(s) and the shard(s) it is
for (--shard picks them; every row is found by NAME and place, never by the other shard's id).

  focus     (both; Heirloom petition 2480: "Focus cm line should open on 100 wis for tank and healer classes, int for
            melee/caster. looks like its int for everyone"). The five Focus ranks need INT 100 of every class; their
            minstats become "INT:100;WIS:100@909": WIS 100 for the warrior, paladin, shadow knight, druid, shaman and
            cleric (the classes column's bits 0, 2, 3, 7, 8, 9 = 909), INT 100 for the rest. NEEDS THE SERVER WITH
            THE CLASS GROUPS OF MINSTATS (entity/cmbuy.go parseMinStats, branch petpass-20261120b): an older one reads
            the new text as no requirement at all. cmcatalog is read at the start.
  looks     (both; Heirloom petitions 2483, 2485, 2486, Live 5359, 5351; Heirloom 2484). Each look copied from a
            record that wears it already (rendered with ITEMICON.ESF / ITEM.ESF first):
              2483 Overlord Frank's ring: the icon of an iron bar (shared by 132 bars, dusts and 7 rings) -> the
                   gold ring of 652 rings ("wooden ring"'s)
              2485 Xannasu's branch: a quarterstaff's icon -> the branch of "Brittle Treant Branch"
              2486 Pristine Creature Hide: a coil of rope's icon (shared by "Creature Hide" and 19 others) -> the
                   pelt of 91 pelts ("Bat Pelt"'s)
              5359 Jet Spear: the harpoon's icon -> the spear of the records sharing its spear model ("Centaurian
                   Battle Spear"'s); the model (0xFF96D99F, 23 spears) is kept: it draws on both discs' ITEM.ESF
              5351 Grimblade (Live only): a scimitar's model with a gladius's icon -> the broadsword model and icon
                   a GM gave it on Heirloom on 2026-11-16 ("Sword of Dreams"'s, a one-hand sword too)
              2484 lava rock shield and Ascended lava rock shield: the warrior's bit added to their classes (524 ->
                   525: warrior, paladin, shadow knight, cleric)
            Item records: new copies at once, held ones at the holder's next login.
  xelebb    (both; Heirloom petition 2481: "Inspector Xeleb is multispawning. And attacking before you can even speak
            to him ... Multi spawns different levels of temple"). Two floating eyes stood in Cazic Thule's pyramid:
            Inspector Xelebb (L50, the base, (14644.3, 0, 30412.8): quest 100948's, "Speak with Inspector Xelebb at the
            base of the pyramid") and Inspector Xalebb (L48, one floor up, (14673.9, 12, 30444.9): no quest names it),
            and an eye's body attacks on sight. Xalebb's row and its respawn row go; Xelebb gets an npcaggro row
            (aggressive 0: he fights when attacked, so he can be spoken to first). npcs and npcaggro at the start.
  tarantula (both; Live petition 5352: "lvl 17 in lvl 1-5 area"). Four tarantulas of 16-17 stood at (4655, 21075),
            among the ants, flies and snakes of 1-3 outside Qeynos; the area's other tarantulas (and the level 3 quests
            "Tarantula Leg Tips" that want them) are level 3: these four go to 3. npcs at the start.
  mystic    (both; Live petition 5355: "under ground"). The Highpass pickclaw mystic at (17107.5, 138.9, 15217.5) is in
            the rock (the ground package: no floor under it, rock over it; the floor over it at 142.14, its camp's at
            144.5-145.4): set on that floor. npcs at the start.
  gurig     (both; Live petition 5354: "inside HP"). Gurig (L23, a pickclaw's body) wanders (npcmove "walks on film")
            within the server's 70 of his spawn among Highpass's stalls (seen at (16953.7, 15254.6), in the merchants'
            hall); his row wanders within 12 of his spawn (npcwander). Read at the start.
  bandits   (Live; a GM's Live petition 5360, approved as asked: "Unless this bandit is a quest npc, it should be KoS").
            The one "bandits" row (L25, (12490.6, 17189.0), the Alseop's Wall bandit camp) is in no quest; "bandit"
            attacks on sight by an npcaggro row (petition 1472) and "bandits" had none: one as "bandit"'s. npcaggro at
            the start.
  taereg    (Live; a GM's Live petition 5362, approved as asked: "Taereg bandits should be KoS"). Every Taereg name
            (bandit, bandit sorcerer, brigand, cutthroat, jewel thief, lookout, thug: a players' race's body, so they
            attacked no one) gets an npcaggro row, aggressive. No quest talks to one (two loot a jewel thief).
  shaman    (Live; Live petition 5356: "doesnt use sha abilities"). A pickclaw shaman (npcclass 8) cast nothing: no
            template named it. Class template "shaman" (class -1, its own: npcclass's shaman), from the GM's seed
            templates' numbers: a nuke of its level every 12 s (35% of its figures, recast 10 s and cast 5 s at most),
            a damage over time of its level every 24 s (35%), and a heal of 15% of its own hit points (no spell) every
            15 s below 50%. Given to "pickclaw shaman" and "adept pickclaw shaman" (Highpass's); --every-shaman also
            to every other name of npcclass 8 with no template (a balance choice: the owner's). Read at the start or a
            panel edit of classtemplates/npctemplate.
  mapmark   (Heirloom's database, which serves the community map marks to every vanilla client of both shards; Live
            petition 5357: "Level 61ish Camp welahi-nai camp needs marked on map"). A camp mark at the Welahi-nai
            village in Odus (world 2), the middle of its 59-61 rows: myrmidons, priestesses, mages, amirs, puzhals,
            fishermen. As a GM's accept in the panel writes one; the list is rebuilt within a minute.

Report only by default; --apply in one transaction, a gm_audit line a row, log petpass_b_1120_log; --undo exact.

    python3 -E -s tools/petpass_b_20261120.py --db eqoa.db --shard live|heirloom [--parts a,b] [--every-shaman]
                                              [--apply | --undo]
"""
import datetime
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_b_1120_log"
DETAIL = "Petition pass 2026-11-20, second lane (2480-2486, 5351-5362): tools/petpass_b_20261120.py"
SOURCE = "petpass-1120b"

BOTH, LIVE, HEIRLOOM = ("live", "heirloom"), ("live",), ("heirloom",)
PARTS = {"focus": BOTH, "looks": BOTH, "xelebb": BOTH, "tarantula": BOTH, "mystic": BOTH, "gurig": BOTH,
         "bandits": LIVE, "taereg": LIVE, "shaman": LIVE, "mapmark": HEIRLOOM}

FOCUS_FROM, FOCUS_TO = "INT:100", "INT:100;WIS:100@909"

# petition, record, the record whose look it copies, its columns; shards
LOOKS = (
    (2483, "overlord frank's ring", "wooden ring", ("itemicon",), BOTH),
    (2485, "xannasu's branch", "brittle treant branch", ("itemicon",), BOTH),
    (2486, "pristine creature hide", "bat pelt", ("itemicon",), BOTH),
    (5359, "jet spear", "centaurian battle spear", ("itemicon",), BOTH),
    (5351, "grimblade", "sword of dreams", ("model", "itemicon"), LIVE),
)
WARRIOR_BIT = 1
SHIELDS = ("lava rock shield", "ascended lava rock shield")

XALEBB = ("inspector xalebb", 0, 14673.9, 30444.9)
XELEBB = "inspector xelebb"
TARANTULAS = (4655.0, 21075.0, 30.0, 3)  # x, z, within, the level they take
MYSTIC = ("pickclaw mystic", 0, 17107.5, 15217.5, 138.9, 142.15)
GURIG = ("gurig", 0, 16988.9, 15230.9, 12.0)
BANDITS = "bandits"
TAEREG = ("taereg bandit", "taereg bandit sorcerer", "taereg brigand", "taereg cutthroat", "taereg jewel thief",
          "taereg lookout", "taereg thug")
SHAMAN_NAMES = ("pickclaw shaman", "adept pickclaw shaman")
SHAMAN = [
    {"kind": "nuke", "every": 12, "first": 3, "damage_pct": 35, "max_recast": 10, "max_cast": 5},
    {"kind": "dot", "every": 24, "first": 6, "damage_pct": 35},
    {"kind": "heal", "every": 15, "first": 2, "heal_pct": 15, "below": 50},
]
MARK = {"world": 2, "x": 4720.0, "z": 6760.0, "name": "Welahi-nai (59-61)",
        "note": "Welahi-nai village: myrmidons, priestesses, mages, amirs, puzhals and fishermen, 59-61",
        "kind": "camp"}

ARGS = {"shard": None, "parts": None, "every_shaman": False}


def near(r, x, z, d=1.0):
    return abs(r["x_coord"] - x) < d and abs(r["z_coord"] - z) < d


def one(db, sql, args, what, refusals):
    rows = tr.rows(db, sql, args)
    if len(rows) != 1:
        refusals.append(f"{len(rows)} rows of {what}, want one")
        return None
    return rows[0]


def item(db, name, refusals):
    return one(db, "SELECT * FROM itempattern WHERE lower(itemname) = ?", (name,), f"item {name!r}", refusals)


def part_focus(db, out, lines, refusals):
    rows = tr.rows(db, "SELECT * FROM cmcatalog WHERE mastery = 'Focus' AND name LIKE 'Focus %' ORDER BY rank")
    if len(rows) != 5:
        refusals.append(f"focus: {len(rows)} Focus ranks, want 5")
        return
    for r in rows:
        if r["minstats"] not in (FOCUS_FROM, FOCUS_TO):
            refusals.append(f"focus: {r['name']} (id {r['id']}) needs {r['minstats']!r}, not {FOCUS_FROM!r}")
            continue
        out.append(("cmcatalog", {"id": r["id"]}, {"minstats": FOCUS_TO}))
    lines.append(f"2480 focus: ids {[r['id'] for r in rows]} minstats {FOCUS_FROM!r} -> {FOCUS_TO!r}")


def part_looks(db, out, lines, refusals, shard):
    for pet, name, src, cols, shards in LOOKS:
        if shard not in shards:
            continue
        r, s = item(db, name, refusals), item(db, src, refusals)
        if r is None or s is None:
            continue
        after = {c: s[c] for c in cols}
        out.append(("itempattern", {"patternid": r["patternid"]}, after))
        lines.append(f"{pet} {r['itemname']} ({r['patternid']}): " +
                     ", ".join(f"{c} {r[c]} -> {s[c]}" for c in cols) + f" (as {s['itemname']})")
    for name in SHIELDS:
        r = item(db, name, refusals)
        if r is None:
            continue
        out.append(("itempattern", {"patternid": r["patternid"]}, {"classuse": r["classuse"] | WARRIOR_BIT}))
        lines.append(f"2484 {r['itemname']} ({r['patternid']}): classuse {r['classuse']} -> {r['classuse'] | WARRIOR_BIT}")


def part_xelebb(db, out, lines, refusals):
    name, world, x, z = XALEBB
    if one(db, "SELECT id FROM npcs WHERE lower(npc_name) = ?", (XELEBB,), XELEBB, refusals) is None:
        return
    rows = [r for r in tr.rows(db, "SELECT * FROM npcs WHERE lower(npc_name) = ? AND world = ?", (name, world))
            if near(r, x, z)]
    if len(rows) > 1:
        refusals.append(f"xelebb: {len(rows)} rows of {name}")
        return
    for r in rows:
        out.append(("npcs", {"id": r["id"]}, None))
        if tr.get(db, "npcrespawn", {"npc_row": r["id"]}) is not None:
            out.append(("npcrespawn", {"npc_row": r["id"]}, None))
        lines.append(f"2481 {r['npc_name']} row {r['id']} (L{r['npc_level']}, y {r['y_coord']}) and its respawn row: deleted")
    if not rows:
        lines.append(f"2481 no {name} row at ({x}, {z}): gone already")
    have = tr.get(db, "npcaggro", {"npc_name": XELEBB})
    if have is not None and have.get("source") != SOURCE:
        refusals.append(f"xelebb: npcaggro {XELEBB!r} is there already ({have})")
        return
    out.append(("npcaggro", {"npc_name": XELEBB}, {"npc_name": XELEBB, "aggressive": 0, "radius": 0,
                "note": "petition 2481: the quest's eye is spoken to first (quest 100948 step 2), then fought",
                "source": SOURCE}))
    lines.append(f"2481 npcaggro {XELEBB!r}: aggressive 0")


def part_tarantula(db, out, lines, refusals):
    x, z, d, level = TARANTULAS
    rows = [r for r in tr.rows(db, "SELECT * FROM npcs WHERE lower(npc_name) = 'tarantula' AND world = 0")
            if near(r, x, z, d)]
    if len(rows) != 4 or any(r["npc_level"] not in (16, 17, level) for r in rows):
        refusals.append(f"tarantula: {[(r['id'], r['npc_level']) for r in rows]} near ({x}, {z}), want four of 16-17")
        return
    for r in rows:
        out.append(("npcs", {"id": r["id"]}, {"npc_level": level}))
    lines.append(f"5352 tarantula rows {[r['id'] for r in rows]}: levels {[r['npc_level'] for r in rows]} -> {level}")


def part_mystic(db, out, lines, refusals):
    name, world, x, z, was, y = MYSTIC
    rows = [r for r in tr.rows(db, "SELECT * FROM npcs WHERE lower(npc_name) = ? AND world = ?", (name, world))
            if near(r, x, z) and abs(r["y_coord"] - was) < 0.5 or near(r, x, z) and abs(r["y_coord"] - y) < 0.01]
    if len(rows) != 1:
        refusals.append(f"mystic: {len(rows)} rows of {name} at ({x}, {was}, {z})")
        return
    out.append(("npcs", {"id": rows[0]["id"]}, {"y_coord": y}))
    lines.append(f"5355 {name} row {rows[0]['id']}: y {rows[0]['y_coord']} -> {y}")


def part_gurig(db, out, lines, refusals):
    name, world, x, z, radius = GURIG
    rows = [r for r in tr.rows(db, "SELECT * FROM npcs WHERE lower(npc_name) = ? AND world = ?", (name, world))
            if near(r, x, z)]
    if len(rows) != 1:
        refusals.append(f"gurig: {len(rows)} rows of {name} at ({x}, {z})")
        return
    rid = rows[0]["id"]
    have = tr.get(db, "npcwander", {"npcid": rid})
    if have is not None and have.get("source") != SOURCE:
        refusals.append(f"gurig: npcwander row {rid} is there already ({have})")
        return
    out.append(("npcwander", {"npcid": rid}, {"npcid": rid, "radius": radius, "note":
                "petition 5354: he wandered into Highpass's stalls; within 12 of his spawn", "source": SOURCE}))
    lines.append(f"5354 {name} row {rid}: wanders within {radius}")


def aggro_row(name, note):
    return ("npcaggro", {"npc_name": name}, {"npc_name": name, "aggressive": 1, "radius": 0, "note": note,
                                             "source": SOURCE})


def part_aggro(db, out, lines, refusals, names, pet, note):
    for name in names:
        n = tr.rows(db, "SELECT count(*) AS c, sum(npc_type & 128 <> 0) AS f FROM npcs WHERE lower(npc_name) = ?",
                    (name,))[0]
        if not n["c"]:
            refusals.append(f"{pet}: no {name!r} rows")
            continue
        talks = tr.rows(db, "SELECT questid, step FROM queststeps WHERE lower(target) = ? AND kind <> 'loot' "
                            "AND kind <> 'kill'", (name,))
        if talks:
            refusals.append(f"{pet}: {name!r} is a quest's ({talks}): left for a GM")
            continue
        have = tr.get(db, "npcaggro", {"npc_name": name})
        if have is not None and have.get("source") != SOURCE:
            refusals.append(f"{pet}: npcaggro {name!r} is there already ({have})")
            continue
        out.append(aggro_row(name, note))
        lines.append(f"{pet} npcaggro {name!r} ({n['c']} rows): aggressive 1")


def part_shaman(db, out, lines, refusals):
    want = {"name": "shaman", "class": -1, "hp_pct": 100, "taken_pct": 100, "melee_pct": 100,
            "abilities": json.dumps(SHAMAN, separators=(",", ":")),
            "note": "petition 5356: a shaman casts a nuke and a damage over time of its level and heals itself: "
                    "tools/petpass_b_20261120.py", "source": SOURCE}
    have = tr.get(db, "classtemplates", {"name": "shaman"})
    if have is not None and have.get("source") != SOURCE:
        refusals.append("shaman: classtemplates 'shaman' is there already and not this tool's")
        return
    out.append(("classtemplates", {"name": "shaman"}, want))
    lines.append(f"5356 classtemplates 'shaman': {want['abilities']}")
    names = list(SHAMAN_NAMES)
    if ARGS["every_shaman"]:
        names += [r["name"] for r in tr.rows(
            db, "SELECT c.name FROM npcclass c WHERE c.class = 8 AND c.name NOT IN (SELECT name FROM npctemplate) "
                "AND EXISTS (SELECT 1 FROM npcs n WHERE lower(trim(n.npc_name)) = c.name AND n.npc_type & 128 = 0) "
                "ORDER BY 1") if r["name"] not in names]
    for n in names:
        cls = tr.get(db, "npcclass", {"name": n})
        if cls is None or cls["class"] != 8:
            refusals.append(f"shaman: {n!r} is not of npcclass 8 ({cls})")
            continue
        t = tr.get(db, "npctemplate", {"name": n})
        if t is not None and t["template"] != "shaman":
            refusals.append(f"shaman: {n!r} fights by {t['template']!r}: left for a GM")
            continue
        out.append(("npctemplate", {"name": n}, {"name": n, "template": "shaman", "note": "petition 5356",
                                                 "source": SOURCE}))
    lines.append(f"5356 npctemplate -> 'shaman': {len(names)} names: {', '.join(names)}")


def part_mapmark(db, out, lines, refusals):
    have = tr.rows(db, "SELECT * FROM mapmarks WHERE world = ? AND lower(name) LIKE 'welahi%'", (MARK["world"],))
    mine = [r for r in have if r["source"] == SOURCE]
    if have and not mine:
        refusals.append(f"mapmark: a Welahi-nai mark is there already ({have})")
        return
    if mine:
        out.append(("mapmarks", {"id": mine[0]["id"]}, dict(MARK, source=SOURCE)))
        return
    nid = tr.rows(db, "SELECT coalesce(max(id), 0) + 1 AS n FROM mapmarks")[0]["n"]
    now = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    out.append(("mapmarks", {"id": nid}, dict(MARK, id=nid, source=SOURCE, suggestion_id=0, created=now, updated=now)))
    lines.append(f"5357 mapmarks {nid}: {MARK['name']!r} at world {MARK['world']} ({MARK['x']}, {MARK['z']}), "
                 f"kind {MARK['kind']}")


def changes(db):
    out, lines, refusals = [], [], []
    shard = ARGS["shard"]
    for part, shards in PARTS.items():
        if shard not in shards or ARGS["parts"] and part not in ARGS["parts"]:
            continue
        if part == "focus":
            part_focus(db, out, lines, refusals)
        elif part == "looks":
            part_looks(db, out, lines, refusals, shard)
        elif part == "xelebb":
            part_xelebb(db, out, lines, refusals)
        elif part == "tarantula":
            part_tarantula(db, out, lines, refusals)
        elif part == "mystic":
            part_mystic(db, out, lines, refusals)
        elif part == "gurig":
            part_gurig(db, out, lines, refusals)
        elif part == "bandits":
            part_aggro(db, out, lines, refusals, (BANDITS,), 5360,
                       "petition 5360 (a GM): bandits are kill on sight, as bandit (petition 1472)")
        elif part == "taereg":
            part_aggro(db, out, lines, refusals, TAEREG, 5362, "petition 5362 (a GM): the Taereg are kill on sight")
        elif part == "shaman":
            part_shaman(db, out, lines, refusals)
        elif part == "mapmark":
            part_mapmark(db, out, lines, refusals)
    applied = bool(out) and all(
        (tr.get(db, t, k) is None) if a is None else
        (tr.get(db, t, k) is not None and all(tr.get(db, t, k).get(c) == v for c, v in a.items()
                                              if c not in ("created", "updated")))
        for t, k, a in out)
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("npcs", "itempattern", "cmcatalog", "npcaggro"), "changes": changes}


def main(argv):
    rest, i = [], 0
    while i < len(argv):
        a = argv[i]
        if a == "--shard" and i + 1 < len(argv):
            ARGS["shard"] = argv[i + 1]
            i += 2
            continue
        if a == "--parts" and i + 1 < len(argv):
            ARGS["parts"] = set(argv[i + 1].split(","))
            i += 2
            continue
        if a == "--every-shaman":
            ARGS["every_shaman"] = True
            i += 1
            continue
        rest.append(a)
        i += 1
    if ARGS["shard"] not in ("live", "heirloom") and "--undo" not in rest:
        print("--shard live or heirloom")
        return 2
    if ARGS["parts"] and not ARGS["parts"] <= set(PARTS):
        print(f"parts: {', '.join(PARTS)}")
        return 2
    return tr.main(SPEC, rest, doc=__doc__)


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
