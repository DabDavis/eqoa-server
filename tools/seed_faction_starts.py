#!/usr/bin/env python3
"""The measured starting standings: factionstart's rows, and the factions
table's own standings and names, as Sandstorm gives a new character
(entity/faction.go; 2026-10-29, the user's new dark elf killed by Neriak's
ghoul servant, which "should be revered").

The measurement is a TSV, a line a combo and a faction:

    race<TAB>class<TAB>humtype<TAB>faction<TAB>standing

race, class and humtype as defaultcharacter has them (humtype 0 for every
race but the humans, 1 the east's, 2 the west's), off a new character of each
of defaultcharacter's combos made on Sandstorm and its faction window read
(the coordinator's survey, harvest-runs/sandstorm/survey/starts.tsv). A first
line naming the columns, blank lines and lines starting with # are passed
over; a combo is read as the server reads a character's (a froglok and a
human of no city as the east's, another race's city 0).

The factions table's standings were one eqoa.live character's (a gnome's,
moved since it was made: a new gnome on Sandstorm differs in 174 of 370), so
they are no one's start, and by default each faction's own standing is made
the start most measured combos have (--keep-defaults leaves them, and the
rows then say every difference from them). On top of that, the fewest rows:

- a faction every combo starts with at one standing gets no row (with
  --keep-defaults, a row a race where the table's own standing differs; a
  GM's own standing is left, said);
- otherwise, for each race, the standing most of its combos start with is one
  row (class -1, humtype -1), unless it is the faction's own; for each human
  city whose start differs from its race's a row (humtype 1 or 2); and a row a
  class whose start differs from its race's (or its city's);
- before anything is written the rows are checked: every measured combo
  comes out (the server's rule: its most specific row, else the faction's
  own) at its measured standing, or the tool stops.

A faction Sandstorm lists and the table lacks is added, at the start most
combos have, so the faction window lists what Sandstorm's does; one whose name
starts with '*' ("*Delete") is junk and left out. A faction of the table the
measurement never lists is said, not deleted (a quest or a hit may name it).

Rows are written with source "seed" and never over a GM's row (another
source) for the same race, class, city and faction, which is said, with every
combo such a row keeps from its measured start. A factions standing a GM
edited in the panel (gm_audit) is left. Everything written is logged in table
factionstart_seed; --undo deletes the rows and the factions it added and puts
the standings back, but not what was changed since (said), and drops the
log. A second --apply is refused while the log is there, and so is an
--apply with a defaultcharacter combo unmeasured (--partial writes anyway: a
race not all measured gets a row a measured combo, so its other classes and
cities keep what they had).
Both take a copy of the database first (<db>.bak-<when>-pre-factionstart).
Report only by default.

The tables are the server's (database/factions.go); --apply makes factionstart
if the server has not yet. The running server reads them at its start, and
again after any GM panel edit of Faction starts or Factions.

    tools/seed_faction_starts.py --db eqoa.db --measured starts.tsv
        [--apply | --undo] [--keep-defaults] [--partial] [--no-backup] [--quiet]
"""
import argparse
import collections
import datetime
import json
import os
import sqlite3
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
SOURCE = "seed"
LOG = "factionstart_seed"
RACE_FROGLOK = 10
RACES = ["Human", "Elf", "Dark Elf", "Gnome", "Dwarf", "Troll", "Barbarian", "Halfling", "Erudite", "Ogre", "Froglok"]
CLASSES = ["Warrior", "Ranger", "Paladin", "Shadow Knight", "Monk", "Bard", "Rogue", "Druid", "Shaman", "Cleric",
           "Magician", "Necromancer", "Enchanter", "Wizard", "Alchemist"]

