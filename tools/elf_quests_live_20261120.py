#!/usr/bin/env python3
"""The Tethelin elf class quests mirrored from Heirloom to Live (work board 1662 with the lore pass 1694, and 1719;
the owner's approval, 2026-11-20). Source: Heirloom's database (--source, read only), its CURRENT rows.

  1662 (Heirloom's 1661, the era rebuild of the elf druid L7-L20, ranger and rogue chains) with 1694 (its lore pass):
    - itempattern 991301-991318 made whole at the same ids (the quests name them);
    - itempattern fixes, the entry's columns only (FIXES), 700674's itemname too (1694) and 700158's lore itemdesc;
    - quest_import_items names of 700170, 700167, 700135, 700146;
    - npcitem: Merchant Kelindira sells 700674 (stack 1, equiploc -1, location 1, her next listnumber);
    - quests, queststeps and queststeptargets of 100722-100746 whole; 100721 step 2 rewarditems and the quest's notes;
      the wizard's and magician's copies' columns (COLUMN_COPIES);
    - Trapmaw: npcs modelid, npcrespawn seconds (and its note), its npcmove row.
  1719 (Heirloom's 1717, the elf bard chain):
    - itempattern 991319-991324 (made on Live 2026-11-19 by a petition fix: checked equal);
    - itempattern fixes (FIXES; 13 of the 16 were made on Live 2026-11-19, so 700128, 700150 and 700127 remain;
      700150's icon is another entry's, 1707, and is not copied);
    - quest_import_items names of 700127, 700128, 700150;
    - quests, queststeps and queststeptargets of 100707-100716 whole. Live's copy of 2026-11-19 had pointed
      100712, 100713 and 100716 at stand-in records 976078-976080 (Heirloom's renamed 700128, 700150, 700127,
      for which Live then had no record of the name); the quests now name 700128, 700150 and 700127 as Heirloom's do,
      a stand-in held by a character becomes its original (charinventory), and a stand-in nothing names is removed.

Characters part-way (charquests, by the step they were on before the copy; STEP_MOVES): a move from a loot step to a
loot step keeps what was counted, any other move counts from 0. 100713's moves of 1719 are not made: Live's 100713 has
had Heirloom's seven steps since 2026-11-19 (no character is part-way on it). The items of GIVES are given to a
character on that step (after a move) who holds none (the pack's next place; a full pack, 100, is reported for a GM).
Every other character part-way keeps its step and count; a step whose items changed is reported with what it holds.

Refused, nothing written: an id taken by another record, a target row not as Live had it before (an item's listed
column neither Heirloom's value before its entry nor its value now; a quest's rows not the fingerprint Live's had on
2026-11-20), a name not found once, a character left past a quest's last step.

Quests, npcs and npcmove load at the server's start, and the server holds a player's pack and quests while they play:
run it in a keep restart's before.sh. Report only by default; --apply in one transaction, a gm_audit line a row, every
row's before and after in elf_quests_live_1120_log; a second --apply writes nothing; --undo puts back every row as it
was (its rowid too), a row changed since left and listed.

    python3 -E -s tools/elf_quests_live_20261120.py --db eqoa.db --source HEIRLOOM.db [--apply | --undo]
"""
import argparse
import datetime
import hashlib
import json
import sqlite3
import sys

LOG = "elf_quests_live_1120_log"
TOOL = "tools/elf_quests_live_20261120.py"
WHO, VIA = "dab", "tool"
DETAIL = ("Tethelin elf quests mirrored from Heirloom (work board 1662 with 1694, and 1719; the owner 2026-11-20): " + TOOL)
INV_CAP = 100  # the pack, worn items among them (config pack_slots)

# Heirloom's audits of the entries: an item's or name's row before its first audit here is what Live must still have.
SOURCE_AUDITS = ((110694, 110941), (112100, 112233), (114068, 114163))

NEW_ITEMS = list(range(991301, 991319)) + list(range(991319, 991325))

_SCROLL = "itemdesc itemicon procanim classuse raceuse levelreq"
FIXES = {
    # 1662 section 2 (with 1694: 700674 itemname, 700158 itemdesc)
    700170: "itemname itemdesc lore maxstack levelreq itemicon",
    700167: "itemname itemdesc equipslot attacktype weapondamage levelreq str dex HPMAX classuse raceuse itemcost maxhp "
            "model itemicon",
    700135: "itemname itemdesc lore itemicon procanim classuse raceuse levelreq",
    700146: "itemname itemdesc lore itemicon procanim classuse raceuse levelreq",
    700138: _SCROLL, 700155: _SCROLL, 700165: _SCROLL, 700156: _SCROLL,
    700673: _SCROLL + " lore",
    700171: "itemdesc procanim classuse raceuse levelreq lore maxstack",
    700158: "itemdesc levelreq",
    700159: "itemdesc craft",
    700163: "itemdesc", 700669: "itemdesc", 700672: "itemdesc", 700164: "itemdesc",
    700674: "itemcost classuse raceuse lore itemname",
    # 1719 section 2
    8316: "itemcost", 8318: "itemcost",
    6542: "itemdesc", 700143: "itemdesc", 700129: "itemdesc", 700173: "itemdesc", 700148: "itemdesc",
    700160: "itemdesc", 700144: "itemdesc",
    700128: "itemname itemdesc", 700150: "itemname itemdesc",
    700126: _SCROLL, 700147: _SCROLL, 700169: _SCROLL,
    700127: "itemname " + _SCROLL,
    700136: "itemdesc procanim classuse raceuse levelreq HoT",
}
IMPORT_NAMES = (700170, 700167, 700135, 700146, 700127, 700128, 700150)

