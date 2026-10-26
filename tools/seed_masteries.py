#!/usr/bin/env python3
"""The class masteries a character may buy (table cmcatalog, entity/cmbuy.go),
out of EQOA-Data's lists (Information/databases/cms/*.json); F3 pass 17, the
user: "we can implement buying class masteries", and the general ones "Make
them buyable too".

A row a rank, in CLIENT's five categories (its words 1653-1656 and 511):

- 0 General: general.json, every class and race;
- 1 Racial: racial.json, by race (both of its human groups are race 0; the
  froglok has none);
- 2 Archetype: archetype.json's Tank, Healer, Melee and Caster, each for the
  classes whose class list names it as their type (warrior, paladin and shadow
  knight are Tank, and so on);
- 3 Class and 4 Master Class: each class list's "Class Masteries" and "Master
  Classes".

Left out: were.json, werehunter.json and the archetypes' "... Were-Hunter"
(the were forms are not on this server; the user, F3 pass 17: "Were forms:
Remove them").

A rank's id is the window's skill id, fixed by where it is in its list so a
new rank in EQOA-Data moves no other: general 1 + 10 x mastery + rank, racial
1000 + 100 x group + 10 x mastery + rank, archetype 2000 + 200 x group + ...,
a class 3000 + 300 x class + 10 x mastery + rank, its master classes 200 above
(the rank counted from 0). Its parent is the rank before it, which must be
owned first. Cost, level, points spent, the base stats it needs and the stats
it gives are the list's (effects in its own names: "STR:1,STRMax:15", "HP
Factor:1", "Pow:500", "MovRate:5"). A rank the list gives no level takes the
rank before it's (Loping Stride 3: OURS), else 1. The text is the mastery's
description and the rank's notes. A Master Class's first rank is the class
itself and is named bare, "Spellbinder" for the list's "Spellbinder 1" (F3
20261026-135728); the ranks after it keep their numbers.

The abilities a rank adds, removes or needs are found among the server's
spells (spellpattern) by name, as the spell importers match a scroll to its
spell (import_eqoadata_spells.spell_name), preferring a spell of the rank's
classes (classspells), then one of the list's level; then with punctuation and
spaces ignored; then a close spelling among the classes' spells (difflib, 0.85:
the report lists every one). A name not found is still shown in the window and
is not given (an addition) or checked (a requirement); a GM may set its id in
the panel (add_spells, remove_spells, require_spells).

Rows are written with source "eqoa-data cms" and never over a row already
there (a GM's); each is logged in cmcatalog_seed, and --undo deletes exactly
those (refused while a character owns one of them, unless --force, which
leaves the characters' ranks in charcms: #cmreset gives their points back).
Both take a copy of the database first (<db>.bak-<when>-pre-cmseed). Report
only by default. The tables are the server's (database/cmcatalog.go); --apply
makes them if the server has not yet. Stop the server around --apply and
--undo, or restart it after (it reads the catalog at its start, and again when
a GM edits a row in the panel).

    tools/seed_masteries.py --eqoa-data ../external/EQOA-Data [--db eqoa.db]
        [--apply | --undo [--force]] [--list]
"""
import argparse
import collections
import datetime
import difflib
import json
import os
import re
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from import_eqoadata_spells import CLASS_FILES, spell_name  # noqa: E402

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
SOURCE = "eqoa-data cms"
ALL_CLASSES = (1 << 15) - 1
ALL_RACES = (1 << 11) - 1  # the froglok (10) too

