#!/usr/bin/env python3
"""A quest's spawned creatures made by the quest alone (the user, 2026-10-30,
playing Neriak's level 10 quest at Jarek N`Toxis's hut: "his creation is
already spawned waiting, but should spawn after I complete the steps to bring
him the crab and fungus cockroach parts. he then makes his creation and thats
when you fight it"; and of its look: "its supposed to look like a steam punk
drider").

**Jarek's creation.** Neriak's four level 10 quests of that name (magician,
necromancer, shadow knight, wizard; EQOA-Data Quests/1-20/neriak-nektulos/
<class>.json, "# Level 10") spawned a fungus cockroach, not hostile, when Jarek
had the parts (step 4's done) and took step 5's mutated shell off ANY fungus
cockroach, while a "Jarek's Creation" stood by the hut all along:
tools/seed_quest_npcs.py put it there for Klick`Anon's rogue quest ("Jarek's
Experiment", which spawned it, not hostile), where FrontiersForge saw one, on a
giant beetle's body. As the content files now have them (tools/quests/data/
neriak-nektulos.json, klick-anon.json), step 4 of all five spawns Jarek's
Creation hostile, the four's step 5 loots it, and the four's notes say so. Its
original gets the body the user chose for it ("yeah deidre"): 0x15764DA8, the
NPC Deidre IV's, a dark elf's body on the Tiny Spiderbot's copper spring legs,
at size 1.0 (the beetle's was 0x4140233B at 0.875); --creation-model and
--creation-size give another. Its level stays.

**Spawned only.** A spawn event copies a LIVE NPC of its name (the server's
entity/devspawn.go), so every creature a quest spawns had to stand somewhere in
the world, to be met and killed by anyone, beside the copies the quest made.
Table npcspawnonly (entity/spawnonly.go) keeps the NPCs of a name out of the
world, there only to be copied, and a player who lost a step's spawn of one
gets it again near where it stands (handlers/questevents.go). This tool surveys
every NPC a quest step's spawn event names, with the evidence (--tsv writes it
all), and marks a name when:
- every npcs row of it was put in the world by tools/seed_quest_npcs.py for the
  quests (quest_npc_seed_rows: where FrontiersForge saw it, EQOA-Data's map
  point, beside the NPC the guide names the place by, or the guide's place);
- every quest step naming it is one a spawn of it brings (the step's start, or
  the step before's done), and none talks to it, gives a quest from it, kills
  or loots it without a spawn, or counts one npcs row of it alone (npcid, which
  a copy never is): any of those needs it standing;
- no row of it carries the NPC bit (players talk to it), walks a GM's path, or
  anchors an XP camp (#tele; internal/handlers/camps.go beside the tools).
Any other is left standing and said why: one from the world's own data (Return
Home's import, our own rows) the user decides on; a name with a row already is
the GM's. Each mark is a row of source questspawn, its note the quest it is for.

Every change is logged in quest_spawn_fix_log; --undo puts back each one still
as written (a value changed since is left and said), deletes npcspawnonly when
this made it and it is empty again, and drops the log. Report only by default.
The quest engine reads the quests at its start and when the GM panel saves one,
npcspawnonly is read at the start and at a panel edit of it, and an npcs row at
the start or the panel's Reload every NPC: apply before the server's (re)start.
A server older than npcspawnonly ignores the table, and the quests' changes run
on it as they are.

    python3 -E -s tools/fix_quest_spawns.py --db eqoa.db [--tsv survey.tsv] [--apply | --undo]
        [--creation-model 0x15764DA8] [--creation-size 1.0]
"""
import argparse
import collections
import datetime
import json
import os
import re
import shutil
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
DEFAULT_DB = os.path.join(HERE, "..", "eqoa.db")
CONTENT = os.path.join(HERE, "quests", "data")
CAMPS = os.path.join(HERE, "..", "internal", "handlers", "camps.go")