# Live's stand-ins (a petition fix of 2026-11-19) -> the record Heirloom's quests name
STAND_INS = {976078: 700128, 976079: 700150, 976080: 700127}
HOLDERS = (("charinventory", "patternid"),)  # where a stand-in held is made its original
# where a stand-in still named keeps it (never removed while any of these names it)
NAMERS = (("charinventory", "patternid"), ("defaultcharinventory", "patternid"), ("npcitem", "patternid"),
          ("loottables", "patternid"), ("lootpool", "patternid"), ("npcheld", "patternid"), ("itemeffect", "patternid"),
          ("itemascended", "patternid"), ("startitems", "patternid"), ("gearsets", "mainpattern"),
          ("gearsets", "offpattern"), ("auction_lots", "patternid"), ("mail_attachments", "patternid"),
          ("charbuyback", "patternid"), ("itemowner", "patternid"), ("charmercitem", "patternid"),
          ("itemspell", "patternid"), ("itemaura", "patternid"), ("lootdroptrigger", "patternid"))

WHOLE_QUESTS = list(range(100707, 100717)) + list(range(100722, 100747))
COLUMN_COPIES = {  # quest: (quest columns, {step: step columns})
    100721: (("notes",), {2: ("rewarditems",)}),
    100059: ((), {s: ("chance", "journal", "items") for s in (3, 4, 5, 9, 10)}),
    100335: ((), {s: ("chance", "journal", "items") for s in (3, 4, 5, 9, 10)}),
    100060: ((), {s: ("journal",) for s in (2, 3, 10, 11)}),
    100336: ((), {2: ("journal",), 3: ("journal",), 9: ("says",), 10: ("journal",), 11: ("journal",)}),
}

# Live's quests as they were on 2026-11-20 (sha256 of quest_fingerprint): a quest not so, and not as Heirloom's, refuses
LIVE_BEFORE = {
    100059: "4d7e1334728384435fcb3e67aaad696969ec6674c7238fe40303b1d625918f7b",
    100060: "4303bc56531c55eb28f7fa2ce6604437683ce8fc585882ca1618140696943fc4",
    100335: "5cc5e532c7512ecdf7eccdae268ab5bc6274641bb3e4cd66660964d2d501109d",
    100336: "84872ff0a730ad133708d45c1d3ff75134d7f1cc44db588581601e04c975f20e",
    100712: "d299a97cdb8365c4a6c573d2c546cc9cb1c8835e60c466003e297666f230cb83",
    100713: "5bc43a671c3073a5bfb40ce55fe957d9559bfdb94006ff5b4d688d931cd12777",
    100716: "491fc23ff030efcc6cf207c6a66ecadc4e16e17ab922ebe6c23e3925a16cd203",
    100721: "4718fb86456498e6af098b2e15425c19cd715c0232c39eac3738e20296b4b395",
    100722: "11287bb3a3ad39a623ba9ff8ea46b261184e3bfc3c4170743f4ef04b68a78d14",
    100723: "ba5cc9500a88a79c9a5776f1fdd5b420adbef414da867d09d44ac7f546e8acfa",
    100724: "3eacdaf135061a0585c1033d87902a6cc42f1b552a17b42b5983394d47e2c93c",
    100725: "383eef251cfa4144c6b3f4337cf69e3d1338dc69407a4c2936a63ad2eebe8619",
    100726: "6350728727ce897a25ebedb24445e9b0001265ff85b51f9f8c351ef6f8c57416",
    100727: "a8092783436ebc7fd416fd5377aa8e9b391e79f0410d3ffffbd7f82a3bc19e61",
    100728: "7a40f919b4470c2ae49ed27a998f5179c9ccd01ccb46f47f2a8cc33fce945e2f",
    100729: "ef517fbc73e6073826053e18e8a72e49eac6eef55363cdffadac5838dbfa5fff",
    100730: "07bb16f0a694f0d2a2e3e9e4898ea831bce97016f2663d53ac44c2aaedb709e7",
    100731: "8fe71b43528f58dee0e56baa917ba79668ae41051c2bfed3afeb69c1189b1854",
    100732: "05bcfda49e65b7f47d5fe2c81041b785f5054f8629a49e31f27b693b578ed5de",
    100733: "db1d10ad1efec4f2f296eb342a3890270997edbbbf969833533f4910006ac18a",
    100734: "344e102a3caf8b42c530aa9fe75229fe4cc8ec985d3813f1c451f687a20f229d",
    100735: "a77f1c250df048ec592910b784d5f91dac83c3b2e8f9cb15fc04c81aaba98ced",
    100736: "6ee28ac7080e225c1f06c75868e11fb4dd3b1efa1b05a52e3248be5dfb76460b",
    100737: "b7ca68236bcc82ea707d37c5f9f1efc5c8610208b96f1dd52664bce82680f232",
    100738: "2c928300d7a096c5b5347c52dfa5a1fa3d7a6086989d4e8f73e02bffc77012da",
    100739: "de91441efb015db43b3a88b4bb953457a75cbba32593d36e52880fbce797926a",
    100740: "83398e39c2525d44562a7e4bca2a17899961b7c563d23393ac54db0943d88cb7",
    100741: "43580eeb0f36c691f543f61d4cea4174e9f078cf51123d068636e3f1e4188a3d",
    100742: "2b7087566861f75ed36e3c762b241861f9ab402595f6658e91992388f9a449e3",
    100743: "17e5c9e482177e4c773b175d4a92bdb8b5c7bbd7ed4d769ecf1e13ab803a9a77",
    100744: "2526b5e73a2091f0cbc89764071329739e0df21957644cabf5dc616d28b51357",
    100745: "55386e2843d3df316d63995ccc289b1b17f652f1db4b4ada4c9102b40625102b",
    100746: "1d3baddb86b9ac98df4403ffc85041445c2688fa7f02137557cf020cd9025c62",
}

