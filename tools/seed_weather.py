#!/usr/bin/env python3
"""The regional weather's seed rows (#1549, internal/weather, database/weather.go): which climate each of
the disc's 22 terrain profiles is, the zones kept off the automatic weather, and how each climate's weather
comes (a GM's numbers; no record of the live game's schedule exists).

- weatherprofiles, 22 rows: 2 and 10 arctic (the icy fog tint 227,247,247), 3 and 11 desert (the sand fog
  tint 233,218,166), 19 none (the Lavastorm weight, where CLIENT drops its fireballs: lava), the rest
  temperate (VIWorld::SetProfiles; vanilla's tod.rs PROFILES).
- weatherzones, 13 rows of none: world 3 cols 0-3 rows 0-2 (the Lavastorm world) and world 5 col 2 row 3
  (Zaoran Plateau, islands in lava).
- weatherclimates: temperate rain, arctic snow (a heavy one a whiteout half the time, held 30-60 s), desert
  sand; a 40-minute cycle, an event each, 6-12 minutes (sand 3-6), light/moderate/heavy 60/30/10, a 30 s fade.

Report only by default: each row, and whether the table holds it already (a GM's row is never written over).
--apply writes the missing rows with source 'seed' and logs each in weather_seed_log; --undo deletes the
logged rows no GM changed since (a row still as it was written). Both take a copy of the database first
(<db>.bak-<when>-pre-weatherseed, sqlite3's backup: never a dump) unless --no-backup. The tables are the
server's; --apply makes them if the server has not yet.

Before: cmd/groundbake -only-profiles beside the ground files (the server reads <world>.profiles at its
start). After: restart the server, or #weather reload, or edit any of the three tables in the GM panel.

    tools/seed_weather.py --db eqoa.db [--apply | --undo] [--no-backup]
"""
import argparse
import datetime
import os
import sqlite3
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
SOURCE = "seed"

# the tables as database/weather.go makes them
SCHEMA = [
    """CREATE TABLE IF NOT EXISTS weatherprofiles (
        profile INTEGER PRIMARY KEY,
        climate TEXT NOT NULL DEFAULT 'temperate',
        note    TEXT NOT NULL DEFAULT '',
        source  TEXT NOT NULL DEFAULT 'gm'
    )""",
    """CREATE TABLE IF NOT EXISTS weatherzones (
        world   INTEGER NOT NULL,
        col     INTEGER NOT NULL,
        row     INTEGER NOT NULL,
        climate TEXT NOT NULL DEFAULT 'none',
        note    TEXT NOT NULL DEFAULT '',
        source  TEXT NOT NULL DEFAULT 'gm',
        PRIMARY KEY (world, col, row)
    )""",
    """CREATE TABLE IF NOT EXISTS weatherclimates (
        climate         TEXT PRIMARY KEY,
        kind            TEXT NOT NULL,
        cycle_minutes   INTEGER NOT NULL DEFAULT 40,
        event_chance    REAL NOT NULL DEFAULT 1,
        event_min       INTEGER NOT NULL,
        event_max       INTEGER NOT NULL,
        weight_light    INTEGER NOT NULL DEFAULT 60,
        weight_moderate INTEGER NOT NULL DEFAULT 30,
        weight_heavy    INTEGER NOT NULL DEFAULT 10,
        whiteout_chance REAL NOT NULL DEFAULT 0,
        whiteout_min    INTEGER NOT NULL DEFAULT 30,
        whiteout_max    INTEGER NOT NULL DEFAULT 60,
        fade_seconds    INTEGER NOT NULL DEFAULT 30,
        note            TEXT NOT NULL DEFAULT ''
    )""",
]
LOG_SCHEMA = """CREATE TABLE IF NOT EXISTS weather_seed_log (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    at      TEXT NOT NULL,
    tbl     TEXT NOT NULL,
    keyjson TEXT NOT NULL,
    rowjson TEXT NOT NULL
)"""

