#!/usr/bin/env python3
"""The Plane of Sky's isles, by name, for every place a location is shown (the owner, 2026-11-14: "for the location of
plane of sky islands, for example dust is on night 1 lets make that the Location like Plane of Sky (Night Chain 1)").

World 4's terrain (the ground file the server bakes from the operator's disc, worlds/ground/4.ground: no game data is
kept here, only what this writes) is cut into its pieces (triangles sharing corners), and each piece goes to an isle:
the piece under one of ANCHORS (each chain's essences as handlers/skypass.go has them, the bosses and landmarks the
forum's 2010 walkthroughs name for each isle: eqoa.forumotion.com t207-t214), else, attached to the nearest named
piece within ATTACH units (an isle's underside, its rocks and ledges), else none. An isle's footprint is its pieces'
cells (CELL units) seen from above, closed by DILATE cells and filled, traced as polygons; its height range is its
standing ground's. A spot names the isle whose polygon holds it (two that overlap from above, snow 1's cliff over snow
2's igloos: the one whose ground is nearest the spot's height), else the nearest isle within NEAR units, else none.

Writes the server's table (internal/skyisles/isles_gen.go), vanilla's (--rust, crates/eqoa/src/skyisles_gen.rs) and a
picture (--png). Refuses, nothing written, when an anchor finds no piece, a piece is two isles', or an isle is left
with no footprint.

    tools/sky_isles.py --ground worlds/ground/4.ground [--go internal/skyisles/isles_gen.go] [--rust ...] [--png ...]
"""
import argparse, struct, sys
import numpy as np
from scipy import ndimage, sparse
from scipy.sparse.csgraph import connected_components
from scipy.spatial import cKDTree

CELL = 4.0      # the footprint's grid, world units
DILATE = 2      # cells an isle's footprint is closed by (its gaps between pieces)
ATTACH = 60.0   # how near a named piece an unnamed one must come to be its isle's
SIMPLIFY = 4.0  # the polygons' Douglas-Peucker tolerance, world units
MINAREA = 400.0 # a polygon smaller than this (square units) is left out

