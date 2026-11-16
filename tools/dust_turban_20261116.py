#!/usr/bin/env python3
"""The Turban of Twisted Virtue: the efreeti lord's turban as an item for Dust (the owner, 2026-11-16: "lets also cut
the turban from the efreeti lord as a turban for dust in his inventory, turban of twisted virtue").

The turban is part of the efreeti lord's body (CHAR.ESF 0xAA489CBB, Efreeti Lord Djarn's), cut out and worn on the head
in place of the helm by vanilla's hat.rs (aura kind 10, hat). Named in the series of Dust's Heirloom Sword, Shield and
Crown of Twisted Virtue (714382, 714383, 714387). Heirloom: Dust is on no other shard.

--apply, the record (each row audited, logged):
1. itempattern, at the next free id after 714387 in the heirloom range: Dust's own Heirloom Helm (714043) copied whole
   (a helm, equipslot 1; no trade; its figures the heirlooms' rule: the best head piece his class wears at his level, as
   the Sword, Shield and Crown of Twisted Virtue are), with its own name and description, LORE (one a character), the
   game's dark turban icon (864498561, Starless Night Turban's), the series' crimson (3238989567) and model 1: the helm
   a PS2 (and a vanilla before the hat) shows for it, the cloth helm the game's own turbans mostly wear.
2. heirlooms: its row as 714043's (class 3, role head): the scaling figures. Mercs and #template take each role's
   lowest id, 714043, so neither hands out the turban.
3. itemaura: kind 10 (hat: database.AuraHat, server branch dust-turban), source 0xAA489CBB, colour 0 (the turban's own
   colours): anyone wearing it in the helm's slot shows the turban (entity.pieceAuras); hidden, none.
4. itemowner: Dust (serverid) alone may wear it.

--give: one into Dust's pack (location 0, the next list number); refused while he holds one or before --apply.

Checked as found, else REFUSED and nothing written. Report by default; a second run finds nothing to do; --undo puts
back exactly what was there, the last write first, all or nothing: refused, nothing changed, while any row written has
changed since (the turban worn or moved: take it off into his pack first) or another copy of the record is held.

Needs a server with dust-turban (itemaura kind 10; an older one drops the row: the turban shows its fallback helm) and a
vanilla with the hat (hoofbite-look). After --apply: the panel's "Reload every NPC" (the item auras); after --give,
Dust's next login (his pack).

    python3 -E -s tools/dust_turban_20261116.py --db eqoa.db [--apply | --give | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "dust_turban_1116_log"
DETAIL = "The Turban of Twisted Virtue, the efreeti lord's turban as an item for Dust (the owner, 2026-11-16); tools/dust_turban_20261116.py"
WHO, VIA = "dab", "tool"
NAME = "Dust"
HELM = 714043  # Heirloom Helm: the record copied
TURBAN_NAME = "Turban of Twisted Virtue"
TURBAN_DESC = ("An efreeti lord's turban, unwound from the desert's fire and wound again round a fallen knight's brow: "
               "what was virtue, twisted. Heirloom gear for testing: always the best of its kind your class can wear at your level.")
ICON = 864498561  # the game's dark turban (Starless Night Turban)
COLOR = 3238989567  # the Twisted Virtue series' crimson (714382, 714383, 714387)
MODEL = 1  # the helm shown where the hat is not drawn (a PS2, an older vanilla)
IDS = (714388, 714999)
EFREETI_LORD = 0xAA489CBB
AURA = {"color": 0, "kind": 10, "source": EFREETI_LORD, "nodes": 0,
        "note": "The Turban of Twisted Virtue: the efreeti lord's turban (0xAA489CBB) worn in place of the helm, the owner 2026-11-16"}
PACK_CAP = 100

LOG_DDL = f"""CREATE TABLE {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    step   TEXT NOT NULL,      -- apply or give
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
    return db.execute("SELECT serverid FROM characters WHERE charName = ?", (NAME,)).fetchall()


def turban_id(db):
    r = db.execute("SELECT patternid FROM itempattern WHERE itemname = ? AND patternid BETWEEN ? AND ?", (TURBAN_NAME, *IDS)).fetchone()
    return r[0] if r else None


def plan_apply(db):
    """(writes: [(tbl, key, want)], lines, refusals)."""
    for t in ("characters", "itempattern", "heirlooms", "itemaura", "itemowner", "charinventory", "gm_audit"):
        if not table(db, t):
            return [], [], [f"no {t} table here"]
    chars = dust(db)
    if not chars:
        return [], [f"no {NAME} on this shard: nothing for him here"], []
    if len(chars) != 1:
        return [], [], [f"{len(chars)} characters named {NAME}, want 1"]
    sid = chars[0][0]
    helm, heir = row(db, "itempattern", {"patternid": HELM}), row(db, "heirlooms", {"patternid": HELM})
    if helm is None or helm["equipslot"] != 1 or heir is None or heir["role"] != "head":
        return [], [], [f"itempattern {HELM} is not the Heirloom Helm of heirlooms' head ({helm and helm['itemname']}, {heir})"]
    lines, writes = [f"{NAME}: serverid {sid}"], []
    pid = turban_id(db)
    if pid is None:
        taken = {r[0] for r in db.execute("SELECT patternid FROM itempattern WHERE patternid BETWEEN ? AND ?", IDS)}
        pid = next((p for p in range(IDS[0], IDS[1] + 1) if p not in taken), None)
        if pid is None:
            return [], [], [f"no free itempattern id in {IDS[0]}-{IDS[1]}"]
        for t in ("heirlooms", "itemaura", "itemowner"):
            if row(db, t, {"patternid": pid}) is not None:
                return [], [], [f"{t} has a row for the free id {pid} already: left, look first"]
        writes.append(("itempattern", {"patternid": pid},
                       dict(helm, patternid=pid, itemname=TURBAN_NAME, itemdesc=TURBAN_DESC, itemicon=ICON, color=COLOR,
                            model=MODEL, lore=1)))
        lines.append(f"  itempattern {pid}: {TURBAN_NAME}, a copy of {HELM} (Heirloom Helm), lore, model {MODEL} "
                     f"(the helm a PS2 shows), icon {ICON}")
    else:
        lines.append(f"  itempattern {pid}: {TURBAN_NAME} there already")
    if row(db, "heirlooms", {"patternid": pid}) is None:
        writes.append(("heirlooms", {"patternid": pid}, dict(heir, patternid=pid)))
        lines.append(f"  heirlooms {pid}: class {heir['class']}, role head (the heirloom figures)")
    want = dict(AURA, patternid=pid)
    have = row(db, "itemaura", {"patternid": pid})
    if have is None or any(have.get(k) != v for k, v in want.items()):
        writes.append(("itemaura", {"patternid": pid}, want))
        lines.append(f"  itemaura {pid}: hat (kind 10) of 0x{EFREETI_LORD:08X}, its own colours")
    own = row(db, "itemowner", {"patternid": pid})
    if own is None:
        writes.append(("itemowner", {"patternid": pid}, {"patternid": pid, "serverid": sid,
                                                        "note": f"{TURBAN_NAME}: {NAME}'s alone, the owner 2026-11-16"}))
        lines.append(f"  itemowner {pid}: {NAME} ({sid}) alone")
    elif own["serverid"] != sid:
        return [], lines, [f"itemowner {pid} names serverid {own['serverid']}, not {NAME}'s {sid}"]
    return writes, lines, []


def plan_give(db):
    """((sid, pid) or None, lines, refusals)."""
    chars = dust(db)
    if len(chars) != 1:
        return None, [], [f"{len(chars)} characters named {NAME}, want 1"]
    sid = chars[0][0]
    pid = turban_id(db)
    if pid is None or row(db, "itemowner", {"patternid": pid}) is None:
        return None, [], [f"no {TURBAN_NAME} record yet: --apply first"]
    lines = [f"{NAME}: serverid {sid}; {TURBAN_NAME} {pid}"]
    if db.execute("SELECT 1 FROM charinventory WHERE serverid = ? AND patternid = ?", (sid, pid)).fetchone():
        lines.append(f"  {NAME} holds the turban already (lore: one a character)")
        return None, lines, []
    held = db.execute("SELECT count(*) FROM charinventory WHERE serverid = ? AND location = 0", (sid,)).fetchone()[0]
    if held >= PACK_CAP:
        return None, lines, [f"{NAME}'s pack is full ({held})"]
    lines.append(f"  {NAME}'s pack: one {TURBAN_NAME} ({held} things there now)")
    return (sid, pid), lines, []


def undo(db):
    """(back, left: [why]). All or nothing: when any row written has changed since (the turban worn, moved, its record
    edited) or a copy --give did not make is held, nothing is undone and each reason is listed."""
    if not table(db, LOG):
        return 0, []
    logged = db.execute(f"SELECT seq, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall()
    left = []
    given = set()
    for _, tbl, rowkey, _, after in logged:
        key = json.loads(rowkey)
        if go_json(row(db, tbl, key)) != after:
            left.append(f"{tbl} {rowkey}: changed since (worn or moved?), nothing undone")
        if tbl == "charinventory":
            given.add(key["itemid"])
    for _, tbl, rowkey, before, _ in logged:
        if tbl == "itempattern" and before is None:
            pid = json.loads(rowkey)["patternid"]
            held = {r[0] for r in db.execute("SELECT itemid FROM charinventory WHERE patternid = ?", (pid,))} - given
            if held:
                left.append(f"itempattern {pid}: other copies held (itemids {sorted(held)}), nothing undone")
    if left:
        return 0, left
    for seq, tbl, rowkey, before, _ in logged:
        write(db, None, tbl, json.loads(rowkey), json.loads(before) if before else None)
    db.execute(f"DROP TABLE {LOG}")
    return len(logged), []


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--give", action="store_true")
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
        give = None
        if a.give:
            give, lines, refusals = plan_give(db)
            writes = []
        else:
            writes, lines, refusals = plan_apply(db)
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
        if not (a.apply or a.give):
            db.execute("ROLLBACK")
            print(f"(report only: {len(writes)} rows; --apply to write the record, then --give for his pack)")
            return 0
        if not table(db, LOG):
            db.execute(LOG_DDL)
        step = "give" if a.give else "apply"
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
        if give:
            print(f"give: one {TURBAN_NAME} in {NAME}'s pack, in gm_audit and {LOG}; he sees it at his next login (eject him to apply now)")
        else:
            print(f"apply: {len(writes)} rows, in gm_audit and {LOG}; press the GM panel's Reload every NPC; then --give")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
