#!/usr/bin/env python3
"""The magician's level 60 pets as petition 1450 asks (the magician Manifest,
2026-11-10; the owner: "apply as he wrote"):

  "Djinni vizier should be scaled 30% larger. First ability should do 1k
  damage, 2nd ability 1.3k damage, 3rd ability aoe 1.7k/1.3k to target, 1.3k
  aoe to surrounding targets. 1st cast should be within 1st sec of engage, 2nd
  ability should follow 2 seconds later followed by the 3rd ability 2 seconds
  later. Badi should taunt and crit. Taunt should be every 15 secs. crit should
  be every 30 secs. Crit recommendation 2k. quad should be 500ish buffed with
  mage ability epic line. Ghilan should cast 2 CR abilities, one for 1k, the
  2nd for 1.3k every 30 seconds, 3 seconds spaced apart and have a base heal of
  800, cooldown 30 secs, main heal LoH style every 60 seconds for 1500. Efreeti
  lord should quad for 600 each hit, have a fire nuke for 2k every 30 seconds."

The four are workbook pets (petstats, entity/petstats.go), found by their
summoning spell's id and their row's name, each a kit of its own in place of
its offensive abilities (as petition 1388 gave the Swarming Deaths theirs); a
pet's buffs, which the petition does not name, are kept:

  - Djinni Vizier (Summon Djinni Vizier (60), 900761, workbook pet 003C, the
    air pet): its size 1.3, 30% over the 1 its body has now (no row size: its
    body's wearers', within a person's); Vizier's Bolt (1000), Vizier's
    Thunderclap (1300) and Vizier's Tempest (an area round its target of
    1700/1300: its target takes both, as the workbook's Air AoE does, everything
    within 15 of it 1300), lightning (LR), instant, at 1, 3 and 5 s into a
    fight and every 12 s after (OURS: the petition gives no recast; 12 s is the
    Air AoE's, and keeps the three 2 s apart each round), in place of Air Nuke
    and Air AoE Level 55 (its Air Root was taken off by the owner's word before
    this: a row still carrying it is taken too); Djinni's Grace kept.
  - Efreeti Lord (Summon Efreeti Lord, 701087, 003D): its weapons 988 and 988,
    so that each of its quad's hits is 600 on a creature of its owner's level
    (60), its main hand's and its off hand's alike (a melee pet's roll is its
    top there: offence.go); Efreeti's Inferno, 2000 fire (FR), a 1 s cast (Fire
    Strike B's) every 30 s from 1 s, in place of Fire Strike A and B Level 55;
    Efreeti's Fire kept; its size (1.75, petition 1447) left.
  - Badi Elder (Summon Badi Elder, 701086, 003B, the earth pet): its taunt (of
    hate alone, as before) every 15 s from 1 s; a crit, Critical Hit, of 2000
    (60 x 33.3333) every 30 s at its foe, the first 3 s in (OURS), the timed
    special of its template "pet badi elder" (classtemplates), which a workbook
    pet takes since this petition (entity/petstats.go petSheetTemplate); its
    weapons 881 and 881, so that its quad's hits come to about 500 with the
    magician's epic line on it (Magi`Kot's Sacrificial Breeding, the 60 epic's
    Mark path: its 135 proc on 12% of the hits, 16 a hit) on a creature of its
    owner's level, 484 a hit without; Earth Strike Level 55 taken off (the
    petition's Badi taunts and crits); Badi's Durability kept.
  - Ghilan Noble (Summon Ghilan Noble, 701088, 003E, the water pet): Ghilan's
    Lay Hands, 1500 at once on its owner or itself, the lower share of its hit
    points, below half (OURS: a lay on hands is for the worst), every 60 s;
    Ghilan's Mending, 800 on the same below 60% (the threshold its heal had),
    a 3 s cast (Water Heal Level 55's) every 30 s; Ghilan's Chill (1000) and
    Ghilan's Frost Wave (1300), cold (CR), 2 s casts (Water Nuke Level 55's) at
    5 and 8 s into a fight and every 30 s after; in place of Water Heal and
    Water Nuke Level 55; Ghilan's Blessing (on its owner) and Purifying Tide
    (petition 1395) kept.

Every figure lands as the pets' others do: a hit through the balance, whole on
a creature up to 4 levels over its owner (90/60/50/45% at 5/6/7/8+ over), less
a creature template's strong resist of its element; a heal as it is, never
over the most. The new spells are 905450-905457 (petition 1450's), refused if
any id or name is taken; their icons and visuals are those of the records
named below (LOOKS).

A row is changed only while it is what this expects (a GM's change is left and
nothing done, said); a row already as this leaves it is skipped. Everything
written is logged in mage_pets_1450_log (each row whole before and after, the
spells' id counter), with its gm_audit line as the pet import's (the panel
lists and reverts it); --undo puts back each row still as this left it (one
changed since is left, and said) and drops the log, the database then as it
was but for gm_audit's lines. Both take a copy of the database first
(--no-backup to skip). Report only by default.

WHEN IT TAKES EFFECT: petstats and classtemplates are read at a start (or a GM
panel edit of either), and a pet is made at its summoning: run it as the
before.sh of the keep restart that brings the mage-pets binary (an older one
leaves the Badi Elder's crit out); its pets out then are made again by their
spells. It runs on each shard's database.

    tools/fix_mage_pets_1450.py --db eqoa.db [--apply | --undo] [--no-backup]
"""
import argparse
import datetime
import json
import os
import shutil
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import import_eqoaemu_pets as pets  # noqa: E402  the pet import's gm_audit lines, one place

