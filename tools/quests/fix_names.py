#!/usr/bin/env python3
"""Quest files in step with a database: each NPC name a quest uses spelled
as the database spells it, and its `unresolved` what still does not resolve.

    python3 tools/quests/fix_names.py --db eqoa.db [--rename [KEY:]OLD=NEW ...]
        [--drop-note REGEX ...] [--case] [--write] [file.json ...]

With no files it takes every tools/quests/data/*.json. A name a quest uses
(its giver, a step's target, a say or spawn event's NPC, the travel NPC of a
trip it unlocks) that no npcs row has is respelled as the one npcs spelling
that agrees with it but for case,
apostrophes and backticks, hyphens, a leading "the" and plurals ("Counselor
Du'Klot" -> "Counselor Du`Klot", "thick coat wolf" -> "Thick Coat Wolves");
--rename gives any other ("OLD=NEW" in every quest, "KEY:OLD=NEW" in the
quests whose key starts with KEY), found or not; --case respells a name found
but for its case as npcs spells it, when npcs spells it one way
(tools/fix_npc_name_case.py's "Tae Ew crusader" for a quest's "Tae Ew
Crusader"). A faction a quest asks a
standing with or moves is respelled the same way, as the factions table
spells it. Then each quest's
`unresolved` is what check_quests.py finds missing (the entries it keeps in
their order, new ones after), and its notes lose what is no longer true: a
sentence that only says names are not in the database (or on this server),
once each name of the files' old `unresolved` it gives resolves, none it
gives is still missing and it gives no other proper name; "It follows L5: L7, L10 before it are drafts until
its names are in the database." (a chain that skipped its drafts), once they
are whole, the quest then requiring the one right before it again; and any
--drop-note match. A sentence that says more
than that about a name now found is listed, for a hand to word again.

Report only by default (every change and every sentence dropped); --write
writes the files in the format they are kept in (json.dumps, indent 1, a
final newline). Run check_quests.py after.
"""
import argparse
import collections
import glob
import json
import os
import re
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import check_quests as cq  # noqa: E402

fold = cq.fold

# a sentence that only says something is missing, and one about the drafts before a quest
MISSING_ONLY = re.compile(r"^(?P<subj>.+?) (?:is|are) not (?:in the database|on this server)(?: either)?[.;]?$")
MISSING_ANY = re.compile(r"\b(?:not|nor|neither|no one)\b.*\b(?:in the database|on this server)\b")
# a proper name of two words or more ("Phil Wiggins", "Duchess Melnorn")
PROPER = re.compile(r"\b[A-Z][\w'`-]*(?:\s+[A-Z][\w'`-]*)+")
CHAIN = re.compile(r"^It follows (?P<base>[^:]+): (?P<skipped>.+) before it (?:is a draft|are drafts) "
                   r"until its names are in the database\.$")


def key(s):
    """A name as seed_quest_npcs.py matches one: case, apostrophes and
    backticks, hyphens and the like, a level in brackets, a leading "the"
    and plurals aside."""
    s = fold(s).replace("`", "'").replace("’", "'")
    s = re.sub(r"\s*\([^)]*\)", "", s)
    s = re.sub(r"^(the|a|an)\s+", "", s.strip())
    s = re.sub(r"[^a-z0-9' ]", " ", s)

    def sing(w):
        if w.endswith("ies"):
            return w[:-3] + "y"
        if w.endswith("ves"):
            return w[:-3] + "f"
        if w.endswith("es") and w[:-2].endswith(("s", "x", "ch", "sh")):
            return w[:-2]
        if w.endswith("s") and not w.endswith("ss"):
            return w[:-1]
        return w
    return " ".join(sing(w) for w in s.split())


def spellings(db_path):
    """npcs' names by key(): how often each spelling is written."""
    db = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True)
    out = collections.defaultdict(collections.Counter)
    for (n,) in db.execute("SELECT npc_name FROM npcs WHERE npc_name IS NOT NULL"):
        n = n.strip()
        if n:
            out[key(n)][n] += 1
    db.close()
    return out


def parse_renames(args):
    out = []
    for a in args:
        left, eq, new = a.partition("=")
        scope, _, old = left.rpartition(":")
        if not eq or not old.strip() or not new.strip():
            sys.exit(f"--rename {a!r}: give OLD=NEW or KEY:OLD=NEW")
        out.append((scope, old.strip(), new.strip()))
    return out


def references(q):
    """The places a quest names an NPC: (holder, field) pairs."""
    out = [(q, "giver")]
    for st in q.get("steps", []):
        if isinstance(st, dict):
            out.append((st, "target"))
            out += [(ev, "npc") for ev in st.get("events", []) if isinstance(ev, dict)]
    out += [(ev, "npc") for ev in q.get("events", []) if isinstance(ev, dict)]
    out += [(u, "npc") for u in q.get("unlocks", []) if isinstance(u, dict)]
    return [(h, f) for h, f in out if isinstance(h.get(f), str) and h[f].strip()]


