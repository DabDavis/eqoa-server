#!/usr/bin/env python3
"""held_barred_20261119.py: the boss and raid loot that no creature drops as what it holds (table heldbarred, read by
the server at its start: internal/database/helddrops.go). The owner, 2026-11-19: "[a player] killed a soluseks eye fire
goblin and waraxe of noroxxus dropped. staff of jade forest keeps dropping for players too much, etc. those ultra rare
items should drop less". Prints the candidates as patternid|name|level|why for a shard's database: hand and shield
records of level 45+ that the guides' raid and loot pages name (questmasters text pages titled Loot/Raid/Solusek's
Eye/Plane of/Isle of Dread/Sky/Nagafen/Vox; EQOA-Data's Portal raids; an item text saying Legendary or "from <boss>"),
leaving out any a world pool (uncommon, trash) or a loot table already places. ~/charmigrate/petfix470_20261119.py
writes them (petfix470_plan_<shard>.json); the server skips the rare pools and a named creature's own table loot by
itself. Reads only.

  held_barred_20261119.py <eqoa.db> [--guides DIR (default ~/claude-eqoa/external)] > candidates.txt
"""
import argparse
import sqlite3, sys, re, os, glob, json, collections
ap = argparse.ArgumentParser(); ap.add_argument('db'); ap.add_argument('--guides', default=os.path.expanduser('~/claude-eqoa/external')); A = ap.parse_args()
db = sqlite3.connect('file:' + A.db + '?mode=ro', uri=True)
qm = ''
for f in glob.glob(os.path.join(A.guides, 'questmasters/text/*.txt')):
    t = open(f, errors='ignore').read()
    title = t[:120].lower()
    if any(w in title for w in ('loot', 'raid', 'solukes eye', 'solusek', 'plane of', 'isle of dread', 'sky', 'nagafen', 'vox')):
        qm += '\n' + t.lower()
ed = ''
for f in glob.glob(os.path.join(A.guides, 'EQOA-Data/Quests/Portal/*.json')):
    ed += '\n' + open(f, errors='ignore').read().lower()
desc = {}
for f in glob.glob(os.path.join(A.guides, 'EQOA-Data/Information/databases/items/*.json')):
    d = json.load(open(f)); it = d if isinstance(d, list) else d.get('items', d)
    for x in (it if isinstance(it, list) else it.values()):
        if isinstance(x, dict) and x.get('name'): desc[x['name'].lower()] = x.get('description') or ''
world = {r[0] for r in db.execute("SELECT patternid FROM lootpool WHERE category IN ('uncommon','trash')")}
tabled = {r[0] for r in db.execute("SELECT patternid FROM loottables")}
out = []
for pid, name, slot, lvl in db.execute("SELECT patternid, itemname, equipslot, levelreq FROM itempattern WHERE equipslot IN (12,13,14,15,16,18) AND model<>0"):
    nl = (name or '').strip().lower()
    if len(nl) < 7 or pid in world or pid in tabled or (lvl or 0) < 45: continue
    dd = desc.get(nl, '')
    hit = []
    if nl in qm: hit.append('questmasters')
    if nl in ed: hit.append('eqoa-data raid')
    if re.search(r'legendary|\bfrom [A-Z][a-z]', dd): hit.append('desc: ' + dd[:50])
    if hit: out.append((pid, name, lvl, '; '.join(hit)))
for o in sorted(out, key=lambda x: (-x[2], x[1])): print('%d|%s|%d|%s' % o)
