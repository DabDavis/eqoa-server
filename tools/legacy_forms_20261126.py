#!/usr/bin/env python3
"""Legacy forms: an ability's older form, owned through a class mastery (Heirloom petition 3184: "the ability to
return abilities back to older versions with CMs, like when the paladin 51 was a weapon proc and Fervor was an HP proc
damage shield"; the owner approved the mechanism, 2026-11-26).

The server (handlers/legacyforms.go, table legacyforms) casts a legacy spell in place of an ability for a character
that owns the mastery rank a legacyforms row names; the book is not changed, so removing the rank gives the present
form back at the next cast. This tool seeds the first two, both the paladin's, each from the era records:

  * Fervor (700032, level 4) as of April 2004, a weapon proc: Allakhazam's spell 154 "Fervor" (Last Import Date
    2004-04-28 13:47:59; web.archive.org 20120214092255): "Add Offensive Proc: Fervor Effect", level 4, casting time
    2s, recast 1s, power 48, Self Only, 5 minutes; its proc, spell 155 "Fervor Effect" (same import; archived
    20120207195752): "Decrease Hitpoints by 8", instant. EQOA-Data's line of the same import: "Self, Decrease
    Hitpoints by 8, 5min". Today's Fervor is a damage shield of 10 that heals its bearer over time when struck
    (EQOAEmu's workbook, imported 2026-11-01).
  * Sacred Heart (701066, level 51, mastery 3671) as of April 2004, a weapon proc that heals: Allakhazam's spell 3961
    "Sacred Heart" (Last Import Date 2004-04-28 13:47:59; archived 20120214090359): "Add Offensive Proc: Sacred Heart
    Aura", level 51, casting time 0s, recast 3600s, power 0, Self Only, 5 minutes; its proc, spell 3163 "Sacred Heart
    Aura" (same import; archived 20120207211307): "Increase Hitpoints by 5 ; Increase 150% based on CHA", instant.
    cmcatalog 3671's own text (EQOA-Data's) says the same: "An aura that sometime heals the paladin as he battles
    enemies." Today's Sacred Heart heals 250 + 150% CHA a tick when the paladin is struck, 90 s (EQOAEmu's workbook).

The records were read out of the private all-classes package's inventory (its historical_inventory, every field with
its archive URL and hash), read only. Kept from today's rows (no record gives them): the icons, the range, the visual,
the workbook's stack type and minimum level. The proc's chance is the server's for any offensive proc (its INT's).

Each gets a spellpattern row and a spelleffect row of its own (ids 761841, 761842, names "Fervor (2004)" and "Sacred
Heart (2004)": no merchant, book or scroll names them), a class mastery of the paladin (cmcatalog 31841, 31842:
category 3 Class, 1 point, the ability's level, the ability needed in the book) and its legacyforms row. Report only
by default; --apply in one transaction, a gm_audit line a row, log legacy_forms_1126_log; --undo exact (a row changed
since is left and said). Needs the server with handlers/legacyforms.go (an older one sells the ranks and casts nothing
else: run --undo there). Data alone: no restart; the next cast reads it.

    python3 -E -s tools/legacy_forms_20261126.py --db eqoa.db [--apply | --undo]
"""
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "legacy_forms_1126_log"
DETAIL = "Legacy forms, Heirloom petition 3184 (the owner approved, 2026-11-26): tools/legacy_forms_20261126.py"
SOURCE = "legacy-forms-1126"
PALADIN = 2

LEGACYFORMS_SCHEMA = """CREATE TABLE IF NOT EXISTS legacyforms (
    spellid        INTEGER NOT NULL,
    legacy_spellid INTEGER NOT NULL,
    mastery_id     INTEGER NOT NULL,
    note           TEXT NOT NULL DEFAULT '',
    enabled        INTEGER NOT NULL DEFAULT 1,
    source         TEXT NOT NULL DEFAULT 'gm',
    PRIMARY KEY (spellid, mastery_id)
)"""
LEGACYFORMS_INDEX = "CREATE INDEX IF NOT EXISTS idx_legacyforms_mastery ON legacyforms(mastery_id)"


