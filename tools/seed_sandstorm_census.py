#!/usr/bin/env python3
"""Creatures Sandstorm shows where this server has none: a census flown over
a world by the headless harvester, each creature it saw that has no namesake
of ours near its place made a row of npcs (the user, 2026-10-31: "we can map
out our missing from sandstorm", after the population tool's guesses for
Rathe's west, cmd/wildspread's made kinds, met their zones' real creatures:
"envar, dead hills have different mobs, undead", "geomancers citadel has the
archon bosses ...").

The census: eqoa-rust-headless's `eqoa harvest` on the user's named
character, `track on` through a route of `goto` steps, a run per zone
(cmd/sandroute -zones -below 12: rows 220 apart, a stop every 50, 12 under
the lowest floor near each leg, where nothing reaches it and it is level
with what walks there: Sandstorm shows the creatures within about 170 of
the client, height counted, and saw twice as many from under Ogre Ruins as
from 60 over them; never under the top of any water, where Sandstorm
drowns a player even under the sea floor), its `<out>.track.jsonl` files: a
`who` line the first time a creature is seen (its body, size, level, class,
race, npc flags, the items it holds, its armour, hair, face and robe) and
its place every half second.

What it decides, and from what:

- A creature is one the census saw with an id under PLAYER_IDS (Sandstorm's
  NPCs' ids; its players' are in the hundreds of millions). Its place is the
  median of the places it was seen at (a wanderer is seen where it walked),
  its facing its last heading.
- It is ours already when a row of npcs of its name (any case, the '$' mark
  dropped) stands within MATCH of its place in its world, each of our rows
  taken once, the nearest first. The rest are missing.
- Only creatures a player fights are made (no NPC bit, 0x80, in its flags),
  and only a kind's: a name two or more of ours or of the census carry, or one
  fix_npc_name_case.py's rules read generic. A named creature is listed, not
  made (a bare row would have none of its words or quests), as is one whose
  name we have elsewhere in the world; a summoned pet (a spell's name: the
  hobgoblin mystics' Feral Spirits) is left out. The townsfolk and merchants
  missing are listed (--with-npcs makes them too, as bare rows: no wares, no
  words).
- A missing creature's row copies one of ours (every column: the server's
  own for the kind, its unknown ones too): of its name, else of its body,
  else the creatures of Return Home's import's usual values; then takes the
  census's own: its name, place, facing, world, body, size, level, flags,
  race, held items, armour, hair, face and robe. npc_id is 0 (no loot rows of
  its own).
- Its name is ours when we have it (any world, our most common spelling),
  else as tools/fix_npc_name_case.py would case it (Sandstorm capitalizes
  every word: "Grass Viper" is "grass viper"; a proper word kept).
- A name this server has no row of takes rows in the tables a name keys:
  npcclass (Sandstorm's class), lootcreature (the world's loot region,
  REGION) and npctemplate (tools/seed_roamers.py's rule: a generic hostile
  warrior the roamer, a wisp the wisp).

Every row added is logged in sandstorm_census_seed (the table, the rowid,
the census id); --undo deletes exactly those. A second --apply is refused
while a census is in. Report only by default (--plan writes the rows it
would add as JSON). The server reads npcs at its start: apply with it
stopped (a swap's before.sh), or restart after.

    tools/seed_sandstorm_census.py --track 'zone-*.json.track.jsonl' --world 1
        [--db eqoa.db] [--match 150] [--with-npcs] [--plan out.json] [--apply | --undo]
"""
import argparse
import collections
import datetime
import glob
import json
import math
import os
import sqlite3
import statistics
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)

DEFAULT_DB = os.path.join(HERE, "..", "eqoa.db")
PLAYER_IDS = 100_000_000
MATCH = 150.0
LOG = "sandstorm_census_seed"
META = "sandstorm_census_meta"
SOURCE = "sandstorm-census"
REGION = {0: "tunaria", 1: "rathe mountains", 2: "odus"}

# Return Home's imported creatures' usual values for the columns the census
# does not see (a basilisk's row, 2026-10-31)
DEFAULTS = dict(hp=1, hp_percent=255, unk_a=1, unk_b=82, unk_d=0, unk_f=0, unk_g=0, unk_h=0, unk_j=0, unk_k=0,
                unk_i="15", pd_icon=0, unk_m=0, unk_n=1, unk_o=0, zone=0, dt_kill=None)


def key(name):
    return (name or "").strip().lower().rstrip("$").strip()