# the table as database/factions.go makes it (factionStartSchema)
SCHEMA = """CREATE TABLE IF NOT EXISTS factionstart (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    race     INTEGER NOT NULL DEFAULT -1,
    class    INTEGER NOT NULL DEFAULT -1,
    humtype  INTEGER NOT NULL DEFAULT -1,
    faction  TEXT NOT NULL,
    standing INTEGER NOT NULL,
    note     TEXT NOT NULL DEFAULT '',
    source   TEXT NOT NULL DEFAULT 'gm',
    UNIQUE (race, class, humtype, faction)
)"""

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    what    TEXT NOT NULL,     -- 'row' a factionstart row written, 'default' a factions standing changed,
                               -- 'faction' a factions row added
    rowid   INTEGER,           -- the factionstart row ('row')
    faction TEXT NOT NULL,
    old     INTEGER,           -- the factions standing before ('default')
    new     INTEGER NOT NULL   -- what was written
)"""


def name_of(names, i):
    return names[i] if 0 <= i < len(names) else f"#{i}"


def combo_name(combo):
    race, cls, hum = combo
    city = {1: " (east)", 2: " (west)"}.get(hum, "") if race == 0 else ""
    return f"{name_of(RACES, race)}{city} {name_of(CLASSES, cls)}"


# The server's rule (entity/faction.go): a character's combo, a row's
# specificity, the most specific row that holds it, else the faction's own.

def combo_of(race, cls, hum):
    """The combo a character's starts are read for: a froglok and a human of
    no west city as the east's, any other race's city 0 (comboOf)."""
    if race == RACE_FROGLOK or (race == 0 and hum != 2):
        return (0, cls, 1)
    if race != 0:
        hum = 0
    return (race, cls, hum)


def specificity(row):
    """(the fields named, race named, class named, city named): the order
    entity/faction.go's specificity sorts by."""
    named = [v >= 0 for v in row[:3]]
    return (sum(named), named[0], named[1], named[2])


def holds(row, combo):
    return all(v < 0 or v == c for v, c in zip(row[:3], combo))


def resolve(rows, base, combo):
    """A combo's start in one faction: its most specific row (race, class,
    humtype, standing), else base, the faction's own."""
    best = None
    for row in rows:
        if holds(row, combo) and (best is None or specificity(row) > specificity(best)):
            best = row
    return best[3] if best else base


def mode(values, prefer=None):
    """The value most often in values; of equals, prefer if it is one, else
    the highest."""
    count = collections.Counter(values)
    top = max(count.values())
    tied = [v for v, n in count.items() if n == top]
    return prefer if prefer in tied else max(tied)


def compress(values, base, combos=None):
    """The rows (race, class, humtype, standing) one faction's measured starts
    need on top of base, the faction's own (the file's doc). A race whose
    combos (defaultcharacter's, when given) are not all measured gets a row a
    measured combo that differs, so nothing is said for the classes and cities
    not measured yet (--partial)."""
    rows = []
    by_race = collections.defaultdict(dict)
    for combo, v in values.items():
        by_race[combo[0]][combo] = v
    for race in sorted(by_race):
        items = by_race[race]
        if combos is not None and any(c not in items for c in combos if c[0] == race):
            rows.extend((r, c, h, v) for (r, c, h), v in sorted(items.items()) if v != base)
            continue
        r_val = mode(list(items.values()), base)
        if r_val != base:
            rows.append((race, -1, -1, r_val))
        hums = sorted({h for _, _, h in items})
        if len(hums) > 1:
            for h in hums:
                h_items = {c: v for c, v in items.items() if c[2] == h}
                h_val = mode(list(h_items.values()), r_val)
                if h_val != r_val:
                    rows.append((race, -1, h, h_val))
                for (_, cls, _), v in sorted(h_items.items()):
                    if v != h_val:
                        rows.append((race, cls, h, v))
        else:
            for (_, cls, _), v in sorted(items.items()):
                if v != r_val:
                    rows.append((race, cls, -1, v))
    return rows


def read_measured(path, factions):
    """The TSV: {faction: {combo: standing}}, and what was said about it
    (factions the table lacks, junk left out, names taken without their
    ends' spaces, combos read otherwise, standings out of range, lines that
    are no row). Two different standings for one combo and faction stop the
    tool."""
    stripped = collections.defaultdict(list)
    for f in factions:
        stripped[f.strip()].append(f)
    measured = collections.defaultdict(dict)
    said = collections.defaultdict(list)
    clash = []
    with open(path, encoding="utf-8") as fh:
        for n, line in enumerate(fh, 1):
            line = line.rstrip("\r\n")
            if not line.strip() or line.lstrip().startswith("#"):
                continue
            parts = line.split("\t")
            if len(parts) != 5:
                said["not a row"].append(f"line {n}: {line[:60]!r}")
                continue
            try:
                race, cls, hum, standing = int(parts[0]), int(parts[1]), int(parts[2]), int(parts[4])
            except ValueError:
                if n == 1:
                    continue  # the columns' names
                said["not a row"].append(f"line {n}: {line[:60]!r}")
                continue
            name = parts[3]
            if name.startswith("*") or not name.strip():
                said["junk, left out"].append(repr(name))
                continue
            if name not in factions:
                cands = stripped.get(name.strip(), [])
                if len(cands) == 1:
                    said["taken without the spaces at its ends"].append(f"{name!r} as {cands[0]!r}")
                    name = cands[0]
                else:
                    said["not in the factions table: added"].append(repr(name))
            if not -2000 <= standing <= 2000:
                said["out of -2000..2000"].append(f"line {n}: {name} {standing}")
            combo = combo_of(race, cls, hum)
            if combo != (race, cls, hum):
                said["read as the server reads them"].append(
                    f"{combo_name((race, cls, hum))} (humtype {hum}) as {combo_name(combo)}")
            if combo in measured[name] and measured[name][combo] != standing:
                clash.append(f"{combo_name(combo)} {name}: {measured[name][combo]} and {standing}")
            measured[name][combo] = standing
    if clash:
        sys.exit("two standings for one combo and faction:\n  " + "\n  ".join(clash))
    for k in said:
        said[k] = sorted(set(said[k]))
    return measured, said


