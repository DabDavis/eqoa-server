#!/usr/bin/env python3
"""Undo one character's vampirism (Live petitions 5184/5187, serverid 54).

A vampire is made by the vampire line (quest import keys Weres/vampire/*: "Razeal Has Won" makes
one at its desmodus bats, through its step's "vampire" event, and its Hinder step teaches Feed and
Sanctuary; Creed's, Weaver's and Simmons's Lessons teach the rest). Deleting the charvampire row
alone stops the sun and the in-memory flag at the next login (handlers/character.go reads it), but
leaves the line's abilities in the book and its standing hit. For one character this tool removes,
and records so it can put back:

  - charvampire: the row (the sun's burn);
  - spells (the spell book, the bar slot is the row's onHotBar/whereonBar): every spell a vampire
    quest's step teaches (queststeps.spells), none other;
  - charbuffs: an effect of one of those spells still kept on him (Vampire Form, Bat Form, ...);
  - charfactions: the standing a vampire quest's steps he did moved, moved back (Hinder's
    Werehunters of Tunaria -1000: the were lines' third quests ask -500..499 of it, so the gator
    line's Julian would never be offered);
  - charquests: the vampire line's rows (so Simmons's Lessons cannot hand vampire abilities to one
    who is no vampire, and the line can be done again); --keep-quests leaves them.

Nothing else is touched (XP, items, other quests). Each change is a row of vampire_reset_20261116_log
and a gm_audit line, in one transaction. --undo puts every logged row back as it was.

The character must be OFFLINE when it is applied (the server keeps the book, the standings and the
quests in memory while he plays and writes them back at logout); no restart is needed.

  python3 vampire_reset_20261116.py --db eqoa.db --serverid 54            # dry run: says what it would do
  python3 vampire_reset_20261116.py --db eqoa.db --serverid 54 --apply
  python3 vampire_reset_20261116.py --db eqoa.db --serverid 54 --undo
"""
import argparse
import datetime
import json
import sqlite3
import sys

WHO = "dab's sessions"
VIA = "cli"
DETAIL = "vampire_reset_20261116.py: undo vampirism (petition 5187)"
LOG = "vampire_reset_20261116_log"
FACTION_MIN, FACTION_MAX = -2000, 2000  # entity.FactionMin/Max
VAMPIRE_KEY = "Weres/vampire/"

LOG_SCHEMA = f"""
CREATE TABLE IF NOT EXISTS {LOG} (
    seq      INTEGER PRIMARY KEY AUTOINCREMENT,
    serverid INTEGER NOT NULL,
    tbl      TEXT NOT NULL,
    rowkey   TEXT NOT NULL,
    before   TEXT,
    after    TEXT,
    at       TEXT NOT NULL,
    undone   TEXT NOT NULL DEFAULT ''
);
"""


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def js(m):
    return None if m is None else json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def import_key(source):
    # database.QuestImportKey: "EQOA-Data <key> (tools/import_quests.py)"
    s = (source or "").strip()
    if not s.startswith("EQOA-Data ") or not s.endswith("(tools/import_quests.py)"):
        return ""
    return s[len("EQOA-Data "):].split(" ", 1)[0]


def int_list(s):
    out = []
    for p in (s or "").split(","):
        p = p.strip()
        if p.lstrip("-").isdigit():
            out.append(int(p))
    return out


def faction_deltas(s):
    # database.ParseFactionDeltas
    out = []
    for f in (s or "").split(","):
        i = f.rfind(":")
        if i < 0:
            continue
        name = f[:i].strip()
        try:
            d = int(f[i + 1:].strip().lstrip("+"))
        except ValueError:
            continue
        if name and d:
            out.append((name, d))
    return out