def read_census(paths):
    """id -> {looks, name, places [(t, x, y, z, h)]} for every creature seen."""
    seen = {}
    for path in paths:
        with open(path) as f:
            for line in f:
                try:
                    r = json.loads(line)
                except ValueError:
                    continue  # a line the harvester was still writing
                if "who" in r:
                    i = r["who"]
                    if i < PLAYER_IDS:
                        seen.setdefault(i, {"places": []})
                        seen[i].setdefault("looks", r)
                elif "id" in r and r["id"] < PLAYER_IDS:
                    c = seen.setdefault(r["id"], {"places": []})
                    c["name"] = r["name"]
                    c["places"].append((r["t"], r["x"], r["y"], r["z"], r.get("h", 0.0)))
    return {i: c for i, c in seen.items() if c.get("places") and c.get("looks")}


def place(c):
    """A creature's place: the median of where it was seen; its last heading."""
    ps = c["places"]
    x = statistics.median(p[1] for p in ps)
    z = statistics.median(p[3] for p in ps)
    # the height of the sighting nearest that place
    near = min(ps, key=lambda p: math.hypot(p[1] - x, p[3] - z))
    return x, near[2], z, ps[-1][4]


def facing_byte(h):
    return int(round((h % (2 * math.pi)) / (2 * math.pi) * 256)) % 256


def has_table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def spellings(db):
    """key -> our most common spelling of the name, any world."""
    c = collections.defaultdict(collections.Counter)
    for (n,) in db.execute("SELECT npc_name FROM npcs WHERE npc_name IS NOT NULL"):
        c[key(n)][n.strip()] += 1
    return {k: v.most_common(1)[0][0] for k, v in c.items()}


def cased(db, names):
    """The spelling fix_npc_name_case.py gives each new name (Sandstorm's, as
    the census has it), by its rules over this database's evidence, and
    whether those rules read it a kind's (generic) rather than someone's."""
    try:
        import fix_npc_name_case as nc
        ev = nc.load_evidence(db)
    except Exception as e:  # no evidence: every name someone's, made only when two or more carry it
        print(f"casing: {e}: Sandstorm's spellings kept", file=sys.stderr)
        return {n: (n, False) for n in names}
    out = {}
    for n in names:
        k = nc.fold(n)
        ev.variants[k][n] += 1
        d = nc.decide(ev, k)
        out[n] = (d.target if d is not None and d.target else n, d is not None and d.kind == "generic")
    return out


def pet_names(db):
    """A summoned pet's names: a spell's (entity/pet.go makes a pet after the
    NPCs of its spell's name) and an enchanter's animations."""
    out = set()
    if has_table(db, "spellpattern"):
        out = {key(n) for (n,) in db.execute("SELECT spellname FROM spellpattern WHERE spellname IS NOT NULL")}
    return out


