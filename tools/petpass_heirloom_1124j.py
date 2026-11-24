#!/usr/bin/env python3
"""Heirloom petition 2836 of 2026-11-24, as data: the homelands' level 7 and 15 quests give their classes no armour.

Every row is found by its name (and what else names it), never by an id alone; a row already as this writes it is left
("already"); anything else refuses the whole run.

  2836  "Satyr Bard - I finished all quests up through 23 ... and only the last 23 quest has given armor" (Kyros, a
        level 28 satyr bard). A homeland's quest of a level stands for the city class quests of that level (its
        classquest event writes them finished with no reward, handlers/questevents.go), so it is to give what they
        give. The city class chains (source "EQOA-Data 1-20/<city>/<class>/L<n>") give armour at level 7 and level 15
        to every class but the monk at 15 (a bard: Silken Leggings, Traveling Boots or Melody's Wrist at 7; Thespian
        Leggings, Traveling Bracers or Melody's Breath at 15); the 22 homelands' level 7 and 15 quests (tools/quests/
        gen_homelands.py, class-rewards.json) give a spell alone to all but the ranger (7 and 15) and the monk (7).
        The rule, as tools/petpass_heirloom_1123l.py's: on each homeland's level 7 and level 15 quest, each class its
        gate admits whose reward (the step's byclass) holds no armour gets a piece of its own set of that level: the
        armour the city chains of that level give the class (any city: rewards, byclass, choices, give events), worn
        by every one of the game's races (raceuse 1023: a race of the server's own passes as one of them; Melody's
        Wrist is the elf's alone), its level at most the quest's; the piece the class gets least often over the
        cities' and homelands' quests of the level, then one in a slot the class's homeland chain has not given yet,
        then the lowest id; a class with no such piece at or below the quest's level takes the lowest level above it
        (the druid's 7: Stormreapers Trousers, level 8). Its spell stays. No item is made or changed. A class the cities give no armour at the
        level (the monk at 15) is left.

Every row written is a gm_audit line (via "tool") and a row of the log; --undo puts each back as it was (a row
changed since is left and said) and drops the log. A report by default; --apply writes once (a second finds the log
and writes nothing); --apply and --undo copy the database first (<db>.bak-<when>-pre-pet1124j) unless --no-backup.
--owed lists the characters who finished a quest changed here (the log's) before it gave their class's piece and
do not hold it, as "serverid name class quest pattern item".

WHEN IT TAKES EFFECT: quests are read at the server's start (and at a GM panel quest reload): the next restart.

    python3 -E -s tools/petpass_heirloom_1124j.py --db eqoa.db            report
    python3 -E -s tools/petpass_heirloom_1124j.py --db eqoa.db --apply    write it
    python3 -E -s tools/petpass_heirloom_1124j.py --db eqoa.db --owed     who missed a piece
    python3 -E -s tools/petpass_heirloom_1124j.py --db eqoa.db --undo     take it back
"""
import argparse
import collections
import datetime
import json
import re
import sqlite3
import sys

LOG = "petpass_heirloom_1124j_log"
WHO, VIA = "claude", "tool"
DETAIL = ("Heirloom petition 2836 (2026-11-24): the homelands' level 7 and 15 quests give each class the armour its "
          "city chain gives at that level: tools/petpass_heirloom_1124j.py")

NAMES = ["WAR", "RAN", "PAL", "SK", "MNK", "BRD", "RGE", "DRD", "SHA", "CLR", "MAG", "NEC", "ENC", "WIZ", "ALC"]
LEVELS = (7, 15)
HOMES = 22  # homelands with a quest of each level; a shard with another count is refused
ALL_RACES = 1023  # raceuse: the game's ten races
HELD = range(12, 19)  # equipslot of held items (weapons, shields, wands, tomes)
CITY = re.compile(r"^EQOA-Data 1-20/([^/]+)/([^/]+)/L(\d+) ")
HOME = re.compile(r"^EQOA-Data homelands/([^/]+)/L(\d+)[ /]")
KEYS = {"queststeps": ("questid", "step")}
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


