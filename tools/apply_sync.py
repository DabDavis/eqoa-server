#!/usr/bin/env python3
"""Apply one shard's half of a reviewed shard-sync plan (2026-11-26), in ONE transaction, or undo it.

The plan is plan_filtered.json: tools/live_sync_20261122.py's comparison plan (version 2, made on VACUUM copies) with
the exclusions of filter.py applied. plan["L"] is written to Live, plan["H"] to Heirloom. Each op is a row change with
the whole row expected before and the whole row after.

  apply_sync.py --db eqoa.db --shard live --plan plan_filtered.json --check     # read only: would it apply?
  apply_sync.py --db eqoa.db --shard live --plan plan_filtered.json --apply
  apply_sync.py --db eqoa.db --shard live --undo --run <run>

--apply REFUSES (nothing written) when any row is not as the plan expects: a row the plan updates or deletes that differs
from its expected before (and is not already as the plan leaves it), a row the plan inserts whose natural key is held by
another row. Ids the plan mints (npcs, itempattern, spellpattern, quests) that a GM's new row has taken since the plan are
moved to fresh ids and every reference inside the plan rewritten (see MINT below); tables whose own id nothing
references give the row the next id, as live_sync's --apply does. --check takes no write lock.
A row already exactly as the plan leaves it is counted and left. A quest whose steps change shape (the plan's sigchange)
is left whole, not refused, while a character of this shard is part-way through it (charquests done = 0): no progress is
moved. Every change writes gm_audit (username dab, via tool) and shard_sync_1126_log (whole rows before/after, the
gm_audit id); sqlite_sequence before/after in shard_sync_1126_seq. --undo puts back exactly one run, in one transaction,
and refuses (nothing written) if any row it would restore changed since the run.

The shard is bound by the server config next to the database (gm.shard_id and server.db_path must name this file); a
rehearsal copy without a config needs --rehearsal. Standard library only.

REFERENCES (2026-11-27, Heirloom petition 3352: the 11-22 sync wrote Live's "#<npcs id>" spawns into Heirloom's triggers
150/161 and quest 100790's events; the 11-26 one deleted NPC rows creatures' pets (npcpets) are made from, and wrote
merchant rows for an NPC its filter had left out). The registry is shard_refs.py (beside this file or in tool/).
--check and --apply both REFUSE (named) when, after the plan, (a) a reference in any row the plan writes names no row of
the shard, (b) a row anywhere on the shard names a row the plan deletes, (c) a translated NPC or trigger reference names
a row that is not the one it came from (the plan's "refs": name and world for an NPC, unless it is the same shared row
below SHARED_NPC_IDS; the trigger's name), or (d) the plan's half lists rows the planner could not translate
("unresolved"; --leave-unresolved applies the rest and leaves them). --check reads it all off the plan (no write lock);
--apply checks again inside its transaction, after its writes and before COMMIT. Run the whole check on a shard at any
time (read only, exit 1 on a miss):
  apply_sync.py --check-refs --db eqoa.db
"""
import argparse
import collections
import datetime
import json
import re
import sqlite3
import sys
import time
from pathlib import Path

HERE = Path(__file__).resolve().parent
for _d in (HERE, HERE / "tool"):
    if (_d / "shard_refs.py").is_file():
        sys.path.insert(0, str(_d))
        break
import shard_refs as refs  # noqa: E402

LOG, SEQ = "shard_sync_1126_log", "shard_sync_1126_seq"
WHO, VIA = "dab", "tool"
SHARED_NPC_IDS = 162560   # npcs ids below it are the rows both shards were cloned from (shard_sync_20261115.py)
NPC_NEAR = 60.0           # a translated NPC reference further than this from its src row is reported (not refused)
TOOL = "tools/apply_sync.py (live_sync_20261122.py comparison)"
DETAIL = "Shard sync 2026-11-26 (owner-approved two-way content sync; outside the ticket workflow)"
SIDE = {"live": "L", "heirloom": "H"}
# tables whose own id nothing references (live_sync_20261122.realloc_ok over the engine's SPECS)
REALLOC = {"cityintro", "defaultcharacter", "defaultcharinventory", "defaultspells", "factionhitareas", "factionhits",
           "fishcatch", "gmportals", "gmtriggers", "hazards", "heroes_stock", "lightspheres", "lightstrings", "lootpool",
           "loottables", "mapmarks", "noroam", "npcdialog", "npcfaction", "npcitem", "npcoverseer", "orbs", "placetexts",
           "questnotyet", "startitems", "tokenshop"}
