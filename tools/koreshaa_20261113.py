#!/usr/bin/env python3
"""Lord Koreshaa, a rare boss on the Welahi-nai myrmidons of Kerran Isle (2026-11-13; the owner: "lord koreshaa looks
like a special lion warlord he is a boss when he gets down to 1/3 hp 3500 ar nuke he fires once, cant solo need group
he can wear crown of the jaguar has a heal when hit effect that heals him, like it would a player. is not in our
database he drops crown of the jaguar. wehali myrmidon in kerran isle is a placeholder"), on the server's code of the
same branch: table npcrare (entity/npcrare.go: a placeholder comes back as its rare at its chance, one at a time) and a
nuke held below a share of its creature's hit points (classtemplate.go's templateWantsLocked). An older server reads
neither (it leaves npcrare unread and casts the nuke at once): apply it with the restart that brings the code.

What it writes (every row in gm_audit and its log, koreshaa_1113_log; --undo exact):

  npcs: "Lord Koreshaa" again: Return Home's own row (119831, level 62, race 6, body 0x5D40C8AB: the maned Kerran
    lion in pauldrons, which only Khonza Meysla wears besides) at (5156.75, 63.8, 7012.12) world 2, Kerran Isle, which
    the Sandstorm census of world 2 took out on 2026-10-31 (its log, sandstorm_census_seed_w2, keeps the row): a new
    row, its size 1.3 (was 1.0; the myrmidons 1.1) so he stands out.
  npcspawnonly "#<his row>": his own row out of the world, there to be copied: he stands only on his placeholder.
  npcrare: the Welahi-nai myrmidon on his spot (132354, at (5156.6, 63.8, 7012.4)) comes back as Lord Koreshaa CHANCE
    percent of its returns (10), never two at once; else as itself.
  npcclass "lord koreshaa" 0 is kept (Return Home's warrior). classtemplates "lord koreshaa", npctemplate "lord
    koreshaa" pointed at it (it named the roamer): hit points for 150,000 at 62 (a group's named: the PoD named of 61
    120k, Sothimus at 63 150k), melee 750 (about 600 a hit on a capped L60 warrior, by the PoD named's 500 at 623), a
    Critical Hit of 1,500 every 25-35 s from 10-20 s in (the roamer's clock); "Fury of the Pride", a fixed 3,500
    arcane (AR) nuke at his target once his hit points are below 34%, once a fight (every 3600 s); a heal of 15 at
    every hit that lands on him (hitheal: the Crown of the Jaguar's 15 a hit on its wearer, entity/wornshield.go
    "crown of the jaguar": {Damage: 30, Heal: 15}).
  npcloot "lord koreshaa" -> loottables "boss lord koreshaa": the Crown of the Jaguar (6248, the base record; 990058 is
    the import damper's copy, 990957 a transfer copy), 100%, one roll. His region's trash (lootcreature "odus", two
    lootpool rows of Return Home's) stays as it was.

Rows are found by key and their present values checked; anything not as the 2026-11-13 Heirloom data had it refuses
the whole run. Report only by default; --apply in one transaction; --undo exact (a row changed since is left, said).

    tools/koreshaa_20261113.py --db eqoa.db [--chance 10] [--apply | --undo]
"""
import argparse
import datetime
import json
import math
import sqlite3
import sys

LOG = "koreshaa_1113_log"
WHO = "koreshaa"
VIA = "tool"
SRC = "koreshaa1113"
DETAIL = "Lord Koreshaa, a rare on Kerran Isle's Welahi-nai myrmidons (the owner, 2026-11-13): tools/koreshaa_20261113.py"

