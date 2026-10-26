#!/usr/bin/env python3
"""The class-mastery catalogue (cmcatalog) where tools/seed_masteries.py left
out or filled in what EQOA-Data's lists say, without disabling anything
(jib's review, F3 pass 19).

Two corrections, each from the lists (Information/databases/cms/*.json):

- A Master Class's first rank names, in the list's notes, the masteries it
  needs: the warrior's Defender 1 "Controlled Defense 3" and "Defender's
  Resolve 3", Hero 1 "Hero's Shout 4" and "Hero's Blood 3", Soldier 1 the
  four Soldier archetype masteries at rank 3; the wizard's Channeler 1
  "Channel" and "Channeler's Bounty 2", Sorcerer 1 "Sorcerous Maelstrom 4".
  The seed wrote them into the rank's text only; they go into require_ranks
  (their ids), which entity/cmbuy.go enforces. A note is found among the
  ranks the master class's class is offered, by its name, a possessive "'s"
  and punctuation aside ("Defender's Resolve 3" is "Defender Resolve 3").
  A master class the lists give to several classes with the same
  requirements and effects, its notes in one class's list only, takes them
  for the others too (Soldier: the warrior's list names them, the paladin's
  and the shadow knight's do not): OURS, by it being the one master class.
  The other master classes' notes name nothing (the lists' silence; the two
  eqoaclassic's catalogue was given by hand, Spellbinder and Animator, keep
  theirs).
- A rank the list gives no level took the rank before it's, else 1 (the
  seed's, OURS): Nife Warding 1 and United Faith 1 were buyable at level 1.
  It is now the highest level of an ability the rank adds or needs (OURS,
  as the list's own ranks after those two sit at their abilities' levels:
  Nife Warding 2-5 at 35, 45, 54, 60), else the rank before it's, else 1;
  the druid's Nature Cloak, Nature Sway and Nature Mantle had the same gaps.

Checked against the lists and left as they are: Soldier 1's STR 20 (all
three lists say 20, not 200), Defender 3 (its list names nothing it needs:
it adds Immortal Guard), and the ranks that add and remove one spell (Quick
Blade, Forester Boon, Natural Stance, Infernal Hatred, Bardic Expertise and
the rest): the list's way of improving a spell in place, which the server
keeps in the book (entity.CMRank.BookChange); it used to give it and take
it out, so Hydromancy 1 cost a magician the spell Hydromancy 2 needs.

Only a rank the seed wrote (cmcatalog_seed, source "eqoa-data cms") is
changed, and only a column still holding the seed's value; a row a GM has
changed in the panel (gm_audit) is left whole, and every row left is said.
Nothing is disabled or added, and what characters own (charcms) is never
touched: the report counts those whose ranks a corrected requirement would
no longer let them buy. Before writing, every rank is walked for every class
and race (parent and require_ranks, levels, points, the spells it needs
against the class's spells, one master class at a time): a plan that leaves
a rank unreachable is refused.

Every change is logged in cmcatalog_fix_log (the row, the column, the value
before and after, and why); --undo puts back each value still as this left
it (one changed since is left, and said) and drops the log. Both take a copy
of the database first (<db>.bak-<when>-pre-cmfix). Report only by default.
The server reads the catalogue at its start, and again when a GM edits a
cmcatalog row in the panel: restart it after --apply or --undo.

    tools/fix_mastery_catalog.py --eqoa-data ../external/EQOA-Data [--db eqoa.db] [--apply | --undo]
"""
import argparse
import collections
import datetime
import json
import os
import re
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import seed_masteries as seed  # noqa: E402

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
MASTER = 4
MAX_LEVEL = 60       # entity.MaxLevel
MASTERY_CAP = 1500   # entity.MasteryCap: points spent and unspent
STAT_CAP = 400       # a base stat's training cap at level 60 (m_p44)
CLASS_NAMES = seed.CLASS_NAMES

LOG_SCHEMA = """CREATE TABLE IF NOT EXISTS cmcatalog_fix_log (
    id      INTEGER NOT NULL,
    col     TEXT NOT NULL,
    before,
    after,
    why     TEXT NOT NULL,
    applied TEXT NOT NULL,
    PRIMARY KEY (id, col)
)"""


def name_key(name):
    """A rank's name compared: no possessive "'s", no punctuation or spaces."""
    n = (name or "").lower().replace("\u2019", "'")
    n = re.sub(r"'s\b", "", n)
    return re.sub(r"[^a-z0-9]", "", n)


