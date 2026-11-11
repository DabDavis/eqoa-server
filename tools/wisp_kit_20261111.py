#!/usr/bin/env python3
"""The Wisp's racial masteries and its Master Class (the owner, 2026-11-11, a proposal forwarded as theirs: "Racials —
Will-of-the-Wisp: 10 pot, 5 cms. Trickster Flame: unlocks a few glamours, allowing the wisp to take mortal form, 10 cms.
Master class title: Transcendent: 50 dex max, 25 int max, 25 wis max, 500 pow max; -25 sta, -25 str, agi max -250 ac.
... It would be cool if all the racial CMs gave base pot since wisps are casters. I like the idea of their MC being
really strong for casting but making them a glass cannon").

Five cmcatalog rows for race 31, the Wisp (races = 1 << 31, the bit the server tests for a race of its own,
entity/race.go raceInMask; every class):

  2141 racial  Will-of-the-Wisp   cost 5,  level 6: PoT 10
  2142 racial  Trickster Flame    cost 10, level 6: PoT 5, and the glamours of mortal form, as the enchanter's Glimmering
               Reality 1 gives them (Glamour: Human 496 @1, Elf 503 @4, Erudite 507 @8, Halfling 517 @12, Dwarf 524 @16,
               Gnome 531 @20), and Remove Illusion (1793 @1) to be the flame again. In a glamour a Wisp shows its whole
               body in its gear (the server's entity/wispform.go); in its own form nothing worn but its cape.
  2143 master  Transcendent 1 "Transcendent"  cost 200, level 55, 500 spent (every Master Class's rank 1): DEX cap +50,
               INT cap +25, WIS cap +25, max power +500; STR cap -25, STA cap -25, AGI cap -25, AC -250 (as written: the
               proposer thought the minuses "might be a bit much", the owner approved them)
  2144 master  Transcendent 2 "Unbound Will"  cost 75, level 60 (as every rank 2): max power +500, PoT 20 (Scholar's and
               Immortal's casting rank)
  2145 master  Transcendent 3 "Pale Fire"     cost 75, level 60: INT +25, WIS +25 (up to the caps rank 1 raised)

Ranks 2 and 3 are ours (the proposal names the Master Class and rank 1's figures; every other has three ranks). The
Wisp had no racial or Master Class row before (none with bit 31).

Refused, nothing written, unless every id is free (or is exactly this tool's row: applied already), every spell is in
spellpattern by its name, and no other row has bit 31 set. Report only by default; --apply in one transaction, a
gm_audit line a row, log wisp_kit_1111_log; --undo exact (a row changed since is left and said). cmcatalog is read at
the start (or the GM panel's reload of the masteries): a restart, or that reload.

    tools/wisp_kit_20261111.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "wisp_kit_1111_log"
DETAIL = "The Wisp's racials and Master Class (the owner, 2026-11-11): tools/wisp_kit_20261111.py"
WHO, VIA = "dab", "tool"
WISP = 31
RACES = 1 << WISP
SOURCE = "ours (the owner's word, 2026-11-11: the Wisp's racials and Master Class)"
GLAMOURS = [("Glamour: Human", 496, 1), ("Glamour: Elf", 503, 4), ("Glamour: Erudite", 507, 8),
            ("Glamour: Halfling", 517, 12), ("Glamour: Dwarf", 524, 16), ("Glamour: Gnome", 531, 20),
            ("Remove Illusion", 1793, 1)]


def rank(id, category, mastery, rank, name, parent, cost, level, spent, effects, description, adds=()):
    return {"id": id, "category": category, "classes": 32767, "races": RACES, "mastery": mastery, "rank": rank,
            "name": name, "parent": parent, "cost": cost, "level": level, "spent": spent, "minstats": "",
            "requires": "", "require_spells": "", "effects": effects,
            "adds": "; ".join(f"{n}@{lv}" for n, _, lv in adds), "add_spells": ",".join(str(s) for _, s, _ in adds),
            "removes": "", "remove_spells": "", "description": description, "enabled": 1, "source": SOURCE,
            "require_ranks": "", "require_quests": ""}


ROWS = [
    rank(2141, 1, "Will-of-the-Wisp", 1, "Will-of-the-Wisp", 0, 5, 6, 0, "PoT:10",
         "A wisp's light feeds on itself: its power returns the faster."),
    rank(2142, 1, "Trickster Flame", 1, "Trickster Flame", 0, 10, 6, 0, "PoT:5",
         "Wisps have long lured travelers with false shapes. The wisp learns the glamours of mortal form, a human's, "
         "an elf's, an erudite's, a halfling's, a dwarf's and a gnome's, its gear shown on it while it wears one, and "
         "Remove Illusion to be the flame again.", GLAMOURS),
    rank(2143, 4, "Transcendent", 1, "Transcendent", 0, 200, 55, 500,
         "DEXMax:50,INTMax:25,WISMax:25,Pow:500,STRMax:-25,STAMax:-25,AGIMax:-25,AC:-250",
         "A wisp that has let go of every mortal shape burns with pure power, and has nothing of a body to shield it."),
    rank(2144, 4, "Transcendent", 2, "Unbound Will", 2143, 75, 60, 0, "Pow:500,PoT:20",
         "Unbound by flesh, the transcendent wisp's will is a well without a bottom."),
    rank(2145, 4, "Transcendent", 3, "Pale Fire", 2143, 75, 60, 0, "INT:25,WIS:25",
         "The pale fire of a transcendent wisp sees through every mind it lights."),
]
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    id      INTEGER NOT NULL,   -- the cmcatalog row written
    after   TEXT NOT NULL,      -- the row as written (JSON)
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def row_of(db, id):
    cur = db.execute("SELECT * FROM cmcatalog WHERE id = ?", (id,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, id, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, 'cmcatalog', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, go_json({"id": id}),
                      None if before is None else go_json(before), None if after is None else go_json(after), detail))
    return cur.lastrowid


def same(now, want):
    return now is not None and all(now.get(k) == v for k, v in want.items())


def plan(db):
    """(lines, refusals, applied)."""
    lines, refusals = [], []
    if not table(db, "cmcatalog") or not table(db, "spellpattern"):
        return lines, ["no cmcatalog or spellpattern table here"], False
    rd = db.execute("SELECT name FROM racedefs WHERE id = ?", (WISP,)).fetchone() if table(db, "racedefs") else None
    if rd is None or rd[0] != "Wisp":
        refusals.append(f"racedefs {WISP} is {rd!r}, not the Wisp")
    cols = {r[1] for r in db.execute("PRAGMA table_info(cmcatalog)")}
    if missing := set(ROWS[0]) - cols:
        return lines, [f"cmcatalog lacks {sorted(missing)}"], False
    for name, sid, _ in GLAMOURS:
        sp = db.execute("SELECT spellname FROM spellpattern WHERE spellid = ?", (sid,)).fetchone()
        if sp is None or sp[0] != name:
            refusals.append(f"spellpattern {sid} is {sp!r}, not {name!r}")
    applied = all(same(row_of(db, r["id"]), r) for r in ROWS)
    if applied:
        return lines, refusals, True
    ours = {r["id"] for r in ROWS}
    for id, name, races in db.execute("SELECT id, name, races FROM cmcatalog WHERE (races >> ?) & 1 = 1 AND races != 2047 "
                                      "AND (races & 2047) != 2047", (WISP,)):
        if id not in ours:
            refusals.append(f"cmcatalog {id} ({name}) has the Wisp's bit already (races {races})")
    for r in ROWS:
        now = row_of(db, r["id"])
        if now is not None:
            refusals.append(f"cmcatalog {r['id']} is taken: {now['name']!r} ({now['mastery']!r})")
        kind = "racial" if r["category"] == 1 else "master class"
        lines.append(f"cmcatalog {r['id']} {kind} {r['mastery']} {r['rank']} {r['name']!r}: cost {r['cost']}, level "
                     f"{r['level']}, spent {r['spent']}, effects {r['effects'] or '-'}" + (f", adds {r['adds']}" if r["adds"] else ""))
    return lines, refusals, False


def apply(db):
    db.execute(LOG_SCHEMA)
    for r in ROWS:
        cols = list(r)
        db.execute(f"INSERT INTO cmcatalog ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [r[c] for c in cols])
        row = row_of(db, r["id"])
        a = audit(db, "insert", r["id"], None, row, DETAIL)
        db.execute(f"INSERT INTO {LOG} (id, after, audit) VALUES (?, ?, ?)", (r["id"], json.dumps(row, sort_keys=True), a))


def undo(db):
    """(back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, id, after in db.execute(f"SELECT seq, id, after FROM {LOG} ORDER BY seq DESC").fetchall():
        now = row_of(db, id)
        if now is None or json.dumps(now, sort_keys=True) != after:
            left += 1
            print(f"  left: cmcatalog {id}, changed since it was written: {now}")
            continue
        db.execute("DELETE FROM cmcatalog WHERE id = ?", (id,))
        audit(db, "delete", id, now, None, "undo: " + DETAIL)
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
        lines, refusals, applied = plan(db)
        for line in lines:
            print(line)
        if applied:
            print(f"applied already (the {len(ROWS)} rows are this tool's): nothing done")
            return 0
        if table(db, LOG):
            print(f"{LOG} is there but the rows are not as written: --undo first")
            return 1
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not a.apply:
            print(f"report only: {len(ROWS)} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            apply(db)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {len(ROWS)} rows, in gm_audit and {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