def plan(db, census, world, match=MATCH, with_npcs=False):
    """(adds, report): the rows to add and what was found."""
    db.row_factory = sqlite3.Row
    cols = [r[1] for r in db.execute("PRAGMA table_info(npcs)")]
    ours = collections.defaultdict(list)
    by_name = collections.defaultdict(list)
    by_model = collections.defaultdict(list)
    for r in db.execute("SELECT * FROM npcs"):
        d = dict(r)
        k = key(d["npc_name"])
        if d["world"] == world:
            ours[k].append(d)
        if k:
            by_name[k].append(d)
        if d["modelid"]:
            by_model[d["modelid"] & 0xFFFFFFFF].append(d)
    db.row_factory = None
    ours_spelling = spellings(db)
    # each census creature to its nearest namesake of ours within match, one each
    pairs = []
    for i, c in census.items():
        x, _, z, _ = place(c)
        for d in ours.get(key(c["name"]), ()):
            dist = math.hypot(d["x_coord"] - x, d["z_coord"] - z)
            if dist <= match:
                pairs.append((dist, i, d["id"]))
    pairs.sort()
    matched, used = {}, set()
    for dist, i, rid in pairs:
        if i in matched or rid in used:
            continue
        matched[i] = (rid, dist)
        used.add(rid)
    pets = pet_names(db)
    is_pet = lambda i: key(census[i]["name"]) in pets or key(census[i]["name"]).endswith("animation")
    missing = [i for i in census if i not in matched and not is_pet(i)]
    pets_seen = collections.Counter(census[i]["name"] for i in census if is_pet(i))
    creatures = [i for i in missing if not census[i]["looks"].get("npc_flags", 0) & 0x80]
    people = [i for i in missing if census[i]["looks"].get("npc_flags", 0) & 0x80]
    new_names = sorted({census[i]["name"].strip() for i in missing if key(census[i]["name"]) not in ours_spelling})
    casing = cased(db, new_names)
    seen_count = collections.Counter(key(census[i]["name"]) for i in census)
    in_world = collections.Counter(key(d["npc_name"]) for k in ours for d in ours[k])

    def generic(i):
        # a kind's: two or more of ours or of the census, or the casing rules' generic
        k = key(census[i]["name"])
        if in_world[k] >= 2 or len(by_name.get(k, ())) >= 2 or seen_count[k] >= 2:
            return True
        return k not in ours_spelling and casing.get(census[i]["name"].strip(), ("", False))[1]

    named = [i for i in creatures if not generic(i)]
    elsewhere = [i for i in named if in_world[key(census[i]["name"])] > 0]
    make = [i for i in creatures if generic(i)] + (people if with_npcs else [])
    adds = []
    for i in sorted(make):
        c, lk = census[i], census[i]["looks"]
        x, y, z, h = place(c)
        k = key(c["name"])
        tmpl = None
        if by_name.get(k):
            tmpl = min(by_name[k], key=lambda d: (d["world"] != world, d["id"]))
        elif by_model.get(lk["model"]):
            tmpl = min(by_model[lk["model"]], key=lambda d: (d["world"] != world, d["id"]))
        row = {c2: None for c2 in cols}
        row.update(DEFAULTS)
        if tmpl:
            row.update(tmpl)
        row.pop("id", None)
        name = ours_spelling.get(k) or casing.get(c["name"].strip(), (c["name"].strip(), True))[0]
        items = (lk.get("items") or [0, 0, 0]) + [0, 0, 0]
        armor = (lk.get("armor") or [0] * 6) + [0] * 6
        hair = (lk.get("hair") or [0, 0, 0]) + [0, 0, 0]
        row.update(npc_name=name, x_coord=round(x, 3), y_coord=round(y, 4), z_coord=round(z, 3),
                   facing=facing_byte(h), world=world, modelid=lk["model"], size=lk.get("scale", 1.0),
                   npc_level=lk.get("lvl", 1), npc_type=lk.get("npc_flags", 0), race=str(lk.get("race", 0)),
                   primary_weapon=items[0], secondary_weapon=items[1], shield=items[2],
                   torso=armor[0], forearm=armor[1], gloves=armor[2], legs=armor[3], feet=armor[4], head=armor[5],
                   hair_color=hair[0], hair_length=hair[1], hair_style=hair[2], face=lk.get("face", 0),
                   robe_style=str(lk.get("robe", 255)), npc_id=0, dt_kill=None)
        adds.append(dict(row={c2: row.get(c2) for c2 in cols if c2 != "id"}, census_id=i, new_name=k not in ours_spelling,
                         sand_class=lk.get("class", 0), template_row=tmpl["id"] if tmpl else None))
    report = dict(seen=len(census), matched=len(matched), missing_creatures=len(creatures), missing_people=len(people),
                  adds=len(adds), new_names=sorted({a["row"]["npc_name"] for a in adds if a["new_name"]}),
                  by_name=collections.Counter(a["row"]["npc_name"] for a in adds).most_common(),
                  people=sorted(collections.Counter(census[i]["name"] for i in people).items()),
                  pets=sorted(pets_seen.items()),
                  named=sorted((census[i]["name"], census[i]["looks"].get("lvl"), [round(v) for v in place(census[i])[:3]])
                               for i in named if i not in elsewhere),
                  elsewhere=sorted((census[i]["name"], [round(v) for v in place(census[i])[:3]]) for i in elsewhere))
    return adds, report


def roamer_template(name, level, cls):
    """tools/seed_roamers.py's verdict for a new name: the wisp for a wisp, the
    roamer for a generic hostile warrior, else none."""
    if "wisp" in key(name).split():
        return "wisp"
    if cls == 0:
        return "roamer"
    return None


