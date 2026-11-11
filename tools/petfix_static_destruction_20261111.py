#!/usr/bin/env python3
"""The wizard's lightning nova line, Static Destruction, beside Icy Destruction (petition 1583, Wisp, 2026-11-11:
"Wizard lightning Nova CM line is missing").

The line is in cmcatalog (Static Destruction 1-4, EQOA-Data cms/wizard.json and EQOAEmu's workbook: "Alters the nova
spell line, making it a lightning spell", cost 25 a rank, levels 20/35/50/60, as Icy Destruction's). It was locked to
a wizard who owned Icy Destruction: each rank of either line needs the plain spell (Explode and Detonate, Annihilate,
Nova, Supernova) and trades it away for its own element's and the Fiery one, so once one line had traded them the
other's ranks could never be bought (Wisp: Icy Destruction 1-4 owned, Static Destruction "???").

With the server's require_spells stand-ins (entity/cmbuy.go, CMAbility.Alts: "a|b"), each rank of both lines accepts
the Fiery spell in the plain one's place; both lines give it, so whichever line came first opens the other:

  Explode -> Fiery Explode, Detonate -> Fiery Detonate, Annihilate -> Fiery Annihilation, Nova -> Fiery Nova,
  Supernova -> Fiery Supernova (the Fiery ids read off the row's own add_spells).

Rows found by mastery and rank (Icy Destruction 1-4, Static Destruction 1-4, wizard), not by id, so the Live shard's
are found as Heirloom's. Refused, nothing written, unless every row is there with require_spells as expected (the plain
ids, or already exactly this tool's), its Fiery spell in its adds. Report only by default; --apply in one transaction,
a gm_audit line a row, log static_destruction_1111_log; --undo exact (a row changed since is left and said).
cmcatalog is read at the start (or the GM panel's reload of the masteries); the a|b reading needs the server of this
branch (an older one reads "11|22" as 11 alone: no harm, the line stays as locked as before).

    tools/petfix_static_destruction_20261111.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "static_destruction_1111_log"
DETAIL = ("petition 1583 (Wisp): Static Destruction buyable beside Icy Destruction, the Fiery spell standing in for the "
          "plain one: tools/petfix_static_destruction_20261111.py")
WHO, VIA = "claude", "tool"
WIZARD = 1 << 13
LINES = ("Icy Destruction", "Static Destruction")
FIERY = {"Explode": "Fiery Explode", "Detonate": "Fiery Detonate", "Annihilate": "Fiery Annihilation",
         "Nova": "Fiery Nova", "Supernova": "Fiery Supernova"}
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    id      INTEGER NOT NULL,   -- the cmcatalog row
    before  TEXT NOT NULL,      -- require_spells before
    after   TEXT NOT NULL,      -- require_spells as written
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def row_of(db, id):
    cur = db.execute("SELECT * FROM cmcatalog WHERE id = ?", (id,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def audit(db, id, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', 'update', 'cmcatalog', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, go_json({"id": id}), go_json(before), go_json(after), detail))
    return cur.lastrowid


def names(s):
    return [f.split("@")[0].strip() for f in s.split(";") if f.strip()]


def ids(s):
    return [f.strip() for f in s.split(",")] if s.strip() else []


def plan(db):
    """(changes [(id, label, before, after)], refusals, applied)."""
    changes, refusals = [], []
    if not table(db, "cmcatalog"):
        return changes, ["no cmcatalog table here"], False
    done = 0
    for line in LINES:
        for rank in range(1, 5):
            rows = db.execute("SELECT id FROM cmcatalog WHERE mastery = ? AND rank = ? AND classes & ? != 0",
                              (line, rank, WIZARD)).fetchall()
            label = f"{line} {rank}"
            if len(rows) != 1:
                refusals.append(f"{label}: {len(rows)} wizard rows, want one")
                continue
            r = row_of(db, rows[0][0])
            req, req_ids = names(r["requires"]), ids(r["require_spells"])
            add_ids = dict(zip(names(r["adds"]), ids(r["add_spells"])))
            if len(req) != len(req_ids) or not req:
                refusals.append(f"{label} ({r['id']}): requires {r['requires']!r} vs require_spells {r['require_spells']!r}")
                continue
            want = []
            for n, i in zip(req, req_ids):
                fiery = FIERY.get(n)
                fid = add_ids.get(fiery) if fiery else None
                if fid is None:
                    refusals.append(f"{label} ({r['id']}): needs {n!r}, which has no Fiery stand-in among its adds")
                    break
                base = i.split("|")[0]
                if i not in (base, f"{base}|{fid}"):
                    refusals.append(f"{label} ({r['id']}): require_spells {i!r} for {n!r} is not as expected")
                    break
                sp = db.execute("SELECT spellname FROM spellpattern WHERE spellid = ?", (int(fid),)).fetchone()
                if sp is None or sp[0] != fiery:
                    refusals.append(f"{label} ({r['id']}): spell {fid} is {sp!r}, not {fiery!r}")
                    break
                want.append(f"{base}|{fid}")
            else:
                after = ",".join(want)
                if r["require_spells"] == after:
                    done += 1
                else:
                    changes.append((r["id"], label, r["require_spells"], after))
    return changes, refusals, done == 2 * 4


def apply(db, changes):
    db.execute(LOG_SCHEMA)
    for id, _, before, after in changes:
        b = row_of(db, id)
        db.execute("UPDATE cmcatalog SET require_spells = ? WHERE id = ?", (after, id))
        a = audit(db, id, b, row_of(db, id), DETAIL)
        db.execute(f"INSERT INTO {LOG} (id, before, after, audit) VALUES (?, ?, ?, ?)", (id, before, after, a))


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, id, before, after in db.execute(f"SELECT seq, id, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        now = row_of(db, id)
        if now is None or now["require_spells"] != after:
            left += 1
            print(f"  left: cmcatalog {id}, require_spells changed since: {None if now is None else now['require_spells']!r}")
            continue
        db.execute("UPDATE cmcatalog SET require_spells = ? WHERE id = ?", (before, id))
        audit(db, id, now, row_of(db, id), "undo: " + DETAIL)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if left == 0:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"put back {back}, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        changes, refusals, applied = plan(db)
        for r in refusals:
            print("REFUSED:", r)
        if refusals:
            db.execute("ROLLBACK")
            return 1
        if applied:
            db.execute("ROLLBACK")
            print("applied already: every rank of both lines takes the Fiery stand-in; nothing done")
            return 0
        for id, label, before, after in changes:
            print(f"cmcatalog {id} {label}: require_spells {before!r} -> {after!r}")
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"report only: {len(changes)} rows would change (--apply to write)")
            return 0
        apply(db, changes)
        db.execute("COMMIT")
        print(f"applied: {len(changes)} rows; a restart (or the GM panel's mastery reload) reads them")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
