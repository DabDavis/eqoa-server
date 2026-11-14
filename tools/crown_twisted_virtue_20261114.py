#!/usr/bin/env python3
"""The Heirloom Crown of Twisted Virtue: Dust's red circlet as an item (the owner, 2026-11-14: "for dusts circlet make
it an actual item with that model we just made, named crown of twist virtue").

Named in the series of Dust's Heirloom Sword and Shield of Twisted Virtue (714382, 714383): "Heirloom Crown of Twisted
Virtue" (the owner typed "twist"; the series says "Twisted"). --apply (Heirloom: Dust is on no other shard):

1. itempattern, at the next free id after 714386 in the heirloom range: Dust's own Heirloom Helm (714043) copied whole
   (a helm, equipslot 1; no trade; its figures the heirlooms' rule: the best head piece his class wears at his level,
   as the Sword and Shield of Twisted Virtue are) with its own name, description, the crown icon of the game's crowns
   (1396793324, Crown of the Jaguar's), the series' crimson (3238989567) and model 0: a crown draws no helm (the game's
   crowns are all model 0), so the circlet, drawn while no helm is, shows.
2. heirlooms: its row as 714043's (class 3, role head): the scaling figures. Mercs and #template take each role's
   lowest id, 714043, so neither hands out the crown.
3. itemaura: kind 8 (circlet: database.ValidItemAura), source 0x1D875ECD (the Ascendant staff's ring), colour
   0xE01818FF, nodes 72 (72% up the head: the temples): anyone wearing it in the helm's slot shows the red circlet
   (entity.pieceAuras); hidden, none.
4. One into Dust's pack (location 0, the next list number).

His own circlet (charaura slot 5, the same look) is KEPT: it shows whatever he wears, so nothing changes for him
until he puts the crown on. Then --finish: his charaura circlet row deleted (the crown's own shows the same), and his
hidden-helm flag (charhiddenlooks bit 0, set for the Heirloom Helm) cleared, since a hidden crown shows nothing;
refused until the crown is worn in equiploc 0.

Checked as found, else REFUSED and nothing written. Report by default; each step's rows in one transaction, a gm_audit
line a row (as the panel's), logged in crown_twisted_virtue_1114_log; a second run finds nothing to do; --undo puts
back exactly what was there (the last step first; the crown taken out of his pack only while it is still there).

Needs a server with crown-twisted-virtue (itemaura kind 8; an older one drops the row: the crown would show nothing)
and a vanilla with circlet-height (65f229d) for the height. After --apply: the panel's "Reload every NPC" (the item
auras) and Dust's next login (his pack).

    python3 -E -s tools/crown_twisted_virtue_20261114.py --db eqoa.db [--apply | --finish | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "crown_twisted_virtue_1114_log"
DETAIL = "The Heirloom Crown of Twisted Virtue, Dust's red circlet as an item (the owner, 2026-11-14); tools/crown_twisted_virtue_20261114.py"
WHO, VIA = "dab", "tool"
NAME = "Dust"
HELM = 714043  # Heirloom Helm: the record copied
CROWN_NAME = "Heirloom Crown of Twisted Virtue"
CROWN_DESC = ("The Sword of Virtue's crown, turned: a ring of red light round the brow where a helm would sit. "
              "Heirloom gear for testing: always the best of its kind your class can wear at your level.")
ICON = 1396793324  # the game's crowns' (Crown of the Jaguar)
COLOR = 3238989567  # the Twisted Virtue series' crimson (714382, 714383)
IDS = (714387, 714999)
AURA = {"color": 0xE01818FF, "kind": 8, "source": 0x1D875ECD, "nodes": 72,
        "note": "The Heirloom Crown of Twisted Virtue: the monk staff's ring, red, at the temples (Dust's circlet), the owner 2026-11-14"}
PACK_CAP = 100

KEYS = {"itempattern": ("patternid",), "heirlooms": ("patternid",), "itemaura": ("patternid",), "charinventory": ("itemid",),
        "charaura": ("charid", "merc", "slot"), "charhiddenlooks": ("serverid",)}

LOG_DDL = f"""CREATE TABLE {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    step   TEXT NOT NULL,      -- apply or finish
    tbl    TEXT NOT NULL,
    rowkey TEXT NOT NULL,
    before TEXT,
    after  TEXT,
    audit  INTEGER NOT NULL
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


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
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key), go_json(before), go_json(after), DETAIL))
    return cur.lastrowid


