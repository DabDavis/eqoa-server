#!/usr/bin/env python3
"""The level 23-37 side quests made as the forum walkthroughs have them where side_quest_guides_20261121.py left ours (the
owner, 2026-11-21, of that tool's "left as ours" list: "lets fix these"). Run it AFTER side_quest_guides_20261121.py
--apply (that tool finds its steps by these NPCs' old names).

Five parts, each its own --part (all by default), logged together in side_quest_guides2_1121_log, undone together:

renames  The NPCs spelt as the guides spell them: Feria Hurkert -> Feria Huckert (her daughter Drew and Jimbo with her:
         one family), Apprentice Buema -> Apprentice Buerma, Razuan Marus -> Razuam Marus, Captain Hogofa -> Captain
         Ogofa, Father Manstein -> Father Manstien, Demetrios Velspane -> Dametrios Velspane, Phoborium -> Phorriun,
         Lelaia Shavrell -> Lelalia Shavererll, Svente Dormio -> Svenle Dormio, Enrique Revnold -> Enrique Reynold,
         Elmo Hagger -> Elmo Haggar. Every place a name keys data (NAME_COLS: the npcs rows and every rule table that
         matches an NPC by its name, any case, as NPCClassKey does, quests.giver, queststeps.target,
         queststeptargets), each value written in its own case style (a lowercase key stays lowercase). Their words go
         with them (TOKENS, whole words: the full names, and a given name said alone): the quests' texts and their steps' (offer,
         accept, decline, declined, started; journal, answer, says, waiting, notyet, reminder) of every quest that names
         one, the names and descriptions of the items those quests' steps want or give (Phoborium's Horn becomes
         Phorriun's Horn), and the renamed NPCs' own npcdialog lines. Not renamed: the faction "Feria Hurkert" (a
         faction, not an NPC: its standings are characters' rows the running server saves back under its own name), the
         guides' "Captain Ravensite" (the same thread also writes "CPT Ravenhite", ours), and quests' notes (a line is
         added). Refused if a new name is already any NPC's or any rule's, or if a gmtriggers step, a quest or step
         event, a camp's kinds or a tour names an old one (not handled here).
creatures The two creatures the guides name and we lacked, each the only source of its quest's item (the step's own
         drop, every kill, questpreloot.go; the kind's 25% stops, as the step no longer names the kind):
           Redheaded Bandit (t105: "a Redheaded Bandit (CONs yel-red?? to 23), this bandit roams and spawns near the
             fort"): a copy of the fort's red-haired arctic bandit (the one L25 row, hair colour 4, at its south
             outpost) beside it, level 26 (yellow-red to a 23, and an era comment's figure), respawn 600 s (era
             comments' ten minutes), a roamer (npcmove roam 1, radius 80); quest 100759 step 2 his alone, every kill.
           Blacktongue Ravager (t111: "kill a Blacktongue Ravager(CONs WHT) in Ferran's Hope (FH) for a Lantern"):
             a copy of the east camp's Blacktongue orc nearest (19330, 16930), level 37 (white to a 37), respawn 600 s
             (ours), standing; quest 100782 step 8 his alone, every kill.
         Each copy is every column of its row but the id (npc_id 0, as the content tools' copies), plus every rule row
         its kind has by name (class, template, faction, loot pools, kill hits ...) under the new name. Refused if a
         loot table, a held item or another quest step gives the quest item.
featherlite The shaman's Hodstock pieces as t105 names them: Featherlite Helm, Featherlite Tunic and Featherlite Gloves,
         new records copied whole from the Featherweave Coif, Tunic and Gloves given now (no figures in the guide), ids
         from ITEM_RANGE; given in parts 1, 2 and 3 (with Tribal Strength in part 1, as before). The Featherweave records
         are kept.
cleric   The cleric's Hodstock list read as t105 orders it: part 1 the cap (our Goldlight Coif) and Weaken Death, part 2
         the next Goldlight piece of the level (the Goldlight Boots; the guide's second "Goldlight Cap" is a slip), part
         3 the Goldlight Shield (it was part 2's, the boots part 3's: swapped).
tsekar   The red diamond's second seller (t107: "from either Grithmil Darkstone in Moradhim or Tsekar in Gree Rift"):
         Tsekar Kelriss a talker (npc_type 0 -> 128, as Grithmil) and quest 100766 step 3 done at him too
         (queststeptargets, the journal saying both). NEEDS THE SERVER that takes a talk, show or give step at any of
         its NPCs (branch quest-step-either-npc); an older one keeps the row and ignores it.

Everything goes by NAME: quests by source and name, NPC rows by name (and place for the copies' models), items by
itemname. Dry run by default (every change printed); --apply writes it in one transaction, each row with its gm_audit
line, logged; a second --apply finds nothing to do; --undo puts each row back still as this tool left it (a row changed
since is left and said), deletes what it made and puts each table's id sequence back.

WHEN IT TAKES EFFECT: NPC rows (the renames, the two creatures, Tsekar) at reload_npcs or the server's next start, the
quests at a GM panel quest edit or reload. The renames must reach the world and the quests together: run it as the
before.sh of a keep restart (the one that brings the tsekar part's server), or reload_npcs and reload the quests at
once.

    python3 -E -s tools/side_quest_guides2_20261121.py --db eqoa.db [--part renames,creatures,...] [--apply | --undo]
"""
import argparse
import datetime
import json
import re
import sqlite3
import sys