# the tables as database/cmcatalog.go makes them (TestSeedSchemaIsTheServers
# holds the two to one text)
SCHEMA = """
	CREATE TABLE IF NOT EXISTS cmcatalog (
		id            INTEGER PRIMARY KEY,
		category      INTEGER NOT NULL DEFAULT 0,
		classes       INTEGER NOT NULL DEFAULT 32767,
		races         INTEGER NOT NULL DEFAULT 2047,
		mastery       TEXT NOT NULL DEFAULT '',
		rank          INTEGER NOT NULL DEFAULT 1,
		name          TEXT NOT NULL DEFAULT '',
		parent        INTEGER NOT NULL DEFAULT 0,
		cost          INTEGER NOT NULL DEFAULT 0,
		level         INTEGER NOT NULL DEFAULT 1,
		spent         INTEGER NOT NULL DEFAULT 0,
		minstats      TEXT NOT NULL DEFAULT '',
		requires      TEXT NOT NULL DEFAULT '',
		require_spells TEXT NOT NULL DEFAULT '',
		require_ranks TEXT NOT NULL DEFAULT '',
		effects       TEXT NOT NULL DEFAULT '',
		adds          TEXT NOT NULL DEFAULT '',
		add_spells    TEXT NOT NULL DEFAULT '',
		removes       TEXT NOT NULL DEFAULT '',
		remove_spells TEXT NOT NULL DEFAULT '',
		description   TEXT NOT NULL DEFAULT '',
		enabled       INTEGER NOT NULL DEFAULT 1,
		source        TEXT NOT NULL DEFAULT 'gm'
	);
	CREATE TABLE IF NOT EXISTS charcms (
		serverid INTEGER NOT NULL,
		cmid     INTEGER NOT NULL,
		cost     INTEGER NOT NULL DEFAULT 0,
		bought   INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (serverid, cmid)
	);"""

LOG_DDL = """CREATE TABLE IF NOT EXISTS cmcatalog_seed (
    id   INTEGER PRIMARY KEY,  -- a cmcatalog row --apply wrote
    note TEXT NOT NULL DEFAULT ''
)"""

RACES = {"human": 0, "elf": 1, "dark elf": 2, "gnome": 3, "dwarf": 4, "troll": 5, "barbarian": 6,
         "halfling": 7, "erudite": 8, "ogre": 9}
ARCHETYPES = ("tank", "healer", "melee", "caster")
CATEGORY_NAMES = ("general", "racial", "archetype", "class", "master class")
CLASS_NAMES = "WAR RAN PAL SK MNK BRD RGE DRD SHA CL MAG NEC ENC WIZ ALC".split()


def cms_dir(path):
    sub = os.path.join(path, "Information", "databases", "cms")
    return sub if os.path.isdir(sub) else path


def load(path):
    d = cms_dir(path)

    def read(name):
        p = os.path.join(d, name + ".json")
        if not os.path.exists(p):
            sys.exit(f"{p}: missing")
        with open(p) as fh:
            return json.load(fh)

    return {"general": read("general"), "racial": read("racial"), "archetype": read("archetype"),
            "classes": [read(f) for f in CLASS_FILES]}


def is_were(name):
    return re.search(r"\bwere\b|were-?hunter|were-?beast", name or "", re.I) is not None


def group_races(name):
    """The races of a racial.json group (a name or a list of names)."""
    names = name if isinstance(name, list) else [name]
    out = 0
    for n in names:
        n = re.sub(r"^human\b.*", "human", str(n).strip().lower())
        if n not in RACES:
            sys.exit(f"racial.json: unknown race {n!r}")
        out |= 1 << RACES[n]
    return out


def master_class_name(category, rank_index, mastery, name):
    """A rank's name: a Master Class's first rank is the class itself, named
    bare ("Spellbinder 1" is "Spellbinder"; F3 20261026-135728, jib); any
    other rank as the list writes it."""
    mastery, name = mastery.strip(), name.strip()
    if category == 4 and rank_index == 0 and name == f"{mastery} 1":
        return mastery
    return name


def ranks(category, classes, races, base, cms, skipped):
    """The rows of one group's masteries."""
    out = []
    for mi, cm in enumerate(cms):
        if is_were(cm["name"]):
            skipped[f"{cm['name']} (a were mastery)"] += len(cm.get("ranks", []))
            continue
        parent, level = 0, 1
        desc = (cm.get("description") or "").strip()
        for ri, r in enumerate(cm.get("ranks", [])):
            if ri > 9:
                sys.exit(f"{cm['name']}: more than ten ranks, past its ids")
            req = r.get("requirements") or {}
            eff = r.get("effects") or {}
            rid = base + 10 * mi + ri + 1
            lvl = req.get("level")
            note = []
            if lvl is None:
                note.append(f"no level in the list: {level}, the rank before it's" if ri else "no level in the list: 1")
            else:
                level = int(lvl)
            text = " ".join([desc] + [n.strip() for n in r.get("notes") or [] if n.strip()]).strip()
            name = master_class_name(category, ri, cm["name"], r["name"])
            out.append(dict(
                id=rid, category=category, classes=classes, races=races, mastery=cm["name"].strip(),
                rank=ri + 1, name=name, parent=parent, cost=int(r.get("cmCost") or 0),
                level=level, spent=int(req.get("spent") or 0),
                minstats=",".join(f"{k}:{v}" for k, v in (req.get("stats") or {}).items()),
                effects=",".join(f"{k}:{v}" for k, v in (eff.get("stats") or {}).items()),
                need=list(req.get("requiredAbilities") or []), add=list(eff.get("addAbilities") or []),
                remove=list(eff.get("removeAbilities") or []), description=text, note="; ".join(note)))
            parent = rid
    return out


