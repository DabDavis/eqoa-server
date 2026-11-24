#!/usr/bin/env python3
"""Heirloom, 2026-11-24 (owner: "yes"): the homelands' level 20 class quests offer each path's spell without its weapon.

Every row is found by its name (and what else names it), never by an id alone; a row already as this writes it is left
("already"); anything else refuses the whole run.

  A homeland's quest of a level stands for the city class quests of that level (its classquest event writes them
  finished with no reward, internal/database/quests.go EventClassQuest), so it is to give what they give
  (tools/petpass_heirloom_1124j.py filled the armour of levels 7 and 15). Compared at every level a homeland quest
  stands in for (1, 2, 3, 4, 5, 7, 10, 13, 15, 20) against the city chains (source "EQOA-Data 1-20/<city>/<class>/L<n>"):
  levels 1-15 give what the cities give (a few cities' own insignia pieces at level 5 aside); at level 20 every city
  "Specialization" quest but the monk's offers a choice of path, each option its spell AND a weapon (a bard: Power
  Dance and the magic foil, Slight Rapier or Melodies Dagger; Sweeping Combat and the magic sabre, Slight Sabre or
  Harmony's Sword), while the 22 homelands' 14 classes' level 20 quests ("The Long Watch" and the like, one a class,
  tools/quests/gen_homelands.py) offer the spell alone.
  The rule, 1124j's extended to the choice: on each homeland level 20 quest, each option of its choice step that holds
  no gear gets one of the pieces the city level 20 quests admitting the class give at an option of the same path (the
  same spell, by name, as a scroll item or a spell), worn by the class and by every one of the game's races (raceuse
  1023: a race of the server's own passes as one; the elf's Dark Steel daggers and Timber Rush Scimitar, the rogues'
  magic dagger and rapier (2015) and the Mystical Tome (1295) are not), its level at most 20; the piece of that path
  the class gets least often over the cities' and homelands' level 20 quests, then the lowest id. Its spell stays.
  No item is made or changed. A path no city gives gear on (the monk's two) is left.

Every row written is a gm_audit line (via "tool") and a row of the log; --undo puts each back as it was (a row
changed since is left and said) and drops the log. A report by default; --apply writes once (a second finds the log
and writes nothing); --apply and --undo copy the database first (<db>.bak-<when>-pre-pet1124q) unless --no-backup.
--owed lists the characters who finished a quest changed here (the log's) before it gave the weapon of the option
they picked (charquests.picks; none recorded: the first option, the city default) and do not hold it, as
"serverid name class quest pattern item".

WHEN IT TAKES EFFECT: quests are read at the server's start (and at a GM panel quest reload): the next restart.

    python3 -E -s tools/petpass_heirloom_1124q.py --db eqoa.db            report
    python3 -E -s tools/petpass_heirloom_1124q.py --db eqoa.db --apply    write it
    python3 -E -s tools/petpass_heirloom_1124q.py --db eqoa.db --owed     who missed a weapon
    python3 -E -s tools/petpass_heirloom_1124q.py --db eqoa.db --undo     take it back
"""
import argparse
import collections
import datetime
import json
import re
import sqlite3
import sys

LOG = "petpass_heirloom_1124q_log"
WHO, VIA = "claude", "tool"
DETAIL = ("Heirloom (owner, 2026-11-24): the homelands' level 20 class quests offer each path's city weapon with its "
          "spell, as the city Specialization quests do: tools/petpass_heirloom_1124q.py")

NAMES = ["WAR", "RAN", "PAL", "SK", "MNK", "BRD", "RGE", "DRD", "SHA", "CLR", "MAG", "NEC", "ENC", "WIZ", "ALC"]
LEVEL = 20
HOMES = 22  # homelands with a level 20 quest of each class; a shard with another count is refused
ALL_RACES = 1023  # raceuse: the game's ten races
CITY = re.compile(r"^EQOA-Data 1-20/([^/]+)/([^/]+)/L(\d+) ")
HOME = re.compile(r"^EQOA-Data homelands/([^/]+)/L20/")
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




def gear(item):
    """worn or held (a scroll, a token: equipslot -1)."""
    return item is not None and item[3] >= 0


