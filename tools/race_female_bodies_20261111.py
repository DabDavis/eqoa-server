#!/usr/bin/env python3
"""The hill giant, the orc and the satyr given their own female bodies in racedefs (the GM Jibrish's petition 1647,
2026-11-11: "when making a race as sylvan the male and female models both show the male but there should def be a
female one - the model is in the game"; "sylvan" is the satyr: the disc's sylvan fauns and faun maidens wear her body).

Each of the three races' options (entity/raceoptions.go) has a woman's body row whose first step is a woman's body of
the disc, but racedefs' body_female named the male's, so the create pages' race and sex page drew the male for both
sexes, and a creation that sends no steps (a PS2, an older client) made a woman in the male's body. Read off CHAR.ESF
by sight (vanilla's eqoa shot, beside each male):

  11 Hill Giant  0xFFB1EA2C -> 0xF34501CB  the hill giantess (the hill giant woman NPC's; the options' "Hill giant" step)
  21 Orc         0x8214A32D -> 0x0A03EB92  the orc matron (the cracktooth and hatebone matrons'; the options' "Orc")
  25 Satyr       0xBE10226F -> 0x097ADB18  the faun (the sylvan fauns', faun maidens' and acolyte fauns'; "Faun")

Every other race was looked at: its woman is its own row already (Kerran, Aviak, the Wisp's human), or the disc has no
woman of that creature (every body on its skeleton rendered or named; claude-notes/race-options-all-20261109.md).

No character changes: a body is characters.modelid, and the women of these races made in vanilla wear the woman's
already (her step); the one hill giant woman in the male's body keeps it. The code of the same branch keeps the hill
giantess in her own body in any armour (entity/armorbody.go: the armour bodies are the male's); an older server would
put her in the male sand and fire giants' bodies by her armour, as it did any hill giant.

Refused, nothing written, unless each row's name is as above and its body_male the male's, and body_female the male's
(or the woman's: applied already). Report only by default; --apply in one transaction, a gm_audit line each, log
race_female_bodies_1111_log; --undo exact (a row changed since is left and said). racedefs is read at the start
(and after a GM's edit of it in the panel): a restart takes it.

    tools/race_female_bodies_20261111.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "race_female_bodies_1111_log"
DETAIL = ("the GM Jibrish's petition 1647: the race's own female body (the options' woman) in racedefs: "
          "tools/race_female_bodies_20261111.py")
WHO, VIA = "dab", "tool"


def s32(u):
    return u - (1 << 32) if u >= 1 << 31 else u


# id: (name, male body, female body), DictIDs as the disc's (stored signed, as the server writes them)
RACES = {
    11: ("Hill Giant", 0xFFB1EA2C, 0xF34501CB),
    21: ("Orc", 0x8214A32D, 0x0A03EB92),
    25: ("Satyr", 0xBE10226F, 0x097ADB18),
}
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    id      INTEGER NOT NULL,   -- racedefs.id
    before  INTEGER NOT NULL,   -- body_female before
    after   INTEGER NOT NULL,   -- body_female written
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def row_of(db, rid):
    cur = db.execute("SELECT * FROM racedefs WHERE id = ?", (rid,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, rid, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', 'update', 'racedefs', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, go_json({"id": rid}), go_json(before), go_json(after), detail))
    return cur.lastrowid


def plan(db):
    """(changes [(id, name, before, after)], lines, refusals, applied)."""
    changes, lines, refusals = [], [], []
    if not table(db, "racedefs"):
        return changes, lines, ["no racedefs table here"], False
    done = 0
    for rid, (name, male, female) in RACES.items():
        r = row_of(db, rid)
        if r is None:
            refusals.append(f"racedefs {rid}: no such row (want {name})")
            continue
        if r["name"] != name or r["body_male"] != s32(male):
            refusals.append(f"racedefs {rid}: {r['name']!r} body_male {r['body_male'] & 0xFFFFFFFF:#010x}, want {name!r} {male:#010x}")
            continue
        if r["body_female"] == s32(female):
            done += 1
            lines.append(f"  {rid} {name}: body_female {female:#010x} already")
            continue
        if r["body_female"] != s32(male):
            refusals.append(f"racedefs {rid} {name}: body_female {r['body_female'] & 0xFFFFFFFF:#010x}, "
                            f"neither the male's {male:#010x} nor {female:#010x}")
            continue
        changes.append((rid, name, r["body_female"], s32(female)))
        lines.append(f"  {rid} {name}: body_female {male:#010x} (the male's) -> {female:#010x}")
    return changes, lines, refusals, done == len(RACES)


def apply(db, changes):
    db.execute(LOG_SCHEMA)
    for rid, _, before, after in changes:
        b = row_of(db, rid)
        db.execute("UPDATE racedefs SET body_female = ? WHERE id = ? AND body_female = ?", (after, rid, before))
        a = audit(db, rid, b, row_of(db, rid), DETAIL)
        db.execute(f"INSERT INTO {LOG} (id, before, after, audit) VALUES (?, ?, ?, ?)", (rid, before, after, a))


def undo(db):
    """(back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, rid, before, after in db.execute(f"SELECT seq, id, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        now = row_of(db, rid)
        if now is None or now["body_female"] != after:
            left += 1
            print(f"  left: racedefs {rid}, changed since it was written: {None if now is None else now['body_female']}")
            continue
        db.execute("UPDATE racedefs SET body_female = ? WHERE id = ?", (before, rid))
        audit(db, rid, now, row_of(db, rid), "undo: " + DETAIL)
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
    try:
        if a.undo:
            db.execute("BEGIN IMMEDIATE")
            try:
                back, left = undo(db)
                db.execute("COMMIT")
            except BaseException:
                db.execute("ROLLBACK")
                raise
            print(f"put back {back}, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        changes, lines, refusals, applied = plan(db)
        for line in lines:
            print(line)
        if applied:
            print("applied already (every row has its woman's body): nothing done")
            return 0
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if table(db, LOG):
            print(f"{LOG} is there but the rows are not all applied: --undo first")
            return 1
        if not a.apply:
            print(f"report only: {len(changes)} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            apply(db, changes)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {len(changes)} rows, in gm_audit and {LOG} (a restart reads racedefs)")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
