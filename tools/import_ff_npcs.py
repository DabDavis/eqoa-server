#!/usr/bin/env python3
"""
Import FrontiersForge's shipped creature database into a STAGING table.

Source: FrontiersForge/scripts/resources/entity_database.json — a player-collected
journal shipped with the world_map mod. Format is one entry per creature name with
three PARALLEL arrays, one element per recorded spawn:

    { "Grass Snake": { "world": [0], "level": [3], "pos": [[x, y, z]] } }

What this does NOT do: invent a model. FrontiersForge records no model id, and
guessing one from the name is unreliable — in our own data the token "guard"
maps to its most common model only 42% of the time, "merchant" 46%, "wolf" 38%.
So a model is only attached when it is EVIDENCED:

  exact       the same npc_name already exists in npcs; take its model/size/race/type
  head:<w>    the HEAD NOUN (last word) matches a word that is the head noun of our
              own names and is >=90% dominant for one model over >=3 rows
  title:<w>   the head noun is a word we have never seen at all (a proper name, e.g.
              "Deputy Crazyeyes"), so the LEADING word decides instead, under the
              same dominance test against our own leading words
  (null)      no evidence. These rows are staged but are NOT spawnable as-is.

The head noun always wins. A modifier is consulted ONLY when the head noun is
unknown to us, never when it is merely ambiguous: "tainted wolf" must come back
unresolved, because in our data "tainted" only ever appears in "Tainted Mummy"
and letting it decide would render a wolf as a mummy.

Everything lands in npcs_ff_import. Nothing touches npcs. Promotion is a separate,
deliberate step (see --promote).

Usage:
    python3 tools/import_ff_npcs.py                 # (re)build the staging table
    python3 tools/import_ff_npcs.py --report        # summarise without writing
    python3 tools/import_ff_npcs.py --promote exact,head    # copy those into npcs
"""

import argparse
import collections
import json
import math
import os
import re
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
SERVER = os.path.dirname(HERE)
REPO = os.path.dirname(SERVER)

DEFAULT_DB = os.path.join(SERVER, "eqoa.db")
DEFAULT_JSON = os.path.join(
    REPO, "FrontiersForge", "scripts", "resources", "entity_database.json")

# A staged spawn within this many world units of an existing row with the same
# name is treated as the same spawn we already have, not a new one.
DUP_RADIUS = 50.0

TOKEN_MIN_DOMINANCE = 0.90
TOKEN_MIN_SIGHTINGS = 3

# A journal position shared by this many DISTINCT creature names is not a place
# any of them stands. The source has several such points — 426 different names at
# exactly (18248, 24, 32000), 355 at (3001, 24, 18025) — where z is the clamp
# value 32000 or x is the world minimum. Whatever produced them (a transport, a
# streaming boundary, an unpopulated position field), the coordinate is junk even
# though the name and level beside it are fine. Rows on such a point are staged
# and marked, never promoted as a spawn.
POS_SENTINEL_NAMES = 8

# The named extent of each world, taken from the game's OWN world record table
# (CLIENT dump 0x007504C0, read live on 2026-08-19 — see
# claude-notes/frontiersforge-layouts-verified-20260819.md). Each record carries
# inclusive place-name grid bounds; the grid is offset one cell from the scene
# grid (row = cell_z - 1) and the scene cell is 2000 units, so a world's named
# region runs (bound + 1) * 2000 on each axis.
#
# This is the right bound to use and our own npcs table is NOT: ours only covers
# x 13335..25681, z 3072..22194, a fraction of Tunaria, so anything measured
# against it would reject the whole frozen north as out of world. It also catches
# the one record where the source duplicated y into z (Stone Golem at
# z = 54.7656 = its own y).
WORLD_CELL = 2000.0
WORLD_BOUNDS = {  # world -> (max_col, max_row) from the game's table
    0: (12, 15), 1: (3, 3), 2: (5, 6), 3: (2, 1), 4: (1, 2), 5: (1, 2),
}


def in_world_bounds(world, x, z):
    limits = WORLD_BOUNDS.get(world)
    if limits is None:
        return True  # a world the table does not describe; do not judge it
    max_x = (limits[0] + 1) * WORLD_CELL
    max_z = (limits[1] + 1) * WORLD_CELL
    return WORLD_CELL <= x <= max_x and WORLD_CELL <= z <= max_z