PETITION = 1450
LOG = "mage_pets_1450_log"
DETAIL = ("petition 1450 (Manifest): the magician's level 60 pets' kits, the Djinni Vizier 30% larger; "
          "tools/fix_mage_pets_1450.py")
NOTE = " Petition 1450 (Manifest, 2026-11-10): %s (tools/fix_mage_pets_1450.py)."
KEYS = {"spellpattern": ("spellid",), "spelleffect": ("spellid",), "petstats": ("spellid",), "classtemplates": ("name",),
        "sqlite_sequence": ("name",)}

VIZIER, EFREETI, BADI, GHILAN = 900761, 701087, 701086, 701088
SUMMONS = {VIZIER: ("Summon Djinni Vizier (60)", "003C", "Djinni Vizier"),
           EFREETI: ("Summon Efreeti Lord", "003D", "Efreeti Lord"),
           BADI: ("Summon Badi Elder", "003B", "Badi Elder"),
           GHILAN: ("Summon Ghilan Noble", "003E", "Ghilan Noble")}

BOLT, CLAP, TEMPEST, INFERNO, CHILL, FROST, MENDING, LAYHANDS = range(905450, 905458)


def ab(kind, every, first, spell=None, **kw):
    """An ability as the pet import writes one (its keys in that order)."""
    a = {"every": float(every), "first": float(first), "kind": kind}
    if spell is not None:
        a["spell_id"] = spell
    a.update(kw)
    return a


# Each pet's abilities as this expects them (the Vizier's either way: its Air
# Root on, as the import made it, or off, as the owner left it), and as it
# leaves them.
OLD = {
    VIZIER: [[ab("nuke", 6, 1, 902158), ab("nuke", 12, 3, 902159), ab("buff", 1800, 7, 902161, target="self")],
             [ab("nuke", 6, 1, 902158), ab("nuke", 12, 3, 902159), ab("root", 30, 5, 902160),
              ab("buff", 1800, 7, 902161, target="self")]],
    EFREETI: [[ab("nuke", 6, 1, 902162), ab("nuke", 12, 3, 902163), ab("buff", 1800, 5, 902164, target="self")]],
    BADI: [[ab("taunt", 12, 1, hits=[[1, 0, 0]]), ab("nuke", 12, 3, 902156), ab("buff", 1800, 5, 902157, target="self")]],
    GHILAN: [[ab("heal", 30, 1, 902165, target="either", below=60), ab("buff", 1800, 3, 902167, target="owner"),
              ab("nuke", 12, 5, 902166), ab("nuke", 30, 8, 905130)]],
}
NEW = {
    VIZIER: [ab("nuke", 12, 1, BOLT), ab("nuke", 12, 3, CLAP), ab("aoe", 12, 5, TEMPEST),
             ab("buff", 1800, 7, 902161, target="self")],
    EFREETI: [ab("nuke", 30, 1, INFERNO), ab("buff", 1800, 5, 902164, target="self")],
    BADI: [ab("taunt", 15, 1, hits=[[1, 0, 0]]), ab("buff", 1800, 5, 902157, target="self")],
    GHILAN: [ab("heal", 60, 1, LAYHANDS, target="either", below=50), ab("heal", 30, 1, MENDING, target="either", below=60),
             ab("buff", 1800, 3, 902167, target="owner"), ab("nuke", 30, 5, CHILL), ab("nuke", 30, 8, FROST),
             ab("nuke", 30, 8, 905130)],
}
# the spells the new kits keep, by name
KEPT = {902161: "Djinni's Grace", 902164: "Efreeti's Fire", 902157: "Badi's Durability", 902167: "Ghilan's Blessing",
        905130: "Purifying Tide"}
