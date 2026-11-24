#!/usr/bin/env python3
"""jib's quest-reward assignments over the level 35-45 side-quest pass, both shards (the owner, 2026-11-24: "keep jibs swaps").

The pass (Live: tools/side_quest_items_35_45_20261123.py, board #2050; Heirloom: tools/petpass_heirloom_1124ad.py,
board #2122) gave every class an item at the last step of eleven side quests. Three of jibrish's planned Heirloom
entries name some of the same records for other quests of that set, from their Allakhazam pages:

  #1068 Kizden G'Lux, 100788 "Three Ambassadors":  WAR 7058 Belt of Giants, SHA 7057 Belt of Four WInds,
                                                   NEC 7064 Belt of the Deathwalker               (AZ 668)
        Parilin / Chastis, 100787 "Old Man Antiok": NEC 5842 Staff of Blood, ALC 5845 Staff of Fire (AZ 671), spells kept
  #1040 Joyce Sandstalker, 100785 "The Three Trials": WAR 7549 Mithron Banded Boots, NEC 7487 Boots of the Grave (AZ 628)
  #1039 Pandrig Fultin, 100789 "The Sword of Olivia McFadden": WAR 6311 Champions Helmet, NEC 6431 Tiara of Spirit (AZ 680)

Each of those cells takes jib's record; the quest the pass had put that record on (same class) takes, in its place,
the record jib's quest gave that class before, so every record still goes to one quest a class and every class keeps
an item at every quest (a swap a class). The pass's picks for the classes jib's entries name nothing for are kept.
Only the cells' "items" change: spells, xp and every other column stay. The rest of the three entries (xp, reach
steps, levels, the pirates, the faction hit, the aggro, record race lists) is not touched here.

Refused, nothing written, if a quest, its last step, or a record is not as read on 2026-11-24 (both shards alike),
or a cell gives neither its before nor its after. A cell already at its after is left (said "already").
--affected lists the characters who finished a changed quest in a changed class, with whether they hold the pass's
record, jib's, or have it waiting in an unapplied chartransfers grant: nothing is taken back and nothing given.

Every row written is a gm_audit line (via "tool") and a row of the log; --undo puts each back as it was (a row
changed since is left and said) and drops the log. A report by default; --apply writes once (a second finds the log
and writes nothing); --apply and --undo copy the database first (<db>.bak-<when>-pre-pet1124ag) unless --no-backup.

WHEN IT TAKES EFFECT: quests are read at the server's start (and at a GM panel quest reload): the next restart.

    python3 -E -s tools/petpass_both_1124ag.py --db eqoa.db               report
    python3 -E -s tools/petpass_both_1124ag.py --db eqoa.db --apply       write it
    python3 -E -s tools/petpass_both_1124ag.py --db eqoa.db --affected    who holds a record that moved
    python3 -E -s tools/petpass_both_1124ag.py --db eqoa.db --undo        take it back
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

LOG = "petpass_both_1124ag_log"
WHO, VIA = "claude", "tool"
DETAIL = ("the owner, 2026-11-24 (\"keep jibs swaps\"): jibrish's board entries 1068/1040/1039 reward records on the "
          "35-45 side quests, swapped with the pass's: tools/petpass_both_1124ag.py")
NAMES = ["WAR", "RAN", "PAL", "SK", "MNK", "BRD", "RGE", "DRD", "SHA", "CLR", "MAG", "NEC", "ENC", "WIZ", "ALC"]
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

# (quest id, name, last step, its kind and NPC)
QUESTS = {100784: ("The Monk Tower", 8, "talk", "Royce Sandstalker"),
          100785: ("The Three Trials", 8, "talk", "Joyce Sandstalker"),
          100786: ("The Restless", 9, "talk", "Broyce Sandstalker"),
          100787: ("Old Man Antiok", 11, "talk", "Gate Guard Chastis"),
          100788: ("Three Ambassadors", 8, "give", "Ambassador Kizden G'Lux"),
          100789: ("The Sword of Olivia McFadden", 9, "give", "Pandrig Fultin")}
RECORDS = {5201: "Ravenblack Robe", 5842: "Staff of Blood", 5845: "Staff of Fire", 6005: "Longsword of Defiance",
           6131: "Aged Tower", 6311: "Champions Helmet", 6431: "Tiara of Spirit", 6929: "Mighty Gauntlets",
           7057: "Belt of Four WInds", 7058: "Belt of Giants", 7064: "Belt of the Deathwalker",
           7487: "Boots of the Grave", 7812: "Bracelet of the Grave", 7883: "Kinetic Bracelet"}
# jib's cells (entry, quest, class, record) and, for each, the pass's quest holding that record for the class
JIB = [(1068, 100788, 0, 7058), (1068, 100788, 8, 7057), (1068, 100788, 11, 7064),
       (1068, 100787, 11, 5842), (1068, 100787, 14, 5845),
       (1040, 100785, 11, 7487),
       (1039, 100789, 0, 6311), (1039, 100789, 11, 6431)]
# quest -> class -> (the pass's record, the record after): jib's cells and the cells they swap with
CELLS = {
    100784: {8: (7057, 6131), 11: (7487, 7812), 14: (5845, 7883)},   # Royce: loses SHA belt, NEC boots, ALC staff
    100785: {11: (7812, 7487)},                                        # Joyce: jib's NEC boots
    100786: {0: (7058, 6005), 11: (7064, 5201)},                       # Broyce: loses WAR and NEC belts
    100787: {0: (6311, 6929), 11: (6431, 5842), 14: (7883, 5845)},    # Chastis: jib's staves, loses WAR helm
    100788: {0: (6005, 7058), 8: (6131, 7057), 11: (5201, 7064)},     # Kizden: jib's three belts
    100789: {0: (6929, 6311), 11: (5842, 6431)},                       # Pandrig: jib's two helms
}


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


def check_tables():
    """the swaps are a permutation a class: every record leaves one quest and arrives at one."""
    for c in range(15):
        out = sorted(v[0] for q in CELLS.values() for k, v in q.items() if k == c)
        into = sorted(v[1] for q in CELLS.values() for k, v in q.items() if k == c)
        if out != into:
            raise Refused(f"{NAMES[c]}: records out {out} and in {into} differ")
    for _, q, c, p in JIB:
        if CELLS[q][c][1] != p:
            raise Refused(f"jib's {q} {NAMES[c]} {p} is not a cell's after")


class Plan:
    def __init__(self, db):
        self.db = db
        self.changes = []  # (tbl, key dict, before row, values)
        self.lines = []

    def build(self):
        db = self.db
        check_tables()
        for pid, name in RECORDS.items():
            r = db.execute("SELECT itemname, classuse FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
            if not r or r[0] != name:
                raise Refused(f"record {pid} is {r}, not {name!r}")
            for q in CELLS.values():
                for c, (_, after) in q.items():
                    if after == pid and not (r[1] >> c) & 1:
                        raise Refused(f"record {pid} {name!r} is not for {NAMES[c]}")
        for qid, cells in sorted(CELLS.items()):
            qname, step, kind, npc = QUESTS[qid]
            q = one(db, f"quest {qid}", "SELECT name FROM quests WHERE questid = ?", (qid,))
            if q["name"] != qname:
                raise Refused(f"quest {qid} is {q['name']!r}, not {qname!r}")
            last = rows(db, "SELECT * FROM queststeps WHERE questid = ? ORDER BY step DESC LIMIT 1", (qid,))
            if not last or last[0]["step"] != step or last[0]["kind"] != kind or last[0]["target"] != npc:
                raise Refused(f"quest {qid}: the last step is {last and (last[0]['step'], last[0]['kind'], last[0]['target'])}")
            last = last[0]
            what = f"queststeps {qid} {qname!r} step {step} ({kind} {npc})"
            bc = json.loads(last["byclass"]) if last["byclass"] else {}
            moves, done = [], 0
            for c, (before, after) in sorted(cells.items()):
                e = dict(bc.get(str(c)) or {})
                have = str(e.get("items", "")).strip()
                if have == str(after):
                    done += 1
                    continue
                if have != str(before):
                    raise Refused(f"{what}: {NAMES[c]} gives items {have!r}, not {before} or {after}")
                e["items"] = str(after)
                bc[str(c)] = e
                moves.append(f"{NAMES[c]} {RECORDS[before]} ({before}) -> {RECORDS[after]} ({after})")
            if not moves:
                self.lines.append(f"already  {what}")
                continue
            if done:
                raise Refused(f"{what}: {done} cells swapped already, {len(moves)} not")
            new = json.dumps(bc, sort_keys=True, separators=(",", ":"))
            self.changes.append(("queststeps", {"questid": qid, "step": step}, last, {"byclass": new}))
            self.lines.append(f"update   {what}: " + "; ".join(moves))


def affected(db):
    """characters who finished a changed quest in a changed class: what they hold of the two records."""
    out = []
    pending = {}
    if has_table(db, "chartransfers"):
        for r in rows(db, "SELECT serverid, plan FROM chartransfers WHERE status = 'approved'"):
            try:
                for it in json.loads(r["plan"] or "{}").get("items") or []:
                    pending.setdefault(r["serverid"], set()).add(int(it.get("pattern", 0)))
            except (ValueError, AttributeError):
                pass
    for qid, cells in sorted(CELLS.items()):
        for c, (before, after) in sorted(cells.items()):
            for ch in rows(db, "SELECT c.serverid, c.charName AS charname FROM charquests q JOIN characters c "
                               "USING (serverid) WHERE q.questid = ? AND q.done > 0 AND c.tclass = ? "
                               "ORDER BY c.serverid", (qid, c)):
                def holds(p):
                    return db.execute("SELECT count(*) FROM charinventory WHERE serverid = ? AND patternid = ?",
                                      (ch["serverid"], p)).fetchone()[0]
                state = []
                if holds(before):
                    state.append(f"holds pass's {before}")
                if before in pending.get(ch["serverid"], ()):
                    state.append(f"pass's {before} in an approved grant")
                if holds(after):
                    state.append(f"holds jib's {after}")
                out.append(f"{ch['serverid']}\t{ch['charname']}\t{NAMES[c]}\t{qid} {QUESTS[qid][0]}\t"
                           f"{RECORDS[before]} -> {RECORDS[after]}\t" + ("; ".join(state) or "holds neither"))
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
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-pet1124ag"
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
    g.add_argument("--affected", action="store_true")
    ap.add_argument("--no-backup", action="store_true")
    a = ap.parse_args(argv)
    log = LOG
    db = sqlite3.connect(a.db, timeout=5, isolation_level=None)
    db.execute("BEGIN IMMEDIATE")
    try:
        logged = has_table(db, log)
        if a.affected:
            print("\n".join(affected(db)))
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