def vampire_line(db):
    """The vampire quests (id -> name) and the spells their steps teach."""
    quests = {qid: name for qid, name, src in db.execute("SELECT questid, name, source FROM quests")
              if import_key(src).startswith(VAMPIRE_KEY)}
    spells = set()
    for qid, sp in db.execute("SELECT questid, spells FROM queststeps"):
        if qid in quests:
            spells.update(int_list(sp))
    return quests, spells


def rows(db, sql, args):
    cur = db.execute(sql, args)
    cols = [c[0] for c in cur.description]
    return [dict(zip(cols, r)) for r in cur.fetchall()]


def audit(db, action, tbl, key, before, after):
    row = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
               (now(), row[0] if row else 0, WHO, VIA, action, tbl, js(key), js(before), js(after), DETAIL))


def plan(db, sid, keep_quests):
    """The changes, each (tbl, key, before, after): after None deletes the row."""
    quests, spells = vampire_line(db)
    changes = []
    if table(db, "charvampire"):
        for r in rows(db, "SELECT * FROM charvampire WHERE serverid = ?", (sid,)):
            changes.append(("charvampire", {"serverid": sid}, r, None))
    if spells:
        marks = ",".join("?" * len(spells))
        for r in rows(db, f"SELECT * FROM spells WHERE serverid = ? AND spellid IN ({marks}) ORDER BY addedorder", (sid, *sorted(spells))):
            changes.append(("spells", {"id": r["id"]}, r, None))
        for r in rows(db, f"SELECT * FROM charbuffs WHERE serverid = ? AND spellid IN ({marks}) ORDER BY seq", (sid, *sorted(spells))):
            changes.append(("charbuffs", {"serverid": sid, "seq": r["seq"]}, r, None))
    # the standings the steps he did moved, moved back
    cqs = {r["questid"]: r for r in rows(db, "SELECT * FROM charquests WHERE serverid = ?", (sid,)) if r["questid"] in quests}
    moved = {}
    for qid, cq in cqs.items():
        for step, fac in db.execute("SELECT step, factions FROM queststeps WHERE questid = ? ORDER BY step", (qid,)):
            if cq["done"] > 0 or step < cq["step"]:
                for name, d in faction_deltas(fac):
                    moved[name] = moved.get(name, 0) + d
    for name, d in sorted(moved.items()):
        r = db.execute("SELECT standing FROM charfactions WHERE serverid = ? AND faction = ?", (sid, name)).fetchone()
        if r is None:
            continue  # at its start already: nothing kept to move back
        before = {"serverid": sid, "faction": name, "standing": r[0]}
        after = dict(before, standing=max(FACTION_MIN, min(FACTION_MAX, r[0] - d)))
        if after["standing"] != r[0]:
            changes.append(("charfactions", {"serverid": sid, "faction": name}, before, after))
    if not keep_quests:
        for qid in sorted(cqs):
            changes.append(("charquests", {"serverid": sid, "questid": qid}, cqs[qid], None))
    return quests, spells, changes


def write_row(db, tbl, key, row):
    """Make the keyed row be row (None: no row)."""
    where = " AND ".join(f"{k} = ?" for k in key)
    db.execute(f"DELETE FROM {tbl} WHERE {where}", tuple(key.values()))
    if row is not None:
        cols = list(row)
        db.execute(f"INSERT INTO {tbl} ({','.join(cols)}) VALUES ({','.join('?' * len(cols))})", tuple(row[c] for c in cols))


