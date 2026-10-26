#!/usr/bin/env python3
"""The spells of EQOA-Data's per-class lists that no level gives: the class
masteries ("CM"), the level-60 epics ("60 Epic") and the master classes'
("MCA", "<master class> MCA"). Their records, lines and classspells rows,
the rows marked by source so no class grant hands them out
(internal/database/grantsource.go), while the community spell-visual page
(internal/spellfx) lists them with their classes and how they are got.

The user, F3 pass 17: "we need to add the missing level 50+ spells to the
database so players can match them". tools/import_eqoadata_spells.py made the
leveled spells and left these out on purpose (#template gives a class every
classspells row it may); this makes them on top of that import, with a log
of its own.

What a made record holds is that tool's own plan (its doc string): the
lists' level, line (spelldesc, cut to 127), scope, the icon by the lists'
icon name, ids continuing above its range, and cast, recast and power from
another rank of the line or the kind's records: OURS, not measured.

classspells: each class listing the spell gets a row of source
"eqoa-data cm", "eqoa-data epic", "eqoa-data mca" or "eqoa-data master
<name>" (the lists' spellings straightened: Gaurdian, Buglar, Minstral,
"Storm caller"). A spell of such a name this server already holds a record
of goes to the class by such a row too, when the class has no row of it.
spelleffect: the list's line for each record it makes, never over a line
already there. No scrolls: these are bought, quested or a master class's.

--retag: the class-spells importer gave a class plain "eqoa-data" rows for
the lists' masteries, epics and master classes' spells it found a record of,
so #template hands those out. Off by default (it changes what #template
gives); with it, such a row (the list names the spell for the class only
that way) takes the kind's source.

Report only by default; --apply writes it all in one transaction after a copy
of the database beside it, and logs every row; --undo takes exactly that back
(refused while a character holds a made spell, unless --force). Stop the
server around --apply and --undo.

    tools/import_eqoadata_mastery_spells.py --eqoa-data ../external/EQOA-Data [--db eqoa.db]
        [--apply | --undo [--force]] [--retag] [--list]
"""
import argparse
import collections
import datetime
import json
import os
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import import_eqoadata_spells as base  # noqa: E402

LOG_DDL = """
CREATE TABLE IF NOT EXISTS eqoadata_extra_import_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
-- Every row written or changed, in order: what --undo takes back, last first.
CREATE TABLE IF NOT EXISTS eqoadata_extra_import_rows (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,     -- spellpattern | spelleffect | classspells | retag
    key    INTEGER NOT NULL,  -- spellid; classspells and retag: the class
    key2   INTEGER,           -- classspells and retag: the spellid
    old    TEXT,              -- retag: the source it had
    note   TEXT
);
"""

# The lists' spellings of master classes, straightened (the key is lower case).
MASTER_SPELLING = {"gaurdian": "Guardian", "buglar": "Burglar", "minstral": "Minstrel"}


def kind_source(how):
    """The classspells source of a spell got that way, or None for a way a
    level gives (or one this tool does not know)."""
    h = " ".join((how or "").split())
    low = h.lower()
    if low == "cm":
        return "eqoa-data cm"
    if low.startswith("60") and "epic" in low:
        return "eqoa-data epic"
    if low == "mca":
        return "eqoa-data mca"
    if low.endswith(" mca"):
        name = h[:-4].strip()
        name = MASTER_SPELLING.get(name.lower(), name)
        name = " ".join(w[:1].upper() + w[1:].lower() for w in name.split())
        return f"eqoa-data master {name}"
    return None


def leveled(how):
    return how in base.LEVELED or how.lower() in {s.lower() for s in base.LEVELED}


