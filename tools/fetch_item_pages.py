#!/usr/bin/env python3
"""Allakhazam's EQOA item pages, fetched from the Wayback Machine into a
folder of the operator's own, for tools/fix_weapon_types.py.

Allakhazam's EQOA item database (eqoa.allakhazam.com, its players'
submissions of 2003-2005), as the Wayback Machine archived it:
- **The item lists by slot** (itemslot.html?slot=Primary, Secondary and
  Held), each item's name and Allakhazam's own id.
- **A page an item** (item.html?eqoaitem=<id>): its slot, weapon type ("One
  Hand Blunt", "Two Hand Piercing", ...), damage, level, classes, races,
  condition and stats.

Nothing of it is in this repository: it is a third party's, and each
operator fetches their own (as the harvests and the disc stay local).

- **With --zones, the item lists by zone** (itemzone.html?area=<n>, each
  area's latest capture, mostly 2012's): every item the site knew by name
  and id, where the slot lists of 2003 hold the early ones only.

**The folder:**
- OUT/lists/slot-<slot>.html and OUT/pages/<id>.html are the captures as
  fetched.
- OUT/lists.json ({name: [[id, slot], ...]}) and OUT/pages.json ({id: {field:
  value}}) are what was read off them.
- OUT/cdx.json says which capture each page is, off the archive's capture
  index (asked once).

A page is one request for its exact capture, the one nearest 2005. The
archive refuses a burst (about 15 requests a minute), so the default pace is
one fetch every 5 s. A refusal waits a minute, then longer, before trying
again (fetch_spell_pages.py's fetch). A capture already in OUT is not fetched
again, so a run that stops is resumed by running it again.

**Which pages:**
- With --db, those the lists name of the database's weapons of no attack
  type (a slot 12, 14, 15 or 16 item with damage and attack type 0; 13 is a shield's).
- With --controls, also those of the melee and ranged weapons Sandstorm's
  records hold (eqoa harvest JSON). Sandstorm's types are the measure
  fix_weapon_types.py holds Allakhazam's to.
- With neither, every listed item.

    tools/fetch_item_pages.py --out ~/.cache/eqoa-item-pages [--db eqoa.db] [--controls <harvests> ...] [--zones] [--lists-only] [--delay 5]
"""
import argparse
import glob
import html
import json
import os
import re
import sqlite3
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from fetch_spell_pages import fetch, read_file  # noqa: E402

SITE = "http://eqoa.allakhazam.com:80"
SLOTS = ("Primary", "Secondary", "Held")
# the fields a page names, as its text has them
FIELDS = ["Slot", "Weapon Type", "DAMAGE", "Item Level", "Max Item HP", "Durability", "Classes", "Races",
          "Description"]
STATS = ["STR", "STA", "AGI", "DEX", "WIS", "INT", "CHA", "HP", "POWER", "AC", "HoT", "PoT"]


def key(name):
    return " ".join(html.unescape(name or "").replace("`", "'").replace("’", "'").lower().split())


def text_of(page):
    t = re.sub(r"<script.*?</script>", " ", page, flags=re.S)
    t = re.sub(r"<[^>]+>", " | ", t)
    t = html.unescape(re.sub(r"\s+", " ", t))
    return re.sub(r"(\| )+", "| ", t)


def read_list(page):
    """[(id, name)] off a list page."""
    return [(int(i), html.unescape(n).strip()) for i, n in re.findall(r"item\.html\?eqoaitem=(\d+)>([^<]+)<", page)]


def read_page(page):
    """{field: value} off an item page, or None when it is no item's."""
    t = text_of(page)
    m = re.search(r"Item: ([^|]+)\|", t)
    if not m:
        return None
    body = t[m.end():]
    end = body.find("Forum Preferences")
    body = body[:end] if end >= 0 else body
    out = {"name": m.group(1).strip()}
    names = FIELDS + STATS
    for f in names:
        mm = re.search(r"\| " + re.escape(f) + r" \| : (.*?)(?=\| (?:" + "|".join(re.escape(g) for g in names)
                       + r"|Submitted by|\[) \|)", "| " + body)
        if mm:
            v = mm.group(1).replace("|", " ")
            v = " ".join(v.split()).strip(" ,")
            out[f] = re.sub(r"\s*,\s*", ", ", v)
    for flag in ("MAGIC", "LORE", "NO TRADE"):
        if re.search(r"\|\s*" + flag + r"\s*\|", "| " + body[:200]):
            out.setdefault("flags", []).append(flag)
    return out


