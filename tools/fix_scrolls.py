#!/usr/bin/env python3
"""Spell scrolls put right (the user, 2026-11-01: "there was abug may be
fixed the veil of death sk ability is lvl 12, but lets you learn at lvl 4"):
a scroll names its spell only by its name (handlers/scribe.go's
scrollSpellFor, tools/import_eqoaemu_spells.py's scroll_spell), and checked
only its own level, so a scroll of a lower level than its spell taught the
spell early; and some scrolls sold or held name no spell at all.

  1. Levels. A scroll asks for the level of the spell it scribes, by the
     scribe's own match, for every class it serves (classuse); one whose
     classes' spells are of different levels is listed, not written, and so
     is one that would scribe another record at the new level (the match
     prefers its class's record at the scroll's level). Its description's
     "Level N -" follows; its price stays (the report gives the old and the
     new level's on Sandstorm's scroll curve, import_eqoaemu_spells.py's
     scroll_curve). Every level is its spell's record's: the workbook's
     where the EQOAEmu import set it (the shadow knight's Veil of Death 12,
     its scroll 4).
  2. A scroll that teaches nothing and is sold (npcitem) or held
     (charinventory): the spell its name means, where that is clear (its
     name less a trailing " scroll" and a leading "Summon ", any case, is
     exactly the name of one spell of a class it serves), is taught by it:
     folded into a scroll that already teaches that spell
     (tools/fix_broken_records.py's fold: every row naming it names that
     scroll, a merchant that sells both keeping one, and its record goes),
     else renamed to the spell's name, its level then the spell's (1.).
     Anything else (a misspelling, a number for a numeral, a name two spells
     could be) is listed with the best candidate among its classes' spells
     and the evidence (the level, the description's words), not written;
     so is a clear one neither sold nor held. A fold the user decided for
     one listed so (DECIDED: each pair of records by its names, with the
     user's word) is made the same way, only while both records are still
     the ones named (the dead scroll teaching nothing, the other teaching);
     else it is listed. A scroll's art on a quest item
     (no word of scribing in its name or description: "mystical parchment")
     or on a tradeskill's recipe (a name ending in "Recipe") is counted and
     left.

The server refuses a scroll whose spell is above its scriber's level
whatever the scroll's own level says (handlers/scribe.go, the same day), so
a scroll this leaves (listed) teaches nothing early. After a fresh import
(tools/import_eqoaemu_spells.py sets the spells' levels to the workbook's
and makes no scroll for a spell a scroll of its name already teaches, nor
brings that scroll's level to the workbook's) run this again: it is the
import's post step for the scrolls.

Report by default. --apply writes it all in one transaction, every row
logged in scroll_fix_log under the run's time (tools/fix_broken_records.py's
log shape); a second --apply writes nothing; --undo takes the last run back,
each row still as it wrote it (one changed since is left, and said), a
folded scroll's record and every row it moved back. Both take a copy of the
database first (--no-backup to skip). The server reads item records and the
merchants' lists as they are used: no restart (a player sees a record's new
level at the next login).

    tools/fix_scrolls.py [--db eqoa.db] [--apply | --undo] [--no-backup]
"""
import argparse
import collections
import datetime
import difflib
import os
import re
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import fix_broken_records as fb  # noqa: E402
import import_eqoaemu_spells as im  # noqa: E402

DEFAULT_DB = os.path.join(HERE, "..", "eqoa.db")
LOG = "scroll_fix_log"
CLASSES = 15
# a merchant lists an item once (handlers' shop list by listnumber): a dead
# scroll's row at one that sells the scroll it is folded into goes
ONCE = (("npcitem", "npcid"),)
LEVEL_WORDS = re.compile(r"\b(Level\s+)(\d+)(\s*-)")
# a scroll's art on what is no spell scroll: no word of scribing or of a
# scroll in its name or description, no "Level N -", no "Summon"
SCROLLISH = re.compile(r"(?i)\bscrib|\bscroll\b|\blevel\s+\d+\s*-|^summon\b")
RECIPE = re.compile(r"(?i)\brecipe$")    # a tradeskill's recipe ("Use it to scribe the recipe")
# Folds the user decided, of dead scrolls the rule lists (2.): the dead
# scroll's id -> (its name, the scroll it is folded into, that one's name,
# the user's word), made only while both records are still the ones named.
DECIDED = {
    20686: ("Hero Guard 3", 901017, "Hero Guard III", 'the user, 2026-11-01: "yes merge them"'),
    20836: ("Infusions", 900061, "Infusion", 'the user, 2026-11-01: "yes merge them"'),
}


def classes_of(classuse):
    return [c for c in range(CLASSES) if (classuse or 0) & (1 << c)]