def apply(db, adds, world, files):
    if has_table(db, META):
        sys.exit(f"a census is applied already ({META}): --undo it first")
    seq = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'npcs'").fetchone()
    db.execute(f"CREATE TABLE {LOG} (seq INTEGER PRIMARY KEY AUTOINCREMENT, tbl TEXT NOT NULL, rid INTEGER NOT NULL, "
               f"census_id INTEGER, name TEXT NOT NULL)")
    db.execute(f"CREATE TABLE {META} (k TEXT PRIMARY KEY, v TEXT NOT NULL)")
    done_names = set()
    for a in adds:
        r = a["row"]
        cols = list(r)
        cur = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [r[c] for c in cols])
        db.execute(f"INSERT INTO {LOG} (tbl, rid, census_id, name) VALUES ('npcs', ?, ?, ?)", (cur.lastrowid, a["census_id"], r["npc_name"]))
        name = r["npc_name"]
        if a["new_name"] and name not in done_names:
            done_names.add(name)
            for tbl, sql, args in (
                ("npcclass", "INSERT INTO npcclass (name, class, source) VALUES (?, ?, ?)", (name, a["sand_class"], SOURCE)),
                ("lootcreature", "INSERT INTO lootcreature (npc_name, region, profile, source) VALUES (?, ?, '', ?)",
                 (name, REGION.get(world, ""), SOURCE)),
            ):
                if has_table(db, tbl) and db.execute(f"SELECT 1 FROM {tbl} WHERE lower(trim({'name' if tbl == 'npcclass' else 'npc_name'})) = ?",
                                                      (key(name),)).fetchone() is None:
                    cur = db.execute(sql, args)
                    db.execute(f"INSERT INTO {LOG} (tbl, rid, census_id, name) VALUES (?, ?, ?, ?)", (tbl, cur.lastrowid, a["census_id"], name))
            t = roamer_template(name, r["npc_level"], a["sand_class"]) if not r["npc_type"] & 0x80 else None
            if t and has_table(db, "npctemplate") and db.execute("SELECT 1 FROM npctemplate WHERE lower(trim(name)) = ?",
                                                                 (key(name),)).fetchone() is None:
                cur = db.execute("INSERT INTO npctemplate (name, template, note, source) VALUES (?, ?, ?, ?)",
                                 (name, t, f"generic, level {r['npc_level']}", SOURCE))
                db.execute(f"INSERT INTO {LOG} (tbl, rid, census_id, name) VALUES ('npctemplate', ?, ?, ?)", (cur.lastrowid, a["census_id"], name))
    for k, v in dict(applied=datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds"),
                     world=str(world), files=json.dumps(files), seq_before=str(seq[0] if seq else ""),
                     added=str(len(adds))).items():
        db.execute(f"INSERT INTO {META} VALUES (?, ?)", (k, v))


def undo(db):
    if not has_table(db, LOG):
        sys.exit("no census is applied")
    meta = dict(db.execute(f"SELECT k, v FROM {META}").fetchall()) if has_table(db, META) else {}
    n = collections.Counter()
    for tbl, rid in db.execute(f"SELECT tbl, rid FROM {LOG} ORDER BY seq DESC").fetchall():
        n[tbl] += db.execute(f"DELETE FROM {tbl} WHERE rowid = ?", (rid,)).rowcount
    seq = meta.get("seq_before", "")
    if seq:
        top = db.execute("SELECT MAX(id) FROM npcs").fetchone()[0] or 0
        if top <= int(seq):
            db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = 'npcs'", (int(seq),))
    db.execute(f"DROP TABLE {LOG}")
    db.execute(f"DROP TABLE IF EXISTS {META}")
    return n


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--track", action="append", default=[], help="the census's track files (a glob; again for more)")
    ap.add_argument("--world", type=int, default=1)
    ap.add_argument("--match", type=float, default=MATCH)
    ap.add_argument("--with-npcs", action="store_true", help="make the missing townsfolk and merchants too")
    ap.add_argument("--plan", help="write the rows it would add, as JSON")
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, timeout=5)
    if a.undo:
        with db:
            n = undo(db)
        print(f"undone: {dict(n)}")
        return
    files = sorted({p for g in a.track for p in glob.glob(g)})
    if not files:
        sys.exit("no track files (--track)")
    census = read_census(files)
    adds, report = plan(db, census, a.world, a.match, a.with_npcs)
    print(f"{len(files)} track files: {report['seen']} creatures seen, {report['matched']} ours already, "
          f"{report['missing_creatures']} creatures and {report['missing_people']} townsfolk missing; {report['adds']} to add")
    print("new names:", ", ".join(report["new_names"]) or "none")
    for name, n in report["by_name"]:
        print(f"  {n:4d}  {name}")
    if report["people"]:
        print("townsfolk missing (not made without --with-npcs):", ", ".join(f"{n} x{c}" for n, c in report["people"]))
    if report["pets"]:
        print("pets seen (a summoning spell's name: not made):", ", ".join(f"{n} x{c}" for n, c in report["pets"]))
    if report["named"]:
        print("named creatures we have none of (not made):", "; ".join(f"{n} L{l} at {p}" for n, l, p in report["named"]))
    if report["elsewhere"]:
        print("named creatures we have elsewhere in the world (not made):", "; ".join(f"{n} at {p}" for n, p in report["elsewhere"]))
    if a.plan:
        with open(a.plan, "w") as f:
            json.dump(dict(report=report, adds=adds), f, indent=1, default=str)
    if a.apply:
        with db:
            apply(db, adds, a.world, files)
        print(f"applied: {len(adds)} rows ({LOG})")


if __name__ == "__main__":
    main()
