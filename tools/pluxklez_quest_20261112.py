#!/usr/bin/env python3
"""Pluxklez's toad eyes as a Pusling faction quest, and the Pusling owed (petition 1651, Skywalker: "Pluxklez quest for
plane of disease hunting and gathering quest ... Players need at least 150 faction with The Pusling to log this quest ... Collect
Bulging Toad Eye x4 ... Each completed turn-in rewards: Scuzz Toad Stew x1, 10 faction with The Pusling. Notes:
changed from an XP reward to a faction reward ... Bulging Toad Eye is tradeable.").

The quest is the import's "Scuzz Toad Eyes" (tools/quests/data/portal.json, Portal/plane-of-disease/stew; found by its
giver Pluxklez and its name, as ids differ between shards; no npcs row is read or written, so a GM's move of
Pluxklez (live Heirloom: row 122029 deleted, now 164589 near (4776, 3695)) changes nothing here): a loot step (four eyes off the scuzz toads, chance 0.5 a
kill, as the plagueswine tusks') and a give step at Pluxklez, repeatable. Its rows changed:

  - itempattern (the eye, found by its name "scuzz toad eye"): itemname -> "bulging toad eye" (a creature part's name
    lowercase, as "crusty goblin eye" which it was made like), itemdesc -> its bulging eye, trade 1 -> 0 (itempattern.
    trade is CLIENT's m_noTrade: 1 had come from "crusty goblin eye"; 0 trades, auctions);
  - quest_import_items: the eye's logged name, so tools/import_quests.py with the file as now edited finds the item it
    made rather than making a second one;
  - quests: offer, started and notes say the eyes and the puslings. NO standing asked (the owner, 2026-11-12, over the
    petition's 150: no character could reach it, -400 the start and +450 the most the one-time quests give): anyone of
    its levels takes it and does it again;
  - queststeps 1 and 2: journals say bulging toad eyes and the lakes (step 2's no longer "where the Lich Overlord spawns": a GM moved
    him away from there); step 2's xp 251000 -> 0 and factions "" ->
    "The Pusling:+10" (each turn-in: the quest stays repeatable).

The stew (scuzz toad stew, one a turn-in) and the drop chance are as they were.

And the Pusling owed (the owner, 2026-11-12): every character with a step done that gives The Pusling (queststeps.
factions; on this database Bonefisher Chklurgl's in 101289, +250, and the stinger shown in 101290, +200; Pluxklez's
own +10 left out, it gave experience until now) is owed the sum of those steps' changes. A step is done when its quest
is finished (charquests.done > 0) or the character is past it (charquests.step > the step). What it has had of them is
its standing less the faction's start (factions.standing; refused if any factionstart row names the faction): nothing
else moves The Pusling (no factionhits, no npcfaction, no event or trigger names it), so a standing above the start
is what played steps gave. It is granted what it lacks: owed less had, none when had is as much or more (no double
grant: a character that earned it by playing has its standing already), the standing kept within -2000..2000; a
character without a charfactions row for it gets one. Sandstorm transfers have the quests marked done and no row.
Each grant is logged in pluxklez_grant_1112_log (the row before, the row after) with a gm_audit line. charfactions is
read at login and written over at logout from the player in memory, so run it with nobody in the world: a keep
restart's before.sh (after the save, before the new process loads the rows) or with the server stopped. Every changed column is logged in
pluxklez_quest_1112_log with its value before and after, a gm_audit line each; the import's digest for the quest is
left, so a run again of tools/import_quests.py keeps these rows as a GM's change.

Refused, nothing written, when a row is not as imported (the quest, its steps, the eye, the stew, the faction). Report
only by default; --apply in one transaction; a second --apply does nothing; --undo exact (a column or standing changed since is
left and said). Quests are read at the server's start and standings at login: run it as a keep restart's before.sh
(items' names and the trade flag are read at each item's load).

    tools/pluxklez_quest_20261112.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "pluxklez_quest_1112_log"
WHO, VIA = "dab", "tool"
DETAIL = ("Pluxklez's toad eyes: bulging toad eyes tradeable, stew and +10 Pusling a turn-in, no experience, and the "
          "Pusling owed for done steps (petition 1651, Skywalker): tools/pluxklez_quest_20261112.py")
GRANT_LOG = "pluxklez_grant_1112_log"
FACTION_MIN, FACTION_MAX = -2000, 2000

GIVER, QUEST = "Pluxklez", "Scuzz Toad Eyes"
EYE_OLD, EYE_NEW = "scuzz toad eye", "bulging toad eye"
STEW = "scuzz toad stew"
FACTION = "The Pusling"

OFFER_OLD = ("Scuzz toads in the lakes past the second tunnel have good eyes for stew. Bring Pluxklez four, and Pluxklez "
             "gives stew. Again and again!")
OFFER_NEW = ("Scuzz toads around the lakes of this plane have bulging eyes, good for stew. Bring Pluxklez four, and "
             "Pluxklez gives stew, and the puslings remember it. Again and again!")
STARTED_OLD, STARTED_NEW = "Four eyes!", "Four bulging eyes!"
J1_OLD = "Take four scuzz toad eyes from the scuzz toads in Gluboxious's lake."
J1_NEW = "Take four bulging toad eyes from the scuzz toads around the lakes of the Plane of Disease."
J2_OLD = "Bring four scuzz toad eyes to Pluxklez, where the Lich Overlord spawns."
J2_NEW = "Bring four bulging toad eyes to Pluxklez in the Plane of Disease."
DESC_OLD, DESC_NEW = "The eye of a scuzz toad.", "The bulging eye of a scuzz toad."
NOTE_ADD = (" Petition 1651 (Skywalker, 2026-11-12): the eyes are bulging toad eyes and tradeable; the quest gives the "
            "stew and 10 with The Pusling a turn-in, no experience; no standing is asked (the owner over the petition's "
            "150: none could reach it) "
            "(tools/pluxklez_quest_20261112.py).")

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    rowkey TEXT NOT NULL,
    col    TEXT NOT NULL,
    before TEXT NOT NULL,
    after  TEXT NOT NULL,
    audit  INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def compact(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False, sort_keys=True)


def audit(db, tbl, key, col, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', 'update', ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, tbl, compact(key), compact({col: before}), compact({col: after}), DETAIL))
    return cur.lastrowid


def where(key):
    cols = sorted(key)
    return " AND ".join(f"{c} = ?" for c in cols), [key[c] for c in cols]


def read(db, tbl, key, col):
    w, args = where(key)
    rows = db.execute(f"SELECT {col} FROM {tbl} WHERE {w}", args).fetchall()
    return rows[0][0] if len(rows) == 1 else ("<missing>" if not rows else "<several>")


def write(db, tbl, key, col, before, after):
    w, args = where(key)
    cur = db.execute(f"UPDATE {tbl} SET {col} = ? WHERE {w} AND {col} IS ?", [after] + args + [before])
    return cur.rowcount == 1


def plan(db):
    """(changes [(tbl, key, col, before, after)], refusals)."""
    bad = []
    q = db.execute("SELECT questid FROM quests WHERE giver = ? AND name = ?", (GIVER, QUEST)).fetchall()
    if len(q) != 1:
        return [], [f"{len(q)} quests {QUEST!r} given by {GIVER}, want 1"]
    qid = q[0][0]
    eye = db.execute("SELECT patternid FROM itempattern WHERE itemname = ?", (EYE_OLD,)).fetchall()
    if len(eye) != 1:
        return [], [f"{len(eye)} items named {EYE_OLD!r}, want 1"]
    eye = eye[0][0]
    if db.execute("SELECT 1 FROM itempattern WHERE lower(trim(itemname)) = ?", (EYE_NEW,)).fetchone():
        bad.append(f"an item is named {EYE_NEW!r} already")
    stew = db.execute("SELECT patternid FROM itempattern WHERE itemname = ?", (STEW,)).fetchall()
    if len(stew) != 1:
        return [], [f"{len(stew)} items named {STEW!r}, want 1"]
    stew = stew[0][0]
    if not db.execute("SELECT 1 FROM factions WHERE name = ?", (FACTION,)).fetchone():
        bad.append(f"no faction {FACTION!r}")
    if db.execute("SELECT count(*) FROM queststeps WHERE questid = ?", (qid,)).fetchone()[0] != 2:
        bad.append(f"quest {qid} has not its two steps")
    r = db.execute("SELECT kind, target, items FROM queststeps WHERE questid = ? AND step = 1", (qid,)).fetchone()
    if r != ("loot", "scuzz toad", f"{eye}*4"):
        bad.append(f"quest {qid} step 1 is {r}, not the loot of four {eye} off scuzz toads")
    r = db.execute("SELECT kind, target, items, rewarditems FROM queststeps WHERE questid = ? AND step = 2", (qid,)).fetchone()
    if r != ("give", GIVER, f"{eye}*4", str(stew)):
        bad.append(f"quest {qid} step 2 is {r}, not four {eye} given to {GIVER} for the stew {stew}")
    r = db.execute("SELECT repeatable FROM quests WHERE questid = ?", (qid,)).fetchone()
    if r != (1,):
        bad.append(f"quest {qid} is not repeatable")

    notes = read(db, "quests", {"questid": qid}, "notes")
    changes = [
        ("itempattern", {"patternid": eye}, "itemname", EYE_OLD, EYE_NEW),
        ("itempattern", {"patternid": eye}, "itemdesc", DESC_OLD, DESC_NEW),
        ("itempattern", {"patternid": eye}, "trade", 1, 0),
        ("quests", {"questid": qid}, "offer", OFFER_OLD, OFFER_NEW),
        ("quests", {"questid": qid}, "started", STARTED_OLD, STARTED_NEW),
        ("quests", {"questid": qid}, "notes", notes, (notes if isinstance(notes, str) else "") + NOTE_ADD),
        ("queststeps", {"questid": qid, "step": 1}, "journal", J1_OLD, J1_NEW),
        ("queststeps", {"questid": qid, "step": 2}, "journal", J2_OLD, J2_NEW),
        ("queststeps", {"questid": qid, "step": 2}, "xp", 251000, 0),
        ("queststeps", {"questid": qid, "step": 2}, "factions", "", f"{FACTION}:+10"),
    ]
    if table(db, "quest_import_items") and db.execute("SELECT 1 FROM quest_import_items WHERE patternid = ?", (eye,)).fetchone():
        changes.append(("quest_import_items", {"patternid": eye}, "name", EYE_OLD, EYE_NEW))
    for tbl, key, col, before, _ in changes:
        have = read(db, tbl, key, col)
        if have != before:
            bad.append(f"{tbl} {compact(key)} {col} is {have!r}, want {before!r}")
    if read(db, "quests", {"questid": qid}, "factionreq") != "":
        bad.append(f"quest {qid} asks a standing already")
    return changes, bad


def pusling_steps(db, skip):
    """[(questid, step, delta)] of every quest step but quest skip's that changes The Pusling."""
    out = []
    for qid, step, f in db.execute("SELECT questid, step, factions FROM queststeps WHERE factions LIKE ? ORDER BY questid, step",
                                   (f"%{FACTION}%",)):
        if qid == skip:
            continue
        for part in f.split(","):
            name, _, d = part.rpartition(":")
            if name.strip() == FACTION:
                out.append((qid, step, int(d.strip().lstrip("+"))))
    return out