LOG = "quest_spawn_fix_log"
LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY,
    action  TEXT NOT NULL,             -- quest | body | spawnonly | table
    questid INTEGER NOT NULL DEFAULT 0,
    step    INTEGER NOT NULL DEFAULT 0, -- quest: 0 for the quests row
    name    TEXT NOT NULL DEFAULT '',  -- quest: the column; body: the npcs id; spawnonly: the name
    old     TEXT,                      -- the value before; NULL for none
    new     TEXT,                      -- the value written
    applied TEXT NOT NULL
)"""
# as the server makes it (internal/database/npcspawnonly.go)
SPAWNONLY_DDL = """CREATE TABLE IF NOT EXISTS npcspawnonly (
		name   TEXT PRIMARY KEY,
		note   TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm'
	)"""
SOURCE = "questspawn"

CREATION = "Jarek's Creation"
CREATION_MODEL = 0x15764DA8  # Deidre IV's body (the user: "yeah deidre")
CREATION_SIZE = 1.0
NERIAK = ["1-20/neriak-nektulos/magician/L10", "1-20/neriak-nektulos/necromancer/L10",
          "1-20/neriak-nektulos/shadowknight/L10", "1-20/neriak-nektulos/wizard/L10"]
ROGUE = "1-20/klick-anon/rogue/L10"
FILES = dict({k: "neriak-nektulos.json" for k in NERIAK}, **{ROGUE: "klick-anon.json"})
# what the import wrote before the content changed, the only values changed here
OLD_SPAWN = '[{"on":"done","do":"spawn","npc":"fungus cockroach"}]'
OLD_TARGET = "fungus cockroach"
OLD_ROGUE_SPAWN = '[{"on":"done","do":"spawn","npc":"Jarek\'s Creation"}]'
OLD_NOTE = "The guide's creation has no name; it is a fungus cockroach made beside the player."
NEW_NOTE = ("The guide's creation is Jarek's Creation, as Klick`Anon's rogue guide names it (\"Jarek uses the items to "
            "create a giant insect, which kills him and attacks you\"): once Jarek has the parts it is made beside the "
            "player and attacks at once (it leaves Jarek alive here, for the next player); it stands nowhere else "
            "(npcspawnonly, tools/fix_quest_spawns.py).")


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def key(name):
    """A name as the server matches a spawn's and a step's: trimmed, any case."""
    return (name or "").strip().lower()


def spawnonly_key(name):
    """A name as npcspawnonly keys it (database.NPCClassKey: and no trailing '$')."""
    k = key(name)
    return (k[:-1] if k.endswith("$") else k).strip()


def events(text):
    """A step's events column, parsed (nothing for none or a broken one)."""
    if not text or not text.strip():
        return []
    try:
        v = json.loads(text)
    except ValueError:
        return []
    return [e for e in v if isinstance(e, dict)] if isinstance(v, list) else []


def compact(evs):
    """Events as the importer writes the column (tools/import_quests.py)."""
    return json.dumps(evs, separators=(",", ":")) if evs else ""


# ---- the quests


def content(keys, content_dir):
    """The quests of keys as the content files have them, by key."""
    out, files = {}, {}
    for k in keys:
        f = FILES[k]
        if f not in files:
            with open(os.path.join(content_dir, f), encoding="utf-8") as fh:
                files[f] = {q["key"]: q for q in json.load(fh)["quests"]}
        if k not in files[f]:
            sys.exit(f"{f} has no quest {k}: the content is not this tool's")
        out[k] = files[f][k]
    return out


def quest_changes(db, content_dir):
    """The quests' values to change to the content's: [(qid, step, column,
    old, new, key)], and what is left and why: [line]. A value already the
    content's is no change; one neither the import's old one nor the content's
    is a GM's, left."""
    if not has_table(db, "quest_import_quests"):
        return [], ["no quest_import_quests: these quests were not imported here"]
    ids = dict(db.execute("SELECT key, questid FROM quest_import_quests"))
    want = content(NERIAK + [ROGUE], content_dir)
    out, left = [], []

    def step_value(qid, n, col):
        r = db.execute(f"SELECT {col} FROM queststeps WHERE questid = ? AND step = ?", (qid, n)).fetchone()
        return None if r is None else (r[0] or "")

    for k in NERIAK + [ROGUE]:
        qid = ids.get(k)
        if qid is None:
            left.append(f"{k}: not imported here")
            continue
        q = want[k]
        olds = [(4, "events", OLD_ROGUE_SPAWN if k == ROGUE else OLD_SPAWN, compact(q["steps"][3].get("events", [])))]
        if k != ROGUE:
            olds.append((5, "target", OLD_TARGET, q["steps"][4]["target"]))
        for n, col, old, new in olds:
            cur = step_value(qid, n, col)
            if cur == new:
                continue
            if cur != old:
                left.append(f"{k} ({qid}) step {n} {col}: {cur!r} is neither the import's nor the content's: left")
                continue
            out.append((qid, n, col, old, new, k))
        if k != ROGUE:
            r = db.execute("SELECT notes FROM quests WHERE questid = ?", (qid,)).fetchone()
            cur = "" if r is None else (r[0] or "")
            if cur != q.get("notes", "") and OLD_NOTE in cur and cur.replace(OLD_NOTE, NEW_NOTE) == q.get("notes", ""):
                out.append((qid, 0, "notes", cur, q["notes"], k))
            elif cur != q.get("notes", ""):
                left.append(f"{k} ({qid}) notes: not the import's: left")
    return out, left


