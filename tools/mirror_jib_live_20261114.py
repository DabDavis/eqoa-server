#!/usr/bin/env python3
"""Jibrish's NPC placements on the Live Server copied to the Heirloom Test Server (the owner, 2026-11-14: "./copy
should work for jib also on live server, also his placements and copies should show on heirloom also").

The game server mirrors his /place, /clone and /copy from now on (world.mirror_npc_edits_accounts,
internal/handlers/npcmirror.go); this tool does the same for the ones he made before it: every npcs insert of the
Live database's gm_audit by his account (ACCOUNTS) whose detail begins "/place in the game:" or "/clone in the game:",
not reverted, whose row is on Live still (a row he deleted since, as the second crocodile 162788, is listed and left).
Each becomes a new npcs row on Heirloom, of Heirloom's own id sequence, as the server's mirror makes one:

  - its source (the detail's "a copy of NPC <n>") a row of the same name on Heirloom: a copy of THAT row, with every
    column in which the Live row differs from its Live source set as on Live (its spot, facing and world; the
    crocodiles' level 24, the owner's fix after the /place);
  - else the Live row's values;
  - its extras (npcrespawn, npcwander, npctitle, signtext, npcstands) as the Live row's (the crocodile's roamer);
  - npc_mirror (Live row -> Heirloom row; the server's later /move, /roamer, /size, /delete of the row change its copy)
    and a line of npc_mirror_inbox (a server with the mirror reads it into its world within seconds).

Checked as found, else REFUSED and nothing written: the tables npcs and gm_audit on both, the account on Live. Live is
opened read-only. Report by default; --apply in one transaction on Heirloom, a gm_audit line a row (as the panel's),
logged in mirror_jib_live_1114_log; a second --apply finds nothing to do (each Live row mapped already); --undo deletes
exactly what was made (and its npc_mirror rows), each a gm_audit line, a row changed since left and listed, and puts a
"gone" line in the inbox for each.

After --apply on a server without the mirror's inbox (before the build with handlers/npcmirror.go): the GM panel's
"Reload every NPC" on Heirloom puts the rows into its world.

    python3 -E -s tools/mirror_jib_live_20261114.py --live <live eqoa.db> --heirloom <heirloom eqoa.db> [--apply | --undo]
"""
import argparse
import datetime
import json
import re
import sqlite3
import sys

LOG = "mirror_jib_live_1114_log"
TOOL = "tools/mirror_jib_live_20261114.py"
WHO, VIA = "dab", "tool"
ACCOUNTS = ("jibrish",)
PREFIXES = ("/place in the game:", "/clone in the game:")
MIRROR_PREFIX = "mirrored from Live: "  # database.NPCMirrorPrefix
# database/gmnpcrow.go's npcRowExtras that a copy carries: (table, key)
EXTRAS = (("npcrespawn", "npc_row"), ("npcwander", "npcid"), ("npctitle", "npcid"), ("signtext", "npcid"),
          ("npcstands", "npc_row"))
SOURCE_RE = re.compile(r"a copy of NPC (\d+)\s*$")

# database/npcmirror.go's npcMirrorSchema
MIRROR_DDL = """
CREATE TABLE IF NOT EXISTS npc_mirror (
	from_id INTEGER PRIMARY KEY,
	to_id   INTEGER NOT NULL UNIQUE,
	account TEXT NOT NULL DEFAULT '',
	at      TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS npc_mirror_inbox (
	id      INTEGER PRIMARY KEY AUTOINCREMENT,
	npc_row INTEGER NOT NULL,
	gone    INTEGER NOT NULL DEFAULT 0,
	tables  TEXT NOT NULL DEFAULT '',
	at      TEXT NOT NULL
);"""

LOG_DDL = f"""CREATE TABLE {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    rowkey TEXT NOT NULL,      -- the key (JSON)
    after  TEXT NOT NULL,      -- the row written (JSON)
    audit  INTEGER NOT NULL
)"""


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def columns(db, tbl):
    return [r[1] for r in db.execute(f"PRAGMA table_info({tbl})")]


def row(db, tbl, key):
    where = " AND ".join(f'"{k}" = ?' for k in key)
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", tuple(key.values()))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return None if m is None else json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, tbl, key, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (now(), acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key), go_json(before), go_json(after), detail))
    return cur.lastrowid