# 1662 section 7: old step -> new step (made only while Live's quest is as LIVE_BEFORE has it)
STEP_MOVES = {
    100727: {1: 1, 2: 1},
    100728: {1: 1, 2: 1, 3: 2, 4: 3},
    100730: {1: 1, 2: 1, 3: 2},
    100737: {1: 1, 2: 1},
    100740: {1: 1, 2: 1, 3: 2},
    100742: {1: 1, 2: 1, 3: 2, 4: 3},
}
# (quest, step after the copy) -> the item a character there is given when it holds none
GIVES = {}
for _q in (100723, 100733, 100743):
    GIVES[(_q, 8)] = 991314
for _q in (100725, 100735, 100745, 100059, 100335):
    GIVES[(_q, 10)] = 991313
for _q, _bone in ((100726, 991312), (100736, 991310), (100746, 991311)):
    GIVES.update({(_q, 1): 991315, (_q, 4): 991316, (_q, 7): 991318, (_q, 9): _bone, (_q, 12): 991317})
GIVES.update({(100713, 1): 991321, (100715, 8): 991323, (100716, 8): 991324})

MERCHANT, MERCHANT_ITEM = "Merchant Kelindira", 700674
TRAPMAW = "Trapmaw"

KEYS = {"itempattern": ("patternid",), "quest_import_items": ("patternid",), "npcitem": ("itemid",),
        "quests": ("questid",), "queststeps": ("questid", "step"), "queststeptargets": ("questid", "step", "target"),
        "npcs": ("id",), "npcrespawn": ("npc_row",), "npcmove": ("name",), "charquests": ("serverid", "questid"),
        "charinventory": ("itemid",)}

SEQ = LOG + "_seq"  # an AUTOINCREMENT table's sqlite_sequence before and after, put back by --undo
SEQ_DDL = f"CREATE TABLE IF NOT EXISTS {SEQ} (tbl TEXT PRIMARY KEY, before INTEGER, after INTEGER)"

LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    part   TEXT NOT NULL,
    tbl    TEXT NOT NULL,
    rowkey TEXT NOT NULL,   -- the key (JSON)
    before TEXT,            -- the whole row before, its rowid as _rowid_ (JSON; NULL: there was none)
    after  TEXT,            -- the whole row after (JSON; NULL: removed)
    audit  INTEGER NOT NULL,
    at     TEXT NOT NULL
)"""


class Refused(Exception):
    pass


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def dumps(v):
    return json.dumps(v, sort_keys=True, separators=(",", ":"), ensure_ascii=False)


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def columns(db, tbl):
    return [r[1] for r in db.execute(f"PRAGMA table_info({tbl})")]


def rows(db, sql, args=()):
    cur = db.execute(sql, args)
    cols = [d[0] for d in cur.description]
    return [dict(zip(cols, r)) for r in cur.fetchall()]


def where(tbl):
    return " AND ".join(f'"{c}" = ?' for c in KEYS[tbl])


def keyof(tbl, r):
    return tuple(r[c] for c in KEYS[tbl])


def full(db, tbl, key):
    """the row with its rowid (_rowid_), or None."""
    r = rows(db, f"SELECT rowid AS _rowid_, * FROM {tbl} WHERE {where(tbl)}", key)
    return r[0] if r else None


def plain(r):
    return None if r is None else {c: v for c, v in r.items() if c != "_rowid_"}


def same(a, b):
    """the same value of the same storage class (what a copy must leave)."""
    return type(a) is type(b) and a == b


def loose(a, b):
    """the same value (gm_audit's JSON, the server's, writes a whole REAL as an integer)."""
    return a == b


def quest_fingerprint(db, q):
    """sha256 of a quest's rows (quests, queststeps, queststeptargets), every column, typed."""
    h = hashlib.sha256()
    for sql in ("SELECT * FROM quests WHERE questid = ?", "SELECT * FROM queststeps WHERE questid = ? ORDER BY step",
                "SELECT * FROM queststeptargets WHERE questid = ? ORDER BY step, target"):
        for r in db.execute(sql, (q,)):
            h.update(repr(tuple((type(v).__name__, v) for v in r)).encode())
        h.update(b"|")
    return h.hexdigest()


