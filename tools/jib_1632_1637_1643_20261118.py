#!/usr/bin/env python3
"""a GM's work board entries 1632 and 1637 (2026-11-18), the data half: owner-approved as submitted. Each part its
own subcommand, log and --undo. Entry 1643's table (fxland) is made and seeded by the server itself
(internal/database/fxland.go), so it has no part here.

Both parts need the server with the entries' code (branch jib-1632-1637-1643) RUNNING on the shard first: each refuses
a database whose classtemplates has no special_fixed column (that server adds it at its start), which is how this tool
tells the code is there. On an older server either write would undo a creature's template: a special_damage of 0 with
a special_every, and a resist on a heal, are refused there and the whole row left out.

croc (1632 §5-6; both shards): Crocosaurus's crit as a flat 850 at any level, and his rows mirrored to the shard that
    lacks them (the entry's title: "mirror his rows to the other shard"). The wanted rows are Heirloom's (work 1629,
    1630) with §5's change; a row already as wanted is left, one missing is made, one as the other shard had it before
    is changed, anything else REFUSED:
      - classtemplates crocosaurus: special_fixed 850, special_damage 0 (§5); made as Heirloom's where missing (Live);
      - npctemplate crocosaurus -> crocosaurus (Live: roamer, its seed);
      - npcs Crocosaurus (world 0, level 55): size 12.15 (§6; Live 1.0);
      - loottables "named crocosaurus": patternid 1183 (Ring of Garthis), weight 1, levels 1-70 (§6);
      - npcloot crocosaurus -> "named crocosaurus", 100%, 1 roll (Heirloom's binding: without it the table drops nothing).
    The note of classtemplates crocosaurus still says "15.4545 x level 55" (the figure it was); left as a GM wrote it.

nagafen (1637 §2; HEIRLOOM only): Lord Nagafen's Lay Hands gives Dragonfire 10% x 6,080 FR for the fight. REFUSED unless
    his abilities are exactly the entry's "now" (set 2026-11-17, entry 1636): "If the row differs from 'now', stop and
    ask a GM". His template's own proc (proc_chance 5, proc_damage 6080, proc_resist FR) is left as it is; REFUSED if
    it is not that. Check after: #stats on Lord Nagafen lists Lay Hands "then Dragonfire for the fight: 10% of its melee
    hits 6080 more, cut by FR".

Each: a report by default; --apply writes in one transaction, a gm_audit line a row and a log row; a second run finds
nothing to do; --undo puts back exactly what was there (only rows as written; a row changed since is left and said).
No restart: a tool's write is not read by the running world until the GM panel's reload_npcs op (the class templates
and npctemplate read again, Crocosaurus's changed row remade at its new size) or the next restart; loot is read at
each kill.

    python3 -E -s tools/jib_1632_1637_1643_20261118.py croc    --db eqoa.db [--apply | --undo]
    python3 -E -s tools/jib_1632_1637_1643_20261118.py nagafen --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

WHO, VIA = "dab", "tool"
DETAIL = "jibrish's work board {} (2026-11-18): tools/jib_1632_1637_1643_20261118.py {}"

# --- croc -------------------------------------------------------------------------------------------------------------
CROC = "crocosaurus"
CROC_NPC = dict(npc_name="Crocosaurus", world=0, npc_level=55)
CROC_SIZE, CROC_SIZE_WAS = 12.15, 1.0
CROC_FIXED = 850
# Heirloom's row (work 1630) as it stood on 2026-11-18, the mirror's source; §5's change on top of it
CROC_TEMPLATE = dict(
    name=CROC, **{"class": 0}, hp_pct=1229, taken_pct=100, melee_pct=100, special="Critical Hit", special_chance=0,
    special_damage=15.4545454545455, special_fx=0, special_every=10.0, special_first=10.0, special_jitter=0.0,
    special_from=10, shield_damage=0, shield_resist="", proc_chance=0, proc_damage=0, proc_name="", proc_resist="",
    resist_fail=0.0, resist_kind="", resist_pct=0, kick_damage=0, kick_every=0.0, kick_first=0.0, fight_regen=0,
    run_speed=0.0, taken_resist="", melee_fixed=0, special_fixed=0, abilities="[]",
    note="work 1630 (jibrish 2026-11-18: \"150k hp, and crit for 850 every 10 seconds\"): 150,060 hit points (1229% of "
         "his level 55's 12,210) and a Critical Hit of 850 (15.4545 x level 55) every 10 s; the roamer's all else. No "
         "film or era figure; jibrish's numbers.",
    source="work 1630")
CROC_CHANGE = dict(special_fixed=CROC_FIXED, special_damage=0)
CROC_NPCTEMPLATE = dict(name=CROC, template=CROC, note="named, level 55; own template (work 1630)", source="work 1630")
CROC_NPCTEMPLATE_WAS = dict(template="roamer")
CROC_TABLE = "named crocosaurus"
CROC_LOOT = dict(tablename=CROC_TABLE, patternid=1183, weight=1, minlevel=1, maxlevel=70, source="work 1629 crocosaurus ring")
CROC_RING = "Ring of Garthis"
CROC_BINDING = dict(npc_name=CROC, tablename=CROC_TABLE, chance=100, rolls=1,
                    note="Cryptic Souls pos.htm (2008): 'obtain the Ring of Garthis by killing Crocosaurus'; the ring's "
                         "text names him. 100% ours, as the other named drops. jibrish 2026-11-18 (work 1629).",
                    source="work 1629 crocosaurus ring")

# --- nagafen ----------------------------------------------------------------------------------------------------------
NAGAFEN = "lord nagafen"
NAGAFEN_NOW = ('[{"kind":"aoe","spell_id":906201,"every":24,"first":12},{"kind":"heal","heal_pct":100,"below":60,'
               '"every":6,"cooldown":600,"name":"Lay Hands"},{"kind":"debuff","spell_id":906202,"every":30,"no_cure":true},'
               '{"kind":"rampage","chance":20}]')
NAGAFEN_NEW = ('[{"kind":"aoe","spell_id":906201,"every":24,"first":12},{"kind":"heal","heal_pct":100,"below":60,'
               '"every":6,"cooldown":600,"name":"Lay Hands","proc_chance":10,"proc_damage":6080,"proc_name":"Dragonfire",'
               '"resist":"FR"},{"kind":"debuff","spell_id":906202,"every":30,"no_cure":true},{"kind":"rampage","chance":20}]')
NAGAFEN_PROC = dict(proc_chance=5, proc_damage=6080, proc_resist="FR")


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
        where = " AND ".join(f'"{k}" = ?' for k in key)
        return one(self.db, f"SELECT * FROM {tbl} WHERE {where}", list(key.values()))

    def write(self, action, tbl, key, set_=None, row=None):
        """action insert (row; key None: the new rowid as id), update (set_); logged and audited."""
        before = None if action == "insert" else self.row(tbl, key)
        if action == "insert":
            c = list(row)
            cur = self.db.execute(f"INSERT INTO {tbl} ({', '.join(chr(34) + x + chr(34) for x in c)}) "
                                  f"VALUES ({', '.join('?' * len(c))})", [row[k] for k in c])
            if key is None:
                key = {"id": cur.lastrowid}
        else:
            where = " AND ".join(f'"{k}" = ?' for k in key)
            n = self.db.execute(f"UPDATE {tbl} SET {', '.join(f'{chr(34)}{k}{chr(34)} = ?' for k in set_)} WHERE {where}",
                                list(set_.values()) + list(key.values())).rowcount
            assert n == 1, (tbl, key, n)
        after = self.row(tbl, key)
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
            where = " AND ".join(f'"{c}" = ?' for c in k)
            if after is not None:
                self.db.execute(f"DELETE FROM {tbl} WHERE {where}", list(k.values()))
            if before is not None:
                b = json.loads(before)
                c = list(b)
                self.db.execute(f"INSERT INTO {tbl} ({', '.join(chr(34) + x + chr(34) for x in c)}) "
                                f"VALUES ({', '.join('?' * len(c))})", [b[x] for x in c])
            audit(self.db, "undo", tbl, k, now, None if before is None else json.loads(before), "undo: " + self.detail)
            self.db.execute(f"DELETE FROM {self.name} WHERE seq = ?", (seq,))
            back += 1
        if left == 0:
            self.db.execute(f"DROP TABLE {self.name}")
        return back, left


def code_there(db):
    """The refusal when the entries' server has not run on this database, else None."""
    if "special_fixed" not in cols(db, "classtemplates"):
        return ("classtemplates has no special_fixed column: the server with work board 1632/1637's code has not run "
                "on this database yet (deploy it first)")
    return None


