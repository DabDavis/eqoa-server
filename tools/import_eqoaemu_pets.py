#!/usr/bin/env python3
"""The pets of EQOAEmu's spell workbook into petstats (the user, 2026-11-01,
asked whether the workbook's combat numbers replace ours, pet hit points and
hits among them: "Workbook wins, re-fit").

EQOAEmu's EQOA_Spells_and_CM workbook (2020; the operator's copy, an .ods, or
its sheets as tab-separated files: neither is in this repository) gives every
pet of the game a row of its Pets sheet: its body (Sprite), weapons held,
level, type (Tank, Melee, Priest, Caster, Non-Combat), stances (Settings),
armour, stats and their caps, hit points, power, HoT and PoT, resists, weapon
damage and kinds, and up to four spells of its own (Spell1-4). The Effects
sheet's "Summon Pet" rows give each pet its summoning effect (PetID), and the
Spells sheet the summoning spell that carries it; the operator's spell map
(the workbook's SpellID to this server's spellid, the import's one map: an
existing spell by name, a new one 900000 + the hex id) names the spell here.

Each pet becomes the petstats row of its summoning spell (entity/petstats.go
says what the server does with it). Its body and weapons are the sheet's
DictIDs BYTE-REVERSED, as npcs.modelid and npcs.primary_weapon keep them (all
113 bodies are CHAR.ESF CSprites read so; the Fire Elemental's is the body
Sandstorm's recorded one wore); --models (cmd/modelbake's file of the disc's
bodies) and the NPCs wearing each are checked and said. Its spells become its
abilities, pinned by their ids here, each by what its effects do: a hit on
the enemy a nuke (an area one on its target alone: a pet casts no area
spell), a drained hit a life tap, a heal on itself, its owner or either (the
sheet's "Owner/Self"), a buff on its owner, a "Modify Hate" a taunt of hate
alone (no damage: the sheet gives none); a debuff or a snare has no pet kind
and is left out, said. Each on a clock of its spell's recast (2 s at least),
the first 1 s into a fight and the next 2 s after (ours). Its spells are new
to this server (900000 + their hex ids): the spell import makes their
records; an ability pinning one not there yet casts nothing, said.

A summoning spell with no pettemplate row of its name gets one where its
class's pets have a kind (ours, the user's of 2026-10-30): a magician's by its
element (pet earth, water, fire, air), a necromancer's "pet necromancer"
(swing_class 0, a warrior's round), an enchanter's "pet animation" (one hit a
round) but Grand Animation (pet.go's two hits); every other class's pets
swing as pet.go has them.

Each row written is a row edit as the GM panel writes one (gm_audit, as dab,
via "tool") and is logged in eqoaemu_pets_log for --undo, which puts back
each row still as written (one a GM changed since is left, and said). The
server reads petstats at its start, or at once at a GM's edit of the
templates or of petstats in the panel.

    tools/import_eqoaemu_pets.py --sheets <dir of the sheets' .tsv> --spellmap <map.tsv> --db <copy> [--models worlds/models.txt] --save-plan plan.json
    tools/import_eqoaemu_pets.py --sheet <workbook.ods> --spellmap <map.tsv> --db <copy> --save-plan plan.json
    tools/import_eqoaemu_pets.py --db eqoa.db --plan plan.json --apply     # one transaction, a copy of the database first
    tools/import_eqoaemu_pets.py --db eqoa.db --undo                       # every row still as written goes back
"""
import argparse
import collections
import csv
import datetime
import json
import os
import re
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
DEFAULT_DB = os.path.join(HERE, "..", "eqoa.db")
SOURCE = "eqoaemu"
WHO, VIA = "dab", "tool"
DETAIL = "EQOAEmu's spell workbook (2020): tools/import_eqoaemu_pets.py"
LOG = "eqoaemu_pets_log"
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,      -- petstats or pettemplate
    rowkey  TEXT NOT NULL,      -- the row's key (JSON)
    before  TEXT,               -- the row before (JSON), NULL: made here
    after   TEXT NOT NULL,      -- the row as written (JSON)
    audit   INTEGER NOT NULL    -- the gm_audit line written
)"""
# database.GMAuditSchema, as the server makes it
AUDIT_SCHEMA = """CREATE TABLE IF NOT EXISTS gm_audit (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	at TEXT NOT NULL, accountid INTEGER NOT NULL, username TEXT NOT NULL,
	via TEXT NOT NULL, ip TEXT NOT NULL DEFAULT '', action TEXT NOT NULL,
	tbl TEXT NOT NULL DEFAULT '', rowkey TEXT NOT NULL DEFAULT '',
	before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER
)"""
# database/petstats.go's petStatsSchema and pettemplates.go's, as the server
# makes them (an older database lacks petstats: made here)
PETSTATS_SCHEMA = """CREATE TABLE IF NOT EXISTS petstats (
		spellid         INTEGER PRIMARY KEY,
		sheet_pet_id    TEXT NOT NULL DEFAULT '',
		name            TEXT NOT NULL DEFAULT '',
		model           INTEGER NOT NULL DEFAULT 0,
		primary_model   INTEGER NOT NULL DEFAULT 0,
		secondary_model INTEGER NOT NULL DEFAULT 0,
		level           INTEGER NOT NULL DEFAULT 0,
		type            TEXT NOT NULL DEFAULT '',
		element         TEXT NOT NULL DEFAULT '',
		stances         TEXT NOT NULL DEFAULT '',
		ac              INTEGER NOT NULL DEFAULT 0,
		str             INTEGER NOT NULL DEFAULT 0,
		sta             INTEGER NOT NULL DEFAULT 0,
		agi             INTEGER NOT NULL DEFAULT 0,
		dex             INTEGER NOT NULL DEFAULT 0,
		wis             INTEGER NOT NULL DEFAULT 0,
		intelligence    INTEGER NOT NULL DEFAULT 0,
		cha             INTEGER NOT NULL DEFAULT 0,
		hp              INTEGER NOT NULL DEFAULT 0,
		power           INTEGER NOT NULL DEFAULT 0,
		hot             INTEGER NOT NULL DEFAULT 0,
		pot             INTEGER NOT NULL DEFAULT 0,
		ar              INTEGER NOT NULL DEFAULT 0,
		cr              INTEGER NOT NULL DEFAULT 0,
		lr              INTEGER NOT NULL DEFAULT 0,
		fr              INTEGER NOT NULL DEFAULT 0,
		pr              INTEGER NOT NULL DEFAULT 0,
		dr              INTEGER NOT NULL DEFAULT 0,
		str_max         INTEGER NOT NULL DEFAULT 0,
		sta_max         INTEGER NOT NULL DEFAULT 0,
		agi_max         INTEGER NOT NULL DEFAULT 0,
		dex_max         INTEGER NOT NULL DEFAULT 0,
		wis_max         INTEGER NOT NULL DEFAULT 0,
		int_max         INTEGER NOT NULL DEFAULT 0,
		cha_max         INTEGER NOT NULL DEFAULT 0,
		hp_factor       INTEGER NOT NULL DEFAULT 0,
		off_mod         INTEGER NOT NULL DEFAULT 0,
		def_mod         INTEGER NOT NULL DEFAULT 0,
		wpn1_dmg        INTEGER NOT NULL DEFAULT 0,
		wpn1_type       TEXT NOT NULL DEFAULT '',
		wpn2_dmg        INTEGER NOT NULL DEFAULT 0,
		wpn2_type       TEXT NOT NULL DEFAULT '',
		abilities       TEXT NOT NULL DEFAULT '[]',
		keep_template   INTEGER NOT NULL DEFAULT 0,
		note            TEXT NOT NULL DEFAULT '',
		source          TEXT NOT NULL DEFAULT 'gm'
	)"""
PETTEMPLATE_SCHEMA = """CREATE TABLE IF NOT EXISTS pettemplate (
		spell       TEXT NOT NULL,
		weapon      INTEGER NOT NULL DEFAULT 0,
		template    TEXT NOT NULL DEFAULT '',
		swing_class INTEGER NOT NULL DEFAULT -1,
		swing       INTEGER NOT NULL DEFAULT -1,
		round_ms    INTEGER NOT NULL DEFAULT 0,
		note        TEXT NOT NULL DEFAULT '',
		source      TEXT NOT NULL DEFAULT 'gm',
		PRIMARY KEY (spell, weapon)
	)"""

# the sheet's numeric columns, by petstats column
NUMS = {"level": "Pet Level", "ac": "AC", "str": "Base STR", "sta": "Base STA", "agi": "Base AGI", "dex": "Base DEX",
        "wis": "Base WIS", "intelligence": "Base INT", "cha": "Base CHA", "hp": "Base HP", "power": "Base POW",
        "hot": "HoT", "pot": "PoT", "ar": "AR", "cr": "CR", "lr": "LR", "fr": "FR", "pr": "PR", "dr": "DR",
        "str_max": "STR Max", "sta_max": "STA Max", "agi_max": "AGI Max", "dex_max": "DEX Max", "wis_max": "WIS Max",
        "int_max": "INT Max", "cha_max": "CHA Max", "hp_factor": "HP Factor", "off_mod": "OFF Mod",
        "def_mod": "DEF Mod", "wpn1_dmg": "WPN1 DMG", "wpn2_dmg": "WPN2 DMG"}
TYPES = {"tank", "melee", "priest", "caster", "non-combat"}
STANCES = ("passive", "defensive", "neutral", "aggressive")
WEAPONS = {"slash", "blunt", "pierce"}
# the magician's pets' kinds by their element, the classes' kinds (pettemplates.go's seed templates)
MAG_KIND = {"earth": "pet earth", "water": "pet water", "fire": "pet fire", "air": "pet air"}
GRAND_ANIMATION = "grand animation"
FIRST, STAGGER, MIN_EVERY = 1.0, 2.0, 2.0
BELOW_OWNER, BELOW_SELF = 60, 50  # the water pet's thresholds (pettemplates.go)
BUFF_STATS = ("STR", "STA", "AGI", "DEX", "WIS", "INT", "CHA", "HPMAX", "POWMAX", "PoT", "HoT", "AC",
              "PR", "DR", "FR", "CR", "LR", "AR")


def fold(s):
    return re.sub(r"\s+", " ", (s or "").strip().lower())


def key(name):
    """database.TemplateKey."""
    return (name or "").strip().lower()


def rev32(hexid):
    """The sheet's DictID byte-reversed (npcs.modelid's and the items' order), or 0."""
    t = (hexid or "").strip()
    t = t[2:] if t[:2].lower() == "0x" else t
    if not re.fullmatch(r"[0-9a-fA-F]{8}", t):
        return 0
    return int.from_bytes(int(t, 16).to_bytes(4, "big"), "little")


