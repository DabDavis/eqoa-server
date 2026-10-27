#!/usr/bin/env python3
"""The Plane of Disease's bosses on a database tools/seed_pod.py seeded before
F3 pass 21, made what its plan gives now (jib's reports 20261026-154852,
-155140 and -155308):

- Noroxus wears the green three-headed hydra (CHAR.ESF A18F3F22) at 1.1 and
  stands in his arena, the north-west lake's lobe, not by the cauldron;
- Bertoxxulous the Plaguebringer wears his own body (A6320E74) at 1.3 where
  he stands, and his npcaggro rule, written for the human body he wore, is
  taken out: he attacks on sight by nature now (entity/aggro.go);
- Poxathixius and Uzrathixius wear the green dragon (B5C8A651) at 1.5 where
  they stand;
- the Fuming Portal (at the cauldron, to Noroxus's lair) and the Portal to
  the Cauldron (in his lobe, back) are added as seed rows (pod_seed_npcs, so
  seed_pod.py --undo takes them too), and pod_seed_meta's counts follow.

A boss's row is changed only while it is still, column for column, what the
old plan wrote (OLD below, through seed_pod.py's own plan) and no GM has
edited it through the panel (gm_audit): any other is left as it is and
listed, as is a rule a GM changed since. A row already as the plan has it
is left too. A portal is added only where no NPC has its name. The columns a
row takes are the plan's: its body row's look (the cave hydra's, the ghoul
reaver's, Cyrcalis's), the body worn, the size, and Noroxus's place.

Everything written is logged in pod_boss_fix_log (each row's columns before
and after, a rule row taken out whole with its rowid, the npcs sequence);
--undo puts it all back as it was (sqlite3 .dump identical) but for a row
changed since, which it lists and leaves, and drops the log. Both take a
copy of the database first (--no-backup to skip). Report only by default.
The plan needs the ground files seed_pod.py needs (worlds 1 and 5). Stop the
server to apply or undo, start it after.

    tools/fix_pod_bosses.py --ground worlds/ground [--db eqoa.db]            report
    tools/fix_pod_bosses.py --ground worlds/ground [--db eqoa.db] --apply    write it
    tools/fix_pod_bosses.py [--db eqoa.db] --undo                            take it back
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import seed_pod  # noqa: E402

DEFAULT_DB = seed_pod.DEFAULT_DB
DEFAULT_GROUND = seed_pod.DEFAULT_GROUND
LOG = "pod_boss_fix_log"
BOSSES = ("Noroxus", "Bertoxxulous the Plaguebringer", "Poxathixius", "Uzrathixius")
# the rows the plan has that the old one had not (no other row is added: a
# seeded row a GM deleted stays deleted)
ADDED = ("Fuming Portal", "Portal to the Cauldron")
HOSTILE = seed_pod.HOSTILE

# The bosses as tools/seed_pod.py's plan had them until F3 pass 21, its
# default facings written out (they counted the rows before them): what a
# seeded row of theirs holds while no one has changed it.
OLD = [
    dict(name="Noroxus", body="ghoul reaver", level=65, kind=HOSTILE, world=5, scale=2.0,
         at=(5620, 3060), y=-30.5, facing=243, roam=0, role="boss (\"Guild needed\"): fetid rotting gland"),
    dict(name="Bertoxxulous the Plaguebringer", body="disciple of plague", model=0x70D898C6, level=66,
         kind=HOSTILE, world=5, scale=1.6, at=(4416, 2113), y=-22.2, facing=192, roam=0, undead=True,
         aggro=True, role="boss (\"Hard, full guild needed\"): soul dust x2"),
    dict(name="Poxathixius", body="emerald drake", model=0x5C9A274E, level=65, kind=HOSTILE, world=5,
         scale=1.5, at=(4130, 2170), y=-18.2, facing=209, roam=0, role="one of the Twins"),
    dict(name="Uzrathixius", body="emerald drake", model=0x5C9A274E, level=65, kind=HOSTILE, world=5,
         scale=1.5, at=(4190, 2140), y=-19.3, facing=246, roam=0, role="one of the Twins"),
]

LOG_DDL = f"""CREATE TABLE {LOG} (
    seq    INTEGER PRIMARY KEY,
    kind   TEXT NOT NULL,      -- update, insert, delete, meta, sequence
    tbl    TEXT NOT NULL,
    row    INTEGER,            -- the row's id or rowid
    before TEXT,               -- JSON: the columns as they were (a deleted row whole)
    after  TEXT,               -- JSON: the columns as this tool left them
    note   TEXT NOT NULL DEFAULT '',
    at     TEXT NOT NULL
)"""


def plan_of(db, ground, entries=None):
    """seed_pod.py's plan, of these entries alone (its PLAN when None)."""
    if entries is None:
        return seed_pod.plan(db, ground)
    keep = seed_pod.PLAN
    seed_pod.PLAN = entries
    try:
        return seed_pod.plan(db, ground)
    finally:
        seed_pod.PLAN = keep


