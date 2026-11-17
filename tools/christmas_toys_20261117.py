#!/usr/bin/env python3
"""Four more of Toymaker Tinsel's Christmas toys (the owner, 2026-11-17: "lets add string of lights candy cane staff",
"add pegasus plushie too", "add gingerbread illusion too, its clay golem with white eyes, smile, liek the cookie").
Heirloom only (the Christmas event is Heirloom's). Needs a server of branch toys-lights-cane (aura kinds 11-13, the
plushie's Use); vanilla of the same branch draws the lights, the stripes and the icing.

1. String of Lights (CAPE_ID): a cape (wear slot 28, the cape slot), a copy of the Heirloom Azure Knight's Cape 717001's
   shape: style STYLE (the knight's long cape; Dread's high collar and Magnus Jaeger's arm corners zigzag the string),
   its colour CAPE_COLOR (a wine red); every class and race, level 1, tradeable, not lore,
   no stats. itemaura kind 11 (lights, database.AuraLights; colour, source and nodes 0): vanilla hangs a string of
   twinkling multicoloured bulbs on a green cord along the cape as fitted to the wearer (up one side, over the
   shoulders, down the other), skinned to its bones. A PS2 never sees a cape (the slot is the server's); an older
   vanilla draws the cape alone.
2. Candy Cane Staff (STAFF_ID): the Apprentice Staff 5489 copied (level 1, the same damage and delay: no better than
   a level-1 staff), every class and race, tradeable, not lore, red (0xC81E1EFF: an older vanilla dyes the plain staff
   red). itemaura first kind 12 (stripes, database.AuraStripes: vanilla paints the staff red and white in a spiral, 9
   stripes), second kind 3 (particles) of 0x1D875ECD along 100 (the monk staff's twinkle at its tip, as Dust's sword
   and Drizzt's Twinkle; a first aura's nodes stop at 32, so the along is the second's). The Weapon Enchanter's aura on a copy takes the hand whole (instanceaura): stripes and
   twinkle give way to it.
3. Pegasus Plushie (PLUSHIE_ID): worn nowhere, CLIENT's usable type (procanim 2: the item menu offers Use), never
   spent, tradeable, not lore, bound to no one, the Illusion: Pegasus scroll's icon. A Use brings out a tiny white
   winged horse (CHAR.ESF 0x2F8721C0 at 0.25, entity/plushie.go) under a cute name, following its owner, never a
   target; the next Use sends it home. Found by NAME (handlers/plushie.go).
4. Gingerbread Man (GINGER_ID): a toy illusion as Santa's Elf 716912 is (itemeffect "Self, Unlimited Uses, 30min",
   cooldown 60 s; used again it ends), the body the Clay Golem 0xEC6A0604, one itemoutfit row (a golem has no armour
   to dress; the row makes it a toy illusion, entity/outfit.go), itemaura kind 13 (icing, database.AuraIcing): while
   the illusion is shown the body's aura carries it (entity.PieceAurasOf) and vanilla bakes the golem gingerbread
   brown and ices it: white eyes, a smile, three buttons, wavy cuffs. A PS2 and an older vanilla draw the golem as
   the disc has it (a clay brown).
5. tokenshop: Toymaker Tinsel sells them, in Holly Tokens: String of Lights 81, Candy Cane Staff 108, Pegasus Plushie
   54, Gingerbread Man 54.

Ids fixed (716915-716918: the next free on the LIVE Heirloom database's .backup of 2026-11-17 08:14 UTC, after
build328's Toymaker's Puppet Strings took 716914) so a re-run agrees. Any target id taken refuses everything: a record
there that this tool did not write (whatever its name), or an itemaura, itemeffect, itemoutfit or tokenshop row for it
with no record of this tool's.
Report by default; --apply in one transaction, each row audited (gm_audit, as the panel's) and logged in
christmas_toys_1117_log; a second --apply finds nothing to do; --undo puts back exactly what it wrote, all or nothing:
refused while any character, bank, merc or auction holds one of the toys, or a row written has changed since.

The server reads itemeffect and itemoutfit at its start: run --apply in a keep restart's before.sh (or press the
panel's "Reload every NPC" for the auras and restart for the illusion). The token shop reads its stock at each visit.

    python3 -E -s tools/christmas_toys_20261117.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "christmas_toys_1117_log"
SRC = "christmas-toys"
DETAIL = "Toymaker Tinsel's String of Lights, Candy Cane Staff, Pegasus Plushie and Gingerbread Man (the owner, 2026-11-17); tools/christmas_toys_20261117.py"
WHO, VIA = "dab", "tool"
SHOP = "Toymaker Tinsel"
DESC_MAX = 200

CAPE_ID, STAFF_ID, PLUSHIE_ID, GINGER_ID = 716915, 716916, 716917, 716918
CAPE_BASE, STAFF_BASE, TOY_BASE = 717001, 5489, 716910  # Heirloom Azure Knight's Cape, Apprentice Staff, Barrel of Monkeys
STYLE = 1  # cape style: the silver knight's long cape (vanilla cape::STYLES), whose plain outline carries the lights best
CAPE_COLOR = 0xE04040FF  # its blue silk made a deep wine red (0xRRGGBBAA; vanilla multiplies the cape's layers by it)
CAPE_ICON = 1414103605  # the Heirloom Dreadlord's Cape's (a red cloak)
STAFF_COLOR = 0xC81E1EFF
PLUSHIE_ICON = 824019198  # Illusion: Pegasus
GINGER_ICON = 1511586099  # hearty bread
GINGER_BUFF_ICON = 1507275072  # Santa's Elf's illusion's icon
CLAY_GOLEM = 0xEC6A0604
TWINKLE = 0x1D875ECD

AURA_PARTICLES, AURA_LIGHTS, AURA_STRIPES, AURA_ICING = 3, 11, 12, 13

TOYS = [
    {"id": CAPE_ID, "name": "String of Lights", "base": CAPE_BASE, "price": 81, "sort": 6,
     "desc": "A string of tiny twinkling lights sewn along a warm red cape, red, green, blue and gold. Wear it in your "
             "cape slot and twinkle all the way home.",
     "says": "Tiny twinkling lights all along a warm red cape, to wear in your cape slot. The whole of Freeport will see "
             "you coming!",
     "set": {"equipslot": 28, "model": STYLE, "color": CAPE_COLOR, "itemicon": CAPE_ICON},
     "aura": {"color": 0, "kind": AURA_LIGHTS, "source": 0, "nodes": 0,
              "note": "The String of Lights: twinkling bulbs along the worn cape (vanilla toys::cape_lights), the owner 2026-11-17"}},
    {"id": STAFF_ID, "name": "Candy Cane Staff", "base": STAFF_BASE, "price": 108, "sort": 7,
     "desc": "A staff of peppermint candy, striped red and white, with a twinkle at its tip. Sweet to look at; no sharper "
             "than an apprentice's staff.",
     "says": "Peppermint through and through, striped red and white, and it twinkles at the tip. Not much of a weapon, "
             "mind, but what a sight!",
     "set": {"color": STAFF_COLOR},
     "aura": {"color": 0, "kind": AURA_STRIPES, "source": 0, "nodes": 0,
              "color2": 0, "kind2": AURA_PARTICLES, "source2": TWINKLE, "nodes2": 100,
              "note": "The Candy Cane Staff: the monk staff's twinkle at its tip, striped red and white (vanilla toys::stripe), the owner 2026-11-17"}},
    {"id": PLUSHIE_ID, "name": "Pegasus Plushie", "base": TOY_BASE, "price": 54, "sort": 8,
     "desc": "A soft white winged horse with button eyes. Use it and a tiny pegasus trots after you; use it again and it "
             "flutters home.",
     "says": "A tiny pegasus of your own! Use the plushie and it trots after you wherever you go. Use it again when it is "
             "time for a nap.",
     "set": {"itemicon": PLUSHIE_ICON, "lore": 0}},
    {"id": GINGER_ID, "name": "Gingerbread Man", "base": TOY_BASE, "price": 54, "sort": 9,
     "desc": "A gingerbread man with icing eyes and a big smile. Use it to become one for 30 minutes; use it again to be "
             "yourself. Run, run, as fast as you can!",
     "says": "Run, run, as fast as you can! Become a gingerbread man, iced eyes and all, for half an hour. Use it again "
             "to be yourself.",
     "set": {"itemicon": GINGER_ICON, "lore": 0},
     "illusion": {"line": "Self, Unlimited Uses, 30min", "name": "Gingerbread Man", "icon": GINGER_BUFF_ICON,
                  "body": CLAY_GOLEM, "cooldown": 60.0},
     # one row makes it a toy illusion (entity/outfit.go); the golem has no armour, so it dresses nothing
     "outfit": {0: (0, 0)},
     "aura": {"color": 0, "kind": AURA_ICING, "source": 0, "nodes": 0,
              "note": "The Gingerbread Man: the clay golem baked brown and iced (vanilla toys::ice), the owner 2026-11-17"}},
]

LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    rowkey TEXT NOT NULL,
    before TEXT,
    after  TEXT,
    audit  INTEGER NOT NULL
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def cols(db, tbl):
    return {r[1] for r in db.execute(f"PRAGMA table_info({tbl})")}


def row(db, tbl, key):
    where = " AND ".join(f"{k} = ?" for k in key)
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", tuple(key.values()))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return None if m is None else json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, tbl, key, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    return db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                      "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                      (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key), go_json(before), go_json(after), DETAIL)).lastrowid


def write(db, log, tbl, key, want):
    """Make tbl's row of key want (a whole row; None deletes it), audited (and logged when log)."""
    before = row(db, tbl, key)
    where = " AND ".join(f"{k} = ?" for k in key)
    if want is None:
        db.execute(f"DELETE FROM {tbl} WHERE {where}", tuple(key.values()))
        action = "delete"
    elif before is None:
        c = list(want)
        db.execute(f"INSERT INTO {tbl} ({', '.join(c)}) VALUES ({', '.join('?' for _ in c)})", tuple(want[k] for k in c))
        action = "insert"
    else:
        sets = [c for c in want if c not in key]
        db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in sets)} WHERE {where}", tuple(want[c] for c in sets) + tuple(key.values()))
        action = "update"
    after = row(db, tbl, key)
    a = audit(db, action, tbl, key, before, after)
    if log:
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                   (tbl, go_json(key), go_json(before), go_json(after), a))


