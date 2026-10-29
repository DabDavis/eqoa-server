#!/usr/bin/env python3
"""Every record the EQOA-Data importers made, swept against its Allakhazam
page (the user, 2026-10-29: "yes sweep all imported spells against their
pages", after the test lanes' report: claude-notes/tested-spells-20261029.md).

The pages: tools/fetch_spell_pages.py's folder (--pages; its captures, read
by that tool's own reader; nothing of them is in this repository). The
records: the ones tools/import_eqoadata_spells.py and
tools/import_eqoadata_mastery_spells.py made (their logs,
eqoadata_spell_import_rows and eqoadata_extra_import_rows), each found on its
classes' Allakhazam lists by its name, the one of its level where a list
names a spell twice.

What it changes, from the page:
  1. The record's scope (Single Target 1, Self Only 0, Group 2, Pet Only 3)
     with the importer's range and icon frame of that scope, its casting
     time, its recast and its power: the importer took none of them off a
     source (another rank's, a kind's, self for want of a target word), and a
     self spell lands on its caster whatever is targeted (the report's
     B02-B04: Annihilate, a nuke at a recast of 2 where its page says 180).
  2. A line of prose (no figure, where its page has figures) becomes the
     page's effects, its scope's word before them and its duration after
     (the report's B01, Pouncing Tiger: a help refused at a creature).
  3. A line that says no time at all takes its page's duration (a buff's
     default is 30 minutes and a tick's is none: Holy Aid healed nothing).
     Instant is no time.
What it leaves, and says: a record that several classes share whose pages
disagree; a value a GM (gm_audit) or an earlier fix changed (the logs of
tools/fix_spell_records.py, fix_archived_spells.py, fix_tested_spells.py and
the enchanter fill's spell_fill_log, missing_line_log, ench_fix_log and
taps_fix_log for lines): those were chosen; a line whose figures or time
differ from its page's (EQOA-Data's and Allakhazam's records disagree: listed
for a decision, not changed); a level that differs (listed).

A value is changed only while it is what this read; every change is logged
in spell_pages_sweep_log (the row's key, the whole row before and after,
why), --undo puts back each row still as this left it (one changed since is
left, and said) and drops the log; a second --apply only adds what is still
to do. Both copy the database first (<db>.bak-<when>-pre-pagesweep). Report
only by default; --list prints every row it would change. The server reads
records and lines at each cast; a client reads its spell book's records at
its next login.

    tools/sweep_spell_pages.py --pages ~/.cache/eqoa-spell-pages [--db eqoa.db] [--apply | --undo] [--list]
"""
import argparse
import collections
import datetime
import difflib
import glob
import json
import os
import re
import shutil
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import fetch_spell_pages as pages_tool  # noqa: E402

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
CLASSES = "WAR RAN PAL SK MNK BRD RGE DRD SHA CL MAG NEC ENC WIZ ALC".split()
SCOPES = {"Self Only": 0, "Single Target": 1, "Group": 2, "Pet Only": 3}
WORD = {0: "Self", 1: "Target", 2: "Group", 3: "Pet"}
# the importer's range and icon frame of each scope (tools/import_eqoadata_spells.py)
SCOPE_RANGE = {0: 16528, 1: 16880, 2: 16880, 3: 16880}
SCOPE_FRAME = {0: -1809115846, 1: 486870879, 2: 593297002, 3: 439871130}
TAG = "pagesweep"