def source_before(src, tbl, key):
    """the row Heirloom had before its first audit in SOURCE_AUDITS, or None when none of them touched it."""
    rk = dumps(dict(zip(KEYS[tbl], key)))
    for lo, hi in SOURCE_AUDITS:
        r = src.execute("SELECT before FROM gm_audit WHERE tbl = ? AND rowkey = ? AND id BETWEEN ? AND ? ORDER BY id LIMIT 1",
                        (tbl, rk, lo, hi)).fetchone()
        if r is not None and r[0]:
            return json.loads(r[0])
    return None


class Plan:
    def __init__(self):
        self.ops = []      # (part, tbl, key, before row|None, after values|None)  after None: delete
        self.lines = []
        self.refusals = []

    def say(self, s):
        self.lines.append(s)

    def refuse(self, s):
        self.refusals.append(s)

    def upsert(self, part, db, tbl, key, values):
        """the row at key made values (every column given); nothing when it is so."""
        have = full(db, tbl, key)
        if have is not None and all(same(have[c], values[c]) for c in values):
            return False
        self.ops.append((part, tbl, key, have, dict(values)))
        return True

    def delete(self, part, db, tbl, key):
        have = full(db, tbl, key)
        if have is not None:
            self.ops.append((part, tbl, key, have, None))

    def insert(self, part, tbl, values):
        self.ops.append((part, tbl, None, None, dict(values)))


