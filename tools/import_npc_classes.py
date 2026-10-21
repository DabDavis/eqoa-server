#!/usr/bin/env python3
"""Each NPC name's class, into the server's npcclass table: the class picks
the NPC's melee swing (internal/entity/rounds.go), as a player's picks its
own (CLIENT's GetSwing: a caster always one hit a round, a paladin or shadow
knight two from level 20, a warrior four from level 35).

An NPC row names no class. Two sources do:

- Sandstorm's entity records (byte 145), as `eqoa harvest` recordings saw
  them ("seen", each creature's name, level and class), or a TSV of them
  (`name<TAB>level<TAB>class[<TAB>...]`, # comments). These win.
- Return Home's EQOA_Master.sql: its npcs table's npc_type column, which
  Return Home's own server never reads, holds 0-14 like a class (19,097
  warriors, 1,015 magicians, 571 bards...). On the 562 names both sources
  hold it agrees with Sandstorm on 526; the rest are mostly merchants it
  calls bards and a few casters it calls warriors.

A name takes the class it has most often in a source. Only the names the
database's npcs table holds are written, keyed as the server looks them up
(database.NPCClassKey: trimmed, any case, no trailing '$'); a name with no
class here stays a warrior.

The table is this tool's alone: --apply replaces every row in one
transaction, --clear empties it. Report only by default.

    tools/import_npc_classes.py [--harvest <recording.json, .tsv or dir> ...]
        [--return-home EQOA_Master.sql] [--db eqoa.db] [--apply | --clear]
"""
import argparse
import collections
import glob
import json
import os
import re
import sqlite3
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
CLASSES = "WAR RAN PAL SK MNK BRD RGE DRD SHA CL MAG NEC ENC WIZ ALC".split()

DDL = """
CREATE TABLE IF NOT EXISTS npcclass (
    name   TEXT PRIMARY KEY,
    class  INTEGER NOT NULL,
    source TEXT NOT NULL
);
"""


def key(name):
    """database.NPCClassKey: trimmed, any case, no trailing '$'."""
    n = (name or "").strip().lower()
    if n.endswith("$"):
        n = n[:-1]
    return n.strip()


def harvest_classes(paths):
    """{name key: Counter(class)} out of harvest recordings and TSVs."""
    out = collections.defaultdict(collections.Counter)
    files = []
    for p in paths:
        files += sorted(glob.glob(os.path.join(p, "*.json")) + glob.glob(os.path.join(p, "*.tsv"))) if os.path.isdir(p) else [p]
    for f in files:
        if f.endswith(".tsv"):
            with open(f) as fh:
                for n, raw in enumerate(fh, 1):
                    line = raw.rstrip("\n")
                    if not line.strip() or line.lstrip().startswith("#"):
                        continue
                    cols = line.split("\t")
                    if len(cols) < 3:
                        sys.exit(f"{f}:{n}: want name<TAB>level<TAB>class")
                    weight = int(cols[3]) if len(cols) > 3 and cols[3].strip().isdigit() else 1
                    out[key(cols[0])][int(cols[2])] += weight
            continue
        with open(f) as fh:
            try:
                d = json.load(fh)
            except ValueError:
                continue
        if not isinstance(d, dict):
            continue
        for s in d.get("seen") or []:
            if isinstance(s, dict) and "class" in s:
                out[key(s.get("name"))][s["class"]] += 1
    out.pop("", None)
    return out


