#!/usr/bin/env python3
"""Check quest content files (tools/quests/FORMAT.md) against a database.

    python3 tools/quests/check_quests.py --db eqoa.db [file.json ...]

With no files it checks every tools/quests/data/*.json. It checks each file's
shape, the chains (every `requires` names a quest of some file, no cycles),
the lengths CLIENT's windows take (handlers/quests.go: a box's paragraph 218
characters, an answer 63, a journal line 127), and, with --db, every name the
way the game matches one (case and surrounding spaces aside): givers and step
targets in npcs, step and reward items in itempattern or the file's `items`,
reward spells in spellpattern, a kill or loot target that can be attacked
(not npc_type 0x80). A quest's `unresolved` must list exactly the names that
do not resolve ("<name> (attackable)" for a target no one can attack). It
prints a summary per file and exits 1 on any error.
"""
import argparse
import collections
import glob
import json
import os
import sqlite3
import sys

KINDS = {'talk', 'show', 'give', 'kill', 'loot', 'reach'}
EVENT_ON = {'accept', 'finish', 'start', 'done'}
EVENT_DO = {'popup', 'say', 'give', 'spawn', 'visual', 'teleport'}
BOX, ANSWER, LINE = 218, 63, 127
TEXT = ('offer', 'accept', 'decline', 'declined', 'started')


def fold(s):
    return (s or '').strip().lower()


class Names:
    def __init__(self, path):
        db = sqlite3.connect('file:' + path + '?mode=ro', uri=True)
        self.npc = collections.defaultdict(list)
        for name, t in db.execute('SELECT npc_name, npc_type FROM npcs WHERE npc_name IS NOT NULL'):
            self.npc[fold(name)].append(t or 0)
        self.item = {fold(n) for (n,) in db.execute('SELECT itemname FROM itempattern')}
        self.spell = {fold(n) for (n,) in db.execute('SELECT spellname FROM spellpattern')}
        db.close()

    def attackable(self, name):
        return any(t & 0x80 == 0 for t in self.npc.get(fold(name), []))