def num(v):
    v = (v or "").strip()
    if not v:
        return 0
    try:
        return int(round(float(v)))
    except ValueError:
        return None


# ---- the workbook --------------------------------------------------------------------

def tsv_rows(path):
    """A sheet as tab-separated text (a cell's line breaks " / "), its rows as dicts by its header."""
    with open(path, encoding="utf-8", newline="") as f:
        rows = list(csv.reader(f, delimiter="\t", quoting=csv.QUOTE_NONE))
    head = rows[0]
    return [dict(zip(head, r + [""] * (len(head) - len(r)))) for r in rows[1:] if any(c.strip() for c in r)]


def read_sheets(a):
    """The Pets, Spells and Effects sheets, from the .tsv files of a folder or the .ods itself."""
    if a.sheets:
        names = {"Pets": "07-Pets.tsv", "Spells": "02-Spells.tsv", "Effects": "03-Effects.tsv"}
        out = {}
        for sheet, fn in names.items():
            p = os.path.join(a.sheets, fn)
            if not os.path.exists(p):
                cands = [x for x in os.listdir(a.sheets) if x.lower().endswith(f"{sheet.lower()}.tsv")]
                if len(cands) != 1:
                    sys.exit(f"{a.sheets}: no {fn}")
                p = os.path.join(a.sheets, cands[0])
            out[sheet] = tsv_rows(p)
        return out
    sys.path.insert(0, HERE)
    from fill_spell_visuals_eqoaemu import ods_rows  # the workbook's reader, stdlib only
    found = {}
    for sheet in ("Pets", "Spells", "Effects"):
        found[sheet] = [{k: (v or "").replace("\n", " / ") for k, v in r.items()} for r in ods_rows(a.sheet, sheet)]
    return found


