#!/usr/bin/env python3
"""The owner's decisions of 2026-11-22 that touch many rows: quest drop rates and the froglok on race lists.

drops    Every quest loot step whose chance is under 100% (queststeps kind 'loot', 0 < chance < 1) has its miss
         chance halved: new = chance + (1 - chance) / 2 (50% -> 75%, 20% -> 60%, 8% -> 54%), rounded to four places,
         never above 1. A chance of 0 or unset (read as 100%, database.LootChance) is left alone.
froglok  An item whose race list is partial (raceuse not 0, not naming all ten of the game's races 1023, and without
         the froglok's bit 1024) gains bit 1024 when its list names every race of the game that can be any of its
         classes (entity.GameClassMask: a human by either city, 0x7EFF) and a froglok can be one of them: every class
         where world.all_classes is on (Heirloom), the S2's east-human mask 0x7E69 where it is off (Live, which makes
         no frogloks today). A list that leaves out a race that can be one of the item's classes is left as it is:
         that is a race restriction the item meant.

As read on 2026-11-22 (Heirloom 19:29 and Live 19:31 UTC pre-swap backups): the step chances and the item counts in
EXPECT. Anything else (another chance, a count off by more than the slack) is refused whole, nothing written.

Every row written is a gm_audit line (via "tool") and a row of owner_decisions_20261122_log; --undo puts each back as
it was (a row changed since is left and said) and drops the log. A report by default; --apply writes once (a second
finds the log and writes nothing); --apply and --undo copy the database first (<db>.bak-<when>-pre-owner1122) unless
--no-backup.

WHEN IT TAKES EFFECT: quests are read again at a GM panel quest edit or reload (handlers.ReloadQuests: the panel's
quest_set_enabled off and on of any one quest), else at the next start; a running server keeps no copy of the steps'
chances past that. Item records are read at each request (a merchant's window at once, a player's own items at their
next login; imported copies at the next start).

    python3 -E -s tools/owner_decisions_20261122.py --db eqoa.db --shard heirloom             report
    python3 -E -s tools/owner_decisions_20261122.py --db eqoa.db --shard heirloom --apply     write it
    python3 -E -s tools/owner_decisions_20261122.py --db eqoa.db --shard heirloom --undo      take it back
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "owner_decisions_20261122_log"
WHO, VIA = "claude", "tool"
DETAIL = ("the owner, 2026-11-22: quest loot steps' miss chance halved; the froglok on every race list that names "
          "each race its classes allow: tools/owner_decisions_20261122.py")

FROGLOK_BIT = 1024
GAME_RACES = 1023
# entity.gameClassMasks, a human by either city (0x7E69 | 0x76F7)
GAME_CLASS_MASKS = [0x7EFF, 0x76E6, 0x7E49, 0x7E41, 0x0245, 0x0109, 0x0141, 0x02C1, 0x7E0C, 0x0909]
ALL_CLASSES = 0x7FFF
FROGLOK_CLASSES = {"heirloom": ALL_CLASSES, "live": 0x7E69}  # world.all_classes on / off

# As read 2026-11-22: the loot steps' chances under 1 with their counts, and the froglok's items.
EXPECT = {
    "heirloom": {"chances": {0.067: 1, 0.08: 5, 0.1: 4, 0.14: 1, 0.15: 3, 0.18: 5, 0.2: 9, 0.25: 37, 0.3: 73,
                             0.33: 14, 0.34: 7, 0.4: 19, 0.5: 1097, 0.75: 3, 0.8: 2},
                 "items": 570},
    "live": {"chances": {0.067: 1, 0.08: 5, 0.1: 4, 0.14: 2, 0.15: 3, 0.18: 5, 0.2: 8, 0.25: 31, 0.3: 73,
                         0.33: 13, 0.34: 7, 0.4: 19, 0.5: 681, 0.75: 3, 0.8: 4},
             "items": 416},
}
SLACK = 10  # rows a count may have moved by since it was read (other work goes on); past it, refused

LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
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
KEYS = {"queststeps": ("questid", "step"), "itempattern": ("patternid",)}


class Refused(Exception):
    pass


def go_json(m):
    """A row as Go's json.Marshal writes a map (keys sorted, <>& escaped), as the panel's audit lines are."""
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def has_table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def raised(chance):
    """The chance with its miss halved, four places, at most 1."""
    return min(1.0, round(chance + (1.0 - chance) / 2.0, 4))


def races_for(classuse):
    """The game's races (a bit each) that can be any of the classes."""
    need = 0
    for r, mask in enumerate(GAME_CLASS_MASKS):
        if mask & classuse:
            need |= 1 << r
    return need


def froglok_gains(classuse, raceuse, froglok_classes):
    c = classuse & ALL_CLASSES
    if raceuse == 0 or raceuse & GAME_RACES == GAME_RACES or raceuse & FROGLOK_BIT or c == 0:
        return False
    if c & froglok_classes == 0:
        return False
    need = races_for(c)
    return raceuse & need == need


