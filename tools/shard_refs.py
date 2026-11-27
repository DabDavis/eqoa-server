#!/usr/bin/env python3
"""The content tables' cross-row references, and a check that every one names a row that exists (2026-11-27).

Why (Heirloom petition 3352): a GM trigger names its spawn templates by their npcs row as "#<id>" inside gmtriggers.steps.
The shard syncs copied that text verbatim from one shard to the other while each shard holds the same templates under its
own ids, so Heirloom's triggers 150 (The Trial of Celerity) and 161 (The Froglok Herbalist) named Live's rows from
2026-11-23 01:33 UTC on, and their spawns did nothing (the 11-15 sync had done the same to Live's copy of trigger 150).
The sync's engine (shard_sync_20261115.py) now translates these references (its "jtext" kind) and the applier refuses a
plan whose rows would leave one that misses; this module is the registry both use and the check:

  python3 -E -s shard_refs.py --check-refs --db ~/eqoa-server/eqoa.db       # read only; exit 1 when any reference misses
  python3 -E -s shard_refs.py --check-refs --db ... --json misses.json       # every miss, for a tool

Each reference is (kind, id): npc = an npcs row (id), item = itempattern.patternid, spell = spellpattern.spellid,
quest = quests.questid, slot = gmtriggers.slot. The sites:
  INT_SITES     an integer column that holds one id (0 / NULL: none)
  ROWKEY_SITES  a text column whose whole value "#<id>" names an npcs row (a per-row override: loot, template, ...)
  LIST_SITES    a text column holding a list of ids ("701447, 701450", "100793|100794", "716105*2": the id before a '*')
  JSON_SITES    a JSON text column read by key: any string "#<id>" is an npcs row; "quest", "spell", "spell_id", "item"
                (an integer) and "items", "spells" (an id list in a string) by their key; "slot" in gmtriggers.steps (a stop
                step's trigger; 0 = every other). Coordinates, counts and the rest are never read as ids.
A miss is a reference whose row does not exist on the database. Standard library only (the applier runs on the box).
"""
import argparse
import collections
import json
import re
import sqlite3
import sys
from pathlib import Path

KINDS = {"npc": ("npcs", "id"), "item": ("itempattern", "patternid"), "spell": ("spellpattern", "spellid"),
         "quest": ("quests", "questid"), "slot": ("gmtriggers", "slot")}
MINT_KIND = {t: k for k, (t, c) in KINDS.items()}

_NPC_INT = ("npcrespawn npc_row|npcwander npcid|npcpaths npcid|npcrare npc_row|npctitle npcid|signtext npcid|campspawns npcid|"
            "npcstands npc_row|npcitem npcid|questwalls npcid|npcrarecycle npc_row|questdoors npcid|queststeps npcid|"
            "gmportals npc_id|gmportals exit_npc_id|npcpets template_row")
_ITEM_INT = ("itemeffect itemaura itemfactions foodeffect gemsetting heldbarred norentvanish itemspeed itemhide itemspell "
             "lootdroptrigger itemoutfit itemascended itemimported heirlooms familiars npcitem tokenshop loottables lootpool "
             "npcheld merchantprice startitems heroes_stock defaultcharinventory")
INT_SITES = ([(t, c, "npc") for t, c in (x.split() for x in _NPC_INT.split("|"))] +
             [(t, "patternid", "item") for t in _ITEM_INT.split()] +
             [("questdoors", "item", "item"), ("questwalls", "item", "item"), ("itemascended", "base", "item"),
              ("itemimported", "base", "item"), ("fishcatch", "itemid", "item"), ("craftrecipes", "scroll", "item")] +
             [(t, "spellid", "spell") for t in ("spelleffect", "spellarea", "petstats", "spellrecastgroup", "classspells",
                                                "spellascended", "itemspell", "defaultspells")] +
             [("spellascended", "base", "spell")] +
             [(t, "questid", "quest") for t in ("queststeps", "queststeptargets", "questunlocks", "questwalls", "questmarks",
                                                "questdoors", "itemhide")])
ROWKEY_SITES = [("npcloot", "npc_name"), ("npctemplate", "name"), ("npcspawnonly", "name"), ("lootcreature", "npc_name"),
                ("npctime", "name"), ("npcpets", "owner")]
