#!/usr/bin/env python3
"""The first isle of snow's essences, its gargoyles and Croc Island's Croc (2026-11-14, batch tix5, lane snow1).

  2077-2079, 2085 (the GM Jibcrotwo; Hoofbite): "We need a snow essence right where im standing like the other islands, it is
       used to interact with and port to another spot" (the main isle, (4888.59, 273.81, 5858.41), facing 1.399), "The
       snow essence from the previous ticket exits right where im standing, with another snow essence being here" (the
       first isle of snow, (5682.72, 553.55, 6418.1), facing -3.093). The guide (forum t212, step 1): "Speak to the
       Mark of the Majestic and he will give you a snow key ... Then go back to the main isle on PoS and find the Snow
       Essence Portal."
         - itempattern "snow key", a copy of Liztin's rain key (icon -441710123, craft 0, stacks of 999) under its own
           name and words;
         - npcs "essence of snow" at Jibcrotwo's main-isle spot (a copy of the main isle's essence of night, 119511),
           on the placed floor there (274.32: he stood half a step into it), facing his way (byte 57); its passage
           (handlers/skypass.go) takes a snow key to his spot on the first isle;
         - npcs "essence of snow" on the first isle beside that landing, six ahead of it the way he faced, facing back
           at it (byte 2): the isle's own essence, which has no passage (the guide leaves the first isle by the
           waterfall, quest 101307's Ryini walk, and gives it no key): talked to, it is dim;
         - quest 101307 (The Chain of Snow, the Mark of the Majestic's, for those who finished the chain of night): its
           accept gives the snow key (one, unless one is held) where it sent the player to the first isle, and its
           words say so; it is repeatable, as the main isle's other chains are, so one who lost a key, or finished
           the chain already (petition 2085, Hoofbite: 101306 and 101307 both done on Heirloom), takes it again for
           another (a finish counts on: its unlocks stay open). The Mark's own trip to the first isle (questunlocks: for those who FINISHED the chain) stays,
           as each chain's way back does.
  2080 (Jibcrotwo, on the first isle of snow): "Double the size of all the icebound gargoyles on this island". Every
       npcs row named icebound gargoyle (all 34 stand in world 4 on the first isle of snow, skyisles' outline; none
       anywhere else) size x2 (1.0 -> 2.0, the largest 1.375 -> 2.75). The two Ryini's walk (gmtriggers 206) spawns
       are copies of a standing row (entity npcByNameForLocked): doubled with it, no trigger edit.
  2081 (Jibcrotwo): "Remove the one im targeting, he should not be spawning here. Dont remove the others unless
       explicitly told": npcs 119611, the icebound gargoyle at (5511.73, 494.90, 6442.68), the target's spot; no other
       table holds a row of it (respawn, wander, paths, loot by row: none). Deleted, the row kept in the log.
  5026 (Innoruuk, Live; Heirloom the same): "Crocs in building on croc island by freeport are messed up. There is only
       one and it's under the floor and can't LoS it." npcs 122113, Croc (the hatestone quests' target, seeded from
       FrontiersForge's sighting at (25371.9, 69.95, 17326.3): tools/seed_quest_npcs.py), stands at 68.22, the terrain
       under the hut's placed floor (69.95, world 0's ground and walls files): y -> 69.95, FrontiersForge's and the
       floor's. FrontiersForge saw one Croc there and no census has more: none added.

Refused, nothing written, when a row is not as expected (a GM's edit since). Report only by default; --apply in one
transaction, a gm_audit line a row, log snow1_1114_log; a second --apply does nothing; --undo exact (a row changed since
is left and said; snow keys characters hold go with an undo). --only 2079,2080,2081,5026 limits it. npcs, items and
quests are read at the server's start: a restart, with skypass.go's passage (the same branch).

    python3 -E -s tools/snow1_essence_20261114.py --db eqoa.db [--apply | --undo] [--only 2079,...]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "snow1_1114_log"
WHO, VIA = "dab", "tool"

KEY = "snow key"
KEY_FROM = "rain key"
KEY_DESC = ("A key of snow, given by the Mark of the Majestic. The essence of snow on the main isle takes it and sends "
            "its bearer, and the group beside them, to the first isle of snow.")
ESS_FROM = 119511  # the main isle's essence of night
MAIN = dict(x_coord=4888.59, y_coord=274.32, z_coord=5858.41, facing=57)
SNOW1 = dict(x_coord=5682.43, y_coord=553.75, z_coord=6412.11, facing=2)
SNOW1_ZONE_FROM = 119600  # Ryini, at the first isle's landing
QUEST = 101307
OLD_EVENTS = '[{"on":"accept","do":"teleport","world":4,"x":5670.8,"y":552.88,"z":6358.8}]'
OLD_OFFER = ("You have walked the night. Now the snow: I will open its first isle to you. Ryini waits there, lost and far "
             "from her town. Be ready before you speak to her.")
NEW_OFFER = ("You have walked the night. Now the snow: take this snow key to the essence of snow on the main isle, and it "
             "will open the first isle to you. Ryini waits there, lost and far from her town. Be ready before you speak "
             "to her.")
OLD_ACCEPT, NEW_ACCEPT = "Send me to Ryini.", "Give me the key."
OLD_STARTED = "Ryini waits. Do not speak to her until you are ready."
NEW_STARTED = ("The essence of snow stands on the main isle, south-east of Liztin's hut. Ryini waits beyond it. Do not "
               "speak to her until you are ready.")
GARGOYLE = "icebound gargoyle"
GARG_BOX = (5380, 5870, 6130, 6590)  # x0, x1, z0, z1: the first isle of snow's gargoyles (skyisles: Snow Chain 1)
TARGET = 119611
TARGET_AT = (5511.73, 494.898, 6442.68)
CROC = 122113
CROC_Y, CROC_FLOOR = 68.2188, 69.95

DETAILS = {
    2079: "petitions 2077-2079, 2085 (Jibcrotwo, Hoofbite): an essence of snow on the main isle taking the Mark of the "
          "Majestic's snow key to the first isle of snow, and the first isle's own essence",
    2080: "petition 2080 (Jibcrotwo): the first isle of snow's icebound gargoyles twice the size",
    2081: "petition 2081 (Jibcrotwo): the icebound gargoyle at (5511.7, 494.9, 6442.7) should not spawn there",
    5026: "petition 5026 (Innoruuk): Croc on Croc Island on his hut's floor, not under it",
}

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    ticket  INTEGER NOT NULL,
    tbl     TEXT NOT NULL,
    keycol  TEXT NOT NULL,
    keyval  TEXT NOT NULL,
    col     TEXT NOT NULL,
    before  TEXT,
    after   TEXT,
    audit   INTEGER NOT NULL DEFAULT 0
)"""