def faction_references(q):
    """The places a quest names a faction: its factions_required and its
    steps' reward factions, as (entry, 0) pairs (the entry a list, the name
    its first)."""
    out = [fr for fr in q.get("factions_required", []) if isinstance(fr, list)]
    for st in q.get("steps", []):
        rw = st.get("rewards") if isinstance(st, dict) else None
        if isinstance(rw, dict):
            out += [f for f in rw.get("factions", []) if isinstance(f, list)]
    return [(e, 0) for e in out if e and isinstance(e[0], str) and e[0].strip()]


def faction_spellings(db_path):
    """The factions table's names by key() (none: no table)."""
    db = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True)
    out = collections.defaultdict(collections.Counter)
    if db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'factions'").fetchone()[0]:
        for (n,) in db.execute("SELECT name FROM factions"):
            if n and n.strip():
                out[key(n)][n.strip()] += 1
    db.close()
    return out


def sentences(text):
    return [s for s in re.split(r"(?<=[.;])\s+", text.strip()) if s]


def join(parts):
    """The sentences kept, back into notes: where one was dropped, the piece
    after it begins a sentence (a capital) and the piece before it ends one."""
    out = []
    for s, dropped_before in parts:
        if dropped_before:
            if (not out or out[-1].endswith(".")) and s[:1].islower():
                s = s[:1].upper() + s[1:]
            if out and out[-1].endswith(";") and s[:1].isupper():
                out[-1] = out[-1][:-1] + "."
        out.append(s)
    if out and out[-1].endswith(";"):
        out[-1] = out[-1][:-1] + "."
    return " ".join(out)


def words(text):
    """A sentence's words as key() has a name's (case, apostrophes and
    backticks, hyphens, plurals), brackets and all."""
    t = re.sub(r"[^a-z0-9' ]", " ", fold(text).replace("`", "'").replace("’", "'"))
    return key(" ".join(w for w in t.split()))


def mentions(text, name):
    """Whether a sentence names a name, a whole word or words."""
    n = key(name)
    return bool(n) and re.search(r"(?<![a-z0-9'])" + re.escape(n) + r"(?![a-z0-9'])", words(text)) is not None


