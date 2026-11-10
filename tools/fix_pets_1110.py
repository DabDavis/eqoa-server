#!/usr/bin/env python3
"""Three pet petitions of 2026-11-10 whose fix is rows (the rest of each is the
server's code, the pets-1110 lane's):

  1478 (the magician Lifeispain): "Efreeti Lord doesn't cast a buff on the
  caster. He casts one only himself, not sure if that is intended or not."
    The workbook has every magician pet's buff on its owner (Efreeti's Fire,
    902164: sheet scope Owner, a "Pet Buff"); the pet import put the
    magician's on the pet itself by the owner's word of 2026-11-01 ("the pets
    buff themselves not the player"), and the pets' balance was fitted with it
    there (tools/fit_pet_selfbuffs.py). So the Efreeti Lord keeps its Efreeti's
    Fire on itself, and casts it on its owner too: a second ability, the same
    spell with target owner, 7 s after its summoning and whenever its owner
    lacks it (30 minutes). No code: a pet's buff on its owner is what the
    Ghilan Noble's blessing is.

  1495 (the magician Waldorf): "Ghilan Noble should have three heals, the large
  one, small one, and an AOE heal for 150 that heals players and pets in range.
  First two seem to work fine but is missing the third".
    A new spell, Ghilan's Healing Tide (905495: 150, instant, Tide of Glory's
    icon, range and visual), and the Ghilan Noble's seventh ability: a heal with
    target "group" (entity/petgroupheal.go, this lane's code), 150 at once on
    everyone of its side within the spell's 30 of it who is hurt (itself, its
    owner, its owner's group's players, their pets and mercenaries), every 6 s
    while any of them is below full (OURS: the petition gives the figure, not
    how often; 6 s is the game's regeneration tick and Healbot's own pace).

  1493 (Waldorf): "Similar to ticket 1305, the shamans in permafrost aren't
  attached to their pets so they don't attack when the shaman does" (his target
  an ice goblin shaman).
    As petition 1305 tied Guk's Mummified Guardians to their necromancers: two
    npcsocial rows, the helper social and its master of the same faction of the
    GM's own. Each of Permafrost's seven ice goblin shamans has a Vigilant
    Spirit 1.6 to 2.4 from its spawn: "Vigilant Spirit" comes to the help of
    anyone of "Shamans of the Vigilant Spirits" hit within 20 of it and in its
    sight, and "ice goblin shaman" is of it (and helps no one). The other
    Vigilant Spirits of the world (the Tae Ew hierophants', the Atesh-Ah
    spiritists' and others': 137 rows in all) help no one until their masters
    are given the faction too (a row each, as the shaman's here).

Each row is changed only while it is what this expects (the Efreeti Lord's and
the Ghilan Noble's abilities as tools/fix_mage_pets_1450.py left them; no
npcsocial row of either name; the spell's id and name free): anything else is
refused and nothing written. Every row written is logged in pets_1110_log (the
whole row before and after, the spells' id counter) with its gm_audit line;
--undo puts back each row still as this left it and drops the log. Both copy
the database first (--no-backup to skip). Report only by default. --only takes
the petitions to do (default all three).

WHEN IT TAKES EFFECT: petstats, spells and npcsocial are read at a start (or a
GM panel edit of the table), and a pet is made at its summoning. 1495's row
needs this lane's binary (an older one refuses target "group" and reads the
Ghilan Noble's whole row as unreadable): run it as the before.sh of the keep
restart that brings the pets-1110 binary, on each shard's database. 1478's and
1493's rows are read by any binary.

    tools/fix_pets_1110.py --db eqoa.db [--only 1478,1493,1495] [--apply | --undo] [--no-backup]
"""
import argparse
import datetime
import json
import os
import shutil
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import fix_mage_pets_1450 as base  # noqa: E402  the rows this goes from, and its helpers
import import_eqoaemu_pets as pets  # noqa: E402  the pet import's gm_audit lines, one place

LOG = "pets_1110_log"
DETAIL = "petitions 1478, 1493, 1495 (pets-1110): tools/fix_pets_1110.py"
KEYS = {"spellpattern": ("spellid",), "spelleffect": ("spellid",), "petstats": ("spellid",), "npcsocial": ("npc_name",),
        "sqlite_sequence": ("name",)}
ALL = (1478, 1493, 1495)

