#!/usr/bin/env python3
"""The level 23-37 side quests made as the forum walkthroughs and the work board's research have them, where
side_quest_guides_20261121.py left ours (the owner, 2026-11-21, of that tool's "left as ours" list: "lets fix these";
then, of the board's entries #975, #931 and #1014 on the same quests: "lets do jibs"). Run it AFTER
side_quest_guides_20261121.py --apply (that tool finds its steps by these NPCs' old names).

Six parts, each its own --part (all by default, in this order), logged together in side_quest_guides2_1121_log, undone
together:

bandit   Work board #975, sections 1-2 (Hodstock 23 part 1). The red-haired bandit is the fort's one level 25 arctic
         bandit (row 107270 on both shards, hair colour 4, at the south outpost), keeping the name "arctic bandit" (the
         era's comments: same name, told by his con and hair): level 26, back in 600 s (npcrespawn). Quest 100759 step 2
         is keyed to his row (npcid; a step pinned to a spawn takes its target at that spawn only, so the other 29 drop
         nothing), every kill, and its item record 700739 becomes the Scribbled Note (the bandana was the import's):
         no slot, level 0, no value, no figures, the Runic Recording's icon, NO TRADE kept. Step 1's words, steps 2-3's
         journals as #975. Not here: his wander (npcpaths, laid on the panel's map; until then he stands) and #975's
         sections 3-6 (the pendant's chance and LORE, the zombies' levels, the kinds' loot tables and record fixes, the
         chain's experience).
ravager  Work board #931, section 2 (Oasis 37, Poisontongue). The Blacktongue Ravager: a new npcs row (level 37, at
         (19330, 54.1, 16930) in the east camp, the orcs' body 939332601, size 1, a copy of the nearest Blacktongue orc
         otherwise), out of the world (npcspawnonly), template roamer, class 0. Quest 100782 gets a new step 8, a reach
         of the east camp (19300, 54, 16950, radius 120) whose done spawns him hostile; the old step 8 becomes 9, his
         alone (target Blacktongue Ravager, every kill: the 25% off any Blacktongue orc stops), and 9-13 become 10-14;
         a character on step 8 or later moves up one. Not here: #931's sections 1 and 3-5 (Blacktongue's rarity, the
         kill hits, the kinds' loot, record fixes); its Elmo Haggar is the renames part's.
renames  The NPCs spelt as the guides spell them: Feria Hurkert -> Feria Huckert (her daughter Drew and Jimbo with her:
         one family), Apprentice Buema -> Apprentice Buerma, Razuan Marus -> Razuam Marus, Captain Hogofa -> Captain
         Ogofa, Father Manstein -> Father Manstien, Demetrios Velspane -> Dametrios Velspane, Phoborium -> Phorriun,
         Lelaia Shavrell -> Lelalia Shavererll, Svente Dormio -> Svenle Dormio, Enrique Revnold -> Enrique Reynold,
         Elmo Hagger -> Elmo Haggar. Every place a name keys data (NAME_COLS: the npcs rows and every rule table that
         matches an NPC by its name, any case, as NPCClassKey does, quests.giver, queststeps.target,
         queststeptargets), each value written in its own case style (a lowercase key stays lowercase). Their words go
         with them (TOKENS, whole words: the full names, and a given name said alone): the quests' texts and their
         steps' (offer, accept, decline, declined, started; journal, answer, says, waiting, notyet, reminder) of every
         quest that names one, the names and descriptions of the items those quests' steps want or give (Phoborium's
         Horn becomes Phorriun's Horn), and the renamed NPCs' own npcdialog lines. Not renamed: the faction "Feria
         Hurkert" (a faction, not an NPC: its standings are characters' rows the running server saves back under its own
         name), the guides' "Captain Ravensite" (the same thread also writes "CPT Ravenhite", ours), and quests' notes
         (a line is added). Refused if a new name is already any NPC's or any rule's, or if a gmtriggers step, a quest
         or step event, a camp's kinds or a tour names an old one (not handled here).
featherlite The shaman's Hodstock pieces as t105 names them: Featherlite Helm, Featherlite Tunic and Featherlite Gloves,
         new records copied whole from the Featherweave Coif, Tunic and Gloves given now (no figures in the guide), ids
         from ITEM_RANGE; given in parts 1, 2 and 3 (with Tribal Strength in part 1, as before). The Featherweave records
         are kept.
cleric   The cleric's Hodstock list read as t105 orders it: part 1 the cap (our Goldlight Coif) and Weaken Death, part 2
         the next Goldlight piece of the level (the Goldlight Boots; the guide's second "Goldlight Cap" is a slip), part
         3 the Goldlight Shield (it was part 2's, the boots part 3's: swapped).
tsekar   The red diamond's second seller (t107: "from either Grithmil Darkstone in Moradhim or Tsekar in Gree Rift";
         work board #1014, section 2 and its code ask): Tsekar Kelriss a talker (npc_type 0 -> 128, as Grithmil), quest
         100766 step 3 done at him too (queststeptargets), 1,500 tunar (the era's page; 1,450 was the import's), the
         journal naming both, its words "A red diamond. Take it to Saerk.". NEEDS THE SERVER that takes a talk, show or
         give step at any of its NPCs (fcb26eca, "A talk, show or give step is done at any of its NPCs"); an older one
         keeps the row and ignores it.

Everything goes by NAME: quests by source and name, NPC rows by name (and place for the copies' models), items by
itemname. Dry run by default (every change printed); --apply writes it in one transaction, each row with its gm_audit
line, logged; a second --apply finds nothing to do; --undo puts each row back still as this tool left it (a row changed
since is left and said), deletes what it made and puts each table's id sequence back.

WHEN IT TAKES EFFECT (no restart, with the running server holding fcb26eca): the GM panel's live reads, in this
order, right after the --apply:
  1. npcspawnonly: any edit of the row "blacktongue ravager" (its note) on the panel reads the table again
     (liveSpawnOnly), so the next step makes the Ravager out of the world; done after it instead, he stands in the
     camp until this read;
  2. npcrespawn: any edit of row 107270's (its note): the bandit's 600 s (liveRespawn); else at the next start;
  3. factionhits or npcfaction: any edit of a renamed NPC's row (liveFactions reads factions, factionhits,
     factionstart and npcfaction): the renamed NPCs' faction and kill hits; else they have none until the next start;
  4. npcdialog: any edit of Jimbo Huckert's line (the dialog cache); else at the next start;
  5. reload_npcs: the renames, the bandit's level, the Ravager's row, Tsekar a talker (it reads npcclass and the
     templates too);
  6. a quest reload (a GM panel quest edit): the quests and their steps, the step's other NPC, the item record's name
     (item records reach each client at its login).
The renames must reach the world and the quests together: 5 and 6 back to back. Loot pools are read at each kill.

    python3 -E -s tools/side_quest_guides2_20261121.py --db eqoa.db [--part bandit,ravager,renames,...] [--apply | --undo]
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
# bandit and ravager first: renames folds its words into the rows they change (Plan.view), and the ravager's step
# shifts must run in their own order (the last step first)
PARTS = ("bandit", "ravager", "renames", "featherlite", "cleric", "tsekar")

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

# work board #975 (sections 1-2): the red-haired arctic bandit is row 107270, named "arctic bandit" as the others
BANDIT_QUEST = ("EQOA-Data 23-47/23/p1", "The Redheaded Bandit")
BANDIT_KIND, BANDIT_AT, BANDIT_WAS, BANDIT_LEVEL, BANDIT_RESPAWN = "arctic bandit", (17653.2, 5593.3), 25, 26, 600
BANDIT_JOURNAL = {2: "Kill the red-haired arctic bandit who wanders outside the bandit fort and take the scribbled note he"
                     " carries.",
                  3: "Bring the scribbled note to Feria Hurkert in Hodstock."}
BANDIT_SAYS1 = ("Coin first, always. Now: a red-haired bandit rides with the arctic bandits east of here. Kill him, and"
                " bring what he carries to Feria.")
BANDIT_ITEM_WAS = "Redheaded Bandit's Bandana"
BANDIT_ITEM = {"itemname": "Scribbled Note", "itemdesc": "A scribbled note, taken from the red-haired arctic bandit.",
               "equipslot": -1, "levelreq": 0, "itemcost": 0, "maxhp": 0, "PR": 0, "AR": 0}
BANDIT_ICON_OF = "Runic Recording"

# work board #931 (section 2): the Ravager, spawn-only, raised by a new reach step 8
RAVAGER_QUEST = ("EQOA-Data 23-47/37/rigo", "Poisontongue")
RAVAGER, RAVAGER_KIND, RAVAGER_LEVEL, RAVAGER_BODY = "Blacktongue Ravager", "Blacktongue orc", 37, 939332601
RAVAGER_AT = (19330.0, 54.1, 16930.0)
RAVAGER_STEP = 8
RAVAGER_REACH = (19300.0, 54.0, 16950.0, 120.0)
RAVAGER_REACH_JOURNAL = ("Find the burned village of Ferran's Hope, due south of Bastable, where the Blacktongue orcs"
                         " camp.")
RAVAGER_KILL_JOURNAL = "Kill the Blacktongue Ravager for his lantern."

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
TSEKAR_TUNAR = 1500  # work board #1014 (the era's page)
TSEKAR_SAYS = "A red diamond. Take it to Saerk."
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
        q = p.view("quests", rows(db, "SELECT rowid AS _rowid, * FROM quests WHERE questid = ?", (qid,))[0])
        ch = {k: replace_tokens(q[k]) for k in QUEST_TEXT if k in q and q[k]}
        ch["notes"] = (q["notes"] or "") + NOTE.format(TOOL, "the NPCs' names as the guides spell them")
        p.update("renames", "quests", q, ch, f"quest {qid}'s words")
        for st in [p.view("queststeps", x) for x in steps_of(db, qid)]:
            p.update("renames", "queststeps", st, {k: replace_tokens(st[k]) for k in STEP_TEXT if st[k]},
                     f"quest {qid} step {st['step']}'s words")
            items |= set(ids_in(st["items"])) | set(ids_in(st["rewarditems"]))
            for e in (json.loads(st["events"]) if st["events"] else []):
                if isinstance(e, dict) and e.get("do") == "give":
                    items |= set(ids_in(e.get("items", "")))
    for pid in sorted(items, key=int):
        r = [p.view("itempattern", x) for x in rows(db, "SELECT rowid AS _rowid, * FROM itempattern WHERE patternid = ?", (int(pid),))]
        if r:
            p.update("renames", "itempattern", r[0], {"itemname": replace_tokens(r[0]["itemname"]),
                                                      "itemdesc": replace_tokens(r[0]["itemdesc"])},
                     f"item {pid}'s words")
    # the renamed NPCs' own lines (their rows' names are planned above: the words fold into those changes)
    for o, _ in RENAMES:
        for r in named(db, "npcdialog", "name", o):
            p.update("renames", "npcdialog", r, {"line": replace_tokens(r["line"])}, f"{o}'s line")


def plan_bandit(db, p):
    """Work board #975 sections 1-2: the red-haired arctic bandit's row, his respawn, the step keyed to his row, the
    note."""
    source, qname = BANDIT_QUEST
    q = find_quest(db, source, qname)
    sts = {s["step"]: s for s in steps_of(db, q["questid"])}
    loot = sts.get(2)
    if not loot or loot["kind"] != "loot" or fold(loot["target"]) != fold(BANDIT_KIND):
        raise Refused(f"bandit: quest {qname!r} step 2 is not a loot step of {BANDIT_KIND!r}")
    near = [r for r in named(db, "npcs", "npc_name", BANDIT_KIND)
            if abs(r["x_coord"] - BANDIT_AT[0]) <= 5 and abs(r["z_coord"] - BANDIT_AT[1]) <= 5
            and r["npc_level"] in (BANDIT_WAS, BANDIT_LEVEL)]
    if len(near) != 1:
        raise Refused(f"bandit: {len(near)} {BANDIT_KIND} rows of level {BANDIT_WAS} at {BANDIT_AT}, not one")
    him = near[0]
    item = ids_in(loot["items"])
    rec = rows(db, "SELECT rowid AS _rowid, * FROM itempattern WHERE patternid = ?", (int(item[0]),)) if len(item) == 1 else []
    if not rec or rec[0]["itemname"] not in (BANDIT_ITEM_WAS, BANDIT_ITEM["itemname"]):
        raise Refused(f"bandit: step 2's item is not {BANDIT_ITEM_WAS!r}")
    icon = rows(db, "SELECT itemicon FROM itempattern WHERE itemname = ?", (BANDIT_ICON_OF,))
    if len(icon) != 1:
        raise Refused(f"bandit: {len(icon)} records named {BANDIT_ICON_OF!r}, not one")
    if loot["npcid"] == him["id"] and rec[0]["itemname"] == BANDIT_ITEM["itemname"]:
        p.done.append(f"bandit: quest {q['questid']} step 2 is row {him['id']}'s, the {BANDIT_ITEM['itemname']}")
        return
    if loot["npcid"] not in (0, him["id"]):
        raise Refused(f"bandit: step 2 is pinned to row {loot['npcid']}")
    p.update("bandit", "npcs", him, {"npc_level": BANDIT_LEVEL}, f"the red-haired arctic bandit (row {him['id']}) level {BANDIT_LEVEL}")
    rs = rows(db, "SELECT rowid AS _rowid, * FROM npcrespawn WHERE npc_row = ?", (him["id"],))
    if rs:
        p.update("bandit", "npcrespawn", rs[0], {"seconds": float(BANDIT_RESPAWN)}, f"row {him['id']} back in {BANDIT_RESPAWN} s")
    else:
        p.insert("bandit", "npcrespawn", {"npc_row": him["id"], "seconds": float(BANDIT_RESPAWN),
                                          "note": "work board #975: " + TOOL, "source": "gm"}, f"row {him['id']} back in {BANDIT_RESPAWN} s")
    p.update("bandit", "queststeps", loot, {"npcid": him["id"], "chance": 1.0, "journal": BANDIT_JOURNAL[2]},
             f"quest {q['questid']} step 2: row {him['id']}'s alone, every kill")
    if 1 in sts:
        p.update("bandit", "queststeps", sts[1], {"says": BANDIT_SAYS1}, f"quest {q['questid']} step 1's words")
    if 3 in sts:
        p.update("bandit", "queststeps", sts[3], {"journal": BANDIT_JOURNAL[3]}, f"quest {q['questid']} step 3's journal")
    p.update("bandit", "itempattern", rec[0], dict(BANDIT_ITEM, itemicon=icon[0]["itemicon"]),
             f"item {item[0]}: the {BANDIT_ITEM['itemname']}")
    p.update("bandit", "quests", q, {"notes": (q["notes"] or "") + NOTE.format(
        TOOL, f"work board #975: the red-haired arctic bandit (row {him['id']}) drops the scribbled note, every kill")},
        f"quest {q['questid']}'s notes")
    p.said.append(f"bandit: row {him['id']} at ({him['x_coord']:.1f}, {him['z_coord']:.1f}); his wander (npcpaths) is laid on "
                  "the panel's map (#975): until then he stands")


def plan_ravager(db, p):
    """Work board #931 section 2: the Blacktongue Ravager out of the world, raised by a new reach step of his quest."""
    source, qname = RAVAGER_QUEST
    q = find_quest(db, source, qname)
    qid = q["questid"]
    sts = steps_of(db, qid)
    by = {s["step"]: s for s in sts}
    if by.get(RAVAGER_STEP) and by[RAVAGER_STEP]["kind"] == "reach" and by.get(RAVAGER_STEP + 1) and \
            fold(by[RAVAGER_STEP + 1]["target"]) == fold(RAVAGER) and named(db, "npcs", "npc_name", RAVAGER):
        p.done.append(f"ravager: quest {qid} step {RAVAGER_STEP} raises him, step {RAVAGER_STEP + 1} is his")
        return
    loot = by.get(RAVAGER_STEP)
    if not loot or loot["kind"] != "loot" or fold(loot["target"]) != fold(RAVAGER_KIND):
        raise Refused(f"ravager: quest {qid} step {RAVAGER_STEP} is not a loot step of {RAVAGER_KIND!r}")
    for t, c in NAME_COLS:
        if named(db, t, c, RAVAGER):
            raise Refused(f"ravager: {t}.{c} already names {RAVAGER!r}")
    item = ids_in(loot["items"])
    for t, c in (("loottables", "patternid"), ("npcheld", "patternid")):
        if has_col(db, t, c) and db.execute(f'SELECT count(*) FROM "{t}" WHERE "{c}" = ?', (int(item[0]),)).fetchone()[0]:
            raise Refused(f"ravager: a {t} row gives item {item[0]} too")
    for t in ("queststeptargets", "questdoors", "questwalls"):
        if has_col(db, t, "step") and db.execute(f'SELECT count(*) FROM "{t}" WHERE questid = ? AND step >= ?',
                                                 (qid, RAVAGER_STEP)).fetchone()[0]:
            raise Refused(f"ravager: {t} rows name quest {qid}'s steps from {RAVAGER_STEP}")
    cands = named(db, "npcs", "npc_name", RAVAGER_KIND)
    if not cands:
        raise Refused(f"ravager: no {RAVAGER_KIND}")
    base = min(cands, key=lambda r: ((r["x_coord"] - RAVAGER_AT[0]) ** 2 + (r["z_coord"] - RAVAGER_AT[2]) ** 2, r["id"]))
    row = {k: v for k, v in base.items() if k not in ("_rowid", "id")}
    row.update(npc_name=RAVAGER, npc_level=RAVAGER_LEVEL, x_coord=RAVAGER_AT[0], y_coord=RAVAGER_AT[1],
               z_coord=RAVAGER_AT[2], modelid=RAVAGER_BODY, size=1.0, npc_type=0)
    if "npc_id" in row:
        row["npc_id"] = 0
    p.insert("ravager", "npcs", row, f"{RAVAGER}, a copy of {RAVAGER_KIND} row {base['id']} (#931)")
    p.insert("ravager", "npcspawnonly", {"name": RAVAGER.lower(), "note": "work board #931: raised by quest "
             f"{qid} step {RAVAGER_STEP}; {TOOL}", "source": "gm"}, f"{RAVAGER} out of the world")
    p.insert("ravager", "npctemplate", {"name": RAVAGER.lower(), "template": "roamer", "note": "#931", "source": "gm"},
             f"{RAVAGER}'s template")
    p.insert("ravager", "npcclass", {"name": RAVAGER.lower(), "class": 0, "source": "gm"}, f"{RAVAGER}'s class")
    # the steps from RAVAGER_STEP one on, the last first (the key is questid and step)
    for s in sorted([s for s in sts if s["step"] >= RAVAGER_STEP], key=lambda s: -s["step"]):
        ch = {"step": s["step"] + 1}
        if s["step"] == RAVAGER_STEP:
            ch.update(target=RAVAGER, chance=1.0, journal=RAVAGER_KILL_JOURNAL)
        p.update("ravager", "queststeps", s, ch, f"quest {qid} step {s['step']} -> {s['step'] + 1}")
    new = {k: v for k, v in loot.items() if k != "_rowid"}
    for k in new:
        if k not in ("questid", "step", "kind", "target", "npcid", "items", "tunar", "count", "world", "x", "y", "z",
                     "radius", "journal", "events", "chance"):
            if isinstance(new[k], (int, float)) and not isinstance(new[k], bool):
                new[k] = type(new[k])(0)
            elif isinstance(new[k], str):
                new[k] = ""
    new.update(step=RAVAGER_STEP, kind="reach", target="", npcid=0, items="", tunar=0, count=1, world=0,
               x=RAVAGER_REACH[0], y=RAVAGER_REACH[1], z=RAVAGER_REACH[2], radius=RAVAGER_REACH[3],
               journal=RAVAGER_REACH_JOURNAL, chance=0.0,
               events=json.dumps([{"on": "done", "do": "spawn", "npc": RAVAGER, "hostile": True}], separators=(",", ":")))
    p.insert("ravager", "queststeps", new, f"quest {qid} step {RAVAGER_STEP}: reach the camp, the Ravager rises")
    for c in rows(db, "SELECT rowid AS _rowid, * FROM charquests WHERE questid = ? AND done = 0 AND step >= ?",
                  (qid, RAVAGER_STEP)):
        p.update("ravager", "charquests", c, {"step": c["step"] + 1}, f"character {c['serverid']} on step {c['step']} moves up one")
    p.update("ravager", "quests", q, {"notes": (q["notes"] or "") + NOTE.format(
        TOOL, f"work board #931: the {RAVAGER} raised at the camp (step {RAVAGER_STEP}), his lantern every kill")},
        f"quest {qid}'s notes")


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
    if st["tunar"] not in (1450, TSEKAR_TUNAR):
        raise Refused(f"tsekar: step {step} asks {st['tunar']} tunar, not 1450")
    p.update("tsekar", "queststeps", st, {"tunar": TSEKAR_TUNAR, "says": TSEKAR_SAYS,
                                          "journal": TSEKAR_JOURNAL.format(tunar=TSEKAR_TUNAR)},
             f"quest {q['questid']} step {step}: 1,500 tunar at either seller (#1014)")
    p.update("tsekar", "quests", q, {"notes": (q["notes"] or "") + NOTE.format(
        TOOL, f"the red diamond bought from {TSEKAR} too (t107)")}, f"quest {q['questid']}'s notes")


PLANNERS = {"bandit": plan_bandit, "ravager": plan_ravager, "renames": plan_renames, "featherlite": plan_featherlite,
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
