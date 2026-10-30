#!/usr/bin/env python3
"""SOE's creatures as the user remembers them (2026-10-30): "SOE had most mobs as warrior class, but stripped down
to just a 'crit' attack once every 30 secs or so. that was the basic roamer. other than wisps were wizards."

Writes a class template and the npctemplate rows that give it (entity/classtemplate.go, database/classtemplates.go):

  roamer    classtemplates' row (if none is there): a warrior (class 0) at 100% hit points, 100% of a melee hit
            taken and 100% of its own melee, whose one special is a Critical Hit every 25-35 s (special_every 30,
            special_jitter 5), the first 10-20 s into a fight (special_first 15), for 7.4 a level, fixed and through
            any armour, with Sandstorm's creature special's visual (special_fx 0: 0x66CD778E), from level 10 on
            (special_from 10, the creature's own level at each fight: the user's "they get it around level 10
            starting, but before only auto attack"; below it a roamer fights with its melee alone). The 7.4 is
            Sandstorm's one measured creature special (a level 34 Desert Crab's 251 and a level 35 Gharial's 252,
            through a tank's 1,763 AC and an enchanter's 167 alike: claude-notes/training-dummies-20261021.md);
            the 30 s or so is the user's. Named as the melee template's Kick is ("Critical Hit"), for #stats.
  roamer    in npctemplate for every hostile creature's name whose class (npcclass) is a warrior's or which has
            none: a name at least one of whose spawns has no NPC bit (0x80) and a body no player race has
            (charactermodel), and none of whose spawns wears a player race's body (townsfolk, guards, bandits).
            Named creatures and bosses alike: the template changes no hit points and no melee, and adds nothing a
            boss's loot table (npcloot) reads; a boss in a player race's body (Bertoxxulous the Plaguebringer) is
            left out with the townsfolk, and said.
  wisp      classtemplates' row (if none is there): a copy of jib's wizard as the table has it (class 13, 85% hit
            points, an area spell of the wizard's every 15 s and a single target's after it) with both abilities'
            figures at 28% of a player's where his are 35%: the user's "28%" of 2026-10-30, after the simulator
            found a rogue of 35-45 and a cleric of 35 lost every wisp yellow at 35% and every class won them at 28%
            (a rogue of 35 taken down to 12%). The wizard row itself is left as he made it.
  wisp      in npctemplate for every wisp (the user's word, "other than wisps were wizards", over Sandstorm's crag
            wisp of class 10): a name whose last word is lower case and ends in "wisp" (crag wisp, malign wisp,
            darkwisp, will o'wisp, ...; not Thal Twisp or Overseer Golwisp), hostile by the same test.

Never a training dummy (a name with "dummy": they fight their own way), never a name npctemplate already gives a
template (a GM's, or a row this wrote before: said, left as it is); a pet or a charmed creature never fights by a
template whatever its name (templateForLocked). Every row it writes (source roamer-seed) is logged in
npc_template_seed_log (the table, the row's key, the row before and after: NULL for one added, why); --undo
deletes each still as written (one a GM changed since is kept, and said) and the roamer and wisp rows with them
once no npctemplate row names them any more, and drops the log; a second --apply only adds what is still to do. Both copy
the database first (<db>.bak-<when>-pre-roamers) unless --no-backup. Report only by default: the names by kind
(boss, named, generic, wisp) and by level band (the highest level of a name's spawns).

The server reads both tables at its start and again after a GM's edit of either in the panel (or its reload_npcs):
after --apply, the next start (or such an edit) gives every NPC of a name its template where it stands.

    tools/seed_roamers.py [--db eqoa.db] [--apply | --undo] [--no-backup] [--list]
"""
import argparse
import datetime
import json
import os
import re
import shutil
import sqlite3
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
SOURCE = "roamer-seed"
LOG = "npc_template_seed_log"
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,   -- classtemplates | npctemplate
    place   TEXT NOT NULL,   -- the row's name
    before  TEXT,            -- the whole row before, JSON; NULL: added
    after   TEXT,            -- the whole row after, JSON
    why     TEXT NOT NULL,
    applied TEXT NOT NULL
)"""
ROAMER = {
    "name": "roamer", "class": 0, "hp_pct": 100, "taken_pct": 100, "melee_pct": 100,
    "special": "Critical Hit", "special_chance": 0, "special_damage": 7.4, "special_fx": 0,
    "special_every": 30.0, "special_first": 15.0, "special_jitter": 5.0, "special_from": 10, "abilities": "[]",
    "note": "SOE's basic roamer (the user, 2026-10-30): a warrior \"stripped down to just a 'crit' attack once every 30 "
            "secs or so\", which \"they get around level 10 starting, but before only auto attack\": from level 10 a "
            "Critical Hit every 25-35 s from 10-20 s into a fight, 7.4 a level, fixed and through any armour (Sandstorm's "
            "crab's 251 at 34); tools/seed_roamers.py",
    "source": SOURCE,
}
WIZARD = "wizard"
WISP = "wisp"
WISP_PCT = 28  # the user's "28%" (2026-10-30): both of the wizard's abilities' damage_pct, his 35
WHY = "SOE's roamers and wisps (the user, 2026-10-30; tools/seed_roamers.py)"
PODBOSSES = ("noroxus", "bertoxxulous the plaguebringer", "poxathixius", "uzrathixius", "patron of bertoxxulous")
BANDS = [(1, 10), (11, 20), (21, 30), (31, 40), (41, 50), (51, 60), (61, 999)]


def key(name):
    """database.NPCClassKey: lower case, trimmed, no trailing $."""
    return name.strip().lower().removesuffix("$").strip()


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def is_wisp(name):
    last = re.split(r"\s+", name.strip())[-1] if name.strip() else ""
    return last == last.lower() and last.endswith("wisp")


def is_named(name):
    """An individual: every word of its name begins with a capital (Lady Vox, Noroxus); a generic creature has a
    lower case word (moss snake, Deathfist reaver, a decaying skeleton)."""
    words = [w for w in re.split(r"[\s\-]+", name.strip()) if w]
    return bool(words) and all(w[0].isupper() for w in words if w[0].isalpha())


def band(level):
    for lo, hi in BANDS:
        if lo <= level <= hi:
            return f"{lo}-{hi}" if hi < 999 else f"{lo}+"
    return "?"


def classify(db):
    """Every NPC name's verdict: (key, display name, kind, template or None, why, top level)."""
    bodies = {m & 0xFFFFFFFF for (m,) in db.execute("SELECT modelid FROM charactermodel")} if has_table(db, "charactermodel") else set()
    classes = {k: c for k, c in db.execute("SELECT name, class FROM npcclass")} if has_table(db, "npcclass") else {}
    taken = {key(n): t for n, t in db.execute("SELECT name, template FROM npctemplate")} if has_table(db, "npctemplate") else {}
    bosses = set(PODBOSSES)
    if has_table(db, "npcloot"):
        bosses |= {key(n) for (n,) in db.execute("SELECT npc_name FROM npcloot WHERE lower(tablename) LIKE 'boss %'")}
    names = {}
    for name, level, flags, model in db.execute("SELECT npc_name, npc_level, npc_type, modelid FROM npcs WHERE npc_name IS NOT NULL"):
        k = key(name)
        if not k:
            continue
        e = names.setdefault(k, {"name": name.strip(), "top": 0, "hostile": 0, "rows": 0, "racebody": 0, "npcbit": 0})
        e["rows"] += 1
        m = (model or 0) & 0xFFFFFFFF
        if m and m in bodies:
            e["racebody"] += 1
        if (flags or 0) & 0x80:
            e["npcbit"] += 1
        elif m and m not in bodies:
            e["hostile"] += 1
            e["top"] = max(e["top"], level or 0)
    out = []
    for k, e in sorted(names.items()):
        name = e["name"]
        kind = "boss" if k in bosses else ("wisp" if is_wisp(name) else ("named" if is_named(name) else "generic"))
        why, tmpl = "", None
        cls = classes.get(k)
        if "dummy" in k:
            why = "a training dummy"
        elif e["racebody"]:
            why = "a player race's body (townsfolk, guards, bandits)"
        elif not e["hostile"]:
            why = "no hostile spawn (the NPC bit on all, or no body)"
        elif kind == "wisp":
            tmpl = WISP
        elif cls in (None, 0):
            tmpl = ROAMER["name"]
        else:
            why = f"its class is {cls}, not a warrior's"
        if tmpl and k in taken:
            why = f"npctemplate gives it {taken[k]!r} already" + ("" if taken[k] == tmpl else ": a GM's")
            tmpl = None
        out.append((k, name, kind, tmpl, why, e["top"], e["npcbit"]))
    return out


