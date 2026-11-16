#!/usr/bin/env python3
"""Item records' missing stats filled from Prima's regional loot tables (the owner, 2026-11-16: "Fill the gaps: give
our records Prima's values where ours are 0 or missing, and leave alone anything we've deliberately changed. These
include the crafting gems from Innoruuk's petitions, the heirlooms and Jib's fixes. A data tool with undo, rehearsed
on copies, and both shards.").

Source: EQOA_Prima_Items.sqlite (table prima_items: Prima's Frontiers guide, pp. 448-652, one row per printed entry;
~/.cache/prima-items-20261116/). Matched as its compare_items.py matches, by name (NFKC, case, apostrophes and
spacing), on the shard given: item patternids differ between shards, so each shard is matched on its own. A Prima row
is used only when its name is printed once in Prima, exactly one itempattern record of this shard has that name, and
the row carries no review_note (every note there flags a parse: "unparsed equipment fields", "equip level above 60").

A field is filled only where our record holds 0 or NULL and Prima prints a positive number. A nonzero value is never
changed. Fields: levelreq, weapondamage, str, sta, agi, wis, dex, cha, intelligence, HPMAX, POWMAX, PoT, HoT, AC, PR,
DR, FR, CR, LR, AR. Not maxhp or durability (their encodings are uncertain). weapondamage IS filled:
itempattern.weapondamage is the item's DMG (the tooltip's VIItem::m_damage, the number the offence roll uses; the GM
catalog's "Weapon DMG"), and Prima's damage figure equals it on every record where both are printed but twelve
(claude-notes/weapondamage-is-a-refmap-20261021.md is about a VIRefMap in RAM, not this column).

Left alone, the whole record (each listed with its reason):
  - any itempattern change in gm_audit, by anyone (GMs, jibrish, dab's sessions, tools); rowkeys that name a tool's
    log table are read through it. Not this tool's own lines. Inserts before 2026-10-25 would be exempt (none exist).
  - any tool log (*_log, bak_*) row on the record that touched a compared field or maxhp, or whose columns are not
    known; logs proved to touch only other columns (value, description, look, class/race masks, slot, reach, stack,
    duration) do not exclude.
  - tonight's crafting fixes: heartsblood_5196_log, two_stat_5198_log, rallosian_5200_log, the gem items (Heartsblood,
    Mindflow, Enstatite, Takish Ivory, Marr Blessed, Lazulite, Rallosian, by name) and every recipe product of a gem.
  - heirlooms: a name starting "Heirloom", patternid 714000-714999, table heirlooms, any record bound in itemowner.
  - jibrish's work-board entries: a record whose id (or an enumerated id range of at most 50) or name appears in one.
  - custom records: patternid >= 700000, and the tables of made records (itemascended, itemimported, itemaura,
    quest_import_items, heroes_stock, familiars, potion_seed, itemhide).
  - a record whose stats do not sit where Prima prints them: a nonzero stat of ours in a field Prima leaves blank
    (the old dump's slide: Icebone Ring holds Str 100 Sta 50 where Prima prints HP 100 AC 50). A fill there would
    give both; such a record needs its stats moved, which is not this tool's to decide.
  - with --other-db (the other shard, a copy): a record of the same name left alone there as deliberately changed
    (not for its id, a made-record table or its own stats' slide, which are each shard's own).

Report only by default, with a CSV of every change (--csv) and one of every exclusion (<csv>-excluded.csv). --apply in
one transaction, a gm_audit line a record (the whole row before and after, as the panel writes), logged in
prima_fill_20261116_log; a second --apply finds nothing to do; --undo puts back exactly what was there (a record changed
since is left and listed) and marks the apply's audit lines reverted. --hash prints a digest of table itempattern.

After --apply: no restart. A character's items are read at its login, so a holder sees the new stats at the next login
(eject to apply now); merchants and drops read the record when they make an item. Imported copies (itemimported) are
made their bases again at the next server start.

    python3 -E -s tools/prima_fill_gaps_20261116.py --db eqoa.db --prima EQOA_Prima_Items.sqlite \\
        [--other-db other-shard-copy.db] [--csv out.csv] [--apply | --undo | --hash]
"""
import argparse
import collections
import csv
import datetime
import hashlib
import json
import os
import re
import sqlite3
import sys
import unicodedata

