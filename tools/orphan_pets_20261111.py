#!/usr/bin/env python3
"""Creatures' pets with no master (2026-11-11; the owner: "there are many pets not connected to their master"), on the
server's code of the same branch (entity/companion.go: a pet walks with its owner, a servant's name finds its owner by
a word of it, Blood Thirsty Minions and the village pet are no one's pets).

The live log counted 238 NPCs named as pets, 178 placed by a master. With this branch's code 217 are named as pets and
182 are paired; the 35 left are the rows below, each checked by hand against every creature near it (Heirloom's rows
of 2026-11-11 06:22 UTC):

  ATTACH (5, --apply): a Companion Spirit whose caster stands near with no pet of its own, put on its spawn 1.5 off
    the caster (toward where it stood; the caster's height and zone), so the companion rule pairs them and it walks
    with it. Sandstorm's census and Return Home's rows caught these spirits where they had followed a wandering
    caster: hobgoblin oracle 6.2 off, gnoll mystic 21.9, merkan shamans 41-79 (the shamans wander, census).

  REMOVE (30, --apply --remove: the owner's yes first): pets with no caster to belong to anywhere near (every caster
    of the kind within 80 has its own pet already, or there is none):
    - 11 Companion Spirits of the goblins' camps (Return Home's rows): 9 by the hill goblins' camp of brigands and
      gladiators, whose 20 witch doctors each have their spirit already (26 to 112 off the nearest), 2 by King
      Merdid's goblins, whose one witchdoctor has his; and 1 (level 47) among the Bouncers of world 1, no caster near;
    - 10 undead minions standing together in world 0, nothing within 140 of them (the only rows of the name);
    - 3 glyphed familiars among the drachnids (a parasite 27-92 off, all with or without one; a Siloth necromancer 60);
    - a decaying minion among skeletal servants (no caster near), a thrall (a player race's body, level 15, among
      level 41-45 terrors), the Arch Familiar of world 2 by Lover's Fear (the other of the name paired).
    A removal is a spawn removal: the row is deleted (no other table names these rows: no path, camp, respawn, quest).

Refused, nothing written, when a row is not as found (its name, world or place) or a caster has a pet beside it now.
Report only by default; --apply in one transaction, a gm_audit line a row, log orphan_pets_1111_log; a second --apply
does only what is not done yet (--remove after an --apply removes); --undo exact (a row changed since left and said).
NPCs are read at the start: after a restart onto this code it is live.

    tools/orphan_pets_20261111.py --db eqoa.db [--apply [--remove] | --undo]
"""
import argparse
import datetime
import json
import math
import sqlite3
import sys

LOG = "orphan_pets_1111_log"
WHO, VIA = "dab", "tool"
DETAIL = "creatures' pets with no master (the owner, 2026-11-11): tools/orphan_pets_20261111.py"
BESIDE = 1.5  # how far off its caster a pet is put (the paired spirits: half within 1.7)
NEAR = 4.0    # entity/companion.go's companionReach: a pet this near a caster is its
FOUND = 0.05  # how far a row may be from where it was found

# (pet row, name, world, x, z), (caster row, name, world, x, z)
ATTACH = [
    ((124606, "Companion Spirit", 1, 7434.69, 5542.55), (124600, "hobgoblin oracle", 1, 7428.66, 5543.98)),
    ((986, "Companion Spirit", 0, 13503.1, 14625.5), (1438, "gnoll mystic", 0, 13507.9, 14646.9)),
    ((134040, "Companion Spirit", 2, 6236.9, 11012.94), (110210, "merkan shaman", 2, 6264.04, 10982.1)),
    ((134038, "Companion Spirit", 2, 6256.5, 11051.6), (110212, "merkan shaman", 2, 6310.27, 11042.1)),
    ((134039, "Companion Spirit", 2, 6228.83, 11009.56), (110211, "merkan shaman", 2, 6305.49, 10991.4)),
]

