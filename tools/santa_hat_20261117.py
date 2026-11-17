#!/usr/bin/env python3
"""The Santa Hat (the owner, 2026-11-17: "for santa clause lets cut the gnome cap to be usuable for all and make it red
and white with a white ball on the end").

The hat is the dressed gnome man's red pointed cap (CHAR.ESF #139, 0xA20590E3, database.GnomeCap), cut out and worn in
place of the helm by vanilla's hat.rs, dressed as Santa's (a white band, a white ball at the tip: aura nodes
database.HatSanta = 1) and fitted to any body, illusions too. On every shard (run it on each database). --apply, each
row audited (gm_audit, as the panel's) and logged in santa_hat_1117_log:

1. itempattern "Santa Hat", at the next free id from 716913 (the Christmas items' range, after Santa's Elf 716912):
   the Tattered Cap (6428: level 1, every class, AC 1) copied, with its own name and description, every race
   (32767), tradeable (trade 0), not lore, red (0xC01818FF: a PS2 and an older vanilla show the record's cloth
   cap, model 1, in red; vanilla dyes the cap's red in the helm's dye and keeps the white).
2. itemaura on it: kind 10 (hat), source 0xA20590E3, nodes 1 (Santa's), colour 0: worn in the helm's slot it is
   drawn (entity.pieceAuras), hidden none.
3. npctint "Santa Claus": aura kind 10, source 0xA20590E3, nodes 1 (server branch santa-hat: database.ValidNPCAuraKind);
   a row there already keeps its colour and glow (its aura columns logged and put back by --undo).
4. tokenshop: Toymaker Tinsel sells it for 27 Holly Tokens (half Santa's Elf's 54), sort 4, source santa-hat.

Checked as found, else REFUSED and nothing written: the Tattered Cap a level-1 helm; Toymaker Tinsel's shop there
(tokenshop rows of that shop); no other record named Santa Hat outside the range. Report by default; a second --apply
finds nothing to do; --undo puts back exactly what was there, all or nothing: refused while any character, bank or
merc holds a Santa Hat, or a row written has changed since.

Needs a server with santa-hat (an older one drops npctint's kind 10: Santa shows no hat; the item's hat shows on any
server with dust-turban). After --apply: the panel's "Reload every NPC" (item auras, NPC tints); the token shop reads
its stock at each visit.

    python3 -E -s tools/santa_hat_20261117.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "santa_hat_1117_log"
DETAIL = "The Santa Hat: the gnome's cap as Santa's, for all, on Santa Claus and in Toymaker Tinsel's shop (the owner, 2026-11-17); tools/santa_hat_20261117.py"
WHO, VIA = "dab", "tool"
NAME = "Santa Hat"
DESC = ("Santa's own pattern: a red cap with a white band and a white ball at its tip, cut to fit any head, "
        "from a gnome's to an ogre's. Merry Christmas from Toymaker Tinsel.")
BASE = 6428  # Tattered Cap
IDS = (716913, 716999)
RED = 0xC01818FF
GNOME_CAP, HAT, SANTA = 0xA20590E3, 10, 1
SANTA_NPC = "Santa Claus"
SHOP, PRICE, SORT = "Toymaker Tinsel", 27, 4
SAYS = "Santa's own hat, red with a white band and a bobble on the end. It fits any head, gnome or ogre, and takes a dye."

LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
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
    return db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                      "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                      (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key), go_json(before), go_json(after), DETAIL)).lastrowid


def write(db, log, tbl, key, want):
    """Make tbl's row of key want (a whole row; None deletes it), audited (and logged when log)."""
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
    if log:
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                   (tbl, go_json(key), go_json(before), go_json(after), a))


def hat_id(db):
    r = db.execute("SELECT patternid FROM itempattern WHERE itemname = ? AND patternid BETWEEN ? AND ?", (NAME, *IDS)).fetchone()
    return r[0] if r else None


