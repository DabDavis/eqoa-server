#!/usr/bin/env python3
"""The tank exemplars' repeatable faction turn-ins stop at the top (petition 2471, Heirloom, a player: "SK exemplar- I've
maxed out my faction iwth Minions of Bertoxxulous (2000) but Urglauh the Foul continues to give me a quest to obtain
screaming souls rather than continuing the quest.").

Each exemplar path is earned at a repeatable hand-in that raises one faction toward 2,000, and the path's reward is a
quest of another giver offered at 2,000 (factionreq "Faction:2000"). The hand-ins asked no standing, so at the top their
giver went on offering them (Urglauh the Foul's Screaming Souls, round and round, each soul for nothing), and nothing
said the reward waits with the priest. Now:

  - the hand-ins are offered, and their turn-in done, only below the top: factionreq "Faction:-2000..1999" (as the
    gypsies' A Set of Tails asks "1851..1999"): Screaming Souls (Urglauh the Foul, Minions of Bertoxxulous), The
    Blasphemous Box (Miasma Malignus, Disciples of Plague), Dhia's Blessed Chest (Dhia Shieldstone, Guards of Diren
    Hold), Ragh's Firepit and Ragh's Firepit: a Charm (Ragh Skulcrak, Soldiers of Rallos Zek);
  - the reward quests that only their hand-in feeds require it too, so the journal's next-giver pointer
    (handlers/questpointers.go) shows "Soul Cleave: find the Priest of Bertoxxulous ..." once the top is reached:
    Soul Cleave (+ Screaming Souls), Soultap (+ The Blasphemous Box), Serenity (+ Dhia's Blessed Chest). No creature,
    area or item raises those three factions (factionhits, factionhitareas, itemfactions: none), so no one reaches the
    top any other way. Dropkick is left as it is: the Zaoran Plateau's weapons quests raise the Soldiers of Rallos Zek
    too.

Rows are found by name and giver, never by id. For both shards (the quests are on both).

WHEN IT TAKES EFFECT: the quest engine reads quests only at its start or a GM panel save of a quest row: run it as a keep
restart's before.sh (or save one of these rows in the panel after it).

Report only by default; --apply writes it, each change with its gm_audit line and its line in exemplar_tops_1120_log;
--undo puts every column back as it was (refused for a row changed since) and drops the log.

    python3 -E -s tools/exemplar_faction_tops_20261120.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

LOG = "exemplar_tops_1120_log"
DETAIL = ("petition 2471 (Heirloom): the tank exemplars' repeatable faction turn-ins stop at the top, their rewards "
          "require them; tools/exemplar_faction_tops_20261120.py")

# (name, giver, faction): a repeatable hand-in, offered below the faction's top
TOPS = [
    ("Screaming Souls", "Urglauh the Foul", "Minions of Bertoxxulous"),
    ("The Blasphemous Box", "Miasma Malignus", "Disciples of Plague"),
    ("Dhia's Blessed Chest", "Dhia Shieldstone", "Guards of Diren Hold"),
    ("Ragh's Firepit", "Ragh Skulcrak", "Soldiers of Rallos Zek"),
    ("Ragh's Firepit: a Charm", "Ragh Skulcrak", "Soldiers of Rallos Zek"),
]
# (reward name, its giver, the hand-in's name and giver it then requires)
FOLLOWS = [
    ("Soul Cleave", "Priest of Bertoxxulous", "Screaming Souls", "Urglauh the Foul"),
    ("Soultap", "Priest of Bertoxxulous", "The Blasphemous Box", "Miasma Malignus"),
    ("Serenity", "Priest of Rallos Zek", "Dhia's Blessed Chest", "Dhia Shieldstone"),
]


def audit(db, qid, col, before, after, detail):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    return db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                      "VALUES (?, 0, 'dab''s sessions', 'tool', '', 'update', 'quests', ?, ?, ?, ?)",
                      (at, json.dumps({"questid": qid}), json.dumps({col: before}), json.dumps({col: after}), detail)).lastrowid


def quest(db, name, giver):
    rows = db.execute("SELECT questid, repeatable, requires, factionreq FROM quests WHERE name = ? AND giver = ?",
                      (name, giver)).fetchall()
    if len(rows) != 1:
        sys.exit(f"refused: {len(rows)} quests named {name!r} given by {giver!r} (want one): nothing written")
    return rows[0]


def plan(db):
    out = []  # (questid, name, column, before, after)
    for name, giver, faction in TOPS:
        qid, rep, _, fr = quest(db, name, giver)
        if not rep:
            sys.exit(f"refused: {name!r} is not repeatable: nothing written")
        want = f"{faction}:-2000..1999"
        if fr.strip() == want:
            continue
        if fr.strip():
            sys.exit(f"refused: {name!r} asks a standing already ({fr!r}): nothing written")
        out.append((qid, name, "factionreq", fr, want))
    for name, giver, feeder, fgiver in FOLLOWS:
        qid, _, req, fr = quest(db, name, giver)
        fid = quest(db, feeder, fgiver)[0]
        ids = [p.strip() for p in req.replace("|", ",").split(",") if p.strip()]
        if str(fid) in ids:
            continue
        if not fr.strip().endswith(":2000"):
            sys.exit(f"refused: {name!r} does not ask a standing of 2000 ({fr!r}): nothing written")
        after = f"{req.strip()}, {fid}" if req.strip() else str(fid)
        out.append((qid, name, "requires", req, after))
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"no database at {a.db}")
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    has_log = db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (LOG,)).fetchone() is not None
    if a.undo:
        if not has_log:
            sys.exit(f"nothing to undo: no {LOG}")
        rows = db.execute(f"SELECT questid, col, before, after FROM {LOG} ORDER BY seq DESC").fetchall()
        for qid, col, before, after in rows:
            now = db.execute(f"SELECT {col} FROM quests WHERE questid = ?", (qid,)).fetchone()
            if now is None or now[0] != after:
                sys.exit(f"undo refused: quest {qid}'s {col} is {now and now[0]!r}, not {after!r} as written: nothing done")
        db.execute("BEGIN IMMEDIATE")
        for qid, col, before, after in rows:
            db.execute(f"UPDATE quests SET {col} = ? WHERE questid = ?", (before, qid))
            audit(db, qid, col, after, before, "undo: " + DETAIL)
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
        print(f"undone: {len(rows)} changes put back")
        return
    if has_log:
        sys.exit(f"{LOG} is there already: applied before (--undo first, or look)")
    steps = plan(db)
    for qid, name, col, before, after in steps:
        print(f"quest {qid} {name!r}: {col} {before!r} -> {after!r}")
    if not steps:
        print("nothing to do: every row is as it should be")
        return
    if not a.apply:
        print(f"report only: {len(steps)} changes; --apply writes them")
        return
    db.execute("BEGIN IMMEDIATE")
    db.execute(f"CREATE TABLE {LOG} (seq INTEGER PRIMARY KEY AUTOINCREMENT, questid INTEGER NOT NULL, col TEXT NOT NULL, "
               "before TEXT NOT NULL, after TEXT NOT NULL, audit INTEGER)")
    for qid, name, col, before, after in steps:
        db.execute(f"UPDATE quests SET {col} = ? WHERE questid = ?", (after, qid))
        db.execute(f"INSERT INTO {LOG} (questid, col, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                   (qid, col, before, after, audit(db, qid, col, before, after, DETAIL)))
    db.execute("COMMIT")
    print(f"applied: {len(steps)} changes")


if __name__ == "__main__":
    main()
