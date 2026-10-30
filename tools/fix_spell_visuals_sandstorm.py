#!/usr/bin/env python3
"""Every spell's visual (0x00B1's last field, spelleffect.fxdict) as Sandstorm plays it,
where Sandstorm was seen casting it, and for the other ranks of its line (the user,
2026-10-30: "we should login to sandstorm cast each spell that dab, dab01, dab2, dab3 chars
have. some spells are a line and have the same spell fx throughout. we can use that to
correct our spell fx on our server").

The recordings are tools/sandstorm_spell_visuals.tsv: each spell Sandstorm's characters
cast (eqoa-rust-headless's harvester, `castlist`), by Sandstorm's ability id and name, with
the visual every cast of it carried (one a spell: a spell seen with two is left out). The
server picks the visual, not the ability record (claude-notes/casting-sandstorm-20261021.md),
so only a cast shows it.

  1. A recorded spell is ours by its id and name, else tools/import_sandstorm_harvest.py's
     ID_OFFSET (300000 + its id: an id ours held for another spell), else the one spell of
     that name a class that cast it has (classspells). Its visual becomes Sandstorm's.
  2. A line is a class's spells (classspells) with the same front icon (spellpattern.icon)
     and scope whose names share a word: the first (not a rank's: Minor, Lesser, Light,
     Greater, Superior, Major, Improved, Group) or the last, of four letters or more, or,
     for names of one word, an ending of four letters or more ("Woodskin", "Rockskin",
     "Steelskin"; "Barbshield", "Brambleshield"); two records of one name are one spell.
     An unrecorded rank takes a recorded rank's visual when it is another record of that
     spell, or when its level lies between two recorded ranks of the line that showed that
     same visual. Sandstorm's lines do not all keep one visual: its taps step 0xAF72B893,
     ...894, ...895 by rank, its disease DoTs 0x052EF90F to ...911, its pet masteries
     0x1DAEB5B6 to ...B7, so a rank beyond the recorded ones is left and said; --whole-lines
     gives it the line's visual anyway where every recorded rank showed the same one (the
     user's rule: "some spells are a line and have the same spell fx throughout"). A spell
     two lines would give different visuals is left and said.
  3. A visual a GM set (the latest change of the row's fxdict in gm_audit, through the GM
     panel, #spellfx or the community visuals, not reverted, and still the row's) is left
     as it is and said.

A spell with no spelleffect row gets one (its line empty: a cast reads it by the spell's
name, as with no row). Every change is logged in spell_visuals_sandstorm_log (the spell, the
row before and after, why); --undo puts back each one still as this left it (one changed
since is left, and said) and drops the log; a second --apply is refused while the log is
there. Report only by default; --tsv writes the comparison (ours against Sandstorm's, every
recorded spell). One transaction each. The server reads a spell's visual at each cast, so
neither needs a restart; the community page's list is built again within a minute.

    tools/fix_spell_visuals_sandstorm.py [--db eqoa.db] [--visuals tools/sandstorm_spell_visuals.tsv]
                                         [--whole-lines] [--tsv comparison.tsv] [--apply | --undo]
"""
import argparse
import collections
import csv
import datetime
import json
import os
import re
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
DEFAULT_DB = os.path.join(HERE, "..", "eqoa.db")
DEFAULT_VISUALS = os.path.join(HERE, "sandstorm_spell_visuals.tsv")
ID_OFFSET = 300000
LOG = "spell_visuals_sandstorm_log"
# --whole-lines: a line whose recorded ranks agree gives its visual to every rank (the user's
# rule), not only to those between its recorded ranks
WHOLE_LINES = False
SOURCE = "Sandstorm's visual (tools/fix_spell_visuals_sandstorm.py)"
LOG_SCHEMA = f"""CREATE TABLE {LOG} (
    spellid    INTEGER PRIMARY KEY,
    had_row    INTEGER NOT NULL,   -- 1: the spelleffect row was there; 0: this made it
    fx_before  INTEGER NOT NULL,
    fx_after   INTEGER NOT NULL,
    via        TEXT NOT NULL,      -- 'recorded' or 'line'
    line_of    INTEGER,            -- a line's: the recorded rank it follows
    why        TEXT NOT NULL,
    applied    TEXT NOT NULL
)"""

# a rank's word: never what joins two spells into a line
RANK_WORDS = {"minor", "lesser", "light", "greater", "superior", "major", "improved", "group", "of", "the",
              "illusion", "glamour"}
# endings of one-word names too common to join them
COMMON_ENDINGS = ("tion", "sion", "ance", "ence", "ment", "ness", "ship", "able", "ious", "ing")


def norm(name):
    return " ".join((name or "").replace("`", "'").lower().split())