def plan(db, src):
    p = Plan()
    for side, d in (("Live", db), ("Heirloom", src)):
        for t in ("itempattern", "quests", "queststeps", "queststeptargets", "quest_import_items", "npcitem", "npcs",
                  "npcrespawn", "npcmove", "charquests", "charinventory", "gm_audit"):
            if not table(d, t):
                p.refuse(f"{side} has no {t} table")
    if p.refusals:
        return p
    for t in ("itempattern", "quests", "queststeps", "queststeptargets", "npcitem", "npcs", "npcrespawn", "npcmove"):
        if columns(db, t) != columns(src, t):
            p.refuse(f"{t}: Live's columns differ from Heirloom's")
    if p.refusals:
        return p

    # 1. new items, at Heirloom's ids
    made = 0
    for i in NEW_ITEMS:
        h = full(src, "itempattern", (i,))
        if h is None:
            p.refuse(f"itempattern {i}: Heirloom has no such record")
            continue
        have = full(db, "itempattern", (i,))
        if have is None:
            p.upsert("items", db, "itempattern", (i,), plain(h))
            made += 1
        elif not all(same(have[c], h[c]) for c in plain(h)):
            p.refuse(f"itempattern {i}: taken on Live by {have['itemname']!r}, not Heirloom's {h['itemname']!r} as it is")
    p.say(f"items: {made} of {len(NEW_ITEMS)} records to make ({len(NEW_ITEMS) - made} there already)")

    # 2. item fixes, the listed columns
    fixed = 0
    for i, cols in FIXES.items():
        cols = cols.split()
        h, have = full(src, "itempattern", (i,)), full(db, "itempattern", (i,))
        if h is None or have is None:
            p.refuse(f"itempattern {i}: missing on {'Heirloom' if h is None else 'Live'}")
            continue
        b = source_before(src, "itempattern", (i,))
        bad = [c for c in cols if not same(have[c], h[c]) and (b is None or not loose(have[c], b.get(c)))]
        if bad:
            p.refuse(f"itempattern {i} ({have['itemname']}): " + "; ".join(
                f"{c} is {have[c]!r}, neither Heirloom's {h[c]!r} nor its before {None if b is None else b.get(c)!r}" for c in bad))
            continue
        if b is not None and not loose(have["itemname"], b["itemname"]) and not same(have["itemname"], h["itemname"]):
            p.refuse(f"itempattern {i}: Live's name {have['itemname']!r} is neither {b['itemname']!r} nor {h['itemname']!r}")
            continue
        new = dict(plain(have), **{c: h[c] for c in cols})
        if p.upsert("item fixes", db, "itempattern", (i,), new):
            fixed += 1
            ch = [c for c in cols if not same(have[c], h[c])]
            p.say(f"  item {i} {have['itemname']!r}: {', '.join(ch)}"
                  + (f" (-> {h['itemname']!r})" if "itemname" in ch else ""))
    p.say(f"item fixes: {fixed} of {len(FIXES)} records to change")

    # 3. quest_import_items names
    for i in IMPORT_NAMES:
        h, have = full(src, "quest_import_items", (i,)), full(db, "quest_import_items", (i,))
        if h is None or have is None:
            p.refuse(f"quest_import_items {i}: missing on {'Heirloom' if h is None else 'Live'}")
            continue
        b = source_before(src, "quest_import_items", (i,))
        if not same(have["name"], h["name"]) and (b is None or not loose(have["name"], b["name"])):
            p.refuse(f"quest_import_items {i}: Live's name {have['name']!r}, not Heirloom's before")
            continue
        if p.upsert("import names", db, "quest_import_items", (i,), dict(plain(have), name=h["name"])):
            p.say(f"  quest_import_items {i}: {have['name']!r} -> {h['name']!r}")

    # 4. the merchant sells the vest
    mine = db.execute("SELECT id FROM npcs WHERE npc_name = ?", (MERCHANT,)).fetchall()
    theirs = src.execute("SELECT id FROM npcs WHERE npc_name = ?", (MERCHANT,)).fetchall()
    if len(mine) != 1 or len(theirs) != 1:
        p.refuse(f"{MERCHANT}: {len(mine)} rows on Live, {len(theirs)} on Heirloom (one each wanted)")
    else:
        hs = rows(src, "SELECT * FROM npcitem WHERE npcid = ? AND patternid = ?", (theirs[0][0], MERCHANT_ITEM))
        if len(hs) != 1:
            p.refuse(f"{MERCHANT} on Heirloom sells {MERCHANT_ITEM} {len(hs)} times (once wanted)")
        elif not db.execute("SELECT 1 FROM npcitem WHERE npcid = ? AND patternid = ?", (mine[0][0], MERCHANT_ITEM)).fetchone():
            nxt = db.execute("SELECT COALESCE(MAX(listnumber), -1) + 1 FROM npcitem WHERE npcid = ?", (mine[0][0],)).fetchone()[0]
            v = {c: hs[0][c] for c in hs[0] if c != "itemid"}
            v.update(npcid=mine[0][0], listnumber=nxt)
            p.insert("merchant", "npcitem", v)
            p.say(f"merchant: {MERCHANT} (row {mine[0][0]}) sells {MERCHANT_ITEM} at listnumber {nxt}")

    # 5. quests
    QS = WHOLE_QUESTS + sorted(COLUMN_COPIES)
    copy = []
    for q in QS:
        if not src.execute("SELECT 1 FROM quests WHERE questid = ?", (q,)).fetchone():
            p.refuse(f"quest {q}: Heirloom has none")
            continue
        hq = src.execute("SELECT giver, name FROM quests WHERE questid = ?", (q,)).fetchone()
        lq = db.execute("SELECT giver FROM quests WHERE questid = ?", (q,)).fetchone()
        fp = quest_fingerprint(db, q)
        if q in WHOLE_QUESTS and fp == quest_fingerprint(src, q):
            continue
        if q in COLUMN_COPIES and column_copy_done(db, src, q):
            continue
        if fp != LIVE_BEFORE.get(q):
            p.refuse(f"quest {q} ({hq[1]}): Live's rows are neither as they were on 2026-11-20 nor Heirloom's"
                     + ("" if lq else " (Live has no such quest)"))
            continue
        copy.append(q)
    p.copied = copy
    nq = ns = nd = nt = 0
    for q in copy:
        if q in WHOLE_QUESTS:
            h = rows(src, "SELECT * FROM quests WHERE questid = ?", (q,))[0]
            nq += p.upsert("quests", db, "quests", (q,), h)
            hsteps = rows(src, "SELECT * FROM queststeps WHERE questid = ? ORDER BY step", (q,))
            for s in hsteps:
                ns += p.upsert("quests", db, "queststeps", (q, s["step"]), s)
            hn = {s["step"] for s in hsteps}
            for (st,) in db.execute("SELECT step FROM queststeps WHERE questid = ? ORDER BY step", (q,)).fetchall():
                if st not in hn:
                    p.delete("quests", db, "queststeps", (q, st))
                    nd += 1
            ht = set(src.execute("SELECT step, target FROM queststeptargets WHERE questid = ?", (q,)).fetchall())
            lt = set(db.execute("SELECT step, target FROM queststeptargets WHERE questid = ?", (q,)).fetchall())
            for st, tg in sorted(lt - ht):
                p.delete("quests", db, "queststeptargets", (q, st, tg))
                nt += 1
            for st, tg in sorted(ht - lt):
                p.upsert("quests", db, "queststeptargets", (q, st, tg), {"questid": q, "step": st, "target": tg})
                nt += 1
        else:
            qcols, steps = COLUMN_COPIES[q]
            if qcols:
                h, have = full(src, "quests", (q,)), full(db, "quests", (q,))
                nq += p.upsert("quests", db, "quests", (q,), dict(plain(have), **{c: h[c] for c in qcols}))
            for st, cols in steps.items():
                h, have = full(src, "queststeps", (q, st)), full(db, "queststeps", (q, st))
                if h is None or have is None:
                    p.refuse(f"queststeps {q}/{st}: missing on {'Heirloom' if h is None else 'Live'}")
                    continue
                ns += p.upsert("quests", db, "queststeps", (q, st), dict(plain(have), **{c: h[c] for c in cols}))
    p.say(f"quests: {len(copy)} to copy ({nq} quests rows, {ns} steps written, {nd} steps removed, {nt} step targets)")

    # 5b. the stand-ins
    for sid, orig in sorted(STAND_INS.items()):
        have = full(db, "itempattern", (sid,))
        if have is None:
            continue
        o, h = full(db, "itempattern", (orig,)), full(src, "itempattern", (orig,))
        if o is None or not same(have["itemname"], h["itemname"]):
            p.refuse(f"itempattern {sid} {have['itemname']!r}: not a stand-in for {orig} as Heirloom names it")
            continue
        named = []
        for st in rows(db, "SELECT questid, step, items, rewarditems, events, choices, byclass FROM queststeps"):
            if any(str(sid) in (st[c] or "") for c in ("items", "rewarditems", "events", "choices", "byclass")):
                moved = any(o_[1] == "queststeps" and o_[2] == (st["questid"], st["step"]) for o_ in p.ops)
                if not (moved and st["questid"] in copy):
                    named.append(f"queststeps {st['questid']}/{st['step']}")
        for tbl, col in HOLDERS:
            for r in rows(db, f"SELECT rowid AS _rowid_, * FROM {tbl} WHERE {col} = ?", (sid,)):
                p.ops.append(("stand-ins", tbl, keyof(tbl, r), r, dict(plain(r), **{col: orig})))
                p.say(f"  {tbl} {keyof(tbl, r)}: stand-in {sid} held by character {r['serverid']} becomes {orig}")
        for tbl, col in NAMERS:
            if (tbl, col) in HOLDERS or not table(db, tbl) or col not in columns(db, tbl):
                continue
            n = db.execute(f"SELECT count(*) FROM {tbl} WHERE {col} = ?", (sid,)).fetchone()[0]
            if n:
                named.append(f"{tbl}.{col} ({n})")
        if named:
            p.say(f"stand-in {sid} {have['itemname']!r} kept: still named by {', '.join(named)}")
        else:
            p.delete("stand-ins", db, "itempattern", (sid,))
            p.say(f"stand-in {sid} {have['itemname']!r} removed (the quests name {orig})")

    # 6. Trapmaw
    mine = rows(db, "SELECT id, modelid FROM npcs WHERE npc_name = ?", (TRAPMAW,))
    theirs = rows(src, "SELECT id, modelid FROM npcs WHERE npc_name = ?", (TRAPMAW,))
    if len(mine) != 1 or len(theirs) != 1:
        p.refuse(f"{TRAPMAW}: {len(mine)} rows on Live, {len(theirs)} on Heirloom (one each wanted)")
    else:
        lid, hid = mine[0]["id"], theirs[0]["id"]
        hb = source_before(src, "npcs", (hid,))
        have = full(db, "npcs", (lid,))
        if not same(have["modelid"], theirs[0]["modelid"]) and (hb is None or not loose(have["modelid"], hb["modelid"])):
            p.refuse(f"{TRAPMAW}: Live's modelid {have['modelid']} is not Heirloom's before")
        elif p.upsert("trapmaw", db, "npcs", (lid,), dict(plain(have), modelid=theirs[0]["modelid"])):
            p.say(f"{TRAPMAW} (row {lid}): modelid {have['modelid']} -> {theirs[0]['modelid']}")
        hr, lr = full(src, "npcrespawn", (hid,)), full(db, "npcrespawn", (lid,))
        rb = source_before(src, "npcrespawn", (hid,))
        if hr is None:
            p.refuse(f"{TRAPMAW}: Heirloom has no npcrespawn row")
        elif lr is not None and not same(lr["seconds"], hr["seconds"]) and (rb is None or not loose(lr["seconds"], rb["seconds"])):
            p.refuse(f"{TRAPMAW}: Live's respawn {lr['seconds']} is not Heirloom's before")
        else:
            v = dict(plain(hr), npc_row=lid) if lr is None else dict(plain(lr), seconds=hr["seconds"], note=hr["note"])
            if p.upsert("trapmaw", db, "npcrespawn", (lid,), v):
                p.say(f"{TRAPMAW}: respawn {None if lr is None else lr['seconds']} -> {hr['seconds']} s")
        hm = full(src, "npcmove", (TRAPMAW.lower(),))
        lm = db.execute("SELECT name FROM npcmove WHERE name = ? COLLATE NOCASE", (TRAPMAW,)).fetchall()
        if hm is None:
            p.refuse(f"{TRAPMAW}: Heirloom has no npcmove row")
        elif lm and lm[0][0] != hm["name"]:
            p.refuse(f"{TRAPMAW}: Live's npcmove row is {lm[0][0]!r}")
        elif lm and not all(same(full(db, "npcmove", (hm["name"],))[c], hm[c]) for c in plain(hm)):
            p.refuse(f"{TRAPMAW}: Live has an npcmove row of its own")
        elif not lm:
            p.upsert("trapmaw", db, "npcmove", (hm["name"],), plain(hm))
            p.say(f"{TRAPMAW}: wanders (npcmove roam {hm['roam']}, radius {hm['radius']})")

    # 7. characters part-way
    plan_chars(db, src, p)
    return p


