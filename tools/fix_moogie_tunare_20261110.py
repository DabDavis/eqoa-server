#!/usr/bin/env python3
"""Mr. Moogie's, Tunare's, Innoruuk's and Metal Moogie V2's data of 2026-11-10: the GM Jibrish's petitions 1437-1443
and the owner's asks of the day, on the server's code of the same branch (the template ability kinds and fields it
writes are that code's: an older server reads these templates as unreadable and leaves them out).

  classtemplates "mr moogie" (petitions 1437-1443), its abilities from the two this tool finds (Royal Decree, his Lay
    Hands of an hour) to:
      Royal Decree as it is;
      Lay Hands every and cooldown 900 s (from 3600), its proc after landing 100% for 350 (from 25% for 1500), Royal
        Fury still, its line "Thats a fifteen minute cool down, you'll pay for that" (petition 1437);
      Royal Siphon, a lifetap of a fixed 4,000 every 30 s (cooldown 30), the necromancer's Spirit Tap's look
        (0xAF72B894, Sandstorm's: tools/sandstorm_spell_visuals.tsv), all of it back on him (petition 1439);
      a heal of 150 at every hit that lands on him (hitheal, petition 1439);
      Tunare his preferred foe (prefer, petition 1438);
      three Servants of Moogie below 25%, at half his figures (summon, petition 1440), a line of his;
      his enrage below 10%: three times his size, every 3 s the players of his hate list beyond his reach pulled to him,
        half of each of his hits on the one he fights on the second of his hate list (enrage, petition 1443), a line;
  classtemplates "tunare" (the owner: "a full heal with a 30 min recast timer", "she cast it at 20% hp"): Tunare's
    Renewal first in her list, 100% of her hit points below 20%, every and cooldown 1800 s, the druid's Healing's
    look (0xC2B700FA, Sandstorm's); her four abilities after it as they are;
  classtemplates "innoruuk" (the owner: "lets make innoruuk have same abilities as red swarming death with Largesse of
    Plague", then "give innoruuk similar to veil of death, but scale it to innoruuks hp"): his four casts (905145-905148)
    out, the Red Swarming Death's kit in (its petstats row's abilities, spell 700823: its damage over time, nuke and
    lifetap, and its template "pet swarming death"'s melee proc, Swarming Sting, as his template's proc), Largesse of
    Plague (700794) as a proc he always has (at the chance a pet of his level has under it), and Veil of Death: 1% of
    his most hit points at every melee hit that lands, AR; his two meleekind lines, his crit and his hit points kept;
  npcs "Servant of Moogie", spawn-only (npcspawnonly), Mr. Moogie's body at half his size, level 65, no NPC bit: the
    original his summon copies; npcaggro: it attacks no one on sight (it fights his hate list);
  npcs "Metal Moogie V2" (petition 1441), spawn-only, a copy of Metal Moogie's row at 1.2 times his size, his npcitem
    rows copied (his corpse gives a Moogie Tribute as Metal Moogie's does); classtemplates "metal moogie v2", Metal
    Moogie's at 1.1 times every figure (hit points, melee, special, kick, proc, damage shield, regeneration in a fight,
    each ability's figure: a spell's damage_pct, a fixed damage or heal, a fight's proc), and Gear Barrage, a fixed
    2,000 every 10 s cast only out of his reach within 100 (petition 1442: the bard's Melodic Blades' look, 0xD92D4042,
    Sandstorm's); npctemplate gives it his name.

It finds every row by its name (Mr. Moogie by name and level 80) and refuses, writing nothing, when one is not as these
asks found it: Mr. Moogie's and Tunare's abilities other than the brief's, Innoruuk's other than his two meleekind lines
and the four casts, the Red Swarming Death's kit other than the brief's, no Metal Moogie (spawn-only) or his template,
Largesse of Plague's record not of its name, or a Servant of Moogie or a Metal Moogie V2 already there. With no Mr.
Moogie of level 80 (another shard) it says so and writes nothing, exit 0. It prints what it would write, Metal Moogie's
figures against V2's, Largesse of Plague's chance at Innoruuk's level, and Innoruuk's hit points against Rallos Zek
Unbound's (the owner's "same health as rallos zek third form": a check, nothing written).

WHEN IT TAKES EFFECT: the server reads npcs, npcspawnonly, npcitem, npcaggro and the templates at its start (a GM
panel's edit of a template, or its reload_npcs, reads the templates again at once); run it as the before.sh of the keep
restart that brings this code, never under an older server. A running Lay Hands cooldown of the old hour is held to the
new 15 minutes from the first look at it (abilitytimes.go).

Report only by default; --apply writes it in one transaction, every row with its gm_audit line and its own line in
moogie_tunare_1110_log; --undo puts back each row still as written (one changed since is left and said) and the ids
its new rows took when no other row took one since.

    tools/fix_moogie_tunare_20261110.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import math
import sqlite3
import sys

LOG = "moogie_tunare_1110_log"
SRC = "moogie1110"
WHO, VIA = "dab", "tool"
DETAIL = ("petitions 1437-1443 (Jibrish) and the owner's Tunare and Innoruuk asks of 2026-11-10: "
          "tools/fix_moogie_tunare_20261110.py")

MOOGIE_NAME, MOOGIE_LEVEL = "Mr. Moogie", 80
MOOGIE_T, TUNARE_T, INNO_T, MM_T, V2_T, RSD_T = "mr moogie", "tunare", "innoruuk", "metal moogie", "metal moogie v2", "pet swarming death"
SERVANT, SERVANT_KEY, SERVANT_LEVEL, SERVANT_SIZE = "Servant of Moogie", "servant of moogie", 65, 0.5
MM_NAME, V2_NAME, V2_KEY = "Metal Moogie", "Metal Moogie V2", "metal moogie v2"
V2_SIZE, V2_FIGURES = 1.2, 1.1
RSD_SPELL, LARGESSE, LARGESSE_NAME = 700823, 700794, "Largesse of Plague"
INNO_CASTS = (905145, 905146, 905147, 905148)
RALLOS = "Rallos Zek Unbound"

FX_SPIRIT_TAP, FX_DRUID_HEALING, FX_MELODIC_BLADES = 0xAF72B894, 0xC2B700FA, 0xD92D4042

MOOGIE_NOW = [
    {"kind": "strip", "every": 30, "first": 30, "name": "Royal Decree"},
    {"kind": "heal", "spell_id": 700046, "heal_pct": 100, "below": 50, "every": 3600, "cooldown": 3600,
     "say": "Thats an hour cool down, you'll pay for that", "proc_chance": 25, "proc_damage": 1500, "proc_name": "Royal Fury"},
]
MOOGIE_NEW = [
    {"kind": "strip", "every": 30, "first": 30, "name": "Royal Decree"},
    {"kind": "heal", "spell_id": 700046, "heal_pct": 100, "below": 50, "every": 900, "cooldown": 900,
     "say": "Thats a fifteen minute cool down, you'll pay for that", "proc_chance": 100, "proc_damage": 350, "proc_name": "Royal Fury"},
    {"kind": "lifetap", "name": "Royal Siphon", "fx": FX_SPIRIT_TAP, "damage": 4000, "every": 30, "cooldown": 30},
    {"kind": "hitheal", "heal": 150},
    {"kind": "prefer", "foes": ["Tunare"]},
    {"kind": "summon", "below": 25, "count": 3, "npc": SERVANT, "share": 50, "say": "Servants of the jungle, to your king!"},
    {"kind": "enrage", "below": 10, "grow": 200, "every": 3, "rampage": 50, "say": "Enough! The king of this jungle does not fall!"},
]
TUNARE_NOW = [
    {"kind": "nuke", "spell_id": 701115, "every": 8, "first": 2, "damage_pct": 200},
    {"kind": "debuff", "spell_id": 700744, "every": 24, "first": 5, "damage_pct": 300},
    {"kind": "nuke", "spell_id": 700372, "every": 30, "first": 12, "damage_pct": 300},
    {"kind": "heal", "spell_id": 900486, "every": 20, "below": 50, "damage_pct": 1000},
]
RENEWAL = {"kind": "heal", "heal_pct": 100, "below": 20, "every": 1800, "cooldown": 1800, "name": "Tunare's Renewal", "fx": FX_DRUID_HEALING}
RSD_ABILITIES = [
    {"cast": 0.5, "every": 24, "first": 1, "kind": "dot", "spell_id": 905100},
    {"every": 50, "first": 3, "kind": "nuke", "spell_id": 905102},
    {"every": 30, "first": 5, "kind": "lifetap", "spell_id": 905101},
]
RSD_PROC = {"proc_chance": 25, "proc_damage": 250, "proc_name": "Swarming Sting", "proc_resist": "AR"}
INNO_ADDED = [{"kind": "proc", "spell_id": LARGESSE}, {"kind": "veil", "tap_pct": 1, "resist": "AR", "name": "Veil of Death"}]
GEAR_BARRAGE = {"kind": "nuke", "name": "Gear Barrage", "fx": FX_MELODIC_BLADES, "damage": 2000, "every": 10, "out_of_reach": True, "range": 100}

# a template's columns V2 takes at V2_FIGURES of Metal Moogie's, as whole numbers but special_damage
V2_SCALED = ("hp_pct", "melee_pct", "special_damage", "kick_damage", "proc_damage", "shield_damage", "fight_regen")
MOST = {"hp_pct": 100000, "melee_pct": 5000, "special_damage": 200, "kick_damage": 1000000, "proc_damage": 100000,
        "shield_damage": 100000, "fight_regen": 1000000}
KEYS = {"classtemplates": ("name",), "npctemplate": ("name",), "npcs": ("id",), "npcspawnonly": ("name",),
        "npcitem": ("itemid",), "npcaggro": ("npc_name",)}
SEQS = ("npcs", "npcitem")


def rnd(x):
    """Go's math.Round: halves away from zero."""
    return int(math.floor(x + 0.5)) if x >= 0 else -int(math.floor(-x + 0.5))


