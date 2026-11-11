#!/usr/bin/env python3
"""The epic quests reward Ascended weapons and spells (petition 1629, the GM Jibrish, 2026-11-11: "For all epic quests
have the rewards give out an Ascended version of their epic weapon and spells. All spells work exactly the same but do
10% more, eg 10% more damage, 10% more healing, give 10% more mana, absorb 10% more damage and so on. The epic weapons
and robes have 10% more stats."). Run it on each shard's database; everything is found by name and content, never by
an id of the other shard's.

The epic quests (the import's keys, quests.source):

  - the level 60 epics' rewards, "The <Class>'s Epic Reward" (60/<group>/reward-<class>): the ability picked (step 1's
    options) and the weapon, robe, tunic, totem or chestplate picked (step 2's options; the paladin's Sword of Virtue
    its step's own reward). Both are made Ascended.
  - the level 49 epic weapons, "The <Class>'s Reward, Level 49" (30,40,49/<group>/L49-<class>): the weapon (or the
    alchemist's robe or gloves) of each option. Their abilities are the class's level 49 PATH spells, which the Spell
    Archivist hands out from these same options (handlers/archivistpaths.go) and a rehabilitator swaps: they stay as
    they are, as does every level 30 and 40 path reward.

Items: an Ascended record by the server's own rule (database/ascended.go, as tools/ascend_boss_loot.py makes one, whose
constants this file imports): "Ascended <name>", each stat, AC, resist, hit point and power maximum and tick and a
weapon's damage above 0 a tenth more (times 110 / 100 in whole numbers, half up: 4 stays 4, 5 is 6, 15 is 17; a
penalty as it is), its durability, price, look, icon, slot, level, classes, races, NO TRADE and Lore its base's, never
NO RENT; listed in itemascended, so the server makes it its base again at every start (its description is its base's:
that is the rule's), and a weapon's proc (read by its base's name, entity/weaponproc.go) does a tenth more. A record
that has an Ascended record already (a tuned boss's drop) is pointed at, not made again. A token (the Rose of
Rehabilitation) is no gear and stays.

Spells (the 60 epics' alone): a spellpattern row of its own, "Ascended <name>", its power cost, cast time, recast,
range, icon, level and scope its base's, its description "Ascended (+10%): " before its base's (CLIENT shows 127
characters); its spelleffect row with its effects (the workbook JSON, entity/spelljson.go) each a tenth more:

  - hp, hp2, backstab, hp_tick, power, power_tick, stat (every stat but speed), drain, drain_group, hate, mark_heal,
    off_mod, def_mod, hp_factor, melee_debuff: the amount, its stat scale (scale_pct) and its level scale (level_pct),
    each a tenth more as an item's figures are (the sign kept: a debuff, a drain and a hate taken off do a tenth more
    too), and a hit's own hate (hate);
  - invulnerable and mitigation: the most it takes (mitigation.max; "no limit" stays); its percent of each hit stays;
  - a proc's and a defensive proc's own effects, by the same rule;
  - create_item: the potion, wine or stone it makes is an Ascended copy of that item (below);
  - summon: the pet is its petstats row copied for the Ascended spell, its stats, AC, resists, hit points, power,
    ticks, caps, mods and weapon damage a tenth more, its abilities' spells Ascended copies (made here, not rewarded);
  - LEFT as they are: the caster's own price (an effect on the caster below 0: Celedonicus' Grand Finale's -5000
    power), speed (a run buff, a snare, the barriers' root), a stat or power SET to a figure (stat_set, an absolute
    power: the rangers' power ceilings), percents of a hit (mitigation.pct), durations, recasts, costs, radii,
    resists, charm caps, cures, illusions, gates.

Its human-readable line (spelleffect.line, the spell's whole behaviour when it has no JSON) gets the same: each "by N"
(and "to M"), "up to N" and "N% based on" a tenth more, but in a part that is the caster's price ("Decrease ... to
self", a Self line's Decrease but of Hate) or a speed. Its spellarea row is copied, and its classspells rows (who it is
for: the spellfx page, the GM panel) with source "eqoa-data epic", which no class grant gives (grantsource.go).
Ascended spells are listed in table spellascended (spellid, base), as itemascended lists the records.

Consumables a 60 epic spell makes (create_item): "Ascended <name>", a copy of the item the spell's words make (the
server's ItemPatternByName: of that name, not a scroll, one with a use first, the lowest id) in the Ascended records'
ids, its figures by the item rule, its use (itemeffect line and target_line) by the line rule above (a drunk potion's
cost to its drinker kept: the Superior Mana Stone's 800 hit points), its foodeffect row copied. Not in itemascended: the
server's start would make it its base again by the gear's line rule.

Quests: each option's and step's item and spell ids pointed at their Ascended ones, and its answer worded with
"Ascended" before the thing's name ("The Ascended Staff of Divinity", "Withering Wind and the Ascended Spear of Fate";
"Ascended " before the answer, after a leading "The ", where the answer does not spell the name), at most 63
characters (CLIENT's answer). What a character already holds is not touched (--swap-report lists them).

Every row written is a gm_audit line (via "tool") and logged in ascended_epics_log for --undo, which takes back each
row still as written (a record or spell a character holds now stays, and is said) and the counters. A second --apply
with nothing new writes nothing. Report only by default; --apply and --undo copy the database first unless
--no-backup. The quests, spells and pets are read at the server's start: RESTART after --apply.

    python3 -E -s tools/ascended_epics_20261111.py --db eqoa.db [--apply | --undo | --swap-report]
"""
import argparse
import datetime
import json
import os
import re
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import ascend_boss_loot as abl  # noqa: E402  the server's Ascended item rule

PREFIX = abl.PREFIX
MAX_ITEM_NAME = abl.MAX_NAME        # VIItem's m_name: 63 characters
MAX_SPELL_NAME = 63                 # VIAbility's m_name[64] (decomp game.h)
MAX_SPELL_DESC = 127                # VIAbility's m_desc[128]
MAX_ANSWER = 63                     # handlers/quests.go questAnswerChars
DESC_PREFIX = "Ascended (+10%): "
NO_LIMIT = 100000                   # entity/spelljson.go mitigateNoLimit
CLASS_SOURCE = "eqoa-data epic"     # database.GotBy "epic": no class grant gives it
WHO, VIA = "dab's sessions", "tool"
DETAIL = "petition 1629 (Jibrish): the epic quests reward Ascended weapons and spells; tools/ascended_epics_20261111.py"
TOOL = "tools/ascended_epics_20261111.py"
LOG = "ascended_epics_log"