# the weapons (wpn1_dmg, wpn2_dmg) as this expects them and as it leaves them
WEAPONS = {EFREETI: ((660, 605), (988, 988)), BADI: ((495, 440), (881, 881))}
# the Vizier's size: 30% over its body's now
VIZIER_SCALE = 1.3

# The Badi Elder's crit: its template's timed special, 2000 at its level 60
TEMPLATE, TEMPLATE_SPELL = "pet badi elder", "summon badi elder"
CRIT = {"special": "Critical Hit", "special_damage": 33.3333, "special_every": 30.0, "special_first": 3.0}
CRIT_OFF = {"special": "", "special_chance": 0, "special_damage": 0.0, "special_every": 0.0, "special_first": 0.0,
            "special_jitter": 0.0, "special_from": 0, "special_fx": 0}
TEMPLATE_NOTE = (" Petition 1450 (Manifest, 2026-11-10): the Badi Elder's crit, Critical Hit, 2000 (33.3333 a level at "
                 "its 60) every 30 s at its foe, the first 3 s in, Sandstorm's creature special's visual; a workbook "
                 "pet takes its template's timed special since (tools/fix_mage_pets_1450.py).")

# The new spells: id, name, cast (s), recast (s), the line for people, the
# effects the server reads (spelleffect.effects, the workbook's form), the
# description, and the record whose icon, frame, range and visual it takes.


def harm(*parts, radius=0):
    """A harmful spell's effects: (kind, amount, resist) parts; round its target within radius when one is given."""
    d = {"v": 1, "scope": "target_aoe" if radius else "target", "target_type": "Enemy", "harmful": True, "stack": 0,
         "level": 60, "min_level": 1}
    if radius:
        d["radius"] = radius
    d["req_equipment"] = "Any"
    d["effects"] = [{"type": "Dmg/Heal", "kind": k, "amount": n, "resist": r} for k, n, r in parts]
    return d


def heal(n):
    return {"v": 1, "scope": "target", "target_type": "Owner/Self", "harmful": False, "stack": 0, "level": 60,
            "min_level": 1, "req_equipment": "Any", "effects": [{"type": "Dmg/Heal", "kind": "hp", "amount": n}]}


SPELLS = [
    (BOLT, "Vizier's Bolt", 0, 12, "Target, Decrease Hitpoints by 1000", harm(("hp", -1000, "lr")),
     "The Djinni Vizier's bolt of lightning: 1000 at its target. Resisted by LR.", 902158, "Air Nuke Level 55"),
    (CLAP, "Vizier's Thunderclap", 0, 12, "Target, Decrease Hitpoints by 1300", harm(("hp", -1300, "lr")),
     "The Djinni Vizier's thunderclap: 1300 at its target. Resisted by LR.", 700384, "Lightning Blast"),
    (TEMPEST, "Vizier's Tempest", 0, 12, "Target, Decrease Hitpoints by 1700 aoe Decrease Hitpoints by 1300",
     harm(("hp", -1700, "lr"), ("hp2", -1300, "lr"), radius=15),
     "The Djinni Vizier's tempest: 1700 and 1300 at its target, 1300 at those near it. Resisted by LR.", 902159,
     "Air AoE Level 55"),
    (INFERNO, "Efreeti's Inferno", 1, 30, "Target, Decrease Hitpoints by 2000", harm(("hp", -2000, "fr")),
     "The Efreeti Lord's inferno: 2000 at its target. Resisted by FR.", 700617, "Inferno"),
    (CHILL, "Ghilan's Chill", 2, 30, "Target, Decrease Hitpoints by 1000", harm(("hp", -1000, "cr")),
     "The Ghilan Noble's chill: 1000 at its target. Resisted by CR.", 902166, "Water Nuke Level 55"),
    (FROST, "Ghilan's Frost Wave", 2, 30, "Target, Decrease Hitpoints by 1300", harm(("hp", -1300, "cr")),
     "The Ghilan Noble's wave of frost: 1300 at its target. Resisted by CR.", 701146, "Ghilan Wash"),
    (MENDING, "Ghilan's Mending", 3, 30, "Target, Increase Hitpoints by 800", heal(800),
     "The Ghilan Noble's heal of its owner or itself: 800.", 902165, "Water Heal Level 55"),
    (LAYHANDS, "Ghilan's Lay Hands", 0, 60, "Target, Increase Hitpoints by 1500", heal(1500),
     "The Ghilan Noble's laying on of hands, on its owner or itself: 1500 at once, once a minute.", 700046, "Lay Hands"),
]
LOOKS = {s[0]: (s[7], s[8]) for s in SPELLS}

