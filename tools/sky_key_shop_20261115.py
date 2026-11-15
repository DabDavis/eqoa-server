#!/usr/bin/env python3
"""Liztin Misseliania's keys in her shop (the owner, 2026-11-15: "the plane of sky key vendor should just show the
keys in the shop menu not the menu prompts").

Liztin (world 4, a merchant) sold the main isle's three keys by quest: the first step of The Key of Rain (101301),
The Key of the Sun (101304) and The Chain of Night (101306) was a give of 2,000 / 2,000 / 3,000 tunar to her that
handed over the rain key, the sun key or the key of the ancients, so her window answered with the quests' boxes
before her shop. This:

  - prices the three keys (itempattern.itemcost) 2,000 / 2,000 / 3,000: a merchant sells a catalog item at its value
    at the rate 100 (handlers/merchant.go: merchantPrice, merchantRate) and pays the same less wear (a key has no
    wear), so a key bought and sold back nets nothing; the keys come from nowhere else (no loot table, loot pool or
    other quest), so there is nothing free to sell. They are no-trade (itempattern.trade 1), which a trade refuses
    and a merchant does not: selling one back returns its price;
  - stocks them in her shop (npcitem), first, her twelve other wares after them;
  - takes the three quests off her: their first step, her tunar step, goes, so each begins at its essence's key give
    (step 2), and each is given by that essence (quests.giver: essence of rain / sun / night; its words to her taken
    off, quests.started ''). handlers/skypass.go (StartPassSteps) takes the chain up there when the player gives the
    essence a key and may take the quest (its level 50 and the rest), and does the step: the chain's kills, rewards
    and journal follow as before. Without that code the quests cannot be taken: the two go together;
  - a character on any of the three at step 1 (the tunar step) is moved: holding the quest's key in the pack, on to
    step 2 (the key given at the essence does it); else the quest is put back to where it stood before it was taken
    (a run finished before: step 0, as finished; never finished: the row goes), so the essence takes it up.

Each row written is logged in sky_key_shop_1115_log and gets its own gm_audit line (tbl, rowkey, before, after), so the
GM panel shows each. Refused, nothing written, unless Liztin is one NPC in world 4, each key one record of value 0 (or
the price this sets), each quest as imported (giver Liztin, step 1 her tunar give rewarding its key, step 2 the key
given to its essence), and none of the keys is in her stock. Report only by default (the change run and rolled back);
--apply in one transaction; a second --apply refuses (the log holds rows); --undo puts every row back, each with its
own gm_audit line, and drops the log.

WHEN IT TAKES EFFECT: the shop and its prices at once (npcitem and itempattern are read at each window and sale; a
PS2 sees the keys' value in the catalog record); the quests and characters' quests at the server's next start, with
the StartPassSteps code.

    python3 -E -s tools/sky_key_shop_20261115.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "sky_key_shop_1115_log"
WHO, VIA = "dab", "tool"
DETAIL = "Liztin Misseliania's keys in her shop, the chains begun at their essences: tools/sky_key_shop_20261115.py"
LIZTIN = "Liztin Misseliania"

# quest, its key's name, the price, the essence that gives it now
CHAINS = [
    (101301, "rain key", 2000, "essence of rain"),
    (101304, "sun key", 2000, "essence of sun"),
    (101306, "key of the ancients", 3000, "essence of night"),
]


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def has(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone() is not None


def row(db, sql, params):
    cur = db.execute(sql, params)
    r = cur.fetchone()
    if r is None:
        return None
    return {d[0]: v for d, v in zip(cur.description, r)}


class Writer:
    def __init__(self, db):
        self.db = db
        acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
        self.acct = acct[0] if acct else 0
        self.n = 0

    def log(self, tbl, key, before, after, action="apply"):
        b = json.dumps(before, sort_keys=True) if before is not None else None
        a = json.dumps(after, sort_keys=True) if after is not None else None
        k = json.dumps(key, sort_keys=True)
        cur = self.db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                              "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)", (now(), self.acct, WHO, VIA, action, tbl, k, b, a, DETAIL))
        if action == "apply":
            self.db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)", (tbl, k, b, a, cur.lastrowid))
        self.n += 1
        return cur.lastrowid


def find(db):
    """What is changed, or the refusals."""
    bad = []
    liz = db.execute("SELECT id FROM npcs WHERE npc_name = ? AND world = 4", (LIZTIN,)).fetchall()
    if len(liz) != 1:
        bad.append(f"{len(liz)} NPCs named {LIZTIN} in world 4, want 1")
        return None, bad
    npcid = liz[0][0]
    keys = {}
    for qid, name, price, essence in CHAINS:
        ps = db.execute("SELECT patternid, itemcost FROM itempattern WHERE lower(trim(itemname)) = ?", (name,)).fetchall()
        if len(ps) != 1:
            bad.append(f"{len(ps)} records named {name!r}, want 1")
            continue
        pid, cost = ps[0]
        if cost not in (0, price):
            bad.append(f"{name!r} ({pid}) is valued {cost}, not 0")
        if db.execute("SELECT 1 FROM npcitem WHERE npcid = ? AND patternid = ?", (npcid, pid)).fetchone():
            bad.append(f"{LIZTIN} stocks {name!r} already")
        q = row(db, "SELECT giver, enabled FROM quests WHERE questid = ?", (qid,))
        s1 = row(db, "SELECT kind, target, tunar, rewarditems FROM queststeps WHERE questid = ? AND step = 1", (qid,))
        s2 = row(db, "SELECT kind, target, items FROM queststeps WHERE questid = ? AND step = 2", (qid,))
        if not q or q["giver"] != LIZTIN:
            bad.append(f"quest {qid} is not given by {LIZTIN}: {q}")
        if not s1 or s1["kind"] != "give" or s1["target"] != LIZTIN or s1["tunar"] != price or s1["rewarditems"] != str(pid):
            bad.append(f"quest {qid} step 1 is not her {price} tunar for {pid}: {s1}")
        if not s2 or s2["kind"] != "give" or s2["target"] != essence or s2["items"] != str(pid):
            bad.append(f"quest {qid} step 2 is not {pid} given to {essence}: {s2}")
        keys[qid] = (pid, name, price, essence)
    return (npcid, keys), bad


def apply(db, npcid, keys, say):
    w = Writer(db)
    # the prices
    for qid, (pid, name, price, _) in keys.items():
        old = db.execute("SELECT itemcost FROM itempattern WHERE patternid = ?", (pid,)).fetchone()[0]
        if old != price:
            db.execute("UPDATE itempattern SET itemcost = ? WHERE patternid = ?", (price, pid))
            w.log("itempattern", {"patternid": pid}, {"itemcost": old}, {"itemcost": price})
        say(f"price: {name} ({pid}) {old} -> {price}")
    # her stock: the keys first, the rest after
    ks = len(keys)
    for itemid, ln in db.execute("SELECT itemid, listnumber FROM npcitem WHERE npcid = ? ORDER BY listnumber, itemid", (npcid,)).fetchall():
        db.execute("UPDATE npcitem SET listnumber = ? WHERE itemid = ?", (ln + ks, itemid))
        w.log("npcitem", {"itemid": itemid}, {"listnumber": ln}, {"listnumber": ln + ks})
    for i, (qid, (pid, name, price, _)) in enumerate(sorted(keys.items())):
        cur = db.execute("INSERT INTO npcitem (npcid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber) "
                         "VALUES (?, 1, 0, 0, ?, -1, 1, ?)", (npcid, pid, i))
        r = row(db, "SELECT * FROM npcitem WHERE itemid = ?", (cur.lastrowid,))
        w.log("npcitem", {"itemid": cur.lastrowid}, None, r)
        say(f"stock: {LIZTIN} ({npcid}) sells {name} at list {i}, npcitem {cur.lastrowid}")
    # the quests: begun at the essence
    for qid, (pid, name, price, essence) in keys.items():
        q = row(db, "SELECT giver, started FROM quests WHERE questid = ?", (qid,))
        db.execute("UPDATE quests SET giver = ?, started = '' WHERE questid = ?", (essence, qid))
        w.log("quests", {"questid": qid}, q, {"giver": essence, "started": ""})
        s1 = row(db, "SELECT * FROM queststeps WHERE questid = ? AND step = 1", (qid,))
        db.execute("DELETE FROM queststeps WHERE questid = ? AND step = 1", (qid,))
        w.log("queststeps", {"questid": qid, "step": 1}, s1, None)
        say(f"quest {qid}: given by {essence}, its step 1 ({LIZTIN}'s {price} tunar) gone")
        # characters on the tunar step
        for cq in db.execute("SELECT * FROM charquests WHERE questid = ? AND step = 1", (qid,)).fetchall():
            cq = row(db, "SELECT * FROM charquests WHERE serverid = ? AND questid = ?", (cq[0], qid))
            sid = cq["serverid"]
            who = db.execute("SELECT charname FROM characters WHERE serverid = ?", (sid,)).fetchone()
            who = who[0] if who else f"character {sid}"
            held = db.execute("SELECT COALESCE(SUM(stackleft), 0) FROM charinventory WHERE serverid = ? AND patternid = ? "
                              "AND location = 0 AND equiploc = -1", (sid, pid)).fetchone()[0]
            key = {"serverid": sid, "questid": qid}
            if held > 0:
                db.execute("UPDATE charquests SET step = 2, count = 0 WHERE serverid = ? AND questid = ?", (sid, qid))
                w.log("charquests", key, cq, dict(cq, step=2, count=0))
                say(f"  {who} ({sid}) holds {held} {name}: on to step 2")
            elif cq["done"] > 0:
                after = dict(cq, step=0, count=0, slot=0, picks="")
                db.execute("UPDATE charquests SET step = 0, count = 0, slot = 0, picks = '' WHERE serverid = ? AND questid = ?", (sid, qid))
                w.log("charquests", key, cq, after)
                say(f"  {who} ({sid}) holds no {name}: put back to finished ({cq['done']} runs), taken up at the essence")
            else:
                db.execute("DELETE FROM charquests WHERE serverid = ? AND questid = ?", (sid, qid))
                w.log("charquests", key, cq, None)
                say(f"  {who} ({sid}) holds no {name}: the quest dropped (never finished), taken up at the essence")
    return w.n


def undo(db, say):
    if not has(db, LOG):
        say("nothing to undo")
        return
    w = Writer(db)
    db.execute("BEGIN IMMEDIATE")
    try:
        for tbl, k, b, a in db.execute(f"SELECT tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
            key, before, after = json.loads(k), json.loads(b) if b else None, json.loads(a) if a else None
            where = " AND ".join(f"{c} = ?" for c in key)
            if before is None:  # inserted: delete
                db.execute(f"DELETE FROM {tbl} WHERE {where}", tuple(key.values()))
            elif after is None:  # deleted: insert it again
                cols = list(before)
                db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", tuple(before[c] for c in cols))
            else:
                cur = db.execute(f"SELECT {', '.join(after)} FROM {tbl} WHERE {where}", tuple(key.values())).fetchone()
                if cur is None or list(cur) != [after[c] for c in after]:
                    say(f"changed since, left: {tbl} {key} now {cur}")
                    continue
                sets = ", ".join(f"{c} = ?" for c in before)
                db.execute(f"UPDATE {tbl} SET {sets} WHERE {where}", tuple(before.values()) + tuple(key.values()))
            w.log(tbl, key, after, before, action="undo")
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except Exception:
        db.execute("ROLLBACK")
        raise
    say(f"undone: {w.n} rows")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, timeout=5, isolation_level=None)
    if a.undo:
        undo(db, print)
        return
    if has(db, LOG) and db.execute(f"SELECT 1 FROM {LOG}").fetchone():
        print(f"refused: {LOG} holds rows: the change is in already")
        sys.exit(1)
    found, bad = find(db)
    if bad:
        for b in bad:
            print("refused:", b)
        sys.exit(1)
    npcid, keys = found
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (seq INTEGER PRIMARY KEY AUTOINCREMENT, tbl TEXT NOT NULL, rowkey TEXT NOT NULL, "
                   "before TEXT, after TEXT, audit INTEGER NOT NULL DEFAULT 0, applied TEXT NOT NULL DEFAULT (datetime('now')))")
        n = apply(db, npcid, keys, print)
        first = db.execute(f"SELECT MIN(audit), MAX(audit) FROM {LOG}").fetchone()
        if a.apply:
            db.execute("COMMIT")
            print(f"applied: {n} rows, gm_audit {first[0]}-{first[1]}")
        else:
            db.execute("ROLLBACK")
            print(f"report only: {n} rows would be written (--apply to write)")
    except Exception:
        db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    main()