ROW = "*row*"  # a change of a whole row: before None an insert, after None a delete (the row as a dict)


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def compact(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False)


def rowdict(db, sql, args):
    cur = db.execute(sql, args)
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def next_id(db, tbl, col):
    seq = db.execute("SELECT seq FROM sqlite_sequence WHERE name = ?", (tbl,)).fetchone() if table(db, "sqlite_sequence") else None
    top = db.execute(f"SELECT COALESCE(MAX({col}), 0) FROM {tbl}").fetchone()[0]
    return max(top, seq[0] if seq else 0) + 1


def audit(db, ticket, tbl, keycol, keyval, col, before, after):
    action = "update" if col != ROW else ("delete" if after is None else "insert")
    if col == ROW:
        before, after = ({} if before is None else before), ({} if after is None else after)
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, compact({keycol: int(keyval)}),
                      compact(before if col == ROW else {col: before}), compact(after if col == ROW else {col: after}),
                      DETAILS[ticket] + ": tools/snow1_essence_20261114.py"))
    return cur.lastrowid


class Plan:
    def __init__(self, db):
        self.db, self.changes, self.lines, self.refusals = db, [], [], []

    def column(self, ticket, tbl, keycol, key, col, want, after, label):
        row = rowdict(self.db, f"SELECT * FROM {tbl} WHERE {keycol} = ?", (key,))
        if row is None:
            self.refusals.append(f"{ticket}: {tbl} {key} ({label}) is not there")
            return
        v = row[col]
        if v not in want:
            self.refusals.append(f"{ticket}: {tbl} {key} ({label}) {col} = {v!r}, want {want!r}")
            return
        self.changes.append((ticket, tbl, keycol, key, col, v, after))
        self.lines.append(f"{ticket}: {tbl} {key} ({label}) {col} {v!r} -> {after!r}")

    def insert(self, ticket, tbl, keycol, row, label):
        self.changes.append((ticket, tbl, keycol, row[keycol], ROW, None, row))
        self.lines.append(f"{ticket}: {tbl} {row[keycol]} ({label}) added")

    def delete(self, ticket, tbl, keycol, key, label, check):
        row = rowdict(self.db, f"SELECT * FROM {tbl} WHERE {keycol} = ?", (key,))
        if row is None:
            self.refusals.append(f"{ticket}: {tbl} {key} ({label}) is not there")
            return
        bad = check(row)
        if bad:
            self.refusals.append(f"{ticket}: {tbl} {key}: {bad}")
            return
        self.changes.append((ticket, tbl, keycol, key, ROW, row, None))
        self.lines.append(f"{ticket}: {tbl} {key} ({label}) deleted")