def plan_grants(db):
    """([(serverid, name, before or None, after, owed, had)], lines, refusals)."""
    if db.execute("SELECT count(*) FROM factionstart WHERE faction = ?", (FACTION,)).fetchone()[0] if table(db, "factionstart") else 0:
        return [], [], [f"factionstart has rows for {FACTION}: a start per character is not this tool's"]
    r = db.execute("SELECT standing FROM factions WHERE name = ?", (FACTION,)).fetchone()
    if r is None:
        return [], [], [f"no faction {FACTION!r}"]
    start = r[0]
    q = db.execute("SELECT questid FROM quests WHERE giver = ? AND name = ?", (GIVER, QUEST)).fetchone()
    steps = pusling_steps(db, q[0] if q else None)
    lines = [f"{FACTION} (start {start}) is given by: " + ", ".join(f"quest {a} step {b} {c:+d}" for a, b, c in steps)]
    owed = {}
    for qid, step, d in steps:
        for sid, in db.execute("SELECT cq.serverid FROM charquests cq JOIN characters c ON c.serverid = cq.serverid "
                               "WHERE cq.questid = ? AND (cq.done > 0 OR cq.step > ?)", (qid, step)):
            owed[sid] = owed.get(sid, 0) + d
    grants = []
    for sid in sorted(owed):
        name = db.execute("SELECT charName FROM characters WHERE serverid = ?", (sid,)).fetchone()[0]
        r = db.execute("SELECT standing FROM charfactions WHERE serverid = ? AND faction = ?", (sid, FACTION)).fetchone()
        before = r[0] if r else None
        had = (before if r else start) - start
        give = owed[sid] - had
        if give <= 0:
            continue
        after = max(FACTION_MIN, min(FACTION_MAX, (before if r else start) + give))
        grants.append((sid, name, before, after, owed[sid], had))
    return grants, lines, []


