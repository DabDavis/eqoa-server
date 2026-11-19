#!/usr/bin/env python3
"""Move every character's hand aura onto the weapon COPY it holds (the owner, 2026-11-16: "for weapons with aura they
can be marked for an aura though they are the same weapon as other players, it will have a tag 'Aura: Virtuous' for
example. so it doesnt just stick to all his weps"; then "do that for all that have the auras").

1. Hand auras: each charaura row on a hand (slot 9 the main hand, 10 the off hand; a mercenary's, merc > 0, too) is
   moved onto the copy that character (or merc) holds in that hand now, as the server reads it (handlers/character.go
   wornLook: the main hand equiploc 16, 14, 19; the off hand 15, 20, 18, then a shield 17; a merc's in charmercitem,
   its key -id): an instanceaura row with the row's first aura, its second (charaura's color2 ... nodes2 where the
   table has them), and a tag named from the aura (TAGS). Then the charaura row is deleted. A GM (197): his twinkle
   (kind 3, 1D875ECD, along 100) goes in as his copy's second where his row carries none (tools/dust_twinkle_20261116.py
   not yet applied).
2. A hand that holds nothing of its own (empty, or a merc's lent heirloom) keeps its charaura row: listed.
3. A copy that has an instanceaura row already is left as it is (and its charaura row kept): listed.
4. Item-record auras (itemaura) of a held item's kinds (3 particles, 7 epic, 9 energy): a record held by ONE character
   (its copies, a merc's too) stays the record's aura, and each of its copies is given a TAG-ONLY instanceaura row (the
   record's aura still shows); a record held by more than one is listed for the owner and given nothing.

Refused (nothing written) without tables charaura, charinventory, itempattern, gm_audit, or without instanceaura (a
server of branch instance-aura makes it at its start; --make-table makes it here, for a rehearsal on a copy). Report by
default; --apply in one transaction, a gm_audit line per row (insert/delete, the whole row before and after, as the
panel's), and instance_aura_migrate_1116_log; --undo puts back exactly what was there (the charaura rows, the
instanceaura rows taken out), a row changed since left and listed.

The server reads charaura and instanceaura at its start and on the GM panel's "Reload every NPC": press it after --apply
(or --undo). The tags show on the items' next record (the holder's next login).

    python3 -E -s tools/instance_aura_migrate_20261116.py --db eqoa.db [--apply | --undo] [--make-table]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "instance_aura_migrate_1116_log"
SRC = "instance_aura_migrate_20261116"
DETAIL = "hand auras onto the held weapon copies, tagged (the owner, 2026-11-16); tools/instance_aura_migrate_20261116.py"
WHO, VIA = "dab", "tool"
HANDS = {9: [16, 14, 19], 10: [15, 20, 18, 17]}  # database.CharAuraMainHand/OffHand: wornLook's equip slots, in order
HAND_NAMES = {9: "main hand", 10: "off hand"}
HAND_KINDS = {3, 7, 9}  # database.HandAuraKind
DUST, TWINKLE = (197, "Dust"), (0, 3, 0x1D875ECD, 100)  # (colour, kind, source, nodes)

INSTANCE_DDL = """CREATE TABLE IF NOT EXISTS instanceaura (
    itemid  INTEGER PRIMARY KEY,
    tag     TEXT NOT NULL DEFAULT '',
    color   INTEGER NOT NULL DEFAULT 0,
    kind    INTEGER NOT NULL DEFAULT 0,
    source  INTEGER NOT NULL DEFAULT 0,
    nodes   INTEGER NOT NULL DEFAULT 0,
    color2  INTEGER NOT NULL DEFAULT 0,
    kind2   INTEGER NOT NULL DEFAULT 0,
    source2 INTEGER NOT NULL DEFAULT 0,
    nodes2  INTEGER NOT NULL DEFAULT 0,
    note    TEXT NOT NULL DEFAULT '',
    source_tool TEXT NOT NULL DEFAULT ''
)"""

LOG_DDL = f"""CREATE TABLE {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,     -- charaura (deleted) or instanceaura (inserted)
    rowkey  TEXT NOT NULL,     -- JSON
    before  TEXT,              -- the row before (JSON), NULL none
    after   TEXT,              -- the row written (JSON), NULL deleted
    audit   INTEGER NOT NULL
)"""


def u32(v):
    return (v or 0) & 0xFFFFFFFF


def dark(c):
    """database.AuraDark: an alpha, and no RGB channel past 0x30."""
    c = u32(c)
    return c & 0xFF != 0 and all(((c >> s) & 0xFF) <= 0x30 for s in (24, 16, 8))


def tag_word(colour, kind, source):
    """The tag's word for an aura (the Weapon Enchanter's catalog names, handlers/enchanter.go), None for none."""
    kind, source = kind or 0, u32(source)
    if kind not in HAND_KINDS:
        return None
    if kind == 3 and dark(colour) and source in (0, 0x88EC092D):
        return "Shadowed"
    if source in (0, 0x88EC092D):
        return "Virtuous"
    return {0x6C151A60: "Freezing", 0x5FD4AE7C: "Tainted", 0x1D875ECD: "Twinkling", 0xCBF4135A: "Druid Glow",
            0x2EC56029: "Energized"}.get(source, "Energized" if kind == 9 else "Enchanted")


def tag_of(first, second):
    words = []
    for a in (first, second):
        w = tag_word(*a[:3]) if a else None
        if w and w not in words:
            words.append(w)
    return ", ".join(words)[:24]


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def columns(db, name):
    return {r[1] for r in db.execute(f"PRAGMA table_info({name})")}


def one(db, sql, args):
    cur = db.execute(sql, args)
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return None if m is None else json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, tbl, key, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key), go_json(before), go_json(after), DETAIL))
    return cur.lastrowid


def name_of(db, sid):
    r = db.execute("SELECT charName FROM characters WHERE serverid = ?", (sid,)).fetchone()
    return r[0] if r else f"#{sid}"


def held_copy(db, sid, merc, slot):
    """(key, patternid, itemname) of the copy held in the hand, None nothing of its own."""
    for loc in HANDS[slot]:
        if merc == 0:
            r = db.execute("SELECT c.itemid, p.patternid, p.itemname FROM charinventory c JOIN itempattern p ON p.patternid = c.patternid "
                           "WHERE c.serverid = ? AND c.equiploc = ? AND c.location = 0 ORDER BY c.itemid LIMIT 1", (sid, loc)).fetchone()
        elif table(db, "charmercitem"):
            r = db.execute("SELECT -m.id, p.patternid, p.itemname FROM charmercitem m JOIN itempattern p ON p.patternid = m.patternid "
                           "WHERE m.serverid = ? AND m.equiploc = ? AND m.slot = ? ORDER BY m.id LIMIT 1", (sid, loc, merc)).fetchone()
        else:
            r = None
        if r:
            return r
    return None


def plan(db):
    """(moves, tag_rows, lines, refusals). moves: (charaura row, instanceaura row); tag_rows: instanceaura rows."""
    refusals, lines, moves, tags = [], [], [], []
    for t in ("charaura", "charinventory", "itempattern", "gm_audit", "characters"):
        if not table(db, t):
            refusals.append(f"no {t} table here")
    if not table(db, "instanceaura"):
        refusals.append("no instanceaura table: start a server of branch instance-aura on this database first (or --make-table on a copy)")
    if refusals:
        return moves, tags, lines, refusals
    second = all(c in columns(db, "charaura") for c in ("color2", "kind2", "source2", "nodes2"))
    lines.append(f"charaura {'has' if second else 'has NO'} second-aura columns (charaura-two)")
    taken = set()
    lines.append("1. hand auras:")
    cur = db.execute("SELECT * FROM charaura WHERE slot IN (9, 10) ORDER BY charid, merc, slot")
    cols = [d[0] for d in cur.description]
    for r in [dict(zip(cols, x)) for x in cur.fetchall()]:
        sid, merc, slot = r["charid"], r["merc"], r["slot"]
        who = name_of(db, sid) + (f"'s merc {merc}" if merc else "") + f" ({sid})"
        first = (r["color"], r["kind"], r["source"], r["nodes"])
        sec = (r["color2"], r["kind2"], r["source2"], r["nodes2"]) if second and r.get("kind2") else None
        if (sid, merc, slot) == (DUST[0], 0, 9) and name_of(db, sid) == DUST[1] and sec is None:
            sec = TWINKLE  # tonight's twinkle, stacked on his copy directly
        if first[1] not in HAND_KINDS:
            first = (0, 0, 0, 0)
        h = held_copy(db, sid, merc, slot)
        words = f"colour {u32(first[0]):08X} kind {first[1]} source {u32(first[2]):08X} along {first[3]}" + \
                (f"; second kind {sec[1]} source {u32(sec[2]):08X} along {sec[3]}" if sec else "")
        if h is None:
            lines.append(f"   KEPT  {who} {HAND_NAMES[slot]}: holds nothing of its own; charaura row kept ({words})")
            continue
        key, pid, iname = h
        if one(db, "SELECT itemid FROM instanceaura WHERE itemid = ?", (key,)) or key in taken:
            lines.append(f"   KEPT  {who} {HAND_NAMES[slot]}: {iname} (copy {key}) has an instanceaura row already; charaura row kept")
            continue
        taken.add(key)
        tag = tag_of(first if first[1] else None, sec)
        inst = {"itemid": key, "tag": tag, "color": first[0], "kind": first[1], "source": first[2], "nodes": first[3],
                "color2": sec[0] if sec else 0, "kind2": sec[1] if sec else 0, "source2": sec[2] if sec else 0,
                "nodes2": sec[3] if sec else 0, "note": f"{who}'s {HAND_NAMES[slot]} aura, moved onto its copy: {r.get('note') or ''}".strip()[:300],
                "source_tool": SRC}
        moves.append((r, inst))
        lines.append(f"   MOVE  {who} {HAND_NAMES[slot]}: {iname} (record {pid}, copy {key}) tag \"Aura: {tag}\" ({words})")
    lines.append("4. item-record auras on held items:")
    cur = db.execute("SELECT a.*, p.itemname FROM itemaura a JOIN itempattern p USING (patternid) ORDER BY a.patternid")
    cols = [d[0] for d in cur.description]
    for a in [dict(zip(cols, x)) for x in cur.fetchall()]:
        if a["kind"] not in HAND_KINDS:
            continue
        copies = db.execute("SELECT itemid, serverid FROM charinventory WHERE patternid = ?", (a["patternid"],)).fetchall()
        if table(db, "charmercitem"):
            copies += db.execute("SELECT -id, serverid FROM charmercitem WHERE patternid = ?", (a["patternid"],)).fetchall()
        holders = sorted({s for _, s in copies})
        sec = (a.get("color2"), a.get("kind2"), a.get("source2"), a.get("nodes2")) if a.get("kind2") else None
        tag = tag_of((a["color"], a["kind"], a["source"]), sec)
        if len(holders) > 1:
            lines.append(f"   SHARED {a['itemname']} ({a['patternid']}): held by {len(holders)} characters "
                         f"({', '.join(name_of(db, s) for s in holders[:8])}): for the owner to decide; nothing written")
            continue
        if not holders:
            lines.append(f"   NONE  {a['itemname']} ({a['patternid']}): no copy held; nothing written")
            continue
        for key, sid in copies:
            if one(db, "SELECT itemid FROM instanceaura WHERE itemid = ?", (key,)) or key in taken:
                lines.append(f"   KEPT  {a['itemname']} copy {key} ({name_of(db, sid)}): an instanceaura row already")
                continue
            taken.add(key)
            tags.append({"itemid": key, "tag": tag, "color": 0, "kind": 0, "source": 0, "nodes": 0, "color2": 0, "kind2": 0,
                         "source2": 0, "nodes2": 0, "note": f"{a['itemname']}'s record aura, its tag on {name_of(db, sid)}'s copy",
                         "source_tool": SRC})
            lines.append(f"   TAG   {a['itemname']} ({a['patternid']}) copy {key}, {name_of(db, sid)}'s: \"Aura: {tag}\" (the record's aura kept)")
    return moves, tags, lines, refusals


def insert_inst(db, inst):
    cols = list(inst)
    db.execute(f"INSERT INTO instanceaura ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [inst[c] for c in cols])
    after = one(db, "SELECT * FROM instanceaura WHERE itemid = ?", (inst["itemid"],))
    key = {"itemid": inst["itemid"]}
    a = audit(db, "insert", "instanceaura", key, None, after)
    db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES ('instanceaura', ?, NULL, ?, ?)", (go_json(key), go_json(after), a))


def apply(db, moves, tags):
    db.execute(LOG_DDL)
    for r, inst in moves:
        insert_inst(db, inst)
        key = {"charid": r["charid"], "merc": r["merc"], "slot": r["slot"]}
        db.execute("DELETE FROM charaura WHERE charid = ? AND merc = ? AND slot = ?", (r["charid"], r["merc"], r["slot"]))
        a = audit(db, "delete", "charaura", key, r, None)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES ('charaura', ?, ?, NULL, ?)", (go_json(key), go_json(r), a))
    for inst in tags:
        insert_inst(db, inst)


def undo(db):
    """(back, left: [why])."""
    if not table(db, LOG):
        return 0, []
    back, left = 0, []
    for tbl, rowkey, before, after in db.execute(f"SELECT tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        k = json.loads(rowkey)
        if tbl == "instanceaura":
            now = one(db, "SELECT * FROM instanceaura WHERE itemid = ?", (k["itemid"],))
            if go_json(now) != after:
                left.append(f"instanceaura {k['itemid']}: changed since, left as it is")
                continue
            db.execute("DELETE FROM instanceaura WHERE itemid = ?", (k["itemid"],))
            audit(db, "delete", "instanceaura", k, now, None)
        else:
            if one(db, "SELECT * FROM charaura WHERE charid = ? AND merc = ? AND slot = ?", (k["charid"], k["merc"], k["slot"])):
                left.append(f"charaura {k}: a row there since, left as it is")
                continue
            b = json.loads(before)
            cols = [c for c in b if c in columns(db, "charaura")]
            db.execute(f"INSERT INTO charaura ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [b[c] for c in cols])
            audit(db, "insert", "charaura", k, None, one(db, "SELECT * FROM charaura WHERE charid = ? AND merc = ? AND slot = ?",
                                                       (k["charid"], k["merc"], k["slot"])))
        back += 1
    if not left:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--make-table", action="store_true", help="make instanceaura here (a rehearsal on a copy)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"undone: {back} rows back" + "".join(f"\n  left: {w}" for w in left) +
                  ("; press the GM panel's Reload every NPC" if back else ""))
            return 0 if not left else 1
        if a.make_table:
            db.execute(INSTANCE_DDL)
        moves, tags, lines, refusals = plan(db)
        for line in lines:
            print(line)
        if refusals:
            db.execute("ROLLBACK")
            print("REFUSED, nothing written:\n  " + "\n  ".join(refusals))
            return 1
        if not moves and not tags:
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if table(db, LOG):
            db.execute("ROLLBACK")
            print(f"applied before ({LOG} is there): --undo first")
            return 1
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"(report only: {len(moves)} moved, {len(tags)} tagged; --apply to write, then the GM panel's Reload every NPC)")
            return 0
        apply(db, moves, tags)
        db.execute("COMMIT")
        print(f"applied: {len(moves)} moved, {len(tags)} tagged, in gm_audit and {LOG}; press the GM panel's Reload every NPC (or restart)")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
