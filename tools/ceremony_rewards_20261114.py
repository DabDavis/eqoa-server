#!/usr/bin/env python3
"""Neriak's level 17 side quest gives its class reward (petition 5028, Trulyblind, Live, 2026-11-14: "NO quest reward
For the 17s? From Denouncer G`Efia?").

"The Ceremony" (quest 100756, Tanadril N`Thael; EQOA-Data Quests/17/neriak-nektulos.json) ends at Denouncer G`Efia,
who "will then give you a Class Specific item" (Quest Masters, external/questmasters/text/id161.txt). Neither guide
names the items, so the import left her step (4) with no reward: the only level 17 city quest of the eleven without
one. Every other city's level 17 quest gives each class an item of its own on its last step (queststeps.byclass).

  queststeps 100756 step 4 byclass: each class the item its level 17 reward is in the other cities, taken from the
    evil cities first (Grobb's "Froglok Liver", Klick'Anon's "Deidre IV", Freeport's "The Scribe's Inks"), then the
    rest (Highpass, Moradhim, Halas, Qeynos, Highbourne, Rivervale, Fayspire): the first of them that gives the class
    one. A shadow knight gets Grobb's Gloves of Malice, a magician Halas's Chromatic Dagger; an alchemist none (no
    city gives one: the experience alone, as everywhere). Each item's class and race masks let a dark elf of its
    class wear it (checked). The quest's notes say the rewards are ours.
  The characters of a class that finished it (charquests done) and lack their class's item are given it the way a
    quest gives one (tools/fill_class_quest_rewards.py: remainHP the record's maxhp, the pack's next place); a full
    pack (40) is reported, not given. Live 2026-11-14: Trulyblind (shadow knight: Gloves of Malice) and Groceries
    (magician: Chromatic Dagger).

The quests load at the server's start, and the server holds a player's pack while they play: run it in a keep
restart's before.sh (or with the finishers out of the world). Refused, nothing written, when the step is not as the
import wrote it (no reward) or an item is missing. Report only by default; --apply in one transaction, a gm_audit
line a row, log ceremony_rewards_1114_log; --undo exact (an item given is taken back only while it is where it was
put, unworn).

    tools/ceremony_rewards_20261114.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "ceremony_rewards_1114_log"
DETAIL = "petition 5028 (Trulyblind): The Ceremony's class rewards from G`Efia: tools/ceremony_rewards_20261114.py"
WHO, VIA = "claude", "tool"
QUEST, STEP = 100756, 4
TARGET = "Denouncer G`Efia"
# the other cities' level 17 quests, the evil cities first
ORDER = [100749, 100754, 100748, 100752, 100755, 100750, 100757, 100751, 100758, 100747]
INV_CAP = 40
NOTE = (" The rewards on G`Efia's step are ours (petition 5028, tools/ceremony_rewards_20261114.py): each class's level "
        "17 item of the other cities' quests, the evil cities' first (Grobb, Klick'Anon, Freeport).")

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    rid    TEXT NOT NULL,
    col    TEXT NOT NULL,
    old    TEXT,
    new    TEXT,
    audit  INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def compact(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False, sort_keys=True)


def audit(db, action, tbl, key, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, compact(key), compact(before or {}),
                      compact(after or {}), DETAIL))
    return cur.lastrowid


def rewards(db):
    """({class: patternid}, refusals)."""
    out, refusals = {}, []
    for q in ORDER:
        r = db.execute("SELECT byclass FROM queststeps WHERE questid = ? AND byclass != '' ORDER BY step DESC", (q,)).fetchone()
        if r is None:
            continue
        for c, v in json.loads(r[0]).items():
            c = int(c)
            if c in out or not v.get("items"):
                continue
            ids = [int(x) for x in str(v["items"]).split(",") if x.strip()]
            if len(ids) != 1:
                continue
            pid = ids[0]
            it = db.execute("SELECT classuse, raceuse FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
            if it is None:
                refusals.append(f"item {pid} (quest {q}'s, class {c}) is not here")
                continue
            if not (it[0] >> c) & 1 or not (it[1] >> 2) & 1:  # its class, a dark elf (race 2)
                continue
            out[c] = pid
    return out, refusals


def plan(db):
    changes, lines, refusals = [], [], []
    st = db.execute("SELECT kind, target, rewarditems, byclass FROM queststeps WHERE questid = ? AND step = ?",
                    (QUEST, STEP)).fetchone()
    if st is None:
        return changes, lines, [f"queststeps {QUEST} step {STEP} is not here"]
    if st[1] != TARGET or st[0] != "talk":
        return changes, lines, [f"queststeps {QUEST} step {STEP} is {st[0]} {st[1]!r}, not talk {TARGET!r}"]
    if st[2]:
        return changes, lines, [f"its step gives {st[2]!r} already: look first"]
    by, refusals = rewards(db)
    if refusals:
        return changes, lines, refusals
    want = compact({str(c): {"items": str(p)} for c, p in sorted(by.items())})
    if st[3] == want:
        lines.append("G`Efia's step has its rewards already")
    elif st[3]:
        return changes, lines, [f"its step's byclass is {st[3]!r}: look first"]
    else:
        changes.append(("queststeps", f"{QUEST},{STEP}", "byclass", "", want))
        for c, p in sorted(by.items()):
            n = db.execute("SELECT itemname FROM itempattern WHERE patternid = ?", (p,)).fetchone()[0]
            lines.append(f"class {c}: {n} ({p})")
        notes = db.execute("SELECT notes FROM quests WHERE questid = ?", (QUEST,)).fetchone()[0]
        if NOTE.strip() not in notes:
            changes.append(("quests", str(QUEST), "notes", notes, notes + NOTE))
    for sid, name, cls in db.execute("SELECT c.serverid, h.charName, h.tclass FROM charquests c JOIN characters h "
                                     "ON h.serverid = c.serverid WHERE c.questid = ? AND c.done >= 1 ORDER BY c.serverid",
                                     (QUEST,)).fetchall():
        pid = by.get(cls)
        if pid is None:
            lines.append(f"{name} ({sid}), class {cls}: no item for the class (the experience alone)")
            continue
        if db.execute("SELECT 1 FROM charinventory WHERE serverid = ? AND patternid = ?", (sid, pid)).fetchone():
            lines.append(f"{name} ({sid}) holds item {pid} already")
            continue
        held = db.execute("SELECT count(*) FROM charinventory WHERE serverid = ? AND location = 0", (sid,)).fetchone()[0]
        if held >= INV_CAP:
            lines.append(f"{name} ({sid}): pack full ({held}): NOT given item {pid}, a GM gives it")
            continue
        changes.append(("give", str(sid), str(pid), None, None))
        lines.append(f"{name} ({sid}), class {cls}: given item {pid}")
    return changes, lines, refusals


def apply(db, changes):
    db.execute(LOG_SCHEMA)
    for tbl, rid, col, old, new in changes:
        if tbl == "give":
            sid, pid = int(rid), int(col)
            cur = db.execute(
                "INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, "
                "listnumber) VALUES (?, 1, (SELECT COALESCE(maxhp, 0) FROM itempattern WHERE patternid = ?), 0, ?, -1, 0, "
                "(SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory WHERE serverid = ? AND location = 0))",
                (sid, pid, pid, sid))
            item = cur.lastrowid
            row = dict(zip([d[0] for d in db.execute("SELECT * FROM charinventory WHERE itemid = ?", (item,)).description],
                           db.execute("SELECT * FROM charinventory WHERE itemid = ?", (item,)).fetchone()))
            a = audit(db, "insert", "charinventory", {"itemid": item}, None, row)
            db.execute(f"INSERT INTO {LOG} (tbl, rid, col, old, new, audit) VALUES (?, ?, ?, ?, ?, ?)",
                       ("charinventory", str(item), "*", None, compact(row), a))
            continue
        if tbl == "queststeps":
            q, s = (int(x) for x in rid.split(","))
            if db.execute(f"UPDATE queststeps SET {col} = ? WHERE questid = ? AND step = ? AND {col} = ?",
                          (new, q, s, old)).rowcount != 1:
                raise RuntimeError(f"queststeps {rid} changed under the update")
            key = {"questid": q, "step": s}
        else:
            if db.execute(f"UPDATE quests SET {col} = ? WHERE questid = ? AND {col} = ?", (new, int(rid), old)).rowcount != 1:
                raise RuntimeError(f"quests {rid} changed under the update")
            key = {"questid": int(rid)}
        a = audit(db, "update", tbl, key, {col: old}, {col: new})
        db.execute(f"INSERT INTO {LOG} (tbl, rid, col, old, new, audit) VALUES (?, ?, ?, ?, ?, ?)",
                   (tbl, rid, col, json.dumps(old), json.dumps(new), a))


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, tbl, rid, col, old, new in db.execute(f"SELECT seq, tbl, rid, col, old, new FROM {LOG} ORDER BY seq DESC").fetchall():
        if tbl == "charinventory":
            row = json.loads(new)
            cur = db.execute("SELECT * FROM charinventory WHERE itemid = ?", (int(rid),))
            r = cur.fetchone()
            now = None if r is None else dict(zip([d[0] for d in cur.description], r))
            if now is None or now["serverid"] != row["serverid"] or now["patternid"] != row["patternid"] or now["equiploc"] != -1:
                print(f"  left: charinventory {rid} (moved, worn or gone since)")
                left += 1
                continue
            db.execute("DELETE FROM charinventory WHERE itemid = ?", (int(rid),))
            audit(db, "delete", "charinventory", {"itemid": int(rid)}, now, None)
        else:
            old, new = json.loads(old), json.loads(new)
            if tbl == "queststeps":
                q, s = (int(x) for x in rid.split(","))
                ok = db.execute(f"UPDATE queststeps SET {col} = ? WHERE questid = ? AND step = ? AND {col} = ?",
                                (old, q, s, new)).rowcount == 1
                key = {"questid": q, "step": s}
            else:
                ok = db.execute(f"UPDATE quests SET {col} = ? WHERE questid = ? AND {col} = ?", (old, int(rid), new)).rowcount == 1
                key = {"questid": int(rid)}
            if not ok:
                print(f"  left: {tbl} {rid} {col} changed since")
                left += 1
                continue
            audit(db, "update", tbl, key, {col: new}, {col: old})
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if left == 0:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        db.execute("BEGIN IMMEDIATE")
        if a.undo:
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"put back {back}, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        if table(db, LOG):
            db.execute("ROLLBACK")
            print(f"applied already ({LOG} is there): --undo first")
            return 0
        changes, lines, refusals = plan(db)
        for line in lines:
            print(line)
        if refusals:
            db.execute("ROLLBACK")
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not changes:
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"report only: {len(changes)} rows would be written (--apply writes them)")
            return 0
        apply(db, changes)
        db.execute("COMMIT")
        print(f"applied: {len(changes)} rows, log {LOG}")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
