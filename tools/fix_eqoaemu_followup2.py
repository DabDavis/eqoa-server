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
     put on ours is two spells: listed, left. The same for a new record the
     import made for a workbook row the map now puts on ours with no rank moved to it
     (the level 60 epic reward quests' options, 2026-11-01, the user: "yes,
     yes check them": the quest's choice gives ours, nothing the new one;
     each pairing checked before its row went in): ours also takes the
     area and class rows a fresh import gives it, and the new record's visual
     where ours has none (the workbook's Graphic, which the import gave the
     record it made; tools/fill_spell_visuals_eqoaemu.py matches spells by
     name, so it never gave ours). And two records of ours that are one spell,
     merged as the user decided (DECIDED_MERGES, 2026-11-01: "yes merge
     them"): the epics' Power Grove into Power Groove and Doomed Tower into
     Doom Tower, the records that carry the workbook's rows; what named the
     folded one names the other (books, kept buffs, masteries, quests: a
     quest step's choices and byclass in their "spells" alone, an item of
     the same number being another record), a scroll of its name folded into
     the scroll of the other's where there is one (Barren Coast Portal's into
     Barrens Coast Portal's: its merchants, copies and quests moved, a
     merchant that sells both keeping one, the kept scroll priced as the
     folded where it had none) and else renamed to the other's, its gate (a
     spellgates row by its name) out where nothing else of that name casts
     it, its class rows, area and record out; the record kept takes nothing
     (it is the workbook's already). Made only while both records are still
     the ones named, else listed (the user, 2026-11-01, of Barren Coast
     Portal: "yes merge it").
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
  4. The user's answers of 2026-11-01 ("yes make it level 60, use workbook
     durations, swap them"): the merged spell's item takes the workbook's
     item's level too (the Transformative Pill, 60); a use of ours the
     workbook gives as invisibility takes the workbook's time (the Ethereal,
     Faulty Ethereal and Aether Potions and the Phase Ring: the Created Item
     Effects sheet's, which the import reads; its words otherwise ours, the
     Volatile Ethereal Potion's speed listed, not said); and the two
     Transmuter ranks whose workbook descriptions are each other's
     (import_eqoaemu_cms.py's DESC_SWAPS) take them so, against the CMs
     sheet's own text (one already so is left; one a GM changed, said).
  5. The workbook's errors, put right as the imports read them now (the user,
     2026-11-01: "yes fix it", "change it"): a Spells row that carries a
     second spell's effects beside its own (import_eqoaemu_spells.py's
     TWO_SPELLS: Flesh Mastery's, its "+" rank's beside its own) takes its own
     alone, and the record of ours of the second's name (Flesh Mastery +) the
     second's row and effects, their lines too; a record of ours whose
     description says other than its row does takes ours (DESCRIPTIONS:
     Flesh Mastery +'s named charisma, Agonize Death's was cut off mid-word);
     the rank that adds the first where it means the second
     (import_eqoaemu_cms.py's RANK_ADDS: Teachings of Morthalis 1) adds the
     second, as the masteries import writes it now. A column of these only
     where it is as the import's log wrote it, or, one the import did not
     write, where no one changed it since the import (gm_audit), else listed;
     a rank's abilities where they are as the masteries import's log wrote
     them. Its owners' books stay as they are (a rank's swap is made at its
     purchase): each is said, one whose book differs from what buying it
     gives now with what it lacks or holds.

