#!/usr/bin/env python3
"""Work board #688 (the GM Jibrish's, 2026-11-10; "jibrish decided all of this"): the Metal Moogie V3-V5 chain's
data, on the server's code of the same branch (melee_fixed, a creature's root, the chain's turn-ins and uses are
that code's: run it as the before.sh of the restart that brings it, never under an older server).

  itempattern, 7 new, each by its name (one there already is left as it is):
    Metal Moogie Legs / Torso / Arms / Head: the Moogie Tribute's record (lore, no trade, maxstack 1, its icon,
      equipslot -1) under its own name and words;
    Snowball: the tribute's record at maxstack 99, equipslot -1, levelreq 1, classuse/raceuse 32767, trade 0, lore 0;
    Potion of Moogie's Insight, Deputy's Draught: the tribute's record at maxstack 1, lore 0, trade 1 (no trade),
      levelreq 1, classuse/raceuse 32767 (the spec names no level or classes: every class may drink it);
    Moogie Winding Stone: Healbot's Cog's record (lore, no trade, procanim 7, duration 80, never used up);
  itemeffect for the three consumables, as the code reads them (entity/moogieitems.go):
    Snowball "Target, Snowball 0x15C6A459" (Frost Shot's projectile, a GM's to change), cooldown 1 s;
    Potion of Moogie's Insight "Self, Moogie's Insight", 1 charge;
    Deputy's Draught "Self, Deputy's Draught 901843, Radius 30" (Halfling Heroism), 1 charge;
  npcs, 4 new, spawn-only (an npcspawnonly row each, before they reach the world): Metal Moogie V3, V4, V5 as
    copies of Metal Moogie V2's row (his body, size, level, npc_type, his spot), and the Moogie Gearling, the same
    row at size 4.5;
  npcitem: V2's Moogie Tribute row out, one Metal Moogie Legs in; V3 one Torso, V4 one Arms, V5 one Head;
  classtemplates + npctemplate (by each name):
    metal moogie v3: V2's row as it is, his abilities and Arcane Siphon (a fixed 1,000 cut by AR, all back on him,
      every 30 s from 15 s);
    metal moogie v4: v3's and two Moogie Gearlings below 25% (share 0: their own template), his line;
    metal moogie v5: v4's and Self Repair (100% of his hit points below 50%, cooldown 3600 s, his line). The spec's
      "every": 1 is under the templates' least clock (2 s: database.CheckTemplateAbilities refuses 1 and the
      template would read as unreadable), so it is 2: a heal waits, due, until he is below half, so the first fall
      below 50% is healed at once either way;
    moogie gearling: class 0, hp_pct 35 (5,040 at 60), taken_pct 100, melee_fixed 25, no special, kick, proc or
      shield; its root pinned to spell 460 (Root, for its look), every 18 s from 3 s;
  familiars: the Winding Stone's, "Mini Metal Moogie", model 100810757, size 2.0, hp 100000, regen 0, cast_ms 2000,
    cast_fx 2831335833, heal 0.
  classtemplates.melee_fixed is added (INTEGER NOT NULL DEFAULT 0, as the server adds it at its start) when the table
    has none, so the rows can be written before the new server first runs; --undo leaves the column.

Every row is found by name, and nothing is written unless what it copies is there as the spec found it: Metal Moogie
V2 (one spawn-only row, npc_type 0, level 60), his template, the Moogie Tribute, Healbot's Cog, spells 460 and 901843.
A shard without Metal Moogie V2 says so and writes nothing (exit 0). It is idempotent: what is there already is left
and said, and a second run writes nothing. The ids are the tables' next free ones at the run, never fixed (the
shards' item ids diverge).

WHEN IT TAKES EFFECT: the server reads npcs, npcspawnonly, npcitem, the templates and familiars at its start (a GM
panel template edit or reload_npcs reads templates again at once); itempattern and itemeffect are read at each use
and login. Run it as the before.sh of the restart that brings this code.

--dry (the default) prints what it would write; --apply writes it in one transaction, every row with its gm_audit
line and its own line in moogie_chain_688_log; --undo takes back each row still as written (one changed since is left
and said) and puts V2's tribute row back.

    tools/moogie_chain_688_20261115.py [--db ~/eqoa-server/eqoa.db] [--dry | --apply | --undo]
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

LOG = "moogie_chain_688_log"
SRC = "moogie688"
WHO, VIA = "dab", "tool"
DETAIL = "work board #688 (Jibrish): the Metal Moogie V3-V5 chain, tools/moogie_chain_688_20261115.py"
DEFAULT_DB = os.path.expanduser("~/eqoa-server/eqoa.db")  # Heirloom's; Live's is another --db

V2, V2_T = "Metal Moogie V2", "metal moogie v2"
VERSIONS = ["Metal Moogie V3", "Metal Moogie V4", "Metal Moogie V5"]
GEARLING, GEARLING_SIZE = "Moogie Gearling", 4.5
TRIBUTE, COG = "Moogie Tribute", "Healbot's Cog"
ROOT_SPELL, HEROISM = 460, 901843
PART_DESC = "A piece of Metal Moogie's clockwork body. Mr. Moogie will want it back."
PARTS = {"Metal Moogie V2": "Metal Moogie Legs", "Metal Moogie V3": "Metal Moogie Torso",
         "Metal Moogie V4": "Metal Moogie Arms", "Metal Moogie V5": "Metal Moogie Head"}
ITEMS = [  # name, the record it copies, the fields set, its description
    ("Metal Moogie Legs", TRIBUTE, {}, PART_DESC),
    ("Metal Moogie Torso", TRIBUTE, {}, PART_DESC),
    ("Metal Moogie Arms", TRIBUTE, {}, PART_DESC),
    ("Metal Moogie Head", TRIBUTE, {}, PART_DESC),
    ("Snowball", TRIBUTE, {"maxstack": 99, "equipslot": -1, "levelreq": 1, "classuse": 32767, "raceuse": 32767, "trade": 0, "lore": 0},
     "Packed tight and cold. Throw it at another player."),
    ("Potion of Moogie's Insight", TRIBUTE, {"maxstack": 1, "lore": 0, "trade": 1, "levelreq": 1, "classuse": 32767, "raceuse": 32767},
     "+25% experience for one hour of play. Stacks with a town's Boon."),
    ("Deputy's Draught", TRIBUTE, {"maxstack": 1, "lore": 0, "trade": 1, "levelreq": 1, "classuse": 32767, "raceuse": 32767},
     "Turns everyone within 30 into a halfling deputy, like it or not."),
    ("Moogie Winding Stone", COG, {},
     "Winds up a tiny Metal Moogie who follows you about. Use it as often as you like: one familiar at a time."),
]
EFFECTS = {  # itemeffect by its item's name
    "Snowball": {"line": "Target, Snowball 0x15C6A459", "name": "", "icon": 0, "charges": 0, "cooldown": 1.0},
    "Potion of Moogie's Insight": {"line": "Self, Moogie's Insight", "name": "Moogie's Insight", "icon": 0xDA6F7DE9, "charges": 1, "cooldown": 0.0},
    "Deputy's Draught": {"line": f"Self, Deputy's Draught {HEROISM}, Radius 30", "name": "", "icon": 0, "charges": 1, "cooldown": 0.0},
}
ARCANE_SIPHON = {"kind": "lifetap", "name": "Arcane Siphon", "fx": 2943531157, "damage": 1000, "resist": "AR", "every": 30, "first": 15}
GEARLINGS = {"kind": "summon", "below": 25, "count": 2, "npc": GEARLING, "say": "Gearlings! Assist your master!"}
SELF_REPAIR = {"kind": "heal", "name": "Self Repair", "heal_pct": 100, "below": 50, "every": 2, "cooldown": 3600, "say": "Self repair complete."}
GEARLING_ROOT = {"kind": "root", "spell_id": ROOT_SPELL, "every": 18, "first": 3}
FAMILIAR = {"name": "Mini Metal Moogie", "model": 100810757, "size": 2.0, "hp": 100000, "regen": 0, "cast_ms": 2000,
            "cast_fx": 2831335833, "heal": 0}
KEYS = {"itempattern": ("patternid",), "itemeffect": ("patternid",), "npcs": ("id",), "npcspawnonly": ("name",),
        "npcitem": ("itemid",), "classtemplates": ("name",), "npctemplate": ("name",), "familiars": ("patternid",)}


def table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def has_column(db, tbl, col):
    return any(r[1] == col for r in db.execute(f"PRAGMA table_info({tbl})"))


def rows(db, q, args=()):
    cur = db.execute(q, args)
    cols = [d[0] for d in cur.description]
    return [dict(zip(cols, r)) for r in cur.fetchall()]


def row_of(db, tbl, key):
    where = " AND ".join(f"{c} = ?" for c in KEYS[tbl])
    r = rows(db, f"SELECT * FROM {tbl} WHERE {where}", tuple(key[c] for c in KEYS[tbl]))
    return r[0] if r else None


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def abilities_json(list_):
    return json.dumps(list_, separators=(",", ":"), ensure_ascii=False)


def npc_key(name):
    return name.strip().lower().rstrip("$").strip()


def item_named(db, name):
    r = rows(db, "SELECT * FROM itempattern WHERE lower(trim(itemname)) = ? ORDER BY patternid", (name.lower(),))
    return r


def plan(db):
    """(steps, lines, refusals, here). A step is (tbl, op, row, ref): op insert/delete; a row's value of the form
    {"ref": name} is the id of the item (or NPC) of that name, once written."""
    steps, lines, refusals = [], [], []
    v2 = rows(db, "SELECT * FROM npcs WHERE npc_name = ?", (V2,))
    if not v2:
        lines.append(f"no {V2} here: not a shard of the Metal Moogie duel; nothing to do")
        return [], lines, [], False
    if len(v2) != 1:
        refusals.append(f"{len(v2)} NPCs named {V2}, want his one spawn-only original")
        return [], lines, refusals, True
    v2 = v2[0]
    if not db.execute("SELECT count(*) FROM npcspawnonly WHERE lower(name) = ?", (npc_key(V2),)).fetchone()[0]:
        refusals.append(f"{V2} is not spawn-only (npcspawnonly)")
    if v2["npc_level"] != 60 or v2["npc_type"] != 0:
        refusals.append(f"{V2} is level {v2['npc_level']}, npc_type {v2['npc_type']}: the spec's 60 and 0")
    v2t = row_of(db, "classtemplates", {"name": V2_T})
    if v2t is None:
        refusals.append(f"no class template {V2_T!r}")
    srcs = {}
    for name in (TRIBUTE, COG):
        found = item_named(db, name)
        if len(found) != 1:
            refusals.append(f"{len(found)} item records named {name!r}, want one")
        else:
            srcs[name] = found[0]
    for sid, name in ((ROOT_SPELL, "Root"), (HEROISM, "Halfling Heroism")):
        r = db.execute("SELECT spellname FROM spellpattern WHERE spellid = ?", (sid,)).fetchone()
        if r is None or (r[0] or "").strip().lower() != name.lower():
            refusals.append(f"spell {sid} is {r and r[0]!r}, not {name}")
    if refusals:
        return [], lines, refusals, True

    # the items, by name
    have = {}
    for name, src, fields, desc in ITEMS:
        found = item_named(db, name)
        if found:
            have[name] = found[0]["patternid"]
            lines.append(f"itempattern {name!r}: there already ({found[0]['patternid']}), left")
            continue
        rec = dict(srcs[src])
        del rec["patternid"]
        rec.update(fields, itemname=name, itemdesc=desc)
        steps.append(("itempattern", "insert", rec, None))
        diff = ", ".join(f"{k} {v}" for k, v in fields.items()) or "as it is"
        lines.append(f"itempattern {name!r}: {src}'s record ({srcs[src]['patternid']}), {diff}")
    for name, e in EFFECTS.items():
        pid = have.get(name)
        if pid is not None and row_of(db, "itemeffect", {"patternid": pid}) is not None:
            lines.append(f"itemeffect of {name!r} ({pid}): there already, left")
            continue
        rec = {"patternid": {"ref": name}, "line": e["line"], "name": e["name"], "icon": e["icon"], "breaks": "", "wear": 0,
               "area": 0, "source": SRC, "body": 0, "target_line": "", "charges": e["charges"], "cooldown": e["cooldown"]}
        steps.append(("itemeffect", "insert", rec, None))
        lines.append(f"itemeffect of {name!r}: {e['line']!r}, {e['charges']} charges, cooldown {e['cooldown']:g} s")

    # the NPCs, spawn-only, and their items
    npc_ids = {V2: v2["id"]}
    for name, size in [(n, v2["size"]) for n in VERSIONS] + [(GEARLING, GEARLING_SIZE)]:
        found = rows(db, "SELECT * FROM npcs WHERE npc_name = ?", (name,))
        if found:
            npc_ids[name] = found[0]["id"]
            lines.append(f"npcs {name!r}: there already ({found[0]['id']}), left")
        else:
            rec = dict(v2, npc_name=name, size=size)
            del rec["id"]
            steps.append(("npcs", "insert", rec, None))
            lines.append(f"npcs {name!r}: {V2}'s row ({v2['id']}) at size {size}, level {v2['npc_level']}, npc_type {v2['npc_type']}, "
                         f"at ({v2['x_coord']}, {v2['y_coord']}, {v2['z_coord']}) world {v2['world']}")
        key = npc_key(name)
        if not db.execute("SELECT count(*) FROM npcspawnonly WHERE lower(name) = ?", (key,)).fetchone()[0]:
            steps.append(("npcspawnonly", "insert", {"name": key, "note": f"Work board #688: Mr. Moogie's chain copies {name}.",
                                                      "source": SRC}, None))
            lines.append(f"npcspawnonly {key!r}")
    tribute_rows = rows(db, "SELECT * FROM npcitem WHERE npcid = ? AND patternid = ?", (v2["id"], srcs[TRIBUTE]["patternid"]))
    for r in tribute_rows:
        steps.append(("npcitem", "delete", r, None))
        lines.append(f"npcitem {r['itemid']}: {V2}'s {TRIBUTE} out")
    for who, part in PARTS.items():
        nid = npc_ids.get(who)
        if nid is not None and have.get(part) is not None and db.execute(
                "SELECT count(*) FROM npcitem WHERE npcid = ? AND patternid = ?", (nid, have[part])).fetchone()[0]:
            lines.append(f"npcitem: {who}'s {part}: there already, left")
            continue
        rec = {"npcid": {"npc": who}, "stackleft": 1, "remainHP": 0, "remaincharge": 0, "patternid": {"ref": part},
               "equiploc": 0, "location": 1, "listnumber": 0}
        steps.append(("npcitem", "insert", rec, None))
        lines.append(f"npcitem: {who} gives one {part}")

    # the templates
    v2_abil = json.loads(v2t["abilities"] or "[]")
    chain = {"metal moogie v3": v2_abil + [ARCANE_SIPHON]}
    chain["metal moogie v4"] = chain["metal moogie v3"] + [GEARLINGS]
    chain["metal moogie v5"] = chain["metal moogie v4"] + [SELF_REPAIR]
    notes = {"metal moogie v3": "Work board #688 (Jibrish): Metal Moogie V3, V2's template and Arcane Siphon (a fixed 1,000 cut by AR, back on him).",
             "metal moogie v4": "Work board #688 (Jibrish): Metal Moogie V4, V3's and two Moogie Gearlings below 25%.",
             "metal moogie v5": "Work board #688 (Jibrish): Metal Moogie V5, V4's and Self Repair (whole below 50%, once a duel); "
                                "the rest of his script is code (entity/metalv5.go). every 2: the templates' least clock (the spec's 1)."}
    for name, abil in chain.items():
        if row_of(db, "classtemplates", {"name": name}) is not None:
            lines.append(f"classtemplates {name!r}: there already, left")
        else:
            rec = dict(v2t, name=name, abilities=abilities_json(abil), note=notes[name], source=SRC)
            if "melee_fixed" in rec:
                rec["melee_fixed"] = 0
            steps.append(("classtemplates", "insert", rec, None))
            lines.append(f"classtemplates {name!r}: {V2_T}'s row (hp {v2t['hp_pct']}%, melee {v2t['melee_pct']}%, proc "
                         f"{v2t['proc_chance']}%/{v2t['proc_damage']} {v2t['proc_name']} {v2t['proc_resist']}, kick "
                         f"{v2t['kick_damage']}/{v2t['kick_every']:g}), {len(abil)} abilities: {abilities_json(abil)}")
    gname = npc_key(GEARLING)
    if row_of(db, "classtemplates", {"name": gname}) is not None:
        lines.append(f"classtemplates {gname!r}: there already, left")
    else:
        rec = {"name": gname, "class": 0, "hp_pct": 35, "taken_pct": 100, "melee_pct": 100, "melee_fixed": 25,
               "abilities": abilities_json([GEARLING_ROOT]),
               "note": "Work board #688 (Jibrish): Moogie Gearling, a warrior (quads at 60), 5,040 hit points, a fixed 25 a hit, "
                       "a root of 6 s every 18 s (spell 460's look).", "source": SRC}
        steps.append(("classtemplates", "insert", rec, None))
        lines.append(f"classtemplates {gname!r}: class 0, hp 35%, melee_fixed 25, {rec['abilities']}")
    for name in list(chain) + [gname]:
        if row_of(db, "npctemplate", {"name": name}) is None:
            steps.append(("npctemplate", "insert", {"name": name, "template": name,
                                                     "note": "Work board #688: fights by its own template.", "source": SRC}, None))
            lines.append(f"npctemplate {name!r} -> {name!r}")

    # the familiar
    stone = have.get("Moogie Winding Stone")
    if stone is not None and table(db, "familiars") and row_of(db, "familiars", {"patternid": stone}) is not None:
        lines.append(f"familiars of the Winding Stone ({stone}): there already, left")
    elif not table(db, "familiars"):
        refusals.append("no table familiars (an older server's database)")
    else:
        rec = dict(FAMILIAR, patternid={"ref": "Moogie Winding Stone"},
                   note="Work board #688 (Jibrish): the Moogie Winding Stone's Mini Metal Moogie, who only follows.", source=SRC)
        steps.append(("familiars", "insert", rec, None))
        lines.append(f"familiars: the Winding Stone's {FAMILIAR['name']!r}, model {FAMILIAR['model']}, size {FAMILIAR['size']}, heal 0")
    return steps, lines, refusals, True


LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,
    rowkey  TEXT NOT NULL,
    before  TEXT,
    after   TEXT,
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def audit(db, action, tbl, key, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key),
                      None if before is None else go_json(before), None if after is None else go_json(after), DETAIL))
    return cur.lastrowid


def log_line(db, tbl, key, before, after, a):
    db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
               (tbl, json.dumps(key), None if before is None else json.dumps(before, sort_keys=True, default=str),
                None if after is None else json.dumps(after, sort_keys=True, default=str), a))


def ensure_melee_fixed(db, out):
    if not has_column(db, "classtemplates", "melee_fixed"):
        db.execute("ALTER TABLE classtemplates ADD COLUMN melee_fixed INTEGER NOT NULL DEFAULT 0")
        out.append("classtemplates.melee_fixed added (INTEGER NOT NULL DEFAULT 0)")


def apply(db, steps):
    """The plan written in the caller's transaction; the ids it took, by name."""
    db.execute(LOG_SCHEMA)
    items = {r["itemname"].strip().lower(): r["patternid"] for r in rows(db, "SELECT patternid, itemname FROM itempattern")}
    npcs = {}
    took = {}
    for tbl, op, rec, _ in steps:
        rec = dict(rec)
        for c, v in list(rec.items()):
            if isinstance(v, dict) and "ref" in v:
                rec[c] = items[v["ref"].lower()]
            elif isinstance(v, dict) and "npc" in v:
                rec[c] = npcs.get(v["npc"]) or db.execute("SELECT id FROM npcs WHERE npc_name = ?", (v["npc"],)).fetchone()[0]
        if op == "delete":
            key = {c: rec[c] for c in KEYS[tbl]}
            db.execute(f"DELETE FROM {tbl} WHERE " + " AND ".join(f"{c} = ?" for c in KEYS[tbl]), [key[c] for c in KEYS[tbl]])
            log_line(db, tbl, key, rec, None, audit(db, "delete", tbl, key, rec, None))
            continue
        cols = list(rec)
        cur = db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [rec[c] for c in cols])
        if tbl == "itempattern":
            key = {"patternid": cur.lastrowid}
            items[rec["itemname"].strip().lower()] = cur.lastrowid
            took[rec["itemname"]] = cur.lastrowid
        elif tbl == "npcs":
            db.execute("UPDATE npcs SET npc_id = ? WHERE id = ?", (cur.lastrowid, cur.lastrowid))
            key = {"id": cur.lastrowid}
            npcs[rec["npc_name"]] = cur.lastrowid
            took[rec["npc_name"]] = cur.lastrowid
        elif tbl == "npcitem":
            key = {"itemid": cur.lastrowid}
            took[f"npcitem {cur.lastrowid}"] = rec["npcid"]
        else:
            key = {c: rec[c] for c in KEYS[tbl]}
        row = row_of(db, tbl, key)
        log_line(db, tbl, key, None, row, audit(db, "insert", tbl, key, None, row))
    return took


