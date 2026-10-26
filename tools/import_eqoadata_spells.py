#!/usr/bin/env python3
"""The spells of EQOA-Data's per-class lists this server has no ability
record for: their records (spellpattern), what each does (spelleffect), the
classes that have them (classspells), and for a merchant spell its scroll at
the class's spell vendors (itempattern, npcitem).

F3 20261025-222444: "Cleric is missing most of its spells after level 20."
spellpattern held only what Sandstorm's dumps and vendors and Return Home's
export gave (402 records); EQOA-Data (Information/databases/spells/
<class>.json) lists 1,693 abilities, each with its level, its line ("Target,
Increase HP per tic by 250 ; Increase 25% based on CHA, 24sec"), how it is
got (Merchant, Quest, CM, ...) and its icon's name.

What a made record holds, and where each value comes from:

  spellid      ID_BASE and up (700001...), clearly this tool's.
  spellname    the list's name; spelldesc its line (the spell book shows it),
               cut to the 127 characters VIAbility::Read takes.
  abilitylvl   the list's level. A name two classes list at one level is one
               record for both; at two levels, two records.
  scope        the line's first word: Self 0, Target 1, Group 2, Pet 3
               (else "group" or "target" anywhere in it, else 0).
  iconColor    the scope's frame, as every record of that scope here has it.
  range        4.5 for a self spell, 30.0 for the rest (a float's high half,
               writeAbilityRecord), as the records of each scope mostly are.
  icon         ICONS: the list's icon name as the records that carry it agree
               (most do), else VIHashResourceID of SOE's own file
               ("P:\\studio\\eqps2\\Game_Assets\\UI\\<name>.tga", the decomp's
               vihashicon.cpp), every one found in the Frontiers disc's
               UI.CSF; a line with no icon name, its kind's.
  casttime, recast, power
               the line's own when it says them ("Power cost: 500, Cast: 3,
               Recast: 19", "45recast"); else from the nearest-level record
               of the same line (its words with the numbers taken out: the
               ranks of one spell), of the class first, then any class; else
               of the same kind and scope; the power scaled by level from it.
               OURS, not measured: say so to anyone who asks.
  eqprequire   -1.

classspells: a made record goes to every class listing it by a way a
character of the class gets it by level (LEVELED: a merchant, a quest, a
drop); a class listing a name this server already has a record for, and no
classspells row of it, gets that record (the one of its level first).
Masteries (CM), master classes (MCA) and the epics are not given out: none
can be bought or earned here yet, and #template gives a class every spell
classspells holds. --all-sources makes their records as well, unlinked.

spelleffect: a line for each made record (a line saying nothing, as
"Unknown", none), source "eqoa-data <class> (<how>); made". A line already
there is never written: GMs edit them.

Scrolls: a merchant spell with no scroll of its name (the scroll's art,
-607649328, worn nowhere: scribe.go) gets one, copied from the class's
nearest-level scroll (its level, a price scaled by the square of the level,
the classes that buy it), sold by every vendor that sells the class's scrolls
in that level band (three bands a class on Sandstorm: 1-20, 24-34, 39-49),
in every city, and above the last band by the last.

Report only by default; --apply writes it all in one transaction and logs
every row; --undo takes exactly that back (refused while a character holds
a made spell or scroll, unless --force). Stop the server around --apply and
--undo, as with every importer.

    tools/import_eqoadata_spells.py --eqoa-data ../external/EQOA-Data [--db eqoa.db]
        [--apply | --undo [--force]] [--all-sources] [--list]
"""
import argparse
import collections
import datetime
import json
import os
import re
import sqlite3
import statistics
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
CLASSES = "WAR RAN PAL SK MNK BRD RGE DRD SHA CL MAG NEC ENC WIZ ALC".split()
# The casting classes: a rank's numbers come from one of the same side first
# (a warrior's instant art is no model for a cleric's spell).
CASTERS = {7, 8, 9, 10, 11, 12, 13, 14}
CLASS_FILES = ["warrior", "ranger", "paladin", "shadowknight", "monk", "bard", "rogue", "druid",
               "shaman", "cleric", "magician", "necromancer", "enchanter", "wizard", "alchemist"]
SCROLL_ICON = -607649328
ALL_CLASSES = 32767
ID_BASE = 700000

