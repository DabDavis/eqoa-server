#!/usr/bin/env python3
"""Spells the test lanes' report of 2026-10-29 found broken, and the ones
broken the same way, put right from Allakhazam's records.

The source, named in every entry below: Allakhazam's EQOA spell database
(eqoa.allakhazam.com, its import of the game's records of 2004-04-28), a
page a spell (spell.html?eqoaspell=<id>, "AK <id>"), as the Wayback Machine
archived it in 2006 and 2012, with each spell's class and level, casting
time, recast, power cost, scope ("Single Target", "Self Only", "Group"),
duration and effect. Its records agree with Sandstorm's where both have a
spell (Decaying Flesh, AK 528: cast 6, recast 30, power 526, Sandstorm's
record exactly).

What it changes:

  1. Scopes: ten spells tools/import_eqoadata_spells.py and its mastery
     sibling made self spells (scope 0: EQOA-Data's line has no target word)
     that their pages say are cast at a target. A self spell lands on its
     caster whatever is targeted (CLIENT's scope, Sandstorm's measured way),
     so Ember Strike, Annihilate and Arcane Bonds landed on the wizard and
     the monk who cast them at a creature (B02-B04), and Plaguebringer's Rash
     (L01) on the necromancer. Each is now scope 1 with the importer's range
     and frame of that scope, and its page's casting time, recast and power:
     the importer's were another rank's or a kind's (Annihilate's recast 2
     where its page says 180), which as a target spell would be a nuke every
     two seconds.
  2. Lines and records: Pouncing Tiger's line was its prose ("A furious
     martial attack that injures the target"), read as nothing, so the cast
     was a help refused at a creature ("Target is not friendly", B01); now
     its page's 1224 and 20% of STA, and its page's record. Poison Ward's
     prose ("Adds to target's poison resistance") did nothing (L05): now its
     page's 100 poison and 100 disease resist for 30 minutes.
  3. The wizard's power line (Harvest, Gather Power, Influx and their
     Bountiful ranks, Channeler's Bounty): the importer gave every one a
     recast of 90 and a power cost (Gather Power 144 for 120 power), where
     their pages give each a cost of 1 and the Bountiful rank half the
     recast of the one it replaces: the mastery's whole point, which with
     the importer's numbers was nothing. Bountiful Gather Power (Channeler's
     Bounty 2's first spell, C01) has no record anywhere, Allakhazam's
     included: it is made here by that rule from Gather Power's page (+120,
     cost 1, recast 90: OURS, by the other three Bountiful ranks), and the
     rank gives it. Focus (the warrior's record the importer found for the
     wizard) is left.
  4. Masteries' grants (cmcatalog.add_spells): Heal Other's (C02) "Mend
     Other" and "Bandage Other" are the spells EQOA-Data's monk list and
     Allakhazam call Mend Wounds and Bandage Wounds (AK 3440, 3441: Single
     Target, 45 and 130, the same rank's other four the "Wounds" ones);
     Guardian 3's "Holy Aid" (C03) is the record the importer made as
     Guardian's Aid (EQOA-Data's spell list's name; its line Holy Aid's page
     word for word, AK 4015), renamed as the game's record names it, with its
     page's record and its 24 seconds (its line had none: no ticks).
  5. Gates (spellgates): Melton, Highpass and Freeport Evacuation as group
     gates at the places their lines name, not the caster's bind (B12, B13:
     tools/seed_spell_gates.py now writes them so; its rows are changed here
     where they are still the seed's).

A value is changed only while it is what this expects (a GM's change is
left, and said); a spell is found by its class, name and level in
classspells. Every change is logged in tested_spells_fix_log (the row's key,
the whole row before and after, why); --undo puts back each row still as
this left it (one changed since is left, and said) and drops the log, a
second --apply only adds what is still to do. Both copy the database first
(<db>.bak-<when>-pre-testedfix). Report only by default. The server reads
the records and lines at each cast; restart it for the masteries
(cmcatalog), the gates and the code that reads the summons.

    tools/fix_tested_spells.py [--db eqoa.db] [--apply | --undo]
"""
import argparse
import datetime
import json
import os
import shutil
import sqlite3
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
CLASSES = "WAR RAN PAL SK MNK BRD RGE DRD SHA CL MAG NEC ENC WIZ ALC".split()
RAN, PAL, SK, MNK, DRD, CLR, MAG, NEC, ENC, WIZ = 1, 2, 3, 4, 7, 9, 10, 11, 12, 13