TOOL = "tools/prima_fill_gaps_20261116.py"
LOG = "prima_fill_20261116_log"
DETAIL = ("Prima's regional loot tables fill item stats that were 0 or missing; nothing nonzero changed, deliberately "
          "changed records left (the owner, 2026-11-16); " + TOOL)
WHO, VIA = "dab", "tool"

FIELDS = ["levelreq", "weapondamage", "str", "sta", "agi", "wis", "dex", "cha", "intelligence", "HPMAX", "POWMAX",
          "PoT", "HoT", "AC", "PR", "DR", "FR", "CR", "LR", "AR"]
STATS = FIELDS[2:]  # the stat and resist columns (not levelreq, weapondamage)
GUARDED = set(FIELDS) | {"maxhp"}  # a log that touched one of these leaves the record alone

# Tool logs whose rows name a record but were checked to write none of GUARDED (column -> what it wrote).
# Logs with before/after JSON or a col/field column are judged row by row instead.
HARMLESS_LOGS = {
    "item_value_fill_log": "itemcost",
    "price_zero_1115_log": "itemcost",
    "item_desc_fill_log": "itemdesc",
    "style0_look_fix_log": "itemicon, model",
    "item_look_fix_log": "itemicon, model",
    "hand_models_1114_log": "model",
    "bardepic_2155_log": "model",
    "dur65_log": "duration",
    "quest_stacks_1114_log": "maxstack, lore",
    "item_stack_fix_log": "maxstack",
    "drinks_5143_log": "maxstack",
    "heirloom_icons_log": "itemicon",
}
CRAFT_LOGS = ("heartsblood_5196_log", "two_stat_5198_log", "rallosian_5200_log")
GEMS = ("heartsblood", "mindflow", "enstatite", "takish ivory", "marr blessed", "lazulite", "rallosian")
MADE_TABLES = ("itemascended", "itemimported", "itemaura", "quest_import_items", "heroes_stock", "familiars",
               "potion_seed", "itemhide")
CUSTOM_FROM = 700000
HEIRLOOM_IDS = (714000, 714999)
IMPORT_EXEMPT_BEFORE = "2026-10-25"

LOG_DDL = f"""CREATE TABLE {LOG} (
    seq      INTEGER PRIMARY KEY AUTOINCREMENT,
    rowkey   TEXT NOT NULL,      -- {{"patternid": n}}
    fields   TEXT NOT NULL,      -- the columns filled
    guide_id INTEGER NOT NULL,   -- prima_items.guide_id
    page     INTEGER NOT NULL,   -- Prima's printed page
    before   TEXT NOT NULL,      -- the row before (JSON)
    after    TEXT NOT NULL,      -- the row written (JSON)
    audit    INTEGER NOT NULL
)"""


def name_key(name):
    text = unicodedata.normalize("NFKC", name or "").replace("’", "'").replace("‘", "'")
    return " ".join(text.casefold().split())


def words_of(text):
    """A text's words, casefolded, apostrophes kept inside a word: the board's names are matched word for word."""
    return re.sub(r"[^\w']+", " ", name_key(text).replace("_", " ")).replace(" '", " ").replace("' ", " ").split()


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def columns(db, name):
    return [r[1] for r in db.execute(f'PRAGMA table_info("{name}")')]


def row(db, tbl, key):
    where = " AND ".join(f"{k} = ?" for k in key)
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", tuple(key.values()))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return None if m is None else json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, tbl, key, action, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key), go_json(before), go_json(after), DETAIL))
    return cur.lastrowid


def put(db, key, want):
    before = row(db, "itempattern", key)
    db.execute(f"UPDATE itempattern SET {', '.join(f'{k} = ?' for k in want)} WHERE patternid = ?",
               tuple(want.values()) + (key["patternid"],))
    after = row(db, "itempattern", key)
    return before, after, audit(db, "itempattern", key, "update", before, after)


def itempattern_hash(db):
    h = hashlib.sha256()
    for r in db.execute("SELECT * FROM itempattern ORDER BY patternid"):
        h.update(repr(r).encode())
    return h.hexdigest()


def ints(text):
    return [int(m) for m in re.findall(r"(?<![\d.])\d+(?![\d.])", "" if text is None else str(text))]


def json_ids(text):
    """patternid / rowid values anywhere in a JSON document."""
    out = set()
    if isinstance(text, int):
        return {text}
    try:
        doc = json.loads(text)
    except (TypeError, ValueError):
        return out
    stack = [doc]
    while stack:
        v = stack.pop()
        if isinstance(v, dict):
            for k, x in v.items():
                if k in ("patternid", "rowid", "product", "item") and isinstance(x, int):
                    out.add(x)
                stack.append(x)
        elif isinstance(v, list):
            stack.extend(v)
    return out