STAGING_DDL = """
CREATE TABLE IF NOT EXISTS npcs_ff_import (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    npc_name     TEXT    NOT NULL,
    npc_level    INTEGER NOT NULL,
    world        INTEGER NOT NULL,
    x_coord      REAL    NOT NULL,
    y_coord      REAL    NOT NULL,
    z_coord      REAL    NOT NULL,
    modelid      INTEGER,          -- NULL when no model could be evidenced
    size         REAL,
    race         TEXT,
    npc_type     INTEGER,
    model_source TEXT,             -- 'exact' | 'head:<word>' | 'title:<word>' | NULL
    dup_of       INTEGER,          -- npcs.id this duplicates, else NULL
    pos_suspect  INTEGER NOT NULL DEFAULT 0,  -- 1 shared sentinel, 2 outside the world
    promoted_id  INTEGER,          -- npcs.id created by --promote, else NULL
    source       TEXT NOT NULL DEFAULT 'frontiersforge'
);
CREATE INDEX IF NOT EXISTS idx_ffimport_name   ON npcs_ff_import(npc_name);
CREATE INDEX IF NOT EXISTS idx_ffimport_source ON npcs_ff_import(model_source);
CREATE INDEX IF NOT EXISTS idx_ffimport_dup    ON npcs_ff_import(dup_of);
CREATE INDEX IF NOT EXISTS idx_ffimport_pos    ON npcs_ff_import(pos_suspect);
"""

ARTICLE_RE = re.compile(r"^(a|an|the)\s+", re.I)
WORD_RE = re.compile(r"[^A-Za-z']+")


def tokens(name):
    return [w for w in WORD_RE.split(ARTICLE_RE.sub("", name.strip())) if w]


def _dominant(counters):
    """word -> model, for words where one model holds >=TOKEN_MIN_DOMINANCE of
    >=TOKEN_MIN_SIGHTINGS observations."""
    out = {}
    for word, counter in counters.items():
        total = sum(counter.values())
        model, hits = counter.most_common(1)[0]
        if total >= TOKEN_MIN_SIGHTINGS and hits / total >= TOKEN_MIN_DOMINANCE:
            out[word] = model
    return out


def load_existing(conn):
    """Existing world-0 NPCs: per-name appearance, a name->positions index, and
    two separately trained model vocabularies (head nouns, leading titles)."""
    rows = conn.execute(
        "SELECT id, npc_name, modelid, size, race, npc_type, x_coord, z_coord "
        "FROM npcs WHERE world = 0").fetchall()
    by_name = {}
    positions = collections.defaultdict(list)
    head_counts = collections.defaultdict(collections.Counter)
    title_counts = collections.defaultdict(collections.Counter)
    vocabulary = set()
    for rid, name, model, size, race, ntype, x, z in rows:
        if name is None:
            continue
        by_name.setdefault(name, (model, size, race, ntype))
        positions[name].append((rid, x, z))
        words = tokens(name)
        if not words:
            continue
        vocabulary.update(w.lower() for w in words)
        head_counts[words[-1].lower()][model] += 1
        if len(words) >= 2:
            title_counts[words[0].lower()][model] += 1
    vocab = {
        "head": _dominant(head_counts),
        "title": _dominant(title_counts),
        "known": vocabulary,
    }
    return by_name, positions, vocab, len(rows)


def resolve_model(name, by_name, vocab):
    """The head noun always decides. A leading title is consulted only when the
    head noun is a word we have never seen at all — never when it is merely
    ambiguous, or a modifier would overrule the thing it modifies."""
    if name in by_name:
        model, size, race, ntype = by_name[name]
        return model, size, race, ntype, "exact"

    words = tokens(name)
    if not words:
        return None, None, None, None, None

    head = words[-1].lower()
    model = vocab["head"].get(head)
    if model is not None:
        return model, 1.0, "", 0, "head:" + head

    if head in vocab["known"]:
        # We know this noun; it just is not decisive. Do not let a modifier speak
        # for it.
        return None, None, None, None, None

    if len(words) >= 2:
        title = words[0].lower()
        model = vocab["title"].get(title)
        if model is not None:
            return model, 1.0, "", 0, "title:" + title

    return None, None, None, None, None