def plan(db):
    """(writes [(tbl, key, want)], lines, refusals)."""
    for t in ("itempattern", "itemaura", "npctint", "tokenshop", "gm_audit"):
        if not table(db, t):
            return [], [], [f"no {t} table here"]
    base = row(db, "itempattern", {"patternid": BASE})
    if base is None or base["equipslot"] != 1 or base["levelreq"] != 1:
        return [], [], [f"itempattern {BASE} is not the level-1 Tattered Cap ({base and base['itemname']})"]
    if not db.execute("SELECT 1 FROM tokenshop WHERE shop = ?", (SHOP,)).fetchone():
        return [], [], [f"no tokenshop rows for {SHOP} here: the Christmas data first"]
    lines, writes = [], []
    pid = hat_id(db)
    if pid is None:
        taken = {r[0] for r in db.execute("SELECT patternid FROM itempattern WHERE patternid BETWEEN ? AND ?", IDS)}
        pid = next((p for p in range(IDS[0], IDS[1] + 1) if p not in taken), None)
        if pid is None:
            return [], [], [f"no free itempattern id in {IDS[0]}-{IDS[1]}"]
        if row(db, "itemaura", {"patternid": pid}) is not None:
            return [], [], [f"itemaura has a row for the free id {pid}: look first"]
        writes.append(("itempattern", {"patternid": pid},
                       dict(base, patternid=pid, itemname=NAME, itemdesc=DESC, model=1, color=RED, trade=0, lore=0, raceuse=32767,
                            classuse=32767, levelreq=1)))
        lines.append(f"itempattern {pid}: {NAME}, a copy of {BASE} (Tattered Cap): every class and race, tradeable, model 1 red")
    else:
        lines.append(f"itempattern {pid}: {NAME} there already")
    want = {"patternid": pid, "color": 0, "kind": HAT, "source": GNOME_CAP, "nodes": SANTA,
            "note": "The Santa Hat: the gnome's cap (0xA20590E3) as Santa's, the owner 2026-11-17"}
    have = row(db, "itemaura", {"patternid": pid})
    if have is None or any(have.get(k) != v for k, v in want.items()):
        writes.append(("itemaura", {"patternid": pid}, want))
        lines.append(f"itemaura {pid}: hat (kind 10) of 0x{GNOME_CAP:08X}, Santa's (nodes 1)")
    t = db.execute("SELECT name FROM npctint WHERE lower(trim(name)) = lower(?)", (SANTA_NPC,)).fetchall()
    if len(t) > 1:
        return [], lines, [f"{len(t)} npctint rows for {SANTA_NPC}: look first"]
    if t:
        cur = row(db, "npctint", {"name": t[0][0]})
        if (cur["aura_kind"], cur["aura_source"] & 0xFFFFFFFF, cur["aura_nodes"]) != (HAT, GNOME_CAP, SANTA):
            writes.append(("npctint", {"name": t[0][0]}, dict(cur, aura_kind=HAT, aura_source=GNOME_CAP, aura_nodes=SANTA)))
            lines.append(f"npctint {t[0][0]!r}: its tint and glow kept, the Santa Hat on its head")
    else:
        cols = {r[1] for r in db.execute("PRAGMA table_info(npctint)")}
        want = {"name": SANTA_NPC, "color": 0, "glow": 0, "aura_kind": HAT, "aura_source": GNOME_CAP, "aura_nodes": SANTA}
        if "note" in cols:
            want["note"] = "Santa Claus wears the Santa Hat (the owner 2026-11-17)"
        if "source" in cols:
            want["source"] = "santa-hat"
        writes.append(("npctint", {"name": SANTA_NPC}, want))
        lines.append(f"npctint {SANTA_NPC!r}: no tint, the Santa Hat on its head")
    shop = row(db, "tokenshop", {"shop": SHOP, "patternid": pid})
    if shop is None:
        writes.append(("tokenshop", {"shop": SHOP, "patternid": pid},
                       {"shop": SHOP, "patternid": pid, "price": PRICE, "token": "", "says": SAYS, "sort": SORT, "source": "santa-hat"}))
        lines.append(f"tokenshop: {SHOP} sells it for {PRICE} Holly Tokens")
    return writes, lines, []


def held(db, pid):
    n = 0
    for tbl in ("charinventory", "accountbank", "charmercitem", "auction_lots"):
        if table(db, tbl) and "patternid" in {r[1] for r in db.execute(f"PRAGMA table_info({tbl})")}:
            n += db.execute(f"SELECT count(*) FROM {tbl} WHERE patternid = ?", (pid,)).fetchone()[0]
    return n


def undo(db):
    if not table(db, LOG):
        return 0, []
    logged = db.execute(f"SELECT seq, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall()
    left = []
    for _, tbl, rowkey, before, after in logged:
        if go_json(row(db, tbl, json.loads(rowkey))) != after:
            left.append(f"{tbl} {rowkey}: changed since, nothing undone")
        if tbl == "itempattern" and before is None:
            n = held(db, json.loads(rowkey)["patternid"])
            if n:
                left.append(f"{n} Santa Hats are held (packs, banks, mercs, auctions): nothing undone")
    if left:
        return 0, left
    for _, tbl, rowkey, before, _ in logged:
        write(db, False, tbl, json.loads(rowkey), json.loads(before) if before else None)
    db.execute(f"DROP TABLE {LOG}")
    return len(logged), []


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
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"undone: {back} rows back" + "".join(f"\n  left: {w}" for w in left) +
                  ("; press the GM panel's Reload every NPC" if back else ""))
            return 0 if not left else 1
        writes, lines, refusals = plan(db)
        for line in lines:
            print(line)
        if refusals:
            db.execute("ROLLBACK")
            print("REFUSED, nothing written:\n  " + "\n  ".join(refusals))
            return 1
        if not writes:
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"(report only: {len(writes)} rows; --apply to write)")
            return 0
        db.execute(LOG_DDL)
        for tbl, key, want in writes:
            write(db, True, tbl, key, want)
        db.execute("COMMIT")
        print(f"applied: {len(writes)} rows, in gm_audit and {LOG}; press the GM panel's Reload every NPC (the hat on Santa, the item's aura); the shop reads its stock at each visit")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