def insert(db, tbl, values, have):
    cols = [c for c in have if c in values]
    cur = db.execute(f"INSERT INTO {tbl} ({', '.join(chr(34) + c + chr(34) for c in cols)}) VALUES ({', '.join('?' for _ in cols)})",
                     tuple(values[c] for c in cols))
    return cur.lastrowid


def placements(live):
    """Jibrish's placements on Live: [(audit id, live row id, source id or 0, detail, username)], and refusals."""
    accts = []
    for name in ACCOUNTS:
        r = live.execute("SELECT accountid, username FROM accountinfo WHERE username = ? COLLATE NOCASE", (name,)).fetchone()
        if r is None:
            return [], [f"no account {name!r} on Live"]
        accts.append(r)
    out = []
    for acct, uname in accts:
        q = ("SELECT id, rowkey, detail FROM gm_audit WHERE accountid = ? AND tbl = 'npcs' AND action = 'insert' "
             "AND reverted_by IS NULL AND (" + " OR ".join("detail LIKE ?" for _ in PREFIXES) + ") ORDER BY id")
        for aid, rowkey, detail in live.execute(q, (acct,) + tuple(p + "%" for p in PREFIXES)).fetchall():
            rid = int(json.loads(rowkey)["id"])
            m = SOURCE_RE.search(detail)
            out.append((aid, rid, int(m.group(1)) if m else 0, detail, uname))
    return out, []


def plan(live, hl):
    """(made: [(live row id, values for Heirloom's npcs, extras {table: row}, detail, username)], lines, refusals)."""
    refusals = [f"no {t} table on {side}" for side, db in (("Live", live), ("Heirloom", hl))
                for t in ("npcs", "gm_audit", "accountinfo") if not table(db, t)]
    if refusals:
        return [], [], refusals
    rows, refusals = placements(live)
    if refusals:
        return [], [], refusals
    mapped = {}
    if table(hl, "npc_mirror"):
        mapped = dict(hl.execute("SELECT from_id, to_id FROM npc_mirror").fetchall())
    hl_cols = columns(hl, "npcs")
    made, lines = [], []
    for aid, rid, src, detail, uname in rows:
        lr = row(live, "npcs", {"id": rid})
        what = f"Live row {rid} (gm_audit {aid}: {detail})"
        if lr is None:
            lines.append(f"  {what}: deleted on Live since: left")
            continue
        if rid in mapped:
            lines.append(f"  {what}: mirrored already as Heirloom row {mapped[rid]}")
            continue
        values, how = dict(lr), "the Live row's values"
        ls = row(live, "npcs", {"id": src}) if src else None
        hs = None
        if ls is not None:
            hs = hl.execute("SELECT * FROM npcs WHERE id = ? AND npc_name = ? COLLATE NOCASE", (src, lr["npc_name"]))
            r = hs.fetchone()
            hs = None if r is None else dict(zip([d[0] for d in hs.description], r))
        if hs is not None:
            values = dict(hs)
            diff = sorted(c for c in lr if c in hs and c != "id" and lr[c] != ls.get(c))
            for c in diff:
                values[c] = lr[c]
            how = f"a copy of Heirloom's row {src}, with {', '.join(f'{c} {lr[c]!r}' for c in diff) or 'nothing'} as on Live"
        values.pop("id", None)
        values = {c: v for c, v in values.items() if c in hl_cols}
        extras = {}
        for t, k in EXTRAS:
            if table(live, t):
                e = row(live, t, {k: rid})
                if e is not None:
                    if not table(hl, t):
                        refusals.append(f"{what}: has a {t} row, Heirloom has no {t} table")
                    extras[t] = e
        lines.append(f"  {what}: {lr['npc_name']} level {lr['npc_level']} at ({lr['x_coord']:.1f}, {lr['y_coord']:.1f}, "
                     f"{lr['z_coord']:.1f}) world {lr['world']} -> a new Heirloom row, {how}"
                     + (f"; with its {', '.join(extras)}" if extras else ""))
        made.append((rid, values, extras, detail, uname))
    return made, lines, refusals