def find_dup(name, x, z, positions):
    best_id, best_d = None, None
    for rid, ox, oz in positions.get(name, ()):
        d = math.hypot(x - ox, z - oz)
        if best_d is None or d < best_d:
            best_id, best_d = rid, d
    if best_d is not None and best_d <= DUP_RADIUS:
        return best_id
    return None


def sentinel_positions(data):
    """Positions occupied by POS_SENTINEL_NAMES or more distinct creature names."""
    at = collections.defaultdict(set)
    for name, rec in data.items():
        if not name:
            continue
        for pos in rec.get("pos") or []:
            if isinstance(pos, list) and len(pos) == 3:
                at[(round(pos[0], 2), round(pos[1], 2), round(pos[2], 2))].add(name)
    return {k for k, names in at.items() if len(names) >= POS_SENTINEL_NAMES}


def build_rows(data, by_name, positions, vocab):
    sentinels = sentinel_positions(data)
    staged, skipped = [], []
    for name, rec in data.items():
        if not name or not name.strip():
            skipped.append(("empty name", name, rec))
            continue
        worlds = rec.get("world") or []
        levels = rec.get("level") or []
        posns = rec.get("pos") or []
        if not (len(worlds) == len(levels) == len(posns)):
            skipped.append(("parallel arrays disagree", name, rec))
            continue
        model, size, race, ntype, src = resolve_model(name, by_name, vocab)
        for world, level, pos in zip(worlds, levels, posns):
            if not isinstance(pos, list) or len(pos) != 3:
                skipped.append(("bad position", name, pos))
                continue
            x, y, z = pos
            if (round(x, 2), round(y, 2), round(z, 2)) in sentinels:
                suspect = 1
            elif not in_world_bounds(int(world), x, z):
                suspect = 2
            else:
                suspect = 0
            staged.append((name, int(level), int(world), float(x), float(y), float(z),
                           model, size, race, ntype, src,
                           find_dup(name, x, z, positions), suspect))
    return staged, skipped


def report(conn):
    q = conn.execute
    total = q("SELECT COUNT(*) FROM npcs_ff_import").fetchone()[0]
    names = q("SELECT COUNT(DISTINCT npc_name) FROM npcs_ff_import").fetchone()[0]
    dups = q("SELECT COUNT(*) FROM npcs_ff_import WHERE dup_of IS NOT NULL").fetchone()[0]
    susp = q("SELECT COUNT(*) FROM npcs_ff_import WHERE pos_suspect = 1").fetchone()[0]
    oob = q("SELECT COUNT(*) FROM npcs_ff_import WHERE pos_suspect = 2").fetchone()[0]
    prom = q("SELECT COUNT(*) FROM npcs_ff_import WHERE promoted_id IS NOT NULL").fetchone()[0]
    print("npcs_ff_import: %d spawn rows over %d distinct names" % (total, names))
    print("  duplicates of an existing npcs row (<= %.0f units, same name): %d" % (DUP_RADIUS, dups))
    print("  on a shared sentinel position (>= %d names on one point): %d" % (POS_SENTINEL_NAMES, susp))
    print("  outside the world's own named extent: %d" % oob)
    print("  already promoted into npcs: %d" % prom)
    print("  PROMOTABLE (model + not a duplicate + real position): %d" % q(
        "SELECT COUNT(*) FROM npcs_ff_import WHERE modelid IS NOT NULL "
        "AND dup_of IS NULL AND pos_suspect = 0 AND promoted_id IS NULL").fetchone()[0])
    print("  model evidence:")
    for src, n, distinct, promotable in q(
            "SELECT COALESCE(SUBSTR(model_source, 1, "
            "  CASE WHEN INSTR(model_source, ':') > 0 THEN INSTR(model_source, ':') - 1 "
            "  ELSE LENGTH(model_source) END), '(none)'), "
            "COUNT(*), COUNT(DISTINCT npc_name), "
            "SUM(dup_of IS NULL AND pos_suspect = 0) "
            "FROM npcs_ff_import GROUP BY 1 ORDER BY 2 DESC"):
        print("    %-8s %6d rows  %5d names  %5d promotable" % (src, n, distinct, promotable))
    print("  level range: %d..%d" % q(
        "SELECT MIN(npc_level), MAX(npc_level) FROM npcs_ff_import").fetchone())


