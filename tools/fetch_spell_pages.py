#!/usr/bin/env python3
"""Allakhazam's EQOA spell pages, fetched from the Wayback Machine into a
folder of the operator's own, for tools/sweep_spell_pages.py.

Allakhazam's EQOA spell database (eqoa.allakhazam.com, its import of the
game's records of 2004-04-28) as the Wayback Machine archived it: the fifteen
class lists (spell.html?scl=<n>, 2012's captures) and a page a spell
(spell.html?eqoaspell=<id>, the capture nearest 2006, else nearest 2012), each
with its class and level, casting time, recast, power cost, scope, duration,
resist, AE radius, hate and effect. Nothing of it is in this repository: it is
a third party's, and each operator fetches their own (as the harvests and the
disc stay local).

OUT/lists/scl<n>.html and OUT/pages/<id>.html are the captures as fetched;
OUT/lists.json ({class: [[id, name, level], ...]}) and OUT/pages.json ({id:
{field: value}}) what was read off them. Which capture each page is comes off
the archive's capture index (its CDX server, asked once: OUT/cdx.json), so a
page is one request for its exact capture and a page with none costs none
(listed in OUT/missing.json). The archive refuses a burst (about 15 requests
a minute): the default pace is one fetch every 5 s, and a refusal waits a
minute, then longer, before trying again. A capture already in OUT is not
fetched again, so a run that stops is resumed by running it again. With --db only the
pages of the spells the database's imported records name (tools/
import_eqoadata_spells.py's and its mastery sibling's: spellid 700001 and up,
by their classes and names) are fetched; without it every listed spell.

    tools/fetch_spell_pages.py --out ~/.cache/eqoa-spell-pages [--db eqoa.db] [--delay 1.5]
"""
import argparse
import html
import json
import os
import re
import sqlite3
import subprocess
import sys
import time

LISTS = {1: "Bard", 2: "Cleric", 3: "Druid", 4: "Enchanter", 5: "Magician", 6: "Monk", 7: "Necromancer",
         8: "Paladin", 9: "Ranger", 10: "Rogue", 11: "Shadowknight", 12: "Shaman", 13: "Warrior", 14: "Wizard",
         15: "Alchemist"}
# the server's class numbers (WAR=0 ... ALC=14)
CLASS_NUMBER = {"Warrior": 0, "Ranger": 1, "Paladin": 2, "Shadowknight": 3, "Monk": 4, "Bard": 5, "Rogue": 6,
                "Druid": 7, "Shaman": 8, "Cleric": 9, "Magician": 10, "Necromancer": 11, "Enchanter": 12,
                "Wizard": 13, "Alchemist": 14}
LISTS_BY_NUMBER = {n: cname for cname, n in CLASS_NUMBER.items()}
SITE = "http://eqoa.allakhazam.com/spell.html"
FIELDS = ["Effect", "Class", "Casting Time", "Recast", "Power Cost", "Scope", "Duration", "Resist",
          "AE Radius", "Hate Rating", "Last Import Date"]


def norm(name):
    """A spell's name compared: lower case, one kind of apostrophe, no other
    punctuation, single spaces."""
    s = (name or "").lower().replace("’", "'").replace("`", "'")
    return re.sub(r"\s+", " ", re.sub(r"[^a-z0-9' ]", " ", s)).strip()


def fetch(url, path, refused=None):
    """The capture at url into path; whether it is a page (not the archive's
    error or an empty answer). A refusal (no answer, 429 or a 5xx) waits and
    tries again, a minute, then two, then five, then gives up."""
    tmp = path + ".part"
    for wait in (60, 120, 300, None):
        r = subprocess.run(["curl", "-s", "-L", "-m", "90", "-o", tmp, "-w", "%{http_code}", url],
                           capture_output=True, text=True)
        code = r.stdout.strip()
        if code in ("000", "429") or code.startswith("5"):
            if os.path.exists(tmp):
                os.remove(tmp)
            if wait is None:
                sys.exit(f"the archive refuses ({code}) after four tries: run again later to go on")
            print(f"  the archive answered {code}: waiting {wait}s", flush=True)
            time.sleep(wait)
            continue
        break
    ok = code == "200" and os.path.exists(tmp) and os.path.getsize(tmp) > 2000
    if ok:
        os.replace(tmp, path)
    elif os.path.exists(tmp):
        os.remove(tmp)
    return ok


def captures(out, delay):
    """{page id: [(timestamp, url)]}: each spell page's captures the archive
    holds with a 200, off its capture index, asked once and kept in
    OUT/cdx.json."""
    path = f"{out}/cdx.json"
    if not os.path.exists(path):
        q = ("https://web.archive.org/cdx/search/cdx?url=eqoa.allakhazam.com/spell.html&matchType=prefix"
             "&filter=statuscode:200&fl=timestamp,original&output=json")
        if not fetch(q, path + ".raw"):
            sys.exit("the archive's capture index did not answer")
        rows = json.load(open(path + ".raw"))
        by = {}
        for ts, orig in rows[1:]:
            m = re.search(r"[?&]eqoaspell=(\d+)", orig)
            if m:
                by.setdefault(m.group(1), []).append([ts, orig])
        json.dump(by, open(path, "w"))
        os.remove(path + ".raw")
        time.sleep(delay)
    return {int(k): v for k, v in json.load(open(path)).items()}


