#!/usr/bin/env python3
"""The Turban of Twisted Virtue undyed by its record (the owner, 2026-11-16: "also turban should be abled to be dyed").

A worn piece's record colour (itempattern.color) is its own colour where no dye is (entity.LookDyes fills an undyed
slot with it), so the turban's series crimson (3238989567, 0xC10F0EFF) drew it crimson until dyed. Set to white
(4294967295: entity.ArmorColor's "none"), the turban shows its own colours undyed (vanilla's hat::dyed leaves a white
alone), and the helm slot's dye (chardyes slot 5: the Equipment Dyer, #dye) colours it. A PS2 draws the fallback helm
(model 1) white undyed instead of the nearest tint to crimson.

Dust's own helm dye (chardyes 197 slot 5, 0xD10000FF on the 2026-11-16 copy: he dyed his set crimson) still applies:
nothing here touches a character's dyes.

Found by name in the heirloom range; checked as found (its colour the crimson, else nothing to do or REFUSED). Report
by default; --apply updates the one column, a gm_audit line (the panel's update, the row before and after), logged in
turban_undyed_1116_log; --undo puts back exactly the colour while the row is as written.

    python3 -E -s tools/turban_undyed_20261116.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "turban_undyed_1116_log"
DETAIL = "The Turban of Twisted Virtue's record colour white: undyed its own colours, dyeable (the owner, 2026-11-16); tools/turban_undyed_20261116.py"
WHO, VIA = "dab", "tool"
NAME, IDS = "Turban of Twisted Virtue", (714388, 714999)
CRIMSON, WHITE = 3238989567, 4294967295


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def row(db, pid):
    cur = db.execute("SELECT * FROM itempattern WHERE patternid = ?", (pid,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return None if m is None else json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, pid, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    return db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                      "VALUES (?, ?, ?, ?, '', 'update', 'itempattern', ?, ?, ?, ?)",
                      (at, acct[0] if acct else 0, WHO, VIA, go_json({"patternid": pid}), go_json(before), go_json(after), DETAIL)).lastrowid


def set_color(db, pid, color):
    before = row(db, pid)
    db.execute("UPDATE itempattern SET color = ? WHERE patternid = ?", (color, pid))
    after = row(db, pid)
    return before, after, audit(db, pid, before, after)


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
            if not table(db, LOG):
                db.execute("ROLLBACK")
                print("nothing to undo")
                return 0
            pid, before, after = db.execute(f"SELECT patternid, before, after FROM {LOG}").fetchone()
            if go_json(row(db, pid)) != after:
                db.execute("ROLLBACK")
                print(f"itempattern {pid} changed since: left as it is")
                return 1
            set_color(db, pid, json.loads(before)["color"])
            db.execute(f"DROP TABLE {LOG}")
            db.execute("COMMIT")
            print(f"undone: itempattern {pid} colour back to {json.loads(before)['color']}; Dust sees it at his next login")
            return 0
        for t in ("itempattern", "gm_audit"):
            if not table(db, t):
                db.execute("ROLLBACK")
                print(f"REFUSED: no {t} table here")
                return 1
        r = db.execute("SELECT patternid, color FROM itempattern WHERE itemname = ? AND patternid BETWEEN ? AND ?", (NAME, *IDS)).fetchall()
        if len(r) != 1:
            db.execute("ROLLBACK")
            print(f"REFUSED: {len(r)} records named {NAME} in {IDS[0]}-{IDS[1]}, want 1 (dust_turban_20261116.py --apply first)")
            return 1
        pid, color = r[0]
        if color == WHITE:
            db.execute("ROLLBACK")
            print(f"itempattern {pid}: white already; nothing to do")
            return 0
        if color != CRIMSON:
            db.execute("ROLLBACK")
            print(f"REFUSED: itempattern {pid} colour {color}, not the crimson {CRIMSON} it was made with: a GM changed it")
            return 1
        print(f"itempattern {pid} {NAME}: colour {CRIMSON} (crimson) -> {WHITE} (white: undyed, its own colours)")
        if not a.apply:
            db.execute("ROLLBACK")
            print("(report only; --apply to write)")
            return 0
        before, after, n = set_color(db, pid, WHITE)
        db.execute(f"CREATE TABLE {LOG} (patternid INTEGER NOT NULL, before TEXT NOT NULL, after TEXT NOT NULL, audit INTEGER NOT NULL)")
        db.execute(f"INSERT INTO {LOG} VALUES (?, ?, ?, ?)", (pid, go_json(before), go_json(after), n))
        db.execute("COMMIT")
        print(f"applied: in gm_audit and {LOG}; worn turbans take it at the wearer's next login (or re-equip)")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