class Excluded:
    """patternid -> [reasons]; reasons are 'category: detail'."""

    def __init__(self):
        self.why = collections.defaultdict(list)

    def add(self, pid, reason):
        if pid is not None and reason not in self.why[pid]:
            self.why[pid].append(reason)


def log_table_ids(db, t):
    """{patternid: touched columns or None (unknown)} of a log table's itempattern rows."""
    cols = columns(db, t)
    out = collections.defaultdict(set)
    if "tbl" in cols and "rowkey" in cols:
        colcol = "col" if "col" in cols else ("cols" if "cols" in cols else None)
        sel = ["tbl", "rowkey"] + [c for c in ("before", "after") if c in cols] + ([colcol] if colcol else [])
        for r in db.execute(f'SELECT {", ".join(sel)} FROM "{t}"'):
            r = dict(zip(sel, r))
            if "itempattern" not in str(r["tbl"] or ""):
                continue
            ids = json_ids(r["rowkey"]) or set(ints(r["rowkey"]))
            touched = set()
            if colcol and r.get(colcol):
                touched |= set(re.split(r"[,\s]+", str(r[colcol]))) - {""}
            for side in ("before", "after"):
                try:
                    d = json.loads(r.get(side) or "null")
                except ValueError:
                    d = None
                if isinstance(d, dict):
                    touched |= set(d)
            for pid in ids:
                out[pid] |= touched if touched else {"?"}
    elif "patternid" in cols:
        js = [c for c in ("before", "after") if c in cols]
        fieldcol = "field" if "field" in cols else ("col" if "col" in cols else None)
        sel = ["patternid"] + js + ([fieldcol] if fieldcol else [])
        for r in db.execute(f'SELECT {", ".join(sel)} FROM "{t}"'):
            r = dict(zip(sel, r))
            touched = set()
            for side in js:
                try:
                    d = json.loads(r[side] or "null")
                except (TypeError, ValueError):
                    d = None
                if isinstance(d, dict):
                    touched |= set(d)
            if fieldcol and r[fieldcol]:
                touched.add(str(r[fieldcol]))
                if t == "armor_class_fix_log":
                    touched.add("AC")
            if t in HARMLESS_LOGS and not touched:
                touched = {c.strip() for c in HARMLESS_LOGS[t].split(",")}
            out[r["patternid"]] |= touched if touched else {"?"}
    return out


