#!/usr/bin/env python3
"""a player's construct as Jarek's Creation, the drachnid's two dye areas, and a Pet Groomer beside every Equipment Dyer
(the owner, 2026-11-16: "lets give [a player] a custom construct for his tinker pet as another option cm only for him. the
drachnid for jarek n toxis lvl 10 delf quest model. also make it have 2 different color areas that can be changed like
armor dye(will be a template for pet color dye). then add Pet Groomer near Equipment Dyer also").

1. The rank (cmcatalog 900917, "Construct (Jarek's Creation)"): Tinker's (category 4, gnomes, every class), a child of
   Construct (1542), rank 4 beside the rat and the clockwork drachnid, level 60, cost 0, needing a Construct in the book
   (require_spells 901835|901836|901837, any of the three looks) and effects "PetLook:360074664,OnlyFor:<his serverid>":
   the body of Jarek's Creation (0x15764DA8, the dark elf torso on the Tiny Spiderbot's legs, the creature the dark elves'
   level 10 quest "Jarek's Creation" sends them to see) on the pet any of those spells summons, its numbers and abilities
   the spell's (entity/cmpetlook.go); offered to a player alone. He buys it at the Master Class window (0 points), as the
   other looks. Only where exactly one a player is a gnome (Heirloom: serverid 747, a level 60 ranger with Tinker and
   Construct); elsewhere (Live: a player is an elf of level 7, who cannot be a Tinker) left out, and said.
2. The drachnid's two areas (petdyeareas 0x15764DA8): area 1 "Torso" (the dark elf: skin, hair, face), area 2 "Spider
   body" (the abdomen and the legs), by the body's palette materials (MATERIALS below, read off the disc with vanilla's
   `eqoa dress 0x15764DA8 out.png petareas=1`). Every shard.
3. The Pet Groomers: an NPC "Pet Groomer" beside each Equipment Dyer (or Armor Dyer) that has none within 6 units: a
   copy of the dyer's row (zone, world, level, type 0x80, gear, facing) 3 units on from the dyer, away from the Heirloom
   Outfitter it stands beside (the line from the outfitter through the dyer, where one is within 8 units: Heirloom's), else
   3 units to the dyer's left (Live has no outfitters); where another NPC stands within 2.5 units of that spot, the
   nearest turn of it round the dyer that is clear (dyers stand in clusters: Freeport's docks); in the other sex's body of the dyer's race where the dyer wears a
   playable race's (a dyer's twin would read as one of two dyers), else the dyer's own. npc_id its own id.

Checked as found, else REFUSED and nothing written: cmcatalog's Tinker Construct (1542) and the three Construct spells
(901835-901837) as the 2026-11-13 rows have them; id 900917 free or this tool's; at least one dyer. Report by default;
--apply in one transaction, a gm_audit line a row (as the panel's), logged in pet_groomers_1116_log; a second --apply finds
nothing to do; --undo deletes exactly what it made (a row changed since is left and listed, the rest still undone).

After --apply: the panel's "Reload every NPC" stands the groomers and reads the areas (a server with the pet-groomer build);
cmcatalog is read at a start or at the panel's cmcatalog edit, so the rank is offered after the build's restart.

    python3 -E -s tools/pet_groomers_20261116.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import math
import sqlite3
import sys

LOG = "pet_groomers_1116_log"
DETAIL = "Zerkziee's construct as Jarek's Creation, the drachnid's two dye areas, the Pet Groomers (the owner, 2026-11-16); tools/pet_groomers_20261116.py"
WHO, VIA = "dab", "tool"
NAME = "Zerkziee"
GNOME = 3
JAREK = 0x15764DA8  # Jarek's Creation (npcs 122241), Deidre IV's body
RANK_ID = 900917
CONSTRUCT_RANK = 1542
CONSTRUCTS = (901835, 901836, 901837)  # Construct, Construct (Drachnid), Construct (Rat)
# area 2 of Jarek's Creation: the spider's abdomen and legs (eqoa dress 0x15764DA8 out.png petareas=1)
MATERIALS = "0"  # material 0, the spider (abdomen, legs); 1 hair, 2 torso, 3 hands, 4 arms, 5 face are area 1
AREA1, AREA2 = "Torso", "Spider body"
GROOMER = "Pet Groomer"
DYERS = ("Equipment Dyer", "Armor Dyer")
OUTFITTER = "Heirloom Outfitter"
STEP = 3.0  # units from the dyer
NEAR_OUTFITTER, NEAR_GROOMER = 8.0, 6.0
CLEAR = 2.5  # no other NPC this near the groomer's spot

# the playable races' bodies, male and female (CLAUDE.md's Model IDs by Race, as npcs.modelid holds them)
BODIES = [(0x70D898C6, 0x0D537335), (0xD8D56FF8, 0xE18919F7), (0x7ED89DAA, 0x8C9B4B39), (0xA99C6B15, 0x4C4084FC),
          (0xAA5B0447, 0x350BFDBE), (0x4C6FA532, 0x4DE32701), (0x61CA3EDF, 0xC44AD516), (0xA3DB3FE2, 0x8A27A731),
          (0x320C0B47, 0x4F1A3CBE), (0x84807490, 0xACC00B4F)]
OTHER_SEX = {a: b for a, b in BODIES} | {b: a for a, b in BODIES}

RANK = {"id": RANK_ID, "category": 4, "classes": 32767, "races": 1 << GNOME, "mastery": "Tinker", "rank": 4,
        "name": "Construct (Jarek's Creation)", "parent": CONSTRUCT_RANK, "cost": 0, "level": 60, "spent": 0, "minstats": "",
        "requires": "Construct@60", "require_spells": "|".join(str(s) for s in CONSTRUCTS), "require_ranks": "", "require_quests": "",
        "adds": "", "add_spells": "", "removes": "", "remove_spells": "",
        "description": "Your clockwork construct takes the form of Jarek's Creation, the drachnid of Jarek N'Toxis's experiments by the "
                       "River Saren. It fights as it always has: only its look changes. Remove this mastery to have your construct's own form back.",
        "enabled": 1, "source": "tools/pet_groomers_20261116.py"}

PETDYEAREAS_DDL = """CREATE TABLE IF NOT EXISTS petdyeareas (
		look      INTEGER PRIMARY KEY,
		area1     TEXT NOT NULL DEFAULT 'Body',
		area2     TEXT NOT NULL DEFAULT '',
		materials TEXT NOT NULL DEFAULT '',
		note      TEXT NOT NULL DEFAULT ''
	)"""

LOG_DDL = f"""CREATE TABLE {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    rowkey TEXT NOT NULL,      -- the key (JSON)
    before TEXT,               -- the row before (JSON), NULL none
    after  TEXT NOT NULL,      -- the row written (JSON)
    audit  INTEGER NOT NULL
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def row(db, tbl, key):
    where = " AND ".join(f"{k} = ?" for k in key)
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", tuple(key.values()))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return None if m is None else json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, tbl, key, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key), go_json(before), go_json(after), DETAIL))
    return cur.lastrowid


def write(db, tbl, key, want):
    """Make tbl's row of key what want says (a whole row; None deletes it), audited. An npcs row with no id is inserted
    and keyed by the id it gets (npc_id made its own). (before, after, audit id, key)."""
    if tbl == "npcs" and key is None:
        cols = [c for c in want if c != "id"]
        cur = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) VALUES ({', '.join('?' for _ in cols)})", tuple(want[c] for c in cols))
        key = {"id": cur.lastrowid}
        db.execute("UPDATE npcs SET npc_id = id WHERE id = ?", (cur.lastrowid,))
        after = row(db, tbl, key)
        return None, after, audit(db, "insert", tbl, key, None, after), key
    before = row(db, tbl, key)
    where = " AND ".join(f"{k} = ?" for k in key)
    if want is None:
        db.execute(f"DELETE FROM {tbl} WHERE {where}", tuple(key.values()))
        action = "delete"
    elif before is None:
        cols = list(want)
        db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' for _ in cols)})", tuple(want[c] for c in cols))
        action = "insert"
    else:
        sets = [c for c in want if c not in key]
        db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in sets)} WHERE {where}", tuple(want[c] for c in sets) + tuple(key.values()))
        action = "update"
    after = row(db, tbl, key)
    return before, after, audit(db, action, tbl, key, before, after), key


