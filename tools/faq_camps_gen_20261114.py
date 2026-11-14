"""faq_camps_gen_20261114.py: the plan of tools/faq_camps_20261114.py (tools/faq_camps_20261114.json)

Per shard (h, l), from that shard's own copy: every row written as [table, key, before, after] (before/after: the
columns the tool checks and writes; None = no row). New npcs rows copy a base row of the shard (every column), at
spots on the ground files' floors (~/.cache/movecheck/ground), level, size, body and name the camp's."""
import json, math, os, random, sqlite3, sys, collections
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import seed_wildlife as sw  # Ground
import seed_pod as sp      # Env

W = os.environ.get('FAQ_CAMPS_WORK', os.path.expanduser('~/.cache/tix5/camps-work'))  # h0.db, l0.db: the shards' copies
GROUND = os.path.expanduser('~/.cache/movecheck/ground')
OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'faq_camps_20261114.json')
SRC = 'faq-camps 20261114'
FIRST_ID = 190001
RESPAWN = 90.0

# a new kind: name, count, levels, body (model hex), base (a row of this name and body to copy: gear, flags), size,
# bind (the kind whose npcclass/npctemplate/lootcreature rows it takes), cls (npcclass override), profile (lootcreature)
CAMPS = [
 dict(key='frosteye', title='Frosteye Orcs (faq L5355): Snowblind Plains SE, the excavation site', world=0,
      at=(7571.3, 5687.0), y=80.6, spread=(25, 190), kinds=[
        dict(name='frosteye centurion', n=4, lv=(55, 57), model='8214A32D', base='frosteye alarmist', bind='frosteye miner', cls=0),
        dict(name='frosteye patroller', n=4, lv=(54, 56), model='84F47767', base='frosteye slayer', bind='frosteye miner', cls=0),
        dict(name='frosteye sage', n=3, lv=(54, 56), model='DA515962', base='frosteye warlock', bind='frosteye miner', cls=8)]),
 dict(key='taeew', title='Lizardmen - Aggressors (faq L5010): Stone Watchers NW', world=0,
      at=(18330.0, 30560.0), y=None, spread=(0, 160), box=(18000, 30000, 18680, 30680), kinds=[
        dict(name='Tae Ew aggressor', n=5, lv=(33, 37), model='2C1D8724', base='Tae Ew knight', bind='Tae Ew destroyer', cls=0),
        dict(name='Tae Ew sacrificer', n=3, lv=(34, 37), model='16BB9E24', base='Tae Ew witchdoctor', bind='Tae Ew destroyer', cls=8)]),
 dict(key='desertbandits', title='Desert Bandits (faq L5291): round Sothimus the Brigand in Tak Xiv', world=0,
      at=(19085.9, 25028.3), y=82.5, spread=(25, 150), kinds=[
        dict(name='desert henchman', n=4, lv=(60, 62), model='70D898C6', base='desert marauder', bind='froglok slayer', cls=0, region='desert'),
        dict(name='desert pillager', n=4, lv=(60, 62), model='70D898C6', base='desert marauder', bind='froglok slayer', cls=0, region='desert')],
      social='Desert Bandits'),
 dict(key='frogloks', title='Frogloks (faq L5342): Burial Mounds, round the froglok slayer', world=0,
      at=(18408.5, 29538.5), y=53.3, spread=(25, 170), kinds=[
        dict(name='froglok slayer', n=5, lv=(61, 63), model='141BFE0C', base='froglok slayer', bind=None),
        dict(name='Froglok Inquisitor', n=4, lv=(60, 62), model='429A04AB', base='froglok tal wizard', bind='froglok slayer', cls=9)]),
 dict(key='gharials', title="MTG Gharials (faq L5030): the island south of Muniel's Tea Garden", world=0,
      at=(23508.0, 20238.0), y=58.0, spread=(20, 140), kinds=[
        dict(name='ancient gharial', n=4, lv=(36, 38), model='015C345E', base='ancient gharial', bind=None),
        dict(name='Mudscale', n=1, lv=(39, 39), model='015C345E', base='ancient gharial', bind='ancient gharial', size=1.5, profile='named')]),
 dict(key='gargoyles', title='Gargoyles (faq L5126): South Barren Coast W, Garhuuga again', world=2,
      restore=['Garhuuga'], kinds=[]),
 dict(key='phantoms', title='Phantoms (faq L5224): West Plateau, Himlar and the elder spirits again, The Ancient One', world=2,
      at=(7060.0, 3368.0), y=None, spread=(40, 200), restore=['Himlar', 'elder spirit'], kinds=[
        dict(name='The Ancient One', n=1, lv=(56, 56), model='582BF657', base='revenant', bind='Himlar', cls=0, profile='named')]),
 dict(key='snowgriffons', title="Snow Griffons (faq L5242): Zentar's Keep NW, by Whitefeather", world=0,
      at=(4336.0, 6599.0), y=None, spread=(40, 160), kinds=[
        dict(name='Elder Squeeks', n=1, lv=(47, 47), model='41DD6575', base='snow griffon', bind='Whitefeather', size=1.4, profile='named')]),
 dict(key='tendorabuki', title='Tendora-Buki Villagers (faq L5250): West Toxxulia, the village', world=2,
      at=(5150.0, 5150.0), y=None, spread=(40, 330), kinds=[
        dict(name='Tendora-Buki guard', n=4, lv=(48, 50), model='0DDD4087', base='Tendora-Buki spahi', bind='Tendora-Buki fisherman', cls=0, size=1.2),
        dict(name='Tendora-Buki villager', n=4, lv=(46, 48), model='0DDD4087', base='Tendora-Buki fisherman', bind='Tendora-Buki fisherman', cls=0, size=1.1)]),
 dict(key='tfgs', title='TFGs (faq L5514): the frost giants south of Snowblind Plains, by the Arctic Spiritcaller', world=0,
      at=(6982.5, 5881.6), y=54.1, spread=(20, 120), kinds=[
        dict(name='Arctic Adept', n=1, lv=(62, 62), model='286C6E9E', base='Ice giant priest', bind='Arctic Spiritcaller', cls=13)]),
 dict(key='permafrost', title="Permafrost (faq L5566): frost drakes by the frost hounds before Vox's castle", world=0,
      at=(4728.8, 4416.0), y=61.6, spread=(30, 200), kinds=[
        dict(name='frost drake', n=4, lv=(50, 53), model='6595A50D', base='ice drake', bind='ice drake', cls=0)]),
 dict(key='quintashi', title="Quintashi's Lair (faq L5600): round Quintashi, under the Abysmal Sea", world=2,
      at=(4169.8, 8130.5), y=-155.0, spread=(40, 75), swim=True, wet=None, kinds=[
        dict(name="Quintashi's guardian", n=4, lv=(58, 60), model='A316EB78', base='merman', bind='merman', cls=0)]),
 dict(key='thehunt', title='The Hunt (faq L5650): south-west Odus', world=2,
      at=(5250.0, 10600.0), y=None, spread=(60, 420), kinds=[
        dict(name='plagued insect cloud', n=4, lv=(48, 52), model='0ED0E723', base='giant mosquito', bind='snapping tortoise', cls=0),
        dict(name='sabretooth huntress', n=4, lv=(50, 54), model='81193975', base='sabretooth jaguar', bind='snapping tortoise', cls=0),
        dict(name='vercozian wyrm', n=2, lv=(54, 56), model='363D65D7', base='forest wyrm', bind='snapping tortoise', cls=0)]),
]