# ---- the creation's body


def body_changes(db, model, size):
    """The creation's rows to give the body: [(id, old, new)], old and new
    {"modelid", "size"}."""
    out = []
    want = {"modelid": model, "size": size}
    for i, name, m, s in db.execute("SELECT id, npc_name, modelid, size FROM npcs ORDER BY id"):
        if key(name) != key(CREATION):
            continue
        have = {"modelid": m, "size": s}
        if have != want:
            out.append((i, have, want))
    return out


# ---- the survey


def origins(db):
    """Where each npcs row came from, by id, for those an import or a seed
    logged: (what, detail)."""
    out = {}
    if has_table(db, "prh_import_npcs"):
        for dst, status in db.execute("SELECT dst_id, status FROM prh_import_npcs WHERE dst_id IS NOT NULL"):
            out[dst] = ("Return Home's import" if status == "imported" else "our own rows (Return Home's too)", status)
    if has_table(db, "npcs_ff_import"):
        for (i,) in db.execute("SELECT promoted_id FROM npcs_ff_import WHERE promoted_id IS NOT NULL"):
            out[i] = ("FrontiersForge's import, promoted", "")
    for table, what in (("pod_seed_npcs", "the Plane of Disease's seed"), ("wildlife_seed_npcs", "the wildlife seed")):
        if has_table(db, table):
            for (i,) in db.execute(f"SELECT npc_row FROM {table}"):
                out[i] = (what, "")
    if has_table(db, "quest_npc_seed_rows"):
        for i, src, seedset in db.execute("SELECT npc_row, source, seedset FROM quest_npc_seed_rows"):
            out[i] = ("seed", f"{src} [{seedset}]")
    return out


def camp_anchors(path):
    """The #tele camps' anchor names (internal/handlers/camps.go), or None when
    the file is not beside the tools."""
    try:
        with open(path, encoding="utf-8") as fh:
            return {key(n) for n in re.findall(r'NPC: "([^"]*)"', fh.read())}
    except OSError:
        return None