LIST_SITES = [("queststeps", "items", "item"), ("queststeps", "rewarditems", "item"), ("queststeps", "spells", "spell"),
              ("quests", "requires", "quest"), ("craftrecipes", "product", "item"), ("cmcatalog", "require_spells", "spell"),
              ("cmcatalog", "add_spells", "spell"), ("cmcatalog", "remove_spells", "spell"),
              ("cmcatalog", "require_quests", "quest")]
JSON_SITES = [("gmtriggers", "steps"), ("queststeps", "events"), ("quests", "events"), ("queststeps", "byclass"),
              ("queststeps", "choices"), ("npctalk", "choices"), ("craftrecipes", "ingredients"),
              ("classtemplates", "abilities"), ("petstats", "abilities")]
SLOT_SITES = {("gmtriggers", "steps")}
SCALAR_KEYS = {"quest": "quest", "spell": "spell", "spell_id": "spell", "item": "item", "slot": "slot"}
LIST_KEYS = {"items": "item", "spells": "spell"}

ROWKEY = re.compile(r"^#(\d+)$")
_J_NPC = re.compile(r'(?<!\\)"#(\d+)"')
_J_SCALAR = re.compile(r'(?<![\\\w])"(quest|spell|spell_id|item|slot)"(\s*:\s*)(-?\d+)(?![\d.eE])')
_J_LIST = re.compile(r'(?<![\\\w])"(items|spells)"(\s*:\s*)"([^"\\]*)"')
_L_ID = re.compile(r"(?<![\w.*])(\d+)(?![\w.])")


class Untranslatable(Exception):
    """a reference the caller's mapping cannot carry to the other shard"""


def q(c):
    return '"' + c.replace('"', '""') + '"'


def site_kinds():
    """{(table, column): 'int'|'rowkey'|'list'|'json'}"""
    out = {}
    for t, c, _ in INT_SITES:
        out[(t, c)] = "int"
    for t, c in ROWKEY_SITES:
        out[(t, c)] = "rowkey"
    for t, c, _ in LIST_SITES:
        out[(t, c)] = "list"
    for t, c in JSON_SITES:
        out[(t, c)] = "json"
    return out


SITE = site_kinds()
INT_KIND = {(t, c): k for t, c, k in INT_SITES}
LIST_KIND = {(t, c): k for t, c, k in LIST_SITES}


def _jmap(o, slot_ok, f, path=""):
    """o (parsed JSON) with every reference replaced by f(kind, id, where); f returns what stands in its place (for an
    id list key, the replacement goes into the list's string)"""
    if isinstance(o, dict):
        out = {}
        for k, x in o.items():
            p = f"{path}.{k}" if path else str(k)
            if isinstance(x, str) and ROWKEY.match(x):
                out[k] = f("npc", int(x[1:]), p)
            elif isinstance(x, str) and k in LIST_KEYS:
                out[k] = _L_ID.sub(lambda m: str(f(LIST_KEYS[k], int(m.group(1)), p)) if int(m.group(1)) else m.group(1), x)
            elif isinstance(x, int) and not isinstance(x, bool) and k in SCALAR_KEYS:
                kind = SCALAR_KEYS[k]
                out[k] = f(kind, x, p) if x and (kind != "slot" or slot_ok) else x
            else:
                out[k] = _jmap(x, slot_ok, f, p)
        return out
    if isinstance(o, list):
        return [f("npc", int(x[1:]), f"{path}[{i}]") if isinstance(x, str) and ROWKEY.match(x) else
                _jmap(x, slot_ok, f, f"{path}[{i}]") for i, x in enumerate(o)]
    return o


def _json_refs(v, slot_ok):
    """[(kind, id, where)] read from parsed JSON"""
    out = []

    def f(kind, i, where):
        out.append((kind, i, where))
        return i

    _jmap(v, slot_ok, f)
    return out


def normalize(table, col, value, tag):
    """a JSON site's value as one canonical string with each reference replaced by tag(kind, id) (for comparing the
    shards' rows by what their references point at); a value that is not JSON is returned as it is"""
    try:
        parsed = json.loads(value)
    except (TypeError, ValueError):
        return value
    return "J" + json.dumps(_jmap(parsed, (table, col) in SLOT_SITES, lambda k, i, w: tag(k, i)), separators=(",", ":"))