def proc_effects(level, min_level, stack, procs):
    """A workbook effects JSON (entity/spelljson.go) of a self buff of 5 minutes holding offensive procs."""
    return json.dumps({"v": 1, "scope": "self", "target_type": "Self", "harmful": False, "duration_s": 300,
                       "stack": stack, "level": level, "min_level": min_level, "req_equipment": "Any",
                       "sheet_type": "Offensive Proc",
                       "effects": [{"type": "Offensive Proc", "kind": "proc", "procs": procs}]},
                      separators=(",", ":"))


# Each form: the ability, its legacy spell's figures (the records'), and its mastery.
FORMS = [
    {
        "ability": 700032, "ability_name": "Fervor", "legacy": 761841, "mastery": 31841,
        "name": "Fervor (2004)", "level": 4, "casttime": 2, "recast": 1, "power": 48,
        "desc": "Self, Add Offensive Proc: Decrease Hitpoints by 8 when you hit, 5min (Fervor as of April 2004)",
        "line": "Self, Add Offensive Proc: Decrease Hitpoints by 8, 5min",
        "procs": [{"type": "Dmg/Heal", "kind": "hp", "amount": -8, "on": "offense", "scope": "target",
                   "target_type": "Enemy"}],
        "cm_name": "Legacy Fervor",
        "cm_text": "Fervor as it was in April 2004: a weapon proc striking your target for 8 when you hit, for 5 "
                   "minutes, in place of its damage shield and heal. Remove this mastery to cast today's again.",
        "records": "Allakhazam spells 154 Fervor + 155 Fervor Effect (imported 2004-04-28; web.archive.org "
                   "20120214092255, 20120207195752): Add Offensive Proc, Decrease Hitpoints by 8, cast 2s, recast 1s, "
                   "power 48, self, 5 minutes",
    },
    {
        "ability": 701066, "ability_name": "Sacred Heart", "legacy": 761842, "mastery": 31842,
        "name": "Sacred Heart (2004)", "level": 51, "casttime": 0, "recast": 3600, "power": 0,
        "desc": "Self, Add Offensive Proc: Increase Hitpoints by 5 ; Increase 150% based on CHA when you hit, 5min "
                "(Sacred Heart as of April 2004)",
        "line": "Self, Add Offensive Proc: Increase Hitpoints by 5 ; Increase 150% based on CHA, 5min",
        "procs": [{"type": "Dmg/Heal", "kind": "hp", "amount": 5, "scale": "cha", "scale_pct": 150, "on": "offense",
                   "scope": "self", "target_type": "Self"}],
        "cm_name": "Legacy Sacred Heart",
        "cm_text": "Sacred Heart as it was in April 2004: a weapon proc healing you for 5 plus 150% of your CHA when "
                   "you hit, for 5 minutes (recast an hour), in place of its heal when struck. Remove this mastery to "
                   "cast today's again.",
        "records": "Allakhazam spells 3961 Sacred Heart + 3163 Sacred Heart Aura (imported 2004-04-28; "
                   "web.archive.org 20120214090359, 20120207211307): Add Offensive Proc, Increase Hitpoints by 5 + "
                   "150% CHA, cast 0s, recast 3600s, power 0, self, 5 minutes",
    },
]


def workbook_of(db, spellid):
    """Today's ability's workbook header (stack, min_level), or the defaults."""
    r = db.execute("SELECT effects FROM spelleffect WHERE spellid = ?", (spellid,)).fetchone()
    try:
        m = json.loads(r[0]) if r and r[0] else {}
    except ValueError:
        m = {}
    return int(m.get("stack") or 0), int(m.get("min_level") or 1)


def ours(now, after, label):
    """Whether a row already there is this tool's: by its source where the table keeps one, else by its name."""
    if "source" in now:
        return str(now["source"]).split(":")[0] == SOURCE
    return now.get(label) == after.get(label)