EFREETI, GHILAN = base.EFREETI, base.GHILAN
FIRE = 902164  # Efreeti's Fire
TIDE = 905495  # Ghilan's Healing Tide (petition 1495's)
TIDE_NAME, TIDE_HEAL, TIDE_EVERY = "Ghilan's Healing Tide", 150, 6
TIDE_LOOK = (701038, "Tide of Glory")

# 1478: the Efreeti Lord's buff on its owner too, after its own
FIRE_ON_OWNER = base.ab("buff", 1800, 7, FIRE, target="owner")
EFREETI_OLD = base.NEW[EFREETI]
EFREETI_NEW = EFREETI_OLD + [FIRE_ON_OWNER]
# 1495: the Ghilan Noble's third heal
TIDE_ABILITY = base.ab("heal", TIDE_EVERY, 2, TIDE, target="group", below=100)
GHILAN_OLD = base.NEW[GHILAN]
GHILAN_NEW = GHILAN_OLD + [TIDE_ABILITY]
TIDE_EFFECTS = {"v": 1, "scope": "target", "target_type": "Owner/Self", "harmful": False, "stack": 0, "level": 60,
                "min_level": 1, "req_equipment": "Any", "effects": [{"type": "Dmg/Heal", "kind": "hp", "amount": TIDE_HEAL}]}
TIDE_LINE = f"Target, Increase Hitpoints by {TIDE_HEAL}"
TIDE_DESC = (f"The Ghilan Noble's healing tide: {TIDE_HEAL} at once on itself, its owner, its owner's group, their pets "
             "and mercenaries within its reach.")

# 1493: Permafrost's shamans and their spirits, as petition 1305's rows are
FACTION = "Shamans of the Vigilant Spirits"
SOCIAL = [
    {"npc_name": "ice goblin shaman", "social": 0, "faction": FACTION, "radius": 0.0,
     "note": "petition 1493: of the shamans whose Vigilant Spirits come to their help", "source": "petition 1493"},
    {"npc_name": "Vigilant Spirit", "social": 1, "faction": FACTION, "radius": 20.0,
     "note": "petition 1493: Permafrost's shamans' spirits fight beside them", "source": "petition 1493"},
]

NOTES = {
    EFREETI: " Petition 1478 (Lifeispain, 2026-11-10): Efreeti's Fire on its owner too, its own kept "
             "(tools/fix_pets_1110.py).",
    GHILAN: f" Petition 1495 (Waldorf, 2026-11-10): {TIDE_NAME}, {TIDE_HEAL} on everyone of its side within 30 every "
            f"{TIDE_EVERY} s while any is hurt (tools/fix_pets_1110.py).",
}

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,   -- spellpattern, spelleffect, petstats, npcsocial or sqlite_sequence
    rowkey TEXT NOT NULL,   -- the row's key, JSON, as gm_audit keys it
    before TEXT,            -- the whole row before, JSON; NULL: made here
    after  TEXT NOT NULL,   -- the whole row as written, JSON
    audit  INTEGER NOT NULL DEFAULT 0,  -- its gm_audit line (0: the id counter's, none)
    note   TEXT NOT NULL DEFAULT ''
)"""

refuse, has_table, row_of, key_text, dumps, norm, kit = (base.refuse, base.has_table, base.row_of, base.key_text,
                                                         base.dumps, base.norm, base.kit)


def pet_row(db, sid, old, new, what, changes, done):
    """A workbook pet's abilities from old to new: planned, done already, or refused."""
    spell, sheet, name = base.SUMMONS[sid]
    if not has_table(db, "petstats"):
        refuse("no table petstats")
    row = row_of(db, "petstats", {"spellid": sid})
    if row is None or row["name"] != name:
        refuse(f"petstats {sid} is {row and row['name']!r}, not {name!r}")
    now = kit(row["abilities"])
    if now == norm(new):
        done.append(f"petstats {sid} {name}: {what} already")
        return
    if now != norm(old):
        refuse(f"petstats {sid} ({name})'s abilities are {row['abilities']}, not what tools/fix_mage_pets_1450.py left "
               f"(changed since: a GM's?)")
    after = dict(row, abilities=dumps(new))
    after["note"] = row["note"] + NOTES[sid]
    changes.append(("petstats", {"spellid": sid}, row, after, f"{name}: {what}"))


def spell_named(db, sid, name):
    r = db.execute("SELECT spellname FROM spellpattern WHERE spellid = ?", (sid,)).fetchone()
    if r is None or r[0] != name:
        refuse(f"spell {sid} is {r and r[0]!r}, not {name!r}")


