#!/usr/bin/env python3
"""The Mermaid's (race 32) pets, the part of her kit (tools/mermaid_kit_20261112.py) left out for its designer Zauz's
math, as he gave it (the owner, 2026-11-13). Verbatim:

  "Aquatic Birthright 1-5: grants a weak version of mage water pet ... if you pick the racial MC it unlocks a MCA for
  the Ghilan. ... MCA 3: Aquatic Ascension, unlockable if purchased Aquatic Birthright 1-5. Summons [a] Ghilan mage pet."
  "the pets dont line up too well having them be the previous mage pet for level. actually if move the levels down by
  5 and start them at lvl 15. These should work the first one is just really gonna suck lol":
    Aquatic Birthright 1, lvl 15: Add Abilities: Water Elementaling (4)
    Aquatic Birthright 2, lvl 25: Add Abilities: Water Elementalkin (17)
    Aquatic Birthright 3, lvl 35: Add Abilities: Water Elementalkin (29)
    Aquatic Birthright 4, lvl 45: Add Abilities: Water Elemental (34)
    Aquatic Birthright 5, lvl 55: Add Abilities: Summon Ghilan (43)
  "Hidden MCA should summon a lvl 51 Ghilan. 15 cms each and 50 cms for the MCA sound fair? the mage CM pets are 20
  each but they give you a pet thats your level. these ones will be 10 levels lower and basically just some extra
  healing on you and crappy cr nukes."

Each ability is the magician's own water pet summoning of that name, COPIED to a racial spell of its own (760180-760184,
and 760179 for Aquatic Ascension, the id the kit kept), rather than the magician's spell granted: a copy has its own
petstats row (the server makes a pet by its spell's id: entity/petstats.go), so the Ascension's Ghilan can be level 51;
no magician Mermaid loses her own scribed spell when a Birthright replacing it is bought or removed; and a GM's tuning
of the magician's pets does not move the racials, or the other way. The copy is the shard's own magician records as
they are at --apply (the two shards' differ a little: range, the Ghilan's size), each of spellpattern, spelleffect and
petstats whole, but:

  spellpattern  id, abilitylvl (the rank's level), spelldesc (ours); name, range, cast, power, recast, icon as the
                magician's (any class's power pool holds them: a warrior's at 60 is about the magician's)
  spelleffect   id, its effects' "level" (the rank's), source (ours); line, visual and summoned pet as the magician's
  petstats      spellid, note, source (ours); the Ascension's level 51 (the Ghilan's row is 50); every number, the
                body, the size and the abilities (its heal on its owner or itself, its cold nuke) as the magician's

  ability (id)                    copy of                         pet (level, HP, its spells)
  760180 Water Elementaling       640 Water Elementaling          Water Elementaling  4,   89: Water Heal/Nuke Level 4
  760181 Water Elementalkin       669 Water Elementalkin          Water Elementalkin 19,  766: Level 19
  760182 Water Elementalkin (29)  900688 Water Elementalkin (29)  Water Elementalkin 26, 1285: Level 26
  760183 Water Elemental (34)     900699 Water Elemental (34)     Water Elemental    31, 1707: Level 31
  760184 Summon Ghilan            700557 Summon Ghilan            Ghilan             43, 3002: Level 43
  760179 Aquatic Ascension        900745 Summon Ghilan (60)       Ghilan             51, 3945: Level 50

  ("(17)": no magician pet is 17, spell or pet; Water Elementalkin, the spell of level 20, makes the 19 one.)

The five keep the magician's names, so each fights by the magician's water pet template (pettemplate, by name: "pet
water", a cleric); "Aquatic Ascension" gets that row of its own (pettemplate "aquatic ascension" -> "pet water").

Six cmcatalog rows for race 32 (races = 1 << 32; classes 32767, every class):

  2177-2181 racial  Aquatic Birthright 1-5   cost 15 each, levels 15/25/35/45/55, each after the one before (parent,
                    as the Wisp's Stoic Empowerment and every First Aid line), each adding its ability at its level and
                    REMOVING the one before's (remove_spells, as First Aid 2-5 do): she keeps her best water pet, one
                    summoning in the book (a pet at a time anyway: a summoning dismisses the pet she has, entity/pet.go
                    summonPetLocked, whatever her class and whatever made the first). A rank removed gives the one
                    before's back (cmRemovalBookLocked).
  2182      master  Siren Royal 4 "Aquatic Ascension"   cost 50, level 60, parent 2174 (the Master Class, as 2175 and
                    2176), require_ranks 2177-2181 (every Birthright: the catalog's own "another mastery's ranks
                    first", entity/cmbuy.go cmBlockLocked, which also refuses a Birthright's removal while the
                    Ascension needs it): adds Aquatic Ascension @60 and removes Birthright 5's Summon Ghilan.

Refused, nothing written, unless racedefs 32 is the Mermaid, the kit's Siren Royal (2174) is there, the six magician
records are what this expects (name, summoned pet), every id here is free and no other row has the Mermaid's bit but
the kit's 2171-2176. Report only by default; --apply in one transaction, a gm_audit line a row, log
mermaid_pets_1113_log; --undo exact (a row changed since is left and said). cmcatalog, spellpattern, spelleffect,
petstats and pettemplate are read at the start (petstats and pettemplate also at a GM panel edit of them): a restart.
Owned ranks and books are read at login.

    tools/mermaid_pets_20261113.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "mermaid_pets_1113_log"
DETAIL = "The Mermaid's pets (Zauz's math, the owner 2026-11-13): tools/mermaid_pets_20261113.py"
WHO, VIA = "dab", "tool"
MERMAID = 32
RACES = 1 << MERMAID
SOURCE = "ours (Zauz's math, the owner 2026-11-13: the Mermaid's pets)"
KIT_IDS = range(2171, 2177)
SIREN_ROYAL = 2174
ASCENSION_SPELL = 760179

KEYS = {"spellpattern": ("spellid",), "spelleffect": ("spellid",), "petstats": ("spellid",),
        "pettemplate": ("spell", "weapon"), "cmcatalog": ("id",)}

# (rank id, its number, its level, the copy's id, the magician's spell copied, its name, its pet's sheet id)
BIRTHRIGHTS = [
    (2177, 1, 15, 760180, 640, "Water Elementaling", "0019"),
    (2178, 2, 25, 760181, 669, "Water Elementalkin", "001D"),
    (2179, 3, 35, 760182, 900688, "Water Elementalkin (29)", "0025"),
    (2180, 4, 45, 760183, 900699, "Water Elemental (34)", "0029"),
    (2181, 5, 55, 760184, 700557, "Summon Ghilan", "0033"),
]
# the Ascension: (rank id, its level, the copy's id, the magician's spell copied, its name, its pet's sheet id, the
# copy's name, its pet's level)
ASCENSION = (2182, 60, ASCENSION_SPELL, 900745, "Summon Ghilan (60)", "003A", "Aquatic Ascension", 51)
BIRTHRIGHT_COST, ASCENSION_COST = 15, 50


def compact(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False)


def go_json(m):
    return json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def key_of(tbl, row):
    return {c: row[c] for c in KEYS[tbl]}


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


def rank(id, category, mastery, rnk, name, parent, cost, level, add, remove, description, require_ranks=""):
    """add, remove: (spell name, id, level) or None."""
    return {"id": id, "category": category, "classes": 32767, "races": RACES, "mastery": mastery, "rank": rnk,
            "name": name, "parent": parent, "cost": cost, "level": level, "spent": 0, "minstats": "",
            "requires": "", "require_spells": "", "effects": "",
            "adds": f"{add[0]}@{add[2]}" if add else "", "add_spells": str(add[1]) if add else "",
            "removes": f"{remove[0]}@{remove[2]}" if remove else "", "remove_spells": str(remove[1]) if remove else "",
            "description": description, "enabled": 1, "source": SOURCE, "require_ranks": require_ranks,
            "require_quests": ""}


def copies():
    """Each copy: (its id, the magician's spell, its name, its pet's sheet id, the copy's name, its level, the pet's
    level or None for the row's)."""
    out = [(new, src, name, sheet, name, lvl, None) for _, _, lvl, new, src, name, sheet in BIRTHRIGHTS]
    _, lvl, new, src, name, sheet, copy_name, pet_level = ASCENSION
    return out + [(new, src, name, sheet, copy_name, lvl, pet_level)]


def summoned(effects):
    """The pet sheet id a spelleffect's effects summon, "" none."""
    try:
        for e in json.loads(effects).get("effects", []):
            if e.get("kind") == "summon":
                return (e.get("pet") or {}).get("sheet_pet_id", "")
    except (ValueError, AttributeError):
        pass
    return ""


def desc_of(name, pet, level, prev):
    return (f"CM: The sea answers the mermaid: a {pet} (level {level}) of the magician's water comes to her aid, to mend "
            f"her and strike with cold." + (f" It takes the place of {prev}." if prev else ""))


def build(db):
    """Every row to write, in order: [(table, row)], and refusals (the magician's records not as expected)."""
    rows, refusals, pets = [], [], {}
    for new, src, name, sheet, copy_name, lvl, pet_level in copies():
        sp = row_of(db, "spellpattern", {"spellid": src})
        se = row_of(db, "spelleffect", {"spellid": src})
        ps = row_of(db, "petstats", {"spellid": src})
        if sp is None or sp["spellname"] != name:
            refusals.append(f"spellpattern {src} is {sp and sp['spellname']!r}, not the magician's {name!r}")
            continue
        if se is None or summoned(se["effects"]) != sheet:
            refusals.append(f"spelleffect {src} summons {se and summoned(se['effects'])!r}, not workbook pet {sheet}")
            continue
        if ps is None or ps["sheet_pet_id"] != sheet:
            refusals.append(f"petstats {src} is {ps and ps['sheet_pet_id']!r}, not workbook pet {sheet}")
            continue
        pets[new] = (ps["name"], pet_level or ps["level"])
        sp2 = dict(sp, spellid=new, abilitylvl=lvl, spellname=copy_name, spelldesc="")
        eff = json.loads(se["effects"])
        eff["level"] = lvl
        se2 = dict(se, spellid=new, effects=compact(eff), source=SOURCE)
        note = (f"the Mermaid's racial copy of the magician's {name!r} ({src}): its row as at the copy"
                + (f", level {pet_level} (Zauz: \"a lvl {pet_level} Ghilan\")" if pet_level else "")
                + " (tools/mermaid_pets_20261113.py)")
        ps2 = dict(ps, spellid=new, note=note, source=SOURCE)
        if pet_level:
            ps2["level"] = pet_level
        rows += [("spellpattern", sp2), ("spelleffect", se2), ("petstats", ps2)]
    if refusals:
        return rows, refusals
    # spelldesc: what each pet is, and what it replaces
    prev = None
    for t, r in rows:
        if t == "spellpattern":
            pet, level = pets[r["spellid"]]
            r["spelldesc"] = desc_of(r["spellname"], pet, level, prev)
            prev = r["spellname"]
    _, _, new, _, _, _, copy_name, _ = ASCENSION
    rows.append(("pettemplate", {"spell": copy_name.lower(), "weapon": 0, "template": "pet water", "swing_class": -1,
                                 "swing": -1, "round_ms": 0,
                                 "note": "the Mermaid's Aquatic Ascension, the magician's water pet (its Ghilan)",
                                 "source": SOURCE}))
    # the ranks
    prev = None
    for id, n, lvl, new, _, name, _ in BIRTHRIGHTS:
        pet, plevel = pets[new]
        rows.append(("cmcatalog", rank(
            id, 1, "Aquatic Birthright", n, f"Aquatic Birthright {n}", id - 1 if n > 1 else 0, BIRTHRIGHT_COST, lvl,
            (name, new, lvl), prev,
            f"The sea's spirits answer a mermaid's call: she learns {name}, a level {plevel} {pet} of the magician's "
            f"water that mends her and strikes with cold, though it is ever well below her own level."
            + (f" It takes the place of {prev[0]}." if prev else ""))))
        prev = (name, new, lvl)
    id, lvl, new, _, _, _, copy_name, _ = ASCENSION
    pet, plevel = pets[new]
    rows.append(("cmcatalog", rank(
        id, 4, "Siren Royal", 4, copy_name, SIREN_ROYAL, ASCENSION_COST, lvl, (copy_name, new, lvl), prev,
        f"The deep's nobility answers the siren royal: she learns {copy_name}, which calls a level {plevel} {pet} to "
        f"mend her and strike with cold, in place of {prev[0]}. Needs Aquatic Birthright 1 to 5.",
        ",".join(str(b[0]) for b in BIRTHRIGHTS))))
    return rows, refusals


def plan(db):
    """(lines, refusals, rows)."""
    lines, refusals = [], []
    for t in ("cmcatalog", "spellpattern", "spelleffect", "petstats", "pettemplate", "racedefs", "gm_audit"):
        if not table(db, t):
            return lines, [f"no {t} table here"], []
    rd = db.execute("SELECT name FROM racedefs WHERE id = ?", (MERMAID,)).fetchone()
    if rd is None or rd[0] != "Mermaid":
        refusals.append(f"racedefs {MERMAID} is {rd and rd[0]!r}, not the Mermaid (the kit, tools/mermaid_kit_20261112.py, "
                        f"is not on this database)")
    sr = row_of(db, "cmcatalog", {"id": SIREN_ROYAL})
    if sr is None or sr["name"] != "Siren Royal" or sr["races"] != RACES:
        refusals.append(f"cmcatalog {SIREN_ROYAL} is {sr and sr['name']!r}, not the kit's Siren Royal")
    rows, bad = build(db)
    refusals += bad
    if bad:
        return lines, refusals, rows
    for tbl in KEYS:
        cols = {r[1] for r in db.execute(f"PRAGMA table_info({tbl})")}
        for t, r in rows:
            if t == tbl and (missing := set(r) - cols):
                return lines, refusals + [f"{tbl} lacks {sorted(missing)}"], rows
    ours = {r["id"] for t, r in rows if t == "cmcatalog"} | set(KIT_IDS)
    for id, name, races in db.execute("SELECT id, name, races FROM cmcatalog WHERE (races >> ?) & 1 = 1 "
                                      "AND (races & 2047) != 2047", (MERMAID,)):
        if id not in ours:
            refusals.append(f"cmcatalog {id} ({name}) has the Mermaid's bit (races {races}): look at it first")
    for key, nm in db.execute("SELECT spellid, spellname FROM spellpattern WHERE spellname = ?", (ASCENSION[6],)):
        if key != ASCENSION_SPELL:
            refusals.append(f"spellpattern {key} is named {nm!r} already")
    for t, r in rows:
        if (now := row_of(db, t, key_of(t, r))) is not None:
            refusals.append(f"{t} {go_json(key_of(t, r))} is taken: "
                            f"{now.get('name') or now.get('spellname') or now.get('template') or now.get('line')!r}")
    for t, r in rows:
        if t == "spellpattern":
            lines.append(f"spellpattern {r['spellid']} {r['spellname']!r}: level {r['abilitylvl']}, cast {r['casttime']}, "
                         f"power {r['power']}, recast {r['recast']}, range {r['range']}")
        elif t == "spelleffect":
            lines.append(f"spelleffect  {r['spellid']}: {r['line']}, fx {r['fxdict']}, pet {summoned(r['effects'])}")
        elif t == "petstats":
            ab = ", ".join(f"{a['kind']} {a.get('spell_id', '')} every {a['every']:g}s"
                           + (f" below {a['below']}%" if "below" in a else "") for a in json.loads(r["abilities"]))
            lines.append(f"petstats     {r['spellid']}: {r['name']!r} level {r['level']}, HP {r['hp']}, power {r['power']}, "
                         f"AC {r['ac']}, {r['type']}, size {r['size']}; {ab}")
        elif t == "pettemplate":
            lines.append(f"pettemplate  {r['spell']!r} -> {r['template']!r}")
        else:
            lines.append(f"cmcatalog {r['id']} {'racial' if r['category'] == 1 else 'master'} {r['mastery']} {r['rank']} "
                         f"{r['name']!r}: cost {r['cost']}, level {r['level']}, parent {r['parent']}, adds {r['adds']} "
                         f"({r['add_spells']})" + (f", removes {r['removes']} ({r['remove_spells']})" if r["removes"] else "")
                         + (f", require_ranks {r['require_ranks']}" if r["require_ranks"] else ""))
    return lines, refusals, rows


def insert(db, tbl, row):
    cols = list(row)
    db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [row[c] for c in cols])


LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,      -- cmcatalog, spellpattern, spelleffect, petstats or pettemplate
    rowkey  TEXT NOT NULL,      -- its key (JSON)
    before  TEXT,               -- NULL: every row here is written new
    after   TEXT NOT NULL,      -- the row as written (JSON)
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def applied(db):
    """True when the log is there and every row in it is as written; False when it is not there; None when it is
    there but a row has changed since."""
    if not table(db, LOG):
        return False
    for tbl, rk, after in db.execute(f"SELECT tbl, rowkey, after FROM {LOG}").fetchall():
        now = row_of(db, tbl, json.loads(rk))
        if now is None or go_json(now) != after:
            return None
    return True


def apply(db, rows):
    db.execute(LOG_SCHEMA)
    for t, r in rows:
        insert(db, t, r)
        k = key_of(t, r)
        after = row_of(db, t, k)
        a = audit(db, "insert", t, k, None, after, DETAIL)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, NULL, ?, ?)",
                   (t, go_json(k), go_json(after), a))


def undo(db):
    """(back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, tbl, rk, after in db.execute(f"SELECT seq, tbl, rowkey, after FROM {LOG} ORDER BY seq DESC").fetchall():
        k = json.loads(rk)
        now = row_of(db, tbl, k)
        if now is None or go_json(now) != after:
            left += 1
            print(f"  left: {tbl} {rk}, changed since it was written: {now}")
            continue
        where = " AND ".join(f"{c} = ?" for c in KEYS[tbl])
        db.execute(f"DELETE FROM {tbl} WHERE {where}", tuple(k[c] for c in KEYS[tbl]))
        audit(db, "delete", tbl, k, now, None, "undo: " + DETAIL)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if left == 0:
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
            print(f"put back {back}, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        state = applied(db)
        if state:
            print("applied already (every row of mermaid_pets_1113_log as written): nothing done")
            return 0
        if state is None:
            print(f"{LOG} is there but its rows are not as written: --undo first")
            return 1
        lines, refusals, rows = plan(db)
        for line in lines:
            print(line)
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not a.apply:
            print(f"report only: {len(rows)} rows new (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            apply(db, rows)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {len(rows)} rows new; in gm_audit and {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
