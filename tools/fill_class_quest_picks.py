#!/usr/bin/env python3
"""The city class quests' item ties and misfits, filled with the suggestions the user approved (2026-10-30,
of lane-rewards2's review, "lets fix that also").

tools/seed_class_quest_rewards.py wrote the 29 quests whose other cities give different items (a tie)
or an item the quest's story does not fit (a misfit) for the user to pick (its REVIEW, --item-review),
with a suggestion each. The user took the suggestions. 26 of them are a record as it is (Freeport's
militia short sword for four warriors' level-4 quests, the Sturdy Two-Handed Sword for six warriors'
level-10 ones, Corse Spiked Club for the shamans' ...); three quests titled for a staff whose class
has no staff of the level are given one made for them as New Gear's were (MADE: a copy of a staff of
the level, the class's mask, under its quest's name, a line saying who made it of what).

Each quest's last step is given its pick; a quest whose last step gives an item already is a problem
(refused). Every item given is checked as the class masks were made right: the quest's class in its
mask and the races that start in its city as that class in its race mask. A character of the quest's
class who finished it (charquests done) and lacks the item is given it as a quest gives one (a full
inventory, 40, is reported). Every row made or changed is logged in class_quest_pick_log; --undo takes
back only what is still as this left it and keeps a made record someone holds. One transaction. A
report by default (--tsv FILE the list); --write-data DIR names the same items in the quest files (text,
their layout); --records-only makes the three records alone (a database the quest files are imported
into afresh, before the import). The quests load at the server's start: a restart (or a GM panel quest
save, which reloads them) after --apply; grants go straight to charinventory: apply with the receivers
offline.

    tools/fill_class_quest_picks.py [--db eqoa.db] [--apply | --undo] [--records-only] [--tsv FILE]
                                    [--write-data tools/quests/data]
"""

import argparse
import collections
import csv
import json
import os
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import fill_class_quest_rewards as fc  # noqa: E402
import fill_weapon_durability as fwd  # noqa: E402
import seed_class_quest_rewards as sc  # noqa: E402

STAMP = "the user's pick of 2026-10-30, tools/fill_class_quest_picks.py"

# The records made for the three quests the review suggested a staff for and no record of the class is:
# (id, quest, its class, the item's name, the record copied, the line, why)
MADE = [
    (709200, 100340, 11, "Bal's Staff", 707114,
     "Carved by Bal Noonlittle from a small gear wheel, a meat chunk, a sliver of snake meat and a fractured firefly thorax.",
     "the Staff of Necromancy's numbers (the necromancer's level-4 staff made for Highbourne), under its own quest's name"),
    (709201, 100024, 12, "Salithal's Staff", 8585,
     "Carved by Salithal Grassweave from a bough of wood, a leather strip and a ruined bat wing.",
     "the wizard's level-4 staff of evocation, for the enchanter (no enchanter's staff of level 3-6 is)"),
    (709202, 100320, 12, "Valespin's Staff", 8585,
     "Made by Prof. Valespin from an empty bottle, a small metal rod, snake scales and a cracked rat claw.",
     "the wizard's level-4 staff of evocation, for the enchanter (no enchanter's staff of level 3-6 is)"),
]


def picks():
    """quest -> (the record, why): the review's suggestions, the three without a record MADE's."""
    made = {q: (pid, why) for pid, q, _, _, _, _, why in MADE}
    out = {}
    for qid, (pid, why) in sc.REVIEW.items():
        if pid is None and qid not in made:
            raise SystemExit(f"quest {qid}: the review suggests no record and MADE makes none")
        out[qid] = made.get(qid) or (pid, why)
    return out