def promote(conn, sources):
    """Copy staged rows into npcs. Only rows with a model, not duplicates, not
    already promoted. Returns the number inserted."""
    # 'observed' is written by re-tools/npc_model_harvest.py --apply, which reads
    # the real model id off a running client (entity record +0x4C). It is the only
    # class here that is measured rather than inferred.
    valid = {"exact", "observed", "head", "title"}
    if not sources or not sources <= valid:
        raise SystemExit("--promote takes a comma list of: " + ", ".join(sorted(valid)))
    where = ["modelid IS NOT NULL", "dup_of IS NULL", "promoted_id IS NULL",
             "pos_suspect = 0"]
    clauses = []
    for cls in sorted(sources):
        clauses.append("model_source = '%s'" % cls if cls in ("exact", "observed")
                       else "model_source LIKE '%s:%%'" % cls)
    where.append("(" + " OR ".join(clauses) + ")")
    rows = conn.execute(
        "SELECT id, npc_name, npc_level, world, x_coord, y_coord, z_coord, "
        "modelid, size, race, npc_type FROM npcs_ff_import WHERE " +
        " AND ".join(where)).fetchall()
    inserted = 0
    for (sid, name, level, world, x, y, z, model, size, race, ntype) in rows:
        cur = conn.execute(
            "INSERT INTO npcs (npc_name, zone, x_coord, y_coord, z_coord, facing, "
            "world, hp, hp_percent, modelid, size, npc_level, npc_type, npc_id) "
            "VALUES (?, 0, ?, ?, ?, 0, ?, 100, 255, ?, ?, ?, ?, 0)",
            (name, x, y, z, world, model, size if size else 1.0, level, ntype or 0))
        conn.execute("UPDATE npcs_ff_import SET promoted_id = ? WHERE id = ?",
                     (cur.lastrowid, sid))
        inserted += 1
    conn.commit()
    return inserted


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--json", default=DEFAULT_JSON)
    ap.add_argument("--report", action="store_true", help="summarise, do not write")
    ap.add_argument("--promote", metavar="SOURCES",
                    help="comma list of model_source classes to copy into npcs: "
                         "exact, observed, head, title")
    args = ap.parse_args()

    conn = sqlite3.connect(args.db)
    conn.execute("PRAGMA foreign_keys = ON")

    if args.report:
        report(conn)
        return
    if args.promote:
        n = promote(conn, set(s.strip() for s in args.promote.split(",")))
        print("promoted %d staged rows into npcs" % n)
        report(conn)
        return

    with open(args.json, encoding="utf-8") as fh:
        data = json.load(fh)

    by_name, positions, vocab, existing = load_existing(conn)
    print("existing world-0 npcs rows: %d over %d names" % (existing, len(by_name)))
    print("decisive head nouns: %d   decisive leading titles: %d   (>=%.0f%% of >=%d rows)"
          % (len(vocab["head"]), len(vocab["title"]),
             TOKEN_MIN_DOMINANCE * 100, TOKEN_MIN_SIGHTINGS))

    staged, skipped = build_rows(data, by_name, positions, vocab)
    print("staging %d spawn rows (%d source entries skipped)" % (len(staged), len(skipped)))
    for reason, name, _ in skipped:
        print("   skipped: %-24s %r" % (reason, name))

    conn.executescript(STAGING_DDL)
    conn.execute("DELETE FROM npcs_ff_import WHERE promoted_id IS NULL")
    conn.executemany(
        "INSERT INTO npcs_ff_import (npc_name, npc_level, world, x_coord, y_coord, "
        "z_coord, modelid, size, race, npc_type, model_source, dup_of, pos_suspect) "
        "VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)", staged)
    conn.commit()
    print()
    report(conn)


if __name__ == "__main__":
    main()