def differs(row, want):
    return {k: (row.get(k), v) for k, v in want.items() if row.get(k) != v}


# --- croc -------------------------------------------------------------------------------------------------------------

def croc_plan(db):
    """(steps [(action, tbl, key, set_or_row, words)], refusals, applied)."""
    for t in ("classtemplates", "npctemplate", "npcs", "loottables", "npcloot", "itempattern", "gm_audit"):
        if not table(db, t):
            return [], [f"no {t} table here"], False
    why = code_there(db)
    if why:
        return [], [why], False
    steps, refusals = [], []
    want = dict(CROC_TEMPLATE, **CROC_CHANGE)
    t = one(db, "SELECT * FROM classtemplates WHERE name = ?", (CROC,))
    if t is None:
        steps.append(("insert", "classtemplates", {"name": CROC}, want,
                      "classtemplates crocosaurus made as Heirloom's (work 1630), special_fixed 850, special_damage 0"))
    elif differs(t, want):
        was = dict(want, special_fixed=0, special_damage=CROC_TEMPLATE["special_damage"])
        d = differs(t, was)
        if set(d) - {"special_damage"} or abs(t["special_damage"] - CROC_TEMPLATE["special_damage"]) > 1e-9:
            refusals.append(f"classtemplates crocosaurus is not work 1630's row as written: {d}")
        else:
            steps.append(("update", "classtemplates", {"name": CROC}, dict(CROC_CHANGE),
                          f"classtemplates crocosaurus: special_damage {t['special_damage']} -> 0, special_fixed 0 -> {CROC_FIXED}"))
    nt = one(db, "SELECT * FROM npctemplate WHERE name = ?", (CROC,))
    if nt is None:
        steps.append(("insert", "npctemplate", {"name": CROC}, dict(CROC_NPCTEMPLATE), "npctemplate crocosaurus -> crocosaurus"))
    elif differs(nt, CROC_NPCTEMPLATE):
        if nt["template"] != CROC_NPCTEMPLATE_WAS["template"]:
            refusals.append(f"npctemplate crocosaurus is {nt}, not the roamer seed nor work 1630's")
        else:
            steps.append(("update", "npctemplate", {"name": CROC}, {k: v for k, v in CROC_NPCTEMPLATE.items() if k != "name"},
                          f"npctemplate crocosaurus: {nt['template']} -> crocosaurus"))
    npcs = db.execute("SELECT id, size FROM npcs WHERE npc_name = ? AND world = ? AND npc_level = ?",
                      (CROC_NPC["npc_name"], CROC_NPC["world"], CROC_NPC["npc_level"])).fetchall()
    if len(npcs) != 1:
        refusals.append(f"{len(npcs)} npcs rows are {CROC_NPC}, want one")
    else:
        nid, size = npcs[0]
        if abs(size - CROC_SIZE) > 1e-9:
            if abs(size - CROC_SIZE_WAS) > 1e-9:
                refusals.append(f"npcs {nid} Crocosaurus's size is {size}, not {CROC_SIZE_WAS} nor {CROC_SIZE}: look first")
            else:
                steps.append(("update", "npcs", {"id": nid}, {"size": CROC_SIZE}, f"npcs {nid} Crocosaurus: size {size} -> {CROC_SIZE}"))
    ring = one(db, "SELECT itemname FROM itempattern WHERE patternid = ?", (CROC_LOOT["patternid"],))
    if ring is None or ring["itemname"] != CROC_RING:
        refusals.append(f"itempattern {CROC_LOOT['patternid']} is {ring}, not {CROC_RING!r}")
    rows = [dict(zip(("id",) + tuple(CROC_LOOT), r)) for r in db.execute(
        f"SELECT id, {', '.join(CROC_LOOT)} FROM loottables WHERE tablename = ?", (CROC_TABLE,))]
    if not rows:
        steps.append(("insert", "loottables", None, dict(CROC_LOOT),
                      f"loottables {CROC_TABLE!r}: patternid {CROC_LOOT['patternid']} ({CROC_RING}), weight 1, levels 1-70"))
    elif len(rows) != 1 or differs(rows[0], CROC_LOOT):
        refusals.append(f"loottables {CROC_TABLE!r} is there already, not as work 1629's: {rows}")
    b = one(db, "SELECT * FROM npcloot WHERE npc_name = ? AND tablename = ?", (CROC, CROC_TABLE))
    if b is None:
        steps.append(("insert", "npcloot", {"npc_name": CROC, "tablename": CROC_TABLE}, dict(CROC_BINDING),
                      f"npcloot crocosaurus -> {CROC_TABLE!r}, 100%, 1 roll"))
    elif differs(b, CROC_BINDING):
        refusals.append(f"npcloot crocosaurus -> {CROC_TABLE!r} is {b}, not work 1629's")
    others = db.execute("SELECT npc_name FROM npcloot WHERE tablename = ? AND npc_name <> ?", (CROC_TABLE, CROC)).fetchall()
    if others:
        refusals.append(f"{CROC_TABLE!r} is bound to others too: {others}")
    return steps, refusals, not steps and not refusals