RE_EPIC = re.compile(r"^EQOA-Data 60/[^/ ]+/reward-[a-z]+ ")
RE_L49 = re.compile(r"^EQOA-Data 30,40,49/[^/ ]+/L49-[a-z]+ ")

SPELL_MAP_SCHEMA = """CREATE TABLE IF NOT EXISTS spellascended (
	spellid INTEGER PRIMARY KEY,
	base    INTEGER NOT NULL UNIQUE,
	made    TEXT NOT NULL DEFAULT '',
	source  TEXT NOT NULL DEFAULT ''
)"""
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    run     TEXT NOT NULL,
    tbl     TEXT NOT NULL,      -- the table; sqlite_sequence a counter, sqlite_master a table made here
    ref     TEXT NOT NULL,      -- the row's key (JSON)
    before  TEXT,               -- the row before (JSON); NULL: made here
    after   TEXT,               -- the row as written (JSON)
    audit   INTEGER
)"""
KEYS = {"itempattern": ("patternid",), "itemeffect": ("patternid",), "foodeffect": ("patternid",),
        "itemascended": ("patternid",), "spellpattern": ("spellid",), "spelleffect": ("spellid",),
        "spellarea": ("spellid",), "classspells": ("class", "spellid"), "petstats": ("spellid",),
        "spellascended": ("spellid",), "queststeps": ("questid", "step")}
OWN = set(KEYS) | {LOG, "ascended_loot_log"}

# the effect kinds whose figures are a tenth more (entity/spelljson.go readEffectJSON)
SCALED_KINDS = {"hp", "hp2", "backstab", "hp_tick", "power", "power_tick", "stat", "drain", "drain_group", "hate",
                "mark_heal", "off_mod", "def_mod", "hp_factor", "melee_debuff"}
PET_SCALED = ("ac", "str", "sta", "agi", "dex", "wis", "intelligence", "cha", "hp", "power", "hot", "pot", "ar", "cr",
              "lr", "fr", "pr", "dr", "str_max", "sta_max", "agi_max", "dex_max", "wis_max", "int_max", "cha_max",
              "hp_factor", "off_mod", "def_mod", "wpn1_dmg", "wpn2_dmg")


# ---- the rule ------------------------------------------------------------------------------------------------------

def up(v):
    """A figure a tenth more, the sign kept: whole numbers as database.AscendedFigure rounds them (half up on the
    magnitude), a fraction to two places; 0 stays 0."""
    if v is None or isinstance(v, bool) or v == 0:
        return v
    neg = v < 0
    m = -v if neg else v
    if isinstance(m, int) or float(m).is_integer():
        r = abl.figure(int(m))
        r = r if isinstance(v, int) else float(r)
    else:
        r = round(m * 1.1 + 1e-9, 2)
    return -r if neg else r


def scale_effect(e, parent_scope, consumables, notes):
    """One workbook effect (and its procs) a tenth more, in place; what is left as it is goes to notes."""
    kind = str(e.get("kind") or "").strip().lower()
    scope = str(e.get("scope") or parent_scope or "").strip().lower()
    amount = e.get("amount") or 0
    price = scope == "self" and isinstance(amount, (int, float)) and amount < 0
    stat = str(e.get("stat") or "").strip().lower()
    if kind in SCALED_KINDS:
        if price:
            notes.append(f"{kind} {amount} on the caster (its price) as it is")
        elif kind in ("stat",) and stat == "speed":
            notes.append(f"speed {amount} as it is")
        elif kind == "power" and e.get("absolute"):
            notes.append(f"power set to {amount} as it is")
        else:
            for k in ("amount", "scale_pct", "level_pct"):
                if e.get(k):
                    e[k] = up(e[k])
    elif kind == "stat_set":
        notes.append(f"{e.get('stat')} set to {amount} as it is")
    elif kind == "speed":
        notes.append(f"speed {amount}{' (set)' if e.get('absolute') else ''} as it is")
    elif kind in ("invulnerable", "mitigation"):
        m = e.get("mitigation")
        if isinstance(m, dict) and m.get("max") and abs(m["max"]) < NO_LIMIT:
            m["max"] = up(m["max"])
    elif kind == "create_item":
        it = e.get("item") or {}
        name = str(it.get("name") or "").strip()
        if name.lower() in consumables:
            c = consumables[name.lower()]
            it["name"] = c["name"]
            if "patternid" in it:
                it["patternid"] = c["id"]
    elif kind in ("proc", "def_proc", "summon"):
        pass  # its procs below; a summon's pet is its petstats row (make_pet)
    elif kind:
        notes.append(f"{kind} as it is")
    if e.get("hate") and kind != "hate" and not price:
        e["hate"] = up(e["hate"])
    for p in e.get("procs") or []:
        scale_effect(p, "target", consumables, notes)


def has_price(text):
    """Whether a spell's effects JSON (or its want of one) makes a Self line's Decrease the caster's price."""
    if not (text or "").strip():
        return True
    notes = []
    scale_effects(text, {}, notes)
    return any("(its price)" in n for n in notes)


def scale_effects(text, consumables, notes=None):
    """A spell's effects JSON a tenth more: (the JSON, notes). The keys keep their order."""
    j = json.loads(text)
    notes = [] if notes is None else notes
    for e in j.get("effects") or []:
        scale_effect(e, j.get("scope"), consumables, notes)
    return json.dumps(j, ensure_ascii=False, separators=(",", ":")), notes


RE_BY = re.compile(r"\b([bB]y )(\d+)(?:( to )(\d+))?")
RE_UPTO = re.compile(r"\b([uU]p to )(\d+)")
RE_PCT = re.compile(r"(\d+)(% based on)")