LOG_SCHEMA = """CREATE TABLE IF NOT EXISTS spell_pages_sweep_log (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,   -- spellpattern | spelleffect
    rowkey  TEXT NOT NULL,   -- the row's key, JSON
    before  TEXT,            -- the whole row before, JSON; NULL: inserted
    after   TEXT NOT NULL,   -- the whole row after, JSON
    why     TEXT NOT NULL,
    applied TEXT NOT NULL
)"""
# the logs whose rows say which values were chosen: (table, its tbl column or
# None for spelleffect-only logs, key column, before, after)
JSON_LOGS = ["spell_records_fix_log", "archived_spells_fix_log", "tested_spells_fix_log"]
LINE_LOGS = ["missing_line_log", "ench_fix_log", "taps_fix_log"]
TIME = re.compile(r"\b(\d+(?:\.\d+)?)\s*(sec|secs|second|seconds|s|min|mins|minute|minutes|hr|hrs|hour|hours)\b", re.I)
FIGURE = re.compile(r"\d+(?:\.\d+)?")
# EQOA-Data's notes in a line that are no effect's figure: "+25 Hate", "180recast"
NOTES = re.compile(r"\+\s*\d+\s*hate\b|\b\d+\s*recast\b", re.I)
PROC = re.compile(r"\badd (?:offensive|defensive) proc\b", re.I)
LEAD = re.compile(r"^\s*(target undead|target|self|group|pet)\b", re.I)
# the words of a line that harms what it lands on (the server's own reading is
# entity.ParseSpellText's; this is enough to keep such a spell off its caster)
HARMS = re.compile(r"decrease hitpoints|decrease hp|debuff hp|\bdrain|damage by|decrease ac|\broot\b|\bstun|mesmer|"
                   r"\bsnare|\bcharm\b|lull", re.I)
REZ = re.compile(r"res+ur+ect|\brez\b", re.I)  # EQOA-Data's "Ressurect" too


def scope_kept(page, line, now, new):
    """Why the record's scope stays as it is although its page says another,
    "" when the page's is taken. The server reads some spells by their record's
    scope in ways a page's scope would break."""
    if REZ.search(line):
        return "a resurrection (resurrect.go finds its own target: the page's Group would be the caster's living group)"
    if re.match(r"\s*target undead\b", line, re.I):
        return "a Target undead line (undead.go refuses it at anything not undead: on its caster it never lands)"
    if page.get("AE Radius", "0") not in ("0", "") and page.get("Scope") != "Group":
        # a Group page's radius is who it lands on; any other's names the
        # area's middle, the target or the caster
        return f"an area spell (AE {page['AE Radius']}: the page's scope names the area's middle, not who it lands on)"
    if new == 0 and HARMS.search(line) and not re.match(r"\s*self\b", line, re.I):
        return "a line that harms what it lands on: on its caster it would harm the caster (the report's B02-B04)"
    if new != 2 and re.match(r"\s*group\b", line, re.I):
        return "its line is a group's"
    return ""


def figures(text):
    """A line's or a page's figures, its times and notes aside, as a set."""
    return set(FIGURE.findall(TIME.sub(" ", NOTES.sub(" ", text)))) - {"0"}


def key_text(key):
    return json.dumps(key, sort_keys=True, separators=(",", ":"))


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def row_of(db, tbl, key):
    where = " AND ".join(f"{c} = ?" for c in key)
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", list(key.values()))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def seconds(text):
    """A page's "90s", "30 minutes", "24 seconds": seconds, or None."""
    m = re.fullmatch(r"\s*(\d+(?:\.\d+)?)\s*(s|sec|secs|second|seconds|min|mins|minute|minutes|hour|hours)\s*", text or "")
    if not m:
        return None
    n = float(m.group(1))
    u = m.group(2)
    return n * (60 if u.startswith("min") else 3600 if u.startswith("hour") else 1)


def duration_words(sec):
    """A duration as the server's lines write one: "24sec", "30min"."""
    if sec >= 60 and sec % 60 == 0:
        return f"{int(sec // 60)}min"
    if sec >= 60 and (sec / 60) * 10 == int((sec / 60) * 10):
        return f"{sec / 60:g}min"
    return f"{sec:g}sec"


