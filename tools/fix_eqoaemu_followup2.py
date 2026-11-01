#!/usr/bin/env python3
"""The EQOAEmu import's second follow-up (the user, 2026-11-01 ~18:55 UTC, of
the first follow-up's open questions: "lets fix all these"), on a database
the import and its first follow-up (tools/fix_eqoaemu_followup.py) were
applied to: what tools/import_eqoaemu_spells.py's rules now make that they did
not then, with the operator's map as it is now, so the database ends as a
fresh import with that map would leave it (the import's plan made again here,
on this database, and only these parts of it taken):

  1. One spell under two names: a mastery rank the masteries import moved
     from a record of ours to a new one (its log, cmcatalog_eqoaemu_log) whose
     workbook row the map now puts on ours ("spelled otherwise", the
     operator's word that they are one spell: the Transmuter's second rank,
     the workbook's and the masteries lists' Creative Transmutation, our
     Create Transformative, the 2004 records' and EQOA-Data's spell list's
     name). Ours stays, its record and effects row the workbook's (its name
     kept: import_eqoaemu_spells.py's rename_safe); the new record goes, and
     what named it names ours, as the first follow-up's merge did it
     (merge_changes: a book holding both keeps ours, one holding the new alone
     takes ours, the masteries, quests and kept buffs naming it name ours, its
     class rows and area go). A rank moved to a new record the map does not
     put on ours is two spells: listed, left.
  2. The item such a spell makes is the one its line names (the
     Transformative Pill), and it takes the workbook's use over its own as the
     record takes the workbook's record (the Creative Mix Potion's: a heal of
     1500 after its 1500 more maximum, 150 more STR, STA, AGI and DEX, 30
     minutes, the ogre's body; its effect's name kept); the item the import
     made for the workbook's name (the Creative Potion) is folded into it:
     every copy anyone holds (inventories and banks, the auction, the weapon
     sets) and every merchant's, loot table's, start kit's, catch's or
     quest's naming it made to name the line's, then its use and its record
     taken out. Only an item the import or the first follow-up made (their
     logs' inserts) is folded; one of ours is listed and left.
  3. What a use's columns say now (itemeffect.body and target_line, the
     engine's of 2026-11-01): a potion's body (its Alter Appearance sprite
     the other way round: the Transmutive Potion's on the creature it hits)
     and a thrown target-based area's target's own share (HP1 beside the
     line's HP2: the Cocktail of Death Potion's, Severupidus' Cocktail's), on
     every use the import wrote (a value a GM gave since left, and said).

A spell's effects row is changed only where it is as the import's log wrote
it (a GM's change since is left, and said); every other row only where it is
still as planned. Report by default. --save-plan FILE writes what --apply
would do and stops (the plan needs the workbook, and with --iso the disc, for
a use's icon as the import reads icons); --plan FILE applies such a plan where
the database is, its merges and folds read there again (a book, a copy or a
lot that came to name the new record or the folded item since the plan was
made is taken too). --apply writes it all in one transaction, every row
logged in eqoaemu_followup2_log by the import's own apply (its key, the
columns before and after, a deleted row's rowid; itemeffect's two new columns
added to a database made before them); --undo puts each logged row back as it
was (one changed since is left, and said) and drops those columns again. Both
take a copy of the database first (SQLite's backup API). The server loads
the potions' uses, the masteries and the books at its start: restart it after
either.

    tools/fix_eqoaemu_followup2.py --sheets <dir of TSVs> | --ods <workbook.ods> --map <spellmap.tsv>
        [--iso <disc.iso>] [--db eqoa.db] [--save-plan FILE]
    tools/fix_eqoaemu_followup2.py --db eqoa.db --plan FILE --apply
    tools/fix_eqoaemu_followup2.py --db eqoa.db --undo
"""
import argparse
import collections
import datetime
import json
import os
import re
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import import_eqoaemu_spells as im  # noqa: E402
import fix_eqoaemu_followup as fu  # noqa: E402

DEFAULT_DB = os.path.join(HERE, "..", "eqoa.db")
LOG = "eqoaemu_followup2_log"
META = "eqoaemu_followup2_meta"
WHAT = "the EQOAEmu import's second follow-up"