PET_NOTES = {
    VIZIER: "Vizier's Bolt (1000), Vizier's Thunderclap (1300) and Vizier's Tempest (1700/1300 round its target), "
            "lightning, instant, at 1, 3 and 5 s and every 12 s, in place of Air Nuke and Air AoE Level 55; size 1.3 "
            "(30% over its body's 1)",
    EFREETI: "Efreeti's Inferno (2000 fire every 30 s) in place of Fire Strike A and B Level 55; weapons 988/988 "
             "(660/605): its quad 600 a hit on a level 60",
    BADI: "its taunt every 15 s (12), its template's Critical Hit 2000 every 30 s, Earth Strike Level 55 off; weapons "
          "881/881 (495/440): its quad about 500 a hit with Magi`Kot's Sacrificial Breeding on it, 484 without, on a "
          "level 60",
    GHILAN: "Ghilan's Lay Hands (1500 every 60 s below 50%) and Ghilan's Mending (800 every 30 s below 60%) on its "
            "owner or itself, Ghilan's Chill (1000) and Ghilan's Frost Wave (1300), cold, every 30 s 3 s apart, in "
            "place of Water Heal and Water Nuke Level 55",
}

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,   -- spellpattern, spelleffect, petstats, classtemplates or sqlite_sequence
    rowkey TEXT NOT NULL,   -- the row's key, JSON, as gm_audit keys it
    before TEXT,            -- the whole row before, JSON; NULL: made here
    after  TEXT NOT NULL,   -- the whole row as written, JSON
    audit  INTEGER NOT NULL DEFAULT 0,  -- its gm_audit line (0: the id counter's, none)
    note   TEXT NOT NULL DEFAULT ''
)"""


def refuse(why):
    raise SystemExit(f"refused, nothing done: {why}")


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def row_of(db, tbl, key):
    where = " AND ".join(f"{c} = ?" for c in key)
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", list(key.values()))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def key_text(key):
    return json.dumps(key, sort_keys=True, separators=(",", ":"))


def dumps(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False)


def norm(v):
    """An ability (or a list of them) as the server reads it: numbers as floats, zero and empty fields left out (Go's
    omitempty), names in lower case."""
    if isinstance(v, bool):
        return v
    if isinstance(v, (int, float)):
        return float(v)
    if isinstance(v, str):
        return v.strip().lower()
    if isinstance(v, list):
        return [norm(x) for x in v]
    if isinstance(v, dict):
        return {k: norm(x) for k, x in v.items() if x not in (0, 0.0, "", None, False, [], {})}
    return v


def kit(text):
    try:
        return norm(json.loads(text or "[]"))
    except ValueError:
        return None


def body_size(db, model):
    """The size a workbook pet of no row size takes from its body (entity/petstats.go petBodyLocked): the commonest of
    its wearers' within a person's (0.75 to 1.5), the lowest id's on a tie; else 1."""
    sizes = {}
    if not has_table(db, "npcs"):
        return 1.0
    for i, s in db.execute("SELECT id, size FROM npcs WHERE modelid = ? ORDER BY id", (model,)):
        if s is not None and 0.75 <= s <= 1.5:
            n, first = sizes.get(s, (0, i))
            sizes[s] = (n + 1, first)
    if not sizes:
        return 1.0
    return float(sorted(sizes.items(), key=lambda kv: (-kv[1][0], kv[1][1]))[0][0])