def record(db, t):
    """The toy's itempattern row as wanted: its base's, made the toy's."""
    base = row(db, "itempattern", {"patternid": t["base"]})
    want = dict(base, patternid=t["id"], itemname=t["name"], itemdesc=t["desc"], trade=0, lore=0, classuse=32767,
                raceuse=32767, levelreq=1, itemcost=0)
    want.update(t["set"])
    return want


def plan(db):
    """(writes [(tbl, key, want)], lines, refusals)."""
    need = ["itempattern", "itemaura", "itemeffect", "itemoutfit", "tokenshop", "gm_audit"]
    missing = [t for t in need if not table(db, t)]
    if missing:
        return [], [], [f"no {t} table here" for t in missing]
    if not {"color2", "kind2", "source2", "nodes2"} <= cols(db, "itemaura"):
        return [], [], ["itemaura has no second aura's columns (a server of two-auras first)"]
    refusals, lines, writes = [], [], []
    checks = {CAPE_BASE: lambda r: r["equipslot"] == 28 and r["model"] == STYLE,
              STAFF_BASE: lambda r: r["levelreq"] == 1 and r["equipslot"] == 15 and r["itemname"] == "Apprentice Staff",
              TOY_BASE: lambda r: r["equipslot"] == -1 and r["procanim"] == 2 and r["itemname"] == "Barrel of Monkeys"}
    for pid, ok in checks.items():
        r = row(db, "itempattern", {"patternid": pid})
        if r is None or not ok(r):
            refusals.append(f"itempattern {pid} is not the base looked for ({r and r['itemname']!r})")
    if not db.execute("SELECT 1 FROM tokenshop WHERE shop = ?", (SHOP,)).fetchone():
        refusals.append(f"no tokenshop rows for {SHOP} here: the Toy Shoppe first (and the Christmas event is Heirloom's)")
    if refusals:
        return [], lines, refusals
    refusals += taken_ids(db)
    if refusals:
        return [], lines, refusals
    for t in TOYS:
        if len(t["desc"]) > DESC_MAX or "|" in t["desc"] or not t["desc"].isascii():
            refusals.append(f"{t['name']}'s description is {len(t['desc'])} characters (at most {DESC_MAX}, ASCII, no '|')")
        if len(t["says"]) > 190 or "%" in t["says"] or "|" in t["says"]:
            refusals.append(f"{t['name']}'s words are {len(t['says'])} characters (at most 190, no '%' or '|')")
        other = db.execute("SELECT patternid FROM itempattern WHERE lower(trim(itemname)) = lower(?) AND patternid != ?",
                           (t["name"], t["id"])).fetchall()
        if other:
            refusals.append(f"another record is named {t['name']!r}: {[o[0] for o in other]}")
        cur = row(db, "itempattern", {"patternid": t["id"]})
        if cur is not None and cur["itemname"] != t["name"]:
            refusals.append(f"itempattern {t['id']} is {cur['itemname']!r}, not free")
            continue
        want = record(db, t)
        if cur != want:
            writes.append(("itempattern", {"patternid": t["id"]}, want))
            lines.append(f"itempattern {t['id']} {t['name']}: " + ("new" if cur is None else "made as below") +
                         f", a copy of {t['base']}'s shape: tradeable, every class and race")
        if table(db, "itemowner") and row(db, "itemowner", {"patternid": t["id"]}):
            refusals.append(f"itempattern {t['id']} is bound in itemowner: a shop's copy must be no one's")
        if "aura" in t:
            a = dict(t["aura"])
            a = {"patternid": t["id"], **{k: 0 for k in ("color2", "kind2", "source2", "nodes2")}, **a}
            have = row(db, "itemaura", {"patternid": t["id"]})
            if have != a:
                ours = table(db, LOG) and db.execute(f"SELECT 1 FROM {LOG} WHERE tbl = 'itemaura' AND rowkey = ?",
                                                     (go_json({"patternid": t["id"]}),)).fetchone()
                if have is not None and not ours:
                    refusals.append(f"itemaura {t['id']} is there and not this tool's")
                writes.append(("itemaura", {"patternid": t["id"]}, a))
                lines.append(f"itemaura {t['id']}: kind {a['kind']}" + (f", second kind {a['kind2']}" if a["kind2"] else ""))
        il = t.get("illusion")
        if il:
            e = {"patternid": t["id"], "line": il["line"], "name": il["name"], "icon": il["icon"], "breaks": "", "wear": 0,
                 "area": 0.0, "source": SRC, "body": il["body"], "target_line": "", "charges": 0, "cooldown": il["cooldown"]}
            have = row(db, "itemeffect", {"patternid": t["id"]})
            if have is not None and have.get("source") != SRC:
                refusals.append(f"itemeffect {t['id']} is there and not this tool's ({have.get('source')})")
            elif have != e:
                writes.append(("itemeffect", {"patternid": t["id"]}, e))
                lines.append(f"itemeffect {t['id']}: {il['line']!r}, body {il['body']:#010x}, cooldown {il['cooldown']:.0f} s")
            for slot, (look, color) in sorted(t["outfit"].items()):
                key = {"patternid": t["id"], "slot": slot}
                o = dict(key, look=look, color=color, note=f"{t['name']}: a toy illusion (the golem wears nothing)", source=SRC)
                if row(db, "itemoutfit", key) != o:
                    writes.append(("itemoutfit", key, o))
            lines.append(f"itemoutfit {t['id']}: {len(t['outfit'])} row (a toy illusion: use again to end it)")
        skey = {"shop": SHOP, "patternid": t["id"]}
        s = row(db, "tokenshop", skey)
        if s is None:
            writes.append(("tokenshop", skey, {"shop": SHOP, "patternid": t["id"], "price": t["price"], "token": "",
                                               "says": t["says"], "sort": t["sort"], "source": SRC}))
            lines.append(f"tokenshop: {SHOP} sells {t['name']} for {t['price']} Holly Tokens")
    return writes, lines, refusals