def words(name):
    return [w for w in re.split(r"[^a-z0-9']+", norm(name)) if w]


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def hex32(v):
    return "0x%08X" % (int(v) & 0xFFFFFFFF)


def same_line(a, b):
    """Whether two spell names (of one class, icon and scope) read as one line's."""
    wa, wb = words(a), words(b)
    if not wa or not wb:
        return False
    if wa == wb:
        return True  # one spell's two records (Sandstorm's own: a summoning with weapons and without)
    if len(wa) == 1 and len(wb) == 1:
        x, y = wa[0], wb[0]
        n = 0
        while n < min(len(x), len(y)) and x[-1 - n] == y[-1 - n]:
            n += 1
        end = x[len(x) - n:]
        return n >= 4 and not end.endswith(COMMON_ENDINGS)
    last = lambda w: w[-1]
    for x, y in ((wa[0], wb[0]), (last(wa), last(wb))):
        if x == y and len(x) >= 4 and x not in RANK_WORDS:
            return True
    # a one-word rank against a longer name's last word ("Healing", "Minor Healing")
    if len(wa) == 1 and wa[0] == last(wb) and len(wa[0]) >= 4 and wa[0] not in RANK_WORDS:
        return True
    if len(wb) == 1 and wb[0] == last(wa) and len(wb[0]) >= 4 and wb[0] not in RANK_WORDS:
        return True
    return False


def read_visuals(path):
    """[{ssid, name, classes, visual, casts}] from the recordings TSV (comment lines start
    with #)."""
    out = []
    with open(path, newline="") as fh:
        rows = csv.DictReader((l for l in fh if not l.startswith("#")), delimiter="\t")
        for r in rows:
            out.append({"ssid": int(r["ssid"]), "name": r["name"].strip(),
                        "classes": [int(c) for c in r["class"].split(",") if c.strip()],
                        "visual": int(r["visual"], 16), "casts": int(r.get("casts") or 1)})
    return out


def load(db):
    spells = {}
    for sid, name, icon, scope, level in db.execute(
            "SELECT spellid, spellname, icon, scope, abilitylvl FROM spellpattern"):
        spells[sid] = {"id": sid, "name": (name or "").strip(), "icon": icon, "scope": scope, "level": level}
    classes = collections.defaultdict(set)   # spell -> classes
    by_class = collections.defaultdict(set)  # class -> spells
    for cls, sid in db.execute("SELECT class, spellid FROM classspells"):
        if sid in spells:
            classes[sid].add(cls)
            by_class[cls].add(sid)
    rows = {sid: (line, fx, source) for sid, line, fx, source in
            db.execute("SELECT spellid, line, fxdict, source FROM spelleffect")}
    return spells, classes, by_class, rows


def gm_set(db):
    """{spellid: the visual a GM's latest change of the row's fxdict gave it}, for changes
    not reverted (gm_audit; none when there is no log)."""
    if not has_table(db, "gm_audit"):
        return {}
    out = {}
    for rowkey, before, after in db.execute(
            "SELECT rowkey, before, after FROM gm_audit WHERE tbl = 'spelleffect' AND reverted_by IS NULL ORDER BY id"):
        try:
            sid = int(json.loads(rowkey)["spellid"])
            b = json.loads(before)["fxdict"] if before else None
            a = json.loads(after)["fxdict"] if after else None
        except (ValueError, KeyError, TypeError):
            continue
        if a is None:
            out.pop(sid, None)  # a delete: no GM's visual now
        elif a != b:
            out[sid] = int(a) & 0xFFFFFFFF
    return out


def ours_of(rec, spells, by_class):
    """Our spellpattern id for a recorded Sandstorm spell, and how it was found."""
    for sid, how in ((rec["ssid"], "id"), (rec["ssid"] + ID_OFFSET, "ID_OFFSET")):
        if sid in spells and norm(spells[sid]["name"]) == norm(rec["name"]):
            return sid, how
    named = sorted({s for c in rec["classes"] for s in by_class.get(c, ()) if norm(spells[s]["name"]) == norm(rec["name"])})
    if len(named) == 1:
        return named[0], "name"
    return None, ("no spell of ours so named" if not named else "%d of ours so named: %s" % (len(named), named))


def lines_of(spells, by_class):
    """{class: [set of spell ids]}: each class's lines (see the module's comment)."""
    out = {}
    for cls, ids in by_class.items():
        groups = collections.defaultdict(list)
        for s in ids:
            groups[(spells[s]["icon"], spells[s]["scope"])].append(s)
        lines = []
        for members in groups.values():
            parent = {s: s for s in members}

            def find(x):
                while parent[x] != x:
                    parent[x] = parent[parent[x]]
                    x = parent[x]
                return x
            for i, a in enumerate(members):
                for b in members[i + 1:]:
                    if same_line(spells[a]["name"], spells[b]["name"]):
                        parent[find(a)] = find(b)
            comp = collections.defaultdict(set)
            for s in members:
                comp[find(s)].add(s)
            lines += [c for c in comp.values() if len(c) > 1]
        out[cls] = lines
    return out