def survey(db, changes, camps):
    """Every NPC a quest step's spawn event names that stands in the world, as
    the quests are once changes are written: [dict] with the evidence, the
    verdict (mark, leave, marked, none) and why."""
    quests = {q: dict(name=n, enabled=e, giver=g or "") for q, n, e, g in
              db.execute("SELECT questid, name, enabled, giver FROM quests")}
    steps = collections.defaultdict(dict)
    for q, n, kind, target, npcid, evs, journal in db.execute(
            "SELECT questid, step, kind, target, npcid, events, journal FROM queststeps"):
        steps[q][n] = dict(kind=kind, target=target or "", npcid=npcid or 0, events=evs or "", journal=journal or "")
    for q, n, col, old, new, _ in changes:
        if n and q in steps and n in steps[q]:
            steps[q][n][col] = new
    also = collections.defaultdict(list)
    if has_table(db, "queststeptargets"):
        for q, n, t in db.execute("SELECT questid, step, target FROM queststeptargets"):
            also[(q, n)].append(t)
    rows = collections.defaultdict(list)
    for i, name, world, x, z, lv, t in db.execute("SELECT id, npc_name, world, x_coord, z_coord, npc_level, npc_type FROM npcs"):
        rows[key(name)].append(dict(id=i, name=name or "", world=world, x=x or 0, z=z or 0, level=lv, type=t or 0))
    came = origins(db)
    pathed = set()
    if has_table(db, "npcpaths"):
        pathed = {i for (i,) in db.execute("SELECT npcid FROM npcpaths")}
    marked = {}
    if has_table(db, "npcspawnonly"):
        marked = {spawnonly_key(n): s for n, s in db.execute("SELECT name, source FROM npcspawnonly")}

    def brings(q, n, name):
        """How step n of quest q gets name: its start's spawn, the step before's done."""
        out = []
        st = steps[q].get(n)
        if st:
            out += ["its start" for e in events(st["events"]) if e.get("do") == "spawn" and e.get("on") == "start"
                    and key(e.get("npc")) == name]
        before = [m for m in steps[q] if m < n]
        if before:
            out += ["the step before's done" for e in events(steps[q][max(before)]["events"])
                    if e.get("do") == "spawn" and e.get("on") == "done" and key(e.get("npc")) == name]
        return out

    spawns = collections.defaultdict(list)
    for q in sorted(steps):
        for n in sorted(steps[q]):
            for e in events(steps[q][n]["events"]):
                if e.get("do") == "spawn" and key(e.get("npc")):
                    spawns[key(e.get("npc"))].append((q, n, e.get("on", ""), bool(e.get("hostile"))))
    out = []
    for name in sorted(spawns):
        rs = rows.get(name, [])
        spelled = collections.Counter(r["name"].strip() for r in rs).most_common(1)[0][0] if rs else \
            next(e.get("npc") for q, n, _, _ in spawns[name][:1] for e in events(steps[q][n]["events"])
                 if key(e.get("npc")) == name)
        named, why = [], []
        for q in sorted(steps):
            for n in sorted(steps[q]):
                st = steps[q][n]
                hunts = st["kind"] in ("kill", "loot")
                targets = [st["target"]] + (also.get((q, n), []) if hunts else [])
                if not any(key(t) == name for t in targets):
                    continue
                by = brings(q, n, name) if hunts else []
                named.append(dict(quest=q, step=n, kind=st["kind"], by=by, journal=st["journal"], npcid=st["npcid"]))
                if not hunts:
                    why.append(f"quest {q} {quests.get(q, {}).get('name', '')!r} step {n} is a {st['kind']} step at it")
                elif not by:
                    why.append(f"quest {q} {quests.get(q, {}).get('name', '')!r} step {n} {st['kind']}s it with no spawn")
                elif st["npcid"]:
                    why.append(f"quest {q} {quests.get(q, {}).get('name', '')!r} step {n} counts its npcs row "
                               f"{st['npcid']} alone, never a copy")
        givers = [q for q, v in quests.items() if key(v["giver"]) == name]
        for q in givers:
            why.append(f"it gives quest {q} {quests[q]['name']!r}")
        seeds = [came.get(r["id"]) for r in rs]
        foreign = sorted({c[0] if c else "the world's own rows" for c in seeds if not c or c[0] != "seed"})
        if foreign:
            why.insert(0, "stands in the world from " + " and ".join(foreign) + ", not the quest seed")
        if any(r["type"] & 0x80 for r in rs):
            why.append("players talk to it (the NPC bit)")
        if any(r["id"] in pathed for r in rs):
            why.append("it walks a GM's path")
        if camps is not None and name in camps:
            why.append("it anchors an XP camp (#tele)")
        if not rs:
            verdict, why = "none", ["no NPC of the name: the spawn makes nothing"]
        elif spawnonly_key(name) in marked:
            verdict, why = "marked", [f"npcspawnonly has it already (source {marked[spawnonly_key(name)]})"]
        elif why:
            verdict = "leave"
        else:
            verdict = "mark"
        out.append(dict(name=spelled, key=name, verdict=verdict, why=why, rows=rs,
                        seeds=sorted({c[1] for c in seeds if c and c[0] == "seed"}),
                        origins=foreign, spawns=spawns[name], named=named, givers=givers, quests=quests))
    return out


def note_of(e):
    """A mark's note: the quests it is for."""
    qs = []
    for q, n, _, _ in e["spawns"]:
        if (q, n) not in qs:
            qs.append((q, n))
    q, n = qs[0]
    more = f" and {len(qs) - 1} more" if len(qs) > 1 else ""
    return (f"the spawn of quest {q} {e['quests'].get(q, {}).get('name', '')!r} step {n}{more}; "
            f"it stood where tools/seed_quest_npcs.py put it ({', '.join(e['seeds'])})")


def survey_lines(entries):
    """The survey as report lines, the marks first."""
    order = {"mark": 0, "marked": 1, "leave": 2, "none": 3}
    lines = []
    for e in sorted(entries, key=lambda e: (order[e["verdict"]], e["key"])):
        r = e["rows"]
        where = f"{len(r)} row{'s' if len(r) != 1 else ''}"
        if r:
            where += f" (world {r[0]['world']}, {r[0]['x']:.0f}, {r[0]['z']:.0f}; L{r[0]['level']})"
        sp = ", ".join(sorted({f"{q} s{n}" for q, n, _, _ in e["spawns"]}))
        src = ", ".join(e["seeds"] + e["origins"]) or "-"
        lines.append(f"{e['verdict']:6} {e['name']!r}: {where}; from {src}; spawned by {sp}"
                     + (f"; {'; '.join(e['why'])}" if e["why"] else ""))
    return lines


