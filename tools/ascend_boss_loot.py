#!/usr/bin/env python3
"""Ascended loot for the tuned raid bosses (petition 1446, the GM Jibrish, 2026-11-10: "For all tuned raid bosses,
and also when we tune new ones going forward, they should drop "Ascended" versions of their regular gear in place of
their normal loot table. These should be the regular items except with a flat +10% to all of their stats. The tuned
raid bosses should no longer drop their regular drops outside of spells, only the Ascended versions. These are NOT
no rent."). Run it for each boss a GM tunes, on each shard's database.

For each boss named (--boss, as npcs spells it, any case; --tuned: TUNED, the bosses tuned so far), found by NAME,
never by id (the two shards' ids differ):

  - Its gear in what its corpse rolls of its own (handlers/loot.go) is pointed at the gear's Ascended record: the
    rows of its loot tables (npcloot -> loottables), its category pool rows of its own (lootpool, npc_name its name)
    and its spawns' own items (npcitem). Gear is what database/ascended.go makes an Ascended record of: a record worn
    in the gear's slots 1-19 (armour, jewellery, weapons, a bow, a held item, gloves), not a stack, not a thrown
    potion (attack type 10), not an Heirloom, not a copy (an import's or an Ascended one). A spell scroll (wear slot
    -1 with the scroll's icon) and everything else (a quest item, a gem, a component) stays as it is.
  - An Ascended record is made where the gear has none, in the server's own SQL (database/ascended.go;
    TestAscendBossLootToolKeepsTheRule holds this file's constants to it): "Ascended <name>"; each stat, AC, resist,
    hit point and power maximum and tick and a weapon's damage above 0 a tenth more (times 110 / 100 in whole
    numbers, half up: 4 stays 4, 5 is 6, 15 is 17; 0 or a penalty as it is), its use effect's "by N" so; its
    durability (maxhp), price (itemcost), look, icon, slot, level, classes, races, NO TRADE and Lore its base's; never
    NO RENT (rent 0 is made 1). The server makes each its base again at every start (fix the base, never a copy).
  - A table another creature rolls too (bound to a name not made Ascended, by this run or an earlier one, or to "*")
    is not changed: the boss is bound instead to a copy of it, "<table> ascended", its rows the table's with the gear
    Ascended (one copy a table: a later boss sharing it is bound to the same copy, and the copy follows the table).
  - The quests whose show or give steps take that gear are listed with the gear they reward: handed in Ascended, the
    server gives the reward's Ascended record (handlers/questascended.go), made here so it can be read before anyone
    earns one; a loot step on it counts the Ascended record for it.

Reported and not changed: every creature's tables ("*") and the loot categories' shared pools, which a tuned boss
rolls as any creature of its region and profile does; a table of Ascended gear a creature not made Ascended rolls.

Every row written is a row edit as the GM panel writes one (gm_audit, via "tool": listed and revertible there) and is
logged in ascended_loot_log by run for --undo, which puts back each row still as written (one changed since is left,
and said) and takes away each Ascended record nothing holds or names any more (one a character holds stays, and is
said); --undo --run R undoes one run alone. A second run with nothing new to do writes nothing. Report only by
default; --apply and --undo copy the database first (<db>.bak-<when>-pre-ascended) unless --no-backup. The server
reads the loot tables at every corpse, the records at each request and the Ascended list every minute: no restart.

    python3 -E -s tools/ascend_boss_loot.py --db eqoa.db --tuned [--boss "<name>" ...] [--apply | --undo [--run R]]
"""
import argparse
import datetime
import json
import os
import re
import sqlite3
import sys

# database/ascended.go's rule, as the server makes an Ascended record (TestAscendBossLootToolKeepsTheRule)
PREFIX = "Ascended "
PERCENT = 10
FIRST_ID, LAST_ID = 980001, 989999
MAX_NAME = 63
GEAR_SLOTS = (1, 19)
THROWN_POTION = 10
SCALED = ("str", "sta", "agi", "wis", "dex", "cha", "intelligence", "hpmax", "powmax", "pot", "hot", "ac",
          "pr", "dr", "fr", "cr", "lr", "ar", "weapondamage")
FIGURE_SQL = ("CASE WHEN {q} IS NULL THEN NULL WHEN CAST({q} AS INTEGER) > 0 "
              "THEN (CAST({q} AS INTEGER) * {m} + 50) / 100 ELSE {q} END")
NAME_SQL = "'Ascended ' || trim({q})"
RENT_SQL = "CASE WHEN {q} = 0 THEN 1 ELSE {q} END"
SCHEMA = """CREATE TABLE IF NOT EXISTS itemascended (
	patternid INTEGER PRIMARY KEY,
	base      INTEGER NOT NULL UNIQUE,
	made      TEXT NOT NULL DEFAULT '',
	source    TEXT NOT NULL DEFAULT ''
)"""
# database/imported.go's: an import's copy is no base
IMPORTED_SCHEMA = """CREATE TABLE IF NOT EXISTS itemimported (
	patternid INTEGER PRIMARY KEY,
	base      INTEGER NOT NULL UNIQUE,
	made      TEXT NOT NULL DEFAULT '',
	source    TEXT NOT NULL DEFAULT ''
)"""
SCROLL_ART = -607649328  # database.ScrollArt, signed and as the panel once kept it unsigned
SCROLL_ARTS = (SCROLL_ART, SCROLL_ART + (1 << 32))

# The bosses tuned so far: the petition's six (its "urza" is Uzrathixius, "bertoxxulus" Bertoxxulous) and Yhafiran,
# tuned as Uzrathixius and new in the same deploy (his own table, "boss yhafiran"). Add each boss tuned later.
TUNED = ["Lord Nagafen", "Noroxus", "Bertoxxulous", "Uzrathixius", "Poxathixius", "Quintessence of Veeshan", "Yhafiran"]