def has_table(db, t):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone()[0] > 0


def cols_of(db, t):
    return [r[1] for r in db.execute(f"PRAGMA table_info({t})")]


def same(a, b):
    """Two column values alike (a float to a millionth)."""
    if isinstance(a, float) or isinstance(b, float):
        try:
            return abs(float(a) - float(b)) < 1e-6
        except (TypeError, ValueError):
            return False
    return a == b


def row_dict(db, tbl, where, args):
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", args)
    r = cur.fetchone()
    if r is None:
        return None
    return {d[0]: v for d, v in zip(cur.description, r)}


def gm_edited(db):
    """The npcs ids a GM has written through the panel (gm_audit's rowkey is
    the key as JSON, {"id": N})."""
    out = set()
    if not has_table(db, "gm_audit"):
        return out
    for (rk,) in db.execute("SELECT rowkey FROM gm_audit WHERE tbl = 'npcs'"):
        try:
            out.add(int(json.loads(rk).get("id")))
        except (ValueError, TypeError, AttributeError):
            continue
    return out


def rule_key(r):
    tbl, keycol, key, _ = r
    return tbl, key


def work(db, old, new):
    """What to do: old is the old plan's (OLD's) rows and rules, new the plan's
    now. Returns the boss rows to change, those left (and why), the portals to
    add, the rules to take out and to add, and what is already done."""
    seeded = db.execute("SELECT npc_row, name, body_row, role FROM pod_seed_npcs ORDER BY npc_row").fetchall()
    gm = gm_edited(db)
    old_rows = {r["row"]["npc_name"]: r for r in old["rows"]}
    new_rows = {}
    for r in new["rows"]:
        new_rows.setdefault(r["row"]["npc_name"], []).append(r)
    cols = [c for c in cols_of(db, "npcs") if c != "id"]
    change, left, done = [], [], []
    for name in BOSSES:
        rows = [s for s in seeded if s[1] == name]
        if len(rows) != 1 or name not in old_rows or len(new_rows.get(name, [])) != 1:
            left.append((name, None, f"{len(rows)} seeded rows of that name, not one"))
            continue
        npc_row = rows[0][0]
        now = row_dict(db, "npcs", "id = ?", (npc_row,))
        want = new_rows[name][0]
        if now is None:
            left.append((name, npc_row, "the row is gone"))
            continue
        if all(same(now[c], want["row"].get(c)) for c in cols):
            done.append((name, npc_row))
            continue
        was = old_rows[name]["row"]
        differs = [c for c in cols if not same(now[c], was.get(c))]
        if differs or npc_row in gm:
            why = f"changed since the seed ({', '.join(differs)})" if differs else "a GM edited it (gm_audit)"
            left.append((name, npc_row, why))
            continue
        diff = [c for c in cols if not same(now[c], want["row"].get(c))]
        change.append(dict(name=name, npc_row=npc_row, before={c: now[c] for c in diff},
                           after={c: want["row"].get(c) for c in diff}, body=want["body"],
                           body_row=want["body_row"], role=want["role"]))
    add = []
    for name in ADDED:
        mine = [s[0] for s in seeded if s[1] == name]
        if mine:
            done.append((name, mine[0]))
            continue
        taken = db.execute("SELECT count(*) FROM npcs WHERE lower(trim(npc_name)) = ?", (name.strip().lower(),)).fetchone()[0]
        if taken:
            left.append((name, None, f"{taken} NPCs of that name already: not added"))
            continue
        add.extend(new_rows.get(name, []))
    old_rules = {rule_key(r): r for r in old["rules"]}
    new_rules = {rule_key(r): r for r in new["rules"] if r[2].strip().lower() in {b.lower() for b in BOSSES}}
    take_out, put_in = [], []
    for k, (tbl, keycol, key, vals) in old_rules.items():
        if k in new_rules or not has_table(db, tbl):
            continue
        logged = db.execute("SELECT rowid, * FROM pod_seed_rules WHERE tbl = ? AND name = ?", (tbl, key)).fetchone()
        cur = db.execute(f"SELECT rowid, * FROM {tbl} WHERE {keycol} = ?", (key,))
        row = cur.fetchone()
        if logged is None or row is None:
            continue  # not the seed's (a GM's row stood) or gone
        now = {d[0]: v for d, v in zip(cur.description, row)}
        if any(not same(now.get(c), v) for c, v in vals.items()):
            left.append((f"{tbl} {key}", now["rowid"], "a rule changed since the seed: kept"))
            continue
        take_out.append(dict(tbl=tbl, keycol=keycol, key=key, row=now,
                             logged={"rowid": logged[0], "tbl": logged[1], "keycol": logged[2], "name": logged[3]}))
    for k, (tbl, keycol, key, vals) in new_rules.items():
        if k in old_rules or not has_table(db, tbl):
            continue
        if db.execute(f"SELECT count(*) FROM {tbl} WHERE {keycol} = ?", (key,)).fetchone()[0]:
            continue  # a GM's (or another seed's) row stands
        put_in.append(dict(tbl=tbl, keycol=keycol, key=key, vals=vals))
    return dict(change=change, left=left, done=done, add=add, take_out=take_out, put_in=put_in)