# each table's key columns and the columns a seed row writes
TABLES = {
    "weatherprofiles": (("profile",), ("profile", "climate", "note", "source")),
    "weatherzones": (("world", "col", "row"), ("world", "col", "row", "climate", "note", "source")),
    "weatherclimates": (("climate",), ("climate", "kind", "cycle_minutes", "event_chance", "event_min", "event_max",
                                       "weight_light", "weight_moderate", "weight_heavy", "whiteout_chance",
                                       "whiteout_min", "whiteout_max", "fade_seconds", "note")),
}


def seed_rows():
    """Every seed row: (table, dict of its columns)."""
    out = []
    for p in range(22):
        if p in (2, 10):
            c, note = "arctic", "icy fog tint (227,247,247): snow"
        elif p in (3, 11):
            c, note = "desert", "sand fog tint (233,218,166): sand"
        elif p == 19:
            c, note = "none", "Lavastorm weight: lava"
        else:
            c, note = "temperate", "rain"
        out.append(("weatherprofiles", {"profile": p, "climate": c, "note": note, "source": SOURCE}))
    for col in range(4):
        for row in range(3):
            out.append(("weatherzones", {"world": 3, "col": col, "row": row, "climate": "none",
                                         "note": "the Lavastorm world", "source": SOURCE}))
    out.append(("weatherzones", {"world": 5, "col": 2, "row": 3, "climate": "none",
                                 "note": "Zaoran Plateau, islands in lava", "source": SOURCE}))
    base = {"cycle_minutes": 40, "event_chance": 1.0, "weight_light": 60, "weight_moderate": 30, "weight_heavy": 10,
            "whiteout_chance": 0.0, "whiteout_min": 30, "whiteout_max": 60, "fade_seconds": 30}
    out.append(("weatherclimates", dict(base, climate="temperate", kind="rain", event_min=6, event_max=12,
                                        note="seed: rain, 6-12 min a 40-minute cycle")))
    out.append(("weatherclimates", dict(base, climate="arctic", kind="snow", event_min=6, event_max=12,
                                        whiteout_chance=0.5,
                                        note="seed: snow, 6-12 min a 40-minute cycle; a heavy one a whiteout half the time")))
    out.append(("weatherclimates", dict(base, climate="desert", kind="sand", event_min=3, event_max=6,
                                        note="seed: sandstorms, 3-6 min a 40-minute cycle")))
    return out


def check(rows):
    """The server's row checks (weather.CheckProfileRow, CheckZoneRow, Rule.Check) on the seed's rows."""
    kinds = {"temperate": "rain", "arctic": "snow", "desert": "sand"}
    for t, r in rows:
        if t == "weatherprofiles":
            assert 0 <= r["profile"] <= 255 and r["climate"] in ("temperate", "arctic", "desert", "none"), r
        elif t == "weatherzones":
            assert 0 <= r["world"] <= 29 and r["climate"] in ("temperate", "arctic", "desert", "none"), r
        else:
            assert kinds[r["climate"]] == r["kind"], r
            assert 10 <= r["cycle_minutes"] <= 240 and 0 <= r["event_chance"] <= 1, r
            assert 1 <= r["event_min"] <= r["event_max"] <= 120, r
            assert r["event_max"] * 60 + 720 <= r["cycle_minutes"] * 60, r
            assert r["weight_light"] + r["weight_moderate"] + r["weight_heavy"] > 0, r
            assert 0 <= r["whiteout_chance"] <= 1 and 1 <= r["whiteout_min"] <= r["whiteout_max"] <= 600, r
            assert 1 <= r["fade_seconds"] <= 60, r


def keyof(t, r):
    return tuple(r[c] for c in TABLES[t][0])


def existing(db, t, r):
    """The table's row of r's key, as a dict of the seed's columns; None when there is none."""
    keys, cols = TABLES[t]
    where = " AND ".join(f"{c} = ?" for c in keys)
    got = db.execute(f"SELECT {', '.join(cols)} FROM {t} WHERE {where}", keyof(t, r)).fetchone()
    return None if got is None else dict(zip(cols, got))