# The ways a character of the class gets a spell by level. "None" and
# "Unknown" are the lists' own words (the shaman's whole list says None).
LEVELED = {"Merchant", "Quest", "Mob Drop", "None", "Unknown", "", "MC", "EG Stone", "PoD Drop"}

SCOPE_FRAME = {0: -1809115846, 1: 486870879, 2: 593297002, 3: 439871130}
SCOPE_RANGE = {0: 16528, 1: 16880, 2: 16880, 3: 16880}

# The list's icon names, as DictIDs (see the doc string). "records n/m": n of
# the m records whose name the lists give with that icon carry it; "disc":
# the file's hash, in UI.CSF. Checked against the Frontiers disc 2026-10-26.
ICONS = {
    "Heal": -2105187409,               # disc heal.tga
    "Profile_Cloud": -1664015648,      # records 17/27
    "Physical_Stat": -630227479,       # records 36/47 (no file of that name)
    "Defense_Active": 1920746744,      # records 21/23 (no file of that name)
    "Summoning": -862007142,           # records 42/42
    "Sword": -1265863830,              # records 30/30
    "Muscle_Arm": 1028836133,          # records 11/12
    "Fire": 829958967,                 # records 12/14 (no file of that name)
    "Profile_Lightning": -901013979,   # records 6/6
    "Mask": 1507275072,                # records 12/20
    "Fear": -49534593,                 # records 32/32
    "Summon": -466420384,              # disc summon.tga
    "DoT": -818129029,                 # records 20/20 (no file of that name)
    "Poison": 819994203,               # records 6/9
    "Cold": 282007337,                 # records 7/8 (no file of that name)
    "Gate": 2072956978,                # records 45/45
    "Bleeding_Heart": 2036326070,      # records 17/17
    "Stun": -901013979,                # records 7/8
    "Boot": 638503769,                 # records 21/21
    "Weapons_Ranged": 2089043806,      # disc weapons_ranged.tga
    "Boot_Shackled": -1663337271,      # disc Boot_Shackled.tga
    "Skeletal_Arm": -1110049323,       # records 7/8
    "Fist": -1249422789,               # disc Fist.tga
    "Wand": -79473185,                 # records 3/3
    "Moon": -568605204,                # records 4/4
    "Brazier": -960607353,             # records 5/6 (no file of that name)
    "Boot_Sticky": -610493769,         # records 6/6
    "Profile_Pinch": 1589074753,       # records 3/3
    "Play_Instruments": -2143509564,   # records 6/6
    "Red_Cross": -213673665,           # disc Red_Cross.tga
    "Physical_Alteration": 1507275072,  # disc physical_alteration.tga
    "Forage": -295381203,              # disc forage.tga
    "Phys_stat_decrease": 667007739,   # disc phys_stat_decrease.tga
    "Profile_Web": -1107090857,        # disc Profile_Web.tga
    "Scales": -582357912,              # disc Scales.tga
    "Swimming": 1533256884,            # disc swimming.tga
    "Bow": -846861641,                 # disc bow.tga
    "Area_of_effect": 2057020990,      # disc area_of_effect.tga
}
KIND_ICON = {"heal": ICONS["Heal"], "hit": ICONS["Profile_Lightning"], "buff": ICONS["Physical_Stat"],
             "debuff": ICONS["Phys_stat_decrease"], "gate": ICONS["Gate"], "pet": ICONS["Summoning"]}

LOG_DDL = """
CREATE TABLE IF NOT EXISTS eqoadata_spell_import_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
-- Every row written, in order: what --undo deletes, last first.
CREATE TABLE IF NOT EXISTS eqoadata_spell_import_rows (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,     -- spellpattern | spelleffect | classspells | itempattern | npcitem
    key    INTEGER NOT NULL,  -- spellid, patternid, npcitem.itemid; classspells: the class
    key2   INTEGER,           -- classspells: the spellid
    note   TEXT               -- the list's class, level and how it is got
);
"""


def spell_name(name):
    """The name a scroll and its spell share (the server's spellKey, and
    import_class_spells.py's)."""
    n = (name or "").strip()
    if n.endswith("$"):
        n = n[:-1].rstrip()
    n = re.sub(r"\s*\((?:[A-Za-z]{2,3}(?:\s*[,/]\s*)?)+\)\s*$", "", n)
    return n.replace("+", "").replace("`", "'").strip().lower()