LOG_SCHEMA = """CREATE TABLE IF NOT EXISTS tested_spells_fix_log (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,   -- spellpattern | spelleffect | classspells | cmcatalog | spellgates
    rowkey  TEXT NOT NULL,   -- the row's key, JSON
    before  TEXT,            -- the whole row before, JSON; NULL: inserted
    after   TEXT NOT NULL,   -- the whole row after, JSON
    why     TEXT NOT NULL,
    applied TEXT NOT NULL
)"""
KEYS = {"spellpattern": ("spellid",), "spelleffect": ("spellid",), "classspells": ("class", "spellid"),
        "cmcatalog": ("id",), "spellgates": ("spell",)}

# The importer's self record and the range and frame it gives a record of
# scope 1 (tools/fix_spell_records.py's, the importer's rule).
SELF_RANGE, SELF_FRAME = 16528, -1809115846
TARGET_RANGE, TARGET_FRAME = 16880, 486870879

# Records, by (class, name, level): ((scope, cast, recast, power) now, the
# page's, the page).
RECORDS = {
    # 1. cast at a target, made self spells
    (MNK, "ember strike", 4): ((0, 3, 2, 48), (1, 0, 30, 30), 3419),
    (WIZ, "annihilate", 34): ((0, 3, 2, 408), (1, 2, 180, 510), 402),
    (WIZ, "arcane bonds", 17): ((0, 3, 6, 85), (1, 3, 6, 60), 440),
    (NEC, "plaguebringer's rash", 29): ((0, 2, 5, 156), (1, 2, 5, 126), 3448),
    (MAG, "lava shock", 12): ((0, 2, 3, 42), (1, 2, 3, 49), 478),
    (NEC, "shock of poison", 24): ((0, 2, 3, 143), (1, 2, 3, 95), 514),
    (RAN, "whirling blades", 29): ((0, 3, 2, 116), (1, 0, 90, 218), 798),
    (ENC, "beguile", 48): ((0, 5, 30, 250), (1, 5, 1, 250), 1625),
    (DRD, "charm animal", 30): ((0, 5, 30, 156), (1, 5, 30, 390), 3236),
    (SK, "inciting touch", 44): ((0, 2, 3, 270), (1, 0, 5, 165), 3570),
    # 2. a line of prose
    (MNK, "pouncing tiger", 49): ((1, 2, 3, 184), (1, 0, 90, 368), 851),
    (CLR, "poison ward", 30): ((1, 2, 3, 113), (1, 3, 5, 180), 1752),
    # 3. the wizard's power line
    (WIZ, "harvest", 12): ((0, 0, 90, 72), (0, 2, 120, 1), 397),
    (WIZ, "gather power", 24): ((0, 0, 90, 144), (0, 2, 180, 1), 398),
    (WIZ, "influx", 39): ((0, 0, 90, 234), (0, 2, 240, 1), 399),
    (WIZ, "bountiful harvest", 12): ((0, 0, 90, 72), (0, 2, 60, 1), 3726),
    (WIZ, "bountiful influx", 39): ((0, 0, 90, 234), (0, 2, 120, 1), 3727),
    (WIZ, "bountiful focus", 60): ((0, 0, 90, 360), (0, 2, 120, 1), 3728),
    # 4. Holy Aid, as the importer made it (renamed below)
    (PAL, "guardian's aid", 60): ((1, 3, 1, 690), (1, 5, 1, 780), 4015),
}

# Lines, by (class, name, level): (the line now, the page's in the words the
# server reads, the page).
LINES = {
    (MNK, "pouncing tiger", 49): ("A furious martial attack that injures the target",
                                  "Target, Decrease Hitpoints by 1224 ; Increase 20% based on STA", 851),
    (CLR, "poison ward", 30): ("Adds to target's poison resistance",
                               "Target, Increase Resist Poison by 100, Increase Resist Disease by 100, 30min", 1752),
    (PAL, "guardian's aid", 60): ("Target, Increase HP per tic by 250 ; Increase 25% based on CHA",
                                  "Target, Increase HP per tic by 250 ; Increase 25% based on CHA, 24sec", 4015),
}