def gm_edited_factions(db):
    """The factions a GM edited in the panel (gm_audit rows of table factions)."""
    try:
        keys = [r[0] for r in db.execute("SELECT rowkey FROM gm_audit WHERE tbl = 'factions'")]
    except sqlite3.OperationalError:
        return set()
    out = set()
    for k in keys:
        try:
            out.add(json.loads(k).get("name"))
        except (ValueError, AttributeError):
            continue
    return out


def plan(db, measured, keep_defaults=False):
    """What --apply would write: the factions to add, the factions standings
    to change, the rows, and what is said: the factions every combo shares a
    start in that the table does not hold (--keep-defaults), the table's
    factions the measurement lacks, a GM's standings and rows kept and the
    combos those rows keep from their measurement, the combos unmeasured."""
    defaults = dict(db.execute("SELECT name, standing FROM factions"))
    combos = sorted({combo_of(r, c, h) for r, c, h in db.execute("SELECT race, tclass, humType FROM defaultcharacter")})
    try:
        gm = [r for r in db.execute(
            "SELECT race, class, humtype, faction, standing FROM factionstart WHERE source != ?", (SOURCE,))]
    except sqlite3.OperationalError:
        gm = []
    gm_by = collections.defaultdict(list)
    for r in gm:
        gm_by[r[3]].append((r[0], r[1], r[2], r[4]))
    gm_keys = {(r[0], r[1], r[2], r[3]) for r in gm}
    edited = gm_edited_factions(db)

    added, new_defaults, rows, shared, gm_defaults, kept_gm, overridden, kept_table = {}, {}, [], [], [], [], [], []
    bases = {}
    for faction in sorted(measured):
        values = measured[faction]
        if faction not in defaults:
            base = mode(list(values.values()))
            added[faction] = base
        else:
            base = defaults[faction]
            want = mode(list(values.values()), base)
            if not keep_defaults and want != base:
                if faction in edited:
                    gm_defaults.append((faction, want, base))
                else:
                    new_defaults[faction] = (base, want)
                    base = want
        bases[faction] = base
        distinct = set(values.values())
        if len(distinct) == 1:
            v = next(iter(distinct))
            if v == base:
                continue
            if faction in edited:
                shared.append((faction, v, base))  # a GM's own standing: left, said
                continue
            kept_table.append((faction, v, base))  # --keep-defaults: a row a race says it
        for race, cls, hum, v in compress(values, base, combos):
            if (race, cls, hum, faction) in gm_keys:
                kept_gm.append((race, cls, hum, faction, v))
                continue
            rows.append((race, cls, hum, faction, v))
    # the check: every measured combo comes out at its measurement, but where
    # a GM's row holds it, or the table keeps its own standing (said)
    by_faction = collections.defaultdict(list)
    for race, cls, hum, faction, v in rows:
        by_faction[faction].append((race, cls, hum, v))
    unshared = {f for f, _, _ in shared}
    for faction, values in measured.items():
        if faction in unshared:
            continue
        effective = by_faction[faction] + gm_by[faction]
        for combo, v in values.items():
            got = resolve(effective, bases[faction], combo)
            if got == v:
                continue
            if any(holds(g, combo) for g in gm_by[faction]):
                overridden.append((combo, faction, v, got))
            else:
                sys.exit(f"the compression is wrong: {combo_name(combo)} {faction} comes out at {got}, measured {v}")
    measured_combos = {c for vals in measured.values() for c in vals}
    return dict(added=added, new_defaults=new_defaults, rows=rows, shared=shared, gm_defaults=gm_defaults,
                kept_gm=kept_gm, overridden=overridden, combos=combos, kept_table=kept_table,
                missing=[c for c in combos if c not in measured_combos],
                unlisted=sorted(measured_combos - set(combos)),
                absent=sorted(f for f in defaults if f not in measured))


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-factionstart"
    n = 2
    while os.path.exists(out):
        out = f"{path}.bak-{when}-{n}-pre-factionstart"
        n += 1
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def apply(db, p):
    db.execute("BEGIN")
    db.execute(SCHEMA)
    db.execute(LOG_SCHEMA)
    for faction, v in sorted(p["added"].items()):
        db.execute("INSERT INTO factions (name, standing) VALUES (?, ?)", (faction, v))
        db.execute(f"INSERT INTO {LOG} (what, faction, new) VALUES ('faction', ?, ?)", (faction, v))
    for faction, (old, new) in sorted(p["new_defaults"].items()):
        db.execute("UPDATE factions SET standing = ? WHERE name = ?", (new, faction))
        db.execute(f"INSERT INTO {LOG} (what, faction, old, new) VALUES ('default', ?, ?, ?)", (faction, old, new))
    for race, cls, hum, faction, v in p["rows"]:
        cur = db.execute("""INSERT INTO factionstart (race, class, humtype, faction, standing, note, source)
            VALUES (?, ?, ?, ?, ?, 'measured on Sandstorm', ?)""", (race, cls, hum, faction, v, SOURCE))
        db.execute(f"INSERT INTO {LOG} (what, rowid, faction, new) VALUES ('row', ?, ?, ?)", (cur.lastrowid, faction, v))
    db.execute("COMMIT")


