#!/usr/bin/env python3
"""The rows of jibrish's lane-1 code asks (work board, 2026-11-22), each after its binary is in:

  706  Coachwoman Darva is Coachman Darva: npcs.npc_name, npcclass.name, npcfaction.name (both shards).
  895  Coachman D'Larik is Coachman D`larik: the same three (both shards).
  823  Riddletic Porter and Rigalig Porter answer a talk: npcs.npc_type 0 -> 128 (both shards).
  875  Take a Shield Break (901008): spelleffect line "Self, Cancel Sacred Shield, Cancel Divine Shield", source
       video-audit, its workbook effects JSON emptied so the line is read (both shards).
  1659 Crusader's Aura (701122): spelleffect effects + line, spellpattern recast 900 + spelldesc, set whole whatever
       they are now (both shards: Heirloom's interim rows, Live's original ones).
  1350 Eldrich Rune (900616): its mitigation's "rune": true, the 3500 pool kept (--shard heirloom only).
  1660 The Grinch (classtemplates 'the grinch'): each summon's "life": 90 and the note's sentence (--shard heirloom
       only; the template is Heirloom's).

Each change is refused (and nothing written) where its row is not as expected: a rename whose old row is missing
and new name taken, a porter whose npc_type is neither 0 nor 128, a template without its three summons. A change
already as written is skipped. Report only by default; --apply writes every change in one transaction, a gm_audit
line each, and the log table jib_lane1_1122_log; --undo puts back each logged value whose row still holds what was
written (a row changed since is left and said). npcs rows are read at the server's start: a rename or npc_type is
seen at the next restart (the binary's swap); the rest at once. Run on each shard's database.

    tools/jib_lane1_20261122.py --db eqoa.db --shard heirloom|live [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "jib_lane1_1122_log"
DETAIL = "jibrish's lane-1 code asks' rows (board %s): tools/jib_lane1_20261122.py"
WHO, VIA = "dab", "tool"
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    id     INTEGER PRIMARY KEY AUTOINCREMENT,
    board  TEXT NOT NULL,
    tbl    TEXT NOT NULL,
    keycol TEXT NOT NULL,
    keyval TEXT NOT NULL,      -- JSON of the key's value
    col    TEXT NOT NULL,
    before TEXT,               -- JSON of the value as it was
    after  TEXT,               -- JSON of the value as written
    audit  INTEGER NOT NULL DEFAULT 0
)"""

RENAMES = [  # (board, old npcs name, new npcs name)
    ("706", "Coachwoman Darva", "Coachman Darva"),
    ("895", "Coachman D'Larik", "Coachman D`larik"),
]
PORTERS = ["Riddletic Porter", "Rigalig Porter"]

SHIELD_BREAK, SHIELD_BREAK_LINE = 901008, "Self, Cancel Sacred Shield, Cancel Divine Shield"

AURA = 701122
AURA_EFFECTS = ('{"v":1,"scope":"self","target_type":"Self","harmful":false,"duration_s":90,"stack":2,"level":60,'
                '"min_level":1,"req_equipment":"Any","sheet_type":"Offensive Proc","effects":[{"id":"0525","type":'
                '"Offensive Proc","kind":"proc","procs":[{"id":"0D57","type":"Dmg/Heal","kind":"hp","amount":200,'
                '"scale":"cha","scale_pct":60,"on":"offense","scope":"group","target_type":"Friendly","radius":30},'
                '{"id":"0D58","type":"Dmg/Heal","kind":"hp","amount":-200,"scale":"cha","scale_pct":50,"on":"offense",'
                '"scope":"target","target_type":"Enemy - Undead"}]}]}')
AURA_LINE = ("Self, Add Offensive Proc: Increase Hitpoints by 200 Increase 60% based on CHA to group (radius 30) when it "
             "hits, Add Offensive Proc: Decrease Hitpoints by 200 Increase 50% based on CHA to undead when it hits, 90sec")
AURA_DESC = ("Self & Group, Increase Hitpoints by 200 when Crusader hits; Increase 60% based on CHA, Decrease Hitpoints "
             "by 200 to undead; Increase 50% based on CHA, 90sec")
AURA_RECAST = 900

RUNE = 900616
GRINCH = "the grinch"
GRINCH_OLD_NOTE = ("The 90-second life jibrish asked for needs code (a summon's lifetime): until then they stay until "
                   "he dies or the fight ends.")
