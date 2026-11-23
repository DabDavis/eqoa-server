#!/usr/bin/env python3
"""Heirloom petition 2809 of 2026-11-23 (the twelfth batch of the day), as data: the city level 17 quests' class gaps.

Every row is found by its name (and what else names it), never by an id alone; a row already as this writes it is left
("already"); anything else refuses the whole run.

  2809  "17 rivervale quest didn't give magician gear reward" (a level 30 Elf Magician, who finished Tesht'uzu at Ren
        Deeppockets 5 minutes before filing). The city level 17 quests give their class gear by class (queststeps
        byclass); Rivervale's turn-in names nine classes and gives a shadowknight, monk, shaman, magician,
        necromancer and alchemist nothing, though the quest is open to every class (quests.classes empty). The
        2026-11-14 class-gap fill (tools/quest_class_gaps_20261114.py) left them: it gives only a set piece no level
        17 quest gives the class already, and eleven cities' quests had used the sets up. Five more cities had the
        same gap: Fayspire, Halas, Highbourne, Highpass (both its parts), Neriak and Qeynos.

        The rule, for each of the twelve city level 17 quests (source "EQOA-Data 17/<city>/...") and each class its
        gate admits that a class-reward step gives nothing: a piece of the class's own level 17 set (its class-only
        wearable records of level 17 carrying the set's word: Chromatic, of Malice, of the Disciple, Spiritrune, of
        the Shadow, Blue Aura, ...; two records of one name and slot are one piece, the lower id kept), the one the
        level 17 quests give the class least, then, on a step whose rewards are held items (Rivervale's: blades, wands,
        a crook, a shield), a held piece, then the lowest id. No item is made or changed. Rivervale's magician gets the
        Chromatic Tome (5324), held where its enchanter's and wizard's wands are.

Every row written is a gm_audit line (via "tool") and a row of the log; --undo puts each back as it was (a row
changed since is left and said) and drops the log. A report by default; --apply writes once (a second finds the log
and writes nothing); --apply and --undo copy the database first (<db>.bak-<when>-pre-pet1123l) unless --no-backup.

WHEN IT TAKES EFFECT: quests are read at the server's start (and at a GM panel quest reload): the next restart.

    python3 -E -s tools/petpass_heirloom_1123l.py --db eqoa.db            report
    python3 -E -s tools/petpass_heirloom_1123l.py --db eqoa.db --apply    write it
    python3 -E -s tools/petpass_heirloom_1123l.py --db eqoa.db --undo     take it back
"""
import argparse
import datetime
import json
import re
import sqlite3
import sys

LOG = "petpass_heirloom_1123l_log"
WHO, VIA = "claude", "tool"
DETAIL = "Heirloom petition 2809 (2026-11-23): the city level 17 quests' class gaps filled from the class sets: tools/petpass_heirloom_1123l.py"

NAMES = ["WAR", "RAN", "PAL", "SK", "MNK", "BRD", "RGE", "DRD", "SHA", "CLR", "MAG", "NEC", "ENC", "WIZ", "ALC"]
LEVEL = 17
SOURCE = "EQOA-Data 17/"
# the twelve quests as they stand (questid: name): a shard where one differs is refused
QUESTS = {100747: "The Necromancer of Sslathis", 100748: "The Scribe's Inks", 100749: "Froglok Liver",
          100750: "Bloodmaw", 100751: "Ulria Veshiue", 100752: "Grave Silk", 100753: "Courier Svex",
          100754: "Deidre IV", 100755: "Murien's Brew", 100756: "The Ceremony", 100757: "The Shadowhand Receipt",
          100758: "Tesht'uzu"}
RIVERVALE, MAG, TOME = 100758, 10, ("Chromatic Tome", 5324)  # the petition's own pick, checked
HELD = range(12, 19)  # equipslot of held items (weapons, shields, wands, tomes)
SLOT_WORDS = {"of", "the", "a", "and", "tunic", "robe", "helm", "gloves", "boots", "bracers", "bracer", "belt", "sash",
              "leggings", "greaves", "legplates", "pendant", "amulet", "ring", "dagger", "blade", "sword", "wand",
              "shield", "tome", "circlet", "slippers", "insignia", "knuckles", "fist", "chestguard", "tabbard",
              "tabard", "spear", "crook", "ahlspiess", "axe", "foil", "gauntlets", "coif", "crown", "pants",
              "bracelet", "earring", "skullcap", "turban", "legwraps", "shamshir", "breastplate", "staff"}

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