LOG = "side_quest_guides2_1121_log"
TOOL = "tools/side_quest_guides2_20261121.py"
DETAIL = "side quests as the forum walkthroughs, part 2 (the owner's \"lets fix these\"): " + TOOL
ACCOUNT, WHO, VIA = 1, "dab", "tool"
PARTS = ("renames", "creatures", "featherlite", "cleric", "tsekar")

RENAMES = [
    ("Feria Hurkert", "Feria Huckert"), ("Drew Hurkert", "Drew Huckert"), ("Jimbo Hurkert", "Jimbo Huckert"),
    ("Apprentice Buema", "Apprentice Buerma"), ("Razuan Marus", "Razuam Marus"), ("Captain Hogofa", "Captain Ogofa"),
    ("Father Manstein", "Father Manstien"), ("Demetrios Velspane", "Dametrios Velspane"), ("Phoborium", "Phorriun"),
    ("Lelaia Shavrell", "Lelalia Shavererll"), ("Svente Dormio", "Svenle Dormio"),
    ("Enrique Revnold", "Enrique Reynold"), ("Elmo Hagger", "Elmo Haggar"),
]
# whole words in the texts: the full names first, then the names said alone in these quests' words ("Tell Buema",
# "Razuan's Signet", "Lelaia's seal"); a surname alone is never taken (Vedel Revnold is another man)
TOKENS = [(o, n) for o, n in RENAMES] + [
    ("Buema", "Buerma"), ("Razuan", "Razuam"), ("Demetrios", "Dametrios"), ("Lelaia", "Lelalia"), ("Svente", "Svenle"),
]
FACTION_KEPT = "Feria Hurkert"  # a faction's name, not an NPC's: never touched

# (table, column) where an NPC's name keys data, any case (database.NPCClassKey)
NAME_COLS = [
    ("npcs", "npc_name"), ("npcclass", "name"), ("npctemplate", "name"), ("npcfaction", "name"),
    ("npcaggro", "npc_name"), ("npcsocial", "npc_name"), ("npcmove", "name"), ("npcloot", "npc_name"),
    ("lootcreature", "npc_name"), ("lootpool", "npc_name"), ("factionhits", "npc_name"),
    ("factionhitareas", "npc_name"), ("npcundead", "name"), ("npcspawnonly", "name"), ("npcdeath", "name"),
    ("npcheld", "npc_name"), ("npctint", "name"), ("npcdialog", "name"), ("npccharm", "name"), ("npcseason", "name"),
    ("npcdyes", "name"), ("npctime", "name"), ("npcwalkon", "name"), ("npctours", "name"), ("itemhide", "npc"),
    ("lightstrings", "npc"), ("gmportals", "npc_name"), ("gmportals", "exit_name"), ("questunlocks", "npc"),
    ("npcoverseer", "overseer"), ("npcoverseer", "boss"), ("queststeptargets", "target"), ("quests", "giver"),
    ("queststeps", "target"),
]
# a kind's rule rows a copy of it takes under its own name (not the rows that are the quests' or the people's)
COPY_RULES = [tc for tc in NAME_COLS if tc[0] not in (
    "npcs", "npcdialog", "queststeptargets", "quests", "queststeps", "questunlocks", "gmportals", "itemhide",
    "lightstrings", "npcoverseer", "npctours")]
# places a name may be said inside a value (refused, not handled, when an old name is there)
TEXT_KEYS = [("gmtriggers", "steps"), ("quests", "events"), ("queststeps", "events"), ("campspawns", "kinds"),
             ("gmcamps", "kinds"), ("npctours", "members"), ("npctours", "host")]
QUEST_TEXT = ("offer", "accept", "decline", "declined", "started")
STEP_TEXT = ("journal", "answer", "says", "waiting", "notyet", "reminder")