EVERY = "*"                  # npcloot's every creature (handlers/loot.go lootEveryCreature)
COPY_SUFFIX = " ascended"    # a shared table's copy for the tuned bosses
COPY_SOURCE = "ascended"     # loottables.source of a copy's rows
WHO, VIA = "dab's sessions", "tool"
DETAIL = "petition 1446 (Jibrish): a tuned raid boss drops Ascended gear; tools/ascend_boss_loot.py"
LOG = "ascended_loot_log"
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    run     TEXT NOT NULL,      -- the run (its UTC time)
    boss    TEXT NOT NULL,      -- the boss (npcloot's name) the change is for; '' a record any may share
    tbl     TEXT NOT NULL,      -- itempattern, itemeffect, itemascended, loottables, npcloot, lootpool, npcitem, sqlite_sequence
    ref     TEXT NOT NULL,      -- the row's key (JSON)
    before  TEXT,               -- the row before (JSON); NULL: made here
    after   TEXT,               -- the row as written (JSON); NULL: taken away here
    audit   INTEGER             -- the gm_audit line written
)"""
# database.GMAuditSchema, as the server makes it
AUDIT_SCHEMA = """CREATE TABLE IF NOT EXISTS gm_audit (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	at TEXT NOT NULL, accountid INTEGER NOT NULL, username TEXT NOT NULL,
	via TEXT NOT NULL, ip TEXT NOT NULL DEFAULT '', action TEXT NOT NULL,
	tbl TEXT NOT NULL DEFAULT '', rowkey TEXT NOT NULL DEFAULT '',
	before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER
)"""
KEYS = {"itempattern": ("patternid",), "itemeffect": ("patternid",), "itemascended": ("patternid",),
        "loottables": ("id",), "lootpool": ("id",), "npcitem": ("itemid",), "npcloot": ("npc_name", "tablename")}
SLOT_NAMES = {1: "head", 2: "robe", 3: "ear", 4: "neck", 5: "chest", 6: "forearm", 7: "bracers", 8: "ring", 9: "belt",
              10: "legs", 11: "feet", 12: "primary", 13: "shield", 14: "either hand", 15: "two hands", 16: "bow",
              17: "thrown", 18: "held", 19: "hands"}
FIGURE_NAMES = {"str": "STR", "sta": "STA", "agi": "AGI", "wis": "WIS", "dex": "DEX", "cha": "CHA", "intelligence": "INT",
                "hpmax": "HP", "powmax": "power", "pot": "PoT", "hot": "HoT", "ac": "AC", "pr": "PR", "dr": "DR",
                "fr": "FR", "cr": "CR", "lr": "LR", "ar": "AR", "weapondamage": "damage"}
BY_RE = re.compile(r"\b([bB][yY] )([0-9]+)")  # imported.go's importedBy, (?i)\bby (\d+)


# ---- the rule ------------------------------------------------------------------------------------------------------

def figure(v):
    """database.AscendedFigure: above 0 a tenth more, half up in whole numbers; 0 or below as it is."""
    return v if v <= 0 else (v * (100 + PERCENT) + 50) // 100


def scaled_line(line):
    """An effect line with each "by N" a tenth more (database/ascended.go's ascendedLine)."""
    if not isinstance(line, str):
        return line
    return BY_RE.sub(lambda m: m.group(1) + str(figure(int(m.group(2)))), line)


def npc_key(name):
    """database.NPCClassKey: lower case, trimmed, no trailing $."""
    n = (name or "").strip().lower()
    return n[:-1].strip() if n.endswith("$") else n


def ascended_name(name):
    return PREFIX + (name or "").strip()


def is_scroll(rec):
    return rec.get("equipslot") == -1 and rec.get("itemicon") in SCROLL_ARTS


def is_gear(rec):
    """database.IsAscendableGear."""
    slot, stack, attack = rec.get("equipslot") or 0, rec.get("maxstack") or 0, rec.get("attacktype") or 0
    return (GEAR_SLOTS[0] <= slot <= GEAR_SLOTS[1] and stack <= 1 and attack != THROWN_POTION
            and not (rec.get("itemname") or "").strip().lower().startswith("heirloom "))


def copy_columns(cols):
    """database/ascended.go's ascendedColumns: itempattern's columns but patternid (set) and what an Ascended record
    takes for each from its base (from), and those columns' names."""
    sets, froms, names = [], [], []
    for c in cols:
        low = c.lower()
        if low == "patternid":
            continue
        q = f'"{c}"'
        names.append(c)
        sets.append(q)
        if low in SCALED:
            froms.append(FIGURE_SQL.format(q=q, m=100 + PERCENT))
        elif low == "itemname":
            froms.append(NAME_SQL.format(q=q))
        elif low == "rent":
            froms.append(RENT_SQL.format(q=q))
        else:
            froms.append(q)
    return ", ".join(sets), ", ".join(froms), names


# ---- reading ---------------------------------------------------------------------------------------------------------

def has_table(db, t):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone()[0] > 0


def columns(db, t):
    return [r[1] for r in db.execute(f"PRAGMA table_info({t})")]


def dict_rows(db, sql, args=()):
    cur = db.execute(sql, args)
    names = [d[0] for d in cur.description]
    return [dict(zip(names, r)) for r in cur.fetchall()]


def one(db, sql, args=()):
    r = dict_rows(db, sql, args)
    return r[0] if r else None


def row_of(db, tbl, ref):
    where = " AND ".join(f"{c} = ?" for c in KEYS[tbl])
    return one(db, f"SELECT * FROM {tbl} WHERE {where}", tuple(ref[c] for c in KEYS[tbl]))


def go_json(m):
    """A row as Go's json.Marshal writes a map (keys sorted, <>& escaped), as the panel's audit lines are."""
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def item_counts(s):
    """database.ParseItemCounts: "4866*2, 8373" -> [(4866, 2), (8373, 1)]."""
    out = []
    for f in (s or "").split(","):
        f = f.strip()
        if not f:
            continue
        m = re.match(r"^\s*(-?\d+)\s*(?:[*x×]\s*(\d+))?\s*$", f)
        if m and int(m.group(2) or 1) >= 1:
            out.append((int(m.group(1)), int(m.group(2) or 1)))
    return out


def step_rewards(st):
    """A step's reward items with whom they are for: its own, its classes' (byclass), its choice's options'."""
    out = [(pid, "") for pid, _ in item_counts(st.get("rewarditems"))]
    try:
        for cls, r in sorted((json.loads(st.get("byclass") or "{}") or {}).items(), key=lambda kv: int(kv[0])):
            out += [(pid, f"class {cls}") for pid, _ in item_counts((r or {}).get("items"))]
    except (ValueError, AttributeError):
        pass
    try:
        for i, o in enumerate((json.loads(st.get("choices") or "{}") or {}).get("options") or []):
            out += [(pid, f"option {i + 1}") for pid, _ in item_counts((o or {}).get("items"))]
    except (ValueError, AttributeError):
        pass
    return out


class World:
    """What the plan reads: the records, the copies, the bosses' NPCs and loot."""

    def __init__(self, db):
        self.db = db
        self.cols = columns(db, "itempattern")
        self.sets, self.froms, self.names = copy_columns(self.cols)
        self.recs = {}
        self.ascended = {}  # copy -> base
        self.imported = set()
        if has_table(db, "itemascended"):
            self.ascended = dict(db.execute("SELECT patternid, base FROM itemascended").fetchall())
        if has_table(db, "itemimported"):
            self.imported = {r[0] for r in db.execute("SELECT patternid FROM itemimported")}
        self.copy_of = {b: c for c, b in self.ascended.items()}

    def rec(self, pid):
        if pid not in self.recs:
            self.recs[pid] = one(self.db, "SELECT * FROM itempattern WHERE patternid = ?", (pid,))
        return self.recs[pid]

    def kind(self, pid):
        """gear (to be Ascended), ascended (an Ascended record), copy (an import's), spell, other, missing."""
        r = self.rec(pid)
        if r is None:
            return "missing"
        if pid in self.ascended:
            return "ascended"
        if pid in self.imported:
            return "copy"
        if is_scroll(r):
            return "spell"
        return "gear" if is_gear(r) else "other"

    def would_be(self, pid):
        """The Ascended record base pid makes, as a row (the server's SQL, nothing written)."""
        r = self.db.execute(f"SELECT {self.froms} FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
        return dict(zip(self.names, r))

    def name(self, pid):
        r = self.rec(pid)
        return r["itemname"] if r else f"item {pid}"


# ---- the plan ----------------------------------------------------------------------------------------------------------

class Plan:
    def __init__(self):
        self.bosses = []        # {"name", "key", "spawns": [...], "lines": [...]}
        self.records = {}       # base -> {"copy": id or None, "for": set(bosses), "reward": bool}
        self.inplace = []       # {"tbl", "id", "from", "boss"}: loottables, lootpool, npcitem rows pointed at a copy
        self.tables = {}        # copy table -> {"of": T, "rows": [...], "bosses": [...], "state": new|same|sync}
        self.rebinds = []       # {"boss", "from", "to"}
        self.quests = []        # report lines
        self.notes = []         # reported, not changed
        self.problems = []      # refuse --apply
        self.warnings = []

    def changes(self):
        made = sum(1 for r in self.records.values() if r["copy"] is None and not r.get("skip"))
        rows = sum(len(t["add"]) + len(t["drop"]) for t in self.tables.values())
        return made + len(self.inplace) + rows + len(self.rebinds)


def logged_bosses(db):
    """The bosses earlier runs made Ascended (and no --undo took back)."""
    if not has_table(db, LOG):
        return set()
    return {b for (b,) in db.execute(f"SELECT DISTINCT boss FROM {LOG} WHERE boss <> ''")}


def logged_tables(db):
    """The copy tables earlier runs made: name -> the ids of the rows they wrote that are still there."""
    out = {}
    if not has_table(db, LOG):
        return out
    for ref, after in db.execute(f"SELECT ref, after FROM {LOG} WHERE tbl = 'loottables' AND before IS NULL"):
        a = json.loads(after) if after else None
        if a and a.get("source") == COPY_SOURCE:
            out.setdefault(a["tablename"], set()).add(json.loads(ref)["id"])
    return out


def suggest(db, name):
    words = sorted(re.findall(r"[a-z']{3,}", name.lower()), key=len, reverse=True)
    if not words:
        return []
    like = f"%{words[0][:6]}%"
    found = {n for (n,) in db.execute("SELECT DISTINCT npc_name FROM npcs WHERE lower(npc_name) LIKE ? LIMIT 12", (like,))}
    found |= {n for (n,) in db.execute("SELECT DISTINCT npc_name FROM npcloot WHERE npc_name LIKE ? LIMIT 12", (like,))}
    return sorted(found)[:12]


def build_plan(db, names):
    p = Plan()
    w = World(db)
    keys = {}
    for n in names:
        keys.setdefault(npc_key(n), n.strip())
    tuned = set(keys) | logged_bosses(db)
    made_tables = logged_tables(db)
    spawns = {}
    item_keys = {}  # npcs.npc_id -> the names whose corpses hold its npcitem rows (handlers/loot.go: GetNPCItems(npc.NPCID))
    has_npc_id = "npc_id" in columns(db, "npcs")
    for r in dict_rows(db, f"SELECT id, npc_name, npc_level, world{', npc_id' if has_npc_id else ''} FROM npcs "
                           "WHERE npc_name IS NOT NULL ORDER BY id"):
        k = npc_key(r["npc_name"])
        if r.get("npc_id"):
            item_keys.setdefault(r["npc_id"], set()).add(k)
        if k in keys:
            spawns.setdefault(k, []).append(r)
    binds = {}
    if has_table(db, "npcloot"):
        for r in dict_rows(db, "SELECT * FROM npcloot ORDER BY npc_name, tablename"):
            binds.setdefault(r["tablename"], []).append(r)
    by_boss = {}
    for t, rs in binds.items():
        for r in rs:
            if r["npc_name"] in keys:
                by_boss.setdefault(r["npc_name"], []).append(r)
            elif npc_key(r["npc_name"]) in keys:
                p.warnings.append(f"npcloot row ({r['npc_name']!r}, {r['tablename']!r}) is never rolled: the server "
                                  f"reads npcloot by the lower-case name ({npc_key(r['npc_name'])!r})")

    def want(pid, boss, reward=False):
        if pid not in p.records:
            p.records[pid] = {"copy": w.copy_of.get(pid), "for": set(), "reward": reward}
        if boss:
            p.records[pid]["for"].add(boss)
        p.records[pid]["reward"] = p.records[pid]["reward"] and reward

    def target(pid):
        """What a row of record pid points at once Ascended: its copy (made or to be made), else pid."""
        return w.copy_of.get(pid, ("new", pid))

    for key, shown in keys.items():
        b = {"name": shown, "key": key, "spawns": spawns.get(key, []), "lines": [], "ascended": set(), "other": set()}
        p.bosses.append(b)
        if not b["spawns"] and not by_boss.get(key):
            like = suggest(db, shown)
            p.problems.append(f"{shown!r}: no NPC and no loot table of that name here"
                              + (f" (names like it: {', '.join(like)})" if like else ""))
            continue
        level = max((s["npc_level"] or 0 for s in b["spawns"]), default=0)
        for bind in by_boss.get(key, []):
            t = bind["tablename"]
            rows = dict_rows(db, "SELECT * FROM loottables WHERE tablename = ? ORDER BY id", (t,))
            gear = [r for r in rows if w.kind(r["patternid"]) == "gear"]
            b["other"] |= {r["patternid"] for r in rows if w.kind(r["patternid"]) == "other"}
            done = [r["patternid"] for r in rows if w.kind(r["patternid"]) == "ascended"]
            for pid in done:
                b["ascended"].add(w.ascended[pid])
            head = f'table "{t}" ({bind["chance"]}% x {bind["rolls"]} rolls, {len(rows)} rows)'
            if not gear:
                b["lines"].append(f"{head}: no regular gear, as it is"
                                  + (f" ({len(done)} Ascended rows)" if done else ""))
                continue
            binders = sorted({r["npc_name"] for r in binds.get(t, [])})
            others = [n for n in binders if n not in tuned]
            if not others:
                b["lines"].append(f"{head}, rolled by no creature not made Ascended ({', '.join(binders)}): changed in place")
                for r in gear:
                    want(r["patternid"], key)
                    b["lines"].append(f"  {r['patternid']} {w.name(r['patternid'])}: its Ascended record")
                    if not any(x["tbl"] == "loottables" and x["id"] == r["id"] for x in p.inplace):
                        p.inplace.append({"tbl": "loottables", "id": r["id"], "from": r["patternid"], "boss": key})
                continue
            c = t + COPY_SUFFIX
            shown_others = ", ".join(others[:6]) + (f" and {len(others) - 6} more" if len(others) > 6 else "")
            b["lines"].append(f"{head}, rolled by {shown_others} too: {shown} rolls a copy of it, \"{c}\"")
            for r in gear:
                want(r["patternid"], key)
            if c not in p.tables:
                desired = [(target(r["patternid"]) if w.kind(r["patternid"]) == "gear" else r["patternid"],
                            r["weight"], r["minlevel"], r["maxlevel"]) for r in rows]
                have = dict_rows(db, "SELECT * FROM loottables WHERE tablename = ? ORDER BY id", (c,))
                mine = made_tables.get(c, set())
                if any(h["id"] not in mine for h in have):
                    p.problems.append(f'table "{c}" holds rows this tool did not write: the copy\'s name is taken')
                    continue
                left = list(desired)
                drop = []
                for h in have:
                    k = (h["patternid"], h["weight"], h["minlevel"], h["maxlevel"])
                    if k in left:
                        left.remove(k)
                    else:
                        drop.append(h)
                p.tables[c] = {"of": t, "add": left, "drop": drop, "bosses": [], "existed": bool(have)}
            p.tables[c]["bosses"].append(key)
            p.rebinds.append({"boss": key, "from": t, "to": c, "row": bind})
            if any(r["npc_name"] == key and r["tablename"] == c for r in binds.get(c, [])):
                p.problems.append(f'{shown} is bound to both "{t}" and its copy "{c}": take one binding away first')
        # its category pool rows of its own, and its spawns' own items
        if has_table(db, "lootpool"):
            own = dict_rows(db, "SELECT * FROM lootpool WHERE npc_name = ? ORDER BY id", (key,))
            g = [r for r in own if w.kind(r["patternid"]) == "gear"]
            for r in g:
                want(r["patternid"], key)
                p.inplace.append({"tbl": "lootpool", "id": r["id"], "from": r["patternid"], "boss": key})
            b["lines"].append(f"its own category pool rows: {len(own)}, {len(g)} of them regular gear"
                              + (" (pointed at their Ascended records)" if g else ""))
        # its spawns' own items: npcitem rows by the spawns' npc_id, a key other creatures may share
        for nid in sorted({s.get("npc_id") for s in b["spawns"] if s.get("npc_id")}):
            if not has_table(db, "npcitem"):
                break
            items = dict_rows(db, "SELECT * FROM npcitem WHERE npcid = ? ORDER BY itemid", (nid,))
            g = [r for r in items if w.kind(r["patternid"]) == "gear"]
            if not items:
                continue
            sharing = sorted(item_keys.get(nid, set()) - tuned)
            if g and sharing:
                p.warnings.append(f"{shown}'s spawns' items (npcitem, npc_id {nid}) are those of {', '.join(sharing[:6])} "
                                  f"too (not made Ascended): their {len(g)} regular gear left as they are")
                continue
            for r in g:
                want(r["patternid"], key)
                if not any(x["tbl"] == "npcitem" and x["id"] == r["itemid"] for x in p.inplace):
                    p.inplace.append({"tbl": "npcitem", "id": r["itemid"], "from": r["patternid"], "boss": key})
            b["lines"].append(f"its spawns' own items (npcitem, npc_id {nid}): {len(items)}, {len(g)} of them regular gear"
                              + (" (pointed at their Ascended records)" if g else ""))
        # what it rolls besides, reported
        for t, rs in binds.items():
            if any(r["npc_name"] == EVERY for r in rs):
                n = sum(1 for (pid,) in db.execute("SELECT patternid FROM loottables WHERE tablename = ?", (t,))
                        if w.kind(pid) == "gear")
                p.notes.append(f'{shown}: every creature\'s table "{t}" is rolled as before '
                               f'({f"{n} rows of regular gear" if n else "no gear"})')
        if has_table(db, "lootcreature") and has_table(db, "lootrate") and has_table(db, "lootpreset"):
            lc = one(db, "SELECT region, profile FROM lootcreature WHERE npc_name = ?", (key,)) or {"region": "", "profile": ""}
            if lc["region"] != "-":
                pre = one(db, "SELECT name FROM lootpreset WHERE active <> 0 ORDER BY name LIMIT 1")
                profile = lc["profile"] or "xp"
                if pre:
                    rates = dict_rows(db, "SELECT category, rate, counts FROM lootrate WHERE preset = ? AND profile = ? "
                                          "ORDER BY position, category", (pre["name"], profile))
                    if not rates:
                        rates = dict_rows(db, "SELECT category, rate, counts FROM lootrate WHERE preset = ? AND profile = 'xp' "
                                              "ORDER BY position, category", (pre["name"],))
                    parts = []
                    for r in rates:
                        pool = [pid for (pid,) in db.execute(
                            "SELECT patternid FROM lootpool WHERE category = ? AND weight > 0 AND minlevel <= ? AND maxlevel >= ? "
                            "AND npc_name = '' AND (region = ? OR region = '*')", (r["category"], level, level, lc["region"]))]
                        n = sum(1 for pid in pool if w.kind(pid) == "gear")
                        if n:
                            rate = f"1 in {1 / r['rate']:.0f}" if r["rate"] and r["rate"] > 0 else (r["counts"] or "0")
                            parts.append(f"{r['category']} {rate} a kill ({n} regular gear in its pool)")
                    if parts:
                        p.notes.append(f"{shown}: the loot categories (preset {pre['name']}, profile {profile}, region "
                                       f"{lc['region'] or '*'}, level {level}) still roll the shared pools: " + "; ".join(parts))
    # a table of Ascended gear that a creature not made Ascended rolls
    if has_table(db, "loottables"):
        for (t,) in db.execute("SELECT DISTINCT tablename FROM loottables WHERE patternid IN (SELECT patternid FROM itemascended)"
                               if has_table(db, "itemascended") else "SELECT '' WHERE 0"):
            others = sorted({r["npc_name"] for r in binds.get(t, [])} - tuned)
            if others:
                p.notes.append(f'table "{t}" holds Ascended gear and {", ".join(others[:6])}'
                               f'{" and more" if len(others) > 6 else ""} (not made Ascended) rolls it')
    # the quests that take this gear (to be made Ascended now, or made so before), and their reward gear's records
    bases = {pid for pid, r in p.records.items() if not r["reward"]}
    for b in p.bosses:
        bases |= b["ascended"]
    others = {pid: b["name"] for b in p.bosses for pid in b["other"]}
    if has_table(db, "queststeps") and (bases or others):
        names = dict(db.execute("SELECT questid, name FROM quests").fetchall()) if has_table(db, "quests") else {}
        for st in dict_rows(db, "SELECT * FROM queststeps WHERE kind IN ('give', 'show') ORDER BY questid, step"):
            # a quest item (no gear) a boss drops that a quest turns into gear: not made Ascended (gear alone is)
            q_items = [pid for pid, _ in item_counts(st["items"]) if pid in others]
            g = [w.name(pid) for pid, _ in step_rewards(st) if w.kind(pid) == "gear"]
            if q_items and g:
                p.notes.append(f'{", ".join(f"{w.name(pid)} ({others[pid]})" for pid in q_items)}, no gear, drops as it is: '
                               f'quest {st["questid"]} "{names.get(st["questid"], "")}" step {st["step"]} turns it into '
                               f'{", ".join(g)}, given regular')
        for st in dict_rows(db, "SELECT * FROM queststeps WHERE kind IN ('give', 'show', 'loot') ORDER BY questid, step"):
            items = item_counts(st["items"])
            if st["kind"] == "loot":  # a loot step's item is its first (QuestStep.LootItem)
                items = items[:1]
            took = [pid for pid, _ in items if pid in bases]
            if not took:
                continue
            what = ", ".join(w.name(pid) for pid in took)
            head = (f'quest {st["questid"]} "{names.get(st["questid"], "")}" step {st["step"]} '
                    f'({st["kind"]} {st["target"]}): {what}')
            if st["kind"] == "loot":
                p.quests.append(head + ": an Ascended one taken off a corpse counts for it")
                continue
            gives, kept = [], []
            for pid, who in step_rewards(st):
                k = w.kind(pid)
                tag = f" ({who})" if who else ""
                if k == "gear":
                    want(pid, "", reward=True)
                    gives.append(f"{ascended_name(w.name(pid))}{tag}")
                elif k != "missing":
                    kept.append(f"{w.name(pid)}{tag}, {'a spell' if k == 'spell' else 'no gear' if k == 'other' else k}")
            line = head + ": handed in Ascended, it gives " + (", ".join(gives) if gives else "no reward gear")
            p.quests.append(line + (f"; {'; '.join(kept)}: as they are" if kept else ""))
    # the records: names that fit the client
    for pid, r in sorted(p.records.items()):
        if r["copy"] is not None:
            continue
        n = ascended_name(w.name(pid))
        if len(n) > MAX_NAME:
            msg = f"{pid} {w.name(pid)}: {n!r} is {len(n)} characters, the client shows {MAX_NAME} (rename the record)"
            if r["for"]:
                p.problems.append(msg)
            else:
                r["skip"] = True
                p.warnings.append(msg + "; as a quest's reward the regular record is given")
    p.world = w
    return p


# ---- reporting ---------------------------------------------------------------------------------------------------------

def figures_line(w, base, new):
    b = w.rec(base)
    up, same = [], []
    for c in w.cols:
        low = c.lower()
        if low in SCALED and b.get(c) not in (None, 0):
            if new[c] != b[c]:
                up.append(f"{FIGURE_NAMES[low]} {b[c]} -> {new[c]}")
            else:
                same.append(f"{FIGURE_NAMES[low]} {b[c]}")
    out = ", ".join(up) or "no figure changes"
    if same:
        out += f" ({', '.join(same)} as they are: too small for a tenth, or a penalty)"
    out += f"; durability {b.get('maxhp')} and price {b.get('itemcost')} the base's"
    if b.get("rent") == 0:
        out += "; rent 1, where the base is NO RENT"
    return out


def report(p):
    w = p.world
    out = [f"Bosses: {', '.join(b['name'] for b in p.bosses)}"]
    for b in p.bosses:
        sp = b["spawns"]
        where = (f"{len(sp)} spawn{'s' if len(sp) != 1 else ''} (npcs {', '.join(str(s['id']) for s in sp[:6])}"
                 f"{' ...' if len(sp) > 6 else ''}; level {max((s['npc_level'] or 0) for s in sp)})") if sp else "no spawn"
        out.append(f"{b['name']} ({b['key']}): {where}")
        out += [f"  {line}" for line in b["lines"]]
    for c, t in sorted(p.tables.items()):
        state = ("as it should be" if not t["add"] and not t["drop"] else
                 f"{'kept in step' if t['existed'] else 'made'}: {len(t['add'])} rows to write, {len(t['drop'])} to take away")
        out.append(f'Copy table "{c}" (of "{t["of"]}", for {", ".join(t["bosses"])}): {state}')
    new = [(pid, r) for pid, r in sorted(p.records.items()) if r["copy"] is None and not r.get("skip")]
    old = [(pid, r) for pid, r in sorted(p.records.items()) if r["copy"] is not None]
    out.append(f"Ascended records: {len(new)} to make, {len(old)} made already")
    for pid, r in new:
        rec = w.would_be(pid)
        why = "a quest's reward" if r["reward"] else ", ".join(sorted(r["for"]))
        out.append(f"  {rec['itemname']} (of {pid}, {SLOT_NAMES.get(w.rec(pid)['equipslot'], '?')}; {why}): "
                   f"{figures_line(w, pid, rec)}")
    for pid, r in old:
        out.append(f"  {r['copy']} {w.name(r['copy'])} (of {pid}): made already")
    out.append(f"Quests that take this gear: {len(p.quests) or 'none'}")
    out += [f"  {q}" for q in p.quests]
    for n in p.notes:
        out.append(f"Not changed: {n}")
    for x in p.warnings:
        out.append(f"WARNING: {x}")
    for x in p.problems:
        out.append(f"PROBLEM: {x}")
    return out


# ---- writing -------------------------------------------------------------------------------------------------------

class Writer:
    def __init__(self, db, run):
        self.db, self.run = db, run
        self.at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
        self.n = 0

    def audit(self, action, tbl, ref, before, after, detail):
        cur = self.db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                              "VALUES (?, 0, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                              (self.at, WHO, VIA, action, tbl, go_json(ref), None if before is None else go_json(before),
                               None if after is None else go_json(after), detail))
        return cur.lastrowid

    def log(self, boss, tbl, ref, before, after, action):
        a = None
        if tbl not in ("sqlite_sequence", "sqlite_master"):
            a = self.audit(action, tbl, ref, before, after, f"{DETAIL} (run {self.run})")
        self.db.execute(f"INSERT INTO {LOG} (run, boss, tbl, ref, before, after, audit) VALUES (?, ?, ?, ?, ?, ?, ?)",
                        (self.run, boss, tbl, go_json(ref), None if before is None else go_json(before),
                         None if after is None else go_json(after), a))
        self.n += 1


