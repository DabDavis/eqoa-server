#!/usr/bin/env python3
"""Every class's epic rewards against questmasters (the owner, 2026-11-13: "i like the questmasters website better
jadiction said those were placeholders"), Jadiction only where questmasters gives no figures, and every Ascended epic
held to the server's rule. Run it on each shard's database: everything is found by NAME, never by an id of the other
shard's (the shards' ids diverge: the Ascended axe is 980081 on the Heirloom shard, 980121 on the Live one).

The data is tools/class_epics_20261113.json (made from the sources, each entry quoting its own):

  - questmasters (external/questmasters/text): id417.txt, "Epic Weapons List" (Rangerskikass): both level 49 epic
    weapons of every class; id419.txt, "60 Epic Reward Items List" (Izaa, 2006): the level 60 rewards it gives
    figures for. PRIMARY. A level 60 block was put on most records already by petfix233-247 (2026-11-11, the same
    page): those entries are checked here and match; what is new is the level 49 list, and the Live shard's
    necromancer, which petfix239 never reached (its ids are the Heirloom shard's).
  - jadiction.com (the class epic pages and their item pages, fetched 2026-11-13, which agree with each other and with
    EQOA-Data's JSON): FALLBACK, only for an epic record questmasters names with no figures or not at all, and only
    where Jadiction gives a stat block.

What an entry does to a record (itempattern, every record of that name that is no copy: not in itemascended or
itemimported):
  - its stats: the named ones set; a "whole" entry (a block the source gives entire) sets every stat column, those
    not named 0;
  - dmg: weapondamage; dur: the durability, RAISED only (the owner's level 60 rule, DUR 65 at least: dur65_log; a
    source's lower DUR 50 is left);
  - look_of: the look (model, icon) of the named record ("appearance of 49 monk epic");
  - keep: a column left as it is, with the owner's reason (the ranger's axes stay one-handed at 801).
An entry of source "owner" is the owner's own figures over the sources' (the ranger's level 60 axes, 2026-11-13: "lets
half the stat increase/decrease for the ranger 60 epics (the website meant for both equipped)": 15 STR 15 DEX -25 CHA
113 HP each, a pair given; their Ascended copies 17/17/-25/124 by the rule): they are kept, never put back to
questmasters' 30/30/-50/225.
Its look otherwise, icon, slot, name and description are never touched. Two entries for one record (a record several
classes' lists share) are merged; one that disagrees with another is a problem: nothing is written.

The owner's procs (2026-11-13, applied live by the coordinator: "Procs 250 damage." on the Axe of Nature's Innocence
and the Bow of Sharp Shooting) are checked and kept, never written again.

Records a shard lacks ("new": the Live shard's Staff of the Banshee Wail and Totem of Ancient Reanimation, made on the
Heirloom shard by petfix239) are made from the Heirloom rows, at their Heirloom id when it is free here, else the next
free id of 718001-718999. The reward options ("quests": the necromancer's step 2) are set to the named records'
Ascended ones ("The Ascended <name>"), each made by the rule when it has none.

Ascended (petition 1629 and database/ascended.go, the owner 2026-11-13: "also fix the ascended epics"): every
itemascended record is made its base again by the server's rule (database.SyncAscendedCopies: each stat, AC, resist,
HP/power and their ticks and a weapon's damage a tenth more, half up, a penalty as it is; name "Ascended <base>";
never NO RENT; everything else, the look and the description too, its base's): the copies of the records changed here,
and any copy found off the rule. The proc of an Ascended weapon is its base's, read by name and a tenth more
(entity/ascended.go): nothing to write. The audit also lists records named "Ascended ..." that are no copy, epic rewards
offered without their Ascended record, and options whose answer or record do not match.

Every row written is a gm_audit line (via "tool") and in table class_epics_1113_log for --undo, which puts back each
row still as written (a record a character holds now stays, and is said). Report only by default; --apply and --undo
copy the database first unless --no-backup. A second --apply writes nothing. Items are read at a character's login,
the quests at the server's start (the Live necromancer's options: a keep restart); the start also makes every Ascended
copy its base again, as this does.

    python3 -E -s tools/class_epics_20261113.py --db eqoa.db [--apply | --undo] [--data FILE]
"""
import argparse
import datetime
import json
import os
import re
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
DATA = os.path.join(HERE, "class_epics_20261113.json")
TOOL = "tools/class_epics_20261113.py"
LOG = "class_epics_1113_log"
WHO, VIA = "dab's sessions", "tool"
DETAIL = ("the owner 2026-11-13: every class's epics to questmasters (Jadiction where it gives none), the Ascended "
          "epics to the rule; " + TOOL)