def plan_rows(data, skipped):
    """Every rank as a row, abilities still by name."""
    rows = []
    for g in data["general"]["groups"]:
        rows += ranks(0, ALL_CLASSES, ALL_RACES, 0, g["cms"], skipped)
    for gi, g in enumerate(data["racial"]["groups"]):
        rows += ranks(1, ALL_CLASSES, group_races(g["name"]), 1000 + 100 * gi, g["cms"], skipped)
    types = {}
    for c, d in enumerate(data["classes"]):
        types.setdefault(str(d.get("type", "")).strip().lower(), 0)
        types[str(d.get("type", "")).strip().lower()] |= 1 << c
    for gi, g in enumerate(data["archetype"]["groups"]):
        arch = str(g["name"]).strip().lower()
        if arch not in ARCHETYPES or not types.get(arch):
            sys.exit(f"archetype.json: no class is of type {g['name']!r}")
        rows += ranks(2, types[arch], ALL_RACES, 2000 + 200 * gi, g["cms"], skipped)
    for c, d in enumerate(data["classes"]):
        for g in d["groups"]:
            kind = str(g["name"]).strip().lower()
            if kind == "class masteries":
                rows += ranks(3, 1 << c, ALL_RACES, 3000 + 300 * c, g["cms"], skipped)
            elif kind == "master classes":
                rows += ranks(4, 1 << c, ALL_RACES, 3000 + 300 * c + 200, g["cms"], skipped)
            else:
                skipped[f"{CLASS_FILES[c]}.json group {g['name']!r}"] += sum(len(m.get("ranks", [])) for m in g["cms"])
    ids = collections.Counter(r["id"] for r in rows)
    if any(n > 1 for n in ids.values()):
        sys.exit(f"two ranks with one id: {[i for i, n in ids.items() if n > 1][:5]}")
    return rows


def mastery_source(src):
    """Whether a classspells source is a mastery's, an epic's or a master
    class's (database.GotBy)."""
    w = (src or "").lower().replace("eqoa-data", " ").split()
    return bool(w) and w[0] in ("cm", "mastery", "epic", "mca", "master")


def loose(name):
    return re.sub(r"[^a-z0-9]", "", spell_name(name))


class Spells:
    """The server's spells by name, and which classes hold which."""

    def __init__(self, db):
        self.rows = {}  # spellid -> (level, name)
        self.by_key = collections.defaultdict(list)
        self.by_loose = collections.defaultdict(list)
        for sid, lvl, name in db.execute("SELECT spellid, abilitylvl, spellname FROM spellpattern"):
            self.rows[sid] = (lvl, name)
            self.by_key[spell_name(name)].append(sid)
            self.by_loose[loose(name)].append(sid)
        self.of_class = collections.defaultdict(dict)  # class -> spellid -> source
        for cls, sid, src in db.execute("SELECT class, spellid, COALESCE(source, '') FROM classspells"):
            self.of_class[cls][sid] = src

    def classes_of(self, mask):
        return [c for c in range(15) if mask & (1 << c)]

    def holders(self, sid):
        return [c for c in range(15) if sid in self.of_class[c]]

    def fits(self, sid, mask):
        """Whether a rank of these classes may give (or take, or need) the
        spell: a class's rank its own class's or one no class holds; a rank
        of several classes one no class holds, one two or more hold, or one a
        class holds as a mastery's. The lists name some of another class's
        spells (the general First Aid 3 "Hatred", Hold Breath 3 "Bloodleech":
        a shadow knight's), which no other class could cast."""
        holders = self.holders(sid)
        if not holders:
            return True
        mine = self.classes_of(mask)
        if len(mine) == 1:
            return mine[0] in holders
        return len(holders) > 1 or any(mastery_source(self.of_class[c][sid]) for c in holders)

    def pick(self, ids, mask, level):
        """The likeliest of several spells of one name for a rank of these
        classes: a class's own, then the list's level, then the lowest id."""
        def score(sid):
            mine = [self.of_class[c][sid] for c in self.classes_of(mask) if sid in self.of_class[c]]
            return (0 if mine else 1, abs((self.rows[sid][0] or 0) - (level or 0)), sid)
        return min(ids, key=score)

    def find(self, name, level, mask, master=""):
        """(spellid or 0, how): by name ("exact"), without punctuation and
        spaces ("loose"), a master class's ability by its name and the master
        class's as the spell lists write it ("Muscle Lock Assassin"), one name
        the other with words after it ("Flaming Fury AoE": "words"), or a close
        spelling with the same first letter and number of words ("close")."""
        key = spell_name(name)
        if not key:
            return 0, "none"
        tries = [(key, "exact")]
        if master:
            tries.append((spell_name(f"{name} {master}"), "exact"))
        for k, how in tries:
            ids = self.by_key.get(k)
            if ids:
                return self.pick(ids, mask, level), how
            ids = self.by_loose.get(loose(k))
            if ids:
                return self.pick(ids, mask, level), "loose"
        pool = {}
        for c in self.classes_of(mask):
            for sid in self.of_class[c]:
                pool.setdefault(spell_name(self.rows[sid][1]), []).append(sid)
        if len(key.split()) >= 2:
            words = [k for k in pool if len(k.split()) >= 2 and (k.startswith(key + " ") or key.startswith(k + " "))]
            if words:
                return self.pick([i for k in words for i in pool[k]], mask, level), "words"
        for want, _ in tries:
            for k in difflib.get_close_matches(want, list(pool), n=3, cutoff=0.85):
                if k[0] == want[0] and len(k.split()) == len(want.split()):
                    return self.pick(pool[k], mask, level), "close"
        return 0, "none"


