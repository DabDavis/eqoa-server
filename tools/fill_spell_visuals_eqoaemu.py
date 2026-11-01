#!/usr/bin/env python3
"""Spell visuals from EQOAEmu's spell workbook (the user, 2026-11-01: "this is
eqoaemu", "we are missing many", "some of ours could possibly be wrong, lets
lean on them", "change gm set also").

EQOAEmu's EQOA_Spells_and_CM workbook (2020; the operator's copy, an .ods: it
is not in this repository) names each spell's visual in its Spells sheet's
Graphic column: the DictID 0x00B1 plays (spelleffect.fxdict). Measured before
it was used: it agreed with what Sandstorm plays for 200 of the 210 spells
recorded there, and its level, cast time and power with Sandstorm's own
ability records for 95% of 163; its icons (byte-reversed) for only 123, so
those are not taken.

Each spell of spellpattern is matched by name to the sheet's rows (several
rows of one name: the spell's classes in classspells decide); a visual is
taken only when it is one of the disc's (SPELLFX.CSF's 0xC200 records, read
off the operator's ISO as the GM panel reads them), the sheet's few typos read
as the disc settles them (9 hex digits: the 8 that are a visual). The sheet's
goes in wherever the spell's differs: a stand-in (0 or no row), and also one
recorded from Sandstorm, an EQOA-Data import, a GM or the community, as the
user chose. Each change is a row edit as the GM panel writes one: the row
before and after in gm_audit (as dab, via "tool"), so the panel lists it with
the spell and reverts it alone; and all of them are logged in
spell_visuals_eqoaemu_log for --undo. A cast reads its visual at the cast, so
no restart is needed (the community page's list is rebuilt within a minute).

Plan where the workbook and the disc are, apply where the database is:

    tools/fill_spell_visuals_eqoaemu.py --sheet <workbook.ods> --iso <disc.iso> --db <copy> --save-plan plan.json
    tools/fill_spell_visuals_eqoaemu.py --db eqoa.db --plan plan.json --apply     # each change only if the row is still as planned
    tools/fill_spell_visuals_eqoaemu.py --db eqoa.db --undo                       # every row still as written goes back
"""
import argparse
import collections
import datetime
import json
import os
import re
import sqlite3
import struct
import sys
import xml.etree.ElementTree as ET
import zipfile
import zlib

HERE = os.path.dirname(os.path.abspath(__file__))
DEFAULT_DB = os.path.join(HERE, "..", "eqoa.db")
LOG = "spell_visuals_eqoaemu_log"
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    spellid       INTEGER PRIMARY KEY,
    had_row       INTEGER NOT NULL,   -- 0: the row was made here
    fx_before     INTEGER NOT NULL,
    source_before TEXT NOT NULL,
    fx_after      INTEGER NOT NULL,
    source_after  TEXT NOT NULL,
    audit         INTEGER NOT NULL    -- the gm_audit line written
)"""
# database.GMAuditSchema, as the server makes it
AUDIT_SCHEMA = """CREATE TABLE IF NOT EXISTS gm_audit (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	at TEXT NOT NULL, accountid INTEGER NOT NULL, username TEXT NOT NULL,
	via TEXT NOT NULL, ip TEXT NOT NULL DEFAULT '', action TEXT NOT NULL,
	tbl TEXT NOT NULL DEFAULT '', rowkey TEXT NOT NULL DEFAULT '',
	before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER
)"""
WHO, VIA = "dab", "tool"
DETAIL = "EQOAEmu's spell workbook (2020): tools/fill_spell_visuals_eqoaemu.py"
SOURCE_NOTE = "visual from EQOAEmu's workbook"
CLASSES = {"WAR": 0, "RAN": 1, "PAL": 2, "SK": 3, "MNK": 4, "BRD": 5, "RGE": 6, "DRD": 7, "SHA": 8, "CL": 9,
           "MAG": 10, "NEC": 11, "ENC": 12, "WIZ": 13, "ALC": 14}


def fold(s):
    return re.sub(r"\s+", " ", (s or "").strip().lower())


# ---- the workbook (an .ods: a zip of XML), stdlib only ------------------------------

_T = "urn:oasis:names:tc:opendocument:xmlns:table:1.0"
_X = "urn:oasis:names:tc:opendocument:xmlns:text:1.0"
_O = "urn:oasis:names:tc:opendocument:xmlns:office:1.0"


def ods_rows(path, sheet):
    """The named sheet's rows as dicts by its header row."""
    z = zipfile.ZipFile(path)
    for _, el in ET.iterparse(z.open("content.xml"), events=("end",)):
        if el.tag != f"{{{_T}}}table":
            continue
        if el.get(f"{{{_T}}}name") != sheet:
            el.clear()
            continue
        rows = []
        for r in el.iter(f"{{{_T}}}table-row"):
            cells = []
            for c in r:
                if c.tag not in (f"{{{_T}}}table-cell", f"{{{_T}}}covered-table-cell"):
                    continue
                v = "\n".join("".join(p.itertext()) for p in c.iter(f"{{{_X}}}p")) or c.get(f"{{{_O}}}value") or ""
                cells.extend([v] * min(int(c.get(f"{{{_T}}}number-columns-repeated", "1")), 200))
            while cells and cells[-1] == "":
                cells.pop()
            if cells:
                rows.append(cells)
        head = rows[0]
        return [dict(zip(head, x)) for x in rows[1:]]
    raise SystemExit(f"{path}: no sheet {sheet!r}")


