#!/usr/bin/env python3
"""The first quest everyone has, for every race and class, flagged for the marks over the NPCs' heads (the GM Jibrish's
petition 1813, 2026-11-12: "Add the ability for us to add yellow ! and ? over NPCs in game ... Once done place these
for the first quest everyone has for every race and class"; handlers/questheadmarks.go, table questmarks).

A new character of each start (table defaultcharacter: a row a race, class and, for a human, east or west) is offered
first, at its start, the quest it finds there: of the enabled quests of level 1 that require no other, asked of a class
or a race (a city's class quest, a homeland's first; never the tradeskills, Hunt and Gather or the like, which any
character may take anywhere), the one this race and class may take (its classes; its races as the server asks them,
questRaceTakes: a race of the game's by its wire race, a server's own race by its own) whose giver stands nearest the
start, in its world. Every start finds one: 72 city class quests for the game's races and 22 homeland quests, one for
every class of each of the server's own races (94 in all on Heirloom of 2026-11-12).

Each is a row of questmarks (questid, a note naming the starts it is first for). A row there already is left as it is
(a GM's). Refused, nothing written, when a start finds no quest. Report only by default; --apply in one transaction,
a gm_audit line a row, log questmarks_1112_log; a second --apply does nothing; --undo exact (a row changed since is
left and said; the table is dropped again only if this tool made it and it is empty). questmarks is read at the
server's start and at the panel's edit of a quest (or of Quest marks): after --apply on a running server, press the
panel's Reload on the quests, or restart.

    tools/questmarks_20261112.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import math
import sqlite3
import sys

LOG = "questmarks_1112_log"
DETAIL = "the first quest of each race and class marked over its NPCs' heads (petition 1813): tools/questmarks_20261112.py"
WHO, VIA = "dab", "tool"
SCHEMA = """CREATE TABLE IF NOT EXISTS questmarks (
	questid INTEGER PRIMARY KEY,
	note    TEXT NOT NULL DEFAULT ''
)"""
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    what    TEXT NOT NULL,      -- 'row' or 'table'
    questid INTEGER NOT NULL DEFAULT 0,
    after   TEXT NOT NULL DEFAULT '',
    audit   INTEGER NOT NULL DEFAULT 0
)"""
RACES = {0: "Human", 1: "Elf", 2: "Dark Elf", 3: "Gnome", 4: "Dwarf", 5: "Troll", 6: "Barbarian", 7: "Halfling",
         8: "Erudite", 9: "Ogre", 10: "Froglok"}
CLASSES = ["Warrior", "Ranger", "Paladin", "Shadowknight", "Monk", "Bard", "Rogue", "Druid", "Shaman", "Cleric",
           "Magician", "Necromancer", "Enchanter", "Wizard", "Alchemist"]
FIRST_CUSTOM = 11  # entity.FirstCustomRace


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def ints(s):
    return [int(x) for x in (s or "").replace(" ", "").split(",") if x != ""]


def fold(s):
    return (s or "").strip().lower()


def race_names(db):
    out = dict(RACES)
    if table(db, "racedefs"):
        for rid, name in db.execute("SELECT id, name FROM racedefs"):
            out[rid] = name
    return out


def wire_races(db):
    if not table(db, "racedefs"):
        return {}
    return {rid: w for rid, w in db.execute("SELECT id, wire_race FROM racedefs")}


def takes(races, race, wire):
    """questRaceTakes: a quest's races (none: any) taking a character of race."""
    if not races:
        return True
    w = wire.get(race, race)
    return any(r == race if r >= FIRST_CUSTOM else r == w for r in races)


def city(source, world, race):
    parts = (source or "").split()[1:2]
    path = parts[0] if parts else ""
    bits = path.split("/")
    if len(bits) >= 2 and bits[0] in ("1-20", "homelands"):
        return bits[1]
    return f"world {world}"


def picks(db):
    """(starts, refusals): each start's (race, class, humtype, quest id, quest name, giver, distance, city)."""
    wire = wire_races(db)
    quests = [(q, n, g, ints(c), ints(r), src) for q, n, g, c, r, src in db.execute(
        "SELECT questid, name, giver, classes, races, source FROM quests WHERE enabled = 1 AND minlevel <= 1 "
        "AND trim(requires) = '' AND trim(giver) != '' AND (trim(classes) != '' OR trim(races) != '') ORDER BY questid")]
    spots = {}
    for name, w, x, z in db.execute("SELECT npc_name, world, x_coord, z_coord FROM npcs"):
        spots.setdefault(fold(name), []).append((w, x, z))
    out, refusals = [], []
    for race, cls, hum, w, x, z in db.execute("SELECT race, tclass, humType, world, x, z FROM defaultcharacter "
                                               "ORDER BY race, humType, tclass"):
        best = None
        for q, n, g, cl, rc, src in quests:
            if cl and cls not in cl or not takes(rc, race, wire):
                continue
            for sw, sx, sz in spots.get(fold(g), []):
                if sw != w:
                    continue
                d = math.hypot(sx - x, sz - z)
                if best is None or d < best[0]:
                    best = (d, q, n, g, city(src, w, race))
        if best is None:
            refusals.append(f"race {race} class {cls} humtype {hum}: no first quest at its start (world {w}, x {x:.0f}, z {z:.0f})")
            continue
        out.append((race, cls, hum, best[1], best[2], best[3], best[0], best[4]))
    return out, refusals