def changes(db):
    out, lines, refusals = [], [], []
    applied = True
    for f in FORMS:
        today = tr.get(db, "spellpattern", {"spellid": f["ability"]})
        if today is None or today["spellname"] != f["ability_name"]:
            refusals.append(f"spell {f['ability']} is not {f['ability_name']!r}: {today and today['spellname']!r}")
            continue
        fx = db.execute("SELECT fxdict FROM spelleffect WHERE spellid = ?", (f["ability"],)).fetchone()
        stack, min_level = workbook_of(db, f["ability"])
        pattern = {"spellid": f["legacy"], "abilitylvl": f["level"], "unk2": 0, "unk3": 0, "range": today["range"],
                   "casttime": f["casttime"], "power": f["power"], "iconColor": today["iconColor"],
                   "icon": today["icon"], "scope": 0, "recast": f["recast"], "eqprequire": -1,
                   "spellname": f["name"], "spelldesc": f["desc"]}
        effect = {"spellid": f["legacy"], "line": f["line"], "fxdict": fx[0] if fx else 0,
                  "source": f"{SOURCE}: {f['records']}",
                  "effects": proc_effects(f["level"], min_level, stack, f["procs"])}
        cm = {"id": f["mastery"], "category": 3, "classes": 1 << PALADIN, "races": 2047, "mastery": f["cm_name"],
              "rank": 1, "name": f["cm_name"], "parent": 0, "cost": 1, "level": f["level"], "spent": 0,
              "minstats": "", "requires": f"{f['ability_name']}@{f['level']}", "require_spells": str(f["ability"]),
              "require_ranks": "", "require_quests": "", "effects": "", "adds": "", "add_spells": "", "removes": "",
              "remove_spells": "", "description": f["cm_text"], "enabled": 1, "source": SOURCE}
        form = {"spellid": f["ability"], "legacy_spellid": f["legacy"], "mastery_id": f["mastery"],
                "note": f"{f['ability_name']} as of April 2004: {f['records']}", "enabled": 1, "source": SOURCE}
        for tbl, key, after, label in (("spellpattern", {"spellid": f["legacy"]}, pattern, "spellname"),
                                       ("spelleffect", {"spellid": f["legacy"]}, effect, None),
                                       ("cmcatalog", {"id": f["mastery"]}, cm, "name")):
            now = tr.get(db, tbl, key)
            if now is not None and not ours(now, after, label):
                refusals.append(f"{tbl} {key} is another's already: {now.get(label or 'source')!r}")
            if now is None or any(now.get(c) != v for c, v in after.items()):
                applied = False
            out.append((tbl, key, after))
        fkey = {"spellid": f["ability"], "mastery_id": f["mastery"]}
        now = tr.get(db, "legacyforms", fkey) if tr.table(db, "legacyforms") else None
        if now is None or any(now.get(c) != v for c, v in form.items()):
            applied = False
        out.append(("legacyforms", fkey, form))
        lines.append(f"{f['ability_name']} ({f['ability']}) -> {f['name']} ({f['legacy']}), mastery {f['mastery']} "
                     f"{f['cm_name']!r} (1 point, level {f['level']}); {f['records']}")
    return out, lines, refusals, applied and not refusals


def prepare(db):
    db.execute(LEGACYFORMS_SCHEMA)
    db.execute(LEGACYFORMS_INDEX)
    return []


SPEC = {
    "log": LOG,
    "detail": DETAIL,
    "basis": "era",
    "basis_ref": "Allakhazam 2004-04-28 records; petition 3184",
    "keys": {"spellpattern": ("spellid",), "spelleffect": ("spellid",), "cmcatalog": ("id",),
             "legacyforms": ("spellid", "mastery_id")},
    "needs": ("spellpattern", "spelleffect", "cmcatalog"),
    "changes": changes,
    "prepare": prepare,
}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