# ---- the disc's visuals (as the GM panel's esfcore.js reads SPELLFX.CSF) ----------------

def disc_records(iso, parts):
    """The top-level records of a CSF on the disc (parts: its path, e.g.
    ("DATA2", "SPELLFX.CSF")) as (type, body offset, size, children) over its
    inflated data: (data, roots)."""
    sec = 2048
    with open(iso, "rb") as f:
        def read(lba, n):
            f.seek(lba * sec)
            return f.read(n)
        pvd = read(16, sec)
        if pvd[1:6] != b"CD001":
            raise SystemExit(f"{iso}: not an ISO 9660 image")
        lba, size = struct.unpack_from("<I", pvd, 156 + 2)[0], struct.unpack_from("<I", pvd, 156 + 10)[0]
        for part in parts:
            data, p, found = read(lba, size), 0, None
            while p < len(data):
                n = data[p]
                if n == 0:
                    p = (p // sec + 1) * sec
                    continue
                name = data[p + 33:p + 33 + data[p + 32]].decode("ascii", "replace").split(";")[0]
                if name.upper() == part:
                    found = (struct.unpack_from("<I", data, p + 2)[0], struct.unpack_from("<I", data, p + 10)[0])
                    break
                p += n
            if not found:
                raise SystemExit(f"{iso}: no {part}")
            lba, size = found
        buf = read(lba, size)
    blocks, pos = struct.unpack_from("<i", buf, 4)[0], struct.unpack_from("<q", buf, 24)[0]
    out = bytearray()
    for _ in range(blocks):
        n = struct.unpack_from("<I", buf, pos)[0]
        out += zlib.decompress(buf[pos + 8:pos + 8 + n])
        pos += 8 + n
    data = bytes(out)
    if data[:4][::-1] != b"OBJF":
        raise SystemExit(f"{parts[-1]}: no OBJF container inside")
    pos, stack, roots = struct.unpack_from("<q", data, 16)[0], [], []
    while True:
        while stack and stack[-1][0] == 0:
            pos = max(pos, stack.pop()[1])
        if pos + 12 > len(data):
            break
        t, _, sz, nsub = struct.unpack_from("<Hhii", data, pos)
        body = pos + 12
        if (t == 0 and not stack) or sz < 0 or body + sz > len(data):
            break
        node = (t, body, sz, [])
        if stack:
            stack[-1][0] -= 1
            stack[-1][2][3].append(node)
        else:
            roots.append(node)
        if nsub > 0:
            stack.append([nsub, body + sz, node])
            pos = body
        else:
            pos = body + sz
    return data, roots


def disc_visuals(iso):
    """Every 0xC200 record's DictID (its 0xC210) in the disc's DATA2/SPELLFX.CSF."""
    data, roots = disc_records(iso, ("DATA2", "SPELLFX.CSF"))
    ids = set()
    for t, _, _, kids in roots:
        if t != 0xC200:
            continue
        for kt, kb, ks, _ in kids:
            if kt == 0xC210 and ks >= 4:
                ids.add(struct.unpack_from("<I", data, kb)[0])
                break
    return ids


def disc_icons(iso):
    """Every surface's DictID in the disc's DATA2/UI.CSF (its top-level 0x1000
    records' first word): the icons an ability record can show (the client
    draws an ability's icon by that DictID; every spell icon eqoaclassic had
    before 2026-11-01, 39, is one of its 83)."""
    data, roots = disc_records(iso, ("DATA2", "UI.CSF"))
    return {struct.unpack_from("<I", data, b)[0] for t, b, sz, _ in roots if t == 0x1000 and sz >= 4}


def dictid(text, disc):
    """The sheet's Graphic as a visual of the disc, or None: 8 hex digits (a 0x
    before them or not), or a typo of 9 whose first or last 8 are one."""
    t = (text or "").strip()
    t = t[2:] if t[:2].lower() == "0x" else t
    if not re.fullmatch(r"[0-9a-fA-F]{8,9}", t):
        return None
    cands = {int(t[:8], 16), int(t[-8:], 16)}
    hits = [c for c in cands if c in disc]
    return hits[0] if len(hits) == 1 else None


# ---- the plan ------------------------------------------------------------------------

def plan(db, sheet, disc):
    """What changes: per spell the sheet's visual where ours differs; and the counts."""
    byname = collections.defaultdict(list)
    for r in sheet:
        if r.get("Name"):
            byname[fold(r["Name"])].append(({CLASSES[c] for c in (r.get("CLASS") or "").split() if c in CLASSES},
                                             dictid(r.get("Graphic"), disc)))
    classes = collections.defaultdict(set)
    for c, s in db.execute("SELECT class, spellid FROM classspells"):
        classes[s].add(c)
    rows = {s: (fx, src) for s, fx, src in db.execute("SELECT spellid, fxdict, source FROM spelleffect")}
    out, why = [], collections.Counter()
    for sid, name in db.execute("SELECT spellid, spellname FROM spellpattern ORDER BY spellid"):
        cands = byname.get(fold(name))
        if not cands:
            why["not in the sheet by name"] += 1
            continue
        good = {g for _, g in cands if g is not None}
        if len(good) > 1:
            good = {g for cs, g in cands if g is not None and cs & classes.get(sid, set())}
        if len(good) != 1:
            why["the sheet gives no single visual of the disc"] += 1
            continue
        fx = good.pop()
        now, src = rows.get(sid, (None, None))
        cur = (now or 0) & 0xFFFFFFFF
        if now is not None and cur == fx:
            why["ours is the sheet's already"] += 1
            continue
        kind = "a stand-in filled" if not cur else "ours replaced"
        why[kind] += 1
        out.append({"spellid": sid, "name": name, "fx": fx, "fx_now": cur, "had_row": now is not None,
                    "source_now": src if src is not None else "", "kind": kind})
    return out, why


# ---- writing it ----------------------------------------------------------------------

def go_json(m):
    """A row as Go's json.Marshal writes a map (keys sorted, <>& escaped)."""
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def row_of(db, sid):
    cur = db.execute("SELECT * FROM spelleffect WHERE spellid = ?", (sid,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def audit(db, action, sid, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone()
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, 'spelleffect', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, go_json({"spellid": sid}),
                      None if before is None else go_json(before), None if after is None else go_json(after), detail))
    return cur.lastrowid


def apply(db, entries):
    """Each planned change whose row is still as planned; returns (done, changed since)."""
    done = stale = 0
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(AUDIT_SCHEMA)
        db.execute(LOG_SCHEMA)
        for e in entries:
            sid = e["spellid"]
            before = row_of(db, sid)
            if (before is not None) != e["had_row"] or (before is not None and (before["fxdict"] & 0xFFFFFFFF) != e["fx_now"]) \
                    or db.execute(f"SELECT 1 FROM {LOG} WHERE spellid = ?", (sid,)).fetchone():
                stale += 1
                continue
            if before is None:
                src = f"{SOURCE_NOTE} (2020)"
                db.execute("INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (?, '', ?, ?)", (sid, e["fx"], src))
                action = "insert"
            else:
                src = before["source"] if SOURCE_NOTE in (before["source"] or "") else f"{before['source']}; {SOURCE_NOTE}".lstrip("; ")
                db.execute("UPDATE spelleffect SET fxdict = ?, source = ? WHERE spellid = ?", (e["fx"], src, sid))
                action = "update"
            a = audit(db, action, sid, before, row_of(db, sid), DETAIL)
            db.execute(f"INSERT INTO {LOG} VALUES (?, ?, ?, ?, ?, ?, ?)",
                       (sid, int(before is not None), e["fx_now"], before["source"] if before else "", e["fx"], src, a))
            done += 1
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return done, stale


def undo(db):
    """Every logged row still as written back as it was; returns (back, left)."""
    if not db.execute(f"SELECT count(*) FROM sqlite_master WHERE name = '{LOG}'").fetchone()[0]:
        return 0, 0
    back = left = 0
    db.execute("BEGIN IMMEDIATE")
    try:
        for sid, had, fxb, srcb, fxa, srca, _ in db.execute(f"SELECT * FROM {LOG}").fetchall():
            now = row_of(db, sid)
            if now is None or (now["fxdict"] & 0xFFFFFFFF) != fxa or now["source"] != srca:
                left += 1
                continue
            if had:
                db.execute("UPDATE spelleffect SET fxdict = ?, source = ? WHERE spellid = ?", (fxb, srcb, sid))
                audit(db, "update", sid, now, row_of(db, sid), "undo: " + DETAIL)
            else:
                db.execute("DELETE FROM spelleffect WHERE spellid = ?", (sid,))
                audit(db, "delete", sid, now, None, "undo: " + DETAIL)
            back += 1
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return back, left


def backup(path):
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-eqoaemu-visuals"
    src, out = sqlite3.connect(path, timeout=30), sqlite3.connect(dst)
    try:
        src.backup(out)
    finally:
        out.close()
        src.close()
    return dst


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--sheet", help="EQOAEmu's workbook (.ods), the operator's copy")
    ap.add_argument("--iso", help="the Frontiers disc, for its visuals")
    ap.add_argument("--save-plan", metavar="FILE", help="write the plan (JSON) and stop")
    ap.add_argument("--plan", metavar="FILE", help="a plan --save-plan wrote, instead of --sheet/--iso")
    ap.add_argument("--no-backup", action="store_true")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    if (a.apply or a.undo) and not a.no_backup:
        print(f"copy: {backup(a.db)}")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        back, left = undo(db)
        print(f"put back {back}; left {left} changed since")
        return
    if a.plan:
        with open(a.plan, encoding="utf-8") as f:
            entries = json.load(f)["changes"]
        why = collections.Counter(e["kind"] for e in entries)
    else:
        if not (a.sheet and a.iso):
            sys.exit("--sheet and --iso (or --plan)")
        disc = disc_visuals(a.iso)
        print(f"the disc's visuals: {len(disc)}")
        entries, why = plan(db, ods_rows(a.sheet, "Spells"), disc)
    for k, n in why.most_common():
        print(f"  {k}: {n}")
    if a.save_plan:
        with open(a.save_plan, "w", encoding="utf-8") as f:
            json.dump({"made": datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds"),
                       "changes": entries}, f, indent=1, ensure_ascii=False)
        print(f"plan: {a.save_plan} ({len(entries)} changes)")
        return
    if a.apply:
        done, stale = apply(db, entries)
        print(f"changed {done}; {stale} changed since the plan, left")
    else:
        print(f"{len(entries)} changes: --apply to write")


if __name__ == "__main__":
    main()