def scale_line(line, renames=(), self_price=True):
    """A spelleffect or itemeffect line a tenth more (the doc's line rule); renames: (old, new) item names it says.
    self_price: a Self line's Decrease (but of Hate) is the caster's price (an item's use, a spell with no effects JSON
    or one whose JSON has a price); else it is what the spell's proc does (Graal's Final Strike's "Self, Decrease
    Hitpoints by 400", its proc's hit) and a tenth more as the JSON's is."""
    if not isinstance(line, str) or not line:
        return line
    for old, new in renames:
        line = re.sub(re.escape(old), new, line, flags=re.I) if new.lower() not in line.lower() else line
    parts = line.split(",")
    who = parts[0].strip().lower()
    out = [parts[0]]
    for p in parts[1:]:
        low = p.strip().lower()
        price = ("decrease" in low and "to self" in low) or (self_price and who.startswith("self")
                                                              and low.startswith("decrease") and "hate" not in low)
        if price or "speed" in low or "movement" in low:
            out.append(p)
            continue
        p = RE_BY.sub(lambda m: m.group(1) + str(up(int(m.group(2))))
                      + (m.group(3) + str(up(int(m.group(4)))) if m.group(4) else ""), p)
        p = RE_UPTO.sub(lambda m: m.group(1) + str(up(int(m.group(2)))), p)
        p = RE_PCT.sub(lambda m: str(up(int(m.group(1)))) + m.group(2), p)
        out.append(p)
    return ",".join(out)


def ascended_desc(desc):
    d = DESC_PREFIX + (desc or "").strip()
    if len(d) <= MAX_SPELL_DESC:
        return d
    cut = d[:MAX_SPELL_DESC - 3]
    if " " in cut[len(DESC_PREFIX):]:
        cut = cut[:cut.rfind(" ")]
    return cut.rstrip(" ,.;") + "..."


def ascend_answer(answer, names):
    """An option's words with "Ascended" before each changed thing's name it spells, else before the answer."""
    out, done = answer, False
    for n in sorted({n for n in names if n}, key=len, reverse=True):
        i = out.lower().find(n.lower())
        if i < 0 or out[:i].endswith(PREFIX):
            continue
        out = out[:i] + PREFIX + out[i:]
        done = True
    if not done:
        out = ("The " + PREFIX + out[4:]) if out.startswith("The ") else PREFIX + out
    return out


# ---- reading ---------------------------------------------------------------------------------------------------------

has_table, columns, dict_rows, one, go_json = abl.has_table, abl.columns, abl.dict_rows, abl.one, abl.go_json


def row_of(db, tbl, ref):
    where = " AND ".join(f"{c} = ?" for c in KEYS[tbl])
    return one(db, f"SELECT * FROM {tbl} WHERE {where}", tuple(ref[c] for c in KEYS[tbl]))


def counts(text):
    """A list of ids, "718029*2, 8373": [(id, the token as written)]."""
    out = []
    for tok in (text or "").split(","):
        m = re.match(r"^\s*(\d+)", tok)
        if m:
            out.append((int(m.group(1)), tok))
    return out


def remap(text, mapping):
    """The list with each id in mapping replaced, every other character as it was."""
    if not text:
        return text
    toks = text.split(",")
    for i, tok in enumerate(toks):
        m = re.match(r"^(\s*)(\d+)(.*)$", tok, re.S)
        if m and int(m.group(2)) in mapping:
            toks[i] = m.group(1) + str(mapping[int(m.group(2))]) + m.group(3)
    return ",".join(toks)


def epic_quests(db):
    """The epic quests: (questid, name, kind) with kind "60" or "49"."""
    out = []
    for r in dict_rows(db, "SELECT questid, name, source FROM quests ORDER BY questid"):
        src = (r["source"] or "") + " "
        if RE_EPIC.match(src) and r["name"].strip().endswith("Epic Reward"):
            out.append((r["questid"], r["name"], "60"))
        elif RE_L49.match(src) and r["name"].strip().endswith("Reward, Level 49"):
            out.append((r["questid"], r["name"], "49"))
    return out


class World:
    def __init__(self, db):
        self.db = db
        self.icols = columns(db, "itempattern")
        self.isets, self.ifroms, self.inames = abl.copy_columns(self.icols)
        self.item_copy = {}  # base -> Ascended record (itemascended)
        self.item_base = {}
        if has_table(db, "itemascended"):
            for c, b in db.execute("SELECT patternid, base FROM itemascended"):
                self.item_copy[b], self.item_base[c] = c, b
        self.imported = {r[0] for r in db.execute("SELECT patternid FROM itemimported")} if has_table(db, "itemimported") else set()
        self.spell_copy, self.spell_base = {}, {}
        if has_table(db, "spellascended"):
            for c, b in db.execute("SELECT spellid, base FROM spellascended"):
                self.spell_copy[b], self.spell_base[c] = c, b
        self.made_consumables = {}  # base item -> copy, from earlier runs' log
        if has_table(db, LOG):
            for ref, after in db.execute(f"SELECT ref, after FROM {LOG} WHERE tbl = 'itempattern' AND before IS NULL"):
                a = json.loads(after) if after else {}
                if a.get("_consumable_of"):
                    self.made_consumables[a["_consumable_of"]] = json.loads(ref)["patternid"]

    def item(self, pid):
        return one(self.db, "SELECT * FROM itempattern WHERE patternid = ?", (pid,))

    def spell(self, sid):
        return one(self.db, "SELECT * FROM spellpattern WHERE spellid = ?", (sid,))

    def effect(self, sid):
        return one(self.db, "SELECT * FROM spelleffect WHERE spellid = ?", (sid,)) if has_table(self.db, "spelleffect") else None

    def item_by_name(self, name):
        """database.ItemPatternByName."""
        has_eff = has_table(self.db, "itemeffect")
        r = self.db.execute(
            "SELECT p.patternid FROM itempattern p "
            + ("LEFT JOIN itemeffect e ON e.patternid = p.patternid " if has_eff else "")
            + "WHERE lower(p.itemname) = ? AND NOT (p.itemicon IN (?, ?) AND p.equipslot = -1) "
            + ("ORDER BY e.patternid IS NULL, p.patternid " if has_eff else "ORDER BY p.patternid ")
            + "LIMIT 1", (name.strip().lower(),) + abl.SCROLL_ARTS).fetchone()
        return r[0] if r else None


# ---- the plan ----------------------------------------------------------------------------------------------------------

