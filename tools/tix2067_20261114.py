#!/usr/bin/env python3
"""Five petitions' data fixes of 2026-11-14 (batch tix5, lane t2067), each a column of a row, both shards.

  2057 (Felthious, Heirloom): "lvl 60 primal spirit should be the big wolf animation"; the owner: "big wolf is the dire
       wolf". petstats 700807 (Primal Spirit) model -> 2925832803 (0xAE64AA63), the spiked big wolf: 1614 vertices, half
       again as broad as the plain wolf, the body the NPCs named "Primal Spirit" wear in the spirit world (world 3). The
       body given on 2026-11-14 (2925832801, 0xAE64AA61, "black wolf"/"dire wolf") is the timber wolf's mesh
       (821616987, 0x30F8E15B, 1092 vertices, same bounds) in a dark skin: "wolf is same just.. lower credit score".
       From either of those two bodies (the Live shard still has 821616987).
  2067 (Lonewulf): "call to line wont stack other buffs". The warrior's Call to line (Call to Action 100184, Arms 358,
       Combat 671, Battle 1552, War 1563, Victory 2776: group +STA +AGI +max HP, workbook stack type 3) shared a stat
       with every other class's group stack-3 buff (Group Tribal Thew +STA, Group Acumen +AGI, Pure Heart +max HP: one
       line, entity/buffline.go sameLine), so the higher rank took the other off. Their stack type -> 2776, one of
       their own, shared by the six so that the ranks stay one line among themselves (Living Stone's way, 2026-11-11).
  2068 (Lonewulf): "leadership wisdom wont stack sha buffs". Leadership (1791, the bard's and magician's group +STA
       +WIS, level 6, stack 3) was one line with Group Tribal Thew (+STA, level 51) and was refused under it. Its stack
       type -> 1791, its own.
  2071 (Lonewulf, Heirloom): "queen saersalias circlet meant to be a head piece?" itempattern 995043 (Queen
       Saersalia's Circlet, raid gear of the Tomb of Kings, tools/dungeons_more_20261112.py) was made from Sacred Peach
       (715015), a held item: slot 18 (held), its hand model 786059401 and the peach's icon. -> slot 1 (head), model 0,
       icon 864498561 (the level-60 circlets' own: Circlet of Loathing, Circlet of Spun Ether). Stats unchanged. A
       circlet worn in a hand (charinventory equiploc 19/20; Heirloom 2026-11-14: two) is taken off (-1) so that it is
       put on the head: run it with the wearers out of the world or in a keep restart's before.sh (the server holds a
       player's inventory while they play).
  2072 (Fullmetal, Heirloom): "Robe of Frozen Throne: Alc Tunic that is mislabled as robe". itempattern 995075 is the
       alchemist's chest piece (slot 5, a tunic: made from Tunic of the Chemist 6475, leather, its icon) named a robe
       (robes are slot 2). itemname -> "Tunic of the Frozen Throne". Its stats are the owner's call: not changed.
  2073 (Fullmetal, Heirloom): "Harbringers of Khalimet ... couldnt continue passed 1st part as its labled on map at
       location that players cannot get to". Step 2 (reach Gaulin's abandoned camp in Box Canyon) was at (20937, 20722),
       radius 40: the top of a mesa (146 high) between two arms of the canyon, whose floor (6) comes no nearer than
       ~125 (a walk over world 0's ground and walls files from the canyon floor never reaches it). -> (20957, 5.3,
       20935), the canyon's floor 215 south of it, reached from the canyon (step 3's Armis is spawned beside the
       player; his standing row on the mesa is left). quests are read at the server's start: a restart (a keep
       restart's before.sh).
  5021 (Innoruuk, Live): "Wrong form for level 16 mage pet". The magician's level 16 pet, Earth Elementalkin (657,
       workbook pet 001C), wore the Earth Elementaling's body (931615263, 0x3787521F: floating rocks). -> 931615264
       (0x37875220), the elementalkin's (the NPCs "Earth Elementalkin" wear it). Both shards had it.

The Live shard has no dungeon raid gear: 2071 and 2072 are skipped there (said).

Refused, nothing written, when a row is not as expected (a GM's edit since). Report only by default; --apply in one
transaction, a gm_audit line a row, log tix2067_1114_log; --undo exact (a row changed since is left and said).
--only 2057,5021 limits it to those petitions. petstats and spelleffect are read at the server's start (petstats) /
every cast (spelleffect); itempattern at the start: run it in a keep restart's before.sh.

    python3 -E -s tools/tix2067_20261114.py --db eqoa.db [--apply | --undo] [--only 2057,2067,...]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "tix2067_1114_log"
WHO, VIA = "dab", "tool"

CALL_TO = {100184: "Call to Action", 358: "Call to Arms", 671: "Call to Combat", 1552: "Call to Battle",
           1563: "Call to War", 2776: "Call to Victory"}
CALL_TO_STACK = 2776
LEADERSHIP = 1791
CAMP = (20957.0, 5.3, 20935.0)  # Box Canyon's floor, 2073

DETAILS = {
    2057: "petition 2057 (Felthious): Primal Spirit the big spiked wolf (0xAE64AA63), the Primal Spirits' own body",
    2067: "petition 2067 (Lonewulf): the Call to line a stack type of its own, so it stacks with other classes' buffs",
    2068: "petition 2068 (Lonewulf): Leadership a stack type of its own, so it stacks with the shaman's buffs",
    2071: "petition 2071 (Lonewulf): Queen Saersalia's Circlet a head piece (it was made from a held item)",
    2072: "petition 2072 (Fullmetal): Robe of the Frozen Throne is a tunic: named so",
    2073: "petition 2073 (Fullmetal): Gaulin's abandoned camp on Box Canyon's floor, not on the mesa top",
    5021: "petition 5021 (Innoruuk): the level 16 Earth Elementalkin wears the elementalkin's body, not the elementaling's",
}

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    ticket  INTEGER NOT NULL,
    tbl     TEXT NOT NULL,
    keycol  TEXT NOT NULL,
    keyval  TEXT NOT NULL,
    col     TEXT NOT NULL,
    before  TEXT,
    after   TEXT,
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def compact(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False)


def audit(db, ticket, tbl, keycol, keyval, col, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', 'update', ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, tbl,
                      compact(dict(zip(str(keycol).split(","), (int(v) for v in str(keyval).split(","))))),
                      compact({col: before}), compact({col: after}), DETAILS[ticket] + ": tools/tix2067_20261114.py"))
    return cur.lastrowid


class Plan:
    def __init__(self):
        self.changes, self.lines, self.refusals = [], [], []

    def column(self, ticket, tbl, keycol, key, col, want, after, label, check=None):
        """want: the values the column may have before (a tuple); after: the new value."""
        row = db_row(self.db, tbl, keycol, key)
        if row is None:
            self.lines.append(f"{ticket}: {tbl} {key} ({label}) not on this shard: skipped")
            return
        if check:
            bad = check(row)
            if bad:
                self.refusals.append(f"{ticket}: {tbl} {key}: {bad}")
                return
        v = row[col]
        if v == after:
            self.lines.append(f"{ticket}: {tbl} {key} ({label}) {col} = {after!r} already")
            return
        if v not in want:
            self.refusals.append(f"{ticket}: {tbl} {key} ({label}) {col} = {v!r}, want one of {want!r}")
            return
        self.changes.append((ticket, tbl, keycol, key, col, v, after))
        self.lines.append(f"{ticket}: {tbl} {key} ({label}) {col} {v!r} -> {after!r}")

    def column_where(self, ticket, tbl, keycols, keys, col, want, after, label, check=None):
        """column on a row of a two-column key: logged with keycol "a,b" and keyval "1,2"."""
        self.db.row_factory = sqlite3.Row
        try:
            row = self.db.execute(f"SELECT * FROM {tbl} WHERE {keycols[0]} = ? AND {keycols[1]} = ?", keys).fetchone()
        finally:
            self.db.row_factory = None
        if row is None:
            self.lines.append(f"{ticket}: {tbl} {keys} ({label}) not on this shard: skipped")
            return
        if check and check(row):
            self.refusals.append(f"{ticket}: {tbl} {keys}: {check(row)}")
            return
        v = row[col]
        if v == after:
            self.lines.append(f"{ticket}: {tbl} {keys} ({label}) {col} = {after!r} already")
            return
        if v not in want:
            self.refusals.append(f"{ticket}: {tbl} {keys} ({label}) {col} = {v!r}, want one of {want!r}")
            return
        self.changes.append((ticket, tbl, ",".join(keycols), ",".join(str(k) for k in keys), col, v, after))
        self.lines.append(f"{ticket}: {tbl} {keys} ({label}) {col} {v!r} -> {after!r}")

    def stack(self, ticket, spellid, name, want, new):
        r = self.db.execute("SELECT p.spellname, e.effects FROM spellpattern p JOIN spelleffect e USING(spellid) "
                            "WHERE p.spellid = ?", (spellid,)).fetchone()
        if r is None or r[0] != name:
            self.refusals.append(f"{ticket}: spell {spellid}: {r[0] if r else 'missing'!r}, want {name}")
            return
        text = r[1]
        try:
            eff = json.loads(text)
        except ValueError:
            self.refusals.append(f"{ticket}: spelleffect {spellid} ({name}): effects are not JSON")
            return
        if compact(eff) != text:
            self.refusals.append(f"{ticket}: spelleffect {spellid} ({name}): effects not compact (would not undo exactly)")
            return
        s = eff.get("stack")
        if s == new:
            self.lines.append(f"{ticket}: spelleffect {spellid} ({name}) stack {new} already")
            return
        if s != want:
            self.refusals.append(f"{ticket}: spelleffect {spellid} ({name}): stack {s!r}, want {want}")
            return
        eff["stack"] = new
        self.changes.append((ticket, "spelleffect", "spellid", spellid, "effects", text, compact(eff)))
        self.lines.append(f"{ticket}: spelleffect {spellid} ({name}) stack {want} -> {new}")


def db_row(db, tbl, keycol, key):
    db.row_factory = sqlite3.Row
    try:
        return db.execute(f"SELECT * FROM {tbl} WHERE {keycol} = ?", (key,)).fetchone()
    finally:
        db.row_factory = None


def plan(db, only):
    p = Plan()
    p.db = db
    if 2057 in only:
        p.column(2057, "petstats", "spellid", 700807, "model", (821616987, 2925832801), 2925832803, "Primal Spirit",
                 check=lambda r: None if r["name"] == "Primal Spirit" else f"name {r['name']!r}")
    if 2067 in only:
        for sid, name in CALL_TO.items():
            p.stack(2067, sid, name, 3, CALL_TO_STACK)
    if 2068 in only:
        p.stack(2068, LEADERSHIP, "Leadership", 3, LEADERSHIP)
    if 2071 in only:
        chk = lambda r: None if r["itemname"] == "Queen Saersalia's Circlet" else f"name {r['itemname']!r}"
        p.column(2071, "itempattern", "patternid", 995043, "equipslot", (18,), 1, "Queen Saersalia's Circlet", chk)
        p.column(2071, "itempattern", "patternid", 995043, "model", (786059401,), 0, "Queen Saersalia's Circlet", chk)
        p.column(2071, "itempattern", "patternid", 995043, "itemicon", (-611086336,), 864498561,
                 "Queen Saersalia's Circlet", chk)
        # worn in a hand (held: 19, 20) it would sit there as a head piece: taken off, into the pack, to be put on
        for (itemid,) in db.execute("SELECT itemid FROM charinventory WHERE patternid = 995043 AND equiploc IN (19, 20) "
                                    "ORDER BY itemid").fetchall():
            p.column(2071, "charinventory", "itemid", itemid, "equiploc", (19, 20), -1,
                     "a Queen Saersalia's Circlet worn in a hand")
    if 2072 in only:
        p.column(2072, "itempattern", "patternid", 995075, "itemname", ("Robe of the Frozen Throne",),
                 "Tunic of the Frozen Throne", "the alchemist's Vox tunic",
                 check=lambda r: None if r["equipslot"] == 5 else f"slot {r['equipslot']}, want 5 (a tunic)")
    if 2073 in only:
        chk = lambda r: None if r["kind"] == "reach" else f"kind {r['kind']!r}, want reach"
        for col, old, new in (("x", 20937.0, CAMP[0]), ("y", 54.0, CAMP[1]), ("z", 20722.0, CAMP[2])):
            p.column_where(2073, "queststeps", ("questid", "step"), (101278, 2), col, (old,), new,
                           "Harbingers of Khalimet step 2, the camp", chk)
    if 5021 in only:
        p.column(5021, "petstats", "spellid", 657, "model", (931615263,), 931615264, "Earth Elementalkin",
                 check=lambda r: None if r["name"] == "Earth Elementalkin" else f"name {r['name']!r}")
    return p


def write(db, tbl, keycol, keyval, col, before, after):
    cols, vals = str(keycol).split(","), [int(v) for v in str(keyval).split(",")]
    where = " AND ".join(f"{c} = ?" for c in cols)
    cur = db.execute(f"UPDATE {tbl} SET {col} = ? WHERE {where} AND {col} IS ?", (after, *vals, before))
    return cur.rowcount == 1


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for _, ticket, tbl, keycol, keyval, col, before, after, _a in db.execute(f"SELECT * FROM {LOG} ORDER BY seq DESC").fetchall():
        before, after = json.loads(before), json.loads(after)
        if not write(db, tbl, keycol, keyval, col, after, before):
            print(f"left: {ticket} {tbl} {keyval} {col} changed since")
            left += 1
            continue
        audit(db, ticket, tbl, keycol, keyval, col, after, before)
        back += 1
    if not left:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    ap.add_argument("--only", default=",".join(str(t) for t in DETAILS))
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    only = {int(t) for t in a.only.split(",") if t.strip()}
    if not only <= set(DETAILS):
        sys.exit(f"--only: unknown petitions {sorted(only - set(DETAILS))}")
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
        p = plan(db, only)
        for line in p.lines:
            print(line)
        if p.refusals:
            print("refused, nothing done:")
            for r in p.refusals:
                print("  " + r)
            return 1
        if not p.changes:
            print("nothing to do")
            return 0
        if not a.apply:
            print(f"report only: {len(p.changes)} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            db.execute(LOG_SCHEMA)
            for ticket, tbl, keycol, keyval, col, before, after in p.changes:
                if not write(db, tbl, keycol, keyval, col, before, after):
                    raise RuntimeError(f"{tbl} {keyval} changed under the update")
                a_id = audit(db, ticket, tbl, keycol, keyval, col, before, after)
                db.execute(f"INSERT INTO {LOG} (ticket, tbl, keycol, keyval, col, before, after, audit) "
                           "VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
                           (ticket, tbl, keycol, str(keyval), col, json.dumps(before), json.dumps(after), a_id))
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {len(p.changes)} rows, log {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