def load(db):
    """The spells (id, name, level) in id order, each class's spells, and
    every scroll (the scroll's art, worn nowhere) as a dict."""
    spells = db.execute("SELECT spellid, spellname, abilitylvl FROM spellpattern ORDER BY spellid").fetchall()
    own = collections.defaultdict(set)
    for c, sid in db.execute("SELECT class, spellid FROM classspells"):
        own[c].add(sid)
    cols = ["patternid", "itemname", "levelreq", "classuse", "itemcost", "itemdesc"]
    scrolls = [dict(zip(cols, r)) for r in db.execute(
        f"SELECT {', '.join(cols)} FROM itempattern WHERE itemicon = ? AND equipslot = -1 ORDER BY patternid", (im.SCROLL_ICON,))]
    return spells, own, scrolls


def scribes(spells, own, name, level, classes):
    """The spell the scroll teaches each class it serves (None: none)."""
    return {c: im.scroll_spell(spells, name, level, lambda sid, c=c: sid in own[c]) for c in classes}


def meant(name):
    """The spell a dead scroll's name means: less a trailing " scroll" and a
    leading "Summon ", any case."""
    n = re.sub(r"(?i)\s+scroll$", "", (name or "").strip())
    return re.sub(r"(?i)^summon\s+", "", n).strip()