# where a folded item's copies and mentions are: (table, the columns that key
# a row, the column naming the item)
FOLD_TABLES = [
    ("charinventory", ["itemid"], "patternid"),       # inventories and banks
    ("auction_lots", ["lot"], "patternid"),
    ("gearsets", ["serverid", "slot"], "mainpattern"),
    ("gearsets", ["serverid", "slot"], "offpattern"),
    ("npcitem", ["itemid"], "patternid"),             # merchants
    ("loottables", ["id"], "patternid"),
    ("lootpool", ["id"], "patternid"),
    ("startitems", ["id"], "patternid"),
    ("fishcatch", ["id"], "itemid"),
]
# and the lists of ids naming items (a quest step's items, rewards, choices)
FOLD_LISTS = [("queststeps", ["questid", "step"], ("items", "rewarditems", "byclass", "choices"))]
for _t, _k, _c in FOLD_TABLES + FOLD_LISTS:
    im.KEYS.setdefault(_t, _k)
# the tables an item the import made is logged in, by the tool that made it
MADE_BY = (im.LOG, fu.LOG)


def merges(db, smap, lists):
    """The doc's 1: (ours, new, the rank) of each rank the masteries import
    moved from a record of ours to a new one whose workbook row the map puts
    on ours."""
    out = []
    if not im.has_table(db, "cmcatalog_eqoaemu_log"):
        return out
    recs = {r[0]: r[1] for r in db.execute("SELECT spellid, spellname FROM main.spellpattern")}
    for rid, op, before, after in db.execute("SELECT id, op, before, after FROM main.cmcatalog_eqoaemu_log ORDER BY rowid"):
        b, a = fu.loads(before) or {}, fu.loads(after) or {}
        if op != "update" or not isinstance(b, dict) or not isinstance(a, dict) or \
                "add_spells" not in b or "add_spells" not in a:
            continue
        old, new = [int(x) for x in re.findall(r"\d+", b["add_spells"])], [int(x) for x in re.findall(r"\d+", a["add_spells"])]
        if len(old) != 1 or len(new) != 1 or old[0] >= im.NEW_BASE or new[0] < im.NEW_BASE:
            continue
        ours, made = old[0], new[0]
        if ours not in recs or made not in recs:
            continue  # one made already (the first follow-up's Transmutive Mixture), or gone
        tag = f"rank {rid}: {made} {recs[made]!r} -> {ours} {recs[ours]!r}"
        m = im.map_entry(smap, f"{made - im.NEW_BASE:04X}")
        if m is None or m[0] != ours or not m[1].startswith("spelled otherwise"):
            lists["a rank the masteries import moved to a new record the map does not put on ours (left: two spells)"].append(tag)
            continue
        out.append(dict(ours=ours, new=made, rank=rid, ours_name=recs[ours], new_name=recs[made]))
        lists["one spell's two records made one (ours kept: the map's row on it)"].append(tag)
    return out


def made_here(db, pid):
    """Whether the import or the first follow-up made the item (their logs'
    itempattern inserts)."""
    key = im.dumps({"patternid": pid})
    return any(im.has_table(db, t) and db.execute(f"SELECT 1 FROM main.{t} WHERE tbl = 'itempattern' AND "
                                                  "action = 'insert' AND key = ?", (key,)).fetchone()
               for t in MADE_BY)


