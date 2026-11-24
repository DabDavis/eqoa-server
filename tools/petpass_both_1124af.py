#!/usr/bin/env python3
"""petpass_both_1124af.py: GM grants that gave imported copies get their real records back.

A grant is a chartransfers row with from_server "GM grant"; the login gave its
gear at full strength only for that exact text (handlers/transfers.go before
branch grant-prefix), so the grant scripts' labelled rows ("GM grant
(petition 5577)", "GM grant (the owner)", ...) gave each gear piece as its
85% imported copy (database/imported.go, table itemimported). The code fix
(database.IsGMGrant) makes the still-approved rows give the records; this
puts the applied rows' copies back on their records.

For each applied labelled grant: the items its run gave (charmigrate_log,
kind 'item', not undone) that were copies, by copy and base. The rows given
are not followed by item id (a logout save writes the inventory anew), so the
copies the character now holds are matched by count: in the pack and worn
(location 0), the bank (1), and, with --account-bank (Heirloom: world.
account_bank on), the account bank (location 1 of the account's other
non-Hardcore characters and its holders). From what is held is first taken
what the character got of the same copy otherwise (another transfer's run,
cmd/importdamper's moves): those stay copies. What is left, up to what the
grant gave, is swapped; a shortfall is reported (sold, destroyed, traded,
banked by another character, on a mercenary, in an auction or the mail).

The swap is cmd/importdamper's own (UPDATE charinventory SET patternid ...
WHERE itemid AND patternid), so the row keeps its item id, slot, location,
count, charges, hit points and its instance aura (instanceaura by item id);
dyes are the character's slots (chardyes), untouched. Each swap is logged in
grantcopy_1124_log and gm_audit; --undo puts every logged row back. The
copies' records stay (others may hold them).

Run with the character out of the world (a logout save writes over it): the
server held (a keep restart's before.sh), or per character with
--only <serverid> under ~/charmigrate/watch-addr.sh. With --account-bank,
every character of the account must be out.

  petpass_both_1124af.py --db eqoa.db [--account-bank] [--only 54,909] [--apply | --undo]
"""
import argparse
import json
import sqlite3
import sys
from collections import defaultdict
from datetime import datetime, timezone

GRANT = "GM grant"
LOG = "grantcopy_1124_log"
TOOL = "tools/petpass_both_1124af.py"
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
	id       INTEGER PRIMARY KEY,
	itemid   INTEGER NOT NULL,
	serverid INTEGER NOT NULL,
	holder   INTEGER NOT NULL,
	transfer TEXT NOT NULL,
	before   INTEGER NOT NULL,
	after    INTEGER NOT NULL,
	at       TEXT NOT NULL,
	undone   TEXT NOT NULL DEFAULT ''
)"""


def is_gm_grant(s):
    """database.IsGMGrant: "GM grant" alone or followed by a space or '('."""
    if not s.startswith(GRANT):
        return False
    rest = s[len(GRANT):]
    return rest == "" or rest[0] in " ("


def has_table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type='table' AND name=?", (name,)).fetchone() is not None


def audit(db, now, action, rowkey, before, after, detail):
    if not has_table(db, "gm_audit"):
        return
    db.execute("""INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail)
		VALUES (?, 1, 'dab', 'tool', '', ?, 'charinventory', ?, ?, ?, ?)""",
               (now, action, rowkey, json.dumps(before), json.dumps(after), detail))


def bank_holders(db, sid, account_bank):
    """serverids whose location-1 rows are this character's bank."""
    if not account_bank:
        return [sid]
    acct, hc = db.execute("SELECT accountid, COALESCE(hardcore, 0) FROM characters WHERE serverid=?", (sid,)).fetchone()
    if hc:
        return [sid]
    ids = [r[0] for r in db.execute("SELECT serverid FROM characters WHERE accountid=? AND COALESCE(hardcore, 0)=0", (acct,))]
    if has_table(db, "accountbank_holders"):
        ids += [r[0] for r in db.execute("SELECT serverid FROM accountbank_holders WHERE accountid=?", (acct,))]
    return sorted(set(ids) | {sid})


