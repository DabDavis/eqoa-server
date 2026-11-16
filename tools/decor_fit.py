#!/usr/bin/env python3
"""Fit a string of decoration lights to a model (the owner, 2026-11-16: "we also need to make the christmas lights able
to be put on any model. in freeport we can decorate put lights on the fence/low wall near freeport coach, the well, etc.").

Reads the triangles of the actors round a place as vanilla's `eqoa actormesh <WORLD.ESF> <x> <z> <radius> [0xDictID]`
prints them (world coordinates, off the operator's own disc), and lays a path of bulbs on one actor (the nearest of the
sprite, or each), in world coordinates, for a lightstrings row of shape 'path' (npc blank: in the world):

  top      along its top edge, its long way (a wall, a fence, a bench): each step of `spacing` along its longest
           horizontal axis, the highest point there, `offset` over it
  outline  round it at a height band (a well's rim, a pedestal): each step round its middle, the outermost point
           between `ymin` and `ymax` over its foot, `offset` out from it and over its highest there; closed
  wrap     wound round it (a post, a tree): `turns` from `ymin` to `ymax` over its foot, at each height and angle the
           outermost point, `offset` out

    decor_fit.py mesh.json --shape top|outline|wrap [--sprite 0x...] [--near x z] [--spacing 0.9] [--offset 0.15]
                 [--ymin 0] [--ymax 99] [--turns 3] [--each]   ->  JSON {"paths": [[[x,y,z], ...], ...]} on stdout
"""
import argparse, json, math, sys


def cloud(tris, step=0.3):
    """Points over each triangle, no two more than about step apart along an edge."""
    out = []
    for a, b, c in tris:
        n = min(60, max(4, math.ceil(max(math.dist(a, b), math.dist(b, c), math.dist(c, a)) / step)))
        for i in range(n + 1):
            for j in range(n + 1 - i):
                u, v = i / n, j / n
                w = 1 - u - v
                out.append(tuple(a[k] * w + b[k] * u + c[k] * v for k in range(3)))
    return out


def fit_top(pts, spacing, offset):
    cx = sum(p[0] for p in pts) / len(pts)
    cz = sum(p[2] for p in pts) / len(pts)
    sxx = sum((p[0] - cx) ** 2 for p in pts)
    szz = sum((p[2] - cz) ** 2 for p in pts)
    sxz = sum((p[0] - cx) * (p[2] - cz) for p in pts)
    ang = 0.5 * math.atan2(2 * sxz, sxx - szz)
    ux, uz = math.cos(ang), math.sin(ang)
    ts = [(p[0] - cx) * ux + (p[2] - cz) * uz for p in pts]
    lo, hi = min(ts), max(ts)
    n = max(1, round((hi - lo) / spacing))
    out = []
    for i in range(n + 1):
        t = lo + (hi - lo) * i / n
        band = [p for p, tt in zip(pts, ts) if abs(tt - t) <= spacing * 0.5]
        if not band:
            continue
        ymax = max(p[1] for p in band)
        top = [p for p in band if p[1] >= ymax - 0.12]
        out.append([round(sum(p[0] for p in top) / len(top), 3), round(ymax + offset, 3), round(sum(p[2] for p in top) / len(top), 3)])
    return out


def fit_outline(pts, spacing, offset, ymin, ymax, foot):
    band = [p for p in pts if ymin <= p[1] - foot <= ymax]
    if not band:
        return []
    cx = sum(p[0] for p in band) / len(band)
    cz = sum(p[2] for p in band) / len(band)
    rmax = max(math.hypot(p[0] - cx, p[2] - cz) for p in band)
    n = max(6, round(2 * math.pi * rmax / spacing))
    out = []
    for i in range(n):
        th = 2 * math.pi * i / n
        sel = [p for p in band if abs((math.atan2(p[2] - cz, p[0] - cx) - th + math.pi) % (2 * math.pi) - math.pi) <= math.pi / n]
        if not sel:
            continue
        r = max(math.hypot(p[0] - cx, p[2] - cz) for p in sel)
        y = max(p[1] for p in sel if math.hypot(p[0] - cx, p[2] - cz) >= r - 0.25)
        out.append([round(cx + (r + offset) * math.cos(th), 3), round(y + offset, 3), round(cz + (r + offset) * math.sin(th), 3)])
    if out:
        out.append(list(out[0]))  # closed
    return out


def fit_wrap(pts, spacing, offset, ymin, ymax, turns, foot):
    band = [p for p in pts if ymin <= p[1] - foot <= ymax]
    if not band:
        return []
    cx = sum(p[0] for p in band) / len(band)
    cz = sum(p[2] for p in band) / len(band)
    y0, y1 = min(p[1] for p in band), max(p[1] for p in band)
    raw = []
    for i in range(401):
        f = i / 400
        y, th = y0 + (y1 - y0) * f, 2 * math.pi * turns * f
        sel = [p for p in band if abs(p[1] - y) <= 0.3 and abs((math.atan2(p[2] - cz, p[0] - cx) - th + math.pi) % (2 * math.pi) - math.pi) <= 0.3]
        if not sel:
            continue
        r = max(math.hypot(p[0] - cx, p[2] - cz) for p in sel) + offset
        raw.append((cx + r * math.cos(th), y, cz + r * math.sin(th)))
    out, acc = ([list(map(lambda v: round(v, 3), raw[0]))] if raw else []), 0.0
    for a, b in zip(raw, raw[1:]):
        acc += math.dist(a, b)
        if acc >= spacing:
            out.append([round(v, 3) for v in b])
            acc = 0.0
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("mesh")
    ap.add_argument("--shape", required=True, choices=["top", "outline", "wrap"])
    ap.add_argument("--sprite")
    ap.add_argument("--near", nargs=2, type=float)
    ap.add_argument("--spacing", type=float, default=0.9)
    ap.add_argument("--offset", type=float, default=0.15)
    ap.add_argument("--ymin", type=float, default=-99)
    ap.add_argument("--ymax", type=float, default=99)
    ap.add_argument("--turns", type=float, default=3)
    ap.add_argument("--each", action="store_true", help="every matching actor, not the nearest")
    a = ap.parse_args()
    actors = json.load(open(a.mesh))["actors"]
    if a.sprite:
        actors = [x for x in actors if int(x["sprite"], 16) == int(a.sprite, 16)]
    if a.near:
        actors.sort(key=lambda x: math.hypot(x["pos"][0] - a.near[0], x["pos"][2] - a.near[1]))
    if not a.each:
        actors = actors[:1]
    paths = []
    for x in actors:
        pts = cloud(x["tris"])
        foot = x["pos"][1]
        if a.shape == "top":
            p = fit_top([q for q in pts if a.ymin <= q[1] - foot <= a.ymax], a.spacing, a.offset)
        elif a.shape == "outline":
            p = fit_outline(pts, a.spacing, a.offset, a.ymin, a.ymax, foot)
        else:
            p = fit_wrap(pts, a.spacing, a.offset, a.ymin, a.ymax, a.turns, foot)
        if p:
            paths.append({"sprite": x["sprite"], "zone": x["zone"], "pos": x["pos"], "points": p})
    json.dump({"paths": paths}, sys.stdout)
    print()


if __name__ == "__main__":
    main()