def ours(db, tbl, key):
    """Whether this tool's log holds a write of tbl's row of key (a re-run's own)."""
    return table(db, LOG) and db.execute(f"SELECT 1 FROM {LOG} WHERE tbl = ? AND rowkey = ?", (tbl, go_json(key))).fetchone() is not None


def taken_ids(db):
    """Refusals for each toy's id another has taken: a record this tool did not write, or rows of its tables for an id
    with no record of this tool's."""
    out = []
    for t in TOYS:
        pid = t["id"]
        rec = row(db, "itempattern", {"patternid": pid})
        if rec is not None and not ours(db, "itempattern", {"patternid": pid}):
            out.append(f"itempattern {pid} is taken ({rec['itemname']!r}), not free for {t['name']}")
            continue
        if rec is None:
            for tbl in ("itemaura", "itemeffect", "itemoutfit", "tokenshop"):
                if db.execute(f"SELECT 1 FROM {tbl} WHERE patternid = ?", (pid,)).fetchone():
                    out.append(f"{tbl} has a row for {pid} with no record: the id is not free for {t['name']}")
    return out


def held(db, pid):
    n = 0
    for tbl in ("charinventory", "accountbank", "charmercitem", "auction_lots"):
        if table(db, tbl) and "patternid" in cols(db, tbl):
            n += db.execute(f"SELECT count(*) FROM {tbl} WHERE patternid = ?", (pid,)).fetchone()[0]
    return n