def elsewhere(db, copy):
    """where else a copy's rows are, for the report: holders and places."""
    out = []
    for sid, loc, n in db.execute("""SELECT i.serverid, i.location, count(*) FROM charinventory i WHERE i.patternid=?
			GROUP BY 1, 2""", (copy,)):
        out.append(f"{sid}/loc{loc}x{n}")
    if has_table(db, "charbuyback"):
        for sid, how, n in db.execute("SELECT serverid, how, count(*) FROM charbuyback WHERE patternid=? GROUP BY 1, 2", (copy,)):
            out.append(f"buyback of {sid} ({how}) x{n}")
    if has_table(db, "auction_lots"):
        for st, n in db.execute("SELECT state, count(*) FROM auction_lots WHERE patternid=? GROUP BY 1", (copy,)):
            out.append(f"auction ({st}) x{n}")
    if has_table(db, "mail_attachments"):
        for to, n in db.execute("SELECT to_id, count(*) FROM mail_attachments WHERE patternid=? GROUP BY 1", (copy,)):
            out.append(f"mail to {to} x{n}")
    if has_table(db, "charmercitem"):
        for sid, n in db.execute("SELECT serverid, count(*) FROM charmercitem WHERE patternid=? GROUP BY 1", (copy,)):
            out.append(f"merc of {sid} x{n}")
    return out


def plan(db, account_bank, only):
    grants = {}  # run -> (transfer id, serverid, charname, from_server)
    labelled = []
    for tid, sid, name, frm, status, run in db.execute(
            "SELECT id, serverid, charname, from_server, status, run FROM chartransfers WHERE from_server LIKE 'GM grant%' ORDER BY id"):
        if not is_gm_grant(frm) or frm == GRANT:
            continue
        labelled.append((tid, sid, name, frm, status, run))
        if status == "applied" and run:
            grants[run] = (tid, sid, name, frm)
    imported = dict(db.execute("SELECT patternid, base FROM itemimported"))
    names = dict(db.execute("SELECT patternid, itemname FROM itempattern WHERE patternid IN (SELECT patternid FROM itemimported)"))
    # what each labelled grant gave as copies: (serverid, copy) -> [transfer ids]
    gave = defaultdict(list)
    for run, (tid, sid, name, frm) in grants.items():
        if only and sid not in only:
            continue
        for (after,) in db.execute("""SELECT after FROM charmigrate_log WHERE run=? AND kind='item' AND tbl='charinventory'
				AND undone='' ORDER BY id""", (run,)):
            p = json.loads(after or "{}").get("patternid")
            if p in imported:
                gave[(sid, p)].append(tid)
    # what each character got of a copy otherwise: other runs, importdamper's moves
    other = defaultdict(int)
    for run, sid, after in db.execute("""SELECT run, serverid, after FROM charmigrate_log WHERE kind='item'
			AND tbl='charinventory' AND undone=''"""):
        if run in grants:
            continue
        p = json.loads(after or "{}").get("patternid")
        if p in imported:
            other[(sid, p)] += 1
    if has_table(db, "importdamper_log"):
        for sid, p, n in db.execute("SELECT serverid, after, count(*) FROM importdamper_log WHERE undone='' GROUP BY 1, 2"):
            other[(sid, p)] += n
    swaps, short = [], []
    for (sid, copy), tids in sorted(gave.items()):
        g = len(tids)
        holders = bank_holders(db, sid, account_bank)
        q = ",".join("?" * len(holders))
        rows = db.execute(f"""SELECT itemid, serverid, location, equiploc, stackleft, remaincharge FROM charinventory
			WHERE patternid=? AND ((serverid=? AND location IN (0, 1)) OR (location=1 AND serverid IN ({q})))
			ORDER BY serverid<>?, location, itemid""", [copy, sid, *holders, sid]).fetchall()
        keep = other[(sid, copy)]
        take = max(0, min(g, len(rows) - keep))
        # the character's own rows first, then the account bank's; the last
        # `keep` stay as the other transfers' copies
        for r in rows[:take]:
            swaps.append(dict(itemid=r[0], serverid=sid, holder=r[1], location=r[2], equiploc=r[3], count=r[4],
                              charges=r[5], before=copy, after=imported[copy], name=names.get(copy, ""),
                              transfer=",".join(str(t) for t in sorted(set(tids)))))
        if take < g:
            short.append(dict(serverid=sid, copy=copy, base=imported[copy], name=names.get(copy, ""), gave=g, held=len(rows),
                              other=keep, missing=g - take, elsewhere=elsewhere(db, copy)))
    return labelled, swaps, short