def plan(db, only=ALL):
    """The rows to make and change: [(table, key, the row before or None, the row after, what)], and what is done
    already. Refuses (SystemExit) when anything is not what this expects."""
    changes, done = [], []
    if 1478 in only:
        spell_named(db, FIRE, base.KEPT[FIRE])
        pet_row(db, EFREETI, EFREETI_OLD, EFREETI_NEW, "Efreeti's Fire on its owner too", changes, done)
    if 1495 in only:
        for t in ("spellpattern", "spelleffect"):
            if not has_table(db, t):
                refuse(f"no table {t}")
        look, look_name = TIDE_LOOK
        lk = db.execute("SELECT p.spellname, p.range, p.iconColor, p.icon, e.fxdict FROM spellpattern p "
                        "JOIN spelleffect e ON e.spellid = p.spellid WHERE p.spellid = ?", (look,)).fetchone()
        if lk is None or lk[0] != look_name:
            refuse(f"spell {look} is {lk and lk[0]!r}, not {look_name!r} (whose look {TIDE_NAME} takes)")
        pattern = {"spellid": TIDE, "abilitylvl": 60, "unk2": 0, "unk3": 0, "range": lk[1], "casttime": 0, "power": 0,
                   "iconColor": lk[2], "icon": lk[3], "scope": 1, "recast": TIDE_EVERY, "eqprequire": -1,
                   "spellname": TIDE_NAME, "spelldesc": TIDE_DESC}
        effect = {"spellid": TIDE, "line": TIDE_LINE, "fxdict": lk[4], "effects": dumps(TIDE_EFFECTS),
                  "source": f"petition 1495 (Waldorf); {look_name}'s look (tools/fix_pets_1110.py)"}
        p0, e0 = row_of(db, "spellpattern", {"spellid": TIDE}), row_of(db, "spelleffect", {"spellid": TIDE})
        if p0 is not None or e0 is not None:
            if p0 == pattern and e0 is not None and all(e0.get(c) == v for c, v in effect.items()):
                done.append(f"spell {TIDE} {TIDE_NAME}: made already")
            else:
                refuse(f"spell id {TIDE} is taken ({p0 and p0['spellname']!r}): pick another id")
        else:
            if has_table(db, "spellarea") and db.execute("SELECT 1 FROM spellarea WHERE spellid = ?", (TIDE,)).fetchone():
                refuse(f"spellarea has a row for spell {TIDE}")
            other = db.execute("SELECT spellid FROM spellpattern WHERE lower(trim(spellname)) = ?",
                               (TIDE_NAME.lower(),)).fetchone()
            if other:
                refuse(f"a spell named {TIDE_NAME} is there already ({other[0]})")
            e_cols = [r[1] for r in db.execute("PRAGMA table_info(spelleffect)")]
            if "effects" not in e_cols:
                refuse("spelleffect has no effects column")
            changes.append(("spellpattern", {"spellid": TIDE}, None, pattern, f"{TIDE_NAME}: made"))
            changes.append(("spelleffect", {"spellid": TIDE}, None, {c: effect[c] for c in e_cols if c in effect},
                            f"{TIDE_NAME}: {TIDE_LINE}"))
        pet_row(db, GHILAN, GHILAN_OLD, GHILAN_NEW, f"{TIDE_NAME}, its group heal", changes, done)
    if 1493 in only:
        if not has_table(db, "npcsocial"):
            refuse("no table npcsocial")
        for want in SOCIAL:
            name = want["npc_name"]
            rows = db.execute("SELECT npc_name FROM npcsocial WHERE lower(trim(npc_name)) = ?", (name.lower(),)).fetchall()
            now = row_of(db, "npcsocial", {"npc_name": name})
            if now == want:
                done.append(f"npcsocial {name!r}: its row already")
            elif rows:
                refuse(f"npcsocial has a row for {name!r} already ({[r[0] for r in rows]}): a GM's, left")
            else:
                if has_table(db, "npcs") and not db.execute("SELECT 1 FROM npcs WHERE lower(trim(npc_name)) = ? LIMIT 1",
                                                            (name.lower(),)).fetchone():
                    refuse(f"no NPC is named {name!r} in this database")
                changes.append(("npcsocial", {"npc_name": name}, None, want, want["note"]))
    return changes, done


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-pets1110"
    shutil.copy2(path, dst)
    return dst