def norm(v):
    """A JSON value with every number a float, for comparing two lists as the server reads them."""
    if isinstance(v, bool):
        return v
    if isinstance(v, (int, float)):
        return float(v)
    if isinstance(v, list):
        return [norm(x) for x in v]
    if isinstance(v, dict):
        return {k: norm(x) for k, x in v.items()}
    return v


def same_abilities(text, want):
    try:
        got = json.loads(text or "[]")
    except ValueError:
        return False
    return norm(got) == norm(want)


def abilities_json(list_):
    return json.dumps(list_, separators=(",", ":"), ensure_ascii=False)


def go_json(m):
    """A row as Go's json.Marshal writes a map (keys sorted, <>& escaped), as the panel's audit has it."""
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def row_of(db, tbl, key):
    where = " AND ".join(f"{c} = ?" for c in KEYS[tbl])
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", tuple(key[c] for c in KEYS[tbl]))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def key_of(tbl, row):
    return {c: row[c] for c in KEYS[tbl]}


def npc_key(name):
    """database.NPCClassKey: lower case, trimmed, no trailing $."""
    return name.strip().lower().rstrip("$").strip()


def between(rows, level, col):
    """balance.go's Level: the row of the level, the last row past the end, the first before it, else the straight
    line between the two rows round it; 100 with no rows."""
    rows = sorted(rows, key=lambda r: r["level"])
    if not rows:
        return 100.0
    for i, r in enumerate(rows):
        if r["level"] >= level:
            if r["level"] == level or i == 0:
                return r[col]
            p = rows[i - 1]
            return p[col] + (r[col] - p[col]) * (level - p["level"]) / (r["level"] - p["level"])
    return rows[-1][col]


