#!/usr/bin/env python3
"""The pet looks bought with masteries replace the pet they change, as their words say (Fallenangel's petition 1994,
2026-11-13: "Nec Pet weapon upgrade 1 CM cosmetic changes should replace the existing spell. Currently it adds
another to spell book allowing you to have many 60 pets in spellbook ... the nec could cast different powerful pets one
after the other if they died without waiting for cooldown").

The necromancer's line (Unholy Sacrament 4, then the Pet Graphic ranks, each the parent of the next):

  cmcatalog 6368 Unholy Sacrament 4         replaces Swarming Death (700823) with Bone Swarming Death (60) (901004);
                                            it gave the new pet and took nothing (Unholy Sacrament 3 takes its
                                            Swarming Bones, as every rank of the line before it)
  cmcatalog 6372 Unholy Ghoulish Sacrament  replaces Bone Swarming Death (60) (901004) with Ghoulish Death (900994)
  cmcatalog 6373 Wraith's Unholy Sacrament  replaces Ghoulish Death (900994) with Haunting Death (900995)
  cmcatalog 6374 Crimson Unholy Sacrament   replaces Haunting Death (900995) with Red Swarming Death (60) (906300)

  (6371 Bone Collection, which needs 6368 and gives its 901004, is left as it is.) Each took nothing: a necromancer
  of the whole line held five level 60 pets, each on its own recast.

The gnome's Construct looks (Tinker's Construct, 1542, gives Construct, 901835):

  cmcatalog 900842 Construct (Rat)       replaces Construct (901835) with Construct (Rat) (901837)
  cmcatalog 900843 Construct (Drachnid)  now after the rat (parent 900842, rank 5; it was the rat's sibling under
                                         1542), needs Construct (Rat) and replaces it with Construct (Drachnid)
                                         (901836): two siblings that each replaced Construct would both be owned and
                                         both looks held; a chain holds one look, the last bought

A line whose ranks each replace the one before is undone by a reset as one since the same branch
(entity.CMRemovalBook): only the first spell goes back (Swarming Death, Construct). A removal of the last rank gives
back the look before it. Of the two characters owning Construct (Drachnid) without the rat (Heirloom 2026-11-13: 498,
692), a removal of it would give them the rat's look: said here, not changed.

The books (spells) of the characters OWNING the ranks: each keeps the look of its last rank (the necromancer: 6374's,
else 6373's, 6372's, else Bone Swarming Death (60); the gnome: the look rank bought last, charcms.bought) and the rest
of the line's spells leave its book (every row of each), a hot bar slot of one that leaves going to the look kept when
it has none. A character holding the line's spells by a GM's grant or #template, owning no rank, is not touched; nor
one whose book lacks the look it keeps (said). Nothing is added to a book.

Refused, nothing written, unless the six catalog rows are as they were on 2026-11-13 (or as this tool writes them).
Report only by default; --apply in one transaction, a gm_audit line a row, log pet_looks_1113_log; --undo exact (a
row changed since is left and said). cmcatalog is read at the start (and at a GM panel edit of it); a book at the
next login.

    tools/pet_looks_20261113.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "pet_looks_1113_log"
DETAIL = "A pet look bought with masteries replaces the pet it changes (petition 1994): tools/pet_looks_20261113.py"
WHO, VIA = "dab", "tool"

SWARMING, BONE, GHOULISH, HAUNTING, RED = 700823, 901004, 900994, 900995, 906300
CONSTRUCT, DRACHNID, RAT = 901835, 901836, 901837
SACRAMENT4, BONECOLL, GHOUL_R, WRAITH_R, CRIMSON_R = 6368, 6371, 6372, 6373, 6374
TINKER2, RAT_R, DRACH_R = 1542, 900842, 900843

# the rows as they were on 2026-11-13 (the fields this tool reads or changes)
WAS = {
    SACRAMENT4: {"parent": 6367, "require_spells": str(SWARMING), "add_spells": str(BONE), "removes": "",
                 "remove_spells": ""},
    GHOUL_R: {"parent": BONECOLL, "add_spells": str(GHOULISH), "removes": "", "remove_spells": ""},
    WRAITH_R: {"parent": GHOUL_R, "add_spells": str(HAUNTING), "removes": "", "remove_spells": ""},
    CRIMSON_R: {"parent": WRAITH_R, "add_spells": str(RED), "removes": "", "remove_spells": ""},
    RAT_R: {"parent": TINKER2, "rank": 4, "requires": "Construct@60", "require_spells": str(CONSTRUCT),
            "add_spells": str(RAT), "removes": "", "remove_spells": ""},
    DRACH_R: {"parent": TINKER2, "rank": 4, "requires": "Construct@60", "require_spells": str(CONSTRUCT),
              "add_spells": str(DRACHNID), "removes": "", "remove_spells": ""},
}

# what this tool makes of them
NOW = {
    SACRAMENT4: {"removes": "Swarming Death@60", "remove_spells": str(SWARMING)},
    GHOUL_R: {"removes": "Bone Swarming Death (60)@60", "remove_spells": str(BONE)},
    WRAITH_R: {"removes": "Ghoulish Death@60", "remove_spells": str(GHOULISH)},
    CRIMSON_R: {"removes": "Haunting Death@60", "remove_spells": str(HAUNTING)},
    RAT_R: {"removes": "Construct@60", "remove_spells": str(CONSTRUCT)},
    DRACH_R: {"parent": RAT_R, "rank": 5, "requires": "Construct (Rat)@60", "require_spells": str(RAT),
              "removes": "Construct (Rat)@60", "remove_spells": str(RAT)},
}

# the lines: the spells of each, and the look each owned rank keeps (the last of the list owned wins)
NECRO = {"name": "necromancer", "spells": (SWARMING, BONE, GHOULISH, HAUNTING, RED),
         "ranks": ((SACRAMENT4, BONE), (BONECOLL, BONE), (GHOUL_R, GHOULISH), (WRAITH_R, HAUNTING), (CRIMSON_R, RED))}
GNOME = {"name": "gnome", "spells": (CONSTRUCT, DRACHNID, RAT), "ranks": ((RAT_R, RAT), (DRACH_R, DRACHNID))}

KEYS = {"cmcatalog": ("id",), "spells": ("id",)}

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,      -- cmcatalog or spells
    rowkey  TEXT NOT NULL,      -- the row's key (JSON)
    before  TEXT,               -- the row before (JSON), NULL for none
    after   TEXT,               -- the row as written (JSON), NULL for deleted
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def rows(db, sql, args=()):
    cur = db.execute(sql, args)
    cols = [d[0] for d in cur.description]
    return [dict(zip(cols, r)) for r in cur.fetchall()]


def key_of(tbl, row):
    return {k: row[k] for k in KEYS[tbl]}


def get(db, tbl, key):
    where = " AND ".join(f"{k} = ?" for k in key)
    r = rows(db, f"SELECT * FROM {tbl} WHERE {where}", list(key.values()))
    return r[0] if r else None


def go_json(m):
    return json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, tbl, key, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() \
        if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key),
                      None if before is None else go_json(before), None if after is None else go_json(after), detail))
    return cur.lastrowid


def matches(row, want):
    return row is not None and all(row.get(k) == v for k, v in want.items())


def keeps(db, line, sid):
    """The look the character's owned ranks of the line keep, None for none owned."""
    owned = {r[0]: r[1] for r in db.execute("SELECT cmid, bought FROM charcms WHERE serverid = ?", (sid,))}
    if line is GNOME:
        mine = [(owned[r], spell) for r, spell in line["ranks"] if r in owned]
        return max(mine)[1] if mine else None
    look = None
    for r, spell in line["ranks"]:
        if r in owned:
            look = spell
    return look


