#!/usr/bin/env python3
"""The Eye of the Sphynx a gem that takes its holder to the Plane of Sky (Live petition 5514, 2026-11-23: "PoS Portal
quests bugs out when giving you the eye of the spinex, it should be a useable gem to port you to Plane of Sky").

The Eye (record 701463) is "The Endless Tower"'s reward (quest 101300's last step, at Sartheret; the guide's reward for
every class but the porting ones, "carried each time"), NO TRADE. It did nothing: a Use of it was any gem's, and it
stacked to 50. Now (both shards alike):

  - its Use casts a new spell, 760400 "Eye of the Sphynx" (a copy of Nerius Gate's: no cast, no power, no recast, its
    visual), a self gate (spellgates) to the Sky Portal's landing on the Plane of Sky (world 4 at 4834.8, 250.06,
    5524.7: where the quest's own step 2 sends one, by Sartheret);
  - it is kept, not spent: an item record that stacks gives one off its stack a Use (handlers/itemspell.go), so it
    holds one (maxstack 50 -> 1; every copy held is a single one, checked), and its itemeffect has no charges;
  - its words say so; its info window shows the gate (itemeffect).

Only a character who finished The Endless Tower holds one (the quest's reward or a GM's grant; NO TRADE), so no other
gate is checked. Every row is checked against what was read here; anything else REFUSES, nothing written. Items, spells
and gates are read at a start: apply in the before.sh of the next keep restart, on each shard.

Dry run by default. --apply writes in one transaction, a gm_audit line a row (account 1, dab, via tool), each row's
before and after in sky_eye_1123_log; a second --apply writes nothing. --undo puts back what the log holds.

    python3 -E -s tools/sky_eye_20261123.py --db eqoa.db --shard heirloom|live [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "sky_eye_1123_log"
WHO, ACCOUNT, VIA = "dab", 1, "tool"
TOOL = "tools/sky_eye_20261123.py"
SRC = "Live petition 5514 (sky_eye_20261123)"
DETAIL = f"Live petition 5514: the Eye of the Sphynx takes its holder to the Plane of Sky: {TOOL}"

EYE, EYE_NAME = 701463, "Eye of the Sphynx"
EYE_DESC_WAS = "A blue gem, the manticores' gift. Carried, it lets one through the portal atop the Endless Tower."
EYE_DESC = ("A blue gem, the manticores' gift. Use it to be carried to the Plane of Sky, by Sartheret; "
            "it is kept, as often as you need it.")
SPELL, GATE_FROM = 760400, "Nerius Gate"
PLACE, AT = "the Plane of Sky", (4, 4834.8, 250.06, 5524.7)
LINE = "Self, Teleports the caster to the Plane of Sky, by Sartheret"

LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    rid    INTEGER NOT NULL,
    rowkey TEXT NOT NULL,
    before TEXT,
    after  TEXT,
    audit  INTEGER NOT NULL,
    at     TEXT NOT NULL
)"""


