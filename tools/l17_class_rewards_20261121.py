#!/usr/bin/env python3
"""The level 17 side quests' class rewards as the guides give them (the owner, 2026-11-21: "check all the other class
rewards").

The twelve "EQOA-Data 17/<city>/<part>" quests give each class its own item from their last step's byclass. Every
cell was checked against the walkthroughs (https://www.projectreturnhome.com/level-17-class-armor-quest.aspx and the
eqoa.forumotion.com threads t93-t103 under f16-level-17-side-quests, one post each, the same lists). Seven cells name
another item than the guides do; each becomes the existing record the guides name:

    Highpass part 2 (Courier Svex): WAR Soldier's Pendant -> Soldier's Tunic, SK Chestguard of Malice -> Tunic of
      Malice, SHA Spiritrune Amulet -> Spiritrune Tunic, CL Amulet of Radiance -> Tunic of Radiance, ENC Slippers of
      Brilliance -> Wand of Brilliance
    Moradhim (Murien's Brew): MNK Tunic of the Disciple -> Helm of the Disciple (the guides' "Disciple's Helm", as
      their "Disciple's Sash" is our Sash of the Disciple), WIZ Sash of Insight -> Gloves of Insight

Every other cell matches, or the guides name only a kind ("Bracers", "a sword"), or say nothing; those are left.

Everything goes by NAME: a quest by its source and name, a class by its abbreviation, an item by its itemname. The
record a name means is the lowest patternid of that name at level 17 that the class may use (a shard can hold a later
copy of a record under the same name). A cell must hold the old item exactly (a single patternid of that name), or
already the new one (then it is done); anything else, a quest or step not found once, a name with no record, and the
tool refuses to write anything.

Report only by default; --apply writes, a gm_audit line a step, the old items kept in l17_class_rewards_20261121_log;
--undo puts back each cell still as this tool left it. A second --apply finds nothing to do.

WHEN IT TAKES EFFECT: the quests are read again at a GM panel quest edit or reload, else at the server's next start.

    tools/l17_class_rewards_20261121.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "l17_class_rewards_20261121_log"
DETAIL = "level 17 side quests' class rewards as the guides give them: tools/l17_class_rewards_20261121.py"
ACCOUNT, WHO, VIA = 1, "dab", "tool"
LEVEL = 17

CLASSES = {"WAR": 0, "RAN": 1, "PAL": 2, "SK": 3, "MNK": 4, "BRD": 5, "RGE": 6, "DRD": 7, "SHA": 8, "CL": 9,
           "MAG": 10, "NEC": 11, "ENC": 12, "WIZ": 13, "ALC": 14}

# (quest source prefix, quest name, class, item now, item the guides name)
CHANGES = [
    ("EQOA-Data 17/highpass/p2", "Courier Svex", "WAR", "Soldier's Pendant", "Soldier's Tunic"),
    ("EQOA-Data 17/highpass/p2", "Courier Svex", "SK", "Chestguard of Malice", "Tunic of Malice"),
    ("EQOA-Data 17/highpass/p2", "Courier Svex", "SHA", "Spiritrune Amulet", "Spiritrune Tunic"),
    ("EQOA-Data 17/highpass/p2", "Courier Svex", "CL", "Amulet of Radiance", "Tunic of Radiance"),
    ("EQOA-Data 17/highpass/p2", "Courier Svex", "ENC", "Slippers of Brilliance", "Wand of Brilliance"),
    ("EQOA-Data 17/moradhim/main", "Murien's Brew", "MNK", "Tunic of the Disciple", "Helm of the Disciple"),
    ("EQOA-Data 17/moradhim/main", "Murien's Brew", "WIZ", "Sash of Insight", "Gloves of Insight"),
]


class Refused(Exception):
    pass


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def byclass_json(m):
    return json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False)


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def audit(db, qid, step, before, after):
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', 'queststeps', ?, ?, ?, ?)",
               (now(), ACCOUNT, WHO, VIA, go_json({"questid": qid, "step": step}), go_json({"byclass": before}),
                go_json({"byclass": after}), DETAIL))


def record(db, name, cls):
    """The record a name means for a class: the lowest patternid of that name at level 17 the class may use."""
    bit = 1 << CLASSES[cls]
    rows = db.execute("SELECT patternid FROM itempattern WHERE itemname = ? AND levelreq = ? AND (classuse & ?) <> 0 "
                      "ORDER BY patternid", (name, LEVEL, bit)).fetchall()
    if not rows:
        raise Refused(f"no level {LEVEL} record named {name!r} a {cls} may use")
    return rows[0][0], len(rows)


def name_of(db, pid):
    row = db.execute("SELECT itemname FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
    return row[0] if row else None


def reward_step(db, source, qname):
    qs = db.execute("SELECT questid FROM quests WHERE name = ? AND source LIKE ?", (qname, source + " %")).fetchall()
    qs += db.execute("SELECT questid FROM quests WHERE name = ? AND source = ?", (qname, source)).fetchall()
    if len(qs) != 1:
        raise Refused(f"{len(qs)} quests named {qname!r} from {source!r}, not one")
    qid = qs[0][0]
    steps = db.execute("SELECT step, byclass FROM queststeps WHERE questid = ? AND byclass <> '' ORDER BY step",
                       (qid,)).fetchall()
    if len(steps) != 1:
        raise Refused(f"quest {qid} {qname!r}: {len(steps)} steps with class rewards, not one")
    return qid, steps[0][0], steps[0][1]


def plan(db):
    """[(qid, step, qname, cls, old items, new items, old name, new name, copies)] still to do, and the cells done."""
    todo, done = [], []
    for source, qname, cls, old_name, new_name in CHANGES:
        qid, step, blob = reward_step(db, source, qname)
        try:
            bc = json.loads(blob)
        except ValueError:
            raise Refused(f"quest {qid} step {step}: byclass is not JSON")
        cell = bc.get(str(CLASSES[cls]))
        if not isinstance(cell, dict) or set(cell) - {"items"}:
            raise Refused(f"quest {qid} {qname!r} {cls}: cell is {cell!r}, expected only items")
        items = str(cell.get("items", "")).strip()
        new_pid, copies = record(db, new_name, cls)
        if not items.isdigit():
            raise Refused(f"quest {qid} {qname!r} {cls}: items {items!r}, expected one patternid")
        held = name_of(db, int(items))
        if int(items) == new_pid:
            done.append((qid, qname, cls, new_name, new_pid))
            continue
        if held != old_name:
            raise Refused(f"quest {qid} {qname!r} {cls}: holds {items} {held!r}, expected {old_name!r}")
        todo.append((qid, step, qname, cls, items, str(new_pid), old_name, new_name, copies))
    return todo, done


def apply(db, todo):
    db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (questid INTEGER NOT NULL, step INTEGER NOT NULL, class INTEGER NOT NULL, "
               "old TEXT NOT NULL, new TEXT NOT NULL, at TEXT NOT NULL, PRIMARY KEY (questid, step, class))")
    by_step = {}
    for qid, step, _, cls, old, new, *_ in todo:
        by_step.setdefault((qid, step), []).append((CLASSES[cls], old, new))
    at = now()
    for (qid, step), cells in sorted(by_step.items()):
        before = db.execute("SELECT byclass FROM queststeps WHERE questid = ? AND step = ?", (qid, step)).fetchone()[0]
        bc = json.loads(before)
        for c, old, new in cells:
            bc[str(c)]["items"] = new
            db.execute(f"INSERT OR REPLACE INTO {LOG} (questid, step, class, old, new, at) VALUES (?, ?, ?, ?, ?, ?)",
                       (qid, step, c, old, new, at))
        after = byclass_json(bc)
        db.execute("UPDATE queststeps SET byclass = ? WHERE questid = ? AND step = ?", (after, qid, step))
        audit(db, qid, step, before, after)
    return len(todo)


def undo(db):
    if db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (LOG,)).fetchone() is None:
        return 0, 0
    back, left = 0, 0
    rows = db.execute(f"SELECT questid, step, class, old, new FROM {LOG} ORDER BY questid, step, class").fetchall()
    by_step = {}
    for qid, step, c, old, new in rows:
        by_step.setdefault((qid, step), []).append((c, old, new))
    for (qid, step), cells in sorted(by_step.items()):
        row = db.execute("SELECT byclass FROM queststeps WHERE questid = ? AND step = ?", (qid, step)).fetchone()
        if row is None:
            left += len(cells)
            print(f"  quest {qid} step {step}: gone, left")
            continue
        before = row[0]
        bc = json.loads(before)
        put = 0
        for c, old, new in cells:
            cell = bc.get(str(c))
            if not isinstance(cell, dict) or str(cell.get("items", "")).strip() != new:
                left += 1
                print(f"  quest {qid} step {step} class {c}: changed since ({cell!r}), left")
                continue
            cell["items"] = old
            db.execute(f"DELETE FROM {LOG} WHERE questid = ? AND step = ? AND class = ?", (qid, step, c))
            put += 1
        if put:
            after = byclass_json(bc)
            db.execute("UPDATE queststeps SET byclass = ? WHERE questid = ? AND step = ?", (after, qid, step))
            audit(db, qid, step, before, after)
            back += put
    return back, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, timeout=5)
    try:
        if a.undo:
            back, left = undo(db)
            db.commit()
            print(f"undo: {back} class rewards put back, {left} left as changed since")
            return 0
        try:
            todo, done = plan(db)
        except Refused as e:
            print(f"REFUSED, nothing written: {e}")
            return 2
        for qid, step, qname, cls, old, new, old_name, new_name, copies in todo:
            extra = f" ({copies} records of that name, the first taken)" if copies > 1 else ""
            print(f"  {qid} {qname} step {step} {cls}: {old} {old_name} -> {new} {new_name}{extra}")
        for qid, qname, cls, new_name, pid in done:
            print(f"  {qid} {qname} {cls}: already {pid} {new_name}")
        print(f"{len(todo)} class rewards to change, {len(done)} already as the guides give them")
        if a.apply:
            n = apply(db, todo)
            db.commit()
            print(f"applied: {n} class rewards changed")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