def best(caps):
    """The capture to read of a page's: the plain page's (no comment view)
    nearest 2006, else any nearest 2006."""
    def plain(c):
        return re.search(r"spell\.html\?eqoaspell=\d+$", c[1]) is not None
    order = sorted(caps, key=lambda c: (not plain(c), abs(int(c[0][:4]) - 2006), c[0]))
    return order[0] if order else None


def text_of(page):
    t = re.sub(r"<script.*?</script>", " ", page, flags=re.S)
    t = re.sub(r"<[^>]+>", " | ", t)
    t = html.unescape(re.sub(r"\s+", " ", t))
    return re.sub(r"(\| )+", "| ", t)


def read_file(path):
    with open(path, errors="replace") as f:
        return f.read()


def read_list(page):
    """A class list's spells: [[id, name, level]]."""
    out = []
    for m in re.finditer(r'eqoaspell=(\d+)">([^<]+)</a></td><td>(\d+)</td>', page):
        out.append([int(m.group(1)), html.unescape(m.group(2)).strip(), int(m.group(3))])
    return out


def read_page(page):
    """A spell page's fields, or None when it holds none."""
    t = text_of(page)
    rec = {}
    m = re.search(r"\| ([^|]+?) :: Spells :: EverQuest", t)
    if m:
        rec["name"] = m.group(1).strip()
    for k, nxt in zip(FIELDS, FIELDS[1:] + ["Report Correction"]):
        mm = re.search(re.escape(k) + r": \| (.*?)\| " + re.escape(nxt), t)
        if mm:
            rec[k] = mm.group(1).strip(" |")
    if "Class" not in rec or "Scope" not in rec:
        return None
    rec["effects"] = [e.strip() for e in rec.pop("Effect", "").split("|") if e.strip()]
    cm = re.match(r"(.+?), level (\d+)$", rec["Class"])
    if cm:
        rec["class"], rec["level"] = cm.group(1).strip(), int(cm.group(2))
    return rec


def wanted(db_path, lists):
    """The page ids the database's imported records name, by their classes
    and names (the level where a list names a spell twice)."""
    by = {}
    for cname, rows in lists.items():
        for sid, name, lvl in rows:
            by.setdefault((CLASS_NUMBER[cname], norm(name)), []).append((sid, lvl))
    db = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True)
    ids = set()
    for name, lvl, cls in db.execute("""SELECT p.spellname, p.abilitylvl, c.class FROM spellpattern p
            JOIN classspells c ON c.spellid = p.spellid WHERE p.spellid >= 700001"""):
        hits = by.get((cls, norm(name)), [])
        same = [h for h in hits if h[1] == lvl]
        for sid, _ in (same or hits):
            ids.add(sid)
    return ids


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--out", required=True)
    ap.add_argument("--db", help="fetch only the pages this database's imported records name")
    ap.add_argument("--delay", type=float, default=5, help="seconds between two fetches (default 5)")
    a = ap.parse_args()
    out = os.path.expanduser(a.out)
    os.makedirs(f"{out}/lists", exist_ok=True)
    os.makedirs(f"{out}/pages", exist_ok=True)

    lists = {}
    for n, cname in LISTS.items():
        path = f"{out}/lists/scl{n}.html"
        if not os.path.exists(path):
            if not fetch(f"https://web.archive.org/web/2012id_/{SITE}?scl={n}", path):
                sys.exit(f"no capture of the {cname} list (scl={n})")
            time.sleep(a.delay)
        lists[cname] = read_list(read_file(path))
    json.dump(lists, open(f"{out}/lists.json", "w"), indent=1)
    print(f"lists: {sum(len(v) for v in lists.values())} spells in {len(lists)} classes")

    ids = wanted(a.db, lists) if a.db else {sid for rows in lists.values() for sid, _, _ in rows}
    todo = sorted(i for i in ids if not os.path.exists(f"{out}/pages/{i}.html"))
    caps = captures(out, a.delay)
    missing = [i for i in todo if not caps.get(i)]
    todo = [i for i in todo if caps.get(i)]
    print(f"pages: {len(ids)} wanted, {len(ids) - len(todo) - len(missing)} here, {len(missing)} with no capture, "
          f"{len(todo)} to fetch")
    for n, sid in enumerate(todo, 1):
        path = f"{out}/pages/{sid}.html"
        ts, orig = best(caps[sid])
        if not fetch(f"https://web.archive.org/web/{ts}id_/{orig}", path) or \
                not read_page(read_file(path)):
            missing.append(sid)
        time.sleep(a.delay)
        if n % 50 == 0 or n == len(todo):
            print(f"  {n} of {len(todo)} fetched, {len(missing)} with no record", flush=True)

    pages, empty = {}, []
    for sid in sorted(ids):
        path = f"{out}/pages/{sid}.html"
        if not os.path.exists(path):
            continue
        rec = read_page(read_file(path))
        if rec is None:
            empty.append(sid)
            continue
        pages[str(sid)] = rec
    json.dump(pages, open(f"{out}/pages.json", "w"), indent=1, sort_keys=True)
    json.dump(sorted(set(missing) | set(empty)), open(f"{out}/missing.json", "w"))
    print(f"read {len(pages)} pages; {len(set(missing) | set(empty))} with no capture or no record (missing.json)")


if __name__ == "__main__":
    main()