# (row, name, world, x, z, why)
REMOVE = [
    (105215, "Companion Spirit", 0, 16243.7, 12194.1, "King Merdid's goblins: their witchdoctor (35 off) has his"),
    (105231, "Companion Spirit", 0, 16293.7, 12175.2, "King Merdid's goblins: their witchdoctor (18 off) has his"),
    (108178, "Companion Spirit", 0, 19336.8, 12752.7, "the goblin camp: every witch doctor has his (53 off)"),
    (108201, "Companion Spirit", 0, 19345.8, 12663.5, "the goblin camp: every witch doctor has his (37 off)"),
    (108221, "Companion Spirit", 0, 19391.0, 12348.4, "the goblin camp: every witch doctor has his (112 off)"),
    (108229, "Companion Spirit", 0, 19265.2, 12222.9, "the goblin camp: every witch doctor has his (34 off)"),
    (108236, "Companion Spirit", 0, 19306.4, 12231.6, "the goblin camp: every witch doctor has his (53 off)"),
    (108239, "Companion Spirit", 0, 19324.6, 12294.2, "the goblin camp: every witch doctor has his (74 off)"),
    (108253, "Companion Spirit", 0, 19400.0, 12228.6, "the goblin camp: every witch doctor has his (99 off)"),
    (108277, "Companion Spirit", 0, 19044.2, 12552.4, "the goblin camp: every witch doctor has his (26 off)"),
    (108320, "Companion Spirit", 0, 19351.8, 12263.2, "the goblin camp: every witch doctor has his (92 off)"),
    (108339, "Companion Spirit", 0, 19458.6, 12351.2, "the goblin camp: no witch doctor within 150"),
    (108340, "Companion Spirit", 0, 19503.0, 12337.6, "the goblin camp: no witch doctor within 150"),
    (106304, "Companion Spirit", 1, 8794.25, 7350.49, "among the Bouncers (level 47): no caster within 120"),
    (112437, "undead minion", 0, 19745.5, 29352.8, "ten together, nothing within 140"),
    (112438, "undead minion", 0, 19783.3, 29364.1, "ten together, nothing within 140"),
    (112439, "undead minion", 0, 19773.6, 29362.6, "ten together, nothing within 140"),
    (112440, "undead minion", 0, 19760.9, 29374.1, "ten together, nothing within 140"),
    (112441, "undead minion", 0, 19797.5, 29346.8, "ten together, nothing within 140"),
    (112442, "undead minion", 0, 19738.5, 29379.5, "ten together, nothing within 140"),
    (112443, "undead minion", 0, 19752.7, 29352.7, "ten together, nothing within 140"),
    (112444, "undead minion", 0, 19816.2, 29340.4, "ten together, nothing within 140"),
    (112445, "undead minion", 0, 19758.0, 29334.7, "ten together, nothing within 140"),
    (112446, "undead minion", 0, 19764.3, 29334.4, "ten together, nothing within 140"),
    (106854, "glyphed familiar", 0, 14488.4, 18977.9, "among the drachnids: a Siloth necromancer 60 off, a parasite 27"),
    (106901, "glyphed familiar", 0, 14444.4, 19065.5, "among the drachnids: no caster within 76"),
    (106902, "glyphed familiar", 0, 14431.7, 19066.2, "among the drachnids: no caster within 88"),
    (1115, "decaying minion", 0, 22872.4, 8781.6, "among skeletal servants: no caster near"),
    (102976, "thrall", 0, 7237.17, 10450.1, "a player race's body, level 15, among level 41-45 terrors"),
    (134091, "Arch Familiar", 2, 9088.06, 11378.17, "by Lover's Fear (19.9 off, no caster): the other paired"),
]

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    npc_row INTEGER NOT NULL,
    action  TEXT NOT NULL,     -- move | delete
    before  TEXT NOT NULL,     -- the whole row before (JSON)
    after   TEXT,              -- the whole row as written (JSON), NULL: deleted
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def cols(db):
    return [r[1] for r in db.execute("PRAGMA table_info(npcs)")]


def row(db, rid):
    c = cols(db)
    r = db.execute(f"SELECT {', '.join(c)} FROM npcs WHERE id = ?", (rid,)).fetchone()
    return None if r is None else dict(zip(c, r))


def as_found(r, name, world, x, z):
    return (r is not None and (r["npc_name"] or "").strip() == name and r["world"] == world and
            abs(r["x_coord"] - x) <= FOUND and abs(r["z_coord"] - z) <= FOUND)


def is_pet_name(name):
    """A name entity/companion.go reads as a pet's, roughly (for the caster-has-a-pet check)."""
    n = (name or "").strip().lower()
    return n.endswith((" pet", " minion", " familiar", " thrall", "companion spirit")) or n in ("thrall", "familiar")


def logged(db):
    if not table(db, LOG):
        return {}
    return {r[0]: r[1] for r in db.execute(f"SELECT npc_row, action FROM {LOG}")}