A spell's effects row is changed only where it is as the import's log wrote
it (a GM's change since is left, and said); every other row only where it is
still as planned. Report by default. --save-plan FILE writes what --apply
would do and stops (the plan needs the workbook, and with --iso the disc, for
a use's icon as the import reads icons; with --bodies, cmd/modelbake's
models.txt, a body the disc has no CSprite of is listed and not written); --plan FILE applies such a plan where
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

    tools/fix_eqoaemu_followup2.py --sheets <dir of TSVs (06-CMs.tsv too)> | --ods <workbook.ods> --map <spellmap.tsv>
        [--iso <disc.iso>] [--bodies <worlds/models.txt>] [--db eqoa.db] [--save-plan FILE]
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
import import_eqoaemu_cms as cms  # noqa: E402
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
# Two records of ours that are one spell, merged as the user decided: the
# record folded -> (its name, the record of ours kept, that one's name, the
# user's word), made only while both are still the ones named (the doc's 1)
DECIDED_MERGES = {
    700629: ("Power Grove", 701201, "Power Groove", 'the user, 2026-11-01: "yes merge them"'),
    701129: ("Doomed Tower", 700590, "Doom Tower", 'the user, 2026-11-01: "yes merge them"'),
    700662: ("Barren Coast Portal", 901755, "Barrens Coast Portal", 'the user, 2026-11-01: "yes merge it"'),
}


def merges(db, smap, lists, decided=None):
    """The doc's 1: (ours, new, the rank) of each rank the masteries import
    moved from a record of ours to a new one whose workbook row the map puts
    on ours, then (ours, new, None) of each new record the import made for a
    workbook row the map now puts on ours with no rank moved to it, then the
    merges the user decided (decided, DECIDED_MERGES by default: "new" the
    record folded)."""
    out = []
    recs = {r[0]: r[1] for r in db.execute("SELECT spellid, spellname FROM main.spellpattern")}
    ranks = db.execute("SELECT id, op, before, after FROM main.cmcatalog_eqoaemu_log ORDER BY rowid").fetchall() \
        if im.has_table(db, "cmcatalog_eqoaemu_log") else []
    for rid, op, before, after in ranks:
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
    # a new record the import made for a row the map now spells otherwise on ours
    seen = {m["new"] for m in out}
    made_by_import = set()
    if im.has_table(db, im.LOG):
        made_by_import = {json.loads(k).get("spellid") for (k,) in db.execute(
            f"SELECT key FROM main.{im.LOG} WHERE tbl = 'spellpattern' AND action = 'insert'")}
    for hx in sorted(smap):
        m = im.map_entry(smap, hx)
        if m is None or not m[1].startswith("spelled otherwise") or not re.fullmatch(r"[0-9A-F]{1,4}", hx):
            continue
        ours, made = m[0], im.NEW_BASE + int(hx, 16)
        if made in seen or made == ours or made not in recs or ours not in recs:
            continue  # merged by its rank, or once already (gone), or not ours
        tag = f"row {hx}: {made} {recs[made]!r} -> {ours} {recs[ours]!r}"
        if made not in made_by_import:
            lists["a record of a row the map spells otherwise on ours that the import did not make (left)"].append(tag)
            continue
        out.append(dict(ours=ours, new=made, rank=None, ours_name=recs[ours], new_name=recs[made]))
        lists["one spell's two records made one (ours kept: the map's row on it)"].append(tag)
    # two of ours, as the user decided
    for old, (was, to, to_name, word) in sorted((DECIDED_MERGES if decided is None else decided).items()):
        tag = f"{old} {was!r} -> {to} {to_name!r}"
        if old not in recs and recs.get(to) == to_name:
            lists["a merge the user decided, made already (its record gone)"].append(tag)
            continue
        if recs.get(old) != was or recs.get(to) != to_name or old in seen:
            lists["a merge the user decided whose records are not the ones named (left)"].append(
                f"{tag}: {old} is {recs.get(old)!r}, {to} {recs.get(to)!r}")
            continue
        out.append(dict(ours=to, new=old, rank=None, ours_name=to_name, new_name=was, decided=word))
        lists["two records of ours made one, as the user decided (the one with the workbook's row kept)"].append(
            f"{tag} ({word})")
    return out


def json_swap(text, old, new, keys):
    """A quest step's byclass or choices (import_quests.py's compact JSON)
    with old's id swapped for new's under the keys named alone ("spells" for
    a spell, "items" for an item: a record of the other kind may carry the
    same number). None when the text is not that JSON."""
    try:
        j = json.loads(text)
    except ValueError:
        return None
    if json.dumps(j, separators=(",", ":")) != text:
        return None

    def walk(o):
        if isinstance(o, dict):
            return {k: im.token_swap(v, old, new) if k in keys and isinstance(v, str) else walk(v) for k, v in o.items()}
        if isinstance(o, list):
            return [walk(x) for x in o]
        return o
    return json.dumps(walk(j), separators=(",", ":"))


def spells_swap(text, old, new):
    """json_swap of a spell's id: in "spells" alone."""
    return json_swap(text, old, new, ("spells",))


def scroll_fold(db, x, y, tag, lists, counts):
    """A scroll x of a folded record's name folded into the scroll y of the
    kept one's: every copy held, lot, weapon set, loot, start kit, catch and
    quest naming x names y (a quest step's byclass and choices in their
    "items" alone), a merchant that sells both keeping y's row alone, y
    priced as x where it has no price of its own and a merchant sold x
    (else the merchant gives it away), then x's record out."""
    out = []
    sells_y = {n for (n,) in db.execute("SELECT npcid FROM main.npcitem WHERE patternid = ?", (y,))} \
        if im.has_table(db, "npcitem") else set()
    sold_x = False
    for tbl, keys, col in FOLD_TABLES:
        if not im.has_table(db, tbl) or col not in im.columns(db, tbl):
            continue
        for row in db.execute(f"SELECT {', '.join(keys)} FROM main.{tbl} WHERE {col} = ? ORDER BY rowid", (x,)).fetchall():
            key = dict(zip(keys, row))
            if tbl == "npcitem":
                sold_x = True
                r = fu.row_of(db, "npcitem", "itemid = ?", (key["itemid"],))
                if r["npcid"] in sells_y:
                    out.append(dict(tbl="npcitem", action="delete", key=key, before=r,
                                    note=f"{tag}: its merchant sells the kept scroll"))
                    counts["merged: a folded scroll's merchant rows deleted (the merchant sells the kept one)"] += 1
                    continue
                sells_y.add(r["npcid"])
            out.append(dict(tbl=tbl, action="update", key=key, before={col: x}, set={col: y}, note=tag))
            counts[f"merged: a folded scroll's {tbl} rows naming the kept one"] += 1
    for tbl, keys, cols in FOLD_LISTS:
        if not im.has_table(db, tbl):
            continue
        cols = [c for c in cols if c in im.columns(db, tbl)]
        for row in db.execute(f"SELECT {', '.join(keys + cols)} FROM main.{tbl}").fetchall():
            before, after = {}, {}
            for c, v in zip(cols, row[len(keys):]):
                w = json_swap(v, x, y, ("items",)) if c in ("byclass", "choices") and v else im.token_swap(v, x, y)
                if w is None:
                    lists["a quest step's JSON not as the quest importer writes it (left)"].append(
                        f"{dict(zip(keys, row[:len(keys)]))} {c}: {tag}")
                elif w != (v or ""):
                    before[c], after[c] = v, w
            if before:
                out.append(dict(tbl=tbl, action="update", key=dict(zip(keys, row[:len(keys)])), before=before, set=after,
                                note=tag))
                counts[f"merged: a folded scroll's {tbl} rows naming the kept one"] += 1
    cx, cy = (db.execute("SELECT itemcost FROM main.itempattern WHERE patternid = ?", (p,)).fetchone()[0] for p in (x, y))
    if sold_x and not cy and cx:
        out.append(dict(tbl="itempattern", action="update", key={"patternid": y}, before={"itemcost": cy},
                        set={"itemcost": cx}, note=f"{tag}: the price its merchants sold the folded scroll at"))
        lists["a kept scroll of no price given the folded one's (its merchants sold that)"].append(f"{y}: {cx} ({tag})")
    for tbl in ("itemeffect", "foodeffect", "itempattern"):
        if im.has_table(db, tbl):
            r = fu.row_of(db, tbl, "patternid = ?", (x,))
            if r is not None:
                out.append(dict(tbl=tbl, action="delete", key={"patternid": x}, before=r, note=tag))
                counts[f"merged: a folded scroll's {tbl} rows deleted"] += 1
    if im.has_table(db, "defaultcharinventory"):
        n = db.execute("SELECT count(*) FROM main.defaultcharinventory WHERE patternid = ?", (x,)).fetchone()[0]
        if n:
            lists["rows of defaultcharinventory (read by no code) naming a folded scroll (left)"].append(f"{tag}: {n}")
    return out


def merge_changes(db, m, lists, counts):
    """The first follow-up's merge_changes, a quest step's byclass and
    choices swapped in their "spells" alone (spells_swap), and for a merge
    the user decided a scroll of the folded record's name renamed to the
    kept one's (its own scrolls' rule: none of that name there already)."""
    out = []
    for c in fu.merge_changes(db, m, lists, counts):
        if c["tbl"] == "queststeps" and c["action"] == "update":
            after = dict(c["set"])
            for col in ("byclass", "choices"):
                if col in c["before"]:
                    v = spells_swap(c["before"][col], m["new"], m["ours"])
                    if v is None:
                        lists["a quest step's JSON not as the quest importer writes it (left)"].append(
                            f"{c['key']} {col}: {m['new']} into {m['ours']}")
                        v = c["before"][col]
                    after[col] = v
            changed = {k: v for k, v in after.items() if v != c["before"].get(k)}
            if not changed:
                counts["merged: queststeps rows naming ours"] -= 1
                continue
            c = dict(c, before={k: c["before"][k] for k in changed}, set=changed)
        out.append(c)
    if m.get("decided"):
        tag = f"{m['new']} into {m['ours']}: its scroll"
        scrolls = db.execute("SELECT patternid, itemname FROM main.itempattern WHERE itemicon = ? AND equipslot = -1 "
                             "ORDER BY patternid", (im.SCROLL_ICON,)).fetchall()
        kept = [pid for pid, name in scrolls if im.exact_key(name) == im.exact_key(m["ours_name"])]
        for pid, name in scrolls:
            if im.exact_key(name) != im.exact_key(m["new_name"]):
                continue
            if len(kept) == 1:
                out += scroll_fold(db, pid, kept[0], f"{tag} {pid} into {kept[0]}", lists, counts)
                counts["merged: scrolls of the folded record's name folded into the kept one's"] += 1
            elif not kept:
                out.append(dict(tbl="itempattern", action="update", key={"patternid": pid}, before={"itemname": name},
                                set={"itemname": m["ours_name"]}, note=tag))
                counts["merged: scrolls of the folded record's name renamed"] += 1
            else:
                lists["a scroll of a folded record's name, several of the kept one's (left: the user)"].append(
                    f"{pid} {name!r}: {kept}")
        # its gate, by its name: out where nothing else of that name casts one
        key = m["new_name"].strip().lower()
        gate = fu.row_of(db, "spellgates", "spell = ?", (key,)) if im.has_table(db, "spellgates") else None
        if gate is not None and key != m["ours_name"].strip().lower():
            others = db.execute("SELECT count(*) FROM main.spellpattern WHERE lower(trim(spellname)) = ? AND spellid != ?",
                                (key, m["new"])).fetchone()[0]
            uses = db.execute("SELECT count(*) FROM main.itempattern p JOIN main.itemeffect e ON e.patternid = p.patternid "
                              "WHERE lower(trim(p.itemname)) = ?", (key,)).fetchone()[0] \
                if im.has_table(db, "itemeffect") else 0
            if others or uses:
                lists["a folded record's gate another of its name casts (left)"].append(f"{key}: {others} spells, {uses} uses")
            else:
                out.append(dict(tbl="spellgates", action="delete", key={"spell": key}, before=gate,
                                note=f"{m['new']} into {m['ours']}: its gate (nothing else of its name casts it)"))
                counts["merged: a folded record's gate deleted"] += 1
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


def desc_changes(db, ranks, lists, counts):
    """The doc's 4, the descriptions: each of DESC_SWAPS' ranks of ours
    (cmcatalog, by name) whose description is the CMs sheet's own for it
    (ranks: read_ranks, unswapped) takes the other's."""
    out = []
    if not ranks or not im.has_table(db, "cmcatalog") or "description" not in im.columns(db, "cmcatalog"):
        return out
    text = {}
    for w in ranks:
        text.setdefault(cms.fold(w["name"]), w["desc"])
    rows = db.execute("SELECT id, name, description FROM main.cmcatalog ORDER BY id").fetchall()
    for a, b in cms.DESC_SWAPS:
        if a not in text or b not in text:
            continue
        for mine, other in ((a, b), (b, a)):
            for rid, name, desc in rows:
                if cms.fold(name) != mine or desc == text[other]:
                    continue
                if desc != text[mine]:
                    lists["a rank's description a GM changed since the import (left)"].append(f"{rid} {name}")
                    continue
                out.append(dict(tbl="cmcatalog", action="update", key={"id": rid}, before={"description": desc},
                                set={"description": text[other]}, note="a pair of ranks' workbook descriptions, swapped"))
                counts["ranks given the other's workbook description"] += 1
                lists["a rank given the other's workbook description"].append(f"{rid} {name}: {text[other]!r}")
    return out


def when(text):
    """A logged time (ISO, Z or an offset; none: UTC), None when it is none."""
    try:
        t = datetime.datetime.fromisoformat((text or "").strip().replace(" ", "T"))
    except ValueError:
        return None
    return t if t.tzinfo else t.replace(tzinfo=datetime.timezone.utc)


def import_applied(db):
    """When the spells import was applied (its meta), None when it was not."""
    if not im.has_table(db, im.META):
        return None
    r = db.execute(f"SELECT value FROM main.{im.META} WHERE key = 'applied_at'").fetchone()
    return when(r[0]) if r else None


def changed_since(db, tbl, key, cols, since):
    """Whether gm_audit (the GM panel's lines, and a tool's that writes
    there) changes any of a row's columns at or after since."""
    if since is None or not im.has_table(db, "gm_audit"):
        return False
    for at, before, after in db.execute("SELECT at, before, after FROM main.gm_audit WHERE tbl = ? AND rowkey = ?",
                                        (tbl, im.dumps(key))):
        t = when(at)
        if t is None or t < since:
            continue
        b, a = fu.loads(before), fu.loads(after)
        b, a = b if isinstance(b, dict) else {}, a if isinstance(a, dict) else {}
        if any(b.get(c) != a.get(c) for c in cols):
            return True
    return False


def touched(db, tbl, sid, cols, now, wrote, since):
    """The doc's 5 rule for an erratum's columns of a row (now: as they are):
    each the import wrote is as its log has it, and the rest no one changed
    since the import (gm_audit). Returns the columns that are not so."""
    logged = wrote.get((tbl, im.dumps({"spellid": sid}))) or {}
    off = []
    for c in cols:
        if c in logged and (fu.loads(now.get(c)) != fu.loads(logged[c]) if c == "effects" else now.get(c) != logged[c]):
            off.append(c)
    rest = [c for c in cols if c not in logged]
    if rest and changed_since(db, tbl, {"spellid": sid}, rest, since):
        off += rest
    return off


def errata_changes(db, p, ours, wrote, lists, counts):
    """The doc's 5, the spells: what the import's plan (p) writes on the
    records the workbook's errata touch (p["errata"]: a row TWO_SPELLS cut
    takes its own effects, the second spell's record of ours its row's
    record and effects, a record DESCRIPTIONS names its description), each
    column where touched says it may; a merged record's (ours) columns are
    the merge's, its description guarded too (guard_merged_descriptions)."""
    er = p.get("errata") or {}
    first, second, described = set(er.get("first", [])), set(er.get("second", [])), set(er.get("description", []))
    since = import_applied(db)
    out = []
    for e in p["spells"]:
        sid = e["spellid"]
        if e["action"] != "update" or sid in ours or not (sid in second or sid in described):
            continue
        cols = [c for c in e["set"] if sid in second or c == "spelldesc"]
        off = touched(db, "spellpattern", sid, cols, e["before"], wrote, since)
        if off:
            lists["a workbook erratum's record changed since the import (left)"].append(f"{sid}: {', '.join(off)}")
        cols = [c for c in cols if c not in off]
        if cols:
            out.append(dict(tbl="spellpattern", action="update", key={"spellid": sid},
                            before={c: e["before"][c] for c in cols}, set={c: e["set"][c] for c in cols},
                            note="the workbook's errata: " + ("the second spell's record" if sid in second else
                                                              "a description of ours put right")))
            counts["errata: records put right"] += 1
    for e in p["effects"]:
        sid = e["spellid"]
        if e["action"] != "update" or sid in ours or sid not in first | second:
            continue
        logged = wrote.get(("spelleffect", im.dumps({"spellid": sid})))
        cols = ["line", "effects"] + ([] if logged is not None and "fxdict" in logged else ["source"])
        now = e["before"]
        before, after = {k: now.get(k) for k in cols}, {k: e["set"].get(k) for k in cols}
        if before == after:
            continue
        off = touched(db, "spelleffect", sid, cols, now, wrote, since)
        if off:
            lists["a workbook erratum's effects row changed since the import (left)"].append(f"{sid}: {', '.join(off)}")
            continue
        out.append(dict(tbl="spelleffect", action="update", key={"spellid": sid}, before=before, set=after,
                        note="the workbook's errata: " + ("the row's own effects" if sid in first else
                                                          "the second spell's effects")))
        counts["errata: effects rows put right"] += 1
    return out


def guard_merged_descriptions(db, p, changes, wrote, lists):
    """A merged record's description put right (DESCRIPTIONS) is the doc's
    5's too: dropped from its update where touched says it may not be."""
    described = set((p.get("errata") or {}).get("description", []))
    since = import_applied(db)
    for c in changes:
        sid = (c.get("key") or {}).get("spellid")
        if c["tbl"] != "spellpattern" or c["action"] != "update" or sid not in described or "spelldesc" not in c["set"]:
            continue
        if touched(db, "spellpattern", sid, ["spelldesc"], c["before"], wrote, since):
            lists["a workbook erratum's record changed since the import (left)"].append(f"{sid}: spelldesc")
            c["before"].pop("spelldesc")
            c["set"].pop("spelldesc")
    return [c for c in changes if c["tbl"] != "spellpattern" or c["action"] != "update" or c["set"]]


def rank_changes(db, ranks, smap, lists, counts):
    """The doc's 5, the masteries: each RANK_ADDS rank of ours (cmcatalog,
    by name) adds what the masteries import's rules write for it now (its
    abilities of the CMs sheet put right, import_eqoaemu_cms.fix_rank_adds,
    and named as its SpellNames names them), where its abilities are as that
    import's log wrote them (else listed); its owners' books stay (a rank's
    swap is made at its purchase), each said."""
    out = []
    W = [dict(w) for w in ranks or [] if "adds" in w and "hex" in w]
    if not W or not im.has_table(db, "cmcatalog") or not {"adds", "add_spells"} <= set(im.columns(db, "cmcatalog")):
        return out
    fixed = cms.fix_rank_adds(W)
    if not fixed:
        return out
    ctx = cms.SpellNames(db, {int(k, 16): (v[0], v[1]) for k, v in smap.items() if re.fullmatch(r"[0-9A-F]{1,4}", k)},
                         {})
    logged = {}
    if im.has_table(db, cms.LOG):
        logged = {rid: (fu.loads(a) or {}, when(t))
                  for rid, a, t in db.execute(f"SELECT id, after, applied FROM main.{cms.LOG}")}
    rows = db.execute("SELECT id, name, adds, add_spells, removes, remove_spells FROM main.cmcatalog ORDER BY id").fetchall()
    for hx, name, was, means in fixed:
        w = next(x for x in W if x["hex"] == hx)
        adds, add_spells = ctx.abilities(w["adds"])
        if ctx.unmapped:
            lists["a rank put right whose spell none of ours, or several, are (left)"].append(
                f"{hx} {name}: {', '.join(sorted(ctx.unmapped))}")
            ctx.unmapped.clear()
            continue
        for rid, nm, a, s, rm, rms in rows:
            if cms.fold(nm) != cms.fold(name):
                continue
            if (a, s) == (adds, add_spells):
                continue
            log, at = logged.get(rid) or ({}, None)
            if log.get("adds", a) != a or log.get("add_spells", s) != s or \
                    ("adds" not in log and changed_since(db, "cmcatalog", {"id": rid}, ["adds", "add_spells"], at)):
                lists["a rank put right that a GM changed since the masteries import (left)"].append(f"{rid} {nm}")
                continue
            out.append(dict(tbl="cmcatalog", action="update", key={"id": rid}, before={"adds": a, "add_spells": s},
                            set={"adds": adds, "add_spells": add_spells},
                            note=f"the workbook's errata: {nm} adds {means}, not {was}"))
            counts["errata: ranks put right"] += 1
            lists["a rank given the spell it means (RANK_ADDS)"].append(f"{rid} {nm}: {a!r} -> {adds!r} ({add_spells})")
            gives, takes = cms.book_change(dict(adds=adds, add_spells=add_spells, removes=rm, remove_spells=rms))
            if im.has_table(db, "charcms") and im.has_table(db, "spells"):
                for (sid,) in db.execute("SELECT serverid FROM main.charcms WHERE cmid = ? ORDER BY serverid", (rid,)):
                    held = {x for (x,) in db.execute("SELECT spellid FROM main.spells WHERE serverid = ?", (sid,))}
                    lack = [g for g in gives if g[1] not in held]
                    keep = [t for t in takes if t[1] in held]
                    lists["an owner of a rank put right (its book as it is: a rank's swap is made at its purchase)"].append(
                        f"character {sid}, {rid} {nm}: " + ("its book as buying it gives now" if not lack and not keep else
                                                            f"lacks {lack}, holds {keep}"))
    return out


def plan(db, wb, smap, disc=None, icons=None, weres=None, bodies=None, ranks=None, decided=None):
    """Everything --apply would write (changes), and the report's lists.
    ranks: the CMs sheet's ranks (import_eqoaemu_cms.read_ranks), for the
    descriptions; decided the merges the user decided (DECIDED_MERGES)."""
    lists, counts = collections.defaultdict(list), collections.Counter()
    pairs = merges(db, smap, lists, decided)
    p = im.plan(db, wb, smap, disc, False, weres or {}, icons, bodies)
    wrote = fu.import_writes(db)
    changes = []
    for m in pairs:
        changes += merge_changes(db, m, lists, counts)
    # the record of ours a merge keeps takes the workbook's (one of a merge
    # the user decided is the workbook's already)
    ours = {m["ours"] for m in pairs if not m.get("decided")}
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
    # the merged record's visual on ours where ours has none
    for m in pairs:
        if m.get("decided"):
            continue
        a = db.execute("SELECT fxdict FROM main.spelleffect WHERE spellid = ?", (m["ours"],)).fetchone()
        b = db.execute("SELECT fxdict FROM main.spelleffect WHERE spellid = ?", (m["new"],)).fetchone()
        if a is None or b is None or not b[0] or a[0] == b[0]:
            continue
        if a[0]:
            lists["a merged record's visual not ours (ours kept)"].append(f"{m['new']} into {m['ours']}: {b[0]}, ours {a[0]}")
            continue
        c = next((c for c in changes if c["tbl"] == "spelleffect" and c["action"] == "update"
                  and c["key"] == {"spellid": m["ours"]}), None)
        if c is None:
            changes.append(dict(tbl="spelleffect", action="update", key={"spellid": m["ours"]}, before={"fxdict": a[0]},
                                set={"fxdict": b[0]}, note="one spell's two records: the merged record's visual on ours"))
        else:
            c["before"]["fxdict"], c["set"]["fxdict"] = a[0], b[0]
        counts["effects rows of ours taking a merged record's visual"] += 1
    # and the area and class rows a fresh import gives ours
    rows = im.changes_of(dict(spells=[], effects=[], areas=[e for e in p["areas"] if e["spellid"] in ours], moves=[],
                              links=[x for x in p["links"] if x["spellid"] in ours], scrolls=[], stock=[], migrations=[],
                              gates=[], foods={}, made={}))
    for c in rows:
        c["note"] = "one spell's two records: the workbook's area or class row on ours"
        counts[f"merged: {c['tbl']} rows of ours ({c['action']})"] += 1
    changes += rows
    # the workbook's errata (the doc's 5): the records they touch, the rank
    changes = guard_merged_descriptions(db, p, changes, wrote, lists)
    changes += errata_changes(db, p, ours, wrote, lists, counts)
    changes += rank_changes(db, ranks, smap, lists, counts)
    for k, v in p["lists"].items():
        if k.endswith(("(TWO_SPELLS)", "(DESCRIPTIONS)")) or k.startswith("a workbook erratum's"):
            lists[k] = v
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
    changes += desc_changes(db, ranks, lists, counts)
    if p.get("made_spells"):
        lists["spells whose items are still none of ours (the first follow-up's to make: left)"] += [
            str(s) for s in p["made_spells"]]
    for k, v in p["lists"].items():
        if k.startswith(("an item of ours", "a use the import wrote", "an item the workbook names that a spell",
                         "a potion made", "a made item", "an item no source gives", "a use's body",
                         "an invisibility of ours")):
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
            fresh += merge_changes(db, m, lists, counts)
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
    ap.add_argument("--bodies", help="cmd/modelbake's models.txt (the disc's CSprites): a use's body the disc lacks "
                                     "listed and not written")
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
        cm_sheet = cms.Sheet(cms.ods_table(a.ods, "CMs")) if a.ods else \
            cms.Sheet(cms.tsv_table(os.path.join(a.sheets, "06-CMs.tsv")))
        p = plan(db, im.Workbook(im.read_workbook(a.sheets, a.ods)), im.read_map(a.map), disc, icons,
                 im.were_rewards(a.were_quests), im.read_bodies(a.bodies) if a.bodies else None, cms.read_ranks(cm_sheet))
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