# Names, by (class, name, level): (the name now, the game's record's).
NAMES = {(PAL, "guardian's aid", 60): ("Guardian's Aid", "Holy Aid")}

# The record made: Bountiful Gather Power, Gather Power's page (AK 398) with
# the Bountiful ranks' half recast (AK 3726-3728).
MADE = {"class": WIZ, "name": "Bountiful Gather Power", "level": 24, "like": (WIZ, "gather power", 24),
        "line": "Self, Increase Power by 120", "recast": 90}

# Masteries' grants, by (mastery, rank name): (add_spells now, adds now,
# what each add is: a spell by (class, name, level), MADE, or the id kept).
GRANTS = {
    ("Channeler's Bounty", "Channeler's Bounty 2"): (
        "0,700971", None, ["made", 700971]),
    ("Sensei", "Heal Other"): (
        "0,0,700920,700964,701015,701070",
        "Mend Other@60; Bandage Other@60; Compress Wounds@60; Salve Wounds@60; Knit Wounds@60; Heal Wounds@60",
        [(MNK, "mend wounds", 1), (MNK, "bandage wounds", 15), 700920, 700964, 701015, 701070]),
    ("Guardian", "Holy Aid"): ("", None, [(PAL, "guardian's aid", 60)]),
}
# the adds' words for the spells they now give (Heal Other's first two)
ADDS = {("Sensei", "Heal Other"):
        "Mend Wounds@60; Bandage Wounds@60; Compress Wounds@60; Salve Wounds@60; Knit Wounds@60; Heal Wounds@60"}

# Gates: the evacuations as group gates at their rows' places.
GATES = ["melton evacuation", "highpass evacuation", "freeport evacuation"]


def key_text(key):
    return json.dumps(key, sort_keys=True, separators=(",", ":"))


def row_of(db, tbl, key):
    where = " AND ".join(f"{c} = ?" for c in key)
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", list(key.values()))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def spell_of(db, cls, name, lvl):
    """The one spell of a class of that name (any case) and level, else the
    reason there is none."""
    ids = [i for (i,) in db.execute(
        "SELECT p.spellid FROM classspells c JOIN spellpattern p ON p.spellid = c.spellid "
        "WHERE c.class = ? AND lower(trim(p.spellname)) = ? AND p.abilitylvl = ?", (cls, name, lvl))]
    if len(ids) != 1:
        return None, f"{len(ids)} {CLASSES[cls]} spells of that name and level"
    return ids[0], None