def undo(db):
    """Put back what the logged --apply wrote; what was changed since is left
    and said."""
    log = list(db.execute(f"SELECT what, rowid, faction, old, new FROM {LOG}"))
    done, kept = collections.Counter(), []
    db.execute("BEGIN")
    for what, rowid, faction, old, new in sorted(log, key=lambda r: ("row", "default", "faction").index(r[0])):
        if what == "row":
            r = db.execute("SELECT standing FROM factionstart WHERE id = ?", (rowid,)).fetchone()
            if r is None:
                continue
            if r[0] != new:
                kept.append(f"factionstart row {rowid} ({faction}): {r[0]} now, the seed wrote {new}")
                continue
            db.execute("DELETE FROM factionstart WHERE id = ?", (rowid,))
            done["rows"] += 1
        elif what == "default":
            r = db.execute("SELECT standing FROM factions WHERE name = ?", (faction,)).fetchone()
            if r is None or r[0] != new:
                kept.append(f"factions {faction!r}: {r[0] if r else 'gone'} now, the seed wrote {new}")
                continue
            db.execute("UPDATE factions SET standing = ? WHERE name = ?", (old, faction))
            done["standings"] += 1
        elif what == "faction":
            r = db.execute("SELECT standing FROM factions WHERE name = ?", (faction,)).fetchone()
            if r is None:
                continue
            if r[0] != new:
                kept.append(f"factions {faction!r}: {r[0]} now, the seed added it at {new}")
                continue
            db.execute("DELETE FROM factions WHERE name = ?", (faction,))
            done["factions"] += 1
            try:
                held = db.execute("SELECT COUNT(*) FROM charfactions WHERE faction = ?", (faction,)).fetchone()[0]
            except sqlite3.OperationalError:
                held = 0
            if held:
                kept.append(f"{held} characters' standings in {faction!r} (charfactions): kept, unlisted")
    db.execute(f"DROP TABLE {LOG}")
    db.execute("COMMIT")
    return done, kept