def scope_of(line):
    d = (line or "").strip().lower()
    if re.search(r"ress?urr?ect|\brez\b", d):
        return 1  # a resurrection is cast at a corpse
    for word, scope in (("target undead", 1), ("target", 1), ("self", 0), ("group", 2), ("pet", 3)):
        if d == word or d.startswith(word + " ") or d.startswith(word + ","):
            return scope
    if re.search(r"\bgroup\b", d):
        return 2
    if re.search(r"\btarget\b", d):
        return 1
    return 0


def kind_of(line):
    d = (line or "").lower()
    if "bind point" in d:
        return "gate"
    if re.search(r"\bsumm[ou]ns?\b", d) and re.search(r"\bpet\b|to aid you|to your defense", d):
        return "pet"
    if re.search(r"(decrease|debuff|damage|drain)\w*\s+(hit ?points|hitoints|hp)\b|\bdamage\b", d):
        return "hit"
    if re.search(r"(increase|buff|heal)\w*\s+(hit ?points|hp)\b|\bheals?\b", d):
        return "heal"
    if re.search(r"\b(decrease|debuff|drain|lower)\b", d):
        return "debuff"
    return "buff"


def signature(line):
    """A line with its numbers taken out: the ranks of one spell worded alike
    share it."""
    return re.sub(r"\s+", " ", re.sub(r"\d+(?:\.\d+)?", "N", (line or "").strip().lower()))


# What a line acts on, longest first (a later word inside an earlier one is
# taken out with it): the ranks of one spell worded differently ("Increase
# Maximum HP by 40, Increase AC by 40"; "Buff MaxHP by 120 and AC by 120")
# share these.
SUBJECTS = [("hp per tic", "hptick"), ("hitpoints per tic", "hptick"), ("hp per tick", "hptick"),
            ("power per tic", "powertick"), ("power per tick", "powertick"),
            ("maximum hp", "maxhp"), ("max hp", "maxhp"), ("maxhp", "maxhp"), ("mhp", "maxhp"),
            ("maximum power", "maxpower"), ("max power", "maxpower"), ("maxpower", "maxpower"),
            ("hit points", "hp"), ("hitpoints", "hp"), ("hitoints", "hp"), ("health", "hp"), ("hp", "hp"),
            ("power", "power"), ("movement speed", "speed"), ("hate", "hate")] + \
           [(w, w) for w in ("str", "sta", "agi", "dex", "wis", "int", "cha", "ac", "fr", "cr", "pr", "dr", "lr", "ar")]
MARKS = ("root", "snare", "sow", "invis", "charm", "lull", "fear", "stun", "mesmer", "summon", "bind point",
         "form", "damage shield", "damageshield", "proc", "resurrect", "ressurect", "rez", "remove", "cure")


def structure(line):
    """(scope, what the line raises, lowers and does), its numbers aside."""
    d = (line or "").lower()
    d = re.sub(r"\b(?:increase|modified|mod)?\s*\d+\s*%\s*(?:based on|mod by|modified by|by)\s+\w+", " ", d)
    # a radius and a buff's hate are notes, not what it does (spellnotes.go)
    d = re.sub(r"\b\d+\s*rad(?:ius)?\b|\bradius\s*\d+|\b\d+\s*hate\b|\bhate\s*\d+|\bno hate\b", " ", d)
    d = re.sub(r"\d+(?:\.\d+)?", " ", d)
    out, sign = set(), "+"
    for clause in re.split(r"[,;]", d):
        m = re.search(r"\b(increase|buff|raise|heal|decrease|debuff|lower|drain|damage)", clause)
        if m:
            sign = "-" if m.group(1) in ("decrease", "debuff", "lower", "drain", "damage") else "+"
        out |= {k for k in MARKS if k in clause}
        for word, subj in SUBJECTS:
            pat = r"\b" + re.escape(word) + r"\b"
            if re.search(pat, clause):
                out.add(sign + subj)
                clause = re.sub(pat, " ", clause)
    return scope_of(line), frozenset(out)


def numbers_said(line):
    """(cast, recast, power) where the line says them, else None each."""
    d = (line or "").lower()
    out = [None, None, None]
    for i, pat in enumerate((r"\bcast:\s*(\d+)", r"\brecast:\s*(\d+)", r"\bpower cost:\s*(\d+)")):
        m = re.search(pat, d)
        if m and int(m.group(1)) > 0:
            out[i] = int(m.group(1))
    if out[1] is None:
        m = re.search(r"(\d+)\s*(?:second\s+)?recast\b", d)
        if m and int(m.group(1)) > 0:
            out[1] = int(m.group(1))
    return out