NAME = "Lord Koreshaa"
KEY = "lord koreshaa"
PH_NAME = "welahi-nai myrmidon"
PH_ROW = 132354  # the myrmidon on his spot (5156.6, 63.8, 7012.4), Kerran Isle
OLD_ROW = 119831  # Return Home's Lord Koreshaa, which the Sandstorm census of world 2 took out (its log keeps the row)
BODY = 0x5D40C8AB  # his record's: the maned Kerran in pauldrons (Khonza Meysla wears it too)
SIZE = 1.3
LEVEL = 62
CROWN = 6248
TABLE = "boss lord koreshaa"
HP = 150_000
MELEE = 750
CRIT = 1500
ABILITIES = [
    {"kind": "nuke", "name": "Fury of the Pride", "damage": 3500, "resist": "AR", "below": 34, "every": 3600},
    {"kind": "hitheal", "heal": 15},
]

KEYS = {"npcs": ("id",), "npcspawnonly": ("name",), "npcrare": ("npc_row",), "npcclass": ("name",),
        "classtemplates": ("name",), "npctemplate": ("name",), "npcloot": ("npc_name", "tablename"),
        "loottables": ("id",)}
SEQS = ("npcs", "loottables")

# What this branch's server makes at its start (internal/database/npcrare.go), made here first when it is not there;
# --undo drops it when it made it and it is empty again.
NPCRARE_SCHEMA = """CREATE TABLE IF NOT EXISTS npcrare (
		npc_row INTEGER PRIMARY KEY,
		rare    TEXT NOT NULL,
		chance  REAL NOT NULL DEFAULT 10,
		note    TEXT NOT NULL DEFAULT '',
		source  TEXT NOT NULL DEFAULT 'gm'
	)"""


def rnd(x):
    """Go's math.Round: halves away from zero."""
    return int(math.floor(x + 0.5)) if x >= 0 else -int(math.floor(-x + 0.5))


def table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def row_of(db, tbl, key):
    where = " AND ".join(f"{c} = ?" for c in KEYS[tbl])
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", tuple(key[c] for c in KEYS[tbl]))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def key_of(tbl, row):
    return {c: row[c] for c in KEYS[tbl]}


def go_json(m):
    """A row as Go's json.Marshal writes a map (keys sorted, <>& escaped), as the panel's audit has it."""
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def between(rows, level):
    """balance.go's Level: the row of the level, the last past the end, the first before it, else the line between."""
    rows = sorted(rows)
    if not rows:
        return 100.0
    for i, (lv, v) in enumerate(rows):
        if lv >= level:
            if lv == level or i == 0:
                return v
            plv, pv = rows[i - 1]
            return pv + (v - pv) * (level - plv) / (lv - plv)
    return rows[-1][1]


def level_hp(db, level):
    """An NPC's hit points at its level's balance before its template (level x 60 at balancelevel's hp_pct)."""
    base = max(level * 60, 10)
    bal = between(list(db.execute("SELECT level, hp_pct FROM balancelevel")) if table(db, "balancelevel") else [], level)
    return max(rnd(base * bal / 100), 1) if bal != 100 else base


def max_hp(db, level, hp_pct):
    return max(1, rnd(level_hp(db, level) * hp_pct / 100))


def old_record(db):
    """Return Home's Lord Koreshaa as the census's log keeps it (sandstorm_census_seed_w2), or None."""
    if not table(db, "sandstorm_census_seed_w2"):
        return None
    r = db.execute("SELECT old FROM sandstorm_census_seed_w2 WHERE rid = ? AND name = ?", (OLD_ROW, NAME)).fetchone()
    return None if r is None else json.loads(r[0])


