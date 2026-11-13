#!/usr/bin/env python3
"""A rogue's skeleton disguise (ticket 1998, Hoofbite, a level 60 halfling rogue, 2026-11-13: "Request to add
skeleton diguise spell for rogue. :)"; the owner approved it).

The spell: "Skeletal Mask", the last of the rogue's masks (Mortal Mask 3, Deceiver's Mask 37, Scholar's Mask 47,
Brute's Mask 57: each a self illusion of 10 minutes whose body is the workbook's sprite), built as they are: Brute's
Mask's frame (a 5 s cast, 80 power, recast 1, its range and scope 0, its cast look), "Self, Gives Form, 10min", the
workbook's illusion effect with the sprite of a skeleton, stack 2 as the masks. Level 60: the masks step ten levels
(37, 47, 57) and this one is the line's last, sold beside the rogue's other level 60 scrolls (Wiles, Electrified
Blade). No "Faction Change" (the masks' line has it; the server does nothing with it, illusion.go): a body alone.

The body: the skeleton most of this server's skeleton NPCs wear (npcs.modelid 1794738817 = CSprite 0x6AF98A81: 241
creatures named skeleton, skeletal guard, skeleton scholar ...), read off the rows at run time and refused if it is
another; the effect's sprite is that id read the other way round (the workbook's byte order, spelljson.go
spriteBody: 0x818AF96A), as every illusion's is. It shows at its bearer's own size (illusion.go landIllusionLocked: a
creature's body by sprite takes the bearer's scale, a race row's or 1.0; a halfling's is 1.0), as a halfling under
Mortal Mask is a full-size human.

Its icon: the enchanter illusions' glowing pink man (UI.CSF 1507275072 with Remove Illusion's colour), which
petition 1347's fix (petfix153) gave every Glamour and Illusion; read off Remove Illusion (by name) and refused if it
is not that icon. It ends as the masks do: at its 10 minutes, at Remove Mask (the rogue's own, a cure "mask": every
illusion, workbook.go cureMatches), at another illusion (one replaces another: endIllusionsLocked) and at death.

How rogues get it: as the rogue's level 60 spells are got here, a scroll sold by the rogue's merchant, Merchant
Morian (both shards), and by every Spell Archivist (the Heirloom server's 23; the Live server has none): its record a
copy of Wiles' scroll ("This scroll will allow you to learn Skeletal Mask.", rogue only, level 60, its price), each
seller's row a copy of its Wiles row, listed beside Brute's Mask where the seller has it, else last. A classspells row
(rogue, source "vendor"): the Spell Archivist's Grant basic spells, #template and a server's start_all_spells give it
by level as they give the other vendor spells.

Ids differ between shards: the spell and the scroll are made at each one's next id (AUTOINCREMENT; the id counters put
back by --undo), the sellers found by name. Refused, writing nothing, when Brute's Mask, Remove Illusion's icon, Wiles'
scroll, the skeleton body or Merchant Morian are not as above, or when a spell or item of the new name is there
already but not all of this tool's rows are. Report only by default; --apply in one transaction, a gm_audit line a
row, logged in rogue_skeleton_1998_log; a second --apply finds it done and writes nothing; --undo exact (a row changed
since is left and said).

WHEN IT TAKES EFFECT: no restart. A cast reads its spell's effects then (handlers/cast.go), the scribe reads the spell
names at each scroll, a merchant's wares at each look, the Spell Archivist's within 10 minutes (archiveTTL); a client
has the new scroll's item record from its next login.

    tools/rogue_skeleton_1998_20261113.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "rogue_skeleton_1998_log"
DETAIL = "ticket 1998 (Hoofbite): a rogue's skeleton disguise, Skeletal Mask: tools/rogue_skeleton_1998_20261113.py"
WHO, VIA = "claude", "tool"
ROGUE, ROGUE_BIT = 6, 64

NAME = "Skeletal Mask"
LEVEL = 60
DESC = "Self, Skeleton Mask, 10min"  # the masks' own way of saying it (Brute's Mask: "Self, Ogre Mask, 10min")
LINE = "Self, Gives Form, 10min"
SOURCE = ("ticket 1998 (Hoofbite); the rogue's masks' frame (Brute's Mask), a skeleton NPC's body, the illusions' "
          "pink man (tools/rogue_skeleton_1998_20261113.py)")
FRAME = "Brute's Mask"          # whose cast, power, range, scope, recast and look it takes
ICON_OF = "Remove Illusion"     # whose icon and colour it takes: the pink man
PINK_MAN = 1507275072
SCROLL_OF = "Wiles"             # the rogue's level 60 scroll whose record it copies
SCROLL_ICON = -607649328
SCROLL_DESC = f"This scroll will allow you to learn {NAME}."
SKELETON = 1794738817           # npcs.modelid of most skeleton NPCs (CSprite 0x6AF98A81)
SELLERS = ("Merchant Morian", "Spell Archivist")
CLASS_SOURCE = "vendor"

TABLES = ("spellpattern", "spelleffect", "classspells", "itempattern", "npcitem", "sqlite_sequence")
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,      -- {' | '.join(TABLES)}
    rowkey  TEXT NOT NULL,      -- the row's key (JSON)
    before  TEXT,               -- the whole row as it was (JSON); NULL: the row was added
    after   TEXT NOT NULL,      -- the whole row as written (JSON)
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def sprite_of(model):
    """The workbook's sprite of a CSprite: its id read the other way round (spelljson.go spriteBody)."""
    return "0x%08X" % int.from_bytes((model & 0xFFFFFFFF).to_bytes(4, "big"), "little")