def read_spellmap(path):
    """The workbook's SpellID (hex, upper case) to this server's spellid."""
    out = {}
    with open(path, encoding="utf-8") as f:
        for line in f:
            if line.startswith("#") or not line.strip():
                continue
            cols = line.rstrip("\n").split("\t")
            if cols[0] == "sheet_id" or len(cols) < 2:
                continue
            try:
                out[cols[0].strip().upper()] = int(cols[1])
            except ValueError:
                continue
    return out


def read_models(path):
    """cmd/modelbake's file: the disc's CHAR.ESF bodies (DictIDs as npcs.modelid keeps them)."""
    out = set()
    with open(path, encoding="utf-8") as f:
        for line in f:
            if line.startswith("#") or not line.strip():
                continue
            out.add(int(line.split()[0], 16))
    return out


# ---- the plan ------------------------------------------------------------------------

def effect_rows(spell, effects):
    return [effects[e] for e in (spell.get(f"EffectID{i}", "").strip().upper() for i in range(1, 5)) if e in effects]


def ability_of(spell, effects, spellid, i):
    """A pet spell as an ability (database.TemplateAbility's JSON), or (None, why not)."""
    effs = effect_rows(spell, effects)
    if not effs:
        return None, "no effect row"
    e = effs[0]
    typ, scope, target = fold(e.get("TYPE")), fold(e.get("Scope")), fold(e.get("Target Type"))
    hp1 = num(e.get("HP1")) or 0
    every = max(float(num(spell.get("Recast")) or 0), MIN_EVERY)
    a = {"every": every, "first": FIRST + STAGGER * i}
    if typ == "modify hate":
        a.update(kind="taunt", hits=[[1, 0, 0]])
        return a, ""
    if typ in ("dmg/heal", "drain") and hp1 < 0 and target == "enemy":
        a.update(kind="lifetap" if typ == "drain" else "nuke", spell_id=spellid)
        return a, ("an area spell, cast on its target alone" if "aoe" in scope else "")
    if typ == "dmg/heal" and hp1 > 0:
        on = {"owner": "owner", "owner/self": "either"}.get(target, "self" if scope == "self" else None)
        if on is None:
            return None, f"a heal on {e.get('Target Type')!r}"
        a.update(kind="heal", spell_id=spellid, target=on, below=BELOW_SELF if on == "self" else BELOW_OWNER)
        return a, ""
    # a lasting raise of a stat (the familiars' "Heal" is a Dmg/Heal of max HP for 600 s): a buff
    lasting = (num(e.get("Duration")) or 0) > 0 and any((num(e.get(c)) or 0) > 0 for c in BUFF_STATS)
    if typ in ("statistic change", "dmg/heal") and hp1 == 0 and lasting:
        if target == "owner" or scope == "owner":
            a.update(kind="buff", spell_id=spellid, target="owner")
            return a, ""
        if scope == "self":
            a.update(kind="buff", spell_id=spellid, target="self")
            return a, ""
    if typ == "drain" and hp1 == 0:
        return None, "a debuff of the enemy's stats (no pet ability kind; a creature here has no such stats)"
    return None, f"{e.get('TYPE')} on {e.get('Scope')} {e.get('Target Type')}: no pet ability kind"


