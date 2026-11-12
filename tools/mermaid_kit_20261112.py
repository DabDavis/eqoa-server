#!/usr/bin/env python3
"""The Mermaid's (race 32) racial kit as the owner designed it (2026-11-12), as the Wisp's (tools/wisp_kit_20261112.py)
and the Jal'Raeth's (tools/jalraeth_masteries_20261111.py) were built. The design, verbatim:

  Racials
  - Hydrate: 20 CMs -- grants a drink/food spell called "Hydrate" that scales with level, weaker than store-bought of
    the same level. +5 PoT/HoT.
  - Splash: +5 PoT, 10 CMs, adds a cantrip CR nuke exactly the same as the gnome racial nuke, scales with level, 5% CHA
    mod. Spell name "Splash", 30 range.
  - Moisture: 10 CMs, +50 CR, -50 LR. CM description "You are moist".
  MC -- Siren Royal: +50 WIS/CHA max, +25 STA max, 2 DF, +50 CR, -50 LR, +500 power, +10 PoT/HoT.
  MCA 1: Siren's Call -- a mana tap that drains 750 power from the target and gives 750 power to the group. Costs 500
    power, 2-minute recast.
  MCA 2: Quintashi's Kiss -- a 1000 CR nuke that debuffs 25 wis,int cr, 20% CHA mod, 1-minute recast, debuff duration
    1 minute.

Left out on the owner's word ("Ignore the stuff about mage pet, have to do the math on the pets when I get back"), ids
kept free for them, NO rows written: Aquatic Birthright 1-5 (racials, cmcatalog 2177-2181) and MCA 3 Aquatic Ascension
(Siren Royal rank 4, cmcatalog 2182; its spell, if it is one, 760179).

Six cmcatalog rows for race 32 (races = 1 << 32, entity/race.go raceInMask; every class):

  2171 racial  Hydrate             cost 20, level 6: PoT +5, HoT +5, and the ability Hydrate (760175) @6
  2172 racial  Splash              cost 10, level 6: PoT +5, and the ability Splash (760176) @6
  2173 racial  Moisture            cost 10, level 6: CR +50, LR -50; its description "You are moist."
  2174 master  Siren Royal 1       cost 200, level 55, 500 spent (every Master Class's rank 1): WIS cap +50, CHA cap
               +50, STA cap +25, DEF MOD +2 (the design's "2 DF", read as the Jal'Raeth's "+2DF": DEF MOD, as their
               Devoted's "Def Mod:2"), CR +50, LR -50, max power +500, PoT +10, HoT +10
  2175 master  Siren Royal 2 "Siren's Call"      cost 75, level 60: the ability Siren's Call (760177) @60
  2176 master  Siren Royal 3 "Quintashi's Kiss"  cost 75, level 60: the ability Quintashi's Kiss (760178) @60

Four spells (spellpattern + spelleffect):

  760175 Hydrate (as the Wisp's Ponder 760173: self, cast 3, power 10, recast 10, 30 minutes, Ponder's icon and
         visual): PoT and HoT each 80% of the caster's level. A grocer's drink gives PoT and its meal HoT equal to the
         item's level (tools/stock_grocers.py), so Hydrate is four fifths of a store-bought of the caster's level (48
         each at 60, against 54 and 49 for the best a grocer sells). A stack line of its own (its id), as Ponder's, so
         it stacks with a meal, a drink and every buff.
  760176 Splash (the gnome's Cantrip 901784 exactly: an enemy, level 6, range 30, no cast time, power 30, recast 10,
         its icon; 5 and 300% of the caster's level), its DEX 10% replaced by CHA 5%, resisted by CR (Cantrip names no
         resist), Ghilan Wash's watery visual
  760177 Siren's Call (as the enchanter's Power Storm 700467, the "Drain Group" power line: an enemy, range 30, its
         icon and visual): drains 750 power, given to the caster and each of its group alive within 30 of it
         (lifetap.go drainersLocked); no cast time, power 500, recast 120 s. A creature keeps no power (npc.go: only a
         pet's), so on one the group gets the 750 as from every Power Storm; on a duel's opponent what it had is taken,
         halved as a duel's hits are, and given (duelfight.go)
  760178 Quintashi's Kiss (as Ghilan Wash 701146, a 60 CR nuke): an enemy, range 30, no cast time, power 250 (the
         design names no cost: the Wisp's and Jal'Raeth's MCA strikes' 250), recast 60 s: 1000 and 20% of the caster's
         CHA, resisted by CR; and WIS -25, INT -25 on the target for 1 minute (the design's "debuffs 25 wis,int cr"
         read as the debuff's two stats, its "cr" the resist the whole spell goes by; not a CR debuff)

Refused, nothing written, unless racedefs 32 is the Mermaid, every id is free (or is exactly this tool's row: applied
already), no other cmcatalog row has bit 32 set and no other spell has one of the four names. Report only by default;
--apply in one transaction, a gm_audit line a row, log mermaid_kit_1112_log; --undo exact (a row changed since is left
and said). cmcatalog, spellpattern and spelleffect are read at the start: a restart.

    tools/mermaid_kit_20261112.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "mermaid_kit_1112_log"
DETAIL = "The Mermaid's kit (the owner's design, 2026-11-12): tools/mermaid_kit_20261112.py"
WHO, VIA = "dab", "tool"
MERMAID = 32
RACES = 1 << MERMAID
SOURCE = "ours (the owner's design, 2026-11-12: the Mermaid's kit)"
HYDRATE, SPLASH, CALL, KISS = 760175, 760176, 760177, 760178
# kept free for the owner's pet math: Aquatic Birthright 1-5 and Aquatic Ascension (no rows)
RESERVED_CM, RESERVED_SPELL = (2177, 2178, 2179, 2180, 2181, 2182), 760179

KEYS = {"spellpattern": "spellid", "spelleffect": "spellid", "cmcatalog": "id"}


def compact(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False)


def stat(eid, st, amount, **more):
    return {"id": eid, "type": "Statistic Change", "kind": "stat", "stat": st, "amount": amount, **more}


def hit(eid, amount, **more):
    return {"id": eid, "type": "Dmg/Heal", "kind": "hp", "amount": amount, **more}


LVL80 = {"level_pct": 80}

# (spellpattern row, spelleffect row): the pattern's columns as its kin's, the effects in their compact form
SPELLS = [
    ({"spellid": HYDRATE, "abilitylvl": 6, "unk2": 0, "unk3": 0, "range": 16528, "casttime": 3, "power": 10,
      "iconColor": 593297002, "icon": 1028836133, "scope": 0, "recast": 10, "eqprequire": -1,
      "spellname": "Hydrate",
      "spelldesc": "CM: The mermaid draws the sea's water into herself: health and power return the faster, the more "
                   "with her level, though never as well as a grocer's fare."},
     {"spellid": HYDRATE, "fxdict": 2826755147, "source": SOURCE,
      "line": "Self, Increase HP per tic by 0, Increase Power per tic by 0 ; Increase 80% based on Level, 30min",
      "effects": compact({"v": 1, "scope": "self", "target_type": "Self", "harmful": False, "duration_s": 1800,
                          "stack": HYDRATE, "level": 6, "min_level": 1, "req_equipment": "Any",
                          "sheet_type": "Statistic Change",
                          "effects": [stat("F083", "hot", 0, **LVL80), stat("F084", "pot", 0, **LVL80)]})}),
    ({"spellid": SPLASH, "abilitylvl": 6, "unk2": 0, "unk3": 0, "range": 16880, "casttime": 0, "power": 30,
      "iconColor": 486870879, "icon": 829958967, "scope": 1, "recast": 10, "eqprequire": -1,
      "spellname": "Splash",
      "spelldesc": "CM: A splash of cold sea water at the target. Modified by CHA."},
     {"spellid": SPLASH, "fxdict": 2654136143, "source": SOURCE,
      "line": "Target, Decrease Hitpoints by 5 ; Increase 5% based on CHA",
      "effects": compact({"v": 1, "scope": "target", "target_type": "Enemy", "harmful": True, "stack": 0,
                          "level": 6, "min_level": 1, "req_equipment": "Any", "sheet_type": "Dmg/Heal",
                          "effects": [hit("F085", -5, scale="cha", scale_pct=5, level_pct=300, resist="cr")]})}),
    ({"spellid": CALL, "abilitylvl": 60, "unk2": 0, "unk3": 0, "range": 16880, "casttime": 0, "power": 500,
      "iconColor": 486870879, "icon": -1664015648, "scope": 1, "recast": 120, "eqprequire": -1,
      "spellname": "Siren's Call",
      "spelldesc": "CM: The siren's song pulls the power from the target's mind and pours it into her companions."},
     {"spellid": CALL, "fxdict": 2106297732, "source": SOURCE,
      "line": "Target, Drain Group Power by 750",
      "effects": compact({"v": 1, "scope": "target", "target_type": "Enemy", "harmful": True, "stack": 0,
                          "level": 60, "min_level": 1, "req_equipment": "Any", "sheet_type": "Drain Group",
                          "effects": [{"id": "F086", "type": "Drain Group", "kind": "drain_group", "stat": "power",
                                       "amount": -750}]})}),
    ({"spellid": KISS, "abilitylvl": 60, "unk2": 0, "unk3": 0, "range": 16880, "casttime": 0, "power": 250,
      "iconColor": 486870879, "icon": 282007337, "scope": 1, "recast": 60, "eqprequire": -1,
      "spellname": "Quintashi's Kiss",
      "spelldesc": "CM: Quintashi's cold kiss strikes the target and dulls its wisdom and intelligence for a minute. "
                   "Modified by CHA."},
     {"spellid": KISS, "fxdict": 2972655020, "source": SOURCE,
      "line": "Target, Decrease Hitpoints by 1000 ; Increase 20% based on CHA, Decrease WIS by 25, Decrease INT by 25, "
              "1min",
      "effects": compact({"v": 1, "scope": "target", "target_type": "Enemy", "harmful": True, "duration_s": 60,
                          "stack": 0, "level": 60, "min_level": 1, "req_equipment": "Any", "sheet_type": "Dmg/Heal",
                          "effects": [hit("F087", -1000, scale="cha", scale_pct=20, resist="cr"),
                                      stat("F088", "wis", -25, resist="cr", duration_s=60),
                                      stat("F089", "int", -25, resist="cr", duration_s=60)]})}),
]


def rank(id, category, mastery, rnk, name, parent, cost, level, spent, effects, description, adds=()):
    return {"id": id, "category": category, "classes": 32767, "races": RACES, "mastery": mastery, "rank": rnk,
            "name": name, "parent": parent, "cost": cost, "level": level, "spent": spent, "minstats": "",
            "requires": "", "require_spells": "", "effects": effects,
            "adds": "; ".join(f"{n}@{lv}" for n, _, lv in adds), "add_spells": ",".join(str(s) for _, s, _ in adds),
            "removes": "", "remove_spells": "", "description": description, "enabled": 1, "source": SOURCE,
            "require_ranks": "", "require_quests": ""}


ROWS = [
    rank(2171, 1, "Hydrate", 1, "Hydrate", 0, 20, 6, 0, "PoT:5,HoT:5",
         "The sea is never far from a mermaid: her health and power return the faster, and she learns Hydrate, a "
         "draught of the sea's own water that grows with her level, though never as good as a grocer's.",
         [("Hydrate", HYDRATE, 6)]),
    rank(2172, 1, "Splash", 1, "Splash", 0, 10, 6, 0, "PoT:5",
         "A mermaid's power returns the faster, and she learns Splash, a cold strike of sea water that grows with her "
         "level. Modified by CHA.", [("Splash", SPLASH, 6)]),
    rank(2173, 1, "Moisture", 1, "Moisture", 0, 10, 6, 0, "CR:50,LR:-50", "You are moist."),
    rank(2174, 4, "Siren Royal", 1, "Siren Royal", 0, 200, 55, 500,
         "WISMax:50,CHAMax:50,STAMax:25,Def Mod:2,CR:50,LR:-50,Pow:500,PoT:10,HoT:10",
         "The siren royal commands the deep: wiser, more beguiling and hardier, at home in the cold, and her power a "
         "tide that does not ebb. WIS and CHA cap +50, STA cap +25, DEF MOD +2, CR +50, LR -50, max power +500, PoT "
         "and HoT +10."),
    rank(2175, 4, "Siren Royal", 2, "Siren's Call", 2174, 75, 60, 0, "",
         "The siren sings the power out of her foe and into her companions: 750 power drained from the target and "
         "given to the group. 2 minute recast.", [("Siren's Call", CALL, 60)]),
    rank(2176, 4, "Siren Royal", 3, "Quintashi's Kiss", 2174, 75, 60, 0, "",
         "Quintashi's cold kiss: 1000 cold damage + 20% Cha Mod, and the target's WIS and INT -25 for a minute. "
         "1 minute recast.", [("Quintashi's Kiss", KISS, 60)]),
]

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,      -- cmcatalog, spellpattern or spelleffect
    rowkey  INTEGER NOT NULL,   -- its id or spellid
    before  TEXT,               -- NULL: every row here is written new
    after   TEXT NOT NULL,      -- the row as written (JSON)
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def row_of(db, tbl, key):
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {KEYS[tbl]} = ?", (key,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, tbl, key, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json({KEYS[tbl]: key}),
                      None if before is None else go_json(before), None if after is None else go_json(after), detail))
    return cur.lastrowid


def same(now, want):
    return now is not None and all(now.get(k) == v for k, v in want.items())


def inserts():
    """Every new row, in order: (table, row)."""
    out = []
    for sp, se in SPELLS:
        out += [("spellpattern", sp), ("spelleffect", se)]
    return out + [("cmcatalog", r) for r in ROWS]


def plan(db):
    """(lines, refusals, applied)."""
    lines, refusals = [], []
    for t in ("cmcatalog", "spellpattern", "spelleffect", "racedefs", "gm_audit"):
        if not table(db, t):
            return lines, [f"no {t} table here"], False
    rd = db.execute("SELECT name FROM racedefs WHERE id = ?", (MERMAID,)).fetchone()
    if rd is None or rd[0] != "Mermaid":
        refusals.append(f"racedefs {MERMAID} is {rd!r}, not the Mermaid")
    for tbl, sample in (("cmcatalog", ROWS[0]), ("spellpattern", SPELLS[0][0]), ("spelleffect", SPELLS[0][1])):
        cols = {r[1] for r in db.execute(f"PRAGMA table_info({tbl})")}
        if missing := set(sample) - cols:
            return lines, [f"{tbl} lacks {sorted(missing)}"], False
    if all(same(row_of(db, t, r[KEYS[t]]), r) for t, r in inserts()):
        return lines, refusals, True
    ours = {r["id"] for r in ROWS}
    for id, name, races in db.execute("SELECT id, name, races FROM cmcatalog WHERE (races >> ?) & 1 = 1 "
                                      "AND (races & 2047) != 2047", (MERMAID,)):
        if id not in ours:
            refusals.append(f"cmcatalog {id} ({name}) has the Mermaid's bit (races {races}): look at it first")
    names = {sp["spellname"] for sp, _ in SPELLS}
    for key, nm in db.execute("SELECT spellid, spellname FROM spellpattern"):
        if nm in names and key not in (HYDRATE, SPLASH, CALL, KISS):
            refusals.append(f"spellpattern {key} is named {nm!r} already")
    for t, r in inserts():
        if (now := row_of(db, t, r[KEYS[t]])) is not None:
            refusals.append(f"{t} {r[KEYS[t]]} is taken: {now.get('name') or now.get('spellname') or now.get('line')!r}")
    for id in RESERVED_CM:
        if (now := row_of(db, "cmcatalog", id)) is not None:
            lines.append(f"note: cmcatalog {id}, kept for Aquatic Birthright/Ascension, is taken: {now['name']!r}")
    for sp, se in SPELLS:
        lines.append(f"spellpattern {sp['spellid']} {sp['spellname']!r}: level {sp['abilitylvl']}, scope {sp['scope']}, "
                     f"cast {sp['casttime']}, power {sp['power']}, recast {sp['recast']}")
        lines.append(f"spelleffect  {se['spellid']}: {se['line']}  {se['effects']}")
    for r in ROWS:
        lines.append(f"cmcatalog {r['id']} {'racial' if r['category'] == 1 else 'master'} {r['mastery']} {r['rank']} "
                     f"{r['name']!r}: cost {r['cost']}, level {r['level']}, parent {r['parent']}, effects {r['effects']}"
                     + (f", adds {r['adds']} ({r['add_spells']})" if r["adds"] else ""))
    return lines, refusals, False


def insert(db, tbl, row):
    cols = list(row)
    db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [row[c] for c in cols])


def apply(db):
    db.execute(LOG_SCHEMA)
    for t, r in inserts():
        insert(db, t, r)
        key = r[KEYS[t]]
        after = row_of(db, t, key)
        a = audit(db, "insert", t, key, None, after, DETAIL)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, NULL, ?, ?)",
                   (t, key, json.dumps(after, sort_keys=True), a))


def undo(db):
    """(back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, tbl, key, after in db.execute(f"SELECT seq, tbl, rowkey, after FROM {LOG} ORDER BY seq DESC").fetchall():
        now = row_of(db, tbl, key)
        if now is None or json.dumps(now, sort_keys=True) != after:
            left += 1
            print(f"  left: {tbl} {key}, changed since it was written: {now}")
            continue
        db.execute(f"DELETE FROM {tbl} WHERE {KEYS[tbl]} = ?", (key,))
        audit(db, "delete", tbl, key, now, None, "undo: " + DETAIL)
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
        lines, refusals, applied = plan(db)
        for line in lines:
            print(line)
        if applied:
            print("applied already (the kit's rows are this tool's): nothing done")
            return 0
        if table(db, LOG):
            print(f"{LOG} is there but the rows are not as written: --undo first")
            return 1
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not a.apply:
            print(f"report only: {len(inserts())} rows new (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            apply(db)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {len(inserts())} rows new; in gm_audit and {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
