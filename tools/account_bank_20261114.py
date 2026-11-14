#!/usr/bin/env python3
"""The account bank's merge (the owner, 2026-11-14: "make the heirloom server bank for players be account wide and
increase capacity to 2000"; world.account_bank, database/accountbank.go, handlers/accountbank.go).

With world.account_bank on, every character of an account shares one bank: its items are the charinventory rows at
location 1 of all the account's characters (each row keeping the serverid of the character that banked it), its
tunar one row of accountbank. This merges the banks as they stand into it, for each account:
  - the bank rows kept for a smaller client (location 3, LocBankKept) come back to the bank (location 1): the page is
    the session's now, never the row's;
  - the account's bank rows are numbered one list (listnumber 0..n-1: its characters in serverid order, each bank in
    its own order, the kept after it), so a page shows one character's items together; their itemids, serverids
    and every other column are as they were;
  - accountbank.tunar is the sum of its characters' bankTunar (at most 2^31-1, CLIENT's signed 32 bits; the excess
    is said and logged). characters.bankTunar is LEFT as it was: the server does not read it with the account bank,
    and --undo starts from it.
An account over 2000 records (world.account_bank_slots) is said: nothing is lost, its deposits are refused until it
holds fewer.

Report by default; --apply in one transaction, a gm_audit line an account, every row's old place and every
character's bankTunar in account_bank_1114_log (items) and account_bank_1114_tunar (tunar). A second --apply finds
nothing to do. --undo parts the banks again: each logged row still in the bank and still its character's goes back
to its old location and listnumber; a row banked since stays its banker's; one a deleted character banked
(accountbank_holders) goes to the account's first character left; each account's tunar is its characters' bankTunar
as logged plus what the account's bank gained or lost since, that change put on (or, a loss, taken from in serverid
order) its first character; the accountbank rows are emptied.

Run --apply with the server stopped (or no player in the world) and turn world.account_bank on as it restarts; run
--undo with the server stopped and world.account_bank off for its restart. Never on the live database file while the
server runs with players in it.

    python3 -E -s tools/account_bank_20261114.py --db eqoa.db [--apply | --undo] [--slots 2000]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "account_bank_1114_log"
TLOG = "account_bank_1114_tunar"
DETAIL = "the account bank's merge (the owner, 2026-11-14); tools/account_bank_20261114.py"
WHO, VIA = "dab", "tool"
MAX_TUNAR = 2**31 - 1
SLOTS = 2000

SCHEMA = """
CREATE TABLE IF NOT EXISTS accountbank (
    accountid INTEGER PRIMARY KEY,
    tunar     INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS accountbank_holders (
    serverid   INTEGER PRIMARY KEY,
    accountid  INTEGER NOT NULL,
    deleted_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_accountbank_holders_account ON accountbank_holders(accountid);
"""

LOG_DDL = f"""CREATE TABLE {LOG} (
    seq        INTEGER PRIMARY KEY AUTOINCREMENT,
    accountid  INTEGER NOT NULL,
    itemid     INTEGER NOT NULL,
    serverid   INTEGER NOT NULL,   -- its character (the owner before the merge)
    location   INTEGER NOT NULL,   -- 1 the bank, 3 kept from it
    listnumber INTEGER NOT NULL,   -- before
    newlist    INTEGER NOT NULL,   -- after
    audit      INTEGER NOT NULL
)"""

TLOG_DDL = f"""CREATE TABLE {TLOG} (
    seq        INTEGER PRIMARY KEY AUTOINCREMENT,
    accountid  INTEGER NOT NULL,
    serverid   INTEGER NOT NULL,   -- 0: the account's row
    banktunar  INTEGER NOT NULL,   -- the character's bankTunar (left as it was); the account's: the sum written
    excess     INTEGER NOT NULL DEFAULT 0,
    audit      INTEGER NOT NULL
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def go_json(m):
    return None if m is None else json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, acct, before, after, detail=DETAIL):
    row = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, 'accountbank', ?, ?, ?, ?)",
                     (at, row[0] if row else 0, WHO, VIA, action, go_json({"accountid": acct}),
                      go_json(before), go_json(after), detail))
    return cur.lastrowid