class Plan:
    def __init__(self):
        self.quests = []        # (questid, name, kind)
        self.items = {}         # base -> {"copy": id or None, "name", "why"}
        self.spells = {}        # base -> {"copy": id or None, "name", "helper": bool}
        self.consumables = {}   # base item -> {"copy": id or None, "name", "spell"}
        self.pets = {}          # base spell -> petstats row
        self.steps = []         # {"ref", "before", "after"}
        self.kept = []          # report lines: what stays as it is
        self.notes = {}         # spell base -> notes
        self.problems = []
        self.warnings = []

    def changes(self):
        return (sum(1 for r in self.items.values() if r["copy"] is None)
                + sum(1 for r in self.spells.values() if r["copy"] is None)
                + sum(1 for r in self.consumables.values() if r["copy"] is None) + len(self.steps))


def build_plan(db):
    p = Plan()
    w = World(db)
    p.world = w
    p.quests = epic_quests(db)
    if not p.quests:
        p.problems.append("no epic quest here (60/<group>/reward-<class>, 30,40,49/<group>/L49-<class>)")
        return p
    item_ids, spell_ids = {}, {}  # id -> first quest naming it

    def want_item(pid, q):
        if pid in w.item_base:
            return  # an Ascended record already
        item_ids.setdefault(pid, q)

    def want_spell(sid, q):
        if sid in w.spell_base:
            return
        spell_ids.setdefault(sid, q)

    steps = {}
    for qid, qname, kind in p.quests:
        for st in dict_rows(db, "SELECT * FROM queststeps WHERE questid = ? ORDER BY step", (qid,)):
            steps[(qid, st["step"])] = (st, kind)
            for pid, _ in counts(st["rewarditems"]):
                want_item(pid, qid)
            if kind == "60":
                for sid, _ in counts(st["spells"]):
                    want_spell(sid, qid)
            for blob in ("byclass", "choices"):
                try:
                    j = json.loads(st[blob] or "{}") or {}
                except ValueError:
                    p.problems.append(f"quest {qid} step {st['step']}: its {blob} is not JSON")
                    continue
                opts = (j.get("options") or []) if blob == "choices" else list(j.values())
                for o in opts:
                    if not isinstance(o, dict):
                        continue
                    for pid, _ in counts(o.get("items")):
                        want_item(pid, qid)
                    if kind == "60":
                        for sid, _ in counts(o.get("spells")):
                            want_spell(sid, qid)
    # items: gear gets the server's Ascended record
    for pid, q in sorted(item_ids.items()):
        rec = w.item(pid)
        if rec is None:
            p.warnings.append(f"quest {q}: item {pid} is no record here, left")
            continue
        if pid in w.imported:
            p.kept.append(f"{pid} {rec['itemname']}: an import's copy, as it is")
            continue
        if not abl.is_gear(rec):
            p.kept.append(f"{pid} {rec['itemname']}: no gear ({'a scroll' if abl.is_scroll(rec) else 'wear slot ' + str(rec['equipslot'])}), as it is")
            continue
        name = abl.ascended_name(rec["itemname"])
        if len(name) > MAX_ITEM_NAME:
            p.problems.append(f"{pid} {rec['itemname']}: {name!r} is {len(name)} characters, the client shows {MAX_ITEM_NAME}")
            continue
        p.items[pid] = {"copy": w.item_copy.get(pid), "name": name}
    # spells (the 60 epics'), then what they make and summon
    queue = [(sid, False) for sid in sorted(spell_ids)]
    seen = set()
    while queue:
        sid, helper = queue.pop(0)
        if sid in seen:
            continue
        seen.add(sid)
        sp = w.spell(sid)
        if sp is None:
            p.warnings.append(f"spell {sid} is no record here, left")
            continue
        name = PREFIX + sp["spellname"].strip()
        if len(name) > MAX_SPELL_NAME:
            p.problems.append(f"spell {sid} {sp['spellname']}: {name!r} is {len(name)} characters, the client shows {MAX_SPELL_NAME}")
            continue
        clash = one(db, "SELECT spellid FROM spellpattern WHERE spellname = ?", (name,))
        if clash and clash["spellid"] != w.spell_copy.get(sid):
            p.problems.append(f"spell {clash['spellid']} is named {name!r} already and is no Ascended copy of {sid}: rename it first")
            continue
        p.spells[sid] = {"copy": w.spell_copy.get(sid), "name": name, "helper": helper}
        if w.spell_copy.get(sid):
            continue
        eff = w.effect(sid)
        js = json.loads(eff["effects"]) if eff and (eff.get("effects") or "").strip() else None
        for e in (js or {}).get("effects") or []:
            if str(e.get("kind", "")).lower() == "create_item":
                iname = str((e.get("item") or {}).get("name") or "").strip()
                ipid = w.item_by_name(iname) if iname else None
                if ipid is None:
                    p.warnings.append(f"spell {sid} {sp['spellname']} makes {iname!r}, no record here: it makes it as it is")
                    continue
                if ipid not in p.consumables:
                    rec = w.item(ipid)
                    cname = PREFIX + rec["itemname"].strip()
                    p.consumables[ipid] = {"copy": w.made_consumables.get(ipid), "name": cname, "spell": sid,
                                           "base_name": rec["itemname"].strip()}
                    if len(cname) > MAX_ITEM_NAME:
                        p.problems.append(f"{ipid} {rec['itemname']}: {cname!r} is too long")
                    other = one(db, "SELECT patternid FROM itempattern WHERE lower(itemname) = ?", (cname.lower(),))
                    if other and other["patternid"] != w.made_consumables.get(ipid):
                        p.problems.append(f"item {other['patternid']} is named {cname!r} already, not this tool's copy of {ipid}")
        if has_table(db, "petstats"):
            pet = one(db, "SELECT * FROM petstats WHERE spellid = ?", (sid,))
            if pet:
                p.pets[sid] = pet
                for a in json.loads(pet.get("abilities") or "[]"):
                    if a.get("spell_id"):
                        queue.append((int(a["spell_id"]), True))
                    elif a.get("spell"):
                        p.warnings.append(f"pet of {sp['spellname']}: an ability pins {a['spell']!r} by name: its pet casts it as it is")
    # the quests' steps, pointed at the Ascended ones
    # (the plan's rows name a copy to be made "new(<base>)"; apply writes the ids: finalize_step)
    item_map = {b: (r["copy"] or f"new({b})") for b, r in p.items.items()}
    spell_map = {b: (r["copy"] or f"new({b})") for b, r in p.spells.items() if not r["helper"]}
    for (qid, step), (st, kind) in sorted(steps.items()):
        new = dict(st)
        imap = {b: v for b, v in item_map.items()}
        smap = spell_map if kind == "60" else {}
        new["rewarditems"] = remap(st["rewarditems"], imap)
        new["spells"] = remap(st["spells"], smap)
        for blob in ("byclass", "choices"):
            if not (st[blob] or "").strip():
                continue
            j = json.loads(st[blob])
            opts = (j.get("options") or []) if blob == "choices" else list(j.values())
            touched = False
            for o in opts:
                if not isinstance(o, dict):
                    continue
                names = []
                for key, m, kindname in (("items", imap, "item"), ("spells", smap, "spell")):
                    ids = [pid for pid, _ in counts(o.get(key)) if pid in m]
                    if ids:
                        o[key] = remap(o[key], m)
                        names += [(w.item(i)["itemname"] if kindname == "item" else w.spell(i)["spellname"]).strip() for i in ids]
                if names and blob == "choices" and "answer" in o:
                    a = ascend_answer(o["answer"], names)
                    if len(a) > MAX_ANSWER:
                        p.problems.append(f"quest {qid} step {step}: {a!r} is {len(a)} characters, an answer shows {MAX_ANSWER}")
                    o["answer"] = a
                touched = touched or bool(names)
            if touched:
                new[blob] = json.dumps(j, ensure_ascii=False, separators=(",", ":"))
        if any(new[c] != st[c] for c in ("rewarditems", "spells", "byclass", "choices")):
            p.steps.append({"ref": {"questid": qid, "step": step}, "before": st, "after": new, "kind": kind})
    return p