GRINCH_NEW_NOTE = "Each Mini Grinch leaves 90 s after it rises (life 90)."


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def get(db, tbl, keycol, key, col):
    r = db.execute(f"SELECT {col} FROM {tbl} WHERE {keycol} = ?", (key,)).fetchone()
    return None if r is None else r[0]


def plan(db, shard):
    """(changes, refusals, notes): each change (board, tbl, keycol, key, col, before, after)."""
    ch, refusals, notes = [], [], []

    def want(board, tbl, keycol, key, col, after):
        before = get(db, tbl, keycol, key, col)
        if before != after:
            ch.append((board, tbl, keycol, key, col, before, after))

    for t in ("npcs", "npcclass", "npcfaction", "spelleffect", "spellpattern"):
        if not table(db, t):
            refusals.append(f"no table {t} here")
    if refusals:
        return ch, refusals, notes

    for board, old, new in RENAMES:
        olds = db.execute("SELECT id FROM npcs WHERE npc_name = ?", (old,)).fetchall()
        news = db.execute("SELECT id FROM npcs WHERE npc_name = ?", (new,)).fetchall()
        if olds and news:
            refusals.append(f"#{board}: npcs holds both {old!r} {olds} and {new!r} {news}: left to a person")
            continue
        if not olds and not news:
            refusals.append(f"#{board}: no npcs row is {old!r} or {new!r}")
            continue
        for (i,) in olds:
            want(board, "npcs", "id", i, "npc_name", new)
        for tbl in ("npcclass", "npcfaction"):
            ko, kn = old.lower(), new.lower()
            if db.execute(f"SELECT 1 FROM {tbl} WHERE name = ?", (kn,)).fetchone() and \
                    db.execute(f"SELECT 1 FROM {tbl} WHERE name = ?", (ko,)).fetchone():
                refusals.append(f"#{board}: {tbl} holds both {ko!r} and {kn!r}: left to a person")
                continue
            for (rid,) in db.execute(f"SELECT rowid FROM {tbl} WHERE name = ?", (ko,)).fetchall():
                want(board, tbl, "rowid", rid, "name", kn)
        for tbl, col in (("lootcreature", "npc_name"), ("npcdialog", "name"), ("factionhits", "npc_name"),
                         ("npctemplate", "name"), ("queststeps", "target"), ("quests", "giver")):
            if table(db, tbl):
                try:
                    n = db.execute(f"SELECT count(*) FROM {tbl} WHERE lower({col}) = ?", (old.lower(),)).fetchone()[0]
                except sqlite3.OperationalError:
                    continue
                if n:
                    notes.append(f"#{board}: {tbl}.{col} still names {old!r} in {n} row(s): not changed here")

    for name in PORTERS:
        rows = db.execute("SELECT id, npc_type FROM npcs WHERE npc_name = ?", (name,)).fetchall()
        if not rows:
            notes.append(f"#823: no npcs row is {name!r}")
        for i, typ in rows:
            if typ not in (0, 128):
                refusals.append(f"#823: npcs {i} {name!r} is npc_type {typ}, neither 0 nor 128: left to a person")
            else:
                want("823", "npcs", "id", i, "npc_type", 128)

    if get(db, "spellpattern", "spellid", SHIELD_BREAK, "spellname") != "Take a Shield Break" or \
            get(db, "spelleffect", "spellid", SHIELD_BREAK, "line") is None:
        refusals.append(f"#875: spell {SHIELD_BREAK} is not Take a Shield Break with a spelleffect row")
    else:
        want("875", "spelleffect", "spellid", SHIELD_BREAK, "line", SHIELD_BREAK_LINE)
        want("875", "spelleffect", "spellid", SHIELD_BREAK, "source", "video-audit")
        want("875", "spelleffect", "spellid", SHIELD_BREAK, "effects", "")

    if get(db, "spellpattern", "spellid", AURA, "spellname") != "Crusader's Aura" or \
            get(db, "spelleffect", "spellid", AURA, "line") is None:
        refusals.append(f"#1659: spell {AURA} is not Crusader's Aura with a spelleffect row")
    else:
        want("1659", "spelleffect", "spellid", AURA, "effects", AURA_EFFECTS)
        want("1659", "spelleffect", "spellid", AURA, "line", AURA_LINE)
        want("1659", "spellpattern", "spellid", AURA, "recast", AURA_RECAST)
        want("1659", "spellpattern", "spellid", AURA, "spelldesc", AURA_DESC)

    if shard == "heirloom":
        eff = get(db, "spelleffect", "spellid", RUNE, "effects")
        if not eff:
            refusals.append(f"#1350: spelleffect {RUNE} has no effects")
        else:
            d = json.loads(eff)
            mits = [e.get("mitigation") for e in d.get("effects", []) if isinstance(e.get("mitigation"), dict)]
            if len(mits) != 1 or abs(mits[0].get("max", 0)) != 3500:
                refusals.append(f"#1350: spelleffect {RUNE}'s mitigation is {mits}, not one pool of 3500: left to a person")
            else:
                mits[0]["rune"] = True
                want("1350", "spelleffect", "spellid", RUNE, "effects", json.dumps(d, separators=(",", ":")))

        if not table(db, "classtemplates") or get(db, "classtemplates", "name", GRINCH, "abilities") is None:
            refusals.append(f"#1660: no classtemplates row {GRINCH!r}")
        else:
            ab = json.loads(get(db, "classtemplates", "name", GRINCH, "abilities"))
            sums = [a for a in ab if a.get("kind") == "summon" and a.get("npc") == "Mini Grinch"]
            if len(sums) != 3:
                refusals.append(f"#1660: {GRINCH!r} has {len(sums)} Mini Grinch summons, not 3: left to a person")
            else:
                for a in sums:
                    a["life"] = 90
                want("1660", "classtemplates", "name", GRINCH, "abilities", json.dumps(ab, separators=(",", ":")))
                note = get(db, "classtemplates", "name", GRINCH, "note") or ""
                if GRINCH_OLD_NOTE in note:
                    want("1660", "classtemplates", "name", GRINCH, "note", note.replace(GRINCH_OLD_NOTE, GRINCH_NEW_NOTE))
    return ch, refusals, notes


