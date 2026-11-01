#!/usr/bin/env python3
"""The EQOAEmu import's follow-ups (the user, 2026-11-01, after its apply),
on a database that import has been applied to: what
tools/import_eqoaemu_spells.py's rules now make that they did not then, so the
database ends as a fresh import would leave it (its plan made again here, on
this database, and only these parts of it taken):

  1. The items its spells make that were none of ours (the server summons an
     item by its name, so those spells made nothing; the user: "lets fix
     these"): the import's MadeItems, each item made (itempattern and its
     itemeffect or foodeffect row) or an item of ours with no use given the
     workbook's, and each spell's effects row (its JSON and line) naming its
     item, a new spell's description too.
  2. A heal of more than a minute heals once, at its landing
     (HEAL_TICKS_UP_TO; the user's Avatar, "a one time max hp buff and massive
     stat buff lasts until next avatar recast"): each spell's effects row
     whose workbook row is such a heal.
  3. One spell under two spellings made two records: a mastery rank the
     masteries import moved from a spell of ours to a new one (its log,
     cmcatalog_eqoaemu_log), the two of one level and class, both making an
     item, their names two letters apart at most (the Transmuter's third
     rank: the 2004 records' and EQOA-Data's spell list's "Transmutive
     Mixture", ours, and the masteries lists' "Transmutative Mixture", the
     workbook's, made new). Ours stays, as a spell of ours the import's map
     names: its record takes the workbook's (level, range, cast, recast,
     power, scope, icon, frame; its name kept: import_eqoaemu_spells.py's
     rename_safe), its effects the workbook's, its item ours by its line;
     the new record goes (its record, effects, class rows and area; a book
     holding both keeps ours, one holding the new alone takes ours; the
     masteries, quests and kept buffs naming it name ours). A rank moved to
     a spell of another name is listed and left (two spells, or the user's
     call). A fresh import makes the same with the map's row on ours.
  4. A worn item whose own description says NO RENT and whose record does not
     is made NO RENT (rent 0, CLIENT's m_rent: the Phase Staff), as every
     conjured weapon of ours is; the balance kits leave NO RENT items out
     (entity/balancekit.go simPick).

A spell's effects row or description is changed only where it is as the
import's log wrote it (a GM's change since is left, and said); the rest only
where the row is still as planned. Report by default. --save-plan FILE writes
what --apply would do and stops (the plan needs the workbook, and with --iso
the disc, for a made potion's effect icon as the import reads icons);
--plan FILE applies such a plan where the database is. --apply writes it all
in one transaction, every row logged in eqoaemu_followup_log by the import's
own apply (its key, the columns before and after, a deleted row's rowid);
--undo puts each logged row back as it was (one changed since is left, and
said), refused while a character holds an item it made unless --force. Both
take a copy of the database first (SQLite's backup API). The server loads the
potions' uses, the masteries and the books at its start: restart it after.

    tools/fix_eqoaemu_followup.py --sheets <dir of TSVs> | --ods <workbook.ods> --map <spellmap.tsv>
        [--iso <disc.iso>] [--db eqoa.db] [--save-plan FILE]
    tools/fix_eqoaemu_followup.py --db eqoa.db --plan FILE --apply
    tools/fix_eqoaemu_followup.py --db eqoa.db --undo [--force]
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

DEFAULT_DB = os.path.join(HERE, "..", "eqoa.db")
LOG = "eqoaemu_followup_log"
META = "eqoaemu_followup_meta"
WHAT = "the EQOAEmu import's follow-up"


def loads(text):
    try:
        return json.loads(text) if (text or "").strip() else None
    except ValueError:
        return text


def import_writes(db):
    """What the import's log says it last wrote to each row, by (table, the
    row's key as logged): the columns, an insert's whole row."""
    out = {}
    if not im.has_table(db, im.LOG):
        return out
    for tbl, key, after in db.execute(f"SELECT tbl, key, after FROM {im.LOG} ORDER BY seq"):
        if after is not None:
            out.setdefault((tbl, key), {}).update(json.loads(after))
    return out


def edits(a, b):
    """Letters put in, taken out or changed between two names."""
    prev = list(range(len(b) + 1))
    for i, ca in enumerate(a, 1):
        cur = [i]
        for j, cb in enumerate(b, 1):
            cur.append(min(prev[j] + 1, cur[j - 1] + 1, prev[j - 1] + (ca != cb)))
        prev = cur
    return prev[-1]


def makes_item(line, effects):
    """Whether a spell's line or effects summon an item."""
    if im.line_items(line or "", "item"):
        return True
    j = loads(effects)
    return isinstance(j, dict) and any(o.get("kind") == "create_item" for o in im.walk_effects(j.get("effects") or []))


