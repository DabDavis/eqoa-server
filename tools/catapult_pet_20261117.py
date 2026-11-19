#!/usr/bin/env python3
"""a player's construct as a catapult (the owner, 2026-11-17, of a player's petition 2131: "where [a GM] is is a
catapult prop. we have templates for props as mobs now. lets give it to [a player]. it can ram for attack and we can
make the catapult fire").

The catapult is Tunaria's siege engine prop 0xE64B8D78 (zone 108: three of them by where a GM stood, at
21228.7, 19147.5; Rathe's zones 13 and 17 hold it too), a zone's sprite no character file has. So it is a pet's ART
(server branch catapult-pet: a rank's PetArt, decorart rows of kind pet, entity/cmpetlook.go and decorlights.go): a
client that draws it (vanilla answering the decorations at layout 3) is sent the construct in the catapult, in a world
whose decorart pet row names the zone that holds it; every other client (a PS2) sees the construct's body as it is
(its spell's, or a PetLook rank's: Jarek's Creation or the giant spider). It fights as it always has: vanilla rams it
forward at each swing, and at each of its casts (Tinker Kick, every 12 s) fires a boulder (0xBFEE4CCD, the rock
beside the catapults in the same zones) from its arm to the target.

Writes (Heirloom only: a player is serverid 747 there, a level 60 gnome with Tinker and Construct; on Live 747 is
someone else, so it REFUSES unless 747 is a player with rank 1542):
  1. cmcatalog "Construct (Catapult)" at the next free id from 900919 (after 900918, the giant spider): Tinker,
     category 4, gnomes, rank 4 under Construct (1542), level 60, cost 0, requires Construct@60 /
     901835|901836|901837, effects PetArt:3863711096,OnlyFor:747 (a look-only rank, as 900917).
  2. decorart, kind pet: "Catapult (pet look, Tunaria)" world 0 zone 108 and "Catapult (pet look, Rathe)" world 1
     zone 13, the art 0xE64B8D78 at SCALE of the pet's size, the shot 0xBFEE4CCD at SHOT_SCALE. The name is a label
     (a pet row is never matched to an NPC's name). The kind/shot/shot_scale/scale columns are added when missing (as
     the server's database.ensureDecorArtSink does; --undo leaves them).
  3. charcms (747, the rank, cost 0): given to him (--no-grant leaves him to take it in the Master Class window, free).
     Of his look ranks the highest id's wins, so the catapult is his construct's look on vanilla until he removes the
     rank; a PS2 still shows Construct (Giant Spider) (900918's PetLook).

Checked as found, else REFUSED and nothing written: a player at 747 with ranks 1541 and 1542; cmcatalog 1542 and the
Construct spells' petstats rows; no other "Construct (Catapult)" row; no decorart rows of these names that differ.
A report by default; --apply in one transaction, a gm_audit line a row, what was there kept in --applied (JSON);
--undo puts back exactly that, refused if a row has changed since (all or nothing).

Live: decorart, cmcatalog and charcms are read at the server's start; so apply it just before the swap that deploys
catapult-pet, and the new process has it all (a later apply: re-save one of the tool's rows in the GM panel's
Decoration art, Mastery ranks and character ranks tables, each reloads its own, or restart). A grant is in his ranks at
his next login (the panel's charcms edit gives it at once). The pet takes the art at its next summoning. Without the
server and vanilla of catapult-pet (layout 3) the rows do nothing (an older server reads the pet rows as decorations
of NPCs no one is named: vanilla reads zone 108 in Tunaria, nothing more).

    python3 -E -s tools/catapult_pet_20261117.py --db eqoa.db --applied FILE [--no-grant] [--apply | --undo]
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

DETAIL = "Zerkziee's construct as Tunaria's catapult prop (the owner, 2026-11-17, petition 2131): tools/catapult_pet_20261117.py"
WHO, VIA = "claude", "tool"
SRC = "catapult-pet-1117"

ZERK, ZERK_NAME = 747, "Zerkziee"
TINKER, CONSTRUCT = 1541, 1542
SPELLS = (901835, 901836, 901837)
NAME = "Construct (Catapult)"
IDS = (900919, 900999)
CATAPULT, BOULDER = 0xE64B8D78, 0xBFEE4CCD
SCALE, SHOT_SCALE = 0.35, 0.3
ART = [("Catapult (pet look, Tunaria)", 0, 108), ("Catapult (pet look, Rathe)", 1, 13)]
DESC = ("Your clockwork construct takes the form of a siege catapult, as the ones that stand by the camps of Tunaria. "
        "It fights as it always has: it rams what it fights and hurls a boulder with each of its strikes. On a PlayStation "
        "it keeps the look it had. Remove this mastery to have your construct's own form back.")
DECOR_COLS = [("kind", "TEXT NOT NULL DEFAULT ''"), ("shot", "INTEGER NOT NULL DEFAULT 0"),
              ("shot_scale", "REAL NOT NULL DEFAULT 0"), ("scale", "REAL NOT NULL DEFAULT 0")]


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def cols(db, tbl):
    return [r[1] for r in db.execute(f"PRAGMA table_info({tbl})")]


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
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key), go_json(before), go_json(after), DETAIL))


def write(db, tbl, key, want):
    """Make tbl's row of key want (a whole row; None deletes it), audited; (before, after)."""
    before = row(db, tbl, key)
    where = " AND ".join(f"{k} = ?" for k in key)
    if want is None:
        db.execute(f"DELETE FROM {tbl} WHERE {where}", tuple(key.values()))
        action = "delete"
    elif before is None:
        c = list(want)
        db.execute(f"INSERT INTO {tbl} ({', '.join(c)}) VALUES ({', '.join('?' for _ in c)})", tuple(want[k] for k in c))
        action = "insert"
    else:
        sets = [c for c in want if c not in key]
        db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in sets)} WHERE {where}", tuple(want[c] for c in sets) + tuple(key.values()))
        action = "update"
    after = row(db, tbl, key)
    audit(db, action, tbl, key, before, after)
    return before, after