def report(w):
    for c in w["change"]:
        a, b = c["after"], c["before"]
        where = ""
        if "x_coord" in a or "z_coord" in a:
            where = (f" moved ({b.get('x_coord')}, {b.get('y_coord')}, {b.get('z_coord')}) -> "
                     f"({a.get('x_coord')}, {a.get('y_coord')}, {a.get('z_coord')})")
        model = f" body {b['modelid']:08X} -> {a['modelid']:08X}" if "modelid" in a else ""
        size = f" size {b['size']} -> {a['size']}" if "size" in a else ""
        print(f"  {c['name']} (row {c['npc_row']}):{model}{size}{where}; columns {', '.join(sorted(a))} "
              f"(the look of {c['body']})")
    for name, npc_row in w["done"]:
        print(f"  {name} (row {npc_row}): already as the plan has it")
    for r in w["add"]:
        row = r["row"]
        print(f"  add {row['npc_name']}: L{row['npc_level']} type {row['npc_type']} world {row['world']} "
              f"({row['x_coord']}, {row['y_coord']}, {row['z_coord']}) facing {row['facing']}, {r['body']}: {r['role']}")
    for t in w["take_out"]:
        print(f"  take out {t['tbl']} {t['key']!r} ({t['row'].get('note', '')}): the seed's rule, no longer the plan's")
    for p in w["put_in"]:
        print(f"  put in {p['tbl']} {p['key']!r}")
    for name, npc_row, why in w["left"]:
        print(f"  left: {name}" + (f" (row {npc_row})" if npc_row is not None else "") + f": {why}")
    print(f"{len(w['change'])} bosses to change, {len(w['done'])} already done, {len(w['left'])} left; "
          f"{len(w['add'])} portals to add; {len(w['take_out'])} rules to take out, {len(w['put_in'])} to put in")


def log(db, kind, tbl, row, before, after, note, at):
    db.execute(f"INSERT INTO {LOG} (kind, tbl, row, before, after, note, at) VALUES (?, ?, ?, ?, ?, ?, ?)",
               (kind, tbl, row, None if before is None else json.dumps(before), None if after is None else json.dumps(after),
                note, at))


def npc_seq(db):
    r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'npcs'").fetchone()
    return None if r is None else r[0]