def row_of(db, tbl, name):
    cur = db.execute(f"SELECT * FROM {tbl} WHERE name = ?", (name,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def wisp_row(wizard):
    """the wisp template: the wizard row as the table has it, each of its abilities' damage_pct WISP_PCT"""
    row = {c: v for c, v in wizard.items() if c in ROAMER}
    abilities = json.loads(row.get("abilities") or "[]")
    for a in abilities:
        a["damage_pct"] = WISP_PCT
    row.update(name=WISP, abilities=json.dumps(abilities, separators=(",", ":")), source=SOURCE,
               note=f"the user's wisp (2026-10-30, \"28%\"): jib's wizard with its abilities at {WISP_PCT}% of a player's "
                    "figures where his are 35% (at 35% a rogue of 35-45 and a cleric of 35 lost every wisp yellow); "
                    "tools/seed_roamers.py")
    return row


def plan(db):
    """The rows to add [(table, row)] and the verdicts."""
    for t in ("npcs", "classtemplates", "npctemplate"):
        if not has_table(db, t):
            sys.exit(f"no table {t}: a server of 2026-10-29 or later makes it")
    verdicts = classify(db)
    adds = []
    if row_of(db, "classtemplates", ROAMER["name"]) is None:
        adds.append(("classtemplates", dict(ROAMER)))
    if row_of(db, "classtemplates", WISP) is None and any(v[3] == WISP for v in verdicts):
        wizard = row_of(db, "classtemplates", WIZARD)
        if wizard is None:
            sys.exit("classtemplates has no wizard row (jib's seed) to copy the wisp from: a GM took it out; put one back first")
        adds.append(("classtemplates", wisp_row(wizard)))
    for k, name, kind, tmpl, why, top, npcbit in verdicts:
        if tmpl:
            adds.append(("npctemplate", {"name": k, "template": tmpl, "note": f"{kind}, level {top}", "source": SOURCE}))
    return adds, verdicts


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst, n = f"{path}.bak-{when}-pre-roamers", 1
    while os.path.exists(dst):
        n += 1
        dst = f"{path}.bak-{when}-{n}-pre-roamers"
    shutil.copy2(path, dst)
    return dst


# classtemplates' timed special columns as a server of 2026-10-30 adds them (database.classTemplateColumns): added
# here when missing, so the order of this and the server's start does not matter; --undo leaves them (at 0 they
# give no template a clock, and an older server reads past them)
TIMED_COLUMNS = (("special_every", "REAL NOT NULL DEFAULT 0"), ("special_first", "REAL NOT NULL DEFAULT 0"),
                 ("special_jitter", "REAL NOT NULL DEFAULT 0"), ("special_from", "INTEGER NOT NULL DEFAULT 0"))


def missing_columns(db):
    have = {r[1] for r in db.execute("PRAGMA table_info(classtemplates)")}
    return [(c, d) for c, d in TIMED_COLUMNS if c not in have]


def apply(db, adds):
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        for col, decl in missing_columns(db):
            db.execute(f"ALTER TABLE classtemplates ADD COLUMN {col} {decl}")
        db.execute(LOG_SCHEMA)
        for tbl, row in adds:
            cols = list(row)
            db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [row[c] for c in cols])
            db.execute(f"INSERT INTO {LOG} (tbl, place, before, after, why, applied) VALUES (?, ?, NULL, ?, ?, ?)",
                       (tbl, row["name"], json.dumps(row_of(db, tbl, row["name"]), sort_keys=True), WHY, when))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db):
    if not has_table(db, LOG):
        print("nothing logged: nothing to undo")
        return
    logged = db.execute(f"SELECT seq, tbl, place, after FROM {LOG} ORDER BY seq DESC").fetchall()
    back, kept = 0, 0
    db.execute("BEGIN IMMEDIATE")
    try:
        for seq, tbl, place, after in logged:
            if tbl not in ("classtemplates", "npctemplate"):
                raise SystemExit(f"{LOG} names table {tbl!r}, which this tool never writes")
            now = row_of(db, tbl, place)
            if now != json.loads(after):
                print(f"  {tbl} {place!r}: changed since: left as it is")
                kept += 1
                continue
            if tbl == "classtemplates" and db.execute("SELECT count(*) FROM npctemplate WHERE template = ?", (place,)).fetchone()[0]:
                print(f"  classtemplates {place!r}: npctemplate rows still name it: left as it is")
                kept += 1
                continue
            db.execute(f"DELETE FROM {tbl} WHERE name = ?", (place,))
            back += 1
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"took out {back} of {len(logged)} rows ({kept} left); the server drops them at its next start or a template edit")