def groomer_spot(dyer, outfitter, taken=lambda x, z: False):
    """(x, z) STEP units from the dyer: away from the outfitter, else to its left (facing 0-255, 0 north (-z)); where
    that spot is taken (another NPC within CLEAR), the nearest turn of it round the dyer, 30 degrees a step either way,
    that is free (then 1.5 STEP out); the first spot when none is."""
    x, z = dyer["x_coord"], dyer["z_coord"]
    base = None
    if outfitter is not None:
        dx, dz = x - outfitter["x_coord"], z - outfitter["z_coord"]
        if math.hypot(dx, dz) > 0.01:
            base = math.atan2(dz, dx)
    if base is None:
        a = (dyer["facing"] or 0) / 256.0 * 2 * math.pi
        fx, fz = math.sin(a), -math.cos(a)  # the way it faces
        base = math.atan2(fx, -fz)  # a quarter turn from it
    first = None
    for r in (STEP, STEP * 1.5):
        for k in (0, 1, -1, 2, -2, 3, -3, 4, -4, 5, -5, 6):
            a = base + k * math.pi / 6
            spot = (x + math.cos(a) * r, z + math.sin(a) * r)
            first = first or spot
            if not taken(*spot):
                return spot
    return first


def plan(db):
    """(writes: [(tbl, key or None, want row, what)], lines, refusals)."""
    refusals, lines, writes = [], [], []
    for t in ("characters", "cmcatalog", "charcms", "spellpattern", "npcs", "gm_audit"):
        if not table(db, t):
            refusals.append(f"no {t} table here")
    if refusals:
        return writes, lines, refusals
    c = row(db, "cmcatalog", {"id": CONSTRUCT_RANK})
    if c is None or c["name"] != "Construct" or c["mastery"] != "Tinker":
        refusals.append(f"cmcatalog {CONSTRUCT_RANK} is not Tinker's Construct: {c and (c['mastery'], c['name'])}")
    for s in CONSTRUCTS:
        r = db.execute("SELECT spellname FROM spellpattern WHERE spellid = ?", (s,)).fetchone()
        if r is None or not r[0].startswith("Construct"):
            refusals.append(f"spellpattern {s} is not a Construct: {r}")
    have = row(db, "cmcatalog", {"id": RANK_ID})
    if have is not None and have["source"] != RANK["source"]:
        refusals.append(f"cmcatalog {RANK_ID} is taken: {have['name']!r} ({have['source']})")
    if refusals:
        return writes, lines, refusals

    # 1. the rank
    chars = db.execute("SELECT serverid, race, tclass, level FROM characters WHERE charName = ?", (NAME,)).fetchall()
    if len(chars) == 1 and chars[0][1] == GNOME:
        sid, race, cls, level = chars[0]
        want = dict(RANK, effects=f"PetLook:{JAREK},OnlyFor:{sid}")
        owns = {r[0] for r in db.execute("SELECT cmid FROM charcms WHERE serverid = ? AND cmid IN (1541, 1542)", (sid,))}
        lines.append(f"{NAME}: serverid {sid}, a gnome of class {cls}, level {level}; owns Tinker {1541 in owns}, Construct {1542 in owns}")
        if have == want:
            lines.append(f"  cmcatalog {RANK_ID}: already his {RANK['name']!r}")
        else:
            lines.append(f"  cmcatalog {RANK_ID}: {RANK['name']!r}, Tinker rank 4 under Construct, cost 0, level 60, "
                         f"needs a Construct ({RANK['require_spells']}), effects {want['effects']} (offered to him alone)")
            writes.append(("cmcatalog", {"id": RANK_ID}, want, "the rank"))
    else:
        what = ", ".join(f"serverid {s} race {r} level {lv}" for s, r, _, lv in chars) or "none"
        lines.append(f"{NAME}: {len(chars)} here ({what}), no gnome: the rank is left out on this shard")

    # 2. the drachnid's areas
    db.execute(PETDYEAREAS_DDL)
    want = {"look": JAREK, "area1": AREA1, "area2": AREA2, "materials": MATERIALS,
            "note": "Jarek's Creation (Zerkziee's construct): the torso, the spider's body and legs; the owner 2026-11-16"}
    if row(db, "petdyeareas", {"look": JAREK}) == want:
        lines.append(f"petdyeareas 0x{JAREK:08X}: already {AREA1} / {AREA2}")
    else:
        lines.append(f"petdyeareas 0x{JAREK:08X}: area 1 {AREA1!r}, area 2 {AREA2!r} (materials {MATERIALS})")
        writes.append(("petdyeareas", {"look": JAREK}, want, "the areas"))

    # 3. the groomers
    cur = db.execute(f"SELECT * FROM npcs WHERE npc_name IN ({', '.join('?' for _ in DYERS)}) ORDER BY id", DYERS)
    cols = [d[0] for d in cur.description]
    dyers = [dict(zip(cols, r)) for r in cur.fetchall()]
    if not dyers:
        return writes, lines, refusals + ["no Equipment Dyer here"]
    cur = db.execute("SELECT * FROM npcs WHERE npc_name IN (?, ?)", (OUTFITTER, GROOMER))
    others = [dict(zip(cols, r)) for r in cur.fetchall()]

    def near(d, name, most):
        best = None
        for o in others:
            if o["npc_name"] != name or o["world"] != d["world"]:
                continue
            dist = math.hypot(o["x_coord"] - d["x_coord"], o["z_coord"] - d["z_coord"])
            if dist <= most and (best is None or dist < best[0]):
                best = (dist, o)
        return best and best[1]

    made = beside = 0
    for d in dyers:
        if near(d, GROOMER, NEAR_GROOMER) is not None:
            continue
        out = near(d, OUTFITTER, NEAR_OUTFITTER)
        around = [(n[0], n[1]) for n in db.execute("SELECT x_coord, z_coord FROM npcs WHERE world = ? AND x_coord BETWEEN ? AND ? "
                                                  "AND z_coord BETWEEN ? AND ?", (d["world"], d["x_coord"] - 10, d["x_coord"] + 10,
                                                                                  d["z_coord"] - 10, d["z_coord"] + 10))]
        around += [(o["x_coord"], o["z_coord"]) for o in others if o["npc_name"] == GROOMER and o["world"] == d["world"]]
        x, z = groomer_spot(d, out, lambda x, z: any(math.hypot(x - ax, z - az) < CLEAR for ax, az in around))
        g = dict(d)
        g.pop("id")
        g["npc_name"] = GROOMER
        g["x_coord"], g["z_coord"] = round(x, 2), round(z, 2)
        body = d["modelid"] & 0xFFFFFFFF
        if body in OTHER_SEX:
            g["modelid"] = OTHER_SEX[body] - (1 << 32) if OTHER_SEX[body] >= 1 << 31 and d["modelid"] < 0 else OTHER_SEX[body]
        writes.append(("npcs", None, g, f"a groomer beside dyer {d['id']}"))
        made += 1
        beside += out is not None
        others.append(dict(g, id=0))
        if made <= 3:
            lines.append(f"  groomer beside dyer {d['id']} (world {d['world']}, zone {d['zone']}) at ({g['x_coord']}, {d['y_coord']}, "
                         f"{g['z_coord']}), body 0x{g['modelid'] & 0xFFFFFFFF:08X} (the dyer's 0x{body:08X})")
    lines.append(f"Pet Groomers: {made} to stand ({beside} away from an outfitter, {made - beside} to a dyer's left), "
                 f"{len(dyers) - made} dyers have one")
    return writes, lines, refusals