def decided_folds(db, decided, taught, sold, held, lists):
    """The folds the user decided (DECIDED) whose records are still the
    ones named, as plan's folds; the rest listed. Returns (folds, the ids
    they name)."""
    cols = fb.columns(db, "itempattern")
    folds = []
    for pid, (was, to, to_name, word) in sorted(decided.items()):
        r = db.execute(f"SELECT {', '.join(cols)} FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
        r = dict(zip(cols, r)) if r else None
        t = db.execute("SELECT itemname FROM itempattern WHERE patternid = ?", (to,)).fetchone()
        to_ok = t is not None and t[0] == to_name and to in taught
        if r is None and to_ok:
            lists["a fold the user decided, made already (its record gone)"].append(f"{pid} {was!r} -> {to} {to_name!r}")
            continue
        why = ("its record is gone" if r is None else
               f"it is {r['itemname']!r} now" if r["itemname"] != was else
               "it is no scroll now" if (r["itemicon"], r["equipslot"]) != (im.SCROLL_ICON, -1) else
               "it teaches a spell now" if pid in taught else
               f"{to} is gone" if t is None else
               f"{to} is {t[0]!r} now" if t[0] != to_name else
               f"{to} teaches no spell" if not to_ok else "")
        if why:
            lists["a fold the user decided whose records are not the ones named (left)"].append(
                f"{pid} {was!r} -> {to} {to_name!r}: {why}")
            continue
        folds.append(dict(patternid=pid, name=was, fold=to, fold_name=to_name, row=r,
                          how=f"teaches nothing (sold by {sold.get(pid, 0)}, held {held.get(pid, 0)}); folded into {to} "
                              f"{to_name!r}, as the user decided ({word})"))
    return folds, set(decided)


def plan(db, decided=None):
    """What --apply would write (renames, levels, folds) and the report's
    lists; decided the folds the user decided (DECIDED by default)."""
    spells, own, scrolls = load(db)
    by_sid = {s[0]: s for s in spells}
    curve = im.scroll_curve(db)
    sold = dict(db.execute("SELECT patternid, count(*) FROM npcitem GROUP BY patternid").fetchall()) \
        if fb.has_table(db, "npcitem") else {}
    held = dict(db.execute("SELECT patternid, count(*) FROM charinventory GROUP BY patternid").fetchall()) \
        if fb.has_table(db, "charinventory") else {}
    lists = collections.defaultdict(list)
    taught = {}       # patternid -> {class: spellid} of every scroll that teaches
    dead = []
    for s in scrolls:
        got = scribes(spells, own, s["itemname"], s["levelreq"], classes_of(s["classuse"]))
        if any(v is not None for v in got.values()):
            taught[s["patternid"]] = got
        else:
            dead.append(s)

    # 2. the dead: folded, renamed or listed; the user's decided folds first
    folds, handled = decided_folds(db, DECIDED if decided is None else decided, taught, sold, held, lists)
    renames, names = [], {}
    for s in dead:
        pid, name, classes = s["patternid"], s["itemname"], classes_of(s["classuse"])
        if pid in handled:
            continue
        where = f"sold by {sold.get(pid, 0)}, held {held.get(pid, 0)}"
        m = meant(name)
        ids = sorted({sid for sid, n, _ in spells if im.exact_key(n) == im.exact_key(m) and any(sid in own[c] for c in classes)})
        if not SCROLLISH.search(name) and not SCROLLISH.search(s["itemdesc"] or ""):
            lists["the scroll's art on a quest item (left)"].append(f"{pid} {name}")
            continue
        if RECIPE.search(name):
            lists["a tradeskill recipe (left)"].append(f"{pid} {name}")
            continue
        if len(ids) != 1:
            mine = [sp for sp in spells if any(sp[0] in own[c] for c in classes)]
            near = difflib.get_close_matches(m.lower(), [sp[1].lower() for sp in mine], n=3, cutoff=0.6)
            best = [sp for sp in mine if sp[1].lower() in near]
            # its own level first (a scroll asks for its spell's), then the nearest name
            best.sort(key=lambda sp: (sp[2] != s["levelreq"], near.index(sp[1].lower())))
            evidence = []
            for sp in best[:3]:
                why = [w for w, ok in (("its level", sp[2] == s["levelreq"]),
                                       ("its description names it", sp[1].lower() in (s["itemdesc"] or "").lower())) if ok]
                by = sorted(p for p, got in taught.items() if any(got.get(c) == sp[0] for c in classes))
                why.append(f"taught by scroll {by[0]}" if by else "no scroll teaches it")
                evidence.append(f"{sp[0]} {sp[1]!r} L{sp[2]} ({', '.join(why)})")
            what = "two spells of its name: " + ", ".join(f"{i} L{by_sid[i][2]}" for i in ids) if len(ids) > 1 else \
                "no spell of its name"
            kind = ("a dead scroll sold or held, its spell unclear (listed: the user's call)" if sold.get(pid) or held.get(pid)
                    else "a dead scroll neither sold nor held, its spell unclear (left)")
            lists[kind].append(f"{pid} {name!r} L{s['levelreq']} classes {classes} ({where}): {what}; best: "
                               f"{'; '.join(evidence) or 'none'}; description {(s['itemdesc'] or '')[:70]!r}")
            continue
        sid = ids[0]
        # a scroll it folds into teaches the spell to every class it serves
        teachers = sorted(p for p, got in taught.items() if all(got.get(c) == sid for c in classes if sid in own[c]))
        some = sorted(p for p, got in taught.items() if any(got.get(c) == sid for c in classes))
        if not (sold.get(pid) or held.get(pid)):
            lists["a dead scroll neither sold nor held whose spell is clear (left)"].append(
                f"{pid} {name!r} -> {sid} {by_sid[sid][1]!r} L{by_sid[sid][2]}: "
                + (f"would fold into {teachers[0]}" if teachers else "would be renamed" if not some else
                   f"taught by {some[0]} to some of its classes only"))
            continue
        if some and not teachers:
            lists["a dead scroll sold or held whose spell a scroll teaches some of its classes only (listed, not written)"].append(
                f"{pid} {name!r} classes {classes} ({where}) -> {sid} {by_sid[sid][1]!r} L{by_sid[sid][2]}: taught by "
                + ", ".join(f"{p} to classes {sorted(c for c, v in taught[p].items() if v == sid)}" for p in some))
            continue
        if teachers:
            to = teachers[0]
            to_row = db.execute("SELECT itemname FROM itempattern WHERE patternid = ?", (to,)).fetchone()
            row = dict(zip(fb.columns(db, "itempattern"), db.execute("SELECT * FROM itempattern WHERE patternid = ?", (pid,)).fetchone()))
            folds.append(dict(patternid=pid, name=name, fold=to, fold_name=to_row[0], row=row,
                              how=f"teaches nothing ({where}); its name means {sid} {by_sid[sid][1]!r}, which {to} "
                                  f"{to_row[0]!r} teaches: folded into it"))
        else:
            renames.append(dict(patternid=pid, name=name, before={"itemname": name}, set={"itemname": by_sid[sid][1]},
                                how=f"teaches nothing ({where}); its name means {sid} {by_sid[sid][1]!r}, which no scroll "
                                    f"teaches: renamed"))
            names[pid] = by_sid[sid][1]

    # 1. levels: every scroll that teaches (a renamed one by its new name)
    levels = []
    folded = {f["patternid"] for f in folds}
    for s in scrolls:
        pid = s["patternid"]
        if pid in folded or (pid not in taught and pid not in names):
            continue
        name, lv, classes = names.get(pid, s["itemname"]), s["levelreq"], classes_of(s["classuse"])
        got = taught.get(pid) or scribes(spells, own, name, lv, classes)
        sids = {v for v in got.values() if v is not None}
        want = {by_sid[v][2] for v in sids}
        if want == {lv}:
            continue
        tag = f"{pid} {name!r} L{lv}: " + ", ".join(f"class {c} {v} L{by_sid[v][2]}" for c, v in sorted(got.items()) if v)
        if len(want) != 1:
            lists["a scroll whose classes' spells are of different levels (listed, not written)"].append(tag)
            continue
        new = want.pop()
        again = scribes(spells, own, name, new, classes)
        if any(again.get(c) != got.get(c) for c in classes):
            lists["a scroll that would scribe another record at its spell's level (listed, not written)"].append(
                tag + f"; at L{new}: " + ", ".join(f"class {c} {v}" for c, v in sorted(again.items())))
            continue
        desc = s["itemdesc"] or ""
        set_ = {"levelreq": new}
        if LEVEL_WORDS.search(desc):
            set_["itemdesc"] = LEVEL_WORDS.sub(lambda mo: f"{mo.group(1)}{new}{mo.group(3)}", desc)
        levels.append(dict(patternid=pid, name=name, before={k: s[k] for k in set_}, set=set_,
                           how=f"its spell {', '.join(str(v) for v in sorted(sids))} is of level {new}; the price stays "
                               f"{s['itemcost']} (the curve's {curve(lv)} at {lv}, {curve(new)} at {new})"))
    lists["scrolls that teach"] = [f"{len(taught)}"]
    lists["dead scrolls"] = [f"{len(dead)}: {len(folds)} folded, {len(renames)} renamed, "
                             f"{sum(1 for s in dead if sold.get(s['patternid']) or held.get(s['patternid']))} sold or held"]
    return dict(renames=renames, levels=levels, folds=folds, lists={k: v for k, v in lists.items()})


def apply(db, p, when=None):
    """The plan in one transaction, every row logged in LOG: (renames,
    levels, folds, rows a fold moved, rows a fold dropped (a merchant that
    sold both), stale)."""
    when = when or datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")
    made_log = not fb.has_table(db, LOG)
    n_ren = n_lv = n_fold = moved = stale = 0
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(fb.log_ddl(LOG))
        last = db.execute(f"SELECT max(run) FROM {LOG}").fetchone()[0]
        if last is not None and when <= last:
            raise SystemExit(f"the last run was applied at {last}, this one at {when}: run it again in a second")
        for kind, items in (("rename", p["renames"]), ("level", p["levels"])):
            for c in items:
                ref = {"patternid": c["patternid"]}
                now = fb.row_now(db, "itempattern", ref, list(c["set"]))
                if now == c["set"]:
                    continue  # as planned already
                if fb.row_now(db, "itempattern", ref, list(c["before"])) != c["before"]:
                    stale += 1
                    continue
                db.execute(f"UPDATE itempattern SET {', '.join(f'{k} = ?' for k in c['set'])} WHERE patternid = ?",
                           [*c["set"].values(), c["patternid"]])
                fb.log(db, when, "itempattern", ref, "update", c["before"], c["set"], f"{c['name']}: {c['how']}", LOG)
                if kind == "rename":
                    n_ren += 1
                else:
                    n_lv += 1
        for c in p["folds"]:
            if fb.row_now(db, "itempattern", {"patternid": c["patternid"]}, ["itemname"]) is None:
                continue  # folded already
            done = fb.fold_into(db, when, c, LOG, ONCE)
            if done is None:
                stale += 1
                continue
            n_fold += 1
            moved += done[0]
        dropped = db.execute(f"SELECT count(*) FROM {LOG} WHERE run = ? AND action = 'delete' AND tbl != 'itempattern'",
                             (when,)).fetchone()[0]
        if not (n_ren or n_lv or n_fold) and made_log:
            db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return n_ren, n_lv, n_fold, moved - dropped, dropped, stale


def report(p):
    print(f"renames {len(p['renames'])}, levels {len(p['levels'])}, folds {len(p['folds'])}")
    for c in p["renames"] + p["levels"]:
        print(f"  {c['patternid']} {c['name']}: {fb.dumps(c['before'])} -> {fb.dumps(c['set'])}")
        print(f"      {c['how']}")
    for c in p["folds"]:
        print(f"  {c['patternid']} {c['name']}: {c['how']}")
    for k, v in p["lists"].items():
        print(f"  {k}: {len(v) if k not in ('scrolls that teach', 'dead scrolls') else v[0]}")
        if k in ("scrolls that teach", "dead scrolls", "the scroll's art on a quest item (left)", "a tradeskill recipe (left)"):
            continue
        for x in v:
            print(f"      {x}")


def backup(path):
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-scrolls"
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
        back, left, run = fb.undo(db, LOG)
        print(f"the run of {run}: put back {back}" + (f"; left, changed since: {', '.join(left)}" if left else ""))
        return
    p = plan(db)
    report(p)
    if a.apply:
        n_ren, n_lv, n_fold, moved, dropped, stale = apply(db, p)
        print(f"written: {n_ren} renamed, {n_lv} levels, {n_fold} folded ({moved} rows moved, {dropped} dropped: "
              f"a merchant that sold both); {stale} changed since the plan, left")
    else:
        print("(report only: --apply to write it)")


if __name__ == "__main__":
    main()