def audit_grant(db, sid, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    action = "insert" if before is None else ("delete" if after is None else "update")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, 'charfactions', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, compact({"serverid": sid, "faction": FACTION}),
                      None if before is None else compact({"standing": before}),
                      None if after is None else compact({"standing": after}), DETAIL))
    return cur.lastrowid


def apply_grants(db, grants):
    db.execute(f"""CREATE TABLE IF NOT EXISTS {GRANT_LOG} (
        serverid INTEGER PRIMARY KEY, name TEXT NOT NULL, before INTEGER, after INTEGER NOT NULL,
        owed INTEGER NOT NULL, had INTEGER NOT NULL, audit INTEGER NOT NULL DEFAULT 0)""")
    for sid, name, before, after, owed, had in grants:
        if before is None:
            db.execute("INSERT INTO charfactions (serverid, faction, standing) VALUES (?, ?, ?)", (sid, FACTION, after))
        elif db.execute("UPDATE charfactions SET standing = ? WHERE serverid = ? AND faction = ? AND standing = ?",
                        (after, sid, FACTION, before)).rowcount != 1:
            raise RuntimeError(f"charfactions {sid} changed under the grant")
        a_id = audit_grant(db, sid, before, after)
        db.execute(f"INSERT INTO {GRANT_LOG} VALUES (?, ?, ?, ?, ?, ?, ?)", (sid, name, before, after, owed, had, a_id))