def write(db, step, tbl, key, want):
    """Make tbl's row of key want (a whole row; None deletes it), audited and logged."""
    before = row(db, tbl, key)
    where = " AND ".join(f"{k} = ?" for k in key)
    if want is None:
        db.execute(f"DELETE FROM {tbl} WHERE {where}", tuple(key.values()))
        action = "delete"
    elif before is None:
        cols = list(want)
        db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' for _ in cols)})", tuple(want[c] for c in cols))
        action = "insert"
    else:
        sets = [c for c in want if c not in key]
        db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in sets)} WHERE {where}", tuple(want[c] for c in sets) + tuple(key.values()))
        action = "update"
    after = row(db, tbl, key)
    a = audit(db, action, tbl, key, before, after)
    if step:
        db.execute(f"INSERT INTO {LOG} (step, tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?, ?)",
                   (step, tbl, go_json(key), go_json(before), go_json(after), a))


def dust(db):
    chars = db.execute("SELECT serverid FROM characters WHERE charName = ?", (NAME,)).fetchall()
    return chars


def crown_id(db):
    r = db.execute("SELECT patternid FROM itempattern WHERE itemname = ? AND patternid BETWEEN ? AND ?", (CROWN_NAME, *IDS)).fetchone()
    return r[0] if r else None


def plan_apply(db):
    """(writes: [(tbl, key, want)], give: (sid, pid) or None, lines, refusals)."""
    for t in ("characters", "itempattern", "heirlooms", "itemaura", "charinventory", "gm_audit"):
        if not table(db, t):
            return [], None, [], [f"no {t} table here"]
    chars = dust(db)
    if not chars:
        return [], None, [f"no {NAME} on this shard: nothing for him here"], []
    if len(chars) != 1:
        return [], None, [], [f"{len(chars)} characters named {NAME}, want 1"]
    sid = chars[0][0]
    helm, heir = row(db, "itempattern", {"patternid": HELM}), row(db, "heirlooms", {"patternid": HELM})
    if helm is None or helm["equipslot"] != 1 or heir is None or heir["role"] != "head":
        return [], None, [], [f"itempattern {HELM} is not the Heirloom Helm of heirlooms' head ({helm and helm['itemname']}, {heir})"]
    lines, writes = [f"{NAME}: serverid {sid}"], []
    pid = crown_id(db)
    if pid is None:
        taken = {r[0] for r in db.execute("SELECT patternid FROM itempattern WHERE patternid BETWEEN ? AND ?", IDS)}
        pid = next((p for p in range(IDS[0], IDS[1] + 1) if p not in taken), None)
        if pid is None:
            return [], None, [], [f"no free itempattern id in {IDS[0]}-{IDS[1]}"]
        writes.append(("itempattern", {"patternid": pid},
                       dict(helm, patternid=pid, itemname=CROWN_NAME, itemdesc=CROWN_DESC, itemicon=ICON, color=COLOR, model=0)))
        lines.append(f"  itempattern {pid}: {CROWN_NAME}, a copy of {HELM} (Heirloom Helm), model 0 (no helm drawn), icon {ICON}")
    else:
        lines.append(f"  itempattern {pid}: {CROWN_NAME} there already")
    if row(db, "heirlooms", {"patternid": pid}) is None:
        writes.append(("heirlooms", {"patternid": pid}, dict(heir, patternid=pid)))
        lines.append(f"  heirlooms {pid}: class {heir['class']}, role head (the heirloom figures)")
    want = dict(AURA, patternid=pid)
    if row(db, "itemaura", {"patternid": pid}) != want:
        writes.append(("itemaura", {"patternid": pid}, want))
        lines.append(f"  itemaura {pid}: circlet (kind 8) of 0x1D875ECD, 0xE01818FF, 72% up the head")
    give = None
    if db.execute("SELECT 1 FROM charinventory WHERE serverid = ? AND patternid = ?", (sid, pid)).fetchone():
        lines.append(f"  {NAME} holds the crown already")
    else:
        held = db.execute("SELECT count(*) FROM charinventory WHERE serverid = ? AND location = 0", (sid,)).fetchone()[0]
        if held >= PACK_CAP:
            return [], None, lines, [f"{NAME}'s pack is full ({held})"]
        give = (sid, pid)
        lines.append(f"  {NAME}'s pack: one {CROWN_NAME} (his own circlet row kept until he wears it: --finish)")
    return writes, give, lines, []