def resolve(rows, spells):
    """The abilities' spells: each row's text and id columns, and the tally."""
    tally = collections.Counter()
    found = []  # (how, rank, list name, server name)
    for r in rows:
        for kind, col, ids in (("need", "requires", "require_spells"), ("add", "adds", "add_spells"),
                               ("remove", "removes", "remove_spells")):
            names, sids = [], []
            for a in r.pop(kind):
                name, level = (a.get("name") or "").strip(), int(a.get("level") or 0)
                if not name:
                    continue
                sid, how = spells.find(name, level, r["classes"], r["mastery"] if r["category"] == 4 else "")
                if sid and not spells.fits(sid, r["classes"]):
                    found.append(("class", r["name"], name, spells.rows[sid][1].strip() + " (" +
                                  ", ".join(CLASS_NAMES[c] for c in spells.holders(sid)) + ")"))
                    sid, how = 0, "class"
                tally[how] += 1
                shown = name
                if sid:
                    shown = spells.rows[sid][1].strip().rstrip("$").strip() or name
                if how not in ("exact", "class"):
                    found.append((how, r["name"], name, shown if sid else ""))
                names.append(f"{shown}@{level}" if level else shown)
                sids.append(str(sid))
            r[col] = "; ".join(names)
            r[ids] = ",".join(sids) if any(s != "0" for s in sids) else ""
    return tally, found


def plan(db, data):
    skipped = collections.Counter()
    rows = plan_rows(data, skipped)
    tally, found = resolve(rows, Spells(db))
    have = {}
    if db.execute("SELECT count(*) FROM sqlite_master WHERE name = 'cmcatalog'").fetchone()[0]:
        have = {i: s for i, s in db.execute("SELECT id, source FROM cmcatalog")}
    write = [r for r in rows if r["id"] not in have]
    kept = [r for r in rows if r["id"] in have]
    return dict(rows=rows, write=write, kept=kept, skipped=skipped, tally=tally, found=found)