def make_record(db, wr, w, base):
    """base's Ascended record, made (the server's SQL), its use effects and its itemascended row, each logged."""
    nid = db.execute("SELECT COALESCE(MAX(patternid), ?) + 1 FROM itempattern WHERE patternid BETWEEN ? AND ?",
                     (FIRST_ID - 1, FIRST_ID, LAST_ID)).fetchone()[0]
    if nid > LAST_ID:
        raise SystemExit(f"no id left for an Ascended record ({FIRST_ID}-{LAST_ID})")
    db.execute(f'INSERT INTO itempattern ("patternid", {w.sets}) SELECT ?, {w.froms} FROM itempattern WHERE patternid = ?',
               (nid, base))
    wr.log("", "itempattern", {"patternid": nid}, None, row_of(db, "itempattern", {"patternid": nid}), "insert")
    if has_table(db, "itemeffect"):
        for e in dict_rows(db, "SELECT * FROM itemeffect WHERE patternid = ?", (base,)):
            e = dict(e, patternid=nid)
            for c in ("line", "target_line"):
                if c in e:
                    e[c] = scaled_line(e[c])
            cs = list(e)
            db.execute(f"INSERT INTO itemeffect ({', '.join(cs)}) VALUES ({', '.join('?' * len(cs))})", [e[c] for c in cs])
            wr.log("", "itemeffect", {"patternid": nid}, None, row_of(db, "itemeffect", {"patternid": nid}), "insert")
    db.execute("INSERT INTO itemascended (patternid, base, made, source) VALUES (?, ?, ?, ?)",
               (nid, base, wr.at, f"tools/ascend_boss_loot.py {wr.run}"))
    wr.log("", "itemascended", {"patternid": nid}, None, row_of(db, "itemascended", {"patternid": nid}), "insert")
    w.copy_of[base] = nid
    w.ascended[nid] = base
    return nid