class Plan:
    def __init__(self, db):
        self.db = db
        self.changes = []  # (tbl, key dict, before row, values)
        self.lines = []
        self.items = {p: (n, cu, lv, sl) for p, n, cu, lv, sl in db.execute(
            "SELECT patternid, itemname, classuse, levelreq, equipslot FROM itempattern")}

    def set_of(self, c):
        """[patternid] of the class's level 17 set, one record a piece (name less of/the and slot), lowest id kept."""
        cands = sorted(p for p, (n, cu, lv, sl) in self.items.items()
                       if cu == 1 << c and lv == LEVEL and sl >= 0 and re.search(r"[A-Z]", n or ""))
        words = {}
        for p in cands:
            for w in set(fold(self.items[p][0]).split()) - SLOT_WORDS:
                words[w] = words.get(w, 0) + 1
        if words:
            top = max(words.items(), key=lambda kv: (kv[1], kv[0]))
            if top[1] >= 2:
                cands = [p for p in cands if top[0] in fold(self.items[p][0]).split()]
        out, seen = [], set()
        for p in cands:
            k = (self.items[p][3], tuple(sorted(set(fold(self.items[p][0]).split()) - {"of", "the", "a"})))
            if k not in seen:
                seen.add(k)
                out.append(p)
        return out

    def build(self):
        db = self.db
        qs = rows(db, "SELECT questid, name, classes, minlevel, enabled, source FROM quests WHERE source LIKE ? "
                      "AND minlevel = ? ORDER BY questid", (SOURCE + "%", LEVEL))
        got = {q["questid"]: q["name"] for q in qs}
        if got != QUESTS:
            raise Refused(f"the city level 17 quests are not as this was made for: {got}")
        steps = rows(db, "SELECT s.questid, s.step, s.byclass FROM queststeps s JOIN quests q USING (questid) "
                         "WHERE q.minlevel = ? AND s.byclass != '' ORDER BY s.questid, s.step", (LEVEL,))
        given = {}  # (class, piece id) -> how many level 17 quest steps give it
        for s in steps:
            for k, e in json.loads(s["byclass"]).items():
                if str(k).isdigit():
                    for p in ids((e or {}).get("items")):
                        given[(int(k), p)] = given.get((int(k), p), 0) + 1
        sets = {c: self.set_of(c) for c in range(15)}
        cls = {q["questid"]: admitted(q["classes"]) for q in qs}
        for s in steps:
            if s["questid"] not in QUESTS:
                continue
            bc = json.loads(s["byclass"])
            rewarded = [p for k, e in bc.items() if str(k).isdigit() for p in ids((e or {}).get("items"))]
            held = sum(1 for p in rewarded if p in self.items and self.items[p][3] in HELD) * 2 > len(rewarded)
            picks = []
            for c in sorted(cls[s["questid"]]):
                if has_reward(bc.get(str(c))):
                    continue
                if not sets[c]:
                    raise Refused(f"{s['questid']} step {s['step']} {NAMES[c]}: no level 17 set for the class")
                p = min(sets[c], key=lambda p: (given.get((c, p), 0), (self.items[p][3] in HELD) != held, p))
                given[(c, p)] = given.get((c, p), 0) + 1
                e = dict(bc.get(str(c)) or {})
                e["items"] = str(p)
                bc[str(c)] = e
                picks.append(f"{NAMES[c]} {self.items[p][0]} ({p})")
            if s["questid"] == RIVERVALE:
                want = bc.get(str(MAG), {}).get("items")
                if want != str(TOME[1]) or self.items.get(TOME[1], ("",))[0] != TOME[0]:
                    raise Refused(f"Rivervale's magician pick is {want}, not {TOME}")
            what = f"2809 queststeps {s['questid']} {QUESTS[s['questid']]} step {s['step']}"
            if not picks:
                self.lines.append(f"already  {what} (every admitted class rewarded)")
                continue
            new = json.dumps(bc, sort_keys=True, separators=(",", ":"))
            before = one(db, what, "SELECT * FROM queststeps WHERE questid = ? AND step = ?", (s["questid"], s["step"]))
            self.changes.append(("queststeps", {"questid": s["questid"], "step": s["step"]}, before, {"byclass": new}))
            self.lines.append(f"update   {what}: " + "; ".join(picks))


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
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-pet1123l"
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