def near(r, x, z, d):
    return abs(r["x_coord"] - x) <= d and abs(r["z_coord"] - z) <= d


def plan(db, only):
    p = Plan(db)
    if 2079 in only:
        if db.execute("SELECT 1 FROM itempattern WHERE lower(trim(itemname)) = ?", (KEY,)).fetchone():
            p.refusals.append(f"2079: a record is named {KEY!r} already")
        src = rowdict(db, "SELECT * FROM itempattern WHERE lower(itemname) = ?", (KEY_FROM,))
        if src is None or src["itemicon"] != -441710123 or src["craft"] != 0 or src["maxstack"] != 999:
            p.refusals.append(f"2079: {KEY_FROM!r} is not as imported (icon -441710123, craft 0, 999 a stack): {src and (src['itemicon'], src['craft'], src['maxstack'])}")
        ess = rowdict(db, "SELECT * FROM npcs WHERE id = ?", (ESS_FROM,))
        if ess is None or ess["npc_name"] != "essence of night" or ess["modelid"] != 2168708208 or ess["npc_type"] != 128:
            p.refusals.append(f"2079: npcs {ESS_FROM} is not the main isle's essence of night")
        ryini = rowdict(db, "SELECT * FROM npcs WHERE id = ?", (SNOW1_ZONE_FROM,))
        if ryini is None or ryini["npc_name"] != "Ryini":
            p.refusals.append(f"2079: npcs {SNOW1_ZONE_FROM} is not Ryini")
        for spot, what in ((MAIN, "the main isle's"), (SNOW1, "the first isle of snow's")):
            for r in db.execute("SELECT id, npc_name, x_coord, z_coord FROM npcs WHERE world = 4 AND lower(npc_name) = 'essence of snow'").fetchall():
                if abs(r[2] - spot["x_coord"]) <= 20 and abs(r[3] - spot["z_coord"]) <= 20:
                    p.refusals.append(f"2079: npcs {r[0]} is an essence of snow at {what} spot already")
        q = rowdict(db, "SELECT * FROM quests WHERE questid = ?", (QUEST,))
        if q is None or q["giver"] != "Mark of the Majestic":
            p.refusals.append(f"2079: quest {QUEST} is not the Mark of the Majestic's chain of snow")
        if p.refusals:
            return p
        pid = next_id(db, "itempattern", "patternid")
        item = dict(src, patternid=pid, itemname=KEY, itemdesc=KEY_DESC)
        p.insert(2079, "itempattern", "patternid", item, KEY)
        nid = next_id(db, "npcs", "id")
        for i, (spot, zone, label) in enumerate(((MAIN, ess["zone"], "essence of snow, the main isle"),
                                                  (SNOW1, ryini["zone"], "essence of snow, the first isle of snow"))):
            row = dict(ess, id=nid + i, npc_id=nid + i, npc_name="essence of snow", zone=zone, **spot)
            p.insert(2079, "npcs", "id", row, label)
        p.column(2079, "quests", "questid", QUEST, "events", (OLD_EVENTS,),
                 compact([{"on": "accept", "do": "give", "items": f"{pid}*1", "unless_held": True}]), "The Chain of Snow")
        p.column(2079, "quests", "questid", QUEST, "offer", (OLD_OFFER,), NEW_OFFER, "The Chain of Snow")
        p.column(2079, "quests", "questid", QUEST, "accept", (OLD_ACCEPT,), NEW_ACCEPT, "The Chain of Snow")
        p.column(2079, "quests", "questid", QUEST, "started", (OLD_STARTED,), NEW_STARTED, "The Chain of Snow")
        # taken again for another key, as the main isle's other chains are (101301, 101304, 101306 repeatable):
        # one who finished it (petition 2085, Hoofbite) keeps its unlocks (a finish counts) and gets a key
        p.column(2079, "quests", "questid", QUEST, "repeatable", (0,), 1, "The Chain of Snow")
    if 2081 in only:
        def target(r):
            if r["npc_name"] != GARGOYLE or r["world"] != 4 or any(abs(a - b) > 0.05 for a, b in
                                                                    zip((r["x_coord"], r["y_coord"], r["z_coord"]), TARGET_AT)):
                return f"{r['npc_name']!r} in world {r['world']} at ({r['x_coord']}, {r['y_coord']}, {r['z_coord']}), not the target"
            return None
        p.delete(2081, "npcs", "id", TARGET, "the icebound gargoyle Jibcrotwo targeted", target)
        for tbl, col in (("npcrespawn", "npc_row"), ("npcwander", "npcid"), ("npcpaths", "npcid"), ("npctitle", "npcid"),
                         ("npcrare", "npc_row"), ("npcstands", "npc_row"), ("npcitem", "npcid")):
            if table(db, tbl) and db.execute(f"SELECT 1 FROM {tbl} WHERE {col} = ?", (TARGET,)).fetchone():
                p.refusals.append(f"2081: {tbl} holds a row of npcs {TARGET}: not foreseen")
    if 2080 in only:
        rows = db.execute("SELECT id, world, x_coord, z_coord, size FROM npcs WHERE lower(npc_name) = ? ORDER BY id",
                          (GARGOYLE,)).fetchall()
        x0, x1, z0, z1 = GARG_BOX
        for rid, world, x, z, size in rows:
            if 2081 in only and rid == TARGET:
                continue
            if world != 4 or not (x0 <= x <= x1 and z0 <= z <= z1):
                p.refusals.append(f"2080: npcs {rid} ({GARGOYLE}) stands in world {world} at ({x}, {z}): not the first isle of snow")
                continue
            if size not in (1.0, 1.375):
                p.refusals.append(f"2080: npcs {rid} size {size}, want 1.0 or 1.375 (doubled already?)")
                continue
            p.column(2080, "npcs", "id", rid, "size", (size,), size * 2, GARGOYLE)
    if 5026 in only:
        r = rowdict(db, "SELECT * FROM npcs WHERE id = ?", (CROC,))
        if r is None or r["npc_name"] != "Croc" or r["world"] != 0 or not near(r, 25371.9, 17326.3, 0.5):
            p.refusals.append(f"5026: npcs {CROC} is not Croc at (25371.9, 17326.3) in world 0")
        else:
            p.column(5026, "npcs", "id", CROC, "y_coord", (CROC_Y,), CROC_FLOOR, "Croc, Croc Island")
    return p


