#!/usr/bin/env python3
"""Christmas decorations' looks: globe bulbs, tinsel and rope garlands, wrapping paper (the owner, 2026-11-16: "where dust
is standing this model the magic globe can be used for bulbs and made different colors"; "the tent dust is standing has
material that can be used for a wrap and the rope hanging also can be used to decorate the tree"; "where dust is in this
camp we can use these tents as wrapping paper for presents"; "the tree where dust is has tinsel looking leaves we can
use").

Writes (source christmas-2026-looks, season Christmas):
  decorassets   the art the looks draw, read off each player's own disc by vanilla (christmas5), whatever world:
                  Magic globe          TUNARIA.ESF zone 108  0xF8E6C045 (Dust's table by the Lightwolf: the globe)
                  Tunaria plaid cloth  TUNARIA.ESF zone 28   0x58B41D37 0x6A41D388 (Odus camp tents' cloth; Tunaria has them)
                  Tunaria damask cloth TUNARIA.ESF zone 16   0x9EEAF67B 0xB078ACCC (the camp's other tents)
                  Odus tent cloth      ODUS.ESF zone 15      0x4BC0D7FA 0xB1C8A396 0x7B3D8677 (Dust's tent: its cloth,
                                                             its bow banner, its hanging rope net)
                  Odus tinsel leaves   ODUS.ESF zone 34      0x9196DAC4 (the spiky leaves of the tree Dust stood by)
  lighttemplates  Christmas globes (the globe at each bulb, a small glow), Gold tinsel, Silver tinsel, Red rope garland
                  (no glow: a textured cord, draped)
  lightstrings    the tree's garlands, between its rings of lights (--garland tinsel: gold and silver; rope: red rope;
                  both), and a globe string along the low wall by the coach (a copy of row 'the low wall ... 15727.4''s
                  path), with --globes also the tree's own lights turned to globes
  presents        the tree's presents row: every other box wrapped in a paper of the set (--papers), tinted its colour
                  made lighter, its gain the paper's (a dark cloth made as bright as plain paper)

A report by default; --apply writes it with gm_audit lines and keeps what it did in --applied; --undo puts every row back
as it was. Data only (needs server and vanilla christmas5); a PS2 draws none of it. Live by the GM panel: touch a Light
strings row; a restart reads it too.

    christmas_decor_looks_20261116.py --db eqoa.db --applied FILE [--apply | --undo] [--garland tinsel|rope|both|none]
        [--globes] [--papers all|tunaria]
"""
import argparse, colorsys, json, math, os, sqlite3, sys, time

SRC, SEASON = "christmas-2026-looks", "Christmas"
ap = argparse.ArgumentParser()
ap.add_argument("--db", required=True)
ap.add_argument("--applied", required=True)
ap.add_argument("--garland", default="tinsel", choices=["tinsel", "rope", "both", "none"])
ap.add_argument("--globes", action="store_true", help="the tree's own lights as globes too")
ap.add_argument("--papers", default="all", choices=["all", "tunaria"])
g = ap.add_mutually_exclusive_group()
g.add_argument("--apply", action="store_true")
g.add_argument("--undo", action="store_true")
a = ap.parse_args()
db = sqlite3.connect(a.db, timeout=30)
now = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())