class Check:
    def __init__(self, names):
        self.names = names
        self.errors = []
        self.warnings = []
        self.misses = {}   # quest key -> what of it does not resolve (with --db), as `unresolved` must list it

    def err(self, where, msg):
        self.errors.append(f'{where}: {msg}')

    def warn(self, where, msg):
        self.warnings.append(f'{where}: {msg}')

    def paragraphs(self, where, field, text, limit):
        for p in [p.strip() for p in (text or '').split('\n\n') if p.strip()]:
            if len(p) > limit:
                self.warn(where, f'{field} paragraph is {len(p)} characters (a window takes {limit})')

    def items_of(self, where, lst):
        out = []
        if not isinstance(lst, list):
            self.err(where, 'items is not a list')
            return out
        for it in lst:
            if not isinstance(it, dict) or not isinstance(it.get('item'), str) or not it['item'].strip():
                self.err(where, f'bad item entry {it!r}')
                continue
            if not isinstance(it.get('count', 1), int) or it.get('count', 1) < 1:
                self.err(where, f"bad count in {it!r}")
            out.append(it['item'])
        return out

    def event(self, where, ev, allowed_on, npcs, items):
        if not isinstance(ev, dict):
            self.err(where, f'event {ev!r} is not an object')
            return
        if ev.get('on') not in allowed_on:
            self.err(where, f"event on {ev.get('on')!r} (here: {sorted(allowed_on)})")
        do = ev.get('do')
        if do not in EVENT_DO:
            self.err(where, f'event does {do!r}')
        if do in ('popup', 'say') and not (ev.get('text') or '').strip():
            self.err(where, f'{do} event without text')
        if do == 'say' or do == 'spawn':
            if not (ev.get('npc') or '').strip():
                self.err(where, f'{do} event without npc')
            else:
                npcs.append(ev['npc'])
        if do == 'give':
            items += self.items_of(where, ev.get('items', []))
        if do == 'popup':
            self.paragraphs(where, 'popup', ev.get('text'), BOX)

    def quest(self, fname, q, file_items):
        key = q.get('key', '?')
        where = f'{fname} {key}'
        for f in ('key', 'name', 'giver'):
            if not isinstance(q.get(f), str) or not q[f].strip():
                self.err(where, f'{f} missing')
        for f in ('classes', 'races', 'requires', 'steps', 'unresolved'):
            if not isinstance(q.get(f, []), list):
                self.err(where, f'{f} is not a list')
        for c in q.get('classes', []):
            if not isinstance(c, int) or not 0 <= c <= 14:
                self.err(where, f'class {c!r}')
        for r in q.get('races', []):
            if not isinstance(r, int) or not 0 <= r <= 10:
                self.err(where, f'race {r!r}')
        lo, hi = q.get('minlevel', 1), q.get('maxlevel', 60)
        if not (isinstance(lo, int) and isinstance(hi, int) and 1 <= lo <= hi <= 60):
            self.err(where, f'levels {lo!r}..{hi!r}')
        for f in TEXT:
            if not isinstance(q.get(f, ''), str):
                self.err(where, f'{f} is not text')
        self.paragraphs(where, 'offer', q.get('offer'), BOX)
        self.paragraphs(where, 'declined', q.get('declined'), BOX)
        self.paragraphs(where, 'started', q.get('started'), BOX)
        for f in ('accept', 'decline'):
            if len((q.get(f) or '').strip()) > ANSWER:
                self.warn(where, f'{f} is longer than an answer ({ANSWER})')
        npcs, items, spells, unattackable = [q.get('giver', '')], [], [], []
        steps = q.get('steps') or []
        if not steps:
            self.err(where, 'no steps')
        for i, st in enumerate(steps, 1):
            sw = f'{where} step {i}'
            if not isinstance(st, dict):
                self.err(sw, 'not an object')
                continue
            kind = st.get('kind')
            if kind not in KINDS:
                self.err(sw, f'kind {kind!r}')
                continue
            if kind != 'reach':
                if not (st.get('target') or '').strip():
                    self.err(sw, 'no target')
                else:
                    npcs.append(st['target'])
                    if kind in ('kill', 'loot') and self.names and fold(st['target']) in self.names.npc \
                            and not self.names.attackable(st['target']):
                        unattackable.append(st['target'])
            if kind in ('show', 'loot') and not st.get('items'):
                self.err(sw, f'{kind} without items')
            if kind == 'give' and not st.get('items') and not st.get('tunar'):
                self.err(sw, 'give without items or tunar')
            if kind == 'loot' and len(st.get('items', [])) != 1:
                self.err(sw, 'loot takes one item')
            items += self.items_of(sw, st.get('items', []))
            if kind in ('kill', 'loot') and (not isinstance(st.get('count', 1), int) or st.get('count', 1) < 1):
                self.err(sw, f"count {st.get('count')!r}")
            if kind == 'loot':
                c = st.get('chance')
                if not isinstance(c, (int, float)) or not 0 < c <= 1:
                    self.err(sw, f'chance {c!r}')
                if st.get('items') and st['items'][0].get('count', 1) != st.get('count', 1):
                    self.warn(sw, 'loot count and item count differ')
            if kind == 'reach':
                for f in ('world', 'x', 'y', 'z', 'radius'):
                    if not isinstance(st.get(f), (int, float)):
                        self.err(sw, f'reach without {f}')
            if not isinstance(st.get('tunar', 0), int) or st.get('tunar', 0) < 0:
                self.err(sw, f"tunar {st.get('tunar')!r}")
            if not (st.get('journal') or '').strip():
                self.err(sw, 'no journal line')
            elif len(st['journal'].strip()) > LINE:
                self.warn(sw, f'journal line is {len(st["journal"].strip())} characters (the journal takes {LINE})')
            for f in ('says', 'waiting', 'notyet', 'reminder'):
                self.paragraphs(sw, f, st.get(f), BOX)
            rw = st.get('rewards')
            if rw is not None:
                if not isinstance(rw, dict):
                    self.err(sw, 'rewards is not an object')
                else:
                    for f in ('xp', 'tunar'):
                        if not isinstance(rw.get(f, 0), int) or rw.get(f, 0) < 0:
                            self.err(sw, f'reward {f} {rw.get(f)!r}')
                    items += [i for i in rw.get('items', []) if isinstance(i, str)]
                    spells += [s for s in rw.get('spells', []) if isinstance(s, str)]
                    for fct in rw.get('factions', []):
                        if not (isinstance(fct, list) and len(fct) == 2 and isinstance(fct[0], str) and isinstance(fct[1], int)):
                            self.err(sw, f'faction {fct!r}')
            for ev in st.get('events', []):
                self.event(sw, ev, {'start', 'done'}, npcs, items)
        for ev in q.get('events', []):
            self.event(where, ev, {'accept', 'finish'}, npcs, items)
        if not any('rewards' in st for st in steps if isinstance(st, dict)):
            self.warn(where, 'no rewards on any step')
        if self.names:
            missing = []
            for n in npcs:
                if n and fold(n) not in self.names.npc:
                    missing.append(n)
            for n in items:
                if fold(n) not in self.names.item and fold(n) not in file_items:
                    missing.append(n)
            for n in spells:
                if fold(n) not in self.names.spell:
                    missing.append(n)
            missing += [f'{n} (attackable)' for n in unattackable]
            self.misses[key] = missing
            want = {fold(m) for m in missing}
            have = {fold(u) for u in q.get('unresolved', [])}
            for m in sorted(want - have):
                self.err(where, f'{m!r} does not resolve and is not in unresolved')
            for u in sorted(have - want):
                self.err(where, f'unresolved {u!r} resolves')
        return q.get('requires', [])

    def file(self, path, all_keys):
        fname = os.path.basename(path)
        try:
            doc = json.load(open(path))
        except (OSError, ValueError) as e:
            self.err(fname, f'cannot read: {e}')
            return None
        if not isinstance(doc, dict) or not isinstance(doc.get('quests'), list):
            self.err(fname, 'not a quest file (no quests list)')
            return None
        if not (doc.get('city') or '').strip():
            self.err(fname, 'no city')
        file_items = set()
        for it in doc.get('items', []):
            if not isinstance(it, dict) or not (it.get('name') or '').strip():
                self.err(fname, f'bad item {it!r}')
                continue
            if self.names and fold(it['name']) in self.names.item:
                self.warn(fname, f"item {it['name']!r} already exists in itempattern")
            if not (it.get('like') or '').strip():
                self.err(fname, f"item {it['name']!r} has no like")
            elif self.names and fold(it['like']) not in self.names.item:
                self.err(fname, f"item {it['name']!r}: like {it['like']!r} is not an item")
            file_items.add(fold(it['name']))
        for q in doc['quests']:
            k = q.get('key')
            if k in all_keys:
                self.err(fname, f'key {k!r} is used twice')
            all_keys[k] = (fname, q)
        return doc, file_items