def plan(db, sheets, smap, models=None):
    """The rows to write and what was found; every count and doubt said."""
    spells = {s["SpellID"].strip().upper(): s for s in sheets["Spells"] if s.get("SpellID", "").strip()}
    effects = {e["EffectID"].strip().upper(): e for e in sheets["Effects"] if e.get("EffectID", "").strip()}
    summoned = {}  # PetID -> its summoning effect
    for e in effects.values():
        if e.get("PetID", "").strip():
            summoned.setdefault(e["PetID"].strip().upper(), e["EffectID"].strip().upper())
    summoning = collections.defaultdict(list)  # EffectID -> the spells carrying it
    for s in spells.values():
        for e in effect_rows(s, effects):
            summoning[e["EffectID"].strip().upper()].append(s)
    ours = {sid: (lvl, name) for sid, lvl, name in db.execute("SELECT spellid, abilitylvl, spellname FROM spellpattern")}
    worn = collections.Counter(m & 0xFFFFFFFF for (m,) in db.execute("SELECT modelid FROM npcs WHERE modelid IS NOT NULL"))
    items = set(m & 0xFFFFFFFF for (m,) in db.execute("SELECT model FROM itempattern WHERE model IS NOT NULL"))
    held = set()
    for a_, b_ in db.execute("SELECT primary_weapon, secondary_weapon FROM npcs"):
        held.update(x & 0xFFFFFFFF for x in (a_, b_) if x)
    pt_rows = {(s, w) for s, w in db.execute("SELECT spell, weapon FROM pettemplate")} if table(db, "pettemplate") else set()
    templates = {n for (n,) in db.execute("SELECT name FROM classtemplates")} if table(db, "classtemplates") else set()

    rows, pts, notes, counts = [], [], [], collections.Counter()
    seen_spell = {}
    for p in sheets["Pets"]:
        pid = p.get("PetID", "").strip().upper()
        if not pid:
            continue
        counts["pets in the sheet"] += 1
        eff = summoned.get(pid)
        carriers = summoning.get(eff, []) if eff else []
        if len(carriers) != 1:
            counts["pets with no single summoning spell (left)"] += 1
            notes.append(f"pet {pid} {p.get('PetName')!r}: {len(carriers)} summoning spells")
            continue
        sp = carriers[0]
        hexid = sp["SpellID"].strip().upper()
        sid = smap.get(hexid)
        if sid is None:
            counts["summoning spells not in the spell map (left)"] += 1
            notes.append(f"pet {pid}: its spell {hexid} {sp.get('Name')!r} is not in the spell map")
            continue
        if sid in seen_spell:
            counts["summoning spells mapped twice (left)"] += 1
            notes.append(f"pet {pid}: spell {sid} is pet {seen_spell[sid]}'s too")
            continue
        seen_spell[sid] = pid
        have = ours.get(sid)
        if have is None:
            counts["summoning spells not in the database yet (the spell import makes them)"] += 1
        else:
            counts["summoning spells in the database"] += 1
            if num(sp.get("Level")) and have[0] != num(sp.get("Level")):
                counts["summoning spells whose level here differs from the sheet's"] += 1
                notes.append(f"pet {pid} {p.get('PetName')!r}: spell {sid} {have[1]!r} is level {have[0]} here, "
                             f"the sheet's {hexid} {sp.get('Name')!r} level {sp.get('Level')}: the spell map?")
        row = {"spellid": sid, "sheet_pet_id": pid, "name": (p.get("PetName") or "").strip(), "source": SOURCE,
               "keep_template": 0}
        bad = []
        for col, head in NUMS.items():
            v = num(p.get(head))
            if v is None or v < 0:
                bad.append(f"{head} {p.get(head)!r}")
                v = 0
            row[col] = v
        typ = fold(p.get("Type"))
        row["type"] = typ if typ in TYPES else ""
        if typ and typ not in TYPES:
            bad.append(f"Type {p.get('Type')!r}")
        row["element"] = fold(p.get("Element"))
        st = [s for s in (fold(x) for x in re.split(r"[,;/]", p.get("Settings") or "")) if s]
        if any(s not in STANCES for s in st):
            bad.append(f"Settings {p.get('Settings')!r}")
            st = []
        row["stances"] = "" if set(st) == set(STANCES) else ", ".join(s for s in STANCES if s in st)
        for n in (1, 2):
            w = fold(p.get(f"WPN{n} Type"))
            row[f"wpn{n}_type"] = w if w in WEAPONS else ""
            if w and w not in WEAPONS:
                bad.append(f"WPN{n} Type {p.get(f'WPN{n} Type')!r}")
        row["model"] = rev32(p.get("Sprite"))
        if not row["model"]:
            bad.append(f"Sprite {p.get('Sprite')!r}")
        if worn.get(row["model"]):
            counts["bodies worn by NPCs here"] += 1
        else:
            counts["bodies no NPC here wears"] += 1
        if models is not None:
            counts["bodies on the disc" if row["model"] in models else "bodies NOT on the disc"] += 1
            if row["model"] and row["model"] not in models:
                notes.append(f"pet {pid}: body {row['model']:#010x} is no CHAR.ESF body of the disc")
        for n, col in ((1, "primary_model"), (2, "secondary_model")):
            row[col] = rev32(p.get(f"WeaponSpriteID{n}"))
            if row[col]:
                counts["weapon models"] += 1
                if row[col] not in items and row[col] not in held:
                    counts["weapon models no item or NPC here has"] += 1
                    notes.append(f"pet {pid}: weapon {row[col]:#010x} is no item's or NPC's model here")
        abilities, ab_notes = [], []
        for i, k in enumerate(("Spell1", "Spell2", "Spell3", "Spell4")):
            ref = (p.get(k) or "").strip().upper()
            if not ref:
                continue
            counts["pet spells named"] += 1
            s = spells.get(ref)
            if s is None:
                counts["pet spells not in the sheet (left)"] += 1
                ab_notes.append(f"{ref}: not in the Spells sheet")
                continue
            psid = smap.get(ref)
            if psid is None:
                counts["pet spells not in the spell map (left)"] += 1
                ab_notes.append(f"{ref} {s.get('Name')!r}: not in the spell map")
                continue
            a, why = ability_of(s, effects, psid, len(abilities))
            if a is None:
                counts["pet spells of no pet ability kind (left)"] += 1
                ab_notes.append(f"{s.get('Name')!r}: {why}")
                continue
            counts[f"abilities: {a['kind']}" + (f" ({a['target']})" if a.get("target") else "")] += 1
            if "spell_id" in a:
                counts["abilities pinning a spell in the database" if psid in ours
                       else "abilities pinning a spell not in the database yet (the spell import makes it)"] += 1
            if why:
                ab_notes.append(f"{s.get('Name')!r}: {why}")
            abilities.append(a)
        row["abilities"] = json.dumps(abilities, separators=(",", ":"))
        spell_names = [spells[r.strip().upper()]["Name"] for r in (p.get(k) or "" for k in ("Spell1", "Spell2", "Spell3", "Spell4"))
                       if r.strip() and r.strip().upper() in spells]
        row["note"] = (f"EQOAEmu's workbook (2020), pet {pid}, summoned by {sp.get('Name')!r}"
                       + (f"; its spells {', '.join(spell_names)}" if spell_names else "")
                       + (f"; {'; '.join(ab_notes)}" if ab_notes else ""))
        if bad:
            counts["pets with a cell not read (0 or empty taken)"] += 1
            notes.append(f"pet {pid}: " + ", ".join(bad))
        rows.append(row)
        counts["petstats rows"] += 1
        # its pettemplate row, where its class's pets have a kind and its spell none of its name
        name = have[1] if have else (sp.get("Name") or "")
        k = key(name)
        cls = (sp.get("CLASS") or "").split()
        tmpl, swing_class, why = None, -1, ""
        if cls == ["MAG"]:
            tmpl, why = MAG_KIND.get(row["element"]), f"a magician's {row['element']} pet"
        elif cls == ["NEC"]:
            tmpl, swing_class, why = "pet necromancer", 0, "a necromancer's pet"
        elif cls == ["ENC"] and key(row["name"]) != GRAND_ANIMATION:
            tmpl, why = "pet animation", "an enchanter's pet"
        if tmpl and k and (k, 0) not in pt_rows and (k, 1) not in pt_rows:
            if templates and tmpl not in templates:
                notes.append(f"pet {pid}: no classtemplates row {tmpl!r} for its pettemplate row")
            else:
                pts.append({"spell": k, "weapon": 0, "template": tmpl, "swing_class": swing_class, "swing": -1,
                            "round_ms": 0, "note": f"{why}, EQOAEmu's workbook pet {pid} (tools/import_eqoaemu_pets.py)",
                            "source": SOURCE})
                pt_rows.add((k, 0))
                counts["pettemplate rows to add"] += 1
    return rows, pts, notes, counts