def load_pages(folder):
    """The class lists ({class: [(id, name, level)]}) and the pages
    ({id: record}) of the folder's captures."""
    lists = {}
    for p in glob.glob(os.path.join(folder, "lists", "scl*.html")):
        n = int(re.search(r"scl(\d+)\.html$", p).group(1))
        cname = pages_tool.LISTS.get(n)
        if cname:
            lists[cname] = pages_tool.read_list(pages_tool.read_file(p))
    pages = {}
    for p in glob.glob(os.path.join(folder, "pages", "*.html")):
        rec = pages_tool.read_page(pages_tool.read_file(p))
        if rec:
            pages[int(os.path.basename(p)[:-5])] = rec
    return lists, pages


def imported_ids(db):
    ids = set()
    for log in ("eqoadata_spell_import_rows", "eqoadata_extra_import_rows"):
        if has_table(db, log):
            ids |= {int(k) for (k,) in db.execute(f"SELECT key FROM {log} WHERE tbl = 'spellpattern'")}
    return ids


def chosen(db):
    """The values chosen since the importers: {spellid: {column: who}} for
    records and {spellid: who} for lines."""
    cols, lines = collections.defaultdict(dict), {}

    def changed(before, after, sid, who, tbl):
        b, a = json.loads(before) if before else {}, json.loads(after) if after else {}
        for c in set(a) | set(b):
            if b.get(c) != a.get(c):
                if tbl == "spellpattern":
                    cols[sid].setdefault(c, who)
                elif c == "line":
                    lines.setdefault(sid, who)

    for log in JSON_LOGS:
        if has_table(db, log):
            for tbl, rowkey, before, after in db.execute(f"SELECT tbl, rowkey, before, after FROM {log}"):
                k = json.loads(rowkey)
                if "spellid" in k:
                    changed(before, after, int(k["spellid"]), log, tbl)
    if has_table(db, "spell_fill_log"):
        for tbl, rowkey, before, after in db.execute("SELECT tbl, rowkey, before, after FROM spell_fill_log"):
            k = json.loads(rowkey)
            if "spellid" in k:
                changed(before, after, int(k["spellid"]), "spell_fill_log", tbl)
    if has_table(db, "gm_audit"):
        for tbl, rowkey, before, after in db.execute(
                "SELECT tbl, rowkey, before, after FROM gm_audit WHERE tbl IN ('spellpattern', 'spelleffect')"):
            try:
                k = json.loads(rowkey)
            except (TypeError, ValueError):
                continue
            if "spellid" in k:
                changed(before, after, int(k["spellid"]), "a GM", tbl)
    for log in LINE_LOGS:
        if has_table(db, log):
            for (sid,) in db.execute(f"SELECT spellid FROM {log}"):
                lines.setdefault(int(sid), log)
    return cols, lines


def page_of(lists, pages, cls, name, lvl):
    """The page of a class's spell of that name: (page id, record), or None
    with why. A name on no list is matched to a close one of the same level
    on its class's (EQOA-Data's spellings: "Mercyls Animation" is Mircyl's
    Animation, "Curse of BertoCMCMulous" Curse of Bertoxxulous)."""
    entries = lists.get(pages_tool.LISTS_BY_NUMBER[cls], [])
    hits = [(i, l) for i, n, l in entries if pages_tool.norm(n) == name]
    if not hits:
        names = {pages_tool.norm(n) for _, n, l in entries if l == lvl}
        m = difflib.get_close_matches(name, sorted(names), n=1, cutoff=0.82)
        if m:
            hits = [(i, l) for i, n, l in entries if pages_tool.norm(n) == m[0] and l == lvl]
    if not hits:
        return None, "on no list of its class"
    same = [h for h in hits if h[1] == lvl]
    if len(same or hits) > 1:
        return None, f"{len(same or hits)} entries of that name on its class's list"
    pid = (same or hits)[0][0]
    if pid not in pages:
        return None, f"no capture of its page (AK {pid})"
    return (pid, pages[pid]), None


