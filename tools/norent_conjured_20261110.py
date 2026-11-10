#!/usr/bin/env python3
"""The twenty records a spell conjures vanish five minutes after their holder logs out (the owner, 2026-11-10, deciding
jib's PR #1: the GM Jibrish's ruling of 2026-10-29 has a spell's summoned items "No rent", which "means it vanishes if
the player logs out for more than 5 minutes"); the Unit of Blood stays (the owner: it STAYS).

A norentvanish row each (database/norent.go: a login takes the copies of the records it names, worn, packed or banked,
unless it comes within five minutes of the character's last logout: handlers/norent.go, table charlogout): the
magician's conjured Phase Shield, Phase Mallet, Phase Ring, Phase Gladius, Phase Crown, Burning Blade and Shadow Knife,
the stones and shards (Lava Stone, Aura Stone, Ice Shard, Fiery Pebble, Frost Orb, Scoria Stone, Mana Stone) and the
six drinks (Unfiltered Water, Weak Ale, Citrus Mixture, Ginger Root Ale, Bitter Draft, Fine Red Wine), 705298-705317 in
the old data. Each is found by its name among the records of rent 0 (the shards' ids diverged after their split, and
seven of these names are also a record of rent 1, the game's own Lava Stone, Phase Ring, Phase Shield, Aura Stone,
Phase Crown, Scoria Stone and Mana Stone, which are left alone), at its old id when that is the one; a name with no such
record here is left out and said, two of them refuse the run. A record a norentvanish row names already is left as it
is, and so is the Unit of Blood (709302), whatever it has.

Report only by default; --apply in one transaction: each row, its gm_audit line and its line in the log
norent_conjured_1110_log; a second --apply finds nothing to add and writes nothing. --undo takes out exactly the rows
this wrote, each with its gm_audit line (a row changed since is left and said), and drops the log.

No restart: a login reads norentvanish as it goes, so each holder loses them at its next login five minutes or more
after its logout. The five minutes are the server's with charlogout (jib's PR #1 on main); on an older server they go
at every login, and a character that left before that server came has no logout kept, so its next login takes them.

    tools/norent_conjured_20261110.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "norent_conjured_1110_log"
WHO, VIA = "dab", "tool"
DETAIL = ("a spell's conjured items vanish 5 minutes after their holder logs out (the owner, 2026-11-10, deciding PR #1: "
          "the GM Jibrish's ruling): tools/norent_conjured_20261110.py")
NOTE = "conjured by a spell: gone 5 minutes after its holder logs out (the owner, 2026-11-10; the GM Jibrish's ruling, PR #1)"
SOURCE = "norent_conjured_20261110"

# the old data's ids and names
ITEMS = [
    (705298, "Phase Shield"), (705299, "Lava Stone"), (705300, "Aura Stone"), (705301, "Phase Mallet"),
    (705302, "Ice Shard"), (705303, "Phase Ring"), (705304, "Phase Gladius"), (705305, "Fiery Pebble"),
    (705306, "Phase Crown"), (705307, "Burning Blade"), (705308, "Frost Orb"), (705309, "Scoria Stone"),
    (705310, "Shadow Knife"), (705311, "Mana Stone"), (705312, "Unfiltered Water"), (705313, "Weak Ale"),
    (705314, "Citrus Mixture"), (705315, "Ginger Root Ale"), (705316, "Bitter Draft"), (705317, "Fine Red Wine"),
]
STAYS = (709302, "Unit of Blood")

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq       INTEGER PRIMARY KEY AUTOINCREMENT,
    patternid INTEGER NOT NULL,
    after     TEXT NOT NULL,      -- the norentvanish row as written (JSON)
    audit     INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def go_json(m):
    """A row as Go's json.Marshal writes a map (keys sorted, <>& escaped), as the panel's audit has it."""
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def vanish_row(db, patternid):
    cur = db.execute("SELECT * FROM norentvanish WHERE patternid = ?", (patternid,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def audit(db, action, key, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, 'norentvanish', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, go_json(key),
                      None if before is None else go_json(before), None if after is None else go_json(after),
                      ("undo: " if action == "delete" else "") + DETAIL))
    return cur.lastrowid


def plan(db):
    """(adds, lines, refusals): the records to give a norentvanish row, and what was found."""
    adds, lines, refusals = [], [], []
    for old_id, name in ITEMS:
        found = [r[0] for r in db.execute("SELECT patternid FROM itempattern WHERE lower(trim(itemname)) = lower(?) AND rent = 0 "
                                          "ORDER BY patternid", (name,))]
        if old_id in found:
            pid = old_id
        elif len(found) == 1:
            pid = found[0]
        elif not found:
            lines.append(f"{name}: no record of rent 0 here: left out")
            continue
        else:
            refusals.append(f"{name}: {len(found)} records of rent 0 ({', '.join(map(str, found))}), none at {old_id}")
            continue
        held = db.execute("SELECT count(*), count(DISTINCT serverid) FROM charinventory WHERE patternid = ?", (pid,)).fetchone()
        where = "" if pid == old_id else f" (at {pid} here, not {old_id})"
        if vanish_row(db, pid) is not None:
            lines.append(f"{name} {pid}{where}: vanishes already: left as it is")
            continue
        adds.append(pid)
        lines.append(f"{name} {pid}{where}: vanishes from now ({held[0]} copies held by {held[1]} characters)")
    sid, sname = STAYS
    r = db.execute("SELECT itemname, rent FROM itempattern WHERE patternid = ?", (sid,)).fetchone()
    if r is not None and vanish_row(db, sid) is not None:
        lines.append(f"{sname} {sid}: a norentvanish row names it already: left as it is (it should stay: take that row out by hand)")
    elif r is not None:
        lines.append(f"{sname} {sid}: stays (no row given)")
    return adds, lines, refusals


def apply(db, adds):
    db.execute(LOG_SCHEMA)
    for pid in adds:
        db.execute("INSERT INTO norentvanish (patternid, note, source) VALUES (?, ?, ?)", (pid, NOTE, SOURCE))
        row = vanish_row(db, pid)
        a = audit(db, "insert", {"patternid": pid}, None, row)
        db.execute(f"INSERT INTO {LOG} (patternid, after, audit) VALUES (?, ?, ?)", (pid, json.dumps(row, sort_keys=True), a))
    return len(adds)


def undo(db):
    """Each logged row still as written taken out, the last first, its gm_audit line; the log dropped when none is
    left. Returns (back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, pid, after in db.execute(f"SELECT seq, patternid, after FROM {LOG} ORDER BY seq DESC").fetchall():
        now = vanish_row(db, pid)
        if now is None or json.dumps(now, sort_keys=True) != after:
            left += 1
            print(f"  left: norentvanish {pid}, {'gone' if now is None else 'changed'} since it was written")
            continue
        db.execute("DELETE FROM norentvanish WHERE patternid = ?", (pid,))
        audit(db, "delete", {"patternid": pid}, now, None)
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
        for t in ("itempattern", "norentvanish", "charinventory", "gm_audit"):
            if not table(db, t):
                print(f"no table {t}: not a server database of this age")
                return 1
        if a.undo:
            db.execute("BEGIN IMMEDIATE")
            try:
                back, left = undo(db)
                db.execute("COMMIT")
            except BaseException:
                db.execute("ROLLBACK")
                raise
            print(f"took out {back} rows, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        adds, lines, refusals = plan(db)
        for line in lines:
            print(line)
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not adds:
            print("nothing to do: every one found vanishes already")
            return 0
        if not a.apply:
            print(f"report only: {len(adds)} norentvanish rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            n = apply(db, adds)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {n} norentvanish rows, each in gm_audit and {LOG}; no restart: each holder loses them at its next "
              f"login 5 minutes or more after its logout")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