def apply(db, w):
    if has_table(db, LOG):
        sys.exit(f"this database holds the fix already ({LOG}): --undo it first")
    at = datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")
    cols = [c for c in cols_of(db, "npcs") if c != "id"]
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_DDL)
        for c in w["change"]:
            sets = ", ".join(f"{k} = ?" for k in c["after"])
            db.execute(f"UPDATE npcs SET {sets} WHERE id = ?", [*c["after"].values(), c["npc_row"]])
            log(db, "update", "npcs", c["npc_row"], c["before"], c["after"], c["name"], at)
            prev = db.execute("SELECT body_row, role FROM pod_seed_npcs WHERE npc_row = ?", (c["npc_row"],)).fetchone()
            if prev is not None and (prev[0], prev[1]) != (c["body_row"], c["role"]):
                db.execute("UPDATE pod_seed_npcs SET body_row = ?, role = ? WHERE npc_row = ?",
                           (c["body_row"], c["role"], c["npc_row"]))
                log(db, "update", "pod_seed_npcs", c["npc_row"], {"body_row": prev[0], "role": prev[1]},
                    {"body_row": c["body_row"], "role": c["role"]}, c["name"], at)
        if w["add"]:
            log(db, "sequence", "npcs", None, {"seq": npc_seq(db)}, None, "", at)
        for r in w["add"]:
            vals = [r["row"].get(k) for k in cols]
            cur = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", vals)
            log(db, "insert", "npcs", cur.lastrowid, None, dict(zip(cols, vals)), r["row"]["npc_name"], at)
            seed_row = dict(npc_row=cur.lastrowid, name=r["row"]["npc_name"], body_row=r["body_row"], role=r["role"])
            db.execute("INSERT INTO pod_seed_npcs (npc_row, name, body_row, role) VALUES (?, ?, ?, ?)",
                       (seed_row["npc_row"], seed_row["name"], seed_row["body_row"], seed_row["role"]))
            log(db, "insert", "pod_seed_npcs", cur.lastrowid, None, seed_row, r["row"]["npc_name"], at)
        for t in w["take_out"]:
            db.execute(f"DELETE FROM {t['tbl']} WHERE rowid = ?", (t["row"]["rowid"],))
            log(db, "delete", t["tbl"], t["row"]["rowid"], t["row"], None, t["key"], at)
            db.execute("DELETE FROM pod_seed_rules WHERE rowid = ?", (t["logged"]["rowid"],))
            log(db, "delete", "pod_seed_rules", t["logged"]["rowid"], t["logged"], None, t["key"], at)
        for p in w["put_in"]:
            names = list(p["vals"])
            cur = db.execute(f"INSERT INTO {p['tbl']} ({', '.join(names)}) VALUES ({', '.join('?' * len(names))})",
                             [p["vals"][n] for n in names])
            log(db, "insert", p["tbl"], cur.lastrowid, None, dict(p["vals"]), p["key"], at)
            cur = db.execute("INSERT INTO pod_seed_rules VALUES (?, ?, ?)", (p["tbl"], p["keycol"], p["key"]))
            log(db, "insert", "pod_seed_rules", cur.lastrowid, None, {"tbl": p["tbl"], "keycol": p["keycol"], "name": p["key"]},
                p["key"], at)
        meta = dict(db.execute("SELECT key, value FROM pod_seed_meta"))
        counts = {"rows": len(w["add"]), "rules": len(w["put_in"]) - len(w["take_out"])}
        for k, d in counts.items():
            if d and k in meta:
                v = str(int(meta[k]) + d)
                db.execute("UPDATE pod_seed_meta SET value = ? WHERE key = ?", (v, k))
                log(db, "meta", "pod_seed_meta", None, {"key": k, "value": meta[k]}, {"key": k, "value": v}, "", at)
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db):
    if not has_table(db, LOG):
        sys.exit(f"nothing of this tool's to undo here (no {LOG})")
    logged = db.execute(f"SELECT seq, kind, tbl, row, before, after, note FROM {LOG} ORDER BY seq DESC").fetchall()
    back, left = 0, []
    db.execute("BEGIN IMMEDIATE")
    try:
        for seq, kind, tbl, row, before, after, note in logged:
            before = None if before is None else json.loads(before)
            after = None if after is None else json.loads(after)
            if kind == "update":
                key = "id" if tbl == "npcs" else "npc_row"
                now = row_dict(db, tbl, f"{key} = ?", (row,))
                if now is None or any(not same(now.get(c), v) for c, v in after.items()):
                    left.append(f"{tbl} row {row} ({note}): changed since, left as it is")
                    continue
                sets = ", ".join(f"{k} = ?" for k in before)
                db.execute(f"UPDATE {tbl} SET {sets} WHERE {key} = ?", [*before.values(), row])
            elif kind == "insert":
                where, args = "rowid = ?", (row,)
                now = row_dict(db, tbl, where, args)
                if now is None:
                    continue
                if any(not same(now.get(c), v) for c, v in after.items()):
                    left.append(f"{tbl} row {row} ({note}): changed since, left as it is")
                    continue
                db.execute(f"DELETE FROM {tbl} WHERE rowid = ?", (row,))
            elif kind == "delete":
                vals = dict(before)
                rowid = vals.pop("rowid")
                keycol = "name" if tbl == "pod_seed_rules" else None
                if tbl == "pod_seed_rules":
                    clash = db.execute("SELECT count(*) FROM pod_seed_rules WHERE tbl = ? AND name = ?",
                                       (vals["tbl"], vals["name"])).fetchone()[0]
                else:
                    keycol = next(c for c in cols_of(db, tbl) if c in ("npc_name", "name"))
                    clash = db.execute(f"SELECT count(*) FROM {tbl} WHERE {keycol} = ?", (vals[keycol],)).fetchone()[0]
                clash += db.execute(f"SELECT count(*) FROM {tbl} WHERE rowid = ?", (rowid,)).fetchone()[0]
                if clash:
                    left.append(f"{tbl} {note!r}: a row of that key stands since, not put back")
                    continue
                names = list(vals)
                db.execute(f"INSERT INTO {tbl} (rowid, {', '.join(names)}) VALUES (?, {', '.join('?' * len(names))})",
                           [rowid, *[vals[n] for n in names]])
            elif kind == "meta":
                now = db.execute("SELECT value FROM pod_seed_meta WHERE key = ?", (after["key"],)).fetchone()
                if now is None or now[0] != after["value"]:
                    left.append(f"pod_seed_meta {after['key']}: changed since, left as it is")
                    continue
                db.execute("UPDATE pod_seed_meta SET value = ? WHERE key = ?", (before["value"], before["key"]))
            elif kind == "sequence":
                was = before["seq"]
                top = db.execute("SELECT max(id) FROM npcs").fetchone()[0] or 0
                if was is None:
                    if top == 0:
                        db.execute("DELETE FROM sqlite_sequence WHERE name = 'npcs'")
                elif top <= was:
                    db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = 'npcs'", (was,))
                else:
                    left.append(f"the npcs sequence: rows past {was} were added since, left at its count")
                    continue
            back += 1
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return back, left


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-podbossfix"
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--ground", default=DEFAULT_GROUND, help="the ground files' folder (worlds/ground)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the changes (logged in pod_boss_fix_log)")
    g.add_argument("--undo", action="store_true", help="put back everything this tool wrote")
    ap.add_argument("--no-backup", action="store_true", help="write without a copy first")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    db.row_factory = sqlite3.Row

    if a.undo:
        if not has_table(db, LOG):
            sys.exit(f"nothing of this tool's to undo here (no {LOG})")
        if not a.no_backup:
            print("copy:", backup(db, a.db))
        back, left = undo(db)
        for line in left:
            print("  left:", line)
        print(f"put back {back} changes; {len(left)} left as they are now; restart the server")
        return

    if not has_table(db, "pod_seed_npcs"):
        sys.exit("this database holds no PoD seed (tools/seed_pod.py): nothing to fix")
    if has_table(db, LOG):
        print(f"this database holds the fix already ({LOG}); what is still to do:")
    old = plan_of(db, a.ground, OLD)
    new = plan_of(db, a.ground)
    for name, p in (("the old plan", old), ("the plan", new)):
        if p["missing"] or p["unplaced"] or p["off"]:
            sys.exit(f"{name} cannot be made here: bodies missing {p['missing']}, unplaced {p['unplaced']}, "
                     f"ground differs {p['off']}")
    w = work(db, old, new)
    report(w)
    if not a.apply:
        print("(report only: --apply to write it)")
        return
    if not (w["change"] or w["add"] or w["take_out"] or w["put_in"]):
        print("nothing to write")
        return
    if not a.no_backup:
        print("copy:", backup(db, a.db))
    apply(db, w)
    print(f"applied to {a.db}; restart the server")


if __name__ == "__main__":
    main()