# (source, quest, step kind, old target, new name, the row's kind, the row's level, a place to stand it near (x, z),
#  offset, level, respawn s, roam radius or None, journal, the guide's words)
CREATURES = [
    ("EQOA-Data 23-47/23/p1", "The Redheaded Bandit", "loot", "arctic bandit", "Redheaded Bandit",
     "arctic bandit", 25, (17653.2, 5593.3), (5.0, 0.0), 26, 600, 80,
     "Kill the Redheaded Bandit, who roams round the bandit fort in Greyvax's Caves east of Bram, for his bandana.",
     "t105: a Redheaded Bandit (CONs yel-red?? to 23), this bandit roams and spawns near the fort"),
    ("EQOA-Data 23-47/37/rigo", "Poisontongue", "loot", "Blacktongue orc", "Blacktongue Ravager",
     "Blacktongue orc", None, (19330.0, 16930.0), None, 37, 600, None,
     "Kill the Blacktongue Ravager at the big orc camp down the river south of Bastable, in Ferran's Hope, for his"
     " lantern.",
     "t111: kill a Blacktongue Ravager(CONs WHT) in Ferran's Hope (FH) for a Lantern"),
]
CREATURE_NEAR = 60.0  # the row copied stands within this of the place given

FEATHERLITE = [  # (quest source, quest, the record given now, the guide's record)
    ("EQOA-Data 23-47/23/p1", "The Redheaded Bandit", "Featherweave Coif", "Featherlite Helm"),
    ("EQOA-Data 23-47/23/p2", "The Glowing Shield", "Featherweave Tunic", "Featherlite Tunic"),
    ("EQOA-Data 23-47/23/p3", "The Remains of Manger MacGray", "Featherweave Gloves", "Featherlite Gloves"),
]
SHAMAN, CLERIC = "8", "9"
ITEM_RANGE = (721000, 721099)
ITEM_LEVEL = 23

CLERIC_SWAP = [  # (quest source, quest, now, the guide's)
    ("EQOA-Data 23-47/23/p2", "The Glowing Shield", "Goldlight Shield", "Goldlight Boots"),
    ("EQOA-Data 23-47/23/p3", "The Remains of Manger MacGray", "Goldlight Boots", "Goldlight Shield"),
]

TSEKAR = "Tsekar Kelriss"
TSEKAR_QUEST = ("EQOA-Data 23-47/27/p2", "Saerk's Ring", 3, "give", "Grithmil Darkstone")
TSEKAR_JOURNAL = ("Buy a red diamond from Grithmil Darkstone in Moradhim or Tsekar Kelriss in the Green Rift"
                  " ({tunar:,} tunar).")
TALKER = 128

NOTE = " Against the forum guides 2026-11-21, part 2 ({}): {}."


class Refused(Exception):
    pass


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def fold(s):
    return " ".join(str(s or "").split()).lower()