def plan(db, slots):
    """(accounts: [{acct, rows: [(itemid, serverid, loc, list, newlist)], chars: [(sid, banktunar)], sum, excess}],
    lines, refusals)."""
    refusals, lines, accounts = [], [], []
    for t in ("characters", "charinventory", "gm_audit"):
        if not table(db, t):
            refusals.append(f"no {t} table here")
    if refusals:
        return accounts, lines, refusals
    if table(db, LOG) or table(db, TLOG):
        lines.append(f"merged before ({LOG} is there): --undo parts the banks again")
        return accounts, lines, refusals
    if table(db, "accountbank"):
        held = db.execute("SELECT COUNT(*), COALESCE(SUM(tunar), 0) FROM accountbank WHERE tunar != 0").fetchone()
        if held[0]:
            refusals.append(f"accountbank holds tunar already ({held[0]} accounts, {held[1]} tunar) and no log says why")
    holders = db.execute("SELECT COUNT(*) FROM accountbank_holders").fetchone()[0] if table(db, "accountbank_holders") else 0
    if holders:
        lines.append(f"note: {holders} deleted characters' banks are the accounts' already (accountbank_holders)")
    chars = db.execute("SELECT accountid, serverid, COALESCE(bankTunar, 0) FROM characters ORDER BY accountid, serverid").fetchall()
    by_acct = {}
    for acct, sid, tunar in chars:
        by_acct.setdefault(acct, []).append((sid, tunar))
    rows_of = {}
    for itemid, sid, loc, lst in db.execute(
            "SELECT itemid, serverid, location, listnumber FROM charinventory WHERE location IN (1, 3) "
            "ORDER BY serverid, location, listnumber, itemid").fetchall():
        rows_of.setdefault(sid, []).append((itemid, sid, loc, lst))
    if table(db, "accountbank_holders"):
        for sid, acct in db.execute("SELECT serverid, accountid FROM accountbank_holders ORDER BY serverid").fetchall():
            by_acct.setdefault(acct, [])
            if sid not in [s for s, _ in by_acct[acct]]:
                by_acct[acct].append((sid, None))  # a deleted character: its rows, no tunar
    over = []
    for acct in sorted(by_acct):
        members = sorted(by_acct[acct], key=lambda m: m[0])
        rows = []
        for sid, _ in members:
            for loc in (1, 3):
                rows.extend(r for r in rows_of.get(sid, []) if r[2] == loc)
        moves = [(itemid, sid, loc, lst, n) for n, (itemid, sid, loc, lst) in enumerate(rows)]
        total = sum(t for _, t in members if t)
        tchars = [(sid, t) for sid, t in members if t is not None]
        if not moves and total == 0:
            continue
        excess = max(0, total - MAX_TUNAR)
        accounts.append({"acct": acct, "rows": moves, "chars": tchars, "sum": min(total, MAX_TUNAR), "excess": excess,
                         "renumbered": sum(1 for m in moves if m[3] != m[4] or m[2] != 1), "kept": sum(1 for m in moves if m[2] == 3),
                         "banks": len({m[1] for m in moves})})
        if len(moves) > slots:
            over.append(f"account {acct}: {len(moves)} records, past {slots} (kept; deposits refused until fewer)")
        if excess:
            lines.append(f"account {acct}: its banks sum to {total} tunar, past {MAX_TUNAR}: {excess} is not kept (logged)")
    n_rows = sum(len(a["rows"]) for a in accounts)
    lines.append(f"{len(accounts)} accounts with a bank: {n_rows} bank records "
                 f"({sum(a['kept'] for a in accounts)} of them kept from the bank, back to it; "
                 f"{sum(a['renumbered'] for a in accounts)} renumbered into one list), "
                 f"{sum(1 for a in accounts if a['banks'] > 1)} accounts with more than one character's bank, "
                 f"{sum(a['sum'] for a in accounts)} tunar in all")
    biggest = sorted(accounts, key=lambda a: -len(a["rows"]))[:5]
    if biggest:
        lines.append("the largest: " + ", ".join(f"account {a['acct']} {len(a['rows'])} records from {a['banks']} banks, {a['sum']} tunar"
                                                  for a in biggest))
    lines.extend(over)
    return accounts, lines, refusals