def load_lists(path):
    """Every ability of the lists: (class, level, name, line, how, icon)."""
    d = path
    sub = os.path.join(path, "Information", "databases", "spells")
    if os.path.isdir(sub):
        d = sub
    out = []
    for cls, fname in enumerate(CLASS_FILES):
        p = os.path.join(d, fname + ".json")
        if not os.path.exists(p):
            sys.exit(f"{p}: missing")
        with open(p) as fh:
            data = json.load(fh)
        for lv in data["list"]:
            for ab in lv["abilities"]:
                out.append(dict(cls=cls, level=int(lv["level"]), name=(ab.get("name") or "").strip(),
                                line=(ab.get("description") or "").strip(), how=(ab.get("source") or "").strip(),
                                icon=ab.get("icon")))
    return out


def columns(db, table):
    return [r[1] for r in db.execute(f"PRAGMA main.table_info({table})")]


def plan(db, lists, all_sources=False):
    """Everything --apply would write, as rows, and the report's tallies."""
    records = {}   # spellid -> row
    by_name = collections.defaultdict(list)
    for r in db.execute("SELECT spellid, abilitylvl, casttime, recast, power, scope, spellname FROM main.spellpattern"):
        records[r[0]] = r
        by_name[spell_name(r[6])].append(r)
    linked = collections.defaultdict(set)   # class -> spellids
    for cls, sid in db.execute("SELECT class, spellid FROM main.classspells"):
        linked[cls].add(sid)

    # what each existing record's line is, by the lists' names: the references
    # a made record takes its cast, recast and power from
    refs = []
    for e in lists:
        for r in by_name.get(spell_name(e["name"]), []):
            refs.append(dict(cls=e["cls"], level=max(1, r[1]), sig=signature(e["line"]), kind=kind_of(e["line"]),
                             shape=structure(e["line"])[1], scope=r[5], cast=r[2], recast=r[3], power=r[4]))

    def from_refs(e, scope):
        """cast, recast and power: a rank of the same line's, else the median
        of the kind's records at this scope (power by level)."""
        sig, kind, shape = signature(e["line"]), kind_of(e["line"]), structure(e["line"])[1]
        level = max(1, e["level"])
        side = e["cls"] in CASTERS
        same = lambda r: r["cls"] == e["cls"]
        kin = lambda r: (r["cls"] in CASTERS) == side
        for pick in (lambda r: r["sig"] == sig and same(r),
                     lambda r: r["sig"] == sig and kin(r),
                     lambda r: shape and r["shape"] == shape and r["scope"] == scope and same(r),
                     lambda r: shape and r["shape"] == shape and r["scope"] == scope and kin(r),
                     lambda r: shape and r["shape"] == shape and same(r),
                     lambda r: shape and r["shape"] == shape and kin(r),
                     lambda r: r["sig"] == sig,
                     lambda r: shape and r["shape"] == shape and r["scope"] == scope):
            cands = [r for r in refs if pick(r)]
            if cands:
                ref = min(cands, key=lambda r: (abs(r["level"] - e["level"]), -r["level"]))
                return ref["cast"], ref["recast"], round(ref["power"] * level / ref["level"])
        for pick in (lambda r: r["kind"] == kind and r["scope"] == scope, lambda r: r["scope"] == scope, lambda r: True):
            cands = [r for r in refs if pick(r)]
            if cands:
                return (round(statistics.median(r["cast"] for r in cands)),
                        round(statistics.median(r["recast"] for r in cands)),
                        round(statistics.median(r["power"] / r["level"] for r in cands) * level))
        return 2, 3, 10 * level

    # one record a (name, level) the server lacks; the classes listing it
    made = {}   # (key, level) -> dict
    links = []  # (class, spellid, note)
    skipped = collections.Counter()
    seen = set()
    for e in lists:
        key = spell_name(e["name"])
        if not key:
            continue
        leveled = e["how"] in LEVELED or e["how"].lower() in {s.lower() for s in LEVELED}
        if (e["cls"], key) in seen:
            skipped["the same name again in one class"] += 1
            continue
        seen.add((e["cls"], key))
        note = f"{CLASS_FILES[e['cls']]} L{e['level']} ({e['how'] or 'None'})"
        have = by_name.get(key)
        if have:
            if leveled and not (linked[e["cls"]] & {r[0] for r in have}):
                r = min(have, key=lambda r: (r[1] != e["level"], r[0]))
                links.append((e["cls"], r[0], note + "; record held"))
                linked[e["cls"]].add(r[0])
            continue
        if not leveled and not all_sources:
            skipped[f"got by {e['how']}"] += 1
            continue
        m = made.get((key, e["level"]))
        if m is None:
            scope = scope_of(e["line"])
            cast, recast, power = from_refs(e, scope)
            said = numbers_said(e["line"])
            cast = said[0] if said[0] is not None else cast
            recast = said[1] if said[1] is not None else recast
            power = said[2] if said[2] is not None else power
            icon = ICONS.get(e["icon"]) if e["icon"] else None
            if icon is None:
                icon = KIND_ICON[kind_of(e["line"])]
            m = dict(row=dict(abilitylvl=e["level"], unk2=0, unk3=0, range=SCOPE_RANGE[scope], casttime=cast,
                              power=power, iconColor=SCOPE_FRAME[scope], icon=icon, scope=scope, recast=recast,
                              eqprequire=-1, spellname=e["name"], spelldesc=e["line"][:127]),
                     line=e["line"], classes=[], hows=[], notes=[], merchant_classes=[])
            made[(key, e["level"])] = m
        if leveled:
            m["classes"].append(e["cls"])
            if e["how"] == "Merchant":
                m["merchant_classes"].append(e["cls"])
        m["hows"].append(e["how"] or "None")
        m["notes"].append(note)

    taken = {r for r in records}
    next_id = max([ID_BASE] + [k for k in taken if k >= ID_BASE]) + 1
    spells = []
    for (key, level), m in sorted(made.items(), key=lambda kv: (kv[1]["row"]["abilitylvl"], kv[0][0])):
        while next_id in taken:
            next_id += 1
        m["row"]["spellid"] = next_id
        taken.add(next_id)
        next_id += 1
        spells.append(m)
        for cls in sorted(set(m["classes"])):
            links.append((cls, m["row"]["spellid"], "; ".join(m["notes"])))
    effects = []
    for m in spells:
        if m["line"] and m["line"].lower() not in {"unknown", "none", "?"}:
            effects.append((m["row"]["spellid"], m["line"],
                            f"eqoa-data {m['notes'][0].split(' L')[0]} ({m['hows'][0]}); made"))
    scrolls, stock = plan_scrolls(db, spells)
    return dict(spells=spells, links=links, effects=effects, scrolls=scrolls, stock=stock, skipped=skipped)


