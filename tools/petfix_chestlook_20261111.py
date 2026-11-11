#!/usr/bin/env python3
"""The casters' Heirloom Chestguard on the cloth chest's icon (petition 1632, Zerkz, an elf enchanter: "Heirloom
chestguard shows pants instead of a chest item.").

The four casters' Heirloom Chestguards (magician, necromancer, enchanter, wizard) were seeded (tools/seed_heirlooms.py,
2026-11-06) in the look of the Fireworm Tunic 2225: model 1 (cloth, Armor[1], whose chest texture draws a cloth shirt
on the chest: rendered) and its icon, -430139392. That icon is the cloth LEGS' picture (178 slot-10 records of model 1
carry it; slot 5's cloth chest picture is -1092141747, 182 records, the Basher Shirt's): the item showed a pair of
pants in the inventory, on the paper doll and in a merchant's or the Outfitter's list. The look worn was right.

Four more chest records carry the same pants picture on a cloth chest (the Fireworm Tunic the heirlooms copied, the
Painted Silk Tunic, the Burnished Chestguard and the Mollusk Hauberk); they are given the chest's picture too.

  itempattern.itemicon -430139392 -> -1092141747 on every record of wear slot 5, model 1, sitting on the legs' icon,
  each looked up by name (+ class mask for the heirlooms): EXPECTED below. A record missing or not as expected is
  skipped and said (the Live shard's records may differ); one of slot 5 / model 1 on the legs' icon NOT named here is
  left and said.

Report only by default; --apply in one transaction, a gm_audit line a row, log chestlook_1632_log; --undo exact (a
row changed since is left and said). Item records go to a client at login: the new picture shows from the next login
(no restart; the look worn never changed).

    tools/petfix_chestlook_20261111.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "chestlook_1632_log"
WHO, VIA = "dab", "tool"
DETAIL = ("cloth chest icon for the casters' Heirloom Chestguard (petition 1632, Zerkz: \"Heirloom chestguard shows "
          "pants instead of a chest item.\"): tools/petfix_chestlook_20261111.py")
LEGS_ICON = -430139392   # slot 10 model 1: the cloth legs' picture
CHEST_ICON = -1092141747  # slot 5 model 1: the cloth chest's picture (the Basher Shirt's)
SLOT, MODEL = 5, 1
# (name, class mask or None for any)
EXPECTED = [
    ("Heirloom Chestguard", 1024),   # magician
    ("Heirloom Chestguard", 2048),   # necromancer
    ("Heirloom Chestguard", 4096),   # enchanter
    ("Heirloom Chestguard", 8192),   # wizard
    ("Fireworm Tunic", None),
    ("Painted Silk Tunic", None),
    ("Burnished Chestguard", None),
    ("Mollusk Hauberk", None),
]

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    keycol TEXT NOT NULL,
    keyval TEXT NOT NULL,
    col    TEXT NOT NULL,
    before TEXT NOT NULL,
    after  TEXT NOT NULL,
    audit  INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def compact(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False)


def audit(db, keyval, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', 'update', 'itempattern', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, compact({"patternid": keyval}),
                      compact({"itemicon": before}), compact({"itemicon": after}), DETAIL))
    return cur.lastrowid


def plan(db):
    """(changes [(patternid, name, before, after)], lines)."""
    changes, lines, named = [], [], set()
    for name, mask in EXPECTED:
        q = "SELECT patternid, equipslot, model, itemicon, classuse FROM itempattern WHERE itemname = ?"
        args = [name]
        if mask is not None:
            q += " AND classuse = ?"
            args.append(mask)
        rows = db.execute(q + " ORDER BY patternid", args).fetchall()
        what = f"{name}" + (f" (classes {mask})" if mask is not None else "")
        if not rows:
            lines.append(f"skip: {what}: no such record")
            continue
        for pid, slot, model, icon, _ in rows:
            named.add(pid)
            if icon == CHEST_ICON:
                lines.append(f"{pid} {what}: on the chest's icon already")
            elif (slot, model, icon) != (SLOT, MODEL, LEGS_ICON):
                lines.append(f"skip: {pid} {what}: slot {slot} model {model} icon {icon}, want slot {SLOT} model "
                             f"{MODEL} icon {LEGS_ICON}")
            else:
                changes.append((pid, what, LEGS_ICON, CHEST_ICON))
                lines.append(f"{pid} {what}: itemicon {LEGS_ICON} (cloth legs) -> {CHEST_ICON} (cloth chest)")
    for pid, name in db.execute("SELECT patternid, itemname FROM itempattern WHERE equipslot = ? AND model = ? AND "
                                "itemicon = ? ORDER BY patternid", (SLOT, MODEL, LEGS_ICON)):
        if pid not in named:
            lines.append(f"left (not named here): {pid} {name}: a chest on the legs' icon")
    return changes, lines


def write(db, keyval, before, after):
    cur = db.execute("UPDATE itempattern SET itemicon = ? WHERE patternid = ? AND itemicon = ?", (after, keyval, before))
    return cur.rowcount == 1


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for _, _, _, keyval, _, before, after, _ in db.execute(f"SELECT * FROM {LOG} ORDER BY seq DESC").fetchall():
        if not write(db, int(keyval), int(after), int(before)):
            print(f"left: itempattern {keyval} changed since")
            left += 1
            continue
        audit(db, int(keyval), int(after), int(before))
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
            print(f"put back {back} rows, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        if table(db, LOG):
            print(f"applied already ({LOG} is there): --undo first")
            return 0
        changes, lines = plan(db)
        for line in lines:
            print(line)
        if not changes:
            print("nothing to do")
            return 0
        if not a.apply:
            print(f"report only: {len(changes)} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            db.execute(LOG_SCHEMA)
            for pid, _, before, after in changes:
                if not write(db, pid, before, after):
                    raise RuntimeError(f"itempattern {pid} changed under the update")
                a_id = audit(db, pid, before, after)
                db.execute(f"INSERT INTO {LOG} (tbl, keycol, keyval, col, before, after, audit) "
                           "VALUES ('itempattern', 'patternid', ?, 'itemicon', ?, ?, ?)",
                           (str(pid), str(before), str(after), a_id))
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {len(changes)} rows, log {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