# (isle, x, y, z, why): the essences are skypass.go's (an isle's essence takes its own key: Rain Key 1's stands on the
# first isle of rain), the rest the walkthroughs' creatures at their npcs rows.
ANCHORS = [
    ("Main Isle", 4482.5, 244.7, 5293.2, "essence of rain (rain key)"),
    ("Main Isle", 4621.4, 270.8, 5729.6, "essence of night (key of the ancients)"),
    ("Main Isle", 5137.3, 261.3, 5188.3, "essence of sun (sun key)"),
    ("Main Isle", 4685.0, 250.0, 5398.0, "Liztin Misseliania"),
    ("Main Isle", 4824.0, 250.0, 5497.0, "the Island of Wonder portal"),
    ("Isle of Wonder", 5000.09, 333.54, 4998.23, "the chains' last landing (skyIsleOfWonder)"),
    ("Isle of Wonder", 4980.0, 333.0, 4985.0, "Mark of the Majestic"),
    ("Isle of the Constructs", 5105.0, 237.0, 5745.0, "the constructs (the Astral Light's isle)"),
    ("Isle of the Constructs", 5103.9, 236.0, 5778.9, "its Astral Light"),
    ("Rain Chain 1", 4222.7, 233.0, 5714.6, "essence of rain (Rain Key 1)"),
    ("Rain Chain 1", 4234.0, 236.0, 5688.0, "Artiokanis"),
    ("Rain Chain 2", 4158.6, 180.9, 5122.5, "essence of rain (Rain Key 2)"),
    ("Rain Chain 2", 4169.0, 181.0, 5214.0, "Trixie Longwings"),
    ("Rain Chain 3", 4638.6, 119.7, 4655.2, "essence of rain (Rain Key 3)"),
    ("Rain Chain 3", 4318.0, -69.0, 4700.0, "Granavik"),
    ("Rain Chain 3", 4611.0, 113.0, 4939.0, "Singe Burnfeathers"),
    ("Rain Chain 4", 4199.7, 80.9, 4148.3, "essence of rain (Rain Key 4)"),
    ("Rain Chain 4", 4232.0, 86.0, 4215.0, "Necrologist"),
    ("Rain Chain 5", 4516.5, 30.9, 4271.6, "essence of rain (Rain Key 5)"),
    ("Rain Chain 5", 4556.0, 31.0, 4276.0, "Chimareon the Tyrant"),
    ("Watrya's Isle", 5640.0, 278.0, 5120.0, "Madam Watrya"),
    ("Watrya's Isle", 5656.6, 280.9, 5107.4, "essence of day (no passage)"),
    ("Day Chain 1", 5597.3, 235.9, 5343.1, "essence of day (Day Key 1)"),
    ("Day Chain 1", 5640.0, 257.0, 5427.0, "brightstar princess"),
    ("Day Chain 2", 5380.8, 282.6, 4755.9, "essence of day (Day Key 2)"),
    ("Day Chain 2", 5406.0, 282.0, 4751.0, "Imerzan"),
    ("Day Chain 3", 5459.6, 381.0, 4237.5, "essence of day (Day Key 3)"),
    ("Day Chain 3", 5538.0, 391.0, 4186.0, "torrential wind"),
    ("Day Chain 4", 5054.4, 431.8, 4243.1, "essence of day (Day Key 4)"),
    ("Day Chain 4", 5073.0, 431.0, 4244.0, "Randarin the Relentless"),
    ("Night Chain 1", 4160.7, 613.3, 6404.1, "essence of night (Night Key 1)"),
    ("Night Chain 1", 4175.0, 612.0, 6216.0, "Ancient Jester"),
    ("Night Chain 2", 4441.2, 449.6, 6436.7, "essence of night (Night Key 2)"),
    ("Night Chain 2", 4442.0, 449.0, 6400.0, "iron knight"),
    ("Night Chain 3", 4250.0, 671.5, 6806.4, "essence of night (Night Key 3)"),
    ("Night Chain 3", 4250.0, 661.0, 6743.0, "Night Maiden"),
    ("Night Chain 3", 4390.0, 662.0, 6608.0, "Arch Bishop Lancel"),
    ("Night Chain 4", 4322.6, 354.0, 7754.4, "essence of night (Night Key 4)"),
    ("Night Chain 4", 4155.0, 353.0, 7758.0, "dark baroness"),
    ("Night Chain 5", 4400.0, 130.6, 7135.8, "essence of night (Night Key 5)"),
    ("Night Chain 5", 4499.0, 99.0, 7234.0, "King Ruthra"),
    ("Night Chain 5", 4160.0, 208.25, 6980.0, "the port-in ledge (Night Key 4's landing)"),
    ("Snow Chain 1", 5668.0, 553.0, 6356.0, "Ryini"),
    ("Snow Chain 1", 5674.0, 488.0, 6554.0, "Wind Mephit"),
    ("Snow Chain 2", 5766.8, 238.3, 6702.1, "essence of snow (Snow Key 2)"),
    ("Snow Chain 2", 5550.0, 238.0, 6688.0, "Tamis"),
    ("Snow Chain 2", 5723.0, 237.0, 6928.0, "Whirlwind"),
    ("Snow Chain 3", 5906.3, 177.5, 7570.5, "essence of snow (Snow Key 3)"),
    ("Snow Chain 3", 5884.0, 177.0, 7560.0, "glacial champion"),
    ("Snow Chain 4", 5525.1, 96.1, 7708.0, "essence of snow (Snow Key 4)"),
    ("Snow Chain 4", 5509.0, 107.0, 7597.0, "Enraged Aeyinar"),
    ("Desolation Chain 1", 4751.5, 568.9, 6418.5, "essence of desolation (Desolation Key 1)"),
    ("Desolation Chain 1", 4760.0, 569.0, 6249.0, "Erych Arbiter"),
    ("Desolation Chain 2", 5146.8, 616.6, 6620.1, "essence of desolation (Desolation Key 2)"),
    ("Desolation Chain 2", 4933.0, 616.0, 6704.0, "aviak elder"),
    ("Desolation Chain 3", 5116.0, 713.5, 7174.8, "essence of desolation (Desolation Key 3)"),
    ("Desolation Chain 3", 5171.0, 713.0, 7162.0, "elite harpy queen"),
    ("Desolation Chain 4", 4734.2, 884.5, 7688.3, "essence of desolation (Desolation Key 4)"),
    ("Desolation Chain 4", 4728.0, 848.0, 7617.0, "Quintessence of Veeshan"),
]