def book_changes(db, line, out, lines):
    ranks = [r for r, _ in line["ranks"]]
    owners = [r[0] for r in db.execute(
        f"SELECT DISTINCT serverid FROM charcms WHERE cmid IN ({', '.join('?' * len(ranks))}) ORDER BY serverid", ranks)]
    for sid in owners:
        look = keeps(db, line, sid)
        held = rows(db, f"SELECT * FROM spells WHERE serverid = ? AND spellid IN ({', '.join('?' * len(line['spells']))}) "
                        "ORDER BY id", [sid] + list(line["spells"]))
        kept = [h for h in held if h["spellid"] == look]
        if not kept:
            lines.append(f"spells: {line['name']} character {sid} keeps {look} by its ranks but its book lacks it: "
                         f"left as it is ({[h['spellid'] for h in held]})")
            continue
        keep = kept[0]
        going = [h for h in held if h["id"] != keep["id"]]
        if not going:
            continue
        bar = next((h for h in going if h["onHotBar"]), None)
        if not keep["onHotBar"] and bar is not None:
            out.append(("spells", {"id": keep["id"]}, {**keep, "onHotBar": bar["onHotBar"], "whereonBar": bar["whereonBar"]}))
            lines.append(f"spells {keep['id']}: character {sid}'s {look} takes the hot bar slot {bar['whereonBar']} of "
                         f"{bar['spellid']}")
        for h in going:
            out.append(("spells", {"id": h["id"]}, None))
        lines.append(f"spells: {line['name']} character {sid} keeps {look} (book place {keep['addedorder']}), "
                     f"out: {[h['spellid'] for h in going]}")