def effects_json(sprite):
    return {"v": 1, "scope": "self", "target_type": "Self", "harmful": False, "duration_s": 600, "stack": 2,
            "level": LEVEL, "min_level": 1, "req_equipment": "Any", "sheet_type": "Alter Appearance",
            "arch_type": "Illusion",
            "effects": [{"type": "Alter Appearance", "kind": "illusion", "sprite": sprite}]}


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def compact(v):
    return json.dumps(v, separators=(",", ":"))


def audit(db, action, tbl, key, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key), go_json(before or {}), go_json(after), detail))
    return cur.lastrowid


def row(db, tbl, key):
    w = " AND ".join(f"{k} = ?" for k in key)
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {w}", list(key.values()))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def one_by_name(db, tbl, idcol, namecol, name, extra=""):
    return [r[0] for r in db.execute(f"SELECT {idcol} FROM {tbl} WHERE lower(trim({namecol})) = lower(?) {extra} ORDER BY {idcol}",
                                     (name,)).fetchall()]


def skeleton_body(db):
    r = db.execute("""SELECT modelid, COUNT(*) FROM npcs WHERE lower(npc_name) LIKE '%skelet%' AND modelid != 0
                      GROUP BY modelid ORDER BY 2 DESC, 1 LIMIT 1""").fetchone()
    return r and r[0]


def sellers(db, scroll):
    """Each seller's id with its row of the copied scroll: Merchant Morian and every Spell Archivist selling it."""
    out = []
    for name in SELLERS:
        for (npc,) in db.execute("SELECT id FROM npcs WHERE npc_name = ? AND (npc_type & 1) != 0 ORDER BY id", (name,)).fetchall():
            r = db.execute("SELECT itemid FROM npcitem WHERE npcid = ? AND patternid = ? ORDER BY itemid LIMIT 1",
                           (npc, scroll)).fetchone()
            if r:
                out.append((name, npc, r[0]))
    return out