def undo(db):
    """Each logged row taken back while still as written, newest first; a deleted row put back."""
    if not table(db, LOG):
        return ["nothing logged: nothing to undo"]
    out = []
    for seq, tbl, rowkey, before, after in db.execute(f"SELECT seq, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key = json.loads(rowkey)
        now = row_of(db, tbl, key)
        if after is not None:  # made here
            if now is None:
                out.append(f"{tbl} {key}: gone already")
            elif json.loads(json.dumps(now, sort_keys=True, default=str)) != json.loads(after):
                out.append(f"{tbl} {key}: changed since, LEFT")
                continue
            else:
                db.execute(f"DELETE FROM {tbl} WHERE " + " AND ".join(f"{c} = ?" for c in KEYS[tbl]), [key[c] for c in KEYS[tbl]])
                audit(db, "delete", tbl, key, now, None)
                out.append(f"{tbl} {key}: taken out")
        else:  # deleted here
            b = json.loads(before)
            if now is not None:
                out.append(f"{tbl} {key}: its id taken since, LEFT out")
                continue
            cols = list(b)
            db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [b[c] for c in cols])
            audit(db, "insert", tbl, key, None, row_of(db, tbl, key))
            out.append(f"{tbl} {key}: put back")
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB, help=f"the server's database (default Heirloom's {DEFAULT_DB})")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--dry", action="store_true", help="print what it would write (the default)")
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    args = ap.parse_args()
    if not os.path.exists(args.db):
        sys.exit(f"no database at {args.db}")
    db = sqlite3.connect(args.db, timeout=5)
    db.execute("PRAGMA foreign_keys = OFF")
    if args.undo:
        with db:
            for line in undo(db):
                print(line)
        return
    steps, lines, refusals, here = plan(db)
    for line in lines:
        print(line)
    if refusals:
        for r in refusals:
            print("REFUSED:", r)
        sys.exit(2)
    if not here or not steps:
        print("nothing to write")
        return
    if not args.apply:
        print(f"--dry: {len(steps)} rows would be written; --apply writes them")
        return
    with db:
        out = []
        ensure_melee_fixed(db, out)
        took = apply(db, steps)
    for line in out:
        print(line)
    for name, id_ in took.items():
        print(f"took {name}: {id_}")
    print(f"applied: {len(steps)} rows, logged in {LOG}")


if __name__ == "__main__":
    main()