def rank_id(db):
    r = db.execute("SELECT id FROM cmcatalog WHERE name = ?", (NAME,)).fetchall()
    if len(r) > 1:
        return None, f"{len(r)} cmcatalog rows named {NAME}: look first"
    if r:
        return r[0][0], None
    taken = {x[0] for x in db.execute("SELECT id FROM cmcatalog WHERE id BETWEEN ? AND ?", IDS)}
    free = next((i for i in range(IDS[0], IDS[1] + 1) if i not in taken), None)
    return free, None if free else f"no free cmcatalog id in {IDS[0]}-{IDS[1]}"


def plan(db, grant):
    """(column adds, writes [(tbl, key, want)], lines, refusals)."""
    for t in ("cmcatalog", "charcms", "characters", "decorart", "petstats", "gm_audit"):
        if not table(db, t):
            return [], [], [], [f"no {t} table here"]
    ch = db.execute("SELECT charName, race FROM characters WHERE serverid = ?", (ZERK,)).fetchone()
    if ch is None or ch[0] != ZERK_NAME:
        return [], [], [], [f"serverid {ZERK} is {ch and ch[0]!r}, not {ZERK_NAME}: this is for Heirloom alone"]
    owned = {r[0] for r in db.execute("SELECT cmid FROM charcms WHERE serverid = ?", (ZERK,))}
    if not {TINKER, CONSTRUCT} <= owned:
        return [], [], [], [f"{ZERK_NAME} does not own Tinker ({TINKER}) and Construct ({CONSTRUCT})"]
    parent = row(db, "cmcatalog", {"id": CONSTRUCT})
    if parent is None or parent["name"] != "Construct" or parent["mastery"] != "Tinker":
        return [], [], [], [f"cmcatalog {CONSTRUCT} is not Tinker's Construct"]
    have = {r[0] for r in db.execute(f"SELECT spellid FROM petstats WHERE spellid IN ({','.join('?' * len(SPELLS))})", SPELLS)}
    if have != set(SPELLS):
        return [], [], [], [f"petstats lacks the Construct spells {sorted(set(SPELLS) - have)}"]
    rid, why = rank_id(db)
    if why:
        return [], [], [], [why]
    lines, writes = [], []
    want = {"id": rid, "category": 4, "classes": 32767, "races": parent["races"], "mastery": "Tinker", "rank": 4, "name": NAME,
            "parent": CONSTRUCT, "cost": 0, "level": 60, "spent": 0, "minstats": "", "requires": "Construct@60",
            "require_spells": "|".join(str(s) for s in SPELLS), "effects": f"PetArt:{CATAPULT},OnlyFor:{ZERK}", "adds": "",
            "add_spells": "", "removes": "", "remove_spells": "", "description": DESC, "enabled": 1, "source": SRC}
    want = {k: v for k, v in want.items() if k in cols(db, "cmcatalog")}
    cur = row(db, "cmcatalog", {"id": rid})
    if cur is None or any(cur.get(k) != v for k, v in want.items()):
        writes.append(("cmcatalog", {"id": rid}, want))
        lines.append(f"cmcatalog {rid}: {NAME}, Tinker rank 4 under Construct, free, PetArt 0x{CATAPULT:08X}, for {ZERK_NAME} ({ZERK}) alone")
    else:
        lines.append(f"cmcatalog {rid}: {NAME} there already")
    have_cols = cols(db, "decorart")
    adds = [(c, d) for c, d in DECOR_COLS if c not in have_cols]
    for name, world, zone in ART:
        want = {"name": name, "model": CATAPULT, "world": world, "zone": zone, "sink": 0.0, "kind": "pet", "shot": BOULDER,
                "shot_scale": SHOT_SCALE, "scale": SCALE,
                "note": f"{ZERK_NAME}'s construct as the catapult (rank {rid}); a PS2 sees its body. The owner 2026-11-17",
                "source": SRC}
        clash = db.execute("SELECT name FROM decorart WHERE lower(trim(name)) = lower(?) AND name <> ?", (name, name)).fetchone()
        if clash:
            return [], [], lines, [f"decorart has {clash[0]!r}, another spelling of {name!r}: look first"]
        cur = row(db, "decorart", {"name": name})
        if cur is not None and cur.get("source") != SRC:
            return [], [], lines, [f"decorart {name!r} is there, not this tool's ({cur.get('source')}): look first"]
        if cur is None or any(cur.get(k) != v for k, v in want.items()):
            writes.append(("decorart", {"name": name}, want))
            lines.append(f"decorart {name!r}: kind pet, 0x{CATAPULT:08X} of world {world} zone {zone} at {SCALE}, "
                         f"firing 0x{BOULDER:08X} at {SHOT_SCALE}")
    if adds:
        lines.insert(0, "decorart: adding columns " + ", ".join(c for c, _ in adds))
    if grant:
        cur = row(db, "charcms", {"serverid": ZERK, "cmid": rid})
        if cur is None:
            writes.append(("charcms", {"serverid": ZERK, "cmid": rid},
                           {"serverid": ZERK, "cmid": rid, "cost": 0, "bought": int(datetime.datetime.now().timestamp())}))
            lines.append(f"charcms: {ZERK_NAME} given {NAME} (free; at his next login)")
    return adds, writes, lines, []


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--applied", required=True, help="JSON of what --apply wrote over, for --undo")
    ap.add_argument("--no-grant", action="store_true", help="offer the rank only; he takes it in the window")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            if not os.path.exists(a.applied):
                db.execute("ROLLBACK")
                print(f"nothing applied ({a.applied} not there)")
                return 1
            done = json.load(open(a.applied))
            left = [f"{w['tbl']} {go_json(w['key'])}: changed since" for w in done["rows"]
                    if go_json(row(db, w["tbl"], w["key"])) != go_json(w["after"])]
            if left:
                db.execute("ROLLBACK")
                print("REFUSED, nothing undone:\n  " + "\n  ".join(left))
                return 1
            for w in reversed(done["rows"]):
                write(db, w["tbl"], w["key"], w["before"])
            db.execute("COMMIT")
            os.rename(a.applied, a.applied + ".undone")
            print(f"undone: {len(done['rows'])} rows back (decorart's added columns stay); read at the next start (or re-save a row in the panel)")
            return 0
        if a.apply and os.path.exists(a.applied):
            db.execute("ROLLBACK")
            print(f"applied already ({a.applied}); --undo first")
            return 1
        adds, writes, lines, refusals = plan(db, not a.no_grant)
        for line in lines:
            print(line)
        if refusals:
            db.execute("ROLLBACK")
            print("REFUSED, nothing written:\n  " + "\n  ".join(refusals))
            return 1
        if not writes and not adds:
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"(report only: {len(writes)} rows; --apply to write)")
            return 0
        for c, d in adds:
            db.execute(f"ALTER TABLE decorart ADD COLUMN {c} {d}")
        done = []
        for tbl, key, want in writes:
            before, after = write(db, tbl, key, want)
            done.append({"tbl": tbl, "key": key, "before": before, "after": after})
        db.execute("COMMIT")
        with open(a.applied, "w") as f:
            json.dump({"rows": done, "columns": [c for c, _ in adds]}, f, indent=1, default=str)
        print(f"applied: {len(writes)} rows, in gm_audit and {a.applied}; read at the server's start (or re-save a row in the "
              "panel's Decoration art / Mastery ranks); his rank at his next login; the pet's look at its next summoning")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