def merges(db, lists):
    """The doc's 3: (ours, new, the rank) of each pair of one spell's two
    records; the ranks moved to a new spell of another name, listed."""
    out = []
    if not im.has_table(db, "cmcatalog_eqoaemu_log"):
        return out
    recs = {r[0]: r for r in db.execute("SELECT spellid, abilitylvl, spellname FROM main.spellpattern")}
    fx = {r[0]: (r[1], r[2]) for r in db.execute("SELECT spellid, line, effects FROM main.spelleffect")}
    # the new records this tool's --apply took out already: the map's rows on ours still
    gone = set()
    if im.has_table(db, LOG):
        for (key,) in db.execute(f"SELECT key FROM main.{LOG} WHERE tbl = 'spellpattern' AND action = 'delete'"):
            gone.add(json.loads(key)["spellid"])
    classes = collections.defaultdict(set)
    for c, sid in db.execute("SELECT class, spellid FROM main.classspells"):
        classes[sid].add(c)
    for rid, op, before, after in db.execute("SELECT id, op, before, after FROM main.cmcatalog_eqoaemu_log ORDER BY rowid"):
        b, a = loads(before) or {}, loads(after) or {}
        if op != "update" or "add_spells" not in b or "add_spells" not in a:
            continue
        old, new = [int(x) for x in re.findall(r"\d+", b["add_spells"])], [int(x) for x in re.findall(r"\d+", a["add_spells"])]
        if len(old) != 1 or len(new) != 1 or old[0] >= im.NEW_BASE or new[0] < im.NEW_BASE:
            continue
        ours, made = old[0], new[0]
        if made in gone and ours in recs:
            out.append(dict(ours=ours, new=made, rank=rid, ours_name=recs[ours][2], new_name="", done=True))
            continue
        if ours not in recs or made not in recs or recs[ours][1] != recs[made][1] or not classes[ours] & classes[made]:
            continue
        if not makes_item(*fx.get(ours, ("", ""))) or not makes_item(*fx.get(made, ("", ""))):
            continue
        tag = f"rank {rid}: {ours} {recs[ours][2]!r} -> {made} {recs[made][2]!r}"
        if edits(im.fold(recs[ours][2]), im.fold(recs[made][2])) > 2:
            lists["a mastery rank the import moved to a new spell of another name (left: two spells, or the user's call)"].append(tag)
            continue
        out.append(dict(ours=ours, new=made, rank=rid, ours_name=recs[ours][2], new_name=recs[made][2]))
        lists["one spell's two records made one (ours kept)"].append(tag)
    return out


def row_of(db, tbl, where, args):
    cols = im.columns(db, tbl)
    r = db.execute(f"SELECT {', '.join(cols)} FROM main.{tbl} WHERE {where}", args).fetchone()
    return None if r is None else dict(zip(cols, r))