def plan(db):
    """(rows, lines, refusals): the questmarks rows to write, by quest id, with their notes."""
    for t in ("quests", "defaultcharacter", "npcs"):
        if not table(db, t):
            return {}, [], [f"no {t} table here"]
    starts, refusals = picks(db)
    names = race_names(db)
    by_quest = {}
    for race, cls, hum, q, n, g, d, c in starts:
        by_quest.setdefault(q, {"name": n, "giver": g, "city": c, "starts": []})["starts"].append((race, cls, hum, d))
    have = {}
    if table(db, "questmarks"):
        have = {q: note for q, note in db.execute("SELECT questid, note FROM questmarks")}
    rows, lines = {}, []
    for q in sorted(by_quest):
        b = by_quest[q]
        who = sorted({(names.get(r, f"race {r}") + (" (east)" if r == 0 and h == 1 else " (west)" if r == 0 and h == 2 else ""))
                      for r, _, h, _ in b["starts"]})
        cls = sorted({c for _, c, _, _ in b["starts"]})
        classes = "every class" if len(cls) == 15 else ", ".join(CLASSES[c] if c < len(CLASSES) else str(c) for c in cls)
        note = f"petition 1813: the first quest of {', '.join(who)}: {classes} ({b['city']})"
        far = max(d for *_, d in b["starts"])
        state = "kept (a row there already)" if q in have else "flagged"
        lines.append(f"  {q} {b['name']!r} at {b['giver']} ({b['city']}, giver within {far:.0f} of {len(b['starts'])} start(s)): "
                     f"{', '.join(who)} / {classes}: {state}")
        if q not in have:
            rows[q] = note
    return rows, lines, refusals


def go_json(m):
    return json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, qid, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, 'questmarks', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, go_json({"questid": qid}),
                      None if before is None else go_json(before), None if after is None else go_json(after), detail))
    return cur.lastrowid


def row_of(db, qid):
    r = db.execute("SELECT questid, note FROM questmarks WHERE questid = ?", (qid,)).fetchone()
    return None if r is None else {"questid": r[0], "note": r[1]}


def apply(db, rows):
    db.execute(LOG_SCHEMA)
    if not table(db, "questmarks"):
        db.execute(SCHEMA)
        db.execute(f"INSERT INTO {LOG} (what) VALUES ('table')")
    for qid in sorted(rows):
        db.execute("INSERT INTO questmarks (questid, note) VALUES (?, ?)", (qid, rows[qid]))
        row = row_of(db, qid)
        a = audit(db, "insert", qid, None, row, DETAIL)
        db.execute(f"INSERT INTO {LOG} (what, questid, after, audit) VALUES ('row', ?, ?, ?)", (qid, json.dumps(row, sort_keys=True), a))


def undo(db):
    """(back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, what, qid, after in db.execute(f"SELECT seq, what, questid, after FROM {LOG} ORDER BY seq DESC").fetchall():
        if what == "row":
            now = row_of(db, qid)
            if now is None or json.dumps(now, sort_keys=True) != after:
                left += 1
                print(f"  left: questmarks {qid}, changed since it was written: {now}")
                continue
            db.execute("DELETE FROM questmarks WHERE questid = ?", (qid,))
            audit(db, "delete", qid, now, None, "undo: " + DETAIL)
        elif what == "table":
            if db.execute("SELECT COUNT(*) FROM questmarks").fetchone()[0] != 0:
                left += 1
                print("  left: table questmarks, which holds rows this tool did not write")
                continue
            db.execute("DROP TABLE questmarks")
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
        if a.undo:
            db.execute("BEGIN IMMEDIATE")
            try:
                back, left = undo(db)
                db.execute("COMMIT")
            except BaseException:
                db.execute("ROLLBACK")
                raise
            print(f"put back {back}, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        if table(db, LOG):
            print(f"applied already ({LOG} is there): nothing done (--undo first to apply again)")
            return 0
        rows, lines, refusals = plan(db)
        for line in lines:
            print(line)
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not rows:
            print("nothing to flag: every first quest is in questmarks already")
            return 0
        if not a.apply:
            print(f"report only: {len(rows)} questmarks rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            apply(db, rows)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {len(rows)} rows, in gm_audit and {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