class Refused(Exception):
    pass


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def dumps(v):
    return json.dumps(v, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def rows(db, tbl, where):
    w = " AND ".join(f'"{c}" = ?' for c in where) or "1"
    cur = db.execute(f'SELECT rowid AS _rowid_, * FROM "{tbl}" WHERE {w} ORDER BY rowid', tuple(where.values()))
    names = [d[0] for d in cur.description]
    return [dict(zip(names, r)) for r in cur.fetchall()]


def plain(r):
    return None if r is None else {k: v for k, v in r.items() if k != "_rowid_"}


def same(a, b):
    if isinstance(a, (int, float)) and isinstance(b, (int, float)) and not isinstance(a, bool):
        return abs(float(a) - float(b)) < 1e-6
    return a == b


def num(v):
    return int(v) if float(v).is_integer() else v


class Plan:
    def __init__(self, db):
        self.db = db
        self.ops = []  # (tbl, rid or None, before or None, after values)
        self.lines = []

    def say(self, s):
        self.lines.append(f"  {s}")

    def add(self, tbl, values, match, what):
        have = rows(self.db, tbl, {c: values[c] for c in match})
        if have:
            if len(have) == 1 and all(same(have[0].get(c), v) for c, v in values.items()):
                self.say(f"{what}: already there")
                return
            raise Refused(f"{what}: {tbl} has {dumps([plain(h) for h in have])[:300]}, not {dumps(values)[:300]}")
        self.ops.append((tbl, None, None, dict(values)))
        self.say(f"{what}: insert {tbl}")

    def change(self, tbl, row, was, now_, what):
        """Set now_'s columns on row, which must read was (or now_ already)."""
        if all(same(row.get(c), v) for c, v in now_.items()):
            self.say(f"{what}: already so")
            return
        for c, v in was.items():
            if not same(row.get(c), v):
                raise Refused(f"{what}: {c} reads {row.get(c)!r}, not {v!r}")
        self.ops.append((tbl, row["_rowid_"], plain(row), dict(now_)))
        self.say(f"{what}: {dumps({c: row.get(c) for c in now_})[:160]} -> {dumps(now_)[:160]}")



def build(p, shard):
    db = p.db
    for t in ("itempattern", "itemeffect", "itemspell", "spellpattern", "spelleffect", "spellgates", "charinventory",
              "gm_audit"):
        if not table(db, t):
            raise Refused(f"no table {t}")
    eye = rows(db, "itempattern", {"patternid": EYE})
    if len(eye) != 1 or eye[0]["itemname"] != EYE_NAME:
        raise Refused(f"record {EYE} is not the {EYE_NAME}")
    more = db.execute("SELECT count(*) FROM charinventory WHERE patternid = ? AND stackleft > 1", (EYE,)).fetchone()[0]
    if more:
        raise Refused(f"{more} copies of the Eye hold more than one")
    p.change("itempattern", eye[0], {"maxstack": 50, "trade": 1, "itemdesc": EYE_DESC_WAS},
             {"maxstack": 1, "itemdesc": EYE_DESC}, f"the {EYE_NAME}: one a copy, its words")
    g = rows(db, "spellpattern", {"spellname": GATE_FROM})
    if len(g) != 1:
        raise Refused(f"{len(g)} spells named {GATE_FROM!r}")
    fx = rows(db, "spelleffect", {"spellid": g[0]["spellid"]})
    if len(fx) != 1 or not fx[0]["effects"]:
        raise Refused(f"{GATE_FROM} has no effects to copy")
    named = [r["spellid"] for r in rows(db, "spellpattern", {"spellname": EYE_NAME}) if r["spellid"] != SPELL]
    if named:
        raise Refused(f"spells {named} are named {EYE_NAME!r} already")
    sp = plain(g[0])
    sp.update(spellid=SPELL, spellname=EYE_NAME, abilitylvl=1, casttime=0, power=0, recast=0, spelldesc=LINE)
    p.add("spellpattern", sp, ("spellid",), f"spell {SPELL} {EYE_NAME!r} (a copy of {GATE_FROM}'s)")
    eff = json.loads(fx[0]["effects"])
    eff["level"] = 1
    for e in eff.get("effects", []):
        if e.get("kind") == "teleport":
            e["location"] = f"Plane of Sky,{AT[1]},{AT[2]},{AT[3]}"
    p.add("spelleffect", {"spellid": SPELL, "line": LINE, "fxdict": fx[0]["fxdict"],
                          "source": f"{SRC} (visual {GATE_FROM}'s)", "effects": json.dumps(eff, separators=(",", ":"))},
          ("spellid",), f"spell {SPELL}'s effect: a teleport")
    p.add("spellgates", {"spell": EYE_NAME.lower(), "kind": "self", "place": PLACE, "world": AT[0], "x": AT[1],
                         "y": AT[2], "z": AT[3], "radius": 0.0,
                         "note": "The Endless Tower's Eye: the Sky Portal's landing, where quest 101300 step 2 sends one",
                         "source": SRC}, ("spell",), f"gate {EYE_NAME.lower()!r} to {PLACE}")
    p.add("itemspell", {"patternid": EYE, "spellid": SPELL, "rest_s": 0.0,
                        "note": "the Eye of the Sphynx carries its holder to the Plane of Sky, kept", "source": SRC},
          ("patternid",), f"the {EYE_NAME}'s Use casts {SPELL}")
    p.add("itemeffect", {"patternid": EYE, "line": LINE, "name": EYE_NAME, "source": SRC, "charges": 0},
          ("patternid",), f"the {EYE_NAME}'s effect line (no charges: never spent)")


def keyof(db, tbl, row, rid):
    pk = sorted((r[5], r[1]) for r in db.execute(f'PRAGMA table_info("{tbl}")') if r[5])
    return {c: row[c] for _, c in pk} if pk else {"rowid": rid}


def audit(db, action, tbl, key, before, after):
    return db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                      "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                      (now(), ACCOUNT, WHO, VIA, action, tbl, dumps(key), None if before is None else dumps(before),
                       None if after is None else dumps(after), DETAIL)).lastrowid