ASSETS = [
    ("Magic globe", 0, 108, "0xF8E6C045", "the magic globe on the table where Dust stood by Castle Lightwolf (21117.7, 69.0, 19019.8)"),
    ("Tunaria plaid cloth", 0, 28, "0x58B41D37 0x6A41D388", "teal and orange plaid tent cloth (Odus's camp at 9117, 11599 wears them; Tunaria's zones hold them)"),
    ("Tunaria damask cloth", 0, 16, "0x9EEAF67B 0xB078ACCC", "red and teal damask tent cloth (the camp's other tents)"),
    ("Odus tent cloth", 2, 15, "0x4BC0D7FA 0xB1C8A396 0x7B3D8677", "Dust's tent at 8838.9, 4388.5: its blue cloth, its bow banner, its hanging rope net"),
    ("Odus tinsel leaves", 2, 34, "0x9196DAC4", "the spiky leaves of the tree Dust stood by at 10342.1, 11206.6"),
]
TEMPLATES = [
    # name, colors, pattern, effect, period, spacing, size, bright, cord, cord_width, bulb, bulb_scale, cord_texture, cord_sag, note
    ("Christmas globes", "FF2A2A, 2AE04A, 3A78FF, FFC23A, FFFFFF, FF4FD0", "random", "twinkle", 2.4, 1.1, 0.95, 0.9, "1E3F1E", 0.07,
     "0xF8E6C045", 0.42, "", 0, "The magic globe at each bulb, tinted its colour, with a soft glow round it: red, green, blue, gold, white, pink."),
    ("Gold tinsel", "FFD040", "cycle", "steady", 1.5, 1.6, 0, 1.0, "E0B040", 0.55, "", 0.35, "0x9196DAC4", 0.22,
     "A garland of tinsel: the spiky leaves of Odus's trees, gold, draped between the lights. No bulbs."),
    ("Silver tinsel", "E0E0F0", "cycle", "steady", 1.5, 1.6, 0, 1.0, "D8DCEC", 0.55, "", 0.35, "0x9196DAC4", 0.22,
     "A garland of tinsel: the spiky leaves of Odus's trees, silver, draped between the lights. No bulbs."),
    ("Red rope garland", "C02020", "cycle", "steady", 1.5, 1.8, 0, 1.0, "D03030", 0.6, "", 0.35, "0x7B3D8677", 0.2,
     "A garland of the rope net that hangs from Odus's tents, red, draped between the lights. No bulbs."),
]
PAPERS = {  # DictID: gain (about 200 / the texture's mean brightness, kept to 3)
    "0x58B41D37": 2.6, "0x6A41D388": 1.25, "0x9EEAF67B": 2.2, "0xB078ACCC": 2.2, "0x4BC0D7FA": 2.5, "0xB1C8A396": 2.8,
}
TUNARIA_PAPERS = ["0x58B41D37", "0x6A41D388", "0x9EEAF67B", "0xB078ACCC"]
COLS = ["id", "template", "npc", "world", "shape", "params", "season", "note", "source"]


def audit(action, tbl, key, before, after):
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) VALUES (?, 1, 'dab', 'tool', '', ?, ?, ?, ?, ?, ?)",
               (now, action, tbl, json.dumps(key), None if before is None else json.dumps(before), None if after is None else json.dumps(after),
                "the owner: globe bulbs, tinsel, rope and tent-cloth paper; tools/christmas_decor_looks_20261116.py"))


def ensure_schema():
    """The columns and table server christmas5 makes at its start, made here too (the tool may run before it)."""
    have = {r[1] for r in db.execute("PRAGMA table_info(lighttemplates)")}
    for c, d in [("bulb", "TEXT NOT NULL DEFAULT ''"), ("bulb_scale", "REAL NOT NULL DEFAULT 0.35"), ("cord_texture", "TEXT NOT NULL DEFAULT ''"),
                 ("cord_repeat", "REAL NOT NULL DEFAULT 1"), ("cord_sag", "REAL NOT NULL DEFAULT 0")]:
        if c not in have:
            db.execute(f"ALTER TABLE lighttemplates ADD COLUMN {c} {d}")
    db.execute("""CREATE TABLE IF NOT EXISTS decorassets (name TEXT PRIMARY KEY COLLATE NOCASE, world INTEGER NOT NULL DEFAULT 0,
        zone INTEGER NOT NULL DEFAULT 0, ids TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm')""")


def lighter(hexc, f=0.45):
    r, g_, b = (int(hexc[i:i + 2], 16) / 255 for i in (0, 2, 4))
    return "%02X%02X%02X" % tuple(round(255 * (v + (1 - v) * f)) for v in (r, g_, b))