def fold(s):
    return re.sub(r"[^a-z0-9 ]", "", (s or "").lower().replace("'", ""))


def ids(s):
    return [int(x) for x in re.findall(r"\d+", str(s or ""))]


def has_reward(e):
    return bool(str((e or {}).get("items") or "").strip() or str((e or {}).get("spells") or "").strip())


def admitted(classes):
    """the classes a quest's gate lets take it (empty: all)."""
    return set(ids(classes)) if str(classes or "").strip() else set(range(15))




def armour(item):
    return item is not None and item[3] >= 0 and item[3] not in HELD


class Plan:
    def __init__(self, db):
        self.db = db
        self.changes = []  # (tbl, key dict, before row, values)
        self.lines = []
        self.items = {p: (n, cu, lv, sl, ru) for p, n, cu, lv, sl, ru in db.execute(
            "SELECT patternid, itemname, classuse, levelreq, equipslot, raceuse FROM itempattern")}
        self.quests = rows(db, "SELECT questid, name, enabled, classes, source FROM quests ORDER BY questid")
        self.steps = collections.defaultdict(list)
        for s in rows(db, "SELECT questid, step, rewarditems, byclass, choices, events FROM queststeps "
                          "ORDER BY questid, step"):
            self.steps[s["questid"]].append(s)

    def rewards(self, qid, c):
        """[patternid] the quest gives class c: its steps' rewards, byclass, choices' options and give events."""
        out = []
        for s in self.steps[qid]:
            out += ids(s["rewarditems"])
            if s["byclass"]:
                out += ids((json.loads(s["byclass"]).get(str(c)) or {}).get("items"))
            if s["choices"]:
                for o in json.loads(s["choices"]).get("options") or []:
                    out += ids(o.get("items"))
            if s["events"]:
                for e in json.loads(s["events"]):
                    if e.get("do") == "give":
                        out += ids(e.get("items"))
        return out

    def build(self):
        city = collections.defaultdict(list)  # level -> [quest]
        home = collections.defaultdict(dict)  # level -> race -> quest
        for q in self.quests:
            m = CITY.match(q["source"])
            if m and q["enabled"]:
                city[int(m.group(3))].append(q)
            m = HOME.match(q["source"])
            if m and int(m.group(2)) in LEVELS:
                if m.group(1) in home[int(m.group(2))]:
                    raise Refused(f"two homeland quests of {m.group(1)} at level {m.group(2)}")
                home[int(m.group(2))][m.group(1)] = q
        for L in LEVELS:
            if len(home[L]) != HOMES:
                raise Refused(f"{len(home[L])} homelands with a level {L} quest, want {HOMES}")
        given = collections.Counter()  # (level, class, piece) -> how many quests of the level give it
        cands = {}
        for L in LEVELS:
            for c in range(15):
                got = set()
                for q in city[L]:
                    if c in admitted(q["classes"]):
                        for p in self.rewards(q["questid"], c):
                            if armour(self.items.get(p)):
                                given[(L, c, p)] += 1
                                got.add(p)
                for q in home[L].values():
                    if c in admitted(q["classes"]):
                        for p in self.rewards(q["questid"], c):
                            if armour(self.items.get(p)):
                                given[(L, c, p)] += 1
                ok = [p for p in got if (self.items[p][1] >> c) & 1 and self.items[p][4] & ALL_RACES == ALL_RACES]
                low = min([self.items[p][2] for p in ok if self.items[p][2] > L], default=None)
                # none of the quest's level or below: the lowest level above it (the druid's 7: the trousers of 8)
                cands[(L, c)] = sorted(p for p in ok if self.items[p][2] <= L) or \
                    sorted(p for p in ok if self.items[p][2] == low)
        races = sorted(home[LEVELS[0]])
        if races != sorted(home[LEVELS[1]]):
            raise Refused("the homelands of the two levels differ")
        for race in races:
            # the armour slots each class has from its homeland chain (every level), and the picks made
            slots = collections.defaultdict(set)
            for q in self.quests:
                m = HOME.match(q["source"])
                if m and m.group(1) == race:
                    for c in admitted(q["classes"]):
                        for p in self.rewards(q["questid"], c):
                            if armour(self.items.get(p)):
                                slots[c].add(self.items[p][3])
            for L in LEVELS:
                q = home[L][race]
                rs = [s for s in self.steps[q["questid"]] if s["byclass"]]
                what = f"2836 queststeps {q['questid']} {race} L{L} {q['name']}"
                if len(rs) != 1:
                    raise Refused(f"{what}: {len(rs)} byclass steps, want one")
                s = rs[0]
                bc = json.loads(s["byclass"])
                picks, left = [], []
                for c in sorted(admitted(q["classes"])):
                    e = dict(bc.get(str(c)) or {})
                    if any(armour(self.items.get(p)) for p in ids(e.get("items"))):
                        continue
                    if not cands[(L, c)]:
                        left.append(NAMES[c])
                        continue
                    p = min(cands[(L, c)], key=lambda p: (given[(L, c, p)], self.items[p][3] in slots[c], p))
                    given[(L, c, p)] += 1
                    slots[c].add(self.items[p][3])
                    e["items"] = ", ".join([str(x) for x in ids(e.get("items"))] + [str(p)])
                    bc[str(c)] = e
                    picks.append(f"{NAMES[c]} {self.items[p][0]} ({p})")
                what += f" step {s['step']}"
                if left:
                    self.lines.append(f"left     {what}: no city armour at L{L} for {' '.join(left)}")
                if not picks:
                    self.lines.append(f"already  {what} (every admitted class has armour)")
                    continue
                new = json.dumps(bc, sort_keys=True, separators=(",", ":"))
                before = one(self.db, what, "SELECT * FROM queststeps WHERE questid = ? AND step = ?",
                             (s["questid"], s["step"]))
                self.changes.append(("queststeps", {"questid": s["questid"], "step": s["step"]}, before,
                                     {"byclass": new}))
                self.lines.append(f"update   {what}: " + "; ".join(picks))


