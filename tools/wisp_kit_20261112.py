#!/usr/bin/env python3
"""The Wisp's racial kit as its designer Zauz wrote it (the owner, 2026-11-12, the image wisp.webp; the owner: "With
the -50 sta max and -875 hp max it gonna be pretty brutal lol"). Verbatim:

  Will-of-the-Wisp -- 10pot 5 cms
  Trickster Flame-- Unlocks a few glamours, allowing the wisp to take mortal form 10cms
  Ponder-- +5 pot +Level scaling+int moded drink spell scale with level to be little stronger than whatever vendor
    drink pot is at your level if you have max int. 30 CMs
  Stolic Empowerment 1-5. What atrophies the body, strengthens the mind. Basically the powerful mind cm line BUT
    1--5cms -25hp +50pow; 2-- 10cms -50hp + 100 pow; 3-- 15cms -75hp +150 pow; 4-- 20cms -100hp +200 pow;
    5-- 25cms -125hp +250 pow
  Master class title: Transcendent. 50 dex max 50 int max 25 wis max 500 pow max +20 pot +20 LR CR FR
  MCA 1: Transcend the material: Wisps forsake material form to strengthen their metaphysical form seeking power at
    any cost. +50dex/int max +25 dex/int 500pow max +25 pot -50 sta/agi -25sta/agi max -500 hp max -25 hot -250ac
  MCA 2: Metaphysical Wound: Debuff -125 FR, CR, LR moded 10% int 1 min dur 10 min recast.

This replaces the first kit (tools/wisp_kit_20261111.py, the earlier proposal; and tools/wisp_raiment_20261111.py's
Wisp's Raiment, which stays with Trickster Flame), its rows changed in place so who owns them keeps them:

  cmcatalog 2141 Will-of-the-Wisp    unchanged (PoT 10, 5 CMs): as written already
  cmcatalog 2142 Trickster Flame     effects "PoT:5" -> "" (the design gives it glamours alone; the glamours, Remove
                                     Illusion and the Wisp's Raiment stay as they are)
  cmcatalog 2143 Transcendent 1      effects -> DEX cap +50, INT cap +50, WIS cap +25, max power +500, PoT +20,
                                     FR, CR, LR +20 each
  cmcatalog 2144 Transcendent 2      "Unbound Will" -> "Transcend the Material", passive (as Grizzled, Reckless Vigor,
                                     Aged Mind): DEX cap +50, INT cap +50, DEX +25, INT +25, max power +500, PoT +25,
                                     STA -50, AGI -50, STA cap -25, AGI cap -25, max HP -500, HoT -25, AC -250
  cmcatalog 2145 Transcendent 3      "Pale Fire" -> "Metaphysical Wound", the ability 760174 @60 (no passive part)
  cmcatalog 2146 racial Ponder       new: cost 30, level 6: PoT +5, and the ability Ponder (760173) @6
  cmcatalog 2147-2151 racial Stoic Empowerment 1-5   new: costs 5/10/15/20/25 at levels 10/20/30/40/50 (Powerful
                                     Mind's), each rank max HP -25*rank and max power +50*rank; owned ranks add up
                                     (entity/cmbuy.go refreshCMLocked, as Powerful Mind's 50/75/100/125/150), so all
                                     five: max HP -375, max power +750 ("Stolic" read as the designer's typo for
                                     Stoic: "what atrophies the body, strengthens the mind")
  spellpattern+spelleffect 760173 Ponder (self, level 6, cast 3, power 10, recast 10, 30 minutes, as Guided
                                     Meditation's icon and visual): PoT 80% of the caster's level + 3% of its INT; a
                                     stack line of its own (its id), so it stacks with a drink and every buff
  spellpattern+spelleffect 760174 Metaphysical Wound (an enemy, level 60, no cast time, power 250, recast 600 s,
                                     1 minute, as Touch of the Devoted's): FR, CR and LR each -125 and 10% of the
                                     caster's INT more
  spells                             each owner of 2145 now (bought as Pale Fire) gets Metaphysical Wound in its book,
                                     as the purchase would give it (a removal of the rank takes it)

Refused, nothing written, unless racedefs 31 is the Wisp, 2141-2145 are as the first kit and the Raiment left them (or
as this tool leaves them: applied already), 2146-2151, 760173 and 760174 are free (or this tool's) and no other row
has the Wisp's bit (bit 31). Report only by default; --apply in one transaction, a gm_audit line a row, log
wisp_kit_1112_log; --undo exact (a row changed since is left and said). cmcatalog, spellpattern and spelleffect are
read at the start: a restart. Owned ranks and books are read at login.

    tools/wisp_kit_20261112.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "wisp_kit_1112_log"
DETAIL = "The Wisp's kit as Zauz designed it (the owner, 2026-11-12): tools/wisp_kit_20261112.py"
WHO, VIA = "dab", "tool"
WISP = 31
RACES = 1 << WISP
SOURCE = "ours (Zauz's design, the owner 2026-11-12: the Wisp's kit)"
PONDER, WOUND = 760173, 760174
WOUND_RANK = 2145

KEYS = {"spellpattern": "spellid", "spelleffect": "spellid", "cmcatalog": "id", "spells": "id"}


def compact(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False)


def stat(eid, st, amount, **more):
    return {"id": eid, "type": "Statistic Change", "kind": "stat", "stat": st, "amount": amount, **more}


INT10 = {"scale": "int", "scale_pct": 10}

SPELLS = [
    ({"spellid": PONDER, "abilitylvl": 6, "unk2": 0, "unk3": 0, "range": 16528, "casttime": 3, "power": 10,
      "iconColor": 593297002, "icon": 1028836133, "scope": 0, "recast": 10, "eqprequire": -1,
      "spellname": "Ponder",
      "spelldesc": "CM: The wisp turns its thoughts inward and drinks of its own mind: its power returns the faster, "
                   "the more with its level and its intelligence."},
     {"spellid": PONDER, "fxdict": 2826755147, "source": SOURCE,
      "line": "Self, Increase Power per tic by 0 ; Increase 80% based on Level ; Increase 3% based on INT, 30min",
      "effects": compact({"v": 1, "scope": "self", "target_type": "Self", "harmful": False, "duration_s": 1800,
                          "stack": PONDER, "level": 6, "min_level": 1, "req_equipment": "Any",
                          "sheet_type": "Statistic Change",
                          "effects": [stat("F079", "pot", 0, level_pct=80, scale="int", scale_pct=3)]})}),
    ({"spellid": WOUND, "abilitylvl": 60, "unk2": 0, "unk3": 0, "range": 16544, "casttime": 0, "power": 250,
      "iconColor": 486870879, "icon": -861288699, "scope": 1, "recast": 600, "eqprequire": -1,
      "spellname": "Metaphysical Wound",
      "spelldesc": "CM: The wisp tears at what lies beyond the target's flesh: its fire, cold and lightning resists "
                   "fall for a minute. Modified by INT."},
     {"spellid": WOUND, "fxdict": 1988865824, "source": SOURCE,
      "line": "Target, Decrease FR by 125, Decrease CR by 125, Decrease LR by 125 ; Increase 10% based on INT, 1min",
      "effects": compact({"v": 1, "scope": "target", "target_type": "Enemy", "harmful": True, "duration_s": 60,
                          "stack": 0, "level": 60, "min_level": 1, "req_equipment": "Any",
                          "sheet_type": "Statistic Change",
                          "effects": [stat("F080", "fr", -125, **INT10), stat("F081", "cr", -125, **INT10),
                                      stat("F082", "lr", -125, **INT10)]})}),
]


def rank(id, mastery, rnk, name, parent, cost, level, effects, description, adds=()):
    return {"id": id, "category": 1, "classes": 32767, "races": RACES, "mastery": mastery, "rank": rnk,
            "name": name, "parent": parent, "cost": cost, "level": level, "spent": 0, "minstats": "",
            "requires": "", "require_spells": "", "effects": effects,
            "adds": "; ".join(f"{n}@{lv}" for n, _, lv in adds), "add_spells": ",".join(str(s) for _, s, _ in adds),
            "removes": "", "remove_spells": "", "description": description, "enabled": 1, "source": SOURCE,
            "require_ranks": "", "require_quests": ""}


STOIC_DESC = ("What atrophies the body, strengthens the mind: the wisp lets its mortal frame wither and its power "
              "grows the deeper.")
NEW_ROWS = [rank(2146, "Ponder", 1, "Ponder", 0, 30, 6, "PoT:5",
                 "The wisp ponders, and its mind is a well it drinks from: its power returns the faster, and it learns "
                 "Ponder, a draught of its own thoughts stronger with its level and its intelligence.",
                 [("Ponder", PONDER, 6)])]
for _r in range(1, 6):
    NEW_ROWS.append(rank(2146 + _r, "Stoic Empowerment", _r, f"Stoic Empowerment {_r}", 2146 + _r - 1 if _r > 1 else 0,
                         5 * _r, 10 * _r, f"HP:{-25 * _r},Pow:{50 * _r}", STOIC_DESC))

# the rows changed in place: (id, before as the first kit and the Raiment left them, after)
CHANGES = [
    (2142, {"name": "Trickster Flame", "effects": "PoT:5"}, {"effects": ""}),
    (2143, {"name": "Transcendent",
            "effects": "DEXMax:50,INTMax:25,WISMax:25,Pow:500,STRMax:-25,STAMax:-25,AGIMax:-25,AC:-250",
            "description": "A wisp that has let go of every mortal shape burns with pure power, and has nothing of a "
                           "body to shield it.", "source": "ours (the owner's word, 2026-11-11: the Wisp's racials "
                                                           "and Master Class)"},
     {"effects": "DEXMax:50,INTMax:50,WISMax:25,Pow:500,PoT:20,FR:20,CR:20,LR:20",
      "description": "The transcendent wisp has grown past the mortal mind: quicker of hand and thought, deeper of "
                     "power, and the elements find less of it to burn.", "source": SOURCE}),
    (2144, {"name": "Unbound Will", "effects": "Pow:500,PoT:20", "adds": "", "add_spells": "",
            "description": "Unbound by flesh, the transcendent wisp's will is a well without a bottom.",
            "source": "ours (the owner's word, 2026-11-11: the Wisp's racials and Master Class)"},
     {"name": "Transcend the Material",
      "effects": "DEXMax:50,INTMax:50,DEX:25,INT:25,Pow:500,PoT:25,STA:-50,AGI:-50,STAMax:-25,AGIMax:-25,HP:-500,"
                 "HoT:-25,AC:-250",
      "description": "Wisps forsake material form to strengthen their metaphysical form, seeking power at any cost. "
                     "DEX and INT cap +50, DEX and INT +25, max power +500, PoT +25; STA and AGI -50, STA and AGI "
                     "cap -25, max HP -500, HoT -25, AC -250.", "source": SOURCE}),
    (2145, {"name": "Pale Fire", "effects": "INT:25,WIS:25", "adds": "", "add_spells": "",
            "description": "The pale fire of a transcendent wisp sees through every mind it lights.",
            "source": "ours (the owner's word, 2026-11-11: the Wisp's racials and Master Class)"},
     {"name": "Metaphysical Wound", "effects": "", "adds": "Metaphysical Wound@60", "add_spells": str(WOUND),
      "description": "The wisp wounds what lies beyond the flesh. FR, CR and LR -125 + 10% Int Mod on the target for "
                     "a minute, 10 minute recast.", "source": SOURCE}),
]
FIRST_KIT = {2141, 2142, 2143, 2144, 2145}

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,      -- cmcatalog, spellpattern, spelleffect or spells
    rowkey  INTEGER NOT NULL,   -- its id or spellid
    before  TEXT,               -- the row before (JSON), NULL when written new
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
    return out + [("cmcatalog", r) for r in NEW_ROWS]


def owners(db):
    """(serverid, name) of each owner of rank 2145 without Metaphysical Wound in its book."""
    return db.execute("SELECT c.serverid, coalesce(ch.charName, '?') FROM charcms c LEFT JOIN characters ch ON "
                      "ch.serverid = c.serverid WHERE c.cmid = ? AND NOT EXISTS (SELECT 1 FROM spells s WHERE "
                      "s.serverid = c.serverid AND s.spellid = ?) ORDER BY c.serverid", (WOUND_RANK, WOUND)).fetchall()


def plan(db):
    """(lines, refusals, applied)."""
    lines, refusals = [], []
    for t in ("cmcatalog", "spellpattern", "spelleffect", "racedefs", "gm_audit", "spells", "charcms"):
        if not table(db, t):
            return lines, [f"no {t} table here"], False
    rd = db.execute("SELECT name FROM racedefs WHERE id = ?", (WISP,)).fetchone()
    if rd is None or rd[0] != "Wisp":
        refusals.append(f"racedefs {WISP} is {rd!r}, not the Wisp")
    for tbl, sample in (("cmcatalog", NEW_ROWS[0]), ("spellpattern", SPELLS[0][0]), ("spelleffect", SPELLS[0][1])):
        cols = {r[1] for r in db.execute(f"PRAGMA table_info({tbl})")}
        if missing := set(sample) - cols:
            return lines, [f"{tbl} lacks {sorted(missing)}"], False
    applied = (all(same(row_of(db, t, r[KEYS[t]]), r) for t, r in inserts())
               and all(same(row_of(db, "cmcatalog", id), after) for id, _, after in CHANGES))
    if applied:
        if left := owners(db):
            lines.append(f"{len(left)} owner(s) of {WOUND_RANK} without Metaphysical Wound (bought since, given by "
                         "the purchase: nothing to do): " + ", ".join(n for _, n in left))
        return lines, refusals, True
    for id, before, after in CHANGES:
        now = row_of(db, "cmcatalog", id)
        if now is None or now["races"] != RACES or not same(now, before):
            refusals.append(f"cmcatalog {id} is not as the first kit left it: "
                            f"{now and {k: now[k] for k in before}}")
        else:
            lines.append(f"cmcatalog {id} {now['name']!r} -> " + ", ".join(f"{k} {v!r}" for k, v in after.items()
                                                                          if k not in ("description", "source")))
    ours = FIRST_KIT | {r["id"] for r in NEW_ROWS}
    for id, name, races in db.execute("SELECT id, name, races FROM cmcatalog WHERE (races >> ?) & 1 = 1 "
                                      "AND (races & 2047) != 2047", (WISP,)):
        if id not in ours:
            refusals.append(f"cmcatalog {id} ({name}) has the Wisp's bit (races {races}): look at it first")
    names = {sp["spellname"] for sp, _ in SPELLS}
    for key, nm in db.execute("SELECT spellid, spellname FROM spellpattern"):
        if nm in names and key not in (PONDER, WOUND):
            refusals.append(f"spellpattern {key} is named {nm!r} already")
    for t, r in inserts():
        if (now := row_of(db, t, r[KEYS[t]])) is not None:
            refusals.append(f"{t} {r[KEYS[t]]} is taken: {now.get('name') or now.get('spellname') or now.get('line')!r}")
    for sp, se in SPELLS:
        lines.append(f"spellpattern {sp['spellid']} {sp['spellname']!r}: level {sp['abilitylvl']}, scope {sp['scope']}, "
                     f"cast {sp['casttime']}, power {sp['power']}, recast {sp['recast']}")
        lines.append(f"spelleffect  {se['spellid']}: {se['line']}  {se['effects']}")
    for r in NEW_ROWS:
        lines.append(f"cmcatalog {r['id']} racial {r['mastery']} {r['rank']} {r['name']!r}: cost {r['cost']}, level "
                     f"{r['level']}, parent {r['parent']}, effects {r['effects']}"
                     + (f", adds {r['adds']} ({r['add_spells']})" if r["adds"] else ""))
    own = owners(db)
    lines.append(f"spells: {len(own)} owner(s) of {WOUND_RANK} given Metaphysical Wound: "
                 + (", ".join(f"{n} ({s})" for s, n in own) or "none"))
    return lines, refusals, False


def log(db, tbl, key, before, after, action):
    a = audit(db, action, tbl, key, before, after, DETAIL)
    db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
               (tbl, key, None if before is None else json.dumps(before, sort_keys=True), json.dumps(after, sort_keys=True), a))


def insert(db, tbl, row):
    cols = list(row)
    cur = db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [row[c] for c in cols])
    return cur.lastrowid


def apply(db):
    db.execute(LOG_SCHEMA)
    for t, r in inserts():
        insert(db, t, r)
        log(db, t, r[KEYS[t]], None, row_of(db, t, r[KEYS[t]]), "insert")
    for id, _, after in CHANGES:
        before = row_of(db, "cmcatalog", id)
        db.execute(f"UPDATE cmcatalog SET {', '.join(c + ' = ?' for c in after)} WHERE id = ?", [*after.values(), id])
        log(db, "cmcatalog", id, before, row_of(db, "cmcatalog", id), "update")
    n = 0
    for sid, _ in owners(db):
        order = db.execute("SELECT coalesce(max(addedorder), -1) + 1 FROM spells WHERE serverid = ?", (sid,)).fetchone()[0]
        id = insert(db, "spells", {"serverid": sid, "spellid": WOUND, "addedorder": order, "onHotBar": 0,
                                   "whereonBar": -1, "unk1": 0, "showhide": -1})
        log(db, "spells", id, None, row_of(db, "spells", id), "insert")
        n += 1
    return n


def undo(db):
    """(back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, tbl, key, before, after in db.execute(f"SELECT seq, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        now = row_of(db, tbl, key)
        if now is None or json.dumps(now, sort_keys=True) != after:
            left += 1
            print(f"  left: {tbl} {key}, changed since it was written: {now}")
            continue
        if before is None:
            db.execute(f"DELETE FROM {tbl} WHERE {KEYS[tbl]} = ?", (key,))
            audit(db, "delete", tbl, key, now, None, "undo: " + DETAIL)
        else:
            b = json.loads(before)
            cols = [c for c in b if c != KEYS[tbl]]
            db.execute(f"UPDATE {tbl} SET {', '.join(c + ' = ?' for c in cols)} WHERE {KEYS[tbl]} = ?", [b[c] for c in cols] + [key])
            audit(db, "update", tbl, key, now, b, "undo: " + DETAIL)
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
            print(f"report only: {len(inserts())} rows new, {len(CHANGES)} changed, {len(owners(db))} book(s) "
                  "(--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            n = apply(db)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {len(inserts())} rows new, {len(CHANGES)} changed, {n} book(s); in gm_audit and {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