def charname(db, sid):
    r = db.execute("SELECT charName FROM characters WHERE serverid=?", (sid,)).fetchone()
    return r[0] if r else "?"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--db", required=True)
    ap.add_argument("--account-bank", action="store_true", help="world.account_bank is on (Heirloom)")
    ap.add_argument("--only", default="", help="serverids, comma separated")
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    if a.apply and a.undo:
        sys.exit("--apply or --undo, not both")
    only = {int(x) for x in a.only.split(",") if x.strip()}
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("BEGIN IMMEDIATE")
    now = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    try:
        if a.undo:
            db.execute(LOG_SCHEMA)
            back = left = 0
            for lid, itemid, sid, before, after in db.execute(
                    f"SELECT id, itemid, serverid, before, after FROM {LOG} WHERE undone='' ORDER BY id DESC").fetchall():
                if only and sid not in only:
                    continue
                n = db.execute("UPDATE charinventory SET patternid=? WHERE itemid=? AND patternid=?", (before, itemid, after)).rowcount
                back += n
                left += 1 - n
                db.execute(f"UPDATE {LOG} SET undone=? WHERE id=?", (now, lid))
                if n:
                    audit(db, now, "update", json.dumps({"itemid": itemid}), {"patternid": after}, {"patternid": before},
                          f"undo: grant's item back on its imported copy ({TOOL} --undo)")
            print(f"undone: {back} rows back on their copies, {left} gone or changed since")
            db.execute("COMMIT")
            return
        labelled, swaps, short = plan(db, a.account_bank, only)
        st = defaultdict(int)
        for t in labelled:
            st[t[4]] += 1
        print(f"labelled grants: {len(labelled)} ({', '.join(f'{k} {v}' for k, v in sorted(st.items()))})")
        pending = [t for t in labelled if t[4] == "approved"]
        if pending:
            print("  still approved (give records once the fix is live): " +
                  ", ".join(f"#{t[0]} {t[2]}" for t in pending))
        by = defaultdict(list)
        for s in swaps:
            by[s["serverid"]].append(s)
        print(f"\nswaps: {len(swaps)} items on {len(by)} characters")
        for sid in sorted(by):
            print(f"  {charname(db, sid)} ({sid}):")
            for s in by[sid]:
                where = "worn" if s["location"] == 0 and s["equiploc"] >= 0 else ("pack" if s["location"] == 0 else "bank")
                if s["holder"] != sid:
                    where = f"account bank (row of {charname(db, s['holder'])})"
                print(f"    item {s['itemid']}: {s['name']} {s['before']} -> {s['after']}, {where}, count {s['count']}, "
                      f"charges {s['charges']} (transfer {s['transfer']})")
        print(f"\nnot matched: {sum(x['missing'] for x in short)} items on {len({x['serverid'] for x in short})} characters")
        for x in short:
            print(f"  {charname(db, x['serverid'])} ({x['serverid']}): {x['name']} copy {x['copy']} (base {x['base']}): "
                  f"gave {x['gave']}, held {x['held']}, other sources {x['other']}, {x['missing']} missing; "
                  f"the copy is now at: {', '.join(x['elsewhere']) or 'nowhere'}")
        if not a.apply:
            print("\ndry run: nothing written")
            db.execute("ROLLBACK")
            return
        db.execute(LOG_SCHEMA)
        for s in swaps:
            n = db.execute("UPDATE charinventory SET patternid=? WHERE itemid=? AND patternid=?",
                           (s["after"], s["itemid"], s["before"])).rowcount
            if n != 1:
                raise RuntimeError(f"item {s['itemid']} changed under the run")
            db.execute(f"INSERT INTO {LOG} (itemid, serverid, holder, transfer, before, after, at) VALUES (?, ?, ?, ?, ?, ?, ?)",
                       (s["itemid"], s["serverid"], s["holder"], s["transfer"], s["before"], s["after"], now))
            audit(db, now, "update", json.dumps({"itemid": s["itemid"]}), {"patternid": s["before"]}, {"patternid": s["after"]},
                  f"GM grant (transfer {s['transfer']}) gave the imported copy; its record restored ({TOOL})")
        db.execute("COMMIT")
        print(f"written: {len(swaps)} rows")
    except BaseException:
        db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    main()