def plan(db, visuals):
    """What the tool would do: (changes, report). A change is (spellid, had_row, before,
    after, via, line_of, why)."""
    spells, classes, by_class, rows = load(db)
    gm = gm_set(db)
    report = collections.defaultdict(list)
    recorded = {}  # our id -> (visual, Sandstorm's name)
    for rec in visuals:
        sid, how = ours_of(rec, spells, by_class)
        if sid is None:
            report["unmatched"].append("%d %s (%s)" % (rec["ssid"], rec["name"], how))
            continue
        if sid in recorded and recorded[sid][0] != rec["visual"]:
            report["recorded twice"].append("%d %s: %s and %s" % (sid, rec["name"], hex32(recorded[sid][0]), hex32(rec["visual"])))
            recorded[sid] = (None, rec["name"])
            continue
        recorded[sid] = (rec["visual"], rec["name"])
        if how != "id":
            report["matched by " + how].append("%d %s -> ours %d" % (rec["ssid"], rec["name"], sid))
    recorded = {s: v for s, v in recorded.items() if v[0] is not None}
    want = {s: (v, "recorded", None) for s, (v, _) in recorded.items()}
    # lines: every class's, each with the visuals its recorded ranks showed
    from_lines = collections.defaultdict(set)  # spell -> {(visual, recorded rank)}
    for cls, lines in sorted(lines_of(spells, by_class).items()):
        for line in lines:
            got = sorted(((spells[s]["level"], s, recorded[s][0]) for s in line if s in recorded))
            if not got:
                continue
            names = ", ".join("%s (%d)%s" % (spells[s]["name"], spells[s]["level"], " " + hex32(recorded[s][0]) if s in recorded else "")
                              for s in sorted(line, key=lambda x: (spells[x]["level"], x)))
            agree = len({v for _, _, v in got}) == 1
            if len(got) > 1:
                report["lines whose recorded ranks agree" if agree else "lines whose recorded ranks differ"].append(
                    "class %d: %s" % (cls, names))
            for s in line:
                if s in recorded:
                    continue
                lvl, name = spells[s]["level"], norm(spells[s]["name"])
                twin = [(v, r) for _, r, v in got if norm(spells[r]["name"]) == name]
                below = [(l, r, v) for l, r, v in got if l <= lvl]
                above = [(l, r, v) for l, r, v in got if l >= lvl]
                if twin:
                    # another record of a recorded spell (Sandstorm's own twins: a summoning
                    # with weapons and without)
                    from_lines[s].add(twin[0])
                elif below and above and below[-1][2] == above[0][2]:
                    # between two recorded ranks that agree
                    from_lines[s].add((below[-1][2], below[-1][1]))
                elif agree and WHOLE_LINES:
                    from_lines[s].add((got[0][2], got[0][1]))
                elif agree:
                    report["beyond their line's recorded ranks, left (--whole-lines gives them its visual)"].append(
                        "%d %s (%d): %s, recorded %d to %d" % (s, spells[s]["name"], lvl, hex32(got[0][2]), got[0][0], got[-1][0]))
                else:
                    report["beyond or between ranks whose visuals differ, left"].append(
                        "%d %s (%d) in class %d's %s" % (s, spells[s]["name"], lvl, cls, spells[got[0][1]]["name"]))
    for s, got in sorted(from_lines.items()):
        if len({v for v, _ in got}) > 1:
            report["ranks two lines disagree on"].append("%d %s: %s" % (s, spells[s]["name"], sorted(hex32(v) for v, _ in got)))
            continue
        v, rank = min(got, key=lambda x: x[1])
        want[s] = (v, "line", rank)
    changes = []
    for s, (v, via, rank) in sorted(want.items()):
        line, fx, _ = rows.get(s, ("", 0, ""))
        fx = int(fx or 0) & 0xFFFFFFFF
        if fx == v:
            report["already " + via].append("%d %s %s" % (s, spells[s]["name"], hex32(v)))
            continue
        if s in gm and gm[s] == fx and fx != 0:
            report["a GM's, left"].append("%d %s: %s, Sandstorm's %s (%s)" % (s, spells[s]["name"], hex32(fx), hex32(v), via))
            continue
        why = ("Sandstorm cast it with %s" % hex32(v)) if via == "recorded" else \
              ("its line's %s (%d) cast with %s" % (spells[rank]["name"], rank, hex32(v)))
        changes.append((s, int(s in rows), fx, v, via, rank, why))
    return changes, report, spells, by_class, rows, gm