def undo(db):
    if not table(db, LOG):
        return 0, []
    logged = db.execute(f"SELECT seq, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall()
    left = []
    for _, tbl, rowkey, before, after in logged:
        if go_json(row(db, tbl, json.loads(rowkey))) != after:
            left.append(f"{tbl} {rowkey}: changed since, nothing undone")
        if tbl == "itempattern" and before is None:
            pid = json.loads(rowkey)["patternid"]
            n = held(db, pid)
            if n:
                left.append(f"{n} copies of {pid} are held (packs, banks, mercs, auctions): nothing undone")
    if left:
        return 0, left
    for _, tbl, rowkey, before, _ in logged:
        write(db, False, tbl, json.loads(rowkey), json.loads(before) if before else None)
    db.execute(f"DROP TABLE {LOG}")
    return len(logged), []


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
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
            print(f"undone: {back} rows back" + "".join(f"\n  left: {w}" for w in left))
            return 0 if not left else 1
        writes, lines, refusals = plan(db)
        for line in lines:
            print(line)
        if refusals:
            db.execute("ROLLBACK")
            print("REFUSED, nothing written:\n  " + "\n  ".join(refusals))
            return 1
        if not writes:
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"(report only: {len(writes)} rows; --apply to write)")
            return 0
        db.execute(LOG_DDL)
        for tbl, key, want in writes:
            write(db, True, tbl, key, want)
        db.execute("COMMIT")
        print(f"applied: {len(writes)} rows, in gm_audit and {LOG}; the server reads itemeffect/itemoutfit at its start "
              "(a keep restart's before.sh); the panel's Reload every NPC reloads the item auras")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
