#!/usr/bin/env python3
"""The Wisp's Raiment: a form of Trickster Flame's in which a Wisp keeps its own look (the wisp in its helm) and shows
all it wears (the owner, 2026-11-11, after playing the wisp kit: "The forms work but there isn't a form that shows the
wisp with all the gear as an orb").

  spellpattern 906720 "Wisp's Raiment": self (scope 0, range as Remove Illusion's), level 1, cast 3 s, power 10,
               recast 5 s, the glamours' icon and colour
  spelleffect  906720: "Self, Shows Gear, 60min", the glamours' visual, and the workbook record the server reads
               (kind "raiment", sheet "Alter Appearance": no buff line; an hour, as a glamour's): while it lasts a
               Wisp in its own form is sent in all it wears (go-eqoa-server's entity/wispform.go, EffectRaiment). It
               ends at its hour, clicked off, with Remove Illusion, or with a glamour cast over it; cast over a
               glamour it ends the glamour.
  cmcatalog    2142 Trickster Flame: adds "Wisp's Raiment@1" (add_spells 906720), its description says so
  spells       every character owning Trickster Flame now (bought before this) gets it in its book, as a purchase
               would have given it (a removal of the mastery takes it, as it takes the glamours)

Refused, nothing written, unless 2142 is Trickster Flame as tools/wisp_kit_20261111.py wrote it (or as this tool
leaves it: applied already), 906720 is free (or this tool's) and Remove Illusion is 1793. Report only by default;
--apply in one transaction, a gm_audit line a row, log wisp_raiment_1111_log; --undo exact (a row changed since is
left and said). spellpattern, spelleffect and cmcatalog are read at the start (or the GM panel's reload): a restart.
The books are read at login: a player online gets it in the book at the next login.

    tools/wisp_raiment_20261111.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "wisp_raiment_1111_log"
DETAIL = "The Wisp's Raiment, Trickster Flame's gear form (the owner, 2026-11-11): tools/wisp_raiment_20261111.py"
WHO, VIA = "dab", "tool"
SPELL, NAME = 906720, "Wisp's Raiment"
FLAME = 2142
GLAMOUR, REMOVE = 496, 1793
EFFECTS = json.dumps({"v": 1, "scope": "self", "target_type": "Self", "harmful": False, "duration_s": 3600,
                      "stack": 0, "level": 1, "min_level": 1, "req_equipment": "Any", "sheet_type": "Alter Appearance",
                      "effects": [{"id": "7E00", "type": NAME, "kind": "raiment"}]}, separators=(",", ":"))
DESC = ("The wisp gathers its mortal trappings about its flame: all it wears is seen on it again, the wisp still burning "
        "in the helm.")
FLAME_BEFORE = {
    "adds": "Glamour: Human@1; Glamour: Elf@4; Glamour: Erudite@8; Glamour: Halfling@12; Glamour: Dwarf@16; "
            "Glamour: Gnome@20; Remove Illusion@1",
    "add_spells": "496,503,507,517,524,531,1793",
    "description": "Wisps have long lured travelers with false shapes. The wisp learns the glamours of mortal form, a "
                   "human's, an elf's, an erudite's, a halfling's, a dwarf's and a gnome's, its gear shown on it while "
                   "it wears one, and Remove Illusion to be the flame again.",
}
FLAME_AFTER = {
    "adds": FLAME_BEFORE["adds"] + "; " + NAME + "@1",
    "add_spells": FLAME_BEFORE["add_spells"] + "," + str(SPELL),
    "description": "Wisps have long lured travelers with false shapes. The wisp learns the glamours of mortal form, a "
                   "human's, an elf's, an erudite's, a halfling's, a dwarf's and a gnome's, its gear shown on it while "
                   "it wears one; the Wisp's Raiment, its own flame shown in all its gear; and Remove Illusion to be "
                   "the flame again.",
}
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,      -- spellpattern, spelleffect, cmcatalog or spells
    rowkey  INTEGER NOT NULL,   -- its spellid, its id (cmcatalog, spells)
    before  TEXT,               -- the row before (JSON), NULL when written new
    after   TEXT NOT NULL,      -- the row as written (JSON)
    audit   INTEGER NOT NULL DEFAULT 0
)"""
KEYS = {"spellpattern": "spellid", "spelleffect": "spellid", "cmcatalog": "id", "spells": "id"}


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


def pattern_row(db):
    """The spellpattern row as written: Remove Illusion's range and colour, the glamour's icon."""
    rng, color = db.execute("SELECT range, iconColor FROM spellpattern WHERE spellid = ?", (REMOVE,)).fetchone()
    icon = db.execute("SELECT icon FROM spellpattern WHERE spellid = ?", (GLAMOUR,)).fetchone()[0]
    return {"spellid": SPELL, "abilitylvl": 1, "unk2": 0, "unk3": 0, "range": rng, "casttime": 3, "power": 10,
            "iconColor": color, "icon": icon, "scope": 0, "recast": 5, "eqprequire": -1, "spellname": NAME,
            "spelldesc": DESC}