def npc_max_hp(db, level, hp_pct):
    """An NPC's most hit points as the server gives them: level x 60 (10 at least) at its level's balance, then its
    template's hp_pct (entity's applyBalanceLocked)."""
    base = max(level * 60, 10)
    levels = [{"level": r[0], "hp_pct": r[1]} for r in db.execute("SELECT level, hp_pct FROM balancelevel")] if table(db, "balancelevel") else []
    bal = between(levels, level, "hp_pct")
    if bal != 100:
        base = max(rnd(base * bal / 100), 1)
    return max(1, rnd(base * hp_pct / 100))


def template_of(db, npc_name):
    """The class template an NPC of the name fights by (npctemplate), its row, or None."""
    t = db.execute("SELECT template FROM npctemplate WHERE name = ?", (npc_key(npc_name),)).fetchone()
    if t is None:
        return None
    return row_of(db, "classtemplates", {"name": t[0].strip().lower()})


def scaled_ability(a, f):
    """An ability of Metal Moogie's at f of its figures (V2's): a fixed damage or a hit heal's heal at f, a spell's
    figures (damage_pct) at f, a fight's proc at f; a heal of its share of hit points (heal_pct) and a veil's tap are
    already V2's at f (his hit points are), and every clock as it is."""
    a = dict(a)
    k = a.get("kind")
    if a.get("damage"):
        a["damage"] = rnd(a["damage"] * f)
    elif k in ("nuke", "aoe", "lifetap", "debuff", "dot", "taunt") or (k == "heal" and not a.get("heal_pct")):
        a["damage_pct"] = min(rnd((a.get("damage_pct") or 100) * f), 1000)
    if a.get("heal") and k == "hitheal":
        a["heal"] = rnd(a["heal"] * f)
    if a.get("proc_damage"):
        a["proc_damage"] = rnd(a["proc_damage"] * f)
    return a