def wanted_record(page):
    """What the page says of the record: {column: value}, and what it could
    not read."""
    out, odd = {}, []
    sc = SCOPES.get(page.get("Scope"))
    if sc is None:
        odd.append(f"scope {page.get('Scope')!r}")
    else:
        out["scope"] = sc
    for col, field in (("casttime", "Casting Time"), ("recast", "Recast")):
        s = seconds(page.get(field))
        if s is None or s != int(s):
            odd.append(f"{field.lower()} {page.get(field)!r}")
        else:
            out[col] = int(s)
    try:
        out["power"] = int(page.get("Power Cost", ""))
    except ValueError:
        odd.append(f"power {page.get('Power Cost')!r}")
    return out, odd


def page_line(page, scope):
    """The page's effects as a line of the server's: its scope's word, the
    effects, its duration."""
    line = f"{WORD[scope]}, " + ", ".join(page["effects"])
    sec = seconds(page.get("Duration"))
    if sec:
        line += ", " + duration_words(sec)
    return line


def plan(db, lists, pages):
    """The changes [(table, key, row before or None, row after, why)], and
    what was left [(what, why)] and listed [(what, why)]."""
    cols_chosen, lines_chosen = chosen(db)
    changes, left, listed = [], [], []
    counts = collections.Counter()
    for sid in sorted(imported_ids(db)):
        rec = row_of(db, "spellpattern", {"spellid": sid})
        if rec is None:
            continue
        what = f"{sid} {rec['spellname']} L{rec['abilitylvl']}"
        classes = [c for (c,) in db.execute("SELECT class FROM classspells WHERE spellid = ? ORDER BY class", (sid,))]
        if not classes:
            left.append((what, "no class has it"))
            counts["no class"] += 1
            continue
        found = {}
        for c in classes:
            hit, why = page_of(lists, pages, c, pages_tool.norm(rec["spellname"]), rec["abilitylvl"])
            if hit:
                found[c] = hit
        if not found:
            counts["no page"] += 1
            continue
        wants = {c: wanted_record(p) for c, (_, p) in found.items()}
        if len({json.dumps(w[0], sort_keys=True) for w in wants.values()}) > 1:
            left.append((what, "its classes' pages disagree: " + "; ".join(
                f"{CLASSES[c]} AK {found[c][0]} {w[0]}" for c, w in wants.items())))
            counts["pages disagree"] += 1
            continue
        c0 = sorted(found)[0]
        pid, page = found[c0]
        want, odd = wants[c0]
        for o in odd:
            left.append((what, f"AK {pid}: {o} not read"))
        if page.get("name") and pages_tool.norm(page["name"]) != pages_tool.norm(rec["spellname"]):
            listed.append((what, f"its page's name is {page['name']!r} (AK {pid}): kept"))
            counts["name differs"] += 1
        if page.get("level") not in (None, rec["abilitylvl"]):
            listed.append((what, f"level {rec['abilitylvl']}, its page's {page['level']} (AK {pid}): kept"))
        counts["matched"] += 1

        after = dict(rec)
        why = []
        eff0 = row_of(db, "spelleffect", {"spellid": sid})
        line0 = (eff0 or {}).get("line", "") or ""
        scope_held = False
        if "scope" in want and want["scope"] != rec["scope"]:
            kept = scope_kept(page, line0, rec["scope"], want["scope"])
            if kept:
                scope_held = True
                listed.append((what, f"scope {rec['scope']} kept, its page's {want['scope']}: {kept}"))
                counts["scope kept"] += 1
                want = dict(want)
                del want["scope"]
        for col, v in want.items():
            if rec[col] == v:
                continue
            if col in cols_chosen.get(sid, {}):
                left.append((what, f"{col} {rec[col]} (set by {cols_chosen[sid][col]}), its page's {v}"))
                continue
            after[col] = v
            why.append(col)
        if "scope" in why:
            old = rec["scope"]
            if (rec["range"], rec["iconColor"]) == (SCOPE_RANGE.get(old), SCOPE_FRAME.get(old)) and \
                    not {"range", "iconColor"} & set(cols_chosen.get(sid, {})):
                after["range"], after["iconColor"] = SCOPE_RANGE[after["scope"]], SCOPE_FRAME[after["scope"]]
            else:
                listed.append((what, "scope changed, its range and frame (not the importer's) kept"))
        if after != rec:
            changes.append(("spellpattern", {"spellid": sid}, rec, after,
                            f"AK {pid}: {', '.join(why)} as its page"))
            counts["records"] += 1

        # the line
        eff = row_of(db, "spelleffect", {"spellid": sid})
        line = (eff or {}).get("line", "") or ""
        scope = after["scope"]
        sec = seconds(page.get("Duration"))
        page_figures = figures(" ".join(page["effects"]))
        new = None
        if sid in lines_chosen:
            if not FIGURE.search(line) or not TIME.search(line):
                listed.append((what, f"its line (set by {lines_chosen[sid]}) kept: {line!r}"))
        elif page["effects"] and page_figures and not FIGURE.search(line) and scope_held:
            # the page's effects are for the page's scope, which the record
            # did not take (Subversion's would drain the caster's group)
            listed.append((what, f"its line of prose kept, its scope not its page's: {line!r}, AK {pid} "
                                 f"{' | '.join(page['effects'])!r}"))
        elif page["effects"] and page_figures and not FIGURE.search(line):
            new, reason = page_line(page, scope), "its line of prose, its page's effects"
        elif line and sec and not TIME.search(line):
            new, reason = f"{line.rstrip(' ,')}, {duration_words(sec)}", "its page's duration"
        elif line:
            # the two sources' figures compared (a page naming a proc has its
            # figures on the proc's own page)
            effects = " | ".join(page["effects"])
            if not PROC.search(effects) and figures(line) != figures(effects):
                listed.append((what, f"figures differ: line {line!r}, AK {pid} {effects!r}: kept"))
                counts["figures differ"] += 1
            m = TIME.findall(line)
            if sec and m:
                last = seconds(f"{m[-1][0]} {m[-1][1]}")
                if last is not None and abs(last - sec) > 0.5:
                    listed.append((what, f"time differs: line {line!r} {last:g}s, AK {pid} {sec:g}s: kept"))
                    counts["time differs"] += 1
        lead = LEAD.match(new or line)
        if lead and lead.group(1).lower() != "target undead" and lead.group(1).lower() != WORD[scope].lower():
            listed.append((what, f"its line says {lead.group(1)!r}, its record's scope {WORD[scope]} (AK {pid}): kept"))
            counts["word differs"] += 1
        if new:
            a = dict(eff) if eff else {"spellid": sid, "line": "", "fxdict": 0, "source": ""}
            a["line"] = new
            a["source"] = (a["source"] + "; " if a["source"] else "") + f"line AK {pid} ({TAG})"
            changes.append(("spelleffect", {"spellid": sid}, eff, a, f"AK {pid}: {reason}"))
            counts["lines"] += 1
            if rec["spelldesc"] == line and line:
                # its description is its line (the importer's): kept with it
                for ch in changes:
                    if ch[0] == "spellpattern" and ch[1] == {"spellid": sid}:
                        ch[3]["spelldesc"] = new
                        break
                else:
                    after2 = dict(rec)
                    after2["spelldesc"] = new
                    changes.append(("spellpattern", {"spellid": sid}, rec, after2, f"AK {pid}: its description is its line"))
    return changes, left, listed, counts


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-pagesweep"
    shutil.copy2(path, dst)
    return dst