def changes(db):
    """(changes, lines, refusals, applied): changes a list of (tbl, key, after) with after None for a delete."""
    lines, refusals = [], []
    for t in ("cmcatalog", "charcms", "spells", "gm_audit"):
        if not table(db, t):
            return [], lines, [f"no {t} table here"], False
    cat = {i: get(db, "cmcatalog", {"id": i}) for i in WAS}
    out = []
    done = all(matches(cat[i], NOW[i]) for i in NOW)
    for i in WAS:
        if done:
            break
        if not matches(cat[i], WAS[i]):
            refusals.append(f"cmcatalog {i} is not as it was on 2026-11-13: { {k: (cat[i] or {}).get(k) for k in WAS[i]} }")
            continue
        out.append(("cmcatalog", {"id": i}, {**cat[i], **NOW[i]}))
        lines.append(f"cmcatalog {i} {cat[i]['name']!r}: " + ", ".join(
            f"{k} {cat[i][k]!r} -> {v!r}" for k, v in NOW[i].items() if cat[i][k] != v))
    for line in (NECRO, GNOME):
        book_changes(db, line, out, lines)
    for sid, in db.execute("SELECT serverid FROM charcms WHERE cmid = ? AND serverid NOT IN "
                           "(SELECT serverid FROM charcms WHERE cmid = ?) ORDER BY serverid", (DRACH_R, RAT_R)).fetchall():
        lines.append(f"charcms: character {sid} owns Construct (Drachnid) without the rat: a removal of it would give "
                     "the rat's look back")
    if done and not out:
        return [], lines, refusals, True
    return out, lines, refusals, False


def write(db, tbl, key, after):
    before = get(db, tbl, key)
    if after is None:
        where = " AND ".join(f"{k} = ?" for k in key)
        db.execute(f"DELETE FROM {tbl} WHERE {where}", list(key.values()))
        action = "delete"
    elif before is None:
        cols = list(after)
        db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                   [after[c] for c in cols])
        action = "insert"
    else:
        sets = [c for c in after if before.get(c) != after[c]]
        if not sets:
            return
        where = " AND ".join(f"{k} = ?" for k in key)
        db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in sets)} WHERE {where}",
                   [after[c] for c in sets] + list(key.values()))
        action = "update"
    now = get(db, tbl, key)
    return action, before, now


def apply(db, out):
    db.execute(LOG_SCHEMA)
    n = 0
    for tbl, key, after in out:
        r = write(db, tbl, key, after)
        if r is None:
            continue
        action, before, now = r
        a = audit(db, action, tbl, key, before, now, DETAIL)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                   (tbl, json.dumps(key, sort_keys=True), None if before is None else json.dumps(before, sort_keys=True),
                    None if now is None else json.dumps(now, sort_keys=True), a))
        n += 1
    return n


def undo(db):
    """(back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, tbl, rowkey, before, after in db.execute(
            f"SELECT seq, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key = json.loads(rowkey)
        now = get(db, tbl, key)
        if (now is None) != (after is None) or (now is not None and json.dumps(now, sort_keys=True) != after):
            left += 1
            print(f"  left: {tbl} {key}, changed since it was written: {now}")
            continue
        was = None if before is None else json.loads(before)
        if was is None:
            where = " AND ".join(f"{k} = ?" for k in key)
            db.execute(f"DELETE FROM {tbl} WHERE {where}", list(key.values()))
        elif now is None:
            cols = list(was)
            db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                       [was[c] for c in cols])
        else:
            where = " AND ".join(f"{k} = ?" for k in key)
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in was)} WHERE {where}",
                       list(was.values()) + list(key.values()))
        audit(db, "undo", tbl, key, now, get(db, tbl, key), "undo: " + DETAIL)
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
        out, lines, refusals, applied = changes(db)
        if applied:
            print("applied already (the rows are as this tool writes them): nothing done")
            return 0
        if table(db, LOG):
            print(f"{LOG} is there but the rows are not as written: --undo first")
            return 1
        for line in lines:
            print(line)
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not a.apply:
            print(f"report only: {len(out)} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            n = apply(db, out)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {n} rows, in gm_audit and {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