def apply(db, accounts):
    for stmt in SCHEMA.split(";"):  # not executescript: it would commit the transaction
        if stmt.strip():
            db.execute(stmt)
    db.execute(LOG_DDL)
    db.execute(TLOG_DDL)
    n = 0
    for a in accounts:
        acct = a["acct"]
        before = {"tunar": None, "characters": {str(s): t for s, t in a["chars"]}, "records": len(a["rows"])}
        after = {"tunar": a["sum"], "records": len(a["rows"]), "kept_back": a["kept"]}
        aid = audit(db, "update", acct, before, after)
        for itemid, sid, loc, lst, new in a["rows"]:
            db.execute("UPDATE charinventory SET location = 1, listnumber = ? WHERE itemid = ? AND serverid = ? AND location = ?",
                       (new, itemid, sid, loc))
            db.execute(f"INSERT INTO {LOG} (accountid, itemid, serverid, location, listnumber, newlist, audit) VALUES (?, ?, ?, ?, ?, ?, ?)",
                       (acct, itemid, sid, loc, lst, new, aid))
            n += 1
        for sid, t in a["chars"]:
            db.execute(f"INSERT INTO {TLOG} (accountid, serverid, banktunar, audit) VALUES (?, ?, ?, ?)", (acct, sid, t, aid))
        db.execute(f"INSERT INTO {TLOG} (accountid, serverid, banktunar, excess, audit) VALUES (?, 0, ?, ?, ?)",
                   (acct, a["sum"], a["excess"], aid))
        db.execute("INSERT INTO accountbank (accountid, tunar) VALUES (?, ?) ON CONFLICT(accountid) DO UPDATE SET tunar = excluded.tunar",
                   (acct, a["sum"]))
    return n


