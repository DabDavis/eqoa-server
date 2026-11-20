#!/usr/bin/env python3
"""petpass_loot_20261120.py: the loot lane's data fixes of the 2026-11-20 petition pass. Each part names its petition
and the shard(s) it is for; --shard picks them (every record is found by NAME or by its own table's rows: the shards'
ids differ).

  ascended  (Live only; a GM's Live petition 5315, approved as asked: "All ascended gear should NOT be dropping on
            live, period. QoV just dropped ascended gear"). Every loot row that names an Ascended record
            (itemascended) is pointed back at its base: loottables (the tuned bosses' tables ascend_boss_loot.py
            pointed at the copies on 2026-11-10: the quintessence of Veeshan's, Lord Nagafen's, ...), lootpool and
            npcitem (none today, kept for a later copy); a row whose table already holds the base is deleted instead,
            so no base's weight doubles. And every Ascended record is barred from the held drops (table heldbarred,
            database/helddrops.go): a base barred as boss loot let a creature holding its look drop the copy. Not
            touched: the class quests' Ascended epic rewards (quest choices, not drops) and the hand-in rule
            (questascended.go) that rewards an Ascended item for an Ascended one held already.
  bandit    (Live only; a GM's Live petition 5333, approved as asked: "The drop rates for statted items are way too
            high at bandits"). The bandits' own statted table, npcloot ("bandit", "wiki bandit": the wiki's Item List,
            Dark Gauntlets L27 and Silvery Bracelet L26), from 5% a kill to 1%. The world's category pools every
            creature rolls (uncommon 1 in 30, rare 1 in 240: lootrate's "jib" preset) are not the bandits' own and are
            left as they are.
  pine      (both; Live petition 5325: "bark of pine doesnt have a minium lvl requirement"). Bark of Pine (the
            forest table's dagger, 350 damage, STR 25, CR 18) had levelreq 0, so it dropped for any level and anyone
            could wear it. The forest table's weapons are level = damage / 10 (Glowing Mist, Roots of Timber and
            Meadows Jinx: 350 at 35): levelreq 35, and its forest pool row drops from creatures of 30-40 as every
            other row of the table does (levelreq -5 to +5), not 1-70.
  parchment (both; Heirloom petition 2474: "Mystical Parchment ... I cannot use it. I can't destroy it"). It is the
            Master Spellsmith's quest item on the Zaoran Plateau (quests "The Master Spellsmith's Gift": 2,000 with
            your side). It is destroyable (the player destroyed his three at 02:14 UTC); its description now says
            where it goes and when.
  legacy    (Heirloom only; Heirloom petition 2454: "ring of toxic breath having 115 str and 70 sta, for legacy rule,
            but it just got alot of lower stats, no legacy item"). petfix532 (2026-11-19 17:50 UTC, petitions
            2425/2427) took Ring of Toxic Breath from the copied Chameleon figures (STR 115, STA 70, durability 12,200
            / 65) to the source's (AGI 25, CHA 25, AC 50, 13,300 / 50) and made no Legacy copy. The Legacy rule (as
            Legacy Bright Ring of Zyphr and Legacy Splendor): a NO TRADE "Legacy <name>" record with the old figures,
            and the copies held from before the change made it. Made from the record petfix532 logged in gm_audit as
            it was; every held copy of durability 12,200 or less (the old record's: all four held today, worn,
            packed and banked) is pointed at it.
  refund    (Heirloom only; Heirloom petition 2472: "i used 54 Holly tokens on my Shield of Woe and it didnt change
            its graphic"). The Weapon Enchanter put Energized (the old energy shield 0x2EC56029's skin) on Larrybird's
            Tower of Woe, whose own look IS the old energy shield: vanilla lends a source nothing on itself, so nothing
            changed (the server now refuses that, free: branch petpass-20261120-loot). Its 54 Holly Tokens back onto
            his pack's Holly Token stack (a new pack row if none has room), and the copy's no-op aura row
            (instanceaura, tag "Energized") taken off. RUN IT WHILE HE IS OFFLINE: an online session's pack mirror
            does not see a database write (or give the 54 by the GM panel and run --parts aura).

Report only by default; --apply writes in one transaction, a gm_audit line a row and the log petpass_loot_1120_log
(every row before and after); a second --apply of a part does nothing; --undo puts back exactly what it wrote (a row
changed since is left and said; a made record still held is left) and drops the log. Copies the database first
(<db>.bak-<when>-pre-petpassloot) unless --no-backup.

WHEN IT TAKES EFFECT: loot tables and pools at once (each corpse's first opening); heldbarred at the next start; an
item record's level and description at once for new copies, at the holder's next login for held ones; the Legacy
swap and the refund at the holder's next login; the aura row at the panel's aura reload or the next start.

    python3 -E -s tools/petpass_loot_20261120.py --db eqoa.db --shard live|heirloom [--parts a,b] [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "petpass_loot_1120_log"
WHO, VIA = "dab", "tool"
SRC = "petpass1120"
DETAIL = "petition pass 2026-11-20, loot lane: tools/petpass_loot_20261120.py"

PARTS = {"ascended": {"live"}, "bandit": {"live"}, "pine": {"live", "heirloom"}, "parchment": {"live", "heirloom"},
         "legacy": {"heirloom"}, "refund": {"heirloom"}}

BANDIT, BANDIT_TABLE, BANDIT_FROM, BANDIT_TO = "bandit", "wiki bandit", 5, 1
PINE, PINE_LEVEL = "Bark of Pine", 35
PARCHMENT = "mystical parchment"
PARCHMENT_DESC = ("A Master Spellsmith on the Zaoran Plateau takes this parchment for an ability once you stand at "
                  "2,000 with the Soldiers of Rallos Zek, Mithaniel Marr's Crusaders or the Akessan Collective.")
TOXIC, TOXIC_AUDIT_DETAIL = "Ring of Toxic Breath", "petfix532_20261119.py%"
LEGACY_RANGE = (966000, 966999)
OWNER, TOKEN, REFUND, AURA_ITEM, AURA_TAG = "Larrybird", "Holly Token", 54, "Tower of Woe", "Energized"


class Refused(Exception):
    pass


def js(v):
    return None if v is None else json.dumps(v, separators=(",", ":"), ensure_ascii=False, sort_keys=True)


def has(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone() is not None


def row(db, tbl, key):
    where = " AND ".join(f'"{k}" = ?' for k in key)
    cur = db.execute(f'SELECT * FROM "{tbl}" WHERE {where}', list(key.values()))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


class Writer:
    def __init__(self, db, apply):
        self.db, self.apply, self.n = db, apply, 0
        self.at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
        a = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() \
            if has(db, "accountinfo") else None
        self.acct = a[0] if a else 0
        if apply and not has(db, LOG):
            db.execute(f"CREATE TABLE {LOG} (seq INTEGER PRIMARY KEY, part TEXT NOT NULL, tbl TEXT NOT NULL, "
                       f"rowkey TEXT NOT NULL, before TEXT, after TEXT)")

    def done(self, part, tbl, key, before, why):
        after = row(self.db, tbl, key)
        action = "insert" if before is None else "delete" if after is None else "update"
        self.db.execute(f"INSERT INTO {LOG} (part, tbl, rowkey, before, after) VALUES (?, ?, ?, ?, ?)",
                        (part, tbl, js(key), js(before), js(after)))
        self.db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, "
                        "detail) VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                        (self.at, self.acct, WHO, VIA, action, tbl, js(key), js(before), js(after),
                         f"{why}; {DETAIL}"))
        self.n += 1

    def update(self, part, tbl, key, cols, why):
        before = row(self.db, tbl, key)
        if before is None:
            raise Refused(f"{tbl} {key}: missing")
        if all(before.get(c) == v for c, v in cols.items()):
            return False
        print(f"  {part}: {tbl} {js(key)}: " + ", ".join(f"{c} {before.get(c)!r} -> {v!r}" for c, v in cols.items()))
        if self.apply:
            where = " AND ".join(f'"{k}" = ?' for k in key)
            self.db.execute(f'UPDATE "{tbl}" SET {", ".join(chr(34) + c + chr(34) + " = ?" for c in cols)} '
                            f'WHERE {where}', list(cols.values()) + list(key.values()))
            self.done(part, tbl, key, before, why)
        return True

    def delete(self, part, tbl, key, why):
        before = row(self.db, tbl, key)
        if before is None:
            return False
        print(f"  {part}: {tbl} {js(key)}: deleted ({why})")
        if self.apply:
            where = " AND ".join(f'"{k}" = ?' for k in key)
            self.db.execute(f'DELETE FROM "{tbl}" WHERE {where}', list(key.values()))
            self.done(part, tbl, key, before, why)
        return True

    def insert(self, part, tbl, r, key, why):
        if row(self.db, tbl, key) is not None:
            raise Refused(f"{tbl} {key}: taken already")
        print(f"  {part}: {tbl} {js(key)}: made")
        if self.apply:
            self.db.execute(f'INSERT INTO "{tbl}" ({", ".join(chr(34) + c + chr(34) for c in r)}) VALUES '
                            f'({", ".join("?" * len(r))})', list(r.values()))
            self.done(part, tbl, key, None, why)
        return True


def one(db, sql, args, what):
    rs = db.execute(sql, args).fetchall()
    if len(rs) != 1:
        raise Refused(f"{what}: {len(rs)} rows, want 1")
    return rs[0]


def part_ascended(db, w):
    why = "Live petition 5315 (a GM, approved): no Ascended record drops on Live"
    asc = dict(db.execute("SELECT patternid, base FROM itemascended").fetchall())
    if not asc:
        print("  ascended: no Ascended records")
        return
    for tbl, col in (("loottables", "tablename"), ("lootpool", "category || '|' || region || '|' || npc_name"),
                     ("npcitem", "npcid")):
        idcol = "id" if tbl != "npcitem" else "itemid"
        for rid, group, pid in db.execute(f"SELECT {idcol}, {col}, patternid FROM {tbl} WHERE patternid IN "
                                          f"(SELECT patternid FROM itemascended) ORDER BY {idcol}").fetchall():
            base = asc[pid]
            same = db.execute(f"SELECT 1 FROM {tbl} WHERE {col} = ? AND patternid = ? LIMIT 1", (group, base)).fetchone()
            if same:
                w.delete("ascended", tbl, {idcol: rid}, why + f": its base {base} is in the same table")
            else:
                w.update("ascended", tbl, {idcol: rid}, {"patternid": base}, why + ": the base in its place")
    if has(db, "heldbarred"):
        for pid in sorted(asc):
            if row(db, "heldbarred", {"patternid": pid}) is None:
                w.insert("ascended", "heldbarred", {"patternid": pid, "note": "Ascended: none drops on Live "
                         "(petition 5315)", "source": SRC}, {"patternid": pid}, why + ": never a held drop")


def part_bandit(db, w):
    r = row(db, "npcloot", {"npc_name": BANDIT, "tablename": BANDIT_TABLE})
    if r is None:
        raise Refused(f"npcloot {BANDIT}/{BANDIT_TABLE}: missing")
    if r["chance"] not in (BANDIT_FROM, BANDIT_TO):
        raise Refused(f"npcloot {BANDIT}/{BANDIT_TABLE}: chance {r['chance']}, not {BANDIT_FROM}")
    w.update("bandit", "npcloot", {"npc_name": BANDIT, "tablename": BANDIT_TABLE}, {"chance": BANDIT_TO},
             f"Live petition 5333 (a GM, approved): the bandits' statted table {BANDIT_FROM}% -> {BANDIT_TO}% a kill")


def part_pine(db, w):
    why = "Live petition 5325: Bark of Pine had no level; the forest table's level of 350 damage"
    pid, lvl = one(db, "SELECT patternid, levelreq FROM itempattern WHERE itemname = ? AND equipslot = 12", (PINE,), PINE)
    if lvl not in (0, PINE_LEVEL):
        raise Refused(f"{PINE}: levelreq {lvl}, not 0")
    w.update("pine", "itempattern", {"patternid": pid}, {"levelreq": PINE_LEVEL}, why)
    for rid, lo, hi in db.execute("SELECT id, minlevel, maxlevel FROM lootpool WHERE patternid = ?", (pid,)).fetchall():
        w.update("pine", "lootpool", {"id": rid}, {"minlevel": PINE_LEVEL - 5, "maxlevel": PINE_LEVEL + 5},
                 why + ": dropped by creatures of its level -5 to +5, as the table's other rows")


def part_parchment(db, w):
    pid, = one(db, "SELECT patternid FROM itempattern WHERE itemname = ? COLLATE NOCASE", (PARCHMENT,), PARCHMENT)
    w.update("parchment", "itempattern", {"patternid": pid}, {"itemdesc": PARCHMENT_DESC},
             "Heirloom petition 2474: the Master Spellsmith's parchment says where it goes")


def part_legacy(db, w):
    why = "Heirloom petition 2454: the Legacy rule for Ring of Toxic Breath (petfix532 lowered it)"
    pid, = one(db, "SELECT patternid FROM itempattern WHERE itemname = ?", (TOXIC,), TOXIC)
    b = db.execute("SELECT before FROM gm_audit WHERE tbl = 'itempattern' AND rowkey IN (?, ?) AND detail LIKE ? "
                   "ORDER BY id LIMIT 1", (js({"patternid": pid}), json.dumps({"patternid": pid}), TOXIC_AUDIT_DETAIL)
                   ).fetchone()
    if b is None:
        raise Refused(f"{TOXIC}: petfix532's gm_audit line not found")
    old = json.loads(b[0])
    if (old.get("str"), old.get("sta"), old.get("maxhp")) != (115, 70, 12200):
        raise Refused(f"{TOXIC}: the logged record is not STR 115 / STA 70 / 12,200: {old}")
    name = "Legacy " + TOXIC
    have = db.execute("SELECT patternid FROM itempattern WHERE itemname = ?", (name,)).fetchone()
    if have:
        lid = have[0]
    else:
        lid = (db.execute("SELECT MAX(patternid) FROM itempattern WHERE patternid BETWEEN ? AND ?", LEGACY_RANGE)
               .fetchone()[0] or LEGACY_RANGE[0]) + 1
        rec = {c: v for c, v in old.items() if c in {r[1] for r in db.execute("PRAGMA table_info(itempattern)")}}
        rec.update({"patternid": lid, "itemname": name, "trade": 1})
        w.insert("legacy", "itempattern", rec, {"patternid": lid}, why + ": the Legacy record, NO TRADE, the old figures")
    for iid, in db.execute("SELECT itemid FROM charinventory WHERE patternid = ? AND remainHP <= ? ORDER BY itemid",
                           (pid, old["maxhp"])).fetchall():
        w.update("legacy", "charinventory", {"itemid": iid}, {"patternid": lid}, why + ": a copy held from before")


def part_refund(db, w, parts):
    why = "Heirloom petition 2472: Energized lends the Tower of Woe nothing (its own look); 54 Holly Tokens back"
    sid, = one(db, "SELECT serverid FROM characters WHERE charName = ?", (OWNER,), OWNER)
    if "aura" in parts or "refund" in parts:
        rs = db.execute("SELECT a.itemid FROM instanceaura a JOIN charinventory c ON c.itemid = a.itemid JOIN "
                        "itempattern p ON p.patternid = c.patternid WHERE c.serverid = ? AND p.itemname = ? AND "
                        "a.tag = ?", (sid, AURA_ITEM, AURA_TAG)).fetchall()
        for iid, in rs:
            w.delete("refund", "instanceaura", {"itemid": iid}, why + ": the no-op aura off")
    if "refund" not in parts:
        return
    if has(db, LOG) and db.execute(f"SELECT 1 FROM {LOG} WHERE part = 'refund' AND tbl = 'charinventory'").fetchone():
        print("  refund: given already")
        return
    tok, top = one(db, "SELECT patternid, maxstack FROM itempattern WHERE itemname = ?", (TOKEN,), TOKEN)
    r = db.execute("SELECT itemid, stackleft FROM charinventory WHERE serverid = ? AND patternid = ? AND location = 0 "
                   "AND equiploc < 0 AND stackleft + ? <= ? ORDER BY itemid LIMIT 1", (sid, tok, REFUND, top)).fetchone()
    if r:
        w.update("refund", "charinventory", {"itemid": r[0]}, {"stackleft": r[1] + REFUND}, why)
    else:
        nxt = db.execute("SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory WHERE serverid = ? AND "
                         "location = 0", (sid,)).fetchone()[0]
        iid = db.execute("SELECT COALESCE(MAX(itemid), 0) + 1 FROM charinventory").fetchone()[0]
        w.insert("refund", "charinventory", {"itemid": iid, "serverid": sid, "stackleft": REFUND, "remainHP": 1,
                 "remaincharge": 0, "patternid": tok, "equiploc": -1, "location": 0, "listnumber": nxt},
                 {"itemid": iid}, why)


def undo(db):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    a = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() \
        if has(db, "accountinfo") else None
    n, left = 0, 0
    for tbl, rowkey, before, after in db.execute(f"SELECT tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key = json.loads(rowkey)
        if js(row(db, tbl, key)) != after:
            print(f"  {tbl} {rowkey}: changed since this tool wrote it, left as it is")
            left += 1
            continue
        where = " AND ".join(f'"{k}" = ?' for k in key)
        if before is None:
            if tbl == "itempattern" and db.execute("SELECT COUNT(*) FROM charinventory WHERE patternid = ?",
                                                   (key["patternid"],)).fetchone()[0]:
                print(f"  itempattern {key['patternid']}: still held, left")
                left += 1
                continue
            db.execute(f'DELETE FROM "{tbl}" WHERE {where}', list(key.values()))
        elif after is None:
            b = json.loads(before)
            db.execute(f'INSERT INTO "{tbl}" ({", ".join(chr(34) + c + chr(34) for c in b)}) VALUES '
                       f'({", ".join("?" * len(b))})', list(b.values()))
        else:
            b = json.loads(before)
            db.execute(f'UPDATE "{tbl}" SET {", ".join(chr(34) + c + chr(34) + " = ?" for c in b)} WHERE {where}',
                       list(b.values()) + list(key.values()))
        n += 1
    db.execute(f"DROP TABLE {LOG}")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', ?, '{}', NULL, NULL, ?)",
               (at, a[0] if a else 0, WHO, VIA, LOG, f"undone, {n} rows put back, {left} left: " + DETAIL))
    print(f"  {n} rows put back, {left} left")


def backup(path):
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-petpassloot"
    src, out = sqlite3.connect(path), sqlite3.connect(dst)
    with out:
        src.backup(out)
    src.close()
    out.close()
    print(f"  backup: {dst}")


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    ap.add_argument("--shard", required=True, choices=("live", "heirloom"))
    ap.add_argument("--parts", default="", help="comma list (default: every part of the shard); 'aura' alone: the "
                                                 "refund's aura row without the tokens")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    ap.add_argument("--no-backup", action="store_true")
    a = ap.parse_args(argv)
    parts = [p for p in a.parts.split(",") if p] or [p for p, s in PARTS.items() if a.shard in s]
    for p in parts:
        if p != "aura" and (p not in PARTS or a.shard not in PARTS[p]):
            print(f"refused: part {p!r} is not for the {a.shard} shard")
            return 2
    if (a.apply or a.undo) and not a.no_backup:
        backup(a.db)
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            if not has(db, LOG):
                print("nothing to undo")
                db.execute("ROLLBACK")
                return 0
            undo(db)
        else:
            w = Writer(db, a.apply)
            for p in parts:
                if p == "ascended":
                    part_ascended(db, w)
                elif p == "bandit":
                    part_bandit(db, w)
                elif p == "pine":
                    part_pine(db, w)
                elif p == "parchment":
                    part_parchment(db, w)
                elif p == "legacy":
                    part_legacy(db, w)
                elif p in ("refund", "aura"):
                    part_refund(db, w, parts)
            print(f"{'applied' if a.apply else 'report only'}: {w.n} rows written")
        db.execute("COMMIT" if (a.apply or a.undo) else "ROLLBACK")
    except Refused as e:
        db.execute("ROLLBACK")
        print(f"refused: {e}")
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