def fold_changes(db, f, lists, counts):
    """The doc's 2: the item the workbook's name found folded into the one
    the merged spell's line names, as it is in the database now."""
    x, y = f["found"], f["line"]
    names = dict(db.execute("SELECT patternid, itemname FROM main.itempattern WHERE patternid IN (?, ?)", (x, y)))
    tag = f"{x} into {y}"
    if x not in names or y not in names:
        return []
    if not made_here(db, x):
        lists["an item the workbook's name finds that a merged spell's line names otherwise, one of ours (left)"].append(
            f"{x} {names[x]} (the line's {y} {names[y]})")
        return []
    out = []
    lists["an item the import made folded into the item its spell's line names"].append(
        f"{x} {names[x]} into {y} {names[y]}")
    lore, maxstack = db.execute("SELECT lore, maxstack FROM main.itempattern WHERE patternid = ?", (y,)).fetchone()
    stacks = im.has_table(db, "charinventory") and "stackleft" in im.columns(db, "charinventory")
    for tbl, keys, col in FOLD_TABLES:
        if not im.has_table(db, tbl) or col not in im.columns(db, tbl):
            continue
        for row in db.execute(f"SELECT {', '.join(keys)} FROM main.{tbl} WHERE {col} = ?", (x,)).fetchall():
            key = dict(zip(keys, row))
            out.append(dict(tbl=tbl, action="update", key=key, before={col: x}, set={col: y}, note=tag))
            counts[f"folded: {tbl} rows naming the kept item"] += 1
            if tbl == "charinventory" and stacks:
                sid, stack = db.execute("SELECT serverid, stackleft FROM main.charinventory WHERE itemid = ?",
                                        (key["itemid"],)).fetchone()
                held = db.execute("SELECT count(*) FROM main.charinventory WHERE serverid = ? AND patternid = ?",
                                  (sid, y)).fetchone()[0]
                if stack > maxstack or (lore and held):
                    lists["a copy folded past the kept item's stack or its LORE (as it is)"].append(
                        f"character {sid}: {stack} of {names[x]} as {names[y]} (stacks to {maxstack}, lore {lore}, "
                        f"holds {held} already)")
    for tbl, keys, cols in FOLD_LISTS:
        if not im.has_table(db, tbl):
            continue
        cols = [c for c in cols if c in im.columns(db, tbl)]
        for row in db.execute(f"SELECT {', '.join(keys + cols)} FROM main.{tbl}").fetchall():
            before = {c: v for c, v in zip(cols, row[len(keys):]) if im.token_swap(v, x, y) != (v or "")}
            if before:
                out.append(dict(tbl=tbl, action="update", key=dict(zip(keys, row[:len(keys)])), before=before,
                                set={c: im.token_swap(v, x, y) for c, v in before.items()}, note=tag))
                counts[f"folded: {tbl} rows naming the kept item"] += 1
    for tbl in ("itemeffect", "foodeffect", "itempattern"):
        if im.has_table(db, tbl):
            r = fu.row_of(db, tbl, "patternid = ?", (x,))
            if r is not None:
                out.append(dict(tbl=tbl, action="delete", key={"patternid": x}, before=r, note=tag))
                counts[f"folded: {tbl} rows of the folded item deleted"] += 1
    if im.has_table(db, "defaultcharinventory"):
        n = db.execute("SELECT count(*) FROM main.defaultcharinventory WHERE patternid = ?", (x,)).fetchone()[0]
        if n:
            lists["rows of defaultcharinventory (read by no code) naming a folded item (left)"].append(f"{tag}: {n}")
    return out


def plan(db, wb, smap, disc=None, icons=None, weres=None):
    """Everything --apply would write (changes), and the report's lists."""
    lists, counts = collections.defaultdict(list), collections.Counter()
    pairs = merges(db, smap, lists)
    p = im.plan(db, wb, smap, disc, False, weres or {}, icons)
    wrote = fu.import_writes(db)
    changes = []
    for m in pairs:
        changes += fu.merge_changes(db, m, lists, counts)
    ours = {m["ours"] for m in pairs}
    # the record of ours a merge keeps takes the workbook's
    for e in p["spells"]:
        if e["action"] == "update" and e["spellid"] in ours:
            changes.append(dict(tbl="spellpattern", action="update", key={"spellid": e["spellid"]}, before=e["before"],
                                set=e["set"], note="one spell's two records: the workbook's on ours"))
            counts["records of ours taking a merged record's workbook row"] += 1
    # and its effects row, as the import's log wrote it (else as the first follow-up's rule)
    for e in p["effects"]:
        sid = e["spellid"]
        if sid not in ours or e["action"] != "update":
            continue
        logged = wrote.get(("spelleffect", im.dumps({"spellid": sid})))
        now = e["before"]
        if logged is not None and (now.get("line") != logged.get("line") or
                                   fu.loads(now.get("effects")) != fu.loads(logged.get("effects"))):
            lists["a spell's effects row a GM changed since the import (left)"].append(f"{sid}")
            continue
        cols = ["line", "effects"] + ([] if logged is not None and "fxdict" in logged else ["source"])
        before, after = {k: now.get(k) for k in cols}, {k: e["set"].get(k) for k in cols}
        if before != after:
            changes.append(dict(tbl="spelleffect", action="update", key={"spellid": sid}, before=before, set=after,
                                note="one spell's two records: the workbook's effects on ours"))
            counts["effects rows of ours taking a merged record's workbook effects"] += 1
    # the items: made ones, their rows, the uses made and changed
    made = im.changes_of(dict(spells=[], effects=[], areas=[], moves=[], links=[], scrolls=[], stock=[], migrations=[],
                              gates=[], foods={}, made=p["made"]))
    changes += made
    for c in made:
        counts[f"items: {c['tbl']} rows {c['action']}"] += 1
    folds = []
    for f in p.get("folds") or []:
        out = fold_changes(db, f, lists, counts)
        if out:
            folds.append(f)
        changes += out
    if p.get("made_spells"):
        lists["spells whose items are still none of ours (the first follow-up's to make: left)"] += [
            str(s) for s in p["made_spells"]]
    for k, v in p["lists"].items():
        if k.startswith(("an item of ours", "a use the import wrote", "an item the workbook names that a spell",
                         "a potion made", "a made item", "an item no source gives")):
            lists[k] = v
    return dict(changes=changes, lists={k: v for k, v in lists.items()}, counts=dict(counts), merges=pairs,
                folds=folds)