def plan(db):
    """What the tool would write: (changes, lines, refusals, here). A change is (tbl, before row or None, after row);
    here is False on a server with no Mr. Moogie of level 80."""
    lines, refusals, changes = [], [], []
    moogies = db.execute("SELECT * FROM npcs WHERE lower(trim(npc_name)) = ?", (MOOGIE_NAME.lower(),)).fetchall()
    cols = [d[0] for d in db.execute("SELECT * FROM npcs LIMIT 0").description]
    moogies = [dict(zip(cols, r)) for r in moogies]
    mine = [m for m in moogies if m["npc_name"] == MOOGIE_NAME and m["npc_level"] == MOOGIE_LEVEL]
    if not mine:
        lines.append(f"no {MOOGIE_NAME} of level {MOOGIE_LEVEL} here (NPCs of the name: {len(moogies)}): not his shard")
        return [], lines, [], False
    if len(mine) != 1:
        refusals.append(f"{len(mine)} NPCs named {MOOGIE_NAME} of level {MOOGIE_LEVEL}: his servants copy one")
    moogie = mine[0]

    def template(name):
        t = row_of(db, "classtemplates", {"name": name})
        if t is None:
            refusals.append(f"no class template {name!r}")
        return t

    # Mr. Moogie
    t = template(MOOGIE_T)
    if t is not None:
        if not same_abilities(t["abilities"], MOOGIE_NOW):
            refusals.append(f"{MOOGIE_T!r}'s abilities are not Royal Decree and his hour's Lay Hands as found: {t['abilities']}")
        else:
            changes.append(("classtemplates", t, dict(t, abilities=abilities_json(MOOGIE_NEW))))
            lines.append(f"classtemplates {MOOGIE_T!r}: Lay Hands every and cooldown 900 s, its proc 100% for 350, its line of fifteen "
                         f"minutes; Royal Siphon (4,000 every 30 s, back on him); a heal of 150 at every hit; Tunare first; three "
                         f"Servants of Moogie below 25% at half; his enrage below 10% (x3, pulls every 3 s, rampage 50%)")
    # Tunare
    t = template(TUNARE_T)
    if t is not None:
        if not same_abilities(t["abilities"], TUNARE_NOW):
            refusals.append(f"{TUNARE_T!r}'s abilities are not the four found: {t['abilities']}")
        else:
            changes.append(("classtemplates", t, dict(t, abilities=abilities_json([RENEWAL] + TUNARE_NOW))))
            lines.append(f"classtemplates {TUNARE_T!r}: Tunare's Renewal first, 100% of her hit points below 20%, every and cooldown "
                         f"1800 s; her four after it")
    # Innoruuk
    t, rsd, pet = template(INNO_T), template(RSD_T), db.execute("SELECT abilities, name FROM petstats WHERE spellid = ?", (RSD_SPELL,)).fetchone()
    if pet is None:
        refusals.append(f"no petstats row of spell {RSD_SPELL} (the Red Swarming Death)")
    elif not same_abilities(pet[0], RSD_ABILITIES):
        refusals.append(f"the Red Swarming Death's kit ({RSD_SPELL}) is not the brief's: {pet[0]}")
    if rsd is not None and any(rsd[c] != v for c, v in RSD_PROC.items()):
        refusals.append(f"{RSD_T!r}'s proc is not Swarming Sting, 25% for 250, AR: "
                        f"{[rsd[c] for c in RSD_PROC]}")
    lp = db.execute("SELECT spellname FROM spellpattern WHERE spellid = ?", (LARGESSE,)).fetchone()
    if lp is None or (lp[0] or "").strip().lower() != LARGESSE_NAME.lower():
        refusals.append(f"spell {LARGESSE} is {lp and lp[0]!r}, not {LARGESSE_NAME}")
    if t is not None:
        try:
            have = json.loads(t["abilities"] or "[]")
        except ValueError:
            have = None
        melee = [a for a in have or [] if a.get("kind") == "meleekind"]
        casts = sorted(int(a.get("spell_id") or 0) for a in have or [] if a.get("kind") != "meleekind")
        if have is None or len(have) != 6 or len(melee) != 2 or casts != sorted(INNO_CASTS):
            refusals.append(f"{INNO_T!r}'s abilities are not his two meleekind lines and the casts {INNO_CASTS}: {t['abilities']}")
        elif pet is not None and rsd is not None:
            after = dict(t, abilities=abilities_json(melee + json.loads(pet[0]) + INNO_ADDED), **RSD_PROC)
            changes.append(("classtemplates", t, after))
            level = db.execute("SELECT npc_level FROM npcs WHERE lower(trim(npc_name)) = 'innoruuk' ORDER BY id LIMIT 1").fetchone()
            lvl = level[0] if level else 70
            chance = min(3 + (3 * lvl) // 20, 25)
            lines.append(f"classtemplates {INNO_T!r}: the casts {INNO_CASTS} out; the Red Swarming Death's damage over time, nuke and "
                         f"lifetap in; Swarming Sting (25% for 250, AR) his template's proc (was {t['proc_chance']}% for "
                         f"{t['proc_damage']}); Largesse of Plague a proc he always has, at a pet's chance of his level {lvl}: "
                         f"{chance}% (3% and 1 more for each 20 of a pet's INT, three a level, at most 25%: proc.go); Veil of Death, "
                         f"1% of his most hit points at every hit, AR")
            if lvl:
                inno_hp = npc_max_hp(db, lvl, t["hp_pct"])
                rz = db.execute("SELECT npc_name, npc_level FROM npcs WHERE lower(trim(npc_name)) = ? ORDER BY id LIMIT 1",
                                (RALLOS.lower(),)).fetchone()
                rt = template_of(db, RALLOS) if rz else None
                if rz and rt:
                    rz_hp = npc_max_hp(db, rz[1], rt["hp_pct"])
                    lines.append(f"Innoruuk's hit points {inno_hp:,} (level {lvl}, hp_pct {t['hp_pct']}), {RALLOS}'s {rz_hp:,} "
                                 f"(level {rz[1]}, hp_pct {rt['hp_pct']}): {'the same' if inno_hp == rz_hp else 'NOT the same'}; "
                                 f"his veil takes {rnd(inno_hp / 100):,} a hit")
                else:
                    lines.append(f"Innoruuk's hit points {inno_hp:,}; no {RALLOS} of a template here to hold them to")
    # the Servants of Moogie
    if db.execute("SELECT count(*) FROM npcs WHERE lower(trim(npc_name)) = ?", (SERVANT_KEY,)).fetchone()[0]:
        refusals.append(f"an NPC named {SERVANT} is there already")
    servant = dict(moogie, npc_name=SERVANT, npc_level=SERVANT_LEVEL, size=round((moogie["size"] or 1) * SERVANT_SIZE, 4), npc_type=0)
    del servant["id"]
    changes.append(("npcs", None, servant))
    changes.append(("npcspawnonly", None, {"name": SERVANT_KEY, "note": "Petition 1440: Mr. Moogie's summon copies it below 25%, "
                                           "three at half his figures.", "source": SRC}))
    changes.append(("npcaggro", None, {"npc_name": SERVANT, "aggressive": 0, "radius": 0,
                                       "note": "Petition 1440: Mr. Moogie's servants fight his hate list, never on sight.", "source": SRC}))
    lines.append(f"npcs {SERVANT!r}: spawn-only, {MOOGIE_NAME}'s body at {servant['size']} (his {moogie['size']}), level "
                 f"{SERVANT_LEVEL}; never attacks on sight (npcaggro)")
    # Metal Moogie V2
    mm = db.execute("SELECT * FROM npcs WHERE npc_name = ?", (MM_NAME,)).fetchall()
    mm = [dict(zip(cols, r)) for r in mm]
    mmt = template(MM_T)
    if len(mm) != 1:
        refusals.append(f"{len(mm)} NPCs named {MM_NAME}, want his one spawn-only original")
    elif not db.execute("SELECT count(*) FROM npcspawnonly WHERE lower(name) = ?", (MM_T,)).fetchone()[0]:
        refusals.append(f"{MM_NAME} is not spawn-only (npcspawnonly)")
    for q, what in [("SELECT count(*) FROM npcs WHERE lower(trim(npc_name)) = ?", "an NPC named Metal Moogie V2"),
                    ("SELECT count(*) FROM classtemplates WHERE name = ?", "a class template 'metal moogie v2'"),
                    ("SELECT count(*) FROM npctemplate WHERE name = ?", "an npctemplate row 'metal moogie v2'"),
                    ("SELECT count(*) FROM npcspawnonly WHERE lower(name) = ?", "a spawn-only row 'metal moogie v2'")]:
        if db.execute(q, (V2_KEY,)).fetchone()[0]:
            refusals.append(f"{what} is there already")
    if len(mm) == 1 and mmt is not None:
        m = mm[0]
        v2 = dict(m, npc_name=V2_NAME, size=round((m["size"] or 1) * V2_SIZE, 4))
        del v2["id"]
        changes.append(("npcs", None, v2))
        changes.append(("npcspawnonly", None, {"name": V2_KEY, "note": "Petition 1441: Mr. Moogie's challenge copies him for a "
                                               "Moogie Tribute.", "source": SRC}))
        items = db.execute("SELECT * FROM npcitem WHERE npcid = ?", (m["id"],)).fetchall()
        icols = [d[0] for d in db.execute("SELECT * FROM npcitem LIMIT 0").description]
        for it in items:
            it = dict(zip(icols, it))
            del it["itemid"]
            changes.append(("npcitem", None, dict(it, npcid="V2")))  # V2's id, once his row is written
        vt = dict(mmt, name=V2_T, source=SRC)
        figures = []
        for c in V2_SCALED:
            v = mmt[c] or 0
            nv = round(v * V2_FIGURES, 4) if c == "special_damage" else rnd(v * V2_FIGURES)
            vt[c] = min(nv, MOST[c])
            if v:
                figures.append(f"{c} {v} -> {vt[c]}")
        abilities = [scaled_ability(a, V2_FIGURES) for a in json.loads(mmt["abilities"] or "[]")] + [GEAR_BARRAGE]
        vt["abilities"] = abilities_json(abilities)
        vt["note"] = ("Petitions 1441/1442 (Jibrish): Metal Moogie V2, Metal Moogie at 1.1 times every figure and 1.2 times his size, "
                      "for a Moogie Tribute; Gear Barrage, a fixed 2,000 every 10 s only while what he fights stands out of his reach "
                      "(within 100). tools/fix_moogie_tunare_20261110.py")
        changes.append(("classtemplates", None, vt))
        changes.append(("npctemplate", None, {"name": V2_KEY, "template": V2_T, "note": "Petition 1441: V2 fights by his own template.",
                                              "source": SRC}))
        lines.append(f"npcs {V2_NAME!r}: spawn-only, {MM_NAME}'s row at size {v2['size']} (his {m['size']}), his {len(items)} "
                     f"npcitem rows copied")
        lines.append(f"classtemplates {V2_T!r}: {', '.join(figures) or 'no figures'}; abilities "
                     f"{mmt['abilities']} -> {vt['abilities']}")
    return changes, lines, refusals, True


LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,
    rowkey  TEXT NOT NULL,      -- the row's key (JSON)
    before  TEXT,               -- the row before (JSON), NULL: made here
    after   TEXT NOT NULL,      -- the row as written (JSON)
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def audit(db, action, tbl, key, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key),
                      None if before is None else go_json(before), None if after is None else go_json(after), detail))
    return cur.lastrowid