QUEST_TABLES = {"quests", "queststeps", "queststeptargets", "questunlocks", "questwalls", "questmarks", "questnotyet", "questdoors"}

# ---- ids the plan mints (an inserted row of a table whose id other rows point at) ---------------------------------------
# A GM working in the game while the plan waits takes the next AUTOINCREMENT ids, which may be the ones the plan chose. At
# apply time each such id that is now another row's is moved to a fresh id above the table's max and sqlite_sequence (and
# above every id the plan itself inserts), and every reference to it INSIDE the plan is rewritten: the integer columns
# below, an exact "#<id>" name, and the id as a token inside the text columns below. Any other place the old id appears in
# the plan (an integer column or a text column not listed) refuses the apply: remapping is only done where it is known
# sound. Rows of the database outside the plan are never rewritten.
MINT = {"npcs": "id", "itempattern": "patternid", "spellpattern": "spellid", "quests": "questid"}
NOTE_COLS = {"note", "notes", "source", "detail", "why"}
REF_INT = {
    "npcs": {("npcrespawn", "npc_row"), ("npcwander", "npcid"), ("npcpaths", "npcid"), ("npcrare", "npc_row"),
             ("npctitle", "npcid"), ("signtext", "npcid"), ("campspawns", "npcid"), ("npcstands", "npc_row"),
             ("npcitem", "npcid"), ("questwalls", "npcid"), ("npcrarecycle", "npc_row"), ("questdoors", "npcid"),
             ("queststeps", "npcid")},
    "itempattern": {(t, "patternid") for t in ("itemeffect", "itemaura", "itemfactions", "foodeffect", "gemsetting",
                    "heldbarred", "norentvanish", "itemspeed", "itemhide", "itemspell", "lootdroptrigger", "itemoutfit",
                    "itemascended", "itemimported", "npcitem", "tokenshop", "loottables", "lootpool", "npcheld",
                    "merchantprice", "startitems", "heroes_stock")} | {("questdoors", "item")},
    "spellpattern": {(t, "spellid") for t in ("spelleffect", "spellarea", "petstats", "spellrecastgroup", "classspells",
                     "spellascended", "itemspell", "defaultspells")},
    "quests": {(t, "questid") for t in ("queststeps", "queststeptargets", "questunlocks", "questwalls", "questmarks",
               "questnotyet", "questdoors", "itemhide")},
}
REF_TXT = {
    "npcs": {("campspawns", "kinds"), ("campspawns", "weights"), ("campspawns", "templates"), ("gmtriggers", "steps"),
             ("queststeps", "events"), ("quests", "events")},
    "itempattern": {("queststeps", c) for c in ("items", "rewarditems", "byclass", "choices", "events")} |
                   {("quests", "events"), ("quests", "requires"), ("craftrecipes", "product"), ("craftrecipes", "ingredients"),
                    ("gmtriggers", "steps"), ("spelleffect", "effects"), ("npctalk", "choices")},
    "spellpattern": {("classtemplates", "abilities"), ("cmcatalog", "require_spells"), ("cmcatalog", "add_spells"),
                     ("cmcatalog", "remove_spells"), ("spelleffect", "effects"), ("gmtriggers", "steps")} |
                    {("queststeps", c) for c in ("spells", "byclass", "choices", "events")},
    "quests": {("quests", "requires"), ("queststeps", "events"), ("quests", "events"), ("gmtriggers", "steps")},
}