# the wrong-level camps: (key, title, world, box (x0, z0, x1, z1) or None, names, (old lo, old hi) or None, (new lo, new hi))
LEVELS = [
 ('mossmouth', 'Mossmouth Goblins (faq L4870): Moss Mouth Cavern, 32-38 -> 21-30', 0, (18000, 12000, 20000, 14000),
  ['goblin gladiator', 'goblin brigand', 'goblin witch doctor', 'goblin scout', 'Companion Spirit'], (30, 38), (21, 30)),
 ('cracktooth', 'Cracktooth Orcs (faq L4830): Hodstock and Temby N, 12-16 -> 21-30', 0, (24000, 11000, 26000, 14000),
  ['cracktooth matron', 'cracktooth orc', 'cracktooth warrior', 'Biter Cracktooth'], (12, 16), (21, 30)),
 ('brokenskull', "Brokenskull Rock (faq L5094): the Tae Ew fort's lizardmen, 2 -> 41-50", 0, (20000, 30000, 22000, 32000),
  ['Tae Ew soldier', 'Tae Ew shaman', 'Feral Spirit'], None, (41, 50)),
 ('hobgoblins', "Fortress Hobgoblins (faq L5118): Cyclops' Fortress, 53-55 -> 46-50", 1, None,
  ['hobgoblin mercenary'], (53, 55), (46, 50)),
 ('gargoyles', 'Gargoyles (faq L5126): South Barren Coast, 46-55 -> 41-50', 2, (9900, 7900, 11900, 10000),
  ['gargoyle soldier', 'gargoyle watcher', 'leering gargoyle', 'statue guardian', 'gargoyle'], (46, 55), (41, 50)),
 ('ratmen', 'Ratmen (faq L5234): South Barren Coast N, 55-57 -> 48-50', 2, (10400, 6400, 11000, 6800),
  ['ratman fighter', 'ratman scavenger'], (55, 57), (48, 50)),
 ('kobolds', 'Kobolds (faq L5200): The Warrens, 51-55 -> 46-50', 2, (8000, 7900, 10000, 9900),
  ['clan kolbok scout', 'clan kolbok shaman'], (51, 55), (46, 50)),
 ('tendorabuki', 'Tendora-Buki (faq L5250): West Toxxulia, 51-53 -> 46-50', 2, None,
  ['Tendora-Buki amir', 'Tendora-Buki fisherman', 'Tendora-Buki ghulam', 'Tendora-Buki ispusar', 'Tendora-Buki mamluk',
   'Tendora-Buki pasdar', 'Tendora-Buki puzhal', 'Tendora-Buki spahi'], (51, 53), (46, 50)),
]
# in the Brokenskull fort every row is level 2: each kind its slice of the band
BAND_BY_NAME = {'Tae Ew soldier': (42, 46), 'Tae Ew shaman': (43, 47), 'Feral Spirit': (41, 45)}