def plan(db, lists, retag=False):
    """What --apply would write: made records with their lines, classspells
    rows, retags, and the report's tallies."""
    skipped = collections.Counter()
    # (class, name) -> sources the lists give it by: a class listing a name both
    # ways (a level and a mastery) has it by level
    ways = collections.defaultdict(set)
    for e in lists:
        key = base.spell_name(e["name"])
        if not key:
            continue
        if leveled(e["how"]):
            ways[(e["cls"], key)].add(None)
            continue
        src = kind_source(e["how"])
        if src is None:
            skipped[f"got by {e['how']!r} (unknown)"] += 1
            continue
        ways[(e["cls"], key)].add(src)

    # the server's records and rows before this import
    by_name = collections.defaultdict(list)  # name -> [(spellid, level)]
    for sid, lvl, name in db.execute("SELECT spellid, abilitylvl, spellname FROM main.spellpattern"):
        by_name[base.spell_name(name)].append((sid, lvl))
    rows_of = collections.defaultdict(dict)  # class -> spellid -> source
    for cls, sid, src in db.execute("SELECT class, spellid, COALESCE(source, '') FROM main.classspells"):
        rows_of[cls][sid] = src

    # the records the server lacks, as the leveled import plans them (its
    # references are every list's, so the numbers are as good as its own)
    p = base.plan(db, lists, all_sources=True)
    made = []
    for m in p["spells"]:
        if m["classes"]:  # a leveled record that tool would make: its job, not this one's
            skipped["a leveled spell import_eqoadata_spells.py would make"] += 1
            continue
        made.append(m)
    made_ids = {m["row"]["spellid"] for m in made}
    effects = [e for e in p["effects"] if e[0] in made_ids]

    links = []   # (class, spellid, source, note)
    retags = []  # (class, spellid, old, new, note)
    taken = {(c, s) for c in rows_of for s in rows_of[c]}
    for m in made:
        key, level = base.spell_name(m["row"]["spellname"]), m["row"]["abilitylvl"]
        for e in lists:
            if e["level"] != level or base.spell_name(e["name"]) != key:
                continue
            src = kind_source(e["how"])
            if src is None or None in ways[(e["cls"], key)] or (e["cls"], m["row"]["spellid"]) in taken:
                continue
            taken.add((e["cls"], m["row"]["spellid"]))
            links.append((e["cls"], m["row"]["spellid"], src, f"{base.CLASS_FILES[e['cls']]} L{level} ({e['how']}); made"))
    for e in lists:
        key = base.spell_name(e["name"])
        src = kind_source(e["how"])
        have = by_name.get(key)
        if not key or src is None or not have or None in ways[(e["cls"], key)]:
            continue
        held = [sid for sid, _ in have if sid in rows_of[e["cls"]]]
        if held:
            for sid in held:
                old = rows_of[e["cls"]][sid]
                if old == "eqoa-data" and retag and (e["cls"], sid) not in {(c, s) for c, s, _, _, _ in retags}:
                    retags.append((e["cls"], sid, old, src, f"{base.CLASS_FILES[e['cls']]} L{e['level']} ({e['how']})"))
            continue
        sid = min(have, key=lambda r: (r[1] != e["level"], r[0]))[0]
        if (e["cls"], sid) in taken:
            continue
        taken.add((e["cls"], sid))
        links.append((e["cls"], sid, src, f"{base.CLASS_FILES[e['cls']]} L{e['level']} ({e['how']}); record held"))

    # what --retag would change, counted even without it
    retag_candidates = 0
    for e in lists:
        key = base.spell_name(e["name"])
        if kind_source(e["how"]) is None or None in ways[(e["cls"], key)]:
            continue
        retag_candidates += sum(1 for sid, _ in by_name.get(key, []) if rows_of[e["cls"]].get(sid) == "eqoa-data")
    return dict(spells=made, effects=effects, links=links, retags=retags, skipped=skipped,
                retag_candidates=retag_candidates)


def kind_of_source(src):
    return src.split(" ", 2)[1] if src.startswith("eqoa-data ") else src


def report(p, full=False):
    print(f"records to make {len(p['spells'])}; lines {len(p['effects'])}; class rows {len(p['links'])} "
          f"(on {sum(1 for l in p['links'] if l[3].endswith('made'))} made records, "
          f"{sum(1 for l in p['links'] if l[3].endswith('record held'))} on records held); "
          f"retags {len(p['retags'])} (rows --retag would change: {p['retag_candidates']})")
    per = collections.Counter()
    kinds = collections.Counter()
    for cls, _, src, _ in p["links"]:
        per[(cls, kind_of_source(src))] += 1
        kinds[kind_of_source(src)] += 1
    for cls, name in enumerate(base.CLASSES):
        row = ", ".join(f"{k} {per[(cls, k)]}" for k in ("cm", "epic", "mca", "master") if per[(cls, k)])
        print(f"  {name:3s} {row or '-'}")
    print("  by kind: " + ", ".join(f"{k} {n}" for k, n in kinds.most_common()))
    levels = collections.Counter("50+" if m["row"]["abilitylvl"] >= 50 else "below 50" for m in p["spells"])
    print("  records by level: " + ", ".join(f"{k} {n}" for k, n in sorted(levels.items())))
    for why, n in p["skipped"].most_common():
        print(f"  not made: {n} {why}")
    if full:
        for m in p["spells"]:
            r = m["row"]
            print(f"  {r['spellid']} L{r['abilitylvl']:<2d} {r['spellname'][:32]:32s} scope {r['scope']} cast {r['casttime']} "
                  f"recast {r['recast']} power {r['power']:4d} [{'; '.join(m['notes'])}]")
        for cls, sid, src, note in p["links"]:
            print(f"  class {base.CLASSES[cls]} <- {sid} '{src}' [{note}]")
        for cls, sid, old, new, note in p["retags"]:
            print(f"  retag class {base.CLASSES[cls]} {sid}: '{old}' -> '{new}' [{note}]")


def log(db, tbl, key, key2=None, old=None, note=None):
    db.execute("INSERT INTO eqoadata_extra_import_rows (tbl, key, key2, old, note) VALUES (?, ?, ?, ?, ?)",
               (tbl, key, key2, old, note))


def backup(path):
    stamp = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{stamp}-pre-masteryspells"
    src = sqlite3.connect(path)
    dst = sqlite3.connect(out)
    with dst:
        src.backup(dst)
    dst.close()
    src.close()
    print(f"copy: {out}")