def undo(db):
    """(rows back, lines)."""
    if not table(db, LOG) and not table(db, "accountbank"):
        return 0, ["nothing merged here"]
    lines, back, stayed, gone = [], 0, 0, 0
    logged = {}
    if table(db, LOG):
        for acct, itemid, sid, loc, lst, new in db.execute(
                f"SELECT accountid, itemid, serverid, location, listnumber, newlist FROM {LOG} ORDER BY seq").fetchall():
            logged.setdefault(acct, []).append((itemid, sid, loc, lst, new))
            now = db.execute("SELECT serverid, location FROM charinventory WHERE itemid = ?", (itemid,)).fetchone()
            if now is None:
                gone += 1  # sold, taken, destroyed since
                continue
            if now != (sid, 1):
                stayed += 1  # withdrawn (its withdrawer's now) or moved: left
                continue
            db.execute("UPDATE charinventory SET location = ?, listnumber = ? WHERE itemid = ? AND serverid = ? AND location = 1",
                       (loc, lst, itemid, sid))
            back += 1
    lines.append(f"{back} bank records back to their characters' banks as they were; {stayed} withdrawn or moved since "
                 f"(left with whoever holds them); {gone} gone since")
    # rows the deleted characters banked: to the account's first character left
    rehomed = 0
    if table(db, "accountbank_holders"):
        for sid, acct in db.execute("SELECT serverid, accountid FROM accountbank_holders ORDER BY serverid").fetchall():
            first = db.execute("SELECT MIN(serverid) FROM characters WHERE accountid = ?", (acct,)).fetchone()[0]
            ids = [r[0] for r in db.execute("SELECT itemid FROM charinventory WHERE serverid = ? AND location IN (1, 3)", (sid,)).fetchall()]
            if not ids:
                db.execute("DELETE FROM accountbank_holders WHERE serverid = ?", (sid,))
                continue
            if first is None:
                lines.append(f"account {acct}: deleted character {sid}'s {len(ids)} bank records have no character to go to: left")
                continue
            nxt = db.execute("SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory WHERE serverid = ? AND location = 1", (first,)).fetchone()[0]
            for k, itemid in enumerate(ids):
                db.execute("UPDATE charinventory SET serverid = ?, location = 1, listnumber = ? WHERE itemid = ?", (first, nxt + k, itemid))
            audit(db, "update", acct, {"holder": sid, "records": len(ids)}, {"serverid": first},
                  DETAIL + ": --undo, a deleted character's bank to the account's first character")
            db.execute("DELETE FROM accountbank_holders WHERE serverid = ?", (sid,))
            rehomed += len(ids)
    if rehomed:
        lines.append(f"{rehomed} records a deleted character banked went to its account's first character")
    # tunar: the logged columns, plus what each account's bank gained or lost since
    merged = {}
    if table(db, TLOG):
        for acct, sid, t in db.execute(f"SELECT accountid, serverid, banktunar FROM {TLOG} WHERE serverid = 0").fetchall():
            merged[acct] = t
    moved = 0
    for acct, tunar in db.execute("SELECT accountid, tunar FROM accountbank").fetchall() if table(db, "accountbank") else []:
        delta = tunar - merged.get(acct, 0)
        sids = [r[0] for r in db.execute("SELECT serverid FROM characters WHERE accountid = ? ORDER BY serverid", (acct,)).fetchall()]
        if delta and not sids:
            lines.append(f"account {acct}: its bank's change of {delta} tunar has no character to go to: left in accountbank")
            continue
        if delta > 0:
            db.execute("UPDATE characters SET bankTunar = MIN(bankTunar + ?, ?) WHERE serverid = ?", (delta, MAX_TUNAR, sids[0]))
        elif delta < 0:
            left = -delta
            for sid in sids:
                have = db.execute("SELECT bankTunar FROM characters WHERE serverid = ?", (sid,)).fetchone()[0]
                take = min(have, left)
                db.execute("UPDATE characters SET bankTunar = bankTunar - ? WHERE serverid = ?", (take, sid))
                left -= take
                if left == 0:
                    break
        if delta:
            audit(db, "update", acct, {"tunar": tunar}, {"tunar": None, "change_to_first": delta},
                  DETAIL + ": --undo, the bank's change since the merge")
            moved += 1
        db.execute("DELETE FROM accountbank WHERE accountid = ?", (acct,))
    lines.append(f"{moved} accounts' bank tunar changed since the merge: the change put on their first character")
    for t in (LOG, TLOG):
        if table(db, t):
            db.execute(f"DROP TABLE {t}")
    return back, lines


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--slots", type=int, default=SLOTS, help="world.account_bank_slots (2000)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, lines = undo(db)
            db.execute("COMMIT")
            print("undone:\n  " + "\n  ".join(lines))
            return 0
        accounts, lines, refusals = plan(db, a.slots)
        for line in lines:
            print(line)
        if refusals:
            db.execute("ROLLBACK")
            print("REFUSED, nothing written:\n  " + "\n  ".join(refusals))
            return 1
        if not accounts:
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"(report only: {len(accounts)} accounts; --apply to write, with the server stopped, then start it with world.account_bank on)")
            return 0
        n = apply(db, accounts)
        db.execute("COMMIT")
        print(f"applied: {len(accounts)} accounts, {n} bank records, in gm_audit, {LOG} and {TLOG}; start the server with world.account_bank on")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