def has_table(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone() is not None


def has_col(db, t, c):
    return has_table(db, t) and any(r[1] == c for r in db.execute(f'PRAGMA table_info("{t}")'))


def rows(db, sql, args=()):
    cur = db.execute(sql, args)
    cols = [c[0] for c in cur.description]
    return [dict(zip(cols, r)) for r in cur.fetchall()]


def by_rowid(db, t, rid):
    r = rows(db, f'SELECT rowid AS _rowid, * FROM "{t}" WHERE rowid = ?', (rid,))
    return r[0] if r else None


def named(db, t, c, name):
    """Rows of table t whose column c is name, any case and spaces round it, with their rowids."""
    if not has_col(db, t, c):
        return []
    return [r for r in rows(db, f'SELECT rowid AS _rowid, * FROM "{t}" WHERE lower(trim("{c}")) = ?', (fold(name),))]


def styled(old_value, new):
    """The new name in the old value's case style: a lowercase key stays lowercase."""
    v = str(old_value).strip()
    return new.lower() if v == v.lower() and v != v.upper() else new


def replace_tokens(text):
    if not text:
        return text
    out = text
    for o, n in TOKENS:
        out = re.sub(r"(?<![A-Za-z])" + re.escape(o) + r"(?![A-Za-z])", n, out)
    return out


def item_id(db, name, level=None, klass=None):
    q, a = "SELECT patternid FROM itempattern WHERE itemname = ?", [name]
    if level is not None:
        q, a = q + " AND levelreq = ?", a + [level]
    if klass is not None:
        q, a = q + " AND (classuse & ?) <> 0", a + [1 << int(klass)]
    ids = [r[0] for r in db.execute(q + " ORDER BY patternid", a)]
    return ids


def find_quest(db, source, name):
    qs = rows(db, "SELECT rowid AS _rowid, * FROM quests WHERE name = ? AND (source = ? OR source LIKE ?)",
              (name, source, source + " %"))
    if len(qs) != 1:
        raise Refused(f"{len(qs)} quests named {name!r} from {source!r}, not one")
    return qs[0]


def steps_of(db, qid):
    return rows(db, "SELECT rowid AS _rowid, * FROM queststeps WHERE questid = ? ORDER BY step", (qid,))


def ids_in(s):
    return [p.strip().split("*")[0] for p in str(s or "").split(",") if p.strip()]


def swap_ids(s, old, new):
    parts = [p.strip() for p in str(s or "").split(",") if p.strip()]
    out = []
    for p in parts:
        pid, _, cnt = p.partition("*")
        out.append((str(new) if pid == str(old) else pid) + (("*" + cnt) if cnt else ""))
    return ", ".join(out)


# ---- the plan: a list of changes, each (part, op, table, rowid or None, before row or None, after row) ----

class Plan:
    def __init__(self):
        self.changes = []  # (part, op, table, rowid, before, after, why)
        self.done = []
        self.said = []

    def update(self, part, t, before, changes, why):
        """A row's change; a second change of the same row is folded into the first (one log row a row)."""
        for i, c in enumerate(self.changes):
            if c[1] == "update" and c[2] == t and c[3] == before["_rowid"]:
                if any(c[5].get(k) != v for k, v in changes.items()):
                    c[5].update(changes)
                    self.changes[i] = c[:6] + (c[6] + "; " + why,)
                return
        after = dict(before)
        after.update(changes)
        if all(before.get(k) == v for k, v in changes.items()):
            return
        self.changes.append((part, "update", t, before["_rowid"], before, after, why))

    def view(self, t, row):
        """The row as the changes planned so far leave it."""
        for c in self.changes:
            if c[1] == "update" and c[2] == t and c[3] == row["_rowid"]:
                return dict(c[5])
        return row

    def insert(self, part, t, row, why):
        self.changes.append((part, "insert", t, None, None, row, why))


def plan_renames(db, p):
    old_names = [o for o, _ in RENAMES]
    if all(not named(db, "npcs", "npc_name", o) for o in old_names) and \
            all(named(db, "npcs", "npc_name", n) for _, n in RENAMES):
        p.done.append("renames: every NPC already has its guide's name")
        return
    for o, n in RENAMES:
        if not named(db, "npcs", "npc_name", o):
            raise Refused(f"renames: no NPC named {o!r}")
        for t, c in NAME_COLS:
            if named(db, t, c, n):
                raise Refused(f"renames: {t}.{c} already names {n!r}")
    for t, c in TEXT_KEYS:
        if not has_col(db, t, c):
            continue
        for o in old_names:
            hit = db.execute(f'SELECT count(*) FROM "{t}" WHERE "{c}" LIKE ?', ("%" + o + "%",)).fetchone()[0]
            if hit:
                raise Refused(f"renames: {hit} {t}.{c} values say {o!r}; not handled here")
    # every name-keyed place
    for o, n in RENAMES:
        for t, c in NAME_COLS:
            for r in named(db, t, c, o):
                p.update("renames", t, r, {c: styled(r[c], n)}, f"{o} -> {n}")
    # the quests that name one, their steps, their items; the renamed NPCs' own lines
    touched = set()
    for o, _ in RENAMES:
        touched |= {r["questid"] for r in named(db, "quests", "giver", o)}
        touched |= {r["questid"] for r in named(db, "queststeps", "target", o)}
        touched |= {r["questid"] for r in named(db, "queststeptargets", "target", o)}
    items = set()
    for qid in sorted(touched):
        q = rows(db, "SELECT rowid AS _rowid, * FROM quests WHERE questid = ?", (qid,))[0]
        ch = {k: replace_tokens(q[k]) for k in QUEST_TEXT if k in q and q[k]}
        ch["notes"] = (q["notes"] or "") + NOTE.format(TOOL, "the NPCs' names as the guides spell them")
        p.update("renames", "quests", q, ch, f"quest {qid}'s words")
        for st in steps_of(db, qid):
            p.update("renames", "queststeps", st, {k: replace_tokens(st[k]) for k in STEP_TEXT if st[k]},
                     f"quest {qid} step {st['step']}'s words")
            items |= set(ids_in(st["items"])) | set(ids_in(st["rewarditems"]))
            for e in (json.loads(st["events"]) if st["events"] else []):
                if isinstance(e, dict) and e.get("do") == "give":
                    items |= set(ids_in(e.get("items", "")))
    for pid in sorted(items, key=int):
        r = rows(db, "SELECT rowid AS _rowid, * FROM itempattern WHERE patternid = ?", (int(pid),))
        if r:
            p.update("renames", "itempattern", r[0], {"itemname": replace_tokens(r[0]["itemname"]),
                                                      "itemdesc": replace_tokens(r[0]["itemdesc"])},
                     f"item {pid}'s words")
    # the renamed NPCs' own lines (their rows' names are planned above: the words fold into those changes)
    for o, _ in RENAMES:
        for r in named(db, "npcdialog", "name", o):
            p.update("renames", "npcdialog", r, {"line": replace_tokens(r["line"])}, f"{o}'s line")


def plan_creatures(db, p):
    for (source, qname, kind, old_t, new_name, kind_name, row_level, (px, pz), off, level, respawn, roam,
         journal, why) in CREATURES:
        q = find_quest(db, source, qname)
        sts = [s for s in steps_of(db, q["questid"]) if s["kind"] == kind and fold(s["target"]) in (fold(old_t), fold(new_name))]
        if len(sts) != 1:
            raise Refused(f"creatures: quest {qname!r} has {len(sts)} {kind} steps of {old_t!r} or {new_name!r}")
        st = sts[0]
        item = ids_in(st["items"])
        if len(item) != 1:
            raise Refused(f"creatures: quest {qname!r} step {st['step']} wants {st['items']!r}")
        have = named(db, "npcs", "npc_name", new_name)
        if fold(st["target"]) == fold(new_name) and have:
            p.done.append(f"creatures: {new_name} stands and quest {q['questid']} step {st['step']} is his")
            continue
        if have:
            raise Refused(f"creatures: an NPC named {new_name!r} stands already, and the step is the kind's")
        for t, c in NAME_COLS:
            if t != "npcs" and named(db, t, c, new_name):
                raise Refused(f"creatures: {t}.{c} already names {new_name!r}")
        # nothing else gives the quest item
        for t, c in (("loottables", "patternid"), ("npcheld", "patternid")):
            if has_col(db, t, c):
                n = db.execute(f'SELECT count(*) FROM "{t}" WHERE "{c}" = ?', (int(item[0]),)).fetchone()[0]
                if n:
                    raise Refused(f"creatures: {n} {t} rows give item {item[0]} too")
        others = [s for s in rows(db, "SELECT questid, step, items FROM queststeps WHERE kind = 'loot' AND items LIKE ?",
                                  ("%" + item[0] + "%",))
                  if item[0] in ids_in(s["items"]) and (s["questid"], s["step"]) != (q["questid"], st["step"])]
        if others:
            raise Refused(f"creatures: other loot steps drop item {item[0]}: {[(s['questid'], s['step']) for s in others]}")
        cands = named(db, "npcs", "npc_name", kind_name)
        cands = [r for r in cands if (row_level is None or r["npc_level"] == row_level)
                 and (r["x_coord"] - px) ** 2 + (r["z_coord"] - pz) ** 2 <= CREATURE_NEAR ** 2]
        if not cands:
            raise Refused(f"creatures: no {kind_name} (level {row_level}) within {CREATURE_NEAR} of ({px}, {pz})")
        base = min(cands, key=lambda r: ((r["x_coord"] - px) ** 2 + (r["z_coord"] - pz) ** 2, r["id"]))
        row = {k: v for k, v in base.items() if k not in ("_rowid", "id")}
        row["npc_name"], row["npc_level"] = new_name, level
        if "npc_id" in row:
            row["npc_id"] = 0
        if off is not None:
            row["x_coord"], row["z_coord"] = base["x_coord"] + off[0], base["z_coord"] + off[1]
        else:
            row["x_coord"], row["z_coord"] = px, pz
        p.insert("creatures", "npcs", row, f"{new_name}, a copy of {kind_name} row {base['id']}: {why}")
        p.said.append(f"creatures: {new_name} from {kind_name} row {base['id']} ({base['x_coord']:.1f}, "
                      f"{base['z_coord']:.1f}) to ({row['x_coord']:.1f}, {row['z_coord']:.1f}), level {level}")
        # its respawn (by its row: filled in at the apply), its roam, its kind's rules under its name
        p.insert("creatures", "npcrespawn", {"npc_row": "$new:" + new_name, "seconds": float(respawn),
                                              "note": "the guide's named: " + TOOL, "source": "gm"}, f"{new_name}'s respawn")
        if roam is not None:
            if named(db, "npcmove", "name", new_name):
                raise Refused(f"creatures: npcmove names {new_name!r} already")
            p.insert("creatures", "npcmove", {"name": new_name.lower(), "roam": 1, "radius": float(roam), "water": "land",
                                              "note": "the guide's: he roams near the fort; " + TOOL, "source": "gm",
                                              "speed": 0.0}, f"{new_name} roams")
        for t, c in COPY_RULES:
            if t == "npcmove" and roam is not None:
                continue
            for r in named(db, t, c, kind_name):
                nr = {k: v for k, v in r.items() if k not in ("_rowid", "id")}
                nr[c] = styled(r[c], new_name)
                p.insert("creatures", t, nr, f"{new_name} takes {kind_name}'s {t} row")
        p.update("creatures", "queststeps", st, {"target": new_name, "chance": 1.0, "npcid": 0, "journal": journal},
                 f"quest {q['questid']} step {st['step']}: {new_name}'s alone, every kill")
        p.update("creatures", "quests", q, {"notes": (q["notes"] or "") + NOTE.format(
            TOOL, f"{new_name} made, the only source of the item ({why})")}, f"quest {q['questid']}'s notes")


def byclass_step(db, p, source, qname):
    """The quest's one step with class rewards, as the plan so far leaves it."""
    q = find_quest(db, source, qname)
    sts = [p.view("queststeps", s) for s in steps_of(db, q["questid"]) if s["byclass"]]
    if len(sts) != 1:
        raise Refused(f"quest {qname!r}: {len(sts)} steps with class rewards, not one")
    return q, sts[0], json.loads(sts[0]["byclass"])


def plan_featherlite(db, p):
    made = {}
    for source, qname, now_name, new_name in FEATHERLITE:
        q, st, bc = byclass_step(db, p, source, qname)
        cell = bc.get(SHAMAN, {})
        have = item_id(db, new_name, ITEM_LEVEL, SHAMAN)
        if have and have[0] in [int(i) for i in ids_in(cell.get("items", ""))]:
            p.done.append(f"featherlite: quest {q['questid']} gives {new_name} {have[0]}")
            continue
        old = item_id(db, now_name, ITEM_LEVEL, SHAMAN)
        if not old:
            raise Refused(f"featherlite: no level {ITEM_LEVEL} shaman record named {now_name!r}")
        old = old[0]
        if str(old) not in ids_in(cell.get("items", "")):
            raise Refused(f"featherlite: quest {q['questid']}'s shaman items are {cell.get('items')!r}, not {old}")
        if have:
            nid = have[0]
        else:
            if db.execute("SELECT count(*) FROM itempattern WHERE itemname = ?", (new_name,)).fetchone()[0]:
                raise Refused(f"featherlite: a record named {new_name!r} exists that no shaman of {ITEM_LEVEL} uses")
            used = {r[0] for r in db.execute("SELECT patternid FROM itempattern WHERE patternid BETWEEN ? AND ?", ITEM_RANGE)}
            used |= set(made.values())
            nid = next(i for i in range(ITEM_RANGE[0], ITEM_RANGE[1] + 1) if i not in used)
            base = rows(db, "SELECT * FROM itempattern WHERE patternid = ?", (old,))[0]
            row = dict(base, patternid=nid, itemname=new_name)
            p.insert("featherlite", "itempattern", row, f"{new_name} {nid}, a copy of {now_name} {old}")
            made[new_name] = nid
        bc2 = json.loads(st["byclass"])
        bc2[SHAMAN] = dict(cell, items=swap_ids(cell.get("items", ""), old, nid))
        p.update("featherlite", "queststeps", st, {"byclass": json.dumps(bc2, sort_keys=True, separators=(",", ":"))},
                 f"quest {q['questid']} step {st['step']} shaman: {now_name} -> {new_name}")


def plan_cleric(db, p):
    for source, qname, now_name, want in CLERIC_SWAP:
        q, st, bc = byclass_step(db, p, source, qname)
        cell = bc.get(CLERIC, {})
        got = ids_in(cell.get("items", ""))
        w = item_id(db, want, ITEM_LEVEL, CLERIC)
        o = item_id(db, now_name, ITEM_LEVEL, CLERIC)
        if not w or not o:
            raise Refused(f"cleric: no level {ITEM_LEVEL} cleric record named {want!r} or {now_name!r}")
        if str(w[0]) in got:
            p.done.append(f"cleric: quest {q['questid']} gives {want}")
            continue
        if str(o[0]) not in got:
            raise Refused(f"cleric: quest {q['questid']}'s cleric items are {cell.get('items')!r}, not {now_name} {o[0]}")
        bc[CLERIC] = dict(cell, items=swap_ids(cell.get("items", ""), o[0], w[0]))
        p.update("cleric", "queststeps", st, {"byclass": json.dumps(bc, sort_keys=True, separators=(",", ":"))},
                 f"quest {q['questid']} step {st['step']} cleric: {now_name} -> {want}")


def plan_tsekar(db, p):
    source, qname, step, kind, first = TSEKAR_QUEST
    q = find_quest(db, source, qname)
    st = [s for s in steps_of(db, q["questid"]) if s["step"] == step]
    if not st or st[0]["kind"] != kind or fold(st[0]["target"]) != fold(first):
        raise Refused(f"tsekar: quest {qname!r} step {step} is not {kind} {first!r}")
    st = st[0]
    ts = named(db, "npcs", "npc_name", TSEKAR)
    if len(ts) != 1:
        raise Refused(f"tsekar: {len(ts)} NPCs named {TSEKAR!r}, not one")
    if ts[0]["npc_type"] not in (0, TALKER):
        raise Refused(f"tsekar: {TSEKAR}'s npc_type is {ts[0]['npc_type']}, not 0 or {TALKER}")
    also = [r for r in rows(db, "SELECT rowid AS _rowid, * FROM queststeptargets WHERE questid = ? AND step = ?",
                            (q["questid"], step))]
    if any(fold(r["target"]) == fold(TSEKAR) for r in also):
        if ts[0]["npc_type"] == TALKER:
            p.done.append(f"tsekar: {TSEKAR} sells the diamond")
            return
    elif also:
        raise Refused(f"tsekar: quest {q['questid']} step {step} has other NPCs already: {[r['target'] for r in also]}")
    else:
        p.insert("tsekar", "queststeptargets", {"questid": q["questid"], "step": step, "target": TSEKAR},
                 f"quest {q['questid']} step {step} at {TSEKAR} too")
    p.update("tsekar", "npcs", ts[0], {"npc_type": TALKER}, f"{TSEKAR} a talker (as Grithmil)")
    p.update("tsekar", "queststeps", st, {"journal": TSEKAR_JOURNAL.format(tunar=st["tunar"])},
             f"quest {q['questid']} step {step}'s journal")
    p.update("tsekar", "quests", q, {"notes": (q["notes"] or "") + NOTE.format(
        TOOL, f"the red diamond bought from {TSEKAR} too (t107)")}, f"quest {q['questid']}'s notes")


PLANNERS = {"renames": plan_renames, "creatures": plan_creatures, "featherlite": plan_featherlite,
            "cleric": plan_cleric, "tsekar": plan_tsekar}


def plan(db, parts):
    p = Plan()
    for part in PARTS:
        if part in parts:
            PLANNERS[part](db, p)
    return p


# ---- apply and undo ----

def audit(db, action, t, key, before, after, detail=DETAIL):
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
               (now(), ACCOUNT, WHO, VIA, action, t, go_json(key), go_json(before or {}), go_json(after or {}), detail))


