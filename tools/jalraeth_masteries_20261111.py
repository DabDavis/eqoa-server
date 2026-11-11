#!/usr/bin/env python3
"""The Jal'Raeth's racial masteries, Master Class and its two abilities (the owner's design, 2026-11-11, the image
jalreathe.webp: "MC Devoted +50sta/wis max +25dex/cha max +200 ac +2DF +2OF. MCA Path of the Devout +250 ac +500hp +2OF
+2DF +25 sta +25 wis. Touch of the Devoted Debuffs 125 sta/agi/ac 30% WIS mod; recast 30 sec duration 1 minute.
Racials: Metaphysical guidance +100 AC -10 cms. Guided Meditation +5 pot/hot group buff scales with level- 25 cms 30min
dur. Bastion of Will 1DF +100 AC -25 cms. Sacred Scripture +10 WIS, +20 WIS, MAX 25 CMS").

Eight cmcatalog rows for race 30, the Jal'Raeth (races = 1 << 30, entity/race.go raceInMask; every class), built as
every other race's (the Wisp's tools/wisp_kit_20261111.py, the gargoyle's Stoneguard 2131-2133):

  2161 racial  Metaphysical Guidance    cost 10, level 6: AC +100
  2162 racial  Guided Meditation        cost 25, level 6: the ability Guided Meditation (760172) @6
  2163 racial  Bastion of Will          cost 25, level 6: DEF MOD +1, AC +100
  2164 racial  Sacred Scripture 1       cost 10, level 6: WIS +10
  2165 racial  Sacred Scripture 2       cost 15, level 20 (Hardened Shell 2's level): WIS +10 more, +20 in all
               (owned ranks add up, entity/cmbuy.go refreshCMLocked: the design's "+10 WIS, +20 WIS" read as the
               line's totals; 10 + 15 = 25, its "MAX 25 CMS")
  2166 master  Devoted 1 "Devoted"      cost 200, level 55, 500 spent (every Master Class's rank 1): STA cap +50,
               WIS cap +50, DEX cap +25, CHA cap +25, AC +200, DEF MOD +2, OFF MOD +2
  2167 master  Devoted 2 "Path of the Devout"   cost 75, level 60: the ability Path of the Devout (760170) @60
  2168 master  Devoted 3 "Touch of the Devoted" cost 75, level 60: the ability Touch of the Devoted (760171) @60

Three spells (spellpattern + spelleffect), each laid out as its nearest kin's record:

  760170 Path of the Devout (as Living Stone 760156: self, no cast time, 500 power, 1 minute, recast 10 minutes):
         max HP +500, AC +250, OFF MOD +2, DEF MOD +2, STA +25, WIS +25; a stack line of its own (its id), as the
         race abilities' since petition 1579, so it stacks with every buff
  760171 Touch of the Devoted (as Dirty Trick 760125: an enemy, no cast time, 250 power): STA, AGI and AC each
         -125, and 30% of the caster's WIS more ("scale":"wis","scale_pct":30, as Hearth Blessing's heal), for
         1 minute, recast 30 seconds
  760172 Guided Meditation (as the human's Leadership 1791: the group within 30, cast 2, 30 power, recast 10,
         30 minutes): PoT +5 and HoT +5, each with level_pct 20 (Life Grasp's HoT 5 + 20%: +1 a 5 levels, 17 at 60);
         a stack line of its own (its id), so it stacks with the classes' group buffs

Refused, nothing written, unless racedefs 30 is the Jal'Raeth, every id is free (or is exactly this tool's row: applied
already) and no other cmcatalog row has bit 30 set. Report only by default; --apply in one transaction, a gm_audit line
a row, log jalraeth_kit_1111_log; --undo exact (a row changed since is left and said). cmcatalog, spellpattern and
spelleffect are read at the start: a restart.

    tools/jalraeth_masteries_20261111.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "jalraeth_kit_1111_log"
DETAIL = "The Jal'Raeth's racials, Master Class and abilities (the owner's design, 2026-11-11): " \
         "tools/jalraeth_masteries_20261111.py"
WHO, VIA = "dab", "tool"
JALRAETH = 30
RACES = 1 << JALRAETH
SOURCE = "ours (the owner's design, 2026-11-11: the Jal'Raeth's kit)"

PATH, TOUCH, MEDITATION = 760170, 760171, 760172


def compact(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False)


def stat(eid, st, amount, **more):
    return {"id": eid, "type": "Statistic Change", "kind": "stat", "stat": st, "amount": amount, **more}


def mod(eid, kind, amount, **more):
    return {"id": eid, "type": "Statistic Change", "kind": kind, "amount": amount, **more}


WIS30 = {"scale": "wis", "scale_pct": 30}
LVL20 = {"level_pct": 20}

# (spellpattern row, spelleffect row): the pattern's columns as its kin's, the effects in their compact form
SPELLS = [
    ({"spellid": PATH, "abilitylvl": 60, "unk2": 0, "unk3": 0, "range": 16528, "casttime": 0, "power": 500,
      "iconColor": 486870879, "icon": -630227479, "scope": 0, "recast": 600, "eqprequire": -1,
      "spellname": "Path of the Devout",
      "spelldesc": "CM: The devout walk their path untouched: health, armor, wisdom and stamina, and a surer hand in "
                   "a fight, for a minute."},
     {"spellid": PATH, "fxdict": 3266773244, "source": SOURCE,
      "line": "Self, Increase Max HP by 500, Increase AC by 250, Increase Offensive Mod by 2, Increase Defensive Mod "
              "by 2, Increase STA by 25, Increase WIS by 25, 1min",
      "effects": compact({"v": 1, "scope": "self", "target_type": "Self", "harmful": False, "duration_s": 60,
                          "stack": PATH, "level": 60, "min_level": 1, "req_equipment": "Any",
                          "sheet_type": "Statistic Change",
                          "effects": [stat("F068", "maxhp", 500), stat("F069", "ac", 250), mod("F070", "off_mod", 2),
                                      mod("F071", "def_mod", 2), stat("F072", "sta", 25), stat("F073", "wis", 25)]})}),
    ({"spellid": TOUCH, "abilitylvl": 60, "unk2": 0, "unk3": 0, "range": 16544, "casttime": 0, "power": 250,
      "iconColor": 486870879, "icon": -861288699, "scope": 1, "recast": 30, "eqprequire": -1,
      "spellname": "Touch of the Devoted",
      "spelldesc": "CM: A touch of the devoted's will saps the target's stamina, agility and armor for a minute. "
                   "Modified by WIS."},
     {"spellid": TOUCH, "fxdict": 1988865824, "source": SOURCE,
      "line": "Target, Decrease STA by 125, Decrease AGI by 125, Decrease AC by 125 ; Increase 30% based on WIS, 1min",
      "effects": compact({"v": 1, "scope": "target", "target_type": "Enemy", "harmful": True, "duration_s": 60,
                          "stack": 0, "level": 60, "min_level": 1, "req_equipment": "Any",
                          "sheet_type": "Statistic Change",
                          "effects": [stat("F074", "sta", -125, **WIS30), stat("F075", "agi", -125, **WIS30),
                                      stat("F076", "ac", -125, **WIS30)]})}),
    ({"spellid": MEDITATION, "abilitylvl": 6, "unk2": 0, "unk3": 0, "range": 16880, "casttime": 2, "power": 30,
      "iconColor": 593297002, "icon": 1028836133, "scope": 2, "recast": 10, "eqprequire": -1,
      "spellname": "Guided Meditation",
      "spelldesc": "CM: The Jal'Raeth lead their companions in meditation: health and power return the faster for "
                   "all nearby allies, the more with the caster's level."},
     {"spellid": MEDITATION, "fxdict": 2826755147, "source": SOURCE,
      "line": "Group, Increase Power per tic by 5, Increase HP per tic by 5, Radius 30, 30min",
      "effects": compact({"v": 1, "scope": "group", "target_type": "Friendly", "harmful": False, "duration_s": 1800,
                          "stack": MEDITATION, "level": 6, "min_level": 1, "radius": 30, "req_equipment": "Any",
                          "sheet_type": "Statistic Change",
                          "effects": [stat("F077", "pot", 5, **LVL20), stat("F078", "hot", 5, **LVL20)]})}),
]


def rank(id, category, mastery, rank, name, parent, cost, level, spent, effects, description, adds=()):
    return {"id": id, "category": category, "classes": 32767, "races": RACES, "mastery": mastery, "rank": rank,
            "name": name, "parent": parent, "cost": cost, "level": level, "spent": spent, "minstats": "",
            "requires": "", "require_spells": "", "effects": effects,
            "adds": "; ".join(f"{n}@{lv}" for n, _, lv in adds), "add_spells": ",".join(str(s) for _, s, _ in adds),
            "removes": "", "remove_spells": "", "description": description, "enabled": 1, "source": SOURCE,
            "require_ranks": "", "require_quests": ""}


ROWS = [
    rank(2161, 1, "Metaphysical Guidance", 1, "Metaphysical Guidance", 0, 10, 6, 0, "AC:100",
         "The Jal'Raeth's meditations reach past the body, and blows meant for it find less to strike."),
    rank(2162, 1, "Guided Meditation", 1, "Guided Meditation", 0, 25, 6, 0, "",
         "The Jal'Raeth teach their companions to meditate as they do: health and power return the faster to the "
         "group, the more as the Jal'Raeth grows.", [("Guided Meditation", MEDITATION, 6)]),
    rank(2163, 1, "Bastion of Will", 1, "Bastion of Will", 0, 25, 6, 0, "Def Mod:1,AC:100",
         "A Jal'Raeth's will stands about it like a wall."),
    rank(2164, 1, "Sacred Scripture", 1, "Sacred Scripture 1", 0, 10, 6, 0, "WIS:10",
         "The scriptures of Envar's temple, learned by heart, deepen a Jal'Raeth's wisdom."),
    rank(2165, 1, "Sacred Scripture", 2, "Sacred Scripture 2", 2164, 15, 20, 0, "WIS:10",
         "The scriptures of Envar's temple, learned by heart, deepen a Jal'Raeth's wisdom."),
    rank(2166, 4, "Devoted", 1, "Devoted", 0, 200, 55, 500,
         "STAMax:50,WISMax:50,DEXMax:25,CHAMax:25,AC:200,Def Mod:2,Off Mod:2",
         "The devoted of the Jal'Raeth have given themselves wholly to their meditations, and body and mind are "
         "the stronger for it."),
    rank(2167, 4, "Devoted", 2, "Path of the Devout", 2166, 75, 60, 0, "",
         "The devout walk their path untouched. Max HP +500, AC +250, Off Mod +2, Def Mod +2, STA +25, WIS +25 for "
         "a minute.", [("Path of the Devout", PATH, 60)]),
    rank(2168, 4, "Devoted", 3, "Touch of the Devoted", 2166, 75, 60, 0, "",
         "A touch of the devoted's will. STA, AGI and AC -125 + 30% Wis Mod on the target for a minute, 30 second "
         "recast.", [("Touch of the Devoted", TOUCH, 60)]),
]

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,      -- cmcatalog, spellpattern or spelleffect
    id      INTEGER NOT NULL,   -- the row's key (id or spellid)
    after   TEXT NOT NULL,      -- the row as written (JSON)
    audit   INTEGER NOT NULL DEFAULT 0
)"""

