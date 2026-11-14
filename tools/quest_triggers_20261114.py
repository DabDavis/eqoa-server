#!/usr/bin/env python3
"""The quests of level 20 and under made doable where the server's own check found a step that cannot be done
(the owner, 2026-11-14: "make sure all the quest triggers work for the 1-20 quests"; the check is the Go test
TestRealQuestTriggers, internal/handlers/questcheck_real_test.go, run on fresh copies of both shards).

  lore     Nine quest items a step wants two to four of were Lore (itempattern.lore 1): a character holds one, so the
           second never came off the corpse (a Lore item's drop is refused while one is held) and the hand-in could
           never be made. Lore -> 0, nothing else of them changed:
             700187 shadow cub pelt x3    Scornblade D'Eleth (100168, 100178, 100188) steps 3-4, Grobb
             700507 triad stone x4        Argra (100516) steps 2-3, Oggok
             700513 ruby half x2          The Split Ruby (100535) step 4 (one off each of two creatures), Oggok
             700522 ogre magi ring x2     The Enchanted Totem (100544) steps 6-7, Oggok
             700528 windstrider jewelry x3 Strategy and Tactics (100546) steps 6-7, Oggok
             700727 Grave Silk x3         Grave Silk (100752)
             701225 Black Gnoll Ring x4   Black Gnoll Rings (101173)
             701233 Rune of Crethley Manor Runes of Crethley Manor (101189)
             701283 Gust Chime x3         Binding an Elemental (101223, 101224) step 7
           and one past level 20 the same way (the real-database chain walk's): 701284 Stick of Dynamite, The
           Excavations of Snowblind (101225, level 50): Demolitionist Osbit gives two at step 6, steps 9 and 11 each
           hand one in; the second never came into the pack.
  aggro    Talk steps at a creature that attacked the quest's characters on sight (the World's target face red at the
           quest's level, their starting standings):
             Gertrude, Argra (100516) step 6, the ogre necromancer's level 20 talk in the Bear Cave: a level 45 bear of
             no faction, hostile by nature -> npcaggro ('Gertrude', aggressive 0), both shards;
             ghostly mage, The Hatestone / Hatestone Dust / The Mindbend Robe (100345, 100435, 100445, 100495) step 2,
             a level 30 creature at the Crest of the Duelists: Heirloom has its npcaggro row (aggressive 0, "white face
             on live", video-audit), the Live Server none -> the same row there.
  scroll   Heirloom: Jibrish renamed spell 700049 "Shadowstrike" -> "Shadow Strike" (2026-11-11), and the rogues'
           reward scroll 8541 "Shadowstrike" (A Stunned Mindwhipper 100351, The Fire Bats 100471, Elddar Seeds 100741
           step 2) scribed nothing (scribe.go reads the spell by the scroll's name) -> the scroll's name and words
           "Shadow Strike", where the spell is so named (Live keeps both "Shadowstrike": skipped).

Refused, nothing written, when a row is not as expected (a GM's edit since). Report only by default; --apply in one
transaction, a gm_audit line a row, log quest_triggers_log; a second --apply does nothing; --undo exact (a row changed
since is left and said). Item records are read at each request and npcaggro at the start: aggro needs a restart (a
keep restart's before.sh), the rest takes at once.

    python3 -E -s tools/quest_triggers_20261114.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "quest_triggers_log"
WHO, VIA = "dab", "tool"
DETAIL = "the 1-20 quests' steps made doable (TestRealQuestTriggers): tools/quest_triggers_20261114.py"

# (patternid, name): Lore 1 -> 0
LORE = ((700187, "shadow cub pelt"), (700507, "triad stone"), (700513, "ruby half"), (700522, "ogre magi ring"),
        (700528, "windstrider jewelry"), (700727, "Grave Silk"), (701225, "Black Gnoll Ring"),
        (701233, "Rune of Crethley Manor"), (701283, "Gust Chime"), (701284, "Stick of Dynamite"))

AGGRO = (
    {"npc_name": "Gertrude", "aggressive": 0, "radius": 0.0, "leash": 0.0, "holding": 0, "see_invis": 0,
     "note": "Argra (100516) step 6 is a talk to her in the Bear Cave: no attack on sight (quest_triggers_20261114)",
     "source": "quest-triggers"},
    {"npc_name": "ghostly mage", "aggressive": 0, "radius": 0.0, "leash": 0.0, "holding": 0, "see_invis": 0,
     "note": "white face on live (yiBW3hjxosE 875 s); the Hatestone quests' step 2 talk (Heirloom's row)",
     "source": "video-audit"},
)

SCROLL = 8541
SPELL = 700049
SCROLL_OLD = ("Shadowstrike", "This allows you to scribe Shadowstrike into your spell book.")
SCROLL_NEW = ("Shadow Strike", "This allows you to scribe Shadow Strike into your spell book.")

ROW = "*row*"  # a change of a whole row: before None an insert, after None a delete (the row as a dict)

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,
    keycol  TEXT NOT NULL,
    keyval  TEXT NOT NULL,
    col     TEXT NOT NULL,
    before  TEXT,
    after   TEXT,
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def compact(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False)


def rowdict(db, sql, args):
    cur = db.execute(sql, args)
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def keyed(keycol, keyval):
    return keyval if keycol == "npc_name" else int(keyval)


def audit(db, tbl, keycol, keyval, col, before, after):
    action = "update" if col != ROW else ("delete" if after is None else "insert")
    if col == ROW:
        before, after = ({} if before is None else before), ({} if after is None else after)
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, compact({keycol: keyed(keycol, keyval)}),
                      compact(before if col == ROW else {col: before}), compact(after if col == ROW else {col: after}), DETAIL))
    return cur.lastrowid


def plan(db):
    changes, lines, refusals = [], [], []

    def column(tbl, keycol, key, col, want, after, label):
        r = rowdict(db, f"SELECT * FROM {tbl} WHERE {keycol} = ?", (key,))
        if r is None:
            lines.append(f"{tbl} {key} ({label}) not on this shard: skipped")
            return
        v = r[col]
        if v == after:
            lines.append(f"{tbl} {key} ({label}) {col} = {after!r} already")
        elif v != want:
            refusals.append(f"{tbl} {key} ({label}) {col} = {v!r}, want {want!r}")
        else:
            changes.append((tbl, keycol, str(key), col, v, after))
            lines.append(f"{tbl} {key} ({label}) {col} {v!r} -> {after!r}")

    for pid, name in LORE:
        r = rowdict(db, "SELECT itemname FROM itempattern WHERE patternid = ?", (pid,))
        if r is not None and r["itemname"] != name:
            refusals.append(f"itempattern {pid} is {r['itemname']!r}, want {name!r}")
            continue
        column("itempattern", "patternid", pid, "lore", 1, 0, name)

    cols = [r[1] for r in db.execute("PRAGMA table_info(npcaggro)")]
    for row in AGGRO:
        name = row["npc_name"]
        have = rowdict(db, "SELECT * FROM npcaggro WHERE lower(npc_name) = lower(?)", (name,))
        if have is not None:
            if have["aggressive"] == 0:
                lines.append(f"npcaggro {name!r}: aggressive 0 already")
            else:
                refusals.append(f"npcaggro {name!r} is there, aggressive {have['aggressive']}: a GM's rule, left to them")
            continue
        if not db.execute("SELECT 1 FROM npcs WHERE lower(npc_name) = lower(?)", (name,)).fetchone():
            lines.append(f"npcaggro {name!r}: no NPC of the name here: skipped")
            continue
        new = {c: row[c] for c in cols if c in row}
        changes.append(("npcaggro", "npc_name", name, ROW, None, new))
        lines.append(f"npcaggro {name!r}: aggressive 0 (no attack on sight)")

    sp = rowdict(db, "SELECT spellname FROM spellpattern WHERE spellid = ?", (SPELL,))
    it = rowdict(db, "SELECT itemname, itemdesc FROM itempattern WHERE patternid = ?", (SCROLL,))
    if sp is None or it is None:
        lines.append(f"scroll {SCROLL} / spell {SPELL} not both here: skipped")
    elif sp["spellname"] != SCROLL_NEW[0]:
        lines.append(f"spell {SPELL} is {sp['spellname']!r} here, the scroll {it['itemname']!r}: skipped")
    else:
        column("itempattern", "patternid", SCROLL, "itemname", SCROLL_OLD[0], SCROLL_NEW[0], "the rogues' scroll")
        column("itempattern", "patternid", SCROLL, "itemdesc", SCROLL_OLD[1], SCROLL_NEW[1], "the rogues' scroll")
    return changes, lines, refusals


def write(db, tbl, keycol, keyval, col, before, after):
    key = keyed(keycol, keyval)
    if col == ROW:
        now = rowdict(db, f"SELECT * FROM {tbl} WHERE {keycol} = ?", (key,))
        if after is None:
            if now != before:
                return False
            return db.execute(f"DELETE FROM {tbl} WHERE {keycol} = ?", (key,)).rowcount == 1
        if now is not None:
            return False
        names = list(after)
        db.execute(f"INSERT INTO {tbl} ({', '.join(names)}) VALUES ({', '.join('?' * len(names))})", [after[n] for n in names])
        return True
    return db.execute(f"UPDATE {tbl} SET {col} = ? WHERE {keycol} = ? AND {col} IS ?", (after, key, before)).rowcount == 1


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for _, tbl, keycol, keyval, col, before, after, _a in db.execute(f"SELECT * FROM {LOG} ORDER BY seq DESC").fetchall():
        before, after = json.loads(before), json.loads(after)
        if not write(db, tbl, keycol, keyval, col, after, before):
            print(f"left: {tbl} {keyval} {col} changed since")
            left += 1
            continue
        audit(db, tbl, keycol, keyval, col, after, before)
        back += 1
    if not left:
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
            print(f"put back {back} rows, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        if table(db, LOG):
            print(f"applied already ({LOG} is there): --undo first")
            return 0
        changes, lines, refusals = plan(db)
        for line in lines:
            print(line)
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not changes:
            print("nothing to do")
            return 0
        if not a.apply:
            print(f"report only: {len(changes)} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            db.execute(LOG_SCHEMA)
            for tbl, keycol, keyval, col, before, after in changes:
                if not write(db, tbl, keycol, keyval, col, before, after):
                    raise RuntimeError(f"{tbl} {keyval} changed under the update")
                a_id = audit(db, tbl, keycol, keyval, col, before, after)
                db.execute(f"INSERT INTO {LOG} (tbl, keycol, keyval, col, before, after, audit) VALUES (?, ?, ?, ?, ?, ?, ?)",
                           (tbl, keycol, keyval, col, json.dumps(before), json.dumps(after), a_id))
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {len(changes)} rows, log {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