def plan_scrolls(db, spells):
    """A scroll for each made merchant spell that has none, and the vendors
    that sell it."""
    scroll_rows = db.execute("SELECT * FROM main.itempattern WHERE itemicon = ? AND equipslot = -1",
                             (SCROLL_ICON,)).fetchall()
    icols = columns(db, "itempattern")
    named = {spell_name(r[icols.index("itemname")]) for r in scroll_rows}
    # vendor bands: npcid -> class -> [levels of its scrolls of the class]
    bands = collections.defaultdict(lambda: collections.defaultdict(list))
    for npcid, cu, lv in db.execute(
            "SELECT i.npcid, p.classuse, p.levelreq FROM main.npcitem i JOIN main.itempattern p ON p.patternid = i.patternid "
            "WHERE p.itemicon = ? AND p.equipslot = -1 AND p.classuse <> ?", (SCROLL_ICON, ALL_CLASSES)):
        for cls in range(len(CLASSES)):
            if cu >> cls & 1:
                bands[npcid][cls].append(lv)
    taken = {r[0] for r in db.execute("SELECT patternid FROM main.itempattern")}
    next_id = max([ID_BASE] + [k for k in taken if k >= ID_BASE]) + 1
    scrolls, stock = [], []
    for m in spells:
        classes = sorted(set(m["merchant_classes"]))
        name = m["row"]["spellname"]
        if not classes or spell_name(name) in named:
            continue
        level = m["row"]["abilitylvl"]
        own = [r for r in scroll_rows if any(r[icols.index("classuse")] >> c & 1 for c in classes)
               and r[icols.index("classuse")] != ALL_CLASSES]
        if not own:
            own = [r for r in scroll_rows if r[icols.index("classuse")] != ALL_CLASSES]
        ref = dict(zip(icols, min(own, key=lambda r: (abs(r[icols.index("levelreq")] - level), r[0]))))
        while next_id in taken:
            next_id += 1
        row = dict(ref)
        ref_level = max(1, ref["levelreq"])
        row.update(patternid=next_id, itemname=name, levelreq=level,
                   itemdesc=f"This scroll will allow you to learn {name}.",
                   itemcost=max(1, round((ref["itemcost"] or 0) * (max(1, level) / ref_level) ** 2)),
                   classuse=sum(1 << c for c in classes))
        taken.add(next_id)
        next_id += 1
        scrolls.append(row)
        named.add(spell_name(name))
        # every vendor of these classes' scrolls whose band holds the level, else the nearest band's
        vendors = set()
        for cls in classes:
            cands = []
            for npcid, per in bands.items():
                lv = per.get(cls)
                if not lv or len(lv) < 3:
                    continue
                lo, hi = min(lv), max(lv)
                cands.append((0 if lo <= level <= hi else min(abs(level - lo), abs(level - hi)), npcid))
            if cands:
                best = min(d for d, _ in cands)
                vendors |= {n for d, n in cands if d == best}
        for npcid in sorted(vendors):
            stock.append((npcid, row["patternid"], name))
    return scrolls, stock