def clean(r):
    return {k: v for k, v in r.items() if k != "_rowid"}


def apply(db, p):
    db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (id INTEGER PRIMARY KEY AUTOINCREMENT, part TEXT NOT NULL, op TEXT NOT NULL,"
               " tbl TEXT NOT NULL, row INTEGER NOT NULL, before TEXT, after TEXT, seq INTEGER, at TEXT NOT NULL)")
    seqs = {}  # each AUTOINCREMENT table's sequence before its first insert here, for the undo
    new_rows = {}
    n = 0
    for part, op, t, rid, before, after, why in p.changes:
        if op == "update":
            cols = [k for k in after if k != "_rowid" and after[k] != before.get(k)]
            db.execute(f'UPDATE "{t}" SET {", ".join(chr(34) + c + chr(34) + " = ?" for c in cols)} WHERE rowid = ?',
                       [after[c] for c in cols] + [rid])
            db.execute(f"INSERT INTO {LOG} (part, op, tbl, row, before, after, at) VALUES (?, 'update', ?, ?, ?, ?, ?)",
                       (part, t, rid, go_json(clean(before)), go_json(clean(after)), now()))
            audit(db, "update", t, {"rowid": rid}, {c: before.get(c) for c in cols}, {c: after[c] for c in cols},
                  DETAIL + " (" + why + ")")
        else:
            row = dict(after)
            for k, v in list(row.items()):
                if isinstance(v, str) and v.startswith("$new:"):
                    row[k] = new_rows[v[5:]]
            if t not in seqs and has_table(db, "sqlite_sequence"):
                r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = ?", (t,)).fetchone()
                seqs[t] = r[0] if r else 0
            cols = list(row)
            cur = db.execute(f'INSERT INTO "{t}" ({", ".join(chr(34) + c + chr(34) for c in cols)}) VALUES '
                             f'({", ".join("?" for _ in cols)})', [row[c] for c in cols])
            rid = cur.lastrowid
            if t == "npcs":
                new_rows[row["npc_name"]] = rid
            db.execute(f"INSERT INTO {LOG} (part, op, tbl, row, before, after, seq, at) VALUES (?, 'insert', ?, ?, NULL, ?, ?, ?)",
                       (part, t, rid, go_json(row), seqs.get(t), now()))
            audit(db, "insert", t, {"rowid": rid}, None, row, DETAIL + " (" + why + ")")
        n += 1
    return n