# ---- reporting ---------------------------------------------------------------------------------------------------------

def report(p):
    w = p.world
    q60 = [q for q in p.quests if q[2] == "60"]
    q49 = [q for q in p.quests if q[2] == "49"]
    out = [f"Epic quests: {len(q60)} level 60 epic rewards, {len(q49)} level 49 epic weapon rewards (their path "
           "spells as they are)"]
    new = [(b, r) for b, r in sorted(p.items.items()) if r["copy"] is None]
    old = [(b, r) for b, r in sorted(p.items.items()) if r["copy"] is not None]
    out.append(f"Ascended records (gear): {len(new)} to make, {len(old)} made already")
    for b, r in new:
        rec = dict(zip(w.inames, db_row(w, b)))
        out.append(f"  {r['name']} (of {b}, {abl.SLOT_NAMES.get(w.item(b)['equipslot'], '?')}): {figures(w, b, rec)}")
    for b, r in old:
        out.append(f"  {r['copy']} {r['name']} (of {b}): made already")
    out.append(f"Ascended consumables: {sum(1 for r in p.consumables.values() if r['copy'] is None)} to make, "
               f"{sum(1 for r in p.consumables.values() if r['copy'] is not None)} made already")
    for b, r in sorted(p.consumables.items()):
        ie = one(w.db, "SELECT * FROM itemeffect WHERE patternid = ?", (b,)) if has_table(w.db, "itemeffect") else None
        use = f"; use {ie['line']!r} -> {scale_line(ie['line'])!r}" if ie else ""
        out.append(f"  {r['name']} (of {b}, made by spell {r['spell']}){' made already' if r['copy'] else ''}{use}")
    sp_new = [(b, r) for b, r in sorted(p.spells.items()) if r["copy"] is None]
    out.append(f"Ascended spells: {len(sp_new)} to make ({sum(1 for _, r in sp_new if r['helper'])} of them a pet's "
               f"ability), {len(p.spells) - len(sp_new)} made already")
    for b, r in sp_new:
        eff = w.effect(b)
        line = eff["line"] if eff else ""
        js_notes = []
        if eff and (eff.get("effects") or "").strip():
            _, js_notes = scale_effects(eff["effects"], {})
        what = (f"{line!r} -> "
                f"{scale_line(line, [(c['base_name'], c['name']) for c in p.consumables.values()], has_price(eff.get('effects') if eff else ''))!r}")
        out.append(f"  {r['name']} (of {b}{', a pet ability' if r['helper'] else ''}): {what}"
                   + (f"; as it is: {', '.join(sorted(set(js_notes)))}" if js_notes else ""))
        if b in p.pets:
            out.append(f"    its pet {p.pets[b]['name']}: hp {p.pets[b]['hp']} -> {up(p.pets[b]['hp'])}, weapon "
                       f"{p.pets[b]['wpn1_dmg']}/{p.pets[b]['wpn2_dmg']} -> {up(p.pets[b]['wpn1_dmg'])}/{up(p.pets[b]['wpn2_dmg'])}, "
                       f"AC {p.pets[b]['ac']} -> {up(p.pets[b]['ac'])}")
    out.append(f"Quest steps to point at them: {len(p.steps)}")
    names = {q[0]: q[1] for q in p.quests}
    for s in p.steps:
        out.append(f"  quest {s['ref']['questid']} {names[s['ref']['questid']]!r} step {s['ref']['step']}")
        for c in ("rewarditems", "spells"):
            if s["before"][c] != s["after"][c]:
                out.append(f"    {c}: {s['before'][c]!r} -> {s['after'][c]!r}")
        if s["before"]["choices"] != s["after"]["choices"]:
            a = (json.loads(s["before"]["choices"]) or {}).get("options") or []
            b = (json.loads(s["after"]["choices"]) or {}).get("options") or []
            for x, y in zip(a, b):
                if x != y:
                    out.append(f"    {x.get('answer')!r} [{x.get('items', '')}{x.get('spells', '')}] -> "
                               f"{y.get('answer')!r} [{y.get('items', '')}{y.get('spells', '')}]")
    for k in p.kept:
        out.append(f"As it is: {k}")
    for x in p.warnings:
        out.append(f"WARNING: {x}")
    for x in p.problems:
        out.append(f"PROBLEM: {x}")
    return out


def db_row(w, base):
    return w.db.execute(f"SELECT {w.ifroms} FROM itempattern WHERE patternid = ?", (base,)).fetchone()


def figures(w, base, new):
    b = w.item(base)
    up_, same = [], []
    for c in w.icols:
        low = c.lower()
        if low in abl.SCALED and b.get(c) not in (None, 0):
            (up_ if new[c] != b[c] else same).append(f"{abl.FIGURE_NAMES[low]} {b[c]}" + (f" -> {new[c]}" if new[c] != b[c] else ""))
    s = ", ".join(up_) or "no figure changes"
    return s + (f" ({', '.join(same)} as they are)" if same else "")