def merge_changes(db, m, lists, counts):
    """The new record taken out, what named it naming ours."""
    ours, new = m["ours"], m["new"]
    out = []
    tag = f"{new} into {ours}"

    def delete(tbl, key, where, args):
        r = row_of(db, tbl, where, args)
        if r is not None:
            out.append(dict(tbl=tbl, action="delete", key=key, before=r, note=tag))
            counts[f"merged: {tbl} rows deleted"] += 1

    def update(tbl, key, before, after):
        if before != after:
            out.append(dict(tbl=tbl, action="update", key=key, before=before, set=after, note=tag))
            counts[f"merged: {tbl} rows naming ours"] += 1
    holds = {sid for (sid,) in db.execute("SELECT serverid FROM main.spells WHERE spellid = ?", (ours,))}
    for (rid,) in db.execute("SELECT id FROM main.spells WHERE spellid = ? ORDER BY id", (new,)).fetchall():
        r = row_of(db, "spells", "id = ?", (rid,))
        if r["serverid"] in holds:
            out.append(dict(tbl="spells", action="delete", key={"id": rid}, before=r, note=tag + ": the book holds ours"))
            counts["merged: books holding both (the new one's row deleted)"] += 1
        else:
            update("spells", {"id": rid}, {"spellid": new}, {"spellid": ours})
    if im.has_table(db, "charbuffs"):
        for sid, seq in db.execute("SELECT serverid, seq FROM main.charbuffs WHERE spellid = ?", (new,)).fetchall():
            update("charbuffs", {"serverid": sid, "seq": seq}, {"spellid": new}, {"spellid": ours})
    if im.has_table(db, "cmcatalog"):
        cols = [c for c in ("add_spells", "require_spells", "remove_spells") if c in im.columns(db, "cmcatalog")]
        for row in db.execute(f"SELECT id, {', '.join(cols)} FROM main.cmcatalog").fetchall():
            before = {c: v for c, v in zip(cols, row[1:]) if im.token_swap(v, new, ours) != (v or "")}
            update("cmcatalog", {"id": row[0]}, before, {c: im.token_swap(v, new, ours) for c, v in before.items()})
    if im.has_table(db, "queststeps"):
        cols = [c for c in ("spells", "byclass", "choices") if c in im.columns(db, "queststeps")]
        for row in db.execute(f"SELECT questid, step, {', '.join(cols)} FROM main.queststeps").fetchall():
            before = {c: v for c, v in zip(cols, row[2:]) if im.token_swap(v, new, ours) != (v or "")}
            update("queststeps", {"questid": row[0], "step": row[1]}, before,
                   {c: im.token_swap(v, new, ours) for c, v in before.items()})
    for c, sid in db.execute("SELECT class, spellid FROM main.classspells WHERE spellid = ?", (new,)).fetchall():
        delete("classspells", {"class": c, "spellid": sid}, "class = ? AND spellid = ?", (c, sid))
    if im.has_table(db, "spellarea"):
        delete("spellarea", {"spellid": new}, "spellid = ?", (new,))
    # a scroll of the new record's, and its merchants' rows
    scroll = row_of(db, "itempattern", "patternid = ? AND itemicon = ? AND equipslot = -1", (new, im.SCROLL_ICON))
    if scroll is not None:
        for (iid,) in db.execute("SELECT itemid FROM main.npcitem WHERE patternid = ?", (new,)).fetchall():
            delete("npcitem", {"itemid": iid}, "itemid = ?", (iid,))
        delete("itempattern", {"patternid": new}, "patternid = ?", (new,))
    delete("spelleffect", {"spellid": new}, "spellid = ?", (new,))
    delete("spellpattern", {"spellid": new}, "spellid = ?", (new,))
    for tbl in ("defaultspells", "spellfx_suggestions"):
        if im.has_table(db, tbl) and "spellid" in im.columns(db, tbl):
            n = db.execute(f"SELECT count(*) FROM main.{tbl} WHERE spellid = ?", (new,)).fetchone()[0]
            if n:
                lists[f"rows of {tbl} naming a merged record (left)"].append(f"{tag}: {n}")
    return out