def write_row(db, tbl, key, row, insert=False):
    if insert:
        cols = list(row)
        db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [row[c] for c in cols])
        return
    where = " AND ".join(f"{c} = ?" for c in key)
    if row is None:
        db.execute(f"DELETE FROM {tbl} WHERE {where}", list(key.values()))
        return
    cols = [c for c in row if c not in key]
    db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in cols)} WHERE {where}",
               [row[c] for c in cols] + list(key.values()))


def apply(db, changes):
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_SCHEMA)
        for tbl, key, before, after, why in changes:
            db.execute("INSERT INTO spell_pages_sweep_log (tbl, rowkey, before, after, why, applied) VALUES (?, ?, ?, ?, ?, ?)",
                       (tbl, key_text(key), None if before is None else json.dumps(before, sort_keys=True),
                        json.dumps(after, sort_keys=True), why, when))
            write_row(db, tbl, key, after, insert=before is None)
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db):
    if not has_table(db, "spell_pages_sweep_log"):
        print("nothing logged: nothing to undo")
        return
    logged = db.execute("SELECT seq, tbl, rowkey, before, after FROM spell_pages_sweep_log ORDER BY seq DESC").fetchall()
    back = 0
    db.execute("BEGIN IMMEDIATE")
    try:
        for seq, tbl, rowkey, before, after in logged:
            if tbl not in ("spellpattern", "spelleffect"):
                raise SystemExit(f"spell_pages_sweep_log names table {tbl!r}, which this tool never writes")
            key = json.loads(rowkey)
            now = row_of(db, tbl, key)
            if now != json.loads(after):
                print(f"  {tbl} {rowkey}: changed since: left as it is")
                continue
            write_row(db, tbl, key, None if before is None else json.loads(before))
            back += 1
        db.execute("DROP TABLE spell_pages_sweep_log")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"put back {back} of {len(logged)} rows")