def plan(db):
    """The rows to make and change: [(table, key, the row before or None, the row after, what)], and what is done
    already. Refuses (SystemExit) when anything is not what this expects."""
    for t in ("spellpattern", "spelleffect", "petstats", "classtemplates", "pettemplate"):
        if not has_table(db, t):
            refuse(f"no table {t}")
    cols = {r[1] for r in db.execute("PRAGMA table_info(classtemplates)")}
    if not set(CRIT_OFF) <= cols:
        refuse("classtemplates has no timed special's columns (a server from before them made this database)")
    if "effects" not in {r[1] for r in db.execute("PRAGMA table_info(spelleffect)")}:
        refuse("spelleffect has no effects column (the new spells' area and resists are in it)")
    changes, done = [], []
    # the summonings and their rows
    for sid, (spell, sheet, name) in SUMMONS.items():
        r = db.execute("SELECT spellname FROM spellpattern WHERE spellid = ?", (sid,)).fetchone()
        if r is None or r[0] != spell:
            refuse(f"spell {sid} is {r and r[0]!r}, not {spell!r}")
        row = row_of(db, "petstats", {"spellid": sid})
        if row is None:
            refuse(f"petstats has no row for {spell} ({sid})")
        if row["name"] != name or row["sheet_pet_id"] != sheet:
            refuse(f"petstats {sid} is {row['name']!r} (pet {row['sheet_pet_id']}), not {name!r} ({sheet})")
    # the spells the new kits keep, and the records the new ones look like
    for sid, name in sorted(KEPT.items()):
        r = db.execute("SELECT spellname FROM spellpattern WHERE spellid = ?", (sid,)).fetchone()
        if r is None or r[0] != name:
            refuse(f"spell {sid} is {r and r[0]!r}, not {name!r}, which a new kit keeps")
    # the new spells: made, or made already by this
    made = {}
    for sid, name, cast, recast, line, effects, desc, look, look_name in SPELLS:
        lk = db.execute("SELECT p.spellname, p.range, p.iconColor, p.icon, e.fxdict FROM spellpattern p "
                        "JOIN spelleffect e ON e.spellid = p.spellid WHERE p.spellid = ?", (look,)).fetchone()
        if lk is None or lk[0] != look_name:
            refuse(f"spell {look} is {lk and lk[0]!r}, not {look_name!r} (whose look {name} takes)")
        pattern = {"spellid": sid, "abilitylvl": 60, "unk2": 0, "unk3": 0, "range": lk[1], "casttime": cast, "power": 0,
                   "iconColor": lk[2], "icon": lk[3], "scope": 1, "recast": recast, "eqprequire": -1, "spellname": name,
                   "spelldesc": desc}
        effect = {"spellid": sid, "line": line, "fxdict": lk[4], "effects": dumps(effects),
                  "source": f"petition 1450 (Manifest); {look_name}'s look (tools/fix_mage_pets_1450.py)"}
        p0, e0 = row_of(db, "spellpattern", {"spellid": sid}), row_of(db, "spelleffect", {"spellid": sid})
        if p0 is not None or e0 is not None:
            if p0 == pattern and e0 is not None and all(e0.get(c) == v for c, v in effect.items()):
                done.append(f"spell {sid} {name}: made already")
                made[sid] = True
                continue
            refuse(f"spell id {sid} is taken ({p0 and p0['spellname']!r}): pick other ids")
        if has_table(db, "spellarea") and db.execute("SELECT 1 FROM spellarea WHERE spellid = ?", (sid,)).fetchone():
            refuse(f"spellarea has a row for spell {sid}")
        other = db.execute("SELECT spellid FROM spellpattern WHERE lower(trim(spellname)) = ?", (name.lower(),)).fetchone()
        if other:
            refuse(f"a spell named {name} is there already ({other[0]})")
        e_cols = [r[1] for r in db.execute("PRAGMA table_info(spelleffect)")]
        changes.append(("spellpattern", {"spellid": sid}, None, pattern, f"{name}: made"))
        changes.append(("spelleffect", {"spellid": sid}, None, {c: effect[c] for c in e_cols if c in effect},
                        f"{name}: {line}"))
    # the pets' rows
    for sid in (VIZIER, EFREETI, BADI, GHILAN):
        row = row_of(db, "petstats", {"spellid": sid})
        after = dict(row)
        now, want = kit(row["abilities"]), norm(NEW[sid])
        if now == want:
            done.append(f"petstats {sid} {row['name']}: its kit already")
        elif now in [norm(k) for k in OLD[sid]]:
            after["abilities"] = dumps(NEW[sid])
        else:
            refuse(f"petstats {sid} ({row['name']})'s abilities are {row['abilities']}, not what this expects "
                   f"(changed since: a GM's?)")
        if sid in WEAPONS:
            old, new = WEAPONS[sid]
            have = (row["wpn1_dmg"], row["wpn2_dmg"])
            if have == new:
                done.append(f"petstats {sid} {row['name']}: its weapons already")
            elif have == old:
                after["wpn1_dmg"], after["wpn2_dmg"] = new
            else:
                refuse(f"petstats {sid} ({row['name']})'s weapons are {have}, not {old}")
        if sid == VIZIER:
            want_size = round(VIZIER_SCALE * body_size(db, row["model"]), 4)
            if abs(row["size"] - want_size) < 1e-9:
                done.append(f"petstats {sid} {row['name']}: its size {want_size} already")
            elif row["size"] == 0:
                after["size"] = want_size
            else:
                refuse(f"petstats {sid} ({row['name']})'s size is {row['size']}, not 0 (its body's)")
        if after != row:
            after["note"] = row["note"] + NOTE % PET_NOTES[sid]
            changes.append(("petstats", {"spellid": sid}, row, after, f"{row['name']}: {PET_NOTES[sid]}"))
    # the Badi Elder's crit: its template row's timed special
    pt = db.execute("SELECT template FROM pettemplate WHERE spell = ? AND weapon = 0", (TEMPLATE_SPELL,)).fetchone()
    if pt is None or pt[0] != TEMPLATE:
        refuse(f"pettemplate {TEMPLATE_SPELL!r} is {pt and pt[0]!r}, not {TEMPLATE!r}: its crit would not reach it")
    others = [s for (s,) in db.execute("SELECT spell FROM pettemplate WHERE template = ? AND spell != ?",
                                       (TEMPLATE, TEMPLATE_SPELL))]
    if others:
        refuse(f"{TEMPLATE!r} is the template of {others} too, whose pets its crit would reach")
    t = row_of(db, "classtemplates", {"name": TEMPLATE})
    if t is None:
        refuse(f"classtemplates has no {TEMPLATE!r}")
    want = dict(CRIT_OFF, **CRIT)
    if all(t[c] == v for c, v in want.items()):
        done.append(f"classtemplates {TEMPLATE}: its crit already")
    elif all(t[c] == v for c, v in CRIT_OFF.items()):
        after = dict(t, **CRIT)
        after["note"] = t["note"] + TEMPLATE_NOTE
        changes.append(("classtemplates", {"name": TEMPLATE}, t, after, "the Badi Elder's crit, 2000 every 30 s"))
    else:
        refuse(f"classtemplates {TEMPLATE!r} has a special already ({ {c: t[c] for c in CRIT_OFF} }): a GM's?")
    return changes, done