def seq(db, t):
    if not has_table(db, "sqlite_sequence"):
        return None
    r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = ?", (t,)).fetchone()
    return None if r is None else r[0]


def apply(db, p, run):
    """Write the plan (one transaction, the caller's): the number of rows written."""
    w = p.world
    made = [t for t in ("itemascended", "itemimported") if not has_table(db, t)]
    for s in (SCHEMA, IMPORTED_SCHEMA, AUDIT_SCHEMA, LOG_SCHEMA):
        db.execute(s)
    wr = Writer(db, run)
    for t in made:  # the server makes both at its start; on a database it never opened, --undo takes them away
        wr.log("", "sqlite_master", {"table": t}, None, {"made": True}, "create")
    seqs = {t: seq(db, t) for t in ("itempattern", "loottables")}
    for base, r in sorted(p.records.items()):
        if r["copy"] is None and not r.get("skip"):
            r["copy"] = make_record(db, wr, w, base)
    for x in p.inplace:
        tbl, keycol = x["tbl"], KEYS[x["tbl"]][0]
        ref = {keycol: x["id"]}
        before = row_of(db, tbl, ref)
        if before is None or before["patternid"] != x["from"]:
            raise SystemExit(f"{tbl} {x['id']} changed under the plan")
        db.execute(f"UPDATE {tbl} SET patternid = ? WHERE {keycol} = ?", (w.copy_of[x["from"]], x["id"]))
        wr.log(x["boss"], tbl, ref, before, row_of(db, tbl, ref), "update")
    for c, t in sorted(p.tables.items()):
        for h in t["drop"]:
            before = row_of(db, "loottables", {"id": h["id"]})
            db.execute("DELETE FROM loottables WHERE id = ?", (h["id"],))
            wr.log("", "loottables", {"id": h["id"]}, before, None, "delete")
        for pid, weight, lo, hi in t["add"]:
            if isinstance(pid, tuple):  # ("new", base): the record made above
                pid = w.copy_of[pid[1]]
            cur = db.execute("INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel, source) "
                             "VALUES (?, ?, ?, ?, ?, ?)", (c, pid, weight, lo, hi, COPY_SOURCE))
            wr.log("", "loottables", {"id": cur.lastrowid}, None, row_of(db, "loottables", {"id": cur.lastrowid}), "insert")
    for rb in p.rebinds:
        ref = {"npc_name": rb["boss"], "tablename": rb["from"]}
        before = row_of(db, "npcloot", ref)
        if before is None:
            raise SystemExit(f"npcloot {ref} changed under the plan")
        db.execute("UPDATE npcloot SET tablename = ? WHERE npc_name = ? AND tablename = ?", (rb["to"], rb["boss"], rb["from"]))
        wr.log(rb["boss"], "npcloot", ref, before, row_of(db, "npcloot", {"npc_name": rb["boss"], "tablename": rb["to"]}),
               "update")
    for t, was in seqs.items():
        now = seq(db, t)
        if now != was:  # the tables' counters, put back at --undo so the database is as it was
            wr.log("", "sqlite_sequence", {"name": t}, None if was is None else {"seq": was}, {"seq": now}, "update")
    return wr.n