def describe(change):
    tbl, key, before, after, why = change
    if before is None:
        return f"{tbl} {key_text(key)}: made {after}  ({why})"
    diff = ", ".join(f"{c} {before[c]!r} -> {after[c]!r}" for c in after if after[c] != before.get(c))
    return f"{tbl} {key_text(key)} {before.get('spellname', '')}: {diff}  ({why})"


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--pages", help="tools/fetch_spell_pages.py's folder")
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    ap.add_argument("--list", action="store_true", help="print every change and every row left or listed")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the changes (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply changed")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    if a.undo:
        if not a.no_backup and has_table(db, "spell_pages_sweep_log"):
            print(f"copy: {backup(a.db)}")
        undo(db)
        return
    if not a.pages:
        sys.exit("--pages is required (tools/fetch_spell_pages.py's folder)")
    lists, pages = load_pages(os.path.expanduser(a.pages))
    if len(lists) != len(pages_tool.LISTS):
        sys.exit(f"{a.pages}: {len(lists)} class lists, not {len(pages_tool.LISTS)} (tools/fetch_spell_pages.py first)")
    changes, left, listed, counts = plan(db, lists, pages)
    if a.list:
        for c in changes:
            print(describe(c))
        for what, why in left:
            print(f"left: {what}: {why}")
        for what, why in listed:
            print(f"listed: {what}: {why}")
    on = counts["matched"] + counts["pages disagree"]
    print(f"{on + counts['no page'] + counts['no class']} imported records: {on} on a page, "
          f"{counts['no page']} on none, {counts['pages disagree']} whose classes' pages disagree, {counts['no class']} of no class")
    print(f"{len(changes)} rows to change ({counts['records']} records, {counts['lines']} lines), {len(left)} values left, "
          f"{len(listed)} listed ({counts['figures differ']} lines' figures, {counts['time differs']} times and "
          f"{counts['word differs']} target words differ from their pages; {counts['name differs']} names differ; "
          f"{counts['scope kept']} scopes kept)")
    if not a.apply or not changes:
        if not a.apply:
            print("report only: --apply to write, --list for every row")
        return
    if not a.no_backup:
        print(f"copy: {backup(a.db)}")
    apply(db, changes)
    print(f"changed {len(changes)} rows")


if __name__ == "__main__":
    main()