# ---- writing it ----------------------------------------------------------------------

def table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def go_json(m):
    """A row as Go's json.Marshal writes a map (keys sorted, <>& escaped)."""
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


KEYS = {"petstats": ("spellid",), "pettemplate": ("spell", "weapon")}


def row_of(db, tbl, k):
    where = " AND ".join(f"{c} = ?" for c in KEYS[tbl])
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", tuple(k[c] for c in KEYS[tbl]))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def audit(db, action, tbl, k, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(k),
                      None if before is None else go_json(before), None if after is None else go_json(after), detail))
    return cur.lastrowid


def write_row(db, tbl, row):
    cols = list(row)
    db.execute(f"INSERT OR REPLACE INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
               tuple(row[c] for c in cols))


def apply(db, rows, pts):
    """Every planned row in one transaction: a row a GM made or changed (source not this tool's) is left; returns
    (written, unchanged, left)."""
    written = same = left = 0
    db.execute("BEGIN IMMEDIATE")
    try:
        for s in (AUDIT_SCHEMA, LOG_SCHEMA, PETSTATS_SCHEMA, PETTEMPLATE_SCHEMA):
            db.execute(s)
        for tbl, entries in (("petstats", rows), ("pettemplate", pts)):
            for row in entries:
                k = {c: row[c] for c in KEYS[tbl]}
                before = row_of(db, tbl, k)
                if before is not None and before.get("source") not in (SOURCE, "seed"):
                    left += 1
                    continue
                if before is not None and all(before.get(c) == v for c, v in row.items()):
                    same += 1
                    continue
                write_row(db, tbl, row)
                after = row_of(db, tbl, k)
                a = audit(db, "insert" if before is None else "update", tbl, k, before, after, DETAIL)
                db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                           (tbl, go_json(k), None if before is None else go_json(before), go_json(after), a))
                written += 1
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return written, same, left


