#!/usr/bin/env python3
"""a GM's requests of 2026-11-16/17 (work board 1567, 1571, 1572, 1599; a GM board 137, 140, 141, 148): owner-approved
as submitted. Three parts, each its own subcommand, log and --undo:

robe (1567, Live): warriors may wear the Spirit Maker's Robe on the Live shard too, as Heirloom already has it.
    itempattern 5209 and every copy whose itemimported.base is 5209: classuse | 1 (Warrior). REFUSED unless 5209's
    classuse is 1006 (a GM: "If 5209's classuse on Live is not 1006, stop and tell me before writing"). Only widens.

testers (1571, 1572, Live: the character Innoruuk plays there alone): two GM charms, Tester Pro (965007) cloned with a
    new name and description (gem type, NO TRADE, LORE, worth 0, the Ring of Speed's icon, 80 DUR, every class/race):
    - 965008 "Tester Supremacy": `Self, Deputy's Draught 452, Radius 30` (entity/moogieitems.go drinkDeputyLocked:
      Spirit of the Wolf on the user and every player within 30), Spirit of the Wolf's icon, 999 charges;
    - 965009 "Pronouns: In/Vis": `Target, Invis, 10min` as "Superior Invisibility" (spellpattern 700685's icon), 999
      charges: a helping Target use, which lands on a friendly player or pet only with work board 693 step 1's server
      (jib-1117 45913d55); before it a use is refused "No target." and spends no charge.
    --give puts one of each into Innoruuk's pack with remaincharge 999 (the panel's offline give writes 0): he sees them
    at his next login (eject him to see them now). Refused while he holds either.

elder (1599, Heirloom): the robe from the two Tower of Phantoms elder spirits alone (npcs #190044, #190045, world 2) at
    0.1% a kill, not the four Tunaria desert elder spirits. NEEDS the server with work board 1554 (open-world "#row"
    npcloot) and the fractional npcloot chance (1310 item 2): jib-1117. In 1599's order:
      1-2. npcloot (#190044 / #190045, rare spirit makers robe, chance 0.1, rolls 1);
      3. lootrate (a GM, xp, spirit makers robe) out: 1598's stopgap (rate 0.001, source workboard 1598);
      4. lootpool 12158 out (spirit makers robe, elder spirit, 5209, source workboard 1598).
    A shard where the name still holds robes5117's binding (npcloot elder spirit -> rare spirit makers robe, 10: Live)
    has that row taken out in 3-4's place. Each removal only as 1598/robes5117 wrote it, else REFUSED.

spectre (the owner, 2026-11-17: "spectre robe should be a 0.1 % drop"; both shards): every npcloot row of the loot
    table "rare spectre robe" (5123: spectre, wandering spectre, spectral guardian/minion/servant, rallosian spectre at
    3, spectral slayer at 1; robes5117, valkanith-ruby1117) to chance 0.1. Needs the fractional chance (jib-1117): an
    older server's int scan fails on 0.1 and drops ALL of that name's tables. REFUSED if a row's chance is other than
    1 or 3 (someone changed it since: look first).

Each: a report by default; --apply writes in one transaction, a gm_audit line a row and a log row; a second run finds
nothing to do; --undo puts back exactly what was there (only rows as written; a row changed since is left and said).
Data only, no restart: the records reach a client at its login; uses and loot tables are read at each use and kill.

    python3 -E -s tools/jib1117_20261117.py robe    --db eqoa.db [--apply | --undo]
    python3 -E -s tools/jib1117_20261117.py testers --db eqoa.db [--apply | --give | --undo]
    python3 -E -s tools/jib1117_20261117.py elder   --db eqoa.db [--apply | --undo]
    python3 -E -s tools/jib1117_20261117.py spectre --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

WHO, VIA = "dab", "tool"
DETAIL = "jibrish's requests of 2026-11-16/17 (work board {}): tools/jib1117_20261117.py {}"

# --- robe -------------------------------------------------------------------------------------------------------------
ROBE = 5209
ROBE_NAME = "Spirit Maker's Robe"
ROBE_WAS = 1006
WARRIOR = 1

# --- testers ----------------------------------------------------------------------------------------------------------
TESTER_PRO = 965007
CHAR = "Innoruuk"
ICON_FROM = (960149, "Ring of Speed", -1505803670)
SOW = (452, "Spirit of the Wolf", 638503769)
SUPINVIS = (700685, "Superior Invisibility", -1664015648)  # its icon as spellpattern keeps it (signed)
CHARGES = 999
TESTERS = [
    dict(patternid=965008, name="Tester Supremacy", entry=1571,
         desc="A tester's charm. Use it and Spirit of the Wolf lands on you and on every player within 30 of you. "
              "It holds 999 uses.",
         effect=dict(line=f"Self, Deputy's Draught {SOW[0]}, Radius 30", name="Tester Supremacy",
                     icon=SOW[2] & 0xFFFFFFFF)),
    dict(patternid=965009, name="Pronouns: In/Vis", entry=1572,
         desc="A tester's charm. Target yourself, a groupmate or a pet and use it for Superior Invisibility, 10 minutes. "
              "It holds 999 uses.",
         effect=dict(line="Target, Invis, 10min", name="Superior Invisibility", icon=SUPINVIS[2] & 0xFFFFFFFF)),
]

# --- elder ------------------------------------------------------------------------------------------------------------
ELDER_ROWS = (190044, 190045)
ELDER_NAME = "elder spirit"
ELDER_WORLD = 2
ELDER_TABLE = "rare spirit makers robe"
ELDER_CHANCE = 0.1
ELDER_NOTE = "jibrish 2026-11-17: the Tower of Phantoms elder spirit only, 0.1%"
ELDER_SOURCE = "workboard 1599"
STOPGAP_RATE = dict(preset="jib", profile="xp", category="spirit makers robe", rate=0.001, source="workboard 1598")
STOPGAP_POOL = dict(id=12158, category="spirit makers robe", npc_name=ELDER_NAME, patternid=ROBE, source="workboard 1598")
OLD_BINDING = dict(npc_name=ELDER_NAME, tablename=ELDER_TABLE, chance=10, rolls=1, source="robes5117")


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def cols(db, tbl):
    return [r[1] for r in db.execute(f"PRAGMA table_info({tbl})")]


def one(db, sql, args=()):
    cur = db.execute(sql, args)
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, tbl, key, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key),
                      None if before is None else go_json(before), None if after is None else go_json(after), detail))
    return cur.lastrowid


class Log:
    """A part's log: each write's table, key (a WHERE as JSON), the row before and after (JSON, null for none)."""

    def __init__(self, db, name, detail):
        self.db, self.name, self.detail = db, name, detail

    def exists(self):
        return table(self.db, self.name)

    def create(self):
        self.db.execute(f"""CREATE TABLE IF NOT EXISTS {self.name} (
            seq INTEGER PRIMARY KEY AUTOINCREMENT, tbl TEXT NOT NULL, key TEXT NOT NULL,
            before TEXT, after TEXT, audit INTEGER NOT NULL DEFAULT 0)""")

    def row(self, tbl, key):
        where = " AND ".join(f"{k} = ?" for k in key)
        return one(self.db, f"SELECT * FROM {tbl} WHERE {where}", list(key.values()))

    def write(self, action, tbl, key, set_=None, row=None):
        """action insert (row), update (set_), delete; logged and audited."""
        before = None if action == "insert" else self.row(tbl, key)
        where = " AND ".join(f"{k} = ?" for k in key)
        if action == "insert":
            c = list(row)
            self.db.execute(f"INSERT INTO {tbl} ({', '.join(c)}) VALUES ({', '.join('?' * len(c))})", [row[k] for k in c])
            if "itemid" in key and key["itemid"] is None:
                key = {"itemid": self.db.execute("SELECT last_insert_rowid()").fetchone()[0]}
        elif action == "update":
            n = self.db.execute(f"UPDATE {tbl} SET {', '.join(f'{k} = ?' for k in set_)} WHERE {where}",
                                list(set_.values()) + list(key.values())).rowcount
            assert n == 1, (tbl, key, n)
        else:
            n = self.db.execute(f"DELETE FROM {tbl} WHERE {where}", list(key.values())).rowcount
            assert n == 1, (tbl, key, n)
        after = None if action == "delete" else self.row(tbl, key)
        a = audit(self.db, action, tbl, key, before, after, self.detail)
        self.db.execute(f"INSERT INTO {self.name} (tbl, key, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                        (tbl, go_json(key), None if before is None else go_json(before),
                         None if after is None else go_json(after), a))
        return key

    def undo(self):
        """(back, left): every write taken back, the last first, only where the row is as written."""
        if not self.exists():
            return 0, 0
        back = left = 0
        for seq, tbl, key, before, after in self.db.execute(
                f"SELECT seq, tbl, key, before, after FROM {self.name} ORDER BY seq DESC").fetchall():
            k = json.loads(key)
            now = self.row(tbl, k)
            if (now is None and after is not None) or (now is not None and (after is None or go_json(now) != after)):
                left += 1
                print(f"  left: {tbl} {k}, changed since it was written: {now}")
                continue
            where = " AND ".join(f"{c} = ?" for c in k)
            if after is not None:
                self.db.execute(f"DELETE FROM {tbl} WHERE {where}", list(k.values()))
            if before is not None:
                b = json.loads(before)
                c = list(b)
                self.db.execute(f"INSERT INTO {tbl} ({', '.join(c)}) VALUES ({', '.join('?' * len(c))})", [b[x] for x in c])
            audit(self.db, "undo", tbl, k, now, None if before is None else json.loads(before), "undo: " + self.detail)
            self.db.execute(f"DELETE FROM {self.name} WHERE seq = ?", (seq,))
            back += 1
        if left == 0:
            self.db.execute(f"DROP TABLE {self.name}")
        return back, left


# --- robe -------------------------------------------------------------------------------------------------------------

def robe_plan(db):
    """(writes [(patternid, name, was, now)], refusals, applied)."""
    rec = one(db, "SELECT patternid, itemname, classuse FROM itempattern WHERE patternid = ?", (ROBE,))
    if rec is None or rec["itemname"] != ROBE_NAME:
        return [], [f"itempattern {ROBE} is {rec!r}, not {ROBE_NAME!r}"], False
    ids = [ROBE]
    if table(db, "itemimported"):
        ids += [r[0] for r in db.execute("SELECT patternid FROM itemimported WHERE base = ? ORDER BY patternid", (ROBE,))]
    rows = [one(db, "SELECT patternid, itemname, classuse FROM itempattern WHERE patternid = ?", (i,)) for i in ids]
    if all(r and r["classuse"] & WARRIOR for r in rows):
        return [], [], True
    if rec["classuse"] != ROBE_WAS:
        return [], [f"itempattern {ROBE}'s classuse is {rec['classuse']}, not {ROBE_WAS}: STOP and tell jibrish (1567)"], False
    writes = [(r["patternid"], r["itemname"], r["classuse"], r["classuse"] | WARRIOR) for r in rows if r and not r["classuse"] & WARRIOR]
    return writes, [], False


def robe(db, a):
    log = Log(db, "jib1117_robe_log", DETAIL.format(1567, "robe"))
    if a.undo:
        return undo(db, log)
    writes, refusals, applied = robe_plan(db)
    for pid, name, was, now in writes:
        print(f"itempattern {pid} {name!r}: classuse {was} -> {now} (Warrior)")
    return finish(db, a, log, applied, refusals, len(writes),
                  lambda: [log.write("update", "itempattern", {"patternid": pid}, {"classuse": now}) for pid, _, _, now in writes])


# --- testers ----------------------------------------------------------------------------------------------------------

def tester_rows(db, t):
    src = one(db, "SELECT * FROM itempattern WHERE patternid = ?", (TESTER_PRO,))
    rec = dict(src)
    rec.update(patternid=t["patternid"], itemname=t["name"], itemdesc=t["desc"])
    src_eff = one(db, "SELECT * FROM itemeffect WHERE patternid = ?", (TESTER_PRO,))
    eff = {k: v for k, v in src_eff.items()}
    eff.update(patternid=t["patternid"], charges=CHARGES, cooldown=0.0, breaks="", wear=0, area=0.0, body=0,
               target_line="", source=f"gm (work board {t['entry']}, jibrish 2026-11-16)", **t["effect"])
    return rec, eff


def testers_plan(db):
    refusals = []
    src = one(db, "SELECT * FROM itempattern WHERE patternid = ?", (TESTER_PRO,))
    if src is None or src["itemname"] != "Tester Pro":
        return [], [f"itempattern {TESTER_PRO} is not Tester Pro (the record they are cloned from)"], False
    want = {"trade": 1, "lore": 1, "procanim": 7, "equipslot": -1, "maxstack": 1, "classuse": 32767, "raceuse": 32767,
            "itemicon": ICON_FROM[2], "itemcost": 0, "duration": 80}
    for k, v in want.items():
        if src.get(k) != v:
            refusals.append(f"Tester Pro's {k} is {src.get(k)!r}, not {v!r}")
    if one(db, "SELECT 1 AS x FROM itemeffect WHERE patternid = ?", (TESTER_PRO,)) is None:
        refusals.append("Tester Pro has no itemeffect row")
    for sid, name, icon in (SOW, SUPINVIS):
        got = db.execute("SELECT spellname, icon FROM spellpattern WHERE spellid = ?", (sid,)).fetchone()
        if got != (name, icon):
            refusals.append(f"spellpattern {sid} is {got!r}, not {(name, icon)!r}")
    if refusals:
        return [], refusals, False
    todo, done = [], 0
    for t in TESTERS:
        rec, eff = tester_rows(db, t)
        now_rec = one(db, "SELECT * FROM itempattern WHERE patternid = ?", (t["patternid"],))
        now_eff = one(db, "SELECT * FROM itemeffect WHERE patternid = ?", (t["patternid"],))
        if now_rec == rec and now_eff == eff:
            done += 1
            continue
        if now_rec is not None:
            refusals.append(f"itempattern {t['patternid']} is taken: {now_rec['itemname']!r}")
        if now_eff is not None:
            refusals.append(f"itemeffect {t['patternid']} is taken: {now_eff['line']!r}")
        for pid, in db.execute("SELECT patternid FROM itempattern WHERE itemname = ? COLLATE NOCASE", (t["name"],)):
            if pid != t["patternid"]:
                refusals.append(f"itempattern {pid} is named {t['name']!r} already")
        todo.append((t, rec, eff))
    return todo, refusals, done == len(TESTERS)


def testers(db, a):
    log = Log(db, "jib1117_testers_log", DETAIL.format("1571, 1572", "testers"))
    give = Log(db, "jib1117_testers_give_log", DETAIL.format("1571, 1572", "testers --give"))
    if a.undo:
        r = undo(db, give)
        return r or undo(db, log)
    todo, refusals, applied = testers_plan(db)
    for t, rec, eff in todo:
        print(f"itempattern {t['patternid']} {t['name']!r} (Tester Pro's record, NO TRADE, LORE, gem); "
              f"itemeffect {eff['line']!r} as {eff['name']!r}, icon {eff['icon']}, {CHARGES} charges")
    if a.give:
        return give_testers(db, give, applied)
    return finish(db, a, log, applied, refusals, 2 * len(todo),
                  lambda: [(log.write("insert", "itempattern", {"patternid": t["patternid"]}, row=rec),
                            log.write("insert", "itemeffect", {"patternid": t["patternid"]}, row=eff)) for t, rec, eff in todo])


def give_testers(db, give, applied):
    if not applied:
        print("refused: --apply the records first")
        return 1
    ch = db.execute("SELECT serverid FROM characters WHERE charname = ? COLLATE NOCASE", (CHAR,)).fetchall()
    if len(ch) != 1:
        print(f"refused: {len(ch)} characters named {CHAR} here (the other shard?)")
        return 1
    sid = ch[0][0]
    held = [t["name"] for t in TESTERS
            if db.execute("SELECT 1 FROM charinventory WHERE serverid = ? AND patternid = ?", (sid, t["patternid"])).fetchone()]
    if held:
        print(f"given already: {CHAR} holds {held}; nothing done (check remaincharge reads {CHARGES})")
        for t in TESTERS:
            for r in db.execute("SELECT itemid, remaincharge, location FROM charinventory WHERE serverid = ? AND patternid = ?",
                                (sid, t["patternid"])):
                print(f"  {t['name']}: charinventory {r[0]}, remaincharge {r[1]}, location {r[2]}")
        return 0
    db.execute("BEGIN IMMEDIATE")
    try:
        give.create()
        for t in TESTERS:
            n = db.execute("SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory WHERE serverid = ? AND location = 0",
                           (sid,)).fetchone()[0]
            k = give.write("insert", "charinventory", {"itemid": None},
                           row=dict(serverid=sid, stackleft=1, remainHP=100, remaincharge=CHARGES, patternid=t["patternid"],
                                    equiploc=-1, location=0, listnumber=n))
            print(f"give: {t['name']} into {CHAR}'s pack (serverid {sid}), charinventory {k['itemid']}, {CHARGES} charges")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print("he sees them at his next login (eject him to see them now)")
    return 0


# --- elder ------------------------------------------------------------------------------------------------------------

def elder_plan(db):
    """(steps [(action, tbl, key, row)], refusals, applied)."""
    refusals = []
    for t in ("npcloot", "loottables", "npcs", "gm_audit"):
        if not table(db, t):
            return [], [f"no {t} table here"], False
    for rid in ELDER_ROWS:
        r = one(db, "SELECT id, npc_name, world FROM npcs WHERE id = ?", (rid,))
        if r is None or r["npc_name"].strip().lower() != ELDER_NAME or r["world"] != ELDER_WORLD:
            refusals.append(f"npcs {rid} is {r!r}, not an {ELDER_NAME} of world {ELDER_WORLD}")
    if one(db, "SELECT 1 AS x FROM loottables WHERE tablename = ? AND patternid = ?", (ELDER_TABLE, ROBE)) is None:
        refusals.append(f"loot table {ELDER_TABLE!r} does not give {ROBE}")
    steps = []
    for rid in ELDER_ROWS:
        key = {"npc_name": f"#{rid}", "tablename": ELDER_TABLE}
        row = dict(key, chance=ELDER_CHANCE, rolls=1, note=ELDER_NOTE, source=ELDER_SOURCE)
        now = one(db, "SELECT * FROM npcloot WHERE npc_name = ? AND tablename = ?", (key["npc_name"], ELDER_TABLE))
        if now is None:
            steps.append(("insert", "npcloot", key, row))
        elif {k: now[k] for k in row} != row:
            refusals.append(f"npcloot {key} is there already, as {now}")
    if table(db, "lootrate"):
        r = one(db, "SELECT * FROM lootrate WHERE preset = ? AND profile = ? AND category = ?",
                (STOPGAP_RATE["preset"], STOPGAP_RATE["profile"], STOPGAP_RATE["category"]))
        if r is not None:
            if any(r[k] != v for k, v in STOPGAP_RATE.items()):
                refusals.append(f"lootrate {r} is not 1598's stopgap as written")
            else:
                steps.append(("delete", "lootrate", {k: STOPGAP_RATE[k] for k in ("preset", "profile", "category")}, None))
    if table(db, "lootpool"):
        r = one(db, "SELECT * FROM lootpool WHERE id = ?", (STOPGAP_POOL["id"],))
        if r is not None:
            if any(r[k] != v for k, v in STOPGAP_POOL.items()):
                refusals.append(f"lootpool {STOPGAP_POOL['id']} is {r}, not 1598's stopgap as written")
            else:
                steps.append(("delete", "lootpool", {"id": STOPGAP_POOL["id"]}, None))
        others = db.execute("SELECT id, npc_name FROM lootpool WHERE patternid = ? AND id <> ?", (ROBE, STOPGAP_POOL["id"])).fetchall()
        if others:
            refusals.append(f"other lootpool rows give {ROBE}: {others} (look before going on)")
    r = one(db, "SELECT * FROM npcloot WHERE npc_name = ? AND tablename = ?", (ELDER_NAME, ELDER_TABLE))
    if r is not None:
        if any(r[k] != v for k, v in OLD_BINDING.items()):
            refusals.append(f"npcloot {r} is not robes5117's binding as written")
        else:
            steps.append(("delete", "npcloot", {"npc_name": ELDER_NAME, "tablename": ELDER_TABLE}, None))
    others = db.execute("SELECT npc_name FROM npcloot WHERE tablename = ? AND npc_name NOT IN (?, ?, ?)",
                        (ELDER_TABLE, ELDER_NAME, *(f"#{r}" for r in ELDER_ROWS))).fetchall()
    if others:
        refusals.append(f"{ELDER_TABLE!r} is bound to others too: {others}")
    return steps, refusals, not steps and not refusals


def elder(db, a):
    log = Log(db, "jib1117_elder_log", DETAIL.format(1599, "elder"))
    if a.undo:
        return undo(db, log)
    steps, refusals, applied = elder_plan(db)
    for action, tbl, key, row in steps:
        print(f"{action} {tbl} {key}" + (f": chance {row['chance']}, rolls {row['rolls']}" if row else ""))
    return finish(db, a, log, applied, refusals, len(steps),
                  lambda: [log.write(action, tbl, key, row=row) for action, tbl, key, row in steps])


# --- spectre --------------------------------------------------------------------------------------------------------

SPECTRE_TABLE = "rare spectre robe"
SPECTRE_ROBE = 5123
SPECTRE_WAS = (1, 3)
SPECTRE_CHANCE = 0.1


def spectre_plan(db):
    if one(db, "SELECT 1 AS x FROM loottables WHERE tablename = ? AND patternid = ?", (SPECTRE_TABLE, SPECTRE_ROBE)) is None:
        return [], [f"loot table {SPECTRE_TABLE!r} does not give {SPECTRE_ROBE}"], False
    rows = db.execute("SELECT npc_name, chance FROM npcloot WHERE tablename = ? ORDER BY npc_name", (SPECTRE_TABLE,)).fetchall()
    if not rows:
        return [], [f"nothing is bound to {SPECTRE_TABLE!r}"], False
    todo, refusals = [], []
    for name, chance in rows:
        if chance == SPECTRE_CHANCE:
            continue
        if chance not in SPECTRE_WAS:
            refusals.append(f"npcloot ({name!r}, {SPECTRE_TABLE!r}) is at {chance}, not 1 or 3: look first")
        todo.append((name, chance))
    return todo, refusals, not todo


def spectre(db, a):
    log = Log(db, "jib1117_spectre_log", DETAIL.format("spectre robe, the owner", "spectre"))
    if a.undo:
        return undo(db, log)
    todo, refusals, applied = spectre_plan(db)
    for name, chance in todo:
        print(f"npcloot ({name!r}, {SPECTRE_TABLE!r}): chance {chance} -> {SPECTRE_CHANCE}")
    return finish(db, a, log, applied, refusals, len(todo),
                  lambda: [log.write("update", "npcloot", {"npc_name": name, "tablename": SPECTRE_TABLE},
                                     {"chance": SPECTRE_CHANCE}) for name, _ in todo])


# --- common -----------------------------------------------------------------------------------------------------------

def undo(db, log):
    db.execute("BEGIN IMMEDIATE")
    try:
        back, left = log.undo()
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"{log.name}: put back {back} writes, {left} left (changed since)" if back or left else f"{log.name}: nothing to undo")
    return 0 if left == 0 else 1


def finish(db, a, log, applied, refusals, n, write):
    if applied:
        print("applied already: nothing done")
        return 0
    if refusals:
        print("REFUSED, nothing done:")
        for r in refusals:
            print("  " + r)
        return 1
    if log.exists():
        print(f"{log.name} is there but the rows are not as written: --undo first")
        return 1
    if not a.apply:
        print(f"report only: {n} writes (--apply writes them)")
        return 0
    db.execute("BEGIN IMMEDIATE")
    try:
        log.create()
        write()
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"applied: {n} writes, in gm_audit and {log.name}")
    return 0


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("part", choices=["robe", "testers", "elder", "spectre"])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    g.add_argument("--give", action="store_true", help="testers: one of each into Innoruuk's pack, 999 charges")
    a = ap.parse_args()
    if a.give and a.part != "testers":
        ap.error("--give is the testers'")
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        return {"robe": robe, "testers": testers, "elder": elder, "spectre": spectre}[a.part](db, a)
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