def fix(files, db_path, renames=(), drops=(), case=False):
    names = cq.Names(db_path)
    by_key = spellings(db_path)
    factions_by_key = faction_spellings(db_path)
    drops = [re.compile(d) for d in drops]
    docs = []
    for path in files:
        with open(path) as f:
            raw = f.read()
        docs.append((path, raw, json.loads(raw)))
    # every name the files' old unresolved lists give, to tell a stale note by
    was_missing = {fold(u.replace(" (attackable)", "")): u.replace(" (attackable)", "")
                   for _, _, d in docs for q in d["quests"] for u in q.get("unresolved", [])}
    report, changed, renamed = [], {}, {}
    ck = cq.Check(names)
    for path, raw, d in docs:
        fname = os.path.basename(path)
        file_items = {fold(it["name"]) for it in d.get("items", []) if isinstance(it, dict) and it.get("name")}
        for q in d["quests"]:
            k = q.get("key", "?")
            for holder, field in references(q):
                n = holder[field]
                new = n
                for scope, old, to in renames:
                    if (not scope or k.startswith(scope)) and fold(n) == fold(old):
                        new = to
                        break
                else:
                    sp = by_key.get(key(n))
                    if fold(n) not in names.npc:
                        if sp and len({fold(s) for s in sp}) == 1:
                            new = sp.most_common(1)[0][0]
                    elif case and sp:
                        # found but for case: npcs' one spelling of this very name
                        same = {s for s in sp if fold(s) == fold(n)}
                        if len(same) == 1:
                            new = same.pop()
                if new != n:
                    holder[field] = new
                    renamed[fold(n)] = new
                    report.append(f"{fname} {k}: {field} {n!r} -> {new!r}")
            for holder, field in faction_references(q):
                n = holder[field]
                if names.faction is None or fold(n) in names.faction:
                    continue
                sp = factions_by_key.get(key(n))
                if sp and len({fold(x) for x in sp}) == 1:
                    new = sp.most_common(1)[0][0]
                    holder[field] = new
                    report.append(f"{fname} {k}: faction {n!r} -> {new!r}")
            ck.quest(fname, q, file_items)
            miss = ck.misses.get(k, [])
            want = {fold(m) for m in miss}
            old = list(q.get("unresolved", []))
            un, seen = [], set()
            for u in old + miss:
                if fold(u) in want and fold(u) not in seen:
                    un.append(u)
                    seen.add(fold(u))
            if un != old:
                report.append(f"{fname} {k}: unresolved {old} -> {un}")
            if un or "unresolved" in q:
                q["unresolved"] = un
    # the notes, once every file's unresolved is new (a chain may cross files)
    drafts = {}
    keys = {q.get("key"): q for _, _, dd in docs for q in dd["quests"]}
    mine = {os.path.abspath(p) for p, _, _ in docs}
    for f in sorted({g for p, _, _ in docs for g in glob.glob(os.path.join(os.path.dirname(os.path.abspath(p)), "*.json"))}):
        if f not in mine:   # the other files beside them, as they are
            with open(f) as fh:
                for q in json.load(fh).get("quests", []):
                    keys.setdefault(q.get("key"), q)

    def found(n):
        return fold(n) in names.npc or fold(renamed.get(fold(n), "")) in names.npc

    def draft(k, seen=()):
        if k in drafts:
            return drafts[k]
        q = keys.get(k)
        if q is None or k in seen:
            return True
        drafts[k] = bool(q.get("unresolved")) or any(draft(r, seen + (k,)) for r in q.get("requires", []))
        return drafts[k]
    for path, raw, d in docs:
        fname = os.path.basename(path)
        for q in d["quests"]:
            notes = q.get("notes", "")
            if not notes:
                continue
            k = q.get("key", "?")
            still = {fold(u.replace(" (attackable)", "")) for u in q.get("unresolved", [])}
            parts, dropped = [], False
            for s in sentences(notes):
                why = None
                m = MISSING_ONLY.match(s)
                if m:
                    subj = m.group("subj")
                    named = [n for f, n in was_missing.items() if mentions(subj, n)]
                    # each proper name it gives is one found (or a part of one: "Duchess
                    # Melnorn"), so a name no file lists ("Phil Wiggins") keeps it
                    known = [key(n).split() for n in was_missing.values() if found(n)]
                    proper = [key(re.sub(r"^The\s+", "", p)).split() for p in PROPER.findall(subj)]
                    if named and all(found(n) for n in named) and \
                            all(not w or any(set(w) <= set(k) for k in known) for w in proper) and \
                            not any(mentions(subj, u) for u in still):
                        why = "its names are found"
                elif CHAIN.match(s):
                    # the chain skipped its drafts (the quest requires the last whole one):
                    # once they are whole, it requires the quest right before it again
                    skipped = [k.rsplit("/", 1)[0] + "/" + lv.strip() for lv in CHAIN.match(s).group("skipped").split(",")]
                    if all(sk in keys and not draft(sk) for sk in skipped):
                        if q.get("requires") != [skipped[-1]]:
                            report.append(f"{fname} {k}: requires {q.get('requires')} -> {[skipped[-1]]}")
                            q["requires"] = [skipped[-1]]
                        why = "the quests it skipped are whole"
                if why is None and any(p.search(s) for p in drops):
                    why = "--drop-note"
                if why:
                    report.append(f"{fname} {k}: notes drop ({why}): {s}")
                    dropped = True
                    continue
                if MISSING_ANY.search(s) and any(mentions(s, n) and found(n) for n in was_missing.values()):
                    report.append(f"{fname} {k}: notes still say a found name is missing, word it again: {s}")
                parts.append((s, dropped))
                dropped = False
            if any(p[1] for p in parts) or dropped:
                q["notes"] = join(parts)
        out = json.dumps(d, indent=1, ensure_ascii=False) + ("\n" if raw.endswith("\n") else "")
        if out != raw:
            changed[path] = out
    return report, changed


def main():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--db", required=True, help="the database the names resolve in (read only)")
    ap.add_argument("--rename", action="append", default=[], metavar="[KEY:]OLD=NEW")
    ap.add_argument("--drop-note", action="append", default=[], metavar="REGEX",
                    help="also drop the notes' sentences this matches")
    ap.add_argument("--case", action="store_true",
                    help="also respell a name npcs spells only otherwise in case (tools/fix_npc_name_case.py's)")
    ap.add_argument("--write", action="store_true", help="write the files (report only without)")
    ap.add_argument("files", nargs="*")
    a = ap.parse_args()
    files = a.files or sorted(glob.glob(os.path.join(HERE, "data", "*.json")))
    report, changed = fix(files, a.db, parse_renames(a.rename), a.drop_note, a.case)
    for line in report:
        print(line)
    for path, out in changed.items():
        if a.write:
            with open(path, "w") as f:
                f.write(out)
    print(f"{len(changed)} of {len(files)} files {'written' if a.write else 'would change (--write writes them)'}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