def undo(db):
    """Every logged row still as written back as it was (the last first); returns (back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    db.execute("BEGIN IMMEDIATE")
    try:
        for _, tbl, rk, before, after, _ in db.execute(f"SELECT * FROM {LOG} ORDER BY id DESC").fetchall():
            k = json.loads(rk)
            now = row_of(db, tbl, k)
            if now is None or go_json(now) != after:
                left += 1
                print(f"  left: {tbl} {rk}, changed since it was written")
                continue
            if before is None:
                where = " AND ".join(f"{c} = ?" for c in KEYS[tbl])
                db.execute(f"DELETE FROM {tbl} WHERE {where}", tuple(k[c] for c in KEYS[tbl]))
                audit(db, "delete", tbl, k, now, None, "undo: " + DETAIL)
            else:
                write_row(db, tbl, json.loads(before))
                audit(db, "update", tbl, k, now, row_of(db, tbl, k), "undo: " + DETAIL)
            back += 1
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return back, left


def backup(path):
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-eqoaemu-pets"
    src, out = sqlite3.connect(path, timeout=30), sqlite3.connect(dst)
    try:
        src.backup(out)
    finally:
        out.close()
        src.close()
    return dst


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--sheet", help="EQOAEmu's workbook (.ods), the operator's copy")
    ap.add_argument("--sheets", help="or a folder of its sheets as .tsv (07-Pets.tsv, 02-Spells.tsv, 03-Effects.tsv)")
    ap.add_argument("--spellmap", help="the workbook's SpellID -> spellid map (.tsv: sheet_id, spellid, ...)")
    ap.add_argument("--models", help="cmd/modelbake's file of the disc's bodies (worlds/models.txt), to check the bodies")
    ap.add_argument("--save-plan", metavar="FILE", help="write the plan (JSON) and stop")
    ap.add_argument("--plan", metavar="FILE", help="a plan --save-plan wrote, instead of the sheets and the map")
    ap.add_argument("--verbose", action="store_true", help="every doubt, not the first twenty")
    ap.add_argument("--no-backup", action="store_true")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    if (a.apply or a.undo) and not a.no_backup:
        print(f"copy: {backup(a.db)}")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        back, left = undo(db)
        print(f"put back {back}; left {left} changed since")
        return
    if a.plan:
        with open(a.plan, encoding="utf-8") as f:
            p = json.load(f)
        rows, pts, notes, counts = p["petstats"], p["pettemplate"], p.get("notes", []), collections.Counter(p.get("counts", {}))
    else:
        if not ((a.sheet or a.sheets) and a.spellmap):
            sys.exit("--sheet or --sheets, and --spellmap (or --plan)")
        rows, pts, notes, counts = plan(db, read_sheets(a), read_spellmap(a.spellmap),
                                        read_models(a.models) if a.models else None)
    for k, n in sorted(counts.items()):
        print(f"  {k}: {n}")
    for n in notes if a.verbose else notes[:20]:
        print(f"  - {n}")
    if len(notes) > 20 and not a.verbose:
        print(f"  ... {len(notes) - 20} more (--verbose)")
    if a.save_plan:
        with open(a.save_plan, "w", encoding="utf-8") as f:
            json.dump({"made": datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds"),
                       "petstats": rows, "pettemplate": pts, "notes": notes, "counts": counts}, f, indent=1, ensure_ascii=False)
        print(f"plan: {a.save_plan} ({len(rows)} pets, {len(pts)} pettemplate rows)")
        return
    if a.apply:
        written, same, left = apply(db, rows, pts)
        print(f"wrote {written}; {same} already so; {left} a GM's, left")
    else:
        print(f"{len(rows)} petstats rows, {len(pts)} pettemplate rows: --apply to write")


if __name__ == "__main__":
    main()