# ---- writing -------------------------------------------------------------------------------------------------------

class Writer:
    def __init__(self, db, run):
        self.db, self.run, self.n = db, run, 0
        self.at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

    def audit(self, action, tbl, ref, before, after, detail):
        cur = self.db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                              "VALUES (?, 0, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                              (self.at, WHO, VIA, action, tbl, go_json(ref), None if before is None else go_json(before),
                               None if after is None else go_json(after), detail))
        return cur.lastrowid

    def log(self, tbl, ref, before, after, action, extra=None):
        a = None
        if tbl not in ("sqlite_sequence", "sqlite_master"):
            a = self.audit(action, tbl, ref, before, after, f"{DETAIL} (run {self.run})")
        logged = after if extra is None or after is None else dict(after, **extra)
        self.db.execute(f"INSERT INTO {LOG} (run, tbl, ref, before, after, audit) VALUES (?, ?, ?, ?, ?, ?)",
                        (self.run, tbl, go_json(ref), None if before is None else go_json(before),
                         None if logged is None else go_json(logged), a))
        self.n += 1

    def insert(self, tbl, row, extra=None):
        cs = list(row)
        self.db.execute(f"INSERT INTO {tbl} ({', '.join(cs)}) VALUES ({', '.join('?' * len(cs))})", [row[c] for c in cs])
        ref = {k: row[k] for k in KEYS[tbl]}
        self.log(tbl, ref, None, row_of(self.db, tbl, ref), "insert", extra)


def next_item_id(db):
    nid = db.execute("SELECT COALESCE(MAX(patternid), ?) + 1 FROM itempattern WHERE patternid BETWEEN ? AND ?",
                     (abl.FIRST_ID - 1, abl.FIRST_ID, abl.LAST_ID)).fetchone()[0]
    if nid > abl.LAST_ID:
        raise SystemExit("no id left among the Ascended records'")
    return nid


def make_item(db, wr, w, base, run):
    """base's Ascended record by the server's rule (as abl.make_record), logged here."""
    nid = next_item_id(db)
    db.execute(f'INSERT INTO itempattern ("patternid", {w.isets}) SELECT ?, {w.ifroms} FROM itempattern WHERE patternid = ?',
               (nid, base))
    wr.log("itempattern", {"patternid": nid}, None, row_of(db, "itempattern", {"patternid": nid}), "insert")
    if has_table(db, "itemeffect"):
        for e in dict_rows(db, "SELECT * FROM itemeffect WHERE patternid = ?", (base,)):
            e = dict(e, patternid=nid)
            for c in ("line", "target_line"):
                if c in e:
                    e[c] = abl.scaled_line(e[c])
            wr.insert("itemeffect", e)
    wr.insert("itemascended", {"patternid": nid, "base": base, "made": wr.at, "source": f"{TOOL} {run}"})
    w.item_copy[base], w.item_base[nid] = nid, base
    return nid


def make_consumable(db, wr, w, base, c):
    nid = next_item_id(db)
    row = dict(zip(w.inames, db_row(w, base)))
    row["itemname"] = c["name"]
    row["patternid"] = nid
    wr.insert("itempattern", row, extra={"_consumable_of": base})
    if has_table(db, "itemeffect"):
        for e in dict_rows(db, "SELECT * FROM itemeffect WHERE patternid = ?", (base,)):
            e = dict(e, patternid=nid)
            for k in ("line", "target_line"):
                if k in e:
                    e[k] = scale_line(e[k])
            if e.get("name") and e["name"].strip().lower() == c["base_name"].lower():
                e["name"] = c["name"]
            e["source"] = f"ascended of {base} ({TOOL})"
            wr.insert("itemeffect", e)
    if has_table(db, "foodeffect"):
        for f in dict_rows(db, "SELECT * FROM foodeffect WHERE patternid = ?", (base,)):
            wr.insert("foodeffect", dict(f, patternid=nid, source=f"ascended of {base} ({TOOL})"))
    w.made_consumables[base] = nid
    return nid


def make_spell(db, wr, w, p, base, r, run):
    sp = w.spell(base)
    row = {k: v for k, v in sp.items() if k != "spellid"}
    row["spellname"] = r["name"]
    row["spelldesc"] = ascended_desc(sp.get("spelldesc"))
    cs = list(row)
    cur = db.execute(f"INSERT INTO spellpattern ({', '.join(cs)}) VALUES ({', '.join('?' * len(cs))})", [row[c] for c in cs])
    nid = cur.lastrowid
    wr.log("spellpattern", {"spellid": nid}, None, row_of(db, "spellpattern", {"spellid": nid}), "insert")
    cons = {c["base_name"].lower(): {"name": c["name"], "id": c["copy"]} for c in p.consumables.values()}
    eff = w.effect(base)
    if eff:
        e = dict(eff, spellid=nid)
        e["line"] = scale_line(eff["line"], [(c["base_name"], c["name"]) for c in p.consumables.values()
                                             if c["spell"] == base], has_price(eff.get("effects")))
        if (eff.get("effects") or "").strip():
            e["effects"], _ = scale_effects(eff["effects"], cons)
        e["source"] = f"ascended of {base} ({TOOL}); {eff.get('source') or ''}".rstrip("; ")
        wr.insert("spelleffect", e)
    if has_table(db, "spellarea"):
        for a in dict_rows(db, "SELECT * FROM spellarea WHERE spellid = ?", (base,)):
            wr.insert("spellarea", dict(a, spellid=nid))
    if not r["helper"] and has_table(db, "classspells"):
        for c in dict_rows(db, "SELECT * FROM classspells WHERE spellid = ? ORDER BY class", (base,)):
            wr.insert("classspells", dict(c, spellid=nid, source=CLASS_SOURCE))
    wr.insert("spellascended", {"spellid": nid, "base": base, "made": wr.at, "source": f"{TOOL} {run}"})
    w.spell_copy[base], w.spell_base[nid] = nid, base
    r["copy"] = nid
    return nid