def undo(db):
    if not has_table(db, LOG):
        return 0, 0
    back, left = 0, 0
    seqs = {}
    for lid, part, op, t, rid, before, after, s in db.execute(
            f"SELECT id, part, op, tbl, row, before, after, seq FROM {LOG} ORDER BY id DESC").fetchall():
        a = json.loads(after)
        cur = by_rowid(db, t, rid)
        same = cur is not None and all(cur.get(k) == v for k, v in a.items())
        if not same:
            print(f"  {part}: {t} row {rid}: changed since, left")
            left += 1
            continue
        if op == "insert":
            db.execute(f'DELETE FROM "{t}" WHERE rowid = ?', (rid,))
            audit(db, "delete", t, {"rowid": rid}, a, None, "undo: " + DETAIL)
            if s is not None:
                seqs[t] = s if t not in seqs else min(seqs[t], s)
        else:
            b = json.loads(before)
            cols = [k for k in b if b[k] != a.get(k)]
            if cols:
                db.execute(f'UPDATE "{t}" SET {", ".join(chr(34) + c + chr(34) + " = ?" for c in cols)} WHERE rowid = ?',
                           [b[c] for c in cols] + [rid])
            audit(db, "update", t, {"rowid": rid}, {c: a.get(c) for c in cols}, {c: b[c] for c in cols}, "undo: " + DETAIL)
        db.execute(f"DELETE FROM {LOG} WHERE id = ?", (lid,))
        back += 1
    for t, s in seqs.items():  # each sequence back, never under a row that stands
        top = db.execute(f'SELECT COALESCE(MAX(rowid), 0) FROM "{t}"').fetchone()[0]
        if max(s, top) == 0:
            db.execute("DELETE FROM sqlite_sequence WHERE name = ?", (t,))
        else:
            db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = ?", (max(s, top), t))
    if left == 0:
        db.execute(f"DROP TABLE {LOG}")
        db.execute("DELETE FROM sqlite_sequence WHERE name = ?", (LOG,))
    return back, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--part", default=",".join(PARTS))
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    parts = [x.strip() for x in a.part.split(",") if x.strip()]
    bad = [x for x in parts if x not in PARTS]
    if bad:
        ap.error(f"no part {bad}; parts: {', '.join(PARTS)}")
    db = sqlite3.connect(a.db, timeout=5)
    try:
        if a.undo:
            back, left = undo(db)
            db.commit()
            print(f"undo: {back} rows put back, {left} left as changed since")
            return 0
        try:
            p = plan(db, parts)
        except Refused as e:
            print(f"REFUSED, nothing written: {e}")
            return 2
        counts = {}
        for part, op, t, rid, before, after, why in p.changes:
            counts[part] = counts.get(part, 0) + 1
            if op == "update":
                cols = [k for k in after if k != "_rowid" and after[k] != before.get(k)]
                short = {c: (str(after[c])[:70] + ("..." if len(str(after[c])) > 70 else "")) for c in cols if c != "notes"}
                print(f"  {part}: {t} row {rid}: {why}: {short}{' (+ note)' if 'notes' in cols else ''}")
            else:
                print(f"  {part}: {t} new row: {why}")
        for s in p.said:
            print(f"  {s}")
        for d in p.done:
            print(f"  {d}")
        print(f"{len(p.changes)} changes ({', '.join(f'{k} {v}' for k, v in counts.items()) or 'none'}), "
              f"{len(p.done)} parts already done")
        if a.apply:
            n = apply(db, p)
            db.commit()
            print(f"applied: {n} changes")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
