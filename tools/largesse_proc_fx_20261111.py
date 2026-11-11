#!/usr/bin/env python3
"""Largesse of Plague's proc shows its own look, SPELLFX.CSF 0x830D8A97 (2026-11-11; the owner: "largesse of plague
when the pets proc is the wrong spell fx at the moment. it looks like a sickly cloud that dissipates quickly", then
"0x830d8a97 is the dictid for LoP proc"), on the server's code of the same branch (proc_fx: a proc's own look at what
each proc hits, a pet's, a player's, a creature template's pinned on the spell; an older server ignores it).

  spelleffect 700794 (Largesse of Plague): its Offensive Proc's proc (0D4E, 650 disease) takes "proc_fx":
    "0x830D8A97" (decimal 2198702743), its look at what each proc hits, wherever the buff is (a necromancer's pet,
    a player, Innoruuk's template);
  classtemplates "innoruuk": his Largesse proc's own fx 0x0CACDEE1 (the disease hit's look, board #597's,
    tools/fix_innoruuk_visuals_20261110.py) taken off, so the spell's look is the one there is (the code plays the
    spell's over it anyway); no Innoruuk template here (another shard): that part is skipped and said.

Refused, nothing written, when a row is not as expected (Largesse's effects not the EQOAEmu import's one proc of 650,
or another proc_fx on it; Innoruuk's proc not {"kind":"proc","spell_id":700794,"fx":212655841}). Report only by
default; --apply in one transaction, gm_audit lines, log largesse_proc_fx_1111_log; --undo exact (a row changed since
left and said). The spell is read at every cast (a Largesse already on a pet keeps its old look until recast); the
templates at the server's start or a GM panel template edit (reload_npcs).

    tools/largesse_proc_fx_20261111.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "largesse_proc_fx_1111_log"
WHO, VIA = "dab", "tool"
DETAIL = ("Largesse of Plague's proc look 0x830D8A97 (the owner, 2026-11-11: \"0x830d8a97 is the dictid for LoP proc\"): "
          "tools/largesse_proc_fx_20261111.py")
LARGESSE = 700794
PROC_ID = "0D4E"
PROC_FX = "0x830D8A97"
INNO_T = "innoruuk"
FX_DISEASE_HIT = 0x0CACDEE1
INNO_NOW = {"kind": "proc", "spell_id": LARGESSE, "fx": FX_DISEASE_HIT}
INNO_NEW = {"kind": "proc", "spell_id": LARGESSE}

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    keycol TEXT NOT NULL,
    keyval TEXT NOT NULL,
    col    TEXT NOT NULL,
    before TEXT NOT NULL,
    after  TEXT NOT NULL,
    audit  INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def compact(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False)


def audit(db, tbl, keycol, keyval, col, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', 'update', ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, tbl, compact({keycol: keyval}),
                      compact({col: before}), compact({col: after}), DETAIL))
    return cur.lastrowid


def plan_largesse(db):
    """(change or None, lines, refusals)."""
    r = db.execute("SELECT effects FROM spelleffect WHERE spellid = ?", (LARGESSE,)).fetchone()
    name = db.execute("SELECT spellname FROM spellpattern WHERE spellid = ?", (LARGESSE,)).fetchone()
    if r is None or name is None or name[0] != "Largesse of Plague":
        return None, [], [f"spell {LARGESSE}: {name[0] if name else 'no spellpattern'!r}, effects row "
                          f"{'there' if r else 'missing'}: want Largesse of Plague with its effects"]
    text = r[0]
    try:
        eff = json.loads(text)
    except ValueError:
        return None, [], [f"spelleffect {LARGESSE}: effects are not JSON: {text!r}"]
    if compact(eff) != text:
        return None, [], [f"spelleffect {LARGESSE}: effects not in their compact form (would not undo exactly)"]
    procs = [p for e in eff.get("effects", []) if e.get("kind") == "proc" for p in e.get("procs", [])]
    if (len(procs) != 1 or procs[0].get("id") != PROC_ID or procs[0].get("kind") != "hp" or procs[0].get("amount") != -650
            or procs[0].get("on") != "offense"):
        return None, [], [f"spelleffect {LARGESSE}: not the one offensive proc of 650 ({PROC_ID}): {text}"]
    p = procs[0]
    outer = [e for e in eff["effects"] if e.get("kind") == "proc"][0]
    if p.get("proc_fx") == PROC_FX and "proc_fx" not in outer:
        return None, [f"spelleffect {LARGESSE}: proc_fx {PROC_FX} already"], []
    if "proc_fx" in p or "proc_fx" in outer:
        return None, [], [f"spelleffect {LARGESSE}: another proc_fx there: {p.get('proc_fx', outer.get('proc_fx'))!r}"]
    p["proc_fx"] = PROC_FX
    after = compact(eff)
    return (("spelleffect", "spellid", LARGESSE, "effects", text, after),
            [f"spelleffect {LARGESSE} (Largesse of Plague): its proc {PROC_ID} takes proc_fx {PROC_FX}"], [])


def plan_innoruuk(db):
    r = db.execute("SELECT abilities FROM classtemplates WHERE name = ?", (INNO_T,)).fetchone()
    if r is None:
        return None, [f"no class template {INNO_T!r} here (another shard): his part skipped"], []
    text = r[0]
    try:
        kit = json.loads(text or "[]")
    except ValueError:
        return None, [], [f"classtemplates {INNO_T!r}: abilities are not JSON: {text!r}"]
    if compact(kit) != text:
        return None, [], [f"classtemplates {INNO_T!r}: abilities not in their compact form (would not undo exactly)"]
    ls = [i for i, a in enumerate(kit) if a.get("kind") == "proc" and a.get("spell_id") == LARGESSE]
    if len(ls) != 1:
        return None, [], [f"classtemplates {INNO_T!r}: {len(ls)} Largesse procs, want 1: {text}"]
    a = kit[ls[0]]
    if a == INNO_NEW:
        return None, [f"classtemplates {INNO_T!r}: his Largesse proc has no fx of its own already"], []
    if a != INNO_NOW:
        return None, [], [f"classtemplates {INNO_T!r}: his Largesse proc is {compact(a)}, want {compact(INNO_NOW)}"]
    kit[ls[0]] = INNO_NEW
    return (("classtemplates", "name", INNO_T, "abilities", text, compact(kit)),
            [f"classtemplates {INNO_T!r}: his Largesse proc's fx 0x{FX_DISEASE_HIT:08X} (#597's disease hit) taken off"], [])


def write(db, tbl, keycol, keyval, col, before, after):
    cur = db.execute(f"UPDATE {tbl} SET {col} = ? WHERE {keycol} = ? AND {col} = ?", (after, keyval, before))
    return cur.rowcount == 1


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, tbl, keycol, keyval, col, before, after, _ in db.execute(f"SELECT * FROM {LOG} ORDER BY seq DESC").fetchall():
        key = int(keyval) if keycol == "spellid" else keyval
        if not write(db, tbl, keycol, key, col, after, before):
            print(f"left: {tbl} {keyval} changed since")
            left += 1
            continue
        audit(db, tbl, keycol, key, col, after, before)
        back += 1
    if not left:
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
            print(f"put back {back} rows, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        if table(db, LOG):
            print(f"applied already ({LOG} is there): --undo first")
            return 0
        changes, lines, refusals = [], [], []
        for f in (plan_largesse, plan_innoruuk):
            c, l, r = f(db)
            lines += l
            refusals += r
            if c:
                changes.append(c)
        for line in lines:
            print(line)
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not changes:
            print("nothing to do")
            return 0
        if not a.apply:
            print(f"report only: {len(changes)} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            db.execute(LOG_SCHEMA)
            for tbl, keycol, keyval, col, before, after in changes:
                if not write(db, tbl, keycol, keyval, col, before, after):
                    raise RuntimeError(f"{tbl} {keyval} changed under the update")
                a_id = audit(db, tbl, keycol, keyval, col, before, after)
                db.execute(f"INSERT INTO {LOG} (tbl, keycol, keyval, col, before, after, audit) VALUES (?, ?, ?, ?, ?, ?, ?)",
                           (tbl, keycol, str(keyval), col, before, after, a_id))
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {len(changes)} rows, in gm_audit and {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