def effect_row(db):
    """The spelleffect row as written: the glamour's visual."""
    fx = db.execute("SELECT fxdict FROM spelleffect WHERE spellid = ?", (GLAMOUR,)).fetchone()
    return {"spellid": SPELL, "line": "Self, Shows Gear, 60min", "fxdict": fx[0] if fx else 0,
            "source": "ours (the owner, 2026-11-11: a Wisp's form in all its gear); the glamours' visual",
            "effects": EFFECTS}


def owners(db):
    """(serverid, name) of each character owning Trickster Flame without the spell in its book."""
    return db.execute("SELECT c.serverid, coalesce(ch.charName, '?') FROM charcms c LEFT JOIN characters ch ON "
                      "ch.serverid = c.serverid WHERE c.cmid = ? AND NOT EXISTS (SELECT 1 FROM spells s WHERE "
                      "s.serverid = c.serverid AND s.spellid = ?) ORDER BY c.serverid", (FLAME, SPELL)).fetchall()


def same(now, want):
    return now is not None and all(now.get(k) == v for k, v in want.items())


def plan(db):
    """(lines, refusals, applied)."""
    lines, refusals = [], []
    for t in ("spellpattern", "spelleffect", "cmcatalog", "spells", "charcms"):
        if not table(db, t):
            return lines, [f"no {t} table here"], False
    rm = db.execute("SELECT spellname FROM spellpattern WHERE spellid = ?", (REMOVE,)).fetchone()
    gl = db.execute("SELECT spellname FROM spellpattern WHERE spellid = ?", (GLAMOUR,)).fetchone()
    if rm is None or rm[0] != "Remove Illusion" or gl is None or gl[0] != "Glamour: Human":
        return lines, [f"spellpattern {REMOVE} is {rm!r} and {GLAMOUR} {gl!r}, not Remove Illusion and Glamour: Human"], False
    flame = row_of(db, "cmcatalog", FLAME)
    pat, eff = pattern_row(db), effect_row(db)
    applied = (same(flame, FLAME_AFTER) and same(row_of(db, "spellpattern", SPELL), pat)
               and same(row_of(db, "spelleffect", SPELL), eff))
    if applied:
        if left := owners(db):
            lines.append(f"{len(left)} owner(s) of Trickster Flame without the spell (bought since, given by the "
                         f"purchase: nothing to do): " + ", ".join(n for _, n in left))
        return lines, refusals, True
    if flame is None or flame["name"] != "Trickster Flame" or not same(flame, FLAME_BEFORE):
        refusals.append(f"cmcatalog {FLAME} is not Trickster Flame as tools/wisp_kit_20261111.py wrote it: "
                        f"{flame and {k: flame[k] for k in ('name', 'adds', 'add_spells')}}")
    for tbl in ("spellpattern", "spelleffect"):
        if (now := row_of(db, tbl, SPELL)) is not None:
            refusals.append(f"{tbl} {SPELL} is taken: {now.get('spellname', now.get('line'))!r}")
    if db.execute("SELECT spellid FROM spellpattern WHERE spellname = ? AND spellid != ?", (NAME, SPELL)).fetchone():
        refusals.append(f"another spell is named {NAME!r}")
    lines.append(f"spellpattern {SPELL} {NAME!r}: self, level 1, cast 3 s, power 10, recast 5 s, icon {pat['icon']}")
    lines.append(f"spelleffect {SPELL}: {eff['line']!r}, visual {eff['fxdict']:#x}, {eff['effects']}")
    lines.append(f"cmcatalog {FLAME} Trickster Flame: adds += {NAME}@1, add_spells += {SPELL}")
    own = owners(db)
    lines.append(f"spells: {len(own)} owner(s) of Trickster Flame given it: " + (", ".join(f"{n} ({s})" for s, n in own) or "none"))
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
    for tbl, row in (("spellpattern", pattern_row(db)), ("spelleffect", effect_row(db))):
        insert(db, tbl, row)
        log(db, tbl, SPELL, None, row_of(db, tbl, SPELL), "insert")
    before = row_of(db, "cmcatalog", FLAME)
    db.execute("UPDATE cmcatalog SET adds = ?, add_spells = ?, description = ? WHERE id = ?",
               (FLAME_AFTER["adds"], FLAME_AFTER["add_spells"], FLAME_AFTER["description"], FLAME))
    log(db, "cmcatalog", FLAME, before, row_of(db, "cmcatalog", FLAME), "update")
    n = 0
    for sid, _ in owners(db):
        order = db.execute("SELECT coalesce(max(addedorder), -1) + 1 FROM spells WHERE serverid = ?", (sid,)).fetchone()[0]
        id = insert(db, "spells", {"serverid": sid, "spellid": SPELL, "addedorder": order, "onHotBar": 0,
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
            print("applied already (the spell and Trickster Flame are this tool's): nothing done")
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
            print("report only (--apply writes it)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            n = apply(db)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: the spell, Trickster Flame, {n} book(s); in gm_audit and {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