def seqs(db):
    if not table(db, "sqlite_sequence"):
        return {}
    return {t: (lambda r: None if r is None else r[0])(db.execute("SELECT seq FROM sqlite_sequence WHERE name = ?", (t,)).fetchone())
            for t in SEQS}


def apply(db, changes):
    """The plan written, in the caller's transaction: each row and its gm_audit line, its own line in the log. Returns
    how many rows."""
    db.execute(LOG_SCHEMA)
    before_seq = seqs(db)
    v2_id = None
    n = 0
    for tbl, before, after in changes:
        after = dict(after)
        if tbl == "npcitem" and after.get("npcid") == "V2":
            after["npcid"] = v2_id
        if before is None:
            cols = list(after)
            cur = db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [after[c] for c in cols])
            if tbl == "npcs":
                db.execute("UPDATE npcs SET npc_id = ? WHERE id = ?", (cur.lastrowid, cur.lastrowid))
                if after["npc_name"] == V2_NAME:
                    v2_id = cur.lastrowid
                key = {"id": cur.lastrowid}
            elif tbl == "npcitem":
                key = {"itemid": cur.lastrowid}
            else:
                key = key_of(tbl, after)
            row = row_of(db, tbl, key)
            a = audit(db, "insert", tbl, key, None, row, DETAIL)
        else:
            key = key_of(tbl, before)
            sets = [c for c in after if after[c] != before.get(c)]
            db.execute(f"UPDATE {tbl} SET {', '.join(c + ' = ?' for c in sets)} WHERE "
                       + " AND ".join(f"{c} = ?" for c in KEYS[tbl]), [after[c] for c in sets] + [key[c] for c in KEYS[tbl]])
            row = row_of(db, tbl, key)
            a = audit(db, "update", tbl, key, before, row, DETAIL)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                   (tbl, json.dumps(key), None if before is None else json.dumps(before, sort_keys=True, default=str),
                    json.dumps(row, sort_keys=True, default=str), a))
        n += 1
    db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES ('sqlite_sequence', '{{}}', ?, ?)",
               (json.dumps(before_seq), json.dumps(seqs(db))))
    return n