def croc(db, a):
    log = Log(db, "jib1632_croc_log", DETAIL.format(1632, "croc"))
    if a.undo:
        return undo(db, log)
    steps, refusals, applied = croc_plan(db)
    for *_, words in steps:
        print(words)
    return finish(db, a, log, applied, refusals, len(steps),
                  lambda: [log.write(action, tbl, key, set_=v if action == "update" else None, row=v if action == "insert" else None)
                           for action, tbl, key, v, _ in steps])


# --- nagafen ----------------------------------------------------------------------------------------------------------

def nagafen_plan(db):
    if not table(db, "classtemplates") or not table(db, "gm_audit"):
        return [], ["no classtemplates or gm_audit table here"], False
    why = code_there(db)
    if why:
        return [], [why], False
    t = one(db, "SELECT * FROM classtemplates WHERE name = ?", (NAGAFEN,))
    if t is None:
        return [], [f"no classtemplates {NAGAFEN!r} here"], False
    refusals = []
    if differs(t, NAGAFEN_PROC):
        refusals.append(f"his template's own proc is not 5% x 6080 FR: {differs(t, NAGAFEN_PROC)}")
    if t["abilities"] == NAGAFEN_NEW:
        return [], refusals, not refusals
    if t["abilities"] != NAGAFEN_NOW:
        refusals.append(f"his abilities differ from the entry's 'now': STOP and ask jibrish (1637): {t['abilities']}")
    return [t["abilities"]], refusals, False


def nagafen(db, a):
    log = Log(db, "jib1637_nagafen_log", DETAIL.format(1637, "nagafen"))
    if a.undo:
        return undo(db, log)
    steps, refusals, applied = nagafen_plan(db)
    for was in steps:
        print(f"classtemplates {NAGAFEN!r}: Lay Hands gives Dragonfire for the fight, 10% x 6080, cut by FR\n  was {was}\n  now {NAGAFEN_NEW}")
    return finish(db, a, log, applied, refusals, len(steps),
                  lambda: [log.write("update", "classtemplates", {"name": NAGAFEN}, set_={"abilities": NAGAFEN_NEW}) for _ in steps])


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
    ap.add_argument("part", choices=["croc", "nagafen"])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        return {"croc": croc, "nagafen": nagafen}[a.part](db, a)
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