def load(path):
    """The ground file's triangles (n, 3, 3), x y z, degenerate ones (a strip's turns) left out."""
    b = open(path, "rb").read()
    if b[:4] != b"EQGR":
        sys.exit(f"{path}: not a ground file")
    _, n = struct.unpack_from("<II", b, 4)
    tris = []
    for i in range(n):
        o = 12 + 44 * i
        f = struct.unpack_from("<7f", b, o)
        off, = struct.unpack_from("<q", b, o + 28)
        strips, _ = struct.unpack_from("<II", b, o + 36)
        p = off
        for _ in range(strips):
            c, = struct.unpack_from("<H", b, p)
            p += 2
            a = np.frombuffer(b, dtype="<i2", count=3 * c, offset=p).reshape(c, 3).astype(np.float64)
            p += 6 * c
            v = np.stack([(a[:, 0] + 32768) / 4 + f[4], a[:, 1] / 8 + f[5], (a[:, 2] + 32768) / 4 + f[6]], 1)
            if c >= 3:
                tr = np.stack([v[:-2], v[1:-1], v[2:]], 1)
                tr[1::2, [0, 1]] = tr[1::2, [1, 0]]  # every other one turned, as the game's collision turns it (stripTri)
                tris.append(tr)
    t = np.concatenate(tris)
    n = np.cross(t[:, 1] - t[:, 0], t[:, 2] - t[:, 0])
    return t[(n * n).sum(1) > 1e-8]


def pieces(t):
    """Each triangle's piece: triangles sharing a corner (to an eighth of a unit) are one."""
    keys, inv = np.unique(np.round(t.reshape(-1, 3) * 8).astype(np.int64), axis=0, return_inverse=True)
    inv = inv.reshape(-1, 3)
    rows = np.concatenate([inv[:, 0], inv[:, 0]])
    cols = np.concatenate([inv[:, 1], inv[:, 2]])
    g = sparse.coo_matrix((np.ones(len(rows)), (rows, cols)), shape=(len(keys), len(keys)))
    _, lab = connected_components(g, directed=False)
    return lab[inv[:, 0]]


def under(t, x, z):
    """The triangles holding (x, z) seen from above, and their heights there."""
    ax, az, bx, bz, cx, cz = t[:, 0, 0], t[:, 0, 2], t[:, 1, 0], t[:, 1, 2], t[:, 2, 0], t[:, 2, 2]
    d = (bz - cz) * (ax - cx) + (cx - bx) * (az - cz)
    with np.errstate(divide="ignore", invalid="ignore"):
        w0 = ((bz - cz) * (x - cx) + (cx - bx) * (z - cz)) / d
        w1 = ((cz - az) * (x - cx) + (ax - cx) * (z - cz)) / d
        w2 = 1 - w0 - w1
        ok = (d != 0) & (w0 >= -1e-4) & (w1 >= -1e-4) & (w2 >= -1e-4)
        h = w0 * t[:, 0, 1] + w1 * t[:, 1, 1] + w2 * t[:, 2, 1]
    return np.nonzero(ok)[0], h[ok]


def floors(t):
    """Whether each triangle is standing ground: within 60 degrees of level (ground.go's floor) and facing up (turned as
    stripTri turns it, the terrain's top faces -y in this cross product: the Isle of Wonder's, Granavik's cave floor),
    not an isle's underside."""
    n = np.cross(t[:, 1] - t[:, 0], t[:, 2] - t[:, 0])
    return (n[:, 1] < 0) & (n[:, 1] ** 2 > 0.25 * (n * n).sum(1))