def column_copy_done(db, src, q):
    """the copy's columns hold Heirloom's values."""
    qcols, steps = COLUMN_COPIES[q]
    pairs = [("quests", (q,), qcols)] + [("queststeps", (q, st), cols) for st, cols in steps.items()]
    for tbl, key, cols in pairs:
        h, have = full(src, tbl, key), full(db, tbl, key)
        if h is None or have is None or not all(same(have[c], h[c]) for c in cols):
            return False
    return True


def after_steps(db, src, p, q):
    """the quest's steps as they will be: {step: row}."""
    steps = {s["step"]: s for s in rows(db, "SELECT * FROM queststeps WHERE questid = ?", (q,))}
    for part, tbl, key, before, after in p.ops:
        if tbl == "queststeps" and key[0] == q:
            if after is None:
                steps.pop(key[1], None)
            else:
                steps[key[1]] = after
    return steps


def holds(db, sid, pid):
    return db.execute("SELECT count(*) FROM charinventory WHERE serverid = ? AND patternid = ?", (sid, pid)).fetchone()[0]


def plan_chars(db, src, p):
    quests = sorted(set(WHOLE_QUESTS) | set(COLUMN_COPIES))
    copied = set(getattr(p, "copied", []))
    marks = ",".join("?" * len(quests))
    for c in rows(db, f"SELECT rowid AS _rowid_, * FROM charquests WHERE questid IN ({marks}) AND step > 0 "
                      "ORDER BY questid, serverid", quests):
        q, sid, step = c["questid"], c["serverid"], c["step"]
        old = {s["step"]: s for s in rows(db, "SELECT * FROM queststeps WHERE questid = ?", (q,))}
        new = after_steps(db, src, p, q)
        to, count = step, c["count"]
        if q in STEP_MOVES and q in copied:
            if step not in STEP_MOVES[q]:
                p.refuse(f"character {sid} on {q} step {step}: not in the entry's step moves")
                continue
            to = STEP_MOVES[q][step]
            o, n = old.get(step), new.get(to)
            if not (o and n and o["kind"] == "loot" and n["kind"] == "loot"):
                count = 0
        if to not in new:
            p.refuse(f"character {sid} on {q} step {step}: no step {to} after the copy")
            continue
        if to != step or count != c["count"]:
            p.ops.append(("characters", "charquests", (sid, q), c, dict(plain(c), step=to, count=count)))
            p.say(f"  character {sid}: {q} step {step} (count {c['count']}) -> step {to} (count {count})")
        give = GIVES.get((q, to)) if q in copied else None
        if give is not None:
            if str(give) not in (new[to]["items"] or ""):
                p.refuse(f"{q} step {to}: does not want {give} after the copy")
                continue
            if holds(db, sid, give):
                p.say(f"  character {sid}: {q} step {to}, holds {give} already")
            elif db.execute("SELECT count(*) FROM charinventory WHERE serverid = ? AND location = 0", (sid,)).fetchone()[0] >= INV_CAP:
                p.say(f"  character {sid}: {q} step {to}: pack full, NOT given {give} (a GM gives it)")
            else:
                p.ops.append(("characters", "give", (sid, give), None, None))
                p.say(f"  character {sid}: {q} step {to}: given {give}")
        elif q in copied:
            o, n = old.get(step), new[to]
            if o is None or any(o[k] != n[k] for k in ("kind", "target", "items", "count", "tunar")):
                want = [x.strip().split("*")[0] for x in (n["items"] or "").split(",") if x.strip()]
                held = [w for w in want if holds(db, sid, int(w))]
                p.say(f"  character {sid}: {q} step {to} now {n['kind']} {n['target']} {n['items'] or ''}"
                      f" (was {'' if o is None else o['kind'] + ' ' + o['target']}); holds {held or 'none'} of {want or 'nothing'}")
            else:
                p.say(f"  character {sid}: {q} step {to}: the same step")