def report(p, full=False):
    spells, links = p["spells"], p["links"]
    per = collections.Counter(c for c, _, _ in links)
    made_per = collections.Counter()
    for m in spells:
        for c in set(m["classes"]):
            made_per[c] += 1
    print(f"records to make {len(spells)}; class spells to add {len(links)}; lines {len(p['effects'])}; "
          f"scrolls {len(p['scrolls'])} sold by {len(p['stock'])} vendor rows")
    for cls, name in enumerate(CLASSES):
        print(f"  {name:3s} +{per[cls]:3d} class spells ({made_per[cls]} made records)")
    for why, n in p["skipped"].most_common():
        print(f"  not made: {n} {why}")
    if full:
        for m in spells:
            r = m["row"]
            print(f"  {r['spellid']} L{r['abilitylvl']:<2d} {r['spellname'][:30]:30s} scope {r['scope']} cast {r['casttime']} "
                  f"recast {r['recast']} power {r['power']:4d} icon {r['icon']:11d} [{'; '.join(m['notes'])}]")


def log(db, tbl, key, key2=None, note=None):
    db.execute("INSERT INTO eqoadata_spell_import_rows (tbl, key, key2, note) VALUES (?, ?, ?, ?)",
               (tbl, key, key2, note))


def apply(db, p):
    db.executescript(LOG_DDL)
    if db.execute("SELECT count(*) FROM eqoadata_spell_import_rows").fetchone()[0]:
        sys.exit("this database already holds an EQOA-Data spell import; --undo it first")
    scols, icols = columns(db, "spellpattern"), columns(db, "itempattern")
    db.execute("BEGIN IMMEDIATE")
    try:
        seqs = dict(db.execute("SELECT name, seq FROM sqlite_sequence "
                               "WHERE name IN ('spellpattern', 'itempattern', 'npcitem')").fetchall())
        for m in p["spells"]:
            r = m["row"]
            cols = [c for c in scols if c in r]
            db.execute(f"INSERT INTO main.spellpattern ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                       [r[c] for c in cols])
            log(db, "spellpattern", r["spellid"], note="; ".join(m["notes"]))
        for sid, line, source in p["effects"]:
            if db.execute("SELECT 1 FROM main.spelleffect WHERE spellid = ?", (sid,)).fetchone():
                continue
            db.execute("INSERT INTO main.spelleffect (spellid, line, fxdict, source) VALUES (?, ?, 0, ?)",
                       (sid, line, source))
            log(db, "spelleffect", sid)
        for cls, sid, note in p["links"]:
            db.execute("INSERT OR IGNORE INTO main.classspells (class, spellid, source) VALUES (?, ?, 'eqoa-data')",
                       (cls, sid))
            log(db, "classspells", cls, sid, note)
        for row in p["scrolls"]:
            cols = [c for c in icols if c in row]
            db.execute(f"INSERT INTO main.itempattern ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                       [row[c] for c in cols])
            log(db, "itempattern", row["patternid"], note=row["itemname"])
        for npcid, pid, name in p["stock"]:
            n = db.execute("SELECT coalesce(max(listnumber), -1) + 1 FROM main.npcitem WHERE npcid = ?",
                           (npcid,)).fetchone()[0]
            c = db.execute("INSERT INTO main.npcitem (npcid, stackleft, remainHP, remaincharge, patternid, equiploc, "
                           "location, listnumber) VALUES (?, 1, 0, 0, ?, -1, 1, ?)", (npcid, pid, n))
            log(db, "npcitem", c.lastrowid, note=name)
        now = datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")
        meta = {"applied_at": now, "sqlite_sequence": json.dumps(seqs),
                "counts": json.dumps({k: len(p[k]) for k in ("spells", "links", "effects", "scrolls", "stock")})}
        for k, v in meta.items():
            db.execute("INSERT OR REPLACE INTO eqoadata_spell_import_meta VALUES (?, ?)", (k, v))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db, force):
    if not db.execute("SELECT count(*) FROM sqlite_master WHERE name = 'eqoadata_spell_import_rows'").fetchone()[0]:
        sys.exit("no EQOA-Data spell import to undo here")
    held = {
        "spells": db.execute("SELECT count(*) FROM main.spells WHERE spellid IN (SELECT key FROM "
                             "eqoadata_spell_import_rows WHERE tbl = 'spellpattern')").fetchone()[0],
        "charinventory": db.execute("SELECT count(*) FROM main.charinventory WHERE patternid IN (SELECT key FROM "
                                    "eqoadata_spell_import_rows WHERE tbl = 'itempattern')").fetchone()[0],
    }
    if any(held.values()) and not force:
        sys.exit(f"characters hold made spells or scrolls ({held}); --force to take them out with the import")
    keys = {"spellpattern": "spellid", "spelleffect": "spellid", "itempattern": "patternid", "npcitem": "itemid"}
    db.execute("BEGIN IMMEDIATE")
    try:
        if force:
            db.execute("DELETE FROM main.spells WHERE spellid IN (SELECT key FROM eqoadata_spell_import_rows "
                       "WHERE tbl = 'spellpattern')")
            db.execute("DELETE FROM main.charinventory WHERE patternid IN (SELECT key FROM "
                       "eqoadata_spell_import_rows WHERE tbl = 'itempattern')")
        for tbl, key, key2 in db.execute(
                "SELECT tbl, key, key2 FROM eqoadata_spell_import_rows ORDER BY seq DESC").fetchall():
            if tbl == "classspells":
                db.execute("DELETE FROM main.classspells WHERE class = ? AND spellid = ?", (key, key2))
            else:
                db.execute(f"DELETE FROM main.{tbl} WHERE {keys[tbl]} = ?", (key,))
        r = db.execute("SELECT value FROM eqoadata_spell_import_meta WHERE key = 'sqlite_sequence'").fetchone()
        seqs = json.loads(r[0]) if r else {}
        for t in ("spellpattern", "itempattern", "npcitem"):
            if t in seqs:
                db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = ?", (seqs[t], t))
            else:
                db.execute("DELETE FROM sqlite_sequence WHERE name = ?", (t,))
        db.execute("DROP TABLE eqoadata_spell_import_rows")
        db.execute("DROP TABLE eqoadata_spell_import_meta")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print("undone")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--eqoa-data", help="EQOA-Data's checkout, or its Information/databases/spells directory")
    ap.add_argument("--apply", action="store_true", help="write it (one transaction, logged)")
    ap.add_argument("--undo", action="store_true", help="take back what --apply wrote")
    ap.add_argument("--force", action="store_true", help="--undo even when characters hold made spells or scrolls")
    ap.add_argument("--all-sources", action="store_true", help="also make masteries', master classes' and epics' records")
    ap.add_argument("--list", action="store_true", help="list every record it would make")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    if a.undo:
        undo(db, a.force)
        return
    if not a.eqoa_data:
        sys.exit("--eqoa-data is required")
    p = plan(db, load_lists(a.eqoa_data), a.all_sources)
    report(p, a.list)
    if not a.apply:
        print("(report only: --apply to write it)")
        return
    apply(db, p)
    print(f"applied to {a.db}")


if __name__ == "__main__":
    main()
