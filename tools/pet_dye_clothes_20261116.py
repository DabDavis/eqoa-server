#!/usr/bin/env python3
"""The pets' dyes by the clothing rule: the drachnid's hand-made areas taken out (the owner, 2026-11-16: "for the pet dye
the pets need their clothes as one dye and the rest another like we two tone equipment").

From the pet-dye-clothes build a body with no petdyeareas row is split by the client's rule (area 1 its clothing, area 2
the rest of it: entity.PetBodyClothed, vanilla's render/petart.rs). The one row there is, Jarek's Creation's
(0x15764DA8: "Torso" / "Spider body", area 2 the spider alone, tools/pet_groomers_20261116.py), would put that split in
place of the rule; the rule's is the owner's (the plating, sleeves and gauntlets one colour, the spider, skin, face and
hair another). So the row goes, a gm_audit line. A character's dyes on the body (petdyes) are kept as they are: its
area 1 colour is now the clothing's, its area 2 the body's.

Only the row as tools/pet_groomers_20261116.py wrote it is taken out; a row a GM changed since is left, and said. Report
by default; --apply in one transaction, logged in pet_dye_clothes_1116_log; --undo puts it back exactly (unless a row is
there again). After --apply: the panel's "Reload every NPC" (or a petdyeareas edit) reads the areas again; the clients
that ask are told at once.

    python3 -E -s tools/pet_dye_clothes_20261116.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "pet_dye_clothes_1116_log"
DETAIL = "The pets' dyes by the clothing rule: the drachnid's hand-made areas out (the owner, 2026-11-16); tools/pet_dye_clothes_20261116.py"
WHO, VIA = "dab", "tool"
JAREK = 0x15764DA8
WRITTEN = {"look": JAREK, "area1": "Torso", "area2": "Spider body", "materials": "0"}


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def row(db, look):
    cur = db.execute("SELECT * FROM petdyeareas WHERE look = ?", (look,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return None if m is None else json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, 'petdyeareas', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, go_json({"look": JAREK}), go_json(before), go_json(after), DETAIL))
    return cur.lastrowid


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
        if not table(db, "petdyeareas") or not table(db, "gm_audit"):
            db.execute("ROLLBACK")
            print("no petdyeareas (or gm_audit) here: nothing to do")
            return 0
        if a.undo:
            if not table(db, LOG):
                db.execute("ROLLBACK")
                print("nothing to undo")
                return 0
            (before,) = db.execute(f"SELECT before FROM {LOG}").fetchone()
            if row(db, JAREK) is not None:
                db.execute("ROLLBACK")
                print(f"petdyeareas 0x{JAREK:08X} is there again: left as it is, nothing undone")
                return 1
            want = json.loads(before)
            cols = list(want)
            db.execute(f"INSERT INTO petdyeareas ({', '.join(cols)}) VALUES ({', '.join('?' for _ in cols)})", tuple(want[c] for c in cols))
            audit(db, "insert", None, row(db, JAREK))
            db.execute(f"DROP TABLE {LOG}")
            db.execute("COMMIT")
            print(f"undone: petdyeareas 0x{JAREK:08X} back; press the GM panel's Reload every NPC")
            return 0
        have = row(db, JAREK)
        n = db.execute("SELECT count(*) FROM petdyes WHERE look = ?", (JAREK,)).fetchone()[0] if table(db, "petdyes") else 0
        if have is None:
            db.execute("ROLLBACK")
            print(f"petdyeareas 0x{JAREK:08X}: no row, the rule's already; nothing to do")
            return 0
        if any(have.get(k) != v for k, v in WRITTEN.items()):
            db.execute("ROLLBACK")
            print(f"REFUSED, nothing written: petdyeareas 0x{JAREK:08X} is not as tools/pet_groomers_20261116.py wrote it: {have}")
            return 1
        print(f"petdyeareas 0x{JAREK:08X} ({have['area1']} / {have['area2']}, materials {have['materials']}) -> no row: the rule's "
              f"clothing / body; {n} character(s) with dyes on the body keep them (area 1 now the clothing's, area 2 the body's)")
        if not a.apply:
            db.execute("ROLLBACK")
            print("(report only: --apply to write)")
            return 0
        db.execute(f"CREATE TABLE {LOG} (before TEXT NOT NULL, audit INTEGER NOT NULL)")
        db.execute("DELETE FROM petdyeareas WHERE look = ?", (JAREK,))
        aid = audit(db, "delete", have, None)
        db.execute(f"INSERT INTO {LOG} (before, audit) VALUES (?, ?)", (go_json(have), aid))
        db.execute("COMMIT")
        print(f"applied: the row out, gm_audit {aid}, logged in {LOG}; press the GM panel's Reload every NPC")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