def comparison(path, visuals, spells, by_class, rows, gm):
    """Ours against Sandstorm's, one line a recorded spell."""
    with open(path, "w") as fh:
        fh.write("ssid\tname\tclasses\tours\tours_name\tours_visual\tours_from\tsandstorm\tsame\n")
        for rec in visuals:
            cls = ",".join(map(str, rec["classes"]))
            sid, how = ours_of(rec, spells, by_class)
            if sid is None:
                fh.write("%d\t%s\t%s\t\t\t\t%s\t%s\t\n" % (rec["ssid"], rec["name"], cls, how, hex32(rec["visual"])))
                continue
            line, fx, source = rows.get(sid, ("", 0, ""))
            fx = int(fx or 0) & 0xFFFFFFFF
            frm = "stand-in" if fx == 0 else ("a GM" if gm.get(sid) == fx else (source or "row"))
            fh.write("%d\t%s\t%s\t%d\t%s\t%s\t%s\t%s\t%s\n" % (rec["ssid"], rec["name"], cls, sid, spells[sid]["name"],
                     hex32(fx) if fx else "0", frm, hex32(rec["visual"]), "yes" if fx == rec["visual"] else "no"))


def undo(db):
    if not has_table(db, LOG):
        print("nothing logged: nothing to undo")
        return
    db.execute("BEGIN IMMEDIATE")
    try:
        back = left = 0
        for sid, had, before, after in db.execute(f"SELECT spellid, had_row, fx_before, fx_after FROM {LOG}").fetchall():
            now = db.execute("SELECT line, fxdict, source FROM spelleffect WHERE spellid = ?", (sid,)).fetchone()
            if now is None or (int(now[1]) & 0xFFFFFFFF) != after:
                left += 1
                print("left: %d changed since (%s)" % (sid, "no row" if now is None else hex32(now[1])))
                continue
            if had:
                db.execute("UPDATE spelleffect SET fxdict = ? WHERE spellid = ? AND fxdict = ?", (before, sid, now[1]))
            elif now[0] == "" and now[2] == SOURCE:
                db.execute("DELETE FROM spelleffect WHERE spellid = ?", (sid,))
            else:
                db.execute("UPDATE spelleffect SET fxdict = ? WHERE spellid = ?", (before, sid))
            back += 1
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print("put back %d visuals, %d changed since left as they are" % (back, left))


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--visuals", default=DEFAULT_VISUALS, help="the recordings (sandstorm_spell_visuals.tsv)")
    ap.add_argument("--tsv", help="write ours against Sandstorm's, every recorded spell, here")
    ap.add_argument("--whole-lines", action="store_true",
                    help="give a line's visual to its ranks beyond the recorded ones too (where its recorded ranks agree)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the visuals (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply changed")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    global WHOLE_LINES
    WHOLE_LINES = a.whole_lines
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        undo(db)
        return
    if a.apply and has_table(db, LOG):
        sys.exit(f"{LOG} is there: applied already (--undo first)")
    visuals = read_visuals(a.visuals)
    changes, report, spells, by_class, rows, gm = plan(db, visuals)
    for title in sorted(report):
        items = report[title]
        print("%s (%d):" % (title, len(items)))
        for it in items:
            print("    " + it)
    for s, had, before, after, via, rank, why in changes:
        print("%7d %s: %s -> %s (%s%s)" % (s, spells[s]["name"], hex32(before) if before else "0 (stand-in)", hex32(after), why,
                                           "" if had else "; a new row"))
    nrec = sum(1 for c in changes if c[4] == "recorded")
    print("%d spells to change: %d recorded, %d by their lines; %d recorded spells read" % (len(changes), nrec, len(changes) - nrec, len(visuals)))
    if a.tsv:
        comparison(a.tsv, visuals, spells, by_class, rows, gm)
        print("comparison written to %s" % a.tsv)
    if not a.apply or not changes:
        if not a.apply:
            print("report only: --apply to write")
        return
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_SCHEMA)
        for s, had, before, after, via, rank, why in changes:
            db.execute(f"INSERT INTO {LOG} VALUES (?, ?, ?, ?, ?, ?, ?, ?)", (s, had, before, after, via, rank, why, when))
            if had:
                n = db.execute("UPDATE spelleffect SET fxdict = ? WHERE spellid = ? AND (fxdict & 4294967295) = ?",
                               (after, s, before)).rowcount
                if n != 1:
                    raise RuntimeError("spell %d changed while this ran" % s)
            else:
                db.execute("INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (?, '', ?, ?)", (s, after, SOURCE))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print("changed %d visuals (logged in %s); the next cast of each plays it" % (len(changes), LOG))


if __name__ == "__main__":
    main()