def main():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument('--db', help='the database to resolve names in (read only)')
    ap.add_argument('files', nargs='*')
    a = ap.parse_args()
    files = a.files or sorted(glob.glob(os.path.join(os.path.dirname(os.path.abspath(__file__)), 'data', '*.json')))
    if not files:
        print('no quest files')
        return 1
    ck = Check(Names(a.db) if a.db else None)
    all_keys, docs = {}, []
    for f in files:
        got = ck.file(f, all_keys)
        if got:
            docs.append((f, got[0], got[1]))
    graph = {}
    for f, doc, fitems in docs:
        for q in doc['quests']:
            graph[q.get('key')] = ck.quest(os.path.basename(f), q, fitems)
    for k, reqs in graph.items():
        for r in reqs:
            if r not in graph:
                ck.err(k, f'requires {r!r}, which no file has')
    state = {}

    def visit(k, path):
        if state.get(k) == 1:
            ck.err(k, 'requires chain loops: ' + ' -> '.join(path + [k]))
            return
        if state.get(k) == 2:
            return
        state[k] = 1
        for r in graph.get(k, []):
            if r in graph:
                visit(r, path + [k])
        state[k] = 2

    for k in graph:
        visit(k, [])
    for f, doc, _ in docs:
        qs = doc['quests']
        whole = sum(1 for q in qs if not q.get('unresolved'))
        un = collections.Counter(u for q in qs for u in q.get('unresolved', []))
        print(f"{os.path.basename(f)}: {len(qs)} quests, {whole} resolve fully, "
              f"{len(un)} unresolved names{': ' + ', '.join(sorted(un)) if un else ''}")
    for w in ck.warnings:
        print('warning:', w)
    for e in ck.errors:
        print('ERROR:', e)
    print(f'{len(ck.errors)} errors, {len(ck.warnings)} warnings')
    return 1 if ck.errors else 0


if __name__ == '__main__':
    sys.exit(main())