def write_tsv(path, entries):
    cols = ["verdict", "name", "rows", "where", "from", "spawned_by", "steps", "journal", "why"]
    with open(path, "w", encoding="utf-8") as fh:
        fh.write("\t".join(cols) + "\n")
        for e in entries:
            r = e["rows"]
            where = "; ".join(f"{x['id']} w{x['world']} ({x['x']:.0f}, {x['z']:.0f}) L{x['level']} t{x['type']}" for x in r[:6])
            sp = "; ".join(f"{q} {e['quests'].get(q, {}).get('name', '')} s{n} on {on}{' hostile' if h else ''}"
                           for q, n, on, h in e["spawns"])
            st = "; ".join(f"{s['quest']} s{s['step']} {s['kind']}" + (f" ({', '.join(s['by'])})" if s["by"] else "")
                           for s in e["named"])
            jn = " | ".join(dict.fromkeys(s["journal"] for s in e["named"]))
            vals = [e["verdict"], e["name"], str(len(r)), where, ", ".join(e["seeds"] + e["origins"]), sp, st, jn,
                    "; ".join(e["why"])]
            fh.write("\t".join(v.replace("\t", " ").replace("\n", " ") for v in vals) + "\n")


# ---- writing


def apply(db, qchanges, bodies, marks):
    """Write it all, logged, in one transaction; returns the counts."""
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_DDL)

        def log(action, q, n, name, old, new):
            db.execute(f"INSERT INTO {LOG} (action, questid, step, name, old, new, applied) VALUES (?, ?, ?, ?, ?, ?, ?)",
                       (action, q, n, name, old, new, when))

        for q, n, col, old, new, _ in qchanges:
            if n:
                db.execute(f"UPDATE queststeps SET {col} = ? WHERE questid = ? AND step = ?", (new, q, n))
            else:
                db.execute(f"UPDATE quests SET {col} = ? WHERE questid = ?", (new, q))
            log("quest", q, n, col, old, new)
        for i, old, new in bodies:
            db.execute("UPDATE npcs SET modelid = ?, size = ? WHERE id = ?", (new["modelid"], new["size"], i))
            log("body", 0, 0, str(i), json.dumps(old), json.dumps(new))
        if marks:
            if not has_table(db, "npcspawnonly"):
                db.execute(SPAWNONLY_DDL)
                log("table", 0, 0, "npcspawnonly", None, None)
            for name, note in marks:
                db.execute("INSERT INTO npcspawnonly (name, note, source) VALUES (?, ?, ?)", (name, note, SOURCE))
                log("spawnonly", 0, 0, name, None, json.dumps({"note": note, "source": SOURCE}))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return len(qchanges), len(bodies), len(marks)


def undo(db):
    """Put back what apply wrote and is still as written; returns (back,
    left lines)."""
    back, kept = 0, []
    db.execute("BEGIN IMMEDIATE")
    try:
        made = False
        for action, q, n, name, old, new in db.execute(
                f"SELECT action, questid, step, name, old, new FROM {LOG} ORDER BY seq DESC").fetchall():
            if action == "quest":
                if n:
                    cur = db.execute(f"SELECT {name} FROM queststeps WHERE questid = ? AND step = ?", (q, n)).fetchone()
                else:
                    cur = db.execute(f"SELECT {name} FROM quests WHERE questid = ?", (q,)).fetchone()
                if cur is None or (cur[0] or "") != new:
                    kept.append(f"quest {q} step {n} {name}: changed since")
                    continue
                if n:
                    db.execute(f"UPDATE queststeps SET {name} = ? WHERE questid = ? AND step = ?", (old, q, n))
                else:
                    db.execute(f"UPDATE quests SET {name} = ? WHERE questid = ?", (old, q))
            elif action == "body":
                o, w = json.loads(old), json.loads(new)
                cur = db.execute("SELECT modelid, size FROM npcs WHERE id = ?", (int(name),)).fetchone()
                if cur is None or {"modelid": cur[0], "size": cur[1]} != w:
                    kept.append(f"npcs {name}'s body: changed since")
                    continue
                db.execute("UPDATE npcs SET modelid = ?, size = ? WHERE id = ?", (o["modelid"], o["size"], int(name)))
            elif action == "spawnonly":
                w = json.loads(new)
                cur = db.execute("SELECT note, source FROM npcspawnonly WHERE name = ?", (name,)).fetchone() \
                    if has_table(db, "npcspawnonly") else None
                if cur is None or (cur[0], cur[1]) != (w["note"], w["source"]):
                    kept.append(f"npcspawnonly {name!r}: changed since")
                    continue
                db.execute("DELETE FROM npcspawnonly WHERE name = ?", (name,))
            elif action == "table":
                made = True
                continue
            back += 1
        if made and has_table(db, "npcspawnonly"):
            if db.execute("SELECT count(*) FROM npcspawnonly").fetchone()[0] == 0:
                db.execute("DROP TABLE npcspawnonly")
            else:
                kept.append("npcspawnonly: made here, rows in it since: kept")
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return back, kept


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-questspawn"
    shutil.copy2(path, dst)
    return dst