def _list_ids(s):
    return [int(m.group(1)) for m in _L_ID.finditer(s) if int(m.group(1))]


def refs_in(table, col, value):
    """[(kind, id, where)]: the references a value of table.col holds (none for a column that is not a site)"""
    site = SITE.get((table, col))
    if site is None or value is None or value == "":
        return []
    if site == "int":
        if isinstance(value, bool) or not isinstance(value, int) or not value:
            return []
        return [(INT_KIND[(table, col)], value, col)]
    if not isinstance(value, str):
        return []
    if site == "rowkey":
        m = ROWKEY.match(value)
        return [("npc", int(m.group(1)), col)] if m else []
    if site == "list":
        return [(LIST_KIND[(table, col)], i, col) for i in _list_ids(value)]
    try:
        parsed = json.loads(value)
    except ValueError:
        return []
    return [(k, i, f"{col}:{w}") for k, i, w in _json_refs(parsed, (table, col) in SLOT_SITES)]


def rewrite(table, col, value, fn):
    """value with every reference (kind, id) replaced by fn(kind, id) (fn returns the new id, or raises Untranslatable);
    the text is otherwise kept as it is (no JSON re-serialisation). A JSON value whose rewritten references do not read
    back as the mapped ones refuses (Untranslatable), so nothing outside a reference is ever touched."""
    site = SITE.get((table, col))
    if site is None or value is None or value == "":
        return value
    memo, fn0 = {}, fn

    def fn(kind, i):
        if (kind, i) not in memo:
            memo[(kind, i)] = fn0(kind, i)
        return memo[(kind, i)]

    if site == "int":
        if isinstance(value, bool) or not isinstance(value, int) or not value:
            return value
        return fn(INT_KIND[(table, col)], value)
    if not isinstance(value, str):
        return value
    if site == "rowkey":
        m = ROWKEY.match(value)
        return f"#{fn('npc', int(m.group(1)))}" if m else value
    if site == "list":
        kind = LIST_KIND[(table, col)]
        return _L_ID.sub(lambda m: str(fn(kind, int(m.group(1)))) if int(m.group(1)) else m.group(1), value)
    try:
        parsed = json.loads(value)
    except ValueError:
        return value
    slot_ok = (table, col) in SLOT_SITES
    want = [(k, fn(k, i)) for k, i, _ in _json_refs(parsed, slot_ok)]
    if not want:
        return value

    def scalar(m):
        kind = SCALAR_KEYS[m.group(1)]
        if (kind == "slot" and not slot_ok) or not int(m.group(3)):
            return m.group(0)
        return f'"{m.group(1)}"{m.group(2)}{fn(kind, int(m.group(3)))}'

    def lst(m):
        kind = LIST_KEYS[m.group(1)]
        inner = _L_ID.sub(lambda n: str(fn(kind, int(n.group(1)))) if int(n.group(1)) else n.group(1), m.group(3))
        return f'"{m.group(1)}"{m.group(2)}"{inner}"'

    out = _J_NPC.sub(lambda m: f'"#{fn("npc", int(m.group(1)))}"', value)
    out = _J_SCALAR.sub(scalar, out)
    out = _J_LIST.sub(lst, out)
    try:
        got = [(k, i) for k, i, _ in _json_refs(json.loads(out), slot_ok)]
    except ValueError:
        raise Untranslatable(f"{table}.{col}: the rewritten text is not JSON")
    if got != want:
        raise Untranslatable(f"{table}.{col}: its references could not be rewritten in place ({len(want)} wanted, read back "
                             f"{len(got)}): make the change by hand")
    return out