def long_heals(wb, smap):
    """The spells whose workbook rows (a proc's too) are a positive Dmg/Heal
    of more than a minute."""
    rows = {i for i, e in wb.effects.items() if im.cell(e, "TYPE") == "Dmg/Heal" and (im.num(e, "HP1") or 0) > 0
            and (im.num(e, "Duration") or 0) > im.HEAL_TICKS_UP_TO}
    out = set()
    for s in wb.spells:
        ids = set(im.effect_ids(s))
        for i in list(ids):
            e = wb.effects.get(i) or {}
            ids |= {im.hexid(e.get(k)) for k in ("ApplyProc1", "ApplyProc2")} - {""}
        m = im.map_entry(smap, im.hexid(s.get("SpellID")))
        if m and ids & rows:
            out.add(m[0])
    return out


def no_rent(db, counts, lists):
    """The doc's 4."""
    out = []
    for pid, name, desc, rent in db.execute("SELECT patternid, itemname, itemdesc, rent FROM main.itempattern "
                                            "WHERE equipslot >= 0 AND rent <> 0 AND itemdesc LIKE '%NO RENT%' "
                                            "ORDER BY patternid").fetchall():
        if "NO RENT" not in (desc or ""):
            continue  # LIKE is blind to case; the words are the record's own
        out.append(dict(tbl="itempattern", action="update", key={"patternid": pid}, before={"rent": rent},
                        set={"rent": 0}, note=f"{name}: its description says NO RENT"))
        counts["items made NO RENT, as their descriptions say"] += 1
        lists["an item made NO RENT, as its description says"].append(f"{pid} {name}")
    return out


def plan(db, wb, smap, disc=None, icons=None, weres=None):
    """Everything --apply would write (changes), and the report's lists."""
    lists, counts = collections.defaultdict(list), collections.Counter()
    smap = dict(smap)
    pairs = merges(db, lists)
    for m in pairs:
        # the map's row of the new record on ours, as a fresh import's map has it
        for hx, (sid, *_rest) in list(smap.items()):
            if sid == m["new"]:
                smap[hx] = (m["ours"], "spelled otherwise", None)
    p = im.plan(db, wb, smap, disc, False, weres or {}, icons)
    wrote = import_writes(db)
    pairs = [m for m in pairs if not m.get("done")]
    touched = set(p.get("made_spells") or []) | long_heals(wb, smap) | {m["ours"] for m in pairs}
    changes = []
    for m in pairs:
        changes += merge_changes(db, m, lists, counts)
    # the record of ours a merge keeps takes the workbook's
    for e in p["spells"]:
        if e["action"] == "update" and e["spellid"] in {m["ours"] for m in pairs}:
            changes.append(dict(tbl="spellpattern", action="update", key={"spellid": e["spellid"]}, before=e["before"],
                                set=e["set"], note="one spell's two records: the workbook's on ours"))
            counts["records of ours taking a merged record's workbook row"] += 1
    # the spells' effects rows, as the import's log wrote them
    effect_after = {}
    for e in p["effects"]:
        sid = e["spellid"]
        if sid not in touched or e["action"] != "update":
            continue
        logged = wrote.get(("spelleffect", im.dumps({"spellid": sid})))
        now = e["before"]
        if logged is not None and (now.get("line") != logged.get("line") or
                                   loads(now.get("effects")) != loads(logged.get("effects"))):
            lists["a spell's effects row a GM changed since the import (left)"].append(f"{sid}")
            continue
        # the line and effects; a row the import made keeps its words for
        # where it came from (a plan made again calls it a spell of ours)
        cols = ["line", "effects"] + ([] if logged is not None and "fxdict" in logged else ["source"])
        before, after = {k: now.get(k) for k in cols}, {k: e["set"].get(k) for k in cols}
        if before == after:
            continue
        changes.append(dict(tbl="spelleffect", action="update", key={"spellid": sid}, before=before, set=after,
                            note="the follow-up's items and heals"))
        effect_after[sid] = after
        counts["effects rows written"] += 1
    # a new spell's description, naming its item
    records = {r[0]: r for r in db.execute("SELECT spellid, spellname, spelldesc FROM main.spellpattern")}
    by_sid = {}
    for s in wb.spells:
        m = im.map_entry(smap, im.hexid(s.get("SpellID")))
        if m:
            by_sid.setdefault(m[0], s)
    for sid, after in sorted(effect_after.items()):
        logged = wrote.get(("spellpattern", im.dumps({"spellid": sid})))
        s, rec = by_sid.get(sid), records.get(sid)
        if logged is None or s is None or rec is None or "spelldesc" not in logged:
            continue
        if rec[2] != logged["spelldesc"]:
            lists["a new spell's description a GM changed since the import (left)"].append(f"{sid} {rec[1]}")
            continue
        j = json.loads(after["effects"])
        quest = (weres or {}).get(im.quest_fold(im.cell(s, "Name")))
        were_form = any(o["kind"] == "illusion" for o in j["effects"]) and len(set(im.class_mask(im.cell(s, "CLASS")))) == len(im.CLASSES)
        desc = im.desc_of(j, im.cell(s, "Learn Method") == "CM", rec[1], quest[1] if quest else "", were_form)
        if desc != rec[2]:
            changes.append(dict(tbl="spellpattern", action="update", key={"spellid": sid}, before={"spelldesc": rec[2]},
                                set={"spelldesc": desc}, note="a new spell's description naming its item"))
            counts["new spells' descriptions naming their items"] += 1
    made = im.changes_of(dict(spells=[], effects=[], areas=[], moves=[], links=[], scrolls=[], stock=[], migrations=[],
                              gates=[], foods={}, made=p["made"]))
    changes += made
    changes += no_rent(db, counts, lists)
    for k, v in p["lists"].items():
        if k.startswith(("a made item", "a potion made", "an item of ours with no use", "an item no source gives",
                         "an item none of ours")):
            lists[k] = v
    for k, v in p["counts"].items():
        if k.startswith("made items") or k.startswith("items of ours"):
            counts[k] = v
    return dict(changes=changes, lists={k: v for k, v in lists.items()}, counts=dict(counts),
                merges=pairs, touched=sorted(touched))