def plan(db, remove):
    """(moves, deletes, lines, refusals): moves (before, after), deletes (before, why), what is not done yet."""
    done = logged(db)
    moves, deletes, lines, refusals = [], [], [], []
    for (pid, pname, pw, px, pz), (cid, cname, cw, cx, cz) in ATTACH:
        if pid in done:
            lines.append(f"done: #{pid} {pname} by #{cid} {cname}")
            continue
        pet, caster = row(db, pid), row(db, cid)
        if not as_found(pet, pname, pw, px, pz):
            refusals.append(f"#{pid} not as found ({pname} w{pw} {px},{pz}): {pet and (pet['npc_name'], pet['world'], pet['x_coord'], pet['z_coord'])}")
            continue
        if not as_found(caster, cname, cw, cx, cz):
            refusals.append(f"caster #{cid} not as found ({cname} w{cw} {cx},{cz}): {caster and (caster['npc_name'], caster['world'], caster['x_coord'], caster['z_coord'])}")
            continue
        near = [r for r in db.execute("SELECT id, npc_name FROM npcs WHERE world = ? AND id != ? AND x_coord BETWEEN ? AND ? "
                                      "AND z_coord BETWEEN ? AND ?", (cw, pid, cx - NEAR, cx + NEAR, cz - NEAR, cz + NEAR))
                if is_pet_name(r[1])]
        if near:
            refusals.append(f"caster #{cid} {cname} has a pet beside it now: {near}")
            continue
        dx, dz = px - cx, pz - cz
        d = math.hypot(dx, dz)
        nx, nz = cx + dx / d * BESIDE, cz + dz / d * BESIDE
        after = dict(pet, x_coord=round(nx, 2), z_coord=round(nz, 2), y_coord=caster["y_coord"], zone=caster["zone"])
        moves.append((pet, after))
        lines.append(f"attach: #{pid} {pname} (w{pw}, {d:.1f} off) -> 1.5 off #{cid} {cname}: ({px}, {pz}) -> ({after['x_coord']}, {after['z_coord']})")
    for rid, name, world, x, z, why in REMOVE:
        if rid in done:
            lines.append(f"done: #{rid} {name} removed")
            continue
        r = row(db, rid)
        if not as_found(r, name, world, x, z):
            refusals.append(f"#{rid} not as found ({name} w{world} {x},{z}): {r and (r['npc_name'], r['world'], r['x_coord'], r['z_coord'])}")
            continue
        if remove:
            deletes.append((r, why))
        lines.append(f"{'remove' if remove else 'to remove (--remove, the owner yes)'}: #{rid} {name} w{world} ({x}, {z}) L{r['npc_level']}: {why}")
    return moves, deletes, lines, refusals


def audit(db, action, rid, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, 'npcs', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, go_json({"id": rid}),
                      None if before is None else go_json(before), None if after is None else go_json(after), DETAIL))
    return cur.lastrowid


def apply(db, moves, deletes):
    db.execute(LOG_SCHEMA)
    for before, after in moves:
        db.execute("UPDATE npcs SET x_coord = ?, y_coord = ?, z_coord = ?, zone = ? WHERE id = ?",
                   (after["x_coord"], after["y_coord"], after["z_coord"], after["zone"], before["id"]))
        a = audit(db, "update", before["id"], before, after)
        db.execute(f"INSERT INTO {LOG} (npc_row, action, before, after, audit) VALUES (?, 'move', ?, ?, ?)",
                   (before["id"], go_json(before), go_json(after), a))
    for before, _ in deletes:
        db.execute("DELETE FROM npcs WHERE id = ?", (before["id"],))
        a = audit(db, "delete", before["id"], before, None)
        db.execute(f"INSERT INTO {LOG} (npc_row, action, before, after, audit) VALUES (?, 'delete', ?, NULL, ?)",
                   (before["id"], go_json(before), a))


def undo(db):
    """(put back, left): each logged row as it was, when it is still as written (a deleted row: its id still free)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    c = cols(db)
    for seq, rid, action, before, after, _ in db.execute(f"SELECT * FROM {LOG} ORDER BY seq DESC").fetchall():
        b = json.loads(before)
        now = row(db, rid)
        if action == "move":
            if now is None or go_json(now) != go_json(json.loads(after)):
                print(f"left: #{rid} changed since: {now}")
                left += 1
                continue
            db.execute("UPDATE npcs SET x_coord = ?, y_coord = ?, z_coord = ?, zone = ? WHERE id = ?",
                       (b["x_coord"], b["y_coord"], b["z_coord"], b["zone"], rid))
            audit(db, "update", rid, now, b)
        else:
            if now is not None:
                print(f"left: #{rid} is taken again: {now['npc_name']}")
                left += 1
                continue
            db.execute(f"INSERT INTO npcs ({', '.join(c)}) VALUES ({', '.join('?' * len(c))})", [b[k] for k in c])
            audit(db, "insert", rid, None, b)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if not left:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    ap.add_argument("--remove", action="store_true", help="also remove the 30 masterless pets (the owner's yes first)")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
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
    moves, deletes, lines, refusals = plan(db, a.remove)
    for line in lines:
        print(line)
    if refusals:
        print("refused, nothing done:")
        for r in refusals:
            print("  " + r)
        return 1
    if not moves and not deletes:
        print("nothing to do")
        return 0
    if not a.apply:
        print(f"report only: {len(moves)} pets attached, {len(deletes)} removed would be written (--apply writes them)")
        return 0
    db.execute("BEGIN IMMEDIATE")
    try:
        apply(db, moves, deletes)
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"applied: {len(moves)} pets attached, {len(deletes)} removed, in gm_audit and {LOG}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
