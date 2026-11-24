#!/usr/bin/env python3
"""Heirloom petition 2858 of 2026-11-24, as data: the level 35-45 side quests give no class item on Heirloom.

  2858  Darkhavictwo asks for the class item Live's level 43 Eternal Desert side quests give ("Broyce rewards you").
        The owner, 2026-11-24 ("yes"): give Heirloom what Live was given on 2026-11-23 by
        tools/side_quest_items_35_45_20261123.py (Live petitions 5522-5538), the same quests and the same picks: the
        eleven side quests of the guides' 23-47 family at levels 35-45 that gave no item (Highpass Quests 4 and 6, the
        Oasis 37 three, the Eternal Desert 43 three, the Castle Lightwolf 45 three). QUESTS and PICKS are that tool's,
        imported, so the two shards cannot drift; each quest is found by its id AND its name, its last step by its
        kind and NPC, each record by its id AND its name and its class mask (Heirloom's read the same on 2026-11-24).

Each quest's last step gets the class's item in its byclass, beside any spells there. Refused, nothing written, if a
quest, step or record is not as read here, or a class's cell already gives another item. Unlike the Live tool this
writes no chartransfers grants: --owed lists the characters who finished a quest changed here (the log's) and hold
none of their class's new item, as "serverid name class quest pattern item", for ~/charmigrate's give-if-none.sh
(offline at once, online at logout through watch-addr.sh).

Every row written is a gm_audit line (via "tool") and a row of the log; --undo puts each back as it was (a row
changed since is left and said) and drops the log. A report by default; --apply writes once (a second finds the log
and writes nothing); --apply and --undo copy the database first (<db>.bak-<when>-pre-pet1124ad) unless --no-backup.

WHEN IT TAKES EFFECT: quests are read at the server's start (and at a GM panel quest reload): the next restart.

    python3 -E -s tools/petpass_heirloom_1124ad.py --db eqoa.db            report
    python3 -E -s tools/petpass_heirloom_1124ad.py --db eqoa.db --apply    write it
    python3 -E -s tools/petpass_heirloom_1124ad.py --db eqoa.db --owed     who missed an item
    python3 -E -s tools/petpass_heirloom_1124ad.py --db eqoa.db --undo     take it back
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from side_quest_items_35_45_20261123 import PICKS, QUESTS  # noqa: E402  the Live pass's choices

LOG = "petpass_heirloom_1124ad_log"
WHO, VIA = "claude", "tool"
DETAIL = ("Heirloom petition 2858 (2026-11-24, the owner's yes): the level 35-45 side quests' class items as Live's "
          "side_quest_items_35_45_20261123: tools/petpass_heirloom_1124ad.py")
NAMES = ["WAR", "RAN", "PAL", "SK", "MNK", "BRD", "RGE", "DRD", "SHA", "CLR", "MAG", "NEC", "ENC", "WIZ", "ALC"]
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


class Plan:
    def __init__(self, db):
        self.db = db
        self.changes = []  # (tbl, key dict, before row, values)
        self.lines = []

    def build(self):
        db = self.db
        for qid, cells in PICKS.items():
            if sorted(cells) != list(range(15)):
                raise Refused(f"quest {qid}: picks for classes {sorted(cells)}")
            for c, (pid, name) in cells.items():
                r = db.execute("SELECT itemname, classuse FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
                if not r or r[0] != name or not (r[1] >> c) & 1:
                    raise Refused(f"record {pid} is {r}, not {name!r} for {NAMES[c]}")
        if sorted(PICKS) != sorted(q[0] for q in QUESTS):
            raise Refused("the picks' quests and the quests differ")
        for qid, qname, kind, npc in QUESTS:
            q = one(db, f"quest {qid}", "SELECT name FROM quests WHERE questid = ?", (qid,))
            if q["name"] != qname:
                raise Refused(f"quest {qid} is {q['name']!r}, not {qname!r}")
            steps = rows(db, "SELECT * FROM queststeps WHERE questid = ? ORDER BY step", (qid,))
            if not steps:
                raise Refused(f"quest {qid}: no steps")
            last = steps[-1]
            what = f"2858 queststeps {qid} {qname!r} step {last['step']} ({kind} {npc})"
            if last["kind"] != kind or last["target"] != npc:
                raise Refused(f"{what}: the last step is {last['kind']} at {last['target']!r}")
            bc = json.loads(last["byclass"]) if last["byclass"] else {}
            picks, have = [], 0
            for c in range(15):
                e = dict(bc.get(str(c)) or {})
                pid = PICKS[qid][c][0]
                if e.get("items"):
                    if str(e["items"]).strip() == str(pid):
                        have += 1
                        continue
                    raise Refused(f"{what}: {NAMES[c]} is given items {e['items']!r} already")
                e["items"] = str(pid)
                bc[str(c)] = e
                picks.append(f"{NAMES[c]} {PICKS[qid][c][1]} ({pid})")
            if not picks:
                self.lines.append(f"already  {what}")
                continue
            if have:
                raise Refused(f"{what}: {have} classes have their item, {len(picks)} not")
            new = json.dumps(bc, sort_keys=True, separators=(",", ":"))
            self.changes.append(("queststeps", {"questid": qid, "step": last["step"]}, last, {"byclass": new}))
            self.lines.append(f"update   {what}: " + "; ".join(picks))


def owed(db, log):
    """the characters who finished a quest the log changed and hold none of their class's new item."""
    out = []
    for r in rows(db, f"SELECT ref, before, after FROM {log} WHERE tbl = 'queststeps' ORDER BY seq"):
        key, before, after = json.loads(r["ref"]), json.loads(r["before"]), json.loads(r["after"])
        b = json.loads(before["byclass"]) if before["byclass"] else {}
        a = json.loads(after["byclass"])
        name = one(db, "quest", "SELECT name FROM quests WHERE questid = ?", (key["questid"],))["name"]
        for k, e in sorted(a.items(), key=lambda kv: int(kv[0])):
            if not e.get("items") or (b.get(k) or {}).get("items") == e["items"]:
                continue
            p = int(e["items"])
            item = one(db, "item", "SELECT itemname FROM itempattern WHERE patternid = ?", (p,))["itemname"]
            for ch in rows(db, "SELECT c.serverid, c.charName AS charname FROM charquests q JOIN characters c "
                               "USING (serverid) WHERE q.questid = ? AND q.done > 0 AND c.tclass = ? "
                               "ORDER BY c.serverid", (key["questid"], int(k))):
                if db.execute("SELECT 1 FROM charinventory WHERE serverid = ? AND patternid = ?",
                              (ch["serverid"], p)).fetchone():
                    continue
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