def plan(db, chance):
    """(changes, lines, refusals). A change is (tbl, before row or None, after row); an npcs insert's id is "$K" in the
    rows after it."""
    changes, lines, refusals = [], [], []
    if db.execute("SELECT count(*) FROM npcs WHERE lower(trim(npc_name)) = ?", (KEY,)).fetchone()[0]:
        refusals.append(f"an NPC called {NAME!r} is there already")
    old = old_record(db)
    if old is None or old.get("modelid") != BODY or old.get("world") != 2:
        refusals.append(f"no census log row of Return Home's {NAME} ({OLD_ROW}) with body 0x{BODY:08X} in world 2: {old}")
    ph = row_of(db, "npcs", {"id": PH_ROW})
    if ph is None or ph["npc_name"].strip().lower() != PH_NAME or ph["world"] != 2 or \
            (old and math.hypot(ph["x_coord"] - old["x_coord"], ph["z_coord"] - old["z_coord"]) > 2):
        refusals.append(f"npcs {PH_ROW} is not the Welahi-nai myrmidon on his spot: {ph}")
    crown = db.execute("SELECT itemname FROM itempattern WHERE patternid = ?", (CROWN,)).fetchone()
    if crown is None or crown[0] != "Crown of the Jaguar":
        refusals.append(f"itempattern {CROWN} is not the Crown of the Jaguar: {crown}")
    cls = row_of(db, "npcclass", {"name": KEY})
    if cls is not None and cls["class"] != 0:
        refusals.append(f"npcclass {KEY!r} is {cls['class']}, the 2026-11-13 data had 0 (a warrior)")
    link = row_of(db, "npctemplate", {"name": KEY})
    if link is not None and link["template"] != "roamer":
        refusals.append(f"npctemplate {KEY!r} names {link['template']!r}, the 2026-11-13 data had 'roamer'")
    for tbl, k in (("classtemplates", {"name": KEY}), ("npcloot", {"npc_name": KEY, "tablename": TABLE})):
        if row_of(db, tbl, k) is not None:
            refusals.append(f"{tbl} {k} is there already")
    if db.execute("SELECT count(*) FROM loottables WHERE tablename = ?", (TABLE,)).fetchone()[0]:
        refusals.append(f"loottables {TABLE!r} has rows already")
    if table(db, "npcrare") and row_of(db, "npcrare", {"npc_row": PH_ROW}) is not None:
        refusals.append(f"npcrare {PH_ROW} is there already")
    if refusals:
        return changes, lines, refusals

    npc = {c: v for c, v in old.items() if c != "id"}
    npc.update(size=SIZE, npc_level=LEVEL, npc_id=0)
    npc["_token"] = "K"
    changes.append(("npcs", None, npc))
    lines.append(f"npcs: {NAME!r}, Return Home's row {OLD_ROW} again (the census took it out) at ({old['x_coord']:.1f}, "
                 f"{old['y_coord']:.1f}, {old['z_coord']:.1f}) world 2, its body 0x{BODY:08X}, size {SIZE} (was "
                 f"{old['size']}), level {LEVEL}")
    changes.append(("npcspawnonly", None, {"name": "$K", "note": "Lord Koreshaa's own row: he stands only on his "
                                           "placeholder, a Welahi-nai myrmidon of Kerran Isle (npcrare)", "source": SRC}))
    lines.append("npcspawnonly: his own row out of the world (copied by the placeholder)")
    changes.append(("npcrare", None, {"npc_row": PH_ROW, "rare": NAME, "chance": float(chance),
                                      "note": "the owner, 2026-11-13: \"wehali myrmidon in kerran isle is a placeholder\": "
                                              "the myrmidon on Return Home's Lord Koreshaa's spot", "source": SRC}))
    lines.append(f"npcrare: placeholder npcs {PH_ROW} ({ph['x_coord']:.1f}, {ph['z_coord']:.1f}) at {chance}% of its returns")
    if cls is None:
        changes.append(("npcclass", None, {"name": KEY, "class": 0, "source": SRC}))
    hp_pct = rnd(HP * 100 / level_hp(db, LEVEL))
    crit = CRIT / LEVEL
    tmpl = {"name": KEY, "class": 0, "hp_pct": hp_pct, "taken_pct": 100, "melee_pct": MELEE, "special": "Critical Hit",
            "special_chance": 0, "special_damage": crit, "special_fx": 0, "special_every": 30.0, "special_first": 15.0,
            "special_jitter": 5.0, "special_from": 10,
            "abilities": json.dumps(ABILITIES, separators=(",", ":"), ensure_ascii=False),
            "note": (f"Lord Koreshaa, L{LEVEL}, a rare on a Welahi-nai myrmidon of Kerran Isle (the owner, 2026-11-13: 'cant "
                     f"solo need group', '1/3 hp 3500 ar nuke he fires once', 'a heal when hit effect that heals him, like it "
                     f"would a player'): {max_hp(db, LEVEL, hp_pct):,} hit points (ours: a group's named, as the PoD named of "
                     f"61's 120k and Sothimus's 150k), melee {MELEE} (~600 a hit on a capped L60 warrior, ours), a {CRIT:,} "
                     f"Critical Hit every 25-35 s (ours); Fury of the Pride, 3,500 AR at his target below 34%, once a fight; "
                     f"15 healed at every hit on him (the Crown of the Jaguar's 15, wornshield.go). tools/koreshaa_20261113.py"),
            "source": SRC}
    changes.append(("classtemplates", None, tmpl))
    if link is None:
        changes.append(("npctemplate", None, {"name": KEY, "template": KEY, "note": "a rare boss, level 62", "source": SRC}))
    else:
        changes.append(("npctemplate", link, dict(link, template=KEY, note="a rare boss, level 62 (was the roamer's)")))
    lines.append(f"classtemplates {KEY!r} (npctemplate: {'roamer -> ' if link else ''}{KEY!r}): hp_pct {hp_pct} "
                 f"({max_hp(db, LEVEL, hp_pct):,} hit points at {LEVEL}), melee {MELEE}, Critical Hit {rnd(LEVEL * crit):,} "
                 f"every 25-35 s; abilities {tmpl['abilities']}")
    changes.append(("npcloot", None, {"npc_name": KEY, "tablename": TABLE, "chance": 100, "rolls": 1,
                                      "note": "the owner, 2026-11-13: \"he drops crown of the jaguar\"", "source": SRC}))
    changes.append(("loottables", None, {"tablename": TABLE, "patternid": CROWN, "weight": 1, "minlevel": 1, "maxlevel": 70,
                                         "source": SRC}))
    lines.append(f"npcloot {KEY!r} -> {TABLE!r}: the Crown of the Jaguar ({CROWN}) at 100%, one roll")
    return changes, lines, refusals


LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,
    rowkey  TEXT NOT NULL,      -- the row's key (JSON)
    before  TEXT,               -- the row before (JSON), NULL: made here
    after   TEXT,               -- the row as written (JSON)
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def audit(db, action, tbl, key, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key),
                      None if before is None else go_json(before), None if after is None else go_json(after), detail))
    return cur.lastrowid


def seqs(db):
    return {t: (lambda r: None if r is None else r[0])(db.execute("SELECT seq FROM sqlite_sequence WHERE name = ?", (t,)).fetchone())
            for t in SEQS}


def apply(db, changes):
    """The plan written, in the caller's transaction: each row and its gm_audit line, its own line in the log. Returns
    (rows, the new NPC's id)."""
    made = not table(db, "npcrare")
    db.execute(NPCRARE_SCHEMA)
    db.execute(LOG_SCHEMA)
    db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES ('schema', '{{}}', NULL, ?)",
               (json.dumps(["table npcrare"] if made else []),))
    before_seq = seqs(db)
    ids = {}
    n = 0
    for tbl, before, after in changes:
        after = dict(after)
        tok = after.pop("_token", None)
        for c, v in list(after.items()):
            if isinstance(v, str) and v.startswith("$") and v[1:] in ids:
                after[c] = "#%d" % ids[v[1:]]
        if before is not None:
            key = key_of(tbl, before)
            sets = [c for c in after if after[c] != before.get(c)]
            db.execute(f"UPDATE {tbl} SET {', '.join(c + ' = ?' for c in sets)} WHERE " + " AND ".join(f"{c} = ?" for c in KEYS[tbl]),
                       [after[c] for c in sets] + [key[c] for c in KEYS[tbl]])
            row = row_of(db, tbl, key)
            a = audit(db, "update", tbl, key, before, row, DETAIL)
        else:
            cols = list(after)
            cur = db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [after[c] for c in cols])
            if tbl == "npcs":
                db.execute("UPDATE npcs SET npc_id = ? WHERE id = ?", (cur.lastrowid, cur.lastrowid))
                ids[tok] = cur.lastrowid
                key = {"id": cur.lastrowid}
            elif tbl == "loottables":
                key = {"id": cur.lastrowid}
            else:
                key = key_of(tbl, after)
            row = row_of(db, tbl, key)
            a = audit(db, "insert", tbl, key, None, row, DETAIL)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                   (tbl, json.dumps(key), None if before is None else json.dumps(before, sort_keys=True, default=str),
                    json.dumps(row, sort_keys=True, default=str), a))
        n += 1
    db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES ('sqlite_sequence', '{{}}', ?, ?)",
               (json.dumps(before_seq), json.dumps(seqs(db))))
    return n, ids.get("K")