def captures(out, delay):
    """{id: [(timestamp, url)]}: each item page's captures with a 200, off the
    archive's capture index, asked once and kept in OUT/cdx.json."""
    path = f"{out}/cdx.json"
    if not os.path.exists(path):
        q = ("https://web.archive.org/cdx/search/cdx?url=eqoa.allakhazam.com/item.html&matchType=prefix"
             "&filter=statuscode:200&fl=timestamp,original&output=json")
        if not fetch(q, path + ".raw"):
            sys.exit("the archive's capture index did not answer")
        with open(path + ".raw") as f:
            rows = json.load(f)
        by = {}
        for ts, orig in rows[1:]:
            m = re.search(r"[?&]eqoaitem=(\d+)", orig)
            if m:
                by.setdefault(m.group(1), []).append([ts, orig])
        with open(path, "w") as f:
            json.dump(by, f)
        os.remove(path + ".raw")
        time.sleep(delay)
    with open(path) as f:
        return {int(k): v for k, v in json.load(f).items()}


def best(caps, iid):
    """The capture of a page to read: the plain page's (no comment view)
    nearest 2005."""
    def plain(c):
        return c[1].endswith(f"eqoaitem={iid}")
    order = sorted(caps, key=lambda c: (not plain(c), abs(int(c[0][:4]) - 2005), c[0]))
    return order[0] if order else None


def list_captures(out, delay):
    """{slot: (timestamp, url)} of the slot lists, off the capture index."""
    path = f"{out}/lists/cdx.json"
    if not os.path.exists(path):
        q = ("https://web.archive.org/cdx/search/cdx?url=eqoa.allakhazam.com/itemslot.html*"
             "&filter=statuscode:200&fl=timestamp,original&output=json")
        if not fetch(q, path + ".raw"):
            sys.exit("the archive's capture index did not answer")
        with open(path + ".raw") as f:
            rows = json.load(f)
        by = {}
        for ts, orig in rows[1:]:
            m = re.search(r"slot=(\w+)$", orig)
            if m:
                by.setdefault(m.group(1), []).append([ts, orig])
        with open(path, "w") as f:
            json.dump(by, f)
        os.remove(path + ".raw")
        time.sleep(delay)
    with open(path) as f:
        return json.load(f)


def zone_captures(out, delay):
    """{area: [timestamp, url]}: each zone list's latest capture with a 200,
    off the capture index, asked once and kept in OUT/lists/zones-cdx.json."""
    path = f"{out}/lists/zones-cdx.json"
    if not os.path.exists(path):
        q = ("https://web.archive.org/cdx/search/cdx?url=eqoa.allakhazam.com/itemzone.html*"
             "&filter=statuscode:200&fl=timestamp,original&output=json")
        if not fetch(q, path + ".raw"):
            sys.exit("the archive's capture index did not answer")
        with open(path + ".raw") as f:
            rows = json.load(f)
        by = {}
        for ts, orig in rows[1:]:
            m = re.search(r"[?&]area=(\d+)$", orig)
            if m and (m.group(1) not in by or ts > by[m.group(1)][0]):
                by[m.group(1)] = [ts, orig]
        with open(path, "w") as f:
            json.dump(by, f)
        os.remove(path + ".raw")
        time.sleep(delay)
    with open(path) as f:
        return json.load(f)