# ---- the check -------------------------------------------------------------------------------------------------------
def has(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone() is not None


def columns(db, t):
    return [r[1] for r in db.execute(f"PRAGMA table_info({q(t)})")]


def pkcols(db, t):
    pk = [r[1] for r in sorted(db.execute(f"PRAGMA table_info({q(t)})"), key=lambda r: r[5]) if r[5]]
    return pk or ["rowid"]


def existing(db):
    """{kind: set of ids the database holds}"""
    out = {}
    for kind, (t, c) in KINDS.items():
        out[kind] = {r[0] for r in db.execute(f"SELECT {q(c)} FROM {q(t)}")} if has(db, t) else set()
    return out


def _label(t, row):
    for c in ("name", "npc_name", "itemname", "spellname"):
        if c in row and row[c]:
            return str(row[c])[:60]
    return ""


def scan(db, ids=None, skip=None, extra=None):
    """every miss, as dicts {table, pk, label, col, kind, id, where}. ids: {kind: set} the rows that exist (default: the
    database's); skip: {table: set of pk json} rows not read from the database (a plan replaces or deletes them); extra:
    {table: [row dicts]} rows read in their place (a plan's after rows)."""
    ids = existing(db) if ids is None else ids
    skip, extra = skip or {}, extra or {}
    misses = []
    for t in sorted({t for t, _ in SITE}):
        if not has(db, t):
            continue
        have = set(columns(db, t))
        cs = [c for c in sorted({c for tt, c in SITE if tt == t}) if c in have]
        if not cs:
            continue
        pk = pkcols(db, t)
        sel = ", ".join(q(c) for c in dict.fromkeys(pk + cs + [c for c in ("name", "npc_name", "itemname") if c in have]))
        cur = db.execute(f"SELECT {sel} FROM {q(t)}")
        names = [d[0] for d in cur.description]
        sk = skip.get(t) or set()

        def look(row, src):
            key = pkjson({c: row.get(c) for c in pk})
            for c in cs:
                for kind, i, where in refs_in(t, c, row.get(c)):
                    if i not in ids[kind]:
                        misses.append({"table": t, "pk": key, "label": _label(t, row), "col": c, "kind": kind, "id": i,
                                       "where": where, "src": src})

        for r in cur:
            row = dict(zip(names, r))
            if sk and pkjson({c: row.get(c) for c in pk}) in sk:
                continue
            look(row, "db")
        for row in extra.get(t, []):
            look(row, "plan")
    return misses


def pkjson(d):
    return json.dumps(d, sort_keys=True, separators=(",", ":"), default=str)


def misskey(m):
    return (m["table"], m["pk"], m["col"], m["kind"], m["id"], m["where"])


def describe(m):
    t, c = KINDS[m["kind"]]
    lab = f" {m['label']!r}" if m["label"] else ""
    return f"{m['table']} {m['pk']}{lab} {m['where']}: {m['kind']} {m['id']} -> no {t} row {c} = {m['id']}"


def report(misses, out=None, limit=60):
    out = out or sys.stdout
    by = collections.Counter((m["table"], m["col"], m["kind"]) for m in misses)
    for (t, c, k), n in sorted(by.items()):
        print(f"  {t}.{c} -> {k}: {n} miss(es)", file=out)
    for m in misses[:limit]:
        print("    " + describe(m), file=out)
    if len(misses) > limit:
        print(f"    ... and {len(misses) - limit} more", file=out)


def connect_ro(path):
    return sqlite3.connect(Path(path).resolve().as_uri() + "?mode=ro", uri=True, timeout=30)


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--check-refs", action="store_true", required=True, help="check every reference of the database")
    ap.add_argument("--db", required=True)
    ap.add_argument("--json", help="write every miss to this file")
    ap.add_argument("--limit", type=int, default=60, help="misses listed (default 60)")
    a = ap.parse_args(argv)
    db = connect_ro(a.db)
    db.execute("BEGIN")   # one read snapshot
    try:
        ids = existing(db)
        misses = scan(db, ids)
        n = sum(len(v) for v in ids.values())
    finally:
        db.execute("ROLLBACK")
        db.close()
    if a.json:
        Path(a.json).write_text(json.dumps(misses, indent=0))
    sites = sum(1 for _ in SITE)
    if not misses:
        print(f"REFS OK {a.db}: every reference of {sites} sites resolves ({n} rows of npcs/items/spells/quests/triggers)")
        return 0
    print(f"REFS FAIL {a.db}: {len(misses)} reference(s) name no row")
    report(misses, limit=a.limit)
    return 1


if __name__ == "__main__":
    sys.exit(main())