def sql_rows(path, table):
    """Each row of `INSERT INTO `table` VALUES (...),(...)` in a MySQL dump, as
    a list of strings (None for NULL), quotes and escapes undone."""
    head = f"INSERT INTO `{table}` VALUES "
    with open(path, encoding="utf-8", errors="replace") as fh:
        for line in fh:
            if not line.startswith(head):
                continue
            s, i, n = line, len(head), len(line)
            row, val, quoted, inrow = [], [], False, False
            while i < n:
                c = s[i]
                if quoted:
                    if c == "\\" and i + 1 < n:
                        val.append({"n": "\n", "t": "\t", "0": "\0"}.get(s[i + 1], s[i + 1]))
                        i += 2
                        continue
                    if c == "'":
                        if i + 1 < n and s[i + 1] == "'":
                            val.append("'")
                            i += 2
                            continue
                        quoted = False
                    else:
                        val.append(c)
                elif c == "'":
                    quoted = True
                elif c == "(" and not inrow:
                    inrow, row, val = True, [], []
                elif c in ",)" and inrow:
                    v = "".join(val).strip()
                    row.append(None if v == "NULL" else v)
                    val = []
                    if c == ")":
                        inrow = False
                        yield row
                elif inrow:
                    val.append(c)
                i += 1


def return_home_classes(path):
    """{name key: Counter(class)} out of EQOA_Master.sql's npcs rows: column
    1 npc_name, column 36 npc_type."""
    out = collections.defaultdict(collections.Counter)
    for row in sql_rows(path, "npcs"):
        if len(row) < 37 or row[36] is None or not re.fullmatch(r"-?\d+", row[36]):
            continue
        out[key(row[1])][int(row[36])] += 1
    out.pop("", None)
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--harvest", nargs="*", default=[], help="eqoa harvest recordings or TSVs of Sandstorm's classes (files or directories)")
    ap.add_argument("--return-home", help="Return Home's EQOA_Master.sql")
    ap.add_argument("--apply", action="store_true", help="replace the table's rows")
    ap.add_argument("--clear", action="store_true", help="empty the table")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    if a.clear:
        db.executescript(DDL)
        db.execute("DELETE FROM main.npcclass")
        print("npcclass emptied")
        return

    ours = collections.Counter(key(n) for (n,) in db.execute("SELECT npc_name FROM main.npcs"))
    ours.pop("", None)
    sandstorm = harvest_classes(a.harvest)
    rh = return_home_classes(a.return_home) if a.return_home else {}

    rows, disagree = {}, []
    for name in ours:
        for source, table in (("sandstorm", sandstorm), ("return home", rh)):
            c = table.get(name)
            if not c:
                continue
            cls = c.most_common(1)[0][0]
            if not 0 <= cls < len(CLASSES):
                continue
            if name not in rows:
                rows[name] = (cls, source)
            elif rows[name][0] != cls:
                disagree.append((name, CLASSES[rows[name][0]], CLASSES[cls]))
            break
    # the disagreements among names both hold, for the report
    for name in ours:
        s, r = sandstorm.get(name), rh.get(name)
        if s and r:
            sc, rc = s.most_common(1)[0][0], r.most_common(1)[0][0]
            if sc != rc and 0 <= sc < 15 and 0 <= rc < 15:
                disagree.append((name, CLASSES[sc], CLASSES[rc]))

    npcs_named = sum(ours.values())
    covered = sum(n for name, n in ours.items() if name in rows)
    print(f"npcs rows {npcs_named}, names {len(ours)}; Sandstorm names {len(sandstorm)}, Return Home names {len(rh)}")
    by = collections.Counter(src for _, src in rows.values())
    print(f"names classed {len(rows)} (Sandstorm {by['sandstorm']}, Return Home {by['return home']}), covering {covered} npcs rows")
    cls_rows = collections.Counter()
    for name, n in ours.items():
        if name in rows:
            cls_rows[CLASSES[rows[name][0]]] += n
    print("npcs rows by class:", ", ".join(f"{c} {n}" for c, n in cls_rows.most_common()))
    print(f"names whose two sources disagree (Sandstorm's taken): {len(disagree)}" + (f", e.g. {disagree[:6]}" if disagree else ""))
    if not a.apply:
        print("(report only: --apply to write it)")
        return
    db.executescript(DDL)
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute("DELETE FROM main.npcclass")
        db.executemany("INSERT INTO main.npcclass (name, class, source) VALUES (?, ?, ?)",
                       [(n, c, src) for n, (c, src) in sorted(rows.items())])
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"applied to {a.db}: {len(rows)} rows")


if __name__ == "__main__":
    main()