def plan(db):
    """The changes [(table, key, row before or None, row after, why)] and the
    rows left as they are, with why [(what, why)]."""
    changes, kept = [], []
    rows = {}  # (tbl, key text) -> [before, after, [why]]

    def change(tbl, key, why):
        k = (tbl, key_text(key))
        if k not in rows:
            r = row_of(db, tbl, key)
            rows[k] = [r, dict(r) if r else None, []]
        rows[k][2].append(why)
        return rows[k][1]

    found = {}
    for (cls, name, lvl), (now, want, page) in RECORDS.items():
        what = f"{CLASSES[cls]} L{lvl} {name}"
        sid, why = spell_of(db, cls, name, lvl)
        if sid is None and (cls, name, lvl) in NAMES:
            sid, _ = spell_of(db, cls, NAMES[(cls, name, lvl)][1].lower(), lvl)
        if sid is None:
            kept.append((what, why))
            continue
        found[(cls, name, lvl)] = sid
        r = row_of(db, "spellpattern", {"spellid": sid})
        have = (r["scope"], r["casttime"], r["recast"], r["power"])
        if have == want:
            continue
        if have != now:
            kept.append((what, f"record {sid} has (scope, cast, recast, power) {have}, not {now}"))
            continue
        a = change("spellpattern", {"spellid": sid}, f"AK {page}")
        a["scope"], a["casttime"], a["recast"], a["power"] = want
        if want[0] != now[0]:
            if (r["range"], r["iconColor"]) != (SELF_RANGE, SELF_FRAME):
                kept.append((what, f"record {sid}'s range and frame are not the importer's self record's"))
                del rows[("spellpattern", key_text({"spellid": sid}))]
                continue
            a["range"], a["iconColor"] = TARGET_RANGE, TARGET_FRAME

    for (cls, name, lvl), (old, new) in NAMES.items():
        sid = found.get((cls, name, lvl))
        if sid is None:
            continue
        r = row_of(db, "spellpattern", {"spellid": sid})
        if r["spellname"] == new:
            continue
        if r["spellname"] != old:
            kept.append((f"{CLASSES[cls]} L{lvl} {name}", f"record {sid} is named {r['spellname']!r}"))
            continue
        change("spellpattern", {"spellid": sid}, "the game's record's name (AK)")["spellname"] = new

    for (cls, name, lvl), (old, new, page) in LINES.items():
        what = f"{CLASSES[cls]} L{lvl} {name}: line"
        sid = found.get((cls, name, lvl))
        if sid is None:
            continue
        e = row_of(db, "spelleffect", {"spellid": sid})
        if e is None or e["line"] == new:
            continue
        if e["line"] != old:
            kept.append((what, f"its line is {e['line']!r}"))
            continue
        a = change("spelleffect", {"spellid": sid}, f"AK {page}")
        a["line"] = new
        a["source"] = f"{e['source']}; line AK {page} (tested19)"
        p = row_of(db, "spellpattern", {"spellid": sid})
        if p["spelldesc"] == old:
            change("spellpattern", {"spellid": sid}, f"its description is its line (AK {page})")["spelldesc"] = new

    # the record made
    made_id, made_why = None, None
    like = found.get(MADE["like"])
    have, _ = spell_of(db, MADE["class"], MADE["name"].lower(), MADE["level"])
    if have is not None:
        made_id = have
    elif like is None:
        made_why = "Gather Power was not found"
    else:
        base = rows.get(("spellpattern", key_text({"spellid": like})), [row_of(db, "spellpattern", {"spellid": like})] * 2)[1]
        made_id = db.execute("SELECT max(spellid) FROM spellpattern").fetchone()[0] + 1
        rec = dict(base)
        rec.update(spellid=made_id, spellname=MADE["name"], spelldesc=MADE["line"], recast=MADE["recast"])
        why = "Gather Power's page (AK 398) with the Bountiful ranks' half recast (AK 3726-3728): ours, by that rule"
        rows[("spellpattern", key_text({"spellid": made_id}))] = [None, rec, [why]]
        e = row_of(db, "spelleffect", {"spellid": like})
        rows[("spelleffect", key_text({"spellid": made_id}))] = [None, {
            "spellid": made_id, "line": MADE["line"], "fxdict": e["fxdict"] if e else 0,
            "source": "Gather Power's line, the Bountiful rank (ours, tested19)"}, [why]]
        rows[("classspells", key_text({"class": MADE["class"], "spellid": made_id}))] = [None, {
            "class": MADE["class"], "spellid": made_id, "source": "eqoa-data cm"}, [why]]
    if made_why:
        kept.append((MADE["name"], made_why))

    for (mastery, rank), (now, adds_now, want) in GRANTS.items():
        what = f"{rank} ({mastery}): add_spells"
        rs = db.execute("SELECT id FROM cmcatalog WHERE mastery = ? AND name = ?", (mastery, rank)).fetchall()
        if len(rs) != 1:
            kept.append((what, f"{len(rs)} ranks of that name"))
            continue
        rid = rs[0][0]
        ids = []
        for w in want:
            if w == "made":
                ids.append(made_id)
            elif isinstance(w, tuple):
                ids.append(found.get(w) or spell_of(db, *w)[0])
            else:
                ids.append(w)
        if None in ids:
            kept.append((what, "a spell it gives was not found"))
            continue
        r = row_of(db, "cmcatalog", {"id": rid})
        target = ",".join(str(i) for i in ids)
        if r["add_spells"] == target:
            continue
        if r["add_spells"] != now or (adds_now is not None and r["adds"] != adds_now):
            kept.append((what, f"rank {rid} gives {r['add_spells']!r} ({r['adds']!r})"))
            continue
        a = change("cmcatalog", {"id": rid}, "the spells it names (AK; EQOA-Data's lists)")
        a["add_spells"] = target
        if (mastery, rank) in ADDS:
            a["adds"] = ADDS[(mastery, rank)]

    if has_table(db, "spellgates"):
        for spell in GATES:
            r = row_of(db, "spellgates", {"spell": spell})
            if r is None or r["kind"] == "group":
                continue
            if r["kind"] != "evac" or r["source"] != "seed":
                kept.append((f"gate {spell}", f"kind {r['kind']!r}, source {r['source']!r}: a GM's"))
                continue
            change("spellgates", {"spell": spell}, "a group gate at the place its line names (the report's B12/B13)")["kind"] = "group"

    for (tbl, k), (before, after, why) in rows.items():
        if before != after:
            changes.append((tbl, json.loads(k), before, after, "; ".join(why)))
    return changes, kept


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-testedfix"
    shutil.copy2(path, dst)
    return dst


