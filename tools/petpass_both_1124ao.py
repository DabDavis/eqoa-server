#!/usr/bin/env python3
"""Zombie training dummies beside the training dummies, both shards (the owner, 2026-11-24: "add zombie dummy",
answering Heirloom petition 2877, Mightyeagle: "flag training dummys as undead please").

The training dummies stay as they are. Three new ones, so the undead-only abilities (Ward/Smite/Disrupt Death, the
Undead line, Coerce Undead, the undead procs) can be tried on a dummy:

  Training Dummy Zombie Lv20  Muniel's Tea Garden, 4 west of the Training Dummy Lv20 (2561)
  Training Dummy Zombie Lv60  Muniel's Tea Garden, 4 west of the Training Dummy Lv60 (2568)
  Training Dummy Zombie Lv50  Freeport, 4 north of the Training Dummy Lv50 (683)

Each is a copy of its neighbour's npcs row (level, facing, hp, type: a dummy is a dummy by its name,
entity/dummy.go DummyKindOf "training dummy...", so it never dies, fights its one player in rounds and heals 300
after each hit) with the zombie body 0x1BBB898C (465275276, the "zombie" rows', size 1.0, race 0) in place of the
dummy post, set down on the ground cmd/campground reads (stand 54.09-54.12, not embedded, no wall from the
neighbour). Each name gets an npcundead row (undead 1; the name says zombie too, undead.go's undeadName) and an
npcclass row as its neighbour's (class 0, the warrior's swing).

Refused, nothing written, if a neighbour is not as read on 2026-11-24 (both shards alike) or the zombie body's rows
are gone. A name already present is left (said "already").

Every row written is a gm_audit line (via "tool", [basis:owner owner 2026-11-24]) and a row of the log; --undo
deletes each row it added (one changed since is left and said) and drops the log. A report by default; --apply
writes once; --apply and --undo copy the database first (<db>.bak-<when>-pre-pet1124ao) unless --no-backup.

WHEN IT TAKES EFFECT: npcs at a GM panel reload_npcs (or the next start); npcundead and npcclass at the start and
at a panel edit (the name already reads undead).

    python3 -E -s tools/petpass_both_1124ao.py --db eqoa.db               report
    python3 -E -s tools/petpass_both_1124ao.py --db eqoa.db --apply       write it
    python3 -E -s tools/petpass_both_1124ao.py --db eqoa.db --undo        take it back
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "petpass_both_1124ao_log"
WHO, VIA = "claude", "tool"
DETAIL = ("[basis:owner owner 2026-11-24] \"add zombie dummy\" (Heirloom petition 2877, Mightyeagle: \"flag training "
          "dummys as undead please\"): zombie training dummies beside the dummies, tools/petpass_both_1124ao.py")
LOG_DDL = """CREATE TABLE IF NOT EXISTS {log} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    ref    TEXT NOT NULL,
    before TEXT,
    after  TEXT,
    audit  INTEGER,
    at     TEXT NOT NULL
)"""
AUDIT_DDL = """CREATE TABLE IF NOT EXISTS gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER,
    username TEXT, via TEXT, ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT)"""

ZOMBIE_MODEL, ZOMBIE_SIZE, ZOMBIE_RACE = 465275276, 1.0, "0"   # 0x1BBB898C, the "zombie" rows' body
DUMMY_MODEL = 1713778460                                       # 0x66262F1C, the dummy post
# (new name, neighbour id, its name, its x, its z, new x, new z); y as the neighbour's (54.1)
NEW = [("Training Dummy Zombie Lv20", 2561, "Training Dummy Lv20", 23339.6, 19575.5, 23335.6, 19575.5),
       ("Training Dummy Zombie Lv60", 2568, "Training Dummy Lv60", 23360.5, 19563.1, 23356.5, 19563.1),
       ("Training Dummy Zombie Lv50", 683, "Training Dummy Lv50", 25365.9, 15716.5, 25365.9, 15712.5)]


class Refused(Exception):
    pass


def go_json(m):
    """A row as Go's json.Marshal writes a map (keys sorted, <>& escaped), as the panel's audit lines are."""
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def rows(db, sql, args=()):
    cur = db.execute(sql, args)
    names = [c[0] for c in cur.description]
    return [dict(zip(names, r)) for r in cur.fetchall()]


def one(db, what, sql, args=()):
    got = rows(db, sql, args)
    if len(got) != 1:
        raise Refused(f"{what}: {len(got)} rows, want one")
    return got[0]


def has_table(db, name):
    return bool(db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone())


class Plan:
    def __init__(self, db):
        self.db = db
        self.changes = []  # (tbl, key column, row dict)
        self.lines = []

    def build(self):
        db = self.db
        n = db.execute("SELECT count(*) FROM npcs WHERE npc_name = 'zombie' AND modelid = ?", (ZOMBIE_MODEL,)).fetchone()[0]
        if n == 0:
            raise Refused("no 'zombie' row wears 465275276 (0x1BBB898C)")
        for name, nid, nname, nx, nz, x, z in NEW:
            have = db.execute("SELECT count(*) FROM npcs WHERE npc_name = ?", (name,)).fetchone()[0]
            if have:
                self.lines.append(f"already  npcs {name!r} ({have} rows)")
            else:
                nb = one(db, f"npcs {nid}", "SELECT * FROM npcs WHERE id = ?", (nid,))
                if (nb["npc_name"] != nname or abs(nb["x_coord"] - nx) > 0.1 or abs(nb["z_coord"] - nz) > 0.1
                        or nb["modelid"] != DUMMY_MODEL or nb["world"] != 0):
                    raise Refused(f"npcs {nid} is {nb['npc_name']!r} at {nb['x_coord']},{nb['z_coord']} "
                                  f"world {nb['world']} model {nb['modelid']}, not {nname!r} at {nx},{nz}")
                row = dict(nb)
                del row["id"]
                row.update(npc_name=name, x_coord=x, z_coord=z, modelid=ZOMBIE_MODEL, size=ZOMBIE_SIZE,
                           race=ZOMBIE_RACE, dt_kill=None)
                self.changes.append(("npcs", "id", row))
                self.lines.append(f"insert   npcs {name!r} L{row['npc_level']} at ({x}, {row['y_coord']}, {z}) "
                                  f"facing {row['facing']}, beside {nid} {nname!r}")
            key = name.lower()
            u = db.execute("SELECT undead FROM npcundead WHERE name = ?", (key,)).fetchone()
            if u:
                if u[0] != 1:
                    raise Refused(f"npcundead {key!r} says undead {u[0]}")
                self.lines.append(f"already  npcundead {key!r}")
            else:
                self.changes.append(("npcundead", "name", dict(
                    name=key, undead=1, source="petition 2877",
                    note="a zombie training dummy (owner 2026-11-24, Heirloom petition 2877): tools/petpass_both_1124ao.py")))
                self.lines.append(f"insert   npcundead {key!r} undead 1")
            c = db.execute("SELECT * FROM npcclass WHERE name = ?", (key,)).fetchone()
            if c:
                self.lines.append(f"already  npcclass {key!r}")
            else:
                nc = rows(db, "SELECT * FROM npcclass WHERE name = ?", (nname.lower(),))
                if len(nc) == 1:
                    r = dict(nc[0])
                    r["name"] = key
                    self.changes.append(("npcclass", "name", r))
                    self.lines.append(f"insert   npcclass {key!r} as {nname.lower()!r}")


def audit(db, action, tbl, key, before, after):
    db.execute(AUDIT_DDL)
    cur = db.execute("""INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after,
        detail) VALUES (?, 0, ?, ?, '', ?, ?, ?, ?, ?, ?)""",
                     (now(), WHO, VIA, action, tbl, go_json(key), go_json(before) if before else None,
                      go_json(after) if after else None, DETAIL))
    return cur.lastrowid


def where(key):
    return " AND ".join(f"{k} = ?" for k in key), list(key.values())


def apply(db, plan, log):
    db.execute(LOG_DDL.format(log=log))
    for tbl, kcol, row in plan.changes:
        cols = list(row)
        cur = db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' for _ in cols)})",
                         [row[c] for c in cols])
        k = cur.lastrowid if kcol == "id" else row[kcol]
        key = {kcol: k}
        after = rows(db, f"SELECT * FROM {tbl} WHERE {kcol} = ?", (k,))[0]
        a = audit(db, "insert", tbl, key, None, after)
        db.execute(f"INSERT INTO {log} (tbl, ref, before, after, audit, at) VALUES (?, ?, ?, ?, ?, ?)",
                   (tbl, json.dumps(key), None, json.dumps(after), a, now()))
        if tbl == "npcs":
            plan.lines.append(f"added    npcs id {k} {row['npc_name']!r}")


def undo(db, log):
    out = []
    for r in rows(db, f"SELECT * FROM {log} ORDER BY seq DESC"):
        tbl, key = r["tbl"], json.loads(r["ref"])
        w, args = where(key)
        now_rows = rows(db, f"SELECT * FROM {tbl} WHERE {w}", args)
        after = json.loads(r["after"])
        if len(now_rows) != 1 or now_rows[0] != after:
            out.append(f"left     {tbl} {key}: changed since")
            continue
        db.execute(f"DELETE FROM {tbl} WHERE {w}", args)
        audit(db, "delete", tbl, key, after, None)
        out.append(f"deleted  {tbl} {key}")
    db.execute(f"DROP TABLE {log}")
    return out


def backup(path):
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-pet1124ao"
    src = sqlite3.connect(path)
    out = sqlite3.connect(dst)
    src.backup(out)
    out.close()
    src.close()
    return dst


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    ap.add_argument("--no-backup", action="store_true")
    a = ap.parse_args(argv)
    log = LOG
    db = sqlite3.connect(a.db, timeout=5, isolation_level=None)
    db.execute("BEGIN IMMEDIATE")
    try:
        logged = has_table(db, log)
        if a.undo:
            if not logged:
                print("nothing to undo: no log")
                db.execute("ROLLBACK")
                return 0
            if not a.no_backup:
                print("backup", backup(a.db))
            for line in undo(db, log):
                print(line)
            db.execute("COMMIT")
            return 0
        if logged and a.apply:
            print(f"applied already ({log} exists): nothing written")
            db.execute("ROLLBACK")
            return 0
        plan = Plan(db)
        plan.build()
        for line in plan.lines:
            print(line)
        if not a.apply:
            print(f"dry run: {len(plan.changes)} rows would change")
            db.execute("ROLLBACK")
            return 0
        if not a.no_backup:
            print("backup", backup(a.db))
        apply(db, plan, log)
        db.execute("COMMIT")
        for line in plan.lines:
            if line.startswith("added"):
                print(line)
        print(f"applied: {len(plan.changes)} rows")
        return 0
    except Refused as e:
        db.execute("ROLLBACK")
        print(f"REFUSED, nothing written: {e}")
        return 1
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
