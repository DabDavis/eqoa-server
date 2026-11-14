#!/usr/bin/env python3
"""Seed the tunar ledger from the server's logs, so the GM panel's Economy page has a trend from its first day (the
owner, 2026-11-14: "lets add an economy tracker for the gm panel shows tunar accumulated from loot, selling, etc. and
trend make it informative").

The server counts every movement of tunar from this build on (internal/economy, tables tunar_ledger and
tunar_ledger_chars). Before it, the logs say part of it, a line an event; this reads those lines and adds them to the
same tables, an hour and a source a row (and a character and a day), for the hours BEFORE the ledger's first hour of
its own (so nothing is counted twice; --until moves the line). What the logs say:

    loot           Loot: <name> took <n> tunar from "<npc>"'s corpse (<m> now)
    loot_split     Loot: <name> took a split of <n> tunar from ...
    merchant_sell  MerchantSell: <name> sold <k> "<item>" to "<npc>" for <n>
    merchant_buy   MerchantBuy: <name> bought <k> "<item>" from "<npc>" for <n>
    buyback        BuyBack: <name> bought back <k> of "<item>" from "<npc>" for <n> (entry ...
    repair         Repair: "<npc>" mended <name>'s "<item>" (a/b) for <n>
    pickpocket     Pickpocket: <name> takes <n> tunar from <npc>
    cure           Weres: <name> was cured of ... for <n> tunar
    quest_refund   Quests: <name>'s "<quest>" put back ... (handed back: <k> items, <n> tunar)
    auction_fee    Auction: <name> put up ... fee <n>; bank ...
    auction_sale   Auction: lot <id> (...) closed sold: winner <id> at <n>, seller <id>; ...
                   Auction: stack lot <id> (...) closed ...: <k> sold at <n> each to <b> bidders, seller <id>; ...
    duel_wager     Duel <id>: the wager (<a> and <b>) pays <x> and <y>   (the loser's stake; no names in the line)
    gm_give/take   GM: bank tunar of '<name>' <a> -> <b>   and   GM: bank tunar of '<name>' (offline) <+n> -> <b>

Not in the logs, so not backfilled: quest rewards, waysender fares, quest payments, trades' tunar, #tunar, #template,
the panel's edits, new characters' purses. The page says so for the backfilled hours. No wealth snapshot can be made
from a log: the money supply's line starts at the first hour the server measured it.

Names are matched to characters by charName (a deleted character's line counts in the hour, not as a character).
Logs: plain or .gz, or '-' for lines on stdin (a grep of the box's logs). A report by default; --apply adds, in one
transaction, every row it adds logged in tunar_backfill_log (run, what, by how much) and one gm_audit line for the run;
a second --apply finds the run there and does nothing; --undo takes each run's additions off again (a row left at 0
deleted), exactly. The ledger is read at each load of the page: no restart.

    python3 -E -s tools/economy_backfill_20261114.py --db eqoa.db --logs gameserver.log gameserver.log.*.gz [--days 30] [--apply | --undo]
"""
import argparse
import datetime
import gzip
import json
import re
import sqlite3
import sys