def backup(db, path):
    stamp = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{stamp}-pre-weatherseed"
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def report(db, rows):
    """Each row and what the table holds: 'new', 'same' (as the seed's) or 'gm' (a GM's, kept)."""
    out = []
    for t, r in rows:
        have = existing(db, t, r)
        if have is None:
            state = "new"
        elif all(have.get(c) == v for c, v in r.items() if c != "source"):
            state = "same"
        else:
            state = "gm"
        out.append((t, r, state, have))
    return out


def tables_exist(db):
    names = {n for (n,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table'")}
    return all(t in names for t in TABLES)


def apply(db, rows):
    """Write the rows the tables lack, each logged; the count."""
    import json
    for s in SCHEMA:
        db.execute(s)
    db.execute(LOG_SCHEMA)
    now = datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")
    n = 0
    for t, r, state, _ in report(db, rows):
        if state != "new":
            continue
        cols = list(r)
        db.execute(f"INSERT INTO {t} ({', '.join(cols)}) VALUES ({', '.join('?' for _ in cols)})", [r[c] for c in cols])
        db.execute("INSERT INTO weather_seed_log (at, tbl, keyjson, rowjson) VALUES (?, ?, ?, ?)",
                   (now, t, json.dumps(keyof(t, r)), json.dumps(r, sort_keys=True)))
        n += 1
    db.commit()
    return n


def undo(db):
    """Delete the logged rows no GM changed since (still as written); the counts deleted and kept."""
    import json
    names = {n for (n,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table'")}
    if "weather_seed_log" not in names:
        return 0, 0
    gone = kept = 0
    for lid, t, kj, rj in db.execute("SELECT id, tbl, keyjson, rowjson FROM weather_seed_log ORDER BY id").fetchall():
        r = json.loads(rj)
        have = existing(db, t, r) if t in names else None
        if have is not None and all(have.get(c) == v for c, v in r.items()):
            keys = TABLES[t][0]
            db.execute(f"DELETE FROM {t} WHERE " + " AND ".join(f"{c} = ?" for c in keys), keyof(t, r))
            gone += 1
        elif have is not None:
            kept += 1
        db.execute("DELETE FROM weather_seed_log WHERE id = ?", (lid,))
    db.commit()
    return gone, kept


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--apply", action="store_true", help="write the seed rows the tables lack")
    ap.add_argument("--undo", action="store_true", help="delete the seeded rows no GM changed")
    ap.add_argument("--no-backup", action="store_true", help="write without a copy first")
    args = ap.parse_args()
    if args.apply and args.undo:
        sys.exit("--apply or --undo, not both")
    if not os.path.exists(args.db):
        sys.exit(f"no database at {args.db}")
    db = sqlite3.connect(args.db, timeout=30)
    rows = seed_rows()
    check(rows)
    if args.undo:
        if not args.no_backup:
            print("copy:", backup(db, args.db))
        gone, kept = undo(db)
        print(f"deleted {gone} seeded rows; kept {kept} a GM changed since")
        return
    if tables_exist(db):
        rep = report(db, rows)
    else:
        rep = [(t, r, "new", None) for t, r in rows]
        print("(the weather tables are not made yet: --apply makes them as the server does)")
    counts = {}
    for t, r, state, have in rep:
        counts[(t, state)] = counts.get((t, state), 0) + 1
        k = "-".join(str(v) for v in keyof(t, r))
        extra = "" if state != "gm" else f"  [a GM's row kept: {have}]"
        print(f"  {state:4} {t:16} {k:10} {r.get('climate')} {r.get('kind', '')} {r.get('note')}{extra}")
    for t in TABLES:
        print(f"{t}: {counts.get((t, 'new'), 0)} new, {counts.get((t, 'same'), 0)} already so, "
              f"{counts.get((t, 'gm'), 0)} a GM's (kept)")
    if not args.apply:
        print("(report only: --apply writes them)")
        return
    if not args.no_backup:
        print("copy:", backup(db, args.db))
    print(f"wrote {apply(db, rows)} rows; restart the server or #weather reload for the world to read them")


if __name__ == "__main__":
    main()