def report(verdicts, adds, show):
    counts = {}
    for k, name, kind, tmpl, why, top, npcbit in verdicts:
        if tmpl:
            counts.setdefault((tmpl, kind), {}).setdefault(band(top), 0)
            counts[(tmpl, kind)][band(top)] += 1
    labels = [f"{lo}-{hi}" if hi < 999 else f"{lo}+" for lo, hi in BANDS]
    print(f"{'template / kind':24s} " + " ".join(f"{b:>6s}" for b in labels) + "   all")
    for (tmpl, kind), by in sorted(counts.items()):
        print(f"{tmpl + ' / ' + kind:24s} " + " ".join(f"{by.get(b, 0):6d}" for b in labels) + f" {sum(by.values()):5d}")
    left = {}
    for v in verdicts:
        if not v[3] and v[4]:
            left.setdefault(v[4].split(":")[0], []).append(v)
    for why, vs in sorted(left.items(), key=lambda kv: -len(kv[1])):
        print(f"left: {len(vs):5d} {why}")
    bosses = sorted((v for v in verdicts if v[2] == "boss"), key=lambda v: v[0])
    print("bosses: " + "; ".join(f"{v[1]} ({v[3] or 'left: ' + v[4]})" for v in bosses))
    mixed = [v for v in verdicts if v[3] and v[6]]
    if mixed:
        print(f"{len(mixed)} names given a template have some spawns with the NPC bit (they never fight): "
              + ", ".join(v[1] for v in mixed[:12]) + ("..." if len(mixed) > 12 else ""))
    if show:
        for v in verdicts:
            if v[3]:
                print(f"  {v[3]:8s} {v[2]:8s} L{v[5]:3d} {v[1]}")
    for tbl, row in adds:
        if tbl == "classtemplates":
            print(f"classtemplates {row['name']!r}: class {row['class']}, hit points {row['hp_pct']}%, abilities {row['abilities']}")
    print(f"{len(adds)} rows to add")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    ap.add_argument("--list", action="store_true", help="name every NPC given a template")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the rows (logged)")
    g.add_argument("--undo", action="store_true", help="take out what --apply wrote")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        if not a.no_backup and has_table(db, LOG):
            print(f"copy: {backup(a.db)}")
        undo(db)
        return
    adds, verdicts = plan(db)
    report(verdicts, adds, a.list)
    if missing_columns(db):
        print("classtemplates' timed special columns are added with them (as a server of 2026-10-30 adds them)")
    if not a.apply or not adds:
        if not a.apply:
            print("report only: --apply to write")
        return
    if not a.no_backup:
        print(f"copy: {backup(a.db)}")
    apply(db, adds)
    print(f"added {len(adds)} rows: the server gives the templates at its next start (or a template edit in the panel)")


if __name__ == "__main__":
    main()