LOG = "tunar_backfill_log"
WHO, VIA = "dab", "tool"
DETAIL = "the tunar ledger seeded from the server's logs before its own first hour; tools/economy_backfill_20261114.py"
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    run    INTEGER NOT NULL,
    tbl    TEXT NOT NULL,              -- tunar_ledger or tunar_ledger_chars
    hour   INTEGER NOT NULL DEFAULT 0, -- tunar_ledger: the hour; tunar_ledger_chars: the day
    serverid INTEGER NOT NULL DEFAULT 0,
    source TEXT NOT NULL,
    name   TEXT NOT NULL DEFAULT '',
    count  INTEGER NOT NULL DEFAULT 0,
    amount INTEGER NOT NULL DEFAULT 0,
    got    INTEGER NOT NULL DEFAULT 0,
    paid   INTEGER NOT NULL DEFAULT 0,
    existed INTEGER NOT NULL DEFAULT 0, -- the row was there before (added to, not made)
    at     TEXT NOT NULL DEFAULT ''
)"""

KIND = {"loot": "faucet", "loot_split": "faucet", "merchant_sell": "faucet", "pickpocket": "faucet",
        "quest_refund": "faucet", "gm_give": "faucet", "merchant_buy": "sink", "buyback": "sink", "repair": "sink",
        "cure": "sink", "auction_fee": "sink", "gm_take": "sink", "auction_sale": "transfer", "duel_wager": "transfer"}

TS = re.compile(r"^(\d{4})/(\d\d)/(\d\d) (\d\d):(\d\d):(\d\d) (.*)$")
N = r"(\S+)"
PATTERNS = [
    # (source, regex, how): how picks (to name, from name, amount) out of the match
    ("loot_split", re.compile(r"^Loot: " + N + r" took a split of (\d+) tunar from "), "made"),
    ("loot", re.compile(r"^Loot: " + N + r" took (\d+) tunar from .*'s corpse \(-?\d+ now\)$"), "made"),
    ("merchant_sell", re.compile(r"^MerchantSell: " + N + r" sold \d+ \".*\" to \".*\" for (\d+)$"), "made"),
    ("merchant_buy", re.compile(r"^MerchantBuy: " + N + r" bought \d+ \".*\" from \".*\" for (\d+)$"), "spent"),
    ("buyback", re.compile(r"^BuyBack: " + N + r" bought back \d+ of \".*\" from \".*\" for (\d+) \(entry "), "spent"),
    ("repair", re.compile(r"^Repair: \".*\" mended " + N + r"'s \".*\" \(\d+/\d+\) for (\d+)$"), "spent"),
    ("pickpocket", re.compile(r"^Pickpocket: " + N + r" takes (\d+) tunar from "), "made"),
    ("cure", re.compile(r"^Weres: " + N + r" was cured of .* for (\d+) tunar$"), "spent"),
    ("quest_refund", re.compile(r"^Quests: " + N + r"'s \".*\" put back .*\(handed back: \d+ items, (\d+) tunar\)$"), "made"),
    ("auction_fee", re.compile(r"^Auction: " + N + r" put up .* fee (\d+); bank -?\d+\)$"), "spent"),
]
SALE = re.compile(r"^Auction: lot \d+ \(.*\) closed sold: winner (\d+) at (\d+), seller (\d+);")
STACK = re.compile(r"^Auction: stack lot \d+ \(.*\) closed \w+: (\d+) sold at (\d+) each to \d+ bidders, seller (\d+);")
DUEL = re.compile(r"^Duel \d+: the wager \((\d+) and (\d+)\) pays (\d+) and (\d+)$")
GMBANK = re.compile(r"^GM: bank tunar of '(.+)' (-?\d+) -> (-?\d+)$")
GMBANK_OFF = re.compile(r"^GM: bank tunar of '(.+)' \(offline\) ([+-]\d+) -> -?\d+$")
PREFIXES = ("Loot: ", "MerchantSell: ", "MerchantBuy: ", "BuyBack: ", "Repair: ", "Pickpocket: ", "Weres: ", "Quests: ",
            "Auction: ", "Duel ", "GM: bank tunar")


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def lines(paths):
    for p in paths:
        if p == "-":
            f = sys.stdin
        elif p.endswith(".gz"):
            f = gzip.open(p, "rt", encoding="utf-8", errors="replace")
        else:
            f = open(p, encoding="utf-8", errors="replace")
        try:
            for line in f:
                yield line.rstrip("\n")
        finally:
            if f is not sys.stdin:
                f.close()


def parse(paths, since, until, ids):
    """The movements of the lines from since to until (unix): [(t, source, to_id, to_name, from_id, from_name, n)]."""
    out, unknown = [], set()
    names = {v: k for k, v in ids.items()}
    for line in lines(paths):
        m = TS.match(line)
        if not m:
            continue
        rest = m.group(7)
        if not rest.startswith(PREFIXES):
            continue
        t = int(datetime.datetime(*map(int, m.groups()[:6]), tzinfo=datetime.timezone.utc).timestamp())
        if t < since or t >= until:
            continue

        def who(name):
            sid = ids.get(name, 0)
            if not sid:
                unknown.add(name)
            return sid

        hit = False
        for src, rx, how in PATTERNS:
            p = rx.match(rest)
            if p:
                name, n = p.group(1), int(p.group(2))
                if how == "made":
                    out.append((t, src, who(name), name, 0, "", n))
                else:
                    out.append((t, src, 0, "", who(name), name, n))
                hit = True
                break
        if hit:
            continue
        p = SALE.match(rest)
        if p:
            w, n, s = int(p.group(1)), int(p.group(2)), int(p.group(3))
            out.append((t, "auction_sale", s, names.get(s, ""), w, names.get(w, ""), n))
            continue
        p = STACK.match(rest)
        if p:
            k, each, s = int(p.group(1)), int(p.group(2)), int(p.group(3))
            if k > 0:
                out.append((t, "auction_sale", s, names.get(s, ""), 0, "", k * each))
            continue
        p = DUEL.match(rest)
        if p:
            a, b, x, y = map(int, p.groups())
            if x == 0 and y > 0:
                out.append((t, "duel_wager", 0, "", 0, "", a))  # A lost its stake to B
            elif y == 0 and x > 0:
                out.append((t, "duel_wager", 0, "", 0, "", b))
            continue
        p = GMBANK.match(rest) or GMBANK_OFF.match(rest)
        if p:
            name = p.group(1)
            d = int(p.group(3)) - int(p.group(2)) if p.re is GMBANK else int(p.group(2))
            if d > 0:
                out.append((t, "gm_give", who(name), name, 0, "", d))
            elif d < 0:
                out.append((t, "gm_take", 0, "", who(name), name, -d))
    return out, unknown


def tally(moves):
    hours, chars = {}, {}
    for t, src, to_id, to_name, from_id, from_name, n in moves:
        if n <= 0:
            continue
        hk = (t // 3600 * 3600, src)
        c, a = hours.get(hk, (0, 0))
        hours[hk] = (c + 1, a + n)
        day = t // 86400 * 86400
        if to_id:
            k = (day, to_id, src)
            name, c, g, p = chars.get(k, (to_name, 0, 0, 0))
            chars[k] = (to_name or name, c + 1, g + n, p)
        if from_id:
            k = (day, from_id, src)
            name, c, g, p = chars.get(k, (from_name, 0, 0, 0))
            chars[k] = (from_name or name, c + 1, g, p + n)
    return hours, chars


def first_own_hour(db):
    """The ledger's first hour the server counted itself (not a backfill's)."""
    q = "SELECT MIN(hour) FROM tunar_ledger"
    if table(db, LOG):
        q += f" WHERE (hour, source) NOT IN (SELECT hour, source FROM {LOG} WHERE tbl = 'tunar_ledger' AND existed = 0)"
    r = db.execute(q).fetchone()
    return r[0] if r and r[0] else None


def fmt(n):
    return f"{n:,}"


def report(hours, chars, unknown, since, until):
    by = {}
    for (h, src), (c, a) in hours.items():
        x = by.setdefault(src, [0, 0])
        x[0] += c
        x[1] += a
    span = (datetime.datetime.fromtimestamp(since, datetime.timezone.utc).strftime("%Y-%m-%d %H:%M"),
            datetime.datetime.fromtimestamp(until, datetime.timezone.utc).strftime("%Y-%m-%d %H:%M"))
    print(f"lines from {span[0]} to {span[1]} UTC: {len(hours)} hour rows, {len(chars)} character-day rows")
    made = sum(a for s, (c, a) in by.items() if KIND[s] == "faucet")
    spent = sum(a for s, (c, a) in by.items() if KIND[s] == "sink")
    for src in sorted(by, key=lambda s: (["faucet", "sink", "transfer"].index(KIND[s]), -by[s][1])):
        c, a = by[src]
        print(f"  {KIND[src]:8} {src:14} {fmt(c):>9} times {fmt(a):>15} tunar")
    print(f"  made {fmt(made)}, spent {fmt(spent)}, net {fmt(made - spent)}")
    if hours:
        first = min(h for h, _ in hours)
        print("  first hour:", datetime.datetime.fromtimestamp(first, datetime.timezone.utc).strftime("%Y-%m-%d %H:%M"))
    if unknown:
        print(f"  {len(unknown)} names no character has (counted in the hours only): {', '.join(sorted(unknown)[:12])}"
              + (" ..." if len(unknown) > 12 else ""))


def audit(db, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'backfill', 'tunar_ledger', '', '', ?, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, json.dumps(after, separators=(",", ":"), sort_keys=True), DETAIL))


def apply(db, hours, chars, since, until):
    db.execute(LOG_SCHEMA)
    run = (db.execute(f"SELECT MAX(run) FROM {LOG}").fetchone()[0] or 0) + 1
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    for (h, src), (c, a) in sorted(hours.items()):
        existed = db.execute("SELECT 1 FROM tunar_ledger WHERE hour = ? AND source = ?", (h, src)).fetchone() is not None
        db.execute("INSERT INTO tunar_ledger (hour, source, kind, count, amount) VALUES (?, ?, ?, ?, ?) "
                   "ON CONFLICT (hour, source) DO UPDATE SET count = count + excluded.count, amount = amount + excluded.amount",
                   (h, src, KIND[src], c, a))
        db.execute(f"INSERT INTO {LOG} (run, tbl, hour, source, count, amount, existed, at) VALUES (?, 'tunar_ledger', ?, ?, ?, ?, ?, ?)",
                   (run, h, src, c, a, int(existed), at))
    for (d, sid, src), (name, c, g, p) in sorted(chars.items()):
        existed = db.execute("SELECT 1 FROM tunar_ledger_chars WHERE day = ? AND serverid = ? AND source = ?",
                             (d, sid, src)).fetchone() is not None
        db.execute("INSERT INTO tunar_ledger_chars (day, serverid, source, name, count, got, paid) VALUES (?, ?, ?, ?, ?, ?, ?) "
                   "ON CONFLICT (day, serverid, source) DO UPDATE SET count = count + excluded.count, got = got + excluded.got, "
                   "paid = paid + excluded.paid", (d, sid, src, name, c, g, p))
        db.execute(f"INSERT INTO {LOG} (run, tbl, hour, serverid, source, name, count, got, paid, existed, at) "
                   "VALUES (?, 'tunar_ledger_chars', ?, ?, ?, ?, ?, ?, ?, ?, ?)", (run, d, sid, src, name, c, g, p, int(existed), at))
    audit(db, {"run": run, "since": since, "until": until, "hours": len(hours), "characterDays": len(chars)})
    return run


def undo(db):
    if not table(db, LOG):
        print("no backfill to undo")
        return 0
    rows = db.execute(f"SELECT run, tbl, hour, serverid, source, count, amount, got, paid, existed FROM {LOG} ORDER BY seq DESC").fetchall()
    for run, tbl, h, sid, src, c, a, g, p, existed in rows:
        if tbl == "tunar_ledger":
            cur = db.execute("SELECT count, amount FROM tunar_ledger WHERE hour = ? AND source = ?", (h, src)).fetchone()
            if cur is None or cur[0] < c or cur[1] < a:
                raise SystemExit(f"refused: tunar_ledger {h} {src} is {cur}, less than run {run} added ({c}, {a})")
            if not existed and cur == (c, a):
                db.execute("DELETE FROM tunar_ledger WHERE hour = ? AND source = ?", (h, src))
            else:
                db.execute("UPDATE tunar_ledger SET count = count - ?, amount = amount - ? WHERE hour = ? AND source = ?", (c, a, h, src))
        else:
            cur = db.execute("SELECT count, got, paid FROM tunar_ledger_chars WHERE day = ? AND serverid = ? AND source = ?",
                             (h, sid, src)).fetchone()
            if cur is None or cur[0] < c or cur[1] < g or cur[2] < p:
                raise SystemExit(f"refused: tunar_ledger_chars {h} {sid} {src} is {cur}, less than run {run} added")
            if not existed and cur == (c, g, p):
                db.execute("DELETE FROM tunar_ledger_chars WHERE day = ? AND serverid = ? AND source = ?", (h, sid, src))
            else:
                db.execute("UPDATE tunar_ledger_chars SET count = count - ?, got = got - ?, paid = paid - ? "
                           "WHERE day = ? AND serverid = ? AND source = ?", (c, g, p, h, sid, src))
    db.execute(f"DROP TABLE {LOG}")
    audit(db, {"undone": len(rows)})
    return len(rows)


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--logs", nargs="*", default=[])
    ap.add_argument("--days", type=int, default=30, help="the days before --until read (default 30)")
    ap.add_argument("--until", help="UTC 'YYYY-MM-DD HH:MM': lines before it (default: the ledger's own first hour, else now's hour)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 5000")
    if not table(db, "tunar_ledger"):
        raise SystemExit("refused: no tunar_ledger here (a server of this build makes it at its start)")
    if a.undo:
        db.execute("BEGIN IMMEDIATE")
        n = undo(db)
        db.execute("COMMIT")
        print(f"undone: {n} additions taken off")
        return
    if table(db, LOG) and db.execute(f"SELECT COUNT(*) FROM {LOG}").fetchone()[0] > 0:
        run, lo, hi = db.execute(f"SELECT MAX(run), MIN(hour), MAX(hour) FROM {LOG} WHERE tbl = 'tunar_ledger'").fetchone()
        print(f"already backfilled (run {run}, hours {lo} to {hi}): nothing to do (--undo first to do it again)")
        return
    if not a.logs:
        raise SystemExit("--logs: the server's log files (or - for stdin)")
    if a.until:
        until = int(datetime.datetime.strptime(a.until, "%Y-%m-%d %H:%M").replace(tzinfo=datetime.timezone.utc).timestamp())
    else:
        until = first_own_hour(db) or int(datetime.datetime.now(datetime.timezone.utc).timestamp()) // 3600 * 3600
    since = until - a.days * 86400
    ids = dict(db.execute("SELECT charName, serverid FROM characters"))
    moves, unknown = parse(a.logs, since, until, ids)
    hours, chars = tally(moves)
    report(hours, chars, unknown, since, until)
    if not a.apply:
        print("a report: --apply writes it")
        return
    db.execute("BEGIN IMMEDIATE")
    run = apply(db, hours, chars, since, until)
    db.execute("COMMIT")
    print(f"applied as run {run}")


if __name__ == "__main__":
    main()