def audit(db, action, tbl, key, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (now(), acct[0] if acct else 0, WHO, VIA, action, tbl, dumps(dict(zip(KEYS[tbl], key))),
                      None if before is None else dumps(before), None if after is None else dumps(after), DETAIL))
    return cur.lastrowid


def write_row(db, tbl, values, rowid=None):
    cols = list(values)
    if rowid is not None:
        cols = ["rowid"] + cols
        vals = [rowid] + [values[c] for c in values]
    else:
        vals = [values[c] for c in values]
    return db.execute(f"INSERT INTO {tbl} ({', '.join(c if c == 'rowid' else chr(34) + c + chr(34) for c in cols)}) "
                      f"VALUES ({', '.join('?' * len(cols))})", vals).lastrowid


def log(db, part, tbl, key, before, after, a):
    db.execute(f"INSERT INTO {LOG} (part, tbl, rowkey, before, after, audit, at) VALUES (?, ?, ?, ?, ?, ?, ?)",
               (part, tbl, dumps(dict(zip(KEYS[tbl], key))), None if before is None else dumps(before),
                None if after is None else dumps(after), a, now()))


def sequence(db, tbl):
    if not table(db, "sqlite_sequence"):
        return None
    r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = ?", (tbl,)).fetchone()
    return None if r is None else r[0]