def deliberate(db, names):
    """Excluded of every record this shard has deliberately changed (names: patternid -> itemname)."""
    ex = Excluded()
    by_name = collections.defaultdict(set)
    for pid, n in names.items():
        by_name[name_key(n)].add(pid)
    tables = {r[0] for r in db.execute("SELECT name FROM sqlite_master WHERE type = 'table'")}

    # gm_audit
    if "gm_audit" in tables:
        for at, user, via, action, rowkey, before, after, detail in db.execute(
                "SELECT at, username, via, action, rowkey, before, after, detail FROM gm_audit "
                "WHERE tbl LIKE '%itempattern%' ORDER BY id"):
            if TOOL in (detail or ""):
                continue
            if action == "insert" and at < IMPORT_EXEMPT_BEFORE:
                continue
            who = f"gm_audit {user} ({via})"
            ids = set()
            rk = (rowkey or "").strip()
            if rk in tables:
                ids |= set(log_table_ids(db, rk))
            else:
                ids |= json_ids(rk) or set(ints(rk))
                try:
                    d = json.loads(rk)
                except ValueError:
                    d = None
                if isinstance(d, dict) and "itemname" in d:
                    ids |= by_name.get(name_key(d["itemname"]), set())
                if isinstance(d, dict) and "set" in d:
                    ids |= {p for p, n in names.items() if str(d["set"]).casefold() in n.casefold()}
                if not d and "/" in rk:  # "bandit sash / 2369"
                    ids |= by_name.get(name_key(rk.split("/")[0]), set())
            for side in (before, after):
                ids |= json_ids(side)
            for pid in ids:
                if pid in names:
                    ex.add(pid, who)

    # tool logs
    for t in sorted(tables):
        if t == LOG or not (t.endswith("_log") or t.startswith("bak_") or t.endswith("_bak_npcitem")):
            continue
        if t in CRAFT_LOGS:
            for pid in log_table_ids(db, t):
                if pid in names:
                    ex.add(pid, f"crafting fix {t}")
            continue
        for pid, touched in log_table_ids(db, t).items():
            if pid not in names:
                continue
            hit = touched & GUARDED
            if "?" in touched:
                ex.add(pid, f"tool log {t} (columns not known)")
            elif hit:
                ex.add(pid, f"tool log {t} ({', '.join(sorted(hit))})")

    # crafting gems and their products
    gem_ids = set()
    for pid, n in names.items():
        k = name_key(n)
        if any(g in k for g in GEMS):
            gem_ids.add(pid)
            ex.add(pid, "crafting gem by name")
    if "craftrecipes" in tables and gem_ids:
        for product, ings in db.execute("SELECT product, ingredients FROM craftrecipes"):
            if json_ids(ings) & gem_ids and product in names:
                ex.add(product, "crafting gem's product")

    # heirlooms
    for pid, n in names.items():
        if name_key(n).startswith("heirloom"):
            ex.add(pid, "heirloom by name")
        if HEIRLOOM_IDS[0] <= pid <= HEIRLOOM_IDS[1]:
            ex.add(pid, "heirloom id range")
    for t, why in (("heirlooms", "heirloom table"), ("itemowner", "bound in itemowner")):
        if t in tables and "patternid" in columns(db, t):
            for (pid,) in db.execute(f"SELECT DISTINCT patternid FROM {t}"):
                if pid in names:
                    ex.add(pid, why)

    # custom records
    for pid in names:
        if pid >= CUSTOM_FROM:
            ex.add(pid, f"custom id >= {CUSTOM_FROM}")
    for t in MADE_TABLES:
        if t in tables:
            cols = columns(db, t)
            if "patternid" in cols:  # itemimported: the copy, not its base (a copy is made its base at each start)
                for (pid,) in db.execute(f"SELECT DISTINCT patternid FROM {t}"):
                    if pid in names:
                        ex.add(pid, f"made record ({t})")

    # jibrish's work board
    if "workboard" in tables:
        by_words = collections.defaultdict(set)  # a name of 6 letters or more, as words
        for pid, n in names.items():
            w = " ".join(words_of(n))
            if len(w) >= 6:
                by_words[w].add(pid)
        longest = max((w.count(" ") + 1 for w in by_words), default=0)
        for wid, title, text in db.execute(
                "SELECT id, title, title || ' ' || touches || ' ' || details || ' ' || notes || ' ' || result FROM workboard "
                "WHERE created_by LIKE '%jib%' OR owner LIKE '%jib%' OR updated_by LIKE '%jib%'"):
            # An id counts only in an entry about item records (its title says "item record(s)": #950, #978-#981,
            # #997-#999, ...), less dates, times, audit line numbers and cited Allakhazam item numbers. Elsewhere
            # numbers are npc rows, quests, factions, seconds, years and counts, so only names count there.
            nums = set()
            if re.search(r"item[- ]records?", title, re.I):
                plain = re.sub(r"\d{4}-\d\d-\d\d(?:[T ][\d:]+Z?)?|\b\d\d:\d\d\b", " ", text)
                plain = re.sub(r"(?:\baudit|\bAZ|Allakhazam(?: item)?)\s*#?\s*\d+(?:\s*-\s*\d+)?", " ", plain, flags=re.I)
                nums = set(ints(plain))
                for a, b in re.findall(r"(?<![\d.])(\d+)\s*[-\u2013]\s*(\d+)(?![\d.])", plain):
                    a, b = int(a), int(b)
                    if 0 < b - a <= 50:
                        nums |= set(range(a, b + 1))
            for pid in nums & names.keys():
                ex.add(pid, f"jib board #{wid} (id)")
            words = words_of(text)
            for n in range(1, longest + 1):
                for i in range(len(words) - n + 1):
                    for pid in by_words.get(" ".join(words[i:i + n]), ()):
                        ex.add(pid, f"jib board #{wid} (name)")
    return ex