class Plan:
    def __init__(self, db):
        self.db = db
        self.changes = []  # (tbl, key dict, before row, values)
        self.lines = []
        self.items = {p: (n, cu, lv, sl, ru) for p, n, cu, lv, sl, ru in db.execute(
            "SELECT patternid, itemname, classuse, levelreq, equipslot, raceuse FROM itempattern")}
        self.spells = {i: (n or "").lower() for i, n in db.execute("SELECT spellid, spellname FROM spellpattern")}
        self.spellnames = set(self.spells.values())
        self.quests = rows(db, "SELECT questid, name, enabled, classes, source FROM quests ORDER BY questid")
        self.steps = collections.defaultdict(list)
        for s in rows(db, "SELECT questid, step, choices FROM queststeps WHERE choices != '' ORDER BY questid, step"):
            self.steps[s["questid"]].append(s)

    def path(self, o):
        """the option's path: its spells' names, as spells or as scroll items."""
        out = {self.spells.get(s, "?") for s in ids(o.get("spells"))}
        out |= {self.items[p][0].lower() for p in ids(o.get("items"))
                if p in self.items and self.items[p][3] < 0 and self.items[p][0].lower() in self.spellnames}
        return frozenset(out)

    def build(self):
        city = collections.defaultdict(set)  # (class, path) -> {piece}
        given = collections.Counter()  # (class, path, piece) -> how many level 20 quests offer it
        home = collections.defaultdict(list)  # class -> [quest]
        for q in self.quests:
            m = CITY.match(q["source"])
            if m and q["enabled"] and int(m.group(3)) == LEVEL:
                for c in admitted(q["classes"]):
                    for s in self.steps[q["questid"]]:
                        for o in json.loads(s["choices"]).get("options") or []:
                            pth = self.path(o)
                            for p in ids(o.get("items")):
                                if pth and gear(self.items.get(p)):
                                    given[(c, pth, p)] += 1
                                    city[(c, pth)].add(p)
            m = HOME.match(q["source"])
            if m:
                cs = admitted(q["classes"])
                if len(cs) != 1:
                    raise Refused(f"homeland quest {q['questid']} {q['source']}: classes {q['classes']!r}, want one")
                home[min(cs)].append(q)
        for c, qs in home.items():
            if len(qs) != HOMES or len({HOME.match(q["source"]).group(1) for q in qs}) != HOMES:
                raise Refused(f"{NAMES[c]}: {len(qs)} homeland level 20 quests, want one in each of {HOMES}")
            for q in qs:
                for s in self.steps[q["questid"]]:
                    for o in json.loads(s["choices"]).get("options") or []:
                        for p in ids(o.get("items")):
                            if gear(self.items.get(p)):
                                given[(c, self.path(o), p)] += 1
        for c in sorted(home):
            for q in sorted(home[c], key=lambda q: q["source"]):
                race = HOME.match(q["source"]).group(1)
                what = f"queststeps {q['questid']} {race} L20 {NAMES[c]} {q['name']}"
                if len(self.steps[q["questid"]]) != 1:
                    raise Refused(f"{what}: {len(self.steps[q['questid']])} choice steps, want one")
                s = self.steps[q["questid"]][0]
                ch = json.loads(s["choices"])
                picks, left = [], []
                for i, o in enumerate(ch.get("options") or []):
                    pth = self.path(o)
                    if not pth:
                        raise Refused(f"{what} option {i}: no spell to name its path")
                    if any(gear(self.items.get(p)) for p in ids(o.get("items"))):
                        continue
                    ok = sorted(p for p in city[(c, pth)] if (self.items[p][1] >> c) & 1
                                and self.items[p][4] & ALL_RACES == ALL_RACES and self.items[p][2] <= LEVEL)
                    if not ok:
                        left.append("/".join(sorted(pth)))
                        continue
                    p = min(ok, key=lambda p: (given[(c, pth, p)], p))
                    given[(c, pth, p)] += 1
                    o["items"] = ", ".join([str(x) for x in ids(o.get("items"))] + [str(p)])
                    picks.append(f"{'/'.join(sorted(pth))} + {self.items[p][0]} ({p})")
                what += f" step {s['step']}"
                if left:
                    self.lines.append(f"left     {what}: no city gear for {', '.join(left)}")
                if not picks:
                    if not left:
                        self.lines.append(f"already  {what} (every option has gear)")
                    continue
                new = json.dumps(ch, separators=(",", ":"), ensure_ascii=False)
                before = one(self.db, what, "SELECT * FROM queststeps WHERE questid = ? AND step = ?",
                             (s["questid"], s["step"]))
                self.changes.append(("queststeps", {"questid": s["questid"], "step": s["step"]}, before,
                                     {"choices": new}))
                self.lines.append(f"update   {what}: " + "; ".join(picks))


def picked(picks, step):
    """the option (from 0) taken at step, or None (charquests.picks "2:1, 3:0")."""
    for part in str(picks or "").split(","):
        a, _, b = part.strip().partition(":")
        if a.strip().isdigit() and b.strip().isdigit() and int(a) == step:
            return int(b)
    return None


def owed(db, log):
    """the characters who finished a quest the log changed and hold none of the new piece of the option they took."""
    out = []
    for r in rows(db, f"SELECT ref, before, after FROM {log} WHERE tbl = 'queststeps' ORDER BY seq"):
        key, before, after = json.loads(r["ref"]), json.loads(r["before"]), json.loads(r["after"])
        b = json.loads(before["choices"])["options"]
        a = json.loads(after["choices"])["options"]
        q = one(db, "quest", "SELECT name, classes FROM quests WHERE questid = ?", (key["questid"],))
        for ch in rows(db, "SELECT c.serverid, c.charName AS charname, c.tclass, q.picks FROM charquests q "
                           "JOIN characters c USING (serverid) WHERE q.questid = ? AND q.done > 0 ORDER BY c.serverid",
                       (key["questid"],)):
            k = picked(ch["picks"], key["step"])
            k = 0 if k is None or k >= len(a) else k
            for p in [p for p in ids(a[k].get("items")) if p not in ids(b[k].get("items"))]:
                if db.execute("SELECT 1 FROM charinventory WHERE serverid = ? AND patternid = ?",
                              (ch["serverid"], p)).fetchone():
                    continue
                item = one(db, "item", "SELECT itemname FROM itempattern WHERE patternid = ?", (p,))["itemname"]
                out.append(f"{ch['serverid']}\t{ch['charname']}\t{NAMES[ch['tclass']]}\t{key['questid']} {q['name']}"
                           f"\t{p}\t{item}\toption {k} ({ch['picks'] or 'none recorded'})")
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
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-pet1124q"
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