# ---- undoing -------------------------------------------------------------------------------------------------------

def referenced(db, pid):
    """Where else record pid is named: every table with a patternid column but the records' own, an import of it,
    and the quests' steps."""
    out = []
    own = {"itempattern", "itemascended", "itemeffect", LOG}
    for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'").fetchall():
        if t in own:
            continue
        cs = [c.lower() for c in columns(db, t)]
        if "patternid" in cs:
            n = db.execute(f'SELECT count(*) FROM "{t}" WHERE patternid = ?', (pid,)).fetchone()[0]
            if n:
                out.append(f"{t} {n}")
    if has_table(db, "itemimported") and db.execute("SELECT count(*) FROM itemimported WHERE base = ?", (pid,)).fetchone()[0]:
        out.append("an import's copy of it")
    if has_table(db, "queststeps"):
        num = re.compile(rf"(?<![0-9]){pid}(?![0-9])")
        for qid, step, *texts in db.execute("SELECT questid, step, items, rewarditems, byclass, choices FROM queststeps "
                                            "WHERE items LIKE ? OR rewarditems LIKE ? OR byclass LIKE ? OR choices LIKE ?",
                                            (f"%{pid}%",) * 4):
            if any(num.search(t or "") for t in texts):
                out.append(f"quest {qid} step {step}")
    return out


