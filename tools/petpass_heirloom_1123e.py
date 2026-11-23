#!/usr/bin/env python3
"""Heirloom petitions and owner calls of 2026-11-23 (the fifth batch of the day, the owner's "yes to all"), as data.

Every row is found by its name (and what else names it), never by an id alone, and must read as it did when this
was made; a row already as this writes it is left ("already"); anything else refuses the whole run.

  (owner) the nine level 46 class-spell scrolls that drop only from Uwu's spirits and phantoms (loot table "uwu 46
        spells", the owner's 2026-11-11 decision, kept as it is) put in every Spell Archivist's shop, as pass 1123d
        put Mark of Anguish there (petition 2778) and petition 1521 Shroud of Death: each archivist's Mark of Anguish
        row copied with the scroll's record. His "Grant basic spells" then writes them, and they can be bought.
  2799  Black Sapphire wore the green gem's icon (140265582, the cut gems' common one): it takes the dark gem's
        (-1071168822), read off Onyx (25640), which Hematite and Flawed Black Diamond wear too.
  (--more) the same for the two older records in Uwu's table no shop sold either: Rend (8702, warrior) and Greater
        Energy Storm (8664, wizard), approved after the first apply (its own log, so each part's --undo is its own).
  2798  (--waldorf, at Waldorf's logout) Waldorf's bearwere line taken back so he can take the wolfwere line, as
        petitions 2527 and 2598 took his ratwere and gatorwere lines: his three bearwere quests (Paws in the Snow
        101354, Bearwere Tails 101355, Mirin 101353: the third is what "turned" means, handlers/wereinfect.go
        turnedOf), the Bearwere form (901804) out of his book, a kept Bearwere effect if any, and his standing with
        the Werehunters of Tunaria (-1000, what the line's turning gave) set to 0 as he asked. His charinfection row
        (bearwere, ended "turned") is left: an ended row is replaced by the next bite (database.ApplyInfection).

Every row written is a gm_audit line (via "tool") and a row of the log; --undo puts each back as it was (a row
changed since is left and said) and drops the log. A report by default; --apply writes once (a second finds the log
and writes nothing); --apply and --undo copy the database first (<db>.bak-<when>-pre-pet1123e) unless --no-backup.

WHEN IT TAKES EFFECT: the archivists' shops at their next opening and the grant within 10 minutes (archiveTTL);
Black Sapphire at each holder's next login. --waldorf only with Waldorf out of the world (quests, book and
standings are a live session's and written at its logout): ~/charmigrate/watch-addr.sh runs it; he sees it at
his next login.

    python3 -E -s tools/petpass_heirloom_1123e.py --db eqoa.db            report
    python3 -E -s tools/petpass_heirloom_1123e.py --db eqoa.db --apply    write it
    python3 -E -s tools/petpass_heirloom_1123e.py --db eqoa.db --undo     take it back
    ... --more [--apply | --undo]                                          Rend and Greater Energy Storm
    ... --waldorf [--apply | --undo]                                       2798, at his logout
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "petpass_heirloom_1123e_log"
WALDORF_LOG = "petpass_heirloom_1123e_waldorf_log"
MORE_LOG = "petpass_heirloom_1123e_more_log"
WHO, VIA = "claude", "tool"
DETAIL = "Heirloom petitions 2798/2799 + the nine level 46 scrolls (2026-11-23): tools/petpass_heirloom_1123e.py"

# the nine scrolls: name -> (scroll record, classuse bit, spell, class)
SCROLLS = {
    "Entanglement": (716265, 2, 700535, 1),
    "Cleave": (716266, 4, 700534, 2),
    "Fierce Strike": (716267, 16, 700536, 4),
    "Lila's Refrain": (716268, 32, 199, 5),
    "Poison Dart": (716269, 64, 700542, 6),
    "Group Wolf Form": (716270, 128, 700538, 7),
    "Burning Blade": (716272, 1024, 700533, 10),
    "Cremation": (716273, 2048, 959, 11),
    "Arcane Bonding": (716274, 4096, 900588, 12),
}
# --more (the owner, the same day: "nine" was the earlier count): the other two in Uwu's table no shop sold
MORE = {
    "Rend": (8702, 1, 2675, 0),
    "Greater Energy Storm": (8664, 8192, 700537, 13),
}
UWU_TABLE = "uwu 46 spells"
ANGUISH, ANGUISH_SCROLL = "Mark of Anguish", 716271
ARCHIVIST = "Spell Archivist"

# 2799
BLACK_SAPPHIRE, BS_NAME, GREEN_GEM = 704208, "Black Sapphire", 140265582
DARK_GEM, DARK_FROM, DARK_FROM_NAME = -1071168822, 25640, "Onyx"

# 2798
WALDORF_SID, WALDORF = 152, "Waldorf"
BEAR_QUESTS = {"Weres/bearwere/p1": (101354, "Paws in the Snow"), "Weres/bearwere/p2": (101355, "Bearwere Tails"),
               "Weres/bearwere/p3": (101353, "Mirin")}
BEARWERE_FORM, BEARWERE_NAME = 901804, "Bearwere"
WEREHUNTERS, WH_OLD, WH_NEW = "Werehunters of Tunaria", -1000, 0

KEYS = {"npcitem": ("itemid",), "itempattern": ("patternid",), "charquests": ("serverid", "questid"),
        "spells": ("id",), "charbuffs": ("serverid", "seq"), "charfactions": ("serverid", "faction")}
LOG_DDL = """CREATE TABLE IF NOT EXISTS {log} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    ref    TEXT NOT NULL,
    before TEXT,
    after  TEXT,
    audit  INTEGER,
    at     TEXT NOT NULL
)"""
AUDIT_DDL = """CREATE TABLE IF NOT EXISTS gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER,
    username TEXT, via TEXT, ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT)"""


class Refused(Exception):
    pass


def go_json(m):
    """A row as Go's json.Marshal writes a map (keys sorted, <>& escaped), as the panel's audit lines are."""
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def rows(db, sql, args=()):
    cur = db.execute(sql, args)
    names = [c[0] for c in cur.description]
    return [dict(zip(names, r)) for r in cur.fetchall()]


def one(db, what, sql, args=()):
    got = rows(db, sql, args)
    if len(got) != 1:
        raise Refused(f"{what}: {len(got)} rows, want one")
    return got[0]


def has_table(db, name):
    return bool(db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone())


class Plan:
    def __init__(self, db, waldorf=False, more=False):
        self.db = db
        self.only_waldorf = waldorf
        self.only_more = more
        self.changes = []  # (tbl, key dict or None, before row or None, values or None: a delete)
        self.lines = []

    def set(self, tbl, row, values, olds, what):
        """values over row, when each column of olds reads its old; already as values: left."""
        if all(row[c] == v for c, v in values.items()):
            self.lines.append(f"already  {what}")
            return
        for c, old in olds.items():
            if row[c] != old:
                raise Refused(f"{what}: {c} reads {row[c]!r}, not {old!r} as read when this was made")
        key = {k: row[k] for k in KEYS[tbl]}
        self.changes.append((tbl, key, dict(row), dict(values)))
        self.lines.append(f"change   {what}: " + ", ".join(f"{c} {row[c]!r} -> {v!r}" for c, v in values.items()))

    def insert(self, tbl, values, what, present):
        """a new row, unless present (a row already there as wanted: left; another: refused)."""
        if present is not None:
            if all(present[c] == v for c, v in values.items()):
                self.lines.append(f"already  {what}")
                return
            raise Refused(f"{what}: a row is there already: {present}")
        self.changes.append((tbl, None, None, dict(values)))
        self.lines.append(f"insert   {what}")

    def delete(self, tbl, row, what):
        self.changes.append((tbl, {k: row[k] for k in KEYS[tbl]}, dict(row), None))
        self.lines.append(f"delete   {what}")

    def build(self):
        if self.only_waldorf:
            self.waldorf()
            return
        if self.only_more:
            self.scrolls(MORE)
            return
        self.scrolls(SCROLLS)
        self.black_sapphire()

    def scrolls(self, which):
        db = self.db
        anguish = one(db, f"{ANGUISH}'s scroll", "SELECT patternid FROM itempattern WHERE itemname = ? AND "
                      "classuse = 512 AND levelreq = 46 AND equipslot = -1", (ANGUISH,))
        if anguish["patternid"] != ANGUISH_SCROLL:
            raise Refused(f"{ANGUISH}'s scroll is {anguish['patternid']}, not {ANGUISH_SCROLL}")
        archs = [r["npc_id"] for r in rows(db, "SELECT DISTINCT npc_id FROM npcs WHERE npc_name = ? AND npc_id != 0 "
                                               "AND (npc_type & 1) = 1 ORDER BY npc_id", (ARCHIVIST,))]
        if not archs:
            raise Refused("no Spell Archivist")
        for name, (pid, bit, sid, cls) in which.items():
            sc = one(db, f"{name}'s scroll", "SELECT patternid FROM itempattern WHERE itemname = ? AND classuse = ? "
                     "AND levelreq = 46 AND equipslot = -1", (name, bit))
            if sc["patternid"] != pid:
                raise Refused(f"{name}'s scroll is {sc['patternid']}, not {pid}")
            one(db, f"{name} ({sid}) a class {cls} spell", "SELECT 1 FROM spellpattern s JOIN classspells c "
                "USING(spellid) WHERE s.spellid = ? AND s.spellname = ? AND c.class = ?", (sid, name, cls))
            one(db, f"{name} in Uwu's table", "SELECT 1 FROM loottables WHERE tablename = ? AND patternid = ?",
                (UWU_TABLE, pid))
            for a in archs:
                tmpl = rows(db, "SELECT * FROM npcitem WHERE npcid = ? AND patternid = ?", (a, ANGUISH_SCROLL))
                if len(tmpl) != 1:
                    raise Refused(f"archivist {a} holds {len(tmpl)} {ANGUISH} rows, want one")
                have = rows(db, "SELECT * FROM npcitem WHERE npcid = ? AND patternid = ?", (a, pid))
                vals = {c: v for c, v in tmpl[0].items() if c != "itemid"}
                vals["patternid"] = pid
                self.insert("npcitem", vals, f"{name} ({pid}) in archivist {a}'s shop", have[0] if have else None)

    def black_sapphire(self):
        db = self.db
        src = one(db, f"{DARK_FROM_NAME}'s icon", "SELECT itemicon FROM itempattern WHERE patternid = ? AND itemname = ?",
                  (DARK_FROM, DARK_FROM_NAME))
        if src["itemicon"] != DARK_GEM:
            raise Refused(f"{DARK_FROM_NAME} wears {src['itemicon']}, not the dark gem {DARK_GEM}")
        bs = one(db, BS_NAME, "SELECT * FROM itempattern WHERE patternid = ? AND itemname = ?", (BLACK_SAPPHIRE, BS_NAME))
        self.set("itempattern", bs, {"itemicon": src["itemicon"]}, {"itemicon": GREEN_GEM},
                 f"2799 {BS_NAME} ({BLACK_SAPPHIRE}) the dark gem's icon, as {DARK_FROM_NAME}'s")

    def waldorf(self):
        db = self.db
        one(db, WALDORF, "SELECT 1 FROM characters WHERE serverid = ? AND charname = ?", (WALDORF_SID, WALDORF))
        for key, (qid, qname) in BEAR_QUESTS.items():
            one(db, f"2798 {key}", "SELECT 1 FROM quest_import_quests k JOIN quests q USING(questid) WHERE k.key = ? "
                "AND q.questid = ? AND q.name = ?", (key, qid, qname))
            got = rows(db, "SELECT * FROM charquests WHERE serverid = ? AND questid = ?", (WALDORF_SID, qid))
            if got:
                self.delete("charquests", got[0], f"2798 {WALDORF}'s {qname} ({qid}, done {got[0]['done']})")
            else:
                self.lines.append(f"already  2798 {WALDORF}'s {qname} ({qid}): none")
        one(db, BEARWERE_NAME, "SELECT 1 FROM spellpattern WHERE spellid = ? AND spellname = ?",
            (BEARWERE_FORM, BEARWERE_NAME))
        got = rows(db, "SELECT * FROM spells WHERE serverid = ? AND spellid = ?", (WALDORF_SID, BEARWERE_FORM))
        for r in got:
            self.delete("spells", r, f"2798 {WALDORF}'s {BEARWERE_NAME} form ({r['id']}) out of his book")
        if not got:
            self.lines.append(f"already  2798 {WALDORF}'s {BEARWERE_NAME} form: none")
        if has_table(db, "charbuffs"):
            for r in rows(db, "SELECT * FROM charbuffs WHERE serverid = ? AND spellid = ?", (WALDORF_SID, BEARWERE_FORM)):
                self.delete("charbuffs", r, f"2798 {WALDORF}'s kept {BEARWERE_NAME} effect (seq {r['seq']})")
        f = rows(db, "SELECT * FROM charfactions WHERE serverid = ? AND faction = ?", (WALDORF_SID, WEREHUNTERS))
        if f:
            self.set("charfactions", f[0], {"standing": WH_NEW}, {"standing": WH_OLD},
                     f"2798 {WALDORF}'s standing with the {WEREHUNTERS}")
        else:
            self.insert("charfactions", {"serverid": WALDORF_SID, "faction": WEREHUNTERS, "standing": WH_NEW},
                        f"2798 {WALDORF}'s standing with the {WEREHUNTERS} 0", None)


def audit(db, action, tbl, key, before, after):
    db.execute(AUDIT_DDL)
    cur = db.execute("""INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after,
        detail) VALUES (?, 0, ?, ?, '', ?, ?, ?, ?, ?, ?)""",
                     (now(), WHO, VIA, action, tbl, go_json(key), go_json(before) if before else None,
                      go_json(after) if after else None, DETAIL))
    return cur.lastrowid


def where(key):
    return " AND ".join(f"{k} = ?" for k in key), list(key.values())


def keyof(tbl, row):
    return {k: row[k] for k in KEYS[tbl]}


def apply(db, plan, log):
    db.execute(LOG_DDL.format(log=log))
    for tbl, key, before, values in plan.changes:
        if values is None:  # a delete
            w, args = where(key)
            db.execute(f"DELETE FROM {tbl} WHERE {w}", args)
            a, after = audit(db, "delete", tbl, key, before, None), None
        elif before is None:
            cols = list(values)
            cur = db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                             [values[c] for c in cols])
            key = keyof(tbl, values) if all(k in values for k in KEYS[tbl]) else {KEYS[tbl][0]: cur.lastrowid}
            after = rows(db, f"SELECT * FROM {tbl} WHERE {where(key)[0]}", where(key)[1])[0]
            a = audit(db, "insert", tbl, key, None, after)
        else:
            w, args = where(key)
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in values)} WHERE {w}",
                       list(values.values()) + args)
            after = rows(db, f"SELECT * FROM {tbl} WHERE {w}", args)[0]
            a = audit(db, "update", tbl, key, before, after)
        db.execute(f"INSERT INTO {log} (tbl, ref, before, after, audit, at) VALUES (?, ?, ?, ?, ?, ?)",
                   (tbl, json.dumps(key), json.dumps(before) if before else None,
                    json.dumps(after) if after else None, a, now()))


def undo(db, log):
    out = []
    for r in rows(db, f"SELECT * FROM {log} ORDER BY seq DESC"):
        tbl, key = r["tbl"], json.loads(r["ref"])
        w, args = where(key)
        now_rows = rows(db, f"SELECT * FROM {tbl} WHERE {w}", args)
        if r["after"] is None:  # a delete: the row goes back unless one holds its key now
            if now_rows:
                out.append(f"left     {tbl} {key}: a row holds its key since")
                continue
            before = json.loads(r["before"])
            cols = list(before)
            db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                       [before[c] for c in cols])
            audit(db, "insert", tbl, key, None, before)
            out.append(f"restored {tbl} {key}")
            continue
        after = json.loads(r["after"])
        if len(now_rows) != 1 or now_rows[0] != after:
            out.append(f"left     {tbl} {key}: changed since")
            continue
        if r["before"] is None:
            db.execute(f"DELETE FROM {tbl} WHERE {w}", args)
            audit(db, "delete", tbl, key, after, None)
        else:
            before = json.loads(r["before"])
            cols = list(before)
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in cols)} WHERE {w}",
                       [before[c] for c in cols] + args)
            audit(db, "update", tbl, keyof(tbl, before), after, before)
        out.append(f"restored {tbl} {key}")
    db.execute(f"DROP TABLE {log}")
    return out


def backup(path):
    dst = f"{path}.bak-{datetime.datetime.now().strftime('%Y%m%d-%H%M%S')}-pre-pet1123e"
    src = sqlite3.connect(path)
    out = sqlite3.connect(dst)
    src.backup(out)
    out.close()
    src.close()
    return dst


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--undo", action="store_true")
    ap.add_argument("--no-backup", action="store_true")
    ap.add_argument("--waldorf", action="store_true", help="2798 alone (with Waldorf out of the world)")
    ap.add_argument("--more", action="store_true", help="Rend and Greater Energy Storm at the archivists")
    a = ap.parse_args(argv)
    if a.waldorf and a.more:
        ap.error("--waldorf and --more are separate parts")
    log = WALDORF_LOG if a.waldorf else MORE_LOG if a.more else LOG
    db = sqlite3.connect(a.db, timeout=5, isolation_level=None)
    db.execute("BEGIN IMMEDIATE")
    try:
        logged = has_table(db, log)
        if a.undo:
            if not logged:
                print("nothing to undo: no log")
                db.execute("ROLLBACK")
                return 0
            if not a.no_backup:
                print("backup", backup(a.db))
            for line in undo(db, log):
                print(line)
            db.execute("COMMIT")
            return 0
        if logged and a.apply:
            print(f"applied already ({log} exists): nothing written")
            db.execute("ROLLBACK")
            return 0
        plan = Plan(db, a.waldorf, a.more)
        plan.build()
        for line in plan.lines:
            print(line)
        if not a.apply:
            print(f"dry run: {len(plan.changes)} rows would change")
            db.execute("ROLLBACK")
            return 0
        if not a.no_backup:
            print("backup", backup(a.db))
        apply(db, plan, log)
        db.execute("COMMIT")
        print(f"applied: {len(plan.changes)} rows")
        return 0
    except Refused as e:
        db.execute("ROLLBACK")
        print("refused:", e, file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
