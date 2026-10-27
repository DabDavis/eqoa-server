#!/usr/bin/env python3
"""Spells whose records, lines or summoned items disagree with the game's own
records of them, put right (jib's bug reports of 2026-10-27).

The source is Allakhazam's EQOA spell database (eqoa.allakhazam.com, its
import of the game's spells of 2004-04-28) as the Wayback Machine archived
it: a page a spell, "AK <id>" below, with its class and level, cast time,
recast, power cost, scope, duration and effects. Each entry names its page.

What it changes:

  1. Summons. The ranger's five Forage spells say "creates food" (EQOA-Data's
     words), which names no item, so they took their power and gave nothing
     (RNG-1); their pages say "Summon Item: foraged berries" (grub, fruit,
     herb, tuber), and the line now says so. The alchemist's master class
     summons named items the server has under other names or not at all
     (ALC-1): Miraculous Mixture's page summons a "Miraculous Draught" (the
     item Return Home's rows already hold), Wonderous Vaccination's a
     "Wonderous Vaccine" (EQOA-Data's own Plane of Disease guide: "Wonderous
     Vaccine, DR Buff Pot"), Transmutive Mixture's a "Transmutive Potion", as
     its line had it. Neriak Cocktail's page summons a "Grobb Cocktail", as its
     line and description say (ALC-3): the item seed_potions.py made of the
     spell's name is renamed so.
  2. Items made: the five foraged foods, meals as the grocers' are (handlers/
     consume.go reads "a meal ... health regeneration" in a description),
     NO TRADE as EQOA-Data's lines say ("Creates no trade/no rent food"), at
     the spell's level, their health a tick the level (the grocers' rule: ours,
     no record gives one; a comment on Forage Berries' page says "24 healing,
     few ticks like regular food"), a stack of 50, the icons of foods already
     here; the Transmutive Potion and the Wonderous Vaccine, as Miraculous
     Draught's row is (a stack of 3, level 1). What the three potions do when
     used is not in any record: none has an itemeffect row, a GM's to give
     (the panel's Item uses page).
  3. Records (spellpattern): the cast time, recast and power the importers
     made with numbers of their own (another rank's, or a kind's), as the pages
     give them: the Forage spells 3 s, 10 s, 15 to 146; the Eye line 0 s,
     300 s (where they had 2 s and 4 s), Falcon and Eagle Eye 384 and 588;
     Wolf's Instinct 3 s, an hour, 490; True Shot's recast 6 s and 362; the
     four alchemist spells'.
  4. Lines (spelleffect): the ranger's two arrow epics are EQOA-Data's prose
     ("caps power at 1500 and buffs at least 305 wisdom"; "290 WIS. Decrease
     Power to 1750. 500 PoT"), which reads as nothing (RNG-6): now in words
     the server reads, the ceiling the Eye line puts on maximum power
     (entity/powercap.go) at 1500 and 1750; their 45 seconds are ours, the Eye
     line's, which their recasts match.
  5. Masteries (cmcatalog): two ranks describe their ability with numbers
     the ability does not have (RNG-2, RNG-3): Wolf Instinct "80
     str/sta/dex/agi + 700 HP buff", Hunter 2 "575+3 x dex"; the abilities
     are as the game's records have them (AK 3164: STR, AGI and DEX 175, max
     HP and HP 1500, 5 minutes; AK 4018: 576 plus 150% of DEX), and the ranks
     now say so.

A row is changed only while it holds what this expects (a record whose
numbers are no longer the ones named, a line not the one named, is left as it
is and said), and not where a GM changed one of the columns this writes
(gm_audit; a GM's new visual on a spell's row leaves its line this tool's to
change). Every change is logged in archived_spells_fix_log (the whole row
before and after, and why); --undo puts back each row still as this left it
(one changed since is left, and said), deletes the items it made, and drops
the log. Both take a copy of the database first (<db>.bak-<when>-pre-
archivedfix). Report only by default.

The server reads a spell's record and line at each cast and an item at each
summon; a client shows a record and an item's name as its world entry sent
them, so players see them at their next login. Run it again after an import
that writes these rows (tools/import_eqoadata_spells.py, import_eqoadata_
mastery_spells.py, seed_masteries.py, seed_potions.py).

    tools/fix_archived_spells.py [--db eqoa.db] [--apply | --undo]
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
RAN, ALC = 1, 14
TAG = "archived27"
FIRST_ITEM = 50101

LOG_SCHEMA = """CREATE TABLE IF NOT EXISTS archived_spells_fix_log (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,   -- spellpattern | spelleffect | itempattern | cmcatalog
    rowkey  TEXT NOT NULL,   -- the row's key, JSON, as gm_audit keys it
    before  TEXT,            -- the whole row before, JSON; NULL: made here
    after   TEXT NOT NULL,   -- the whole row after, JSON
    why     TEXT NOT NULL,
    applied TEXT NOT NULL
)"""

# 1, 3. Records, by (class, name, level): ((cast, recast, power) now, the
# page's, the page).
RECORDS = {
    (RAN, "forage berries", 5): ((2, 4, 30), (3, 10, 15), 209),
    (RAN, "forage grub", 25): ((2, 4, 150), (3, 10, 68), 1869),
    (RAN, "forage fruit", 35): ((2, 4, 210), (3, 10, 89), 1887),
    (RAN, "forage herb", 45): ((2, 4, 270), (3, 10, 122), 2075),
    (RAN, "forage tuber", 54): ((2, 4, 324), (3, 10, 146), 3084),
    (RAN, "sparrow eye", 20): ((2, 4, 120), (0, 300, 120), 1587),
    (RAN, "raven eye", 30): ((2, 4, 180), (0, 300, 180), 1747),
    (RAN, "hawk eye", 40): ((2, 4, 240), (0, 300, 240), 1748),
    (RAN, "falcon eye", 49): ((2, 4, 294), (0, 300, 384), 1749),
    (RAN, "eagle eye", 49): ((2, 4, 294), (0, 300, 588), 1750),
    (RAN, "wolf's instinct", 51): ((1, 300, 306), (3, 3600, 490), 3164),
    (RAN, "true shot", 60): ((0, 300, 460), (0, 6, 362), 4018),
    (ALC, "neriak cocktail", 40): ((2, 3, 240), (1, 1, 80), 3902),
    (ALC, "miraculous mixture", 60): ((2, 3, 360), (1, 3, 153), 4058),
    (ALC, "transmutive mixture", 60): ((2, 3, 360), (1, 3, 153), 4062),
    (ALC, "wonderous vaccination", 60): ((2, 3, 360), (2, 3, 172), 4262),
}

# 1, 4. Lines, by (class, name, level): (the line now, the line, the page or
# None for ours, whether the record's description says it too).
LINES = {
    (RAN, "forage berries", 5): ("Self, creates food", "Self, Summon Item: foraged berries", 209, False),
    (RAN, "forage grub", 25): ("Creates no trade/no rent food.", "Self, Summon Item: foraged grub", 1869, False),
    (RAN, "forage fruit", 35): ("Creates no trade/no rent food", "Self, Summon Item: foraged fruit", 1887, False),
    (RAN, "forage herb", 45): ("Creates food", "Self, Summon Item: foraged herb", 2075, False),
    (RAN, "forage tuber", 54): ("Creates food", "Self, Summon Item: foraged tuber", 3084, False),
    (ALC, "miraculous mixture", 60): ("SELF Summon Miraculous Potion", "SELF Summon Miraculous Draught", 4058, True),
    (ALC, "wonderous vaccination", 60): ("SELF Summon Wonderous Vaccination", "SELF Summon Wonderous Vaccine", 4262, True),
    (RAN, "gelledyn's perfect vision", 60): (
        "Level 60 Epic Arrow path. Power cost: 420, Cast: 0, Recast: 295. caps power at 1500 and buffs at least 305 wisdom",
        "Self, Increase WIS by 305, Set Maximum Power to 1500, 45sec", None, False),
    (RAN, "gelledyn's feral imitation", 60): (
        "Level 60 Epic Arrow path. Power cost: 0, Cast: 0, Recast: 290. 290 WIS. Decrease Power to 1750. 500 PoT",
        "Self, Increase WIS by 290, Set Maximum Power to 1750, Increase Power per tic by 500, 45sec", None, False),
}

# 2. Items: Neriak Cocktail's (the seeded one: an itemeffect row of source
# potions) renamed; the ones made, with the row a new one copies.
RENAMES = [("Neriak Cocktail", "Grobb Cocktail", 3902)]

ICON_BERRIES = 1511586099  # Red Burn Berries' and hearty bread's
ICON_JERKY = -84908632     # dried beef's
ICON_FRUIT = 1342617881    # Thornless Treant Fruit's
ICON_HERBS = 806013905     # weak herbs'
ICON_TUBER = 8516444       # Ambroise Fruit's and levain's
ICON_FLASK = 209763529     # Alacrity Potion's (seed_potions.py ICON_FLASK)
ICON_TONIC = 824019198     # Major Tonic's (seed_potions.py ICON_TONIC)

MEAL = "{}, a meal that will increase health regeneration."
FOODS = [
    # name, level, icon, words, the page
    ("foraged berries", 5, ICON_BERRIES, "Berries foraged from the wild", 209),
    ("foraged grub", 25, ICON_JERKY, "A grub foraged from the wild", 1869),
    ("foraged fruit", 35, ICON_FRUIT, "Fruit foraged from the wild", 1887),
    ("foraged herb", 45, ICON_HERBS, "An herb foraged from the wild", 2075),
    ("foraged tuber", 54, ICON_TUBER, "A tuber foraged from the wild", 3084),
]
POTIONS = [
    # name, icon, description (ours), the page
    ("Transmutive Potion", ICON_FLASK, "A potion of a transmuter's making.", 4062),
    ("Wonderous Vaccine", ICON_TONIC, "A vaccine against disease.", 4262),
]

# 5. Masteries, by (mastery, rank): (the description now, the description,
# the page of the ability).
MASTERIES = {
    ("Wolf Instinct", 1): ("80 str/sta/dex/agi + 700 HP buff",
                           "Wolf's Instinct: STR, AGI and DEX +175, max HP and HP +1500, for 5 minutes (recast 1 hour)", 3164),
    ("Hunter", 2): ("575+3 x dex (2510 at 645 dex) pierce resist 35 range",
                    "True Shot: 576 damage, plus 150% of DEX", 4018),
}


def spell_name(name):
    return (name or "").replace("`", "'").strip().lower()


def key_text(key):
    """A key as gm_audit writes it (Go's json.Marshal of a map: sorted, no
    spaces)."""
    return json.dumps(key, sort_keys=True, separators=(",", ":"))


def gm_columns(db):
    """The columns a GM has changed, by row: {(table, key text): {column}}."""
    try:
        rows = db.execute("SELECT tbl, rowkey, before, after FROM gm_audit").fetchall()
    except sqlite3.OperationalError:
        return {}
    out = {}
    for tbl, k, before, after in rows:
        try:
            key = key_text(json.loads(k))
            b = json.loads(before) if before else {}
            a = json.loads(after) if after else {}
        except (ValueError, TypeError):
            continue
        cols = {c for c in set(b) | set(a) if b.get(c) != a.get(c)}
        if not b or not a:
            cols = {"*"}  # made or deleted by a GM: every column
        out.setdefault((tbl, key), set()).update(cols)
    return out


def row_of(db, tbl, key):
    where = " AND ".join(f"{c} = ?" for c in key)
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", list(key.values()))
    r = cur.fetchone()
    if r is None:
        return None
    return dict(zip([d[0] for d in cur.description], r))


def plan(db):
    """The changes [(table, key, row before or None, row after, why)] and the
    rows left as they are, with why [(what, why)]."""
    gm = gm_columns(db)
    changes, kept = [], []
    by_name = {}
    for cls, sid, name, lvl in db.execute("""
            SELECT cs.class, cs.spellid, sp.spellname, sp.abilitylvl
            FROM classspells cs JOIN spellpattern sp ON sp.spellid = cs.spellid ORDER BY cs.spellid"""):
        by_name.setdefault((cls, spell_name(name), lvl), set()).add(sid)
    rows = {}  # (table, key text) -> [key, before, after, [why]]

    def one(cls, name, lvl, what):
        ids = sorted(by_name.get((cls, name, lvl), ()))
        if len(ids) != 1:
            kept.append((what, f"{len(ids)} {CLASSES[cls]} spells of that name and level here {ids or ''}".strip()))
            return None
        return ids[0]

    def edit(tbl, key, cols, what):
        """The row to change, or None (gone, or a GM changed one of cols)."""
        touched = gm.get((tbl, key_text(key)), set())
        if touched & (set(cols) | {"*"}):
            kept.append((what, f"a GM changed {', '.join(sorted(touched & (set(cols) | {'*'})))} of {tbl} {key_text(key)}"))
            return None
        k = (tbl, key_text(key))
        if k not in rows:
            r = row_of(db, tbl, key)
            if r is None:
                kept.append((what, f"no {tbl} row {key_text(key)}"))
                return None
            rows[k] = [key, r, dict(r), []]
        return rows[k]

    for (cls, name, lvl), (now, want, page) in sorted(RECORDS.items()):
        what = f"{CLASSES[cls]} L{lvl} {name}: cast, recast, power"
        sid = one(cls, name, lvl, what)
        e = sid and edit("spellpattern", {"spellid": sid}, ("casttime", "recast", "power"), what)
        if not e:
            continue
        have = (e[2]["casttime"], e[2]["recast"], e[2]["power"])
        if have == want:
            continue
        if have != now:
            kept.append((what, f"record {sid} has {have}, not {now}"))
            continue
        e[2]["casttime"], e[2]["recast"], e[2]["power"] = want
        e[3].append(f"cast, recast, power {now} -> {want}, AK {page}")

    for (cls, name, lvl), (now, want, page, desc) in sorted(LINES.items()):
        what = f"{CLASSES[cls]} L{lvl} {name}: line"
        sid = one(cls, name, lvl, what)
        e = sid and edit("spelleffect", {"spellid": sid}, ("line",), what)
        if not e:
            continue
        src = f"AK {page}" if page else "ours, the Eye line's ceiling"
        if e[2]["line"] != want:
            if e[2]["line"] != now:
                kept.append((what, f"its line is not the one expected ({e[2]['line']!r})"))
                continue
            e[2]["line"] = want
            e[2]["source"] = f"{e[2]['source']}; line as {src} ({TAG})"
            e[3].append(f"line {now!r} -> {want!r}, {src}")
        if desc:
            what = f"{CLASSES[cls]} L{lvl} {name}: description"
            p = edit("spellpattern", {"spellid": sid}, ("spelldesc",), what)
            if p and p[2]["spelldesc"] != want:
                if p[2]["spelldesc"] != now:
                    kept.append((what, f"its description is not the one expected ({p[2]['spelldesc']!r})"))
                else:
                    p[2]["spelldesc"] = want
                    p[3].append(f"description {now!r} -> {want!r}, {src}")

    for old, new, page in RENAMES:
        what = f"item {old!r}"
        ids = [r[0] for r in db.execute("""SELECT p.patternid FROM itempattern p JOIN itemeffect e ON e.patternid = p.patternid
                WHERE lower(p.itemname) = lower(?) AND e.source = 'potions'""", (old,))]
        if not ids:
            if not db.execute("SELECT 1 FROM itempattern WHERE lower(itemname) = lower(?)", (new,)).fetchone():
                kept.append((what, "no seeded item of that name here"))
            continue
        if len(ids) > 1:
            kept.append((what, f"{len(ids)} seeded items of that name ({ids})"))
            continue
        e = edit("itempattern", {"patternid": ids[0]}, ("itemname",), what)
        if e:
            e[2]["itemname"] = new
            e[3].append(f"named {new!r}, the item AK {page}'s spell summons")

    for k, (key, before, after, why) in rows.items():
        if why and after != before:
            changes.append((k[0], key, before, after, "; ".join(why)))

    # the items made: each once, by name
    taken = {r[0] for r in db.execute("SELECT patternid FROM itempattern")}
    next_id = FIRST_ITEM

    def new_id():
        nonlocal next_id
        while next_id in taken:
            next_id += 1
        taken.add(next_id)
        return next_id

    meal = db.execute("SELECT patternid FROM itempattern WHERE equipslot = -1 AND itemdesc LIKE '%meal%health regeneration%' "
                      "ORDER BY patternid LIMIT 1").fetchone()
    draught = db.execute("SELECT patternid FROM itempattern WHERE lower(itemname) = 'miraculous draught' ORDER BY patternid LIMIT 1").fetchone()
    for name, lvl, icon, words, page in FOODS:
        if db.execute("SELECT 1 FROM itempattern WHERE lower(itemname) = lower(?)", (name,)).fetchone():
            continue
        if not meal:
            kept.append((f"item {name!r}", "no meal here to copy"))
            continue
        row = row_of(db, "itempattern", {"patternid": meal[0]})
        row.update(patternid=new_id(), itemname=name, itemdesc=MEAL.format(words), levelreq=lvl, HoT=lvl, PoT=0,
                   itemicon=icon, trade=1, maxstack=50, itemcost=0, lore=0, craft=0, weaponproc=0, model=0)
        changes.append(("itempattern", {"patternid": row["patternid"]}, None, row,
                        f"made: Forage's item, AK {page}; a meal of its level (ours)"))
    for name, icon, desc, page in POTIONS:
        if db.execute("SELECT 1 FROM itempattern WHERE lower(itemname) = lower(?)", (name,)).fetchone():
            continue
        if not draught:
            kept.append((f"item {name!r}", "no Miraculous Draught here to copy"))
            continue
        row = row_of(db, "itempattern", {"patternid": draught[0]})
        row.update(patternid=new_id(), itemname=name, itemdesc=desc, itemicon=icon, weaponproc=0)
        changes.append(("itempattern", {"patternid": row["patternid"]}, None, row,
                        f"made: the item AK {page}'s spell summons; no use yet (a GM's, Item uses)"))

    for (mastery, rank), (now, want, page) in sorted(MASTERIES.items()):
        what = f"mastery {mastery} {rank}: description"
        ids = [r[0] for r in db.execute("SELECT id FROM cmcatalog WHERE mastery = ? AND rank = ?", (mastery, rank))]
        if len(ids) != 1:
            kept.append((what, f"{len(ids)} ranks of that mastery and rank here"))
            continue
        key = {"id": ids[0]}
        touched = gm.get(("cmcatalog", key_text(key)), set())
        if touched & {"description", "*"}:
            kept.append((what, f"a GM changed rank {ids[0]}'s description"))
            continue
        before = row_of(db, "cmcatalog", key)
        if before["description"] == want:
            continue
        if before["description"] != now:
            kept.append((what, f"its description is not the one expected ({before['description']!r})"))
            continue
        changes.append(("cmcatalog", key, before, dict(before, description=want),
                        f"description {now!r} -> {want!r}: the ability's own numbers, AK {page}"))
    return changes, kept


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-archivedfix"
    shutil.copy2(path, dst)
    return dst


def write_row(db, tbl, key, row, insert=False):
    """Updates a row to row's values, makes it (insert), or deletes it (row
    None)."""
    where = " AND ".join(f"{c} = ?" for c in key)
    if row is None:
        db.execute(f"DELETE FROM {tbl} WHERE {where}", list(key.values()))
        return
    if insert:
        cols = list(row)
        db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [row[c] for c in cols])
        return
    cols = [c for c in row if c not in key]
    db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in cols)} WHERE {where}",
               [row[c] for c in cols] + list(key.values()))


def apply(db, path, changes):
    print(f"backup: {backup(path)}")
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_SCHEMA)
        for tbl, key, before, after, why in changes:
            db.execute("INSERT INTO archived_spells_fix_log (tbl, rowkey, before, after, why, applied) VALUES (?, ?, ?, ?, ?, ?)",
                       (tbl, key_text(key), None if before is None else json.dumps(before, sort_keys=True),
                        json.dumps(after, sort_keys=True), why, when))
            write_row(db, tbl, key, after, insert=before is None)
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"changed {len(changes)} rows")


def undo(db, path):
    try:
        logged = db.execute("SELECT seq, tbl, rowkey, before, after FROM archived_spells_fix_log ORDER BY seq DESC").fetchall()
    except sqlite3.OperationalError:
        logged = []
    if not logged:
        print("nothing logged: nothing to undo")
        return
    print(f"backup: {backup(path)}")
    back = 0
    db.execute("BEGIN IMMEDIATE")
    try:
        for seq, tbl, rowkey, before, after in logged:
            key, after = json.loads(rowkey), json.loads(after)
            now = row_of(db, tbl, key)
            if now != after:
                print(f"  {tbl} {rowkey}: changed since ({'gone' if now is None else 'not as this left it'}): left as it is")
                continue
            if before is None:
                held = 0
                if tbl == "itempattern":
                    held = db.execute("SELECT count(*) FROM charinventory WHERE patternid = ?", (key["patternid"],)).fetchone()[0]
                if held:
                    print(f"  {tbl} {rowkey}: {held} held by characters: left as it is")
                    continue
                write_row(db, tbl, key, None)
            else:
                write_row(db, tbl, key, json.loads(before))
            back += 1
        db.execute("DROP TABLE archived_spells_fix_log")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"put back {back} of {len(logged)} rows")


def describe(change):
    tbl, key, before, after, why = change
    if before is None:
        return f"{tbl} {key_text(key)} made: {after.get('itemname')!r} ({why})"
    diff = ", ".join(f"{c} {before[c]!r} -> {after[c]!r}" for c in after if after[c] != before[c] and c != "source")
    name = before.get("spellname") or before.get("itemname") or before.get("name") or ""
    return f"{tbl} {key_text(key)} {name}: {diff}\n          ({why})"


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)

    if a.undo:
        undo(db, a.db)
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
    apply(db, a.db, changes)


if __name__ == "__main__":
    main()