def tok(i):
    return re.compile(r"(?<![\w.])" + str(i) + r"(?![\w.])")


def remap(db, ops, refmap=None):
    """(ops with every minted id now held by another row moved to a fresh id, [(table, old, new)]); the plan's refs map
    (dst id -> src row) follows each move. A reference site of shard_refs.py is rewritten by key (2026-11-27): only a
    reference of the moved id's own kind changes (an item list holding the same number is left)."""
    taken = []
    for op in ops:
        t = op["t"]
        if op["before"] is None and t in MINT:
            cur = fetch(db, t, op["pk"])
            if cur is not None and jn(cur) != jn(op["after"]):
                taken.append((t, op["pk"][MINT[t]]))
    if not taken:
        return ops, []
    ops = json.loads(json.dumps(ops))
    nxt = {}
    moves = []
    for t, old in taken:
        col = MINT[t]
        if t not in nxt:
            planned = max([o["pk"][col] for o in ops if o["t"] == t and o["before"] is None] + [0])
            top = db.execute(f"SELECT max({q(col)}) FROM {q(t)}").fetchone()[0] or 0
            nxt[t] = max(top, seq_of(db, t) or 0, planned) + 1
        new = nxt[t]
        nxt[t] += 1
        if fetch(db, t, {col: new}) is not None:
            raise Refused(f"{t} {new}: the fresh id is taken")
        rx, name = tok(old), f"#{old}"
        kind = refs.MINT_KIND[t]
        if refmap and str(old) in (refmap.get(kind) or {}):
            refmap[kind][str(new)] = refmap[kind].pop(str(old))
        for o in ops:
            for part in ("pk", "after"):
                d = o.get(part)
                if not d:
                    continue
                for c, v in list(d.items()):
                    own = o["t"] == t and c == col
                    site = refs.SITE.get((o["t"], c))
                    if isinstance(v, int) and not isinstance(v, bool) and v == old:
                        if own or (o["t"], c) in REF_INT[t] or refs.INT_KIND.get((o["t"], c)) == kind:
                            d[c] = new
                        elif o["before"] is not None and part == "pk":
                            raise Refused(f"{o['t']} {jn(o['pk'])}: an existing row keyed by {t} {old}, which another row now holds")
                        else:
                            raise Refused(f"{t} {old} appears in {o['t']}.{c}, a column not known to reference it: make a new plan")
                    elif isinstance(v, str) and (v == name or rx.search(v)):
                        if site in ("rowkey", "list", "json"):
                            d[c] = refs.rewrite(o["t"], c, v, lambda k, i: new if (k == kind and i == old) else i)
                        elif v == name:
                            d[c] = f"#{new}"
                        elif (o["t"], c) in REF_TXT[t] or c in NOTE_COLS:
                            d[c] = rx.sub(str(new), v)
                        else:
                            raise Refused(f"{t} {old} appears in the text of {o['t']}.{c}, not known to reference it: make a new plan")
            if o["before"] is not None and o["after"] is not None:
                o["pk"] = {c: o["after"].get(c, v) for c, v in o["pk"].items()}
        moves.append((t, old, new))
    return ops, moves


# ---- the reference check (2026-11-27) -----------------------------------------------------------------------------------
def deleted_ids(ops, skip=()):
    """{kind: ids of rows the plan deletes} (npcs, items, spells, quests, triggers)"""
    out = collections.defaultdict(set)
    for i, op in enumerate(ops):
        k = refs.MINT_KIND.get(op["t"])
        if k and i not in skip and op["before"] is not None and op["after"] is None:
            out[k].add(op["pk"][refs.KINDS[k][1]])
    return out