def ids(text):
    """A comma list of ids, as database.SpellIDs reads one: numbers above 0."""
    out = []
    for f in str(text or "").split(","):
        try:
            n = int(f.strip())
        except ValueError:
            continue
        if n > 0:
            out.append(n)
    return out


def stats(text):
    out = {}
    for f in str(text or "").split(","):
        k, _, v = f.partition(":")
        try:
            out[k.strip().lower()] = int(v)
        except ValueError:
            pass
    return out


# ---- what the lists say -----------------------------------------------------

def list_master_classes(data):
    """Every master class's first rank in the lists: [(class, mastery name,
    rank-1 dict)], in class order."""
    out = []
    for c, d in enumerate(data["classes"]):
        for g in d["groups"]:
            if str(g["name"]).strip().lower() != "master classes":
                continue
            for cm in g["cms"]:
                if cm.get("ranks"):
                    out.append((c, cm["name"].strip(), cm["ranks"][0]))
    return out


def same_rank(a, b):
    """Two lists' first ranks of one master class say the same (but notes)."""
    return (a.get("cmCost"), a.get("requirements"), a.get("effects")) == \
        (b.get("cmCost"), b.get("requirements"), b.get("effects"))


def master_notes(data):
    """{(class, mastery): (notes, why)}: each master class's first rank's
    notes, from its own list, else (OURS) from another class's list of the
    same master class saying the same."""
    mcs = list_master_classes(data)
    by_name = collections.defaultdict(list)
    for c, m, r in mcs:
        by_name[m.lower()].append((c, m, r))
    out = {}
    for c, m, r in mcs:
        notes = [n.strip() for n in r.get("notes") or [] if n.strip()]
        if notes:
            out[(c, m)] = (notes, f"{seed.CLASS_FILES[c]}.json's {r['name']} notes")
            continue
        others = [(oc, orank) for oc, _, orank in by_name[m.lower()]
                  if oc != c and orank.get("notes") and same_rank(orank, r)]
        named = {tuple(n.strip() for n in orank["notes"]) for _, orank in others}
        if len(named) == 1:
            oc = others[0][0]
            out[(c, m)] = (list(named.pop()), f"OURS: {seed.CLASS_FILES[oc]}.json's {m} 1 notes, "
                                              f"the same master class")
    return out


def rank_levels(data):
    """({rank id: (level, why, the seed's level)} for every rank the list
    gives no level: the highest level of an ability it adds or needs (OURS),
    else the rank before it's, else 1; and the seed's rows by id)."""
    skipped = collections.Counter()
    rows = seed.plan_rows(data, skipped)  # the seed's ids and levels
    seeded = {r["id"]: r for r in rows}
    out = {}

    def walk(category, base, cms):
        for mi, cm in enumerate(cms):
            if seed.is_were(cm["name"]):
                continue
            level = 1
            for ri, r in enumerate(cm.get("ranks", [])):
                rid = base + 10 * mi + ri + 1  # seed_masteries.py's ids
                if rid not in seeded or (seeded[rid]["category"], seeded[rid]["rank"]) != (category, ri + 1):
                    sys.exit(f"{r['name']}: id {rid} is not seed_masteries.py's; the two are out of step")
                req = r.get("requirements") or {}
                if req.get("level") is not None:
                    level = int(req["level"])
                    continue
                eff = r.get("effects") or {}
                named = [a.get("level") for a in (eff.get("addAbilities") or []) + (req.get("requiredAbilities") or [])]
                named = [int(n) for n in named if isinstance(n, (int, float)) and n > 0]
                if named:
                    level, why = max(named), "OURS: the highest level of an ability it adds or needs"
                elif ri:
                    why = "the rank before it's"
                else:
                    level, why = 1, "no level and no ability: 1"
                out[rid] = (level, why, seeded[rid]["level"])

    for g in data["general"]["groups"]:
        walk(0, 0, g["cms"])
    for gi, g in enumerate(data["racial"]["groups"]):
        walk(1, 1000 + 100 * gi, g["cms"])
    for gi, g in enumerate(data["archetype"]["groups"]):
        walk(2, 2000 + 200 * gi, g["cms"])
    for c, d in enumerate(data["classes"]):
        for g in d["groups"]:
            kind = str(g["name"]).strip().lower()
            if kind == "class masteries":
                walk(3, 3000 + 300 * c, g["cms"])
            elif kind == "master classes":
                walk(4, 3000 + 300 * c + 200, g["cms"])
    return out, seeded


# ---- the database -----------------------------------------------------------