def report(p, quiet):
    rows = p["rows"]
    per_race = collections.Counter(r[0] for r in rows)
    print(f"{len(p['combos'])} combos in defaultcharacter, {len(p['combos']) - len(p['missing'])} measured; "
          f"{len(rows)} rows on {len({r[3] for r in rows})} factions (by race: "
          + ", ".join(f"{name_of(RACES, r)} {n}" for r, n in sorted(per_race.items())) + ")")
    if p["missing"]:
        print(f"  {len(p['missing'])} combos not measured (--apply refuses but with --partial): "
              + ", ".join(combo_name(c) for c in p["missing"]))
    if p["unlisted"]:
        print(f"  {len(p['unlisted'])} measured combos defaultcharacter lacks (their rows still count): "
              + ", ".join(combo_name(c) for c in p["unlisted"]))
    if p["added"]:
        print(f"  {len(p['added'])} factions Sandstorm lists added to the factions table, at the commonest start:")
        if not quiet:
            for f, v in sorted(p["added"].items()):
                print(f"      {f!r}: {v}")
    if p["absent"]:
        print(f"  {len(p['absent'])} factions of the table the measurement does not list (left): "
              + ", ".join(repr(f) for f in p["absent"]))
    if p["new_defaults"]:
        print(f"  {len(p['new_defaults'])} factions' own standings set to the commonest start (--keep-defaults leaves them):")
        if not quiet:
            for f, (old, new) in sorted(p["new_defaults"].items()):
                print(f"      {f!r}: {old} -> {new}")
    if p["kept_table"]:
        print(f"  {len(p['kept_table'])} factions every combo starts at one standing the factions table does not hold "
              f"(--keep-defaults: a row a race):")
        if not quiet:
            for f, v, base in p["kept_table"]:
                print(f"      {f!r}: measured {v}, the table {base}")
    if p["shared"]:
        print(f"  {len(p['shared'])} factions every combo starts at one standing a GM's own standing in the factions "
              f"table does not hold (left; every character starts at the GM's):")
        for f, v, base in p["shared"]:
            print(f"      {f!r}: measured {v}, the table {base}")
    if p["gm_defaults"]:
        print(f"  {len(p['gm_defaults'])} factions' own standings a GM edited, left:")
        for f, v, base in p["gm_defaults"]:
            print(f"      {f!r}: commonest start {v}, the table {base}")
    if p["kept_gm"]:
        print(f"  {len(p['kept_gm'])} rows left to a GM's own:")
        for race, cls, hum, f, v in p["kept_gm"]:
            print(f"      race {race} class {cls} humtype {hum} {f!r}: measured {v}")
    if p["overridden"]:
        print(f"  {len(p['overridden'])} combos a GM's row keeps from their measured start:")
        for combo, f, v, got in p["overridden"][:40]:
            print(f"      {combo_name(combo)} {f!r}: measured {v}, starts {got}")
    if not quiet:
        for race, cls, hum, f, v in rows:
            who = name_of(RACES, race) + ("" if cls < 0 else " " + name_of(CLASSES, cls)) + \
                {-1: "", 0: "", 1: " (east)", 2: " (west)"}.get(hum, f" (city {hum})")
            print(f"    {who:32} {f!r}: {v}")


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--measured", help="the measured TSV: race, class, humtype, faction, standing")
    ap.add_argument("--apply", action="store_true", help="write the rows, the standings and the factions")
    ap.add_argument("--undo", action="store_true", help="put back what --apply wrote")
    ap.add_argument("--keep-defaults", action="store_true",
                    help="leave each faction's own standing; the rows say every difference from it")
    ap.add_argument("--partial", action="store_true",
                    help="--apply with a defaultcharacter combo unmeasured (a race not all measured gets a row a combo)")
    ap.add_argument("--no-backup", action="store_true", help="write without a copy first")
    ap.add_argument("--quiet", action="store_true", help="the counts, not every row")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None)
    try:
        return run(ap, a, db)
    finally:
        db.close()


def run(ap, a, db):
    db.execute("PRAGMA busy_timeout = 5000")
    logged = db.execute("SELECT name FROM sqlite_master WHERE name = ?", (LOG,)).fetchone()
    if a.undo:
        if not logged:
            print(f"{LOG} is not there: nothing to put back")
            return 0
        if not a.no_backup:
            print("backup:", backup(db, a.db))
        done, kept = undo(db)
        print("put back:", dict(done) or "nothing")
        for k in kept:
            print("  left:", k)
        print("restart the server, or edit a Faction starts row in the GM panel, for the world to read it")
        return 0
    if not a.measured:
        ap.error("--measured is needed")
    if a.apply and logged:
        ap.error(f"{LOG} is there: this database was seeded already (--undo first)")
    factions = [r[0] for r in db.execute("SELECT name FROM factions")]
    if not factions:
        sys.exit("the factions table is empty or missing: start the server once (it makes and fills it)")
    measured, said = read_measured(a.measured, set(factions))
    for what, items in said.items():
        print(f"{len(items)} {what}: " + "; ".join(items[:12]) + (" ..." if len(items) > 12 else ""))
    p = plan(db, measured, a.keep_defaults)
    report(p, a.quiet)
    if not a.apply:
        print("(report only: --apply writes them)")
        return 0
    if p["missing"] and not a.partial:
        ap.error(f"{len(p['missing'])} combos are not measured: the factions' own standings would be the others' "
                 f"(--partial writes anyway)")
    if not a.no_backup:
        print("backup:", backup(db, a.db))
    apply(db, p)
    print(f"wrote {len(p['rows'])} rows, {len(p['new_defaults'])} factions' own standings and "
          f"{len(p['added'])} new factions; restart the server, or edit a Faction starts row in the GM panel, "
          f"for the world to read them")
    return 0


if __name__ == "__main__":
    sys.exit(main())