def apply(db, p):
    db.execute(LOG_DDL)
    db.execute(SEQ_DDL)
    grows = sorted({"charinventory" if o[1] == "give" else o[1] for o in p.ops if o[3] is None})
    seqs = {t: sequence(db, t) for t in grows}
    n = 0
    for part, tbl, key, before, after in p.ops:
        if tbl == "give":
            sid, pid = key
            cur = db.execute(
                "INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, "
                "listnumber) VALUES (?, 1, (SELECT COALESCE(maxhp, 0) FROM itempattern WHERE patternid = ?), 0, ?, -1, 0, "
                "(SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory WHERE serverid = ? AND location = 0))",
                (sid, pid, pid, sid))
            k = (cur.lastrowid,)
            row = full(db, "charinventory", k)
            log(db, part, "charinventory", k, None, row, audit(db, "insert", "charinventory", k, None, plain(row)))
            n += 1
            continue
        if before is None and key is None:  # a new row of its own id
            rid = write_row(db, tbl, after)
            key = keyof(tbl, full_by_rowid(db, tbl, rid))
        elif before is None:
            if full(db, tbl, key) is not None:
                raise RuntimeError(f"{tbl} {key}: there under us")
            write_row(db, tbl, after)
        else:
            now_ = full(db, tbl, key)
            if now_ != before:
                raise RuntimeError(f"{tbl} {key}: changed under us")
            if after is None:
                db.execute(f"DELETE FROM {tbl} WHERE {where(tbl)}", key)
            else:
                sets = [c for c in after if not same(before.get(c), after[c])]
                db.execute(f"UPDATE {tbl} SET {', '.join(chr(34) + c + chr(34) + ' = ?' for c in sets)} WHERE {where(tbl)}",
                           [after[c] for c in sets] + list(key))
                key = keyof(tbl, after)
        row = full(db, tbl, key) if after is not None else None
        action = "insert" if before is None else ("delete" if after is None else "update")
        log(db, part, tbl, key, before, row, audit(db, action, tbl, key, plain(before), plain(row)))
        n += 1
    for t, b in seqs.items():
        a = sequence(db, t)
        if a != b:
            db.execute(f"INSERT OR REPLACE INTO {SEQ} (tbl, before, after) VALUES (?, ?, ?)", (t, b, a))
    return n


def full_by_rowid(db, tbl, rid):
    return rows(db, f"SELECT rowid AS _rowid_, * FROM {tbl} WHERE rowid = ?", (rid,))[0]


def undo(db):
    """(rows put back, [left])."""
    if not table(db, LOG):
        return 0, []
    back, left = 0, []
    for seq, tbl, rowkey, before, after in db.execute(
            f"SELECT seq, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        k = json.loads(rowkey)
        key = tuple(k[c] for c in KEYS[tbl])
        have = full(db, tbl, key)
        if (have is None) != (after is None) or (have is not None and dumps(have) != after):
            left.append(f"{tbl} {rowkey}: changed since, left as it is")
            continue
        b = None if before is None else json.loads(before)
        if have is not None:
            db.execute(f"DELETE FROM {tbl} WHERE {where(tbl)}", key)
        if b is not None:
            rid = b.pop("_rowid_")
            write_row(db, tbl, b, rid)
        action = "delete" if b is None else ("insert" if have is None else "update")
        audit(db, action, tbl, key, plain(have), b)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if not left:
        db.execute(f"DROP TABLE {LOG}")
        if table(db, SEQ):
            for t, b, a in db.execute(f"SELECT tbl, before, after FROM {SEQ}").fetchall():
                if sequence(db, t) == a:
                    if b is None:
                        db.execute("DELETE FROM sqlite_sequence WHERE name = ?", (t,))
                    else:
                        db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = ?", (b, t))
            db.execute(f"DROP TABLE {SEQ}")
    return back, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True, help="Live's database (written)")
    ap.add_argument("--source", help="Heirloom's database (read only)")
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
            print(f"undone: {back} rows put back" + "".join(f"\n  left: {w}" for w in left))
            return 0 if not left else 1
        if not a.source:
            ap.error("--source names Heirloom's database")
        src = sqlite3.connect(f"file:{a.source}?mode=ro", uri=True)
        try:
            p = plan(db, src)
        finally:
            src.close()
        for line in p.lines:
            print(line)
        if p.refusals:
            db.execute("ROLLBACK")
            print("REFUSED, nothing written:\n  " + "\n  ".join(p.refusals))
            return 1
        if not p.ops:
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"(report only: {len(p.ops)} rows to write; --apply to write)")
            return 0
        n = apply(db, p)
        db.execute("COMMIT")
        print(f"applied: {n} rows written, each in gm_audit and {LOG}; the quests load at the server's next start")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