BIND_TABLES = [('npcclass', 'name'), ('npctemplate', 'name'), ('lootcreature', 'npc_name')]


def rows(db, sql, a=()):
    cur = db.execute(sql, a)
    cols = [d[0] for d in cur.description]
    return [dict(zip(cols, r)) for r in cur.fetchall()]


def flat(g, x, z, y0, tol):
    """The floor at (x, z) nearest y0 (within tol), and level round it (within 2.5 over 6 units), else None."""
    fs = g.floors(x, z)
    if not fs:
        return None
    h = min(fs, key=lambda v: abs(v - y0))
    if abs(h - y0) > tol:
        return None
    for dx, dz in ((6, 0), (-6, 0), (0, 6), (0, -6)):
        f2 = g.floors(x + dx, z + dz)
        if not f2 or min(abs(v - h) for v in f2) > 2.5:
            return None
    return h


def place(g, env, camp, count, taken, others, rnd):
    ax, az = camp['at']
    y0 = camp['y']
    if y0 is None:
        fs = g.floors(ax, az)
        y0 = fs[-1] if fs else 54.0
        for r in range(0, 200, 10):
            fs = g.floors(ax + r, az)
            if fs:
                y0 = min(fs, key=lambda v: abs(v - 60)); break
    tol = camp.get('near', 25.0)
    lo, hi = camp['spread']
    out = []
    for _ in range(4000):
        if len(out) == count:
            break
        r = math.sqrt(rnd.uniform(lo * lo, hi * hi))
        a = rnd.uniform(0, 2 * math.pi)
        x, z = round(ax + r * math.cos(a), 1), round(az + r * math.sin(a), 1)
        if 'box' in camp:
            x0, z0, x1, z1 = camp['box']
            if not (x0 + 20 <= x <= x1 - 20 and z0 + 20 <= z <= z1 - 20):
                continue
        h = y0 + rnd.uniform(-12, 12) if camp.get('swim') else flat(g, x, z, y0, tol)
        if h is None:
            continue
        if camp.get('wet', False) is not None and env is not None and env.flags(x, h + 0.5, z) & 7:
            continue
        if any(math.hypot(x - p[0], z - p[2]) < 18 for p in taken + out):
            continue
        if any(math.hypot(x - o[0], z - o[1]) < 12 and abs(h - o[2]) < 15 for o in others):
            continue
        out.append((x, round(h + 0.1, 2), z))
    return out