def plan(db):
    """(state, refusals, notes, found): state "todo" or "done"; found holds what apply copies."""
    refusals, notes, found = [], [], {}
    for t in ("spellpattern", "spelleffect", "classspells", "itempattern", "npcitem", "npcs", "gm_audit"):
        if not table(db, t):
            return None, [f"no {t} table here"], notes, found
    frame = db.execute("""SELECT p.spellid FROM spellpattern p JOIN classspells c ON c.spellid = p.spellid AND c.class = ?
                          WHERE p.spellname = ?""", (ROGUE, FRAME)).fetchall()
    if len(frame) != 1 or row(db, "spelleffect", {"spellid": frame[0][0]}) is None:
        refusals.append(f"{FRAME!r}: {len(frame)} rogue spells of the name (want one, with its effects row)")
    else:
        found["frame"] = frame[0][0]
    icon = db.execute("SELECT spellid, icon, iconColor FROM spellpattern WHERE spellname = ? AND icon = ? ORDER BY spellid LIMIT 1",
                      (ICON_OF, PINK_MAN)).fetchone()
    if icon is None:
        refusals.append(f"no {ICON_OF!r} carries the pink man ({PINK_MAN})")
    else:
        found["icon"], found["iconColor"] = icon[1], icon[2]
    scroll = db.execute("SELECT patternid FROM itempattern WHERE itemname = ? AND classuse = ? AND itemicon = ? ORDER BY patternid",
                        (SCROLL_OF, ROGUE_BIT, SCROLL_ICON)).fetchall()
    if len(scroll) != 1:
        refusals.append(f"{SCROLL_OF!r}: {len(scroll)} rogue scrolls of the name (want one)")
    else:
        found["scroll"] = scroll[0][0]
    body = skeleton_body(db)
    if body != SKELETON:
        refusals.append(f"the skeleton NPCs' most common body is {body}, not {SKELETON} (0x6AF98A81): look first")
    if refusals:
        return None, refusals, notes, found
    found["sprite"] = sprite_of(body)
    sell = sellers(db, found["scroll"])
    if sum(1 for s in sell if s[0] == SELLERS[0]) != 1:
        return None, [f"{SELLERS[0]} selling {SCROLL_OF}: {sum(1 for s in sell if s[0] == SELLERS[0])} (want one)"], notes, found
    found["sellers"] = sell

    spells = one_by_name(db, "spellpattern", "spellid", "spellname", NAME)
    items = one_by_name(db, "itempattern", "patternid", "itemname", NAME)
    if not spells and not items:
        notes.append(f"{NAME}: rogue level {LEVEL}, sprite {found['sprite']} (body {body}), sold by "
                     f"{len(sell)} sellers ({', '.join(sorted({s[0] for s in sell}))})")
        return "todo", refusals, notes, found
    if len(spells) == 1 and len(items) == 1 and done(db, spells[0], items[0], sell):
        notes.append(f"{NAME} is there already: spell {spells[0]}, scroll {items[0]}, every seller has it")
        return "done", refusals, notes, found
    return None, [f"a spell ({spells}) or item ({items}) named {NAME!r} is there, but not as this tool leaves it: look first"], notes, found


def done(db, sid, pid, sell):
    if row(db, "spelleffect", {"spellid": sid}) is None or row(db, "classspells", {"class": ROGUE, "spellid": sid}) is None:
        return False
    for _, npc, _ in sell:
        if db.execute("SELECT 1 FROM npcitem WHERE npcid = ? AND patternid = ?", (npc, pid)).fetchone() is None:
            return False
    return True


def seq(db, name):
    r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = ?", (name,)).fetchone()
    return None if r is None else r[0]


def log(db, tbl, key, before, after, a):
    db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
               (tbl, json.dumps(key, sort_keys=True), None if before is None else json.dumps(before, sort_keys=True),
                json.dumps(after, sort_keys=True), a))


def insert(db, tbl, values, keycol):
    cols = list(values)
    rid = db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' for _ in cols)})",
                     [values[c] for c in cols]).lastrowid
    key = {keycol: values.get(keycol, rid)} if keycol else {k: values[k] for k in ("class", "spellid")}
    written = row(db, tbl, key)
    log(db, tbl, key, None, written, audit(db, "insert", tbl, key, None, written, DETAIL))
    return rid