def report(p, full=False):
    rows = p["rows"]
    by_cat = collections.Counter(r["category"] for r in rows)
    print(f"{len(rows)} ranks of {len({(r['category'], r['classes'], r['races'], r['mastery']) for r in rows})} masteries: "
          + ", ".join(f"{CATEGORY_NAMES[c]} {by_cat[c]}" for c in range(5)))
    per_class = collections.Counter()
    for r in rows:
        if r["category"] >= 3:
            per_class[CLASS_NAMES[r["classes"].bit_length() - 1]] += 1
    print("  class and master class ranks: " + ", ".join(f"{c} {per_class[c]}" for c in CLASS_NAMES))
    print(f"to write: {len(p['write'])}; already there (kept as they are): {len(p['kept'])}")
    t = p["tally"]
    print(f"abilities: {sum(t.values())} named, {t['exact']} found by name, {t['loose']} by name without "
          f"punctuation, {t['words']} as one name with words after the other, {t['close']} by a close spelling; "
          f"{t['class']} found as another class's spell and left out, {t['none']} not found")
    for how, rank, name, shown in sorted(p["found"]):
        print(f"  {how:5s} {rank}: {name!r}" + (f" -> {shown!r}" if shown else ""))
    for what, n in sorted(p["skipped"].items()):
        print(f"left out: {what}: {n} ranks")
    for r in rows:
        if r["note"]:
            print(f"  {r['name']}: {r['note']}")
    if full:
        for r in rows:
            print(f"{r['id']:5d} {CATEGORY_NAMES[r['category']]:12s} {r['name']:32s} parent {r['parent']:5d} "
                  f"cost {r['cost']:3d} level {r['level']:2d} spent {r['spent']:3d} classes {r['classes']:5d} "
                  f"races {r['races']:4d} | {r['minstats']} | {r['effects']} | +{r['adds']} -{r['removes']} "
                  f"need {r['requires']}")


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-cmseed"
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


COLS = ("id", "category", "classes", "races", "mastery", "rank", "name", "parent", "cost", "level", "spent",
        "minstats", "requires", "require_spells", "effects", "adds", "add_spells", "removes", "remove_spells",
        "description")


def apply(db, p):
    db.executescript(SCHEMA)
    db.execute(LOG_DDL)
    db.execute("BEGIN IMMEDIATE")
    try:
        for r in p["write"]:
            db.execute(f"INSERT INTO cmcatalog ({', '.join(COLS)}, enabled, source) "
                       f"VALUES ({', '.join('?' * len(COLS))}, 1, ?)", [r[c] for c in COLS] + [SOURCE])
            db.execute("INSERT INTO cmcatalog_seed (id, note) VALUES (?, ?)", (r["id"], r["name"]))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db, force):
    if not db.execute("SELECT count(*) FROM sqlite_master WHERE name = 'cmcatalog_seed'").fetchone()[0]:
        sys.exit("no mastery seed to undo here")
    n = db.execute("SELECT count(*) FROM cmcatalog_seed").fetchone()[0]
    held = db.execute("SELECT count(*), count(DISTINCT serverid) FROM charcms WHERE cmid IN "
                      "(SELECT id FROM cmcatalog_seed)").fetchone()
    if held[0] and not force:
        sys.exit(f"{held[1]} characters own {held[0]} of the seeded ranks: --force to take the ranks out of the "
                 f"catalog anyway (their charcms rows stay; #cmreset gives a character's points back)")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute("DELETE FROM cmcatalog WHERE id IN (SELECT id FROM cmcatalog_seed)")
        db.execute("DROP TABLE cmcatalog_seed")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"took back {n} ranks" + (f" ({held[0]} owned by {held[1]} characters stay in charcms)" if held[0] else ""))


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--eqoa-data", help="EQOA-Data's checkout, or its Information/databases/cms directory")
    ap.add_argument("--apply", action="store_true", help="write the ranks not there yet (logged)")
    ap.add_argument("--undo", action="store_true", help="delete the ranks --apply wrote")
    ap.add_argument("--force", action="store_true", help="--undo even when characters own seeded ranks")
    ap.add_argument("--no-backup", action="store_true", help="write without a copy first")
    ap.add_argument("--list", action="store_true", help="list every rank")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, isolation_level=None)
    if a.undo:
        if not a.no_backup:
            print("copy:", backup(db, a.db))
        undo(db, a.force)
        return
    if not a.eqoa_data:
        sys.exit("--eqoa-data is required")
    p = plan(db, load(a.eqoa_data))
    report(p, a.list)
    if not a.apply:
        print("(report only: --apply to write it)")
        return
    if db.execute("SELECT count(*) FROM sqlite_master WHERE name = 'cmcatalog_seed'").fetchone()[0] and \
            db.execute("SELECT count(*) FROM cmcatalog_seed").fetchone()[0]:
        sys.exit("this database already holds a mastery seed; --undo it first")
    if not a.no_backup:
        print("copy:", backup(db, a.db))
    apply(db, p)
    print(f"wrote {len(p['write'])} ranks to {a.db}")


if __name__ == "__main__":
    main()