def apply(hl, made):
    for stmt in MIRROR_DDL.split(";"):
        if stmt.strip():
            hl.execute(stmt)
    hl.execute(LOG_DDL)
    hl_cols = columns(hl, "npcs")
    out = []

    def log(tbl, key, after, a):
        hl.execute(f"INSERT INTO {LOG} (tbl, rowkey, after, audit) VALUES (?, ?, ?, ?)", (tbl, go_json(key), go_json(after), a))

    for rid, values, extras, detail, uname in made:
        d = f"{MIRROR_PREFIX}{uname}'s {detail} (Live row {rid}); {TOOL}"
        nid = insert(hl, "npcs", values, hl_cols)
        after = row(hl, "npcs", {"id": nid})
        log("npcs", {"id": nid}, after, audit(hl, "insert", "npcs", {"id": nid}, None, after, d))
        tables = ["npcs"]
        for t, k in EXTRAS:
            if t not in extras:
                continue
            e = dict(extras[t], **{k: nid})
            insert(hl, t, e, columns(hl, t))
            after = row(hl, t, {k: nid})
            log(t, {k: nid}, after, audit(hl, "insert", t, {k: nid}, None, after, d))
            tables.append(t)
        m = {"from_id": rid, "to_id": nid, "account": uname, "at": now()}
        insert(hl, "npc_mirror", m, columns(hl, "npc_mirror"))
        log("npc_mirror", {"from_id": rid}, row(hl, "npc_mirror", {"from_id": rid}), 0)
        hl.execute("INSERT INTO npc_mirror_inbox (npc_row, gone, tables, at) VALUES (?, 0, ?, ?)", (nid, ",".join(tables), now()))
        out.append((rid, nid))
    return out


def undo(hl):
    """(back, left: [why])."""
    if not table(hl, LOG):
        return 0, []
    back, left, gone = 0, [], []
    for tbl, rowkey, after in hl.execute(f"SELECT tbl, rowkey, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key = json.loads(rowkey)
        have = row(hl, tbl, key)
        if go_json(have) != after:
            left.append(f"{tbl} {rowkey}: changed since, left as it is")
            continue
        where = " AND ".join(f'"{k}" = ?' for k in key)
        hl.execute(f"DELETE FROM {tbl} WHERE {where}", tuple(key.values()))
        if tbl != "npc_mirror":
            audit(hl, "delete", tbl, key, have, None, f"{MIRROR_PREFIX}undone; {TOOL} --undo")
        if tbl == "npcs":
            gone.append(key["id"])
        back += 1
    for nid in gone:
        hl.execute("INSERT INTO npc_mirror_inbox (npc_row, gone, tables, at) VALUES (?, 1, 'npcs', ?)", (nid, now()))
    if not left:
        hl.execute(f"DROP TABLE {LOG}")
    return back, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--live", required=True, help="the Live Server's database (read only)")
    ap.add_argument("--heirloom", required=True, help="the Heirloom Test Server's database (written)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    hl = sqlite3.connect(a.heirloom, isolation_level=None, timeout=30)
    hl.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = undo(hl)
            hl.execute("COMMIT")
            print(f"undone: {back} rows deleted" + "".join(f"\n  left: {w}" for w in left))
            return 0 if not left else 1
        live = sqlite3.connect(f"file:{a.live}?mode=ro", uri=True, timeout=30)
        try:
            made, lines, refusals = plan(live, hl)
        finally:
            live.close()
        print(f"Jibrish's placements on Live ({', '.join(ACCOUNTS)}):")
        for line in lines:
            print(line)
        if refusals:
            hl.execute("ROLLBACK")
            print("REFUSED, nothing written:\n  " + "\n  ".join(refusals))
            return 1
        if not made:
            hl.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if table(hl, LOG):
            hl.execute("ROLLBACK")
            print(f"applied before and changed since ({LOG} is there): --undo first")
            return 1
        if not a.apply:
            hl.execute("ROLLBACK")
            print(f"(report only: {len(made)} rows to make on Heirloom; --apply to write)")
            return 0
        done = apply(hl, made)
        hl.execute("COMMIT")
        print("applied: " + ", ".join(f"Live row {r} -> Heirloom row {n}" for r, n in done) +
              f"; in gm_audit, npc_mirror and {LOG}. A server with the mirror reads them into its world within seconds; "
              "else press Heirloom's GM panel's Reload every NPC")
        return 0
    except BaseException:
        hl.execute("ROLLBACK")
        raise
    finally:
        hl.close()


if __name__ == "__main__":
    sys.exit(main())
