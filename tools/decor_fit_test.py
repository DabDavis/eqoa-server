#!/usr/bin/env python3
"""decor_fit.py on made-up meshes: a wall's top runs its length at its height plus the offset; a ring's outline goes
round it at its radius plus the offset, closed; a post's wrap winds up it outside its surface."""
import json
import math
import os
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import decor_fit as f  # noqa: E402


def box(x0, x1, y0, y1, z0, z1):
    c = [(x, y, z) for x in (x0, x1) for y in (y0, y1) for z in (z0, z1)]
    q = [(0, 1, 3, 2), (4, 5, 7, 6), (0, 1, 5, 4), (2, 3, 7, 6), (0, 2, 6, 4), (1, 3, 7, 5)]
    tris = []
    for a, b, cc, d in q:
        tris += [[c[a], c[b], c[cc]], [c[a], c[cc], c[d]]]
    return tris


def cylinder(r, h, n=24):
    tris = []
    for i in range(n):
        a, b = 2 * math.pi * i / n, 2 * math.pi * (i + 1) / n
        p = [(r * math.cos(a), 0, r * math.sin(a)), (r * math.cos(b), 0, r * math.sin(b)), (r * math.cos(b), h, r * math.sin(b)), (r * math.cos(a), h, r * math.sin(a))]
        tris += [[p[0], p[1], p[2]], [p[0], p[2], p[3]]]
    return tris


class DecorFitTest(unittest.TestCase):
    def test_top(self):
        p = f.fit_top(f.cloud(box(0, 10, 0, 2, 0, 1)), 0.9, 0.15)
        self.assertGreaterEqual(len(p), 10)
        self.assertTrue(all(abs(q[1] - 2.15) < 1e-6 for q in p), p)
        xs = [q[0] for q in p]
        self.assertTrue(min(xs) < 0.6 and max(xs) > 9.4, "end to end")

    def test_outline(self):
        p = f.fit_outline(f.cloud(cylinder(4, 1.2)), 0.8, 0.12, 0, 1.3, 0)
        self.assertEqual(p[0], p[-1], "closed")
        self.assertTrue(all(abs(math.hypot(q[0], q[2]) - 4.12) < 0.1 for q in p))
        self.assertTrue(all(abs(q[1] - 1.32) < 1e-6 for q in p))

    def test_wrap(self):
        p = f.fit_wrap(f.cloud(cylinder(0.3, 4)), 0.5, 0.1, 0.5, 3.5, 3, 0)
        self.assertGreater(len(p), 10)
        self.assertTrue(all(math.hypot(q[0], q[2]) > 0.3 for q in p), "outside the post")
        self.assertLess(p[0][1], p[-1][1], "rising")

    def test_command_line(self):
        d = tempfile.mkdtemp()
        mesh = os.path.join(d, "mesh.json")
        json.dump({"actors": [{"zone": 84, "sprite": "0xECF3ECAE", "pos": [5, 0, 0.5], "rot": 0, "scale": 1, "tris": box(0, 10, 0, 2, 0, 1)}]}, open(mesh, "w"))
        r = subprocess.run([sys.executable, os.path.join(HERE, "decor_fit.py"), mesh, "--shape", "top", "--sprite", "0xecf3ecae"], capture_output=True, text=True)
        self.assertEqual(r.returncode, 0, r.stderr)
        out = json.loads(r.stdout)["paths"]
        self.assertEqual(len(out), 1)
        self.assertEqual(out[0]["zone"], 84)


if __name__ == "__main__":
    unittest.main()