def owed(db, log):
    """the characters who finished a quest the log changed and hold none of their class's new piece."""
    out = []
    for r in rows(db, f"SELECT ref, before, after FROM {log} WHERE tbl = 'queststeps' ORDER BY seq"):
        key, before, after = json.loads(r["ref"]), json.loads(r["before"]), json.loads(r["after"])
        b, a = json.loads(before["byclass"]), json.loads(after["byclass"])
        name = one(db, "quest", "SELECT name FROM quests WHERE questid = ?", (key["questid"],))["name"]
        for k, e in a.items():
            new = [p for p in ids(e.get("items")) if p not in ids((b.get(k) or {}).get("items"))]
            if not new:
                continue
            for ch in rows(db, "SELECT c.serverid, c.charName AS charname FROM charquests q JOIN characters c USING (serverid) "
                               "WHERE q.questid = ? AND q.done > 0 AND c.tclass = ? ORDER BY c.serverid",
                           (key["questid"], int(k))):
                for p in new:
                    if db.execute("SELECT 1 FROM charinventory WHERE serverid = ? AND patternid = ?",
                                  (ch["serverid"], p)).fetchone():
                        continue
                    item = one(db, "item", "SELECT itemname FROM itempattern WHERE patternid = ?", (p,))["itemname"]
                    out.append(f"{ch['serverid']}\t{ch['charname']}\t{NAMES[int(k)]}\t{key['questid']} {name}"
                               f"\t{p}\t{item}")
    return out


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
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-pet1124j"
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
    ap.add_argument("--owed", action="store_true")
    ap.add_argument("--no-backup", action="store_true")
    a = ap.parse_args(argv)
    log = LOG
    db = sqlite3.connect(a.db, timeout=5, isolation_level=None)
    db.execute("BEGIN IMMEDIATE")
    try:
        logged = has_table(db, log)
        if a.owed:
            if not logged:
                print("not applied: no log")
            else:
                for line in owed(db, log):
                    print(line)
            db.execute("ROLLBACK")
            return 0
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