STATS = {"str": "str", "sta": "sta", "agi": "agi", "dex": "dex", "wis": "wis", "int": "intelligence", "cha": "cha",
         "hp": "HPMAX", "pow": "POWMAX", "pot": "PoT", "hot": "HoT", "ac": "AC", "pr": "PR", "dr": "DR", "fr": "FR",
         "cr": "CR", "lr": "LR", "ar": "AR"}
STAT_COLS = list(STATS.values())
NAMES = {v: k.upper() for k, v in STATS.items()}
NAMES.update({"weapondamage": "DMG", "duration": "DUR", "model": "look", "itemicon": "icon"})

# database/ascended.go
PREFIX = "Ascended "
SCALED = {"str", "sta", "agi", "wis", "dex", "cha", "intelligence", "hpmax", "powmax", "pot", "hot", "ac", "pr", "dr",
          "fr", "cr", "lr", "ar", "weapondamage"}
ASC_FIRST, ASC_LAST = 980001, 989999
NEW_FIRST, NEW_LAST = 718001, 718999
BY_RE = re.compile(r"\b([bB][yY] )([0-9]+)")

KEYS = {"itempattern": ("patternid",), "itemeffect": ("patternid",), "itemascended": ("patternid",),
        "queststeps": ("questid", "step")}
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    run     TEXT NOT NULL,
    tbl     TEXT NOT NULL,
    ref     TEXT NOT NULL,      -- the row's key (JSON)
    before  TEXT,               -- the row before (JSON); NULL: made here
    after   TEXT,               -- the row as written (JSON)
    audit   INTEGER
)"""
ASC_SCHEMA = """CREATE TABLE IF NOT EXISTS itemascended (
	patternid INTEGER PRIMARY KEY,
	base      INTEGER NOT NULL UNIQUE,
	made      TEXT NOT NULL DEFAULT '',
	source    TEXT NOT NULL DEFAULT ''
)"""
AUDIT_SCHEMA = """CREATE TABLE IF NOT EXISTS gm_audit (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	at TEXT NOT NULL, accountid INTEGER NOT NULL, username TEXT NOT NULL,
	via TEXT NOT NULL, ip TEXT NOT NULL DEFAULT '', action TEXT NOT NULL,
	tbl TEXT NOT NULL DEFAULT '', rowkey TEXT NOT NULL DEFAULT '',
	before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER
)"""


# ---- small helpers ---------------------------------------------------------------------------------------------------

def figure(v):
    """database.AscendedFigure."""
    if v is None or isinstance(v, str):
        return v
    v = int(v)
    return v if v <= 0 else (v * 110 + 50) // 100


def scaled_line(line):
    if not isinstance(line, str):
        return line
    return BY_RE.sub(lambda m: m.group(1) + str(figure(int(m.group(2)))), line)


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def has_table(db, t):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone()[0] > 0


def columns(db, t):
    return [r[1] for r in db.execute(f'PRAGMA table_info("{t}")')]


def rows(db, sql, args=()):
    cur = db.execute(sql, args)
    names = [d[0] for d in cur.description]
    return [dict(zip(names, r)) for r in cur.fetchall()]


def one(db, sql, args=()):
    r = rows(db, sql, args)
    return r[0] if r else None


def row_of(db, tbl, ref):
    where = " AND ".join(f"{c} = ?" for c in KEYS[tbl])
    return one(db, f"SELECT * FROM {tbl} WHERE {where}", tuple(ref[c] for c in KEYS[tbl]))


def same(col, a, b):
    if col in STAT_COLS or col in ("weapondamage", "duration"):
        return (a or 0) == (b or 0)
    return a == b


def s32(v):
    if v is None:
        return None
    v = int(v) & 0xFFFFFFFF
    return v - (1 << 32) if v >= 1 << 31 else v


def ascended_rule(base, cols):
    """An Ascended record's columns (but patternid) from its base's row (database/ascended.go ascendedColumns)."""
    out = {}
    for c in cols:
        low = c.lower()
        if low == "patternid":
            continue
        v = base[c]
        if low in SCALED:
            if v is None:
                out[c] = None
            else:
                try:
                    iv = int(v)
                except (TypeError, ValueError):
                    out[c] = v
                    continue
                out[c] = (iv * 110 + 50) // 100 if iv > 0 else v
        elif low == "itemname":
            out[c] = PREFIX + (v or "").strip()
        elif low == "rent":
            out[c] = 1 if v == 0 else v
        else:
            out[c] = v
    return out


def ids_in(text):
    """The record ids of an items text ("980083, 980084", "980148*2")."""
    return [int(m) for m in re.findall(r"(?<![0-9*])([0-9]+)", text or "")]


def fmt(col, v):
    return f"{NAMES.get(col, col)} {v}"


# ---- the plan --------------------------------------------------------------------------------------------------------

class Plan:
    def __init__(self):
        self.updates = []      # (patternid, name, {col: (before, after)}, [why])
        self.new = []          # dicts: name, id, row, source
        self.asc_new = []      # (base name, base id or None for a new record's)
        self.asc_sync = []     # (copy, base, {col: (before, after)})
        self.quests = []       # (questid, step, before choices, after choices, source, names)
        self.problems = []
        self.notes = []
        self.kept = []         # (class, name, what, reason)
        self.matched = []      # (class, level, name, source)
        self.owner = []        # (name, ids, present)
        self.audit = []        # Ascended audit lines (nothing written)
        self.byclass = {}      # class -> [(level, name, ids, changes text, source)]

    def changes(self):
        return len(self.updates) + len(self.new) + len(self.asc_new) + len(self.asc_sync) + len(self.quests)


def copies(db):
    a = dict(db.execute("SELECT patternid, base FROM itemascended")) if has_table(db, "itemascended") else {}
    i = set(r[0] for r in db.execute("SELECT patternid FROM itemimported")) if has_table(db, "itemimported") else set()
    return a, i


def records_named(db, name, asc, imp):
    """The records of that name that are no copy (an Ascended or imported one), gear only."""
    return [r for r in rows(db, "SELECT * FROM itempattern WHERE itemname = ? COLLATE NOCASE AND equipslot > 0 "
                                "ORDER BY patternid", (name.strip(),))
            if r["patternid"] not in asc and r["patternid"] not in imp]


def wanted(entry, rec):
    """What an entry asks of a record: {col: value}, and the columns kept with their reasons."""
    keep = entry.get("keep") or {}
    want, kept = {}, []
    st = entry.get("stats") or {}
    if entry.get("whole"):
        for k, c in STATS.items():
            want[c] = st.get(k, 0)
    else:
        for k, v in st.items():
            want[STATS[k]] = v
    if "dmg" in entry:
        want["weapondamage"] = entry["dmg"]
    if "dur" in entry:
        if (rec.get("duration") or 0) < entry["dur"]:
            want["duration"] = entry["dur"]
        elif (rec.get("duration") or 0) > entry["dur"]:
            kept.append(("DUR", f"{rec['duration']} kept over the source's {entry['dur']}: the owner's level 60 DUR 65 "
                                f"rule (dur65_log), raised only"))
    for k, why in keep.items():
        col = "weapondamage" if k == "dmg" else ("duration" if k == "dur" else STATS.get(k, k))
        if col in want:
            if not same(col, rec.get(col), want[col]):
                kept.append((NAMES.get(col, col), f"{rec.get(col)} (source {want[col]}): {why}"))
            del want[col]
    return want, kept


def build_plan(db, data):
    p = Plan()
    asc, imp = copies(db)
    cols = columns(db, "itempattern")
    by_name = {}
    # merge the entries by record name; a disagreement is a problem
    for e in data["items"]:
        by_name.setdefault(e["name"].strip().lower(), []).append(e)
    planned = {}  # patternid -> (name, {col: after}, [sources])
    for low, entries in by_name.items():
        name = entries[0]["name"]
        recs = records_named(db, name, asc, imp)
        if not recs:
            if not any(n["name"].strip().lower() == low for n in data.get("new", [])):
                p.notes.append(f"no record named {name!r} here ({entries[0]['cls']} {entries[0]['level']}): nothing done")
            continue
        for rec in recs:
            merged, why, srcs = {}, {}, []
            for e in entries:
                want, kept = wanted(e, rec)
                for k, v in kept:
                    p.kept.append((e["cls"], name, k, v))
                for c, v in want.items():
                    if c in merged and not same(c, merged[c], v):
                        p.problems.append(f"{name} ({rec['patternid']}): {NAMES.get(c, c)} {merged[c]} ({why[c]}) and "
                                          f"{v} ({e['cls']} {e['source']}) disagree")
                    merged[c], why[c] = v, f"{e['cls']} {e['source']}"
                if e.get("look_of"):
                    o = records_named(db, e["look_of"], asc, imp)
                    if len(o) != 1:
                        p.problems.append(f"{name}: the look of {e['look_of']!r} is {len(o)} records")
                    else:
                        merged["model"], merged["itemicon"] = o[0]["model"], o[0]["itemicon"]
                        why["model"] = why["itemicon"] = f"{e['cls']} {e['source']} (look of {e['look_of']})"
                srcs.append(f"{e['cls']} {e['level']} {e['source']}")
            change = {}
            for c, v in merged.items():
                if c in ("model", "itemicon"):
                    if s32(rec[c]) != s32(v):
                        change[c] = (rec[c], v)
                elif not same(c, rec[c], v):
                    change[c] = (rec[c], v)
            planned[rec["patternid"]] = (name, change, sorted(set(srcs)))
            cls = sorted({e["cls"] for e in entries})
            for c in cls:
                text = "; ".join(f"{NAMES.get(k, k)} {a}->{b}" for k, (a, b) in change.items()) or "matches"
                src = ", ".join(sorted({x["source"] for x in entries if x["cls"] == c}))
                p.byclass.setdefault(c, []).append((entries[0]["level"], name, rec["patternid"], text, src))
            if change:
                p.updates.append((rec["patternid"], name, change, sorted(set(srcs))))
            else:
                p.matched.append((",".join(cls), entries[0]["level"], name, entries[0]["source"]))
    # new records
    used = set(r[0] for r in db.execute("SELECT patternid FROM itempattern"))
    for n in data.get("new", []):
        if records_named(db, n["name"], asc, imp):
            continue
        if one(db, "SELECT patternid FROM itempattern WHERE itemname = ? COLLATE NOCASE", (n["name"],)):
            p.problems.append(f"{n['name']!r}: a record of that name is here, but a copy or no gear")
            continue
        pid = n["prefer_id"] if n["prefer_id"] not in used else None
        if pid is None:
            pid = next((i for i in range(NEW_FIRST, NEW_LAST + 1) if i not in used), None)
            if pid is None:
                p.problems.append(f"no id left in {NEW_FIRST}-{NEW_LAST} for {n['name']!r}")
                continue
        used.add(pid)
        row = {c: n["row"][c] for c in cols if c != "patternid" and c in n["row"]}
        row["patternid"] = pid
        p.new.append({"name": n["name"], "id": pid, "row": row, "source": n["source"]})
    # the owner's procs: kept as they are
    for o in data.get("owner", []):
        recs = records_named(db, o["name"], asc, imp)
        for r in recs:
            p.owner.append((o["name"], r["patternid"], o["words"].lower() in (r["itemdesc"] or "").lower(), o["said"]))
    # the reward options
    new_names = {n["name"].lower(): n for n in p.new}
    for q in data.get("quests", []):
        qs = rows(db, "SELECT questid FROM quests WHERE source LIKE ?", (f"EQOA-Data {q['key']} %",))
        if len(qs) != 1:
            p.problems.append(f"quest {q['key']}: {len(qs)} quests here")
            continue
        st = row_of(db, "queststeps", {"questid": qs[0]["questid"], "step": q["step"]})
        if st is None or not st["choices"]:
            p.problems.append(f"quest {q['key']} step {q['step']}: no choices here")
            continue
        ch = json.loads(st["choices"])
        old = ch.get("options") or []
        says = {}
        for o in old:
            for i in ids_in(o.get("items")):
                says[i] = o.get("says")
        opts, targets = [], []
        for nm in q["options"]:
            recs = records_named(db, nm, asc, imp)
            if len(recs) > 1:
                p.problems.append(f"quest {q['key']}: {len(recs)} records named {nm!r}")
                continue
            base = recs[0]["patternid"] if recs else (new_names.get(nm.lower()) or {}).get("id")
            if base is None:
                p.problems.append(f"quest {q['key']}: no record named {nm!r}")
                continue
            cp = next((c for c, b in asc.items() if b == base), None)
            targets.append((nm, base, cp))
        if p.problems:
            continue
        for nm, base, cp in targets:
            opts.append({"answer": f"The Ascended {nm}", "says": says.get(cp) or "Wear it well.",
                         "items": str(cp) if cp else f"@{base}"})
        if [(o.get("answer"), o.get("items")) for o in old] != [(o["answer"], o["items"]) for o in opts]:
            for nm, base, cp in targets:
                if cp is None:
                    p.asc_new.append((nm, base))
            after = dict(ch, options=opts)
            p.quests.append((qs[0]["questid"], q["step"], st["choices"], after, q["source"], q["options"]))
    # Ascended: every copy its base again by the rule, the records changed here first
    after_rows = {}
    for pid, name, change, _ in p.updates:
        r = one(db, "SELECT * FROM itempattern WHERE patternid = ?", (pid,))
        r.update({c: b for c, (a, b) in change.items()})
        after_rows[pid] = r
    for cp, base in sorted(asc.items()):
        b = after_rows.get(base) or one(db, "SELECT * FROM itempattern WHERE patternid = ?", (base,))
        c = one(db, "SELECT * FROM itempattern WHERE patternid = ?", (cp,))
        if b is None or c is None:
            p.audit.append(f"itemascended {cp} -> {base}: {'no copy record' if c is None else 'no base record'} (left)")
            continue
        want = ascended_rule(b, cols)
        diff = {k: (c[k], v) for k, v in want.items() if c[k] != v and not (c[k] in (None, "") and v in (None, ""))}
        if diff:
            p.asc_sync.append((cp, base, diff))
            if base not in after_rows:
                p.audit.append(f"{c['itemname']} ({cp}) was off the rule of its base {base}: "
                               + "; ".join(f"{NAMES.get(k, k)} {a!r}->{v!r}" for k, (a, v) in diff.items()))
    # the audit's other findings (nothing written)
    mapped = set(asc)
    for r in rows(db, "SELECT patternid, itemname, equipslot FROM itempattern WHERE itemname LIKE 'Ascended %'"):
        if r["patternid"] not in mapped and r["patternid"] not in imp:
            p.audit.append(f"{r['itemname']} ({r['patternid']}, slot {r['equipslot']}) is named Ascended but is no "
                           f"itemascended copy (a 60 epic spell's consumable or a Testing Heroes record; left)")
    names = dict(db.execute("SELECT patternid, itemname FROM itempattern"))
    for qid, step, ch, src in db.execute(
            "SELECT s.questid, s.step, s.choices, q.source FROM queststeps s JOIN quests q USING (questid) "
            "WHERE q.source LIKE 'EQOA-Data 60/%/reward-%' OR q.source LIKE 'EQOA-Data 30,40,49/%/L49-%'"):
        if not ch:
            continue
        for o in json.loads(ch).get("options") or []:
            for i in ids_in(o.get("items")):
                if names.get(i) is None:
                    p.audit.append(f"quest {qid} step {step}: option {o.get('answer')!r} names no record ({i})")
                elif i not in asc and (one(db, "SELECT equipslot FROM itempattern WHERE patternid = ?", (i,)) or {}).get("equipslot", 0) > 0:
                    p.audit.append(f"quest {qid} step {step}: option {o.get('answer')!r} gives {names[i]} ({i}), "
                                   f"no Ascended record")
                elif i in asc and "ascended" not in (o.get("answer") or "").lower():
                    p.audit.append(f"quest {qid} step {step}: option {o.get('answer')!r} gives {names[i]} but does "
                                   f"not say Ascended")
    return p


def holders(db, ids):
    if not ids or not has_table(db, "charinventory"):
        return 0, 0
    q = ",".join("?" * len(ids))
    n, c = db.execute(f"SELECT count(*), count(DISTINCT serverid) FROM charinventory WHERE patternid IN ({q})",
                      list(ids)).fetchone()
    return n, c


def report(db, p):
    out = []
    asc, _ = copies(db)
    inv = {}
    for c, b in asc.items():
        inv.setdefault(b, []).append(c)
    out.append("== per class (record: what changes, the source)")
    for cls in sorted(p.byclass):
        for lvl, name, pid, text, src in sorted(p.byclass[cls]):
            if text != "matches":
                out.append(f"  {cls:12} {lvl} {name} ({pid}; Ascended {', '.join(map(str, inv.get(pid, []))) or '-'}): "
                           f"{text}  [{src}]")
        same_n = sum(1 for x in p.byclass[cls] if x[3] == "matches")
        out.append(f"  {cls:12} {same_n} record(s) match their source already")
    if p.kept:
        out.append("== kept as they are")
        for cls, name, what, why in sorted(set(p.kept)):
            out.append(f"  {cls:12} {name}: {what} {why}")
    if p.new:
        out.append("== records made here")
        for n in p.new:
            out.append(f"  {n['id']} {n['name']}: {n['source']}")
    for nm, base in p.asc_new:
        out.append(f"  an Ascended {nm} made by the rule (of {base if base else 'the new record'})")
    if p.quests:
        out.append("== reward options")
        for qid, step, before, after, src, names in p.quests:
            old = [o.get("answer") for o in json.loads(before).get("options") or []]
            out.append(f"  quest {qid} step {step}: {old} -> {[o['answer'] for o in after['options']]}  [{src}]")
    if p.owner:
        out.append("== the owner's procs (kept, never written)")
        for name, pid, present, said in p.owner:
            out.append(f"  {name} ({pid}): {'present' if present else 'NOT HERE (not added: the coordinator applies it)'}"
                       f" - {said}")
    changed = [pid for pid, *_ in p.updates]
    held = holders(db, changed + [c for b in changed for c in inv.get(b, [])])
    out.append(f"== Ascended: {len(asc)} copies; {len(p.asc_sync)} to make their base again by the rule")
    for cp, base, diff in p.asc_sync:
        out.append(f"  {cp} <- {base}: " + "; ".join(f"{NAMES.get(k, k)} {a!r}->{b!r}" for k, (a, b) in diff.items()))
    if p.audit:
        out.append("== Ascended audit (nothing written for these)")
        out.extend(f"  {x}" for x in p.audit)
    for x in p.notes:
        out.append(f"note: {x}")
    for x in p.problems:
        out.append(f"PROBLEM: {x}")
    out.append(f"held: {held[0]} rows of the changed records and their Ascended copies, by {held[1]} characters "
               f"(a record is shared: they change with it)")
    return out


# ---- writing ---------------------------------------------------------------------------------------------------------

class Writer:
    def __init__(self, db, run):
        self.db, self.run, self.n = db, run, 0
        self.at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

    def audit(self, action, tbl, ref, before, after, detail):
        cur = self.db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, "
                              "after, detail) VALUES (?, 0, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                              (self.at, WHO, VIA, action, tbl, go_json(ref),
                               None if before is None else go_json(before), None if after is None else go_json(after),
                               detail))
        return cur.lastrowid

    def log(self, tbl, ref, before, after, action):
        a = self.audit(action, tbl, ref, before, after, f"{DETAIL} (run {self.run})")
        self.db.execute(f"INSERT INTO {LOG} (run, tbl, ref, before, after, audit) VALUES (?, ?, ?, ?, ?, ?)",
                        (self.run, tbl, go_json(ref), None if before is None else go_json(before),
                         None if after is None else go_json(after), a))
        self.n += 1

    def insert(self, tbl, row):
        cs = list(row)
        self.db.execute(f"INSERT INTO {tbl} ({', '.join(cs)}) VALUES ({', '.join('?' * len(cs))})",
                        [row[c] for c in cs])
        ref = {k: row[k] for k in KEYS[tbl]}
        self.log(tbl, ref, None, row_of(self.db, tbl, ref), "insert")

    def update(self, tbl, ref, cols):
        before = row_of(self.db, tbl, ref)
        cs = list(cols)
        self.db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in cs)} WHERE "
                        + " AND ".join(f"{k} = ?" for k in KEYS[tbl]), [cols[c] for c in cs] + [ref[k] for k in KEYS[tbl]])
        self.log(tbl, ref, before, row_of(self.db, tbl, ref), "update")


def make_ascended(db, wr, base, run):
    cols = columns(db, "itempattern")
    b = one(db, "SELECT * FROM itempattern WHERE patternid = ?", (base,))
    nid = db.execute("SELECT COALESCE(MAX(patternid), ?) + 1 FROM itempattern WHERE patternid BETWEEN ? AND ?",
                     (ASC_FIRST - 1, ASC_FIRST, ASC_LAST)).fetchone()[0]
    if nid > ASC_LAST:
        raise SystemExit("no id left for an Ascended record")
    row = ascended_rule(b, cols)
    row["patternid"] = nid
    wr.insert("itempattern", row)
    if has_table(db, "itemeffect"):
        for e in rows(db, "SELECT * FROM itemeffect WHERE patternid = ?", (base,)):
            e = dict(e, patternid=nid)
            for k in ("line", "target_line"):
                if k in e:
                    e[k] = scaled_line(e[k])
            wr.insert("itemeffect", e)
    wr.insert("itemascended", {"patternid": nid, "base": base, "made": wr.at, "source": f"{TOOL} {run}"})
    return nid


def apply(db, p, run):
    db.execute(LOG_SCHEMA)
    db.execute(AUDIT_SCHEMA)
    db.execute(ASC_SCHEMA)
    wr = Writer(db, run)
    for pid, name, change, srcs in p.updates:
        wr.update("itempattern", {"patternid": pid}, {c: b for c, (a, b) in change.items()})
    for n in p.new:
        wr.insert("itempattern", n["row"])
    made = {}
    for nm, base in p.asc_new:
        if base is None:
            base = next(n["id"] for n in p.new if n["name"].lower() == nm.lower())
        made[base] = make_ascended(db, wr, base, run)
    for cp, base, diff in p.asc_sync:
        wr.update("itempattern", {"patternid": cp}, {c: b for c, (a, b) in diff.items()})
        # its use effects, as SyncAscendedCopies has them
        if has_table(db, "itemeffect"):
            be = one(db, "SELECT * FROM itemeffect WHERE patternid = ?", (base,))
            ce = one(db, "SELECT * FROM itemeffect WHERE patternid = ?", (cp,))
            if be is not None:
                want = dict(be, patternid=cp)
                for k in ("line", "target_line"):
                    if k in want:
                        want[k] = scaled_line(want[k])
                if ce is None:
                    wr.insert("itemeffect", want)
                elif go_json(ce) != go_json(want):
                    wr.update("itemeffect", {"patternid": cp}, {k: v for k, v in want.items() if k != "patternid"})
    for qid, step, before, after, src, names in p.quests:
        now = row_of(db, "queststeps", {"questid": qid, "step": step})
        if now["choices"] != before:
            raise SystemExit(f"queststeps {qid}/{step} changed under the plan")
        asc = dict((b, c) for c, b in db.execute("SELECT patternid, base FROM itemascended"))
        for o in after["options"]:
            if o["items"].startswith("@"):
                o["items"] = str(asc[int(o["items"][1:])])
        wr.update("queststeps", {"questid": qid, "step": step},
                  {"choices": json.dumps(after, separators=(",", ":"), ensure_ascii=False)})
    return wr.n


def held_anywhere(db, pid):
    out = []
    if has_table(db, "charinventory"):
        n = db.execute("SELECT count(*) FROM charinventory WHERE patternid = ?", (pid,)).fetchone()[0]
        if n:
            out.append(f"charinventory {n}")
    num = re.compile(rf"(?<![0-9]){pid}(?![0-9])")
    for qid, step, *texts in db.execute("SELECT questid, step, items, rewarditems, byclass, choices FROM queststeps"):
        if any(num.search(t or "") for t in texts):
            out.append(f"quest {qid} step {step}")
    return out


def undo(db):
    if not has_table(db, LOG):
        return 0, []
    wr = Writer(db, "undo")
    back, left = 0, []
    entries = db.execute(f"SELECT seq, run, tbl, ref, before, after FROM {LOG} ORDER BY seq DESC").fetchall()
    for sq, r, tbl, ref, before, after in entries:
        ref, before = json.loads(ref), (json.loads(before) if before is not None else None)
        after = json.loads(after) if after is not None else None
        now = row_of(db, tbl, ref)
        if before is None:
            if tbl in ("itempattern",):
                h = held_anywhere(db, ref["patternid"])
                if h:
                    left.append(f"itempattern {ref['patternid']} stays: {', '.join(h)}")
                    db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (sq,))
                    continue
            if now is not None:
                if go_json(now) != go_json(after):
                    left.append(f"{tbl} {go_json(ref)}: changed since it was written, taken away all the same")
                db.execute(f"DELETE FROM {tbl} WHERE " + " AND ".join(f"{c} = ?" for c in KEYS[tbl]),
                           tuple(ref[c] for c in KEYS[tbl]))
                wr.audit("delete", tbl, ref, now, None, f"undo: {DETAIL} (run {r})")
            back += 1
        elif now is None or go_json(now) != go_json(after):
            left.append(f"{tbl} {go_json(ref)}: changed since it was written, left")
        else:
            cs = [c for c in before if c not in KEYS[tbl]]
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in cs)} WHERE "
                       + " AND ".join(f"{c} = ?" for c in KEYS[tbl]),
                       [before[c] for c in cs] + [ref[c] for c in KEYS[tbl]])
            wr.audit("update", tbl, ref, now, row_of(db, tbl, ref), f"undo: {DETAIL} (run {r})")
            back += 1
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (sq,))
    if not db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0]:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def _backup(path):
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-class-epics"
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
    ap.add_argument("--data", default=DATA)
    ap.add_argument("--no-backup", action="store_true")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write it")
    g.add_argument("--undo", action="store_true", help="put back what --apply wrote")
    a = ap.parse_args(argv)
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    data = json.load(open(a.data, encoding="utf-8"))
    db = connect(a.db)
    try:
        if a.undo:
            if not a.no_backup:
                print(f"copy: {_backup(a.db)}")
            db.execute("BEGIN IMMEDIATE")
            try:
                db.execute(AUDIT_SCHEMA)
                back, left = undo(db)
                db.execute("COMMIT")
            except BaseException:
                db.execute("ROLLBACK")
                raise
            for x in left:
                print("left:", x)
            print(f"put back {back} rows; {len(left)} left as they are; restart the server (the quests)")
            return 0
        p = build_plan(db, data)
        for line in report(db, p):
            print(line)
        n = p.changes()
        if not a.apply:
            print(f"{n} changes: report only, --apply writes them")
            return 2 if p.problems else 0
        if p.problems:
            print(f"nothing written: {len(p.problems)} problems")
            return 2
        if n == 0:
            print("nothing to do")
            return 0
        if not a.no_backup:
            print(f"copy: {_backup(a.db)}")
        run = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%S.%fZ")
        db.execute("BEGIN IMMEDIATE")
        try:
            p = build_plan(db, data)
            if p.problems:
                raise SystemExit(f"nothing written: {len(p.problems)} problems (the database changed)")
            wrote = apply(db, p, run)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"run {run}: wrote {wrote} rows (each in gm_audit); items are read at login, the quests at the server's "
              f"start (a keep restart when a quest changed); --undo takes it back")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