def apply(db, found):
    """Writes the rows; returns (spell id, scroll id, seller rows)."""
    db.execute(LOG_SCHEMA)
    seqs = {t: seq(db, t) for t in ("spellpattern", "itempattern", "npcitem")}
    frame = row(db, "spellpattern", {"spellid": found["frame"]})
    frame_fx = row(db, "spelleffect", {"spellid": found["frame"]})["fxdict"]
    pattern = {"abilitylvl": LEVEL, "unk2": frame["unk2"], "unk3": frame["unk3"], "range": frame["range"],
               "casttime": frame["casttime"], "power": frame["power"], "iconColor": found["iconColor"],
               "icon": found["icon"], "scope": frame["scope"], "recast": frame["recast"],
               "eqprequire": frame["eqprequire"], "spellname": NAME, "spelldesc": DESC}
    sid = insert(db, "spellpattern", pattern, "spellid")
    insert(db, "spelleffect", {"spellid": sid, "line": LINE, "fxdict": frame_fx, "source": SOURCE,
                               "effects": compact(effects_json(found["sprite"]))}, "spellid")
    insert(db, "classspells", {"class": ROGUE, "spellid": sid, "source": CLASS_SOURCE}, None)

    scroll = row(db, "itempattern", {"patternid": found["scroll"]})
    item = {k: v for k, v in scroll.items() if k != "patternid"}
    item.update(itemname=NAME, itemdesc=SCROLL_DESC, levelreq=LEVEL)
    pid = insert(db, "itempattern", item, "patternid")

    n = 0
    for _, npc, copy in found["sellers"]:
        src = row(db, "npcitem", {"itemid": copy})
        beside = db.execute("""SELECT i.listnumber FROM npcitem i JOIN itempattern p ON p.patternid = i.patternid
                               WHERE i.npcid = ? AND p.itemname = ? AND p.classuse = ? ORDER BY i.itemid LIMIT 1""",
                            (npc, FRAME, ROGUE_BIT)).fetchone()
        last = db.execute("SELECT MAX(listnumber) FROM npcitem WHERE npcid = ?", (npc,)).fetchone()[0]
        values = {k: v for k, v in src.items() if k != "itemid"}
        values.update(patternid=pid, listnumber=beside[0] if beside else last + 1)
        insert(db, "npcitem", values, "itemid")
        n += 1
    # the id counters, put back by --undo as the rows are (no gm_audit line: not rows of the game's)
    for t, s0 in seqs.items():
        log(db, "sqlite_sequence", {"name": t}, None if s0 is None else {"name": t, "seq": s0}, {"name": t, "seq": seq(db, t)}, 0)
    return sid, pid, n


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for s, tbl, key, before, after in db.execute(f"SELECT seq, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        if tbl not in TABLES:
            raise SystemExit(f"{LOG} names table {tbl!r}, which this tool never writes")
        key, after = json.loads(key), json.loads(after)
        now = row(db, tbl, key)
        if now != after:
            left += 1
            print(f"  left: {tbl} {key}, {'gone' if now is None else 'changed since it was written'}")
            continue
        w = " AND ".join(f"{k} = ?" for k in key)
        if before is None:
            db.execute(f"DELETE FROM {tbl} WHERE {w}", list(key.values()))
            if tbl != "sqlite_sequence":
                audit(db, "delete", tbl, key, after, {}, "undo: " + DETAIL)
        else:
            before = json.loads(before)
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in before)} WHERE {w}",
                       list(before.values()) + list(key.values()))
            if tbl != "sqlite_sequence":
                audit(db, "update", tbl, key, after, before, "undo: " + DETAIL)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (s,))
        back += 1
    if left == 0:
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
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"put back {back}, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        state, refusals, notes, found = plan(db)
        for r in refusals:
            print("REFUSED:", r)
        if refusals:
            db.execute("ROLLBACK")
            return 1
        for n in notes:
            print(n)
        if state == "done":
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"report only: {5 + len(found['sellers'])} rows to write (--apply to write)")
            return 0
        sid, pid, n = apply(db, found)
        db.execute("COMMIT")
        print(f"applied: {NAME} is spell {sid}, its scroll item {pid}, sold by {n} sellers")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