def other_crits(db):
    """The summonings of a workbook pet whose pet template has a timed special already (not the Badi Elder's): since
    this petition's code their pets crit by it too, which the report says."""
    q = ("SELECT pt.spell, c.name, c.special_every, c.special_damage FROM pettemplate pt JOIN classtemplates c ON c.name = "
         "pt.template WHERE c.special_every > 0 AND c.special_damage > 0 AND c.name != ? AND EXISTS (SELECT 1 FROM petstats "
         "s JOIN spellpattern p ON p.spellid = s.spellid WHERE lower(trim(p.spellname)) = pt.spell) ORDER BY pt.spell")
    return db.execute(q, (TEMPLATE,)).fetchall()


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-magepets1450"
    shutil.copy2(path, dst)
    return dst


def write_row(db, tbl, key, row, insert=False):
    where = " AND ".join(f"{c} = ?" for c in key)
    if row is None:
        db.execute(f"DELETE FROM {tbl} WHERE {where}", list(key.values()))
    elif insert:
        cols = list(row)
        db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [row[c] for c in cols])
    else:
        cols = [c for c in row if c not in key]
        db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in cols)} WHERE {where}",
                   [row[c] for c in cols] + list(key.values()))


def seq(db):
    if not has_table(db, "sqlite_sequence"):
        return None
    r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'spellpattern'").fetchone()
    return None if r is None else r[0]