COLS = ("id", "category", "classes", "races", "mastery", "rank", "name", "parent", "cost", "level", "spent",
        "minstats", "require_ranks", "require_spells", "add_spells", "remove_spells", "enabled", "source")


def catalogue(db):
    return {r[0]: dict(zip(COLS, r)) for r in db.execute(f"SELECT {', '.join(COLS)} FROM cmcatalog")}


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def seeded_ids(db):
    if not has_table(db, "cmcatalog_seed"):
        return set()
    return {i for (i,) in db.execute("SELECT id FROM cmcatalog_seed")}


def gm_changed(db):
    """The cmcatalog rows a GM has changed through the panel (gm_audit)."""
    if not has_table(db, "gm_audit"):
        return set()
    out = set()
    for (k,) in db.execute("SELECT rowkey FROM gm_audit WHERE tbl = 'cmcatalog'"):
        try:
            out.add(int(json.loads(k)["id"]))
        except (ValueError, KeyError, TypeError):
            m = re.search(r"\d+", k or "")
            if m:
                out.add(int(m.group()))
    return out


def class_spells(db):
    out = collections.defaultdict(set)
    if has_table(db, "classspells"):
        for c, sid in db.execute("SELECT class, spellid FROM classspells"):
            out[c].add(sid)
    return out


# ---- the plan ---------------------------------------------------------------

def find_rank(cat, c, note):
    """The id of the rank a note names among those class c is offered (no
    master class), or (0, why)."""
    want, key = note.strip().lower(), name_key(note)
    pool = [r for r in cat.values() if r["category"] != MASTER and r["classes"] >> c & 1]
    for test in (lambda r: r["name"].strip().lower() == want, lambda r: name_key(r["name"]) == key):
        hits = [r for r in pool if test(r)]
        if hits:
            order = {3: 0, 2: 1, 0: 2, 1: 3}
            hits.sort(key=lambda r: (order.get(r["category"], 9), r["id"]))
            if len(hits) > 1 and order.get(hits[0]["category"]) == order.get(hits[1]["category"]):
                return 0, f"{note!r} names {len(hits)} ranks"
            return hits[0]["id"], ""
    return 0, f"{note!r} is no rank the {CLASS_NAMES[c]} is offered"


def plan(db, data):
    """(changes, left, problems): changes [(id, col, before, after, why)];
    left [(id, name, why)] for a row the plan would change but must not;
    problems [text] for notes it could not read."""
    cat = catalogue(db)
    mine = seeded_ids(db)
    gm = gm_changed(db)
    levels, seeded = rank_levels(data)
    by_mc = {(r["category"], r["classes"], r["mastery"].lower(), r["rank"]): r["id"] for r in seeded.values()}
    wants, problems = [], []

    for (c, m), (notes, why) in sorted(master_notes(data).items()):
        rid = by_mc.get((MASTER, 1 << c, m.lower(), 1))
        if rid is None or rid not in cat:
            problems.append(f"{m} 1 ({CLASS_NAMES[c]}): not in the catalogue")
            continue
        found, missing = [], []
        for n in notes:
            nid, err = find_rank(cat, c, n)
            if nid:
                found.append(nid)
            else:
                missing.append(err)
        if missing and why.startswith("OURS"):
            # another class's notes hold for this one only when they are all
            # ranks it is offered (the archetype's, as Soldier's are)
            problems.append(f"{cat[rid]['name']} ({CLASS_NAMES[c]}): not taking {why[6:]}: " + "; ".join(missing))
            continue
        for err in missing:
            problems.append(f"{cat[rid]['name']} ({CLASS_NAMES[c]}): {err}; left out")
        if found:
            wants.append((rid, "require_ranks", "", ",".join(str(i) for i in found),
                          f"{why}: " + ", ".join(cat[i]["name"] for i in found)))

    for rid, (level, why, was) in sorted(levels.items()):
        if level != was:
            wants.append((rid, "level", was, level, f"no level in the list: {why} (was {was}, the seed's)"))

    changes, left = [], []
    for rid, col, seeds, after, why in wants:
        r = cat.get(rid)
        if r is None:
            continue
        now = r[col]
        if str(now) == str(after):
            continue  # already so
        name = r["name"]
        if rid not in mine or r["source"] != seed.SOURCE:
            left.append((rid, name, f"{col}: not a row the seed wrote (source {r['source']!r})"))
        elif rid in gm:
            left.append((rid, name, f"{col}: a GM has changed the row (gm_audit)"))
        elif str(now) != str(seeds):
            left.append((rid, name, f"{col} is {now!r}, not the seed's {seeds!r}: changed since, left"))
        else:
            changes.append((rid, col, now, after, why))
    return changes, left, problems