def refresh(db, p):
    """A plan's merges and folds read again where the database is (--plan F
    --apply on the server): a book, buff, mastery or quest that came to name
    the new record, or a copy, lot or set that came to hold the folded item,
    after the plan was made is taken too, as it is now."""
    lists, counts = collections.defaultdict(list), collections.Counter()
    tags = [f"{m['new']} into {m['ours']}" for m in p.get("merges") or []] + \
        [f"{f['found']} into {f['line']}" for f in p.get("folds") or []]
    keep = [c for c in p["changes"] if not any((c.get("note") or "").startswith(t) for t in tags)]
    fresh = []
    for m in p.get("merges") or []:
        if db.execute("SELECT 1 FROM main.spellpattern WHERE spellid = ?", (m["new"],)).fetchone():
            fresh += fu.merge_changes(db, m, lists, counts)
    for f in p.get("folds") or []:
        fresh += fold_changes(db, f, lists, counts)
    return dict(p, changes=fresh + keep)


def backup(path):
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-eqoaemu-followup2"
    src, out = sqlite3.connect(path, timeout=30), sqlite3.connect(dst)
    try:
        src.backup(out)
    finally:
        out.close()
        src.close()
    return dst


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--sheets", help="a folder of the workbook's sheets as TSV (02-Spells.tsv, 03-Effects.tsv, ...)")
    ap.add_argument("--ods", help="the workbook itself (.ods)")
    ap.add_argument("--map", help="the import's SpellID map (sheet_id, spellid, how), the merged row on ours")
    ap.add_argument("--iso", help="the Frontiers disc: a use's effect icon read the way round the disc has it")
    ap.add_argument("--were-quests", default=im.WERE_QUESTS)
    ap.add_argument("--save-plan", metavar="FILE")
    ap.add_argument("--plan", metavar="FILE")
    ap.add_argument("--list", action="store_true", help="every listed row, not twenty a list")
    ap.add_argument("--no-backup", action="store_true")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    if (a.apply or a.undo) and not a.no_backup:
        print(f"copy: {backup(a.db)}")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        back, left, leftover = im.undo(db, True, LOG, META, WHAT)
        print(f"put back {back}; left {left} changed since" + (f": {', '.join(leftover[:20])}" if leftover else "") +
              "; restart the server")
        return
    if a.plan:
        with open(a.plan, encoding="utf-8") as f:
            p = refresh(db, json.load(f))
    else:
        if not (a.sheets or a.ods) or not a.map:
            sys.exit("--sheets or --ods, and --map (or --plan)")
        disc = icons = None
        if a.iso:
            import fill_spell_visuals_eqoaemu as fsv
            disc, icons = fsv.disc_visuals(a.iso), fsv.disc_icons(a.iso)
        p = plan(db, im.Workbook(im.read_workbook(a.sheets, a.ods)), im.read_map(a.map), disc, icons,
                 im.were_rewards(a.were_quests))
    fu.report(p, a.list)
    if a.save_plan:
        p["made"] = datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")
        with open(a.save_plan, "w", encoding="utf-8") as f:
            json.dump(p, f, indent=1, ensure_ascii=False)
        print(f"plan: {a.save_plan}")
        return
    if a.apply:
        done, stale = im.apply(db, p, LOG, META, WHAT)
        print(f"written {done}; {stale} changed since the plan, left; restart the server")
    else:
        print("(report only: --apply to write it)")


if __name__ == "__main__":
    main()