def report(p, full=False):
    for k, v in sorted(p["counts"].items()):
        print(f"  {k}: {v}")
    print(f"  changes: {len(p['changes'])} ({', '.join(f'{t} {n}' for t, n in sorted(collections.Counter(c['tbl'] + ' ' + c['action'] for c in p['changes']).items()))})")
    for k, v in p["lists"].items():
        print(f"  {k}: {len(v)}")
        for x in (v if full else v[:20]):
            print(f"      {x}")
        if not full and len(v) > 20:
            print("      ...")


def backup(path):
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-eqoaemu-followup"
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
    ap.add_argument("--map", help="the import's SpellID map (sheet_id, spellid, how)")
    ap.add_argument("--iso", help="the Frontiers disc: a made potion's effect icon read the way round the disc has it")
    ap.add_argument("--were-quests", default=im.WERE_QUESTS)
    ap.add_argument("--save-plan", metavar="FILE")
    ap.add_argument("--plan", metavar="FILE")
    ap.add_argument("--list", action="store_true", help="every listed row, not twenty a list")
    ap.add_argument("--no-backup", action="store_true")
    ap.add_argument("--force", action="store_true", help="--undo even when characters hold an item it made")
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
        back, left, leftover = im.undo(db, a.force, LOG, META, WHAT)
        print(f"put back {back}; left {left} changed since" + (f": {', '.join(leftover[:20])}" if leftover else ""))
        return
    if a.plan:
        with open(a.plan, encoding="utf-8") as f:
            p = json.load(f)
    else:
        if not (a.sheets or a.ods) or not a.map:
            sys.exit("--sheets or --ods, and --map (or --plan)")
        disc = icons = None
        if a.iso:
            import fill_spell_visuals_eqoaemu as fsv
            disc, icons = fsv.disc_visuals(a.iso), fsv.disc_icons(a.iso)
        p = plan(db, im.Workbook(im.read_workbook(a.sheets, a.ods)), im.read_map(a.map), disc, icons,
                 im.were_rewards(a.were_quests))
    report(p, a.list)
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