def applied(cat, changes):
    """The catalogue with the changes made (a copy)."""
    out = {i: dict(r) for i, r in cat.items()}
    for rid, col, _, after, _ in changes:
        out[rid][col] = after
    return out


# ---- the walk ---------------------------------------------------------------

def book_change(r):
    """entity.CMRank.BookChange: the spells a purchase gives and takes, one
    both added and removed in neither."""
    adds, removes = set(ids(r["add_spells"])), set(ids(r["remove_spells"]))
    return adds - removes, removes - adds


def prereqs(cat, r):
    """The ranks a rank needs: the rank before it and require_ranks' ids the
    catalogue holds (cmbuy.go checks no other)."""
    out = [r["parent"]] if r["parent"] else []
    return out + [i for i in ids(r["require_ranks"]) if i in cat]


def ancestors(cat, rid):
    seen, stack = set(), list(prereqs(cat, cat[rid]))
    while stack:
        a = stack.pop()
        if a in seen or a not in cat:
            continue
        seen.add(a)
        stack += prereqs(cat, cat[a])
    return seen


def cycles(cat):
    """The ranks on a cycle of parent and require_ranks."""
    state, on = {}, set()

    def visit(i, path):
        state[i] = 1
        path.append(i)
        for p in prereqs(cat, cat[i]):
            if p not in cat:
                continue
            if state.get(p) == 1:
                on.update(path[path.index(p):])
            elif p not in state:
                visit(p, path)
        path.pop()
        state[i] = 2

    sys.setrecursionlimit(max(10000, sys.getrecursionlimit()))
    for i in sorted(cat):
        if i not in state:
            visit(i, [])
    return on


def walk(cat, spells_of):
    """{rank id: [(class, race), ...]} for every rank a class and race it is
    offered can never buy, as cmbuy.go has it: enabled, the rank before it
    and require_ranks first (offered and bought), its level at most 60, its
    points spent before it and its cost within the 1500, its stats within
    the trainer's cap, the spells it needs the class's or a bought rank's
    and not taken out by a rank it needs, and no other master class needed."""
    anc = {i: ancestors(cat, i) for i in cat}
    taken = {i: set().union(*(book_change(cat[a])[1] for a in anc[i])) if anc[i] else set() for i in cat}
    bad = collections.defaultdict(list)
    for c in range(15):
        for race in range(11):
            offered = {i: r for i, r in cat.items() if r["enabled"] and r["classes"] >> c & 1 and r["races"] >> race & 1}
            reach, gained, spent = set(), set(), 0
            grew = True
            while grew:
                grew = False
                for i in sorted(offered):
                    r = offered[i]
                    if i in reach:
                        continue
                    pre = prereqs(cat, r)
                    if any(p not in offered or p not in reach for p in pre):
                        continue
                    if r["category"] == MASTER and any(cat[a]["category"] == MASTER and cat[a]["mastery"] != r["mastery"]
                                                       for a in anc[i]):
                        continue
                    if r["level"] > MAX_LEVEL or r["spent"] + r["cost"] > MASTERY_CAP or r["spent"] > spent:
                        continue
                    if any(v > STAT_CAP for v in stats(r["minstats"]).values()):
                        continue
                    if any(s in taken[i] or (s not in spells_of[c] and s not in gained) for s in ids(r["require_spells"])):
                        continue
                    reach.add(i)
                    gained |= book_change(r)[0]
                    if r["category"] != MASTER:
                        spent += r["cost"]
                    grew = True
            for i in offered:
                if i not in reach:
                    bad[i].append((c, race))
    return bad


def owners_short(db, cat, changes):
    """{serverid: [(rank, what)]}: characters owning a changed rank whose new
    requirement they would not meet now (nothing is taken from them)."""
    if not has_table(db, "charcms"):
        return {}
    owned = collections.defaultdict(set)
    for sid, cmid in db.execute("SELECT serverid, cmid FROM charcms"):
        owned[sid].add(cmid)
    level = {}
    if has_table(db, "characters"):
        level = dict(db.execute("SELECT serverid, level FROM characters"))
    out = collections.defaultdict(list)
    for rid, col, _, after, _ in changes:
        for sid, have in owned.items():
            if rid not in have:
                continue
            if col == "require_ranks":
                lack = [i for i in ids(after) if i not in have]
                if lack:
                    out[sid].append((cat[rid]["name"], "lacks " + ", ".join(cat[i]["name"] for i in lack)))
            elif col == "level" and level.get(sid, MAX_LEVEL) < after:
                out[sid].append((cat[rid]["name"], f"level {level.get(sid)} below {after}"))
    return out