def plan(db, prima, other=None):
    """(changes [(pid, name, {field: (before, after)}, guide row)], excluded {pid: (name, [reasons])}, stats)."""
    pr = [dict(r) for r in prima.execute("SELECT * FROM prima_items ORDER BY guide_id")]
    pcount = collections.Counter(name_key(r["itemname"]) for r in pr)
    db.row_factory = sqlite3.Row
    recs = {r["patternid"]: dict(r) for r in db.execute("SELECT * FROM itempattern")}
    db.row_factory = None
    names = {p: r["itemname"] for p, r in recs.items()}
    by_name = collections.defaultdict(list)
    for p, r in recs.items():
        if r["itemname"]:
            by_name[name_key(r["itemname"])].append(p)

    stats = collections.Counter()
    slid = {}
    wanted = []  # (pid, fields, guide row)
    for g in pr:
        k = name_key(g["itemname"])
        cands = by_name.get(k, [])
        if pcount[k] > 1 or len(cands) > 1:
            stats["prima rows: ambiguous name"] += 1
            continue
        if not cands:
            stats["prima rows: missing on this shard"] += 1
            continue
        if g["review_note"]:
            stats[f"prima rows: review_note {g['review_note']!r}"] += 1
            continue
        rec = recs[cands[0]]
        fill = {f: (rec[f], g[f]) for f in FIELDS
                if isinstance(g[f], int) and g[f] > 0 and (rec[f] is None or rec[f] == 0)}
        if not fill:
            stats["prima rows: nothing to fill"] += 1
            continue
        stats["prima rows: a gap to fill"] += 1
        # Our stats must sit where Prima prints them: a nonzero stat of ours in a field Prima leaves blank is the old
        # dump's slide (Icebone Ring: ours Str 100 Sta 50, Prima's HP 100 AC 50), and a fill would double it.
        stray = {f: rec[f] for f in STATS if rec[f] and g[f] in (None, 0)}
        if stray:
            slid[cands[0]] = "stats out of place: ours " + ", ".join(f"{f} {v}" for f, v in stray.items()) + \
                " where Prima prints none"
        wanted.append((cands[0], fill, g))

    ex = deliberate(db, names)
    for pid, why in slid.items():
        ex.add(pid, why)
    if other is not None:
        onames = {p: n for p, n in other.execute("SELECT patternid, itemname FROM itempattern")}
        oex = deliberate(other, onames)
        okeys = collections.defaultdict(set)
        for p, reasons in oex.why.items():
            for r in reasons:
                if not r.startswith(("custom id", "made record", "stats out of place")):
                    okeys[name_key(onames[p])].add(r)
        for p, n in names.items():
            for r in sorted(okeys.get(name_key(n), ())):
                ex.add(p, f"other shard: {r}")

    changes, excluded = [], {}
    for pid, fill, g in wanted:
        if pid in ex.why:
            excluded[pid] = (names[pid], ex.why[pid], fill, g)
        else:
            changes.append((pid, names[pid], fill, g))
    return changes, excluded, stats


def held_below(db, pid, level):
    if not table(db, "charinventory"):
        return 0
    return db.execute("SELECT count(DISTINCT i.serverid) FROM charinventory i JOIN characters c ON c.serverid = i.serverid "
                      "WHERE i.patternid = ? AND c.level < ?", (pid, level)).fetchone()[0] if table(db, "characters") else 0


def write_csvs(path, shard, changes, excluded):
    with open(path, "w", newline="") as f:
        w = csv.writer(f)
        w.writerow(["shard", "patternid", "itemname", "field", "before", "after", "prima_guide_id", "prima_page",
                    "region", "rarity"])
        for pid, name, fill, g in changes:
            for fld, (b, a) in fill.items():
                w.writerow([shard, pid, name, fld, "NULL" if b is None else b, a, g["guide_id"], g["printed_page"],
                            g["region"], g["rarity"]])
    root, ext = os.path.splitext(path)
    with open(f"{root}-excluded{ext or '.csv'}", "w", newline="") as f:
        w = csv.writer(f)
        w.writerow(["shard", "patternid", "itemname", "reasons", "would_fill", "prima_guide_id", "prima_page"])
        for pid, (name, reasons, fill, g) in sorted(excluded.items()):
            w.writerow([shard, pid, name, "; ".join(reasons),
                        " ".join(f"{k} {'NULL' if b is None else b}->{a}" for k, (b, a) in fill.items()),
                        g["guide_id"], g["printed_page"]])


def reason_kind(r):
    """A reason without its board entry or column detail, for counting."""
    pre = "other shard: " if r.startswith("other shard: ") else ""
    r = r[len(pre):]
    if r.startswith("jib board"):
        r = "jib board " + r.rsplit(" ", 1)[1]
    elif r.startswith("stats out of place"):
        r = "stats out of place (the dump's slide)"
    elif r.startswith(("made record", "tool log")):
        r = r.split(" (")[0]
    return pre + r