def undo(db, run=None):
    """Put back every logged row still as written, the last first (of one run, or all): (back, left lines)."""
    if not has_table(db, LOG):
        return 0, []
    wr = Writer(db, "undo")
    back, left = 0, []
    where, args = ("WHERE run = ?", (run,)) if run else ("", ())
    entries = db.execute(f"SELECT seq, run, boss, tbl, ref, before, after FROM {LOG} {where} ORDER BY seq DESC", args).fetchall()
    kept = {}  # Ascended record -> why it stays

    def note(action, tbl, ref, before, after, r):
        wr.audit(action, tbl, ref, before, after, f"undo: {DETAIL} (run {r})")

    for sq, r, boss, tbl, ref, before, after in entries:
        ref = json.loads(ref)
        before = json.loads(before) if before is not None else None
        after = json.loads(after) if after is not None else None
        done = False
        if tbl == "sqlite_master":
            if has_table(db, ref["table"]) and not db.execute(f'SELECT count(*) FROM "{ref["table"]}"').fetchone()[0]:
                db.execute(f'DROP TABLE "{ref["table"]}"')
                done = True
            elif has_table(db, ref["table"]):
                left.append(f"table {ref['table']}: rows in it, kept")
                continue
        elif tbl == "sqlite_sequence":
            cur = seq(db, ref["name"])
            if cur == after["seq"]:
                if before is None:
                    db.execute("DELETE FROM sqlite_sequence WHERE name = ?", (ref["name"],))
                else:
                    db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = ?", (before["seq"], ref["name"]))
                done = True
            else:
                left.append(f"the {ref['name']} counter: {cur}, not {after['seq']} (rows made since): left")
        elif tbl in ("itempattern", "itemeffect", "itemascended"):
            pid = ref["patternid"]
            if pid not in kept:
                refs = referenced(db, pid)
                kept[pid] = ", ".join(refs)
                if refs:
                    left.append(f"Ascended record {pid} stays: {kept[pid]}")
            if kept[pid]:
                continue
            now = row_of(db, tbl, ref)
            if now is not None:
                db.execute(f"DELETE FROM {tbl} WHERE patternid = ?", (pid,))
                note("delete", tbl, ref, now, None, r)
            done = True
        else:
            if tbl == "npcloot":
                cur_ref = {"npc_name": after["npc_name"], "tablename": after["tablename"]}
            else:
                cur_ref = ref
            now = row_of(db, tbl, cur_ref) if after is not None else row_of(db, tbl, ref)
            if after is not None and (now is None or go_json(now) != go_json(after)):
                left.append(f"{tbl} {go_json(cur_ref)}: changed since it was written, left")
            elif after is None and now is not None:
                left.append(f"{tbl} {go_json(ref)}: made again since it was taken away, left")
            elif before is None:  # made here: taken away
                db.execute(f"DELETE FROM {tbl} WHERE " + " AND ".join(f"{c} = ?" for c in KEYS[tbl]),
                           tuple(cur_ref[c] for c in KEYS[tbl]))
                note("delete", tbl, cur_ref, now, None, r)
                done = True
            elif after is None:  # taken away here: put back
                cs = list(before)
                db.execute(f"INSERT INTO {tbl} ({', '.join(cs)}) VALUES ({', '.join('?' * len(cs))})", [before[c] for c in cs])
                note("insert", tbl, ref, None, row_of(db, tbl, ref), r)
                done = True
            else:  # changed here: as it was
                cs = [c for c in before if c not in KEYS[tbl]] + ([] if tbl != "npcloot" else ["tablename"])
                cs = list(dict.fromkeys(cs))
                db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in cs)} WHERE "
                           + " AND ".join(f"{c} = ?" for c in KEYS[tbl]),
                           [before[c] for c in cs] + [cur_ref[c] for c in KEYS[tbl]])
                note("update", tbl, cur_ref, now, row_of(db, tbl, ref), r)
                done = True
        if done:
            back += 1
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (sq,))
    if not db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0]:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def backup(path):
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-ascended"
    src, out = sqlite3.connect(path, timeout=30), sqlite3.connect(dst)
    try:
        src.backup(out)
    finally:
        out.close()
        src.close()
    return dst