def undo_grants(db):
    if not table(db, GRANT_LOG):
        return 0, 0
    back = left = 0
    for sid, before, after in db.execute(f"SELECT serverid, before, after FROM {GRANT_LOG} ORDER BY serverid").fetchall():
        if before is None:
            n = db.execute("DELETE FROM charfactions WHERE serverid = ? AND faction = ? AND standing = ?", (sid, FACTION, after)).rowcount
        else:
            n = db.execute("UPDATE charfactions SET standing = ? WHERE serverid = ? AND faction = ? AND standing = ?",
                           (before, sid, FACTION, after)).rowcount
        if n != 1:
            print(f"left: charfactions {sid} {FACTION} changed since the grant (it played on)")
            left += 1
            continue
        audit_grant(db, sid, after, before)
        back += 1
    if not left:
        db.execute(f"DROP TABLE {GRANT_LOG}")
    return back, left


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for _, tbl, key, col, before, after, _ in db.execute(f"SELECT * FROM {LOG} ORDER BY seq DESC").fetchall():
        key, before, after = json.loads(key), json.loads(before), json.loads(after)
        if not write(db, tbl, key, col, after, before):
            print(f"left: {tbl} {compact(key)} {col} changed since")
            left += 1
            continue
        audit(db, tbl, key, col, after, before)
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
                gback, gleft = undo_grants(db)
                back, left = undo(db)
                db.execute("COMMIT")
            except BaseException:
                db.execute("ROLLBACK")
                raise
            print(f"put back {back} columns and {gback} standings, {left + gleft} left (changed since)"
                  if back or left or gback or gleft else "nothing to undo")
            return 0 if left + gleft == 0 else 1
        if table(db, LOG):
            print(f"applied already ({LOG} is there): --undo first")
            return 0
        changes, refusals = plan(db)
        grants, glines, grefusals = plan_grants(db)
        refusals += grefusals
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        for tbl, key, col, before, after in changes:
            show = (lambda v: (v[:60] + "...") if isinstance(v, str) and len(v) > 63 else v)
            print(f"{tbl} {compact(key)} {col}: {show(before)!r} -> {show(after)!r}")
        for line in glines:
            print(line)
        for sid, name, before, after, owed, had in grants:
            print(f"  {name} ({sid}): owed {owed:+d}, had {had:+d}: {FACTION} {'none (start)' if before is None else before} -> {after}")
        if not a.apply:
            print(f"report only: {len(changes)} columns and {len(grants)} standings would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            db.execute(LOG_SCHEMA)
            for tbl, key, col, before, after in changes:
                if not write(db, tbl, key, col, before, after):
                    raise RuntimeError(f"{tbl} {compact(key)} {col} changed under the update")
                a_id = audit(db, tbl, key, col, before, after)
                db.execute(f"INSERT INTO {LOG} (tbl, rowkey, col, before, after, audit) VALUES (?, ?, ?, ?, ?, ?)",
                           (tbl, compact(key), col, compact(before), compact(after), a_id))
            apply_grants(db, grants)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {len(changes)} columns ({LOG}), {len(grants)} standings ({GRANT_LOG}); restart the server (quests are read at its start)")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