# ---- the tool ---------------------------------------------------------------

def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-cmfix"
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def describe(bad, cat):
    lines = []
    for i in sorted(bad):
        who = bad[i]
        classes = sorted({c for c, _ in who})
        lines.append(f"  {i} {cat[i]['name']}: {', '.join(CLASS_NAMES[c] for c in classes)} "
                     f"({len(who)} class and race pairs)")
    return lines


def report(db, data):
    """The plan, said; what it would leave unreachable (refused when it is
    more than today's)."""
    changes, left, problems = plan(db, data)
    cat = catalogue(db)
    for rid, col, before, after, why in changes:
        print(f"{rid:6d} {cat[rid]['name']}: {col} {before!r} -> {after!r}  ({why})")
    for rid, name, why in left:
        print(f"{rid:6d} {name}: left as it is ({why})")
    for p in problems:
        print(f"  note: {p}")
    print(f"{len(changes)} values to change, {len(left)} left")
    spells_of = class_spells(db)
    fixed = applied(cat, changes)
    before, after = walk(cat, spells_of), walk(fixed, spells_of)
    loops = cycles(fixed) - cycles(cat)
    worse = {i: v for i, v in after.items() if set(v) - set(before.get(i, []))}
    print(f"walk: {len(cat)} ranks, every class and race: {len(after)} unreachable after "
          f"({len(before)} before), {len(cycles(fixed))} on a cycle")
    for line in describe(after, fixed):
        print(line)
    short = owners_short(db, cat, changes)
    print(f"characters whose ranks a corrected requirement would not let them buy now: {len(short)} "
          f"(nothing is taken from them)")
    for sid, what in sorted(short.items()):
        print(f"  character {sid}: " + "; ".join(f"{n} ({w})" for n, w in what))
    return changes, worse, loops


def apply(db, changes):
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_SCHEMA)
        for rid, col, before, after, why in changes:
            # a later run keeps the first value before, so --undo goes back to it
            db.execute("INSERT OR IGNORE INTO cmcatalog_fix_log (id, col, before, after, why, applied) "
                       "VALUES (?, ?, ?, ?, ?, ?)", (rid, col, before, after, why, when))
            db.execute("UPDATE cmcatalog_fix_log SET after = ?, why = ?, applied = ? WHERE id = ? AND col = ?",
                       (after, why, when, rid, col))
            db.execute(f"UPDATE cmcatalog SET {col} = ? WHERE id = ?", (after, rid))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db, path):
    if not has_table(db, "cmcatalog_fix_log"):
        print("nothing logged: nothing to undo")
        return
    logged = db.execute("SELECT id, col, before, after FROM cmcatalog_fix_log ORDER BY id, col").fetchall()
    print(f"copy: {backup(db, path)}")
    db.execute("BEGIN IMMEDIATE")
    try:
        back = 0
        for rid, col, before, after in logged:
            if col not in ("require_ranks", "level"):
                raise SystemExit(f"cmcatalog_fix_log names column {col!r}, which this tool never writes")
            now = db.execute(f"SELECT {col} FROM cmcatalog WHERE id = ?", (rid,)).fetchone()
            if now is None or now[0] != after:
                print(f"  {rid} {col}: changed since ({now[0] if now else 'no row'!r}): left as it is")
                continue
            db.execute(f"UPDATE cmcatalog SET {col} = ? WHERE id = ?", (before, rid))
            back += 1
        db.execute("DROP TABLE cmcatalog_fix_log")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"put back {back} of {len(logged)} values; restart the server to read them")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--eqoa-data", help="EQOA-Data's checkout, or its Information/databases/cms directory")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the changes (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply changed")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    if not has_table(db, "cmcatalog"):
        sys.exit(f"{a.db}: no cmcatalog (tools/seed_masteries.py fills it)")
    if a.undo:
        undo(db, a.db)
        return
    if not a.eqoa_data:
        sys.exit("--eqoa-data is required")
    changes, worse, loops = report(db, seed.load(a.eqoa_data))
    if worse or loops:
        sys.exit("refused: the plan would leave the ranks above unreachable" if worse else
                 "refused: ranks on a cycle of parent and require_ranks")
    if not a.apply or not changes:
        if not a.apply:
            print("report only: --apply to write")
        return
    print(f"copy: {backup(db, a.db)}")
    apply(db, changes)
    print(f"changed {len(changes)} values; restart the server to read them")


if __name__ == "__main__":
    main()