def undo(db):
    """Each logged row still as written put back as it was, the last first, its gm_audit line; then the ids the new rows
    took, when no other row took one since. Returns (back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    seq_row = db.execute(f"SELECT before, after FROM {LOG} WHERE tbl = 'sqlite_sequence'").fetchone()
    for seq, tbl, key, before, after in db.execute(
            f"SELECT seq, tbl, rowkey, before, after FROM {LOG} WHERE tbl != 'sqlite_sequence' ORDER BY seq DESC").fetchall():
        key = json.loads(key)
        now = row_of(db, tbl, key)
        if now is None or json.dumps(now, sort_keys=True, default=str) != after:
            left += 1
            print(f"  left: {tbl} {key}, changed since it was written")
            continue
        where = " AND ".join(f"{c} = ?" for c in KEYS[tbl])
        if before is None:
            db.execute(f"DELETE FROM {tbl} WHERE {where}", [key[c] for c in KEYS[tbl]])
            audit(db, "delete", tbl, key, now, None, "undo: " + DETAIL)
        else:
            b = json.loads(before)
            db.execute(f"UPDATE {tbl} SET {', '.join(c + ' = ?' for c in b)} WHERE {where}", [b[c] for c in b] + [key[c] for c in KEYS[tbl]])
            audit(db, "update", tbl, key, now, row_of(db, tbl, key), "undo: " + DETAIL)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if left == 0:
        if seq_row is not None:
            was, written = json.loads(seq_row[0]), json.loads(seq_row[1])
            cur = seqs(db)
            for t in SEQS:
                if was.get(t) is not None and cur.get(t) == written.get(t):
                    db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = ?", (was[t], t))
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        if a.undo:
            db.execute("BEGIN IMMEDIATE")
            try:
                back, left = undo(db)
                db.execute("COMMIT")
            except BaseException:
                db.execute("ROLLBACK")
                raise
            print(f"put back {back} rows, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        if table(db, LOG):
            print(f"applied already ({LOG} is there): --undo first")
            return 1
        changes, lines, refusals, here = plan(db)
        for l in lines:
            print(l)
        if not here:
            print("nothing done")
            return 0
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not a.apply:
            print(f"report only: {len(changes)} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            n = apply(db, changes)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {n} rows, each in gm_audit and {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