def undo(db):
    """Each logged row still as written put back as it was (an insert taken out), the last first, its gm_audit line; then the ids the new rows took,
    when no other row took one since, and table npcrare when this tool made it and it is empty. Returns (back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    seq_row = db.execute(f"SELECT before, after FROM {LOG} WHERE tbl = 'sqlite_sequence'").fetchone()
    for seq, tbl, key, before, after in db.execute(
            f"SELECT seq, tbl, rowkey, before, after FROM {LOG} WHERE tbl NOT IN ('sqlite_sequence', 'schema') ORDER BY seq DESC").fetchall():
        key = json.loads(key)
        now = row_of(db, tbl, key)
        if now is None or json.dumps(now, sort_keys=True, default=str) != after:
            left += 1
            print(f"  left: {tbl} {key}, changed since it was written")
            continue
        where = " AND ".join(f"{c} = ?" for c in KEYS[tbl])
        if before is None:
            db.execute(f"DELETE FROM {tbl} WHERE {where}", [key[c] for c in KEYS[tbl]])
            audit(db, "delete", tbl, key, now, None, "undo: " + DETAIL)
        else:
            b = json.loads(before)
            db.execute(f"UPDATE {tbl} SET {', '.join(c + ' = ?' for c in b)} WHERE {where}", [b[c] for c in b] + [key[c] for c in KEYS[tbl]])
            audit(db, "update", tbl, key, now, row_of(db, tbl, key), "undo: " + DETAIL)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if left == 0:
        if seq_row is not None:
            was, written = json.loads(seq_row[0]), json.loads(seq_row[1])
            cur = seqs(db)
            for t in SEQS:
                if was.get(t) is not None and cur.get(t) == written.get(t):
                    db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = ?", (was[t], t))
        schema = db.execute(f"SELECT after FROM {LOG} WHERE tbl = 'schema'").fetchone()
        if schema and "table npcrare" in json.loads(schema[0]):
            if db.execute("SELECT count(*) FROM npcrare").fetchone()[0]:
                print("  table npcrare kept: it has rows of another's now")
            else:
                db.execute("DROP TABLE npcrare")
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    ap.add_argument("--chance", type=float, default=10.0, help="percent of the placeholder's returns as Koreshaa (default 10)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    if not 0 < a.chance <= 100:
        ap.error("--chance: above 0 and at most 100")
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        if a.undo:
            db.execute("BEGIN IMMEDIATE")
            try:
                back, left = undo(db)
                db.execute("COMMIT")
            except BaseException:
                db.execute("ROLLBACK")
                raise
            print(f"put back {back} rows, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        if table(db, LOG):
            print(f"applied already ({LOG} is there): --undo first")
            return 1
        changes, lines, refusals = plan(db, a.chance)
        for line in lines:
            print(line)
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not a.apply:
            print(f"report only: {len(changes)} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            n, kid = apply(db, changes)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {n} rows, each in gm_audit and {LOG}; Lord Koreshaa's row {kid}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
