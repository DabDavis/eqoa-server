#!/usr/bin/env python3
"""Board #742's five data rows: the words and factions of the coachmen and shopkeepers filmed refusing a player their
faction holds in the red bands (jibrish's decision; the refusal itself is code: handlers/factionrefusal.go,
entity.World.Refuses, at or below -350).

  npcdialog, mood hostile, of their own names (said in place of a coachman's menu or a merchant's window):
    Coachman Lothi       "I will not help you %p. Be glad I do not slay you where you stand for crimes against Neriak!"
    Coachman Zuggug      "Gah! You be evil! Go way! Me eat you if you don't!"
    Jextalan shopkeeper  "You have nothing I want and I have nothing you need. Good day."
  npcfaction, anywhere (world -1, no place), note "filmed refusal (video audit)":
    baga shopkeeper      Citizens of Baga
    jextalan shopkeeper  People of Jextalan

All five carry the NPC bit (0x80), so a faction row never makes them attack. Rows are matched by names: a dialog row
already there (same name, mood and line) or a faction row of the same name and place is not written again; a faction
the factions table lacks is reported and skipped. Source 'video-audit'.

Report only by default; --apply writes in one transaction, a gm_audit line a row, each row kept in jib742_20261121_log;
--undo deletes each row still as this tool wrote it (one changed since is left, and said). A second --apply finds
nothing to do. Run it on both shards.

WHEN IT TAKES EFFECT: npcdialog within 30 s (its read is kept that long); npcfaction at a GM panel faction edit or
reload, else at the server's next start.

    tools/jib742_20261121.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "jib742_20261121_log"
SRC = "video-audit"
DETAIL = "board #742: coachmen and merchants refuse the red bands, in the filmed words: tools/jib742_20261121.py"
WHO, VIA = "dab", "tool"
NOTE = "filmed refusal (video audit)"

DIALOG = [
    ("Coachman Lothi", "I will not help you %p. Be glad I do not slay you where you stand for crimes against Neriak!"),
    ("Coachman Zuggug", "Gah! You be evil! Go way! Me eat you if you don't!"),
    ("Jextalan shopkeeper", "You have nothing I want and I have nothing you need. Good day."),
]
FACTION = [
    ("baga shopkeeper", "Citizens of Baga"),
    ("jextalan shopkeeper", "People of Jextalan"),
]


def js(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def has(db, tbl):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (tbl,)).fetchone() is not None


def plan(db):
    """The rows to write, as (table, values), and what was found already there or wrong."""
    out, notes = [], []
    for name, line in DIALOG:
        n = db.execute("SELECT count(*) FROM npcs WHERE lower(npc_name) = lower(?)", (name,)).fetchone()[0] if has(db, "npcs") else 0
        if db.execute("SELECT 1 FROM npcdialog WHERE lower(name) = lower(?) AND mood = 'hostile' AND line = ?",
                      (name, line)).fetchone():
            notes.append(f"npcdialog {name!r} hostile: there already")
            continue
        out.append(("npcdialog", {"town": "", "role": "", "name": name, "mood": "hostile", "race": -1, "line": line,
                                  "weight": 1, "source": SRC, "sex": "", "speaker_race": -1, "away": 0, "class": -1,
                                  "not_race": -1}, n))
    for name, faction in FACTION:
        n = db.execute("SELECT count(*) FROM npcs WHERE lower(npc_name) = lower(?)", (name,)).fetchone()[0] if has(db, "npcs") else 0
        if has(db, "factions") and not db.execute("SELECT 1 FROM factions WHERE name = ?", (faction,)).fetchone():
            notes.append(f"npcfaction {name!r}: {faction!r} is no faction here, skipped")
            continue
        have = db.execute("SELECT faction FROM npcfaction WHERE lower(name) = lower(?) AND world = -1 AND x = 0 AND z = 0 "
                          "AND radius = 0", (name,)).fetchone()
        if have:
            notes.append(f"npcfaction {name!r} anywhere: there already ({have[0]!r})")
            continue
        out.append(("npcfaction", {"name": name, "world": -1, "x": 0, "z": 0, "radius": 0, "faction": faction,
                                   "note": NOTE, "source": SRC}, n))
    return out, notes


def apply(db, rows):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (seq INTEGER PRIMARY KEY, tbl TEXT NOT NULL, id INTEGER NOT NULL, "
               "row TEXT NOT NULL, at TEXT NOT NULL)")
    for tbl, values, _ in rows:
        cols = [r[1] for r in db.execute(f'PRAGMA table_info("{tbl}")')]
        values = {c: v for c, v in values.items() if c in cols}
        cur = db.execute(f'INSERT INTO "{tbl}" ({", ".join(values)}) VALUES ({", ".join("?" * len(values))})',
                         list(values.values()))
        c2 = db.execute(f'SELECT * FROM "{tbl}" WHERE id = ?', (cur.lastrowid,))
        row = dict(zip([x[0] for x in c2.description], c2.fetchone()))
        db.execute(f"INSERT INTO {LOG} (tbl, id, row, at) VALUES (?, ?, ?, ?)", (tbl, cur.lastrowid, js(row), at))
        db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                   "VALUES (?, ?, ?, ?, '', 'insert', ?, ?, ?, ?, ?)",
                   (at, acct[0] if acct else 0, WHO, VIA, tbl, js({"id": cur.lastrowid}), js(None), js(row), DETAIL))
    return len(rows)


def undo(db):
    if not has(db, LOG):
        return 0, 0
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    back, left = 0, 0
    for seq, tbl, rid, was in db.execute(f"SELECT seq, tbl, id, row FROM {LOG} ORDER BY seq DESC").fetchall():
        c2 = db.execute(f'SELECT * FROM "{tbl}" WHERE id = ?', (rid,))
        r = c2.fetchone()
        now = dict(zip([x[0] for x in c2.description], r)) if r else None
        if js(now) != was:
            print(f"  {tbl} id {rid}: changed since ({'gone' if now is None else 'edited'}), left as it is")
            left += 1
            continue
        db.execute(f'DELETE FROM "{tbl}" WHERE id = ?', (rid,))
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                   "VALUES (?, ?, ?, ?, '', 'delete', ?, ?, ?, ?, ?)",
                   (at, acct[0] if acct else 0, WHO, VIA, tbl, js({"id": rid}), js(now), js(None), "undone: " + DETAIL))
        back += 1
    if not db.execute(f"SELECT 1 FROM {LOG} LIMIT 1").fetchone():
        db.execute(f"DROP TABLE {LOG}")
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
            print(f"undo: {back} rows deleted, {left} left as changed since")
            return 0
        rows, notes = plan(db)
        for n in notes:
            print("  " + n)
        for tbl, v, n in rows:
            what = v["line"] if tbl == "npcdialog" else v["faction"]
            print(f"  write {tbl} {v['name']!r}: {what!r} ({n} npcs rows of the name)")
        print(f"{len(rows)} rows to write")
        if a.apply:
            done = apply(db, rows)
            db.commit()
            print(f"applied: {done} rows written")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