def plan_finish(db):
    """(writes, lines, refusals)."""
    chars = dust(db)
    if len(chars) != 1:
        return [], [], [f"{len(chars)} characters named {NAME}, want 1"]
    sid = chars[0][0]
    pid = crown_id(db)
    worn = pid and db.execute("SELECT 1 FROM charinventory WHERE serverid = ? AND patternid = ? AND equiploc = 0", (sid, pid)).fetchone()
    if not worn:
        return [], [], [f"{NAME} does not wear the crown (equiploc 0) yet: --finish after he puts it on"]
    lines, writes = [f"{NAME}: serverid {sid}, wears {CROWN_NAME} ({pid})"], []
    if row(db, "charaura", {"charid": sid, "merc": 0, "slot": 5}) is not None:
        writes.append(("charaura", {"charid": sid, "merc": 0, "slot": 5}, None))
        lines.append("  charaura slot 5 (his own circlet): deleted, the crown's own shows")
    h = row(db, "charhiddenlooks", {"serverid": sid}) if table(db, "charhiddenlooks") else None
    if h is not None and h["locs"] & 1:
        writes.append(("charhiddenlooks", {"serverid": sid}, dict(h, locs=h["locs"] & ~1)))
        lines.append(f"  charhiddenlooks: helm slot shown again (locs {h['locs']} -> {h['locs'] & ~1}): a hidden crown shows nothing")
    return writes, lines, []


def undo(db):
    if not table(db, LOG):
        return 0, []
    back, left = 0, []
    for seq, tbl, rowkey, before, after in db.execute(f"SELECT seq, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key = json.loads(rowkey)
        now = row(db, tbl, key)
        if go_json(now) != after:
            left.append(f"{tbl} {rowkey}: changed since, left as it is")
            continue
        if tbl == "itempattern" and before is None and \
                db.execute("SELECT count(*) FROM charinventory WHERE patternid = ?", (key["patternid"],)).fetchone()[0]:
            left.append(f"itempattern {key['patternid']}: still held, left")
            continue
        write(db, None, tbl, key, json.loads(before) if before else None)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if not left:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--finish", action="store_true")
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
        if a.finish:
            writes, lines, refusals = plan_finish(db)
            give = None
        else:
            writes, give, lines, refusals = plan_apply(db)
        for line in lines:
            print(line)
        if refusals:
            db.execute("ROLLBACK")
            print("REFUSED, nothing written:\n  " + "\n  ".join(refusals))
            return 1
        if not writes and give is None:
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if not (a.apply or a.finish):
            db.execute("ROLLBACK")
            print(f"(report only: {len(writes) + (give is not None)} rows; --apply to write)")
            return 0
        if not table(db, LOG):
            db.execute(LOG_DDL)
        step = "finish" if a.finish else "apply"
        for tbl, key, want in writes:
            write(db, step, tbl, key, want)
        if give:
            sid, pid = give
            cur = db.execute(
                "INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber) "
                "VALUES (?, 1, (SELECT COALESCE(maxhp, 0) FROM itempattern WHERE patternid = ?), 0, ?, -1, 0, "
                "(SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory WHERE serverid = ? AND location = 0))",
                (sid, pid, pid, sid))
            key = {"itemid": cur.lastrowid}
            after = row(db, "charinventory", key)
            n = audit(db, "insert", "charinventory", key, None, after)
            db.execute(f"INSERT INTO {LOG} (step, tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?, ?)",
                       (step, "charinventory", go_json(key), None, go_json(after), n))
        db.execute("COMMIT")
        print(f"{step}: {len(writes) + (give is not None)} rows, in gm_audit and {LOG}; press the GM panel's Reload every NPC"
              + ("; Dust sees the crown in his pack at his next login (eject him to apply now)" if give else ""))
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
