#!/usr/bin/env python3
"""The Plane of Disease's raid bosses take 15% more lightning damage (the GM Jibwiz's petition 1944, 2026-11-13: "All
plane of disease raid bosses should take 15% more damage from lightning resistance based damage, procs and nukes").

What it writes (a report by default; --apply writes, in one transaction with a gm_audit line a row; --undo takes it back):
- classtemplates.taken_resist = 'LR:115' on each PoD boss template (TEMPLATES): a hit or tick of LR on a creature of the
  template lands at 115%, after its strong resist's cut (entity/enchanter.go creatureResists: a nuke, a DoT's tick, a
  weapon's or a pet's proc, a damage shield). Queen Qy'tha, strong in LR at 50, takes 57.5% of a lightning nuke;
- the taken_resist column itself, when the table has none yet (the server that reads it adds it at its start; this adds
  the same column, so the tool can run before or after that start).

Not touched: 'pod named 61' (Gluboxious, Rawlg, Rotwood), 'pod monarch' (the Monarchs and Matriarchs) and 'pod roamer',
PoD's nameds and trash, not its raid bosses; a GM can give any of them the same on the panel (Resist damage taken %).

Needs the server with taken_resist (an older one ignores the column). A running server reads classtemplates again at a
GM panel template edit (reload), else at its next start.

Refused, nothing written, unless every template is there with a taken_resist of '' (or all 'LR:115': applied already).
Log pod_lightning_1113_log; --undo puts each value back only where it is still 'LR:115' (the column stays).

    tools/pod_lightning_20261113.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "pod_lightning_1113_log"
DETAIL = "PoD raid bosses take 15% more LR (the GM Jibwiz's petition 1944, 2026-11-13): tools/pod_lightning_20261113.py"
WHO, VIA = "dab", "tool"
VALUE = "LR:115"
COLUMN_DECL = "TEXT NOT NULL DEFAULT ''"
TEMPLATES = [
    "pod boss",                     # the million-HP bosses (petition 1144)
    "pod twins",                    # Yhafiran
    "bertoxxulous",
    "poxathixius and uzrathixius",  # the twins
    "pod queen",                    # Queen Qy'tha
    "pod noroxus",
    "pod fleshripper",
    "pod lich overlord",
    "pod sthlumchoxius",
    "pod grum",
    "pod rallius",                  # Rallius Rattican
]
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    name   TEXT,              -- the template, or NULL for the column this tool added
    before TEXT,
    audit  INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def has_column(db):
    return any(r[1] == "taken_resist" for r in db.execute("PRAGMA table_info(classtemplates)"))


def row_of(db, name):
    cur = db.execute("SELECT * FROM classtemplates WHERE name = ?", (name,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, name, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', 'update', 'classtemplates', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, go_json({"name": name}), go_json(before), go_json(after), DETAIL))
    return cur.lastrowid


def plan(db):
    """(lines, refusals, applied)."""
    for t in ("classtemplates", "gm_audit"):
        if not table(db, t):
            return [], [f"no {t} table here"], False
    col = has_column(db)
    lines, refusals, values = [], [], []
    for name in TEMPLATES:
        r = row_of(db, name)
        if r is None:
            refusals.append(f"classtemplates {name!r}: no row")
            continue
        v = r.get("taken_resist", "") if col else ""
        values.append(v)
        if v not in ("", VALUE):
            refusals.append(f"classtemplates {name!r}: taken_resist {v!r} already (a GM's?)")
        users = db.execute("SELECT count(*) FROM npctemplate WHERE template = ?", (name,)).fetchone()[0] if table(db, "npctemplate") else 0
        lines.append(f"classtemplates {name!r} (hp_pct {r['hp_pct']}, strong {r.get('resist_kind', '')!r} at {r.get('resist_pct', 0)}, "
                     f"{users} npctemplate names): taken_resist {v!r} -> {VALUE!r}")
    if not refusals and values and all(v == VALUE for v in values):
        return [], [], True
    if not refusals and any(v == VALUE for v in values):
        refusals.append("some templates have it and some not: half applied by someone else?")
    if not col:
        lines.insert(0, f"classtemplates: add column taken_resist {COLUMN_DECL} (as the server's own start does)")
    return lines, refusals, False


def apply(db):
    db.execute(LOG_SCHEMA)
    if not has_column(db):
        db.execute(f"ALTER TABLE classtemplates ADD COLUMN taken_resist {COLUMN_DECL}")
        db.execute(f"INSERT INTO {LOG} (name) VALUES (NULL)")
    for name in TEMPLATES:
        before = row_of(db, name)
        db.execute("UPDATE classtemplates SET taken_resist = ? WHERE name = ?", (VALUE, name))
        a = audit(db, name, before, row_of(db, name))
        db.execute(f"INSERT INTO {LOG} (name, before, audit) VALUES (?, ?, ?)", (name, json.dumps(before), a))
    return len(TEMPLATES)


def undo(db):
    """(back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, name, before in db.execute(f"SELECT seq, name, before FROM {LOG} ORDER BY seq DESC").fetchall():
        if name is None:
            continue  # the column stays: the server makes it at its start anyway
        was = json.loads(before)
        now = row_of(db, name)
        if now is None or now.get("taken_resist") != VALUE:
            left += 1
            continue
        db.execute("UPDATE classtemplates SET taken_resist = ? WHERE name = ?", (was.get("taken_resist", ""), name))
        audit(db, name, now, row_of(db, name))
        back += 1
    db.execute(f"DROP TABLE {LOG}")
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
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
            print(f"undone: {back} templates back" + (f", {left} changed since (left)" if left else ""))
            return 0
        lines, refusals, applied = plan(db)
        if applied:
            db.execute("ROLLBACK")
            print("applied already: nothing to do")
            return 0
        for l in lines:
            print(l)
        if refusals:
            db.execute("ROLLBACK")
            print("REFUSED:\n  " + "\n  ".join(refusals))
            return 1
        if not a.apply:
            db.execute("ROLLBACK")
            print("(report only: --apply to write)")
            return 0
        n = apply(db)
        db.execute("COMMIT")
        print(f"applied: {n} templates, in gm_audit and {LOG}")
        return 0
    except Exception:
        db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