def connect(path):
    db = sqlite3.connect(path, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    return db


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0], formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--boss", action="append", default=[], metavar="NAME", help="a boss to make Ascended, as npcs spells it")
    ap.add_argument("--tuned", action="store_true", help="every boss of TUNED: " + ", ".join(TUNED))
    ap.add_argument("--no-backup", action="store_true")
    ap.add_argument("--run", help="with --undo: that run alone (as --apply printed it)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write it")
    g.add_argument("--undo", action="store_true", help="put back what --apply wrote")
    a = ap.parse_args(argv)
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    if a.undo:
        if not a.no_backup:
            print(f"copy: {backup(a.db)}")
        db = connect(a.db)
        try:
            db.execute("BEGIN IMMEDIATE")
            try:
                db.execute(AUDIT_SCHEMA)
                back, left = undo(db, a.run)
                db.execute("COMMIT")
            except BaseException:
                db.execute("ROLLBACK")
                raise
        finally:
            db.close()
        for x in left:
            print("left:", x)
        print(f"put back {back} rows; {len(left)} left as they are")
        return 0
    names = (TUNED if a.tuned else []) + a.boss
    if not names:
        ap.error("name a boss (--boss) or --tuned")
    db = connect(a.db)
    try:
        p = build_plan(db, names)
        for line in report(p):
            print(line)
        n = p.changes()
    finally:
        db.close()
    if not a.apply:
        print(f"{n} changes: report only, --apply writes them")
        return 2 if p.problems else 0
    if p.problems:
        print(f"nothing written: {len(p.problems)} problems")
        return 2
    if n == 0:
        print("nothing to do: these bosses' loot is Ascended already")
        return 0
    if not a.no_backup:
        print(f"copy: {backup(a.db)}")
    run = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%S.%fZ")
    db = connect(a.db)
    try:
        db.execute("BEGIN IMMEDIATE")
        try:
            p = build_plan(db, names)  # again, under the lock
            if p.problems:
                raise SystemExit(f"nothing written: {len(p.problems)} problems (the database changed)")
            wrote = apply(db, p, run)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
    finally:
        db.close()
    print(f"run {run}: wrote {wrote} rows (each in gm_audit); no restart needed (--undo --run {run} takes it back)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
