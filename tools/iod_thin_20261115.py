#!/usr/bin/env python3
"""The Isle of Dread's common creatures: their melee put where their template's note means it, the template given to
Live, and 60% of their spawns held out of the world (the owner, 2026-11-15: "the Isle of dread common mobs lets reduce
density by 60% players cannot manage so many. also the mobs are hitting players for no damage hardly. the new damage
formula may be why and the Isle of dread creatures need a template").

THE DAMAGE. Not the formula: Heirloom's runs in shadow (it logs, it changes no hit) and Live has none. The common
creatures fight by classtemplates "iod native" ("iod crawler" for the flesh crawler, the same plus an AoE), melee_pct
200, whose note asks "108 a hit on a L60 warrior at AC 2-3k". An NPC's melee on a player is (entity/defence.go,
systems.go npcAttackPlayer, balance.go):

    roll  = creatureRoll(53): floor 27, top 234.2, width 158, AC slope 0.1161
    top   = 234.2 - 0.1161*AC - 0.305*AGI (a tank)        at AC 2500, AGI 250: -132, so every roll is under the floor
    hit   = softHit (melee_pct > 100: petition 1289's window, commit 6e861e69, 2026-11-08 20:10):
            13.5 + 13.5*158/(158 + 27 - roll), roll -290..-132: 18.0..20.2 (two thirds of the floor)
    x melee_pct 200 -> 36..40;  x balancelevel 53 (181%: 190 at 50, 160 at 60) -> 65..73;
    x balancecon -7 (100);  x balanceclasslevel warrior 60 (Heirloom 85%, Live 100%) -> Heirloom 55-61, mean 58.3
    then a mitigation spell's share (only with one on), the formula (shadow: nothing), melee_fixed (0).

Heirloom's log today has the creatures at 49-64 on its tanks: the same. AC is taken once (in top); nothing is applied
twice. The 108 was reckoned the day the template was made, before the window, when a roll under the floor hit for the
floor times melee_pct (27 x 200% = 54, x ~200% of the level's balance = 108); the window lands those rolls at 50-100%
of the floor, and Heirloom's warriors take 85%. melee_pct is set so the mean on a L60 warrior of AC 2500, AGI 250 is
108 again, by the shard's own warrior share (balanceclasslevel class 0 level 60):

    Heirloom (85%):  melee_pct 370 -> 103-116, mean 108.1 (AC 2000: 113.6, AC 3000: 103.9)
    Live (100%):     melee_pct 315 -> 103-116, mean 108.1 (AC 2000: 113.6, AC 3000: 104.0)

melee_pct also scales a roll over the floor: a low-AC player takes 1.85x (Heirloom) what he took (a wizard at AC
1000, AGI 150: mean 250 -> 462).

THE TEMPLATES ON LIVE. Live's 12 of these names fight as "roamer" (npctemplate, roamer-seed / sandstorm-census /
virulentblade1114), siren and Rotting Disease by none; Live has no "iod native" or "iod crawler". They are made as
Heirloom's rows are (every column; melee_pct as above, the note saying so) and the 14 names mapped as Heirloom maps
them (its rows' notes and sources).

THE DENSITY. A row stops standing in the world, kept whole, by npcspawnonly "#<row>" (entity/spawnonly.go
npcSpawnOnlyLocked: in no view, attacks and helps no one, never wanders; the GM panel's "out of the world"); a
removal would take its "#n" rows and respawn with it. Of each common name's standing rows in world 0 on the island
(x 24000-26000, z 2000-4000), 40% stay (round half up, at least 1): the rows are joined into camps (within 40 of one
another, every common name together), each camp keeps floor(40%) of each name's rows in it (the ones spread farthest
apart in the camp, from the one nearest its middle), and the rest of the name's 40% are the rows standing farthest
from every row it keeps, the island over. Left out (never held): a name standing out of the world already (Rotting
Disease, the Dracoliche's add), a row held already, a row that stands despite its name (npcstands), a rare's
placeholder (npcrare), a quest step's own NPC (queststeps.npcid), a row with its own template or loot ("#n"). Quest
101310 kills dread rots and wicker men: 40% of them stay. Named, boss and quest creatures are other names: untouched.

Report only by default; --apply in one transaction with gm_audit lines and the log iod_thin_1115_log (every row
before and after); a second --apply does nothing; --undo puts back exactly what it wrote (a row changed since is left
and said) and drops the log.

WHEN IT TAKES EFFECT. Templates and npctemplate: the GM panel's reload_npcs (it reads classtemplates and npctemplate
first), or a restart. npcspawnonly: read at the start, or live after a panel edit of an npcspawnonly row (any:
update_row of one of this tool's rows' note and back again reads the whole table); reload_npcs does not read it.
After --undo the same: a restart or a panel edit of npcspawnonly brings the held rows back.

    python3 -E -s tools/iod_thin_20261115.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import math
import sqlite3
import sys

LOG = "iod_thin_1115_log"
SRC = "iodthin1115"
WHO, VIA = "dab", "tool"
DETAIL = ("the Isle of Dread's common creatures (the owner, 2026-11-15): melee_pct to the template's 108 on a L60 "
          "warrior, the templates on Live, 60% of their spawns held out of the world: tools/iod_thin_20261115.py")
WORLD = 0
BOX = (24000.0, 2000.0, 26000.0, 4000.0)  # x0, z0, x1, z1: the island
KEEP = 0.4
LINK = 40.0

# the common creatures and their template (Heirloom's npctemplate, work board 430 + dracoliche1114 + virulentblade1114)
COMMON = {
    "breath stealer": "iod native", "dead rider": "iod native", "dead rot": "iod native", "devourer": "iod native",
    "dread rot": "iod native", "flesh crawler": "iod crawler", "imprisoned soul": "iod native",
    "manta rays": "iod native", "ray of agony": "iod native", "regan piranha": "iod native", "siren": "iod native",
    "virulent blade": "iod native", "wicker man": "iod native", "rotting disease": "iod native",
}
# Heirloom's npctemplate rows' notes and sources, as they are there
MAP_NOTE = {
    "devourer": ("work board 430 (jibrish 2026-11-08): Sandstorm census rows in IoD's south waters, put level with the "
                 "piranhas beside them", "workboard 430"),
    "ray of agony": ("work board 430 (jibrish 2026-11-08): Sandstorm census rows in IoD's south waters, put level with "
                     "the manta rays beside them", "workboard 430"),
    "rotting disease": ("the Dracoliche's add: the island's natives' profile", "dracoliche1114"),
    "siren": ("work board 430 (jibrish 2026-11-08): the IoD profile (had no row)", "workboard 430"),
    "virulent blade": ("the island's natives' (wicker man, breath stealer)", "virulentblade1114"),
}
MAP_DEFAULT = ("work board 430 (jibrish 2026-11-08): the IoD profile", "workboard 430")

NATIVE_NOTE = ("The Isle of Dread's ordinary creatures, L53 (work board 430, jibrish 2026-11-08): 'All normal IoD mobs "
               "are lvl 53 ... undercons (low lvls with abilities of higher lvls)' [forum 2004]; 50k effective (a dead "
               "rider takes a group of 4 about 2 min [forum 2003]: 48-96k), 108 a hit on a L60 warrior at AC 2-3k "
               "(ours), the roamer's crit at 800 ('critical hit is 800 some' [forum 2003]), the isle's AR 50 (t281; 50 "
               "ours).")
CRAWLER_NOTE = ("Flesh crawler, L53 (work board 430, jibrish 2026-11-08): iod native plus a 1,000 AoE every 20 s "
                "('they do a 1000 dmg AoE spell' [forum 2003]; timer ours).")
# Heirloom's rows, every column, as they stood before this tool (melee_pct and note are this tool's on each shard)
TEMPLATE_ROWS = {
    "iod native": {"name": "iod native", "class": 0, "hp_pct": 439, "taken_pct": 100, "melee_pct": 200,
                   "special": "Critical Hit", "special_chance": 0, "special_damage": 15.0943396226415, "special_fx": 0,
                   "abilities": "[]", "note": NATIVE_NOTE, "source": "workboard 430", "special_every": 30.0,
                   "special_first": 15.0, "special_jitter": 5.0, "special_from": 10, "shield_damage": 0,
                   "shield_resist": "", "proc_chance": 0, "proc_damage": 0, "proc_name": "", "proc_resist": "",
                   "resist_fail": 0.0, "resist_kind": "AR", "resist_pct": 50, "kick_damage": 0, "kick_every": 0.0,
                   "kick_first": 0.0, "fight_regen": 0, "run_speed": 0.0, "taken_resist": "", "melee_fixed": 0},
    "iod crawler": {"name": "iod crawler", "class": 0, "hp_pct": 439, "taken_pct": 100, "melee_pct": 200,
                    "special": "Critical Hit", "special_chance": 0, "special_damage": 15.0943396226415, "special_fx": 0,
                    "abilities": '[{"kind":"aoe","spell_id":905002,"damage_pct":67,"every":20,"first":8}]',
                    "note": CRAWLER_NOTE, "source": "workboard 430", "special_every": 30.0, "special_first": 15.0,
                    "special_jitter": 5.0, "special_from": 10, "shield_damage": 0, "shield_resist": "",
                    "proc_chance": 0, "proc_damage": 0, "proc_name": "", "proc_resist": "", "resist_fail": 0.0,
                    "resist_kind": "AR", "resist_pct": 50, "kick_damage": 0, "kick_every": 0.0, "kick_first": 0.0,
                    "fight_regen": 0, "run_speed": 0.0, "taken_resist": "", "melee_fixed": 0},
}
# melee_pct by the shard's warrior share at 60 (balanceclasslevel class 0 level 60): 108 mean at AC 2500, AGI 250
MELEE_BY_WARRIOR_SHARE = {85.0: 370, 100.0: 315}
MELEE_WORDS = ("(melee_pct 200 -> {m}, 2026-11-15, the owner's 'hitting players for no damage hardly': the 200 hit a L60 "
               "warrior at AC 2,500 for 55-61, mean 58, under petition 1289's window and this shard's warrior share "
               "{s:g}%; {m} lands 103-116, mean 108: tools/iod_thin_20261115.py)")


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def rows(db, sql, args=()):
    cur = db.execute(sql, args)
    cols = [d[0] for d in cur.description]
    return [dict(zip(cols, r)) for r in cur.fetchall()]


def go_json(v):
    return json.dumps(v, separators=(",", ":"), sort_keys=True)


def ensure_log(db):
    db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (seq INTEGER PRIMARY KEY AUTOINCREMENT, tbl TEXT NOT NULL, "
               "keycols TEXT NOT NULL, before TEXT, after TEXT, why TEXT NOT NULL DEFAULT '')")


def log(db, tbl, keys, before, after, why):
    db.execute(f"INSERT INTO {LOG} (tbl, keycols, before, after, why) VALUES (?, ?, ?, ?, ?)",
               (tbl, json.dumps(list(keys)), json.dumps(before, sort_keys=True) if before is not None else None,
                json.dumps(after, sort_keys=True) if after is not None else None, why))


def audit(db, tbl, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() \
        if table(db, "accountinfo") else None
    now = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', ?, ?, ?, ?, ?)",
               (now, acct[0] if acct else 0, WHO, VIA, tbl, go_json({"tool": "iod_thin_20261115", "log": LOG}),
                go_json(before), go_json(after), detail))


def insert(db, tbl, r):
    cols = list(r)
    db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [r[c] for c in cols])


def current(db, tbl, keys, r):
    got = rows(db, f"SELECT * FROM {tbl} WHERE {' AND '.join(f'{k} = ?' for k in keys)}", [r[k] for k in keys])
    return got[0] if got else None


# ---------------------------------------------------------------- the templates

def warrior_share(db):
    r = db.execute("SELECT melee_taken_pct FROM balanceclasslevel WHERE class = 0 AND level = 60").fetchone() \
        if table(db, "balanceclasslevel") else None
    c = db.execute("SELECT melee_taken_pct FROM balanceclass WHERE class = 0").fetchone() \
        if table(db, "balanceclass") else None
    return (r[0] if r else 100.0) * (c[0] if c else 100.0) / 100.0


def plan_templates(db):
    """(changes, lines): changes are (tbl, keys, before, after)."""
    share = warrior_share(db)
    melee = MELEE_BY_WARRIOR_SHARE.get(share)
    if melee is None:
        raise SystemExit(f"refused, nothing done: this shard's warrior share at 60 is {share:g}%, not one this tool "
                         f"was reckoned for ({', '.join(f'{k:g}' for k in MELEE_BY_WARRIOR_SHARE)})")
    words = MELEE_WORDS.format(m=melee, s=share)
    changes, lines = [], [f"warrior share at 60: {share:g}% -> melee_pct {melee}"]
    for name, base in TEMPLATE_ROWS.items():
        old = current(db, "classtemplates", ("name",), {"name": name})
        if old is None:
            new = dict(base, melee_pct=melee, note=base["note"] + " " + words, source=SRC)
            changes.append(("classtemplates", ("name",), None, new))
            lines.append(f"classtemplates {name!r}: made, as Heirloom's, melee_pct {melee}")
        elif old["melee_pct"] == melee:
            lines.append(f"classtemplates {name!r}: melee_pct {melee} already")
        elif old["melee_pct"] == 200:
            new = dict(old, melee_pct=melee, note=old["note"] + " " + words)
            changes.append(("classtemplates", ("name",), old, new))
            lines.append(f"classtemplates {name!r}: melee_pct 200 -> {melee}")
        else:
            lines.append(f"classtemplates {name!r}: melee_pct {old['melee_pct']} (not 200): left for a GM")
    for name, tmpl in sorted(COMMON.items()):
        old = rows(db, "SELECT * FROM npctemplate WHERE lower(trim(name)) = ?", (name,))
        note, src = MAP_NOTE.get(name, MAP_DEFAULT)
        if not old:
            changes.append(("npctemplate", ("name",), None, {"name": name, "template": tmpl, "note": note, "source": src}))
            lines.append(f"npctemplate {name!r}: none -> {tmpl!r}")
        elif old[0]["template"] == tmpl:
            lines.append(f"npctemplate {name!r}: {tmpl!r} already")
        else:
            o = old[0]
            changes.append(("npctemplate", ("name",), o, dict(o, template=tmpl, note=note, source=src)))
            lines.append(f"npctemplate {name!r}: {o['template']!r} -> {tmpl!r}")
    return changes, lines


# ---------------------------------------------------------------- the density

def held_keys(db):
    return {r[0] for r in db.execute("SELECT name FROM npcspawnonly")} if table(db, "npcspawnonly") else set()


def lowered(db, sql):
    return {(r[0] or "").strip().lower() for r in db.execute(sql)}


def ids(db, tbl, col):
    return {r[0] for r in db.execute(f"SELECT {col} FROM {tbl}")} if table(db, tbl) else set()


def candidates(db):
    """{name: [row]} of the common rows that may be held, and the lines of those left out."""
    held = held_keys(db)
    held_names = {k.strip().lower().rstrip("$") for k in held if not k.startswith("#")}
    stands, rares = ids(db, "npcstands", "npc_row"), ids(db, "npcrare", "npc_row")
    stepnpc = ids(db, "queststeps", "npcid")
    own = lowered(db, "SELECT name FROM npctemplate WHERE name LIKE '#%'") | \
        (lowered(db, "SELECT npc_name FROM npcloot WHERE npc_name LIKE '#%'") if table(db, "npcloot") else set())
    names = sorted(COMMON)
    got = rows(db, f"SELECT id, npc_name, x_coord, z_coord, world FROM npcs WHERE lower(trim(npc_name)) IN "
                   f"({', '.join('?' * len(names))}) ORDER BY id", names)
    out, left = {}, []
    for r in got:
        n = r["npc_name"].strip().lower()
        x, z = r["x_coord"], r["z_coord"]
        why = None
        if n in held_names:
            why = "its name stands out of the world (npcspawnonly)"
        elif f"#{r['id']}" in held:
            why = "held already (npcspawnonly #row)"
        elif r["world"] != WORLD or not (BOX[0] <= x <= BOX[2] and BOX[1] <= z <= BOX[3]):
            why = f"not on the island (world {r['world']}, {x:.0f}, {z:.0f})"
        elif r["id"] in stands:
            why = "stands despite its name (npcstands)"
        elif r["id"] in rares:
            why = "a rare's placeholder (npcrare)"
        elif r["id"] in stepnpc:
            why = "a quest step's NPC (queststeps.npcid)"
        elif f"#{r['id']}" in own:
            why = "has its own template or loot (#row)"
        if why:
            left.append(f"{r['id']} {r['npc_name']}: {why}")
            continue
        out.setdefault(n, []).append(r)
    return out, left


def dist(a, b):
    return math.hypot(a["x_coord"] - b["x_coord"], a["z_coord"] - b["z_coord"])


def camps(all_rows):
    """Row id -> camp: rows within LINK of one another, every common name together."""
    par = {r["id"]: r["id"] for r in all_rows}

    def find(i):
        while par[i] != i:
            par[i] = par[par[i]]
            i = par[i]
        return i
    for i, a in enumerate(all_rows):
        for b in all_rows[i + 1:]:
            if dist(a, b) <= LINK:
                par[find(a["id"])] = find(b["id"])
    return {r["id"]: find(r["id"]) for r in all_rows}


def spread(pool, kept, k):
    """k rows of pool, each the one farthest from every row kept (the first, when none is, the one nearest the pool's
    middle); ties by id."""
    pool, kept, out = list(pool), list(kept), []
    for _ in range(k):
        if not pool:
            break
        if kept:
            best = max(pool, key=lambda r: (min(dist(r, q) for q in kept), -r["id"]))
        else:
            cx = sum(r["x_coord"] for r in pool) / len(pool)
            cz = sum(r["z_coord"] for r in pool) / len(pool)
            best = min(pool, key=lambda r: (math.hypot(r["x_coord"] - cx, r["z_coord"] - cz), r["id"]))
        pool.remove(best)
        kept.append(best)
        out.append(best)
    return out


def keep_count(n):
    return max(1, int(math.floor(KEEP * n + 0.5))) if n else 0


def pick(by_name):
    """{name: (kept rows, held rows)}."""
    camp = camps([r for rs in by_name.values() for r in rs])
    out = {}
    for name, rs in sorted(by_name.items()):
        k = keep_count(len(rs))
        groups = {}
        for r in rs:
            groups.setdefault(camp[r["id"]], []).append(r)
        kept = []
        for c in sorted(groups, key=lambda c: min(r["id"] for r in groups[c])):
            kept += spread(groups[c], [], int(math.floor(KEEP * len(groups[c]))))
        kept_ids = {r["id"] for r in kept}
        kept += spread([r for r in rs if r["id"] not in kept_ids], kept, k - len(kept))
        kept_ids = {r["id"] for r in kept}
        out[name] = (sorted(kept, key=lambda r: r["id"]), [r for r in rs if r["id"] not in kept_ids])
    return out


def plan_density(db):
    if not table(db, "npcspawnonly"):
        raise SystemExit("refused, nothing done: no npcspawnonly table here")
    done = db.execute("SELECT count(*) FROM npcspawnonly WHERE source = ?", (SRC,)).fetchone()[0]
    by_name, left = candidates(db)
    lines = []
    if done:
        lines.append(f"density: applied already ({done} rows held by {SRC})")
        for name, rs in sorted(by_name.items()):
            lines.append(f"  {name}: {len(rs)} standing")
        return [], lines, left
    changes = []
    picked = pick(by_name)
    lines.append(f"{'name':16} {'before':>6} {'after':>6} {'held':>5}")
    tb = ta = 0
    for name, (kept, held) in picked.items():
        n = len(kept) + len(held)
        tb, ta = tb + n, ta + len(kept)
        lines.append(f"{name:16} {n:6d} {len(kept):6d} {len(held):5d}")
        for r in held:
            changes.append(("npcspawnonly", ("name",), None, {
                "name": f"#{r['id']}", "source": SRC,
                "note": (f"{r['npc_name']}: one of the Isle of Dread's common creatures held out of the world, 60% of "
                         f"them (the owner, 2026-11-15: 'players cannot manage so many'): tools/iod_thin_20261115.py")}))
    lines.append(f"{'all':16} {tb:6d} {ta:6d} {tb - ta:5d}")
    return changes, lines, left


# ---------------------------------------------------------------- apply / undo

def do_apply(db, changes):
    ensure_log(db)
    by_tbl = {}
    for tbl, keys, before, after in changes:
        if before is None:
            if current(db, tbl, keys, after) is not None:
                raise RuntimeError(f"{tbl} {[after[k] for k in keys]} appeared under the transaction")
            insert(db, tbl, after)
        else:
            sets = [c for c in after if after[c] != before[c]]
            w = " AND ".join(f"{k} = ?" for k in keys)
            cur = db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in sets)} WHERE {w}",
                             [after[c] for c in sets] + [before[k] for k in keys])
            if cur.rowcount != 1:
                raise RuntimeError(f"{tbl} {[before[k] for k in keys]} changed under the transaction")
        log(db, tbl, keys, before, current(db, tbl, keys, after), "apply")
        by_tbl.setdefault(tbl, []).append(after[keys[0]])
    for tbl, keys in sorted(by_tbl.items()):
        audit(db, tbl, {}, {"rows": keys if len(keys) <= 40 else len(keys)}, f"{DETAIL}: {tbl} {len(keys)} row(s)")


def do_undo(db):
    if not table(db, LOG):
        print("nothing to undo")
        return 0
    back = kept = 0
    for seq, tbl, keycols, before, after in db.execute(
            f"SELECT seq, tbl, keycols, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        keys = json.loads(keycols)
        was = json.loads(before) if before else None
        wrote = json.loads(after) if after else None
        ref = wrote if wrote is not None else was
        now = current(db, tbl, keys, ref)
        if wrote is not None and (now is None or json.dumps(now, sort_keys=True) != json.dumps(wrote, sort_keys=True)):
            kept += 1
            print(f"  left: {tbl} {[ref[k] for k in keys]} changed since it was written")
            continue
        if wrote is None and now is not None:
            kept += 1
            print(f"  left: {tbl} {[ref[k] for k in keys]} is there again (a row made since)")
            continue
        if wrote is not None:
            db.execute(f"DELETE FROM {tbl} WHERE {' AND '.join(f'{k} = ?' for k in keys)}", [ref[k] for k in keys])
        if was is not None:
            insert(db, tbl, was)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if kept == 0:
        db.execute(f"DROP TABLE {LOG}")
    audit(db, "npcspawnonly", {}, {"put_back": back, "left": kept}, "undo: " + DETAIL)
    print(f"put back {back} rows, {kept} left (changed since)")
    return 0 if kept == 0 else 1


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    ap.add_argument("--list", action="store_true", help="also list the rows held")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        for t in ("npcs", "gm_audit", "classtemplates", "npctemplate"):
            if not table(db, t):
                print(f"refused, nothing done: no {t} table here")
                return 1
        db.execute("BEGIN IMMEDIATE")
        try:
            if a.undo:
                rc = do_undo(db)
                db.execute("COMMIT")
                return rc
            tchanges, tlines = plan_templates(db)
            dchanges, dlines, left = plan_density(db)
            print("templates:")
            for x in tlines:
                print("  " + x)
            print("density (world 0, the island; 40% kept):")
            for x in dlines:
                print("  " + x)
            already = [x for x in left if x.endswith("held already (npcspawnonly #row)")]
            for x in left:
                if x not in already:
                    print("  left out: " + x)
            if already:
                print(f"  left out: {len(already)} row(s) held already (npcspawnonly #row)")
            if a.list:
                for _, _, _, r in dchanges:
                    print("  hold " + r["name"] + " " + r["note"].split(":")[0])
            changes = tchanges + dchanges
            if not changes:
                print("nothing to do" + (" (applied already)" if table(db, LOG) else ""))
                db.execute("ROLLBACK")
                return 0
            if not a.apply:
                print(f"report only: {len(tchanges)} template row(s), {len(dchanges)} spawn(s) to hold (--apply)")
                db.execute("ROLLBACK")
                return 0
            do_apply(db, changes)
            db.execute("COMMIT")
            print(f"applied: {len(tchanges)} template row(s), {len(dchanges)} spawn(s) held, in gm_audit and {LOG}; "
                  "the GM panel's reload_npcs for the templates, a panel edit of npcspawnonly (or a restart) for the "
                  "spawns")
            return 0
        except BaseException:
            if db.in_transaction:
                db.execute("ROLLBACK")
            raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