def write(db, tbl, keycol, keyval, col, before, after):
    if col == ROW:
        now = rowdict(db, f"SELECT * FROM {tbl} WHERE {keycol} = ?", (keyval,))
        if after is None:  # delete: the row must be as it was read
            if now != before:
                return False
            return db.execute(f"DELETE FROM {tbl} WHERE {keycol} = ?", (keyval,)).rowcount == 1
        if now is not None:  # insert: nothing there
            return False
        names = list(after)
        db.execute(f"INSERT INTO {tbl} ({', '.join(names)}) VALUES ({', '.join('?' * len(names))})", [after[n] for n in names])
        return True
    return db.execute(f"UPDATE {tbl} SET {col} = ? WHERE {keycol} = ? AND {col} IS ?", (after, keyval, before)).rowcount == 1


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for _, ticket, tbl, keycol, keyval, col, before, after, _a in db.execute(f"SELECT * FROM {LOG} ORDER BY seq DESC").fetchall():
        before, after = json.loads(before), json.loads(after)
        keyval = int(keyval)
        if tbl == "itempattern" and col == ROW and before is None:
            # the snow keys characters hold go with the record
            n = db.execute("DELETE FROM charinventory WHERE patternid = ?", (keyval,)).rowcount if table(db, "charinventory") else 0
            if n:
                print(f"{ticket}: {n} snow keys characters held taken back")
        if not write(db, tbl, keycol, keyval, col, after, before):
            print(f"left: {ticket} {tbl} {keyval} {col} changed since")
            left += 1
            continue
        audit(db, ticket, tbl, keycol, keyval, col, after, before)
        back += 1
    if not left:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    ap.add_argument("--only", default=",".join(str(t) for t in DETAILS))
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    only = {int(t) for t in a.only.split(",") if t.strip()}
    if not only <= set(DETAILS):
        sys.exit(f"--only: unknown petitions {sorted(only - set(DETAILS))}")
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
            print(f"applied already ({LOG} is there): nothing to do (--undo first to redo)")
            return 0
        p = plan(db, only)
        for line in p.lines:
            print(line)
        if p.refusals:
            print("refused, nothing done:")
            for r in p.refusals:
                print("  " + r)
            return 1
        if not p.changes:
            print("nothing to do")
            return 0
        if not a.apply:
            print(f"report only: {len(p.changes)} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            db.execute(LOG_SCHEMA)
            for ticket, tbl, keycol, keyval, col, before, after in p.changes:
                if not write(db, tbl, keycol, keyval, col, before, after):
                    raise RuntimeError(f"{tbl} {keyval} changed under the update")
                a_id = audit(db, ticket, tbl, keycol, keyval, col, before, after)
                db.execute(f"INSERT INTO {LOG} (ticket, tbl, keycol, keyval, col, before, after, audit) "
                           "VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
                           (ticket, tbl, keycol, str(keyval), col, json.dumps(before), json.dumps(after), a_id))
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {len(p.changes)} rows, log {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