LOG = "class_quest_pick_log"
LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl TEXT NOT NULL,
    rid TEXT NOT NULL,
    col TEXT NOT NULL,
    old TEXT,
    new TEXT
)"""


class Plan:
    def __init__(self, db):
        self.db = db
        self.rec = fc.Records(db)
        self.quests = fc.load(db, self.rec)
        self.rec.city_races(db, self.quests)
        self.made, self.fills, self.problems = [], [], []

    def make_records(self):
        hp = fwd.level_hp(self.db)
        for pid, qid, c, name, base, line, why in MADE:
            b = sc.row(self.db, "SELECT * FROM itempattern WHERE patternid = ?", (base,))
            if b is None:
                self.problems.append(f"quest {qid}: the record {base} to copy is not here")
                continue
            r = dict(b, patternid=pid, itemname=name, itemdesc=line, classuse=1 << c)
            if not r["maxhp"]:
                r["maxhp"] = fwd.hp_for(r["levelreq"], hp)
            if not r["duration"]:
                r["duration"] = fwd.DURATION
            here = sc.row(self.db, "SELECT itemname FROM itempattern WHERE patternid = ?", (pid,))
            if here is not None:
                if here["itemname"] != name:
                    self.problems.append(f"id {pid} is {here['itemname']!r} here, not {name!r}")
                continue  # made by an earlier run
            if self.rec.by_fold.get(fc.fold(name)):
                self.problems.append(f"{name!r}: item {self.rec.by_fold[fc.fold(name)][0]} has the name")
                continue
            self.made.append(dict(row=r, why=f"a copy of {b['itemname']} ({base}), for the {fc.CLASSES[c]}: {why}"))
            self.rec.item[pid] = dict(name=name, scroll=False, level=r["levelreq"], classes=r["classuse"], races=r["raceuse"],
                                      maxhp=r["maxhp"], slot=r["equipslot"], desc=line)
            self.rec.by_fold[fc.fold(name)].append(pid)

    def build(self, records_only=False):
        self.make_records()
        if records_only:
            return
        made_class = {q: c for _, q, c, _, _, _, _ in MADE}
        for qid, (pid, why) in picks().items():
            q = self.quests.get(qid)
            if q is None:
                self.problems.append(f"quest {qid} is not a city class quest here")
                continue
            if qid in made_class and made_class[qid] != q["c"]:
                self.problems.append(f"quest {qid} is a {fc.CLASSES[q['c']]}'s, not a {fc.CLASSES[made_class[qid]]}'s")
                continue
            if pid in q["items"]:
                continue  # given already
            if q["items"]:
                self.problems.append(f"quest {qid}: its last step gives an item already: left")
                continue
            it = self.rec.item.get(pid)
            if it is None:
                self.problems.append(f"quest {qid}: record {pid} is not here")
                continue
            # the class masks right: the quest's class, and the races that start in its city as it
            if not it["classes"] >> q["c"] & 1:
                self.problems.append(f"quest {qid}: {it['name']!r} ({pid}) is not the {fc.CLASSES[q['c']]}'s (mask {it['classes']})")
                continue
            need = self.rec.at.get((q["city"], q["c"])) or self.rec.races.get(q["c"]) or 1023
            if it["races"] & need != need:
                self.problems.append(f"quest {qid}: {it['name']!r} ({pid}) misses races {need & ~it['races']} of {q['city']}")
                continue
            if self.rec.file_name(pid) is None:
                self.problems.append(f"quest {qid}: the quest file cannot name {pid} {it['name']!r}")
                continue
            before = q["last"]["rewarditems"]
            f = dict(qid=qid, key=q["key"], file=q["file"], city=q["city"], c=q["c"], cls=fc.CLASSES[q["c"]], level=q["level"],
                     name=q["name"], last_step=q["last"]["step"], rewards_before=before,
                     rewards_after=f"{before}, {pid}" if before else str(pid), item=pid, why=why,
                     note=f"The reward is ours: {it['name']}, {why} ({STAMP}).")
            f["owed"] = self.owed(q, pid)
            self.fills.append(f)

    def owed(self, q, pid):
        out = []
        for sid, name, cls in self.db.execute(
                "SELECT c.serverid, h.charName, h.tclass FROM charquests c JOIN characters h ON h.serverid = c.serverid "
                "WHERE c.questid = ? AND c.done = 1 ORDER BY c.serverid", (q["qid"],)):
            if cls != q["c"] or self.db.execute("SELECT 1 FROM charinventory WHERE serverid = ? AND patternid = ?", (sid, pid)).fetchone():
                continue
            held = self.db.execute("SELECT count(*) FROM charinventory WHERE serverid = ? AND location = 0", (sid,)).fetchone()[0]
            out.append((sid, name, held + 1 > fc.INV_CAP))
        return out


def log(db, tbl, rid, col, old, new):
    db.execute(f"INSERT INTO {LOG} (tbl, rid, col, old, new) VALUES (?, ?, ?, ?, ?)",
               (tbl, str(rid), col, None if old is None else json.dumps(old), None if new is None else json.dumps(new)))


def apply(db, p):
    db.execute(LOG_DDL)
    n = collections.Counter()
    for m in p.made:
        r = m["row"]
        cols = list(r)
        db.execute(f"INSERT INTO itempattern ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [r[c] for c in cols])
        log(db, "itempattern", r["patternid"], "*", None, r)
        n["made"] += 1
    for f in p.fills:
        cur = db.execute("SELECT rewarditems FROM queststeps WHERE questid = ? AND step = ?", (f["qid"], f["last_step"])).fetchone()[0]
        if cur != f["rewards_before"]:
            raise SystemExit(f"quest {f['qid']} step {f['last_step']} changed under the plan")
        db.execute("UPDATE queststeps SET rewarditems = ? WHERE questid = ? AND step = ?", (f["rewards_after"], f["qid"], f["last_step"]))
        log(db, "queststeps", f"{f['qid']}/{f['last_step']}", "rewarditems", cur, f["rewards_after"])
        old_notes = db.execute("SELECT notes FROM quests WHERE questid = ?", (f["qid"],)).fetchone()[0]
        notes = (old_notes + " " if old_notes else "") + f["note"]
        db.execute("UPDATE quests SET notes = ? WHERE questid = ?", (notes, f["qid"]))
        log(db, "quests", f["qid"], "notes", old_notes, notes)
        n["quests"] += 1
        for sid, _, full in f["owed"]:
            if full:
                n["full"] += 1
                continue
            cur = db.execute(
                "INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, "
                "listnumber) VALUES (?, 1, (SELECT COALESCE(maxhp, 0) FROM itempattern WHERE patternid = ?), 0, ?, -1, 0, "
                "(SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory WHERE serverid = ? AND location = 0))",
                (sid, f["item"], f["item"], sid))
            log(db, "charinventory", cur.lastrowid, "*", None, {"serverid": sid, "patternid": f["item"], "questid": f["qid"]})
            n["given"] += 1
    return n


def undo(db):
    if not fc.has_table(db, LOG):
        print("nothing logged: nothing to undo")
        return
    back = kept = 0
    for seq, tbl, rid, col, old, new in db.execute(f"SELECT seq, tbl, rid, col, old, new FROM {LOG} ORDER BY seq DESC").fetchall():
        old = json.loads(old) if old is not None else None
        new = json.loads(new) if new is not None else None
        if tbl == "charinventory":
            cur = db.execute("DELETE FROM charinventory WHERE itemid = ? AND serverid = ? AND patternid = ?",
                             (int(rid), new["serverid"], new["patternid"]))
        elif tbl == "queststeps":
            q, st = rid.split("/")
            cur = db.execute("UPDATE queststeps SET rewarditems = ? WHERE questid = ? AND step = ? AND rewarditems = ?",
                             (old, int(q), int(st), new))
        elif tbl == "quests":
            cur = db.execute("UPDATE quests SET notes = ? WHERE questid = ? AND notes = ?", (old, int(rid), new))
        elif tbl == "itempattern":
            now = sc.row(db, "SELECT * FROM itempattern WHERE patternid = ?", (int(rid),))
            held = db.execute("SELECT count(*) FROM charinventory WHERE patternid = ?", (int(rid),)).fetchone()[0]
            if now == new and not held:
                db.execute("DELETE FROM itempattern WHERE patternid = ?", (int(rid),))
                back += 1
            else:
                kept += 1
                print(f"kept item {rid} {new['itemname']!r}: {'held by ' + str(held) + ' records' if held else 'changed since'}")
            continue
        else:
            continue
        back, kept = (back + 1, kept) if cur.rowcount else (back, kept + 1)
    db.execute(f"DROP TABLE {LOG}")
    print(f"put back {back}; left {kept} changed since")


def write_data(datadir, p):
    """Name each pick on its quest's last step in the quest files, and its note: the quest's own text alone."""
    by_file = collections.defaultdict(list)
    for f in p.fills:
        by_file[f["file"]].append(f)
    changed = 0
    for fname, fs in sorted(by_file.items()):
        path = os.path.join(datadir, fname)
        with open(path, encoding="utf-8") as fh:
            text = fh.read()
        for f in fs:
            sp = fc._span(text, f["key"])
            if sp is None:
                raise SystemExit(f"{fname}: no quest {f['key']!r}")
            s, e, ind = sp
            q = json.loads(text[s:e])
            if fc._dump(q, ind) != text[s:e]:
                raise SystemExit(f"{fname}: {f['key']!r} is not laid out as json.dump(indent=1) writes it: left")
            items = q["steps"][-1].setdefault("rewards", {}).setdefault("items", [])
            nm = p.rec.file_name(f["item"])
            if nm is None:
                raise SystemExit(f"{fname}: {f['key']!r}: item {f['item']} the file cannot name")
            if nm not in items:
                items.append(nm)
            if f["note"] not in (q.get("notes") or ""):
                q["notes"] = ((q.get("notes") or "") + " " + f["note"]).strip()
            text = text[:s] + fc._dump(q, ind) + text[e:]
            changed += 1
        json.loads(text)
        with open(path, "w", encoding="utf-8") as out:
            out.write(text)
    return changed


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", default="eqoa.db")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    ap.add_argument("--records-only", action="store_true", help="make the three records alone")
    ap.add_argument("--tsv", help="write the list here")
    ap.add_argument("--write-data", metavar="DIR", help="name the picks in the quest files of DIR")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 10000")
    if a.undo:
        db.execute("BEGIN IMMEDIATE")
        try:
            undo(db)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        return
    if a.apply and fc.has_table(db, LOG) and db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0]:
        sys.exit(f"{LOG} holds an earlier apply: --undo it first")
    p = Plan(db)
    p.build(records_only=a.records_only)
    for m in p.made:
        r = m["row"]
        print(f"make {r['patternid']} {r['itemname']!r} (level {r['levelreq']}, classes {r['classuse']}, races {r['raceuse']}, "
              f"HP {r['maxhp']}): {m['why']}")
    for f in p.fills:
        given = ", ".join(f"{n} ({s}){' FULL' if full else ''}" for s, n, full in f["owed"])
        print(f"{f['cls']} L{f['level']} {f['city']} {f['qid']} {f['name']!r}: {f['item']} {p.rec.item[f['item']]['name']!r}"
              + (f"; given to {given}" if given else ""))
    for x in p.problems:
        print("PROBLEM:", x)
    print(f"{len(p.made)} items to make; {len(p.fills)} quests to fill; {sum(len(f['owed']) for f in p.fills)} characters owed")
    if a.tsv:
        made = {m["row"]["patternid"]: m["why"] for m in p.made}
        with open(a.tsv, "w", newline="") as out:
            w = csv.writer(out, delimiter="\t", lineterminator="\n")
            w.writerow(["questid", "key", "city", "class", "level", "quest", "item", "patternid", "why", "made", "given_to"])
            for f in p.fills:
                w.writerow([f["qid"], f["key"], f["city"], f["cls"], f["level"], f["name"], p.rec.item[f["item"]]["name"], f["item"],
                            f["why"], made.get(f["item"], ""), ", ".join(f"{n} ({s})" for s, n, _ in f["owed"])])
    if a.write_data:
        print(f"named in the quest files: {write_data(a.write_data, p)}")
    if not a.apply:
        print("report only: --apply to write")
        return
    if p.problems:
        sys.exit("refused: the problems above first")
    db.execute("BEGIN IMMEDIATE")
    try:
        n = apply(db, p)
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"made {n['made']} items, filled {n['quests']} quests, {n['given']} given ({n['full']} not: a full inventory); "
          "restart the server (or save a quest in the GM panel) to load the quests")


if __name__ == "__main__":
    main()