def apply(db, writes):
    db.execute(LOG_DDL)
    for tbl, key, want, _ in writes:
        before, after, a, key = write(db, tbl, key, want)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                   (tbl, go_json(key), go_json(before), go_json(after), a))
    return len(writes)


def undo(db):
    """(back, left: [why])."""
    if not table(db, LOG):
        return 0, []
    back, left = 0, []
    for tbl, rowkey, before, after in db.execute(f"SELECT tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key = json.loads(rowkey)
        if go_json(row(db, tbl, key)) != after:
            left.append(f"{tbl} {rowkey}: changed since, left as it is")
            continue
        if tbl == "cmcatalog" and before is None and \
                db.execute("SELECT count(*) FROM charcms WHERE cmid = ?", (key["id"],)).fetchone()[0]:
            left.append(f"cmcatalog {key['id']}: owned by a character (charcms), left; remove it from them first")
            continue
        write(db, tbl, key, json.loads(before) if before else None)
        back += 1
    if not left:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"undone: {back} rows back" + "".join(f"\n  left: {w}" for w in left) +
                  ("; press the GM panel's Reload every NPC (the rank leaves at the next start)" if back else ""))
            return 0 if not left else 1
        writes, lines, refusals = plan(db)
        for line in lines:
            print(line)
        if refusals:
            db.execute("ROLLBACK")
            print("REFUSED, nothing written:\n  " + "\n  ".join(refusals))
            return 1
        if not writes:
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if table(db, LOG):
            db.execute("ROLLBACK")
            print(f"applied before and changed since ({LOG} is there): --undo first")
            return 1
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"(report only: {len(writes)} rows; --apply to write)")
            return 0
        n = apply(db, writes)
        db.execute("COMMIT")
        print(f"applied: {n} rows, in gm_audit and {LOG}; press the GM panel's Reload every NPC for the groomers and the areas; "
              "the rank is read at the next start")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