def make_pet(db, wr, w, base):
    pet = one(db, "SELECT * FROM petstats WHERE spellid = ?", (base,))
    row = dict(pet, spellid=w.spell_copy[base])
    for c in PET_SCALED:
        if c in row and isinstance(row[c], (int, float)):
            row[c] = up(row[c])
    abil = json.loads(pet.get("abilities") or "[]")
    for a in abil:
        if a.get("spell_id") and int(a["spell_id"]) in w.spell_copy:
            a["spell_id"] = w.spell_copy[int(a["spell_id"])]
    row["abilities"] = json.dumps(abil, separators=(",", ":"))
    row["note"] = f"Ascended copy of petstats {base} (a tenth more; {TOOL}). " + (pet.get("note") or "")
    row["source"] = "ascended"
    wr.insert("petstats", row)


def seq(db, t):
    if not has_table(db, "sqlite_sequence"):
        return None
    r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = ?", (t,)).fetchone()
    return None if r is None else r[0]


def apply(db, p, run):
    w = p.world
    made = [t for t in ("itemascended", "spellascended") if not has_table(db, t)]
    for s in (abl.SCHEMA, SPELL_MAP_SCHEMA, abl.AUDIT_SCHEMA, LOG_SCHEMA):
        db.execute(s)
    wr = Writer(db, run)
    for t in made:
        wr.log("sqlite_master", {"table": t}, None, {"made": True}, "create")
    seqs = {t: seq(db, t) for t in ("itempattern", "spellpattern", "gm_audit") if t != "gm_audit"}
    for base, r in sorted(p.items.items()):
        if r["copy"] is None:
            r["copy"] = make_item(db, wr, w, base, run)
    for base, c in sorted(p.consumables.items()):
        if c["copy"] is None:
            c["copy"] = make_consumable(db, wr, w, base, c)
    # the pets' abilities first, so a pet's row pins their copies
    order = sorted(p.spells.items(), key=lambda kv: (not kv[1]["helper"], kv[0]))
    for base, r in order:
        if r["copy"] is None:
            make_spell(db, wr, w, p, base, r, run)
    for base in sorted(p.pets):
        if not one(db, "SELECT spellid FROM petstats WHERE spellid = ?", (w.spell_copy[base],)):
            make_pet(db, wr, w, base)
    # the steps: their ids now the copies'
    for s in p.steps:
        mapped = finalize_step(s, w)
        now = row_of(db, "queststeps", s["ref"])
        if now is None or any(now[c] != s["before"][c] for c in ("rewarditems", "spells", "byclass", "choices")):
            raise SystemExit(f"queststeps {s['ref']} changed under the plan")
        db.execute("UPDATE queststeps SET rewarditems = ?, spells = ?, byclass = ?, choices = ? WHERE questid = ? AND step = ?",
                   (mapped["rewarditems"], mapped["spells"], mapped["byclass"], mapped["choices"],
                    s["ref"]["questid"], s["ref"]["step"]))
        wr.log("queststeps", s["ref"], now, row_of(db, "queststeps", s["ref"]), "update")
    for t, was in seqs.items():
        cur = seq(db, t)
        if cur != was:
            wr.log("sqlite_sequence", {"name": t}, None if was is None else {"seq": was}, {"seq": cur}, "update")
    return wr.n


def finalize_step(s, w):
    """The step's after-row with each planned id its copy's id (the plan names bases; the copies exist now)."""
    imap = {b: c for b, c in w.item_copy.items()}
    smap = {b: c for b, c in w.spell_copy.items()} if s["kind"] == "60" else {}
    before, after = s["before"], dict(s["after"])
    after["rewarditems"] = remap(before["rewarditems"], imap)
    after["spells"] = remap(before["spells"], smap)
    for blob in ("byclass", "choices"):
        if not (after[blob] or "").strip() or after[blob] == before[blob]:
            continue
        jb, ja = json.loads(before[blob]), json.loads(after[blob])
        if blob == "choices":
            pairs = zip(jb.get("options") or [], ja.get("options") or [])
        else:
            pairs = ((jb[k], ja[k]) for k in jb)
        for ob, oa in pairs:
            if isinstance(ob, dict):
                for key, m in (("items", imap), ("spells", smap)):
                    if ob.get(key):
                        oa[key] = remap(ob[key], m)
        after[blob] = json.dumps(ja, ensure_ascii=False, separators=(",", ":"))
    return after


# ---- undoing -------------------------------------------------------------------------------------------------------

def held(db, tbl, ref):
    """Where a made item or spell is held or named outside this tool's rows (a character's, a loot table, a quest)."""
    out = []
    col, val = ("patternid", ref.get("patternid")) if "patternid" in ref else ("spellid", ref.get("spellid"))
    if tbl not in ("itempattern", "spellpattern"):
        return out
    for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'").fetchall():
        if t in OWN or t.endswith("_log") or t == "gm_audit":
            continue
        if col in [c.lower() for c in columns(db, t)]:
            n = db.execute(f'SELECT count(*) FROM "{t}" WHERE {col} = ?', (val,)).fetchone()[0]
            if n:
                out.append(f"{t} {n}")
    if has_table(db, "queststeps"):
        num = re.compile(rf"(?<![0-9]){val}(?![0-9])")
        for qid, step, *texts in db.execute("SELECT questid, step, items, rewarditems, spells, byclass, choices FROM queststeps"):
            if any(num.search(t or "") for t in texts):
                out.append(f"quest {qid} step {step}")
    if col == "spellid" and has_table(db, "petstats"):
        for (sid, ab) in db.execute("SELECT spellid, abilities FROM petstats"):
            if re.search(rf'"spell_id":\s*{val}\b', ab or "") and not one(db, f"SELECT 1 AS x FROM {LOG} WHERE tbl = 'petstats' AND ref = ?",
                                                                            (go_json({"spellid": sid}),)):
                out.append(f"petstats {sid}'s abilities")
    return out