def ref_problems(db, rows, ids, gone, refmap, row_of, scan_skip=None, scan_extra=None):
    """[problem text]: (a) a reference of a written row naming no row; (b) a row of the shard naming a deleted row;
    (c) a translated NPC/trigger reference naming another row than the one it came from. rows: [(table, row)] the
    plan writes; ids: {kind: set} the rows the shard holds after the plan; gone: deleted_ids(); row_of(kind, id): that
    row after the plan (a dict) or None."""
    bad, notes = [], []
    refmap = refmap or {}
    for t, row in rows:
        lab = f"{t} {jn({c: row.get(c) for c in refs.pkcols(db, t) if c in row})}"
        for c, v in row.items():
            for kind, i, where in refs.refs_in(t, c, v):
                if i not in ids[kind]:
                    bad.append(f"(a) {lab} {where}: {kind} {i} names no {refs.KINDS[kind][0]} row on this shard")
                    continue
                ident = (refmap.get(kind) or {}).get(str(i))
                if not ident:
                    continue
                cur = row_of(kind, i)
                if cur is None:
                    continue
                if kind == "slot":
                    if (cur.get("name") or "").strip().lower() != ident["name"]:
                        bad.append(f"(c) {lab} {where}: trigger slot {i} is {cur.get('name')!r} here, but the plan "
                                   f"translated it from {ident['name']!r} (slot {ident['src']})")
                    continue
                shared = ident["src"] == i and i < SHARED_NPC_IDS
                same = ((cur.get("npc_name") or "").strip().lower() == (ident["name"] or "").strip().lower() and
                        cur.get("world") == ident["world"])
                if not same and not shared:
                    bad.append(f"(c) {lab} {where}: npcs {i} is {cur.get('npc_name')!r} world {cur.get('world')} here, but "
                               f"the plan translated it from {ident['name']!r} world {ident['world']} (row {ident['src']})")
                elif not same or (ident.get("modelid") is not None and cur.get("modelid") != ident["modelid"]) or \
                        ((cur.get("x_coord") or 0) - ident["x"]) ** 2 + ((cur.get("z_coord") or 0) - ident["z"]) ** 2 > NPC_NEAR ** 2:
                    notes.append(f"{lab} {where}: npcs {i} ({cur.get('npc_name')!r}) differs in place, look or name from "
                                 f"its src row {ident['src']} ({ident['name']!r}); the same row by the plan's pairing")
    if any(gone.values()):
        for m in refs.scan(db, ids, scan_skip, scan_extra):
            if m["id"] in gone.get(m["kind"], ()):
                bad.append(f"(b) {refs.describe(m)}: the plan deletes that row")
    return bad, notes


def check_plan_refs(db, ops, skip, states, refmap):
    """the check on the plan, before any write (--check, and --apply before it writes): the shard as the plan leaves it"""
    ids = refs.existing(db)
    gone = deleted_ids(ops, skip)
    rows, after_of, scan_skip, scan_extra = [], {}, collections.defaultdict(set), collections.defaultdict(list)
    for i, op in enumerate(ops):
        if i in skip:
            continue
        t, k = op["t"], refs.MINT_KIND.get(op["t"])
        pkc = refs.pkcols(db, t)
        if op["before"] is not None or states.get(i, ("",))[0] == "there":
            scan_skip[t].add(refs.pkjson({c: (op["before"] or op["after"]).get(c) for c in pkc}))
        if op["after"] is None:
            if k:
                ids[k].discard(op["pk"][refs.KINDS[k][1]])
            continue
        rows.append((t, op["after"]))
        scan_extra[t].append(op["after"])
        if k:
            ids[k].add(op["after"][refs.KINDS[k][1]])
            after_of[(k, op["after"][refs.KINDS[k][1]])] = op["after"]

    def row_of(kind, i):
        if (kind, i) in after_of:
            return after_of[(kind, i)]
        t, c = refs.KINDS[kind]
        return fetch(db, t, {c: i})

    return ref_problems(db, rows, ids, gone, refmap, row_of, scan_skip, scan_extra)


def refuse_on(bad, notes, when):
    for n in notes[:20]:
        print("  NOTE", n)
    if bad:
        raise Refused(f"{len(bad)} reference problem(s) {when}; nothing written:\n  " + "\n  ".join(bad[:40]) +
                      (f"\n  ... and {len(bad) - 40} more" if len(bad) > 40 else ""))


LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq INTEGER PRIMARY KEY AUTOINCREMENT, run TEXT NOT NULL, tbl TEXT NOT NULL, pk TEXT NOT NULL,
    before TEXT, after TEXT, audit INTEGER NOT NULL DEFAULT 0, undone INTEGER NOT NULL DEFAULT 0)"""
SEQ_DDL = f"""CREATE TABLE IF NOT EXISTS {SEQ} (
    run TEXT NOT NULL, tbl TEXT NOT NULL, before INTEGER, after INTEGER, PRIMARY KEY (run, tbl))"""


class Refused(Exception):
    pass


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def jn(v):
    return json.dumps(v, sort_keys=True, separators=(",", ":"), default=str)


def q(c):
    return '"' + c.replace('"', '""') + '"'


def fetch(db, t, pk):
    cur = db.execute(f"SELECT * FROM {q(t)} WHERE " + " AND ".join(f"{q(c)} = ?" for c in pk), tuple(pk.values()))
    r = cur.fetchone()
    return None if r is None else json.loads(jn(dict(zip([d[0] for d in cur.description], r))))


def has(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone() is not None


def seq_of(db, t):
    if not has(db, "sqlite_sequence"):
        return None
    r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = ?", (t,)).fetchone()
    return r[0] if r else None


def autoinc(db, t):
    r = db.execute("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone()
    return bool(r and "AUTOINCREMENT" in (r[0] or "").upper())


def config_shard(dbpath):
    """(shard_id, db_path) of the config.yaml beside the database, or None"""
    cfg = Path(dbpath).resolve().parent / "config.yaml"
    if not cfg.is_file():
        return None
    vals, section, child = {}, None, None
    for line in cfg.read_text(encoding="utf-8-sig").splitlines():
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        ind = len(line) - len(line.lstrip(" "))
        key, sep, val = line.strip().partition(":")
        if not ind:
            section, child = (key if sep else None), None
            if key == "db_path":
                vals["db_path"] = val
            continue
        if child is None:
            child = ind
        if ind == child and ((section == "server" and key == "db_path") or (section == "gm" and key == "shard_id")):
            vals[key] = val
    clean = {k: re.sub(r"\s+#.*$", "", v).strip().strip("\"'") for k, v in vals.items()}
    return clean.get("shard_id"), (cfg.parent / clean.get("db_path", "")).resolve()


def bind(dbpath, shard, rehearsal):
    c = config_shard(dbpath)
    if c is None:
        if not rehearsal:
            raise Refused("no config.yaml beside the database: a rehearsal copy needs --rehearsal")
        return
    sid, path = c
    if path != Path(dbpath).resolve():
        if rehearsal:
            return
        raise Refused(f"config.yaml beside the database names {path}, not this file")
    if sid != shard:
        raise Refused(f"this database is the {sid} shard's (config.yaml), not {shard}")


def connect(dbpath):
    return sqlite3.connect(Path(dbpath).resolve().as_uri() + "?mode=rw", uri=True, timeout=30, isolation_level=None)


def state(db, op):
    cur = fetch(db, op["t"], op["pk"])
    b, a = op["before"], op["after"]
    if b is None:
        if cur is None:
            return "do", cur
        if jn(cur) == jn(a):
            return "there", cur
        return ("realloc" if op["t"] in REALLOC else "taken"), cur
    if a is None:
        if cur is None:
            return "there", cur
        return ("do" if jn(cur) == jn(b) else "changed"), cur
    if cur is not None and jn(cur) == jn(a):
        return "there", cur
    if cur is not None and jn(cur) == jn(b):
        return "do", cur
    return "changed", cur


def apply(dbpath, shard, plan_path, check, rehearsal, leave_unresolved=False):
    bind(dbpath, shard, rehearsal)
    t0 = time.time()
    plan = json.load(open(plan_path))
    half = plan[SIDE[shard]]
    ops, sig = half["ops"], set(half.get("sigchange") or [])
    refmap = json.loads(json.dumps(half.get("refs") or {}))
    unresolved = half.get("unresolved") or []
    for u in unresolved[:40]:
        print(f"  UNRESOLVED (not in the plan) {u.get('table')} {jn(u.get('key'))}: {u.get('why', '')[:200]}")
    if unresolved and not leave_unresolved:
        raise Refused(f"the plan's {shard} half lists {len(unresolved)} row(s) whose references the planner could not "
                      f"translate (above): fix them or the plan, or pass --leave-unresolved to apply the rest without them")
    db = connect(dbpath)
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    acct = acct[0] if acct else 0
    run = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%SZ") + "-" + shard
    done, there, left, bad = collections.Counter(), 0, [], []
    db.execute("BEGIN" if check else "BEGIN IMMEDIATE")
    try:
        ops, moves = remap(db, ops, refmap)
        for t, old, new in moves:
            print(f"  REMAP {t} {old} -> {new} (the planned id is another row's now)")
        # quests whose steps change shape while a character is part-way: left whole
        skip = set()
        units = collections.defaultdict(list)
        for i, op in enumerate(ops):
            if op.get("unit"):
                units[op["unit"]].append(i)
        for u in sig:
            qids = {ops[i]["pk"].get("questid") for i in units.get(u, []) if ops[i]["t"] in QUEST_TABLES} - {None}
            if qids and has(db, "charquests"):
                n = db.execute(f"SELECT COUNT(*) FROM charquests WHERE done = 0 AND questid IN ({','.join('?' * len(qids))})",
                               tuple(qids)).fetchone()[0]
                if n:
                    left.append(f"{u}: its steps change shape and {n} character(s) are part-way: left whole")
                    skip.update(units[u])
        states = {}
        for i, op in enumerate(ops):
            if i in skip:
                continue
            st, cur = state(db, op)
            states[i] = (st, cur)
            if st in ("changed", "taken"):
                bad.append(f"{op['t']} {jn(op['pk'])}: {st} since the plan (now {jn(cur)[:200]})")
        if bad:
            raise Refused(f"{len(bad)} row(s) not as the plan expects; nothing written. First ones:\n  " + "\n  ".join(bad[:20]))
        if check:   # (--apply makes the same check on the shard as written, before COMMIT: one scan, not two)
            rb, rn = check_plan_refs(db, ops, skip, states, refmap)
            refuse_on(rb, rn, "in the plan as this shard would hold it")
            c = collections.Counter(s for s, _ in states.values())
            db.execute("ROLLBACK")
            print(f"CHECK OK on {shard}: {dict(c)}; references resolve; {len(moves)} minted id(s) remapped; {len(skip)} op(s) of {len(left)} quest(s) would be left whole; "
                  f"{time.time() - t0:.2f} s")
            for x in left:
                print("  LEFT", x)
            return
        db.execute(LOG_DDL)
        db.execute(SEQ_DDL)
        seqs = {}
        written = []
        for i, op in enumerate(ops):
            if i in skip:
                continue
            st, cur = states[i]
            if st == "there":
                there += 1
                continue
            t, pk = op["t"], dict(op["pk"])
            if t not in seqs and autoinc(db, t):
                seqs[t] = seq_of(db, t)
            if op["before"] is None:
                vals = dict(op["after"])
                if st == "realloc":
                    for c in pk:
                        vals.pop(c, None)
                cs = list(vals)
                c2 = db.execute(f"INSERT INTO {q(t)} ({', '.join(q(c) for c in cs)}) VALUES ({', '.join('?' * len(cs))})",
                                tuple(vals[c] for c in cs))
                if st == "realloc":
                    pk = {c: c2.lastrowid for c in pk}
                after, before, action = fetch(db, t, pk), None, "insert"
                want = dict(op["after"], **pk) if st == "realloc" else op["after"]
                if jn(after) != jn(want):
                    raise Refused(f"{t} {jn(pk)}: the inserted row reads back otherwise ({jn(after)[:200]}); schema changed? make a new plan")
            elif op["after"] is None:
                rid = db.execute(f"SELECT rowid FROM {q(t)} WHERE " + " AND ".join(f"{q(c)} = ?" for c in pk), tuple(pk.values())).fetchone()
                db.execute(f"DELETE FROM {q(t)} WHERE " + " AND ".join(f"{q(c)} = ?" for c in pk), tuple(pk.values()))
                after, before, action = None, dict(cur, _rowid_=rid[0] if rid else None), "delete"
            else:
                sets = {c: v for c, v in op["after"].items() if c not in pk and cur.get(c) != v}
                db.execute(f"UPDATE {q(t)} SET " + ", ".join(f"{q(c)} = ?" for c in sets) + " WHERE " +
                           " AND ".join(f"{q(c)} = ?" for c in pk), tuple(sets.values()) + tuple(pk.values()))
                after, before, action = fetch(db, t, pk), cur, "update"
                if jn(after) != jn(op["after"]):
                    raise Refused(f"{t} {jn(pk)}: the updated row reads back otherwise; make a new plan")
            a = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                           "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                           (now(), acct, WHO, VIA, action, t, jn(pk),
                            jn({c: v for c, v in before.items() if c != "_rowid_"}) if before is not None else "",
                            jn(after) if after is not None else "",
                            f"{DETAIL} run {run}: {op.get('why', '')}; {TOOL}"[:900])).lastrowid
            db.execute(f"INSERT INTO {LOG} (run, tbl, pk, before, after, audit) VALUES (?, ?, ?, ?, ?, ?)",
                       (run, t, jn(pk), jn(before) if before is not None else None, jn(after) if after is not None else None, a))
            done[(t, action)] += 1
            if after is not None:
                written.append((t, after))
        # the shard as written, before COMMIT: every reference of a written row resolves to the row it came from, and no
        # row names one this run deleted
        rb, rn = ref_problems(db, written + [(op["t"], op["after"]) for i, op in enumerate(ops)
                                             if i not in skip and states[i][0] == "there" and op["after"] is not None],
                              refs.existing(db), deleted_ids(ops, skip), refmap,
                              lambda k, i: fetch(db, refs.KINDS[k][0], {refs.KINDS[k][1]: i}))
        refuse_on(rb, [], "in the shard as written (rolled back)")
        for t, b in seqs.items():
            db.execute(f"INSERT INTO {SEQ} (run, tbl, before, after) VALUES (?, ?, ?, ?)", (run, t, b, seq_of(db, t)))
        db.execute("COMMIT")
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise
    finally:
        db.close()
    print(f"APPLIED run {run} on {shard}: references checked; {sum(done.values())} row changes, {there} already as planned, {len(moves)} id(s) remapped, "
          f"{len(skip)} op(s) left whole; {time.time() - t0:.2f} s")
    for (t, k), n in sorted(done.items()):
        print(f"  {t:20} {k:7} {n}")
    for x in left:
        print("  LEFT", x)
    print(f"UNDO: {sys.argv[0]} --db {dbpath} --shard {shard} --undo --run {run}" + (" --rehearsal" if rehearsal else ""))


def undo(dbpath, shard, run, rehearsal):
    bind(dbpath, shard, rehearsal)
    if not run.endswith("-" + shard):
        raise Refused(f"run {run} is not a {shard} run")
    t0 = time.time()
    db = connect(dbpath)
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    acct = acct[0] if acct else 0
    n = collections.Counter()
    db.execute("BEGIN IMMEDIATE")
    try:
        if not has(db, LOG):
            raise Refused("no runs on this database")
        rows = db.execute(f"SELECT seq, tbl, pk, before, after FROM {LOG} WHERE run = ? AND undone = 0 ORDER BY seq DESC", (run,)).fetchall()
        if not rows:
            raise Refused(f"no undoable rows of run {run} here")
        bad = []
        for seq, t, pk, before, after in rows:
            cur = fetch(db, t, json.loads(pk))
            exp = json.loads(after) if after else None
            if jn(cur) != jn(exp):
                bad.append(f"{t} {pk} (log {seq}) changed since the run")
        if bad:
            raise Refused(f"{len(bad)} row(s) changed since the run; nothing undone:\n  " + "\n  ".join(bad[:20]))
        for seq, t, pk, before, after in rows:
            pk = json.loads(pk)
            before = json.loads(before) if before else None
            after = json.loads(after) if after else None
            where = " AND ".join(f"{q(c)} = ?" for c in pk)
            if before is None:
                db.execute(f"DELETE FROM {q(t)} WHERE {where}", tuple(pk.values()))
                act = "delete"
            elif after is None:
                rid = before.pop("_rowid_", None)
                vals = dict(before)
                if rid is not None and not db.execute(f"SELECT 1 FROM {q(t)} WHERE rowid = ?", (rid,)).fetchone():
                    vals["rowid"] = rid
                cs = list(vals)
                db.execute(f"INSERT INTO {q(t)} ({', '.join(q(c) for c in cs)}) VALUES ({', '.join('?' * len(cs))})",
                           tuple(vals[c] for c in cs))
                act = "insert"
            else:
                sets = [c for c in before if before[c] != after.get(c)]
                if sets:
                    db.execute(f"UPDATE {q(t)} SET " + ", ".join(f"{q(c)} = ?" for c in sets) + f" WHERE {where}",
                               tuple(before[c] for c in sets) + tuple(pk.values()))
                act = "update"
            if jn(fetch(db, t, pk)) != jn(before):
                raise Refused(f"{t} {jn(pk)}: not restored exactly; nothing undone")
            db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                       "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                       (now(), acct, WHO, VIA, act, t, jn(pk), jn(after) if after is not None else "",
                        jn(before) if before is not None else "", f"undo of {DETAIL} run {run}, log {seq}; {TOOL}"[:900]))
            db.execute(f"UPDATE {LOG} SET undone = 1 WHERE seq = ?", (seq,))
            n[(t, act)] += 1
        if has(db, SEQ):
            for t, b, a in db.execute(f"SELECT tbl, before, after FROM {SEQ} WHERE run = ?", (run,)).fetchall():
                if seq_of(db, t) == a:
                    if b is None:
                        db.execute("DELETE FROM sqlite_sequence WHERE name = ?", (t,))
                    else:
                        db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = ?", (b, t))
            db.execute(f"DELETE FROM {SEQ} WHERE run = ?", (run,))
        db.execute("COMMIT")
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise
    finally:
        db.close()
    print(f"UNDONE run {run} on {shard}: {sum(n.values())} rows; {time.time() - t0:.2f} s")
    for (t, a), c in sorted(n.items()):
        print(f"  {t:20} {a:7} {c}")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    if "--check-refs" in sys.argv[1:]:
        sys.exit(refs.main([x for x in sys.argv[1:]]))
    ap.add_argument("--db", required=True)
    ap.add_argument("--shard", required=True, choices=sorted(SIDE))
    ap.add_argument("--leave-unresolved", action="store_true",
                    help="apply although the plan lists rows the planner could not translate (they stay unsynced)")
    ap.add_argument("--plan")
    ap.add_argument("--run")
    ap.add_argument("--rehearsal", action="store_true", help="a copy with no config.yaml beside it")
    g = ap.add_mutually_exclusive_group(required=True)
    g.add_argument("--check", action="store_true")
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    if a.undo:
        if not a.run or a.plan:
            ap.error("--undo takes --run and no --plan")
        undo(a.db, a.shard, a.run, a.rehearsal)
    else:
        if not a.plan or a.run:
            ap.error("--check/--apply take --plan")
        apply(a.db, a.shard, a.plan, a.check, a.rehearsal, a.leave_unresolved)


if __name__ == "__main__":
    try:
        main()
    except (Refused, refs.Untranslatable, sqlite3.Error, OSError, ValueError, KeyError) as e:
        sys.exit(f"REFUSED (nothing written): {type(e).__name__}: {e}")