def summary(db, changes, excluded, stats):
    lines = []
    for k in sorted(stats):
        lines.append(f"  {k}: {stats[k]}")
    byfield = collections.Counter(f for _, _, fill, _ in changes for f in fill)
    lines.append(f"to fill: {len(changes)} records, {sum(byfield.values())} fields")
    for f in FIELDS:
        if byfield[f]:
            lines.append(f"  {f:<13}{byfield[f]:>5}")
    cat = collections.Counter()
    for pid, (name, reasons, fill, g) in excluded.items():
        for r in {reason_kind(r) for r in reasons}:
            cat[r] += 1
    lines.append(f"left alone: {len(excluded)} records with a gap (a record can have several reasons)")
    for r, n in sorted(cat.items(), key=lambda x: (-x[1], x[0])):
        lines.append(f"  {n:>5}  {r}")
    lv = [(pid, name, fill["levelreq"][1]) for pid, name, fill, _ in changes if "levelreq" in fill]
    for pid, name, lvl in lv:
        n = held_below(db, pid, lvl)
        if n:
            lines.append(f"  note: {pid} {name!r} levelreq 0 -> {lvl}: held by {n} characters below it")
    return lines


def apply(db, changes):
    db.execute(LOG_DDL)
    for pid, name, fill, g in changes:
        key = {"patternid": pid}
        before, after, n = put(db, key, {f: a for f, (b, a) in fill.items()})
        db.execute(f"INSERT INTO {LOG} (rowkey, fields, guide_id, page, before, after, audit) VALUES (?, ?, ?, ?, ?, ?, ?)",
                   (go_json(key), ",".join(fill), g["guide_id"], g["printed_page"], go_json(before), go_json(after), n))
    return len(changes)


def undo(db):
    back, left = 0, []
    if not table(db, LOG):
        return back, left
    for seq, rowkey, fields, before, after, n in db.execute(
            f"SELECT seq, rowkey, fields, before, after, audit FROM {LOG} ORDER BY seq DESC").fetchall():
        key = json.loads(rowkey)
        if go_json(row(db, "itempattern", key)) != after:
            left.append(f"itempattern {rowkey}: changed since, left as it is")
            continue
        b = json.loads(before)
        _, _, m = put(db, key, {f: b[f] for f in fields.split(",")})
        db.execute("UPDATE gm_audit SET reverted_by = ? WHERE id = ?", (m, n))
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if not left:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--prima", default=os.path.expanduser("~/.cache/prima-items-20261116/EQOA_Prima_Items.sqlite"))
    ap.add_argument("--other-db", help="the other shard (a copy), read only: its deliberate changes carry over by name")
    ap.add_argument("--csv", help="every change (and <csv>-excluded.csv, every record left alone)")
    ap.add_argument("--shard", default="", help="a label for the CSVs")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    g.add_argument("--hash", action="store_true", help="print a digest of table itempattern and stop")
    a = ap.parse_args(argv)

    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    if a.hash:
        print(itempattern_hash(db))
        return 0
    prima = sqlite3.connect(f"file:{os.path.abspath(a.prima)}?mode=ro", uri=True)
    prima.row_factory = sqlite3.Row
    other = sqlite3.connect(f"file:{os.path.abspath(a.other_db)}?mode=ro", uri=True) if a.other_db else None
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"undone: {back} records back" + "".join(f"\n  left: {w}" for w in left))
            return 0 if not left else 1
        for t in ("itempattern", "gm_audit"):
            if not table(db, t):
                db.execute("ROLLBACK")
                print(f"REFUSED: no {t} table here")
                return 1
        changes, excluded, stats = plan(db, prima, other)
        print("\n".join(summary(db, changes, excluded, stats)))
        if a.csv:
            write_csvs(a.csv, a.shard or os.path.basename(a.db), changes, excluded)
            print(f"csv: {a.csv} (+ -excluded)")
        if not changes:
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if table(db, LOG):
            db.execute("ROLLBACK")
            print(f"applied before and gaps found since ({LOG} is there): --undo first")
            return 1
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"(report only: {len(changes)} records; --apply to write)")
            return 0
        n = apply(db, changes)
        db.execute("COMMIT")
        print(f"applied: {n} records, in gm_audit and {LOG}; holders see it at their next login, imported copies at the next start")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