def undo(db):
    if not has_table(db, LOG):
        return 0, []
    wr = Writer(db, "undo")
    back, left = 0, []
    entries = db.execute(f"SELECT seq, run, tbl, ref, before, after FROM {LOG} ORDER BY seq DESC").fetchall()
    # the counters last, once the rows above them are gone
    entries = [e for e in entries if e[2] != "sqlite_sequence"] + [e for e in entries if e[2] == "sqlite_sequence"]
    # what stays: a made record or spell held somewhere now (once the steps are put back)
    kept = {}
    for sq, r, tbl, ref, before, after in entries:
        ref = json.loads(ref)
        before = json.loads(before) if before is not None else None
        after = json.loads(after) if after is not None else None
        if after is not None:
            after = {k: v for k, v in after.items() if not k.startswith("_")}
        done = False
        if tbl == "sqlite_master":
            t = ref["table"]
            if has_table(db, t) and not db.execute(f'SELECT count(*) FROM "{t}"').fetchone()[0]:
                db.execute(f'DROP TABLE "{t}"')
                done = True
            elif has_table(db, t):
                left.append(f"table {t}: rows in it, kept")
        elif tbl == "sqlite_sequence":
            cur = seq(db, ref["name"])
            mx = db.execute(f'SELECT MAX(rowid) FROM "{ref["name"]}"').fetchone()[0] or 0
            if before is not None and mx <= before["seq"]:
                db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = ?", (before["seq"], ref["name"]))
                done = True
            elif before is None and mx == 0:
                db.execute("DELETE FROM sqlite_sequence WHERE name = ?", (ref["name"],))
                done = True
            else:
                left.append(f"the {ref['name']} counter: {cur}, rows above it stay (left)")
        else:
            if before is None and tbl in ("itempattern", "spellpattern", "itemeffect", "foodeffect", "itemascended",
                                          "spelleffect", "spellarea", "classspells", "petstats", "spellascended"):
                owner = ("patternid", ref["patternid"]) if "patternid" in ref else ("spellid", ref["spellid"])
                if owner not in kept:
                    main = "itempattern" if owner[0] == "patternid" else "spellpattern"
                    h = held(db, main, {owner[0]: owner[1]})
                    kept[owner] = ", ".join(h)
                    if h:
                        left.append(f"{main} {owner[1]} stays: {kept[owner]}")
                if kept[owner]:
                    db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (sq,))
                    continue
            now = row_of(db, tbl, ref)
            if before is None:
                if now is not None:
                    if go_json(now) != go_json(after):
                        left.append(f"{tbl} {go_json(ref)}: changed since it was written, taken away all the same")
                    db.execute(f"DELETE FROM {tbl} WHERE " + " AND ".join(f"{c} = ?" for c in KEYS[tbl]),
                               tuple(ref[c] for c in KEYS[tbl]))
                    wr.audit("delete", tbl, ref, now, None, f"undo: {DETAIL} (run {r})")
                done = True
            else:
                if now is None or go_json(now) != go_json(after):
                    left.append(f"{tbl} {go_json(ref)}: changed since it was written, left")
                else:
                    cs = [c for c in before if c not in KEYS[tbl]]
                    db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in cs)} WHERE "
                               + " AND ".join(f"{c} = ?" for c in KEYS[tbl]),
                               [before[c] for c in cs] + [ref[c] for c in KEYS[tbl]])
                    wr.audit("update", tbl, ref, now, row_of(db, tbl, ref), f"undo: {DETAIL} (run {r})")
                    done = True
        if done:
            back += 1
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (sq,))
    if not db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0]:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


# ---- characters holding the regular rewards ---------------------------------------------------------------------------

def swap_report(db):
    """Who holds a regular epic reward that has an Ascended one now (nothing written): the swap a GM may ask for."""
    w = World(db)
    p = build_plan(db)
    out = []
    items = {b: c for b, c in w.item_copy.items() if b in p.items}
    spells = {b: c for b, c in w.spell_copy.items() if b in p.spells and not p.spells[b]["helper"]}
    if has_table(db, "charinventory") and items:
        q = ",".join("?" * len(items))
        for sid, pid, n in db.execute(f"SELECT serverid, patternid, count(*) FROM charinventory WHERE patternid IN ({q}) "
                                      "GROUP BY serverid, patternid ORDER BY serverid", list(items)):
            who = one(db, "SELECT charname FROM characters WHERE serverid = ?", (sid,)) if has_table(db, "characters") else None
            out.append(f"{(who or {}).get('charname', sid)}: item {pid} {w.item(pid)['itemname']} x{n} -> {items[pid]}")
    if has_table(db, "spells") and spells:
        q = ",".join("?" * len(spells))
        for sid, spid in db.execute(f"SELECT serverid, spellid FROM spells WHERE spellid IN ({q}) ORDER BY serverid", list(spells)):
            who = one(db, "SELECT charname FROM characters WHERE serverid = ?", (sid,)) if has_table(db, "characters") else None
            out.append(f"{(who or {}).get('charname', sid)}: spell {spid} {w.spell(spid)['spellname']} -> {spells[spid]}")
    return out


def _backup(path):
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-ascended-epics"
    src, out = sqlite3.connect(path, timeout=30), sqlite3.connect(dst)
    try:
        src.backup(out)
    finally:
        out.close()
        src.close()
    return dst


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0], formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--no-backup", action="store_true")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write it")
    g.add_argument("--undo", action="store_true", help="put back what --apply wrote")
    g.add_argument("--swap-report", action="store_true", help="list who holds a regular epic reward (nothing written)")
    a = ap.parse_args(argv)
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = abl.connect(a.db)
    try:
        if a.swap_report:
            lines = swap_report(db)
            for x in lines:
                print(x)
            print(f"{len(lines)} held regular epic rewards (run --apply first for the Ascended ids); nothing written")
            return 0
        if a.undo:
            if not a.no_backup:
                print(f"copy: {_backup(a.db)}")
            db.execute("BEGIN IMMEDIATE")
            try:
                db.execute(abl.AUDIT_SCHEMA)
                back, left = undo(db)
                db.execute("COMMIT")
            except BaseException:
                db.execute("ROLLBACK")
                raise
            for x in left:
                print("left:", x)
            print(f"put back {back} rows; {len(left)} left as they are; restart the server")
            return 0
        p = build_plan(db)
        for line in report(p):
            print(line)
        n = p.changes()
        if not a.apply:
            print(f"{n} changes: report only, --apply writes them")
            return 2 if p.problems else 0
        if p.problems:
            print(f"nothing written: {len(p.problems)} problems")
            return 2
        if n == 0:
            print("nothing to do: the epic quests reward Ascended already")
            return 0
        if not a.no_backup:
            print(f"copy: {_backup(a.db)}")
        run = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%S.%fZ")
        db.execute("BEGIN IMMEDIATE")
        try:
            p = build_plan(db)
            if p.problems:
                raise SystemExit(f"nothing written: {len(p.problems)} problems (the database changed)")
            wrote = apply(db, p, run)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"run {run}: wrote {wrote} rows (each in gm_audit); RESTART the server (quests, spells and pets are read "
              f"at its start); --undo takes it back")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