def audit(db, board, tbl, keycol, key, col, before, after, undo=False):
    if not table(db, "gm_audit"):
        return 0
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    detail = ("undo: " if undo else "") + DETAIL % board
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', 'update', ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, tbl, json.dumps({keycol: key}, separators=(",", ":")),
                      json.dumps({col: before}), json.dumps({col: after}), detail))
    return cur.lastrowid


def apply(db, ch):
    db.execute(LOG_SCHEMA)
    for board, tbl, keycol, key, col, before, after in ch:
        db.execute(f"UPDATE {tbl} SET {col} = ? WHERE {keycol} = ?", (after, key))
        aid = audit(db, board, tbl, keycol, key, col, before, after)
        db.execute(f"INSERT INTO {LOG} (board, tbl, keycol, keyval, col, before, after, audit) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
                   (board, tbl, keycol, json.dumps(key), col, json.dumps(before), json.dumps(after), aid))


def undo(db):
    """(back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for lid, board, tbl, keycol, keyval, col, before, after in db.execute(
            f"SELECT id, board, tbl, keycol, keyval, col, before, after FROM {LOG} ORDER BY id DESC").fetchall():
        key, b, a = json.loads(keyval), json.loads(before), json.loads(after)
        now = get(db, tbl, keycol, key, col)
        if now != a:
            left += 1
            print(f"  left: #{board} {tbl} {keycol}={key} {col}, changed since it was written: {now!r}")
            continue
        db.execute(f"UPDATE {tbl} SET {col} = ? WHERE {keycol} = ?", (b, key))
        audit(db, board, tbl, keycol, key, col, a, b, undo=True)
        db.execute(f"DELETE FROM {LOG} WHERE id = ?", (lid,))
        back += 1
    if left == 0:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def short(v):
    s = repr(v)
    return s if len(s) <= 90 else s[:87] + "..."


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    ap.add_argument("--shard", required=True, choices=["heirloom", "live"])
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
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
            print(f"put back {back}, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        ch, refusals, notes = plan(db, a.shard)
        for n in notes:
            print("note: " + n)
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not ch:
            print("applied already: nothing to do")
            return 0
        for board, tbl, keycol, key, col, before, after in ch:
            print(f"#{board} {tbl} {keycol}={key} {col}: {short(before)} -> {short(after)}")
        if not a.apply:
            print(f"report only: {len(ch)} value(s) would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            apply(db, ch)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {len(ch)} value(s), in gm_audit and {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