def apply(db, p):
    db.executescript(LOG_DDL)
    if db.execute("SELECT count(*) FROM eqoadata_extra_import_rows").fetchone()[0]:
        sys.exit("this database already holds this import; --undo it first")
    scols = base.columns(db, "spellpattern")
    db.execute("BEGIN IMMEDIATE")
    try:
        seq = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'spellpattern'").fetchone()
        for m in p["spells"]:
            r = m["row"]
            cols = [c for c in scols if c in r]
            db.execute(f"INSERT INTO main.spellpattern ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                       [r[c] for c in cols])
            log(db, "spellpattern", r["spellid"], note="; ".join(m["notes"]))
        for sid, line, source in p["effects"]:
            if db.execute("SELECT 1 FROM main.spelleffect WHERE spellid = ?", (sid,)).fetchone():
                continue
            db.execute("INSERT INTO main.spelleffect (spellid, line, fxdict, source) VALUES (?, ?, 0, ?)",
                       (sid, line, source))
            log(db, "spelleffect", sid)
        for cls, sid, src, note in p["links"]:
            c = db.execute("INSERT OR IGNORE INTO main.classspells (class, spellid, source) VALUES (?, ?, ?)",
                           (cls, sid, src))
            if c.rowcount:
                log(db, "classspells", cls, sid, note=note)
        for cls, sid, old, new, note in p["retags"]:
            db.execute("UPDATE main.classspells SET source = ? WHERE class = ? AND spellid = ? AND source = ?",
                       (new, cls, sid, old))
            log(db, "retag", cls, sid, old=old, note=note)
        now = datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")
        meta = {"applied_at": now, "spellpattern_seq": json.dumps(seq[0] if seq else None),
                "counts": json.dumps({k: len(p[k]) for k in ("spells", "effects", "links", "retags")})}
        for k, v in meta.items():
            db.execute("INSERT OR REPLACE INTO eqoadata_extra_import_meta VALUES (?, ?)", (k, v))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db, force):
    if not db.execute("SELECT count(*) FROM sqlite_master WHERE name = 'eqoadata_extra_import_rows'").fetchone()[0]:
        sys.exit("no mastery/epic spell import to undo here")
    held = db.execute("SELECT count(*) FROM main.spells WHERE spellid IN (SELECT key FROM "
                      "eqoadata_extra_import_rows WHERE tbl = 'spellpattern')").fetchone()[0]
    if held and not force:
        sys.exit(f"characters hold {held} made spells; --force to take them out with the import")
    db.execute("BEGIN IMMEDIATE")
    try:
        if force:
            db.execute("DELETE FROM main.spells WHERE spellid IN (SELECT key FROM eqoadata_extra_import_rows "
                       "WHERE tbl = 'spellpattern')")
        for tbl, key, key2, old in db.execute(
                "SELECT tbl, key, key2, old FROM eqoadata_extra_import_rows ORDER BY seq DESC").fetchall():
            if tbl == "classspells":
                db.execute("DELETE FROM main.classspells WHERE class = ? AND spellid = ?", (key, key2))
            elif tbl == "retag":
                db.execute("UPDATE main.classspells SET source = ? WHERE class = ? AND spellid = ?", (old, key, key2))
            else:
                db.execute(f"DELETE FROM main.{tbl} WHERE spellid = ?", (key,))
        r = db.execute("SELECT value FROM eqoadata_extra_import_meta WHERE key = 'spellpattern_seq'").fetchone()
        seq = json.loads(r[0]) if r else None
        if seq is None:
            db.execute("DELETE FROM sqlite_sequence WHERE name = 'spellpattern'")
        else:
            db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = 'spellpattern'", (seq,))
        db.execute("DROP TABLE eqoadata_extra_import_rows")
        db.execute("DROP TABLE eqoadata_extra_import_meta")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print("undone")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=base.DEFAULT_DB)
    ap.add_argument("--eqoa-data", help="EQOA-Data's checkout, or its Information/databases/spells directory")
    ap.add_argument("--apply", action="store_true", help="write it (a copy first, one transaction, logged)")
    ap.add_argument("--undo", action="store_true", help="take back what --apply wrote")
    ap.add_argument("--force", action="store_true", help="--undo even when characters hold made spells")
    ap.add_argument("--retag", action="store_true",
                    help="also mark the class-spells importer's plain rows of these spells (changes #template)")
    ap.add_argument("--no-backup", action="store_true", help="--apply without the copy")
    ap.add_argument("--list", action="store_true", help="list every record and row it would write")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    if a.undo:
        undo(db, a.force)
        return
    if not a.eqoa_data:
        sys.exit("--eqoa-data is required")
    p = plan(db, base.load_lists(a.eqoa_data), a.retag)
    report(p, a.list)
    if not a.apply:
        print("(report only: --apply to write it)")
        return
    if not a.no_backup:
        db.close()
        backup(a.db)
        db = sqlite3.connect(a.db, isolation_level=None)
    apply(db, p)
    print(f"applied to {a.db}")


if __name__ == "__main__":
    main()