def by_rowid(db, tbl, rid):
    r = rows(db, tbl, {"rowid": rid})
    return plain(r[0]) if r else None


def apply(db, p):
    db.execute(LOG_DDL)
    at = now()
    for tbl, rid, before, new in p.ops:
        if rid is None:
            cs = list(new)
            rid = db.execute(f'INSERT INTO "{tbl}" ({", ".join(chr(34) + c + chr(34) for c in cs)}) '
                             f'VALUES ({", ".join("?" * len(cs))})', [new[c] for c in cs]).lastrowid
            action = "insert"
        else:
            db.execute(f'UPDATE "{tbl}" SET {", ".join(chr(34) + c + chr(34) + " = ?" for c in new)} WHERE rowid = ?',
                       [*new.values(), rid])
            action = "update"
        after = by_rowid(db, tbl, rid)
        key = keyof(db, tbl, after, rid)
        a = audit(db, action, tbl, key, before, after)
        db.execute(f"INSERT INTO {LOG} (tbl, rid, rowkey, before, after, audit, at) VALUES (?, ?, ?, ?, ?, ?, ?)",
                   (tbl, rid, dumps(key), None if before is None else dumps(before), dumps(after), a, at))
    return len(p.ops)


def undo(db):
    if not table(db, LOG):
        return 0, []
    back, left = 0, []
    for seq, tbl, rid, rowkey, before, after in db.execute(
            f"SELECT seq, tbl, rid, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        have = by_rowid(db, tbl, rid)
        if have is None or dumps(have) != after:
            left.append(f"{tbl} {rowkey}: changed since, left as it is")
            continue
        b = None if before is None else json.loads(before)
        if b is None:
            db.execute(f'DELETE FROM "{tbl}" WHERE rowid = ?', (rid,))
            action = "delete"
        else:
            db.execute(f'UPDATE "{tbl}" SET {", ".join(chr(34) + c + chr(34) + " = ?" for c in b)} WHERE rowid = ?',
                       [*b.values(), rid])
            action = "update"
        audit(db, action, tbl, json.loads(rowkey), have, b)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if not db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0]:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--shard", required=True, choices=("heirloom", "live"))
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("PRAGMA busy_timeout = 5000")
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"undone: {back} rows put back" + "".join(f"\n  left: {w}" for w in left))
            return 0
        p = Plan(db)
        try:
            build(p, a.shard)
        except Refused as e:
            db.execute("ROLLBACK")
            print("\n".join(p.lines))
            print(f"REFUSED, nothing written: {e}")
            return 1
        print(f"{a.shard}:")
        print("\n".join(p.lines))
        print(f"totals: {len(p.ops)} rows to write")
        if not a.apply or not p.ops:
            db.execute("ROLLBACK")
            print("dry run: nothing written (--apply to write)" if not a.apply else "nothing to write: all in place")
            return 0
        n = apply(db, p)
        db.execute("COMMIT")
        print(f"applied: {n} rows, a gm_audit line each, logged in {LOG}")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