def show(db, sid, quests, spells):
    name = db.execute("SELECT charName, level, tclass FROM characters WHERE serverid = ?", (sid,)).fetchone()
    print(f"character {sid}: {name}")
    print("  charvampire:", rows(db, "SELECT * FROM charvampire WHERE serverid = ?", (sid,)) if table(db, "charvampire") else "no table")
    marks = ",".join("?" * len(spells)) or "NULL"
    names = dict(db.execute(f"SELECT spellid, spellname FROM spellpattern WHERE spellid IN ({marks})", tuple(sorted(spells))))
    for r in rows(db, f"SELECT * FROM spells WHERE serverid = ? AND spellid IN ({marks}) ORDER BY addedorder", (sid, *sorted(spells))):
        bar = f"bar slot {r['whereonBar']}" if r["onHotBar"] else "not on the bar"
        print(f"  spell {r['spellid']} {names.get(r['spellid'], '?')}: book row {r['id']}, {bar}")
    for r in rows(db, f"SELECT seq, spellid, name FROM charbuffs WHERE serverid = ? AND spellid IN ({marks})", (sid, *sorted(spells))):
        print(f"  effect kept: {r}")
    qm = ",".join("?" * len(quests)) or "NULL"
    for r in rows(db, f"SELECT questid, step, done, updated FROM charquests WHERE serverid = ? AND questid IN ({qm}) ORDER BY questid", (sid, *sorted(quests))):
        print(f"  quest {r['questid']} {quests[r['questid']]}: step {r['step']} done {r['done']} ({r['updated']})")
    for r in db.execute("SELECT faction, standing FROM charfactions WHERE serverid = ? AND faction = 'Werehunters of Tunaria'", (sid,)):
        print(f"  standing {r[0]}: {r[1]}")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    ap.add_argument("--serverid", type=int, required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    ap.add_argument("--keep-quests", action="store_true", help="leave the vampire line's charquests rows")
    a = ap.parse_args()

    db = sqlite3.connect(a.db, timeout=10, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 10000")
    sid = a.serverid
    if db.execute("SELECT 1 FROM characters WHERE serverid = ?", (sid,)).fetchone() is None:
        sys.exit(f"no character {sid}")
    quests, spells = vampire_line(db)
    if not quests or not spells:
        sys.exit("no vampire line in the quests (import keys Weres/vampire/*)")
    print(f"vampire line: {len(quests)} quests {sorted(quests)}, spells {sorted(spells)}")

    if a.undo:
        if not table(db, LOG):
            sys.exit(f"nothing to undo: no {LOG}")
        logged = rows(db, f"SELECT * FROM {LOG} WHERE serverid = ? AND undone = '' ORDER BY seq DESC", (sid,))
        if not logged:
            sys.exit("nothing to undo")
        db.execute("BEGIN IMMEDIATE")
        try:
            for r in logged:
                key, before, after = json.loads(r["rowkey"]), json.loads(r["before"]) if r["before"] else None, json.loads(r["after"]) if r["after"] else None
                write_row(db, r["tbl"], key, before)
                audit(db, "revert", r["tbl"], key, after, before)
                db.execute(f"UPDATE {LOG} SET undone = ? WHERE seq = ?", (now(), r["seq"]))
            db.execute("COMMIT")
        except Exception:
            db.execute("ROLLBACK")
            raise
        print(f"undone: {len(logged)} rows put back")
        show(db, sid, quests, spells)
        return

    print("before:")
    show(db, sid, quests, spells)
    _, _, changes = plan(db, sid, a.keep_quests)
    for tbl, key, before, after in changes:
        what = "delete" if after is None else f"set {after}"
        print(f"  {tbl} {js(key)}: {what}")
    if not changes:
        print("nothing to change")
        return
    if not a.apply:
        print(f"dry run: {len(changes)} changes; --apply to make them")
        return
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_SCHEMA)
        for tbl, key, before, after in changes:
            write_row(db, tbl, key, after)
            audit(db, "delete" if after is None else "update", tbl, key, before, after)
            db.execute(f"INSERT INTO {LOG} (serverid, tbl, rowkey, before, after, at) VALUES (?, ?, ?, ?, ?, ?)",
                       (sid, tbl, js(key), js(before), js(after), now()))
        db.execute("COMMIT")
    except Exception:
        db.execute("ROLLBACK")
        raise
    print(f"applied: {len(changes)} changes, in {LOG} and gm_audit")
    print("after:")
    show(db, sid, quests, spells)


if __name__ == "__main__":
    main()