def model_arg(s):
    v = int(s, 0)
    if not 0 < v <= 0xFFFFFFFF:
        raise argparse.ArgumentTypeError("a body is a CHAR.ESF DictID, as 0x15764DA8")
    return v


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--content", default=CONTENT, help="the quest content files (tools/quests/data)")
    ap.add_argument("--camps", default=CAMPS, help="the #tele camps' file (internal/handlers/camps.go)")
    ap.add_argument("--creation-model", type=model_arg, default=CREATION_MODEL,
                    help=f"Jarek's Creation's body, a CHAR.ESF DictID (0x{CREATION_MODEL:08X}, Deidre IV's)")
    ap.add_argument("--creation-size", type=float, default=CREATION_SIZE, help=f"its size ({CREATION_SIZE})")
    ap.add_argument("--tsv", help="write the survey, with its evidence, here")
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    if not 0.1 <= a.creation_size <= 10:
        sys.exit("--creation-size: 0.1 to 10")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        if not has_table(db, LOG):
            print(f"no {LOG}: nothing to undo")
            return
        if not a.no_backup:
            print(f"copy: {backup(a.db)}")
        back, kept = undo(db)
        for k in kept:
            print(f"left: {k}")
        print(f"put back {back}; {len(kept)} changed since, left as they are")
        return
    if a.apply and has_table(db, LOG):
        sys.exit(f"{LOG} is there: applied already (--undo first)")

    qchanges, qleft = quest_changes(db, a.content)
    for q, n, col, old, new, k in qchanges:
        what = "the notes" if not n else f"step {n} {col}"
        print(f"quest {q} ({k}) {what}: {old[:70]!r} -> {new[:90]!r}")
    for ln in qleft:
        print(f"quest left: {ln}")
    bodies = body_changes(db, a.creation_model, a.creation_size)
    for i, old, new in bodies:
        print(f"npcs {i} {CREATION!r}: body 0x{(old['modelid'] or 0) & 0xFFFFFFFF:08X} size {old['size']} -> "
              f"0x{new['modelid']:08X} size {new['size']}")
    camps = camp_anchors(a.camps)
    if camps is None:
        print(f"no {a.camps}: the XP camps' anchors are not looked at")
    entries = survey(db, qchanges, camps)
    for ln in survey_lines(entries):
        print(ln)
    marks = [(e["name"], note_of(e)) for e in entries if e["verdict"] == "mark"]
    counts = collections.Counter(e["verdict"] for e in entries)
    print(f"{len(qchanges)} quest values, {len(bodies)} bodies; of {len(entries)} names quests spawn: "
          f"{counts['mark']} to mark spawned only, {counts['marked']} marked already, {counts['leave']} left standing, "
          f"{counts['none']} with no NPC")
    if a.tsv:
        write_tsv(a.tsv, entries)
        print(f"wrote {a.tsv}")
    if not a.apply:
        print("report only: --apply to write")
        return
    if not (qchanges or bodies or marks):
        print("nothing to write")
        return
    if not a.no_backup:
        print(f"copy: {backup(a.db)}")
    nq, nb, nm = apply(db, qchanges, bodies, marks)
    print(f"wrote {nq} quest values, {nb} bodies, {nm} spawn-only names; the quests are the engine's at its next "
          f"(re)start or a quest saved in the GM panel; the rest at the next start")


if __name__ == "__main__":
    main()