def wanted_names(db_path, controls):
    """The names to fetch: the database's weapons of no attack type, and
    Sandstorm's weapons' names (--controls). None: every listed item."""
    if not db_path and not controls:
        return None
    names = set()
    if db_path:
        db = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True)
        try:
            for (n,) in db.execute("SELECT itemname FROM itempattern WHERE weapondamage > 0 AND attacktype = 0 "
                                   "AND equipslot IN (12, 14, 15, 16)"):
                names.add(key(n))
        finally:
            db.close()
    files = []
    for p in controls:
        files += glob.glob(os.path.join(p, "**", "*.json"), recursive=True) if os.path.isdir(p) else [p]

    def walk(o):
        if isinstance(o, dict):
            if o.get("name") and isinstance(o.get("attack_type"), int) and 1 <= o["attack_type"] <= 10 \
                    and o.get("damage", 0) > 0:
                yield o
            for v in o.values():
                yield from walk(v)
        elif isinstance(o, list):
            for v in o:
                yield from walk(v)

    for f in files:
        try:
            with open(f) as fh:
                d = json.load(fh)
        except (OSError, ValueError):
            continue
        for w in walk(d):
            names.add(key(w["name"]))
    return names


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--out", required=True, help="the folder to fetch into (yours: nothing of it is committed)")
    ap.add_argument("--db", help="fetch only the pages of this database's weapons of no attack type (and --controls)")
    ap.add_argument("--controls", nargs="*", default=[], help="eqoa harvest JSON: Sandstorm's weapons' pages too")
    ap.add_argument("--zones", action="store_true", help="the item lists by zone too")
    ap.add_argument("--lists-only", action="store_true", help="fetch the lists and stop")
    ap.add_argument("--delay", type=float, default=5.0, help="seconds between fetches (default 5)")
    a = ap.parse_args()
    out = os.path.expanduser(a.out)
    os.makedirs(f"{out}/lists", exist_ok=True)
    os.makedirs(f"{out}/pages", exist_ok=True)

    caps = list_captures(out, a.delay)
    lists = {}
    for slot in SLOTS:
        path = f"{out}/lists/slot-{slot}.html"
        if not os.path.exists(path):
            c = sorted(caps.get(slot, []), key=lambda c: c[0])
            if not c:
                print(f"no capture of the {slot} list")
                continue
            ts, url = c[-1]
            if not fetch(f"https://web.archive.org/web/{ts}id_/{url}", path):
                print(f"the {slot} list did not come")
                continue
            time.sleep(a.delay)
        for iid, name in read_list(read_file(path)):
            lists.setdefault(key(name), [])
            if [iid, slot] not in lists[key(name)]:
                lists[key(name)].append([iid, slot])
    if a.zones:
        zones = zone_captures(out, a.delay)
        for n, area in enumerate(sorted(zones, key=int), 1):
            path = f"{out}/lists/zone-{area}.html"
            if not os.path.exists(path):
                ts, url = zones[area]
                if not fetch(f"https://web.archive.org/web/{ts}id_/{url}", path):
                    print(f"the list of area {area} did not come")
                    continue
                time.sleep(a.delay)
            for iid, name in read_list(read_file(path)):
                lists.setdefault(key(name), [])
                if iid not in [e[0] for e in lists[key(name)]]:
                    lists[key(name)].append([iid, "zone"])
            if n % 20 == 0:
                print(f"  {n} of {len(zones)} zone lists", flush=True)
    with open(f"{out}/lists.json", "w") as f:
        json.dump(lists, f, indent=1, sort_keys=True)
    print(f"{len(lists)} names on the lists", flush=True)
    if a.lists_only:
        return

    want = wanted_names(a.db, a.controls)
    ids = sorted({iid for k, v in lists.items() if want is None or k in want for iid, _ in v})
    page_caps = captures(out, a.delay)
    pages_path = f"{out}/pages.json"
    pages = {}
    if os.path.exists(pages_path):
        with open(pages_path) as f:
            pages = json.load(f)
    missing = []
    for n, iid in enumerate(ids, 1):
        path = f"{out}/pages/{iid}.html"
        if not os.path.exists(path):
            c = best(page_caps.get(iid, []), iid)
            if not c:
                missing.append(iid)
                continue
            ok = fetch(f"https://web.archive.org/web/{c[0]}id_/{c[1]}", path)
            time.sleep(a.delay)
            if not ok:
                missing.append(iid)
                continue
        if str(iid) not in pages:
            p = read_page(read_file(path))
            if p:
                pages[str(iid)] = p
        if n % 20 == 0:
            print(f"  {n} of {len(ids)}", flush=True)
            with open(pages_path, "w") as f:
                json.dump(pages, f, indent=1, sort_keys=True)
    with open(pages_path, "w") as f:
        json.dump(pages, f, indent=1, sort_keys=True)
    with open(f"{out}/missing.json", "w") as f:
        json.dump(missing, f)
    print(f"{len(ids)} pages wanted, {len(pages)} read, {len(missing)} with no capture or none came")


if __name__ == "__main__":
    main()