def apply(db, plan, log):
    db.execute(LOG_DDL.format(log=log))
    for tbl, key, before, values in plan.changes:
        w, args = where(key)
        db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in values)} WHERE {w}",
                   list(values.values()) + args)
        after = rows(db, f"SELECT * FROM {tbl} WHERE {w}", args)[0]
        a = audit(db, "update", tbl, key, before, after)
        db.execute(f"INSERT INTO {log} (tbl, ref, before, after, audit, at) VALUES (?, ?, ?, ?, ?, ?)",
                   (tbl, json.dumps(key), json.dumps(before), json.dumps(after), a, now()))


def undo(db, log):
    out = []
    for r in rows(db, f"SELECT * FROM {log} ORDER BY seq DESC"):
        tbl, key = r["tbl"], json.loads(r["ref"])
        w, args = where(key)
        now_rows = rows(db, f"SELECT * FROM {tbl} WHERE {w}", args)
        after, before = json.loads(r["after"]), json.loads(r["before"])
        if len(now_rows) != 1 or now_rows[0] != after:
            out.append(f"left     {tbl} {key}: changed since")
            continue
        cols = list(before)
        db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in cols)} WHERE {w}",
                   [before[c] for c in cols] + args)
        audit(db, "update", tbl, key, after, before)
        out.append(f"restored {tbl} {key}")
    db.execute(f"DROP TABLE {log}")
    return out


def backup(path):
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-pet1124ad"
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
    g.add_argument("--owed", action="store_true")
    ap.add_argument("--no-backup", action="store_true")
    a = ap.parse_args(argv)
    log = LOG
    db = sqlite3.connect(a.db, timeout=5, isolation_level=None)
    db.execute("BEGIN IMMEDIATE")
    try:
        logged = has_table(db, log)
        if a.owed:
            print("\n".join(owed(db, log)) if logged else "not applied: no log")
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
