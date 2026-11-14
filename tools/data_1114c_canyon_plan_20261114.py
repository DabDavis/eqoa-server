#!/usr/bin/env python3
"""Box Canyon's plan for tools/data_1114c_20261114.py (data_1114c_20261114.json's moves, as plan.json): from
cmd/canyonspread's survey of each shard (go run ./cmd/canyonspread -db <shard db> -ground <ground dir> -out DIR):

    python3 tools/data_1114c_canyon_plan_20261114.py SURVEY_HEIRLOOM SURVEY_LIVE plan.json check.json

check.json is for go run ./cmd/canyonspread -ground <dir> -check check.json (each place stood on, joined to the floor).
The tool's JSON is plan.json's moves, one a line."""
import json, sys, numpy as np
from scipy.optimize import linear_sum_assignment
from scipy.ndimage import binary_erosion, distance_transform_edt

BOX = (20200, 20200, 21900, 21900)     # the canyon's mesa complex (zone 121, Box Canyons, is 20000-22000)
FLOOR_MAX = 25.0                        # the canyon floor: reachable samples under this (the floor ~5, the desert ~55)
EDGE = 20.0                             # a place this far inside the floor (off the cliffs' feet)
CAMP_CLEAR = 45.0                       # from any NPC that stays (camps, quest NPCs, named)
REACH_CLEAR = 40.0                      # from a quest's reach place
SPACING_MIN = 40.0
REACH = [(21502.0, 20928.0), (21615.0, 20631.0), (20957.0, 20935.0)]  # queststeps reach places in the canyon

def load(d):
    m = json.load(open(d + '/grid.json'))
    h = np.fromfile(d + '/grid.bin', dtype='<f4').reshape(m['nz'], m['nx'])
    r = np.fromfile(d + '/reach.bin', dtype='u1').reshape(m['nz'], m['nx'])
    return m, h, r, json.load(open(d + '/npcs.json'))

mh, h, r, nh = load(sys.argv[1])
ml, hl, rl, nl = load(sys.argv[2])
assert mh == ml and np.array_equal(np.nan_to_num(h), np.nan_to_num(hl)) and np.array_equal(r, rl)
m = mh; st = m['step']
X = m['x0'] + np.arange(m['nx']) * st; Z = m['z0'] + np.arange(m['nz']) * st
inbox = (X[None, :] >= BOX[0]) & (X[None, :] <= BOX[2]) & (Z[:, None] >= BOX[1]) & (Z[:, None] <= BOX[3])
floor = (r == 1) & (np.nan_to_num(h, nan=1e9) < FLOOR_MAX) & inbox

def ij(n):
    return int(round((n['z'] - m['z0']) / st)), int(round((n['x'] - m['x0']) / st))

def candidate(n):
    if not n['movement'].startswith('it wanders: wildlife') or n.get('protected'):
        return False
    if not (BOX[0] <= n['x'] <= BOX[2] and BOX[1] <= n['z'] <= BOX[3]) or abs(n['floor'] - n['y']) > 3:
        return False
    j, i = ij(n)
    return bool(floor[j, i] or not r[j, i])   # on the canyon floor, or on a mesa/cliff no one walks to

ch = {n['id']: n for n in nh if candidate(n)}
cl = {n['id']: n for n in nl if candidate(n)}
assert set(ch) == set(cl), (set(ch) ^ set(cl))
ids = sorted(ch)
stay = [n for n in nh + nl if n['id'] not in ch]
# eligible places
inner = distance_transform_edt(floor) * st > EDGE
ok = inner.copy()
JJ, II = np.nonzero(ok)
px, pz = X[II], Z[JJ]
keep = np.ones(len(px), bool)
for n in stay:
    keep &= (px - n['x']) ** 2 + (pz - n['z']) ** 2 >= CAMP_CLEAR ** 2
for x, z in REACH:
    keep &= (px - x) ** 2 + (pz - z) ** 2 >= REACH_CLEAR ** 2
px, pz, JJ, II = px[keep], pz[keep], JJ[keep], II[keep]
print('eligible samples', len(px), 'creatures', len(ids))
# farthest-point sampling, from the place farthest from the eligible set's middle (fixed: no randomness)
cx, cz = px.mean(), pz.mean()
first = int(np.argmax((px - cx) ** 2 + (pz - cz) ** 2))
chosen = [first]
d = (px - px[first]) ** 2 + (pz - pz[first]) ** 2
for _ in range(len(ids) - 1):
    k = int(np.argmax(d)); chosen.append(k)
    d = np.minimum(d, (px - px[k]) ** 2 + (pz - pz[k]) ** 2)
P = np.array([(px[k], pz[k], float(h[JJ[k], II[k]])) for k in chosen])
dd = np.sqrt(((P[:, None, :2] - P[None, :, :2]) ** 2).sum(-1)) + np.eye(len(P)) * 1e9
print('nearest neighbour: min %.1f median %.1f' % (dd.min(1).min(), np.median(dd.min(1))))
assert dd.min() >= SPACING_MIN
# each creature to a place, least walking in all (heirloom's spots; live's are within a few units)
C = np.array([[np.hypot(ch[i]['x'] - p[0], ch[i]['z'] - p[1]) for p in P] for i in ids])
ri, ci = linear_sum_assignment(C)
moves = []
for a, b in zip(ri, ci):
    i = ids[a]; x, z, y = P[b]
    frm = sorted({(round(n['x'], 3), round(n['y'], 3), round(n['z'], 3)) for n in (ch[i], cl[i])})
    moves.append({'id': i, 'name': ch[i]['name'], 'level': ch[i]['level'], 'from': [list(f) for f in frm],
                  'to': [round(float(x), 2), round(float(y), 3), round(float(z), 2)]})
moves.sort(key=lambda v: v['id'])
json.dump(moves, open(sys.argv[3], 'w'), indent=1)
json.dump([{'id': v['id'], 'x': v['to'][0], 'y': v['to'][1], 'z': v['to'][2]} for v in moves], open(sys.argv[4], 'w'))
import collections
print(collections.Counter(v['name'] for v in moves))
print('walk: median %.0f max %.0f' % (np.median(C[ri, ci]), C[ri, ci].max()))