def apply(db, changes):
    """The plan written in one transaction, each row logged with its gm_audit line; returns how many rows."""
    order = ["spellpattern", "spelleffect", "petstats", "classtemplates"]
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_SCHEMA)
        db.execute(pets.AUDIT_SCHEMA)
        before = seq(db)
        for tbl, key, b, a, why in sorted(changes, key=lambda c: order.index(c[0])):
            now = row_of(db, tbl, key)
            if now != b:
                raise RuntimeError(f"{tbl} {key_text(key)} changed under us")
            write_row(db, tbl, key, a, insert=b is None)
            written = row_of(db, tbl, key)
            line = pets.audit(db, "insert" if b is None else "update", tbl, key, b, written, DETAIL)
            db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit, note) VALUES (?, ?, ?, ?, ?, ?)",
                       (tbl, key_text(key), None if b is None else json.dumps(b, sort_keys=True),
                        json.dumps(written, sort_keys=True), line, why))
        after = seq(db)
        if after != before:
            # the spells' id counter (spellpattern is AUTOINCREMENT): put back by --undo, as the rows are
            was = None if before is None else {"name": "spellpattern", "seq": before}
            db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, note) VALUES (?, ?, ?, ?, ?)",
                       ("sqlite_sequence", key_text({"name": "spellpattern"}),
                        None if was is None else json.dumps(was, sort_keys=True),
                        json.dumps({"name": "spellpattern", "seq": after}, sort_keys=True), "the spells' id counter"))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return len(changes)


def undo(db):
    """Each logged row still as this left it put back, the last first; returns (back, left)."""
    if not has_table(db, LOG):
        return 0, 0
    back = left = 0
    db.execute("BEGIN IMMEDIATE")
    try:
        for s, tbl, rk, before, after in db.execute(
                f"SELECT seq, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
            if tbl not in KEYS:
                raise SystemExit(f"{LOG} names table {tbl!r}, which this tool never writes")
            key = json.loads(rk)
            now = row_of(db, tbl, key)
            if now != json.loads(after):
                print(f"  left: {tbl} {rk}, {'gone' if now is None else 'changed since it was written'}")
                left += 1
                continue
            b = None if before is None else json.loads(before)
            write_row(db, tbl, key, b)  # a row made here deleted, a row changed here put back
            if tbl != "sqlite_sequence":
                pets.audit(db, "delete" if b is None else "update", tbl, key, now, row_of(db, tbl, key), "undo: " + DETAIL)
            db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (s,))
            back += 1
        if not db.execute(f"SELECT 1 FROM {LOG} LIMIT 1").fetchone():
            db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return back, left


def describe(change):
    tbl, key, before, after, why = change
    if before is None:
        return f"{PETITION} {tbl} {key_text(key)}: made {dumps(after)[:240]}"
    diff = "; ".join(f"{c} {str(before[c])[-70:]!r} -> {str(after[c])[-200:]!r}" for c in after
                     if after[c] != before[c] and c != "note")
    return f"{PETITION} {tbl} {key_text(key)} ({why.split(':')[0]}): {diff}"


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the changes (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply wrote")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    try:
        if a.undo:
            if not has_table(db, LOG):
                print("nothing logged: nothing to undo")
                return
            if not a.no_backup:
                print(f"copy: {backup(a.db)}")
            back, left = undo(db)
            print(f"put back {back} rows, {left} left (changed since); restart the server to read them")
            return
        changes, done = plan(db)
        for c in changes:
            print(describe(c))
        for d in done:
            print(f"done already: {d}")
        for spell, tmpl, every, dmg in other_crits(db):
            print(f"note: {spell!r}'s workbook pet crits by its template {tmpl!r} with this code too ({dmg} a level every "
                  f"{every} s): a GM's special, left")
        print(f"{len(changes)} rows to write, {len(done)} done already")
        if not a.apply:
            print("report only: --apply to write")
            return
        if not changes:
            return
        if not a.no_backup:
            print(f"copy: {backup(a.db)}")
        n = apply(db, changes)
        print(f"written {n} rows, logged in {LOG}; restart the server (keep restart) to read them")
    finally:
        db.close()


if __name__ == "__main__":
    main()
