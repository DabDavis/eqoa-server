#!/usr/bin/env python3
"""Heirloom petition 2800 of 2026-11-23 (the sixth batch of the day), as data: Ancient Death's own body, larger.

Every row is found by its name (and what else names it), never by an id alone, and must read as it did when this
was made; a row already as this writes it is left ("already"); anything else refuses the whole run.

  2800  "Ancient death isnt supposed to look like howling death, its a much larger floating spectre." The
        necromancer's Ancient Death pets (petstats 963 and 973, the two spells named Ancient Death, EQOAEmu's
        workbook's 'Ancient Death' and 'Ancient Death (49)') wore Howling Death's body, 0xD8C9F052, at its wearers'
        size (the four Howling Death rows' 1.0). They take the body this server's own Ancient Death NPCs wear
        (rows 741 and 120070, from the captured data: 0xD8C9F053, the red-robed floating spectre, seen on
        `eqoa shot CHAR.ESF` beside Howling Death's two), its decimal copied from row 741, and the larger of those NPCs' sizes, 1.625
        (row 120070, the level 48 one; Howling Death's pets stay 1.0). A size over 0 is the pet's own over its body's
        wearers' (entity/petstats.go applyPetSheet, petition 1283).

Every row written is a gm_audit line (via "tool") and a row of the log; --undo puts each back as it was (a row
changed since is left and said) and drops the log. A report by default; --apply writes once (a second finds the log
and writes nothing); --apply and --undo copy the database first (<db>.bak-<when>-pre-pet1123f) unless --no-backup.

WHEN IT TAKES EFFECT: petstats is read at the server's start: a pet summoned after the next restart.

    python3 -E -s tools/petpass_heirloom_1123f.py --db eqoa.db            report
    python3 -E -s tools/petpass_heirloom_1123f.py --db eqoa.db --apply    write it
    python3 -E -s tools/petpass_heirloom_1123f.py --db eqoa.db --undo     take it back
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "petpass_heirloom_1123f_log"
WHO, VIA = "claude", "tool"
DETAIL = "Heirloom petition 2800 (2026-11-23): Ancient Death's own body, larger: tools/petpass_heirloom_1123f.py"

# 2800
PET_NAME = "Ancient Death"
PETS = {963: "Ancient Death", 973: "Ancient Death"}  # petstats spellid -> its spell (the workbook's "(49)" one too)
HOWLING = 3637112914          # 0xD8C9F052, Howling Death's body, what the pets wore
ANCIENT_ROW, ANCIENT_BODY = 741, 3637112915  # 0xD8C9F053, read off this NPC row named Ancient Death
BIG_ROW, BIG_SIZE = 120070, 1.625            # the larger Ancient Death row's size

KEYS = {"petstats": ("spellid",)}
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
        self.changes = []  # (tbl, key dict or None, before row or None, values or None: a delete)
        self.lines = []

    def set(self, tbl, row, values, olds, what):
        """values over row, when each column of olds reads its old; already as values: left."""
        if all(row[c] == v for c, v in values.items()):
            self.lines.append(f"already  {what}")
            return
        for c, old in olds.items():
            if row[c] != old:
                raise Refused(f"{what}: {c} reads {row[c]!r}, not {old!r} as read when this was made")
        key = {k: row[k] for k in KEYS[tbl]}
        self.changes.append((tbl, key, dict(row), dict(values)))
        self.lines.append(f"change   {what}: " + ", ".join(f"{c} {row[c]!r} -> {v!r}" for c, v in values.items()))

    def build(self):
        db = self.db
        src = one(db, f"NPC row {ANCIENT_ROW}", "SELECT modelid, size FROM npcs WHERE id = ? AND npc_name = ?",
                  (ANCIENT_ROW, PET_NAME))
        body = src["modelid"] & 0xFFFFFFFF
        if body != ANCIENT_BODY:
            raise Refused(f"row {ANCIENT_ROW} ({PET_NAME}) wears {body:#010x}, not {ANCIENT_BODY:#010x}")
        big = one(db, f"NPC row {BIG_ROW}", "SELECT modelid, size FROM npcs WHERE id = ? AND npc_name = ?",
                  (BIG_ROW, PET_NAME))
        if big["modelid"] & 0xFFFFFFFF != ANCIENT_BODY or big["size"] != BIG_SIZE:
            raise Refused(f"row {BIG_ROW} ({PET_NAME}) reads {big}, not body {ANCIENT_BODY:#010x} at {BIG_SIZE}")
        for spellid, spell in PETS.items():
            one(db, f"the spell {spell}", "SELECT 1 FROM spellpattern WHERE spellid = ? AND spellname = ?",
                (spellid, spell))
            row = one(db, f"petstats {spellid}", "SELECT * FROM petstats WHERE spellid = ? AND name = ?",
                      (spellid, PET_NAME))
            self.set("petstats", row, {"model": src["modelid"], "size": BIG_SIZE}, {"model": HOWLING, "size": 0.0},
                     f"2800 {PET_NAME} (petstats {spellid}) the Ancient Death NPCs' body {ANCIENT_BODY:#010x}"
                     f" at {BIG_SIZE}")


def audit(db, action, tbl, key, before, after):
    db.execute(AUDIT_DDL)
    cur = db.execute("""INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after,
        detail) VALUES (?, 0, ?, ?, '', ?, ?, ?, ?, ?, ?)""",
                     (now(), WHO, VIA, action, tbl, go_json(key), go_json(before) if before else None,
                      go_json(after) if after else None, DETAIL))
    return cur.lastrowid


def where(key):
    return " AND ".join(f"{k} = ?" for k in key), list(key.values())


def keyof(tbl, row):
    return {k: row[k] for k in KEYS[tbl]}


def apply(db, plan, log):
    db.execute(LOG_DDL.format(log=log))
    for tbl, key, before, values in plan.changes:
        if values is None:  # a delete
            w, args = where(key)
            db.execute(f"DELETE FROM {tbl} WHERE {w}", args)
            a, after = audit(db, "delete", tbl, key, before, None), None
        elif before is None:
            cols = list(values)
            cur = db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                             [values[c] for c in cols])
            key = keyof(tbl, values) if all(k in values for k in KEYS[tbl]) else {KEYS[tbl][0]: cur.lastrowid}
            after = rows(db, f"SELECT * FROM {tbl} WHERE {where(key)[0]}", where(key)[1])[0]
            a = audit(db, "insert", tbl, key, None, after)
        else:
            w, args = where(key)
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in values)} WHERE {w}",
                       list(values.values()) + args)
            after = rows(db, f"SELECT * FROM {tbl} WHERE {w}", args)[0]
            a = audit(db, "update", tbl, key, before, after)
        db.execute(f"INSERT INTO {log} (tbl, ref, before, after, audit, at) VALUES (?, ?, ?, ?, ?, ?)",
                   (tbl, json.dumps(key), json.dumps(before) if before else None,
                    json.dumps(after) if after else None, a, now()))


def undo(db, log):
    out = []
    for r in rows(db, f"SELECT * FROM {log} ORDER BY seq DESC"):
        tbl, key = r["tbl"], json.loads(r["ref"])
        w, args = where(key)
        now_rows = rows(db, f"SELECT * FROM {tbl} WHERE {w}", args)
        if r["after"] is None:  # a delete: the row goes back unless one holds its key now
            if now_rows:
                out.append(f"left     {tbl} {key}: a row holds its key since")
                continue
            before = json.loads(r["before"])
            cols = list(before)
            db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                       [before[c] for c in cols])
            audit(db, "insert", tbl, key, None, before)
            out.append(f"restored {tbl} {key}")
            continue
        after = json.loads(r["after"])
        if len(now_rows) != 1 or now_rows[0] != after:
            out.append(f"left     {tbl} {key}: changed since")
            continue
        if r["before"] is None:
            db.execute(f"DELETE FROM {tbl} WHERE {w}", args)
            audit(db, "delete", tbl, key, after, None)
        else:
            before = json.loads(r["before"])
            cols = list(before)
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in cols)} WHERE {w}",
                       [before[c] for c in cols] + args)
            audit(db, "update", tbl, keyof(tbl, before), after, before)
        out.append(f"restored {tbl} {key}")
    db.execute(f"DROP TABLE {log}")
    return out


def backup(path):
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-pet1123f"
    src = sqlite3.connect(path)
    out = sqlite3.connect(dst)
    src.backup(out)
    out.close()
    src.close()
    return dst


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--undo", action="store_true")
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
        print(f"applied: {len(plan.changes)} rows")
        return 0
    except Refused as e:
        db.execute("ROLLBACK")
        print("refused:", e, file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