def apply(db, changes):
    """The plan written in one transaction, each row logged with its gm_audit line; returns how many rows."""
    order = ["spellpattern", "spelleffect", "petstats", "npcsocial"]
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_SCHEMA)
        db.execute(pets.AUDIT_SCHEMA)
        before = base.seq(db)
        for tbl, key, b, a, why in sorted(changes, key=lambda c: order.index(c[0])):
            if row_of(db, tbl, key) != b:
                raise RuntimeError(f"{tbl} {key_text(key)} changed under us")
            base.write_row(db, tbl, key, a, insert=b is None)
            written = row_of(db, tbl, key)
            line = pets.audit(db, "insert" if b is None else "update", tbl, key, b, written, DETAIL)
            db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit, note) VALUES (?, ?, ?, ?, ?, ?)",
                       (tbl, key_text(key), None if b is None else json.dumps(b, sort_keys=True),
                        json.dumps(written, sort_keys=True), line, why))
        after = base.seq(db)
        if after != before:
            # the spells' id counter (spellpattern is AUTOINCREMENT): put back by --undo, as the rows are
            was = None if before is None else {"name": "spellpattern", "seq": before}
            db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, note) VALUES (?, ?, ?, ?, ?)",
                       ("sqlite_sequence", key_text({"name": "spellpattern"}),
                        None if was is None else json.dumps(was, sort_keys=True),
                        json.dumps({"name": "spellpattern", "seq": after}, sort_keys=True), "the spells' id counter"))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return len(changes)


def undo(db):
    """Each logged row still as this left it put back, the last first; returns (back, left)."""
    if not has_table(db, LOG):
        return 0, 0
    back = left = 0
    db.execute("BEGIN IMMEDIATE")
    try:
        for s, tbl, rk, before, after in db.execute(
                f"SELECT seq, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
            if tbl not in KEYS:
                raise SystemExit(f"{LOG} names table {tbl!r}, which this tool never writes")
            key = json.loads(rk)
            now = row_of(db, tbl, key)
            if now != json.loads(after):
                print(f"  left: {tbl} {rk}, {'gone' if now is None else 'changed since it was written'}")
                left += 1
                continue
            b = None if before is None else json.loads(before)
            base.write_row(db, tbl, key, b)  # a row made here deleted, a row changed here put back
            if tbl != "sqlite_sequence":
                pets.audit(db, "delete" if b is None else "update", tbl, key, now, row_of(db, tbl, key), "undo: " + DETAIL)
            db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (s,))
            back += 1
        if not db.execute(f"SELECT 1 FROM {LOG} LIMIT 1").fetchone():
            db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return back, left


def describe(change):
    tbl, key, before, after, why = change
    if before is None:
        return f"{tbl} {key_text(key)}: made {dumps(after)[:300]}"
    diff = "; ".join(f"{c} ...{str(before[c])[-90:]!r} -> ...{str(after[c])[-220:]!r}" for c in after
                     if after[c] != before[c] and c != "note")
    return f"{tbl} {key_text(key)} ({why.split(':')[0]}): {diff}"


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--only", default=",".join(map(str, ALL)), help="the petitions to do, by number (default all)")
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the changes (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply wrote")
    a = ap.parse_args()
    try:
        only = tuple(int(x) for x in a.only.split(",") if x.strip())
    except ValueError:
        sys.exit(f"--only {a.only}: petition numbers, by commas")
    if not only or any(n not in ALL for n in only):
        sys.exit(f"--only {a.only}: of {', '.join(map(str, ALL))}")
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    try:
        if a.undo:
            if not has_table(db, LOG):
                print("nothing logged: nothing to undo")
                return
            if not a.no_backup:
                print(f"copy: {backup(a.db)}")
            back, left = undo(db)
            print(f"put back {back} rows, {left} left (changed since); restart the server to read them")
            return
        changes, done = plan(db, only)
        for c in changes:
            print(describe(c))
        for d in done:
            print(f"done already: {d}")
        print(f"{len(changes)} rows to write, {len(done)} done already")
        if not a.apply:
            print("report only: --apply to write")
            return
        if not changes:
            return
        if not a.no_backup:
            print(f"copy: {backup(a.db)}")
        n = apply(db, changes)
        print(f"written {n} rows, logged in {LOG}; restart the server (keep restart) to read them")
    finally:
        db.close()


if __name__ == "__main__":
    main()