KEY = {"cmcatalog": "id", "spellpattern": "spellid", "spelleffect": "spellid"}


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def row_of(db, tbl, id):
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {KEY[tbl]} = ?", (id,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, tbl, id, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json({KEY[tbl]: id}),
                      None if before is None else go_json(before), None if after is None else go_json(after), detail))
    return cur.lastrowid


def same(now, want):
    return now is not None and all(now.get(k) == v for k, v in want.items())


def writes():
    """Every row to write, in order: (table, row)."""
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
    rd = db.execute("SELECT name FROM racedefs WHERE id = ?", (JALRAETH,)).fetchone()
    if rd is None or rd[0] != "Jal'Raeth":
        refusals.append(f"racedefs {JALRAETH} is {rd!r}, not the Jal'Raeth")
    for tbl, sample in (("cmcatalog", ROWS[0]), ("spellpattern", SPELLS[0][0]), ("spelleffect", SPELLS[0][1])):
        cols = {r[1] for r in db.execute(f"PRAGMA table_info({tbl})")}
        if missing := set(sample) - cols:
            return lines, [f"{tbl} lacks {sorted(missing)}"], False
    ws = writes()
    applied = all(same(row_of(db, t, r[KEY[t]]), r) for t, r in ws)
    if applied:
        return lines, refusals, True
    ours = {r["id"] for r in ROWS}
    for id, name, races in db.execute("SELECT id, name, races FROM cmcatalog WHERE (races >> ?) & 1 = 1 "
                                      "AND (races & 2047) != 2047", (JALRAETH,)):
        if id not in ours:
            refusals.append(f"cmcatalog {id} ({name}) has the Jal'Raeth's bit already (races {races}): a placeholder "
                            "to look at first")
    names = {sp["spellname"] for sp, _ in SPELLS} | {r["name"] for r in ROWS}
    for t, col in (("spellpattern", "spellname"), ("cmcatalog", "name")):
        for key, nm in db.execute(f"SELECT {KEY[t]}, {col} FROM {t}"):
            if nm in names and key not in ({sp["spellid"] for sp, _ in SPELLS} if t == "spellpattern" else ours):
                refusals.append(f"{t} {key} is named {nm!r} already")
    for t, r in ws:
        now = row_of(db, t, r[KEY[t]])
        if now is not None:
            refusals.append(f"{t} {r[KEY[t]]} is taken: {now.get('name') or now.get('spellname') or now.get('line')!r}")
    for sp, se in SPELLS:
        lines.append(f"spellpattern {sp['spellid']} {sp['spellname']!r}: level {sp['abilitylvl']}, scope {sp['scope']}, "
                     f"cast {sp['casttime']}, power {sp['power']}, recast {sp['recast']}")
        lines.append(f"spelleffect  {se['spellid']}: {se['line']}  {se['effects']}")
    for r in ROWS:
        kind = "racial" if r["category"] == 1 else "master class"
        lines.append(f"cmcatalog {r['id']} {kind} {r['mastery']} {r['rank']} {r['name']!r}: cost {r['cost']}, level "
                     f"{r['level']}, spent {r['spent']}, parent {r['parent']}, effects {r['effects'] or '-'}"
                     + (f", adds {r['adds']} ({r['add_spells']})" if r["adds"] else ""))
    return lines, refusals, False


def apply(db):
    db.execute(LOG_SCHEMA)
    for t, r in writes():
        cols = list(r)
        db.execute(f"INSERT INTO {t} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [r[c] for c in cols])
        row = row_of(db, t, r[KEY[t]])
        a = audit(db, "insert", t, r[KEY[t]], None, row, DETAIL)
        db.execute(f"INSERT INTO {LOG} (tbl, id, after, audit) VALUES (?, ?, ?, ?)",
                   (t, r[KEY[t]], json.dumps(row, sort_keys=True), a))


def undo(db):
    """(back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, t, id, after in db.execute(f"SELECT seq, tbl, id, after FROM {LOG} ORDER BY seq DESC").fetchall():
        now = row_of(db, t, id)
        if now is None or json.dumps(now, sort_keys=True) != after:
            left += 1
            print(f"  left: {t} {id}, changed since it was written: {now}")
            continue
        db.execute(f"DELETE FROM {t} WHERE {KEY[t]} = ?", (id,))
        audit(db, "delete", t, id, now, None, "undo: " + DETAIL)
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
        n = len(writes())
        if applied:
            print(f"applied already (the {n} rows are this tool's): nothing done")
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
            print(f"report only: {n} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            apply(db)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {n} rows, in gm_audit and {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