def name_pieces(t, lab):
    """Each piece's isle (an index into the isles' names, -1 none), and the isles' names."""
    names = list(dict.fromkeys(a[0] for a in ANCHORS))
    isle = np.full(lab.max() + 1, -1)
    tree = cKDTree(t.reshape(-1, 3))
    for name, x, y, z, why in ANCHORS:
        idx, h = under(t, x, z)
        near = np.abs(h - y)
        if len(idx) and near.min() <= 40:
            p = lab[idx[near.argmin()]]
        else:  # standing on something placed: the nearest terrain
            d, k = tree.query([x, y, z])
            if d > 60:
                sys.exit(f"anchor {why} ({name}) at ({x}, {y}, {z}): no ground within 60")
            p = lab[k // 3]
        if isle[p] not in (-1, names.index(name)):
            sys.exit(f"anchor {why} ({name}): its piece is {names[isle[p]]}'s already")
        isle[p] = names.index(name)
    # the rest attach to the nearest named piece, a round at a time (a piece reached through another the round after)
    verts = t.reshape(-1, 3)
    vlab = np.repeat(lab, 3)
    while True:
        named = isle[vlab] >= 0
        if named.all():
            break
        nv = np.nonzero(named)[0]
        d, k = cKDTree(verts[nv]).query(verts[~named], distance_upper_bound=ATTACH)
        ok = np.isfinite(d)
        if not ok.any():
            break
        best = {}
        for p, dd, kk in zip(vlab[~named][ok], d[ok], k[ok]):
            if p not in best or dd < best[p][0]:
                best[p] = (dd, isle[vlab[nv[kk]]])
        for p, (dd, i) in best.items():
            isle[p] = i
    return isle, names


def footprints(t, lab, isle, names):
    """Each isle's footprint: (mask, x0, z0) on the CELL grid, and its standing ground's height range."""
    x0, z0 = np.floor(t[:, :, 0].min() / CELL) * CELL - 8 * CELL, np.floor(t[:, :, 2].min() / CELL) * CELL - 8 * CELL
    W = int((t[:, :, 0].max() - x0) / CELL) + 9
    H = int((t[:, :, 2].max() - z0) / CELL) + 9
    fl = floors(t)
    out = []
    for i, name in enumerate(names):
        sel = isle[lab] == i
        mask = np.zeros((H, W), bool)
        for tr in t[sel]:
            raster(mask, tr, x0, z0)
        mask = ndimage.binary_closing(mask, np.ones((3, 3)), iterations=DILATE)
        mask = ndimage.binary_fill_holes(ndimage.binary_dilation(mask, np.ones((3, 3)), iterations=1))
        lo, hi = np.full((H, W), np.inf), np.full((H, W), -np.inf)
        for tr in t[sel & fl]:
            raster(np.zeros((H, W), bool), tr, x0, z0, lo, hi)
        ys = t[sel & fl][:, :, 1]
        out.append((mask, float(ys.min()), float(ys.max()), lo, hi))
    return out, x0, z0


SPLIT = 16  # an overlap's cells, world units (a multiple of CELL)


def splits(fp, names, x0, z0):
    """Where two isles' footprints overlap from above (snow 1's cliff over snow 2's igloos): for each SPLIT cell of
    it, (x, z, the height dividing them, the upper isle, the lower): above the lower's highest ground there and below
    the upper's lowest, half way; a cell where only one has ground divides 30 under the upper's or over the lower's."""
    out = []
    k = int(SPLIT / CELL)
    for a in range(len(fp)):
        for b in range(a + 1, len(fp)):
            ov = fp[a][0] & fp[b][0]
            if not ov.any():
                continue
            up, dn = (a, b) if (fp[a][1] + fp[a][2]) > (fp[b][1] + fp[b][2]) else (b, a)
            H, W = ov.shape
            for j in range(0, H, k):
                for i in range(0, W, k):
                    if not ov[j:j + k, i:i + k].any():
                        continue
                    ulo = fp[up][3][j:j + k, i:i + k].min()
                    dhi = fp[dn][4][j:j + k, i:i + k].max()
                    if np.isfinite(ulo) and np.isfinite(dhi):
                        y = (ulo + dhi) / 2
                    elif np.isfinite(ulo):
                        y = ulo - 30
                    elif np.isfinite(dhi):
                        y = dhi + 30
                    else:
                        y = (fp[up][1] + fp[dn][2]) / 2
                    out.append((x0 + i * CELL, z0 + j * CELL, round(float(y), 1), names[up], names[dn]))
    return out


def raster(mask, tr, x0, z0, lo=None, hi=None):
    """Marks the cells tr covers; with lo and hi, keeps each cell's lowest and highest height of it."""
    xs, zs = (tr[:, 0] - x0) / CELL, (tr[:, 2] - z0) / CELL
    i0, i1, j0, j1 = int(xs.min()), int(xs.max()), int(zs.min()), int(zs.max())
    ii, jj = np.meshgrid(np.arange(i0, i1 + 1), np.arange(j0, j1 + 1))
    px, pz = ii + 0.5, jj + 0.5
    ax, az, bx, bz, cx, cz = xs[0], zs[0], xs[1], zs[1], xs[2], zs[2]
    d = (bz - cz) * (ax - cx) + (cx - bx) * (az - cz)
    if d != 0:
        w0 = ((bz - cz) * (px - cx) + (cx - bx) * (pz - cz)) / d
        w1 = ((cz - az) * (px - cx) + (ax - cx) * (pz - cz)) / d
        m = (w0 >= -1e-4) & (w1 >= -1e-4) & (1 - w0 - w1 >= -1e-4)
        mask[jj[m], ii[m]] = True
        if lo is not None:
            y = w0 * tr[0, 1] + w1 * tr[1, 1] + (1 - w0 - w1) * tr[2, 1]
            np.minimum.at(lo, (jj[m], ii[m]), y[m])
            np.maximum.at(hi, (jj[m], ii[m]), y[m])
    for x, y, z in zip(xs, tr[:, 1], zs):
        mask[int(z), int(x)] = True
        if lo is not None:
            lo[int(z), int(x)] = min(lo[int(z), int(x)], y)
            hi[int(z), int(x)] = max(hi[int(z), int(x)], y)


def trace(mask):
    """The outlines of a filled mask's parts, each a list of grid corners (x, z), ccw seen with +z down."""
    H, W = mask.shape
    m = np.zeros((H + 2, W + 2), bool)
    m[1:-1, 1:-1] = mask
    # directed boundary edges, the inside on the left walking along: corner (x, z) -> corner
    nxt = {}
    for j, i in zip(*np.nonzero(m)):
        if not m[j - 1, i]:
            nxt.setdefault((i + 1, j), []).append((i, j))      # top edge, walked right to left
        if not m[j + 1, i]:
            nxt.setdefault((i, j + 1), []).append((i + 1, j + 1))
        if not m[j, i - 1]:
            nxt.setdefault((i, j), []).append((i, j + 1))
        if not m[j, i + 1]:
            nxt.setdefault((i + 1, j + 1), []).append((i + 1, j))
    loops = []
    while nxt:
        start = next(iter(nxt))
        loop, cur = [start], start
        while True:
            outs = nxt[cur]
            n = outs.pop()
            if not outs:
                del nxt[cur]
            if n == start:
                break
            loop.append(n)
            cur = n
        loops.append([(x - 1, z - 1) for x, z in loop])
    return loops


def simplify(pts, tol):
    """Douglas-Peucker on a closed loop."""
    pts = np.array(pts, float)
    def dp(a):
        if len(a) < 3:
            return a
        s, e = a[0], a[-1]
        v = e - s
        L = np.hypot(*v)
        d = np.abs(v[0] * (a[:, 1] - s[1]) - v[1] * (a[:, 0] - s[0])) / L if L else np.hypot(*(a - s).T)
        k = d.argmax()
        if d[k] > tol:
            return np.vstack([dp(a[:k + 1])[:-1], dp(a[k:])])
        return np.vstack([s, e])
    far = np.hypot(*(pts - pts[0]).T).argmax()
    a = dp(pts[:far + 1])
    b = dp(np.vstack([pts[far:], pts[:1]]))
    return np.vstack([a[:-1], b[:-1]])


def area(poly):
    x, z = poly[:, 0], poly[:, 1]
    return 0.5 * abs(np.dot(x, np.roll(z, -1)) - np.dot(z, np.roll(x, -1)))


def polygons(fp, x0, z0):
    out = []
    for mask, lo, hi, _, _ in fp:
        polys = []
        for loop in trace(mask):
            p = simplify(loop, SIMPLIFY / CELL) * CELL + [x0, z0]
            if area(p) >= MINAREA:
                polys.append(np.round(p, 1))
        out.append((polys, lo, hi))
    return out


def inside(poly, x, z):
    c = False
    n = len(poly)
    for i in range(n):
        (ax, az), (bx, bz) = poly[i], poly[i - 1]
        if (az > z) != (bz > z) and x < (bx - ax) * (z - az) / (bz - az) + ax:
            c = not c
    return c


def edge_dist(poly, x, z):
    a, b = poly, np.roll(poly, 1, 0)
    ab = b - a
    t = np.clip(((x - a[:, 0]) * ab[:, 0] + (z - a[:, 1]) * ab[:, 1]) / np.maximum((ab * ab).sum(1), 1e-9), 0, 1)
    return np.hypot(a[:, 0] + t * ab[:, 0] - x, a[:, 1] + t * ab[:, 1] - z).min()


NEAR = 100.0  # how far off an isle's footprint a spot still names it (a fall, a jump, a bridge)


def lookup(isles, cuts, x, y, z):
    """The isle at (x, y, z), as the Go and Rust tables answer it."""
    for cx, cz, cy, up, dn in cuts:
        if cx <= x < cx + SPLIT and cz <= z < cz + SPLIT:
            return up if y >= cy else dn
    hits = [i for i, (_, polys, lo, hi) in enumerate(isles) if any(inside(p, x, z) for p in polys)]
    if hits:
        return isles[min(hits, key=lambda i: max(isles[i][2] - y, y - isles[i][3], 0))][0]
    best, bd = None, NEAR
    for name, polys, lo, hi in isles:
        for p in polys:
            d = edge_dist(p, x, z)
            if d < bd:
                best, bd = name, d
    return best


def write_go(path, isles, cuts):
    with open(path, "w") as f:
        f.write("// Code generated by tools/sky_isles.py from world 4's ground file; DO NOT EDIT.\n\npackage skyisles\n\n")
        f.write("var isles = []isle{\n")
        for name, polys, lo, hi in isles:
            f.write(f'\t{{name: "{name}", lo: {lo:.1f}, hi: {hi:.1f}, polys: [][]float32{{\n')
            for p in polys:
                f.write("\t\t{" + ", ".join(f"{v:.1f}" for v in p.ravel()) + "},\n")
            f.write("\t}},\n")
        f.write("}\n\n")
        f.write(f"// splitSide is an overlap cell's side, world units.\nconst splitSide = {SPLIT:g}\n\n")
        f.write("var splits = []split{\n")
        for x, z, y, up, dn in cuts:
            f.write(f'\t{{{x:.1f}, {z:.1f}, {y:.1f}, "{up}", "{dn}"}},\n')
        f.write("}\n")


def write_rust(path, isles, cuts):
    with open(path, "w") as f:
        f.write("//! Generated by go-eqoa-server's tools/sky_isles.py from world 4's ground file (names and outlines, no\n")
        f.write("//! game data): do not edit. `ISLES`: each isle's name, its standing ground's height range and its\n")
        f.write("//! outlines (x, z pairs). `SPLITS`: where two overlap from above, each `SPLIT`-unit cell's corner\n")
        f.write("//! (x, z), the height dividing them, the isle above and the one below.\n\n")
        f.write("#[rustfmt::skip]\npub(crate) const ISLES: &[(&str, f32, f32, &[&[f32]])] = &[\n")
        for name, polys, lo, hi in isles:
            f.write(f'    ("{name}", {lo:.1f}, {hi:.1f}, &[\n')
            for p in polys:
                f.write("        &[" + ", ".join(f"{v:.1f}" for v in p.ravel()) + "],\n")
            f.write("    ]),\n")
        f.write("];\n\n")
        f.write(f"pub(crate) const SPLIT: f32 = {SPLIT:.1f};\n\n")
        f.write("#[rustfmt::skip]\npub(crate) const SPLITS: &[(f32, f32, f32, &str, &str)] = &[\n")
        for x, z, y, up, dn in cuts:
            f.write(f'    ({x:.1f}, {z:.1f}, {y:.1f}, "{up}", "{dn}"),\n')
        f.write("];\n")


def picture(path, isles, t, lab, isle):
    import matplotlib
    matplotlib.use("Agg")
    import matplotlib.pyplot as plt
    fig, ax = plt.subplots(figsize=(10, 18), dpi=110)
    cent = t.mean(1)
    ax.scatter(cent[:, 0], cent[:, 2], s=0.05, c=np.where(isle[lab] >= 0, 0.6, 0.0), cmap="Greys", vmin=0, vmax=1)
    cmap = plt.get_cmap("tab20")
    for k, (name, polys, lo, hi) in enumerate(isles):
        for p in polys:
            q = np.vstack([p, p[:1]])
            ax.plot(q[:, 0], q[:, 1], color=cmap(k % 20), lw=1.2)
            ax.fill(q[:, 0], q[:, 1], color=cmap(k % 20), alpha=0.18)
        big = max(polys, key=area)
        ax.text(big[:, 0].mean(), big[:, 1].mean(), f"{name}\ny {lo:.0f}..{hi:.0f}", ha="center", va="center", fontsize=7, weight="bold")
    for name, x, y, z, why in ANCHORS:
        ax.plot(x, z, "k.", ms=3)
    ax.set_aspect("equal")
    ax.invert_yaxis()  # +z is south: north up
    ax.set_xlabel("x (east)")
    ax.set_ylabel("z (south)")
    ax.set_title("Plane of Sky (world 4): isles' footprints, north up")
    fig.tight_layout()
    fig.savefig(path)


def build(ground):
    t = load(ground)
    lab = pieces(t)
    isle, names = name_pieces(t, lab)
    fp, x0, z0 = footprints(t, lab, isle, names)
    isles = []
    for name, (polys, lo, hi) in zip(names, polygons(fp, x0, z0)):
        if not polys:
            sys.exit(f"{name}: no footprint")
        isles.append((name, polys, lo, hi))
    return isles, splits(fp, names, x0, z0), t, lab, isle


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--ground", required=True)
    ap.add_argument("--go")
    ap.add_argument("--rust")
    ap.add_argument("--png")
    a = ap.parse_args()
    isles, cuts, t, lab, isle = build(a.ground)
    for name, x, y, z, why in ANCHORS:
        got = lookup(isles, cuts, x, y, z)
        if got != name:
            sys.exit(f"anchor {why} at ({x}, {y}, {z}) reads {got}, not {name}")
    unnamed = (isle[lab] < 0).sum()
    print(f"{len(isles)} isles, {sum(len(p) for _, p, _, _ in isles)} outlines, {sum(sum(len(q) for q in p) for _, p, _, _ in isles)} corners; {unnamed} of {len(t)} triangles on no isle")
    for name, polys, lo, hi in isles:
        print(f"  {name}: {len(polys)} outline(s), standing ground y {lo:.0f}..{hi:.0f}")
    for pair in sorted({(c[3], c[4]) for c in cuts}):
        print(f"  {pair[0]} over {pair[1]}: {sum(1 for c in cuts if (c[3], c[4]) == pair)} cells divided by height")
    if a.go:
        write_go(a.go, isles, cuts)
    if a.rust:
        write_rust(a.rust, isles, cuts)
    if a.png:
        picture(a.png, isles, t, lab, isle)


if __name__ == "__main__":
    main()