def write_row(db, tbl, key, row, insert=False):
    if insert:
        cols = list(row)
        db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [row[c] for c in cols])
        return
    where = " AND ".join(f"{c} = ?" for c in key)
    if row is None:
        db.execute(f"DELETE FROM {tbl} WHERE {where}", list(key.values()))
        return
    cols = [c for c in row if c not in key]
    db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in cols)} WHERE {where}",
               [row[c] for c in cols] + list(key.values()))


def apply(db, changes):
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    order = ["spellpattern", "spelleffect", "classspells", "cmcatalog", "spellgates"]
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_SCHEMA)
        for tbl, key, before, after, why in sorted(changes, key=lambda c: order.index(c[0])):
            db.execute("INSERT INTO tested_spells_fix_log (tbl, rowkey, before, after, why, applied) VALUES (?, ?, ?, ?, ?, ?)",
                       (tbl, key_text(key), None if before is None else json.dumps(before, sort_keys=True),
                        json.dumps(after, sort_keys=True), why, when))
            write_row(db, tbl, key, after, insert=before is None)
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db):
    if not has_table(db, "tested_spells_fix_log"):
        print("nothing logged: nothing to undo")
        return
    logged = db.execute("SELECT seq, tbl, rowkey, before, after FROM tested_spells_fix_log ORDER BY seq DESC").fetchall()
    back = 0
    db.execute("BEGIN IMMEDIATE")
    try:
        for seq, tbl, rowkey, before, after in logged:
            if tbl not in KEYS:
                raise SystemExit(f"tested_spells_fix_log names table {tbl!r}, which this tool never writes")
            key = json.loads(rowkey)
            now = row_of(db, tbl, key)
            if now != json.loads(after):
                print(f"  {tbl} {rowkey}: changed since ({'gone' if now is None else 'not as this left it'}): left as it is")
                continue
            write_row(db, tbl, key, None if before is None else json.loads(before))
            back += 1
        db.execute("DROP TABLE tested_spells_fix_log")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"put back {back} of {len(logged)} rows; restart the server to read them")


def describe(change):
    tbl, key, before, after, why = change
    if before is None:
        return f"{tbl} {key_text(key)}: made {json.dumps(after, sort_keys=True)}\n          ({why})"
    diff = ", ".join(f"{c} {before[c]!r} -> {after[c]!r}" for c in after if after[c] != before[c])
    return f"{tbl} {key_text(key)}: {diff}\n          ({why})"


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the changes (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply changed")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    if a.undo:
        if not a.no_backup and has_table(db, "tested_spells_fix_log"):
            print(f"copy: {backup(a.db)}")
        undo(db)
        return
    changes, kept = plan(db)
    for c in changes:
        print(describe(c))
    for what, why in kept:
        print(f"left as it is: {what} ({why})")
    print(f"{len(changes)} rows to change, {len(kept)} left")
    if not a.apply or not changes:
        if not a.apply:
            print("report only: --apply to write")
        return
    if not a.no_backup:
        print(f"copy: {backup(a.db)}")
    apply(db, changes)
    print(f"changed {len(changes)} rows; restart the server to read the masteries and the gates")


if __name__ == "__main__":
    main()
