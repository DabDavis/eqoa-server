#!/usr/bin/env python3
"""Dust's Heirloom Shield of Twisted Virtue as an energy shield (the owner, 2026-11-14: "dusts shield is just metal but
should look like the shield he had before but shaped like the current shield now with the same diamond and cross
pattern but with the energy shield effect").

The shield (itempattern 714383) keeps its model, 0x2486A8D7: the casters' square shield, whose diamond and cross is its
own relief, and which IS plain metal (one opaque metal layer, no glow: nothing killed its energy). The energy was the
old model's, 0x2EC56029: a round disc of one additive layer, a streak texture (0xE7520641) scrolling round. This gives
714383 an itemaura row of kind 9 (database.AuraEnergy: an energy skin, a held item's own aura shown on the hand that
holds it), source 0x2EC56029, colour 0xC10F0EFF (the record's own Twisted Virtue crimson): vanilla lays the disc's
streak layer over the square's metal in crimson and lights it by night (aura::energy_skin). Every holder of 714383
shows it (Dust holds the only one); a PS2 and an older client draw the metal shield as now.

Checked as found, else REFUSED and nothing written: itempattern 714383 the Heirloom Shield of Twisted Virtue; tables
itemaura and gm_audit. A row there already (a GM's) is changed and its old values kept in the log. Report by default;
--apply in one transaction, a gm_audit line, logged in twisted_virtue_shield_1114_log; a second --apply finds nothing
to do; --undo puts back exactly what was there. On a shard without 714383 (Live) it reports so and writes nothing.

Needs a server with shield-energy (an older one drops a kind 9 row on read: nothing changes) and a vanilla with it.
After --apply: the panel's "Reload every NPC".

    python3 -E -s tools/twisted_virtue_shield_energy_20261114.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "twisted_virtue_shield_1114_log"
DETAIL = "The Heirloom Shield of Twisted Virtue an energy shield (the owner, 2026-11-14); tools/twisted_virtue_shield_energy_20261114.py"
WHO, VIA = "dab", "tool"
SHIELD, NAME = 714383, "Heirloom Shield of Twisted Virtue"
WANT = {"patternid": SHIELD, "color": 0xC10F0EFF, "kind": 9, "source": 0x2EC56029, "nodes": 0,
        "note": "The Heirloom Shield of Twisted Virtue: the old energy shield's streaks over its square, crimson, the owner 2026-11-14"}

LOG_DDL = f"""CREATE TABLE {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    before TEXT,
    after  TEXT NOT NULL,
    audit  INTEGER NOT NULL
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def row(db):
    cur = db.execute("SELECT * FROM itemaura WHERE patternid = ?", (SHIELD,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return None if m is None else json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, 'itemaura', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, go_json({"patternid": SHIELD}), go_json(before), go_json(after), DETAIL))
    return cur.lastrowid


def put(db, want):
    """itemaura's row of the shield made want (None: deleted); (before, after, audit)."""
    before = row(db)
    if want is None:
        db.execute("DELETE FROM itemaura WHERE patternid = ?", (SHIELD,))
        action = "delete"
    elif before is None:
        db.execute("INSERT INTO itemaura (patternid, color, kind, source, nodes, note) VALUES (?, ?, ?, ?, ?, ?)",
                   (SHIELD, want["color"], want["kind"], want["source"], want["nodes"], want["note"]))
        action = "insert"
    else:
        db.execute("UPDATE itemaura SET color = ?, kind = ?, source = ?, nodes = ?, note = ? WHERE patternid = ?",
                   (want["color"], want["kind"], want["source"], want["nodes"], want["note"], SHIELD))
        action = "update"
    after = row(db)
    return before, after, audit(db, action, before, after)


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
            back, left = 0, []
            if table(db, LOG):
                for seq, before, after in db.execute(f"SELECT seq, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
                    if go_json(row(db)) != after:
                        left.append(f"itemaura {SHIELD}: changed since, left as it is")
                        continue
                    put(db, json.loads(before) if before else None)
                    db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
                    back += 1
                if not left:
                    db.execute(f"DROP TABLE {LOG}")
            db.execute("COMMIT")
            print(f"undone: {back} rows back" + "".join(f"\n  left: {w}" for w in left) +
                  ("; press the GM panel's Reload every NPC" if back else ""))
            return 0 if not left else 1
        for t in ("itempattern", "itemaura", "gm_audit"):
            if not table(db, t):
                db.execute("ROLLBACK")
                print(f"REFUSED, nothing written:\n  no {t} table here")
                return 1
        it = db.execute("SELECT itemname, equipslot, model FROM itempattern WHERE patternid = ?", (SHIELD,)).fetchone()
        if it is None:
            db.execute("ROLLBACK")
            print(f"no itempattern {SHIELD} on this shard: nothing to do")
            return 0
        if it[0] != NAME or it[1] != 13:
            db.execute("ROLLBACK")
            print(f"REFUSED, nothing written:\n  itempattern {SHIELD} is {it[0]!r} (wear slot {it[1]}), not the {NAME}")
            return 1
        held = db.execute("SELECT c.charName, i.equiploc FROM charinventory i JOIN characters c ON c.serverid = i.serverid "
                          "WHERE i.patternid = ?", (SHIELD,)).fetchall() if table(db, "characters") else []
        print(f"{NAME} ({SHIELD}), model 0x{it[2] & 0xFFFFFFFF:08X}; held by " + (", ".join(f"{n} (equiploc {e})" for n, e in held) or "no one"))
        before = row(db)
        if before == WANT:
            db.execute("ROLLBACK")
            print("  already the energy skin\nnothing to do")
            return 0
        if table(db, LOG):
            db.execute("ROLLBACK")
            print(f"applied before and changed since ({LOG} is there): --undo first")
            return 1
        was = "none" if before is None else f"kind {before['kind']} 0x{before['color'] & 0xFFFFFFFF:08X} (kept in the log)"
        print(f"  itemaura {SHIELD}: {was} -> kind 9 (energy skin) of 0x2EC56029 in 0xC10F0EFF")
        if not a.apply:
            db.execute("ROLLBACK")
            print("(report only: 1 row; --apply to write)")
            return 0
        db.execute(LOG_DDL)
        before, after, n = put(db, WANT)
        db.execute(f"INSERT INTO {LOG} (before, after, audit) VALUES (?, ?, ?)", (go_json(before), go_json(after), n))
        db.execute("COMMIT")
        print(f"applied: 1 row, in gm_audit and {LOG}; press the GM panel's Reload every NPC")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