class Plan:
    def __init__(self, db, shard):
        self.db, self.shard = db, shard
        self.changes = []  # (tbl, ref, before, after): the changed columns alone
        self.lines = []

    def plan(self):
        exp = EXPECT[self.shard]
        steps = self.db.execute("SELECT questid, step, chance FROM queststeps WHERE kind = 'loot' AND chance > 0 "
                                "AND chance < 1 ORDER BY questid, step").fetchall()
        seen = {}
        for _, _, c in steps:
            seen[c] = seen.get(c, 0) + 1
        other = sorted(set(seen) - set(exp["chances"]))
        if other:
            raise Refused(f"loot step chances not read before: {other}")
        for c, n in sorted(exp["chances"].items()):
            if abs(seen.get(c, 0) - n) > SLACK:
                raise Refused(f"{seen.get(c, 0)} loot steps at {c}, read {n}")
        for qid, step, c in steps:
            self.changes.append(("queststeps", {"questid": qid, "step": step}, {"chance": c}, {"chance": raised(c)}))
        for c in sorted(seen):
            self.lines.append(f"  loot steps at {c:g}: {seen[c]} -> {raised(c):g}")
        self.lines.insert(0, f"drops: {len(steps)} loot steps raised")

        fc = FROGLOK_CLASSES[self.shard]
        items = [r for r in self.db.execute("SELECT patternid, itemname, classuse, raceuse FROM itempattern "
                                            "ORDER BY patternid") if froglok_gains(r[2], r[3], fc)]
        if abs(len(items) - exp["items"]) > SLACK:
            raise Refused(f"{len(items)} items gain the froglok, read {exp['items']}")
        for pid, name, cu, ru in items:
            self.changes.append(("itempattern", {"patternid": pid}, {"raceuse": ru}, {"raceuse": ru | FROGLOK_BIT}))
        self.lines.append(f"froglok: {len(items)} items gain bit 1024 (froglok classes 0x{fc:04X})")
        by = {}
        for _, _, _, ru in items:
            by[ru] = by.get(ru, 0) + 1
        self.lines.append("  by list: " + ", ".join(f"{ru}: {n}" for ru, n in sorted(by.items(), key=lambda x: -x[1])[:8]))
        for pid, name, cu, ru in items[:8]:
            self.lines.append(f"  {pid} {name!r} classes 0x{cu & ALL_CLASSES:04X} races {ru} -> {ru | FROGLOK_BIT}")

    def apply(self):
        db = self.db
        db.execute(LOG_DDL)
        db.execute(AUDIT_DDL)
        at = now()
        n = 0
        for tbl, ref, before, after in self.changes:
            keys = KEYS[tbl]
            cur = db.execute(f"UPDATE {tbl} SET {', '.join(f'{k} = ?' for k in after)} WHERE "
                             + " AND ".join(f"{k} = ?" for k in keys)
                             + "".join(f" AND {k} = ?" for k in before),
                             list(after.values()) + [ref[k] for k in keys] + list(before.values()))
            if cur.rowcount != 1:
                raise Refused(f"{tbl} {ref} moved while it was written")
            aid = audit(db, at, "update", tbl, ref, before, after, f"{DETAIL} ({self.shard})")
            db.execute(f"INSERT INTO {LOG} (tbl, ref, before, after, audit, at) VALUES (?, ?, ?, ?, ?, ?)",
                       (tbl, go_json(ref), go_json(before), go_json(after), aid, at))
            n += 1
        return n


def audit(db, at, action, tbl, ref, before, after, detail):
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, 0, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, WHO, VIA, action, tbl, go_json(ref), go_json(before), go_json(after), detail))
    return cur.lastrowid


def undo(db, shard):
    """Each logged row put back while it is as this tool left it; (back, left)."""
    db.execute(AUDIT_DDL)
    at = now()
    back = left = 0
    for seq, tbl, ref, before, after in db.execute(f"SELECT seq, tbl, ref, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        ref, before, after = json.loads(ref), json.loads(before), json.loads(after)
        keys = KEYS[tbl]
        cur = db.execute(f"UPDATE {tbl} SET {', '.join(f'{k} = ?' for k in before)} WHERE "
                         + " AND ".join(f"{k} = ?" for k in keys) + "".join(f" AND {k} = ?" for k in after),
                         list(before.values()) + [ref[k] for k in keys] + list(after.values()))
        if cur.rowcount != 1:
            print(f"  left {tbl} {ref}: changed since")
            left += 1
            continue
        audit(db, at, "update", tbl, ref, after, before, f"{DETAIL} ({shard}): undone")
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if not left:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def backup(path):
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-owner1122"
    src = sqlite3.connect(path, timeout=5)
    out = sqlite3.connect(dst)
    with out:
        src.backup(out)
    out.close()
    src.close()
    return dst


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--shard", required=True, choices=sorted(EXPECT))
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    ap.add_argument("--no-backup", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, timeout=5)
    try:
        if a.undo:
            if not has_table(db, LOG):
                print("nothing to undo: no log")
                return 0
            if not a.no_backup:
                print(f"backup: {backup(a.db)}")
            back, left = undo(db, a.shard)
            db.commit()
            print(f"undo: {back} rows put back, {left} left")
            return 0
        if has_table(db, LOG):
            n = db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0]
            print(f"applied already ({n} rows in {LOG}): nothing to do")
            return 0
        p = Plan(db, a.shard)
        try:
            p.plan()
        except Refused as e:
            print(f"REFUSED, nothing written: {e}")
            return 2
        for line in p.lines:
            print(line)
        print(f"{len(p.changes)} rows to change")
        if a.apply:
            if not a.no_backup:
                print(f"backup: {backup(a.db)}")
            try:
                n = p.apply()
            except Refused as e:
                db.rollback()
                print(f"REFUSED, nothing written: {e}")
                return 2
            db.commit()
            print(f"applied: {n} rows; quests live at a GM panel quest reload or the next start, items at once")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