def binds(db, kin, name, k):
    out = []
    for t, c in BIND_TABLES:
        src = rows(db, f'SELECT * FROM {t} WHERE lower({c}) = lower(?)', (kin,))
        if t == 'npcclass' and 'cls' in k:
            src = [{'name': name, 'class': k['cls'], 'source': SRC}]
        if t == 'lootcreature' and not src and k.get('region'):
            src = [{'npc_name': name, 'region': k['region'], 'profile': '', 'source': SRC}]
        for r in src[:1]:
            r = dict(r)
            r[c] = name if t == 'npcclass' else name.lower() if t != 'lootcreature' else name.lower()
            if t == 'npctemplate':
                r['note'] = 'faq camps, level %d-%d' % k['lv']
            if t == 'lootcreature':
                if k.get('region'):
                    r['region'] = k['region']
                r['profile'] = k.get('profile', r.get('profile', ''))
            r['source'] = SRC
            out.append((t, {c: r[c]}, r))
    return out


def shard(sh):
    db = sqlite3.connect(f'file:{W}/{sh}0.db?mode=ro', uri=True)
    rnd = random.Random(20261114)
    grounds, envs = {}, {}
    ops, summary = [], collections.defaultdict(list)
    nid = FIRST_ID
    cols = [r[1] for r in db.execute('PRAGMA table_info(npcs)')]
    allnpcs = collections.defaultdict(list)
    for w, x, y, z in db.execute('SELECT world, x_coord, y_coord, z_coord FROM npcs'):
        allnpcs[w].append((x, z, y))
    census = {}
    for (old,) in db.execute("SELECT old FROM sandstorm_census_seed_w2 WHERE tbl = 'npcs removed'") if db.execute(
            "SELECT 1 FROM sqlite_master WHERE name = 'sandstorm_census_seed_w2'").fetchone() else []:
        r = json.loads(old)
        census.setdefault(r['npc_name'], []).append(r)
    for camp in CAMPS:
        w = camp['world']
        if w not in grounds:
            grounds[w] = sw.Ground(f'{GROUND}/{w}.ground')
            envs[w] = sp.Env(f'{GROUND}/{w}.env')
        taken = []
        for rn in camp.get('restore', []):
            olds = [r for r in census.get(rn, []) if r['world'] == w]
            if rn == 'elder spirit':
                olds = [r for r in olds if abs(r['x_coord'] - 7060) < 300 and abs(r['z_coord'] - 3360) < 300]
            for o in olds:
                if db.execute('SELECT 1 FROM npcs WHERE npc_name = ? AND abs(x_coord - ?) < 5 AND abs(z_coord - ?) < 5',
                              (o['npc_name'], o['x_coord'], o['z_coord'])).fetchone():
                    summary[camp['key']].append(f'{rn}: already stands at its old spot'); continue
                row = {c: o.get(c) for c in cols}
                row['id'] = nid; row['npc_id'] = nid
                if camp['key'] == 'gargoyles':
                    row['npc_level'] = o['npc_level'] - 5  # the camp's band moved down five (LEVELS)
                ops.append(['npcs', {'id': nid}, None, row])
                ops.append(['npcrespawn', {'npc_row': nid}, None, {'npc_row': nid, 'seconds': RESPAWN, 'note': 'faq camps: as the camp\'s', 'source': SRC}])
                summary[camp['key']].append(f"{rn} #{nid} L{row['npc_level']} restored at ({o['x_coord']:.0f}, {o['z_coord']:.0f}) "
                                            f"(the row the Odus census removed, {o['id']})")
                taken.append((o['x_coord'], o['y_coord'], o['z_coord']))
                nid += 1
        for k in camp['kinds']:
            model = int(k['model'], 16)
            base = rows(db, 'SELECT * FROM npcs WHERE lower(npc_name) = lower(?) AND (modelid & 4294967295) = ? ORDER BY id LIMIT 1',
                        (k['base'], model))
            if not base:
                base = rows(db, 'SELECT * FROM npcs WHERE (modelid & 4294967295) = ? AND npc_type = 0 ORDER BY id LIMIT 1', (model,))
            if not base:
                raise SystemExit(f'{sh}: no base row for {k["name"]} ({k["base"]}, {k["model"]})')
            base = base[0]
            spots = place(grounds[w], envs[w], camp, k['n'], taken, allnpcs[w], rnd)
            if len(spots) < k['n']:
                raise SystemExit(f'{sh} {camp["key"]} {k["name"]}: {len(spots)} spots of {k["n"]}')
            taken += spots
            lv0, lv1 = k['lv']
            for i, (x, y, z) in enumerate(spots):
                row = dict(base)
                row.update(id=nid, npc_id=nid, npc_name=k['name'], x_coord=x, y_coord=y, z_coord=z, world=w,
                           facing=rnd.randrange(256), npc_level=lv0 + (i * (lv1 - lv0 + 1)) // k['n'], dt_kill=None)
                if 'size' in k:
                    row['size'] = k['size']
                ops.append(['npcs', {'id': nid}, None, row])
                ops.append(['npcrespawn', {'npc_row': nid}, None, {'npc_row': nid, 'seconds': RESPAWN, 'note': 'faq camps: as the camp\'s', 'source': SRC}])
                nid += 1
            summary[camp['key']].append(f"{k['name']} x{k['n']} L{lv0}-{lv1}, body {k['model']} (base row {base['id']} {base['npc_name']}), "
                                        + ', '.join(f'({x:.0f},{y:.0f},{z:.0f})' for x, y, z in spots))
            have = db.execute('SELECT 1 FROM npctemplate WHERE lower(name) = lower(?)', (k['name'],)).fetchone()
            if k.get('bind') and not have:
                for t, key, r in binds(db, k['bind'], k['name'], k):
                    if not rows(db, f'SELECT * FROM {t} WHERE {list(key)[0]} = ?', (list(key.values())[0],)):
                        ops.append([t, key, None, r])
            if camp.get('social'):
                key = {'npc_name': k['name']}
                if not rows(db, 'SELECT * FROM npcsocial WHERE npc_name = ?', (k['name'],)):
                    ops.append(['npcsocial', key, None, {'npc_name': k['name'], 'social': 1, 'faction': camp['social'], 'radius': 0.0,
                                                         'note': 'faq camps: aggro-linked (FAQ)', 'source': SRC}])
    # the wrong-level camps
    for key, title, w, box, names, old, new in LEVELS:
        q = 'SELECT id, npc_name, npc_level, x_coord, z_coord FROM npcs WHERE world = ? AND npc_name IN (%s)' % ','.join('?' * len(names))
        rs = rows(db, q, [w] + names)
        if box:
            rs = [r for r in rs if box[0] <= r['x_coord'] <= box[2] and box[1] <= r['z_coord'] <= box[3]]
        if key == 'hobgoblins':
            rs = [r for r in rs if 6500 <= r['x_coord'] <= 7500 and 2800 <= r['z_coord'] <= 3700]
        if key == 'brokenskull':
            rs = [r for r in rs if r['npc_level'] <= 5]
        byname = collections.defaultdict(list)
        for r in sorted(rs, key=lambda r: r['id']):
            byname[r['npc_name']].append(r)
        n = 0
        for name, rr in byname.items():
            for i, r in enumerate(rr):
                if old is None:
                    b0, b1 = BAND_BY_NAME[name]
                    lv = b0 + (i * (b1 - b0 + 1)) // len(rr)
                else:
                    lv = round(new[0] + (r['npc_level'] - old[0]) * (new[1] - new[0]) / (old[1] - old[0]))
                    lv = max(new[0], min(new[1], lv))
                if lv != r['npc_level']:
                    ops.append(['npcs', {'id': r['id']}, {'npc_level': r['npc_level']}, {'npc_level': lv}])
                    n += 1
            ls = sorted({r['npc_level'] for r in rr})
            summary['level ' + key].append(f'{name} x{len(rr)} L{ls[0]}-{ls[-1]}')
        summary['level ' + key].append(f'{n} rows re-levelled')
    return ops, summary


def main():
    plan = {'about': 'tools/faq_camps_20261114.py: per shard, [table, key, before, after]', 'shards': {}, 'summary': {}}
    for sh in 'hl':
        ops, summary = shard(sh)
        plan['shards'][sh] = ops
        plan['summary'][sh] = summary
        c = collections.Counter((o[0], 'new' if o[2] is None else 'update') for o in ops)
        print(sh, len(ops), dict(c))
    json.dump(plan, open(OUT, 'w'), indent=0, ensure_ascii=False)
    open(OUT, 'a').write('\n')
    for k, v in plan['summary']['h'].items():
        print('##', k)
        for line in v:
            print('  ', line)


if __name__ == '__main__':
    main()