def ring_between(p0, p1, step):
    """A garland halfway up between two of the tree's rings of lights (path rows' points, the tree's frame): at each angle
    the radius halfway between theirs, a little proud of the boughs, a point every `step` along it."""
    def by_angle(pts):
        bins = {}
        for x, y, z in pts:
            k = int((math.degrees(math.atan2(z, x)) % 360) // 10)
            bins.setdefault(k, []).append(math.hypot(x, z))
        out = []
        for k in range(36):
            if k in bins:
                out.append(sum(bins[k]) / len(bins[k]))
            else:
                out.append(None)
        # fill gaps from the neighbours
        for k in range(36):
            if out[k] is None:
                for d in range(1, 18):
                    for kk in ((k - d) % 36, (k + d) % 36):
                        if out[kk] is not None:
                            out[k] = out[kk]
                            break
                    if out[k] is not None:
                        break
        return out
    r0, r1 = by_angle(p0), by_angle(p1)
    y = (sum(p[1] for p in p0) / len(p0) + sum(p[1] for p in p1) / len(p1)) / 2
    rmid = [1.04 * (u + v) / 2 for u, v in zip(r0, r1)]
    circ = sum(rmid) / 36 * 2 * math.pi
    n = max(8, int(circ / step))
    pts = []
    for i in range(n + 1):
        t = i / n * 360
        k = int(t // 10) % 36
        f = (t % 10) / 10
        r = rmid[k] * (1 - f) + rmid[(k + 1) % 36] * f
        th = math.radians(t)
        pts.append([round(r * math.cos(th), 3), round(y + 0.4 * math.sin(3 * th), 3), round(r * math.sin(th), 3)])
    return pts


if a.undo:
    j = json.load(open(a.applied))
    for sid in j["strings"]:
        r = db.execute(f"SELECT {', '.join(COLS)} FROM lightstrings WHERE id = ?", (sid,)).fetchone()
        if r is not None and r[COLS.index("source")] == SRC:
            db.execute("DELETE FROM lightstrings WHERE id = ?", (sid,))
            audit("delete", "lightstrings", {"id": sid}, dict(zip(COLS, r)), None)
    for sid, before in j["changed"].items():
        now_row = db.execute("SELECT template, params, note FROM lightstrings WHERE id = ?", (int(sid),)).fetchone()
        db.execute("UPDATE lightstrings SET template = ?, params = ?, note = ? WHERE id = ?", (before["template"], before["params"], before["note"], int(sid)))
        audit("update", "lightstrings", {"id": int(sid)}, None if now_row is None else {"template": now_row[0], "note": now_row[2]}, {"template": before["template"], "note": before["note"]})
    for name in j["templates"]:
        db.execute("DELETE FROM lighttemplates WHERE name = ? AND source = ?", (name, SRC))
        audit("delete", "lighttemplates", {"name": name}, {"source": SRC}, None)
    for name in j["assets"]:
        db.execute("DELETE FROM decorassets WHERE name = ? AND source = ?", (name, SRC))
        audit("delete", "decorassets", {"name": name}, {"source": SRC}, None)
    db.commit(); os.rename(a.applied, a.applied + ".undone"); print("undone"); sys.exit()
if a.apply and os.path.exists(a.applied):
    sys.exit("applied already")
ensure_schema()
if db.execute("SELECT count(*) FROM decorassets WHERE source = ?", (SRC,)).fetchone()[0]:
    sys.exit("these looks are here already: nothing done")
tree = db.execute("SELECT id, params FROM lightstrings WHERE npc = 'Christmas Tree' AND shape = 'path' AND template = 'Christmas lights' ORDER BY id").fetchall()
gifts = db.execute("SELECT id, template, params, note FROM lightstrings WHERE npc = 'Christmas Tree' AND shape = 'presents'").fetchone()
wall = db.execute("SELECT params FROM lightstrings WHERE note = ? AND npc = ''", ("the low wall by Freeport's coach, its segment at (25308.5, 15727.4)",)).fetchone()
if len(tree) < 2 or gifts is None:
    sys.exit("no Christmas tree's lights and presents (christmas3's tools first): nothing done")
done = {"assets": [], "templates": [], "strings": [], "changed": {}}
for name, world, zone, ids, note in ASSETS:
    db.execute("INSERT INTO decorassets (name, world, zone, ids, note, source) VALUES (?, ?, ?, ?, ?, ?)", (name, world, zone, ids, note, SRC))
    audit("insert", "decorassets", {"name": name}, None, {"world": world, "zone": zone, "ids": ids})
    done["assets"].append(name)
for t in TEMPLATES:
    name = t[0]
    if db.execute("SELECT count(*) FROM lighttemplates WHERE name = ? COLLATE NOCASE", (name,)).fetchone()[0]:
        sys.exit(f"a light template {name!r} is here already: nothing done")
    db.execute("""INSERT INTO lighttemplates (name, colors, pattern, effect, period, spacing, size, bright, cord, cord_width, bulb, bulb_scale,
        cord_texture, cord_sag, note, source) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)""", t + (SRC,))
    audit("insert", "lighttemplates", {"name": name}, None, {"colors": t[1], "bulb": t[10], "cord_texture": t[12]})
    done["templates"].append(name)


def add_string(template, npc, world, params, note):
    sid = db.execute("INSERT INTO lightstrings (template, npc, world, shape, params, season, note, source) VALUES (?, ?, ?, 'path', ?, ?, ?, ?)",
                     (template, npc, world, json.dumps(params, separators=(",", ":")), SEASON, note, SRC)).lastrowid
    audit("insert", "lightstrings", {"id": sid}, None, {"template": template, "npc": npc, "note": note, "points": len(params["points"])})
    done["strings"].append(sid)


rings = [json.loads(p)["points"] for _, p in tree]
kinds = {"tinsel": ["Gold tinsel", "Silver tinsel"], "rope": ["Red rope garland"], "both": ["Gold tinsel", "Red rope garland"], "none": []}[a.garland]
for i in range(len(rings) - 1):
    if not kinds:
        break
    t = kinds[i % len(kinds)]
    add_string(t, "Christmas Tree", 0, {"points": ring_between(rings[i], rings[i + 1], 1.6)}, f"the tree's {t.lower()}, between its rings of lights {i + 1} and {i + 2}")
if wall is not None:
    add_string("Christmas globes", "", 0, json.loads(wall[0]), "globes along the low wall by Freeport's coach (beside its lights)")
if a.globes:
    for sid, p in tree:
        r = db.execute("SELECT template, params, note FROM lightstrings WHERE id = ?", (sid,)).fetchone()
        done["changed"][str(sid)] = {"template": r[0], "params": r[1], "note": r[2]}
        db.execute("UPDATE lightstrings SET template = 'Christmas globes', note = note || ' (globes)' WHERE id = ?", (sid,))
        audit("update", "lightstrings", {"id": sid}, {"template": r[0]}, {"template": "Christmas globes"})
# the papers: every other box (but the stacked tops' bottoms stay plain), a paper of the set in turn
papers = TUNARIA_PAPERS if a.papers == "tunaria" else list(PAPERS)
pj = json.loads(gifts[2])
k = 0
for i, b in enumerate(pj["boxes"]):
    if i % 2 == 1:
        p = papers[k % len(papers)]
        k += 1
        b["paper"], b["gain"] = p, PAPERS[p]
        b["color"] = lighter(b["color"])
done["changed"][str(gifts[0])] = {"template": gifts[1], "params": gifts[2], "note": gifts[3]}
db.execute("UPDATE lightstrings SET params = ?, note = ? WHERE id = ?", (json.dumps(pj, separators=(",", ":")), gifts[3] + f"; {k} wrapped in tent cloth", gifts[0]))
audit("update", "lightstrings", {"id": gifts[0]}, {"papers": 0}, {"papers": k})
print(f"{len(done['assets'])} assets, {len(done['templates'])} templates, {len(done['strings'])} strings ({a.garland} garlands, a globe wall),"
      f" {k} presents wrapped in {len(papers)} papers{', the tree in globes' if a.globes else ''}")
if a.apply:
    json.dump(done, open(a.applied, "w")); db.commit(); print("applied")
else:
    db.rollback()
